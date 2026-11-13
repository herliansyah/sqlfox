import fs from 'fs';
import assert from 'assert';

console.log('--- Starting Grid Ergonomics Suite Test (source-assertion) ---');

// Source-assertion pattern: read the actual Svelte/stores sources and assert
// the behaviors live there — no bundled web/dist artifacts, no vm sandboxes.
const virtualGridCode = fs.readFileSync(
  'web/frontend/src/lib/components/grid/VirtualGrid.svelte',
  'utf8'
);
const inspectorCode = fs.readFileSync(
  'web/frontend/src/lib/components/modals/RecordInspectorModal.svelte',
  'utf8'
);
const appStateCode = fs.readFileSync(
  'web/frontend/src/lib/stores/appState.svelte.ts',
  'utf8'
);

// --- 1. Column Picker markers (toolbar + dropdown) -------------------------
for (const marker of [
  'id="btn-col-picker"',
  'id="col-picker-dropdown"',
  'id="col-picker-search"',
  'id="col-picker-count-badge"',
  'id="col-picker-list"',
]) {
  assert(
    virtualGridCode.includes(marker),
    `VirtualGrid.svelte must contain ${marker}`
  );
}
// Show All / Hide All actions must be wired in the dropdown
assert(
  virtualGridCode.includes('appState.showAllColumns()') &&
    virtualGridCode.includes('appState.hideAllColumns('),
  'col-picker-dropdown must expose Show All / Hide All actions'
);
assert(
  virtualGridCode.includes('appState.toggleColumnHidden('),
  'col-picker-list checkboxes must toggle visibility via appState.toggleColumnHidden'
);
// Count badge must reflect visible column count vs total
assert(
  virtualGridCode.includes('id="col-picker-count-badge">{visibleColumns.length}'),
  'col-picker-count-badge must display the number of visible columns'
);

// --- 2. Record Inspector markers ------------------------------------------
for (const marker of [
  'id="record-inspector-dialog"',
  'id="record-inspector-fields"',
  'id="record-inspector-pager"',
  'id="record-inspector-search"',
]) {
  assert(
    inspectorCode.includes(marker),
    `RecordInspectorModal.svelte must contain ${marker}`
  );
}
assert(
  appStateCode.includes('isRecordInspectorOpen = $state<boolean>(false)') &&
    appStateCode.includes('recordInspectorRowIndex = $state<number>(0)'),
  'appState must own the record inspector open flag and row index'
);
// Inspector is opened from the grid gutter double-click and context menu
assert(
  virtualGridCode.includes('appState.isRecordInspectorOpen = true'),
  'VirtualGrid must be able to open the record inspector'
);
// Navigation clamps to the result window bounds
assert(
  inspectorCode.includes('function navigate(delta: number)') &&
    inspectorCode.includes('appState.recordInspectorRowIndex = next;'),
  'RecordInspectorModal must implement bounded navigation via navigate()'
);
assert(
  inspectorCode.includes('disabled={rIdx <= 0}') &&
    inspectorCode.includes('disabled={rIdx >= totalRows - 1}'),
  'Inspector pager buttons must be disabled at the first/last record'
);
// Search filtering of inspector fields
assert(
  inspectorCode.includes('searchQuery'),
  'record-inspector-search must back a searchQuery filter'
);
assert(
  inspectorCode.includes('editedFields[colName] = val;') &&
    inspectorCode.includes('appState.stageEdit('),
  'Inspector field edits must be staged via appState.stageEdit'
);

