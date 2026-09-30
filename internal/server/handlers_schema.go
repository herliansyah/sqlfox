package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sqlfox/internal/vault"
)

func (s *Server) handleConnectSession(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	if connID == "" {
		writeError(w, http.StatusBadRequest, "connection id required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	sess, err := s.sessionMgr.ConnectSession(ctx, connID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	targetDB := r.URL.Query().Get("db")
	if targetDB != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		schema, err := sess.Session.InspectSchema(ctx, targetDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, schema)
		return
	} else if sess.CachedSchema == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		schema, err := sess.Session.InspectSchema(ctx, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
			return
		}
		sess.CachedSchema = schema
	}

	writeJSON(w, http.StatusOK, sess.CachedSchema)
}

func (s *Server) handleSelectDatabase(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		Database string `json:"database"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Database == "" {
		writeError(w, http.StatusBadRequest, "database name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := sess.Session.SelectDatabase(ctx, req.Database); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	schema, err := sess.Session.InspectSchema(ctx, req.Database)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
		return
	}
	sess.CachedSchema = schema

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleRefreshSchema(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	schema, err := sess.Session.InspectSchema(ctx, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh schema: "+err.Error())
		return
	}
	sess.CachedSchema = schema

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleGetTableDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	ddl, err := sess.Session.GetTableDDL(ctx, tableName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch DDL: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"table": tableName,
		"ddl":   ddl,
	})
}

func (s *Server) handleGetTableRows(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 500 {
			limit = val
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	sortCol := r.URL.Query().Get("sort_col")
	sortDir := strings.ToUpper(r.URL.Query().Get("sort_dir"))
	if sortDir != "DESC" {
		sortDir = "ASC"
	}

	// Keyset marker if provided
	lastKey := r.URL.Query().Get("last_key")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Find primary key if exists
	var pkCol string
	if sess.CachedSchema != nil {
		for _, t := range sess.CachedSchema.Tables {
			if t.Name == tableName {
				for _, c := range t.Columns {
					if c.IsPrimaryKey {
						pkCol = c.Name
						break
					}
				}
				break
			}
		}
	}

	var querySQL string
	if sortCol == "" && pkCol != "" {
		sortCol = pkCol
	}

	if lastKey != "" && pkCol != "" && sortCol == pkCol {
		// Keyset pagination: fast O(1) seek
		op := ">"
		if sortDir == "DESC" {
			op = "<"
		}
		querySQL = fmt.Sprintf("SELECT * FROM %s WHERE %s %s %s ORDER BY %s %s LIMIT %d",
			tableName, pkCol, op, lastKey, pkCol, sortDir, limit)
	} else if sortCol != "" {
		// Bounded offset
		querySQL = fmt.Sprintf("SELECT * FROM %s ORDER BY %s %s LIMIT %d OFFSET %d",
			tableName, sortCol, sortDir, limit, offset)
	} else {
		querySQL = fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", tableName, limit, offset)
	}

	stream, err := sess.Session.QueryStream(ctx, querySQL, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed: "+err.Error())
		return
	}
	defer stream.Close()

	cols := stream.Columns()
	chunk, err := stream.NextChunk(limit)
	if err != nil && len(chunk) == 0 {
		chunk = [][]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"columns": cols,
		"rows":    chunk,
		"limit":   limit,
		"offset":  offset,
		"pk_col":  pkCol,
	})
}

func (s *Server) handleCellEdit(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Table    string `json:"table"`
		Column   string `json:"column"`
		Value    any    `json:"value"`
		PKColumn string `json:"pk_column"`
		PKValue  any    `json:"pk_value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cell edit payload: "+err.Error())
		return
	}

	if req.Table == "" || req.Column == "" || req.PKColumn == "" || req.PKValue == nil {
		writeError(w, http.StatusBadRequest, "table, column, pk_column, and pk_value are required")
		return
	}

	// Format safe single-cell update statement
	var valStr string
	if req.Value == nil {
		valStr = "NULL"
	} else {
		valStr = fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", req.Value), "'", "''"))
	}
	pkStr := fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", req.PKValue), "'", "''"))

	updateSQL := fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
		req.Table, req.Column, valStr, req.PKColumn, pkStr)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := sess.Session.Exec(ctx, updateSQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleToggleReadOnly(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	sess.ReadOnly = !sess.ReadOnly
	writeJSON(w, http.StatusOK, map[string]any{
		"connection_id": connID,
		"read_only":     sess.ReadOnly,
	})
}

func (s *Server) handleGetTableNote(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")
	dbName := r.URL.Query().Get("db")

	if connID == "" || tableName == "" {
		writeError(w, http.StatusBadRequest, "connection id and table name are required")
		return
	}

	note, err := s.vault.GetTableNote(connID, dbName, tableName)
	if err != nil {
		if err == vault.ErrTableNoteNotFound {
			writeJSON(w, http.StatusOK, map[string]any{
				"content":    "",
				"updated_at": nil,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleSaveTableNote(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")
	dbName := r.URL.Query().Get("db")

	if connID == "" || tableName == "" {
		writeError(w, http.StatusBadRequest, "connection id and table name are required")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	note := &vault.TableNote{
		ConnectionID: connID,
		DatabaseName: dbName,
		TableName:    tableName,
		Content:      req.Content,
	}

	if err := s.vault.SaveTableNote(note); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save table note: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, note)
}

