package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

// formatSQLValue formats a Go value into a valid SQL literal for dumps and inserts.
func formatSQLValue(val any, driverName string) string {
	return driver.FormatSQLValueForDriver(val, driverName)
}

func (s *Server) handleExportDump(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	targetDB := r.URL.Query().Get("db")
	targetTable := r.URL.Query().Get("table")
	includeSchema := r.URL.Query().Get("schema") != "false"
	includeData := r.URL.Query().Get("data") != "false"

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	if targetDB != "" {
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	schema, err := sess.Session.InspectSchema(ctx, targetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
		return
	}

	var tablesToDump []driver.TableMetadata
	if targetTable != "" {
		for _, t := range schema.Tables {
			if strings.EqualFold(t.Name, targetTable) {
				tablesToDump = append(tablesToDump, t)
				break
			}
		}
		if len(tablesToDump) == 0 {
			writeError(w, http.StatusNotFound, "table not found")
			return
		}
	} else {
		// FK parents before children and views last, so the dump restores without a FK-check toggle (PostgreSQL).
		tablesToDump = viewsLast(driver.TopologicalSortTables(schema.Tables))
	}

	dbName := targetDB
	if dbName == "" {
		dbName = schema.DatabaseName
	}
	if dbName == "" {
		dbName = "database"
	}

	fileName := fmt.Sprintf("%s_dump.sql", dbName)
	if targetTable != "" {
		fileName = fmt.Sprintf("%s_%s_dump.sql", dbName, targetTable)
	}

	w.Header().Set("Content-Type", "text/sql; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))

	flusher, _ := w.(http.Flusher)
	out := &dumpWriter{w: w, flusher: flusher}
	defer out.abortOnError()

	quoteIdent := func(name string) string { return quoteIdent(name, sess.DriverName) }

	// Dump header
	out.printf("-- SqlFox SQL Dump\n-- Database: %s\n-- Dump Date: %s\n\n", dbName, time.Now().Format(time.RFC3339))
	writeDumpPrologue(out, sess.DriverName)

	if includeSchema {
		out.fail(writeDumpDrops(ctx, out, sess, tablesToDump))
	}

	for _, tbl := range tablesToDump {
		if out.err != nil {
			return
		}
		// Schema DDL
		if includeSchema {
			ddl, err := sess.Session.GetTableDDL(ctx, tbl.Name)
			if err != nil {
				out.fail(fmt.Errorf("get DDL for %s: %w", tbl.Name, err))
				return
			}
			if ddl != "" {
				out.printf("%s;\n\n", strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
				out.flush()
			}
		}

		// Table Rows Data
		if includeData && !isView(tbl) {
			out.fail(writeDumpRows(ctx, out, sess, tbl.Name, fmt.Sprintf("SELECT * FROM %s", quoteIdent(tbl.Name)), 0))
			for _, stmt := range driver.SequenceResetStatements(tbl, sess.DriverName) {
				out.printf("%s\n\n", stmt)
			}
		}
	}

	writeDumpEpilogue(out, sess.DriverName)
}

// dumpWriter streams a SQL dump and remembers the first error. The HTTP status is already 200 once
// streaming starts, so on failure it appends an error marker and aborts the connection: the browser
// then reports a failed (not a silently truncated) download.
type dumpWriter struct {
	w       io.Writer
	flusher http.Flusher
	err     error
}

func (d *dumpWriter) printf(format string, args ...any) {
	if d.err == nil {
		_, d.err = fmt.Fprintf(d.w, format, args...)
	}
}

func (d *dumpWriter) fail(err error) {
	if d.err == nil && err != nil {
		d.err = err
	}
}

func (d *dumpWriter) flush() {
	if d.flusher != nil && d.err == nil {
		d.flusher.Flush()
	}
}

func (d *dumpWriter) abortOnError() {
	if d.err == nil {
		d.flush()
		return
	}
	_, _ = fmt.Fprintf(d.w, "\n-- SQLFOX DUMP INCOMPLETE: %s\n", strings.ReplaceAll(d.err.Error(), "\n", " "))
	if d.flusher != nil {
		d.flusher.Flush()
	}
	panic(http.ErrAbortHandler)
}

func writeDumpPrologue(out *dumpWriter, driverName string) {
	if driverName == "mysql" {
		out.printf("SET FOREIGN_KEY_CHECKS = 0;\nSET SQL_MODE = 'NO_AUTO_VALUE_ON_ZERO';\n\n")
	} else if driverName == "sqlite" {
		out.printf("PRAGMA foreign_keys = OFF;\n\n")
	}
	out.flush()
}

func writeDumpEpilogue(out *dumpWriter, driverName string) {
	if driverName == "mysql" {
		out.printf("SET FOREIGN_KEY_CHECKS = 1;\n")
	} else if driverName == "sqlite" {
		out.printf("PRAGMA foreign_keys = ON;\n")
	}
}

// writeDumpDrops emits DROP statements in reverse dependency order (children and views first), with
// CASCADE on PostgreSQL, so re-importing over an existing database does not fail on dependencies.
func writeDumpDrops(ctx context.Context, out *dumpWriter, sess *ActiveSession, tables []driver.TableMetadata) error {
	isPg := sess.DriverName == "postgres" || sess.DriverName == "postgresql" || sess.DriverName == "pg"
	for i := len(tables) - 1; i >= 0; i-- {
		tbl := tables[i]
		kind := "TABLE"
		if isView(tbl) {
			ddl, err := sess.Session.GetTableDDL(ctx, tbl.Name)
			if err != nil {
				return fmt.Errorf("get DDL for %s: %w", tbl.Name, err)
			}
			kind = dropKind(tbl, ddl)
		}
		cascade := ""
		if isPg {
			cascade = " CASCADE"
		}
		out.printf("DROP %s IF EXISTS %s%s;\n", kind, quoteIdent(tbl.Name, sess.DriverName), cascade)
	}
	out.printf("\n")
	return nil
}

// writeDumpRows streams the rows of querySQL as batched INSERTs; any read error is returned rather
// than treated as end of data. maxRows <= 0 means unbounded.
func writeDumpRows(ctx context.Context, out *dumpWriter, sess *ActiveSession, table, querySQL string, maxRows int) error {
	stream, err := sess.Session.QueryStream(ctx, querySQL, maxRows)
	if err != nil {
		return fmt.Errorf("read rows of %s: %w", table, err)
	}
	defer stream.Close()

	cols := stream.Columns()
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = quoteIdent(c.Name, sess.DriverName)
	}
	colHeader := strings.Join(colNames, ", ")

	written := 0
	for out.err == nil {
		batch := 50
		if maxRows > 0 && maxRows-written < batch {
			batch = maxRows - written
		}
		rowsChunk, err := stream.NextChunk(batch)
		if len(rowsChunk) > 0 {
			valueClauses := make([]string, 0, len(rowsChunk))
			for _, row := range rowsChunk {
				rowVals := make([]string, len(row))
				for i, val := range row {
					rowVals[i] = formatSQLValue(val, sess.DriverName)
				}
				valueClauses = append(valueClauses, fmt.Sprintf("  (%s)", strings.Join(rowVals, ", ")))
			}
			out.printf("INSERT INTO %s (%s) VALUES\n%s;\n\n", quoteIdent(table, sess.DriverName), colHeader, strings.Join(valueClauses, ",\n"))
			out.flush()
			written += len(rowsChunk)
		}
		if err == io.EOF || (err == nil && len(rowsChunk) == 0) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read rows of %s: %w", table, err)
		}
		if maxRows > 0 && written >= maxRows {
			return nil
		}
	}
	return out.err
}

func (s *Server) handleExportSmartSeed(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	targetDB := r.URL.Query().Get("db")
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	includeDDL := r.URL.Query().Get("ddl") != "false"

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	if targetDB != "" {
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	schema, err := sess.Session.InspectSchema(ctx, targetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
		return
	}

	sortedTables := viewsLast(driver.TopologicalSortTables(schema.Tables))

	dbName := targetDB
	if dbName == "" {
		dbName = schema.DatabaseName
	}
	if dbName == "" {
		dbName = "database"
	}

	w.Header().Set("Content-Type", "text/sql; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fmt.Sprintf("%s_smart_seed.sql", dbName)))

	flusher, _ := w.(http.Flusher)
	out := &dumpWriter{w: w, flusher: flusher}
	defer out.abortOnError()

	quoteIdent := func(name string) string { return quoteIdent(name, sess.DriverName) }

	// Smart seed header
	out.printf("-- SqlFox FK-Aware Smart Seed Dump (Transitive Closure)\n-- Database: %s\n-- Sample Limit: %d rows/table\n-- Exported At: %s\n\n",
		dbName, limit, time.Now().Format(time.RFC3339))
	writeDumpPrologue(out, sess.DriverName)

	if includeDDL {
		out.fail(writeDumpDrops(ctx, out, sess, sortedTables))
	}

	// Output tables in topological order (parents before children)
	for _, tbl := range sortedTables {
		if out.err != nil {
			return
		}
		if includeDDL {
			ddl, err := sess.Session.GetTableDDL(ctx, tbl.Name)
			if err != nil {
				out.fail(fmt.Errorf("get DDL for %s: %w", tbl.Name, err))
				return
			}
			if ddl != "" {
				out.printf("%s;\n\n", strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
				out.flush()
			}
		}

		if isView(tbl) {
			continue
		}

		// Query bounded sample rows
		querySQL := fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoteIdent(tbl.Name), limit)
		out.fail(writeDumpRows(ctx, out, sess, tbl.Name, querySQL, limit))
		for _, stmt := range driver.SequenceResetStatements(tbl, sess.DriverName) {
			out.printf("%s\n\n", stmt)
		}
	}

	writeDumpEpilogue(out, sess.DriverName)
}

// splitSQLStatements splits a multi-statement SQL script into individual statements (see driver.SplitSQLStatements).
func splitSQLStatements(reader io.Reader, driverName ...string) []string {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil
	}
	drv := ""
	if len(driverName) > 0 {
		drv = driverName[0]
	}
	return driver.SplitSQLStatements(string(content), drv)
}

func (s *Server) handleRunSQLScript(w http.ResponseWriter, r *http.Request) {
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

	stopOnError := r.URL.Query().Get("stop_on_error") != "false"
	targetDB := r.URL.Query().Get("db")

	var scriptReader io.Reader

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(50 * 1024 * 1024); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
			return
		}
		if r.MultipartForm != nil {
			defer func() {
				_ = r.MultipartForm.RemoveAll()
			}()
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing 'file' form field")
			return
		}
		defer file.Close()
		scriptReader = file
	} else {
		scriptReader = r.Body
	}

	statements := splitSQLStatements(scriptReader, sess.DriverName)
	if len(statements) == 0 {
		writeError(w, http.StatusBadRequest, "no executable SQL statements found in script")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	if targetDB != "" {
		if err := sess.Session.SelectDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	totalStatements := len(statements)
	executedStatements := 0
	errorCount := 0

	for i, stmt := range statements {
		start := time.Now()
		res, err := sess.Session.Exec(ctx, stmt)
		durationMs := time.Since(start).Milliseconds()

		var rowsAffected int64
		if res != nil {
			rowsAffected = res.RowsAffected
		}
		sess.RecordAudit(stmt, "script_runner", durationMs, rowsAffected, err, start)

		preview := stmt
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}

		progressEvent := map[string]any{
			"statement_index": i + 1,
			"total":           totalStatements,
			"sql_preview":     preview,
			"duration_ms":     durationMs,
			"rows_affected":   rowsAffected,
			"status":          "ok",
		}

		if err != nil {
			errorCount++
			progressEvent["status"] = "error"
			progressEvent["error"] = err.Error()
		} else {
			executedStatements++
		}

		_ = json.NewEncoder(w).Encode(progressEvent)
		if flusher != nil {
			flusher.Flush()
		}

		if err != nil && stopOnError {
			break
		}
	}

	finalStatus := "completed"
	if errorCount > 0 && stopOnError && executedStatements < totalStatements {
		finalStatus = "halted"
	}

	summaryEvent := map[string]any{
		"status":              finalStatus,
		"total_statements":    totalStatements,
		"executed_statements": executedStatements,
		"error_count":         errorCount,
	}
	_ = json.NewEncoder(w).Encode(summaryEvent)
	if flusher != nil {
		flusher.Flush()
	}
}

func isView(t driver.TableMetadata) bool { return strings.HasSuffix(strings.ToUpper(t.Type), "VIEW") }

// dropKind returns the DROP object keyword for a dumped relation (DROP TABLE fails on views).
func dropKind(t driver.TableMetadata, ddl string) string {
	if !isView(t) {
		return "TABLE"
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(ddl)), "CREATE MATERIALIZED VIEW") {
		return "MATERIALIZED VIEW"
	}
	return "VIEW"
}

// viewsLast keeps table order but moves views after base tables so CREATE VIEW finds its sources.
// ponytail: views referencing other views keep their relative order; upgrade path: view dependency sort.
func viewsLast(tables []driver.TableMetadata) []driver.TableMetadata {
	out := slices.Clone(tables)
	slices.SortStableFunc(out, func(a, b driver.TableMetadata) int {
		switch av, bv := isView(a), isView(b); {
		case av == bv:
			return 0
		case av:
			return 1
		default:
			return -1
		}
	})
	return out
}
