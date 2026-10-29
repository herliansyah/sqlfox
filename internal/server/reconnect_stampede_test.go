package server

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"sqlfox/internal/vault"
)

// TestReconnectStampede_ConcurrentGetSession asserts that when an evicted session
// is accessed concurrently by multiple requests (e.g. processlist 1s polling),
// it does NOT perform eager heavy InspectSchema or dial multiple concurrent redundant connections.
func TestReconnectStampede_ConcurrentGetSession(t *testing.T) {
	tempDir := t.TempDir()
	vaultPath := filepath.Join(tempDir, "vault.db")
	v, err := vault.OpenVault(vaultPath, "test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	// Create an SQLite connection in vault
	sqlitePath := filepath.Join(tempDir, "test.db")
	conn := &vault.ConnectionRecord{
		ID:           "test-conn-stampede",
		Name:         "Stampede Test DB",
		Driver:       "sqlite",
		DatabaseName: sqlitePath,
		Environment:  "Development",
	}
	if err := v.SaveConnection(conn); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(tempDir, "backups")
	sm := NewSessionManager(v, backupDir)
	defer sm.CloseAll()

	// 1. Initial connect
	sess, err := sm.ConnectSession(context.Background(), conn.ID)
	if err != nil {
		t.Fatalf("ConnectSession failed: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}

	// 2. Simulate session eviction after 30m idle
	if err := sm.CloseSession(conn.ID); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	// Ensure it is evicted from memory
	sm.mu.RLock()
	_, exists := sm.sessions[conn.ID]
	sm.mu.RUnlock()
	if exists {
		t.Fatal("session should be evicted from memory")
	}

	// 3. Fire 10 concurrent GetSession requests simulating rapid 1s processlist polling
	const numCallers = 10
	var wg sync.WaitGroup
	var successCount int64
	sessions := make([]*ActiveSession, numCallers)

	startGate := make(chan struct{})

	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			<-startGate
			s, err := sm.GetSession(conn.ID)
			if err == nil && s != nil {
				atomic.AddInt64(&successCount, 1)
				sessions[idx] = s
			}
		}()
	}

	// Release all goroutines simultaneously
	close(startGate)
	wg.Wait()

	if successCount != numCallers {
		t.Fatalf("expected all %d callers to succeed, got %d", numCallers, successCount)
	}

	// Verify all returned sessions point to the exact same ActiveSession pointer
	first := sessions[0]
	for i := 1; i < numCallers; i++ {
		if sessions[i] != first {
			t.Errorf("caller %d received different session instance %p vs %p", i, sessions[i], first)
		}
	}
}

// TestConnectSession_DoesNotEagerlyInspectSchema verifies that ConnectSession does not
// eagerly execute full schema introspection (which runs heavy KEY_COLUMN_USAGE queries on MySQL),
// leaving schema inspection to lazy on-demand invocation.
func TestConnectSession_DoesNotEagerlyInspectSchema(t *testing.T) {
	tempDir := t.TempDir()
	vaultPath := filepath.Join(tempDir, "vault.db")
	v, err := vault.OpenVault(vaultPath, "test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	sqlitePath := filepath.Join(tempDir, "test.db")
	conn := &vault.ConnectionRecord{
		ID:           "test-conn-lazy",
		Name:         "Lazy Schema Test DB",
		Driver:       "sqlite",
		DatabaseName: sqlitePath,
		Environment:  "Development",
	}
	if err := v.SaveConnection(conn); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(tempDir, "backups")
	sm := NewSessionManager(v, backupDir)
	defer sm.CloseAll()

	sess, err := sm.ConnectSession(context.Background(), conn.ID)
	if err != nil {
		t.Fatalf("ConnectSession failed: %v", err)
	}

	// CachedSchema should be nil upon connect to prevent metadata lock contention and delay.
	// Schema inspection should only happen when explicitly requested via handleGetSchema.
	if sess.CachedSchema != nil {
		t.Errorf("expected CachedSchema to be nil upon connect (lazy), got %+v", sess.CachedSchema)
	}
}
