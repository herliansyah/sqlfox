package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"sqlfox/internal/driver"
	"sqlfox/internal/safety"
	"sqlfox/internal/vault"
)

var (
	ErrReadOnlySession = errors.New("Session is locked in READ-ONLY mode. Cannot execute destructive statement.")
)

// ExecuteRequest encapsulates SQL query parameters and challenge verification.
type ExecuteRequest struct {
	SQL                   string `json:"sql"`
	Confirmed             bool   `json:"confirmed"`
	ChallengeInput        string `json:"challenge_input"`
	QueryID               string `json:"query_id"`
	Database              string `json:"database"`
	Profile               bool   `json:"profile"`
	Limit                 int    `json:"limit,omitempty"`
	LockTimeoutSec        int    `json:"lock_timeout_sec,omitempty"`        // Fail-fast session lock timeout (default 3s for DDL)
	BypassMutationConfirm bool   `json:"bypass_mutation_confirm,omitempty"` // Dev-only session toggle (ADR 0057)
}

// ExecuteResponse provides the unified outcome of a mutation or buffered query.
type ExecuteResponse struct {
	Type               string                    `json:"type"` // "exec" or "query"
	Blocked            bool                      `json:"blocked,omitempty"`
	Inspection         *safety.InspectionResult  `json:"inspection,omitempty"`
	RowsAffected       int64                     `json:"rows_affected,omitempty"`
	LastInsertID       int64                     `json:"last_insert_id,omitempty"`
	Message            string                    `json:"message,omitempty"`
	BackupFile         string                    `json:"backup_file,omitempty"`
	BackupWarning      string                    `json:"backup_warning,omitempty"`
	BackupDataIncluded bool                      `json:"backup_data_included,omitempty"`
	Columns            []driver.ColumnDefinition `json:"columns,omitempty"`
	Rows               [][]any                   `json:"rows,omitempty"`
	RowCount           int                       `json:"row_count,omitempty"`
	ExecutionMs        int64                     `json:"execution_ms"`
	Profile            *driver.QueryProfile      `json:"profile,omitempty"`
	TxActive           bool                      `json:"tx_active"`
}

// ActiveSession holds an active database connection session and its cached metadata.
// It acts as a deep module providing the Safe Execution Pipeline across all transports.
type ActiveSession struct {
	ID             string                 `json:"id"`
	ConnectionID   string                 `json:"connection_id"`
	Name           string                 `json:"name"`
	Environment    string                 `json:"environment"`
	DriverName     string                 `json:"driver_name"`
	ReadOnly       bool                   `json:"read_only"`
	Session        driver.Session         `json:"-"`
	CachedSchema   *driver.SchemaMetadata `json:"schema,omitempty"`
	ServerInfo     *driver.ServerInfo     `json:"server_info,omitempty"`
	LatencyMs      int64                  `json:"latency_ms"`
	MemoryMB       float64                `json:"memory_mb,omitempty"`
	LastActive     time.Time              `json:"last_active"`
	backupDir      string
	vault          *vault.Vault
	runningQueries map[string]context.CancelFunc
	activeTx       driver.TxSession
	txStartedAt    time.Time
	txLastActive   time.Time
	closed         bool
	runSeq         uint64
	mu             sync.RWMutex
}

