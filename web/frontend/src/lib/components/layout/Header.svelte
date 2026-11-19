<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { tick, onDestroy } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import type { ConnectionProfile } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import { viewportFit } from '../../actions/viewportFit';

  let showConnDropdown = $state(false);
  let showMasterPin = $state(false);
  let copiedPin = $state(false);
  let connSearchQuery = $state('');
  let connActiveIndex = $state(0);
  let connSearchInput = $state<HTMLInputElement | null>(null);
  let connMenuList = $state<HTMLDivElement | null>(null);
  let pinAutoMaskTimer: any = null;

  interface ToolMenuItem {
    label: string;
    desc: string;
    icon: string;
    iconColor: string;
    badge?: string | number;
    badgeClass?: string;
    hidden?: boolean;
    action: () => void;
  }

  let openToolsMenu = $state<'monitor' | 'dbtools' | 'settings' | null>(null);

  const navBtn =
    'h-7 px-2 rounded-md bg-[#1e293b] border border-slate-700/80 text-slate-300 hover:bg-slate-700 hover:text-white hover:border-slate-600 transition shadow-sm flex items-center gap-1.5';
  const navIconBtn = navBtn + ' w-7 justify-center px-0';
  const navDivider = 'w-px h-4 bg-slate-800 shrink-0';

  let isSqlite = $derived((appState.activeConnection?.driver || '').toLowerCase() === 'sqlite');
  let hasConn = $derived(Boolean(appState.activeConnection));

  let monitorItems = $derived<ToolMenuItem[]>([
    {
      label: 'Processlist',
      desc: 'Live threads, slow queries and kill',
      icon: 'cpu',
      iconColor: 'text-sky-400',
      badge: appState.dbaSlowQueriesCount > 0 ? `${appState.dbaSlowQueriesCount} slow` : undefined,
      badgeClass: 'bg-amber-500/20 text-amber-300 border-amber-500/40',
      hidden: !hasConn || isSqlite,
      action: () => appState.openProcesslistTab('threads'),
    },
    {
      label: 'Lock Contention & Blockers',
      desc: 'Blocking chains and root lock holders',
      icon: 'lock',
      iconColor: 'text-amber-400',
      badge: appState.dbaBlockersCount > 0 ? appState.dbaBlockersCount : undefined,
      badgeClass: 'bg-rose-500 text-white',
      hidden: !hasConn || isSqlite,
      action: () => (appState.isLockInspectorOpen = true),
    },
    {
      label: 'Statement Digest',
      desc: 'Top slow normalized queries & latency',
      icon: 'trending-up',
      iconColor: 'text-cyan-400',
      hidden: !hasConn || isSqlite,
      action: () => appState.openProcesslistTab('digest'),
    },
    {
      label: 'Deadlock Inspector',
      desc: 'InnoDB deadlock post-mortem & lock matrix',
      icon: 'shield-alert',
      iconColor: 'text-rose-400',
      hidden: !hasConn || isSqlite,
      action: () => appState.openProcesslistTab('deadlock'),
    },
    {
      label: 'Cluster & Replication',
      desc: 'Replication lag and node state',
      icon: 'network',
      iconColor: 'text-emerald-400',
      badge: appState.dbaClusterLabel || undefined,
      badgeClass:
        appState.dbaClusterStatus === 'critical'
          ? 'bg-rose-500/20 text-rose-300 border-rose-500/50'
          : appState.dbaClusterStatus === 'warn'
            ? 'bg-amber-500/20 text-amber-300 border-amber-500/50'
            : 'bg-emerald-500/20 text-emerald-300 border-emerald-500/50',
      hidden: !hasConn || isSqlite,
      action: () => appState.openProcesslistTab('cluster'),
    },
    {
      label: 'Server Variables',
      desc: 'Runtime parameters and tuning hints',
      icon: 'sliders',
      iconColor: 'text-cyan-400',
      hidden: !hasConn || isSqlite,
      action: () => appState.openProcesslistTab('variables'),
    },
    {
      label: 'Flight Recorder',
      desc: 'Record performance telemetry over time',
      icon: 'disc',
      iconColor: 'text-rose-400',
      badge: appState.flightRecordingActive ? 'REC' : undefined,
      badgeClass: 'bg-rose-500 text-white animate-pulse',
      hidden: !hasConn || isSqlite,
      action: () => appState.openProcesslistTab('flight-recorder'),
    },
    {
      label: 'Storage & Health',
      desc: 'Database & table footprints and health advisor',
      icon: 'hard-drive',
      iconColor: 'text-amber-400',
      hidden: !hasConn,
      action: () => appState.openDbDiagnosticsModal(),
    },
  ].filter((item) => !item.hidden));

  let dbToolItems = $derived<ToolMenuItem[]>([
    {
      label: 'Cross-DB Schema Search (Ctrl+P)',
      desc: 'Jump to tables, views and columns across DBs',
      icon: 'search',
      iconColor: 'text-sky-400',
      action: () => (appState.isSchemaSearchOpen = true),
    },
    {
      label: 'Dump Entire Database',
      desc: 'Export selected tables DDL + INSERTs (.sql)',
      icon: 'download',
      iconColor: 'text-sky-400',
      action: () => appState.openDumpModal(),
    },
    {
      label: 'Export Smart Seed (FK-Aware)',
      desc: 'Topologically ordered bounded test dataset',
      icon: 'sparkles',
      iconColor: 'text-emerald-400',
      action: () => (appState.isSmartSeedModalOpen = true),
    },
    {
      label: 'Run Large SQL Script',
      desc: 'Stream and execute multi-megabyte .sql',
      icon: 'file-code',
      iconColor: 'text-indigo-400',
      action: () => appState.openRunSqlModal(),
    },
    {
      label: 'Structure Sync (Schema Diff)',
      desc: 'Compare schemas and generate migration DDL',
      icon: 'git-diff',
      iconColor: 'text-emerald-400',
      action: () => appState.openStructureSyncModal(),
    },
    {
      label: 'Cross-Connection Transfer',
      desc: 'Stream copy tables between sessions',
      icon: 'arrow-left-right',
      iconColor: 'text-cyan-400',
      action: () => (appState.isDataTransferOpen = true),
    },
    {
      label: 'Row-Level Data Sync',
      desc: 'Keyset delta reconciler (insert/update/delete)',
      icon: 'refresh-cw',
      iconColor: 'text-purple-400',
      action: () => (appState.isDataSyncOpen = true),
    },
    {
      label: 'Visual ERD Canvas',
      desc: 'Interactive ER diagram with FK relations',
      icon: 'layers',
      iconColor: 'text-indigo-400',
      action: () => appState.openErdTab(),
    },
    {
      label: 'Users & Privileges',
      desc: 'Database accounts and permission matrix',
      icon: 'users',
      iconColor: 'text-rose-400',
      hidden: !hasConn || isSqlite,
      action: () => appState.openPrivilegeMatrixModal(),
    },
  ].filter((item) => !item.hidden));

  const settingsItems: ToolMenuItem[] = [
    { label: 'Settings', desc: 'Vault backup, cloud sync, notes and LAN PIN', icon: 'settings', iconColor: 'text-cyan-400', action: () => (appState.isSettingsModalOpen = true) },
    { label: 'AI Model Config', desc: 'Provider credentials, privacy firewall and prompt', icon: 'bot', iconColor: 'text-indigo-400', action: () => (appState.isAiSettingsModalOpen = true) },
    { label: 'Keyboard Shortcuts', desc: 'Hotkeys for navigation and execution', icon: 'keyboard', iconColor: 'text-amber-400', action: () => (appState.isShortcutsModalOpen = true) },
    { label: 'Check for Updates', desc: 'Latest release notes and self-update', icon: 'arrow-down-circle', iconColor: 'text-emerald-400', action: () => (appState.isUpdateModalOpen = true) },
    { label: 'Open in Web Browser', desc: 'Launch this UI in your default browser', icon: 'share-2', iconColor: 'text-sky-400', action: () => appState.openInSystemBrowser() },
    { label: 'Copy App URL', desc: 'Share this SqlFox address', icon: 'copy', iconColor: 'text-slate-300', action: () => appState.copyAppUrl() },
    { label: 'Shut Down Daemon', desc: 'Gracefully stop the local server', icon: 'rotate-ccw', iconColor: 'text-rose-400', action: () => appState.shutdownDaemon() },
  ];

  let masterPin = $derived(appState.authStatus?.master_password || '');
  let isLanAvailable = $derived(
    Boolean(masterPin && (appState.authStatus?.lan_enabled || !appState.authStatus?.is_remote))
  );

  let filteredConnections = $derived.by(() => {
    const q = connSearchQuery.toLowerCase().trim();
    if (!q) return appState.connections;
    return appState.connections.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        (c.host && c.host.toLowerCase().includes(q)) ||
        (c.driver && c.driver.toLowerCase().includes(q)) ||
        (c.environment && c.environment.toLowerCase().includes(q))
    );
  });

  let groupedConnections = $derived.by(() => {
    const groups: { env: 'Production' | 'Staging' | 'Development'; items: typeof appState.connections }[] = [
      { env: 'Production', items: [] },
      { env: 'Staging', items: [] },
      { env: 'Development', items: [] },
    ];
    filteredConnections.forEach((conn) => {
      const g = groups.find((grp) => grp.env === conn.environment) || groups[2];
      g.items.push(conn);
    });
    return groups.filter((g) => g.items.length > 0);
  });

  let orderedConnections = $derived(groupedConnections.flatMap((g) => g.items));

  $effect(() => {
    if (connActiveIndex >= orderedConnections.length) {
      connActiveIndex = Math.max(0, orderedConnections.length - 1);
    }
  });

  async function openConnDropdown() {
    showConnDropdown = true;
    connSearchQuery = '';
    const idx = orderedConnections.findIndex((c) => c.id === appState.activeConnection?.id);
    connActiveIndex = idx >= 0 ? idx : 0;
    await tick();
    connSearchInput?.focus();
  }

  function closeConnDropdown(restoreFocus = false) {
    showConnDropdown = false;
    connSearchQuery = '';
    if (restoreFocus) document.getElementById('session-switcher-btn')?.focus();
  }

  function scrollConnActive() {
    tick().then(() =>
      connMenuList?.querySelector('[data-conn-active="true"]')?.scrollIntoView({ block: 'nearest' })
    );
  }

  function chooseConnection(conn: ConnectionProfile) {
    appState.selectConnection(conn);
    closeConnDropdown();
  }

  async function handleDeleteConnection(e: MouseEvent, conn: ConnectionProfile) {
    e.stopPropagation();
    const ok = await appState.showNativeConfirm(
      `Delete connection profile "${conn.name}"? This cannot be undone.`,
      'Delete Connection'
    );
    if (!ok) return;
    await appState.deleteConnection(conn.id);
  }

  function handleConnSearchKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (orderedConnections.length) {
        connActiveIndex = (connActiveIndex + 1) % orderedConnections.length;
        scrollConnActive();
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (orderedConnections.length) {
        connActiveIndex = (connActiveIndex - 1 + orderedConnections.length) % orderedConnections.length;
        scrollConnActive();
      }
    } else if (e.key === 'Home') {
      e.preventDefault();
      connActiveIndex = 0;
      scrollConnActive();
    } else if (e.key === 'End') {
      e.preventDefault();
      connActiveIndex = Math.max(0, orderedConnections.length - 1);
      scrollConnActive();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const conn = orderedConnections[connActiveIndex];
      if (conn) chooseConnection(conn);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      closeConnDropdown(true);
    } else if (e.key === 'Tab') {
      closeConnDropdown();
    }
  }

  function getEnvBadgeClass(env: string) {
    switch (env) {
      case 'Production':
        return 'bg-rose-500/15 text-rose-300 border-rose-500/30';
      case 'Staging':
        return 'bg-amber-500/15 text-amber-300 border-amber-500/30';
      default:
        return 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30';
    }
  }

  function getDriverName(driver?: string) {
    switch (driver) {
      case 'postgres':
      case 'postgresql':
        return 'PostgreSQL';
      case 'sqlite':
        return 'SQLite';
      default:
        return 'MySQL';
    }
  }

  function toggleMasterPin() {
    showMasterPin = !showMasterPin;
    if (pinAutoMaskTimer) clearTimeout(pinAutoMaskTimer);
    if (showMasterPin) {
      pinAutoMaskTimer = setTimeout(() => {
        showMasterPin = false;
      }, 10000);
    }
  }

  let copiedPinTimer: ReturnType<typeof setTimeout> | null = null;

  function copyMasterPin() {
    if (!masterPin) return;
    copyText(masterPin);
    copiedPin = true;
    appState.showToast('Master PIN copied to clipboard!', 'info');
    if (copiedPinTimer) clearTimeout(copiedPinTimer);
    copiedPinTimer = setTimeout(() => (copiedPin = false), 2000);
  }

  onDestroy(() => {
    if (pinAutoMaskTimer) clearTimeout(pinAutoMaskTimer);
    if (copiedPinTimer) clearTimeout(copiedPinTimer);
  });
