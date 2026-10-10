package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sqlfox/internal/driver"

	_ "modernc.org/sqlite"
)

func init() {
	driver.Register("sqlite", &SQLiteDriver{})
}

type SQLiteDriver struct{}

func (d *SQLiteDriver) Name() string {
	return "sqlite"
}

func (d *SQLiteDriver) Connect(ctx context.Context, cfg driver.Config) (driver.Session, error) {
	dsn := cfg.DatabaseName
	if dsn == "" {
		dsn = cfg.Host // Fallback to host if user put path there
	}
	if dsn == "" {
		return nil, fmt.Errorf("sqlite database path cannot be empty")
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	return &SQLiteSession{
		db:      db,
		dsn:     dsn,
		queries: make(map[string]context.CancelFunc),
	}, nil
}

type SQLiteSession struct {
	db      *sql.DB
	dsn     string
	mu      sync.Mutex
	queries map[string]context.CancelFunc
}

func (s *SQLiteSession) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	err := s.db.PingContext(ctx)
	return time.Since(start), err
}

func (s *SQLiteSession) ServerInfo(ctx context.Context) (*driver.ServerInfo, error) {
	var version string
	if err := s.db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		return nil, err
	}
	return &driver.ServerInfo{
		Version:     "SQLite " + version,
		UptimeSec:   0,
		ActiveConns: 1,
	}, nil
}

func (s *SQLiteSession) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
	// Apply implicit safe limit if query is a SELECT without LIMIT
	trimmed := strings.TrimSpace(querySQL)
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "SELECT") && !strings.Contains(upper, "LIMIT") && limit > 0 {
		querySQL = fmt.Sprintf("%s LIMIT %d", strings.TrimSuffix(trimmed, ";"), limit)
	}

	queryID := driver.QueryIDFromContext(ctx)
	if queryID != "" {
		ctx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.queries[queryID] = cancel
		s.mu.Unlock()
		cleanup := func() {
			s.mu.Lock()
			delete(s.queries, queryID)
			s.mu.Unlock()
			cancel()
		}
		rows, err := s.db.QueryContext(ctx, querySQL)
		if err != nil {
			cleanup()
			return nil, err
		}
		colTypes, err := rows.ColumnTypes()
		if err != nil {
			rows.Close()
			cleanup()
			return nil, err
		}
		cols := make([]driver.ColumnDefinition, len(colTypes))
		for i, ct := range colTypes {
			nullable, _ := ct.Nullable()
			cols[i] = driver.ColumnDefinition{
				Name:     ct.Name(),
				Type:     ct.DatabaseTypeName(),
				Nullable: nullable,
			}
		}
		return driver.NewSQLRowStream(rows, cols, cleanup), nil
	}

	rows, err := s.db.QueryContext(ctx, querySQL)
	if err != nil {
		return nil, err
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		rows.Close()
		return nil, err
	}

	cols := make([]driver.ColumnDefinition, len(colTypes))
	for i, ct := range colTypes {
		nullable, _ := ct.Nullable()
		cols[i] = driver.ColumnDefinition{
			Name:     ct.Name(),
			Type:     ct.DatabaseTypeName(),
			Nullable: nullable,
		}
	}

	return driver.NewSQLRowStream(rows, cols), nil
}

func (s *SQLiteSession) Exec(ctx context.Context, querySQL string, args ...any) (*driver.ExecResult, error) {
	start := time.Now()
	res, err := s.db.ExecContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
	}

	rowsAffected, _ := res.RowsAffected()
	lastInsertID, _ := res.LastInsertId()

	return &driver.ExecResult{
		RowsAffected: rowsAffected,
		LastInsertID: lastInsertID,
		ExecutionMs:  time.Since(start).Milliseconds(),
		Message:      fmt.Sprintf("Query executed successfully. %d rows affected.", rowsAffected),
	}, nil
}

type sqliteTx struct {
	tx *sql.Tx
}