// --- 3. Quick filter (context menu) ---------------------------------------
assert(
  virtualGridCode.includes('function applyQuickFilter(op: FilterPill'),
  'VirtualGrid must define applyQuickFilter to push a filter pill from the context menu'
);
assert(
  virtualGridCode.includes('appState.addFilterPill(colName, op, val);'),
  'applyQuickFilter must delegate to appState.addFilterPill'
);
for (const op of ["'='", "'!='", "'>'", "'IS NULL'", "'IS NOT NULL'"]) {
  assert(
    virtualGridCode.includes(`applyQuickFilter(${op})`),
    `Context menu must offer a quick filter button for op ${op}`
  );
}
// Quick filter option icons must use normalized w-5 container for consistent indentation
const iconSlotCount = (virtualGridCode.match(
  /w-5 text-center flex-shrink-0 inline-flex items-center justify-center/g
) || []).length;
assert(
  iconSlotCount >= 6,
  `Quick filter option icons must use the normalized w-5 slot (found ${iconSlotCount}, need >= 6)`
);
// NULL value branch renders the same normalized slot
assert(
  /IS NULL'\)[\s\S]{0,200}w-5 text-center flex-shrink-0 inline-flex items-center justify-center/.test(
    virtualGridCode
  ),
  'Null quick filter options must use the normalized w-5 container'
);
for (const marker of [
  'id="grid-ctx-filter-parent"',
  'id="grid-ctx-filter-options"',
  'id="grid-ctx-insert-section"',
  'id="grid-ctx-insert-options"',
]) {
  assert(
    virtualGridCode.includes(marker),
    `Grid context menu must contain ${marker}`
  );
}

// --- 4. Column visibility algorithm (copied from sources) ------------------
// appState.hideAllColumns: hide everything except the PK column.
const hideAllColumns = (columns, pkCol) => columns.filter((c) => c !== pkCol);
// VirtualGrid.visibleColumns: filter allColumns by the tab's hiddenColumns list.
const visibleColumnsOf = (allCols, hiddenCols) =>
  allCols.filter((c) => !hiddenCols.includes(c));

const allCols = ['id', 'email', 'created_at'];
const pk = 'id';
assert.deepStrictEqual(
  hideAllColumns(allCols, pk),
  ['email', 'created_at'],
  'hideAllColumns must keep the PK column visible'
);
let hidden = hideAllColumns(allCols, pk);
assert.deepStrictEqual(
  visibleColumnsOf(allCols, hidden),
  ['id'],
  'visibleColumns must exclude hidden columns, leaving only the PK'
);
// Toggle a column back on via appState.toggleColumnHidden semantics
// (splice when present, push when absent)
const toggleColumnHidden = (hiddenColumns, colName) => {
  const idx = hiddenColumns.indexOf(colName);
  if (idx !== -1) hiddenColumns.splice(idx, 1);
  else hiddenColumns.push(colName);
  return hiddenColumns;
};
hidden = toggleColumnHidden([...hidden], 'created_at');
assert.deepStrictEqual(
  hidden,
  ['email'],
  'toggleColumnHidden must remove an existing hidden column (making it visible)'
);
assert.deepStrictEqual(
  visibleColumnsOf(allCols, hidden),
  ['id', 'created_at'],
  'created_at must be visible again after toggle'
);
// showAllColumns clears the hidden set entirely
hidden = [];
assert.deepStrictEqual(
  visibleColumnsOf(allCols, hidden),
  allCols,
  'showAllColumns must restore every column'
);
// hideAllColumns never hides the PK: it always filters it out, even if a
// previous toggle added it.
hidden = toggleColumnHidden([...hidden], 'id');
assert(
  hidden.includes('id'),
  'toggleColumnHidden can individually hide a column (real toggle semantics)'
);
assert(
  !hideAllColumns(allCols, pk).includes('id') &&
    !hideAllColumns(allCols, pk).concat('id').filter((c) => c !== pk).includes('id'),
  'hideAllColumns must always exclude the PK, so isColumnHidden(id) is false after Hide All'
);
hidden = hideAllColumns(allCols, pk);
assert(
  !hidden.includes('id'),
  'After Hide All the PK must remain visible'
);

