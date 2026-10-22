package mysql

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestGetProcesslist_FallbackWhenPerformanceSchemaEmpty(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	// Attach performance_schema with empty threads table (simulates performance_schema = OFF)
	if _, err := db.Exec(`ATTACH ':memory:' AS performance_schema;`); err != nil {
		t.Fatalf("failed to attach performance_schema: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE performance_schema.threads (
			PROCESSLIST_ID INTEGER,
			PROCESSLIST_USER TEXT,
			PROCESSLIST_HOST TEXT,
			PROCESSLIST_DB TEXT,
			PROCESSLIST_COMMAND TEXT,
			PROCESSLIST_TIME INTEGER,
			PROCESSLIST_STATE TEXT,
			PROCESSLIST_INFO TEXT
		);
	`); err != nil {
		t.Fatalf("failed to create performance_schema.threads: %v", err)
	}

	// Attach information_schema with populated PROCESSLIST table
	if _, err := db.Exec(`ATTACH ':memory:' AS information_schema;`); err != nil {
		t.Fatalf("failed to attach information_schema: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE information_schema.PROCESSLIST (
			ID INTEGER,
			USER TEXT,
			HOST TEXT,
			DB TEXT,
			COMMAND TEXT,
			TIME INTEGER,
			STATE TEXT,
			INFO TEXT
		);
		INSERT INTO information_schema.PROCESSLIST (ID, USER, HOST, DB, COMMAND, TIME, STATE, INFO) VALUES
		(101, 'app_user', '127.0.0.1:5001', 'main', 'Query', 10, 'executing', 'SELECT * FROM users');
	`); err != nil {
		t.Fatalf("failed to setup information_schema.PROCESSLIST: %v", err)
	}

	// Create dummy sys schema to avoid syntax error on sys.innodb_lock_waits
	if _, err := db.Exec(`ATTACH ':memory:' AS sys;`); err != nil {
		t.Fatalf("failed to attach sys: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE sys.innodb_lock_waits (
			waiting_pid INTEGER,
			waiting_query TEXT,
			wait_age_secs INTEGER,
			blocking_pid INTEGER,
			blocking_query TEXT,
			locked_table TEXT,
			waiting_lock_mode TEXT
		);
	`); err != nil {
		t.Fatalf("failed to setup sys.innodb_lock_waits: %v", err)
	}

	session := &MySQLSession{
		db: db,
	}

	res, err := session.GetProcesslist(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Threads) == 0 {
		t.Fatalf("expected threads from information_schema fallback when performance_schema is empty/disabled, got 0")
	}
	if res.Threads[0].ID != 101 {
		t.Errorf("expected thread ID 101, got %d", res.Threads[0].ID)
	}
}

func TestMySQLKillProcess_Validation(t *testing.T) {
	s := &MySQLSession{}
	ctx := context.Background()

	if err := s.KillProcess(ctx, 0, true); err == nil {
		t.Fatalf("expected error for thread ID 0, got nil")
	}
	if err := s.KillProcess(ctx, -1, false); err == nil {
		t.Fatalf("expected error for thread ID -1, got nil")
	}

	// Without db or dsn, should return error
	if err := s.KillProcess(ctx, 100, true); err == nil {
		t.Fatalf("expected error on unconfigured session, got nil")
	}
}