</script>

<header class="h-12 bg-[#0f172a] border-b border-slate-800 flex items-center gap-3 px-4 z-40 flex-shrink-0 select-none">
  <!-- Brand, Sidebar Toggle & Active Connection -->
  <div class="flex items-center gap-2 shrink-0 min-w-0">
    <button
      onclick={() => appState.toggleSidebarCollapsed()}
      class="{navIconBtn} text-slate-400"
      title="Toggle Sidebar (Ctrl+B)"
    >
      <Icon name="sidebar" class="w-3.5 h-3.5" />
    </button>

    <!-- SqlFox Logo -->
    <div class="flex items-center gap-2">
      <Icon name="brand" class="w-6 h-6" />
      <span class="font-bold tracking-wide text-white text-base">Sql<span class="text-sky-400">Fox</span></span>
    </div>

    <div class={navDivider}></div>

    <!-- Active Connection Selector & Quick Edit Pill -->
    <div class="relative flex items-center gap-1">
      <button
        id="session-switcher-btn"
        onclick={() => (showConnDropdown ? closeConnDropdown() : openConnDropdown())}
        aria-haspopup="listbox"
        aria-expanded={showConnDropdown}
        class="h-7 px-2 rounded-md bg-[#1e293b] hover:bg-slate-700/80 border-slate-700/80 border flex items-center gap-2 text-xs text-slate-200 transition shadow-sm max-w-[18rem]"
        title={appState.activeConnection ? `${getDriverName(appState.activeConnection.driver)} • ${appState.activeConnection.host || 'localhost'}` : 'Select connection'}
      >
        <span class="w-2 h-2 rounded-full {appState.isConnecting ? 'bg-amber-400 animate-ping' : appState.isProduction ? 'bg-rose-500' : (appState.activeConnection ? 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]' : 'bg-slate-500')}"></span>
        <span class="font-medium truncate text-slate-100">
          {appState.isConnecting ? 'Connecting...' : (appState.activeConnection?.name || 'No active connection')}
        </span>
        {#if appState.activeConnection?.environment}
          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium border {getEnvBadgeClass(appState.activeConnection.environment)}">
            {appState.activeConnection.environment}
          </span>
        {/if}
        <Icon name="chevron-down" class="w-3 h-3 text-slate-400" />
      </button>

      {#if showConnDropdown}
        <button
          tabindex="-1"
          aria-label="Close"
          class="fixed inset-0 z-40 bg-transparent cursor-default"
          onclick={() => closeConnDropdown()}
        ></button>

        <div
          use:viewportFit
          class="absolute left-0 top-full mt-1 w-80 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in zoom-in-95 duration-100"
          role="dialog"
          aria-label="Switch connection"
        >
          <div class="p-2 border-b border-slate-800">
            <div class="relative flex items-center">
              <input
                id="session-search-input"
                type="text"
                bind:this={connSearchInput}
                bind:value={connSearchQuery}
                onkeydown={handleConnSearchKeydown}
                oninput={() => (connActiveIndex = 0)}
                role="combobox"
                aria-expanded="true"
                aria-controls="session-listbox"
                aria-autocomplete="list"
                placeholder="Search connections... (↑↓ Enter)"
                class="w-full bg-[#050811] border border-slate-700 rounded px-2.5 py-1 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500"
              />
              {#if connSearchQuery}
                <button
                  type="button"
                  id="session-search-clear-btn"
                  onclick={() => (connSearchQuery = '')}
                  class="absolute right-2 text-slate-400 hover:text-slate-200 p-0.5 rounded hover:bg-slate-800 transition"
                  aria-label="Clear connection search"
                >
                  <Icon name="x" class="w-3.5 h-3.5" />
                </button>
              {/if}
            </div>
          </div>

          <div id="session-listbox" bind:this={connMenuList} role="listbox" class="max-h-72 overflow-y-auto py-1">
            {#if appState.connections.length === 0}
              <div class="p-4 text-center space-y-2">
                <p class="text-xs text-slate-500">No saved connections yet.</p>
                <button
                  onclick={() => {
                    closeConnDropdown();
                    appState.openNewConnectionModal();
                  }}
                  class="px-3 py-1 rounded bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold"
                >
                  + Create one
                </button>
              </div>
            {:else if groupedConnections.length === 0}
              <div class="p-3 text-center text-xs text-slate-500">No matching connections</div>
            {:else}
              {#each groupedConnections as group}
                <div class="px-3 py-1 text-[10px] font-bold uppercase tracking-wider bg-slate-900/60 text-slate-400 flex items-center justify-between">
                  <div class="flex items-center space-x-1.5">
                    <span class="w-1.5 h-1.5 rounded-full {group.env === 'Production' ? 'bg-rose-400' : group.env === 'Staging' ? 'bg-amber-400' : 'bg-emerald-400'}"></span>
                    <span>{group.env}</span>
                  </div>
                  <span class="font-mono text-slate-500">{group.items.length}</span>
                </div>

                {#each group.items as conn}
                  {@const connIdx = orderedConnections.findIndex((c) => c.id === conn.id)}
                  {@const isActiveConn = connIdx === connActiveIndex}
                  <div
                    data-conn-active={isActiveConn}
                    role="option"
                    tabindex="-1"
                    aria-selected={conn.id === appState.activeConnection?.id}
                    onmouseenter={() => (connActiveIndex = connIdx)}
                    class="flex items-center px-3 py-1.5 transition-colors group {isActiveConn
                      ? 'bg-slate-800/90 ring-1 ring-inset ring-sky-500/40'
                      : 'hover:bg-slate-800/80'} {conn.id === appState.activeConnection?.id ? 'bg-sky-500/10' : ''}"
                  >
                    <button
                      onclick={() => chooseConnection(conn)}
                      class="flex-1 text-left flex items-center space-x-2.5 truncate"
                    >
                      <Icon name="database" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                      <div class="truncate">
                        <div class="truncate text-xs font-semibold {conn.id === appState.activeConnection?.id ? 'text-sky-400' : 'text-slate-200'}">
                          {conn.name}
                        </div>
                        <div class="text-[10px] text-slate-500 font-mono">
                          {getDriverName(conn.driver)} • {conn.host || 'localhost'}
                        </div>
                      </div>
                    </button>

                    <div class="flex items-center space-x-1 shrink-0 ml-1">
                      <button
                        onclick={(e) => {
                          e.stopPropagation();
                          closeConnDropdown();
                          appState.openEditConnectionModal(conn);
                        }}
                        class="p-1 hover:bg-slate-700 text-slate-400 hover:text-white rounded transition-opacity {conn.id === appState.activeConnection?.id ? 'opacity-70 hover:opacity-100' : 'opacity-0 group-hover:opacity-100 focus:opacity-100'}"
                        title="Edit Connection"
                      >
                        <Icon name="edit" class="w-3 h-3" />
                      </button>
                      <button
                        onclick={(e) => handleDeleteConnection(e, conn)}
                        class="p-1 opacity-0 group-hover:opacity-100 hover:bg-rose-950/70 text-slate-400 hover:text-rose-400 rounded"
                        title="Delete Connection"
                      >
                        <Icon name="trash-2" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              {/each}
            {/if}
          </div>

          <div class="p-1.5">
            <button
              onclick={() => {
                closeConnDropdown();
                appState.openNewConnectionModal();
              }}
              class="w-full py-1.5 px-3 bg-sky-950/60 hover:bg-sky-900/60 border border-sky-800 text-sky-300 font-semibold rounded text-xs flex items-center justify-center space-x-1.5 transition"
            >
              <Icon name="plus" class="w-3.5 h-3.5" />
              <span>Add New Connection</span>
            </button>
          </div>
        </div>
      {/if}
    </div>

    <!-- Read-Only / Write-Allowed Lock Pill -->
    {#if appState.activeConnection}
      <button
        onclick={() => appState.toggleReadOnlyMode()}
        class="h-7 px-2 rounded-md font-medium transition border flex items-center gap-1.5 shrink-0 {appState.isReadOnly
          ? 'bg-slate-800/80 border-slate-700 text-slate-300 hover:bg-slate-700/80 hover:text-slate-100'
          : (appState.isProduction
            ? 'bg-amber-950/50 border-amber-600/60 text-amber-200 hover:bg-amber-900/50'
            : 'bg-emerald-950/40 border-emerald-700/60 text-emerald-300 hover:bg-emerald-900/40')}"
        title={appState.isReadOnly
          ? 'Read-Only mode active — click to allow writes'
          : (appState.isProduction ? 'Write mode active on PRODUCTION — click to lock into read-only mode' : 'Write mode active — click to lock into read-only mode')}
      >
        <Icon
          name={appState.isReadOnly ? 'lock' : 'unlock'}
          class="w-3 h-3 {appState.isReadOnly
            ? 'text-slate-400'
            : (appState.isProduction ? 'text-amber-400' : 'text-emerald-400')}"
        />
        <span class="hidden xl:inline text-[11px] font-semibold">{appState.isReadOnly ? 'Read-Only' : 'Write Allowed'}</span>
      </button>
    {/if}
  </div>

  <!-- Center: Unified Omnibar (Ctrl+K) -->
  <div class="hidden md:flex flex-1 justify-center items-center min-w-0 px-2">
    <button
      onclick={() => (appState.isCommandPaletteOpen = true)}
      class="h-7 w-full max-w-md px-2.5 rounded-md bg-[#0b0f19] border border-slate-700/80 hover:border-slate-600 hover:bg-[#1e293b] text-slate-400 hover:text-slate-200 transition shadow-sm flex items-center justify-between gap-3"
      title="Search tables, #columns, databases and actions (Ctrl+K / Ctrl+P)"
    >
      <span class="flex items-center gap-1.5 text-xs truncate">
        <Icon name="search" class="w-3.5 h-3.5 text-slate-500" />
        <span class="truncate">Search tables, #columns, databases, actions...</span>
      </span>
      <kbd class="shrink-0 text-[9px] font-mono bg-slate-800/80 border border-slate-700 px-1 py-0.5 rounded text-slate-400">Ctrl K</kbd>
    </button>
  </div>

  <!-- Right Actions: tools menus, live DBA indicators, drawers, LAN PIN -->
  <div class="flex items-center gap-2 text-xs shrink-0">
    {#snippet toolMenu(key: 'monitor' | 'dbtools' | 'settings', label: string, icon: string, iconColor: string, items: ToolMenuItem[])}
      <div class="relative">
        <button
          onclick={() => (openToolsMenu = openToolsMenu === key ? null : key)}
          class="{navBtn} relative font-medium {openToolsMenu === key ? 'bg-slate-700 text-white border-sky-600/60 ring-1 ring-sky-500/40' : ''}"
        >
          <Icon name={icon} class="w-3.5 h-3.5 {iconColor}" />
          <span class="hidden lg:inline">{label}</span>
          {#if key === 'monitor'}
            {#if appState.dbaBlockersCount > 0}
              <span id="monitor-alert-dot" class="w-2 h-2 rounded-full bg-rose-500 animate-ping"></span>
            {/if}
            {#if appState.flightRecordingActive}
              <span id="monitor-rec-dot" class="w-2 h-2 rounded-full bg-rose-400 animate-pulse" title="Flight Recorder Active"></span>
            {/if}
            {#if appState.dbaSlowQueriesCount > 0}
              <span id="processlist-slow-badge" class="px-1 rounded text-[9px] font-mono font-bold bg-amber-500/20 text-amber-300 border border-amber-500/40">
                {appState.dbaSlowQueriesCount}
              </span>
            {/if}
            {#if appState.dbaClusterLabel}
              <span
                id="cluster-status-badge"
                class="hidden xl:inline px-1 rounded text-[9px] font-mono font-bold border {appState.dbaClusterStatus === 'critical'
                  ? 'bg-rose-500/20 text-rose-300 border-rose-500/40'
                  : appState.dbaClusterStatus === 'warn'
                    ? 'bg-amber-500/20 text-amber-300 border-amber-500/40'
                    : 'bg-emerald-500/20 text-emerald-300 border-emerald-500/40'}"
              >
                {appState.dbaClusterLabel}
              </span>
            {/if}
          {/if}
          <Icon name="chevron-down" class="hidden lg:inline w-3 h-3 text-slate-400 transition-transform {openToolsMenu === key ? 'rotate-180' : ''}" />
        </button>

        {#if openToolsMenu === key}
          <button
            tabindex="-1"
            aria-label="Close"
            class="fixed inset-0 z-40 bg-transparent cursor-default"
            onclick={() => (openToolsMenu = null)}
          ></button>
          <div
            use:viewportFit
            class="absolute right-0 mt-1.5 w-72 max-w-[calc(100vw-2rem)] bg-[#0b0f19] border border-slate-700 rounded-lg shadow-lg z-50 overflow-hidden py-1 text-xs"
            role="menu"
            aria-label={label}
          >
            {#each items as item (item.label)}
              <button
                type="button"
                role="menuitem"
                onclick={() => {
                  item.action();
                  openToolsMenu = null;
                }}
                class="w-full text-left px-3 py-1.5 hover:bg-slate-800/70 flex flex-col gap-0.5"
              >
                <span class="flex items-center justify-between gap-2 w-full">
                  <span class="flex items-center space-x-1.5 font-medium {item.iconColor}">
                    <Icon name={item.icon} class="w-3.5 h-3.5" />
                    <span>{item.label}</span>
                  </span>
                  {#if item.badge !== undefined}
                    <span class="px-1.5 py-0.2 rounded text-[9px] font-bold border {item.badgeClass || 'bg-slate-800 text-slate-300 border-slate-700'}">
                      {item.badge}
                    </span>
                  {/if}
                </span>
                <span class="text-[10px] text-slate-400 pl-5">{item.desc}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/snippet}

    <div class="flex items-center gap-1">
      {@render toolMenu('monitor', 'Monitor', 'activity', 'text-emerald-400', monitorItems)}
      {@render toolMenu('dbtools', 'DB Tools', 'wrench', 'text-sky-400', dbToolItems)}
      {@render toolMenu('settings', 'Settings', 'settings', 'text-cyan-400', settingsItems)}
    </div>

    {#if appState.flightRecordingActive}
      <button
        id="flight-recorder-timer-badge"
        onclick={() => appState.openProcesslistTab('flight-recorder')}
        class="h-7 px-2 rounded-md bg-rose-950/70 border border-rose-600/60 text-rose-300 text-[10px] font-mono font-bold flex items-center gap-1.5 animate-pulse"
        title="Flight Recorder active — click to open timeline"
      >
        <span id="flight-recorder-rec-dot" class="w-2 h-2 rounded-full bg-rose-500"></span>
        <span>REC {appState.flightRecordingRemainingSec > 0 ? `${appState.flightRecordingRemainingSec}s` : ''}</span>
      </button>
    {/if}

    <div class={navDivider}></div>

    <!-- Drawers: operational notes, AI copilot -->
    <div class="flex items-center gap-1">
      <button
        id="notes-inbox-button"
        onclick={() => appState.toggleNotesDrawer()}
        class="{navBtn} relative {appState.isNotesDrawerOpen ? 'bg-amber-950/40 border-amber-600 text-amber-300' : ''}"
        title="Operational Notes"
      >
        <Icon name="file-text" class="w-3.5 h-3.5 text-amber-400" />
        <span class="hidden xl:inline">Notes</span>
        {#if appState.dueNotesCount > 0}
          <span
            id="notes-inbox-badge"
            class="px-1.5 py-0.2 rounded-full bg-rose-500 text-white font-bold text-[9px] leading-tight animate-pulse"
          >
            {appState.dueNotesCount}
          </span>
        {/if}
      </button>

      <button
        onclick={() => appState.toggleAiDrawer()}
        class="{navBtn} {appState.isAiDrawerOpen ? 'bg-sky-950/40 border-sky-500 text-sky-300' : ''}"
        title="AI Copilot (Ctrl+L)"
      >
        <Icon name="bot" class="w-3.5 h-3.5 text-sky-400" />
        <span class="hidden xl:inline">AI Copilot</span>
      </button>
    </div>

    <!-- LAN access PIN (shown only when locally permitted) -->
    {#if isLanAvailable}
      <div class={navDivider}></div>
      <div class="h-7 bg-slate-900 rounded-md border border-slate-700/80 font-mono inline-flex items-center overflow-hidden select-none shadow-sm">
        <button
          onclick={copyMasterPin}
          class="h-full px-2 hover:bg-slate-800 text-sky-400 hover:text-sky-300 transition flex items-center gap-1.5"
          title="Copy Master PIN for LAN access"
        >
          <Icon name="key" class="w-3 h-3 text-sky-400" />
          <span class="text-[10px] text-slate-400">PIN</span>
          <span class="font-bold text-white tracking-widest text-[11px]">
            {showMasterPin ? masterPin : '••••••••'}
          </span>
          {#if copiedPin}
            <span class="text-[9px] text-emerald-400 font-sans font-semibold">Copied</span>
          {/if}
        </button>
        <button
          onclick={toggleMasterPin}
          class="h-full px-1.5 text-slate-400 hover:text-white hover:bg-slate-800 border-l border-slate-700/80 transition flex items-center justify-center"
          title="Toggle PIN Visibility (auto-hides in 10s)"
        >
          <Icon name={showMasterPin ? 'eye-off' : 'eye'} class="w-3 h-3" />
        </button>
      </div>
    {/if}
  </div>
</header>