func (t *sqliteTx) Exec(ctx context.Context, query string, args ...any) (*driver.ExecResult, error) {
	start := time.Now()
	res, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	ra, _ := res.RowsAffected()
	li, _ := res.LastInsertId()
	return &driver.ExecResult{
		RowsAffected: ra,
		LastInsertID: li,
		ExecutionMs:  time.Since(start).Milliseconds(),
		Message:      fmt.Sprintf("Query executed successfully. %d rows affected.", ra),
	}, nil
}

func (t *sqliteTx) ExecRaw(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (t *sqliteTx) Commit() error   { return t.tx.Commit() }
func (t *sqliteTx) Rollback() error { return t.tx.Rollback() }

func (t *sqliteTx) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
	trimmed := strings.TrimSpace(querySQL)
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "SELECT") && !strings.Contains(upper, "LIMIT") && limit > 0 {
		querySQL = fmt.Sprintf("%s LIMIT %d", strings.TrimSuffix(trimmed, ";"), limit)
	}

	rows, err := t.tx.QueryContext(ctx, querySQL)
	if err != nil {
		return nil, err
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		rows.Close()
		return nil, err
	}

	cols := make([]driver.ColumnDefinition, len(colTypes))
	for i, ct := range colTypes {
		nullable, _ := ct.Nullable()
		cols[i] = driver.ColumnDefinition{
			Name:     ct.Name(),
			Type:     ct.DatabaseTypeName(),
			Nullable: nullable,
		}
	}

	return driver.NewSQLRowStream(rows, cols), nil
}

func (s *SQLiteSession) BeginTx(ctx context.Context) (driver.TxSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx: tx}, nil
}

func (s *SQLiteSession) InspectSchema(ctx context.Context, dbName string) (*driver.SchemaMetadata, error) {
	// In SQLite, dbName is typically 'main'
	tablesQuery := `
	SELECT name, type FROM sqlite_master 
	WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
	ORDER BY name ASC;
	`
	rows, err := s.db.QueryContext(ctx, tablesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []driver.TableMetadata
	for rows.Next() {
		var name, tableType string
		if err := rows.Scan(&name, &tableType); err != nil {
			return nil, err
		}

		tbl := driver.TableMetadata{
			Name: name,
			Type: strings.ToUpper(tableType),
		}

		// Inspect Columns via PRAGMA table_info
		colRows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(name)))
		if err == nil {
			for colRows.Next() {
				var cid, notnull, pk int
				var colName, colType string
				var dfltValue sql.NullString
				if err := colRows.Scan(&cid, &colName, &colType, &notnull, &dfltValue, &pk); err == nil {
					tbl.Columns = append(tbl.Columns, driver.ColumnMetadata{
						Name:         colName,
						Type:         colType,
						IsNullable:   notnull == 0,
						IsPrimaryKey: pk > 0,
						DefaultValue: dfltValue.String,
					})
				}
			}
			colRows.Close()
		}

		// Inspect Indexes via PRAGMA index_list
		idxRows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", quoteIdentifier(name)))
		if err == nil {
			for idxRows.Next() {
				var seq, unique int
				var idxName, origin, partial string
				if err := idxRows.Scan(&seq, &idxName, &unique, &origin, &partial); err == nil {
					im := driver.IndexMetadata{
						Name:     idxName,
						IsUnique: unique == 1,
						Type:     "BTREE",
					}
					// ponytail: fetch index column list via PRAGMA index_info; bounded by index count per table
					if cRows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%s)", quoteIdentifier(idxName))); err == nil {
						for cRows.Next() {
							var seqno, cid int
							var cname string
							if err := cRows.Scan(&seqno, &cid, &cname); err == nil && cname != "" {
								im.Columns = append(im.Columns, cname)
							}
						}
						cRows.Close()
					}
					tbl.Indexes = append(tbl.Indexes, im)
				}
			}
			idxRows.Close()
		}

		// Inspect foreign keys
		fkRows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", quoteIdentifier(name)))
		if err == nil {
			for fkRows.Next() {
				var id, seq int
				var refTable, fromCol, toCol, onUpdate, onDelete, match string
				if err := fkRows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err == nil {
					tbl.ForeignKeys = append(tbl.ForeignKeys, driver.ForeignKeyMetadata{
						Name:             fmt.Sprintf("fk_%s_%s", name, fromCol),
						Column:           fromCol,
						ReferencedTable:  refTable,
						ReferencedColumn: toCol,
						OnDelete:         onDelete,
						OnUpdate:         onUpdate,
					})
				}
			}
			fkRows.Close()
		}

		tables = append(tables, tbl)
	}

	// Inspect Triggers via sqlite_master
	var triggers []driver.TriggerMetadata
	trigRows, err := s.db.QueryContext(ctx, `
		SELECT name, tbl_name, IFNULL(sql, '') 
		FROM sqlite_master 
		WHERE type = 'trigger' AND name NOT LIKE 'sqlite_%'
		ORDER BY name ASC;
	`)
	if err == nil {
		defer trigRows.Close()
		for trigRows.Next() {
			var tName, tblName, tSQL string
			if err := trigRows.Scan(&tName, &tblName, &tSQL); err == nil {
				timing := "AFTER"
				event := "INSERT"
				upperSQL := strings.ToUpper(tSQL)
				if strings.Contains(upperSQL, "BEFORE") {
					timing = "BEFORE"
				} else if strings.Contains(upperSQL, "INSTEAD OF") {
					timing = "INSTEAD OF"
				}
				if strings.Contains(upperSQL, "DELETE") {
					event = "DELETE"
				} else if strings.Contains(upperSQL, "UPDATE") {
					event = "UPDATE"
				}
				triggers = append(triggers, driver.TriggerMetadata{
					Name:      tName,
					Table:     tblName,
					Timing:    timing,
					Event:     event,
					Statement: tSQL,
				})
			}
		}
	}

	return &driver.SchemaMetadata{
		DatabaseName: "main",
		Databases:    []string{"main"},
		Tables:       tables,
		Triggers:     triggers,
	}, nil
}

