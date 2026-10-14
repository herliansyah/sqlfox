package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/driver"
	"sqlfox/internal/vault"
)

func TestCLI_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") || !strings.Contains(stdout.String(), "query") {
		t.Fatalf("expected usage info in stdout, got: %s", stdout.String())
	}
}

func TestCLI_QueryDirectDSN_Formats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Pre-create table and data
	ctx := context.Background()
	sess, err := driver.Connect(ctx, driver.Config{Driver: "sqlite", DatabaseName: dbPath})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, _ = sess.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);")
	_, _ = sess.Exec(ctx, "INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com'), ('Bob', 'bob@example.com');")
	sess.Close()

	dsn := "sqlite://" + dbPath

	// 1. Table format
	{
		var stdout, stderr bytes.Buffer
		code := Run([]string{"query", "--dsn", dsn, "--format", "table", "SELECT id, name FROM users ORDER BY id ASC"}, &stdout, &stderr, nil)
		if code != 0 {
			t.Fatalf("table format failed: code=%d, stderr=%s", code, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") || !strings.Contains(out, "name") {
			t.Fatalf("unexpected table output: %s", out)
		}
	}

	// 2. JSON format
	{
		var stdout, stderr bytes.Buffer
		code := Run([]string{"query", "--dsn", dsn, "--format", "json", "SELECT id, name FROM users ORDER BY id ASC"}, &stdout, &stderr, nil)
		if code != 0 {
			t.Fatalf("json format failed: code=%d, stderr=%s", code, stderr.String())
		}
		var list []map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
			t.Fatalf("failed to parse json output: %v, raw: %s", err, stdout.String())
		}
		if len(list) != 2 || list[0]["name"] != "Alice" {
			t.Fatalf("unexpected json data: %+v", list)
		}
	}

	// 3. NDJSON format
	{
		var stdout, stderr bytes.Buffer
		code := Run([]string{"query", "--dsn", dsn, "--format", "ndjson", "SELECT id, name FROM users ORDER BY id ASC"}, &stdout, &stderr, nil)
		if code != 0 {
			t.Fatalf("ndjson format failed: code=%d, stderr=%s", code, stderr.String())
		}
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 ndjson lines, got %d: %s", len(lines), stdout.String())
		}
		var row map[string]any
		_ = json.Unmarshal([]byte(lines[0]), &row)
		if row["name"] != "Alice" {
			t.Fatalf("unexpected ndjson row 0: %+v", row)
		}
	}

	// 4. CSV format
	{
		var stdout, stderr bytes.Buffer
		code := Run([]string{"query", "--dsn", dsn, "--format", "csv", "SELECT id, name FROM users ORDER BY id ASC"}, &stdout, &stderr, nil)
		if code != 0 {
			t.Fatalf("csv format failed: code=%d, stderr=%s", code, stderr.String())
		}
		out := stdout.String()
		if !strings.HasPrefix(out, "id,name") || !strings.Contains(out, "1,Alice") {
			t.Fatalf("unexpected csv output: %s", out)
		}
	}
}

func TestCLI_Query_StdinPipe(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := "sqlite://" + dbPath

	stdin := strings.NewReader(`
		CREATE TABLE items (id INTEGER PRIMARY KEY, title TEXT);
		INSERT INTO items (title) VALUES ('Widget A'), ('Widget B');
	`)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"query", "--dsn", dsn}, &stdout, &stderr, stdin)
	if code != 0 {
		t.Fatalf("stdin execution failed: code=%d, stderr=%s", code, stderr.String())
	}

	// Verify items inserted
	var outBuf, errBuf bytes.Buffer
	code = Run([]string{"query", "--dsn", dsn, "--format", "csv", "SELECT COUNT(*) AS total FROM items"}, &outBuf, &errBuf, nil)
	if code != 0 {
		t.Fatalf("count failed: %s", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "2") {
		t.Fatalf("expected count 2, got: %s", outBuf.String())
	}
}

func TestCLI_Query_SafetyGuard_FailClosed(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := "sqlite://" + dbPath

	// 1. Setup table
	ctx := context.Background()
	sess, _ := driver.Connect(ctx, driver.Config{Driver: "sqlite", DatabaseName: dbPath})
	_, _ = sess.Exec(ctx, "CREATE TABLE sensitive_data (id INT, secret TEXT);")
	sess.Close()

	// 2. Destructive query without --force should fail-closed with code 2
	var stdout, stderr bytes.Buffer
	code := Run([]string{"query", "--dsn", dsn, "DROP TABLE sensitive_data"}, &stdout, &stderr, nil)
	if code != 2 {
		t.Fatalf("expected exit code 2 (Safety Guard Block), got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Safety Guard Block") {
		t.Fatalf("expected Safety Guard message, got: %s", stderr.String())
	}

	// 3. With --force, should succeed with exit code 0
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"query", "--dsn", dsn, "--force", "DROP TABLE sensitive_data"}, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("expected exit code 0 with --force, got %d, stderr=%s", code, stderr.String())
	}
}

