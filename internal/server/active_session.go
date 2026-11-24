package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/vault"
)

type activeTxInfo struct {
	tx          driver.TxSession
	startedAt   time.Time
	lastActive  time.Time
	aborted     bool
	abortReason string
}

// ActiveSession holds an active database connection session and its cached metadata.
// It acts as a deep module providing the Safe Execution Pipeline across all transports.
type ActiveSession struct {
	ID              string                 `json:"id"`
	ConnectionID    string                 `json:"connection_id"`
	Name            string                 `json:"name"`
	DatabaseName    string                 `json:"database_name,omitempty"`
	Environment     string                 `json:"environment"`
	DriverName      string                 `json:"driver_name"`
	ReadOnly        bool                   `json:"read_only"`
	Session         driver.Session         `json:"-"`
	CachedSchema    *driver.SchemaMetadata `json:"schema,omitempty"`
	ServerInfo      *driver.ServerInfo     `json:"server_info,omitempty"`
	LatencyMs       int64                  `json:"latency_ms"`
	MemoryMB        float64                `json:"memory_mb,omitempty"`
	LastActive      time.Time              `json:"last_active"`
	backupDir       string
	vault           *vault.Vault
	sessionMgr      *SessionManager
	runningQueries  map[string]context.CancelFunc
	tabTransactions map[string]*activeTxInfo
	closed          bool
	runSeq          uint64
	inFlight        int // number of Execute/ExecuteStream/Stream calls currently running
	auditWg         sync.WaitGroup
	dbSwitchMu      sync.Mutex
	mu              sync.RWMutex
}

// beginInFlight marks that a request is actively executing against this session (and any pinned
// transaction), so the reaper will not roll back its transaction or evict it mid-flight.
func (s *ActiveSession) beginInFlight() {
	s.mu.Lock()
	s.inFlight++
	s.LastActive = time.Now()
	s.mu.Unlock()
}

// endInFlight marks the end of an in-flight request without refreshing unrelated tab transactions.
func (s *ActiveSession) endInFlight() {
	s.endInFlightTab("")
}

// endInFlightTab marks the end of an in-flight request and refreshes only the specified tab's
// transaction activity timestamp so activity in Tab B never keeps an abandoned transaction in Tab A alive.
func (s *ActiveSession) endInFlightTab(tabID string) {
	s.mu.Lock()
	if s.inFlight > 0 {
		s.inFlight--
	}
	now := time.Now()
	s.LastActive = now
	if tabID != "" && s.tabTransactions != nil {
		if entry, ok := s.tabTransactions[tabID]; ok && entry != nil && entry.tx != nil {
			entry.lastActive = now
		}
	}
	s.mu.Unlock()
}

// Close gracefully releases the session, rolling back any uncommitted transactions to avoid orphaned locks.
func (s *ActiveSession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	var txsToRollback []driver.TxSession
	if s.tabTransactions != nil {
		for _, entry := range s.tabTransactions {
			if entry != nil && entry.tx != nil {
				txsToRollback = append(txsToRollback, entry.tx)
			}
		}
		s.tabTransactions = nil
	}
	// Keep s.Session set: in-flight requests holding this *ActiveSession then get a clean
	// "database is closed" error from the driver instead of a nil-pointer panic.
	sess := s.Session
	alreadyClosed := s.closed
	s.closed = true
	s.mu.Unlock()

	for _, tx := range txsToRollback {
		_ = tx.Rollback()
	}

	s.auditWg.Wait()

	if sess != nil && !alreadyClosed {
		return sess.Close()
	}
	return nil
}

func normalizeTabID(tabID string) string {
	if tabID == "" {
		return "default"
	}
	return tabID
}

