package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"sqlfox/internal/driver"
)

// ----------------------------------------------------------------------------
// Milestone 11: Schema & Data Synchronization Engine
// ----------------------------------------------------------------------------

type StructureDiffItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // table_missing, column_missing, column_mismatch, index_missing, table_extra
	Table     string `json:"table"`
	Column    string `json:"column,omitempty"`
	Index     string `json:"index,omitempty"`
	Action    string `json:"action"` // CREATE TABLE, ADD COLUMN, MODIFY COLUMN, CREATE INDEX, DROP TABLE
	SourceDef string `json:"source_def,omitempty"`
	TargetDef string `json:"target_def,omitempty"`
	SQL       string `json:"sql"`
	Selected  bool   `json:"selected"`
}

type SyncStructureCompareRequest struct {
	SourceSessionID string `json:"source_session_id"`
	SourceDB        string `json:"source_db"`
	TargetSessionID string `json:"target_session_id"`
	TargetDB        string `json:"target_db"`
	IncludeDrops    bool   `json:"include_drops"`
}

type SyncStructureCompareResponse struct {
	SourceDB        string              `json:"source_db"`
	TargetDB        string              `json:"target_db"`
	Diffs           []StructureDiffItem `json:"diffs"`
	MigrationScript string              `json:"migration_script"`
}

type SyncStructureApplyRequest struct {
	TargetSessionID string   `json:"target_session_id"`
	TargetDB        string   `json:"target_db"`
	Statements      []string `json:"statements"`
	Confirmed       bool     `json:"confirmed"`
	ChallengeInput  string   `json:"challenge_input"`
}

type SyncDataTransferRequest struct {
	SourceSessionID string   `json:"source_session_id"`
	SourceDB        string   `json:"source_db"`
	TargetSessionID string   `json:"target_session_id"`
	TargetDB        string   `json:"target_db"`
	Tables          []string `json:"tables"`
	CreateTables    bool     `json:"create_tables"`
	TruncateTarget  bool     `json:"truncate_target"`
	BatchSize       int      `json:"batch_size"`
}

type SyncDataCompareRequest struct {
	SourceSessionID string `json:"source_session_id"`
	SourceDB        string `json:"source_db"`
	TargetSessionID string `json:"target_session_id"`
	TargetDB        string `json:"target_db"`
	Table           string `json:"table"`
	DeleteMissing   bool   `json:"delete_missing"`
}

type SyncDataOperation struct {
	Type    string `json:"type"` // "insert", "update", "delete"
	PK      any    `json:"pk"`
	OldData []any  `json:"old_data,omitempty"`
	Data    []any  `json:"data,omitempty"`
}

type SyncDataCompareResponse struct {
	Table      string              `json:"table"`
	PKColumns  []string            `json:"pk_columns"`
	Columns    []string            `json:"columns"`
	Summary    map[string]int      `json:"summary"`
	Operations []SyncDataOperation `json:"operations"`
}

type SyncDataApplyRequest struct {
	TargetSessionID string              `json:"target_session_id"`
	TargetDB        string              `json:"target_db"`
	Table           string              `json:"table"`
	Columns         []string            `json:"columns,omitempty"` // column order of op.Data (from compare response)
	Operations      []SyncDataOperation `json:"operations"`
	Confirmed       bool                `json:"confirmed"`
	ChallengeInput  string              `json:"challenge_input"`
}

// ----------------------------------------------------------------------------
// 1. Structure Sync Handlers
// ----------------------------------------------------------------------------

