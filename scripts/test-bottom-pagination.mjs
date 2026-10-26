import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

console.log("--- Starting Bottom Pagination Bar Comprehensive Test ---");

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// Verify HTML DOM elements exist
assert(html.includes('id="grid-pagination-bar"'), 'grid-pagination-bar HTML must exist');
assert(html.includes('id="grid-bottom-stats"'), 'grid-bottom-stats HTML must exist');
assert(html.includes('id="btn-count-accurate"'), 'btn-count-accurate HTML must exist');
assert(html.includes('id="grid-page-size-select"'), 'grid-page-size-select HTML must exist');
assert(html.includes('id="btn-page-first"'), 'btn-page-first HTML must exist');
assert(html.includes('id="btn-page-prev"'), 'btn-page-prev HTML must exist');
assert(html.includes('id="grid-page-input"'), 'grid-page-input HTML must exist');
assert(html.includes('id="grid-total-pages"'), 'grid-total-pages HTML must exist');
assert(html.includes('id="btn-page-next"'), 'btn-page-next HTML must exist');
assert(html.includes('id="btn-page-last"'), 'btn-page-last HTML must exist');

// Extract JS
const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

// Mock DOM
const domElements = new Map();
function createMockEl(id = '', tag = 'div') {
  return {
    id,
    tagName: tag.toUpperCase(),
    value: '',
    innerHTML: '',
    textContent: '',
    max: '',
    disabled: false,
    dataset: {},
    classList: {
      set: new Set(),
      add(c) { this.set.add(c); },
      remove(c) { this.set.delete(c); },
      toggle(c, f) {
        if (f === undefined) { if (this.set.has(c)) this.set.delete(c); else this.set.add(c); }
        else if (f) this.set.add(c);
        else this.set.delete(c);
      },
      contains(c) { return this.set.has(c); }
    },
    style: {},
    children: [],
    querySelectorAll() { return []; },
    querySelector() { return null; },
    addEventListener() {},
    setAttribute(k, v) { this[k] = v; },
    getAttribute(k) { return this[k] || null; },
    removeAttribute(k) { delete this[k]; },
    focus() {},
    blur() {}
  };
}

function getEl(id, tag = 'div') {
  if (!domElements.has(id)) {
    domElements.set(id, createMockEl(id, tag));
  }
  return domElements.get(id);
}

const mockDocument = {
  getElementById: (id) => getEl(id),
  createElement: (tag) => createMockEl('', tag),
  querySelectorAll: () => [],
  querySelector: () => null,
  addEventListener: () => {}
};

const sandbox = {
  document: mockDocument,
  window: {
    addEventListener: () => {},
    location: { origin: 'http://localhost' },
    localStorage: { getItem: () => null, setItem: () => {} },
    sessionStorage: { getItem: () => null, setItem: () => {} }
  },
  localStorage: { getItem: () => null, setItem: () => {} },
  sessionStorage: { getItem: () => null, setItem: () => {} },
  setTimeout: () => {},
  setInterval: () => {},
  clearTimeout: () => {},
  clearInterval: () => {},
  console: console,
  fetch: () => {},
  apiFetch: async () => ({ ok: true, json: async () => ({}) }),
  CodeMirror: {
    fromTextArea: () => ({ getValue: () => '', setValue: () => {}, on: () => {}, getWrapperElement: () => createMockEl(), refresh: () => {} })
  }
};

vm.createContext(sandbox);
vm.runInContext(scriptContent, sandbox);

// TEST 1: Query Results In-Memory Pagination
vm.runInContext(`
  tabs = [{
    id: 'tab-query',
    title: 'test.sql',
    isTableViewer: false,
    queryPage: 0,
    pageSize: 50
  }];
  activeTabId = 'tab-query';
  currentColumns = [{ name: 'id', type: 'INT' }, { name: 'val', type: 'VARCHAR' }];
  // Generate 250 rows
  currentResults = Array.from({ length: 250 }, (_, i) => [i + 1, 'item_' + (i + 1)]);
  renderGridResults(currentColumns, currentResults);
`, sandbox);

