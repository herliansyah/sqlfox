import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

console.log("--- Starting Grid Context Menu Grouping & Overflow Resilience Tests ---");

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// 1. Verify HTML Structure & Submenus exist
assert(html.includes('id="grid-context-menu"'), 'grid-context-menu must exist');
assert(html.includes('id="grid-ctx-filter-parent"'), 'grid-ctx-filter-parent trigger button must exist');
assert(html.includes('id="grid-ctx-insert-parent"'), 'grid-ctx-insert-parent trigger button must exist');
assert(html.includes('id="grid-ctx-copy-parent"'), 'grid-ctx-copy-parent trigger button must exist');
assert(html.includes('id="grid-ctx-mutations-parent"'), 'grid-ctx-mutations-parent trigger button must exist');
assert(html.includes('id="grid-ctx-filter-submenu"'), 'grid-ctx-filter-submenu must exist');
assert(html.includes('id="grid-ctx-insert-submenu"'), 'grid-ctx-insert-submenu must exist');
assert(html.includes('id="grid-ctx-copy-submenu"'), 'grid-ctx-copy-submenu must exist');
assert(html.includes('id="grid-ctx-mutations-submenu"'), 'grid-ctx-mutations-submenu must exist');

// Verify existing required IDs are preserved inside their respective submenus
assert(html.includes('id="grid-ctx-filter-section"'), 'grid-ctx-filter-section must exist');
assert(html.includes('id="grid-ctx-filter-options"'), 'grid-ctx-filter-options must exist');
assert(html.includes('id="grid-ctx-insert-section"'), 'grid-ctx-insert-section must exist');
assert(html.includes('id="grid-ctx-insert-options"'), 'grid-ctx-insert-options must exist');
assert(html.includes('id="grid-ctx-btn-copy-row-tsv"'), 'grid-ctx-btn-copy-row-tsv must exist');
assert(html.includes('id="grid-ctx-btn-copy-row-json"'), 'grid-ctx-btn-copy-row-json must exist');
assert(html.includes('id="grid-ctx-btn-copy-row-sql"'), 'grid-ctx-btn-copy-row-sql must exist');
assert(html.includes('id="grid-ctx-table-mutations"'), 'grid-ctx-table-mutations must exist');
assert(html.includes('id="grid-ctx-btn-dup-nokey"'), 'grid-ctx-btn-dup-nokey must exist');
assert(html.includes('id="grid-ctx-btn-dup-key"'), 'grid-ctx-btn-dup-key must exist');
assert(html.includes('id="grid-ctx-btn-add-draft"'), 'grid-ctx-btn-add-draft must exist');
assert(html.includes('id="grid-ctx-btn-delete-row"'), 'grid-ctx-btn-delete-row must exist');

console.log("✓ HTML structural checks for context menu and submenus passed!");

// 2. Extract script content
const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
assert(lastScriptStart !== -1 && lastScriptEnd !== -1, "Could not find main <script> tag");
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

// 3. Build sandbox
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
    this.rect = { left: 100, right: 320, top: 200, bottom: 230, width: 220, height: 220 };
  }
  getBoundingClientRect() { return this.rect; }
  addEventListener() {}
  appendChild() {}
  closest() { return null; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  setAttribute(attr, val) { this[attr] = val; }
  removeAttribute(attr) { delete this[attr]; }
}

const elements = new Map();
function getOrCreateElement(id, initialClasses = []) {
  if (!elements.has(id)) {
    elements.set(id, new DOMElement(id, initialClasses));
  }
  return elements.get(id);
}

const ids = [
  'grid-context-menu',
  'grid-ctx-smart-actions',
  'grid-ctx-filter-parent',
  'grid-ctx-insert-parent',
  'grid-ctx-copy-parent',
  'grid-ctx-mutations-parent',
  'grid-ctx-mutations-label',
  'grid-ctx-filter-submenu',
  'grid-ctx-insert-submenu',
  'grid-ctx-copy-submenu',
  'grid-ctx-mutations-submenu',
  'grid-ctx-filter-section',
  'grid-ctx-filter-options',
  'grid-ctx-filter-colname',
  'grid-ctx-insert-section',
  'grid-ctx-insert-options',
  'grid-ctx-colname',
  'grid-ctx-btn-inspect',
  'grid-ctx-btn-edit',
  'grid-ctx-btn-json',
  'grid-ctx-btn-copy-cell',
  'grid-ctx-btn-copy-row-tsv',
  'grid-ctx-copy-tsv-label',
  'grid-ctx-btn-copy-row-json',
  'grid-ctx-copy-json-label',
  'grid-ctx-btn-copy-row-sql',
  'grid-ctx-copy-sql-label',
  'grid-ctx-table-mutations',
  'grid-ctx-btn-dup-nokey',
  'grid-ctx-btn-dup-key',
  'grid-ctx-btn-add-draft',
  'grid-ctx-btn-delete-row',
  'grid-ctx-delete-row-label'
];