func (s *Server) handleSyncStructureCompare(w http.ResponseWriter, r *http.Request) {
	var req SyncStructureCompareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if req.SourceSessionID == "" || req.TargetSessionID == "" {
		writeError(w, http.StatusBadRequest, "source_session_id and target_session_id are required")
		return
	}

	srcSess, err := s.sessionMgr.GetSession(req.SourceSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source session not found: "+err.Error())
		return
	}

	tgtSess, err := s.sessionMgr.GetSession(req.TargetSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target session not found: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := srcSess.UseDatabase(ctx, req.SourceDB); err != nil {
		writeError(w, http.StatusBadRequest, "source: "+err.Error())
		return
	}
	if err := tgtSess.UseDatabase(ctx, req.TargetDB); err != nil {
		writeError(w, http.StatusBadRequest, "target: "+err.Error())
		return
	}

	srcSchema, err := srcSess.Session.InspectSchema(ctx, req.SourceDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect source schema: "+err.Error())
		return
	}

	tgtSchema, err := tgtSess.Session.InspectSchema(ctx, req.TargetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect target schema: "+err.Error())
		return
	}

	srcDriver := srcSess.DriverName
	tgtDriver := tgtSess.DriverName
	tgtTableMap := make(map[string]driver.TableMetadata)
	for _, t := range tgtSchema.Tables {
		tgtTableMap[strings.ToLower(t.Name)] = t
	}

	srcTableMap := make(map[string]driver.TableMetadata)
	for _, t := range srcSchema.Tables {
		srcTableMap[strings.ToLower(t.Name)] = t
	}

	var diffs []StructureDiffItem
	diffIndex := 1

	// Check tables in source; FK parents first so generated CREATE TABLEs apply in order.
	for _, srcTbl := range driver.TopologicalSortTables(srcSchema.Tables) {
		tgtTbl, exists := tgtTableMap[strings.ToLower(srcTbl.Name)]
		if !exists {
			// Missing table in target -> CREATE TABLE
			var ddl string
			if strings.EqualFold(srcDriver, tgtDriver) {
				ddl, _ = srcSess.Session.GetTableDDL(ctx, srcTbl.Name)
			}
			if strings.TrimSpace(ddl) == "" {
				ddl = buildCreateTableDDL(srcTbl, srcDriver, tgtDriver)
			} else {
				ddl = strings.TrimSpace(ddl)
				if !strings.HasSuffix(ddl, ";") {
					ddl += ";"
				}
			}

			diffs = append(diffs, StructureDiffItem{
				ID:        fmt.Sprintf("diff-%d", diffIndex),
				Type:      "table_missing",
				Table:     srcTbl.Name,
				Action:    "CREATE TABLE",
				SourceDef: fmt.Sprintf("Table %s (%d columns)", srcTbl.Name, len(srcTbl.Columns)),
				TargetDef: "Does not exist",
				SQL:       ddl,
				Selected:  true,
			})
			diffIndex++
			continue
		}

		// Table exists in both -> compare columns
		tgtColMap := make(map[string]driver.ColumnMetadata)
		for _, col := range tgtTbl.Columns {
			tgtColMap[strings.ToLower(col.Name)] = col
		}

		srcColMap := make(map[string]driver.ColumnMetadata)
		for _, col := range srcTbl.Columns {
			srcColMap[strings.ToLower(col.Name)] = col
		}

		for _, srcCol := range srcTbl.Columns {
			tgtCol, colExists := tgtColMap[strings.ToLower(srcCol.Name)]
			if !colExists {
				// Column missing in target -> ADD COLUMN
				clause := buildColumnDefinition(srcCol, srcDriver, tgtDriver)
				sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;",
					quoteIdent(srcTbl.Name, tgtDriver),
					quoteIdent(srcCol.Name, tgtDriver),
					clause,
				)

				diffs = append(diffs, StructureDiffItem{
					ID:        fmt.Sprintf("diff-%d", diffIndex),
					Type:      "column_missing",
					Table:     srcTbl.Name,
					Column:    srcCol.Name,
					Action:    "ADD COLUMN",
					SourceDef: fmt.Sprintf("%s %s", srcCol.Name, clause),
					TargetDef: "Does not exist",
					SQL:       sql,
					Selected:  true,
				})
				diffIndex++
			} else {
				// Column exists -> check for type/nullability discrepancy
				if !columnEquals(srcCol, tgtCol, srcDriver, tgtDriver) {
					clause := buildColumnDefinition(srcCol, srcDriver, tgtDriver)
					var sql string
					if tgtDriver == "sqlite" {
						sql = fmt.Sprintf("-- SQLite: column %s definition differs (%s vs %s)", srcCol.Name, srcCol.Type, tgtCol.Type)
					} else if tgtDriver == "postgres" || tgtDriver == "postgresql" || tgtDriver == "pg" {
						// ponytail: Atomic granular ALTER statements for PostgreSQL; upgrade path: full alter column state machine
						var pgStmts []string
						tblQ := quoteIdent(srcTbl.Name, tgtDriver)
						colQ := quoteIdent(srcCol.Name, tgtDriver)
						srcType := normalizeColumnType(srcCol.Type, tgtDriver)
						if !strings.EqualFold(srcType, normalizeColumnType(tgtCol.Type, tgtDriver)) {
							pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;", tblQ, colQ, srcType, colQ, srcType))
						}
						if srcCol.IsNullable != tgtCol.IsNullable {
							if srcCol.IsNullable {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", tblQ, colQ))
							} else {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tblQ, colQ))
							}
						}
						srcDefault := renderColumnDefault(srcCol, srcDriver, tgtDriver)
						if !strings.EqualFold(canonDefault(srcDefault), canonDefault(renderColumnDefault(tgtCol, tgtDriver, tgtDriver))) {
							if srcDefault == "" {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", tblQ, colQ))
							} else {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", tblQ, colQ, srcDefault))
							}
						}
						if len(pgStmts) == 0 {
							pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tblQ, colQ, srcType))
						}
						sql = strings.Join(pgStmts, "\n")
					} else {
						sql = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s;",
							quoteIdent(srcTbl.Name, tgtDriver),
							quoteIdent(srcCol.Name, tgtDriver),
							clause,
						)
					}

					diffs = append(diffs, StructureDiffItem{
						ID:        fmt.Sprintf("diff-%d", diffIndex),
						Type:      "column_mismatch",
						Table:     srcTbl.Name,
						Column:    srcCol.Name,
						Action:    "MODIFY COLUMN",
						SourceDef: fmt.Sprintf("%s %s", srcCol.Name, clause),
						TargetDef: fmt.Sprintf("%s %s", tgtCol.Name, buildColumnDefinition(tgtCol, tgtDriver, tgtDriver)),
						SQL:       sql,
						Selected:  true,
					})
					diffIndex++
				}
			}
		}

		// If include_drops: check extra columns in target
		if req.IncludeDrops && tgtDriver != "sqlite" {
			for _, tgtCol := range tgtTbl.Columns {
				if _, exists := srcColMap[strings.ToLower(tgtCol.Name)]; !exists {
					sql := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;",
						quoteIdent(srcTbl.Name, tgtDriver),
						quoteIdent(tgtCol.Name, tgtDriver),
					)
					diffs = append(diffs, StructureDiffItem{
						ID:        fmt.Sprintf("diff-%d", diffIndex),
						Type:      "column_extra",
						Table:     srcTbl.Name,
						Column:    tgtCol.Name,
						Action:    "DROP COLUMN",
						SourceDef: "Does not exist",
						TargetDef: fmt.Sprintf("%s %s", tgtCol.Name, buildColumnDefinition(tgtCol, tgtDriver, tgtDriver)),
						SQL:       sql,
						Selected:  false,
					})
					diffIndex++
				}
			}
		}

		// Compare indexes
		tgtIdxMap := make(map[string]driver.IndexMetadata)
		for _, idx := range tgtTbl.Indexes {
			tgtIdxMap[strings.ToLower(idx.Name)] = idx
		}

		for _, srcIdx := range srcTbl.Indexes {
			if strings.EqualFold(srcIdx.Name, "PRIMARY") {
				continue
			}
			_, idxExists := tgtIdxMap[strings.ToLower(srcIdx.Name)]
			if !idxExists {
				uniqueStr := ""
				if srcIdx.IsUnique {
					uniqueStr = "UNIQUE "
				}
				quotedCols := make([]string, len(srcIdx.Columns))
				for i, c := range srcIdx.Columns {
					quotedCols[i] = quoteIdent(c, tgtDriver)
				}
				sql := fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);",
					uniqueStr,
					quoteIdent(srcIdx.Name, tgtDriver),
					quoteIdent(srcTbl.Name, tgtDriver),
					strings.Join(quotedCols, ", "),
				)

				diffs = append(diffs, StructureDiffItem{
					ID:        fmt.Sprintf("diff-%d", diffIndex),
					Type:      "index_missing",
					Table:     srcTbl.Name,
					Index:     srcIdx.Name,
					Action:    "CREATE INDEX",
					SourceDef: fmt.Sprintf("Index %s (%s)", srcIdx.Name, strings.Join(srcIdx.Columns, ", ")),
					TargetDef: "Does not exist",
					SQL:       sql,
					Selected:  true,
				})
				diffIndex++
			}
		}
	}

	// If include_drops: check extra tables in target
	if req.IncludeDrops {
		for _, tgtTbl := range tgtSchema.Tables {
			if _, exists := srcTableMap[strings.ToLower(tgtTbl.Name)]; !exists {
				sql := fmt.Sprintf("DROP TABLE %s;", quoteIdent(tgtTbl.Name, tgtDriver))
				diffs = append(diffs, StructureDiffItem{
					ID:        fmt.Sprintf("diff-%d", diffIndex),
					Type:      "table_extra",
					Table:     tgtTbl.Name,
					Action:    "DROP TABLE",
					SourceDef: "Does not exist",
					TargetDef: fmt.Sprintf("Table %s", tgtTbl.Name),
					SQL:       sql,
					Selected:  false,
				})
				diffIndex++
			}
		}
	}

	// Build combined migration script
	var scriptLines []string
	for _, diff := range diffs {
		if diff.Selected && diff.SQL != "" {
			scriptLines = append(scriptLines, diff.SQL)
		}
	}

	writeJSON(w, http.StatusOK, SyncStructureCompareResponse{
		SourceDB:        req.SourceDB,
		TargetDB:        req.TargetDB,
		Diffs:           diffs,
		MigrationScript: strings.Join(scriptLines, "\n\n"),
	})
}

