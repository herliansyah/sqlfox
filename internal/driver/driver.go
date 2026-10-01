package driver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
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
	Exec(ctx context.Context, sql string, args ...any) (*ExecResult, error)

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

	// GetProcesslist retrieves active threads and lock wait relationships.
	GetProcesslist(ctx context.Context) (*ProcesslistResult, error)

	// KillProcess interrupts a query or connection out-of-band.
	KillProcess(ctx context.Context, threadID int64, killConnection bool) error

	// GetTableDiagnostics returns physical storage and fragmentation overhead.
	GetTableDiagnostics(ctx context.Context, database, table string) ([]TableDiagnostics, error)

	// ExecuteMaintenance executes table maintenance operations.
	ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]MaintenanceResult, error)

	// GetRedundantIndexes identifies shadowed or duplicate indexes.
	GetRedundantIndexes(ctx context.Context, database, table string) ([]RedundantIndex, error)

	// PeekForeignKeyRow queries parent table for a single row matching a foreign key value.
	PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*PeekResult, error)

	// GetUserList lists server user accounts.
	GetUserList(ctx context.Context) ([]DatabaseUser, error)

	// GetUserGrants returns active grants for a user account.
	GetUserGrants(ctx context.Context, user, host string) ([]string, error)

	// SaveUser creates or updates a user account with optional password change.
	SaveUser(ctx context.Context, user, host, password string, isNew bool) error

	// DropUser removes a user account.
	DropUser(ctx context.Context, user, host string) error

	// ApplyUserGrants grants privileges to a user.
	ApplyUserGrants(ctx context.Context, req UserGrantRequest) error

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
// ponytail: Flattened column mapper handles MySQL 5.5-8.0, MariaDB, and SQLite column name variances without closure allocations.
func ScanExplainRows(rows *sql.Rows) ([]ExplainRow, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	colMap := make(map[string]int, len(cols))
	for i, c := range cols {
		colMap[strings.ToLower(strings.TrimSpace(c))] = i
	}

	rawVals := make([]any, len(cols))
	scanArgs := make([]any, len(cols))
	for i := range rawVals {
		scanArgs[i] = &rawVals[i]
	}

	var planRows []ExplainRow
	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		get := func(names ...string) string {
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

		id, _ := strconv.Atoi(get("id", "selectid", "addr"))
		rowsCount, _ := strconv.ParseInt(get("rows", "count", "p4"), 10, 64)
		filtered, _ := strconv.ParseFloat(get("filtered"), 64)
		typeStr := get("type", "access_type", "p2")
		extra := get("extra", "detail", "comment")
		if typeStr == "" && extra != "" {
			typeStr = "SCAN/SEARCH"
		}

		planRows = append(planRows, ExplainRow{
			ID:           id,
			SelectType:   get("select_type", "opcode"),
			Table:        get("table", "tbl", "p1"),
			Partitions:   get("partitions"),
			Type:         typeStr,
			PossibleKeys: get("possible_keys", "keys"),
			Key:          get("key"),
			KeyLen:       get("key_len"),
			Ref:          get("ref"),
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

// ProcessItem represents an active thread / query on the database server.
type ProcessItem struct {
	ID      int64  `json:"id"`
	User    string `json:"user"`
	Host    string `json:"host"`
	DB      string `json:"db"`
	Command string `json:"command"`
	Time    int64  `json:"time"`
	State   string `json:"state"`
	Info    string `json:"info"`
}

// LockWait represents lock contention between two transactions or threads.
type LockWait struct {
	WaitingThreadID  int64  `json:"waiting_thread_id"`
	WaitingQuery     string `json:"waiting_query"`
	WaitingUser      string `json:"waiting_user"`
	WaitingSeconds   int64  `json:"waiting_seconds"`
	BlockingThreadID int64  `json:"blocking_thread_id"`
	BlockingQuery    string `json:"blocking_query"`
	BlockingUser     string `json:"blocking_user"`
	LockedResource   string `json:"locked_resource"`
	LockMode         string `json:"lock_mode"`
	IsRootBlocker    bool   `json:"is_root_blocker"`
}

// ProcesslistResult holds active threads and lock analysis.
type ProcesslistResult struct {
	CurrentThreadID int64         `json:"current_thread_id"`
	Threads         []ProcessItem `json:"threads"`
	Locks           []LockWait    `json:"locks"`
}

// TableDiagnostics contains physical storage and fragmentation metrics.
type TableDiagnostics struct {
	Name         string `json:"name"`
	Engine       string `json:"engine"`
	Rows         int64  `json:"rows"`
	DataSize     int64  `json:"data_size"`
	IndexSize    int64  `json:"index_size"`
	OverheadSize int64  `json:"overhead_size"`
	RowFormat    string `json:"row_format"`
	Collation    string `json:"collation"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// MaintenanceResult holds the output of OPTIMIZE/ANALYZE/CHECK/VACUUM operations.
type MaintenanceResult struct {
	Table   string `json:"table"`
	Op      string `json:"op"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// RedundantIndex describes an index that is duplicated, shadowed, or redundant with a primary key.
type RedundantIndex struct {
	Table          string   `json:"table"`
	RedundantIndex string   `json:"redundant_index"`
	RedundantCols  []string `json:"redundant_cols"`
	ShadowedBy     string   `json:"shadowed_by"`
	ShadowedCols   []string `json:"shadowed_cols"`
	Reason         string   `json:"reason"`
	DropSQL        string   `json:"drop_sql"`
}

// PeekResult contains the referenced parent row for foreign key data peeking.
type PeekResult struct {
	Table   string         `json:"table"`
	KeyCol  string         `json:"key_col"`
	KeyVal  any            `json:"key_val"`
	Row     map[string]any `json:"row"`
	Columns []string       `json:"columns"`
}

// ProfileStage represents a single phase in query execution.
type ProfileStage struct {
	Stage      string  `json:"stage"`
	DurationUs int64   `json:"duration_us"`
	Percentage float64 `json:"percentage"`
}

// QueryProfile summarizes total execution duration and breakdown by stage.
type QueryProfile struct {
	DurationMs float64        `json:"duration_ms"`
	Stages     []ProfileStage `json:"stages"`
}

// DatabaseUser represents a server user account.
type DatabaseUser struct {
	User string `json:"user"`
	Host string `json:"host"`
}

// UserGrantRequest specifies privilege mutation parameters for a user account.
type UserGrantRequest struct {
	User       string   `json:"user"`
	Host       string   `json:"host"`
	Scope      string   `json:"scope"` // GLOBAL or DATABASE
	Database   string   `json:"database,omitempty"`
	Privileges []string `json:"privileges"`
	WithGrant  bool     `json:"with_grant"`
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

var drivers = make(map[string]Driver)

// Register registers a database driver by name.
// ponytail: Simple map without mutex; driver registration occurs strictly during init().
func Register(name string, d Driver) {
	drivers[name] = d
}

// GetDriver returns the driver registered with the given name.
func GetDriver(name string) (Driver, error) {
	d, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDriver, name)
	}
	return d, nil
}

// Connect opens a session to the target database using the driver specified in cfg.
func Connect(ctx context.Context, cfg Config) (Session, error) {
	d, err := GetDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}
	return d.Connect(ctx, cfg)
}