const qStats0 = vm.runInContext(`document.getElementById('grid-bottom-stats').textContent`, sandbox);
const qTotalPages0 = vm.runInContext(`document.getElementById('grid-total-pages').textContent`, sandbox);
const qPageInput0 = vm.runInContext(`document.getElementById('grid-page-input').value`, sandbox);
const qBtnFirst0 = vm.runInContext(`document.getElementById('btn-page-first').disabled`, sandbox);
const qBtnPrev0 = vm.runInContext(`document.getElementById('btn-page-prev').disabled`, sandbox);
const qBtnNext0 = vm.runInContext(`document.getElementById('btn-page-next').disabled`, sandbox);
const qBtnLast0 = vm.runInContext(`document.getElementById('btn-page-last').disabled`, sandbox);

console.log('Test 1 (Initial page 1 of 5):', { stats: qStats0, totalPages: qTotalPages0, page: qPageInput0, firstDisabled: qBtnFirst0, nextDisabled: qBtnNext0 });
assert(qStats0.includes('1 - 50 of 250 rows'), 'Expected stats to state 1 - 50 of 250, got: ' + qStats0);
assert.strictEqual(qTotalPages0, '5', 'Expected 5 total pages');
assert.strictEqual(qPageInput0, 1, 'Expected page input 1');
assert.strictEqual(qBtnFirst0, true, 'First button should be disabled on page 1');
assert.strictEqual(qBtnPrev0, true, 'Prev button should be disabled on page 1');
assert.strictEqual(qBtnNext0, false, 'Next button should be enabled on page 1');
assert.strictEqual(qBtnLast0, false, 'Last button should be enabled on page 1');

// Next page navigation
vm.runInContext(`goToNextPage();`, sandbox);
const qStats1 = vm.runInContext(`document.getElementById('grid-bottom-stats').textContent`, sandbox);
const qPageInput1 = vm.runInContext(`document.getElementById('grid-page-input').value`, sandbox);
console.log('Test 1 (Page 2):', { stats: qStats1, page: qPageInput1 });
assert(qStats1.includes('51 - 100 of 250 rows'), 'Expected 51 - 100, got: ' + qStats1);
assert.strictEqual(qPageInput1, 2);

// Jump to last page
vm.runInContext(`goToLastPage();`, sandbox);
const qStatsLast = vm.runInContext(`document.getElementById('grid-bottom-stats').textContent`, sandbox);
const qPageInputLast = vm.runInContext(`document.getElementById('grid-page-input').value`, sandbox);
const qBtnNextLast = vm.runInContext(`document.getElementById('btn-page-next').disabled`, sandbox);
const qBtnLastLast = vm.runInContext(`document.getElementById('btn-page-last').disabled`, sandbox);
console.log('Test 1 (Last page):', { stats: qStatsLast, page: qPageInputLast, nextDisabled: qBtnNextLast });
assert(qStatsLast.includes('201 - 250 of 250 rows'), 'Expected 201 - 250, got: ' + qStatsLast);
assert.strictEqual(qPageInputLast, 5);
assert.strictEqual(qBtnNextLast, true, 'Next button should be disabled on last page');
assert.strictEqual(qBtnLastLast, true, 'Last button should be disabled on last page');

// Jump directly to page 3
vm.runInContext(`goToPage(3);`, sandbox);
const qStatsPage3 = vm.runInContext(`document.getElementById('grid-bottom-stats').textContent`, sandbox);
assert(qStatsPage3.includes('101 - 150 of 250 rows'), 'Expected 101 - 150, got: ' + qStatsPage3);

// Change page size to 100
vm.runInContext(`onGridPageSizeChange(100);`, sandbox);
const qStats100 = vm.runInContext(`document.getElementById('grid-bottom-stats').textContent`, sandbox);
const qTotalPages100 = vm.runInContext(`document.getElementById('grid-total-pages').textContent`, sandbox);
console.log('Test 1 (Page size 100):', { stats: qStats100, totalPages: qTotalPages100 });
assert(qStats100.includes('1 - 100 of 250 rows'), 'Expected 1 - 100 with size 100, got: ' + qStats100);
assert.strictEqual(qTotalPages100, '3', 'Expected 3 pages with 100 page size');

