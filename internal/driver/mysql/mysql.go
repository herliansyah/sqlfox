package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"sqlfox/internal/advisor"
	"sqlfox/internal/driver"

	gomysql "github.com/go-sql-driver/mysql"
)

func init() {
	driver.Register("mysql", &MySQLDriver{})
	driver.Register("mariadb", &MySQLDriver{})
}

type MySQLDriver struct{}

func (d *MySQLDriver) Name() string {
	return "mysql"
}

func (d *MySQLDriver) Connect(ctx context.Context, cfg driver.Config) (driver.Session, error) {
	if cfg.Port <= 0 {
		cfg.Port = 3306
	}

	// 1. Configure TLS / SSL if enabled
	tlsParam := "false"
	if cfg.UseSSL {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
		}

		if cfg.SSLCA != "" {
			caCert, err := os.ReadFile(cfg.SSLCA)
			if err != nil {
				caCert = []byte(cfg.SSLCA) // Direct string content fallback
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = caCertPool
		}

		if cfg.SSLCert != "" && cfg.SSLKey != "" {
			cert, err := tls.LoadX509KeyPair(cfg.SSLCert, cfg.SSLKey)
			if err != nil {
				cert, err = tls.X509KeyPair([]byte(cfg.SSLCert), []byte(cfg.SSLKey))
			}
			if err == nil {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		tlsName := fmt.Sprintf("custom-tls-%d", time.Now().UnixNano())
		if err := gomysql.RegisterTLSConfig(tlsName, tlsConfig); err == nil {
			tlsParam = tlsName
		} else {
			tlsParam = "skip-verify"
		}
	}

	// 2. Assemble DSN
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s&timeout=10s",
		cfg.User, cfg.Password, addr, cfg.DatabaseName, tlsParam,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	// Bounded connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to MySQL server (%s:%d): %w", cfg.Host, cfg.Port, err)
	}

	return &MySQLSession{
		db:           db,
		cfg:          cfg,
		queryThreads: make(map[string]int64),
	}, nil
}

type MySQLSession struct {
	db           *sql.DB
	cfg          driver.Config
	mu           sync.Mutex
	queryThreads map[string]int64
}

func (s *MySQLSession) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	err := s.db.PingContext(ctx)
	return time.Since(start), err
}

func (s *MySQLSession) ServerInfo(ctx context.Context) (*driver.ServerInfo, error) {
	var version string
	if err := s.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return nil, err
	}

	var uptimeSec int64
	var activeConns int
	_ = s.db.QueryRowContext(ctx, "SHOW STATUS LIKE 'Uptime'").Scan(new(string), &uptimeSec)
	_ = s.db.QueryRowContext(ctx, "SHOW STATUS LIKE 'Threads_connected'").Scan(new(string), &activeConns)

	return &driver.ServerInfo{
		Version:     version,
		UptimeSec:   uptimeSec,
		ActiveConns: activeConns,
	}, nil
}