// Close gracefully releases the session, rolling back any uncommitted transactions to avoid orphaned locks.
func (s *ActiveSession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.activeTx != nil {
		_ = s.activeTx.Rollback()
		s.activeTx = nil
	}
	// Keep s.Session set: in-flight requests holding this *ActiveSession then get a clean
	// "database is closed" error from the driver instead of a nil-pointer panic.
	sess := s.Session
	alreadyClosed := s.closed
	s.closed = true
	s.mu.Unlock()

	if sess != nil && !alreadyClosed {
		return sess.Close()
	}
	return nil
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
	defer s.mu.Unlock()
	s.ReadOnly = !s.ReadOnly
	return s.ReadOnly
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

// MarshalJSON snapshots the exported fields under the lock.
func (s *ActiveSession) MarshalJSON() ([]byte, error) {
	type view struct {
		ID           string                 `json:"id"`
		ConnectionID string                 `json:"connection_id"`
		Name         string                 `json:"name"`
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
	v := view{s.ID, s.ConnectionID, s.Name, s.Environment, s.DriverName, s.ReadOnly, s.CachedSchema, s.ServerInfo, s.LatencyMs, s.MemoryMB, s.LastActive}
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
	tx := s.activeTx
	if tx != nil {
		s.txLastActive = time.Now()
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

// UseDatabase selects the active database context if a non-empty name is provided.
// ponytail: Inlined check eliminates repeated if db != "" { sess.Session.SelectDatabase(...) } boilerplate.
func (s *ActiveSession) UseDatabase(ctx context.Context, dbName string) error {
	if dbName != "" && s.Session != nil {
		return s.Session.SelectDatabase(ctx, dbName)
	}
	return nil
}

// RecordAudit asynchronously records an executed SQL statement or data mutation into the vault audit log.
// ponytail: Non-blocking fire-and-forget goroutine ensures zero impact on query execution latency.
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
	go func() {
		_ = s.vault.RecordAuditLog(&vault.AuditLogRecord{
			SessionID:    s.ID,
			ConnectionID: s.ConnectionID,
			QuerySQL:     cleanSQL,
			Source:       source,
			DurationMs:   durationMs,
			RowsAffected: rowsAffected,
			Error:        errStr,
			ExecutedAt:   t,
		})
	}()

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

// SessionManager manages connected database sessions.
type SessionManager struct {
	vault      *vault.Vault
	backupDir  string
	sessions   map[string]*ActiveSession
	stopReaper chan struct{}
	mu         sync.RWMutex
}

func NewSessionManager(v *vault.Vault, backupDir string) *SessionManager {
	sm := &SessionManager{
		vault:      v,
		backupDir:  backupDir,
		sessions:   make(map[string]*ActiveSession),
		stopReaper: make(chan struct{}),
	}
	go sm.startReaper()
	return sm
}

// startReaper evicts inactive sessions older than 30 minutes to prevent resource and connection pool leaks.
// ponytail: Background ticker reaps dead connections and rollbacks idle transactions without blocking active requests.
func (sm *SessionManager) startReaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sm.stopReaper:
			return
		case <-ticker.C:
			sm.mu.Lock()
			now := time.Now()
			for id, sess := range sm.sessions {
				// Rollback idle transactions inactive for 60 seconds to prevent orphan lock contention on DB server
				sess.mu.Lock()
				refTime := sess.txLastActive
				if refTime.IsZero() {
					refTime = sess.txStartedAt
				}
				if sess.activeTx != nil && now.Sub(refTime) > 60*time.Second {
					_ = sess.activeTx.Rollback()
					sess.activeTx = nil
				}
				isIdle := now.Sub(sess.LastActive) > 30*time.Minute
				sess.mu.Unlock()

				if isIdle {
					_ = sess.Close()
					delete(sm.sessions, id)
				}
			}
			sm.mu.Unlock()
		}
	}
}

// ConnectSession creates or activates a session for the given connection ID.
func (sm *SessionManager) ConnectSession(ctx context.Context, connectionID string) (*ActiveSession, error) {
	// Check existing session. The network I/O below (dial, ServerInfo, InspectSchema) runs without
	// sm.mu so one slow or unreachable database cannot stall requests for every other connection.
	sm.mu.RLock()
	if sess, ok := sm.sessions[connectionID]; ok {
		sm.mu.RUnlock()
		sess.mu.Lock()
		sess.LastActive = time.Now()
		sess.mu.Unlock()
		return sess, nil
	}
	sm.mu.RUnlock()

	conn, err := sm.vault.GetConnection(connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection config: %w", err)
	}

	cfg := driver.Config{
		Driver:       conn.Driver,
		Host:         conn.Host,
		Port:         conn.Port,
		DatabaseName: conn.DatabaseName,
		User:         conn.User,
		Password:     conn.Password,
		UseSSL:       conn.UseSSL,
		SSLCA:        conn.SSLCA,
		SSLCert:      conn.SSLCert,
		SSLKey:       conn.SSLKey,
	}

	dbSession, err := driver.Connect(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	info, _ := dbSession.ServerInfo(ctx)
	schema, _ := dbSession.InspectSchema(ctx, conn.DatabaseName)
	latency, _ := dbSession.Ping(ctx)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocMB := math.Round(float64(m.Alloc)/1024/1024*10) / 10

	active := &ActiveSession{
		ID:             connectionID,
		ConnectionID:   connectionID,
		Name:           conn.Name,
		Environment:    conn.Environment,
		DriverName:     conn.Driver,
		ReadOnly:       conn.Environment == "Production", // Production defaults to Read-Only
		Session:        dbSession,
		CachedSchema:   schema,
		ServerInfo:     info,
		LatencyMs:      latency.Milliseconds(),
		MemoryMB:       allocMB,
		LastActive:     time.Now(),
		backupDir:      sm.backupDir,
		vault:          sm.vault,
		runningQueries: make(map[string]context.CancelFunc),
	}

	sm.mu.Lock()
	if existing, ok := sm.sessions[connectionID]; ok {
		// A concurrent request connected first; keep its session and discard ours.
		sm.mu.Unlock()
		_ = dbSession.Close()
		existing.mu.Lock()
		existing.LastActive = time.Now()
		existing.mu.Unlock()
		return existing, nil
	}
	sm.sessions[connectionID] = active
	sm.mu.Unlock()
	return active, nil
}

func (sm *SessionManager) GetSession(connectionID string) (*ActiveSession, error) {
	sm.mu.RLock()
	sess, ok := sm.sessions[connectionID]
	if ok {
		sm.mu.RUnlock()
		sess.mu.Lock()
		sess.LastActive = time.Now()
		sess.mu.Unlock()
		return sess, nil
	}
	sm.mu.RUnlock()

	// ponytail: lazy auto-reconnect on demand when an evicted session is accessed
	if sm.vault != nil && connectionID != "" {
		if _, err := sm.vault.GetConnection(connectionID); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return sm.ConnectSession(ctx, connectionID)
		}
	}

	return nil, fmt.Errorf("session not found for connection %s; please connect first", connectionID)
}

// GetActiveOrSingleSession finds the session by ID, or if ID is empty and exactly one session is active, returns it.
func (sm *SessionManager) GetActiveOrSingleSession(connectionID string) (*ActiveSession, error) {
	if connectionID != "" {
		return sm.GetSession(connectionID)
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.sessions) == 1 {
		for _, sess := range sm.sessions {
			sess.mu.Lock()
			sess.LastActive = time.Now()
			sess.mu.Unlock()
			return sess, nil
		}
	}
	return nil, fmt.Errorf("no active database session found")
}

func (sm *SessionManager) CloseSession(connectionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sess, ok := sm.sessions[connectionID]; ok {
		delete(sm.sessions, connectionID)
		return sess.Close()
	}
	return nil
}

func (sm *SessionManager) CloseAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	select {
	case <-sm.stopReaper:
	default:
		close(sm.stopReaper)
	}

	for id, sess := range sm.sessions {
		_ = sess.Close()
		delete(sm.sessions, id)
	}
}

