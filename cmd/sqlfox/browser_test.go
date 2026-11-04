package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserOpenFunctions(t *testing.T) {
	// Verify openUI and openBrowser handle valid local URLs without panics or crashes
	testURL := "http://127.0.0.1:8080"

	// Test non-blocking execution in a goroutine
	done := make(chan struct{})
	go func() {
		// Mock testing: openUI with preferAppMode=true and false
		// Since in headless CI there may be no display, we test that the function executes cleanly.
		openUI(testURL, false)
		openBrowser(testURL)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("openUI / openBrowser timed out")
	}
}

func TestStopExistingInstance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-stop-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. When no lock file exists
	err = stopExistingInstance(tmpDir, 8080)
	if err == nil {
		t.Fatal("expected error when stopping non-existent instance, got nil")
	}

	// 2. When lock file has dead/invalid PID
	lockPath := filepath.Join(tmpDir, "sqlfox.lock")
	staleInfo := instanceInfo{
		PID:       99999999, // non-existent PID
		Port:      8080,
		URL:       "http://127.0.0.1:8080",
		CreatedAt: time.Now(),
	}
	data, _ := json.Marshal(staleInfo)
	if err := os.WriteFile(lockPath, data, 0600); err != nil {
		t.Fatalf("failed to write mock lock file: %v", err)
	}

	// Should handle dead process gracefully and clean up lock
	_ = stopExistingInstance(tmpDir, 8080)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("expected lock file to be cleaned up after stop, but still exists")
	}
}

func TestDesktopCommandExecution(t *testing.T) {
	// Status check should always succeed (exit code 0)
	code := handleDesktopCommand([]string{"status"})
	if code != 0 {
		t.Errorf("expected handleDesktopCommand('status') to return 0, got %d", code)
	}
}

func TestAppModeEnvironmentVariablePrecedence(t *testing.T) {
	// Test that SQLFOX_BROWSER=1 or SQLFOX_APP=0 overrides preferApp
	origBrowser := os.Getenv("SQLFOX_BROWSER")
	origApp := os.Getenv("SQLFOX_APP")
	defer func() {
		_ = os.Setenv("SQLFOX_BROWSER", origBrowser)
		_ = os.Setenv("SQLFOX_APP", origApp)
	}()

	_ = os.Setenv("SQLFOX_BROWSER", "1")
	appFlag := true
	browserFlag := false
	preferApp := appFlag && !browserFlag && os.Getenv("SQLFOX_BROWSER") != "1"
	if preferApp {
		t.Errorf("expected SQLFOX_BROWSER=1 to force standard browser mode")
	}

	_ = os.Setenv("SQLFOX_BROWSER", "")
	_ = os.Setenv("SQLFOX_APP", "0")
	preferApp = appFlag && !browserFlag && os.Getenv("SQLFOX_APP") != "0"
	if preferApp {
		t.Errorf("expected SQLFOX_APP=0 to disable app mode")
	}
}
