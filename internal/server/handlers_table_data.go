package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/vault"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleGetTableRows(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			if val > 1000 {
				limit = 1000
			} else {
				limit = val
			}
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	sortCol := r.URL.Query().Get("sort_col")
	if sortCol == "" {
		sortCol = r.URL.Query().Get("sort")
	}
	sortDir := strings.ToUpper(r.URL.Query().Get("sort_dir"))
	if sortDir == "" {
		sortDir = strings.ToUpper(r.URL.Query().Get("dir"))
	}
	if sortDir != "DESC" {
		sortDir = "ASC"
	}

	// Keyset marker if provided. It is a value from a prior page's key column and is spliced into the
	// seek predicate as a dialect-escaped literal (QueryStream has no bind-parameter channel), so
	// reject NUL bytes (not neutralized by non-MySQL escaping) and cap its length at the trust
	// boundary before it ever reaches EscapeSQLString.
	lastKey := r.URL.Query().Get("last_key")
	if strings.IndexByte(lastKey, 0) >= 0 {
		writeError(w, http.StatusBadRequest, "invalid last_key: NUL bytes are not permitted")
		return
	}
	if len(lastKey) > 4096 {
		writeError(w, http.StatusBadRequest, "invalid last_key: value too long")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	targetDB := r.URL.Query().Get("db")
	if targetDB == "" && sess.DatabaseName != "" {
		targetDB = sess.DatabaseName
	}
	if targetDB != "" {
		if err := sess.UseDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	findKeyCol := func(tables []driver.TableMetadata) string {
		for _, t := range tables {
			if strings.EqualFold(t.Name, tableName) {
				// 1. Primary key - only if single column
				var pkCols []string
				for _, c := range t.Columns {
					if c.IsPrimaryKey {
						pkCols = append(pkCols, c.Name)
					}
				}
				if len(pkCols) == 1 {
					return pkCols[0]
				}
				if len(pkCols) > 1 {
					return "" // Composite PK: cannot use single-column seek or single WHERE pk=val
				}
				// 2. Single-column unique key fallback
				for _, idx := range t.Indexes {
					if idx.IsUnique && len(idx.Columns) == 1 {
						return idx.Columns[0]
					}
				}
				break
			}
		}
		return ""
	}

	// Find primary key or unique key if exists
	var pkCol string
	tableFoundInCache := false
	if sess.Schema() != nil {
		for _, t := range sess.Schema().Tables {
			if strings.EqualFold(t.Name, tableName) {
				tableFoundInCache = true
				break
			}
		}
		pkCol = findKeyCol(sess.Schema().Tables)
	}

	// If table not found in cache (e.g. multi-db context, newly created table, or stale cache), inspect on demand
	if !tableFoundInCache {
		if schema, err := sess.Session.InspectSchema(ctx, targetDB); err == nil && schema != nil {
			sess.SetSchema(schema)
			pkCol = findKeyCol(schema.Tables)
		}
	}

	targetTable := quoteIdent(tableName, sess.DriverName)
	if targetDB != "" && sess.DriverName != "sqlite" {
		targetTable = quoteIdent(targetDB, sess.DriverName) + "." + targetTable
	}

	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	structuredFilters := strings.TrimSpace(r.URL.Query().Get("filters"))
	if structuredFilters != "" {
		compiled, err := buildStructuredFilterClause(structuredFilters, sess.DriverName)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid structured filters: "+err.Error())
			return
		}
		filter = compiled
	} else if err := validateTableFilter(filter); err != nil {
		writeError(w, http.StatusBadRequest, "invalid filter: "+err.Error())
		return
	}

	whereClause := ""
	if filter != "" {
		whereClause = fmt.Sprintf("WHERE (%s)", filter)
	}

	var querySQL string
	if sortCol == "" && pkCol != "" {
		sortCol = pkCol
	}

	var escSortCol string
	if sortCol != "" {
		escSortCol = quoteIdent(sortCol, sess.DriverName)
	}

	if lastKey != "" && pkCol != "" && sortCol == pkCol {
		// Keyset pagination: fast O(1) seek
		op := ">"
		if sortDir == "DESC" {
			op = "<"
		}
		if whereClause != "" {
			querySQL = fmt.Sprintf("SELECT * FROM %s %s AND %s %s %s ORDER BY %s %s LIMIT %d",
				targetTable, whereClause, escSortCol, op, driver.EscapeSQLString(lastKey, sess.DriverName), escSortCol, sortDir, limit)
		} else {
			querySQL = fmt.Sprintf("SELECT * FROM %s WHERE %s %s %s ORDER BY %s %s LIMIT %d",
				targetTable, escSortCol, op, driver.EscapeSQLString(lastKey, sess.DriverName), escSortCol, sortDir, limit)
		}
	} else if sortCol != "" {
		// Bounded offset
		if whereClause != "" {
			querySQL = fmt.Sprintf("SELECT * FROM %s %s ORDER BY %s %s LIMIT %d OFFSET %d",
				targetTable, whereClause, escSortCol, sortDir, limit, offset)
		} else {
			querySQL = fmt.Sprintf("SELECT * FROM %s ORDER BY %s %s LIMIT %d OFFSET %d",
				targetTable, escSortCol, sortDir, limit, offset)
		}
	} else {
		if whereClause != "" {
			querySQL = fmt.Sprintf("SELECT * FROM %s %s LIMIT %d OFFSET %d", targetTable, whereClause, limit, offset)
		} else {
			querySQL = fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", targetTable, limit, offset)
		}
	}

	stream, err := sess.Session.QueryStream(ctx, querySQL, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
		return
	}
	defer stream.Close()

	cols := stream.Columns()
	chunk, err := stream.NextChunk(limit)
	if err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
		return
	}
	if chunk == nil {
		chunk = [][]any{}
	}

	var totalRows *int64
	var isExact bool

	// Optimization: If we queried from offset 0 (no keyset seek) and chunk returned fewer rows than limit,
	// then the exact total rows is known without querying SELECT COUNT(*).
	if lastKey == "" && len(chunk) < limit {
		cnt := int64(offset + len(chunk))
		totalRows = &cnt
		isExact = true
	} else {
		// Run a quick SELECT COUNT(*) with a 1-second timeout
		countCtx, countCancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer countCancel()

		countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s", targetTable)
		if whereClause != "" {
			countSQL = fmt.Sprintf("SELECT COUNT(*) FROM %s %s", targetTable, whereClause)
		}

		cStream, cErr := sess.Session.QueryStream(countCtx, countSQL, 1)
		if cErr == nil {
			cChunk, nErr := cStream.NextChunk(1)
			_ = cStream.Close()
			if nErr == nil && len(cChunk) > 0 && len(cChunk[0]) > 0 {
				if parsed, ok := parseCountValue(cChunk[0][0]); ok {
					totalRows = &parsed
					isExact = true
				}
			}
		}

		// Fallback: If fast count failed/timed out and there is no where clause, use table estimate
		if totalRows == nil && whereClause == "" {
			if est, err := sess.Session.GetTableEstimate(countCtx, tableName); err == nil && est != nil && est.RowCount > 0 {
				totalRows = &est.RowCount
				isExact = false
			}
		}
	}

	res := map[string]any{
		"columns":  cols,
		"rows":     chunk,
		"limit":    limit,
		"offset":   offset,
		"pk_col":   pkCol,
		"filter":   filter,
		"is_exact": isExact,
	}
	if totalRows != nil {
		res["total_rows"] = *totalRows
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetTableCount(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")

	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	targetDB := r.URL.Query().Get("db")
	if targetDB == "" && sess.DatabaseName != "" {
		targetDB = sess.DatabaseName
	}
	if targetDB != "" {
		if err := sess.UseDatabase(ctx, targetDB); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	structuredFilters := strings.TrimSpace(r.URL.Query().Get("filters"))
	if structuredFilters != "" {
		compiled, err := buildStructuredFilterClause(structuredFilters, sess.DriverName)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid structured filters: "+err.Error())
			return
		}
		filter = compiled
	} else if err := validateTableFilter(filter); err != nil {
		writeError(w, http.StatusBadRequest, "invalid filter: "+err.Error())
		return
	}

	targetTable := quoteIdent(tableName, sess.DriverName)
	if targetDB != "" && sess.DriverName != "sqlite" {
		targetTable = quoteIdent(targetDB, sess.DriverName) + "." + targetTable
	}

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s", targetTable)
	if filter != "" {
		countSQL = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE (%s)", targetTable, filter)
	}

	start := time.Now()
	stream, err := sess.Session.QueryStream(ctx, countSQL, 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "count failed: "+err.Error())
		return
	}
	defer stream.Close()

	chunk, err := stream.NextChunk(1)
	if err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "count failed: "+err.Error())
		return
	}

	var count int64
	if len(chunk) > 0 && len(chunk[0]) > 0 {
		if c, ok := parseCountValue(chunk[0][0]); ok {
			count = c
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"table":       tableName,
		"count":       count,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

// allowedFilterKeywords is the whitelist of bare keyword tokens permitted in a table quick-filter.
// Anything outside this set that is not a column identifier, quoted literal, number or operator is
// rejected. This is a positive grammar guard (allowlist), not a denylist: it blocks subquery
// injection (SELECT/UNION), time-based probes (SLEEP/pg_sleep/benchmark/waitfor), and any other
// function call, because those tokens are neither keywords here nor followed-identifier columns.
var allowedFilterKeywords = map[string]bool{
	"AND": true, "OR": true, "NOT": true, "IS": true, "NULL": true,
	"IN": true, "LIKE": true, "ILIKE": true, "BETWEEN": true, "ESCAPE": true,
	"TRUE": true, "FALSE": true, "DIV": true, "MOD": true,
}

// reFilterToken lexes one token: a quoted literal ('..' with doubled-quote / backslash escapes),
// a number, a word (keyword or identifier, optionally db.col), a comparison/arithmetic operator,
// or a parenthesis/comma. The alternation is ordered so quoted strings are consumed whole.
var reFilterToken = regexp.MustCompile(
	`^(?:` +
		`'(?:[^'\\]|'')*'` + // single-quoted string literal (no backslash escapes; dialect-safe)
		`|"(?:[^"\\]|"")*"` + // double-quoted identifier/literal (no backslash escapes; dialect-safe)
		"|`(?:[^`]|``)*`" + // backtick-quoted identifier
		`|[0-9]+(?:\.[0-9]+)?(?:[eE][-+]?[0-9]+)?` + // number
		`|[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*` + // word / qualified identifier
		`|<=|>=|<>|!=|<|>|=|\+|-|\*|/|%|\(|\)|,` + // operators & grouping
		`)`)

// reFilterComment matches any SQL comment opener; comments are never valid inside a quick-filter.
var reFilterComment = regexp.MustCompile(`--|#|/\*`)

// structuredFilterPill mirrors a typed frontend filter pill so pill-based filters never
// rely on raw SQL string splicing from the client.
type structuredFilterPill struct {
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// buildStructuredFilterClause compiles a JSON array of structured filter pills into a
// strictly quoted, dialect-escaped WHERE predicate without accepting raw SQL expressions.
func buildStructuredFilterClause(rawJSON, driverName string) (string, error) {
	var pills []structuredFilterPill
	if err := json.Unmarshal([]byte(rawJSON), &pills); err != nil {
		return "", fmt.Errorf("malformed filter JSON: %w", err)
	}
	if len(pills) == 0 {
		return "", nil
	}
	var parts []string
	for _, p := range pills {
		col := strings.TrimSpace(p.Column)
		if col == "" || strings.ContainsRune(col, 0) {
			return "", errors.New("filter column name is required")
		}
		qCol := driver.QuoteIdentifier(col, driverName)
		op := strings.ToUpper(strings.TrimSpace(p.Operator))
		switch op {
		case "IS NULL", "IS NOT NULL":
			parts = append(parts, fmt.Sprintf("%s %s", qCol, op))
		case "=", "!=", "<>", "<", "<=", ">", ">=", "LIKE", "NOT LIKE":
			if strings.ContainsRune(p.Value, 0) {
				return "", errors.New("NUL bytes are not permitted in filter values")
			}
			parts = append(parts, fmt.Sprintf("%s %s %s", qCol, op, driver.EscapeSQLString(p.Value, driverName)))
		case "IN", "NOT IN":
			rawItems := strings.Split(p.Value, ",")
			var quoted []string
			for _, item := range rawItems {
				trimmed := strings.TrimSpace(item)
				if trimmed == "" {
					continue
				}
				if strings.ContainsRune(trimmed, 0) {
					return "", errors.New("NUL bytes are not permitted in filter values")
				}
				quoted = append(quoted, driver.EscapeSQLString(trimmed, driverName))
			}
			if len(quoted) == 0 {
				return "", fmt.Errorf("%s operator requires at least one value", op)
			}
			parts = append(parts, fmt.Sprintf("%s %s (%s)", qCol, op, strings.Join(quoted, ", ")))
		default:
			return "", fmt.Errorf("unsupported filter operator %q", p.Operator)
		}
	}
	return strings.Join(parts, " AND "), nil
}

// validateTableFilter enforces a positive grammar on the table quick-filter before it is spliced
// into a server-side WHERE clause. It rejects statement terminators, comments, and any token that
// is not a column identifier, literal, permitted keyword or comparison operator. A word immediately
// followed by '(' is a function call and is rejected, which blocks SLEEP()/pg_sleep()/(SELECT ...).
func validateTableFilter(filter string) error {
	trimmed := strings.TrimSpace(filter)
	if trimmed == "" {
		return nil
	}
	if strings.ContainsRune(trimmed, ';') {
		return errors.New("semicolons are not permitted in filter expressions")
	}
	if strings.ContainsRune(trimmed, 0) {
		return errors.New("NUL bytes are not permitted in filter expressions")
	}
	if reFilterComment.MatchString(trimmed) {
		return errors.New("comments are not permitted in filter expressions")
	}

	rest := trimmed
	depth := 0
	prevWasValue := false // previous token was an identifier/literal/')' — a '(' after it is a call
	for {
		rest = strings.TrimLeft(rest, " \t\r\n")
		if rest == "" {
			break
		}
		loc := reFilterToken.FindString(rest)
		if loc == "" {
			return fmt.Errorf("illegal character in filter expression near %q", truncateForError(rest))
		}
		tok := loc

		switch {
		case tok == "(":
			if prevWasValue {
				// identifier/')' directly before '(' means a function call or subquery — forbidden.
				return errors.New("function calls and subqueries are not permitted in filter expressions")
			}
			depth++
			prevWasValue = false
		case tok == ")":
			depth--
			if depth < 0 {
				return errors.New("unbalanced parentheses in filter expression")
			}
			prevWasValue = true
		case tok == ",":
			prevWasValue = false
		case isFilterOperator(tok):
			prevWasValue = false
		case isFilterWord(tok):
			upper := strings.ToUpper(tok)
			if allowedFilterKeywords[upper] {
				// A bare keyword is not itself a value, except NULL/TRUE/FALSE which terminate a term.
				prevWasValue = upper == "NULL" || upper == "TRUE" || upper == "FALSE"
			} else if isReservedSQLVerb(upper) {
				return errors.New("filter expression contains a forbidden SQL keyword")
			} else {
				// Treated as a column identifier.
				prevWasValue = true
			}
		default:
			// Quoted literal or number.
			prevWasValue = true
		}

		rest = rest[len(loc):]
	}

	if depth != 0 {
		return errors.New("unbalanced parentheses in filter expression")
	}
	return nil
}

func isFilterWord(tok string) bool {
	c := tok[0]
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '`' || c == '"'
}

func isFilterOperator(tok string) bool {
	switch tok {
	case "<=", ">=", "<>", "!=", "<", ">", "=", "+", "-", "*", "/", "%":
		return true
	}
	return false
}

// isReservedSQLVerb blocks statement-leading verbs and set operators that must never appear in a
// value predicate even without parentheses (e.g. "id = 1 UNION SELECT ...").
func isReservedSQLVerb(upper string) bool {
	switch upper {
	case "SELECT", "UNION", "INTERSECT", "EXCEPT", "INSERT", "UPDATE", "DELETE",
		"DROP", "ALTER", "TRUNCATE", "CREATE", "GRANT", "REVOKE", "SHUTDOWN",
		"INTO", "EXEC", "EXECUTE", "CALL", "MERGE", "REPLACE", "LOAD", "HANDLER",
		"VALUES", "SET", "DECLARE", "WAITFOR", "WITH":
		return true
	}
	return false
}

func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 24 {
		return s[:24] + "…"
	}
	return s
}

func (s *Server) handleCellEdit(w http.ResponseWriter, r *http.Request) {
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

	type CellEditItem struct {
		Database    string         `json:"database"`
		Table       string         `json:"table"`
		Column      string         `json:"column"`
		Value       any            `json:"value"`
		PKColumn    string         `json:"pk_column"`
		PKValue     any            `json:"pk_value"`
		PKValues    map[string]any `json:"pk_values,omitempty"` // For composite primary keys
		RowCriteria map[string]any `json:"row_criteria,omitempty"`
	}

	var req struct {
		Database    string         `json:"database"`
		Table       string         `json:"table"`
		Column      string         `json:"column"`
		Value       any            `json:"value"`
		PKColumn    string         `json:"pk_column"`
		PKValue     any            `json:"pk_value"`
		PKValues    map[string]any `json:"pk_values,omitempty"`
		RowCriteria map[string]any `json:"row_criteria,omitempty"`
		Edits       []CellEditItem `json:"edits"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid cell edit payload: "+err.Error())
		return
	}

	var items []CellEditItem
	if len(req.Edits) > 0 {
		items = req.Edits
	} else if req.Table != "" && req.Column != "" && ((req.PKColumn != "" && req.PKValue != nil) || len(req.PKValues) > 0 || len(req.RowCriteria) > 0) {
		items = append(items, CellEditItem{
			Database:    req.Database,
			Table:       req.Table,
			Column:      req.Column,
			Value:       req.Value,
			PKColumn:    req.PKColumn,
			PKValue:     req.PKValue,
			PKValues:    req.PKValues,
			RowCriteria: req.RowCriteria,
		})
	} else {
		writeError(w, http.StatusBadRequest, "table, column, and either pk_column/pk_value, pk_values, or row_criteria are required")
		return
	}

	quoteIdent := func(name string) string { return quoteIdent(name, sess.DriverName) }

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	exec, finish, err := sess.gridWriter(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
		return
	}

	var totalAffected int64
	for _, item := range items {
		tableRef := quoteIdent(item.Table)
		db := item.Database
		if db == "" {
			db = req.Database
		}
		if db != "" && sess.DriverName != "sqlite" {
			tableRef = fmt.Sprintf("%s.%s", quoteIdent(db), quoteIdent(item.Table))
		}

		var updateSQL string
		var auditSQL string
		var args []any
		pIdx := 1
		pVal := driver.Placeholder(sess.DriverName, pIdx)
		pIdx++
		args = append(args, item.Value)
		valLiteral := driver.FormatSQLValueForDriver(item.Value, sess.DriverName)

		if len(item.PKValues) > 0 && len(item.RowCriteria) == 0 {
			var pkWheres []string
			var pkAuditWheres []string
			keys := make([]string, 0, len(item.PKValues))
			for k := range item.PKValues {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				val := item.PKValues[k]
				pPK := driver.Placeholder(sess.DriverName, pIdx)
				pIdx++
				args = append(args, val)
				pkLiteral := driver.FormatSQLValueForDriver(val, sess.DriverName)
				pkWheres = append(pkWheres, fmt.Sprintf("%s = %s", quoteIdent(k), pPK))
				pkAuditWheres = append(pkAuditWheres, fmt.Sprintf("%s = %s", quoteIdent(k), pkLiteral))
			}
			updateSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s",
				tableRef, quoteIdent(item.Column), pVal, strings.Join(pkWheres, " AND "))
			auditSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s",
				tableRef, quoteIdent(item.Column), valLiteral, strings.Join(pkAuditWheres, " AND "))
		} else if item.PKColumn != "" && item.PKValue != nil && len(item.RowCriteria) == 0 {
			pPK := driver.Placeholder(sess.DriverName, pIdx)
			pIdx++
			args = append(args, item.PKValue)
			pkLiteral := driver.FormatSQLValueForDriver(item.PKValue, sess.DriverName)
			updateSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
				tableRef, quoteIdent(item.Column), pVal, quoteIdent(item.PKColumn), pPK)
			auditSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s = %s",
				tableRef, quoteIdent(item.Column), valLiteral, quoteIdent(item.PKColumn), pkLiteral)
		} else if len(item.RowCriteria) > 0 {
			var whereParts []string
			var auditWhereParts []string
			for cName, cVal := range item.RowCriteria {
				if cVal == nil {
					whereParts = append(whereParts, fmt.Sprintf("%s IS NULL", quoteIdent(cName)))
					auditWhereParts = append(auditWhereParts, fmt.Sprintf("%s IS NULL", quoteIdent(cName)))
				} else {
					pCrit := driver.Placeholder(sess.DriverName, pIdx)
					pIdx++
					args = append(args, cVal)
					whereParts = append(whereParts, fmt.Sprintf("%s = %s", quoteIdent(cName), pCrit))
					auditWhereParts = append(auditWhereParts, fmt.Sprintf("%s = %s", quoteIdent(cName), driver.FormatSQLValueForDriver(cVal, sess.DriverName)))
				}
			}
			updateSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s",
				tableRef, quoteIdent(item.Column), pVal, singleRowWhere(sess.DriverName, tableRef, strings.Join(whereParts, " AND ")))
			auditSQL = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s",
				tableRef, quoteIdent(item.Column), valLiteral, singleRowWhere(sess.DriverName, tableRef, strings.Join(auditWhereParts, " AND ")))
		} else {
			continue
		}

		cellStart := time.Now()
		res, err := exec.Exec(ctx, updateSQL, args...)
		cellDur := time.Since(cellStart).Milliseconds()
		var cellAffected int64
		if res != nil {
			cellAffected = res.RowsAffected
			totalAffected += res.RowsAffected
		}
		sess.RecordAudit(auditSQL, "grid_cell_edit", cellDur, cellAffected, err, cellStart)
		if err != nil {
			_ = finish(err)
			writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
			return
		}
	}
	if err := finish(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cell: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"rows_affected": totalAffected,
		"applied_count": len(items),
	})
}

func (s *Server) handleToggleReadOnly(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	sess, err := s.sessionMgr.GetSession(connID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ro := sess.ToggleReadOnly()
	s.sessionMgr.SetReadOnly(connID, ro)
	writeJSON(w, http.StatusOK, map[string]any{
		"connection_id": connID,
		"read_only":     ro,
	})
}

func (s *Server) handleGetTableNote(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")
	dbName := r.URL.Query().Get("db")

	if connID == "" || tableName == "" {
		writeError(w, http.StatusBadRequest, "connection id and table name are required")
		return
	}

	targetTag := "db://" + connID + "/" + dbName + "/" + tableName
	if opNotes, err := s.vault.ListOperationalNotes(vault.OperationalNoteFilter{ObjectTag: targetTag}); err == nil && len(opNotes) > 0 {
		top := opNotes[0]
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            top.ID,
			"connection_id": connID,
			"database_name": dbName,
			"table_name":    tableName,
			"content":       top.Content,
			"side_note":     top.SideNote,
			"updated_at":    top.UpdatedAt,
		})
		return
	}

	note, err := s.vault.GetTableNote(connID, dbName, tableName)
	if err != nil {
		if err == vault.ErrTableNoteNotFound {
			writeJSON(w, http.StatusOK, map[string]any{
				"content":    "",
				"updated_at": nil,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleSaveTableNote(w http.ResponseWriter, r *http.Request) {
	connID := r.PathValue("id")
	tableName := r.PathValue("table")
	dbName := r.URL.Query().Get("db")

	if connID == "" || tableName == "" {
		writeError(w, http.StatusBadRequest, "connection id and table name are required")
		return
	}

	var req struct {
		Content  string `json:"content"`
		SideNote string `json:"side_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	targetTag := "db://" + connID + "/" + dbName + "/" + tableName
	// Check if operational note already exists for this table tag
	var existingNote *vault.OperationalNote
	if opNotes, err := s.vault.ListOperationalNotes(vault.OperationalNoteFilter{ObjectTag: targetTag}); err == nil && len(opNotes) > 0 {
		for _, n := range opNotes {
			if n.IsPinned || strings.HasPrefix(n.Title, "Runbook:") {
				existingNote = n
				break
			}
		}
		if existingNote == nil && len(opNotes) > 0 {
			existingNote = opNotes[0]
		}
	}

	if existingNote != nil {
		existingNote.Content = req.Content
		if req.SideNote != "" {
			existingNote.SideNote = req.SideNote
		}
		if err := s.vault.UpdateOperationalNote(existingNote); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update table note: "+err.Error())
			return
		}
	} else {
		newOpNote := &vault.OperationalNote{
			Title:      "Runbook: " + tableName,
			Content:    req.Content,
			SideNote:   req.SideNote,
			Status:     "inbox",
			IsPinned:   true,
			CapturedAt: time.Now(),
			ObjectTags: []string{targetTag},
			UserTags:   []string{"#runbook"},
		}
		if err := s.vault.CreateOperationalNote(newOpNote); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create table note: "+err.Error())
			return
		}
	}

	note := &vault.TableNote{
		ConnectionID: connID,
		DatabaseName: dbName,
		TableName:    tableName,
		Content:      req.Content,
	}

	s.notifyCloudSyncChange()
	writeJSON(w, http.StatusOK, note)
}
