package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/driver"
	"sqlfox/internal/safety"
	"sqlfox/internal/vault"
)

func TestActiveSession_SafeExecutionPipeline(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-execution-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	dbSession, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer dbSession.Close()

	guard := safety.NewSafetyGuard()
	backupMgr := safety.NewBackupManager(backupDir)

	sess := &ActiveSession{
		ID:          "conn-1",
		Environment: "Development",
		DriverName:  "sqlite",
		ReadOnly:    false,
		Session:     dbSession,
		guard:       guard,
		backupMgr:   backupMgr,
	}

	ctx := context.Background()

	// 1. Execute DDL: CREATE TABLE
	res, err := sess.Execute(ctx, ExecuteRequest{
		SQL: "CREATE TABLE items (id INTEGER PRIMARY KEY, title TEXT, price REAL);",
	})
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if res.Type != "exec" {
		t.Fatalf("expected exec type, got %s", res.Type)
	}

	// 2. Execute Mutation: INSERT
	res, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "INSERT INTO items (title, price) VALUES ('Widget A', 19.99), ('Widget B', 29.99);",
	})
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}
	if res.RowsAffected != 2 {
		t.Fatalf("expected 2 rows affected, got %d", res.RowsAffected)
	}

	// 3. Execute Query with Implicit Safe Limit
	res, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "SELECT * FROM items ORDER BY id ASC;",
	})
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if res.Type != "query" || res.RowCount != 2 {
		t.Fatalf("expected 2 query rows, got %d (type %s)", res.RowCount, res.Type)
	}
	if len(res.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(res.Columns))
	}

	// 4. Destructive Query without confirmation -> Blocked
	res, err = sess.Execute(ctx, ExecuteRequest{
		SQL:       "DROP TABLE items;",
		Confirmed: false,
	})
	if err != nil {
		t.Fatalf("unexpected error on unconfirmed destructive query: %v", err)
	}
	if !res.Blocked || res.Inspection == nil || !res.Inspection.RequiresChallenge {
		t.Fatalf("expected blocked challenge, got %+v", res)
	}

	// 5. Destructive Query with wrong challenge input -> Error
	_, err = sess.Execute(ctx, ExecuteRequest{
		SQL:            "DROP TABLE items;",
		Confirmed:      true,
		ChallengeInput: "wrong_table",
	})
	if err == nil || !strings.Contains(err.Error(), "Challenge confirmation failed") {
		t.Fatalf("expected challenge mismatch error, got: %v", err)
	}

	// 6. Destructive Query with correct challenge input -> Triggers pre-DDL backup & executes
	res, err = sess.Execute(ctx, ExecuteRequest{
		SQL:            "DROP TABLE items;",
		Confirmed:      true,
		ChallengeInput: "items",
	})
	if err != nil {
		t.Fatalf("DROP TABLE with valid challenge failed: %v", err)
	}
	if res.BackupFile == "" {
		t.Fatalf("expected pre-DDL backup file to be recorded, got empty")
	}
	if _, statErr := os.Stat(res.BackupFile); statErr != nil {
		t.Fatalf("expected backup file on disk at %s, stat error: %v", res.BackupFile, statErr)
	}

	// 7. Test Stream Seam with SELECT query
	// Recreate table and insert row
	_, _ = sess.Execute(ctx, ExecuteRequest{SQL: "CREATE TABLE logs (id INT, msg TEXT);"})
	_, _ = sess.Execute(ctx, ExecuteRequest{SQL: "INSERT INTO logs VALUES (1, 'hello');"})

	stream, err := sess.Stream(ctx, "SELECT * FROM logs", 100)
	if err != nil {
		t.Fatalf("Stream failed for SELECT: %v", err)
	}
	chunk, err := stream.NextChunk(10)
	if err != nil && len(chunk) == 0 {
		t.Fatalf("Stream chunk failed: %v", err)
	}
	if len(chunk) != 1 {
		t.Fatalf("expected 1 row in stream chunk, got %d", len(chunk))
	}
	stream.Close()

	// 8. Test Stream Seam blocks Destructive Query (closing the WebSocket security leak)
	_, err = sess.Stream(ctx, "DROP TABLE logs;", 100)
	if err == nil || !strings.Contains(err.Error(), "destructive query cannot be streamed") {
		t.Fatalf("expected destructive query to be blocked from Stream, got: %v", err)
	}

	// 9. Read-Only Session Mode Enforcement
	sess.ReadOnly = true
	_, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "DROP TABLE logs;",
	})
	if err != ErrReadOnlySession {
		t.Fatalf("expected ErrReadOnlySession, got: %v", err)
	}

	_, err = sess.Stream(ctx, "DROP TABLE logs;", 100)
	if err != ErrReadOnlySession {
		t.Fatalf("expected ErrReadOnlySession on Stream, got: %v", err)
	}
}

func TestSessionManager_ConnectPassesGuardAndBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-sm-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "vault.db")
	targetDB := filepath.Join(tempDir, "target.db")
	v, err := vault.OpenVault(vaultDB, "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	conn := &vault.ConnectionRecord{
		Name:         "SM SQLite",
		Environment:  "Production",
		Driver:       "sqlite",
		DatabaseName: targetDB,
	}
	if err := v.SaveConnection(conn); err != nil {
		t.Fatal(err)
	}

	guard := safety.NewSafetyGuard()
	backupMgr := safety.NewBackupManager(filepath.Join(tempDir, "backups"))
	sm := NewSessionManager(v, guard, backupMgr)
	defer sm.CloseAll()

	sess, err := sm.ConnectSession(context.Background(), conn.ID)
	if err != nil {
		t.Fatalf("ConnectSession failed: %v", err)
	}

	if sess.guard != guard {
		t.Fatal("expected session to receive guard from SessionManager")
	}
	if sess.backupMgr != backupMgr {
		t.Fatal("expected session to receive backupMgr from SessionManager")
	}
	if !sess.ReadOnly {
		t.Fatal("expected Production session to default to ReadOnly")
	}
}

func TestActiveSession_CancelQuery(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-cancel-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "cancel.db")
	dbSession, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer dbSession.Close()

	sess := &ActiveSession{
		ID:             "conn-cancel",
		Environment:    "Development",
		DriverName:     "sqlite",
		Session:        dbSession,
		runningQueries: make(map[string]context.CancelFunc),
	}

	// Register a simulated running query
	cancelled := false
	sess.mu.Lock()
	sess.runningQueries["active"] = func() {
		cancelled = true
	}
	sess.mu.Unlock()

	// Cancel query
	err = sess.CancelQuery(context.Background(), "active")
	if err != nil {
		t.Fatalf("CancelQuery failed: %v", err)
	}

	if !cancelled {
		t.Fatal("expected registered cancel func to be triggered")
	}
}
