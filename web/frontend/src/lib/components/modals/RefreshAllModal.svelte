<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';

  let activeThreads = $state<number>(0);
  let isLoading = $state(false);
  let acknowledged = $state(false);
  let isRefreshing = $state(false);
  let progressPercent = $state(0);
  let currentDbProgress = $state('');

  $effect(() => {
    if (appState.isRefreshAllOpen && appState.activeSessionId) {
      loadThreadHealth();
    }
  });

  async function loadThreadHealth() {
    if (!appState.activeSessionId) return;
    isLoading = true;
    try {
      const res = await api.getActiveThreads(appState.activeSessionId);
      activeThreads = typeof res?.active_threads === 'number' ? res.active_threads : 1;
    } catch (_) {
      activeThreads = 1;
    } finally {
      isLoading = false;
    }
  }

  async function handleProceed() {
    if (isRefreshing || !appState.activeSessionId) return;
    if (activeThreads >= 10 && !acknowledged) return;

    isRefreshing = true;
    progressPercent = 0;
    const dbs = appState.availableDatabases.length > 0 ? appState.availableDatabases : [appState.currentDatabase];

    for (let i = 0; i < dbs.length; i++) {
      const db = dbs[i];
      currentDbProgress = `Refreshing catalog for '${db}'...`;
      try {
        await api.getSchema(appState.activeSessionId, db);
      } catch (_) {}
      progressPercent = Math.round(((i + 1) / dbs.length) * 100);
    }

    await appState.refreshCatalog();
    appState.showToast(`Successfully refreshed catalogs for all ${dbs.length} databases`, 'success');
    isRefreshing = false;
    appState.isRefreshAllOpen = false;
  }

  function handleCancel() {
    if (isRefreshing) return;
    appState.isRefreshAllOpen = false;
  }
</script>

{#if appState.isRefreshAllOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-lg rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center space-x-2">
          <span class="text-sky-400">🔄</span>
          <h3 class="text-sm font-semibold text-slate-100">
            Refresh All Database Catalogs
          </h3>
        </div>
        <button
          type="button"
          onclick={handleCancel}
          disabled={isRefreshing}
          class="text-slate-400 hover:text-slate-200 text-lg leading-none disabled:opacity-50"
        >
          ✕
        </button>
      </div>

      <!-- Content -->
      <div class="p-5 space-y-4">
        <p class="text-xs text-slate-400">
          This operation will scan the schema dictionary across all available databases on the active server connection.
        </p>

        <!-- Thread Health Indicator -->
        <div class="p-4 rounded-lg border bg-[#050811] {activeThreads < 10
          ? 'border-emerald-800/80 bg-emerald-950/20'
          : activeThreads < 20
            ? 'border-amber-800/80 bg-amber-950/20'
            : 'border-rose-800/80 bg-rose-950/20'}">
          <div class="flex items-center justify-between mb-2">
            <span class="text-xs font-semibold text-slate-300">Active Threads Safety Guard</span>
            <span class="px-2 py-0.5 rounded text-[11px] font-mono font-bold {activeThreads < 10
              ? 'bg-emerald-950 text-emerald-400 border border-emerald-800'
              : activeThreads < 20
                ? 'bg-amber-950 text-amber-400 border border-amber-800'
                : 'bg-rose-950 text-rose-400 border border-rose-800'}">
              {isLoading ? 'Checking...' : `${activeThreads} active thread(s)`}
            </span>
          </div>

          <p class="text-xs {activeThreads < 10
            ? 'text-emerald-300'
            : activeThreads < 20
              ? 'text-amber-300'
              : 'text-rose-300'}">
            {#if activeThreads < 10}
              ✓ Safe Condition: Server workload is low. Full catalog synchronization is safe to execute.
            {:else if activeThreads < 20}
              ⚠ Elevated Workload Warning: Server is processing active queries. Schema refresh may incur slight dictionary I/O.
            {:else}
              🚫 High Workload Alert: High query traffic detected. Refreshing all catalogs concurrently may cause thread queueing.
            {/if}
          </p>
        </div>

        {#if activeThreads >= 10}
          <div class="flex items-start space-x-2 pt-1">
            <input
              id="refresh-all-ack"
              type="checkbox"
              bind:checked={acknowledged}
              class="mt-0.5 rounded border-slate-700 bg-slate-900 text-sky-500 focus:ring-sky-500"
            />
            <label for="refresh-all-ack" class="text-xs text-slate-300 cursor-pointer select-none">
              I acknowledge the active server workload and want to proceed with full catalog refresh.
            </label>
          </div>
        {/if}

        {#if isRefreshing}
          <div class="space-y-1.5 pt-2">
            <div class="flex justify-between text-[11px] text-slate-400 font-mono">
              <span>{currentDbProgress}</span>
              <span>{progressPercent}%</span>
            </div>
            <div class="w-full bg-slate-800 rounded-full h-2 overflow-hidden">
              <div
                class="bg-sky-500 h-2 rounded-full transition-all duration-300"
                style="width: {progressPercent}%"
              ></div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-end space-x-2.5">
        <button
          type="button"
          onclick={handleCancel}
          disabled={isRefreshing}
          class="px-4 py-1.5 rounded text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition disabled:opacity-50"
        >
          Cancel
        </button>

        <button
          type="button"
          onclick={handleProceed}
          disabled={isRefreshing || (activeThreads >= 10 && !acknowledged)}
          class="px-5 py-1.5 rounded text-xs font-bold bg-sky-600 hover:bg-sky-500 text-white transition shadow flex items-center space-x-1.5 disabled:opacity-50"
        >
          {#if isRefreshing}
            <span class="animate-spin text-xs">⟳</span>
            <span>Refreshing...</span>
          {:else}
            <span>Start Refresh All</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
