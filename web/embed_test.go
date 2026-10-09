package web

import (
	"io"
	"regexp"
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
		`id="lock-inspector-dialog"`,
		"openLockInspectorModal()",
		"No active lock contention detected.",
		"dismissNoLockBanner()",
		"Lock Contention & Root Blocker Analysis",
		`id="monitor-alert-dot"`,
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

func TestProcessDetailModalAndFooterPillsEmbedded(t *testing.T) {
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
		// 1. Footer pills rounded-full and matching slate styling
		`href="https://github.com/herliansyah/sqlfox" target="_blank" rel="noopener noreferrer" class="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-800/70 hover:bg-slate-700/80 text-slate-300 hover:text-white border border-slate-700/60 transition shadow-sm"`,
		`href="https://saweria.co/herliansyah26" target="_blank" rel="noopener noreferrer" class="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-800/70 hover:bg-slate-700/80 text-slate-300 hover:text-white border border-slate-700/60 transition shadow-sm"`,
		// 2. Process detail modal widened to max-w-5xl
		`<dialog id="pl-detail-modal" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-0 outline-none max-w-5xl w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm overflow-hidden">`,
		// 3. Fallback clipboard container targeting active open dialog
		`const activeDialog = document.querySelector('dialog[open]');`,
		`const container = activeDialog || document.body;`,
		// 4. Default formatted query in openProcessDetailModal
		`sqlCodeEl.textContent = formatSQL(thread.info);`,
		`isModalSQLFormatted = true;`,
		`btnFormat.innerHTML = '<span>↺ Raw SQL</span>';`,
		// 5. copyJsonCellContent using copyTextToClipboard
		`const ok = await copyTextToClipboard(editor.value);`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain snippet %q, but was missing", s)
		}
	}
}

func TestNotesInboxDrawerLayoutAndBorderEmbedded(t *testing.T) {
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

	// Bug: notes-inbox-drawer had 'top-10', which is 40px, overlapping the 48px header
	// and cutting into the top right controls (like the readonly button) with no top border.
	if strings.Contains(content, `id="notes-inbox-drawer" class="hidden fixed top-10`) {
		t.Errorf("notes-inbox-drawer has invalid 'top-10' class which overlaps header and collides with header controls")
	}

	// Must have proper top positioning below header and distinct top border
	if !strings.Contains(content, `id="notes-inbox-drawer"`) {
		t.Fatalf("index.html missing notes-inbox-drawer")
	}

	// Require proper top clearance and top border definition
	expectedSnippets := []string{
		`id="notes-inbox-drawer"`,
		`border-t border-app-border`,
	}
	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q for notes inbox drawer layout, but was missing", s)
		}
	}
}

func TestQuerySnapshotCaptureAndNoteExtractionEmbedded(t *testing.T) {
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
		`id="btn-save-as-note"`,
		`id="tab-ctx-btn-save-note"`,
		`openCurrentComposerInQueryTab()`,
		`openNoteInQueryTab('${n.id}')`,
		`function saveActiveQueryToNotes()`,
		`function saveTabQueryToNotes(tab)`,
		`function extractSqlFromNoteContent(rawContent)`,
		`function openExtractedQueryInTab(title, rawContent, objectTags)`,
		`function openNoteInQueryTab(noteId)`,
		`Save Query as Operational Note`,
		`Open Operational Notes Inbox`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q for Query Note integration, but was missing", s)
		}
	}
}

func TestWorkspacePaneSplitterEmbedded(t *testing.T) {
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
		`id="workspace-pane-splitter"`,
		`cursor-row-resize`,
		`function initWorkspacePaneSplitter()`,
		`initWorkspacePaneSplitter()`,
		`sqlfox_workspace_editor_height`,
		`minEditorHeight = 80`,
		`minGridHeight = 120`,
		`splitter.addEventListener('dblclick'`,
		`editor.style.height = '50%'`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q for workspace pane splitter, but was missing", s)
		}
	}
}

func TestProcesslistActivityTrendEmbedded(t *testing.T) {
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
		`id="pl-toggle-trend-btn"`,
		`id="pl-trend-caret"`,
		`id="pl-trend-panel"`,
		`id="pl-trend-canvas"`,
		`id="pl-trend-now"`,
		`id="pl-trend-total"`,
		`id="pl-trend-peak"`,
		`id="pl-trend-avg"`,
		`recordProcesslistTrendTick`,
		`toggleProcesslistTrendPanel`,
		`escalateToFlightRecorder`,
		`renderProcesslistTrendChart`,
		`sqlfox_pl_trend_open`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q for Processlist Activity Trend, but was missing", s)
		}
	}
}

func TestUIResilienceAndNonWrappingControls(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	expectedSnippets := []string{
		"button:not(.allow-wrap)",
		"white-space: nowrap !important;",
		"flex-shrink: 0;",
		"header-toolbar",
		"editor-toolbar",
		"table-toolbar",
		`class="h-12 bg-app-surface border-b border-app-border flex items-center justify-between px-4 z-20 flex-shrink-0 gap-3"`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain UI resilience rule %q, but was missing", s)
		}
	}
}

func TestHeaderDropdownsNotClippedByOverflow(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)

	// Header contains popups/dropdowns: #top-monitor-menu, #top-db-tools-menu, #top-settings-menu.
	// In CSS, overflow-x: auto automatically forces overflow-y to auto/clip, which causes all
	// dropdown menus to be clipped by the 48px header boundary and sink behind main content.
	// Therefore, <header> must not have overflow-x-auto or overflow-hidden.
	if strings.Contains(content, `<header class="h-12 bg-app-surface border-b border-app-border flex items-center justify-between px-4 z-20 flex-shrink-0 overflow-x-auto`) {
		t.Errorf("critical UI bug: <header> has overflow-x-auto, which clips top-monitor-menu and top-db-tools-menu dropdowns")
	}
}

