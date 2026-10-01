package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
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