func (s *MySQLSession) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
	// Apply implicit safe limit for unbounded SELECTs
	trimmed := strings.TrimSpace(querySQL)
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "SELECT") && !strings.Contains(upper, "LIMIT") && limit > 0 {
		querySQL = fmt.Sprintf("%s LIMIT %d", strings.TrimSuffix(trimmed, ";"), limit)
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

func (s *MySQLSession) Exec(ctx context.Context, querySQL string, args ...any) (*driver.ExecResult, error) {
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
		Message:      fmt.Sprintf("Query OK, %d rows affected.", rowsAffected),
	}, nil
}

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
		dbName = s.cfg.DatabaseName
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

	return &driver.SchemaMetadata{
		DatabaseName: dbName,
		Databases:    databases,
		Tables:       tables,
		Triggers:     triggers,
	}, nil
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
	if s.cfg.DatabaseName != "" {
		escapedDB := strings.ReplaceAll(s.cfg.DatabaseName, "`", "``")
		query = fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`", escapedDB, escapedTbl)
	} else {
		query = fmt.Sprintf("SHOW CREATE TABLE `%s`", escapedTbl)
	}
	if err := s.db.QueryRowContext(ctx, query).Scan(&tbl, &ddl); err != nil {
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
	err := s.db.QueryRowContext(ctx, query, s.cfg.DatabaseName, tableName).Scan(&rowCount, &sizeBytes)
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

	// Classic tabular EXPLAIN (always supported)
	rows, err := s.db.QueryContext(ctx, "EXPLAIN "+cleanedSQL)
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
	_ = s.db.QueryRowContext(ctx, "EXPLAIN FORMAT=JSON "+cleanedSQL).Scan(&rawJSON)

	return &driver.ExplainPlan{
		RawJSON: rawJSON,
		Rows:    planRows,
	}, nil
}

func (s *MySQLSession) CancelQuery(ctx context.Context, queryID string) error {
	s.mu.Lock()
	threadID, ok := s.queryThreads[queryID]
	s.mu.Unlock()

	if !ok || threadID <= 0 {
		return nil
	}

	// Fail-fast timeout for out-of-band control connection (3s max)
	ctxCancel, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Execute out-of-band KILL QUERY on independent connection
	_, err := s.db.ExecContext(ctxCancel, fmt.Sprintf("KILL QUERY %d", threadID))
	if err != nil {
		if errors.Is(ctxCancel.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("control channel timeout (3s): target database connection pool may be saturated: %w", err)
		}
		return fmt.Errorf("failed to cancel query via control channel: %w", err)
	}
	return nil
}

func (s *MySQLSession) GetProcesslist(ctx context.Context) (*driver.ProcesslistResult, error) {
	// 1. Get current connection thread ID
	var currentThreadID int64
	_ = s.db.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&currentThreadID)

	// 2. Fetch processes from information_schema.PROCESSLIST or SHOW FULL PROCESSLIST
	var threads []driver.ProcessItem
	rows, err := s.db.QueryContext(ctx, `
		SELECT ID, USER, HOST, IFNULL(DB, ''), COMMAND, TIME, IFNULL(STATE, ''), IFNULL(INFO, '')
		FROM information_schema.PROCESSLIST
		ORDER BY TIME DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p driver.ProcessItem
			if err := rows.Scan(&p.ID, &p.User, &p.Host, &p.DB, &p.Command, &p.Time, &p.State, &p.Info); err == nil {
				threads = append(threads, p)
			}
		}
	} else {
		// Fallback to SHOW FULL PROCESSLIST
		fallbackRows, fErr := s.db.QueryContext(ctx, "SHOW FULL PROCESSLIST")
		if fErr != nil {
			return nil, fmt.Errorf("failed to fetch processlist: %w", fErr)
		}
		defer fallbackRows.Close()
		for fallbackRows.Next() {
			var id int64
			var user, host, command string
			var db, state, info sql.NullString
			var timeVal int64
			if err := fallbackRows.Scan(&id, &user, &host, &db, &command, &timeVal, &state, &info); err == nil {
				threads = append(threads, driver.ProcessItem{
					ID:      id,
					User:    user,
					Host:    host,
					DB:      db.String,
					Command: command,
					Time:    timeVal,
					State:   state.String,
					Info:    info.String,
				})
			}
		}
	}

	if threads == nil {
		threads = []driver.ProcessItem{}
	}

	// 3. Inspect Lock Waits (Tier 1: sys.innodb_lock_waits)
	var locks []driver.LockWait
	lockRows, lErr := s.db.QueryContext(ctx, `
		SELECT
			IFNULL(waiting_pid, 0),
			IFNULL(waiting_query, ''),
			IFNULL(waiting_account, ''),
			IFNULL(wait_age_secs, 0),
			IFNULL(blocking_pid, 0),
			IFNULL(blocking_query, ''),
			IFNULL(blocking_account, ''),
			IFNULL(locked_table, ''),
			IFNULL(waiting_lock_mode, '')
		FROM sys.innodb_lock_waits
	`)
	if lErr == nil {
		defer lockRows.Close()
		waitingPIDs := make(map[int64]bool)
		for lockRows.Next() {
			var lw driver.LockWait
			if err := lockRows.Scan(
				&lw.WaitingThreadID,
				&lw.WaitingQuery,
				&lw.WaitingUser,
				&lw.WaitingSeconds,
				&lw.BlockingThreadID,
				&lw.BlockingQuery,
				&lw.BlockingUser,
				&lw.LockedResource,
				&lw.LockMode,
			); err == nil {
				waitingPIDs[lw.WaitingThreadID] = true
				locks = append(locks, lw)
			}
		}

		// Identify root blockers: a blocking PID that is NOT currently waiting on any lock
		for i := range locks {
			if !waitingPIDs[locks[i].BlockingThreadID] {
				locks[i].IsRootBlocker = true
			}
		}
	} else {
		// Tier 2 Fallback: heuristic scan of threads with lock states
		for _, t := range threads {
			st := strings.ToLower(t.State)
			if strings.Contains(st, "lock") || strings.Contains(st, "waiting for table") {
				locks = append(locks, driver.LockWait{
					WaitingThreadID: t.ID,
					WaitingQuery:    t.Info,
					WaitingUser:     t.User,
					WaitingSeconds:  t.Time,
					LockedResource:  t.DB,
					LockMode:        t.State,
					IsRootBlocker:   false,
				})
			}
		}
	}

	if locks == nil {
		locks = []driver.LockWait{}
	}

	return &driver.ProcesslistResult{
		CurrentThreadID: currentThreadID,
		Threads:         threads,
		Locks:           locks,
	}, nil
}

func (s *MySQLSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	if threadID <= 0 {
		return errors.New("invalid thread id")
	}

	var cmd string
	if killConnection {
		cmd = fmt.Sprintf("KILL %d", threadID)
	} else {
		cmd = fmt.Sprintf("KILL QUERY %d", threadID)
	}

	_, err := s.db.ExecContext(ctx, cmd)
	return err
}

