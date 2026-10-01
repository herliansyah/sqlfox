package server

import (
	"context"
	"encoding/json"
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

func (s *Server) handleKillProcess(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
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

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"thread_id": req.ThreadID,
		"mode":      req.Mode,
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

	logs, total, err := s.vault.ListAuditLogs(connID, limit, offset, search)
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

