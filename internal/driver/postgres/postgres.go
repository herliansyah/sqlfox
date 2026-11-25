package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"net"
	"net/url"
	"os"
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

	sslMode := strings.ToLower(strings.TrimSpace(cfg.SSLMode))
	if sslMode == "" {
		if cfg.UseSSL {
			// Secure-by-default: verify CA and server hostname (prevents MITM).
			sslMode = "verify-full"
		} else {
			sslMode = "disable"
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
	// Always pass disable to ParseConfig when building an explicit in-memory tls.Config so inline PEM
	// certificates and strict verify-full / verify-ca / require semantics work identically to MySQL.
	if sslMode == "disable" {
		q.Set("sslmode", "disable")
	} else {
		q.Set("sslmode", "require")
	}
	u.RawQuery = q.Encode()

	pgxCfg, err := pgx.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgresql config: %w", err)
	}

	if sslMode != "disable" {
		tlsConfig, err := buildPostgresTLSConfig(cfg, sslMode)
		if err != nil {
			return nil, err
		}
		pgxCfg.TLSConfig = tlsConfig
	}

	db := stdlib.OpenDB(*pgxCfg)
	if db == nil {
		return nil, fmt.Errorf("failed to open postgresql connection")
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
		connStates:    driver.NewConnStateMap[pgConnState](64),
	}, nil
}

func buildPostgresTLSConfig(cfg driver.Config, sslMode string) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: cfg.Host,
	}

	if cfg.SSLCA != "" {
		var caBytes []byte
		if strings.Contains(cfg.SSLCA, "-----BEGIN ") {
			caBytes = []byte(cfg.SSLCA)
		} else {
			var err error
			caBytes, err = os.ReadFile(cfg.SSLCA)
			if err != nil {
				return nil, fmt.Errorf("failed to read PostgreSQL SSL CA certificate: %w", err)
			}
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("failed to parse PostgreSQL SSL CA certificate PEM")
		}
		tlsCfg.RootCAs = pool
	}

	if cfg.SSLCert != "" && cfg.SSLKey != "" {
		var cert tls.Certificate
		var err error
		if strings.Contains(cfg.SSLCert, "-----BEGIN ") || strings.Contains(cfg.SSLKey, "-----BEGIN ") {
			cert, err = tls.X509KeyPair([]byte(cfg.SSLCert), []byte(cfg.SSLKey))
		} else {
			cert, err = tls.LoadX509KeyPair(cfg.SSLCert, cfg.SSLKey)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load PostgreSQL SSL client certificate/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	switch sslMode {
	case "require", "prefer", "allow":
		tlsCfg.InsecureSkipVerify = true
	case "verify-ca":
		tlsCfg.InsecureSkipVerify = true
		tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("server presented no TLS certificate")
			}
			certs := make([]*x509.Certificate, len(rawCerts))
			for i, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					return err
				}
				certs[i] = c
			}
			opts := x509.VerifyOptions{
				Roots:         tlsCfg.RootCAs,
				Intermediates: x509.NewCertPool(),
			}
			for _, ic := range certs[1:] {
				opts.Intermediates.AddCert(ic)
			}
			_, err := certs[0].Verify(opts)
			return err
		}
	case "verify-full":
		tlsCfg.InsecureSkipVerify = false
	default:
		return nil, fmt.Errorf("unsupported PostgreSQL sslmode %q", sslMode)
	}
	return tlsCfg, nil
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
	connStates    *driver.ConnStateMap[pgConnState]
}

func (s *PostgresSession) getConnState(conn *sql.Conn) *pgConnState {
	if s.connStates == nil {
		return nil
	}
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

	// Never hold s.mu across network I/O, and always execute SET search_path on an explicit
	// *sql.Conn whose connStates cache entry we update atomically to prevent pool poisoning.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire PostgreSQL connection: %w", err)
	}
	defer conn.Close()

	// PostgreSQL physical database name != schema name. If caller passed the physical
	// database name (e.g. default session.DatabaseName), verify whether a schema with
	// that name actually exists; if not, keep/fall back to activeSchema ("public").
	if schemaName == s.cfg.DatabaseName && schemaName != "public" {
		var exists bool
		_ = conn.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace WHERE nspname = $1)", schemaName,
		).Scan(&exists)
		if !exists {
			if cur := s.activeSchema(); cur != "" {
				schemaName = cur
			} else {
				schemaName = "public"
			}
		}
	}

	quoted := quoteIdentifier(schemaName)
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("SET search_path = %s", quoted)); err != nil {
		return fmt.Errorf("failed to switch PostgreSQL schema to %s: %w", schemaName, err)
	}
	if st := s.getConnState(conn); st != nil {
		st.mu.Lock()
		st.lastSchema = schemaName
		st.mu.Unlock()
	}

	s.mu.Lock()
	s.currentSchema = schemaName
	s.mu.Unlock()
	return nil
}

