import test from 'node:test';
import assert from 'node:assert/strict';
import {
  addFilterPillAction,
  applyTableFilterAction,
  clearTableFilterAction,
  removeFilterPillAction,
  handoffTableQueryToEditorAction,
  type TableFilterContext,
} from './tableFilters.ts';
import type { EditorTab } from '../../types/index.ts';

function makeCtx(tabOverrides: Partial<EditorTab> = {}) {
  const tab: EditorTab = {
    id: 't1',
    title: 'users',
    sql: '',
    isTableViewer: true,
    tableName: 'users',
    activeResult: {
      columns: [
        { name: 'id', type: 'int', is_nullable: false, is_primary_key: true },
        { name: 'status', type: 'varchar', is_nullable: true, is_primary_key: false },
      ],
      rows: [],
      duration_ms: 0,
      row_count: 0,
    },
    ...tabOverrides,
  };
  let loadCalls = 0;
  let saveCalls = 0;
  const addedTabs: Array<{ title?: string; sql?: string }> = [];
  let tabs = [tab];
  const ctx: TableFilterContext = {
    get activeTab() { return tabs[0]; },
    get tabs() { return tabs; },
    currentDatabase: 'app_db',
    driver: 'postgres',
    setTabs(next) { tabs = next; },
    saveTabsState() { saveCalls++; },
    loadTableData() { loadCalls++; },
    addTab(title, sql) { addedTabs.push({ title, sql }); },
  };
  return { ctx, tab, getLoadCalls: () => loadCalls, getSaveCalls: () => saveCalls, addedTabs };
}

test('addFilterPillAction compiles filter and triggers table reload when value is present', () => {
  const { ctx, tab, getLoadCalls, getSaveCalls } = makeCtx();
  addFilterPillAction(ctx, 'status', '=', "active' OR '1'='1");

  assert.equal(tab.filterMode, 'pills');
  assert.equal(tab.showFilterBar, true);
  assert.equal(tab.appliedFilter, `"status" = 'active'' OR ''1''=''1'`);
  assert.equal(getLoadCalls(), 1);
  assert.equal(getSaveCalls(), 1);
});

test('removeFilterPillAction updates appliedFilter and reloads table', () => {
  const { ctx, tab, getLoadCalls } = makeCtx({
    filterPills: [
      { col: 'id', op: '>', val: '10' },
      { col: 'status', op: '=', val: 'active' },
    ],
  });
  removeFilterPillAction(ctx, 0);
  assert.equal(tab.filterPills?.length, 1);
  assert.equal(tab.appliedFilter, `"status" = 'active'`);
  assert.equal(getLoadCalls(), 1);
});

test('applyTableFilterAction strips leading WHERE in raw mode and handoff builds valid SQL', () => {
  const { ctx, tab, addedTabs } = makeCtx({
    filterMode: 'raw',
    filterRaw: 'WHERE id > 5',
    sortCol: 'id',
    sortDir: 'DESC',
  });
  applyTableFilterAction(ctx);
  assert.equal(tab.filterRaw, 'id > 5');
  assert.equal(tab.appliedFilter, 'id > 5');

  handoffTableQueryToEditorAction(ctx);
  assert.equal(addedTabs.length, 1);
  assert.match(addedTabs[0].sql || '', /SELECT \* FROM "users"\nWHERE id > 5\nORDER BY "id" DESC\nLIMIT 50;/);

  clearTableFilterAction(ctx);
  assert.equal(tab.appliedFilter, '');
  assert.equal(tab.filterRaw, '');
  assert.deepEqual(tab.filterPills, []);
});
