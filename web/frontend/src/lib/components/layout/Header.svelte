<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let showConnDropdown = $state(false);
  let showHeaderDbDropdown = $state(false);
  let showToolsDropdown = $state(false);
  let showMasterPin = $state(false);
  let copiedPin = $state(false);
  let connSearchQuery = $state('');
  let pinAutoMaskTimer: any = null;

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
    navigator.clipboard.writeText(masterPin);
    copiedPin = true;
    appState.showToast('Master PIN copied to clipboard!', 'info');
    setTimeout(() => (copiedPin = false), 2000);
  }
</script>

<header class="h-12 bg-[#0f172a] border-b border-[#334155] flex items-center justify-between px-4 z-40 flex-shrink-0 gap-3 select-none">
  <!-- Brand, Sidebar Toggle & Active Connection -->
  <div class="flex items-center space-x-3 shrink-0">
    <button
      onclick={() => (appState.sidebarCollapsed = !appState.sidebarCollapsed)}
      class="h-7 w-7 flex items-center justify-center rounded bg-[#1e293b] hover:bg-slate-700 text-slate-400 hover:text-white border border-[#334155] transition text-xs cursor-pointer shadow-sm"
      title="Toggle Sidebar (Ctrl+B)"
    >
      <Icon name="sidebar" class="w-3.5 h-3.5" />
    </button>

    <!-- SqlFox Logo -->
    <div class="flex items-center space-x-2">
      <Icon name="brand" class="w-6 h-6" />
      <span class="font-bold tracking-wide text-white text-base">Sql<span class="text-sky-400">Fox</span></span>
    </div>

    <div class="h-4 w-px bg-[#334155] mx-1"></div>

    <!-- Active Connection Selector Pill -->
    <div class="relative">
      <button
        onclick={() => (showConnDropdown = !showConnDropdown)}
        class="h-7 px-2.5 rounded {appState.isProduction ? 'bg-rose-950/40 border-rose-600/60 hover:bg-rose-900/40 shadow-rose-950/30' : 'bg-[#1e293b] hover:bg-slate-700/80 border-[#334155]'} border flex items-center space-x-2 text-xs text-slate-200 transition shadow-sm"
      >
        <span class="w-2 h-2 rounded-full {appState.isProduction ? 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.8)] animate-pulse' : (appState.activeConnection ? 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]' : 'bg-slate-500')}"></span>
        <span class="font-medium {appState.isProduction ? 'text-rose-100 font-semibold' : 'text-slate-100'}">{appState.activeConnection?.name || 'No active connection'}</span>
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
          onclick={() => (showConnDropdown = false)}
        ></button>

        <div class="absolute left-0 mt-1 w-72 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in zoom-in-95 duration-100">
          <!-- Search filter for connections -->
          <div class="p-2 border-b border-slate-800">
            <div class="relative flex items-center">
              <input
                id="session-search-input"
                type="text"
                bind:value={connSearchQuery}
                placeholder="Search connections..."
                class="w-full bg-[#050811] border border-slate-700 rounded px-2.5 py-1 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500"
              />
              {#if connSearchQuery}
                <button
                  type="button"
                  id="session-search-clear-btn"
                  onclick={() => (connSearchQuery = '')}
                  class="absolute right-2 text-slate-400 hover:text-slate-200 text-xs"
                >
                  ✕
                </button>
              {/if}
            </div>
          </div>

          <div class="max-h-72 overflow-y-auto py-1">
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
                  <div class="flex items-center hover:bg-slate-800/80 px-3 py-1.5 transition-colors group {conn.id === appState.activeConnection?.id ? 'bg-sky-500/10' : ''}">
                    <button
                      onclick={() => {
                        appState.selectConnection(conn);
                        showConnDropdown = false;
                      }}
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
                          showConnDropdown = false;
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
                showConnDropdown = false;
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

    <!-- Header Database Selector -->
    {#if appState.currentDatabase}
      <span class="text-xs text-slate-600">/</span>
      <div class="relative">
        <button
          onclick={() => (showHeaderDbDropdown = !showHeaderDbDropdown)}
          class="h-7 px-2 rounded bg-sky-950/40 hover:bg-sky-900/50 border border-sky-800/40 text-xs font-mono text-sky-300 font-semibold flex items-center space-x-1.5 transition shadow-sm"
          title="Switch database"
        >
          <Icon name="database" class="w-3 h-3 text-sky-400" />
          <span>{appState.currentDatabase}</span>
          <Icon name="chevron-down" class="w-3 h-3 text-sky-400/70" />
        </button>

        {#if showHeaderDbDropdown}
          <button
            tabindex="-1"
            aria-label="Close"
            class="fixed inset-0 z-40 bg-transparent cursor-default"
            onclick={() => (showHeaderDbDropdown = false)}
          ></button>

          <div class="absolute left-0 mt-1 w-60 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in duration-100">
            <div class="px-3 py-1.5 text-[10px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between">
              <span>Switch Database</span>
              <span class="font-mono text-slate-500">{appState.availableDatabases.length} DBs</span>
            </div>
            <div class="max-h-64 overflow-y-auto py-1">
              {#each appState.availableDatabases as db}
                <button
                  onclick={() => {
                    appState.selectDatabase(db);
                    showHeaderDbDropdown = false;
                  }}
                  class="w-full text-left px-3 py-1.5 text-xs font-mono hover:bg-slate-800 flex items-center justify-between transition-colors {db === appState.currentDatabase ? 'text-sky-400 font-bold bg-sky-500/10' : 'text-slate-300'}"
                >
                  <span class="truncate">{db}</span>
                  {#if db === appState.currentDatabase}
                    <Icon name="check" class="w-3.5 h-3.5 text-sky-400 shrink-0 ml-1" />
                  {/if}
                </button>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Center Quick Search Buttons: Command Palette -->
  <div class="hidden md:flex items-center space-x-2 shrink-0">
    <button
      onclick={() => (appState.isCommandPaletteOpen = true)}
      class="h-7 bg-[#1e293b] hover:bg-slate-700 text-slate-300 px-2.5 rounded text-xs flex items-center space-x-1.5 border border-[#334155] transition shadow-sm"
    >
      <Icon name="terminal" class="w-3.5 h-3.5 text-sky-400" />
      <span>Command Palette</span>
      <kbd class="ml-1 text-[9px] bg-slate-900 px-1 py-0.5 rounded text-slate-400 border border-slate-700 font-mono">Ctrl+K</kbd>
    </button>
  </div>

  <!-- Right Actions: DBA Tools Menu, Master PIN, Production Indicator, Refresh -->
  <div class="flex items-center space-x-2 text-xs shrink-0">
    <!-- DBA Tools Dropdown -->
    <div class="relative">
      <button
        onclick={() => (showToolsDropdown = !showToolsDropdown)}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-[#334155] flex items-center space-x-1.5 transition shadow-sm"
      >
        <Icon name="sliders" class="w-3.5 h-3.5 text-sky-400" />
        <span>Tools</span>
        <Icon name="chevron-down" class="w-3 h-3 text-slate-400" />
      </button>

      {#if showToolsDropdown}
        <button
          tabindex="-1"
          aria-label="Close"
          class="fixed inset-0 z-40 bg-transparent cursor-default"
          onclick={() => (showToolsDropdown = false)}
        ></button>

        <div class="absolute right-0 mt-1 w-52 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in duration-100 text-xs">
          <div class="py-1">
            <button
              onclick={() => {
                appState.openProcesslistTab();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="cpu" class="w-3.5 h-3.5 text-sky-400" />
              <span>Processlist & Threads</span>
            </button>
            <button
              onclick={() => {
                appState.openErdTab();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="layers" class="w-3.5 h-3.5 text-indigo-400" />
              <span>ERD Schema Canvas</span>
            </button>
            <button
              onclick={() => {
                appState.openNewConnectionModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="database" class="w-3.5 h-3.5 text-emerald-400" />
              <span>Connection Manager</span>
            </button>
          </div>

          <div class="py-1">
            <button
              onclick={() => {
                appState.openDumpModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="download" class="w-3.5 h-3.5 text-sky-400" />
              <span>Export SQL Dump</span>
            </button>
            <button
              onclick={() => {
                appState.openStructureSyncModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="refresh-cw" class="w-3.5 h-3.5 text-emerald-400" />
              <span>Structure Synchronization</span>
            </button>
            <button
              onclick={() => {
                appState.isDataTransferOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <span class="text-sky-400 text-xs">🚀</span>
              <span>Data Transfer (Cross-Conn)</span>
            </button>
            <button
              onclick={() => {
                appState.isDataSyncOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <span class="text-emerald-400 text-xs">⚖️</span>
              <span>Row-Level Data Sync</span>
            </button>
            <button
              onclick={() => {
                appState.isRefreshAllOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <span class="text-amber-400 text-xs">🔄</span>
              <span>Refresh All Catalogs</span>
            </button>
            <button
              onclick={() => {
                appState.isAuditDrawerOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <span class="text-sky-400 text-xs">📜</span>
              <span>Session Audit Log</span>
            </button>
            <button
              onclick={() => {
                appState.openRunSqlModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="file-text" class="w-3.5 h-3.5 text-amber-400" />
              <span>Run Large SQL Script</span>
            </button>
            <button
              onclick={() => {
                appState.openSnippetsModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="bookmark" class="w-3.5 h-3.5 text-sky-400" />
              <span>Query Snippets & Templates</span>
            </button>
            <button
              onclick={() => {
                appState.openCsvImportModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="upload" class="w-3.5 h-3.5 text-emerald-400" />
              <span>Import CSV Records</span>
            </button>
            <button
              onclick={() => {
                appState.openDbDiagnosticsModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="activity" class="w-3.5 h-3.5 text-rose-400" />
              <span>Storage & Diagnostics</span>
            </button>
            <button
              onclick={() => {
                appState.openPrivilegeMatrixModal();
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="shield" class="w-3.5 h-3.5 text-purple-400" />
              <span>User Privilege Matrix</span>
            </button>
            <button
              onclick={() => {
                appState.isMockDataModalOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="database" class="w-3.5 h-3.5 text-emerald-400" />
              <span>Smart Mock Data Generator</span>
            </button>
            <button
              onclick={() => {
                appState.isUpdateModalOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="zap" class="w-3.5 h-3.5 text-amber-400" />
              <span>Self-Update & Releases</span>
            </button>
            <button
              onclick={() => {
                appState.isActivityDrawerOpen = !appState.isActivityDrawerOpen;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="terminal" class="w-3.5 h-3.5 text-sky-400" />
              <span>Live Activity Console</span>
            </button>
          </div>

          <div class="py-1">
            <button
              onclick={() => {
                appState.isSettingsModalOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="sliders" class="w-3.5 h-3.5 text-sky-400" />
              <span>Global Settings & Sync</span>
            </button>
            <button
              onclick={() => {
                appState.isShortcutsModalOpen = true;
                showToolsDropdown = false;
              }}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="keyboard" class="w-3.5 h-3.5 text-amber-400" />
              <span>Keyboard Shortcuts (?)</span>
            </button>
          </div>
        </div>
      {/if}
    </div>

    <!-- Read-Only / Write-Allowed Lock Pill -->
    {#if appState.activeConnection}
      <button
        onclick={handleToggleReadOnly}
        class="h-7 px-2.5 rounded font-medium transition border flex items-center space-x-1.5 {appState.isReadOnly
          ? 'bg-rose-950/60 border-rose-700 text-rose-300 hover:bg-rose-900/60'
          : 'bg-emerald-950/60 border-emerald-700 text-emerald-300 hover:bg-emerald-900/60'}"
        title={appState.isReadOnly ? 'Click to enable write permissions' : 'Click to lock into read-only mode'}
      >
        <Icon name={appState.isReadOnly ? 'lock' : 'unlock'} class="w-3 h-3 {appState.isReadOnly ? 'text-rose-400' : 'text-emerald-400'}" />
        <span class="text-[11px] font-semibold">{appState.isReadOnly ? 'Read-Only' : 'Write Allowed'}</span>
      </button>
    {/if}

    <!-- Operational Notes Drawer Button -->
    <button
      id="notes-inbox-button"
      onclick={() => (appState.isNotesDrawerOpen = !appState.isNotesDrawerOpen)}
      class="h-7 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-[#334155] transition flex items-center space-x-1 shadow-sm relative {appState.isNotesDrawerOpen ? 'bg-amber-950/40 border-amber-600 text-amber-300' : ''}"
      title="Operational Notes"
    >
      <Icon name="file-text" class="w-3.5 h-3.5 text-amber-400" />
      <span class="hidden xl:inline text-xs">Notes</span>
      {#if appState.dueNotesCount > 0}
        <span
          id="notes-inbox-badge"
          class="ml-1 px-1.5 py-0.2 rounded-full bg-rose-500 text-white font-bold text-[9px] leading-tight animate-pulse"
        >
          {appState.dueNotesCount}
        </span>
      {/if}
    </button>

    <!-- AI Copilot Drawer Button -->
    <button
      onclick={() => (appState.isAiDrawerOpen = !appState.isAiDrawerOpen)}
      class="h-7 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-[#334155] transition flex items-center space-x-1 shadow-sm {appState.isAiDrawerOpen ? 'bg-sky-950/40 border-sky-500 text-sky-300' : ''}"
      title="AI Copilot"
    >
      <Icon name="bot" class="w-3.5 h-3.5 text-sky-400" />
      <span class="hidden xl:inline text-xs">AI Copilot</span>
    </button>

    <!-- Master PIN Pill with Copy & Reveal -->
    {#if isLanAvailable}
      <div class="h-7 bg-slate-900 text-sky-400 rounded border border-sky-900/60 font-mono transition inline-flex items-center text-xs overflow-hidden select-none shadow-sm">
        <button
          onclick={copyMasterPin}
          class="h-full px-2 hover:bg-slate-800 text-sky-400 hover:text-sky-300 transition flex items-center space-x-1.5"
          title="Copy Master PIN for LAN access"
        >
          <Icon name="key" class="w-3 h-3 text-sky-400" />
          <span class="text-[10px] text-slate-400">PIN:</span>
          <span class="font-bold text-white tracking-widest text-[11px]">
            {showMasterPin ? masterPin : '••••••••'}
          </span>
          {#if copiedPin}
            <span class="text-[9px] text-emerald-400 font-sans font-semibold">Copied!</span>
          {/if}
        </button>
        <button
          onclick={toggleMasterPin}
          class="h-full px-1.5 text-slate-400 hover:text-white hover:bg-slate-800 border-l border-sky-900/60 transition flex items-center justify-center"
          title="Toggle PIN Visibility (auto-hides in 10s)"
        >
          <Icon name={showMasterPin ? 'eye-off' : 'eye'} class="w-3 h-3" />
        </button>
      </div>
    {/if}

    <button
      onclick={() => appState.refreshCatalog()}
      class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-[#334155] flex items-center space-x-1.5 transition shadow-sm"
      title="Refresh Schema Catalog"
    >
      <Icon name="refresh-cw" class="w-3.5 h-3.5 {appState.isLoadingCatalog ? 'animate-spin text-sky-400' : 'text-slate-400'}" />
      <span class="hidden sm:inline">Refresh</span>
    </button>
  </div>
</header>