ids.forEach(id => getOrCreateElement(id));

const documentMock = {
  getElementById: (id) => getOrCreateElement(id),
  querySelectorAll: (selector) => {
    if (selector === '.grid-submenu') {
      return [
        getOrCreateElement('grid-ctx-filter-submenu'),
        getOrCreateElement('grid-ctx-insert-submenu'),
        getOrCreateElement('grid-ctx-copy-submenu'),
        getOrCreateElement('grid-ctx-mutations-submenu')
      ];
    }
    return [];
  },
  querySelector: () => null,
  addEventListener: () => {}
};

const windowMock = {
  addEventListener: () => {},
  innerWidth: 1200,
  innerHeight: 800
};

const sandbox = {
  console,
  ClassList,
  document: documentMock,
  window: windowMock,
  localStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  sessionStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  setTimeout: (cb) => { cb(); return 1; },
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
  console.log("Note during script execution:", err.message);
}

// 4. Test Viewport Collision & Clamping when clicking near bottom-right
console.log("Testing bottom-right viewport clamping...");
vm.runInContext(`
  tabs = [{ id: 'tab-users', title: 'users', tableName: 'users', isTableViewer: true, tableMode: 'data' }];
  activeTabId = 'tab-users';
  currentColumns = [{ name: 'id', type: 'int' }, { name: 'email', type: 'varchar(255)' }];
  currentResults = [[1, 'test@example.com']];
  activeGridTable = 'users';

  const menuEl = document.getElementById('grid-context-menu');
  menuEl.rect = { width: 230, height: 260 };

  const fakeTd = {
    closest: (sel) => fakeTd,
    classList: new ClassList(),
    dataset: { col: '1' }
  };

  // Click near the bottom right: (clientX = 1180, clientY = 780) on 1200x800 screen
  const eventBottomRight = {
    preventDefault: () => {},
    target: fakeTd,
    clientX: 1180,
    clientY: 780
  };

  openGridContextMenu(eventBottomRight, 0);
`, sandbox);

const menuEl = getOrCreateElement('grid-context-menu');
const leftVal = parseInt(menuEl.style.left, 10);
const topVal = parseInt(menuEl.style.top, 10);

assert(!menuEl.classList.contains('hidden'), 'Menu should be visible');
assert(leftVal + 230 <= 1200, `Menu left (${leftVal}) + width (230) must not overflow window.innerWidth (1200)`);
assert(topVal + 260 <= 800, `Menu top (${topVal}) + height (260) must not overflow window.innerHeight (800)`);
assert(topVal >= 8, `Menu top (${topVal}) must be >= padding`);
console.log(`✓ Context menu placed securely at (${leftVal}px, ${topVal}px), within 1200x800 bounds!`);

// 5. Test Submenu Positioning & Flip on Right Boundary
console.log("Testing submenu opening and leftward-flip near edge...");
vm.runInContext(`
  const triggerEl = document.getElementById('grid-ctx-copy-parent');
  // Trigger button is located at right edge: left 1050, right 1180, top 520
  triggerEl.rect = { left: 1050, right: 1180, top: 520, bottom: 550, width: 130, height: 30 };

  const copySub = document.getElementById('grid-ctx-copy-submenu');
  copySub.rect = { width: 240, height: 180 };

  openGridSubmenu('grid-ctx-copy-submenu', triggerEl);
`, sandbox);

const copySub = getOrCreateElement('grid-ctx-copy-submenu');
const subLeft = parseInt(copySub.style.left, 10);
const subTop = parseInt(copySub.style.top, 10);

assert(!copySub.classList.contains('hidden'), 'Copy submenu must be visible');
assert(subLeft + 240 <= 1200, `Submenu left (${subLeft}) + width (240) must fit within viewport`);
// Since right = 1180 + 240 > 1200, it must have flipped to the left of trigger (left <= 1050 - 240 - 2)
assert(subLeft < 1050, `Submenu should have flipped leftwards when right side lacked room (left = ${subLeft})`);
assert(subTop + 180 <= 800, `Submenu top (${subTop}) + height (180) must fit within window.innerHeight`);
console.log(`✓ Submenu properly flipped leftwards to ${subLeft}px to avoid overflow!`);

// 6. Test Closing All Context Menus
console.log("Testing closeGridContextMenu closes all submenus...");
vm.runInContext(`
  closeGridContextMenu();
`, sandbox);

assert(menuEl.classList.contains('hidden'), 'Main menu must be hidden');
assert(copySub.classList.contains('hidden'), 'Submenus must be hidden');
console.log("✓ closeGridContextMenu cleanly dismisses main menu and submenus!");

console.log("\n🎉 ALL GRID CONTEXT MENU GROUPING & OVERFLOW TESTS PASSED 100%!");
