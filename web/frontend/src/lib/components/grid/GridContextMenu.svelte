<script lang="ts">
  import Icon from '../common/Icon.svelte';
  import { appState } from '../../stores/appState.svelte';
  import { copyText } from '../../utils/clipboard';
  import { classifyColumnType, resolveEnumValues } from '../../utils/dataTypes';
  import { viewportFit } from '../../actions/viewportFit';
  import type { FilterPill, QueryExecutionResult, TableMetadata } from '../../../types';

  export interface GridContextMenuState {
    open: boolean;
    x: number;
    y: number;
    rIdx: number;
    cIdx: number;
    colName: string;
    cellValue: any;
  }

  let {
    contextMenu,
    result,
    tableMeta,
    effectiveTable,
    canEdit,
    selectedRowIndices,
    isColNullable,
    resolveColType,
    closeContextMenu,
    applyQuickFilter,
    startCellEdit,
    applyEditPreset,
    duplicateMultipleRows,
    duplicateRowAsDraft,
    addDraftRow,
    handleBatchDelete,
    handleDeleteRowAt,
    copySelectedRows,
    copyRowAsTSV,
    copyRowAsJSON,
    copyRowAsMarkdown,
    copyRowAsInsertSQL,
  }: {
    contextMenu: GridContextMenuState;
    result: QueryExecutionResult | null;
    tableMeta: TableMetadata | undefined;
    effectiveTable: string;
    canEdit: boolean;
    selectedRowIndices: number[];
    isColNullable: (col: any) => boolean;
    resolveColType: (col: any) => string;
    closeContextMenu: () => void;
    applyQuickFilter: (op: FilterPill['op']) => void;
    startCellEdit: (rIdx: number, cIdx: number, val: any) => void;
    applyEditPreset: (preset: 'NULL' | 'EMPTY' | 'NOW' | 'DATE' | 'UUID', rIdx: number, cIdx: number) => void;
    duplicateMultipleRows: (indices: number[]) => void;
    duplicateRowAsDraft: (rIdx: number, withoutKey: boolean) => void;
    addDraftRow: () => void;
    handleBatchDelete: () => void;
    handleDeleteRowAt: (rIdx: number) => void;
    copySelectedRows: (fmt: 'tsv' | 'json' | 'md' | 'sql') => void;
    copyRowAsTSV: (rIdx: number) => void;
    copyRowAsJSON: (rIdx: number) => void;
    copyRowAsMarkdown: (rIdx: number) => void;
    copyRowAsInsertSQL: (rIdx: number) => void;
  } = $props();
</script>

<div
  id="grid-context-menu"
  use:viewportFit={{ x: contextMenu.x, y: contextMenu.y }}
  style="top: {contextMenu.y}px; left: {contextMenu.x}px;"
  class="fixed z-50 w-60 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
