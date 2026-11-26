package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *ActiveSession) ExecuteStream(ctx context.Context, req ExecuteRequest, w io.Writer, flusher http.Flusher) (*ExecuteResponse, error) {
	s.beginInFlight()
	defer s.endInFlightTab(req.TabID)

	trimmedSQL := strings.TrimSpace(req.SQL)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

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

	if req.Database != "" {
		ctx = driver.WithDatabase(ctx, req.Database)
	}

	// 1. Safety Guard Inspection with Scoped Database Context
	targetDB := req.Database
	if targetDB == "" && s.Schema() != nil {
		targetDB = s.Schema().DatabaseName
	}
	if targetDB == "" {
		targetDB = s.GetDatabaseName()
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}
	if req.RemoteRequest && hasRemoteBlockedHostFileOp(trimmedSQL, s.Environment, s.DriverName, targetDB) {
		return nil, ErrRemoteHostFileSQLBlocked
	}

	// 2. Read-Only Enforcement
	if s.IsReadOnly() && (!allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
	}
	if inspection.RequiresChallenge && !req.Confirmed {
		return &ExecuteResponse{
			Blocked:    true,
			Inspection: inspection,
		}, nil
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
	} else if limit > 100000 {
		limit = 100000
	}

	isMySQLFamily := strings.EqualFold(s.DriverName, "mysql") || strings.EqualFold(s.DriverName, "mariadb")

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
	var finishProfile func() *driver.QueryProfile
	txProfEnabled := false
	if currentTx != nil {
		if req.Profile && isMySQLFamily {
			if _, pErr := currentTx.Exec(execCtx, "SET profiling = 1"); pErr == nil {
				txProfEnabled = true
			}
		}
		stream, streamErr = currentTx.QueryStream(execCtx, trimmedSQL, limit+1)
		if streamErr != nil && txProfEnabled {
			cCtx, cCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = currentTx.Exec(cCtx, "SET profiling = 0")
			cCancel()
		}
	} else {
		if s.Session == nil {
			return nil, errors.New("no active database session")
		}
		if req.Profile && isMySQLFamily {
			if profiler, ok := s.Session.(driver.PinnedProfiler); ok {
				stream, finishProfile, streamErr = profiler.QueryStreamProfiled(execCtx, trimmedSQL, limit+1, s.IsReadOnly())
			}
		}
		if stream == nil && streamErr == nil {
			if s.IsReadOnly() {
				if roSess, ok := s.Session.(driver.ReadOnlySession); ok {
					stream, streamErr = roSess.QueryStreamReadOnly(execCtx, trimmedSQL, limit+1)
				} else {
					stream, streamErr = s.Session.QueryStream(execCtx, trimmedSQL, limit+1)
				}
			} else {
				stream, streamErr = s.Session.QueryStream(execCtx, trimmedSQL, limit+1)
			}
		}
	}
	if streamErr != nil {
		return nil, streamErr
	}
	if finishProfile != nil {
		defer finishProfile()
	} else {
		defer stream.Close()
	}

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
			"rows": driver.FormatRowsForJSON(firstChunk),
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
				"rows": driver.FormatRowsForJSON(chunk),
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
	if finishProfile != nil {
		prof = finishProfile()
	} else if txProfEnabled && currentTx != nil {
		_ = stream.Close()
		profCtx, profCancel := context.WithTimeout(context.Background(), 3*time.Second)
		if pStream, pErr := currentTx.QueryStream(profCtx, "SHOW PROFILE", 100); pErr == nil && pStream != nil {
			prof = readProfileFromStream(pStream)
			_ = pStream.Close()
		}
		_, _ = currentTx.Exec(profCtx, "SET profiling = 0")
		profCancel()
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
