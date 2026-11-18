<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { api } from '../../api/client';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  interface NormalizedAuditEntry {
    id: string;
    executed_at: string;
    source: string;
    database?: string;
    sql: string;
    duration_ms: number;
    rows_affected?: number;
    error?: string;
  }

  let filterQuery = $state('');
  let sourceFilter = $state('all');
  let serverLogs = $state<NormalizedAuditEntry[]>([]);
  let totalCount = $state<number>(0);
  let isLoading = $state(false);
  let hasLoadedServer = $state(false);
  let fetchDebounce: ReturnType<typeof setTimeout> | null = null;
  let refreshDelayTimer: ReturnType<typeof setTimeout> | null = null;

  const SOURCE_BADGES: Record<string, { label: string; cls: string }> = {
    editor: { label: 'Editor', cls: 'bg-sky-950/60 text-sky-300 border-sky-800/70' },
    editor_stream: { label: 'Stream', cls: 'bg-cyan-950/60 text-cyan-300 border-cyan-800/70' },
    grid_cell_edit: { label: 'Inline Edit', cls: 'bg-amber-950/60 text-amber-300 border-amber-800/70' },
    grid_insert: { label: 'Insert Row', cls: 'bg-emerald-950/60 text-emerald-300 border-emerald-800/70' },
    grid_delete: { label: 'Delete Rows', cls: 'bg-rose-950/60 text-rose-300 border-rose-800/70' },
    script_runner: { label: 'Script Runner', cls: 'bg-indigo-950/60 text-indigo-300 border-indigo-800/70' },
    csv_import: { label: 'CSV Import', cls: 'bg-teal-950/60 text-teal-300 border-teal-800/70' },
    mock_data: { label: 'Mock Data', cls: 'bg-fuchsia-950/60 text-fuchsia-300 border-fuchsia-800/70' },
    sync_transfer: { label: 'Data Sync', cls: 'bg-violet-950/60 text-violet-300 border-violet-800/70' },
    schema_sync: { label: 'Structure Sync', cls: 'bg-purple-950/60 text-purple-300 border-purple-800/70' },
  };

  function badgeFor(source: string) {
    return (
      SOURCE_BADGES[source] || {
        label: source || 'editor',
        cls: 'bg-slate-800 text-slate-300 border-slate-700',
      }
    );
  }

  function normalizeServerLog(raw: any): NormalizedAuditEntry {
    return {
      id: String(raw.id || `${raw.executed_at}-${Math.random()}`),
      executed_at: String(raw.executed_at || ''),
      source: String(raw.source || 'editor'),
      database: raw.database || '',
      sql: String(raw.query_sql ?? raw.sql ?? ''),
      duration_ms: Number(raw.duration_ms ?? 0),
      rows_affected: raw.rows_affected != null ? Number(raw.rows_affected) : undefined,
      error: raw.error || undefined,
    };
  }

  async function fetchAuditLogs() {
    const sid = appState.activeSessionId;
    if (!sid) {
      hasLoadedServer = false;
      return;
    }
    isLoading = true;
    try {
      const res = await api.getAuditLogs(sid, {
        limit: 200,
        search: filterQuery.trim() || undefined,
        source: sourceFilter !== 'all' ? sourceFilter : undefined,
      });
      serverLogs = (res.logs || []).map(normalizeServerLog);
      totalCount = typeof res.total === 'number' ? res.total : serverLogs.length;
      hasLoadedServer = true;
    } catch {
      // Fallback to in-memory session logs if server query fails
      hasLoadedServer = false;
    } finally {
      isLoading = false;
    }
  }

  function scheduleFetch(delayMs = 180) {
    if (fetchDebounce) clearTimeout(fetchDebounce);
    fetchDebounce = setTimeout(() => {
      fetchAuditLogs();
    }, delayMs);
  }

  $effect(() => {
    const isOpen = appState.isAuditDrawerOpen;
    const sid = appState.activeSessionId;
    const src = sourceFilter;
    const rev = appState.auditRefreshToken;
    if (!isOpen || !sid) return;
    // Brief delay on auditRefreshToken so backend RecordAuditLog goroutine finishes writing
    if (refreshDelayTimer) clearTimeout(refreshDelayTimer);
    refreshDelayTimer = setTimeout(() => {
      void src;
      void rev;
      fetchAuditLogs();
    }, 150);
    return () => {
      if (refreshDelayTimer) clearTimeout(refreshDelayTimer);
    };
  });

  let localFallbackLogs = $derived.by<NormalizedAuditEntry[]>(() => {
    const q = filterQuery.toLowerCase().trim();
    const src = sourceFilter;
    return appState.auditLogs
      .map((l) => ({
        id: l.id,
        executed_at: l.timestamp,
        source: l.source || 'editor',
        database: l.database,
        sql: l.sql,
        duration_ms: l.duration_ms,
        rows_affected: l.affected_rows ?? l.row_count,
        error: l.error,
      }))
      .filter((log) => {
        if (src !== 'all' && log.source !== src) return false;
        if (!q) return true;
        return (
          log.sql.toLowerCase().includes(q) ||
          (log.database && log.database.toLowerCase().includes(q)) ||
          (log.error && log.error.toLowerCase().includes(q))
        );
      });
  });

  let displayedLogs = $derived<NormalizedAuditEntry[]>(
    appState.activeSessionId && hasLoadedServer ? serverLogs : localFallbackLogs
  );

  let displayedTotal = $derived<number>(
    appState.activeSessionId && hasLoadedServer ? totalCount : displayedLogs.length
  );

  function formatTimestamp(ts: string): string {
    if (!ts) return '-';
    const dt = new Date(ts);
    if (isNaN(dt.getTime())) return ts;
    return dt.toLocaleString([], {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }

  function copySql(sql: string) {
    copyText(sql);
    appState.showToast('Query copied to clipboard', 'info');
  }

  function sendToEditor(entry: NormalizedAuditEntry) {
    const active = appState.activeTab;
    const isSpecial = (t: any) =>
      t.isTableViewer || t.isProcesslist || t.isCluster || t.isVariables || t.isFlightRecorder || t.isNote;

    if (active && !isSpecial(active) && !active.sql.trim()) {
      active.sql = entry.sql;
      if (entry.database) active.database = entry.database;
      appState.tabs = [...appState.tabs];
      appState.saveTabsState();
    } else {
      const existingEmpty = appState.tabs.find((t) => !isSpecial(t) && !t.sql.trim());
      if (existingEmpty) {
        existingEmpty.sql = entry.sql;
        if (entry.database) existingEmpty.database = entry.database;
        appState.activeTabId = existingEmpty.id;
        appState.tabs = [...appState.tabs];
        appState.saveTabsState();
      } else {
        appState.addTab('New Query.sql', entry.sql);
      }
    }
    appState.isAuditDrawerOpen = false;
  }

  function exportCsv() {
    if (displayedLogs.length === 0) {
      appState.showToast('No audit logs available to export', 'info');
      return;
    }
    const esc = (v: unknown) => `"${String(v ?? '').replace(/"/g, '""')}"`;
    const headers = ['Executed At', 'Source', 'Database', 'SQL', 'Status', 'Duration (ms)', 'Rows Affected', 'Error'];
    const rows = displayedLogs.map((l) => [
      esc(l.executed_at),
      esc(l.source || 'editor'),
      esc(l.database || ''),
      esc(l.sql),
      l.error ? 'ERROR' : 'OK',
      l.duration_ms,
      l.rows_affected ?? 0,
      esc(l.error || ''),
    ]);
    const csvContent = '\uFEFF' + [headers.join(','), ...rows.map((r) => r.join(','))].join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `session-audit-logs-${Date.now()}.csv`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function clearLogs() {
    const ok = await appState.showNativeConfirm(
      'Are you sure you want to permanently clear all query execution logs for this session?',
      'Clear Audit History'
    );
    if (!ok) return;
    try {
      if (appState.activeSessionId) {
        await api.clearAuditLogs(appState.activeSessionId);
      }
      appState.auditLogs = [];
      serverLogs = [];
      totalCount = 0;
      appState.showToast('Audit log cleared', 'info');
    } catch (err: any) {
      appState.showToast(`Failed to clear audit logs: ${err?.message || err}`, 'error');
    }
  }
</script>

{#if appState.isAuditDrawerOpen}
  <div
    class="fixed inset-0 z-[100] flex justify-end bg-black/50 animate-fade-in"
    role="presentation"
  >
    <div
      class="w-full max-w-2xl h-full bg-[#0a0f1d] border-l border-slate-800 flex flex-col shadow-2xl text-slate-100"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center space-x-2">
          <Icon name="clipboard-list" class="w-4 h-4 text-sky-400" />
          <h3 class="text-sm font-semibold text-slate-100">
            Session Audit Log <span class="text-xs font-normal text-slate-400">({displayedTotal} entries)</span>
          </h3>
          {#if isLoading}
            <span class="text-[10px] font-mono text-sky-400">Syncing…</span>
          {/if}
        </div>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={exportCsv}
            class="px-2.5 py-1 text-[11px] rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 transition flex items-center space-x-1"
            title="Export visible logs to UTF-8 BOM CSV"
          >
            <Icon name="download" class="w-3 h-3 text-sky-400" />
            <span>Export CSV</span>
          </button>
          <button
            type="button"
            onclick={clearLogs}
            class="px-2.5 py-1 text-[11px] rounded bg-rose-950/50 hover:bg-rose-900/60 text-rose-300 border border-rose-900/50 transition flex items-center space-x-1"
          >
            <Icon name="trash-2" class="w-3 h-3 text-rose-400" />
            <span>Clear</span>
          </button>
          <button
            type="button"
            onclick={() => (appState.isAuditDrawerOpen = false)}
            class="text-slate-400 hover:text-slate-200 p-1 rounded hover:bg-slate-800 transition ml-1"
            aria-label="Close"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Search & Source Filters -->
      <div class="p-3 border-b border-slate-800/80 bg-slate-900/30 flex items-center gap-2">
        <div class="relative flex-1 flex items-center">
          <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2.5 pointer-events-none" />
          <input
            type="text"
            bind:value={filterQuery}
            oninput={() => scheduleFetch(220)}
            onkeydown={(e) => {
              if (e.key === 'Escape' && filterQuery) {
                e.stopPropagation();
                filterQuery = '';
                fetchAuditLogs();
              }
            }}
            placeholder="Filter audit logs by SQL, database, error..."
            class="w-full bg-[#050811] border border-slate-800 rounded pl-8 pr-7 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500"
          />
          {#if filterQuery}
            <button
              type="button"
              onclick={() => {
                filterQuery = '';
                fetchAuditLogs();
              }}
              class="absolute right-2 text-slate-500 hover:text-slate-200 p-0.5 rounded"
              aria-label="Clear search"
            >
              <Icon name="x" class="w-3 h-3" />
            </button>
          {/if}
        </div>

        <select
          id="audit-source-filter"
          bind:value={sourceFilter}
          class="bg-[#050811] border border-slate-800 rounded px-2.5 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-sky-500 cursor-pointer shrink-0"
        >
          <option value="all">All Sources</option>
          <option value="editor">Editor (Manual)</option>
          <option value="editor_stream">Editor (Stream)</option>
          <option value="grid_cell_edit">Inline Edit (Grid)</option>
          <option value="grid_insert">Insert Row (+ Add Row)</option>
          <option value="grid_delete">Delete Rows (Grid)</option>
          <option value="script_runner">Script Runner (.sql)</option>
          <option value="csv_import">CSV Import</option>
          <option value="mock_data">Mock Data</option>
          <option value="sync_transfer">Data Sync</option>
          <option value="schema_sync">Structure Sync (DDL)</option>
        </select>
      </div>

      <!-- Logs List -->
      <div class="flex-1 overflow-y-auto divide-y divide-slate-800/60 p-2 space-y-2">
        {#if displayedLogs.length === 0}
          <div class="p-12 text-center text-slate-500 text-xs">
            No query history records found.
          </div>
        {:else}
          {#each displayedLogs as log (log.id)}
            {@const badge = badgeFor(log.source)}
            <div class="p-3 rounded-lg bg-[#050811] border border-slate-800/80 hover:border-slate-700 transition space-y-2">
              <div class="flex items-center justify-between text-[11px] text-slate-400 gap-2 flex-wrap">
                <div class="flex items-center space-x-2">
                  <span class="font-mono text-slate-400">{formatTimestamp(log.executed_at)}</span>
                  <span class="px-1.5 py-0.5 rounded border text-[10px] font-sans font-medium {badge.cls}">
                    {badge.label}
                  </span>
                  {#if log.database}
                    <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-300 font-mono text-[10px]">
                      {log.database}
                    </span>
                  {/if}
                </div>

                <div class="flex items-center space-x-2">
                  <span class="font-mono {log.duration_ms > 1000 ? 'text-amber-400' : 'text-slate-400'}">
                    {log.duration_ms} ms
                  </span>
                  {#if log.error}
                    <span class="px-1.5 py-0.5 rounded bg-rose-950 text-rose-400 font-bold text-[10px] border border-rose-800">
                      FAILED
                    </span>
                  {:else}
                    <span class="px-1.5 py-0.5 rounded bg-emerald-950 text-emerald-400 font-bold text-[10px] border border-emerald-800">
                      SUCCESS
                    </span>
                  {/if}
                </div>
              </div>

              <!-- SQL snippet -->
              <div class="relative group">
                <pre class="p-2.5 pr-28 bg-slate-950 border border-slate-800/80 rounded font-mono text-[11px] text-sky-200 overflow-x-auto whitespace-pre-wrap max-h-28">{log.sql}</pre>
                <div class="absolute top-1.5 right-1.5 opacity-0 group-hover:opacity-100 focus-within:opacity-100 flex items-center space-x-1 transition">
                  <button
                    type="button"
                    onclick={() => copySql(log.sql)}
                    class="px-2 py-0.5 bg-slate-800 hover:bg-slate-700 rounded text-[10px] font-mono text-slate-300 border border-slate-700 transition"
                  >
                    Copy
                  </button>
                  <button
                    type="button"
                    onclick={() => sendToEditor(log)}
                    class="px-2 py-0.5 bg-sky-500 hover:bg-sky-400 text-slate-950 font-bold rounded text-[10px] font-sans transition"
                  >
                    To Editor
                  </button>
                </div>
              </div>

              {#if log.error}
                <div class="text-[11px] text-rose-400 font-mono bg-rose-950/20 p-2 rounded border border-rose-900/40">
                  {log.error}
                </div>
              {:else if log.rows_affected != null}
                <div class="text-[10px] font-mono text-slate-500 flex space-x-3">
                  <span>Rows affected: {log.rows_affected.toLocaleString()}</span>
                </div>
              {/if}
            </div>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}
