<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface TableDiag {
    table: string;
    engine?: string;
    rows?: number;
    data_length?: number;
    index_length?: number;
    data_free?: number;
  }

  let tables = $state<TableDiag[]>([]);
  let isLoading = $state(false);
  let maintenanceStatus = $state('');

  $effect(() => {
    if (appState.isDbDiagnosticsOpen && appState.activeSessionId) {
      loadDiagnostics();
    }
  });

  async function loadDiagnostics() {
    isLoading = true;
    try {
      const res = await api.getTableDiagnostics(appState.activeSessionId!);
      tables = res.tables || (Array.isArray(res) ? res : []);
    } catch (err: any) {
      appState.showToast(`Failed to load diagnostics: ${err.message}`, 'error');
    } finally {
      isLoading = false;
    }
  }

  function formatBytes(bytes?: number): string {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
  }

  async function runMaintenance(tableName: string, op: 'OPTIMIZE' | 'ANALYZE') {
    if (!appState.activeSessionId) return;
    maintenanceStatus = `Running ${op} on ${tableName}...`;
    try {
      await api.executeMaintenance(appState.activeSessionId, appState.currentDatabase || '', tableName, op);
      appState.showToast(`Completed ${op} on ${tableName}`, 'success');
      await loadDiagnostics();
    } catch (err: any) {
      appState.showToast(`Maintenance failed: ${err.message}`, 'error');
    } finally {
      maintenanceStatus = '';
    }
  }

  function handleClose() {
    appState.isDbDiagnosticsOpen = false;
  }
</script>

{#if appState.isDbDiagnosticsOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 font-sans">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="diag-modal-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-4xl flex flex-col max-h-[88vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400">
            <Icon name="activity" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="diag-modal-title" class="font-bold text-sm text-slate-100">
              Database Storage & Health Diagnostics
            </h3>
            <p class="text-[11px] text-slate-400">
              Inspect storage sizes, fragmentation free space, and perform table optimizations
            </p>
          </div>
        </div>
        <button
          onclick={handleClose}
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Content Area -->
      <div class="flex-1 overflow-auto">
        {#if isLoading}
          <div class="p-12 text-center text-slate-400 flex flex-col items-center gap-2">
            <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
            <span>Analyzing database storage statistics...</span>
          </div>
        {:else}
          <table class="w-full text-left border-collapse text-xs font-mono">
            <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-400 text-[10.5px] uppercase tracking-wider sticky top-0 font-sans z-10">
              <tr>
                <th class="py-2 px-3">Table Name</th>
                <th class="py-2 px-3">Engine</th>
                <th class="py-2 px-3 text-right">Rows Est.</th>
                <th class="py-2 px-3 text-right">Data Size</th>
                <th class="py-2 px-3 text-right">Index Size</th>
                <th class="py-2 px-3 text-right">Free / Fragmented</th>
                <th class="py-2 px-3 text-center font-sans">Maintenance</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#1e293b]/60">
              {#if tables.length === 0}
                <tr>
                  <td colspan="7" class="p-8 text-center text-slate-500 font-sans italic">
                    No table storage statistics found for current database.
                  </td>
                </tr>
              {:else}
                {#each tables as t}
                  <tr class="hover:bg-slate-800/30 transition-colors">
                    <td class="py-2 px-3 font-bold text-slate-100 font-mono">{t.table}</td>
                    <td class="py-2 px-3 text-slate-400">{t.engine || 'InnoDB'}</td>
                    <td class="py-2 px-3 text-right text-slate-300">{(t.rows || 0).toLocaleString()}</td>
                    <td class="py-2 px-3 text-right text-sky-400 font-semibold">{formatBytes(t.data_length)}</td>
                    <td class="py-2 px-3 text-right text-emerald-400">{formatBytes(t.index_length)}</td>
                    <td class="py-2 px-3 text-right {(t.data_free || 0) > 1024 * 1024 ? 'text-amber-400 font-bold' : 'text-slate-400'}">
                      {formatBytes(t.data_free)}
                    </td>
                    <td class="py-2 px-3 text-center font-sans space-x-1">
                      <button
                        onclick={() => runMaintenance(t.table, 'OPTIMIZE')}
                        class="px-2 py-0.5 rounded bg-sky-950/60 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[10px] font-semibold transition"
                        title="Reclaim fragmented disk space"
                      >
                        Optimize
                      </button>
                      <button
                        onclick={() => runMaintenance(t.table, 'ANALYZE')}
                        class="px-2 py-0.5 rounded bg-emerald-950/60 hover:bg-emerald-900 border border-emerald-800 text-emerald-300 text-[10px] font-semibold transition"
                        title="Update query optimizer key distribution stats"
                      >
                        Analyze
                      </button>
                    </td>
                  </tr>
                {/each}
              {/if}
            </tbody>
          </table>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0 font-sans text-xs">
        <span class="text-slate-400">{maintenanceStatus}</span>
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
