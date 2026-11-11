<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { ProcesslistThread } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import ProcessTrendPanel from './ProcessTrendPanel.svelte';

  type DbaSubTab = 'threads' | 'digest' | 'deadlock' | 'cluster' | 'variables';
  type FilterChip = 'all' | 'active' | 'sleep' | 'locked' | 'slow';

  let currentSubTab = $state<DbaSubTab>('threads');
  let threads = $state<ProcesslistThread[]>([]);
  let locks = $state<any[]>([]);
  let currentThreadId = $state<number>(0);
  let isLoading = $state(false);

  // Column Visibility Management (ADR 0054)
  let visibleColumns = $state<Record<string, boolean>>({
    id: true,
    user: true,
    host: true,
    db: true,
    command: true,
    time: true,
    state: true,
    info: true,
  });
  let showColSelector = $state(false);
  let showTrendPanel = $state(true);

  // Threads filtering & selection
  let filterText = $state('');
  let activeChip = $state<FilterChip>('all');
  let selectedThreadIds = $state<Set<number>>(new Set());
  let refreshInterval = $state<number>(0); // 0 = pause, 1000, 2000, 5000
  let intervalTimer: any = null;
  let inspectingThread = $state<ProcesslistThread | null>(null);

  // Statement Digest State
  let digestRows = $state<any[]>([]);
  let isDigestLoading = $state(false);

  // Deadlock State
  let deadlockData = $state<any>(null);
  let isDeadlockLoading = $state(false);

  // Cluster State
  let clusterReport = $state<any>(null);
  let isClusterLoading = $state(false);

  // Variables State
  let variablesReport = $state<any>(null);
  let isVariablesLoading = $state(false);
  let varFilterText = $state('');
  let varCategory = $state('All');

  $effect(() => {
    if (appState.activeSessionId) {
      if (currentSubTab === 'threads') {
        loadProcesslist();
      } else if (currentSubTab === 'digest') {
        loadStatementDigest();
      } else if (currentSubTab === 'deadlock') {
        loadDeadlock();
      } else if (currentSubTab === 'cluster') {
        loadClusterReport();
      } else if (currentSubTab === 'variables') {
        loadVariablesReport();
      }
    }
  });

  $effect(() => {
    if (intervalTimer) {
      clearInterval(intervalTimer);
      intervalTimer = null;
    }
    if (refreshInterval > 0 && currentSubTab === 'threads') {
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
      locks = res.locks || [];
      currentThreadId = res.current_thread_id || 0;
    } catch (err: any) {
      appState.showToast(`Failed to load processlist: ${err.message}`, 'error');
    } finally {
      if (showSpinner) isLoading = false;
    }
  }

  async function loadStatementDigest() {
    if (!appState.activeSessionId) return;
    isDigestLoading = true;
    try {
      const res = await api.getStatementDigest(appState.activeSessionId, 30);
      digestRows = res.digest || res.statements || (Array.isArray(res) ? res : []);
    } catch (err: any) {
      appState.showToast(`Failed to load statement digest: ${err.message}`, 'error');
    } finally {
      isDigestLoading = false;
    }
  }

  async function handleResetDigest() {
    if (!confirm('Are you sure you want to reset statement digest performance statistics?')) return;
    if (!appState.activeSessionId) return;
    try {
      await api.resetStatementDigest(appState.activeSessionId);
      appState.showToast('Reset statement digest stats', 'success');
      await loadStatementDigest();
    } catch (err: any) {
      appState.showToast(`Failed to reset: ${err.message}`, 'error');
    }
  }

  async function loadDeadlock() {
    if (!appState.activeSessionId) return;
    isDeadlockLoading = true;
    try {
      const res = await api.getDeadlock(appState.activeSessionId);
      deadlockData = res;
    } catch (err: any) {
      appState.showToast(`Failed to inspect deadlock: ${err.message}`, 'error');
    } finally {
      isDeadlockLoading = false;
    }
  }

  async function loadClusterReport() {
    if (!appState.activeSessionId) return;
    isClusterLoading = true;
    try {
      const res = await api.getClusterHealth(appState.activeSessionId);
      clusterReport = res;
    } catch (err: any) {
      clusterReport = null;
    } finally {
      isClusterLoading = false;
    }
  }

  async function loadVariablesReport() {
    if (!appState.activeSessionId) return;
    isVariablesLoading = true;
    try {
      const res = await api.getServerVariables(appState.activeSessionId);
      variablesReport = res;
    } catch (err: any) {
      variablesReport = null;
    } finally {
      isVariablesLoading = false;
    }
  }

  async function killSingleThread(id: number) {
    if (!confirm(`Are you sure you want to kill thread ID ${id}?`)) return;
    try {
      await api.killProcess(appState.activeSessionId!, id);
      appState.showToast(`Killed thread ${id}`, 'success');
      selectedThreadIds.delete(id);
      if (inspectingThread?.id === id) inspectingThread = null;
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Failed to kill thread: ${err.message}`, 'error');
    }
  }

  async function handleBatchKill() {
    const ids = Array.from(selectedThreadIds);
    if (ids.length === 0) return;
    if (!confirm(`Kill ${ids.length} selected database thread(s)?`)) return;

    try {
      const res = await api.batchKillProcesses(appState.activeSessionId!, ids, 'connection');
      appState.showToast(`Killed ${res.killed_count || ids.length} threads`, 'success');
      selectedThreadIds.clear();
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Batch kill failed: ${err.message}`, 'error');
    }
  }

  let filteredThreads = $derived.by(() => {
    const q = filterText.toLowerCase().trim();
    return threads.filter((t) => {
      if (q) {
        const match =
          t.user?.toLowerCase().includes(q) ||
          t.host?.toLowerCase().includes(q) ||
          t.db?.toLowerCase().includes(q) ||
          t.command?.toLowerCase().includes(q) ||
          t.state?.toLowerCase().includes(q) ||
          t.info?.toLowerCase().includes(q) ||
          String(t.id).includes(q);
        if (!match) return false;
      }

      if (activeChip === 'active') {
        if (t.command === 'Sleep' || t.state === 'Sleep') return false;
      } else if (activeChip === 'sleep') {
        if (t.command !== 'Sleep' && t.state !== 'Sleep') return false;
      } else if (activeChip === 'locked') {
        const isLocked =
          (t.state && t.state.toLowerCase().includes('lock')) ||
          locks.some((l) => l.waiting_thread_id === t.id || l.blocking_thread_id === t.id);
        if (!isLocked) return false;
      } else if (activeChip === 'slow') {
        if (t.time < 5) return false;
      }

      return true;
    });
  });

  let allFilteredSelected = $derived.by(() => {
    const selectable = filteredThreads.filter((t) => t.id !== currentThreadId);
    if (selectable.length === 0) return false;
    return selectable.every((t) => selectedThreadIds.has(t.id));
  });

  function toggleSelectAll(checked: boolean) {
    const selectable = filteredThreads.filter((t) => t.id !== currentThreadId);
    if (checked) {
      selectable.forEach((t) => selectedThreadIds.add(t.id));
    } else {
      selectable.forEach((t) => selectedThreadIds.delete(t.id));
    }
  }

  function toggleThreadSelection(id: number) {
    if (selectedThreadIds.has(id)) {
      selectedThreadIds.delete(id);
    } else {
      selectedThreadIds.add(id);
    }
  }

  let filteredVariables = $derived.by(() => {
    if (!variablesReport?.variables) return [];
    let list = variablesReport.variables;
    if (varCategory !== 'All') {
      list = list.filter((v: any) => v.category === varCategory);
    }
    if (varFilterText.trim()) {
      const q = varFilterText.toLowerCase();
      list = list.filter(
        (v: any) =>
          v.name.toLowerCase().includes(q) ||
          String(v.value).toLowerCase().includes(q) ||
          (v.description && v.description.toLowerCase().includes(q))
      );
    }
    return list;
  });

  let varCategories = $derived.by(() => {
    if (!variablesReport?.variables) return ['All'];
    const cats = new Set<string>(['All']);
    variablesReport.variables.forEach((v: any) => {
      if (v.category) cats.add(v.category);
    });
    return Array.from(cats);
  });

  function explainThreadQuery(sql?: string) {
    if (!sql) return;
    const tabId = appState.createTab({
      title: 'Explain Plan',
      type: 'worksheet',
      query: `EXPLAIN ${sql};`,
    });
    appState.setActiveTab(tabId);
    appState.executeCurrentTab();
  }

  function sendThreadToAI(sql?: string) {
    if (!sql) return;
    appState.isAiDrawerOpen = true;
    appState.showToast('Thread query sent to AI Copilot context', 'info');
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text font-mono text-xs">
  <!-- Top Navigation & Sub-tabs Bar -->
  <div class="h-10 bg-[#0b0f19] border-b border-[#334155] px-4 flex items-center justify-between text-xs flex-shrink-0 select-none">
    <div class="flex items-center space-x-4">
      <div class="flex items-center space-x-1.5 font-sans font-bold text-slate-200">
        <Icon name="cpu" class="w-4 h-4 text-sky-400" />
        <span>DBA Telemetry</span>
      </div>

      <!-- Sub-Tabs Selector -->
      <div class="flex items-center space-x-1 bg-[#090d16] p-0.5 rounded-lg border border-[#334155]">
        <button
          onclick={() => (currentSubTab = 'threads')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans {currentSubTab === 'threads' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Threads ({threads.length})
        </button>
        <button
          onclick={() => (currentSubTab = 'digest')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans {currentSubTab === 'digest' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Statement Digest
        </button>
        <button
          onclick={() => (currentSubTab = 'deadlock')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans {currentSubTab === 'deadlock' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Deadlock Inspector
        </button>
        <button
          onclick={() => (currentSubTab = 'cluster')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans {currentSubTab === 'cluster' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Cluster & Replication
        </button>
        <button
          onclick={() => (currentSubTab = 'variables')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans {currentSubTab === 'variables' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Server Variables
        </button>
      </div>
    </div>

    <!-- Right Controls for Active SubTab -->
    <div class="flex items-center space-x-3">
      {#if currentSubTab === 'threads'}
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
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh thread list"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'digest'}
        <button
          onclick={handleResetDigest}
          class="px-2.5 py-1 rounded bg-rose-950/60 hover:bg-rose-900 border border-rose-800 text-rose-300 text-xs font-semibold transition flex items-center space-x-1 font-sans"
        >
          <Icon name="trash-2" class="w-3.5 h-3.5" />
          <span>Reset Stats</span>
        </button>
        <button
          onclick={loadStatementDigest}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh statement digest"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isDigestLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'deadlock'}
        <button
          onclick={loadDeadlock}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh deadlock report"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isDeadlockLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'cluster'}
        <button
          onclick={loadClusterReport}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh cluster telemetry"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isClusterLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'variables'}
        <button
          onclick={loadVariablesReport}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh server variables"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isVariablesLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {/if}
    </div>
  </div>

  <!-- Content for Threads SubTab -->
  {#if currentSubTab === 'threads'}
    <div class="h-9 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between text-xs select-none">
      <div class="flex items-center space-x-1 font-sans text-[11px]">
        <button
          onclick={() => (activeChip = 'all')}
          class="px-2 py-0.5 rounded transition {activeChip === 'all' ? 'bg-sky-500/20 text-sky-300 border border-sky-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          All ({threads.length})
        </button>
        <button
          onclick={() => (activeChip = 'active')}
          class="px-2 py-0.5 rounded transition {activeChip === 'active' ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Active
        </button>
        <button
          onclick={() => (activeChip = 'sleep')}
          class="px-2 py-0.5 rounded transition {activeChip === 'sleep' ? 'bg-slate-700 text-slate-200 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Sleep
        </button>
        <button
          onclick={() => (activeChip = 'locked')}
          class="px-2 py-0.5 rounded transition {activeChip === 'locked' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Locked
        </button>
        <button
          onclick={() => (activeChip = 'slow')}
          class="px-2 py-0.5 rounded transition {activeChip === 'slow' ? 'bg-rose-500/20 text-rose-300 border border-rose-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Slow (&gt;5s)
        </button>
      </div>

      <div class="flex items-center space-x-2">
        {#if selectedThreadIds.size > 0}
          <div class="flex items-center space-x-2 bg-rose-950/40 border border-rose-800/80 px-2 py-0.5 rounded text-[11px] animate-in fade-in">
            <span class="text-rose-300 font-semibold">{selectedThreadIds.size} selected</span>
            <button
              onclick={handleBatchKill}
              class="px-2 py-0.5 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold transition flex items-center space-x-1"
            >
              <Icon name="trash-2" class="w-3 h-3" />
              <span>Kill Selected</span>
            </button>
          </div>
        {/if}

        <button
          onclick={() => (showTrendPanel = !showTrendPanel)}
          class="h-7 px-2 rounded border border-[#334155] text-[11px] font-sans flex items-center space-x-1 transition {showTrendPanel ? 'bg-sky-500/20 text-sky-300 border-sky-500/40' : 'bg-[#0b0f19] text-slate-400 hover:text-white'}"
          title="Toggle telemetry trend chart"
        >
          <Icon name="activity" class="w-3 h-3 text-sky-400" />
          <span>Trend</span>
        </button>

        <!-- Column Visibility Dropdown (ADR 0054) -->
        <div class="relative">
          <button
            onclick={(e) => { e.stopPropagation(); showColSelector = !showColSelector; }}
            class="h-7 px-2 rounded bg-[#0b0f19] border border-[#334155] text-slate-300 hover:text-white text-[11px] font-sans flex items-center space-x-1 transition"
            title="Manage column visibility"
          >
            <Icon name="columns" class="w-3 h-3 text-sky-400" />
            <span>Columns</span>
            <Icon name="chevron-down" class="w-2.5 h-2.5 text-slate-400" />
          </button>

          {#if showColSelector}
            <button
              tabindex="-1"
              aria-label="Close"
              class="fixed inset-0 z-30 bg-transparent cursor-default"
              onclick={() => (showColSelector = false)}
            ></button>
            <div
              class="absolute right-0 mt-1 w-44 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-40 p-2 text-xs font-sans space-y-1 animate-in fade-in duration-100"
            >
              <div class="text-[10px] font-bold text-slate-400 uppercase tracking-wider px-1 pb-1 border-b border-slate-800">
                Visible Columns
              </div>
              {#each Object.keys(visibleColumns) as colKey}
                <label class="flex items-center space-x-2 py-1 px-1 rounded hover:bg-slate-800 cursor-pointer text-slate-300 text-[11px]">
                  <input
                    type="checkbox"
                    bind:checked={visibleColumns[colKey]}
                    class="rounded bg-[#090d16] border-[#334155] text-sky-500 focus:ring-0"
                  />
                  <span class="capitalize">{colKey === 'db' ? 'Database' : colKey === 'time' ? 'Time (s)' : colKey === 'info' ? 'Query / Info' : colKey}</span>
                </label>
              {/each}
            </div>
          {/if}
        </div>

        <input
          type="text"
          bind:value={filterText}
          placeholder="Filter thread list..."
          class="bg-[#0b0f19] border border-[#334155] rounded px-2.5 py-1 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-44 font-sans"
        />
      </div>
    </div>

    <!-- Active Threads Telemetry Trend (ADR 0054) -->
    {#if showTrendPanel}
      <div class="border-b border-[#1e293b] shrink-0 bg-[#070b12]">
        <ProcessTrendPanel {threads} />
      </div>
    {/if}

    <!-- Threads Table View -->
    <div class="flex-1 overflow-auto">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] sticky top-0 z-10 border-b border-[#334155] text-slate-400 text-[11px] uppercase tracking-wider font-sans select-none">
          <tr>
            <th class="py-2 px-3 w-8 text-center">
              <input
                type="checkbox"
                checked={allFilteredSelected}
                onchange={(e) => toggleSelectAll((e.target as HTMLInputElement).checked)}
                class="rounded border-slate-700 text-sky-500"
              />
            </th>
            {#if visibleColumns.id}<th class="py-2 px-3 w-16">ID</th>{/if}
            {#if visibleColumns.user}<th class="py-2 px-3 w-28">User</th>{/if}
            {#if visibleColumns.host}<th class="py-2 px-3 w-36">Host</th>{/if}
            {#if visibleColumns.db}<th class="py-2 px-3 w-28">DB</th>{/if}
            {#if visibleColumns.command}<th class="py-2 px-3 w-24">Command</th>{/if}
            {#if visibleColumns.time}<th class="py-2 px-3 w-16 text-right">Time (s)</th>{/if}
            {#if visibleColumns.state}<th class="py-2 px-3 w-36">State</th>{/if}
            {#if visibleColumns.info}<th class="py-2 px-3">Info / Query (Click to Inspect)</th>{/if}
            <th class="py-2 px-3 w-16 text-center">Action</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#if filteredThreads.length === 0}
            <tr>
              <td colspan={Object.values(visibleColumns).filter(Boolean).length + 2} class="p-8 text-center text-slate-500">
                {isLoading ? 'Loading active database threads...' : 'No threads matching the current criteria.'}
              </td>
            </tr>
          {:else}
            {#each filteredThreads as thread}
              {@const isSelf = thread.id === currentThreadId}
              {@const isSelected = selectedThreadIds.has(thread.id)}
              <tr
                class="transition-colors cursor-pointer {isSelected
                  ? 'bg-rose-950/20'
                  : isSelf
                  ? 'bg-sky-950/20'
                  : 'hover:bg-slate-800/40'}"
                onclick={() => (inspectingThread = thread)}
              >
                <td class="py-2 px-3 text-center" onclick={(e) => e.stopPropagation()}>
                  {#if !isSelf}
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onchange={() => toggleThreadSelection(thread.id)}
                      class="rounded border-slate-700 text-rose-500"
                    />
                  {/if}
                </td>
                {#if visibleColumns.id}
                  <td class="py-2 px-3 font-mono font-bold {isSelf ? 'text-sky-400' : 'text-slate-300'}">
                    {thread.id}
                    {#if isSelf}
                      <span class="text-[9px] text-sky-500 uppercase ml-1 block font-sans font-normal">(you)</span>
                    {/if}
                  </td>
                {/if}
                {#if visibleColumns.user}
                  <td class="py-2 px-3 text-slate-200">{thread.user || '-'}</td>
                {/if}
                {#if visibleColumns.host}
                  <td class="py-2 px-3 text-slate-400 truncate max-w-[140px]" title={thread.host}>{thread.host || '-'}</td>
                {/if}
                {#if visibleColumns.db}
                  <td class="py-2 px-3 text-emerald-400">{thread.db || '-'}</td>
                {/if}
                {#if visibleColumns.command}
                  <td class="py-2 px-3 text-amber-300">{thread.command || '-'}</td>
                {/if}
                {#if visibleColumns.time}
                  <td class="py-2 px-3 text-right font-mono {thread.time > 5 ? 'text-rose-400 font-bold' : 'text-slate-300'}">
                    {thread.time}
                  </td>
                {/if}
                {#if visibleColumns.state}
                  <td class="py-2 px-3 text-slate-400 truncate max-w-[140px]" title={thread.state}>{thread.state || '-'}</td>
                {/if}
                {#if visibleColumns.info}
                  <td class="py-2 px-3 text-slate-200 font-mono text-[11px] truncate max-w-md hover:text-sky-300" title={thread.info}>
                    {thread.info || '-'}
                  </td>
                {/if}
                <td class="py-2 px-3 text-center" onclick={(e) => e.stopPropagation()}>
                  {#if !isSelf}
                    <button
                      onclick={() => killSingleThread(thread.id)}
                      class="p-1 rounded text-slate-500 hover:text-rose-400 hover:bg-slate-800 transition"
                      title="Kill this thread"
                    >
                      <Icon name="trash-2" class="w-3.5 h-3.5" />
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  {:else if currentSubTab === 'digest'}
    <!-- Statement Digest View -->
    <div class="flex-1 overflow-auto">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] sticky top-0 z-10 border-b border-[#334155] text-slate-400 text-[11px] uppercase tracking-wider font-sans select-none">
          <tr>
            <th class="py-2 px-3 w-32">Database</th>
            <th class="py-2 px-3">Normalized Statement</th>
            <th class="py-2 px-3 w-24 text-right">Exec Count</th>
            <th class="py-2 px-3 w-28 text-right">Total Latency</th>
            <th class="py-2 px-3 w-28 text-right">Avg Latency</th>
            <th class="py-2 px-3 w-28 text-right">Max Latency</th>
            <th class="py-2 px-3 w-28 text-right">Rows Examined</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#if isDigestLoading}
            <tr>
              <td colspan="7" class="p-8 text-center text-slate-500">
                Aggregating slow query digest from performance_schema...
              </td>
            </tr>
          {:else if digestRows.length === 0}
            <tr>
              <td colspan="7" class="p-8 text-center text-slate-500">
                No slow statement digest statistics recorded yet.
              </td>
            </tr>
          {:else}
            {#each digestRows as row}
              <tr class="hover:bg-slate-800/40 transition-colors">
                <td class="py-2 px-3 text-emerald-400 font-semibold">{row.schema_name || row.db || '-'}</td>
                <td class="py-2 px-3 text-slate-200 font-mono text-[11px] truncate max-w-md" title={row.digest_text || row.query}>
                  {row.digest_text || row.query || '-'}
                </td>
                <td class="py-2 px-3 text-right font-bold text-sky-300">{row.count_star || row.exec_count || 0}</td>
                <td class="py-2 px-3 text-right text-slate-300">{row.sum_timer_wait || row.total_time || '-'}</td>
                <td class="py-2 px-3 text-right font-semibold text-amber-300">{row.avg_timer_wait || row.avg_latency || '-'}</td>
                <td class="py-2 px-3 text-right font-bold text-rose-400">{row.max_timer_wait || row.max_latency || '-'}</td>
                <td class="py-2 px-3 text-right text-slate-400">{row.sum_rows_examined || row.rows_examined || 0}</td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  {:else if currentSubTab === 'deadlock'}
    <!-- Deadlock View -->
    <div class="flex-1 overflow-auto p-4 space-y-4">
      {#if isDeadlockLoading}
        <div class="p-12 text-center text-slate-400 flex flex-col items-center gap-2">
          <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
          <span>Inspecting InnoDB engine status and deadlock logs...</span>
        </div>
      {:else if !deadlockData || !deadlockData.has_deadlock}
        <div class="bg-emerald-950/30 border border-emerald-800/60 rounded-xl p-6 text-center space-y-2">
          <div class="inline-flex p-3 bg-emerald-500/10 rounded-full text-emerald-400">
            <Icon name="check-circle" class="w-8 h-8" />
          </div>
          <h3 class="text-sm font-bold text-emerald-300 font-sans">No Deadlock Detected</h3>
          <p class="text-xs text-slate-400 max-w-md mx-auto font-sans">
            {deadlockData?.summary || 'No deadlocks detected since the database engine was started.'}
          </p>
        </div>
      {:else}
        <div class="bg-rose-950/40 border border-rose-800 rounded-xl p-4 flex items-center justify-between">
          <div class="flex items-center space-x-3">
            <div class="p-2 bg-rose-500/20 text-rose-400 rounded-lg">
              <Icon name="alert-triangle" class="w-6 h-6" />
            </div>
            <div>
              <h3 class="font-bold text-rose-300 font-sans text-sm">
                {deadlockData.summary || 'Deadlock detected between concurrent transactions'}
              </h3>
              {#if deadlockData.detected_at}
                <p class="text-[11px] text-slate-400 font-mono mt-0.5">Detected At: {deadlockData.detected_at}</p>
              {/if}
            </div>
          </div>
        </div>

        {#if deadlockData.transactions && deadlockData.transactions.length > 0}
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            {#each deadlockData.transactions as tx, i}
              <div class="bg-[#0b0f19] border border-[#334155] rounded-xl p-4 space-y-3 shadow-lg">
                <div class="flex items-center justify-between border-b border-[#1e293b] pb-2">
                  <span class="font-bold text-xs text-slate-200 font-sans">Transaction {i + 1}</span>
                  <span class="px-2 py-0.5 rounded text-[10px] font-bold font-sans {tx.rolled_back ? 'bg-rose-950 text-rose-400 border border-rose-800' : 'bg-emerald-950 text-emerald-300 border border-emerald-800'}">
                    {tx.rolled_back ? 'ROLLED BACK' : 'COMMITTED'}
                  </span>
                </div>
                <div class="grid grid-cols-2 gap-2 text-[11px]">
                  <div>
                    <span class="text-slate-500 block">Thread ID:</span>
                    <span class="text-slate-200 font-bold">{tx.thread_id || '-'}</span>
                  </div>
                  <div>
                    <span class="text-slate-500 block">Lock Mode:</span>
                    <span class="text-amber-400 font-bold">{tx.lock_mode || '-'}</span>
                  </div>
                </div>
                <div>
                  <span class="text-slate-500 text-[11px] block mb-1">Query Statement:</span>
                  <pre class="p-2 rounded bg-[#090d16] border border-slate-800 text-[11px] text-sky-300 font-mono overflow-x-auto whitespace-pre-wrap">{tx.query || '-'}</pre>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  {:else if currentSubTab === 'cluster'}
    <!-- Cluster & Replication Telemetry -->
    <div class="flex-1 overflow-auto p-4 space-y-4">
      {#if isClusterLoading}
        <div class="p-12 text-center text-slate-400 flex flex-col items-center gap-2">
          <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
          <span>Introspecting cluster topology and replication status...</span>
        </div>
      {:else if !clusterReport}
        <div class="p-8 text-center text-slate-500">
          No cluster or replication metrics available for this database session.
        </div>
      {:else}
        <!-- Topology Summary Card -->
        <div class="grid grid-cols-4 gap-3">
          <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-xl">
            <span class="text-slate-500 text-[10px] uppercase font-sans block">Topology</span>
            <span class="text-sm font-bold text-sky-400 capitalize">{clusterReport.topology_type || 'Standalone'}</span>
          </div>
          <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-xl">
            <span class="text-slate-500 text-[10px] uppercase font-sans block">Cluster State</span>
            <span class="text-sm font-bold text-emerald-400">{clusterReport.cluster_status || 'Primary / Online'}</span>
          </div>
          <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-xl">
            <span class="text-slate-500 text-[10px] uppercase font-sans block">Cluster Size</span>
            <span class="text-sm font-bold text-slate-200">{clusterReport.cluster_size || 1} node(s)</span>
          </div>
          <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-xl">
            <span class="text-slate-500 text-[10px] uppercase font-sans block">Bottleneck Severity</span>
            <span class="text-sm font-bold {clusterReport.bottleneck_severity === 'critical' ? 'text-rose-400' : clusterReport.bottleneck_severity === 'warning' ? 'text-amber-400' : 'text-emerald-400'} uppercase">
              {clusterReport.bottleneck_severity || 'Optimal'}
            </span>
          </div>
        </div>

        <!-- Nodes Table -->
        {#if clusterReport.nodes && clusterReport.nodes.length > 0}
          <div class="border border-[#1e293b] rounded-xl overflow-hidden bg-[#0b0f19]">
            <div class="px-4 py-2 bg-[#0f172a] border-b border-[#334155] font-sans font-bold text-slate-300 text-xs">
              Cluster Members ({clusterReport.nodes.length})
            </div>
            <table class="w-full text-left border-collapse text-xs">
              <thead class="bg-[#0b0f19] border-b border-[#1e293b] text-slate-400 text-[11px]">
                <tr>
                  <th class="py-2 px-3">Node Name</th>
                  <th class="py-2 px-3">Address</th>
                  <th class="py-2 px-3">State</th>
                  <th class="py-2 px-3 text-right">Recv Queue</th>
                  <th class="py-2 px-3 text-right">Send Queue</th>
                  <th class="py-2 px-3 text-center">Status</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[#1e293b]/60">
                {#each clusterReport.nodes as node}
                  <tr class="hover:bg-slate-800/40">
                    <td class="py-2 px-3 font-bold text-sky-300">{node.name}</td>
                    <td class="py-2 px-3 text-slate-400 font-mono">{node.address}</td>
                    <td class="py-2 px-3 text-emerald-400">{node.state}</td>
                    <td class="py-2 px-3 text-right text-slate-300">{node.receive_queue}</td>
                    <td class="py-2 px-3 text-right text-slate-300">{node.send_queue}</td>
                    <td class="py-2 px-3 text-center">
                      <span class="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-950 text-emerald-300 border border-emerald-800">
                        {node.status}
                      </span>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}

        <!-- Tuning Checks -->
        {#if clusterReport.tuning_checks && clusterReport.tuning_checks.length > 0}
          <div class="space-y-2">
            <span class="font-sans font-bold text-slate-300 text-xs">Cluster Configuration Tuning Checks</span>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              {#each clusterReport.tuning_checks as chk}
                <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-xl space-y-1">
                  <div class="flex items-center justify-between">
                    <span class="font-mono font-bold text-sky-300 text-xs">{chk.variable}</span>
                    <span class="px-1.5 py-0.2 rounded text-[10px] font-bold uppercase {chk.status === 'optimal' ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' : 'bg-amber-950 text-amber-300 border border-amber-800'}">
                      {chk.status}
                    </span>
                  </div>
                  <div class="text-[11px] text-slate-400">
                    Current: <strong class="text-slate-200">{chk.current_value}</strong> • Recommended: <strong class="text-emerald-400">{chk.recommended}</strong>
                  </div>
                  <p class="text-[11px] text-slate-500 font-sans">{chk.summary}</p>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  {:else if currentSubTab === 'variables'}
    <!-- Server Variables View -->
    <div class="flex-1 flex flex-col overflow-hidden">
      <!-- Variables Filter Bar -->
      <div class="h-9 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between text-xs select-none">
        <div class="flex items-center space-x-2">
          <span class="text-slate-400 font-sans text-xs">Category:</span>
          <select
            bind:value={varCategory}
            class="bg-[#0b0f19] border border-[#334155] rounded px-2 py-0.5 text-xs text-sky-400 font-semibold focus:outline-none"
          >
            {#each varCategories as cat}
              <option value={cat}>{cat}</option>
            {/each}
          </select>
        </div>

        <input
          type="text"
          bind:value={varFilterText}
          placeholder="Search variables by name or value..."
          class="bg-[#0b0f19] border border-[#334155] rounded px-2.5 py-1 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-56 font-sans"
        />
      </div>

      <div class="flex-1 overflow-auto">
        <table class="w-full text-left border-collapse text-xs">
          <thead class="bg-[#0f172a] sticky top-0 z-10 border-b border-[#334155] text-slate-400 text-[11px] uppercase tracking-wider font-sans select-none">
            <tr>
              <th class="py-2 px-3 w-64">Variable Name</th>
              <th class="py-2 px-3 w-48">Value</th>
              <th class="py-2 px-3 w-28">Category</th>
              <th class="py-2 px-3 w-20 text-center">Dynamic</th>
              <th class="py-2 px-3">Tuning Recommendation</th>
              <th class="py-2 px-3 w-20 text-center">Snippet</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#1e293b]/60">
            {#if isVariablesLoading}
              <tr>
                <td colspan="6" class="p-8 text-center text-slate-500">
                  Inspecting server variables...
                </td>
              </tr>
            {:else if filteredVariables.length === 0}
              <tr>
                <td colspan="6" class="p-8 text-center text-slate-500">
                  No server variables match your filter.
                </td>
              </tr>
            {:else}
              {#each filteredVariables as v}
                <tr class="hover:bg-slate-800/40">
                  <td class="py-2 px-3 font-mono font-bold text-sky-300">{v.name}</td>
                  <td class="py-2 px-3 font-mono text-slate-200 truncate max-w-[200px]" title={String(v.value)}>{v.value}</td>
                  <td class="py-2 px-3 text-slate-400">{v.category || 'General'}</td>
                  <td class="py-2 px-3 text-center">
                    {#if v.is_dynamic}
                      <span class="px-1.5 py-0.2 rounded bg-emerald-950 text-emerald-300 border border-emerald-800 text-[10px] font-bold">YES</span>
                    {:else}
                      <span class="text-slate-600 text-[10px]">RESTART</span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 text-slate-400 text-[11px] font-sans">
                    {#if v.recommendation || v.suggested_value}
                      <span class="text-amber-300 font-semibold">{v.suggested_value ? `Suggested: ${v.suggested_value}` : ''}</span>
                      <span class="text-slate-400 block">{v.recommendation || v.description}</span>
                    {:else}
                      <span class="text-slate-600">-</span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 text-center">
                    {#if v.set_global_snippet}
                      <button
                        onclick={() => {
                          navigator.clipboard.writeText(v.set_global_snippet);
                          appState.showToast('Copied SET GLOBAL statement to clipboard!', 'info');
                        }}
                        class="p-1 rounded text-sky-400 hover:text-white hover:bg-slate-800 transition"
                        title="Copy SET GLOBAL snippet"
                      >
                        <Icon name="copy" class="w-3.5 h-3.5" />
                      </button>
                    {:else}
                      <span class="text-slate-600">-</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            {/if}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>

<!-- Thread Detail Modal -->
{#if inspectingThread}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-xl shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="cpu" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">Thread Inspector (ID: {inspectingThread.id})</h3>
        </div>
        <button onclick={() => (inspectingThread = null)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div class="grid grid-cols-3 gap-3">
          <div class="p-2.5 bg-[#050811] border border-[#1e293b] rounded-lg">
            <span class="text-slate-500 text-[10px] uppercase block">User @ Host</span>
            <span class="font-bold text-slate-200 truncate block">{inspectingThread.user}@{inspectingThread.host}</span>
          </div>
          <div class="p-2.5 bg-[#050811] border border-[#1e293b] rounded-lg">
            <span class="text-slate-500 text-[10px] uppercase block">Database</span>
            <span class="font-bold text-emerald-400 truncate block">{inspectingThread.db || '-'}</span>
          </div>
          <div class="p-2.5 bg-[#050811] border border-[#1e293b] rounded-lg">
            <span class="text-slate-500 text-[10px] uppercase block">Execution Time</span>
            <span class="font-bold {inspectingThread.time > 5 ? 'text-rose-400' : 'text-slate-200'}">{inspectingThread.time} sec</span>
          </div>
        </div>

        <div>
          <div class="flex items-center justify-between mb-1.5">
            <span class="text-slate-400 text-xs font-medium">SQL Statement</span>
            <div class="flex items-center space-x-2">
              <button
                onclick={() => {
                  navigator.clipboard.writeText(inspectingThread?.info || '');
                  appState.showToast('Thread SQL copied to clipboard', 'info');
                }}
                class="text-[11px] text-sky-400 hover:underline flex items-center space-x-1"
              >
                <Icon name="copy" class="w-3 h-3" />
                <span>Copy SQL</span>
              </button>
              <button
                onclick={() => explainThreadQuery(inspectingThread?.info)}
                class="text-[11px] text-emerald-400 hover:underline flex items-center space-x-1"
              >
                <Icon name="play" class="w-3 h-3" />
                <span>Explain Query</span>
              </button>
              <button
                onclick={() => sendThreadToAI(inspectingThread?.info)}
                class="text-[11px] text-purple-400 hover:underline flex items-center space-x-1"
              >
                <Icon name="bot" class="w-3 h-3" />
                <span>Send to AI</span>
              </button>
            </div>
          </div>
          <pre class="p-3 bg-[#050811] border border-[#1e293b] rounded-xl text-sky-300 font-mono text-xs overflow-x-auto whitespace-pre-wrap max-h-48">{inspectingThread.info || '(No active query statement)'}</pre>
        </div>
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <button
          onclick={() => killSingleThread(inspectingThread!.id)}
          class="px-3.5 py-1.5 rounded bg-rose-600 hover:bg-rose-500 text-white text-xs font-bold transition flex items-center space-x-1.5 shadow-lg shadow-rose-950/30"
        >
          <Icon name="trash-2" class="w-3.5 h-3.5" />
          <span>Kill Thread</span>
        </button>

        <button
          onclick={() => (inspectingThread = null)}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