func TestCLI_Query_VaultProfile(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "vault.db")
	dbPath := filepath.Join(tmpDir, "app.db")

	v, err := vault.OpenVault(vaultPath, "masterpass123")
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	err = v.SaveConnection(&vault.ConnectionRecord{
		Name:         "LocalSQLiteApp",
		Environment:  "Staging",
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("failed to save connection: %v", err)
	}
	v.Close()

	// Setup table in app.db
	ctx := context.Background()
	sess, _ := driver.Connect(ctx, driver.Config{Driver: "sqlite", DatabaseName: dbPath})
	_, _ = sess.Exec(ctx, "CREATE TABLE settings (key TEXT, val TEXT);")
	_, _ = sess.Exec(ctx, "INSERT INTO settings VALUES ('theme', 'dark');")
	sess.Close()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"query", "--conn", "LocalSQLiteApp", "--vault-db", vaultPath, "--pass", "masterpass123", "--format", "json", "SELECT key, val FROM settings"}, &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("vault profile query failed: code=%d, stderr=%s", code, stderr.String())
	}

	var res []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v, raw: %s", err, stdout.String())
	}
	if len(res) != 1 || res[0]["key"] != "theme" || res[0]["val"] != "dark" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestCLI_Transfer(t *testing.T) {
	tmpDir := t.TempDir()
	srcDb := filepath.Join(tmpDir, "src.db")
	dstDb := filepath.Join(tmpDir, "dst.db")

	// 1. Setup source database with 25 rows
	ctx := context.Background()
	srcSess, err := driver.Connect(ctx, driver.Config{Driver: "sqlite", DatabaseName: srcDb})
	if err != nil {
		t.Fatalf("failed to connect src: %v", err)
	}
	_, _ = srcSess.Exec(ctx, "CREATE TABLE products (id INTEGER PRIMARY KEY, sku TEXT, price REAL);")
	for i := 1; i <= 25; i++ {
		_, _ = srcSess.Exec(ctx, fmt.Sprintf("INSERT INTO products (sku, price) VALUES ('SKU-%03d', %d.5);", i, i*10))
	}
	srcSess.Close()

	fromDSN := "sqlite://" + srcDb
	toDSN := "sqlite://" + dstDb

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"transfer",
		"--from", fromDSN,
		"--to", toDSN,
		"--tables", "products",
		"--create-missing",
		"--batch-size", "10",
	}, &stdout, &stderr, nil)

	if code != 0 {
		t.Fatalf("transfer failed: code=%d, stderr=%s", code, stderr.String())
	}

	// Verify destination database
	dstSess, err := driver.Connect(ctx, driver.Config{Driver: "sqlite", DatabaseName: dstDb})
	if err != nil {
		t.Fatalf("failed to connect dst: %v", err)
	}
	defer dstSess.Close()

	stream, err := dstSess.QueryStream(ctx, "SELECT COUNT(*) AS total FROM products", 0)
	if err != nil {
		t.Fatalf("failed to count dst products: %v", err)
	}
	defer stream.Close()

	chunk, err := stream.NextChunk(10)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read count: %v", err)
	}
	if len(chunk) > 0 {
		countVal := fmt.Sprint(chunk[0][0])
		if countVal != "25" {
			t.Fatalf("expected 25 transferred rows, got: %s", countVal)
		}
	}
}

func TestParseDirectDSN_Postgres(t *testing.T) {
	driverName, cfg, ok := parseDirectDSN("postgres://postgres:secret@localhost:5432/myapp?sslmode=disable", "")
	if !ok {
		t.Fatalf("expected DSN to parse successfully")
	}
	if driverName != "postgres" || cfg.Driver != "postgres" {
		t.Errorf("expected driver 'postgres', got %q", driverName)
	}
	if cfg.Host != "localhost" || cfg.Port != 5432 || cfg.DatabaseName != "myapp" {
		t.Errorf("unexpected host/port/db: %s:%d/%s", cfg.Host, cfg.Port, cfg.DatabaseName)
	}
	if cfg.User != "postgres" || cfg.Password != "secret" {
		t.Errorf("unexpected credentials: %s:%s", cfg.User, cfg.Password)
	}
	if cfg.UseSSL {
		t.Errorf("expected UseSSL=false for sslmode=disable")
	}

	// Test postgresql alias with sslmode=require
	driverName2, cfg2, ok2 := parseDirectDSN("postgresql://dbuser:mypass@remotehost/analytics?sslmode=require", "")
	if !ok2 || driverName2 != "postgres" {
		t.Fatalf("expected postgresql scheme to parse")
	}
	if cfg2.Port != 5432 || cfg2.DatabaseName != "analytics" {
		t.Errorf("unexpected host/port/db: %s:%d/%s", cfg2.Host, cfg2.Port, cfg2.DatabaseName)
	}
	if !cfg2.UseSSL {
		t.Errorf("expected UseSSL=true for sslmode=require")
	}

	// Test driver override
	driverName3, cfg3, ok3 := parseDirectDSN("mydb", "postgres")
	if !ok3 || driverName3 != "postgres" || cfg3.DatabaseName != "mydb" {
		t.Errorf("expected driver override to return postgres")
	}
}

