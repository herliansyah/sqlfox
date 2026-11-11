<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import type { ColumnDefinition, FilterPill } from '../../../types';
  import Icon from '../common/Icon.svelte';

  const ROW_HEIGHT = 28;
  const OVERSCAN = 15;

  let containerRef: HTMLDivElement | null = $state(null);
  let scrollTop = $state(0);
  let containerHeight = $state(500);

  let currentTab = $derived(appState.activeTab);
  let result = $derived(currentTab?.activeResult);
  let isExecuting = $derived(currentTab?.isExecuting);

  // Sorting
  let sortCol = $state<string | null>(null);
  let sortDir = $state<'ASC' | 'DESC'>('ASC');

  // Selected row / cell & Context Menu
  let selectedRowIndex = $state<number | null>(null);
  let selectedCell = $state<{ rIdx: number; cIdx: number } | null>(null);
  let editingCell = $state<{ rIdx: number; cIdx: number; tempValue: string } | null>(null);

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
      const pkVal = pkColObj ? result.rows[origIdx][allColumns.indexOf(pkCol)] : undefined;

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
    if (confirm(`Delete record with ${pkCol} = ${pkVal}?`)) {
      await appState.deleteRows(pkCol, [pkVal]);
      selectedRowIndex = null;
    }
  }
</script>

