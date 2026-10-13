package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"sqlfox/internal/logger"
)

// handleLogsStream provides an SSE (Server-Sent Events) live log telemetry stream.
// ponytail: Uses standard library http.Flusher and EventSource format with zero external dependencies.
func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming unsupported by response writer")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	broadcaster := logger.DefaultBroadcaster()
	if broadcaster == nil {
		writeError(w, http.StatusInternalServerError, "Telemetry broadcaster not available")
		return
	}

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

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
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
