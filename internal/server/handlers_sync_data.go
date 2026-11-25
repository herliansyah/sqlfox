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

	// Production safety guard: require explicit confirmation challenge before writing or truncating Production.
	if strings.EqualFold(tgtSess.Environment, "production") {
		if !req.Confirmed {
			writeJSON(w, http.StatusConflict, map[string]any{
				"blocked":          true,
				"requires_confirm": true,
				"message":          "Target is a Production database. Confirmation challenge required before data transfer.",
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

	sameSession := srcSess == tgtSess
	if sameSession && strings.EqualFold(strings.TrimSpace(req.SourceDB), strings.TrimSpace(req.TargetDB)) {
		writeError(w, http.StatusBadRequest, "source and target database cannot be identical on the same session")
		return
	}
	srcSess.beginInFlight()
	defer srcSess.endInFlight()
	if !sameSession {
		tgtSess.beginInFlight()
		defer tgtSess.endInFlight()
	}

	// Only mutate session-global UseDatabase when source and target are distinct sessions.
	// For same-server sync (SourceSessionID == TargetSessionID), calling tgtSess.UseDatabase
	// would overwrite SourceDB with TargetDB, causing DataTransfer --truncate to wipe TargetDB
	// and read from the emptied TargetDB! Instead, pass per-execution DB context AND qualify tables.
	if !sameSession {
		if err := srcSess.UseDatabase(r.Context(), req.SourceDB); err != nil {
			writeError(w, http.StatusBadRequest, "source: "+err.Error())
			return
		}
		if err := tgtSess.UseDatabase(r.Context(), req.TargetDB); err != nil {
			writeError(w, http.StatusBadRequest, "target: "+err.Error())
			return
		}
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
	srcCtx := driver.WithDatabase(ctx, req.SourceDB)
	tgtCtx := driver.WithDatabase(ctx, req.TargetDB)

	srcDriver := srcSess.DriverName
	tgtDriver := tgtSess.DriverName

	qualifyTable := func(db, tbl, drv string) string {
		qTbl := quoteIdent(tbl, drv)
		if db == "" || isSqliteDriver(drv) {
			return qTbl
		}
		return quoteIdent(db, drv) + "." + qTbl
	}

	var grandTotalRows int64
	var hasErrors bool
	cachedSrcSchema, _ := srcSess.Session.InspectSchema(srcCtx, req.SourceDB)

	// Order tables topologically by FK dependencies (parents before children) when schema is known.
	orderedTables := req.Tables
	if cachedSrcSchema != nil && len(req.Tables) > 1 {
		byLower := make(map[string]driver.TableMetadata, len(cachedSrcSchema.Tables))
		for _, t := range cachedSrcSchema.Tables {
			byLower[strings.ToLower(t.Name)] = t
		}
		var subset []driver.TableMetadata
		var extras []string
		for _, name := range req.Tables {
			if tm, ok := byLower[strings.ToLower(name)]; ok {
				subset = append(subset, tm)
			} else {
				extras = append(extras, name)
			}
		}
		if len(subset) > 0 {
			sorted := driver.TopologicalSortTables(subset)
			orderedTables = make([]string, 0, len(sorted)+len(extras))
			for _, t := range sorted {
				orderedTables = append(orderedTables, t.Name)
			}
			orderedTables = append(orderedTables, extras...)
		}
	}

	// Pre-create missing tables in parent->child order if requested so reverse-order truncate can succeed cleanly.
	if req.CreateTables {
		for _, tbl := range orderedTables {
			var ddl string
			var ddlErr error
			if cachedSrcSchema != nil && (!strings.EqualFold(srcDriver, tgtDriver) || (sameSession && isPgDriver(tgtDriver))) {
				for _, t := range cachedSrcSchema.Tables {
					if strings.EqualFold(t.Name, tbl) {
						ddl = buildCreateTableDDL(t, srcDriver, tgtDriver)
						break
					}
				}
			}
			if ddl == "" {
				ddl, ddlErr = srcSess.Session.GetTableDDL(srcCtx, tbl)
			}
			if ddlErr == nil && ddl != "" {
				if _, execErr := tgtSess.Session.Exec(tgtCtx, ddl); execErr != nil {
					lowerErr := strings.ToLower(execErr.Error())
					if !strings.Contains(lowerErr, "already exists") {
						sendEvent(map[string]any{
							"event": "table_warning",
							"table": tbl,
							"error": "create table DDL warning: " + execErr.Error(),
						})
					}
				}
			}
		}
	}

	// When truncating multiple FK-coupled tables, keep the reverse-topological DELETEs (children->parents)
	// and all table INSERTs (parents->children) inside a single transaction so any mid-transfer failure
	// rolls back every DELETE and never leaves target tables permanently wiped.
	var sharedTx driver.TxSession
	if req.TruncateTarget && len(orderedTables) > 1 {
		tx, txErr := tgtSess.Session.BeginTx(tgtCtx)
		if txErr != nil {
			hasErrors = true
			sendEvent(map[string]any{
				"event": "table_error",
				"table": orderedTables[0],
				"error": "failed to begin multi-table truncate transaction: " + txErr.Error(),
			})
			sendEvent(map[string]any{
				"event":             "done",
				"total_transferred": int64(0),
				"has_errors":        true,
			})
			return
		}
		var delErr error
		for i := len(orderedTables) - 1; i >= 0; i-- {
			tbl := orderedTables[i]
			if _, err := tx.Exec(tgtCtx, fmt.Sprintf("DELETE FROM %s", qualifyTable(req.TargetDB, tbl, tgtDriver))); err != nil {
				delErr = fmt.Errorf("failed to clear target table %s: %w", tbl, err)
				break
			}
		}
		if delErr != nil {
			_ = tx.Rollback()
			sendEvent(map[string]any{
				"event": "table_error",
				"table": orderedTables[0],
				"error": delErr.Error(),
			})
			sendEvent(map[string]any{
				"event":             "done",
				"total_transferred": int64(0),
				"has_errors":        true,
			})
			return
		}
		sharedTx = tx
		defer func() {
			if sharedTx != nil {
				_ = sharedTx.Rollback()
			}
		}()
	}

	for _, tbl := range orderedTables {
		sendEvent(map[string]any{
			"event":  "table_start",
			"table":  tbl,
			"status": "Starting transfer for " + tbl,
		})

		// Stream rows from source using source DB context and qualified table name
		querySQL := fmt.Sprintf("SELECT * FROM %s", qualifyTable(req.SourceDB, tbl, srcDriver))
		stream, err := srcSess.Session.QueryStream(srcCtx, querySQL, 0)
		if err != nil {
			hasErrors = true
			sendEvent(map[string]any{
				"event": "table_error",
				"table": tbl,
				"error": err.Error(),
			})
			if sharedTx != nil {
				break
			}
			continue
		}

		cols := stream.Columns()
		colNames := make([]string, len(cols))
		for i, c := range cols {
			colNames[i] = quoteIdent(c.Name, tgtDriver)
		}
		colListStr := strings.Join(colNames, ", ")

		// Reuse sharedTx for atomic multi-table truncate, or begin a per-table transaction.
		tx := sharedTx
		if tx == nil {
			tx, err = tgtSess.Session.BeginTx(tgtCtx)
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
		}

		var tableRowsCopied int64
		var tableErr error

		// Empty the target inside the transaction (DELETE, not TRUNCATE: MySQL TRUNCATE implicitly
		// commits) so a failed copy rolls back to the original rows instead of leaving the table empty.
		if req.TruncateTarget && sharedTx == nil {
			if _, err := tx.Exec(tgtCtx, fmt.Sprintf("DELETE FROM %s", qualifyTable(req.TargetDB, tbl, tgtDriver))); err != nil {
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
					qualifyTable(req.TargetDB, tbl, tgtDriver),
					colListStr,
					strings.Join(rowPlaceholders, ", "),
				)

				if _, execErr := tx.Exec(tgtCtx, insertSQL, args...); execErr != nil {
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
			hasErrors = true
			sendEvent(map[string]any{
				"event": "table_error",
				"table": tbl,
				"error": tableErr.Error(),
			})
			if sharedTx != nil {
				// Abort all remaining tables and roll back the shared truncate+insert transaction.
				break
			}
			_ = tx.Rollback()
			continue
		}

		if sharedTx == nil {
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

	if sharedTx != nil {
		if hasErrors {
			_ = sharedTx.Rollback()
			sharedTx = nil
			grandTotalRows = 0
		} else if err := sharedTx.Commit(); err != nil {
			_ = sharedTx.Rollback()
			sharedTx = nil
			hasErrors = true
			grandTotalRows = 0
			sendEvent(map[string]any{
				"event": "table_error",
				"table": orderedTables[len(orderedTables)-1],
				"error": "failed to commit multi-table transfer transaction: " + err.Error(),
			})
		} else {
			sharedTx = nil
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
	// the target table orders its columns differently; qualify with DB/schema for same-server comparisons.
	selectSQL := func(db, drv string) string {
		cols := make([]string, len(srcTbl.Columns))
		for i, c := range srcTbl.Columns {
			cols[i] = quoteIdent(c.Name, drv)
		}
		pks := make([]string, len(pkCols))
		for i, pk := range pkCols {
			pks[i] = quoteIdent(pk, drv)
		}
		tblRef := quoteIdent(req.Table, drv)
		if db != "" && !isSqliteDriver(drv) {
			tblRef = quoteIdent(db, drv) + "." + tblRef
		}
		return fmt.Sprintf("SELECT %s FROM %s ORDER BY %s", strings.Join(cols, ", "), tblRef, strings.Join(pks, ", "))
	}

	// Fetch all rows from source. Comparison is in-memory and capped by both row count and byte budget;
	// one extra row is fetched to detect overflow, because diffing a truncated window emits bogus diffs.
	const maxSyncRows = 10000
	const maxSyncBytes = 64 * 1024 * 1024
	estimateRowBytes := func(row []any) int {
		n := 64
		for _, v := range row {
			switch s := v.(type) {
			case string:
				n += len(s)
			case []byte:
				n += len(s)
			default:
				n += 16
			}
		}
		return n
	}

	srcStream, err := srcSess.Session.QueryStream(srcCtx, selectSQL(req.SourceDB, srcDriver), maxSyncRows+1)
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
	totalBytes := 0
	for {
		chunk, err := srcStream.NextChunk(1000)
		if err != nil && err != io.EOF {
			writeError(w, http.StatusInternalServerError, "error reading source rows: "+err.Error())
			return
		}
		for _, row := range chunk {
			totalBytes += estimateRowBytes(row)
			if totalBytes > maxSyncBytes {
				writeError(w, http.StatusRequestEntityTooLarge, "table row payload exceeds 64MB in-memory comparison limit; use Data Transfer instead")
				return
			}
		}
		srcRows = append(srcRows, chunk...)
		if err == io.EOF {
			break
		}
	}

	// Fetch all rows from target
	tgtStream, err := tgtSess.Session.QueryStream(tgtCtx, selectSQL(req.TargetDB, tgtDriver), maxSyncRows+1)
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
		for _, row := range chunk {
			totalBytes += estimateRowBytes(row)
			if totalBytes > maxSyncBytes {
				writeError(w, http.StatusRequestEntityTooLarge, "table row payload exceeds 64MB in-memory comparison limit; use Data Transfer instead")
				return
			}
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
