<script lang="ts">
  import type { TableMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableMeta: TableMetadata | null;
  }

  let { tableMeta }: Props = $props();
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-auto p-4 select-text font-mono text-xs">
  {#if tableMeta && tableMeta.foreign_keys && tableMeta.foreign_keys.length > 0}
    <div class="border border-[#1e293b] rounded-xl overflow-hidden shadow-xl bg-[#0b0f19]">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-300 text-[11px] uppercase tracking-wider font-sans">
          <tr>
            <th class="py-2.5 px-4">#</th>
            <th class="py-2.5 px-4">Constraint Name</th>
            <th class="py-2.5 px-4">Source Column</th>
            <th class="py-2.5 px-4">Referenced Table</th>
            <th class="py-2.5 px-4">Referenced Column</th>
            <th class="py-2.5 px-4">On Update</th>
            <th class="py-2.5 px-4">On Delete</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#each tableMeta.foreign_keys as fk, i}
            <tr class="hover:bg-sky-500/5 transition-colors">
              <td class="py-2 px-4 text-slate-500">{i + 1}</td>
              <td class="py-2 px-4 font-bold text-slate-100 flex items-center space-x-1.5">
                <Icon name="link" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
                <span>{fk.name || fk.constraint_name || `fk_${i + 1}`}</span>
              </td>
              <td class="py-2 px-4 text-sky-400">{fk.column || fk.column_name}</td>
              <td class="py-2 px-4 text-emerald-400 font-semibold">{fk.referenced_table}</td>
              <td class="py-2 px-4 text-sky-400">{fk.referenced_column}</td>
              <td class="py-2 px-4 text-slate-400">{fk.on_update || 'NO ACTION'}</td>
              <td class="py-2 px-4 text-slate-400">{fk.on_delete || 'NO ACTION'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else}
    <div class="p-8 text-center text-slate-500">
      No foreign keys defined for this table.
    </div>
  {/if}
</div>
