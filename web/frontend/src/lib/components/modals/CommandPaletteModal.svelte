<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let searchInput: HTMLInputElement | null = $state(null);
  let query = $state('');
  let selectedIndex = $state(0);

  interface CommandItem {
    id: string;
    title: string;
    subtitle?: string;
    category: 'Tables' | 'Databases' | 'Actions';
    icon: string;
    action: () => void;
  }

  let items = $derived.by<CommandItem[]>(() => {
    const list: CommandItem[] = [];

    // 1. Actions
    list.push({
      id: 'action-new-query',
      title: 'New SQL Query Tab',
      subtitle: 'Create a blank SQL worksheet',
      category: 'Actions',
      icon: 'zap',
      action: () => appState.addTab(),
    });
    list.push({
      id: 'action-processlist',
      title: 'Open Processlist & DBA Telemetry',
      subtitle: 'Inspect running queries and threads',
      category: 'Actions',
      icon: 'cpu',
      action: () => appState.openProcesslistTab(),
    });
    list.push({
      id: 'action-erd-canvas',
      title: 'Open ERD Schema Canvas',
      subtitle: 'Interactive visual entity-relationship diagrams and schema export',
      category: 'Actions',
      icon: 'layers',
      action: () => appState.openErdTab(),
    });
    list.push({
      id: 'action-create-table',
      title: 'Create Table Designer',
      subtitle: 'Visual designer for new database tables and columns',
      category: 'Actions',
      icon: 'plus',
      action: () => (appState.isTableCreatorOpen = true),
    });
    list.push({
      id: 'action-create-view',
      title: 'Create Database View',
      subtitle: 'Visual query builder and view DDL designer',
      category: 'Actions',
      icon: 'eye',
      action: () => appState.openViewDesigner(),
    });
    list.push({
      id: 'action-create-trigger',
      title: 'Create Database Trigger',
      subtitle: 'Event listener trigger designer for tables',
      category: 'Actions',
      icon: 'zap',
      action: () => appState.openTriggerDesigner(),
    });
    list.push({
      id: 'action-create-routine',
      title: 'Create Stored Procedure / Function',
      subtitle: 'Programmability routine designer with parameters',
      category: 'Actions',
      icon: 'code',
      action: () => appState.openRoutineDesigner(),
    });
    list.push({
      id: 'action-create-event',
      title: 'Create Scheduled Event',
      subtitle: 'Cron-like scheduled event designer for MySQL/MariaDB',
      category: 'Actions',
      icon: 'clock',
      action: () => appState.openEventDesigner(),
    });
    list.push({
      id: 'action-dump-db',
      title: 'Export SQL Dump',
      subtitle: 'Download complete SQL backup of database or table',
      category: 'Actions',
      icon: 'download',
      action: () => appState.openDumpModal(),
    });
    list.push({
      id: 'action-structure-sync',
      title: 'Structure Synchronization Engine',
      subtitle: 'Compare schemas between environments and generate ALTER scripts',
      category: 'Actions',
      icon: 'refresh-cw',
      action: () => appState.openStructureSyncModal(),
    });
    list.push({
      id: 'action-run-sql',
      title: 'Run Large SQL Script File',
      subtitle: 'Execute multi-statement SQL migration files and dumps',
      category: 'Actions',
      icon: 'file-text',
      action: () => appState.openRunSqlModal(),
    });
    list.push({
      id: 'action-snippets',
      title: 'Query Snippets & Templates Library',
      subtitle: 'Reusable parameterized SQL query templates',
      category: 'Actions',
      icon: 'bookmark',
      action: () => appState.openSnippetsModal(),
    });
    list.push({
      id: 'action-csv-import',
      title: 'Import CSV Records',
      subtitle: 'Upload delimited CSV file and map columns to table',
      category: 'Actions',
      icon: 'upload',
      action: () => appState.openCsvImportModal(),
    });
    list.push({
      id: 'action-db-diagnostics',
      title: 'Database Storage & Health Diagnostics',
      subtitle: 'Inspect table fragmentation and run OPTIMIZE/ANALYZE',
      category: 'Actions',
      icon: 'activity',
      action: () => appState.openDbDiagnosticsModal(),
    });
    list.push({
      id: 'action-privilege-matrix',
      title: 'Database Users & Privilege Matrix',
      subtitle: 'Manage DB user permissions and GRANT/REVOKE matrix',
      category: 'Actions',
      icon: 'shield',
      action: () => appState.openPrivilegeMatrixModal(),
    });
    list.push({
      id: 'action-new-conn',
      title: 'Manage Database Connections',
      subtitle: 'Add, edit, or test database connections',
      category: 'Actions',
      icon: 'database',
      action: () => appState.openNewConnectionModal(),
    });
    list.push({
      id: 'action-refresh-schema',
      title: 'Refresh Schema Catalog',
      subtitle: 'Reload tables and databases from server',
      category: 'Actions',
      icon: 'refresh-cw',
      action: () => appState.refreshCatalog(),
    });

    // 2. Tables
    if (appState.catalog?.tables) {
      for (const t of appState.catalog.tables) {
        list.push({
          id: `tbl-${t.name}`,
          title: t.name,
          subtitle: `${t.type} • ${t.columns?.length || 0} columns`,
          category: 'Tables',
          icon: 'table',
          action: () => appState.openTableDataTab(t.name),
        });
      }
    }

    // 3. Databases
    for (const db of appState.availableDatabases || []) {
      list.push({
        id: `db-${db}`,
        title: db,
        subtitle: 'Switch active database',
        category: 'Databases',
        icon: 'database',
        action: () => appState.selectDatabase(db),
      });
    }

    const q = query.toLowerCase().trim();
    if (!q) return list;
    return list.filter(
      (item) => item.title.toLowerCase().includes(q) || item.subtitle?.toLowerCase().includes(q)
    );
  });

  $effect(() => {
    if (appState.isCommandPaletteOpen) {
      query = '';
      selectedIndex = 0;
      setTimeout(() => searchInput?.focus(), 50);
    }
  });

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIndex = Math.min(items.length - 1, selectedIndex + 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIndex = Math.max(0, selectedIndex - 1);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (items[selectedIndex]) {
        items[selectedIndex].action();
        appState.isCommandPaletteOpen = false;
      }
    } else if (e.key === 'Escape') {
      appState.isCommandPaletteOpen = false;
    }
  }
