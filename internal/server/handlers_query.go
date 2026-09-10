package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type ExecuteRequest struct {
	SQL            string `json:"sql"`
	Confirmed      bool   `json:"confirmed"`
	ChallengeInput string `json:"challenge_input"`
	QueryID        string `json:"query_id"`
}

func (s *Server) handleInspectQuery(w http.ResponseWriter, r *http.Request) {
	connID := chi.URLParam(r, "id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	res := s.guard.Inspect(req.SQL, sess.Environment)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExplainQuery(w http.ResponseWriter, r *http.Request) {
	connID := chi.URLParam(r, "id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SQL == "" {
		writeError(w, http.StatusBadRequest, "sql is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	plan, err := sess.Session.Explain(ctx, req.SQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "explain failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleExecuteQuery(w http.ResponseWriter, r *http.Request) {
	connID := chi.URLParam(r, "id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.SQL) == "" {
		writeError(w, http.StatusBadRequest, "sql query is required")
		return
	}

	trimmedSQL := strings.TrimSpace(req.SQL)
	upperSQL := strings.ToUpper(trimmedSQL)

	// 1. Safety Guard Inspection
	inspection := s.guard.Inspect(trimmedSQL, sess.Environment)

	// 2. Read-Only Enforcement
	if sess.ReadOnly && inspection.IsDestructive {
		writeError(w, http.StatusForbidden, "Session is locked in READ-ONLY mode. Cannot execute destructive statement.")
		return
	}

	// 3. Challenge Enforcement
	if inspection.RequiresChallenge && !req.Confirmed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blocked":    true,
			"inspection": inspection,
		})
		return
	}

	// Challenge typed table name validation
	if inspection.RequiresChallenge && req.Confirmed && inspection.TargetTable != "" {
		if !strings.EqualFold(strings.TrimSpace(req.ChallengeInput), inspection.TargetTable) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Challenge confirmation failed: expected '%s', got '%s'", inspection.TargetTable, req.ChallengeInput))
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// 4. Pre-DDL Automated Backup
	var backupFile string
	var backupDataIncluded bool
	if inspection.Operation == "DROP" || inspection.Operation == "TRUNCATE" {
		if inspection.TargetTable != "" {
			var err error
			backupFile, backupDataIncluded, err = s.backupMgr.BackupTable(ctx, sess.Session, inspection.TargetTable)
			if err != nil {
				// Log backup warning, proceed with explicit challenge confirmation
			}
		}
	}

	start := time.Now()

	// 5. Route to Exec vs QueryStream
	isSelect := strings.HasPrefix(upperSQL, "SELECT") || strings.HasPrefix(upperSQL, "SHOW") || strings.HasPrefix(upperSQL, "DESC") || strings.HasPrefix(upperSQL, "EXPLAIN")

	if !isSelect {
		// Mutation execution
		execRes, err := sess.Session.Exec(ctx, trimmedSQL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"type":                 "exec",
			"rows_affected":        execRes.RowsAffected,
			"last_insert_id":       execRes.LastInsertID,
			"execution_ms":         execRes.ExecutionMs,
			"message":              execRes.Message,
			"backup_file":          backupFile,
			"backup_data_included": backupDataIncluded,
		})
		return
	}

	// Select stream execution with implicit safe limit
	stream, err := sess.Session.QueryStream(ctx, trimmedSQL, 1000)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer stream.Close()

	cols := stream.Columns()
	var allRows [][]any

	for len(allRows) < 1000 {
		chunk, err := stream.NextChunk(200)
		if len(chunk) > 0 {
			allRows = append(allRows, chunk...)
		}
		if err != nil {
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"type":         "query",
		"columns":      cols,
		"rows":         allRows,
		"row_count":    len(allRows),
		"execution_ms": time.Since(start).Milliseconds(),
	})
}

func (s *Server) handleCancelQuery(w http.ResponseWriter, r *http.Request) {
	connID := chi.URLParam(r, "id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		QueryID string `json:"query_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.QueryID == "" {
		writeError(w, http.StatusBadRequest, "query_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := sess.Session.CancelQuery(ctx, req.QueryID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel query: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
