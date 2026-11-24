<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { appState } from '../../stores/appState.svelte';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';

  let {
    variablesReport = null,
    isVariablesLoading = false,
  }: {
    variablesReport: any;
    isVariablesLoading: boolean;
  } = $props();

  let varFilterText = $state('');
  let varCategory = $state('All');
  let showCuratedOnly = $state(false);
  let varSeverityFilter = $state<'all' | 'critical' | 'warning' | 'optimal'>('all');
  let expandedVars = new SvelteSet<string>();

  function toggleVarExpand(name: string) {
    if (expandedVars.has(name)) expandedVars.delete(name);
    else expandedVars.add(name);
  }

  function copyWithToast(text: string | undefined, label: string) {
    if (!text) return;
    copyText(text);
    appState.showToast(`Copied ${label} to clipboard`, 'success');
  }
</script>

    <!-- Server Variables View -->
    <div class="flex-1 flex flex-col overflow-hidden">
      <!-- Variables Filter Bar -->
      <div class="min-h-9 py-1.5 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between flex-wrap gap-2 text-xs select-none">
        <div class="flex items-center space-x-2 flex-wrap gap-y-1">
          <button
            type="button"
            onclick={() => (showCuratedOnly = !showCuratedOnly)}
            class="px-2 py-0.5 rounded border text-[11px] font-sans font-semibold flex items-center space-x-1 transition {showCuratedOnly
              ? 'bg-amber-950/70 border-amber-600 text-amber-300'
              : 'bg-[#0b0f19] border-[#334155] text-slate-300 hover:text-white'}"
            title="Show curated performance tuning variables only"
          >
            <Icon name="star" class="w-3 h-3 text-amber-400" />
            <span>Curated ({curatedVarCount})</span>
          </button>

          <span class="text-slate-400 font-sans text-xs">Category:</span>
          <select
            bind:value={varCategory}
            class="bg-[#0b0f19] border border-[#334155] rounded px-2 py-0.5 text-xs text-sky-400 font-semibold focus:outline-none cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220] [&>option]:text-slate-200"
          >
            {#each varCategories as cat}
              <option value={cat}>{cat}</option>
            {/each}
          </select>

          <select
            bind:value={varSeverityFilter}
            class="bg-[#0b0f19] border border-[#334155] rounded px-2 py-0.5 text-xs text-slate-300 focus:outline-none cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220]"
            aria-label="Filter variables by severity"
          >
            <option value="all">All Severities</option>
            <option value="critical">Critical</option>
            <option value="warning">Warning</option>
            <option value="optimal">Optimal</option>
          </select>
        </div>

        <input
          type="text"
          bind:value={varFilterText}
          placeholder="Search variables by name or value..."
          class="bg-[#0b0f19] border border-[#334155] rounded px-2.5 py-1 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-56 font-sans"
        />
      </div>

      <div class="flex-1 overflow-auto">
        <table class="w-full text-left border-collapse text-xs">
          <thead class="bg-[#0f172a] sticky top-0 z-10 border-b border-[#334155] text-slate-400 text-[11px] uppercase tracking-wider font-sans select-none">
            <tr>
              <th class="py-2 px-3 w-7"></th>
              <th class="py-2 px-3 w-64">Variable Name</th>
              <th class="py-2 px-3 w-44">Value</th>
              <th class="py-2 px-3 w-28">Category</th>
              <th class="py-2 px-3 w-20 text-center">Severity</th>
              <th class="py-2 px-3 w-20 text-center">Dynamic</th>
              <th class="py-2 px-3">Tuning Recommendation</th>
              <th class="py-2 px-3 w-24 text-center">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#1e293b]/60">
            {#if isVariablesLoading}
              <tr>
                <td colspan="8" class="p-8 text-center text-slate-500">
                  Inspecting server variables...
                </td>
              </tr>
            {:else if filteredVariables.length === 0}
              <tr>
                <td colspan="8" class="p-8 text-center text-slate-500">
                  No server variables match your filter.
                </td>
              </tr>
            {:else}
              {#each filteredVariables as v (v.name)}
                {@const isExpanded = expandedVars.has(v.name)}
                {@const sev = String(v.severity || 'info').toLowerCase()}
                <tr
                  class="hover:bg-slate-800/40 cursor-pointer transition-colors"
                  onclick={() => toggleVarExpand(v.name)}
                >
                  <td class="py-2 px-3 text-slate-500">
                    <Icon name={isExpanded ? 'chevron-down' : 'chevron-right'} class="w-3.5 h-3.5" />
                  </td>
                  <td class="py-2 px-3 font-mono font-bold text-sky-300">
                    {v.name}
                    {#if v.is_curated}
                      <span class="ml-1 text-amber-400" title="Curated tuning knob">★</span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 font-mono text-slate-200 truncate max-w-[180px]" title={String(v.value)}>{v.value}</td>
                  <td class="py-2 px-3 text-slate-400">{v.category || 'General'}</td>
                  <td class="py-2 px-3 text-center font-sans">
                    <span class="px-1.5 py-0.5 rounded text-[9.5px] font-bold uppercase border {sev === 'critical' ? 'bg-rose-950 text-rose-300 border-rose-800' : sev === 'warning' ? 'bg-amber-950 text-amber-300 border-amber-800' : sev === 'optimal' ? 'bg-emerald-950 text-emerald-300 border-emerald-800' : 'bg-slate-800 text-slate-400 border-slate-700'}">
                      {sev}
                    </span>
                  </td>
                  <td class="py-2 px-3 text-center">
                    {#if v.is_dynamic}
                      <span class="px-1.5 py-0.2 rounded bg-emerald-950 text-emerald-300 border border-emerald-800 text-[10px] font-bold">YES</span>
                    {:else}
                      <span class="text-slate-600 text-[10px]">RESTART</span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 text-slate-400 text-[11px] font-sans">
                    {#if v.recommendation || v.suggested_value}
                      <span class="text-amber-300 font-semibold">{v.suggested_value ? `Suggested: ${v.suggested_value}` : ''}</span>
                      <span class="text-slate-400 block truncate max-w-md">{v.recommendation || v.description}</span>
                    {:else}
                      <span class="text-slate-600">{v.description || '-'}</span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 text-center whitespace-nowrap" onclick={(e) => e.stopPropagation()}>
                    <div class="inline-flex items-center space-x-1">
                      {#if v.set_global_snippet}
                        <button
                          type="button"
                          onclick={() => copyWithToast(v.set_global_snippet, 'SET GLOBAL statement')}
                          class="p-1 rounded text-sky-400 hover:text-white hover:bg-slate-800 transition"
                          title="Copy SET GLOBAL snippet"
                        >
                          <Icon name="copy" class="w-3.5 h-3.5" />
                        </button>
                      {/if}
                      <button
                        type="button"
                        onclick={() => appState.sendToAi(`Explain server variable "${v.name}" (current value="${v.value}", scope="${v.scope || 'Global'}", dynamic=${!!v.is_dynamic}). ${v.description || ''} ${v.recommendation || ''}`, `Variable: ${v.name}`)}
                        class="p-1 rounded text-purple-400 hover:text-purple-200 hover:bg-slate-800 transition"
                        title="Ask Copilot about this variable"
                      >
                        <Icon name="bot" class="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
                {#if isExpanded}
                  <tr class="bg-[#060913]">
                    <td colspan="8" class="px-6 py-3 border-t border-slate-800/70">
                      <div class="grid grid-cols-1 md:grid-cols-3 gap-3 text-[11px] font-sans">
                        <div>
                          <span class="text-[10px] uppercase text-slate-500 font-bold block">Scope &amp; Description</span>
                          <span class="text-sky-300 font-mono text-[10.5px] block mb-1">Scope: {v.scope || 'Global'}</span>
                          <p class="text-slate-300 leading-relaxed">{v.description || 'No description available.'}</p>
                        </div>
                        <div>
                          <span class="text-[10px] uppercase text-slate-500 font-bold block">Impact if Too Low</span>
                          <p class="text-amber-200/90 leading-relaxed">{v.impact_low || '—'}</p>
                        </div>
                        <div>
                          <span class="text-[10px] uppercase text-slate-500 font-bold block">Impact if Too High</span>
                          <p class="text-rose-200/90 leading-relaxed">{v.impact_high || '—'}</p>
                        </div>
                      </div>
                      {#if v.my_cnf_snippet || v.set_global_snippet}
                        <div class="mt-2.5 pt-2 border-t border-slate-800/70 flex flex-wrap items-center gap-2">
                          {#if v.my_cnf_snippet}
                            <button
                              type="button"
                              onclick={() => copyWithToast(v.my_cnf_snippet, 'my.cnf directive')}
                              class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-mono text-[10.5px] flex items-center space-x-1"
                            >
                              <Icon name="copy" class="w-3 h-3 text-emerald-400" />
                              <span>Copy my.cnf: {v.my_cnf_snippet}</span>
                            </button>
                          {/if}
                          {#if v.set_global_snippet}
                            <button
                              type="button"
                              onclick={() => copyWithToast(v.set_global_snippet, 'SET GLOBAL statement')}
                              class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-sky-200 border border-slate-700 font-mono text-[10.5px] flex items-center space-x-1"
                            >
                              <Icon name="copy" class="w-3 h-3 text-sky-400" />
                              <span>Copy SQL: {v.set_global_snippet}</span>
                            </button>
                          {/if}
                        </div>
                      {/if}
                    </td>
                  </tr>
                {/if}
              {/each}
            {/if}
          </tbody>
        </table>
      </div>
    </div>
