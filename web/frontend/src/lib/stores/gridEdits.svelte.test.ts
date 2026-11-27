import { describe, it, expect, vi } from 'vitest';
import { commitStagedEditsAction, type GridEditsContext } from './gridEdits';
import { api } from '../api/client';
import type { EditorTab, StagedEdit } from '../../types';

describe('commitStagedEditsAction tab scoping', () => {
  it('commits only staged edits belonging to the active tab even when another tab views the same table', async () => {
    const cellEditSpy = vi.spyOn(api, 'cellEdit').mockResolvedValue({ rows_affected: 1 } as any);
    const tabA: EditorTab = { id: 'tab-a', title: 'users A', sql: '', isTableViewer: true, tableName: 'users', database: 'app' };
    const stagedEdits: Record<string, StagedEdit> = {
      'tab-a:users:1:1': { tabId: 'tab-a', rIdx: 0, cIdx: 1, table: 'users', database: 'app', colName: 'name', pkCol: 'id', pkVal: 1, oldVal: 'a', newVal: 'b' },
      'tab-b:users:2:1': { tabId: 'tab-b', rIdx: 0, cIdx: 1, table: 'users', database: 'app', colName: 'name', pkCol: 'id', pkVal: 2, oldVal: 'c', newVal: 'd' },
    };
    const ctx: GridEditsContext = {
      activeSessionId: 'sess-1',
      isReadOnly: false,
      currentDatabase: 'app',
      activeTab: tabA,
      tabs: [tabA],
      stagedEdits,
      gridEditMode: 'staged',
      gridConfirmSave: false,
      auditRefreshToken: 0,
      isInsertRowModalOpen: false,
      setTabs: () => {},
      setStagedEdits: () => {},
      showToast: () => 't1',
      showNativeConfirm: async () => true,
      loadTableData: async () => {},
      executeCurrentTab: async () => {},
    };

    await commitStagedEditsAction(ctx);

    expect(cellEditSpy).toHaveBeenCalledTimes(1);
    expect(cellEditSpy.mock.calls[0][1].edits).toHaveLength(1);
    expect(cellEditSpy.mock.calls[0][1].edits[0].pk_value).toBe(1);
    expect(stagedEdits['tab-a:users:1:1']).toBeUndefined();
    expect(stagedEdits['tab-b:users:2:1']).toBeDefined();
    cellEditSpy.mockRestore();
  });

  it('commits an explicit target tab and leaves the active tab alone', async () => {
    const cellEditSpy = vi.spyOn(api, 'cellEdit').mockResolvedValue({ rows_affected: 1 } as any);
    const tabA: EditorTab = { id: 'tab-a', title: 'users A', sql: 'SELECT 1', isTableViewer: false };
    const tabB: EditorTab = { id: 'tab-b', title: 'users B', sql: 'SELECT 2', isTableViewer: false };
    const stagedEdits: Record<string, StagedEdit> = {
      'tab-a:users:1:1': { tabId: 'tab-a', rIdx: 0, cIdx: 1, table: 'users', database: 'app', colName: 'name', pkCol: 'id', pkVal: 1, oldVal: 'a', newVal: 'b' },
    };
    const executeCurrentTab = vi.fn(async () => {});
    const ctx: GridEditsContext = {
      activeSessionId: 'sess-1',
      isReadOnly: false,
      currentDatabase: 'app',
      activeTab: tabB,
      tabs: [tabA, tabB],
      stagedEdits,
      gridEditMode: 'auto',
      gridConfirmSave: true,
      auditRefreshToken: 0,
      isInsertRowModalOpen: false,
      setTabs: () => {},
      setStagedEdits: () => {},
      showToast: () => 't1',
      showNativeConfirm: async () => true,
      loadTableData: async () => {},
      executeCurrentTab,
    };

    await commitStagedEditsAction(ctx, tabA);

    expect(cellEditSpy).toHaveBeenCalledTimes(1);
    expect(stagedEdits['tab-a:users:1:1']).toBeUndefined();
    expect(executeCurrentTab).not.toHaveBeenCalled();
    cellEditSpy.mockRestore();
  });
});
