<script lang="ts">
  import { focusTrap } from '../../actions/focusTrap';
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { copyText } from '../../utils/clipboard';
  import type { ProcessLockWait } from '../../../types';
  import Icon from '../common/Icon.svelte';

  let locks = $state<ProcessLockWait[]>([]);
  let isLoading = $state(false);
  let errorMsg = $state('');

  async function refreshLocks() {
    if (!appState.activeSessionId) return;
    isLoading = true;
    errorMsg = '';
    try {
      const res = await api.getProcesslist(appState.activeSessionId);
      locks = Array.isArray(res?.locks) ? res.locks : [];
      appState.dbaBlockersCount = locks.length;
    } catch (err: any) {
      errorMsg = err.message || 'Failed to load lock contention data';
    } finally {
      isLoading = false;
    }
  }

  async function killBlocker(threadId: number, mode: 'query' | 'connection' = 'connection') {
    if (!appState.activeSessionId) return;
    const ok = await appState.showNativeConfirm(
      `Execute ${mode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY'} on blocking thread #${threadId}?`,
      'Kill Blocking Thread'
    );
    if (!ok) return;
    try {
      await api.killProcess(appState.activeSessionId, threadId, mode);
      appState.showToast(`Killed blocking thread #${threadId}`, 'success');
      await refreshLocks();
    } catch (err: any) {
      appState.showToast(`Failed to kill thread #${threadId}: ${err.message}`, 'error');
    }
  }

  $effect(() => {
    if (appState.isLockInspectorOpen && appState.activeSessionId) {
      void refreshLocks();
    }
  });
</script>

