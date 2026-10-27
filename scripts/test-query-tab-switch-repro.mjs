import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf8');

const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
if (lastScriptStart === -1 || lastScriptEnd === -1) {
  console.error("Could not find main <script> tag in web/dist/index.html");
  process.exit(2);
}
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

const storage = new Map();
const localStorageMock = {
  getItem: (k) => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: (k) => storage.delete(k),
  clear: () => storage.clear()
};

const domElementStub = (id = '') => ({
  id,
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
  width: 800,
  height: 200,
  disabled: false,
  addEventListener: () => {},
  removeEventListener: () => {},
  focus: () => {},
  select: () => {},
  blur: () => {},
  contains: () => false,
  setAttribute: () => {},
  removeAttribute: () => {},
  querySelector: (sel) => getEl(sel),
  querySelectorAll: () => [],
  appendChild: () => {},
  cloneNode: function() { return domElementStub(this.id); }
});

const elements = new Map();
function getEl(id) {
  if (!elements.has(id)) {
    elements.set(id, domElementStub(id));
  }
  return elements.get(id);
}

let fetchHandler = null;

const sandbox = {
  console,
  devicePixelRatio: 1,
  fetch: async (url, opts) => {
    if (fetchHandler) {
      return fetchHandler(url, opts);
    }
    return { ok: true, status: 200, headers: { get: () => null }, json: async () => ({}) };
  },
  setTimeout: (fn) => setTimeout(fn, 0),
  clearTimeout: () => {},
  setInterval: () => {},
  clearInterval: () => {},
  localStorage: localStorageMock,
  sessionStorage: localStorageMock,
  TextDecoder,
  window: {
    devicePixelRatio: 1,
    innerWidth: 1920,
    innerHeight: 1080,
    addEventListener: () => {},
    removeEventListener: () => {},
    location: { reload: () => {} },
    alert: () => {},
    confirm: () => true,
    prompt: () => ''
  },
  document: {
    body: domElementStub('body'),
    getElementById: (id) => getEl(id),
    querySelectorAll: () => [],
    querySelector: (sel) => getEl(sel),
    createElement: (tag) => domElementStub(tag),
    addEventListener: () => {},
    hidden: false
  },
  navigator: { clipboard: { writeText: async () => {} } },
  Event: class {},
  Blob: class {},
  URL: { createObjectURL: () => '', revokeObjectURL: () => {} },
  CodeMirror: {
    fromTextArea: () => ({
      getValue: () => '',
      setValue: () => {},
      on: () => {},
      refresh: () => {},
      setSize: () => {}
    })
  }
};

vm.createContext(sandbox);

try {
  vm.runInContext(scriptContent, sandbox);
  vm.runInContext(`showNativeConfirm = async () => true;`, sandbox);
} catch (err) {
  console.error("Script execution error:", err.message);
  process.exit(2);
}

