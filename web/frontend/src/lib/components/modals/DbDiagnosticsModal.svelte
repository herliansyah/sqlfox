<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';

  interface DbDiag {
    name: string;
    table_count?: number;
    data_size?: number;
    index_size?: number;
    overhead_size?: number;
    total_size?: number;
    charset?: string;
    collation?: string;
    is_system?: boolean;
  }

  interface TableDiag {
    name: string;
    engine?: string;
    rows?: number;
    data_size?: number;
    index_size?: number;
    overhead_size?: number;
    row_format?: string;
    collation?: string;
    has_primary_key?: boolean;
    created_at?: string;
    updated_at?: string;
  }

  interface RedundantIdx {
    table: string;
    redundant_index: string;
    redundant_cols?: string[];
    shadowed_by: string;
    shadowed_cols?: string[];
    reason?: string;
    drop_sql?: string;
  }

  let viewLevel = $state<'databases' | 'tables' | 'redundant'>('databases');
  let selectedDb = $state('');

  // Level 1: Databases
  let databases = $state<DbDiag[]>([]);
  let isLoadingDbs = $state(false);
  let dbLoadError = $state('');
  let showSystemDbs = $state(false);
  let dbFilterText = $state('');

  // Level 2: Tables
  let tables = $state<TableDiag[]>([]);
  let isLoadingTables = $state(false);
  let tableLoadError = $state('');
  let maintenanceStatus = $state('');
  let filterText = $state('');
  let sortKey = $state<'size' | 'rows' | 'name' | 'frag'>('size');
  let healthFilter = $state<'all' | 'no_pk' | 'frag' | 'myisam' | 'legacy_charset'>('all');
  let selectedTableNames = $state<string[]>([]);

  // Level 3: Redundant Indexes
  let redundantIndexes = $state<RedundantIdx[]>([]);
  let isLoadingRedundant = $state(false);

  $effect(() => {
    if (appState.isDbDiagnosticsOpen && appState.activeSessionId) {
      filterText = '';
      dbFilterText = '';
      healthFilter = 'all';
      selectedTableNames = [];
      selectedDb = appState.activeTab?.database || appState.currentDatabase || '';
      viewLevel = selectedDb ? 'tables' : 'databases';
      void loadAllOverview();
    }
  });

  async function loadAllOverview() {
    if (!appState.activeSessionId) return;
    void loadDatabases();
    if (selectedDb) {
      void loadTablesForDb(selectedDb);
      void loadRedundantForDb(selectedDb);
    }
  }

  async function loadDatabases() {
    if (!appState.activeSessionId) return;
    isLoadingDbs = true;
    dbLoadError = '';
    try {
      const res = await api.getDatabaseDiagnostics(appState.activeSessionId);
      const list = res?.databases || (Array.isArray(res) ? res : []);
      databases = list.filter((d: any) => d && d.name);
    } catch (err: any) {
      dbLoadError = err.message || 'Failed to inspect databases';
      databases = [];
    } finally {
      isLoadingDbs = false;
    }
  }

  async function loadTablesForDb(dbName: string) {
    if (!appState.activeSessionId || !dbName) return;
    isLoadingTables = true;
    tableLoadError = '';
    selectedTableNames = [];
    try {
      const res = await api.getTableDiagnostics(appState.activeSessionId, dbName);
      const list: TableDiag[] = (res.tables || (Array.isArray(res) ? res : [])) as TableDiag[];
      tables = list.filter((t) => t && t.name);
    } catch (err: any) {
      tableLoadError = err.message || 'unknown error';
      tables = [];
    } finally {
      isLoadingTables = false;
    }
  }

  async function loadRedundantForDb(dbName: string) {
    if (!appState.activeSessionId || !dbName) return;
    isLoadingRedundant = true;
    try {
      const res = await api.getRedundantIndexes(appState.activeSessionId, dbName);
      const list = res?.indexes || res?.redundant || (Array.isArray(res) ? res : []);
      redundantIndexes = list;
    } catch {
      redundantIndexes = [];
    } finally {
      isLoadingRedundant = false;
    }
  }

  function drillIntoDatabase(dbName: string) {
    selectedDb = dbName;
    filterText = '';
    healthFilter = 'all';
    viewLevel = 'tables';
    void loadTablesForDb(dbName);
    void loadRedundantForDb(dbName);
  }

  function isLegacyCharset(val?: string): boolean {
    return /^(latin1|utf8($|_)|utf8mb3)/i.test((val || '').trim());
  }

  const visibleDatabases = $derived.by(() => {
    const q = dbFilterText.trim().toLowerCase();
    return databases
      .filter((d) => showSystemDbs || !d.is_system)
      .filter((d) => !q || d.name.toLowerCase().includes(q) || (d.charset || '').toLowerCase().includes(q))
      .sort((a, b) => (b.total_size || (b.data_size || 0) + (b.index_size || 0)) - (a.total_size || (a.data_size || 0) + (a.index_size || 0)));
  });

  const serverTotalBytes = $derived(
    visibleDatabases.reduce((acc, d) => acc + (d.total_size || (d.data_size || 0) + (d.index_size || 0)), 0)
  );

  // ---- Level 2 Derived Aggregates -----------------------------------------
  const totalData = $derived(tables.reduce((a, t) => a + (t.data_size || 0), 0));
  const totalIndex = $derived(tables.reduce((a, t) => a + (t.index_size || 0), 0));
  const totalFree = $derived(tables.reduce((a, t) => a + (t.overhead_size || 0), 0));
  const noPk = $derived(tables.filter((t) => !t.has_primary_key));
  const myisam = $derived(tables.filter((t) => (t.engine || '').toLowerCase() === 'myisam'));
  const fragmented = $derived(tables.filter((t) => (t.overhead_size || 0) > 1024 * 1024));
  const legacyCollationTables = $derived(tables.filter((t) => isLegacyCharset(t.collation)));

  const indexRatio = $derived(totalData > 0 ? (totalIndex / totalData) * 100 : 0);

  const visibleTables = $derived.by(() => {
    const q = filterText.trim().toLowerCase();
    let list = tables.filter((t) => {
      if (healthFilter === 'no_pk' && t.has_primary_key) return false;
      if (healthFilter === 'frag' && (t.overhead_size || 0) <= 1024 * 1024) return false;
      if (healthFilter === 'myisam' && (t.engine || '').toLowerCase() !== 'myisam') return false;
      if (healthFilter === 'legacy_charset' && !isLegacyCharset(t.collation)) return false;
      if (!q) return true;
      return (
        t.name.toLowerCase().includes(q) ||
        (t.engine || '').toLowerCase().includes(q) ||
        (t.collation || '').toLowerCase().includes(q)
      );
    });
    list = [...list].sort((a, b) => {
      if (sortKey === 'name') return a.name.localeCompare(b.name);
      if (sortKey === 'rows') return (b.rows || 0) - (a.rows || 0);
      if (sortKey === 'frag') return (b.overhead_size || 0) - (a.overhead_size || 0);
      return (b.data_size || 0) + (b.index_size || 0) - ((a.data_size || 0) + (a.index_size || 0));
    });
    return list;
  });

  function toggleSelectTable(name: string) {
    if (selectedTableNames.includes(name)) {
      selectedTableNames = selectedTableNames.filter((t) => t !== name);
    } else {
      selectedTableNames = [...selectedTableNames, name];
    }
  }

  function toggleSelectAllTables() {
    if (selectedTableNames.length === visibleTables.length && visibleTables.length > 0) {
      selectedTableNames = [];
    } else {
      selectedTableNames = visibleTables.map((t) => t.name);
    }
  }

  function formatBytes(bytes?: number): string {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
    return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
  }

  function pct(part: number, whole: number): number {
    if (whole <= 0) return 0;
    return Math.max(0.6, (part / whole) * 100);
  }

  async function runMaintenance(targetTables: string | string[], op: 'OPTIMIZE' | 'ANALYZE' | 'CHECK') {
    if (!appState.activeSessionId) return;
    if (appState.isReadOnly && op === 'OPTIMIZE') {
      appState.showToast('Session is Read-Only', 'warning');
      return;
    }
    const list = Array.isArray(targetTables) ? targetTables : [targetTables];
    if (list.length === 0) return;
    const targetDb = selectedDb || appState.activeTab?.database || appState.currentDatabase || '';
    maintenanceStatus = `Running ${op} on ${list.length === 1 ? list[0] : `${list.length} tables`}...`;
    try {
      await api.executeMaintenance(appState.activeSessionId, targetDb, list, op);
      appState.showToast(`Completed ${op} on ${list.length} table(s)`, 'success');
      await loadTablesForDb(targetDb);
    } catch (err: any) {
      appState.showToast(`Maintenance failed: ${err.message}`, 'error');
    } finally {
      maintenanceStatus = '';
    }
  }

  function dropRedundantIndex(item: RedundantIdx) {
    if (!item.drop_sql || !appState.activeSessionId) return;
    const db = selectedDb || appState.currentDatabase || '';
    appState.openDDLReviewModal(
      item.drop_sql,
      async () => {
        if (!(await appState.executeDDL(item.drop_sql!, db))) return;
        appState.showToast(`Dropped redundant index ${item.redundant_index}`, 'info');
        await loadRedundantForDb(db);
        if (db === appState.currentDatabase) await appState.refreshCatalog(db);
      },
      `Drop Redundant Index: ${item.redundant_index}`,
      { tableName: item.table, database: db }
    );
  }

  function sendDigestToAI() {
    const top = [...tables]
      .sort((a, b) => (b.data_size || 0) + (b.index_size || 0) - ((a.data_size || 0) + (a.index_size || 0)))
      .slice(0, 15)
      .map(
        (t) =>
          `- ${t.name}: rows~${t.rows || 0}, data ${formatBytes(t.data_size)}, index ${formatBytes(
            t.index_size
          )}, free ${formatBytes(t.overhead_size)}, engine ${t.engine || '-'}, pk ${
            t.has_primary_key ? 'yes' : 'NO'
          }, collation ${t.collation || '-'}`
      )
      .join('\n');
    const ctx = [
      `Review storage layout and health for database \`${selectedDb || appState.currentDatabase || '-'}\`.`,
      '',
      `Totals: data ${formatBytes(totalData)}, index ${formatBytes(totalIndex)}, reclaimable ${formatBytes(totalFree)}. Index/data ratio ${indexRatio.toFixed(1)}%. ${noPk.length} table(s) without PK, ${myisam.length} MyISAM, ${fragmented.length} fragmented (>1MB), ${redundantIndexes.length} redundant indexes.`,
      '',
      'Largest tables:',
      top,
      '',
      'Give a prioritized remediation plan.',
    ].join('\n');
    appState.sendToAi(ctx);
  }

  function handleClose() {
    appState.isDbDiagnosticsOpen = false;
  }