func (s *Server) handleSyncStructureApply(w http.ResponseWriter, r *http.Request) {
	var req SyncStructureApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	tgtSess, err := s.sessionMgr.GetSession(req.TargetSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target session not found: "+err.Error())
		return
	}

	if tgtSess.IsReadOnly() {
		writeError(w, http.StatusForbidden, "Target session is locked in READ-ONLY mode")
		return
	}

	// Production safety check
	if strings.EqualFold(tgtSess.Environment, "production") {
		if !req.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{
				"blocked":          true,
				"requires_confirm": true,
				"message":          "Target is a Production database. Confirmation challenge required.",
			})
			return
		}
		expectedChallenge := req.TargetDB
		if expectedChallenge == "" {
			expectedChallenge = tgtSess.Name
		}
		if !strings.EqualFold(strings.TrimSpace(req.ChallengeInput), expectedChallenge) && !strings.EqualFold(strings.TrimSpace(req.ChallengeInput), "MIGRATE") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Challenge confirmation failed: expected '%s' or 'MIGRATE', got '%s'", expectedChallenge, req.ChallengeInput))
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := tgtSess.UseDatabase(ctx, req.TargetDB); err != nil {
		writeError(w, http.StatusBadRequest, "target: "+err.Error())
		return
	}

	var executedCount int
	var errors []string

	for _, stmt := range req.Statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		start := time.Now()
		res, err := tgtSess.Session.Exec(ctx, stmt)
		dur := time.Since(start).Milliseconds()
		var rowsAff int64
		if res != nil {
			rowsAff = res.RowsAffected
		}
		tgtSess.RecordAudit(stmt, "schema_sync", dur, rowsAff, err, start)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error running [%s]: %s", stmt, err.Error()))
		} else {
			executedCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  len(errors) == 0,
		"executed": executedCount,
		"errors":   errors,
	})
}

// ----------------------------------------------------------------------------
// 2. Cross-Connection Data Transfer Handlers
// ----------------------------------------------------------------------------

func (s *Server) handleSyncDataTransfer(w http.ResponseWriter, r *http.Request) {
	var req SyncDataTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if req.SourceSessionID == "" || req.TargetSessionID == "" || len(req.Tables) == 0 {
		writeError(w, http.StatusBadRequest, "source_session_id, target_session_id, and at least one table are required")
		return
	}

	srcSess, err := s.sessionMgr.GetSession(req.SourceSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source session not found: "+err.Error())
		return
	}

	tgtSess, err := s.sessionMgr.GetSession(req.TargetSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target session not found: "+err.Error())
		return
	}

	if tgtSess.IsReadOnly() {
		writeError(w, http.StatusForbidden, "Target session is locked in READ-ONLY mode")
		return
	}

	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}
	if batchSize > 2000 {
		batchSize = 2000
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported by server")
		return
	}

	if err := srcSess.UseDatabase(r.Context(), req.SourceDB); err != nil {
		writeError(w, http.StatusBadRequest, "source: "+err.Error())
		return
	}
	if err := tgtSess.UseDatabase(r.Context(), req.TargetDB); err != nil {
		writeError(w, http.StatusBadRequest, "target: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sendEvent := func(evt map[string]any) {
		data, _ := json.Marshal(evt)
		_, _ = w.Write(append(data, '\n'))
		flusher.Flush()
	}

	ctx := r.Context()

	srcDriver := srcSess.DriverName
	tgtDriver := tgtSess.DriverName

	var grandTotalRows int64
	var hasErrors bool

	for _, tbl := range req.Tables {
		sendEvent(map[string]any{
			"event":  "table_start",
			"table":  tbl,
			"status": "Starting transfer for " + tbl,
		})

		// Auto-create table if requested
		if req.CreateTables {
			var ddl string
			var ddlErr error

			// If cross-driver, construct target-compatible DDL from schema inspection
			if !strings.EqualFold(srcDriver, tgtDriver) {
				if srcSchema, err := srcSess.Session.InspectSchema(ctx, req.SourceDB); err == nil {
					for _, t := range srcSchema.Tables {
						if strings.EqualFold(t.Name, tbl) {
							ddl = buildCreateTableDDL(t, srcDriver, tgtDriver)
							break
						}
					}
				}
			}

			// Fallback to GetTableDDL if same driver or if schema lookup didn't yield DDL
			if ddl == "" {
				ddl, ddlErr = srcSess.Session.GetTableDDL(ctx, tbl)
			}

			if ddlErr == nil && ddl != "" {
				_, _ = tgtSess.Session.Exec(ctx, ddl)
			}
		}

		// Stream rows from source
		querySQL := fmt.Sprintf("SELECT * FROM %s", quoteIdent(tbl, srcDriver))
		stream, err := srcSess.Session.QueryStream(ctx, querySQL, 0)
		if err != nil {
			hasErrors = true
			sendEvent(map[string]any{
				"event": "table_error",
				"table": tbl,
				"error": err.Error(),
			})
			continue
		}

		cols := stream.Columns()
		colNames := make([]string, len(cols))
		for i, c := range cols {
			colNames[i] = quoteIdent(c.Name, tgtDriver)
		}
		colListStr := strings.Join(colNames, ", ")

		// Begin transaction for per-table atomicity
		tx, err := tgtSess.Session.BeginTx(ctx)
		if err != nil {
			stream.Close()
			hasErrors = true
			sendEvent(map[string]any{
				"event": "table_error",
				"table": tbl,
				"error": "failed to begin transaction: " + err.Error(),
			})
			continue
		}

		var tableRowsCopied int64
		var tableErr error

		// Empty the target inside the transaction (DELETE, not TRUNCATE: MySQL TRUNCATE implicitly
		// commits) so a failed copy rolls back to the original rows instead of leaving the table empty.
		if req.TruncateTarget {
			if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s", quoteIdent(tbl, tgtDriver))); err != nil {
				tableErr = fmt.Errorf("failed to clear target table: %w", err)
			}
		}

		for tableErr == nil {
			// One placeholder per cell: keep each INSERT under the driver's bind-variable limit
			// (SQLite 32766, MySQL/PostgreSQL 65535) however wide the table is.
			maxParams := 65535
			if isSqliteDriver(tgtDriver) {
				maxParams = 32766
			}
			tblBatch := batchSize
			if n := len(cols); n > 0 && tblBatch*n > maxParams {
				tblBatch = max(1, maxParams/n)
			}
			chunk, err := stream.NextChunk(tblBatch)
			if err != nil && err != io.EOF {
				tableErr = err
				break
			}

			if len(chunk) > 0 {
				// Build multi-row parameterized insert statement
				rowPlaceholders := make([]string, len(chunk))
				var args []any
				for rIdx, row := range chunk {
					ph := make([]string, len(row))
					for cIdx, val := range row {
						args = append(args, val)
						ph[cIdx] = driver.Placeholder(tgtDriver, len(args))
					}
					rowPlaceholders[rIdx] = "(" + strings.Join(ph, ", ") + ")"
				}

				insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
					quoteIdent(tbl, tgtDriver),
					colListStr,
					strings.Join(rowPlaceholders, ", "),
				)

				if _, execErr := tx.Exec(ctx, insertSQL, args...); execErr != nil {
					tableErr = execErr
					break
				}

				tableRowsCopied += int64(len(chunk))

				sendEvent(map[string]any{
					"event":             "progress",
					"table":             tbl,
					"rows_copied":       tableRowsCopied,
					"total_transferred": grandTotalRows + tableRowsCopied,
				})
			}

			if err == io.EOF {
				break
			}
		}

		_ = stream.Close()

		if tableErr != nil {
			_ = tx.Rollback()
			hasErrors = true
			sendEvent(map[string]any{
				"event": "table_error",
				"table": tbl,
				"error": tableErr.Error(),
			})
			continue
		}

		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			hasErrors = true
			sendEvent(map[string]any{
				"event": "table_error",
				"table": tbl,
				"error": "failed to commit transaction: " + err.Error(),
			})
			continue
		}

		grandTotalRows += tableRowsCopied
		sendEvent(map[string]any{
			"event":       "table_done",
			"table":       tbl,
			"rows_copied": tableRowsCopied,
		})

		if tableRowsCopied > 0 {
			tgtSess.RecordAudit(
				fmt.Sprintf("-- Sync Data Transfer on %s: %d rows inserted", tbl, tableRowsCopied),
				"sync_transfer",
				0,
				tableRowsCopied,
				nil,
			)
		}
	}

	status := "Transfer completed successfully"
	if hasErrors {
		status = "Transfer completed with errors"
	}
	sendEvent(map[string]any{
		"event":        "done",
		"total_tables": len(req.Tables),
		"total_rows":   grandTotalRows,
		"status":       status,
		"has_errors":   hasErrors,
	})
}

