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
assert(typeof sandbox.openRecordInspector === 'function', 'openRecordInspector must be defined as a function');
assert(typeof sandbox.navigateRecordInspector === 'function', 'navigateRecordInspector must be defined as a function');
assert(typeof sandbox.onRecordFieldChange === 'function', 'onRecordFieldChange must be defined as a function');

console.log("✓ All assertions passed successfully!");
