import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

console.log("--- Starting Query Grid Mutations & Cross-Database SQL Copy Tests ---");

const html = fs.readFileSync(fs.existsSync('web/dist/index.legacy.html') ? 'web/dist/index.legacy.html' : 'web/dist/index.html', 'utf8');

// Extract Javascript from the main application <script> tag
const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
assert(lastScriptStart !== -1 && lastScriptEnd !== -1, "Could not find main <script> tag in web/dist/index.html");
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

class ClassList {
  constructor(initial = []) {
    this.classes = new Set(initial);
  }
  add(...names) { names.forEach(n => this.classes.add(n)); }
  remove(...names) { names.forEach(n => this.classes.delete(n)); }
  contains(name) { return this.classes.has(name); }
  toggle(name, force) {
    if (force !== undefined) {
      if (force) this.classes.add(name);
      else this.classes.delete(name);
      return force;
    }
    if (this.classes.has(name)) {
      this.classes.delete(name);
      return false;
    }
    this.classes.add(name);
    return true;
  }
}

class DOMElement {
  constructor(id, initialClasses = []) {
    this.id = id;
    this.classList = new ClassList(initialClasses);
    this.style = {};
    this.dataset = {};
    this.innerHTML = '';
    this.textContent = '';
    this.value = '';
    this.disabled = false;
    this.children = [];
  }
  addEventListener() {}
  appendChild() {}
  closest() { return null; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  setAttribute() {}
  removeAttribute() {}
}

const elements = new Map();
function getOrCreateElement(id, initialClasses = []) {
  if (!elements.has(id)) {
    elements.set(id, new DOMElement(id, initialClasses));
  }
  return elements.get(id);
}

const elementsToInit = [
  ['btn-grid-add-row', ['hidden']],
  ['btn-grid-delete-selected', ['hidden']],
  ['btn-mock-data', ['hidden']],
  ['btn-import-csv', ['hidden']],
  ['grid-edit-mode-container', ['hidden']],
  ['export-menu-table-dump', ['hidden']],
  ['grid-ctx-table-mutations', ['hidden']],
  ['grid-ctx-filter-section', ['hidden']],
  ['grid-ctx-insert-section', ['hidden']],
  ['grid-ctx-btn-edit', ['hidden']],
  ['grid-ctx-btn-json', ['hidden']],
  ['grid-ctx-btn-dup-nokey', []],
  ['grid-ctx-btn-dup-key', []],
  ['grid-ctx-btn-add-draft', []],
  ['grid-ctx-btn-delete-row', []],
  ['grid-ctx-btn-copy-cell', []],
  ['grid-ctx-delete-row-label', []],
  ['grid-ctx-copy-tsv-label', []],
  ['grid-ctx-copy-json-label', []],
  ['grid-ctx-copy-sql-label', []],
  ['grid-toolbar', []],
  ['table-filter-bar', ['hidden']],
  ['virtual-grid-wrapper', []],
  ['grid-pagination-bar', []],
  ['grid-context-menu', ['hidden']],
  ['grid-thead', []],
  ['grid-tbody', []],
  ['results-table', []],
  ['grid-result-stats', []],
  ['grid-limit-badge', []],
  ['btn-export-csv', []],
  ['btn-grid-apply', ['hidden']],
  ['btn-grid-discard', ['hidden']],
  ['grid-staged-count', []],
  ['grid-selected-count', []]
];

elementsToInit.forEach(([id, classes]) => getOrCreateElement(id, classes));

let copiedClipboardText = '';
const sandbox = {
  console,
  showToast: () => {},
  document: {
    body: { querySelector: () => null, appendChild: () => {} },
    createElement: () => new DOMElement('created'),
    getElementById: (id) => getOrCreateElement(id),
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener: () => {}
  },
  window: {
    addEventListener: () => {},
    innerWidth: 1200,
    innerHeight: 800
  },
  navigator: {
    clipboard: {
      writeText: async (text) => {
        copiedClipboardText = text;
        return Promise.resolve();
      }
    }
  },
  localStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  sessionStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  setTimeout: () => {},
  clearTimeout: () => {},
  setInterval: () => {},
  clearInterval: () => {},
  requestAnimationFrame: () => {},
  fetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) })
};