// TEST 2: Table Viewer Server-side Pagination
vm.runInContext(`
  schemaCatalogCache = {
    tables: [{
      name: 'customers',
      row_count: 1500,
      columns: [{ name: 'id', type: 'INT' }, { name: 'name', type: 'VARCHAR' }]
    }]
  };
  let loadedCalls = [];
  loadTableRows = (tbl) => { loadedCalls.push({ tbl, offset: tabs[0].offset, pageSize: tabs[0].pageSize }); };
  tabs = [{
    id: 'tab-table',
    title: 'customers',
    tableName: 'customers',
    isTableViewer: true,
    tableMode: 'data',
    offset: 0,
    pageSize: 50
  }];
  activeTabId = 'tab-table';
  activeGridTable = 'customers';
  // Simulate 50 rows returned
  renderGridResults(
    [{ name: 'id', type: 'INT' }, { name: 'name', type: 'VARCHAR' }],
    Array.from({ length: 50 }, (_, i) => [i + 1, 'cust_' + (i + 1)]),
    'id',
    'customers'
  );
`, sandbox);

const tblStats0 = vm.runInContext(`document.getElementById('grid-bottom-stats').textContent`, sandbox);
const tblPages0 = vm.runInContext(`document.getElementById('grid-total-pages').textContent`, sandbox);
const tblPage0 = vm.runInContext(`document.getElementById('grid-page-input').value`, sandbox);
console.log('Test 2 Table Viewer (Initial):', { stats: tblStats0, totalPages: tblPages0, page: tblPage0 });
assert(tblStats0.includes('1 - 50 of 1,500 rows'), 'Expected 1 - 50 of 1,500, got: ' + tblStats0);
assert.strictEqual(tblPages0, '30', '1500 / 50 = 30 pages');
assert.strictEqual(tblPage0, 1);

// Go to next page
vm.runInContext(`goToNextPage();`, sandbox);
const nextCall = vm.runInContext(`loadedCalls[loadedCalls.length - 1]`, sandbox);
console.log('Test 2 Table Viewer nextCall:', nextCall);
assert.strictEqual(nextCall.offset, 50, 'Next page should advance offset to 50');

// Go to last page
vm.runInContext(`goToLastPage();`, sandbox);
const lastCall = vm.runInContext(`loadedCalls[loadedCalls.length - 1]`, sandbox);
console.log('Test 2 Table Viewer lastCall:', lastCall);
assert.strictEqual(lastCall.offset, 1450, 'Last page (30) offset should be 29 * 50 = 1450');

// Go to page 10
vm.runInContext(`goToPage(10);`, sandbox);
const page10Call = vm.runInContext(`loadedCalls[loadedCalls.length - 1]`, sandbox);
console.log('Test 2 Table Viewer page10Call:', page10Call);
assert.strictEqual(page10Call.offset, 450, 'Page 10 offset should be 9 * 50 = 450');

// Filtered table without exact count
vm.runInContext(`
  tabs[0].appliedFilter = "\`status\` = 'active'";
  tabs[0].offset = 0;
  renderGridResults(
    [{ name: 'id', type: 'INT' }, { name: 'name', type: 'VARCHAR' }],
    Array.from({ length: 50 }, (_, i) => [i + 1, 'cust_' + (i + 1)]),
    'id',
    'customers'
  );
`, sandbox);

const filteredStats = vm.runInContext(`document.getElementById('grid-bottom-stats').textContent`, sandbox);
const accurateBtnHidden = vm.runInContext(`document.getElementById('btn-count-accurate').classList.contains('hidden')`, sandbox);
console.log('Test 2 Filtered Table:', { stats: filteredStats, accurateBtnHidden });
assert(filteredStats.includes('Showing 1 - 50 rows (filtered)'), 'Expected filtered stats, got: ' + filteredStats);
assert.strictEqual(accurateBtnHidden, false, 'Exact Count button should be visible when filtered');

console.log("🎉 ALL BOTTOM PAGINATION TESTS PASSED 100%!");
