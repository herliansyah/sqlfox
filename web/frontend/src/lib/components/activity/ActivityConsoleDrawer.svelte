<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { appState, type ActivityLogEntry } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';

  // Stream lifecycle owns its EventSource inside appState.svelte.ts (never declare `eventSource` with $state)
  // so SSE events with `data.module` stay buffered across open/close toggles.
  let consoleMode = $state<'live' | 'disk'>('live');
  let diskLogLoading = $state(false);
  let diskLogError = $state('');
  let diskLogData = $state<{ path: string; size_bytes: number; rotated_count: number; lines: string[] } | null>(null);

  let filterLevel = $state<'ALL' | 'DEBUG' | 'INFO' | 'WARN' | 'ERROR'>('ALL');
  let searchQuery = $state('');
  let autoScroll = $state(true);
  let containerRef = $state<HTMLDivElement | null>(null);
  let expandedJsonIds = new SvelteSet<string>();

  async function loadDiskLogs() {
    diskLogLoading = true;
    diskLogError = '';
    try {
      diskLogData = await api.getLogFile(500);
    } catch (err: any) {
      diskLogError = err?.message || 'Failed to load persistent log file';
    } finally {
      diskLogLoading = false;
    }
  }

  function switchMode(mode: 'live' | 'disk') {
    consoleMode = mode;
    if (mode === 'disk') {
      void loadDiskLogs();
    }
  }

  $effect(() => {
    if (appState.isActivityDrawerOpen) {
      appState.unreadActivityErrors = 0;
    }
  });

  $effect(() => {
    if (appState.isActivityDrawerOpen && autoScroll && appState.activityLogs.length && containerRef) {
      const t = setTimeout(() => {
        containerRef?.scrollTo({ top: containerRef.scrollHeight, behavior: 'smooth' });
      }, 40);
      return () => clearTimeout(t);
    }
  });

  function normLevel(level: string): 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' {
    const up = (level || 'INFO').toUpperCase();
    if (up === 'WARNING') return 'WARN';
    if (up === 'DEBUG' || up === 'WARN' || up === 'ERROR') return up;
    return 'INFO';
  }

  const logSearchCache = new WeakMap<object, string>();
  function getLogSearchText(l: any): string {
    let cached = logSearchCache.get(l);
    if (cached === undefined) {
      const mod = l.module || l.data?.module || '';
      const attrsStr = l.attrs ? JSON.stringify(l.attrs).toLowerCase() : '';
      cached = `${(l.message || '').toLowerCase()} ${mod.toLowerCase()} ${attrsStr}`;
      logSearchCache.set(l, cached);
    }
    return cached;
  }

  let filteredLogs = $derived.by(() => {
    let list = appState.activityLogs;
    if (filterLevel !== 'ALL') {
      list = list.filter((l) => normLevel(l.level) === filterLevel);
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      list = list.filter((l) => getLogSearchText(l).includes(q));
    }
    return list;
  });

  function togglePause() {
    appState.isActivityPaused = !appState.isActivityPaused;
    appState.activityStatus = appState.isActivityPaused ? 'paused' : 'live';
  }

  function clearLogs() {
    appState.activityLogs = [];
    appState.unreadActivityErrors = 0;
    expandedJsonIds.clear();
  }

  function toggleExpandJson(id: string) {
    if (expandedJsonIds.has(id)) expandedJsonIds.delete(id);
    else expandedJsonIds.add(id);
  }

  function downloadFile(content: string, filename: string, mime: string) {
    const blob = new Blob([content], { type: mime });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    appState.showToast(`Exported ${filename}`, 'info');
  }

  function formatLiveLogsText(): string {
    return filteredLogs
      .map(
        (l) =>
          `[${l.timeMs || l.time}] [${l.level}] [${l.module}]: ${l.message}${l.attrs ? ' ' + JSON.stringify(l.attrs) : ''}`
      )
      .join('\n');
  }

  function exportLogsText() {
    downloadFile(formatLiveLogsText(), `sqlfox-activity-${new Date().toISOString().slice(0, 10)}.log`, 'text/plain;charset=utf-8');
  }

  async function copyLiveLogs() {
    const text = formatLiveLogsText();
    if (!text) return;
    await copyText(text);
    appState.showToast(`Copied ${filteredLogs.length} log entries to clipboard`, 'info');
  }

  let filteredDiskLines = $derived.by(() => {
    const lines = diskLogData?.lines || [];
    const q = searchQuery.trim().toLowerCase();
    if (!q) return lines;
    return lines.filter((l) => l.toLowerCase().includes(q));
  });

  async function copyDiskLogs() {
    const text = filteredDiskLines.join('\n');
    if (!text) return;
    await copyText(text);
    appState.showToast(`Copied ${filteredDiskLines.length} persistent log lines to clipboard`, 'info');
  }

  function downloadDiskLogs() {
    const text = filteredDiskLines.join('\n');
    if (!text) return;
    downloadFile(text, `sqlfox-diagnostic-${new Date().toISOString().slice(0, 10)}.log`, 'text/plain;charset=utf-8');
  }

  function exportLogsJson() {
    downloadFile(
      JSON.stringify(filteredLogs, null, 2),
      `sqlfox-activity-${new Date().toISOString().slice(0, 10)}.json`,
      'application/json;charset=utf-8'
    );
  }

  function formatInlineMeta(entry: ActivityLogEntry): string {
    if (!entry.attrs || typeof entry.attrs !== 'object') return '';
    const keys = Object.keys(entry.attrs).slice(0, 3);
    if (keys.length === 0) return '';
    return keys
      .map((k) => {
        const v = entry.attrs![k];
        const s = typeof v === 'string' ? v : JSON.stringify(v);
        return `${k}=${s.length > 36 ? s.slice(0, 33) + '...' : s}`;
      })
      .join(' · ');
  }
