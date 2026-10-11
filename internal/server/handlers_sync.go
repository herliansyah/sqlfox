package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

// ----------------------------------------------------------------------------
// Milestone 11: Schema & Data Synchronization Engine
// ----------------------------------------------------------------------------

type StructureDiffItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`      // table_missing, column_missing, column_mismatch, index_missing, table_extra
	Table     string `json:"table"`
	Column    string `json:"column,omitempty"`
	Index     string `json:"index,omitempty"`
	Action    string `json:"action"`    // CREATE TABLE, ADD COLUMN, MODIFY COLUMN, CREATE INDEX, DROP TABLE
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
	Table       string              `json:"table"`
	PKColumns   []string            `json:"pk_columns"`
	Columns     []string            `json:"columns"`
	Summary     map[string]int      `json:"summary"`
	Operations  []SyncDataOperation `json:"operations"`
}

type SyncDataApplyRequest struct {
	TargetSessionID string              `json:"target_session_id"`
	TargetDB        string              `json:"target_db"`
	Table           string              `json:"table"`
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

	_ = srcSess.UseDatabase(ctx, req.SourceDB)
	_ = tgtSess.UseDatabase(ctx, req.TargetDB)

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

	// Check tables in source
	for _, srcTbl := range srcSchema.Tables {
		tgtTbl, exists := tgtTableMap[strings.ToLower(srcTbl.Name)]
		if !exists {
			// Missing table in target -> CREATE TABLE
			ddl, err := srcSess.Session.GetTableDDL(ctx, srcTbl.Name)
			if err != nil || strings.TrimSpace(ddl) == "" {
				ddl = buildCreateTableDDL(srcTbl, tgtDriver)
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
				clause := buildColumnDefinition(srcCol)
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
				if !columnEquals(srcCol, tgtCol) {
					clause := buildColumnDefinition(srcCol)
					var sql string
					if tgtDriver == "sqlite" {
						sql = fmt.Sprintf("-- SQLite: column %s definition differs (%s vs %s)", srcCol.Name, srcCol.Type, tgtCol.Type)
					} else if tgtDriver == "postgres" || tgtDriver == "postgresql" || tgtDriver == "pg" {
						// ponytail: Atomic granular ALTER statements for PostgreSQL; upgrade path: full alter column state machine
						var pgStmts []string
						tblQ := quoteIdent(srcTbl.Name, tgtDriver)
						colQ := quoteIdent(srcCol.Name, tgtDriver)
						if !strings.EqualFold(srcCol.Type, tgtCol.Type) {
							pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tblQ, colQ, srcCol.Type))
						}
						if srcCol.IsNullable != tgtCol.IsNullable {
							if srcCol.IsNullable {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", tblQ, colQ))
							} else {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tblQ, colQ))
							}
						}
						if srcCol.DefaultValue != tgtCol.DefaultValue {
							if srcCol.DefaultValue == "" {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", tblQ, colQ))
							} else {
								pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", tblQ, colQ, srcCol.DefaultValue))
							}
						}
						if len(pgStmts) == 0 {
							pgStmts = append(pgStmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tblQ, colQ, srcCol.Type))
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
						TargetDef: fmt.Sprintf("%s %s", tgtCol.Name, buildColumnDefinition(tgtCol)),
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
						TargetDef: fmt.Sprintf("%s %s", tgtCol.Name, buildColumnDefinition(tgtCol)),
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

	if tgtSess.ReadOnly {
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

	_ = tgtSess.UseDatabase(ctx, req.TargetDB)

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

	if tgtSess.ReadOnly {
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
	_ = srcSess.UseDatabase(ctx, req.SourceDB)
	_ = tgtSess.UseDatabase(ctx, req.TargetDB)

	srcDriver := srcSess.DriverName
	tgtDriver := tgtSess.DriverName

	var grandTotalRows int64

	for _, tbl := range req.Tables {
		sendEvent(map[string]any{
			"event":  "table_start",
			"table":  tbl,
			"status": "Starting transfer for " + tbl,
		})

		// Auto-create table if requested
		if req.CreateTables {
			ddl, err := srcSess.Session.GetTableDDL(ctx, tbl)
			if err == nil && ddl != "" {
				// Strip foreign keys or dialect specifics if cross-driver, but standard exec is best-effort
				_, _ = tgtSess.Session.Exec(ctx, ddl)
			}
		}

		// Truncate table if requested
		if req.TruncateTarget {
			truncSQL := fmt.Sprintf("TRUNCATE TABLE %s", quoteIdent(tbl, tgtDriver))
			if tgtDriver == "sqlite" {
				truncSQL = fmt.Sprintf("DELETE FROM %s", quoteIdent(tbl, tgtDriver))
			}
			_, _ = tgtSess.Session.Exec(ctx, truncSQL)
		}

		// Stream rows from source
		querySQL := fmt.Sprintf("SELECT * FROM %s", quoteIdent(tbl, srcDriver))
		stream, err := srcSess.Session.QueryStream(ctx, querySQL, 0)
		if err != nil {
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

		var tableRowsCopied int64

		for {
			chunk, err := stream.NextChunk(batchSize)
			if err != nil && err != io.EOF {
				sendEvent(map[string]any{
					"event": "table_error",
					"table": tbl,
					"error": err.Error(),
				})
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

				if _, execErr := tgtSess.Session.Exec(ctx, insertSQL, args...); execErr != nil {
					sendEvent(map[string]any{
						"event": "table_error",
						"table": tbl,
						"error": execErr.Error(),
					})
					break
				}

				tableRowsCopied += int64(len(chunk))
				grandTotalRows += int64(len(chunk))

				sendEvent(map[string]any{
					"event":             "progress",
					"table":             tbl,
					"rows_copied":       tableRowsCopied,
					"total_transferred": grandTotalRows,
				})
			}

			if err == io.EOF {
				break
			}
		}

		_ = stream.Close()

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

	sendEvent(map[string]any{
		"event":        "done",
		"total_tables": len(req.Tables),
		"total_rows":   grandTotalRows,
		"status":       "Transfer completed successfully",
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

	_ = srcSess.UseDatabase(ctx, req.SourceDB)
	_ = tgtSess.UseDatabase(ctx, req.TargetDB)

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

	// Order by PK for fast deterministic alignment
	var pkOrderClauses []string
	for _, pk := range pkCols {
		pkOrderClauses = append(pkOrderClauses, quoteIdent(pk, srcDriver))
	}
	orderBySQL := " ORDER BY " + strings.Join(pkOrderClauses, ", ")

	// Fetch all rows from source (capped at 10,000 for safe comparison)
	srcStream, err := srcSess.Session.QueryStream(ctx, fmt.Sprintf("SELECT * FROM %s%s", quoteIdent(req.Table, srcDriver), orderBySQL), 10000)
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
	tgtStream, err := tgtSess.Session.QueryStream(ctx, fmt.Sprintf("SELECT * FROM %s%s", quoteIdent(req.Table, tgtDriver), orderBySQL), 10000)
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

	makePKKey := func(row []any, indices []int) string {
		var parts []string
		for _, idx := range indices {
			if idx < len(row) {
				parts = append(parts, fmt.Sprintf("%v", row[idx]))
			}
		}
		return strings.Join(parts, ":")
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

	for _, sRow := range srcRows {
		key := makePKKey(sRow, pkIndices)
		tRow, exists := targetMap[key]
		if !exists {
			// Row in source but not in target -> INSERT
			operations = append(operations, SyncDataOperation{
				Type: "insert",
				PK:   sRow[pkIndices[0]],
				Data: sRow,
			})
			insertCount++
		} else {
			visitedTargetKeys[key] = true
			if !rowValuesEqual(sRow, tRow) {
				// Row exists in both but data has changed -> UPDATE
				operations = append(operations, SyncDataOperation{
					Type:    "update",
					PK:      sRow[pkIndices[0]],
					OldData: tRow,
					Data:    sRow,
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
					PK:   tRow[pkIndices[0]],
					Data: tRow,
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	tgtSess, err := s.sessionMgr.GetSession(req.TargetSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target session not found: "+err.Error())
		return
	}

	if tgtSess.ReadOnly {
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

	_ = tgtSess.UseDatabase(ctx, req.TargetDB)
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

	var pkCol string
	for _, c := range tgtTbl.Columns {
		if c.IsPrimaryKey {
			pkCol = c.Name
			break
		}
	}
	if pkCol == "" {
		writeError(w, http.StatusBadRequest, "table has no primary key")
		return
	}

	colNames := make([]string, len(tgtTbl.Columns))
	for i, c := range tgtTbl.Columns {
		colNames[i] = quoteIdent(c.Name, tgtDriver)
	}
	colListStr := strings.Join(colNames, ", ")

	var inserted, updated, deleted int
	var errors []string

	for _, op := range req.Operations {
		switch op.Type {
		case "insert":
			placeholders := make([]string, len(op.Data))
			for i := range op.Data {
				placeholders[i] = driver.Placeholder(tgtDriver, i+1)
			}
			sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
				quoteIdent(req.Table, tgtDriver),
				colListStr,
				strings.Join(placeholders, ", "),
			)
			if _, err := tgtSess.Session.Exec(ctx, sql, op.Data...); err != nil {
				errors = append(errors, fmt.Sprintf("insert error (pk %v): %s", op.PK, err.Error()))
			} else {
				inserted++
			}

		case "update":
			var setClauses []string
			var args []any
			pIdx := 1
			for i, c := range tgtTbl.Columns {
				if c.IsPrimaryKey {
					continue
				}
				setClauses = append(setClauses, fmt.Sprintf("%s = %s", quoteIdent(c.Name, tgtDriver), driver.Placeholder(tgtDriver, pIdx)))
				pIdx++
				if i < len(op.Data) {
					args = append(args, op.Data[i])
				} else {
					args = append(args, nil)
				}
			}
			args = append(args, op.PK)
			sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s",
				quoteIdent(req.Table, tgtDriver),
				strings.Join(setClauses, ", "),
				quoteIdent(pkCol, tgtDriver),
				driver.Placeholder(tgtDriver, pIdx),
			)
			if _, err := tgtSess.Session.Exec(ctx, sql, args...); err != nil {
				errors = append(errors, fmt.Sprintf("update error (pk %v): %s", op.PK, err.Error()))
			} else {
				updated++
			}

		case "delete":
			sql := fmt.Sprintf("DELETE FROM %s WHERE %s = %s",
				quoteIdent(req.Table, tgtDriver),
				quoteIdent(pkCol, tgtDriver),
				driver.Placeholder(tgtDriver, 1),
			)
			if _, err := tgtSess.Session.Exec(ctx, sql, op.PK); err != nil {
				errors = append(errors, fmt.Sprintf("delete error (pk %v): %s", op.PK, err.Error()))
			} else {
				deleted++
			}
		}
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

func buildColumnDefinition(col driver.ColumnMetadata) string {
	clause := col.Type
	if !col.IsNullable {
		clause += " NOT NULL"
	}
	if col.DefaultValue != "" {
		if strings.EqualFold(col.DefaultValue, "null") || strings.Contains(col.DefaultValue, "CURRENT_TIMESTAMP") || strings.HasPrefix(col.DefaultValue, "'") {
			clause += " DEFAULT " + col.DefaultValue
		} else {
			clause += " DEFAULT '" + strings.ReplaceAll(col.DefaultValue, "'", "''") + "'"
		}
	}
	if col.Extra != "" {
		clause += " " + col.Extra
	}
	return clause
}

func buildCreateTableDDL(tbl driver.TableMetadata, driverName string) string {
	var lines []string
	var pkCols []string

	for _, col := range tbl.Columns {
		def := fmt.Sprintf("  %s %s", quoteIdent(col.Name, driverName), buildColumnDefinition(col))
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

func columnEquals(a, b driver.ColumnMetadata) bool {
	// Standardize types (e.g. INT vs int, VARCHAR(255) vs varchar(255))
	normA := strings.ToLower(strings.TrimSpace(a.Type))
	normB := strings.ToLower(strings.TrimSpace(b.Type))
	if normA != normB {
		return false
	}
	if a.IsNullable != b.IsNullable {
		return false
	}
	if strings.TrimSpace(a.DefaultValue) != strings.TrimSpace(b.DefaultValue) {
		return false
	}
	return true
}

func rowValuesEqual(rowA, rowB []any) bool {
	return slices.EqualFunc(rowA, rowB, func(a, b any) bool {
		return fmt.Sprint(a) == fmt.Sprint(b)
	})
}