// ----------------------------------------------------------------------------
// 3. Row-Level Data Sync Handlers
// ----------------------------------------------------------------------------

func (s *Server) handleSyncDataCompare(w http.ResponseWriter, r *http.Request) {
	var req SyncDataCompareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if req.SourceSessionID == "" || req.TargetSessionID == "" || req.Table == "" {
		writeError(w, http.StatusBadRequest, "source_session_id, target_session_id, and table are required")
		return
	}

	srcSess, err := s.sessionMgr.GetSession(req.SourceSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source session not found: "+err.Error())
		return
	}

	tgtSess, err := s.sessionMgr.GetSession(req.TargetSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target session not found: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := srcSess.UseDatabase(ctx, req.SourceDB); err != nil {
		writeError(w, http.StatusBadRequest, "source: "+err.Error())
		return
	}
	if err := tgtSess.UseDatabase(ctx, req.TargetDB); err != nil {
		writeError(w, http.StatusBadRequest, "target: "+err.Error())
		return
	}

	srcSchema, err := srcSess.Session.InspectSchema(ctx, req.SourceDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect source schema: "+err.Error())
		return
	}

	// Find table in source
	var srcTbl *driver.TableMetadata
	for _, t := range srcSchema.Tables {
		if strings.EqualFold(t.Name, req.Table) {
			tblCopy := t
			srcTbl = &tblCopy
			break
		}
	}

	if srcTbl == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("table '%s' not found in source database", req.Table))
		return
	}

	// Find Primary Key
	var pkCols []string
	var pkIndices []int
	for idx, col := range srcTbl.Columns {
		if col.IsPrimaryKey {
			pkCols = append(pkCols, col.Name)
			pkIndices = append(pkIndices, idx)
		}
	}

	if len(pkCols) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Table '%s' has no primary key. Row-level synchronization requires a primary key.", req.Table))
		return
	}

	srcDriver := srcSess.DriverName
	tgtDriver := tgtSess.DriverName

	// Select the source column list explicitly on both sides so rows align by column name even when
	// the target table orders its columns differently; quote per driver (MySQL backticks break PostgreSQL).
	selectSQL := func(drv string) string {
		cols := make([]string, len(srcTbl.Columns))
		for i, c := range srcTbl.Columns {
			cols[i] = quoteIdent(c.Name, drv)
		}
		pks := make([]string, len(pkCols))
		for i, pk := range pkCols {
			pks[i] = quoteIdent(pk, drv)
		}
		return fmt.Sprintf("SELECT %s FROM %s ORDER BY %s", strings.Join(cols, ", "), quoteIdent(req.Table, drv), strings.Join(pks, ", "))
	}

	// Fetch all rows from source. Comparison is in-memory and capped; one extra row is fetched to
	// detect overflow, because diffing a truncated window would emit bogus INSERT/DELETE operations.
	const maxSyncRows = 10000
	srcStream, err := srcSess.Session.QueryStream(ctx, selectSQL(srcDriver), maxSyncRows+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query source rows: "+err.Error())
		return
	}
	defer srcStream.Close()

	srcCols := srcStream.Columns()
	colNames := make([]string, len(srcCols))
	for i, c := range srcCols {
		colNames[i] = c.Name
	}

	var srcRows [][]any
	for {
		chunk, err := srcStream.NextChunk(1000)
		if err != nil && err != io.EOF {
			writeError(w, http.StatusInternalServerError, "error reading source rows: "+err.Error())
			return
		}
		srcRows = append(srcRows, chunk...)
		if err == io.EOF {
			break
		}
	}

	// Fetch all rows from target
	tgtStream, err := tgtSess.Session.QueryStream(ctx, selectSQL(tgtDriver), maxSyncRows+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query target rows: "+err.Error())
		return
	}
	defer tgtStream.Close()

	var tgtRows [][]any
	for {
		chunk, err := tgtStream.NextChunk(1000)
		if err != nil && err != io.EOF {
			writeError(w, http.StatusInternalServerError, "error reading target rows: "+err.Error())
			return
		}
		tgtRows = append(tgtRows, chunk...)
		if err == io.EOF {
			break
		}
	}

	if len(srcRows) > maxSyncRows || len(tgtRows) > maxSyncRows {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("table %q has more than %d rows on source or target; row-level sync would compare a partial window and produce wrong inserts/deletes. Use Data Transfer instead", req.Table, maxSyncRows))
		return
	}

	makePKKey := func(row []any, indices []int) string {
		var parts []string
		for _, idx := range indices {
			if idx < len(row) {
				parts = append(parts, syncValueKey(row[idx]))
			}
		}
		return strings.Join(parts, "\x1f") // unit separator: ":" could collide inside composite key values
	}

	// Map target rows
	targetMap := make(map[string][]any)
	for _, row := range tgtRows {
		key := makePKKey(row, pkIndices)
		targetMap[key] = row
	}

	var operations []SyncDataOperation
	visitedTargetKeys := make(map[string]bool)

	insertCount := 0
	updateCount := 0
	deleteCount := 0

	formatPKVal := func(row []any) any {
		if len(pkIndices) == 1 && pkIndices[0] < len(row) {
			return row[pkIndices[0]]
		}
		return makePKKey(row, pkIndices)
	}

	for _, sRow := range srcRows {
		key := makePKKey(sRow, pkIndices)
		tRow, exists := targetMap[key]
		if !exists {
			// Row in source but not in target -> INSERT
			operations = append(operations, SyncDataOperation{
				Type: "insert",
				PK:   syncWireValue(formatPKVal(sRow)),
				Data: syncWireRow(sRow),
			})
			insertCount++
		} else {
			visitedTargetKeys[key] = true
			if !rowValuesEqual(sRow, tRow) {
				// Row exists in both but data has changed -> UPDATE
				operations = append(operations, SyncDataOperation{
					Type:    "update",
					PK:      syncWireValue(formatPKVal(sRow)),
					OldData: syncWireRow(tRow),
					Data:    syncWireRow(sRow),
				})
				updateCount++
			}
		}
	}

	if req.DeleteMissing {
		for _, tRow := range tgtRows {
			key := makePKKey(tRow, pkIndices)
			if !visitedTargetKeys[key] {
				// Row exists in target but not in source -> DELETE
				operations = append(operations, SyncDataOperation{
					Type: "delete",
					PK:   syncWireValue(formatPKVal(tRow)),
					Data: syncWireRow(tRow),
				})
				deleteCount++
			}
		}
	}

	writeJSON(w, http.StatusOK, SyncDataCompareResponse{
		Table:     req.Table,
		PKColumns: pkCols,
		Columns:   colNames,
		Summary: map[string]int{
			"insert_count": insertCount,
			"update_count": updateCount,
			"delete_count": deleteCount,
			"total_diffs":  len(operations),
		},
		Operations: operations,
	})
}

