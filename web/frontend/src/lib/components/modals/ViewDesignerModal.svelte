<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';
  import SqlCodeEditor from '../editor/SqlCodeEditor.svelte';

  let viewName = $state('');
  let algorithm = $state<'UNDEFINED' | 'MERGE' | 'TEMPTABLE'>('UNDEFINED');
  let checkOption = $state<'NONE' | 'CASCADED' | 'LOCAL'>('NONE');
  let selectQuery = $state('SELECT * FROM ');
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isViewDesignerOpen) {
      if (appState.viewToEdit) {
        viewName = appState.viewToEdit.name;
        selectQuery = appState.viewToEdit.query || 'SELECT * FROM ';
      } else {
        viewName = '';
        algorithm = 'UNDEFINED';
        checkOption = 'NONE';
        const firstTable = appState.filteredTables[0]?.name || 'my_table';
        selectQuery = `SELECT id, created_at FROM \`${firstTable}\` WHERE 1=1;`;
      }
      isSubmitting = false;
    }
  });

  let generatedDDL = $derived.by(() => {
    const driver = appState.activeConnection?.driver;
    const isSQLite = driver === 'sqlite';
    const isPostgres = driver === 'postgres';
    const quote = isSQLite || isPostgres ? '"' : '`';
    const cleanName = viewName.trim() || 'v_sample_view';
    const cleanQuery = selectQuery.trim().replace(/;+$/, '');

    let sql = '';
    if (isSQLite) {
      sql = `CREATE VIEW IF NOT EXISTS ${quote}${cleanName}${quote} AS\n${cleanQuery};`;
    } else if (isPostgres) {
      let checkPart = checkOption !== 'NONE' ? `\nWITH ${checkOption} CHECK OPTION` : '';
      sql = `CREATE OR REPLACE VIEW ${quote}${cleanName}${quote} AS\n${cleanQuery}${checkPart};`;
    } else {
      let algoPart = algorithm !== 'UNDEFINED' ? ` ALGORITHM = ${algorithm}` : '';
      let checkPart = checkOption !== 'NONE' ? `\nWITH ${checkOption} CHECK OPTION` : '';
      sql = `CREATE OR REPLACE${algoPart} VIEW ${quote}${cleanName}${quote} AS\n${cleanQuery}${checkPart};`;
    }
    return sql;
  });

  function openInEditor() {
    appState.addTab(`create_view_${viewName.trim() || 'view'}.sql`, generatedDDL);
    appState.isViewDesignerOpen = false;
  }

  async function handleCreate() {
    if (!viewName.trim()) {
      appState.showToast('View name is required', 'error');
      return;
    }
    if (!selectQuery.trim()) {
      appState.showToast('SELECT query statement is required', 'error');
      return;
    }
    if (!appState.activeSessionId) return;

    isSubmitting = true;
    try {
      const res = await api.executeQuery(appState.activeSessionId, generatedDDL, false);
      if (res.error) {
        appState.showToast(`Failed to save view: ${res.error}`, 'error');
        return;
      }
      appState.showToast(`View "${viewName.trim()}" created successfully`, 'success');
      appState.isViewDesignerOpen = false;
      await appState.refreshCatalog();
    } catch (err: any) {
      appState.showToast(`Error: ${err.message}`, 'error');
    } finally {
      isSubmitting = false;
    }
  }

  function handleClose() {
    appState.isViewDesignerOpen = false;
  }
</script>

{#if appState.isViewDesignerOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="view-designer-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-2xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
            <Icon name="eye" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="view-designer-title" class="font-bold text-sm text-slate-100 font-sans">
              {appState.viewToEdit ? `Edit View "${appState.viewToEdit.name}"` : 'Create Database View'}
            </h3>
            <p class="text-[11px] text-slate-400">
              Design a virtual table view with real-time DDL generation
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
        <!-- View Name & Algorithm -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
          <div class="md:col-span-2 space-y-1">
            <label for="view-name" class="text-xs font-semibold text-slate-300">View Name *</label>
            <input
              id="view-name"
              type="text"
              bind:value={viewName}
              placeholder="e.g. v_active_users"
              class="w-full bg-[#090d16] border border-[#334155] focus:border-emerald-500 rounded px-3 py-1.5 text-xs text-white font-mono outline-none"
            />
          </div>

          <div class="space-y-1">
            <label for="view-algorithm" class="text-xs font-semibold text-slate-300">Algorithm</label>
            <select
              id="view-algorithm"
              bind:value={algorithm}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-emerald-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
            >
              <option value="UNDEFINED">UNDEFINED (Default)</option>
              <option value="MERGE">MERGE</option>
              <option value="TEMPTABLE">TEMPTABLE</option>
            </select>
          </div>
        </div>

        <!-- Check Option -->
        <div class="space-y-1">
          <label for="view-check" class="text-xs font-semibold text-slate-300">WITH CHECK OPTION</label>
          <select
            id="view-check"
            bind:value={checkOption}
            class="w-full bg-[#090d16] border border-[#334155] focus:border-emerald-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
          >
            <option value="NONE">None</option>
            <option value="CASCADED">CASCADED</option>
            <option value="LOCAL">LOCAL</option>
          </select>
        </div>

        <!-- SELECT Query Statement -->
        <div class="space-y-1">
          <label for="view-query" class="text-xs font-semibold text-slate-300 flex items-center justify-between">
            <span>SELECT Query Statement *</span>
            <span class="text-[10px] text-slate-500 font-mono">SQL SELECT definition</span>
          </label>
          <div class="h-36">
            <SqlCodeEditor
              id="view-query"
              bind:value={selectQuery}
              placeholder="SELECT id, name, created_at FROM users WHERE status = 'active'"
            />
          </div>
        </div>

        <!-- DDL Preview -->
        <div class="space-y-1">
          <div class="text-[11px] font-semibold text-slate-400 uppercase tracking-wider flex items-center space-x-1">
            <Icon name="file-code" class="w-3.5 h-3.5 text-emerald-400" />
            <span>Generated DDL Preview</span>
          </div>
          <pre class="p-3 rounded bg-[#090d16] border border-slate-800 text-[11px] font-mono text-emerald-300/90 overflow-x-auto whitespace-pre-wrap select-text">{generatedDDL}</pre>
        </div>
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0">
        <button
          onclick={openInEditor}
          type="button"
          class="px-3 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition border border-[#334155]"
        >
          <Icon name="edit" class="w-3.5 h-3.5 text-emerald-400" />
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
            disabled={isSubmitting || !viewName.trim()}
            type="button"
            class="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow-lg shadow-emerald-950/40"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Saving...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Create View</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
