package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/safety"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

var (
	ErrReadOnlySession = errors.New("Session is locked in READ-ONLY mode. Cannot execute destructive statement.")
)

// ExecuteRequest encapsulates SQL query parameters and challenge verification.
type ExecuteRequest struct {
	SQL                   string `json:"sql"`
	Confirmed             bool   `json:"confirmed"`
	ChallengeInput        string `json:"challenge_input"`
	QueryID               string `json:"query_id"`
	Database              string `json:"database"`
	Profile               bool   `json:"profile"`
	Limit                 int    `json:"limit,omitempty"`
	LockTimeoutSec        int    `json:"lock_timeout_sec,omitempty"`        // Fail-fast session lock timeout (default 3s for DDL)
	BypassMutationConfirm bool   `json:"bypass_mutation_confirm,omitempty"` // Dev-only session toggle (ADR 0057)
	TabID                 string `json:"tab_id,omitempty"`                  // Tab-scoped transaction isolation
}

// ExecuteResponse provides the unified outcome of a mutation or buffered query.
type ExecuteResponse struct {
	SQL                string                    `json:"sql,omitempty"`
	Type               string                    `json:"type"` // "exec", "query", or "batch"
	Blocked            bool                      `json:"blocked,omitempty"`
	Inspection         *safety.InspectionResult  `json:"inspection,omitempty"`
	RowsAffected       int64                     `json:"rows_affected,omitempty"`
	LastInsertID       int64                     `json:"last_insert_id,omitempty"`
	Message            string                    `json:"message,omitempty"`
	BackupFile         string                    `json:"backup_file,omitempty"`
	BackupWarning      string                    `json:"backup_warning,omitempty"`
	BackupDataIncluded bool                      `json:"backup_data_included,omitempty"`
	Columns            []driver.ColumnDefinition `json:"columns,omitempty"`
	Rows               [][]any                   `json:"rows,omitempty"`
	RowCount           int                       `json:"row_count,omitempty"`
	ExecutionMs        int64                     `json:"execution_ms"`
	Profile            *driver.QueryProfile      `json:"profile,omitempty"`
	TxActive           bool                      `json:"tx_active"`
	Results            []*ExecuteResponse        `json:"results,omitempty"`
}