func (s *Server) handleSyncDataApply(w http.ResponseWriter, r *http.Request) {
	var req SyncDataApplyRequest
	dec := json.NewDecoder(r.Body)
	dec.UseNumber() // keep BIGINT/DECIMAL digits exact instead of rounding through float64
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	tgtSess, err := s.sessionMgr.GetSession(req.TargetSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target session not found: "+err.Error())
		return
	}

	if tgtSess.IsReadOnly() {
		writeError(w, http.StatusForbidden, "Target session is locked in READ-ONLY mode")
		return
	}

	// Production check
	if strings.EqualFold(tgtSess.Environment, "production") {
		if !req.Confirmed {
			writeJSON(w, http.StatusOK, map[string]any{
				"blocked":          true,
				"requires_confirm": true,
				"message":          "Target is a Production database. Confirmation challenge required.",
			})
			return
		}
		if !strings.EqualFold(strings.TrimSpace(req.ChallengeInput), req.Table) && !strings.EqualFold(strings.TrimSpace(req.ChallengeInput), "SYNC") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Challenge confirmation failed: expected '%s' or 'SYNC', got '%s'", req.Table, req.ChallengeInput))
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := tgtSess.UseDatabase(ctx, req.TargetDB); err != nil {
		writeError(w, http.StatusBadRequest, "target: "+err.Error())
		return
	}
	syncStart := time.Now()

	tgtDriver := tgtSess.DriverName
	tgtSchema, err := tgtSess.Session.InspectSchema(ctx, req.TargetDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect target schema: "+err.Error())
		return
	}

	var tgtTbl *driver.TableMetadata
	for _, t := range tgtSchema.Tables {
		if strings.EqualFold(t.Name, req.Table) {
			tblCopy := t
			tgtTbl = &tblCopy
			break
		}
	}
	if tgtTbl == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("table '%s' not found on target", req.Table))
		return
	}

	var pkIndices []int
	for idx, c := range tgtTbl.Columns {
		if c.IsPrimaryKey {
			pkIndices = append(pkIndices, idx)
		}
	}
	if len(pkIndices) == 0 {
		writeError(w, http.StatusBadRequest, "table has no primary key")
		return
	}

	// dataIdx maps each target column to its position in op.Data. Compare emits rows in the order
	// of req.Columns (source column order); legacy clients without columns fall back to target order.
	dataIdx := make([]int, len(tgtTbl.Columns))
	for i, c := range tgtTbl.Columns {
		dataIdx[i] = i
		if len(req.Columns) > 0 {
			dataIdx[i] = slices.IndexFunc(req.Columns, func(n string) bool { return strings.EqualFold(n, c.Name) })
		}
	}
	valueAt := func(op SyncDataOperation, tgtIdx int) (any, bool) {
		if di := dataIdx[tgtIdx]; di >= 0 && di < len(op.Data) {
			return fromSyncWire(op.Data[di], tgtTbl.Columns[tgtIdx].Type), true
		}
		return nil, false
	}
	// pkWhere builds the PK predicate; it refuses to guess (NULL) a missing PK value, which would
	// otherwise silently match nothing or the wrong row.
	pkWhere := func(op SyncDataOperation, pIdx int) (string, []any, error) {
		var clauses []string
		var args []any
		for _, pkIdx := range pkIndices {
			v, ok := valueAt(op, pkIdx)
			if !ok {
				if len(pkIndices) != 1 || op.PK == nil {
					return "", nil, fmt.Errorf("missing primary key column %s", tgtTbl.Columns[pkIdx].Name)
				}
				v = fromSyncWire(op.PK, tgtTbl.Columns[pkIdx].Type)
			}
			clauses = append(clauses, fmt.Sprintf("%s = %s", quoteIdent(tgtTbl.Columns[pkIdx].Name, tgtDriver), driver.Placeholder(tgtDriver, pIdx)))
			args = append(args, v)
			pIdx++
		}
		return strings.Join(clauses, " AND "), args, nil
	}

	var inserted, updated, deleted int
	var errors []string

	// All operations apply in one transaction: a failure rolls everything back, so the target is
	// never left half-synced (which the next compare would misreport as new diffs).
	tx, err := tgtSess.Session.BeginTx(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin target transaction: "+err.Error())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, op := range req.Operations {
		switch op.Type {
		case "insert":
			var cols, placeholders []string
			var args []any
			for i, c := range tgtTbl.Columns {
				if v, ok := valueAt(op, i); ok {
					cols = append(cols, quoteIdent(c.Name, tgtDriver))
					args = append(args, v)
					placeholders = append(placeholders, driver.Placeholder(tgtDriver, len(args)))
				}
			}
			sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
				quoteIdent(req.Table, tgtDriver),
				strings.Join(cols, ", "),
				strings.Join(placeholders, ", "),
			)
			if _, err := tx.Exec(ctx, sql, args...); err != nil {
				errors = append(errors, fmt.Sprintf("insert error (pk %v): %s", op.PK, err.Error()))
			} else {
				inserted++
			}

		case "update":
			var setClauses []string
			var args []any
			for i, c := range tgtTbl.Columns {
				if c.IsPrimaryKey {
					continue
				}
				// Columns absent from the source row are left untouched rather than overwritten with NULL.
				if v, ok := valueAt(op, i); ok {
					args = append(args, v)
					setClauses = append(setClauses, fmt.Sprintf("%s = %s", quoteIdent(c.Name, tgtDriver), driver.Placeholder(tgtDriver, len(args))))
				}
			}
			if len(setClauses) == 0 {
				continue
			}
			where, whereArgs, err := pkWhere(op, len(args)+1)
			if err != nil {
				errors = append(errors, fmt.Sprintf("update error (pk %v): %s", op.PK, err.Error()))
				continue
			}
			sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
				quoteIdent(req.Table, tgtDriver),
				strings.Join(setClauses, ", "),
				where,
			)
			if _, err := tx.Exec(ctx, sql, append(args, whereArgs...)...); err != nil {
				errors = append(errors, fmt.Sprintf("update error (pk %v): %s", op.PK, err.Error()))
			} else {
				updated++
			}

		case "delete":
			where, args, err := pkWhere(op, 1)
			if err != nil {
				errors = append(errors, fmt.Sprintf("delete error (pk %v): %s", op.PK, err.Error()))
				continue
			}
			sql := fmt.Sprintf("DELETE FROM %s WHERE %s", quoteIdent(req.Table, tgtDriver), where)
			if _, err := tx.Exec(ctx, sql, args...); err != nil {
				errors = append(errors, fmt.Sprintf("delete error (pk %v): %s", op.PK, err.Error()))
			} else {
				deleted++
			}
		}
	}

	if len(errors) == 0 {
		if err := tx.Commit(); err != nil {
			errors = append(errors, "commit failed: "+err.Error())
		} else {
			committed = true
		}
	}
	if !committed {
		_ = tx.Rollback()
		committed = true // rolled back explicitly; skip the deferred rollback
		errors = append(errors, "no changes were applied (transaction rolled back)")
		inserted, updated, deleted = 0, 0, 0
	}

	if inserted > 0 || updated > 0 || deleted > 0 {
		tgtSess.RecordAudit(
			fmt.Sprintf("-- Sync Data Apply on %s: %d inserted, %d updated, %d deleted", req.Table, inserted, updated, deleted),
			"sync_transfer",
			time.Since(syncStart).Milliseconds(),
			int64(inserted+updated+deleted),
			nil,
			syncStart,
		)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  len(errors) == 0,
		"inserted": inserted,
		"updated":  updated,
		"deleted":  deleted,
		"errors":   errors,
	})
}