// Execute processes a SQL query through the Safe Execution Pipeline:
// 1. Evaluates statement against Safety Guard AST inspection
// 2. Enforces read-only session restrictions
// 3. Enforces Challenge Modal verification and table name matching
// 4. Triggers pre-DDL Smart Tiered Backup on destructive schema operations (DROP, TRUNCATE)
// 5. Routes to mutation execution or stream execution with the Implicit Safe Limit (1,000 rows).
func (s *ActiveSession) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	s.mu.Lock()
	s.LastActive = time.Now()
	s.mu.Unlock()

	trimmedSQL := strings.TrimSpace(req.SQL)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

	cleanSQL := driver.StripLeadingComments(trimmedSQL)
	cleanUpper := strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
	isSelect := driver.IsQueryStatement(trimmedSQL)

	if strings.HasPrefix(cleanUpper, "USE ") {
		dbTarget := strings.TrimSpace(cleanSQL[4:])
		dbTarget = strings.Trim(strings.TrimRight(dbTarget, "; \t\r\n"), "`'\"")
		if dbTarget != "" {
			_ = s.UseDatabase(ctx, dbTarget)
		}
	}

	if req.Database != "" && s.Session != nil {
		if err := s.Session.SelectDatabase(ctx, req.Database); err != nil {
			return nil, err
		}
	}

	// 1. Safety Guard Inspection with Scoped Database Context
	targetDB := req.Database
	if targetDB == "" && s.Schema() != nil {
		targetDB = s.Schema().DatabaseName
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}

	// 2. Read-Only Enforcement: blocks all destructive operations or any mutation when locked
	if s.IsReadOnly() && (!isSelect || !allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
	}

	// 3. Pre-Execution Row Estimation & Tiered Affirmation Gate (ADR 0057)
	isDev := strings.EqualFold(s.Environment, "Development")
	isMutation := inspection.Operation == "DELETE" || inspection.Operation == "UPDATE" || len(inspection.StatementEstimates) > 0
	canBypass := isDev && req.BypassMutationConfirm && !inspection.RequiresChallenge

	if isMutation && !canBypass {
		// Re-derived on confirmed requests too, so a client cannot skip the typed challenge by sending confirmed=true first.
		s.estimateAffectedRows(ctx, inspection)

		anyMissingWhere := false
		anyOverThreshold := false
		if len(inspection.StatementEstimates) > 0 {
			for _, est := range inspection.StatementEstimates {
				if !est.HasWhere {
					anyMissingWhere = true
				}
				if est.EstimatedRows != nil && *est.EstimatedRows > 100 {
					anyOverThreshold = true
				}
			}
		} else {
			if !inspection.HasWhere {
				anyMissingWhere = true
			}
			if inspection.EstimatedRows != nil && *inspection.EstimatedRows > 100 {
				anyOverThreshold = true
			}
		}

		if isDev {
			// Single-row PK/Unique mutation in Development: auto-allow without challenge
			if len(inspection.StatementEstimates) == 0 && inspection.HasWhere && inspection.EstimatedRows != nil && *inspection.EstimatedRows == 1 {
				inspection.RequiresAffirmation = false
				inspection.RequiresChallenge = false
			} else {
				// > 1 rows, 0 rows, unknown rows, unqualified, or multi-statement batch
				if anyOverThreshold || anyMissingWhere {
					inspection.RequiresChallenge = true
				} else {
					inspection.RequiresAffirmation = true
				}
			}
		} else {
			// Staging or Production: universal confirmation
			if anyOverThreshold || anyMissingWhere {
				inspection.RequiresChallenge = true
			} else {
				inspection.RequiresAffirmation = true
			}
		}
	}

	requiresGate := inspection.RequiresChallenge || (isMutation && !canBypass && inspection.RequiresAffirmation) || (!isDev && inspection.RequiresAffirmation)
	if requiresGate && !req.Confirmed {
		if inspection.TargetTable != "" && s.Session != nil {
			if locks, err := s.Session.InspectTableLocks(ctx, req.Database, inspection.TargetTable); err == nil {
				inspection.TableLocks = locks
			}
			schema := s.Schema()
			if schema == nil || (req.Database != "" && schema.DatabaseName != req.Database) {
				if sSc, sErr := s.Session.InspectSchema(ctx, req.Database); sErr == nil {
					schema = sSc
					s.SetSchema(sSc)
				}
			}
			if schema != nil {
				inspection.Relationships = driver.InspectTableRelationships(schema, inspection.TargetTable)
			}
		}
		return &ExecuteResponse{
			Blocked:    true,
			Inspection: inspection,
		}, nil
	}

	// Challenge typed table name validation (required on RequiresChallenge when not a 1-click Dev affirmation)
	if inspection.RequiresChallenge && req.Confirmed && inspection.TargetTable != "" {
		expected := inspection.TargetTable
		typed := strings.TrimSpace(req.ChallengeInput)
		if typed == "" {
			// Confirmed without a typed challenge (the requirement escalated since the preview, e.g. the
			// row estimate grew past the threshold): ask again instead of failing the request.
			return &ExecuteResponse{Blocked: true, Inspection: inspection}, nil
		}
		if !strings.EqualFold(typed, expected) && !strings.EqualFold(typed, inspection.TargetDatabase) {
			return nil, fmt.Errorf("Challenge confirmation failed: expected '%s', got '%s'", expected, req.ChallengeInput)
		}
	}

	// 4. Pre-DDL Automated Backup
	var backupFile string
	var backupDataIncluded bool
	var backupWarning string
	tt := strings.ToUpper(inspection.TargetType)
	if (inspection.Operation == "DROP" || inspection.Operation == "TRUNCATE") && (tt == "" || tt == "TABLE") {
		if inspection.TargetTable != "" && s.backupDir != "" && s.Session != nil {
			// BackupTable reads the table from the session's active database. A qualified target in
			// another database (DROP TABLE archive.orders) is not snapshotted at all rather than
			// snapshotting a same-named table here, or switching the shared session's database
			// under concurrent requests.
			if inspection.TargetDatabase != "" && targetDB != "" && !strings.EqualFold(inspection.TargetDatabase, targetDB) {
				backupWarning = fmt.Sprintf("Pre-execution backup skipped: '%s.%s' is outside the active database '%s'; run it from that database to get an automatic snapshot.",
					inspection.TargetDatabase, inspection.TargetTable, targetDB)
			} else {
				var err error
				backupFile, backupDataIncluded, err = safety.BackupTable(ctx, s.Session, s.backupDir, inspection.TargetTable, s.DriverName)
				if err != nil {
					backupWarning = "Pre-execution backup incomplete: " + err.Error()
				}
			}
			if backupWarning != "" {
				slog.Warn("pre_ddl_backup_failed", "module", "safety", "table", inspection.TargetTable, "warning", backupWarning)
			}
		}
	}

	start := time.Now()

	queryID := req.QueryID
	if queryID == "" {
		queryID = "active"
	}

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Each execution gets a unique run key: clients reuse query IDs (the editor always sends
	// "active"), and a shared key let the first query to finish unregister a still-running one.
	runKey := s.registerQuery(queryID, cancel)
	execCtx = driver.WithQueryID(execCtx, runKey)

	defer func() {
		s.mu.Lock()
		delete(s.runningQueries, runKey)
		s.mu.Unlock()
		if execCtx.Err() != nil && s.Session != nil {
			// ponytail: Actively cancel runaway query on DB server to prevent CPU/IO exhaustion
			cancelCtx, cCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.Session.CancelQuery(cancelCtx, runKey)
			cCancel()
		}
	}()

	var execErr error
	var rowsAffected int64
	defer func() {
		if s.vault != nil && (inspection == nil || !inspection.RequiresChallenge || req.Confirmed) {
			dur := time.Since(start).Milliseconds()
			s.RecordAudit(trimmedSQL, "editor", dur, rowsAffected, execErr, start)
		}
	}()

	// Enable profiling if requested
	if req.Profile && s.Session != nil && s.DriverName == "mysql" {
		_, _ = s.Session.Exec(execCtx, "SET profiling = 1")
	}

	collectProfile := func() *driver.QueryProfile {
		if !req.Profile || s.Session == nil {
			return nil
		}
		if s.DriverName == "mysql" {
			defer func() { _, _ = s.Session.Exec(execCtx, "SET profiling = 0") }()
			pStream, pErr := s.Session.QueryStream(execCtx, "SHOW PROFILE", 100)
			if pErr != nil {
				return nil
			}
			defer pStream.Close()
			var stages []driver.ProfileStage
			var totalDuration float64
			for {
				chunk, err := pStream.NextChunk(100)
				for _, row := range chunk {
					if len(row) >= 2 {
						stg := fmt.Sprintf("%v", row[0])
						durSec, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[1]), 64)
						durUs := int64(durSec * 1e6)
						totalDuration += durSec
						stages = append(stages, driver.ProfileStage{
							Stage:      stg,
							DurationUs: durUs,
						})
					}
				}
				if err != nil || len(chunk) == 0 {
					break
				}
			}
			if totalDuration > 0 {
				for i := range stages {
					stages[i].Percentage = (float64(stages[i].DurationUs) / (totalDuration * 1e6)) * 100.0
				}
			}
			return &driver.QueryProfile{
				DurationMs: totalDuration * 1000.0,
				Stages:     stages,
			}
		}

		// SQLite profile fallback
		durUs := time.Since(start).Microseconds()
		return &driver.QueryProfile{
			DurationMs: float64(durUs) / 1000.0,
			Stages: []driver.ProfileStage{
				{
					Stage:      "Query Execution (SQLite embedded engine)",
					DurationUs: durUs,
					Percentage: 100.0,
				},
			},
		}
	}

	// Intercept explicit transaction statements to ensure connection pinning and prevent orphaned locks
	cleanUpper = strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
	isTxBegin := cleanUpper == "BEGIN" || cleanUpper == "START TRANSACTION" || cleanUpper == "BEGIN WORK" || cleanUpper == "BEGIN TRANSACTION" ||
		strings.HasPrefix(cleanUpper, "START TRANSACTION ") ||
		strings.HasPrefix(cleanUpper, "BEGIN TRANSACTION ") ||
		strings.HasPrefix(cleanUpper, "BEGIN WORK ") ||
		(strings.HasPrefix(cleanUpper, "BEGIN ") && (strings.Contains(cleanUpper, "DEFERRED") || strings.Contains(cleanUpper, "IMMEDIATE") || strings.Contains(cleanUpper, "EXCLUSIVE") || strings.Contains(cleanUpper, "READ ONLY") || strings.Contains(cleanUpper, "READ WRITE") || strings.Contains(cleanUpper, "ISOLATION LEVEL")))
	isTxCommit := cleanUpper == "COMMIT" || cleanUpper == "COMMIT WORK" || cleanUpper == "COMMIT TRANSACTION" || cleanUpper == "END" || cleanUpper == "END WORK" || cleanUpper == "END TRANSACTION"
	isTxRollback := cleanUpper == "ROLLBACK" || cleanUpper == "ROLLBACK WORK" || cleanUpper == "ROLLBACK TRANSACTION" || cleanUpper == "ABORT" || cleanUpper == "ABORT WORK" || cleanUpper == "ABORT TRANSACTION"

	if isTxBegin {
		s.mu.Lock()
		if s.activeTx != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("a transaction is already active in this session; please COMMIT or ROLLBACK first")
		}
		// ponytail: Use non-cancelled background context so transaction persists across subsequent HTTP requests
		tx, err := s.Session.BeginTx(context.Background())
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		s.activeTx = tx
		s.txStartedAt = time.Now()
		s.txLastActive = time.Now()
		s.mu.Unlock()

		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction started. All subsequent queries will run atomically on this dedicated connection.",
			ExecutionMs: time.Since(start).Milliseconds(),
			TxActive:    true,
		}, nil
	}

	if isTxCommit {
		s.mu.Lock()
		tx := s.activeTx
		s.activeTx = nil
		s.mu.Unlock()

		if tx == nil {
			_, err := s.Session.Exec(execCtx, trimmedSQL)
			if err != nil {
				return nil, err
			}
			return &ExecuteResponse{
				Type:        "exec",
				Message:     "COMMIT executed.",
				ExecutionMs: time.Since(start).Milliseconds(),
				TxActive:    false,
			}, nil
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction committed successfully.",
			ExecutionMs: time.Since(start).Milliseconds(),
			TxActive:    false,
		}, nil
	}

	if isTxRollback {
		s.mu.Lock()
		tx := s.activeTx
		s.activeTx = nil
		s.mu.Unlock()

		if tx == nil {
			_, err := s.Session.Exec(execCtx, trimmedSQL)
			if err != nil {
				return nil, err
			}
			return &ExecuteResponse{
				Type:        "exec",
				Message:     "ROLLBACK executed.",
				ExecutionMs: time.Since(start).Milliseconds(),
				TxActive:    false,
			}, nil
		}

		if err := tx.Rollback(); err != nil {
			return nil, fmt.Errorf("failed to rollback transaction: %w", err)
		}
		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction rolled back successfully.",
			ExecutionMs: time.Since(start).Milliseconds(),
			TxActive:    false,
		}, nil
	}

	// 5. Route to Exec vs QueryStream
	// If transaction is active, execute strictly on pinned transaction connection
	s.mu.Lock()
	currentTx := s.activeTx
	if currentTx != nil {
		s.txLastActive = time.Now()
	}
	s.mu.Unlock()

	if currentTx != nil {
		if !isSelect {
			execRes, err := currentTx.Exec(execCtx, trimmedSQL)
			if err != nil {
				execErr = err
				return nil, err
			}
			rowsAffected = execRes.RowsAffected
			msg := execRes.Message + " (in transaction)"
			if isMutation && inspection.EstimatedRows != nil && *inspection.EstimatedRows != rowsAffected {
				msg = fmt.Sprintf("%s (Estimated: %d rows, Actual: %d rows)", msg, *inspection.EstimatedRows, rowsAffected)
			}
			return &ExecuteResponse{
				Type:               "exec",
				RowsAffected:       execRes.RowsAffected,
				LastInsertID:       execRes.LastInsertID,
				ExecutionMs:        execRes.ExecutionMs,
				Message:            msg,
				BackupFile:         backupFile,
				BackupDataIncluded: backupDataIncluded,
				BackupWarning:      backupWarning,
				Profile:            collectProfile(),
				Inspection:         inspection,
			}, nil
		}

		stream, err := currentTx.QueryStream(execCtx, trimmedSQL, 1000)
		if err != nil {
			execErr = err
			return nil, err
		}
		defer stream.Close()

		cols := stream.Columns()
		var allRows [][]any
		for len(allRows) < 1000 {
			chunk, err := stream.NextChunk(200)
			if len(chunk) > 0 {
				allRows = append(allRows, chunk...)
			}
			if err != nil {
				if err != io.EOF {
					execErr = err
					return nil, err
				}
				break
			}
		}

		rowsAffected = int64(len(allRows))
		return &ExecuteResponse{
			Type:        "query",
			Columns:     cols,
			Rows:        allRows,
			RowCount:    len(allRows),
			ExecutionMs: time.Since(start).Milliseconds(),
			Profile:     collectProfile(),
			TxActive:    currentTx != nil,
		}, nil
	}

	if !isSelect {
		// Mutation execution
		var execRes *driver.ExecResult
		var err error
		if inspection.Operation == "ALTER" {
			lockTimeout := req.LockTimeoutSec
			if lockTimeout <= 0 {
				lockTimeout = 3 // default fail-fast timeout (3s)
			}
			execRes, err = s.Session.ExecSafeDDL(execCtx, trimmedSQL, lockTimeout)
		} else {
			execRes, err = s.Session.Exec(execCtx, trimmedSQL)
		}
		if err != nil {
			execErr = err
			return nil, err
		}
		rowsAffected = execRes.RowsAffected
		msg := execRes.Message
		if isMutation && inspection.EstimatedRows != nil && *inspection.EstimatedRows != rowsAffected {
			msg = fmt.Sprintf("%s (Estimated: %d rows, Actual: %d rows)", msg, *inspection.EstimatedRows, rowsAffected)
		}
		return &ExecuteResponse{
			Type:               "exec",
			RowsAffected:       execRes.RowsAffected,
			LastInsertID:       execRes.LastInsertID,
			ExecutionMs:        execRes.ExecutionMs,
			Message:            msg,
			BackupFile:         backupFile,
			BackupDataIncluded: backupDataIncluded,
			BackupWarning:      backupWarning,
			Profile:            collectProfile(),
			TxActive:           currentTx != nil,
			Inspection:         inspection,
		}, nil
	}

	// Select stream execution with implicit safe limit (1,000 rows)
	stream, err := s.Session.QueryStream(execCtx, trimmedSQL, 1000)
	if err != nil {
		execErr = err
		return nil, err
	}
	defer stream.Close()

	cols := stream.Columns()
	var allRows [][]any

	for len(allRows) < 1000 {
		chunk, err := stream.NextChunk(200)
		if len(chunk) > 0 {
			allRows = append(allRows, chunk...)
		}
		if err != nil {
			if err != io.EOF {
				execErr = err
				return nil, err
			}
			break
		}
	}

	rowsAffected = int64(len(allRows))
	return &ExecuteResponse{
		Type:        "query",
		Columns:     cols,
		Rows:        allRows,
		RowCount:    len(allRows),
		ExecutionMs: time.Since(start).Milliseconds(),
		Profile:     collectProfile(),
		TxActive:    currentTx != nil,
	}, nil
}

