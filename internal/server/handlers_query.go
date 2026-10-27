package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sqlfox/internal/advisor"
	"sqlfox/internal/driver"
)

func (s *Server) handleInspectQuery(w http.ResponseWriter, r *http.Request) {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	targetDB := req.Database
	if targetDB == "" && sess.CachedSchema != nil {
		targetDB = sess.CachedSchema.DatabaseName
	}
	res, _, err := inspectAllStatements(req.SQL, sess.Environment, sess.DriverName, targetDB)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
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
		if err := sess.Session.SelectDatabase(ctx, req.Database); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
	sc := sess.CachedSchema
	if sc == nil || (req.Database != "" && sc.DatabaseName != req.Database) {
		if newSc, sErr := sess.Session.InspectSchema(ctx, req.Database); sErr == nil && newSc != nil {
			sc = newSc
			sess.CachedSchema = newSc
		}
	}
	if sc != nil {
		for _, t := range sc.Tables {
			tblsMap[t.Name] = t
		}
	}
	dialect := sess.DriverName
	if sess.ServerInfo == nil {
		if sInfo, sErr := sess.Session.ServerInfo(ctx); sErr == nil && sInfo != nil {
			sess.ServerInfo = sInfo
		}
	}
	if sess.ServerInfo != nil && sess.ServerInfo.Version != "" {
		dialect = fmt.Sprintf("%s %s", sess.DriverName, sess.ServerInfo.Version)
	}
	plan.Advisor = advisor.AdviseWithExplain(req.SQL, dialect, tblsMap, plan)

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
		if err := sess.Session.SelectDatabase(ctx, req.Database); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	tblsMap := make(map[string]driver.TableMetadata)
	advSc := sess.CachedSchema
	if advSc == nil || (req.Database != "" && advSc.DatabaseName != req.Database) {
		if newSc, sErr := sess.Session.InspectSchema(ctx, req.Database); sErr == nil && newSc != nil {
			advSc = newSc
			sess.CachedSchema = newSc
		}
	}
	if advSc != nil {
		for _, t := range advSc.Tables {
			tblsMap[t.Name] = t
		}
	}

	plan, _ := sess.Session.Explain(ctx, req.SQL)
	dialect := sess.DriverName
	if sess.ServerInfo == nil {
		if sInfo, sErr := sess.Session.ServerInfo(ctx); sErr == nil && sInfo != nil {
			sess.ServerInfo = sInfo
		}
	}
	if sess.ServerInfo != nil && sess.ServerInfo.Version != "" {
		dialect = fmt.Sprintf("%s %s", sess.DriverName, sess.ServerInfo.Version)
	}
	adv := advisor.AdviseWithExplain(req.SQL, dialect, tblsMap, plan)
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

func (s *Server) handleExecuteStream(w http.ResponseWriter, r *http.Request) {
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

	if _, ok := w.(http.Flusher); !ok {
		writeError(w, http.StatusInternalServerError, "Streaming unsupported")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Only set streaming headers once pre-execution gate passes
	headerWritten := false
	writeStreamHeaders := func() {
		if !headerWritten {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-Accel-Buffering", "no")
			headerWritten = true
		}
	}

	// Wrapper writer that sets headers on first write if not blocked
	sw := &streamResponseWriter{
		ResponseWriter: w,
		beforeWrite:    writeStreamHeaders,
	}

	resp, err := sess.ExecuteStream(ctx, req, sw, sw)
	if err != nil {
		if headerWritten {
			// Stream headers already sent; error was emitted as an NDJSON line
			return
		}
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

	if resp != nil && resp.Blocked {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blocked":    true,
			"inspection": resp.Inspection,
		})
		return
	}
}

type streamResponseWriter struct {
	http.ResponseWriter
	beforeWrite func()
}

func (s *streamResponseWriter) Write(p []byte) (int, error) {
	if s.beforeWrite != nil {
		s.beforeWrite()
	}
	return s.ResponseWriter.Write(p)
}

func (s *streamResponseWriter) Flush() {
	if s.beforeWrite != nil {
		s.beforeWrite()
	}
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
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
