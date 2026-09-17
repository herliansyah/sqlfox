package driver_test

import (
	"database/sql"
	"testing"

	"sqlfox/internal/driver"
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