</script>

{#if appState.isActivityDrawerOpen}
  <aside
    class="fixed bottom-6 left-0 right-0 h-72 bg-[#090d16] border-t border-[#1e293b] shadow-2xl z-40 flex flex-col font-mono text-xs select-text animate-slide-up"
  >
    <!-- Console Toolbar -->
    <div class="h-9 bg-[#0b0f19] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0 select-none font-sans">
      <div class="flex items-center space-x-2">
        <Icon name="terminal" class="w-4 h-4 text-emerald-400" />
        <span class="font-bold text-slate-100 text-xs">Diagnostic Console</span>
        <div class="flex items-center bg-[#050811] border border-slate-800 rounded p-0.5 text-[10px] font-semibold">
          <button
            type="button"
            onclick={() => switchMode('live')}
            class="px-2 py-0.5 rounded transition {consoleMode === 'live' ? 'bg-slate-800 text-sky-300' : 'text-slate-400 hover:text-slate-200'}"
          >
            Live Stream
          </button>
          <button
            type="button"
            onclick={() => switchMode('disk')}
            class="px-2 py-0.5 rounded transition {consoleMode === 'disk' ? 'bg-slate-800 text-amber-300' : 'text-slate-400 hover:text-slate-200'}"
            title="View persistent rotating log file on disk (survives crashes & restarts)"
          >
            Disk Logs
          </button>
        </div>
        {#if consoleMode === 'live'}
          <span
            class="px-1.5 py-0.5 rounded-full text-[9.5px] font-bold uppercase border flex items-center space-x-1 {appState.activityStatus === 'live'
              ? 'bg-emerald-950/60 border-emerald-800 text-emerald-300'
              : appState.activityStatus === 'reconnecting'
                ? 'bg-amber-950/60 border-amber-800 text-amber-300'
                : 'bg-slate-800 border-slate-700 text-slate-400'}"
          >
            <span class="w-1.5 h-1.5 rounded-full {appState.activityStatus === 'live' ? 'bg-emerald-400 animate-pulse' : appState.activityStatus === 'reconnecting' ? 'bg-amber-400 animate-ping' : 'bg-slate-500'}"></span>
            <span>{appState.activityStatus}</span>
          </span>
          <span class="text-slate-500 font-mono text-[10px]">({filteredLogs.length} / {appState.activityLogs.length})</span>
        {:else if diskLogData}
          <span class="text-slate-400 font-mono text-[10px] truncate max-w-[260px]" title={diskLogData.path}>
            {diskLogData.path} ({(diskLogData.size_bytes / 1024).toFixed(1)} KB)
          </span>
        {/if}
      </div>

      <div class="flex items-center space-x-2 text-xs">
        {#if consoleMode === 'live'}
          <div class="flex items-center space-x-1">
            {#each ['ALL', 'DEBUG', 'INFO', 'WARN', 'ERROR'] as lvl}
              <button
                type="button"
                onclick={() => (filterLevel = lvl as any)}
                class="px-2 py-0.5 rounded text-[10px] font-mono font-bold transition {filterLevel === lvl
                  ? 'bg-slate-800 text-sky-300 border border-slate-700'
                  : 'text-slate-500 hover:text-slate-300'}"
              >
                {lvl}
              </button>
            {/each}
          </div>
        {/if}

        <input
          type="text"
          bind:value={searchQuery}
          placeholder={consoleMode === 'live' ? 'Filter message, module, attrs...' : 'Filter disk log lines...'}
          class="bg-[#050811] border border-[#1e293b] rounded px-2 py-0.5 text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:border-sky-500 w-44"
        />

        {#if consoleMode === 'live'}
          <button
            type="button"
            onclick={togglePause}
            class="p-1 rounded text-slate-400 hover:text-white transition"
            title={appState.isActivityPaused ? 'Resume streaming' : 'Pause streaming'}
          >
            <Icon name={appState.isActivityPaused ? 'play' : 'pause'} class="w-3.5 h-3.5 {!appState.isActivityPaused ? 'text-emerald-400' : 'text-amber-400'}" />
          </button>

          <button
            type="button"
            onclick={() => (autoScroll = !autoScroll)}
            class="p-1 rounded text-slate-400 hover:text-white transition"
            title={autoScroll ? 'Auto-scroll ON' : 'Auto-scroll OFF'}
          >
            <Icon name="arrow-down-circle" class="w-3.5 h-3.5 {autoScroll ? 'text-sky-400' : 'text-slate-600'}" />
          </button>

          <button
            type="button"
            onclick={copyLiveLogs}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-[10px] font-sans font-semibold border border-slate-700 transition flex items-center space-x-1"
            title="Copy filtered logs to clipboard"
          >
            <Icon name="copy" class="w-3 h-3" />
            <span>Copy</span>
          </button>

          <button
            type="button"
            onclick={exportLogsText}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] font-mono border border-slate-700 transition"
            title="Export logs as .log"
          >
            .log
          </button>

          <button
            type="button"
            onclick={exportLogsJson}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] font-mono border border-slate-700 transition"
            title="Export logs as .json"
          >
            .json
          </button>

          <button
            type="button"
            onclick={clearLogs}
            class="p-1 rounded text-slate-400 hover:text-white transition"
            title="Clear console"
          >
            <Icon name="trash-2" class="w-3.5 h-3.5" />
          </button>
        {:else}
          <button
            type="button"
            onclick={() => void loadDiskLogs()}
            disabled={diskLogLoading}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-[10px] font-sans font-semibold border border-slate-700 transition flex items-center space-x-1"
            title="Reload persistent log file from disk"
          >
            <Icon name="refresh-cw" class="w-3 h-3 {diskLogLoading ? 'animate-spin text-sky-400' : ''}" />
            <span>Refresh</span>
          </button>

          <button
            type="button"
            onclick={copyDiskLogs}
            disabled={filteredDiskLines.length === 0}
            class="px-1.5 py-0.5 rounded bg-sky-900/60 hover:bg-sky-800 disabled:opacity-40 text-sky-200 text-[10px] font-sans font-semibold border border-sky-700/70 transition flex items-center space-x-1"
            title="Copy all displayed disk log lines to clipboard"
          >
            <Icon name="copy" class="w-3 h-3" />
            <span>Copy All</span>
          </button>

          <button
            type="button"
            onclick={downloadDiskLogs}
            disabled={filteredDiskLines.length === 0}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 disabled:opacity-40 text-slate-300 text-[10px] font-mono border border-slate-700 transition"
            title="Download persistent log tail as .log"
          >
            .log
          </button>
        {/if}

        <button
          type="button"
          onclick={() => (appState.isActivityDrawerOpen = false)}
          class="p-1 rounded text-slate-400 hover:text-white transition"
          title="Close console"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <!-- Structured Logs Table or Persistent Disk Log Viewer -->
    {#if consoleMode === 'disk'}
      <div class="flex-1 overflow-auto bg-[#050811] p-3">
        {#if diskLogLoading && !diskLogData}
          <div class="p-6 text-center text-slate-500">Loading persistent diagnostic logs...</div>
        {:else if diskLogError}
          <div class="p-4 rounded bg-rose-950/40 border border-rose-800 text-rose-300">{diskLogError}</div>
        {:else if filteredDiskLines.length === 0}
          <div class="p-6 text-center text-slate-500">
            No persistent log entries found{diskLogData?.path ? ` at ${diskLogData.path}` : ''}.
          </div>
        {:else}
          <pre class="font-mono text-[11px] leading-relaxed text-slate-300 whitespace-pre-wrap break-all select-text">{filteredDiskLines.join('\n')}</pre>
        {/if}
      </div>
    {:else}
      <div bind:this={containerRef} class="flex-1 overflow-auto bg-[#050811]">
      {#if filteredLogs.length === 0}
        <div class="p-8 text-center text-slate-600 select-none">
          Live stream active. Waiting for database queries, errors or telemetry events...
        </div>
      {:else}
        <table class="w-full text-left border-collapse font-mono text-[11px]">
          <thead class="sticky top-0 bg-[#0b0f19] border-b border-slate-800 text-[10px] text-slate-500 uppercase font-sans select-none">
            <tr>
              <th class="py-1 px-2.5 w-24">Time</th>
              <th class="py-1 px-2 w-16">Level</th>
              <th class="py-1 px-2 w-24">Module</th>
              <th class="py-1 px-2">Activity &amp; Metadata</th>
              <th class="py-1 px-2 w-16 text-right">Attrs</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-900/80">
            {#each filteredLogs as log (log.id)}
              {@const lvl = normLevel(log.level)}
              {@const meta = formatInlineMeta(log)}
              {@const isOpen = expandedJsonIds.has(log.id)}
              <tr class="hover:bg-slate-900/40 align-top">
                <td class="py-1 px-2.5 text-slate-500 whitespace-nowrap select-none text-[10px]">{log.timeMs || log.time}</td>
                <td class="py-1 px-2 whitespace-nowrap select-none">
                  <span
                    class="px-1.5 py-0.5 rounded text-[9px] font-bold border {lvl === 'ERROR'
                      ? 'bg-rose-950 text-rose-300 border-rose-800'
                      : lvl === 'WARN'
                      ? 'bg-amber-950 text-amber-300 border-amber-800'
                      : lvl === 'DEBUG'
                      ? 'bg-slate-800 text-slate-400 border-slate-700'
                      : 'bg-sky-950 text-sky-300 border-sky-800'}"
                  >
                    {lvl}
                  </span>
                </td>
                <td class="py-1 px-2 whitespace-nowrap select-none">
                  <span class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-slate-400 text-[10px]">
                    {log.module}
                  </span>
                </td>
                <td class="py-1 px-2 text-slate-300 break-words">
                  <span>{log.message}</span>
                  {#if meta}
                    <span class="ml-2 text-[10px] text-slate-500">{meta}</span>
                  {/if}
                  {#if isOpen && log.attrs}
                    <pre class="mt-1 p-2 rounded bg-[#0b0f19] border border-slate-800 text-[10.5px] text-sky-200 overflow-x-auto whitespace-pre-wrap">{JSON.stringify(log.attrs, null, 2)}</pre>
                  {/if}
                </td>
                <td class="py-1 px-2 text-right whitespace-nowrap">
                  {#if log.attrs && Object.keys(log.attrs).length > 0}
                    <button
                      type="button"
                      onclick={() => toggleExpandJson(log.id)}
                      class="px-1.5 py-0.5 rounded border text-[9.5px] {isOpen
                        ? 'bg-sky-950 border-sky-700 text-sky-300'
                        : 'bg-slate-900 border-slate-800 text-slate-400 hover:text-white'}"
                    >
                      {'{ JSON }'}
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
    {/if}
  </aside>
{/if}
