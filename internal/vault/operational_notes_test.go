package vault

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOperationalNotesPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vault-operational-notes-test-*")
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

	now := time.Now().Truncate(time.Second)
	reminderTime := now.Add(2 * time.Hour)

	// 1. Create Note
	note := &OperationalNote{
		Title:      "Slow Query Analysis - Orders Table",
		Content:    "SELECT * FROM orders WHERE status = 'pending' AND total > 1000",
		SideNote:   "Check why index idx_orders_status was ignored in EXPLAIN plan",
		Status:     "inbox",
		IsPinned:   true,
		CapturedAt: now,
		ReminderAt: &reminderTime,
		ObjectTags: []string{"db://conn-1/sales_db/orders"},
		UserTags:   []string{"#slow-query", "#incident"},
	}

	if err := v.CreateOperationalNote(note); err != nil {
		t.Fatalf("failed to create operational note: %v", err)
	}
	if note.ID == "" {
		t.Fatalf("expected non-empty note ID after create")
	}

	// 2. Get Note
	fetched, err := v.GetOperationalNote(note.ID)
	if err != nil {
		t.Fatalf("failed to get operational note: %v", err)
	}
	if fetched.Title != note.Title || fetched.SideNote != note.SideNote || !fetched.IsPinned {
		t.Fatalf("note content mismatch: %+v", fetched)
	}
	if len(fetched.ObjectTags) != 1 || fetched.ObjectTags[0] != "db://conn-1/sales_db/orders" {
		t.Fatalf("object tags mismatch: %v", fetched.ObjectTags)
	}

	// 3. Update Note
	fetched.Title = "Updated Slow Query Title"
	fetched.Status = "resolved"
	fetched.IsPinned = false
	if err := v.UpdateOperationalNote(fetched); err != nil {
		t.Fatalf("failed to update operational note: %v", err)
	}

	updated, err := v.GetOperationalNote(note.ID)
	if err != nil {
		t.Fatalf("failed to get updated note: %v", err)
	}
	if updated.Title != "Updated Slow Query Title" || updated.Status != "resolved" || updated.IsPinned {
		t.Fatalf("updated note mismatch: %+v", updated)
	}

	// 4. Create another note for filtering
	pastReminder := now.Add(-10 * time.Minute)
	dueNote := &OperationalNote{
		Title:      "Follow up on missing index",
		Content:    "ALTER TABLE users ADD INDEX idx_email(email)",
		SideNote:   "Follow up with DBA team",
		Status:     "inbox",
		IsPinned:   false,
		CapturedAt: now,
		ReminderAt: &pastReminder,
		ObjectTags: []string{"db://conn-1/users_db/users"},
		UserTags:   []string{"#dba"},
	}
	if err := v.CreateOperationalNote(dueNote); err != nil {
		t.Fatalf("failed to create due note: %v", err)
	}

	// 5. Test ListOperationalNotes with filters
	// 5a. Filter by status 'inbox'
	inboxNotes, err := v.ListOperationalNotes(OperationalNoteFilter{Status: "inbox"})
	if err != nil {
		t.Fatalf("failed to list inbox notes: %v", err)
	}
	if len(inboxNotes) != 1 || inboxNotes[0].ID != dueNote.ID {
		t.Fatalf("expected 1 inbox note, got %d", len(inboxNotes))
	}

	// 5b. Filter by object tag
	ordersNotes, err := v.ListOperationalNotes(OperationalNoteFilter{ObjectTag: "db://conn-1/sales_db/orders"})
	if err != nil {
		t.Fatalf("failed to list notes by object tag: %v", err)
	}
	if len(ordersNotes) != 1 || ordersNotes[0].ID != note.ID {
		t.Fatalf("expected 1 note for orders tag, got %d", len(ordersNotes))
	}

	// 5c. Filter by search query
	searchNotes, err := v.ListOperationalNotes(OperationalNoteFilter{Search: "missing index"})
	if err != nil {
		t.Fatalf("failed to search notes: %v", err)
	}
	if len(searchNotes) != 1 || searchNotes[0].ID != dueNote.ID {
		t.Fatalf("expected 1 note matching search, got %d", len(searchNotes))
	}

	// 6. Test ListDueReminders
	dueList, err := v.ListDueReminders(time.Now())
	if err != nil {
		t.Fatalf("failed to list due reminders: %v", err)
	}
	if len(dueList) != 1 || dueList[0].ID != dueNote.ID {
		t.Fatalf("expected 1 due reminder, got %d", len(dueList))
	}

	// 7. Test SnoozeReminder
	if err := v.SnoozeReminder(dueNote.ID, 30*time.Minute); err != nil {
		t.Fatalf("failed to snooze reminder: %v", err)
	}
	// Verify no longer due right now
	dueAfterSnooze, err := v.ListDueReminders(time.Now())
	if err != nil {
		t.Fatalf("failed to list due reminders after snooze: %v", err)
	}
	if len(dueAfterSnooze) != 0 {
		t.Fatalf("expected 0 due reminders after snooze, got %d", len(dueAfterSnooze))
	}

	// 8. Delete note
	if err := v.DeleteOperationalNote(note.ID); err != nil {
		t.Fatalf("failed to delete note: %v", err)
	}
	_, err = v.GetOperationalNote(note.ID)
	if err != ErrOperationalNoteNotFound {
		t.Fatalf("expected ErrOperationalNoteNotFound, got: %v", err)
	}
}
