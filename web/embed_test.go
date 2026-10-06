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
		"#results-table.table-empty",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q, but was missing", snippet)
		}
	}

	// Verify global min-width: 100% is NOT applied directly to #results-table (only to empty state)
	if strings.Contains(content, "#results-table { min-width: 100%; }") {
		t.Errorf("expected #results-table to NOT have unconditional min-width: 100%%, allowing natural column sizing when data is present")
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
		"isProcesslist: !!t.isProcesslist",
		"isFlightRecorder: !!t.isFlightRecorder",
		"isCluster: !!t.isCluster",
		"isVariables: !!t.isVariables",
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

func TestCopilotAndDrawerNonPushingOverlayEmbedded(t *testing.T) {
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
		"ai-sidebar-overlay",
		"position: absolute !important;",
		"z-index: 30 !important;",
		`id="audit-drawer" class="hidden h-72 bg-slate-950 border-t border-app-border flex flex-col z-20 shadow-2xl absolute bottom-0 left-0 right-0"`,
		`<main class="flex-1 flex flex-col overflow-hidden bg-app-bg min-w-0 relative" style="min-width: 480px;">`,
		"sidebar.classList.add('absolute', 'top-0', 'right-0', 'bottom-0', 'z-30', 'shadow-2xl', 'ai-sidebar-overlay')",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for non-pushing copilot overlay layout, but was missing", snippet)
		}
	}
}

func TestTabGridDataIsolationEmbedded(t *testing.T) {
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
		"function clearGridToInitialState()",
		"clearGridToInitialState();",
		"activeTab.rawQueryError = null;",
		"renderQueryErrorBanner(nextTab.rawQueryError.sql, nextTab.rawQueryError.message);",
		"activeTab.queryStats = `${data.row_count} rows (${data.execution_ms}ms)`;",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for tab grid data isolation, but was missing", snippet)
		}
	}
}

func TestLockContentionAndRootBlockerInspectorEmbedded(t *testing.T) {
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
		`id="btn-open-lock-inspector"`,
		"openLockInspector()",
		`id="lock-blocker-badge"`,
		`id="pl-no-lock-banner"`,
		"No active lock contention detected.",
		"dismissNoLockBanner()",
		"Lock Contention & Root Blocker Analysis",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for lock contention inspector, but was missing", snippet)
		}
	}
}



