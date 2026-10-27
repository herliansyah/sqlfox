package driver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrUnsupportedDriver = errors.New("unsupported database driver")
	ErrSessionClosed     = errors.New("database session is closed")
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
}

// Session represents an active connection to a target database.
type Session interface {
	// QueryStream executes a query and returns a chunked row stream.
	QueryStream(ctx context.Context, sql string, limit int) (RowStream, error)

	// Exec executes a mutation (INSERT, UPDATE, DELETE, DDL).
	Exec(ctx context.Context, sql string, args ...any) (*ExecResult, error)

	// InspectSchema returns database metadata (tables, columns, indexes).
	InspectSchema(ctx context.Context, dbName string) (*SchemaMetadata, error)

	// InspectFastCatalog returns lightweight table & column metadata for all user databases without heavy foreign key or index scans.
	InspectFastCatalog(ctx context.Context) (map[string][]TableMetadata, error)

	// SelectDatabase switches the active database context for subsequent operations.
	SelectDatabase(ctx context.Context, dbName string) error

	// GetTableDDL returns the raw CREATE TABLE statement.
	GetTableDDL(ctx context.Context, tableName string) (string, error)

	// GetRoutineDDL returns the raw CREATE PROCEDURE/FUNCTION statement.
	GetRoutineDDL(ctx context.Context, routineType, name string) (string, error)

	// GetEventDDL returns the raw CREATE EVENT statement.
	GetEventDDL(ctx context.Context, name string) (string, error)

	// ToggleEventStatus toggles MySQL scheduled event ENABLE / DISABLE.
	ToggleEventStatus(ctx context.Context, name string, enable bool) error

	// GetTablePartitions returns partition telemetry for a table.
	GetTablePartitions(ctx context.Context, database, table string) ([]PartitionMetadata, error)

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

	// GetDatabaseDiagnostics returns physical storage and metadata for all databases in the session.
	GetDatabaseDiagnostics(ctx context.Context) ([]DatabaseDiagnostics, error)

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

	// InspectTableLocks returns active transactions and lock holders touching a specific table.
	InspectTableLocks(ctx context.Context, database, table string) ([]TableLockInfo, error)

	// ExecSafeDDL executes a DDL statement on a dedicated connection with a strict session lock timeout.
	ExecSafeDDL(ctx context.Context, sql string, lockTimeoutSec int) (*ExecResult, error)

	// InspectClusterHealth returns cluster topology, synchronization health, and bottleneck diagnostics.
	InspectClusterHealth(ctx context.Context) (*ClusterHealthReport, error)

	// InspectServerVariables introspects server configuration parameters and evaluates tuning heuristics.
	InspectServerVariables(ctx context.Context) (*ServerVariablesReport, error)

	// SampleFlightMetrics captures an on-demand performance telemetry slice with correlated active queries.
	SampleFlightMetrics(ctx context.Context) (*FlightMetricSample, error)

	// InspectDeadlock parses latest detected InnoDB deadlock from engine status.
	InspectDeadlock(ctx context.Context) (*DeadlockReport, error)

	// GetStatementDigest retrieves aggregated top SQL query statistics from sys/performance_schema.
	GetStatementDigest(ctx context.Context, limit int) ([]StatementDigestEntry, error)

	// ResetStatementDigest clears collected performance_schema statement statistics.
	ResetStatementDigest(ctx context.Context) error

	// ExecRaw executes a raw SQL statement bypassing the Safety Guard. For internal server use only.
	ExecRaw(ctx context.Context, sql string, args ...any) (int64, error)

	// QueryRaw executes a raw SQL query bypassing the Safety Guard and returns all rows. For internal server use only.
	QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error)

	// BeginTx initiates a database transaction on a dedicated physical connection.
	BeginTx(ctx context.Context) (TxSession, error)

	io.Closer
}

// TxSession represents a database transaction bound to a dedicated connection.
type TxSession interface {
	Exec(ctx context.Context, sql string, args ...any) (*ExecResult, error)
	ExecRaw(ctx context.Context, sql string, args ...any) (int64, error)
	QueryStream(ctx context.Context, sql string, limit int) (RowStream, error)
	Commit() error
	Rollback() error
}

type queryIDContextKey struct{}

// WithQueryID associates a query ID with context for out-of-band cancellation tracking.
func WithQueryID(ctx context.Context, queryID string) context.Context {
	return context.WithValue(ctx, queryIDContextKey{}, queryID)
}

// QueryIDFromContext extracts the query ID from context if present.
func QueryIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(queryIDContextKey{}).(string); ok {
		return v
	}
	return ""
}

// FlightMetricSample represents an instant performance telemetry slice.
type FlightMetricSample struct {
	SampledAt          time.Time         `json:"sampled_at"`
	Questions          int64             `json:"questions"`
	ThreadsRunning     int64             `json:"threads_running"`
	ThreadsConnected   int64             `json:"threads_connected"`
	RowLockWaits       int64             `json:"row_lock_waits"`
	BufferPoolWaitFree int64             `json:"buffer_pool_wait_free"`
	ActiveQueries      []FlightQueryInfo `json:"active_queries"`
}

