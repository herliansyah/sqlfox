<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let modal = $derived(appState.safetyInspectionModal);
  let insp = $derived(modal.inspection);

  function riskBadgeClass(level?: string) {
    switch ((level || '').toUpperCase()) {
      case 'CRITICAL':
      case 'DANGEROUS':
        return 'bg-rose-500 text-slate-950';
      case 'HIGH':
      case 'WARNING':
        return 'bg-amber-500 text-slate-950';
      case 'MEDIUM':
        return 'bg-yellow-500/30 text-yellow-200 border border-yellow-500/50';
      default:
        return 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/40';
    }
  }
</script>

{#if modal.isOpen && insp}
  <div
    id="safety-inspection-dialog"
    class="fixed inset-0 z-[95] flex items-center justify-center bg-black/75 p-4"
  >
    <div class="bg-[#0f172a] border border-slate-700 w-full max-w-xl rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[85vh]">
      <div class="px-5 py-3.5 border-b border-slate-800 bg-slate-900/60 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <Icon name="shield" class="w-4 h-4 text-sky-400" />
          <h3 class="text-sm font-bold text-slate-100">Pre-Flight Safety Guard Inspection</h3>
        </div>
        <span class="px-2 py-0.5 rounded text-[10px] font-bold uppercase {riskBadgeClass(insp.risk_level)}">
          {insp.risk_level || 'LOW'}
        </span>
      </div>

      <div class="p-5 space-y-3.5 text-xs overflow-y-auto">
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 p-3 rounded-lg bg-[#050811] border border-slate-800">
          <div>
            <div class="text-[10px] text-slate-500 uppercase font-semibold">Destructive</div>
            <div class="font-bold mt-0.5 {insp.is_destructive ? 'text-rose-400' : 'text-emerald-400'}">
              {insp.is_destructive ? 'YES' : 'NO'}
            </div>
          </div>
          <div>
            <div class="text-[10px] text-slate-500 uppercase font-semibold">Challenge Gate</div>
            <div class="font-bold mt-0.5 {insp.requires_challenge ? 'text-rose-400' : insp.requires_affirmation ? 'text-amber-300' : 'text-slate-300'}">
              {insp.requires_challenge ? 'Typed Required' : insp.requires_affirmation ? 'Affirmation' : 'None'}
            </div>
          </div>
          <div>
            <div class="text-[10px] text-slate-500 uppercase font-semibold">Target Table</div>
            <div class="font-mono text-sky-300 font-semibold mt-0.5 truncate">{insp.target_table || '-'}</div>
          </div>
          <div>
            <div class="text-[10px] text-slate-500 uppercase font-semibold">Target DB</div>
            <div class="font-mono text-slate-200 font-semibold mt-0.5 truncate">{insp.target_database || appState.currentDatabase || '-'}</div>
          </div>
        </div>

        <div class="p-3 rounded-lg bg-slate-900/60 border border-slate-800 space-y-1">
          <div class="text-[10px] text-slate-500 uppercase font-semibold">Audit Analysis</div>
          <p class="text-slate-200 leading-relaxed">{insp.reason || 'Query passed deterministic AST safety analysis.'}</p>
          {#if insp.suggestion}
            <p class="text-sky-300 text-[11px] pt-1">Tip: {insp.suggestion}</p>
          {/if}
        </div>

        <div>
          <div class="text-[10px] text-slate-500 uppercase font-semibold mb-1">Inspected Statement</div>
          <pre class="p-3 rounded-lg bg-[#050811] border border-slate-800 font-mono text-[11px] text-slate-300 overflow-x-auto whitespace-pre-wrap max-h-48">{modal.sql}</pre>
        </div>
      </div>

      <div class="px-5 py-3 border-t border-slate-800 bg-slate-900/60 flex items-center justify-end gap-2">
        <button
          onclick={() => (appState.safetyInspectionModal.isOpen = false)}
          class="px-4 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
