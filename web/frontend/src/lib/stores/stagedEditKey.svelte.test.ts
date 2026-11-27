import { describe, it, expect, beforeEach } from 'vitest';
import { appState } from './appState.svelte';

describe('staged edit key contract', () => {
  beforeEach(() => {
    appState.tabs = [
      { id: 'tab-a', title: 'A', sql: '', isTableViewer: true, tableName: 'users', database: 'testdb',
        activeResult: { columns: ['id', 'name'], rows: [[1, 'x']] } as any },
      { id: 'tab-b', title: 'B', sql: '', isTableViewer: true, tableName: 'users', database: 'testdb',
        activeResult: { columns: ['id', 'name'], rows: [[2, 'y']] } as any },
    ];
    appState.activeTabId = 'tab-a';
    appState.activeConnection = { id: 'conn-1', name: 'T', driver: 'sqlite', environment: 'Development' } as any;
    appState.stagedEdits = {};
    appState.gridEditMode = 'staged';
    appState.isReadOnly = false;
  });

  it('grid and inspector lookups resolve the key stageEdit wrote', () => {
    appState.stageEdit('users', 0, 1, 'name', 'id', 1, 'x', 'z', 'testdb');
    const row = appState.activeTab!.activeResult!.rows[0];
    const uid = appState.getRowUid(row);
    expect(appState.stagedEdits[appState.stagedEditKey('users', uid, 1)]).toBeDefined();
  });

  it('stagedCount ignores edits belonging to other tabs', () => {
    appState.stageEdit('users', 0, 1, 'name', 'id', 1, 'x', 'z', 'testdb');
    appState.activeTabId = 'tab-b';
    appState.stageEdit('users', 0, 1, 'name', 'id', 2, 'y', 'w', 'testdb');
    expect(appState.stagedCount).toBe(1);
    appState.activeTabId = 'tab-a';
    expect(appState.stagedCount).toBe(1);
  });
});