async function runTest() {
  console.log("=== Scenario 1: Heavy SELECT query finishes while on Tab 2 ===");
  let resolveScenario1 = null;
  fetchHandler = (url, opts) => {
    if (url.includes('/execute-stream')) {
      return new Promise((resolve) => {
        resolveScenario1 = () => {
          resolve({
            ok: true,
            status: 200,
            headers: { get: (h) => (h.toLowerCase() === 'content-type' ? 'application/json' : null) },
            json: async () => ({
              type: 'query',
              columns: ['id', 'name'],
              rows: [[1, 'Alice'], [2, 'Bob']],
              row_count: 2,
              execution_ms: 1500
            })
          });
        };
      });
    }
    return { ok: true, status: 200, headers: { get: () => null }, json: async () => ({}) };
  };

  vm.runInContext(`
    activeConnection = { id: 'conn-1', name: 'Dev DB', driver: 'sqlite' };
    tabs = [
      { id: 'tab-1', title: 'Heavy Query.sql', sql: 'SELECT * FROM big_table;', isTableViewer: false, rawQueryRows: null, rawQueryCols: null },
      { id: 'tab-2', title: 'Other Query.sql', sql: 'SELECT 1;', isTableViewer: false, rawQueryRows: null, rawQueryCols: null }
    ];
    activeTabId = 'tab-1';
    setEditorSQL('SELECT * FROM big_table;');
  `, sandbox);

  console.log("1. Starting query on tab-1...");
  const queryPromise1 = vm.runInContext(`runActiveQuery()`, sandbox);

  const tab1Executing = vm.runInContext(`tabs.find(t => t.id === 'tab-1').isExecuting`, sandbox);
  assert.strictEqual(tab1Executing, true, "tab-1 must be marked isExecuting while in-flight");

  console.log("2. Switching to tab-2 while query is in-flight...");
  vm.runInContext(`switchTab('tab-2')`, sandbox);

  console.log("3. Server finishes heavy query execution...");
  resolveScenario1();
  await queryPromise1;

  const tab1 = vm.runInContext(`tabs.find(t => t.id === 'tab-1')`, sandbox);
  const tab2 = vm.runInContext(`tabs.find(t => t.id === 'tab-2')`, sandbox);

  assert.notStrictEqual(tab1.rawQueryRows, null, "Tab 1 must get its query results");
  assert.strictEqual(tab1.rawQueryRows.length, 2, "Tab 1 must have 2 rows");
  assert.strictEqual(tab1.isExecuting, false, "Tab 1 isExecuting must be reset to false");
  assert.strictEqual(tab2.rawQueryRows, null, "Tab 2 must not be polluted with Tab 1's query results");
  console.log("Scenario 1 PASSED!");

  console.log("\n=== Scenario 2: Error query on Tab 1 while on Tab 2 ===");
  let resolveScenario2 = null;
  fetchHandler = (url, opts) => {
    if (url.includes('/execute-stream')) {
      return new Promise((resolve) => {
        resolveScenario2 = () => {
          resolve({
            ok: false,
            status: 400,
            headers: { get: (h) => (h.toLowerCase() === 'content-type' ? 'application/json' : null) },
            json: async () => ({
              error: 'Table big_table not found'
            })
          });
        };
      });
    }
    return { ok: true, status: 200, headers: { get: () => null }, json: async () => ({}) };
  };

  vm.runInContext(`
    tabs = [
      { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT * FROM wrong_table;', isTableViewer: false, rawQueryRows: null, rawQueryCols: null },
      { id: 'tab-2', title: 'Query 2.sql', sql: 'SELECT 1;', isTableViewer: false, rawQueryRows: null, rawQueryCols: null }
    ];
    activeTabId = 'tab-1';
    setEditorSQL('SELECT * FROM wrong_table;');
  `, sandbox);

  const queryPromise2 = vm.runInContext(`runActiveQuery()`, sandbox);
  vm.runInContext(`switchTab('tab-2')`, sandbox);
  resolveScenario2();
  await queryPromise2;

  const tab1Err = vm.runInContext(`tabs.find(t => t.id === 'tab-1')`, sandbox);
  const tab2Err = vm.runInContext(`tabs.find(t => t.id === 'tab-2')`, sandbox);

  assert.notStrictEqual(tab1Err.rawQueryError, null, "Tab 1 must receive rawQueryError");
  assert.strictEqual(tab1Err.rawQueryError.message, 'Table big_table not found');
  assert.strictEqual(tab1Err.isExecuting, false, "Tab 1 isExecuting must be false");
  assert.ok(!tab2Err.rawQueryError, "Tab 2 must not receive Tab 1's error");
  console.log("Scenario 2 PASSED!");

  console.log("\n=== Scenario 3: Non-select EXEC on Tab 1 while on Tab 2 ===");
  let resolveScenario3 = null;
  fetchHandler = (url, opts) => {
    if (url.includes('/execute-stream')) {
      return new Promise((resolve) => {
        resolveScenario3 = () => {
          resolve({
            ok: true,
            status: 200,
            headers: { get: (h) => (h.toLowerCase() === 'content-type' ? 'application/json' : null) },
            json: async () => ({
              type: 'exec',
              message: '1 row affected',
              rows_affected: 1,
              execution_ms: 25
            })
          });
        };
      });
    }
    return { ok: true, status: 200, headers: { get: () => null }, json: async () => ({}) };
  };

  vm.runInContext(`
    tabs = [
      { id: 'tab-1', title: 'Query 1.sql', sql: 'UPDATE users SET name = "Bob" WHERE id = 1;', isTableViewer: false },
      { id: 'tab-2', title: 'Query 2.sql', sql: 'SELECT 1;', isTableViewer: false }
    ];
    activeTabId = 'tab-1';
    setEditorSQL('UPDATE users SET name = "Bob" WHERE id = 1;');
  `, sandbox);

  const queryPromise3 = vm.runInContext(`runActiveQuery()`, sandbox);
  vm.runInContext(`switchTab('tab-2')`, sandbox);
  resolveScenario3();
  await queryPromise3;

  const tab1Exec = vm.runInContext(`tabs.find(t => t.id === 'tab-1')`, sandbox);
  const tab2Exec = vm.runInContext(`tabs.find(t => t.id === 'tab-2')`, sandbox);

  assert.notStrictEqual(tab1Exec.rawQueryExec, null, "Tab 1 must receive rawQueryExec");
  assert.strictEqual(tab1Exec.rawQueryExec.message, '1 row affected');
  assert.strictEqual(tab1Exec.isExecuting, false, "Tab 1 isExecuting must be false");
  assert.ok(!tab2Exec.rawQueryExec, "Tab 2 must not receive Tab 1's rawQueryExec");
  console.log("Scenario 3 PASSED!");

  console.log("\n=== Scenario 4: Switching back to Tab 1 restores results ===");
  vm.runInContext(`switchTab('tab-1')`, sandbox);
  const curCols = vm.runInContext(`currentColumns`, sandbox);
  const curRows = vm.runInContext(`currentResults`, sandbox);
  console.log("Active Tab is now tab-1, currentResults:", curRows);
  console.log("Scenario 4 PASSED!");

  console.log("\n=== Scenario 5: Closing tab while executing triggers query cancellation ===");
  let cancelCalled = false;
  let resolveScenario5 = null;
  fetchHandler = (url, opts) => {
    if (url.includes('/cancel')) {
      cancelCalled = true;
      return { ok: true, json: async () => ({}) };
    }
    if (url.includes('/execute-stream')) {
      return new Promise((resolve) => {
        resolveScenario5 = () => {
          resolve({
            ok: true,
            status: 200,
            headers: { get: () => 'application/json' },
            json: async () => ({ type: 'query', columns: ['id'], rows: [[1]] })
          });
        };
      });
    }
    return { ok: true, json: async () => ({}) };
  };

  vm.runInContext(`
    tabs = [
      { id: 'tab-run', title: 'Running.sql', sql: 'SELECT SLEEP(10);', isTableViewer: false },
      { id: 'tab-other', title: 'Other.sql', sql: 'SELECT 1;', isTableViewer: false }
    ];
    activeTabId = 'tab-run';
    setEditorSQL('SELECT SLEEP(10);');
  `, sandbox);

  const queryPromise5 = vm.runInContext(`runActiveQuery()`, sandbox);
  assert.strictEqual(vm.runInContext(`isQueryExecuting`, sandbox), true, "isQueryExecuting must be true");

  // User closes the executing tab
  await vm.runInContext(`closeTab('tab-run')`, sandbox);
  assert.strictEqual(cancelCalled, true, "Closing executing tab must call cancelRunningQuery()");

  // Complete backend response
  resolveScenario5();
  await queryPromise5;

  assert.strictEqual(vm.runInContext(`isQueryExecuting`, sandbox), false, "isQueryExecuting must be false after closed tab query ends");
  assert.strictEqual(vm.runInContext(`tabs.some(t => t.id === 'tab-run')`, sandbox), false, "tab-run must be gone");
  console.log("Scenario 5 PASSED!");

  console.log("\n=== Scenario 6: Re-running query clears stale results during in-flight period ===");
  let resolveScenario6 = null;
  fetchHandler = (url, opts) => {
    if (url.includes('/execute-stream')) {
      return new Promise((resolve) => {
        resolveScenario6 = () => {
          resolve({
            ok: true,
            status: 200,
            headers: { get: () => 'application/json' },
            json: async () => ({ type: 'query', columns: ['id'], rows: [[99]], row_count: 1, execution_ms: 50 })
          });
        };
      });
    }
    return { ok: true, json: async () => ({}) };
  };

  vm.runInContext(`
    tabs = [
      { id: 'tab-1', title: 'Query.sql', sql: 'SELECT 1;', isTableViewer: false, rawQueryRows: [[1], [2]], rawQueryCols: ['id'] }
    ];
    activeTabId = 'tab-1';
    setEditorSQL('SELECT 99;');
  `, sandbox);

  const queryPromise6 = vm.runInContext(`runActiveQuery()`, sandbox);
  const tabWhileRunning = vm.runInContext(`tabs.find(t => t.id === 'tab-1')`, sandbox);
  assert.strictEqual(tabWhileRunning.rawQueryRows, null, "Stale rawQueryRows must be cleared immediately when new query begins");

  resolveScenario6();
  await queryPromise6;
  const tabAfter = vm.runInContext(`tabs.find(t => t.id === 'tab-1')`, sandbox);
  assert.strictEqual(tabAfter.rawQueryRows.length, 1, "New query rows must be saved");
  console.log("Scenario 6 PASSED!");

  console.log("\n=== Scenario 7: clearGridToInitialState deactivates virtual grid ===");
  vm.runInContext(`
    isVirtualGridActive = true;
    virtualGridRows = [[1, 'Alice'], [2, 'Bob']];
    clearGridToInitialState();
  `, sandbox);
  assert.strictEqual(vm.runInContext(`isVirtualGridActive`, sandbox), false, "clearGridToInitialState must set isVirtualGridActive to false");
  assert.strictEqual(vm.runInContext(`virtualGridRows.length`, sandbox), 0, "clearGridToInitialState must clear virtualGridRows");
  console.log("Scenario 7 PASSED!");

  console.log("\n=== Scenario 8: NDJSON stream error line sets rawQueryError ===");
  let resolveScenario8 = null;
  fetchHandler = (url, opts) => {
    if (url.includes('/execute-stream')) {
      return new Promise((resolve) => {
        resolveScenario8 = () => {
          const streamChunks = [
            '{"type":"meta","columns":[{"name":"id","type":"int"}]}\n',
            '{"type":"chunk","rows":[[1],[2]]}\n',
            '{"type":"error","error":"connection reset mid-query"}\n'
          ];
          let chunkIdx = 0;
          resolve({
            ok: true,
            status: 200,
            headers: { get: () => 'application/x-ndjson' },
            body: {
              getReader: () => ({
                read: async () => {
                  if (chunkIdx < streamChunks.length) {
                    const enc = new TextEncoder();
                    const val = enc.encode(streamChunks[chunkIdx++]);
                    return { done: false, value: val };
                  }
                  return { done: true };
                },
                cancel: async () => {}
              })
            }
          });
        };
      });
    }
    return { ok: true, json: async () => ({}) };
  };

  vm.runInContext(`
    tabs = [
      { id: 'tab-stream-err', title: 'Err.sql', sql: 'SELECT * FROM crashing;', isTableViewer: false }
    ];
    activeTabId = 'tab-stream-err';
    setEditorSQL('SELECT * FROM crashing;');
  `, sandbox);

  const queryPromise8 = vm.runInContext(`runActiveQuery()`, sandbox);
  resolveScenario8();
  await queryPromise8;

  const tabWithErr = vm.runInContext(`tabs.find(t => t.id === 'tab-stream-err')`, sandbox);
  assert.notStrictEqual(tabWithErr.rawQueryError, null, "tab must receive rawQueryError on stream error");
  assert.strictEqual(tabWithErr.rawQueryError.message, "connection reset mid-query", "error message must match");
  assert.strictEqual(tabWithErr.isExecuting, false, "tab.isExecuting must be reset");
  console.log("Scenario 8 PASSED!");

  console.log("\n=== Scenario 9: schemaTreeFastIndex invalidates on refreshed schema object ===");
  const schemaV1 = { database_name: 'testdb', tables: [{ name: 't1', type: 'TABLE' }], triggers: [] };
  const schemaV2 = { database_name: 'testdb', tables: [{ name: 't1', type: 'TABLE' }, { name: 't2_new', type: 'TABLE' }], triggers: [] };

  sandbox.schemaV1 = schemaV1;
  sandbox.schemaV2 = schemaV2;

  vm.runInContext(`
    renderSchemaTree(schemaV1);
    fastIdx1 = schemaTreeFastIndex;
    renderSchemaTree(schemaV2);
    fastIdx2 = schemaTreeFastIndex;
  `, sandbox);

  const tCount = vm.runInContext(`fastIdx2.tables.length`, sandbox);
  assert.strictEqual(tCount, 2, "Refreshed schema must rebuild fast index and contain 2 tables");
  console.log("Scenario 9 PASSED!");

  console.log("\n=== Scenario 10: schemaTreeFastIndex preserves table metadata (row_count) ===");
  const schemaWithMeta = { database_name: 'testdb', tables: [{ name: 'users', type: 'TABLE', row_count: 42000 }], triggers: [] };
  sandbox.schemaWithMeta = schemaWithMeta;
  vm.runInContext(`
    renderSchemaTree(schemaWithMeta);
  `, sandbox);
  const fastIdxMeta = vm.runInContext(`schemaTreeFastIndex.tables[0].row_count`, sandbox);
  assert.strictEqual(fastIdxMeta, 42000, "fast index must preserve row_count attribute on table object");
  console.log("Scenario 10 PASSED!");

  console.log("\n=== Scenario 11: hasExplicitSQLLimit handles trailing comments ===");
  const hasLimitWithComment = vm.runInContext(`hasExplicitSQLLimit("SELECT * FROM users LIMIT 10 -- trailing note")`, sandbox);
  assert.strictEqual(hasLimitWithComment, true, "hasExplicitSQLLimit must detect LIMIT even with trailing SQL comments");
  console.log("Scenario 11 PASSED!");

  console.log("\n=== Scenario 12: wrapSQLWithPagination handles CTE with top-level WITH ===");
  const cteSQL = "WITH regional_sales AS (SELECT * FROM orders) SELECT * FROM regional_sales";
  const wrapped = vm.runInContext(`wrapSQLWithPagination("${cteSQL}", 5000, 5000)`, sandbox);
  assert.ok(wrapped.startsWith("WITH regional_sales AS"), "Wrapped CTE query must preserve root WITH clause");
  assert.ok(wrapped.includes("LIMIT 5000 OFFSET 5000"), "Wrapped query must append LIMIT and OFFSET");
  console.log("Scenario 12 PASSED!");

  console.log("\n=== Scenario 13: navigateGridCellByArrow moves activeGridSelectedRow and Col ===");
  vm.runInContext(`
    currentColumns = [{ name: 'id', type: 'int' }, { name: 'name', type: 'text' }];
    currentResults = [[1, 'Alice'], [2, 'Bob'], [3, 'Charlie']];
    activeGridSelectedRow = 0;
    activeGridSelectedCol = 0;
    activeGridSelectedTd = { classList: { add() {}, remove() {} } };
    navigateGridCellByArrow('ArrowDown');
  `, sandbox);
  assert.strictEqual(vm.runInContext(`activeGridSelectedRow`, sandbox), 1, "ArrowDown must increment selected row");
  vm.runInContext(`navigateGridCellByArrow('ArrowRight');`, sandbox);
  assert.strictEqual(vm.runInContext(`activeGridSelectedCol`, sandbox), 1, "ArrowRight must increment selected column");
  console.log("Scenario 13 PASSED!");

  console.log("\n=== Scenario 14: renderSchemaTree(null) gracefully handles null/undefined ===");
  let nullRenderThrown = false;
  try {
    vm.runInContext(`renderSchemaTree(null);`, sandbox);
  } catch (err) {
    nullRenderThrown = true;
  }
  assert.strictEqual(nullRenderThrown, false, "renderSchemaTree(null) must not throw TypeError");
  console.log("Scenario 14 PASSED!");

  console.log("\n=== Scenario 15: wrapSQLWithPagination handles trailing line comments safely ===");
  const commentedSQL = "SELECT * FROM users -- all users";
  const wrappedWithComment = vm.runInContext(`wrapSQLWithPagination("${commentedSQL}", 5000, 5000)`, sandbox);
  assert.ok(wrappedWithComment.includes("\n) AS _p LIMIT"), "Closing subquery bracket must be preceded by a newline to prevent line comment truncation");
  console.log("Scenario 15 PASSED!");

  console.log("\n=== Scenario 16: clearGridToInitialState resets active cell selection and prevents detached DOM leak ===");
  vm.runInContext(`
    activeGridSelectedRow = 5;
    activeGridSelectedCol = 2;
    activeGridSelectedTd = { classList: { add() {}, remove() {} } };
    clearGridToInitialState();
  `, sandbox);
  assert.strictEqual(vm.runInContext(`activeGridSelectedRow`, sandbox), null, "activeGridSelectedRow must be reset to null");
  assert.strictEqual(vm.runInContext(`activeGridSelectedCol`, sandbox), null, "activeGridSelectedCol must be reset to null");
  assert.strictEqual(vm.runInContext(`activeGridSelectedTd`, sandbox), null, "activeGridSelectedTd must be reset to null");
  console.log("Scenario 16 PASSED!");

  console.log("\nALL 16 REGRESSION TESTS PASSED!");
}

runTest().catch((err) => {
  console.error("FAILURE:", err.message);
  process.exit(1);
});