// FlightQueryInfo represents a non-idle running thread correlated to a metric tick.
type FlightQueryInfo struct {
	ThreadID int64  `json:"thread_id"`
	User     string `json:"user"`
	Host     string `json:"host"`
	DB       string `json:"db"`
	Command  string `json:"command"`
	Time     int64  `json:"time"`
	State    string `json:"state"`
	Info     string `json:"info"`
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
	RowsAffected int64  `json:"rows_affected"`
	LastInsertID int64  `json:"last_insert_id"`
	ExecutionMs  int64  `json:"execution_ms"`
	Message      string `json:"message"`
}

// TableEstimate provides fast schema size estimations for Safety Guard decisions.
type TableEstimate struct {
	RowCount  int64 `json:"row_count"`
	SizeBytes int64 `json:"size_bytes"`
}

// ExplainPlan represents query execution plan items.
type ExplainPlan struct {
	RawJSON string       `json:"raw_json"`
	Rows    []ExplainRow `json:"rows"`
	Advisor any          `json:"advisor,omitempty"`
}

type ExplainRow struct {
	ID           int     `json:"id"`
	SelectType   string  `json:"select_type"`
	Table        string  `json:"table"`
	Partitions   string  `json:"partitions,omitempty"`
	Type         string  `json:"type"` // ALL, index, range, ref, eq_ref, const
	PossibleKeys string  `json:"possible_keys"`
	Key          string  `json:"key"`
	KeyLen       string  `json:"key_len"`
	Ref          string  `json:"ref"`
	Rows         int64   `json:"rows"`
	Filtered     float64 `json:"filtered"`
	Extra        string  `json:"extra"`
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
	Version       string `json:"version"`
	UptimeSec     int64  `json:"uptime_sec"`
	ActiveConns   int    `json:"active_conns"`
	ActiveThreads int    `json:"active_threads"`
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

// TableLockInfo describes an active transaction or metadata lock touching a table.
type TableLockInfo struct {
	ThreadID    int64  `json:"thread_id"`
	User        string `json:"user"`
	Host        string `json:"host"`
	Database    string `json:"database"`
	Query       string `json:"query"`
	DurationSec int64  `json:"duration_sec"`
	State       string `json:"state"`
	LockMode    string `json:"lock_mode,omitempty"`
}

// ProcesslistResult holds active threads and lock analysis.
type ProcesslistResult struct {
	CurrentThreadID int64         `json:"current_thread_id"`
	Threads         []ProcessItem `json:"threads"`
	Locks           []LockWait    `json:"locks"`
}

// DatabaseDiagnostics contains aggregated storage and metadata for a database schema.
type DatabaseDiagnostics struct {
	Name         string `json:"name"`
	TableCount   int64  `json:"table_count"`
	DataSize     int64  `json:"data_size"`
	IndexSize    int64  `json:"index_size"`
	OverheadSize int64  `json:"overhead_size"`
	TotalSize    int64  `json:"total_size"`
	Charset      string `json:"charset"`
	Collation    string `json:"collation"`
	IsSystem     bool   `json:"is_system"`
}

// TableDiagnostics contains physical storage and fragmentation metrics.
type TableDiagnostics struct {
	Name          string `json:"name"`
	Engine        string `json:"engine"`
	Rows          int64  `json:"rows"`
	DataSize      int64  `json:"data_size"`
	IndexSize     int64  `json:"index_size"`
	OverheadSize  int64  `json:"overhead_size"`
	RowFormat     string `json:"row_format"`
	Collation     string `json:"collation"`
	HasPrimaryKey bool   `json:"has_primary_key"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
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
	Routines     []RoutineMetadata `json:"routines,omitempty"`
	Events       []EventMetadata   `json:"events,omitempty"`
}

type TriggerMetadata struct {
	Name      string `json:"name"`
	Table     string `json:"table"`
	Timing    string `json:"timing"`    // BEFORE, AFTER, INSTEAD OF
	Event     string `json:"event"`     // INSERT, UPDATE, DELETE
	Statement string `json:"statement"` // SQL body
}

// RoutineMetadata represents a Stored Procedure or Stored Function.
type RoutineMetadata struct {
	Name       string             `json:"name"`
	Type       string             `json:"type"`                // "PROCEDURE" or "FUNCTION"
	DataType   string             `json:"data_type,omitempty"` // return type for functions
	Definition string             `json:"definition,omitempty"`
	Parameters []RoutineParameter `json:"parameters,omitempty"`
}

// RoutineParameter describes an IN, OUT, or INOUT parameter of a routine.
type RoutineParameter struct {
	Name string `json:"name"`
	Mode string `json:"mode"` // "IN", "OUT", "INOUT"
	Type string `json:"type"`
}

// EventMetadata represents a MySQL scheduled event.
type EventMetadata struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "ENABLED", "DISABLED", "SLAVESIDE_DISABLED"
	Interval   string `json:"interval,omitempty"`
	StartsAt   string `json:"starts_at,omitempty"`
	EndsAt     string `json:"ends_at,omitempty"`
	Definition string `json:"definition,omitempty"`
}

// PartitionMetadata represents physical storage and row distribution of a table partition.
type PartitionMetadata struct {
	Name        string `json:"name"`
	Method      string `json:"method"`
	Expression  string `json:"expression,omitempty"`
	Description string `json:"description,omitempty"` // partition range/list values
	TableRows   int64  `json:"table_rows"`
	DataLength  int64  `json:"data_length"`
	IndexLength int64  `json:"index_length"`
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
	Name             string `json:"name"`
	Column           string `json:"column"`
	ReferencedTable  string `json:"referenced_table"`
	ReferencedColumn string `json:"referenced_column"`
	OnDelete         string `json:"on_delete"`
	OnUpdate         string `json:"on_update"`
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

// TableRelationshipInfo describes an inbound or outbound foreign key coupling.
type TableRelationshipInfo struct {
	Direction        string `json:"direction"` // "PARENT" (referenced by other table) or "CHILD" (references other table)
	RelatedTable     string `json:"related_table"`
	ForeignKey       string `json:"foreign_key"`
	Column           string `json:"column"`
	ReferencedColumn string `json:"referenced_column"`
}

// InspectTableRelationships scans schema metadata to detect inbound and outbound foreign key dependencies.
func InspectTableRelationships(schema *SchemaMetadata, table string) []TableRelationshipInfo {
	if schema == nil || table == "" {
		return []TableRelationshipInfo{}
	}
	var rels []TableRelationshipInfo
	for _, t := range schema.Tables {
		if strings.EqualFold(t.Name, table) {
			for _, fk := range t.ForeignKeys {
				rels = append(rels, TableRelationshipInfo{
					Direction:        "CHILD",
					RelatedTable:     fk.ReferencedTable,
					ForeignKey:       fk.Name,
					Column:           fk.Column,
					ReferencedColumn: fk.ReferencedColumn,
				})
			}
		} else {
			for _, fk := range t.ForeignKeys {
				if strings.EqualFold(fk.ReferencedTable, table) {
					rels = append(rels, TableRelationshipInfo{
						Direction:        "PARENT",
						RelatedTable:     t.Name,
						ForeignKey:       fk.Name,
						Column:           fk.Column,
						ReferencedColumn: fk.ReferencedColumn,
					})
				}
			}
		}
	}
	if rels == nil {
		rels = []TableRelationshipInfo{}
	}
	return rels
}

// GenerateOSCCommand formats a production-ready Online Schema Change command.
func GenerateOSCCommand(tool, host string, port int, user, database, table, alterClause string) string {
	if port <= 0 {
		port = 3306
	}
	if user == "" {
		user = "root"
	}
	if host == "" {
		host = "127.0.0.1"
	}
	cleanAlter := strings.TrimSpace(alterClause)
	if strings.HasPrefix(strings.ToUpper(cleanAlter), "ALTER TABLE ") {
		idx := strings.Index(cleanAlter, table)
		if idx != -1 {
			cleanAlter = strings.TrimSpace(cleanAlter[idx+len(table):])
		}
	}

	if strings.EqualFold(tool, "pt-osc") || strings.EqualFold(tool, "pt-online-schema-change") {
		return fmt.Sprintf("pt-online-schema-change \\\n  --host=%q \\\n  --port=%d \\\n  --user=%q \\\n  --ask-pass \\\n  --alter=%q \\\n  D=%q,t=%q \\\n  --chunk-size=1000 \\\n  --max-load=\"Threads_running=30\" \\\n  --critical-load=\"Threads_running=60\" \\\n  --execute",
			host, port, user, cleanAlter, database, table,
		)
	}

	// Default: gh-ost
	return fmt.Sprintf("gh-ost \\\n  --host=%q \\\n  --port=%d \\\n  --user=%q \\\n  --ask-pass \\\n  --database=%q \\\n  --table=%q \\\n  --alter=%q \\\n  --chunk-size=1000 \\\n  --max-load=\"Threads_running=30\" \\\n  --critical-load=\"Threads_running=60\" \\\n  --allow-on-master \\\n  --execute",
		host, port, user, database, table, cleanAlter,
	)
}

// ClusterHealthReport describes database cluster synchronization, topology, and performance diagnostics.
type ClusterHealthReport struct {
	TopologyType        string               `json:"topology_type"` // "galera", "replication", "standalone"
	ClusterName         string               `json:"cluster_name,omitempty"`
	ClusterStatus       string               `json:"cluster_status,omitempty"` // "Primary", "Non-Primary", etc.
	ClusterSize         int                  `json:"cluster_size,omitempty"`
	LocalNodeName       string               `json:"local_node_name,omitempty"`
	LocalNodeAddress    string               `json:"local_node_address,omitempty"`
	LocalNodeState      string               `json:"local_node_state,omitempty"` // "Synced", "Donor", "Joiner", etc.
	FlowControlPaused   float64              `json:"flow_control_paused"`        // fraction paused (0.0 to 1.0)
	FlowControlSent     int64                `json:"flow_control_sent"`
	FlowControlRecv     int64                `json:"flow_control_recv"`
	BottleneckDiagnosis string               `json:"bottleneck_diagnosis,omitempty"`
	BottleneckSeverity  string               `json:"bottleneck_severity,omitempty"` // "ok", "warning", "critical"
	SlowestNode         string               `json:"slowest_node,omitempty"`
	Nodes               []ClusterNodeInfo    `json:"nodes,omitempty"`
	Replication         *ReplicationInfo     `json:"replication,omitempty"`
	TriMetric           TriMetricStats       `json:"tri_metric"`
	TuningChecks        []ClusterTuningCheck `json:"tuning_checks,omitempty"`
}

// ClusterTuningCheck evaluates a critical cluster configuration knob against operational guardrails.
type ClusterTuningCheck struct {
	Variable     string `json:"variable"`
	CurrentValue string `json:"current_value"`
	Recommended  string `json:"recommended"`
	Status       string `json:"status"` // "optimal", "warning", "critical"
	Summary      string `json:"summary"`
	TradeOff     string `json:"trade_off,omitempty"`
	Remediation  string `json:"remediation,omitempty"`
}

// ServerVariable represents an engine configuration parameter annotated with heuristic tuning advice.
type ServerVariable struct {
	Name             string `json:"name"`
	Value            string `json:"value"`
	Scope            string `json:"scope,omitempty"` // "Global", "Session", "Both"
	IsDynamic        bool   `json:"is_dynamic"`      // can be changed live without restart
	Category         string `json:"category"`        // "Cluster & WSREP", "Memory & Buffer", "Concurrency & Lock", "IO & Durability", "Optimizer & Performance", "General"
	Description      string `json:"description,omitempty"`
	ImpactLow        string `json:"impact_low,omitempty"`      // what happens if too low
	ImpactHigh       string `json:"impact_high,omitempty"`     // what happens if too high
	Recommendation   string `json:"recommendation,omitempty"`  // baseline formula or best-practice value
	SuggestedValue   string `json:"suggested_value,omitempty"` // concrete recommended value
	Severity         string `json:"severity"`                  // "optimal", "warning", "critical", "info"
	IsCurated        bool   `json:"is_curated"`
	MyCnfSnippet     string `json:"my_cnf_snippet,omitempty"`
	SetGlobalSnippet string `json:"set_global_snippet,omitempty"`
}

// ServerVariablesReport contains complete introspected database variables and tuning metrics.
type ServerVariablesReport struct {
	Engine       string           `json:"engine"` // "mysql", "postgres", "sqlite"
	Version      string           `json:"version,omitempty"`
	TotalCount   int              `json:"total_count"`
	CuratedCount int              `json:"curated_count"`
	Variables    []ServerVariable `json:"variables"`
}

// ClusterNodeInfo represents status and queue telemetry for an individual cluster member.
type ClusterNodeInfo struct {
	Name            string `json:"name"`
	Address         string `json:"address"`
	State           string `json:"state"`
	ReceiveQueue    int64  `json:"receive_queue"`
	SendQueue       int64  `json:"send_queue"`
	FlowControlSent int64  `json:"flow_control_sent"`
	IsLocal         bool   `json:"is_local"`
	IsCulprit       bool   `json:"is_culprit"`
	Status          string `json:"status"` // "online", "unreachable"
	Message         string `json:"message,omitempty"`
}

// DeadlockReport contains post-mortem analysis of the latest detected InnoDB deadlock.
type DeadlockReport struct {
	HasDeadlock    bool                  `json:"has_deadlock"`
	DetectedAt     string                `json:"detected_at,omitempty"`
	VictimTxID     string                `json:"victim_tx_id,omitempty"`
	VictimThreadID int64                 `json:"victim_thread_id,omitempty"`
	Summary        string                `json:"summary"`
	Transactions   []DeadlockTransaction `json:"transactions"`
	RawSection     string                `json:"raw_section,omitempty"`
}

// DeadlockTransaction represents one of the conflicting transactions in an InnoDB deadlock.
type DeadlockTransaction struct {
	Index          int    `json:"index"` // 1 or 2
	TxID           string `json:"tx_id"`
	ThreadID       int64  `json:"thread_id"`
	Query          string `json:"query"`
	LockMode       string `json:"lock_mode"`
	LockHeld       string `json:"lock_held,omitempty"`
	LockWaited     string `json:"lock_waited,omitempty"`
	AppUser        string `json:"app_user,omitempty"`
	Host           string `json:"host,omitempty"`
	IsVictim       bool   `json:"is_victim"`
	ActiveSeconds  int64  `json:"active_seconds"`
	UndoLogEntries int64  `json:"undo_log_entries"`
}

// StatementDigestEntry aggregates query performance statistics from sys/performance_schema.
type StatementDigestEntry struct {
	Digest         string  `json:"digest"`
	Query          string  `json:"query"`
	ExecCount      int64   `json:"exec_count"`
	TotalLatencyMs float64 `json:"total_latency_ms"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	MaxLatencyMs   float64 `json:"max_latency_ms"`
	LockTimeMs     float64 `json:"lock_time_ms"`
	RowsSent       int64   `json:"rows_sent"`
	RowsExamined   int64   `json:"rows_examined"`
	RowsAffected   int64   `json:"rows_affected"`
	FullScan       bool    `json:"full_scan"`
	NoIndexUsed    int64   `json:"no_index_used"`
	FirstSeen      string  `json:"first_seen,omitempty"`
	LastSeen       string  `json:"last_seen,omitempty"`
}

// ReplicationInfo holds replication metrics for MySQL source/replica setups.
type ReplicationInfo struct {
	Role                string `json:"role"` // "Source", "Replica"
	SecondsBehindMaster *int64 `json:"seconds_behind_master,omitempty"`
	IORunning           bool   `json:"io_running"`
	SQLRunning          bool   `json:"sql_running"`
	MasterHost          string `json:"master_host,omitempty"`
	LastError           string `json:"last_error,omitempty"`
}

// TriMetricStats details network latency, disk write contention, and worker capacity.
type TriMetricStats struct {
	NetworkLatencyMinMs float64 `json:"network_latency_min_ms"`
	NetworkLatencyAvgMs float64 `json:"network_latency_avg_ms"`
	NetworkLatencyMaxMs float64 `json:"network_latency_max_ms"`
	BufferPoolWaitFree  int64   `json:"buffer_pool_wait_free"`
	LogWaits            int64   `json:"log_waits"`
	OSLogFsyncs         int64   `json:"os_log_fsyncs"`
	SlaveThreads        int     `json:"slave_threads"`
	CertFailures        int64   `json:"cert_failures"`
	BFAborts            int64   `json:"bf_aborts"`
}

// QuoteIdentifier formats an identifier safely with driver-aware quotes.
func QuoteIdentifier(name, driverName string) string {
	name = strings.Trim(name, "`\"")
	if strings.EqualFold(driverName, "sqlite") || strings.EqualFold(driverName, "postgres") || strings.EqualFold(driverName, "postgresql") || strings.EqualFold(driverName, "pg") {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Placeholder returns the parameter placeholder for the given driver and 1-based index.
func Placeholder(driverName string, index int) string {
	d := strings.ToLower(driverName)
	if strings.HasPrefix(d, "postgre") || d == "pg" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

var (
	reLimitClause  = regexp.MustCompile(`(?i)\b(LIMIT\s+(\d+|ALL\b|\?|\$\d+)|FETCH\s+(FIRST|NEXT)\b)`)
	reLockingTail  = regexp.MustCompile(`(?i)\b(FOR\s+(UPDATE|SHARE|NO\s+KEY\s+UPDATE|KEY\s+SHARE)|LOCK\s+IN\s+SHARE\s+MODE)\b`)
	reTopLevelInto = regexp.MustCompile(`(?i)\bINTO\b`)
)

// maskNonTopLevel blanks comments, quoted literals/identifiers and everything inside parentheses
// (byte offsets preserved), so keyword scans only see the outermost statement: a LIMIT inside a
// subquery, string or comment must not count as the statement's own LIMIT.
// ponytail: backslash is treated as an escape inside quotes (MySQL); a PostgreSQL literal ending in
// a backslash can desync the mask, in which case the cap may be skipped or the query rejected by
// the server — never silently widened.
func maskNonTopLevel(q string) string {
	b := []byte(q)
	depth := 0
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '-' && i+1 < len(b) && b[i+1] == '-':
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(q[i+2:], "*/")
			stop := len(b)
			if end >= 0 {
				stop = i + 2 + end + 2
			}
			for ; i < stop; i++ {
				b[i] = ' '
			}
			i--
		case b[i] == '\'' || b[i] == '"' || b[i] == '`':
			quote := b[i]
			b[i] = ' '
			for i++; i < len(b); i++ {
				if b[i] == '\\' && quote != '`' && i+1 < len(b) {
					b[i], b[i+1] = ' ', ' '
					i++
					continue
				}
				if b[i] == quote {
					b[i] = ' '
					break
				}
				b[i] = ' '
			}
		case b[i] == '(':
			depth++
			b[i] = ' '
		case b[i] == ')':
			if depth > 0 {
				depth--
			}
			b[i] = ' '
		default:
			if depth > 0 {
				b[i] = ' '
			}
		}
	}
	return string(b)
}

// HasLimitClause reports whether the SQL statement's outermost query has a row limit
// (LIMIT n / LIMIT ALL / LIMIT ? / FETCH FIRST|NEXT).
func HasLimitClause(sql string) bool {
	return reLimitClause.MatchString(maskNonTopLevel(sql))
}

// ApplyLimitClause adds a LIMIT to SELECT queries that have no outermost limit and limit > 0.
// It strips leading comments before detecting the SELECT keyword and puts LIMIT on its own line so
// trailing single-line comments cannot swallow it. A locking clause (FOR UPDATE, LOCK IN SHARE MODE)
// must follow LIMIT, so LIMIT is inserted before it; SELECT ... INTO is left untouched.
func ApplyLimitClause(querySQL string, limit int) string {
	if limit <= 0 {
		return querySQL
	}
	trimmed := strings.TrimSpace(querySQL)
	cleanUpper := strings.ToUpper(StripLeadingComments(trimmed))
	if !strings.HasPrefix(cleanUpper, "SELECT") || HasLimitClause(trimmed) {
		return querySQL
	}
	// Drop a trailing terminator followed only by comments/whitespace (e.g. "SELECT 1; -- note")
	if i := strings.LastIndex(trimmed, ";"); i >= 0 && !HasExecutableSQL(trimmed[i+1:], true) {
		trimmed = strings.TrimSpace(trimmed[:i])
	}
	masked := maskNonTopLevel(trimmed)
	if reTopLevelInto.MatchString(masked) {
		return querySQL
	}
	if loc := reLockingTail.FindStringIndex(masked); loc != nil {
		return fmt.Sprintf("%s\nLIMIT %d\n%s", strings.TrimRight(trimmed[:loc[0]], " \t\r\n"), limit, trimmed[loc[0]:])
	}
	return fmt.Sprintf("%s\nLIMIT %d", trimmed, limit)
}

// StripLeadingComments removes leading whitespace and SQL comments (-- , #, and /* ... */).
func StripLeadingComments(sql string) string {
	s := strings.TrimSpace(sql)
	for {
		if strings.HasPrefix(s, "--") {
			idx := strings.IndexByte(s, '\n')
			if idx == -1 {
				return ""
			}
			s = strings.TrimSpace(s[idx+1:])
			continue
		}
		if strings.HasPrefix(s, "#") {
			idx := strings.IndexByte(s, '\n')
			if idx == -1 {
				return ""
			}
			s = strings.TrimSpace(s[idx+1:])
			continue
		}
		if strings.HasPrefix(s, "/*") {
			idx := strings.Index(s, "*/")
			if idx == -1 {
				return ""
			}
			// MySQL executes /*! ... */ and /*!NNNNN ... */ bodies, so unwrap them instead of
			// discarding; otherwise callers (e.g. the Safety Guard) would miss the real statement.
			if strings.HasPrefix(s, "/*!") {
				body := strings.TrimLeft(s[3:idx], "0123456789")
				s = strings.TrimSpace(body + " " + s[idx+2:])
				continue
			}
			s = strings.TrimSpace(s[idx+2:])
			continue
		}
		break
	}
	return s
}

// ErrExplainTarget is returned when SQL cannot be explained without risking its execution.
var ErrExplainTarget = errors.New("explain requires exactly one statement without EXPLAIN ANALYZE/options (they are applied automatically)")

var reExplainPrefix = regexp.MustCompile(`(?is)^EXPLAIN(\s+QUERY\s+PLAN)?\s+`)

// ExplainTarget validates the statements of an Explain request and returns the bare statement to plan.
// stmts must come from a quote-aware splitter. Multiple statements are rejected because drivers that
// accept multi-statement text would execute everything after the first one, and EXPLAIN ANALYZE /
// EXPLAIN (options) are rejected because ANALYZE executes the statement.
func ExplainTarget(stmts []string) (string, error) {
	if len(stmts) != 1 {
		return "", ErrExplainTarget
	}
	s := strings.TrimRight(strings.TrimSpace(StripLeadingComments(stmts[0])), "; \t\r\n")
	if loc := reExplainPrefix.FindStringIndex(s); loc != nil {
		s = strings.TrimSpace(s[loc[1]:])
		up := strings.ToUpper(s)
		if strings.HasPrefix(up, "ANALYZE") || strings.HasPrefix(up, "ANALYSE") || strings.HasPrefix(up, "(") || strings.HasPrefix(up, "FORMAT") {
			return "", ErrExplainTarget
		}
	}
	if s == "" {
		return "", ErrExplainTarget
	}
	return s, nil
}

// IsQueryStatement reports whether a SQL statement returns rows (SELECT, SHOW, DESC, EXPLAIN, WITH, PRAGMA, VALUES, TABLE, etc.).
func IsQueryStatement(sql string) bool {
	clean := StripLeadingComments(sql)
	clean = strings.TrimLeft(clean, "(\t\r\n ")
	upper := strings.ToUpper(clean)
	if strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESC") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "VALUES") ||
		strings.HasPrefix(upper, "TABLE") ||
		strings.HasPrefix(upper, "CHECK TABLE") ||
		strings.HasPrefix(upper, "ANALYZE TABLE") ||
		strings.HasPrefix(upper, "OPTIMIZE TABLE") ||
		strings.HasPrefix(upper, "REPAIR TABLE") {
		return true
	}
	if strings.HasPrefix(upper, "WITH") && !strings.Contains(upper, "INSERT ") && !strings.Contains(upper, "UPDATE ") && !strings.Contains(upper, "DELETE ") {
		return true
	}
	return false
}

// SplitSQLStatements splits SQL text into executable statements. It honors MySQL `DELIMITER` lines
// and, under the default ";" terminator, is quote/comment aware: '...' (with ” and, for MySQL,
// backslash escapes), "...", `...`, -- and /* */ comments (# too for MySQL), and PostgreSQL
// dollar quotes ($$...$$ and $tag$...$tag$). Comment-only fragments are dropped.
func SplitSQLStatements(sqlText, driverName string) []string {
	isMySQL := strings.EqualFold(driverName, "mysql") || strings.EqualFold(driverName, "mariadb")
	return SplitWithDelimiters(sqlText, isMySQL, func(seg string) []string {
		return splitDefaultDelimiter(seg, isMySQL)
	})
}

// dollarTagAt returns the dollar-quote opener at runes[i] ("$$" or "$tag$"), or "" if none.
// A tag cannot start with a digit, so positional parameters like $1 are never mistaken for one.
func dollarTagAt(runes []rune, i int) string {
	if runes[i] != '$' {
		return ""
	}
	// Like PostgreSQL's lexer: "$" inside an identifier (price$usd) never opens a dollar quote.
	if i > 0 && (runes[i-1] == '_' || runes[i-1] == '$' || unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
		return ""
	}
	j := i + 1
	for j < len(runes) && (runes[j] == '_' || unicode.IsLetter(runes[j]) || (j > i+1 && unicode.IsDigit(runes[j]))) {
		j++
	}
	if j < len(runes) && runes[j] == '$' {
		return string(runes[i : j+1])
	}
	return ""
}

func splitDefaultDelimiter(content string, isMySQL bool) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote, inDoubleQuote, inBacktick := false, false, false
	inLineComment, inBlockComment := false, false
	dollarTag := "" // non-empty while inside a dollar-quoted body

	runes := []rune(content)
	n := len(runes)

	isQuoteEscaped := func(i int) bool {
		if !isMySQL {
			return false
		}
		bs := 0
		for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
			bs++
		}
		return bs%2 == 1
	}
	hasPrefixAt := func(i int, s string) bool {
		rs := []rune(s)
		if i+len(rs) > n {
			return false
		}
		for k, r := range rs {
			if runes[i+k] != r {
				return false
			}
		}
		return true
	}

	for i := 0; i < n; i++ {
		r := runes[i]

		if inLineComment {
			current.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			current.WriteRune(r)
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				inBlockComment = false
				current.WriteRune('/')
				i++
			}
			continue
		}
		if dollarTag != "" {
			if hasPrefixAt(i, dollarTag) {
				current.WriteString(dollarTag)
				i += len([]rune(dollarTag)) - 1
				dollarTag = ""
				continue
			}
			current.WriteRune(r)
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inLineComment = true
				current.WriteString("--")
				i++
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				current.WriteRune('#')
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				inBlockComment = true
				current.WriteString("/*")
				i++
				continue
			}
			if !isMySQL {
				if tag := dollarTagAt(runes, i); tag != "" {
					dollarTag = tag
					current.WriteString(tag)
					i += len([]rune(tag)) - 1
					continue
				}
			}
		}

		switch {
		case r == '\'' && !inDoubleQuote && !inBacktick:
			if inSingleQuote && i+1 < n && runes[i+1] == '\'' {
				current.WriteString("''")
				i++
				continue
			}
			if !isQuoteEscaped(i) {
				inSingleQuote = !inSingleQuote
			}
		case r == '"' && !inSingleQuote && !inBacktick:
			if !isQuoteEscaped(i) {
				inDoubleQuote = !inDoubleQuote
			}
		case r == '`' && !inSingleQuote && !inDoubleQuote:
			inBacktick = !inBacktick
		}

		if r == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			if stmt := strings.TrimSpace(current.String()); stmt != "" && HasExecutableSQL(stmt, isMySQL) {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(r)
	}

	if last := strings.TrimSpace(current.String()); last != "" && HasExecutableSQL(last, isMySQL) {
		statements = append(statements, last)
	}
	return statements
}

