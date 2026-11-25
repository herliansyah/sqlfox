<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { focusTrap } from '../../actions/focusTrap';
  import Icon from '../common/Icon.svelte';
  import SearchableSelect from '../common/SearchableSelect.svelte';

  let selectedDb = $state('');
  let scopeMode = $state<'all' | 'selected'>('all');
  let selectedTables = $state<string[]>([]);
  let tableFilter = $state('');
  let includeSchema = $state(true);
  let includeData = $state(true);

  let isLoadingTables = $state(false);
  let isExporting = $state(false);
  let wasOpen = false;

  async function handleDbChange(db: string) {
    selectedDb = db;
    selectedTables = [];
    if (appState.activeSessionId && db && !appState.catalogs[db.toLowerCase()]) {
      isLoadingTables = true;
      try {
        await appState.loadCatalog(appState.activeSessionId, db);
      } finally {
        isLoadingTables = false;
      }
    }
  }

  let allCatalogTables = $derived(
    (selectedDb && appState.catalogs[selectedDb.toLowerCase()]?.tables) ||
    appState.catalog?.tables ||
    []
  );
  let visibleTables = $derived.by(() => {
    const q = tableFilter.trim().toLowerCase();
    if (!q) return allCatalogTables;
    return allCatalogTables.filter((t) => t.name.toLowerCase().includes(q));
  });

  $effect(() => {
    if (appState.isDumpModalOpen) {
      if (!wasOpen) {
        selectedDb = appState.dumpTargetTable ? (appState.activeTab?.database || appState.currentDatabase || '') : (appState.currentDatabase || '');
        if (appState.dumpTargetTable) {
          scopeMode = 'selected';
          selectedTables = [appState.dumpTargetTable];
        } else {
          scopeMode = 'all';
          selectedTables = [];
        }
        tableFilter = '';
        includeSchema = true;
        includeData = true;
        isExporting = false;
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  function toggleTable(name: string) {
    if (selectedTables.includes(name)) {
      selectedTables = selectedTables.filter((t) => t !== name);
    } else {
      selectedTables = [...selectedTables, name];
    }
  }

  function selectAllVisible() {
    const merged = new Set(selectedTables);
    for (const t of visibleTables) merged.add(t.name);
    selectedTables = Array.from(merged);
  }

  function clearSelectedTables() {
    selectedTables = [];
  }

  function handleExport() {
    if (!includeSchema && !includeData) {
      appState.showToast('Please select at least Table Structure or Table Data', 'error');
      return;
    }
    if (scopeMode === 'selected' && selectedTables.length === 0) {
      appState.showToast('Please select at least one table to export', 'error');
      return;
    }
    if (!appState.activeSessionId) return;

    let url = `/api/session/${appState.activeSessionId}/export-dump?schema=${includeSchema}&data=${includeData}`;
    if (selectedDb) {
      url += `&db=${encodeURIComponent(selectedDb)}`;
    }
    if (scopeMode === 'selected') {
      for (const tbl of selectedTables) {
        url += `&table=${encodeURIComponent(tbl)}`;
      }
    }

    isExporting = true;
    appState.showToast('Starting SQL dump stream download...', 'info');

    const baseName = selectedDb || 'dump';
    let downloadName = `${baseName}.sql`;
    if (scopeMode === 'selected') {
      downloadName =
        selectedTables.length === 1
          ? `${baseName}_${selectedTables[0]}.sql`
          : `${baseName}_${selectedTables.length}_tables.sql`;
    }

    // Trigger browser download via invisible link
    const a = document.createElement('a');
    a.href = url;
    a.download = downloadName;
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
  <div class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      use:focusTrap
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
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={() => {
              appState.isDumpModalOpen = false;
              appState.isSmartSeedModalOpen = true;
            }}
            class="px-2.5 py-1 rounded-md bg-emerald-950/60 hover:bg-emerald-900/70 border border-emerald-700/50 text-emerald-300 text-[11px] font-semibold transition"
            title="Switch to Smart Seed (Relational Subset)"
          >
            Smart Seed →
          </button>
          <button
            onclick={handleClose}
            class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
            aria-label="Close"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Dialog Body -->
      <div class="p-5 space-y-4 text-xs font-sans">
        <!-- Target Database -->
        <div class="space-y-1">
          <span class="font-semibold text-slate-300 block">Target Database</span>
          <SearchableSelect
            options={appState.availableDatabases}
            value={selectedDb}
            onchange={handleDbChange}
            placeholder="Filter databases..."
            title="Select target database"
            panelWidth="w-full"
            triggerClass="w-full justify-between !bg-[#090d16] !border-[#334155] !py-1.5 !px-2.5 !text-xs !font-mono"
          />
        </div>

        <!-- Target Scope / Table -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-semibold text-slate-300">Export Scope</span>
            {#if scopeMode === 'selected'}
              <span class="text-[11px] font-mono text-sky-400">
                {selectedTables.length} selected
              </span>
            {/if}
          </div>

          <div class="grid grid-cols-2 gap-1.5 bg-[#090d16] p-1 rounded-lg border border-[#1e293b]">
            <button
              type="button"
              onclick={() => (scopeMode = 'all')}
              class="px-2.5 py-1.5 rounded text-xs font-semibold transition {scopeMode === 'all'
                ? 'bg-sky-600/20 text-sky-300 border border-sky-500/40'
                : 'text-slate-400 hover:text-slate-200 border border-transparent'}"
            >
              Entire Database
            </button>
            <button
              type="button"
              onclick={() => (scopeMode = 'selected')}
              class="px-2.5 py-1.5 rounded text-xs font-semibold transition {scopeMode === 'selected'
                ? 'bg-sky-600/20 text-sky-300 border border-sky-500/40'
                : 'text-slate-400 hover:text-slate-200 border border-transparent'}"
            >
              Select Tables ({allCatalogTables.length})
            </button>
          </div>

          {#if scopeMode === 'selected'}
            <div class="bg-[#090d16] border border-[#1e293b] rounded-lg p-2.5 space-y-2">
              <div class="flex items-center space-x-2">
                <div class="relative flex-1">
                  <input
                    type="text"
                    bind:value={tableFilter}
                    placeholder="Filter tables..."
                    class="w-full bg-[#0b0f19] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-1 text-xs text-slate-200 font-mono outline-none pr-6"
                  />
                  {#if tableFilter}
                    <button
                      type="button"
                      onclick={() => (tableFilter = '')}
                      class="absolute right-1.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-white text-[11px]"
                      aria-label="Clear table filter"
                    >
                      ✕
                    </button>
                  {/if}
                </div>
                <button
                  type="button"
                  onclick={selectAllVisible}
                  class="px-2 py-1 text-[11px] font-semibold text-sky-400 hover:text-sky-300 bg-sky-500/10 hover:bg-sky-500/20 border border-sky-500/20 rounded transition shrink-0"
                >
                  Select All
                </button>
                <button
                  type="button"
                  onclick={clearSelectedTables}
                  class="px-2 py-1 text-[11px] font-semibold text-slate-400 hover:text-white bg-slate-800 hover:bg-slate-700 rounded transition shrink-0"
                >
                  Clear
                </button>
              </div>

              <div class="max-h-44 overflow-y-auto space-y-0.5 pr-1">
                {#if visibleTables.length === 0}
                  <p class="text-[11px] text-slate-500 text-center py-3">
                    No matching tables found
                  </p>
                {:else}
                  {#each visibleTables as tbl (tbl.name)}
                    <label class="flex items-center justify-between px-2 py-1 rounded hover:bg-slate-800/60 cursor-pointer select-none">
                      <span class="flex items-center space-x-2 min-w-0">
                        <input
                          type="checkbox"
                          checked={selectedTables.includes(tbl.name)}
                          onchange={() => toggleTable(tbl.name)}
                          class="rounded bg-slate-900 border-slate-700 text-sky-500 focus:ring-0 shrink-0"
                        />
                        <span class="font-mono text-xs text-slate-200 truncate">{tbl.name}</span>
                      </span>
                      {#if tbl.type === 'VIEW'}
                        <span class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-purple-500/10 text-purple-300 border border-purple-500/20 shrink-0 ml-2">
                          VIEW
                        </span>
                      {/if}
                    </label>
                  {/each}
                {/if}
              </div>
            </div>
          {/if}
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

        {#if scopeMode === 'all' && includeData && allCatalogTables.length >= 25}
          <div class="p-3 rounded-lg bg-amber-950/35 border border-amber-800/70 text-[11px] text-amber-200 flex items-start space-x-2">
            <Icon name="alert-triangle" class="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
            <div>
              <span class="font-bold block">Large Schema Notice ({allCatalogTables.length} tables)</span>
              <span class="text-amber-300/90">
                Full-database dumps with data on large schemas can generate large files or increase read load. Consider
                <button type="button" onclick={() => (scopeMode = 'selected')} class="underline font-semibold text-amber-100 hover:text-white">selecting specific tables</button>
                or using <strong>Smart Seed Export</strong> for relational subsets.
              </span>
            </div>
          </div>
        {/if}
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
          disabled={isExporting || (!includeSchema && !includeData) || (scopeMode === 'selected' && selectedTables.length === 0)}
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
