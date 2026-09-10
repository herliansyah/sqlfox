package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sqlfox/internal/driver"
	"sqlfox/internal/safety"
	"sqlfox/internal/vault"
)

// ActiveSession holds an active database connection session and its cached metadata.
type ActiveSession struct {
	ID           string                 `json:"id"`
	ConnectionID string                 `json:"connection_id"`
	Name         string                 `json:"name"`
	Environment  string                 `json:"environment"`
	DriverName   string                 `json:"driver_name"`
	ReadOnly     bool                   `json:"read_only"`
	Session      driver.Session         `json:"-"`
	CachedSchema *driver.SchemaMetadata `json:"schema,omitempty"`
	ServerInfo   *driver.ServerInfo     `json:"server_info,omitempty"`
	LastActive   time.Time              `json:"last_active"`
	mu           sync.RWMutex
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
		LastActive:   time.Now(),
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
