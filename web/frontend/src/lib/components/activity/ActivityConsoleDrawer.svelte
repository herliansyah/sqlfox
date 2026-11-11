<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  interface LogEntry {
    id: string;
    timestamp: Date;
    level: 'INFO' | 'WARN' | 'ERROR';
    source: string;
    message: string;
  }

  let logs = $state<LogEntry[]>([]);
  let filterLevel = $state<'ALL' | 'INFO' | 'WARN' | 'ERROR'>('ALL');
  let searchQuery = $state('');
  let isStreaming = $state(true);
  let autoScroll = $state(true);
  let containerRef = $state<HTMLDivElement | null>(null);
  let eventSource = $state<EventSource | null>(null);

  function startLogStream() {
    if (eventSource) {
      eventSource.close();
    }

    try {
      eventSource = new EventSource('/api/logs/stream');
      eventSource.onmessage = (event) => {
        if (!isStreaming) return;
        try {
          const data = JSON.parse(event.data);
          const entry: LogEntry = {
            id: String(Date.now()) + Math.random(),
            timestamp: data.timestamp ? new Date(data.timestamp) : new Date(),
            level: data.level?.toUpperCase() || 'INFO',
            source: data.source || 'SYSTEM',
            message: data.message || event.data,
          };
          logs = [...logs.slice(-500), entry]; // Keep last 500 logs
        } catch (e) {
          const entry: LogEntry = {
            id: String(Date.now()) + Math.random(),
            timestamp: new Date(),
            level: 'INFO',
            source: 'SYSTEM',
            message: event.data,
          };
          logs = [...logs.slice(-500), entry];
        }
      };

      eventSource.onerror = () => {
        // Fallback retry
      };
    } catch (e) {}
  }

  function stopLogStream() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
  }

  $effect(() => {
    if (appState.isActivityDrawerOpen) {
      startLogStream();
    } else {
      stopLogStream();
    }
    return () => stopLogStream();
  });

  $effect(() => {
    if (autoScroll && logs.length && containerRef) {
      setTimeout(() => {
        containerRef?.scrollTo({ top: containerRef.scrollHeight, behavior: 'smooth' });
      }, 50);
    }
  });

  let filteredLogs = $derived.by(() => {
    let list = logs;
    if (filterLevel !== 'ALL') {
      list = list.filter((l) => l.level === filterLevel);
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      list = list.filter(
        (l) =>
          l.message.toLowerCase().includes(q) ||
          l.source.toLowerCase().includes(q)
      );
    }
    return list;
  });

  function clearLogs() {
    logs = [];
  }

  function exportLogs() {
    const content = filteredLogs
      .map(
        (l) =>
          `[${l.timestamp.toISOString()}] [${l.level}] [${l.source}]: ${l.message}`
      )
      .join('\n');
    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `sqlfox-activity-logs-${new Date().toISOString().slice(0, 10)}.txt`;
    a.click();
    URL.revokeObjectURL(url);
    appState.showToast('Activity logs exported successfully!', 'info');
  }
</script>

{#if appState.isActivityDrawerOpen}
  <aside
    class="fixed bottom-6 left-0 right-0 h-64 bg-[#090d16] border-t border-[#1e293b] shadow-2xl z-40 flex flex-col font-mono text-xs select-text animate-slide-up"
  >
    <!-- Console Toolbar -->
    <div class="h-9 bg-[#0b0f19] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0 select-none font-sans">
      <div class="flex items-center space-x-2">
        <Icon name="terminal" class="w-4 h-4 text-emerald-400" />
        <span class="font-bold text-slate-100 text-xs">Live Activity Console</span>
        <span class="w-2 h-2 rounded-full {isStreaming ? 'bg-emerald-500 animate-pulse' : 'bg-slate-600'}"></span>
        <span class="text-slate-500 font-mono text-[10px]">({filteredLogs.length} events)</span>
      </div>

      <div class="flex items-center space-x-2 text-xs">
        <div class="flex items-center space-x-1">
          {#each ['ALL', 'INFO', 'WARN', 'ERROR'] as lvl}
            <button
              onclick={() => (filterLevel = lvl as any)}
              class="px-2 py-0.5 rounded text-[10px] font-mono font-bold transition {filterLevel === lvl
                ? 'bg-slate-800 text-sky-300 border border-slate-700'
                : 'text-slate-500 hover:text-slate-300'}"
            >
              {lvl}
            </button>
          {/each}
        </div>

        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Filter logs..."
          class="bg-[#050811] border border-[#1e293b] rounded px-2 py-0.5 text-xs text-slate-200 placeholder-slate-600 focus:outline-none w-36"
        />

        <button
          onclick={() => (isStreaming = !isStreaming)}
          class="p-1 rounded text-slate-400 hover:text-white transition"
          title={isStreaming ? 'Pause streaming' : 'Resume streaming'}
        >
          <Icon name={isStreaming ? 'pause' : 'play'} class="w-3.5 h-3.5 {isStreaming ? 'text-emerald-400' : 'text-slate-500'}" />
        </button>

        <button
          onclick={() => (autoScroll = !autoScroll)}
          class="p-1 rounded text-slate-400 hover:text-white transition"
          title={autoScroll ? 'Auto-scroll ON' : 'Auto-scroll OFF'}
        >
          <Icon name="arrow-down-circle" class="w-3.5 h-3.5 {autoScroll ? 'text-sky-400' : 'text-slate-600'}" />
        </button>

        <button
          onclick={exportLogs}
          class="p-1 rounded text-slate-400 hover:text-white transition"
          title="Export logs to file"
        >
          <Icon name="download" class="w-3.5 h-3.5" />
        </button>

        <button
          onclick={clearLogs}
          class="p-1 rounded text-slate-400 hover:text-white transition"
          title="Clear console"
        >
          <Icon name="trash-2" class="w-3.5 h-3.5" />
        </button>

        <button
          onclick={() => (appState.isActivityDrawerOpen = false)}
          class="p-1 rounded text-slate-400 hover:text-white transition"
          title="Close console"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <!-- Logs Stream Viewport -->
    <div
      bind:this={containerRef}
      class="flex-1 overflow-y-auto p-3 space-y-1 bg-[#050811] font-mono text-[11px] leading-relaxed"
    >
      {#if filteredLogs.length === 0}
        <div class="p-8 text-center text-slate-600 select-none">
          Live stream active. Waiting for database queries, errors or background synchronization events...
        </div>
      {:else}
        {#each filteredLogs as log}
          <div class="flex items-start space-x-2 py-0.5 hover:bg-slate-900/40 px-1 rounded">
            <span class="text-slate-600 select-none shrink-0 text-[10px]">
              {log.timestamp.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })}
            </span>
            <span
              class="px-1 py-0.2 rounded text-[9px] font-bold select-none shrink-0 {log.level === 'ERROR'
                ? 'bg-rose-950 text-rose-300 border border-rose-800'
                : log.level === 'WARN'
                ? 'bg-amber-950 text-amber-300 border border-amber-800'
                : 'bg-sky-950 text-sky-300 border border-sky-800'}"
            >
              {log.level}
            </span>
            <span class="text-slate-500 font-bold select-none shrink-0">[{log.source}]</span>
            <span class="text-slate-300 break-all">{log.message}</span>
          </div>
        {/each}
      {/if}
    </div>
  </aside>
{/if}
