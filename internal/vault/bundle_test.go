package vault

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNotesBundle_ExportAndImport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlfox-bundle-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath1 := filepath.Join(tmpDir, "vault1.db")
	v1, err := OpenVault(dbPath1, "test-master-pin")
	if err != nil {
		t.Fatalf("OpenVault v1 failed: %v", err)
	}
	defer v1.Close()

	// Seed Operational Note
	opNote := &OperationalNote{
		ID:         "note-123",
		Title:      "Slow Query Checkout",
		Content:    "SELECT * FROM checkout WHERE duration > 5000;",
		SideNote:   "Check missing compound index on (status, created_at)",
		Status:     "inbox",
		IsPinned:   true,
		CapturedAt: time.Now(),
		ObjectTags: []string{"db://conn-1/shop/checkout"},
		UserTags:   []string{"#slow-query", "#incident"},
	}
	if err := v1.CreateOperationalNote(opNote); err != nil {
		t.Fatalf("CreateOperationalNote failed: %v", err)
	}

	// Seed Table Note
	tbNote := &TableNote{
		ConnectionID: "conn-1",
		DatabaseName: "shop",
		TableName:    "orders",
		Content:      "# Orders Table\nStores all completed user checkouts.",
	}
	if err := v1.SaveTableNote(tbNote); err != nil {
		t.Fatalf("SaveTableNote failed: %v", err)
	}

	// Seed Snippet
	snip := &Snippet{
		ID:         "snip-456",
		Title:      "Monthly Revenue Breakdown",
		SQLContent: "SELECT strftime('%Y-%m', created_at) AS month, SUM(total) FROM orders GROUP BY 1;",
		Tags:       "analytics,sales",
	}
	if err := v1.SaveSnippet(snip); err != nil {
		t.Fatalf("SaveSnippet failed: %v", err)
	}

	// 1. Export Bundle
	zipBytes, err := ExportNotesBundle(v1)
	if err != nil {
		t.Fatalf("ExportNotesBundle failed: %v", err)
	}
	if len(zipBytes) == 0 {
		t.Fatalf("expected non-empty zip bytes")
	}

	// 2. Import into a fresh vault
	dbPath2 := filepath.Join(tmpDir, "vault2.db")
	v2, err := OpenVault(dbPath2, "test-master-pin-2")
	if err != nil {
		t.Fatalf("OpenVault v2 failed: %v", err)
	}
	defer v2.Close()

	importedCount, err := ImportNotesBundle(v2, zipBytes)
	if err != nil {
		t.Fatalf("ImportNotesBundle failed: %v", err)
	}
	if importedCount < 3 {
		t.Fatalf("expected at least 3 imported items, got %d", importedCount)
	}

	// 3. Verify Operational Note
	importedOpNote, err := v2.GetOperationalNote("note-123")
	if err != nil {
		t.Fatalf("GetOperationalNote failed: %v", err)
	}
	if importedOpNote.Title != opNote.Title {
		t.Errorf("title mismatch: got %q, want %q", importedOpNote.Title, opNote.Title)
	}
	if importedOpNote.SideNote != opNote.SideNote {
		t.Errorf("side note mismatch: got %q, want %q", importedOpNote.SideNote, opNote.SideNote)
	}
	if importedOpNote.Content != opNote.Content {
		t.Errorf("content mismatch: got %q, want %q", importedOpNote.Content, opNote.Content)
	}

	// 4. Verify Table Note
	importedTbNote, err := v2.GetTableNote("conn-1", "shop", "orders")
	if err != nil {
		t.Fatalf("GetTableNote failed: %v", err)
	}
	if importedTbNote.Content != tbNote.Content {
		t.Errorf("table note content mismatch: got %q, want %q", importedTbNote.Content, tbNote.Content)
	}

	// 5. Verify Snippet
	snippets, err := v2.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	var foundSnippet *Snippet
	for _, s := range snippets {
		if s.ID == "snip-456" {
			foundSnippet = s
			break
		}
	}
	if foundSnippet == nil {
		t.Fatalf("snippet snip-456 not found in imported vault")
	}
	if foundSnippet.Title != snip.Title {
		t.Errorf("snippet title mismatch: got %q, want %q", foundSnippet.Title, snip.Title)
	}
	if foundSnippet.SQLContent != snip.SQLContent {
		t.Errorf("snippet sql content mismatch: got %q, want %q", foundSnippet.SQLContent, snip.SQLContent)
	}
}

func TestNotesBundle_TableNoteIdentifiersRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	v1, err := OpenVault(filepath.Join(tmpDir, "a.db"), "pin")
	if err != nil {
		t.Fatal(err)
	}
	defer v1.Close()
	v2, err := OpenVault(filepath.Join(tmpDir, "b.db"), "pin")
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()

	names := [][3]string{
		{"conn-1", "my_app", "order_items"},
		{"conn-1", "my_app", "Order_Items"},
		{"conn-1", "", "a/b"},
		{"conn-1", "db", ".."},
		{"conn-1", "db", "pengguna_ü"},
	}
	for _, n := range names {
		if err := v1.SaveTableNote(&TableNote{ConnectionID: n[0], DatabaseName: n[1], TableName: n[2], Content: "note " + n[2]}); err != nil {
			t.Fatalf("SaveTableNote %v: %v", n, err)
		}
	}
	zipBytes, err := ExportNotesBundle(v1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportNotesBundle(v2, zipBytes); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		got, err := v2.GetTableNote(n[0], n[1], n[2])
		if err != nil {
			t.Fatalf("table note %v not restored: %v", n, err)
		}
		if got.Content != "note "+n[2] {
			t.Errorf("table note %v content = %q", n, got.Content)
		}
	}
}
