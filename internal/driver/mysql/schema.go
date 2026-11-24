package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"strings"
	"time"
)

func (s *MySQLSession) InspectSchema(ctx context.Context, dbName string) (*driver.SchemaMetadata, error) {
	// 0. Discover databases on MySQL server
	var databases []string
	dbRows, err := s.db.QueryContext(ctx, `
		SELECT SCHEMA_NAME 
		FROM information_schema.SCHEMATA 
		ORDER BY 
			CASE WHEN SCHEMA_NAME IN ('information_schema', 'mysql', 'performance_schema', 'sys') THEN 1 ELSE 0 END,
			SCHEMA_NAME ASC;
	`)
	if err == nil {
		for dbRows.Next() {
			var dName string
			if err := dbRows.Scan(&dName); err == nil {
				databases = append(databases, dName)
			}
		}
		dbRows.Close()
	} else {
		// Fallback to SHOW DATABASES
		if fallbackRows, fErr := s.db.QueryContext(ctx, "SHOW DATABASES"); fErr == nil {
			for fallbackRows.Next() {
				var dName string
				if err := fallbackRows.Scan(&dName); err == nil {
					databases = append(databases, dName)
				}
			}
			fallbackRows.Close()
		}
	}

	if dbName == "" {
		dbName = s.activeDB()
	}
	if dbName == "" {
		_ = s.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&dbName)
	}
	// If still empty and databases were discovered, pick the first user database
	if dbName == "" && len(databases) > 0 {
		for _, d := range databases {
			if d != "information_schema" && d != "mysql" && d != "performance_schema" && d != "sys" {
				dbName = d
				break
			}
		}
		if dbName == "" {
			dbName = databases[0]
		}
		_ = s.SelectDatabase(ctx, dbName)
	}

	// 1. Fetch tables & views
	tablesQuery := `
	SELECT TABLE_NAME, TABLE_TYPE, IFNULL(TABLE_ROWS, 0)
	FROM information_schema.TABLES
	WHERE TABLE_SCHEMA = ?
	ORDER BY TABLE_NAME ASC;
	`
	rows, err := s.db.QueryContext(ctx, tablesQuery, dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect tables: %w", err)
	}
	defer rows.Close()

	var tables []driver.TableMetadata
	// ponytail: slice index mapping avoids pointer invalidation when tables slice dynamically reallocates
	tableIndexMap := make(map[string]int)

	for rows.Next() {
		var name, tableType string
		var rowCount int64
		if err := rows.Scan(&name, &tableType, &rowCount); err != nil {
			return nil, err
		}

		tbl := driver.TableMetadata{
			Name:        name,
			Type:        tableType,
			RowCount:    rowCount,
			Columns:     []driver.ColumnMetadata{},
			Indexes:     []driver.IndexMetadata{},
			ForeignKeys: []driver.ForeignKeyMetadata{},
		}
		tables = append(tables, tbl)
		tableIndexMap[name] = len(tables) - 1
	}

	// 2. Fetch Columns
	colsQuery := `
	SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, IFNULL(COLUMN_DEFAULT, ''), EXTRA
	FROM information_schema.COLUMNS
	WHERE TABLE_SCHEMA = ?
	ORDER BY TABLE_NAME, ORDINAL_POSITION;
	`
	colRows, err := s.db.QueryContext(ctx, colsQuery, dbName)
	if err == nil {
		for colRows.Next() {
			var tblName, colName, colType, isNullable, colKey, colDefault, extra string
			if err := colRows.Scan(&tblName, &colName, &colType, &isNullable, &colKey, &colDefault, &extra); err == nil {
				if idx, ok := tableIndexMap[tblName]; ok {
					tables[idx].Columns = append(tables[idx].Columns, driver.ColumnMetadata{
						Name:         colName,
						Type:         colType,
						IsNullable:   isNullable == "YES",
						IsPrimaryKey: colKey == "PRI",
						DefaultValue: colDefault,
						Extra:        extra,
					})
				}
			}
		}
		colRows.Close()
	}

	// 3. Fetch Indexes
	idxQuery := `
	SELECT TABLE_NAME, INDEX_NAME, COLUMN_NAME, NON_UNIQUE, INDEX_TYPE
	FROM information_schema.STATISTICS
	WHERE TABLE_SCHEMA = ?
	ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX;
	`
	idxRows, err := s.db.QueryContext(ctx, idxQuery, dbName)
	if err == nil {
		type idxKey struct {
			table string
			index string
		}
		indexAgg := make(map[idxKey]*driver.IndexMetadata)
		var indexOrder []idxKey

		for idxRows.Next() {
			var tblName, idxName, colName, idxType string
			var nonUnique int
			if err := idxRows.Scan(&tblName, &idxName, &colName, &nonUnique, &idxType); err == nil {
				k := idxKey{table: tblName, index: idxName}
				if existing, exists := indexAgg[k]; exists {
					existing.Columns = append(existing.Columns, colName)
				} else {
					im := &driver.IndexMetadata{
						Name:     idxName,
						Columns:  []string{colName},
						IsUnique: nonUnique == 0,
						Type:     idxType,
					}
					indexAgg[k] = im
					indexOrder = append(indexOrder, k)
				}
			}
		}
		idxRows.Close()

		for _, k := range indexOrder {
			if im, ok := indexAgg[k]; ok {
				if idx, ok := tableIndexMap[k.table]; ok {
					tables[idx].Indexes = append(tables[idx].Indexes, *im)
				}
			}
		}
	}

	// 4. Fetch Triggers
	var triggers []driver.TriggerMetadata
	trigQuery := `
	SELECT TRIGGER_NAME, EVENT_OBJECT_TABLE, ACTION_TIMING, EVENT_MANIPULATION, ACTION_STATEMENT
	FROM information_schema.TRIGGERS
	WHERE TRIGGER_SCHEMA = ?
	ORDER BY TRIGGER_NAME ASC;
	`
	trigRows, err := s.db.QueryContext(ctx, trigQuery, dbName)
	if err == nil {
		for trigRows.Next() {
			var name, tbl, timing, event, stmt string
			if err := trigRows.Scan(&name, &tbl, &timing, &event, &stmt); err == nil {
				triggers = append(triggers, driver.TriggerMetadata{
					Name:      name,
					Table:     tbl,
					Timing:    timing,
					Event:     event,
					Statement: stmt,
				})
			}
		}
		trigRows.Close()
	}

	// 5. Fetch Foreign Keys
	// ponytail: introspect foreign key constraints via KEY_COLUMN_USAGE & REFERENTIAL_CONSTRAINTS
	fkQuery := `
	SELECT 
		kcu.CONSTRAINT_NAME,
		kcu.TABLE_NAME,
		kcu.COLUMN_NAME,
		kcu.REFERENCED_TABLE_NAME,
		kcu.REFERENCED_COLUMN_NAME,
		IFNULL(rc.UPDATE_RULE, ''),
		IFNULL(rc.DELETE_RULE, '')
	FROM information_schema.KEY_COLUMN_USAGE kcu
	LEFT JOIN information_schema.REFERENTIAL_CONSTRAINTS rc
		ON rc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
		AND rc.CONSTRAINT_SCHEMA = kcu.TABLE_SCHEMA
	WHERE kcu.TABLE_SCHEMA = ?
		AND kcu.REFERENCED_TABLE_NAME IS NOT NULL
	ORDER BY kcu.TABLE_NAME, kcu.ORDINAL_POSITION;
	`
	fkRows, err := s.db.QueryContext(ctx, fkQuery, dbName)
	if err != nil {
		fkFallback := `
		SELECT 
			CONSTRAINT_NAME,
			TABLE_NAME,
			COLUMN_NAME,
			REFERENCED_TABLE_NAME,
			REFERENCED_COLUMN_NAME,
			'',
			''
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = ?
			AND REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY TABLE_NAME, ORDINAL_POSITION;
		`
		fkRows, err = s.db.QueryContext(ctx, fkFallback, dbName)
	}
	if err == nil {
		for fkRows.Next() {
			var cName, tblName, colName, refTbl, refCol, onUpdate, onDelete string
			if err := fkRows.Scan(&cName, &tblName, &colName, &refTbl, &refCol, &onUpdate, &onDelete); err == nil {
				if idx, ok := tableIndexMap[tblName]; ok {
					tables[idx].ForeignKeys = append(tables[idx].ForeignKeys, driver.ForeignKeyMetadata{
						Name:             cName,
						Column:           colName,
						ReferencedTable:  refTbl,
						ReferencedColumn: refCol,
						OnUpdate:         onUpdate,
						OnDelete:         onDelete,
					})
				}
			}
		}
		fkRows.Close()
	}

	// 6. Fetch Stored Routines (Procedures and Functions)
	var routines []driver.RoutineMetadata
	routineQuery := `
	SELECT ROUTINE_NAME, ROUTINE_TYPE, IFNULL(DTD_IDENTIFIER, ''), IFNULL(ROUTINE_DEFINITION, '')
	FROM information_schema.ROUTINES
	WHERE ROUTINE_SCHEMA = ?
	ORDER BY ROUTINE_NAME ASC;
	`
	rRows, err := s.db.QueryContext(ctx, routineQuery, dbName)
	if err == nil {
		for rRows.Next() {
			var rName, rType, dtd, rDef string
			if err := rRows.Scan(&rName, &rType, &dtd, &rDef); err == nil {
				routines = append(routines, driver.RoutineMetadata{
					Name:       rName,
					Type:       rType,
					DataType:   dtd,
					Definition: rDef,
				})
			}
		}
		rRows.Close()
	}

	// 7. Fetch Parameters for routines (to help smart caller)
	if len(routines) > 0 {
		paramQuery := `
		SELECT SPECIFIC_NAME, ROUTINE_TYPE, IFNULL(PARAMETER_NAME, ''), IFNULL(PARAMETER_MODE, 'IN'), IFNULL(DTD_IDENTIFIER, '')
		FROM information_schema.PARAMETERS
		WHERE SPECIFIC_SCHEMA = ?
		ORDER BY SPECIFIC_NAME, ORDINAL_POSITION;
		`
		pRows, pErr := s.db.QueryContext(ctx, paramQuery, dbName)
		if pErr == nil {
			rMap := make(map[string]int)
			for i, r := range routines {
				rMap[r.Name] = i
			}
			for pRows.Next() {
				var sName, rType, pName, pMode, dtd string
				if err := pRows.Scan(&sName, &rType, &pName, &pMode, &dtd); err == nil {
					if idx, ok := rMap[sName]; ok {
						if pName != "" {
							routines[idx].Parameters = append(routines[idx].Parameters, driver.RoutineParameter{
								Name: pName,
								Mode: pMode,
								Type: dtd,
							})
						}
					}
				}
			}
			pRows.Close()
		}
	}

	// 8. Fetch Scheduled Events
	var events []driver.EventMetadata
	eventQuery := `
	SELECT EVENT_NAME, STATUS, IFNULL(INTERVAL_VALUE, ''), IFNULL(INTERVAL_FIELD, ''), IFNULL(STARTS, ''), IFNULL(ENDS, ''), IFNULL(EVENT_DEFINITION, '')
	FROM information_schema.EVENTS
	WHERE EVENT_SCHEMA = ?
	ORDER BY EVENT_NAME ASC;
	`
	eRows, eErr := s.db.QueryContext(ctx, eventQuery, dbName)
	if eErr == nil {
		for eRows.Next() {
			var eName, eStatus, intVal, intField, starts, ends, eDef string
			if err := eRows.Scan(&eName, &eStatus, &intVal, &intField, &starts, &ends, &eDef); err == nil {
				intervalStr := ""
				if intVal != "" && intField != "" {
					intervalStr = fmt.Sprintf("EVERY %s %s", intVal, intField)
				}
				events = append(events, driver.EventMetadata{
					Name:       eName,
					Status:     eStatus,
					Interval:   intervalStr,
					StartsAt:   starts,
					EndsAt:     ends,
					Definition: eDef,
				})
			}
		}
		eRows.Close()
	}

	return &driver.SchemaMetadata{
		DatabaseName: dbName,
		Databases:    databases,
		Tables:       tables,
		Triggers:     triggers,
		Routines:     routines,
		Events:       events,
	}, nil
}