// Execute processes a SQL query through the Safe Execution Pipeline:
// 1. Evaluates statement against Safety Guard AST inspection
// 2. Enforces read-only session restrictions
// 3. Enforces Challenge Modal verification and table name matching
// 4. Triggers pre-DDL Smart Tiered Backup on destructive schema operations (DROP, TRUNCATE)
// 5. Routes to mutation execution or stream execution with the Implicit Safe Limit (1,000 rows).
func (s *ActiveSession) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	s.beginInFlight()
	defer s.endInFlightTab(req.TabID)

	trimmedSQL := strings.TrimSpace(req.SQL)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

	cleanSQL := driver.StripLeadingComments(trimmedSQL)
	cleanUpper := strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
	isSelect := driver.IsQueryStatement(trimmedSQL)

	if req.Database != "" && s.Session != nil {
		if err := s.UseDatabase(ctx, req.Database); err != nil {
			return nil, err
		}
	}

	// 1. Safety Guard Inspection with Scoped Database Context
	targetDB := req.Database
	if targetDB == "" && s.Schema() != nil {
		targetDB = s.Schema().DatabaseName
	}
	if targetDB == "" {
		targetDB = s.DatabaseName
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}

	// 2. Read-Only Enforcement: blocks all destructive operations or any mutation when locked
	if s.IsReadOnly() && (!isSelect || !allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
	}

	// Switch active database only after Read-Only and Safety Guard checks pass.
	if strings.HasPrefix(cleanUpper, "USE ") {
		dbTarget := strings.TrimSpace(cleanSQL[4:])
		dbTarget = strings.Trim(strings.TrimRight(dbTarget, "; \t\r\n"), "`'\"")
		if dbTarget != "" {
			if err := s.UseDatabase(ctx, dbTarget); err != nil {
				return nil, err
			}
			return &ExecuteResponse{
				Type:    "exec",
				Message: fmt.Sprintf("Database changed to %s", dbTarget),
			}, nil
		}
	}

	// 3. Pre-Execution Row Estimation & Tiered Affirmation Gate (ADR 0057)
	isDev := strings.EqualFold(s.Environment, "Development")
	isMutation := inspection.Operation == "DELETE" || inspection.Operation == "UPDATE" || len(inspection.StatementEstimates) > 0
	canBypass := isDev && req.BypassMutationConfirm && !inspection.RequiresChallenge

	if isMutation && !canBypass {
		// Re-derived on confirmed requests too, so a client cannot skip the typed challenge by sending confirmed=true first.
		s.estimateAffectedRows(ctx, inspection)

		anyMissingWhere := false
		anyOverThreshold := false
		if len(inspection.StatementEstimates) > 0 {
			for _, est := range inspection.StatementEstimates {
				if !est.HasWhere {
					anyMissingWhere = true
				}
				if est.EstimatedRows != nil && *est.EstimatedRows > 100 {
					anyOverThreshold = true
				}
			}
		} else {
			if !inspection.HasWhere {
				anyMissingWhere = true
			}
			if inspection.EstimatedRows != nil && *inspection.EstimatedRows > 100 {
				anyOverThreshold = true
			}
		}

		if isDev {
			// Single-row PK/Unique mutation in Development: auto-allow without challenge
			if len(inspection.StatementEstimates) == 0 && inspection.HasWhere && inspection.EstimatedRows != nil && *inspection.EstimatedRows == 1 {
				inspection.RequiresAffirmation = false
				inspection.RequiresChallenge = false
			} else {
				// > 1 rows, 0 rows, unknown rows, unqualified, or multi-statement batch
				if anyOverThreshold || anyMissingWhere {
					inspection.RequiresChallenge = true
				} else {
					inspection.RequiresAffirmation = true
				}
			}
		} else {
			// Staging or Production: universal confirmation
			if anyOverThreshold || anyMissingWhere {
				inspection.RequiresChallenge = true
			} else {
				inspection.RequiresAffirmation = true
			}
		}
	}

	requiresGate := inspection.RequiresChallenge || (isMutation && !canBypass && inspection.RequiresAffirmation) || (!isDev && inspection.RequiresAffirmation)
	if requiresGate && !req.Confirmed {
		if inspection.TargetTable != "" && s.Session != nil {
			if dp, ok := s.Session.(driver.DiagnosticsProvider); ok {
				if locks, err := dp.InspectTableLocks(ctx, req.Database, inspection.TargetTable); err == nil {
					inspection.TableLocks = locks
				}
			}
			schema := s.Schema()
			if schema == nil || (req.Database != "" && schema.DatabaseName != req.Database) {
				if sSc, sErr := s.Session.InspectSchema(ctx, req.Database); sErr == nil {
					schema = sSc
					s.SetSchema(sSc)
				}
			}
			if schema != nil {
				inspection.Relationships = driver.InspectTableRelationships(schema, inspection.TargetTable)
			}
		}
		return &ExecuteResponse{
			Blocked:    true,
			Inspection: inspection,
		}, nil
	}

	// Challenge typed table name validation (required on RequiresChallenge when not a 1-click Dev affirmation)
	if inspection.RequiresChallenge && req.Confirmed && inspection.TargetTable != "" {
		expected := inspection.TargetTable
		typed := strings.TrimSpace(req.ChallengeInput)
		if typed == "" {
			// Confirmed without a typed challenge (the requirement escalated since the preview, e.g. the
			// row estimate grew past the threshold): ask again instead of failing the request.
			return &ExecuteResponse{Blocked: true, Inspection: inspection}, nil
		}
		if !strings.EqualFold(typed, expected) && !strings.EqualFold(typed, inspection.TargetDatabase) {
			return nil, fmt.Errorf("Challenge confirmation failed: expected '%s', got '%s'", expected, req.ChallengeInput)
		}
	}

	// 4. Pre-DDL Automated Backup
	var backupFile string
	var backupDataIncluded bool
	var backupWarning string
	tt := strings.ToUpper(inspection.TargetType)
	if (inspection.Operation == "DROP" || inspection.Operation == "TRUNCATE") && (tt == "" || tt == "TABLE") {
		if inspection.TargetTable != "" && s.backupDir != "" && s.Session != nil {
			// BackupTable reads the table from the session's active database. A qualified target in
			// another database (DROP TABLE archive.orders) is not snapshotted at all rather than
			// snapshotting a same-named table here, or switching the shared session's database
			// under concurrent requests.
			if inspection.TargetDatabase != "" && targetDB != "" && !strings.EqualFold(inspection.TargetDatabase, targetDB) {
				backupWarning = fmt.Sprintf("Pre-execution backup skipped: '%s.%s' is outside the active database '%s'; run it from that database to get an automatic snapshot.",
					inspection.TargetDatabase, inspection.TargetTable, targetDB)
			} else {
				var err error
				backupFile, backupDataIncluded, err = safety.BackupTable(ctx, s.Session, s.backupDir, inspection.TargetTable, s.DriverName)
				if err != nil {
					backupWarning = "Pre-execution backup incomplete: " + err.Error()
				}
			}
			if backupWarning != "" {
				slog.Warn("pre_ddl_backup_failed", "module", "safety", "table", inspection.TargetTable, "warning", backupWarning)
			}
		}
	}

	start := time.Now()

	queryID := req.QueryID
	if queryID == "" {
		queryID = "active"
	}

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Each execution gets a unique run key: clients reuse query IDs (the editor always sends
	// "active"), and a shared key let the first query to finish unregister a still-running one.
	runKey := s.registerQuery(queryID, cancel)
	execCtx = driver.WithQueryID(execCtx, runKey)

	defer func() {
		s.mu.Lock()
		delete(s.runningQueries, runKey)
		s.mu.Unlock()
		if execCtx.Err() != nil && s.Session != nil {
			// ponytail: Actively cancel runaway query on DB server to prevent CPU/IO exhaustion
			cancelCtx, cCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.Session.CancelQuery(cancelCtx, runKey)
			cCancel()
		}
	}()

	var execErr error
	var rowsAffected int64
	defer func() {
		if s.vault != nil && (inspection == nil || !inspection.RequiresChallenge || req.Confirmed) {
			dur := time.Since(start).Milliseconds()
			s.RecordAudit(trimmedSQL, "editor", dur, rowsAffected, execErr, start)
		}
	}()

	// Enable profiling if requested
	if req.Profile && s.Session != nil && s.DriverName == "mysql" {
		_, _ = s.Session.Exec(execCtx, "SET profiling = 1")
	}

	collectProfile := func() *driver.QueryProfile {
		if !req.Profile || s.Session == nil {
			return nil
		}
		if s.DriverName == "mysql" {
			defer func() { _, _ = s.Session.Exec(execCtx, "SET profiling = 0") }()
			pStream, pErr := s.Session.QueryStream(execCtx, "SHOW PROFILE", 100)
			if pErr != nil {
				return nil
			}
			defer pStream.Close()
			var stages []driver.ProfileStage
			var totalDuration float64
			for {
				chunk, err := pStream.NextChunk(100)
				for _, row := range chunk {
					if len(row) >= 2 {
						stg := fmt.Sprintf("%v", row[0])
						durSec, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[1]), 64)
						durUs := int64(durSec * 1e6)
						totalDuration += durSec
						stages = append(stages, driver.ProfileStage{
							Stage:      stg,
							DurationUs: durUs,
						})
					}
				}
				if err != nil || len(chunk) == 0 {
					break
				}
			}
			if totalDuration > 0 {
				for i := range stages {
					stages[i].Percentage = (float64(stages[i].DurationUs) / (totalDuration * 1e6)) * 100.0
				}
			}
			return &driver.QueryProfile{
				DurationMs: totalDuration * 1000.0,
				Stages:     stages,
			}
		}

		// SQLite profile fallback
		durUs := time.Since(start).Microseconds()
		return &driver.QueryProfile{
			DurationMs: float64(durUs) / 1000.0,
			Stages: []driver.ProfileStage{
				{
					Stage:      "Query Execution (SQLite embedded engine)",
					DurationUs: durUs,
					Percentage: 100.0,
				},
			},
		}
	}

	// Intercept explicit transaction statements to ensure connection pinning and prevent orphaned locks
	cleanUpper = strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
	isTxBegin := cleanUpper == "BEGIN" || cleanUpper == "START TRANSACTION" || cleanUpper == "BEGIN WORK" || cleanUpper == "BEGIN TRANSACTION" ||
		strings.HasPrefix(cleanUpper, "START TRANSACTION ") ||
		strings.HasPrefix(cleanUpper, "BEGIN TRANSACTION ") ||
		strings.HasPrefix(cleanUpper, "BEGIN WORK ") ||
		(strings.HasPrefix(cleanUpper, "BEGIN ") && (strings.Contains(cleanUpper, "DEFERRED") || strings.Contains(cleanUpper, "IMMEDIATE") || strings.Contains(cleanUpper, "EXCLUSIVE") || strings.Contains(cleanUpper, "READ ONLY") || strings.Contains(cleanUpper, "READ WRITE") || strings.Contains(cleanUpper, "ISOLATION LEVEL")))
	isTxCommit := cleanUpper == "COMMIT" || cleanUpper == "COMMIT WORK" || cleanUpper == "COMMIT TRANSACTION" || cleanUpper == "END" || cleanUpper == "END WORK" || cleanUpper == "END TRANSACTION"
	isTxRollback := cleanUpper == "ROLLBACK" || cleanUpper == "ROLLBACK WORK" || cleanUpper == "ROLLBACK TRANSACTION" || cleanUpper == "ABORT" || cleanUpper == "ABORT WORK" || cleanUpper == "ABORT TRANSACTION"

	if isTxBegin {
		tx, _, _ := s.getTxStatus(req.TabID)
		if tx != nil {
			return nil, fmt.Errorf("a transaction is already active in this tab; please COMMIT or ROLLBACK first")
		}
		// Clear any previous aborted state upon starting a fresh transaction
		s.clearTx(req.TabID)

		// ponytail: Use non-cancelled background context so transaction persists across subsequent HTTP requests
		newTx, err := s.Session.BeginTx(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		s.setTx(req.TabID, newTx)

		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction started. All subsequent queries will run atomically on this dedicated connection.",
			ExecutionMs: time.Since(start).Milliseconds(),
			TxActive:    true,
		}, nil
	}

	if isTxCommit {
		tx, aborted, reason := s.getTxStatus(req.TabID)
		if aborted {
			s.clearTx(req.TabID)
			return nil, fmt.Errorf("transaction in this tab was aborted: %s; COMMIT cannot be applied", reason)
		}

		if tx == nil {
			_, err := s.Session.Exec(execCtx, trimmedSQL)
			if err != nil {
				return nil, err
			}
			return &ExecuteResponse{
				Type:        "exec",
				Message:     "COMMIT executed.",
				ExecutionMs: time.Since(start).Milliseconds(),
				TxActive:    false,
			}, nil
		}

		s.clearTx(req.TabID)
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction committed successfully.",
			ExecutionMs: time.Since(start).Milliseconds(),
			TxActive:    false,
		}, nil
	}

	if isTxRollback {
		tx, aborted, reason := s.getTxStatus(req.TabID)
		s.clearTx(req.TabID)

		if aborted {
			return &ExecuteResponse{
				Type:        "exec",
				Message:     fmt.Sprintf("Transaction was already rolled back: %s. Tab transaction state is now cleared.", reason),
				ExecutionMs: time.Since(start).Milliseconds(),
				TxActive:    false,
			}, nil
		}

		if tx == nil {
			_, err := s.Session.Exec(execCtx, trimmedSQL)
			if err != nil {
				return nil, err
			}
			return &ExecuteResponse{
				Type:        "exec",
				Message:     "ROLLBACK executed.",
				ExecutionMs: time.Since(start).Milliseconds(),
				TxActive:    false,
			}, nil
		}

		if err := tx.Rollback(); err != nil {
			return nil, fmt.Errorf("failed to rollback transaction: %w", err)
		}
		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction rolled back successfully.",
			ExecutionMs: time.Since(start).Milliseconds(),
			TxActive:    false,
		}, nil
	}

	// 5. Route to Exec vs QueryStream
	// If transaction was aborted by timeout reaper, block all subsequent queries until user runs ROLLBACK
	currentTx, aborted, reason := s.getTxStatus(req.TabID)
	if aborted {
		return nil, fmt.Errorf("transaction in this tab was aborted: %s. You must issue ROLLBACK before running new statements", reason)
	}

	stmts := splitSQLStatements(strings.NewReader(trimmedSQL), s.DriverName)
	if len(stmts) > 1 {
		return s.executeMulti(execCtx, req, stmts, inspection, targetDB, start, currentTx, collectProfile)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	} else if limit > 100000 {
		limit = 100000
	}

	if currentTx != nil {
		if !isSelect {
			execRes, err := currentTx.Exec(execCtx, trimmedSQL)
			if err != nil {
				execErr = err
				return nil, err
			}
			rowsAffected = execRes.RowsAffected
			msg := execRes.Message + " (in transaction)"
			if isMutation && inspection.EstimatedRows != nil && *inspection.EstimatedRows != rowsAffected {
				msg = fmt.Sprintf("%s (Estimated: %d rows, Actual: %d rows)", msg, *inspection.EstimatedRows, rowsAffected)
			}
			return &ExecuteResponse{
				Type:               "exec",
				RowsAffected:       execRes.RowsAffected,
				LastInsertID:       execRes.LastInsertID,
				ExecutionMs:        execRes.ExecutionMs,
				Message:            msg,
				BackupFile:         backupFile,
				BackupDataIncluded: backupDataIncluded,
				BackupWarning:      backupWarning,
				Profile:            collectProfile(),
				TxActive:           true,
				Inspection:         inspection,
			}, nil
		}

		stream, err := currentTx.QueryStream(execCtx, trimmedSQL, limit)
		if err != nil {
			execErr = err
			return nil, err
		}
		defer stream.Close()

		cols := stream.Columns()
		var allRows [][]any
		for len(allRows) < limit {
			chunk, err := stream.NextChunk(200)
			if len(chunk) > 0 {
				allRows = append(allRows, chunk...)
			}
			if err != nil {
				if err != io.EOF {
					execErr = err
					return nil, err
				}
				break
			}
		}

		rowsAffected = int64(len(allRows))
		return &ExecuteResponse{
			Type:        "query",
			Columns:     cols,
			Rows:        driver.FormatRowsForJSON(allRows),
			RowCount:    len(allRows),
			ExecutionMs: time.Since(start).Milliseconds(),
			Profile:     collectProfile(),
			TxActive:    currentTx != nil,
		}, nil
	}

	if !isSelect {
		// Mutation execution
		var execRes *driver.ExecResult
		var err error
		if inspection.Operation == "ALTER" {
			execRes, err = execSafeDDLOrFallback(execCtx, s.Session, trimmedSQL, req.LockTimeoutSec)
		} else {
			execRes, err = s.Session.Exec(execCtx, trimmedSQL)
		}
		if err != nil {
			execErr = err
			return nil, err
		}
		rowsAffected = execRes.RowsAffected
		msg := execRes.Message
		if isMutation && inspection.EstimatedRows != nil && *inspection.EstimatedRows != rowsAffected {
			msg = fmt.Sprintf("%s (Estimated: %d rows, Actual: %d rows)", msg, *inspection.EstimatedRows, rowsAffected)
		}
		return &ExecuteResponse{
			Type:               "exec",
			RowsAffected:       execRes.RowsAffected,
			LastInsertID:       execRes.LastInsertID,
			ExecutionMs:        execRes.ExecutionMs,
			Message:            msg,
			BackupFile:         backupFile,
			BackupDataIncluded: backupDataIncluded,
			BackupWarning:      backupWarning,
			Profile:            collectProfile(),
			TxActive:           currentTx != nil,
			Inspection:         inspection,
		}, nil
	}

	// Select stream execution with implicit safe limit.
	// Enforce DB-engine read-only mode when session is locked in Read-Only mode.
	var stream driver.RowStream
	if s.IsReadOnly() {
		if roSess, ok := s.Session.(driver.ReadOnlySession); ok {
			stream, err = roSess.QueryStreamReadOnly(execCtx, trimmedSQL, limit)
		} else {
			stream, err = s.Session.QueryStream(execCtx, trimmedSQL, limit)
		}
	} else {
		stream, err = s.Session.QueryStream(execCtx, trimmedSQL, limit)
	}
	if err != nil {
		execErr = err
		return nil, err
	}
	defer stream.Close()

	cols := stream.Columns()
	var allRows [][]any

	for len(allRows) < limit {
		chunk, err := stream.NextChunk(200)
		if len(chunk) > 0 {
			allRows = append(allRows, chunk...)
		}
		if err != nil {
			if err != io.EOF {
				execErr = err
				return nil, err
			}
			break
		}
	}

	rowsAffected = int64(len(allRows))
	return &ExecuteResponse{
		Type:        "query",
		Columns:     cols,
		Rows:        driver.FormatRowsForJSON(allRows),
		RowCount:    len(allRows),
		ExecutionMs: time.Since(start).Milliseconds(),
		Profile:     collectProfile(),
		TxActive:    currentTx != nil,
	}, nil
}

