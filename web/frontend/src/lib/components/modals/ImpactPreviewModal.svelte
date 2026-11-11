<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

  let activeTab = $state<'tree' | 'raw' | 'advisor'>('tree');

  let planData = $derived(appState.impactPreviewData || appState.activeTab?.explainResult);

  let advisor = $derived(planData?.advisor);
  let recommendations = $derived(advisor?.recommendations || []);
  let bloatWarnings = $derived(advisor?.bloat_warnings || []);

  let rawRows = $derived(Array.isArray(planData?.rows) ? planData.rows : []);
  let rawJson = $derived(planData?.raw_json ? JSON.stringify(JSON.parse(planData.raw_json), null, 2) : '');

  // Parse nodes for DAG tree
  let planNodes = $derived.by(() => {
    if (!planData) return [];
    if (rawRows.length > 0) {
      return rawRows.map((r: any, idx: number) => {
        const table = r.table || r.TableName || (Array.isArray(r) ? r[2] : '') || '(subquery)';
        const op = r.type || r.Operation || (Array.isArray(r) ? r[3] : '') || 'SCAN';
        const key = r.key || r.Key || (Array.isArray(r) ? r[4] : '') || '';
        const rowsEst = r.rows || r.Rows || (Array.isArray(r) ? r[8] : null);
        const isDanger = String(op).toUpperCase() === 'ALL' || !key;
        return {
          step: idx + 1,
          table: String(table),
          operation: String(op),
          key: String(key || 'None'),
          rows: rowsEst != null ? Number(rowsEst) : null,
          isDanger,
        };
      });
    }
    return [];
  });

  function handleClose() {
    appState.isImpactPreviewOpen = false;
  }
</script>