// ----------------------------------------------------------------------------
// Helper Functions
// ----------------------------------------------------------------------------

func isPgDriver(d string) bool {
	return strings.EqualFold(d, "postgres") || strings.EqualFold(d, "postgresql") || strings.EqualFold(d, "pg")
}

func isSqliteDriver(d string) bool {
	return strings.EqualFold(d, "sqlite") || strings.EqualFold(d, "sqlite3")
}

var reMySQLTypeModifiers = regexp.MustCompile(`(?i)\s+(unsigned|zerofill|signed)\b`)

// normalizeColumnType maps a source column type onto a type the target driver accepts.
// ponytail: covers the common MySQL<->PostgreSQL<->SQLite types; exotic types pass through unchanged.
func normalizeColumnType(t string, driverName string) string {
	raw := strings.TrimSpace(t)
	upper := strings.ToUpper(raw)

	if isSqliteDriver(driverName) {
		if strings.Contains(upper, "INT") {
			return "INTEGER"
		}
		if strings.Contains(upper, "TEXT") || strings.Contains(upper, "CHAR") || strings.HasPrefix(upper, "ENUM") || strings.HasPrefix(upper, "SET") || strings.HasPrefix(upper, "JSON") || upper == "UUID" {
			return "TEXT"
		}
		if strings.HasPrefix(upper, "DOUBLE") || strings.HasPrefix(upper, "FLOAT") || strings.HasPrefix(upper, "REAL") {
			return "REAL"
		}
		if strings.HasPrefix(upper, "DECIMAL") || strings.HasPrefix(upper, "NUMERIC") {
			return "NUMERIC"
		}
		if strings.Contains(upper, "BLOB") || strings.Contains(upper, "BINARY") || upper == "BYTEA" {
			return "BLOB"
		}
		return raw
	}

	if isPgDriver(driverName) {
		base := strings.TrimSpace(reMySQLTypeModifiers.ReplaceAllString(upper, ""))
		unsigned := base != upper
		switch {
		case base == "TINYINT(1)":
			return "SMALLINT"
		case strings.HasPrefix(base, "BIGINT"):
			if unsigned {
				return "NUMERIC(20)"
			}
			return "BIGINT"
		case strings.HasPrefix(base, "MEDIUMINT"):
			return "INTEGER"
		case strings.HasPrefix(base, "INT(") || base == "INT" || base == "INTEGER":
			if unsigned {
				return "BIGINT"
			}
			return "INTEGER"
		case strings.HasPrefix(base, "TINYINT") || strings.HasPrefix(base, "SMALLINT"):
			if unsigned && strings.HasPrefix(base, "SMALLINT") {
				return "INTEGER"
			}
			return "SMALLINT"
		case base == "YEAR" || strings.HasPrefix(base, "YEAR("):
			return "SMALLINT"
		case strings.Contains(base, "TEXT") || strings.HasPrefix(base, "ENUM") || strings.HasPrefix(base, "SET("):
			return "TEXT"
		case base == "DATETIME":
			return "TIMESTAMP"
		case strings.HasPrefix(base, "DATETIME("):
			return "TIMESTAMP" + strings.TrimPrefix(base, "DATETIME")
		case base == "DOUBLE" || strings.HasPrefix(base, "DOUBLE("):
			return "DOUBLE PRECISION"
		case base == "FLOAT" || strings.HasPrefix(base, "FLOAT("):
			return "REAL"
		case strings.Contains(base, "BLOB") || strings.Contains(base, "BINARY"):
			return "BYTEA"
		}
		if unsigned {
			return base
		}
		return raw
	}

	// MySQL/MariaDB target: map PostgreSQL information_schema type names (never produced by MySQL).
	switch strings.ToLower(raw) {
	case "integer":
		return "INT"
	case "boolean":
		return "TINYINT(1)"
	case "character varying", "character", "uuid":
		if strings.ToLower(raw) == "uuid" {
			return "CHAR(36)"
		}
		return "TEXT"
	case "timestamp without time zone":
		return "DATETIME(6)"
	case "timestamp with time zone":
		return "TIMESTAMP(6)"
	case "time without time zone", "time with time zone":
		return "TIME(6)"
	case "double precision":
		return "DOUBLE"
	case "real":
		return "FLOAT"
	case "numeric":
		return "DECIMAL(65,30)"
	case "bytea":
		return "LONGBLOB"
	case "jsonb", "json":
		return "JSON"
	}
	return raw
}

