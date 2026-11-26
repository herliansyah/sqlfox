package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
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
	includeSchema := r.URL.Query().Get("schema") != "false"
	includeData := r.URL.Query().Get("data") != "false"

	// Collect requested tables from repeated ?table=a&table=b and/or comma-separated ?tables=a,b.
	var requestedTables []string
	seenReq := make(map[string]bool)
	addReq := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if !seenReq[key] {
			seenReq[key] = true
			requestedTables = append(requestedTables, name)
		}
	}
	for _, t := range r.URL.Query()["table"] {
		addReq(t)
	}
	if rawList := r.URL.Query().Get("tables"); rawList != "" {
		for _, t := range strings.Split(rawList, ",") {
			addReq(t)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	sess.beginInFlight()
	defer sess.endInFlight()

	if targetDB != "" {
		if err := sess.UseDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx = driver.WithDatabase(ctx, targetDB)
	}

	schema, err := sess.Session.InspectSchema(ctx, targetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
		return
	}

	var tablesToDump []driver.TableMetadata
	if len(requestedTables) > 0 {
		byLowerName := make(map[string]driver.TableMetadata, len(schema.Tables))
		for _, t := range schema.Tables {
			byLowerName[strings.ToLower(t.Name)] = t
		}
		selected := make([]driver.TableMetadata, 0, len(requestedTables))
		for _, req := range requestedTables {
			t, ok := byLowerName[strings.ToLower(req)]
			if !ok {
				writeError(w, http.StatusNotFound, fmt.Sprintf("table not found: %s", req))
				return
			}
			selected = append(selected, t)
		}
		// Keep FK parents before children and views last within the selected subset.
		tablesToDump = viewsLast(driver.TopologicalSortTables(selected))
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
	if len(requestedTables) == 1 {
		fileName = fmt.Sprintf("%s_%s_dump.sql", dbName, tablesToDump[0].Name)
	} else if len(requestedTables) > 1 {
		fileName = fmt.Sprintf("%s_%d_tables_dump.sql", dbName, len(tablesToDump))
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

	// Pin a single read-only snapshot transaction for table data so concurrent writes
	// between parent and child tables never produce torn / orphan-FK dumps.
	var snapTx driver.TxSession
	if includeData {
		if tx, txErr := sess.Session.BeginTx(ctx); txErr == nil {
			if isPgDriver(strings.ToLower(sess.DriverName)) {
				_, _ = tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
			}
			snapTx = tx
			defer func() { _ = snapTx.Rollback() }()
		}
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
			out.fail(writeDumpRowsFrom(ctx, out, sess, snapTx, tbl.Name, fmt.Sprintf("SELECT * FROM %s", quoteIdent(tbl.Name)), 0))
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
	d := strings.ToLower(driverName)
	if d == "mysql" || d == "mariadb" {
		// Save and normalize SQL_MODE (clearing NO_BACKSLASH_ESCAPES) so escaped literals restore faithfully.
		out.printf("SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO';\nSET FOREIGN_KEY_CHECKS = 0;\n\n")
	} else if d == "sqlite" || d == "sqlite3" {
		out.printf("PRAGMA foreign_keys = OFF;\n\n")
	}
	out.flush()
}

func writeDumpEpilogue(out *dumpWriter, driverName string) {
	d := strings.ToLower(driverName)
	if d == "mysql" || d == "mariadb" {
		out.printf("SET FOREIGN_KEY_CHECKS = 1;\nSET SQL_MODE=@OLD_SQL_MODE;\n")
	} else if d == "sqlite" || d == "sqlite3" {
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

func writeDumpRowsFrom(ctx context.Context, out *dumpWriter, sess *ActiveSession, tx driver.TxSession, table, querySQL string, maxRows int) error {
	var stream driver.RowStream
	var err error
	if tx != nil {
		stream, err = tx.QueryStream(ctx, querySQL, maxRows)
	} else {
		stream, err = sess.Session.QueryStream(ctx, querySQL, maxRows)
	}
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
	if limit > 10000 {
		limit = 10000
	}
	includeDDL := r.URL.Query().Get("ddl") != "false"

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	sess.beginInFlight()
	defer sess.endInFlight()

	if targetDB != "" {
		if err := sess.UseDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx = driver.WithDatabase(ctx, targetDB)
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

	// Output tables in topological order (parents before children), tracking sampled parent
	// key values so child tables filter via FK IN (...) to maintain referential integrity.
	sampledKeys := make(map[string]map[string][]any) // lower(table) -> lower(col) -> values
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

		// Build FK-filtered query referencing already-sampled parent rows when available.
		var fkConds []string
		for _, fk := range tbl.ForeignKeys {
			refTbl := strings.ToLower(fk.ReferencedTable)
			refCol := strings.ToLower(fk.ReferencedColumn)
			if refTbl == strings.ToLower(tbl.Name) {
				continue // skip self-referential FK filter
			}
			parentVals := sampledKeys[refTbl][refCol]
			if len(parentVals) == 0 {
				continue
			}
			lits := make([]string, 0, len(parentVals))
			for _, pv := range parentVals {
				if pv != nil {
					lits = append(lits, formatSQLValue(pv, sess.DriverName))
				}
			}
			if len(lits) > 0 {
				qFKCol := quoteIdent(fk.Column)
				fkConds = append(fkConds, fmt.Sprintf("(%s IS NULL OR %s IN (%s))", qFKCol, qFKCol, strings.Join(lits, ", ")))
			}
		}

		querySQL := fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoteIdent(tbl.Name), limit)
		if len(fkConds) > 0 {
			querySQL = fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT %d", quoteIdent(tbl.Name), strings.Join(fkConds, " AND "), limit)
		}
		sampled, dumpErr := writeSmartSeedRows(ctx, out, sess, tbl.Name, querySQL, limit)
		out.fail(dumpErr)
		if len(sampled) > 0 {
			sampledKeys[strings.ToLower(tbl.Name)] = sampled
		}
		for _, stmt := range driver.SequenceResetStatements(tbl, sess.DriverName) {
			out.printf("%s\n\n", stmt)
		}
	}

	writeDumpEpilogue(out, sess.DriverName)
}

func writeSmartSeedRows(ctx context.Context, out *dumpWriter, sess *ActiveSession, table, querySQL string, maxRows int) (map[string][]any, error) {
	stream, err := sess.Session.QueryStream(ctx, querySQL, maxRows)
	if err != nil {
		return nil, fmt.Errorf("read rows of %s: %w", table, err)
	}
	defer stream.Close()

	cols := stream.Columns()
	colNames := make([]string, len(cols))
	collected := make(map[string][]any, len(cols))
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
					if i < len(cols) && len(collected[strings.ToLower(cols[i].Name)]) < maxRows {
						collected[strings.ToLower(cols[i].Name)] = append(collected[strings.ToLower(cols[i].Name)], val)
					}
				}
				valueClauses = append(valueClauses, fmt.Sprintf("  (%s)", strings.Join(rowVals, ", ")))
			}
			out.printf("INSERT INTO %s (%s) VALUES\n%s;\n\n", quoteIdent(table, sess.DriverName), colHeader, strings.Join(valueClauses, ",\n"))
			out.flush()
			written += len(rowsChunk)
		}
		if err == io.EOF || (err == nil && len(rowsChunk) == 0) {
			return collected, nil
		}
		if err != nil {
			return collected, fmt.Errorf("read rows of %s: %w", table, err)
		}
		if maxRows > 0 && written >= maxRows {
			return collected, nil
		}
	}
	return collected, out.err
}

// maxScriptPayloadBytes caps in-memory SQL script ingestion to 100 MB to prevent OOM spikes.
const maxScriptPayloadBytes = 100 * 1024 * 1024

// splitSQLStatements splits a multi-statement SQL script into individual statements (see driver.SplitSQLStatements).
func splitSQLStatements(reader io.Reader, driverName ...string) []string {
	content, err := io.ReadAll(io.LimitReader(reader, maxScriptPayloadBytes))
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
	// confirm=true is the power-user acknowledgment that the Safety Guard surfaced a destructive
	// statement and the operator chose to proceed, mirroring ExecuteRequest.Confirmed in the editor.
	confirmed := r.URL.Query().Get("confirm") == "true"

	var scriptReader io.Reader

	if r.ContentLength > maxScriptPayloadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "SQL script exceeds maximum size limit (100MB)")
		return
	}
	// Cap body even for Transfer-Encoding: chunked (where ContentLength == -1) so multipart
	// parsing cannot spool unbounded data onto temp disk.
	r.Body = http.MaxBytesReader(w, r.Body, maxScriptPayloadBytes+1024*1024)

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(4 * 1024 * 1024); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "SQL script exceeds maximum size limit (100MB)")
				return
			}
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

	// Spool uploaded script to a bounded temp file so both the Safety Guard pre-pass and
	// statement execution stream statement-by-statement with O(largest statement) RAM instead of
	// allocating ~750MB-1GB for a 100MB script via io.ReadAll + []rune conversion.
	tmpFile, err := os.CreateTemp("", "sqlfox-script-*.sql")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp script file: "+err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	written, err := io.Copy(tmpFile, io.LimitReader(scriptReader, maxScriptPayloadBytes+1))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "SQL script exceeds maximum size limit (100MB)")
			return
		}
		writeError(w, http.StatusBadRequest, "failed to read SQL script: "+err.Error())
		return
	}
	if written > maxScriptPayloadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "SQL script exceeds maximum size limit (100MB)")
		return
	}

	// Pass 1: Safety Guard pre-pass via streaming statement scanner (zero whole-file buffering).
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rewind temp script file: "+err.Error())
		return
	}
	isRemote := s.IsRemoteRequest(r)
	totalStatements := 0
	var blockers []ScriptBlocker
	scanErr := driver.ScanSQLStatements(tmpFile, sess.DriverName, func(stmt string) error {
		totalStatements++
		if isRemote && hasRemoteBlockedHostFileOp(stmt, sess.Environment, sess.DriverName, targetDB) {
			return ErrRemoteHostFileSQLBlocked
		}
		if b := sess.InspectScriptStatement(stmt, totalStatements, targetDB); b != nil {
			blockers = append(blockers, *b)
		}
		return nil
	})
	if scanErr != nil {
		if errors.Is(scanErr, ErrRemoteHostFileSQLBlocked) {
			writeError(w, http.StatusForbidden, scanErr.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "failed to scan SQL script: "+scanErr.Error())
		return
	}
	if totalStatements == 0 {
		writeError(w, http.StatusBadRequest, "no executable SQL statements found in script")
		return
	}

	// Safety Guard gate: mirrors editor's destructive-statement challenge.
	if len(blockers) > 0 && !confirmed {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":              "Script contains destructive statements that require confirmation. Re-run with confirm=true to proceed.",
			"requires_challenge": true,
			"blockers":           blockers,
		})
		return
	}
	if len(blockers) > 0 && confirmed && strings.EqualFold(sess.Environment, "Production") {
		expectedDB := targetDB
		if expectedDB == "" {
			expectedDB = sess.GetDatabaseName()
		}
		typed := strings.TrimSpace(r.URL.Query().Get("challenge_input"))
		if typed == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":              fmt.Sprintf("Production script requires non-empty challenge_input matching '%s' or target table.", expectedDB),
				"requires_challenge": true,
				"blockers":           blockers,
			})
			return
		}
		matched := strings.EqualFold(typed, expectedDB) || strings.EqualFold(typed, sess.Name)
		if !matched {
			for _, b := range blockers {
				if b.TargetTable != "" && strings.EqualFold(typed, b.TargetTable) {
					matched = true
					break
				}
			}
		}
		if !matched {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":              fmt.Sprintf("Production script challenge_input did not match '%s'.", expectedDB),
				"requires_challenge": true,
				"blockers":           blockers,
			})
			return
		}
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	sess.beginInFlight()
	defer sess.endInFlight()

	if targetDB != "" {
		if err := sess.UseDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx = driver.WithDatabase(ctx, targetDB)
	}

	executedStatements := 0
	errorCount := 0

	// Pass 2: Stream statements one-by-one from disk into the database executor.
	runStatements := func(execFn func(context.Context, string) (*driver.ExecResult, error)) error {
		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			return err
		}
		stmtIdx := 0
		return driver.ScanSQLStatements(tmpFile, sess.DriverName, func(stmt string) error {
			stmtIdx++
			// Pre-DDL Smart Tiered Backup for destructive schema ops, matching the editor pipeline.
			sess.BackupBeforeDestructive(ctx, stmt, targetDB)

			start := time.Now()
			res, err := execFn(ctx, stmt)
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
				"statement_index": stmtIdx,
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
				return driver.ErrStopScan
			}
			return nil
		})
	}

	// Execute all script statements on a single pinned physical connection when supported,
	// so session-scoped commands (SET FOREIGN_KEY_CHECKS=0, PRAGMA foreign_keys=OFF, BEGIN/COMMIT)
	// stay together and never leak onto other pooled connections.
	if scriptSess, ok := sess.Session.(driver.ScriptSession); ok {
		if pinErr := scriptSess.ExecScriptConn(ctx, runStatements); pinErr != nil {
			errorCount++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "error",
				"error":  pinErr.Error(),
			})
			if flusher != nil {
				flusher.Flush()
			}
		}
	} else {
		_ = runStatements(func(c context.Context, q string) (*driver.ExecResult, error) {
			return sess.Session.Exec(c, q)
		})
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