// SplitWithDelimiters honors MySQL client `DELIMITER xx` lines (used by mysqldump and routine scripts
// so BEGIN ... END bodies keep their inner semicolons). Segments under the default ";" delimiter go
// through split; segments under a custom delimiter are cut at each occurrence of it.
// ponytail: custom-delimiter segments are cut textually (not quote-aware), same as the mysql client's
// common usage; upgrade path: teach the token-aware splitters a variable terminator.
func SplitWithDelimiters(sqlText string, isMySQL bool, split func(string) []string) []string {
	delim := ";"
	var out []string
	var seg strings.Builder
	flush := func() {
		text := seg.String()
		seg.Reset()
		if delim == ";" {
			out = append(out, split(text)...)
			return
		}
		for _, part := range strings.Split(text, delim) {
			if part = strings.TrimSpace(part); part != "" && HasExecutableSQL(part, isMySQL) {
				out = append(out, part)
			}
		}
	}
	for _, line := range strings.SplitAfter(sqlText, "\n") {
		t := strings.TrimSpace(line)
		if len(t) > len("DELIMITER ") && strings.EqualFold(t[:len("DELIMITER ")], "DELIMITER ") {
			flush()
			delim = strings.Fields(t[len("DELIMITER "):])[0]
			continue
		}
		seg.WriteString(line)
	}
	flush()
	return out
}

