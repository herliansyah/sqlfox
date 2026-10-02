package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

// formatSQLValue formats a Go value into a valid SQL literal for dumps and inserts.
func formatSQLValue(val any) string {
	return driver.FormatSQLValue(val)
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
		_ = sess.Session.SelectDatabase(ctx, targetDB)
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
		tablesToDump = schema.Tables
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
				_, _ = fmt.Fprintf(w, "DROP TABLE IF EXISTS %s;\n%s;\n\n", quoteIdent(tbl.Name), strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
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
								rowVals = append(rowVals, formatSQLValue(val))
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
		_ = sess.Session.SelectDatabase(ctx, targetDB)
	}

	schema, err := sess.Session.InspectSchema(ctx, targetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect schema: "+err.Error())
		return
	}

	sortedTables := driver.TopologicalSortTables(schema.Tables)

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
				_, _ = fmt.Fprintf(w, "DROP TABLE IF EXISTS %s;\n%s;\n\n", quoteIdent(tbl.Name), strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
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
						rowVals = append(rowVals, formatSQLValue(val))
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
func splitSQLStatements(reader io.Reader) []string {
	var statements []string
	scanner := bufio.NewScanner(reader)
	// Buffer up to 10MB per line/token if needed
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var currentStmt strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inBlockComment := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// Check for line comments when not inside quotes or block comment
		if !inSingleQuote && !inDoubleQuote && !inBacktick && !inBlockComment {
			if strings.HasPrefix(trimmedLine, "--") || strings.HasPrefix(trimmedLine, "#") {
				continue
			}
		}

		runes := []rune(line)
		for i := 0; i < len(runes); i++ {
			r := runes[i]

			// Handle block comments
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				if inBlockComment {
					if r == '*' && i+1 < len(runes) && runes[i+1] == '/' {
						inBlockComment = false
						i++
						continue
					}
					continue
				}
				if r == '/' && i+1 < len(runes) && runes[i+1] == '*' {
					inBlockComment = true
					i++
					continue
				}
			}

			// Handle quotes
			if !inBlockComment {
				if r == '\'' && !inDoubleQuote && !inBacktick {
					// Check for escape \'
					if i > 0 && runes[i-1] == '\\' {
						// escaped quote
					} else {
						inSingleQuote = !inSingleQuote
					}
				} else if r == '"' && !inSingleQuote && !inBacktick {
					if i > 0 && runes[i-1] == '\\' {
						// escaped quote
					} else {
						inDoubleQuote = !inDoubleQuote
					}
				} else if r == '`' && !inSingleQuote && !inDoubleQuote {
					inBacktick = !inBacktick
				}
			}

			// Statement terminator
			if r == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick && !inBlockComment {
				stmtStr := strings.TrimSpace(currentStmt.String())
				if stmtStr != "" {
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
	if remaining != "" {
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

	statements := splitSQLStatements(scriptReader)
	if len(statements) == 0 {
		writeError(w, http.StatusBadRequest, "no executable SQL statements found in script")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	if targetDB != "" {
		_ = sess.Session.SelectDatabase(ctx, targetDB)
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
