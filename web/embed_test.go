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
		// 1. Clean footer version launcher
		`id="footer-version-btn" onclick="openChangelogDialog()"`,
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
		// 6. copyAdvisorDDL using copyTextToClipboard with visual toast and button feedback
		`async function copyAdvisorDDL(btn) {`,
		`onclick="copyAdvisorDDL(this)"`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain snippet %q, but was missing", s)
		}
	}

	if strings.Contains(content, "saweria.co") {
		t.Errorf("expected index.html to NOT contain saweria link")
	}

	footerIdx := strings.Index(content, "<footer")
	footerEndIdx := strings.Index(content, "</footer>")
	if footerIdx != -1 && footerEndIdx != -1 {
		footerHTML := content[footerIdx:footerEndIdx]
		if strings.Contains(footerHTML, "Created by") {
			t.Errorf("expected status bar footer to NOT contain 'Created by'")
		}
		if strings.Contains(footerHTML, "href=\"https://github.com/herliansyah/sqlfox\"") {
			t.Errorf("expected status bar footer to NOT contain GitHub pill button")
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
		`class="h-12 bg-app-surface border-b border-app-border flex items-center justify-between px-4 z-40 flex-shrink-0 gap-3"`,
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

	// Dropdown menus must have solid background so underlying tabs/editors do not bleed through transparently.
	if !strings.Contains(content, "#top-monitor-menu") || !strings.Contains(content, "background-color: #0f172a !important;") {
		t.Errorf("expected solid opaque background (#0f172a) for header dropdown menus in index.html")
	}

	// Header must have higher z-index (z-40) than workspace toolbars/splitters (z-20) and overlay panels (z-30)
	// so that top header dropdowns (Monitor, DB Tools, Settings) always render on top of the workspace.
	if !strings.Contains(content, `<header class="h-12 bg-app-surface border-b border-app-border flex items-center justify-between px-4 z-40 flex-shrink-0 gap-3">`) {
		t.Errorf("expected <header> to have z-40 so header dropdown menus float over editor, grid toolbars, and splitters")
	}

	// top-monitor-menu must have adequate width and non-shrinking badge layout so dynamic blocker badges don't spill
	if !strings.Contains(content, `id="top-monitor-menu" class="hidden absolute right-0 mt-1 w-72`) {
		t.Errorf("expected #top-monitor-menu to have w-72 to fit dynamic lock badges cleanly")
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

	// Verify processlist toolbar left control group has min-w-max to prevent overlapping right summary controls on narrow viewports
	if !regexpMatch(`class="[^"]*processlist-toolbar[^"]*"[\s\S]*?<div[^>]*class="[^"]*min-w-max`, content) {
		t.Errorf("processlist-toolbar left control group must have min-w-max to prevent overlapping right summary controls on resize")
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

func TestChangelogCheckForUpdatesButtonEmbedded(t *testing.T) {
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

	if !strings.Contains(content, `id="btn-changelog-check-update"`) || !strings.Contains(content, `checkOrOpenUpdate(true)`) {
		t.Errorf("changelog modal must have a manual Check for Updates button")
	}
}

func TestUIGlitchAndResilienceGuards(t *testing.T) {
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

	// 1. Unified context menu cleanup & mutual exclusion
	if !strings.Contains(content, "function closeAllContextMenus()") {
		t.Errorf("expected closeAllContextMenus() to exist in index.html")
	}
	if !strings.Contains(content, "id=\"grid-context-menu\"") || !strings.Contains(content, "max-h-[85vh] overflow-y-auto") {
		t.Errorf("expected grid-context-menu to have max-h-[85vh] and overflow-y-auto to prevent offscreen overflow")
	}

	// 2. Tab container and item non-shrinking resilience (ADR 0048)
	if !strings.Contains(content, `id="tabs-container" class="flex items-center space-x-1 shrink-0"`) {
		t.Errorf("expected #tabs-container to have shrink-0 to prevent flex compression when multiple tabs exist")
	}
	if !strings.Contains(content, "tab-label shrink-0 whitespace-nowrap") {
		t.Errorf("expected tab items in renderTabs() to enforce tab-label shrink-0 whitespace-nowrap")
	}

	// 3. Dense modal dialogs height resilience
	dialogs := []string{
		`id="structure-sync-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-4xl w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
		`id="data-sync-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-4xl w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
		`id="data-transfer-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-2xl w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
		`id="table-creator-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-4xl w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
	}
	for _, dlg := range dialogs {
		if !strings.Contains(content, dlg) {
			t.Errorf("expected dialog to enforce max-h-[90vh] overflow-y-auto: %s", dlg)
		}
	}

	// 4. Scope change handler existence in Privilege Matrix
	if !strings.Contains(content, "function onPrivScopeChanged()") {
		t.Errorf("expected onPrivScopeChanged() to be defined to prevent ReferenceError when toggling scope in privilege matrix")
	}

	// 5. Cluster node session switcher input ID correctness
	if strings.Contains(content, "document.getElementById('conn-host')") ||
		strings.Contains(content, "document.getElementById('conn-port')") ||
		strings.Contains(content, "document.getElementById('conn-name')") {
		t.Errorf("switchSessionToNode references non-existent conn-host/conn-port/conn-name IDs instead of modal-conn-*")
	}

	// 6. Height resilience for modal forms and dialogs to prevent viewport overflow on smaller displays
	additionalHeightGuards := []string{
		`id="view-designer-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-xl w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
		`id="trigger-designer-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-xl w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
		`id="database-creator-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-md w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
		`id="safety-inspection-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-lg w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`,
		`id="connection-modal" class="fixed inset-0 bg-black/70 flex items-center justify-center p-4 z-50 hidden"`,
	}
	for _, guard := range additionalHeightGuards {
		if !strings.Contains(content, guard) {
			t.Errorf("expected index.html to contain dialog height guard: %s", guard)
		}
	}
	if !strings.Contains(content, `id="connection-modal"`) || !regexpMatch(`id="connection-modal"[\s\S]*?max-h-\[90vh\] overflow-y-auto`, content) {
		t.Errorf("expected connection-modal inner container to enforce max-h-[90vh] overflow-y-auto")
	}
	if !strings.Contains(content, `id="vault-modal"`) || !regexpMatch(`id="vault-modal"[\s\S]*?max-h-\[90vh\] overflow-y-auto`, content) {
		t.Errorf("expected vault-modal inner container to enforce max-h-[90vh] overflow-y-auto")
	}
	// Unified Settings modal 4-tab assertions
	for _, tabId := range []string{`id="tab-btn-vault"`, `id="tab-btn-sync"`, `id="tab-btn-bundle"`, `id="tab-btn-ai"`} {
		if !strings.Contains(content, tabId) {
			t.Errorf("expected unified settings modal to contain tab: %s", tabId)
		}
	}
	if !strings.Contains(content, `openSettingsModal()`) {
		t.Errorf("expected index.html to have openSettingsModal handler")
	}
	if !strings.Contains(content, `id="safety-challenge-modal"`) || !regexpMatch(`id="safety-challenge-modal"[\s\S]*?max-h-\[90vh\] overflow-y-auto`, content) {
		t.Errorf("expected safety-challenge-modal inner container to enforce max-h-[90vh] overflow-y-auto")
	}

	// 7. Sidebar session & database dropdown mutual exclusion
	if !strings.Contains(content, "toggleDatabaseDropdown(false);") || !strings.Contains(content, "toggleSessionDropdown(false);") {
		t.Errorf("expected toggleSessionDropdown and toggleDatabaseDropdown to enforce mutual exclusion")
	}
}

func TestComprehensiveUIGlitchGuards(t *testing.T) {
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

	// 1. DDL modal inner container height resilience
	if !regexpMatch(`id="ddl-modal"[^>]*>\s*<div class="[^"]*max-h-\[90vh\] overflow-y-auto`, content) {
		t.Errorf("expected ddl-modal inner container to enforce max-h-[90vh] overflow-y-auto to prevent button clipping on compact displays")
	}

	// 2. Native app alert/confirm dialog height resilience
	if !strings.Contains(content, `id="app-native-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-6 outline-none max-w-md w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`) {
		t.Errorf("expected app-native-dialog to enforce max-h-[90vh] overflow-y-auto to prevent viewport overflow on long error messages")
	}

	// 3. Mock data dialog height resilience & backdrop click close
	if !regexpMatch(`id="mock-data-dialog"[^>]*>\s*<div class="[^"]*max-h-\[85vh\]`, content) &&
		!strings.Contains(content, `id="mock-data-dialog" class="m-auto bg-slate-900 border border-slate-700 text-white rounded-xl shadow-2xl p-0 outline-none max-w-md w-full backdrop:bg-black/75 backdrop:backdrop-blur-sm max-h-[90vh] overflow-y-auto"`) {
		t.Errorf("expected mock-data-dialog to enforce height resilience")
	}
	if !regexpMatch(`\[[^\]]*'mock-data-dialog'[^\]]*\]\.forEach`, content) {
		t.Errorf("expected mock-data-dialog to be included in backdrop click dismiss listeners")
	}

	// 4. Processlist view switcher must reference existing pl-lock-summary, not orphan pl-lock-banner
	if strings.Contains(content, "document.getElementById('pl-lock-banner')") || strings.Contains(content, "document.getElementById('pl-no-lock-banner')") {
		t.Errorf("critical UI glitch: setProcesslistView queries orphan pl-lock-banner instead of pl-lock-summary")
	}

	// 5. escapeHtml must escape single quotes (&#39;) to prevent JavaScript SyntaxError in inline onclick handlers
	if !strings.Contains(content, "replace(/'/g, '&#39;')") {
		t.Errorf("critical UI glitch: escapeHtml does not escape single quotes, risking SyntaxError in onclick attributes")
	}

	// 6. DDL modal backdrop click dismissal
	if !strings.Contains(content, `id="ddl-modal" class="fixed inset-0 bg-black/70 flex items-center justify-center p-4 z-50 hidden" onclick="if(event.target === this) closeDDLModal()"`) {
		t.Errorf("expected ddl-modal to support backdrop click dismissal via onclick='if(event.target === this) closeDDLModal()'")
	}

	// 7. Tabs bar add tab '+' button shrink-0 resilience
	if !regexpMatch(`<button onclick="addNewQueryTab\(\)" class="[^"]*shrink-0`, content) {
		t.Errorf("expected new query tab '+' button to have shrink-0 to prevent flex compression")
	}

	// 8. Action handlers attribute safety against single-quote decode SyntaxError
	if strings.Contains(content, `<button onclick="sendProcessQueryToEditor('${escapeHtml(tx.query || '')}')"`) {
		t.Errorf("critical UI glitch: sendProcessQueryToEditor interpolates tx.query directly into inline JS string literal")
	}
	if !strings.Contains(content, `data-query="${escapeHtml(tx.query || '')}" onclick="sendProcessQueryToEditor(this.dataset.query)"`) {
		t.Errorf("expected lock root blocker button to use data-query attribute")
	}
	if !strings.Contains(content, `peekForeignKey(event, this.dataset.table, this.dataset.col, this.dataset.reftable, this.dataset.refcol, this.dataset.val)`) {
		t.Errorf("expected peekForeignKey to use dataset attributes to avoid single-quote decode SyntaxError on row values")
	}

	// 9. JSON Cell Viewer modal height resilience
	if !regexpMatch(`id="json-cell-editor"[\s\S]*?resize-none`, content) {
		t.Errorf("expected json-cell-editor to use resize-none to prevent manual vertical overflow")
	}
	if !regexpMatch(`id="json-cell-dialog"[\s\S]*?min-h-\[180px\][\s\S]*?overflow-y-auto`, content) {
		t.Errorf("expected json-cell-dialog body to enforce min-h-[180px] and overflow-y-auto")
	}

	// 10. Backdrop click auto-dismiss listener inclusion
	if !strings.Contains(content, "'note-composer-dialog'") || !strings.Contains(content, "'erd-dialog'") {
		t.Errorf("expected note-composer-dialog and erd-dialog to be in backdrop auto-dismiss list")
	}

	// 11. Trigger sidebar dataset onclick handler safety
	if strings.Contains(content, "onclick=\"openCreateTriggerModal('${escapeHtml(trg.name)}', ${escapeHtml(JSON.stringify(trg))})\"") {
		t.Errorf("expected openCreateTriggerModal to use dataset and avoid inline JSON.stringify to prevent SyntaxError")
	}
	if !strings.Contains(content, `data-name="${escapeHtml(trg.name)}" onclick="openCreateTriggerModal(this.dataset.name)"`) {
		t.Errorf("expected trigger item in sidebar to use data-name and openCreateTriggerModal(this.dataset.name)")
	}

	// 12. Operational dialogs included in backdrop dismissal loop
	for _, opDlg := range []string{"'smart-seed-dialog'", "'run-sql-dialog'", "'structure-sync-dialog'", "'data-transfer-dialog'", "'data-sync-dialog'"} {
		if !strings.Contains(content, opDlg) {
			t.Errorf("expected %s to be included in backdrop auto-dismiss listener loop", opDlg)
		}
	}

	// 13. Drop redundant index buttons dataset safety
	if strings.Contains(content, "promptDropRedundantIndex(${JSON.stringify(item.drop_sql)") {
		t.Errorf("expected promptDropRedundantIndex to use data-drop-sql and dataset attributes instead of inline JSON.stringify")
	}

	// 14. All utility dialogs included in backdrop dismissal loop
	for _, dlg := range []string{"'table-creator-dialog'", "'view-designer-dialog'", "'trigger-designer-dialog'", "'safety-inspection-dialog'", "'ddl-review-dialog'"} {
		if !strings.Contains(content, dlg) {
			t.Errorf("expected %s to be included in backdrop auto-dismiss listener loop", dlg)
		}
	}

	// 15. Escape key handling includes safety-inspection-dialog and ddl-review-dialog
	if !regexpMatch(`\['erd-dialog'[^\]]*'safety-inspection-dialog'[^\]]*'ddl-review-dialog'[^\]]*\]\.forEach`, content) {
		t.Errorf("expected safety-inspection-dialog and ddl-review-dialog to be included in Escape key teardown loop")
	}

	// 16. Notes search input Escape key handling with event.stopPropagation
	if !strings.Contains(content, `id="notes-search-input"`) || !strings.Contains(content, `clearNotesSearch(); this.blur(); event.stopPropagation();`) {
		t.Errorf("expected notes-search-input to have onkeydown Escape handling with event.stopPropagation to prevent drawer collapse")
	}

	// 17. Schema tree table and view items dataset onclick safety
	if strings.Contains(content, `onclick="openTableViewer('${escapeHtml(t.name)}')`) || strings.Contains(content, `onclick="openTableViewer('${escapeHtml(v.name)}')`) {
		t.Errorf("critical UI glitch: openTableViewer interpolates table/view name directly into single-quoted string literal")
	}
	if !strings.Contains(content, `data-name="${escapeHtml(t.name)}" onclick="openTableViewer(this.dataset.name)"`) {
		t.Errorf("expected schema tree table items to use data-name and openTableViewer(this.dataset.name)")
	}

	// 18. Virtual grid double-click inline edit dataset safety
	if strings.Contains(content, `ondblclick="startInlineEdit(this, ${rIdx}, ${cIdx}, '${pkCol || ''}', '${tableName || ''}')"`) {
		t.Errorf("critical UI glitch: startInlineEdit interpolates tableName into single-quoted string literal risking SyntaxError on quotes")
	}
	if !strings.Contains(content, `ondblclick="startInlineEdit(this, ${rIdx}, ${cIdx}, this.dataset.pk, this.dataset.table)"`) {
		t.Errorf("expected grid cells to use dataset attributes for startInlineEdit")
	}
}

func TestRemoteAccessLockScreenEmbedded(t *testing.T) {
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
		`id="remote-lock-modal"`,
		`id="remote-unlock-password" autofocus`,
		`function checkAuthStatus()`,
		`apiFetch('/api/auth/status')`,
		`function unlockRemoteVault(`,
		`sqlfox_token=`,
		`body.vault-locked`,
		`classList.add('vault-locked')`,
		`classList.remove('vault-locked')`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain remote lock screen snippet %q, but was missing", s)
		}
	}
}

func TestNotesCardMarkdownPreviewEmbedded(t *testing.T) {
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
		`function renderNoteSnippetPreview(`,
		`renderCopilotMarkdown(snippet)`,
		`note-preview-content`,
		`renderNoteSnippetPreview(n.content)`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain note card markdown preview snippet %q, but was missing", s)
		}
	}
}

