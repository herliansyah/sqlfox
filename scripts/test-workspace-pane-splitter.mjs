import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// 1. Verify HTML elements
assert(html.includes('id="workspace-pane-splitter"'), 'workspace-pane-splitter element must exist in HTML');
assert(html.includes('cursor-row-resize'), 'workspace-pane-splitter must have cursor-row-resize class');

// 2. Extract script
const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

// 3. Build sandbox
const storage = new Map();
const localStorageMock = {
  getItem: (k) => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: (k) => storage.delete(k),
  clear: () => storage.clear()
};

const domElementStub = (id = '') => {
  const listeners = new Map();
  return {
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
    offsetHeight: 300,
    clientHeight: 600,
    addEventListener(event, fn) {
      if (!listeners.has(event)) listeners.set(event, []);
      listeners.get(event).push(fn);
    },
    removeEventListener(event, fn) {
      if (listeners.has(event)) {
        listeners.set(event, listeners.get(event).filter(f => f !== fn));
      }
    },
    dispatchEvent(event, data = {}) {
      if (listeners.has(event)) {
        listeners.get(event).forEach(fn => fn({ preventDefault: () => {}, stopPropagation: () => {}, ...data }));
      }
    },
    contains: () => false,
    querySelector: () => null,
    parentElement: null
  };
};

const elements = new Map();
function getEl(id) {
  if (!elements.has(id)) {
    elements.set(id, domElementStub(id));
  }
  return elements.get(id);
}

const windowListeners = new Map();
const sandbox = {
  console,
  devicePixelRatio: 1,
  fetch: async () => ({ ok: true, json: async () => ({}) }),
  setTimeout: (fn) => fn(),
  clearTimeout: () => {},
  setInterval: () => {},
  clearInterval: () => {},
  localStorage: localStorageMock,
  sessionStorage: localStorageMock,
  window: {
    devicePixelRatio: 1,
    innerWidth: 1920,
    innerHeight: 1080,
    addEventListener: (event, fn) => {
      if (!windowListeners.has(event)) windowListeners.set(event, []);
      windowListeners.get(event).push(fn);
    },
    removeEventListener: () => {},
    dispatchEvent: (event, data = {}) => {
      if (windowListeners.has(event)) {
        windowListeners.get(event).forEach(fn => fn(data));
      }
    },
    location: { reload: () => {} },
    alert: () => {},
    confirm: () => true,
    prompt: () => ''
  },
  document: {
    body: { style: {} },
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
vm.runInContext(scriptContent, sandbox);

async function runTests() {
  console.log("=== Testing Workspace Pane Splitter ===");

  const splitterEl = getEl('workspace-pane-splitter');
  const editorEl = getEl('editor-container');
  const mainEl = domElementStub('main-workspace');
  mainEl.clientHeight = 800;
  editorEl.parentElement = mainEl;
  editorEl.offsetHeight = 350;
  splitterEl.offsetHeight = 6;

  // Test 1: initWorkspacePaneSplitter function exists and runs
  assert(typeof sandbox.initWorkspacePaneSplitter === 'function', 'initWorkspacePaneSplitter function must exist');
  sandbox.initWorkspacePaneSplitter();
  console.log("✅ Test 1 Passed: initWorkspacePaneSplitter initializes cleanly");

  // Test 2: Double-click resets to 50% and clears localStorage
  storage.set('sqlfox_workspace_editor_height', '450px');
  splitterEl.dispatchEvent('dblclick');
  assert.strictEqual(editorEl.style.height, '50%', 'Double-click must reset height to 50%');
  assert.strictEqual(storage.has('sqlfox_workspace_editor_height'), false, 'Double-click must clear stored preference');
  console.log("✅ Test 2 Passed: Double-click resets editor height to 50%");

  // Test 3: Mouse drag down increases editor height
  splitterEl.dispatchEvent('mousedown', { clientY: 400 });
  sandbox.window.dispatchEvent('mousemove', { clientY: 460 }); // +60px
  assert.strictEqual(editorEl.style.height, '410px', 'Editor height should increase by 60px');
  sandbox.window.dispatchEvent('mouseup');
  assert.strictEqual(storage.get('sqlfox_workspace_editor_height'), '410px', 'Mouseup must persist height in localStorage');
  console.log("✅ Test 3 Passed: Mouse drag down expands editor and saves preference");

  // Test 4: Dragging beyond min-height bounds is clamped
  splitterEl.dispatchEvent('mousedown', { clientY: 400 });
  sandbox.window.dispatchEvent('mousemove', { clientY: 50 }); // attempt to shrink below 80px
  assert.strictEqual(editorEl.style.height, '80px', 'Editor height must not shrink below 80px');
  sandbox.window.dispatchEvent('mouseup');
  console.log("✅ Test 4 Passed: Min-height constraint (80px) is strictly enforced");

  // Test 5: Dragging down respects grid min-height (120px)
  // Available height = 800 - 36 (tabs) - 6 (splitter) = 758px. Max editor = 758 - 120 = 638px.
  splitterEl.dispatchEvent('mousedown', { clientY: 400 });
  sandbox.window.dispatchEvent('mousemove', { clientY: 9999 }); // attempt to shrink grid below 120px
  assert.strictEqual(editorEl.style.height, '638px', 'Grid min-height constraint (120px) is strictly enforced');
  sandbox.window.dispatchEvent('mouseup');
  console.log("✅ Test 5 Passed: Grid min-height constraint (120px) is strictly enforced");

  // Test 6: renderTabs toggles splitter visibility
  vm.runInContext(`
    tabs = [
      { id: 'tab-query', title: 'Query.sql', sql: '', isTableViewer: false },
      { id: 'tab-table', title: 'users', isTableViewer: true, tableName: 'users' }
    ];
    activeTabId = 'tab-query';
    renderTabs();
  `, sandbox);
  assert(!splitterEl.classList.contains('hidden'), 'Splitter should be visible when Query tab is active');

  vm.runInContext(`
    activeTabId = 'tab-table';
    renderTabs();
  `, sandbox);
  assert(splitterEl.classList.contains('hidden'), 'Splitter should be hidden when Table Viewer tab is active');
  console.log("✅ Test 6 Passed: Splitter dynamically hides on Table Viewer and shows on Query tab");

  console.log("\n🎉 ALL SPLITTER TESTS PASSED (GREEN)!");
}

runTests().catch(err => {
  console.error("Test failed:", err);
  process.exit(1);
});
