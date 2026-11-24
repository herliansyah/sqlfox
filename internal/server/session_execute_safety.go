package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/safety"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

func inspectAllStatements(sql, environment, driverName, scopedDB string) (*safety.InspectionResult, bool, error) {
	stmts := splitSQLStatements(strings.NewReader(sql), driverName)
	if len(stmts) <= 1 {
		return safety.InspectWithScope(sql, environment, scopedDB), driver.IsQueryStatement(sql), nil
	}
	var gate, firstDestructive *safety.InspectionResult
	var stmtEstimates []safety.StatementEstimate
	allQueries := true
	for idx, stmt := range stmts {
		if !driver.IsQueryStatement(stmt) {
			allQueries = false
		}
		insp := safety.InspectWithScope(stmt, environment, scopedDB)
		if insp.Operation == "DELETE" || insp.Operation == "UPDATE" {
			stmtEstimates = append(stmtEstimates, safety.StatementEstimate{
				StatementIndex:    idx + 1,
				SQL:               stmt,
				Operation:         insp.Operation,
				TargetTable:       insp.TargetTable,
				TargetDatabase:    insp.TargetDatabase,
				HasWhere:          insp.HasWhere,
				CountQuery:        insp.CountQuery,
				RequiresChallenge: insp.RequiresChallenge,
				Reason:            insp.Reason,
			})
		}
		if insp.RequiresChallenge {
			if gate != nil && gate.RequiresChallenge && (gate.Operation == "DROP" || insp.Operation == "DROP" || gate.Operation == "TRUNCATE" || insp.Operation == "TRUNCATE") {
				return nil, false, errors.New("multiple destructive statements require confirmation: run them one at a time")
			}
			gate = insp
		} else if insp.IsDestructive && firstDestructive == nil {
			firstDestructive = insp
		} else if insp.RequiresAffirmation && gate == nil {
			gate = insp
		}
	}
	if gate == nil {
		gate = firstDestructive
	}
	if gate == nil && len(stmtEstimates) > 0 {
		gate = &safety.InspectionResult{
			RiskLevel:           safety.RiskWarning,
			IsDestructive:       true,
			Operation:           "MUTATION_BATCH",
			RequiresAffirmation: true,
			Reason:              fmt.Sprintf("Batch contains %d mutation statements.", len(stmtEstimates)),
		}
	}
	if gate == nil {
		gate = &safety.InspectionResult{RiskLevel: safety.RiskNone}
	}
	if len(stmtEstimates) > 0 {
		gate.StatementEstimates = stmtEstimates
	}
	return gate, allQueries, nil
}