func (s *SQLiteSession) SelectDatabase(ctx context.Context, dbName string) error {
	return nil
}

func (s *SQLiteSession) GetTableDDL(ctx context.Context, tableName string) (string, error) {
	var sqlStr sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE name = ?", tableName).Scan(&sqlStr)
	if err != nil {
		return "", err
	}
	return sqlStr.String, nil
}

func (s *SQLiteSession) GetTableEstimate(ctx context.Context, tableName string) (*driver.TableEstimate, error) {
	var count int64
	// Fast count for safety guard
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(tableName))
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return &driver.TableEstimate{RowCount: 0, SizeBytes: 0}, nil
	}
	return &driver.TableEstimate{
		RowCount:  count,
		SizeBytes: count * 128, // Approximation
	}, nil
}

func (s *SQLiteSession) Explain(ctx context.Context, querySQL string) (*driver.ExplainPlan, error) {
	cleanedSQL := strings.TrimSpace(querySQL)
	cleanedSQL = strings.TrimRight(cleanedSQL, "; \t\r\n")
	upper := strings.ToUpper(cleanedSQL)
	if strings.HasPrefix(upper, "EXPLAIN QUERY PLAN ") {
		cleanedSQL = strings.TrimSpace(cleanedSQL[19:])
	} else if strings.HasPrefix(upper, "EXPLAIN ") {
		cleanedSQL = strings.TrimSpace(cleanedSQL[8:])
	}
	if cleanedSQL == "" {
		return &driver.ExplainPlan{Rows: []driver.ExplainRow{}}, nil
	}

	rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+cleanedSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	planRows, err := driver.ScanExplainRows(rows)
	if err != nil {
		return nil, err
	}

	return &driver.ExplainPlan{
		Rows: planRows,
	}, nil
}

func (s *SQLiteSession) CancelQuery(ctx context.Context, queryID string) error {
	s.mu.Lock()
	cancel, ok := s.queries[queryID]
	s.mu.Unlock()

	if ok && cancel != nil {
		cancel()
		return nil
	}
	return nil
}