</script>

{#if appState.isCommandPaletteOpen}
  <div
    class="fixed inset-0 z-50 flex items-start justify-center pt-24 p-4 bg-black/75 backdrop-blur-sm select-none"
    onclick={(e) => e.target === e.currentTarget && (appState.isCommandPaletteOpen = false)}
    role="dialog"
    tabindex="-1"
    onkeydown={handleKeyDown}
  >
    <div class="w-full max-w-xl bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col">
      <!-- Search Bar -->
      <div class="p-3 border-b border-[#334155] flex items-center space-x-2.5 bg-[#0b0f19]">
        <Icon name="search" class="w-4 h-4 text-sky-400 shrink-0" />
        <input
          bind:this={searchInput}
          type="text"
          bind:value={query}
          placeholder="Type a table name, database, or action..."
          class="flex-1 bg-transparent text-sm text-slate-100 placeholder-slate-500 focus:outline-none font-sans"
        />
        <kbd class="text-[10px] bg-slate-900 border border-slate-700 text-slate-400 px-1.5 py-0.5 rounded font-mono">
          ESC
        </kbd>
      </div>

      <!-- Results list -->
      <div class="max-h-80 overflow-y-auto p-1 divide-y divide-slate-800/40">
        {#if items.length === 0}
          <div class="p-8 text-center text-xs text-slate-500 italic">
            No matching tables or actions found
          </div>
        {:else}
          {#each items as item, idx}
            <button
              onclick={() => {
                item.action();
                appState.isCommandPaletteOpen = false;
              }}
              class="w-full px-3 py-2 text-left flex items-center justify-between text-xs rounded transition-colors {
                idx === selectedIndex ? 'bg-sky-500/15 text-sky-300 font-semibold' : 'text-slate-300 hover:bg-slate-800/60'
              }"
            >
              <div class="flex items-center space-x-2.5 truncate">
                <Icon name={item.icon} class="w-4 h-4 text-sky-400 shrink-0" />
                <div class="truncate">
                  <div class="font-mono text-[12px] truncate">{item.title}</div>
                  {#if item.subtitle}
                    <div class="text-[10px] text-slate-500">{item.subtitle}</div>
                  {/if}
                </div>
              </div>

              <span class="text-[9px] uppercase font-mono px-1.5 py-0.2 rounded bg-slate-900 border border-slate-800 text-slate-400 shrink-0 ml-2">
                {item.category}
              </span>
            </button>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}
