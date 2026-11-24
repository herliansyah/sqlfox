package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SetOpenBrowserFn configures the system callback to open an external URL in the OS default web browser.
func (s *Server) SetOpenBrowserFn(fn func(string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openBrowserFn = fn
}

// SetShutdownFn configures the graceful shutdown callback for the host HTTP server daemon.
func (s *Server) SetShutdownFn(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shutdownFn = fn
}

// SetRestartFn configures the callback invoked when an in-app update triggers a server restart.
func (s *Server) SetRestartFn(fn func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartFn = fn
}

// hasForwardingHeaders returns true if the request carries any reverse-proxy forwarding header,
// preventing loopback-only host endpoints from being reached through a local reverse proxy.
func hasForwardingHeaders(r *http.Request) bool {
	return r.Header.Get("X-Forwarded-For") != "" ||
		r.Header.Get("X-Real-IP") != "" ||
		r.Header.Get("Forwarded") != ""
}

// handleOpenBrowser handles POST /api/system/open-browser requests to launch the URL in the system browser.
func (s *Server) handleOpenBrowser(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) || s.IsRemoteRequest(r) || hasForwardingHeaders(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "Opening host desktop browser is only permitted from direct local machine sessions",
		})
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		targetURL = "http://localhost:8080"
	}

	u, err := url.Parse(targetURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "Invalid URL: only http and https protocols are permitted",
		})
		return
	}

	s.mu.Lock()
	openFn := s.openBrowserFn
	s.mu.Unlock()
	if openFn != nil {
		go func() {
			_ = openFn(targetURL)
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":  true,
		"url": targetURL,
	})
}

// handleShutdown handles POST /api/system/shutdown requests to gracefully terminate the SqlFox daemon.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) || s.IsRemoteRequest(r) || hasForwardingHeaders(r) {
		writeError(w, http.StatusForbidden, "Shutdown is only permitted from direct local machine sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "SqlFox server is shutting down gracefully...",
	})

	s.mu.Lock()
	shutdownFn := s.shutdownFn
	s.mu.Unlock()
	if shutdownFn != nil {
		go func() {
			time.Sleep(250 * time.Millisecond)
			shutdownFn()
		}()
	}
}
