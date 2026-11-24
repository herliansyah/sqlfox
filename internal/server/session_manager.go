package server

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/vault"
)

// connectCall tracks in-flight connection dialing to deduplicate concurrent connection attempts.
type connectCall struct {
	wg  sync.WaitGroup
	val *ActiveSession
	err error
}

// SessionManager manages connected database sessions.
type SessionManager struct {
	vault          *vault.Vault
	backupDir      string
	sessions       map[string]*ActiveSession
	connecting     map[string]*connectCall
	readOnlyStates map[string]bool
	stopReaper     chan struct{}
	mu             sync.RWMutex
}

func NewSessionManager(v *vault.Vault, backupDir string) *SessionManager {
	sm := &SessionManager{
		vault:          v,
		backupDir:      backupDir,
		sessions:       make(map[string]*ActiveSession),
		connecting:     make(map[string]*connectCall),
		readOnlyStates: make(map[string]bool),
		stopReaper:     make(chan struct{}),
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
			now := time.Now()

			// Phase 1: under the locks, snapshot the transactions to roll back and the sessions to
			// evict, and detach them from the maps. No blocking DB I/O is performed while any lock
			// is held — rollbacks and Close run in phase 2 after the locks are released.
			var txToRollback []driver.TxSession
			var sessToClose []*ActiveSession

			sm.mu.Lock()
			for id, sess := range sm.sessions {
				sess.mu.Lock()
				// Never roll back or evict a session with a request actively executing against it:
				// a long-running statement or stream legitimately holds its transaction past 60s.
				if sess.inFlight > 0 {
					sess.mu.Unlock()
					continue
				}
				if sess.tabTransactions != nil {
					for tabID, entry := range sess.tabTransactions {
						if entry == nil {
							delete(sess.tabTransactions, tabID)
							continue
						}
						if entry.aborted {
							// If already aborted for > 5 mins, clean up memory
							if now.Sub(entry.lastActive) > 5*time.Minute {
								delete(sess.tabTransactions, tabID)
							}
							continue
						}
						if entry.tx == nil {
							delete(sess.tabTransactions, tabID)
							continue
						}
						refTime := entry.lastActive
						if refTime.IsZero() {
							refTime = entry.startedAt
						}
						if now.Sub(refTime) > 60*time.Second {
							txToRollback = append(txToRollback, entry.tx)
							entry.tx = nil
							entry.aborted = true
							entry.lastActive = now
							entry.abortReason = "transaction timed out after 60s idle and was automatically rolled back to prevent orphaned database locks"
						}
					}
				}
				isIdle := now.Sub(sess.LastActive) > 30*time.Minute
				sess.mu.Unlock()

				if isIdle {
					sessToClose = append(sessToClose, sess)
					delete(sm.sessions, id)
				}
			}
			sm.mu.Unlock()

			// Phase 2: blocking teardown without any lock held.
			for _, tx := range txToRollback {
				_ = tx.Rollback()
			}
			for _, sess := range sessToClose {
				_ = sess.Close()
			}
		}
	}
}

// ConnectSession creates or activates a session for the given connection ID.
func (sm *SessionManager) ConnectSession(ctx context.Context, connectionID string) (*ActiveSession, error) {
	sm.mu.Lock()
	if sess, ok := sm.sessions[connectionID]; ok {
		sm.mu.Unlock()
		sess.mu.Lock()
		sess.LastActive = time.Now()
		sess.mu.Unlock()
		return sess, nil
	}
	// ponytail: singleflight in-flight connection requests for the same connectionID to prevent stampeding the DB
	if call, ok := sm.connecting[connectionID]; ok {
		sm.mu.Unlock()
		call.wg.Wait()
		return call.val, call.err
	}
	call := &connectCall{}
	call.wg.Add(1)
	sm.connecting[connectionID] = call
	sm.mu.Unlock()

	defer func() {
		sm.mu.Lock()
		delete(sm.connecting, connectionID)
		sm.mu.Unlock()
		call.wg.Done()
	}()

	conn, err := sm.vault.GetConnection(connectionID)
	if err != nil {
		call.err = fmt.Errorf("failed to get connection config: %w", err)
		return nil, call.err
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
		call.err = fmt.Errorf("failed to connect to database: %w", err)
		return nil, call.err
	}

	info, _ := dbSession.ServerInfo(ctx)
	// ponytail: defer schema introspection to lazy on-demand fetching (handleGetSchema)
	// to eliminate heavy KEY_COLUMN_USAGE lock contention during session connection / reconnect
	latency, _ := dbSession.Ping(ctx)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocMB := math.Round(float64(m.Alloc)/1024/1024*10) / 10

	sm.mu.Lock()
	ro, hasOverride := sm.readOnlyStates[connectionID]
	if !hasOverride {
		ro = conn.Environment == "Production"
		sm.readOnlyStates[connectionID] = ro
	}
	sm.mu.Unlock()

	active := &ActiveSession{
		ID:             connectionID,
		ConnectionID:   connectionID,
		Name:           conn.Name,
		DatabaseName:   conn.DatabaseName,
		Environment:    conn.Environment,
		DriverName:     conn.Driver,
		ReadOnly:       ro,
		Session:        dbSession,
		ServerInfo:     info,
		LatencyMs:      latency.Milliseconds(),
		MemoryMB:       allocMB,
		LastActive:     time.Now(),
		backupDir:      sm.backupDir,
		vault:          sm.vault,
		sessionMgr:     sm,
		runningQueries: make(map[string]context.CancelFunc),
	}

	sm.mu.Lock()
	sm.sessions[connectionID] = active
	sm.mu.Unlock()

	call.val = active
	return active, nil
}

// SetReadOnly updates and persists the read-only state override for a connection.
func (sm *SessionManager) SetReadOnly(connectionID string, ro bool) {
	sm.mu.Lock()
	if sm.readOnlyStates == nil {
		sm.readOnlyStates = make(map[string]bool)
	}
	sm.readOnlyStates[connectionID] = ro
	sess := sm.sessions[connectionID]
	sm.mu.Unlock()

	if sess != nil {
		sess.mu.Lock()
		sess.ReadOnly = ro
		sess.mu.Unlock()
	}
}

// RemoveReadOnly clears the stored read-only state for a connection.
func (sm *SessionManager) RemoveReadOnly(connectionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.readOnlyStates, connectionID)
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
	sess, ok := sm.sessions[connectionID]
	if ok {
		delete(sm.sessions, connectionID)
	}
	sm.mu.Unlock()

	// Close outside the manager lock: Close performs blocking rollbacks + socket teardown and must
	// not serialize every other ConnectSession/GetSession for its duration.
	if ok {
		return sess.Close()
	}
	return nil
}

func (sm *SessionManager) CloseAll() {
	sm.mu.Lock()
	select {
	case <-sm.stopReaper:
	default:
		close(sm.stopReaper)
	}
	toClose := make([]*ActiveSession, 0, len(sm.sessions))
	for id, sess := range sm.sessions {
		toClose = append(toClose, sess)
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()

	for _, sess := range toClose {
		_ = sess.Close()
	}
}
