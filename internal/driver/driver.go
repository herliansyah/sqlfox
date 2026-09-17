package driver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	ErrUnsupportedDriver   = errors.New("unsupported database driver")
	ErrQueryCancelled      = errors.New("query execution was cancelled")
	ErrSessionClosed       = errors.New("database session is closed")
	ErrConnectionFailed    = errors.New("failed to connect to database")
)

// Driver manages database connection dialing.
type Driver interface {
	Connect(ctx context.Context, cfg Config) (Session, error)
	Name() string
}

// Config holds all connection parameters needed by a driver.
type Config struct {
	Driver       string // mysql, sqlite
	Host         string
	Port         int
	DatabaseName string
	User         string
	Password     string
	UseSSL       bool
	SSLCA        string
	SSLCert      string
	SSLKey       string
	UseSSH       bool
	SSHHost      string
	SSHPort      int
	SSHUser      string
	SSHPassword  string
	SSHKey       string
}

// Session represents an active connection to a target database.
type Session interface {
	// QueryStream executes a query and returns a chunked row stream.
	QueryStream(ctx context.Context, sql string, limit int) (RowStream, error)

	// Exec executes a mutation (INSERT, UPDATE, DELETE, DDL).
	Exec(ctx context.Context, sql string) (*ExecResult, error)

	// InspectSchema returns database metadata (tables, columns, indexes).
	InspectSchema(ctx context.Context, dbName string) (*SchemaMetadata, error)

	// SelectDatabase switches the active database context for subsequent operations.
	SelectDatabase(ctx context.Context, dbName string) error

	// GetTableDDL returns the raw CREATE TABLE statement.
	GetTableDDL(ctx context.Context, tableName string) (string, error)

	// GetTableEstimate returns estimated row count and byte size.
	GetTableEstimate(ctx context.Context, tableName string) (*TableEstimate, error)

	// Explain returns the execution plan of a query.
	Explain(ctx context.Context, sql string) (*ExplainPlan, error)

	// CancelQuery attempts to interrupt a running query out-of-band.
	CancelQuery(ctx context.Context, queryID string) error

	// Ping tests if the connection is alive and returns latency.
	Ping(ctx context.Context) (time.Duration, error)

	// ServerInfo returns database version and active threads.
	ServerInfo(ctx context.Context) (*ServerInfo, error)

	io.Closer
}

// ColumnDefinition describes a column in a query resultset.
type ColumnDefinition struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// RowStream provides chunked row access without buffering the full result in memory.
type RowStream interface {
	Columns() []ColumnDefinition
	// NextChunk returns up to batchSize rows. Returns nil, io.EOF when exhausted.
	NextChunk(batchSize int) ([][]any, error)
	io.Closer
}

// ExecResult contains the outcome of an Exec statement.
type ExecResult struct {
	RowsAffected int64         `json:"rows_affected"`
	LastInsertID int64         `json:"last_insert_id"`
	ExecutionMs  int64         `json:"execution_ms"`
	Message      string        `json:"message"`
}

// TableEstimate provides fast schema size estimations for Safety Guard decisions.
type TableEstimate struct {
	RowCount int64 `json:"row_count"`
	SizeBytes int64 `json:"size_bytes"`
}

// ExplainPlan represents query execution plan items.
type ExplainPlan struct {
	RawJSON string         `json:"raw_json"`
	Rows    []ExplainRow   `json:"rows"`
}

type ExplainRow struct {
	ID           int    `json:"id"`
	SelectType   string `json:"select_type"`
	Table        string `json:"table"`
	Partitions   string `json:"partitions,omitempty"`
	Type         string `json:"type"` // ALL, index, range, ref, eq_ref, const
	PossibleKeys string `json:"possible_keys"`
	Key          string `json:"key"`
	KeyLen       string `json:"key_len"`
	Ref          string `json:"ref"`
	Rows         int64  `json:"rows"`
	Filtered     float64 `json:"filtered"`
	Extra        string `json:"extra"`
}

