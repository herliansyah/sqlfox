<script lang="ts">
  import type { TableMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableMeta: TableMetadata | null;
  }

  let { tableMeta }: Props = $props();
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-auto p-4 select-text font-mono text-xs">
  {#if tableMeta && tableMeta.columns && tableMeta.columns.length > 0}
    <div class="border border-[#1e293b] rounded-xl overflow-hidden shadow-xl bg-[#0b0f19]">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans">
          <tr>
            <th class="py-2.5 px-4">#</th>
            <th class="py-2.5 px-4">Column Name</th>
            <th class="py-2.5 px-4">Data Type</th>
            <th class="py-2.5 px-4">Nullable</th>
            <th class="py-2.5 px-4">Key</th>
            <th class="py-2.5 px-4">Default Value</th>
            <th class="py-2.5 px-4">Extra</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#each tableMeta.columns as col, idx}
            <tr class="hover:bg-sky-500/5 transition-colors">
              <td class="py-2 px-4 text-slate-500">{idx + 1}</td>
              <td class="py-2 px-4 font-bold text-slate-100 flex items-center space-x-1.5">
                {#if col.is_primary_key}
                  <Icon name="key" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
                {/if}
                <span>{col.name}</span>
              </td>
              <td class="py-2 px-4 text-sky-400">{col.type}</td>
              <td class="py-2 px-4">
                {#if col.is_nullable}
                  <span class="text-emerald-400">YES</span>
                {:else}
                  <span class="text-rose-400 font-semibold">NO</span>
                {/if}
              </td>
              <td class="py-2 px-4">
                {#if col.is_primary_key}
                  <span class="px-1.5 py-0.2 rounded bg-amber-950/60 border border-amber-800 text-amber-300 text-[10px] font-bold">
                    PRI
                  </span>
                {:else}
                  <span class="text-slate-600">-</span>
                {/if}
              </td>
              <td class="py-2 px-4 text-slate-400">
                {col.default_value !== undefined && col.default_value !== null ? col.default_value : '-'}
              </td>
              <td class="py-2 px-4 text-slate-500 text-[11px]">
                {col.extra || col.comment || '-'}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else}
    <div class="p-8 text-center text-slate-500">
      No column structure details found for this table.
    </div>
  {/if}
</div>
