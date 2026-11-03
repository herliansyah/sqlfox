package mysql

import (
	"context"
	"database/sql"
	"testing"
)

func TestMySQLCancelQueryFallback(t *testing.T) {
	s := &MySQLSession{
		queryThreads: map[string]int64{
			"active#1": 9999,
		},
	}
	ctx := context.Background()
	// Should gracefully resolve fallback thread ID and return nil when db and dsn are nil
	if err := s.CancelQuery(ctx, "active"); err != nil {
		t.Fatalf("unexpected CancelQuery error: %v", err)
	}

	// Verify queryThreads mapping was cleared
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queryThreads) != 0 {
		t.Errorf("expected queryThreads to be cleared, got %v", s.queryThreads)
	}
}

func TestMySQLCancelQuery_DoesNotKillUnrelatedQueryOnSpecificKeyMiss(t *testing.T) {
	s := &MySQLSession{
		queryThreads: map[string]int64{
			"active#2": 8888,
		},
	}
	ctx := context.Background()
	// Cancelling "active#1" should NOT cancel or delete "active#2"
	if err := s.CancelQuery(ctx, "active#1"); err != nil {
		t.Fatalf("unexpected CancelQuery error: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.queryThreads["active#2"]; !ok {
		t.Errorf("active#2 was wrongly killed/deleted when active#1 was cancelled!")
	}
}

func TestMySQLCancelQuery_MultipleQueriesCancelledOnFallback(t *testing.T) {
	s := &MySQLSession{
		queryThreads: map[string]int64{
			"active#1": 1001,
			"active#2": 1002,
		},
	}
	ctx := context.Background()
	if err := s.CancelQuery(ctx, "active"); err != nil {
		t.Fatalf("unexpected CancelQuery error: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queryThreads) != 0 {
		t.Errorf("expected all queryThreads to be cleared on fallback 'active', got: %v", s.queryThreads)
	}
}

func TestMySQLCancelQuery_RetainsTargetOnFailure(t *testing.T) {
	db, err := sql.Open("mysql", "invalid:dsn@tcp(127.0.0.1:9)/db")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	// Close db so any ExecContext on it fails immediately
	db.Close()

	s := &MySQLSession{
		db: db,
		queryThreads: map[string]int64{
			"active#1": 1001,
		},
	}

	ctx := context.Background()
	err = s.CancelQuery(ctx, "active#1")
	if err == nil {
		t.Fatal("expected CancelQuery to fail on closed db, got nil")
	}

	// Verify that active#1 thread mapping was NOT deleted because cancellation failed
	s.mu.Lock()
	defer s.mu.Unlock()
	if tid, ok := s.queryThreads["active#1"]; !ok || tid != 1001 {
		t.Fatalf("expected queryThreads['active#1'] to be retained after failure, got: %v", s.queryThreads)
	}
}

func TestMySQLCancelQuery_PrefixMatching(t *testing.T) {
	s := &MySQLSession{
		queryThreads: map[string]int64{
			"tab-abc#1": 5555,
		},
	}
	ctx := context.Background()
	// Cancelling with base query ID "tab-abc" should find "tab-abc#1"
	if err := s.CancelQuery(ctx, "tab-abc"); err != nil {
		t.Fatalf("unexpected CancelQuery error: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queryThreads) != 0 {
		t.Errorf("expected tab-abc#1 to be cancelled and cleared, got %v", s.queryThreads)
	}
}