// activeSchema returns the current schema; SelectDatabase may change it concurrently.
func (s *PostgresSession) activeSchema() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentSchema
}

func (s *PostgresSession) applyActiveSchema(ctx context.Context, conn *sql.Conn) error {
	schema := driver.DatabaseFromContext(ctx)
	if schema == "" {
		schema = s.activeSchema()
	}
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

func (s *PostgresSession) invalidateConnSchemaState(conn *sql.Conn) {
	if st := s.getConnState(conn); st != nil {
		st.mu.Lock()
		st.lastSchema = ""
		st.mu.Unlock()
	}
}

func isRawSearchPathChangePG(q string) bool {
	up := strings.ToUpper(driver.StripLeadingComments(q))
	return strings.Contains(up, "SEARCH_PATH") && (strings.HasPrefix(up, "SET ") || strings.HasPrefix(up, "RESET "))
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
	if isRawSearchPathChangePG(querySQL) {
		s.invalidateConnSchemaState(conn)
	}
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
	sess *PostgresSession
	conn *sql.Conn
	tx   *sql.Tx
	pid  int
}

func (t *postgresTx) trackQuery(ctx context.Context) func() {
	queryID := driver.QueryIDFromContext(ctx)
	if queryID == "" || t.sess == nil || t.pid <= 0 {
		return func() {}
	}
	t.sess.mu.Lock()
	t.sess.queryPids[queryID] = t.pid
	t.sess.mu.Unlock()
	return func() {
		t.sess.mu.Lock()
		delete(t.sess.queryPids, queryID)
		t.sess.mu.Unlock()
	}
}

func (t *postgresTx) closeConn() {
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
}

func (t *postgresTx) Exec(ctx context.Context, query string, args ...any) (*driver.ExecResult, error) {
	start := time.Now()
	untrack := t.trackQuery(ctx)
	defer untrack()
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
	untrack := t.trackQuery(ctx)
	defer untrack()
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
	untrack := t.trackQuery(ctx)

	rows, err := t.tx.QueryContext(ctx, querySQL)
	if err != nil {
		untrack()
		return nil, err
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		rows.Close()
		untrack()
		return nil, err
	}

	cols := driver.ColumnsFromTypes(colTypes)

	return driver.NewSQLRowStream(rows, cols, untrack), nil
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
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &postgresTx{sess: s, conn: conn, tx: tx, pid: pid}, nil
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
			rowMap[col] = driver.FormatValueForJSON(val)
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

// ExecScriptConn pins a single physical PostgreSQL connection for a multi-statement script
// so session settings and BEGIN/COMMIT blocks execute on the same backend.
func (s *PostgresSession) ExecScriptConn(ctx context.Context, fn func(exec func(context.Context, string) (*driver.ExecResult, error)) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = conn.ExecContext(restoreCtx, "ROLLBACK")
		cancel()
		s.invalidateConnSchemaState(conn)
		_ = conn.Close()
	}()
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		return err
	}
	return fn(func(c context.Context, q string) (*driver.ExecResult, error) {
		start := time.Now()
		res, err := conn.ExecContext(c, q)
		if isRawSearchPathChangePG(q) {
			s.invalidateConnSchemaState(conn)
		}
		if err != nil {
			return nil, err
		}
		rowsAff, _ := res.RowsAffected()
		return &driver.ExecResult{
			RowsAffected: rowsAff,
			ExecutionMs:  time.Since(start).Milliseconds(),
			Message:      fmt.Sprintf("Query OK, %d rows affected", rowsAff),
		}, nil
	})
}

// QueryStreamReadOnly enforces PostgreSQL engine-level READ ONLY transaction mode.
func (s *PostgresSession) QueryStreamReadOnly(ctx context.Context, sqlStr string, limit int) (driver.RowStream, error) {
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
	untrack := func() {
		if queryID != "" {
			s.mu.Lock()
			delete(s.queryPids, queryID)
			s.mu.Unlock()
		}
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		untrack()
		_ = conn.Close()
		return nil, err
	}
	querySQL := driver.ApplyLimitClause(sqlStr, limit)
	rows, err := tx.QueryContext(ctx, querySQL)
	if err != nil {
		untrack()
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		_ = rows.Close()
		untrack()
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, err
	}
	return driver.NewSQLRowStream(rows, driver.ColumnsFromTypes(colTypes), func() {
		untrack()
		_ = tx.Rollback()
		_ = conn.Close()
	}), nil
}

func quoteIdentifier(id string) string {
	return driver.QuoteIdentifier(id, "postgres")
}