func TestHeaderDropdownsAboveNotesDrawerEmbedded(t *testing.T) {
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

	// Bug: notes-inbox-drawer had z-40, matching <header class="... z-40">.
	// Because notes-inbox-drawer comes later in DOM, it stacked on top of <header>'s dropdown menus
	// (#top-settings-menu, #top-db-tools-menu, #top-monitor-menu).
	// notes-inbox-drawer must be strictly lower than header (e.g. z-[35] or z-index: 35)
	if strings.Contains(content, `id="notes-inbox-drawer" class="hidden absolute top-0 right-0 bottom-0 w-[540px] max-w-[95vw] bg-slate-950 border-l border-t border-app-border flex flex-col z-40`) {
		t.Errorf("critical UI stacking bug: notes-inbox-drawer has z-40 which collides with header z-40 and covers header dropdown menus")
	}

	if !strings.Contains(content, `z-[35]`) && !strings.Contains(content, `z-index: 35`) {
		t.Errorf("expected notes-inbox-drawer to have z-[35] so header dropdown menus float over notes drawer")
	}
}

func TestBrandMarkCanonicalIconConsistency(t *testing.T) {
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

	// Verify old low-poly polygons are completely removed from index.html symbol
	if strings.Contains(content, `points="256,140 200,140 108,48 170,180"`) {
		t.Errorf("critical visual bug: index.html still embeds legacy low-poly fox facets in #sqlfox-brand-mark")
	}

	// Verify canonical Puma-style Leaping Fox mark is embedded inside #sqlfox-brand-mark
	if !strings.Contains(content, `id="sqlfox-brand-mark"`) || !strings.Contains(content, `id="sqlfox-fox-mark"`) {
		t.Errorf("expected #sqlfox-brand-mark symbol to contain #sqlfox-fox-mark path in index.html")
	}
}

