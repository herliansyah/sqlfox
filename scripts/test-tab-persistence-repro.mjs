import fs from 'fs';
import vm from 'vm';
import path from 'path';

const html = fs.readFileSync('web/dist/index.html', 'utf8');

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

const canvasCtxStub = {
  fillRect: () => {},
  strokeRect: () => {},
  stroke: () => {},
  beginPath: () => {},
  closePath: () => {},
  moveTo: () => {},
  lineTo: () => {},
  arc: () => {},
  fill: () => {},
  clearRect: () => {},
  measureText: () => ({ width: 0 }),
  fillText: () => {},
  strokeText: () => {},
  scale: () => {},
  save: () => {},
  restore: () => {},
  setLineDash: () => {},
  canvas: { width: 800, height: 200 }
};

const domElementStub = () => ({
  classList: { add: () => {}, remove: () => {}, contains: () => false, toggle: () => {} },
  dataset: {},
  innerHTML: '',
  textContent: '',
  value: '',
  style: {},
  width: 800,
  height: 200,
  addEventListener: () => {},
  removeEventListener: () => {},
  focus: () => {},
  blur: () => {},
  scrollIntoView: () => {},
  getContext: () => canvasCtxStub,
  getBoundingClientRect: () => ({ left: 0, top: 0, width: 800, height: 200 })
});

const sandbox = {
  console,
  devicePixelRatio: 1,
  fetch: async () => ({ ok: true, json: async () => ({}) }),
  setTimeout: (fn) => {},
  clearTimeout: () => {},
  setInterval: () => {},
  clearInterval: () => {},
  localStorage: localStorageMock,
  sessionStorage: localStorageMock,
  window: {
    devicePixelRatio: 1,
    addEventListener: () => {},
    removeEventListener: () => {},
    location: { reload: () => {} }
  },
  document: {
    getElementById: () => domElementStub(),
    querySelectorAll: () => [],
    querySelector: () => domElementStub(),
    createElement: () => domElementStub(),
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
  console.error("Script execution error:", err.message);
  process.exit(2);
}

// 4. Test Scenario: Active connection with Monitoring Tabs opened
// Set lexical variables inside the VM context
vm.runInContext(`
  activeConnection = { id: 'conn-prod-1', name: 'Production DB', driver: 'mysql' };
  tabs = [
    { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT 1;', isTableViewer: false },
    { id: 'tab-processlist', title: 'Processlist', isProcesslist: true },
    { id: 'tab-flight-recorder', title: 'Flight Recorder', isFlightRecorder: true },
    { id: 'tab-cluster', title: 'Cluster Health', isCluster: true },
    { id: 'tab-server-variables', title: 'Server Variables', isVariables: true }
  ];
  activeTabId = 'tab-flight-recorder';
`, sandbox);

const initialTabs = vm.runInContext(`tabs.map(t => ({ id: t.id, title: t.title }))`, sandbox);
const initialActiveTab = vm.runInContext(`activeTabId`, sandbox);

console.log("=== Initial State Before Refresh ===");
console.log("Tabs:", initialTabs);
console.log("Active Tab ID:", initialActiveTab);

// 5. Trigger saveTabsState()
vm.runInContext(`saveTabsState()`, sandbox);

// Check what was stored in localStorage
const storedRaw = storage.get('sqlfox_tabs_conn-prod-1');
console.log("\n=== Stored in localStorage ===");
console.log(storedRaw);

// 6. Simulate Browser Refresh (F5): Reset in-memory tabs & activeTabId
vm.runInContext(`
  tabs = [];
  activeTabId = null;
`, sandbox);

// 7. Trigger restoreTabsState()
vm.runInContext(`restoreTabsState('conn-prod-1')`, sandbox);

const restoredTabs = vm.runInContext(`tabs.map(t => ({ id: t.id, title: t.title }))`, sandbox);
const restoredActiveTab = vm.runInContext(`activeTabId`, sandbox);

console.log("\n=== State After Browser Refresh ===");
console.log("Restored Tabs:", restoredTabs);
console.log("Restored Active Tab ID:", restoredActiveTab);

// 8. Assertions: Monitoring tabs MUST be preserved across refresh
const expectedTabIds = ['tab-processlist', 'tab-flight-recorder', 'tab-cluster', 'tab-server-variables'];
const missingTabs = expectedTabIds.filter(id => !restoredTabs.some(t => t.id === id));

if (missingTabs.length > 0 || restoredActiveTab !== 'tab-flight-recorder') {
  console.error(`\n❌ TEST FAILED: Monitoring tabs lost upon refresh!`);
  console.error(`Missing tabs: [${missingTabs.join(', ')}]`);
  console.error(`Expected activeTabId: tab-flight-recorder, got: ${restoredActiveTab}`);
  process.exit(1);
} else {
  console.log("\n✅ TEST PASSED (GREEN): All monitoring tabs and active tab persisted across refresh.");
  process.exit(0);
}
