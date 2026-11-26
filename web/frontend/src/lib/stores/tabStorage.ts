import type { EditorTab } from '../../types';
import { storage } from '../utils/storage';

export function resolveTabsStorageKey(connId?: string): string {
  const id = connId || storage.get('sqlfox_last_conn_id') || '';
  return id ? `sqlfox_tabs_${id}` : 'sqlfox_tabs_default';
}

export function persistTabsToStorage(key: string, activeTabId: string, tabs: EditorTab[]): void {
  if (typeof window === 'undefined') return;
  const serializable = {
    activeTabId,
    tabs: tabs.map((t) => ({
      id: t.id,
      title: t.title,
      sql: t.sql,
      database: t.database,
      isTableViewer: Boolean(t.isTableViewer),
      tableName: t.tableName,
      tableMode: t.tableMode,
      pageSize: t.pageSize || 25,
      currentPage: t.currentPage || 1,
      sortCol: t.sortCol || null,
      sortDir: t.sortDir || null,
      filterPills: t.filterPills || [],
      filterMode: t.filterMode || 'pills',
      filterRaw: t.filterRaw || '',
      appliedFilter: t.appliedFilter || '',
      queryPage: t.queryPage || 1,
      queryPageSize: t.queryPageSize || 100,
      showFilterBar: Boolean(t.showFilterBar),
      hiddenColumns: t.hiddenColumns || [],
      isProcesslist: Boolean(t.isProcesslist),
      isCluster: Boolean(t.isCluster),
      isFlightRecorder: Boolean(t.isFlightRecorder),
      isVariables: Boolean(t.isVariables),
      isErdCanvas: Boolean(t.isErdCanvas),
      isNote: Boolean(t.isNote),
      noteMode: t.noteMode || 'edit',
    })),
  };
  storage.setJSON(key, serializable);
}

export function loadTabsFromStorage(
  key: string,
  driverName?: string
): { tabs: EditorTab[]; activeTabId: string } | null {
  if (typeof window === 'undefined') return null;
  const parsed = storage.getJSON<{ tabs?: EditorTab[]; activeTabId?: string } | null>(key, null);
  if (parsed && Array.isArray(parsed.tabs) && parsed.tabs.length > 0) {
    const isSqlite = (driverName || '').toLowerCase() === 'sqlite';
    const filteredTabs: EditorTab[] = isSqlite
      ? parsed.tabs.filter((t: EditorTab) => !t.isProcesslist && !t.isCluster && !t.isVariables)
      : parsed.tabs;
    const tabs: EditorTab[] =
      filteredTabs.length > 0
        ? filteredTabs
        : [{ id: `tab-${Date.now()}`, title: 'Query 1.sql', sql: '', isTableViewer: false }];
    let activeTabId = parsed.activeTabId || parsed.tabs[0].id;
    if (!tabs.some((t) => t.id === activeTabId)) {
      activeTabId = tabs[0].id;
    }
    return { tabs, activeTabId };
  }
  return null;
}
