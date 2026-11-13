<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  // Format server uptime e.g. Up 2d 4h
  function formatUptime(seconds?: number): string {
    if (!seconds || seconds <= 0) return '';
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    if (d > 0) return `Up ${d}d ${h}h`;
    if (h > 0) return `Up ${h}h ${m}m`;
    return `Up ${m}m`;
  }

  // Engine label & styling. Status-bar metadata stays neutral: semantic colors
  // (amber/rose) are reserved for warnings and destructive states.
  const neutralBadge = 'bg-slate-800/50 border-slate-700/70';
  let engineInfo = $derived.by(() => {
    const driver = appState.activeConnection?.driver?.toLowerCase() || '';
    const ver = appState.serverInfo?.version || '';
    if (driver.includes('postgres') || driver.includes('pg')) {
      return {
        label: ver ? `PostgreSQL ${ver}` : 'PostgreSQL',
        color: 'text-indigo-400',
        badgeBg: neutralBadge,
      };
    }
    if (driver.includes('sqlite')) {
      return {
        label: ver ? `SQLite ${ver}` : 'SQLite 3',
        color: 'text-sky-400',
        badgeBg: neutralBadge,
      };
    }
    if (driver.includes('mysql') || driver.includes('maria')) {
      return {
        label: ver ? `MySQL ${ver}` : 'MySQL',
        color: 'text-cyan-400',
        badgeBg: neutralBadge,
      };
    }
    return {
      label: ver || 'Database Engine',
      color: 'text-slate-400',
      badgeBg: neutralBadge,
    };
  });

  let latencyText = $derived.by(() => {
    if (appState.isPingingServer) return 'Ping...';
    if (appState.serverLatency == null) return '-- ms';
    if (appState.serverLatency < 0) return 'Error';
    return `${appState.serverLatency} ms`;
  });

  let latencyColor = $derived.by(() => {
    if (appState.serverLatency == null) return 'text-slate-500';
    if (appState.serverLatency < 0) return 'text-rose-400';
    if (appState.serverLatency < 60) return 'text-emerald-400';
    if (appState.serverLatency < 200) return 'text-amber-400';
    return 'text-rose-400';
  });
</script>

<footer class="h-7 bg-[#070a13] border-t border-slate-800 px-3 flex items-center justify-between text-[11px] text-slate-400 flex-shrink-0 z-30 select-none overflow-x-auto gap-3">
  <!-- Left: Database Engine & Version, Target Host & DB, Uptime, Latency -->
  <div class="flex items-center space-x-2 shrink-0 truncate">
    {#if appState.activeConnection}
      <!-- Engine & Version Badge -->
      <span
        class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-medium border {engineInfo.badgeBg} shadow-sm"
        title="Server Version: {appState.serverInfo?.version || engineInfo.label}"
      >
        <Icon name="database" class="w-3 h-3 {engineInfo.color}" />
        <span class="font-mono text-slate-200">{engineInfo.label}</span>
      </span>

      <span class="text-slate-600 select-none">•</span>

      <!-- Host Target (database identity lives in the sidebar selector) -->
      <span class="font-mono text-slate-400 truncate" title="Host: {appState.activeConnection.host || 'localhost'}">
        {appState.activeConnection.host || 'localhost'}{appState.activeConnection.port ? `:${appState.activeConnection.port}` : ''}
      </span>

      <!-- Uptime -->
      {#if appState.serverInfo?.uptime_sec}
        <span class="text-slate-600 select-none">•</span>
        <span class="font-mono text-slate-400 truncate hidden md:inline" title="Server Uptime">
          {formatUptime(appState.serverInfo.uptime_sec)}
        </span>
      {/if}

      <!-- Latency & Ping Button -->
      <span class="text-slate-600 select-none">•</span>
      <button
        onclick={() => appState.pingActiveSession()}
        class="inline-flex items-center space-x-1 font-mono transition hover:text-white cursor-pointer group"
        title="Click to ping database session"
      >
        <span class="w-1.5 h-1.5 rounded-full {appState.serverLatency != null && appState.serverLatency >= 0 ? (appState.serverLatency < 60 ? 'bg-emerald-400' : 'bg-amber-400') : 'bg-slate-500'}"></span>
        <span class="{latencyColor} group-hover:underline font-bold">{latencyText}</span>
      </button>

      <!-- In-flight Transaction indicator -->
      {#if appState.isTransactionActive}
        <span class="px-1.5 py-0.2 rounded bg-amber-500/20 text-amber-300 text-[10px] font-bold border border-amber-500/50 flex items-center space-x-1 animate-pulse">
          <span class="w-1.5 h-1.5 rounded-full bg-amber-400"></span>
          <span>TX ACTIVE</span>
        </span>
      {/if}
    {:else}
      <div class="flex items-center space-x-1.5 text-slate-500 font-mono text-[11px]">
        <span class="w-1.5 h-1.5 rounded-full bg-slate-600"></span>
        <span>No active connection</span>
      </div>
    {/if}
  </div>

  <!-- Right: Activity Console, Memory, Docs, Shortcuts, Version -->
  <div class="flex items-center space-x-2 font-mono shrink-0">
    <!-- Live Activity Console Button -->
    <button
      onclick={() => (appState.isActivityDrawerOpen = !appState.isActivityDrawerOpen)}
      class="text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 px-1.5 py-0.5 rounded transition cursor-pointer flex items-center space-x-1.5 select-none"
      title="Toggle Live Activity Console (Ctrl+`)"
    >
      <span class="w-2 h-2 rounded-full {appState.isActivityDrawerOpen ? 'bg-emerald-400 animate-ping' : 'bg-emerald-500'}"></span>
      <span class="font-sans">Activity</span>
    </button>

    <!-- Server Heap Memory -->
    {#if appState.serverMemoryMb != null}
      <span class="text-slate-400 px-1.5 py-0.5 rounded hover:bg-slate-800/60 transition cursor-default select-none hidden sm:inline" title="Server Heap Allocation">
        Mem: {appState.serverMemoryMb} MB
      </span>
    {/if}

    <!-- Documentation Link -->
    <a
      href="https://github.com/herliansyah/sqlfox"
      target="_blank"
      rel="noopener noreferrer"
      class="text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 px-1.5 py-0.5 rounded transition cursor-pointer flex items-center space-x-1 font-sans"
      title="Open SqlFox Documentation"
    >
      <Icon name="book-open" class="w-3 h-3 text-sky-400" />
      <span class="underline decoration-slate-700 underline-offset-2">Docs</span>
    </a>

    <!-- Keyboard Shortcuts -->
    <button
      onclick={() => (appState.isShortcutsModalOpen = true)}
      class="text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 px-1.5 py-0.5 rounded transition cursor-pointer flex items-center space-x-1 font-sans"
      title="View Keyboard Shortcuts (?)"
    >
      <Icon name="keyboard" class="w-3 h-3 text-amber-400" />
      <span class="underline decoration-slate-700 underline-offset-2">Shortcuts</span>
    </button>

    <!-- SqlFox Version & Changelog Dialog -->
    <button
      onclick={() => (appState.isUpdateModalOpen = true)}
      class="text-slate-400 hover:text-slate-200 cursor-pointer transition select-none flex items-center space-x-1 hover:bg-slate-800/60 px-1.5 py-0.5 rounded"
      title="Click to view Changelog & Self-Update"
    >
      <span>SqlFox {appState.appVersion}</span>
    </button>
  </div>
</footer>
