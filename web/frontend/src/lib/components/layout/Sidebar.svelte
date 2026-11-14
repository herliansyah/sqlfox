<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
  import { onDestroy, tick } from 'svelte';

  // Resizable sidebar width with persistence
  let sidebarWidth = $state(
    Math.max(200, Math.min(500, Number(localStorage.getItem('sqlfox_sidebar_width')) || 260))
  );
  let isDragging = $state(false);

  function startResize(e: MouseEvent) {
    e.preventDefault();
    isDragging = true;
    window.addEventListener('mousemove', onResize);
    window.addEventListener('mouseup', stopResize);
  }

  function onResize(e: MouseEvent) {
    if (!isDragging) return;
    const newWidth = Math.max(200, Math.min(520, e.clientX));
    sidebarWidth = Math.round(newWidth);
  }

  function stopResize() {
    if (!isDragging) return;
    isDragging = false;
    window.removeEventListener('mousemove', onResize);
    window.removeEventListener('mouseup', stopResize);
    localStorage.setItem('sqlfox_sidebar_width', String(sidebarWidth));
  }

  // Release window listeners if the component unmounts mid-resize.
  $effect(() => {
    return () => {
      window.removeEventListener('mousemove', onResize);
      window.removeEventListener('mouseup', stopResize);
    };
  });

  function resetWidth() {
    sidebarWidth = 260;
    localStorage.setItem('sqlfox_sidebar_width', '260');
  }

  onDestroy(stopResize);

  let showDbDropdown = $state(false);
  let dbFilter = $state('');
  let dbActiveIndex = $state(0);
  let dbSearchInput = $state<HTMLInputElement | null>(null);
  let dbMenuList = $state<HTMLDivElement | null>(null);

  $effect(() => {
    if (dbActiveIndex >= filteredDbList.length) dbActiveIndex = Math.max(0, filteredDbList.length - 1);
  });

  async function openDbDropdown() {
    showDbDropdown = true;
    dbFilter = '';
    const idx = (appState.availableDatabases || []).indexOf(appState.currentDatabase || '');
    dbActiveIndex = idx >= 0 ? idx : 0;
    await tick();
    dbSearchInput?.focus();
  }

  function closeDbDropdown(restoreFocus = false) {
    showDbDropdown = false;
    dbFilter = '';
    if (restoreFocus) document.getElementById('sidebar-db-selector-btn')?.focus();
  }

  function chooseDatabase(db: string) {
    appState.selectDatabase(db);
    closeDbDropdown(true);
  }

  function scrollDbActiveIntoView() {
    tick().then(() => dbMenuList?.querySelector('[data-db-active="true"]')?.scrollIntoView({ block: 'nearest' }));
  }

  function handleDbSearchKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (filteredDbList.length) {
        dbActiveIndex = (dbActiveIndex + 1) % filteredDbList.length;
        scrollDbActiveIntoView();
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (filteredDbList.length) {
        dbActiveIndex = (dbActiveIndex - 1 + filteredDbList.length) % filteredDbList.length;
        scrollDbActiveIntoView();
      }
    } else if (e.key === 'Home') {
      e.preventDefault();
      dbActiveIndex = 0;
      scrollDbActiveIntoView();
    } else if (e.key === 'End') {
      e.preventDefault();
      dbActiveIndex = Math.max(0, filteredDbList.length - 1);
      scrollDbActiveIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const db = filteredDbList[dbActiveIndex];
      if (db) chooseDatabase(db);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      closeDbDropdown(true);
    } else if (e.key === 'Tab') {
      closeDbDropdown();
    }
  }

  // Table Context Menu
  let tableContextMenu = $state<{
    open: boolean;
    x: number;
    y: number;
    tableName: string;
  } | null>(null);

  function handleTableContextMenu(e: MouseEvent, tableName: string) {
    e.preventDefault();
    e.stopPropagation();
    const menuWidth = 224;
    const menuHeight = 380;
    const clampedX = Math.max(10, Math.min(e.clientX, window.innerWidth - menuWidth - 10));
    const clampedY = Math.max(10, Math.min(e.clientY, window.innerHeight - menuHeight - 10));

    tableContextMenu = {
      open: true,
      x: clampedX,
      y: clampedY,
      tableName,
    };
  }


  function aiTablePrompt(menu: { tableName: string }): string {
    const db = appState.currentDatabase || 'default';
    return [
      `Analyse table \`${db}\`.\`${menu.tableName}\`.`,
      '',
      'Review the columns used by its keys and indexes, call out missing or redundant indexes, potential data-type risks, and query patterns that would suffer full scans. Propose concrete DDL.',
    ].join('\n');
  }

  function closeTableMenu() {
    if (tableContextMenu) tableContextMenu.open = false;
  }

  function formatCount(cnt?: number): string {
    if (cnt === undefined || cnt === null) return '';
    if (cnt >= 1_000_000) return `${(cnt / 1_000_000).toFixed(1)}M`;
    if (cnt >= 1_000) return `${(cnt / 1_000).toFixed(1)}k`;
    return cnt.toLocaleString();
  }

  let filteredDbList = $derived.by(() => {
    const list = appState.availableDatabases || [];
    const q = dbFilter.toLowerCase().trim();
    if (!q) return list;
    return list.filter((db) => db.toLowerCase().includes(q));
  });

  let collapsedPrefixes = $state<Record<string, boolean>>({});

  function togglePrefix(p: string) {
    collapsedPrefixes[p] = !collapsedPrefixes[p];
  }

  let prefixTree = $derived.by(() => {
    if (!appState.prefixGroupingEnabled) return null;
    const groups: Record<string, typeof appState.filteredTables> = {};
    const ungrouped: typeof appState.filteredTables = [];

    appState.filteredTables.forEach((t) => {
      const idx = t.name.indexOf('_');
      if (idx > 0) {
        const prefix = t.name.substring(0, idx);
        if (!groups[prefix]) groups[prefix] = [];
        groups[prefix].push(t);
      } else {
        ungrouped.push(t);
      }
    });

    const finalGroups: { prefix: string; tables: typeof appState.filteredTables }[] = [];
    Object.entries(groups).forEach(([prefix, tbls]) => {
      if (tbls.length >= 2) {
        finalGroups.push({ prefix, tables: tbls });
      } else {
        ungrouped.push(...tbls);
      }
    });

    return {
      groups: finalGroups.sort((a, b) => a.prefix.localeCompare(b.prefix)),
      ungrouped: ungrouped.sort((a, b) => a.name.localeCompare(b.name)),
    };
  });

  const isActiveTable = (tableName: string) => {
    return appState.activeTab?.tableName === tableName;
  };

  function copySelectStatement(tableName: string) {
    const sql = `SELECT * FROM \`${tableName}\` LIMIT 100;`;
    navigator.clipboard.writeText(sql);
    appState.showToast(`Copied SELECT query for "${tableName}"`, 'info');
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (showDbDropdown) {
        showDbDropdown = false;
        dbFilter = '';
      }
      closeTableMenu();
    }
  }
