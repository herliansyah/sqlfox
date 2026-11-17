<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  function formatUptime(seconds?: number): string {
    if (!seconds || seconds <= 0) return '';
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    if (d > 0) return `Up ${d}d ${h}h`;
    if (h > 0) return `Up ${h}h ${m}m`;
    return `Up ${m}m`;
  }

  const neutralBadge = 'bg-slate-800/50 border-slate-700/70';
  let engineInfo = $derived.by(() => {
    const driver = appState.activeConnection?.driver?.toLowerCase() || '';
    const ver = appState.serverInfo?.version || '';
    if (driver.includes('postgres') || driver.includes('pg')) {
      return { label: ver ? `PostgreSQL ${ver}` : 'PostgreSQL', color: 'text-indigo-400', badgeBg: neutralBadge };
    }
    if (driver.includes('sqlite')) {
      return { label: ver ? `SQLite ${ver}` : 'SQLite 3', color: 'text-sky-400', badgeBg: neutralBadge };
    }
    if (driver.includes('mysql') || driver.includes('maria')) {
      return { label: ver ? `MySQL ${ver}` : 'MySQL', color: 'text-cyan-400', badgeBg: neutralBadge };
    }
    return { label: ver || 'Database Engine', color: 'text-slate-400', badgeBg: neutralBadge };
  });

  let latencyText = $derived.by(() => {
    if (appState.isPingingServer) return 'Ping...';
    if (appState.serverLatency == null) return '-- ms';
    if (appState.serverLatency < 0) return 'Error';
    if (appState.serverLatency === 0) return '<1ms';
    return `${appState.serverLatency} ms`;
  });

  let latencyColor = $derived.by(() => {
    if (appState.serverLatency == null) return 'text-slate-500';
    if (appState.serverLatency < 0) return 'text-rose-400';
    if (appState.serverLatency < 50) return 'text-emerald-400';
    if (appState.serverLatency < 150) return 'text-amber-400';
    return 'text-rose-400';
  });

  let memoryLabel = $derived.by(() => {
    if (appState.serverMemoryMb != null) return `Mem: ${appState.serverMemoryMb} MB`;
    const perfMem = typeof performance !== 'undefined' ? (performance as any).memory : null;
    if (perfMem?.usedJSHeapSize) {
      return `Mem: ${Math.round(perfMem.usedJSHeapSize / (1024 * 1024))} MB`;
    }
    return 'Mem: --';
  });

  let isStandaloneMode = $derived(
    typeof window !== 'undefined' &&
      (window.matchMedia?.('(display-mode: standalone)').matches || (window.navigator as any)?.standalone || window.outerHeight - window.innerHeight < 80)
  );
</script>

