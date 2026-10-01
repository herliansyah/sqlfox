package server

import (
	"context"
	"errors"
	"fmt"
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
}

// ActiveSession holds an active database connection session and its cached metadata.
// It acts as a deep module providing the Safe Execution Pipeline across all transports.
type ActiveSession struct {
	ID           string                 `json:"id"`
	ConnectionID string                 `json:"connection_id"`
	Name         string                 `json:"name"`
	Environment  string                 `json:"environment"`
	DriverName   string                 `json:"driver_name"`
	ReadOnly     bool                   `json:"read_only"`
	Session      driver.Session         `json:"-"`
	CachedSchema *driver.SchemaMetadata `json:"schema,omitempty"`
	ServerInfo     *driver.ServerInfo     `json:"server_info,omitempty"`
	LastActive     time.Time              `json:"last_active"`
	guard          *safety.SafetyGuard
	backupMgr      *safety.BackupManager
	vault          *vault.Vault
	runningQueries map[string]context.CancelFunc
	mu             sync.RWMutex
}

// SessionManager manages connected database sessions.
type SessionManager struct {
	vault       *vault.Vault
	guard       *safety.SafetyGuard
	backupMgr   *safety.BackupManager
	sessions    map[string]*ActiveSession
	mu          sync.RWMutex
}

func NewSessionManager(v *vault.Vault, guard *safety.SafetyGuard, backupMgr *safety.BackupManager) *SessionManager {
	return &SessionManager{
		vault:     v,
		guard:     guard,
		backupMgr: backupMgr,
		sessions:  make(map[string]*ActiveSession),
	}
}