</script>

<svelte:window onclick={closeTableMenu} onkeydown={handleKeydown} />

{#if !appState.sidebarCollapsed}
  <aside
    style="width: {sidebarWidth}px;"
    class="relative border-r border-slate-800 bg-[#090d16] flex flex-col h-full select-none shrink-0 text-slate-300 transition-[width] duration-75"
  >
    <!-- Database Switcher Header (Height h-9: 36px - aligned with TabBar) -->
    <div class="h-9 px-2 border-b border-slate-800 bg-[#0b0f19] flex items-center justify-between gap-1.5 relative shrink-0">
      <!-- Database Selector Trigger Button -->
      <button
        id="sidebar-db-selector-btn"
        type="button"
        onclick={() => (showDbDropdown ? closeDbDropdown() : openDbDropdown())}
        aria-haspopup="listbox"
        aria-expanded={showDbDropdown}
        class="flex-1 h-7 bg-[#0f172a] hover:bg-[#1e293b] border border-slate-700/80 hover:border-slate-600 rounded px-2 text-xs text-left text-slate-200 flex items-center justify-between transition-colors group shadow-sm cursor-pointer min-w-0"
        title="Switch database (searchable)"
      >
        <div class="flex items-center space-x-1.5 truncate min-w-0">
          <span
            class="w-1.5 h-1.5 rounded-full {appState.currentDatabase ? 'bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.7)]' : 'bg-slate-600'} shrink-0"
          ></span>
          <Icon name="database" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
          <span class="font-mono font-semibold text-slate-100 truncate text-[11.5px]">
            {appState.currentDatabase || 'Select Database...'}
          </span>
        </div>
        <div class="flex items-center space-x-1.5 shrink-0 ml-1">
          {#if appState.availableDatabases.length > 0}
            <span class="text-[9px] font-mono text-slate-400 bg-slate-900/90 px-1.5 py-0.2 rounded border border-slate-800">
              {appState.availableDatabases.length}
            </span>
          {/if}
          <Icon
            name="chevron-down"
            class="w-3 h-3 text-slate-400 group-hover:text-sky-400 shrink-0 transition-transform {showDbDropdown ? 'rotate-180' : ''}"
          />
        </div>
      </button>

      <!-- Quick Reload Schema Button -->
      <button
        type="button"
        onclick={() => appState.refreshDatabases()}
        disabled={appState.isLoadingCatalog}
        class="h-7 w-7 flex items-center justify-center rounded bg-[#0f172a] hover:bg-[#1e293b] border border-slate-700/80 hover:border-slate-600 text-slate-400 hover:text-sky-300 transition-colors shadow-sm cursor-pointer shrink-0 disabled:opacity-50"
        title="Refresh schema and database list"
      >
        <Icon name="refresh-cw" class="w-3.5 h-3.5 {appState.isLoadingCatalog ? 'animate-spin text-sky-400' : ''}" />
      </button>

      <!-- Database Dropdown Menu -->
      {#if showDbDropdown}
        <button
          tabindex="-1"
          aria-label="Close"
          class="fixed inset-0 z-40 bg-transparent cursor-default"
          onclick={() => closeDbDropdown()}
        ></button>

        <div class="absolute left-2 right-2 top-full mt-1 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 animate-in fade-in duration-100">
          <!-- DB Search Filter -->
          <div class="p-1.5">
            <div class="relative flex items-center">
              <Icon name="search" class="w-3 h-3 text-slate-500 absolute left-2 pointer-events-none" />
              <input
                type="text"
                bind:this={dbSearchInput}
                bind:value={dbFilter}
                onkeydown={handleDbSearchKeydown}
                oninput={() => (dbActiveIndex = 0)}
                role="combobox"
                aria-expanded="true"
                aria-controls="sidebar-db-listbox"
                aria-autocomplete="list"
                placeholder="Search databases... (↑↓ Enter)"
                class="w-full bg-[#090d16] border border-slate-700/80 rounded pl-7 pr-6 py-1 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 font-mono"
              />
              {#if dbFilter}
                <button
                  type="button"
                  onclick={() => (dbFilter = '')}
                  class="absolute right-1.5 top-1.5 text-slate-500 hover:text-slate-300 p-0.5 rounded"
                  aria-label="Clear filter"
                >
                  <Icon name="x" class="w-3 h-3" />
                </button>
              {/if}
            </div>
          </div>

          <!-- DB List in Dropdown -->
          <div id="sidebar-db-listbox" bind:this={dbMenuList} role="listbox" class="max-h-56 overflow-y-auto py-1">
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
                  type="button"
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
                  type="button"
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
                  type="button"
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
                  type="button"
                  onclick={() => (dbFilter = '')}
                  class="text-sky-400 hover:underline text-[11px]"
                >
                  Clear search
                </button>
              </div>
            {:else}
              {#each filteredDbList as db, idx (db)}
                {@const isActiveDb = idx === dbActiveIndex}
                <button
                  type="button"
                  data-db-active={isActiveDb}
                  role="option"
                  aria-selected={db === appState.currentDatabase}
                  onmouseenter={() => (dbActiveIndex = idx)}
                  onclick={() => chooseDatabase(db)}
                  class="w-full text-left px-3 py-1.5 flex items-center justify-between text-xs transition-colors {isActiveDb
                    ? 'bg-slate-800/90 ring-1 ring-inset ring-sky-500/40'
                    : 'hover:bg-slate-800/80'} {db === appState.currentDatabase ? 'bg-sky-500/10 text-sky-400 font-bold' : 'text-slate-300'}"
                >
                  <div class="flex items-center space-x-2 truncate min-w-0">
                    <Icon name="database" class="w-3.5 h-3.5 {db === appState.currentDatabase ? 'text-sky-400' : 'text-slate-500'} shrink-0" />
                    <span class="truncate font-mono">{db}</span>
                  </div>
                  <div class="flex items-center space-x-1.5 shrink-0 ml-1">
                    {#if appState.databaseTableCounts[db] !== undefined}
                      <span class="text-[9px] text-slate-400 font-mono bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800">
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

          <!-- Bottom Action: Create Database -->
          <div class="p-1.5 border-t border-slate-800">
            <button
              type="button"
              onclick={() => {
                showDbDropdown = false;
                appState.isCreateDbModalOpen = true;
              }}
              class="w-full py-1.5 px-3 bg-sky-950/60 hover:bg-sky-900/60 border border-sky-800 text-sky-300 font-semibold rounded text-xs flex items-center justify-center space-x-1.5 transition"
            >
              <Icon name="plus" class="w-3.5 h-3.5" />
              <span>Create New Database</span>
            </button>
          </div>
        </div>
      {/if}
    </div>

    <!-- Search Filter & View Mode Switcher Header (Height h-9: 36px - aligned with Content Toolbar) -->
    <div class="h-9 px-2 border-b border-slate-800 bg-[#0b0f19] flex items-center space-x-1.5 relative shrink-0">
      <!-- Search Filter Input -->
      <div class="relative flex-1 flex items-center min-w-0">
        <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2 pointer-events-none" />
        <input
          type="text"
          bind:value={appState.schemaFilter}
          placeholder={appState.sidebarViewMode === 'databases' ? 'Filter DBs...' : 'Filter schema...'}
          class="w-full h-7 bg-[#0f172a] border border-slate-700/80 rounded pl-7 pr-6 text-[11px] text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500 transition-all font-mono"
        />
        {#if appState.schemaFilter}
          <button
            type="button"
            onclick={() => (appState.schemaFilter = '')}
            class="absolute right-1.5 text-slate-400 hover:text-slate-200 p-0.5 rounded transition"
            aria-label="Clear filter"
          >
            <Icon name="x" class="w-3 h-3" />
          </button>
        {/if}
      </div>

      <!-- Segmented View Mode Switcher -->
      <div class="inline-flex rounded border border-slate-700/80 bg-[#090d16] p-0.5 shrink-0" role="group">
        <button
          type="button"
          onclick={() => (appState.sidebarViewMode = 'objects')}
          class="px-2 py-0.5 text-[10px] font-semibold rounded transition {appState.sidebarViewMode === 'objects' ? 'bg-sky-500 text-slate-950 font-bold shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
          title="Schema Objects ({appState.filteredTables.length})"
        >
          Objects
        </button>
        <button
          type="button"
          onclick={() => (appState.sidebarViewMode = 'databases')}
          class="px-2 py-0.5 text-[10px] font-semibold rounded transition {appState.sidebarViewMode === 'databases' ? 'bg-sky-500 text-slate-950 font-bold shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
          title="Available Databases ({appState.availableDatabases.length})"
        >
          DBs
        </button>
      </div>
    </div>

    <!-- Content: Databases List Mode OR Schema Objects Mode -->
    <div class="flex-1 overflow-y-auto p-1.5 space-y-1">
      {#if appState.isLoadingCatalog}
        <div class="p-8 text-center text-xs text-slate-400 flex flex-col items-center gap-2">
          <Icon name="refresh-cw" class="w-5 h-5 animate-spin text-sky-400" />
          <span class="font-medium text-slate-300">Loading catalog...</span>
          <span class="text-[10px] text-slate-500 font-mono truncate max-w-full px-2">{appState.currentDatabase}</span>
        </div>
      {:else if appState.sidebarViewMode === 'databases'}
        <!-- ALL DATABASES VIEW -->
        <div class="space-y-0.5">
          <div class="px-2 py-1 text-[10px] font-bold text-slate-500 uppercase tracking-wider flex items-center justify-between">
            <span>Available Databases</span>
            <span class="font-mono text-slate-400 bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800">{appState.filteredDatabases.length}</span>
          </div>

          {#if appState.availableDatabases.length === 0}
            <div class="p-6 text-center text-xs text-slate-500 space-y-2">
              <p class="italic">No databases found for this connection</p>
              <button
                type="button"
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
                type="button"
                onclick={() => (appState.schemaFilter = '')}
                class="text-sky-400 hover:underline text-xs"
              >
                Clear filter
              </button>
            </div>
          {:else}
            {#each appState.filteredDatabases as db}
              <div
                class="w-full text-left px-2.5 py-1 rounded-md flex items-center justify-between group hover:bg-[#1e293b]/80 text-xs transition-colors {db === appState.currentDatabase ? 'bg-sky-500/10 text-sky-300 font-bold border border-sky-800/40 shadow-[inset_0_0_10px_rgba(56,189,248,0.06)]' : 'text-slate-300'}"
              >
                <button
                  type="button"
                  onclick={() => {
                    appState.selectDatabase(db);
                    appState.sidebarViewMode = 'objects';
                  }}
                  class="flex-1 flex items-center space-x-2 truncate text-left min-w-0"
                >
                  <Icon name="database" class="w-3.5 h-3.5 shrink-0 {db === appState.currentDatabase ? 'text-sky-400' : 'text-slate-500 group-hover:text-sky-400'}" />
                  <span class="truncate font-mono text-[11.5px]">{db}</span>
                </button>
                <div class="flex items-center space-x-1.5 shrink-0 ml-1">
                  {#if appState.databaseTableCounts[db] !== undefined}
                    <span class="text-[10px] text-slate-400 font-mono bg-slate-900/80 px-1.5 py-0.2 rounded border border-slate-800">
                      {appState.databaseTableCounts[db]} tbls
                    </span>
                  {/if}
                  {#if db === appState.currentDatabase}
                    <span class="text-[9px] uppercase font-mono px-1.5 py-0.2 rounded bg-sky-950 text-sky-400 border border-sky-800">active</span>
                  {/if}
                  <button
                    type="button"
                    onclick={(e) => {
                      e.stopPropagation();
                      appState.dropDatabase(db);
                    }}
                    class="opacity-0 group-hover:opacity-100 p-1 text-slate-500 hover:text-rose-400 hover:bg-rose-950/50 rounded transition"
                    title="Drop database '{db}'"
                  >
                    <Icon name="trash-2" class="w-3 h-3" />
                  </button>
                </div>
              </div>
            {/each}
          {/if}
        </div>
      {:else if !appState.catalog || (!appState.catalog.tables?.length && !appState.catalog.routines?.length && !appState.catalog.triggers?.length)}
        <div class="p-8 text-center text-xs text-slate-500 flex flex-col items-center gap-3">
          <Icon name="database" class="w-8 h-8 text-slate-600" />
          <div class="space-y-1">
            <span class="font-medium text-slate-300 block">Database "{appState.currentDatabase || 'None'}" has no tables</span>
            <span class="text-[11px] text-slate-500 block">Select another database or create a new table</span>
          </div>
          <div class="flex flex-col gap-2 w-full pt-1">
            <button
              type="button"
              onclick={() => (appState.isTableCreatorOpen = true)}
              class="w-full px-3 py-1.5 bg-sky-950/80 hover:bg-sky-900 border border-sky-800 text-sky-300 rounded text-xs font-semibold transition flex items-center justify-center space-x-1.5 shadow-sm"
            >
              <Icon name="plus" class="w-3.5 h-3.5" />
              <span>Create New Table</span>
            </button>
            <button
              type="button"
              onclick={() => (appState.sidebarViewMode = 'databases')}
              class="w-full px-3 py-1.5 bg-[#0f172a] hover:bg-slate-800 text-slate-300 border border-slate-700 rounded text-xs font-medium transition flex items-center justify-center space-x-1.5 shadow-sm"
            >
              <Icon name="database" class="w-3.5 h-3.5 text-sky-400" />
              <span>Browse All {appState.availableDatabases.length} Databases</span>
            </button>
          </div>
        </div>
      {:else}
        <!-- SECTION: Tables -->
        {#if appState.filteredTables.length > 0}
          <div class="rounded overflow-hidden">
            <div class="flex items-center justify-between hover:bg-slate-800/50 rounded pr-1 group">
              <button
                type="button"
                onclick={() => appState.toggleSection('tables')}
                class="flex-1 px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between transition-colors min-w-0"
              >
                <span class="flex items-center space-x-1.5 truncate">
                  <Icon
                    name={appState.collapsedSections.tables ? 'chevron-right' : 'chevron-down'}
                    class="w-3 h-3 text-slate-500 shrink-0"
                  />
                  <Icon name="table" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                  <span class="truncate">Tables</span>
                </span>
                <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400 shrink-0 ml-1">
                  {appState.filteredTables.length}
                </span>
              </button>

              <div class="flex items-center space-x-0.5 shrink-0">
                <button
                  type="button"
                  onclick={() => (appState.prefixGroupingEnabled = !appState.prefixGroupingEnabled)}
                  class="px-1.5 py-0.5 rounded text-[10px] font-mono transition flex items-center space-x-1 {appState.prefixGroupingEnabled ? 'text-sky-300 bg-sky-950/80 font-bold border border-sky-800 shadow-sm' : 'text-slate-400 hover:text-slate-200 bg-slate-900/60 border border-slate-800/80'}"
                  title="Toggle Schema Tree Prefix Grouping (Group by prefix_)"
                >
                  <Icon name="folder" class="w-2.5 h-2.5 {appState.prefixGroupingEnabled ? 'text-sky-400' : 'text-slate-500'}" />
                  <span>grp</span>
                </button>
                <button
                  type="button"
                  onclick={() => appState.openErdTab()}
                  class="p-1 text-slate-400 hover:text-indigo-400 hover:bg-indigo-950/40 rounded transition"
                  title="Open ERD Schema Canvas"
                >
                  <Icon name="layers" class="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  onclick={() => (appState.isTableCreatorOpen = true)}
                  class="p-1 text-slate-400 hover:text-sky-300 hover:bg-sky-950/60 rounded transition"
                  title="Create New Table"
                >
                  <Icon name="plus" class="w-3.5 h-3.5" />
                </button>
              </div>
            </div>

            {#if !appState.collapsedSections.tables}
              <div class="space-y-0.5 mt-0.5 pl-0.5">
                {#if prefixTree}
                  <!-- Grouped by Prefix (ADR 0058) -->
                  {#each prefixTree.groups as grp}
                    <div class="rounded overflow-hidden">
                      <button
                        type="button"
                        onclick={() => togglePrefix(grp.prefix)}
                        class="w-full px-2 py-1 text-[11px] font-semibold text-slate-300 hover:text-white hover:bg-slate-800/50 rounded flex items-center justify-between transition group/folder"
                      >
                        <span class="flex items-center space-x-1.5 truncate min-w-0">
                          <Icon name={collapsedPrefixes[grp.prefix] ? 'chevron-right' : 'chevron-down'} class="w-3 h-3 text-slate-500 shrink-0" />
                          <Icon name="folder" class="w-3.5 h-3.5 text-sky-400/80 shrink-0" />
                          <span class="text-sky-300 font-mono truncate">{grp.prefix}_*</span>
                        </span>
                        <span class="text-[9.5px] font-mono text-slate-400 bg-slate-900/90 px-1.5 py-0.2 rounded border border-slate-800 shrink-0 ml-1">
                          {grp.tables.length}
                        </span>
                      </button>

                      {#if !collapsedPrefixes[grp.prefix]}
                        <div class="pl-2 space-y-0.5 border-l border-slate-800/80 ml-2.5 my-0.5">
                          {#each grp.tables as table (table.name)}
                            <div
                              role="listitem"
                              oncontextmenu={(e) => handleTableContextMenu(e, table.name)}
                              class="relative flex items-center group rounded-md transition-all text-xs {isActiveTable(table.name)
                                ? 'bg-sky-500/15 text-sky-200 font-medium border-l-2 border-sky-400 shadow-[inset_0_0_12px_rgba(56,189,248,0.06)]'
                                : tableContextMenu?.open && tableContextMenu.tableName === table.name
                                  ? 'bg-[#1e293b] text-slate-100 border-l-2 border-sky-500/60'
                                  : 'text-slate-300 hover:bg-[#1e293b]/70 hover:text-slate-100 border-l-2 border-transparent'}"
                            >
                              <button
                                type="button"
                                onclick={() => appState.openTableDataTab(table.name)}
                                class="flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                                title="Open {table.name} (Right-click for options)"
                              >
                                <div class="flex items-center space-x-2 truncate min-w-0 pr-1">
                                  <Icon
                                    name="table"
                                    class="w-3.5 h-3.5 shrink-0 transition-colors {isActiveTable(table.name) ? 'text-sky-400' : 'text-slate-500 group-hover:text-sky-400'}"
                                  />
                                  <span class="truncate font-mono text-[11.5px]">{table.name}</span>
                                </div>

                                {#if (table.row_count !== undefined && table.row_count !== null) || (table.rows_count !== undefined && table.rows_count !== null)}
                                  <span
                                    class="text-[9.5px] text-slate-400 font-mono bg-slate-900/80 group-hover:bg-slate-800 px-1.5 py-0.2 rounded border border-slate-800/80 ml-1.5 shrink-0 tabular-nums transition-opacity {tableContextMenu?.open && tableContextMenu.tableName === table.name ? 'opacity-0' : 'group-hover:opacity-0'}"
                                  >
                                    {formatCount(table.row_count ?? table.rows_count)}
                                  </span>
                                {/if}
                              </button>

                              <!-- Quick action buttons -->
                              <div
                                class="absolute right-1 flex items-center space-x-0.5 rounded transition-all {tableContextMenu?.open && tableContextMenu.tableName === table.name
                                  ? 'opacity-100 pointer-events-auto bg-[#0f172a]/95 border border-slate-700/80 shadow-sm px-0.5 py-0.5'
                                  : 'opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/90 hover:bg-[#0f172a] border border-slate-700/60 shadow-sm px-0.5 py-0.5 backdrop-blur-xs'}"
                              >
                                <button
                                  type="button"
                                  onclick={(e) => { e.stopPropagation(); appState.openTableStructureTab(table.name); }}
                                  class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                                  title="View Table Structure"
                                >
                                  <Icon name="columns" class="w-3 h-3" />
                                </button>
                                <button
                                  type="button"
                                  onclick={(e) => handleTableContextMenu(e, table.name)}
                                  class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-white transition"
                                  title="More options"
                                >
                                  <Icon name="more-vertical" class="w-3 h-3" />
                                </button>
                              </div>
                            </div>
                          {/each}
                        </div>
                      {/if}
                    </div>
                  {/each}

                  {#each prefixTree.ungrouped as table (table.name)}
                    <div
                      role="listitem"
                      oncontextmenu={(e) => handleTableContextMenu(e, table.name)}
                      class="relative flex items-center group rounded-md transition-all text-xs {isActiveTable(table.name)
                        ? 'bg-sky-500/15 text-sky-200 font-medium border-l-2 border-sky-400 shadow-[inset_0_0_12px_rgba(56,189,248,0.06)]'
                        : tableContextMenu?.open && tableContextMenu.tableName === table.name
                          ? 'bg-[#1e293b] text-slate-100 border-l-2 border-sky-500/60'
                          : 'text-slate-300 hover:bg-[#1e293b]/70 hover:text-slate-100 border-l-2 border-transparent'}"
                    >
                      <button
                        type="button"
                        onclick={() => appState.openTableDataTab(table.name)}
                        class="flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                        title="Open {table.name} (Right-click for options)"
                      >
                        <div class="flex items-center space-x-2 truncate min-w-0 pr-1">
                          <Icon
                            name="table"
                            class="w-3.5 h-3.5 shrink-0 transition-colors {isActiveTable(table.name) ? 'text-sky-400' : 'text-slate-500 group-hover:text-sky-400'}"
                          />
                          <span class="truncate font-mono text-[11.5px]">{table.name}</span>
                        </div>

                        {#if (table.row_count !== undefined && table.row_count !== null) || (table.rows_count !== undefined && table.rows_count !== null)}
                          <span
                            class="text-[9.5px] text-slate-400 font-mono bg-slate-900/80 group-hover:bg-slate-800 px-1.5 py-0.2 rounded border border-slate-800/80 ml-1.5 shrink-0 tabular-nums transition-opacity {tableContextMenu?.open && tableContextMenu.tableName === table.name ? 'opacity-0' : 'group-hover:opacity-0'}"
                          >
                            {formatCount(table.row_count ?? table.rows_count)}
                          </span>
                        {/if}
                      </button>

                      <!-- Quick action buttons -->
                      <div
                        class="absolute right-1 flex items-center space-x-0.5 rounded transition-all {tableContextMenu?.open && tableContextMenu.tableName === table.name
                          ? 'opacity-100 pointer-events-auto bg-[#0f172a]/95 border border-slate-700/80 shadow-sm px-0.5 py-0.5'
                          : 'opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/90 hover:bg-[#0f172a] border border-slate-700/60 shadow-sm px-0.5 py-0.5 backdrop-blur-xs'}"
                      >
                        <button
                          type="button"
                          onclick={(e) => { e.stopPropagation(); appState.openTableStructureTab(table.name); }}
                          class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                          title="View Table Structure"
                        >
                          <Icon name="columns" class="w-3 h-3" />
                        </button>
                        <button
                          type="button"
                          onclick={(e) => handleTableContextMenu(e, table.name)}
                          class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-white transition"
                          title="More options"
                        >
                          <Icon name="more-vertical" class="w-3 h-3" />
                        </button>
                      </div>
                    </div>
                  {/each}
                {:else}
                  <!-- Flat Table List -->
                  {#each appState.filteredTables as table (table.name)}
                    <div
                      role="listitem"
                      oncontextmenu={(e) => handleTableContextMenu(e, table.name)}
                      class="relative flex items-center group rounded-md transition-all text-xs {isActiveTable(table.name)
                        ? 'bg-sky-500/15 text-sky-200 font-medium border-l-2 border-sky-400 shadow-[inset_0_0_12px_rgba(56,189,248,0.06)]'
                        : tableContextMenu?.open && tableContextMenu.tableName === table.name
                          ? 'bg-[#1e293b] text-slate-100 border-l-2 border-sky-500/60'
                          : 'text-slate-300 hover:bg-[#1e293b]/70 hover:text-slate-100 border-l-2 border-transparent'}"
                    >
                      <button
                        type="button"
                        onclick={() => appState.openTableDataTab(table.name)}
                        class="flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                        title="Open {table.name} (Right-click for options)"
                      >
                        <div class="flex items-center space-x-2 truncate min-w-0 pr-1">
                          <Icon
                            name="table"
                            class="w-3.5 h-3.5 shrink-0 transition-colors {isActiveTable(table.name) ? 'text-sky-400' : 'text-slate-500 group-hover:text-sky-400'}"
                          />
                          <span class="truncate font-mono text-[11.5px]">{table.name}</span>
                        </div>

                        {#if (table.row_count !== undefined && table.row_count !== null) || (table.rows_count !== undefined && table.rows_count !== null)}
                          <span
                            class="text-[9.5px] text-slate-400 font-mono bg-slate-900/80 group-hover:bg-slate-800 px-1.5 py-0.2 rounded border border-slate-800/80 ml-1.5 shrink-0 tabular-nums transition-opacity {tableContextMenu?.open && tableContextMenu.tableName === table.name ? 'opacity-0' : 'group-hover:opacity-0'}"
                          >
                            {formatCount(table.row_count ?? table.rows_count)}
                          </span>
                        {/if}
                      </button>

                      <!-- Quick action buttons -->
                      <div
                        class="absolute right-1 flex items-center space-x-0.5 rounded transition-all {tableContextMenu?.open && tableContextMenu.tableName === table.name
                          ? 'opacity-100 pointer-events-auto bg-[#0f172a]/95 border border-slate-700/80 shadow-sm px-0.5 py-0.5'
                          : 'opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/90 hover:bg-[#0f172a] border border-slate-700/60 shadow-sm px-0.5 py-0.5 backdrop-blur-xs'}"
                      >
                        <button
                          type="button"
                          onclick={(e) => { e.stopPropagation(); appState.openTableStructureTab(table.name); }}
                          class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                          title="View Table Structure"
                        >
                          <Icon name="columns" class="w-3 h-3" />
                        </button>
                        <button
                          type="button"
                          onclick={(e) => handleTableContextMenu(e, table.name)}
                          class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-white transition"
                          title="More options"
                        >
                          <Icon name="more-vertical" class="w-3 h-3" />
                        </button>
                      </div>
                    </div>
                  {/each}
                {/if}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Views -->
        {#if appState.filteredViews.length > 0}
          <div class="rounded overflow-hidden pt-1">
            <div class="flex items-center justify-between hover:bg-slate-800/50 rounded pr-1 group">
              <button
                type="button"
                onclick={() => appState.toggleSection('views')}
                class="flex-1 px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between transition-colors min-w-0"
              >
                <span class="flex items-center space-x-1.5 truncate">
                  <Icon
                    name={appState.collapsedSections.views ? 'chevron-right' : 'chevron-down'}
                    class="w-3 h-3 text-slate-500 shrink-0"
                  />
                  <Icon name="eye" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                  <span class="truncate">Views</span>
                </span>
                <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400 shrink-0 ml-1">
                  {appState.filteredViews.length}
                </span>
              </button>

              <button
                type="button"
                onclick={() => appState.openViewDesigner()}
                class="p-1 text-slate-400 hover:text-emerald-300 hover:bg-emerald-950/60 rounded transition shrink-0"
                title="Create New View"
              >
                <Icon name="plus" class="w-3.5 h-3.5" />
              </button>
            </div>

            {#if !appState.collapsedSections.views}
              <div class="space-y-0.5 mt-0.5 pl-0.5">
                {#each appState.filteredViews as view (view.name)}
                  <div
                    class="relative flex items-center group rounded-md transition-all text-xs {isActiveTable(view.name)
                      ? 'bg-emerald-500/15 text-emerald-200 font-medium border-l-2 border-emerald-400 shadow-[inset_0_0_12px_rgba(52,211,153,0.06)]'
                      : 'text-slate-300 hover:bg-[#1e293b]/70 hover:text-emerald-300 border-l-2 border-transparent'}"
                  >
                    <button
                      type="button"
                      onclick={() => appState.openTableDataTab(view.name)}
                      class="flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                      title="View view {view.name}"
                    >
                      <div class="flex items-center space-x-2 truncate min-w-0 pr-1">
                        <Icon name="eye" class="w-3.5 h-3.5 {isActiveTable(view.name) ? 'text-emerald-400' : 'text-emerald-500 group-hover:text-emerald-400'} shrink-0 transition-colors" />
                        <span class="truncate font-mono text-[11.5px]">{view.name}</span>
                      </div>
                    </button>

                    <!-- Quick action buttons -->
                    <div
                      class="absolute right-1 flex items-center opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/90 hover:bg-[#0f172a] border border-slate-700/60 shadow-sm px-0.5 py-0.5 rounded transition-opacity backdrop-blur-xs"
                    >
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openTableStructureTab(view.name); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-emerald-300 transition"
                        title="View View Structure"
                      >
                        <Icon name="columns" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Routines & Procedures -->
        {#if appState.filteredRoutines.length > 0}
          <div class="rounded overflow-hidden pt-1">
            <div class="flex items-center justify-between hover:bg-slate-800/50 rounded pr-1 group">
              <button
                type="button"
                onclick={() => appState.toggleSection('routines')}
                class="flex-1 px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between transition-colors min-w-0"
              >
                <span class="flex items-center space-x-1.5 truncate">
                  <Icon
                    name={appState.collapsedSections.routines ? 'chevron-right' : 'chevron-down'}
                    class="w-3 h-3 text-slate-500 shrink-0"
                  />
                  <Icon name="code" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
                  <span class="truncate">Routines</span>
                </span>
                <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400 shrink-0 ml-1">
                  {appState.filteredRoutines.length}
                </span>
              </button>

              <button
                type="button"
                onclick={() => appState.openRoutineDesigner()}
                class="p-1 text-slate-400 hover:text-amber-300 hover:bg-amber-950/60 rounded transition shrink-0"
                title="Create Stored Procedure / Function"
              >
                <Icon name="plus" class="w-3.5 h-3.5" />
              </button>
            </div>

            {#if !appState.collapsedSections.routines}
              <div class="space-y-0.5 mt-0.5 pl-0.5">
                {#each appState.filteredRoutines as routine (routine.name)}
                  <button
                    type="button"
                    onclick={() => appState.openRoutineTab(routine.name, (routine.type as any) || 'PROCEDURE')}
                    class="w-full text-left px-2 py-1 rounded-md flex items-center justify-between group hover:bg-[#1e293b]/80 text-slate-300 hover:text-amber-300 text-xs transition-colors"
                    title="View definition for {routine.name}"
                  >
                    <div class="flex items-center space-x-2 truncate min-w-0">
                      <Icon name="code" class="w-3.5 h-3.5 text-amber-500 group-hover:text-amber-400 shrink-0 transition-colors" />
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
            <div class="flex items-center justify-between hover:bg-slate-800/50 rounded pr-1 group">
              <button
                type="button"
                onclick={() => appState.toggleSection('triggers')}
                class="flex-1 px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between transition-colors min-w-0"
              >
                <span class="flex items-center space-x-1.5 truncate">
                  <Icon
                    name={appState.collapsedSections.triggers ? 'chevron-right' : 'chevron-down'}
                    class="w-3 h-3 text-slate-500 shrink-0"
                  />
                  <Icon name="zap" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
                  <span class="truncate">Triggers</span>
                </span>
                <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400 shrink-0 ml-1">
                  {appState.filteredTriggers.length}
                </span>
              </button>

              <button
                type="button"
                onclick={() => appState.openTriggerDesigner()}
                class="p-1 text-slate-400 hover:text-purple-300 hover:bg-purple-950/60 rounded transition shrink-0"
                title="Create Database Trigger"
              >
                <Icon name="plus" class="w-3.5 h-3.5" />
              </button>
            </div>

            {#if !appState.collapsedSections.triggers}
              <div class="space-y-0.5 mt-0.5 pl-0.5">
                {#each appState.filteredTriggers as trigger (trigger.name)}
                  <button
                    type="button"
                    onclick={() => appState.openTriggerTab(trigger)}
                    title="Open trigger definition in a SQL tab"
                    class="w-full text-left px-2 py-1 rounded-md flex items-center justify-between group hover:bg-[#1e293b]/80 text-slate-300 text-xs transition-colors"
                  >
                    <div class="flex items-center space-x-2 truncate min-w-0">
                      <Icon name="zap" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
                      <span class="truncate font-mono text-[11.5px]">{trigger.name}</span>
                    </div>
                    {#if trigger.timing || trigger.event}
                      <span class="text-[9px] text-purple-400 font-mono bg-purple-950/40 px-1 rounded border border-purple-800/40 shrink-0 ml-1">
                        {trigger.timing} {trigger.event}
                      </span>
                    {/if}
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/if}
    </div>

    <!-- Draggable Sidebar Resizer Handle -->
    <div
      id="sidebar-pane-splitter"
      role="separator"
      aria-orientation="vertical"
      aria-label="Sidebar resizer"
      onmousedown={startResize}
      ondblclick={resetWidth}
      title="Drag to resize sidebar, double-click to reset"
      class="absolute right-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-sky-500/60 transition-colors z-20 group {isDragging ? 'bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.8)]' : ''}"
    >
      <div class="w-full h-full"></div>
    </div>

    <!-- Table Context Menu -->
    {#if tableContextMenu && tableContextMenu.open}
      <div
        style="top: {tableContextMenu.y}px; left: {tableContextMenu.x}px;"
        class="fixed z-50 w-56 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
      >
        <!-- Table Header in Menu -->
        <div class="px-3 py-1.5 flex items-center space-x-2 bg-slate-900/60">
          <Icon name="table" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
          <span class="text-[11px] font-bold text-slate-200 font-mono truncate">
            {tableContextMenu.tableName}
          </span>
        </div>

        <!-- Section 1: Views & Structure -->
        <div class="py-1">
          <button
            type="button"
            onclick={() => {
              appState.openTableDataTab(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="table" class="w-3.5 h-3.5 text-emerald-400" />
            <span>Open Data Grid</span>
          </button>
          <button
            type="button"
            onclick={() => {
              appState.openTableStructureTab(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="columns" class="w-3.5 h-3.5 text-sky-400" />
            <span>View Structure</span>
          </button>
          <button
            type="button"
            onclick={() => {
              appState.openTableDDLTab(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="file-code" class="w-3.5 h-3.5 text-purple-400" />
            <span>View CREATE TABLE DDL</span>
          </button>
        </div>

        <!-- Section 2: Management & Export -->
        <div class="py-1">
          <button
            type="button"
            onclick={() => {
              appState.exportTableDump(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="download" class="w-3.5 h-3.5 text-emerald-400" />
            <span>Export Table SQL Dump</span>
          </button>
          <button
            type="button"
            onclick={() => {
              appState.openDumpModal(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="sliders" class="w-3.5 h-3.5 text-sky-400" />
            <span>Export Dump Options...</span>
          </button>
          <button
            type="button"
            onclick={() => {
              appState.mockDataTableName = tableContextMenu!.tableName;
              appState.isMockDataModalOpen = true;
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="sparkles" class="w-3.5 h-3.5 text-purple-400" />
            <span>Generate Mock Data...</span>
          </button>
          <button
            type="button"
            onclick={() => {
              appState.sendToAi(aiTablePrompt(tableContextMenu!), 'Table context sent to AI Copilot');
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="bot" class="w-3.5 h-3.5 text-sky-400" />
            <span>Send Table to AI Copilot</span>
          </button>
          <button
            type="button"
            onclick={() => {
              appState.tableToDuplicate = tableContextMenu!.tableName;
              appState.isDuplicateTableModalOpen = true;
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400" />
            <span>Duplicate Table...</span>
          </button>
          <button
            type="button"
            onclick={() => {
              appState.tableToRename = tableContextMenu!.tableName;
              appState.isRenameTableModalOpen = true;
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="edit" class="w-3.5 h-3.5 text-sky-400" />
            <span>Rename Table...</span>
          </button>
        </div>

        <!-- Section 3: Destructive Actions -->
        <div class="py-1">
          <button
            type="button"
            onclick={() => {
              const tbl = tableContextMenu!.tableName;
              closeTableMenu();
              appState.truncateTable(tbl);
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-amber-950/40 flex items-center space-x-2 text-amber-300 transition-colors"
          >
            <Icon name="alert-triangle" class="w-3.5 h-3.5 text-amber-400" />
            <span>Truncate Table</span>
          </button>
          <button
            type="button"
            onclick={() => {
              const tbl = tableContextMenu!.tableName;
              closeTableMenu();
              appState.dropTable(tbl);
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-rose-950/40 flex items-center space-x-2 text-rose-400 transition-colors"
          >
            <Icon name="trash-2" class="w-3.5 h-3.5 text-rose-400" />
            <span>Drop Table</span>
          </button>
        </div>

        <!-- Section 4: Copy Actions -->
        <div class="py-1">
          <button
            type="button"
            onclick={() => {
              navigator.clipboard.writeText(tableContextMenu!.tableName);
              appState.showToast(`Copied table name "${tableContextMenu!.tableName}"`, 'info');
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="clipboard-list" class="w-3.5 h-3.5 text-slate-400" />
            <span>Copy Table Name</span>
          </button>
          <button
            type="button"
            onclick={() => {
              copySelectStatement(tableContextMenu!.tableName);
              closeTableMenu();
            }}
            class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
          >
            <Icon name="code" class="w-3.5 h-3.5 text-sky-400" />
            <span>Copy SELECT 100 Query</span>
          </button>
        </div>
      </div>
    {/if}
  </aside>
{/if}