func (s *SQLiteSession) GetProcesslist(ctx context.Context) (*driver.ProcesslistResult, error) {
	// ponytail: SQLite is an embedded single-process engine without multi-client thread visibility.
	return &driver.ProcesslistResult{
		CurrentThreadID: 1,
		Threads: []driver.ProcessItem{
			{
				ID:      1,
				User:    "local",
				Host:    "localhost",
				DB:      "main",
				Command: "Embedded",
				Time:    0,
				State:   "idle",
				Info:    "SQLite embedded connection",
			},
		},
		Locks: []driver.LockWait{},
	}, nil
}

func (s *SQLiteSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	// ponytail: No-op for SQLite embedded database
	return nil
}

// GetDatabaseDiagnostics returns physical storage and metadata for SQLite.
func (s *SQLiteSession) GetDatabaseDiagnostics(ctx context.Context) ([]driver.DatabaseDiagnostics, error) {
	var pageSize, pageCount, freelistCount int64
	_ = s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
	_ = s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	_ = s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freelistCount)

	totalSize := pageCount * pageSize
	freeSize := freelistCount * pageSize

	schema, _ := s.InspectSchema(ctx, "main")
	tableCount := int64(0)
	if schema != nil {
		tableCount = int64(len(schema.Tables))
	}

	name := "main"
	if s.dsn != "" {
		name = filepath.Base(s.dsn)
	}

	return []driver.DatabaseDiagnostics{
		{
			Name:         name,
			TableCount:   tableCount,
			DataSize:     totalSize - freeSize,
			IndexSize:    0,
			OverheadSize: freeSize,
			TotalSize:    totalSize,
			Charset:      "UTF-8",
			Collation:    "BINARY",
			IsSystem:     false,
		},
	}, nil
}

func (s *SQLiteSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	var pageSize, pageCount, freelistCount int64
	_ = s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
	_ = s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	_ = s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freelistCount)

	totalSize := pageCount * pageSize
	freeSize := freelistCount * pageSize

	schema, err := s.InspectSchema(ctx, "main")
	if err != nil {
		return nil, err
	}

	var results []driver.TableDiagnostics
	for _, tbl := range schema.Tables {
		if table != "" && !strings.EqualFold(tbl.Name, table) {
			continue
		}
		var rowCount int64
		_ = s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(tbl.Name))).Scan(&rowCount)

		hasPk := false
		for _, col := range tbl.Columns {
			if col.IsPrimaryKey {
				hasPk = true
				break
			}
		}

		results = append(results, driver.TableDiagnostics{
			Name:          tbl.Name,
			Engine:        "SQLite",
			Rows:          rowCount,
			DataSize:      totalSize,
			IndexSize:     0,
			OverheadSize:  freeSize,
			RowFormat:     "Dynamic",
			Collation:     "BINARY",
			HasPrimaryKey: hasPk,
		})
	}
	if results == nil {
		results = []driver.TableDiagnostics{}
	}
	return results, nil
}

func (s *SQLiteSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	upperAction := strings.ToUpper(strings.TrimSpace(action))
	var results []driver.MaintenanceResult

	switch upperAction {
	case "OPTIMIZE", "VACUUM":
		_, err := s.db.ExecContext(ctx, "VACUUM")
		status := "OK"
		msg := "Database vacuumed successfully"
		if err != nil {
			status = "Error"
			msg = err.Error()
		}
		results = append(results, driver.MaintenanceResult{
			Table:   "database",
			Op:      "VACUUM",
			Status:  status,
			Message: msg,
		})
	case "ANALYZE":
		_, err := s.db.ExecContext(ctx, "ANALYZE")
		status := "OK"
		msg := "Statistics gathered successfully"
		if err != nil {
			status = "Error"
			msg = err.Error()
		}
		results = append(results, driver.MaintenanceResult{
			Table:   "database",
			Op:      "ANALYZE",
			Status:  status,
			Message: msg,
		})
	case "CHECK", "INTEGRITY_CHECK":
		var checkRes string
		err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&checkRes)
		status := "OK"
		if err != nil {
			status = "Error"
			checkRes = err.Error()
		}
		results = append(results, driver.MaintenanceResult{
			Table:   "database",
			Op:      "INTEGRITY_CHECK",
			Status:  status,
			Message: checkRes,
		})
	default:
		return nil, fmt.Errorf("unsupported SQLite maintenance action: %s", action)
	}

	return results, nil
}

