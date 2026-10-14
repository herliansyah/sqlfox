package driver_test

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
	"sqlfox/internal/driver"
)

func TestSQLRowStream(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-rowstream-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `
		CREATE TABLE users (id INT, name TEXT, bio BLOB);
		INSERT INTO users VALUES (1, 'Alice', 'Engineer'), (2, 'Bob', 'Designer'), (3, 'Charlie', 'Manager');
	`)
	if err != nil {
		t.Fatalf("failed to seed users: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT id, name, bio FROM users ORDER BY id ASC")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	colDefs := []driver.ColumnDefinition{
		{Name: "id", Type: "INTEGER", Nullable: false},
		{Name: "name", Type: "TEXT", Nullable: false},
		{Name: "bio", Type: "BLOB", Nullable: true},
	}

	stream := driver.NewSQLRowStream(rows, colDefs)
	defer stream.Close()

	// 1. Verify Columns
	if len(stream.Columns()) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(stream.Columns()))
	}

	// 2. Fetch first chunk (batch size 2)
	chunk1, err := stream.NextChunk(2)
	if err != nil {
		t.Fatalf("NextChunk(2) failed: %v", err)
	}
	if len(chunk1) != 2 {
		t.Fatalf("expected 2 rows in first chunk, got %d", len(chunk1))
	}
	// Verify blob byte conversion to string
	if chunk1[0][2] != "Engineer" {
		t.Fatalf("expected blob converted to string 'Engineer', got %v", chunk1[0][2])
	}

	// 3. Fetch second chunk (batch size 2, remaining 1)
	chunk2, err := stream.NextChunk(2)
	if err != nil {
		t.Fatalf("NextChunk(2) second call failed: %v", err)
	}
	if len(chunk2) != 1 {
		t.Fatalf("expected 1 row in second chunk, got %d", len(chunk2))
	}

	// 4. Next call should return io.EOF
	chunk3, err := stream.NextChunk(2)
	if err != io.EOF || len(chunk3) != 0 {
		t.Fatalf("expected io.EOF on exhausted stream, got chunk len %d, err %v", len(chunk3), err)
	}
}
