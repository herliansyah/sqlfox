package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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
			Name:     name,
			Type:     tableType,
			RowCount: rowCount,
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
					if idx, ok := tableIndexMap[tblName]; ok {
						tables[idx].Indexes = append(tables[idx].Indexes, *im)
					}
				}
			}
		}
		idxRows.Close()
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

	rows, err := s.db.QueryContext(ctx, "EXPLAIN "+cleanedSQL)
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

func (s *MySQLSession) CancelQuery(ctx context.Context, queryID string) error {
	s.mu.Lock()
	threadID, ok := s.queryThreads[queryID]
	s.mu.Unlock()

	if !ok || threadID <= 0 {
		return nil
	}

	// Execute out-of-band KILL QUERY on independent connection
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("KILL QUERY %d", threadID))
	return err
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

func (s *MySQLSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	if database == "" {
		database = s.cfg.DatabaseName
	}
	query := `
		SELECT 
			TABLE_NAME, 
			IFNULL(ENGINE, ''), 
			IFNULL(TABLE_ROWS, 0), 
			IFNULL(DATA_LENGTH, 0), 
			IFNULL(INDEX_LENGTH, 0), 
			IFNULL(DATA_FREE, 0), 
			IFNULL(ROW_FORMAT, ''), 
			IFNULL(TABLE_COLLATION, ''), 
			IFNULL(DATE_FORMAT(CREATE_TIME, '%Y-%m-%d %H:%i:%s'), ''), 
			IFNULL(DATE_FORMAT(UPDATE_TIME, '%Y-%m-%d %H:%i:%s'), '')
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ?
	`
	var args []any
	args = append(args, database)
	if table != "" {
		query += " AND TABLE_NAME = ?"
		args = append(args, table)
	}
	query += " ORDER BY (DATA_LENGTH + INDEX_LENGTH) DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query table diagnostics: %w", err)
	}
	defer rows.Close()

	var result []driver.TableDiagnostics
	for rows.Next() {
		var td driver.TableDiagnostics
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
		); err == nil {
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

func (s *MySQLSession) Close() error {
	return s.db.Close()
}
