package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"strings"
	"time"
)

func (s *PostgresSession) InspectSchema(ctx context.Context, schemaName string) (*driver.SchemaMetadata, error) {
	return s.inspectSchemaFiltered(ctx, schemaName, "")
}

func (s *PostgresSession) inspectSchemaFiltered(ctx context.Context, schemaName, onlyTable string) (*driver.SchemaMetadata, error) {
	if schemaName == "" || schemaName == s.cfg.DatabaseName {
		schemaName = s.activeSchema()
	}
	if schemaName == "" {
		schemaName = "public"
	}

	var allSchemas []string
	if onlyTable == "" {
		// 1. Fetch all user-defined schemas in the active physical database
		// ponytail: Close each result set immediately to prevent holding 7 concurrent connections and deadlocking sql.DB pool.
		schemaRows, err := s.db.QueryContext(ctx, `
			SELECT nspname FROM pg_catalog.pg_namespace
			WHERE nspname NOT LIKE 'pg_%' AND nspname != 'information_schema'
			ORDER BY nspname;
		`)
		if err == nil {
			for schemaRows.Next() {
				var sch string
				if err := schemaRows.Scan(&sch); err == nil {
					allSchemas = append(allSchemas, sch)
				}
			}
			schemaRows.Close()
		}
		if len(allSchemas) == 0 {
			allSchemas = []string{schemaName}
		}
	}

	// 2. Fetch tables and views in current schema
	tableRows, err := s.db.QueryContext(ctx, `
		SELECT c.relname,
		       CASE c.relkind WHEN 'r' THEN 'TABLE' WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'VIEW' ELSE 'TABLE' END as reltype,
		       COALESCE(c.reltuples::bigint, 0) as row_count
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'v', 'm', 'p')
		  AND ($2 = '' OR LOWER(c.relname) = LOWER($2))
		ORDER BY c.relname;
	`, schemaName, onlyTable)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tables for schema %s: %w", schemaName, err)
	}

	tablesMap := make(map[string]*driver.TableMetadata)
	var tableOrder []string

	for tableRows.Next() {
		var name, reltype string
		var rowCount int64
		if err := tableRows.Scan(&name, &reltype, &rowCount); err != nil {
			tableRows.Close()
			return nil, err
		}
		if rowCount < 0 {
			rowCount = 0
		}
		tablesMap[name] = &driver.TableMetadata{
			Name:        name,
			Type:        reltype,
			RowCount:    rowCount,
			Columns:     []driver.ColumnMetadata{},
			Indexes:     []driver.IndexMetadata{},
			ForeignKeys: []driver.ForeignKeyMetadata{},
		}
		tableOrder = append(tableOrder, name)
	}
	tableRows.Close()

	// 3. Fetch columns for all tables in this schema
	colRows, err := s.db.QueryContext(ctx, `
		SELECT c.relname, a.attname,
		       pg_catalog.format_type(a.atttypid, a.atttypmod) as data_type,
		       NOT a.attnotnull as is_nullable,
		       COALESCE(pg_catalog.pg_get_expr(ad.adbin, ad.adrelid), '') as default_value,
		       COALESCE(i.indisprimary, false) as is_pk,
		       CASE WHEN t.typtype = 'e' THEN COALESCE((
		           SELECT string_agg(e.enumlabel, E'\x1f' ORDER BY e.enumsortorder)
		           FROM pg_catalog.pg_enum e
		           WHERE e.enumtypid = a.atttypid
		       ), '') ELSE '' END as enum_values
		FROM pg_catalog.pg_attribute a
		JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_type t ON t.oid = a.atttypid
		LEFT JOIN pg_catalog.pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		LEFT JOIN pg_catalog.pg_index i ON i.indrelid = c.oid AND a.attnum = ANY(i.indkey) AND i.indisprimary
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'v', 'm', 'p') AND a.attnum > 0 AND NOT a.attisdropped
		  AND ($2 = '' OR LOWER(c.relname) = LOWER($2))
		ORDER BY c.relname, a.attnum;
	`, schemaName, onlyTable)
	if err == nil {
		for colRows.Next() {
			var tbl, col, dataType, defaultVal, enumRaw string
			var isNullable, isPK bool
			if err := colRows.Scan(&tbl, &col, &dataType, &isNullable, &defaultVal, &isPK, &enumRaw); err == nil {
				if t, ok := tablesMap[tbl]; ok {
					extra := ""
					if strings.Contains(strings.ToLower(defaultVal), "nextval(") {
						extra = "auto_increment"
					}
					var enumVals []string
					if enumRaw != "" {
						enumVals = strings.Split(enumRaw, "\x1f")
					}
					t.Columns = append(t.Columns, driver.ColumnMetadata{
						Name:         col,
						Type:         dataType,
						IsNullable:   isNullable,
						IsPrimaryKey: isPK,
						DefaultValue: defaultVal,
						Extra:        extra,
						EnumValues:   enumVals,
					})
				}
			}
		}
		colRows.Close()
	}

	// 4. Fetch indexes
	idxRows, err := s.db.QueryContext(ctx, `
		SELECT c.relname as table_name,
		       i.relname as index_name,
		       ix.indisunique as is_unique,
		       ix.indisprimary as is_primary,
		       pg_catalog.pg_get_indexdef(ix.indexrelid, 0, true) as index_def,
		       am.amname as index_type,
		       COALESCE(pg_catalog.pg_get_expr(ix.indpred, ix.indrelid), '') as where_clause
		FROM pg_catalog.pg_index ix
		JOIN pg_catalog.pg_class c ON c.oid = ix.indrelid
		JOIN pg_catalog.pg_class i ON i.oid = ix.indexrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_am am ON am.oid = i.relam
		WHERE n.nspname = $1
		  AND ($2 = '' OR LOWER(c.relname) = LOWER($2))
		ORDER BY c.relname, i.relname;
	`, schemaName, onlyTable)
	if err == nil {
		for idxRows.Next() {
			var tbl, idxName, indexDef, indexType, whereClause string
			var isUnique, isPrimary bool
			if err := idxRows.Scan(&tbl, &idxName, &isUnique, &isPrimary, &indexDef, &indexType, &whereClause); err == nil {
				if t, ok := tablesMap[tbl]; ok {
					cols := extractColumnsFromIndexDef(indexDef)
					if whereClause == "" {
						whereClause = extractWhereFromIndexDef(indexDef)
					}
					t.Indexes = append(t.Indexes, driver.IndexMetadata{
						Name:        idxName,
						Columns:     cols,
						IsUnique:    isUnique,
						IsPrimary:   isPrimary,
						Type:        strings.ToUpper(indexType),
						WhereClause: strings.TrimSpace(whereClause),
					})
				}
			}
		}
		idxRows.Close()
	}

	// 5. Fetch Foreign Keys
	fkRows, err := s.db.QueryContext(ctx, `
		SELECT con.conname as fk_name,
		       c.relname as table_name,
		       a.attname as column_name,
		       cf.relname as ref_table,
		       af.attname as ref_column,
		       CASE con.confdeltype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' WHEN 'r' THEN 'RESTRICT' ELSE 'NO ACTION' END,
		       CASE con.confupdtype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' WHEN 'r' THEN 'RESTRICT' ELSE 'NO ACTION' END
		FROM pg_catalog.pg_constraint con
		JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_class cf ON cf.oid = con.confrelid
		-- One row per column pair so composite FKs keep every column (same Name groups them).
		CROSS JOIN LATERAL unnest(con.conkey, con.confkey) WITH ORDINALITY AS k(attnum, fattnum, pos)
		JOIN pg_catalog.pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.attnum
		JOIN pg_catalog.pg_attribute af ON af.attrelid = con.confrelid AND af.attnum = k.fattnum
		WHERE con.contype = 'f' AND n.nspname = $1
		  AND ($2 = '' OR LOWER(c.relname) = LOWER($2))
		ORDER BY con.conname, k.pos;
	`, schemaName, onlyTable)
	if err == nil {
		for fkRows.Next() {
			var fkName, tbl, col, refTbl, refCol, onDel, onUpd string
			if err := fkRows.Scan(&fkName, &tbl, &col, &refTbl, &refCol, &onDel, &onUpd); err == nil {
				if t, ok := tablesMap[tbl]; ok {
					t.ForeignKeys = append(t.ForeignKeys, driver.ForeignKeyMetadata{
						Name:             fkName,
						Column:           col,
						ReferencedTable:  refTbl,
						ReferencedColumn: refCol,
						OnDelete:         onDel,
						OnUpdate:         onUpd,
					})
				}
			}
		}
		fkRows.Close()
	}

	// 6. Fetch Triggers
	var triggers []driver.TriggerMetadata
	trigRows, err := s.db.QueryContext(ctx, `
		SELECT tgname as trigger_name,
		       relname as table_name,
		       pg_catalog.pg_get_triggerdef(t.oid, true) as trigger_def
		FROM pg_catalog.pg_trigger t
		JOIN pg_catalog.pg_class c ON c.oid = t.tgrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND NOT t.tgisinternal
		ORDER BY tgname;
	`, schemaName)
	if err == nil {
		for trigRows.Next() {
			var tgName, tbl, def string
			if err := trigRows.Scan(&tgName, &tbl, &def); err == nil {
				triggers = append(triggers, driver.TriggerMetadata{
					Name:      tgName,
					Table:     tbl,
					Timing:    "TRIGGER",
					Event:     "STATEMENT",
					Statement: def,
				})
			}
		}
		trigRows.Close()
	}

	// 7. Fetch Stored Procedures & Functions
	var routines []driver.RoutineMetadata
	procRows, err := s.db.QueryContext(ctx, `
		SELECT 
			p.proname,
			COALESCE(pg_catalog.pg_get_function_identity_arguments(p.oid), '') as args,
			COALESCE(pg_catalog.pg_get_function_result(p.oid), 'void') as res_type
		FROM pg_catalog.pg_proc p
		JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = $1
		  AND n.nspname NOT LIKE 'pg_%' 
		  AND n.nspname != 'information_schema'
		ORDER BY p.proname;
	`, schemaName)
	if err == nil {
		for procRows.Next() {
			var proname, args, resType string
			if err := procRows.Scan(&proname, &args, &resType); err == nil {
				displayName := proname
				if args != "" {
					displayName = fmt.Sprintf("%s(%s)", proname, args)
				}
				rType := "FUNCTION"
				if strings.EqualFold(resType, "void") {
					rType = "PROCEDURE"
				}
				routines = append(routines, driver.RoutineMetadata{
					Name:     displayName,
					Type:     rType,
					DataType: resType,
				})
			}
		}
		procRows.Close()
	}

	var tables []driver.TableMetadata
	for _, name := range tableOrder {
		tables = append(tables, *tablesMap[name])
	}

	return &driver.SchemaMetadata{
		DatabaseName: schemaName,
		Databases:    allSchemas,
		Tables:       tables,
		Triggers:     triggers,
		Routines:     routines,
	}, nil
}