func TestCodeMirrorDarkThemeTokensEmbedded(t *testing.T) {
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

	requiredRules := []string{
		".cm-variable-2, .cm-variable-3 { color: #7dd3fc !important; }",
		".cm-type { color: #c084fc !important;",
		".cm-atom { color: #fb923c !important; }",
		"div.CodeMirror span.CodeMirror-matchingbracket",
	}

	for _, rule := range requiredRules {
		if !strings.Contains(content, rule) {
			t.Errorf("expected index.html to contain CodeMirror dark theme override %q, but was missing", rule)
		}
	}
}

func TestAutoRefreshCountdownAndTopStripeEmbedded(t *testing.T) {
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

	// 1. Invariant: top-ambient-stripe removed in favor of SSOT Environment Badge
	if strings.Contains(content, "top-ambient-stripe") {
		t.Errorf("expected top-ambient-stripe to be completely removed from index.html")
	}

	// 2. Invariant: countdown indicators present on both Processlist and Cluster Inspector
	expectedSnippets := []string{
		`id="pl-refresh-bar"`,
		`id="cl-refresh-bar"`,
		`function triggerCountdownBar(barId, interval)`,
		`triggerCountdownBar('pl-refresh-bar', interval)`,
		`triggerCountdownBar('cl-refresh-bar', interval)`,
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Errorf("expected index.html to contain countdown snippet %q, but was missing", snippet)
		}
	}
}

