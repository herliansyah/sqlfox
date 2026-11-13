<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import type { ColumnDefinition, FilterPill } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import QueryProfilerPanel from '../editor/QueryProfilerPanel.svelte';

  const ROW_HEIGHT = 28;
  const OVERSCAN = 15;

  let containerRef: HTMLDivElement | null = $state(null);
  let scrollTop = $state(0);
  let containerHeight = $state(500);

  let currentTab = $derived(appState.activeTab);
  let result = $derived(appState.activeTab?.activeResult);
  let isExecuting = $derived(Boolean(appState.activeTab?.isExecuting));

  // Sorting
  let sortCol = $state<string | null>(null);
  let sortDir = $state<'ASC' | 'DESC'>('ASC');

  // Selected row / cell & Context Menu
  let selectedRowIndex = $state<number | null>(null);
  let selectedCell = $state<{ rIdx: number; cIdx: number } | null>(null);
  let editingCell = $state<{ rIdx: number; cIdx: number; tempValue: string } | null>(null);

  // Multi-row selection
  let selectedRowIndices = $state<number[]>([]);

  // Context Menu State
  let contextMenu = $state<{
    open: boolean;
    x: number;
    y: number;
    rIdx: number;
    cIdx: number;
    colName: string;
    cellValue: any;
  } | null>(null);

  // Column Picker Dropdown State
  let showColPicker = $state(false);
  let colPickerSearch = $state('');

  // Export Dropdown State
  let showExportMenu = $state(false);

  // Cell Detail Drawer
  let showDetailDrawer = $state(false);

  // Filter Bar Toggle State
  let isFilterOpen = $derived(Boolean(currentTab?.showFilterBar));
  let hasActiveFilter = $derived(
    Boolean(
      currentTab?.appliedFilter ||
      (currentTab?.filterPills && currentTab.filterPills.length > 0) ||
      currentTab?.filterRaw
    )
  );

  function toggleFilterBar() {
    appState.toggleFilterBar();
  }

  function getColName(col: any): string {
    if (!col) return '';
    return typeof col === 'string' ? col : col.name || '';
  }

  function getColType(col: any): string {
    if (!col || typeof col !== 'object') return '';
    return col.type || '';
  }

  function isColPK(col: any): boolean {
    return typeof col === 'object' ? Boolean(col.is_primary_key) : false;
  }

  let allColumns = $derived<string[]>(
    result?.columns ? result.columns.map((c) => getColName(c)) : []
  );

  let hiddenCols = $derived<string[]>(currentTab?.hiddenColumns || []);

  let visibleColumns = $derived.by(() => {
    if (!result?.columns) return [];
    return result.columns.filter((c) => !hiddenCols.includes(getColName(c)));
  });

  // ponytail: Precomputed Map lookups replace O(N) array scans during virtual window cell rendering
  let colIndexMap = $derived.by(() => {
    const map = new Map<string, number>();
    allColumns.forEach((c, idx) => map.set(c, idx));
    return map;
  });

  let fkMap = $derived.by(() => {
    const map = new Map<string, any>();
    const table = appState.catalog?.tables?.find(
      (t) => t.name.toLowerCase() === (currentTab?.tableName || '').toLowerCase()
    );
    for (const fk of table?.foreign_keys || []) {
      const col = (fk as any).column || (fk as any).column_name;
      if (col) map.set(col, fk);
    }
    return map;
  });

  let processedRows = $derived.by(() => {
    if (!result?.rows) return [];
    let list = result.rows.map((r, idx) => ({ origIdx: idx, data: r }));

    // Apply sorting if active
    if (sortCol !== null && result.columns) {
      const cIdx = allColumns.indexOf(sortCol);
      if (cIdx !== -1) {
        list = [...list].sort((a, b) => {
          const valA = a.data[cIdx];
          const valB = b.data[cIdx];
          if (valA === valB) return 0;
          if (valA === null || valA === undefined) return 1;
          if (valB === null || valB === undefined) return -1;
          if (typeof valA === 'number' && typeof valB === 'number') {
            return sortDir === 'ASC' ? valA - valB : valB - valA;
          }
          return sortDir === 'ASC'
            ? String(valA).localeCompare(String(valB))
            : String(valB).localeCompare(String(valA));
        });
      }
    }
    return list;
  });

  let totalRows = $derived(processedRows.length);
  let totalCols = $derived(visibleColumns.length + 1);

  let startIndex = $derived(Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN));
  let endIndex = $derived(Math.min(totalRows, Math.ceil((scrollTop + containerHeight) / ROW_HEIGHT) + OVERSCAN));

  let topSpacerHeight = $derived(startIndex * ROW_HEIGHT);
  let bottomSpacerHeight = $derived(Math.max(0, (totalRows - endIndex) * ROW_HEIGHT));

  let visibleRows = $derived.by(() => {
    return processedRows.slice(startIndex, endIndex).map((item, idx) => ({
      displayIdx: startIndex + idx,
      origIdx: item.origIdx,
      data: item.data,
    }));
  });

  function onScroll(e: Event) {
    const target = e.currentTarget as HTMLDivElement;
    scrollTop = target.scrollTop;
    containerHeight = target.clientHeight || 500;
  }

  function handleHeaderSort(colName: string) {
    if (sortCol === colName) {
      if (sortDir === 'ASC') {
        sortDir = 'DESC';
      } else {
        sortCol = null;
        sortDir = 'ASC';
      }
    } else {
      sortCol = colName;
      sortDir = 'ASC';
    }
  }

  function formatCellValue(val: any): { display: string; type: string } {
    if (val === null || val === undefined) {
      return { display: 'NULL', type: 'null' };
    }
    if (typeof val === 'boolean') {
      return { display: val ? 'TRUE' : 'FALSE', type: 'boolean' };
    }
    if (typeof val === 'number') {
      return { display: val.toLocaleString(), type: 'number' };
    }
    if (typeof val === 'object') {
      try {
        return { display: JSON.stringify(val), type: 'json' };
      } catch {
        return { display: String(val), type: 'text' };
      }
    }
    return { display: String(val), type: 'text' };
  }

  function getCellClass(type: string) {
    switch (type) {
      case 'null':
        return 'text-rose-400/80 italic font-medium';
      case 'number':
        return 'text-amber-300 font-mono text-right';
      case 'boolean':
        return 'text-emerald-400 font-semibold';
      case 'json':
        return 'text-purple-300 font-mono';
      default:
        return 'text-slate-200 font-mono';
    }
  }

  let isAllSelected = $derived(
    processedRows.length > 0 && selectedRowIndices.length === processedRows.length
  );

  function toggleSelectAll() {
    if (isAllSelected) {
      selectedRowIndices = [];
    } else {
      selectedRowIndices = processedRows.map((r) => r.origIdx);
    }
  }

  function toggleRowSelection(origIdx: number) {
    const idx = selectedRowIndices.indexOf(origIdx);
    if (idx === -1) {
      selectedRowIndices.push(origIdx);
    } else {
      selectedRowIndices.splice(idx, 1);
    }
  }

  async function handleBatchDelete() {
    if (selectedRowIndices.length === 0 || !currentTab?.tableName || !result?.rows) return;
    const pkColObj = result.columns.find((c) => isColPK(c));
    const pkCol = pkColObj ? getColName(pkColObj) : undefined;
    if (!pkCol) {
      appState.showToast('Batch delete requires a table with a Primary Key', 'error');
      return;
    }
    const pkIdx = allColumns.indexOf(pkCol);
    const pkVals = selectedRowIndices.map((idx) => result!.rows[idx][pkIdx]);
    const ok = await appState.showNativeConfirm(
      `Are you sure you want to delete ${selectedRowIndices.length} selected row(s)?`,
      'Batch Deletion'
    );
    if (!ok) return;

    await appState.deleteRows(pkCol, pkVals);
    selectedRowIndices = [];
  }

  function addDraftRow() {
    if (!currentTab) return;
    if (!currentTab.stagedDraftRows) currentTab.stagedDraftRows = [];
    const blank: Record<string, any> = {};
    allColumns.forEach((c) => {
      blank[c] = '';
    });
    currentTab.stagedDraftRows.unshift(blank);
    appState.showToast('Draft row added. Enter values to save.', 'info');
  }

  function removeDraftRow(dIdx: number) {
    if (!currentTab?.stagedDraftRows) return;
    currentTab.stagedDraftRows.splice(dIdx, 1);
  }

  /**
   * Clones a visible row into an editable draft.
   * @param withoutKey when true, primary-key values are blanked so the database
   *   can generate a new identity (the safe default); when false the key is
   *   copied verbatim, which only makes sense for composite/derived keys.
   */
  function duplicateRowAsDraft(rIdx: number, withoutKey = true) {
    if (!currentTab || !result?.rows || !result.rows[rIdx]) return;
    if (!currentTab.stagedDraftRows) currentTab.stagedDraftRows = [];
    const row = result.rows[rIdx];
    const draft: Record<string, any> = {};
    allColumns.forEach((colName, cIdx) => {
      const isPK = Boolean(result!.columns[cIdx] && isColPK(result!.columns[cIdx]));
      draft[colName] = withoutKey && isPK ? '' : row[cIdx];
    });
    currentTab.stagedDraftRows.unshift(draft);
    appState.showToast(
      `Row #${rIdx + 1} duplicated as editable draft${withoutKey ? ' (key cleared)' : ' (key retained)'}`,
      'success'
    );
  }

  function getFkInfo(colName: string) {
    return fkMap.get(colName) || null;
  }

  // Inline Editing
  function startCellEdit(origIdx: number, cIdx: number, val: any) {
    editingCell = {
      rIdx: origIdx,
      cIdx,
      tempValue: val === null || val === undefined ? '' : String(val),
    };
  }

  function commitCellEdit(origIdx: number, cIdx: number) {
    if (!editingCell || !currentTab?.tableName || !result?.rows) return;
    const colName = allColumns[cIdx];
    const oldVal = result.rows[origIdx][cIdx];
    const newVal = editingCell.tempValue;

    if (newVal !== (oldVal === null ? '' : String(oldVal))) {
      // Find PK col
      const pkColObj = result.columns.find((c) => isColPK(c));
      const pkCol = pkColObj ? getColName(pkColObj) : undefined;
      const pkVal = pkCol ? result.rows[origIdx][allColumns.indexOf(pkCol)] : undefined;

      appState.stageEdit(currentTab.tableName, origIdx, cIdx, colName, pkCol, pkVal, oldVal, newVal);
      result.rows[origIdx][cIdx] = newVal;
    }
    editingCell = null;
  }

  function isCellStaged(table: string, rIdx: number, cIdx: number): boolean {
    return Boolean(appState.stagedEdits[`${table}:${rIdx}:${cIdx}`]);
  }

  // Context Menu
  function handleCellContextMenu(e: MouseEvent, origIdx: number, cIdx: number, val: any) {
    e.preventDefault();
    contextMenu = {
      open: true,
      x: Math.min(window.innerWidth - 200, e.clientX),
      y: Math.min(window.innerHeight - 240, e.clientY),
      rIdx: origIdx,
      cIdx,
      colName: allColumns[cIdx],
      cellValue: val,
    };
  }

  function closeContextMenu() {
    if (contextMenu) contextMenu.open = false;
  }

  // Quick Filter helpers
  function applyQuickFilter(op: FilterPill['op']) {
    if (!contextMenu) return;
    const { colName, cellValue } = contextMenu;
    const val = cellValue === null || cellValue === undefined ? '' : String(cellValue);
    appState.addFilterPill(colName, op, val);
    closeContextMenu();
  }

  // Export functions
  function exportCSV() {
    if (!result?.rows || !result?.columns) return;
    const headers = result.columns.map((c) => `"${getColName(c)}"`).join(',');
    const rows = result.rows.map((row) =>
      row.map((cell) => `"${String(cell ?? '').replace(/"/g, '""')}"`).join(',')
    );
    const csv = [headers, ...rows].join('\n');
    downloadFile(csv, `${currentTab?.tableName || 'query_export'}.csv`, 'text/csv');
    showExportMenu = false;
  }

  function exportJSON() {
    if (!result?.rows || !result?.columns) return;
    const cols = result.columns.map((c) => getColName(c));
    const data = result.rows.map((row) => {
      const obj: Record<string, any> = {};
      cols.forEach((col, idx) => {
        obj[col] = row[idx];
      });
      return obj;
    });
    const jsonStr = JSON.stringify(data, null, 2);
    downloadFile(jsonStr, `${currentTab?.tableName || 'query_export'}.json`, 'application/json');
    showExportMenu = false;
  }

  function exportSQL() {
    if (!result?.rows || !result?.columns) return;
    const tbl = currentTab?.tableName || 'exported_table';
    const cols = result.columns.map((c) => `\`${getColName(c)}\``).join(', ');
    const inserts = result.rows.map((row) => {
      const vals = row.map((c) => {
        if (c === null || c === undefined) return 'NULL';
        if (typeof c === 'number') return c;
        return `'${String(c).replace(/'/g, "''")}'`;
      }).join(', ');
      return `INSERT INTO \`${tbl}\` (${cols}) VALUES (${vals});`;
    });
    downloadFile(inserts.join('\n'), `${tbl}.sql`, 'text/plain');
    showExportMenu = false;
  }

  function downloadFile(content: string, filename: string, type: string) {
    const blob = new Blob([content], { type });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
    appState.showToast(`Exported ${filename}`, 'success');
  }

  // Delete Row
  async function handleDeleteCurrentRow() {
    if (selectedRowIndex === null || !currentTab?.tableName || !result?.rows) return;
    const pkColObj = result.columns.find((c) => isColPK(c));
    if (!pkColObj) {
      appState.showToast('Table does not have a primary key for deletion', 'warning');
      return;
    }
    const pkCol = getColName(pkColObj);
    const pkVal = result.rows[selectedRowIndex][allColumns.indexOf(pkCol)];
    const ok = await appState.showNativeConfirm(
      `Delete record with ${pkCol} = ${pkVal}?`,
      'Confirm Deletion'
    );
    if (ok) {
      await appState.deleteRows(pkCol, [pkVal]);
      selectedRowIndex = null;
    }
  }

  function copyRowAsTSV(rIdx: number) {
    if (!result?.rows || !result.rows[rIdx] || !result.columns) return;
    const cols = result.columns.map((c) => getColName(c)).join('\t');
    const vals = result.rows[rIdx].map((v) => (v === null || v === undefined ? '' : String(v))).join('\t');
    navigator.clipboard.writeText(`${cols}\n${vals}`);
    appState.showToast('Copied row as TSV', 'info');
  }

  function copyRowAsMarkdown(rIdx: number) {
    if (!result?.rows || !result.rows[rIdx] || !result.columns) return;
    const colNames = result.columns.map((c) => getColName(c));
    const header = `| ${colNames.join(' | ')} |`;
    const separator = `| ${colNames.map(() => '---').join(' | ')} |`;
    const row = `| ${result.rows[rIdx].map((v) => (v === null ? '*NULL*' : String(v).replace(/\|/g, '\\|'))).join(' | ')} |`;
    navigator.clipboard.writeText(`${header}\n${separator}\n${row}`);
    appState.showToast('Copied row as Markdown Table', 'info');
  }

  function copyRowAsInsertSQL(rIdx: number) {
    if (!result?.rows || !result.rows[rIdx] || !result.columns) return;
    const tbl = currentTab?.tableName || 'table_name';
    const cols = result.columns.map((c) => `\`${getColName(c)}\``).join(', ');
    const vals = result.rows[rIdx]
      .map((c) => {
        if (c === null || c === undefined) return 'NULL';
        if (typeof c === 'number') return c;
        return `'${String(c).replace(/'/g, "''")}'`;
      })
      .join(', ');
    const sql = `INSERT INTO \`${tbl}\` (${cols}) VALUES (${vals});`;
    navigator.clipboard.writeText(sql);
    appState.showToast('Copied as INSERT SQL statement', 'info');
  }
