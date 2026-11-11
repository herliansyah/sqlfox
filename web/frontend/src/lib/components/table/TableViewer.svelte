<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import type { TableMode } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import VirtualGrid from '../grid/VirtualGrid.svelte';
  import TableStructure from './TableStructure.svelte';
  import TableIndexes from './TableIndexes.svelte';
  import TableForeignKeys from './TableForeignKeys.svelte';
  import TableDDL from './TableDDL.svelte';
  import TableDiagnostics from './TableDiagnostics.svelte';
  import TablePartitions from './TablePartitions.svelte';

  let currentTab = $derived(appState.activeTab);
  let activeMode = $derived<TableMode>(currentTab?.tableMode || 'data');
  let tableName = $derived(currentTab?.tableName || '');

  let tableMeta = $derived.by(() => {
    if (!tableName || !appState.catalog?.tables) return null;
    return (
      appState.catalog.tables.find(
        (t) => t.name.toLowerCase() === tableName.toLowerCase()
      ) || null
    );
  });

  function setMode(mode: TableMode) {
    if (currentTab) {
      appState.setTableMode(currentTab.id, mode);
    }
  }

  $effect(() => {
    if (currentTab?.isTableViewer && activeMode === 'data' && !currentTab.activeResult && !currentTab.isExecuting) {
      appState.loadTableData(currentTab);
    }
  });

  const subTabs: Array<{ id: TableMode; label: string; icon: string }> = [
    { id: 'data', label: 'Data Grid', icon: 'table' },
    { id: 'structure', label: 'Structure', icon: 'columns' },
    { id: 'indexes', label: 'Indexes', icon: 'key' },
    { id: 'foreign-keys', label: 'Foreign Keys', icon: 'link' },
    { id: 'ddl', label: 'DDL', icon: 'file-code' },
    { id: 'diagnostics', label: 'Diagnostics', icon: 'activity' },
    { id: 'partitions', label: 'Partitions', icon: 'layers' },
  ];
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-none">
  <!-- Sub-Tabs Navigation Bar -->
  <div class="h-8 bg-[#0b0f19] border-b border-[#334155] px-3 flex items-center justify-between text-xs flex-shrink-0">
    <div class="flex items-center space-x-1">
      {#each subTabs as sub}
        {@const isActive = activeMode === sub.id}
        <button
          onclick={() => setMode(sub.id)}
          class="px-2.5 py-1 rounded text-xs transition flex items-center space-x-1.5 {
            isActive
              ? 'bg-[#1e293b] text-sky-400 font-bold border border-[#334155] shadow-sm'
              : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'
          }"
        >
          <Icon name={sub.icon} class="w-3.5 h-3.5 {isActive ? 'text-sky-400' : 'text-slate-500'}" />
          <span>{sub.label}</span>
        </button>
      {/each}
    </div>

    <!-- Active Table Name Pill -->
    <div class="flex items-center space-x-2 font-mono text-[11px] text-slate-400">
      <span class="text-slate-500">{appState.currentDatabase} /</span>
      <span class="text-sky-300 font-bold">{tableName}</span>
      {#if tableMeta && (tableMeta.row_count !== undefined || tableMeta.rows_count !== undefined)}
        <span class="bg-slate-900 border border-slate-800 text-slate-400 px-1.5 py-0.2 rounded text-[10px]">
          {(tableMeta.row_count ?? tableMeta.rows_count ?? 0).toLocaleString()} rows
        </span>
      {/if}
    </div>
  </div>

  <!-- Content Pane -->
  <div class="flex-1 overflow-hidden relative">
    {#if activeMode === 'data'}
      <VirtualGrid />
    {:else if activeMode === 'structure'}
      <TableStructure {tableMeta} />
    {:else if activeMode === 'indexes'}
      <TableIndexes {tableMeta} />
    {:else if activeMode === 'foreign-keys'}
      <TableForeignKeys {tableMeta} />
    {:else if activeMode === 'ddl'}
      <TableDDL {tableName} />
    {:else if activeMode === 'diagnostics'}
      <TableDiagnostics {tableMeta} />
    {:else if activeMode === 'partitions'}
      <TablePartitions {tableName} />
    {/if}
  </div>
</div>