// InspectFastCatalog retrieves lightweight tables and columns across all non-system schemas in a single fast sweep.
func (s *PostgresSession) InspectFastCatalog(ctx context.Context) (map[string][]driver.TableMetadata, error) {
	tblQuery := `
	SELECT table_schema, table_name, table_type
	FROM information_schema.tables
	WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
	  AND table_schema NOT LIKE 'pg_toast%'
	ORDER BY table_schema, table_name;
	`
	rows, err := s.db.QueryContext(ctx, tblQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect catalog tables: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]driver.TableMetadata)
	type tblRef struct {
		dbName string
		idx    int
	}
	tableMap := make(map[string]tblRef)

	for rows.Next() {
		var schemaName, tblName, tblType string
		if err := rows.Scan(&schemaName, &tblName, &tblType); err == nil {
			t := driver.TableMetadata{
				Name:        tblName,
				Type:        tblType,
				Columns:     []driver.ColumnMetadata{},
				Indexes:     []driver.IndexMetadata{},
				ForeignKeys: []driver.ForeignKeyMetadata{},
			}
			lowerSchema := strings.ToLower(schemaName)
			result[lowerSchema] = append(result[lowerSchema], t)
			tableMap[lowerSchema+"."+strings.ToLower(tblName)] = tblRef{
				dbName: lowerSchema,
				idx:    len(result[lowerSchema]) - 1,
			}
		}
	}

	colQuery := `
	SELECT table_schema, table_name, column_name, data_type, is_nullable, COALESCE(column_default, '')
	FROM information_schema.columns
	WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
	  AND table_schema NOT LIKE 'pg_toast%'
	ORDER BY table_schema, table_name, ordinal_position;
	`
	cRows, cErr := s.db.QueryContext(ctx, colQuery)
	if cErr == nil {
		defer cRows.Close()
		for cRows.Next() {
			var schemaName, tblName, colName, colType, isNullable, colDefault string
			if err := cRows.Scan(&schemaName, &tblName, &colName, &colType, &isNullable, &colDefault); err == nil {
				key := strings.ToLower(schemaName) + "." + strings.ToLower(tblName)
				if ref, ok := tableMap[key]; ok {
					result[ref.dbName][ref.idx].Columns = append(result[ref.dbName][ref.idx].Columns, driver.ColumnMetadata{
						Name:         colName,
						Type:         colType,
						IsNullable:   isNullable == "YES",
						DefaultValue: colDefault,
					})
				}
			}
		}
	}

	return result, nil
}