// ExecuteStream streams query results line-by-line as NDJSON (ADR 0058).
// It delivers an immediate micro-chunk (20 rows) for sub-50ms TTFR, followed by batched chunks.

func (s *ActiveSession) Stream(ctx context.Context, sql string, maxRows int) (driver.RowStream, error) {
	s.mu.Lock()
	s.LastActive = time.Now()
	s.mu.Unlock()

	trimmedSQL := strings.TrimSpace(sql)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

	isSelect := driver.IsQueryStatement(trimmedSQL)
	targetDB := ""
	if s.Schema() != nil {
		targetDB = s.Schema().DatabaseName
	}
	if targetDB == "" {
		targetDB = s.DatabaseName
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}
	if s.IsReadOnly() && (!isSelect || !allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
	}
	if inspection.IsDestructive || inspection.RequiresChallenge {
		return nil, fmt.Errorf("destructive query cannot be streamed: %s; use query editor with confirmation", inspection.Reason)
	}
	if !isSelect || !allQueries {
		return nil, errors.New("only query statements (SELECT, SHOW, DESCRIBE, EXPLAIN) can be streamed")
	}

	if maxRows <= 0 {
		maxRows = 100000
	}

	return s.Session.QueryStream(ctx, trimmedSQL, maxRows)
}

// challenge is rejected, since one typed confirmation and one pre-DDL backup can only cover one target.
