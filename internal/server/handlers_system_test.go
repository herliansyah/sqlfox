package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSystemOpenBrowserHandler(t *testing.T) {
	s := &Server{}
	var openedURL string
	done := make(chan struct{}, 1)
	s.SetOpenBrowserFn(func(u string) error {
		openedURL = u
		done <- struct{}{}
		return nil
	})

	body, _ := json.Marshal(map[string]string{"url": "http://localhost:8080/dashboard"})
	req := httptest.NewRequest(http.MethodPost, "/api/system/open-browser", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()

	s.handleOpenBrowser(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	select {
	case <-done:
		if openedURL != "http://localhost:8080/dashboard" {
			t.Errorf("expected opened URL 'http://localhost:8080/dashboard', got %q", openedURL)
		}
	case <-time.After(time.Second):
		t.Fatal("openBrowserFn was not invoked")
	}

	// 2. Reject remote non-loopback IP
	{
		remoteReq := httptest.NewRequest(http.MethodPost, "/api/system/open-browser", bytes.NewReader(body))
		remoteReq.RemoteAddr = "192.168.1.100:54321"
		rw := httptest.NewRecorder()
		s.handleOpenBrowser(rw, remoteReq)
		if rw.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for remote caller, got %d", rw.Code)
		}
	}

	// 3. Reject invalid URL scheme (e.g., file://, javascript:, executable)
	{
		badBody, _ := json.Marshal(map[string]string{"url": "file:///etc/passwd"})
		badReq := httptest.NewRequest(http.MethodPost, "/api/system/open-browser", bytes.NewReader(badBody))
		badReq.RemoteAddr = "127.0.0.1:54321"
		rw := httptest.NewRecorder()
		s.handleOpenBrowser(rw, badReq)
		if rw.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for file:// scheme, got %d", rw.Code)
		}
	}
}

func TestSystemShutdownHandler(t *testing.T) {
	s := &Server{}
	shutdownTriggered := make(chan struct{}, 1)
	s.SetShutdownFn(func() {
		shutdownTriggered <- struct{}{}
	})

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", nil)
	w := httptest.NewRecorder()

	s.handleShutdown(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	select {
	case <-shutdownTriggered:
		// shutdownFn successfully called asynchronously
	case <-time.After(1 * time.Second):
		t.Fatal("shutdownFn was not invoked within timeout")
	}
}
