package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/safety"
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
	defer s.endInFlight()

	trimmedSQL := strings.TrimSpace(req.SQL)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

	cleanSQL := driver.StripLeadingComments(trimmedSQL)
	cleanUpper := strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
	isSelect := driver.IsQueryStatement(trimmedSQL)

	if strings.HasPrefix(cleanUpper, "USE ") {
		dbTarget := strings.TrimSpace(cleanSQL[4:])
		dbTarget = strings.Trim(strings.TrimRight(dbTarget, "; \t\r\n"), "`'\"")
		if dbTarget != "" {
			_ = s.UseDatabase(ctx, dbTarget)
		}
	}

	if req.Database != "" && s.Session != nil {
		if err := s.Session.SelectDatabase(ctx, req.Database); err != nil {
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
			Rows:        allRows,
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

	// Select stream execution with implicit safe limit
	stream, err := s.Session.QueryStream(execCtx, trimmedSQL, limit)
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
		Rows:        allRows,
		RowCount:    len(allRows),
		ExecutionMs: time.Since(start).Milliseconds(),
		Profile:     collectProfile(),
		TxActive:    currentTx != nil,
	}, nil
}

// ExecuteStream streams query results line-by-line as NDJSON (ADR 0058).
// It delivers an immediate micro-chunk (20 rows) for sub-50ms TTFR, followed by batched chunks.
func (s *ActiveSession) ExecuteStream(ctx context.Context, req ExecuteRequest, w io.Writer, flusher http.Flusher) (*ExecuteResponse, error) {
	s.beginInFlight()
	defer s.endInFlight()

	trimmedSQL := strings.TrimSpace(req.SQL)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

	cleanSQL := driver.StripLeadingComments(trimmedSQL)
	cleanUpper := strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
	isSelect := driver.IsQueryStatement(trimmedSQL)
	allStmtsQuery := isSelect
	if isSelect {
		for _, st := range driver.SplitSQLStatements(trimmedSQL, s.DriverName) {
			if !driver.IsQueryStatement(st) {
				allStmtsQuery = false
				break
			}
		}
	}

	// Non-SELECT queries (mutations/DDL/transactions) and mixed batches such as "SELECT 1; DROP TABLE t"
	// run synchronously through Execute (which owns safety inspection, row estimation, and pre-DDL backup)
	// and emit a single NDJSON line, avoiding duplicate estimation/inspection work.
	if !isSelect || !allStmtsQuery {
		resp, execErr := s.Execute(ctx, req)
		if execErr != nil {
			return nil, execErr
		}
		if resp != nil && resp.Blocked {
			return resp, nil
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(resp)
		if flusher != nil {
			flusher.Flush()
		}
		return nil, nil
	}

	if strings.HasPrefix(cleanUpper, "USE ") {
		dbTarget := strings.TrimSpace(cleanSQL[4:])
		dbTarget = strings.Trim(strings.TrimRight(dbTarget, "; \t\r\n"), "`'\"")
		if dbTarget != "" {
			_ = s.UseDatabase(ctx, dbTarget)
		}
	}

	if req.Database != "" && s.Session != nil {
		if err := s.Session.SelectDatabase(ctx, req.Database); err != nil {
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

	// 2. Read-Only Enforcement
	if s.IsReadOnly() && (!isSelect || !allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
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
			if len(inspection.StatementEstimates) == 0 && inspection.HasWhere && inspection.EstimatedRows != nil && *inspection.EstimatedRows == 1 {
				inspection.RequiresAffirmation = false
				inspection.RequiresChallenge = false
			} else {
				if anyOverThreshold || anyMissingWhere {
					inspection.RequiresChallenge = true
				} else {
					inspection.RequiresAffirmation = true
				}
			}
		} else {
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

	// Challenge typed table name validation
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

	// Non-SELECT queries (mutations/DDL/transactions) and mixed batches such as "SELECT 1; DROP TABLE t"
	// run synchronously through Execute (which owns the pre-DDL backup) and output a single NDJSON line.
	if !isSelect || !allQueries {
		resp, execErr := s.Execute(ctx, req)
		if execErr != nil {
			return nil, execErr
		}
		if resp != nil && resp.Blocked {
			return resp, nil
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(resp)
		if flusher != nil {
			flusher.Flush()
		}
		return nil, nil
	}

	start := time.Now()
	queryID := req.QueryID
	if queryID == "" {
		queryID = "active"
	}

	totalRows := 0
	var streamErr error
	defer func() {
		dur := time.Since(start).Milliseconds()
		if s.vault != nil {
			s.RecordAudit(trimmedSQL, "editor_stream", dur, int64(totalRows), streamErr, start)
		}
	}()

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
			cancelCtx, cCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.Session.CancelQuery(cancelCtx, runKey)
			cCancel()
		}
	}()

	limit := req.Limit
	if limit <= 0 {
		limit = 5000
	}

	if req.Profile && s.Session != nil && s.DriverName == "mysql" {
		_, _ = s.Session.Exec(execCtx, "SET profiling = 1")
	}

	currentTx, streamAborted, streamReason := s.getTxStatus(req.TabID)
	if streamAborted {
		return nil, fmt.Errorf("transaction in this tab was aborted: %s. You must issue ROLLBACK before running new statements", streamReason)
	}

	stmts := splitSQLStatements(strings.NewReader(trimmedSQL), s.DriverName)
	if len(stmts) > 1 {
		return s.executeStreamMulti(execCtx, req, stmts, inspection, targetDB, start, currentTx, w, flusher, runKey)
	}

	// Fetch one row past the limit so has_more is exact (a result of exactly `limit` rows is complete).
	var stream driver.RowStream
	if currentTx != nil {
		stream, streamErr = currentTx.QueryStream(execCtx, trimmedSQL, limit+1)
	} else {
		if s.Session == nil {
			return nil, errors.New("no active database session")
		}
		stream, streamErr = s.Session.QueryStream(execCtx, trimmedSQL, limit+1)
	}
	if streamErr != nil {
		return nil, streamErr
	}
	defer stream.Close()

	enc := json.NewEncoder(w)

	// 1. Meta Line (columns, target table, database, tx status)
	meta := map[string]any{
		"type":         "meta",
		"columns":      stream.Columns(),
		"tx_active":    currentTx != nil,
		"target_table": inspection.TargetTable,
		"database":     targetDB,
	}
	if err := enc.Encode(meta); err != nil {
		streamErr = err
		return nil, err
	}
	if flusher != nil {
		flusher.Flush()
	}

	// 2. Micro-Chunk: first 20 rows flushed immediately (<15ms)
	firstChunk, err := stream.NextChunk(min(20, limit))
	if len(firstChunk) > 0 {
		totalRows += len(firstChunk)
		if encErr := enc.Encode(map[string]any{
			"type": "chunk",
			"rows": firstChunk,
		}); encErr != nil {
			streamErr = encErr
			return nil, encErr
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err != nil && err != io.EOF {
		streamErr = err
		_ = enc.Encode(map[string]any{
			"type":  "error",
			"error": err.Error(),
		})
		if flusher != nil {
			flusher.Flush()
		}
		return nil, err
	}

	// 3. Subsequent chunks: batched at 200 rows
	for totalRows < limit && err == nil {
		batchSize := 200
		if totalRows+batchSize > limit {
			batchSize = limit - totalRows
		}
		var chunk [][]any
		chunk, err = stream.NextChunk(batchSize)
		if len(chunk) > 0 {
			totalRows += len(chunk)
			if encErr := enc.Encode(map[string]any{
				"type": "chunk",
				"rows": chunk,
			}); encErr != nil {
				streamErr = encErr
				return nil, encErr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}

	if err != nil && err != io.EOF {
		streamErr = err
		_ = enc.Encode(map[string]any{
			"type":  "error",
			"error": err.Error(),
		})
		if flusher != nil {
			flusher.Flush()
		}
		return nil, err
	}

	hasMore := false
	if err == nil && totalRows >= limit {
		extra, _ := stream.NextChunk(1)
		hasMore = len(extra) > 0
	}

	// 4. Final Line: done
	dur := time.Since(start).Milliseconds()
	var prof *driver.QueryProfile
	if req.Profile && s.Session != nil && s.DriverName == "mysql" {
		defer func() { _, _ = s.Session.Exec(execCtx, "SET profiling = 0") }()
		pStream, pErr := s.Session.QueryStream(execCtx, "SHOW PROFILE", 100)
		if pErr == nil && pStream != nil {
			defer pStream.Close()
			var stages []driver.ProfileStage
			var totalDuration float64
			for {
				pChunk, pChunkErr := pStream.NextChunk(100)
				for _, pRow := range pChunk {
					if len(pRow) >= 2 {
						stg := fmt.Sprintf("%v", pRow[0])
						durSec, _ := strconv.ParseFloat(fmt.Sprintf("%v", pRow[1]), 64)
						durUs := int64(durSec * 1e6)
						totalDuration += durSec
						stages = append(stages, driver.ProfileStage{
							Stage:      stg,
							DurationUs: durUs,
						})
					}
				}
				if pChunkErr != nil || len(pChunk) == 0 {
					break
				}
			}
			if totalDuration > 0 {
				for i := range stages {
					stages[i].Percentage = (float64(stages[i].DurationUs) / (totalDuration * 1e6)) * 100.0
				}
			}
			if len(stages) > 0 {
				prof = &driver.QueryProfile{
					DurationMs: totalDuration * 1000.0,
					Stages:     stages,
				}
			}
		}
	}

	donePayload := map[string]any{
		"type":         "done",
		"row_count":    totalRows,
		"execution_ms": dur,
		"has_more":     hasMore,
		"tx_active":    s.hasActiveTx(req.TabID),
	}
	if prof != nil {
		donePayload["profile"] = prof
	}
	if err := enc.Encode(donePayload); err != nil {
		streamErr = err
		return nil, err
	}
	if flusher != nil {
		flusher.Flush()
	}

	return nil, nil
}

func previewStmt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

func (s *ActiveSession) executeMulti(
	ctx context.Context,
	req ExecuteRequest,
	stmts []string,
	inspection *safety.InspectionResult,
	targetDB string,
	start time.Time,
	currentTx driver.TxSession,
	collectProfile func() *driver.QueryProfile,
) (*ExecuteResponse, error) {
	var results []*ExecuteResponse
	var totalRowsAffected int64
	var firstQuery *ExecuteResponse
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}

	for i, stmt := range stmts {
		stmtTrimmed := strings.TrimSpace(stmt)
		if stmtTrimmed == "" {
			continue
		}
		stmtStart := time.Now()
		cleanSQL := driver.StripLeadingComments(stmtTrimmed)
		cleanUpper := strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
		isStmtSelect := driver.IsQueryStatement(stmtTrimmed)

		if strings.HasPrefix(cleanUpper, "USE ") {
			dbTarget := strings.TrimSpace(cleanSQL[4:])
			dbTarget = strings.Trim(strings.TrimRight(dbTarget, "; \t\r\n"), "`'\"")
			if dbTarget != "" {
				_ = s.UseDatabase(ctx, dbTarget)
			}
		}

		isTxBegin := cleanUpper == "BEGIN" || cleanUpper == "START TRANSACTION" || cleanUpper == "BEGIN WORK" || cleanUpper == "BEGIN TRANSACTION" ||
			strings.HasPrefix(cleanUpper, "START TRANSACTION ") ||
			strings.HasPrefix(cleanUpper, "BEGIN TRANSACTION ") ||
			strings.HasPrefix(cleanUpper, "BEGIN WORK ") ||
			(strings.HasPrefix(cleanUpper, "BEGIN ") && (strings.Contains(cleanUpper, "DEFERRED") || strings.Contains(cleanUpper, "IMMEDIATE") || strings.Contains(cleanUpper, "EXCLUSIVE") || strings.Contains(cleanUpper, "READ ONLY") || strings.Contains(cleanUpper, "READ WRITE") || strings.Contains(cleanUpper, "ISOLATION LEVEL")))
		isTxCommit := cleanUpper == "COMMIT" || cleanUpper == "COMMIT WORK" || cleanUpper == "COMMIT TRANSACTION" || cleanUpper == "END" || cleanUpper == "END WORK" || cleanUpper == "END TRANSACTION"
		isTxRollback := cleanUpper == "ROLLBACK" || cleanUpper == "ROLLBACK WORK" || cleanUpper == "ROLLBACK TRANSACTION" || cleanUpper == "ABORT" || cleanUpper == "ABORT WORK" || cleanUpper == "ABORT TRANSACTION"

		if isTxBegin {
			if s.hasActiveTx(req.TabID) {
				return nil, fmt.Errorf("a transaction is already active in this tab; please COMMIT or ROLLBACK first")
			}
			tx, err := s.Session.BeginTx(context.Background())
			if err != nil {
				return nil, fmt.Errorf("failed to begin transaction: %w", err)
			}
			s.setTx(req.TabID, tx)
			currentTx = tx
			res := &ExecuteResponse{
				SQL:         stmtTrimmed,
				Type:        "exec",
				Message:     "Transaction started.",
				ExecutionMs: time.Since(stmtStart).Milliseconds(),
				TxActive:    true,
			}
			results = append(results, res)
			continue
		}

		if isTxCommit {
			tx := s.clearTx(req.TabID)
			currentTx = nil
			if tx == nil {
				_, err := s.Session.Exec(ctx, stmtTrimmed)
				if err != nil {
					return nil, err
				}
				results = append(results, &ExecuteResponse{
					SQL:         stmtTrimmed,
					Type:        "exec",
					Message:     "COMMIT executed.",
					ExecutionMs: time.Since(stmtStart).Milliseconds(),
					TxActive:    false,
				})
			} else {
				if err := tx.Commit(); err != nil {
					return nil, fmt.Errorf("failed to commit transaction: %w", err)
				}
				results = append(results, &ExecuteResponse{
					SQL:         stmtTrimmed,
					Type:        "exec",
					Message:     "Transaction committed successfully.",
					ExecutionMs: time.Since(stmtStart).Milliseconds(),
					TxActive:    false,
				})
			}
			continue
		}

		if isTxRollback {
			tx := s.clearTx(req.TabID)
			currentTx = nil
			if tx == nil {
				_, err := s.Session.Exec(ctx, stmtTrimmed)
				if err != nil {
					return nil, err
				}
				results = append(results, &ExecuteResponse{
					SQL:         stmtTrimmed,
					Type:        "exec",
					Message:     "ROLLBACK executed.",
					ExecutionMs: time.Since(stmtStart).Milliseconds(),
					TxActive:    false,
				})
			} else {
				if err := tx.Rollback(); err != nil {
					return nil, fmt.Errorf("failed to rollback transaction: %w", err)
				}
				results = append(results, &ExecuteResponse{
					SQL:         stmtTrimmed,
					Type:        "exec",
					Message:     "Transaction rolled back successfully.",
					ExecutionMs: time.Since(stmtStart).Milliseconds(),
					TxActive:    false,
				})
			}
			continue
		}

		if isStmtSelect {
			var stream driver.RowStream
			var streamErr error
			if currentTx != nil {
				stream, streamErr = currentTx.QueryStream(ctx, stmtTrimmed, limit)
			} else {
				if s.Session == nil {
					return nil, errors.New("no active database session")
				}
				stream, streamErr = s.Session.QueryStream(ctx, stmtTrimmed, limit)
			}
			if streamErr != nil {
				if s.vault != nil {
					s.RecordAudit(stmtTrimmed, "editor_batch", time.Since(stmtStart).Milliseconds(), 0, streamErr, stmtStart)
				}
				return nil, fmt.Errorf("statement %d (%s) failed: %w", i+1, previewStmt(stmtTrimmed), streamErr)
			}
			cols := stream.Columns()
			var allRows [][]any
			var readErr error
			for len(allRows) < limit {
				chunk, err := stream.NextChunk(200)
				if len(chunk) > 0 {
					allRows = append(allRows, chunk...)
				}
				if err != nil {
					if err != io.EOF {
						readErr = err
					}
					break
				}
			}
			_ = stream.Close()
			if readErr != nil {
				return nil, fmt.Errorf("statement %d reading rows failed: %w", i+1, readErr)
			}
			qRes := &ExecuteResponse{
				SQL:         stmtTrimmed,
				Type:        "query",
				Columns:     cols,
				Rows:        allRows,
				RowCount:    len(allRows),
				ExecutionMs: time.Since(stmtStart).Milliseconds(),
				TxActive:    currentTx != nil,
			}
			if firstQuery == nil {
				firstQuery = qRes
			}
			results = append(results, qRes)
			if s.vault != nil {
				s.RecordAudit(stmtTrimmed, "editor_batch", qRes.ExecutionMs, int64(len(allRows)), nil, stmtStart)
			}
			continue
		}

		var execRes *driver.ExecResult
		var execErr error
		if currentTx != nil {
			execRes, execErr = currentTx.Exec(ctx, stmtTrimmed)
		} else {
			if strings.HasPrefix(cleanUpper, "ALTER ") {
				execRes, execErr = execSafeDDLOrFallback(ctx, s.Session, stmtTrimmed, req.LockTimeoutSec)
			} else {
				execRes, execErr = s.Session.Exec(ctx, stmtTrimmed)
			}
		}
		if execErr != nil {
			if s.vault != nil {
				s.RecordAudit(stmtTrimmed, "editor_batch", time.Since(stmtStart).Milliseconds(), 0, execErr, stmtStart)
			}
			return nil, fmt.Errorf("statement %d (%s) failed: %w", i+1, previewStmt(stmtTrimmed), execErr)
		}
		totalRowsAffected += execRes.RowsAffected
		eRes := &ExecuteResponse{
			SQL:          stmtTrimmed,
			Type:         "exec",
			RowsAffected: execRes.RowsAffected,
			LastInsertID: execRes.LastInsertID,
			ExecutionMs:  execRes.ExecutionMs,
			Message:      execRes.Message,
			TxActive:     currentTx != nil,
		}
		results = append(results, eRes)
		if s.vault != nil {
			s.RecordAudit(stmtTrimmed, "editor_batch", eRes.ExecutionMs, execRes.RowsAffected, nil, stmtStart)
		}
	}

	finalResp := &ExecuteResponse{
		Type:         "batch",
		Results:      results,
		ExecutionMs:  time.Since(start).Milliseconds(),
		TxActive:     s.hasActiveTx(req.TabID),
		Profile:      collectProfile(),
		Inspection:   inspection,
		RowsAffected: totalRowsAffected,
	}
	if firstQuery != nil {
		finalResp.Type = "query"
		finalResp.Columns = firstQuery.Columns
		finalResp.Rows = firstQuery.Rows
		finalResp.RowCount = firstQuery.RowCount
	} else if len(results) > 0 {
		finalResp.Type = "exec"
		finalResp.Message = fmt.Sprintf("%d statements executed successfully (%d rows affected)", len(results), totalRowsAffected)
	}
	return finalResp, nil
}

func (s *ActiveSession) executeStreamMulti(
	ctx context.Context,
	req ExecuteRequest,
	stmts []string,
	inspection *safety.InspectionResult,
	targetDB string,
	start time.Time,
	currentTx driver.TxSession,
	w io.Writer,
	flusher http.Flusher,
	runKey string,
) (*ExecuteResponse, error) {
	enc := json.NewEncoder(w)
	limit := req.Limit
	if limit <= 0 {
		limit = 5000
	}
	totalRowsAll := 0
	var streamErr error

	for i, stmt := range stmts {
		stmtTrimmed := strings.TrimSpace(stmt)
		if stmtTrimmed == "" {
			continue
		}
		stmtStart := time.Now()
		cleanSQL := driver.StripLeadingComments(stmtTrimmed)
		cleanUpper := strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
		isStmtSelect := driver.IsQueryStatement(stmtTrimmed)

		if strings.HasPrefix(cleanUpper, "USE ") {
			dbTarget := strings.TrimSpace(cleanSQL[4:])
			dbTarget = strings.Trim(strings.TrimRight(dbTarget, "; \t\r\n"), "`'\"")
			if dbTarget != "" {
				_ = s.UseDatabase(ctx, dbTarget)
			}
		}

		isTxBegin := cleanUpper == "BEGIN" || cleanUpper == "START TRANSACTION" || cleanUpper == "BEGIN WORK" || cleanUpper == "BEGIN TRANSACTION" ||
			strings.HasPrefix(cleanUpper, "START TRANSACTION ") ||
			strings.HasPrefix(cleanUpper, "BEGIN TRANSACTION ") ||
			strings.HasPrefix(cleanUpper, "BEGIN WORK ") ||
			(strings.HasPrefix(cleanUpper, "BEGIN ") && (strings.Contains(cleanUpper, "DEFERRED") || strings.Contains(cleanUpper, "IMMEDIATE") || strings.Contains(cleanUpper, "EXCLUSIVE") || strings.Contains(cleanUpper, "READ ONLY") || strings.Contains(cleanUpper, "READ WRITE") || strings.Contains(cleanUpper, "ISOLATION LEVEL")))
		isTxCommit := cleanUpper == "COMMIT" || cleanUpper == "COMMIT WORK" || cleanUpper == "COMMIT TRANSACTION" || cleanUpper == "END" || cleanUpper == "END WORK" || cleanUpper == "END TRANSACTION"
		isTxRollback := cleanUpper == "ROLLBACK" || cleanUpper == "ROLLBACK WORK" || cleanUpper == "ROLLBACK TRANSACTION" || cleanUpper == "ABORT" || cleanUpper == "ABORT WORK" || cleanUpper == "ABORT TRANSACTION"

		if isTxBegin {
			if s.hasActiveTx(req.TabID) {
				streamErr = fmt.Errorf("a transaction is already active in this tab; please COMMIT or ROLLBACK first")
				_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": streamErr.Error()})
				if flusher != nil {
					flusher.Flush()
				}
				return nil, streamErr
			}
			tx, err := s.Session.BeginTx(context.Background())
			if err != nil {
				streamErr = fmt.Errorf("failed to begin transaction: %w", err)
				_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": streamErr.Error()})
				if flusher != nil {
					flusher.Flush()
				}
				return nil, streamErr
			}
			s.setTx(req.TabID, tx)
			currentTx = tx
			_ = enc.Encode(map[string]any{
				"type":         "exec",
				"result_index": i,
				"sql":          stmtTrimmed,
				"message":      "Transaction started.",
				"tx_active":    true,
				"execution_ms": time.Since(stmtStart).Milliseconds(),
			})
			if flusher != nil {
				flusher.Flush()
			}
			continue
		}

		if isTxCommit {
			tx := s.clearTx(req.TabID)
			currentTx = nil
			if tx == nil {
				_, err := s.Session.Exec(ctx, stmtTrimmed)
				if err != nil {
					_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": err.Error()})
					if flusher != nil {
						flusher.Flush()
					}
					return nil, err
				}
				_ = enc.Encode(map[string]any{
					"type":         "exec",
					"result_index": i,
					"sql":          stmtTrimmed,
					"message":      "COMMIT executed.",
					"tx_active":    false,
					"execution_ms": time.Since(stmtStart).Milliseconds(),
				})
			} else {
				if err := tx.Commit(); err != nil {
					_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": err.Error()})
					if flusher != nil {
						flusher.Flush()
					}
					return nil, err
				}
				_ = enc.Encode(map[string]any{
					"type":         "exec",
					"result_index": i,
					"sql":          stmtTrimmed,
					"message":      "Transaction committed successfully.",
					"tx_active":    false,
					"execution_ms": time.Since(stmtStart).Milliseconds(),
				})
			}
			if flusher != nil {
				flusher.Flush()
			}
			continue
		}

		if isTxRollback {
			tx := s.clearTx(req.TabID)
			currentTx = nil
			if tx == nil {
				_, err := s.Session.Exec(ctx, stmtTrimmed)
				if err != nil {
					_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": err.Error()})
					if flusher != nil {
						flusher.Flush()
					}
					return nil, err
				}
				_ = enc.Encode(map[string]any{
					"type":         "exec",
					"result_index": i,
					"sql":          stmtTrimmed,
					"message":      "ROLLBACK executed.",
					"tx_active":    false,
					"execution_ms": time.Since(stmtStart).Milliseconds(),
				})
			} else {
				if err := tx.Rollback(); err != nil {
					_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": err.Error()})
					if flusher != nil {
						flusher.Flush()
					}
					return nil, err
				}
				_ = enc.Encode(map[string]any{
					"type":         "exec",
					"result_index": i,
					"sql":          stmtTrimmed,
					"message":      "Transaction rolled back successfully.",
					"tx_active":    false,
					"execution_ms": time.Since(stmtStart).Milliseconds(),
				})
			}
			if flusher != nil {
				flusher.Flush()
			}
			continue
		}

		if isStmtSelect {
			var stream driver.RowStream
			var err error
			if currentTx != nil {
				stream, err = currentTx.QueryStream(ctx, stmtTrimmed, limit+1)
			} else {
				if s.Session == nil {
					return nil, errors.New("no active database session")
				}
				stream, err = s.Session.QueryStream(ctx, stmtTrimmed, limit+1)
			}
			if err != nil {
				_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": err.Error()})
				if flusher != nil {
					flusher.Flush()
				}
				return nil, err
			}

			_ = enc.Encode(map[string]any{
				"type":         "meta",
				"result_index": i,
				"sql":          stmtTrimmed,
				"columns":      stream.Columns(),
				"tx_active":    currentTx != nil,
				"database":     targetDB,
			})
			if flusher != nil {
				flusher.Flush()
			}

			stmtRows := 0
			firstChunk, cErr := stream.NextChunk(min(20, limit))
			if len(firstChunk) > 0 {
				stmtRows += len(firstChunk)
				totalRowsAll += len(firstChunk)
				_ = enc.Encode(map[string]any{
					"type":         "chunk",
					"result_index": i,
					"rows":         firstChunk,
				})
				if flusher != nil {
					flusher.Flush()
				}
			}
			if cErr != nil && cErr != io.EOF {
				stream.Close()
				_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": cErr.Error()})
				if flusher != nil {
					flusher.Flush()
				}
				return nil, cErr
			}

			for stmtRows < limit && cErr == nil {
				batchSize := 200
				if stmtRows+batchSize > limit {
					batchSize = limit - stmtRows
				}
				chunk, nextErr := stream.NextChunk(batchSize)
				if len(chunk) > 0 {
					stmtRows += len(chunk)
					totalRowsAll += len(chunk)
					_ = enc.Encode(map[string]any{
						"type":         "chunk",
						"result_index": i,
						"rows":         chunk,
					})
					if flusher != nil {
						flusher.Flush()
					}
				}
				if nextErr != nil {
					if nextErr != io.EOF {
						stream.Close()
						_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": nextErr.Error()})
						if flusher != nil {
							flusher.Flush()
						}
						return nil, nextErr
					}
					break
				}
			}
			stream.Close()

			_ = enc.Encode(map[string]any{
				"type":         "statement_done",
				"result_index": i,
				"sql":          stmtTrimmed,
				"row_count":    stmtRows,
				"execution_ms": time.Since(stmtStart).Milliseconds(),
			})
			if flusher != nil {
				flusher.Flush()
			}
			if s.vault != nil {
				s.RecordAudit(stmtTrimmed, "editor_stream", time.Since(stmtStart).Milliseconds(), int64(stmtRows), nil, stmtStart)
			}
			continue
		}

		// Mutation statement
		var execRes *driver.ExecResult
		var execErr error
		if currentTx != nil {
			execRes, execErr = currentTx.Exec(ctx, stmtTrimmed)
		} else {
			if strings.HasPrefix(cleanUpper, "ALTER ") {
				execRes, execErr = execSafeDDLOrFallback(ctx, s.Session, stmtTrimmed, req.LockTimeoutSec)
			} else {
				execRes, execErr = s.Session.Exec(ctx, stmtTrimmed)
			}
		}
		if execErr != nil {
			_ = enc.Encode(map[string]any{"type": "error", "result_index": i, "sql": stmtTrimmed, "error": execErr.Error()})
			if flusher != nil {
				flusher.Flush()
			}
			return nil, execErr
		}
		_ = enc.Encode(map[string]any{
			"type":           "exec",
			"result_index":   i,
			"sql":            stmtTrimmed,
			"rows_affected":  execRes.RowsAffected,
			"last_insert_id": execRes.LastInsertID,
			"message":        execRes.Message,
			"execution_ms":   execRes.ExecutionMs,
			"tx_active":      currentTx != nil,
		})
		if flusher != nil {
			flusher.Flush()
		}
		if s.vault != nil {
			s.RecordAudit(stmtTrimmed, "editor_stream", execRes.ExecutionMs, execRes.RowsAffected, nil, stmtStart)
		}
	}

	_ = enc.Encode(map[string]any{
		"type":          "done",
		"total_results": len(stmts),
		"execution_ms":  time.Since(start).Milliseconds(),
		"tx_active":     s.hasActiveTx(req.TabID),
	})
	if flusher != nil {
		flusher.Flush()
	}
	return nil, nil
}


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
