<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let sourceConnId = $state('');
  let sourceDb = $state('');
  let sourceTable = $state('');

  let targetConnId = $state('');
  let targetDb = $state('');
  let targetTable = $state('');

  let isComparing = $state(false);
  let isApplying = $state(false);

  let syncDiff = $state<{
    table?: string;
    pk_columns?: string[];
    columns?: string[];
    summary?: Record<string, number>;
    operations?: Array<{ type: string; pk: any; old_data?: any[]; data?: any[] }>;
    error?: string;
  } | null>(null);

  $effect(() => {
    if (appState.isDataSyncOpen) {
      sourceConnId = appState.activeConnection?.id || '';
      sourceDb = appState.currentDatabase || '';
      sourceTable = appState.activeTab?.tableName || '';
      targetTable = sourceTable;
      syncDiff = null;
    }
  });

  async function handleCompare() {
    if (!sourceConnId || !targetConnId || !sourceTable) {
      appState.showToast('Source and target configuration required', 'warning');
      return;
    }

    isComparing = true;
    syncDiff = null;
    try {
      const srcSession = (await api.connectSession(sourceConnId)).id || sourceConnId;
      const tgtSession = (await api.connectSession(targetConnId)).id || targetConnId;
      const res = await api.compareDataSync({
        source_session_id: srcSession,
        source_db: sourceDb,
        target_session_id: tgtSession,
        target_db: targetDb,
        table: sourceTable || targetTable,
      });
      syncDiff = res;
      appState.showToast('Row comparison completed', 'info');
    } catch (err: any) {
      syncDiff = { error: err.message || 'Comparison failed' };
      appState.showToast(`Sync comparison failed: ${err.message}`, 'error');
    } finally {
      isComparing = false;
    }
  }

  async function handleApply() {
    if (!syncDiff?.operations || syncDiff.operations.length === 0) return;

    isApplying = true;
    try {
      const tgtSession = (await api.connectSession(targetConnId)).id || targetConnId;
      await api.applyDataSync({
        target_session_id: tgtSession,
        target_db: targetDb,
        table: sourceTable || targetTable,
        columns: syncDiff.columns || [],
        operations: syncDiff.operations,
      });
      appState.showToast(`Applied ${syncDiff.operations.length} synchronization operations!`, 'success');
      syncDiff = null;
      appState.isDataSyncOpen = false;
    } catch (err: any) {
      appState.showToast(`Failed to apply sync: ${err.message}`, 'error');
    } finally {
      isApplying = false;
    }
  }

  function handleCancel() {
    if (isComparing || isApplying) return;
    appState.isDataSyncOpen = false;
  }
</script>

{#if appState.isDataSyncOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-2xl rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100 max-h-[90vh]"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center space-x-2">
          <Icon name="scale" class="w-4 h-4 text-emerald-400" />
          <h3 class="text-sm font-semibold text-slate-100">
            Row-Level Data Synchronization
          </h3>
        </div>
        <button
          type="button"
          onclick={handleCancel}
          disabled={isComparing || isApplying}
          class="text-slate-400 hover:text-slate-200 p-1 rounded hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 space-y-4 overflow-y-auto flex-1">
        <div class="grid grid-cols-2 gap-3 text-xs">
          <!-- Source -->
          <div class="p-3 rounded-lg border border-slate-800 bg-[#050811] space-y-2">
            <h4 class="font-bold text-sky-400 uppercase">Source (Master / Truth)</h4>
            <div>
              <label class="block text-slate-400 mb-1">Connection</label>
              <select
                bind:value={sourceConnId}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100"
              >
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label class="block text-slate-400 mb-1">Table</label>
              <input
                type="text"
                bind:value={sourceTable}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100"
              />
            </div>
          </div>

          <!-- Target -->
          <div class="p-3 rounded-lg border border-slate-800 bg-[#050811] space-y-2">
            <h4 class="font-bold text-emerald-400 uppercase">Target (To Reconcile)</h4>
            <div>
              <label class="block text-slate-400 mb-1">Connection</label>
              <select
                bind:value={targetConnId}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100"
              >
                <option value="">Select Target Connection...</option>
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label class="block text-slate-400 mb-1">Table</label>
              <input
                type="text"
                bind:value={targetTable}
                placeholder="Defaults to source table"
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100"
              />
            </div>
          </div>
        </div>

        {#if syncDiff}
          <div class="p-4 rounded-lg border border-slate-800 bg-[#050811] space-y-3">
            <h4 class="text-xs font-bold text-slate-200">Reconciliation Summary</h4>
            <div class="grid grid-cols-3 gap-2 text-center text-xs">
              <div class="p-2 rounded bg-emerald-950/40 border border-emerald-800/60">
                <div class="text-emerald-400 font-bold text-lg font-mono">{syncDiff.summary?.insert_count ?? 0}</div>
                <div class="text-[11px] text-slate-400">Rows to Insert</div>
              </div>
              <div class="p-2 rounded bg-amber-950/40 border border-amber-800/60">
                <div class="text-amber-400 font-bold text-lg font-mono">{syncDiff.summary?.update_count ?? 0}</div>
                <div class="text-[11px] text-slate-400">Rows to Update</div>
              </div>
              <div class="p-2 rounded bg-rose-950/40 border border-rose-800/60">
                <div class="text-rose-400 font-bold text-lg font-mono">{syncDiff.summary?.delete_count ?? 0}</div>
                <div class="text-[11px] text-slate-400">Rows to Delete</div>
              </div>
            </div>

            {#if syncDiff.operations && syncDiff.operations.length > 0}
              <div class="space-y-1">
                <span class="text-[11px] font-semibold text-slate-400">
                  {syncDiff.operations.length} operation(s) pending — PK: {syncDiff.pk_columns?.join(', ') || 'auto'}
                </span>
              </div>
            {:else}
              <div class="text-center p-3 text-xs text-slate-400">
                Both tables are already in complete synchronization. No modifications needed.
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-between">
        <button
          type="button"
          onclick={handleCompare}
          disabled={isComparing || isApplying || !sourceConnId || !targetConnId || !sourceTable}
          class="px-4 py-1.5 rounded text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-slate-200 transition disabled:opacity-50"
        >
          {isComparing ? 'Comparing Checksums...' : 'Compare Tables'}
        </button>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={handleCancel}
            disabled={isComparing || isApplying}
            class="px-3.5 py-1.5 rounded text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition disabled:opacity-50"
          >
            Cancel
          </button>

          {#if syncDiff?.operations && syncDiff.operations.length > 0}
            <button
              type="button"
              onclick={handleApply}
              disabled={isApplying}
              class="px-4 py-1.5 rounded text-xs font-bold bg-emerald-600 hover:bg-emerald-500 text-white transition shadow flex items-center space-x-1 disabled:opacity-50"
            >
              {#if isApplying}
                <Icon name="loader" class="w-3.5 h-3.5 animate-spin" />
                <span>Applying...</span>
              {:else}
                <span>Apply Sync Changes</span>
              {/if}
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}
