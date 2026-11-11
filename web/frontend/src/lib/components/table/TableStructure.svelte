<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { TableMetadata, ColumnMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableMeta: TableMetadata | null;
  }

  interface StagedColumn extends ColumnMetadata {
    _isNew?: boolean;
    _modified?: boolean;
    _deleted?: boolean;
    _oldName?: string;
  }

  let { tableMeta }: Props = $props();

  let isAddModalOpen = $state(false);
  let isReviewModalOpen = $state(false);
  let isExecuting = $state(false);
  let reviewDDL = $state('');

  // Staged columns working copy
  let stagedColumns = $state<StagedColumn[]>([]);

  // Add / Edit Column Form State
  let editingIndex = $state<number | null>(null);
  let formColName = $state('');
  let formColType = $state('VARCHAR(255)');
  let formColNullable = $state(true);
  let formColPK = $state(false);
  let formColDefault = $state('');
  let formColExtra = $state('');

  const commonTypes = [
    'INT',
    'BIGINT',
    'VARCHAR(255)',
    'VARCHAR(100)',
    'TEXT',
    'LONGTEXT',
    'BOOLEAN',
    'DATETIME',
    'TIMESTAMP',
    'DATE',
    'DECIMAL(10,2)',
    'FLOAT',
    'DOUBLE',
    'JSON',
  ];

  // Sync staged columns whenever tableMeta changes
  $effect(() => {
    if (tableMeta && tableMeta.columns) {
      stagedColumns = tableMeta.columns.map((c) => ({
        ...c,
        _oldName: c.name,
      }));
    } else {
      stagedColumns = [];
    }
  });

  let pendingChangesCount = $derived(
    stagedColumns.filter((c) => c._isNew || c._modified || c._deleted).length
  );

  function resetForm() {
    editingIndex = null;
    formColName = '';
    formColType = 'VARCHAR(255)';
    formColNullable = true;
    formColPK = false;
    formColDefault = '';
    formColExtra = '';
  }

  function openAddColumn() {
    resetForm();
    isAddModalOpen = true;
  }

  function openEditColumn(index: number) {
    const col = stagedColumns[index];
    if (!col) return;
    editingIndex = index;
    formColName = col.name;
    formColType = col.type || 'VARCHAR(255)';
    formColNullable = !!col.is_nullable;
    formColPK = !!col.is_primary_key;
    formColDefault = col.default_value !== null && col.default_value !== undefined ? String(col.default_value) : '';
    formColExtra = col.extra || '';
    isAddModalOpen = true;
  }

  function saveColumnForm() {
    if (!formColName.trim()) {
      appState.showToast('Column name cannot be empty', 'warning');
      return;
    }

    if (editingIndex === null) {
      // Add new column
      stagedColumns = [
        ...stagedColumns,
        {
          name: formColName.trim(),
          type: formColType.trim(),
          is_nullable: formColNullable,
          is_primary_key: formColPK,
          default_value: formColDefault.trim() || undefined,
          extra: formColExtra.trim() || undefined,
          _isNew: true,
        },
      ];
      appState.showToast(`Column "${formColName.trim()}" staged for addition`, 'info');
    } else {
      // Edit existing column
      stagedColumns = stagedColumns.map((c, i) => {
        if (i === editingIndex) {
          return {
            ...c,
            name: formColName.trim(),
            type: formColType.trim(),
            is_nullable: formColNullable,
            is_primary_key: formColPK,
            default_value: formColDefault.trim() || undefined,
            extra: formColExtra.trim() || undefined,
            _modified: !c._isNew,
          };
        }
        return c;
      });
      appState.showToast(`Column "${formColName.trim()}" modified`, 'info');
    }

    isAddModalOpen = false;
    resetForm();
  }

  function toggleDeleteColumn(index: number) {
    const col = stagedColumns[index];
    if (!col) return;

    if (col._isNew) {
      // Completely remove newly added unsaved column
      stagedColumns = stagedColumns.filter((_, i) => i !== index);
    } else {
      // Toggle delete mark
      stagedColumns = stagedColumns.map((c, i) =>
        i === index ? { ...c, _deleted: !c._deleted } : c
      );
    }
  }

  function discardAllChanges() {
    if (tableMeta && tableMeta.columns) {
      stagedColumns = tableMeta.columns.map((c) => ({
        ...c,
        _oldName: c.name,
      }));
    }
    appState.showToast('All staged structural modifications discarded', 'info');
  }

  function generateAlterDDL(): string {
    if (!tableMeta || !appState.activeConnection) return '';
    const driver = appState.activeConnection.driver;
    const isSQLite = driver === 'sqlite';
    const isPostgres = driver === 'postgres';
    const tableName = tableMeta.name;

    const q = (name: string) => (isSQLite || isPostgres ? `"${name}"` : `\`${name}\``);
    const statements: string[] = [];

    // 1. Dropped columns
    stagedColumns
      .filter((c) => c._deleted && !c._isNew)
      .forEach((c) => {
        statements.push(`ALTER TABLE ${q(tableName)} DROP COLUMN ${q(c._oldName || c.name)};`);
      });

    // 2. Added columns
    stagedColumns
      .filter((c) => c._isNew && !c._deleted)
      .forEach((c) => {
        const nullClause = c.is_nullable ? 'NULL' : 'NOT NULL';
        const dfltClause = c.default_value ? `DEFAULT '${c.default_value}'` : '';
        const extraClause = c.extra || '';
        statements.push(
          `ALTER TABLE ${q(tableName)} ADD COLUMN ${q(c.name)} ${c.type} ${nullClause} ${dfltClause} ${extraClause};`.replace(/\s+/g, ' ').trim()
        );
      });

    // 3. Modified columns
    stagedColumns
      .filter((c) => c._modified && !c._isNew && !c._deleted)
      .forEach((c) => {
        const nullClause = c.is_nullable ? 'NULL' : 'NOT NULL';
        const dfltClause = c.default_value ? `DEFAULT '${c.default_value}'` : '';
        const extraClause = c.extra || '';

        if (isSQLite) {
          statements.push(`-- SQLite doesn't natively support ALTER COLUMN, requires table recreation.`);
        } else if (isPostgres) {
          if (c._oldName && c._oldName !== c.name) {
            statements.push(`ALTER TABLE ${q(tableName)} RENAME COLUMN ${q(c._oldName)} TO ${q(c.name)};`);
          }
          statements.push(`ALTER TABLE ${q(tableName)} ALTER COLUMN ${q(c.name)} TYPE ${c.type};`);
          if (c.is_nullable) {
            statements.push(`ALTER TABLE ${q(tableName)} ALTER COLUMN ${q(c.name)} DROP NOT NULL;`);
          } else {
            statements.push(`ALTER TABLE ${q(tableName)} ALTER COLUMN ${q(c.name)} SET NOT NULL;`);
          }
        } else {
          // MySQL
          if (c._oldName && c._oldName !== c.name) {
            statements.push(
              `ALTER TABLE ${q(tableName)} CHANGE COLUMN ${q(c._oldName)} ${q(c.name)} ${c.type} ${nullClause} ${dfltClause} ${extraClause};`.replace(/\s+/g, ' ').trim()
            );
          } else {
            statements.push(
              `ALTER TABLE ${q(tableName)} MODIFY COLUMN ${q(c.name)} ${c.type} ${nullClause} ${dfltClause} ${extraClause};`.replace(/\s+/g, ' ').trim()
            );
          }
        }
      });

    return statements.join('\n');
  }

  function handleReviewAndApply() {
    const ddl = generateAlterDDL();
    if (!ddl.trim()) {
      appState.showToast('No structural modifications detected to apply.', 'warning');
      return;
    }
    reviewDDL = ddl;
    isReviewModalOpen = true;
  }

  async function executeAlterDDL() {
    if (!appState.activeConnection || !reviewDDL.trim()) return;
    isExecuting = true;

    try {
      const connId = appState.activeConnection.id;
      // Execute statement(s)
      const stmts = reviewDDL
        .split(';')
        .map((s) => s.trim())
        .filter((s) => s.length > 0 && !s.startsWith('--'));

      for (const sql of stmts) {
        await api.executeQuery(connId, sql, appState.currentDatabase || undefined);
      }

      appState.showToast(`Table "${tableMeta?.name}" structure updated successfully!`, 'success');
      isReviewModalOpen = false;

      // Reload catalog to refresh schema tree and active viewer
      await appState.loadCatalog(connId, appState.currentDatabase || undefined);
    } catch (err: any) {
      appState.showToast(`Failed to update structure: ${err.message}`, 'error');
    } finally {
      isExecuting = false;
    }
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text font-mono text-xs relative">
  <!-- Top Action Toolbar -->
  <div class="h-10 bg-[#0b0f19] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0">
    <div class="flex items-center space-x-2">
      <Icon name="columns" class="w-4 h-4 text-sky-400" />
      <span class="font-sans font-bold text-slate-200">Table Columns</span>
      <span class="text-slate-500 font-mono text-xs">({stagedColumns.filter(c => !c._deleted).length} active)</span>
    </div>

    <div class="flex items-center space-x-2">
      <button
        onclick={openAddColumn}
        class="px-2.5 py-1 rounded bg-sky-600 hover:bg-sky-500 text-white font-medium text-xs transition flex items-center space-x-1.5 shadow-sm"
      >
        <Icon name="plus" class="w-3.5 h-3.5" />
        <span>Add Column</span>
      </button>
    </div>
  </div>

  <!-- Column Table Body -->
  <div class="flex-1 overflow-auto p-4 pb-20">
    {#if stagedColumns.length > 0}
      <div class="border border-[#1e293b] rounded-xl overflow-hidden shadow-xl bg-[#0b0f19]">
        <table class="w-full text-left border-collapse">
          <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans">
            <tr>
              <th class="py-2.5 px-4 w-12">#</th>
              <th class="py-2.5 px-4">Column Name</th>
              <th class="py-2.5 px-4">Data Type</th>
              <th class="py-2.5 px-4">Nullable</th>
              <th class="py-2.5 px-4">Key</th>
              <th class="py-2.5 px-4">Default Value</th>
              <th class="py-2.5 px-4">Extra</th>
              <th class="py-2.5 px-4 text-center w-24">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#1e293b]/60">
            {#each stagedColumns as col, idx}
              {@const isRowDeleted = !!col._deleted}
              {@const isRowNew = !!col._isNew}
              {@const isRowModified = !!col._modified}
              <tr
                class="transition-colors {isRowDeleted
                  ? 'bg-rose-950/20 line-through opacity-50 text-rose-400'
                  : isRowNew
                  ? 'bg-emerald-950/20 text-emerald-200'
                  : isRowModified
                  ? 'bg-amber-950/20 text-amber-200'
                  : 'hover:bg-sky-500/5'}"
              >
                <td class="py-2 px-4 text-slate-500">{idx + 1}</td>
                <td class="py-2 px-4 font-bold text-slate-100">
                  <div class="flex items-center space-x-1.5">
                    {#if col.is_primary_key}
                      <Icon name="key" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
                    {/if}
                    <span>{col.name}</span>
                    {#if isRowNew}
                      <span class="text-[9px] uppercase px-1 py-0.2 rounded bg-emerald-900 border border-emerald-600 text-emerald-300 ml-1">New</span>
                    {:else if isRowModified}
                      <span class="text-[9px] uppercase px-1 py-0.2 rounded bg-amber-900 border border-amber-600 text-amber-300 ml-1">Modified</span>
                    {/if}
                  </div>
                </td>
                <td class="py-2 px-4 text-sky-400">{col.type}</td>
                <td class="py-2 px-4">
                  {#if col.is_nullable}
                    <span class="text-emerald-400">YES</span>
                  {:else}
                    <span class="text-rose-400 font-semibold">NO</span>
                  {/if}
                </td>
                <td class="py-2 px-4">
                  {#if col.is_primary_key}
                    <span class="px-1.5 py-0.2 rounded bg-amber-950/60 border border-amber-800 text-amber-300 text-[10px] font-bold">
                      PRI
                    </span>
                  {:else}
                    <span class="text-slate-600">-</span>
                  {/if}
                </td>
                <td class="py-2 px-4 text-slate-400">
                  {col.default_value !== undefined && col.default_value !== null ? col.default_value : '-'}
                </td>
                <td class="py-2 px-4 text-slate-500 text-[11px]">
                  {col.extra || col.comment || '-'}
                </td>
                <td class="py-2 px-4 text-center">
                  <div class="flex items-center justify-center space-x-1">
                    {#if !isRowDeleted}
                      <button
                        onclick={() => openEditColumn(idx)}
                        class="p-1 rounded text-slate-400 hover:text-sky-300 hover:bg-slate-800 transition"
                        title="Edit Column"
                      >
                        <Icon name="edit" class="w-3.5 h-3.5" />
                      </button>
                    {/if}
                    <button
                      onclick={() => toggleDeleteColumn(idx)}
                      class="p-1 rounded transition {isRowDeleted ? 'text-amber-400 hover:text-amber-200 hover:bg-slate-800' : 'text-slate-500 hover:text-rose-400 hover:bg-slate-800'}"
                      title={isRowDeleted ? 'Undo Drop Column' : 'Drop Column'}
                    >
                      <Icon name={isRowDeleted ? 'rotate-ccw' : 'trash-2'} class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <div class="p-8 text-center text-slate-500">
        No column structure details found for this table.
      </div>
    {/if}
  </div>

  <!-- Bottom Staged Changes Review Bar -->
  {#if pendingChangesCount > 0}
    <div class="absolute bottom-0 inset-x-0 bg-[#0f172a] border-t border-sky-500/50 p-3 shadow-2xl flex items-center justify-between z-10 animate-fade-in">
      <div class="flex items-center space-x-3">
        <span class="flex h-2.5 w-2.5 relative">
          <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
          <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-amber-500"></span>
        </span>
        <span class="text-slate-200 font-sans font-medium">
          <strong class="text-amber-400">{pendingChangesCount}</strong> structural modification(s) staged
        </span>
      </div>

      <div class="flex items-center space-x-2">
        <button
          onclick={discardAllChanges}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
        >
          Discard
        </button>
        <button
          onclick={handleReviewAndApply}
          class="px-3.5 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-emerald-900/30"
        >
          <Icon name="check" class="w-3.5 h-3.5" />
          <span>Review & Apply ALTER TABLE</span>
        </button>
      </div>
    </div>
  {/if}
</div>

<!-- Add / Edit Column Modal -->
{#if isAddModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-md shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="columns" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">
            {editingIndex === null ? 'Add New Column' : 'Edit Column'}
          </h3>
        </div>
        <button onclick={() => (isAddModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Column Name *</label>
          <input
            type="text"
            bind:value={formColName}
            placeholder="e.g. user_id, status, created_at"
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
          />
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Data Type *</label>
          <div class="space-y-1.5">
            <input
              type="text"
              bind:value={formColType}
              placeholder="e.g. VARCHAR(255), INT, JSON"
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
            />
            <div class="flex flex-wrap gap-1">
              {#each commonTypes as ct}
                <button
                  type="button"
                  onclick={() => (formColType = ct)}
                  class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 hover:border-sky-500 text-slate-400 hover:text-sky-300 font-mono text-[10px] transition"
                >
                  {ct}
                </button>
              {/each}
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <label class="flex items-center space-x-2 p-2.5 rounded-lg bg-slate-900/50 border border-slate-800 cursor-pointer">
            <input type="checkbox" bind:checked={formColNullable} class="rounded border-slate-700 text-sky-500" />
            <span class="text-slate-300 text-xs">Nullable (NULL)</span>
          </label>
          <label class="flex items-center space-x-2 p-2.5 rounded-lg bg-slate-900/50 border border-slate-800 cursor-pointer">
            <input type="checkbox" bind:checked={formColPK} class="rounded border-slate-700 text-amber-500" />
            <span class="text-slate-300 text-xs">Primary Key (PK)</span>
          </label>
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Default Value</label>
          <input
            type="text"
            bind:value={formColDefault}
            placeholder="e.g. 0, active, CURRENT_TIMESTAMP"
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
          />
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Extra Attributes</label>
          <select
            bind:value={formColExtra}
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
          >
            <option value="">(None)</option>
            <option value="AUTO_INCREMENT">AUTO_INCREMENT</option>
            <option value="on update CURRENT_TIMESTAMP">ON UPDATE CURRENT_TIMESTAMP</option>
          </select>
        </div>
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-end space-x-2 bg-[#0f172a]">
        <button
          onclick={() => (isAddModalOpen = false)}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
        >
          Cancel
        </button>
        <button
          onclick={saveColumnForm}
          class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold transition"
        >
          {editingIndex === null ? 'Add Column' : 'Update Column'}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- DDL Review & Apply Modal -->
{#if isReviewModalOpen}
  <div class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-sky-500/40 rounded-2xl w-full max-w-xl shadow-2xl overflow-hidden flex flex-col animate-scale-in font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="terminal" class="w-4 h-4 text-emerald-400" />
          <h3 class="font-bold text-slate-100 text-sm">Review Generated ALTER TABLE DDL</h3>
        </div>
        <button onclick={() => (isReviewModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-3">
        <p class="text-xs text-slate-400">
          The following DDL statements will be executed against <strong class="text-sky-300">{tableMeta?.name}</strong>:
        </p>
        <pre class="p-3 bg-[#050811] border border-[#1e293b] rounded-lg text-emerald-300 font-mono text-xs overflow-x-auto whitespace-pre-wrap">{reviewDDL}</pre>
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <span class="text-slate-500 text-xs">Verify carefully before applying.</span>
        <div class="flex items-center space-x-2">
          <button
            onclick={() => (isReviewModalOpen = false)}
            disabled={isExecuting}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Cancel
          </button>
          <button
            onclick={executeAlterDDL}
            disabled={isExecuting}
            class="px-4 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-emerald-900/30"
          >
            <Icon name="zap" class="w-3.5 h-3.5 {isExecuting ? 'animate-spin' : ''}" />
            <span>{isExecuting ? 'Executing...' : 'Apply Modifications'}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
