package driver_test

import (
	"database/sql"
	"encoding/json"
	"math"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/mysql"
	_ "sqlfox/internal/driver/postgres"
	_ "sqlfox/internal/driver/sqlite"

	_ "modernc.org/sqlite"
)

func TestScanExplainRows_MariaDB10Columns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	// Simulate 10-column EXPLAIN output from MariaDB / older MySQL
	query := `SELECT 1 as id, 'SIMPLE' as select_type, 'users' as ` + "`table`" + `, 'ALL' as type,
		'idx_users_email' as possible_keys, NULL as ` + "`key`" + `, NULL as key_len, NULL as ref,
		1500 as ` + "`rows`" + `, 'Using where' as extra`

	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("failed to execute query: %v", err)
	}
	defer rows.Close()

	planRows, err := driver.ScanExplainRows(rows)
	if err != nil {
		t.Fatalf("ScanExplainRows failed: %v", err)
	}

	if len(planRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(planRows))
	}

	row := planRows[0]
	if row.ID != 1 {
		t.Errorf("expected ID 1, got %d", row.ID)
	}
	if row.SelectType != "SIMPLE" {
		t.Errorf("expected SelectType SIMPLE, got %s", row.SelectType)
	}
	if row.Table != "users" {
		t.Errorf("expected Table users, got %s", row.Table)
	}
	if row.Type != "ALL" {
		t.Errorf("expected Type ALL, got %s", row.Type)
	}
	if row.PossibleKeys != "idx_users_email" {
		t.Errorf("expected PossibleKeys idx_users_email, got %s", row.PossibleKeys)
	}
	if row.Rows != 1500 {
		t.Errorf("expected Rows 1500, got %d", row.Rows)
	}
	if row.Extra != "Using where" {
		t.Errorf("expected Extra 'Using where', got %s", row.Extra)
	}
}

func TestScanExplainRows_MySQL12Columns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	// Simulate 12-column EXPLAIN output from MySQL 8.0
	query := `SELECT 1 as id, 'PRIMARY' as select_type, 'orders' as ` + "`table`" + `, 'p0,p1' as partitions,
		'ref' as type, 'PRIMARY,idx_cust' as possible_keys, 'idx_cust' as ` + "`key`" + `, '8' as key_len,
		'const' as ref, 25 as ` + "`rows`" + `, 100.0 as filtered, 'Backward index scan' as extra`

	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("failed to execute query: %v", err)
	}
	defer rows.Close()

	planRows, err := driver.ScanExplainRows(rows)
	if err != nil {
		t.Fatalf("ScanExplainRows failed: %v", err)
	}

	if len(planRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(planRows))
	}

	row := planRows[0]
	if row.Table != "orders" || row.Partitions != "p0,p1" || row.Key != "idx_cust" || row.Filtered != 100.0 {
		t.Errorf("unexpected row values: %+v", row)
	}
}

func TestRegistry(t *testing.T) {
	d, err := driver.GetDriver("sqlite")
	if err != nil {
		t.Fatalf("expected sqlite driver to be registered, got: %v", err)
	}
	if d.Name() != "sqlite" {
		t.Fatalf("expected sqlite name, got: %s", d.Name())
	}

	dMySQL, err := driver.GetDriver("mysql")
	if err != nil {
		t.Fatalf("expected mysql driver to be registered, got: %v", err)
	}
	if dMySQL.Name() != "mysql" {
		t.Fatalf("expected mysql name, got: %s", dMySQL.Name())
	}

	dPG, err := driver.GetDriver("postgres")
	if err != nil {
		t.Fatalf("expected postgres driver to be registered, got: %v", err)
	}
	if dPG.Name() != "postgres" {
		t.Fatalf("expected postgres name, got: %s", dPG.Name())
	}

	_, err = driver.GetDriver("unregistered-db")
	if err == nil {
		t.Fatal("expected error for unregistered driver, got nil")
	}
}

