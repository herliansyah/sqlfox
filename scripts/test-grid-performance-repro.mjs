import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

console.log("--- Starting Grid Large Dataset (1000 rows, 40 cols) Performance & Pagination Regression Suite ---");

const html = fs.readFileSync('web/dist/index.html', 'utf8');

const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
if (lastScriptStart === -1 || lastScriptEnd === -1) {
  console.error("Could not find main <script> tag in web/dist/index.html");
  process.exit(2);
}
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

const sessionStorageMap = new Map();
const sessionStorageMock = {
  getItem: (k) => (sessionStorageMap.has(k) ? sessionStorageMap.get(k) : null),
  setItem: (k, v) => sessionStorageMap.set(k, String(v)),
  removeItem: (k) => sessionStorageMap.delete(k),
  clear: () => sessionStorageMap.clear()
};

const elementsMap = new Map();
const domElementStub = (id = '', tagName = 'div') => ({
  id,
  tagName: tagName.toUpperCase(),
  children: [],
  classList: {
    classes: new Set(),
    add(c) { this.classes.add(c); },
    remove(c) { this.classes.delete(c); },
    contains(c) { return this.classes.has(c); },
    toggle(c, force) {
      if (force === undefined) {
        if (this.classes.has(c)) this.classes.delete(c); else this.classes.add(c);
      } else if (force) {
        this.classes.add(c);
      } else {
        this.classes.delete(c);
      }
    }
  },
  dataset: {},
  innerHTML: '',
  textContent: '',
  value: '',
  style: {},
  disabled: false,
  matches(selector) { return false; },
  closest(selector) { return null; },
  querySelector(selector) {
    if (selector === 'button:first-child') {
      if (!this._firstBtn) this._firstBtn = domElementStub('', 'button');
      return this._firstBtn;
    }
    if (selector === 'button:last-child') {
      if (!this._lastBtn) this._lastBtn = domElementStub('', 'button');
      return this._lastBtn;
    }
    return null;
  },
  querySelectorAll(selector) { return []; },
  setAttribute(k, v) { this[k] = v; },
  getAttribute(k) { return this[k] || null; },
  focus() {},
  blur() {},
  showModal() { this.open = true; },
  close() { this.open = false; }
});

const getOrCreateElement = (id, tag = 'div') => {
  if (!elementsMap.has(id)) {
    elementsMap.set(id, domElementStub(id, tag));
  }
  return elementsMap.get(id);
};

const documentMock = {
  getElementById: (id) => getOrCreateElement(id),
  querySelector: (sel) => {
    if (sel.startsWith('#')) return getOrCreateElement(sel.substring(1));
    return domElementStub();
  },
  querySelectorAll: (sel) => [],
  addEventListener: () => {},
  removeEventListener: () => {}
};

const windowMock = {
  sessionStorage: sessionStorageMock,
  localStorage: sessionStorageMock,
  document: documentMock,
  innerWidth: 1200,
  innerHeight: 800,
  addEventListener: () => {},
  dispatchEvent: () => {}
};

const sandbox = {
  window: windowMock,
  document: documentMock,
  sessionStorage: sessionStorageMock,
  localStorage: sessionStorageMock,
  console: console,
  performance: performance,
  domElementStub: domElementStub,
  setTimeout: (fn) => fn(),
  clearTimeout: () => {},
  setInterval: () => {},
  clearInterval: () => {},
  fetch: async () => ({ ok: true, json: async () => ({}) })
};

vm.createContext(sandbox);
vm.runInContext(scriptContent, sandbox);

// 1. Benchmark: Render 1,000 rows x 40 cols
const runResult = vm.runInContext(`
  const numRows = 1000;
  const numCols = 40;
  const cols = [];
  for (let c = 0; c < numCols; c++) {
    cols.push({ name: 'col_' + c, type: c % 3 === 0 ? 'varchar(255)' : (c % 3 === 1 ? 'int' : 'datetime') });
  }
  const rows = [];
  for (let r = 0; r < numRows; r++) {
    const row = [];
    for (let c = 0; c < numCols; c++) {
      row.push(c % 3 === 0 ? 'string_val_' + r + '_' + c : (c % 3 === 1 ? (r * 100 + c) : '2026-03-31 10:20:30'));
    }
    rows.push(row);
  }

  // Active tab setup (Query Editor tab)
  const qTab = {
    id: 'tab-query-perf',
    title: 'perf-test.sql',
    sql: 'SELECT * FROM big_table;',
    isTableViewer: false,
    queryPage: 0
  };
  tabs.length = 0;
  tabs.push(qTab);
  activeTabId = qTab.id;

  const start = performance.now();
  renderGridResults(cols, rows, 'col_0', 'big_table', 'testdb');
  const duration = performance.now() - start;

  const tbody = document.getElementById('grid-tbody');
  const innerHTMLSize = tbody.innerHTML.length;
  const tdCount = (tbody.innerHTML.match(/<td/g) || []).length;
  const buttonCount = (tbody.innerHTML.match(/<button/g) || []).length;
  const trCount = (tbody.innerHTML.match(/<tr/g) || []).length;

  const stats = document.getElementById('grid-result-stats').textContent;
  const paginationBar = document.getElementById('grid-pagination-bar');
  const isPaginationBarHidden = paginationBar.classList.contains('hidden');
  const prevBtn = document.getElementById('btn-page-prev');
  const nextBtn = document.getElementById('btn-page-next');

  ({
    durationMs: duration,
    innerHTMLBytes: innerHTMLSize,
    trCount,
    tdCount,
    buttonCount,
    stats,
    isPaginationBarHidden,
    prevDisabled: prevBtn.disabled,
    nextDisabled: nextBtn.disabled
  });
`, sandbox);

console.log("Initial Render Benchmark Results:", runResult);

