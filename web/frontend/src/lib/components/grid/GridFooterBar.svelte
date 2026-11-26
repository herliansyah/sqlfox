<script lang="ts">
  import type { EditorTab, QueryExecutionResult } from '../../../types';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let {
    currentTab,
    result,
    totalRows,
    tableEffectiveTotal,
    tableTotalIsExact,
    isExecuting,
  }: {
    currentTab: EditorTab | undefined;
    result: QueryExecutionResult;
    totalRows: number;
    tableEffectiveTotal: number | null | undefined;
    tableTotalIsExact: boolean;
    isExecuting: boolean;
  } = $props();
</script>

<div class="h-8 bg-[#0b0f19] border-t border-[#334155] px-4 flex items-center justify-between text-xs text-slate-400 font-mono select-none shrink-0">
  <div id="grid-bottom-stats" class="flex items-center space-x-3">
    <span class="flex items-center space-x-1.5">
      <Icon name="table" class="w-3.5 h-3.5 text-sky-400" />
      {#if currentTab?.isTableViewer && tableEffectiveTotal !== undefined && tableEffectiveTotal !== null}
        {@const curPage = currentTab.currentPage || 1}
        {@const pageSize = currentTab.pageSize || 25}
        <span class="font-bold text-slate-200">
          {totalRows > 0 ? `${((curPage - 1) * pageSize + 1).toLocaleString()}–${((curPage - 1) * pageSize + totalRows).toLocaleString()}` : '0'}
        </span>
        <span class="text-slate-500">
          of {tableTotalIsExact ? '' : '~'}{tableEffectiveTotal.toLocaleString()} records
        </span>
      {:else}
        <span class="font-bold text-slate-200">{totalRows.toLocaleString()}</span>
        <span class="text-slate-500">records</span>
        {#if currentTab?.manualTotalRows !== undefined && currentTab?.manualTotalRows !== null}
          <span class="text-slate-600">•</span>
          <span class="text-slate-400">Total:</span>
          <span class="font-bold text-emerald-400">{currentTab.manualTotalRows.toLocaleString()}</span>
          <span class="text-slate-500">records</span>
        {:else if !currentTab?.isTableViewer && (totalRows > 0 || currentTab?.hasMoreStream)}
          <button
            onclick={() => appState.countQueryTotal(currentTab)}
            disabled={currentTab?.isCountingTotal || isExecuting}
            class="ml-1.5 px-2 py-0.5 rounded bg-slate-900 hover:bg-slate-800 disabled:opacity-50 text-[10px] text-sky-400 hover:text-sky-300 border border-slate-700/80 transition flex items-center space-x-1"
            title="Calculate exact total rows for this query via SELECT COUNT(*)"
          >
            <Icon name={currentTab?.isCountingTotal ? 'refresh-cw' : 'hash'} class="w-3 h-3 {currentTab?.isCountingTotal ? 'animate-spin text-sky-300' : ''}" />
            <span>{currentTab?.isCountingTotal ? 'Counting...' : 'Count Total'}</span>
          </button>
        {/if}
      {/if}
    </span>
    {#if result.duration_ms !== undefined}
      <span class="text-slate-600">•</span>
      <span class="flex items-center space-x-1">
        <Icon name="clock" class="w-3 h-3 text-slate-500" />
        <span class="text-slate-300">{result.duration_ms} ms</span>
      </span>
    {/if}
  </div>

  {#if currentTab?.isTableViewer}
    {@const pageSize = currentTab.pageSize || 25}
    {@const curPage = currentTab.currentPage || 1}
    {@const totalPages = typeof tableEffectiveTotal === 'number' && tableEffectiveTotal >= 0 ? Math.max(1, Math.ceil(tableEffectiveTotal / pageSize)) : null}
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
        <option value={25}>25</option>
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
        <input
          id="grid-page-input"
          type="number"
          min="1"
          max={tableTotalIsExact ? (totalPages ?? undefined) : undefined}
          value={curPage}
          onkeydown={(e) => {
            if (e.key === 'Escape') {
              (e.currentTarget as HTMLInputElement).value = String(curPage);
              (e.currentTarget as HTMLInputElement).blur();
            }
          }}
          onchange={(e) => {
            const target = Number((e.currentTarget as HTMLInputElement).value);
            if (!currentTab || !Number.isFinite(target)) return;
            const clamped = Math.max(1, tableTotalIsExact && totalPages !== null ? Math.min(totalPages, Math.floor(target)) : Math.floor(target));
            if (clamped === curPage) return;
            currentTab.currentPage = clamped;
            appState.loadTableData(currentTab);
          }}
          class="w-14 bg-[#090d16] border border-[#334155] rounded px-1 py-0.5 text-right text-slate-200 font-mono focus:outline-none focus:border-sky-500 [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
          aria-label="Jump to page"
        />
        {#if totalPages}
          <span>of <span id="grid-total-pages">{tableTotalIsExact ? '' : '~'}{totalPages.toLocaleString()}</span></span>
        {:else if totalRows >= pageSize}
          <span class="text-slate-400">({curPage}+)</span>
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
        disabled={totalRows < pageSize || (tableTotalIsExact && totalPages !== null && curPage >= totalPages)}
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
          disabled={tableTotalIsExact ? curPage >= totalPages : totalRows < pageSize}
          class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
          title="Last Page"
        >
          <Icon name="chevrons-right" class="w-3.5 h-3.5" />
        </button>
      {/if}
    </div>
  {:else if result && result.rows && (result.rows.length >= (currentTab?.queryLimit || 1000) || (currentTab?.streamBatchOffset ?? 0) > 0 || currentTab?.hasMoreStream)}
    {@const streamOff = currentTab?.streamBatchOffset || 0}
    {@const batchLimit = currentTab?.queryLimit || 1000}
    {@const batchLabel = batchLimit >= 1000 ? `${batchLimit / 1000}k` : `${batchLimit}`}
    <div class="flex items-center space-x-2 text-[11px]">
      <span class="px-2 py-0.5 rounded bg-sky-950/80 border border-sky-800 text-sky-300 font-mono text-[10px] font-bold">
        Batch #{Math.floor(streamOff / batchLimit) + 1} (Rows {(streamOff + 1).toLocaleString()}–{(streamOff + totalRows).toLocaleString()})
      </span>
      <button
        onclick={() => appState.pageStreamBatch('prev')}
        disabled={(currentTab?.streamBatchOffset || 0) <= 0 || isExecuting}
        class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 transition"
        title={`Load previous ${batchLabel} records`}
      >
        Prev {batchLabel}
      </button>
      <button
        onclick={() => appState.pageStreamBatch('next')}
        disabled={(!currentTab?.hasMoreStream && (result.rows?.length || 0) < batchLimit) || isExecuting}
        class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 transition"
        title={`Load next ${batchLabel} records`}
      >
        Next {batchLabel}
      </button>
    </div>
  {:else}
    <div class="text-[11px] text-slate-500">
      Double-click to edit cell • Right-click for quick filters
    </div>
  {/if}
</div>
