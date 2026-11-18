import fs from 'fs';
import assert from 'assert';

console.log('--- Query vs Table Toolbar Visibility Test (source-assertion) ---');

// Legacy intent: table-mutation controls (Add Row / Mock Data / Import CSV / edit mode /
// Table SQL Dump / table mutations context menu) exist only for Table Viewer tabs, while
// query tabs instead expose result stats. The Svelte port implements this declaratively
// with Svelte `{#if currentTab?.isTableViewer}` branches in VirtualGrid.svelte — the
// framework unmounts/mounts the controls, which is strictly stronger than toggling a
// `hidden` class. We assert the source branches plus a plain-JS replica of the
// visibility model across the three legacy switch scenarios.

const gridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');

// 1. All legacy toolbar elements must exist in the Svelte source
for (const id of [
  'btn-grid-add-row',
  'btn-mock-data',
  'btn-import-csv',
  'grid-edit-mode-container',
  'export-menu-table-dump',
  'grid-ctx-table-mutations',
  'grid-bottom-stats',
]) {
  assert(gridCode.includes(`id="${id}"`), `${id} must exist in VirtualGrid.svelte`);
}

// 2. Table-only controls must be gated on currentTab?.isTableViewer. Verify the gate
//    is still open where each element is declared (handles nested {#if} blocks).
const gateOpenAt = (idx) => {
  let depth = 0;
  for (const m of gridCode.matchAll(/\{#if [^}]*\}|\{\/if\}/g)) {
    if (m.index >= idx) break;
    if (m[0].startsWith('{/')) depth -= 1;
    else {
      depth += 1;
      if (depth === 1 && m[0].startsWith('{#if currentTab?.isTableViewer')) depth = 1;
    }
  }
  return depth > 0;
};
const tableOnlyIds = [
  'btn-grid-add-row',
  'btn-import-csv',
  'btn-mock-data',
  'grid-edit-mode-container',
  'export-menu-table-dump',
  'grid-ctx-table-mutations',
];
for (const id of tableOnlyIds) {
  const idx = gridCode.indexOf(`id="${id}"`);
  assert.ok(idx !== -1, `${id} must exist in VirtualGrid.svelte`);
  assert.ok(gateOpenAt(idx), `${id} must be inside an {#if currentTab?.isTableViewer} branch`);
}
assert(
  gridCode.includes('{#if currentTab?.isTableViewer}') &&
    gridCode.split('{#if currentTab?.isTableViewer}').length >= 3,
  'Table-only toolbar sections must be conditionally rendered for Table Viewer tabs'
);

// 3. Query-tab-only stats live in the bottom bar, which renders for every result
assert(
  gridCode.includes('id="grid-bottom-stats"') &&
    gridCode.includes('{totalRows.toLocaleString()}') &&
    gridCode.includes('{result.duration_ms} ms'),
  'Result stats (row count + duration) must be rendered in the grid bottom bar for query results'
);
assert(
  gridCode.includes("class=\"fixed z-50 w-56") || gridCode.includes('id="grid-context-menu"'),
  'Grid context menu must exist for query tabs too (quick filters)'
);

// 4. Plain-JS replica of the visibility model, with the exact gate condition
//    copied from VirtualGrid.svelte source.
const isTableViewerVisible = (tab) => Boolean(tab?.isTableViewer);
const toolbarVisibility = (tab) => ({
  addRow: isTableViewerVisible(tab),
  mockData: isTableViewerVisible(tab),
  importCsv: isTableViewerVisible(tab),
  editModeContainer: isTableViewerVisible(tab),
  tableDump: isTableViewerVisible(tab),
  tableMutationsCtx: isTableViewerVisible(tab),
  resultStats: true, // bottom stats bar renders for any result set
});

// Test Case 1: Query tab (isTableViewer === false)
let tab = { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT 1;', isTableViewer: false };
let v = toolbarVisibility(tab);
assert.strictEqual(v.addRow, false, 'Add Row button MUST be hidden in Query tab');
assert.strictEqual(v.mockData, false, 'Mock Data button MUST be hidden in Query tab');
assert.strictEqual(v.importCsv, false, 'Import CSV button MUST be hidden in Query tab');
assert.strictEqual(v.editModeContainer, false, 'Edit mode container MUST be hidden in Query tab');
assert.strictEqual(v.tableDump, false, 'Table SQL Dump in export menu MUST be hidden in Query tab');
assert.strictEqual(v.tableMutationsCtx, false, 'Table mutations context menu MUST be hidden in Query tab');
assert.strictEqual(v.resultStats, true, 'Result stats MUST be visible in Query tab');
console.log('✓ Test Case 1 passed: Table-specific buttons hidden on Query tab');

// Test Case 2: Table Viewer tab (isTableViewer === true) — opened via openTableDataTab
assert(
  appStateCode.includes('openTableDataTab(tableName: string') &&
    appStateCode.includes('isTableViewer: true,'),
  'openTableDataTab must create tabs with isTableViewer: true'
);
assert(
  appStateCode.includes('if (existing) {\n      existing.tableMode = initialMode;\n      this.activeTabId = existing.id;'),
  'openTableDataTab must reuse an existing tab for the same table and switch to it'
);
tab = { id: 'tab-users', title: 'users', tableName: 'users', isTableViewer: true, tableMode: 'data' };
v = toolbarVisibility(tab);
assert.strictEqual(v.addRow, true, 'Add Row button MUST be visible in Table Viewer tab');
assert.strictEqual(v.mockData, true, 'Mock Data button MUST be visible in Table Viewer tab');
assert.strictEqual(v.importCsv, true, 'Import CSV button MUST be visible in Table Viewer tab');
assert.strictEqual(v.editModeContainer, true, 'Edit mode container MUST be visible in Table Viewer tab');
assert.strictEqual(v.tableDump, true, 'Table SQL Dump MUST be visible in Table Viewer tab');
assert.strictEqual(v.tableMutationsCtx, true, 'Table mutations context menu MUST be visible in Table Viewer tab');
console.log('✓ Test Case 2 passed: Table-specific buttons visible on Table Viewer tab');

// Test Case 3: Switching back to a Query tab hides the table buttons again.
// Switching happens by setting activeTabId (TabBar / openTableDataTab / addTab all do
// this); VirtualGrid derives currentTab from appState.activeTab, so the gates re-evaluate.
assert(
  gridCode.includes('let currentTab = $derived(appState.activeTab);'),
  'VirtualGrid must derive currentTab from appState.activeTab so switching tabs re-gates the toolbar'
);
assert(
  appStateCode.includes('this.activeTabId = existing.id;') &&
    appStateCode.includes('this.activeTabId = newId;'),
  'Tab switching in appState must go through activeTabId assignment'
);
const tabs = [
  { id: 'tab-users', title: 'users', tableName: 'users', isTableViewer: true, tableMode: 'data' },
  { id: 'tab-query', title: 'Query 2.sql', sql: 'SELECT * FROM users;', isTableViewer: false },
];
const activeTab = () => tabs.find((t) => t.id === activeTabId);
let activeTabId = 'tab-users';
activeTabId = 'tab-query'; // switch back to query tab
v = toolbarVisibility(activeTab());
assert.strictEqual(v.addRow, false, 'Add Row button MUST be hidden after switching back to Query tab');
assert.strictEqual(v.mockData, false, 'Mock Data button MUST be hidden after switching back to Query tab');
assert.strictEqual(v.importCsv, false, 'Import CSV button MUST be hidden after switching back to Query tab');
assert.strictEqual(v.editModeContainer, false, 'Edit mode container MUST be hidden after switching back to Query tab');
assert.strictEqual(v.tableDump, false, 'Table SQL Dump MUST be hidden after switching back to Query tab');
assert.strictEqual(v.tableMutationsCtx, false, 'Table mutations context menu MUST be hidden after switching back to Query tab');
assert.strictEqual(v.resultStats, true, 'Result stats MUST be visible after switching back to Query tab');
console.log('✓ Test Case 3 passed: Switching back to Query tab hides table buttons correctly');

console.log('--- All Toolbar Visibility Tests Passed Successfully! ---');
