package web

import (
	"io"
	"strings"
	"testing"
)

func TestTypeColoredGridEmbedded(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	expectedSnippets := []string{
		"function getGridTypeCategory(typeStr)",
		"function getGridTypeStyle(typeStr)",
		"text-emerald-400",
		"text-purple-400",
		"text-rose-400",
		"text-amber-400",
		"getGridTypeStyle(colType)",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q, but was missing", snippet)
		}
	}
}

func TestNaturalColumnSizingEmbedded(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	expectedSnippets := []string{
		`<table id="results-table" class="border-collapse text-left font-mono text-xs w-auto">`,
		"min-w-[90px] max-w-md whitespace-nowrap",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q, but was missing", snippet)
		}
	}
}

func TestSessionTabStatePersistenceEmbedded(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	expectedSnippets := []string{
		"function saveTabsState()",
		"function restoreTabsState(connId)",
		"function scheduleSaveTabs()",
		"function getTabsStorageKey(",
		"sqlfox_tabs_",
		"window.addEventListener('beforeunload'",
		"isQueryExecuting",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for session tab persistence, but was missing", snippet)
		}
	}
}

func TestAuditSourceAttributionEmbedded(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	expectedSnippets := []string{
		`id="audit-source-filter"`,
		"function onAuditSourceChange(val)",
		"function getAuditSourceBadge(src)",
		"grid_cell_edit",
		"grid_insert",
		"grid_delete",
		"script_runner",
		"csv_import",
		"mock_data",
		"sync_transfer",
		"schema_sync",
		"sourceParam",
		`headers = ['Executed At', 'Source'`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for audit source attribution, but was missing", snippet)
		}
	}
}

func TestDatabaseStorageAndHealthInspectorEmbedded(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	expectedSnippets := []string{
		"Database Storage &amp; Health Inspector",
		"dbdiag-level-databases",
		"dbdiag-level-tables",
		"loadAllDatabasesDiagnostics",
		"inspectDatabaseTables",
		"openTableHealthFix",
		"Table Health Advisor",
		"/api/session/${activeConnection.id}/diagnostics/databases",
		"dbdiag-toggle-system-dbs",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for database storage & health inspector, but was missing", snippet)
		}
	}
}