<footer class="h-7 bg-[#070a13] border-t border-slate-800 px-3 flex items-center justify-between text-[11px] text-slate-400 flex-shrink-0 z-30 select-none overflow-x-auto gap-3">
  <!-- Left: Database Engine & Version, Target Host & DB, Uptime, Latency -->
  <div class="flex items-center space-x-2 shrink-0 truncate">
    {#if appState.activeConnection}
      <span
        class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-medium border {engineInfo.badgeBg} shadow-sm"
        title="Server Version: {appState.serverInfo?.version || engineInfo.label}{appState.serverInfo?.active_conns != null ? ` • Active Conns: ${appState.serverInfo.active_conns}` : ''}"
      >
        <Icon name="database" class="w-3 h-3 {engineInfo.color}" />
        <span class="font-mono text-slate-200">{engineInfo.label}</span>
      </span>

      <span class="text-slate-600 select-none">•</span>

      <span class="font-mono text-slate-400 truncate" title="Target Host">
        {appState.activeConnection.host || 'localhost'}{appState.activeConnection.port ? `:${appState.activeConnection.port}` : ''}
      </span>

      {#if appState.serverInfo?.uptime_sec}
        <span class="text-slate-600 select-none">•</span>
        <span class="font-mono text-slate-400 truncate hidden md:inline" title="Server Uptime">
          {formatUptime(appState.serverInfo.uptime_sec)}
        </span>
      {/if}

      <span class="text-slate-600 select-none">•</span>
      <button
        onclick={() => appState.pingActiveSession()}
        class="inline-flex items-center space-x-1 font-mono transition hover:text-white cursor-pointer group"
        title="Click to ping database session"
      >
        <span class="w-1.5 h-1.5 rounded-full {appState.serverLatency != null && appState.serverLatency >= 0 ? (appState.serverLatency < 50 ? 'bg-emerald-400' : appState.serverLatency < 150 ? 'bg-amber-400' : 'bg-rose-400') : 'bg-slate-500'}"></span>
        <span class="{latencyColor} group-hover:underline font-bold">{latencyText}</span>
      </button>

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

  <!-- Right: Activity Console, Memory, Browser Popout, Update Pill, Docs, Shortcuts, Version -->
  <div class="flex items-center space-x-2 font-mono shrink-0">
    <!-- Live Activity Console Button -->
    <button
      onclick={() => appState.toggleActivityDrawer()}
      class="text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 px-1.5 py-0.5 rounded transition cursor-pointer flex items-center space-x-1.5 select-none"
      title="Toggle Live Activity Console (Ctrl+`)"
    >
      <span
        class="w-2 h-2 rounded-full {appState.activityStatus === 'reconnecting'
          ? 'bg-amber-400 animate-pulse'
          : appState.activityStatus === 'paused'
            ? 'bg-slate-500'
            : appState.isActivityDrawerOpen
              ? 'bg-emerald-400 animate-ping'
              : 'bg-emerald-500'}"
      ></span>
      <span class="font-sans">Activity</span>
      {#if appState.unreadActivityErrors > 0}
        <span
          id="footer-activity-err-badge"
          class="px-1 py-0.2 rounded-full bg-rose-500 text-white text-[9px] font-bold leading-none"
        >
          {appState.unreadActivityErrors}
        </span>
      {/if}
    </button>

    <!-- Memory Readout -->
    <span class="text-slate-400 px-1.5 py-0.5 rounded hover:bg-slate-800/60 transition cursor-default select-none hidden sm:inline" title="Server / Client Heap Allocation">
      {memoryLabel}
    </span>

    <!-- Standalone Popout in Browser -->
    {#if isStandaloneMode}
      <button
        id="btn-open-in-browser"
        onclick={() => appState.openInSystemBrowser()}
        class="text-sky-400 hover:text-sky-300 hover:bg-slate-800/60 px-1.5 py-0.5 rounded transition cursor-pointer flex items-center space-x-1 font-sans"
        title="Open SqlFox in System Web Browser"
      >
        <Icon name="share-2" class="w-3 h-3" />
        <span>Browser</span>
      </button>
    {/if}

    <!-- Pulsing Update Available Pill -->
    {#if appState.hasAvailableUpdate}
      <button
        id="footer-update-btn"
        onclick={() => (appState.isUpdateModalOpen = true)}
        class="px-2 py-0.5 rounded-full bg-emerald-500/20 border border-emerald-500/50 text-emerald-300 hover:bg-emerald-500/30 font-sans font-semibold text-[10px] flex items-center space-x-1 animate-pulse transition"
        title="Update {appState.updateInfo?.latest_version} available"
      >
        <Icon name="arrow-down-circle" class="w-3 h-3 text-emerald-400" />
        <span>Update {appState.updateInfo?.latest_version || 'Available'}</span>
      </button>
    {/if}

    <!-- Documentation Link -->
    <a
      href="https://github.com/herliansyah/sqlfox/blob/main/docs/USER_GUIDE.md"
      target="_blank"
      rel="noopener noreferrer"
      class="text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 px-1.5 py-0.5 rounded transition cursor-pointer flex items-center space-x-1 font-sans"
      title="Open SqlFox Documentation on GitHub (Online Manual)"
    >
      <Icon name="book-open" class="w-3 h-3 text-sky-400" />
      <span class="underline decoration-slate-700 underline-offset-2">Docs</span>
    </a>

    <!-- Keyboard Shortcuts -->
    <button
      onclick={() => (appState.isShortcutsModalOpen = true)}
      class="text-slate-400 hover:text-slate-200 hover:bg-slate-800/60 px-1.5 py-0.5 rounded transition cursor-pointer flex items-center space-x-1 font-sans"
      title="View Keyboard Shortcuts (? / F1)"
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
