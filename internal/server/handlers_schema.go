package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"sqlfox/internal/driver"
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

func (s *Server) handlePingSession(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	latency, err := sess.Session.Ping(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ping failed: "+err.Error())
		return
	}
	sess.LatencyMs = latency.Milliseconds()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocMB := math.Round(float64(m.Alloc)/1024/1024*10) / 10
	info, _ := sess.Session.ServerInfo(ctx)
	if info != nil {
		sess.ServerInfo = info
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"latency_ms":  latency.Milliseconds(),
		"memory_mb":   allocMB,
		"server_info": sess.ServerInfo,
	})
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

	targetDB := r.URL.Query().Get("db")
	if targetDB != "" {
		_ = sess.Session.SelectDatabase(ctx, targetDB)
	}

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

func (s *Server) handlePreflightDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	targetDB := r.URL.Query().Get("db")
	if targetDB != "" {
		_ = sess.Session.SelectDatabase(ctx, targetDB)
	}

	locks, err := sess.Session.InspectTableLocks(ctx, targetDB, tableName)
	if err != nil {
		locks = []driver.TableLockInfo{}
	}

	var rels []driver.TableRelationshipInfo
	if schema, err := sess.Session.InspectSchema(ctx, targetDB); err == nil {
		rels = driver.InspectTableRelationships(schema, tableName)
	} else {
		rels = []driver.TableRelationshipInfo{}
	}

	alterClause := r.URL.Query().Get("alter")
	if alterClause == "" {
		alterClause = "/* your ALTER TABLE statement */"
	}

	host := "127.0.0.1"
	port := 3306
	user := "root"
	if s.vault != nil && sess.ConnectionID != "" {
		if c, err := s.vault.GetConnection(sess.ConnectionID); err == nil && c != nil {
			if c.Host != "" {
				host = c.Host
			}
			if c.Port > 0 {
				port = c.Port
			}
			if c.User != "" {
				user = c.User
			}
		}
	}

	ghostCmd := driver.GenerateOSCCommand("gh-ost", host, port, user, targetDB, tableName, alterClause)
	ptCmd := driver.GenerateOSCCommand("pt-osc", host, port, user, targetDB, tableName, alterClause)

	writeJSON(w, http.StatusOK, map[string]any{
		"table":         tableName,
		"database":      targetDB,
		"active_locks":  locks,
		"relationships": rels,
		"osc_ghost":     ghostCmd,
		"osc_pt":        ptCmd,
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

	targetDB := r.URL.Query().Get("db")
	if targetDB != "" {
		_ = sess.Session.SelectDatabase(ctx, targetDB)
	}

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

	var targetTable string
	if targetDB != "" && sess.DriverName != "sqlite" {
		targetTable = fmt.Sprintf("`%s`.`%s`", strings.ReplaceAll(targetDB, "`", "``"), strings.ReplaceAll(tableName, "`", "``"))
	} else if sess.DriverName == "sqlite" {
		targetTable = fmt.Sprintf(`"%s"`, strings.ReplaceAll(tableName, `"`, `""`))
	} else {
		targetTable = fmt.Sprintf("`%s`", strings.ReplaceAll(tableName, "`", "``"))
	}

	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if err := validateTableFilter(filter); err != nil {
		writeError(w, http.StatusBadRequest, "invalid filter: "+err.Error())
		return
	}

	whereClause := ""
	if filter != "" {
		whereClause = fmt.Sprintf("WHERE (%s)", filter)
	}

	var querySQL string
	if sortCol == "" && pkCol != "" {
		sortCol = pkCol
	}

	var escSortCol string
	if sortCol != "" {
		if sess.DriverName == "sqlite" {
			escSortCol = fmt.Sprintf(`"%s"`, strings.ReplaceAll(sortCol, `"`, `""`))
		} else {
			escSortCol = fmt.Sprintf("`%s`", strings.ReplaceAll(sortCol, "`", "``"))
		}
	}

	if lastKey != "" && pkCol != "" && sortCol == pkCol {
		// Keyset pagination: fast O(1) seek
		op := ">"
		if sortDir == "DESC" {
			op = "<"
		}
		if whereClause != "" {
			querySQL = fmt.Sprintf("SELECT * FROM %s %s AND %s %s '%s' ORDER BY %s %s LIMIT %d",
				targetTable, whereClause, escSortCol, op, strings.ReplaceAll(lastKey, "'", "''"), escSortCol, sortDir, limit)
		} else {
			querySQL = fmt.Sprintf("SELECT * FROM %s WHERE %s %s '%s' ORDER BY %s %s LIMIT %d",
				targetTable, escSortCol, op, strings.ReplaceAll(lastKey, "'", "''"), escSortCol, sortDir, limit)
		}
	} else if sortCol != "" {
		// Bounded offset
		if whereClause != "" {
			querySQL = fmt.Sprintf("SELECT * FROM %s %s ORDER BY %s %s LIMIT %d OFFSET %d",
				targetTable, whereClause, escSortCol, sortDir, limit, offset)
		} else {
			querySQL = fmt.Sprintf("SELECT * FROM %s ORDER BY %s %s LIMIT %d OFFSET %d",
				targetTable, escSortCol, sortDir, limit, offset)
		}
	} else {
		if whereClause != "" {
			querySQL = fmt.Sprintf("SELECT * FROM %s %s LIMIT %d OFFSET %d", targetTable, whereClause, limit, offset)
		} else {
			querySQL = fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", targetTable, limit, offset)
		}
	}

	stream, err := sess.Session.QueryStream(ctx, querySQL, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
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
		"filter":  filter,
	})
}