// GetDatabaseDiagnostics returns physical storage and metadata for all databases on the MySQL server.
func (s *MySQLSession) GetDatabaseDiagnostics(ctx context.Context) ([]driver.DatabaseDiagnostics, error) {
	query := `
		SELECT 
			s.SCHEMA_NAME, 
			IFNULL(s.DEFAULT_CHARACTER_SET_NAME, ''), 
			IFNULL(s.DEFAULT_COLLATION_NAME, ''), 
			COUNT(t.TABLE_NAME), 
			IFNULL(SUM(t.DATA_LENGTH), 0), 
			IFNULL(SUM(t.INDEX_LENGTH), 0), 
			IFNULL(SUM(t.DATA_FREE), 0)
		FROM information_schema.SCHEMATA s
		LEFT JOIN information_schema.TABLES t ON s.SCHEMA_NAME = t.TABLE_SCHEMA
		GROUP BY s.SCHEMA_NAME, s.DEFAULT_CHARACTER_SET_NAME, s.DEFAULT_COLLATION_NAME
		ORDER BY 
			CASE WHEN s.SCHEMA_NAME IN ('information_schema', 'mysql', 'performance_schema', 'sys') THEN 1 ELSE 0 END,
			(IFNULL(SUM(t.DATA_LENGTH), 0) + IFNULL(SUM(t.INDEX_LENGTH), 0)) DESC,
			s.SCHEMA_NAME ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query database diagnostics: %w", err)
	}
	defer rows.Close()

	var result []driver.DatabaseDiagnostics
	for rows.Next() {
		var dd driver.DatabaseDiagnostics
		if err := rows.Scan(
			&dd.Name,
			&dd.Charset,
			&dd.Collation,
			&dd.TableCount,
			&dd.DataSize,
			&dd.IndexSize,
			&dd.OverheadSize,
		); err == nil {
			dd.TotalSize = dd.DataSize + dd.IndexSize
			lowerName := strings.ToLower(dd.Name)
			dd.IsSystem = lowerName == "information_schema" || lowerName == "mysql" || lowerName == "performance_schema" || lowerName == "sys"
			result = append(result, dd)
		}
	}
	if result == nil {
		result = []driver.DatabaseDiagnostics{}
	}
	return result, nil
}

func (s *MySQLSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	if database == "" {
		database = s.cfg.DatabaseName
	}
	query := `
		SELECT 
			t.TABLE_NAME, 
			IFNULL(t.ENGINE, ''), 
			IFNULL(t.TABLE_ROWS, 0), 
			IFNULL(t.DATA_LENGTH, 0), 
			IFNULL(t.INDEX_LENGTH, 0), 
			IFNULL(t.DATA_FREE, 0), 
			IFNULL(t.ROW_FORMAT, ''), 
			IFNULL(t.TABLE_COLLATION, ''), 
			IFNULL(DATE_FORMAT(t.CREATE_TIME, '%Y-%m-%d %H:%i:%s'), ''), 
			IFNULL(DATE_FORMAT(t.UPDATE_TIME, '%Y-%m-%d %H:%i:%s'), ''),
			CASE WHEN pk.TABLE_NAME IS NOT NULL THEN 1 ELSE 0 END
		FROM information_schema.TABLES t
		LEFT JOIN (
			SELECT DISTINCT TABLE_NAME
			FROM information_schema.STATISTICS
			WHERE TABLE_SCHEMA = ? AND INDEX_NAME = 'PRIMARY'
		) pk ON t.TABLE_NAME = pk.TABLE_NAME
		WHERE t.TABLE_SCHEMA = ?
	`
	args := []any{database, database}
	if table != "" {
		query += " AND t.TABLE_NAME = ?"
		args = append(args, table)
	}
	query += " ORDER BY (t.DATA_LENGTH + t.INDEX_LENGTH) DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query table diagnostics: %w", err)
	}
	defer rows.Close()

	var result []driver.TableDiagnostics
	for rows.Next() {
		var td driver.TableDiagnostics
		var hasPk int
		if err := rows.Scan(
			&td.Name,
			&td.Engine,
			&td.Rows,
			&td.DataSize,
			&td.IndexSize,
			&td.OverheadSize,
			&td.RowFormat,
			&td.Collation,
			&td.CreatedAt,
			&td.UpdatedAt,
			&hasPk,
		); err == nil {
			td.HasPrimaryKey = hasPk == 1
			result = append(result, td)
		}
	}
	if result == nil {
		result = []driver.TableDiagnostics{}
	}
	return result, nil
}

func (s *MySQLSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	if database == "" {
		database = s.cfg.DatabaseName
	}
	upperAction := strings.ToUpper(strings.TrimSpace(action))
	if upperAction != "OPTIMIZE" && upperAction != "ANALYZE" && upperAction != "CHECK" {
		return nil, fmt.Errorf("unsupported maintenance action: %s", action)
	}

	var results []driver.MaintenanceResult
	for _, tbl := range tables {
		sqlCmd := fmt.Sprintf("%s TABLE `%s`.`%s`", upperAction, database, tbl)
		rows, err := s.db.QueryContext(ctx, sqlCmd)
		if err != nil {
			results = append(results, driver.MaintenanceResult{
				Table:   tbl,
				Op:      upperAction,
				Status:  "Error",
				Message: err.Error(),
			})
			continue
		}
		for rows.Next() {
			var tblName, op, msgType, msgText string
			if err := rows.Scan(&tblName, &op, &msgType, &msgText); err == nil {
				results = append(results, driver.MaintenanceResult{
					Table:   tbl,
					Op:      op,
					Status:  msgType,
					Message: msgText,
				})
			}
		}
		rows.Close()
	}
	if results == nil {
		results = []driver.MaintenanceResult{}
	}
	return results, nil
}

func (s *MySQLSession) GetRedundantIndexes(ctx context.Context, database, table string) ([]driver.RedundantIndex, error) {
	if database == "" {
		database = s.cfg.DatabaseName
	}
	schema, err := s.InspectSchema(ctx, database)
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
	return driver.DetectRedundantIndexes("mysql", database, tables), nil
}

func (s *MySQLSession) PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*driver.PeekResult, error) {
	if database == "" {
		database = s.cfg.DatabaseName
	}
	sqlCmd := fmt.Sprintf("SELECT * FROM `%s`.`%s` WHERE `%s` = ? LIMIT 1", database, table, column)
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

func (s *MySQLSession) GetUserList(ctx context.Context) ([]driver.DatabaseUser, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT user, host FROM mysql.user ORDER BY user ASC, host ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to list database users: %w", err)
	}
	defer rows.Close()

	var users []driver.DatabaseUser
	for rows.Next() {
		var u driver.DatabaseUser
		if err := rows.Scan(&u.User, &u.Host); err == nil {
			users = append(users, u)
		}
	}
	if users == nil {
		users = []driver.DatabaseUser{}
	}
	return users, nil
}

func (s *MySQLSession) GetUserGrants(ctx context.Context, user, host string) ([]string, error) {
	sqlCmd := fmt.Sprintf("SHOW GRANTS FOR '%s'@'%s'", strings.ReplaceAll(user, "'", "''"), strings.ReplaceAll(host, "'", "''"))
	rows, err := s.db.QueryContext(ctx, sqlCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user grants: %w", err)
	}
	defer rows.Close()

	var grants []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err == nil {
			grants = append(grants, g)
		}
	}
	if grants == nil {
		grants = []string{}
	}
	return grants, nil
}

func (s *MySQLSession) SaveUser(ctx context.Context, user, host, password string, isNew bool) error {
	uEsc := strings.ReplaceAll(user, "'", "''")
	hEsc := strings.ReplaceAll(host, "'", "''")
	pEsc := strings.ReplaceAll(password, "'", "''")

	if isNew {
		cmd := fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s'", uEsc, hEsc, pEsc)
		if _, err := s.db.ExecContext(ctx, cmd); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	} else if password != "" {
		cmd := fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s'", uEsc, hEsc, pEsc)
		if _, err := s.db.ExecContext(ctx, cmd); err != nil {
			return fmt.Errorf("failed to update user password: %w", err)
		}
	}
	_, _ = s.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return nil
}

func (s *MySQLSession) DropUser(ctx context.Context, user, host string) error {
	uEsc := strings.ReplaceAll(user, "'", "''")
	hEsc := strings.ReplaceAll(host, "'", "''")
	cmd := fmt.Sprintf("DROP USER '%s'@'%s'", uEsc, hEsc)
	if _, err := s.db.ExecContext(ctx, cmd); err != nil {
		return fmt.Errorf("failed to drop user: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return nil
}

func (s *MySQLSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	uEsc := strings.ReplaceAll(req.User, "'", "''")
	hEsc := strings.ReplaceAll(req.Host, "'", "''")
	privs := strings.Join(req.Privileges, ", ")
	if privs == "" {
		privs = "USAGE"
	}

	target := "*.*"
	if req.Scope == "DATABASE" && req.Database != "" {
		target = fmt.Sprintf("`%s`.*", req.Database)
	}

	cmd := fmt.Sprintf("GRANT %s ON %s TO '%s'@'%s'", privs, target, uEsc, hEsc)
	if req.WithGrant {
		cmd += " WITH GRANT OPTION"
	}

	if _, err := s.db.ExecContext(ctx, cmd); err != nil {
		return fmt.Errorf("failed to apply user grants: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return nil
}

func (s *MySQLSession) InspectTableLocks(ctx context.Context, database, table string) ([]driver.TableLockInfo, error) {
	if database == "" {
		database = s.cfg.DatabaseName
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
		JOIN information_schema.processlist p ON m.OWNER_THREAD_ID = p.ID
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
func (s *MySQLSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	// 1. Check if Galera / WSREP is active
	var wsrepReadyVal string
	_ = s.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'wsrep_ready'").Scan(new(string), &wsrepReadyVal)

	if strings.EqualFold(wsrepReadyVal, "ON") {
		return s.inspectGaleraCluster(ctx)
	}

	// 2. Fallback: inspect standard replication or standalone
	return s.inspectStandardReplicationOrStandalone(ctx)
}

func (s *MySQLSession) inspectGaleraCluster(ctx context.Context) (*driver.ClusterHealthReport, error) {
	statusVars := make(map[string]string)
	rows, err := s.db.QueryContext(ctx, `SHOW GLOBAL STATUS WHERE Variable_name IN (
		'wsrep_cluster_size', 'wsrep_cluster_status', 'wsrep_cluster_name',
		'wsrep_node_name', 'wsrep_local_state_comment', 'wsrep_flow_control_paused',
		'wsrep_flow_control_sent', 'wsrep_flow_control_recv', 'wsrep_local_recv_queue',
		'wsrep_local_send_queue', 'wsrep_evs_repl_latency', 'wsrep_incoming_addresses',
		'wsrep_local_cert_failures', 'wsrep_local_bf_aborts',
		'Innodb_buffer_pool_wait_free', 'Innodb_log_waits', 'Innodb_os_log_fsyncs'
	)`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				statusVars[k] = v
			}
		}
	}

	sysVars := make(map[string]string)
	vRows, err := s.db.QueryContext(ctx, `SHOW VARIABLES WHERE Variable_name IN (
		'wsrep_slave_threads', 'wsrep_node_address', 'innodb_autoinc_lock_mode',
		'binlog_format', 'wsrep_sync_wait', 'wsrep_retry_autocommit'
	)`)
	if err == nil {
		defer vRows.Close()
		for vRows.Next() {
			var k, v string
			if err := vRows.Scan(&k, &v); err == nil {
				sysVars[k] = v
			}
		}
	}

	clusterSize, _ := strconv.Atoi(statusVars["wsrep_cluster_size"])
	clusterStatus := statusVars["wsrep_cluster_status"]
	if clusterStatus == "" {
		clusterStatus = "Primary"
	}
	clusterName := statusVars["wsrep_cluster_name"]
	localNodeName := statusVars["wsrep_node_name"]
	localNodeAddress := sysVars["wsrep_node_address"]
	if localNodeAddress == "" {
		localNodeAddress = fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	}
	localNodeState := statusVars["wsrep_local_state_comment"]
	if localNodeState == "" {
		localNodeState = "Synced"
	}

	fcPaused, _ := strconv.ParseFloat(statusVars["wsrep_flow_control_paused"], 64)
	fcSent, _ := strconv.ParseInt(statusVars["wsrep_flow_control_sent"], 10, 64)
	fcRecv, _ := strconv.ParseInt(statusVars["wsrep_flow_control_recv"], 10, 64)
	localRecvQueue, _ := strconv.ParseInt(statusVars["wsrep_local_recv_queue"], 10, 64)
	localSendQueue, _ := strconv.ParseInt(statusVars["wsrep_local_send_queue"], 10, 64)
	certFailures, _ := strconv.ParseInt(statusVars["wsrep_local_cert_failures"], 10, 64)
	bfAborts, _ := strconv.ParseInt(statusVars["wsrep_local_bf_aborts"], 10, 64)

	bufWaitFree, _ := strconv.ParseInt(statusVars["Innodb_buffer_pool_wait_free"], 10, 64)
	logWaits, _ := strconv.ParseInt(statusVars["Innodb_log_waits"], 10, 64)
	osLogFsyncs, _ := strconv.ParseInt(statusVars["Innodb_os_log_fsyncs"], 10, 64)

	slaveThreads, _ := strconv.Atoi(sysVars["wsrep_slave_threads"])
	if slaveThreads <= 0 {
		slaveThreads = 1
	}

	// Parse wsrep_evs_repl_latency: min/avg/max/stddev in seconds
	var latMin, latAvg, latMax float64
	evsLatStr := statusVars["wsrep_evs_repl_latency"]
	if evsLatStr != "" {
		parts := strings.Split(evsLatStr, "/")
		if len(parts) >= 3 {
			p0, _ := strconv.ParseFloat(parts[0], 64)
			p1, _ := strconv.ParseFloat(parts[1], 64)
			p2, _ := strconv.ParseFloat(parts[2], 64)
			latMin = p0 * 1000.0 // seconds to milliseconds
			latAvg = p1 * 1000.0
			latMax = p2 * 1000.0
		}
	}

	// 3. Peer Node Auto-Discovery via wsrep_incoming_addresses
	incomingAddrs := statusVars["wsrep_incoming_addresses"]
	addrList := strings.Split(incomingAddrs, ",")
	cleanAddrs := make([]string, 0, len(addrList))
	for _, a := range addrList {
		trimmed := strings.TrimSpace(a)
		if trimmed != "" {
			cleanAddrs = append(cleanAddrs, trimmed)
		}
	}

	if len(cleanAddrs) == 0 {
		cleanAddrs = []string{fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)}
	}

	nodeResults := make([]driver.ClusterNodeInfo, len(cleanAddrs))
	var wg sync.WaitGroup

	for i, addr := range cleanAddrs {
		idx := i
		nodeAddr := addr
		hostPort := nodeAddr
		if !strings.Contains(hostPort, ":") {
			hostPort = fmt.Sprintf("%s:3306", hostPort)
		}

		isLocal := strings.HasPrefix(nodeAddr, s.cfg.Host) ||
			strings.HasPrefix(nodeAddr, "127.0.0.1") ||
			nodeAddr == localNodeAddress ||
			len(cleanAddrs) == 1

		if isLocal {
			nodeResults[idx] = driver.ClusterNodeInfo{
				Name:            localNodeName,
				Address:         nodeAddr,
				State:           localNodeState,
				ReceiveQueue:    localRecvQueue,
				SendQueue:       localSendQueue,
				FlowControlSent: fcSent,
				IsLocal:         true,
				Status:          "online",
			}
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				nodeResults[idx] = s.probePeerNode(ctx, hostPort, nodeAddr)
			}()
		}
	}
	wg.Wait()

	// 4. Identify Culprit Node (Slowest Node triggering Flow Control / Queue backlog)
	var culpritNode *driver.ClusterNodeInfo
	var highestQueue int64 = -1

	for i := range nodeResults {
		n := &nodeResults[i]
		if n.Status == "online" {
			// A node is a culprit if it sent Flow Control or has an elevated receive queue
			if n.FlowControlSent > 0 && n.ReceiveQueue > highestQueue {
				highestQueue = n.ReceiveQueue
				culpritNode = n
			} else if culpritNode == nil && n.ReceiveQueue > 20 && n.ReceiveQueue > highestQueue {
				highestQueue = n.ReceiveQueue
				culpritNode = n
			}
		}
	}

	slowestNodeAddr := ""
	if culpritNode != nil {
		culpritNode.IsCulprit = true
		slowestNodeAddr = culpritNode.Address
		if culpritNode.Name != "" {
			slowestNodeAddr = fmt.Sprintf("%s (%s)", culpritNode.Name, culpritNode.Address)
		}
	}

	// 5. Tri-Metric Bottleneck Detective Evaluation
	diagnosis, severity := s.diagnoseClusterBottleneck(
		fcPaused, fcSent, fcRecv, culpritNode, localNodeName,
		latAvg, latMax, bufWaitFree, logWaits, localRecvQueue,
		slaveThreads, certFailures, bfAborts, clusterStatus,
	)

	return &driver.ClusterHealthReport{
		TopologyType:        "galera",
		ClusterName:         clusterName,
		ClusterStatus:       clusterStatus,
		ClusterSize:         clusterSize,
		LocalNodeName:       localNodeName,
		LocalNodeAddress:    localNodeAddress,
		LocalNodeState:      localNodeState,
		FlowControlPaused:   fcPaused,
		FlowControlSent:     fcSent,
		FlowControlRecv:     fcRecv,
		BottleneckDiagnosis: diagnosis,
		BottleneckSeverity:  severity,
		SlowestNode:         slowestNodeAddr,
		Nodes:               nodeResults,
		TriMetric: driver.TriMetricStats{
			NetworkLatencyMinMs: latMin,
			NetworkLatencyAvgMs: latAvg,
			NetworkLatencyMaxMs: latMax,
			BufferPoolWaitFree:  bufWaitFree,
			LogWaits:            logWaits,
			OSLogFsyncs:         osLogFsyncs,
			SlaveThreads:        slaveThreads,
			CertFailures:        certFailures,
			BFAborts:            bfAborts,
		},
		TuningChecks: advisor.EvaluateClusterTuning(sysVars, statusVars),
	}, nil
}

// probePeerNode connects to a cluster peer node with a 1500ms timeout to read queue and FC sent.
// ponytail: Quick single-shot query with fail-fast timeout prevents hanging on firewalled peer nodes.
func (s *MySQLSession) probePeerNode(parentCtx context.Context, hostPort, origAddr string) driver.ClusterNodeInfo {
	ctx, cancel := context.WithTimeout(parentCtx, 1500*time.Millisecond)
	defer cancel()

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&timeout=1200ms&readTimeout=1200ms",
		s.cfg.User, s.cfg.Password, hostPort, s.cfg.DatabaseName,
	)
	peerDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return driver.ClusterNodeInfo{
			Address: origAddr,
			Status:  "unreachable",
			Message: "Failed to initialize dialer",
		}
	}
	defer peerDB.Close()

	if err := peerDB.PingContext(ctx); err != nil {
		return driver.ClusterNodeInfo{
			Address: origAddr,
			Status:  "unreachable",
			Message: "🔒 Unreachable directly from client (VPC/Firewall restricted)",
		}
	}

	var nodeName, nodeState string
	var recvQueue, fcSent int64

	rows, err := peerDB.QueryContext(ctx, `SHOW GLOBAL STATUS WHERE Variable_name IN ('wsrep_node_name', 'wsrep_local_state_comment', 'wsrep_local_recv_queue', 'wsrep_flow_control_sent')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				switch k {
				case "wsrep_node_name":
					nodeName = v
				case "wsrep_local_state_comment":
					nodeState = v
				case "wsrep_local_recv_queue":
					recvQueue, _ = strconv.ParseInt(v, 10, 64)
				case "wsrep_flow_control_sent":
					fcSent, _ = strconv.ParseInt(v, 10, 64)
				}
			}
		}
	}

	return driver.ClusterNodeInfo{
		Name:            nodeName,
		Address:         origAddr,
		State:           nodeState,
		ReceiveQueue:    recvQueue,
		FlowControlSent: fcSent,
		Status:          "online",
	}
}

