<script lang="ts">
import { copyText } from '../../utils/clipboard';
  import { tick } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

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
    action: () => void;
  }

  let openToolsMenu = $state<'monitor' | 'dbtools' | 'settings' | null>(null);

  // One neutral style for every header action so heights, padding, borders and hover stay aligned.
  const navBtn =
    'h-7 px-2 rounded-md bg-[#1e293b] border border-slate-700/80 text-slate-300 hover:bg-slate-700 hover:text-white hover:border-slate-600 transition shadow-sm flex items-center gap-1.5';
  const navIconBtn = navBtn + ' w-7 justify-center px-0';
  const navDivider = 'w-px h-4 bg-slate-800 shrink-0';

  const monitorItems: ToolMenuItem[] = [
    { label: 'Processlist', desc: 'Live threads, slow queries and kill', icon: 'cpu', iconColor: 'text-sky-400', action: () => appState.openProcesslistTab('threads') },
    { label: 'Deadlock Inspector', desc: 'Blocking chains and deadlock victims', icon: 'shield-alert', iconColor: 'text-rose-400', action: () => appState.openProcesslistTab('deadlock') },
    { label: 'Cluster & Replication', desc: 'Replication lag and node state', icon: 'network', iconColor: 'text-emerald-400', action: () => appState.openProcesslistTab('cluster') },
    { label: 'Server Variables', desc: 'Runtime parameters and tuning hints', icon: 'sliders', iconColor: 'text-cyan-400', action: () => appState.openProcesslistTab('variables') },
    { label: 'Flight Recorder', desc: 'Record performance telemetry over time', icon: 'disc', iconColor: 'text-rose-400', action: () => appState.openProcesslistTab('flight-recorder') },
    { label: 'Storage & Health', desc: 'Table footprints and health advisor', icon: 'hard-drive', iconColor: 'text-amber-400', action: () => appState.openDbDiagnosticsModal() },
    { label: 'Audit Log', desc: 'Session query history and execution log', icon: 'clipboard-list', iconColor: 'text-purple-400', action: () => (appState.isAuditDrawerOpen = true) },
  ];

  const dbToolItems: ToolMenuItem[] = [
    { label: 'Dump Entire Database', desc: 'Export all tables DDL + INSERTs (.sql)', icon: 'download', iconColor: 'text-sky-400', action: () => appState.openDumpModal() },
    { label: 'Run Large SQL Script', desc: 'Stream and execute multi-megabyte .sql', icon: 'file-code', iconColor: 'text-indigo-400', action: () => appState.openRunSqlModal() },
    { label: 'Structure Sync (Schema Diff)', desc: 'Compare schemas and generate migration DDL', icon: 'git-diff', iconColor: 'text-emerald-400', action: () => appState.openStructureSyncModal() },
    { label: 'Cross-Connection Transfer', desc: 'Stream copy tables between sessions', icon: 'arrow-left-right', iconColor: 'text-cyan-400', action: () => (appState.isDataTransferOpen = true) },
    { label: 'Row-Level Data Sync', desc: 'Keyset delta reconciler (insert/update/delete)', icon: 'refresh-cw', iconColor: 'text-purple-400', action: () => (appState.isDataSyncOpen = true) },
    { label: 'Visual ERD Canvas', desc: 'Interactive ER diagram with FK relations', icon: 'layers', iconColor: 'text-indigo-400', action: () => appState.openErdTab() },
    { label: 'Users & Privileges', desc: 'MySQL accounts and permission matrix', icon: 'users', iconColor: 'text-rose-400', action: () => appState.openPrivilegeMatrixModal() },
  ];

  const settingsItems: ToolMenuItem[] = [
    { label: 'Settings', desc: 'Connections, sync, notes and AI', icon: 'settings', iconColor: 'text-cyan-400', action: () => (appState.isSettingsModalOpen = true) },
    { label: 'AI Model Config', desc: 'Provider credentials and parameters', icon: 'bot', iconColor: 'text-indigo-400', action: () => (appState.isAiSettingsModalOpen = true) },
    { label: 'Keyboard Shortcuts', desc: 'Hotkeys for navigation and execution', icon: 'keyboard', iconColor: 'text-amber-400', action: () => (appState.isShortcutsModalOpen = true) },
    { label: 'Check for Updates', desc: 'Latest release notes and self-update', icon: 'arrow-down-circle', iconColor: 'text-emerald-400', action: () => (appState.isUpdateModalOpen = true) },
    { label: 'Open in Web Browser', desc: 'Launch this UI in your default browser', icon: 'share-2', iconColor: 'text-sky-400', action: () => appState.openInSystemBrowser() },
    { label: 'Copy App URL', desc: 'Share this SqlFox address', icon: 'copy', iconColor: 'text-slate-300', action: () => appState.copyAppUrl() },
    { label: 'Shut Down Daemon', desc: 'Gracefully stop the local server', icon: 'rotate-ccw', iconColor: 'text-rose-400', action: () => appState.shutdownDaemon() },
  ];

  let masterPin = $derived(appState.authStatus?.master_password || 'fox-0304');
  let isLanAvailable = $derived(
    Boolean(appState.authStatus?.lan_enabled || appState.authStatus?.is_remote || appState.authStatus?.master_password)
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

  // Flatten in display order so arrow keys follow what the user sees.
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

  function chooseConnection(conn: (typeof appState.connections)[number]) {
    appState.selectConnection(conn);
    closeConnDropdown();
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

  async function handleToggleReadOnly() {
    if (!appState.activeConnection) return;
    if (appState.activeConnection.environment === 'Production' && appState.isReadOnly) {
      const ok = confirm('Switch to Read-Write mode for this Production session? Data modification operations (INSERT/UPDATE/DELETE/DDL) will be permitted.');
      if (!ok) return;
    }
    try {
      const res = await api.toggleReadOnly(appState.activeConnection.id);
      appState.isReadOnly = res.read_only;
      appState.showToast(
        res.read_only ? 'Read-only mode enabled: modifications blocked' : 'Write mode active: modifications allowed',
        'info'
      );
    } catch (err: any) {
      appState.showToast(`Failed to toggle read-only: ${err.message}`, 'error');
    }
  }

  function getEnvBadgeClass(env: string) {
    switch (env) {
      case 'Production':
        return 'bg-rose-500/20 text-rose-300 border-rose-500/40 animate-pulse';
      case 'Staging':
        return 'bg-amber-500/20 text-amber-300 border-amber-500/40';
      default:
        return 'bg-emerald-500/20 text-emerald-300 border-emerald-500/40';
    }
  }

  function getDriverName(driver?: string) {
    switch (driver) {
      case 'postgres':
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

  function copyMasterPin() {
    copyText(masterPin);
    copiedPin = true;
    appState.showToast('Master PIN copied to clipboard!', 'info');
    setTimeout(() => (copiedPin = false), 2000);
  }
</script>

<header class="h-12 bg-[#0f172a] border-b border-slate-800 flex items-center gap-3 px-4 z-40 flex-shrink-0 select-none">
  <!-- Brand, Sidebar Toggle & Active Connection -->
  <div class="flex items-center gap-2 shrink-0 min-w-0">
    <button
      onclick={() => (appState.sidebarCollapsed = !appState.sidebarCollapsed)}
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

    <!-- Active Connection Selector Pill -->
    <div class="relative">
      <button
        id="session-switcher-btn"
        onclick={() => (showConnDropdown ? closeConnDropdown() : openConnDropdown())}
        aria-haspopup="listbox"
        aria-expanded={showConnDropdown}
        class="h-7 px-2 rounded-md {appState.isProduction ? 'bg-rose-950/40 border-rose-600/60 hover:bg-rose-900/40 shadow-rose-950/30' : 'bg-[#1e293b] hover:bg-slate-700/80 border-slate-700/80'} border flex items-center gap-2 text-xs text-slate-200 transition shadow-sm max-w-[18rem]"
      >
        <span class="w-2 h-2 rounded-full {appState.isProduction ? 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.8)] animate-pulse' : (appState.activeConnection ? 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]' : 'bg-slate-500')}"></span>
        <span class="font-medium truncate {appState.isProduction ? 'text-rose-100 font-semibold' : 'text-slate-100'}">{appState.activeConnection?.name || 'No active connection'}</span>
        {#if appState.activeConnection?.environment}
          <span class="px-1.5 py-0.5 rounded text-[10px] font-semibold border {getEnvBadgeClass(appState.activeConnection.environment)}">
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
          class="absolute left-0 mt-1 w-72 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in zoom-in-95 duration-100"
          role="dialog"
          aria-label="Switch connection"
        >
          <!-- Search filter for connections -->
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
            {#if groupedConnections.length === 0}
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

                    <div class="flex items-center space-x-1.5 shrink-0 ml-1">
                      <button
                        onclick={(e) => {
                          e.stopPropagation();
                          closeConnDropdown();
                          appState.openEditConnectionModal(conn);
                        }}
                        class="p-1 opacity-0 group-hover:opacity-100 hover:bg-slate-700 text-slate-400 hover:text-white rounded"
                        title="Edit Connection"
                      >
                        <Icon name="edit" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              {/each}
            {/if}
          </div>

          <!-- Bottom Action: Add Connection -->
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
  </div>

  <!-- Center: command palette trigger, styled as a search field -->
  <div class="hidden md:flex flex-1 justify-center min-w-0 px-2">
    <button
      onclick={() => (appState.isCommandPaletteOpen = true)}
      class="h-7 w-full max-w-md px-2.5 rounded-md bg-[#0b0f19] border border-slate-700/80 hover:border-slate-600 hover:bg-[#1e293b] text-slate-400 hover:text-slate-200 transition shadow-sm flex items-center justify-between gap-3"
      title="Search tables, databases and actions"
    >
      <span class="flex items-center gap-1.5 text-xs truncate">
        <Icon name="search" class="w-3.5 h-3.5 text-slate-500" />
        <span class="truncate">Search tables, databases, actions...</span>
      </span>
      <kbd class="shrink-0 text-[9px] font-mono bg-slate-800/80 border border-slate-700 px-1 py-0.5 rounded text-slate-400">Ctrl K</kbd>
    </button>
  </div>

  <!-- Right Actions: tools menus, session mode, drawers, LAN PIN, refresh -->
  <div class="flex items-center gap-2 text-xs shrink-0">
    <!-- Monitor / DB Tools / Settings menus (legacy layout) -->
    {#snippet toolMenu(key: 'monitor' | 'dbtools' | 'settings', label: string, icon: string, iconColor: string, items: ToolMenuItem[])}
      <div class="relative">
        <button
          onclick={() => (openToolsMenu = openToolsMenu === key ? null : key)}
          class="{navBtn} font-medium {openToolsMenu === key ? 'bg-slate-700 text-white border-sky-600/60 ring-1 ring-sky-500/40' : ''}"
        >
          <Icon name={icon} class="w-3.5 h-3.5 {iconColor}" />
          <span class="hidden lg:inline">{label}</span>
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
            class="absolute right-0 mt-1.5 w-64 max-w-[calc(100vw-2rem)] bg-[#0b0f19] border border-slate-700 rounded-lg shadow-lg z-50 overflow-hidden py-1 text-xs"
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
                <span class="flex items-center space-x-1.5 font-medium {item.iconColor}">
                  <Icon name={item.icon} class="w-3.5 h-3.5" />
                  <span>{item.label}</span>
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

    <div class={navDivider}></div>

    <!-- Read-Only / Write-Allowed Lock Pill -->
    {#if appState.activeConnection}
      <button
        onclick={handleToggleReadOnly}
        class="h-7 px-2 rounded-md font-medium transition border flex items-center gap-1.5 {appState.isReadOnly
          ? 'bg-rose-950/60 border-rose-700 text-rose-300 hover:bg-rose-900/60'
          : 'bg-emerald-950/60 border-emerald-700 text-emerald-300 hover:bg-emerald-900/60'}"
        title={appState.isReadOnly ? 'Click to enable write permissions' : 'Click to lock into read-only mode'}
      >
        <Icon name={appState.isReadOnly ? 'lock' : 'unlock'} class="w-3 h-3 {appState.isReadOnly ? 'text-rose-400' : 'text-emerald-400'}" />
        <span class="hidden xl:inline text-[11px] font-semibold">{appState.isReadOnly ? 'Read-Only' : 'Write Allowed'}</span>
      </button>
    {/if}

    <div class={navDivider}></div>

    <!-- Drawers: operational notes, AI copilot -->
    <div class="flex items-center gap-1">
      <button
        id="notes-inbox-button"
        onclick={() => (appState.isNotesDrawerOpen = !appState.isNotesDrawerOpen)}
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
        onclick={() => (appState.isAiDrawerOpen = !appState.isAiDrawerOpen)}
        class="{navBtn} {appState.isAiDrawerOpen ? 'bg-sky-950/40 border-sky-500 text-sky-300' : ''}"
        title="AI Copilot"
      >
        <Icon name="bot" class="w-3.5 h-3.5 text-sky-400" />
        <span class="hidden xl:inline">AI Copilot</span>
      </button>
    </div>

    <!-- LAN access PIN -->
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
