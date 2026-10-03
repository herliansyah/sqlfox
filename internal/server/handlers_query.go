package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"sqlfox/internal/advisor"
	"sqlfox/internal/driver"
	"sqlfox/internal/safety"
)

func (s *Server) handleInspectQuery(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
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

	res := safety.Inspect(req.SQL, sess.Environment)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExplainQuery(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		SQL      string `json:"sql"`
		Database string `json:"database"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SQL == "" {
		writeError(w, http.StatusBadRequest, "sql is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if req.Database != "" {
		_ = sess.Session.SelectDatabase(ctx, req.Database)
	}

	plan, err := sess.Session.Explain(ctx, req.SQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "explain failed: "+err.Error())
		return
	}
	if plan == nil {
		plan = &driver.ExplainPlan{Rows: []driver.ExplainRow{}}
	} else if plan.Rows == nil {
		plan.Rows = []driver.ExplainRow{}
	}

	// Fetch table metadata to formulate heuristic index recommendations & bloat warnings
	tblsMap := make(map[string]driver.TableMetadata)
	if sc, sErr := sess.Session.InspectSchema(ctx, req.Database); sErr == nil && sc != nil {
		for _, t := range sc.Tables {
			tblsMap[t.Name] = t
		}
	}
	plan.Advisor = advisor.AdviseWithExplain(req.SQL, sess.DriverName, tblsMap, plan)

	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleAdviseQuery(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		SQL      string `json:"sql"`
		Database string `json:"database"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SQL == "" {
		writeError(w, http.StatusBadRequest, "sql is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if req.Database != "" {
		_ = sess.Session.SelectDatabase(ctx, req.Database)
	}

	tblsMap := make(map[string]driver.TableMetadata)
	if sc, sErr := sess.Session.InspectSchema(ctx, req.Database); sErr == nil && sc != nil {
		for _, t := range sc.Tables {
			tblsMap[t.Name] = t
		}
	}

	plan, _ := sess.Session.Explain(ctx, req.SQL)
	adv := advisor.AdviseWithExplain(req.SQL, sess.DriverName, tblsMap, plan)
	writeJSON(w, http.StatusOK, adv)
}

func (s *Server) handleExecuteQuery(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
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

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := sess.Execute(ctx, req)
	if err != nil {
		if errors.Is(err, ErrReadOnlySession) {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if strings.HasPrefix(err.Error(), "Challenge confirmation failed") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if resp.Blocked {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blocked":    true,
			"inspection": resp.Inspection,
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCancelQuery(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		QueryID string `json:"query_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.QueryID == "" {
		req.QueryID = "active"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := sess.CancelQuery(ctx, req.QueryID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "saturated") {
			status = http.StatusGatewayTimeout
		}
		writeError(w, status, "failed to cancel query: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
