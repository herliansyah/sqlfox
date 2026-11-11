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

  // Form State
  let fkName = $state('');
  let sourceCol = $state('');
  let refTable = $state('');
  let refCol = $state('');
  let onDelete = $state<'CASCADE' | 'SET NULL' | 'RESTRICT' | 'NO ACTION'>('RESTRICT');
  let onUpdate = $state<'CASCADE' | 'SET NULL' | 'RESTRICT' | 'NO ACTION'>('RESTRICT');

  let currentTableCols = $derived(
    (tableMeta?.columns || []).map((c: ColumnMetadata) => c.name)
  );

  let catalogTables = $derived(
    (appState.catalog?.tables || []).map((t) => t.name)
  );

  let refTableCols = $derived.by(() => {
    if (!refTable || !appState.catalog?.tables) return [];
    const t = appState.catalog.tables.find(
      (tbl) => tbl.name.toLowerCase() === refTable.toLowerCase()
    );
    return t ? t.columns.map((c: ColumnMetadata) => c.name) : [];
  });

  function resetForm() {
    fkName = '';
    sourceCol = currentTableCols[0] || '';
    refTable = catalogTables.find((t) => t !== tableMeta?.name) || '';
    refCol = '';
    onDelete = 'RESTRICT';
    onUpdate = 'RESTRICT';
  }

  function openAddFK() {
    resetForm();
    if (refTable) {
      const firstCol = refTableCols[0];
      if (firstCol) refCol = firstCol;
    }
    autoGenerateFKName();
    isAddModalOpen = true;
  }

  function autoGenerateFKName() {
    if (!tableMeta) return;
    const s = sourceCol || 'col';
    const r = refTable || 'ref';
    fkName = `fk_${tableMeta.name}_${s}_${r}`;
  }

  $effect(() => {
    if (refTable && refTableCols.length > 0 && !refTableCols.includes(refCol)) {
      refCol = refTableCols[0] || '';
    }
    autoGenerateFKName();
  });

  let sqlPreview = $derived.by(() => {
    if (!tableMeta || !appState.activeConnection) return '';
    const driver = appState.activeConnection.driver;
    const isSQLite = driver === 'sqlite';
    const isPostgres = driver === 'postgres';
    const q = (s: string) => (isSQLite || isPostgres ? `"${s}"` : `\`${s}\``);

    const name = fkName.trim() || 'fk_name';
    const sCol = sourceCol || 'column';
    const rTable = refTable || 'ref_table';
    const rCol = refCol || 'ref_column';

    if (isSQLite) {
      return `-- SQLite foreign keys must be declared during CREATE TABLE.`;
    }

    return `ALTER TABLE ${q(tableMeta.name)} ADD CONSTRAINT ${q(name)} FOREIGN KEY (${q(sCol)}) REFERENCES ${q(rTable)} (${q(rCol)}) ON DELETE ${onDelete} ON UPDATE ${onUpdate};`;
  });

  async function createForeignKey() {
    if (!appState.activeConnection || !tableMeta) return;
    if (!sourceCol || !refTable || !refCol) {
      appState.showToast('Please select source column, referenced table, and referenced column', 'warning');
      return;
    }
    if (!fkName.trim()) {
      appState.showToast('Constraint name cannot be empty', 'warning');
      return;
    }

    isExecuting = true;
    try {
      const connId = appState.activeConnection.id;
      await api.executeQuery(connId, sqlPreview, appState.currentDatabase || undefined);
      appState.showToast(`Foreign Key "${fkName.trim()}" created successfully!`, 'success');
      isAddModalOpen = false;
      resetForm();

      // Refresh catalog
      await appState.loadCatalog(connId, appState.currentDatabase || undefined);
    } catch (err: any) {
      appState.showToast(`Failed to create foreign key: ${err.message}`, 'error');
    } finally {
      isExecuting = false;
    }
  }

  async function dropForeignKey(constraintName: string) {
    if (!appState.activeConnection || !tableMeta) return;
    const confirmed = confirm(`Are you sure you want to drop foreign key constraint "${constraintName}" from table "${tableMeta.name}"?`);
    if (!confirmed) return;

    const driver = appState.activeConnection.driver;
    const isPostgres = driver === 'postgres';
    const q = (s: string) => (isPostgres ? `"${s}"` : `\`${s}\``);

    let dropSql = `ALTER TABLE ${q(tableMeta.name)} DROP FOREIGN KEY ${q(constraintName)};`;
    if (isPostgres) {
      dropSql = `ALTER TABLE ${q(tableMeta.name)} DROP CONSTRAINT ${q(constraintName)};`;
    }

    try {
      const connId = appState.activeConnection.id;
      await api.executeQuery(connId, dropSql, appState.currentDatabase || undefined);
      appState.showToast(`Foreign key "${constraintName}" dropped successfully!`, 'info');

      // Refresh catalog
      await appState.loadCatalog(connId, appState.currentDatabase || undefined);
    } catch (err: any) {
      appState.showToast(`Failed to drop foreign key: ${err.message}`, 'error');
    }
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text font-mono text-xs">
  <!-- Top Action Toolbar -->
  <div class="h-10 bg-[#0b0f19] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0">
    <div class="flex items-center space-x-2">
      <Icon name="link" class="w-4 h-4 text-sky-400" />
      <span class="font-sans font-bold text-slate-200">Foreign Key Constraints</span>
      <span class="text-slate-500 font-mono text-xs">({tableMeta?.foreign_keys?.length || 0} total)</span>
    </div>

    <button
      onclick={openAddFK}
      class="px-2.5 py-1 rounded bg-sky-600 hover:bg-sky-500 text-white font-medium text-xs transition flex items-center space-x-1.5 shadow-sm"
    >
      <Icon name="plus" class="w-3.5 h-3.5" />
      <span>Add Foreign Key</span>
    </button>
  </div>

  <!-- Foreign Keys Table Body -->
  <div class="flex-1 overflow-auto p-4">
    {#if tableMeta && tableMeta.foreign_keys && tableMeta.foreign_keys.length > 0}
      <div class="border border-[#1e293b] rounded-xl overflow-hidden shadow-xl bg-[#0b0f19]">
        <table class="w-full text-left border-collapse">
          <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans">
            <tr>
              <th class="py-2.5 px-4 w-12">#</th>
              <th class="py-2.5 px-4">Constraint Name</th>
              <th class="py-2.5 px-4">Source Column</th>
              <th class="py-2.5 px-4">Referenced Table</th>
              <th class="py-2.5 px-4">Referenced Column</th>
              <th class="py-2.5 px-4">On Delete</th>
              <th class="py-2.5 px-4">On Update</th>
              <th class="py-2.5 px-4 text-center w-20">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#1e293b]/60">
            {#each tableMeta.foreign_keys as fk, i}
              <tr class="hover:bg-sky-500/5 transition-colors">
                <td class="py-2 px-4 text-slate-500">{i + 1}</td>
                <td class="py-2 px-4 font-bold text-slate-100 flex items-center space-x-1.5">
                  <Icon name="link" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                  <span>{fk.name || `fk_${i + 1}`}</span>
                </td>
                <td class="py-2 px-4 text-sky-300 font-semibold">{fk.column}</td>
                <td class="py-2 px-4 text-emerald-400 font-semibold">{fk.referenced_table}</td>
                <td class="py-2 px-4 text-amber-300 font-semibold">{fk.referenced_column}</td>
                <td class="py-2 px-4 text-slate-400 font-mono text-[11px]">{fk.on_delete || 'RESTRICT'}</td>
                <td class="py-2 px-4 text-slate-400 font-mono text-[11px]">{fk.on_update || 'RESTRICT'}</td>
                <td class="py-2 px-4 text-center">
                  {#if fk.name}
                    <button
                      onclick={() => dropForeignKey(fk.name)}
                      class="p-1 rounded text-slate-500 hover:text-rose-400 hover:bg-slate-800 transition"
                      title="Drop Constraint"
                    >
                      <Icon name="trash-2" class="w-3.5 h-3.5" />
                    </button>
                  {:else}
                    <span class="text-slate-600">-</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <div class="p-8 text-center text-slate-500">
        No foreign key constraints defined for this table.
      </div>
    {/if}
  </div>
</div>

<!-- Add Foreign Key Modal -->
{#if isAddModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="link" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">Add Foreign Key Constraint</h3>
        </div>
        <button onclick={() => (isAddModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Source Column ({tableMeta?.name}) *</label>
          <select
            bind:value={sourceCol}
            onchange={autoGenerateFKName}
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
          >
            {#each currentTableCols as c}
              <option value={c}>{c}</option>
            {/each}
          </select>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Referenced Table *</label>
            <select
              bind:value={refTable}
              onchange={autoGenerateFKName}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
            >
              {#each catalogTables as t}
                <option value={t}>{t}</option>
              {/each}
            </select>
          </div>
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Referenced Column *</label>
            <select
              bind:value={refCol}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
            >
              {#each refTableCols as c}
                <option value={c}>{c}</option>
              {/each}
            </select>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">ON DELETE Rule</label>
            <select
              bind:value={onDelete}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
            >
              <option value="CASCADE">CASCADE</option>
              <option value="SET NULL">SET NULL</option>
              <option value="RESTRICT">RESTRICT</option>
              <option value="NO ACTION">NO ACTION</option>
            </select>
          </div>
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">ON UPDATE Rule</label>
            <select
              bind:value={onUpdate}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
            >
              <option value="CASCADE">CASCADE</option>
              <option value="SET NULL">SET NULL</option>
              <option value="RESTRICT">RESTRICT</option>
              <option value="NO ACTION">NO ACTION</option>
            </select>
          </div>
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Constraint Name *</label>
          <input
            type="text"
            bind:value={fkName}
            placeholder="e.g. fk_orders_user_id"
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
          onclick={createForeignKey}
          disabled={isExecuting || !sourceCol || !refTable || !refCol}
          class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-sky-900/30"
        >
          <Icon name="check" class="w-3.5 h-3.5 {isExecuting ? 'animate-spin' : ''}" />
          <span>{isExecuting ? 'Creating...' : 'Create Foreign Key'}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
