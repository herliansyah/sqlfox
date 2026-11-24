package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	driver.Register("postgres", &PostgresDriver{})
	driver.Register("postgresql", &PostgresDriver{})
	driver.Register("pg", &PostgresDriver{})
}

// PostgresDriver manages PostgreSQL connections via pgx/v5.
type PostgresDriver struct{}

func (d *PostgresDriver) Name() string {
	return "postgres"
}

func (d *PostgresDriver) Connect(ctx context.Context, cfg driver.Config) (driver.Session, error) {
	if cfg.Port <= 0 {
		cfg.Port = 5432
	}
	if cfg.DatabaseName == "" {
		cfg.DatabaseName = "postgres"
	}

	sslMode := "disable"
	if cfg.UseSSL {
		if cfg.SSLCA != "" {
			sslMode = "verify-ca"
		} else {
			sslMode = "require"
		}
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:   "/" + strings.TrimPrefix(cfg.DatabaseName, "/"),
	}
	if cfg.User != "" || cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}

	q := u.Query()
	q.Set("sslmode", sslMode)
	if cfg.SSLCA != "" {
		q.Set("sslrootcert", cfg.SSLCA)
	}
	if cfg.SSLCert != "" {
		q.Set("sslcert", cfg.SSLCert)
	}
	if cfg.SSLKey != "" {
		q.Set("sslkey", cfg.SSLKey)
	}
	u.RawQuery = q.Encode()

	db, err := sql.Open("pgx", u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to open postgresql connection: %w", err)
	}

	// Bounded connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to PostgreSQL server (%s:%d): %w", cfg.Host, cfg.Port, err)
	}

	return &PostgresSession{
		db:            db,
		cfg:           cfg,
		dsn:           u.String(),
		currentSchema: "public",
		queryPids:     make(map[string]int),
	}, nil
}

type pgConnState struct {
	mu         sync.Mutex
	lastSchema string
	pid        int
}

// PostgresSession represents an active connection to a PostgreSQL database.
type PostgresSession struct {
	db            *sql.DB
	cfg           driver.Config
	dsn           string
	currentSchema string
	mu            sync.Mutex
	queryPids     map[string]int
	connStates    driver.ConnStateMap[pgConnState]
}

func (s *PostgresSession) getConnState(conn *sql.Conn) *pgConnState {
	return s.connStates.Get(conn)
}

func (s *PostgresSession) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	err := s.db.PingContext(ctx)
	return time.Since(start), err
}

func (s *PostgresSession) ServerInfo(ctx context.Context) (*driver.ServerInfo, error) {
	var version string
	var uptimeSec int64
	var activeConns, activeThreads int
	err := s.db.QueryRowContext(ctx, `
		SELECT version(),
		       COALESCE(EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time()))::bigint, 0),
		       (SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()),
		       (SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND pid <> pg_backend_pid())
	`).Scan(&version, &uptimeSec, &activeConns, &activeThreads)
	if err != nil {
		return nil, err
	}

	return &driver.ServerInfo{
		Version:       version,
		UptimeSec:     uptimeSec,
		ActiveConns:   activeConns,
		ActiveThreads: activeThreads,
	}, nil
}

func (s *PostgresSession) SelectDatabase(ctx context.Context, schemaName string) error {
	if schemaName == "" {
		schemaName = "public"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	quoted := quoteIdentifier(schemaName)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("SET search_path = %s", quoted)); err != nil {
		return fmt.Errorf("failed to switch PostgreSQL schema to %s: %w", schemaName, err)
	}
	s.currentSchema = schemaName
	return nil
}

// activeSchema returns the current schema; SelectDatabase may change it concurrently.
func (s *PostgresSession) activeSchema() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentSchema
}

func (s *PostgresSession) applyActiveSchema(ctx context.Context, conn *sql.Conn) error {
	schema := s.activeSchema()
	if schema == "" {
		return nil
	}
	st := s.getConnState(conn)
	if st != nil {
		st.mu.Lock()
		if st.lastSchema == schema {
			st.mu.Unlock()
			return nil
		}
		st.mu.Unlock()
	}
	// A failed SET must abort the statement; otherwise unqualified names resolve in the default schema.
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("SET search_path = %s", quoteIdentifier(schema))); err != nil {
		return fmt.Errorf("failed to switch PostgreSQL schema to %s: %w", schema, err)
	}
	if st != nil {
		st.mu.Lock()
		st.lastSchema = schema
		st.mu.Unlock()
	}
	return nil
}

func (s *PostgresSession) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
	querySQL = driver.ApplyLimitClause(querySQL, limit)

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	queryID := driver.QueryIDFromContext(ctx)
	if queryID != "" {
		var pid int
		st := s.getConnState(conn)
		if st != nil {
			st.mu.Lock()
			pid = st.pid
			st.mu.Unlock()
		}
		if pid == 0 {
			_ = conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid)
			if st != nil && pid > 0 {
				st.mu.Lock()
				st.pid = pid
				st.mu.Unlock()
			}
		}
		if pid > 0 {
			s.mu.Lock()
			s.queryPids[queryID] = pid
			s.mu.Unlock()
		}
	}

	rows, qErr := conn.QueryContext(ctx, querySQL)
	if qErr != nil {
		if queryID != "" {
			s.mu.Lock()
			delete(s.queryPids, queryID)
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
			delete(s.queryPids, queryID)
			s.mu.Unlock()
		}
		conn.Close()
		return nil, cErr
	}

	cols := driver.ColumnsFromTypes(colTypes)
	cleanup := func() {
		if queryID != "" {
			s.mu.Lock()
			delete(s.queryPids, queryID)
			s.mu.Unlock()
		}
		conn.Close()
	}
	return driver.NewSQLRowStream(rows, cols, cleanup), nil
}