func (s *MySQLSession) diagnoseClusterBottleneck(
	fcPaused float64, fcSent, fcRecv int64, culprit *driver.ClusterNodeInfo, localName string,
	latAvg, latMax float64, bufWaitFree, logWaits, localQueue int64,
	slaveThreads int, certFailures, bfAborts int64, clusterStatus string,
) (string, string) {
	if !strings.EqualFold(clusterStatus, "Primary") {
		return fmt.Sprintf("Cluster quorum lost: current cluster status is '%s' (split-brain danger)", clusterStatus), "critical"
	}

	// 1. Flow Control Active
	if fcPaused > 0.05 {
		if culprit != nil && culprit.IsLocal {
			return fmt.Sprintf("Local node '%s' is bottlenecking the cluster: Flow Control active (paused %.1f%% of time, queue: %d)", localName, fcPaused*100, localQueue), "critical"
		} else if culprit != nil {
			return fmt.Sprintf("Cluster write freeze: Remote node '%s' (%s) triggered Flow Control (Receive Queue: %d, FC Sent: %d)", culprit.Name, culprit.Address, culprit.ReceiveQueue, culprit.FlowControlSent), "critical"
		}
		return fmt.Sprintf("Cluster write freeze: Flow Control is paused %.1f%% of time across cluster", fcPaused*100), "critical"
	} else if fcPaused > 0.005 {
		if culprit != nil {
			return fmt.Sprintf("Intermittent Flow Control throttling: node '%s' is applying writesets slowly (Queue: %d)", culprit.Address, culprit.ReceiveQueue), "warning"
		}
		return fmt.Sprintf("Intermittent Flow Control throttling (paused %.2f%%)", fcPaused*100), "warning"
	}

	// 2. Storage / Disk I/O Bottleneck
	if bufWaitFree > 0 || logWaits > 0 {
		return fmt.Sprintf("InnoDB Storage / Disk I/O stall detected: %d buffer pool wait free events, %d log flush waits", bufWaitFree, logWaits), "warning"
	}

	// 3. Network Latency Spike
	if latAvg > 5.0 || latMax > 50.0 {
		return fmt.Sprintf("High inter-node network replication latency: avg %.2fms (peak %.2fms)", latAvg, latMax), "warning"
	}

	// 4. Worker Concurrency Saturation
	if localQueue > int64(slaveThreads*15) && slaveThreads <= 4 {
		return fmt.Sprintf("Apply queue backlog (%d items): wsrep_slave_threads (%d) is under-provisioned for current write rate", localQueue, slaveThreads), "warning"
	}

	// 5. Certification Aborts (Concurrent write contention)
	if certFailures > 50 || bfAborts > 50 {
		return fmt.Sprintf("Optimistic certification conflicts detected: %d cert failures, %d brute-force aborts", certFailures, bfAborts), "warning"
	}

	return "Cluster is healthy: quorum primary, all nodes synchronized without flow control pressure", "ok"
}

