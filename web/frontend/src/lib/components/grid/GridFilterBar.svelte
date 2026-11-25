<script lang="ts">
  import type { EditorTab, FilterPill } from '../../../types';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let {
    currentTab,
    allColumns,
    hasActiveFilter,
    hasFilterEntries,
    onToggleFilterBar,
  }: {
    currentTab: EditorTab | undefined;
    allColumns: string[];
    hasActiveFilter: boolean;
    hasFilterEntries: boolean;
    onToggleFilterBar: () => void;
  } = $props();
</script>

<div class="min-h-9 bg-[#0b0f19] border-b border-slate-800 px-3 py-1.5 flex flex-wrap items-center justify-between gap-2 text-xs select-none shrink-0 animate-in fade-in duration-100">
  <div class="flex flex-wrap items-center gap-2 flex-1 min-w-0">
    {#if currentTab?.isTableViewer}
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
    {:else}
      <span class="px-1.5 py-0.5 bg-sky-950/80 text-sky-400 border border-sky-800/80 rounded font-mono text-[10px] font-bold tracking-wider select-none shrink-0">
        FILTER
      </span>
    {/if}

    {#if currentTab?.isTableViewer && currentTab.filterMode === 'raw'}
      <div class="flex-1 flex items-center space-x-1.5 min-w-[260px]">
        <span class="px-1.5 py-0.5 bg-sky-950 text-sky-400 border border-sky-800 rounded font-mono text-[10px] font-bold tracking-wider select-none shrink-0">
          WHERE
        </span>
        <div class="relative flex-1 flex items-center">
          <input
            id="table-filter-raw-input"
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
      <div class="flex flex-wrap items-center gap-1.5">
        {#each currentTab?.filterPills || [] as pill, idx}
          {@const isUnary = pill.op === 'IS NULL' || pill.op === 'IS NOT NULL'}
          <div class="inline-flex items-center space-x-1 bg-[#090d16] border border-sky-800/80 rounded px-1.5 py-0.5 text-[11px] font-mono shadow-sm">
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

            {#if !isUnary}
              <input
                id="grid-filter-pill-val-{idx}"
                type="text"
                value={pill.val}
                oninput={(e) => {
                  appState.updateFilterPill(idx, { val: (e.target as HTMLInputElement).value });
                }}
                onkeydown={(e) => {
                  if (e.key === 'Enter' && currentTab?.isTableViewer) appState.applyTableFilter();
                }}
                placeholder={pill.op === 'LIKE' || pill.op === 'NOT LIKE' ? 'substring or %' : (pill.op === 'IN' || pill.op === 'NOT IN' ? '1, 2, 3' : 'val')}
                class="bg-transparent text-slate-100 font-mono text-[11px] outline-none px-1 py-0.5 w-20 focus:w-36 transition-all border-b border-transparent focus:border-sky-400"
              />
            {/if}

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

        <button
          type="button"
          onclick={() => appState.addFilterPill()}
          class="text-xs text-slate-400 hover:text-sky-300 hover:bg-slate-800 px-2 py-0.5 rounded border border-dashed border-slate-700 transition flex items-center space-x-1"
          title="Add condition (+ AND)"
        >
          <Icon name="plus" class="w-3 h-3 text-sky-400" />
          <span>AND</span>
        </button>

        {#if currentTab?.isTableViewer && currentTab.filterPills && currentTab.filterPills.length > 0}
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

    {#if currentTab?.isTableViewer ? currentTab.appliedFilter : hasActiveFilter}
      <span class="px-2 py-0.5 rounded text-[10px] font-medium bg-emerald-950 text-emerald-300 border border-emerald-700 flex items-center space-x-1" title="Active filter applied">
        <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
        <span>Filtered</span>
      </span>
    {/if}
  </div>

  <div class="flex items-center space-x-2 shrink-0">
    {#if hasFilterEntries}
      <button
        type="button"
        onclick={() => appState.clearTableFilter()}
        class="text-[11px] text-slate-400 hover:text-rose-400 transition underline"
        title="Clear all filters"
      >
        Clear Filter
      </button>
    {/if}

    {#if currentTab?.isTableViewer}
      <button
        type="button"
        onclick={() => appState.handoffTableQueryToEditor()}
        class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-sky-400 hover:text-sky-300 border border-[#334155] flex items-center space-x-1 text-[11px] transition font-mono"
        title="Open filtered query in full SQL Editor"
      >
        <Icon name="file-code" class="w-3 h-3" />
        <span>SQL</span>
      </button>
    {/if}

    <button
      type="button"
      onclick={onToggleFilterBar}
      class="h-6 w-6 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-400 hover:text-white border border-slate-700/80 hover:border-slate-600 flex items-center justify-center transition"
      title="Hide filter bar"
    >
      <Icon name="x" class="w-3.5 h-3.5" />
    </button>
  </div>
</div>
