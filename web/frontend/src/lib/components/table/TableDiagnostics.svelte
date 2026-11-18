<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { TableMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableMeta: TableMetadata | null;
  }

  let { tableMeta }: Props = $props();

  let diag = $state<any>(null);
  let redundantIndexes = $state<any[]>([]);
  let isLoading = $state(false);
  let maintAction = $state('');
  let maintResult = $state<string>('');

  let driver = $derived(appState.activeConnection?.driver?.toLowerCase() || 'mysql');
  let isPostgres = $derived(driver === 'postgres' || driver === 'postgresql');
  let isSqlite = $derived(driver === 'sqlite');

  $effect(() => {
    const tbl = tableMeta?.name;
    const sid = appState.activeSessionId;
    if (tbl && sid) {
      void loadTableHealth(tbl);
    } else {
      diag = null;
      redundantIndexes = [];
    }
  });

  async function loadTableHealth(tblName: string) {
    if (!appState.activeSessionId) return;
    isLoading = true;
    const db = appState.activeTab?.database || appState.currentDatabase || '';
    try {
      const [td, ri] = await Promise.allSettled([
        api.getTableDiagnostics(appState.activeSessionId, db, tblName),
        api.getRedundantIndexes(appState.activeSessionId, db, tblName),
      ]);
      if (td.status === 'fulfilled') {
        const list = td.value?.tables || (Array.isArray(td.value) ? td.value : []);
        diag = list.find((t: any) => t.name?.toLowerCase() === tblName.toLowerCase()) || list[0] || null;
      }
      if (ri.status === 'fulfilled') {
        const list = ri.value?.indexes || ri.value?.redundant || (Array.isArray(ri.value) ? ri.value : []);
        redundantIndexes = list.filter((r: any) => !r.table || r.table.toLowerCase() === tblName.toLowerCase());
      }
    } finally {
      isLoading = false;
    }
  }

  async function runMaintenance(op: 'OPTIMIZE' | 'ANALYZE' | 'CHECK' | 'VACUUM') {
    if (!appState.activeSessionId || !tableMeta?.name) return;
    if (appState.isReadOnly && op !== 'ANALYZE' && op !== 'CHECK') {
      appState.showToast('Session is Read-Only', 'warning');
      return;
    }
    const db = appState.activeTab?.database || appState.currentDatabase || '';
    maintAction = op;
    maintResult = '';
    try {
      const res = await api.executeMaintenance(appState.activeSessionId, db, [tableMeta.name], op);
      const item = res?.results?.[0];
      maintResult = item ? `${op}: ${item.status || 'OK'}${item.message ? ` — ${item.message}` : ''}` : `${op} completed on ${tableMeta.name}`;
      appState.showToast(`${op} completed on ${tableMeta.name}`, 'success');
      await loadTableHealth(tableMeta.name);
    } catch (err: any) {
      appState.showToast(`${op} failed: ${err.message || err}`, 'error');
    } finally {
      maintAction = '';
    }
  }

  function dropRedundantIndex(item: any) {
    if (!item?.drop_sql || !appState.activeSessionId || !tableMeta) return;
    const db = appState.activeTab?.database || appState.currentDatabase || '';
    appState.openDDLReviewModal(
      item.drop_sql,
      async () => {
        if (!(await appState.executeDDL(item.drop_sql, db))) return;
        appState.showToast(`Dropped redundant index ${item.redundant_index}`, 'info');
        await appState.refreshCatalog(db);
        await loadTableHealth(tableMeta!.name);
      },
      `Drop Redundant Index: ${item.redundant_index}`,
      { tableName: tableMeta.name, database: db }
    );
  }

  function formatBytes(bytes?: number): string {
    if (!bytes || bytes <= 0 || !Number.isFinite(bytes)) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
    return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-auto p-4 select-text text-xs font-sans space-y-4">
  {#if tableMeta}
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="flex items-center space-x-2">
        <Icon name="activity" class="w-4 h-4 text-sky-400" />
        <h3 class="font-bold text-sm text-slate-100">Storage &amp; Health: {tableMeta.name}</h3>
        {#if isLoading}
          <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin text-sky-400" />
        {/if}
      </div>

      <div class="flex flex-wrap items-center gap-1.5">
        {#if !isPostgres && !isSqlite}
          <button
            type="button"
            onclick={() => runMaintenance('OPTIMIZE')}
            disabled={!!maintAction}
            class="px-2.5 py-1 rounded bg-sky-950/70 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[11px] font-semibold transition"
          >
            {maintAction === 'OPTIMIZE' ? 'Running...' : 'Optimize'}
          </button>
          <button
            type="button"
            onclick={() => runMaintenance('CHECK')}
            disabled={!!maintAction}
            class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-[11px] font-semibold transition"
          >
            {maintAction === 'CHECK' ? 'Checking...' : 'Check'}
          </button>
        {:else}
          <button
            type="button"
            onclick={() => runMaintenance('VACUUM')}
            disabled={!!maintAction}
            class="px-2.5 py-1 rounded bg-sky-950/70 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[11px] font-semibold transition"
          >
            {maintAction === 'VACUUM' ? 'Vacuuming...' : 'Vacuum'}
          </button>
        {/if}
        <button
          type="button"
          onclick={() => runMaintenance('ANALYZE')}
          disabled={!!maintAction}
          class="px-2.5 py-1 rounded bg-emerald-950/70 hover:bg-emerald-900 border border-emerald-800 text-emerald-300 text-[11px] font-semibold transition"
        >
          {maintAction === 'ANALYZE' ? 'Analyzing...' : 'Analyze'}
        </button>
        <button
          type="button"
          onclick={() => tableMeta && loadTableHealth(tableMeta.name)}
          class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[11px]"
          title="Reload table diagnostics"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin' : ''}" />
        </button>
      </div>
    </div>

    {#if maintResult}
      <div class="p-2.5 rounded-lg bg-emerald-950/40 border border-emerald-800/70 text-emerald-300 font-mono text-[11px]">
        {maintResult}
      </div>
    {/if}

    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Estimated Rows</span>
        <div class="text-xl font-bold font-mono text-slate-100">
          {(diag?.rows ?? tableMeta.row_count ?? tableMeta.rows_count ?? 0).toLocaleString()}
        </div>
      </div>
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Data Size</span>
        <div class="text-xl font-bold font-mono text-sky-400">
          {formatBytes(diag?.data_size ?? tableMeta.data_length)}
        </div>
      </div>
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Index Size</span>
        <div class="text-xl font-bold font-mono text-emerald-400">
          {formatBytes(diag?.index_size)}
        </div>
      </div>
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Reclaimable Overhead</span>
        <div class="text-xl font-bold font-mono {(diag?.overhead_size || 0) > 1024 * 1024 ? 'text-amber-400' : 'text-slate-300'}">
          {formatBytes(diag?.overhead_size)}
        </div>
      </div>
    </div>

    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-5 space-y-3">
      <h4 class="font-bold text-sm text-slate-200">Table Engine &amp; Properties</h4>
      <div class="divide-y divide-[#1e293b]/60 font-mono text-xs">
        <div class="py-2 flex justify-between">
          <span class="text-slate-400">Table Name</span>
          <span class="text-slate-100 font-semibold">{tableMeta.name}</span>
        </div>
        <div class="py-2 flex justify-between">
          <span class="text-slate-400">Object Type / Engine</span>
          <span class="text-slate-100">{tableMeta.type}{diag?.engine ? ` · ${diag.engine}` : ''}</span>
        </div>
        {#if diag?.row_format}
          <div class="py-2 flex justify-between">
            <span class="text-slate-400">Row Format</span>
            <span class="text-slate-200">{diag.row_format}</span>
          </div>
        {/if}
        {#if diag?.collation}
          {@const legacyCharset = /^(latin1|utf8_|utf8mb3_)/i.test(diag.collation)}
          <div class="py-2 flex justify-between">
            <span class="text-slate-400">Collation</span>
            <span class={legacyCharset ? 'text-amber-300 font-semibold' : 'text-slate-200'}>
              {diag.collation}{legacyCharset ? ' (Legacy charset — consider utf8mb4)' : ''}
            </span>
          </div>
        {/if}
        <div class="py-2 flex justify-between">
          <span class="text-slate-400">Primary Key</span>
          <span class="text-amber-300">{tableMeta.primary_key ? tableMeta.primary_key.join(', ') : (diag?.has_primary_key === false ? 'None (missing PK)' : 'None')}</span>
        </div>
        {#if diag?.created_at || diag?.updated_at}
          <div class="py-2 flex justify-between">
            <span class="text-slate-400">Created / Updated</span>
            <span class="text-slate-300">{diag.created_at || '-'} / {diag.updated_at || '-'}</span>
          </div>
        {/if}
        {#if tableMeta.comment}
          <div class="py-2 flex justify-between">
            <span class="text-slate-400">Table Comment</span>
            <span class="text-slate-200 italic">{tableMeta.comment}</span>
          </div>
        {/if}
      </div>
    </div>

    <!-- Redundant Indexes Advisory -->
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-5 space-y-3">
      <div class="flex items-center justify-between">
        <h4 class="font-bold text-sm text-slate-200 flex items-center space-x-2">
          <Icon name="layers" class="w-4 h-4 text-amber-400" />
          <span>Redundant Indexes ({redundantIndexes.length})</span>
        </h4>
      </div>

      {#if redundantIndexes.length === 0}
        <p class="text-slate-500 text-xs italic">✓ No shadowed or duplicate indexes detected on {tableMeta.name}.</p>
      {:else}
        <div class="space-y-2.5">
          {#each redundantIndexes as ri}
            <div class="p-3 rounded-lg bg-[#050811] border border-amber-800/50 space-y-2">
              <div class="flex items-center justify-between">
                <div class="font-mono text-xs">
                  <span class="text-amber-300 font-bold">{ri.redundant_index}</span>
                  <span class="text-slate-500">({(ri.redundant_cols || []).join(', ')}) shadowed by </span>
                  <span class="text-sky-300 font-semibold">{ri.shadowed_by}</span>
                  <span class="text-slate-500">({(ri.shadowed_cols || []).join(', ')})</span>
                </div>
                {#if ri.drop_sql}
                  <button
                    type="button"
                    onclick={() => dropRedundantIndex(ri)}
                    class="px-2.5 py-1 rounded bg-rose-950/70 hover:bg-rose-900 border border-rose-800 text-rose-300 text-[11px] font-semibold transition"
                  >
                    Drop Index
                  </button>
                {/if}
              </div>
              {#if ri.reason}
                <p class="text-[11px] text-slate-400">{ri.reason}</p>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {:else}
    <div class="p-8 text-center text-slate-500">
      No diagnostics available.
    </div>
  {/if}
</div>