func (s *PostgresSession) Exec(ctx context.Context, querySQL string, args ...any) (*driver.ExecResult, error) {
	start := time.Now()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		return nil, err
	}

	queryID := driver.QueryIDFromContext(ctx)
	if queryID != "" {
		var pid int
		_ = conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid)
		if pid > 0 {
			s.mu.Lock()
			s.queryPids[queryID] = pid
			s.mu.Unlock()
			defer func() {
				s.mu.Lock()
				delete(s.queryPids, queryID)
				s.mu.Unlock()
			}()
		}
	}

	res, execErr := conn.ExecContext(ctx, querySQL, args...)
	if execErr != nil {
		return nil, execErr
	}
	duration := time.Since(start).Milliseconds()
	rowsAff, _ := res.RowsAffected()
	lastID, _ := res.LastInsertId()
	return &driver.ExecResult{
		RowsAffected: rowsAff,
		LastInsertID: lastID,
		ExecutionMs:  duration,
		Message:      fmt.Sprintf("Query executed successfully. %d rows affected.", rowsAff),
	}, nil
}

type postgresTx struct {
	conn *sql.Conn
	tx   *sql.Tx
}

func (t *postgresTx) closeConn() {
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
}

func (t *postgresTx) Exec(ctx context.Context, query string, args ...any) (*driver.ExecResult, error) {
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

func (t *postgresTx) ExecRaw(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (t *postgresTx) Commit() error {
	defer t.closeConn()
	return t.tx.Commit()
}

func (t *postgresTx) Rollback() error {
	defer t.closeConn()
	return t.tx.Rollback()
}

func (t *postgresTx) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
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

func (s *PostgresSession) BeginTx(ctx context.Context) (driver.TxSession, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &postgresTx{conn: conn, tx: tx}, nil
}

func (s *PostgresSession) CancelQuery(ctx context.Context, queryID string) error {
	type queryTarget struct {
		key string
		pid int
	}
	var targets []queryTarget
	s.mu.Lock()
	if pid, ok := s.queryPids[queryID]; ok && pid > 0 {
		targets = append(targets, queryTarget{key: queryID, pid: pid})
	} else if queryID == "active" || queryID == "" {
		for k, p := range s.queryPids {
			if p > 0 {
				targets = append(targets, queryTarget{key: k, pid: p})
			}
		}
	} else {
		for k, p := range s.queryPids {
			if strings.HasPrefix(k, queryID+"#") && p > 0 {
				targets = append(targets, queryTarget{key: k, pid: p})
			}
		}
	}
	s.mu.Unlock()

	if len(targets) == 0 {
		return nil
	}

	successfulKeys := make(map[string]int)
	defer func() {
		s.mu.Lock()
		for key, pid := range successfulKeys {
			if s.queryPids[key] == pid {
				delete(s.queryPids, key)
			}
		}
		s.mu.Unlock()
	}()

	// Fail-fast timeout for out-of-band control connection (3s max)
	ctxCancel, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// The query may finish while the control connection is being dialed and its backend be reused
	// by another query from the pool; re-check targetKey mapping right before cancelling.
	stillRunning := func(t queryTarget) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.queryPids[t.key] == t.pid
	}

	// ponytail: Dial independent single-shot control connection to bypass saturated s.db pool
	var remainingTargets []queryTarget
	if s.dsn != "" {
		controlDB, dErr := sql.Open("pgx", s.dsn)
		if dErr == nil {
			defer controlDB.Close()
			if conn, cErr := controlDB.Conn(ctxCancel); cErr == nil {
				defer conn.Close()
				for _, t := range targets {
					if !stillRunning(t) {
						successfulKeys[t.key] = t.pid
						continue
					}
					if _, kErr := conn.ExecContext(ctxCancel, "SELECT pg_cancel_backend($1)", t.pid); kErr != nil {
						remainingTargets = append(remainingTargets, t)
					} else {
						successfulKeys[t.key] = t.pid
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
					successfulKeys[t.key] = t.pid
					continue
				}
				_, err := s.db.ExecContext(ctxCancel, "SELECT pg_cancel_backend($1)", t.pid)
				if err != nil && firstErr == nil {
					firstErr = err
				} else if err == nil {
					successfulKeys[t.key] = t.pid
				}
			}
			if firstErr != nil {
				if errors.Is(ctxCancel.Err(), context.DeadlineExceeded) {
					return fmt.Errorf("control channel timeout (3s): target database connection pool may be saturated: %w", firstErr)
				}
				return fmt.Errorf("failed to cancel query via control channel: %w", firstErr)
			}
		} else {
			// ponytail: in headless test mocks without db or dsn, gracefully clear simulated PID mappings
			for _, t := range remainingTargets {
				successfulKeys[t.key] = t.pid
			}
		}
	}
	return nil
}

func (s *PostgresSession) PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*driver.PeekResult, error) {
	schema := s.activeSchema()
	if database != "" {
		schema = database
	}
	regClass := fmt.Sprintf("%s.%s", quoteIdentifier(schema), quoteIdentifier(table))
	sqlCmd := fmt.Sprintf("SELECT * FROM %s WHERE %s = $1 LIMIT 1", regClass, quoteIdentifier(column))

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

func (s *PostgresSession) Close() error {
	return s.db.Close()
}

// ExecRaw/QueryRaw run on a dedicated connection with the active database/schema applied:
// a random pooled connection may carry another database from a previous USE/search_path.
func (s *PostgresSession) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		return 0, err
	}
	res, err := conn.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *PostgresSession) QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := s.applyActiveSchema(ctx, conn); err != nil {
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

func quoteIdentifier(id string) string {
	return driver.QuoteIdentifier(id, "postgres")
}