// ScanExplainRows scans rows from an EXPLAIN query dynamically mapping column names.
// This supports varying column counts across MySQL 5.5, 5.6, 5.7, 8.0+, MariaDB 10.x/11.x, and SQLite.
func ScanExplainRows(rows *sql.Rows) ([]ExplainRow, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	colMap := make(map[string]int, len(cols))
	for i, c := range cols {
		colMap[strings.ToLower(strings.TrimSpace(c))] = i
	}

	var planRows []ExplainRow
	for rows.Next() {
		rawVals := make([]any, len(cols))
		scanArgs := make([]any, len(cols))
		for i := range rawVals {
			scanArgs[i] = &rawVals[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		getString := func(names ...string) string {
			for _, name := range names {
				if idx, ok := colMap[name]; ok && rawVals[idx] != nil {
					switch v := rawVals[idx].(type) {
					case []byte:
						return string(v)
					case string:
						return v
					default:
						return fmt.Sprintf("%v", v)
					}
				}
			}
			return ""
		}

		getInt := func(names ...string) int {
			for _, name := range names {
				if idx, ok := colMap[name]; ok && rawVals[idx] != nil {
					switch v := rawVals[idx].(type) {
					case int:
						return v
					case int64:
						return int(v)
					case int32:
						return int(v)
					default:
						var val int
						_, _ = fmt.Sscanf(fmt.Sprintf("%v", v), "%d", &val)
						return val
					}
				}
			}
			return 0
		}

		getInt64 := func(names ...string) int64 {
			for _, name := range names {
				if idx, ok := colMap[name]; ok && rawVals[idx] != nil {
					switch v := rawVals[idx].(type) {
					case int64:
						return v
					case int:
						return int64(v)
					case int32:
						return int64(v)
					default:
						var val int64
						_, _ = fmt.Sscanf(fmt.Sprintf("%v", v), "%d", &val)
						return val
					}
				}
			}
			return 0
		}

		getFloat := func(names ...string) float64 {
			for _, name := range names {
				if idx, ok := colMap[name]; ok && rawVals[idx] != nil {
					switch v := rawVals[idx].(type) {
					case float64:
						return v
					case float32:
						return float64(v)
					default:
						var val float64
						_, _ = fmt.Sscanf(fmt.Sprintf("%v", v), "%f", &val)
						return val
					}
				}
			}
			return 0
		}

		// Support MySQL / MariaDB and SQLite column names / aliases
		id := getInt("id", "selectid", "addr")
		selectType := getString("select_type", "opcode")
		table := getString("table", "tbl", "p1")
		partitions := getString("partitions")
		typeStr := getString("type", "access_type", "p2")
		possibleKeys := getString("possible_keys", "keys")
		key := getString("key")
		keyLen := getString("key_len")
		ref := getString("ref")
		rowsCount := getInt64("rows", "count", "p4")
		filtered := getFloat("filtered")
		extra := getString("extra", "detail", "comment")

		if typeStr == "" && extra != "" {
			typeStr = "SCAN/SEARCH"
		}

		planRows = append(planRows, ExplainRow{
			ID:           id,
			SelectType:   selectType,
			Table:        table,
			Partitions:   partitions,
			Type:         typeStr,
			PossibleKeys: possibleKeys,
			Key:          key,
			KeyLen:       keyLen,
			Ref:          ref,
			Rows:         rowsCount,
			Filtered:     filtered,
			Extra:        extra,
		})
	}

	if planRows == nil {
		planRows = []ExplainRow{}
	}

	return planRows, rows.Err()
}

// ServerInfo reports database runtime statistics.
type ServerInfo struct {
	Version     string `json:"version"`
	UptimeSec   int64  `json:"uptime_sec"`
	ActiveConns int    `json:"active_conns"`
}

// Schema metadata structures
type SchemaMetadata struct {
	DatabaseName string            `json:"database_name"`
	Databases    []string          `json:"databases,omitempty"`
	Tables       []TableMetadata   `json:"tables"`
	Triggers     []TriggerMetadata `json:"triggers,omitempty"`
}

type TriggerMetadata struct {
	Name      string `json:"name"`
	Table     string `json:"table"`
	Timing    string `json:"timing"`    // BEFORE, AFTER, INSTEAD OF
	Event     string `json:"event"`     // INSERT, UPDATE, DELETE
	Statement string `json:"statement"` // SQL body
}

type TableMetadata struct {
	Name        string               `json:"name"`
	Type        string               `json:"type"` // TABLE or VIEW
	RowCount    int64                `json:"row_count"`
	Columns     []ColumnMetadata     `json:"columns"`
	Indexes     []IndexMetadata      `json:"indexes"`
	ForeignKeys []ForeignKeyMetadata `json:"foreign_keys"`
}

type ColumnMetadata struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	IsNullable   bool   `json:"is_nullable"`
	IsPrimaryKey bool   `json:"is_primary_key"`
	DefaultValue string `json:"default_value"`
	Extra        string `json:"extra"` // auto_increment, etc.
}

type IndexMetadata struct {
	Name     string   `json:"name"`
	Columns  []string `json:"columns"`
	IsUnique bool     `json:"is_unique"`
	Type     string   `json:"type"` // BTREE, etc.
}

type ForeignKeyMetadata struct {
	Name              string `json:"name"`
	Column            string `json:"column"`
	ReferencedTable   string `json:"referenced_table"`
	ReferencedColumn  string `json:"referenced_column"`
	OnDelete          string `json:"on_delete"`
	OnUpdate          string `json:"on_update"`
}