// ExecuteStream streams query results line-by-line as NDJSON (ADR 0058).
// It delivers an immediate micro-chunk (20 rows) for sub-50ms TTFR, followed by batched chunks.
func (s *ActiveSession) ExecuteStream(ctx context.Context, req ExecuteRequest, w io.Writer, flusher http.Flusher) (*ExecuteResponse, error) {
	s.mu.Lock()
	s.LastActive = time.Now()
	s.mu.Unlock()

	trimmedSQL := strings.TrimSpace(req.SQL)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

	cleanSQL := driver.StripLeadingComments(trimmedSQL)
	cleanUpper := strings.TrimRight(strings.ToUpper(cleanSQL), "; \t\r\n")
	isSelect := driver.IsQueryStatement(trimmedSQL)

	if strings.HasPrefix(cleanUpper, "USE ") {
		dbTarget := strings.TrimSpace(cleanSQL[4:])
		dbTarget = strings.Trim(strings.TrimRight(dbTarget, "; \t\r\n"), "`'\"")
		if dbTarget != "" {
			_ = s.UseDatabase(ctx, dbTarget)
		}
	}

	if req.Database != "" && s.Session != nil {
		if err := s.Session.SelectDatabase(ctx, req.Database); err != nil {
			return nil, err
		}
	}

	// 1. Safety Guard Inspection with Scoped Database Context
	targetDB := req.Database
	if targetDB == "" && s.Schema() != nil {
		targetDB = s.Schema().DatabaseName
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}

	// 2. Read-Only Enforcement
	if s.IsReadOnly() && (!isSelect || !allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
	}

	// 3. Pre-Execution Row Estimation & Tiered Affirmation Gate (ADR 0057)
	isDev := strings.EqualFold(s.Environment, "Development")
	isMutation := inspection.Operation == "DELETE" || inspection.Operation == "UPDATE" || len(inspection.StatementEstimates) > 0
	canBypass := isDev && req.BypassMutationConfirm && !inspection.RequiresChallenge

	if isMutation && !canBypass {
		// Re-derived on confirmed requests too, so a client cannot skip the typed challenge by sending confirmed=true first.
		s.estimateAffectedRows(ctx, inspection)

		anyMissingWhere := false
		anyOverThreshold := false
		if len(inspection.StatementEstimates) > 0 {
			for _, est := range inspection.StatementEstimates {
				if !est.HasWhere {
					anyMissingWhere = true
				}
				if est.EstimatedRows != nil && *est.EstimatedRows > 100 {
					anyOverThreshold = true
				}
			}
		} else {
			if !inspection.HasWhere {
				anyMissingWhere = true
			}
			if inspection.EstimatedRows != nil && *inspection.EstimatedRows > 100 {
				anyOverThreshold = true
			}
		}

		if isDev {
			if len(inspection.StatementEstimates) == 0 && inspection.HasWhere && inspection.EstimatedRows != nil && *inspection.EstimatedRows == 1 {
				inspection.RequiresAffirmation = false
				inspection.RequiresChallenge = false
			} else {
				if anyOverThreshold || anyMissingWhere {
					inspection.RequiresChallenge = true
				} else {
					inspection.RequiresAffirmation = true
				}
			}
		} else {
			if anyOverThreshold || anyMissingWhere {
				inspection.RequiresChallenge = true
			} else {
				inspection.RequiresAffirmation = true
			}
		}
	}

	requiresGate := inspection.RequiresChallenge || (isMutation && !canBypass && inspection.RequiresAffirmation) || (!isDev && inspection.RequiresAffirmation)
	if requiresGate && !req.Confirmed {
		if inspection.TargetTable != "" && s.Session != nil {
			if locks, err := s.Session.InspectTableLocks(ctx, req.Database, inspection.TargetTable); err == nil {
				inspection.TableLocks = locks
			}
			schema := s.Schema()
			if schema == nil || (req.Database != "" && schema.DatabaseName != req.Database) {
				if sSc, sErr := s.Session.InspectSchema(ctx, req.Database); sErr == nil {
					schema = sSc
					s.SetSchema(sSc)
				}
			}
			if schema != nil {
				inspection.Relationships = driver.InspectTableRelationships(schema, inspection.TargetTable)
			}
		}
		return &ExecuteResponse{
			Blocked:    true,
			Inspection: inspection,
		}, nil
	}

	// Challenge typed table name validation
	if inspection.RequiresChallenge && req.Confirmed && inspection.TargetTable != "" {
		expected := inspection.TargetTable
		typed := strings.TrimSpace(req.ChallengeInput)
		if typed == "" {
			// Confirmed without a typed challenge (the requirement escalated since the preview, e.g. the
			// row estimate grew past the threshold): ask again instead of failing the request.
			return &ExecuteResponse{Blocked: true, Inspection: inspection}, nil
		}
		if !strings.EqualFold(typed, expected) && !strings.EqualFold(typed, inspection.TargetDatabase) {
			return nil, fmt.Errorf("Challenge confirmation failed: expected '%s', got '%s'", expected, req.ChallengeInput)
		}
	}

	// Non-SELECT queries (mutations/DDL/transactions) and mixed batches such as "SELECT 1; DROP TABLE t"
	// run synchronously through Execute (which owns the pre-DDL backup) and output a single NDJSON line.
	if !isSelect || !allQueries {
		resp, execErr := s.Execute(ctx, req)
		if execErr != nil {
			return nil, execErr
		}
		if resp != nil && resp.Blocked {
			return resp, nil
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(resp)
		if flusher != nil {
			flusher.Flush()
		}
		return nil, nil
	}

	start := time.Now()
	queryID := req.QueryID
	if queryID == "" {
		queryID = "active"
	}

	totalRows := 0
	var streamErr error
	defer func() {
		dur := time.Since(start).Milliseconds()
		if s.vault != nil {
			s.RecordAudit(trimmedSQL, "editor_stream", dur, int64(totalRows), streamErr, start)
		}
	}()

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Each execution gets a unique run key: clients reuse query IDs (the editor always sends
	// "active"), and a shared key let the first query to finish unregister a still-running one.
	runKey := s.registerQuery(queryID, cancel)
	execCtx = driver.WithQueryID(execCtx, runKey)

	defer func() {
		s.mu.Lock()
		delete(s.runningQueries, runKey)
		s.mu.Unlock()
		if execCtx.Err() != nil && s.Session != nil {
			cancelCtx, cCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.Session.CancelQuery(cancelCtx, runKey)
			cCancel()
		}
	}()

	limit := req.Limit
	if limit <= 0 {
		limit = 5000
	}

	if req.Profile && s.Session != nil && s.DriverName == "mysql" {
		_, _ = s.Session.Exec(execCtx, "SET profiling = 1")
	}

	s.mu.Lock()
	currentTx := s.activeTx
	if currentTx != nil {
		s.txLastActive = time.Now()
	}
	s.mu.Unlock()

	// Fetch one row past the limit so has_more is exact (a result of exactly `limit` rows is complete).
	var stream driver.RowStream
	if currentTx != nil {
		stream, streamErr = currentTx.QueryStream(execCtx, trimmedSQL, limit+1)
	} else {
		if s.Session == nil {
			return nil, errors.New("no active database session")
		}
		stream, streamErr = s.Session.QueryStream(execCtx, trimmedSQL, limit+1)
	}
	if streamErr != nil {
		return nil, streamErr
	}
	defer stream.Close()

	enc := json.NewEncoder(w)

	// 1. Meta Line (columns, target table, database, tx status)
	meta := map[string]any{
		"type":         "meta",
		"columns":      stream.Columns(),
		"tx_active":    currentTx != nil,
		"target_table": inspection.TargetTable,
		"database":     targetDB,
	}
	if err := enc.Encode(meta); err != nil {
		streamErr = err
		return nil, err
	}
	if flusher != nil {
		flusher.Flush()
	}

	// 2. Micro-Chunk: first 20 rows flushed immediately (<15ms)
	firstChunk, err := stream.NextChunk(min(20, limit))
	if len(firstChunk) > 0 {
		totalRows += len(firstChunk)
		if encErr := enc.Encode(map[string]any{
			"type": "chunk",
			"rows": firstChunk,
		}); encErr != nil {
			streamErr = encErr
			return nil, encErr
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err != nil && err != io.EOF {
		streamErr = err
		_ = enc.Encode(map[string]any{
			"type":  "error",
			"error": err.Error(),
		})
		if flusher != nil {
			flusher.Flush()
		}
		return nil, err
	}

	// 3. Subsequent chunks: batched at 200 rows
	for totalRows < limit && err == nil {
		batchSize := 200
		if totalRows+batchSize > limit {
			batchSize = limit - totalRows
		}
		var chunk [][]any
		chunk, err = stream.NextChunk(batchSize)
		if len(chunk) > 0 {
			totalRows += len(chunk)
			if encErr := enc.Encode(map[string]any{
				"type": "chunk",
				"rows": chunk,
			}); encErr != nil {
				streamErr = encErr
				return nil, encErr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}

	if err != nil && err != io.EOF {
		streamErr = err
		_ = enc.Encode(map[string]any{
			"type":  "error",
			"error": err.Error(),
		})
		if flusher != nil {
			flusher.Flush()
		}
		return nil, err
	}

	hasMore := false
	if err == nil && totalRows >= limit {
		extra, _ := stream.NextChunk(1)
		hasMore = len(extra) > 0
	}

	// 4. Final Line: done
	dur := time.Since(start).Milliseconds()
	var prof *driver.QueryProfile
	if req.Profile && s.Session != nil && s.DriverName == "mysql" {
		defer func() { _, _ = s.Session.Exec(execCtx, "SET profiling = 0") }()
		pStream, pErr := s.Session.QueryStream(execCtx, "SHOW PROFILE", 100)
		if pErr == nil && pStream != nil {
			defer pStream.Close()
			var stages []driver.ProfileStage
			var totalDuration float64
			for {
				pChunk, pChunkErr := pStream.NextChunk(100)
				for _, pRow := range pChunk {
					if len(pRow) >= 2 {
						stg := fmt.Sprintf("%v", pRow[0])
						durSec, _ := strconv.ParseFloat(fmt.Sprintf("%v", pRow[1]), 64)
						durUs := int64(durSec * 1e6)
						totalDuration += durSec
						stages = append(stages, driver.ProfileStage{
							Stage:      stg,
							DurationUs: durUs,
						})
					}
				}
				if pChunkErr != nil || len(pChunk) == 0 {
					break
				}
			}
			if totalDuration > 0 {
				for i := range stages {
					stages[i].Percentage = (float64(stages[i].DurationUs) / (totalDuration * 1e6)) * 100.0
				}
			}
			if len(stages) > 0 {
				prof = &driver.QueryProfile{
					DurationMs: totalDuration * 1000.0,
					Stages:     stages,
				}
			}
		}
	}

	donePayload := map[string]any{
		"type":         "done",
		"row_count":    totalRows,
		"execution_ms": dur,
		"has_more":     hasMore,
		"tx_active":    currentTx != nil,
	}
	if prof != nil {
		donePayload["profile"] = prof
	}
	if err := enc.Encode(donePayload); err != nil {
		streamErr = err
		return nil, err
	}
	if flusher != nil {
		flusher.Flush()
	}

	return nil, nil
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
// both client-side (context) and server-side (driver KILL / pg_cancel_backend per run key).
func (s *ActiveSession) CancelQuery(ctx context.Context, queryID string) error {
	all := queryID == "" || queryID == "active"
	s.mu.Lock()
	var keys []string
	for k, c := range s.runningQueries {
		if all || k == queryID || strings.HasPrefix(k, queryID+"#") {
			keys = append(keys, k)
			if c != nil {
				c()
			}
		}
	}
	sess := s.Session
	s.mu.Unlock()

	if sess == nil {
		return nil
	}
	if len(keys) == 0 {
		return sess.CancelQuery(ctx, queryID)
	}
	var firstErr error
	for _, k := range keys {
		if err := sess.CancelQuery(ctx, k); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// GetProcesslist retrieves active threads and lock wait graph from the active driver.
func (s *ActiveSession) GetProcesslist(ctx context.Context) (*driver.ProcesslistResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetProcesslist(ctx)
}

// InspectClusterHealth retrieves cluster topology, node synchronization, and bottleneck diagnostics.
func (s *ActiveSession) InspectClusterHealth(ctx context.Context) (*driver.ClusterHealthReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.InspectClusterHealth(ctx)
}

// InspectServerVariables retrieves server configuration parameters and tuning heuristics.
func (s *ActiveSession) InspectServerVariables(ctx context.Context) (*driver.ServerVariablesReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.InspectServerVariables(ctx)
}

// KillProcess terminates an active query or client connection out-of-band.
func (s *ActiveSession) KillProcess(ctx context.Context, threadID int64, killConnection bool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Session == nil {
		return driver.ErrSessionClosed
	}
	return s.Session.KillProcess(ctx, threadID, killConnection)
}

// Stream executes a non-destructive query, returning a chunked RowStream.
// Destructive queries are rejected to protect WebSocket callers from uninspected DDL/DML.
func (s *ActiveSession) Stream(ctx context.Context, sql string, maxRows int) (driver.RowStream, error) {
	s.mu.Lock()
	s.LastActive = time.Now()
	s.mu.Unlock()

	trimmedSQL := strings.TrimSpace(sql)
	if trimmedSQL == "" {
		return nil, errors.New("sql query is required")
	}

	isSelect := driver.IsQueryStatement(trimmedSQL)
	targetDB := ""
	if s.Schema() != nil {
		targetDB = s.Schema().DatabaseName
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}
	if s.IsReadOnly() && (!isSelect || !allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
	}
	if inspection.IsDestructive || inspection.RequiresChallenge {
		return nil, fmt.Errorf("destructive query cannot be streamed: %s; use query editor with confirmation", inspection.Reason)
	}
	if !isSelect || !allQueries {
		return nil, errors.New("only query statements (SELECT, SHOW, DESCRIBE, EXPLAIN) can be streamed")
	}

	if maxRows <= 0 {
		maxRows = 100000
	}

	return s.Session.QueryStream(ctx, trimmedSQL, maxRows)
}

// GetDatabaseDiagnostics returns physical storage and metadata for all databases.
func (s *ActiveSession) GetDatabaseDiagnostics(ctx context.Context) ([]driver.DatabaseDiagnostics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetDatabaseDiagnostics(ctx)
}

// GetTableDiagnostics returns physical storage and overhead metrics.
func (s *ActiveSession) GetTableDiagnostics(ctx context.Context, database, table string) ([]driver.TableDiagnostics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetTableDiagnostics(ctx, database, table)
}

// ExecuteMaintenance runs maintenance actions (OPTIMIZE, ANALYZE, CHECK, VACUUM).
func (s *ActiveSession) ExecuteMaintenance(ctx context.Context, database string, tables []string, action string) ([]driver.MaintenanceResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.ExecuteMaintenance(ctx, database, tables, action)
}

// GetRedundantIndexes detects duplicate, shadowed, and redundant primary key indexes.
func (s *ActiveSession) GetRedundantIndexes(ctx context.Context, database, table string) ([]driver.RedundantIndex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetRedundantIndexes(ctx, database, table)
}

// PeekForeignKeyRow queries parent table for a single row matching a foreign key value.
func (s *ActiveSession) PeekForeignKeyRow(ctx context.Context, database, table, column string, value any) (*driver.PeekResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.PeekForeignKeyRow(ctx, database, table, column, value)
}

// GetUserList lists database users.
func (s *ActiveSession) GetUserList(ctx context.Context) ([]driver.DatabaseUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetUserList(ctx)
}

// GetUserGrants returns active grants for a user account.
func (s *ActiveSession) GetUserGrants(ctx context.Context, user, host string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetUserGrants(ctx, user, host)
}

// SaveUser creates or updates a user account.
func (s *ActiveSession) SaveUser(ctx context.Context, user, host, password string, isNew bool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return driver.ErrSessionClosed
	}
	return s.Session.SaveUser(ctx, user, host, password, isNew)
}

// DropUser removes a user account.
func (s *ActiveSession) DropUser(ctx context.Context, user, host string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return driver.ErrSessionClosed
	}
	return s.Session.DropUser(ctx, user, host)
}

// ApplyUserGrants grants privileges to a user.
func (s *ActiveSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return driver.ErrSessionClosed
	}
	return s.Session.ApplyUserGrants(ctx, req)
}

// SampleFlightMetrics captures performance telemetry and correlated queries from the driver session.
func (s *ActiveSession) SampleFlightMetrics(ctx context.Context) (*driver.FlightMetricSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.SampleFlightMetrics(ctx)
}

// InspectDeadlock delegates deadlock analysis to the driver session.
func (s *ActiveSession) InspectDeadlock(ctx context.Context) (*driver.DeadlockReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.InspectDeadlock(ctx)
}

// GetStatementDigest delegates statement digest queries to the driver session.
func (s *ActiveSession) GetStatementDigest(ctx context.Context, limit int) ([]driver.StatementDigestEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetStatementDigest(ctx, limit)
}

// ResetStatementDigest delegates statement digest statistics clearing to the driver session.
func (s *ActiveSession) ResetStatementDigest(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return driver.ErrSessionClosed
	}
	return s.Session.ResetStatementDigest(ctx)
}

// GetRoutineDDL retrieves the raw CREATE statement for a Stored Procedure or Function.
func (s *ActiveSession) GetRoutineDDL(ctx context.Context, routineType, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return "", driver.ErrSessionClosed
	}
	return s.Session.GetRoutineDDL(ctx, routineType, name)
}

// GetEventDDL retrieves the raw CREATE statement for a Scheduled Event.
func (s *ActiveSession) GetEventDDL(ctx context.Context, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return "", driver.ErrSessionClosed
	}
	return s.Session.GetEventDDL(ctx, name)
}

// ToggleEventStatus toggles the status of a Scheduled Event.
func (s *ActiveSession) ToggleEventStatus(ctx context.Context, name string, enable bool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return driver.ErrSessionClosed
	}
	return s.Session.ToggleEventStatus(ctx, name, enable)
}

// GetTablePartitions retrieves partition telemetry for a table.
func (s *ActiveSession) GetTablePartitions(ctx context.Context, database, table string) ([]driver.PartitionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Session == nil {
		return nil, driver.ErrSessionClosed
	}
	return s.Session.GetTablePartitions(ctx, database, table)
}

// inspectAllStatements runs the Safety Guard on every statement of a (possibly multi-statement) input,
// because drivers such as SQLite execute all of them in one Exec. It returns the inspection that must
// gate execution and whether every statement is a read-only query. More than one statement requiring a
// challenge is rejected, since one typed confirmation and one pre-DDL backup can only cover one target.
func inspectAllStatements(sql, environment, driverName, scopedDB string) (*safety.InspectionResult, bool, error) {
	stmts := splitSQLStatements(strings.NewReader(sql), driverName)
	if len(stmts) <= 1 {
		return safety.InspectWithScope(sql, environment, scopedDB), driver.IsQueryStatement(sql), nil
	}
	var gate, firstDestructive *safety.InspectionResult
	var stmtEstimates []safety.StatementEstimate
	allQueries := true
	for idx, stmt := range stmts {
		if !driver.IsQueryStatement(stmt) {
			allQueries = false
		}
		insp := safety.InspectWithScope(stmt, environment, scopedDB)
		if insp.Operation == "DELETE" || insp.Operation == "UPDATE" {
			stmtEstimates = append(stmtEstimates, safety.StatementEstimate{
				StatementIndex:    idx + 1,
				SQL:               stmt,
				Operation:         insp.Operation,
				TargetTable:       insp.TargetTable,
				TargetDatabase:    insp.TargetDatabase,
				HasWhere:          insp.HasWhere,
				CountQuery:        insp.CountQuery,
				RequiresChallenge: insp.RequiresChallenge,
				Reason:            insp.Reason,
			})
		}
		if insp.RequiresChallenge {
			if gate != nil && gate.RequiresChallenge && (gate.Operation == "DROP" || insp.Operation == "DROP" || gate.Operation == "TRUNCATE" || insp.Operation == "TRUNCATE") {
				return nil, false, errors.New("multiple destructive statements require confirmation: run them one at a time")
			}
			gate = insp
		} else if insp.IsDestructive && firstDestructive == nil {
			firstDestructive = insp
		} else if insp.RequiresAffirmation && gate == nil {
			gate = insp
		}
	}
	if gate == nil {
		gate = firstDestructive
	}
	if gate == nil && len(stmtEstimates) > 0 {
		gate = &safety.InspectionResult{
			RiskLevel:           safety.RiskWarning,
			IsDestructive:       true,
			Operation:           "MUTATION_BATCH",
			RequiresAffirmation: true,
			Reason:              fmt.Sprintf("Batch contains %d mutation statements.", len(stmtEstimates)),
		}
	}
	if gate == nil {
		gate = &safety.InspectionResult{RiskLevel: safety.RiskNone}
	}
	if len(stmtEstimates) > 0 {
		gate.StatementEstimates = stmtEstimates
	}
	return gate, allQueries, nil
}

func (s *ActiveSession) estimateAffectedRows(ctx context.Context, insp *safety.InspectionResult) {
	if insp == nil || s.Session == nil {
		return
	}
	if len(insp.StatementEstimates) > 0 {
		var total int64
		allExact := true
		for i := range insp.StatementEstimates {
			est := &insp.StatementEstimates[i]
			if est.CountQuery != "" {
				count, exact := s.runSingleCountEstimate(ctx, est.CountQuery)
				if count != nil {
					est.EstimatedRows = count
					est.EstimatedIsExact = exact
					total += *count
					if !exact {
						allExact = false
					}
				}
			}
		}
		insp.EstimatedRows = &total
		insp.EstimatedIsExact = allExact
		return
	}

	if insp.CountQuery != "" {
		count, exact := s.runSingleCountEstimate(ctx, insp.CountQuery)
		if count != nil {
			insp.EstimatedRows = count
			insp.EstimatedIsExact = exact
		}
	}
}

func (s *ActiveSession) runSingleCountEstimate(ctx context.Context, countSQL string) (*int64, bool) {
	if s.Session == nil || countSQL == "" {
		return nil, false
	}
	countCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// No SelectDatabase here: it would persist the session's active DB before the user confirms.
	// Qualified statements yield qualified count queries; unqualified ones already run in req.Database.

	rows, err := s.Session.QueryRaw(countCtx, countSQL)
	if err == nil && len(rows) > 0 && len(rows[0]) > 0 {
		if cnt, ok := parseCountValue(rows[0][0]); ok {
			return &cnt, true
		}
	}

	// Fallback to EXPLAIN
	plan, expErr := s.Session.Explain(countCtx, countSQL)
	if expErr == nil && plan != nil && len(plan.Rows) > 0 {
		if plan.Rows[0].Rows > 0 {
			est := plan.Rows[0].Rows
			return &est, false
		}
	}

	return nil, false
}

func parseCountValue(v any) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case int32:
		return int64(val), true
	case uint64:
		return int64(val), true
	case float64:
		return int64(val), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		return n, err == nil
	case []byte:
		n, err := strconv.ParseInt(strings.TrimSpace(string(val)), 10, 64)
		return n, err == nil
	}
	return 0, false
}