var (
	rePgCast          = regexp.MustCompile(`::[A-Za-z_][\w ]*(\[\])?(\(\s*\d+(\s*,\s*\d+)?\s*\))?`)
	reNumericLiteral  = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][-+]?\d+)?$`)
	reFuncCallExpr    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\(.*\)$`) // MariaDB unquoted expression default, e.g. uuid()
	reCurrentTimeExpr = regexp.MustCompile(`(?i)^(CURRENT_TIMESTAMP|CURRENT_DATE|CURRENT_TIME|LOCALTIMESTAMP|LOCALTIME|NOW)\s*(\(\s*\d*\s*\))?$`)
)

// renderColumnDefault returns the DEFAULT expression (without the keyword) for col as read from
// srcDriver, rendered for tgtDriver; "" means no default. MySQL's information_schema returns literal
// defaults unquoted and marks expression defaults with EXTRA=DEFAULT_GENERATED, while PostgreSQL and
// SQLite already return SQL expressions (PostgreSQL with ::type casts).
func renderColumnDefault(col driver.ColumnMetadata, srcDriver, tgtDriver string) string {
	v := strings.TrimSpace(col.DefaultValue)
	if v == "" {
		return ""
	}
	if strings.EqualFold(v, "NULL") {
		return "NULL"
	}
	if reCurrentTimeExpr.MatchString(v) {
		if !strings.EqualFold(srcDriver, tgtDriver) && (isPgDriver(tgtDriver) || isSqliteDriver(tgtDriver)) {
			if strings.HasPrefix(strings.ToUpper(v), "CURRENT_TIMESTAMP") || strings.HasPrefix(strings.ToUpper(v), "NOW") {
				return "CURRENT_TIMESTAMP"
			}
		}
		return v
	}

	srcIsMySQL := !isPgDriver(srcDriver) && !isSqliteDriver(srcDriver)
	if srcIsMySQL {
		if strings.Contains(strings.ToUpper(col.Extra), "DEFAULT_GENERATED") {
			// MySQL 8 expression default: must be parenthesized when re-emitted.
			return "(" + v + ")"
		}
		// MariaDB already returns quoted literals / expressions; bit literals are SQL as-is.
		if strings.HasPrefix(v, "'") || strings.HasPrefix(strings.ToLower(v), "b'") || reFuncCallExpr.MatchString(v) {
			return v
		}
		if reNumericLiteral.MatchString(v) {
			return v
		}
		return driver.EscapeSQLString(v, tgtDriver)
	}

	// PostgreSQL / SQLite source: already an SQL expression.
	if isPgDriver(srcDriver) && !isPgDriver(tgtDriver) {
		if strings.HasPrefix(strings.ToLower(v), "nextval(") {
			return "" // sequence default has no portable equivalent (auto-increment is a column property)
		}
		v = strings.TrimSpace(stripPgCasts(v))
		if strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")") && strings.Count(v, "(") == 1 {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
		if strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") && len(v) >= 2 && !isSqliteDriver(tgtDriver) {
			// Re-escape for MySQL (backslashes are escapes there).
			return driver.EscapeSQLString(strings.ReplaceAll(v[1:len(v)-1], "''", "'"), tgtDriver)
		}
	}
	return v
}

