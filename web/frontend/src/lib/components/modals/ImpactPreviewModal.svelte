<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
  import { parseExplainPlan } from '../../utils/explainPlan';
  import { formatSQL } from '../../utils/sqlFormatter';

  let activeTab = $state<'grid' | 'tree' | 'advisor' | 'raw'>('grid');
  let isQueryFormatted = $state(false);

  let planData = $derived(appState.impactPreviewData || appState.activeTab?.explainResult);
  let targetSql = $derived(appState.impactPreviewSql || appState.activeTab?.explainSql || appState.activeTab?.sql || '');
  let displaySql = $derived(isQueryFormatted && targetSql ? formatSQL(targetSql) : targetSql);

  let engineBadge = $derived((appState.activeConnection?.driver || 'OPTIMIZER').toUpperCase());

  let advisor = $derived(planData?.advisor);
  let recommendations = $derived(advisor?.recommendations || []);
  let bloatWarnings = $derived(advisor?.bloat_warnings || []);
  let advisorCount = $derived(recommendations.length + bloatWarnings.length);

  let parsedPlan = $derived(parseExplainPlan(planData));
  let planNodes = $derived(parsedPlan.nodes);
  let rawRows = $derived(parsedPlan.rawRows);
  let rawJson = $derived(parsedPlan.rawJsonFormatted);

  let hasFullScan = $derived(planNodes.some((n) => n.isDanger));
  let totalEstRows = $derived(planNodes.reduce((sum, n) => sum + (n.rows ?? 0), 0));

  let rawOutputText = $derived.by(() => {
    if (rawJson) return rawJson;
    if (planData) return JSON.stringify(planData, null, 2);
    return '';
  });

  // Reset tab to Plan Grid whenever modal opens with new data
  $effect(() => {
    if (appState.isImpactPreviewOpen) {
      activeTab = 'grid';
      isQueryFormatted = false;
    }
  });

  function handleClose() {
    appState.isImpactPreviewOpen = false;
  }

  async function copyText(text: string, label: string) {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      appState.showToast(`${label} copied to clipboard`, 'success');
    } catch {
      appState.showToast('Failed to copy to clipboard', 'error');
    }
  }

  function handleExecuteQuery() {
    const sql = targetSql;
    appState.isImpactPreviewOpen = false;
    if (sql.trim()) {
      appState.executeCurrentTab(sql);
    }
  }
</script>