// ConnectSession creates or activates a session for the given connection ID.
func (sm *SessionManager) ConnectSession(ctx context.Context, connectionID string) (*ActiveSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check existing session
	if sess, ok := sm.sessions[connectionID]; ok {
		sess.LastActive = time.Now()
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
		UseSSH:       conn.UseSSH,
		SSHHost:      conn.SSHHost,
		SSHPort:      conn.SSHPort,
		SSHUser:      conn.SSHUser,
		SSHPassword:  conn.SSHPassword,
		SSHKey:       conn.SSHKey,
	}

	dbSession, err := driver.Connect(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	info, _ := dbSession.ServerInfo(ctx)
	schema, _ := dbSession.InspectSchema(ctx, conn.DatabaseName)

	active := &ActiveSession{
		ID:           connectionID,
		ConnectionID: connectionID,
		Name:         conn.Name,
		Environment:  conn.Environment,
		DriverName:   conn.Driver,
		ReadOnly:     conn.Environment == "Production", // Production defaults to Read-Only
		Session:      dbSession,
		CachedSchema: schema,
		ServerInfo:   info,
		LastActive:     time.Now(),
		guard:          sm.guard,
		backupMgr:      sm.backupMgr,
		vault:          sm.vault,
		runningQueries: make(map[string]context.CancelFunc),
	}

	sm.sessions[connectionID] = active
	return active, nil
}

func (sm *SessionManager) GetSession(connectionID string) (*ActiveSession, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sess, ok := sm.sessions[connectionID]
	if !ok {
		return nil, fmt.Errorf("session not found for connection %s; please connect first", connectionID)
	}
	return sess, nil
}

// GetActiveOrSingleSession finds the session by ID, or if ID is empty and exactly one session is active, returns it.
func (sm *SessionManager) GetActiveOrSingleSession(connectionID string) (*ActiveSession, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if connectionID != "" {
		if sess, ok := sm.sessions[connectionID]; ok {
			return sess, nil
		}
	}
	if len(sm.sessions) == 1 {
		for _, sess := range sm.sessions {
			return sess, nil
		}
	}
	if connectionID != "" {
		return nil, fmt.Errorf("session not found for connection %s; please connect first", connectionID)
	}
	return nil, fmt.Errorf("no active database session found")
}

func (sm *SessionManager) CloseSession(connectionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sess, ok := sm.sessions[connectionID]; ok {
		delete(sm.sessions, connectionID)
		return sess.Session.Close()
	}
	return nil
}

func (sm *SessionManager) CloseAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, sess := range sm.sessions {
		_ = sess.Session.Close()
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

	if req.Database != "" && s.Session != nil {
		_ = s.Session.SelectDatabase(ctx, req.Database)
	}

	// 1. Safety Guard Inspection
	var inspection *safety.InspectionResult
	if s.guard != nil {
		inspection = s.guard.Inspect(trimmedSQL, s.Environment)
	} else {
		inspection = &safety.InspectionResult{RiskLevel: safety.RiskNone}
	}

	// 2. Read-Only Enforcement
	if s.ReadOnly && inspection.IsDestructive {
		return nil, ErrReadOnlySession
	}

	// 3. Challenge Enforcement
	if inspection.RequiresChallenge && !req.Confirmed {
		return &ExecuteResponse{
			Blocked:    true,
			Inspection: inspection,
		}, nil
	}

	// Challenge typed table name validation
	if inspection.RequiresChallenge && req.Confirmed && inspection.TargetTable != "" {
		if !strings.EqualFold(strings.TrimSpace(req.ChallengeInput), inspection.TargetTable) {
			return nil, fmt.Errorf("Challenge confirmation failed: expected '%s', got '%s'", inspection.TargetTable, req.ChallengeInput)
		}
	}

	// 4. Pre-DDL Automated Backup
	var backupFile string
	var backupDataIncluded bool
	if inspection.Operation == "DROP" || inspection.Operation == "TRUNCATE" {
		if inspection.TargetTable != "" && s.backupMgr != nil && s.Session != nil {
			var err error
			backupFile, backupDataIncluded, err = s.backupMgr.BackupTable(ctx, s.Session, inspection.TargetTable)
			if err != nil {
				// Log backup warning, proceed with explicit challenge confirmation
			}
		}
	}

	start := time.Now()
	upperSQL := strings.ToUpper(trimmedSQL)

	queryID := req.QueryID
	if queryID == "" {
		queryID = "active"
	}

	execCtx, cancel := context.WithCancel(ctx)
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
	}()

	var execErr error
	var rowsAffected int64
	defer func() {
		if s.vault != nil && (inspection == nil || !inspection.RequiresChallenge || req.Confirmed) {
			dur := time.Since(start).Milliseconds()
			errStr := ""
			if execErr != nil {
				errStr = execErr.Error()
			}
			go func() {
				_ = s.vault.RecordAuditLog(&vault.AuditLogRecord{
					SessionID:    s.ID,
					ConnectionID: s.ConnectionID,
					QuerySQL:     trimmedSQL,
					DurationMs:   dur,
					RowsAffected: rowsAffected,
					Error:        errStr,
					ExecutedAt:   start,
				})
			}()
		}
	}()

	// 5. Route to Exec vs QueryStream
	isSelect := strings.HasPrefix(upperSQL, "SELECT") || strings.HasPrefix(upperSQL, "SHOW") || strings.HasPrefix(upperSQL, "DESC") || strings.HasPrefix(upperSQL, "EXPLAIN")

	if !isSelect {
		// Mutation execution
		execRes, err := s.Session.Exec(execCtx, trimmedSQL)
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
	}, nil
}

// CancelQuery attempts to interrupt a running query by queryID.
func (s *ActiveSession) CancelQuery(ctx context.Context, queryID string) error {
	s.mu.Lock()
	cancel, ok := s.runningQueries[queryID]
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

	if s.guard != nil {
		inspection := s.guard.Inspect(trimmedSQL, s.Environment)
		if s.ReadOnly && inspection.IsDestructive {
			return nil, ErrReadOnlySession
		}
		if inspection.IsDestructive || inspection.RequiresChallenge {
			return nil, fmt.Errorf("destructive query cannot be streamed: %s; use query editor with confirmation", inspection.Reason)
		}
	}

	if maxRows <= 0 {
		maxRows = 100000
	}

	return s.Session.QueryStream(ctx, trimmedSQL, maxRows)
}