func TestProcesslistBatchOperationsEmbedded(t *testing.T) {
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
		`id="pl-filter-chips"`,
		`data-pl-chip="all"`,
		`data-pl-chip="active"`,
		`data-pl-chip="locked"`,
		`data-pl-chip="slow"`,
		`data-pl-chip="sleep"`,
		`id="pl-select-all"`,
		`id="pl-batch-action-bar"`,
		`id="pl-batch-count"`,
		`id="pl-batch-kill-dialog"`,
		`id="pl-batch-challenge-input"`,
		`id="pl-batch-execute-btn"`,
		`function setProcesslistFilterChip(chip)`,
		`function toggleSelectAllProcesslistThreads(checked)`,
		`function openBatchKillModal(mode)`,
		`function executeBatchKill()`,
		`function removeThreadFromBatchKill(threadId)`,
		`function renderBatchKillModalContent()`,
		`removeThreadFromBatchKill(${t.id})`,
		`/api/session/${activeConnection.id}/batch-kill`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q for Processlist Batch Operations, but was missing", s)
		}
	}

	// Ensure batch kill modal has clean English copy and no leftover untranslated strings
	if strings.Contains(content, "Untuk mencegah") || strings.Contains(content, "Ketik KILL") {
		t.Errorf("batch kill modal contains untranslated Indonesian copy")
	}
}