// ScriptBlocker summarizes a destructive statement in a batch script that the Safety Guard requires
// the operator to confirm before the script runner will execute it.
type ScriptBlocker struct {
	StatementIndex int    `json:"statement_index"`
	Operation      string `json:"operation"`
	TargetTable    string `json:"target_table,omitempty"`
	TargetDatabase string `json:"target_database,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// InspectScriptStatements runs the Safety Guard over each statement of a script and returns the set
// that demands a typed Challenge (CRITICAL destructive ops or a schema-scope mismatch). An empty
// result means the script is safe to run without confirmation. This mirrors the editor's gate so the
// Large SQL Runner cannot bypass the Safe Execution Pipeline.
func (s *ActiveSession) InspectScriptStatements(statements []string, scopedDB string) []ScriptBlocker {
	var blockers []ScriptBlocker
	for i, stmt := range statements {
		insp := safety.InspectWithScope(stmt, s.Environment, scopedDB)
		if insp == nil {
			continue
		}
		if insp.RequiresChallenge {
			blockers = append(blockers, ScriptBlocker{
				StatementIndex: i + 1,
				Operation:      insp.Operation,
				TargetTable:    insp.TargetTable,
				TargetDatabase: insp.TargetDatabase,
				Reason:         insp.Reason,
			})
		}
	}
	return blockers
}

// BackupBeforeDestructive triggers a pre-DDL Smart Tiered Backup for a single DROP/TRUNCATE TABLE
// statement, matching the editor's Execute pipeline. It is best-effort: failures are logged, not
// fatal, so the script run proceeds as the operator confirmed. A qualified target outside the active
// database is skipped rather than snapshotting a same-named table in the wrong database.
func (s *ActiveSession) BackupBeforeDestructive(ctx context.Context, stmt, scopedDB string) {
	if s.backupDir == "" || s.Session == nil {
		return
	}
	insp := safety.InspectWithScope(stmt, s.Environment, scopedDB)
	if insp == nil {
		return
	}
	tt := strings.ToUpper(insp.TargetType)
	if (insp.Operation != "DROP" && insp.Operation != "TRUNCATE") || (tt != "" && tt != "TABLE") {
		return
	}
	if insp.TargetTable == "" {
		return
	}
	if insp.TargetDatabase != "" && scopedDB != "" && !strings.EqualFold(insp.TargetDatabase, scopedDB) {
		slog.Warn("pre_ddl_backup_skipped", "module", "safety", "table", insp.TargetTable,
			"reason", "target outside active database", "target_db", insp.TargetDatabase, "active_db", scopedDB)
		return
	}
	if _, _, err := safety.BackupTable(ctx, s.Session, s.backupDir, insp.TargetTable, s.DriverName); err != nil {
		slog.Warn("pre_ddl_backup_failed", "module", "safety", "table", insp.TargetTable, "warning", err.Error())
	}
}

func (s *ActiveSession) estimateAffectedRows(ctx context.Context, insp *safety.InspectionResult) {
	if insp == nil || s.Session == nil {
		return
	}

	getCachedRowCount := func(tableName string) (int64, bool) {
		sc := s.Schema()
		if sc == nil || tableName == "" {
			return 0, false
		}
		cleanName := strings.Trim(tableName, "`'\"[]")
		for _, tbl := range sc.Tables {
			if strings.EqualFold(tbl.Name, cleanName) && tbl.RowCount > 0 {
				return tbl.RowCount, true
			}
		}
		return 0, false
	}

	if len(insp.StatementEstimates) > 0 {
		var total int64
		allExact := true
		for i := range insp.StatementEstimates {
			est := &insp.StatementEstimates[i]
			// Blast-radius optimization: If this is an unqualified mutation, check cached catalog row count first
			if !est.HasWhere && est.TargetTable != "" {
				if rc, ok := getCachedRowCount(est.TargetTable); ok {
					est.EstimatedRows = &rc
					est.EstimatedIsExact = false
					total += rc
					allExact = false
					continue
				}
			}
			if est.CountQuery != "" {
				count, exact := s.runSingleCountEstimate(ctx, est.CountQuery)
				if count != nil {
					est.EstimatedRows = count
					est.EstimatedIsExact = exact
					total += *count
					if !exact {
						allExact = false
					}
				}
			}
		}
		insp.EstimatedRows = &total
		insp.EstimatedIsExact = allExact
		return
	}

	// Blast-radius optimization: If this is an unqualified mutation (no WHERE),
	// check if the catalog already knows the approximate table row count.
	// This avoids triggering an expensive full-table scan on large production tables!
	if !insp.HasWhere && insp.TargetTable != "" {
		if rc, ok := getCachedRowCount(insp.TargetTable); ok {
			insp.EstimatedRows = &rc
			insp.EstimatedIsExact = false
			return
		}
	}

	if insp.CountQuery != "" {
		count, exact := s.runSingleCountEstimate(ctx, insp.CountQuery)
		if count != nil {
			insp.EstimatedRows = count
			insp.EstimatedIsExact = exact
		}
	}
}

func (s *ActiveSession) runSingleCountEstimate(ctx context.Context, countSQL string) (*int64, bool) {
	if s.Session == nil || countSQL == "" {
		return nil, false
	}
	countCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// No SelectDatabase here: it would persist the session's active DB before the user confirms.
	// Qualified statements yield qualified count queries; unqualified ones already run in req.Database.

	rows, err := s.Session.QueryRaw(countCtx, countSQL)
	if err == nil && len(rows) > 0 && len(rows[0]) > 0 {
		if cnt, ok := parseCountValue(rows[0][0]); ok {
			return &cnt, true
		}
	}

	// Fallback to EXPLAIN
	plan, expErr := s.Session.Explain(countCtx, countSQL)
	if expErr == nil && plan != nil && len(plan.Rows) > 0 {
		if plan.Rows[0].Rows > 0 {
			est := plan.Rows[0].Rows
			return &est, false
		}
	}

	return nil, false
}

func parseCountValue(v any) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case int32:
		return int64(val), true
	case uint64:
		return int64(val), true
	case float64:
		return int64(val), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		return n, err == nil
	case []byte:
		n, err := strconv.ParseInt(strings.TrimSpace(string(val)), 10, 64)
		return n, err == nil
	}
	return 0, false
}
