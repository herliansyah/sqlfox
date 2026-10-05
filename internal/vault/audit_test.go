package vault

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditLogPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vault-audit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "vault.db")
	v, err := OpenVault(dbPath, "test-master-password")
	if err != nil {
		t.Fatalf("failed to open vault: %v", err)
	}
	defer v.Close()

	sessID := "sess-123"

	// Record 3 logs with different sources
	_ = v.RecordAuditLog(&AuditLogRecord{
		SessionID:    sessID,
		ConnectionID: sessID,
		QuerySQL:     "SELECT * FROM users",
		Source:       "editor",
		DurationMs:   12,
		RowsAffected: 10,
		ExecutedAt:   time.Now().Add(-2 * time.Minute),
	})

	_ = v.RecordAuditLog(&AuditLogRecord{
		SessionID:    sessID,
		ConnectionID: sessID,
		QuerySQL:     "UPDATE users SET status = 'active' WHERE id = 1",
		Source:       "grid_cell_edit",
		DurationMs:   25,
		RowsAffected: 1,
		ExecutedAt:   time.Now().Add(-1 * time.Minute),
	})

	_ = v.RecordAuditLog(&AuditLogRecord{
		SessionID:    sessID,
		ConnectionID: sessID,
		QuerySQL:     "SELECT COUNT(*) FROM orders",
		DurationMs:   8,
		RowsAffected: 1,
		Error:        "",
		ExecutedAt:   time.Now(),
	})

	// List all logs
	logs, total, err := v.ListAuditLogs(sessID, 10, 0, "", "")
	if err != nil {
		t.Fatalf("ListAuditLogs failed: %v", err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("expected 3 logs, got total=%d len=%d", total, len(logs))
	}

	// Verify default source is "editor" for third record
	for _, l := range logs {
		if l.QuerySQL == "UPDATE users SET status = 'active' WHERE id = 1" && l.Source != "grid_cell_edit" {
			t.Fatalf("expected source grid_cell_edit, got %s", l.Source)
		}
		if l.QuerySQL == "SELECT COUNT(*) FROM orders" && l.Source != "editor" {
			t.Fatalf("expected default source editor, got %s", l.Source)
		}
	}

	// Test search filter
	logsFiltered, totalFiltered, err := v.ListAuditLogs(sessID, 10, 0, "orders", "")
	if err != nil {
		t.Fatalf("ListAuditLogs with search failed: %v", err)
	}
	if totalFiltered != 1 || len(logsFiltered) != 1 {
		t.Fatalf("expected 1 log with 'orders', got total=%d", totalFiltered)
	}
	if logsFiltered[0].QuerySQL != "SELECT COUNT(*) FROM orders" {
		t.Fatalf("unexpected query: %s", logsFiltered[0].QuerySQL)
	}

	// Test source filter
	gridLogs, gridTotal, err := v.ListAuditLogs(sessID, 10, 0, "", "grid_cell_edit")
	if err != nil {
		t.Fatalf("ListAuditLogs with source failed: %v", err)
	}
	if gridTotal != 1 || len(gridLogs) != 1 || gridLogs[0].Source != "grid_cell_edit" {
		t.Fatalf("expected 1 grid_cell_edit log, got total=%d", gridTotal)
	}

	// Test ClearAuditLogs
	if err := v.ClearAuditLogs(sessID); err != nil {
		t.Fatalf("ClearAuditLogs failed: %v", err)
	}

	afterLogs, afterTotal, err := v.ListAuditLogs(sessID, 10, 0, "", "")
	if err != nil {
		t.Fatalf("ListAuditLogs after clear failed: %v", err)
	}
	if afterTotal != 0 || len(afterLogs) != 0 {
		t.Fatalf("expected 0 logs after clear, got %d", afterTotal)
	}
}
