<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let showDbDropdown = $state(false);
  let dbFilter = $state('');

  // Table Context Menu
  let tableContextMenu = $state<{
    open: boolean;
    x: number;
    y: number;
    tableName: string;
  } | null>(null);

  function handleTableContextMenu(e: MouseEvent, tableName: string) {
    e.preventDefault();
    tableContextMenu = {
      open: true,
      x: e.clientX,
      y: e.clientY,
      tableName,
    };
  }

  function closeTableMenu() {
    if (tableContextMenu) tableContextMenu.open = false;
  }

  function formatCount(cnt?: number): string {
    if (cnt === undefined || cnt === null) return '';
    if (cnt >= 1_000_000) return `${(cnt / 1_000_000).toFixed(1)}M`;
    if (cnt >= 1_000) return `${(cnt / 1_000).toFixed(1)}k`;
    return cnt.toString();
  }

  let filteredDbList = $derived.by(() => {
    const list = appState.availableDatabases || [];
    const q = dbFilter.toLowerCase().trim();
    if (!q) return list;
    return list.filter((db) => db.toLowerCase().includes(q));
  });
</script>

<svelte:window onclick={closeTableMenu} />

{#if !appState.sidebarCollapsed}
  <aside class="w-64 border-r border-[#334155] bg-[#090d16] flex flex-col h-full select-none shrink-0 text-slate-300">
    <!-- Database Switcher Header -->
    <div class="p-2 border-b border-[#1e293b] bg-[#0b0f19] relative">
      <div class="text-[10px] uppercase font-bold tracking-wider text-slate-500 mb-1 flex items-center justify-between px-1">
        <span class="flex items-center space-x-1">
          <Icon name="database" class="w-3 h-3 text-sky-400" />
          <span>Active Database</span>
        </span>
        {#if appState.availableDatabases.length > 0}
          <span class="text-[9px] font-mono text-slate-400 bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800">
            {appState.availableDatabases.length} DBs
          </span>
        {/if}
      </div>

      <!-- Database Selector Trigger Button -->
      <button
        onclick={() => (showDbDropdown = !showDbDropdown)}
        class="w-full bg-[#0f172a] hover:bg-[#1e293b] border border-[#334155] rounded px-2.5 py-1.5 text-xs text-left text-slate-200 flex items-center justify-between transition-colors group shadow-sm"
        title="Click to switch database"
      >
        <div class="flex items-center space-x-2 truncate">
          <span class="w-2 h-2 rounded-full {appState.currentDatabase ? 'bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.6)]' : 'bg-slate-600'} shrink-0"></span>
          <span class="font-mono font-semibold text-slate-100 truncate text-[12px]">
            {appState.currentDatabase || 'Select Database...'}
          </span>
        </div>
        <Icon name="chevron-down" class="w-3.5 h-3.5 text-slate-400 group-hover:text-sky-400 shrink-0 transition-transform {showDbDropdown ? 'rotate-180' : ''}" />
      </button>

      <!-- Database Dropdown Menu -->
      {#if showDbDropdown}
        <button
          tabindex="-1"
          aria-label="Close"
          class="fixed inset-0 z-40 bg-transparent cursor-default"
          onclick={() => { showDbDropdown = false; dbFilter = ''; }}
        ></button>

        <div class="absolute left-2 right-2 top-full mt-1 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in duration-100">
          <!-- DB Search Filter -->
          <div class="p-1.5">
            <div class="relative flex items-center">
              <Icon name="search" class="w-3 h-3 text-slate-500 absolute left-2 pointer-events-none" />
              <input
                type="text"
                bind:value={dbFilter}
                placeholder="Search databases..."
                class="w-full bg-[#090d16] border border-[#334155] rounded pl-7 pr-5 py-1 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 font-mono"
              />
              {#if dbFilter}
                <button
                  onclick={() => (dbFilter = '')}
                  class="absolute right-1.5 text-slate-500 hover:text-slate-300 text-xs"
                >
                  ×
                </button>
              {/if}
            </div>
          </div>

          <!-- DB List in Dropdown -->
          <div class="max-h-56 overflow-y-auto py-1">
            {#if appState.isConnecting}
              <div class="p-4 text-center text-xs text-sky-400 flex items-center justify-center space-x-2">
                <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin shrink-0" />
                <span>Connecting to database...</span>
              </div>
            {:else if appState.connectionError}
              <div class="p-3 text-center text-xs text-rose-300 space-y-1.5">
                <p class="font-medium">Connection failed</p>
                <p class="text-[10px] text-rose-400 font-mono truncate">{appState.connectionError}</p>
                <button
                  onclick={() => appState.activeConnection && appState.selectConnection(appState.activeConnection)}
                  class="px-2 py-0.5 bg-rose-950 hover:bg-rose-900 border border-rose-800 text-rose-200 rounded text-[10px] font-semibold transition"
                >
                  Retry Connect
                </button>
              </div>
            {:else if !appState.activeConnection}
              <div class="p-4 text-center text-xs text-slate-400 space-y-2">
                <p class="font-medium">No connection selected</p>
                <button
                  onclick={() => { showDbDropdown = false; appState.openNewConnectionModal(); }}
                  class="px-2.5 py-1 bg-sky-950 hover:bg-sky-900 border border-sky-800 text-sky-400 rounded text-xs font-semibold"
                >
                  Add Connection
                </button>
              </div>
            {:else if appState.availableDatabases.length === 0}
              <div class="p-4 text-center text-xs text-slate-400 space-y-2">
                <p class="italic text-slate-500">No databases found</p>
                <button
                  onclick={() => appState.refreshDatabases()}
                  class="px-2.5 py-1 bg-sky-950 hover:bg-sky-900 border border-sky-800 text-sky-400 rounded text-[11px] font-semibold transition inline-flex items-center space-x-1"
                >
                  <Icon name="refresh-cw" class="w-3 h-3 shrink-0" />
                  <span>Reload Databases</span>
                </button>
              </div>
            {:else if filteredDbList.length === 0}
              <div class="p-3 text-center text-xs text-slate-500 space-y-1">
                <p class="italic">No databases matching "{dbFilter}"</p>
                <button
                  onclick={() => (dbFilter = '')}
                  class="text-sky-400 hover:underline text-[11px]"
                >
                  Clear search
                </button>
              </div>
            {:else}
              {#each filteredDbList as db}
                <button
                  onclick={() => {
                    appState.selectDatabase(db);
                    showDbDropdown = false;
                    dbFilter = '';
                  }}
                  class="w-full text-left px-3 py-1.5 flex items-center justify-between hover:bg-slate-800 text-xs transition-colors {db === appState.currentDatabase ? 'bg-sky-500/10 text-sky-400 font-bold' : 'text-slate-300'}"
                >
                  <span class="truncate font-mono">{db}</span>
                  <div class="flex items-center space-x-1.5 shrink-0 ml-1">
                    {#if appState.databaseTableCounts[db] !== undefined}
                      <span class="text-[9px] text-slate-500 font-mono">
                        {appState.databaseTableCounts[db]} tbls
                      </span>
                    {/if}
                    {#if db === appState.currentDatabase}
                      <Icon name="check" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                    {/if}
                  </div>
                </button>
              {/each}
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <!-- View Mode Switcher: Objects vs All Databases -->
    <div class="px-2 pt-1.5 pb-1 border-b border-[#1e293b] bg-[#0b0f19] flex items-center space-x-1">
      <button
        onclick={() => (appState.sidebarViewMode = 'objects')}
        class="flex-1 py-1 px-2 text-center text-[11px] font-medium rounded transition {appState.sidebarViewMode === 'objects' ? 'bg-[#1e293b] text-sky-400 font-semibold shadow-sm' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900/50'}"
      >
        Objects ({appState.filteredTables.length})
      </button>
      <button
        onclick={() => (appState.sidebarViewMode = 'databases')}
        class="flex-1 py-1 px-2 text-center text-[11px] font-medium rounded transition {appState.sidebarViewMode === 'databases' ? 'bg-[#1e293b] text-sky-400 font-semibold shadow-sm' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900/50'}"
      >
        Databases ({appState.availableDatabases.length})
      </button>
    </div>

    <!-- Search Filter Header -->
    <div class="p-2 border-b border-[#1e293b] bg-[#0b0f19]">
      <div class="relative flex items-center">
        <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2.5 pointer-events-none" />
        <input
          type="text"
          bind:value={appState.schemaFilter}
          placeholder={appState.sidebarViewMode === 'databases' ? "Filter databases..." : "Filter tables, views..."}
          class="w-full bg-[#0f172a] border border-[#334155] rounded pl-8 pr-6 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500 transition-all font-mono"
        />
        {#if appState.schemaFilter}
          <button
            onclick={() => (appState.schemaFilter = '')}
            class="absolute right-2 text-slate-500 hover:text-slate-300 text-xs"
            aria-label="Clear filter"
          >
            ×
          </button>
        {/if}
      </div>
    </div>

    <!-- Content: Databases List Mode OR Schema Objects Mode -->
    <div class="flex-1 overflow-y-auto p-1.5 space-y-1">
      {#if appState.isLoadingCatalog}
        <div class="p-8 text-center text-xs text-slate-400 flex flex-col items-center gap-2">
          <Icon name="refresh-cw" class="w-5 h-5 animate-spin text-sky-400" />
          <span class="font-medium text-slate-300">Loading catalog...</span>
          <span class="text-[10px] text-slate-500 font-mono">{appState.currentDatabase}</span>
        </div>
      {:else if appState.sidebarViewMode === 'databases'}\
        <!-- ALL DATABASES VIEW -->
        <div class="space-y-0.5">
          <div class="px-2 py-1 text-[10px] font-bold text-slate-500 uppercase tracking-wider flex items-center justify-between">
            <span>Available Databases</span>
            <span class="font-mono text-slate-400">{appState.filteredDatabases.length}</span>
          </div>

          {#if appState.availableDatabases.length === 0}
            <div class="p-6 text-center text-xs text-slate-500 space-y-2">
              <p class="italic">No databases found for this connection</p>
              <button
                onclick={() => appState.refreshDatabases()}
                class="px-2.5 py-1 bg-sky-950 hover:bg-sky-900 border border-sky-800 text-sky-400 rounded text-xs font-semibold transition inline-flex items-center space-x-1"
              >
                <Icon name="refresh-cw" class="w-3 h-3 shrink-0" />
                <span>Reload Databases</span>
              </button>
            </div>
          {:else if appState.filteredDatabases.length === 0}
            <div class="p-6 text-center text-xs text-slate-500 space-y-1">
              <p class="italic">No databases matching "{appState.schemaFilter}"</p>
              <button
                onclick={() => (appState.schemaFilter = '')}
                class="text-sky-400 hover:underline text-xs"
              >
                Clear filter
              </button>
            </div>
          {:else}
            {#each appState.filteredDatabases as db}
              <button
                onclick={() => {
                  appState.selectDatabase(db);
                  appState.sidebarViewMode = 'objects';
                }}
                class="w-full text-left px-2.5 py-1.5 rounded flex items-center justify-between group hover:bg-[#1e293b]/80 text-xs transition-colors {db === appState.currentDatabase ? 'bg-sky-500/10 text-sky-400 font-bold border border-sky-800/40' : 'text-slate-300'}"
              >
                <div class="flex items-center space-x-2 truncate">
                  <Icon name="database" class="w-3.5 h-3.5 shrink-0 {db === appState.currentDatabase ? 'text-sky-400' : 'text-slate-500 group-hover:text-sky-400'}" />
                  <span class="truncate font-mono text-[11.5px]">{db}</span>
                </div>
                <div class="flex items-center space-x-1.5 shrink-0">
                  {#if appState.databaseTableCounts[db] !== undefined}
                    <span class="text-[10px] text-slate-400 font-mono bg-slate-900/80 px-1.5 py-0.2 rounded border border-slate-800">
                      {appState.databaseTableCounts[db]} tbls
                    </span>
                  {/if}
                  {#if db === appState.currentDatabase}
                    <span class="text-[9px] uppercase font-mono px-1.5 py-0.2 rounded bg-sky-950 text-sky-400 border border-sky-800">active</span>
                  {/if}
                </div>
              </button>
            {/each}
          {/if}
        </div>
      {:else if !appState.catalog || (!appState.catalog.tables?.length && !appState.catalog.routines?.length && !appState.catalog.triggers?.length)}
        <div class="p-8 text-center text-xs text-slate-500 flex flex-col items-center gap-3">
          <Icon name="database" class="w-8 h-8 text-slate-600" />
          <div class="space-y-1">
            <span class="font-medium text-slate-300 block">Database "{appState.currentDatabase || 'None'}" has no tables</span>
            <span class="text-[11px] text-slate-500 block">Select another database with tables</span>
          </div>
          <button
            onclick={() => (appState.sidebarViewMode = 'databases')}
            class="px-3 py-1.5 bg-sky-950/60 hover:bg-sky-900/60 text-sky-400 border border-sky-800/60 rounded text-xs font-semibold transition flex items-center space-x-1.5 shadow-sm"
          >
            <Icon name="database" class="w-3.5 h-3.5" />
            <span>Browse All {appState.availableDatabases.length} Databases</span>
          </button>
        </div>
      {:else}
        <!-- SECTION: Tables -->
        {#if appState.filteredTables.length > 0}
          <div class="rounded overflow-hidden">
            <button
              onclick={() => appState.toggleSection('tables')}
              class="w-full px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between hover:bg-slate-800/50 rounded transition-colors"
            >
              <span class="flex items-center space-x-1.5">
                <Icon
                  name={appState.collapsedSections.tables ? 'chevron-right' : 'chevron-down'}
                  class="w-3 h-3 text-slate-500"
                />
                <Icon name="layers" class="w-3.5 h-3.5 text-sky-400" />
                <span>Tables</span>
              </span>
              <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400">
                {appState.filteredTables.length}
              </span>
            </button>

            {#if !appState.collapsedSections.tables}
              <div class="space-y-0.5 mt-0.5 pl-1">
                {#each appState.filteredTables as table (table.name)}
                  <div
                    role="listitem"
                    oncontextmenu={(e) => handleTableContextMenu(e, table.name)}
                    class="flex items-center group rounded hover:bg-[#1e293b]/80 pr-1 transition-colors"
                  >
                    <button
                      onclick={() => appState.openTableDataTab(table.name)}
                      class="flex-1 text-left px-2 py-1 flex items-center justify-between text-slate-300 group-hover:text-sky-300 text-xs truncate"
                      title="View data for {table.name} (Right-click for options)"
                    >
                      <div class="flex items-center space-x-2 truncate">
                        <Icon name="table" class="w-3.5 h-3.5 text-slate-500 group-hover:text-sky-400 shrink-0 transition-colors" />
                        <span class="truncate font-mono text-[11.5px]">{table.name}</span>
                      </div>

                      {#if (table.row_count !== undefined && table.row_count !== null) || (table.rows_count !== undefined && table.rows_count !== null)}
                        <span class="text-[10px] text-slate-500 group-hover:text-slate-400 font-mono bg-slate-900/60 px-1.5 py-0.2 rounded border border-slate-800/80 ml-1.5 shrink-0">
                          {formatCount(table.row_count ?? table.rows_count)}
                        </span>
                      {/if}
                    </button>

                    <!-- Structure / DDL Action Button -->
                    <button
                      onclick={() => appState.openTableStructureTab(table.name)}
                      class="opacity-0 group-hover:opacity-100 p-1 hover:bg-slate-700 rounded text-slate-400 hover:text-white transition-opacity"
                      title="View Table Structure"
                    >
                      <Icon name="columns" class="w-3 h-3" />
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Views -->
        {#if appState.filteredViews.length > 0}
          <div class="rounded overflow-hidden pt-1">
            <button
              onclick={() => appState.toggleSection('views')}
              class="w-full px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between hover:bg-slate-800/50 rounded transition-colors"
            >
              <span class="flex items-center space-x-1.5">
                <Icon
                  name={appState.collapsedSections.views ? 'chevron-right' : 'chevron-down'}
                  class="w-3 h-3 text-slate-500"
                />
                <Icon name="eye" class="w-3.5 h-3.5 text-emerald-400" />
                <span>Views</span>
              </span>
              <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400">
                {appState.filteredViews.length}
              </span>
            </button>

            {#if !appState.collapsedSections.views}
              <div class="space-y-0.5 mt-0.5 pl-1">
                {#each appState.filteredViews as view (view.name)}
                  <button
                    onclick={() => appState.openTableDataTab(view.name)}
                    class="w-full text-left px-2 py-1 rounded flex items-center justify-between group hover:bg-[#1e293b]/80 text-slate-300 hover:text-emerald-300 text-xs transition-colors"
                    title="View view {view.name}"
                  >
                    <div class="flex items-center space-x-2 truncate">
                      <Icon name="eye" class="w-3.5 h-3.5 text-emerald-500 group-hover:text-emerald-400 shrink-0 transition-colors" />
                      <span class="truncate font-mono text-[11.5px]">{view.name}</span>
                    </div>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Routines & Procedures -->
        {#if appState.filteredRoutines.length > 0}
          <div class="rounded overflow-hidden pt-1">
            <button
              onclick={() => appState.toggleSection('routines')}
              class="w-full px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between hover:bg-slate-800/50 rounded transition-colors"
            >
              <span class="flex items-center space-x-1.5">
                <Icon
                  name={appState.collapsedSections.routines ? 'chevron-right' : 'chevron-down'}
                  class="w-3 h-3 text-slate-500"
                />
                <Icon name="zap" class="w-3.5 h-3.5 text-amber-400" />
                <span>Routines</span>
              </span>
              <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400">
                {appState.filteredRoutines.length}
              </span>
            </button>

            {#if !appState.collapsedSections.routines}
              <div class="space-y-0.5 mt-0.5 pl-1">
                {#each appState.filteredRoutines as routine (routine.name)}
                  <button
                    onclick={() => appState.openRoutineTab(routine.name, (routine.type as any) || 'PROCEDURE')}
                    class="w-full text-left px-2 py-1 rounded flex items-center justify-between group hover:bg-[#1e293b]/80 text-slate-300 hover:text-amber-300 text-xs transition-colors"
                    title="View definition for {routine.name}"
                  >
                    <div class="flex items-center space-x-2 truncate">
                      <Icon name="zap" class="w-3.5 h-3.5 text-amber-500 group-hover:text-amber-400 shrink-0 transition-colors" />
                      <span class="truncate font-mono text-[11.5px]">{routine.name}</span>
                    </div>
                    <span class="text-[9px] text-amber-400 font-mono bg-amber-950/40 px-1 rounded border border-amber-800/40 shrink-0 ml-1">
                      {routine.type || 'PROC'}
                    </span>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Triggers -->
        {#if appState.filteredTriggers.length > 0}
          <div class="rounded overflow-hidden pt-1">
            <button
              onclick={() => appState.toggleSection('triggers')}
              class="w-full px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between hover:bg-slate-800/50 rounded transition-colors"
            >
              <span class="flex items-center space-x-1.5">
                <Icon
                  name={appState.collapsedSections.triggers ? 'chevron-right' : 'chevron-down'}
                  class="w-3 h-3 text-slate-500"
                />
                <Icon name="target" class="w-3.5 h-3.5 text-purple-400" />
                <span>Triggers</span>
              </span>
              <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400">
                {appState.filteredTriggers.length}
              </span>
            </button>

            {#if !appState.collapsedSections.triggers}
              <div class="space-y-0.5 mt-0.5 pl-1">
                {#each appState.filteredTriggers as trigger (trigger.name)}
                  <div class="w-full text-left px-2 py-1 rounded flex items-center justify-between group hover:bg-[#1e293b]/80 text-slate-300 text-xs transition-colors">
                    <div class="flex items-center space-x-2 truncate">
                      <Icon name="target" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
                      <span class="truncate font-mono text-[11.5px]">{trigger.name}</span>
                    </div>
                    {#if trigger.timing || trigger.event}
                      <span class="text-[9px] text-purple-400 font-mono bg-purple-950/40 px-1 rounded border border-purple-800/40 shrink-0 ml-1">
                        {trigger.timing} {trigger.event}
                      </span>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/if}
    </div>

    <!-- Bottom Footer Status -->
    <div class="border-t border-[#1e293b] p-2.5 flex items-center justify-between text-xs text-slate-400 bg-[#070a14]">
      <div class="flex items-center space-x-2 font-mono text-[11px] truncate">
        <span class="w-2 h-2 rounded-full {appState.activeSessionId ? 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.6)]' : 'bg-slate-600'}"></span>
        <span class="truncate font-medium">{appState.activeConnection?.name || 'Ready'}</span>
      </div>
      <span class="text-[10px] font-mono text-slate-500 bg-slate-900 px-1.5 py-0.5 rounded border border-slate-800">Svelte 5</span>
    </div>

    <!-- Table Context Menu -->
    {#if tableContextMenu && tableContextMenu.open}
      <div
        style="top: {tableContextMenu.y}px; left: {tableContextMenu.x}px;"
        class="fixed z-50 w-52 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
      >
        <div class="px-3 py-1 text-[10px] font-bold text-slate-400 uppercase tracking-wider font-mono truncate">
          {tableContextMenu.tableName}
        </div>

        <div class="py-1">
          <button
            onclick={() => {
              appState.openTableDataTab(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
          >
            <Icon name="table" class="w-3.5 h-3.5 text-emerald-400" />
            <span>Open Data Grid</span>
          </button>
          <button
            onclick={() => {
              appState.openTableStructureTab(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
          >
            <Icon name="columns" class="w-3.5 h-3.5 text-sky-400" />
            <span>View Structure</span>
          </button>
          <button
            onclick={() => {
              appState.openTableDDLTab(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
          >
            <Icon name="file-code" class="w-3.5 h-3.5 text-purple-400" />
            <span>View CREATE TABLE DDL</span>
          </button>
        </div>

        <div class="py-1">
          <button
            onclick={() => {
              navigator.clipboard.writeText(tableContextMenu!.tableName);
              appState.showToast(`Copied table name "${tableContextMenu!.tableName}"`, 'info');
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
          >
            <Icon name="copy" class="w-3.5 h-3.5 text-slate-400" />
            <span>Copy Table Name</span>
          </button>
        </div>
      </div>
    {/if}
  </aside>
{/if}