func (s *ActiveSession) getTxStatus(tabID string) (tx driver.TxSession, aborted bool, reason string) {
	key := normalizeTabID(tabID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tabTransactions != nil {
		if entry, ok := s.tabTransactions[key]; ok && entry != nil {
			if entry.aborted {
				return nil, true, entry.abortReason
			}
			if entry.tx != nil {
				entry.lastActive = time.Now()
				return entry.tx, false, ""
			}
		}
	}
	return nil, false, ""
}

func (s *ActiveSession) getTx(tabID string) driver.TxSession {
	tx, _, _ := s.getTxStatus(tabID)
	return tx
}

func (s *ActiveSession) setTx(tabID string, tx driver.TxSession) {
	key := normalizeTabID(tabID)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.tabTransactions == nil {
		s.tabTransactions = make(map[string]*activeTxInfo)
	}
	if tx != nil {
		s.tabTransactions[key] = &activeTxInfo{
			tx:         tx,
			startedAt:  now,
			lastActive: now,
		}
	} else {
		delete(s.tabTransactions, key)
	}
}

func (s *ActiveSession) clearTx(tabID string) driver.TxSession {
	key := normalizeTabID(tabID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tabTransactions != nil {
		if entry, ok := s.tabTransactions[key]; ok && entry != nil {
			delete(s.tabTransactions, key)
			return entry.tx
		}
	}
	return nil
}

func (s *ActiveSession) hasActiveTx(tabID string) bool {
	key := normalizeTabID(tabID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.tabTransactions != nil {
		if entry, ok := s.tabTransactions[key]; ok && entry != nil && entry.tx != nil && !entry.aborted {
			return true
		}
	}
	return false
}

// Mutable session state is read and written by concurrent requests, so it goes through s.mu.

func (s *ActiveSession) IsReadOnly() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ReadOnly
}

// ToggleReadOnly flips the read-only lock and returns the new state.
func (s *ActiveSession) ToggleReadOnly() bool {
	s.mu.Lock()
	s.ReadOnly = !s.ReadOnly
	ro := s.ReadOnly
	sm := s.sessionMgr
	connID := s.ConnectionID
	s.mu.Unlock()

	if sm != nil && connID != "" {
		sm.SetReadOnly(connID, ro)
	}
	return ro
}

func (s *ActiveSession) Schema() *driver.SchemaMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CachedSchema
}

// SetSchema replaces the cached schema; nil is ignored so a nil-check followed by Schema() stays safe.
func (s *ActiveSession) SetSchema(sc *driver.SchemaMetadata) {
	if sc == nil {
		return
	}
	s.mu.Lock()
	s.CachedSchema = sc
	s.mu.Unlock()
}

func (s *ActiveSession) Info() *driver.ServerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ServerInfo
}

func (s *ActiveSession) SetInfo(info *driver.ServerInfo) {
	if info == nil {
		return
	}
	s.mu.Lock()
	s.ServerInfo = info
	s.mu.Unlock()
}

func (s *ActiveSession) SetLatency(ms int64) {
	s.mu.Lock()
	s.LatencyMs = ms
	s.mu.Unlock()
}

// liveSession returns the driver session for a read-only diagnostic call without holding the
// session lock across the (blocking) DB round-trip. s.Session is immutable after the session is
// published, so snapshotting it under a brief lock is race-free; callers must not hold s.mu while
// invoking the returned session. Returns ErrSessionClosed once Close has run.
func (s *ActiveSession) liveSession() (driver.Session, error) {
	s.mu.RLock()
	sess := s.Session
	closed := s.closed
	s.mu.RUnlock()
	if sess == nil || closed {
		return nil, driver.ErrSessionClosed
	}
	return sess, nil
}

// MarshalJSON snapshots the exported fields under the lock.
func (s *ActiveSession) MarshalJSON() ([]byte, error) {
	type view struct {
		ID           string                 `json:"id"`
		ConnectionID string                 `json:"connection_id"`
		Name         string                 `json:"name"`
		DatabaseName string                 `json:"database_name,omitempty"`
		Environment  string                 `json:"environment"`
		DriverName   string                 `json:"driver_name"`
		ReadOnly     bool                   `json:"read_only"`
		CachedSchema *driver.SchemaMetadata `json:"schema,omitempty"`
		ServerInfo   *driver.ServerInfo     `json:"server_info,omitempty"`
		LatencyMs    int64                  `json:"latency_ms"`
		MemoryMB     float64                `json:"memory_mb,omitempty"`
		LastActive   time.Time              `json:"last_active"`
	}
	s.mu.RLock()
	v := view{s.ID, s.ConnectionID, s.Name, s.DatabaseName, s.Environment, s.DriverName, s.ReadOnly, s.CachedSchema, s.ServerInfo, s.LatencyMs, s.MemoryMB, s.LastActive}
	s.mu.RUnlock()
	return json.Marshal(v)
}

// sqlExecer is satisfied by driver.Session and driver.TxSession.
type sqlExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (*driver.ExecResult, error)
}

