package server

import (
	"context"
	"encoding/json"
	"fmt"
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

	mockStart := time.Now()
	result, err := mock.Generate(ctx, sess.Session, mock.Config{
		DriverName: sess.DriverName,
		Database:   req.Database,
		Table:      tableName,
		Rows:       req.Rows,
	})
	mockDur := time.Since(mockStart).Milliseconds()
	if err != nil {
		sess.RecordAudit(fmt.Sprintf("-- Mock data generation failed for %s", tableName), "mock_data", mockDur, 0, err, mockStart)
		writeError(w, http.StatusInternalServerError, "mock generation failed: "+err.Error())
		return
	}
	var insertedCount int64
	if result != nil {
		insertedCount = int64(result.Inserted)
	}
	sess.RecordAudit(fmt.Sprintf("-- Mock Data Generation: %d rows inserted into %s", insertedCount, tableName), "mock_data", mockDur, insertedCount, nil, mockStart)
	writeJSON(w, http.StatusOK, result)
}
