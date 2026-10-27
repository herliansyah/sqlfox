package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
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

	var activeThreads int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND pid <> pg_backend_pid()").Scan(&activeThreads)

	var uptimeSec int64
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time()))::bigint, 0)").Scan(&uptimeSec)

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
	if schema != "" {
		// A failed SET must abort the statement; otherwise unqualified names resolve in the default schema.
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("SET search_path = %s", quoteIdentifier(schema))); err != nil {
			return fmt.Errorf("failed to switch PostgreSQL schema to %s: %w", schema, err)
		}
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
		_ = conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid)
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

	cols := make([]driver.ColumnDefinition, len(colTypes))
	for i, ct := range colTypes {
		nullable, _ := ct.Nullable()
		cols[i] = driver.ColumnDefinition{
			Name:     ct.Name(),
			Type:     ct.DatabaseTypeName(),
			Nullable: nullable,
		}
	}
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

func (s *PostgresSession) InspectSchema(ctx context.Context, schemaName string) (*driver.SchemaMetadata, error) {
	if schemaName == "" || schemaName == s.cfg.DatabaseName {
		schemaName = s.activeSchema()
	}
	if schemaName == "" {
		schemaName = "public"
	}

	// 1. Fetch all user-defined schemas in the active physical database
	// ponytail: Close each result set immediately to prevent holding 7 concurrent connections and deadlocking sql.DB pool.
	schemaRows, err := s.db.QueryContext(ctx, `
		SELECT nspname FROM pg_catalog.pg_namespace
		WHERE nspname NOT LIKE 'pg_%' AND nspname != 'information_schema'
		ORDER BY nspname;
	`)
	var allSchemas []string
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
		colRows.Close()
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
		ORDER BY con.conname, k.pos;
	`, schemaName)
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
	schemaName := s.activeSchema()
	schema, err := s.InspectSchema(ctx, schemaName)
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

	regClass := fmt.Sprintf("%s.%s", quoteIdentifier(s.activeSchema()), quoteIdentifier(tableName))
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

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := s.applyActiveSchema(ctx, conn); err != nil {
		return nil, err
	}

	// ANALYZE executes the statement, so every tier runs inside a READ ONLY transaction that is
	// always rolled back: DML (or data-modifying CTEs) can never be applied by an Explain request.
	inReadOnlyTx := func(fn func(tx *sql.Tx) error) error {
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		return fn(tx)
	}

	// Tier 1: Full ANALYZE+BUFFERS plan (actual row counts) — only for row-returning statements.
	var rawJSON string
	if driver.IsQueryStatement(cleaned) {
		const analyzeFmt = "EXPLAIN (ANALYZE, COSTS, VERBOSE, BUFFERS, FORMAT JSON) "
		if err := inReadOnlyTx(func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, analyzeFmt+cleaned).Scan(&rawJSON)
		}); err == nil {
			return &driver.ExplainPlan{RawJSON: rawJSON, Rows: []driver.ExplainRow{}}, nil
		}
	}

	// Tier 2: Cost-only JSON plan (no execution; works for DML without running it)
	if err := inReadOnlyTx(func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "EXPLAIN (FORMAT JSON) "+cleaned).Scan(&rawJSON)
	}); err == nil {
		return &driver.ExplainPlan{RawJSON: rawJSON, Rows: []driver.ExplainRow{}}, nil
	}

	// Tier 3: Text EXPLAIN fallback (ancient PG or unusual syntax)
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

func (s *PostgresSession) CancelQuery(ctx context.Context, queryID string) error {
	s.mu.Lock()
	pid, ok := s.queryPids[queryID]
	if !ok && (queryID == "active" || queryID == "") {
		for _, p := range s.queryPids {
			if p > 0 {
				pid = p
				ok = true
				break
			}
		}
	}
	s.mu.Unlock()

	if !ok || pid <= 0 {
		return nil
	}

	// Fail-fast timeout for out-of-band control connection (3s max)
	ctxCancel, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// The query may finish while the control connection is being dialed and its backend be reused
	// by another query from the pool; re-check right before cancelling.
	stillRunning := func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, p := range s.queryPids {
			if p == pid {
				return true
			}
		}
		return false
	}

	// ponytail: Dial independent single-shot control connection to bypass saturated s.db pool
	if s.dsn != "" {
		controlDB, dErr := sql.Open("pgx", s.dsn)
		if dErr == nil {
			defer controlDB.Close()
			if conn, cErr := controlDB.Conn(ctxCancel); cErr == nil {
				defer conn.Close()
				if !stillRunning() {
					return nil
				}
				if _, kErr := conn.ExecContext(ctxCancel, "SELECT pg_cancel_backend($1)", pid); kErr == nil {
					return nil
				}
			}
		}
	}

	// Fallback to pool if independent connection dial failed
	if s.db != nil {
		if !stillRunning() {
			return nil
		}
		_, err := s.db.ExecContext(ctxCancel, "SELECT pg_cancel_backend($1)", pid)
		if err != nil {
			if errors.Is(ctxCancel.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("control channel timeout (3s): target database connection pool may be saturated: %w", err)
			}
			return fmt.Errorf("failed to cancel query via control channel: %w", err)
		}
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
		var rawBlockers any

		if err := rows.Scan(&pid, &user, &host, &db, &state, &timeSec, &command, &query, &rawBlockers); err != nil {
			continue
		}

		blockingPids := parseBlockingPids(rawBlockers)

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

func parseBlockingPids(rawBlockers any) []int64 {
	var blockingPids []int64
	switch v := rawBlockers.(type) {
	case []int64:
		blockingPids = v
	case []int32:
		for _, b := range v {
			blockingPids = append(blockingPids, int64(b))
		}
	case []any:
		for _, item := range v {
			switch num := item.(type) {
			case int64:
				blockingPids = append(blockingPids, num)
			case int32:
				blockingPids = append(blockingPids, int64(num))
			}
		}
	case string:
		trimmed := strings.Trim(v, "{}")
		if trimmed != "" {
			for _, part := range strings.Split(trimmed, ",") {
				if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
					blockingPids = append(blockingPids, n)
				}
			}
		}
	}
	return blockingPids
}

func (s *PostgresSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	if threadID <= 0 {
		return errors.New("invalid thread id")
	}

	var query string
	if killConnection {
		query = "SELECT pg_terminate_backend($1)"
	} else {
		query = "SELECT pg_cancel_backend($1)"
	}

	executeKill := func(db *sql.DB) (bool, error) {
		var ok bool
		err := db.QueryRowContext(ctx, query, threadID).Scan(&ok)
		return ok, err
	}

	// ponytail: Dial independent single-shot control connection to bypass saturated s.db pool
	if s.dsn != "" {
		controlDB, dErr := sql.Open("pgx", s.dsn)
		if dErr == nil {
			defer controlDB.Close()
			ok, kErr := executeKill(controlDB)
			if kErr == nil {
				if !ok {
					action := "cancel query"
					if killConnection {
						action = "terminate connection"
					}
					return fmt.Errorf("failed to %s for backend PID %d (process may not exist or insufficient privilege)", action, threadID)
				}
				return nil
			}
		}
	}

	if s.db != nil {
		ok, err := executeKill(s.db)
		if err != nil {
			return err
		}
		if !ok {
			action := "cancel query"
			if killConnection {
				action = "terminate connection"
			}
			return fmt.Errorf("failed to %s for backend PID %d (process may not exist or insufficient privilege)", action, threadID)
		}
		return nil
	}
	return errors.New("database connection not available")
}

// GetDatabaseDiagnostics returns physical storage and metadata for all databases on PostgreSQL.
func (s *PostgresSession) GetDatabaseDiagnostics(ctx context.Context) ([]driver.DatabaseDiagnostics, error) {
	query := `
		SELECT 
			d.datname,
			COALESCE(pg_database_size(d.datname), 0),
			COALESCE(d.datcollate, ''),
			COALESCE(pg_encoding_to_char(d.encoding), ''),
			d.datistemplate
		FROM pg_database d
		ORDER BY 
			CASE WHEN d.datname IN ('postgres', 'template0', 'template1') THEN 1 ELSE 0 END,
			pg_database_size(d.datname) DESC,
			d.datname ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query postgres database diagnostics: %w", err)
	}
	defer rows.Close()

	var result []driver.DatabaseDiagnostics
	for rows.Next() {
		var dd driver.DatabaseDiagnostics
		var isTemplate bool
		if err := rows.Scan(
			&dd.Name,
			&dd.TotalSize,
			&dd.Collation,
			&dd.Charset,
			&isTemplate,
		); err == nil {
			dd.DataSize = dd.TotalSize
			lowerName := strings.ToLower(dd.Name)
			dd.IsSystem = isTemplate || lowerName == "postgres" || strings.HasPrefix(lowerName, "template")
			result = append(result, dd)
		}
	}
	if result == nil {
		result = []driver.DatabaseDiagnostics{}
	}
	return result, nil
}

