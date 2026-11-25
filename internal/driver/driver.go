package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	ErrUnsupportedDriver   = errors.New("unsupported database driver")
	ErrSessionClosed       = errors.New("database session is closed")
	ErrFeatureNotSupported = errors.New("feature not supported by this database driver")
)

// Driver manages database connection dialing.
type Driver interface {
	Connect(ctx context.Context, cfg Config) (Session, error)
	Name() string
}

// Config holds all connection parameters needed by a driver.
type Config struct {
	Driver       string // mysql, postgres, sqlite
	Host         string
	Port         int
	DatabaseName string
	User         string
	Password     string
	UseSSL       bool
	SSLMode      string // disable, require, verify-ca, verify-full (optional; defaults to verify-full when UseSSL is true)
	SSLCA        string
	SSLCert      string
	SSLKey       string
}

// Session represents an active connection to a target database with core query, schema,
// and transaction capabilities. Optional engine-specific features (processlist, cluster health,
// user management, etc.) are exposed via dedicated capability interfaces below.
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

	// PeekForeignKeyRow queries parent table for a single row matching a foreign key value.
	PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*PeekResult, error)

	// ExecRaw executes a raw SQL statement bypassing the Safety Guard. For internal server use only.
	ExecRaw(ctx context.Context, sql string, args ...any) (int64, error)

	// QueryRaw executes a raw SQL query bypassing the Safety Guard and returns all rows. For internal server use only.
	QueryRaw(ctx context.Context, sql string, args ...any) ([][]any, error)

	// BeginTx initiates a database transaction on a dedicated physical connection.
	BeginTx(ctx context.Context) (TxSession, error)

	io.Closer
}

// ScriptSession is an optional capability that executes a multi-statement script on a single
// pinned physical connection so session-scoped commands (SET FOREIGN_KEY_CHECKS, BEGIN/COMMIT,
// PRAGMA foreign_keys) apply consistently and never pollute other pooled connections.
type ScriptSession interface {
	ExecScriptConn(ctx context.Context, fn func(exec func(context.Context, string) (*ExecResult, error)) error) error
}

// ReadOnlySession is an optional capability that enforces database-engine read-only execution
// (e.g., READ ONLY transaction in MySQL/PostgreSQL, PRAGMA query_only=ON in SQLite).
type ReadOnlySession interface {
	QueryStreamReadOnly(ctx context.Context, sql string, limit int) (RowStream, error)
}

// ProcessManager is an optional capability for drivers that support active thread/process inspection and termination.
type ProcessManager interface {
	GetProcesslist(ctx context.Context) (*ProcesslistResult, error)
	KillProcess(ctx context.Context, threadID int64, killConnection bool) error
}

// ClusterInspector is an optional capability for cluster topology, server variables, and flight telemetry.
type ClusterInspector interface {
	InspectClusterHealth(ctx context.Context) (*ClusterHealthReport, error)
	InspectServerVariables(ctx context.Context) (*ServerVariablesReport, error)
	SampleFlightMetrics(ctx context.Context) (*FlightMetricSample, error)
}

// DeadlockInspector is an optional capability for engine deadlock diagnostics (e.g. InnoDB).
type DeadlockInspector interface {
	InspectDeadlock(ctx context.Context) (*DeadlockReport, error)
}

// StatementDigestProvider is an optional capability for aggregated SQL digest telemetry.
type StatementDigestProvider interface {
	GetStatementDigest(ctx context.Context, limit int) ([]StatementDigestEntry, error)
	ResetStatementDigest(ctx context.Context) error
}

// DiagnosticsProvider is an optional capability for storage diagnostics, maintenance, and lock inspection.
type DiagnosticsProvider interface {
	GetDatabaseDiagnostics(ctx context.Context) ([]DatabaseDiagnostics, error)
	GetTableDiagnostics(ctx context.Context, database, table string) ([]TableDiagnostics, error)
	ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]MaintenanceResult, error)
	GetRedundantIndexes(ctx context.Context, database, table string) ([]RedundantIndex, error)
	InspectTableLocks(ctx context.Context, database, table string) ([]TableLockInfo, error)
}

// PartitionInspector is an optional capability for table partition telemetry.
type PartitionInspector interface {
	GetTablePartitions(ctx context.Context, database, table string) ([]PartitionMetadata, error)
}

// UserManager is an optional capability for database user and privilege management.
type UserManager interface {
	GetUserList(ctx context.Context) ([]DatabaseUser, error)
	GetUserGrants(ctx context.Context, user, host string) ([]string, error)
	SaveUser(ctx context.Context, user, host, password string, isNew bool) error
	DropUser(ctx context.Context, user, host string) error
	ApplyUserGrants(ctx context.Context, req UserGrantRequest) error
}

// RoutineInspector is an optional capability for stored procedure/function DDL inspection.
type RoutineInspector interface {
	GetRoutineDDL(ctx context.Context, routineType, name string) (string, error)
}

// EventManager is an optional capability for scheduled event DDL and status management.
type EventManager interface {
	GetEventDDL(ctx context.Context, name string) (string, error)
	ToggleEventStatus(ctx context.Context, name string, enable bool) error
}

// SafeDDLExecutor is an optional capability for executing DDL with session-level lock timeouts.
type SafeDDLExecutor interface {
	ExecSafeDDL(ctx context.Context, sql string, lockTimeoutSec int) (*ExecResult, error)
}

// PinnedProfiler is an optional capability for drivers that support session-scoped query profiling
// (e.g. MySQL SET profiling = 1 / SHOW PROFILE / SET profiling = 0) pinned to a single *sql.Conn.
// Callers of QueryStreamProfiled must invoke the returned finish function (safe to call multiple times)
// after closing/draining the stream so the profile is collected and the pinned connection is cleaned up.
type PinnedProfiler interface {
	QueryStreamProfiled(ctx context.Context, sql string, limit int, readOnly bool) (RowStream, func() *QueryProfile, error)
	ExecProfiled(ctx context.Context, sql string, args ...any) (*ExecResult, *QueryProfile, error)
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
type databaseContextKey struct{}
type explainAnalyzeContextKey struct{}

// WithQueryID associates a query ID with context for out-of-band cancellation tracking.
func WithQueryID(ctx context.Context, queryID string) context.Context {
	return context.WithValue(ctx, queryIDContextKey{}, queryID)
}

// QueryIDFromContext extracts the query ID from context if present.
func QueryIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(queryIDContextKey{}).(string); ok {
		return v
	}
	return ""
}

// WithDatabase associates an explicit target database/schema with context so concurrent
// requests on a shared Session do not race over global activeDB/activeSchema state.
func WithDatabase(ctx context.Context, dbName string) context.Context {
	if dbName == "" {
		return ctx
	}
	return context.WithValue(ctx, databaseContextKey{}, dbName)
}

// DatabaseFromContext extracts the per-execution target database/schema from context if present.
func DatabaseFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(databaseContextKey{}).(string); ok {
		return v
	}
	return ""
}

// WithExplainAnalyze enables opt-in EXPLAIN ANALYZE execution (which actually runs the query).
func WithExplainAnalyze(ctx context.Context, analyze bool) context.Context {
	return context.WithValue(ctx, explainAnalyzeContextKey{}, analyze)
}

// ExplainAnalyzeFromContext reports whether the caller explicitly opted into EXPLAIN ANALYZE.
func ExplainAnalyzeFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(explainAnalyzeContextKey{}).(bool)
	return v
}

// FlightMetricSample represents an instant performance telemetry slice.

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
