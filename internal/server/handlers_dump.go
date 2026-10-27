package server

import (
	"bufio"
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
		tablesToDump = viewsLast(schema.Tables)
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

	quoteIdent := func(name string) string { return quoteIdent(name, sess.DriverName) }

	// Dump header
	_, _ = fmt.Fprintf(w, "-- SqlFox SQL Dump\n-- Database: %s\n-- Dump Date: %s\n\n", dbName, time.Now().Format(time.RFC3339))
	if sess.DriverName == "mysql" {
		_, _ = fmt.Fprint(w, "SET FOREIGN_KEY_CHECKS = 0;\nSET SQL_MODE = 'NO_AUTO_VALUE_ON_ZERO';\n\n")
	} else if sess.DriverName == "sqlite" {
		_, _ = fmt.Fprint(w, "PRAGMA foreign_keys = OFF;\n\n")
	}
	if flusher != nil {
		flusher.Flush()
	}

	for _, tbl := range tablesToDump {
		// Schema DDL
		if includeSchema {
			ddl, err := sess.Session.GetTableDDL(ctx, tbl.Name)
			if err == nil && ddl != "" {
				_, _ = fmt.Fprintf(w, "DROP %s IF EXISTS %s;\n%s;\n\n", dropKind(tbl, ddl), quoteIdent(tbl.Name), strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
				if flusher != nil {
					flusher.Flush()
				}
			}
		}

		// Table Rows Data
		if includeData && tbl.Type != "VIEW" {
			querySQL := fmt.Sprintf("SELECT * FROM %s", quoteIdent(tbl.Name))
			stream, err := sess.Session.QueryStream(ctx, querySQL, 0)
			if err == nil {
				cols := stream.Columns()
				var colNames []string
				for _, c := range cols {
					colNames = append(colNames, quoteIdent(c.Name))
				}
				colHeader := strings.Join(colNames, ", ")

				batchSize := 50
				for {
					rowsChunk, err := stream.NextChunk(batchSize)
					if len(rowsChunk) > 0 {
						var valueClauses []string
						for _, row := range rowsChunk {
							var rowVals []string
							for _, val := range row {
								rowVals = append(rowVals, formatSQLValue(val, sess.DriverName))
							}
							valueClauses = append(valueClauses, fmt.Sprintf("  (%s)", strings.Join(rowVals, ", ")))
						}
						_, _ = fmt.Fprintf(w, "INSERT INTO %s (%s) VALUES\n%s;\n\n",
							quoteIdent(tbl.Name), colHeader, strings.Join(valueClauses, ",\n"))
						if flusher != nil {
							flusher.Flush()
						}
					}
					if err != nil || len(rowsChunk) == 0 {
						break
					}
				}
				_ = stream.Close()
			}
		}
	}

	// Dump footer
	if sess.DriverName == "mysql" {
		_, _ = fmt.Fprint(w, "SET FOREIGN_KEY_CHECKS = 1;\n")
	} else if sess.DriverName == "sqlite" {
		_, _ = fmt.Fprint(w, "PRAGMA foreign_keys = ON;\n")
	}
	if flusher != nil {
		flusher.Flush()
	}
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

	quoteIdent := func(name string) string { return quoteIdent(name, sess.DriverName) }

	// Smart seed header
	_, _ = fmt.Fprintf(w, "-- SqlFox FK-Aware Smart Seed Dump (Transitive Closure)\n-- Database: %s\n-- Sample Limit: %d rows/table\n-- Exported At: %s\n\n",
		dbName, limit, time.Now().Format(time.RFC3339))

	if sess.DriverName == "mysql" {
		_, _ = fmt.Fprint(w, "SET FOREIGN_KEY_CHECKS = 0;\nSET SQL_MODE = 'NO_AUTO_VALUE_ON_ZERO';\n\n")
	} else if sess.DriverName == "sqlite" {
		_, _ = fmt.Fprint(w, "PRAGMA foreign_keys = OFF;\n\n")
	}
	if flusher != nil {
		flusher.Flush()
	}

	// Output tables in topological order (parents before children)
	for _, tbl := range sortedTables {
		if includeDDL {
			ddl, err := sess.Session.GetTableDDL(ctx, tbl.Name)
			if err == nil && ddl != "" {
				_, _ = fmt.Fprintf(w, "DROP %s IF EXISTS %s;\n%s;\n\n", dropKind(tbl, ddl), quoteIdent(tbl.Name), strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
				if flusher != nil {
					flusher.Flush()
				}
			}
		}

		if tbl.Type == "VIEW" {
			continue
		}

		// Query bounded sample rows
		querySQL := fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoteIdent(tbl.Name), limit)
		stream, err := sess.Session.QueryStream(ctx, querySQL, limit)
		if err == nil {
			cols := stream.Columns()
			var colNames []string
			for _, c := range cols {
				colNames = append(colNames, quoteIdent(c.Name))
			}
			colHeader := strings.Join(colNames, ", ")

			rowsChunk, _ := stream.NextChunk(limit)
			if len(rowsChunk) > 0 {
				var valueClauses []string
				for _, row := range rowsChunk {
					var rowVals []string
					for _, val := range row {
						rowVals = append(rowVals, formatSQLValue(val, sess.DriverName))
					}
					valueClauses = append(valueClauses, fmt.Sprintf("  (%s)", strings.Join(rowVals, ", ")))
				}
				_, _ = fmt.Fprintf(w, "INSERT INTO %s (%s) VALUES\n%s;\n\n",
					quoteIdent(tbl.Name), colHeader, strings.Join(valueClauses, ",\n"))
				if flusher != nil {
					flusher.Flush()
				}
			}
			_ = stream.Close()
		}
	}

	if sess.DriverName == "mysql" {
		_, _ = fmt.Fprint(w, "SET FOREIGN_KEY_CHECKS = 1;\n")
	} else if sess.DriverName == "sqlite" {
		_, _ = fmt.Fprint(w, "PRAGMA foreign_keys = ON;\n")
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// splitSQLStatements splits a multi-statement SQL script into individual statements,
// respecting single/double quotes, backticks, and single-line/multi-line comments.
func splitSQLStatements(reader io.Reader, driverName ...string) []string {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil
	}
	isMySQL := len(driverName) > 0 && (strings.EqualFold(driverName[0], "mysql") || strings.EqualFold(driverName[0], "mariadb"))
	return driver.SplitWithDelimiters(string(content), isMySQL, func(seg string) []string {
		return splitDefaultDelimiter(strings.NewReader(seg), driverName...)
	})
}

func splitDefaultDelimiter(reader io.Reader, driverName ...string) []string {
	isMySQL := len(driverName) > 0 && (strings.EqualFold(driverName[0], "mysql") || strings.EqualFold(driverName[0], "mariadb"))
	var statements []string
	scanner := bufio.NewScanner(reader)
	// Buffer up to 10MB per line/token if needed
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var currentStmt strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inDollarQuote := false
	inBlockComment := false

	isQuoteEscaped := func(runes []rune, i int) bool {
		if !isMySQL {
			return false
		}
		bsCount := 0
		for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
			bsCount++
		}
		return bsCount%2 == 1
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// Check for line comments when not inside quotes or block comment
		if !inSingleQuote && !inDoubleQuote && !inBacktick && !inDollarQuote && !inBlockComment {
			if strings.HasPrefix(trimmedLine, "--") || (isMySQL && strings.HasPrefix(trimmedLine, "#")) {
				continue
			}
		}

		runes := []rune(line)
		for i := 0; i < len(runes); i++ {
			r := runes[i]

			// Handle block and inline comments
			if !inSingleQuote && !inDoubleQuote && !inBacktick && !inDollarQuote {
				if inBlockComment {
					currentStmt.WriteRune(r)
					if r == '*' && i+1 < len(runes) && runes[i+1] == '/' {
						inBlockComment = false
						currentStmt.WriteRune('/')
						i++
					}
					continue
				}
				if r == '/' && i+1 < len(runes) && runes[i+1] == '*' {
					inBlockComment = true
					currentStmt.WriteRune('/')
					currentStmt.WriteRune('*')
					i++
					continue
				}
				if (r == '-' && i+1 < len(runes) && runes[i+1] == '-') || (isMySQL && r == '#') {
					// Inline comment extends to end of line
					currentStmt.WriteString(string(runes[i:]))
					break
				}
			}

			// Handle quotes
			if !inBlockComment {
				if r == '$' && i+1 < len(runes) && runes[i+1] == '$' && !inSingleQuote && !inDoubleQuote && !inBacktick {
					inDollarQuote = !inDollarQuote
					currentStmt.WriteString("$$")
					i++
					continue
				} else if r == '\'' && !inDoubleQuote && !inBacktick && !inDollarQuote {
					if inSingleQuote && i+1 < len(runes) && runes[i+1] == '\'' {
						// Doubled single quote escape '' inside string
						currentStmt.WriteRune('\'')
						currentStmt.WriteRune('\'')
						i++
						continue
					}
					// Check for escape \'
					if isQuoteEscaped(runes, i) {
						// escaped quote
					} else {
						inSingleQuote = !inSingleQuote
					}
				} else if r == '"' && !inSingleQuote && !inBacktick && !inDollarQuote {
					if isQuoteEscaped(runes, i) {
						// escaped quote
					} else {
						inDoubleQuote = !inDoubleQuote
					}
				} else if r == '`' && !inSingleQuote && !inDoubleQuote && !inDollarQuote {
					inBacktick = !inBacktick
				}
			}

			// Statement terminator
			if r == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick && !inDollarQuote && !inBlockComment {
				stmtStr := strings.TrimSpace(currentStmt.String())
				if stmtStr != "" && driver.HasExecutableSQL(stmtStr, isMySQL) {
					statements = append(statements, stmtStr)
				}
				currentStmt.Reset()
				continue
			}

			currentStmt.WriteRune(r)
		}
		currentStmt.WriteString("\n")
	}

	remaining := strings.TrimSpace(currentStmt.String())
	if remaining != "" && driver.HasExecutableSQL(remaining, isMySQL) {
		statements = append(statements, remaining)
	}

	return statements
}

func (s *Server) handleRunSQLScript(w http.ResponseWriter, r *http.Request) {
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
