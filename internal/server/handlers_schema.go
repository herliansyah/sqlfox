package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	sess.SetLatency(latency.Milliseconds())

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocMB := math.Round(float64(m.Alloc)/1024/1024*10) / 10
	info, _ := sess.Session.ServerInfo(ctx)
	if info != nil {
		sess.SetInfo(info)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"latency_ms":  latency.Milliseconds(),
		"memory_mb":   allocMB,
		"server_info": sess.Info(),
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
		sess.SetSchema(schema)
		writeJSON(w, http.StatusOK, schema)
		return
	} else if sess.Schema() == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		schema, err := sess.Session.InspectSchema(ctx, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
			return
		}
		sess.SetSchema(schema)
	}

	writeJSON(w, http.StatusOK, sess.Schema())
}

func (s *Server) handleGetFastCatalog(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	catalog, err := sess.Session.InspectFastCatalog(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect fast catalog: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleGetActiveThreads(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	info, err := sess.Session.ServerInfo(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get server info: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"active_threads": info.ActiveThreads,
		"active_conns":   info.ActiveConns,
		"version":        info.Version,
	})
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
	sess.SetSchema(schema)

	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleRefreshSchema(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	targetDB := r.URL.Query().Get("db")

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	schema, err := sess.Session.InspectSchema(ctx, targetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh schema: "+err.Error())
		return
	}
	sess.SetSchema(schema)

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
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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

func (s *Server) handleGetRoutineDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	routineName := r.PathValue("name")
	routineType := r.URL.Query().Get("type")
	if routineType == "" {
		routineType = "PROCEDURE"
	}

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	ddl, err := sess.GetRoutineDDL(ctx, routineType, routineName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch routine DDL: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"name": routineName,
		"type": routineType,
		"ddl":  ddl,
	})
}

func (s *Server) handleGetEventDDL(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	eventName := r.PathValue("name")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	ddl, err := sess.GetEventDDL(ctx, eventName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch event DDL: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"name": eventName,
		"ddl":  ddl,
	})
}