func (s *PostgresSession) GetTableDDL(ctx context.Context, tableName string) (string, error) {
	schemaName := driver.DatabaseFromContext(ctx)
	if schemaName == "" {
		schemaName = s.activeSchema()
	}
	if ddl, err := s.viewDDL(ctx, schemaName, tableName); err == nil {
		return ddl, nil
	}
	schema, err := s.inspectSchemaFiltered(ctx, schemaName, tableName)
	if err != nil {
		return "", err
	}

	for _, tbl := range schema.Tables {
		if strings.EqualFold(tbl.Name, tableName) {
			if tbl.Type == "VIEW" {
				return s.viewDDL(ctx, schemaName, tbl.Name)
			}
			return generatePostgresCreateDDL(schemaName, tbl), nil
		}
	}

	return "", fmt.Errorf("table %s not found in schema %s", tableName, schemaName)
}

// viewDDL rebuilds CREATE [MATERIALIZED] VIEW from the catalog (views have no column DDL).
func (s *PostgresSession) viewDDL(ctx context.Context, schemaName, viewName string) (string, error) {
	var relkind, def string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.relkind::text, pg_get_viewdef(c.oid, true)
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('v', 'm')`, schemaName, viewName).Scan(&relkind, &def)
	if err != nil {
		return "", err
	}
	kind := "VIEW"
	if relkind == "m" {
		kind = "MATERIALIZED VIEW"
	}
	return fmt.Sprintf("CREATE %s %s.%s AS\n%s", kind, quoteIdentifier(schemaName), quoteIdentifier(viewName), strings.TrimSuffix(strings.TrimSpace(def), ";")), nil
}

func (s *PostgresSession) GetTableEstimate(ctx context.Context, tableName string) (*driver.TableEstimate, error) {
	var rowCount int64
	var sizeBytes int64

	schemaName := driver.DatabaseFromContext(ctx)
	if schemaName == "" {
		schemaName = s.activeSchema()
	}
	regClass := fmt.Sprintf("%s.%s", quoteIdentifier(schemaName), quoteIdentifier(tableName))
	query := `
		SELECT COALESCE(reltuples::bigint, 0),
		       pg_total_relation_size($1::regclass)
		FROM pg_catalog.pg_class
		WHERE oid = $1::regclass;
	`
	if err := s.db.QueryRowContext(ctx, query, regClass).Scan(&rowCount, &sizeBytes); err != nil {
		// Fallback fast count
		_ = s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", regClass)).Scan(&rowCount)
		sizeBytes = rowCount * 128
	}

	if rowCount < 0 {
		rowCount = 0
	}
	return &driver.TableEstimate{
		RowCount:  rowCount,
		SizeBytes: sizeBytes,
	}, nil
}

func (s *PostgresSession) Explain(ctx context.Context, querySQL string) (*driver.ExplainPlan, error) {
	if strings.TrimSpace(querySQL) == "" {
		return &driver.ExplainPlan{Rows: []driver.ExplainRow{}}, nil
	}
	cleaned, err := driver.ExplainTarget(driver.SplitSQLStatements(querySQL, "postgres"))
	if err != nil {
		return nil, err
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		return nil, err
	}

	// Always run inside a READ ONLY transaction that is rolled back so Explain can never mutate data.
	inReadOnlyTx := func(fn func(tx *sql.Tx) error) error {
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		return fn(tx)
	}

	var rawJSON string
	// Opt-in Tier 1: EXPLAIN ANALYZE actually executes the query (can cause heavy CPU/IO/lock load
	// on Production), so it only runs when the caller explicitly opts in via WithExplainAnalyze.
	if driver.ExplainAnalyzeFromContext(ctx) && driver.IsQueryStatement(cleaned) {
		const analyzeFmt = "EXPLAIN (ANALYZE, COSTS, VERBOSE, BUFFERS, FORMAT JSON) "
		if err := inReadOnlyTx(func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, analyzeFmt+cleaned).Scan(&rawJSON)
		}); err == nil {
			return &driver.ExplainPlan{RawJSON: rawJSON, Rows: extractPGPlanRows(rawJSON)}, nil
		}
	}

	// Default Tier: Cost-only JSON plan (safe, never executes the query).
	if err := inReadOnlyTx(func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "EXPLAIN (COSTS, VERBOSE, FORMAT JSON) "+cleaned).Scan(&rawJSON)
	}); err == nil {
		return &driver.ExplainPlan{RawJSON: rawJSON, Rows: extractPGPlanRows(rawJSON)}, nil
	}

	// Fallback: Basic JSON plan
	if err := inReadOnlyTx(func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "EXPLAIN (FORMAT JSON) "+cleaned).Scan(&rawJSON)
	}); err == nil {
		return &driver.ExplainPlan{RawJSON: rawJSON, Rows: extractPGPlanRows(rawJSON)}, nil
	}

	// Text EXPLAIN fallback (ancient PG or unusual syntax)
	var planRows []driver.ExplainRow
	err = inReadOnlyTx(func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "EXPLAIN "+cleaned)
		if err != nil {
			return err
		}
		defer rows.Close()
		id := 1
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err == nil {
				planRows = append(planRows, driver.ExplainRow{ID: id, Table: line, Extra: line})
				id++
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	return &driver.ExplainPlan{Rows: planRows}, nil
}

func (s *PostgresSession) InspectTableLocks(ctx context.Context, database, table string) ([]driver.TableLockInfo, error) {
	schema := database
	if schema == "" || schema == s.cfg.DatabaseName {
		schema = s.activeSchema()
	}
	if schema == "" {
		schema = "public"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.pid,
		       COALESCE(a.usename, '') AS usr,
		       COALESCE(a.client_addr::text, 'local') AS host,
		       COALESCE(a.query, '') AS query,
		       COALESCE(a.state, '') AS state,
		       COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - a.query_start))::bigint, 0) AS time_sec,
		       COALESCE(l.mode, '') AS lock_mode
		FROM pg_catalog.pg_locks l
		JOIN pg_catalog.pg_class c ON c.oid = l.relation
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_stat_activity a ON a.pid = l.pid
		WHERE a.datname = current_database()
		  AND n.nspname = $1
		  AND c.relname = $2
		  AND a.pid != pg_backend_pid();
	`, schema, table)
	if err != nil {
		return []driver.TableLockInfo{}, nil
	}
	defer rows.Close()

	var locks []driver.TableLockInfo
	for rows.Next() {
		var pid, timeSec int64
		var user, host, query, state, lockMode string
		if err := rows.Scan(&pid, &user, &host, &query, &state, &timeSec, &lockMode); err == nil {
			locks = append(locks, driver.TableLockInfo{
				ThreadID:    pid,
				User:        user,
				Host:        host,
				Database:    schema,
				State:       state,
				Query:       query,
				DurationSec: timeSec,
				LockMode:    lockMode,
			})
		}
	}
	return locks, nil
}

