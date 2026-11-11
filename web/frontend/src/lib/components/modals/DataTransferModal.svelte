<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';

  let sourceConnId = $state('');
  let sourceDb = $state('');
  let sourceTable = $state('');

  let targetConnId = $state('');
  let targetDb = $state('');
  let targetTable = $state('');
  let truncateTarget = $state(false);
  let batchSize = $state(1000);

  let isTransferring = $state(false);
  let transferResult = $state<{ success: boolean; rows_transferred?: number; error?: string } | null>(null);

  $effect(() => {
    if (appState.isDataTransferOpen) {
      sourceConnId = appState.activeConnection?.id || '';
      sourceDb = appState.currentDatabase || '';
      sourceTable = appState.activeTab?.tableName || '';
      targetTable = sourceTable;
      transferResult = null;
    }
  });

  async function handleTransfer() {
    if (!sourceConnId || !targetConnId || !sourceTable) {
      appState.showToast('Source and target connection, plus table name are required', 'warning');
      return;
    }

    isTransferring = true;
    transferResult = null;
    try {
      const res = await api.transferData({
        source_connection_id: sourceConnId,
        source_database: sourceDb,
        source_table: sourceTable,
        target_connection_id: targetConnId,
        target_database: targetDb,
        target_table: targetTable || sourceTable,
        truncate_target: truncateTarget,
        batch_size: batchSize,
      });
      transferResult = res;
      appState.showToast(`Transferred ${res.rows_transferred ?? 0} rows successfully!`, 'success');
    } catch (err: any) {
      transferResult = { success: false, error: err.message || 'Transfer failed' };
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
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-xl rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center space-x-2">
          <span class="text-sky-400">🚀</span>
          <h3 class="text-sm font-semibold text-slate-100">
            Cross-Connection Data Transfer
          </h3>
        </div>
        <button
          type="button"
          onclick={handleCancel}
          disabled={isTransferring}
          class="text-slate-400 hover:text-slate-200 text-lg leading-none"
        >
          ✕
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 space-y-4">
        <!-- Source Settings -->
        <div class="p-3.5 rounded-lg border border-slate-800 bg-[#050811] space-y-3">
          <h4 class="text-xs font-semibold text-sky-400 uppercase tracking-wide">Source Configuration</h4>
          <div class="grid grid-cols-2 gap-3 text-xs">
            <div>
              <label class="block text-slate-400 mb-1">Source Connection *</label>
              <select
                bind:value={sourceConnId}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 focus:outline-none focus:border-sky-500"
              >
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label class="block text-slate-400 mb-1">Table *</label>
              <input
                type="text"
                bind:value={sourceTable}
                placeholder="e.g. users"
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 focus:outline-none focus:border-sky-500"
              />
            </div>
          </div>
        </div>

        <!-- Target Settings -->
        <div class="p-3.5 rounded-lg border border-slate-800 bg-[#050811] space-y-3">
          <h4 class="text-xs font-semibold text-emerald-400 uppercase tracking-wide">Target Configuration</h4>
          <div class="grid grid-cols-2 gap-3 text-xs">
            <div>
              <label class="block text-slate-400 mb-1">Target Connection *</label>
              <select
                bind:value={targetConnId}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 focus:outline-none focus:border-emerald-500"
              >
                <option value="">Select Target Connection...</option>
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label class="block text-slate-400 mb-1">Target Table</label>
              <input
                type="text"
                bind:value={targetTable}
                placeholder="Defaults to source table"
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 focus:outline-none focus:border-emerald-500"
              />
            </div>
          </div>

          <div class="flex items-center justify-between pt-1 text-xs">
            <label class="flex items-center space-x-2 text-slate-300 cursor-pointer">
              <input
                type="checkbox"
                bind:checked={truncateTarget}
                class="rounded border-slate-700 bg-slate-900 text-rose-500 focus:ring-rose-500"
              />
              <span class="text-rose-300">Truncate target table before inserting</span>
            </label>

            <div class="flex items-center space-x-1.5">
              <span class="text-slate-400">Batch Size:</span>
              <input
                type="number"
                bind:value={batchSize}
                min="50"
                max="5000"
                step="50"
                class="w-20 bg-slate-900 border border-slate-700 rounded px-2 py-1 text-right text-slate-100"
              />
            </div>
          </div>
        </div>

        {#if transferResult}
          <div class="p-3 rounded-lg border text-xs {transferResult.success
            ? 'border-emerald-800 bg-emerald-950/30 text-emerald-300'
            : 'border-rose-800 bg-rose-950/30 text-rose-300'}">
            {#if transferResult.success}
              ✓ Transfer completed successfully! Total rows transferred: {transferResult.rows_transferred?.toLocaleString() ?? 0}
            {:else}
              ✕ Transfer failed: {transferResult.error}
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-end space-x-2.5">
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
          disabled={isTransferring || !sourceConnId || !targetConnId || !sourceTable}
          class="px-5 py-1.5 rounded text-xs font-bold bg-sky-600 hover:bg-sky-500 text-white transition shadow flex items-center space-x-1.5 disabled:opacity-50"
        >
          {#if isTransferring}
            <span class="animate-spin text-xs">⟳</span>
            <span>Transferring Rows...</span>
          {:else}
            <span>Start Transfer</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
