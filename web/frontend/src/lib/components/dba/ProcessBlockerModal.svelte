<script lang="ts">
  import type { ProcessLockWait } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import { focusTrap } from '../../actions/focusTrap';

  let {
    open = $bindable(false),
    locks,
    rootBlockerCount,
    onInspectThread,
    onKillThread,
    onCopy,
  }: {
    open: boolean;
    locks: ProcessLockWait[];
    rootBlockerCount: number;
    onInspectThread: (lock: ProcessLockWait, side: 'blocking' | 'waiting') => void;
    onKillThread: (id: number, mode: 'query' | 'connection') => void;
    onCopy: (text: string | undefined, label: string) => void;
  } = $props();
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) open = false;
    }}
  >
    <div
      role="dialog"
      aria-modal="true"
      use:focusTrap
      class="bg-[#0b0f19] border border-[#334155] rounded-xl w-full max-w-4xl max-h-[86vh] shadow-2xl overflow-hidden flex flex-col text-xs font-sans"
    >
      <div class="px-5 py-3.5 border-b border-[#334155] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2.5">
          <Icon name="shield-alert" class="w-4 h-4 {rootBlockerCount > 0 ? 'text-rose-400' : 'text-slate-400'}" />
          <div>
            <h3 class="font-bold text-slate-100 text-sm">Lock Contention &amp; Root Blocker Triage</h3>
            <p class="text-[11px] text-slate-500 mt-0.5">
              {rootBlockerCount > 0
                ? `${rootBlockerCount} root blocker(s) holding transactions`
                : locks.length > 0
                  ? `${locks.length} lock wait(s), no root blocker`
                  : 'No active lock contention'}
            </p>
          </div>
        </div>
        <div class="flex items-center space-x-2">
          <span class="px-2 py-0.5 rounded border text-[10px] font-bold {rootBlockerCount > 0
            ? 'bg-rose-950/60 border-rose-700 text-rose-300'
            : 'bg-slate-900 border-slate-700 text-slate-400'}">
            {rootBlockerCount > 0 ? `${rootBlockerCount} ROOT BLOCKER${rootBlockerCount > 1 ? 'S' : ''}` : `${locks.length} LOCK WAITS`}
          </span>
          <button onclick={() => (open = false)} class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition" title="Close" aria-label="Close">
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <div class="flex-1 overflow-y-auto p-4 space-y-3">
        {#if locks.length === 0}
          <div class="py-10 text-center space-y-2">
            <div class="w-10 h-10 mx-auto rounded-full bg-emerald-500/10 border border-emerald-600/30 flex items-center justify-center text-emerald-400">
              <Icon name="check-circle" class="w-5 h-5" />
            </div>
            <div class="font-bold text-slate-200">No Active Lock Contention</div>
            <div class="text-slate-500 text-[11px]">All threads and transactions are running unobstructed.</div>
          </div>
        {:else}
          {#each locks as lock, idx (idx)}
            {@const isRoot = !!lock.is_root_blocker}
            <div class="rounded-lg border {isRoot ? 'border-rose-800 bg-rose-950/25' : 'border-slate-800 bg-[#0f172a]'}">
              <div class="px-3.5 py-2.5 flex flex-wrap items-center justify-between gap-2 border-b {isRoot ? 'border-rose-900/60' : 'border-slate-800'}">
                <div class="flex flex-wrap items-center gap-2 min-w-0">
                  {#if isRoot}
                    <span class="px-1.5 py-0.5 rounded bg-rose-600 text-white text-[10px] font-bold tracking-wide flex items-center space-x-1">
                      <Icon name="alert-triangle" class="w-3 h-3" />
                      <span>ROOT BLOCKER</span>
                    </span>
                  {:else}
                    <span class="px-1.5 py-0.5 rounded bg-amber-950/60 border border-amber-800/70 text-amber-300 text-[10px] font-bold">
                      LOCK WAIT
                    </span>
                  {/if}
                  <span class="font-bold text-slate-100">Thread #{lock.blocking_thread_id}</span>
                  <span class="text-slate-500">{lock.blocking_user || 'unknown'}</span>
                  <span class="px-1.5 py-0.5 rounded bg-amber-950/50 border border-amber-900/70 text-amber-300 text-[10px] font-mono">
                    holding {lock.lock_mode || 'LOCK'}
                  </span>
                  {#if lock.locked_resource}
                    <span class="px-1.5 py-0.5 rounded bg-sky-950/50 border border-sky-900/70 text-sky-300 text-[10px] font-mono truncate max-w-[18rem]" title={lock.locked_resource}>
                      on {lock.locked_resource}
                    </span>
                  {/if}
                </div>
                <div class="flex items-center space-x-1.5 shrink-0">
                  <button
                    onclick={() => onInspectThread(lock, 'blocking')}
                    class="px-2 py-1 rounded border border-slate-700 bg-slate-900 hover:bg-slate-800 text-slate-300 transition flex items-center space-x-1"
                  >
                    <Icon name="search" class="w-3 h-3 text-sky-400" />
                    <span>Detail</span>
                  </button>
                  <button
                    onclick={() => onKillThread(lock.blocking_thread_id, 'query')}
                    class="px-2 py-1 rounded border border-amber-700/70 bg-amber-950/50 hover:bg-amber-900/60 text-amber-300 font-semibold transition"
                  >
                    Kill Query
                  </button>
                  <button
                    onclick={() => onKillThread(lock.blocking_thread_id, 'connection')}
                    class="px-2 py-1 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold transition"
                  >
                    Kill Blocker
                  </button>
                </div>
              </div>

              <div class="p-3 space-y-2">
                {#if lock.blocking_query}
                  <div class="rounded border border-slate-800 overflow-hidden">
                    <div class="px-2.5 py-1.5 bg-[#090d16] border-b border-slate-800 flex items-center justify-between">
                      <span class="text-[10px] font-semibold text-slate-400 uppercase tracking-wide">Blocking Query</span>
                      <div class="flex items-center space-x-2">
                        <button onclick={() => onCopy(lock.blocking_query, 'Blocking query')} class="text-[10px] text-slate-400 hover:text-white flex items-center space-x-1">
                          <Icon name="copy" class="w-3 h-3" />
                          <span>Copy</span>
                        </button>
                        <button onclick={() => onInspectThread(lock, 'blocking')} class="text-[10px] text-sky-400 hover:text-sky-300 flex items-center space-x-1">
                          <Icon name="search" class="w-3 h-3" />
                          <span>Inspect</span>
                        </button>
                      </div>
                    </div>
                    <pre class="p-2.5 bg-[#050811] text-sky-200 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all max-h-40 overflow-y-auto select-text">{lock.blocking_query}</pre>
                  </div>
                {:else}
                  <div class="px-2.5 py-2 rounded border border-slate-800 bg-[#090d16] text-slate-500 italic flex items-center justify-between">
                    <span>Idle in transaction: holding locks without a running query.</span>
                    <button onclick={() => onInspectThread(lock, 'blocking')} class="text-sky-400 hover:text-sky-300 not-italic text-[10px]">Show detail</button>
                  </div>
                {/if}

                <div class="pl-3 border-l-2 border-rose-700/70 flex flex-wrap items-center justify-between gap-2">
                  <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
                    <span class="text-rose-300">Blocked:</span>
                    <span class="font-bold text-slate-100">Thread #{lock.waiting_thread_id}</span>
                    <span class="text-slate-500">{lock.waiting_user || 'unknown'}</span>
                    <span class="text-slate-400">waiting</span>
                    <span class="text-amber-300 font-mono font-bold">{lock.waiting_seconds ?? 0}s</span>
                    <span class="text-slate-400">for {lock.lock_mode || 'lock'}</span>
                  </div>
                  <div class="flex items-center space-x-1.5">
                    <button
                      onclick={() => onInspectThread(lock, 'waiting')}
                      class="px-2 py-0.5 rounded border border-slate-700 bg-slate-900 hover:bg-slate-800 text-slate-300 text-[10px] transition flex items-center space-x-1"
                    >
                      <Icon name="search" class="w-3 h-3 text-sky-400" />
                      <span>Detail</span>
                    </button>
                    <button
                      onclick={() => onKillThread(lock.waiting_thread_id, 'connection')}
                      class="px-2 py-0.5 rounded border border-rose-800 bg-rose-950/50 hover:bg-rose-900/60 text-rose-300 text-[10px] transition"
                    >
                      Kill Waiting #{lock.waiting_thread_id}
                    </button>
                  </div>
                </div>

                {#if lock.waiting_query}
                  <div class="rounded border border-slate-800 overflow-hidden">
                    <div class="px-2.5 py-1.5 bg-[#090d16] border-b border-slate-800 flex items-center justify-between">
                      <span class="text-[10px] font-semibold text-slate-400 uppercase tracking-wide">Waiting Query</span>
                      <button onclick={() => onCopy(lock.waiting_query, 'Waiting query')} class="text-[10px] text-slate-400 hover:text-white flex items-center space-x-1">
                        <Icon name="copy" class="w-3 h-3" />
                        <span>Copy</span>
                      </button>
                    </div>
                    <pre class="p-2.5 bg-[#050811] text-slate-300 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all max-h-32 overflow-y-auto select-text">{lock.waiting_query}</pre>
                  </div>
                {/if}
              </div>
            </div>
          {/each}
        {/if}
      </div>

      <div class="px-5 py-2.5 border-t border-[#334155] bg-[#0f172a] flex items-center justify-between text-[10px] text-slate-500 font-mono">
        <span>Root blockers are blocking threads that are not waiting on another lock.</span>
        <button onclick={() => (open = false)} class="px-2.5 py-1 rounded border border-slate-700 bg-slate-900 hover:bg-slate-800 text-slate-300 font-sans text-xs transition">
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