// HasExecutableSQL checks whether a SQL statement string contains executable tokens
// (i.e. not entirely comments, whitespace, or empty semicolons).
func HasExecutableSQL(stmt string, isMySQL bool) bool {
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inBlockComment := false
	inLineComment := false

	runes := []rune(stmt)
	n := len(runes)
	tokenCount := 0

	for i := 0; i < n; i++ {
		r := runes[i]
		if inLineComment {
			if r == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inLineComment = true
				i++
				continue
			}
			if isMySQL && r == '#' {
				inLineComment = true
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				if isMySQL && i+2 < n && runes[i+2] == '!' {
					// In MySQL, /*! ... */ is a versioned executable comment containing SQL
					tokenCount++
					i += 2
					continue
				}
				inBlockComment = true
				i++
				continue
			}
		}
		if r == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
			tokenCount++
			continue
		}
		if r == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
			tokenCount++
			continue
		}
		if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
			tokenCount++
			continue
		}
		if !unicode.IsSpace(r) && r != ';' {
			tokenCount++
		}
	}
	return tokenCount > 0
}

// EscapeSQLString escapes a string literal for the specified SQL driver dialect.
// ponytail: MySQL/MariaDB requires escaping backslashes and control chars; standard SQL doubles single quotes.
func EscapeSQLString(s string, driverName string) string {
	switch strings.ToLower(driverName) {
	case "mysql", "mariadb":
		var sb strings.Builder
		sb.WriteByte('\'')
		for i := 0; i < len(s); i++ {
			b := s[i]
			switch b {
			case 0:
				sb.WriteString(`\0`)
			case '\n':
				sb.WriteString(`\n`)
			case '\r':
				sb.WriteString(`\r`)
			case '\\':
				sb.WriteString(`\\`)
			case '\'':
				sb.WriteString(`\'`)
			case '"':
				sb.WriteString(`\"`)
			case '\032':
				sb.WriteString(`\Z`)
			default:
				sb.WriteByte(b)
			}
		}
		sb.WriteByte('\'')
		return sb.String()
	default:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''"))
	}
}