func (s *SQLiteSession) GetRedundantIndexes(ctx context.Context, database, table string) ([]driver.RedundantIndex, error) {
	schema, err := s.InspectSchema(ctx, "main")
	if err != nil {
		return nil, err
	}
	var tables []driver.TableMetadata
	if table != "" {
		for _, tbl := range schema.Tables {
			if strings.EqualFold(tbl.Name, table) {
				tables = append(tables, tbl)
			}
		}
	} else {
		tables = schema.Tables
	}
	return driver.DetectRedundantIndexes("sqlite", "main", tables), nil
}

func (s *SQLiteSession) PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*driver.PeekResult, error) {
	sqlCmd := fmt.Sprintf("SELECT * FROM %s WHERE %s = ? LIMIT 1", quoteIdentifier(table), quoteIdentifier(column))
	rows, err := s.db.QueryContext(ctx, sqlCmd, value)
	if err != nil {
		return nil, fmt.Errorf("failed to peek foreign key row: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	if !rows.Next() {
		return &driver.PeekResult{
			Table:   table,
			KeyCol:  column,
			KeyVal:  value,
			Row:     nil,
			Columns: cols,
		}, nil
	}

	rawVals := make([]any, len(cols))
	scanArgs := make([]any, len(cols))
	for i := range rawVals {
		scanArgs[i] = &rawVals[i]
	}

	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}

	rowMap := make(map[string]any, len(cols))
	for i, col := range cols {
		val := rawVals[i]
		if b, ok := val.([]byte); ok {
			rowMap[col] = string(b)
		} else {
			rowMap[col] = val
		}
	}

	return &driver.PeekResult{
		Table:   table,
		KeyCol:  column,
		KeyVal:  value,
		Row:     rowMap,
		Columns: cols,
	}, nil
}

func (s *SQLiteSession) GetUserList(ctx context.Context) ([]driver.DatabaseUser, error) {
	return []driver.DatabaseUser{}, errors.New("user privilege management is not supported on SQLite")
}

func (s *SQLiteSession) GetUserGrants(ctx context.Context, user, host string) ([]string, error) {
	return nil, errors.New("user privilege management is not supported on SQLite")
}

func (s *SQLiteSession) SaveUser(ctx context.Context, user, host, password string, isNew bool) error {
	return errors.New("user privilege management is not supported on SQLite")
}

func (s *SQLiteSession) DropUser(ctx context.Context, user, host string) error {
	return errors.New("user privilege management is not supported on SQLite")
}

func (s *SQLiteSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	return errors.New("user privilege management is not supported on SQLite")
}

func (s *SQLiteSession) InspectTableLocks(ctx context.Context, database, table string) ([]driver.TableLockInfo, error) {
	// In-process SQLite does not maintain an external metadata lock wait table.
	return []driver.TableLockInfo{}, nil
}

func (s *SQLiteSession) ExecSafeDDL(ctx context.Context, ddl string, lockTimeoutSec int) (*driver.ExecResult, error) {
	if lockTimeoutSec <= 0 {
		lockTimeoutSec = 3
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain dedicated sqlite connection: %w", err)
	}
	defer conn.Close()

	_, _ = conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", lockTimeoutSec*1000))

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
		Message:      "Safe DDL executed successfully on SQLite",
	}, nil
}

func (s *SQLiteSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	return &driver.ClusterHealthReport{
		TopologyType:        "standalone",
		ClusterName:         "standalone-sqlite",
		ClusterStatus:       "Standalone File Engine",
		ClusterSize:         1,
		LocalNodeName:       "local",
		LocalNodeState:      "Active",
		BottleneckDiagnosis: "SQLite embedded standalone engine (clustering not applicable)",
		BottleneckSeverity:  "ok",
		Nodes: []driver.ClusterNodeInfo{
			{
				Name:    "local",
				Address: s.dsn,
				State:   "Active",
				IsLocal: true,
				Status:  "online",
			},
		},
	}, nil
}

