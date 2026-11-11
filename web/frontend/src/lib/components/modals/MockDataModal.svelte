<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let selectedTable = $state('');
  let rowCount = $state(50);
  let batchSize = $state(50);
  let truncateFirst = $state(false);
  let locale = $state('en_US');
  let isGenerating = $state(false);

  let catalogTables = $derived(
    (appState.catalog?.tables || []).map((t) => t.name)
  );

  let activeTableMeta = $derived.by(() => {
    if (!selectedTable || !appState.catalog?.tables) return null;
    return appState.catalog.tables.find(
      (t) => t.name.toLowerCase() === selectedTable.toLowerCase()
    );
  });

  $effect(() => {
    if (appState.isMockDataModalOpen) {
      selectedTable = appState.mockDataTableName || appState.activeTab?.tableName || catalogTables[0] || '';
      rowCount = 50;
      truncateFirst = false;
    }
  });

  async function handleGenerate() {
    if (!appState.activeConnection || !selectedTable) {
      appState.showToast('Select a table first', 'warning');
      return;
    }

    if (truncateFirst) {
      const ok = confirm(`CAUTION: You checked "Truncate table first". All existing rows in "${selectedTable}" will be permanently deleted before generating mock data. Continue?`);
      if (!ok) return;
    }

    isGenerating = true;
    try {
      const connId = appState.activeConnection.id;
      const res = await api.generateMockData(connId, selectedTable, {
        row_count: rowCount,
        batch_size: batchSize,
        truncate_first: truncateFirst,
        locale,
      });

      appState.showToast(
        res.message || `Successfully generated ${rowCount} realistic mock records in "${selectedTable}"!`,
        'success'
      );
      appState.isMockDataModalOpen = false;

      // Reload catalog & refresh data if viewer is open
      await appState.loadCatalog(connId, appState.currentDatabase || undefined);
    } catch (err: any) {
      appState.showToast(`Mock data generation failed: ${err.message}`, 'error');
    } finally {
      isGenerating = false;
    }
  }
</script>

{#if appState.isMockDataModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="database" class="w-4 h-4 text-emerald-400" />
          <h3 class="font-bold text-slate-100 text-sm">Smart Seed / Mock Data Generator</h3>
        </div>
        <button onclick={() => (appState.isMockDataModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Target Table *</label>
          <select
            bind:value={selectedTable}
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
          >
            {#each catalogTables as tbl}
              <option value={tbl}>{tbl}</option>
            {/each}
          </select>
        </div>

        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="text-slate-400 text-xs font-medium">Number of Records to Generate</label>
            <div class="flex items-center space-x-1">
              {#each [25, 50, 100, 500, 1000] as cnt}
                <button
                  type="button"
                  onclick={() => (rowCount = cnt)}
                  class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-slate-400 hover:text-emerald-300 font-mono"
                >
                  {cnt}
                </button>
              {/each}
            </div>
          </div>
          <input
            type="number"
            min="1"
            max="10000"
            bind:value={rowCount}
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
          />
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Batch Size</label>
            <input
              type="number"
              min="10"
              max="500"
              bind:value={batchSize}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
            />
          </div>
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Locale Data Presets</label>
            <select
              bind:value={locale}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-emerald-500"
            >
              <option value="en_US">English (US)</option>
              <option value="id_ID">Indonesian (ID)</option>
              <option value="ja_JP">Japanese (JP)</option>
              <option value="de_DE">German (DE)</option>
            </select>
          </div>
        </div>

        {#if activeTableMeta && activeTableMeta.columns}
          <div>
            <span class="block text-slate-400 text-xs font-medium mb-1.5">Columns Being Synthesized</span>
            <div class="flex flex-wrap gap-1 p-2 bg-[#050811] border border-[#1e293b] rounded-lg max-h-24 overflow-y-auto">
              {#each activeTableMeta.columns as col}
                <span class="px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-[11px] font-mono text-slate-300 flex items-center space-x-1">
                  <span class="text-emerald-400">{col.name}</span>
                  <span class="text-[10px] text-slate-500">({col.type})</span>
                </span>
              {/each}
            </div>
          </div>
        {/if}

        <label class="flex items-center space-x-2 p-2.5 rounded-xl bg-rose-950/20 border border-rose-900/40 cursor-pointer">
          <input type="checkbox" bind:checked={truncateFirst} class="rounded border-rose-700 text-rose-500" />
          <div>
            <span class="text-rose-300 font-medium text-xs block">Truncate table before insertion</span>
            <span class="text-rose-400/80 text-[11px]">Wipes all current records in this table first</span>
          </div>
        </label>
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-end space-x-2 bg-[#0f172a]">
        <button
          onclick={() => (appState.isMockDataModalOpen = false)}
          disabled={isGenerating}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
        >
          Cancel
        </button>
        <button
          onclick={handleGenerate}
          disabled={isGenerating || !selectedTable}
          class="px-4 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-emerald-900/30"
        >
          <Icon name="database" class="w-3.5 h-3.5 {isGenerating ? 'animate-spin' : ''}" />
          <span>{isGenerating ? 'Generating...' : `Generate ${rowCount} Rows`}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
