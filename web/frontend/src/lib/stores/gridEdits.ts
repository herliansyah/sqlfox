import type { EditorTab, StagedEdit, Toast } from '../../types';
import { api } from '../api/client';

export interface GridEditsContext {
  readonly activeSessionId: string | null;
  readonly isReadOnly: boolean;
  readonly currentDatabase: string;
  readonly activeTab: EditorTab | undefined;
  readonly tabs: EditorTab[];
  stagedEdits: Record<string, StagedEdit>;
  gridEditMode: 'staged' | 'auto';
  gridConfirmSave: boolean;
  auditRefreshToken: number;
  isInsertRowModalOpen: boolean;
  setTabs(next: EditorTab[]): void;
  setStagedEdits(next: Record<string, StagedEdit>): void;
  showToast(msg: string, type?: Toast['type']): string;
  showNativeConfirm(message: string, title?: string): Promise<boolean>;
  loadTableData(tab: EditorTab): Promise<void>;
  executeCurrentTab(): Promise<void>;
}

export async function commitStagedEditsAction(ctx: GridEditsContext): Promise<void> {
  if (!ctx.activeSessionId) return;
  if (ctx.isReadOnly) {
    ctx.showToast('Read-only mode is active — writes blocked', 'warning');
    return;
  }
  const committedEntries = Object.entries(ctx.stagedEdits);
  const tab = ctx.activeTab;
  const draftRows = tab?.stagedDraftRows || [];
  if (committedEntries.length === 0 && draftRows.length === 0) return;

  const slowWarning =
    committedEntries.length + draftRows.length >= 20
      ? setTimeout(() => ctx.showToast('Applying batch changes...', 'info'), 1500)
      : null;

  const groups = new Map<
    string,
    { table: string; database?: string; entries: Array<[string, StagedEdit]> }
  >();
  for (const entry of committedEntries) {
    const item = entry[1];
    const targetDb = item.database || ctx.currentDatabase;
    const key = `${targetDb || ''}::${item.table}`;
    let group = groups.get(key);
    if (!group) {
      group = { table: item.table, database: targetDb, entries: [] };
      groups.set(key, group);
    }
    group.entries.push(entry);
  }

  try {
    let totalAffected = 0;
    for (const group of groups.values()) {
      const edits = group.entries.map(([, item]) => ({
        database: item.database || group.database,
        table: item.table,
        column: item.colName,
        value: item.newVal,
        pk_column: item.pkCol,
        pk_value: item.pkVal,
        pk_values: item.pkValues,
        row_criteria: item.rowCriteria,
      }));
      const res = await api.cellEdit(ctx.activeSessionId, {
        database: group.database,
        table: group.table,
        edits,
      });
      totalAffected += res.rows_affected ?? group.entries.length;
      for (const [k] of group.entries) {
        delete ctx.stagedEdits[k];
      }
    }

    const draftTable = tab?.tableName || tab?.detectedTableName;
    if (draftRows.length > 0 && draftTable) {
      const remaining: Record<string, any>[] = [];
      for (let i = 0; i < draftRows.length; i++) {
        const d = draftRows[i];
        const cleanPayload: Record<string, any> = {};
        for (const [k, v] of Object.entries(d)) {
          if (!k.startsWith('__') && v !== '' && v !== undefined) {
            cleanPayload[k] = v === '__NULL__' ? null : v;
          }
        }
        try {
          await api.insertRow(ctx.activeSessionId, {
            database: tab?.detectedTableDatabase || tab?.database || ctx.currentDatabase,
            table: draftTable,
            data: cleanPayload,
          });
          totalAffected++;
        } catch (draftErr: any) {
          remaining.push(d);
          ctx.showToast(`Draft row insert failed: ${draftErr.message}`, 'error');
        }
      }
      if (tab) tab.stagedDraftRows = remaining;
    }

    ctx.showToast(`Saved ${totalAffected} change(s) successfully`, 'success');
    ctx.auditRefreshToken++;

    if (ctx.activeTab?.isTableViewer) {
      await ctx.loadTableData(ctx.activeTab);
    } else {
      await ctx.executeCurrentTab();
    }
  } catch (err: any) {
    ctx.showToast(`Failed to commit edits: ${err.message}`, 'error');
  } finally {
    if (slowWarning) clearTimeout(slowWarning);
  }
}

export async function insertRowAction(
  ctx: GridEditsContext,
  data: Record<string, any>
): Promise<boolean> {
  const tab = ctx.activeTab;
  const targetTable = tab?.tableName || tab?.detectedTableName;
  if (!ctx.activeSessionId || !targetTable) return false;
  if (ctx.isReadOnly) {
    ctx.showToast('Read-only mode is active — insert blocked', 'warning');
    return false;
  }

  try {
    await api.insertRow(ctx.activeSessionId, {
      database: tab?.detectedTableDatabase || tab?.database || ctx.currentDatabase,
      table: targetTable,
      data,
    });
    ctx.showToast(`Row inserted into ${targetTable}`, 'success');
    ctx.isInsertRowModalOpen = false;
    ctx.auditRefreshToken++;
    if (tab?.isTableViewer) await ctx.loadTableData(tab);
    else await ctx.executeCurrentTab();
    return true;
  } catch (err: any) {
    ctx.showToast(`Failed to insert row: ${err.message}`, 'error');
    return false;
  }
}

export async function deleteRowsAction(
  ctx: GridEditsContext,
  payload: {
    pk_column?: string;
    pk_values?: any[];
    row_criterias?: Record<string, any>[];
    pk_rows?: Record<string, any>[];
  },
  rowCount: number
): Promise<void> {
  const tab = ctx.activeTab;
  const targetTable = tab?.tableName || tab?.detectedTableName;
  if (!ctx.activeSessionId || !targetTable || rowCount === 0) return;
  if (ctx.isReadOnly) {
    ctx.showToast('Read-only mode is active — delete blocked', 'warning');
    return;
  }

  try {
    await api.deleteRows(ctx.activeSessionId, {
      database: tab?.detectedTableDatabase || tab?.database || ctx.currentDatabase,
      table: targetTable,
      ...payload,
    });
    ctx.showToast(`Deleted ${rowCount} row(s)`, 'success');
    ctx.auditRefreshToken++;
    if (tab?.isTableViewer) await ctx.loadTableData(tab);
    else await ctx.executeCurrentTab();
  } catch (err: any) {
    ctx.showToast(`Failed to delete row(s): ${err.message}`, 'error');
  }
}
