import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import GridContextMenu from './GridContextMenu.svelte';
import GridColumnPicker from './GridColumnPicker.svelte';
import RecordInspectorModal from '../modals/RecordInspectorModal.svelte';
import { appState } from '../../stores/appState.svelte';
import { compileFilterPills } from '../../utils/filterCompiler';
import { computeVisibleColumnWindow } from './gridHelpers';
import type { FilterPill } from '../../../types';

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

describe('Grid ergonomics behavioral tests', () => {
  beforeEach(() => {
    const tab = {
      id: 'tab-ergo',
      title: 'users',
      sql: 'SELECT * FROM users',
      isTableViewer: true,
      tableName: 'users',
      database: 'app',
      hiddenColumns: [] as string[],
      activeResult: {
        columns: [
          { name: 'id', type: 'int', is_primary_key: true, is_nullable: false },
          { name: 'email', type: 'varchar(255)', is_primary_key: false, is_nullable: true },
          { name: 'created_at', type: 'timestamp', is_primary_key: false, is_nullable: true },
        ],
        rows: [
          [1, 'alice@example.com', '2026-01-01'],
          [2, 'bob@example.com', '2026-01-02'],
        ],
        duration_ms: 5,
        row_count: 2,
      },
    };
    appState.tabs = [tab];
    appState.activeTabId = tab.id;
    appState.currentDatabase = 'app';
    appState.isRecordInspectorOpen = false;
    appState.recordInspectorRowIndex = 0;
  });

  it('GridContextMenu renders filter & insert sections and triggers quick filter operators', async () => {
    const appliedOps: FilterPill['op'][] = [];
    const noop = () => {};
    const { container } = render(GridContextMenu, {
      props: {
        contextMenu: {
          open: true,
          x: 100,
          y: 100,
          rIdx: 0,
          cIdx: 1,
          colName: 'email',
          cellValue: 'alice@example.com',
        },
        result: appState.activeTab!.activeResult!,
        tableMeta: undefined,
        effectiveTable: 'users',
        canEdit: true,
        selectedRowIndices: [0],
        isColNullable: () => true,
        resolveColType: () => 'varchar',
        closeContextMenu: noop,
        applyQuickFilter: (op) => appliedOps.push(op),
        startCellEdit: noop,
        applyEditPreset: noop,
        duplicateMultipleRows: noop,
        duplicateRowAsDraft: noop,
        addDraftRow: noop,
        handleBatchDelete: noop,
        handleDeleteRowAt: noop,
        copySelectedRows: noop,
        copyRowAsTSV: noop,
        copyRowAsJSON: noop,
        copyRowAsMarkdown: noop,
        copyRowAsInsertSQL: noop,
      },
    });

    expect(container.querySelector('#grid-ctx-filter-parent')).toBeTruthy();
    expect(container.querySelector('#grid-ctx-filter-options')).toBeTruthy();
    expect(container.querySelector('#grid-ctx-insert-section')).toBeTruthy();
    expect(container.querySelector('#grid-ctx-insert-options')).toBeTruthy();

    const filterBtns = Array.from(
      container.querySelectorAll('#grid-ctx-filter-options button')
    ) as HTMLButtonElement[];
    expect(filterBtns.length).toBeGreaterThanOrEqual(10);
    for (const btn of filterBtns) {
      await fireEvent.click(btn);
    }
    for (const op of ['=', '!=', '<', '<=', '>', '>=', 'LIKE', 'NOT LIKE', 'IS NULL', 'IS NOT NULL'] as const) {
      expect(appliedOps).toContain(op);
    }
  });

  it('RecordInspectorModal supports bounded paging, field search, and staging edits', async () => {
    const stageSpy = vi.spyOn(appState, 'stageEdit').mockImplementation(() => {});
    appState.isRecordInspectorOpen = true;
    appState.recordInspectorRowIndex = 0;

    const { container } = render(RecordInspectorModal);
    expect(container.querySelector('#record-inspector-dialog')).toBeTruthy();
    expect(container.querySelector('#record-inspector-fields')).toBeTruthy();
    const pager = container.querySelector('#record-inspector-pager');
    expect(pager).toBeTruthy();

    const pagerBtns = Array.from(pager!.querySelectorAll('button')) as HTMLButtonElement[];
    expect(pagerBtns[0].disabled).toBe(true); // First at row 0
    expect(pagerBtns[1].disabled).toBe(true); // Prev at row 0
    expect(pagerBtns[2].disabled).toBe(false); // Next
    expect(pagerBtns[3].disabled).toBe(false); // Last

    await fireEvent.click(pagerBtns[2]);
    expect(appState.recordInspectorRowIndex).toBe(1);
    expect(pagerBtns[2].disabled).toBe(true); // Next at last row

    const searchInput = container.querySelector('#record-inspector-search') as HTMLInputElement;
    expect(searchInput).toBeTruthy();
    await fireEvent.input(searchInput, { target: { value: 'email' } });

    const fieldInputs = Array.from(
      container.querySelectorAll('#record-inspector-fields input[type="text"], #record-inspector-fields textarea')
    ) as (HTMLInputElement | HTMLTextAreaElement)[];
    expect(fieldInputs.length).toBeGreaterThan(0);
    await fireEvent.input(fieldInputs[0], { target: { value: 'bob+new@example.com' } });
    await fireEvent.blur(fieldInputs[0]);
    expect(stageSpy).toHaveBeenCalled();

    stageSpy.mockRestore();
  });

  it('appState column visibility preserves PK on hideAllColumns and toggles/restores columns', () => {
    const allCols = ['id', 'email', 'created_at'];
    appState.hideAllColumns(allCols, 'id');
    expect(appState.activeTab?.hiddenColumns).toEqual(['email', 'created_at']);

    appState.toggleColumnHidden('created_at');
    expect(appState.activeTab?.hiddenColumns).toEqual(['email']);

    appState.showAllColumns();
    expect(appState.activeTab?.hiddenColumns).toEqual([]);
  });

  it('compileFilterPills and computeVisibleColumnWindow behave accurately', () => {
    expect(
      compileFilterPills(
        [
          { col: 'status', op: '=', val: 'active' },
          { col: 'deleted_at', op: 'IS NULL', val: '' },
        ],
        'mysql'
      )
    ).toBe("`status` = 'active' AND `deleted_at` IS NULL");

    expect(
      compileFilterPills([{ col: 'age', op: '>', val: '30' }], 'postgres')
    ).toBe('"age" > 30');

    const win = computeVisibleColumnWindow(['a', 'b', 'c'], { a: 120, b: 120, c: 120 }, 0, 800, 64, 1);
    expect(win.startColIdx).toBe(0);
    expect(win.endColIdx).toBe(3);
  });

  it('GridColumnPicker toggles dropdown, filters columns, and calls appState column actions', async () => {
    const showAllSpy = vi.spyOn(appState, 'showAllColumns').mockImplementation(() => {});
    const hideAllSpy = vi.spyOn(appState, 'hideAllColumns').mockImplementation(() => {});
    const toggleSpy = vi.spyOn(appState, 'toggleColumnHidden').mockImplementation(() => {});

    const allColumns = ['id', 'email', 'created_at'];
    const { container } = render(GridColumnPicker, {
      props: { allColumns, visibleCount: allColumns.length, hiddenCols: [] },
    });

    const btn = container.querySelector('#btn-col-picker') as HTMLButtonElement;
    expect(btn).toBeTruthy();
    expect(container.querySelector('#col-picker-count-badge')?.textContent).toBe('3');
    await fireEvent.click(btn);

    expect(container.querySelector('#col-picker-dropdown')).toBeTruthy();
    const search = container.querySelector('#col-picker-search') as HTMLInputElement;
    await fireEvent.input(search, { target: { value: 'email' } });

    const checkboxes = container.querySelectorAll('#col-picker-list input[type="checkbox"]');
    expect(checkboxes.length).toBe(1);
    await fireEvent.click(checkboxes[0]);
    expect(toggleSpy).toHaveBeenCalledWith('email');

    const buttons = Array.from(container.querySelectorAll('#col-picker-dropdown button')) as HTMLButtonElement[];
    await fireEvent.click(buttons[0]);
    expect(showAllSpy).toHaveBeenCalled();
    await fireEvent.click(buttons[1]);
    expect(hideAllSpy).toHaveBeenCalledWith(allColumns);

    showAllSpy.mockRestore();
    hideAllSpy.mockRestore();
    toggleSpy.mockRestore();
  });

  it('appState.hasOpenModal and closeTopOverlay share consistent modal registry', () => {
    appState.isCommandPaletteOpen = false;
    appState.isSnippetsModalOpen = false;
    appState.isRemoteLocked = false;
    expect(appState.hasOpenModal).toBe(false);

    appState.isCommandPaletteOpen = true;
    expect(appState.hasOpenModal).toBe(true);
    expect(appState.closeTopOverlay()).toBe(true);
    expect(appState.isCommandPaletteOpen).toBe(false);
    expect(appState.hasOpenModal).toBe(false);

    appState.isSnippetsModalOpen = true;
    expect(appState.hasOpenModal).toBe(true);
    expect(appState.closeTopOverlay()).toBe(true);
    expect(appState.isSnippetsModalOpen).toBe(false);
  });
});
