package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type InsertRowRequest struct {
	Database string         `json:"database"`
	Table    string         `json:"table"`
	Data     map[string]any `json:"data"`
}

type DeleteRowsRequest struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	PKColumn string `json:"pk_column"`
	PKValues []any  `json:"pk_values"`
}

func (s *Server) handleInsertRow(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if sess.ReadOnly {
		writeError(w, http.StatusForbidden, "Session is locked in READ-ONLY mode")
		return
	}

	var req InsertRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid insert-row payload: "+err.Error())
		return
	}

	if req.Table == "" || len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "table and non-empty data map are required")
		return
	}

	targetTable := quoteIdent(req.Table, sess.DriverName)
	if req.Database != "" {
		targetTable = quoteIdent(req.Database, sess.DriverName) + "." + targetTable
	}

	var cols []string
	var placeholders []string
	var args []any
	for col, val := range req.Data {
		cols = append(cols, quoteIdent(col, sess.DriverName))
		placeholders = append(placeholders, "?")
		args = append(args, val)
	}

	sqlQuery := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", targetTable, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	_ = sess.UseDatabase(ctx, req.Database)

	res, err := sess.Session.Exec(ctx, sqlQuery, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rows_affected":  res.RowsAffected,
		"last_insert_id": res.LastInsertID,
		"execution_ms":   res.ExecutionMs,
	})
}

func (s *Server) handleDeleteRows(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if sess.ReadOnly {
		writeError(w, http.StatusForbidden, "Session is locked in READ-ONLY mode")
		return
	}

	var req DeleteRowsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid delete-rows payload: "+err.Error())
		return
	}

	if req.Table == "" || req.PKColumn == "" || len(req.PKValues) == 0 {
		writeError(w, http.StatusBadRequest, "table, pk_column, and non-empty pk_values are required")
		return
	}

	targetTable := quoteIdent(req.Table, sess.DriverName)
	if req.Database != "" {
		targetTable = quoteIdent(req.Database, sess.DriverName) + "." + targetTable
	}
	targetPK := quoteIdent(req.PKColumn, sess.DriverName)

	placeholders := make([]string, len(req.PKValues))
	args := make([]any, len(req.PKValues))
	for i, v := range req.PKValues {
		placeholders[i] = "?"
		args[i] = v
	}

	sqlQuery := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)", targetTable, targetPK, strings.Join(placeholders, ", "))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	_ = sess.UseDatabase(ctx, req.Database)

	res, err := sess.Session.Exec(ctx, sqlQuery, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rows_affected": res.RowsAffected,
		"execution_ms":  res.ExecutionMs,
	})
}
