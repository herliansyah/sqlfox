package sqlite

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"sqlfox/internal/driver"
)

func TestSQLiteDriver_Lifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-sqlite-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	ctx := context.Background()

	sess, err := driver.Connect(ctx, driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer sess.Close()

	// 1. Ping and ServerInfo
	latency, err := sess.Ping(ctx)
	if err != nil || latency < 0 {
		t.Fatalf("Ping failed: %v", err)
	}

	info, err := sess.ServerInfo(ctx)
	if err != nil || info.Version == "" {
		t.Fatalf("ServerInfo failed: %v", err)
	}

	// 2. Exec CREATE TABLE
	createSQL := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		name TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX idx_users_name ON users(name);
	`
	_, err = sess.Exec(ctx, createSQL)
	if err != nil {
		t.Fatalf("Exec create table failed: %v", err)
	}

	// 3. InspectSchema
	schema, err := sess.InspectSchema(ctx, "main")
	if err != nil {
		t.Fatalf("InspectSchema failed: %v", err)
	}

	if len(schema.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(schema.Tables))
	}

	tbl := schema.Tables[0]
	if tbl.Name != "users" {
		t.Fatalf("expected table 'users', got %s", tbl.Name)
	}
	if len(tbl.Columns) < 4 {
		t.Fatalf("expected at least 4 columns, got %d", len(tbl.Columns))
	}
	if len(tbl.Indexes) == 0 {
		t.Fatal("expected idx_users_name index")
	}

	// 4. Insert rows & QueryStream
	_, _ = sess.Exec(ctx, "BEGIN TRANSACTION;")
	for i := 1; i <= 50; i++ {
		_, _ = sess.Exec(ctx, "INSERT INTO users (email, name) VALUES ('user"+string(rune(i+64))+"@example.com', 'User')")
	}
	_, _ = sess.Exec(ctx, "COMMIT;")

	stream, err := sess.QueryStream(ctx, "SELECT id, email, name FROM users", 100)
	if err != nil {
		t.Fatalf("QueryStream failed: %v", err)
	}
	defer stream.Close()

	if len(stream.Columns()) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(stream.Columns()))
	}

	chunk, err := stream.NextChunk(50)
	if err != nil && err != io.EOF {
		t.Fatalf("NextChunk failed: %v", err)
	}
	if len(chunk) == 0 {
		t.Fatal("expected non-empty chunk")
	}

	// 5. GetTableDDL
	ddl, err := sess.GetTableDDL(ctx, "users")
	if err != nil || ddl == "" {
		t.Fatalf("GetTableDDL failed: %v", err)
	}

	// 6. Explain
	plan, err := sess.Explain(ctx, "SELECT * FROM users WHERE name = 'User'")
	if err != nil || len(plan.Rows) == 0 {
		t.Fatalf("Explain failed: %v", err)
	}
}
