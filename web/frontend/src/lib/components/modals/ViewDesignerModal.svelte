<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { quoteSqlIdentifier, stripDefiner } from '../../utils/sqlStatements';
  import Icon from '../common/Icon.svelte';
  import SqlCodeEditor from '../editor/SqlCodeEditor.svelte';

  let viewName = $state('');
  let origName = $state<string | null>(null);
  let algorithm = $state<'UNDEFINED' | 'MERGE' | 'TEMPTABLE'>('UNDEFINED');
  let checkOption = $state<'NONE' | 'CASCADED' | 'LOCAL'>('NONE');
  let selectQuery = $state('SELECT * FROM ');
  let isSubmitting = $state(false);
  let isLoadingDdl = $state(false);

  let driver = $derived(appState.activeConnection?.driver?.toLowerCase() || 'mysql');
  let isSQLite = $derived(driver === 'sqlite');
  let isPostgres = $derived(driver === 'postgres' || driver === 'postgresql');

  let wasOpen = false;

  async function hydrateFromServer(name: string, fallbackQuery?: string) {
    if (!appState.activeSessionId) return;
    isLoadingDdl = true;
    try {
      const res = await api.getTableDDL(appState.activeSessionId, name, appState.currentDatabase || undefined);
      const clean = stripDefiner(res?.ddl || '');
      if (clean) {
        const algoMatch = clean.match(/\bALGORITHM\s*=\s*(UNDEFINED|MERGE|TEMPTABLE)\b/i);
        if (algoMatch) algorithm = algoMatch[1].toUpperCase() as any;

        const checkMatch = clean.match(/\bWITH\s+(CASCADED|LOCAL)\s+CHECK\s+OPTION\b/i);
        checkOption = checkMatch ? (checkMatch[1].toUpperCase() as any) : 'NONE';

        const asIdx = clean.search(/\bAS\s+(?:SELECT|WITH|\()/i);
        if (asIdx !== -1) {
          let body = clean.slice(asIdx).replace(/^AS\s+/i, '').trim();
          body = body.replace(/\bWITH\s+(?:CASCADED|LOCAL)?\s*CHECK\s+OPTION\s*;?\s*$/i, '').trim();
          selectQuery = body;
          return;
        }
      }
      if (fallbackQuery) selectQuery = fallbackQuery;
    } catch {
      if (fallbackQuery) selectQuery = fallbackQuery;
    } finally {
      isLoadingDdl = false;
    }
  }

  $effect(() => {
    if (appState.isViewDesignerOpen) {
      if (!wasOpen) {
        algorithm = 'UNDEFINED';
        checkOption = 'NONE';
        if (appState.viewToEdit?.name) {
          origName = appState.viewToEdit.name;
          viewName = appState.viewToEdit.name;
          selectQuery = appState.viewToEdit.query || '-- Loading view definition...';
          void hydrateFromServer(appState.viewToEdit.name, appState.viewToEdit.query);
        } else {
          origName = null;
          viewName = '';
          const firstTable = appState.filteredTables[0]?.name || 'my_table';
          const qTbl = quoteSqlIdentifier(firstTable, driver);
          selectQuery = `SELECT id, created_at FROM ${qTbl} WHERE 1=1;`;
        }
        isSubmitting = false;
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  let generatedDDL = $derived.by(() => {
    const cleanName = viewName.trim() || 'v_sample_view';
    const qName = quoteSqlIdentifier(cleanName, driver);
    const cleanQuery = selectQuery.trim().replace(/;+$/, '');
    const renamed = Boolean(origName && origName !== cleanName);
    const dropPrefix = renamed && origName ? `DROP VIEW IF EXISTS ${quoteSqlIdentifier(origName, driver)};\n` : '';

    if (isSQLite) {
      const sqliteDrop = origName ? `DROP VIEW IF EXISTS ${quoteSqlIdentifier(origName, driver)};\n` : '';
      return `${sqliteDrop}CREATE VIEW IF NOT EXISTS ${qName} AS\n${cleanQuery};`;
    }
    if (isPostgres) {
      const checkPart = checkOption !== 'NONE' ? `\nWITH ${checkOption} CHECK OPTION` : '';
      return `${dropPrefix}CREATE OR REPLACE VIEW ${qName} AS\n${cleanQuery}${checkPart};`;
    }
    const algoPart = algorithm !== 'UNDEFINED' ? ` ALGORITHM = ${algorithm}` : '';
    const checkPart = checkOption !== 'NONE' ? `\nWITH ${checkOption} CHECK OPTION` : '';
    return `${dropPrefix}CREATE OR REPLACE${algoPart} VIEW ${qName} AS\n${cleanQuery}${checkPart};`;
  });

  function openInEditor() {
    appState.addTab(`view_${viewName.trim() || 'sample'}.sql`, generatedDDL);
    appState.isViewDesignerOpen = false;
  }

  function handleCreate() {
    if (!viewName.trim()) {
      appState.showToast('View name is required', 'error');
      return;
    }
    if (!selectQuery.trim()) {
      appState.showToast('SELECT query statement is required', 'error');
      return;
    }
    if (appState.isReadOnly) {
      appState.showToast('Session is Read-Only', 'warning');
      return;
    }
    if (!appState.activeSessionId) return;

    const ddl = generatedDDL;
    const db = appState.currentDatabase || undefined;
    appState.openDDLReviewModal(
      ddl,
      async () => {
        isSubmitting = true;
        try {
          const res = await api.executeQuery(appState.activeSessionId!, ddl, { confirmed: true, database: db });
          if (res.error) throw new Error(res.error);
          appState.showToast(`View "${viewName.trim()}" saved successfully`, 'success');
          appState.isViewDesignerOpen = false;
          await appState.refreshCatalog();
        } finally {
          isSubmitting = false;
        }
      },
      `${origName ? 'Update View' : 'Create View'}: ${viewName.trim()}`,
      { database: db }
    );
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

          {#if !isSQLite && !isPostgres}
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
          {/if}
        </div>

        {#if !isSQLite}
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
        {/if}

        <!-- SELECT Query Statement -->
        <div class="space-y-1">
          <label for="view-query" class="text-xs font-semibold text-slate-300 flex items-center justify-between">
            <span>SELECT Query Statement *</span>
            <span class="text-[10px] text-slate-500 font-mono">{isLoadingDdl ? 'Fetching DDL from server...' : 'SQL SELECT definition'}</span>
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
              <span>{origName ? 'Review & Update View' : 'Review & Create View'}</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