func TestTxSession_CommitAndRollback(t *testing.T) {
	d, err := driver.GetDriver("sqlite")
	if err != nil {
		t.Fatalf("failed to get sqlite driver: %v", err)
	}

	sess, err := d.Connect(t.Context(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: ":memory:",
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Exec(t.Context(), "CREATE TABLE items (id INT, name TEXT)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// 1. Test Rollback
	tx, err := sess.BeginTx(t.Context())
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	if _, err := tx.Exec(t.Context(), "INSERT INTO items VALUES (1, 'Alpha')"); err != nil {
		t.Fatalf("insert in tx failed: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	stream, err := sess.QueryStream(t.Context(), "SELECT count(*) FROM items", 1)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	chunk, _ := stream.NextChunk(1)
	_ = stream.Close()
	if len(chunk) != 1 || chunk[0][0].(int64) != 0 {
		t.Fatalf("expected 0 items after rollback, got %v", chunk)
	}

	// 2. Test Commit
	tx2, err := sess.BeginTx(t.Context())
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	if _, err := tx2.Exec(t.Context(), "INSERT INTO items VALUES (2, 'Beta')"); err != nil {
		t.Fatalf("insert in tx failed: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	stream2, err := sess.QueryStream(t.Context(), "SELECT count(*) FROM items", 1)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	chunk2, _ := stream2.NextChunk(1)
	_ = stream2.Close()
	if len(chunk2) != 1 || chunk2[0][0].(int64) != 1 {
		t.Fatalf("expected 1 item after commit, got %v", chunk2)
	}
}

func TestQueryStream_WithQueryIDCancellation(t *testing.T) {
	d, err := driver.GetDriver("sqlite")
	if err != nil {
		t.Fatalf("failed to get sqlite driver: %v", err)
	}

	sess, err := d.Connect(t.Context(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: ":memory:",
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer sess.Close()

	ctx := driver.WithQueryID(t.Context(), "test-query-id")
	stream, err := sess.QueryStream(ctx, "SELECT 1", 1)
	if err != nil {
		t.Fatalf("QueryStream failed: %v", err)
	}

	// Cancel query
	if err := sess.CancelQuery(t.Context(), "test-query-id"); err != nil {
		t.Fatalf("CancelQuery failed: %v", err)
	}

	_ = stream.Close()
}

func TestPlaceholder_Dialects(t *testing.T) {
	if p := driver.Placeholder("postgres", 1); p != "$1" {
		t.Fatalf("expected $1, got %s", p)
	}
	if p := driver.Placeholder("PostgreSQL", 5); p != "$5" {
		t.Fatalf("expected $5, got %s", p)
	}
	if p := driver.Placeholder("pg", 2); p != "$2" {
		t.Fatalf("expected $2, got %s", p)
	}
	if p := driver.Placeholder("mysql", 1); p != "?" {
		t.Fatalf("expected ?, got %s", p)
	}
	if p := driver.Placeholder("sqlite", 3); p != "?" {
		t.Fatalf("expected ?, got %s", p)
	}
}

func TestFormatSQLValue_Booleans(t *testing.T) {
	if v := driver.FormatSQLValue(true); v != "TRUE" {
		t.Fatalf("expected TRUE, got %s", v)
	}
	if v := driver.FormatSQLValue(false); v != "FALSE" {
		t.Fatalf("expected FALSE, got %s", v)
	}
}

func TestFormatSQLValue_LargeNumber(t *testing.T) {
	cases := []struct {
		input    any
		expected string
	}{
		{float64(16906691), "16906691"},
		{float32(12345), "12345"},
		{float64(16906691.5), "16906691.5"},
		{json.Number("16906691"), "16906691"},
		{json.Number("99.95"), "99.95"},
		{math.NaN(), "NULL"},
		{math.Inf(1), "NULL"},
		{math.Inf(-1), "NULL"},
	}

	for _, tc := range cases {
		if v := driver.FormatSQLValue(tc.input); v != tc.expected {
			t.Errorf("FormatSQLValue(%v) = %q, expected %q", tc.input, v, tc.expected)
		}
	}
}

func TestHasLimitClause(t *testing.T) {
	if !driver.HasLimitClause("SELECT * FROM users LIMIT 10") {
		t.Fatal("expected true for explicit LIMIT 10")
	}
	if !driver.HasLimitClause("SELECT * FROM users limit 50;") {
		t.Fatal("expected true for explicit limit 50")
	}
	if driver.HasLimitClause("SELECT id, credit_limit FROM accounts") {
		t.Fatal("expected false for column name containing limit")
	}
	if driver.HasLimitClause("SELECT * FROM users WHERE status = 'UNLIMITED'") {
		t.Fatal("expected false for string containing UNLIMITED")
	}
}

func TestStripLeadingComments(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"-- line comment\nSELECT 1", "SELECT 1"},
		{"/* block comment */ SELECT 2", "SELECT 2"},
		{"# mysql comment\nSELECT 3", "SELECT 3"},
		{"-- c1\n/* c2 */\n# c3\nSELECT 4", "SELECT 4"},
		{"   \n\t-- c1\n  SELECT 5", "SELECT 5"},
		{"SELECT 6", "SELECT 6"},
		{"-- only comment", ""},
	}
	for _, tc := range cases {
		out := driver.StripLeadingComments(tc.input)
		if out != tc.expected {
			t.Fatalf("StripLeadingComments(%q) = %q, want %q", tc.input, out, tc.expected)
		}
	}
}

func TestIsQueryStatement(t *testing.T) {
	queries := []string{
		"SELECT * FROM tbl",
		"-- comment\nSELECT * FROM tbl",
		"/* block */ SHOW TABLES",
		"(SELECT 1)",
		"((SELECT 1))",
		"DESCRIBE users",
		"EXPLAIN SELECT 1",
		"PRAGMA table_info('items')",
		"WITH cte AS (SELECT 1) SELECT * FROM cte",
		"VALUES (1, 2)",
		"TABLE my_table",
	}
	for _, q := range queries {
		if !driver.IsQueryStatement(q) {
			t.Fatalf("expected IsQueryStatement(%q) = true", q)
		}
	}

	mutations := []string{
		"INSERT INTO tbl VALUES (1)",
		"-- comment\nINSERT INTO tbl VALUES (1)",
		"UPDATE tbl SET a = 1",
		"DELETE FROM tbl",
		"DROP TABLE tbl",
		"ALTER TABLE tbl ADD COLUMN c INT",
		"CREATE TABLE tbl (id INT)",
		"TRUNCATE TABLE tbl",
	}
	for _, m := range mutations {
		if driver.IsQueryStatement(m) {
			t.Fatalf("expected IsQueryStatement(%q) = false", m)
		}
	}
}

