<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let sourceConnId = $state('');
  let sourceDb = $state('');
  let targetConnId = $state('');
  let targetDb = $state('');

  let srcDatabases = $state<string[]>([]);
  let tgtDatabases = $state<string[]>([]);
  let srcTables = $state<string[]>([]);
  let selectedTables = $state<string[]>([]);
  let tableSearch = $state('');

  let createTables = $state(true);
  let truncateTarget = $state(false);
  let batchSize = $state(1000);

  let isLoadingSourceSchema = $state(false);
  let isTransferring = $state(false);
  let progressLogs = $state<string[]>([]);
  let transferResult = $state<{ success: boolean; total_rows?: number; error?: string; errors?: string[] } | null>(null);

  let wasOpen = false;

  async function loadSourceMetadata(connId: string, db: string) {
    if (!connId) return;
    isLoadingSourceSchema = true;
    try {
      const sess = await api.connectSession(connId);
      srcDatabases = sess.schema?.databases || [];
      const activeDb = db || sess.current_database || sess.schema?.database_name || '';
      if (!sourceDb && activeDb) sourceDb = activeDb;
      const schema = activeDb ? await api.getSchema(sess.id, activeDb) : sess.schema;
      srcTables = (schema?.tables || []).filter((t) => t.type !== 'VIEW').map((t) => t.name);
      if (selectedTables.length === 0 && appState.activeTab?.tableName && srcTables.includes(appState.activeTab.tableName)) {
        selectedTables = [appState.activeTab.tableName];
      } else {
        selectedTables = selectedTables.filter((name) => srcTables.includes(name));
      }
    } catch {
      srcTables = [];
    } finally {
      isLoadingSourceSchema = false;
    }
  }

  async function loadTargetMetadata(connId: string) {
    if (!connId) {
      tgtDatabases = [];
      return;
    }
    try {
      const sess = await api.connectSession(connId);
      tgtDatabases = sess.schema?.databases || [];
      if (!targetDb && (sess.current_database || sess.schema?.database_name)) {
        targetDb = sess.current_database || sess.schema?.database_name || '';
      }
    } catch {
      tgtDatabases = [];
    }
  }

  $effect(() => {
    if (appState.isDataTransferOpen) {
      if (!wasOpen) {
        sourceConnId = appState.activeConnection?.id || '';
        sourceDb = appState.currentDatabase || '';
        targetConnId = '';
        targetDb = '';
        selectedTables = appState.activeTab?.tableName ? [appState.activeTab.tableName] : [];
        tableSearch = '';
        createTables = true;
        truncateTarget = false;
        batchSize = 1000;
        progressLogs = [];
        transferResult = null;
        wasOpen = true;
        if (sourceConnId) loadSourceMetadata(sourceConnId, sourceDb);
      }
    } else {
      wasOpen = false;
    }
  });

  let visibleSrcTables = $derived.by(() => {
    const q = tableSearch.trim().toLowerCase();
    if (!q) return srcTables;
    return srcTables.filter((t) => t.toLowerCase().includes(q));
  });

  function toggleTable(name: string) {
    if (selectedTables.includes(name)) {
      selectedTables = selectedTables.filter((t) => t !== name);
    } else {
      selectedTables = [...selectedTables, name];
    }
  }

  function selectAllTables() {
    selectedTables = Array.from(new Set([...selectedTables, ...visibleSrcTables]));
  }

  function clearSelectedTables() {
    selectedTables = [];
  }

  async function handleTransfer() {
    if (!sourceConnId || !targetConnId || selectedTables.length === 0) {
      appState.showToast('Source connection, target connection, and at least one table are required', 'warning');
      return;
    }

    const clampedBatch = Math.max(50, Math.min(2000, Number(batchSize) || 1000));
    batchSize = clampedBatch;

    isTransferring = true;
    transferResult = null;
    progressLogs = [`Connecting sessions & preparing transfer of ${selectedTables.length} table(s)...`];

    try {
      const srcSession = (await api.connectSession(sourceConnId)).id || sourceConnId;
      const tgtSession = (await api.connectSession(targetConnId)).id || targetConnId;
      const res = await api.transferData(
        {
          source_session_id: srcSession,
          source_db: sourceDb,
          target_session_id: tgtSession,
          target_db: targetDb,
          tables: selectedTables,
          create_tables: createTables,
          truncate_target: truncateTarget,
          batch_size: clampedBatch,
        },
        (evt) => {
          if (evt.message) {
            progressLogs = [...progressLogs, evt.message];
          } else if (evt.table && evt.rows_transferred != null) {
            progressLogs = [...progressLogs, `${evt.table}: transferred ${evt.rows_transferred} rows...`];
          } else if (evt.error) {
            progressLogs = [...progressLogs, `[ERROR] ${evt.table ? evt.table + ': ' : ''}${evt.error}`];
          }
        }
      );

      transferResult = {
        success: !res.has_errors,
        total_rows: res.total_rows,
        errors: Array.isArray(res.errors) ? res.errors : undefined,
        error: res.has_errors ? (res.status || 'Transfer completed with errors') : undefined,
      };
      progressLogs = [...progressLogs, `Done: ${res.total_rows ?? 0} total rows transferred.`];
      appState.showToast(res.status || 'Transfer completed!', res.has_errors ? 'warning' : 'success');
    } catch (err: any) {
      transferResult = { success: false, error: err.message || 'Transfer failed' };
      progressLogs = [...progressLogs, `[FAILED] ${err.message}`];
      appState.showToast(`Transfer failed: ${err.message}`, 'error');
    } finally {
      isTransferring = false;
    }
  }

  function handleCancel() {
    if (isTransferring) return;
    appState.isDataTransferOpen = false;
  }