</script>

{#if appState.isDbDiagnosticsOpen}
  <div
    class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4 font-sans"
    role="presentation"
    onclick={(e) => e.target === e.currentTarget && handleClose()}
  >
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="diag-modal-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl w-full max-w-5xl flex flex-col max-h-[88vh] overflow-hidden"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400">
            <Icon name="activity" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="diag-modal-title" class="font-bold text-sm text-slate-100">
              Database Storage &amp; Health Diagnostics
            </h3>
            <p class="text-[11px] text-slate-500">
              {viewLevel === 'databases'
                ? 'Server-wide database footprints & charset overview'
                : `${selectedDb || 'no database selected'} · ${viewLevel === 'redundant' ? 'redundant index scan' : 'per-table physical footprint & maintenance'}`}
            </p>
          </div>
        </div>
        <div class="flex items-center space-x-1.5">
          <div class="inline-flex rounded border border-slate-700 bg-[#090d16] p-0.5 text-[11px] mr-1">
            <button
              type="button"
              onclick={() => (viewLevel = 'databases')}
              class="px-2.5 py-1 rounded font-semibold transition {viewLevel === 'databases' ? 'bg-sky-600 text-white' : 'text-slate-400 hover:text-slate-200'}"
            >
              All Databases ({visibleDatabases.length})
            </button>
            <button
              type="button"
              onclick={() => (viewLevel = 'tables')}
              disabled={!selectedDb}
              class="px-2.5 py-1 rounded font-semibold transition disabled:opacity-40 {viewLevel === 'tables' ? 'bg-sky-600 text-white' : 'text-slate-400 hover:text-slate-200'}"
            >
              Tables ({tables.length})
            </button>
            <button
              type="button"
              onclick={() => {
                viewLevel = 'redundant';
                if (selectedDb) void loadRedundantForDb(selectedDb);
              }}
              disabled={!selectedDb}
              class="px-2.5 py-1 rounded font-semibold transition disabled:opacity-40 {viewLevel === 'redundant' ? 'bg-amber-600 text-white' : 'text-slate-400 hover:text-slate-200'}"
            >
              Redundant Indexes ({redundantIndexes.length})
            </button>
          </div>

          <button
            type="button"
            onclick={sendDigestToAI}
            disabled={tables.length === 0}
            class="px-2.5 py-1 rounded bg-sky-950/60 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[11px] font-semibold transition disabled:opacity-40 flex items-center space-x-1.5"
            title="Hand this storage report to the AI Copilot"
          >
            <Icon name="bot" class="w-3.5 h-3.5" />
            <span>Send to AI</span>
          </button>
          <button
            type="button"
            onclick={() => loadAllOverview()}
            class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[11px] font-semibold transition flex items-center space-x-1.5"
          >
            <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoadingDbs || isLoadingTables ? 'animate-spin' : ''}" />
            <span>Refresh</span>
          </button>
          <button
            type="button"
            onclick={handleClose}
            class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
            aria-label="Close"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      {#if viewLevel === 'databases'}
        <!-- LEVEL 1: ALL DATABASES OVERVIEW -->
        <div class="px-5 py-2.5 border-b border-[#1e293b] bg-[#090d16] flex items-center justify-between gap-3 shrink-0 text-xs">
          <div class="flex items-center space-x-3">
            <span class="text-slate-400">
              Total Server Footprint: <strong class="font-mono text-sky-300">{formatBytes(serverTotalBytes)}</strong>
            </span>
            <label class="flex items-center space-x-1.5 text-[11px] text-slate-400 cursor-pointer">
              <input type="checkbox" bind:checked={showSystemDbs} class="rounded border-slate-700 bg-slate-900 text-sky-500" />
              <span>Show System Databases</span>
            </label>
          </div>
          <input
            type="text"
            bind:value={dbFilterText}
            placeholder="Filter databases or charset..."
            class="bg-[#0b0f19] border border-[#334155] rounded px-2.5 py-1 text-[11px] text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-64"
          />
        </div>

        <div class="flex-1 overflow-auto">
          {#if isLoadingDbs}
            <div class="p-12 text-center text-slate-400 flex flex-col items-center gap-2">
              <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
              <span>Scanning server databases...</span>
            </div>
          {:else if dbLoadError}
            <div class="p-10 text-center text-rose-300 text-xs">{dbLoadError}</div>
          {:else}
            <table class="w-full text-left border-collapse text-xs font-mono">
              <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-400 text-[10.5px] uppercase tracking-wider sticky top-0 font-sans">
                <tr>
                  <th class="py-2 px-4">Database</th>
                  <th class="py-2 px-4 text-right">Tables</th>
                  <th class="py-2 px-4 text-right">Data</th>
                  <th class="py-2 px-4 text-right">Index</th>
                  <th class="py-2 px-4 text-right">Overhead</th>
                  <th class="py-2 px-4 text-right">Total Footprint</th>
                  <th class="py-2 px-4">Charset / Collation</th>
                  <th class="py-2 px-4 text-right font-sans">Inspect</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[#1e293b]/60">
                {#each visibleDatabases as db}
                  {@const tot = db.total_size || (db.data_size || 0) + (db.index_size || 0)}
                  {@const legacyCs = isLegacyCharset(db.charset) || isLegacyCharset(db.collation)}
                  <tr
                    onclick={() => drillIntoDatabase(db.name)}
                    class="hover:bg-sky-500/10 cursor-pointer transition-colors {db.name === selectedDb ? 'bg-sky-500/5' : ''}"
                  >
                    <td class="py-2 px-4 font-bold text-slate-100 flex items-center space-x-2">
                      <Icon name="database" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                      <span>{db.name}</span>
                      {#if db.is_system}
                        <span class="px-1 py-0.2 rounded bg-slate-800 border border-slate-700 text-[9px] text-slate-400 font-sans uppercase">sys</span>
                      {/if}
                    </td>
                    <td class="py-2 px-4 text-right text-slate-300">{(db.table_count || 0).toLocaleString()}</td>
                    <td class="py-2 px-4 text-right text-sky-400">{formatBytes(db.data_size)}</td>
                    <td class="py-2 px-4 text-right text-emerald-400">{formatBytes(db.index_size)}</td>
                    <td class="py-2 px-4 text-right text-amber-400">{formatBytes(db.overhead_size)}</td>
                    <td class="py-2 px-4 text-right font-bold text-slate-100">{formatBytes(tot)}</td>
                    <td class="py-2 px-4 text-[11px]">
                      <span class={legacyCs ? 'text-amber-300 font-semibold' : 'text-slate-400'}>
                        {db.charset || '-'}{db.collation ? ` / ${db.collation}` : ''}
                      </span>
                      {#if legacyCs}
                        <span class="ml-1 px-1 rounded bg-amber-950/70 border border-amber-800 text-[9px] text-amber-300 font-sans">legacy</span>
                      {/if}
                    </td>
                    <td class="py-2 px-4 text-right font-sans">
                      <span class="text-sky-400 text-[11px] font-semibold">Drill In →</span>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>

      {:else if viewLevel === 'tables'}
        <!-- LEVEL 2: TABLES HEALTH & MAINTENANCE -->
        <div class="grid grid-cols-2 sm:grid-cols-5 gap-2 px-5 py-3 border-b border-[#1e293b] bg-[#090d16] shrink-0 text-xs">
          <button
            type="button"
            onclick={() => (healthFilter = 'all')}
            class="px-3 py-2 rounded-lg border text-left transition {healthFilter === 'all' ? 'border-sky-600 bg-sky-950/30' : 'border-[#1e293b] bg-[#0b0f19]'}"
          >
            <span class="text-[10px] uppercase text-slate-500 block">Tables</span>
            <div class="text-sm font-bold text-slate-100 font-mono">{tables.length}</div>
            <div class="text-[10px] text-slate-500 truncate">{formatBytes(totalData + totalIndex)} total</div>
          </button>
          <button
            type="button"
            onclick={() => (healthFilter = healthFilter === 'frag' ? 'all' : 'frag')}
            class="px-3 py-2 rounded-lg border text-left transition {healthFilter === 'frag' ? 'border-amber-600 bg-amber-950/30' : 'border-[#1e293b] bg-[#0b0f19]'}"
          >
            <span class="text-[10px] uppercase text-slate-500 block">Reclaimable</span>
            <div class="text-sm font-bold text-amber-300 font-mono">{formatBytes(totalFree)}</div>
            <div class="text-[10px] text-slate-500">{fragmented.length} fragmented &gt;1MB</div>
          </button>
          <button
            type="button"
            onclick={() => (healthFilter = healthFilter === 'no_pk' ? 'all' : 'no_pk')}
            class="px-3 py-2 rounded-lg border text-left transition {healthFilter === 'no_pk' ? 'border-rose-600 bg-rose-950/30' : 'border-[#1e293b] bg-[#0b0f19]'}"
          >
            <span class="text-[10px] uppercase text-slate-500 block">Missing PK</span>
            <div class="text-sm font-bold {noPk.length ? 'text-rose-300' : 'text-emerald-300'} font-mono">{noPk.length}</div>
            <div class="text-[10px] text-slate-500">Click to filter</div>
          </button>
          <button
            type="button"
            onclick={() => (healthFilter = healthFilter === 'legacy_charset' ? 'all' : 'legacy_charset')}
            class="px-3 py-2 rounded-lg border text-left transition {healthFilter === 'legacy_charset' ? 'border-amber-600 bg-amber-950/30' : 'border-[#1e293b] bg-[#0b0f19]'}"
          >
            <span class="text-[10px] uppercase text-slate-500 block">Legacy Charset</span>
            <div class="text-sm font-bold {legacyCollationTables.length ? 'text-amber-300' : 'text-emerald-300'} font-mono">{legacyCollationTables.length}</div>
            <div class="text-[10px] text-slate-500">latin1 / utf8mb3</div>
          </button>
          <button
            type="button"
            onclick={() => {
              viewLevel = 'redundant';
              if (selectedDb) void loadRedundantForDb(selectedDb);
            }}
            class="px-3 py-2 rounded-lg border border-[#1e293b] hover:border-amber-700 bg-[#0b0f19] text-left transition"
          >
            <span class="text-[10px] uppercase text-slate-500 block">Redundant Indexes</span>
            <div class="text-sm font-bold {redundantIndexes.length ? 'text-amber-300' : 'text-emerald-300'} font-mono">{redundantIndexes.length}</div>
            <div class="text-[10px] text-sky-400">Inspect Advisor →</div>
          </button>
        </div>

        <!-- Toolbar -->
        <div class="px-5 py-2 border-b border-[#1e293b] flex flex-wrap items-center justify-between gap-2 shrink-0 bg-[#0b0f19]">
          <div class="flex items-center space-x-1 text-[11px]">
            <button
              type="button"
              onclick={() => (viewLevel = 'databases')}
              class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-semibold mr-2"
            >
              ← DBs
            </button>
            <span class="text-slate-500 mr-1">Sort:</span>
            {#each [['size', 'Total size'], ['rows', 'Rows'], ['frag', 'Fragmented'], ['name', 'Name']] as [key, label]}
              <button
                type="button"
                onclick={() => (sortKey = key as 'size' | 'rows' | 'frag' | 'name')}
                class="px-2 py-0.5 rounded border transition {sortKey === key
                  ? 'bg-sky-950/70 border-sky-800 text-sky-300 font-semibold'
                  : 'border-transparent text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'}"
              >
                {label}
              </button>
            {/each}
            {#if selectedTableNames.length > 0}
              <div class="ml-3 flex items-center space-x-1 bg-sky-950/50 border border-sky-800/60 rounded px-2 py-0.5">
                <span class="text-sky-300 font-semibold">{selectedTableNames.length} selected:</span>
                <button onclick={() => runMaintenance(selectedTableNames, 'OPTIMIZE')} class="px-1.5 py-0.2 rounded bg-sky-600 text-white text-[10px] font-bold">Optimize</button>
                <button onclick={() => runMaintenance(selectedTableNames, 'ANALYZE')} class="px-1.5 py-0.2 rounded bg-emerald-600 text-white text-[10px] font-bold">Analyze</button>
                <button onclick={() => runMaintenance(selectedTableNames, 'CHECK')} class="px-1.5 py-0.2 rounded bg-slate-700 text-white text-[10px] font-bold">Check</button>
              </div>
            {/if}
          </div>
          <div class="flex items-center space-x-2">
            <input
              type="text"
              bind:value={filterText}
              placeholder="Filter table, engine, collation..."
              class="bg-[#090d16] border border-[#334155] rounded px-2.5 py-1 text-[11px] text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-56"
            />
            <span class="text-[11px] text-slate-500 font-mono">{visibleTables.length}/{tables.length}</span>
          </div>
        </div>

        <div class="flex-1 overflow-auto">
          {#if isLoadingTables}
            <div class="p-12 text-center text-slate-400 flex flex-col items-center gap-2">
              <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
              <span>Analyzing {selectedDb} storage statistics...</span>
            </div>
          {:else if tableLoadError}
            <div class="p-10 text-center space-y-2 text-xs text-rose-300">{tableLoadError}</div>
          {:else}
            <table class="w-full text-left border-collapse text-xs font-mono">
              <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-400 text-[10.5px] uppercase tracking-wider sticky top-0 font-sans z-10">
                <tr>
                  <th class="py-2 px-3 w-8 text-center">
                    <input
                      type="checkbox"
                      checked={visibleTables.length > 0 && selectedTableNames.length === visibleTables.length}
                      onchange={toggleSelectAllTables}
                      class="rounded border-slate-700 bg-slate-900 text-sky-500"
                    />
                  </th>
                  <th class="py-2 px-3">Table</th>
                  <th class="py-2 px-3">Engine / Format</th>
                  <th class="py-2 px-3 text-right">Rows est.</th>
                  <th class="py-2 px-3 text-right">Data / Index</th>
                  <th class="py-2 px-3 text-right">Reclaimable</th>
                  <th class="py-2 px-3 w-24">Footprint</th>
                  <th class="py-2 px-3">Health Flags</th>
                  <th class="py-2 px-3 text-center font-sans">Maintenance</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[#1e293b]/60">
                {#each visibleTables as t}
                  <tr class="hover:bg-slate-800/30 transition-colors">
                    <td class="py-1.5 px-3 text-center">
                      <input
                        type="checkbox"
                        checked={selectedTableNames.includes(t.name)}
                        onchange={() => toggleSelectTable(t.name)}
                        class="rounded border-slate-700 bg-slate-900 text-sky-500"
                      />
                    </td>
                    <td class="py-1.5 px-3 font-bold text-slate-100 truncate max-w-[14rem]" title={t.name}>{t.name}</td>
                    <td class="py-1.5 px-3 text-slate-400">
                      {t.engine || 'InnoDB'}{#if t.row_format}<span class="text-slate-600"> · {t.row_format}</span>{/if}
                    </td>
                    <td class="py-1.5 px-3 text-right text-slate-300">{(t.rows || 0).toLocaleString()}</td>
                    <td class="py-1.5 px-3 text-right whitespace-nowrap">
                      <span class="text-sky-400 font-semibold">{formatBytes(t.data_size)}</span>
                      <span class="text-slate-600"> / </span>
                      <span class="text-emerald-400">{formatBytes(t.index_size)}</span>
                    </td>
                    <td class="py-1.5 px-3 text-right {(t.overhead_size || 0) > 1024 * 1024 ? 'text-amber-400 font-bold' : 'text-slate-400'}">
                      {formatBytes(t.overhead_size)}
                    </td>
                    <td class="py-1.5 px-3">
                      <div class="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden flex">
                        <div class="h-full bg-sky-500" style="width: {pct(t.data_size || 0, totalData)}%"></div>
                        <div class="h-full bg-emerald-500" style="width: {pct(t.index_size || 0, totalData)}%"></div>
                      </div>
                    </td>
                    <td class="py-1.5 px-3">
                      <div class="flex flex-wrap items-center gap-1">
                        {#if !t.has_primary_key}
                          <span class="px-1 py-0.2 rounded bg-rose-950/60 border border-rose-800 text-rose-300 text-[9px] font-sans font-bold uppercase">no pk</span>
                        {/if}
                        {#if (t.engine || '').toLowerCase() === 'myisam'}
                          <span class="px-1 py-0.2 rounded bg-amber-950/60 border border-amber-800 text-amber-300 text-[9px] font-sans font-bold uppercase">myisam</span>
                        {/if}
                        {#if isLegacyCharset(t.collation)}
                          <span class="px-1 py-0.2 rounded bg-amber-950/60 border border-amber-800 text-amber-300 text-[9px] font-sans font-bold uppercase" title={t.collation}>legacy charset</span>
                        {/if}
                      </div>
                    </td>
                    <td class="py-1.5 px-3 text-center font-sans space-x-1 whitespace-nowrap">
                      <button onclick={() => runMaintenance(t.name, 'CHECK')} class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[10px]">Check</button>
                      <button onclick={() => runMaintenance(t.name, 'OPTIMIZE')} class="px-1.5 py-0.5 rounded bg-sky-950/60 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[10px]">Optimize</button>
                      <button onclick={() => runMaintenance(t.name, 'ANALYZE')} class="px-1.5 py-0.5 rounded bg-emerald-950/60 hover:bg-emerald-900 border border-emerald-800 text-emerald-300 text-[10px]">Analyze</button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>

      {:else}
        <!-- LEVEL 3: REDUNDANT INDEXES TAB -->
        <div class="flex-1 overflow-auto p-5 space-y-3">
          {#if isLoadingRedundant}
            <div class="p-10 text-center text-slate-400 text-xs">Scanning index definitions across {selectedDb}...</div>
          {:else if redundantIndexes.length === 0}
            <div class="p-10 text-center text-emerald-300 text-xs">
              ✓ No shadowed or duplicate indexes detected in <span class="font-mono font-bold">{selectedDb}</span>.
            </div>
          {:else}
            {#each redundantIndexes as ri}
              <div class="p-4 rounded-xl bg-[#050811] border border-amber-800/50 space-y-2 text-xs">
                <div class="flex items-center justify-between gap-2">
                  <div class="font-mono">
                    <span class="text-sky-400 font-bold">{ri.table}.</span><span class="text-amber-300 font-bold">{ri.redundant_index}</span>
                    <span class="text-slate-400"> ({(ri.redundant_cols || []).join(', ')})</span>
                    <span class="text-slate-500"> shadowed by </span>
                    <span class="text-emerald-300 font-semibold">{ri.shadowed_by}</span>
                    <span class="text-slate-400"> ({(ri.shadowed_cols || []).join(', ')})</span>
                  </div>
                  <div class="flex items-center space-x-1.5 shrink-0">
                    {#if ri.drop_sql}
                      <button
                        type="button"
                        onclick={() => { copyText(ri.drop_sql!); appState.showToast('Copied DROP INDEX SQL', 'info'); }}
                        class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[10.5px]"
                      >
                        Copy SQL
                      </button>
                      <button
                        type="button"
                        onclick={() => dropRedundantIndex(ri)}
                        class="px-2.5 py-1 rounded bg-rose-950/70 hover:bg-rose-900 border border-rose-800 text-rose-300 text-[10.5px] font-semibold"
                      >
                        Drop Index
                      </button>
                    {/if}
                  </div>
                </div>
                {#if ri.reason}
                  <p class="text-[11px] text-slate-400">{ri.reason}</p>
                {/if}
                {#if ri.drop_sql}
                  <pre class="p-2 rounded bg-slate-900/80 border border-slate-800 font-mono text-[11px] text-rose-300 overflow-x-auto">{ri.drop_sql}</pre>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      {/if}

      <!-- Footer -->
      <div class="px-5 py-2.5 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0 font-sans text-xs">
        <span class="text-slate-400 flex items-center gap-2">
          {#if maintenanceStatus}
            <Icon name="refresh-cw" class="w-3 h-3 animate-spin text-sky-400" />
          {/if}
          {maintenanceStatus || 'Statistics read from information_schema; Batch Optimize locks rebuilt tables briefly.'}
        </span>
        <button
          onclick={handleClose}
          type="button"
          class="px-4 py-1.5 bg-slate-800 hover:bg-slate-700 text-white font-semibold rounded transition"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
