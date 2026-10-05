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

	if sess.ReadOnly {
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
	if req.Database != "" {
		targetTable = quoteIdent(req.Database, sess.DriverName) + "." + targetTable
	}

	var cols []string
	var placeholders []string
	var args []any
	for col, val := range req.Data {
		cols = append(cols, quoteIdent(col, sess.DriverName))
		placeholders = append(placeholders, "?")
		args = append(args, val)
	}

	sqlQuery := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", targetTable, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	_ = sess.UseDatabase(ctx, req.Database)

	res, err := sess.Session.Exec(ctx, sqlQuery, args...)
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

	if sess.ReadOnly {
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
	if req.Database != "" {
		targetTable = quoteIdent(req.Database, sess.DriverName) + "." + targetTable
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	_ = sess.UseDatabase(ctx, req.Database)

	var totalAffected int64
	var execMs int64

	if req.PKColumn != "" && len(req.PKValues) > 0 {
		targetPK := quoteIdent(req.PKColumn, sess.DriverName)
		placeholders := make([]string, len(req.PKValues))
		args := make([]any, len(req.PKValues))
		for i, v := range req.PKValues {
			placeholders[i] = "?"
			args[i] = v
		}

		sqlQuery := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)", targetTable, targetPK, strings.Join(placeholders, ", "))
		res, err := sess.Session.Exec(ctx, sqlQuery, args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
			return
		}
		if res != nil {
			totalAffected = res.RowsAffected
			execMs = res.ExecutionMs
		}
	} else if len(req.RowCriterias) > 0 {
		start := time.Now()
		limitClause := ""
		if sess.DriverName == "mysql" {
			limitClause = " LIMIT 1"
		}
		for _, rc := range req.RowCriterias {
			var whereParts []string
			for cName, cVal := range rc {
				if cVal == nil {
					whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", quoteIdent(cName, sess.DriverName)))
				} else {
					whereParts = append(whereParts, fmt.Sprintf("%s = %s", quoteIdent(cName, sess.DriverName), driver.FormatSQLValue(cVal)))
				}
			}
			delSQL := fmt.Sprintf("DELETE FROM %s WHERE %s%s", targetTable, strings.Join(whereParts, " AND "), limitClause)
			res, err := sess.Session.Exec(ctx, delSQL)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
				return
			}
			if res != nil {
				totalAffected += res.RowsAffected
			}
		}
		execMs = time.Since(start).Milliseconds()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rows_affected": totalAffected,
		"execution_ms":  execMs,
	})
}
