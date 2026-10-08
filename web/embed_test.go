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

func TestDocumentationLinkEmbedded(t *testing.T) {
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
		`https://github.com/herliansyah/sqlfox/blob/main/docs/USER_GUIDE.md`,
		`title="Open SqlFox Documentation on GitHub (Online Manual)"`,
		`title="Open complete User Guide & DBA Playbook on GitHub"`,
		`Open Documentation (GitHub)`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for documentation link, but was missing", snippet)
		}
	}
}

func TestERDCanvasPrimaryAndForeignKeyEmbedded(t *testing.T) {
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
		"c => c.is_primary_key || c.key === 'PRI'",
		"erdRenderCards()",
		"erdRenderLines()",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain %q for ERD canvas, but was missing", snippet)
		}
	}
}

func TestCrossDatabaseGridTargetSchemaPrecedenceEmbedded(t *testing.T) {
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

	// Verify that activeGridDB takes precedence over activeTab.database in cell-edit and grid operations
	expectedPatterns := []string{
		"const dbName = activeGridDB || (activeTab && activeTab.database) || '';",
		"multiDBCatalogCache[detectedDB.toLowerCase()] = s",
		"async function loadTableRows(tableName, targetDb = null)",
		"const dbName = targetDb || (activeTab && activeTab.isTableViewer && activeTab.database) || activeGridDB || (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';",
		"await loadTableRows(activeGridTable, activeGridDB);",
	}

	for _, p := range expectedPatterns {
		if !strings.Contains(content, p) {
			t.Errorf("expected index.html to contain cross-database precedence pattern %q, but was missing", p)
		}
	}

	// Verify old anti-pattern where stale activeTab.database took precedence over activeGridDB is gone in cell edit
	if strings.Contains(content, "const dbName = (activeTab && activeTab.database) || activeGridDB || '';") {
		t.Errorf("found anti-pattern 'const dbName = (activeTab && activeTab.database) || activeGridDB || '';' which breaks cross-database inline editing")
	}
}

func TestProcesslistDOMContainmentEmbedded(t *testing.T) {
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

	// Verify that pl-threads-section is strictly contained inside processlist-view-container
	plStart := strings.Index(content, `id="processlist-view-container"`)
	if plStart == -1 {
		t.Fatal("processlist-view-container element missing in index.html")
	}

	clStart := strings.Index(content, `id="cluster-view-container"`)
	if clStart == -1 {
		t.Fatal("cluster-view-container element missing in index.html")
	}

	plChunk := content[plStart:clStart]
	threadsPos := strings.Index(plChunk, `id="pl-threads-section"`)
	if threadsPos == -1 {
		t.Fatal("pl-threads-section is missing or outside processlist-view-container chunk")
	}

	beforeThreads := plChunk[:threadsPos]
	opens := strings.Count(beforeThreads, "<div")
	closes := strings.Count(beforeThreads, "</div")
	if opens-closes <= 0 {
		t.Fatalf("DOM containment error: processlist-view-container was closed before pl-threads-section (open: %d, close: %d)", opens, closes)
	}
}

func TestDeadlockCardsResponsiveGridEmbedded(t *testing.T) {
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

	// Verify that renderDeadlockUI enforces grid constraint and text wrapping
	expectedSnippets := []string{
		`grid grid-cols-1 md:grid-cols-2 gap-4 w-full`,
		`min-w-0 overflow-hidden shadow`,
		`block break-all whitespace-normal`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain deadlock card layout snippet %q, but was missing", s)
		}
	}
}

func TestTabContextMenuAndCloseGuardEmbedded(t *testing.T) {
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
		`id="tab-context-menu"`,
		`id="tab-ctx-btn-close"`,
		`id="tab-ctx-btn-close-others"`,
		`id="tab-ctx-btn-close-right"`,
		`id="tab-ctx-btn-close-all"`,
		`id="tab-ctx-btn-duplicate"`,
		`id="tab-ctx-btn-rename"`,
		`function showTabContextMenu(e, tabId)`,
		`function closeTabContextMenu()`,
		`async function tabContextMenuAction(action)`,
		`async function closeTab(tabId)`,
		`async function closeOtherTabs(keepTabId)`,
		`async function closeTabsToTheRight(fromTabId)`,
		`async function closeAllTabs()`,
		`function duplicateTab(tabId)`,
		`async function renameTab(tabId)`,
		`function isTabDirtyOrNonEmpty(tab)`,
		`function showNativePrompt(message, defaultValue`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain tab context menu snippet %q, but was missing", s)
		}
	}
}

func TestMasterPINMaskingAndToggleEmbedded(t *testing.T) {
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
		`id="master-pin-badge"`,
		`id="master-pin-copy-btn"`,
		`id="master-pin-toggle-btn"`,
		`id="master-pin-eye-icon"`,
		"function toggleMasterPINVisibility(",
		"function updateMasterPINDisplay()",
		"cachedMasterPIN",
		"masterPINAutoMaskTimer",
		"Master PIN copied to clipboard!",
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain master pin security snippet %q, but was missing", s)
		}
	}

	// Verify plaintext PIN injection into copy alert popup is removed
	if strings.Contains(content, "Master PIN '${pin}' copied to clipboard!") {
		t.Errorf("expected raw PIN to NOT be exposed in copy alert popup")
	}
}


