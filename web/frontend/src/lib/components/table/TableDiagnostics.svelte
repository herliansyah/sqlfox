<script lang="ts">
  import type { TableMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableMeta: TableMetadata | null;
  }

  let { tableMeta }: Props = $props();

  function formatBytes(bytes?: number): string {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-auto p-4 select-text text-xs font-sans">
  {#if tableMeta}
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Estimated Rows</span>
        <div class="text-xl font-bold font-mono text-slate-100">
          {(tableMeta.row_count ?? tableMeta.rows_count ?? 0).toLocaleString()}
        </div>
      </div>
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Data Size</span>
        <div class="text-xl font-bold font-mono text-sky-400">
          {formatBytes(tableMeta.data_length)}
        </div>
      </div>
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Columns Count</span>
        <div class="text-xl font-bold font-mono text-emerald-400">
          {tableMeta.columns?.length || 0}
        </div>
      </div>
      <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 space-y-1">
        <span class="text-[10px] uppercase font-bold text-slate-500 tracking-wider">Indexes Count</span>
        <div class="text-xl font-bold font-mono text-amber-400">
          {tableMeta.indexes?.length || 0}
        </div>
      </div>
    </div>

    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-xl p-5 space-y-3">
      <h4 class="font-bold text-sm text-slate-200">Table Overview & Properties</h4>
      <div class="divide-y divide-[#1e293b]/60 font-mono text-xs">
        <div class="py-2.5 flex justify-between">
          <span class="text-slate-400">Table Name</span>
          <span class="text-slate-100 font-semibold">{tableMeta.name}</span>
        </div>
        <div class="py-2.5 flex justify-between">
          <span class="text-slate-400">Object Type</span>
          <span class="text-slate-100">{tableMeta.type}</span>
        </div>
        <div class="py-2.5 flex justify-between">
          <span class="text-slate-400">Primary Key</span>
          <span class="text-amber-300">{tableMeta.primary_key ? tableMeta.primary_key.join(', ') : 'None'}</span>
        </div>
        {#if tableMeta.comment}
          <div class="py-2.5 flex justify-between">
            <span class="text-slate-400">Table Comment</span>
            <span class="text-slate-200 italic">{tableMeta.comment}</span>
          </div>
        {/if}
      </div>
    </div>
  {:else}
    <div class="p-8 text-center text-slate-500">
      No diagnostics available.
    </div>
  {/if}
</div>