// InspectFastCatalog retrieves lightweight tables and columns across all non-system databases in a single fast sweep.
// ponytail: eliminates O(N) information_schema.KEY_COLUMN_USAGE foreign key joins that flood processlist on session switches
func (s *MySQLSession) InspectFastCatalog(ctx context.Context) (map[string][]driver.TableMetadata, error) {
	// Query all user tables
	tblQuery := `
	SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_TYPE, IFNULL(TABLE_ROWS, 0)
	FROM information_schema.TABLES
	WHERE TABLE_SCHEMA NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys')
	ORDER BY TABLE_SCHEMA, TABLE_NAME;
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
	tableMap := make(map[string]tblRef) // key: db.table

	for rows.Next() {
		var schemaName, tblName, tblType string
		var rowCount int64
		if err := rows.Scan(&schemaName, &tblName, &tblType, &rowCount); err == nil {
			t := driver.TableMetadata{
				Name:        tblName,
				Type:        tblType,
				RowCount:    rowCount,
				Columns:     []driver.ColumnMetadata{},
				Indexes:     []driver.IndexMetadata{},
				ForeignKeys: []driver.ForeignKeyMetadata{},
			}
			lowerDB := strings.ToLower(schemaName)
			result[lowerDB] = append(result[lowerDB], t)
			tableMap[lowerDB+"."+strings.ToLower(tblName)] = tblRef{
				dbName: lowerDB,
				idx:    len(result[lowerDB]) - 1,
			}
		}
	}

	// Query columns across user tables in single pass
	colQuery := `
	SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, IFNULL(COLUMN_DEFAULT, ''), EXTRA
	FROM information_schema.COLUMNS
	WHERE TABLE_SCHEMA NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys')
	ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION;
	`
	cRows, cErr := s.db.QueryContext(ctx, colQuery)
	if cErr == nil {
		defer cRows.Close()
		for cRows.Next() {
			var schemaName, tblName, colName, colType, isNullable, colKey, colDefault, extra string
			if err := cRows.Scan(&schemaName, &tblName, &colName, &colType, &isNullable, &colKey, &colDefault, &extra); err == nil {
				key := strings.ToLower(schemaName) + "." + strings.ToLower(tblName)
				if ref, ok := tableMap[key]; ok {
					result[ref.dbName][ref.idx].Columns = append(result[ref.dbName][ref.idx].Columns, driver.ColumnMetadata{
						Name:         colName,
						Type:         colType,
						IsNullable:   isNullable == "YES",
						IsPrimaryKey: colKey == "PRI",
						DefaultValue: colDefault,
						Extra:        extra,
					})
				}
			}
		}
	}

	return result, nil
}

func (s *MySQLSession) SelectDatabase(ctx context.Context, dbName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dbName == "" {
		return nil
	}

	escaped := strings.ReplaceAll(dbName, "`", "``")
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("USE `%s`", escaped)); err != nil {
		return fmt.Errorf("failed to select database %s: %w", dbName, err)
	}
	s.cfg.DatabaseName = dbName
	return nil
}

