<script lang="ts">
import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let isExecuting = $state(false);

  async function handleConfirm() {
    if (isExecuting) return;
    isExecuting = true;
    try {
      if (appState.ddlReview.onConfirm) {
        await appState.ddlReview.onConfirm();
      }
      appState.closeDDLReviewModal();
    } catch (err: any) {
      appState.showToast(`DDL execution failed: ${err.message}`, 'error');
    } finally {
      isExecuting = false;
    }
  }

  function handleCancel() {
    if (isExecuting) return;
    appState.closeDDLReviewModal();
  }

  function copyDDL() {
    copyText(appState.ddlReview.sql);
    appState.showToast('DDL copied to clipboard', 'info');
  }
</script>

{#if appState.ddlReview.isOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-2xl rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100 max-h-[85vh]"
      role="dialog"
      aria-modal="true"
    >
      <!-- Modal Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center space-x-2">
          <Icon name="zap" class="w-4 h-4 text-amber-400" />
          <h3 class="text-sm font-semibold text-slate-100">
            {appState.ddlReview.title || 'Pre-Execution DDL Review'}
          </h3>
        </div>
        <button
          type="button"
          onclick={handleCancel}
          class="text-slate-400 hover:text-slate-200 p-1 rounded hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Modal Body -->
      <div class="p-5 overflow-y-auto space-y-4 flex-1">
        <p class="text-xs text-slate-400">
          Please carefully review the generated DDL statements before executing them against the target database:
        </p>

        <div class="relative">
          <pre class="p-4 bg-[#050811] border border-slate-800 rounded-lg text-emerald-400 font-mono text-xs overflow-x-auto whitespace-pre-wrap leading-relaxed max-h-[350px]">{appState.ddlReview.sql}</pre>
          <button
            type="button"
            onclick={copyDDL}
            class="absolute top-2 right-2 px-2 py-1 bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 rounded text-[11px] font-mono transition"
          >
            Copy
          </button>
        </div>
      </div>

      <!-- Modal Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-end space-x-2.5">
        <button
          type="button"
          onclick={handleCancel}
          disabled={isExecuting}
          class="px-4 py-1.5 rounded text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition disabled:opacity-50"
        >
          Cancel
        </button>

        <button
          type="button"
          onclick={handleConfirm}
          disabled={isExecuting}
          class="px-5 py-1.5 rounded text-xs font-bold bg-amber-600 hover:bg-amber-500 text-white transition shadow flex items-center space-x-1.5 disabled:opacity-50"
        >
          {#if isExecuting}
            <Icon name="loader" class="w-3.5 h-3.5 animate-spin" />
            <span>Executing...</span>
          {:else}
            <span>Execute DDL</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
