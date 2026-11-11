<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  interface TableColDraft {
    name: string;
    type: string;
    length: string;
    isPK: boolean;
    isNullable: boolean;
    autoIncrement: boolean;
    defaultValue: string;
  }

  let tableName = $state('');
  let engine = $state('InnoDB');
  let comment = $state('');
  let columns = $state<TableColDraft[]>([]);
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isTableCreatorOpen) {
      tableName = '';
      engine = 'InnoDB';
      comment = '';
      columns = [
        {
          name: 'id',
          type: 'INT',
          length: '',
          isPK: true,
          isNullable: false,
          autoIncrement: true,
          defaultValue: '',
        },
        {
          name: 'created_at',
          type: 'TIMESTAMP',
          length: '',
          isPK: false,
          isNullable: false,
          autoIncrement: false,
          defaultValue: 'CURRENT_TIMESTAMP',
        },
      ];
      isSubmitting = false;
    }
  });

  const availableTypes = [
    'INT',
    'BIGINT',
    'VARCHAR',
    'TEXT',
    'LONGTEXT',
    'DATETIME',
    'TIMESTAMP',
    'BOOLEAN',
    'DECIMAL',
    'JSON',
    'ENUM',
    'BLOB',
  ];

  function addColumn() {
    columns.push({
      name: `col_${columns.length + 1}`,
      type: 'VARCHAR',
      length: '255',
      isPK: false,
      isNullable: true,
      autoIncrement: false,
      defaultValue: '',
    });
  }

  function removeColumn(idx: number) {
    if (columns.length > 1) {
      columns.splice(idx, 1);
    }
  }

  function togglePK(idx: number) {
    const col = columns[idx];
    if (!col) return;
    col.isPK = !col.isPK;
    if (col.isPK) {
      col.isNullable = false;
      if (col.defaultValue === 'NULL') col.defaultValue = '';
    } else {
      col.autoIncrement = false;
    }
  }

  let generatedDDL = $derived.by(() => {
    const tbl = tableName.trim() || 'new_table';
    const lines: string[] = [];
    const pks: string[] = [];

    columns.forEach((col) => {
      const colName = col.name.trim() || 'unnamed';
      let typeDef = col.type;
      if (col.length) {
        typeDef += `(${col.length})`;
      } else if (col.type === 'VARCHAR') {
        typeDef += '(255)';
      }

      let line = `  \`${colName}\` ${typeDef}`;
      if (!col.isNullable) {
        line += ' NOT NULL';
      } else {
        line += ' NULL';
      }

      if (col.autoIncrement && (col.type === 'INT' || col.type === 'BIGINT')) {
        line += ' AUTO_INCREMENT';
      }

      if (col.defaultValue) {
        if (col.defaultValue === 'NULL' || col.defaultValue === 'CURRENT_TIMESTAMP') {
          line += ` DEFAULT ${col.defaultValue}`;
        } else {
          line += ` DEFAULT '${col.defaultValue.replace(/'/g, "''")}'`;
        }
      }

      lines.push(line);
      if (col.isPK) {
        pks.push(`\`${colName}\``);
      }
    });

    if (pks.length > 0) {
      lines.push(`  PRIMARY KEY (${pks.join(', ')})`);
    }

    let ddl = `CREATE TABLE \`${tbl}\` (\n${lines.join(',\n')}\n) ENGINE=${engine} DEFAULT CHARSET=utf8mb4;`;
    return ddl;
  });

  async function handleCreateDirect() {
    if (!tableName.trim()) return;
    isSubmitting = true;
    try {
      await appState.createTableDirect(generatedDDL, tableName.trim());
      appState.isTableCreatorOpen = false;
    } finally {
      isSubmitting = false;
    }
  }

  function handleOpenInEditor() {
    const tbl = tableName.trim() || 'new_table';
    appState.addTab(`${tbl}.ddl.sql`, generatedDDL);
    appState.isTableCreatorOpen = false;
    appState.showToast(`Opened CREATE TABLE DDL for ${tbl} in SQL Worksheet`, 'info');
  }
</script>

