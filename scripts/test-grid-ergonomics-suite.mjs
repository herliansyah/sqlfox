import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

console.log("--- Starting Grid Ergonomics Suite Test ---");

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// 1. Verify HTML DOM elements exist
assert(html.includes('id="grid-ctx-filter-section"'), 'grid-ctx-filter-section HTML must exist in context menu');
assert(html.includes('id="grid-ctx-filter-options"'), 'grid-ctx-filter-options HTML must exist in context menu');
assert(html.includes('id="btn-col-picker"'), 'btn-col-picker button must exist in toolbar');
assert(html.includes('id="col-picker-count-badge"'), 'col-picker-count-badge must exist in toolbar');
assert(html.includes('id="col-picker-dropdown"'), 'col-picker-dropdown must exist in toolbar');
assert(html.includes('id="col-picker-search"'), 'col-picker-search input must exist');
assert(html.includes('id="col-picker-list"'), 'col-picker-list container must exist');
assert(html.includes('id="record-inspector-dialog"'), 'record-inspector-dialog native dialog must exist');
assert(html.includes('id="record-inspector-fields"'), 'record-inspector-fields container must exist');
assert(html.includes('id="record-inspector-pager"'), 'record-inspector-pager container must exist');
assert(html.includes('id="record-inspector-search"'), 'record-inspector-search input must exist');

// 2. Extract Javascript from the main application <script> tag
const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
if (lastScriptStart === -1 || lastScriptEnd === -1) {
  console.error("Could not find main <script> tag in web/dist/index.html");
  process.exit(2);
}
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

// 3. Build browser sandbox
const sessionStorageMap = new Map();
const sessionStorageMock = {
  getItem: (k) => (sessionStorageMap.has(k) ? sessionStorageMap.get(k) : null),
  setItem: (k, v) => sessionStorageMap.set(k, String(v)),
  removeItem: (k) => sessionStorageMap.delete(k),
  clear: () => sessionStorageMap.clear()
};

const localStorageMap = new Map();
const localStorageMock = {
  getItem: (k) => (localStorageMap.has(k) ? localStorageMap.get(k) : null),
  setItem: (k, v) => localStorageMap.set(k, String(v)),
  removeItem: (k) => localStorageMap.delete(k),
  clear: () => localStorageMap.clear()
};

let appliedFiltersCalled = 0;
let alertCalls = [];

const domElementStub = (id = '', tagName = 'div') => ({
  id,
  tagName: tagName.toUpperCase(),
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
  matches(selector) { return false; },
  closest(selector) { return null; },
  querySelector(selector) { return null; },
  querySelectorAll(selector) { return []; },
  setAttribute(k, v) { this[k] = v; },
  getAttribute(k) { return this[k] || null; },
  focus() {},
  blur() {},
  showModal() { this.open = true; },
  close() { this.open = false; }
});

const elementsMap = new Map();
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
  localStorage: localStorageMock,
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
  localStorage: localStorageMock,
  console: {
    log: () => {},
    warn: () => {},
    error: () => {}
  },
  setTimeout: (fn) => fn(),
  clearTimeout: () => {},
  setInterval: () => {},
  clearInterval: () => {},
  fetch: async () => ({ ok: true, json: async () => ({}) })
};

vm.createContext(sandbox);

try {
  vm.runInContext(scriptContent, sandbox);
} catch (e) {
  console.error("Syntax or runtime error evaluating index.html script:", e);
  process.exit(1);
}

// 4. Test Functions & Logic
assert(typeof sandbox.applyQuickFilterFromCell === 'function', 'applyQuickFilterFromCell must be defined as a function');
assert(typeof sandbox.toggleColumnPickerDropdown === 'function', 'toggleColumnPickerDropdown must be defined as a function');
assert(typeof sandbox.toggleColumnVisibility === 'function', 'toggleColumnVisibility must be defined as a function');
assert(typeof sandbox.showAllColumns === 'function', 'showAllColumns must be defined as a function');
assert(typeof sandbox.hideAllColumns === 'function', 'hideAllColumns must be defined as a function');
assert(typeof sandbox.openRecordInspector === 'function', 'openRecordInspector must be defined as a function');
assert(typeof sandbox.navigateRecordInspector === 'function', 'navigateRecordInspector must be defined as a function');
assert(typeof sandbox.onRecordFieldChange === 'function', 'onRecordFieldChange must be defined as a function');

