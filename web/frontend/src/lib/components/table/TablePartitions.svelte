<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableName: string;
  }

  interface PartitionRow {
    name: string;
    method?: string;
    expression?: string;
    description?: string;
    table_rows?: number;
    data_length?: number;
    index_length?: number;
  }

  let { tableName }: Props = $props();

  let partitions = $state<PartitionRow[]>([]);
  let isLoading = $state(false);
  let errorMessage = $state<string | null>(null);

  function formatBytes(bytes?: number): string {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  let totalRows = $derived(
    partitions.reduce((acc, p) => acc + (p.table_rows || 0), 0)
  );

  let totalDataLength = $derived(
    partitions.reduce((acc, p) => acc + (p.data_length || 0), 0)
  );

  let totalIndexLength = $derived(
    partitions.reduce((acc, p) => acc + (p.index_length || 0), 0)
  );

  async function loadPartitions() {
    if (!appState.activeConnection || !tableName) return;
    isLoading = true;
    errorMessage = null;

    try {
      const data = await api.getTablePartitions(
        appState.activeConnection.id,
        tableName,
        appState.currentDatabase || undefined
      );
      partitions = Array.isArray(data) ? data : [];
    } catch (err: any) {
      errorMessage = err.message || 'Failed to load partition metadata';
      partitions = [];
    } finally {
      isLoading = false;
    }
  }

  $effect(() => {
    if (tableName && appState.activeConnection) {
      loadPartitions();
    }
  });
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-auto p-4 select-text font-mono text-xs">
  <div class="flex items-center justify-between mb-4 flex-shrink-0">
    <div class="flex items-center space-x-2">
      <Icon name="layers" class="w-4 h-4 text-sky-400" />
      <span class="font-sans font-bold text-slate-200 text-sm">Table Partitions</span>
      <span class="text-slate-500 font-mono text-xs">({partitions.length} partitions)</span>
    </div>
    <button
      onclick={loadPartitions}
      disabled={isLoading}
      class="px-2.5 py-1 rounded bg-[#1e293b] hover:bg-[#334155] text-slate-300 hover:text-white transition flex items-center space-x-1.5 border border-[#334155]"
    >
      <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin text-sky-400' : ''}" />
      <span>Refresh</span>
    </button>
  </div>

  {#if isLoading}
    <div class="p-12 text-center text-slate-500 flex flex-col items-center justify-center space-y-2">
      <Icon name="loader" class="w-6 h-6 animate-spin text-sky-400" />
      <span>Inspecting partition metadata...</span>
    </div>
  {:else if errorMessage}
    <div class="p-4 rounded-xl bg-rose-950/30 border border-rose-800 text-rose-300">
      <div class="font-bold flex items-center space-x-1.5 mb-1">
        <Icon name="alert-triangle" class="w-4 h-4 text-rose-400" />
        <span>Failed to load partitions</span>
      </div>
      <p class="text-xs text-rose-400">{errorMessage}</p>
    </div>
  {:else if partitions.length > 0}
    <div class="border border-[#1e293b] rounded-xl overflow-hidden shadow-xl bg-[#0b0f19] mb-4">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans">
          <tr>
            <th class="py-2.5 px-4 w-12">#</th>
            <th class="py-2.5 px-4">Partition Name</th>
            <th class="py-2.5 px-4">Method</th>
            <th class="py-2.5 px-4">Expression</th>
            <th class="py-2.5 px-4">Values / Description</th>
            <th class="py-2.5 px-4 text-right">Rows</th>
            <th class="py-2.5 px-4 text-right">Data Size</th>
            <th class="py-2.5 px-4 text-right">Index Size</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#each partitions as part, idx}
            <tr class="hover:bg-sky-500/5 transition-colors">
              <td class="py-2 px-4 text-slate-500">{idx + 1}</td>
              <td class="py-2 px-4 font-bold text-sky-300 flex items-center space-x-1.5">
                <Icon name="layers" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                <span>{part.name || `p${idx}`}</span>
              </td>
              <td class="py-2 px-4 text-slate-300">
                <span class="px-1.5 py-0.5 rounded bg-slate-800 border border-slate-700 text-slate-300 text-[10px] font-mono">
                  {part.method || 'RANGE'}
                </span>
              </td>
              <td class="py-2 px-4 text-slate-400 font-mono text-[11px]">
                {part.expression || '-'}
              </td>
              <td class="py-2 px-4 text-amber-300 font-mono text-[11px]">
                {part.description || '-'}
              </td>
              <td class="py-2 px-4 text-right text-slate-200">
                {(part.table_rows || 0).toLocaleString()}
              </td>
              <td class="py-2 px-4 text-right text-emerald-400">
                {formatBytes(part.data_length)}
              </td>
              <td class="py-2 px-4 text-right text-cyan-400">
                {formatBytes(part.index_length)}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Summary Metrics Cards -->
    <div class="grid grid-cols-4 gap-3 text-xs">
      <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-lg">
        <span class="text-slate-500 text-[10px] uppercase block font-sans">Total Partitions</span>
        <span class="text-base font-bold text-slate-100">{partitions.length}</span>
      </div>
      <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-lg">
        <span class="text-slate-500 text-[10px] uppercase block font-sans">Total Estimated Rows</span>
        <span class="text-base font-bold text-slate-100">{totalRows.toLocaleString()}</span>
      </div>
      <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-lg">
        <span class="text-slate-500 text-[10px] uppercase block font-sans">Total Data Length</span>
        <span class="text-base font-bold text-emerald-400">{formatBytes(totalDataLength)}</span>
      </div>
      <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-lg">
        <span class="text-slate-500 text-[10px] uppercase block font-sans">Total Index Length</span>
        <span class="text-base font-bold text-cyan-400">{formatBytes(totalIndexLength)}</span>
      </div>
    </div>
  {:else}
    <div class="p-12 text-center text-slate-500 bg-[#0b0f19] border border-[#1e293b] rounded-xl flex flex-col items-center justify-center space-y-3">
      <Icon name="layers" class="w-8 h-8 text-slate-600" />
      <span class="text-slate-300 font-semibold font-sans">No Partitions Found</span>
      <p class="text-xs text-slate-500 max-w-md">
        Table <code class="text-sky-300 bg-slate-900 px-1 rounded">{tableName}</code> is not partitioned, or the storage engine does not support partition telemetry.
      </p>
    </div>
  {/if}
</div>
