<script lang="ts">
  import { api } from '../../api/client';
  import { copyText } from '../../utils/clipboard';
  import { quoteSqlIdentifier } from '../../utils/sqlStatements';
  import type { TriggerMetadata } from '../../../types';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
  import { onDestroy, tick } from 'svelte';
  import { viewportFit } from '../../actions/viewportFit';

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
  let showDbActionsMenu = $state(false);
  let dbFilter = $state('');
  let dbActiveIndex = $state(0);
  let dbSearchInput = $state<HTMLInputElement | null>(null);
  let dbMenuList = $state<HTMLDivElement | null>(null);

  $effect(() => {
    if (dbActiveIndex >= filteredDbList.length) dbActiveIndex = Math.max(0, filteredDbList.length - 1);
  });

  async function openDbDropdown() {
    showDbActionsMenu = false;
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
    tableContextMenu = {
      open: true,
      x: e.clientX,
      y: e.clientY,
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
    const qTable = quoteSqlIdentifier(tableName, appState.activeConnection?.driver);
    const sql = `SELECT * FROM ${qTable} LIMIT 100;`;
    copyText(sql);
    appState.showToast(`Copied SELECT query for "${tableName}"`, 'info');
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (showDbDropdown) {
        showDbDropdown = false;
        dbFilter = '';
      }
      if (showDbActionsMenu) {
        showDbActionsMenu = false;
      }
      closeTableMenu();
    }
  }

  function handleSchemaSearchKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      appState.schemaFilter = '';
      (e.currentTarget as HTMLInputElement).blur();
      return;
    }
    if (e.key === 'ArrowDown') {
      const firstBtn = document.querySelector<HTMLButtonElement>('#sidebar-object-tree .schema-tree-item');
      if (firstBtn) {
        e.preventDefault();
        firstBtn.focus();
      }
      return;
    }
    if (e.key === 'Enter') {
      if (appState.filteredTables[0]) {
        e.preventDefault();
        appState.openTableDataTab(appState.filteredTables[0].name);
      } else if (appState.filteredViews[0]) {
        e.preventDefault();
        appState.openTableDataTab(appState.filteredViews[0].name);
      } else if (appState.filteredProcedures[0]) {
        e.preventDefault();
        appState.openRoutineTab(appState.filteredProcedures[0].name, 'PROCEDURE');
      } else if (appState.filteredFunctions[0]) {
        e.preventDefault();
        appState.openRoutineTab(appState.filteredFunctions[0].name, 'FUNCTION');
      } else if (appState.filteredTriggers[0]) {
        e.preventDefault();
        appState.openTriggerTab(appState.filteredTriggers[0]);
      } else if (appState.filteredEvents[0]) {
        e.preventDefault();
        appState.openEventDDLTab(appState.filteredEvents[0].name);
      }
    }
  }

  async function inspectDdlModal(
    kind: 'table' | 'view' | 'procedure' | 'function' | 'trigger' | 'event',
    name: string,
    triggerObj?: TriggerMetadata
  ) {
    if (kind === 'trigger' && triggerObj) {
      const body = triggerObj.statement.trim().replace(/;\s*$/, '');
      const ddl = `CREATE TRIGGER \`${triggerObj.name}\` ${triggerObj.timing} ${triggerObj.event} ON \`${triggerObj.table}\` FOR EACH ROW\n${body};`;
      appState.openDdlViewModal(`TRIGGER: ${triggerObj.name}`, ddl);
      return;
    }
    const sid = appState.activeSessionId;
    if (!sid) return;
    try {
      let ddl = '';
      if (kind === 'procedure') {
        ddl = (await api.getRoutineDDL(sid, name, 'PROCEDURE')).ddl;
      } else if (kind === 'function') {
        ddl = (await api.getRoutineDDL(sid, name, 'FUNCTION')).ddl;
      } else if (kind === 'event') {
        ddl = (await api.getEventDDL(sid, name)).ddl;
      } else {
        ddl = (await api.getTableDDL(sid, name)).ddl;
      }
      appState.openDdlViewModal(`${kind.toUpperCase()}: ${name}`, ddl || '-- No DDL returned');
    } catch (err: any) {
      appState.showToast(`Failed to load DDL for ${name}: ${err.message || err}`, 'error');
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

      <!-- Single 3-Dots Menu for Schema & Database Actions -->
      <div class="relative shrink-0">
        <button
          id="sidebar-db-actions-btn"
          type="button"
          onclick={() => {
            showDbDropdown = false;
            showDbActionsMenu = !showDbActionsMenu;
          }}
          aria-haspopup="menu"
          aria-expanded={showDbActionsMenu}
          class="h-7 w-7 flex items-center justify-center rounded bg-[#0f172a] hover:bg-[#1e293b] border border-slate-700/80 hover:border-slate-600 text-slate-400 hover:text-sky-300 transition-colors shadow-sm cursor-pointer"
          title="Schema & database actions"
        >
          <Icon
            name={appState.isLoadingCatalog ? 'refresh-cw' : 'more-vertical'}
            class="w-3.5 h-3.5 {appState.isLoadingCatalog ? 'animate-spin text-sky-400' : ''}"
          />
        </button>

        {#if showDbActionsMenu}
          <button
            tabindex="-1"
            aria-label="Close"
            class="fixed inset-0 z-40 bg-transparent cursor-default"
            onclick={() => (showDbActionsMenu = false)}
          ></button>

          <div
            role="menu"
            aria-label="Schema and database actions"
            use:viewportFit
            class="absolute right-0 top-full mt-1 w-56 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-50 py-1 text-xs divide-y divide-slate-800/80 animate-in fade-in duration-100"
          >
            {#if appState.currentDatabase && appState.catalogTimestamps[appState.currentDatabase.toLowerCase()]}
              {@const activeStamp = appState.catalogTimestamps[appState.currentDatabase.toLowerCase()]}
              <div
                id="schema-cache-badge"
                class="px-3 py-1.5 flex items-center justify-between text-[11px] text-slate-400"
                title="Cached schema loaded at {activeStamp}"
              >
                <span class="inline-flex items-center gap-1.5 text-amber-400/90 font-medium">
                  <Icon name="zap" class="w-3 h-3 shrink-0" />
                  <span>Schema cache</span>
                </span>
                <span class="font-mono text-[10px] text-amber-300 bg-amber-950/40 border border-amber-800/40 px-1.5 py-0.5 rounded">
                  {activeStamp}
                </span>
              </div>
            {/if}

            <div class="py-1">
              <button
                type="button"
                role="menuitem"
                disabled={appState.isLoadingCatalog || !appState.currentDatabase}
                onclick={() => {
                  showDbActionsMenu = false;
                  appState.refreshCatalog();
                }}
                class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 disabled:opacity-40 flex items-center gap-2 text-slate-200 transition-colors"
              >
                <Icon name="refresh-cw" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                <span>Refresh Active Schema</span>
              </button>
              <button
                type="button"
                role="menuitem"
                disabled={!appState.activeSessionId}
                onclick={() => {
                  showDbActionsMenu = false;
                  appState.refreshDatabases();
                }}
                class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 disabled:opacity-40 flex items-center gap-2 text-slate-200 transition-colors"
              >
                <Icon name="database" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                <span>Reload Database List</span>
              </button>
              <button
                type="button"
                role="menuitem"
                disabled={!appState.activeSessionId}
                onclick={() => {
                  showDbActionsMenu = false;
                  appState.isRefreshAllOpen = true;
                }}
                class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 disabled:opacity-40 flex items-center gap-2 text-slate-200 transition-colors"
              >
                <Icon name="zap" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
                <span>Refresh All Catalogs...</span>
              </button>
            </div>

            <div class="py-1">
              <button
                type="button"
                role="menuitem"
                disabled={!appState.activeSessionId}
                onclick={() => {
                  showDbActionsMenu = false;
                  appState.isCreateDbModalOpen = true;
                }}
                class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 disabled:opacity-40 flex items-center gap-2 text-slate-200 transition-colors"
              >
                <Icon name="plus" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span>Create New Database...</span>
              </button>
              <button
                type="button"
                role="menuitem"
                disabled={!appState.activeSessionId || !appState.currentDatabase}
                onclick={() => {
                  const target = appState.currentDatabase;
                  showDbActionsMenu = false;
                  if (target) appState.dropDatabase(target);
                }}
                class="w-full text-left px-3 py-1.5 hover:bg-rose-950/60 disabled:opacity-40 flex items-center gap-2 text-rose-400 transition-colors"
              >
                <Icon name="trash-2" class="w-3.5 h-3.5 shrink-0" />
                <span>Drop Active Database...</span>
              </button>
            </div>
          </div>
        {/if}
      </div>

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
                {@const stamp = appState.catalogTimestamps[db.toLowerCase()]}
                <div
                  data-db-active={isActiveDb}
                  role="option"
                  tabindex="-1"
                  aria-selected={db === appState.currentDatabase}
                  onmouseenter={() => (dbActiveIndex = idx)}
                  class="relative w-full px-2 py-1 flex items-center justify-between text-xs transition-colors group/dbrow {isActiveDb
                    ? 'bg-slate-800/90 ring-1 ring-inset ring-sky-500/40'
                    : 'hover:bg-slate-800/80'} {db === appState.currentDatabase ? 'bg-sky-500/10 text-sky-400 font-bold' : 'text-slate-300'}"
                >
                  <button
                    type="button"
                    onclick={() => chooseDatabase(db)}
                    class="flex-1 flex items-center space-x-2 truncate min-w-0 text-left py-0.5"
                  >
                    <Icon name="database" class="w-3.5 h-3.5 {db === appState.currentDatabase ? 'text-sky-400' : 'text-slate-500'} shrink-0" />
                    <span class="truncate font-mono">{db}</span>
                  </button>
                  <div class="flex items-center space-x-1 shrink-0 ml-1.5 transition-opacity group-hover/dbrow:opacity-0">
                    {#if stamp}
                      <span class="inline-flex items-center gap-1 text-[9px] text-amber-400/90 font-mono bg-amber-950/30 px-1.5 py-0.5 rounded border border-amber-800/30" title="Cached at {stamp}">
                        <Icon name="zap" class="w-2.5 h-2.5 shrink-0" />
                        <span>{stamp}</span>
                      </span>
                    {/if}
                    {#if appState.databaseTableCounts[db] !== undefined}
                      <span class="text-[9px] text-slate-400 font-mono bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800">
                        {appState.databaseTableCounts[db]} tbls
                      </span>
                    {/if}
                    {#if db === appState.currentDatabase}
                      <Icon name="check" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                    {/if}
                  </div>
                  <div class="absolute right-1.5 flex items-center space-x-0.5 rounded opacity-0 pointer-events-none group-hover/dbrow:opacity-100 group-hover/dbrow:pointer-events-auto bg-[#0f172a]/90 hover:bg-[#0f172a] border border-slate-700/60 shadow-sm px-0.5 py-0.5 transition-all">
                    <button
                      type="button"
                      onclick={(e) => {
                        e.stopPropagation();
                        appState.refreshSingleDatabase(db);
                      }}
                      class="p-1 rounded text-slate-400 hover:text-sky-300 hover:bg-slate-700/80 transition"
                      title="Refresh catalog for {db}"
                    >
                      <Icon name="refresh-cw" class="w-3 h-3" />
                    </button>
                    <button
                      type="button"
                      onclick={(e) => {
                        e.stopPropagation();
                        closeDbDropdown();
                        appState.dropDatabase(db);
                      }}
                      class="p-1 rounded text-slate-400 hover:text-rose-400 hover:bg-rose-950/60 transition"
                      title="Drop database '{db}'"
                    >
                      <Icon name="trash-2" class="w-3 h-3" />
                    </button>
                  </div>
                </div>
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

    <!-- Schema Filter Header (Height h-9: 36px - aligned with Content Toolbar) -->
    <div class="h-9 px-2 border-b border-slate-800 bg-[#0b0f19] flex items-center relative shrink-0">
      <!-- Search Filter Input -->
      <div class="relative flex-1 flex items-center min-w-0">
        <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2 pointer-events-none" />
        <input
          id="schema-search-input"
          type="text"
          bind:value={appState.schemaFilter}
          onkeydown={handleSchemaSearchKeydown}
          placeholder="Filter schema... (/)"
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
    </div>

    <!-- Content: Schema Objects Mode -->
    <div id="sidebar-object-tree" class="flex-1 overflow-y-auto p-1.5 space-y-1">
      {#if appState.isLoadingCatalog}
        <div class="p-8 text-center text-xs text-slate-400 flex flex-col items-center gap-2">
          <Icon name="refresh-cw" class="w-5 h-5 animate-spin text-sky-400" />
          <span class="font-medium text-slate-300">Loading catalog...</span>
          <span class="text-[10px] text-slate-500 font-mono truncate max-w-full px-2">{appState.currentDatabase}</span>
        </div>
      {:else if !appState.catalog}
        <div class="p-8 text-center text-xs text-slate-500 flex flex-col items-center gap-3">
          <Icon name="database" class="w-8 h-8 text-slate-600" />
          <div class="space-y-1">
            <span class="font-medium text-slate-300 block">Database "{appState.currentDatabase || 'None'}" has no catalog loaded</span>
            <span class="text-[11px] text-slate-500 block">Select a database or refresh schema</span>
          </div>
          <button
            type="button"
            onclick={() => openDbDropdown()}
            class="w-full px-3 py-1.5 bg-[#0f172a] hover:bg-slate-800 text-slate-300 border border-slate-700 rounded text-xs font-medium transition flex items-center justify-center space-x-1.5 shadow-sm"
          >
            <Icon name="database" class="w-3.5 h-3.5 text-sky-400" />
            <span>Browse All {appState.availableDatabases.length} Databases</span>
          </button>
        </div>
      {:else}
        {@const totalTables = appState.catalog.tables?.length || 0}
        {@const totalViews = appState.catalog.views?.length || 0}
        {@const totalTriggers = appState.catalog.triggers?.length || 0}
        {@const totalEvents = appState.catalog.events?.length || 0}
        {@const driverLower = appState.activeConnection?.driver?.toLowerCase() || ''}
        {@const supportsEvents = !driverLower || driverLower.includes('mysql') || driverLower.includes('maria')}
        {@const hasFilteredMatch =
          appState.filteredTables.length > 0 ||
          appState.filteredViews.length > 0 ||
          appState.filteredProcedures.length > 0 ||
          appState.filteredFunctions.length > 0 ||
          appState.filteredTriggers.length > 0 ||
          appState.filteredEvents.length > 0}

        {#if appState.schemaFilter && !hasFilteredMatch}
          <div class="p-6 text-center text-xs text-slate-500 space-y-1.5">
            <p class="italic">No schema objects matching "{appState.schemaFilter}"</p>
            <button
              type="button"
              onclick={() => (appState.schemaFilter = '')}
              class="text-sky-400 hover:underline text-xs"
            >
              Clear filter
            </button>
          </div>
        {:else if !appState.schemaFilter && totalTables === 0 && totalViews === 0 && !appState.catalog.routines?.length && totalTriggers === 0 && totalEvents === 0}
          <div class="mx-1 mb-1 px-2.5 py-2 rounded border border-slate-800/80 bg-[#0f172a]/70 text-[11px] text-slate-400 flex items-center justify-between gap-2">
            <span class="truncate">Database is empty</span>
            <button
              type="button"
              onclick={() => (appState.isTableCreatorOpen = true)}
              class="px-2 py-0.5 bg-sky-950/80 hover:bg-sky-900 border border-sky-800 text-sky-300 rounded text-[10px] font-semibold shrink-0 transition"
            >
              + Create Table
            </button>
          </div>
        {/if}

        <!-- SECTION: Tables -->
        {#if appState.filteredTables.length > 0 || !appState.schemaFilter}
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
                  {#if appState.schemaFilter && totalTables > 0}
                    {appState.filteredTables.length}/{totalTables}
                  {:else}
                    {appState.filteredTables.length}
                  {/if}
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
                                    title="{table.row_count_exact ? 'Exact rows' : 'Estimated rows'}: {(table.row_count ?? table.rows_count).toLocaleString()}"
                                  >
                                    {table.row_count_exact ? '' : '~'}{formatCount(table.row_count ?? table.rows_count)}
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
                        class="schema-tree-item flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
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
                            title="{table.row_count_exact ? 'Exact rows' : 'Estimated rows'}: {(table.row_count ?? table.rows_count).toLocaleString()}"
                          >
                            {table.row_count_exact ? '' : '~'}{formatCount(table.row_count ?? table.rows_count)}
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
                        class="schema-tree-item flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
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
                            title="{table.row_count_exact ? 'Exact rows' : 'Estimated rows'}: {(table.row_count ?? table.rows_count).toLocaleString()}"
                          >
                            {table.row_count_exact ? '' : '~'}{formatCount(table.row_count ?? table.rows_count)}
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
        {#if appState.filteredViews.length > 0 || !appState.schemaFilter}
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
                      class="schema-tree-item flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                      title="Browse view {view.name}"
                    >
                      <div class="flex items-center space-x-2 truncate min-w-0 pr-1">
                        <Icon name="eye" class="w-3.5 h-3.5 {isActiveTable(view.name) ? 'text-emerald-400' : 'text-emerald-500 group-hover:text-emerald-400'} shrink-0 transition-colors" />
                        <span class="truncate font-mono text-[11.5px]">{view.name}</span>
                      </div>
                    </button>

                    <!-- Quick action buttons -->
                    <div
                      class="absolute right-1 flex items-center space-x-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/95 border border-slate-700/60 shadow-sm px-0.5 py-0.5 rounded transition-opacity backdrop-blur-xs"
                    >
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openTableStructureTab(view.name); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-emerald-300 transition"
                        title="View Structure"
                      >
                        <Icon name="columns" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); inspectDdlModal('view', view.name); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                        title="View DDL"
                      >
                        <Icon name="file-code" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openViewDesigner({ name: view.name, query: view.definition }); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-amber-300 transition"
                        title="Edit View"
                      >
                        <Icon name="edit" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.dropView(view.name); }}
                        class="p-1 hover:bg-rose-950/70 rounded text-slate-400 hover:text-rose-400 transition"
                        title="Drop View"
                      >
                        <Icon name="trash-2" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Stored Procedures -->
        {#if appState.filteredProcedures.length > 0 || !appState.schemaFilter}
          <div class="rounded overflow-hidden pt-1">
            <div class="flex items-center justify-between hover:bg-slate-800/50 rounded pr-1 group">
              <button
                type="button"
                onclick={() => appState.toggleSection('procedures')}
                class="flex-1 px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between transition-colors min-w-0"
              >
                <span class="flex items-center space-x-1.5 truncate">
                  <Icon
                    name={appState.collapsedSections.procedures ? 'chevron-right' : 'chevron-down'}
                    class="w-3 h-3 text-slate-500 shrink-0"
                  />
                  <Icon name="terminal" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
                  <span class="truncate">Procedures</span>
                </span>
                <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400 shrink-0 ml-1">
                  {appState.filteredProcedures.length}
                </span>
              </button>

              <button
                type="button"
                onclick={() => appState.openRoutineDesigner({ name: '', type: 'PROCEDURE', definition: '' })}
                class="p-1 text-slate-400 hover:text-amber-300 hover:bg-amber-950/60 rounded transition shrink-0"
                title="Create Stored Procedure"
              >
                <Icon name="plus" class="w-3.5 h-3.5" />
              </button>
            </div>

            {#if !appState.collapsedSections.procedures}
              <div class="space-y-0.5 mt-0.5 pl-0.5">
                {#each appState.filteredProcedures as proc (proc.name)}
                  <div class="relative flex items-center group rounded-md text-xs text-slate-300 hover:bg-[#1e293b]/80 hover:text-amber-300 transition-colors">
                    <button
                      type="button"
                      onclick={() => appState.openRoutineTab(proc.name, 'PROCEDURE')}
                      class="schema-tree-item flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                      title="Open procedure {proc.name} DDL in editor"
                    >
                      <div class="flex items-center space-x-2 truncate min-w-0">
                        <Icon name="code" class="w-3.5 h-3.5 text-amber-500 group-hover:text-amber-400 shrink-0 transition-colors" />
                        <span class="truncate font-mono text-[11.5px]">{proc.name}</span>
                      </div>
                    </button>
                    <div
                      class="absolute right-1 flex items-center space-x-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/95 border border-slate-700/60 shadow-sm px-0.5 py-0.5 rounded transition-opacity backdrop-blur-xs"
                    >
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); inspectDdlModal('procedure', proc.name); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                        title="View DDL"
                      >
                        <Icon name="file-code" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openRoutineCaller(proc); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-emerald-300 transition"
                        title="Call in Editor"
                      >
                        <Icon name="play" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openRoutineDesigner(proc); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-amber-300 transition"
                        title="Edit Procedure"
                      >
                        <Icon name="edit" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.dropRoutine(proc.name, 'PROCEDURE'); }}
                        class="p-1 hover:bg-rose-950/70 rounded text-slate-400 hover:text-rose-400 transition"
                        title="Drop Procedure"
                      >
                        <Icon name="trash-2" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Functions -->
        {#if appState.filteredFunctions.length > 0 || !appState.schemaFilter}
          <div class="rounded overflow-hidden pt-1">
            <div class="flex items-center justify-between hover:bg-slate-800/50 rounded pr-1 group">
              <button
                type="button"
                onclick={() => appState.toggleSection('functions')}
                class="flex-1 px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between transition-colors min-w-0"
              >
                <span class="flex items-center space-x-1.5 truncate">
                  <Icon
                    name={appState.collapsedSections.functions ? 'chevron-right' : 'chevron-down'}
                    class="w-3 h-3 text-slate-500 shrink-0"
                  />
                  <Icon name="code" class="w-3.5 h-3.5 text-cyan-400 shrink-0" />
                  <span class="truncate">Functions</span>
                </span>
                <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400 shrink-0 ml-1">
                  {appState.filteredFunctions.length}
                </span>
              </button>

              <button
                type="button"
                onclick={() => appState.openRoutineDesigner({ name: '', type: 'FUNCTION', definition: '' })}
                class="p-1 text-slate-400 hover:text-cyan-300 hover:bg-cyan-950/60 rounded transition shrink-0"
                title="Create Stored Function"
              >
                <Icon name="plus" class="w-3.5 h-3.5" />
              </button>
            </div>

            {#if !appState.collapsedSections.functions}
              <div class="space-y-0.5 mt-0.5 pl-0.5">
                {#each appState.filteredFunctions as fn (fn.name)}
                  <div class="relative flex items-center group rounded-md text-xs text-slate-300 hover:bg-[#1e293b]/80 hover:text-cyan-300 transition-colors">
                    <button
                      type="button"
                      onclick={() => appState.openRoutineTab(fn.name, 'FUNCTION')}
                      class="schema-tree-item flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                      title="Open function {fn.name} DDL in editor"
                    >
                      <div class="flex items-center space-x-2 truncate min-w-0">
                        <Icon name="code" class="w-3.5 h-3.5 text-cyan-500 group-hover:text-cyan-400 shrink-0 transition-colors" />
                        <span class="truncate font-mono text-[11.5px]">{fn.name}</span>
                      </div>
                    </button>
                    <div
                      class="absolute right-1 flex items-center space-x-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/95 border border-slate-700/60 shadow-sm px-0.5 py-0.5 rounded transition-opacity backdrop-blur-xs"
                    >
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); inspectDdlModal('function', fn.name); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                        title="View DDL"
                      >
                        <Icon name="file-code" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openRoutineCaller(fn); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-emerald-300 transition"
                        title="Call in Editor"
                      >
                        <Icon name="play" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openRoutineDesigner(fn); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-amber-300 transition"
                        title="Edit Function"
                      >
                        <Icon name="edit" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.dropRoutine(fn.name, 'FUNCTION'); }}
                        class="p-1 hover:bg-rose-950/70 rounded text-slate-400 hover:text-rose-400 transition"
                        title="Drop Function"
                      >
                        <Icon name="trash-2" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Triggers -->
        {#if appState.filteredTriggers.length > 0 || !appState.schemaFilter}
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
                  <div class="relative flex items-center group rounded-md text-xs text-slate-300 hover:bg-[#1e293b]/80 hover:text-purple-200 transition-colors">
                    <button
                      type="button"
                      onclick={() => appState.openTriggerTab(trigger)}
                      title="Open trigger definition in a SQL tab"
                      class="schema-tree-item flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                    >
                      <div class="flex items-center space-x-2 truncate min-w-0">
                        <Icon name="zap" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
                        <span class="truncate font-mono text-[11.5px]">{trigger.name}</span>
                      </div>
                      {#if trigger.timing || trigger.event}
                        <span class="text-[9px] text-purple-400 font-mono bg-purple-950/40 px-1 rounded border border-purple-800/40 shrink-0 ml-1 group-hover:opacity-0 transition-opacity">
                          {trigger.timing} {trigger.event}
                        </span>
                      {/if}
                    </button>
                    <div
                      class="absolute right-1 flex items-center space-x-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/95 border border-slate-700/60 shadow-sm px-0.5 py-0.5 rounded transition-opacity backdrop-blur-xs"
                    >
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); inspectDdlModal('trigger', trigger.name, trigger); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                        title="View DDL"
                      >
                        <Icon name="file-code" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openTriggerDesigner(trigger); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-amber-300 transition"
                        title="Edit Trigger"
                      >
                        <Icon name="edit" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.dropTrigger(trigger.name, trigger.table); }}
                        class="p-1 hover:bg-rose-950/70 rounded text-slate-400 hover:text-rose-400 transition"
                        title="Drop Trigger"
                      >
                        <Icon name="trash-2" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        <!-- SECTION: Scheduled Events -->
        {#if (supportsEvents || appState.filteredEvents.length > 0) && (appState.filteredEvents.length > 0 || !appState.schemaFilter)}
          <div class="rounded overflow-hidden pt-1">
            <div class="flex items-center justify-between hover:bg-slate-800/50 rounded pr-1 group">
              <button
                type="button"
                onclick={() => appState.toggleSection('events')}
                class="flex-1 px-2 py-1 text-[10.5px] font-bold text-slate-400 uppercase tracking-wider flex items-center justify-between transition-colors min-w-0"
              >
                <span class="flex items-center space-x-1.5 truncate">
                  <Icon
                    name={appState.collapsedSections.events ? 'chevron-right' : 'chevron-down'}
                    class="w-3 h-3 text-slate-500 shrink-0"
                  />
                  <Icon name="clock" class="w-3.5 h-3.5 text-pink-400 shrink-0" />
                  <span class="truncate">Events</span>
                </span>
                <span class="font-mono text-[10px] bg-slate-900 px-1.5 py-0.2 rounded border border-slate-800 text-slate-400 shrink-0 ml-1">
                  {appState.filteredEvents.length}
                </span>
              </button>

              <button
                type="button"
                onclick={() => appState.openEventDesigner()}
                class="p-1 text-slate-400 hover:text-pink-300 hover:bg-pink-950/60 rounded transition shrink-0"
                title="Create Scheduled Event"
              >
                <Icon name="plus" class="w-3.5 h-3.5" />
              </button>
            </div>

            {#if !appState.collapsedSections.events}
              <div class="space-y-0.5 mt-0.5 pl-0.5">
                {#each appState.filteredEvents as ev (ev.name)}
                  {@const isEnabled = (ev.status || '').toUpperCase().includes('ENABLED')}
                  {@const evSched = ev.interval || ev.schedule || ''}
                  <div class="relative flex items-center group rounded-md text-xs text-slate-300 hover:bg-[#1e293b]/80 hover:text-pink-200 transition-colors">
                    <button
                      type="button"
                      onclick={() => appState.openEventDDLTab(ev.name)}
                      title="Event {ev.name} ({ev.status || 'UNKNOWN'} — {evSched})"
                      class="schema-tree-item flex-1 text-left px-2 py-1 flex items-center justify-between truncate min-w-0"
                    >
                      <div class="flex items-center space-x-2 truncate min-w-0">
                        <span
                          class="w-1.5 h-1.5 rounded-full shrink-0 {isEnabled ? 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.7)]' : 'bg-slate-500'}"
                          title={ev.status || 'DISABLED'}
                        ></span>
                        <Icon name="clock" class="w-3.5 h-3.5 text-pink-400 shrink-0" />
                        <span class="truncate font-mono text-[11.5px]">{ev.name}</span>
                      </div>
                      {#if evSched}
                        <span class="text-[9px] text-pink-300 font-mono bg-pink-950/40 px-1 rounded border border-pink-800/40 shrink-0 ml-1 max-w-[76px] truncate group-hover:opacity-0 transition-opacity">
                          {evSched}
                        </span>
                      {/if}
                    </button>
                    <div
                      class="absolute right-1 flex items-center space-x-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto bg-[#0f172a]/95 border border-slate-700/60 shadow-sm px-0.5 py-0.5 rounded transition-opacity backdrop-blur-xs"
                    >
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); inspectDdlModal('event', ev.name); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-sky-300 transition"
                        title="View DDL"
                      >
                        <Icon name="file-code" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.toggleEventStatus(ev); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-emerald-300 transition"
                        title={isEnabled ? 'Disable Event' : 'Enable Event'}
                      >
                        <Icon name={isEnabled ? 'pause' : 'play'} class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.openEventDesigner(ev); }}
                        class="p-1 hover:bg-slate-700/80 rounded text-slate-400 hover:text-amber-300 transition"
                        title="Edit Event"
                      >
                        <Icon name="edit" class="w-3 h-3" />
                      </button>
                      <button
                        type="button"
                        onclick={(e) => { e.stopPropagation(); appState.dropEvent(ev.name); }}
                        class="p-1 hover:bg-rose-950/70 rounded text-slate-400 hover:text-rose-400 transition"
                        title="Drop Event"
                      >
                        <Icon name="trash-2" class="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/if}
    </div>

    <!-- Draggable Sidebar Resizer Handle -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
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
        use:viewportFit={{ x: tableContextMenu.x, y: tableContextMenu.y }}
        style="top: {tableContextMenu.y}px; left: {tableContextMenu.x}px;"
        class="fixed z-50 w-60 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
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
          <div class="px-3 py-1 flex items-center justify-between gap-1.5">
            <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
              <Icon name="file-code" class="w-3.5 h-3.5 text-purple-400" />
              <span>DDL</span>
            </span>
            <div class="flex items-center gap-1 text-[10px]">
              <button
                type="button"
                onclick={() => {
                  appState.openTableDDLTab(tableContextMenu!.tableName);
                  closeTableMenu();
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-purple-300 font-medium transition-colors"
                title="Open CREATE TABLE in Tab"
              >Tab</button>
              <button
                type="button"
                onclick={() => {
                  const tbl = tableContextMenu!.tableName;
                  closeTableMenu();
                  inspectDdlModal('table', tbl);
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
                title="View DDL in Modal"
              >Modal</button>
            </div>
          </div>
        </div>

        <!-- Section 2: Tools & Export -->
        <div class="py-1">
          <div class="px-3 py-1 flex items-center justify-between gap-1.5">
            <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
              <Icon name="download" class="w-3.5 h-3.5 text-emerald-400" />
              <span>SQL Dump</span>
            </span>
            <div class="flex items-center gap-1 text-[10px]">
              <button
                type="button"
                onclick={() => {
                  appState.exportTableDump(tableContextMenu!.tableName);
                  closeTableMenu();
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium transition-colors"
                title="Export Table SQL Dump"
              >Quick</button>
              <button
                type="button"
                onclick={() => {
                  appState.openDumpModal(tableContextMenu!.tableName);
                  closeTableMenu();
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
                title="Export Dump Options..."
              >Options...</button>
            </div>
          </div>
          <button
            type="button"
            onclick={() => {
              const tbl = tableContextMenu!.tableName;
              closeTableMenu();
              appState.openMockDataModal(tbl);
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
          <div class="px-3 py-1 flex items-center justify-between gap-1.5">
            <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
              <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400" />
              <span>Manage</span>
            </span>
            <div class="flex items-center gap-1 text-[10px]">
              <button
                type="button"
                onclick={() => {
                  appState.tableToDuplicate = tableContextMenu!.tableName;
                  appState.isDuplicateTableModalOpen = true;
                  closeTableMenu();
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium transition-colors"
              >Duplicate...</button>
              <button
                type="button"
                onclick={() => {
                  appState.tableToRename = tableContextMenu!.tableName;
                  appState.isRenameTableModalOpen = true;
                  closeTableMenu();
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
              >Rename...</button>
            </div>
          </div>
        </div>

        <!-- Section 3: Copy & Destructive Actions -->
        <div class="py-1 space-y-0.5">
          <div class="px-3 py-1 flex items-center justify-between gap-1.5">
            <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
              <Icon name="clipboard-list" class="w-3.5 h-3.5 text-slate-400" />
              <span>Copy</span>
            </span>
            <div class="flex items-center gap-1 text-[10px]">
              <button
                type="button"
                onclick={() => {
                  copyText(tableContextMenu!.tableName);
                  appState.showToast(`Copied table name "${tableContextMenu!.tableName}"`, 'info');
                  closeTableMenu();
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 font-medium transition-colors"
              >Name</button>
              <button
                type="button"
                onclick={() => {
                  copySelectStatement(tableContextMenu!.tableName);
                  closeTableMenu();
                }}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
              >SELECT 100</button>
            </div>
          </div>
          <div class="px-3 py-1 flex items-center justify-between gap-1.5">
            <span class="text-[11px] text-rose-400/90 flex items-center gap-1.5">
              <Icon name="alert-triangle" class="w-3.5 h-3.5 text-amber-400" />
              <span>Danger</span>
            </span>
            <div class="flex items-center gap-1 text-[10px]">
              <button
                type="button"
                onclick={() => {
                  const tbl = tableContextMenu!.tableName;
                  closeTableMenu();
                  appState.truncateTable(tbl);
                }}
                class="px-2 py-0.5 rounded bg-amber-950/60 hover:bg-amber-900/70 text-amber-300 font-medium transition-colors"
              >Truncate</button>
              <button
                type="button"
                onclick={() => {
                  const tbl = tableContextMenu!.tableName;
                  closeTableMenu();
                  appState.dropTable(tbl);
                }}
                class="px-2 py-0.5 rounded bg-rose-950/60 hover:bg-rose-900/70 text-rose-300 font-medium transition-colors"
              >Drop</button>
            </div>
          </div>
        </div>
      </div>
    {/if}
  </aside>
{/if}