// FormatSQLValueForDriver formats a Go value into a valid SQL literal for a specific driver dialect.
func FormatSQLValueForDriver(val any, driverName string) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return "NULL"
		}
		return strconv.FormatFloat(f, 'f', -1, 32)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "NULL"
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	case []byte:
		return hexLiteral(v, driverName)
	case string:
		// Streamed rows carry binary columns as Go strings; NUL or non-UTF-8 bytes cannot
		// round-trip through a quoted literal, so emit them as a binary hex literal.
		if strings.IndexByte(v, 0) >= 0 || !utf8.ValidString(v) {
			return hexLiteral([]byte(v), driverName)
		}
		return EscapeSQLString(v, driverName)
	default:
		return EscapeSQLString(fmt.Sprintf("%v", v), driverName)
	}
}

// hexLiteral renders raw bytes as a dialect-correct binary literal (PostgreSQL X” is a bit string, not bytea).
func hexLiteral(b []byte, driverName string) string {
	switch strings.ToLower(driverName) {
	case "postgres", "postgresql", "pgx":
		return fmt.Sprintf("'\\x%x'::bytea", b)
	default:
		return fmt.Sprintf("X'%X'", b)
	}
}

// FormatSQLValue formats a Go value into a valid SQL literal for dumps, inserts, and filters.
// ponytail: Uses 'f' formatting to avoid scientific notation on large numeric IDs and metrics.
func FormatSQLValue(val any) string {
	return FormatSQLValueForDriver(val, "")
}

