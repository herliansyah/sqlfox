package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
