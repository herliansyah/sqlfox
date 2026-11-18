<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let selectedDb = $state('');
  let sampleLimit = $state(20);
  let includeDDL = $state(true);
  let wasOpen = false;

  $effect(() => {
    if (appState.isSmartSeedModalOpen) {
      if (!wasOpen) {
        selectedDb = appState.currentDatabase || appState.availableDatabases[0] || '';
        sampleLimit = 20;
        includeDDL = true;
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  function handleExport() {
    if (!appState.activeSessionId) {
      appState.showToast('Connect to a database session first', 'warning');
      return;
    }
    const clampedLimit = Math.max(1, Math.min(1000, Number(sampleLimit) || 20));
    const url = api.exportSmartSeedUrl(appState.activeSessionId, {
      limit: clampedLimit,
      includeDDL,
      db: selectedDb || undefined,
    });
    window.open(url, '_blank', 'noopener,noreferrer');
    appState.showToast(`Downloading FK-aware Smart Seed (${clampedLimit} rows/table)...`, 'success');
    appState.isSmartSeedModalOpen = false;
  }
</script>

{#if appState.isSmartSeedModalOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 select-none"
    role="dialog"
    aria-modal="true"
  >
    <button
      tabindex="-1"
      aria-label="Close"
      class="fixed inset-0 bg-transparent cursor-default"
      onclick={() => (appState.isSmartSeedModalOpen = false)}
    ></button>

    <div class="w-full max-w-md bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col relative z-10 text-xs font-sans">
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-2.5">
          <Icon name="package" class="w-4 h-4 text-emerald-400" />
          <h3 class="font-bold text-sm text-slate-100">Export FK-Aware Smart Seed</h3>
        </div>
        <button
          onclick={() => (appState.isSmartSeedModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 space-y-4">
        <div class="p-3 rounded-lg bg-emerald-950/30 border border-emerald-800/50 text-[11px] text-emerald-200/90 leading-relaxed">
          Automatically resolves foreign-key dependencies via topological sort and transitive closure, producing a minimal referentially-intact SQL seed dataset for local development or staging.
        </div>

        <div class="space-y-1.5">
          <label for="smart-seed-db" class="block font-semibold text-slate-300">Target Database</label>
          <select
            id="smart-seed-db"
            bind:value={selectedDb}
            class="w-full bg-[#090d16] border border-[#334155] focus:border-emerald-500 rounded px-3 py-2 text-slate-100 font-mono text-xs outline-none"
          >
            {#if appState.availableDatabases.length === 0}
              <option value="">{appState.currentDatabase || 'Active Database'}</option>
            {:else}
              {#each appState.availableDatabases as db}
                <option value={db}>{db}</option>
              {/each}
            {/if}
          </select>
        </div>

        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <label for="smart-seed-limit" class="font-semibold text-slate-300">Rows Per Root Table</label>
            <span class="font-mono text-emerald-400 font-bold">{sampleLimit} rows</span>
          </div>
          <div class="flex items-center space-x-2">
            <input
              type="range"
              min="5"
              max="200"
              step="5"
              bind:value={sampleLimit}
              class="flex-1 accent-emerald-500"
            />
            <input
              id="smart-seed-limit"
              type="number"
              min="1"
              max="1000"
              bind:value={sampleLimit}
              class="w-20 bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-slate-100 font-mono text-xs text-right outline-none"
            />
          </div>
        </div>

        <label class="flex items-center space-x-2 p-2.5 rounded-lg bg-[#090d16] border border-[#1e293b] cursor-pointer">
          <input type="checkbox" bind:checked={includeDDL} class="rounded border-slate-700 text-emerald-500" />
          <div>
            <span class="text-slate-200 font-semibold block">Include Schema DDL (DROP + CREATE TABLE)</span>
            <span class="text-[10.5px] text-slate-400">Emit structure statements before inserting seeded records</span>
          </div>
        </label>
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#334155] bg-[#0b0f19] flex items-center justify-end space-x-2">
        <button
          type="button"
          onclick={() => (appState.isSmartSeedModalOpen = false)}
          class="px-4 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 rounded font-semibold transition"
        >
          Cancel
        </button>
        <button
          type="button"
          onclick={handleExport}
          disabled={!appState.activeSessionId}
          class="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white font-bold rounded transition flex items-center space-x-1.5 shadow-md"
        >
          <Icon name="download" class="w-3.5 h-3.5" />
          <span>Download .sql Seed</span>
        </button>
      </div>
    </div>
  </div>
{/if}
