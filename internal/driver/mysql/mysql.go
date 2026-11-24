package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	gomysql "github.com/go-sql-driver/mysql"
	"github.com/herliansyah/sqlfox/internal/driver"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
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
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse MySQL SSL CA certificate PEM")
			}
			tlsConfig.RootCAs = caCertPool
		}

		if cfg.SSLCert != "" && cfg.SSLKey != "" {
			cert, err := tls.LoadX509KeyPair(cfg.SSLCert, cfg.SSLKey)
			if err != nil {
				cert, err = tls.X509KeyPair([]byte(cfg.SSLCert), []byte(cfg.SSLKey))
			}
			if err != nil {
				return nil, fmt.Errorf("failed to load MySQL SSL client certificate/key: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		tlsName := fmt.Sprintf("custom-tls-%d", time.Now().UnixNano())
		if err := gomysql.RegisterTLSConfig(tlsName, tlsConfig); err != nil {
			// Never silently downgrade to skip-verify when the user asked for TLS.
			return nil, fmt.Errorf("failed to register TLS config: %w", err)
		}
		tlsParam = tlsName
	}

	// 2. Assemble DSN safely via gomysql.Config.FormatDSN() (prevents parameter injection via user/password/db)
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	mc := gomysql.NewConfig()
	mc.User = cfg.User
	mc.Passwd = cfg.Password
	mc.Net = "tcp"
	mc.Addr = addr
	mc.DBName = cfg.DatabaseName
	mc.Loc = time.Local
	mc.ParseTime = true
	mc.MultiStatements = true
	mc.Timeout = 10 * time.Second
	mc.ReadTimeout = 60 * time.Second
	mc.WriteTimeout = 30 * time.Second
	mc.TLSConfig = tlsParam
	mc.Params = map[string]string{"charset": "utf8mb4"}
	dsn := mc.FormatDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		if strings.HasPrefix(tlsParam, "custom-tls-") {
			gomysql.DeregisterTLSConfig(tlsParam)
		}
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	// Bounded connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		if strings.HasPrefix(tlsParam, "custom-tls-") {
			gomysql.DeregisterTLSConfig(tlsParam)
		}
		return nil, fmt.Errorf("failed to connect to MySQL server (%s:%d): %w", cfg.Host, cfg.Port, err)
	}

	var pfsVal int
	pfsEnabled := false
	if err := db.QueryRowContext(ctx, "SELECT @@performance_schema").Scan(&pfsVal); err == nil && pfsVal == 1 {
		pfsEnabled = true
	}

	tlsKey := ""
	if strings.HasPrefix(tlsParam, "custom-tls-") {
		tlsKey = tlsParam
	}

	return &MySQLSession{
		db:           db,
		cfg:          cfg,
		dsn:          dsn,
		tlsKey:       tlsKey,
		queryThreads: make(map[string]int64),
		connStates:   *driver.NewConnStateMap[mysqlConnState](64),
		pfsEnabled:   pfsEnabled,
	}, nil
}

type mysqlConnState struct {
	mu       sync.Mutex
	lastDB   string
	threadID int64
}

type MySQLSession struct {
	db           *sql.DB
	cfg          driver.Config
	dsn          string
	tlsKey       string
	mu           sync.Mutex
	queryThreads map[string]int64
	connStates   driver.ConnStateMap[mysqlConnState]
	pfsEnabled   bool
}

func (s *MySQLSession) getConnState(conn *sql.Conn) *mysqlConnState {
	return s.connStates.Get(conn)
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
	var activeThreads int
	if rows, err := s.db.QueryContext(ctx, "SHOW GLOBAL STATUS WHERE Variable_name IN ('Uptime', 'Threads_connected', 'Threads_running')"); err == nil {
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				switch k {
				case "Uptime":
					uptimeSec, _ = strconv.ParseInt(v, 10, 64)
				case "Threads_connected":
					activeConns, _ = strconv.Atoi(v)
				case "Threads_running":
					activeThreads, _ = strconv.Atoi(v)
				}
			}
		}
		rows.Close()
	}

	return &driver.ServerInfo{
		Version:       version,
		UptimeSec:     uptimeSec,
		ActiveConns:   activeConns,
		ActiveThreads: activeThreads,
	}, nil
}

func (s *MySQLSession) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
	// Apply implicit safe limit for unbounded SELECTs
	querySQL = driver.ApplyLimitClause(querySQL, limit)

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	queryID := driver.QueryIDFromContext(ctx)
	if queryID != "" {
		var threadID int64
		st := s.getConnState(conn)
		if st != nil {
			st.mu.Lock()
			threadID = st.threadID
			st.mu.Unlock()
		}
		if threadID == 0 {
			_ = conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&threadID)
			if st != nil && threadID > 0 {
				st.mu.Lock()
				st.threadID = threadID
				st.mu.Unlock()
			}
		}
		if threadID > 0 {
			s.mu.Lock()
			s.queryThreads[queryID] = threadID
			s.mu.Unlock()
		}
	}

	rows, qErr := conn.QueryContext(ctx, querySQL)
	if qErr != nil {
		if queryID != "" {
			s.mu.Lock()
			delete(s.queryThreads, queryID)
			s.mu.Unlock()
		}
		conn.Close()
		return nil, qErr
	}

	colTypes, cErr := rows.ColumnTypes()
	if cErr != nil {
		rows.Close()
		if queryID != "" {
			s.mu.Lock()
			delete(s.queryThreads, queryID)
			s.mu.Unlock()
		}
		conn.Close()
		return nil, cErr
	}

	cols := driver.ColumnsFromTypes(colTypes)
	cleanup := func() {
		if queryID != "" {
			s.mu.Lock()
			delete(s.queryThreads, queryID)
			s.mu.Unlock()
		}
		conn.Close()
	}
	return driver.NewSQLRowStream(rows, cols, cleanup), nil
}

