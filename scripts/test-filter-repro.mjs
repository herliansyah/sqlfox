import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf-8');

// Extract the main script from index.html (the last and largest script block)
const scriptMatches = [...html.matchAll(/<script>([\s\S]*?)<\/script>/gi)];
const mainScript = scriptMatches[scriptMatches.length - 1][1];

// Minimal DOM mock
const elements = {};
function createMockElement(id = '', tag = 'div') {
  return {
    id,
    tagName: tag.toUpperCase(),
    value: '',
    innerHTML: '',
    textContent: '',
    dataset: {},
    classList: {
      add() {},
      remove() {},
      toggle() {},
      contains() { return false; }
    },
    style: {},
    children: [],
    querySelectorAll() { return []; },
    querySelector() { return null; },
    addEventListener() {},
    focus() {},
    blur() {},
    setAttribute() {},
    removeAttribute() {}
  };
}

const mockDoc = {
  getElementById(id) {
    if (!elements[id]) elements[id] = createMockElement(id);
    return elements[id];
  },
  createElement(tag) { return createMockElement('', tag); },
  querySelectorAll() { return []; },
  querySelector() { return null; },
  addEventListener() {}
};

const sandbox = {
  document: mockDoc,
  window: {
    addEventListener() {},
    location: { origin: 'http://localhost' },
    localStorage: { getItem() { return null; }, setItem() {} }
  },
  localStorage: { getItem() { return null; }, setItem() {} },
  setTimeout() {},
  setInterval() {},
  clearTimeout() {},
  clearInterval() {},
  console: console,
  fetch() {},
  navigator: { clipboard: { writeText() {} } },
  CodeMirror: {
    fromTextArea() {
      return {
        getValue() { return ''; },
        setValue() {},
        on() {},
        getWrapperElement() { return createMockElement(); },
        refresh() {}
      };
    }
  }
};

vm.createContext(sandbox);

try {
  vm.runInContext(mainScript, sandbox);
  console.log('Script loaded successfully into sandbox!');
} catch (e) {
  console.error('Script load error:', e.message);
  process.exit(1);
}

// Run setup inside the context
vm.runInContext(`
  activeConnection = { id: 'c1', name: 'test' };
  tabs = [{
    id: 'tab-table',
    title: 'users',
    tableName: 'users',
    isTableViewer: true,
    tableMode: 'data',
    filterPills: []
  }];
  activeTabId = 'tab-table';
  currentColumns = [{ name: 'name', type: 'VARCHAR' }, { name: 'age', type: 'INT' }];
  loadTableRows = (tbl) => {};

  const inputMock = {
    dataset: { col: 'name' },
    value: 'Alice'
  };
  const eventMock = {
    key: 'Enter',
    preventDefault() {}
  };

  handleColFilterKeydown(eventMock, inputMock);
`, sandbox);

const tabInfo = vm.runInContext(`({
  appliedFilter: tabs[0].appliedFilter,
  filterPills: tabs[0].filterPills
})`, sandbox);

console.log('Test 1 Tab appliedFilter:', JSON.stringify(tabInfo.appliedFilter));
console.log('Test 1 Filter pills:', JSON.stringify(tabInfo.filterPills));

// Assertion 1: appliedFilter must be compiled from the column filter input
assert.strictEqual(tabInfo.appliedFilter, "`name` = 'Alice'", 'Test 1 Failed: appliedFilter should be "`name` = \'Alice\'", but got ' + JSON.stringify(tabInfo.appliedFilter));

// TEST 2: Query result rendering has filter row (headerRow2)
vm.runInContext(`
  tabs.push({
    id: 'tab-query',
    title: 'Query 1.sql',
    sql: 'SELECT name, age FROM users',
    isTableViewer: false
  });
  activeTabId = 'tab-query';
  renderGridResults(
    [{ name: 'name', type: 'VARCHAR' }, { name: 'age', type: 'INT' }],
    [['Alice', 25], ['Bob', 30], ['Charlie', 35]]
  );
`, sandbox);

const theadHTML = vm.runInContext(`document.getElementById('grid-thead').innerHTML`, sandbox);
console.log('Test 2 Thead HTML includes col-filter-input?:', theadHTML.includes('col-filter-input'));
assert.ok(theadHTML.includes('col-filter-input'), 'Test 2 Failed: thead should render col-filter-input for query results');

// TEST 3: Query result filtering by column
vm.runInContext(`
  const queryInputMock = {
    dataset: { col: 'name' },
    value: 'Bob'
  };
  handleColFilterKeydown({ key: 'Enter', preventDefault() {} }, queryInputMock);
`, sandbox);

const tbodyHTML = vm.runInContext(`document.getElementById('grid-tbody').innerHTML`, sandbox);
console.log('Test 3 Query filtered tbody has Bob and not Alice or Charlie?:', tbodyHTML.includes('Bob') && !tbodyHTML.includes('Alice') && !tbodyHTML.includes('Charlie'));
assert.ok(tbodyHTML.includes('Bob') && !tbodyHTML.includes('Alice') && !tbodyHTML.includes('Charlie'), 'Test 3 Failed: tbody should filter query results to only Bob');

