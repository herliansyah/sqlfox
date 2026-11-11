<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let selectedDb = $state('');
  let selectedTable = $state('');
  let includeSchema = $state(true);
  let includeData = $state(true);
  let isExporting = $state(false);

  $effect(() => {
    if (appState.isDumpModalOpen) {
      selectedDb = appState.currentDatabase || '';
      selectedTable = appState.dumpTargetTable || '';
      includeSchema = true;
      includeData = true;
      isExporting = false;
    }
  });

  function handleExport() {
    if (!includeSchema && !includeData) {
      appState.showToast('Please select at least Table Structure or Table Data', 'error');
      return;
    }
    if (!appState.activeSessionId) return;

    let url = `/api/session/${appState.activeSessionId}/export-dump?schema=${includeSchema}&data=${includeData}`;
    if (selectedDb) {
      url += `&db=${encodeURIComponent(selectedDb)}`;
    }
    if (selectedTable) {
      url += `&table=${encodeURIComponent(selectedTable)}`;
    }

    isExporting = true;
    appState.showToast('Starting SQL dump stream download...', 'info');

    // Trigger browser download via invisible link
    const a = document.createElement('a');
    a.href = url;
    a.download = `${selectedDb || 'dump'}${selectedTable ? `_${selectedTable}` : ''}.sql`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);

    setTimeout(() => {
      isExporting = false;
      appState.isDumpModalOpen = false;
    }, 1200);
  }

  function handleClose() {
    appState.isDumpModalOpen = false;
  }
</script>

{#if appState.isDumpModalOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="dump-modal-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-lg flex flex-col overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400">
            <Icon name="download" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="dump-modal-title" class="font-bold text-sm text-slate-100 font-sans">
              Export SQL Database / Table Dump
            </h3>
            <p class="text-[11px] text-slate-400">
              Download standard SQL backup script with structure and data
            </p>
          </div>
        </div>
        <button
          onclick={handleClose}
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Dialog Body -->
      <div class="p-5 space-y-4 text-xs font-sans">
        <!-- Target Database -->
        <div class="space-y-1">
          <label for="dump-db-select" class="font-semibold text-slate-300">Target Database</label>
          <select
            id="dump-db-select"
            bind:value={selectedDb}
            class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
          >
            {#each appState.availableDatabases as db}
              <option value={db}>{db}</option>
            {/each}
          </select>
        </div>

        <!-- Target Scope / Table -->
        <div class="space-y-1">
          <label for="dump-tbl-select" class="font-semibold text-slate-300">Export Scope</label>
          <select
            id="dump-tbl-select"
            bind:value={selectedTable}
            class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
          >
            <option value="">Entire Database (All Tables & Views)</option>
            {#each appState.filteredTables as tbl}
              <option value={tbl.name}>Single Table: {tbl.name}</option>
            {/each}
          </select>
        </div>

        <!-- Dump Options -->
        <div class="space-y-2.5 bg-[#090d16] border border-[#1e293b] p-3 rounded-lg">
          <span class="font-semibold text-slate-400 text-[11px] uppercase tracking-wider block">
            Contents to Include
          </span>

          <label class="flex items-center space-x-2.5 text-slate-200 cursor-pointer select-none">
            <input
              type="checkbox"
              bind:checked={includeSchema}
              class="rounded bg-slate-900 border-slate-700 text-sky-500 focus:ring-0"
            />
            <div>
              <span class="font-semibold block">Table Structure (DDL)</span>
              <span class="text-[11px] text-slate-400 block">CREATE TABLE, DROP TABLE IF EXISTS, indexes, and constraints</span>
            </div>
          </label>

          <label class="flex items-center space-x-2.5 text-slate-200 cursor-pointer select-none">
            <input
              type="checkbox"
              bind:checked={includeData}
              class="rounded bg-slate-900 border-slate-700 text-sky-500 focus:ring-0"
            />
            <div>
              <span class="font-semibold block">Table Data (Records)</span>
              <span class="text-[11px] text-slate-400 block">INSERT statements with batched rows</span>
            </div>
          </label>
        </div>
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-end space-x-2 shrink-0">
        <button
          onclick={handleClose}
          type="button"
          class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition"
        >
          Cancel
        </button>
        <button
          onclick={handleExport}
          disabled={isExporting || (!includeSchema && !includeData)}
          type="button"
          class="px-4 py-1.5 bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow-lg shadow-sky-950/40"
        >
          {#if isExporting}
            <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
            <span>Streaming Dump...</span>
          {:else}
            <Icon name="download" class="w-3.5 h-3.5" />
            <span>Download SQL Dump</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