func TestProcesslistBatchModalScriptIntegrity(t *testing.T) {
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

	// Must NOT reference undeclared plInterval or plTimer
	if strings.Contains(content, "plInterval") {
		t.Errorf("critical bug: index.html references undeclared variable 'plInterval'")
	}
	if strings.Contains(content, "plTimer") {
		t.Errorf("critical bug: index.html references undeclared variable 'plTimer'")
	}
}

func TestProcesslistTwoTierToolbarLayout(t *testing.T) {
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

	// Verify Primary Toolbar (Tier 1) and subtoolbar (Tier 2) exist
	if !strings.Contains(content, `id="pl-primary-toolbar"`) {
		t.Errorf("expected id=\"pl-primary-toolbar\" in index.html")
	}
	if !strings.Contains(content, `id="pl-filter-subtoolbar"`) {
		t.Errorf("expected id=\"pl-filter-subtoolbar\" in index.html")
	}

	// Verify primary toolbar order and elements
	tier1Snippets := []string{
		`id="pl-view-threads-btn"`,
		`id="pl-view-digest-btn"`,
		`id="pl-view-deadlock-btn"`,
		`id="pl-summary-text"`,
		`id="pl-lock-summary"`,
		`id="pl-refresh-control-group"`,
		`id="pl-refresh-now-btn"`,
		`id="pl-toggle-trend-btn"`,
	}
	for _, snip := range tier1Snippets {
		if !strings.Contains(content, snip) {
			t.Errorf("expected Tier 1 toolbar snippet %q in index.html", snip)
		}
	}

	// Verify filter subtoolbar elements
	tier2Snippets := []string{
		`id="pl-filter-chips"`,
		`id="pl-search-input"`,
		`id="pl-search-clear-btn"`,
		`id="pl-hide-sleep"`,
		`id="pl-hide-system"`,
		`id="pl-slow-threshold"`,
	}
	for _, snip := range tier2Snippets {
		if !strings.Contains(content, snip) {
			t.Errorf("expected Tier 2 subtoolbar snippet %q in index.html", snip)
		}
	}

	// Verify JS switchProcesslistView toggles pl-filter-subtoolbar
	if !strings.Contains(content, `document.getElementById('pl-filter-subtoolbar')`) {
		t.Errorf("switchProcesslistView should query pl-filter-subtoolbar")
	}
}