// 5. Behavioral Test: Quick-Filters
vm.runInContext(`
  tabs = [{
    id: 'tab-test',
    isTableViewer: true,
    tableName: 'users',
    filterPills: []
  }];
  activeTabId = 'tab-test';

  applyQuickFilterFromCell('status', '=', 'active');
  const t = tabs[0];
  if (t.filterPills.length !== 1) throw new Error('filterPills should have 1 pill');
  if (t.filterPills[0].col !== 'status') throw new Error('filterPill col should be status');
  if (t.filterPills[0].op !== '=') throw new Error('filterPill op should be =');
  if (t.filterPills[0].val !== 'active') throw new Error('filterPill val should be active');

  applyQuickFilterFromCell('deleted_at', 'IS NULL', '');
  if (t.filterPills.length !== 2) throw new Error('filterPills should have 2 pills');
  if (t.filterPills[1].col !== 'deleted_at') throw new Error('filterPill col should be deleted_at');
  if (t.filterPills[1].op !== 'IS NULL') throw new Error('filterPill op should be IS NULL');
`, sandbox);
console.log("✓ Quick-Filter behavioral tests passed!");

// 6. Behavioral Test: Column Visibility Picker
vm.runInContext(`
  currentColumns = [
    { name: 'id', type: 'bigint' },
    { name: 'email', type: 'varchar(255)' },
    { name: 'created_at', type: 'datetime' }
  ];
  activeGridPKCol = 'id';
  activeGridTable = 'users';

  toggleColumnVisibility('created_at', false);
  const t2 = tabs.find(x => x.id === activeTabId);
  if (!t2.hiddenColumns.has('created_at')) throw new Error('created_at should be in hiddenColumns');
  if (!isColumnHidden('created_at', 'users')) throw new Error('isColumnHidden(created_at) should return true');
  if (isColumnHidden('email', 'users')) throw new Error('isColumnHidden(email) should return false');
  if (isColumnHidden('id', 'users')) throw new Error('PK column id should NEVER be hidden');

  hideAllColumns();
  if (!t2.hiddenColumns.has('created_at')) throw new Error('hideAllColumns should hide created_at');
  if (!t2.hiddenColumns.has('email')) throw new Error('hideAllColumns should hide email');
  if (t2.hiddenColumns.has('id')) throw new Error('hideAllColumns should keep PK id visible');
  if (isColumnHidden('id', 'users')) throw new Error('isColumnHidden(id) should be false after hideAllColumns');

  showAllColumns();
  if (t2.hiddenColumns.size !== 0) throw new Error('showAllColumns should clear hiddenColumns');
`, sandbox);
console.log("✓ Column Visibility Picker behavioral tests passed!");

// 7. Behavioral Test: Vertical Record Inspector & Navigation & stagedEdits
vm.runInContext(`
  currentResults = [
    ['101', 'alice@test.com', '2026-01-01'],
    ['102', 'bob@test.com', '2026-01-02']
  ];

  openRecordInspector(0);
  if (activeInspectorRowIdx !== 0) throw new Error('activeInspectorRowIdx should be 0');

  navigateRecordInspector(1);
  if (activeInspectorRowIdx !== 1) throw new Error('navigateRecordInspector(1) should move to row 1');

  navigateRecordInspector(1);
  if (activeInspectorRowIdx !== 1) throw new Error('navigateRecordInspector should clamp to last row');

  navigateRecordInspector(-1);
  if (activeInspectorRowIdx !== 0) throw new Error('navigateRecordInspector(-1) should move back to row 0');

  onRecordFieldChange('email', 1, 'alice_updated@test.com');
  const stagedKey = stagedKeyFor('users', 0, 1);
  if (!stagedEdits.has(stagedKey)) throw new Error('stagedEdits should record change on users:0:1');
  if (stagedEdits.get(stagedKey).newVal !== 'alice_updated@test.com') throw new Error('stagedEdits should have updated newVal');
`, sandbox);
console.log("✓ Record Inspector behavioral & stagedEdits tests passed!");

