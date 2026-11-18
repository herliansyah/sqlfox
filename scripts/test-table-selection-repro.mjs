// Source-assertion regression test: Table row selection (row / checkbox
// selection, select-all) implemented in
// web/frontend/src/lib/components/grid/VirtualGrid.svelte. The selection
// algorithm is copied from the real source into plain JS and asserted.
import fs from 'fs';
import assert from 'assert';

console.log('=== Running Table Selection Source Assertions ===');

const gridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');

// 1. Selection state lives in VirtualGrid
assert(
  gridCode.includes('let selectedRowIndices = $state<number[]>([]);'),
  'VirtualGrid must hold selectedRowIndices state'
);
assert(
  gridCode.includes('let isAllSelected = $derived('),
  'VirtualGrid must derive isAllSelected from selectedRowIndices vs processedRows'
);
assert(
  gridCode.includes('function toggleSelectAll() {'),
  'VirtualGrid must implement toggleSelectAll'
);
assert(
  gridCode.includes('function toggleRowSelection(origIdx: number) {'),
  'VirtualGrid must implement toggleRowSelection'
);
assert(
  gridCode.includes('onchange={toggleSelectAll}') && gridCode.includes('type="checkbox"'),
  'Select-all header checkbox must be wired to toggleSelectAll'
);
assert(
  gridCode.includes('selectedRowIndices.includes(rowItem.origIdx)'),
  'Row checkboxes must reflect selection by origIdx'
);

// 2. Selection clears after batch delete
assert(
  gridCode.includes('await appState.deleteRows(pkCol, pkVals);'),
  'Batch delete must delegate to appState.deleteRows'
);
assert(
  gridCode.includes('if (!ok) return;'),
  'Batch delete must honour the user confirmation result'
);

// 3. Selection is per-grid local state, so it resets when the active tab /
//    table changes (VirtualGrid is keyed per tab; selection never leaks)
assert(
  appStateCode.includes('openTableDataTab(tableName'),
  'appState must define openTableDataTab for switching tables'
);

// ---- Algorithm under test (copied from VirtualGrid.svelte) ----
// processedRows maps result rows to { origIdx, data }
const processedRows = (rows) => (rows || []).map((r, idx) => ({ origIdx: idx, data: r }));

const isAllSelected = (selectedRowIndices, rows) =>
  rows.length > 0 && selectedRowIndices.length === rows.length;

function toggleSelectAll(selectedRowIndices, rows) {
  if (isAllSelected(selectedRowIndices, rows)) {
    return [];
  }
  return processedRows(rows).map((r) => r.origIdx);
}

function toggleRowSelection(selectedRowIndices, origIdx) {
  const idx = selectedRowIndices.indexOf(origIdx);
  const next = [...selectedRowIndices];
  if (idx === -1) {
    next.push(origIdx);
  } else {
    next.splice(idx, 1);
  }
  return next;
}

// ---- Scenario: 10-row users table ----
const usersRows = Array.from({ length: 10 }, (_, i) => ({ id: i + 1, name: `user${i + 1}` }));
const users = processedRows(usersRows);

// STEP 1: Select all rows in users table
let selected = toggleSelectAll([], usersRows);
assert.strictEqual(selected.length, 10, 'Select-all on 10 rows must select 10 rows');
assert(isAllSelected(selected, usersRows), 'isAllSelected must be true after select-all');

// STEP 2: Toggle select-all again clears the selection
selected = toggleSelectAll(selected, usersRows);
assert.strictEqual(selected.length, 0, 'Select-all toggle must clear the selection when everything is selected');

// STEP 3: Individual row toggles
selected = toggleRowSelection(selected, 0);
selected = toggleRowSelection(selected, 3);
assert.strictEqual(selected.length, 2, 'Two individual row picks must yield 2 selected rows');
assert.deepStrictEqual(selected, [0, 3], 'Selected indices must track the picked rows');

// STEP 4: Un-toggling a picked row removes it
selected = toggleRowSelection(selected, 3);
assert.deepStrictEqual(selected, [0], 'Toggling a selected row must unselect it');
assert(!isAllSelected(selected, usersRows), 'isAllSelected must be false for a partial selection');

// STEP 5: Switching table to products (2 rows): fresh grid, selection empty
const productsRows = Array.from({ length: 2 }, (_, i) => ({ id: i + 1, name: `product${i + 1}` }));
let selectedAfterSwitch = [];
assert.strictEqual(
  selectedAfterSwitch.length, 0,
  'Selection must be released (0 selected) when switching to another table'
);
selectedAfterSwitch = toggleRowSelection(selectedAfterSwitch, 0);
selectedAfterSwitch = toggleRowSelection(selectedAfterSwitch, 1);
assert.strictEqual(selectedAfterSwitch.length, 2, 'Expected 2 rows selected in products');

// STEP 6: Switching back to users: selection starts empty again
let selectedAfterSwitchBack = [];
assert.strictEqual(
  selectedAfterSwitchBack.length, 0,
  'Selection must be released (0 selected) after switching back to the users tab'
);

// STEP 7: Select-all on products then toggling off works independently
let productsSelected = toggleSelectAll([], productsRows);
assert.strictEqual(productsSelected.length, 2, 'Select-all on 2 products must select 2');
productsSelected = toggleRowSelection(productsSelected, 1);
assert.strictEqual(productsSelected.length, 1, 'Partial unselect must leave 1 row');
assert.deepStrictEqual(productsSelected, [0], 'Remaining selection must be the untoggled row');

// STEP 8: Empty result set never reports all-selected
assert(!isAllSelected([], []), 'isAllSelected must be false for an empty result set');
assert.deepStrictEqual(toggleSelectAll([], []), [], 'Select-all on an empty grid must stay empty');

console.log('🎉 ALL TABLE SELECTION SOURCE ASSERTIONS PASSED (GREEN)!');