// TEST 4: Numeric comparison in query results (age > 28)
vm.runInContext(`
  // Clear name filter first
  handleColFilterKeydown({ key: 'Enter', preventDefault() {} }, { dataset: { col: 'name' }, value: '' });
  // Filter age > 28
  handleColFilterKeydown({ key: 'Enter', preventDefault() {} }, { dataset: { col: 'age' }, value: '> 28' });
`, sandbox);

const tbodyHTML4 = vm.runInContext(`document.getElementById('grid-tbody').innerHTML`, sandbox);
console.log('Test 4 (age > 28) has Bob (30) and Charlie (35) but not Alice (25)?:', !tbodyHTML4.includes('Alice') && tbodyHTML4.includes('Bob') && tbodyHTML4.includes('Charlie'));
assert.ok(!tbodyHTML4.includes('Alice') && tbodyHTML4.includes('Bob') && tbodyHTML4.includes('Charlie'), 'Test 4 Failed');

// TEST 5: Escape clears query filter
vm.runInContext(`
  const escInput = { dataset: { col: 'age' }, value: '> 28', blur() {} };
  handleColFilterKeydown({ key: 'Escape', preventDefault() {} }, escInput);
`, sandbox);

const tbodyHTML5 = vm.runInContext(`document.getElementById('grid-tbody').innerHTML`, sandbox);
console.log('Test 5 (Escape reset) shows all 3 rows?:', tbodyHTML5.includes('Alice') && tbodyHTML5.includes('Bob') && tbodyHTML5.includes('Charlie'));
assert.ok(tbodyHTML5.includes('Alice') && tbodyHTML5.includes('Bob') && tbodyHTML5.includes('Charlie'), 'Test 5 Failed');

// TEST 6: Table viewer operator prefixes (e.g. > 10, != Inactive)
vm.runInContext(`
  activeTabId = 'tab-table';
  handleColFilterKeydown({ key: 'Enter', preventDefault() {} }, { dataset: { col: 'age' }, value: '> 20' });
`, sandbox);

const tabInfo6 = vm.runInContext(`({
  appliedFilter: tabs[0].appliedFilter
})`, sandbox);
console.log('Test 6 Table viewer operator filter:', tabInfo6.appliedFilter);
assert.strictEqual(tabInfo6.appliedFilter, "`name` = 'Alice' AND `age` > 20", 'Test 6 Failed');

// TEST 7: clearTableFilter resets everything
vm.runInContext(`
  clearTableFilter();
`, sandbox);
const tabInfo7 = vm.runInContext(`({
  appliedFilter: tabs[0].appliedFilter,
  pillsLength: tabs[0].filterPills.length
})`, sandbox);
console.log('Test 7 clearTableFilter reset appliedFilter to empty?:', tabInfo7.appliedFilter === '' && tabInfo7.pillsLength === 0);
assert.strictEqual(tabInfo7.appliedFilter, '', 'Test 7 Failed');
assert.strictEqual(tabInfo7.pillsLength, 0, 'Test 7 Failed');

// TEST 8: LIKE filter smart auto-wrap (%effendi% when no wildcard provided)
vm.runInContext(`
  tabs[0].filterPills = [{ col: 'name', op: 'LIKE', val: 'effendi' }];
  applyTableFilter();
`, sandbox);
const tabInfo8 = vm.runInContext(`tabs[0].appliedFilter`, sandbox);
console.log('Test 8 LIKE smart auto-wrap filter:', tabInfo8);
assert.strictEqual(tabInfo8, "`name` LIKE '%effendi%'", 'Test 8 Failed');

// TEST 9: LIKE filter preserves explicit wildcards (e.g. 'effendi%')
vm.runInContext(`
  tabs[0].filterPills = [{ col: 'name', op: 'LIKE', val: 'effendi%' }];
  applyTableFilter();
`, sandbox);
const tabInfo9 = vm.runInContext(`tabs[0].appliedFilter`, sandbox);
console.log('Test 9 LIKE explicit wildcard filter:', tabInfo9);
assert.strictEqual(tabInfo9, "`name` LIKE 'effendi%'", 'Test 9 Failed');

// TEST 10: In-memory predicate matchesFilterCondition with LIKE auto-wrap
const matchRes = vm.runInContext(`matchesFilterCondition('Ahmad Effendi Siregar', 'LIKE effendi')`, sandbox);
const nonMatchRes = vm.runInContext(`matchesFilterCondition('Budi Santoso', 'LIKE effendi')`, sandbox);
console.log('Test 10 matchesFilterCondition LIKE auto-wrap:', matchRes, nonMatchRes);
assert.strictEqual(matchRes, true, 'Test 10 Failed: should match substring');
assert.strictEqual(nonMatchRes, false, 'Test 10 Failed: should not match');

console.log('ALL COMPREHENSIVE TESTS PASSED!');



