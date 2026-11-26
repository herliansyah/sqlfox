<script lang="ts">
  import Icon from '../common/Icon.svelte';
  import { viewportFit } from '../../actions/viewportFit';
  import { appState } from '../../stores/appState.svelte';

  let {
    allColumns,
    visibleCount,
    hiddenCols,
  }: {
    allColumns: string[];
    visibleCount: number;
    hiddenCols: string[];
  } = $props();

  let showColPicker = $state(false);
  let colPickerSearch = $state('');
</script>

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
    <span>Columns (<span id="col-picker-count-badge">{visibleCount}</span>/{allColumns.length})</span>
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
      use:viewportFit
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