>
  <div class="px-3 py-1 text-[10px] font-bold text-slate-400 uppercase tracking-wider font-mono truncate">
    Filter: {contextMenu.colName}
  </div>

  <div id="grid-ctx-filter-parent" class="py-1">
    <div id="grid-ctx-filter-submenu">
      <div id="grid-ctx-filter-options" class="space-y-0.5">
        <button
          onclick={() => applyQuickFilter('=')}
          class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-4 text-center flex-shrink-0 font-mono font-bold text-sky-400">=</span>
          <span class="truncate">Filter = '{String(contextMenu.cellValue ?? 'NULL')}'</span>
        </button>
        <button
          onclick={() => applyQuickFilter('!=')}
          class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <span class="w-4 text-center flex-shrink-0 font-mono font-bold text-amber-400">!=</span>
          <span class="truncate">Filter != '{String(contextMenu.cellValue ?? 'NULL')}'</span>
        </button>
        {#if contextMenu.cellValue !== null && contextMenu.cellValue !== undefined}
          <div class="grid grid-cols-6 gap-1 px-3 py-1">
            <button
              onclick={() => applyQuickFilter('<')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
              title="Less than"
            >&lt;</button>
            <button
              onclick={() => applyQuickFilter('<=')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
              title="Less or equal"
            >&le;</button>
            <button
              onclick={() => applyQuickFilter('>')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
              title="Greater than"
            >&gt;</button>
            <button
              onclick={() => applyQuickFilter('>=')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
              title="Greater or equal"
            >&ge;</button>
            <button
              onclick={() => applyQuickFilter('LIKE')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-purple-300 font-mono text-[10px] font-bold text-center"
              title="Filter LIKE '%{String(contextMenu.cellValue)}%'"
            >%</button>
            <button
              onclick={() => applyQuickFilter('NOT LIKE')}
              class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-rose-300 font-mono text-[10px] font-bold text-center"
              title="Filter NOT LIKE '%{String(contextMenu.cellValue)}%'"
            >!%</button>
          </div>
        {/if}
        <div class="px-3 py-1 flex items-center justify-between gap-1.5">
          <span class="text-[10px] text-slate-500">Null check</span>
          <div class="flex items-center gap-1 text-[10px] font-mono">
            <button
              onclick={() => applyQuickFilter('IS NULL')}
              class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-rose-300 font-medium"
            >IS NULL</button>
            <button
              onclick={() => applyQuickFilter('IS NOT NULL')}
              class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium"
            >NOT NULL</button>
          </div>
        </div>
      </div>
    </div>
  </div>

  <div id="grid-ctx-mutations-parent" class="py-1">
    <div id="grid-ctx-table-mutations">
      <div id="grid-ctx-insert-section">
        <div id="grid-ctx-insert-options">
          {#if canEdit}
            {@const ctxColDef = result?.columns?.[contextMenu.cIdx]}
            {@const ctxNullable = ctxColDef ? isColNullable(ctxColDef) : true}
            {@const ctxEnumVals = ctxColDef ? resolveEnumValues(ctxColDef, tableMeta) : []}
            {@const ctxKind = classifyColumnType(ctxColDef ? resolveColType(ctxColDef) : '', ctxEnumVals)}
            {@const ctxShowUuid = ctxKind.isUuid || (ctxKind.isString && (!ctxKind.maxLength || ctxKind.maxLength >= 36))}
            {@const ctxHasPresets = ctxNullable || ctxKind.isDateTime || ctxKind.isDate || ctxShowUuid}
            <button
              onclick={() => {
                const { rIdx, cIdx, cellValue } = contextMenu;
                closeContextMenu();
                startCellEdit(rIdx, cIdx, cellValue);
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-sky-300 font-medium"
            >
              <Icon name="edit" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
              <span>Edit Cell (Inline)</span>
            </button>
            {#if ctxHasPresets}
              <div class="px-3 py-1 border-y border-slate-800/60 flex items-center justify-between gap-1 text-[10px] font-mono">
                <span class="text-slate-500">⚡ Insert:</span>
                <div class="flex items-center gap-1">
                  {#if ctxNullable}
                    <button onclick={() => { const { rIdx, cIdx } = contextMenu; closeContextMenu(); applyEditPreset('NULL', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-rose-300">NULL</button>
                  {/if}
                  {#if ctxKind.isDateTime}
                    <button onclick={() => { const { rIdx, cIdx } = contextMenu; closeContextMenu(); applyEditPreset('NOW', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-sky-300">NOW</button>
                  {/if}
                  {#if ctxKind.isDate || ctxKind.isDateTime}
                    <button onclick={() => { const { rIdx, cIdx } = contextMenu; closeContextMenu(); applyEditPreset('DATE', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300">TODAY</button>
                  {/if}
                  {#if ctxShowUuid}
                    <button onclick={() => { const { rIdx, cIdx } = contextMenu; closeContextMenu(); applyEditPreset('UUID', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-purple-300">UUID</button>
                  {/if}
                </div>
              </div>
            {/if}
            {#if selectedRowIndices.length > 1}
              <button
                id="grid-ctx-btn-dup-nokey"
                onclick={() => {
                  const indices = [...selectedRowIndices];
                  closeContextMenu();
                  duplicateMultipleRows(indices);
                }}
                class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-emerald-300 font-medium"
              >
                <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span>Duplicate {selectedRowIndices.length} Rows</span>
              </button>
            {:else}
              <div class="px-3 py-1 flex items-center justify-between gap-1.5">
                <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
                  <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                  <span>Duplicate</span>
                </span>
                <div class="flex items-center gap-1 text-[10px]">
                  <button
                    id="grid-ctx-btn-dup-nokey"
                    onclick={() => {
                      duplicateRowAsDraft(contextMenu.rIdx, true);
                      closeContextMenu();
                    }}
                    class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium"
                    title="Duplicate Row (Without Key)"
                  >No Key</button>
                  <button
                    id="grid-ctx-btn-dup-key"
                    onclick={() => {
                      duplicateRowAsDraft(contextMenu.rIdx, false);
                      closeContextMenu();
                    }}
                    class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-indigo-300 font-medium"
                    title="Duplicate Row (With Key)"
                  >With Key</button>
                </div>
              </div>
            {/if}
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
                if (selectedRowIndices.length > 1) {
                  closeContextMenu();
                  handleBatchDelete();
                } else {
                  handleDeleteRowAt(contextMenu.rIdx);
                  closeContextMenu();
                }
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-rose-950/60 flex items-center space-x-2 text-rose-300"
            >
              <Icon name="trash-2" class="w-3.5 h-3.5 text-rose-400 shrink-0" />
              <span>{selectedRowIndices.length > 1 ? `Delete ${selectedRowIndices.length} Selected Rows` : 'Delete Selected Row(s)'}</span>
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>

  <div class="py-1">
    <button
      onclick={() => {
        const { rIdx, cIdx, colName, cellValue } = contextMenu;
        closeContextMenu();
        appState.openJsonCellModal(effectiveTable || undefined, rIdx, cIdx, colName, cellValue, !canEdit);
      }}
      class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
    >
      <Icon name="code" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
      <span>Open in JSON Editor</span>
    </button>
    <button
      onclick={() => {
        appState.recordInspectorRowIndex = contextMenu.rIdx;
        appState.isRecordInspectorOpen = true;
        closeContextMenu();
      }}
      class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
    >
      <Icon name="eye" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
      <span>Inspect Record</span>
    </button>
    <button
      id="grid-ctx-btn-copy-cell"
      onclick={() => {
        const cv = contextMenu.cellValue;
        copyText(cv === null || cv === undefined ? '' : typeof cv === 'object' ? JSON.stringify(cv, null, 2) : String(cv));
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
    <div id="grid-ctx-copy-submenu" class="px-3 py-1 flex items-center justify-between gap-1.5">
      <span class="text-[11px] text-slate-400 shrink-0">
        {selectedRowIndices.length > 1 ? `Copy ${selectedRowIndices.length}` : 'Copy Row'}
      </span>
      <div class="flex items-center gap-1 text-[10px] font-mono">
        <button
          id="grid-ctx-btn-copy-row-tsv"
          onclick={() => {
            if (selectedRowIndices.length > 1) copySelectedRows('tsv');
            else copyRowAsTSV(contextMenu.rIdx);
            closeContextMenu();
          }}
          class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium"
          title="Copy as TSV"
        >TSV</button>
        <button
          id="grid-ctx-btn-copy-row-json"
          onclick={() => {
            if (selectedRowIndices.length > 1) copySelectedRows('json');
            else copyRowAsJSON(contextMenu.rIdx);
            closeContextMenu();
          }}
          class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-amber-300 font-medium"
          title="Copy as JSON"
        >JSON</button>
        <button
          id="grid-ctx-btn-copy-row-md"
          onclick={() => {
            if (selectedRowIndices.length > 1) copySelectedRows('md');
            else copyRowAsMarkdown(contextMenu.rIdx);
            closeContextMenu();
          }}
          class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium"
          title="Copy as Markdown"
        >MD</button>
        <button
          id="grid-ctx-btn-copy-row-sql"
          onclick={() => {
            if (selectedRowIndices.length > 1) copySelectedRows('sql');
            else copyRowAsInsertSQL(contextMenu.rIdx);
            closeContextMenu();
          }}
          class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-purple-300 font-medium"
          title="Copy as INSERT SQL"
        >SQL</button>
      </div>
    </div>
  </div>
</div>
