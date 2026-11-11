<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  function jumpToTable() {
    const refTable = appState.fkPeekState.refTable;
    appState.closeFkPeek();
    if (refTable) {
      appState.openTableViewer(refTable);
    }
  }

  function handleOutsideClick(e: MouseEvent) {
    const el = document.getElementById('fk-peek-popover-box');
    if (el && !el.contains(e.target as Node)) {
      appState.closeFkPeek();
    }
  }
</script>

<svelte:window onclick={handleOutsideClick} />

{#if appState.fkPeekState.isOpen}
  <div
    id="fk-peek-popover-box"
    style="left: {appState.fkPeekState.x}px; top: {appState.fkPeekState.y}px;"
    class="fixed z-[90] w-80 max-h-96 bg-[#0f172a] border border-sky-500/50 rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100 text-xs animate-fade-in"
    role="dialog"
  >
    <!-- Header -->
    <div class="px-3.5 py-2.5 bg-slate-900 border-b border-slate-800 flex items-center justify-between">
      <div class="flex items-center space-x-1.5 truncate">
        <Icon name="link" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
        <span class="font-bold text-white truncate">
          {appState.fkPeekState.refTable} ({appState.fkPeekState.refCol} = {String(appState.fkPeekState.val ?? '')})
        </span>
      </div>

      <div class="flex items-center space-x-1.5 shrink-0">
        <button
          type="button"
          onclick={jumpToTable}
          class="px-2 py-0.5 bg-sky-600 hover:bg-sky-500 text-white font-bold rounded text-[10px] transition"
        >
          Jump
        </button>
        <button
          type="button"
          onclick={() => appState.closeFkPeek()}
          class="text-slate-400 hover:text-white p-0.5 rounded hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <!-- Body -->
    <div class="p-3 overflow-y-auto flex-1">
      {#if appState.fkPeekState.isLoading}
        <div class="p-4 text-center text-slate-400 flex items-center justify-center space-x-2">
          <Icon name="loader" class="w-4 h-4 animate-spin text-sky-400" />
          <span>Fetching referenced record...</span>
        </div>
      {:else if appState.fkPeekState.error}
        <div class="p-3 bg-amber-950/30 border border-amber-800/60 rounded text-amber-300 text-[11px]">
          {appState.fkPeekState.error}
        </div>
      {:else if appState.fkPeekState.data?.row}
        <div class="divide-y divide-slate-800/80">
          {#each Object.entries(appState.fkPeekState.data.row) as [col, val]}
            <div class="py-1.5 flex items-start justify-between space-x-2">
              <span class="font-semibold text-slate-400 font-mono text-[11px] truncate">{col}:</span>
              <span class="font-mono text-slate-200 text-[11px] text-right truncate select-all">
                {val === null ? 'NULL' : String(val)}
              </span>
            </div>
          {/each}
        </div>
      {:else}
        <div class="p-4 text-center text-slate-500 text-xs">
          No matching record found.
        </div>
      {/if}
    </div>
  </div>
{/if}
