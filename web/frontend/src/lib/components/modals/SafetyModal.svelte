<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let typedAffirmation = $state('');

  let challenge = $derived(appState.safetyChallenge);
  let inspection = $derived(challenge.inspection);
  let targetTable = $derived(inspection?.target_table || '');

  let isAffirmationMatched = $derived(
    targetTable ? typedAffirmation.trim().toLowerCase() === targetTable.toLowerCase() : true
  );

  function handleConfirm() {
    if (!isAffirmationMatched) return;
    appState.confirmSafetyChallenge();
    typedAffirmation = '';
  }

  function handleCancel() {
    appState.cancelSafetyChallenge();
    typedAffirmation = '';
  }
</script>

{#if challenge.isOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md p-4 select-none">
    <div class="bg-[#0f172a] border-2 border-red-500 rounded-xl shadow-2xl max-w-lg w-full overflow-hidden flex flex-col animate-in fade-in zoom-in-95 duration-150">
      <!-- Modal Header -->
      <div class="bg-red-950/60 border-b border-red-500/40 px-5 py-3.5 flex items-center justify-between">
        <div class="flex items-center space-x-2 text-red-400 font-bold text-sm tracking-wide">
          <Icon name="shield-alert" class="w-5 h-5 text-red-400 shrink-0" />
          <span>DETERMINISTIC SAFETY GUARD™</span>
        </div>
        <span class="text-[10px] font-bold px-2 py-0.5 rounded bg-red-500 text-slate-950 uppercase tracking-wider">
          {inspection?.risk_level || 'CRITICAL'}
        </span>
      </div>

      <!-- Content -->
      <div class="p-5 space-y-4 text-xs">
        <p class="text-slate-200 leading-relaxed font-sans">
          {inspection?.reason || 'This statement contains destructive mutations that will permanently modify or delete schema objects.'}
        </p>

        <!-- Blast Radius Metrics -->
        <div class="grid grid-cols-2 gap-3 p-3.5 rounded-lg bg-[#090d16] border border-slate-800">
          <div>
            <span class="text-[10px] text-slate-500 uppercase tracking-wider block font-semibold">Target Environment</span>
            <span class="font-bold text-red-400 font-mono text-sm">{appState.activeConnection?.environment || 'Production'}</span>
          </div>
          <div>
            <span class="text-[10px] text-slate-500 uppercase tracking-wider block font-semibold">Estimated Impact</span>
            <span class="font-bold text-amber-300 font-mono text-sm">
              {inspection?.estimated_rows !== undefined && inspection?.estimated_rows !== null
                ? `${inspection.estimated_rows.toLocaleString()} rows`
                : 'All Rows / Table Drop'}
            </span>
          </div>
        </div>

        <!-- Query Preview -->
        <div>
          <span class="text-[10px] text-slate-500 uppercase tracking-wider block font-semibold mb-1">Statement Preview</span>
          <pre class="bg-[#090d16] p-3 rounded border border-slate-800 font-mono text-slate-300 text-[11px] overflow-x-auto whitespace-pre-wrap">{challenge.sql}</pre>
        </div>

        <!-- Typed Affirmation Challenge -->
        {#if targetTable}
          <div class="pt-2 border-t border-slate-800">
            <label for="affirmation-input" class="text-slate-300 block mb-1.5 font-medium">
              Type the table name <span class="font-mono text-red-400 font-bold bg-red-950/40 px-1.5 py-0.5 rounded border border-red-900/50">"{targetTable}"</span> to confirm execution:
            </label>
            <input
              id="affirmation-input"
              type="text"
              bind:value={typedAffirmation}
              placeholder={targetTable}
              class="w-full bg-[#090d16] border border-slate-700 focus:border-red-500 rounded px-3 py-2 text-slate-100 font-mono text-xs focus:outline-none focus:ring-1 focus:ring-red-500 transition-colors"
              onkeydown={(e) => e.key === 'Enter' && handleConfirm()}
            />
          </div>
        {/if}
      </div>

      <!-- Footer Buttons -->
      <div class="px-5 py-3.5 bg-[#090d16] border-t border-slate-800 flex items-center justify-end space-x-2.5">
        <button
          onclick={handleCancel}
          class="px-4 py-1.5 rounded text-slate-400 hover:text-white hover:bg-slate-800 text-xs transition"
        >
          Abort Execution
        </button>

        <button
          onclick={handleConfirm}
          disabled={!isAffirmationMatched}
          class="px-4 py-1.5 rounded bg-red-600 hover:bg-red-500 disabled:opacity-40 disabled:hover:bg-red-600 text-white font-bold text-xs transition shadow-lg flex items-center space-x-1.5"
        >
          <Icon name="shield-check" class="w-3.5 h-3.5" />
          <span>Confirm & Execute</span>
        </button>
      </div>
    </div>
  </div>
{/if}