func (s *Server) handleToggleEvent(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	eventName := r.PathValue("name")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if rejectIfReadOnly(w, sess) {
		return
	}

	var req struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "enable flag required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := sess.ToggleEventStatus(ctx, eventName, req.Enable); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle event: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleGetTablePartitions(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	database := r.URL.Query().Get("db")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	partitions, err := sess.GetTablePartitions(ctx, database, tableName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch partitions: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, partitions)
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
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	findKeyCol := func(tables []driver.TableMetadata) string {
		for _, t := range tables {
			if strings.EqualFold(t.Name, tableName) {
				// 1. Primary key - only if single column
				var pkCols []string
				for _, c := range t.Columns {
					if c.IsPrimaryKey {
						pkCols = append(pkCols, c.Name)
					}
				}
				if len(pkCols) == 1 {
					return pkCols[0]
				}
				if len(pkCols) > 1 {
					return "" // Composite PK: cannot use single-column seek or single WHERE pk=val
				}
				// 2. Single-column unique key fallback
				for _, idx := range t.Indexes {
					if idx.IsUnique && len(idx.Columns) == 1 {
						return idx.Columns[0]
					}
				}
				break
			}
		}
		return ""
	}

	// Find primary key or unique key if exists
	var pkCol string
	tableFoundInCache := false
	if sess.Schema() != nil {
		for _, t := range sess.Schema().Tables {
			if strings.EqualFold(t.Name, tableName) {
				tableFoundInCache = true
				break
			}
		}
		pkCol = findKeyCol(sess.Schema().Tables)
	}

	// If table not found in cache (e.g. multi-db context, newly created table, or stale cache), inspect on demand
	if !tableFoundInCache {
		if schema, err := sess.Session.InspectSchema(ctx, targetDB); err == nil && schema != nil {
			sess.SetSchema(schema)
			pkCol = findKeyCol(schema.Tables)
		}
	}

	targetTable := quoteIdent(tableName, sess.DriverName)
	if targetDB != "" && sess.DriverName != "sqlite" {
		targetTable = quoteIdent(targetDB, sess.DriverName) + "." + targetTable
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
		escSortCol = quoteIdent(sortCol, sess.DriverName)
	}

	if lastKey != "" && pkCol != "" && sortCol == pkCol {
		// Keyset pagination: fast O(1) seek
		op := ">"
		if sortDir == "DESC" {
			op = "<"
		}
		if whereClause != "" {
			querySQL = fmt.Sprintf("SELECT * FROM %s %s AND %s %s %s ORDER BY %s %s LIMIT %d",
				targetTable, whereClause, escSortCol, op, driver.EscapeSQLString(lastKey, sess.DriverName), escSortCol, sortDir, limit)
		} else {
			querySQL = fmt.Sprintf("SELECT * FROM %s WHERE %s %s %s ORDER BY %s %s LIMIT %d",
				targetTable, escSortCol, op, driver.EscapeSQLString(lastKey, sess.DriverName), escSortCol, sortDir, limit)
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
	if err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
		return
	}
	if chunk == nil {
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

	if sess.IsReadOnly() {
		writeError(w, http.StatusForbidden, "Session is locked in READ-ONLY mode")
		return
	}

	type CellEditItem struct {
		Database    string         `json:"database"`
		Table       string         `json:"table"`
		Column      string         `json:"column"`
		Value       any            `json:"value"`
		PKColumn    string         `json:"pk_column"`
		PKValue     any            `json:"pk_value"`
		RowCriteria map[string]any `json:"row_criteria,omitempty"`
	}

	var req struct {
		Database    string         `json:"database"`
		Table       string         `json:"table"`
		Column      string         `json:"column"`
		Value       any            `json:"value"`
		PKColumn    string         `json:"pk_column"`
		PKValue     any            `json:"pk_value"`
		RowCriteria map[string]any `json:"row_criteria,omitempty"`
		Edits       []CellEditItem `json:"edits"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cell edit payload: "+err.Error())
		return
	}

	var items []CellEditItem
	if len(req.Edits) > 0 {
		items = req.Edits
	} else if req.Table != "" && req.Column != "" && ((req.PKColumn != "" && req.PKValue != nil) || len(req.RowCriteria) > 0) {
		items = append(items, CellEditItem{
			Database:    req.Database,
			Table:       req.Table,
			Column:      req.Column,
			Value:       req.Value,
			PKColumn:    req.PKColumn,
			PKValue:     req.PKValue,
			RowCriteria: req.RowCriteria,
		})
	} else {
		writeError(w, http.StatusBadRequest, "table, column, and either pk_column/pk_value or row_criteria are required")
		return
	}

	quoteIdent := func(name string) string { return quoteIdent(name, sess.DriverName) }

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	exec, finish, err := sess.gridWriter(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
		return
	}

	var totalAffected int64
	for _, item := range items {
		tableRef := quoteIdent(item.Table)
		db := item.Database
		if db == "" {
			db = req.Database
		}
		if db != "" && sess.DriverName != "sqlite" {
			tableRef = fmt.Sprintf("%s.%s", quoteIdent(db), quoteIdent(item.Table))
		}

		var updateSQL string
		var auditSQL string
		var args []any
		pIdx := 1
		pVal := driver.Placeholder(sess.DriverName, pIdx)
		pIdx++
		args = append(args, item.Value)
		valLiteral := driver.FormatSQLValueForDriver(item.Value, sess.DriverName)

		if item.PKColumn != "" && item.PKValue != nil && len(item.RowCriteria) == 0 {
			pPK := driver.Placeholder(sess.DriverName, pIdx)
			pIdx++
			args = append(args, item.PKValue)
			pkLiteral := driver.FormatSQLValueForDriver(item.PKValue, sess.DriverName)
			updateSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
				tableRef, quoteIdent(item.Column), pVal, quoteIdent(item.PKColumn), pPK)
			auditSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
				tableRef, quoteIdent(item.Column), valLiteral, quoteIdent(item.PKColumn), pkLiteral)
		} else if len(item.RowCriteria) > 0 {
			var whereParts []string
			var auditWhereParts []string
			for cName, cVal := range item.RowCriteria {
				if cVal == nil {
					whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", quoteIdent(cName)))
					auditWhereParts = append(auditWhereParts, fmt.Sprintf("%s IS NULL", quoteIdent(cName)))
				} else {
					pCrit := driver.Placeholder(sess.DriverName, pIdx)
					pIdx++
					args = append(args, cVal)
					whereParts = append(whereParts, fmt.Sprintf("%s = %s", quoteIdent(cName), pCrit))
					auditWhereParts = append(auditWhereParts, fmt.Sprintf("%s = %s", quoteIdent(cName), driver.FormatSQLValueForDriver(cVal, sess.DriverName)))
				}
			}
			updateSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s",
				tableRef, quoteIdent(item.Column), pVal, singleRowWhere(sess.DriverName, tableRef, strings.Join(whereParts, " AND ")))
			auditSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s",
				tableRef, quoteIdent(item.Column), valLiteral, singleRowWhere(sess.DriverName, tableRef, strings.Join(auditWhereParts, " AND ")))
		} else {
			continue
		}

		cellStart := time.Now()
		res, err := exec.Exec(ctx, updateSQL, args...)
		cellDur := time.Since(cellStart).Milliseconds()
		var cellAffected int64
		if res != nil {
			cellAffected = res.RowsAffected
			totalAffected += res.RowsAffected
		}
		sess.RecordAudit(auditSQL, "grid_cell_edit", cellDur, cellAffected, err, cellStart)
		if err != nil {
			_ = finish(err)
			writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
			return
		}
	}
	if err := finish(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
		return
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

	ro := sess.ToggleReadOnly()
	writeJSON(w, http.StatusOK, map[string]any{
		"connection_id": connID,
		"read_only":     ro,
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

	targetTag := "db://" + connID + "/" + dbName + "/" + tableName
	if opNotes, err := s.vault.ListOperationalNotes(vault.OperationalNoteFilter{ObjectTag: targetTag}); err == nil && len(opNotes) > 0 {
		top := opNotes[0]
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            top.ID,
			"connection_id": connID,
			"database_name": dbName,
			"table_name":    tableName,
			"content":       top.Content,
			"side_note":     top.SideNote,
			"updated_at":    top.UpdatedAt,
		})
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
		Content  string `json:"content"`
		SideNote string `json:"side_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	targetTag := "db://" + connID + "/" + dbName + "/" + tableName
	// Check if operational note already exists for this table tag
	var existingNote *vault.OperationalNote
	if opNotes, err := s.vault.ListOperationalNotes(vault.OperationalNoteFilter{ObjectTag: targetTag}); err == nil && len(opNotes) > 0 {
		for _, n := range opNotes {
			if n.IsPinned || strings.HasPrefix(n.Title, "Runbook:") {
				existingNote = n
				break
			}
		}
		if existingNote == nil && len(opNotes) > 0 {
			existingNote = opNotes[0]
		}
	}

	if existingNote != nil {
		existingNote.Content = req.Content
		if req.SideNote != "" {
			existingNote.SideNote = req.SideNote
		}
		_ = s.vault.UpdateOperationalNote(existingNote)
	} else {
		newOpNote := &vault.OperationalNote{
			Title:      "Runbook: " + tableName,
			Content:    req.Content,
			SideNote:   req.SideNote,
			Status:     "inbox",
			IsPinned:   true,
			CapturedAt: time.Now(),
			ObjectTags: []string{targetTag},
			UserTags:   []string{"#runbook"},
		}
		_ = s.vault.CreateOperationalNote(newOpNote)
	}

	// Also maintain legacy table note for full compatibility
	note := &vault.TableNote{
		ConnectionID: connID,
		DatabaseName: dbName,
		TableName:    tableName,
		Content:      req.Content,
	}
	_ = s.vault.SaveTableNote(note)

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, note)
}
