<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
  import { viewportFit } from '../../actions/viewportFit';

  let {
    menu,
    onClose,
    onInspectDdl,
    onCopySelect,
    aiTablePrompt,
  }: {
    menu: { open: boolean; x: number; y: number; tableName: string } | null;
    onClose: () => void;
    onInspectDdl: (kind: 'table', name: string) => void;
    onCopySelect: (tableName: string) => void;
    aiTablePrompt: (menu: { tableName: string }) => string;
  } = $props();
</script>

{#if menu && menu.open}
  <div
    use:viewportFit={{ x: menu.x, y: menu.y }}
    style="top: {menu.y}px; left: {menu.x}px;"
    class="fixed z-50 w-60 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
  >
    <div class="px-3 py-1.5 flex items-center space-x-2 bg-slate-900/60">
      <Icon name="table" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
      <span class="text-[11px] font-bold text-slate-200 font-mono truncate">
        {menu.tableName}
      </span>
    </div>

    <div class="py-1">
      <button
        type="button"
        onclick={() => {
          appState.openTableDataTab(menu.tableName);
          onClose();
        }}
        class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
      >
        <Icon name="table" class="w-3.5 h-3.5 text-emerald-400" />
        <span>Open Data Grid</span>
      </button>
      <button
        type="button"
        onclick={() => {
          appState.openTableStructureTab(menu.tableName);
          onClose();
        }}
        class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
      >
        <Icon name="columns" class="w-3.5 h-3.5 text-sky-400" />
        <span>View Structure</span>
      </button>
      <div class="px-3 py-1 flex items-center justify-between gap-1.5">
        <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
          <Icon name="file-code" class="w-3.5 h-3.5 text-purple-400" />
          <span>DDL</span>
        </span>
        <div class="flex items-center gap-1 text-[10px]">
          <button
            type="button"
            onclick={() => {
              appState.openTableDDLTab(menu.tableName);
              onClose();
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-purple-300 font-medium transition-colors"
            title="Open CREATE TABLE in Tab"
          >Tab</button>
          <button
            type="button"
            onclick={() => {
              const tbl = menu.tableName;
              onClose();
              onInspectDdl('table', tbl);
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
            title="View DDL in Modal"
          >Modal</button>
        </div>
      </div>
    </div>

    <div class="py-1">
      <div class="px-3 py-1 flex items-center justify-between gap-1.5">
        <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
          <Icon name="download" class="w-3.5 h-3.5 text-emerald-400" />
          <span>SQL Dump</span>
        </span>
        <div class="flex items-center gap-1 text-[10px]">
          <button
            type="button"
            onclick={() => {
              appState.exportTableDump(menu.tableName);
              onClose();
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium transition-colors"
            title="Export Table SQL Dump"
          >Quick</button>
          <button
            type="button"
            onclick={() => {
              appState.openDumpModal(menu.tableName);
              onClose();
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
            title="Export Dump Options..."
          >Options...</button>
        </div>
      </div>
      <button
        type="button"
        onclick={() => {
          const tbl = menu.tableName;
          onClose();
          appState.openMockDataModal(tbl);
        }}
        class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
      >
        <Icon name="sparkles" class="w-3.5 h-3.5 text-purple-400" />
        <span>Generate Mock Data...</span>
      </button>
      <button
        type="button"
        onclick={() => {
          appState.sendToAi(aiTablePrompt(menu), 'Table context sent to AI Copilot');
          onClose();
        }}
        class="w-full text-left px-3 py-1.5 hover:bg-slate-800/80 flex items-center space-x-2 text-slate-200 transition-colors"
      >
        <Icon name="bot" class="w-3.5 h-3.5 text-sky-400" />
        <span>Send Table to AI Copilot</span>
      </button>
      <div class="px-3 py-1 flex items-center justify-between gap-1.5">
        <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
          <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400" />
          <span>Manage</span>
        </span>
        <div class="flex items-center gap-1 text-[10px]">
          <button
            type="button"
            onclick={() => {
              appState.tableToDuplicate = menu.tableName;
              appState.isDuplicateTableModalOpen = true;
              onClose();
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium transition-colors"
          >Duplicate...</button>
          <button
            type="button"
            onclick={() => {
              appState.tableToRename = menu.tableName;
              appState.isRenameTableModalOpen = true;
              onClose();
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
          >Rename...</button>
        </div>
      </div>
    </div>

    <div class="py-1 space-y-0.5">
      <div class="px-3 py-1 flex items-center justify-between gap-1.5">
        <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
          <Icon name="clipboard-list" class="w-3.5 h-3.5 text-slate-400" />
          <span>Copy</span>
        </span>
        <div class="flex items-center gap-1 text-[10px]">
          <button
            type="button"
            onclick={() => {
              copyText(menu.tableName);
              appState.showToast(`Copied table name "${menu.tableName}"`, 'info');
              onClose();
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 font-medium transition-colors"
          >Name</button>
          <button
            type="button"
            onclick={() => {
              onCopySelect(menu.tableName);
              onClose();
            }}
            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium transition-colors"
          >SELECT 100</button>
        </div>
      </div>
      <div class="px-3 py-1 flex items-center justify-between gap-1.5">
        <span class="text-[11px] text-rose-400/90 flex items-center gap-1.5">
          <Icon name="alert-triangle" class="w-3.5 h-3.5 text-amber-400" />
          <span>Danger</span>
        </span>
        <div class="flex items-center gap-1 text-[10px]">
          <button
            type="button"
            onclick={() => {
              const tbl = menu.tableName;
              onClose();
              appState.truncateTable(tbl);
            }}
            class="px-2 py-0.5 rounded bg-amber-950/60 hover:bg-amber-900/70 text-amber-300 font-medium transition-colors"
          >Truncate</button>
          <button
            type="button"
            onclick={() => {
              const tbl = menu.tableName;
              onClose();
              appState.dropTable(tbl);
            }}
            class="px-2 py-0.5 rounded bg-rose-950/60 hover:bg-rose-900/70 text-rose-300 font-medium transition-colors"
          >Drop</button>
        </div>
      </div>
    </div>
  </div>
{/if}
