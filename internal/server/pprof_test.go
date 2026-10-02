package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPprofEndpoints(t *testing.T) {
	// Case 1: SQLFOX_PPROF is disabled by default
	_ = os.Unsetenv("SQLFOX_PPROF")
	s1 := &Server{}
	mux1 := http.NewServeMux()
	s1.registerPprof(mux1)

	req1 := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	rec1 := httptest.NewRecorder()
	mux1.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when SQLFOX_PPROF is unset, got %d", rec1.Code)
	}

	// Case 2: SQLFOX_PPROF is enabled
	_ = os.Setenv("SQLFOX_PPROF", "1")
	defer func() { _ = os.Unsetenv("SQLFOX_PPROF") }()

	s2 := &Server{}
	mux2 := http.NewServeMux()
	s2.registerPprof(mux2)

	// Sub-test 2a: Request from loopback should succeed
	reqLoopback := httptest.NewRequest("GET", "/debug/pprof/", nil)
	reqLoopback.RemoteAddr = "127.0.0.1:12345"
	recLoopback := httptest.NewRecorder()
	mux2.ServeHTTP(recLoopback, reqLoopback)
	if recLoopback.Code != http.StatusOK {
		t.Fatalf("expected 200 from loopback, got %d", recLoopback.Code)
	}

	// Sub-test 2b: Request from non-loopback must be forbidden
	reqRemote := httptest.NewRequest("GET", "/debug/pprof/", nil)
	reqRemote.RemoteAddr = "192.168.1.100:54321"
	recRemote := httptest.NewRecorder()
	mux2.ServeHTTP(recRemote, reqRemote)
	if recRemote.Code != http.StatusForbidden {
		t.Fatalf("expected 403 from remote client, got %d", recRemote.Code)
	}
}