</script>

{#if appState.isDataTransferOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 p-4 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-2xl max-h-[90vh] rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60 shrink-0">
        <div class="flex items-center space-x-2">
          <Icon name="rocket" class="w-4 h-4 text-sky-400" />
          <h3 class="text-sm font-semibold text-slate-100">
            Cross-Connection Data Transfer
          </h3>
        </div>
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={() => {
              appState.isDataTransferOpen = false;
              appState.isDataSyncOpen = true;
            }}
            disabled={isTransferring}
            class="px-2.5 py-1 rounded-md bg-sky-950/60 hover:bg-sky-900/70 border border-sky-700/50 text-sky-300 text-[11px] font-semibold transition disabled:opacity-40"
            title="Switch to Data Compare & Sync"
          >
            Compare &amp; Sync →
          </button>
          <button
            type="button"
            onclick={handleCancel}
            disabled={isTransferring}
            class="text-slate-400 hover:text-slate-200 p-1 rounded hover:bg-slate-800 transition"
            aria-label="Close"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Body -->
      <div class="p-5 space-y-4 overflow-y-auto flex-1 text-xs">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <!-- Source Settings -->
          <div class="p-3.5 rounded-lg border border-slate-800 bg-[#050811] space-y-2.5">
            <h4 class="text-xs font-semibold text-sky-400 uppercase tracking-wide">Source Configuration</h4>
            <div>
              <label for="dt-src-conn" class="block text-slate-400 mb-1">Source Connection *</label>
              <select
                id="dt-src-conn"
                bind:value={sourceConnId}
                onchange={() => { sourceDb = ''; selectedTables = []; loadSourceMetadata(sourceConnId, ''); }}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 focus:outline-none focus:border-sky-500"
              >
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="dt-src-db" class="block text-slate-400 mb-1">Source Database</label>
              {#if srcDatabases.length > 0}
                <select
                  id="dt-src-db"
                  bind:value={sourceDb}
                  onchange={() => loadSourceMetadata(sourceConnId, sourceDb)}
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono focus:outline-none focus:border-sky-500"
                >
                  {#each srcDatabases as db}
                    <option value={db}>{db}</option>
                  {/each}
                </select>
              {:else}
                <input
                  id="dt-src-db"
                  type="text"
                  bind:value={sourceDb}
                  placeholder="Optional / Default schema"
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono focus:outline-none focus:border-sky-500"
                />
              {/if}
            </div>
          </div>

          <!-- Target Settings -->
          <div class="p-3.5 rounded-lg border border-slate-800 bg-[#050811] space-y-2.5">
            <h4 class="text-xs font-semibold text-emerald-400 uppercase tracking-wide">Target Configuration</h4>
            <div>
              <label for="dt-tgt-conn" class="block text-slate-400 mb-1">Target Connection *</label>
              <select
                id="dt-tgt-conn"
                bind:value={targetConnId}
                onchange={() => { targetDb = ''; loadTargetMetadata(targetConnId); }}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 focus:outline-none focus:border-emerald-500"
              >
                <option value="">Select Target Connection...</option>
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="dt-tgt-db" class="block text-slate-400 mb-1">Target Database</label>
              {#if tgtDatabases.length > 0}
                <select
                  id="dt-tgt-db"
                  bind:value={targetDb}
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono focus:outline-none focus:border-emerald-500"
                >
                  {#each tgtDatabases as db}
                    <option value={db}>{db}</option>
                  {/each}
                </select>
              {:else}
                <input
                  id="dt-tgt-db"
                  type="text"
                  bind:value={targetDb}
                  placeholder="Target DB"
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono focus:outline-none focus:border-emerald-500"
                />
              {/if}
            </div>
          </div>
        </div>

        <!-- Multi-Table Selection Checklist -->
        <div class="p-3.5 rounded-lg border border-slate-800 bg-[#050811] space-y-2">
          <div class="flex items-center justify-between flex-wrap gap-2">
            <span class="font-semibold text-slate-300">
              Select Tables to Transfer ({selectedTables.length} of {srcTables.length} selected)
            </span>
            <div class="flex items-center space-x-2">
              <input
                type="text"
                bind:value={tableSearch}
                placeholder="Filter tables..."
                class="bg-slate-900 border border-slate-700 rounded px-2 py-1 text-[11px] text-slate-200 w-36 focus:outline-none focus:border-sky-500"
              />
              <button type="button" onclick={selectAllTables} class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-sky-400 font-semibold text-[11px]">
                Select All
              </button>
              <button type="button" onclick={clearSelectedTables} class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-semibold text-[11px]">
                Select None
              </button>
            </div>
          </div>

          <div class="max-h-36 overflow-y-auto grid grid-cols-2 sm:grid-cols-3 gap-1 p-2 rounded bg-slate-950 border border-slate-800/80">
            {#if isLoadingSourceSchema}
              <div class="col-span-full py-4 text-center text-slate-500">Loading source tables...</div>
            {:else if visibleSrcTables.length === 0}
              <div class="col-span-full py-4 text-center text-slate-500">No tables found</div>
            {:else}
              {#each visibleSrcTables as tbl (tbl)}
                <label class="flex items-center space-x-2 px-2 py-1 rounded hover:bg-slate-900 cursor-pointer select-none min-w-0">
                  <input
                    type="checkbox"
                    checked={selectedTables.includes(tbl)}
                    onchange={() => toggleTable(tbl)}
                    class="rounded border-slate-700 bg-slate-900 text-sky-500 focus:ring-0 shrink-0"
                  />
                  <span class="font-mono text-[11px] text-slate-200 truncate" title={tbl}>{tbl}</span>
                </label>
              {/each}
            {/if}
          </div>
        </div>

        <!-- Transfer Options -->
        <div class="p-3 rounded-lg border border-slate-800 bg-[#050811] flex flex-wrap items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-4">
            <label class="flex items-center space-x-2 text-slate-300 cursor-pointer">
              <input type="checkbox" bind:checked={createTables} class="rounded border-slate-700 bg-slate-900 text-emerald-500 focus:ring-0" />
              <span>Create missing target tables automatically</span>
            </label>
            <label class="flex items-center space-x-2 text-slate-300 cursor-pointer">
              <input type="checkbox" bind:checked={truncateTarget} class="rounded border-slate-700 bg-slate-900 text-rose-500 focus:ring-0" />
              <span class="text-rose-300">Truncate target tables first</span>
            </label>
          </div>

          <div class="flex items-center space-x-1.5">
            <span class="text-slate-400">Batch Size (50–2000):</span>
            <input
              type="number"
              bind:value={batchSize}
              min="50"
              max="2000"
              step="50"
              class="w-20 bg-slate-900 border border-slate-700 rounded px-2 py-1 text-right text-slate-100 font-mono"
            />
          </div>
        </div>

        {#if progressLogs.length > 0}
          <div class="max-h-28 overflow-y-auto bg-[#050811] border border-slate-800 rounded-lg p-2.5 font-mono text-[11px] space-y-0.5 text-slate-300">
            {#each progressLogs as line}
              <div class={line.includes('[ERROR') || line.includes('[FAILED') ? 'text-rose-400' : 'text-slate-300'}>{line}</div>
            {/each}
          </div>
        {/if}

        {#if transferResult}
          <div class="p-3 rounded-lg border text-xs flex items-center space-x-2 {transferResult.success
            ? 'border-emerald-800 bg-emerald-950/30 text-emerald-300'
            : 'border-rose-800 bg-rose-950/30 text-rose-300'}">
            {#if transferResult.success}
              <Icon name="check-circle" class="w-4 h-4 text-emerald-400 shrink-0" />
              <span>Transfer completed! Total rows transferred: {transferResult.total_rows?.toLocaleString() ?? 0}</span>
            {:else}
              <Icon name="alert-triangle" class="w-4 h-4 text-rose-400 shrink-0" />
              <span>Transfer failed: {transferResult.error}</span>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-end space-x-2.5 shrink-0">
        <button
          type="button"
          onclick={handleCancel}
          disabled={isTransferring}
          class="px-4 py-1.5 rounded text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition disabled:opacity-50"
        >
          Cancel
        </button>

        <button
          type="button"
          onclick={handleTransfer}
          disabled={isTransferring || !sourceConnId || !targetConnId || selectedTables.length === 0}
          class="px-5 py-1.5 rounded text-xs font-bold bg-sky-600 hover:bg-sky-500 text-white transition shadow flex items-center space-x-1.5 disabled:opacity-50"
        >
          {#if isTransferring}
            <Icon name="loader" class="w-3.5 h-3.5 animate-spin" />
            <span>Transferring Rows...</span>
          {:else}
            <span>Transfer {selectedTables.length} Table(s)</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
