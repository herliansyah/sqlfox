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
  let toolsSearchQuery = $state('');
  let pinAutoMaskTimer: any = null;

  type ToolCategoryKey = 'all' | 'dba' | 'data' | 'schema' | 'security' | 'preferences';
  let activeToolsCategory = $state<ToolCategoryKey>('all');
  let selectedToolIndex = $state(0);

  interface ToolItem {
    id: string;
    category: 'dba' | 'data' | 'schema' | 'security' | 'preferences';
    categoryLabel: string;
    name: string;
    desc: string;
    icon: string;
    iconColor: string;
    shortcut?: string;
    badge?: string;
    action: () => void;
  }

  const toolCategories: { id: ToolCategoryKey; label: string; dotColor: string }[] = [
    { id: 'all', label: 'All Tools', dotColor: 'bg-slate-400' },
    { id: 'dba', label: 'DBA & Monitoring', dotColor: 'bg-sky-400' },
    { id: 'data', label: 'Data Movement & ETL', dotColor: 'bg-emerald-400' },
    { id: 'schema', label: 'Schema & Modeling', dotColor: 'bg-indigo-400' },
    { id: 'security', label: 'Security & Access', dotColor: 'bg-purple-400' },
    { id: 'preferences', label: 'System & Preferences', dotColor: 'bg-slate-400' }
  ];

  const allTools: ToolItem[] = [
    // DBA & Monitoring
    {
      id: 'processlist',
      category: 'dba',
      categoryLabel: 'DBA & Monitoring',
      name: 'Active Sessions & Processlist',
      desc: 'Inspect real-time server connections, running queries & kill thread',
      icon: 'cpu',
      iconColor: 'text-sky-400',
      badge: 'Live',
      action: () => appState.openProcesslistTab()
    },
    {
      id: 'diagnostics',
      category: 'dba',
      categoryLabel: 'DBA & Monitoring',
      name: 'Database Telemetry & Diagnostics',
      desc: 'Tablespace usage, buffer cache hit-ratio & engine metrics',
      icon: 'activity',
      iconColor: 'text-rose-400',
      action: () => appState.openDbDiagnosticsModal()
    },
    {
      id: 'activity-console',
      category: 'dba',
      categoryLabel: 'DBA & Monitoring',
      name: 'Live System Activity Console',
      desc: 'Real-time daemon log streams, background workers & locks',
      icon: 'terminal',
      iconColor: 'text-emerald-400',
      action: () => (appState.isActivityDrawerOpen = !appState.isActivityDrawerOpen)
    },
    {
      id: 'audit-log',
      category: 'dba',
      categoryLabel: 'DBA & Monitoring',
      name: 'Audit Trail & Forensic Log',
      desc: 'Execution forensics, security history & statement audit trail',
      icon: 'clipboard-list',
      iconColor: 'text-amber-400',
      action: () => (appState.isAuditDrawerOpen = true)
    },

    // Data Movement & ETL
    {
      id: 'csv-import',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Import CSV / Delimited Files',
      desc: 'High-speed streaming tabular CSV/TSV table loader',
      icon: 'upload',
      iconColor: 'text-emerald-400',
      action: () => appState.openCsvImportModal()
    },
    {
      id: 'sql-dump',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Export Database Dump (SQL)',
      desc: 'Full schema DDL structure and row data backup dump',
      icon: 'download',
      iconColor: 'text-sky-400',
      action: () => appState.openDumpModal()
    },
    {
      id: 'data-sync',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Row-Level Data Sync & Compare',
      desc: 'Differential row comparison and reconciliation via hashes',
      icon: 'scale',
      iconColor: 'text-teal-400',
      action: () => (appState.isDataSyncOpen = true)
    },
    {
      id: 'data-transfer',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Cross-Host Transfer Pipeline',
      desc: 'Direct pipe streaming tables between database instances',
      icon: 'rocket',
      iconColor: 'text-indigo-400',
      action: () => (appState.isDataTransferOpen = true)
    },
    {
      id: 'mock-generator',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Synthetic Mock Data Generator',
      desc: 'Synthesize realistic schema-aware test fixtures & rows',
      icon: 'database',
      iconColor: 'text-purple-400',
      action: () => (appState.isMockDataModalOpen = true)
    },

    // Schema & Modeling
    {
      id: 'erd',
      category: 'schema',
      categoryLabel: 'Schema & Modeling',
      name: 'Entity-Relationship (ERD) Canvas',
      desc: 'Interactive schema visualizer, relationship graph & SVG export',
      icon: 'layers',
      iconColor: 'text-indigo-400',
      action: () => appState.openErdTab()
    },
    {
      id: 'structure-sync',
      category: 'schema',
      categoryLabel: 'Schema & Modeling',
      name: 'Schema Structure Comparison',
      desc: 'Diff schemas across environments and produce alter DDL scripts',
      icon: 'refresh-cw',
      iconColor: 'text-cyan-400',
      action: () => appState.openStructureSyncModal()
    },
    {
      id: 'run-sql',
      category: 'schema',
      categoryLabel: 'Schema & Modeling',
      name: 'Batch SQL Script Runner',
      desc: 'Stream and execute large multi-statement migration scripts',
      icon: 'file-text',
      iconColor: 'text-amber-400',
      action: () => appState.openRunSqlModal()
    },
    {
      id: 'snippets',
      category: 'schema',
      categoryLabel: 'Schema & Modeling',
      name: 'SQL Query Snippets & Templates',
      desc: 'Reusable SQL templates, boilerplate & procedural snippets',
      icon: 'bookmark',
      iconColor: 'text-sky-400',
      action: () => appState.openSnippetsModal()
    },
    {
      id: 'refresh-catalogs',
      category: 'schema',
      categoryLabel: 'Schema & Modeling',
      name: 'Purge & Reload Catalog Cache',
      desc: 'Force rescan metadata dictionary across all connected schemas',
      icon: 'rotate-ccw',
      iconColor: 'text-amber-400',
      action: () => (appState.isRefreshAllOpen = true)
    },

    // Security & Access
    {
      id: 'connections',
      category: 'security',
      categoryLabel: 'Security & Access',
      name: 'Connection Profiles & Vault',
      desc: 'Database server configurations, encrypted credentials & SSH jump hosts',
      icon: 'database',
      iconColor: 'text-sky-400',
      action: () => appState.openNewConnectionModal()
    },
    {
      id: 'privilege-matrix',
      category: 'security',
      categoryLabel: 'Security & Access',
      name: 'User Privileges & Access Grants',
      desc: 'Database user accounts, roles & granular table permissions',
      icon: 'shield',
      iconColor: 'text-purple-400',
      action: () => appState.openPrivilegeMatrixModal()
    },

    // System & Preferences
    {
      id: 'settings',
      category: 'preferences',
      categoryLabel: 'System & Preferences',
      name: 'Application Preferences & Config',
      desc: 'Query timeouts, display scaling, telemetry & master encryption key',
      icon: 'sliders',
      iconColor: 'text-slate-300',
      action: () => (appState.isSettingsModalOpen = true)
    },
    {
      id: 'shortcuts',
      category: 'preferences',
      categoryLabel: 'System & Preferences',
      name: 'Keyboard Shortcuts Reference',
      desc: 'Comprehensive hotkeys cheatsheet and navigation guide',
      icon: 'keyboard',
      iconColor: 'text-amber-400',
      shortcut: '?',
      action: () => (appState.isShortcutsModalOpen = true)
    },
    {
      id: 'updates',
      category: 'preferences',
      categoryLabel: 'System & Preferences',
      name: 'Software Releases & Updates',
      desc: 'Version changelog and deterministic in-app self update',
      icon: 'zap',
      iconColor: 'text-amber-400',
      action: () => (appState.isUpdateModalOpen = true)
    }
  ];

  let filteredTools = $derived.by(() => {
    let list = allTools;
    if (activeToolsCategory !== 'all') {
      list = list.filter((t) => t.category === activeToolsCategory);
    }
    const q = toolsSearchQuery.toLowerCase().trim();
    if (!q) return list;
    return list.filter(
      (t) =>
        t.name.toLowerCase().includes(q) ||
        t.desc.toLowerCase().includes(q) ||
        t.categoryLabel.toLowerCase().includes(q)
    );
  });

  function handleToolsKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (filteredTools.length > 0) {
        selectedToolIndex = (selectedToolIndex + 1) % filteredTools.length;
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (filteredTools.length > 0) {
        selectedToolIndex = (selectedToolIndex - 1 + filteredTools.length) % filteredTools.length;
      }
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (filteredTools[selectedToolIndex]) {
        filteredTools[selectedToolIndex].action();
        showToolsDropdown = false;
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      showToolsDropdown = false;
    }
  }

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
                  class="absolute right-2 text-slate-400 hover:text-slate-200 p-0.5 rounded hover:bg-slate-800 transition"
                  aria-label="Clear connection search"
                >
                  <Icon name="x" class="w-3.5 h-3.5" />
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
        onclick={() => {
          showToolsDropdown = !showToolsDropdown;
          if (showToolsDropdown) {
            toolsSearchQuery = '';
            activeToolsCategory = 'all';
            selectedToolIndex = 0;
          }
        }}
        class="h-7 px-2.5 rounded {showToolsDropdown
          ? 'bg-slate-700 text-white ring-1 ring-sky-500/50'
          : 'bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white'} border border-[#334155] flex items-center space-x-1.5 transition shadow-sm font-medium"
        title="Database Tools & Enterprise Utilities"
      >
        <Icon name="wrench" class="w-3.5 h-3.5 text-sky-400" />
        <span>Tools</span>
        <Icon name="chevron-down" class="w-3 h-3 text-slate-400 transition-transform {showToolsDropdown ? 'rotate-180' : ''}" />
      </button>

      {#if showToolsDropdown}
        <button
          tabindex="-1"
          aria-label="Close"
          class="fixed inset-0 z-40 bg-transparent cursor-default"
          onclick={() => (showToolsDropdown = false)}
        ></button>

        <div class="absolute right-0 mt-1.5 w-[680px] max-w-[calc(100vw-2rem)] bg-[#0b0f19]/95 border border-slate-700/80 rounded-2xl shadow-2xl z-50 overflow-hidden animate-in fade-in duration-100 text-xs backdrop-blur-2xl">
          <!-- Search Header & Category Tabs -->
          <div class="p-3 border-b border-slate-800/80 bg-slate-900/60 space-y-2.5">
            <div class="relative">
              <Icon name="search" class="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5 pointer-events-none" />
              <input
                type="text"
                bind:value={toolsSearchQuery}
                onkeydown={handleToolsKeydown}
                placeholder="Search tools by name, description, or enterprise category (↑↓ to navigate, Enter to run)..."
                class="w-full bg-[#050811] border border-slate-700/80 rounded-lg pl-9 pr-8 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500 transition shadow-inner"
              />
              {#if toolsSearchQuery}
                <button
                  type="button"
                  onclick={() => {
                    toolsSearchQuery = '';
                    selectedToolIndex = 0;
                  }}
                  class="absolute right-2.5 top-2 text-slate-400 hover:text-slate-200 p-0.5 rounded"
                  aria-label="Clear search"
                >
                  <Icon name="x" class="w-3 h-3" />
                </button>
              {/if}
            </div>

            <!-- Category Filter Tabs -->
            <div class="flex items-center gap-1.5 overflow-x-auto pb-0.5 scrollbar-none text-[11px]">
              {#each toolCategories as cat}
                {@const isCatActive = activeToolsCategory === cat.id}
                {@const catCount = cat.id === 'all' ? allTools.length : allTools.filter((t) => t.category === cat.id).length}
                <button
                  type="button"
                  onclick={() => {
                    activeToolsCategory = cat.id;
                    selectedToolIndex = 0;
                  }}
                  class="px-2.5 py-1 rounded-md transition flex items-center space-x-1.5 whitespace-nowrap font-medium border {isCatActive
                    ? 'bg-sky-950/80 text-sky-200 border-sky-600/80 shadow-sm'
                    : 'bg-slate-900/60 text-slate-400 hover:text-slate-200 hover:bg-slate-800/80 border-slate-800/80'}"
                >
                  <span class="w-1.5 h-1.5 rounded-full {cat.dotColor}"></span>
                  <span>{cat.label}</span>
                  <span class="text-[9px] font-mono px-1 py-0.2 rounded bg-slate-800/80 text-slate-400">{catCount}</span>
                </button>
              {/each}
            </div>
          </div>

          <!-- Body -->
          <div class="max-h-[70vh] overflow-y-auto p-3.5">
            {#if toolsSearchQuery.trim() || activeToolsCategory !== 'all'}
              <!-- Flat Filtered View -->
              {#if filteredTools.length === 0}
                <div class="py-10 text-center text-slate-500 text-xs">
                  <Icon name="search" class="w-6 h-6 mx-auto mb-2 text-slate-600 opacity-60" />
                  No tools found matching "{toolsSearchQuery}" in {activeToolsCategory === 'all' ? 'any category' : activeToolsCategory}
                </div>
              {:else}
                <div class="space-y-1">
                  {#each filteredTools as tool, idx}
                    {@const isHighlighted = selectedToolIndex === idx}
                    <button
                      type="button"
                      onmouseenter={() => (selectedToolIndex = idx)}
                      onclick={() => {
                        tool.action();
                        showToolsDropdown = false;
                      }}
                      class="w-full text-left px-3 py-2 rounded-xl transition flex items-center space-x-3 group border {isHighlighted
                        ? 'bg-slate-800/90 border-slate-700/80 shadow-sm'
                        : 'hover:bg-slate-800/60 border-transparent text-slate-300'}"
                    >
                      <div class="w-8 h-8 rounded-lg bg-slate-900 border border-slate-700/70 flex items-center justify-center shrink-0 group-hover:border-slate-600 group-hover:shadow transition">
                        <Icon name={tool.icon} class="w-4 h-4 {tool.iconColor}" />
                      </div>
                      <div class="min-w-0 flex-1">
                        <div class="flex items-center justify-between gap-2">
                          <span class="font-semibold text-slate-200 group-hover:text-white truncate">{tool.name}</span>
                          <div class="flex items-center space-x-1.5 shrink-0">
                            {#if tool.badge}
                              <span class="text-[9px] font-mono px-1.5 py-0.2 rounded bg-emerald-950 text-emerald-300 border border-emerald-800">{tool.badge}</span>
                            {/if}
                            {#if tool.shortcut}
                              <kbd class="text-[9px] font-mono text-slate-400 bg-slate-800 px-1 py-0.2 rounded border border-slate-700">{tool.shortcut}</kbd>
                            {/if}
                            <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-slate-800 text-slate-400 font-mono">{tool.categoryLabel}</span>
                          </div>
                        </div>
                        <div class="text-[11px] text-slate-400 truncate mt-0.5">{tool.desc}</div>
                      </div>
                    </button>
                  {/each}
                </div>
              {/if}
            {:else}
              <!-- 2-Column Enterprise Overview Layout -->
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <!-- Left Column -->
                <div class="space-y-4">
                  <!-- Section: DBA & Monitoring -->
                  <div class="space-y-1">
                    <div class="text-[10.5px] font-bold uppercase tracking-wider text-slate-400 px-2.5 py-1 flex items-center justify-between bg-slate-900/40 rounded-lg border border-slate-800/40 mb-1.5">
                      <span class="flex items-center space-x-2">
                        <span class="w-2 h-2 rounded-full bg-sky-400"></span>
                        <span class="text-slate-200 font-semibold">DBA & Monitoring</span>
                      </span>
                      <span class="text-[9px] font-mono text-slate-500">4 tools</span>
                    </div>
                    {#each allTools.filter((t) => t.category === 'dba') as tool}
                      <button
                        type="button"
                        onclick={() => {
                          tool.action();
                          showToolsDropdown = false;
                        }}
                        class="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-slate-800/90 text-slate-300 hover:text-white flex items-center space-x-2.5 transition group border border-transparent hover:border-slate-700/60"
                      >
                        <div class="w-7 h-7 rounded-lg bg-slate-900 border border-slate-800 flex items-center justify-center shrink-0 group-hover:border-slate-700">
                          <Icon name={tool.icon} class="w-3.5 h-3.5 {tool.iconColor}" />
                        </div>
                        <div class="min-w-0 flex-1">
                          <div class="flex items-center justify-between">
                            <span class="text-xs font-semibold text-slate-200 group-hover:text-white truncate">{tool.name}</span>
                            {#if tool.badge}
                              <span class="text-[8.5px] font-mono px-1 py-0.2 rounded bg-emerald-950 text-emerald-300 border border-emerald-800">{tool.badge}</span>
                            {/if}
                          </div>
                          <div class="text-[10px] text-slate-400 truncate leading-tight mt-0.5">{tool.desc}</div>
                        </div>
                      </button>
                    {/each}
                  </div>

                  <!-- Section: Security & Access -->
                  <div class="space-y-1">
                    <div class="text-[10.5px] font-bold uppercase tracking-wider text-slate-400 px-2.5 py-1 flex items-center justify-between bg-slate-900/40 rounded-lg border border-slate-800/40 mb-1.5">
                      <span class="flex items-center space-x-2">
                        <span class="w-2 h-2 rounded-full bg-purple-400"></span>
                        <span class="text-slate-200 font-semibold">Security & Access</span>
                      </span>
                      <span class="text-[9px] font-mono text-slate-500">2 tools</span>
                    </div>
                    {#each allTools.filter((t) => t.category === 'security') as tool}
                      <button
                        type="button"
                        onclick={() => {
                          tool.action();
                          showToolsDropdown = false;
                        }}
                        class="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-slate-800/90 text-slate-300 hover:text-white flex items-center space-x-2.5 transition group border border-transparent hover:border-slate-700/60"
                      >
                        <div class="w-7 h-7 rounded-lg bg-slate-900 border border-slate-800 flex items-center justify-center shrink-0 group-hover:border-slate-700">
                          <Icon name={tool.icon} class="w-3.5 h-3.5 {tool.iconColor}" />
                        </div>
                        <div class="min-w-0 flex-1">
                          <div class="text-xs font-semibold text-slate-200 group-hover:text-white truncate">{tool.name}</div>
                          <div class="text-[10px] text-slate-400 truncate leading-tight mt-0.5">{tool.desc}</div>
                        </div>
                      </button>
                    {/each}
                  </div>

                  <!-- Section: System & Preferences -->
                  <div class="space-y-1">
                    <div class="text-[10.5px] font-bold uppercase tracking-wider text-slate-400 px-2.5 py-1 flex items-center justify-between bg-slate-900/40 rounded-lg border border-slate-800/40 mb-1.5">
                      <span class="flex items-center space-x-2">
                        <span class="w-2 h-2 rounded-full bg-slate-400"></span>
                        <span class="text-slate-200 font-semibold">System & Preferences</span>
                      </span>
                      <span class="text-[9px] font-mono text-slate-500">3 tools</span>
                    </div>
                    {#each allTools.filter((t) => t.category === 'preferences') as tool}
                      <button
                        type="button"
                        onclick={() => {
                          tool.action();
                          showToolsDropdown = false;
                        }}
                        class="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-slate-800/90 text-slate-300 hover:text-white flex items-center space-x-2.5 transition group border border-transparent hover:border-slate-700/60"
                      >
                        <div class="w-7 h-7 rounded-lg bg-slate-900 border border-slate-800 flex items-center justify-center shrink-0 group-hover:border-slate-700">
                          <Icon name={tool.icon} class="w-3.5 h-3.5 {tool.iconColor}" />
                        </div>
                        <div class="min-w-0 flex-1">
                          <div class="flex items-center justify-between">
                            <span class="text-xs font-semibold text-slate-200 group-hover:text-white truncate">{tool.name}</span>
                            {#if tool.shortcut}
                              <kbd class="text-[9px] font-mono text-slate-400 bg-slate-800 px-1 py-0.2 rounded border border-slate-700">{tool.shortcut}</kbd>
                            {/if}
                          </div>
                          <div class="text-[10px] text-slate-400 truncate leading-tight mt-0.5">{tool.desc}</div>
                        </div>
                      </button>
                    {/each}
                  </div>
                </div>

                <!-- Right Column -->
                <div class="space-y-4">
                  <!-- Section: Data Movement & ETL -->
                  <div class="space-y-1">
                    <div class="text-[10.5px] font-bold uppercase tracking-wider text-slate-400 px-2.5 py-1 flex items-center justify-between bg-slate-900/40 rounded-lg border border-slate-800/40 mb-1.5">
                      <span class="flex items-center space-x-2">
                        <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
                        <span class="text-slate-200 font-semibold">Data Movement & ETL</span>
                      </span>
                      <span class="text-[9px] font-mono text-slate-500">5 tools</span>
                    </div>
                    {#each allTools.filter((t) => t.category === 'data') as tool}
                      <button
                        type="button"
                        onclick={() => {
                          tool.action();
                          showToolsDropdown = false;
                        }}
                        class="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-slate-800/90 text-slate-300 hover:text-white flex items-center space-x-2.5 transition group border border-transparent hover:border-slate-700/60"
                      >
                        <div class="w-7 h-7 rounded-lg bg-slate-900 border border-slate-800 flex items-center justify-center shrink-0 group-hover:border-slate-700">
                          <Icon name={tool.icon} class="w-3.5 h-3.5 {tool.iconColor}" />
                        </div>
                        <div class="min-w-0 flex-1">
                          <div class="text-xs font-semibold text-slate-200 group-hover:text-white truncate">{tool.name}</div>
                          <div class="text-[10px] text-slate-400 truncate leading-tight mt-0.5">{tool.desc}</div>
                        </div>
                      </button>
                    {/each}
                  </div>

                  <!-- Section: Schema & Modeling -->
                  <div class="space-y-1">
                    <div class="text-[10.5px] font-bold uppercase tracking-wider text-slate-400 px-2.5 py-1 flex items-center justify-between bg-slate-900/40 rounded-lg border border-slate-800/40 mb-1.5">
                      <span class="flex items-center space-x-2">
                        <span class="w-2 h-2 rounded-full bg-indigo-400"></span>
                        <span class="text-slate-200 font-semibold">Schema & Modeling</span>
                      </span>
                      <span class="text-[9px] font-mono text-slate-500">5 tools</span>
                    </div>
                    {#each allTools.filter((t) => t.category === 'schema') as tool}
                      <button
                        type="button"
                        onclick={() => {
                          tool.action();
                          showToolsDropdown = false;
                        }}
                        class="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-slate-800/90 text-slate-300 hover:text-white flex items-center space-x-2.5 transition group border border-transparent hover:border-slate-700/60"
                      >
                        <div class="w-7 h-7 rounded-lg bg-slate-900 border border-slate-800 flex items-center justify-center shrink-0 group-hover:border-slate-700">
                          <Icon name={tool.icon} class="w-3.5 h-3.5 {tool.iconColor}" />
                        </div>
                        <div class="min-w-0 flex-1">
                          <div class="text-xs font-semibold text-slate-200 group-hover:text-white truncate">{tool.name}</div>
                          <div class="text-[10px] text-slate-400 truncate leading-tight mt-0.5">{tool.desc}</div>
                        </div>
                      </button>
                    {/each}
                  </div>
                </div>
              </div>
            {/if}
          </div>

          <!-- Footer Bar -->
          <div class="px-3.5 py-2.5 bg-slate-900/80 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-400 font-sans">
            <span class="flex items-center space-x-2">
              <Icon name="brand" class="w-4 h-4 text-orange-500" />
              <span class="font-medium text-slate-300">SqlFox Enterprise Database Suite</span>
              <span class="text-slate-600">•</span>
              <span class="text-[10px] text-slate-500">19 Managed Utilities</span>
            </span>
            <div class="flex items-center space-x-3 text-[10px] text-slate-400">
              <span><kbd class="px-1 py-0.5 rounded bg-slate-800 border border-slate-700 text-slate-300 font-mono">↑↓</kbd> Navigate</span>
              <span><kbd class="px-1 py-0.5 rounded bg-slate-800 border border-slate-700 text-slate-300 font-mono">↵</kbd> Select</span>
              <span><kbd class="px-1 py-0.5 rounded bg-slate-800 border border-slate-700 text-slate-300 font-mono">Esc</kbd> Close</span>
            </div>
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
