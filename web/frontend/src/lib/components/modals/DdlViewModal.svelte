<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';

  function copyDDL() {
    copyText(appState.ddlViewModal.ddl);
    appState.showToast('DDL copied to clipboard', 'info');
  }

  function insertIntoEditor() {
    const ddl = appState.ddlViewModal.ddl;
    const title = appState.ddlViewModal.title || 'DDL';
    appState.ddlViewModal.isOpen = false;
    appState.insertSnippetSQL(ddl, title);
    appState.showToast('DDL inserted into editor', 'success');
  }
</script>

{#if appState.ddlViewModal.isOpen}
  <div
    id="ddl-modal"
    class="fixed inset-0 z-[95] flex items-center justify-center bg-black/75 backdrop-blur-sm p-4"
  >
    <div class="bg-[#0f172a] border border-slate-700 w-full max-w-3xl rounded-xl shadow-2xl flex flex-col overflow-hidden max-h-[85vh]">
      <div class="px-5 py-3 border-b border-slate-800 bg-slate-900/60 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <Icon name="file-code" class="w-4 h-4 text-purple-400" />
          <h3 class="text-sm font-bold text-slate-100 font-mono">{appState.ddlViewModal.title || 'Object DDL'}</h3>
        </div>
        <button
          onclick={() => (appState.ddlViewModal.isOpen = false)}
          class="p-1 text-slate-400 hover:text-white rounded hover:bg-slate-800"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-4 flex-1 overflow-auto bg-[#050811]">
        <pre class="text-xs font-mono text-emerald-300 whitespace-pre-wrap leading-relaxed select-text">{appState.ddlViewModal.ddl}</pre>
      </div>

      <div class="px-5 py-3 border-t border-slate-800 bg-slate-900/60 flex items-center justify-end gap-2">
        <button
          type="button"
          onclick={copyDDL}
          class="px-3.5 py-1.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-medium flex items-center gap-1.5 transition"
        >
          <Icon name="copy" class="w-3.5 h-3.5" />
          <span>Copy DDL</span>
        </button>
        <button
          type="button"
          onclick={insertIntoEditor}
          class="px-4 py-1.5 rounded bg-sky-500 hover:bg-sky-400 text-slate-950 text-xs font-bold flex items-center gap-1.5 transition"
        >
          <Icon name="corner-down-left" class="w-3.5 h-3.5" />
          <span>Insert to Editor</span>
        </button>
      </div>
    </div>
  </div>
{/if}
