<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';
  import { focusTrap } from '../../actions/focusTrap';

  let selectedFile: File | null = $state(null);
  let targetDb = $state('');
  let stopOnError = $state(true);
  let isRunning = $state(false);
  let progressPct = $state(0);
  let currentStmtIndex = $state(0);
  let totalStmts = $state(0);
  let logs = $state<string[]>([]);
  let pendingBlockers = $state<any[] | null>(null);
  let challengeInput = $state('');
  let isProduction = $derived(appState.activeConnection?.environment === 'Production');

  let wasOpen = false;

  $effect(() => {
    if (appState.isRunSqlModalOpen) {
      if (!wasOpen) {
        selectedFile = null;
        targetDb = appState.activeTab?.database || appState.currentDatabase || '';
        stopOnError = true;
        isRunning = false;
        progressPct = 0;
        currentStmtIndex = 0;
        totalStmts = 0;
        logs = [];
        pendingBlockers = null;
        challengeInput = '';
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  function formatSize(bytes: number): string {
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
    return `${(bytes / 1024).toFixed(1)} KB`;
  }

  function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement;
    if (input.files && input.files[0]) {
      selectedFile = input.files[0];
      pendingBlockers = null;
      logs = [`Loaded file: ${selectedFile.name} (${formatSize(selectedFile.size)})`];
    }
  }

  async function handleStartExecution(confirmChallenge = false) {
    if (!selectedFile || !appState.activeSessionId) return;

    const effectiveDb = targetDb || appState.activeTab?.database || appState.currentDatabase || undefined;
    pendingBlockers = null;
    isRunning = true;
    progressPct = 0;
    currentStmtIndex = 0;
    totalStmts = 0;
    logs = [`Streaming SQL script "${selectedFile.name}" (${formatSize(selectedFile.size)}) on "${effectiveDb || 'default'}"...`];
    let successCount = 0;
    let errorCount = 0;

    try {
      const res = await api.runSqlScript(
        appState.activeSessionId,
        selectedFile,
        {
          stopOnError,
          database: effectiveDb,
          confirm: confirmChallenge,
          challengeInput: confirmChallenge ? challengeInput.trim() : undefined,
        },
        (evt) => {
          if (evt.total) totalStmts = evt.total;
          if (evt.statement_index) {
            currentStmtIndex = evt.statement_index;
            if (totalStmts > 0) {
              progressPct = Math.round((currentStmtIndex / totalStmts) * 100);
            }
          }
          if (evt.status === 'error') {
            errorCount++;
            logs = [...logs, `[ERROR #${evt.statement_index}]: ${evt.error || 'Failed'}`];
          } else if (evt.sql_preview) {
            successCount++;
            const aff = evt.rows_affected != null ? ` · ${evt.rows_affected} rows` : '';
            logs = [...logs, `[#${evt.statement_index} ${evt.duration_ms}ms${aff}]: ${evt.sql_preview}`];
          }
        }
      );

      logs = [...logs, `Finished: ${successCount} succeeded, ${errorCount} errors. Status: ${res.status || 'completed'}`];
      appState.showToast(`Script executed: ${successCount} statements succeeded`, errorCount > 0 ? 'warning' : 'success');
      await appState.refreshCatalog();
    } catch (err: any) {
      if (err?.status === 409 || err?.data?.requires_challenge || err?.message?.includes('destructive statements')) {
        const blockers = Array.isArray(err?.data?.blockers) ? err.data.blockers : [];
        pendingBlockers = blockers.length > 0 ? blockers : [{ reason: err.message }];
        logs = [...logs, `[SAFETY GUARD 409]: Script paused pending explicit operator confirmation.`];
        return;
      }
      logs = [...logs, `[CRITICAL ERROR]: ${err.message}`];
      appState.showToast(`Execution failed: ${err.message}`, 'error');
    } finally {
      isRunning = false;
    }
  }

  async function handleClose() {
    if (isRunning) {
      const ok = await appState.showNativeConfirm(
        'SQL script execution is currently running. Stop and close this dialog anyway?',
        'Script Execution Running'
      );
      if (!ok) return;
    }
    appState.isRunSqlModalOpen = false;
  }
</script>

{#if appState.isRunSqlModalOpen}
  <div class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      use:focusTrap
      aria-labelledby="run-sql-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-2xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-400">
            <Icon name="file-code" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="run-sql-title" class="font-bold text-sm text-slate-100 font-sans">
              Run Large SQL Script File
            </h3>
            <p class="text-[11px] text-slate-400 font-sans">
              Execute multi-statement SQL migration files and dumps with Safety Guard preflight
            </p>
          </div>
        </div>
        <button
          type="button"
          onclick={handleClose}
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Dialog Body -->
      <div class="p-5 space-y-4 text-xs font-sans flex-1 overflow-y-auto">
        <!-- File & Target DB -->
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div class="sm:col-span-2 space-y-1">
            <div class="flex items-center justify-between">
              <label for="run-sql-file" class="font-semibold text-slate-300">Select SQL File (.sql, .txt)</label>
              {#if selectedFile}
                <span class="px-1.5 py-0.5 rounded bg-sky-950/70 border border-sky-800 text-sky-300 font-mono text-[10px]">
                  {formatSize(selectedFile.size)}
                </span>
              {/if}
            </div>
            <input
              id="run-sql-file"
              type="file"
              accept=".sql,text/plain,.txt"
              onchange={handleFileSelect}
              class="w-full text-xs text-slate-400 file:mr-3 file:py-1.5 file:px-3 file:rounded file:border-0 file:text-xs file:font-semibold file:bg-sky-950 file:text-sky-400 hover:file:bg-sky-900 cursor-pointer bg-[#090d16] border border-[#334155] rounded p-1"
            />
          </div>

          <div class="space-y-1">
            <label for="run-sql-db" class="font-semibold text-slate-300">Target Database</label>
            <select
              id="run-sql-db"
              bind:value={targetDb}
              class="w-full bg-[#090d16] border border-[#334155] rounded px-2.5 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-sky-500"
            >
              <option value="">(Session Default)</option>
              {#each appState.availableDatabases as db}
                <option value={db}>{db}</option>
              {/each}
            </select>
          </div>
        </div>

        <!-- Options -->
        <label class="flex items-center space-x-2 text-slate-200 cursor-pointer select-none">
          <input
            type="checkbox"
            bind:checked={stopOnError}
            class="rounded bg-slate-900 border-slate-700 text-amber-500 focus:ring-0"
          />
          <span>Stop execution on first error</span>
        </label>

        <!-- 409 Destructive Statements Lock Banner -->
        {#if pendingBlockers}
          <div class="p-3.5 rounded-xl bg-rose-950/40 border border-rose-800/80 space-y-2.5">
            <div class="flex items-center space-x-2 text-rose-300 font-bold">
              <Icon name="shield-alert" class="w-4 h-4 text-rose-400 shrink-0" />
              <span>Safety Guard Hold (HTTP 409): Destructive Statements Detected</span>
            </div>
            <div class="max-h-32 overflow-y-auto space-y-1 bg-slate-950/70 border border-rose-900/60 rounded p-2 font-mono text-[11px] text-rose-200">
              {#each pendingBlockers as b, bIdx}
                <div>
                  {bIdx + 1}. {typeof b === 'string' ? b : (b.reason || b.message || JSON.stringify(b))}
                  {#if b?.statement}
                    <span class="block text-slate-400 truncate">{b.statement}</span>
                  {/if}
                </div>
              {/each}
            </div>
            {#if isProduction}
              <div class="space-y-1">
                <label for="run-sql-challenge-input" class="block text-[11px] text-rose-200 font-semibold">
                  Production confirmation required: type target database or table name (<span class="font-mono text-white">{targetDb || appState.currentDatabase || appState.activeConnection?.name}</span>)
                </label>
                <input
                  id="run-sql-challenge-input"
                  type="text"
                  bind:value={challengeInput}
                  placeholder={targetDb || appState.currentDatabase || 'target database or table'}
                  class="w-full bg-[#090d16] border border-rose-700 rounded px-2.5 py-1.5 text-xs text-white font-mono focus:outline-none focus:border-rose-400"
                />
              </div>
            {/if}
            <div class="flex items-center justify-end space-x-2">
              <button
                type="button"
                onclick={() => { pendingBlockers = null; challengeInput = ''; }}
                class="px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px] font-medium"
              >
                Abort
              </button>
              <button
                type="button"
                disabled={isProduction && !challengeInput.trim()}
                onclick={() => handleStartExecution(true)}
                class="px-3.5 py-1 rounded bg-rose-600 hover:bg-rose-500 disabled:opacity-40 text-white text-[11px] font-bold flex items-center space-x-1"
              >
                <Icon name="zap" class="w-3 h-3" />
                <span>Acknowledge &amp; Execute Script Anyway</span>
              </button>
            </div>
          </div>
        {/if}

        <!-- Progress Bar (when running or completed) -->
        {#if isRunning || totalStmts > 0}
          <div class="space-y-1.5 bg-[#090d16] border border-[#1e293b] p-3 rounded-lg">
            {#if selectedFile}
              <div class="text-[11px] font-semibold text-sky-300 truncate">
                Executing: {selectedFile.name}
              </div>
            {/if}
            <div class="flex items-center justify-between text-[11px] font-mono">
              <span class="text-slate-300">Progress: {currentStmtIndex} / {totalStmts}</span>
              <span class="text-sky-400 font-bold">{progressPct}%</span>
            </div>
            <div class="w-full bg-slate-800 rounded-full h-2 overflow-hidden">
              <div
                class="bg-sky-500 h-2 transition-all duration-150"
                style="width: {progressPct}%;"
              ></div>
            </div>
          </div>
        {/if}

        <!-- Execution Logs -->
        <div class="space-y-1">
          <span class="font-semibold text-slate-400 text-[11px] uppercase tracking-wider block">
            Execution Log
          </span>
          <div class="h-36 overflow-y-auto bg-[#090d16] border border-[#1e293b] p-2.5 rounded-lg font-mono text-[11px] space-y-1 select-text">
            {#if logs.length === 0}
              <span class="text-slate-600 italic">No file selected yet.</span>
            {:else}
              {#each logs as log}
                <div class="{log.includes('[ERROR') || log.includes('[CRITICAL') || log.includes('[SAFETY') ? 'text-rose-400' : log.includes('Finished') ? 'text-emerald-400 font-bold' : 'text-slate-300'}">
                  {log}
                </div>
              {/each}
            {/if}
          </div>
        </div>
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-end space-x-2 shrink-0 font-sans">
        <button
          onclick={handleClose}
          type="button"
          class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition"
        >
          Cancel
        </button>
        <button
          onclick={() => handleStartExecution(false)}
          disabled={isRunning || !selectedFile}
          type="button"
          class="px-4 py-1.5 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow"
        >
          {#if isRunning}
            <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
            <span>Running SQL...</span>
          {:else}
            <Icon name="play" class="w-3.5 h-3.5" />
            <span>Start Execution</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
