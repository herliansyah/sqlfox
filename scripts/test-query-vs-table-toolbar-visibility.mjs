import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

console.log("--- Starting Query vs Table Toolbar Visibility Test ---");

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// 1. Verify HTML IDs and initial hidden classes
assert(html.includes('id="btn-grid-add-row"'), 'btn-grid-add-row must exist');
assert(html.includes('id="btn-mock-data"'), 'btn-mock-data must exist');
assert(html.includes('id="btn-import-csv"'), 'btn-import-csv must exist');
assert(html.includes('id="grid-edit-mode-container"'), 'grid-edit-mode-container must exist');
assert(html.includes('id="export-menu-table-dump"'), 'export-menu-table-dump must exist');
assert(html.includes('id="grid-ctx-table-mutations"'), 'grid-ctx-table-mutations must exist');

// 2. Extract Javascript from the main application <script> tag
const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
if (lastScriptStart === -1 || lastScriptEnd === -1) {
  console.error("Could not find main <script> tag in web/dist/index.html");
  process.exit(2);
}
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

// 3. Build browser sandbox
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
  closest() { return null; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
}

const elements = new Map();
function getOrCreateElement(id, initialClasses = []) {
  if (!elements.has(id)) {
    elements.set(id, new DOMElement(id, initialClasses));
  }
  return elements.get(id);
}

// Pre-create relevant DOM elements
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

const documentMock = {
  getElementById: (id) => getOrCreateElement(id),
  querySelector: () => null,
  querySelectorAll: () => [],
  addEventListener: () => {}
};

const windowMock = {
  addEventListener: () => {},
  innerWidth: 1200,
  innerHeight: 800
};

const sandbox = {
  console,
  document: documentMock,
  window: windowMock,
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
  // Ignore top-level boot errors if any missing DOM references
  console.log("Note during script execution:", err.message);
}

// 4. Test Case 1: Initial Query Tab (isTableViewer === false)
vm.runInContext(`
  tabs = [{ id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT 1;', isTableViewer: false }];
  activeTabId = 'tab-1';
  syncGridToolbarMode();
`, sandbox);

assert(getOrCreateElement('btn-grid-add-row').classList.contains('hidden'), 'Add Row button MUST be hidden in Query tab');
assert(getOrCreateElement('btn-mock-data').classList.contains('hidden'), 'Mock Data button MUST be hidden in Query tab');
assert(getOrCreateElement('btn-import-csv').classList.contains('hidden'), 'Import CSV button MUST be hidden in Query tab');
assert(getOrCreateElement('grid-edit-mode-container').classList.contains('hidden'), 'Edit mode container MUST be hidden in Query tab');
assert(getOrCreateElement('export-menu-table-dump').classList.contains('hidden'), 'Table SQL Dump in export menu MUST be hidden in Query tab');

console.log("✓ Test Case 1 passed: Table-specific buttons hidden on Query tab");

// 5. Test Case 2: Table Viewer Tab (isTableViewer === true)
vm.runInContext(`
  tabs = [{ id: 'tab-users', title: 'users', tableName: 'users', isTableViewer: true, tableMode: 'data' }];
  activeTabId = 'tab-users';
  syncGridToolbarMode();
`, sandbox);

assert(!getOrCreateElement('btn-grid-add-row').classList.contains('hidden'), 'Add Row button MUST be visible in Table Viewer tab');
assert(!getOrCreateElement('btn-mock-data').classList.contains('hidden'), 'Mock Data button MUST be visible in Table Viewer tab');
assert(!getOrCreateElement('btn-import-csv').classList.contains('hidden'), 'Import CSV button MUST be visible in Table Viewer tab');
assert(!getOrCreateElement('grid-edit-mode-container').classList.contains('hidden'), 'Edit mode container MUST be visible in Table Viewer tab');
assert(!getOrCreateElement('export-menu-table-dump').classList.contains('hidden'), 'Table SQL Dump MUST be visible in Table Viewer tab');

console.log("✓ Test Case 2 passed: Table-specific buttons visible on Table Viewer tab");

// 6. Test Case 3: Switching back to Query Tab hides buttons again
vm.runInContext(`
  tabs.push({ id: 'tab-query', title: 'Query 2.sql', sql: 'SELECT * FROM users;', isTableViewer: false });
  activeTabId = 'tab-query';
  syncGridToolbarMode();
`, sandbox);

assert(getOrCreateElement('btn-grid-add-row').classList.contains('hidden'), 'Add Row button MUST be hidden after switching back to Query tab');
assert(getOrCreateElement('btn-mock-data').classList.contains('hidden'), 'Mock Data button MUST be hidden after switching back to Query tab');
assert(getOrCreateElement('btn-import-csv').classList.contains('hidden'), 'Import CSV button MUST be hidden after switching back to Query tab');
assert(getOrCreateElement('grid-edit-mode-container').classList.contains('hidden'), 'Edit mode container MUST be hidden after switching back to Query tab');
assert(getOrCreateElement('export-menu-table-dump').classList.contains('hidden'), 'Table SQL Dump MUST be hidden after switching back to Query tab');

console.log("✓ Test Case 3 passed: Switching back to Query tab hides table buttons correctly");

console.log("✓ Test Case 3 passed: Switching back to Query tab hides table buttons correctly");

console.log("--- All Toolbar Visibility Tests Passed Successfully! ---");
