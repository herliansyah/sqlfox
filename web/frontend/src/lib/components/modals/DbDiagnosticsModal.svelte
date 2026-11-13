<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

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

  let tables = $state<TableDiag[]>([]);
  let isLoading = $state(false);
  let loadError = $state('');
  let maintenanceStatus = $state('');
  let filterText = $state('');
  let sortKey = $state<'size' | 'rows' | 'name' | 'frag'>('size');

  $effect(() => {
    if (appState.isDbDiagnosticsOpen && appState.activeSessionId) {
      filterText = '';
      loadDiagnostics();
    }
  });

  async function loadDiagnostics() {
    isLoading = true;
    loadError = '';
    try {
      const res = await api.getTableDiagnostics(
        appState.activeSessionId!,
        appState.currentDatabase || ''
      );
      const list: TableDiag[] = (res.tables || (Array.isArray(res) ? res : [])) as TableDiag[];
      tables = list.filter((t) => t && t.name);
    } catch (err: any) {
      loadError = err.message || 'unknown error';
      tables = [];
    } finally {
      isLoading = false;
    }
  }

  // ---- Derived aggregates -------------------------------------------------
  const totalData = $derived(tables.reduce((a, t) => a + (t.data_size || 0), 0));
  const totalIndex = $derived(tables.reduce((a, t) => a + (t.index_size || 0), 0));
  const totalFree = $derived(tables.reduce((a, t) => a + (t.overhead_size || 0), 0));
  const noPk = $derived(tables.filter((t) => !t.has_primary_key));
  const myisam = $derived(tables.filter((t) => (t.engine || '').toLowerCase() === 'myisam'));
  const fragmented = $derived(
    tables.filter((t) => (t.overhead_size || 0) > 1024 * 1024)
  );

  const indexRatio = $derived(
    totalData > 0 ? (totalIndex / totalData) * 100 : 0
  );

  const visibleTables = $derived.by(() => {
    const q = filterText.trim().toLowerCase();
    const list = q
      ? tables.filter(
          (t) =>
            t.name.toLowerCase().includes(q) ||
            (t.engine || '').toLowerCase().includes(q) ||
            (t.collation || '').toLowerCase().includes(q)
        )
      : [...tables];
    list.sort((a, b) => {
      if (sortKey === 'name') return a.name.localeCompare(b.name);
      if (sortKey === 'rows') return (b.rows || 0) - (a.rows || 0);
      if (sortKey === 'frag') return (b.overhead_size || 0) - (a.overhead_size || 0);
      return (
        (b.data_size || 0) + (b.index_size || 0) - ((a.data_size || 0) + (a.index_size || 0))
      );
    });
    return list;
  });

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

  async function runMaintenance(tableName: string, op: 'OPTIMIZE' | 'ANALYZE') {
    if (!appState.activeSessionId) return;
    maintenanceStatus = `Running ${op} on ${tableName}...`;
    try {
      await api.executeMaintenance(
        appState.activeSessionId,
        appState.currentDatabase || '',
        tableName,
        op
      );
      appState.showToast(`Completed ${op} on ${tableName}`, 'success');
      await loadDiagnostics();
    } catch (err: any) {
      appState.showToast(`Maintenance failed: ${err.message}`, 'error');
    } finally {
      maintenanceStatus = '';
    }
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
          }`
      )
      .join('\n');
    const ctx = [
      `Review storage layout and health for database \`${appState.currentDatabase || '-'}\`.`,
      '',
      `Totals: data ${formatBytes(totalData)}, index ${formatBytes(totalIndex)}, reclaimable ${formatBytes(totalFree)}. Index/data ratio ${indexRatio.toFixed(1)}%. ${noPk.length} table(s) without a primary key, ${myisam.length} MyISAM table(s), ${fragmented.length} table(s) with >1MB reclaimable.`,
      '',
      'Largest tables:',
      top,
      '',
      'Give a prioritized remediation plan: which tables to OPTIMIZE, which need a primary key, engine migrations, and index bloat to review.',
    ].join('\n');
    appState.sendToAi(ctx);
  }

  function handleClose() {
    appState.isDbDiagnosticsOpen = false;
  }

  const healthCards = $derived([
    {
      label: 'Tables',
      value: String(tables.length),
      hint: appState.currentDatabase || 'no database selected',
      tone: 'text-slate-100',
      icon: 'table',
    },
    {
      label: 'Data footprint',
      value: formatBytes(totalData),
      hint: `${formatBytes(totalIndex)} index · ratio ${indexRatio.toFixed(0)}%`,
      tone: 'text-sky-300',
      icon: 'hard-drive',
    },
    {
      label: 'Reclaimable',
      value: formatBytes(totalFree),
      hint: `${fragmented.length} table(s) over 1 MB`,
      tone: totalFree > 64 * 1024 * 1024 ? 'text-amber-300' : 'text-slate-100',
      icon: 'database',
    },
    {
      label: 'No primary key',
      value: String(noPk.length),
      hint: myisam.length ? `${myisam.length} MyISAM table(s)` : 'all engines InnoDB',
      tone: noPk.length ? 'text-rose-300' : 'text-emerald-300',
      icon: 'key',
    },
  ]);
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
              {appState.currentDatabase || 'no database selected'} · physical footprint per table
            </p>
          </div>
        </div>
        <div class="flex items-center space-x-1.5">
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
            onclick={loadDiagnostics}
            class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[11px] font-semibold transition flex items-center space-x-1.5"
            title="Re-run storage introspection"
          >
            <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin' : ''}" />
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

      <!-- Health summary -->
      <div class="grid grid-cols-4 gap-2.5 px-5 py-3 border-b border-[#1e293b] bg-[#090d16] shrink-0">
        {#each healthCards as card}
          <div class="px-3 py-2 rounded-lg border border-[#1e293b] bg-[#0b0f19]">
            <div class="flex items-center space-x-1.5 text-slate-500">
              <Icon name={card.icon} class="w-3 h-3" />
              <span class="text-[10px] uppercase tracking-wide">{card.label}</span>
            </div>
            <div class="text-sm font-bold {card.tone} font-mono mt-0.5">{card.value}</div>
            <div class="text-[10px] text-slate-500 truncate" title={card.hint}>{card.hint}</div>
          </div>
        {/each}
      </div>

      <!-- Toolbar -->
      <div class="px-5 py-2 border-b border-[#1e293b] flex items-center justify-between shrink-0 bg-[#0b0f19]">
        <div class="flex items-center space-x-1 text-[11px] font-sans">
          <span class="text-slate-500 mr-1">Sort</span>
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
        </div>
        <div class="flex items-center space-x-2">
          <input
            type="text"
            bind:value={filterText}
            placeholder="Filter by table, engine or collation..."
            class="bg-[#090d16] border border-[#334155] rounded px-2.5 py-1 text-[11px] text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-64 font-sans"
          />
          <span class="text-[11px] text-slate-500 font-mono">
            {visibleTables.length}/{tables.length}
          </span>
        </div>
      </div>

      <!-- Table list -->
      <div class="flex-1 overflow-auto">
        {#if isLoading}
          <div class="p-12 text-center text-slate-400 flex flex-col items-center gap-2">
            <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
            <span>Analyzing database storage statistics...</span>
          </div>
        {:else if loadError}
          <div class="p-10 text-center space-y-2">
            <Icon name="alert-triangle" class="w-6 h-6 text-rose-400 mx-auto" />
            <p class="text-sm text-rose-300 font-semibold">Could not read storage statistics</p>
            <p class="text-xs text-slate-400 font-mono max-w-xl mx-auto break-words">{loadError}</p>
            <button
              type="button"
              onclick={loadDiagnostics}
              class="mt-1 px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-semibold"
            >
              Retry
            </button>
          </div>
        {:else if tables.length === 0}
          <div class="p-10 text-center text-slate-500 space-y-1">
            <Icon name="database" class="w-6 h-6 mx-auto text-slate-600" />
            <p class="text-sm">
              {appState.currentDatabase
                ? `No tables found in "${appState.currentDatabase}".`
                : 'Select a database first — storage statistics are read per schema.'}
            </p>
            <p class="text-[11px] text-slate-600">
              MySQL/MariaDB report footprints from information_schema.TABLES; SQLite reports file size only.
            </p>
          </div>
        {:else}
          <table class="w-full text-left border-collapse text-xs font-mono">
            <thead
              class="bg-[#0f172a] border-b border-[#334155] text-slate-400 text-[10.5px] uppercase tracking-wider sticky top-0 font-sans z-10"
            >
              <tr>
                <th class="py-2 px-3">Table</th>
                <th class="py-2 px-3">Engine / Format</th>
                <th class="py-2 px-3 text-right">Rows est.</th>
                <th class="py-2 px-3 text-right">Data</th>
                <th class="py-2 px-3 text-right">Index</th>
                <th class="py-2 px-3 text-right">Reclaimable</th>
                <th class="py-2 px-3 w-32">Footprint</th>
                <th class="py-2 px-3">Flags</th>
                <th class="py-2 px-3 text-center font-sans">Maintenance</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#1e293b]/60">
              {#each visibleTables as t}
                <tr class="hover:bg-slate-800/30 transition-colors">
                  <td class="py-1.5 px-3 font-bold text-slate-100 truncate max-w-[16rem]" title={t.name}>
                    {t.name}
                  </td>
                  <td class="py-1.5 px-3 text-slate-400">
                    {t.engine || 'InnoDB'}{#if t.row_format}<span class="text-slate-600"> · {t.row_format}</span>{/if}
                  </td>
                  <td class="py-1.5 px-3 text-right text-slate-300">{(t.rows || 0).toLocaleString()}</td>
                  <td class="py-1.5 px-3 text-right text-sky-400 font-semibold">{formatBytes(t.data_size)}</td>
                  <td class="py-1.5 px-3 text-right text-emerald-400">{formatBytes(t.index_size)}</td>
                  <td
                    class="py-1.5 px-3 text-right {(t.overhead_size || 0) > 1024 * 1024
                      ? 'text-amber-400 font-bold'
                      : 'text-slate-400'}"
                  >
                    {formatBytes(t.overhead_size)}
                  </td>
                  <td class="py-1.5 px-3">
                    <div class="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden flex">
                      <div
                        class="h-full bg-sky-500"
                        style="width: {pct(t.data_size || 0, totalData)}%"
                        title="data {formatBytes(t.data_size)}"
                      ></div>
                      <div
                        class="h-full bg-emerald-500"
                        style="width: {pct(t.index_size || 0, totalData)}%"
                        title="index {formatBytes(t.index_size)}"
                      ></div>
                    </div>
                  </td>
                  <td class="py-1.5 px-3">
                    <div class="flex items-center gap-1">
                      {#if !t.has_primary_key}
                        <span
                          class="px-1.5 py-0.5 rounded bg-rose-950/60 border border-rose-800 text-rose-300 text-[9.5px] font-sans font-bold uppercase"
                          title="No PRIMARY KEY — replication lag and lock escalation risk">no pk</span>
                      {/if}
                      {#if (t.engine || '').toLowerCase() === 'myisam'}
                        <span
                          class="px-1.5 py-0.5 rounded bg-amber-950/60 border border-amber-800 text-amber-300 text-[9.5px] font-sans font-bold uppercase"
                          title="MyISAM has no transactions or crash safety">myisam</span>
                      {/if}
                      {#if (t.overhead_size || 0) > 1024 * 1024}
                        <span
                          class="px-1.5 py-0.5 rounded bg-slate-800 border border-slate-700 text-slate-300 text-[9.5px] font-sans font-bold uppercase"
                          title="OPTIMIZE would return this space to the filesystem">frag</span>
                      {/if}
                    </div>
                  </td>
                  <td class="py-1.5 px-3 text-center font-sans space-x-1">
                    <button
                      type="button"
                      onclick={() => runMaintenance(t.name, 'OPTIMIZE')}
                      class="px-2 py-0.5 rounded bg-sky-950/60 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[10px] font-semibold transition"
                      title="Reclaim fragmented disk space (table is locked during rebuild)"
                    >
                      Optimize
                    </button>
                    <button
                      type="button"
                      onclick={() => runMaintenance(t.name, 'ANALYZE')}
                      class="px-2 py-0.5 rounded bg-emerald-950/60 hover:bg-emerald-900 border border-emerald-800 text-emerald-300 text-[10px] font-semibold transition"
                      title="Update optimizer key distribution statistics"
                    >
                      Analyze
                    </button>
                  </td>
                </tr>
              {/each}
              {#if visibleTables.length === 0}
                <tr>
                  <td colspan="9" class="p-8 text-center text-slate-500 font-sans italic">
                    No table matches "{filterText}".
                  </td>
                </tr>
              {/if}
            </tbody>
          </table>
        {/if}
      </div>

      <!-- Footer -->
      <div
        class="px-5 py-2.5 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0 font-sans text-xs"
      >
        <span class="text-slate-400 flex items-center gap-2">
          {#if maintenanceStatus}
            <Icon name="refresh-cw" class="w-3 h-3 animate-spin text-sky-400" />
          {/if}
          {maintenanceStatus || 'Rows are InnoDB estimates from information_schema.TABLES, not COUNT(*).'}
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
