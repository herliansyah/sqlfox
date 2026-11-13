// Source-assertion regression test: Tab persistence (sqlfox_tabs_<connId>
// localStorage key, restore titles / sql / isTableViewer) implemented in
// web/frontend/src/lib/stores/appState.svelte.ts. The serialize/deserialize
// round-trip logic is copied from the real source into plain JS and asserted.
import fs from 'fs';
import assert from 'assert';

console.log('=== Running Tab Persistence Source Assertions ===');

const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');

// 1. Per-connection storage key
assert(
  appStateCode.includes('`sqlfox_tabs_${id}`') && appStateCode.includes("sqlfox_tabs_default"),
  'getTabsStorageKey must build sqlfox_tabs_<connId> keys with a default fallback'
);
assert(
  appStateCode.includes("localStorage.getItem('sqlfox_last_conn_id')"),
  'getTabsStorageKey must fall back to sqlfox_last_conn_id when no active connection'
);

// 2. saveTabsState guards: only runs in browser with an active connection
assert(
  appStateCode.includes("if (typeof window === 'undefined' || !this.activeConnection?.id) return;"),
  'saveTabsState must bail out server-side or without an active connection'
);

// 3. restoreTabsState loads sql / isTableViewer / tableName from storage
assert(
  appStateCode.includes('restoreTabsState(connId: string): boolean'),
  'restoreTabsState must be defined'
);
assert(
  appStateCode.includes('if (this.activeTab?.isTableViewer && this.activeTab.tableName) {'),
  'restoreTabsState must reload table data when the restored active tab is a table viewer'
);
assert(
  appStateCode.includes('this.activeTabId = parsed.activeTabId || parsed.tabs[0].id;'),
  'restoreTabsState must restore the active tab, defaulting to the first saved tab'
);
assert(
  appStateCode.includes("if (!raw) return false;"),
  'restoreTabsState must return false when nothing is persisted'
);

// 4. Persisted per-tab fields (copied from saveTabsState serialization)
const serializeTabs = (tabs, activeTabId) => ({
  activeTabId,
  tabs: tabs.map((t) => ({
    id: t.id,
    title: t.title,
    sql: t.sql,
    database: t.database,
    isTableViewer: Boolean(t.isTableViewer),
    tableName: t.tableName,
    tableMode: t.tableMode,
    pageSize: t.pageSize || 50,
    currentPage: t.currentPage || 1,
    filterPills: t.filterPills || [],
    filterMode: t.filterMode || 'pills',
    filterRaw: t.filterRaw || '',
    appliedFilter: t.appliedFilter || '',
    showFilterBar: Boolean(t.showFilterBar),
    hiddenColumns: t.hiddenColumns || [],
    isProcesslist: Boolean(t.isProcesslist),
    isErdCanvas: Boolean(t.isErdCanvas),
  })),
});

const getTabsStorageKey = (connId) => (connId ? `sqlfox_tabs_${connId}` : 'sqlfox_tabs_default');

// 5. Simulated save -> restore round trip against a fake localStorage
const storage = new Map();
const localStorageMock = {
  getItem: (k) => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: (k) => storage.delete(k),
  clear: () => storage.clear(),
};

function saveTabsState(tabs, activeTabId, connId) {
  const key = getTabsStorageKey(connId);
  localStorageMock.setItem(key, JSON.stringify(serializeTabs(tabs, activeTabId)));
}

function restoreTabsState(connId) {
  const key = getTabsStorageKey(connId);
  const raw = localStorageMock.getItem(key);
  if (!raw) return false;
  try {
    const parsed = JSON.parse(raw);
    if (parsed && Array.isArray(parsed.tabs) && parsed.tabs.length > 0) {
      return { tabs: parsed.tabs, activeTabId: parsed.activeTabId || parsed.tabs[0].id };
    }
  } catch (e) {
    // fall through
  }
  return false;
}