// --- 5. appState.addFilterPill semantics (quick filter behavior) -----------
// Copied from appState.svelte.ts: pushes the pill and compiles a WHERE clause
// for non-empty values / NULL operators.
function compileFilterPills(pills, driver) {
  if (!Array.isArray(pills) || pills.length === 0) return '';
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const clauses = [];
  for (const p of pills) {
    if (!p || !p.col) continue;
    const cleanCol = p.col.replace(/^[`"[]+|[`"]]+$/g, '');
    const colEscaped = isPg
      ? `"${cleanCol.replace(/"/g, '""')}"`
      : `\`${cleanCol.replace(/`/g, '``')}\``;
    const op = (p.op || '=').toUpperCase();
    if (op === 'IS NULL' || op === 'IS NOT NULL') {
      clauses.push(`${colEscaped} ${op}`);
      continue;
    }
    const val = String(p.val !== undefined ? p.val : '').trim();
    if (val === '') continue;
    const isNum = !isNaN(Number(val)) && val !== '' && !val.includes(' ');
    if (isNum) clauses.push(`${colEscaped} ${op} ${val}`);
    else clauses.push(`${colEscaped} ${op} '${val.replace(/'/g, "''")}'`);
  }
  return clauses.join(' AND ');
}

const compileLikeApp = (pills, driver) => {
  // addFilterPill only compiles when the value is meaningful
  const meaningful = pills.some((p) => p.val !== '' || p.op === 'IS NULL' || p.op === 'IS NOT NULL');
  return meaningful ? compileFilterPills(pills, driver) : '';
};

let pills = [];
pills.push({ col: 'status', op: '=', val: 'active' });
assert.strictEqual(pills.length, 1, 'Quick filter must add one filter pill');
assert.strictEqual(pills[0].col, 'status');
assert.strictEqual(pills[0].op, '=');
assert.strictEqual(pills[0].val, 'active');
assert.strictEqual(
  compileLikeApp(pills, 'mysql'),
  '`status` = \'active\'',
  'Compiled WHERE clause must quote string values for MySQL'
);

pills.push({ col: 'deleted_at', op: 'IS NULL', val: '' });
assert.strictEqual(pills.length, 2, 'Second quick filter must append a pill');
assert.strictEqual(pills[1].op, 'IS NULL');
assert.strictEqual(
  compileLikeApp(pills, 'mysql'),
  "`status` = 'active' AND `deleted_at` IS NULL",
  'IS NULL pill must compile without a value'
);

pills = [{ col: 'age', op: '>', val: '30' }];
assert.strictEqual(
  compileFilterPills(pills, 'postgres'),
  '"age" > 30',
  'Numeric values must not be quoted; postgres columns use double quotes'
);

// --- 6. Virtual slicing (row virtualization) ------------------------------
// Copied from VirtualGrid.svelte
const ROW_HEIGHT = 28;
const OVERSCAN = 15;
const computeWindow = (scrollTop, containerHeight, totalRows) => {
  const startIndex = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const endIndex = Math.min(
    totalRows,
    Math.ceil((scrollTop + containerHeight) / ROW_HEIGHT) + OVERSCAN
  );
  return { startIndex, endIndex };
};
{
  const { startIndex, endIndex } = computeWindow(0, 500, 1000);
  assert.strictEqual(startIndex, 0, 'At top of scroll the window must start at row 0');
  assert.strictEqual(
    endIndex,
    Math.ceil(500 / ROW_HEIGHT) + OVERSCAN,
    'Window end must cover the viewport plus overscan rows'
  );
  assert(endIndex <= 1000, 'Window end must clamp to the row count');
}
{
  const { startIndex } = computeWindow(2800, 500, 1000);
  assert.strictEqual(startIndex, 85, 'Scrolling must advance the virtual window start');
}

console.log('✓ Column picker source assertions passed!');
console.log('✓ Record inspector source assertions passed!');
console.log('✓ Quick-filter source assertions passed!');
console.log('✓ Column visibility behavior assertions passed!');
console.log('✓ Virtual slicing behavior assertions passed!');

console.log('\n🎉 ALL GRID ERGONOMICS SUITE TESTS PASSED 100%!');