func (s *MySQLSession) GetTableDDL(ctx context.Context, tableName string) (string, error) {
	var tbl, ddl string
	escapedTbl := strings.ReplaceAll(tableName, "`", "``")
	var query string
	if dbName := s.activeDB(); dbName != "" {
		escapedDB := strings.ReplaceAll(dbName, "`", "``")
		query = fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`", escapedDB, escapedTbl)
	} else {
		query = fmt.Sprintf("SHOW CREATE TABLE `%s`", escapedTbl)
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "", sql.ErrNoRows
	}
	// Views return 4 columns (View, Create View, character_set_client, collation_connection).
	dest := []any{&tbl, &ddl}
	for i := 2; i < len(cols); i++ {
		dest = append(dest, new(sql.RawBytes))
	}
	if err := rows.Scan(dest...); err != nil {
		return "", err
	}
	return ddl, nil
}

func (s *MySQLSession) GetTableEstimate(ctx context.Context, tableName string) (*driver.TableEstimate, error) {
	var rowCount, sizeBytes sql.NullInt64
	query := `
	SELECT TABLE_ROWS, (DATA_LENGTH + INDEX_LENGTH)
	FROM information_schema.TABLES
	WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?;
	`
	err := s.db.QueryRowContext(ctx, query, s.activeDB(), tableName).Scan(&rowCount, &sizeBytes)
	if err != nil {
		return &driver.TableEstimate{RowCount: 0, SizeBytes: 0}, nil
	}
	return &driver.TableEstimate{
		RowCount:  rowCount.Int64,
		SizeBytes: sizeBytes.Int64,
	}, nil
}

func (s *MySQLSession) Explain(ctx context.Context, querySQL string) (*driver.ExplainPlan, error) {
	cleanedSQL := strings.TrimSpace(querySQL)
	cleanedSQL = strings.TrimRight(cleanedSQL, "; \t\r\n")
	if strings.HasPrefix(strings.ToUpper(cleanedSQL), "EXPLAIN ") {
		cleanedSQL = strings.TrimSpace(cleanedSQL[8:])
	}
	if cleanedSQL == "" {
		return &driver.ExplainPlan{Rows: []driver.ExplainRow{}}, nil
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return nil, err
	}

	// Classic tabular EXPLAIN (always supported)
	rows, err := conn.QueryContext(ctx, "EXPLAIN "+cleanedSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	planRows, err := driver.ScanExplainRows(rows)
	if err != nil {
		return nil, err
	}

	// Best-effort JSON plan for visual tree (MySQL 5.6+, ignored on older/MariaDB)
	// ponytail: single-row scan; FORMAT=JSON not supported everywhere, so we silently drop errors.
	var rawJSON string
	_ = conn.QueryRowContext(ctx, "EXPLAIN FORMAT=JSON "+cleanedSQL).Scan(&rawJSON)

	return &driver.ExplainPlan{
		RawJSON: rawJSON,
		Rows:    planRows,
	}, nil
}

func (s *MySQLSession) InspectTableLocks(ctx context.Context, database, table string) ([]driver.TableLockInfo, error) {
	if database == "" {
		database = s.activeDB()
	}
	var locks []driver.TableLockInfo

	// 1. Try performance_schema.metadata_locks if available (MySQL 5.7+ / 8.0)
	queryPfs := `
		SELECT 
			p.ID, 
			IFNULL(p.USER, ''), 
			IFNULL(p.HOST, ''), 
			IFNULL(p.DB, ''), 
			IFNULL(p.INFO, ''), 
			IFNULL(p.TIME, 0), 
			IFNULL(p.STATE, ''), 
			IFNULL(m.LOCK_TYPE, '')
		FROM performance_schema.metadata_locks m
		JOIN performance_schema.threads pt ON m.OWNER_THREAD_ID = pt.THREAD_ID
		JOIN information_schema.processlist p ON pt.PROCESSLIST_ID = p.ID
		WHERE m.OBJECT_SCHEMA = ? AND m.OBJECT_NAME = ?
	`
	if rows, err := s.db.QueryContext(ctx, queryPfs, database, table); err == nil {
		for rows.Next() {
			var l driver.TableLockInfo
			if err := rows.Scan(&l.ThreadID, &l.User, &l.Host, &l.Database, &l.Query, &l.DurationSec, &l.State, &l.LockMode); err == nil {
				locks = append(locks, l)
			}
		}
		rows.Close()
	}

	// 2. Query information_schema.innodb_trx joined with processlist
	queryTrx := `
		SELECT 
			t.trx_mysql_thread_id, 
			IFNULL(p.user, ''), 
			IFNULL(p.host, ''), 
			IFNULL(p.db, ''), 
			IFNULL(COALESCE(p.info, t.trx_query), ''), 
			IFNULL(TIMESTAMPDIFF(SECOND, t.trx_started, NOW()), 0), 
			IFNULL(t.trx_state, ''), 
			IFNULL(t.trx_operation_state, '')
		FROM information_schema.innodb_trx t
		LEFT JOIN information_schema.processlist p ON t.trx_mysql_thread_id = p.id
		WHERE (p.info LIKE ? OR t.trx_query LIKE ? OR ? = '')
	`
	tblPattern := "%" + table + "%"
	if rows, err := s.db.QueryContext(ctx, queryTrx, tblPattern, tblPattern, table); err == nil {
		seen := make(map[int64]bool)
		for _, l := range locks {
			seen[l.ThreadID] = true
		}
		for rows.Next() {
			var l driver.TableLockInfo
			if err := rows.Scan(&l.ThreadID, &l.User, &l.Host, &l.Database, &l.Query, &l.DurationSec, &l.State, &l.LockMode); err == nil {
				if !seen[l.ThreadID] {
					locks = append(locks, l)
					seen[l.ThreadID] = true
				}
			}
		}
		rows.Close()
	}

	if locks == nil {
		locks = []driver.TableLockInfo{}
	}
	return locks, nil
}

func (s *MySQLSession) ExecSafeDDL(ctx context.Context, ddl string, lockTimeoutSec int) (*driver.ExecResult, error) {
	if lockTimeoutSec <= 0 {
		lockTimeoutSec = 3
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain dedicated connection: %w", err)
	}
	defer conn.Close()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return nil, err
	}
	defer func() {
		// ponytail: Reset session lock_wait_timeout back to DEFAULT before returning connection to pool
		_, _ = conn.ExecContext(context.Background(), "SET SESSION lock_wait_timeout = DEFAULT")
	}()

	// Enforce fail-fast session lock timeout on dedicated connection
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("SET SESSION lock_wait_timeout = %d", lockTimeoutSec)); err != nil {
		return nil, fmt.Errorf("failed to set fail-fast lock_wait_timeout: %w", err)
	}

	start := time.Now()
	res, err := conn.ExecContext(ctx, ddl)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return nil, err
	}

	rowsAff, _ := res.RowsAffected()
	lastID, _ := res.LastInsertId()
	return &driver.ExecResult{
		RowsAffected: rowsAff,
		LastInsertID: lastID,
		ExecutionMs:  duration,
		Message:      "Safe DDL executed successfully with fail-fast timeout",
	}, nil
}

// InspectClusterHealth inspects Galera wsrep cluster state, peer nodes, and Tri-Metric bottleneck indicators.

func (s *MySQLSession) GetRoutineDDL(ctx context.Context, routineType, name string) (string, error) {
	upperType := strings.ToUpper(strings.TrimSpace(routineType))
	if upperType != "FUNCTION" {
		upperType = "PROCEDURE"
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return "", err
	}
	query := fmt.Sprintf("SHOW CREATE %s `%s`", upperType, strings.ReplaceAll(name, "`", "``"))
	var rName, sqlMode, createStmt, charSet, coll, dbColl string
	row := conn.QueryRowContext(ctx, query)
	// SHOW CREATE PROCEDURE returns 6 columns: Procedure, sql_mode, Create Procedure, character_set_client, collation_connection, Database Collation
	if err := row.Scan(&rName, &sqlMode, &createStmt, &charSet, &coll, &dbColl); err != nil {
		return "", fmt.Errorf("failed to get %s DDL: %w", upperType, err)
	}
	return createStmt, nil
}

// GetEventDDL returns the raw CREATE statement for a Scheduled Event.
func (s *MySQLSession) GetEventDDL(ctx context.Context, name string) (string, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return "", err
	}
	query := fmt.Sprintf("SHOW CREATE EVENT `%s`", strings.ReplaceAll(name, "`", "``"))
	var eName, sqlMode, tz, createStmt, charSet, coll, dbColl string
	row := conn.QueryRowContext(ctx, query)
	if err := row.Scan(&eName, &sqlMode, &tz, &createStmt, &charSet, &coll, &dbColl); err != nil {
		return "", fmt.Errorf("failed to get EVENT DDL: %w", err)
	}
	return createStmt, nil
}