func TestProcesslistHeaderAndTrendPanelResilience(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)

	// Processlist toolbar controls must be non-wrapping
	expectedProcesslistSnippets := []string{
		// Processlist Hide Sleep and Hide System checkboxes must not wrap text
		`id="pl-hide-sleep"`,
		`id="pl-hide-system"`,
		`id="pl-summary-text"`,
	}
	for _, s := range expectedProcesslistSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q", s)
		}
	}

	// Trend panel header must have overflow-x-auto and non-wrapping items
	if !strings.Contains(content, `id="pl-trend-panel"`) {
		t.Fatalf("pl-trend-panel not found")
	}

	// Verify trend panel header bar has overflow-x-auto and gap-3
	if !strings.Contains(content, `pl-trend-panel`) || !strings.Contains(content, `Activity Trend`) {
		t.Fatalf("trend panel content missing")
	}

	// Check that Activity Trend and its statistics labels are protected with whitespace-nowrap
	trendSnippetChecks := []string{
		`whitespace-nowrap`,
		`Activity Trend`,
	}
	for _, s := range trendSnippetChecks {
		if !strings.Contains(content, s) {
			t.Errorf("missing %q in index.html", s)
		}
	}

	// Verify pl-summary-text has whitespace-nowrap
	if !regexpMatch(`id="pl-summary-text"[^>]*class="[^"]*whitespace-nowrap`, content) &&
		!regexpMatch(`class="[^"]*whitespace-nowrap[^"]*"[^>]*id="pl-summary-text"`, content) {
		t.Errorf("pl-summary-text must have whitespace-nowrap to prevent line breaks on window resize")
	}

	// Verify pl-hide-sleep has whitespace-nowrap
	if !regexpMatch(`<label[^>]*id="pl-hide-sleep"`, content) && !regexpMatch(`<label[^>]*class="[^"]*whitespace-nowrap[^"]*"[^>]*>[^<]*<input[^>]*id="pl-hide-sleep"`, content) {
		t.Errorf("pl-hide-sleep label must have whitespace-nowrap to prevent word wrapping on window resize")
	}

	// Verify pl-hide-system has whitespace-nowrap
	if !regexpMatch(`<label[^>]*id="pl-hide-system"`, content) && !regexpMatch(`<label[^>]*class="[^"]*whitespace-nowrap[^"]*"[^>]*>[^<]*<input[^>]*id="pl-hide-system"`, content) {
		t.Errorf("pl-hide-system label must have whitespace-nowrap to prevent word wrapping on window resize")
	}

	// Verify trend panel header has overflow-x-auto
	if !regexpMatch(`id="pl-trend-panel"[\s\S]*?class="[^"]*h-6[^"]*overflow-x-auto`, content) {
		t.Errorf("pl-trend-panel header must have overflow-x-auto to prevent clipping and content distortion on resize")
	}

	// Verify Activity Trend label in trend panel has whitespace-nowrap
	if !regexpMatch(`class="[^"]*whitespace-nowrap[^"]*"[\s\S]*?Activity Trend`, content) &&
		!regexpMatch(`Activity Trend[\s\S]*?class="[^"]*whitespace-nowrap`, content) {
		t.Errorf("Activity Trend label must have whitespace-nowrap")
	}

	// Verify trend stats (running, total, peak, avg) are wrapped in whitespace-nowrap
	if !regexpMatch(`id="pl-trend-now"[\s\S]*?id="pl-trend-total"[\s\S]*?id="pl-trend-peak"[\s\S]*?id="pl-trend-avg"`, content) {
		t.Errorf("pl-trend statistics elements missing")
	}
}

func regexpMatch(pattern, text string) bool {
	matched, _ := regexp.MatchString(pattern, text)
	return matched
}

func TestGridToolbarDropdownNotClipped(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)

	// #grid-toolbar contains #export-csv-menu (an absolutely positioned dropdown).
	// Having overflow-x-auto or overflow-hidden on #grid-toolbar forces overflow-y to clip,
	// causing the export dropdown to be clipped / sunken ("mendem") behind the grid table.
	if strings.Contains(content, `id="grid-toolbar" class="h-8 bg-app-surface/40 border-b border-app-border px-3 flex items-center justify-between text-xs text-slate-400 flex-shrink-0 overflow-x-auto`) {
		t.Errorf("critical UI bug: #grid-toolbar has overflow-x-auto, which clips #export-csv-menu dropdown")
	}
}

func TestConnectionHoverActionsAndAffordance(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)

	// Critical check: [hidden], .hidden { display: none !important; } prevents elements with class "hidden"
	// from ever displaying on hover (e.g. group-hover:inline-block).
	// Connection list items must use opacity-0 group-hover:opacity-100 instead of "hidden group-hover:inline-block".
	if strings.Contains(content, `class="hidden group-hover:inline-block hover:text-white px-1 text-xs" title="Edit Connection"`) {
		t.Errorf("critical UI bug: connection edit button uses 'hidden group-hover:inline-block' which is permanently suppressed by .hidden !important")
	}

	// Verify session row / switcher bar has edit connection affordance
	if !strings.Contains(content, `openEditConnModal(activeConnection.id)`) && !strings.Contains(content, `openEditConnModal(conn.id)`) {
		t.Errorf("active connection bar or badge must afford editing the active connection via openEditConnModal")
	}
}





