package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/herliansyah/sqlfox/internal/logger"
)

const maxLogStreamSubscribers = 32

var activeLogStreamSubscribers atomic.Int32

// handleLogsStream provides an SSE (Server-Sent Events) live log telemetry stream.
// ponytail: Uses standard library http.Flusher and EventSource format with zero external dependencies.
func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	if activeLogStreamSubscribers.Add(1) > maxLogStreamSubscribers {
		activeLogStreamSubscribers.Add(-1)
		writeError(w, http.StatusTooManyRequests, "too many concurrent log stream subscribers")
		return
	}
	defer activeLogStreamSubscribers.Add(-1)
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming unsupported by response writer")
		return
	}

	broadcaster := logger.DefaultBroadcaster()
	if broadcaster == nil {
		writeError(w, http.StatusInternalServerError, "Telemetry broadcaster not available")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 1. Cold start: send historical events from Telemetry Ring Buffer
	history := broadcaster.History()
	for _, evt := range history {
		data, err := json.Marshal(evt)
		if err == nil {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}
	flusher.Flush()

	// 2. Subscribe to live stream
	ch, unsub := broadcaster.Subscribe()
	defer unsub()

	isRemote := s.IsRemoteRequest(r)
	remoteToken := ""
	if isRemote {
		remoteToken = extractToken(r)
	}
	var authTick <-chan time.Time
	if isRemote && s.authMgr != nil {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		authTick = ticker.C
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-authTick:
			if !s.authMgr.ValidateToken(remoteToken) {
				return
			}
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

// handleGetLogFile returns the tail of the persistent rotating diagnostic log file on disk.
func (s *Server) handleGetLogFile(w http.ResponseWriter, r *http.Request) {
	tail := 500
	if raw := r.URL.Query().Get("tail"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > 2000 {
				n = 2000
			}
			tail = n
		}
	}

	res, err := logger.ReadLogTail(logger.ActiveLogFile(), 256*1024, tail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read diagnostic log file: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleClientLog persists uncaught frontend errors and telemetry warnings into the server log file.
func (s *Server) handleClientLog(w http.ResponseWriter, r *http.Request) {
	body := io.LimitReader(r.Body, 8*1024)
	var req struct {
		Level   string `json:"level"`
		Module  string `json:"module"`
		Message string `json:"message"`
		Stack   string `json:"stack"`
	}
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid client log payload")
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	mod := strings.TrimSpace(req.Module)
	if mod == "" {
		mod = "frontend"
	}
	lvl, _ := logger.LevelFromString(req.Level)
	if lvl < slog.LevelInfo {
		lvl = slog.LevelInfo
	}
	attrs := []any{"module", mod, "source", "frontend"}
	if st := strings.TrimSpace(req.Stack); st != "" {
		if len(st) > 2048 {
			st = st[:2048]
		}
		attrs = append(attrs, "stack", st)
	}
	slog.Log(r.Context(), lvl, msg, attrs...)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