{#if appState.isLockInspectorOpen}
  <div
    id="lock-inspector-dialog"
    class="fixed inset-0 z-[95] flex items-center justify-center bg-black/75 p-4"
  >
    <div role="dialog" aria-modal="true" use:focusTrap class="bg-[#0f172a] border border-slate-700 w-full max-w-3xl rounded-xl shadow-2xl flex flex-col overflow-hidden max-h-[85vh]">
      <div class="px-5 py-3.5 border-b border-slate-800 bg-slate-900/60 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <Icon name="lock" class="w-4 h-4 text-amber-400" />
          <h3 class="text-sm font-bold text-slate-100">Lock Contention & Root Blocker Analysis</h3>
          {#if locks.length > 0}
            <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-rose-500/20 text-rose-300 border border-rose-500/40">
              {locks.length} lock wait(s)
            </span>
          {/if}
        </div>
        <button
          onclick={() => (appState.isLockInspectorOpen = false)}
          class="p-1 text-slate-400 hover:text-white rounded hover:bg-slate-800"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-4 flex-1 overflow-y-auto space-y-3 text-xs">
        {#if errorMsg}
          <div class="p-3 rounded-lg bg-rose-950/50 border border-rose-800 text-rose-200">{errorMsg}</div>
        {:else if isLoading && locks.length === 0}
          <div class="p-10 text-center text-slate-400 flex items-center justify-center gap-2">
            <Icon name="loader" class="w-4 h-4 animate-spin text-sky-400" />
            <span>Inspecting active lock graph...</span>
          </div>
        {:else if locks.length === 0}
          <div class="p-10 text-center space-y-2">
            <Icon name="check-circle" class="w-8 h-8 text-emerald-400 mx-auto" />
            <p class="text-slate-300 font-semibold">No active lock contention detected.</p>
            <p class="text-slate-500 text-[11px]">All running queries are proceeding without row or metadata lock waits.</p>
          </div>
        {:else}
          {#each locks as lock}
            <div class="p-3.5 rounded-lg bg-[#050811] border {lock.is_root_blocker ? 'border-rose-600/60' : 'border-amber-600/40'} space-y-2.5">
              <div class="flex items-center justify-between gap-2 flex-wrap">
                <div class="flex items-center gap-2">
                  {#if lock.is_root_blocker}
                    <span class="px-2 py-0.5 rounded bg-rose-500/20 text-rose-300 border border-rose-500/50 font-bold text-[10px]">
                      ROOT BLOCKER #{lock.blocking_thread_id}
                    </span>
                  {:else}
                    <span class="px-2 py-0.5 rounded bg-amber-500/20 text-amber-300 border border-amber-500/40 font-bold text-[10px]">
                      BLOCKER #{lock.blocking_thread_id}
                    </span>
                  {/if}
                  <span class="text-slate-400">blocking</span>
                  <span class="font-mono text-sky-300 font-bold">#{lock.waiting_thread_id}</span>
                  {#if lock.waiting_seconds != null}
                    <span class="text-rose-400 font-mono">({lock.waiting_seconds}s wait)</span>
                  {/if}
                </div>
                <div class="flex items-center gap-1.5">
                  {#if lock.lock_mode}
                    <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-300 font-mono text-[10px]">{lock.lock_mode}</span>
                  {/if}
                  {#if lock.locked_resource}
                    <span class="px-1.5 py-0.5 rounded bg-slate-800 text-amber-300 font-mono text-[10px]">{lock.locked_resource}</span>
                  {/if}
                </div>
              </div>

              <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5">
                <div class="p-2.5 rounded bg-slate-900/70 border border-slate-800 space-y-1">
                  <div class="flex items-center justify-between">
                    <span class="text-[10px] font-bold text-rose-300 uppercase">Blocking Query ({lock.blocking_user || 'user'})</span>
                    <div class="flex items-center gap-1">
                      {#if lock.blocking_query}
                        <button
                          onclick={() => { copyText(lock.blocking_query || ''); appState.showToast('Copied SQL', 'info'); }}
                          class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] flex items-center gap-1"
                        >
                          <Icon name="copy" class="w-2.5 h-2.5" />
                          <span>Copy SQL</span>
                        </button>
                      {/if}
                      <button
                        onclick={() => killBlocker(lock.blocking_thread_id, 'connection')}
                        class="px-1.5 py-0.5 rounded bg-rose-950/70 hover:bg-rose-900 border border-rose-700 text-rose-300 text-[10px] font-semibold"
                      >
                        Kill Blocker
                      </button>
                    </div>
                  </div>
                  <pre class="font-mono text-[10.5px] text-slate-200 whitespace-pre-wrap break-all">{lock.blocking_query || '(idle in transaction)'}</pre>
                </div>

                <div class="p-2.5 rounded bg-slate-900/70 border border-slate-800 space-y-1">
                  <div class="flex items-center justify-between">
                    <span class="text-[10px] font-bold text-amber-300 uppercase">Waiting Query ({lock.waiting_user || 'user'})</span>
                    {#if lock.waiting_query}
                      <button
                        onclick={() => { copyText(lock.waiting_query || ''); appState.showToast('Copied SQL', 'info'); }}
                        class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] flex items-center gap-1"
                      >
                        <Icon name="copy" class="w-2.5 h-2.5" />
                        <span>Copy SQL</span>
                      </button>
                    {/if}
                  </div>
                  <pre class="font-mono text-[10.5px] text-slate-300 whitespace-pre-wrap break-all">{lock.waiting_query || '-'}</pre>
                </div>
              </div>
            </div>
          {/each}
        {/if}
      </div>

      <div class="px-5 py-3 border-t border-slate-800 bg-slate-900/60 flex items-center justify-between">
        <button
          onclick={() => {
            appState.isLockInspectorOpen = false;
            appState.openProcesslistTab('threads');
          }}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 text-xs font-medium flex items-center gap-1.5"
        >
          <Icon name="cpu" class="w-3.5 h-3.5" />
          <span>Processlist Tab</span>
        </button>

        <div class="flex items-center gap-2">
          <button
            onclick={refreshLocks}
            disabled={isLoading}
            class="px-3.5 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium flex items-center gap-1.5"
          >
            <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin' : ''}" />
            <span>Refresh</span>
          </button>
          <button
            onclick={() => (appState.isLockInspectorOpen = false)}
            class="px-4 py-1.5 rounded bg-sky-500 hover:bg-sky-400 text-slate-950 text-xs font-bold"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
