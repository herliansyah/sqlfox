<script lang="ts">
  import type { TableMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableMeta: TableMetadata | null;
  }

  let { tableMeta }: Props = $props();
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-auto p-4 select-text font-mono text-xs">
  {#if tableMeta && tableMeta.indexes && tableMeta.indexes.length > 0}
    <div class="border border-[#1e293b] rounded-xl overflow-hidden shadow-xl bg-[#0b0f19]">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans">
          <tr>
            <th class="py-2.5 px-4">#</th>
            <th class="py-2.5 px-4">Index Name</th>
            <th class="py-2.5 px-4">Indexed Column(s)</th>
            <th class="py-2.5 px-4">Unique</th>
            <th class="py-2.5 px-4">Primary</th>
            <th class="py-2.5 px-4">Index Type</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#each tableMeta.indexes as idx, i}
            <tr class="hover:bg-sky-500/5 transition-colors">
              <td class="py-2 px-4 text-slate-500">{i + 1}</td>
              <td class="py-2 px-4 font-bold text-slate-100 flex items-center space-x-1.5">
                <Icon name="key" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                <span>{idx.name}</span>
              </td>
              <td class="py-2 px-4 text-sky-300 font-semibold">
                {Array.isArray(idx.columns) ? idx.columns.join(', ') : idx.columns}
              </td>
              <td class="py-2 px-4">
                {#if idx.is_unique}
                  <span class="text-amber-400 font-semibold">UNIQUE</span>
                {:else}
                  <span class="text-slate-500">NO</span>
                {/if}
              </td>
              <td class="py-2 px-4">
                {#if idx.is_primary}
                  <span class="text-emerald-400 font-semibold">YES</span>
                {:else}
                  <span class="text-slate-600">-</span>
                {/if}
              </td>
              <td class="py-2 px-4 text-slate-400">{idx.type || 'BTREE'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else}
    <div class="p-8 text-center text-slate-500">
      No indexes defined for this table.
    </div>
  {/if}
</div>
