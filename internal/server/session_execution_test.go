package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/driver"
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

	sess := &ActiveSession{
		ID:          "conn-1",
		Environment: "Development",
		DriverName:  "sqlite",
		ReadOnly:    false,
		Session:     dbSession,
		backupDir:   backupDir,
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
		t.Fatalf("expected ErrReadOnlySession on DROP, got: %v", err)
	}

	_, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "INSERT INTO items (title, price) VALUES ('Evil', 0.0);",
	})
	if err != ErrReadOnlySession {
		t.Fatalf("expected ErrReadOnlySession on INSERT, got: %v", err)
	}

	_, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "UPDATE items SET price = 9.99 WHERE id = 1;",
	})
	if err != ErrReadOnlySession {
		t.Fatalf("expected ErrReadOnlySession on UPDATE, got: %v", err)
	}

	_, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "CREATE TABLE evil (id INT);",
	})
	if err != ErrReadOnlySession {
		t.Fatalf("expected ErrReadOnlySession on CREATE TABLE, got: %v", err)
	}

	// SELECT must still be permitted in Read-Only mode
	selRes, err := sess.Execute(ctx, ExecuteRequest{
		SQL: "SELECT * FROM logs;",
	})
	if err != nil || selRes.Type != "query" {
		t.Fatalf("expected SELECT to succeed in Read-Only mode, got: %v", err)
	}

	// Comment-prefixed SELECT must succeed and return rows in Read-Only mode
	commentSel, err := sess.Execute(ctx, ExecuteRequest{
		SQL: "-- check logs\n/* multiline */\nSELECT * FROM logs;",
	})
	if err != nil || commentSel.Type != "query" {
		t.Fatalf("expected comment-prefixed SELECT to succeed in Read-Only mode, got: %v (%+v)", err, commentSel)
	}

	// PRAGMA query on SQLite must return rows as query type
	pragmaRes, err := sess.Execute(ctx, ExecuteRequest{
		SQL: "PRAGMA table_info('logs');",
	})
	if err != nil || pragmaRes.Type != "query" || len(pragmaRes.Rows) == 0 {
		t.Fatalf("expected PRAGMA table_info to return query rows, got: %v (%+v)", err, pragmaRes)
	}

	// Stream must reject DDL and DML statements
	_, err = sess.Stream(ctx, "DROP TABLE logs;", 100)
	if err != ErrReadOnlySession {
		t.Fatalf("expected ErrReadOnlySession on Stream, got: %v", err)
	}

	_, err = sess.Stream(ctx, "INSERT INTO items (id, title) VALUES (999, 'Hacked');", 100)
	if err == nil {
		t.Fatal("expected Stream to reject non-query INSERT")
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

	backupDir := filepath.Join(tempDir, "backups")
	sm := NewSessionManager(v, backupDir)
	defer sm.CloseAll()

	sess, err := sm.ConnectSession(context.Background(), conn.ID)
	if err != nil {
		t.Fatalf("ConnectSession failed: %v", err)
	}

	if sess.backupDir != backupDir {
		t.Fatal("expected session to receive backupDir from SessionManager")
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

func TestActiveSession_CancelQuery_ClientCancelGuaranteed(t *testing.T) {
	sess := &ActiveSession{
		ID:             "conn-sat",
		Environment:    "Development",
		DriverName:     "sqlite",
		Session:        nil, // no driver session
		runningQueries: make(map[string]context.CancelFunc),
	}

	cancelled := false
	sess.mu.Lock()
	sess.runningQueries["q-sat"] = func() {
		cancelled = true
	}
	sess.mu.Unlock()

	// Calling cancel must trigger client-side cancel immediately even if driver session is nil
	err := sess.CancelQuery(context.Background(), "q-sat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cancelled {
		t.Fatal("expected client cancel to be executed")
	}
}

func TestActiveSession_TransactionLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-tx-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "tx.db")
	dbSession, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer dbSession.Close()

	sess := &ActiveSession{
		ID:          "conn-tx",
		Environment: "Development",
		DriverName:  "sqlite",
		Session:     dbSession,
	}

	ctx := context.Background()

	// 1. Create table
	_, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance REAL);",
	})
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	// 2. Begin transaction
	res, err := sess.Execute(ctx, ExecuteRequest{SQL: "BEGIN;"})
	if err != nil {
		t.Fatalf("BEGIN failed: %v", err)
	}
	if !strings.Contains(res.Message, "Transaction started") {
		t.Fatalf("expected transaction started message, got: %s", res.Message)
	}

	// 3. Double BEGIN should fail
	_, err = sess.Execute(ctx, ExecuteRequest{SQL: "START TRANSACTION;"})
	if err == nil {
		t.Fatal("expected error on nested/duplicate transaction begin")
	}

	// 4. Mutation within transaction
	res, err = sess.Execute(ctx, ExecuteRequest{
		SQL: "INSERT INTO accounts (id, balance) VALUES (1, 500.0);",
	})
	if err != nil {
		t.Fatalf("INSERT in tx failed: %v", err)
	}
	if !strings.Contains(res.Message, "in transaction") {
		t.Fatalf("expected '(in transaction)' note, got: %s", res.Message)
	}

	// 5. Query within transaction
	qRes, err := sess.Execute(ctx, ExecuteRequest{
		SQL: "SELECT balance FROM accounts WHERE id = 1;",
	})
	if err != nil {
		t.Fatalf("SELECT in tx failed: %v", err)
	}
	if qRes.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", qRes.RowCount)
	}

	// 6. Commit
	cRes, err := sess.Execute(ctx, ExecuteRequest{SQL: "COMMIT;"})
	if err != nil {
		t.Fatalf("COMMIT failed: %v", err)
	}
	if !strings.Contains(cRes.Message, "committed") {
		t.Fatalf("expected commit message, got: %s", cRes.Message)
	}

	// 7. Verify committed data
	vRes, err := sess.Execute(ctx, ExecuteRequest{
		SQL: "SELECT balance FROM accounts WHERE id = 1;",
	})
	if err != nil || vRes.RowCount != 1 {
		t.Fatalf("expected committed data to persist, got rows: %d, err: %v", vRes.RowCount, err)
	}

	// 8. Test Rollback
	_, _ = sess.Execute(ctx, ExecuteRequest{SQL: "BEGIN;"})
	_, _ = sess.Execute(ctx, ExecuteRequest{
		SQL: "INSERT INTO accounts (id, balance) VALUES (2, 300.0);",
	})
	_, err = sess.Execute(ctx, ExecuteRequest{SQL: "ROLLBACK;"})
	if err != nil {
		t.Fatalf("ROLLBACK failed: %v", err)
	}

	rbRes, err := sess.Execute(ctx, ExecuteRequest{
		SQL: "SELECT balance FROM accounts WHERE id = 2;",
	})
	if err != nil {
		t.Fatalf("query after rollback failed: %v", err)
	}
	if rbRes.RowCount != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", rbRes.RowCount)
	}

	// 8b. Test BEGIN TRANSACTION and COMMIT TRANSACTION syntax
	bRes, err := sess.Execute(ctx, ExecuteRequest{SQL: "BEGIN TRANSACTION;"})
	if err != nil || !strings.Contains(bRes.Message, "Transaction started") {
		t.Fatalf("expected BEGIN TRANSACTION to start dedicated transaction, got: %v (msg: %v)", err, bRes)
	}
	_, _ = sess.Execute(ctx, ExecuteRequest{SQL: "INSERT INTO accounts (id, balance) VALUES (99, 123.0);"})
	cRes2, err := sess.Execute(ctx, ExecuteRequest{SQL: "COMMIT TRANSACTION;"})
	if err != nil || !strings.Contains(cRes2.Message, "committed") {
		t.Fatalf("expected COMMIT TRANSACTION to succeed, got: %v", err)
	}

	// 8c. Test ROLLBACK TRANSACTION syntax
	_, _ = sess.Execute(ctx, ExecuteRequest{SQL: "BEGIN TRANSACTION;"})
	_, _ = sess.Execute(ctx, ExecuteRequest{SQL: "INSERT INTO accounts (id, balance) VALUES (100, 456.0);"})
	_, err = sess.Execute(ctx, ExecuteRequest{SQL: "ROLLBACK TRANSACTION;"})
	if err != nil {
		t.Fatalf("expected ROLLBACK TRANSACTION to succeed, got: %v", err)
	}
	rRes, _ := sess.Execute(ctx, ExecuteRequest{SQL: "SELECT balance FROM accounts WHERE id = 100;"})
	if rRes.RowCount != 0 {
		t.Fatalf("expected 0 rows after ROLLBACK TRANSACTION, got %d", rRes.RowCount)
	}

	// 8d. Test BEGIN with leading comments
	bResComment, err := sess.Execute(ctx, ExecuteRequest{SQL: "-- start tx\n/* block */\nBEGIN TRANSACTION;"})
	if err != nil || !strings.Contains(bResComment.Message, "Transaction started") {
		t.Fatalf("expected BEGIN with comments to start transaction, got: %v (msg: %v)", err, bResComment)
	}
	_, err = sess.Execute(ctx, ExecuteRequest{SQL: "ROLLBACK;"})
	if err != nil {
		t.Fatalf("expected ROLLBACK to succeed, got: %v", err)
	}

	// 9. Test Auto-Rollback on sess.Close()
	_, _ = sess.Execute(ctx, ExecuteRequest{SQL: "BEGIN;"})
	_, _ = sess.Execute(ctx, ExecuteRequest{
		SQL: "INSERT INTO accounts (id, balance) VALUES (3, 999.0);",
	})
	// Close session without committing
	_ = sess.Close()
	if sess.activeTx != nil {
		t.Fatal("expected activeTx to be cleared on Close()")
	}
}

func TestActiveSession_CancelQueryFallback(t *testing.T) {
	cancelled := false
	sess := &ActiveSession{
		ID: "test-sess",
		runningQueries: map[string]context.CancelFunc{
			"q-specific-123": func() {
				cancelled = true
			},
		},
	}

	// Calling CancelQuery with "active" should invoke any running query's cancel func
	err := sess.CancelQuery(context.Background(), "active")
	if err != nil {
		t.Fatalf("unexpected CancelQuery error: %v", err)
	}
	if !cancelled {
		t.Fatal("expected CancelQuery with 'active' to cancel running query via fallback")
	}
}


