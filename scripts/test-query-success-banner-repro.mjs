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

const domMap = new Map();
function getOrCreateElement(id) {
  if (!domMap.has(id)) {
    domMap.set(id, {
      id,
      innerHTML: '',
      textContent: '',
      value: '',
      classList: {
        classes: new Set(),
        add(c) { this.classes.add(c); },
        remove(c) { this.classes.delete(c); },
        contains(c) { return this.classes.has(c); },
        toggle(c) { if (this.classes.has(c)) this.classes.delete(c); else this.classes.add(c); }
      },
      style: {},
      setAttribute: () => {},
      removeAttribute: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      focus: () => {},
      blur: () => {}
    });
  }
  return domMap.get(id);
}

let fetchMockResponse = { ok: false, status: 400, json: async () => ({ error: 'Table users does not exist' }) };

const sandbox = {
  console,
  setTimeout: (fn) => {},
  clearTimeout: () => {},
  setInterval: () => {},
  clearInterval: () => {},
  localStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  sessionStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  window: { addEventListener: () => {}, removeEventListener: () => {}, location: { reload: () => {} } },
  document: { getElementById: getOrCreateElement, querySelectorAll: () => [], querySelector: getOrCreateElement, addEventListener: () => {} },
  Event: class {},
  fetch: async () => fetchMockResponse
};

vm.createContext(sandbox);
vm.runInContext(scriptContent, sandbox);

vm.runInContext(`
  activeConnection = { id: 'conn-1', name: 'Test DB', driver: 'mysql' };
  tabs = [
    { id: 'tab-1', title: 'Query 1.sql', sql: '', isTableViewer: false },
    { id: 'tab-2', title: 'Query 2.sql', sql: '', isTableViewer: false }
  ];
  activeTabId = 'tab-1';
`, sandbox);

// 1. Run failing query
getOrCreateElement('sql-editor-textarea').value = 'ALTER TABLE users ADD COLUMN age INT';
await sandbox.runActiveQuery(true);

const tbody = getOrCreateElement('grid-tbody');
const thead = getOrCreateElement('grid-thead');
assert(thead.innerHTML.includes('Execution Error'), 'thead should show Execution Error');
assert(tbody.innerHTML.includes('Database Execution Error'), 'tbody should show Database Execution Error');

// 2. Run successful ALTER query (type: "exec")
fetchMockResponse = {
  ok: true,
  status: 200,
  json: async () => ({
    type: 'exec',
    rows_affected: 0,
    execution_ms: 25,
    message: 'Query executed successfully. 0 rows affected'
  })
};
getOrCreateElement('sql-editor-textarea').value = 'ALTER TABLE existing_users ADD COLUMN age INT';
await sandbox.runActiveQuery(true);

// Error banner must be gone!
assert(!tbody.innerHTML.includes('Database Execution Error'), 'ERROR BANNER MUST NOT PERSIST AFTER SUCCESSFUL QUERY!');
assert(!thead.innerHTML.includes('Execution Error'), 'thead must not show Execution Error after success!');

// Success banner must be displayed
assert(thead.innerHTML.includes('Execution Succeeded'), 'thead should display execution success');
assert(tbody.innerHTML.includes('Command Executed Successfully') || tbody.innerHTML.includes('Query executed successfully'), 'tbody should display success banner');

// 3. Tab switching should preserve and re-render the success banner
sandbox.switchTab('tab-2');
sandbox.switchTab('tab-1');

assert(!tbody.innerHTML.includes('Database Execution Error'), 'ERROR BANNER MUST NOT RE-APPEAR ON TAB SWITCH!');
assert(thead.innerHTML.includes('Execution Succeeded'), 'thead should retain execution success on tab restore');
assert(tbody.innerHTML.includes('Command Executed Successfully') || tbody.innerHTML.includes('Query executed successfully'), 'tbody should retain success banner on tab restore');

// 4. Run a SELECT query -> success banner replaced by data grid
fetchMockResponse = {
  ok: true,
  status: 200,
  json: async () => ({
    type: 'query',
    columns: [{ name: 'id', type: 'int' }, { name: 'name', type: 'varchar' }],
    rows: [[1, 'Alice'], [2, 'Bob']],
    row_count: 2,
    execution_ms: 10
  })
};
getOrCreateElement('sql-editor-textarea').value = 'SELECT * FROM existing_users';
await sandbox.runActiveQuery(true);

assert(!tbody.innerHTML.includes('Command Executed Successfully'), 'Success banner should be replaced by table rows');
assert(!thead.innerHTML.includes('Execution Succeeded'), 'Success header should be replaced by column headers');
assert(thead.innerHTML.includes('id') && thead.innerHTML.includes('name'), 'Headers should include columns');

// 5. Run a failing UPDATE query -> table grid replaced by error banner
fetchMockResponse = {
  ok: false,
  status: 400,
  json: async () => ({ error: 'Unknown column invalid_col' })
};
getOrCreateElement('sql-editor-textarea').value = 'UPDATE existing_users SET invalid_col = 1';
await sandbox.runActiveQuery(true);

assert(thead.innerHTML.includes('Execution Error'), 'thead should show error');
assert(tbody.innerHTML.includes('Database Execution Error'), 'tbody should show error banner');

// 6. Run a succeeding UPDATE query -> error banner replaced by success banner
fetchMockResponse = {
  ok: true,
  status: 200,
  json: async () => ({
    type: 'exec',
    rows_affected: 2,
    execution_ms: 15,
    message: '2 rows updated'
  })
};
getOrCreateElement('sql-editor-textarea').value = 'UPDATE existing_users SET age = 25';
await sandbox.runActiveQuery(true);

assert(!tbody.innerHTML.includes('Database Execution Error'), 'Error banner must be cleared');
assert(thead.innerHTML.includes('Execution Succeeded'), 'thead should show execution succeeded');
assert(tbody.innerHTML.includes('2 rows updated'), 'tbody should show updated rows count');

console.log('✅ ALL TESTS PASSED: State transitions (Error -> Success -> Query Grid -> Error -> Success) verified completely.');