// forbiddenFilterRegex detects statement terminators and top-level destructive SQL verbs.
// ponytail: regex token guard protects server-side filter pushdown at the trust boundary.
var forbiddenFilterRegex = regexp.MustCompile(`(?i)\b(UNION|INTO\s+(OUTFILE|DUMPFILE)|DROP|DELETE|UPDATE|INSERT|ALTER|TRUNCATE|GRANT|REVOKE|SHUTDOWN)\b`)

func validateTableFilter(filter string) error {
	trimmed := strings.TrimSpace(filter)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, ";") {
		return errors.New("semicolons are not permitted in filter expressions")
	}
	if forbiddenFilterRegex.MatchString(trimmed) {
		return errors.New("filter expression contains forbidden statements or clauses")
	}
	return nil
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

	type CellEditItem struct {
		Database string `json:"database"`
		Table    string `json:"table"`
		Column   string `json:"column"`
		Value    any    `json:"value"`
		PKColumn string `json:"pk_column"`
		PKValue  any    `json:"pk_value"`
	}

	var req struct {
		Database string         `json:"database"`
		Table    string         `json:"table"`
		Column   string         `json:"column"`
		Value    any            `json:"value"`
		PKColumn string         `json:"pk_column"`
		PKValue  any            `json:"pk_value"`
		Edits    []CellEditItem `json:"edits"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cell edit payload: "+err.Error())
		return
	}

	var items []CellEditItem
	if len(req.Edits) > 0 {
		items = req.Edits
	} else if req.Table != "" && req.Column != "" && req.PKColumn != "" && req.PKValue != nil {
		items = append(items, CellEditItem{
			Database: req.Database,
			Table:    req.Table,
			Column:   req.Column,
			Value:    req.Value,
			PKColumn: req.PKColumn,
			PKValue:  req.PKValue,
		})
	} else {
		writeError(w, http.StatusBadRequest, "table, column, pk_column, and pk_value (or edits array) are required")
		return
	}

	quoteIdent := func(name string) string { return quoteIdent(name, sess.DriverName) }

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var totalAffected int64
	for _, item := range items {
		valStr := driver.FormatSQLValue(item.Value)
		pkStr := driver.FormatSQLValue(item.PKValue)

		tableRef := quoteIdent(item.Table)
		db := item.Database
		if db == "" {
			db = req.Database
		}
		if db != "" {
			tableRef = fmt.Sprintf("%s.%s", quoteIdent(db), quoteIdent(item.Table))
		}

		updateSQL := fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
			tableRef, quoteIdent(item.Column), valStr, quoteIdent(item.PKColumn), pkStr)

		res, err := sess.Session.Exec(ctx, updateSQL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
			return
		}
		if res != nil {
			totalAffected += res.RowsAffected
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"rows_affected": totalAffected,
		"applied_count": len(items),
	})
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