func (s *PostgresSession) ExecSafeDDL(ctx context.Context, ddl string, lockTimeoutSec int) (*driver.ExecResult, error) {
	if lockTimeoutSec <= 0 {
		lockTimeoutSec = 3
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire dedicated postgres connection: %w", err)
	}
	defer conn.Close()
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		return nil, err
	}
	defer func() {
		// ponytail: Reset session lock_timeout to prevent pool state pollution
		_, _ = conn.ExecContext(context.Background(), "RESET lock_timeout")
	}()

	_, _ = conn.ExecContext(ctx, fmt.Sprintf("SET lock_timeout = '%ds'", lockTimeoutSec))

	start := time.Now()
	res, err := conn.ExecContext(ctx, ddl)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return nil, err
	}

	rowsAff, _ := res.RowsAffected()
	lastID, _ := res.LastInsertId()

	return &driver.ExecResult{
		RowsAffected: rowsAff,
		LastInsertID: lastID,
		ExecutionMs:  dur,
		Message:      "Safe DDL executed successfully on PostgreSQL",
	}, nil
}

func extractIndexParenSpan(def string) (int, int) {
	openIdx := strings.Index(def, "(")
	if openIdx == -1 {
		return -1, -1
	}
	// Walk forward from the opening '(' to find its matching ')' so trailing clauses
	// such as WHERE (...) in partial indexes are not included.
	depth := 0
	inDouble := false
	inSingle := false
	for i := openIdx; i < len(def); i++ {
		c := def[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(def) && def[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '"' {
				if i+1 < len(def) && def[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return openIdx, i
			}
		}
	}
	return openIdx, -1
}

func extractWhereFromIndexDef(def string) string {
	_, closeIdx := extractIndexParenSpan(def)
	if closeIdx == -1 || closeIdx+1 >= len(def) {
		return ""
	}
	tail := strings.TrimSpace(def[closeIdx+1:])
	upperTail := strings.ToUpper(tail)
	idx := strings.Index(upperTail, "WHERE ")
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(tail[idx+len("WHERE "):])
}

func extractColumnsFromIndexDef(def string) []string {
	openIdx, closeIdx := extractIndexParenSpan(def)
	if openIdx == -1 || closeIdx == -1 || closeIdx <= openIdx {
		return []string{}
	}
	raw := def[openIdx+1 : closeIdx]
	var cols []string
	start := 0
	depth := 0
	inDouble := false
	inSingle := false
	for i := 0; i <= len(raw); i++ {
		if i < len(raw) {
			c := raw[i]
			if inSingle {
				if c == '\'' {
					if i+1 < len(raw) && raw[i+1] == '\'' {
						i++
						continue
					}
					inSingle = false
				}
				continue
			}
			if inDouble {
				if c == '"' {
					if i+1 < len(raw) && raw[i+1] == '"' {
						i++
						continue
					}
					inDouble = false
				}
				continue
			}
			if c == '\'' {
				inSingle = true
				continue
			}
			if c == '"' {
				inDouble = true
				continue
			}
			if c == '(' {
				depth++
			} else if c == ')' && depth > 0 {
				depth--
			}
			if c != ',' || depth > 0 {
				continue
			}
		}
		c := strings.TrimSpace(raw[start:i])
		// Only strip surrounding double quotes when the entire entry is a single quoted identifier;
		// preserve expressions like lower(email) or modifiers like "order_date" DESC intact.
		if len(c) >= 2 && c[0] == '"' && c[len(c)-1] == '"' && strings.Count(c, `"`) == 2 {
			c = c[1 : len(c)-1]
		}
		if c != "" {
			cols = append(cols, c)
		}
		start = i + 1
	}
	return cols
}

func extractPGPlanRows(rawJSON string) []driver.ExplainRow {
	var parsed []struct {
		Plan struct {
			NodeType     string  `json:"Node Type"`
			RelationName string  `json:"Relation Name"`
			PlanRows     float64 `json:"Plan Rows"`
			ActualRows   float64 `json:"Actual Rows"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil || len(parsed) == 0 {
		return []driver.ExplainRow{}
	}
	p := parsed[0].Plan
	est := int64(p.PlanRows)
	if p.ActualRows > 0 {
		est = int64(p.ActualRows)
	}
	return []driver.ExplainRow{{
		ID:    1,
		Table: p.RelationName,
		Type:  p.NodeType,
		Rows:  est,
	}}
}

func isSimplePGColumnIdent(s string) bool {
	return s != "" && !strings.ContainsAny(s, " ()\t\n\"")
}

func generatePostgresCreateDDL(schema string, tbl driver.TableMetadata) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE %s.%s (\n", quoteIdentifier(schema), quoteIdentifier(tbl.Name)))

	var lines []string
	for _, col := range tbl.Columns {
		colType := col.Type
		defaultValue := col.DefaultValue
		// nextval('x_seq') defaults reference a sequence the DDL never creates; emit the equivalent
		// serial type instead so the CREATE succeeds on a fresh database (the sequence is created and owned).
		if strings.HasPrefix(strings.ToLower(defaultValue), "nextval(") {
			switch strings.ToLower(colType) {
			case "integer", "int", "int4":
				colType, defaultValue = "serial", ""
			case "bigint", "int8":
				colType, defaultValue = "bigserial", ""
			case "smallint", "int2":
				colType, defaultValue = "smallserial", ""
			}
		}
		line := fmt.Sprintf("  %s %s", quoteIdentifier(col.Name), colType)
		if !col.IsNullable {
			line += " NOT NULL"
		}
		if defaultValue != "" {
			line += " DEFAULT " + defaultValue
		}
		lines = append(lines, line)
	}

	// Primary Key: prefer the PK index's indkey column order over attnum column definition order
	// so composite primary keys like PRIMARY KEY (b, a) preserve their exact key sequence.
	var pkCols []string
	pkColMap := make(map[string]bool)
	for _, idx := range tbl.Indexes {
		if (idx.IsPrimary || strings.HasSuffix(idx.Name, "_pkey")) && len(idx.Columns) > 0 {
			for _, c := range idx.Columns {
				if isSimplePGColumnIdent(c) {
					pkCols = append(pkCols, quoteIdentifier(c))
				} else {
					pkCols = append(pkCols, c)
				}
				pkColMap[strings.ToLower(c)] = true
			}
			break
		}
	}
	if len(pkCols) == 0 {
		for _, col := range tbl.Columns {
			if col.IsPrimaryKey {
				pkCols = append(pkCols, quoteIdentifier(col.Name))
				pkColMap[strings.ToLower(col.Name)] = true
			}
		}
	}
	if len(pkCols) > 0 {
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	// Foreign Keys
	// Composite FKs arrive as one entry per column sharing the same Name; group them in order.
	var fkOrder []string
	fkGroups := map[string][]driver.ForeignKeyMetadata{}
	for _, fk := range tbl.ForeignKeys {
		if _, seen := fkGroups[fk.Name]; !seen {
			fkOrder = append(fkOrder, fk.Name)
		}
		fkGroups[fk.Name] = append(fkGroups[fk.Name], fk)
	}
	for _, name := range fkOrder {
		group := fkGroups[name]
		cols := make([]string, len(group))
		refCols := make([]string, len(group))
		for i, fk := range group {
			cols[i] = quoteIdentifier(fk.Column)
			refCols[i] = quoteIdentifier(fk.ReferencedColumn)
		}
		line := fmt.Sprintf("  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			quoteIdentifier(name), strings.Join(cols, ", "), quoteIdentifier(group[0].ReferencedTable), strings.Join(refCols, ", "))
		if r := group[0].OnDelete; r != "" && r != "NO ACTION" {
			line += " ON DELETE " + r
		}
		if r := group[0].OnUpdate; r != "" && r != "NO ACTION" {
			line += " ON UPDATE " + r
		}
		lines = append(lines, line)
	}

	sb.WriteString(strings.Join(lines, ",\n"))
	sb.WriteString("\n);")

	// Indexes
	for _, idx := range tbl.Indexes {
		if idx.IsPrimary || strings.HasSuffix(idx.Name, "_pkey") {
			continue // Skip PK indexes
		}
		if len(pkColMap) > 0 && len(idx.Columns) == len(pkColMap) && idx.IsUnique && strings.TrimSpace(idx.WhereClause) == "" && (idx.Type == "" || strings.EqualFold(idx.Type, "BTREE")) {
			allMatch := true
			for _, c := range idx.Columns {
				if !pkColMap[strings.ToLower(c)] {
					allMatch = false
					break
				}
			}
			if allMatch {
				continue
			}
		}
		uniqueStr := ""
		if idx.IsUnique {
			uniqueStr = "UNIQUE "
		}
		usingClause := ""
		if idxType := strings.ToLower(strings.TrimSpace(idx.Type)); idxType != "" && idxType != "btree" {
			// Allowlist safe identifier chars for access method name (gin, gist, hash, brin, spgist, bloom, etc.)
			if isSimplePGColumnIdent(idxType) {
				usingClause = " USING " + idxType
			}
		}
		var quotedCols []string
		for _, c := range idx.Columns {
			if isSimplePGColumnIdent(c) {
				quotedCols = append(quotedCols, quoteIdentifier(c))
			} else {
				// Expression or ordered index item (e.g. lower(email), "created_at" DESC).
				quotedCols = append(quotedCols, c)
			}
		}
		wherePart := ""
		if w := strings.TrimSpace(idx.WhereClause); w != "" {
			wherePart = " WHERE " + w
		}
		if len(quotedCols) > 0 {
			sb.WriteString(fmt.Sprintf("\nCREATE %sINDEX %s ON %s.%s%s (%s)%s;",
				uniqueStr,
				quoteIdentifier(idx.Name),
				quoteIdentifier(schema),
				quoteIdentifier(tbl.Name),
				usingClause,
				strings.Join(quotedCols, ", "),
				wherePart,
			))
		}
	}

	return sb.String()
}

// InspectServerVariables introspects all PostgreSQL settings from pg_settings and applies curated DBA tuning rules.

func parseRoutineBaseName(name string) string {
	baseName := name
	if idx := strings.Index(name, "("); idx > 0 {
		baseName = strings.TrimSpace(name[:idx])
	}
	return baseName
}

func (s *PostgresSession) GetRoutineDDL(ctx context.Context, routineType, name string) (string, error) {
	// Parse base function name if signature was included e.g. "calc_tax(numeric, integer)"
	baseName := parseRoutineBaseName(name)
	var ddl string
	err := s.db.QueryRowContext(ctx, `
		SELECT pg_catalog.pg_get_functiondef(p.oid)
		FROM pg_catalog.pg_proc p
		JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = $1 
		  AND (p.proname = $2 OR (p.proname || '(' || COALESCE(pg_catalog.pg_get_function_identity_arguments(p.oid), '') || ')') = $3)
		LIMIT 1
	`, s.activeSchema(), baseName, name).Scan(&ddl)
	if err != nil {
		return "", fmt.Errorf("routine %s not found: %w", name, err)
	}
	return ddl + ";", nil
}

func (s *PostgresSession) GetTablePartitions(ctx context.Context, database, table string) ([]driver.PartitionMetadata, error) {
	return []driver.PartitionMetadata{}, nil
}
