package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

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
	var cachedSrcSchema *driver.SchemaMetadata
	if req.CreateTables && !strings.EqualFold(srcDriver, tgtDriver) {
		cachedSrcSchema, _ = srcSess.Session.InspectSchema(ctx, req.SourceDB)
	}

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
			if cachedSrcSchema != nil {
				for _, t := range cachedSrcSchema.Tables {
					if strings.EqualFold(t.Name, tbl) {
						ddl = buildCreateTableDDL(t, srcDriver, tgtDriver)
						break
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