func TestProcesslistColumnVisibilityPicker(t *testing.T) {
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

	// Verify trigger and dropdown elements
	expectedSnippets := []string{
		`id="pl-btn-col-picker"`,
		`id="pl-col-picker-dropdown"`,
		`id="pl-col-picker-list"`,
		`id="pl-col-picker-count"`,
		`data-pl-col="select"`,
		`data-pl-col="action"`,
		`data-pl-col="id"`,
		`data-pl-col="user"`,
		`data-pl-col="host"`,
		`data-pl-col="db"`,
		`data-pl-col="command"`,
		`data-pl-col="time"`,
		`data-pl-col="state"`,
		`data-pl-col="query"`,
		`function toggleProcesslistColPicker(e)`,
		`function renderProcesslistColPickerList()`,
		`function setProcesslistColVisibility(key, isVisible)`,
		`function showAllProcesslistColumns()`,
		`function resetDefaultProcesslistColumns()`,
		`function isProcesslistColVisible(key)`,
		`sqlfox-processlist-hidden-cols`,
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q for Processlist Column Visibility, but was missing", s)
		}
	}

	// Verify stacking context and non-clipping: pl-filter-subtoolbar must not have overflow-x-auto,
	// and must have z-30 or z-20 to elevate above sticky thead (z-10)
	if strings.Contains(content, `id="pl-filter-subtoolbar" class="h-9 bg-slate-900/70 border-b border-app-border px-3 flex items-center justify-between text-xs flex-shrink-0 gap-3 overflow-x-auto`) {
		t.Errorf("critical UI bug: #pl-filter-subtoolbar has overflow-x-auto, which clips #pl-col-picker-dropdown")
	}
	if !regexpMatch(`id="pl-filter-subtoolbar"[^>]*class="[^"]*z-(20|30)`, content) {
		t.Errorf("#pl-filter-subtoolbar must have elevated z-index (z-20 or z-30) to sit above table sticky header (z-10)")
	}
}

