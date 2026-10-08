import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// 1. Verify HTML elements exist
assert(html.includes('id="tab-context-menu"'), 'tab-context-menu HTML must exist');
assert(html.includes('id="tab-ctx-btn-close"'), 'tab-ctx-btn-close must exist');
assert(html.includes('id="tab-ctx-btn-close-others"'), 'tab-ctx-btn-close-others must exist');
assert(html.includes('id="tab-ctx-btn-close-right"'), 'tab-ctx-btn-close-right must exist');
assert(html.includes('id="tab-ctx-btn-close-all"'), 'tab-ctx-btn-close-all must exist');
assert(html.includes('id="tab-ctx-btn-duplicate"'), 'tab-ctx-btn-duplicate must exist');
assert(html.includes('id="tab-ctx-btn-rename"'), 'tab-ctx-btn-rename must exist');
assert(html.includes('id="app-native-dialog-input-container"'), 'app-native-dialog-input-container must exist');

// 2. Extract Javascript from the main application <script> tag
const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
if (lastScriptStart === -1 || lastScriptEnd === -1) {
  console.error("Could not find main <script> tag in web/dist/index.html");
  process.exit(2);
}
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

// 3. Build browser sandbox
const storage = new Map();
const localStorageMock = {
  getItem: (k) => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: (k) => storage.delete(k),
  clear: () => storage.clear()
};

let confirmMockResult = true;
let confirmCalls = [];
let promptMockResult = 'Renamed Tab.sql';
let promptCalls = [];

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
  cloneNode: function() { return domElementStub(this.id); }
});

const elements = new Map();
function getEl(id) {
  if (!elements.has(id)) {
    elements.set(id, domElementStub(id));
  }
  return elements.get(id);
}

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
    addEventListener: () => {},
    removeEventListener: () => {},
    location: { reload: () => {} },
    alert: () => {},
    confirm: () => confirmMockResult,
    prompt: () => promptMockResult
  },
  document: {
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
} catch (err) {
  console.error("Script execution error:", err);
  process.exit(2);
}

// Override showNativeConfirm and showNativePrompt inside VM for deterministic testing
sandbox.confirmCalls = confirmCalls;
sandbox.promptCalls = promptCalls;
sandbox.getConfirmMockResult = () => confirmMockResult;
sandbox.getPromptMockResult = () => promptMockResult;

vm.runInContext(`
  showNativeConfirm = async function(msg, title) {
    confirmCalls.push({ msg, title });
    return getConfirmMockResult();
  };
  showNativePrompt = async function(msg, def, title) {
    promptCalls.push({ msg, def, title });
    return getPromptMockResult();
  };
`, sandbox);