func (s *SQLiteSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	return &driver.FlightMetricSample{
		SampledAt:          time.Now(),
		Questions:          1,
		ThreadsRunning:     1,
		ThreadsConnected:   1,
		RowLockWaits:       0,
		BufferPoolWaitFree: 0,
		ActiveQueries:      []driver.FlightQueryInfo{},
	}, nil
}

func (s *SQLiteSession) Close() error {
	return s.db.Close()
}

func (s *SQLiteSession) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *SQLiteSession) QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error) {
	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var result [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		result = append(result, vals)
	}
	return result, rows.Err()
}

// InspectServerVariables introspects SQLite configuration pragmas.
func (s *SQLiteSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	pragmas := []struct {
		name, desc, category string
	}{
		{"journal_mode", "Controls write-ahead logging (WAL) vs rollback journal.", "IO & Durability"},
		{"synchronous", "Controls disk sync durability (FULL, NORMAL, OFF).", "IO & Durability"},
		{"cache_size", "Number of database disk pages held in memory cache.", "Memory & Buffer"},
		{"temp_store", "Location of temporary tables and indices (MEMORY vs FILE).", "Memory & Buffer"},
		{"foreign_keys", "Enforcement of foreign key constraints.", "General"},
		{"busy_timeout", "Milliseconds to sleep and retry when database is locked.", "Concurrency & Lock"},
		{"wal_autocheckpoint", "Number of WAL pages before auto-checkpoint to DB.", "IO & Durability"},
	}

	var vars []driver.ServerVariable
	for _, p := range pragmas {
		var val string
		_ = s.db.QueryRowContext(ctx, fmt.Sprintf("PRAGMA %s", p.name)).Scan(&val)
		vars = append(vars, driver.ServerVariable{
			Name:             p.name,
			Value:            val,
			Scope:            "Global",
			IsDynamic:        true,
			Category:         p.category,
			Description:      p.desc,
			Severity:         "info",
			IsCurated:        false,
			MyCnfSnippet:     fmt.Sprintf("PRAGMA %s = %s;", p.name, val),
			SetGlobalSnippet: fmt.Sprintf("PRAGMA %s = %s;", p.name, val),
		})
	}

	var version string
	_ = s.db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version)

	return &driver.ServerVariablesReport{
		Engine:       "sqlite",
		Version:      version,
		TotalCount:   len(vars),
		CuratedCount: 0,
		Variables:    vars,
	}, nil
}


func quoteIdentifier(id string) string {
	return driver.QuoteIdentifier(id, "sqlite")
}

func (s *SQLiteSession) InspectDeadlock(ctx context.Context) (*driver.DeadlockReport, error) {
	return &driver.DeadlockReport{
		HasDeadlock: false,
		Summary:     "SQLite operates with database file-level locking and busy timeouts; deadlocks do not apply.",
	}, nil
}

func (s *SQLiteSession) GetStatementDigest(ctx context.Context, limit int) ([]driver.StatementDigestEntry, error) {
	return []driver.StatementDigestEntry{}, nil
}

func (s *SQLiteSession) ResetStatementDigest(ctx context.Context) error {
	return nil
}

func (s *SQLiteSession) GetRoutineDDL(ctx context.Context, routineType, name string) (string, error) {
	return "-- Stored Routines not supported in SQLite", nil
}

func (s *SQLiteSession) GetEventDDL(ctx context.Context, name string) (string, error) {
	return "-- Scheduled Events not supported in SQLite", nil
}

func (s *SQLiteSession) ToggleEventStatus(ctx context.Context, name string, enable bool) error {
	return nil
}

func (s *SQLiteSession) GetTablePartitions(ctx context.Context, database, table string) ([]driver.PartitionMetadata, error) {
	return []driver.PartitionMetadata{}, nil
}