{#if appState.isImpactPreviewOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-4xl rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100 max-h-[90vh]"
      role="dialog"
      aria-modal="true"
    >
      <!-- Modal Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60">
        <div class="flex items-center space-x-2.5">
          <span class="text-sky-400 font-bold">🔍</span>
          <h3 class="text-sm font-semibold text-slate-100">
            Pre-Flight Query Impact & Execution Plan Tree
          </h3>
        </div>

        <div class="flex items-center space-x-2">
          <!-- Tab selector -->
          <div class="flex bg-slate-800/80 p-0.5 rounded border border-slate-700/60 text-xs">
            <button
              type="button"
              onclick={() => (activeTab = 'tree')}
              class="px-3 py-1 rounded transition font-medium {activeTab === 'tree' ? 'bg-sky-600 text-white font-bold' : 'text-slate-400 hover:text-white'}"
            >
              Visual Plan Tree
            </button>
            <button
              type="button"
              onclick={() => (activeTab = 'advisor')}
              class="px-3 py-1 rounded transition font-medium flex items-center space-x-1.5 {activeTab === 'advisor' ? 'bg-sky-600 text-white font-bold' : 'text-slate-400 hover:text-white'}"
            >
              <span>Advisor</span>
              {#if recommendations.length > 0 || bloatWarnings.length > 0}
                <span class="px-1.5 py-0.2 rounded-full text-[10px] font-bold bg-amber-500 text-slate-950">
                  {recommendations.length + bloatWarnings.length}
                </span>
              {/if}
            </button>
            <button
              type="button"
              onclick={() => (activeTab = 'raw')}
              class="px-3 py-1 rounded transition font-medium {activeTab === 'raw' ? 'bg-sky-600 text-white font-bold' : 'text-slate-400 hover:text-white'}"
            >
              Raw Plan
            </button>
          </div>

          <button
            type="button"
            onclick={handleClose}
            class="text-slate-400 hover:text-slate-200 text-lg leading-none ml-2"
          >
            ✕
          </button>
        </div>
      </div>

      <!-- Modal Body -->
      <div class="p-5 overflow-y-auto flex-1 space-y-4">
        {#if activeTab === 'tree'}
          <!-- Visual Plan Tree -->
          {#if planNodes.length === 0}
            <div class="p-8 text-center text-slate-500 text-xs">
              No tabular or JSON execution plan available. Run EXPLAIN query first.
            </div>
          {:else}
            <div class="space-y-3">
              {#each planNodes as node, idx}
                <div class="rounded-xl border p-4 transition w-full shadow-lg {node.isDanger
                  ? 'border-rose-700/60 bg-gradient-to-r from-rose-950/30 to-slate-900'
                  : 'border-slate-800 bg-[#050811]'}">
                  <div class="flex items-center justify-between border-b border-slate-800 pb-2 mb-2.5">
                    <div class="flex items-center space-x-2">
                      <span class="px-2 py-0.5 rounded text-[10px] font-mono font-bold bg-slate-800 text-slate-300 border border-slate-700">
                        Step {node.step}
                      </span>
                      <span class="text-xs font-bold text-white tracking-wide">
                        Table Access: {node.table}
                      </span>
                    </div>

                    <span class="px-2 py-0.5 rounded text-[10px] font-mono font-bold uppercase border {node.isDanger
                      ? 'bg-rose-950 text-rose-400 border-rose-800'
                      : 'bg-emerald-950 text-emerald-400 border-emerald-800'}">
                      {node.operation}
                    </span>
                  </div>

                  <div class="grid grid-cols-2 gap-4 text-xs">
                    <div>
                      <span class="text-slate-400">Index Used: </span>
                      <span class="font-mono {node.key === 'None' ? 'text-amber-400' : 'text-sky-300 font-bold'}">
                        {node.key}
                      </span>
                    </div>
                    {#if node.rows != null}
                      <div class="text-right">
                        <span class="text-slate-400">Estimated Rows: </span>
                        <span class="font-mono text-slate-200 font-semibold">{node.rows.toLocaleString()}</span>
                      </div>
                    {/if}
                  </div>
                </div>

                {#if idx < planNodes.length - 1}
                  <div class="flex justify-center -my-1 text-slate-600 text-sm">
                    ↓
                  </div>
                {/if}
              {/each}
            </div>
          {/if}
        {:else if activeTab === 'advisor'}
          <!-- Index & Performance Advisor -->
          <div class="space-y-4">
            {#if bloatWarnings.length > 0}
              {#each bloatWarnings as bw}
                <div class="p-3.5 rounded-lg border border-amber-800/80 bg-amber-950/30 text-amber-200 text-xs flex items-start space-x-2.5">
                  <span class="text-amber-400 font-bold">⚠️</span>
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
                      <pre class="p-2.5 bg-slate-900 rounded border border-slate-800 text-emerald-400 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap">{rec.safe_ddl}</pre>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}

            {#if bloatWarnings.length === 0 && recommendations.length === 0}
              <div class="p-8 text-center text-slate-500 text-xs">
                No performance warnings or index recommendations detected. The query structure is optimal.
              </div>
            {/if}
          </div>
        {:else}
          <!-- Raw Plan / JSON -->
          {#if rawJson}
            <pre class="p-4 bg-[#050811] border border-slate-800 rounded-lg text-sky-300 font-mono text-xs overflow-x-auto whitespace-pre-wrap">{rawJson}</pre>
          {:else if rawRows.length > 0}
            <div class="overflow-x-auto border border-slate-800 rounded-lg">
              <table class="w-full text-left text-xs font-mono">
                <thead class="bg-slate-900 border-b border-slate-800 text-slate-400">
                  <tr>
                    {#each Object.keys(rawRows[0] || {}) as col}
                      <th class="p-2.5">{col}</th>
                    {/each}
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-800 bg-[#050811]">
                  {#each rawRows as row}
                    <tr class="hover:bg-slate-800/40">
                      {#each Object.values(row) as val}
                        <td class="p-2.5 text-slate-200">{String(val ?? '')}</td>
                      {/each}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}
            <div class="p-8 text-center text-slate-500 text-xs">No raw plan output found.</div>
          {/if}
        {/if}
      </div>

      <!-- Modal Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-end">
        <button
          type="button"
          onclick={handleClose}
          class="px-4 py-1.5 rounded text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-slate-200 transition"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