func TestUpdateAndToastIntegration(t *testing.T) {
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

	// 1. Ensure stale v0.2.0 hardcoded text is completely removed from footer
	if strings.Contains(content, `<span id="footer-version-text">SqlFox v0.2.0</span>`) {
		t.Errorf("critical bug: index.html still contains stale hardcoded 'SqlFox v0.2.0' in footer")
	}

	// 2. Ensure showToast is defined as a global function
	if !strings.Contains(content, `function showToast(message, type = 'info')`) {
		t.Errorf("critical bug: showToast definition is missing in index.html, causing ReferenceError on 34 call sites")
	}

	// 3. Ensure toast container targets active open dialog or document.body for top-layer visibility
	if !strings.Contains(content, `const activeDialog = document.querySelector('dialog[open]');`) ||
		!strings.Contains(content, `const host = activeDialog || document.body;`) {
		t.Errorf("expected showToast to resolve container via active open dialog or document.body")
	}

	// 4. Ensure checkOrOpenUpdate provides immediate visual feedback on the changelog button
	if !strings.Contains(content, `document.getElementById('btn-changelog-check-update')`) ||
		!strings.Contains(content, `Up to date`) {
		t.Errorf("expected checkOrOpenUpdate to provide button feedback on #btn-changelog-check-update")
	}
}

func TestQueryExecutionConcurrencyAndDebounceGuardsEmbedded(t *testing.T) {
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

	// 1. Ensure runActiveQuery has isQueryExecuting concurrency guard
	if !strings.Contains(content, "if (isQueryExecuting) return;") {
		t.Errorf("expected runActiveQuery to guard against concurrent/duplicate execution via 'if (isQueryExecuting) return;'")
	}

	// 2. Ensure Run button has distinct id and is toggled in setQueryRunning
	if !strings.Contains(content, `id="btn-run-query"`) {
		t.Errorf("expected editor Run button to have id='btn-run-query'")
	}
	if !strings.Contains(content, `runBtn.disabled = isRunning;`) {
		t.Errorf("expected setQueryRunning to disable #btn-run-query when query is executing")
	}

	// 3. Ensure keyboard shortcuts has idempotency guard against multiple registration
	if !strings.Contains(content, "let keyboardShortcutsInitialized = false;") ||
		!strings.Contains(content, "if (keyboardShortcutsInitialized) return;") {
		t.Errorf("expected setupKeyboardShortcuts to have idempotency guard")
	}

	// 4. Ensure window keydown handler does not double-dispatch Ctrl+Enter when CodeMirror is focused
	if !strings.Contains(content, "if (cmEditor && cmEditor.hasFocus()) return;") {
		t.Errorf("expected window keydown listener to check cmEditor.hasFocus() to prevent double-execution")
	}
}

func TestFooterLatencyIntegrity(t *testing.T) {
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

	// 1. Ensure query execution does not overwrite ping latency with data.execution_ms
	if strings.Contains(content, "updateFooterStats(data.execution_ms)") {
		t.Errorf("expected index.html to NOT pass data.execution_ms to updateFooterStats")
	}

	// 2. Ensure sub-millisecond latency is displayed cleanly as <1ms
	if !strings.Contains(content, "lat === 0 ? '<1ms' : `${lat}ms`") {
		t.Errorf("expected updateFooterStats to format 0ms latency as '<1ms'")
	}
}