{#if appState.isImpactPreviewOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 backdrop-blur-sm p-3 sm:p-5 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-slate-900 border border-slate-700/80 w-full max-w-5xl rounded-xl shadow-2xl p-5 space-y-3.5 flex flex-col max-h-[88vh] text-slate-100"
      role="dialog"
      aria-modal="true"
    >
      <!-- Modal Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-800 pb-3 shrink-0">
        <div class="flex items-center space-x-3">
          <div class="w-9 h-9 rounded-lg bg-sky-500/10 border border-sky-500/25 flex items-center justify-center text-sky-400 shadow-sm">
            <Icon name="zap" class="w-5 h-5" />
          </div>
          <div>
            <div class="flex items-center space-x-2">
              <h3 class="text-base font-bold text-white tracking-wide">Execution Plan (EXPLAIN)</h3>
              <span class="px-1.5 py-0.5 rounded text-[10px] font-mono uppercase bg-slate-800 text-slate-300 border border-slate-700">
                {engineBadge}
              </span>
            </div>
            <p class="text-xs text-slate-400">Query cost analysis, index utilization & visual execution tree</p>
          </div>
        </div>

        <div class="flex items-center space-x-3">
          <!-- 4-Tab Segmented Switcher -->
          <div class="flex bg-slate-950/80 rounded-lg p-1 border border-slate-800 text-xs shadow-inner">
            <button
              type="button"
              onclick={() => (activeTab = 'grid')}
              class="px-3 py-1.5 rounded-md font-medium transition flex items-center space-x-1.5 {activeTab === 'grid'
                ? 'bg-sky-600 text-white font-bold shadow-sm'
                : 'text-slate-400 hover:text-white'}"
            >
              <Icon name="activity" class="w-3.5 h-3.5" />
              <span>Plan Grid</span>
            </button>
            <button
              type="button"
              onclick={() => (activeTab = 'tree')}
              class="px-3 py-1.5 rounded-md font-medium transition flex items-center space-x-1.5 {activeTab === 'tree'
                ? 'bg-sky-600 text-white font-bold shadow-sm'
                : 'text-slate-400 hover:text-white'}"
            >
              <Icon name="git-branch" class="w-3.5 h-3.5" />
              <span>Visual Tree</span>
              <span class="ml-1 px-1.5 py-0.2 bg-sky-950 text-sky-300 text-[10px] rounded-full border border-sky-800 font-mono font-bold">
                {planNodes.length}
              </span>
            </button>
            <button
              type="button"
              onclick={() => (activeTab = 'advisor')}
              class="px-3 py-1.5 rounded-md font-medium transition flex items-center space-x-1.5 {activeTab === 'advisor'
                ? 'bg-sky-600 text-white font-bold shadow-sm'
                : 'text-slate-400 hover:text-white'}"
            >
              <Icon name="sparkles" class="w-3.5 h-3.5" />
              <span>Advisor</span>
              {#if advisorCount > 0}
                <span class="ml-1 px-1.5 py-0.2 bg-amber-950 text-amber-300 text-[10px] rounded-full border border-amber-800 font-bold">
                  {advisorCount}
                </span>
              {/if}
            </button>
            <button
              type="button"
              onclick={() => (activeTab = 'raw')}
              class="px-3 py-1.5 rounded-md font-medium transition flex items-center space-x-1.5 {activeTab === 'raw'
                ? 'bg-sky-600 text-white font-bold shadow-sm'
                : 'text-slate-400 hover:text-white'}"
            >
              <span class="font-mono">{'{ }'}</span>
              <span>Raw Plan</span>
            </button>
          </div>

          <button
            type="button"
            onclick={handleClose}
            class="w-8 h-8 rounded-lg bg-slate-800/60 hover:bg-slate-800 text-slate-400 hover:text-white flex items-center justify-center transition border border-slate-700"
            aria-label="Close"
          >
            <Icon name="x" class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      <!-- Target Query Card -->
      {#if targetSql}
        <div class="bg-slate-950/90 border border-slate-800 rounded-lg p-3 shrink-0 space-y-2">
          <div class="flex items-center justify-between">
            <div class="flex items-center space-x-2">
              <span class="text-[10px] font-bold uppercase tracking-wider text-slate-400 bg-slate-800/80 px-2 py-0.5 rounded border border-slate-700/60">
                Target Query
              </span>
              <span class="text-[10px] text-slate-500 font-mono">{targetSql.length} chars</span>
            </div>
            <div class="flex items-center space-x-1.5">
              <button
                type="button"
                onclick={() => (isQueryFormatted = !isQueryFormatted)}
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px] border border-slate-700 transition flex items-center space-x-1"
              >
                <Icon name="sparkles" class="w-3.5 h-3.5" />
                <span>{isQueryFormatted ? 'Raw' : 'Format'}</span>
              </button>
              <button
                type="button"
                onclick={() => copyText(displaySql, 'Query')}
                class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px] border border-slate-700 transition flex items-center space-x-1"
              >
                <Icon name="copy" class="w-3.5 h-3.5 text-slate-400" />
                <span>Copy</span>
              </button>
            </div>
          </div>
          <pre class="font-mono text-[11px] text-sky-200/90 whitespace-pre-wrap break-all max-h-24 overflow-y-auto bg-slate-900/80 p-2.5 rounded border border-slate-800 select-all">{displaySql}</pre>
        </div>
      {/if}

      <!-- Tab Body -->
      {#if activeTab === 'grid'}
        <!-- Tab 1: Plan Grid -->
        <div class="flex-1 overflow-auto border border-slate-800 rounded-lg bg-slate-950 min-h-[200px]">
          {#if planNodes.length === 0}
            <div class="p-12 text-center text-slate-500 italic text-xs">
              No execution plan steps returned by the database engine.
            </div>
          {:else}
            <table class="w-full text-left text-xs border-collapse">
              <thead class="bg-slate-800/90 text-slate-300 font-semibold sticky top-0 border-b border-slate-700 z-10 backdrop-blur-sm">
                <tr>
                  <th class="p-2.5 w-10 text-right pr-2 text-slate-500 font-mono text-xs select-none border-r border-slate-800">#</th>
                  <th class="p-2.5">Table</th>
                  <th class="p-2.5">Access Type</th>
                  <th class="p-2.5">Key Used</th>
                  <th class="p-2.5 text-right">{parsedPlan.dialectHint === 'postgres' ? 'Rows / Cost' : 'Rows Est.'}</th>
                  <th class="p-2.5">Extra / Condition</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-800/70 font-mono text-[11px]">
                {#each planNodes as node}
                  <tr class="hover:bg-slate-800/50 transition">
                    <td class="p-2.5 w-10 text-right pr-2 text-slate-500 border-r border-slate-800/40 select-none">
                      {node.step}
                    </td>
                    <td class="p-2.5 font-bold text-white">
                      <span style={node.depth > 0 ? `margin-left: ${Math.min(node.depth, 5) * 14}px` : ''}>
                        {node.table || '-'}
                      </span>
                    </td>
                    <td class="p-2.5 whitespace-nowrap">
                      {#if node.isDanger}
                        <span class="bg-red-950/80 text-red-300 border border-red-700/80 px-2 py-0.5 rounded text-[10px] font-bold shadow-sm inline-flex items-center gap-1">
                          <span>⚠</span><span>{node.operation}</span>
                        </span>
                      {:else if node.isWarning}
                        <span class="bg-amber-950/80 text-amber-300 border border-amber-700/80 px-2 py-0.5 rounded text-[10px] font-semibold">
                          {node.operation}
                        </span>
                      {:else}
                        <span class="bg-emerald-950/80 text-emerald-300 border border-emerald-700/80 px-2 py-0.5 rounded text-[10px] font-semibold inline-flex items-center gap-1">
                          <span>✓</span><span>{node.operation}</span>
                        </span>
                      {/if}
                    </td>
                    <td class="p-2.5">
                      {#if node.key}
                        <span class="text-sky-300 font-semibold">{node.key}</span>
                      {:else}
                        <span class="text-slate-600">NULL</span>
                      {/if}
                    </td>
                    <td class="p-2.5 text-right font-semibold text-slate-200 whitespace-nowrap">
                      {#if node.actualRows != null}
                        {node.actualRows.toLocaleString()} <span class="text-slate-500 font-normal">/ {node.rows?.toLocaleString() ?? '-'}</span>
                      {:else if node.rows != null}
                        {node.rows.toLocaleString()}
                      {:else if node.cost != null}
                        <span class="text-slate-400">cost {node.cost.toFixed(1)}</span>
                      {:else}
                        <span class="text-slate-500">-</span>
                      {/if}
                    </td>
                    <td class="p-2.5 text-slate-300 max-w-md break-words">
                      {node.extra || '-'}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>
      {:else if activeTab === 'tree'}
        <!-- Tab 2: Visual Tree -->
        <div class="flex-1 overflow-auto border border-slate-800 rounded-lg bg-slate-950 p-4 min-h-[200px]">
          {#if planNodes.length === 0}
            <div class="p-12 text-center text-slate-500 italic text-xs">
              No execution steps available to render visual tree.
            </div>
          {:else}
            <!-- Summary Stats Banner -->
            <div class="mb-4 p-3 bg-slate-900/80 border border-slate-800 rounded-lg flex flex-wrap items-center justify-between gap-2 text-xs">
              <div class="flex items-center space-x-3">
                <span class="text-slate-400">Steps: <strong class="text-white">{planNodes.length}</strong></span>
                <span class="text-slate-700">|</span>
                <span class="text-slate-400">Est. Rows: <strong class="text-sky-300">{totalEstRows.toLocaleString()}</strong></span>
              </div>
              {#if hasFullScan}
                <span class="text-rose-400 font-semibold">⚠ Full scan step detected</span>
              {:else}
                <span class="text-emerald-400 font-semibold">✓ Indexed / bounded access</span>
              {/if}
            </div>

            <div class="space-y-3">
              {#each planNodes as node, idx}
                <div
                  style={node.depth > 0 ? `margin-left: ${Math.min(node.depth, 6) * 20}px` : ''}
                  class="rounded-xl border p-4 transition shadow-lg {node.isDanger
                    ? 'border-rose-700/60 bg-rose-950/30'
                    : node.isWarning
                      ? 'border-amber-700/60 bg-amber-950/20'
                      : 'border-slate-800 bg-[#050811]'}"
                >
                  <div class="flex items-center justify-between border-b border-slate-800 pb-2 mb-2.5">
                    <div class="flex items-center space-x-2">
                      <span class="px-2 py-0.5 rounded text-[10px] font-mono font-bold bg-slate-800 text-slate-300 border border-slate-700">
                        Step {node.step}
                      </span>
                      <span class="text-xs font-bold text-white tracking-wide">
                        {node.table ? `Table Access: ${node.table}` : node.operation}
                      </span>
                    </div>

                    <span class="px-2 py-0.5 rounded text-[10px] font-mono font-bold uppercase border {node.isDanger
                      ? 'bg-rose-950 text-rose-400 border-rose-800'
                      : node.isWarning
                        ? 'bg-amber-950 text-amber-400 border-amber-800'
                        : 'bg-emerald-950 text-emerald-400 border-emerald-800'}">
                      {node.operation}
                    </span>
                  </div>

                  <div class="grid grid-cols-2 gap-4 text-xs">
                    <div>
                      <span class="text-slate-400">Index Used: </span>
                      <span class="font-mono {!node.key ? 'text-amber-400' : 'text-sky-300 font-bold'}">
                        {node.key || 'None'}
                      </span>
                    </div>
                    {#if node.actualRows != null}
                      <div class="text-right">
                        <span class="text-slate-400">Actual / Est. Rows: </span>
                        <span class="font-mono text-slate-200 font-semibold">
                          {node.actualRows.toLocaleString()} / {node.rows?.toLocaleString() ?? '-'}
                        </span>
                      </div>
                    {:else if node.rows != null}
                      <div class="text-right">
                        <span class="text-slate-400">Estimated Rows: </span>
                        <span class="font-mono text-slate-200 font-semibold">{node.rows.toLocaleString()}</span>
                      </div>
                    {:else if node.cost != null}
                      <div class="text-right">
                        <span class="text-slate-400">Total Cost: </span>
                        <span class="font-mono text-slate-200 font-semibold">{node.cost.toFixed(2)}</span>
                      </div>
                    {/if}
                  </div>
                  {#if node.extra}
                    <div class="mt-2 pt-2 border-t border-slate-800/70 text-[11px] font-mono text-slate-400 break-words">
                      {node.extra}
                    </div>
                  {/if}
                </div>

                {#if idx < planNodes.length - 1}
                  <div class="flex justify-center -my-1 text-slate-500">
                    <Icon name="arrow-down" class="w-3.5 h-3.5" />
                  </div>
                {/if}
              {/each}
            </div>
          {/if}
        </div>
      {:else if activeTab === 'advisor'}
        <!-- Tab 3: Index & Performance Advisor -->
        <div class="flex-1 overflow-auto border border-slate-800 rounded-lg bg-slate-950 p-4 min-h-[200px] space-y-3.5">
          {#if bloatWarnings.length > 0}
            {#each bloatWarnings as bw}
              <div class="p-3.5 rounded-lg border border-amber-800/80 bg-amber-950/30 text-amber-200 text-xs flex items-start space-x-2.5">
                <Icon name="alert-triangle" class="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
                <div>
                  <div class="font-bold text-amber-300">Index Bloat Warning ({bw.table || 'Table'}):</div>
                  <div class="mt-0.5 text-slate-300">{bw.warning || bw.message}</div>
                </div>
              </div>
            {/each}
          {/if}

          {#if recommendations.length > 0}
            <div class="space-y-3">
              <h4 class="text-xs font-semibold text-slate-300">Recommended Indexes:</h4>
              {#each recommendations as rec}
                <div class="p-3.5 rounded-lg border border-slate-800 bg-[#050811] space-y-2 text-xs">
                  <div class="flex items-center justify-between">
                    <span class="font-bold text-sky-400">{rec.table || 'Target Table'}</span>
                    {#if rec.estimated_gain}
                      <span class="text-emerald-400 text-[11px] font-mono">Gain: {rec.estimated_gain}</span>
                    {/if}
                  </div>
                  <p class="text-slate-300">{rec.reason || 'Add compound index for optimized filter selectivity.'}</p>
                  {#if rec.safe_ddl}
                    <div class="relative">
                      <pre class="p-2.5 pr-16 bg-slate-900 rounded border border-slate-800 text-emerald-400 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap">{rec.safe_ddl}</pre>
                      <button
                        type="button"
                        onclick={() => copyText(rec.safe_ddl, 'DDL')}
                        class="absolute top-2 right-2 px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] border border-slate-700"
                      >
                        Copy
                      </button>
                    </div>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}

          {#if bloatWarnings.length === 0 && recommendations.length === 0}
            <div class="p-12 text-center text-slate-500 italic text-xs">
              ✓ No new index recommendations needed. Query predicates are either covered by existing indexes or not indexable.
            </div>
          {/if}
        </div>
      {:else}
        <!-- Tab 4: Raw Plan -->
        <div class="flex-1 overflow-auto border border-slate-800 rounded-lg bg-slate-950 p-3 min-h-[200px]">
          <div class="flex items-center justify-between pb-2 mb-2 border-b border-slate-800 text-xs">
            <span class="text-slate-400 font-mono text-[11px]">Raw Optimizer Output</span>
            {#if rawOutputText}
              <button
                type="button"
                onclick={() => copyText(rawOutputText, 'Raw plan')}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px] border border-slate-700 transition flex items-center space-x-1"
              >
                <Icon name="copy" class="w-3 h-3" />
                <span>Copy Raw</span>
              </button>
            {/if}
          </div>
          {#if rawOutputText}
            <pre class="font-mono text-slate-300 text-xs whitespace-pre overflow-x-auto select-all">{rawOutputText}</pre>
          {:else if rawRows.length === 0}
            <div class="p-8 text-center text-slate-500 text-xs">No raw plan output found.</div>
          {/if}
        </div>
      {/if}

      <!-- Modal Footer -->
      <div class="flex items-center justify-between pt-2.5 border-t border-slate-800 shrink-0">
        <div class="text-xs text-slate-400 flex items-center space-x-2">
          {#if planNodes.length === 0}
            <span class="text-slate-500 italic">0 execution steps returned</span>
          {:else if hasFullScan}
            <span class="text-amber-400 font-semibold inline-flex items-center gap-1.5">
              <Icon name="alert-triangle" class="w-4 h-4 text-amber-400" />
              <span>Full table scan detected!</span>
            </span>
            {#if advisorCount > 0}
              <button
                type="button"
                onclick={() => (activeTab = 'advisor')}
                class="text-amber-300 hover:underline font-bold"
              >
                View Index Advisor ({advisorCount})
              </button>
            {/if}
          {:else}
            <span class="text-emerald-400 font-semibold inline-flex items-center gap-1">
              <Icon name="check" class="w-4 h-4 text-emerald-400" />
              <span>{planNodes.length} execution {planNodes.length === 1 ? 'step' : 'steps'} planned</span>
            </span>
            {#if totalEstRows > 0}
              <span class="text-slate-500">(~{totalEstRows.toLocaleString()} rows examined)</span>
            {/if}
          {/if}
        </div>

        <div class="flex space-x-2">
          <button
            type="button"
            onclick={handleClose}
            class="px-4 py-1.5 text-xs bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg transition border border-slate-700"
          >
            Close
          </button>
          {#if targetSql.trim()}
            <button
              type="button"
              onclick={handleExecuteQuery}
              class="px-4 py-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-lg shadow-sm transition flex items-center space-x-1.5"
            >
              <Icon name="play" class="w-3.5 h-3.5" />
              <span>Execute Query</span>
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}