// Hook into VM variables
async function runTests() {
  console.log("=== Testing Tab Context Menu & Close Confirmation ===");

  // Setup tabs
  vm.runInContext(`
    tabs = [
      { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT * FROM users;', isTableViewer: false },
      { id: 'tab-2', title: 'Query 2.sql', sql: '', isTableViewer: false },
      { id: 'tab-3', title: 'Query 3.sql', sql: 'SELECT 42;', isTableViewer: false }
    ];
    activeTabId = 'tab-1';
    renderTabs();
  `, sandbox);

  // Test 1: renderTabs includes oncontextmenu
  const tabsContainerHtml = getEl('tabs-container').innerHTML;
  assert(tabsContainerHtml.includes('oncontextmenu="showTabContextMenu(event,'), 'renderTabs must wire oncontextmenu handler');
  console.log("✅ Test 1 Passed: oncontextmenu wired in renderTabs");

  // Test 2: showTabContextMenu
  vm.runInContext(`
    showTabContextMenu({ preventDefault: () => {}, stopPropagation: () => {}, clientX: 200, clientY: 100 }, 'tab-1');
  `, sandbox);
  const selectedTabId = vm.runInContext(`selectedContextMenuTabId`, sandbox);
  assert.strictEqual(selectedTabId, 'tab-1', 'selectedContextMenuTabId must be set');
  const menuEl = getEl('tab-context-menu');
  assert(!menuEl.classList.contains('hidden'), 'tab-context-menu must not be hidden');
  console.log("✅ Test 2 Passed: showTabContextMenu opens menu correctly");

  // Test 3: Close clean/empty tab (tab-2) -> should NOT prompt confirmation
  confirmCalls.length = 0;
  await vm.runInContext(`closeTab('tab-2')`, sandbox);
  let currentTabs = vm.runInContext(`tabs.map(t => t.id)`, sandbox);
  assert.strictEqual(JSON.stringify(currentTabs), JSON.stringify(['tab-1', 'tab-3']), 'Empty tab-2 should be closed');
  assert.strictEqual(confirmCalls.length, 0, 'No confirmation prompt should appear for empty tab');
  console.log("✅ Test 3 Passed: Empty tab closes without confirmation prompt");

  // Test 4: Close dirty/non-empty tab (tab-3) -> user cancels
  confirmCalls.length = 0;
  confirmMockResult = false; // User cancels
  await vm.runInContext(`closeTab('tab-3')`, sandbox);
  currentTabs = vm.runInContext(`tabs.map(t => t.id)`, sandbox);
  assert.strictEqual(JSON.stringify(currentTabs), JSON.stringify(['tab-1', 'tab-3']), 'Tab-3 must remain open when user cancels');
  assert.strictEqual(confirmCalls.length, 1, 'Confirmation prompt must be called');
  console.log("✅ Test 4 Passed: Non-empty tab canceled by user is not closed");

  // Test 5: Close dirty/non-empty tab (tab-3) -> user confirms
  confirmCalls.length = 0;
  confirmMockResult = true; // User confirms
  await vm.runInContext(`closeTab('tab-3')`, sandbox);
  currentTabs = vm.runInContext(`tabs.map(t => t.id)`, sandbox);
  assert.strictEqual(JSON.stringify(currentTabs), JSON.stringify(['tab-1']), 'Tab-3 must be closed after user confirmation');
  assert.strictEqual(confirmCalls.length, 1, 'Confirmation prompt must be called');
  console.log("✅ Test 5 Passed: Non-empty tab confirmed by user is closed");

  // Test 6: Duplicate Tab (tab-1)
  vm.runInContext(`duplicateTab('tab-1')`, sandbox);
  currentTabs = vm.runInContext(`tabs.map(t => ({ id: t.id, title: t.title, sql: t.sql }))`, sandbox);
  assert.strictEqual(currentTabs.length, 2, 'Should have 2 tabs after duplicate');
  assert.strictEqual(currentTabs[1].title, 'Query 1 (Copy).sql', 'Title should be Query 1 (Copy).sql');
  assert.strictEqual(currentTabs[1].sql, 'SELECT * FROM users;', 'SQL should be copied');
  const activeId = vm.runInContext(`activeTabId`, sandbox);
  assert.strictEqual(activeId, currentTabs[1].id, 'Active tab should switch to duplicated tab');
  console.log("✅ Test 6 Passed: Duplicate tab clones content and title correctly");

  // Test 7: Rename Tab
  promptCalls.length = 0;
  promptMockResult = 'Custom Query.sql';
  await vm.runInContext(`renameTab('${activeId}')`, sandbox);
  const renamedTab = vm.runInContext(`tabs.find(t => t.id === '${activeId}')`, sandbox);
  assert.strictEqual(renamedTab.title, 'Custom Query.sql', 'Tab title must be updated');
  assert.strictEqual(promptCalls.length, 1, 'Prompt should be called');
  console.log("✅ Test 7 Passed: Rename tab updates title correctly");

  // Test 8: Close Others
  confirmCalls.length = 0;
  confirmMockResult = true;
  await vm.runInContext(`closeOtherTabs('tab-1')`, sandbox);
  currentTabs = vm.runInContext(`tabs.map(t => t.id)`, sandbox);
  assert.strictEqual(JSON.stringify(currentTabs), JSON.stringify(['tab-1']), 'Only tab-1 should remain');
  console.log("✅ Test 8 Passed: Close others keeps only target tab");

  // Test 9: Close last remaining tab -> resets to fresh Query 1.sql
  confirmCalls.length = 0;
  confirmMockResult = true;
  await vm.runInContext(`closeTab('tab-1')`, sandbox);
  currentTabs = vm.runInContext(`tabs.map(t => ({ title: t.title, sql: t.sql }))`, sandbox);
  assert.strictEqual(currentTabs.length, 1, 'Must have 1 tab');
  assert.strictEqual(currentTabs[0].title, 'Query 1.sql', 'Reset tab should be Query 1.sql');
  assert.strictEqual(currentTabs[0].sql, '', 'Reset tab should have empty SQL');
  console.log("✅ Test 9 Passed: Closing last tab resets to fresh Query 1.sql");

  // Test 10: Close All Tabs
  vm.runInContext(`
    tabs = [
      { id: 'tab-10', title: 'A.sql', sql: 'SELECT 1;', isTableViewer: false },
      { id: 'tab-20', title: 'B.sql', sql: 'SELECT 2;', isTableViewer: false }
    ];
    activeTabId = 'tab-10';
  `, sandbox);
  confirmCalls.length = 0;
  confirmMockResult = true;
  await vm.runInContext(`closeAllTabs()`, sandbox);
  currentTabs = vm.runInContext(`tabs.map(t => ({ title: t.title, sql: t.sql }))`, sandbox);
  assert.strictEqual(currentTabs.length, 1, 'Must have 1 tab after close all');
  assert.strictEqual(currentTabs[0].title, 'Query 1.sql', 'Reset tab should be Query 1.sql');
  assert.strictEqual(confirmCalls.length, 1, 'Close all must ask confirmation');
  console.log("✅ Test 10 Passed: Close all tabs resets workspace with confirmation");

  console.log("\n🎉 ALL 10 TESTS PASSED (GREEN)!");
}

runTests().catch(err => {
  console.error("Test failed:", err);
  process.exit(1);
});
