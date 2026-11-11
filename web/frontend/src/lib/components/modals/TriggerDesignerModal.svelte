<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let triggerName = $state('');
  let targetTable = $state('');
  let timing = $state<'BEFORE' | 'AFTER'>('BEFORE');
  let event = $state<'INSERT' | 'UPDATE' | 'DELETE'>('INSERT');
  let body = $state('BEGIN\n  -- Trigger logic here\nEND;');
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isTriggerDesignerOpen) {
      if (appState.triggerToEdit) {
        triggerName = appState.triggerToEdit.name;
        targetTable = appState.triggerToEdit.table;
        timing = (appState.triggerToEdit.timing as any) || 'BEFORE';
        event = (appState.triggerToEdit.event as any) || 'INSERT';
        body = appState.triggerToEdit.statement || 'BEGIN\n  -- Trigger logic here\nEND;';
      } else {
        triggerName = '';
        targetTable = appState.filteredTables[0]?.name || '';
        timing = 'BEFORE';
        event = 'INSERT';
        body = 'BEGIN\n  -- Trigger logic here\nEND;';
      }
      isSubmitting = false;
    }
  });

  let generatedDDL = $derived.by(() => {
    const driver = appState.activeConnection?.driver;
    const isSQLite = driver === 'sqlite';
    const isPostgres = driver === 'postgres';
    const quote = isSQLite || isPostgres ? '"' : '`';
    const cleanName = triggerName.trim() || 'trg_sample';
    const cleanTable = targetTable.trim() || 'target_table';
    const cleanBody = body.trim().replace(/;+$/, '');

    let sql = '';
    if (isSQLite) {
      sql = `CREATE TRIGGER ${quote}${cleanName}${quote} ${timing} ${event} ON ${quote}${cleanTable}${quote}\n${cleanBody};`;
    } else {
      sql = `CREATE TRIGGER ${quote}${cleanName}${quote} ${timing} ${event} ON ${quote}${cleanTable}${quote} FOR EACH ROW\n${cleanBody};`;
    }
    return sql;
  });

  function openInEditor() {
    appState.addTab(`create_trigger_${triggerName.trim() || 'trg'}.sql`, generatedDDL);
    appState.isTriggerDesignerOpen = false;
  }

  async function handleCreate() {
    if (!triggerName.trim()) {
      appState.showToast('Trigger name is required', 'error');
      return;
    }
    if (!targetTable.trim()) {
      appState.showToast('Target table is required', 'error');
      return;
    }
    if (!body.trim()) {
      appState.showToast('Trigger body statements are required', 'error');
      return;
    }
    if (!appState.activeSessionId) return;

    isSubmitting = true;
    try {
      const res = await api.executeQuery(appState.activeSessionId, generatedDDL, false);
      if (res.error) {
        appState.showToast(`Failed to save trigger: ${res.error}`, 'error');
        return;
      }
      appState.showToast(`Trigger "${triggerName.trim()}" created successfully`, 'success');
      appState.isTriggerDesignerOpen = false;
      await appState.refreshCatalog();
    } catch (err: any) {
      appState.showToast(`Error: ${err.message}`, 'error');
    } finally {
      isSubmitting = false;
    }
  }

  function handleClose() {
    appState.isTriggerDesignerOpen = false;
  }
</script>

{#if appState.isTriggerDesignerOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="trigger-designer-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-2xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-400">
            <Icon name="zap" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="trigger-designer-title" class="font-bold text-sm text-slate-100 font-sans">
              {appState.triggerToEdit ? `Edit Trigger "${appState.triggerToEdit.name}"` : 'Create Database Trigger'}
            </h3>
            <p class="text-[11px] text-slate-400">
              Visual event listener trigger designer for database tables
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
      <div class="p-5 overflow-y-auto space-y-4 flex-1">
        <!-- Trigger Name & Target Table -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div class="space-y-1">
            <label for="trigger-name" class="text-xs font-semibold text-slate-300">Trigger Name *</label>
            <input
              id="trigger-name"
              type="text"
              bind:value={triggerName}
              placeholder="e.g. trg_users_before_insert"
              class="w-full bg-[#090d16] border border-[#334155] focus:border-amber-500 rounded px-3 py-1.5 text-xs text-white font-mono outline-none"
            />
          </div>

          <div class="space-y-1">
            <label for="trigger-table" class="text-xs font-semibold text-slate-300">Target Table *</label>
            <select
              id="trigger-table"
              bind:value={targetTable}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-amber-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
            >
              {#each appState.filteredTables as tbl}
                <option value={tbl.name}>{tbl.name}</option>
              {/each}
            </select>
          </div>
        </div>

        <!-- Timing & Event -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div class="space-y-1">
            <label for="trigger-timing" class="text-xs font-semibold text-slate-300">Timing</label>
            <select
              id="trigger-timing"
              bind:value={timing}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-amber-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
            >
              <option value="BEFORE">BEFORE (Prior to row operation)</option>
              <option value="AFTER">AFTER (Following row operation)</option>
            </select>
          </div>

          <div class="space-y-1">
            <label for="trigger-event" class="text-xs font-semibold text-slate-300">Event</label>
            <select
              id="trigger-event"
              bind:value={event}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-amber-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
            >
              <option value="INSERT">INSERT</option>
              <option value="UPDATE">UPDATE</option>
              <option value="DELETE">DELETE</option>
            </select>
          </div>
        </div>

        <!-- Body Statements -->
        <div class="space-y-1">
          <label for="trigger-body" class="text-xs font-semibold text-slate-300 flex items-center justify-between">
            <span>Trigger Body Statements *</span>
            <span class="text-[10px] text-slate-500 font-mono">BEGIN ... END block</span>
          </label>
          <textarea
            id="trigger-body"
            bind:value={body}
            rows="6"
            placeholder="BEGIN\n  SET NEW.updated_at = NOW();\nEND;"
            class="w-full bg-[#090d16] border border-[#334155] focus:border-amber-500 rounded p-2.5 text-xs text-slate-100 font-mono outline-none resize-y"
          ></textarea>
        </div>

        <!-- DDL Preview -->
        <div class="space-y-1">
          <div class="text-[11px] font-semibold text-slate-400 uppercase tracking-wider flex items-center space-x-1">
            <Icon name="file-code" class="w-3.5 h-3.5 text-amber-400" />
            <span>Generated DDL Preview</span>
          </div>
          <pre class="p-3 rounded bg-[#090d16] border border-slate-800 text-[11px] font-mono text-amber-300/90 overflow-x-auto whitespace-pre-wrap select-text">{generatedDDL}</pre>
        </div>
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0">
        <button
          onclick={openInEditor}
          type="button"
          class="px-3 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition border border-[#334155]"
        >
          <Icon name="edit" class="w-3.5 h-3.5 text-amber-400" />
          <span>Open in SQL Editor</span>
        </button>

        <div class="flex items-center space-x-2">
          <button
            onclick={handleClose}
            type="button"
            class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition"
          >
            Cancel
          </button>
          <button
            onclick={handleCreate}
            disabled={isSubmitting || !triggerName.trim() || !targetTable.trim()}
            type="button"
            class="px-4 py-1.5 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow-lg shadow-amber-950/40"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Saving...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Create Trigger</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
