import fs from 'fs';
import assert from 'assert';

console.log('--- Starting Bottom Pagination Bar Comprehensive Test (source assertions) ---');
// Source-assertion pattern: read the Svelte sources directly and assert on the
// real component/store wiring instead of executing a bundled script.
const virtualGridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');
const clientCode = fs.readFileSync('web/frontend/src/lib/api/client.ts', 'utf8');

const has = (needle) => virtualGridCode.includes(needle);

// TEST 1: Pagination bar DOM structure (all legacy ids preserved in VirtualGrid.svelte)
for (const id of [
  'grid-pagination-bar',
  'grid-bottom-stats',
  'btn-count-accurate',
  'grid-page-size-select',
  'btn-page-first',
  'btn-page-prev',
  'grid-page-input',
  'grid-total-pages',
  'btn-page-next',
  'btn-page-last',
]) {
  assert(has(`id="${id}"`), `${id} must exist in VirtualGrid.svelte`);
}
console.log('✅ Test 1 Passed: all pagination bar ids exist in VirtualGrid.svelte');

// TEST 2: Stats bar — records count + exact-count affordance
assert(has('grid-bottom-stats') && has('records'), 'grid-bottom-stats must render record counts');
assert(has("appState.requestExactRowCount(currentTab)"), '#btn-count-accurate must trigger appState.requestExactRowCount');
assert(appStateCode.includes('async requestExactRowCount'), 'appState must implement requestExactRowCount for exact counts');
console.log('✅ Test 2 Passed: bottom stats bar wires exact row counting through appState');

// TEST 3: Page size select resets to page 1 and reloads table data
assert(has("id=\"grid-page-size-select\"") && has('currentTab.currentPage = 1;'), 'page-size change must reset currentPage to 1');
assert(
  /id="grid-page-size-select"[\s\S]*?appState\.loadTableData\(currentTab\)/.test(virtualGridCode),
  'page-size change must call appState.loadTableData(currentTab)'
);
assert(has('<option value={50}>50</option>') && has('<option value={1000}>1000</option>'), 'page-size select must offer 50..1000 options');
console.log('✅ Test 3 Passed: page-size select resets paging and reloads');

// TEST 4: Navigation buttons page through appState.loadTableData
assert(
  /id="btn-page-first"[\s\S]{0,400}?currentTab\.currentPage = 1;[\s\S]{0,200}?appState\.loadTableData\(currentTab\)/.test(virtualGridCode),
  'btn-page-first must jump to page 1 and reload'
);
assert(
  /id="btn-page-prev"[\s\S]{0,400}?currentTab\.currentPage = curPage - 1;/.test(virtualGridCode),
  'btn-page-prev must decrement currentPage'
);
assert(
  /id="btn-page-next"[\s\S]{0,400}?currentTab\.currentPage = curPage \+ 1;/.test(virtualGridCode),
  'btn-page-next must increment currentPage'
);
assert(
  /id="btn-page-last"[\s\S]{0,400}?currentTab\.currentPage = totalPages;/.test(virtualGridCode),
  'btn-page-last must jump to totalPages'
);
// Disabled state guards (server-paginated: next disabled when rows < pageSize or on last page)
assert(has('disabled={curPage <= 1}'), 'first/prev buttons must be disabled on page 1');
assert(has('disabled={totalRows < pageSize || (totalPages !== null && curPage >= totalPages)}'), 'next button must be disabled at end of data');
console.log('✅ Test 4 Passed: first/prev/next/last paging buttons and their guards');

// TEST 5: Editable page-jump input is clamped to the valid range
assert(
  /id="grid-page-input"[\s\S]{0,600}?Math\.max\(1, Math\.min\(totalPages \?\? target, Math\.floor\(target\)\)\)/.test(virtualGridCode),
  'grid-page-input must clamp jump target between 1 and totalPages'
);
assert(has('aria-label="Jump to page"'), 'grid-page-input must be labelled for accessibility');
console.log('✅ Test 5 Passed: page-jump input clamped to valid page range');

// TEST 6: Total pages derived from row estimate / page size (in-memory pagination parity)
assert(has('Math.ceil(estRows / pageSize)'), 'totalPages must be derived as Math.ceil(estRows / pageSize)');
assert(has('{@const pageSize = currentTab.pageSize || 50}'), 'default page size must be 50');
console.log('✅ Test 6 Passed: total pages derivation matches legacy in-memory pagination');

// TEST 7: Pagination is driven through appState.loadTableData -> api.getTableRows(db)
assert(appStateCode.includes('async loadTableData'), 'appState.loadTableData must exist');
assert(
  /loadTableData[\s\S]{0,2000}?api\.getTableRows\(/.test(appStateCode),
  'appState.loadTableData must call api.getTableRows for server-side paging'
);
assert(
  /getTableRows\(\s*sessionId: string,\s*tableName: string,\s*limit = 50,\s*offset = 0,\s*db\?: string/.test(clientCode),
  'client.getTableRows must accept limit/offset/db parameters'
);
console.log('✅ Test 7 Passed: paging flows through appState.loadTableData -> api.getTableRows');

console.log('ALL BOTTOM PAGINATION TESTS PASSED!');
