<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let showConnDropdown = $state(false);
  let showHeaderDbDropdown = $state(false);
  let showToolsDropdown = $state(false);
  let showMasterPin = $state(false);
  let copiedPin = $state(false);

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

  function copyMasterPin() {
    navigator.clipboard.writeText('fox-0304');
    copiedPin = true;
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
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700/80 border border-[#334155] flex items-center space-x-2 text-xs text-slate-200 transition shadow-sm"
      >
        <span class="w-2 h-2 rounded-full {appState.activeConnection ? 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]' : 'bg-slate-500'}"></span>
        <span class="font-medium text-slate-100">{appState.activeConnection?.name || 'No active connection'}</span>
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
          <div class="px-3 py-1.5 text-[10px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between">
            <span>Configured Connections</span>
            <span class="font-mono text-slate-500">{appState.connections.length}</span>
          </div>

          <div class="max-h-64 overflow-y-auto py-1">
            {#each appState.connections as conn}
              <div class="flex items-center hover:bg-slate-850 px-3 py-1.5 transition-colors group {conn.id === appState.activeConnection?.id ? 'bg-sky-500/10' : ''}">
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
                  <span class="text-[9px] px-1 py-0.2 rounded border {getEnvBadgeClass(conn.environment)}">
                    {conn.environment}
                  </span>
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

    <!-- Master PIN Pill with Copy & Reveal -->
    <div class="h-7 bg-slate-900 text-sky-400 rounded border border-sky-900/60 font-mono transition inline-flex items-center text-xs overflow-hidden select-none shadow-sm">
      <button
        onclick={copyMasterPin}
        class="h-full px-2 hover:bg-slate-800 text-sky-400 hover:text-sky-300 transition flex items-center space-x-1.5"
        title="Copy Master PIN for LAN access"
      >
        <Icon name="key" class="w-3 h-3 text-sky-400" />
        <span class="text-[10px] text-slate-400">PIN:</span>
        <span class="font-bold text-white tracking-widest text-[11px]">
          {showMasterPin ? 'fox-0304' : '••••••••'}
        </span>
        {#if copiedPin}
          <span class="text-[9px] text-emerald-400 font-sans font-semibold">Copied!</span>
        {/if}
      </button>
      <button
        onclick={() => (showMasterPin = !showMasterPin)}
        class="h-full px-1.5 text-slate-400 hover:text-white hover:bg-slate-800 border-l border-sky-900/60 transition flex items-center justify-center"
        title="Toggle PIN Visibility"
      >
        <Icon name={showMasterPin ? 'eye-off' : 'eye'} class="w-3 h-3" />
      </button>
    </div>

    {#if appState.isProduction}
      <div class="h-7 px-2.5 rounded bg-rose-950/40 border border-rose-500/50 text-rose-300 flex items-center space-x-1.5 shadow-sm">
        <span class="w-1.5 h-1.5 rounded-full bg-rose-500 animate-ping"></span>
        <span class="text-[10px] font-bold tracking-wider uppercase">Production</span>
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
