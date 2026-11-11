<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { ProcesslistThread } from '../../../types';
  import Icon from '../common/Icon.svelte';

  let threads = $state<ProcesslistThread[]>([]);
  let currentThreadId = $state<number>(0);
  let isLoading = $state(false);
  let filterText = $state('');
  let refreshInterval = $state<number>(0); // 0 = pause, 1000, 2000, 5000
  let intervalTimer: any = null;

  $effect(() => {
    if (appState.activeSessionId) {
      loadProcesslist();
    }
  });

  $effect(() => {
    if (intervalTimer) {
      clearInterval(intervalTimer);
      intervalTimer = null;
    }
    if (refreshInterval > 0) {
      intervalTimer = setInterval(() => {
        loadProcesslist(false);
      }, refreshInterval);
    }
    return () => {
      if (intervalTimer) clearInterval(intervalTimer);
    };
  });

  async function loadProcesslist(showSpinner = true) {
    if (!appState.activeSessionId) return;
    if (showSpinner) isLoading = true;
    try {
      const res = await api.getProcesslist(appState.activeSessionId);
      threads = res.threads || [];
      currentThreadId = res.current_thread_id || 0;
    } catch (err: any) {
      appState.showToast(`Failed to load processlist: ${err.message}`, 'error');
    } finally {
      if (showSpinner) isLoading = false;
    }
  }

  async function killThread(id: number) {
    if (!confirm(`Are you sure you want to kill thread ID ${id}?`)) return;
    try {
      await api.killProcess(appState.activeSessionId!, id);
      appState.showToast(`Killed thread ${id}`, 'success');
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Failed to kill thread: ${err.message}`, 'error');
    }
  }

  let filteredThreads = $derived.by(() => {
    const q = filterText.toLowerCase().trim();
    if (!q) return threads;
    return threads.filter(
      (t) =>
        t.user?.toLowerCase().includes(q) ||
        t.host?.toLowerCase().includes(q) ||
        t.db?.toLowerCase().includes(q) ||
        t.command?.toLowerCase().includes(q) ||
        t.state?.toLowerCase().includes(q) ||
        t.info?.toLowerCase().includes(q)
    );
  });
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text font-mono text-xs">
  <!-- Toolbar -->
  <div class="h-10 bg-[#0b0f19] border-b border-[#334155] px-4 flex items-center justify-between text-xs flex-shrink-0 select-none">
    <div class="flex items-center space-x-3">
      <div class="flex items-center space-x-1.5 font-sans font-bold text-slate-200">
        <Icon name="cpu" class="w-4 h-4 text-sky-400" />
        <span>Processlist ({threads.length} active threads)</span>
      </div>

      <!-- Auto Refresh Control -->
      <div class="flex items-center space-x-1.5 bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-[11px]">
        <span class="text-slate-400 font-sans">Refresh:</span>
        <select
          bind:value={refreshInterval}
          class="bg-transparent text-sky-400 font-semibold focus:outline-none"
        >
          <option value={0}>Paused</option>
          <option value={1000}>1s</option>
          <option value={2000}>2s</option>
          <option value={5000}>5s</option>
        </select>
      </div>

      <button
        onclick={() => loadProcesslist(true)}
        class="p-1 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
        title="Refresh now"
      >
        <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin text-sky-400' : ''}" />
      </button>
    </div>

    <!-- Search filter -->
    <div class="w-64">
      <input
        type="text"
        bind:value={filterText}
        placeholder="Filter queries, users, states..."
        class="w-full bg-[#090d16] border border-[#334155] rounded px-2.5 py-1 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500"
      />
    </div>
  </div>

  <!-- Table View -->
  <div class="flex-1 overflow-auto">
    <table class="w-full text-left border-collapse min-w-full">
      <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans sticky top-0 z-10">
        <tr>
          <th class="py-2 px-3">ID</th>
          <th class="py-2 px-3">User</th>
          <th class="py-2 px-3">Host</th>
          <th class="py-2 px-3">DB</th>
          <th class="py-2 px-3">Command</th>
          <th class="py-2 px-3 text-right">Time</th>
          <th class="py-2 px-3">State</th>
          <th class="py-2 px-3 w-1/3">Query / Info</th>
          <th class="py-2 px-3 text-center">Action</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-[#1e293b]/60">
        {#if filteredThreads.length === 0}
          <tr>
            <td colspan="9" class="p-8 text-center text-slate-500 italic">
              {isLoading ? 'Loading active database processes...' : 'No active threads matching filter'}
            </td>
          </tr>
        {:else}
          {#each filteredThreads as t (t.id)}
            {@const isSelf = t.id === currentThreadId}
            <tr class="hover:bg-sky-500/5 transition-colors {t.time > 10 ? 'bg-amber-950/20' : ''}">
              <td class="py-2 px-3 font-bold text-slate-200">{t.id}</td>
              <td class="py-2 px-3 text-sky-300 font-semibold">{t.user}</td>
              <td class="py-2 px-3 text-slate-400 truncate max-w-[120px]">{t.host}</td>
              <td class="py-2 px-3 text-emerald-400">{t.db || '-'}</td>
              <td class="py-2 px-3 text-slate-300">{t.command}</td>
              <td class="py-2 px-3 text-right font-bold {t.time > 30 ? 'text-rose-400' : t.time > 5 ? 'text-amber-400' : 'text-slate-300'}">
                {t.time}s
              </td>
              <td class="py-2 px-3 text-slate-400 truncate max-w-[150px]">{t.state || '-'}</td>
              <td class="py-2 px-3 text-slate-200 font-mono text-[11px] truncate max-w-sm" title={t.info}>
                {t.info || '<idle>'}
              </td>
              <td class="py-2 px-3 text-center">
                {#if isSelf}
                  <span class="text-[10px] text-slate-500 font-sans italic">Current</span>
                {:else}
                  <button
                    onclick={() => killThread(t.id)}
                    class="px-2 py-0.5 bg-rose-950/60 hover:bg-rose-900 border border-rose-800 text-rose-300 rounded text-[10px] font-sans font-semibold transition"
                    title="Kill thread"
                  >
                    Kill
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        {/if}
      </tbody>
    </table>
  </div>
</div>