func (s *MySQLSession) inspectStandardReplicationOrStandalone(ctx context.Context) (*driver.ClusterHealthReport, error) {
	// Query InnoDB storage waits for all MySQL engines
	var bufWaitFree, logWaits, osLogFsyncs int64
	rows, err := s.db.QueryContext(ctx, `SHOW GLOBAL STATUS WHERE Variable_name IN ('Innodb_buffer_pool_wait_free', 'Innodb_log_waits', 'Innodb_os_log_fsyncs')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				switch k {
				case "Innodb_buffer_pool_wait_free":
					bufWaitFree, _ = strconv.ParseInt(v, 10, 64)
				case "Innodb_log_waits":
					logWaits, _ = strconv.ParseInt(v, 10, 64)
				case "Innodb_os_log_fsyncs":
					osLogFsyncs, _ = strconv.ParseInt(v, 10, 64)
				}
			}
		}
	}

	triMetric := driver.TriMetricStats{
		BufferPoolWaitFree: bufWaitFree,
		LogWaits:           logWaits,
		OSLogFsyncs:        osLogFsyncs,
	}

	// Check SHOW SLAVE STATUS / SHOW REPLICA STATUS
	slaveRows, err := s.db.QueryContext(ctx, "SHOW SLAVE STATUS")
	if err == nil {
		defer slaveRows.Close()
		cols, err := slaveRows.Columns()
		if err == nil && slaveRows.Next() {
			colVals := make([]any, len(cols))
			colPtrs := make([]any, len(cols))
			for i := range colVals {
				colPtrs[i] = &colVals[i]
			}
			if err := slaveRows.Scan(colPtrs...); err == nil {
				colMap := make(map[string]string)
				for i, c := range cols {
					if colVals[i] != nil {
						switch v := colVals[i].(type) {
						case []byte:
							colMap[strings.ToLower(c)] = string(v)
						case string:
							colMap[strings.ToLower(c)] = v
						default:
							colMap[strings.ToLower(c)] = fmt.Sprintf("%v", v)
						}
					}
				}

				ioRunning := strings.EqualFold(colMap["slave_io_running"], "yes")
				sqlRunning := strings.EqualFold(colMap["slave_sql_running"], "yes")
				var secBehind *int64
				if sbm, ok := colMap["seconds_behind_master"]; ok && sbm != "NULL" && sbm != "" {
					val, _ := strconv.ParseInt(sbm, 10, 64)
					secBehind = &val
				}
				lastErr := colMap["last_error"]
				if lastErr == "" {
					lastErr = colMap["last_sql_error"]
				}
				masterHost := colMap["master_host"]

				diagnosis := "Replication synchronized and operating normally"
				severity := "ok"
				if !ioRunning || !sqlRunning {
					diagnosis = fmt.Sprintf("Replication broken (IO: %s, SQL: %s): %s", colMap["slave_io_running"], colMap["slave_sql_running"], lastErr)
					severity = "critical"
				} else if secBehind != nil && *secBehind > 10 {
					diagnosis = fmt.Sprintf("Replication lag behind master: %d seconds", *secBehind)
					severity = "warning"
				} else if bufWaitFree > 0 || logWaits > 0 {
					diagnosis = fmt.Sprintf("InnoDB storage write stall detected (wait_free: %d, log_waits: %d)", bufWaitFree, logWaits)
					severity = "warning"
				}

				return &driver.ClusterHealthReport{
					TopologyType:        "replication",
					ClusterName:         "mysql-replication",
					ClusterStatus:       "Replica",
					ClusterSize:         2,
					LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
					LocalNodeState:      "Replica",
					BottleneckDiagnosis: diagnosis,
					BottleneckSeverity:  severity,
					Replication: &driver.ReplicationInfo{
						Role:                "Replica",
						SecondsBehindMaster: secBehind,
						IORunning:           ioRunning,
						SQLRunning:          sqlRunning,
						MasterHost:          masterHost,
						LastError:           lastErr,
					},
					Nodes: []driver.ClusterNodeInfo{
						{
							Name:    "replica-local",
							Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
							State:   "Replica",
							IsLocal: true,
							Status:  "online",
						},
						{
							Name:    "source-master",
							Address: masterHost,
							State:   "Source",
							Status:  "online",
						},
					},
					TriMetric: triMetric,
				}, nil
			}
		}
	}

	// Check if this instance is a Source / Master with binary logging enabled
	var binlogFile string
	var binlogPos int64
	_ = s.db.QueryRowContext(ctx, "SHOW MASTER STATUS").Scan(&binlogFile, &binlogPos, new(string), new(string), new(string))
	if binlogFile != "" {
		diagnosis := "Replication Source (Master): binary logging active"
		severity := "ok"
		if bufWaitFree > 0 || logWaits > 0 {
			diagnosis = fmt.Sprintf("InnoDB storage write stall on Source (wait_free: %d, log_waits: %d)", bufWaitFree, logWaits)
			severity = "warning"
		}
		return &driver.ClusterHealthReport{
			TopologyType:        "replication",
			ClusterName:         "mysql-source",
			ClusterStatus:       "Source (Master)",
			ClusterSize:         1,
			LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
			LocalNodeState:      "Source",
			BottleneckDiagnosis: diagnosis,
			BottleneckSeverity:  severity,
			Replication: &driver.ReplicationInfo{
				Role:      "Source (Master)",
				IORunning: true,
				SQLRunning: true,
			},
			Nodes: []driver.ClusterNodeInfo{
				{
					Name:    "source-master",
					Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
					State:   "Source",
					IsLocal: true,
					Status:  "online",
				},
			},
			TriMetric: triMetric,
		}, nil
	}

	// Standalone instance fallback
	diagnosis := "Standalone MySQL / MariaDB instance"
	severity := "ok"
	if bufWaitFree > 0 || logWaits > 0 {
		diagnosis = fmt.Sprintf("InnoDB storage write stall detected (wait_free: %d, log_waits: %d)", bufWaitFree, logWaits)
		severity = "warning"
	}

	return &driver.ClusterHealthReport{
		TopologyType:        "standalone",
		ClusterName:         "standalone-mysql",
		ClusterStatus:       "Standalone",
		ClusterSize:         1,
		LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
		LocalNodeState:      "Standalone",
		BottleneckDiagnosis: diagnosis,
		BottleneckSeverity:  severity,
		Nodes: []driver.ClusterNodeInfo{
			{
				Name:    "standalone-node",
				Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
				State:   "Standalone",
				IsLocal: true,
				Status:  "online",
			},
		},
		TriMetric: triMetric,
	}, nil
}

// SampleFlightMetrics captures an on-demand performance telemetry slice with correlated active queries.
func (s *MySQLSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	sample := &driver.FlightMetricSample{
		SampledAt:     time.Now(),
		ActiveQueries: []driver.FlightQueryInfo{},
	}

	// 1. Fetch Global Status variables
	rows, err := s.db.QueryContext(ctx, `
		SHOW GLOBAL STATUS WHERE Variable_name IN (
			'Questions',
			'Threads_running',
			'Threads_connected',
			'Innodb_row_lock_waits',
			'Innodb_row_lock_current_waits',
			'Innodb_buffer_pool_wait_free'
		)
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, val string
			if err := rows.Scan(&name, &val); err == nil {
				intVal, _ := strconv.ParseInt(val, 10, 64)
				switch name {
				case "Questions":
					sample.Questions = intVal
				case "Threads_running":
					sample.ThreadsRunning = intVal
				case "Threads_connected":
					sample.ThreadsConnected = intVal
				case "Innodb_row_lock_waits", "Innodb_row_lock_current_waits":
					if intVal > sample.RowLockWaits {
						sample.RowLockWaits = intVal
					}
				case "Innodb_buffer_pool_wait_free":
					sample.BufferPoolWaitFree = intVal
				}
			}
		}
	}

	// 2. Adaptive Correlated Queries: fetch active non-idle threads (Top 5)
	qRows, qErr := s.db.QueryContext(ctx, `
		SELECT ID, USER, HOST, IFNULL(DB, ''), COMMAND, TIME, IFNULL(STATE, ''), IFNULL(INFO, '')
		FROM information_schema.PROCESSLIST
		WHERE COMMAND != 'Sleep' AND (TIME >= 1 OR (INFO IS NOT NULL AND INFO != ''))
		ORDER BY TIME DESC
		LIMIT 5
	`)
	if qErr == nil {
		defer qRows.Close()
		for qRows.Next() {
			var q driver.FlightQueryInfo
			if err := qRows.Scan(&q.ThreadID, &q.User, &q.Host, &q.DB, &q.Command, &q.Time, &q.State, &q.Info); err == nil {
				sample.ActiveQueries = append(sample.ActiveQueries, q)
			}
		}
	}

	return sample, nil
}

func (s *MySQLSession) Close() error {
	return s.db.Close()
}

func (s *MySQLSession) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *MySQLSession) QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error) {
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

// InspectServerVariables introspects all server variables and correlates them with tuning rules.
func (s *MySQLSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	rows, err := s.db.QueryContext(ctx, "SHOW GLOBAL VARIABLES")
	if err != nil {
		return nil, fmt.Errorf("failed to introspect global variables: %w", err)
	}
	defer rows.Close()

	allVars := make(map[string]string)
	type kv struct {
		k, v string
	}
	var pairs []kv

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			allVars[k] = v
			pairs = append(pairs, kv{k: k, v: v})
		}
	}

	var version string
	_ = s.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)

	var vars []driver.ServerVariable
	curatedCount := 0
	for _, p := range pairs {
		sv := advisor.EvaluateMySQLVariable(p.k, p.v, allVars)
		if sv.IsCurated {
			curatedCount++
		}
		vars = append(vars, sv)
	}

	return &driver.ServerVariablesReport{
		Engine:       "mysql",
		Version:      version,
		TotalCount:   len(vars),
		CuratedCount: curatedCount,
		Variables:    vars,
	}, nil
}

