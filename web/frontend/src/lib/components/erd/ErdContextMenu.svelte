<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
  import { viewportFit } from '../../actions/viewportFit';

  let {
    contextMenu = $bindable(null),
    onViewDdl,
    onGenerateLeftJoin,
    onAskAi,
  }: {
    contextMenu: { tableName: string; x: number; y: number } | null;
    onViewDdl: (tableName: string) => void;
    onGenerateLeftJoin: (tableName: string) => void;
    onAskAi: (tableName: string) => void;
  } = $props();
</script>

{#if contextMenu}
  <div class="fixed inset-0 z-40" role="presentation" onclick={() => (contextMenu = null)}></div>
  <div
    use:viewportFit={{ x: contextMenu.x, y: contextMenu.y }}
    class="erd-menu fixed z-50 w-60 bg-[#0f172a] border border-slate-700 rounded-xl shadow-2xl py-1.5 text-xs font-sans text-slate-200 divide-y divide-slate-800"
    style="left: {contextMenu.x}px; top: {contextMenu.y}px;"
  >
    <div class="px-3 py-1 text-[10.5px] font-mono font-bold text-sky-400 truncate">
      {contextMenu.tableName}
    </div>
    <div class="py-1">
      <div class="px-3 py-1 flex items-center justify-between gap-1.5">
        <span class="text-[11px] text-slate-400">Open</span>
        <div class="flex items-center gap-1 text-[10px]">
          <button
            type="button"
            onclick={() => { const t = contextMenu!.tableName; contextMenu = null; appState.openTableDataTab(t); }}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium"
          >Data</button>
          <button
            type="button"
            onclick={() => { const t = contextMenu!.tableName; contextMenu = null; appState.openTableStructureTab(t); }}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium"
          >Structure</button>
          <button
            type="button"
            onclick={() => onViewDdl(contextMenu!.tableName)}
            class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-cyan-300 font-medium"
          >DDL</button>
        </div>
      </div>
      <button
        type="button"
        onclick={() => onGenerateLeftJoin(contextMenu!.tableName)}
        class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-amber-300"
      >
        <Icon name="file-code" class="w-3.5 h-3.5 text-amber-400" />
        <span>Generate SELECT SQL (LEFT JOIN)</span>
      </button>
      <button
        type="button"
        onclick={() => onAskAi(contextMenu!.tableName)}
        class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-purple-300"
      >
        <Icon name="bot" class="w-3.5 h-3.5 text-purple-400" />
        <span>Ask AI About Schema</span>
      </button>
    </div>
    <div class="py-1">
      <button
        type="button"
        onclick={() => { copyText(contextMenu!.tableName); appState.showToast('Copied table name', 'info'); contextMenu = null; }}
        class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-400"
      >
        <Icon name="copy" class="w-3.5 h-3.5" />
        <span>Copy Table Name</span>
      </button>
    </div>
  </div>
{/if}
