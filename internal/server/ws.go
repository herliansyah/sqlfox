package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Localhost only
	},
}

type WSQueryRequest struct {
	Action    string `json:"action"` // "start" or "cancel"
	SQL       string `json:"sql"`
	BatchSize int    `json:"batch_size"`
	QueryID   string `json:"query_id"`
}

func (s *Server) handleStreamWebSocket(w http.ResponseWriter, r *http.Request) {
	connID := chi.URLParam(r, "id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	for {
		var req WSQueryRequest
		if err := ws.ReadJSON(&req); err != nil {
			break
		}

		if req.Action == "cancel" && req.QueryID != "" {
			_ = sess.Session.CancelQuery(r.Context(), req.QueryID)
			_ = ws.WriteJSON(map[string]any{"type": "cancelled", "query_id": req.QueryID})
			continue
		}

		if req.Action == "start" && req.SQL != "" {
			batchSize := req.BatchSize
			if batchSize <= 0 || batchSize > 1000 {
				batchSize = 100
			}

			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()

			start := time.Now()
			stream, err := sess.Session.QueryStream(ctx, req.SQL, 100000)
			if err != nil {
				_ = ws.WriteJSON(map[string]any{
					"type":  "error",
					"error": err.Error(),
				})
				continue
			}

			// Send column metadata
			_ = ws.WriteJSON(map[string]any{
				"type":    "columns",
				"columns": stream.Columns(),
			})

			totalRows := 0
			for {
				chunk, err := stream.NextChunk(batchSize)
				if len(chunk) > 0 {
					totalRows += len(chunk)
					writeErr := ws.WriteJSON(map[string]any{
						"type": "chunk",
						"rows": chunk,
					})
					if writeErr != nil {
						break
					}
				}
				if err != nil {
					break
				}
			}
			stream.Close()

			_ = ws.WriteJSON(map[string]any{
				"type":        "done",
				"total_rows":  totalRows,
				"duration_ms": time.Since(start).Milliseconds(),
			})
		}
	}
}
