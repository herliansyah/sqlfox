package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/safety"
	"io"
	"net/http"
	"strings"
	"time"
)

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
	} else if limit > 100000 {
		limit = 100000
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
			_, wasAborted, abortReason := s.getTxStatus(req.TabID)
			if wasAborted {
				return nil, fmt.Errorf("cannot COMMIT: transaction in this tab was aborted (%s). Issue ROLLBACK to clear the aborted state", abortReason)
			}
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
				stream, streamErr = s.queryStreamSafe(ctx, stmtTrimmed, limit)
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
			_, wasAborted, abortReason := s.getTxStatus(req.TabID)
			if wasAborted {
				return nil, fmt.Errorf("cannot COMMIT: transaction in this tab was aborted (%s). Issue ROLLBACK to clear the aborted state", abortReason)
			}
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
				stream, err = s.queryStreamSafe(ctx, stmtTrimmed, limit+1)
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