// activeDB returns the current database; SelectDatabase may change it concurrently.
func (s *MySQLSession) activeDB() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.DatabaseName
}

func (s *MySQLSession) applyActiveDatabase(ctx context.Context, conn *sql.Conn) error {
	dbName := s.activeDB()
	if dbName == "" {
		return nil
	}
	st := s.getConnState(conn)
	if st != nil {
		st.mu.Lock()
		if st.lastDB == dbName {
			st.mu.Unlock()
			return nil
		}
		st.mu.Unlock()
	}
	// A failed USE must abort the statement; otherwise it runs against the conn's previous database.
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("USE `%s`", strings.ReplaceAll(dbName, "`", "``"))); err != nil {
		return fmt.Errorf("failed to select database %s: %w", dbName, err)
	}
	if st != nil {
		st.mu.Lock()
		st.lastDB = dbName
		st.mu.Unlock()
	}
	return nil
}

func (s *MySQLSession) Exec(ctx context.Context, querySQL string, args ...any) (*driver.ExecResult, error) {
	start := time.Now()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return nil, err
	}

	queryID := driver.QueryIDFromContext(ctx)
	if queryID != "" {
		var threadID int64
		_ = conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&threadID)
		if threadID > 0 {
			s.mu.Lock()
			s.queryThreads[queryID] = threadID
			s.mu.Unlock()
			defer func() {
				s.mu.Lock()
				delete(s.queryThreads, queryID)
				s.mu.Unlock()
			}()
		}
	}

	res, execErr := conn.ExecContext(ctx, querySQL, args...)
	if execErr != nil {
		return nil, execErr
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

type mysqlTx struct {
	conn *sql.Conn
	tx   *sql.Tx
}

func (t *mysqlTx) closeConn() {
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
}

func (t *mysqlTx) Exec(ctx context.Context, query string, args ...any) (*driver.ExecResult, error) {
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
		Message:      fmt.Sprintf("Query OK, %d rows affected.", ra),
	}, nil
}

func (t *mysqlTx) ExecRaw(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (t *mysqlTx) Commit() error {
	defer t.closeConn()
	return t.tx.Commit()
}

func (t *mysqlTx) Rollback() error {
	defer t.closeConn()
	return t.tx.Rollback()
}

func (t *mysqlTx) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
	querySQL = driver.ApplyLimitClause(querySQL, limit)

	rows, err := t.tx.QueryContext(ctx, querySQL)
	if err != nil {
		return nil, err
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		rows.Close()
		return nil, err
	}

	cols := driver.ColumnsFromTypes(colTypes)

	return driver.NewSQLRowStream(rows, cols), nil
}

func (s *MySQLSession) BeginTx(ctx context.Context) (driver.TxSession, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &mysqlTx{conn: conn, tx: tx}, nil
}

