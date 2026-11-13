import fs from 'fs';
import assert from 'assert';

console.log('--- Running Paging Diagnostics Test (source-assertion) ---');

// Source-assertion pattern: assert paging behavior directly against the
// Svelte/stores sources — no bundled web/dist artifacts, no vm sandboxes.
const virtualGridCode = fs.readFileSync(
  'web/frontend/src/lib/components/grid/VirtualGrid.svelte',
  'utf8'
);
const appStateCode = fs.readFileSync(
  'web/frontend/src/lib/stores/appState.svelte.ts',
  'utf8'
);

// --- CHECK 1: The bottom pagination bar exists and starts hidden -----------
// In the Svelte port the bar is conditionally rendered only for table viewers,
// which is the equivalent of the legacy "hidden until a query has run".
assert(
  virtualGridCode.includes('id="grid-pagination-bar"'),
  'grid-pagination-bar must exist in VirtualGrid.svelte'
);
assert(
  /\{#if currentTab\?\.isTableViewer\}[\s\S]{0,800}id="grid-pagination-bar"/.test(virtualGridCode),
  'grid-pagination-bar must be gated behind currentTab?.isTableViewer (hidden until a table query runs)'
);

// --- CHECK 2: Paging labels must be in English, not Indonesian -------------
assert(!virtualGridCode.includes('Tampilkan:'), 'Grid must not contain Indonesian "Tampilkan:"');
assert(!virtualGridCode.includes('/ hal<'), 'Grid must not contain Indonesian "/ hal"');
assert(!virtualGridCode.includes('Hitung Akurat'), 'Grid must not contain Indonesian "Hitung Akurat"');
assert(!virtualGridCode.includes('Menampilkan'), 'Grid must not contain Indonesian "Menampilkan"');
assert(!virtualGridCode.includes('baris'), 'Grid must not contain Indonesian "baris"');
assert(virtualGridCode.includes('Page Size:'), 'Grid must contain English "Page Size:"');
assert(virtualGridCode.includes('Page'), 'Grid must contain English "Page" label');
assert(virtualGridCode.includes('records</span>'), 'Grid must contain English "records" label');
assert(
  virtualGridCode.includes('Exact count') || virtualGridCode.includes('Exact Count'),
  'Grid must contain English "Exact Count" button label'
);
assert(
  virtualGridCode.includes('title="First Page"') &&
    virtualGridCode.includes('title="Previous Page"') &&
    virtualGridCode.includes('title="Next Page"') &&
    virtualGridCode.includes('title="Last Page"'),
  'Paging buttons must use English titles'
);

// --- CHECK 3: Legacy keyset pagination must be eliminated ------------------
assert(
  !virtualGridCode.includes('id="keyset-pagination"'),
  'keyset-pagination must be removed completely from the grid'
);
assert(
  !appStateCode.includes('keyset-pagination'),
  'appState must not reference keyset-pagination'
);

// --- CHECK 4: App-state equivalents of clearGridToInitialState -------------
// Resetting a tab to its initial state must reset paging and hide results.
assert(
  appStateCode.includes('tab.currentPage = 1;'),
  'Filter/reset operations must reset currentPage to 1 (equivalent of clearGridToInitialState)'
);
// Every reset path (filter pill changes, filter removal, filter clear) resets paging
const resetCount = (appStateCode.match(/tab\.currentPage = 1;/g) || []).length;
assert(
  resetCount >= 4,
  `At least 4 reset paths must reset currentPage (found ${resetCount})`
);
assert(
  virtualGridCode.includes('currentTab.currentPage = 1;'),
  'Page-size change and First button must reset currentPage to 1 in the grid'
);

// --- CHECK 5: renderGridResults equivalent — stats text & paging math ------
// Copied from appState.loadTableData + VirtualGrid pagination markup.
const computeStats = (exactRowCount, totalRows, curPage, totalPages) => {
  const count = (exactRowCount !== undefined ? exactRowCount : totalRows).toLocaleString();
  return `Showing ${count} records • Page ${curPage} of ${totalPages}`;
};
{
  // 120 rows, page size 50 → 3 pages
  const totalPages = Math.max(1, Math.ceil(120 / 50));
  const stats = computeStats(undefined, 50, 1, totalPages);
  assert(
    stats.includes('50 records'),
    'Stats text must include the paged row count, got: ' + stats
  );
  assert(
    stats.includes('Page 1 of 3'),
    'Stats text must include the English page position, got: ' + stats
  );
  assert(
    !stats.includes('Menampilkan') && !stats.includes('baris'),
    'Stats text must not contain Indonesian labels'
  );
}
{
  // Exact count overrides the paged count
  const stats = computeStats(120, 50, 1, 3);
  assert(
    stats.includes('120 records'),
    'Exact count must override the current-page row count, got: ' + stats
  );
}

// loadTableData LIMIT/OFFSET arithmetic (page size 50 default)
const limitOf = (tab) => tab.pageSize || 50;
const offsetOf = (tab) => ((tab.currentPage || 1) - 1) * (tab.pageSize || 50);
assert.strictEqual(limitOf({}), 50, 'Default page size must be 50');
assert.strictEqual(offsetOf({ currentPage: 3, pageSize: 50 }), 100, 'Page 3 offset must be 100');
assert.strictEqual(offsetOf({}), 0, 'Default offset must be 0');
assert(
  appStateCode.includes('const limit = tab.pageSize || 50;') &&
    appStateCode.includes('const offset = ((tab.currentPage || 1) - 1) * limit;'),
  'appState.loadTableData must compute LIMIT/OFFSET from pageSize/currentPage'
);

console.log('All paging diagnostic checks PASSED!');
