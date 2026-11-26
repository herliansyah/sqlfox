package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/safety"
	"net/http"
	"strings"
	"time"
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
	Confirmed       bool     `json:"confirmed"`
	ChallengeInput  string   `json:"challenge_input"`
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

	sameSession := srcSess == tgtSess
	if !sameSession {
		if err := srcSess.UseDatabase(ctx, req.SourceDB); err != nil {
			writeError(w, http.StatusBadRequest, "source: "+err.Error())
			return
		}
		if err := tgtSess.UseDatabase(ctx, req.TargetDB); err != nil {
			writeError(w, http.StatusBadRequest, "target: "+err.Error())
			return
		}
	}
	srcCtx := driver.WithDatabase(ctx, req.SourceDB)
	tgtCtx := driver.WithDatabase(ctx, req.TargetDB)

	srcSchema, err := srcSess.Session.InspectSchema(srcCtx, req.SourceDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect source schema: "+err.Error())
		return
	}

	tgtSchema, err := tgtSess.Session.InspectSchema(tgtCtx, req.TargetDB)
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

	isRemote := s.IsRemoteRequest(r)
	var parsedStmts []string
	var needsChallenge bool
	var challengeReason string
	for _, rawStmt := range req.Statements {
		for _, stmt := range driver.SplitSQLStatements(rawStmt, tgtSess.DriverName) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			parsedStmts = append(parsedStmts, stmt)
			insp := safety.InspectWithScope(stmt, tgtSess.Environment, req.TargetDB)
			if isRemote && isRemoteBlockedHostFileOp(insp) {
				writeError(w, http.StatusForbidden, ErrRemoteHostFileSQLBlocked.Error())
				return
			}
			if insp != nil && insp.RequiresChallenge {
				needsChallenge = true
				if challengeReason == "" {
					challengeReason = insp.Reason
				}
			}
		}
	}

	// Safety Guard & Production confirmation check
	if strings.EqualFold(tgtSess.Environment, "production") || needsChallenge {
		if !req.Confirmed {
			msg := "Target is a Production database. Confirmation challenge required."
			if challengeReason != "" && !strings.EqualFold(tgtSess.Environment, "production") {
				msg = challengeReason
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"blocked":          true,
				"requires_confirm": true,
				"message":          msg,
			})
			return
		}
		expectedChallenge := req.TargetDB
		if expectedChallenge == "" {
			expectedChallenge = tgtSess.Name
		}
		if !strings.EqualFold(strings.TrimSpace(req.ChallengeInput), expectedChallenge) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Challenge confirmation failed: expected '%s', got '%s'", expectedChallenge, req.ChallengeInput))
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	tgtSess.beginInFlight()
	defer tgtSess.endInFlight()

	if err := tgtSess.UseDatabase(ctx, req.TargetDB); err != nil {
		writeError(w, http.StatusBadRequest, "target: "+err.Error())
		return
	}

	var executedCount int
	var errors []string

	for _, stmt := range parsedStmts {
		// Pre-DDL Smart Tiered Backup for destructive schema operations.
		tgtSess.BackupBeforeDestructive(ctx, stmt, req.TargetDB)

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
