<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let {
    digestRows = [],
    isDigestLoading = false,
    digestError = '',
    onOpenSqlInEditor,
  }: {
    digestRows: any[];
    isDigestLoading: boolean;
    digestError: string;
    onOpenSqlInEditor: (sql?: string, title?: string) => void;
  } = $props();
</script>

    <!-- Statement Digest View -->
    <div class="flex-1 overflow-auto">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] sticky top-0 z-10 border-b border-[#334155] text-slate-400 text-[11px] uppercase tracking-wider font-sans select-none">
          <tr>
            <th class="py-2 px-3 w-12 text-right">#</th>
            <th class="py-2 px-3">Normalized Statement</th>
            <th class="py-2 px-3 w-20 text-center">Scan</th>
            <th class="py-2 px-3 w-20 text-right">Exec</th>
            <th class="py-2 px-3 w-24 text-right">Total (ms)</th>
            <th class="py-2 px-3 w-24 text-right">Avg (ms)</th>
            <th class="py-2 px-3 w-24 text-right">Max (ms)</th>
            <th class="py-2 px-3 w-24 text-right">Lock (ms)</th>
            <th class="py-2 px-3 w-24 text-right">Rows Sent</th>
            <th class="py-2 px-3 w-24 text-right">Examined</th>
            <th class="py-2 px-3 w-28 text-center">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#if isDigestLoading}
            <tr>
              <td colspan="11" class="p-8 text-center text-slate-500">
                Aggregating slow query digest from performance_schema...
              </td>
            </tr>
          {:else if digestError}
            <tr>
              <td colspan="11" class="p-8">
                <div class="flex flex-col items-center gap-2 text-center">
                  <Icon name="alert-triangle" class="w-5 h-5 text-amber-400" />
                  <p class="text-sm font-semibold text-amber-300">Statement digest unavailable</p>
                  <p class="text-xs text-slate-400 max-w-2xl font-mono break-words">{digestError}</p>
                  <button
                    type="button"
                    onclick={loadStatementDigest}
                    class="mt-1 px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-semibold"
                  >
                    Retry
                  </button>
                </div>
              </td>
            </tr>
          {:else if digestRows.length === 0}
            <tr>
              <td colspan="11" class="p-8 text-center text-slate-500 space-y-1">
                <p class="text-sm">No statement digests recorded yet on this server.</p>
                <p class="text-[11px] text-slate-600">
                  performance_schema is enabled but has not aggregated any statement yet — run some
                  traffic, then hit refresh.
                </p>
              </td>
            </tr>
          {:else}
            {#each digestRows as row, dIdx}
              {@const stmt = row.query || row.digest_text || '-'}
              {@const totalMs = row.total_latency_ms ?? row.sum_timer_wait ?? row.total_time}
              {@const avgMs = row.avg_latency_ms ?? row.avg_timer_wait ?? row.avg_latency}
              {@const maxMs = row.max_latency_ms ?? row.max_timer_wait ?? row.max_latency}
              {@const lockMs = row.lock_time_ms ?? 0}
              <tr class="hover:bg-slate-800/40 transition-colors">
                <td class="py-2 px-3 text-right text-slate-500 tabular-nums">{dIdx + 1}</td>
                <td class="py-2 px-3 text-slate-200 font-mono text-[11px] truncate max-w-md" title={stmt}>
                  {stmt}
                </td>
                <td class="py-2 px-3 text-center">
                  {#if row.full_scan}
                    <span class="px-1.5 py-0.5 rounded bg-rose-950/70 border border-rose-800 text-rose-300 text-[9.5px] font-sans font-bold uppercase" title="Full table scan / no index used ({row.no_index_used ?? 0}x)">FULL SCAN</span>
                  {:else}
                    <span class="text-[10px] text-emerald-400 font-sans">Indexed</span>
                  {/if}
                </td>
                <td class="py-2 px-3 text-right font-bold text-sky-300 tabular-nums">{(row.exec_count ?? row.count_star ?? 0).toLocaleString()}</td>
                <td class="py-2 px-3 text-right text-slate-300 tabular-nums">{typeof totalMs === 'number' ? totalMs.toFixed(1) : (totalMs ?? '-')}</td>
                <td class="py-2 px-3 text-right font-semibold text-amber-300 tabular-nums">{typeof avgMs === 'number' ? avgMs.toFixed(2) : (avgMs ?? '-')}</td>
                <td class="py-2 px-3 text-right font-bold text-rose-400 tabular-nums">{typeof maxMs === 'number' ? maxMs.toFixed(1) : (maxMs ?? '-')}</td>
                <td class="py-2 px-3 text-right text-slate-400 tabular-nums">{typeof lockMs === 'number' ? lockMs.toFixed(1) : '-'}</td>
                <td class="py-2 px-3 text-right text-emerald-300/90 tabular-nums">{(row.rows_sent ?? 0).toLocaleString()}</td>
                <td class="py-2 px-3 text-right text-slate-400 tabular-nums">{(row.rows_examined ?? row.sum_rows_examined ?? 0).toLocaleString()}</td>
                <td class="py-2 px-3 text-center whitespace-nowrap">
                  <div class="inline-flex items-center space-x-1 font-sans">
                    <button
                      type="button"
                      onclick={() => copyWithToast(stmt, 'Digest SQL')}
                      class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] border border-slate-700"
                    >
                      Copy
                    </button>
                    <button
                      type="button"
                      onclick={() => openSqlInEditor(stmt, `Digest_${dIdx + 1}.sql`)}
                      class="px-1.5 py-0.5 rounded bg-sky-500 hover:bg-sky-400 text-slate-950 font-bold text-[10px]"
                    >
                      Editor
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