func (s *PostgresSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	if database == "" {
		database = s.activeSchema()
	}
	query := `
		SELECT 
			c.relname,
			COALESCE(c.reltuples::bigint, 0),
			COALESCE(pg_relation_size(c.oid), 0),
			COALESCE(pg_indexes_size(c.oid), 0),
			CASE WHEN pk.indrelid IS NOT NULL THEN 1 ELSE 0 END
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN (
			SELECT DISTINCT indrelid
			FROM pg_index
			WHERE indisprimary = true
		) pk ON c.oid = pk.indrelid
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'p')
	`
	args := []any{database}
	if table != "" {
		query += " AND c.relname = $2"
		args = append(args, table)
	}
	query += " ORDER BY (pg_relation_size(c.oid) + pg_indexes_size(c.oid)) DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get table diagnostics: %w", err)
	}
	defer rows.Close()

	var results []driver.TableDiagnostics
	for rows.Next() {
		var td driver.TableDiagnostics
		var hasPk int
		if err := rows.Scan(
			&td.Name,
			&td.Rows,
			&td.DataSize,
			&td.IndexSize,
			&hasPk,
		); err == nil {
			td.Engine = "PostgreSQL Heap"
			td.RowFormat = "Dynamic"
			td.Collation = "default"
			td.HasPrimaryKey = hasPk == 1
			results = append(results, td)
		}
	}
	if results == nil {
		results = []driver.TableDiagnostics{}
	}
	return results, nil
}

