package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

type InsertRowRequest struct {
	Database string         `json:"database"`
	Table    string         `json:"table"`
	Data     map[string]any `json:"data"`
}

type DeleteRowsRequest struct {
	Database     string           `json:"database"`
	Table        string           `json:"table"`
	PKColumn     string           `json:"pk_column"`
	PKValues     []any            `json:"pk_values"`
	RowCriterias []map[string]any `json:"row_criterias,omitempty"`
}

func (s *Server) handleInsertRow(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if sess.IsReadOnly() {
		writeError(w, http.StatusForbidden, "Session is locked in READ-ONLY mode")
		return
	}

	var req InsertRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid insert-row payload: "+err.Error())
		return
	}

	if req.Table == "" || len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "table and non-empty data map are required")
		return
	}

	targetTable := quoteIdent(req.Table, sess.DriverName)
	if req.Database != "" && sess.DriverName != "sqlite" {
		targetTable = quoteIdent(req.Database, sess.DriverName) + "." + targetTable
	}

	var cols []string
	var placeholders []string
	var args []any
	for col, val := range req.Data {
		cols = append(cols, quoteIdent(col, sess.DriverName))
		placeholders = append(placeholders, driver.Placeholder(sess.DriverName, len(placeholders)+1))
		args = append(args, val)
	}

	sqlQuery := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", targetTable, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := sess.UseDatabase(ctx, req.Database); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var valStrs []string
	for _, a := range args {
		valStrs = append(valStrs, driver.FormatSQLValueForDriver(a, sess.DriverName))
	}
	auditSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", targetTable, strings.Join(cols, ", "), strings.Join(valStrs, ", "))

	insertStart := time.Now()
	res, err := sess.Session.Exec(ctx, sqlQuery, args...)
	insertDur := time.Since(insertStart).Milliseconds()
	var affected int64
	if res != nil {
		affected = res.RowsAffected
	}
	sess.RecordAudit(auditSQL, "grid_insert", insertDur, affected, err, insertStart)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rows_affected":  res.RowsAffected,
		"last_insert_id": res.LastInsertID,
		"execution_ms":   res.ExecutionMs,
	})
}

func (s *Server) handleDeleteRows(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if sess.IsReadOnly() {
		writeError(w, http.StatusForbidden, "Session is locked in READ-ONLY mode")
		return
	}

	var req DeleteRowsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid delete-rows payload: "+err.Error())
		return
	}

	if req.Table == "" || ((req.PKColumn == "" || len(req.PKValues) == 0) && len(req.RowCriterias) == 0) {
		writeError(w, http.StatusBadRequest, "table and either (pk_column + pk_values) or row_criterias are required")
		return
	}

	targetTable := quoteIdent(req.Table, sess.DriverName)
	if req.Database != "" && sess.DriverName != "sqlite" {
		targetTable = quoteIdent(req.Database, sess.DriverName) + "." + targetTable
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := sess.UseDatabase(ctx, req.Database); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var totalAffected int64
	var execMs int64

	if req.PKColumn != "" && len(req.PKValues) > 0 {
		targetPK := quoteIdent(req.PKColumn, sess.DriverName)
		placeholders := make([]string, len(req.PKValues))
		args := make([]any, len(req.PKValues))
		valStrs := make([]string, len(req.PKValues))
		for i, v := range req.PKValues {
			placeholders[i] = driver.Placeholder(sess.DriverName, i+1)
			args[i] = v
			valStrs[i] = driver.FormatSQLValueForDriver(v, sess.DriverName)
		}

		sqlQuery := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)", targetTable, targetPK, strings.Join(placeholders, ", "))
		auditSQL := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)", targetTable, targetPK, strings.Join(valStrs, ", "))

		delStart := time.Now()
		res, err := sess.Session.Exec(ctx, sqlQuery, args...)
		delDur := time.Since(delStart).Milliseconds()
		if res != nil {
			totalAffected = res.RowsAffected
			execMs = res.ExecutionMs
		}
		sess.RecordAudit(auditSQL, "grid_delete", delDur, totalAffected, err, delStart)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
			return
		}
	} else if len(req.RowCriterias) > 0 {
		start := time.Now()
		exec, finish, err := sess.gridWriter(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
			return
		}
		for _, rc := range req.RowCriterias {
			var whereParts []string
			var auditWhereParts []string
			var args []any
			pIdx := 1
			for cName, cVal := range rc {
				if cVal == nil {
					whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", quoteIdent(cName, sess.DriverName)))
					auditWhereParts = append(auditWhereParts, fmt.Sprintf("%s IS NULL", quoteIdent(cName, sess.DriverName)))
				} else {
					pCrit := driver.Placeholder(sess.DriverName, pIdx)
					pIdx++
					args = append(args, cVal)
					whereParts = append(whereParts, fmt.Sprintf("%s = %s", quoteIdent(cName, sess.DriverName), pCrit))
					auditWhereParts = append(auditWhereParts, fmt.Sprintf("%s = %s", quoteIdent(cName, sess.DriverName), driver.FormatSQLValueForDriver(cVal, sess.DriverName)))
				}
			}
			delSQL := "DELETE FROM " + targetTable + " WHERE " + singleRowWhere(sess.DriverName, targetTable, strings.Join(whereParts, " AND "))
			auditSQL := "DELETE FROM " + targetTable + " WHERE " + singleRowWhere(sess.DriverName, targetTable, strings.Join(auditWhereParts, " AND "))
			rowStart := time.Now()
			res, err := exec.Exec(ctx, delSQL, args...)
			rowDur := time.Since(rowStart).Milliseconds()
			var rowAffected int64
			if res != nil {
				rowAffected = res.RowsAffected
				totalAffected += res.RowsAffected
			}
			sess.RecordAudit(auditSQL, "grid_delete", rowDur, rowAffected, err, rowStart)
			if err != nil {
				_ = finish(err)
				writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
				return
			}
		}
		if err := finish(nil); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
			return
		}
		execMs = time.Since(start).Milliseconds()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rows_affected": totalAffected,
		"execution_ms":  execMs,
	})
}

// singleRowWhere turns a full-row criteria predicate into one that matches at most ONE physical row,
// so editing/deleting one of several identical rows in a PK-less table never touches its duplicates.
// MySQL supports UPDATE/DELETE ... LIMIT 1; SQLite and Postgres need a physical row-id subquery.
func singleRowWhere(driverName, tableRef, where string) string {
	switch driverName {
	case "mysql":
		return where + " LIMIT 1"
	case "sqlite":
		// PK-less tables are never WITHOUT ROWID, so rowid always exists here.
		return fmt.Sprintf("rowid IN (SELECT rowid FROM %s WHERE %s LIMIT 1)", tableRef, where)
	case "postgres":
		// tableoid keeps ctid unique across partitions of a partitioned table.
		return fmt.Sprintf("(tableoid, ctid) IN (SELECT tableoid, ctid FROM %s WHERE %s LIMIT 1)", tableRef, where)
	}
	return where
}