// gridWriter returns the executor for grid mutations plus a finish func. When the user has an open
// transaction the grid joins it (instead of blocking on that transaction's own row locks on another
// pooled connection) and finish is a no-op; otherwise a fresh transaction makes multi-row edits
// all-or-nothing: finish(nil) commits, finish(err) rolls back and returns err.
func (s *ActiveSession) gridWriter(ctx context.Context) (sqlExecer, func(error) error, error) {
	s.mu.Lock()
	var tx driver.TxSession
	if s.tabTransactions != nil {
		if entry, ok := s.tabTransactions["default"]; ok && entry != nil && entry.tx != nil {
			tx = entry.tx
			entry.lastActive = time.Now()
		} else if s.DriverName == "sqlite" {
			for _, entry := range s.tabTransactions {
				if entry != nil && entry.tx != nil {
					tx = entry.tx
					entry.lastActive = time.Now()
					break
				}
			}
		}
	}
	sess := s.Session
	s.mu.Unlock()
	if tx != nil {
		return tx, func(err error) error { return err }, nil
	}
	if sess == nil {
		return nil, nil, errors.New("no active database session")
	}
	own, err := sess.BeginTx(ctx)
	if err != nil {
		return nil, nil, err
	}
	return own, func(err error) error {
		if err != nil {
			_ = own.Rollback()
			return err
		}
		return own.Commit()
	}, nil
}

// rejectIfReadOnly writes 403 and returns true when the session is locked read-only.
func rejectIfReadOnly(w http.ResponseWriter, sess *ActiveSession) bool {
	if sess.IsReadOnly() {
		writeError(w, http.StatusForbidden, "Session is locked in READ-ONLY mode")
		return true
	}
	return false
}

// GetDatabaseName returns the current active database name in a thread-safe manner.
func (s *ActiveSession) GetDatabaseName() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DatabaseName
}

// UseDatabase selects the active database context if a non-empty name is provided,
// serializing driver-level database switches on the shared session.
func (s *ActiveSession) UseDatabase(ctx context.Context, dbName string) error {
	if dbName == "" || s.Session == nil {
		return nil
	}
	s.dbSwitchMu.Lock()
	defer s.dbSwitchMu.Unlock()
	if err := s.Session.SelectDatabase(ctx, dbName); err != nil {
		return err
	}
	s.mu.Lock()
	s.DatabaseName = dbName
	s.mu.Unlock()
	return nil
}

// RecordAudit asynchronously records an executed SQL statement or data mutation into the vault audit log.
// WaitGroup-tracked goroutines ensure zero impact on query latency while preventing lost writes on Close().
func (s *ActiveSession) RecordAudit(querySQL, source string, durationMs, rowsAffected int64, execErr error, executedAt ...time.Time) {
	if s == nil || s.vault == nil || strings.TrimSpace(querySQL) == "" {
		return
	}
	if source == "" {
		source = "editor"
	}
	errStr := ""
	if execErr != nil {
		errStr = execErr.Error()
	}
	t := time.Now()
	if len(executedAt) > 0 && !executedAt[0].IsZero() {
		t = executedAt[0]
	}
	cleanSQL := strings.TrimSpace(querySQL)

	s.mu.RLock()
	closed := s.closed
	if !closed {
		s.auditWg.Add(1)
	}
	s.mu.RUnlock()

	if !closed {
		go func() {
			defer s.auditWg.Done()
			if err := s.vault.RecordAuditLog(&vault.AuditLogRecord{
				SessionID:    s.ID,
				ConnectionID: s.ConnectionID,
				QuerySQL:     cleanSQL,
				Source:       source,
				DurationMs:   durationMs,
				RowsAffected: rowsAffected,
				Error:        errStr,
				ExecutedAt:   t,
			}); err != nil {
				slog.Warn("audit_log_write_failed", "module", "audit", "error", err.Error())
			}
		}()
	}

	// ponytail: Emits structured telemetry event for live Activity Console
	if execErr != nil {
		slog.Error("query_failed",
			"module", "query",
			"sql", cleanSQL,
			"source", source,
			"duration_ms", durationMs,
			"error", errStr,
		)
	} else {
		slog.Info("query_executed",
			"module", "query",
			"sql", cleanSQL,
			"source", source,
			"duration_ms", durationMs,
			"rows", rowsAffected,
		)
	}
}

// CancelQuery attempts to interrupt a running query by queryID.
// registerQuery records cancel under a unique run key derived from the client's query ID.
func (s *ActiveSession) registerQuery(queryID string, cancel context.CancelFunc) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runningQueries == nil {
		s.runningQueries = make(map[string]context.CancelFunc)
	}
	s.runSeq++
	key := fmt.Sprintf("%s#%d", queryID, s.runSeq)
	s.runningQueries[key] = cancel
	return key
}