// 8. Regression: staged edits follow the row, not the array index (sort/filter must not re-target them)
vm.runInContext(`
  stagedEdits.clear();
  const rowA = [1, 'a@test.com', 'x'];
  const rowB = [2, 'b@test.com', 'y'];
  currentResults = [rowA, rowB];
  stageGridEdit(stagedKeyFor('users', 0, 1), { rIdx: 0, cIdx: 1, table: 'users', colName: 'email', oldVal: 'a@test.com', newVal: 'a2@test.com' });
  rowA[1] = 'a2@test.com';
  stageGridEdit(stagedKeyFor('users', 0, 1), { rIdx: 0, cIdx: 1, table: 'users', colName: 'email', oldVal: 'a2@test.com', newVal: 'a3@test.com' });
  rowA[1] = 'a3@test.com';
  if (stagedEdits.size !== 1) throw new Error('re-editing a cell must update one staged entry');
  if ([...stagedEdits.values()][0].oldVal !== 'a@test.com') throw new Error('re-edit must keep the ORIGINAL old value');
  currentResults = [rowB, rowA]; // sorted
  if (!stagedEdits.has(stagedKeyFor('users', 1, 1))) throw new Error('staged edit must follow its row after sort');
  if (stagedEdits.has(stagedKeyFor('users', 0, 1))) throw new Error('another row must not inherit the staged edit');
  for (const item of stagedEdits.values()) { (item.rowRef || currentResults[item.rIdx])[item.cIdx] = item.oldVal; }
  if (rowA[1] !== 'a@test.com' || rowB[1] !== 'b@test.com') throw new Error('discard must restore the edited row only');
`, sandbox);
console.log("✓ Staged-edit row identity regression passed!");

// 9. Behavioral Test: Grid Context Menu Icon Slot Normalization (consistent w-5 slot)
vm.runInContext(`
  tabs = [{
    id: 'tab-align-test',
    isTableViewer: true,
    tableName: 'users',
    filterPills: []
  }];
  activeTabId = 'tab-align-test';
  activeGridTable = 'users';
  activeGridPKCol = 'id';
  currentColumns = [
    { name: 'id', type: 'int' },
    { name: 'name', type: 'varchar(100)' }
  ];
  currentResults = [[1, 'Alice']];

  const fakeTd = {
    dataset: { col: '1' },
    closest: (sel) => fakeTd,
    classList: { add: () => {}, remove: () => {} }
  };
  const fakeEvent = {
    preventDefault: () => {},
    target: fakeTd,
    clientX: 200,
    clientY: 200
  };

  openGridContextMenu(fakeEvent, 0);

  const filterOptsHtml = document.getElementById('grid-ctx-filter-options').innerHTML;
  const insertOptsHtml = document.getElementById('grid-ctx-insert-options').innerHTML;

  if (!filterOptsHtml.includes('w-5 text-center flex-shrink-0 inline-flex items-center justify-center')) {
    throw new Error('Quick filter option icons must use normalized w-5 container for consistent indentation');
  }
  if (!insertOptsHtml.includes('w-5 text-center flex-shrink-0 inline-flex items-center justify-center')) {
    throw new Error('Insert value option icons must use normalized w-5 container for consistent indentation');
  }

  // Also verify NULL value branch in quick filter
  currentResults = [[1, null]];
  openGridContextMenu(fakeEvent, 0);
  const filterNullHtml = document.getElementById('grid-ctx-filter-options').innerHTML;
  if (!filterNullHtml.includes('w-5 text-center flex-shrink-0 inline-flex items-center justify-center')) {
    throw new Error('Null quick filter options must use normalized w-5 container');
  }
`, sandbox);
console.log("✓ Grid context menu icon slot normalization test passed!");

console.log("\n🎉 ALL GRID ERGONOMICS SUITE TESTS PASSED 100%!");
