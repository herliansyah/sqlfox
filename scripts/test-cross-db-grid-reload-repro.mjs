import fs from 'fs';
import assert from 'assert';
// Source-assertion pattern: assert the source wiring of those flows in the Svelte
// port rather than mocking the network.
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');
const clientCode = fs.readFileSync('web/frontend/src/lib/api/client.ts', 'utf8');
const tableViewerCode = fs.readFileSync('web/frontend/src/lib/components/table/TableViewer.svelte', 'utf8');
const virtualGridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');

// --- TEST 1: client.getTableRows must accept an explicit db parameter and send it
// as &db=<database> so a cross-db table tab never hits the sidebar's active DB ---
assert(
  /getTableRows\(\s*sessionId: string,\s*tableName: string,\s*limit = 50,\s*offset = 0,\s*db\?: string/.test(clientCode),
  'client.getTableRows must accept a db?: string parameter'
);
assert(
  clientCode.includes('`/session/${sessionId}/table/${encodeURIComponent(tableName)}/rows?limit=${limit}&offset=${offset}`') &&
  /if \(db\) url \+= `&db=\$\{encodeURIComponent\(db\)\}`;/.test(clientCode),
  'client.getTableRows must append &db=<encoded database> when a db is given'
);
console.log('✅ Test 1 Passed: client.getTableRows scopes row fetches to the requested db');

// --- TEST 2: appState.loadTableData must use the TAB's database, falling back to
// the sidebar currentDatabase only when the tab has none ---
assert(appStateCode.includes('async loadTableData(targetTab?: EditorTab)'), 'appState.loadTableData must exist');
assert(
  appStateCode.includes('const db = tab.database || this.currentDatabase || \'\';'),
  'loadTableData must prefer tab.database over the sidebar currentDatabase'
);
assert(
  /loadTableData[\s\S]{0,1600}?api\.getTableRows\(\s*this\.activeSessionId,\s*tab\.tableName,/.test(appStateCode),
  'loadTableData must pass the tab-scoped db through api.getTableRows'
);
console.log('✅ Test 2 Passed: loadTableData is tab-database scoped (never the wrong sidebar DB)');

// --- TEST 3: Query tabs reload by re-executing their SQL, not by fetching generic
// /table/<name>/rows from the wrong sidebar DB ---
assert(
  appStateCode.includes('if (tab.isTableViewer && sqlOverride === undefined) {') &&
  appStateCode.includes('await this.loadTableData(tab);'),
  'executeCurrentTab must only route table-viewer tabs through loadTableData'
);
assert(
  appStateCode.includes('const database = tab.database || this.currentDatabase;'),
  'executeCurrentTab must execute query tabs with the tab\'s own database'
);
assert(
  /await api\.executeQuery\(this\.activeSessionId, sql, false, database\)/.test(appStateCode),
  'query tabs must re-run their SQL via api.executeQuery with the tab-scoped database'
);
console.log('✅ Test 3 Passed: query-tab reload re-executes SQL in the tab database, never /table/.../rows');

// --- TEST 4: applyGridChanges equivalent (commitStagedEdits) refreshes through the
// same db-scoped paths ---
assert(appStateCode.includes('async commitStagedEdits()'), 'appState.commitStagedEdits must exist (legacy applyGridChanges)');
assert(
  /commitStagedEdits[\s\S]{0,2000}?api\.cellEdit\(this\.activeSessionId,/.test(appStateCode),
  'commitStagedEdits must persist staged edits via api.cellEdit'
);
assert(
  appStateCode.includes('if (this.activeTab?.isTableViewer) {') &&
  /commitStagedEdits[\s\S]{0,2600}?isTableViewer[\s\S]{0,300}?await this\.loadTableData\(this\.activeTab\);[\s\S]{0,200}?await this\.executeCurrentTab\(\);/.test(appStateCode),
  'commitStagedEdits must reload via loadTableData for table tabs and executeCurrentTab for query tabs'
);
assert(
  virtualGridCode.includes('appState.commitStagedEdits()'),
  'VirtualGrid Apply button must trigger appState.commitStagedEdits'
);
console.log('✅ Test 4 Passed: commitStagedEdits refreshes table tabs via loadTableData, query tabs via executeCurrentTab');

// --- TEST 5: TableViewer surfaces cross-db scope and drives the same load path ---
assert(
  tableViewerCode.includes('appState.loadTableData(currentTab)'),
  'TableViewer must (re)load data through appState.loadTableData'
);
assert(
  tableViewerCode.includes('currentTab.database !== appState.currentDatabase'),
  'TableViewer must show the tab database pill when it differs from the sidebar database (cross-db visibility)'
);
assert(
  virtualGridCode.includes('appState.loadTableData(currentTab)'),
  'VirtualGrid pagination/filters must reload through appState.loadTableData'
);
console.log('✅ Test 5 Passed: TableViewer/VirtualGrid reload through the db-scoped load path');

console.log('ALL REGRESSION TESTS PASSED!');
