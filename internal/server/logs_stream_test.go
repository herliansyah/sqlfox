package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/herliansyah/sqlfox/internal/logger"
	"github.com/herliansyah/sqlfox/internal/vault"
)

func TestLogsStreamSSE(t *testing.T) {
	// Setup custom broadcaster
	broadcaster := logger.NewBroadcaster(10)
	logger.SetDefaultBroadcaster(broadcaster)

	// Pre-seed 2 events
	broadcaster.Broadcast(logger.Event{
		Level:   "INFO",
		Module:  "server",
		Message: "preseed_1",
	})
	broadcaster.Broadcast(logger.Event{
		Level:   "WARN",
		Module:  "query",
		Message: "preseed_2",
	})

	srv := &Server{}
	ts := httptest.NewServer(http.HandlerFunc(srv.handleLogsStream))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream Content-Type, got %s", ct)
	}

	reader := bufio.NewReader(resp.Body)

	// Helper to read next SSE data line
	readNextData := func() logger.Event {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("error reading stream line: %v", err)
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				var evt logger.Event
				if err := json.Unmarshal([]byte(jsonStr), &evt); err != nil {
					t.Fatalf("failed to unmarshal SSE event: %v", err)
				}
				return evt
			}
		}
	}

	// 1. Should receive preseed_1
	evt1 := readNextData()
	if evt1.Message != "preseed_1" || evt1.Level != "INFO" {
		t.Errorf("unexpected event 1: %+v", evt1)
	}

	// 2. Should receive preseed_2
	evt2 := readNextData()
	if evt2.Message != "preseed_2" || evt2.Level != "WARN" {
		t.Errorf("unexpected event 2: %+v", evt2)
	}

	// 3. Broadcast live event in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		broadcaster.Broadcast(logger.Event{
			Level:   "ERROR",
			Module:  "auth",
			Message: "live_broadcast",
		})
	}()

	evt3 := readNextData()
	if evt3.Message != "live_broadcast" || evt3.Level != "ERROR" {
		t.Errorf("unexpected live event: %+v", evt3)
	}
}

func TestLogsFileAndClientTelemetry(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/sqlfox.log"
	res, err := logger.Setup(logger.Config{
		Level:    "info",
		Format:   "text",
		FilePath: logPath,
		Silent:   true,
	})
	if err != nil {
		t.Fatalf("logger.Setup failed: %v", err)
	}
	defer res.Closer.Close()
	defer logger.SetActiveLogFile("")

	v, err := vault.OpenVault(dir+"/config.db", "testpass")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	srv := NewServer(v, nil, dir, "testpass")
	defer srv.Close()

	// Report a frontend error
	postReq := httptest.NewRequest(http.MethodPost, "/api/logs/client", strings.NewReader(`{"level":"error","module":"frontend","message":"boom_oom_test"}`))
	postReq.RemoteAddr = "127.0.0.1:12345"
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	srv.Router().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/logs/client status=%d body=%s", postRec.Code, postRec.Body.String())
	}

	// Read back via GET /api/logs/file
	getReq := httptest.NewRequest(http.MethodGet, "/api/logs/file?tail=50", nil)
	getReq.RemoteAddr = "127.0.0.1:12345"
	getRec := httptest.NewRecorder()
	srv.Router().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/logs/file status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var payload logger.LogTailResult
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode /api/logs/file: %v", err)
	}
	if payload.Path != logPath || len(payload.Lines) == 0 {
		t.Fatalf("expected non-empty log lines at %s, got %+v", logPath, payload)
	}
	found := false
	for _, l := range payload.Lines {
		if strings.Contains(l, "boom_oom_test") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected boom_oom_test in log tail, got %v", payload.Lines)
	}
}
