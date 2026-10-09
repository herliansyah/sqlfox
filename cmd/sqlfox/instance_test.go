package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSingleInstanceGuard(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-instance-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Initial state: no lock file, no process
	url, pid, running := checkExistingInstance(tempDir, 0)
	if running {
		t.Fatalf("expected running=false initially, got true (url=%s, pid=%d)", url, pid)
	}

	// 2. Start mock SqlFox HTTP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mockPort := listener.Addr().(*net.TCPAddr).Port

	mockServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth/status" {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-SqlFox-App", "sqlfox")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"app":       "sqlfox",
					"is_remote": false,
					"unlocked":  true,
				})
				return
			}
			http.NotFound(w, r)
		}),
	}
	go func() {
		_ = mockServer.Serve(listener)
	}()
	defer mockServer.Close()

	// 3. Write instance lock
	mockURL := "http://localhost:12345"
	cleanup, err := writeInstanceLock(tempDir, mockPort, mockURL)
	if err != nil {
		t.Fatalf("failed to write instance lock: %v", err)
	}

	// 4. Check existing instance: should detect active mock SqlFox
	url, pid, running = checkExistingInstance(tempDir, 0)
	if !running {
		t.Fatalf("expected running=true, got false")
	}
	if url != mockURL {
		t.Errorf("expected url=%q, got %q", mockURL, url)
	}
	if pid != os.Getpid() {
		t.Errorf("expected pid=%d, got %d", os.Getpid(), pid)
	}

	// 5. Cleanup lock file
	cleanup()
	lockPath := filepath.Join(tempDir, "sqlfox.lock")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("expected lock file to be removed after cleanup")
	}

	// 6. Test stale lock cleanup (simulate process killed without cleanup)
	deadPortListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := deadPortListener.Addr().(*net.TCPAddr).Port
	deadPortListener.Close() // Immediately close so port is dead

	staleInfo := instanceInfo{
		PID:       999999,
		Port:      deadPort,
		URL:       "http://localhost:9999",
		CreatedAt: time.Now(),
	}
	staleData, _ := json.Marshal(staleInfo)
	_ = os.WriteFile(lockPath, staleData, 0600)

	url, pid, running = checkExistingInstance(tempDir, 0)
	if running {
		t.Fatalf("expected running=false for dead port, got true")
	}
	// The stale lock file should have been cleaned up automatically
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("expected stale lock file to be automatically removed")
	}

	// 7. Test fallback direct probe on default port when lock file is absent
	url, _, running = checkExistingInstance(tempDir, mockPort)
	if !running {
		t.Fatalf("expected fallback probe to detect running instance on mockPort")
	}
	expectedFallbackURL := "http://localhost:" + string(rune(mockPort))
	_ = expectedFallbackURL
}

func TestThirdPartyServiceOnPortNotDetectedAsSqlFox(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-thirdparty-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Simulate a third-party application (e.g. auth service, mock API, etc.)
	// that returns 200 OK with generic JSON containing unlocked: true or status: ok
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	thirdPartyPort := listener.Addr().(*net.TCPAddr).Port

	thirdPartyServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"service":  "some-other-app",
				"unlocked": true,
			})
		}),
	}
	go func() {
		_ = thirdPartyServer.Serve(listener)
	}()
	defer thirdPartyServer.Close()

	// Direct probe on the third party port should NOT identify it as SqlFox
	url, pid, running := checkExistingInstance(tempDir, thirdPartyPort)
	if running {
		t.Fatalf("expected running=false for third party application, got true (url=%s, pid=%d)", url, pid)
	}
}

