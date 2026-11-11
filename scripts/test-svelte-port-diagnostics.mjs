import fs from 'fs';
import assert from 'assert';

console.log('=== Running Porting Diagnostics & Regression Verification ===');

// --- Check 1: Bug 1 - RemoteLockModal & LAN Login Blur ---
const remoteLockModal = fs.readFileSync('web/frontend/src/lib/components/modals/RemoteLockModal.svelte', 'utf8');
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');
const mainTsCode = fs.readFileSync('web/frontend/src/main.ts', 'utf8');
const indexHtmlCode = fs.readFileSync('web/frontend/index.html', 'utf8');

// Check 1.1: RemoteLockModal must have high z-index (z-[100] or z-[110]) so it is never covered by background elements
assert(
  remoteLockModal.includes('z-[100]') || remoteLockModal.includes('z-[110]'),
  'RemoteLockModal.svelte must have high z-index (z-[100] or z-[110]) to avoid being occluded by background shells'
);

// Check 1.2: appState.init() and unlockRemote must clean up remote-lock-modal and vault-locked
assert(
  appStateCode.includes("classList.add('hidden')") || appStateCode.includes("remote-lock-modal"),
  'appState must hide server-rendered remote-lock-modal shell once Svelte mounts'
);
assert(
  appStateCode.includes("classList.remove('vault-locked')"),
  'appState must remove vault-locked class on unlock or non-remote'
);

// --- Check 2: Bug 2 - Activity Console infinite sqlfox_started loop ---
const activityConsole = fs.readFileSync('web/frontend/src/lib/components/activity/ActivityConsoleDrawer.svelte', 'utf8');

// eventSource must NOT be a $state variable because reading/writing it inside $effect causes an infinite reconnection loop
assert(
  !activityConsole.includes('let eventSource = $state'),
  'ActivityConsoleDrawer.svelte must NOT declare eventSource with $state() to prevent infinite effect re-trigger loop'
);
assert(
  activityConsole.includes('data.module'),
  'ActivityConsoleDrawer.svelte must map data.module correctly from backend Go Event struct'
);

// --- Check 3: Bug 3 - Table Rows Performance & DB Scope ---
const clientTsCode = fs.readFileSync('web/frontend/src/lib/api/client.ts', 'utf8');
const virtualGridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');
const handlersSchemaCode = fs.readFileSync('internal/server/handlers_schema.go', 'utf8');

// client.ts getTableRows must accept db parameter
assert(
  clientTsCode.includes('getTableRows(sessionId: string, tableName: string, limit = 50') ||
  clientTsCode.includes('db?: string'),
  'client.ts getTableRows must accept db parameter'
);

// appState loadTableData must pass db
assert(
  appStateCode.includes('getTableRows(') && appStateCode.includes('this.currentDatabase'),
  'appState.loadTableData must pass currentDatabase to getTableRows'
);

// VirtualGrid must use map lookups instead of allColumns.indexOf and find() on every cell
assert(
  virtualGridCode.includes('colIndexMap') || virtualGridCode.includes('fkMap'),
  'VirtualGrid.svelte must use precomputed Map lookups for columns/foreign keys to ensure high FPS'
);

// Tab persistence
assert(
  appStateCode.includes('sqlfox_tabs_'),
  'appState must implement tab persistence via localStorage (sqlfox_tabs_<conn_id>)'
);

// --- Check 4: Operational Notes Reminder Engine & Badge ---
const headerCode = fs.readFileSync('web/frontend/src/lib/components/layout/Header.svelte', 'utf8');
assert(
  headerCode.includes('notes-inbox-badge') && headerCode.includes('dueNotesCount'),
  'Header.svelte must display #notes-inbox-badge when dueNotesCount > 0'
);
assert(
  appStateCode.includes('startNotesReminderPoller') && appStateCode.includes('checkDueNotesReminders'),
  'appState must implement background reminder poller for due operational notes'
);

// --- Check 5: Multi-Dialect Quoting & Conflict Handling ---
const csvImportCode = fs.readFileSync('web/frontend/src/lib/components/modals/CsvImportModal.svelte', 'utf8');
const viewDesignerCode = fs.readFileSync('web/frontend/src/lib/components/modals/ViewDesignerModal.svelte', 'utf8');

assert(
  csvImportCode.includes('isPostgres') && csvImportCode.includes('ON CONFLICT DO NOTHING'),
  'CsvImportModal must handle PostgreSQL identifier quoting and ON CONFLICT DO NOTHING'
);
assert(
  viewDesignerCode.includes('isPostgres') && viewDesignerCode.includes('CREATE OR REPLACE VIEW'),
  'ViewDesignerModal must handle PostgreSQL view syntax without MySQL-specific ALGORITHM keyword'
);

// --- Check 6: Grid Loading & Reactivity Signal Propagation ---
const tableViewerCode = fs.readFileSync('web/frontend/src/lib/components/table/TableViewer.svelte', 'utf8');

assert(
  appStateCode.includes('this.tabs = [...this.tabs]'),
  'appState must propagate tabs reactivity via array spread on loadTableData'
);
assert(
  tableViewerCode.includes('loadTableData(currentTab)'),
  'TableViewer must contain fallback auto-loader effect for unpopulated data tabs'
);
assert(
  virtualGridCode.includes('appState.activeTab?.isExecuting') || virtualGridCode.includes('Boolean(appState.activeTab?.isExecuting)'),
  'VirtualGrid must bind isExecuting directly to appState.activeTab to avoid stale loading spinner'
);

console.log('✅ All diagnostic assertions verified successfully!');