<svelte:window onclick={closeContextMenu} />

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text relative">
  <!-- Grid Control Toolbar -->
  <div class="h-9 bg-[#0b0f19] border-b border-[#334155] px-3 flex items-center justify-between text-xs select-none shrink-0 gap-2">
    <!-- Left: CRUD & Staged Actions -->
    <div class="flex items-center space-x-2">
      {#if currentTab?.isTableViewer}
        <button
          onclick={() => (appState.isInsertRowModalOpen = true)}
          class="h-6 px-2.5 rounded bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-300 border border-emerald-500/40 font-semibold flex items-center space-x-1.5 transition text-[11px]"
          title="Insert new record"
        >
          <Icon name="plus" class="w-3 h-3 text-emerald-400" />
          <span>Insert Row</span>
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
          onclick={(e) => { e.stopPropagation(); showColPicker = !showColPicker; }}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-[#334155] flex items-center space-x-1 text-[11px] transition"
          title="Toggle column visibility"
        >
          <Icon name="columns" class="w-3 h-3 text-sky-400" />
          <span>Columns ({visibleColumns.length}/{allColumns.length})</span>
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
            class="absolute right-0 mt-1 w-56 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-40 p-2 text-xs space-y-2 animate-in fade-in duration-100"
          >
            <input
              type="text"
              bind:value={colPickerSearch}
              placeholder="Search columns..."
              class="w-full bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-xs text-slate-100 font-mono focus:outline-none"
            />
            <div class="flex items-center justify-between text-[10px] text-sky-400 px-1">
              <button onclick={() => appState.showAllColumns()} class="hover:underline">Show All</button>
              <button onclick={() => appState.hideAllColumns(allColumns)} class="hover:underline">Hide All</button>
            </div>
            <div class="max-h-48 overflow-y-auto space-y-1 py-1 divide-y divide-slate-800/40">
              {#each allColumns as col}
                {#if !colPickerSearch || col.toLowerCase().includes(colPickerSearch.toLowerCase())}
                  <label class="flex items-center space-x-2 py-1 px-1 rounded hover:bg-slate-800 cursor-pointer text-slate-300 font-mono text-[11px]">
                    <input
                      type="checkbox"
                      checked={!hiddenCols.includes(col)}
                      onchange={() => appState.toggleColumnHidden(col)}
                      class="rounded bg-[#090d16] border-[#334155] text-sky-500 focus:ring-0"
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
      <div class="relative">
        <button
          onclick={(e) => { e.stopPropagation(); showExportMenu = !showExportMenu; }}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-[#334155] flex items-center space-x-1 text-[11px] transition"
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
            class="absolute right-0 mt-1 w-44 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-40 py-1 divide-y divide-slate-800 animate-in fade-in duration-100 text-xs"
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
        class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 border border-[#334155] flex items-center space-x-1 text-[11px] transition"
        title="Inspect record in vertical form"
      >
        <Icon name="eye" class="w-3 h-3 text-sky-400" />
        <span>Inspect</span>
      </button>

      <!-- Cell Drawer Toggle -->
      <button
        onclick={() => (showDetailDrawer = !showDetailDrawer)}
        class="h-6 px-2 rounded {showDetailDrawer ? 'bg-sky-500/20 text-sky-300 border-sky-500/40' : 'bg-[#1e293b] text-slate-400 hover:text-white'} border border-[#334155] flex items-center space-x-1 text-[11px] transition"
        title="Toggle Cell Drawer"
      >
        <Icon name="sidebar" class="w-3 h-3" />
        <span>Drawer</span>
      </button>
    </div>
  </div>

  <!-- Filter Pills Bar (if filters active) -->
  {#if currentTab?.filterPills && currentTab.filterPills.length > 0}
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
              class="text-sky-400 hover:text-white ml-1 p-0.5"
            >
              ×
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
            <th class="w-12 px-2.5 py-2 text-center text-[10px] text-slate-500 bg-[#090d16] border-r border-[#1e293b] font-medium sticky left-0 z-30">
              #
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
                    <span class="text-sky-400 font-bold ml-1">
                      {sortDir === 'ASC' ? '▲' : '▼'}
                    </span>
                  {/if}
                </div>
              </th>
            {/each}
          </tr>
        </thead>

        <!-- Virtual Rows Body -->
        <tbody>
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
              <tr
                onclick={() => (selectedRowIndex = rowItem.origIdx)}
                class="border-b border-[#1e293b]/60 transition-colors h-7 {
                  isRowSelected ? 'bg-sky-500/15' : rowItem.displayIdx % 2 === 1 ? 'bg-slate-900/20' : 'bg-transparent'
                } hover:bg-sky-500/10"
              >
                <!-- Row Gutter -->
                <td
                  ondblclick={() => {
                    appState.recordInspectorRowIndex = rowItem.origIdx;
                    appState.isRecordInspectorOpen = true;
                  }}
                  class="w-12 px-2.5 py-1 text-center text-[10px] text-slate-500 bg-[#090d16] border-r border-[#1e293b] select-none font-mono sticky left-0 z-10 cursor-pointer hover:text-sky-400"
                  title="Double-click to open Record Inspector"
                >
                  {rowItem.displayIdx + 1}
                </td>

                <!-- Cells -->
                {#each visibleColumns as col}
                  {@const colName = getColName(col)}
                  {@const cIdx = allColumns.indexOf(colName)}
                  {@const cellVal = rowItem.data[cIdx]}
                  {@const formatted = formatCellValue(cellVal)}
                  {@const isStaged = currentTab?.tableName ? isCellStaged(currentTab.tableName, rowItem.origIdx, cIdx) : false}
                  {@const isEditingThis = editingCell && editingCell.rIdx === rowItem.origIdx && editingCell.cIdx === cIdx}

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
                      <span class="truncate block">{formatted.display}</span>
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
      <div class="flex items-center space-x-3">
        <span class="flex items-center space-x-1.5">
          <Icon name="table" class="w-3.5 h-3.5 text-sky-400" />
          <span class="font-bold text-slate-200">{totalRows.toLocaleString()}</span>
          <span class="text-slate-500">records</span>
        </span>
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
        <div class="flex items-center space-x-2 text-[11px]">
          <span class="text-slate-500">Page Size:</span>
          <select
            value={currentTab.pageSize || 100}
            onchange={(e) => {
              if (currentTab) {
                currentTab.pageSize = Number((e.currentTarget as HTMLSelectElement).value);
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
            onclick={() => {
              if (currentTab && (currentTab.currentPage || 1) > 1) {
                currentTab.currentPage = (currentTab.currentPage || 1) - 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={(currentTab?.currentPage || 1) <= 1}
            class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200"
          >
            Prev
          </button>
          <span class="px-1 text-slate-300">Page {currentTab?.currentPage || 1}</span>
          <button
            onclick={() => {
              if (currentTab) {
                currentTab.currentPage = (currentTab.currentPage || 1) + 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={totalRows < (currentTab?.pageSize || 100)}
            class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200"
          >
            Next
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
      style="top: {contextMenu.y}px; left: {contextMenu.x}px;"
      class="fixed z-50 w-52 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
    >
      <div class="px-3 py-1 text-[10px] font-bold text-slate-400 uppercase tracking-wider font-mono truncate">
        Filter: {contextMenu.colName}
      </div>

      <div class="py-1">
        <button
          onclick={() => applyQuickFilter('=')}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono font-bold text-sky-400">=</span>
          <span class="truncate">Filter = '{String(contextMenu.cellValue ?? 'NULL')}'</span>
        </button>
        <button
          onclick={() => applyQuickFilter('!=')}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono font-bold text-amber-400">!=</span>
          <span class="truncate">Filter != '{String(contextMenu.cellValue ?? 'NULL')}'</span>
        </button>
        {#if contextMenu.cellValue !== null && contextMenu.cellValue !== undefined}
          <button
            onclick={() => applyQuickFilter('LIKE')}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
          >
            <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono font-bold text-purple-400">%</span>
            <span class="truncate">Filter LIKE '%{String(contextMenu.cellValue)}%'</span>
          </button>
        {/if}
        <button
          onclick={() => applyQuickFilter('IS NULL')}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-rose-400">∅</span>
          <span>Filter IS NULL</span>
        </button>
      </div>

      <div class="py-1">
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
    </div>
  {/if}
</div>
