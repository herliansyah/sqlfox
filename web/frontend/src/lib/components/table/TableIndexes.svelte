<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { TableMetadata, ColumnMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableMeta: TableMetadata | null;
  }

  let { tableMeta }: Props = $props();

  let isAddModalOpen = $state(false);
  let isExecuting = $state(false);

  // Index Form
  let indexName = $state('');
  let indexType = $state<'INDEX' | 'UNIQUE' | 'FULLTEXT' | 'SPATIAL'>('INDEX');
  let selectedCols = $state<string[]>([]);

  let availableCols = $derived(
    (tableMeta?.columns || []).map((c: ColumnMetadata) => c.name)
  );

  function resetForm() {
    indexName = '';
    indexType = 'INDEX';
    selectedCols = [];
  }

  function openAddIndex() {
    resetForm();
    isAddModalOpen = true;
  }

  function toggleColumn(colName: string) {
    if (selectedCols.includes(colName)) {
      selectedCols = selectedCols.filter((c) => c !== colName);
    } else {
      selectedCols = [...selectedCols, colName];
    }
    autoGenerateIndexName();
  }

  function autoGenerateIndexName() {
    if (!tableMeta) return;
    const prefix =
      indexType === 'UNIQUE'
        ? 'uniq'
        : indexType === 'FULLTEXT'
        ? 'ft'
        : indexType === 'SPATIAL'
        ? 'sp'
        : 'idx';
    const colSlug = selectedCols.slice(0, 3).join('_') || 'cols';
    indexName = `${prefix}_${tableMeta.name}_${colSlug}`;
  }

  let sqlPreview = $derived.by(() => {
    if (!tableMeta || !appState.activeConnection) return '';
    const driver = appState.activeConnection.driver;
    const isSQLite = driver === 'sqlite';
    const isPostgres = driver === 'postgres';
    const q = (s: string) => (isSQLite || isPostgres ? `"${s}"` : `\`${s}\``);

    const name = indexName.trim() || 'idx_name';
    const typeClause =
      indexType === 'UNIQUE'
        ? 'UNIQUE '
        : indexType === 'FULLTEXT'
        ? 'FULLTEXT '
        : indexType === 'SPATIAL'
        ? 'SPATIAL '
        : '';

    if (selectedCols.length === 0) {
      return '-- Select column(s) to generate index SQL definition';
    }

    const colsFormatted = selectedCols.map((c) => q(c)).join(', ');
    return `CREATE ${typeClause}INDEX ${q(name)} ON ${q(tableMeta.name)} (${colsFormatted});`;
  });

  async function createIndex() {
    if (!appState.activeConnection || !tableMeta) return;
    if (selectedCols.length === 0) {
      appState.showToast('Please select at least one column for the index', 'warning');
      return;
    }
    if (!indexName.trim()) {
      appState.showToast('Index name cannot be empty', 'warning');
      return;
    }

    isExecuting = true;
    try {
      const connId = appState.activeConnection.id;
      await api.executeQuery(connId, sqlPreview, appState.currentDatabase || undefined);
      appState.showToast(`Index "${indexName.trim()}" created successfully!`, 'success');
      isAddModalOpen = false;
      resetForm();

      // Refresh catalog
      await appState.loadCatalog(connId, appState.currentDatabase || undefined);
    } catch (err: any) {
      appState.showToast(`Failed to create index: ${err.message}`, 'error');
    } finally {
      isExecuting = false;
    }
  }

  async function dropIndex(idxName: string) {
    if (!appState.activeConnection || !tableMeta) return;
    const confirmed = confirm(`Are you sure you want to drop index "${idxName}" from table "${tableMeta.name}"?`);
    if (!confirmed) return;

    const driver = appState.activeConnection.driver;
    const isSQLite = driver === 'sqlite';
    const isPostgres = driver === 'postgres';
    const q = (s: string) => (isSQLite || isPostgres ? `"${s}"` : `\`${s}\``);

    let dropSql = `DROP INDEX ${q(idxName)} ON ${q(tableMeta.name)};`;
    if (isPostgres || isSQLite) {
      dropSql = `DROP INDEX ${q(idxName)};`;
    }

    try {
      const connId = appState.activeConnection.id;
      await api.executeQuery(connId, dropSql, appState.currentDatabase || undefined);
      appState.showToast(`Index "${idxName}" dropped successfully!`, 'info');

      // Refresh catalog
      await appState.loadCatalog(connId, appState.currentDatabase || undefined);
    } catch (err: any) {
      appState.showToast(`Failed to drop index: ${err.message}`, 'error');
    }
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text font-mono text-xs">
  <!-- Top Action Toolbar -->
  <div class="h-10 bg-[#0b0f19] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0">
    <div class="flex items-center space-x-2">
      <Icon name="key" class="w-4 h-4 text-sky-400" />
      <span class="font-sans font-bold text-slate-200">Table Indexes</span>
      <span class="text-slate-500 font-mono text-xs">({tableMeta?.indexes?.length || 0} total)</span>
    </div>

    <button
      onclick={openAddIndex}
      class="px-2.5 py-1 rounded bg-sky-600 hover:bg-sky-500 text-white font-medium text-xs transition flex items-center space-x-1.5 shadow-sm"
    >
      <Icon name="plus" class="w-3.5 h-3.5" />
      <span>Add Index</span>
    </button>
  </div>

  <!-- Indexes Table Body -->
  <div class="flex-1 overflow-auto p-4">
    {#if tableMeta && tableMeta.indexes && tableMeta.indexes.length > 0}
      <div class="border border-[#1e293b] rounded-xl overflow-hidden shadow-xl bg-[#0b0f19]">
        <table class="w-full text-left border-collapse">
          <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans">
            <tr>
              <th class="py-2.5 px-4 w-12">#</th>
              <th class="py-2.5 px-4">Index Name</th>
              <th class="py-2.5 px-4">Indexed Column(s)</th>
              <th class="py-2.5 px-4">Unique</th>
              <th class="py-2.5 px-4">Primary</th>
              <th class="py-2.5 px-4">Index Type</th>
              <th class="py-2.5 px-4 text-center w-20">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#1e293b]/60">
            {#each tableMeta.indexes as idx, i}
              {@const isPK = idx.name === 'PRIMARY' || idx.is_primary}
              <tr class="hover:bg-sky-500/5 transition-colors">
                <td class="py-2 px-4 text-slate-500">{i + 1}</td>
                <td class="py-2 px-4 font-bold text-slate-100 flex items-center space-x-1.5">
                  <Icon name="key" class="w-3.5 h-3.5 {isPK ? 'text-amber-400' : 'text-sky-400'} shrink-0" />
                  <span>{idx.name}</span>
                </td>
                <td class="py-2 px-4 text-sky-300 font-semibold">
                  {Array.isArray(idx.columns) ? idx.columns.join(', ') : idx.columns}
                </td>
                <td class="py-2 px-4">
                  {#if idx.is_unique}
                    <span class="text-amber-400 font-semibold">UNIQUE</span>
                  {:else}
                    <span class="text-slate-500">NO</span>
                  {/if}
                </td>
                <td class="py-2 px-4">
                  {#if isPK}
                    <span class="text-emerald-400 font-semibold">YES</span>
                  {:else}
                    <span class="text-slate-600">-</span>
                  {/if}
                </td>
                <td class="py-2 px-4 text-slate-400">{idx.type || 'BTREE'}</td>
                <td class="py-2 px-4 text-center">
                  {#if !isPK}
                    <button
                      onclick={() => dropIndex(idx.name)}
                      class="p-1 rounded text-slate-500 hover:text-rose-400 hover:bg-slate-800 transition"
                      title="Drop Index"
                    >
                      <Icon name="trash-2" class="w-3.5 h-3.5" />
                    </button>
                  {:else}
                    <span class="text-slate-600 text-[10px] font-sans">PK</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <div class="p-8 text-center text-slate-500">
        No indexes defined for this table.
      </div>
    {/if}
  </div>
</div>

<!-- Add Index Modal -->
{#if isAddModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="key" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">Add New Table Index</h3>
        </div>
        <button onclick={() => (isAddModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Index Type *</label>
          <div class="grid grid-cols-4 gap-2">
            {#each ['INDEX', 'UNIQUE', 'FULLTEXT', 'SPATIAL'] as t}
              <button
                type="button"
                onclick={() => {
                  indexType = t as any;
                  autoGenerateIndexName();
                }}
                class="py-1.5 px-2 rounded-lg border text-xs font-medium transition text-center {indexType === t
                  ? 'bg-sky-600/30 border-sky-500 text-sky-200'
                  : 'bg-slate-900 border-slate-800 text-slate-400 hover:border-slate-700'}"
              >
                {t}
              </button>
            {/each}
          </div>
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Select Column(s) in Order *</label>
          <div class="flex flex-wrap gap-1.5 p-2 bg-[#050811] border border-[#1e293b] rounded-lg min-h-[50px] items-center">
            {#if selectedCols.length === 0}
              <span class="text-slate-600 text-xs italic">Click column tags below to add them to this index...</span>
            {:else}
              {#each selectedCols as col, idx}
                <span class="inline-flex items-center space-x-1.5 px-2 py-0.5 rounded bg-sky-950 border border-sky-700 text-sky-200 font-mono text-[11px]">
                  <span class="text-slate-400 text-[9px]">#{idx + 1}</span>
                  <span class="font-bold">{col}</span>
                  <button type="button" onclick={() => toggleColumn(col)} class="hover:text-rose-400 ml-1">
                    <Icon name="x" class="w-3 h-3" />
                  </button>
                </span>
              {/each}
            {/if}
          </div>

          <!-- Available Column Badges -->
          <div class="flex flex-wrap gap-1 mt-2">
            {#each availableCols as col}
              {@const isSelected = selectedCols.includes(col)}
              <button
                type="button"
                onclick={() => toggleColumn(col)}
                class="px-2 py-0.5 rounded font-mono text-[11px] transition border flex items-center space-x-1 {isSelected
                  ? 'bg-sky-900/40 border-sky-600 text-sky-300'
                  : 'bg-slate-900 border-slate-800 text-slate-400 hover:text-white hover:border-slate-700'}"
              >
                <Icon name={isSelected ? 'check' : 'plus'} class="w-2.5 h-2.5" />
                <span>{col}</span>
              </button>
            {/each}
          </div>
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Index Name *</label>
          <input
            type="text"
            bind:value={indexName}
            placeholder="e.g. idx_users_email"
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
          />
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Generated DDL Preview</label>
          <pre class="p-3 bg-[#050811] border border-[#1e293b] rounded-lg text-emerald-300 font-mono text-xs overflow-x-auto whitespace-pre-wrap">{sqlPreview}</pre>
        </div>
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-end space-x-2 bg-[#0f172a]">
        <button
          onclick={() => (isAddModalOpen = false)}
          disabled={isExecuting}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
        >
          Cancel
        </button>
        <button
          onclick={createIndex}
          disabled={isExecuting || selectedCols.length === 0}
          class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-sky-900/30"
        >
          <Icon name="check" class="w-3.5 h-3.5 {isExecuting ? 'animate-spin' : ''}" />
          <span>{isExecuting ? 'Creating...' : 'Create Index'}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
