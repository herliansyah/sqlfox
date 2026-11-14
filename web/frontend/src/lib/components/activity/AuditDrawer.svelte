<script lang="ts">
import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let filterQuery = $state('');

  let filteredLogs = $derived.by(() => {
    const q = filterQuery.toLowerCase().trim();
    if (!q) return appState.auditLogs;
    return appState.auditLogs.filter(
      (log) =>
        log.sql.toLowerCase().includes(q) ||
        (log.database && log.database.toLowerCase().includes(q)) ||
        (log.error && log.error.toLowerCase().includes(q))
    );
  });

  function copySql(sql: string) {
    copyText(sql);
    appState.showToast('Query copied to clipboard', 'info');
  }

  function clearLogs() {
    appState.auditLogs = [];
    appState.showToast('Audit log cleared', 'info');
  }
</script>

{#if appState.isAuditDrawerOpen}
  <div
    class="fixed inset-0 z-[100] flex justify-end bg-black/50 backdrop-blur-sm animate-fade-in"
    role="presentation"
  >
    <div
      class="w-full max-w-xl h-full bg-[#0a0f1d] border-l border-slate-800 flex flex-col shadow-2xl text-slate-100"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center space-x-2">
          <Icon name="clipboard-list" class="w-4 h-4 text-sky-400" />
          <h3 class="text-sm font-semibold text-slate-100">
            Session Audit Log ({appState.auditLogs.length})
          </h3>
        </div>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={clearLogs}
            class="px-2.5 py-1 text-[11px] rounded bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-white transition"
          >
            Clear
          </button>
          <button
            type="button"
            onclick={() => (appState.isAuditDrawerOpen = false)}
            class="text-slate-400 hover:text-slate-200 p-1 rounded hover:bg-slate-800 transition ml-2"
            aria-label="Close"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Search Filter -->
      <div class="p-3 border-b border-slate-800/80 bg-slate-900/30">
        <input
          type="text"
          bind:value={filterQuery}
          placeholder="Filter audit logs by SQL, database, error..."
          class="w-full bg-[#050811] border border-slate-800 rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500"
        />
      </div>

      <!-- Logs List -->
      <div class="flex-1 overflow-y-auto divide-y divide-slate-800/60 p-2 space-y-2">
        {#if filteredLogs.length === 0}
          <div class="p-12 text-center text-slate-500 text-xs">
            No queries recorded in session audit log yet.
          </div>
        {:else}
          {#each filteredLogs as log (log.id)}
            <div class="p-3 rounded-lg bg-[#050811] border border-slate-800/80 hover:border-slate-700 transition space-y-2">
              <div class="flex items-center justify-between text-[11px] text-slate-400">
                <div class="flex items-center space-x-2">
                  <span class="font-mono text-slate-500">{log.timestamp}</span>
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
                <pre class="p-2.5 bg-slate-950 border border-slate-800/80 rounded font-mono text-[11px] text-sky-200 overflow-x-auto whitespace-pre-wrap max-h-24">{log.sql}</pre>
                <button
                  type="button"
                  onclick={() => copySql(log.sql)}
                  class="absolute top-1.5 right-1.5 opacity-0 group-hover:opacity-100 px-2 py-0.5 bg-slate-800 hover:bg-slate-700 rounded text-[10px] font-mono text-slate-300 transition"
                >
                  Copy
                </button>
              </div>

              {#if log.error}
                <div class="text-[11px] text-rose-400 font-mono bg-rose-950/20 p-2 rounded border border-rose-900/40">
                  {log.error}
                </div>
              {:else if log.row_count != null || log.affected_rows != null}
                <div class="text-[10px] font-mono text-slate-500 flex space-x-3">
                  {#if log.row_count != null}
                    <span>Rows: {log.row_count.toLocaleString()}</span>
                  {/if}
                  {#if log.affected_rows != null}
                    <span>Affected: {log.affected_rows.toLocaleString()}</span>
                  {/if}
                </div>
              {/if}
            </div>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}
