package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
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
	SQL            string `json:"sql"`
	Confirmed      bool   `json:"confirmed"`
	ChallengeInput string `json:"challenge_input"`
	QueryID        string `json:"query_id"`
	Database       string `json:"database"`
	Profile        bool   `json:"profile"`
	LockTimeoutSec int    `json:"lock_timeout_sec,omitempty"` // Fail-fast session lock timeout (default 3s for DDL)
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
	BackupDataIncluded bool                      `json:"backup_data_included,omitempty"`
	Columns            []driver.ColumnDefinition `json:"columns,omitempty"`
	Rows               [][]any                   `json:"rows,omitempty"`
	RowCount           int                       `json:"row_count,omitempty"`
	ExecutionMs        int64                     `json:"execution_ms"`
	Profile            *driver.QueryProfile      `json:"profile,omitempty"`
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
	sess := s.Session
	s.Session = nil
	s.mu.Unlock()

	if sess != nil {
		return sess.Close()
	}
	return nil
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check existing session
	if sess, ok := sm.sessions[connectionID]; ok {
		sess.mu.Lock()
		sess.LastActive = time.Now()
		sess.mu.Unlock()
		return sess, nil
	}

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

	sm.sessions[connectionID] = active
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
			return sm.ConnectSession(context.Background(), connectionID)
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
	if targetDB == "" && s.CachedSchema != nil {
		targetDB = s.CachedSchema.DatabaseName
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}

	// 2. Read-Only Enforcement: blocks all destructive operations or any mutation when locked
	if s.ReadOnly && (!isSelect || !allQueries || inspection.IsDestructive) {
		return nil, ErrReadOnlySession
	}

	// 3. Challenge Enforcement (ADR 0055)
	requiresGate := inspection.RequiresChallenge
	if requiresGate && !req.Confirmed {
		if inspection.TargetTable != "" && s.Session != nil {
			if locks, err := s.Session.InspectTableLocks(ctx, req.Database, inspection.TargetTable); err == nil {
				inspection.TableLocks = locks
			}
			schema := s.CachedSchema
			if schema == nil || (req.Database != "" && schema.DatabaseName != req.Database) {
				if sSc, sErr := s.Session.InspectSchema(ctx, req.Database); sErr == nil {
					schema = sSc
					s.CachedSchema = sSc
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
		if !strings.EqualFold(typed, expected) && !strings.EqualFold(typed, inspection.TargetDatabase) {
			return nil, fmt.Errorf("Challenge confirmation failed: expected '%s', got '%s'", expected, req.ChallengeInput)
		}
	}

	// 4. Pre-DDL Automated Backup
	var backupFile string
	var backupDataIncluded bool
	if (inspection.Operation == "DROP" || inspection.Operation == "TRUNCATE") && !strings.EqualFold(inspection.TargetType, "DATABASE") && !strings.EqualFold(inspection.TargetType, "VIEW") {
		if inspection.TargetTable != "" && s.backupDir != "" && s.Session != nil {
			var err error
			backupFile, backupDataIncluded, err = safety.BackupTable(ctx, s.Session, s.backupDir, inspection.TargetTable, s.DriverName)
			if err != nil {
				// Log backup warning, proceed with explicit challenge confirmation
			}
		}
	}

	start := time.Now()

	queryID := req.QueryID
	if queryID == "" {
		queryID = "active"
	}

	execCtx, cancel := context.WithCancel(ctx)
	execCtx = driver.WithQueryID(execCtx, queryID)
	defer cancel()

	s.mu.Lock()
	if s.runningQueries == nil {
		s.runningQueries = make(map[string]context.CancelFunc)
	}
	s.runningQueries[queryID] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.runningQueries, queryID)
		s.mu.Unlock()
		if execCtx.Err() != nil && s.Session != nil {
			// ponytail: Actively cancel runaway query on DB server to prevent CPU/IO exhaustion
			cancelCtx, cCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.Session.CancelQuery(cancelCtx, queryID)
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
	isTxBegin := cleanUpper == "BEGIN" || cleanUpper == "START TRANSACTION" || cleanUpper == "BEGIN WORK" || cleanUpper == "BEGIN TRANSACTION"
	isTxCommit := cleanUpper == "COMMIT" || cleanUpper == "COMMIT WORK" || cleanUpper == "COMMIT TRANSACTION"
	isTxRollback := cleanUpper == "ROLLBACK" || cleanUpper == "ROLLBACK WORK" || cleanUpper == "ROLLBACK TRANSACTION"

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
			}, nil
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction committed successfully.",
			ExecutionMs: time.Since(start).Milliseconds(),
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
			}, nil
		}

		if err := tx.Rollback(); err != nil {
			return nil, fmt.Errorf("failed to rollback transaction: %w", err)
		}
		return &ExecuteResponse{
			Type:        "exec",
			Message:     "Transaction rolled back successfully.",
			ExecutionMs: time.Since(start).Milliseconds(),
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
			return &ExecuteResponse{
				Type:               "exec",
				RowsAffected:       execRes.RowsAffected,
				LastInsertID:       execRes.LastInsertID,
				ExecutionMs:        execRes.ExecutionMs,
				Message:            execRes.Message + " (in transaction)",
				BackupFile:         backupFile,
				BackupDataIncluded: backupDataIncluded,
				Profile:            collectProfile(),
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
		return &ExecuteResponse{
			Type:               "exec",
			RowsAffected:       execRes.RowsAffected,
			LastInsertID:       execRes.LastInsertID,
			ExecutionMs:        execRes.ExecutionMs,
			Message:            execRes.Message,
			BackupFile:         backupFile,
			BackupDataIncluded: backupDataIncluded,
			Profile:            collectProfile(),
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
	}, nil
}

// CancelQuery attempts to interrupt a running query by queryID.
func (s *ActiveSession) CancelQuery(ctx context.Context, queryID string) error {
	s.mu.Lock()
	cancel, ok := s.runningQueries[queryID]
	if !ok && (queryID == "active" || queryID == "") {
		for _, c := range s.runningQueries {
			if c != nil {
				c()
			}
		}
	}
	s.mu.Unlock()

	if ok && cancel != nil {
		cancel()
	}

	if s.Session != nil {
		return s.Session.CancelQuery(ctx, queryID)
	}
	return nil
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
	if s.CachedSchema != nil {
		targetDB = s.CachedSchema.DatabaseName
	}
	inspection, allQueries, err := inspectAllStatements(trimmedSQL, s.Environment, s.DriverName, targetDB)
	if err != nil {
		return nil, err
	}
	if s.ReadOnly && (!isSelect || !allQueries || inspection.IsDestructive) {
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
	allQueries := true
	for _, stmt := range stmts {
		if !driver.IsQueryStatement(stmt) {
			allQueries = false
		}
		insp := safety.InspectWithScope(stmt, environment, scopedDB)
		if insp.RequiresChallenge {
			if gate != nil && gate.RequiresChallenge {
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
	if gate == nil {
		gate = &safety.InspectionResult{RiskLevel: safety.RiskNone}
	}
	return gate, allQueries, nil
}