func TestTabDatabaseScopeAndDDLAffirmationEmbedded(t *testing.T) {
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

	requiredElements := []string{
		`id="tab-database-pill-container"`,
		`id="tab-database-pill-name"`,
		`id="tab-db-switcher-menu"`,
		`id="tab-db-search-input"`,
		`id="tab-db-switcher-items"`,
		`id="challenge-target-db-badge"`,
		`id="challenge-target-db-name"`,
		`id="challenge-mismatch-tag"`,
		"function updateTabDatabasePill(tab)",
		"function openTabDatabaseSwitcher(e)",
		"function filterTabDatabaseList(query)",
		"function handleTabDbSearchKeydown(e)",
		"function selectTabDatabase(dbName)",
		"Tab database context switched to",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(content, elem) {
			t.Errorf("expected index.html to contain %q, but was missing", elem)
		}
	}
}

func TestTableDesignerFocusRetentionAndConstraintBuildersEmbedded(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open embedded index.html: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	requiredElements := []string{
		`id="enum-builder-popover"`,
		`id="enum-quick-input"`,
		`id="enum-tags-list"`,
		`id="enum-builder-warning"`,
		`id="enum-builder-sql-preview"`,
		`id="fk-type-mismatch-warning"`,
		`id="index-selected-chips"`,
		`id="index-available-chips"`,
		"function parseEnumOptionList(raw)",
		"function renderEnumPillSummary(lengthStr)",
		"function renderDefaultPresetSelect(c, idx, context)",
		"function validateTableColumns(columns, isCreator)",
		"function updateStagedColLength(idx, newLength)",
		"function updateCreatorColLength(idx, newLength)",
		"function openEnumValuesPopover(event, context, colIdx)",
		"function checkFKTypeCompatibility()",
		"function autoGenerateIndexName()",
		"function autoGenerateFKName()",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(content, elem) {
			t.Errorf("expected index.html to contain %q, but was missing", elem)
		}
	}
}

func TestExportDropdownClickAffordanceAndExclusion(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open embedded index.html: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	// Critical UI bug: If the export dropdown toggle button does not have id="btn-export-csv",
	// or if #export-csv-dropdown-container / #btn-export-csv is not matched in document click listeners,
	// then clicking it fails the e.target.closest check in the document-level contextmenu/click listener,
	// immediately triggering closeAllContextMenus() which re-adds 'hidden' to #export-csv-menu
	// in the exact same click, causing the button to have zero reaction.
	if !strings.Contains(content, `id="btn-export-csv"`) {
		t.Errorf("critical UI bug: export toggle button in #grid-toolbar must have id=\"btn-export-csv\" to match closest selector in document click listener")
	}

	if !strings.Contains(content, `id="export-csv-dropdown-container"`) {
		t.Errorf("expected id=\"export-csv-dropdown-container\" to exist in index.html")
	}
}

func TestExplainQueryToEditorWiring(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open embedded index.html: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	// Ensure window.editorInstance is assigned to cmEditor
	if !strings.Contains(content, "window.editorInstance = cmEditor;") {
		t.Errorf("expected window.editorInstance = cmEditor; to be assigned for legacy/external compatibility")
	}

	// Ensure openImpactQueryInEditor uses setEditorSQL and opens/switches to a query tab if needed
	expectedSnippets := []string{
		"function openImpactQueryInEditor()",
		"setEditorSQL(currentImpactRawSQL)",
		"addNewQueryTab()",
		"function applyAdvisorDDLToEditor(ddl)",
		"setEditorSQL(ddl)",
	}
	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q, but was missing", s)
		}
	}
}

func TestQuerySuccessBannerEmbedded(t *testing.T) {
	fsys := Assets()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open embedded index.html: %v", err)
	}
	defer f.Close()

	contentBytes, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	content := string(contentBytes)

	expectedSnippets := []string{
		"function renderQuerySuccessBanner(sql, message, executionMs, rowsAffected, backupFile)",
		"nextTab.rawQueryExec",
		"renderQuerySuccessBanner(sql, data.message, data.execution_ms, data.rows_affected, data.backup_file)",
		"Command Executed Successfully",
		"Execution Succeeded",
	}
	for _, s := range expectedSnippets {
		if !strings.Contains(content, s) {
			t.Errorf("expected index.html to contain %q, but was missing", s)
		}
	}
}


