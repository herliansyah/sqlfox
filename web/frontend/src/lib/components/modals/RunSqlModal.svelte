<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let selectedFile: File | null = $state(null);
  let stopOnError = $state(true);
  let isRunning = $state(false);
  let progressPct = $state(0);
  let currentStmtIndex = $state(0);
  let totalStmts = $state(0);
  let logs = $state<string[]>([]);

  $effect(() => {
    if (appState.isRunSqlModalOpen) {
      selectedFile = null;
      stopOnError = true;
      isRunning = false;
      progressPct = 0;
      currentStmtIndex = 0;
      totalStmts = 0;
      logs = [];
    }
  });

  function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement;
    if (input.files && input.files[0]) {
      selectedFile = input.files[0];
      logs.push(`Loaded file: ${selectedFile.name} (${(selectedFile.size / 1024).toFixed(1)} KB)`);
    }
  }

  async function handleStartExecution(confirmChallenge = false) {
    if (!selectedFile || !appState.activeSessionId) return;

    isRunning = true;
    logs = [`Streaming SQL script "${selectedFile.name}" (${(selectedFile.size / 1024).toFixed(1)} KB) to server...`];
    let successCount = 0;
    let errorCount = 0;

    try {
      const res = await api.runSqlScript(
        appState.activeSessionId,
        selectedFile,
        {
          stopOnError,
          database: appState.currentDatabase || undefined,
          confirm: confirmChallenge,
        },
        (evt) => {
          if (evt.total) {
            totalStmts = evt.total;
          }
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
            logs = [...logs, `[#${evt.statement_index} ${evt.duration_ms}ms]: ${evt.sql_preview}`];
          }
        }
      );

      logs = [...logs, `Finished: ${successCount} succeeded, ${errorCount} errors. Status: ${res.status || 'completed'}`];
      appState.showToast(`Script executed: ${successCount} statements succeeded`, errorCount > 0 ? 'warning' : 'success');
      await appState.refreshCatalog();
    } catch (err: any) {
      if (err.message?.includes('destructive statements') || err.message?.includes('requires_challenge')) {
        const ok = confirm(`Script contains destructive statements that require confirmation:\n\n${err.message}\n\nDo you want to proceed and execute anyway?`);
        if (ok) {
          return handleStartExecution(true);
        }
      }
      logs = [...logs, `[CRITICAL ERROR]: ${err.message}`];
      appState.showToast(`Execution failed: ${err.message}`, 'error');
    } finally {
      isRunning = false;
    }
  }

  function handleClose() {
    if (isRunning) {
      if (!confirm('SQL script execution is currently running. Stop and close?')) return;
    }
    appState.isRunSqlModalOpen = false;
  }
</script>

{#if appState.isRunSqlModalOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="run-sql-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-xl flex flex-col max-h-[85vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
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
              Execute multi-statement SQL migration files and dumps
            </p>
          </div>
        </div>
        <button
          onclick={handleClose}
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Dialog Body -->
      <div class="p-5 space-y-4 text-xs font-sans flex-1 overflow-y-auto">
        <!-- File Picker -->
        <div class="space-y-1">
          <label for="run-sql-file" class="font-semibold text-slate-300">Select SQL File (.sql)</label>
          <input
            id="run-sql-file"
            type="file"
            accept=".sql"
            onchange={handleFileSelect}
            class="w-full text-xs text-slate-400 file:mr-3 file:py-1.5 file:px-3 file:rounded file:border-0 file:text-xs file:font-semibold file:bg-sky-950 file:text-sky-400 hover:file:bg-sky-900 cursor-pointer bg-[#090d16] border border-[#334155] rounded p-1.5"
          />
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

        <!-- Progress Bar (when running or completed) -->
        {#if isRunning || totalStmts > 0}
          <div class="space-y-1.5 bg-[#090d16] border border-[#1e293b] p-3 rounded-lg">
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
                <div class="{log.includes('[ERROR') ? 'text-rose-400' : log.includes('Finished') ? 'text-emerald-400 font-bold' : 'text-slate-300'}">
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
          onclick={() => handleStartExecution()}
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