{#if appState.isTableCreatorOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm select-none"
    role="dialog"
    aria-modal="true"
  >
    <button
      tabindex="-1"
      aria-label="Close"
      class="fixed inset-0 bg-transparent cursor-default"
      onclick={() => (appState.isTableCreatorOpen = false)}
    ></button>

    <div class="w-full max-w-4xl bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl flex flex-col max-h-[90vh] overflow-hidden relative z-10 animate-in fade-in zoom-in-95 duration-150">
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <Icon name="table" class="w-5 h-5 text-emerald-400" />
          <h3 class="font-bold text-sm text-slate-100">
            Create Table in <span class="font-mono text-sky-400">{appState.currentDatabase || 'current database'}</span>
          </h3>
        </div>
        <button
          onclick={() => (appState.isTableCreatorOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Content -->
      <div class="p-5 flex-1 overflow-y-auto space-y-4 text-xs font-sans">
        <!-- Table Name & Engine -->
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div class="sm:col-span-2 space-y-1">
            <label for="new-table-name" class="block font-semibold text-slate-300">
              Table Name <span class="text-rose-400">*</span>
            </label>
            <input
              id="new-table-name"
              type="text"
              required
              placeholder="e.g. customers, order_items"
              bind:value={tableName}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-1.5 text-slate-100 font-mono text-xs outline-none transition"
            />
          </div>
          <div class="space-y-1">
            <label for="new-table-engine" class="block font-semibold text-slate-300">Storage Engine</label>
            <select
              id="new-table-engine"
              bind:value={engine}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-1.5 text-slate-100 font-mono text-xs outline-none"
            >
              <option value="InnoDB">InnoDB (Standard ACID)</option>
              <option value="MyISAM">MyISAM</option>
              <option value="MEMORY">MEMORY</option>
            </select>
          </div>
        </div>

        <!-- Columns Designer Table -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-semibold text-slate-300 uppercase tracking-wider text-[11px]">Table Columns</span>
            <button
              type="button"
              onclick={addColumn}
              class="px-2.5 py-1 rounded bg-sky-950/80 hover:bg-sky-900 border border-sky-800 text-sky-300 font-semibold text-xs flex items-center space-x-1 transition"
            >
              <Icon name="plus" class="w-3.5 h-3.5" />
              <span>Add Column</span>
            </button>
          </div>

          <div class="border border-[#1e293b] rounded-lg overflow-x-auto bg-[#090d16]">
            <table class="w-full text-left border-collapse text-xs">
              <thead class="bg-[#0b0f19] border-b border-[#1e293b] text-slate-400 text-[10px] uppercase font-semibold">
                <tr>
                  <th class="p-2 text-center w-10">PK</th>
                  <th class="p-2">Column Name</th>
                  <th class="p-2 w-32">Data Type</th>
                  <th class="p-2 w-24">Length/Values</th>
                  <th class="p-2 text-center w-14">Null</th>
                  <th class="p-2 text-center w-14">A_I</th>
                  <th class="p-2">Default Value</th>
                  <th class="p-2 text-center w-10"></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[#1e293b]/60 font-mono">
                {#each columns as col, idx}
                  <tr class="hover:bg-slate-800/30">
                    <td class="p-1.5 text-center">
                      <button
                        type="button"
                        onclick={() => togglePK(idx)}
                        class="p-1 rounded {col.isPK ? 'text-amber-400 bg-amber-950/40' : 'text-slate-600 hover:text-slate-400'}"
                        title="Toggle Primary Key"
                      >
                        <Icon name="key" class="w-3.5 h-3.5" />
                      </button>
                    </td>
                    <td class="p-1.5">
                      <input
                        type="text"
                        bind:value={col.name}
                        placeholder="column_name"
                        class="w-full bg-[#0f172a] border border-[#334155] rounded px-2 py-1 text-slate-100 text-xs outline-none focus:border-sky-500"
                      />
                    </td>
                    <td class="p-1.5">
                      <select
                        bind:value={col.type}
                        class="w-full bg-[#0f172a] border border-[#334155] rounded px-1.5 py-1 text-sky-300 text-xs outline-none"
                      >
                        {#each availableTypes as t}
                          <option value={t}>{t}</option>
                        {/each}
                      </select>
                    </td>
                    <td class="p-1.5">
                      <input
                        type="text"
                        bind:value={col.length}
                        placeholder={col.type === 'VARCHAR' ? '255' : ''}
                        class="w-full bg-[#0f172a] border border-[#334155] rounded px-1.5 py-1 text-slate-100 text-xs outline-none focus:border-sky-500"
                      />
                    </td>
                    <td class="p-1.5 text-center">
                      <input
                        type="checkbox"
                        bind:checked={col.isNullable}
                        disabled={col.isPK}
                        class="rounded bg-[#0f172a] border-[#334155] text-sky-500 focus:ring-0 disabled:opacity-30"
                      />
                    </td>
                    <td class="p-1.5 text-center">
                      <input
                        type="checkbox"
                        bind:checked={col.autoIncrement}
                        disabled={!col.isPK || (col.type !== 'INT' && col.type !== 'BIGINT')}
                        class="rounded bg-[#0f172a] border-[#334155] text-amber-500 focus:ring-0 disabled:opacity-30"
                      />
                    </td>
                    <td class="p-1.5">
                      <input
                        type="text"
                        bind:value={col.defaultValue}
                        placeholder="NULL / CURRENT_TIMESTAMP"
                        class="w-full bg-[#0f172a] border border-[#334155] rounded px-2 py-1 text-slate-100 text-xs outline-none focus:border-sky-500"
                      />
                    </td>
                    <td class="p-1.5 text-center">
                      <button
                        type="button"
                        onclick={() => removeColumn(idx)}
                        disabled={columns.length <= 1}
                        class="text-slate-500 hover:text-rose-400 disabled:opacity-20 p-1"
                      >
                        \u00d7
                      </button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>

        <!-- Real-time DDL Preview -->
        <div class="space-y-1.5">
          <span class="text-[10px] font-bold uppercase tracking-wider text-slate-400">Generated SQL DDL</span>
          <pre class="bg-[#090d16] border border-slate-800 rounded-lg p-3 font-mono text-[11px] text-sky-300 overflow-x-auto leading-relaxed select-all max-h-36">{generatedDDL}</pre>
        </div>
      </div>

      <!-- Footer Buttons -->
      <div class="px-5 py-3 border-t border-[#334155] bg-[#0b0f19] flex items-center justify-between shrink-0">
        <button
          type="button"
          onclick={() => (appState.isTableCreatorOpen = false)}
          class="px-4 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 rounded font-semibold text-xs transition"
        >
          Cancel
        </button>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={handleOpenInEditor}
            class="px-3.5 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-600 rounded font-semibold text-xs transition flex items-center space-x-1.5"
          >
            <Icon name="code" class="w-3.5 h-3.5 text-purple-400" />
            <span>Open in SQL Editor</span>
          </button>

          <button
            type="button"
            onclick={handleCreateDirect}
            disabled={isSubmitting || !tableName.trim()}
            class="px-4 py-1.5 bg-sky-500 hover:bg-sky-400 disabled:opacity-50 text-slate-950 font-bold rounded text-xs transition flex items-center space-x-1.5 shadow"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Creating...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Create Table</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