vm.createContext(sandbox);

try {
  vm.runInContext(scriptContent, sandbox);
} catch (err) {
  // Ignore missing DOM references on boot
}

sandbox.assert = assert;

// 1. Test quoteIdentifierForDriver across all drivers
console.log("Testing quoteIdentifierForDriver...");
vm.runInContext(`
  activeConnection = { driver: 'mysql' };
  assert.strictEqual(quoteIdentifierForDriver('users'), '\`users\`');
  assert.strictEqual(quoteIdentifierForDriver('table\`with\`ticks'), '\`table\`\`with\`\`ticks\`');

  activeConnection = { driver: 'postgres' };
  assert.strictEqual(quoteIdentifierForDriver('users'), '"users"');
  assert.strictEqual(quoteIdentifierForDriver('col"with"quotes'), '"col""with""quotes"');

  activeConnection = { driver: 'sqlite' };
  assert.strictEqual(quoteIdentifierForDriver('users'), '"users"');
`, sandbox);
console.log("✓ quoteIdentifierForDriver passed for MySQL, PostgreSQL, and SQLite!");

// 2. Test copySelectedRowsAsSQL for PostgreSQL dialect
console.log("Testing copySelectedRowsAsSQL for PostgreSQL...");
vm.runInContext(`
  activeConnection = { driver: 'postgres' };
  currentColumns = [
    { name: 'id', type: 'integer' },
    { name: 'username', type: 'text' },
    { name: 'is_active', type: 'boolean' }
  ];
  currentResults = [
    [1, "O'Connor", true],
    [2, 'Alice', false]
  ];
  selectedRowIndices = new Set([0, 1]);
  activeGridTable = 'user_accounts';
`, sandbox);

vm.runInContext(`copySelectedRowsAsSQL()`, sandbox);
assert(copiedClipboardText.includes('INSERT INTO "user_accounts" ("id", "username", "is_active") VALUES (1, \'O\'\'Connor\', TRUE);'), "PostgreSQL INSERT statement did not match expected double quotes and TRUE boolean");
assert(copiedClipboardText.includes('INSERT INTO "user_accounts" ("id", "username", "is_active") VALUES (2, \'Alice\', FALSE);'), "PostgreSQL INSERT statement did not match expected double quotes and FALSE boolean");
console.log("✓ copySelectedRowsAsSQL produced dialect-perfect PostgreSQL INSERT statements!");

// 3. Test duplicateGridRow without key
console.log("Testing duplicateGridRow without key...");
vm.runInContext(`
  stagedDraftRows = [];
  activeConnection = { driver: 'postgres' };
  tabs = [{ id: 'tab-q1', title: 'Query', sql: 'SELECT id, username, is_active FROM user_accounts;', isTableViewer: false }];
  activeTabId = 'tab-q1';
  schemaCatalogCache = {
    database_name: 'public',
    tables: [{
      name: 'user_accounts',
      columns: [
        { name: 'id', is_primary_key: true },
        { name: 'username', is_primary_key: false },
        { name: 'is_active', is_primary_key: false }
      ]
    }]
  };
  activeGridTable = 'user_accounts';
  activeGridPKCol = 'id';
  selectedRowIndices = new Set([0]);
  activeGridRightClickedRow = 0;
  duplicateGridRow(true);
`, sandbox);

const draftRows = vm.runInContext(`stagedDraftRows`, sandbox);
assert.strictEqual(draftRows.length, 1, "Expected 1 draft row");
assert.strictEqual(draftRows[0].data['id'], undefined, "Primary Key 'id' MUST NOT be populated with empty string on duplicate without key");
assert.strictEqual(draftRows[0].data['username'], "O'Connor", "Username must be copied");
assert.strictEqual(draftRows[0].data['is_active'], true, "is_active boolean must be copied");
console.log("✓ duplicateGridRow without key properly omits PK so DB sequences/auto-increments work cleanly!");

console.log("--- ALL TESTS PASSED SUCCESSFULLY! ---");
