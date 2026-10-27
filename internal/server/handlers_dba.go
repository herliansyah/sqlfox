package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"sqlfox/internal/driver"
)

func (s *Server) handleGetProcesslist(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := sess.GetProcesslist(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get processlist: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetClusterHealth(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	report, err := sess.InspectClusterHealth(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect cluster health: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleGetServerVariables(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	report, err := sess.InspectServerVariables(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect server variables: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleGetDeadlock(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	report, err := sess.InspectDeadlock(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect deadlock: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleGetStatementDigest(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	limit := 25
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	digest, err := sess.GetStatementDigest(ctx, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get statement digest: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, digest)
}

func (s *Server) handleResetStatementDigest(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := sess.ResetStatementDigest(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset statement digest: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleKillProcess(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	var req struct {
		ThreadID int64  `json:"thread_id"`
		Mode     string `json:"mode"` // "query" or "connection"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ThreadID <= 0 {
		writeError(w, http.StatusBadRequest, "valid thread_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	killConn := req.Mode == "connection"
	if err := sess.KillProcess(ctx, req.ThreadID, killConn); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to kill process: "+err.Error())
		return
	}

	modeLabel := "KILL QUERY"
	if killConn {
		modeLabel = "KILL CONNECTION"
	}
	auditMsg := fmt.Sprintf("%s on thread #%d", modeLabel, req.ThreadID)
	sess.RecordAudit(auditMsg, "processlist", 0, 1, nil)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"thread_id": req.ThreadID,
		"mode":      req.Mode,
	})
}

func (s *Server) handleBatchKillProcess(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	var req struct {
		ThreadIDs []int64 `json:"thread_ids"`
		Mode      string  `json:"mode"` // "query" or "connection"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.ThreadIDs) == 0 {
		writeError(w, http.StatusBadRequest, "at least one thread_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// ponytail: detect immune threads (self connection and critical system processes) to enforce safety guardrails at backend layer
	immunePIDs := make(map[int64]string)
	pl, pErr := sess.GetProcesslist(ctx)
	if pErr != nil || pl == nil {
		// Fail closed: without the processlist we cannot tell which threads are immune.
		msg := "cannot verify immune threads: processlist unavailable"
		if pErr != nil {
			msg += ": " + pErr.Error()
		}
		writeError(w, http.StatusServiceUnavailable, msg)
		return
	}
	if pl.CurrentThreadID > 0 {
		immunePIDs[pl.CurrentThreadID] = "own active connection (self)"
	}
	for _, th := range pl.Threads {
		if th.User == "system user" || th.User == "event_scheduler" || th.Command == "Binlog Dump" {
			immunePIDs[th.ID] = fmt.Sprintf("system process (%s)", th.User)
		}
	}

	killConn := req.Mode == "connection"
	type ItemResult struct {
		ThreadID int64  `json:"thread_id"`
		Success  bool   `json:"success"`
		Error    string `json:"error,omitempty"`
	}

	results := make([]ItemResult, 0, len(req.ThreadIDs))
	successCount := 0
	failedCount := 0

	for _, tid := range req.ThreadIDs {
		if tid <= 0 {
			results = append(results, ItemResult{
				ThreadID: tid,
				Success:  false,
				Error:    "invalid thread id",
			})
			failedCount++
			continue
		}

		if reason, immune := immunePIDs[tid]; immune {
			results = append(results, ItemResult{
				ThreadID: tid,
				Success:  false,
				Error:    "protected immune thread: " + reason,
			})
			failedCount++
			continue
		}

		kErr := sess.KillProcess(ctx, tid, killConn)
		if kErr != nil {
			results = append(results, ItemResult{
				ThreadID: tid,
				Success:  false,
				Error:    kErr.Error(),
			})
			failedCount++
		} else {
			results = append(results, ItemResult{
				ThreadID: tid,
				Success:  true,
			})
			successCount++
		}
	}

	modeLabel := "KILL QUERY"
	if killConn {
		modeLabel = "KILL CONNECTION"
	}
	auditMsg := fmt.Sprintf("BATCH %s on %d thread(s): %d succeeded, %d failed", modeLabel, len(req.ThreadIDs), successCount, failedCount)
	sess.RecordAudit(auditMsg, "processlist", 0, int64(successCount), nil)

	writeJSON(w, http.StatusOK, map[string]any{
		"total":         len(req.ThreadIDs),
		"success_count": successCount,
		"failed_count":  failedCount,
		"mode":          req.Mode,
		"results":       results,
	})
}

func (s *Server) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	if connID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	limit := 100
	offset := 0
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if oStr := r.URL.Query().Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}
	search := r.URL.Query().Get("search")
	source := r.URL.Query().Get("source")

	logs, total, err := s.vault.ListAuditLogs(connID, limit, offset, search, source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit logs: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleClearAuditLogs(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	if connID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	if err := s.vault.ClearAuditLogs(connID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear audit logs: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleGetDatabaseDiagnostics(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := sess.GetDatabaseDiagnostics(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get database diagnostics: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetTableDiagnostics(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	database := r.URL.Query().Get("database")
	table := r.URL.Query().Get("table")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := sess.GetTableDiagnostics(ctx, database, table)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get table diagnostics: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleExecuteMaintenance(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	var req struct {
		Database string   `json:"database"`
		Tables   []string `json:"tables"`
		Action   string   `json:"action"` // OPTIMIZE, ANALYZE, CHECK, VACUUM
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Action == "" {
		writeError(w, http.StatusBadRequest, "valid action is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := sess.ExecuteMaintenance(ctx, req.Database, req.Tables, req.Action)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to execute maintenance: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetRedundantIndexes(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	database := r.URL.Query().Get("database")
	table := r.URL.Query().Get("table")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := sess.GetRedundantIndexes(ctx, database, table)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get redundant indexes: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePeekForeignKey(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	database := r.URL.Query().Get("database")
	table := r.URL.Query().Get("table")
	column := r.URL.Query().Get("column")
	value := r.URL.Query().Get("value")

	if table == "" || column == "" {
		writeError(w, http.StatusBadRequest, "table and column are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := sess.PeekForeignKeyRow(ctx, database, table, column, value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to peek foreign key: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	users, err := sess.GetUserList(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleGetUserGrants(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	user := r.URL.Query().Get("user")
	host := r.URL.Query().Get("host")
	if user == "" || host == "" {
		writeError(w, http.StatusBadRequest, "user and host query parameters are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	grants, err := sess.GetUserGrants(ctx, user, host)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, grants)
}

func (s *Server) handleSaveUser(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	var req struct {
		User     string `json:"user"`
		Host     string `json:"host"`
		Password string `json:"password"`
		IsNew    bool   `json:"is_new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.User == "" || req.Host == "" {
		writeError(w, http.StatusBadRequest, "user and host are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := sess.SaveUser(ctx, req.User, req.Host, req.Password, req.IsNew); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	user := r.URL.Query().Get("user")
	host := r.URL.Query().Get("host")
	if user == "" || host == "" {
		writeError(w, http.StatusBadRequest, "user and host query parameters are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := sess.DropUser(ctx, user, host); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleApplyUserGrants(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	var req driver.UserGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.User == "" || req.Host == "" {
		writeError(w, http.StatusBadRequest, "user and host are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := sess.ApplyUserGrants(ctx, req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