// SequenceResetStatements returns PostgreSQL statements that advance each serial/nextval column's
// sequence past the dumped rows, so inserts after a restore don't collide with restored ids.
// Other drivers need nothing (AUTO_INCREMENT / rowid track the max automatically).
func SequenceResetStatements(tbl TableMetadata, driverName string) []string {
	d := strings.ToLower(driverName)
	if d != "postgres" && d != "postgresql" && d != "pg" {
		return nil
	}
	var out []string
	qTable := QuoteIdentifier(tbl.Name, d)
	for _, col := range tbl.Columns {
		if !strings.HasPrefix(strings.ToLower(col.DefaultValue), "nextval(") {
			continue
		}
		qCol := QuoteIdentifier(col.Name, d)
		out = append(out, fmt.Sprintf("SELECT setval(pg_get_serial_sequence(%s, %s), COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false);",
			EscapeSQLString(qTable, d), EscapeSQLString(col.Name, d), qCol, qTable))
	}
	return out
}

// TopologicalSortTables orders tables so that referenced parent tables appear before dependent child tables.
// ponytail: Kahn's algorithm resolves FK dependency DAG with deterministic cycle fallback.
func TopologicalSortTables(tables []TableMetadata) []TableMetadata {
	tableMap := make(map[string]TableMetadata, len(tables))
	for _, t := range tables {
		tableMap[strings.ToLower(t.Name)] = t
	}

	dependents := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if _, ok := inDegree[lowName]; !ok {
			inDegree[lowName] = 0
		}
		for _, fk := range t.ForeignKeys {
			refTable := strings.ToLower(fk.ReferencedTable)
			if refTable != "" && refTable != lowName {
				if _, exists := tableMap[refTable]; exists {
					dependents[refTable] = append(dependents[refTable], lowName)
					inDegree[lowName]++
				}
			}
		}
	}

	var queue []string
	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if inDegree[lowName] == 0 {
			queue = append(queue, lowName)
		}
	}

	var sortedNames []string
	visited := make(map[string]bool)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if visited[curr] {
			continue
		}
		visited[curr] = true
		sortedNames = append(sortedNames, curr)

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 && !visited[dep] {
				queue = append(queue, dep)
			}
		}
	}

	for _, t := range tables {
		lowName := strings.ToLower(t.Name)
		if !visited[lowName] {
			sortedNames = append(sortedNames, lowName)
			visited[lowName] = true
		}
	}

	var result []TableMetadata
	for _, name := range sortedNames {
		if t, ok := tableMap[name]; ok {
			result = append(result, t)
		}
	}
	return result
}