</script>

<svelte:window onclick={closeContextMenu} />

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text relative">
  <!-- Multi-Statement Execution Results Tabs -->
  {#if currentTab?.multiResults && currentTab.multiResults.length > 1}
    <div class="h-8 bg-[#0b0f19] border-b border-slate-800 px-3 flex items-center space-x-1 select-none overflow-x-auto shrink-0">
      {#each currentTab.multiResults as mRes, mIdx}
        <button
          type="button"
          onclick={() => {
            if (currentTab) {
              currentTab.activeResultIndex = mIdx;
              currentTab.activeResult = mRes;
            }
          }}
          class="px-2.5 py-1 rounded text-xs font-mono font-medium transition {(currentTab.activeResultIndex || 0) === mIdx ? 'bg-sky-500/20 text-sky-300 font-bold border border-sky-500/50' : 'text-slate-400 hover:text-white hover:bg-slate-800'}"
        >
          Result {mIdx + 1} ({mRes.row_count ?? mRes.rows?.length ?? 0} rows)
        </button>
      {/each}
    </div>
  {/if}

  <!-- Profiler Panel Drawer -->
  {#if currentTab?.showQueryProfiler}
    <div class="h-48 shrink-0 overflow-hidden">
      <QueryProfilerPanel />
    </div>
  {/if}

  <!-- Grid Control Toolbar -->
  <div id="grid-toolbar" class="h-9 bg-[#0b0f19] border-b border-slate-800 px-3 flex items-center justify-between text-xs select-none shrink-0 gap-2">
    <!-- Left: CRUD & Staged Actions -->
    <div class="flex items-center space-x-2">
      <!-- In-Place Draft Row Button -->
      <button
        onclick={addDraftRow}
        class="h-6 px-2.5 rounded bg-sky-500/20 hover:bg-sky-500/30 text-sky-300 border border-sky-500/40 font-semibold flex items-center space-x-1 transition text-[11px]"
        title="Add in-place draft row"
      >
        <Icon name="plus" class="w-3 h-3 text-sky-400" />
        <span>Draft Row</span>
      </button>

      {#if selectedRowIndices.length > 0}
        <button
          onclick={handleBatchDelete}
          class="h-6 px-2.5 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold flex items-center space-x-1 transition text-[11px] shadow animate-in fade-in"
          title="Delete selected rows"
        >
          <Icon name="trash-2" class="w-3 h-3" />
          <span>Delete ({selectedRowIndices.length})</span>
        </button>
      {/if}

      {#if currentTab?.isTableViewer}
        <button
          id="btn-grid-add-row"
          onclick={() => (appState.isInsertRowModalOpen = true)}
          class="h-6 px-2.5 rounded bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-300 border border-emerald-500/40 font-semibold flex items-center space-x-1.5 transition text-[11px]"
          title="Insert new record"
        >
          <Icon name="plus" class="w-3 h-3 text-emerald-400" />
          <span>Insert Row</span>
        </button>

        <button
          id="btn-import-csv"
          onclick={() => appState.openCsvImportModal()}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 flex items-center space-x-1 transition text-[11px]"
          title="Import records from a CSV file"
        >
          <Icon name="upload" class="w-3 h-3 text-indigo-300" />
          <span>Import CSV</span>
        </button>

        <button
          id="btn-mock-data"
          onclick={() => (appState.isMockDataModalOpen = true)}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 flex items-center space-x-1 transition text-[11px]"
          title="Generate synthetic test records"
        >
          <Icon name="sparkles" class="w-3 h-3 text-purple-300" />
          <span>Mock Data</span>
        </button>

        <button
          onclick={handleDeleteCurrentRow}
          disabled={selectedRowIndex === null}
          class="h-6 px-2 rounded bg-rose-500/10 hover:bg-rose-500/20 disabled:opacity-40 text-rose-300 border border-rose-500/30 flex items-center space-x-1 transition text-[11px]"
          title="Delete selected row"
        >
          <Icon name="trash-2" class="w-3 h-3 text-rose-400" />
          <span>Delete</span>
        </button>

        <div class="h-4 w-px bg-slate-800 mx-0.5"></div>

        <!-- Filter Bar Toggle Button -->
        <button
          type="button"
          onclick={toggleFilterBar}
          class="h-6 px-2 rounded {isFilterOpen ? 'bg-sky-500/20 text-sky-300 border-sky-500/40 font-medium' : hasActiveFilter ? 'bg-emerald-950/40 text-emerald-300 border-emerald-600/50 hover:bg-emerald-900/40' : 'bg-[#1e293b] text-slate-300 hover:text-white'} border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1.5 text-[11px] transition"
          title={hasActiveFilter ? 'Filter active (click to toggle WHERE / Pills bar)' : 'Toggle Filter (WHERE / Pills)'}
        >
          <Icon name="filter" class="w-3 h-3 {isFilterOpen ? 'text-sky-400' : hasActiveFilter ? 'text-emerald-400' : 'text-slate-400'}" />
          <span>Filter</span>
          {#if hasActiveFilter}
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" title="Active filter applied"></span>
          {/if}
        </button>

        <!-- Edit mode: staged vs auto-commit -->
        <div
          id="grid-edit-mode-container"
          class="flex items-center space-x-1 rounded border border-slate-800 bg-slate-900 px-2 py-0.5 text-[11px]"
        >
          <span class="text-[10px] text-slate-400">Edit:</span>
          <button
            id="btn-grid-mode-staged"
            type="button"
            onclick={() => appState.setGridEditMode('staged')}
            class="rounded px-1.5 py-0.5 text-[10px] font-medium transition {appState.gridEditMode === 'staged'
              ? 'bg-sky-500/20 text-sky-300'
              : 'text-slate-400 hover:text-slate-200'}"
            title="Manual apply: changes stay staged until you click Apply"
          >
            Staged
          </button>
          <button
            id="btn-grid-mode-auto"
            type="button"
            onclick={() => appState.setGridEditMode('auto')}
            class="rounded px-1.5 py-0.5 text-[10px] font-medium transition {appState.gridEditMode === 'auto'
              ? 'bg-amber-500/20 text-amber-300'
              : 'text-slate-400 hover:text-slate-200'}"
            title="Auto-commit: persist directly on blur/Enter"
          >
            Auto
          </button>
        </div>
      {/if}

      <!-- Staged Edits Review Pill -->
      {#if appState.stagedCount > 0}
        <div class="flex items-center space-x-1.5 bg-amber-950/60 border border-amber-600/70 rounded px-2 py-0.5 animate-in fade-in">
          <span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
          <span class="text-[11px] font-bold text-amber-300 font-mono">
            {appState.stagedCount} Staged Edit{appState.stagedCount > 1 ? 's' : ''}
          </span>
          <button
            onclick={() => appState.commitStagedEdits()}
            class="ml-1 px-1.5 py-0.2 rounded bg-amber-500 text-slate-950 font-bold hover:bg-amber-400 text-[10px] transition"
          >
            Save All
          </button>
          <button
            onclick={() => appState.discardStagedEdits()}
            class="px-1 text-slate-400 hover:text-white text-[10px] transition"
          >
            Discard
          </button>
        </div>
      {/if}
    </div>

    <!-- Right: Column Picker, Export & Inspect -->
    <div class="flex items-center space-x-2">
      <!-- Column Picker Dropdown -->
      <div class="relative">
        <button
          id="btn-col-picker"
          onclick={(e) => { e.stopPropagation(); showColPicker = !showColPicker; }}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
          title="Toggle column visibility"
          aria-expanded={showColPicker}
          aria-controls="col-picker-dropdown"
        >
          <Icon name="columns" class="w-3 h-3 text-sky-400" />
          <span>Columns (<span id="col-picker-count-badge">{visibleColumns.length}</span>/{allColumns.length})</span>
          <Icon name="chevron-down" class="w-2.5 h-2.5 text-slate-400" />
        </button>

        {#if showColPicker}
          <button
            tabindex="-1"
            aria-label="Close"
            class="fixed inset-0 z-30 bg-transparent cursor-default"
            onclick={() => (showColPicker = false)}
          ></button>
          <div
            id="col-picker-dropdown"
            class="absolute right-0 mt-1 w-56 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-40 p-2 text-xs space-y-2 animate-in fade-in duration-100"
          >
            <input
              id="col-picker-search"
              type="text"
              bind:value={colPickerSearch}
              placeholder="Search columns..."
              class="w-full bg-[#090d16] border border-slate-700/80 rounded px-2 py-1 text-xs text-slate-100 font-mono focus:outline-none"
            />
            <div class="flex items-center justify-between text-[10px] text-sky-400 px-1">
              <button onclick={() => appState.showAllColumns()} class="hover:underline">Show All</button>
              <button onclick={() => appState.hideAllColumns(allColumns)} class="hover:underline">Hide All</button>
            </div>
            <div id="col-picker-list" class="max-h-48 overflow-y-auto space-y-1 py-1 divide-y divide-slate-800/40">
              {#each allColumns as col}
                {#if !colPickerSearch || col.toLowerCase().includes(colPickerSearch.toLowerCase())}
                  <label class="flex items-center space-x-2 py-1 px-1 rounded hover:bg-slate-800 cursor-pointer text-slate-300 font-mono text-[11px]">
                    <input
                      type="checkbox"
                      checked={!hiddenCols.includes(col)}
                      onchange={() => appState.toggleColumnHidden(col)}
                      class="rounded bg-[#090d16] border-slate-700 text-sky-500 focus:ring-0"
                    />
                    <span class="truncate">{col}</span>
                  </label>
                {/if}
              {/each}
            </div>
          </div>
        {/if}
      </div>

      <!-- Export Dropdown -->
      <div id="export-csv-dropdown-container" class="relative">
        <button
          onclick={(e) => { e.stopPropagation(); showExportMenu = !showExportMenu; }}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
          title="Export data"
        >
          <Icon name="share-2" class="w-3 h-3 text-sky-400" />
          <span>Export</span>
          <Icon name="chevron-down" class="w-2.5 h-2.5 text-slate-400" />
        </button>

        {#if showExportMenu}
          <button
            tabindex="-1"
            aria-label="Close"
            class="fixed inset-0 z-30 bg-transparent cursor-default"
            onclick={() => (showExportMenu = false)}
          ></button>
          <div
            id="export-csv-menu"
            class="absolute right-0 mt-1 w-44 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-40 py-1 divide-y divide-slate-800 animate-in fade-in duration-100 text-xs"
          >
            <button
              onclick={exportCSV}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="file-text" class="w-3.5 h-3.5 text-emerald-400" />
              <span>Export as CSV</span>
            </button>
            <button
              onclick={exportJSON}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="file-code" class="w-3.5 h-3.5 text-amber-400" />
              <span>Export as JSON</span>
            </button>
            <button
              onclick={exportSQL}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="code" class="w-3.5 h-3.5 text-sky-400" />
              <span>Export as SQL</span>
            </button>
            {#if currentTab?.isTableViewer && currentTab.tableName}
              <button
                id="export-menu-table-dump"
                onclick={() => {
                  showExportMenu = false;
                  appState.exportTableDump(currentTab!.tableName);
                }}
                class="flex w-full flex-col items-start px-3 py-1.5 text-left hover:bg-slate-800"
              >
                <span class="font-medium text-sky-400">Table SQL Dump</span>
                <span class="text-[10px] text-slate-400">Export this table's DDL + data (.sql)</span>
              </button>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Record Inspector Trigger -->
      <button
        onclick={() => {
          appState.recordInspectorRowIndex = selectedRowIndex ?? 0;
          appState.isRecordInspectorOpen = true;
        }}
        disabled={totalRows === 0}
        class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
        title="Inspect record in vertical form"
      >
        <Icon name="eye" class="w-3 h-3 text-sky-400" />
        <span>Inspect</span>
      </button>

      <!-- Cell Drawer Toggle -->
      <button
        onclick={() => (showDetailDrawer = !showDetailDrawer)}
        class="h-6 px-2 rounded {showDetailDrawer ? 'bg-sky-500/20 text-sky-300 border-sky-500/40' : 'bg-[#1e293b] text-slate-400 hover:text-white'} border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
        title="Toggle Cell Drawer"
      >
        <Icon name="sidebar" class="w-3 h-3" />
        <span>Drawer</span>
      </button>
    </div>
  </div>

  <!-- TablePlus-style Filter Bar (Table Viewer) or Filter Pills Bar -->
  {#if currentTab?.isTableViewer && isFilterOpen}
    <div class="min-h-9 bg-[#0b0f19] border-b border-slate-800 px-3 py-1.5 flex flex-wrap items-center justify-between gap-2 text-xs select-none shrink-0 animate-in fade-in duration-100">
      <div class="flex flex-wrap items-center gap-2 flex-1 min-w-0">
        <!-- Mode Toggle: Pills vs WHERE -->
        <div class="inline-flex rounded border border-slate-700/80 bg-[#090d16] p-0.5 select-none" role="group">
          <button
            type="button"
            onclick={() => appState.setFilterMode('pills')}
            class="px-2 py-0.5 text-[11px] font-semibold rounded transition {currentTab.filterMode !== 'raw' ? 'bg-sky-500 text-slate-950 shadow-sm' : 'text-slate-400 hover:text-white'}"
            title="Filter by column pills"
          >
            Pills
          </button>
          <button
            type="button"
            onclick={() => appState.setFilterMode('raw')}
            class="px-2 py-0.5 text-[11px] font-semibold rounded transition {currentTab.filterMode === 'raw' ? 'bg-sky-500 text-slate-950 shadow-sm' : 'text-slate-400 hover:text-white'}"
            title="Freeform SQL WHERE expression"
          >
            WHERE
          </button>
        </div>

        {#if currentTab.filterMode === 'raw'}
          <!-- Freeform SQL WHERE Expression Input -->
          <div class="flex-1 flex items-center space-x-1.5 min-w-[260px]">
            <span class="px-1.5 py-0.5 bg-sky-950 text-sky-400 border border-sky-800 rounded font-mono text-[10px] font-bold tracking-wider select-none shrink-0">
              WHERE
            </span>
            <div class="relative flex-1 flex items-center">
              <input
                type="text"
                value={currentTab.filterRaw || ''}
                oninput={(e) => appState.setFilterRaw((e.target as HTMLInputElement).value)}
                onkeydown={(e) => {
                  if (e.key === 'Enter') appState.applyTableFilter();
                  if (e.key === 'Escape') {
                    appState.setFilterRaw('');
                    (e.target as HTMLInputElement).blur();
                  }
                }}
                placeholder="e.g. status = 'active' AND (views > 100 OR is_featured = 1)"
                class="w-full bg-[#090d16] text-slate-100 placeholder-slate-500 border border-[#334155] focus:border-sky-400 pl-2.5 pr-7 py-1 rounded outline-none font-mono text-xs ring-1 ring-transparent focus:ring-sky-500/30 transition"
              />
              {#if currentTab.filterRaw}
                <button
                  type="button"
                  onclick={() => appState.setFilterRaw('')}
                  class="absolute right-2 text-slate-500 hover:text-slate-200 text-xs px-1 rounded transition"
                  title="Clear input (Esc)"
                >
                  <Icon name="x" class="w-3 h-3" />
                </button>
              {/if}
            </div>
            <button
              type="button"
              onclick={() => appState.applyTableFilter()}
              class="px-2.5 py-1 rounded bg-sky-600 hover:bg-sky-500 text-white font-semibold text-[11px] transition shrink-0"
            >
              Apply
            </button>
          </div>
        {:else}
          <!-- Pills Mode: List of condition pills + '+ AND' button -->
          <div class="flex flex-wrap items-center gap-1.5">
            {#each currentTab.filterPills || [] as pill, idx}
              {@const isUnary = pill.op === 'IS NULL' || pill.op === 'IS NOT NULL'}
              <div class="inline-flex items-center space-x-1 bg-[#090d16] border border-sky-800/80 rounded px-1.5 py-0.5 text-[11px] font-mono shadow-sm">
                <!-- Column Select -->
                <select
                  value={pill.col}
                  onchange={(e) => {
                    appState.updateFilterPill(idx, { col: (e.target as HTMLSelectElement).value });
                  }}
                  class="bg-transparent text-sky-300 font-bold outline-none cursor-pointer text-[11px] py-0.5 border-none"
                >
                  {#each allColumns as col}
                    <option value={col} class="bg-[#0f172a] text-slate-200">{col}</option>
                  {/each}
                </select>

                <!-- Operator Select -->
                <select
                  value={pill.op}
                  onchange={(e) => {
                    const newOp = (e.target as HTMLSelectElement).value as FilterPill['op'];
                    appState.updateFilterPill(idx, { op: newOp });
                  }}
                  class="bg-transparent text-sky-400 font-semibold outline-none cursor-pointer text-[11px] py-0.5 border-none"
                >
                  <option value="=" class="bg-[#0f172a] text-slate-200">=</option>
                  <option value="!=" class="bg-[#0f172a] text-slate-200">!=</option>
                  <option value="LIKE" class="bg-[#0f172a] text-slate-200">LIKE</option>
                  <option value="NOT LIKE" class="bg-[#0f172a] text-slate-200">NOT LIKE</option>
                  <option value=">" class="bg-[#0f172a] text-slate-200">&gt;</option>
                  <option value="<" class="bg-[#0f172a] text-slate-200">&lt;</option>
                  <option value=">=" class="bg-[#0f172a] text-slate-200">&gt;=</option>
                  <option value="<=" class="bg-[#0f172a] text-slate-200">&lt;=</option>
                  <option value="IN" class="bg-[#0f172a] text-slate-200">IN</option>
                  <option value="NOT IN" class="bg-[#0f172a] text-slate-200">NOT IN</option>
                  <option value="BETWEEN" class="bg-[#0f172a] text-slate-200">BETWEEN</option>
                  <option value="IS NULL" class="bg-[#0f172a] text-slate-200">IS NULL</option>
                  <option value="IS NOT NULL" class="bg-[#0f172a] text-slate-200">IS NOT NULL</option>
                </select>

                <!-- Value Input (hidden if IS NULL / IS NOT NULL) -->
                {#if !isUnary}
                  <input
                    type="text"
                    value={pill.val}
                    oninput={(e) => {
                      appState.updateFilterPill(idx, { val: (e.target as HTMLInputElement).value });
                    }}
                    onkeydown={(e) => {
                      if (e.key === 'Enter') appState.applyTableFilter();
                    }}
                    placeholder={pill.op === 'LIKE' || pill.op === 'NOT LIKE' ? 'substring or %' : (pill.op === 'IN' || pill.op === 'NOT IN' ? '1, 2, 3' : 'val')}
                    class="bg-transparent text-slate-100 font-mono text-[11px] outline-none px-1 py-0.5 w-20 focus:w-36 transition-all border-b border-transparent focus:border-sky-400"
                  />
                {/if}

                <!-- Remove Pill Button -->
                <button
                  type="button"
                  onclick={() => appState.removeFilterPill(idx)}
                  class="text-slate-500 hover:text-rose-400 p-0.5 transition"
                  title="Remove condition"
                >
                  <Icon name="x" class="w-3 h-3" />
                </button>
              </div>
            {/each}

            <!-- + AND Button -->
            <button
              type="button"
              onclick={() => appState.addFilterPill()}
              class="text-xs text-slate-400 hover:text-sky-300 hover:bg-slate-800 px-2 py-0.5 rounded border border-dashed border-slate-700 transition flex items-center space-x-1"
              title="Add condition (+ AND)"
            >
              <Icon name="plus" class="w-3 h-3 text-sky-400" />
              <span>AND</span>
            </button>

            {#if currentTab.filterPills && currentTab.filterPills.length > 0}
              <button
                type="button"
                onclick={() => appState.applyTableFilter()}
                class="px-2 py-0.5 rounded bg-sky-600 hover:bg-sky-500 text-white font-semibold text-[11px] transition"
              >
                Apply
              </button>
            {/if}
          </div>
        {/if}

        <!-- Active Filter Indicator Badge -->
        {#if currentTab.appliedFilter}
          <span class="px-2 py-0.5 rounded text-[10px] font-medium bg-emerald-950 text-emerald-300 border border-emerald-700 flex items-center space-x-1" title="Active filter applied">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
            <span>Filtered</span>
          </span>
        {/if}
      </div>

      <!-- Right side: Clear Filter & Open in SQL Tab -->
      <div class="flex items-center space-x-2 shrink-0">
        {#if currentTab.appliedFilter || (currentTab.filterPills && currentTab.filterPills.length > 0) || currentTab.filterRaw}
          <button
            type="button"
            onclick={() => appState.clearTableFilter()}
            class="text-[11px] text-slate-400 hover:text-rose-400 transition underline"
            title="Clear all filters"
          >
            Clear Filter
          </button>
        {/if}

        <button
          type="button"
          onclick={() => appState.handoffTableQueryToEditor()}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-sky-400 hover:text-sky-300 border border-[#334155] flex items-center space-x-1 text-[11px] transition font-mono"
          title="Open filtered query in full SQL Editor"
        >
          <Icon name="file-code" class="w-3 h-3" />
          <span>SQL</span>
        </button>

        <button
          type="button"
          onclick={toggleFilterBar}
          class="h-6 w-6 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-400 hover:text-white border border-slate-700/80 hover:border-slate-600 flex items-center justify-center transition"
          title="Hide filter bar"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  {:else if currentTab?.filterPills && currentTab.filterPills.length > 0}
    <!-- Query Tab Filter Pills Bar (if quick filters active from context menu) -->
    <div class="min-h-8 bg-[#090d16] border-b border-[#1e293b] px-3 py-1 flex items-center justify-between gap-2 text-xs">
      <div class="flex flex-wrap items-center gap-1.5">
        <span class="text-[10px] text-slate-500 font-bold uppercase tracking-wider flex items-center space-x-1">
          <Icon name="filter" class="w-3 h-3 text-sky-400" />
          <span>Filters:</span>
        </span>
        {#each currentTab.filterPills as pill, idx}
          <div class="inline-flex items-center space-x-1 bg-sky-950/60 border border-sky-800/60 text-sky-300 px-2 py-0.5 rounded text-[11px] font-mono">
            <span class="font-bold">{pill.col}</span>
            <span class="text-sky-400 font-semibold">{pill.op}</span>
            {#if pill.op !== 'IS NULL' && pill.op !== 'IS NOT NULL'}
              <span class="text-slate-100">'{pill.val}'</span>
            {/if}
            <button
              onclick={() => appState.removeFilterPill(idx)}
              class="text-sky-400 hover:text-white ml-1 p-0.5 rounded"
              title="Remove filter"
              aria-label="Remove filter"
            >
              <Icon name="x" class="w-2.5 h-2.5" />
            </button>
          </div>
        {/each}
      </div>

      <button
        onclick={() => appState.clearFilterPills()}
        class="text-[11px] text-slate-400 hover:text-rose-400 transition underline shrink-0"
      >
        Clear All
      </button>
    </div>
  {/if}

  <!-- Data Grid Virtual Table -->
  {#if isExecuting}
    <div class="flex-1 flex flex-col items-center justify-center p-8 space-y-3">
      <Icon name="refresh-cw" class="w-7 h-7 text-sky-400 animate-spin" />
      <span class="text-xs text-slate-300 font-mono">Executing query on {appState.currentDatabase}...</span>
    </div>
  {:else if result?.error}
    <div class="p-6 m-4 bg-rose-950/20 border border-rose-500/40 rounded-xl max-w-3xl shadow-xl overflow-y-auto">
      <div class="flex items-center space-x-2 text-rose-400 font-bold text-sm mb-2.5">
        <Icon name="alert-triangle" class="w-4 h-4 text-rose-400" />
        <span>Execution Error</span>
      </div>
      <pre class="text-xs font-mono text-rose-200 bg-rose-950/50 p-3.5 rounded-lg border border-rose-900/60 overflow-x-auto whitespace-pre-wrap leading-relaxed">{result.error}</pre>
    </div>
  {:else if result?.columns && result.columns.length > 0}
    <div
      bind:this={containerRef}
      onscroll={onScroll}
      class="flex-1 overflow-auto relative border-t border-[#1e293b]"
    >
      <table class="border-collapse text-xs font-mono w-full text-left min-w-full">
        <!-- Sticky Header -->
        <thead class="sticky top-0 z-20 bg-[#0f172a] border-b border-[#334155] shadow-sm select-none">
          <tr>
            <th class="w-16 px-2 py-2 text-center text-[10px] text-slate-500 bg-[#090d16] border-r border-[#1e293b] font-medium sticky left-0 z-30">
              <div class="flex items-center justify-center space-x-1.5">
                <input
                  type="checkbox"
                  checked={isAllSelected}
                  onchange={toggleSelectAll}
                  class="rounded border-slate-700 bg-slate-900 text-sky-500 focus:ring-0"
                  title="Select all rows"
                />
                <span>#</span>
              </div>
            </th>
            {#each visibleColumns as col}
              {@const colName = getColName(col)}
              {@const colType = getColType(col)}
              {@const isPK = isColPK(col)}
              <th
                onclick={() => handleHeaderSort(colName)}
                class="px-3 py-1.5 font-semibold text-slate-200 border-r border-[#1e293b] whitespace-nowrap bg-[#0f172a] text-[11px] tracking-tight hover:bg-slate-800 cursor-pointer transition-colors"
              >
                <div class="flex items-center space-x-1.5">
                  {#if isPK}
                    <Icon name="key" class="w-3 h-3 text-amber-400 shrink-0" />
                  {/if}
                  <span class="font-mono text-slate-100">{colName}</span>
                  {#if colType}
                    <span class="text-[9px] font-mono text-slate-400 bg-slate-800 px-1 py-0.2 rounded border border-slate-700 uppercase">
                      {colType}
                    </span>
                  {/if}
                  {#if sortCol === colName}
                    <Icon name={sortDir === 'ASC' ? 'chevron-up' : 'chevron-down'} class="w-3 h-3 text-sky-400 shrink-0 ml-1" />
                  {/if}
                </div>
              </th>
            {/each}
          </tr>
        </thead>

        <!-- Virtual Rows Body -->
        <tbody>
          <!-- In-Place Draft Rows -->
          {#if currentTab?.stagedDraftRows && currentTab.stagedDraftRows.length > 0}
            {#each currentTab.stagedDraftRows as draft, dIdx}
              <tr class="bg-sky-950/40 border-b border-sky-800/60 text-sky-200 h-7 font-mono text-xs">
                <td class="w-16 px-2 py-1 text-center bg-sky-950/80 border-r border-sky-800/60 sticky left-0 z-10">
                  <button
                    type="button"
                    onclick={() => removeDraftRow(dIdx)}
                    class="text-rose-400 hover:text-rose-200 p-0.5 rounded hover:bg-rose-950/50 inline-flex items-center justify-center transition"
                    title="Discard draft row"
                    aria-label="Discard draft row"
                  >
                    <Icon name="x" class="w-3.5 h-3.5" />
                  </button>
                </td>
                {#each visibleColumns as col}
                  {@const colName = getColName(col)}
                  <td class="px-2 py-0.5 border-r border-sky-800/40">
                    <input
                      type="text"
                      bind:value={draft[colName]}
                      placeholder="draft..."
                      class="w-full bg-transparent text-sky-100 placeholder-sky-600/60 outline-none text-xs font-mono"
                    />
                  </td>
                {/each}
              </tr>
            {/each}
          {/if}

          {#if topSpacerHeight > 0}
            <tr style="height: {topSpacerHeight}px; pointer-events: none;">
              <td colspan={totalCols} style="padding:0; border:0; height:{topSpacerHeight}px;"></td>
            </tr>
          {/if}

          {#if processedRows.length === 0}
            <tr>
              <td colspan={totalCols} class="p-12 text-center text-slate-500 italic text-xs">
                Zero records returned.
              </td>
            </tr>
          {:else}
            {#each visibleRows as rowItem (rowItem.origIdx)}
              {@const isRowSelected = selectedRowIndex === rowItem.origIdx}
              {@const isChecked = selectedRowIndices.includes(rowItem.origIdx)}
              <tr
                onclick={() => (selectedRowIndex = rowItem.origIdx)}
                class="border-b border-[#1e293b]/60 transition-colors h-7 {
                  isChecked ? 'bg-sky-500/20' : isRowSelected ? 'bg-sky-500/15' : rowItem.displayIdx % 2 === 1 ? 'bg-slate-900/20' : 'bg-transparent'
                } hover:bg-sky-500/10"
              >
                <!-- Row Gutter -->
                <td
                  ondblclick={() => {
                    appState.recordInspectorRowIndex = rowItem.origIdx;
                    appState.isRecordInspectorOpen = true;
                  }}
                  class="w-16 px-2 py-1 text-center text-[10px] text-slate-500 bg-[#090d16] border-r border-[#1e293b] select-none font-mono sticky left-0 z-10 cursor-pointer hover:text-sky-400"
                  title="Double-click to open Record Inspector"
                >
                  <div class="flex items-center justify-center space-x-1.5">
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onclick={(e) => e.stopPropagation()}
                      onchange={() => toggleRowSelection(rowItem.origIdx)}
                      class="rounded border-slate-700 bg-slate-900 text-sky-500 focus:ring-0"
                    />
                    <span>{rowItem.displayIdx + 1}</span>
                  </div>
                </td>

                <!-- Cells -->
                {#each visibleColumns as col}
                  {@const colName = getColName(col)}
                  {@const cIdx = colIndexMap.get(colName) ?? -1}
                  {@const cellVal = cIdx >= 0 ? rowItem.data[cIdx] : undefined}
                  {@const formatted = formatCellValue(cellVal)}
                  {@const isStaged = currentTab?.tableName && cIdx >= 0 ? isCellStaged(currentTab.tableName, rowItem.origIdx, cIdx) : false}
                  {@const isEditingThis = editingCell && editingCell.rIdx === rowItem.origIdx && editingCell.cIdx === cIdx}
                  {@const fk = fkMap.get(colName)}

                  <td
                    onclick={() => (selectedCell = { rIdx: rowItem.origIdx, cIdx })}
                    ondblclick={() => startCellEdit(rowItem.origIdx, cIdx, cellVal)}
                    oncontextmenu={(e) => handleCellContextMenu(e, rowItem.origIdx, cIdx, cellVal)}
                    class="px-3 py-1 border-r border-[#1e293b]/40 whitespace-nowrap max-w-sm truncate {getCellClass(formatted.type)} cursor-cell hover:bg-sky-900/30 transition-colors relative {
                      isStaged ? 'bg-amber-950/40 text-amber-200 font-semibold ring-1 ring-amber-500' : ''
                    }"
                    title="{formatted.display} (Double-click to edit, right-click for filters)"
                  >
                    {#if isEditingThis}
                      <input
                        type="text"
                        bind:value={editingCell!.tempValue}
                        onblur={() => commitCellEdit(rowItem.origIdx, cIdx)}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') commitCellEdit(rowItem.origIdx, cIdx);
                          if (e.key === 'Escape') editingCell = null;
                        }}
                        class="w-full bg-[#0f172a] text-slate-100 border border-sky-400 px-1 py-0 text-xs font-mono outline-none"
                      />
                    {:else}
                      <div class="flex items-center justify-between group/cell">
                        <span class="truncate block">{formatted.display}</span>
                        {#if fk && cellVal !== null && cellVal !== undefined}
                          <button
                            type="button"
                            onclick={(e) => {
                              e.stopPropagation();
                              appState.openFkPeek(e, currentTab?.tableName || '', colName, fk.referenced_table, fk.referenced_column, cellVal);
                            }}
                            class="opacity-0 group-hover/cell:opacity-100 hover:text-sky-300 text-sky-400 p-0.5 rounded hover:bg-slate-800 ml-1 inline-flex items-center"
                            title="Peek {fk.referenced_table}.{fk.referenced_column}"
                            aria-label="Peek {fk.referenced_table}.{fk.referenced_column}"
                          >
                            <Icon name="link" class="w-3 h-3" />
                          </button>
                        {/if}
                      </div>
                    {/if}
                  </td>
                {/each}
              </tr>
            {/each}
          {/if}

          {#if bottomSpacerHeight > 0}
            <tr style="height: {bottomSpacerHeight}px; pointer-events: none;">
              <td colspan={totalCols} style="padding:0; border:0; height:{bottomSpacerHeight}px;"></td>
            </tr>
          {/if}
        </tbody>
      </table>
    </div>

    <!-- Cell Details Drawer (expandable) -->
    {#if showDetailDrawer && selectedCell && result?.rows[selectedCell.rIdx]}
      {@const val = result.rows[selectedCell.rIdx][selectedCell.cIdx]}
      {@const colName = allColumns[selectedCell.cIdx]}
      <div class="h-32 bg-[#0b0f19] border-t border-[#334155] flex flex-col p-2.5 z-20 shrink-0">
        <div class="flex items-center justify-between text-[11px] text-slate-400 mb-1 select-none">
          <span class="font-bold text-slate-200">
            Cell Value: <span class="text-sky-400 font-mono">{colName}</span>
          </span>
          <button
            onclick={() => navigator.clipboard.writeText(String(val ?? ''))}
            class="hover:text-white flex items-center space-x-1"
          >
            <Icon name="copy" class="w-3 h-3" />
            <span>Copy</span>
          </button>
        </div>
        <textarea
          readonly
          class="flex-1 bg-[#090d16] border border-[#1e293b] rounded p-2 text-xs font-mono text-slate-200 resize-none outline-none"
          value={typeof val === 'object' ? JSON.stringify(val, null, 2) : String(val ?? 'NULL')}
        ></textarea>
      </div>
    {/if}

    <!-- Bottom Status & Pagination Bar -->
    <div class="h-8 bg-[#0b0f19] border-t border-[#334155] px-4 flex items-center justify-between text-xs text-slate-400 font-mono select-none shrink-0">
      <div id="grid-bottom-stats" class="flex items-center space-x-3">
        <span class="flex items-center space-x-1.5">
          <Icon name="table" class="w-3.5 h-3.5 text-sky-400" />
          {#if currentTab?.exactRowCount !== undefined}
            <span class="font-bold text-emerald-300">{currentTab.exactRowCount.toLocaleString()}</span>
            <span class="text-slate-500">exact records</span>
          {:else}
            <span class="font-bold text-slate-200">{totalRows.toLocaleString()}</span>
            <span class="text-slate-500">records</span>
          {/if}
        </span>
        {#if currentTab?.isTableViewer}
          <button
            id="btn-count-accurate"
            onclick={() => currentTab && appState.requestExactRowCount(currentTab)}
            disabled={currentTab.isCountingExact}
            class="px-1.5 py-0.5 rounded border border-[#334155] text-[10px] text-slate-400 hover:text-sky-300 hover:border-sky-700 disabled:opacity-50 transition"
            title="Run SELECT COUNT(*) for an exact total (ignores the catalog estimate)"
          >
            {currentTab.isCountingExact ? 'Counting…' : 'Exact count'}
          </button>
        {/if}
        {#if result.duration_ms !== undefined}
          <span class="text-slate-600">•</span>
          <span class="flex items-center space-x-1">
            <Icon name="clock" class="w-3 h-3 text-slate-500" />
            <span class="text-slate-300">{result.duration_ms} ms</span>
          </span>
        {/if}
      </div>

      <!-- Pagination Controls for Table Viewer -->
      {#if currentTab?.isTableViewer}
        {@const estRows =
          currentTab.exactRowCount ??
          appState.catalog?.tables?.find((t) => t.name.toLowerCase() === currentTab?.tableName?.toLowerCase())
            ?.row_count}
        {@const pageSize = currentTab.pageSize || 50}
        {@const curPage = currentTab.currentPage || 1}
        {@const totalPages = typeof estRows === 'number' && estRows > 0 ? Math.max(1, Math.ceil(estRows / pageSize)) : null}
        <div id="grid-pagination-bar" class="flex items-center space-x-2 text-[11px]">
          <span class="text-slate-500">Page Size:</span>
          <select
            id="grid-page-size-select"
            value={pageSize}
            onchange={(e) => {
              if (currentTab) {
                currentTab.pageSize = Number((e.currentTarget as HTMLSelectElement).value);
                currentTab.currentPage = 1;
                appState.loadTableData(currentTab);
              }
            }}
            class="bg-[#090d16] border border-[#334155] rounded px-1.5 py-0.5 text-slate-200 focus:outline-none"
          >
            <option value={50}>50</option>
            <option value={100}>100</option>
            <option value={250}>250</option>
            <option value={500}>500</option>
            <option value={1000}>1000</option>
          </select>

          <button
            id="btn-page-first"
            onclick={() => {
              if (currentTab && curPage > 1) {
                currentTab.currentPage = 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={curPage <= 1}
            class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
            title="First Page"
          >
            <Icon name="chevrons-left" class="w-3.5 h-3.5" />
          </button>
          <button
            id="btn-page-prev"
            onclick={() => {
              if (currentTab && curPage > 1) {
                currentTab.currentPage = curPage - 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={curPage <= 1}
            class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
            title="Previous Page"
          >
            <Icon name="chevron-left" class="w-3.5 h-3.5" />
          </button>
          <span class="flex items-center space-x-1 px-1 text-slate-300">
            <span>Page</span>
            <!-- Editable page jump: parity with the legacy pager, clamped to the valid range. -->
            <input
              id="grid-page-input"
              type="number"
              min="1"
              max={totalPages ?? undefined}
              value={curPage}
              onchange={(e) => {
                const target = Number((e.currentTarget as HTMLInputElement).value);
                if (!currentTab || !Number.isFinite(target)) return;
                const clamped = Math.max(1, Math.min(totalPages ?? target, Math.floor(target)));
                if (clamped === curPage) return;
                currentTab.currentPage = clamped;
                appState.loadTableData(currentTab);
              }}
              class="w-14 bg-[#090d16] border border-[#334155] rounded px-1 py-0.5 text-right text-slate-200 font-mono focus:outline-none focus:border-sky-500 [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
              aria-label="Jump to page"
            />
            {#if totalPages}
              <span>of <span id="grid-total-pages">{totalPages.toLocaleString()}</span></span>
            {/if}
          </span>
          <button
            id="btn-page-next"
            onclick={() => {
              if (currentTab) {
                currentTab.currentPage = curPage + 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={totalRows < pageSize || (totalPages !== null && curPage >= totalPages)}
            class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
            title="Next Page"
          >
            <Icon name="chevron-right" class="w-3.5 h-3.5" />
          </button>
          {#if totalPages !== null}
            <button
              id="btn-page-last"
              onclick={() => {
                if (currentTab && curPage < totalPages) {
                  currentTab.currentPage = totalPages;
                  appState.loadTableData(currentTab);
                }
              }}
              disabled={curPage >= totalPages}
              class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
              title="Last Page"
            >
              <Icon name="chevrons-right" class="w-3.5 h-3.5" />
            </button>
          {/if}
        </div>
      {:else if result && result.rows && (result.rows.length >= 5000 || (currentTab?.streamBatchOffset ?? 0) > 0 || currentTab?.hasMoreStream)}
        <!-- Query Stream 5k Batch Pager (ADR 0058) -->
        <div class="flex items-center space-x-2 text-[11px]">
          <span class="px-2 py-0.5 rounded bg-sky-950/80 border border-sky-800 text-sky-300 font-mono text-[10px] font-bold">
            Batch #{Math.floor((currentTab?.streamBatchOffset || 0) / 5000) + 1} (Offset {(currentTab?.streamBatchOffset || 0).toLocaleString()})
          </span>
          <button
            onclick={() => appState.pageStreamBatch('prev')}
            disabled={(currentTab?.streamBatchOffset || 0) <= 0 || isExecuting}
            class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 transition"
            title="Load previous 5,000 records"
          >
            Prev 5k
          </button>
          <button
            onclick={() => appState.pageStreamBatch('next')}
            disabled={(!currentTab?.hasMoreStream && (result.rows?.length || 0) < 5000) || isExecuting}
            class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 transition"
            title="Load next 5,000 records"
          >
            Next 5k
          </button>
        </div>
      {:else}
        <div class="text-[11px] text-slate-500">
          Double-click to edit cell • Right-click for quick filters
        </div>
      {/if}
    </div>
  {:else}
    <div class="flex-1 flex flex-col items-center justify-center p-8 space-y-3 text-slate-500 select-none">
      <Icon name="terminal" class="w-8 h-8 text-slate-600" />
      <span class="text-xs">No query results yet. Click a table in the sidebar or press Ctrl+Enter to run SQL.</span>
    </div>
  {/if}

  <!-- Cell Context Menu -->
  {#if contextMenu && contextMenu.open}
    <div
      id="grid-context-menu"
      style="top: {contextMenu.y}px; left: {contextMenu.x}px;"
      class="fixed z-50 w-56 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
    >
      <div class="px-3 py-1 text-[10px] font-bold text-slate-400 uppercase tracking-wider font-mono truncate">
        Filter: {contextMenu.colName}
      </div>

      <div id="grid-ctx-filter-parent" class="py-1">
        <div id="grid-ctx-filter-submenu">
          <div id="grid-ctx-filter-options">
        <button
          onclick={() => applyQuickFilter('=')}
          class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono font-bold text-sky-400">=</span>
          <span class="truncate">Filter = '{String(contextMenu.cellValue ?? 'NULL')}'</span>
        </button>
        <button
          onclick={() => applyQuickFilter('!=')}
          class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono font-bold text-amber-400">!=</span>
          <span class="truncate">Filter != '{String(contextMenu.cellValue ?? 'NULL')}'</span>
        </button>
        {#if contextMenu.cellValue !== null && contextMenu.cellValue !== undefined}
          <div class="grid grid-cols-4 gap-1 px-3 py-1 border-y border-slate-800/60 my-0.5">
            <button
              onclick={() => applyQuickFilter('<')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[11px] font-bold text-center"
              title="Less than"
            >&lt;</button>
            <button
              onclick={() => applyQuickFilter('<=')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[11px] font-bold text-center"
              title="Less or equal"
            >&le;</button>
            <button
              onclick={() => applyQuickFilter('>')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[11px] font-bold text-center"
              title="Greater than"
            >&gt;</button>
            <button
              onclick={() => applyQuickFilter('>=')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[11px] font-bold text-center"
              title="Greater or equal"
            >&ge;</button>
          </div>
          <button
            onclick={() => applyQuickFilter('LIKE')}
            class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
          >
            <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono font-bold text-purple-400">%</span>
            <span class="truncate">Filter LIKE '%{String(contextMenu.cellValue)}%'</span>
          </button>
          <button
            onclick={() => applyQuickFilter('NOT LIKE')}
            class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
          >
            <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono font-bold text-rose-400">!%</span>
            <span class="truncate">Filter NOT LIKE '%{String(contextMenu.cellValue)}%'</span>
          </button>
        {/if}
        <button
          onclick={() => applyQuickFilter('IS NULL')}
          class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-rose-400">∅</span>
          <span>Filter IS NULL</span>
        </button>
        <button
          onclick={() => applyQuickFilter('IS NOT NULL')}
          class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-emerald-400">!∅</span>
          <span>Filter IS NOT NULL</span>
        </button>
      </div>

          </div>
        </div>
      </div>

      <div id="grid-ctx-mutations-parent" class="py-1">
        <div id="grid-ctx-table-mutations">
          <div id="grid-ctx-insert-section">
            <div id="grid-ctx-insert-options">
        {#if currentTab?.tableName}
          <button
            id="grid-ctx-btn-dup-nokey"
            onclick={() => {
              duplicateRowAsDraft(contextMenu!.rIdx, true);
              closeContextMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-emerald-300 font-medium"
          >
            <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
            <span>Duplicate Row (Without Key)</span>
          </button>
          <button
            id="grid-ctx-btn-dup-key"
            onclick={() => {
              duplicateRowAsDraft(contextMenu!.rIdx, false);
              closeContextMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-indigo-300 font-medium"
          >
            <Icon name="key" class="w-3.5 h-3.5 text-indigo-400 shrink-0" />
            <span>Duplicate Row (With Key)</span>
          </button>
          <button
            id="grid-ctx-btn-add-draft"
            onclick={() => {
              addDraftRow();
              closeContextMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-emerald-300"
          >
            <Icon name="plus" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
            <span>Add Row Draft</span>
          </button>
          <button
            id="grid-ctx-btn-delete-row"
            onclick={() => {
              handleDeleteCurrentRow();
              closeContextMenu();
            }}
            disabled={selectedRowIndex === null}
            class="w-full text-left px-3 py-1.5 hover:bg-rose-950/60 disabled:opacity-40 flex items-center space-x-2 text-rose-300"
          >
            <Icon name="trash-2" class="w-3.5 h-3.5 text-rose-400 shrink-0" />
            <span>Delete Selected Row(s)</span>
          </button>
        {/if}
          </div>
        </div>
      </div>

        <button
          onclick={() => {
            const { rIdx, cIdx, colName, cellValue } = contextMenu!;
            closeContextMenu();
            appState.openJsonCellModal(currentTab?.tableName, rIdx, cIdx, colName, cellValue, !currentTab?.tableName);
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="code" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
          <span>Open in JSON Editor</span>
        </button>
        <button
          onclick={() => {
            appState.recordInspectorRowIndex = contextMenu!.rIdx;
            appState.isRecordInspectorOpen = true;
            closeContextMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="eye" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
          <span>Inspect Record</span>
        </button>
        <button
          onclick={() => {
            navigator.clipboard.writeText(String(contextMenu!.cellValue ?? ''));
            appState.showToast('Copied cell value to clipboard', 'info');
            closeContextMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="copy" class="w-3.5 h-3.5 text-slate-400 shrink-0" />
          <span>Copy Cell Value</span>
        </button>
      </div>

      <div id="grid-ctx-copy-parent" class="py-1">
        <div id="grid-ctx-copy-submenu">
        <button
          id="grid-ctx-btn-copy-row-tsv"
          onclick={() => {
            copyRowAsTSV(contextMenu!.rIdx);
            closeContextMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="file-text" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
          <span>Copy Row as TSV</span>
        </button>
        <button
          id="grid-ctx-btn-copy-row-json"
          onclick={() => {
            copyRowAsMarkdown(contextMenu!.rIdx);
            closeContextMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="share-2" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
          <span>Copy Row as Markdown</span>
        </button>
        <button
          id="grid-ctx-btn-copy-row-sql"
          onclick={() => {
            copyRowAsInsertSQL(contextMenu!.rIdx);
            closeContextMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="code" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
          <span>Copy as INSERT SQL</span>
        </button>
      </div>
    </div>
  {/if}
</div>
