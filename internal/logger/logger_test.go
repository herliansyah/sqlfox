package logger

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"silent", slog.Level(999)},
	}

	for _, tt := range tests {
		lvl, ok := LevelFromString(tt.input)
		if !ok && tt.input != "unknown" {
			t.Errorf("expected ok for input %q", tt.input)
		}
		if lvl != tt.expected {
			t.Errorf("input %q: expected %v, got %v", tt.input, tt.expected, lvl)
		}
	}
}

func TestMaskSecret(t *testing.T) {
	if MaskSecret("") != "" {
		t.Errorf("expected empty string for empty input")
	}
	if MaskSecret("my-secret-password") != "[REDACTED]" {
		t.Errorf("expected [REDACTED], got %q", MaskSecret("my-secret-password"))
	}
}

func TestSetupFileSinkAndLogging(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-log-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")
	cfg := Config{
		Level:    "debug",
		Format:   "json",
		FilePath: logPath,
		Silent:   true,
	}

	res, err := Setup(cfg)
	if err != nil {
		t.Fatalf("failed to setup logger: %v", err)
	}
	defer res.Closer.Close()

	res.Logger.Info("test_event", "param", "value123")
	res.Logger.Debug("debug_event", "query", "SELECT 1")

	// Read file
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "test_event") || !strings.Contains(content, "value123") {
		t.Errorf("expected test_event in log file, got:\n%s", content)
	}
	if !strings.Contains(content, "debug_event") || !strings.Contains(content, "SELECT 1") {
		t.Errorf("expected debug_event in log file, got:\n%s", content)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-mw-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "http.log")
	res, err := Setup(Config{
		Level:    "debug",
		Format:   "text",
		FilePath: logPath,
		Silent:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Closer.Close()

	mw := HTTPMiddleware(res.Logger)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/error" {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// 1. Regular API request
	req1 := httptest.NewRequest("GET", "/api/connections", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// 2. Auth request (masked)
	req2 := httptest.NewRequest("POST", "/api/auth/unlock", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// 3. Static asset (filtered out)
	req3 := httptest.NewRequest("GET", "/assets/logo.svg", nil)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logContent := string(data)

	if !strings.Contains(logContent, "/api/connections") {
		t.Errorf("expected /api/connections in log content, got:\n%s", logContent)
	}
	if !strings.Contains(logContent, "[MASKED]") {
		t.Errorf("expected [MASKED] for /api/auth/unlock, got:\n%s", logContent)
	}
	if strings.Contains(logContent, "logo.svg") {
		t.Errorf("static asset logo.svg should have been filtered out")
	}
}

func TestBroadcasterAndLiveStreaming(t *testing.T) {
	b := NewBroadcaster(3)
	ch, unsub := b.Subscribe()
	defer unsub()

	// Emit events
	b.Broadcast(Event{Message: "event 1", Level: "INFO"})
	b.Broadcast(Event{Message: "event 2", Level: "WARN"})
	b.Broadcast(Event{Message: "event 3", Level: "ERROR"})
	b.Broadcast(Event{Message: "event 4", Level: "DEBUG"})

	// Check channel received
	select {
	case evt := <-ch:
		if evt.Message != "event 1" {
			t.Errorf("expected event 1, got %s", evt.Message)
		}
	default:
		t.Errorf("expected event on subscriber channel")
	}

	// Check history (ring buffer capacity was 3, so should hold events 2, 3, 4)
	hist := b.History()
	if len(hist) != 3 {
		t.Fatalf("expected 3 history items, got %d", len(hist))
	}
	if hist[0].Message != "event 2" || hist[1].Message != "event 3" || hist[2].Message != "event 4" {
		t.Errorf("unexpected history order: %+v", hist)
	}
}

func TestDefaultBroadcasterConcurrent(t *testing.T) {
	SetDefaultBroadcaster(nil)
	done := make(chan bool)
	for i := 0; i < 20; i++ {
		go func() {
			b := DefaultBroadcaster()
			if b == nil {
				t.Error("expected non-nil broadcaster")
			}
			done <- true
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