func (s *PostgresSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	var results []driver.MaintenanceResult
	for _, tbl := range tables {
		regClass := fmt.Sprintf("%s.%s", quoteIdentifier(s.activeSchema()), quoteIdentifier(tbl))
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
	schema, err := s.InspectSchema(ctx, s.activeSchema())
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
	return driver.DetectRedundantIndexes("postgres", s.activeSchema(), tables), nil
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
	// ponytail: Dialect-Aware Role Matrix: query role attributes and memberships from pg_roles
	var isSuper, canCreateDB, canCreateRole, canLogin, isRep, bypassRLS bool
	_ = s.db.QueryRowContext(ctx, `
		SELECT rolsuper, rolcreatedb, rolcreaterole, rolcanlogin, rolreplication, rolbypassrls
		FROM pg_catalog.pg_roles 
		WHERE rolname = $1
	`, user).Scan(&isSuper, &canCreateDB, &canCreateRole, &canLogin, &isRep, &bypassRLS)

	var grants []string
	if isSuper {
		grants = append(grants, "SUPERUSER")
	}
	if canCreateDB {
		grants = append(grants, "CREATEDB")
	}
	if canCreateRole {
		grants = append(grants, "CREATEROLE")
	}
	if canLogin {
		grants = append(grants, "LOGIN")
	}
	if isRep {
		grants = append(grants, "REPLICATION")
	}
	if bypassRLS {
		grants = append(grants, "BYPASSRLS")
	}

	// Membership in other roles
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.rolname 
		FROM pg_catalog.pg_roles r
		JOIN pg_catalog.pg_auth_members m ON m.roleid = r.oid
		JOIN pg_catalog.pg_roles u ON u.oid = m.member
		WHERE u.rolname = $1
	`, user)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var roleName string
			if err := rows.Scan(&roleName); err == nil {
				grants = append(grants, fmt.Sprintf("MEMBER OF %s", roleName))
			}
		}
	}
	if len(grants) == 0 {
		grants = []string{"CONNECT", "USAGE"}
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

// rePrivilegeName allows only bare privilege keywords (e.g. "CONNECT", "TEMPORARY").
var rePrivilegeName = regexp.MustCompile(`^[A-Za-z]+( [A-Za-z]+)*$`)

func (s *PostgresSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	quotedUser := quoteIdentifier(req.User)
	for _, priv := range req.Privileges {
		if !rePrivilegeName.MatchString(priv) {
			return fmt.Errorf("invalid privilege name: %q", priv)
		}
		cmd := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s", priv, quoteIdentifier(s.cfg.DatabaseName), quotedUser)
		if _, err := s.db.ExecContext(ctx, cmd); err != nil {
			return fmt.Errorf("failed to grant %s on database %s to %s: %w", priv, s.cfg.DatabaseName, req.User, err)
		}
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

func (s *PostgresSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	var isRecovery bool
	_ = s.db.QueryRowContext(ctx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)

	role := "Primary (Source)"
	if isRecovery {
		role = "Standby (Replica)"
	}

	nodes := []driver.ClusterNodeInfo{
		{
			Name:    fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
			Address: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
			State:   role,
			IsLocal: true,
			Status:  "online",
		},
	}

	diagnosis := "PostgreSQL instance operating normally"
	severity := "ok"
	var repInfo *driver.ReplicationInfo

	if !isRecovery {
		// Primary: probe pg_stat_replication for active standbys
		rows, err := s.db.QueryContext(ctx, `
			SELECT 
				COALESCE(client_addr::text, 'unknown'),
				COALESCE(application_name, 'walreceiver'),
				COALESCE(state, 'streaming'),
				COALESCE(sync_state, 'async'),
				COALESCE(EXTRACT(EPOCH FROM write_lag)::bigint, 0)
			FROM pg_stat_replication
		`)
		if err == nil {
			defer rows.Close()
			standbyCount := 0
			for rows.Next() {
				var addr, app, st, syncSt string
				var lagSec int64
				if err := rows.Scan(&addr, &app, &st, &syncSt, &lagSec); err == nil {
					standbyCount++
					nodes = append(nodes, driver.ClusterNodeInfo{
						Name:    fmt.Sprintf("%s (%s)", app, syncSt),
						Address: addr,
						State:   "Standby",
						Status:  st,
					})
					if lagSec > 10 {
						diagnosis = fmt.Sprintf("Replication lag on standby %s: %d seconds", app, lagSec)
						severity = "warning"
					}
				}
			}
			if standbyCount > 0 {
				repInfo = &driver.ReplicationInfo{
					Role:       "Primary (Source)",
					IORunning:  true,
					SQLRunning: true,
				}
			}
		}
	} else {
		// Standby: probe pg_stat_wal_receiver
		var senderHost, status string
		var lastMsgReceipt time.Time
		err := s.db.QueryRowContext(ctx, `
			SELECT COALESCE(sender_host, ''), COALESCE(status, ''), COALESCE(last_msg_receipt_time, now())
			FROM pg_stat_wal_receiver
		`).Scan(&senderHost, &status, &lastMsgReceipt)
		if err == nil && senderHost != "" {
			repInfo = &driver.ReplicationInfo{
				Role:       "Standby (Replica)",
				MasterHost: senderHost,
				IORunning:  strings.EqualFold(status, "streaming"),
				SQLRunning: true,
			}
			nodes = append(nodes, driver.ClusterNodeInfo{
				Name:    "primary-source",
				Address: senderHost,
				State:   "Primary",
				Status:  "online",
			})
		}
	}

	return &driver.ClusterHealthReport{
		TopologyType:        "replication",
		ClusterName:         s.cfg.DatabaseName,
		ClusterStatus:       role,
		ClusterSize:         len(nodes),
		LocalNodeName:       fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
		LocalNodeState:      role,
		BottleneckDiagnosis: diagnosis,
		BottleneckSeverity:  severity,
		Replication:         repInfo,
		Nodes:               nodes,
	}, nil
}

func (s *PostgresSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	var activeConns, activeThreads, questions, rowLockWaits int64
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()").Scan(&activeConns)
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'active'").Scan(&activeThreads)
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&rowLockWaits)
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(sum(xact_commit + xact_rollback), 0) FROM pg_stat_database WHERE datname = current_database()").Scan(&questions)

	// Fetch active queries
	// ponytail: Filter out empty queries/daemon tasks and order by duration descending.
	rows, err := s.db.QueryContext(ctx, `
		SELECT pid, COALESCE(usename, '') as u, COALESCE(client_addr::text, 'local') as h,
		       COALESCE(datname, '') as db, COALESCE(state, '') as st,
		       COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - query_start))::bigint, 0) as dur,
		       COALESCE(query, '') as q
		FROM pg_stat_activity
		WHERE datname = current_database() AND state = 'active' AND pid != pg_backend_pid()
		  AND query IS NOT NULL AND TRIM(query) != ''
		ORDER BY dur DESC
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
		Questions:          questions,
		ThreadsRunning:     activeThreads,
		ThreadsConnected:   activeConns,
		RowLockWaits:       rowLockWaits,
		BufferPoolWaitFree: 0,
		ActiveQueries:      queries,
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

	// Primary Key
	var pkCols []string
	pkColMap := make(map[string]bool)
	for _, col := range tbl.Columns {
		if col.IsPrimaryKey {
			pkCols = append(pkCols, quoteIdentifier(col.Name))
			pkColMap[strings.ToLower(col.Name)] = true
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
		if strings.HasSuffix(idx.Name, "_pkey") {
			continue // Skip PK indexes
		}
		if len(pkColMap) > 0 && len(idx.Columns) == len(pkColMap) && idx.IsUnique {
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

// InspectServerVariables introspects all PostgreSQL settings from pg_settings and applies curated DBA tuning rules.
func (s *PostgresSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name, setting, category, short_desc, context FROM pg_settings ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to introspect pg_settings: %w", err)
	}
	defer rows.Close()

	curatedRules := map[string]struct {
		Category       string
		Recommendation string
		SuggestedValue string
		ImpactLow      string
		ImpactHigh     string
	}{
		"shared_buffers": {
			Category:       "Memory & Buffer",
			Recommendation: "Typically set to 25% of total system RAM (e.g. 1GB - 8GB on modern servers)",
			SuggestedValue: "25% of RAM",
			ImpactLow:      "Frequent disk reads and cache thrashing",
			ImpactHigh:     "Diminishing returns, double-buffering with OS page cache",
		},
		"work_mem": {
			Category:       "Memory & Buffer",
			Recommendation: "Allocate based on available RAM divided by max_connections (e.g. 16MB - 64MB)",
			SuggestedValue: "16MB - 64MB",
			ImpactLow:      "Complex sorts and hash joins spill to temporary disk files",
			ImpactHigh:     "OOM risk when multiple concurrent backends run multi-node queries",
		},
		"maintenance_work_mem": {
			Category:       "Memory & Buffer",
			Recommendation: "10% of system RAM up to 1GB or 2GB for VACUUM, CREATE INDEX, and ALTER TABLE",
			SuggestedValue: "256MB - 1GB",
			ImpactLow:      "Slow VACUUM, autovacuum, and index build operations",
			ImpactHigh:     "Starves other processes during bulk maintenance tasks",
		},
		"effective_cache_size": {
			Category:       "Optimizer & Performance",
			Recommendation: "Set to 50% - 75% of total system RAM to help planner estimate OS disk cache",
			SuggestedValue: "50% - 75% of RAM",
			ImpactLow:      "Planner underestimates memory cache and favors sequential scans over index scans",
			ImpactHigh:     "Planner may overly favor index scans when pages aren't cached",
		},
		"wal_buffers": {
			Category:       "IO & Durability",
			Recommendation: "Typically 16MB (or automatic -1) for high-write transaction workloads",
			SuggestedValue: "16MB",
			ImpactLow:      "Frequent WAL flushes before transaction commit",
			ImpactHigh:     "Wasted shared memory",
		},
		"checkpoint_completion_target": {
			Category:       "IO & Durability",
			Recommendation: "Set to 0.9 to spread checkpoint writes across 90% of checkpoint interval",
			SuggestedValue: "0.9",
			ImpactLow:      "Spiky I/O bursts causing query latency jitter during checkpoints",
			ImpactHigh:     "Slightly longer crash recovery time",
		},
		"max_connections": {
			Category:       "Concurrency & Lock",
			Recommendation: "Keep between 100 - 300; use connection pooler (PgBouncer) for higher concurrency",
			SuggestedValue: "100 - 200",
			ImpactLow:      "Connection exhaustion ('too many clients already')",
			ImpactHigh:     "Severe memory overhead and CPU context-switching contention",
		},
		"autovacuum": {
			Category:       "IO & Durability",
			Recommendation: "Keep 'on' to prevent table bloat and transaction ID (XID) wraparound failure",
			SuggestedValue: "on",
			ImpactLow:      "Critical table bloat, slow queries, and database shutdown on XID wraparound",
			ImpactHigh:     "Excessive background I/O if cost limits are not properly tuned",
		},
	}

	var vars []driver.ServerVariable
	curatedCount := 0
	for rows.Next() {
		var name, setting, category, shortDesc, contextStr string
		if err := rows.Scan(&name, &setting, &category, &shortDesc, &contextStr); err == nil {
			isDynamic := (contextStr == "user" || contextStr == "superuser")
			scope := "Global"
			if contextStr == "user" {
				scope = "Both"
			}
			isCurated := false
			severity := "info"
			recommendation := ""
			suggestedVal := ""
			impactLow := ""
			impactHigh := ""

			if rule, ok := curatedRules[name]; ok {
				isCurated = true
				curatedCount++
				severity = "optimal"
				category = rule.Category
				recommendation = rule.Recommendation
				suggestedVal = rule.SuggestedValue
				impactLow = rule.ImpactLow
				impactHigh = rule.ImpactHigh

				if name == "autovacuum" && !strings.EqualFold(setting, "on") {
					severity = "critical"
				}
			}

			vars = append(vars, driver.ServerVariable{
				Name:             name,
				Value:            setting,
				Scope:            scope,
				IsDynamic:        isDynamic,
				Category:         category,
				Description:      shortDesc,
				ImpactLow:        impactLow,
				ImpactHigh:       impactHigh,
				Recommendation:   recommendation,
				SuggestedValue:   suggestedVal,
				Severity:         severity,
				IsCurated:        isCurated,
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
		CuratedCount: curatedCount,
		Variables:    vars,
	}, nil
}

func (s *PostgresSession) InspectDeadlock(ctx context.Context) (*driver.DeadlockReport, error) {
	return &driver.DeadlockReport{
		HasDeadlock: false,
		Summary:     "PostgreSQL deadlock post-mortem is captured in pg_stat_activity / server logs.",
	}, nil
}

func (s *PostgresSession) GetStatementDigest(ctx context.Context, limit int) ([]driver.StatementDigestEntry, error) {
	if limit <= 0 {
		limit = 25
	}
	// ponytail: Adaptive Telemetry Probe: check pg_stat_statements (PG13+ uses total_exec_time, PG<13 uses total_time)
	query := fmt.Sprintf(`
		SELECT 
			query,
			calls,
			ROUND(total_exec_time::numeric, 2) as total_latency,
			ROUND(mean_exec_time::numeric, 2) as avg_latency,
			ROUND(max_exec_time::numeric, 2) as max_latency,
			0 as lock_time,
			rows,
			rows as rows_examined,
			0 as rows_affected,
			false as no_index_used,
			'' as first_seen,
			'' as last_seen,
			queryid::text as digest
		FROM pg_stat_statements
		WHERE query IS NOT NULL AND TRIM(query) != ''
		ORDER BY total_exec_time DESC
		LIMIT %d;
	`, limit)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		fallbackQuery := fmt.Sprintf(`
			SELECT 
				query,
				calls,
				ROUND(total_time::numeric, 2) as total_latency,
				ROUND(mean_time::numeric, 2) as avg_latency,
				0 as max_latency,
				0 as lock_time,
				rows,
				rows as rows_examined,
				0 as rows_affected,
				false as no_index_used,
				'' as first_seen,
				'' as last_seen,
				queryid::text as digest
			FROM pg_stat_statements
			WHERE query IS NOT NULL AND TRIM(query) != ''
			ORDER BY total_time DESC
			LIMIT %d;
		`, limit)
		rows, err = s.db.QueryContext(ctx, fallbackQuery)
	}
	if err != nil {
		return []driver.StatementDigestEntry{
			{
				Query:  "-- pg_stat_statements extension is not installed or enabled in shared_preload_libraries. Run: CREATE EXTENSION IF NOT EXISTS pg_stat_statements;",
				Digest: "uninstalled",
			},
		}, nil
	}
	defer rows.Close()

	var list []driver.StatementDigestEntry
	for rows.Next() {
		var e driver.StatementDigestEntry
		if err := rows.Scan(
			&e.Query, &e.ExecCount, &e.TotalLatencyMs, &e.AvgLatencyMs, &e.MaxLatencyMs,
			&e.LockTimeMs, &e.RowsSent, &e.RowsExamined, &e.RowsAffected,
			&e.NoIndexUsed, &e.FirstSeen, &e.LastSeen, &e.Digest,
		); err == nil {
			list = append(list, e)
		}
	}
	if list == nil {
		list = []driver.StatementDigestEntry{}
	}
	return list, nil
}

func (s *PostgresSession) ResetStatementDigest(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "SELECT pg_stat_statements_reset()")
	return err
}

func (s *PostgresSession) GetRoutineDDL(ctx context.Context, routineType, name string) (string, error) {
	// Parse base function name if signature was included e.g. "calc_tax(numeric, integer)"
	baseName := name
	if idx := strings.Index(name, "("); idx > 0 {
		baseName = strings.TrimSpace(name[:idx])
	}
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

func (s *PostgresSession) GetEventDDL(ctx context.Context, name string) (string, error) {
	return "-- Scheduled Events not supported natively in PostgreSQL (use pg_cron or external scheduler)", nil
}

func (s *PostgresSession) ToggleEventStatus(ctx context.Context, name string, enable bool) error {
	return errors.New("scheduled events not supported on PostgreSQL")
}

func (s *PostgresSession) GetTablePartitions(ctx context.Context, database, table string) ([]driver.PartitionMetadata, error) {
	return []driver.PartitionMetadata{}, nil
}
