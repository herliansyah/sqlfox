package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"sqlfox/internal/mock"
)

func (s *Server) handleGenerateMockData(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		Rows     int    `json:"rows"`
		Database string `json:"database"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Rows <= 0 {
		writeError(w, http.StatusBadRequest, "rows must be a positive integer")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := mock.Generate(ctx, sess.Session, mock.Config{
		DriverName: sess.DriverName,
		Database:   req.Database,
		Table:      tableName,
		Rows:       req.Rows,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mock generation failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
