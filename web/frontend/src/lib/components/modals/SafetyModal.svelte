<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let typedAffirmation = $state('');

  let challenge = $derived(appState.safetyChallenge);
  let inspection = $derived(challenge.inspection);
  let targetTable = $derived(inspection?.target_table || '');
  let targetDb = $derived(inspection?.target_database || appState.activeTab?.database || appState.currentDatabase || '');
  let challengeTarget = $derived(targetTable || targetDb);
  let isSchemaMismatch = $derived(Boolean(inspection?.schema_scope_mismatch));
  let isDevAffirmation = $derived(Boolean(inspection?.requires_affirmation && !inspection?.requires_challenge));
  let requiresTypedChallenge = $derived(
    Boolean(!isDevAffirmation && (inspection?.requires_challenge || inspection?.is_destructive) && challengeTarget)
  );

  let isAffirmationMatched = $derived(
    !requiresTypedChallenge
      ? true
      : typedAffirmation.trim().toLowerCase() === challengeTarget.toLowerCase() ||
          (Boolean(targetDb) && typedAffirmation.trim().toLowerCase() === targetDb.toLowerCase())
  );

  function handleConfirm() {
    if (!isAffirmationMatched) {
      appState.showToast(`Please type "${challengeTarget}" to confirm execution.`, 'warning');
      return;
    }
    const input = typedAffirmation.trim() || challengeTarget;
    appState.confirmSafetyChallenge(input);
    typedAffirmation = '';
  }

  function handleCancel() {
    appState.cancelSafetyChallenge();
    typedAffirmation = '';
  }
</script>

