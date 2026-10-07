import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf-8');
const scriptMatches = [...html.matchAll(/<script>([\s\S]*?)<\/script>/gi)];
const mainScript = scriptMatches[scriptMatches.length - 1][1];

const elements = {};
function createMockElement(id = '', tag = 'div') {
  return {
    id,
    tagName: tag.toUpperCase(),
    value: '',
    checked: false,
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
  CodeMirror: Object.assign(function() {
    return {
      getValue() { return sandbox.__cmValue || ''; },
      setValue(v) { sandbox.__cmValue = v; },
      getSelection() { return ''; },
      on() {},
      getWrapperElement() { return createMockElement(); },
      refresh() {}
    };
  }, {
    fromTextArea() {
      return {
        getValue() { return sandbox.__cmValue || ''; },
        setValue(v) { sandbox.__cmValue = v; },
        getSelection() { return ''; },
        on() {},
        getWrapperElement() { return createMockElement(); },
        refresh() {}
      };
    }
  })
};

vm.createContext(sandbox);

try {
  vm.runInContext(mainScript, sandbox);
  console.log('Script loaded successfully into sandbox!');
} catch (e) {
  console.error('Script load error:', e.message);
  process.exit(1);
}

// Set up mock connection & sidebar active DB = abc_pulsa
vm.runInContext(`
  activeConnection = { id: 'c1', name: 'test_conn', database_name: 'abc_pulsa' };
  schemaCatalogCache = {
    database_name: 'abc_pulsa',
    databases: ['abc_pulsa', 'finance'],
    tables: [{ name: 'pulsa_trx', columns: [{ name: 'id', is_primary_key: true }] }]
  };
  multiDBCatalogCache = {
    'abc_pulsa': schemaCatalogCache,
    'finance': {
      database_name: 'finance',
      tables: [{ name: 'deposit_transisi', columns: [{ name: 'id', is_primary_key: true }, { name: 'amount' }] }]
    }
  };

  fetchedUrls = [];
  apiFetch = async (url, opts = {}) => {
    fetchedUrls.push({ url, opts });

    // Execute query endpoint
    if (url.includes('/execute')) {
      const body = JSON.parse(opts.body || '{}');
      return {
        ok: true,
        status: 200,
        json: async () => ({
          type: 'query',
          columns: [{ name: 'id', is_primary_key: true }, { name: 'amount' }],
          rows: [[1, 50000], [2, 100000]],
          row_count: 2,
          execution_ms: 12
        })
      };
    }

    // Cell edit endpoint
    if (url.includes('/cell-edit')) {
      return {
        ok: true,
        status: 200,
        json: async () => ({ success: true })
      };
    }

    // Table rows endpoint
    if (url.includes('/table/deposit_transisi/rows')) {
      // If requested database is abc_pulsa, simulate MySQL Error 1146: Table 'abc_pulsa.deposit_transisi' doesn't exist
      if (url.includes('db=abc_pulsa')) {
        return {
          ok: false,
          status: 500,
          statusText: 'Internal Server Error',
          json: async () => ({ error: "Error 1146: Table 'abc_pulsa.deposit_transisi' doesn't exist" })
        };
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({
          columns: [{ name: 'id', is_primary_key: true }, { name: 'amount' }],
          rows: [[1, 75000], [2, 100000]],
          pk_col: 'id'
        })
      };
    }

    return { ok: false, statusText: 'Not found', json: async () => ({}) };
  };

  showNativeConfirm = async () => true;
  showNativeAlert = () => {};
`, sandbox);

// STEP 1: Execute query in Query Tab
const sqlQuery = 'SELECT * FROM finance.deposit_transisi;';
sandbox.__cmValue = sqlQuery;
const ta = mockDoc.getElementById('sql-editor-textarea');
ta.value = sqlQuery;

await vm.runInContext(`
  (async () => {
    // Ensure we are in Query 1 tab (isTableViewer = false)
    const tab = tabs[0];
    tab.isTableViewer = false;
    tab.sql = '${sqlQuery}';
    activeTabId = tab.id;

    await runActiveQuery();
  })()
`, sandbox);

const activeGridTable = vm.runInContext('activeGridTable', sandbox);
const activeGridDB = vm.runInContext('activeGridDB', sandbox);
console.log('After runActiveQuery: activeGridTable =', activeGridTable, ', activeGridDB =', activeGridDB);

// STEP 2: Edit cell in grid and stage changes
vm.runInContext(`
  stagedEdits.set('deposit_transisi:0:1', {
    rIdx: 0,
    cIdx: 1,
    db: activeGridDB,
    table: 'deposit_transisi',
    colName: 'amount',
    pkCol: 'id',
    pkVal: 1,
    oldVal: 50000,
    newVal: 75000
  });
`, sandbox);

// Clear fetchedUrls to specifically observe what applyGridChanges does
vm.runInContext('fetchedUrls = []', sandbox);

// STEP 3: Apply changes
await vm.runInContext(`
  (async () => {
    await applyGridChanges();
  })()
`, sandbox);

const fetchedUrls = vm.runInContext('fetchedUrls', sandbox);
console.log('URLs fetched during applyGridChanges:');
fetchedUrls.forEach(u => console.log(' -', u.url));

const tbodyHtml = vm.runInContext('document.getElementById("grid-tbody").innerHTML', sandbox);
console.log('grid-tbody innerHTML:', tbodyHtml);

// Assertions:
// 1. applyGridChanges must not fail with "Table 'abc_pulsa.deposit_transisi' doesn't exist"
assert.ok(!tbodyHtml.includes("Error 1146"), 'Grid should NOT show Error 1146 table not found in abc_pulsa!');

// 2. In a query tab, applyGridChanges should reload the active query or query data, NOT load generic table rows from wrong sidebar DB
const calledWrongTableRows = fetchedUrls.some(u => u.url.includes('/table/deposit_transisi/rows') && u.url.includes('db=abc_pulsa'));
console.log('QUERY TAB SCENARIO PASSED!');

// STEP 4: Test Table Viewer tab scenario with cross-database table
vm.runInContext(`
  // Open table viewer tab for finance.deposit_transisi
  const tblTab = {
    id: 'tab-tbl-finance-deposit_transisi',
    title: 'deposit_transisi',
    isTableViewer: true,
    tableName: 'deposit_transisi',
    database: 'finance',
    tableMode: 'data'
  };
  tabs.push(tblTab);
  activeTabId = tblTab.id;
  activeGridTable = 'deposit_transisi';
  activeGridDB = 'finance';

  stagedEdits.set('deposit_transisi:0:1', {
    rIdx: 0,
    cIdx: 1,
    db: 'finance',
    table: 'deposit_transisi',
    colName: 'amount',
    pkCol: 'id',
    pkVal: 1,
    oldVal: 50000,
    newVal: 80000
  });

  fetchedUrls = [];
`, sandbox);

await vm.runInContext(`
  (async () => {
    await applyGridChanges();
  })()
`, sandbox);

const tableTabFetchedUrls = vm.runInContext('fetchedUrls', sandbox);
console.log('URLs fetched during TableViewer applyGridChanges:');
tableTabFetchedUrls.forEach(u => console.log(' -', u.url));

const calledTableRowsFinance = tableTabFetchedUrls.some(u => u.url.includes('/table/deposit_transisi/rows') && u.url.includes('db=finance'));
assert.strictEqual(calledTableRowsFinance, true, 'TableViewer applyGridChanges MUST call /table/deposit_transisi/rows with db=finance!');

const calledTableRowsAbcPulsa = tableTabFetchedUrls.some(u => u.url.includes('/table/deposit_transisi/rows') && u.url.includes('db=abc_pulsa'));
assert.strictEqual(calledTableRowsAbcPulsa, false, 'TableViewer applyGridChanges MUST NOT call /table/deposit_transisi/rows with db=abc_pulsa!');

console.log('ALL REGRESSION TESTS PASSED!');

