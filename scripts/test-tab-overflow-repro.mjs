import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// 1. Verify HTML elements exist
assert(html.includes('id="tab-overflow-wrapper"'), 'tab-overflow-wrapper HTML must exist');
assert(html.includes('id="tab-overflow-btn"'), 'tab-overflow-btn must exist');
assert(html.includes('id="tab-overflow-count"'), 'tab-overflow-count must exist');
assert(html.includes('id="tab-overflow-dropdown"'), 'tab-overflow-dropdown must exist');

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

const domElementStub = (id = '') => {
  const el = {
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
    clientWidth: 800,
    offsetWidth: 120,
    children: [],
    disabled: false,
    addEventListener: () => {},
    removeEventListener: () => {},
    focus: () => {},
    select: () => {},
    blur: () => {},
    contains: () => false,
    getBoundingClientRect: () => ({ top: 40, bottom: 76, left: 700, right: 750, width: 50, height: 36 }),
    cloneNode: function() { return domElementStub(this.id); }
  };
  return el;
};

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
  requestAnimationFrame: (fn) => fn(),
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
    prompt: () => 'Test'
  },
  document: {
    getElementById: (id) => getEl(id),
    querySelectorAll: () => [],
    querySelector: (sel) => getEl(sel.replace(/^[#.]/, '')),
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

// Override showNativeConfirm inside VM
sandbox.confirmCalls = confirmCalls;
sandbox.getConfirmMockResult = () => confirmMockResult;

vm.runInContext(`
  showNativeConfirm = async function(msg, title) {
    confirmCalls.push({ msg, title });
    return getConfirmMockResult();
  };
`, sandbox);

async function runTests() {
  console.log("=== Testing Tab Overflow Dropdown Menu ===");

  // Test 1: Function definitions exist
  assert(typeof sandbox.updateTabOverflow === 'function', 'updateTabOverflow function must exist');
  assert(typeof sandbox.toggleTabOverflowMenu === 'function', 'toggleTabOverflowMenu function must exist');
  assert(typeof sandbox.closeTabOverflowMenu === 'function', 'closeTabOverflowMenu function must exist');
  assert(typeof sandbox.selectOverflowTab === 'function', 'selectOverflowTab function must exist');
  assert(typeof sandbox.closeTabFromOverflow === 'function', 'closeTabFromOverflow function must exist');
  console.log("✅ Test 1 Passed: Tab overflow functions exist in global scope");

  // Test 2: In narrow container, extra tabs are marked hidden and overflow button is visible with counter
  const tabsBar = getEl('tabs-bar');
  const container = getEl('tabs-container');
  const overflowWrapper = getEl('tab-overflow-wrapper');
  const overflowCountEl = getEl('tab-overflow-count');
  const addBtn = getEl('add-tab-btn');

  // Configure mock layout: bar width = 400px, 4 tabs each 120px = 480px total (cannot all fit!)
  tabsBar.clientWidth = 400;
  addBtn.offsetWidth = 30;

  vm.runInContext(`
    tabs = [
      { id: 'tab-1', title: 'Query 1.sql', sql: '', isTableViewer: false },
      { id: 'tab-2', title: 'Query 2.sql', sql: '', isTableViewer: false },
      { id: 'tab-3', title: 'Query 3.sql', sql: '', isTableViewer: false },
      { id: 'tab-4', title: 'Query 4.sql', sql: '', isTableViewer: false }
    ];
    activeTabId = 'tab-1';
  `, sandbox);

  // Setup children in container
  const child1 = domElementStub('t-1'); child1.offsetWidth = 120;
  const child2 = domElementStub('t-2'); child2.offsetWidth = 120;
  const child3 = domElementStub('t-3'); child3.offsetWidth = 120;
  const child4 = domElementStub('t-4'); child4.offsetWidth = 120;
  container.children = [child1, child2, child3, child4];

  vm.runInContext(`updateTabOverflow()`, sandbox);

  assert(!overflowWrapper.classList.contains('hidden'), 'overflowWrapper must be visible when tabs overflow');
  const count = parseInt(overflowCountEl.textContent, 10);
  assert(count > 0, 'overflow count must be > 0 when tabs exceed available width');
  assert(child4.classList.contains('hidden'), '4th tab must be hidden from tab strip');
  console.log(`✅ Test 2 Passed: Overflow detected correctly (count = ${count}), extra tabs hidden`);

  // Test 3: Dropdown menu toggle
  const dd = getEl('tab-overflow-dropdown');
  dd.classList.add('hidden');
  vm.runInContext(`toggleTabOverflowMenu()`, sandbox);
  assert(!dd.classList.contains('hidden'), 'dropdown must be visible after toggle');
  vm.runInContext(`closeTabOverflowMenu()`, sandbox);
  assert(dd.classList.contains('hidden'), 'dropdown must be hidden after close');
  console.log("✅ Test 3 Passed: toggleTabOverflowMenu opens and closes dropdown properly");

  // Test 4: Selecting an overflowed tab swaps it into visible strip so activeTabId is visible
  // Let active tab be tab-4 (which was overflowed)
  vm.runInContext(`selectOverflowTab('tab-4')`, sandbox);
  const newActive = vm.runInContext(`activeTabId`, sandbox);
  assert.strictEqual(newActive, 'tab-4', 'activeTabId must switch to selected overflow tab');
  const updatedTabs = vm.runInContext(`tabs.map(t => t.id)`, sandbox);
  assert(updatedTabs.indexOf('tab-4') < 3, 'tab-4 must be moved into visible slot of tabs array');
  console.log("✅ Test 4 Passed: selectOverflowTab successfully swaps active tab into visible area");

  // Test 5: All tabs fit when bar width is large -> overflow wrapper is hidden
  tabsBar.clientWidth = 1200;
  container.children = [child1, child2, child3, child4];
  vm.runInContext(`updateTabOverflow()`, sandbox);
  assert(overflowWrapper.classList.contains('hidden'), 'overflowWrapper must be hidden when all tabs fit');
  console.log("✅ Test 5 Passed: Overflow wrapper disappears when all tabs fit without overflow");

  // Test 6: addNewQueryTab() on narrow width immediately triggers overflow & shows » button without refresh
  tabsBar.clientWidth = 350;
  vm.runInContext(`
    Object.defineProperty(document.getElementById('tabs-container'), 'innerHTML', {
      set(val) {
        this.children = tabs.map(t => {
          const el = document.createElement('div');
          el.offsetWidth = 120;
          el.dataset = { tabId: t.id };
          return el;
        });
      },
      get() { return ''; }
    });
  `, sandbox);

  vm.runInContext(`addNewQueryTab()`, sandbox);
  assert(!overflowWrapper.classList.contains('hidden'), 'overflowWrapper must be visible immediately on addNewQueryTab without refresh');
  const finalCount = parseInt(overflowCountEl.textContent, 10);
  assert(finalCount > 0, 'overflow count must be > 0 immediately');
  console.log(`✅ Test 6 Passed: addNewQueryTab immediately shows overflow button (count = ${finalCount}) without page refresh`);

  // Test 7: Closing hidden tabs from overflow dropdown properly closes them, and closes dropdown when last is closed
  while (vm.runInContext(`tabs.length`, sandbox) > 2) {
    const hiddenTabs = vm.runInContext(`tabs.slice(-1)`, sandbox);
    const hiddenTabId = hiddenTabs[0].id;
    await vm.runInContext(`closeTabFromOverflow('${hiddenTabId}')`, sandbox);
    const remainingTabs = vm.runInContext(`tabs.map(t => t.id)`, sandbox);
    assert(!remainingTabs.includes(hiddenTabId), 'Closed hidden tab must be removed from tabs array');
  }
  assert(dd.classList.contains('hidden'), 'Dropdown menu must be hidden when all overflow tabs are gone');
  assert.strictEqual(dd.innerHTML, '', 'Dropdown HTML must be cleared when overflow is empty');
  console.log("✅ Test 7 Passed: Closing last hidden tab from overflow menu cleanly removes it and closes dropdown");

  // Test 8: getNextQueryTabTitle() prevents duplicate numbers (Query 12 twice or Query 17 twice)
  vm.runInContext(`
    tabs = Array.from({ length: 12 }, (_, i) => ({
      id: 'tab-' + (i + 1),
      title: 'Query ' + (i + 1) + '.sql',
      sql: '',
      isTableViewer: false
    }));
  `, sandbox);

  // Close tab 5 (leaving 11 tabs, max is still 12)
  vm.runInContext(`tabs = tabs.filter(t => t.id !== 'tab-5');`, sandbox);
  assert.strictEqual(vm.runInContext(`tabs.length`, sandbox), 11);
  const nextTitle1 = vm.runInContext(`getNextQueryTabTitle()`, sandbox);
  assert.strictEqual(nextTitle1, 'Query 13.sql', 'Next query tab must be Query 13.sql and never duplicate Query 12.sql');

  // Push Query 13, and another up to 17
  vm.runInContext(`
    tabs.push({ id: 'tab-13', title: 'Query 13.sql', sql: '', isTableViewer: false });
    tabs.push({ id: 'tab-17', title: 'Query 17.sql', sql: '', isTableViewer: false });
  `, sandbox);
  // Close an earlier tab
  vm.runInContext(`tabs = tabs.filter(t => t.id !== 'tab-2');`, sandbox);
  const nextTitle2 = vm.runInContext(`getNextQueryTabTitle()`, sandbox);
  assert.strictEqual(nextTitle2, 'Query 18.sql', 'Next query tab must be Query 18.sql and never duplicate Query 17.sql');
  console.log("✅ Test 8 Passed: Tab titles never produce duplicate numbers when tabs are closed out of order");

  console.log("\n🎉 ALL 8 TAB OVERFLOW TESTS PASSED (GREEN)!");
}

runTests().catch(err => {
  console.error("Test failed:", err);
  process.exit(1);
});
