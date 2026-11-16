<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let isExecuting = $state(false);
  let elapsedSec = $state(0);
  let preflightData = $state<any>(null);
  let isPreflighting = $state(false);

  async function runPreflight() {
    if (!appState.activeSessionId || !appState.ddlReview.tableName) return;
    isPreflighting = true;
    try {
      preflightData = await api.preflightDDL(
        appState.activeSessionId,
        appState.ddlReview.tableName,
        appState.ddlReview.database || appState.currentDatabase || '',
        appState.ddlReview.sql
      );
    } catch (err: any) {
      appState.showToast(`Preflight check failed: ${err.message}`, 'warning');
    } finally {
      isPreflighting = false;
    }
  }

  $effect(() => {
    if (appState.ddlReview.isOpen) {
      preflightData = null;
      elapsedSec = 0;
    }
  });

  async function handleConfirm() {
    if (isExecuting) return;
    isExecuting = true;
    elapsedSec = 0;
    const started = performance.now();
    const timer = setInterval(() => {
      elapsedSec = (performance.now() - started) / 1000;
    }, 100);
    try {
      if (appState.ddlReview.onConfirm) {
        await appState.ddlReview.onConfirm();
      }
      appState.closeDDLReviewModal();
    } catch (err: any) {
      appState.showToast(`DDL execution failed: ${err.message}`, 'error');
    } finally {
      clearInterval(timer);
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
        <div class="p-3 rounded-lg bg-amber-950/30 border border-amber-700/40 text-xs text-amber-200 flex items-start gap-2">
          <Icon name="shield-alert" class="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
          <div>
            <p class="font-semibold text-amber-300">Safety Guard & Pre-DDL Backup Notice</p>
            <p class="text-[11px] text-amber-200/80 mt-0.5">
              Destructive schema changes automatically capture a local pre-DDL snapshot before applying. Please verify the generated SQL below.
            </p>
          </div>
        </div>

        <div class="relative">
          <pre class="p-4 bg-[#050811] border border-slate-800 rounded-lg text-emerald-400 font-mono text-xs overflow-x-auto whitespace-pre-wrap leading-relaxed max-h-[320px]">{appState.ddlReview.sql}</pre>
          <button
            type="button"
            onclick={copyDDL}
            class="absolute top-2 right-2 px-2 py-1 bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 rounded text-[11px] font-mono transition"
          >
            Copy
          </button>
        </div>

        {#if appState.ddlReview.tableName}
          <div class="flex items-center justify-between pt-1">
            <button
              type="button"
              onclick={runPreflight}
              disabled={isPreflighting}
              class="px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-sky-300 text-[11px] font-medium flex items-center gap-1.5 transition"
            >
              <Icon name={isPreflighting ? 'loader' : 'activity'} class="w-3.5 h-3.5 {isPreflighting ? 'animate-spin' : ''}" />
              <span>Run Lock & OSC Preflight Check</span>
            </button>
          </div>

          {#if preflightData}
            <div class="p-3 rounded-lg bg-[#050811] border border-slate-800 space-y-2 text-[11px]">
              <div class="flex items-center justify-between">
                <span class="font-semibold text-slate-300">Preflight Telemetry ({preflightData.table})</span>
                <span class="text-slate-400">
                  Locks: <strong class="text-amber-300">{preflightData.active_locks?.length || 0}</strong> ·
                  FKs: <strong class="text-sky-300">{preflightData.relationships?.length || 0}</strong>
                </span>
              </div>
              {#if preflightData.osc_ghost}
                <div>
                  <div class="text-[10px] text-slate-500 mb-0.5">gh-ost Zero-Downtime Equivalent:</div>
                  <code class="block p-2 rounded bg-slate-900 text-emerald-300 font-mono text-[10px] overflow-x-auto">{preflightData.osc_ghost}</code>
                </div>
              {/if}
              {#if preflightData.osc_pt}
                <div>
                  <div class="text-[10px] text-slate-500 mb-0.5">pt-online-schema-change Equivalent:</div>
                  <code class="block p-2 rounded bg-slate-900 text-cyan-300 font-mono text-[10px] overflow-x-auto">{preflightData.osc_pt}</code>
                </div>
              {/if}
            </div>
          {/if}
        {/if}
      </div>

      <!-- Modal Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-between gap-2.5">
        <div class="text-[11px] font-mono text-slate-400">
          {#if isExecuting}
            <span class="text-amber-300">Executing DDL... ({elapsedSec.toFixed(1)}s)</span>
          {/if}
        </div>
        <div class="flex items-center space-x-2.5">
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
              <span>Executing ({elapsedSec.toFixed(1)}s)...</span>
            {:else}
              <span>{appState.ddlReview.confirmLabel || 'Execute DDL'}</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
