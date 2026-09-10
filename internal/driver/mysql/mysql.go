package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"sqlfox/internal/driver"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
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

	var tunnel *sshTunnel
	network := "tcp"

	// 1. Configure SSH Tunnel if enabled
	if cfg.UseSSH && cfg.SSHHost != "" {
		var err error
		tunnel, err = createSSHTunnel(ctx, cfg.SSHHost, cfg.SSHPort, cfg.SSHUser, cfg.SSHPassword, cfg.SSHKey)
		if err != nil {
			return nil, fmt.Errorf("failed to establish SSH tunnel: %w", err)
		}

		networkName := "ssh-" + uuid.NewString()
		gomysql.RegisterDialContext(networkName, func(ctx context.Context, addr string) (net.Conn, error) {
			return tunnel.DialContext(ctx, addr)
		})
		network = networkName
	}

	// 2. Configure TLS / SSL if enabled
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

		tlsName := "custom-tls-" + uuid.NewString()
		if err := gomysql.RegisterTLSConfig(tlsName, tlsConfig); err == nil {
			tlsParam = tlsName
		} else {
			tlsParam = "skip-verify"
		}
	}

	// 3. Assemble DSN
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	dsn := fmt.Sprintf("%s:%s@%s(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s&timeout=10s",
		cfg.User, cfg.Password, network, addr, cfg.DatabaseName, tlsParam,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		if tunnel != nil {
			tunnel.Close()
		}
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	// Bounded connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		if tunnel != nil {
			tunnel.Close()
		}
		return nil, fmt.Errorf("failed to connect to MySQL server (%s:%d): %w", cfg.Host, cfg.Port, err)
	}

	return &MySQLSession{
		db:           db,
		cfg:          cfg,
		tunnel:       tunnel,
		queryThreads: make(map[string]int64),
	}, nil
}

type MySQLSession struct {
	db           *sql.DB
	cfg          driver.Config
	tunnel       *sshTunnel
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

	return &mysqlRowStream{
		rows: rows,
		cols: cols,
	}, nil
}

func (s *MySQLSession) Exec(ctx context.Context, querySQL string) (*driver.ExecResult, error) {
	start := time.Now()
	res, err := s.db.ExecContext(ctx, querySQL)
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
	tableMap := make(map[string]*driver.TableMetadata)

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
		tableMap[name] = &tables[len(tables)-1]
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
				if tbl, ok := tableMap[tblName]; ok {
					tbl.Columns = append(tbl.Columns, driver.ColumnMetadata{
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
					if tbl, ok := tableMap[tblName]; ok {
						tbl.Indexes = append(tbl.Indexes, *im)
					}
				}
			}
		}
		idxRows.Close()
	}

	return &driver.SchemaMetadata{
		DatabaseName: dbName,
		Databases:    databases,
		Tables:       tables,
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
	query := fmt.Sprintf("SHOW CREATE TABLE `%s`", strings.ReplaceAll(tableName, "`", "``"))
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
	rows, err := s.db.QueryContext(ctx, "EXPLAIN "+querySQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var planRows []driver.ExplainRow
	for rows.Next() {
		var id int
		var selectType, table, typeStr, extra string
		var partitions, possibleKeys, key, keyLen, ref sql.NullString
		var rowsCount sql.NullInt64
		var filtered sql.NullFloat64

		// Standard MySQL EXPLAIN column order
		err := rows.Scan(
			&id, &selectType, &table, &partitions, &typeStr,
			&possibleKeys, &key, &keyLen, &ref, &rowsCount, &filtered, &extra,
		)
		if err == nil {
			planRows = append(planRows, driver.ExplainRow{
				ID:           id,
				SelectType:   selectType,
				Table:        table,
				Type:         typeStr,
				PossibleKeys: possibleKeys.String,
				Key:          key.String,
				KeyLen:       keyLen.String,
				Ref:          ref.String,
				Rows:         rowsCount.Int64,
				Filtered:     filtered.Float64,
				Extra:        extra,
			})
		}
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

func (s *MySQLSession) Close() error {
	dbErr := s.db.Close()
	var tunnelErr error
	if s.tunnel != nil {
		tunnelErr = s.tunnel.Close()
	}
	if dbErr != nil {
		return dbErr
	}
	return tunnelErr
}

type mysqlRowStream struct {
	rows *sql.Rows
	cols []driver.ColumnDefinition
}

func (r *mysqlRowStream) Columns() []driver.ColumnDefinition {
	return r.cols
}

func (r *mysqlRowStream) NextChunk(batchSize int) ([][]any, error) {
	var chunk [][]any
	count := len(r.cols)

	for len(chunk) < batchSize {
		if !r.rows.Next() {
			if len(chunk) > 0 {
				return chunk, nil
			}
			return nil, io.EOF
		}

		rawValues := make([]any, count)
		valuePtrs := make([]any, count)
		for i := range rawValues {
			valuePtrs[i] = &rawValues[i]
		}

		if err := r.rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make([]any, count)
		for i, val := range rawValues {
			if b, ok := val.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = val
			}
		}
		chunk = append(chunk, row)
	}

	return chunk, nil
}

func (r *mysqlRowStream) Close() error {
	return r.rows.Close()
}
