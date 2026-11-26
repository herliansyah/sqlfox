import type { EditorTab, FilterPill } from '../../types';
import { compileFilterPills } from '../utils/filterCompiler';
import { quoteSqlIdentifier } from '../utils/sqlStatements';

export interface TableFilterContext {
  readonly activeTab: EditorTab | undefined;
  readonly tabs: EditorTab[];
  readonly currentDatabase: string;
  readonly driver?: string;
  setTabs(next: EditorTab[]): void;
  saveTabsState(): void;
  loadTableData(tab: EditorTab): Promise<void> | void;
  addTab(title?: string, sql?: string): void;
}

export function setFilterModeAction(ctx: TableFilterContext, mode: 'pills' | 'raw') {
  const tab = ctx.activeTab;
  if (!tab) return;
  tab.filterMode = mode;
  ctx.setTabs([...ctx.tabs]);
  ctx.saveTabsState();
}

export function setFilterRawAction(ctx: TableFilterContext, raw: string) {
  const tab = ctx.activeTab;
  if (!tab) return;
  tab.filterRaw = raw;
  ctx.setTabs([...ctx.tabs]);
}

export function toggleFilterBarAction(ctx: TableFilterContext, tabId?: string) {
  const tab = tabId ? ctx.tabs.find((t) => t.id === tabId) : ctx.activeTab;
  if (!tab) return;
  tab.showFilterBar = !tab.showFilterBar;
  ctx.setTabs([...ctx.tabs]);
  ctx.saveTabsState();
}

export function addFilterPillAction(
  ctx: TableFilterContext,
  col?: string,
  op: FilterPill['op'] = '=',
  val: string = ''
) {
  const tab = ctx.activeTab;
  if (!tab) return;
  if (!tab.filterPills) tab.filterPills = [];
  const defaultCol =
    col ||
    (tab.activeResult?.columns?.[0]
      ? typeof tab.activeResult.columns[0] === 'string'
        ? tab.activeResult.columns[0]
        : tab.activeResult.columns[0].name
      : 'id');
  tab.filterPills.push({ col: defaultCol, op, val });
  tab.filterMode = 'pills';
  tab.showFilterBar = true;
  if (val !== '' || op === 'IS NULL' || op === 'IS NOT NULL') {
    tab.appliedFilter = compileFilterPills(tab.filterPills, ctx.driver);
    if (tab.isTableViewer) {
      tab.currentPage = 1;
      void ctx.loadTableData(tab);
    }
  } else {
    ctx.setTabs([...ctx.tabs]);
  }
  ctx.saveTabsState();
}

export function updateFilterPillAction(
  ctx: TableFilterContext,
  index: number,
  patch: Partial<FilterPill>
) {
  const tab = ctx.activeTab;
  if (!tab || !tab.filterPills || !tab.filterPills[index]) return;
  Object.assign(tab.filterPills[index], patch);
  ctx.setTabs([...ctx.tabs]);
}

export function removeFilterPillAction(ctx: TableFilterContext, index: number) {
  const tab = ctx.activeTab;
  if (!tab || !tab.filterPills) return;
  tab.filterPills.splice(index, 1);
  tab.appliedFilter = compileFilterPills(tab.filterPills, ctx.driver);
  if (tab.isTableViewer) {
    tab.currentPage = 1;
    void ctx.loadTableData(tab);
  }
  ctx.saveTabsState();
}

export function applyTableFilterAction(ctx: TableFilterContext) {
  const tab = ctx.activeTab;
  if (!tab || !tab.isTableViewer) return;
  tab.currentPage = 1;
  if (tab.filterMode === 'raw') {
    let rawVal = (tab.filterRaw || '').trim();
    rawVal = rawVal.replace(/^where\s+/i, '').trim();
    tab.filterRaw = rawVal;
    tab.appliedFilter = rawVal;
  } else {
    tab.appliedFilter = compileFilterPills(tab.filterPills || [], ctx.driver);
  }
  void ctx.loadTableData(tab);
  ctx.saveTabsState();
}

export function clearTableFilterAction(ctx: TableFilterContext) {
  const tab = ctx.activeTab;
  if (!tab) return;
  tab.appliedFilter = '';
  tab.filterRaw = '';
  tab.filterPills = [];
  tab.currentPage = 1;
  if (tab.isTableViewer) {
    void ctx.loadTableData(tab);
  }
  ctx.saveTabsState();
}

export function handoffTableQueryToEditorAction(ctx: TableFilterContext) {
  const tab = ctx.activeTab;
  if (!tab || !tab.isTableViewer || !tab.tableName) return;
  const targetTable = quoteSqlIdentifier(tab.tableName, ctx.driver);
  let sql = `SELECT * FROM ${targetTable}`;
  const filter = (tab.appliedFilter || '').trim();
  if (filter) {
    sql += `\nWHERE ${filter}`;
  }
  if (tab.sortCol) {
    const escSortCol = quoteSqlIdentifier(tab.sortCol, ctx.driver);
    sql += `\nORDER BY ${escSortCol} ${tab.sortDir || 'ASC'}`;
  }
  sql += `\nLIMIT 50;`;
  ctx.addTab(`query_${tab.tableName}.sql`, sql);
}

export function persistHiddenColumns(tab: EditorTab, fallbackDb: string) {
  if (typeof sessionStorage === 'undefined' || !tab.tableName) return;
  const db = tab.database || fallbackDb || '';
  try {
    sessionStorage.setItem(
      `sqlfox-hidden-cols:${db}:${tab.tableName}`,
      JSON.stringify(tab.hiddenColumns || [])
    );
  } catch {
    /* ignore */
  }
}
