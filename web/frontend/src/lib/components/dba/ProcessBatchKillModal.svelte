<script lang="ts">
  import type { ProcesslistThread } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import { focusTrap } from '../../actions/focusTrap';

  let {
    open = $bindable(false),
    batchKillMode,
    batchKillTargetIds,
    threads,
    needsBatchChallenge,
    isProduction,
    batchKillChallengeInput = $bindable(''),
    batchChallengeValid,
    isBatchKilling,
    onRemoveTarget,
    onExecute,
  }: {
    open: boolean;
    batchKillMode: 'query' | 'connection';
    batchKillTargetIds: number[];
    threads: ProcesslistThread[];
    needsBatchChallenge: boolean;
    isProduction: boolean;
    batchKillChallengeInput: string;
    batchChallengeValid: boolean;
    isBatchKilling: boolean;
    onRemoveTarget: (id: number) => void;
    onExecute: () => void;
  } = $props();
</script>

{#if open}
  <div class="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      use:focusTrap
      class="bg-[#0b0f19] border border-slate-700 rounded-2xl w-full max-w-2xl shadow-2xl overflow-hidden flex flex-col text-xs font-sans"
    >
      <div class="px-5 py-4 border-b border-slate-800 flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2.5">
          <Icon name={batchKillMode === 'connection' ? 'x-circle' : 'square'} class="w-5 h-5 text-rose-400" />
          <div>
            <h3 class="font-bold text-slate-100 text-sm flex items-center space-x-2">
              <span>Confirm Batch {batchKillMode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY'}</span>
              <span class="px-2 py-0.5 rounded bg-rose-950 text-rose-300 text-[11px] font-mono border border-rose-800">
                {batchKillTargetIds.length} Threads
              </span>
            </h3>
            <p class="text-[11px] text-slate-400">
              {batchKillMode === 'connection'
                ? 'Disconnecting client connections terminates sessions immediately and rolls back uncommitted work.'
                : 'Cancelling running queries aborts active statements while keeping client connections open.'}
            </p>
          </div>
        </div>
        <button type="button" onclick={() => (open = false)} class="text-slate-400 hover:text-white" aria-label="Close">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div class="max-h-56 overflow-y-auto bg-[#050811] border border-slate-800 rounded-xl divide-y divide-slate-800/70 p-2 font-mono text-[11px]">
          {#each batchKillTargetIds as id (id)}
            {@const bt = threads.find((x) => x.id === id)}
            <div class="py-1.5 px-2 flex items-center justify-between gap-2 hover:bg-slate-900/60 rounded">
              <div class="min-w-0 flex-1">
                <div class="flex items-center space-x-1.5">
                  <span class="font-bold text-sky-400">#{id}</span>
                  {#if bt}
                    <span class="text-slate-300 font-semibold">{bt.user}@{bt.host}</span>
                    <span class="text-slate-500">({bt.db || '-'})</span>
                    <span class="px-1 rounded bg-slate-800 text-slate-300 text-[10px]">{bt.command}</span>
                  {/if}
                </div>
                {#if bt}
                  <div class="text-[10px] text-slate-400 truncate mt-0.5" title={bt.info || ''}>
                    {bt.info || (bt.command === 'Sleep' ? 'Idle connection (Sleep)' : '-')}
                  </div>
                {/if}
              </div>
              <div class="flex items-center space-x-2 shrink-0">
                {#if bt}
                  <span class="font-bold {bt.time >= 30 ? 'text-rose-400' : bt.time >= 5 ? 'text-amber-400' : 'text-slate-300'}">
                    {bt.time}s
                  </span>
                {/if}
                <button
                  type="button"
                  onclick={() => onRemoveTarget(id)}
                  class="text-slate-500 hover:text-rose-400 p-1 rounded hover:bg-slate-800"
                  title="Remove thread #{id} from batch"
                >
                  <Icon name="x" class="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          {/each}
        </div>

        {#if needsBatchChallenge}
          <div class="p-3.5 bg-rose-950/40 border border-rose-800/80 rounded-xl space-y-2">
            <div class="font-bold text-rose-300 flex items-center space-x-1.5">
              <Icon name="shield-alert" class="w-4 h-4 text-rose-400" />
              <span>
                {isProduction
                  ? 'Production Environment Safety Guardrail'
                  : `Large Batch Safety Guardrail (${batchKillTargetIds.length} threads)`}
              </span>
            </div>
            <p class="text-[11px] text-slate-300">
              Type <strong class="text-white font-mono bg-rose-900/60 px-1.5 py-0.5 rounded border border-rose-700/80">KILL</strong> below to unlock batch execution:
            </p>
            <input
              type="text"
              bind:value={batchKillChallengeInput}
              placeholder="Type KILL here..."
              class="w-full bg-slate-900 border border-rose-700/80 px-3 py-1.5 rounded text-white font-mono text-xs outline-none focus:border-rose-500 uppercase tracking-widest"
            />
          </div>
        {/if}
      </div>

      <div class="px-5 py-3 border-t border-slate-800 flex items-center justify-between bg-[#0f172a]">
        <span class="text-[11px] text-slate-500">Recorded in Session Audit Log.</span>
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={() => (open = false)}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-medium transition"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!batchChallengeValid || isBatchKilling}
            onclick={onExecute}
            class="px-3.5 py-1.5 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold transition flex items-center space-x-1.5 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Icon name="zap" class="w-3.5 h-3.5" />
            <span>{isBatchKilling ? 'Executing...' : `Execute ${batchKillMode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY'}`}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