func TestSplitStatements_EscapedBackslashAndInlineComment(t *testing.T) {
	script := "INSERT INTO paths VALUES ('c:\\\\');\nSELECT 2;"
	stmts := SplitStatements(strings.NewReader(script))
	if len(stmts) != 2 {
		t.Fatalf("expected exactly 2 statements, got %d: %+v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "c:\\") {
		t.Errorf("stmt 0 should contain path: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "SELECT 2") {
		t.Errorf("stmt 1 should be SELECT 2: %s", stmts[1])
	}
}

func TestSplitStatements_PostgresOperatorAndTrailingComments(t *testing.T) {
	pgScript := "SELECT data #>> '{a}' FROM t;\nSELECT 2;"
	stmts1 := SplitStatements(strings.NewReader(pgScript), "postgres")
	if len(stmts1) != 2 {
		t.Fatalf("expected 2 statements for postgres #>> operator, got %d: %+v", len(stmts1), stmts1)
	}
	if !strings.Contains(stmts1[0], "#>>") {
		t.Errorf("stmt 0 should contain #>>: %s", stmts1[0])
	}
	if !strings.Contains(stmts1[1], "SELECT 2") {
		t.Errorf("stmt 1 should be SELECT 2: %s", stmts1[1])
	}

	script2 := "COMMIT;\n-- done comment"
	stmts2 := SplitStatements(strings.NewReader(script2))
	if len(stmts2) != 1 {
		t.Fatalf("expected 1 statement (excluding comment-only trailing statement), got %d: %+v", len(stmts2), stmts2)
	}
}

type mockTxSession struct {
	lastSQL string
}

func (m *mockTxSession) Exec(ctx context.Context, sql string, args ...any) (*driver.ExecResult, error) {
	m.lastSQL = sql
	return &driver.ExecResult{RowsAffected: 1}, nil
}
func (m *mockTxSession) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	m.lastSQL = sql
	return 1, nil
}
func (m *mockTxSession) QueryStream(ctx context.Context, sql string, limit int) (driver.RowStream, error) {
	return nil, nil
}
func (m *mockTxSession) Commit() error   { return nil }
func (m *mockTxSession) Rollback() error { return nil }

func TestFlushInsertBatch_MySQLBackslash(t *testing.T) {
	mockTx := &mockTxSession{}
	err := flushInsertBatch(context.Background(), mockTx, "paths", []string{"dir"}, [][]any{{`C:\temp\`}}, "mysql")
	if err != nil {
		t.Fatalf("flushInsertBatch failed: %v", err)
	}
	expected := `'C:\\temp\\'`
	if !strings.Contains(mockTx.lastSQL, expected) {
		t.Errorf("expected MySQL query to contain %s, got: %s", expected, mockTx.lastSQL)
	}
}

func TestSplitStatements_MySQLVersionComments(t *testing.T) {
	script := `
/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
CREATE TABLE users (id int) ENGINE=InnoDB /*!40101 DEFAULT CHARSET=utf8mb4 */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
`
	stmts := SplitStatements(strings.NewReader(script), "mysql")
	if len(stmts) != 4 {
		t.Fatalf("expected 4 statements including version comments, got %d: %+v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "CHARACTER_SET_CLIENT") {
		t.Errorf("stmt 0 should be character set client statement: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "FOREIGN_KEY_CHECKS=0") {
		t.Errorf("stmt 1 should contain FOREIGN_KEY_CHECKS=0: %s", stmts[1])
	}
	if !strings.Contains(stmts[2], "/*!40101 DEFAULT CHARSET=utf8mb4 */") {
		t.Errorf("stmt 2 should contain version comment: %s", stmts[2])
	}
}

func TestSplitStatements_PostgresTrailingBackslashInString(t *testing.T) {
	pgScript := "INSERT INTO paths VALUES ('C:\\');\nSELECT 2;"
	stmts := SplitStatements(strings.NewReader(pgScript), "postgres")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements in postgres, got %d: %+v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "'C:\\'") {
		t.Errorf("stmt 0 should contain 'C:\\', got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "SELECT 2") {
		t.Errorf("stmt 1 should be SELECT 2, got: %s", stmts[1])
	}
}

func TestIsRowReturningQuery_WithLeadingComments(t *testing.T) {
	// Leading single-line comment
	if !isRowReturningQuery("-- list\nSELECT 42 AS answer;") {
		t.Errorf("expected isRowReturningQuery to be true for SELECT with leading comment")
	}

	// Leading block comment
	if !isRowReturningQuery("/* block comment */ SELECT 1;") {
		t.Errorf("expected isRowReturningQuery to be true for SELECT with block comment")
	}

	// Leading comment before mutation should still be false
	if isRowReturningQuery("-- comment\nUPDATE users SET active = 1;") {
		t.Errorf("expected isRowReturningQuery to be false for UPDATE statement")
	}
}