// stripPgCasts removes ::type casts outside quoted literals ('a::b'::text keeps 'a::b').
func stripPgCasts(v string) string {
	var out strings.Builder
	for i := 0; i < len(v); {
		if v[i] == '\'' {
			j := i + 1
			for j < len(v) {
				if v[j] == '\'' {
					if j+1 < len(v) && v[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			end := min(j+1, len(v))
			out.WriteString(v[i:end])
			i = end
			continue
		}
		next := strings.IndexByte(v[i:], '\'')
		if next < 0 {
			next = len(v) - i
		}
		out.WriteString(rePgCast.ReplaceAllString(v[i:i+next], ""))
		i += next
	}
	return out.String()
}

// canonDefault folds equivalent current-time spellings so they don't count as a difference.
func canonDefault(d string) string {
	if reCurrentTimeExpr.MatchString(d) && (strings.HasPrefix(strings.ToUpper(d), "CURRENT_TIMESTAMP") || strings.HasPrefix(strings.ToUpper(d), "NOW")) {
		return "CURRENT_TIMESTAMP"
	}
	return d
}

// extraClause keeps only the MySQL EXTRA attributes that are valid in a column definition
// (auto_increment, ON UPDATE ..., INVISIBLE); DEFAULT_GENERATED and VIRTUAL/STORED GENERATED are
// metadata flags, not syntax.
func extraClause(extra string) string {
	e := strings.TrimSpace(extra)
	if e == "" {
		return ""
	}
	upper := strings.ToUpper(e)
	var parts []string
	if strings.Contains(upper, "AUTO_INCREMENT") {
		parts = append(parts, "AUTO_INCREMENT")
	}
	if i := strings.Index(upper, "ON UPDATE "); i >= 0 {
		rest := e[i:]
		if j := strings.Index(strings.ToUpper(rest), " INVISIBLE"); j >= 0 {
			rest = rest[:j]
		}
		parts = append(parts, strings.TrimSpace(rest))
	}
	if strings.Contains(upper, "INVISIBLE") {
		parts = append(parts, "INVISIBLE")
	}
	return strings.Join(parts, " ")
}

func buildColumnDefinition(col driver.ColumnMetadata, srcDriver, driverName string) string {
	clause := normalizeColumnType(col.Type, driverName)
	if !col.IsNullable {
		clause += " NOT NULL"
	}
	if def := renderColumnDefault(col, srcDriver, driverName); def != "" {
		clause += " DEFAULT " + def
	}
	srcIsMySQL := !isPgDriver(srcDriver) && !isSqliteDriver(srcDriver)
	if srcIsMySQL && !isPgDriver(driverName) && !isSqliteDriver(driverName) {
		if x := extraClause(col.Extra); x != "" {
			clause += " " + x
		}
	}
	return clause
}

func buildCreateTableDDL(tbl driver.TableMetadata, srcDriver, driverName string) string {
	var lines []string
	var pkCols []string

	for _, col := range tbl.Columns {
		def := fmt.Sprintf("  %s %s", quoteIdent(col.Name, driverName), buildColumnDefinition(col, srcDriver, driverName))
		lines = append(lines, def)
		if col.IsPrimaryKey {
			pkCols = append(pkCols, quoteIdent(col.Name, driverName))
		}
	}

	if len(pkCols) > 0 {
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n);", quoteIdent(tbl.Name, driverName), strings.Join(lines, ",\n"))
}

// columnEquals compares a source column with a target column as the target would see them,
// so cross-driver syncs don't flag every column merely for spelling types/defaults differently.
func columnEquals(src, tgt driver.ColumnMetadata, srcDriver, tgtDriver string) bool {
	if !strings.EqualFold(normalizeColumnType(src.Type, tgtDriver), normalizeColumnType(tgt.Type, tgtDriver)) {
		return false
	}
	if src.IsNullable != tgt.IsNullable {
		return false
	}
	return strings.EqualFold(canonDefault(renderColumnDefault(src, srcDriver, tgtDriver)), canonDefault(renderColumnDefault(tgt, tgtDriver, tgtDriver)))
}

// maxJSONSafeInt is the largest integer a browser's JSON.parse (float64) represents exactly.
const maxJSONSafeInt = 1<<53 - 1

// syncWireValue makes a scanned value survive the browser round-trip between compare and apply:
// integers beyond 2^53 travel as decimal strings and non-UTF-8 (binary) values as {"$base64": ...}
// (encoding/json would otherwise replace invalid bytes with U+FFFD). fromSyncWire reverses this.
func syncWireValue(v any) any {
	switch x := v.(type) {
	case int64:
		if x > maxJSONSafeInt || x < -maxJSONSafeInt {
			return strconv.FormatInt(x, 10)
		}
	case uint64:
		if x > maxJSONSafeInt {
			return strconv.FormatUint(x, 10)
		}
	case []byte:
		if !utf8.Valid(x) {
			return map[string]string{"$base64": base64.StdEncoding.EncodeToString(x)}
		}
		return string(x)
	case string:
		if !utf8.ValidString(x) {
			return map[string]string{"$base64": base64.StdEncoding.EncodeToString([]byte(x))}
		}
	}
	return v
}

func syncWireRow(row []any) []any {
	out := make([]any, len(row))
	for i, v := range row {
		out[i] = syncWireValue(v)
	}
	return out
}

var reIntegerColumnType = regexp.MustCompile(`(?i)^((tiny|small|medium|big)?int(eger|[248])?|(small|big)?serial)\b`)

// isIntegerColumnType matches integer types only (not INTERVAL/POINT, which merely contain "INT").
func isIntegerColumnType(t string) bool {
	return reIntegerColumnType.MatchString(strings.TrimSpace(t))
}

// fromSyncWire converts a JSON-decoded (UseNumber) apply value back into a bind parameter for a
// target column of colType. Integers are bound as int64/uint64 so comparisons never go through
// float64 (MySQL compares BIGINT with a string/double argument as DOUBLE and could match a neighbour).
func fromSyncWire(v any, colType string) any {
	switch x := v.(type) {
	case map[string]any:
		if b64, ok := x["$base64"].(string); ok && len(x) == 1 {
			if b, err := base64.StdEncoding.DecodeString(b64); err == nil {
				return b
			}
		}
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if u, err := strconv.ParseUint(string(x), 10, 64); err == nil {
			return u
		}
		return string(x) // decimal text keeps full precision for DECIMAL/NUMERIC columns
	case string:
		if isIntegerColumnType(colType) {
			if i, err := strconv.ParseInt(x, 10, 64); err == nil {
				return i
			}
			if u, err := strconv.ParseUint(x, 10, 64); err == nil {
				return u
			}
		}
	}
	return v
}

// syncValueKey canonicalizes a scanned value for cross-driver comparison: drivers return the same
// logical value as int64, string or []byte, and NULL must never collide with any text value.
func syncValueKey(v any) string {
	switch x := v.(type) {
	case nil:
		return "\x00NULL"
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

func rowValuesEqual(rowA, rowB []any) bool {
	return slices.EqualFunc(rowA, rowB, func(a, b any) bool {
		return syncValueKey(a) == syncValueKey(b)
	})
}
