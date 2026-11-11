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

  type ToolCategoryKey = 'all' | 'schema' | 'data' | 'dba' | 'security' | 'ai' | 'preferences';
  let activeToolsCategory = $state<ToolCategoryKey>('all');
  let selectedToolIndex = $state(0);

  interface ToolItem {
    id: string;
    category: 'schema' | 'data' | 'dba' | 'security' | 'ai' | 'preferences';
    categoryLabel: string;
    name: string;
    desc: string;
    icon: string;
    iconColor: string;
    shortcut?: string;
    badge?: string;
    badgeColor?: string;
    action: () => void;
  }

  const toolCategories: { id: ToolCategoryKey; label: string; icon: string; dotColor: string; description: string }[] = [
    { id: 'all', label: 'All Tools', icon: 'wrench', dotColor: 'bg-slate-400', description: 'Complete catalog of database management tools' },
    { id: 'schema', label: 'Schema & Architecture', icon: 'layers', dotColor: 'bg-indigo-400', description: 'Visual ERD, table/view designers, DDL generators & schema diff' },
    { id: 'data', label: 'Data Movement & ETL', icon: 'upload', dotColor: 'bg-emerald-400', description: 'CSV import, SQL dumps, cross-host transfers & mock generation' },
    { id: 'dba', label: 'DBA & Performance', icon: 'cpu', dotColor: 'bg-sky-400', description: 'Live threads, query kill, tablespace diagnostics & audit logs' },
    { id: 'security', label: 'Security & Access', icon: 'shield', dotColor: 'bg-rose-400', description: 'Connection vaults, credentials & RBAC privilege matrix' },
    { id: 'ai', label: 'Developer & AI', icon: 'bot', dotColor: 'bg-purple-400', description: 'AI Copilot, query snippets, batch runners & team runbooks' },
    { id: 'preferences', label: 'System & Workspace', icon: 'sliders', dotColor: 'bg-slate-400', description: 'Cloud backups, keyboard shortcuts & self-update engine' }
  ];

  const allTools: ToolItem[] = [
    // 1. Schema & Architecture (9 items)
    {
      id: 'erd',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Visual ERD Schema Canvas',
      desc: 'Interactive entity-relationship diagram with zoom, pan & SVG export',
      icon: 'layers',
      iconColor: 'text-indigo-400',
      badge: 'Visual',
      badgeColor: 'bg-indigo-950/80 text-indigo-300 border-indigo-700/60',
      action: () => appState.openErdTab()
    },
    {
      id: 'structure-sync',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Schema Structure Diff & Sync',
      desc: 'Compare database catalog structures across environments and generate migration DDL',
      icon: 'git-diff',
      iconColor: 'text-cyan-400',
      badge: 'Diff',
      badgeColor: 'bg-cyan-950/80 text-cyan-300 border-cyan-700/60',
      action: () => appState.openStructureSyncModal()
    },
    {
      id: 'table-creator',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Table Designer',
      desc: 'Visual table architect for columns, primary keys, engines, and character sets',
      icon: 'table',
      iconColor: 'text-emerald-400',
      badge: 'Designer',
      badgeColor: 'bg-emerald-950/80 text-emerald-300 border-emerald-700/60',
      action: () => (appState.isTableCreatorOpen = true)
    },
    {
      id: 'view-designer',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'View Designer',
      desc: 'Visual query builder and DDL designer for virtual and materialized database views',
      icon: 'eye',
      iconColor: 'text-teal-400',
      badge: 'Designer',
      badgeColor: 'bg-teal-950/80 text-teal-300 border-teal-700/60',
      action: () => appState.openViewDesigner()
    },
    {
      id: 'routine-designer',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Stored Routine Designer',
      desc: 'Programmability designer for stored procedures, functions, parameters & return types',
      icon: 'code',
      iconColor: 'text-amber-400',
      badge: 'Designer',
      badgeColor: 'bg-amber-950/80 text-amber-300 border-amber-700/60',
      action: () => appState.openRoutineDesigner()
    },
    {
      id: 'trigger-designer',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Trigger Designer',
      desc: 'Event-driven triggers configured for BEFORE/AFTER INSERT, UPDATE & DELETE operations',
      icon: 'zap',
      iconColor: 'text-purple-400',
      badge: 'Designer',
      badgeColor: 'bg-purple-950/80 text-purple-300 border-purple-700/60',
      action: () => appState.openTriggerDesigner()
    },
    {
      id: 'event-designer',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Scheduled Event Designer',
      desc: 'Cron-like scheduled database maintenance tasks and recurring automated queries',
      icon: 'clock',
      iconColor: 'text-sky-400',
      badge: 'Designer',
      badgeColor: 'bg-sky-950/80 text-sky-300 border-sky-700/60',
      action: () => appState.openEventDesigner()
    },
    {
      id: 'create-database',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Create New Database',
      desc: 'Provision a new database catalog with custom charset, collation, and driver rules',
      icon: 'database',
      iconColor: 'text-blue-400',
      badge: 'Catalog',
      badgeColor: 'bg-sky-950/80 text-sky-300 border-sky-700/60',
      action: () => (appState.isCreateDbModalOpen = true)
    },
    {
      id: 'refresh-catalogs',
      category: 'schema',
      categoryLabel: 'Schema & Architecture',
      name: 'Reload Metadata Catalog',
      desc: 'Purge cache and synchronize schema dictionary across all active connections',
      icon: 'rotate-ccw',
      iconColor: 'text-teal-400',
      badge: 'Purge',
      badgeColor: 'bg-teal-950/80 text-teal-300 border-teal-700/60',
      action: () => (appState.isRefreshAllOpen = true)
    },

    // 2. Data Movement & ETL (5 items)
    {
      id: 'csv-import',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Import Delimited File (CSV/TSV)',
      desc: 'High-speed chunked CSV/TSV table importer with custom column mapping',
      icon: 'upload',
      iconColor: 'text-emerald-400',
      badge: 'ETL',
      badgeColor: 'bg-emerald-950/80 text-emerald-300 border-emerald-700/60',
      action: () => appState.openCsvImportModal()
    },
    {
      id: 'sql-dump',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Export Database Dump (SQL)',
      desc: 'Full schema DDL definitions and row-level SQL backup dump generator',
      icon: 'download',
      iconColor: 'text-sky-400',
      badge: 'Backup',
      badgeColor: 'bg-sky-950/80 text-sky-300 border-sky-700/60',
      action: () => appState.openDumpModal()
    },
    {
      id: 'data-sync',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Row-Level Data Sync & Diff',
      desc: 'Differential table row comparison and bidirectional reconciliation',
      icon: 'scale',
      iconColor: 'text-teal-400',
      badge: 'Sync',
      badgeColor: 'bg-teal-950/80 text-teal-300 border-teal-700/60',
      action: () => (appState.isDataSyncOpen = true)
    },
    {
      id: 'data-transfer',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Cross-Host Data Transfer',
      desc: 'Direct streaming pipeline between disparate database hosts & engines',
      icon: 'arrow-left-right',
      iconColor: 'text-indigo-400',
      badge: 'Pipeline',
      badgeColor: 'bg-indigo-950/80 text-indigo-300 border-indigo-700/60',
      action: () => (appState.isDataTransferOpen = true)
    },
    {
      id: 'mock-generator',
      category: 'data',
      categoryLabel: 'Data Movement & ETL',
      name: 'Synthetic Mock Data Generator',
      desc: 'Generate schema-aware, foreign-key compliant mock test datasets',
      icon: 'sparkles',
      iconColor: 'text-purple-400',
      badge: 'Mock',
      badgeColor: 'bg-purple-950/80 text-purple-300 border-purple-700/60',
      action: () => (appState.isMockDataModalOpen = true)
    },

    // 3. DBA & Performance (4 items)
    {
      id: 'processlist',
      category: 'dba',
      categoryLabel: 'DBA & Performance',
      name: 'Processlist & Live Sessions',
      desc: 'Inspect active client threads, execution times & kill runaway queries',
      icon: 'cpu',
      iconColor: 'text-sky-400',
      badge: 'Live',
      badgeColor: 'bg-emerald-950/80 text-emerald-300 border-emerald-700/60',
      action: () => appState.openProcesslistTab()
    },
    {
      id: 'diagnostics',
      category: 'dba',
      categoryLabel: 'DBA & Performance',
      name: 'Database Health & Telemetry',
      desc: 'Tablespace fragmentation, buffer cache hit-ratio & engine metrics',
      icon: 'activity',
      iconColor: 'text-rose-400',
      badge: 'Metrics',
      badgeColor: 'bg-rose-950/80 text-rose-300 border-rose-700/60',
      action: () => appState.openDbDiagnosticsModal()
    },
    {
      id: 'activity-console',
      category: 'dba',
      categoryLabel: 'DBA & Performance',
      name: 'System Activity Console',
      desc: 'Real-time daemon log stream, background events & lock monitors',
      icon: 'terminal',
      iconColor: 'text-emerald-400',
      badge: 'Stream',
      badgeColor: 'bg-emerald-950/80 text-emerald-300 border-emerald-700/60',
      action: () => (appState.isActivityDrawerOpen = !appState.isActivityDrawerOpen)
    },
    {
      id: 'audit-log',
      category: 'dba',
      categoryLabel: 'DBA & Performance',
      name: 'Audit Trail & Mutation Logs',
      desc: 'Historical query logs, security events & destructive statement attribution',
      icon: 'clipboard-list',
      iconColor: 'text-amber-400',
      badge: 'Forensics',
      badgeColor: 'bg-amber-950/80 text-amber-300 border-amber-700/60',
      action: () => (appState.isAuditDrawerOpen = true)
    },

    // 4. Security & Access (2 items)
    {
      id: 'connections',
      category: 'security',
      categoryLabel: 'Security & Access',
      name: 'Connection Profiles Vault',
      desc: 'Server endpoints, AES-256 encrypted passwords & SSH jump-host tunnels',
      icon: 'database',
      iconColor: 'text-sky-400',
      badge: 'Vault',
      badgeColor: 'bg-sky-950/80 text-sky-300 border-sky-700/60',
      action: () => appState.openNewConnectionModal()
    },
    {
      id: 'privilege-matrix',
      category: 'security',
      categoryLabel: 'Security & Access',
      name: 'Users & Privileges Matrix',
      desc: 'Manage database accounts, roles, host masks & granular GRANT permissions',
      icon: 'users',
      iconColor: 'text-purple-400',
      badge: 'RBAC',
      badgeColor: 'bg-purple-950/80 text-purple-300 border-purple-700/60',
      action: () => appState.openPrivilegeMatrixModal()
    },

    // 5. Developer & AI Productivity (5 items)
    {
      id: 'copilot',
      category: 'ai',
      categoryLabel: 'Developer & AI',
      name: 'SqlFox AI Copilot Assistant',
      desc: 'Natural language query generation, SQL explanation & query optimization',
      icon: 'bot',
      iconColor: 'text-cyan-400',
      badge: 'Copilot',
      badgeColor: 'bg-cyan-950/80 text-cyan-300 border-cyan-700/60',
      action: () => (appState.isAiDrawerOpen = true)
    },
    {
      id: 'run-sql',
      category: 'ai',
      categoryLabel: 'Developer & AI',
      name: 'Batch SQL Script Runner',
      desc: 'Stream large multi-statement migration scripts and SQL backup files',
      icon: 'file-code',
      iconColor: 'text-amber-400',
      badge: 'Batch',
      badgeColor: 'bg-amber-950/80 text-amber-300 border-amber-700/60',
      action: () => appState.openRunSqlModal()
    },
    {
      id: 'snippets',
      category: 'ai',
      categoryLabel: 'Developer & AI',
      name: 'Query Snippets & Templates',
      desc: 'Curated catalog of reusable parameterized queries and DBA boilerplate',
      icon: 'bookmark',
      iconColor: 'text-sky-400',
      badge: 'Snippets',
      badgeColor: 'bg-sky-950/80 text-sky-300 border-sky-700/60',
      action: () => appState.openSnippetsModal()
    },
    {
      id: 'notes',
      category: 'ai',
      categoryLabel: 'Developer & AI',
      name: 'Operational Runbooks & Notes',
      desc: 'Team knowledge base, table documentation, runbooks & incident logs',
      icon: 'book-open',
      iconColor: 'text-amber-400',
      badge: 'Runbooks',
      badgeColor: 'bg-amber-950/80 text-amber-300 border-amber-700/60',
      action: () => (appState.isNotesDrawerOpen = true)
    },
    {
      id: 'ai-settings',
      category: 'ai',
      categoryLabel: 'Developer & AI',
      name: 'AI Model & Provider Config',
      desc: 'LLM provider credentials (OpenAI, Claude, Gemini, Ollama) & parameters',
      icon: 'sliders',
      iconColor: 'text-purple-400',
      badge: 'Config',
      badgeColor: 'bg-purple-950/80 text-purple-300 border-purple-700/60',
      action: () => (appState.isAiSettingsModalOpen = true)
    },

    // 6. System & Workspace (3 items)
    {
      id: 'settings',
      category: 'preferences',
      categoryLabel: 'System & Workspace',
      name: 'Cloud Sync & Vault Backup',
      desc: 'GitHub sync repository, vault backups, and cloud migration bundles',
      icon: 'cloud',
      iconColor: 'text-sky-400',
      badge: 'Cloud',
      badgeColor: 'bg-sky-950/80 text-sky-300 border-sky-700/60',
      action: () => (appState.isSettingsModalOpen = true)
    },
    {
      id: 'shortcuts',
      category: 'preferences',
      categoryLabel: 'System & Workspace',
      name: 'Keyboard Shortcuts Cheatsheet',
      desc: 'Essential hotkeys for rapid navigation, grid editing & SQL execution',
      icon: 'keyboard',
      iconColor: 'text-amber-400',
      shortcut: '?',
      badge: 'Hotkeys',
      badgeColor: 'bg-slate-800 text-slate-300 border-slate-700',
      action: () => (appState.isShortcutsModalOpen = true)
    },
    {
      id: 'updates',
      category: 'preferences',
      categoryLabel: 'System & Workspace',
      name: 'Check for Software Updates',
      desc: 'Inspect latest release notes, changelog and perform self-update',
      icon: 'arrow-down-circle',
      iconColor: 'text-emerald-400',
      badge: 'Release',
      badgeColor: 'bg-emerald-950/80 text-emerald-300 border-emerald-700/60',
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

<header class="h-12 bg-[#0f172a] border-b border-slate-800 flex items-center justify-between px-4 z-40 flex-shrink-0 gap-3 select-none">
  <!-- Brand, Sidebar Toggle & Active Connection -->
  <div class="flex items-center space-x-3 shrink-0">
    <button
      onclick={() => (appState.sidebarCollapsed = !appState.sidebarCollapsed)}
      class="h-7 w-7 flex items-center justify-center rounded bg-[#1e293b] hover:bg-slate-700 text-slate-400 hover:text-white border border-slate-700/80 hover:border-slate-600 transition text-xs cursor-pointer shadow-sm"
      title="Toggle Sidebar (Ctrl+B)"
    >
      <Icon name="sidebar" class="w-3.5 h-3.5" />
    </button>

    <!-- SqlFox Logo -->
    <div class="flex items-center space-x-2">
      <Icon name="brand" class="w-6 h-6" />
      <span class="font-bold tracking-wide text-white text-base">Sql<span class="text-sky-400">Fox</span></span>
    </div>

    <div class="h-4 w-px bg-slate-800 mx-1"></div>

    <!-- Active Connection Selector Pill -->
    <div class="relative">
      <button
        onclick={() => (showConnDropdown = !showConnDropdown)}
        class="h-7 px-2.5 rounded {appState.isProduction ? 'bg-rose-950/40 border-rose-600/60 hover:bg-rose-900/40 shadow-rose-950/30' : 'bg-[#1e293b] hover:bg-slate-700/80 border-slate-700/80'} border flex items-center space-x-2 text-xs text-slate-200 transition shadow-sm"
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

        <div class="absolute left-0 mt-1 w-72 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in zoom-in-95 duration-100">
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

          <div class="absolute left-0 mt-1 w-60 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in duration-100">
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
      class="h-7 bg-[#1e293b] hover:bg-slate-700 text-slate-300 px-2.5 rounded text-xs flex items-center space-x-1.5 border border-slate-700/80 hover:border-slate-600 transition shadow-sm"
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
          : 'bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white'} border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1.5 transition shadow-sm font-medium"
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

        <div class="absolute right-0 mt-1.5 w-[840px] max-w-[calc(100vw-2rem)] bg-[#0b0f19]/98 border border-slate-700/80 rounded-2xl shadow-2xl z-50 overflow-hidden animate-in fade-in duration-100 text-xs backdrop-blur-2xl ring-1 ring-black/50 flex flex-col max-h-[82vh]">
          <!-- Search Header & Active Scope -->
          <div class="p-3.5 border-b border-slate-800/80 bg-slate-900/80 space-y-2.5 shrink-0">
            <div class="flex items-center justify-between gap-3">
              <div class="relative flex-1 flex items-center">
                <Icon name="search" class="w-4 h-4 text-slate-400 absolute left-3 pointer-events-none" />
                <input
                  type="text"
                  bind:value={toolsSearchQuery}
                  onkeydown={handleToolsKeydown}
                  placeholder="Search database tools by name, category, or keyword (↑↓ to navigate, Enter to launch)..."
                  class="w-full bg-[#050811] border border-slate-700/80 rounded-xl pl-9 pr-24 py-2 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500 transition shadow-inner font-sans"
                />
                <div class="absolute right-3 flex items-center space-x-1.5 pointer-events-none">
                  {#if toolsSearchQuery}
                    <button
                      type="button"
                      onclick={() => {
                        toolsSearchQuery = '';
                        selectedToolIndex = 0;
                      }}
                      class="pointer-events-auto text-slate-400 hover:text-slate-200 p-0.5 rounded transition"
                      aria-label="Clear search"
                    >
                      <Icon name="x" class="w-3.5 h-3.5" />
                    </button>
                  {/if}
                  <span class="text-[10px] font-mono text-slate-400 bg-slate-800/90 px-1.5 py-0.5 rounded-md border border-slate-700/60">
                    {filteredTools.length} {filteredTools.length === 1 ? 'tool' : 'tools'}
                  </span>
                </div>
              </div>

              <!-- Scope Pill -->
              <div class="hidden sm:flex items-center space-x-1.5 px-2.5 py-1.5 rounded-xl bg-slate-950/70 border border-slate-800 text-[11px] font-mono text-slate-300 shrink-0">
                <span class="w-1.5 h-1.5 rounded-full {appState.activeConnection ? 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.8)]' : 'bg-slate-500'}"></span>
                <span class="text-slate-400">Scope:</span>
                <span class="font-semibold text-slate-200 truncate max-w-[140px]">{appState.activeConnection?.name || 'No Connection'}</span>
                {#if appState.currentDatabase}
                  <span class="text-slate-600">/</span>
                  <span class="text-sky-300 font-bold truncate max-w-[120px]">{appState.currentDatabase}</span>
                {/if}
              </div>
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
                  class="px-2.5 py-1 rounded-lg transition flex items-center space-x-1.5 whitespace-nowrap font-medium border {isCatActive
                    ? 'bg-sky-950/90 text-sky-200 border-sky-500/70 shadow-sm shadow-sky-950'
                    : 'bg-slate-900/60 text-slate-400 hover:text-slate-200 hover:bg-slate-800/80 border-slate-800/80'}"
                >
                  <Icon name={cat.icon} class="w-3.5 h-3.5 {isCatActive ? 'text-sky-300' : 'text-slate-500'}" />
                  <span>{cat.label}</span>
                  <span class="text-[9px] font-mono px-1.5 py-0.2 rounded-md {isCatActive ? 'bg-sky-900/80 text-sky-200' : 'bg-slate-800/80 text-slate-400'}">{catCount}</span>
                </button>
              {/each}
            </div>
          </div>

          <!-- Body -->
          <div class="flex-1 overflow-y-auto p-4 space-y-5">
            {#if toolsSearchQuery.trim() || activeToolsCategory !== 'all'}
              <!-- Filtered List / Category View with Keyboard Navigation -->
              {#if filteredTools.length === 0}
                <div class="py-14 text-center text-slate-500 text-xs">
                  <Icon name="search" class="w-8 h-8 mx-auto mb-2 text-slate-600 opacity-60" />
                  <div class="text-slate-200 font-semibold text-sm">No database tools found matching "{toolsSearchQuery}"</div>
                  <div class="text-[11px] text-slate-400 mt-1">Try keywords like "table", "import", "dump", "process", "diff", or "privilege"</div>
                </div>
              {:else}
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
                  {#each filteredTools as tool, idx}
                    {@const isHighlighted = selectedToolIndex === idx}
                    <button
                      type="button"
                      onmouseenter={() => (selectedToolIndex = idx)}
                      onclick={() => {
                        tool.action();
                        showToolsDropdown = false;
                      }}
                      class="text-left p-3 rounded-xl transition-all flex items-start space-x-3 group border {isHighlighted
                        ? 'bg-slate-800/95 border-sky-500/60 shadow-lg ring-1 ring-sky-500/20 text-white'
                        : 'bg-slate-900/40 hover:bg-slate-800/70 border-slate-800/70 text-slate-300 hover:border-slate-700/80'}"
                    >
                      <div class="w-8 h-8 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-center shrink-0 group-hover:border-slate-700 group-hover:scale-105 transition shadow-inner mt-0.5">
                        <Icon name={tool.icon} class="w-4 h-4 {tool.iconColor}" />
                      </div>
                      <div class="min-w-0 flex-1">
                        <div class="flex items-center justify-between gap-1.5">
                          <span class="font-semibold text-slate-100 group-hover:text-white truncate">{tool.name}</span>
                          <div class="flex items-center space-x-1 shrink-0">
                            {#if tool.badge}
                              <span class="text-[9px] font-mono px-1.5 py-0.2 rounded border {tool.badgeColor || 'bg-slate-800 text-slate-300 border-slate-700'}">{tool.badge}</span>
                            {/if}
                            {#if tool.shortcut}
                              <kbd class="text-[9px] font-mono text-slate-400 bg-slate-800 px-1 py-0.2 rounded border border-slate-700">{tool.shortcut}</kbd>
                            {/if}
                          </div>
                        </div>
                        <div class="text-[11px] text-slate-400 group-hover:text-slate-300 leading-snug mt-1 line-clamp-2">{tool.desc}</div>
                      </div>
                      <Icon name="corner-down-left" class="w-3.5 h-3.5 text-sky-400 {isHighlighted ? 'opacity-100' : 'opacity-0'} transition shrink-0 mt-1" />
                    </button>
                  {/each}
                </div>
              {/if}
            {:else}
              <!-- Enterprise Grouped View: 6 Canonical Sections in a Clean Structured Layout -->
              {#each toolCategories.filter((c) => c.id !== 'all') as category}
                {@const catTools = allTools.filter((t) => t.category === category.id)}
                {#if catTools.length > 0}
                  <section class="space-y-2">
                    <!-- Section Header -->
                    <div class="px-3 py-1.5 bg-slate-900/60 rounded-xl border border-slate-800/80 flex items-center justify-between">
                      <div class="flex items-center space-x-2">
                        <span class="w-2 h-2 rounded-full {category.dotColor} shadow-[0_0_8px_rgba(56,189,248,0.5)]"></span>
                        <span class="font-bold text-slate-200 uppercase tracking-wider text-[11px]">{category.label}</span>
                        <span class="text-slate-600 hidden md:inline">•</span>
                        <span class="text-[11px] text-slate-400 hidden md:inline">{category.description}</span>
                      </div>
                      <span class="text-[9.5px] font-mono text-slate-400 bg-slate-800/90 px-2 py-0.5 rounded-md border border-slate-700/60">
                        {catTools.length} {catTools.length === 1 ? 'tool' : 'tools'}
                      </span>
                    </div>

                    <!-- Tools Grid for this Category -->
                    <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
                      {#each catTools as tool}
                        <button
                          type="button"
                          onclick={() => {
                            tool.action();
                            showToolsDropdown = false;
                          }}
                          class="text-left p-2.5 rounded-xl bg-slate-900/40 hover:bg-slate-800/80 border border-slate-800/70 hover:border-slate-700 hover:shadow-md transition-all flex items-start space-x-3 group"
                        >
                          <div class="w-8 h-8 rounded-lg bg-slate-950/80 border border-slate-800/90 flex items-center justify-center shrink-0 group-hover:border-slate-700 group-hover:scale-105 transition shadow-inner mt-0.5">
                            <Icon name={tool.icon} class="w-4 h-4 {tool.iconColor}" />
                          </div>
                          <div class="min-w-0 flex-1">
                            <div class="flex items-center justify-between gap-1.5">
                              <span class="font-semibold text-slate-200 group-hover:text-white truncate text-xs">{tool.name}</span>
                              <div class="flex items-center space-x-1 shrink-0">
                                {#if tool.badge}
                                  <span class="text-[8.5px] font-mono px-1.5 py-0.2 rounded border {tool.badgeColor || 'bg-slate-800 text-slate-300 border-slate-700'}">{tool.badge}</span>
                                {/if}
                                {#if tool.shortcut}
                                  <kbd class="text-[9px] font-mono text-slate-400 bg-slate-800 px-1 py-0.2 rounded border border-slate-700">{tool.shortcut}</kbd>
                                {/if}
                              </div>
                            </div>
                            <div class="text-[10.5px] text-slate-400 group-hover:text-slate-300 leading-snug mt-0.5 line-clamp-1">{tool.desc}</div>
                          </div>
                          <Icon name="chevron-right" class="w-3.5 h-3.5 text-slate-600 opacity-0 group-hover:opacity-100 group-hover:text-slate-400 transition shrink-0 mt-1" />
                        </button>
                      {/each}
                    </div>
                  </section>
                {/if}
              {/each}
            {/if}
          </div>

          <!-- Footer Bar -->
          <div class="px-4 py-2.5 bg-slate-950/95 border-t border-slate-800/90 flex items-center justify-between text-[11px] text-slate-400 font-sans shrink-0">
            <span class="flex items-center space-x-2">
              <Icon name="brand" class="w-4 h-4 text-orange-500" />
              <span class="font-semibold text-slate-200">SqlFox Enterprise Tools</span>
              <span class="text-slate-600">•</span>
              <span class="text-[10px] text-slate-400 font-mono">{allTools.length} Standardized Utilities</span>
            </span>
            <div class="flex items-center space-x-3 text-[10px] text-slate-400">
              <span><kbd class="px-1.5 py-0.5 rounded bg-slate-800/90 border border-slate-700 text-slate-300 font-mono">↑↓</kbd> Navigate</span>
              <span><kbd class="px-1.5 py-0.5 rounded bg-slate-800/90 border border-slate-700 text-slate-300 font-mono">↵</kbd> Launch</span>
              <span><kbd class="px-1.5 py-0.5 rounded bg-slate-800/90 border border-slate-700 text-slate-300 font-mono">Esc</kbd> Close</span>
              <span><kbd class="px-1.5 py-0.5 rounded bg-slate-800/90 border border-slate-700 text-slate-300 font-mono">Ctrl+K</kbd> Palette</span>
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
      class="h-7 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 transition flex items-center space-x-1 shadow-sm relative {appState.isNotesDrawerOpen ? 'bg-amber-950/40 border-amber-600 text-amber-300' : ''}"
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
      class="h-7 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 transition flex items-center space-x-1 shadow-sm {appState.isAiDrawerOpen ? 'bg-sky-950/40 border-sky-500 text-sky-300' : ''}"
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
      class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1.5 transition shadow-sm"
      title="Refresh Schema Catalog"
    >
      <Icon name="refresh-cw" class="w-3.5 h-3.5 {appState.isLoadingCatalog ? 'animate-spin text-sky-400' : 'text-slate-400'}" />
      <span class="hidden sm:inline">Refresh</span>
    </button>
  </div>
</header>
