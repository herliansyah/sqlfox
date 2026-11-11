<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let profile = $derived(appState.activeTab?.activeResult?.profile);
  let stages = $derived(profile?.stages || []);
  let durationMs = $derived(profile?.duration_ms || 0);

  function closePanel() {
    if (appState.activeTab) {
      appState.activeTab.showQueryProfiler = false;
    }
  }
</script>

{#if appState.activeTab?.showQueryProfiler}
  <div class="h-full w-full flex flex-col bg-[#050811] text-slate-100 overflow-hidden font-sans border-t border-slate-800">
    <!-- Header -->
    <div class="px-4 py-2 bg-slate-900/80 border-b border-slate-800 flex items-center justify-between">
      <div class="flex items-center space-x-2.5">
        <Icon name="clock" class="w-4 h-4 text-sky-400" />
        <span class="text-xs font-semibold text-slate-200">Query Stage Profiler</span>
        <span class="px-2 py-0.5 rounded text-[11px] font-mono font-bold bg-sky-950 text-sky-400 border border-sky-800">
          {durationMs.toFixed(2)} ms total
        </span>
      </div>

      <button
        type="button"
        onclick={closePanel}
        class="text-slate-400 hover:text-white px-2 py-0.5 rounded hover:bg-slate-800 text-xs transition"
      >
        Close Profiler
      </button>
    </div>

    <!-- Table -->
    <div class="flex-1 overflow-y-auto">
      {#if stages.length === 0}
        <div class="p-8 text-center text-slate-500 text-xs">
          No detailed stage profile available for this query dialect. (MySQL/MariaDB supported).
        </div>
      {:else}
        <table class="w-full text-left text-xs">
          <thead class="bg-slate-900 border-b border-slate-800 text-slate-400 sticky top-0 font-mono">
            <tr>
              <th class="p-2 text-center w-12">#</th>
              <th class="p-2">Stage</th>
              <th class="p-2 text-right w-28">Duration</th>
              <th class="p-2 w-48">Execution Share</th>
              <th class="p-2 text-right w-16">%</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-800/60 font-mono">
            {#each stages as stage, idx}
              {@const pct = Math.max(0, Math.min(100, stage.percentage || 0))}
              <tr class="hover:bg-slate-800/40 transition">
                <td class="p-2 text-center text-slate-500">{idx + 1}</td>
                <td class="p-2 text-white font-sans font-medium">{stage.stage}</td>
                <td class="p-2 text-right text-slate-300">
                  {stage.duration_us >= 1000
                    ? `${(stage.duration_us / 1000).toFixed(2)} ms`
                    : `${stage.duration_us} µs`}
                </td>
                <td class="p-2">
                  <div class="w-full bg-slate-800 rounded-full h-2 overflow-hidden">
                    <div
                      class="{pct > 50 ? 'bg-rose-500' : pct > 20 ? 'bg-amber-500' : 'bg-sky-500'} h-2 rounded-full transition-all duration-300"
                      style="width: {pct}%"
                    ></div>
                  </div>
                </td>
                <td class="p-2 text-right text-slate-400 text-[11px]">{pct.toFixed(1)}%</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  </div>
{/if}