func (s *MySQLSession) CancelQuery(ctx context.Context, queryID string) error {
	type queryTarget struct {
		key string
		tid int64
	}
	var targets []queryTarget
	s.mu.Lock()
	if tid, ok := s.queryThreads[queryID]; ok && tid > 0 {
		targets = append(targets, queryTarget{key: queryID, tid: tid})
	} else if queryID == "active" || queryID == "" {
		for k, tid := range s.queryThreads {
			if tid > 0 {
				targets = append(targets, queryTarget{key: k, tid: tid})
			}
		}
	} else {
		for k, tid := range s.queryThreads {
			if strings.HasPrefix(k, queryID+"#") && tid > 0 {
				targets = append(targets, queryTarget{key: k, tid: tid})
			}
		}
	}
	s.mu.Unlock()

	if len(targets) == 0 {
		return nil
	}

	successfulKeys := make(map[string]int64)
	defer func() {
		s.mu.Lock()
		for key, tid := range successfulKeys {
			if s.queryThreads[key] == tid {
				delete(s.queryThreads, key)
			}
		}
		s.mu.Unlock()
	}()

	// Fail-fast timeout for out-of-band control connection (3s max)
	ctxCancel, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// The query may finish while the control connection is being dialed; its connection then
	// returns to the pool with the same CONNECTION_ID and could be running another tab's query.
	// Re-check targetKey mapping right before KILL so we never interrupt an unrelated statement.
	stillRunning := func(t queryTarget) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.queryThreads[t.key] == t.tid
	}

	isUnknownThread := func(err error) bool {
		if err == nil {
			return false
		}
		msg := err.Error()
		return strings.Contains(msg, "Unknown thread id") || strings.Contains(msg, "1094")
	}

	// ponytail: Dial independent single-shot control connection to bypass saturated s.db pool
	var remainingTargets []queryTarget
	if s.dsn != "" {
		controlDB, dErr := sql.Open("mysql", s.dsn)
		if dErr == nil {
			defer controlDB.Close()
			if conn, cErr := controlDB.Conn(ctxCancel); cErr == nil {
				defer conn.Close()
				for _, t := range targets {
					if !stillRunning(t) {
						successfulKeys[t.key] = t.tid
						continue
					}
					if _, kErr := conn.ExecContext(ctxCancel, fmt.Sprintf("KILL QUERY %d", t.tid)); kErr != nil {
						if isUnknownThread(kErr) {
							successfulKeys[t.key] = t.tid
						} else {
							remainingTargets = append(remainingTargets, t)
						}
					} else {
						successfulKeys[t.key] = t.tid
					}
				}
			} else {
				remainingTargets = targets
			}
		} else {
			remainingTargets = targets
		}
	} else {
		remainingTargets = targets
	}

	// Fallback to pool if independent connection dial failed or targets remain
	if len(remainingTargets) > 0 {
		if s.db != nil {
			var firstErr error
			for _, t := range remainingTargets {
				if !stillRunning(t) {
					successfulKeys[t.key] = t.tid
					continue
				}
				_, err := s.db.ExecContext(ctxCancel, fmt.Sprintf("KILL QUERY %d", t.tid))
				if err != nil {
					if isUnknownThread(err) {
						successfulKeys[t.key] = t.tid
					} else if firstErr == nil {
						firstErr = err
					}
				} else {
					successfulKeys[t.key] = t.tid
				}
			}
			if firstErr != nil {
				if errors.Is(ctxCancel.Err(), context.DeadlineExceeded) {
					return fmt.Errorf("control channel timeout (3s): target database connection pool may be saturated: %w", firstErr)
				}
				return fmt.Errorf("failed to cancel query via control channel: %w", firstErr)
			}
		} else {
			// ponytail: in headless test mocks without db or dsn, gracefully clear simulated thread mappings
			for _, t := range remainingTargets {
				successfulKeys[t.key] = t.tid
			}
		}
	}
	return nil
}

func (s *MySQLSession) PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*driver.PeekResult, error) {
	if database == "" {
		database = s.activeDB()
	}
	sqlCmd := fmt.Sprintf("SELECT * FROM %s.%s WHERE %s = ? LIMIT 1", driver.QuoteIdentifier(database, "mysql"), driver.QuoteIdentifier(table, "mysql"), driver.QuoteIdentifier(column, "mysql"))
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

func (s *MySQLSession) Close() error {
	if s.tlsKey != "" {
		gomysql.DeregisterTLSConfig(s.tlsKey)
		s.tlsKey = ""
	}
	return s.db.Close()
}

// ExecRaw/QueryRaw run on a dedicated connection with the active database/schema applied:
// a random pooled connection may carry another database from a previous USE/search_path.
func (s *MySQLSession) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return 0, err
	}
	res, err := conn.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *MySQLSession) QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, sql, args...)
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

// ExecScriptConn pins a single physical MySQL connection for the duration of a multi-statement
// script so SET FOREIGN_KEY_CHECKS, SET SQL_MODE, and BEGIN/COMMIT execute on the same session,
// and restores FOREIGN_KEY_CHECKS=1 before releasing the connection back to the pool.
func (s *MySQLSession) ExecScriptConn(ctx context.Context, fn func(exec func(context.Context, string) (*driver.ExecResult, error)) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = conn.ExecContext(restoreCtx, "SET FOREIGN_KEY_CHECKS = 1")
		cancel()
		_ = conn.Close()
	}()
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		return err
	}
	return fn(func(c context.Context, q string) (*driver.ExecResult, error) {
		start := time.Now()
		res, err := conn.ExecContext(c, q)
		if err != nil {
			return nil, err
		}
		rowsAff, _ := res.RowsAffected()
		lastID, _ := res.LastInsertId()
		return &driver.ExecResult{
			RowsAffected: rowsAff,
			LastInsertID: lastID,
			ExecutionMs:  time.Since(start).Milliseconds(),
			Message:      fmt.Sprintf("Query OK, %d rows affected", rowsAff),
		}, nil
	})
}

// QueryStreamReadOnly enforces MySQL engine-level READ ONLY transaction mode for read-only sessions.
func (s *MySQLSession) QueryStreamReadOnly(ctx context.Context, sqlStr string, limit int) (driver.RowStream, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.applyActiveDatabase(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	querySQL := driver.ApplyLimitClause(sqlStr, limit)
	rows, err := tx.QueryContext(ctx, querySQL)
	if err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		_ = rows.Close()
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, err
	}
	return driver.NewSQLRowStream(rows, driver.ColumnsFromTypes(colTypes), func() {
		_ = tx.Rollback()
		_ = conn.Close()
	}), nil
}

// InspectServerVariables introspects all server variables and correlates them with tuning rules.
