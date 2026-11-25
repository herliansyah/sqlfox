<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let {
    val,
    colName,
    colType,
    rowIndex,
  }: {
    val: any;
    colName: string;
    colType?: string;
    rowIndex: number;
  } = $props();

  let detailPrettyJson = $state(false);

  let safeValStr = $derived.by(() => {
    if (val === null) return 'NULL';
    if (typeof val === 'object') return JSON.stringify(val, null, 2);
    const s = String(val ?? '');
    if (detailPrettyJson) {
      try {
        return JSON.stringify(JSON.parse(s), null, 2);
      } catch {
        return s;
      }
    }
    return s;
  });

  function copyCellValue() {
    const rawCopy =
      val === null || val === undefined
        ? ''
        : typeof val === 'object'
          ? JSON.stringify(val, null, 2)
          : String(val);
    copyText(rawCopy);
    appState.showToast('Copied cell value', 'info');
  }
</script>

<div class="h-36 bg-[#0b0f19] border-t border-[#334155] flex flex-col p-2.5 z-20 shrink-0">
  <div class="flex items-center justify-between text-[11px] text-slate-400 mb-1.5 select-none">
    <div class="flex items-center space-x-2">
      <span class="font-bold text-slate-200">
        Cell: <span class="text-sky-400 font-mono">{colName}</span>
      </span>
      {#if colType}
        <span class="px-1.5 py-0.2 rounded bg-slate-900 border border-slate-800 font-mono text-[9.5px] uppercase text-slate-400">
          {colType}
        </span>
      {/if}
      <span class="text-[10px] font-mono text-slate-500">Row #{rowIndex + 1}</span>
    </div>
    <div class="flex items-center space-x-2">
      <button
        type="button"
        onclick={() => (detailPrettyJson = !detailPrettyJson)}
        class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-[10px] {detailPrettyJson ? 'text-purple-300 border-purple-700' : 'text-slate-300'}"
      >
        {detailPrettyJson ? 'Raw' : 'Format JSON'}
      </button>
      <button
        type="button"
        onclick={copyCellValue}
        class="hover:text-white flex items-center space-x-1"
      >
        <Icon name="copy" class="w-3 h-3" />
        <span>Copy</span>
      </button>
    </div>
  </div>
  <textarea
    readonly
    class="flex-1 bg-[#090d16] border border-[#1e293b] rounded p-2 text-xs font-mono text-slate-200 resize-none outline-none"
    value={safeValStr}
  ></textarea>
</div>