// ToggleEventStatus enables or disables a MySQL scheduled event.
func (s *MySQLSession) ToggleEventStatus(ctx context.Context, name string, enable bool) error {
	status := "DISABLE"
	if enable {
		status = "ENABLE"
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return err
	}
	query := fmt.Sprintf("ALTER EVENT `%s` %s", strings.ReplaceAll(name, "`", "``"), status)
	if _, err := conn.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to %s event %s: %w", status, name, err)
	}
	return nil
}

// GetTablePartitions returns partition telemetry from information_schema.partitions.
func (s *MySQLSession) GetTablePartitions(ctx context.Context, database, table string) ([]driver.PartitionMetadata, error) {
	if database == "" {
		database = s.activeDB()
	}
	query := `
		SELECT 
			IFNULL(PARTITION_NAME, '') AS p_name,
			IFNULL(PARTITION_METHOD, '') AS p_method,
			IFNULL(PARTITION_EXPRESSION, '') AS p_expr,
			IFNULL(PARTITION_DESCRIPTION, '') AS p_desc,
			IFNULL(TABLE_ROWS, 0) AS p_rows,
			IFNULL(DATA_LENGTH, 0) AS p_data,
			IFNULL(INDEX_LENGTH, 0) AS p_index
		FROM information_schema.PARTITIONS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND PARTITION_NAME IS NOT NULL
		ORDER BY PARTITION_ORDINAL_POSITION ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, database, table)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect table partitions: %w", err)
	}
	defer rows.Close()

	var partitions []driver.PartitionMetadata
	for rows.Next() {
		var p driver.PartitionMetadata
		if err := rows.Scan(&p.Name, &p.Method, &p.Expression, &p.Description, &p.TableRows, &p.DataLength, &p.IndexLength); err == nil {
			partitions = append(partitions, p)
		}
	}
	return partitions, nil
}