// Verify performance constraints
const MAX_ACCEPTABLE_HTML_BYTES = 2.5 * 1024 * 1024; // 2.5MB
const MAX_ACCEPTABLE_TD_COUNT = 3000;

assert(
  runResult.tdCount <= MAX_ACCEPTABLE_TD_COUNT,
  'FAILURE: Grid rendered ' + runResult.tdCount + ' TD cells, exceeding ' + MAX_ACCEPTABLE_TD_COUNT
);

assert(
  runResult.innerHTMLBytes <= MAX_ACCEPTABLE_HTML_BYTES,
  'FAILURE: tbody.innerHTML is ' + (runResult.innerHTMLBytes / (1024 * 1024)).toFixed(2) + ' MB'
);

assert.strictEqual(runResult.trCount, 50, 'Page 0 must render exactly 50 rows in DOM');
assert.strictEqual(runResult.isPaginationBarHidden, false, 'Pagination bar should be visible for 1,000 rows');
assert.strictEqual(runResult.prevDisabled, true, 'Prev button should be disabled on first page');
assert.strictEqual(runResult.nextDisabled, false, 'Next button should be enabled on first page');
assert(runResult.stats.includes('1 - 50 of 1000 rows'), 'Stats text should state "1 - 50 of 1000 rows", got: ' + runResult.stats);

console.log("✓ Benchmark & Initial Page 0 verification passed!");

// 2. Behavioral Test: Next Page Navigation (Page 1)
const page1Result = vm.runInContext(`
  loadNextPage();
  const curTab1 = tabs.find(t => t.id === activeTabId);
  const tbody1 = document.getElementById('grid-tbody');
  const stats1 = document.getElementById('grid-result-stats').textContent;
  const firstDataRowMatch1 = tbody1.innerHTML.match(/data-row="(\\d+)"/);
  const firstDataRow1 = firstDataRowMatch1 ? parseInt(firstDataRowMatch1[1], 10) : -1;
  const prevBtn1 = document.getElementById('btn-page-prev');
  const nextBtn1 = document.getElementById('btn-page-next');

  ({
    page: curTab1.queryPage,
    stats: stats1,
    firstDataRow: firstDataRow1,
    prevDisabled: prevBtn1.disabled,
    nextDisabled: nextBtn1.disabled
  });
`, sandbox);

console.log("Page 1 Navigation Result:", page1Result);
assert.strictEqual(page1Result.page, 1, 'activeTab.queryPage should be 1');
assert.strictEqual(page1Result.firstDataRow, 50, 'First row rendered on page 1 must have data-row="50"');
assert(page1Result.stats.includes('51 - 100 of 1000 rows'), 'Stats should show "51 - 100 of 1000 rows"');
assert.strictEqual(page1Result.prevDisabled, false, 'Prev button must be enabled on page 1');
assert.strictEqual(page1Result.nextDisabled, false, 'Next button must remain enabled on page 1');

console.log("✓ Next Page (Page 1) navigation passed!");

// 3. Behavioral Test: Cell selection on row 75
const cellSelectionResult = vm.runInContext(`
  const dummyTd = domElementStub('', 'td');
  selectGridCell(dummyTd, 75, 2);
  ({
    selectedRow: activeGridSelectedRow,
    selectedCol: activeGridSelectedCol,
    hasSelectedClass: dummyTd.classList.contains('cell-selected')
  });
`, sandbox);

console.log("Cell selection result:", cellSelectionResult);
assert.strictEqual(cellSelectionResult.selectedRow, 75, 'activeGridSelectedRow must be 75');
assert.strictEqual(cellSelectionResult.selectedCol, 2, 'activeGridSelectedCol must be 2');
assert.strictEqual(cellSelectionResult.hasSelectedClass, true, 'cell-selected class should be present');

console.log("✓ Cell selection on paged row passed!");

// 4. Behavioral Test: Previous Page Navigation (Back to Page 0)
const backToPage0Result = vm.runInContext(`
  loadPreviousPage();
  const curTab0 = tabs.find(t => t.id === activeTabId);
  const tbody0 = document.getElementById('grid-tbody');
  const stats0 = document.getElementById('grid-result-stats').textContent;
  const firstDataRowMatch0 = tbody0.innerHTML.match(/data-row="(\\d+)"/);
  const firstDataRow0 = firstDataRowMatch0 ? parseInt(firstDataRowMatch0[1], 10) : -1;

  ({
    page: curTab0.queryPage,
    stats: stats0,
    firstDataRow: firstDataRow0
  });
`, sandbox);

console.log("Back to Page 0 Navigation Result:", backToPage0Result);
assert.strictEqual(backToPage0Result.page, 0, 'activeTab.queryPage should be back to 0');
assert.strictEqual(backToPage0Result.firstDataRow, 0, 'First row rendered on page 0 must have data-row="0"');
assert(backToPage0Result.stats.includes('1 - 50 of 1000 rows'), 'Stats should show "1 - 50 of 1000 rows"');

console.log("✓ Previous Page (Back to Page 0) navigation passed!");

// 5. Verification: currentResults retains complete 1,000 rows for export
const exportCheck = vm.runInContext(`
  ({
    currentResultsCount: currentResults.length,
    rawQueryRowsCount: tabs.find(t => t.id === activeTabId).rawQueryRows.length
  });
`, sandbox);

assert.strictEqual(exportCheck.currentResultsCount, 1000, 'currentResults must still hold all 1000 rows');
assert.strictEqual(exportCheck.rawQueryRowsCount, 1000, 'rawQueryRows must still hold all 1000 rows');
console.log("✓ Full 1,000 rows preserved in memory for exports!");

console.log("\n🎉 ALL LARGE DATASET PERFORMANCE & PAGINATION TESTS PASSED 100%!");
