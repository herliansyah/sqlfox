package sqlite

import (
	"context"
	"database/sql"
	"fmt"
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
					tbl.Indexes = append(tbl.Indexes, driver.IndexMetadata{
						Name:     idxName,
						IsUnique: unique == 1,
						Type:     "BTREE",
					})
				}
			}
			idxRows.Close()
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

func (s *SQLiteSession) Close() error {
	return s.db.Close()
}

func quoteIdentifier(id string) string {
	return `"` + strings.ReplaceAll(id, `"`, `""`) + `"`
}
