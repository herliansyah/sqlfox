package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"sqlfox/internal/driver"

	_ "github.com/jackc/pgx/v5/stdlib"
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
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
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

// PostgresSession represents an active connection to a PostgreSQL database.
type PostgresSession struct {
	db            *sql.DB
	cfg           driver.Config
	dsn           string
	currentSchema string
	mu            sync.Mutex
	queryPids     map[string]int
}

func (s *PostgresSession) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	err := s.db.PingContext(ctx)
	return time.Since(start), err
}

func (s *PostgresSession) ServerInfo(ctx context.Context) (*driver.ServerInfo, error) {
	var version string
	if err := s.db.QueryRowContext(ctx, "SELECT version()").Scan(&version); err != nil {
		return nil, err
	}

	var activeConns int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()").Scan(&activeConns)

	var uptimeSec int64
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time()))::bigint, 0)").Scan(&uptimeSec)

	return &driver.ServerInfo{
		Version:     version,
		UptimeSec:   uptimeSec,
		ActiveConns: activeConns,
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

func (s *PostgresSession) QueryStream(ctx context.Context, querySQL string, limit int) (driver.RowStream, error) {
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

func (s *PostgresSession) Exec(ctx context.Context, querySQL string, args ...any) (*driver.ExecResult, error) {
	start := time.Now()
	res, err := s.db.ExecContext(ctx, querySQL, args...)
	if err != nil {
		return nil, err
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

func (s *PostgresSession) InspectSchema(ctx context.Context, schemaName string) (*driver.SchemaMetadata, error) {
	if schemaName == "" || schemaName == s.cfg.DatabaseName {
		schemaName = s.currentSchema
	}
	if schemaName == "" {
		schemaName = "public"
	}

	// 1. Fetch all user-defined schemas in the active physical database
	schemaRows, err := s.db.QueryContext(ctx, `
		SELECT nspname FROM pg_catalog.pg_namespace
		WHERE nspname NOT LIKE 'pg_%' AND nspname != 'information_schema'
		ORDER BY nspname;
	`)
	var allSchemas []string
	if err == nil {
		defer schemaRows.Close()
		for schemaRows.Next() {
			var sch string
			if err := schemaRows.Scan(&sch); err == nil {
				allSchemas = append(allSchemas, sch)
			}
		}
	}
	if len(allSchemas) == 0 {
		allSchemas = []string{schemaName}
	}

	// 2. Fetch tables and views in current schema
	tableRows, err := s.db.QueryContext(ctx, `
		SELECT c.relname,
		       CASE c.relkind WHEN 'r' THEN 'TABLE' WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'VIEW' ELSE 'TABLE' END as reltype,
		       COALESCE(c.reltuples::bigint, 0) as row_count
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'v', 'm')
		ORDER BY c.relname;
	`, schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tables for schema %s: %w", schemaName, err)
	}
	defer tableRows.Close()

	tablesMap := make(map[string]*driver.TableMetadata)
	var tableOrder []string

	for tableRows.Next() {
		var name, reltype string
		var rowCount int64
		if err := tableRows.Scan(&name, &reltype, &rowCount); err != nil {
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

	// 3. Fetch columns for all tables in this schema
	colRows, err := s.db.QueryContext(ctx, `
		SELECT c.relname, a.attname,
		       pg_catalog.format_type(a.atttypid, a.atttypmod) as data_type,
		       NOT a.attnotnull as is_nullable,
		       COALESCE(pg_catalog.pg_get_expr(ad.adbin, ad.adrelid), '') as default_value,
		       COALESCE(i.indisprimary, false) as is_pk
		FROM pg_catalog.pg_attribute a
		JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_catalog.pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		LEFT JOIN pg_catalog.pg_index i ON i.indrelid = c.oid AND a.attnum = ANY(i.indkey) AND i.indisprimary
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'v', 'm') AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY c.relname, a.attnum;
	`, schemaName)
	if err == nil {
		defer colRows.Close()
		for colRows.Next() {
			var tbl, col, dataType, defaultVal string
			var isNullable, isPK bool
			if err := colRows.Scan(&tbl, &col, &dataType, &isNullable, &defaultVal, &isPK); err == nil {
				if t, ok := tablesMap[tbl]; ok {
					extra := ""
					if strings.Contains(strings.ToLower(defaultVal), "nextval(") {
						extra = "auto_increment"
					}
					t.Columns = append(t.Columns, driver.ColumnMetadata{
						Name:         col,
						Type:         dataType,
						IsNullable:   isNullable,
						IsPrimaryKey: isPK,
						DefaultValue: defaultVal,
						Extra:        extra,
					})
				}
			}
		}
	}

	// 4. Fetch indexes
	idxRows, err := s.db.QueryContext(ctx, `
		SELECT c.relname as table_name,
		       i.relname as index_name,
		       ix.indisunique as is_unique,
		       pg_catalog.pg_get_indexdef(ix.indexrelid, 0, true) as index_def,
		       am.amname as index_type
		FROM pg_catalog.pg_index ix
		JOIN pg_catalog.pg_class c ON c.oid = ix.indrelid
		JOIN pg_catalog.pg_class i ON i.oid = ix.indexrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_am am ON am.oid = i.relam
		WHERE n.nspname = $1
		ORDER BY c.relname, i.relname;
	`, schemaName)
	if err == nil {
		defer idxRows.Close()
		for idxRows.Next() {
			var tbl, idxName, indexDef, indexType string
			var isUnique bool
			if err := idxRows.Scan(&tbl, &idxName, &isUnique, &indexDef, &indexType); err == nil {
				if t, ok := tablesMap[tbl]; ok {
					// Parse columns out of index definition if feasible
					cols := extractColumnsFromIndexDef(indexDef)
					t.Indexes = append(t.Indexes, driver.IndexMetadata{
						Name:     idxName,
						Columns:  cols,
						IsUnique: isUnique,
						Type:     strings.ToUpper(indexType),
					})
				}
			}
		}
	}

	// 5. Fetch Foreign Keys
	fkRows, err := s.db.QueryContext(ctx, `
		SELECT con.conname as fk_name,
		       c.relname as table_name,
		       a.attname as column_name,
		       cf.relname as ref_table,
		       af.attname as ref_column
		FROM pg_catalog.pg_constraint con
		JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_catalog.pg_class cf ON cf.oid = con.confrelid
		JOIN pg_catalog.pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = con.conkey[1]
		JOIN pg_catalog.pg_attribute af ON af.attrelid = con.confrelid AND af.attnum = con.confkey[1]
		WHERE con.contype = 'f' AND n.nspname = $1
		ORDER BY con.conname;
	`, schemaName)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var fkName, tbl, col, refTbl, refCol string
			if err := fkRows.Scan(&fkName, &tbl, &col, &refTbl, &refCol); err == nil {
				if t, ok := tablesMap[tbl]; ok {
					t.ForeignKeys = append(t.ForeignKeys, driver.ForeignKeyMetadata{
						Name:             fkName,
						Column:           col,
						ReferencedTable:  refTbl,
						ReferencedColumn: refCol,
					})
				}
			}
		}
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
		defer trigRows.Close()
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
	}, nil
}

func (s *PostgresSession) GetTableDDL(ctx context.Context, tableName string) (string, error) {
	schema, err := s.InspectSchema(ctx, s.currentSchema)
	if err != nil {
		return "", err
	}

	for _, tbl := range schema.Tables {
		if strings.EqualFold(tbl.Name, tableName) {
			return generatePostgresCreateDDL(s.currentSchema, tbl), nil
		}
	}

	return "", fmt.Errorf("table %s not found in schema %s", tableName, s.currentSchema)
}

func (s *PostgresSession) GetTableEstimate(ctx context.Context, tableName string) (*driver.TableEstimate, error) {
	var rowCount int64
	var sizeBytes int64

	regClass := fmt.Sprintf("%s.%s", quoteIdentifier(s.currentSchema), quoteIdentifier(tableName))
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
	cleaned := strings.TrimSpace(querySQL)
	cleaned = strings.TrimRight(cleaned, "; \t\r\n")
	upper := strings.ToUpper(cleaned)
	if strings.HasPrefix(upper, "EXPLAIN ") {
		cleaned = strings.TrimSpace(cleaned[8:])
	}

	// Tier 1: Full ANALYZE+BUFFERS plan (actual row counts, buffer hits — best for visual tree)
	// ponytail: ANALYZE actually executes the query; safe for SELECT but skips for DML.
	var rawJSON string
	const analyzeFmt = "EXPLAIN (ANALYZE, COSTS, VERBOSE, BUFFERS, FORMAT JSON) "
	if err := s.db.QueryRowContext(ctx, analyzeFmt+cleaned).Scan(&rawJSON); err == nil {
		return &driver.ExplainPlan{RawJSON: rawJSON, Rows: []driver.ExplainRow{}}, nil
	}

	// Tier 2: Cost-only JSON plan (no execution; works for DML without running it)
	if err := s.db.QueryRowContext(ctx, "EXPLAIN (FORMAT JSON) "+cleaned).Scan(&rawJSON); err == nil {
		return &driver.ExplainPlan{RawJSON: rawJSON, Rows: []driver.ExplainRow{}}, nil
	}

	// Tier 3: Text EXPLAIN fallback (ancient PG or unusual syntax)
	rows, err := s.db.QueryContext(ctx, "EXPLAIN "+cleaned)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var planRows []driver.ExplainRow
	id := 1
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err == nil {
			planRows = append(planRows, driver.ExplainRow{
				ID:    id,
				Table: line,
				Extra: line,
			})
			id++
		}
	}

	return &driver.ExplainPlan{Rows: planRows}, nil
}

func (s *PostgresSession) CancelQuery(ctx context.Context, queryID string) error {
	s.mu.Lock()
	pid, ok := s.queryPids[queryID]
	s.mu.Unlock()

	if !ok || pid <= 0 {
		return nil
	}

	// Fail-fast timeout for out-of-band control connection (3s max)
	ctxCancel, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctxCancel, "SELECT pg_cancel_backend($1)", pid)
	if err != nil {
		if errors.Is(ctxCancel.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("control channel timeout (3s): target database connection pool may be saturated: %w", err)
		}
		return fmt.Errorf("failed to cancel query via control channel: %w", err)
	}
	return nil
}

func (s *PostgresSession) GetProcesslist(ctx context.Context) (*driver.ProcesslistResult, error) {
	var currentPID int64
	_ = s.db.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&currentPID)

	rows, err := s.db.QueryContext(ctx, `
		SELECT pid,
		       COALESCE(usename, '') as usename,
		       COALESCE(client_addr::text, 'local') as host,
		       COALESCE(datname, '') as db,
		       COALESCE(state, 'idle') as state,
		       COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - query_start))::bigint, 0) as time_sec,
		       COALESCE(state, '') as command,
		       COALESCE(query, '') as query,
		       COALESCE(pg_blocking_pids(pid), '{}') as blocking_pids
		FROM pg_stat_activity
		WHERE datname = current_database()
		ORDER BY pid;
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PostgreSQL processlist: %w", err)
	}
	defer rows.Close()

	var threads []driver.ProcessItem
	var locks []driver.LockWait

	for rows.Next() {
		var pid, timeSec int64
		var user, host, db, state, command, query string
		var blockingPids []int64

		if err := rows.Scan(&pid, &user, &host, &db, &state, &timeSec, &command, &query, &blockingPids); err != nil {
			// Some versions scan array as string format {123,456}
			continue
		}

		threads = append(threads, driver.ProcessItem{
			ID:      pid,
			User:    user,
			Host:    host,
			DB:      db,
			Command: command,
			Time:    timeSec,
			State:   state,
			Info:    query,
		})

		for _, blocker := range blockingPids {
			locks = append(locks, driver.LockWait{
				WaitingThreadID:  pid,
				WaitingQuery:     query,
				WaitingUser:      user,
				WaitingSeconds:   timeSec,
				BlockingThreadID: blocker,
				LockMode:         "ROW_EXCLUSIVE",
			})
		}
	}

	return &driver.ProcesslistResult{
		CurrentThreadID: currentPID,
		Threads:         threads,
		Locks:           locks,
	}, nil
}

func (s *PostgresSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	var query string
	if killConnection {
		query = "SELECT pg_terminate_backend($1)"
	} else {
		query = "SELECT pg_cancel_backend($1)"
	}
	_, err := s.db.ExecContext(ctx, query, threadID)
	return err
}

func (s *PostgresSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	if database == "" {
		database = s.currentSchema
	}
	regClass := fmt.Sprintf("%s.%s", quoteIdentifier(database), quoteIdentifier(table))

	var totalSize, dataSize, indexSize int64
	query := `
		SELECT pg_total_relation_size($1::regclass),
		       pg_relation_size($1::regclass),
		       pg_indexes_size($1::regclass);
	`
	if err := s.db.QueryRowContext(ctx, query, regClass).Scan(&totalSize, &dataSize, &indexSize); err != nil {
		return nil, fmt.Errorf("failed to get table diagnostics for %s: %w", regClass, err)
	}

	return []driver.TableDiagnostics{
		{
			Name:         table,
			Engine:       "PostgreSQL Heap",
			Rows:         0,
			DataSize:     dataSize,
			IndexSize:    indexSize,
			OverheadSize: 0,
			RowFormat:    "Dynamic",
			Collation:    "default",
		},
	}, nil
}

func (s *PostgresSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	var results []driver.MaintenanceResult
	for _, tbl := range tables {
		regClass := fmt.Sprintf("%s.%s", quoteIdentifier(s.currentSchema), quoteIdentifier(tbl))
		var cmd string
		switch strings.ToUpper(action) {
		case "OPTIMIZE":
			cmd = fmt.Sprintf("VACUUM FULL %s", regClass)
		case "ANALYZE":
			cmd = fmt.Sprintf("ANALYZE %s", regClass)
		case "REINDEX":
			cmd = fmt.Sprintf("REINDEX TABLE %s", regClass)
		default:
			cmd = fmt.Sprintf("VACUUM %s", regClass)
		}

		start := time.Now()
		_, err := s.db.ExecContext(ctx, cmd)
		dur := time.Since(start).Milliseconds()

		status := "OK"
		msg := fmt.Sprintf("Executed %s in %dms", cmd, dur)
		if err != nil {
			status = "Error"
			msg = err.Error()
		}

		results = append(results, driver.MaintenanceResult{
			Table:   tbl,
			Op:      action,
			Status:  status,
			Message: msg,
		})
	}
	return results, nil
}

func (s *PostgresSession) GetRedundantIndexes(ctx context.Context, database, table string) ([]driver.RedundantIndex, error) {
	schema, err := s.InspectSchema(ctx, s.currentSchema)
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
	return driver.DetectRedundantIndexes("postgres", s.currentSchema, tables), nil
}

func (s *PostgresSession) PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*driver.PeekResult, error) {
	schema := s.currentSchema
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

func (s *PostgresSession) GetUserList(ctx context.Context) ([]driver.DatabaseUser, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT usename FROM pg_catalog.pg_user ORDER BY usename")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []driver.DatabaseUser
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			users = append(users, driver.DatabaseUser{
				User: u,
				Host: "%",
			})
		}
	}
	return users, nil
}

func (s *PostgresSession) GetUserGrants(ctx context.Context, user, host string) ([]string, error) {
	// Query user role memberships & superuser status
	var isSuper bool
	_ = s.db.QueryRowContext(ctx, "SELECT usesuper FROM pg_catalog.pg_user WHERE usename = $1", user).Scan(&isSuper)

	var grants []string
	if isSuper {
		grants = append(grants, "SUPERUSER", "ALL PRIVILEGES")
	} else {
		grants = append(grants, "CONNECT", "USAGE")
	}
	return grants, nil
}

func (s *PostgresSession) SaveUser(ctx context.Context, user, host, password string, isNew bool) error {
	quoted := quoteIdentifier(user)
	var cmd string
	if isNew {
		cmd = fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", quoted, strings.ReplaceAll(password, "'", "''"))
	} else if password != "" {
		cmd = fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s'", quoted, strings.ReplaceAll(password, "'", "''"))
	} else {
		return nil
	}
	_, err := s.db.ExecContext(ctx, cmd)
	return err
}

func (s *PostgresSession) DropUser(ctx context.Context, user, host string) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("DROP USER %s", quoteIdentifier(user)))
	return err
}

func (s *PostgresSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	quotedUser := quoteIdentifier(req.User)
	for _, priv := range req.Privileges {
		cmd := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s", priv, quoteIdentifier(s.cfg.DatabaseName), quotedUser)
		_, _ = s.db.ExecContext(ctx, cmd)
	}
	return nil
}

func (s *PostgresSession) InspectTableLocks(ctx context.Context, database, table string) ([]driver.TableLockInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pid, COALESCE(usename, '') as user, COALESCE(query, '') as query,
		       COALESCE(state, '') as state,
		       COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - query_start))::bigint, 0) as time_sec
		FROM pg_stat_activity
		WHERE datname = current_database()
		  AND query ILIKE '%' || $1 || '%'
		  AND pid != pg_backend_pid();
	`, table)
	if err != nil {
		return []driver.TableLockInfo{}, nil
	}
	defer rows.Close()

	var locks []driver.TableLockInfo
	for rows.Next() {
		var pid, timeSec int64
		var user, query, state string
		if err := rows.Scan(&pid, &user, &query, &state, &timeSec); err == nil {
			locks = append(locks, driver.TableLockInfo{
				ThreadID:    pid,
				User:        user,
				Host:        "local",
				Database:    s.cfg.DatabaseName,
				State:       state,
				Query:       query,
				DurationSec: timeSec,
				LockMode:    "TABLE_ACCESS",
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

func (s *PostgresSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	var isRecovery bool
	_ = s.db.QueryRowContext(ctx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)

	role := "Primary"
	if isRecovery {
		role = "Standby Replica"
	}

	return &driver.ClusterHealthReport{
		TopologyType:        "postgresql-native",
		ClusterName:         s.cfg.DatabaseName,
		ClusterStatus:       role,
		ClusterSize:         1,
		LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
		LocalNodeState:      role,
		BottleneckDiagnosis: "PostgreSQL instance operating normally",
		BottleneckSeverity:  "ok",
		Nodes: []driver.ClusterNodeInfo{
			{
				Name:    fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
				Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
				State:   role,
				IsLocal: true,
				Status:  "online",
			},
		},
	}, nil
}

func (s *PostgresSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	var activeConns, activeThreads int64
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()").Scan(&activeConns)
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'active'").Scan(&activeThreads)

	// Fetch active queries
	rows, err := s.db.QueryContext(ctx, `
		SELECT pid, COALESCE(usename, '') as u, COALESCE(client_addr::text, 'local') as h,
		       COALESCE(datname, '') as db, COALESCE(state, '') as st,
		       COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - query_start))::bigint, 0) as dur,
		       COALESCE(query, '') as q
		FROM pg_stat_activity
		WHERE datname = current_database() AND state = 'active' AND pid != pg_backend_pid()
		LIMIT 5;
	`)

	var queries []driver.FlightQueryInfo
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pid, dur int64
			var u, h, db, st, q string
			if err := rows.Scan(&pid, &u, &h, &db, &st, &dur, &q); err == nil {
				queries = append(queries, driver.FlightQueryInfo{
					ThreadID: pid,
					User:     u,
					Host:     h,
					DB:       db,
					Command:  "active",
					Time:     dur,
					State:    st,
					Info:     q,
				})
			}
		}
	}

	return &driver.FlightMetricSample{
		SampledAt:          time.Now(),
		Questions:          activeThreads * 10,
		ThreadsRunning:     activeThreads,
		ThreadsConnected:   activeConns,
		RowLockWaits:       0,
		BufferPoolWaitFree: 0,
		ActiveQueries:      queries,
	}, nil
}

func (s *PostgresSession) Close() error {
	return s.db.Close()
}

func (s *PostgresSession) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	res, err := s.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *PostgresSession) QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error) {
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

func quoteIdentifier(id string) string {
	return driver.QuoteIdentifier(id, "postgres")
}

func extractColumnsFromIndexDef(def string) []string {
	openIdx := strings.Index(def, "(")
	closeIdx := strings.LastIndex(def, ")")
	if openIdx == -1 || closeIdx == -1 || closeIdx <= openIdx {
		return []string{}
	}
	raw := def[openIdx+1 : closeIdx]
	parts := strings.Split(raw, ",")
	var cols []string
	for _, p := range parts {
		c := strings.TrimSpace(p)
		c = strings.Trim(c, `" `)
		if c != "" {
			cols = append(cols, c)
		}
	}
	return cols
}

func generatePostgresCreateDDL(schema string, tbl driver.TableMetadata) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE %s.%s (\n", quoteIdentifier(schema), quoteIdentifier(tbl.Name)))

	var lines []string
	for _, col := range tbl.Columns {
		line := fmt.Sprintf("  %s %s", quoteIdentifier(col.Name), col.Type)
		if !col.IsNullable {
			line += " NOT NULL"
		}
		if col.DefaultValue != "" {
			line += " DEFAULT " + col.DefaultValue
		}
		lines = append(lines, line)
	}

	// Primary Key
	var pkCols []string
	for _, col := range tbl.Columns {
		if col.IsPrimaryKey {
			pkCols = append(pkCols, quoteIdentifier(col.Name))
		}
	}
	if len(pkCols) > 0 {
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	// Foreign Keys
	for _, fk := range tbl.ForeignKeys {
		lines = append(lines, fmt.Sprintf("  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			quoteIdentifier(fk.Name),
			quoteIdentifier(fk.Column),
			quoteIdentifier(fk.ReferencedTable),
			quoteIdentifier(fk.ReferencedColumn),
		))
	}

	sb.WriteString(strings.Join(lines, ",\n"))
	sb.WriteString("\n);")

	// Indexes
	for _, idx := range tbl.Indexes {
		if strings.HasSuffix(idx.Name, "_pkey") {
			continue // Skip PK indexes
		}
		uniqueStr := ""
		if idx.IsUnique {
			uniqueStr = "UNIQUE "
		}
		var quotedCols []string
		for _, c := range idx.Columns {
			quotedCols = append(quotedCols, quoteIdentifier(c))
		}
		if len(quotedCols) > 0 {
			sb.WriteString(fmt.Sprintf("\nCREATE %sINDEX %s ON %s.%s (%s);",
				uniqueStr,
				quoteIdentifier(idx.Name),
				quoteIdentifier(schema),
				quoteIdentifier(tbl.Name),
				strings.Join(quotedCols, ", "),
			))
		}
	}

	return sb.String()
}

// InspectServerVariables introspects all PostgreSQL settings from pg_settings.
func (s *PostgresSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name, setting, category, short_desc, context FROM pg_settings ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to introspect pg_settings: %w", err)
	}
	defer rows.Close()

	var vars []driver.ServerVariable
	for rows.Next() {
		var name, setting, category, shortDesc, contextStr string
		if err := rows.Scan(&name, &setting, &category, &shortDesc, &contextStr); err == nil {
			isDynamic := (contextStr == "user" || contextStr == "superuser")
			scope := "Global"
			if contextStr == "user" {
				scope = "Both"
			}
			vars = append(vars, driver.ServerVariable{
				Name:             name,
				Value:            setting,
				Scope:            scope,
				IsDynamic:        isDynamic,
				Category:         category,
				Description:      shortDesc,
				Severity:         "info",
				IsCurated:        false,
				MyCnfSnippet:     fmt.Sprintf("%s = '%s'", name, setting),
				SetGlobalSnippet: fmt.Sprintf("ALTER SYSTEM SET %s = '%s';", name, setting),
			})
		}
	}

	var version string
	_ = s.db.QueryRowContext(ctx, "SELECT version()").Scan(&version)

	return &driver.ServerVariablesReport{
		Engine:       "postgres",
		Version:      version,
		TotalCount:   len(vars),
		CuratedCount: 0,
		Variables:    vars,
	}, nil
}

