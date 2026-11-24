<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let selectedTable = $state('');
  let rowCount = $state(50);
  let truncateFirst = $state(false);
  let isGenerating = $state(false);
  let elapsedMs = $state(0);
  let statusMessage = $state('');
  let statusError = $state(false);
  let elapsedTimer: any = null;

  let activeCatalog = $derived(
    (appState.activeTab?.database && appState.catalogs[appState.activeTab.database.toLowerCase()]) ||
    (appState.currentDatabase && appState.catalogs[appState.currentDatabase.toLowerCase()]) ||
    appState.catalog
  );

  let catalogTables = $derived(
    (activeCatalog?.tables || []).map((t) => t.name)
  );

  let activeTableMeta = $derived.by(() => {
    if (!selectedTable || !activeCatalog?.tables) return null;
    return activeCatalog.tables.find(
      (t) => t.name.toLowerCase() === selectedTable.toLowerCase()
    );
  });

  let wasOpen = false;

  $effect(() => {
    if (appState.isMockDataModalOpen) {
      if (!wasOpen) {
        selectedTable = appState.mockDataTableName || appState.activeTab?.tableName || catalogTables[0] || '';
        rowCount = 50;
        truncateFirst = false;
        elapsedMs = 0;
        statusMessage = '';
        statusError = false;
        wasOpen = true;
      }
    } else {
      if (elapsedTimer) {
        clearInterval(elapsedTimer);
        elapsedTimer = null;
      }
      wasOpen = false;
    }
    return () => {
      if (elapsedTimer) {
        clearInterval(elapsedTimer);
        elapsedTimer = null;
      }
    };
  });

  function handleClose() {
    if (isGenerating) return;
    if (elapsedTimer) {
      clearInterval(elapsedTimer);
      elapsedTimer = null;
    }
    appState.closeMockDataModal();
  }

  async function handleGenerate() {
    if (!appState.activeConnection || !selectedTable) {
      appState.showToast('Select a table first', 'warning');
      return;
    }

    const clampedRows = Math.max(1, Math.min(5000, Number(rowCount) || 50));
    rowCount = clampedRows;

    if (truncateFirst) {
      const ok = await appState.showNativeConfirm(
        `CAUTION: You checked "Truncate table first". All existing rows in "${selectedTable}" will be permanently deleted before generating mock data. Continue?`,
        'Truncate Before Seeding'
      );
      if (!ok) return;
    }

    const start = performance.now();
    elapsedMs = 0;
    statusError = false;
    statusMessage = `Synthesizing ${clampedRows} mock rows for "${selectedTable}"...`;
    isGenerating = true;
    if (elapsedTimer) clearInterval(elapsedTimer);
    elapsedTimer = setInterval(() => {
      elapsedMs = Math.round(performance.now() - start);
    }, 100);

    try {
      const connId = appState.activeSessionId || appState.activeConnection.id;
      const targetDb = appState.activeTab?.database || appState.currentDatabase || undefined;
      if (truncateFirst) {
        const truncated = await appState.truncateTable(selectedTable, targetDb);
        if (!truncated) {
          statusError = true;
          statusMessage = 'Truncate cancelled or failed; mock data generation aborted.';
          return;
        }
      }
      const res = await api.generateMockData(connId, selectedTable, {
        rows: clampedRows,
        database: targetDb,
      });

      const finalElapsed = Math.round(performance.now() - start);
      elapsedMs = finalElapsed;
      const inserted = res.inserted ?? clampedRows;
      statusError = false;
      statusMessage = `Inserted ${inserted.toLocaleString()} mock record(s) into "${selectedTable}" in ${finalElapsed} ms.`;
      appState.showToast(`Generated ${inserted} mock records in "${selectedTable}" (${finalElapsed}ms)`, 'success');

      // Reload catalog & refresh data if viewer is open
      await appState.loadCatalog(connId, targetDb);
      if (appState.activeTab?.isTableViewer && appState.activeTab.tableName?.toLowerCase() === selectedTable.toLowerCase()) {
        await appState.loadTableData(appState.activeTab);
      }
    } catch (err: any) {
      elapsedMs = Math.round(performance.now() - start);
      statusError = true;
      statusMessage = err.message || 'Mock data generation failed';
      appState.showToast(`Mock data generation failed: ${err.message}`, 'error');
    } finally {
      if (elapsedTimer) {
        clearInterval(elapsedTimer);
        elapsedTimer = null;
      }
      isGenerating = false;
    }
  }
</script>

{#if appState.isMockDataModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-lg max-h-[90vh] shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="sparkles" class="w-4 h-4 text-purple-400" />
          <h3 class="font-bold text-slate-100 text-sm">Smart Seed / Mock Data Generator</h3>
        </div>
        <button type="button" onclick={handleClose} disabled={isGenerating} class="text-slate-400 hover:text-white disabled:opacity-40" aria-label="Close">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4 overflow-y-auto flex-1">
        <div>
          <label for="mock-target-table" class="block text-slate-400 text-xs font-medium mb-1">Target Table *</label>
          <select
            id="mock-target-table"
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
            <label for="mock-row-count" class="text-slate-400 text-xs font-medium">Rows to Generate (max 5,000)</label>
            <div class="flex items-center space-x-1">
              {#each [25, 50, 100, 500, 1000, 5000] as cnt}
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
            id="mock-row-count"
            type="number"
            min="1"
            max="5000"
            bind:value={rowCount}
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
          />
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

        {#if statusMessage}
          <div
            id="mock-data-status"
            class="p-2.5 rounded-lg border text-[11px] font-mono flex items-center justify-between gap-2 {statusError
              ? 'bg-rose-950/40 border-rose-800 text-rose-300'
              : isGenerating
                ? 'bg-sky-950/30 border-sky-800 text-sky-300'
                : 'bg-emerald-950/30 border-emerald-800 text-emerald-300'}"
          >
            <span>{statusMessage}</span>
            <span class="shrink-0 text-slate-400">{elapsedMs} ms</span>
          </div>
        {/if}
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-end space-x-2 bg-[#0f172a]">
        <button
          type="button"
          onclick={handleClose}
          disabled={isGenerating}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition disabled:opacity-40"
        >
          Close
        </button>
        <button
          type="button"
          onclick={handleGenerate}
          disabled={isGenerating || !selectedTable}
          class="px-4 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-emerald-900/30"
        >
          <Icon name="sparkles" class="w-3.5 h-3.5 {isGenerating ? 'animate-spin' : ''}" />
          <span>{isGenerating ? `Generating (${elapsedMs}ms)...` : `Generate ${Math.min(5000, Math.max(1, Number(rowCount) || 50))} Rows`}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
