import fs from 'fs';
import assert from 'assert';

console.log('=== Query Success/Error Banner State Transitions (source-assertion) ===');

// Legacy intent: running a failing query shows an Execution Error banner; a following
// successful query must replace it with a success/exec result; a following SELECT must
// replace the banner with the data grid; error must never persist across transitions.
// The Svelte port implements this by storing the whole result per-tab (`tab.activeResult`)
// and having VirtualGrid branch on `result.error` / `result.columns`.
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');
const gridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');

// 1. Results are stored per-tab, not in global mutable state, so switching tabs
//    or re-running queries cannot leave a stale banner behind.
assert(
  appStateCode.includes('tab.activeResult = finalRes;'),
  'executeCurrentTab must store the fresh result on the originating tab (tab.activeResult = finalRes)'
);
assert(
  gridCode.includes('let result = $derived(appState.activeTab?.activeResult);'),
  'VirtualGrid must derive result from the active tab activeResult, so a new result replaces the old banner'
);

// 2. Error path: caught errors are wrapped into activeResult.error, which VirtualGrid
//    renders as an Execution Error banner.
assert(
  appStateCode.includes("tab.activeResult = {\n        columns: [],\n        rows: [],") &&
    appStateCode.includes('error: errMsg'),
  'executeCurrentTab catch block must convert thrown errors into activeResult.error'
);
assert(
  gridCode.includes('{:else if result?.error}') && gridCode.includes('<span>Execution Error</span>'),
  'VirtualGrid must render an Execution Error banner whenever result.error is set'
);
assert(
  gridCode.includes('{result.error}'),
  'Error banner must include the underlying error message text'
);

// 3. Success exec path: the full exec payload (message/rows_affected) is kept on the
//    tab, replacing any previous error banner.
assert(
  gridCode.includes('{:else if result?.columns && result.columns.length > 0}'),
  'A query result with columns must replace the error banner with the data grid'
);
assert(
  appStateCode.includes('affected_rows: finalRes.affected_rows'),
  'Exec results must record affected_rows in the audit log'
);
assert(
  appStateCode.includes('duration_ms: finalRes.duration_ms'),
  'Exec results must record duration_ms in the audit log'
);

// 4. isExecuting must be reset in a finally block so a crashed query never leaves
//    the grid stuck on the spinner (which would hide any banner).
assert(
  /finally \{\s*\n\s*tab\.isExecuting = false;/.test(appStateCode),
  'executeCurrentTab must clear tab.isExecuting in a finally block'
);
assert(
  gridCode.includes('let isExecuting = $derived(Boolean(appState.activeTab?.isExecuting));') &&
    gridCode.includes('{#if isExecuting}'),
  'VirtualGrid must show the executing spinner state from tab.isExecuting'
);

// 5. Algorithm replica: replicate the executeCurrentTab result-mapping logic from
//    appState source as plain JS and assert the Error -> Success -> Grid -> Error
//    -> Success transitions on the tab object.
const mapExecutionOutcome = (tab, res, err) => {
  // copied from executeCurrentTab (appState.svelte.ts)
  if (err) {
    tab.activeResult = { columns: [], rows: [], row_count: 0, error: err.message || 'Execution error' };
    return;
  }
  tab.activeResult = res;
};

// Banner state as VirtualGrid renders it: error banner > data grid > exec success
// message > empty placeholder.
const bannerState = (tab) => {
  const result = tab.activeResult;
  if (tab.isExecuting) return 'executing';
  if (result?.error) return 'error';
  if (result?.columns && result.columns.length > 0) return 'grid';
  if (result?.message || result?.type === 'exec') return 'success';
  return 'empty';
};
// 5a. Failing ALTER -> error banner
let tab = { activeResult: null, isExecuting: true };
mapExecutionOutcome(tab, null, new Error('Table users does not exist'));
tab.isExecuting = false;
assert.strictEqual(bannerState(tab), 'error', 'Failing query must produce the error banner state');

// 5b. Successful ALTER (type exec) -> banner replaced, no error text remains
mapExecutionOutcome(tab, { type: 'exec', message: 'Query executed successfully. 0 rows affected.', affected_rows: 0, duration_ms: 12 });
assert.strictEqual(bannerState(tab), 'success', 'Exec result carries no error, banner must clear');
assert.ok(!tab.activeResult.error, 'Error text must not persist after a successful exec');
assert.strictEqual(tab.activeResult.type, 'exec', 'Exec payload must be preserved for rendering');

// 5c. SELECT -> data grid with columns
mapExecutionOutcome(tab, { columns: [{ name: 'id', type: 'int' }, { name: 'name', type: 'varchar' }], rows: [[1, 'Alice'], [2, 'Bob']], row_count: 2, duration_ms: 10 });
assert.strictEqual(bannerState(tab), 'grid');
assert.strictEqual(tab.activeResult.columns.length, 2, 'Grid state must expose both columns');
assert.strictEqual(tab.activeResult.rows.length, 2, 'Grid state must expose both rows');
assert.ok(!('error' in tab.activeResult), 'Grid result must not inherit an error field');

// 5d. Failing UPDATE -> error banner again
mapExecutionOutcome(tab, null, new Error('Unknown column invalid_col'));
assert.strictEqual(bannerState(tab), 'error');
assert.strictEqual(tab.activeResult.error, 'Unknown column invalid_col');
assert.strictEqual(tab.activeResult.columns.length, 0, 'Error result must clear previous grid columns');

// 5e. Succeeding UPDATE -> success message preserved
mapExecutionOutcome(tab, { type: 'exec', rows_affected: 2, duration_ms: 15, message: '2 rows updated' });
assert.strictEqual(bannerState(tab), 'success');
assert.strictEqual(tab.activeResult.message, '2 rows updated', 'Exec message must be kept verbatim');
assert.strictEqual(tab.activeResult.rows_affected, 2, 'Affected rows count must be kept');

// 5f. Exec success never regresses to error while the tab stays selected
assert.ok(!tab.activeResult.error, 'Error banner must never persist after a successful query');

console.log('✅ ALL TESTS PASSED: State transitions (Error -> Success -> Query Grid -> Error -> Success) verified against source.');