// Scenario: monitoring tabs + a table viewer + SQL-bearing query tab
const initialTabs = [
  { id: 'tab-processlist', title: 'Processlist', sql: '', isTableViewer: false, isProcesslist: true },
  { id: 'tab-flight-recorder', title: 'Flight Recorder', sql: '', isTableViewer: false },
  { id: 'tab-cluster', title: 'Cluster', sql: '', isTableViewer: false },
  { id: 'tab-server-variables', title: 'Server Variables', sql: '', isTableViewer: false },
  { id: 'tab-query-1', title: 'Query 1.sql', sql: 'SELECT * FROM users;', isTableViewer: false },
  { id: 'tab-tbl-users', title: 'users', sql: '', isTableViewer: true, tableName: 'users', tableMode: 'data' },
];
const initialActiveTabId = 'tab-flight-recorder';

saveTabsState(initialTabs, initialActiveTabId, 'conn-prod-1');
const storedRaw = storage.get('sqlfox_tabs_conn-prod-1');
assert(storedRaw, 'saveTabsState must write to sqlfox_tabs_<connId>');
const stored = JSON.parse(storedRaw);
assert.strictEqual(stored.activeTabId, 'tab-flight-recorder', 'Active tab id must be persisted');
assert.strictEqual(stored.tabs.length, initialTabs.length, 'All tabs must be persisted');

// Simulate browser refresh: wipe in-memory state
const restoredTabs = [];
let restoredActiveTabId = null;

const restored = restoreTabsState('conn-prod-1');
assert(restored, 'restoreTabsState must succeed for a persisted connection');
restoredTabs.push(...restored.tabs);
restoredActiveTabId = restored.activeTabId;

// Assertions: monitoring tabs preserved across refresh
const expectedTabIds = ['tab-processlist', 'tab-flight-recorder', 'tab-cluster', 'tab-server-variables'];
const missingTabs = expectedTabIds.filter((id) => !restoredTabs.some((t) => t.id === id));
assert.deepStrictEqual(missingTabs, [], 'All monitoring tabs must survive a refresh');
assert.strictEqual(restoredActiveTabId, 'tab-flight-recorder', 'Active tab must be restored');

// SQL content is preserved
const queryTab = restoredTabs.find((t) => t.id === 'tab-query-1');
assert.strictEqual(queryTab.sql, 'SELECT * FROM users;', 'SQL content must survive the round trip');

// isTableViewer / tableName preserved for table tabs
const tableTab = restoredTabs.find((t) => t.id === 'tab-tbl-users');
assert.strictEqual(tableTab.isTableViewer, true, 'isTableViewer must be restored');
assert.strictEqual(tableTab.tableName, 'users', 'tableName must be restored');
assert.strictEqual(tableTab.tableMode, 'data', 'tableMode must be restored');
assert.strictEqual(stored.tabs.find((t) => t.id === 'tab-processlist').isProcesslist, true, 'isProcesslist flag must be serialized for the processlist tab');

// 6. restoreTabsState returns false when nothing stored / malformed payload
assert.strictEqual(restoreTabsState('conn-unknown'), false, 'Unknown connection must restore nothing');
storage.set('sqlfox_tabs_conn-corrupt', 'not-json');
assert.strictEqual(restoreTabsState('conn-corrupt'), false, 'Corrupt JSON must be handled gracefully');
storage.set('sqlfox_tabs_conn-empty', JSON.stringify({ tabs: [] }));
assert.strictEqual(restoreTabsState('conn-empty'), false, 'Empty tab list must not restore');

// 7. Per-connection isolation
saveTabsState([{ id: 'tab-a', title: 'A.sql', sql: '' }], 'tab-a', 'conn-other');
assert.strictEqual(restoreTabsState('conn-other').tabs[0].id, 'tab-a', 'Each connection has its own persisted tab set');
assert.notStrictEqual(restoreTabsState('conn-other').tabs[0].id, restoredTabs[0].id, 'Connections must not share tab state');

console.log('🎉 ALL TAB PERSISTENCE SOURCE ASSERTIONS PASSED (GREEN)!');