{#if challenge.isOpen}
  <div class="fixed inset-0 z-[110] flex items-center justify-center bg-black/80 p-4 select-none">
    <div style="contain: layout style paint;" class="bg-[#0f172a] border-2 {isDevAffirmation ? 'border-amber-500' : 'border-red-500'} rounded-xl shadow-2xl max-w-lg w-full overflow-hidden flex flex-col">
      <!-- Modal Header -->
      <div class="{isDevAffirmation ? 'bg-amber-950/60 border-amber-500/40' : 'bg-red-950/60 border-red-500/40'} border-b px-5 py-3.5 flex items-center justify-between gap-2">
        <div class="flex items-center space-x-2 {isDevAffirmation ? 'text-amber-300' : 'text-red-400'} font-bold text-sm tracking-wide">
          <Icon name="shield-alert" class="w-5 h-5 shrink-0" />
          <span>
            {isSchemaMismatch
              ? 'SAFETY GUARD: SCHEMA SCOPE MISMATCH DETECTED'
              : isDevAffirmation
                ? 'SAFETY GUARD: AFFIRMATION REQUIRED'
                : 'DETERMINISTIC SAFETY GUARD™'}
          </span>
        </div>
        <div class="flex items-center gap-1.5 shrink-0">
          {#if isSchemaMismatch}
            <span class="text-[10px] font-bold px-2 py-0.5 rounded bg-amber-500 text-slate-950 uppercase">
              Schema Mismatch
            </span>
          {/if}
          <span class="text-[10px] font-bold px-2 py-0.5 rounded {isDevAffirmation ? 'bg-amber-500' : 'bg-red-500'} text-slate-950 uppercase tracking-wider">
            {inspection?.risk_level || 'CRITICAL'}
          </span>
        </div>
      </div>

      <!-- Content -->
      <div class="p-5 space-y-4 text-xs max-h-[75vh] overflow-y-auto">
        <p class="text-slate-200 leading-relaxed font-sans">
          {inspection?.reason || 'This statement contains destructive mutations that will permanently modify or delete schema objects.'}
        </p>

        <!-- Blast Radius Metrics -->
        <div class="grid grid-cols-3 gap-2.5 p-3 rounded-lg bg-[#090d16] border border-slate-800">
          <div>
            <span class="text-[10px] text-slate-500 uppercase tracking-wider block font-semibold">Environment</span>
            <span class="font-bold {appState.isProduction ? 'text-red-400' : 'text-amber-300'} font-mono text-xs">{appState.activeConnection?.environment || 'Development'}</span>
          </div>
          <div>
            <span class="text-[10px] text-slate-500 uppercase tracking-wider block font-semibold">Target Database</span>
            <span class="font-bold text-sky-300 font-mono text-xs truncate block" title={targetDb}>{targetDb || '(default)'}</span>
          </div>
          <div>
            <span class="text-[10px] text-slate-500 uppercase tracking-wider block font-semibold">Estimated Impact</span>
            <span class="font-bold text-amber-300 font-mono text-xs">
              {inspection?.estimated_rows !== undefined && inspection?.estimated_rows !== null
                ? `${inspection.estimated_rows.toLocaleString()} rows`
                : 'All Rows / DDL'}
            </span>
          </div>
        </div>

        {#if inspection?.statement_estimates && inspection.statement_estimates.length > 0}
          <div class="bg-[#090d16] border border-slate-800 rounded-lg p-2.5 space-y-1.5">
            <span class="text-[10px] text-slate-500 uppercase tracking-wider font-semibold block">Multi-Statement Blast Breakdown</span>
            {#each inspection.statement_estimates as est}
              <div class="flex items-center justify-between text-[11px] bg-slate-900/70 px-2 py-1 rounded border border-slate-800/80">
                <span class="font-mono text-slate-300 truncate max-w-[260px]">{est.operation} {est.target_table || est.target_database || ''}</span>
                <span class="font-mono text-amber-300">
                  {est.estimated_rows != null ? `~${est.estimated_rows.toLocaleString()} rows` : (est.has_where ? 'Filtered' : 'Full Table')}
                </span>
              </div>
            {/each}
          </div>
        {/if}

        {#if (inspection?.table_locks && inspection.table_locks.length > 0) || (inspection?.relationships && inspection.relationships.length > 0)}
          <div class="grid grid-cols-2 gap-2 text-[11px]">
            {#if inspection?.table_locks && inspection.table_locks.length > 0}
              <div class="p-2 rounded bg-amber-950/30 border border-amber-800/50 text-amber-200">
                <div class="font-semibold text-[10px] uppercase text-amber-400">Active Table Locks</div>
                <div>{inspection.table_locks.length} active lock(s) detected</div>
              </div>
            {/if}
            {#if inspection?.relationships && inspection.relationships.length > 0}
              <div class="p-2 rounded bg-sky-950/30 border border-sky-800/50 text-sky-200">
                <div class="font-semibold text-[10px] uppercase text-sky-400">FK Dependencies</div>
                <div>{inspection.relationships.length} relationship(s) linked</div>
              </div>
            {/if}
          </div>
        {/if}

        <!-- Query Preview -->
        <div>
          <span class="text-[10px] text-slate-500 uppercase tracking-wider block font-semibold mb-1">Statement Preview</span>
          <pre class="bg-[#090d16] p-3 rounded border border-slate-800 font-mono text-slate-300 text-[11px] overflow-x-auto whitespace-pre-wrap max-h-40">{challenge.sql}</pre>
        </div>

        <!-- Typed Affirmation Challenge -->
        {#if requiresTypedChallenge}
          <div class="pt-2 border-t border-slate-800">
            <label for="affirmation-input" class="text-slate-300 block mb-1.5 font-medium">
              Type <span class="font-mono text-red-400 font-bold bg-red-950/40 px-1.5 py-0.5 rounded border border-red-900/50">"{challengeTarget}"</span> to confirm execution:
            </label>
            <input
              id="affirmation-input"
              type="text"
              bind:value={typedAffirmation}
              placeholder={challengeTarget}
              class="w-full bg-[#090d16] border border-slate-700 focus:border-red-500 rounded px-3 py-2 text-slate-100 font-mono text-xs focus:outline-none focus:ring-1 focus:ring-red-500"
              onkeydown={(e) => {
                if (e.key !== 'Escape') e.stopPropagation();
                if (e.key === 'Enter') handleConfirm();
              }}
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
          class="px-4 py-1.5 rounded {isDevAffirmation
            ? 'bg-amber-500 hover:bg-amber-400 text-slate-950'
            : 'bg-red-600 hover:bg-red-500 text-white'} disabled:opacity-40 font-bold text-xs transition shadow-lg flex items-center space-x-1.5"
        >
          <Icon name="shield-check" class="w-3.5 h-3.5" />
          <span>{isDevAffirmation && targetDb ? `Confirm & Execute on ${targetDb}` : 'Confirm & Execute'}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