// CancelQuery cancels every running execution registered under queryID ("active"/"" cancels all),
// both server-side (driver KILL / pg_cancel_backend per run key) and client-side (context).
func (s *ActiveSession) CancelQuery(ctx context.Context, queryID string) error {
	all := queryID == "" || queryID == "active"
	s.mu.Lock()
	var keys []string
	var cancels []context.CancelFunc
	for k, c := range s.runningQueries {
		if all || k == queryID || strings.HasPrefix(k, queryID+"#") {
			keys = append(keys, k)
			if c != nil {
				cancels = append(cancels, c)
			}
		}
	}
	sess := s.Session
	s.mu.Unlock()

	// ponytail: Dispatch server-side KILL first while query mapping is still registered in driver
	var firstErr error
	if sess != nil {
		if all || len(keys) == 0 {
			firstErr = sess.CancelQuery(ctx, queryID)
		} else {
			for _, k := range keys {
				if err := sess.CancelQuery(ctx, k); err != nil && firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	// Always invoke client-side cancel funcs after server-side cancellation has been dispatched
	for _, c := range cancels {
		c()
	}

	return firstErr
}

func execSafeDDLOrFallback(ctx context.Context, sess driver.Session, sql string, lockTimeoutSec int) (*driver.ExecResult, error) {
	if safeExec, ok := sess.(driver.SafeDDLExecutor); ok {
		if lockTimeoutSec <= 0 {
			lockTimeoutSec = 3 // default fail-fast timeout (3s)
		}
		return safeExec.ExecSafeDDL(ctx, sql, lockTimeoutSec)
	}
	return sess.Exec(ctx, sql)
}

func requireCapability[T any](s *ActiveSession, feature string) (T, error) {
	var zero T
	sess, err := s.liveSession()
	if err != nil {
		return zero, err
	}
	capImpl, ok := sess.(T)
	if !ok {
		return zero, fmt.Errorf("%w: %s", driver.ErrFeatureNotSupported, feature)
	}
	return capImpl, nil
}

// GetProcesslist retrieves active threads and lock wait graph from the active driver.
func (s *ActiveSession) GetProcesslist(ctx context.Context) (*driver.ProcesslistResult, error) {
	pm, err := requireCapability[driver.ProcessManager](s, "processlist")
	if err != nil {
		return nil, err
	}
	return pm.GetProcesslist(ctx)
}

// InspectClusterHealth retrieves cluster topology, node synchronization, and bottleneck diagnostics.
func (s *ActiveSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	ci, err := requireCapability[driver.ClusterInspector](s, "cluster health")
	if err != nil {
		return nil, err
	}
	return ci.InspectClusterHealth(ctx)
}

// InspectServerVariables retrieves server configuration parameters and tuning heuristics.
func (s *ActiveSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	ci, err := requireCapability[driver.ClusterInspector](s, "server variables")
	if err != nil {
		return nil, err
	}
	return ci.InspectServerVariables(ctx)
}

// KillProcess terminates an active query or client connection out-of-band.
func (s *ActiveSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	pm, err := requireCapability[driver.ProcessManager](s, "kill process")
	if err != nil {
		return err
	}
	return pm.KillProcess(ctx, threadID, killConnection)
}

// Stream executes a non-destructive query, returning a chunked RowStream.
// Destructive queries are rejected to protect WebSocket callers from uninspected DDL/DML.

// GetDatabaseDiagnostics returns physical storage and metadata for all databases.
func (s *ActiveSession) GetDatabaseDiagnostics(ctx context.Context) ([]driver.DatabaseDiagnostics, error) {
	dp, err := requireCapability[driver.DiagnosticsProvider](s, "database diagnostics")
	if err != nil {
		return nil, err
	}
	return dp.GetDatabaseDiagnostics(ctx)
}

// GetTableDiagnostics returns physical storage and overhead metrics.
func (s *ActiveSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	dp, err := requireCapability[driver.DiagnosticsProvider](s, "table diagnostics")
	if err != nil {
		return nil, err
	}
	return dp.GetTableDiagnostics(ctx, database, table)
}

// ExecuteMaintenance runs maintenance actions (OPTIMIZE, ANALYZE, CHECK, VACUUM).
func (s *ActiveSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	dp, err := requireCapability[driver.DiagnosticsProvider](s, "table maintenance")
	if err != nil {
		return nil, err
	}
	return dp.ExecuteMaintenance(ctx, database, tables, action)
}

// GetRedundantIndexes detects duplicate, shadowed, and redundant primary key indexes.
func (s *ActiveSession) GetRedundantIndexes(ctx context.Context, database, table string) ([]driver.RedundantIndex, error) {
	dp, err := requireCapability[driver.DiagnosticsProvider](s, "redundant index detection")
	if err != nil {
		return nil, err
	}
	return dp.GetRedundantIndexes(ctx, database, table)
}

// PeekForeignKeyRow queries parent table for a single row matching a foreign key value.
func (s *ActiveSession) PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*driver.PeekResult, error) {
	sess, err := s.liveSession()
	if err != nil {
		return nil, err
	}
	return sess.PeekForeignKeyRow(ctx, database, table, column, value)
}

// GetUserList lists database users.
func (s *ActiveSession) GetUserList(ctx context.Context) ([]driver.DatabaseUser, error) {
	um, err := requireCapability[driver.UserManager](s, "user management")
	if err != nil {
		return nil, err
	}
	return um.GetUserList(ctx)
}

// GetUserGrants returns active grants for a user account.
func (s *ActiveSession) GetUserGrants(ctx context.Context, user, host string) ([]string, error) {
	um, err := requireCapability[driver.UserManager](s, "user management")
	if err != nil {
		return nil, err
	}
	return um.GetUserGrants(ctx, user, host)
}

// SaveUser creates or updates a user account.
func (s *ActiveSession) SaveUser(ctx context.Context, user, host, password string, isNew bool) error {
	um, err := requireCapability[driver.UserManager](s, "user management")
	if err != nil {
		return err
	}
	return um.SaveUser(ctx, user, host, password, isNew)
}

// DropUser removes a user account.
func (s *ActiveSession) DropUser(ctx context.Context, user, host string) error {
	um, err := requireCapability[driver.UserManager](s, "user management")
	if err != nil {
		return err
	}
	return um.DropUser(ctx, user, host)
}

// ApplyUserGrants grants privileges to a user.
func (s *ActiveSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	um, err := requireCapability[driver.UserManager](s, "user management")
	if err != nil {
		return err
	}
	return um.ApplyUserGrants(ctx, req)
}

// SampleFlightMetrics captures performance telemetry and correlated queries from the driver session.
func (s *ActiveSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	ci, err := requireCapability[driver.ClusterInspector](s, "flight metrics")
	if err != nil {
		return nil, err
	}
	return ci.SampleFlightMetrics(ctx)
}

// InspectDeadlock delegates deadlock analysis to the driver session.
func (s *ActiveSession) InspectDeadlock(ctx context.Context) (*driver.DeadlockReport, error) {
	di, err := requireCapability[driver.DeadlockInspector](s, "deadlock inspection")
	if err != nil {
		return nil, err
	}
	return di.InspectDeadlock(ctx)
}

// GetStatementDigest delegates statement digest queries to the driver session.
func (s *ActiveSession) GetStatementDigest(ctx context.Context, limit int) ([]driver.StatementDigestEntry, error) {
	sp, err := requireCapability[driver.StatementDigestProvider](s, "statement digest")
	if err != nil {
		return nil, err
	}
	return sp.GetStatementDigest(ctx, limit)
}

// ResetStatementDigest delegates statement digest statistics clearing to the driver session.
func (s *ActiveSession) ResetStatementDigest(ctx context.Context) error {
	sp, err := requireCapability[driver.StatementDigestProvider](s, "statement digest")
	if err != nil {
		return err
	}
	return sp.ResetStatementDigest(ctx)
}

// GetRoutineDDL retrieves the raw CREATE statement for a Stored Procedure or Function.
func (s *ActiveSession) GetRoutineDDL(ctx context.Context, routineType, name string) (string, error) {
	ri, err := requireCapability[driver.RoutineInspector](s, "stored routine DDL")
	if err != nil {
		return "", err
	}
	return ri.GetRoutineDDL(ctx, routineType, name)
}

// GetEventDDL retrieves the raw CREATE statement for a Scheduled Event.
func (s *ActiveSession) GetEventDDL(ctx context.Context, name string) (string, error) {
	em, err := requireCapability[driver.EventManager](s, "scheduled events")
	if err != nil {
		return "", err
	}
	return em.GetEventDDL(ctx, name)
}

// ToggleEventStatus toggles the status of a Scheduled Event.
func (s *ActiveSession) ToggleEventStatus(ctx context.Context, name string, enable bool) error {
	em, err := requireCapability[driver.EventManager](s, "scheduled events")
	if err != nil {
		return err
	}
	return em.ToggleEventStatus(ctx, name, enable)
}

// GetTablePartitions retrieves partition telemetry for a table.
func (s *ActiveSession) GetTablePartitions(ctx context.Context, database, table string) ([]driver.PartitionMetadata, error) {
	pi, err := requireCapability[driver.PartitionInspector](s, "table partitions")
	if err != nil {
		return nil, err
	}
	return pi.GetTablePartitions(ctx, database, table)
}

// inspectAllStatements runs the Safety Guard on every statement of a (possibly multi-statement) input,
// because drivers such as SQLite execute all of them in one Exec. It returns the inspection that must
// gate execution and whether every statement is a read-only query. More than one statement requiring a
