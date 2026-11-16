<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { quoteSqlIdentifier, stripDefiner, splitTopLevelList } from '../../utils/sqlStatements';
  import Icon from '../common/Icon.svelte';
  import SqlCodeEditor from '../editor/SqlCodeEditor.svelte';

  interface RoutineParam {
    mode: 'IN' | 'OUT' | 'INOUT';
    name: string;
    type: string;
  }

  let routineName = $state('');
  let origName = $state<string | null>(null);
  let routineType = $state<'PROCEDURE' | 'FUNCTION'>('PROCEDURE');
  let returnType = $state('INT');
  let isDeterministic = $state(true);
  let params = $state<RoutineParam[]>([]);
  let body = $state('BEGIN\n  -- Routine body statements\nEND');
  let isSubmitting = $state(false);
  let isLoadingDdl = $state(false);

  let driver = $derived(appState.activeConnection?.driver?.toLowerCase() || 'mysql');
  let isPostgres = $derived(driver === 'postgres' || driver === 'postgresql');

  let wasOpen = false;

  function parseRoutineDDL(raw: string, fallbackType: 'PROCEDURE' | 'FUNCTION') {
    const clean = stripDefiner(raw);
    const headerMatch = clean.match(/\bCREATE\s+(?:OR\s+REPLACE\s+)?(PROCEDURE|FUNCTION)\s+(?:`[^`]+`|"[^"]+"|\S+)\s*\(([\s\S]*)/i);
    if (!headerMatch) return;

    routineType = headerMatch[1].toUpperCase() === 'FUNCTION' ? 'FUNCTION' : fallbackType;
    const afterParen = headerMatch[2];
    let depth = 1;
    let q = '';
    let closeIdx = -1;
    for (let i = 0; i < afterParen.length; i++) {
      const ch = afterParen[i];
      if (q) {
        if (ch === '\\' && i + 1 < afterParen.length) { i++; continue; }
        if (ch === q) q = '';
        continue;
      }
      if (ch === "'" || ch === '"' || ch === '`') { q = ch; continue; }
      if (ch === '(') depth++;
      else if (ch === ')') {
        depth--;
        if (depth === 0) { closeIdx = i; break; }
      }
    }
    if (closeIdx === -1) return;

    const paramSection = afterParen.slice(0, closeIdx).trim();
    const rest = afterParen.slice(closeIdx + 1).trim();

    params = splitTopLevelList(paramSection).map((pStr) => {
      const m = pStr.match(/^(?:(INOUT|IN|OUT)\s+)?(`[^`]+`|"[^"]+"|\S+)\s+(.+)$/i);
      if (m) {
        return {
          mode: (m[1]?.toUpperCase() as any) || 'IN',
          name: m[2].replace(/^[`"]|[`"]$/g, ''),
          type: m[3].trim(),
        };
      }
      return { mode: 'IN', name: pStr, type: 'VARCHAR(255)' };
    });

    if (routineType === 'FUNCTION') {
      const retMatch = rest.match(/\bRETURNS\s+([^\n]+?)(?:\s+(?:NOT\s+DETERMINISTIC|DETERMINISTIC|NO\s+SQL|READS\s+SQL|COMMENT|BEGIN|AS|LANGUAGE|RETURN)\b|$)/i);
      if (retMatch) returnType = retMatch[1].trim();
    }
    if (/\bNOT\s+DETERMINISTIC\b/i.test(rest)) isDeterministic = false;
    else if (/\bDETERMINISTIC\b/i.test(rest)) isDeterministic = true;

    const beginIdx = rest.search(/\bBEGIN\b/i);
    if (beginIdx !== -1) {
      body = rest.slice(beginIdx).trim();
    } else {
      const retBodyIdx = rest.search(/\bRETURN\b/i);
      if (retBodyIdx !== -1) body = rest.slice(retBodyIdx).trim();
    }
  }

  async function hydrateFromServer(name: string, rType: 'PROCEDURE' | 'FUNCTION', fallbackDef?: string) {
    if (!appState.activeSessionId) return;
    isLoadingDdl = true;
    try {
      const res = await api.getRoutineDDL(appState.activeSessionId, name, rType);
      if (res?.ddl) {
        parseRoutineDDL(res.ddl, rType);
        return;
      }
      if (fallbackDef) body = stripDefiner(fallbackDef);
    } catch {
      if (fallbackDef) body = stripDefiner(fallbackDef);
    } finally {
      isLoadingDdl = false;
    }
  }

  $effect(() => {
    if (appState.isRoutineDesignerOpen) {
      if (!wasOpen) {
        const edit = appState.routineToEdit;
        if (edit && edit.name) {
          origName = edit.name;
          routineName = edit.name;
          routineType = (edit.type as any)?.toUpperCase() === 'FUNCTION' ? 'FUNCTION' : 'PROCEDURE';
          returnType = edit.data_type || 'INT';
          body = edit.definition || '-- Loading routine DDL...';
          params = [];
          void hydrateFromServer(edit.name, routineType, edit.definition);
        } else {
          origName = null;
          routineName = '';
          routineType = (edit?.type as any)?.toUpperCase() === 'FUNCTION' ? 'FUNCTION' : 'PROCEDURE';
          returnType = 'INT';
          isDeterministic = true;
          params = [{ mode: 'IN', name: 'p_id', type: 'INT' }];
          body = routineType === 'FUNCTION' ? 'BEGIN\n  RETURN 0;\nEND' : 'BEGIN\n  -- Statements\nEND';
        }
        isSubmitting = false;
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  function addParam() {
    params.push({
      mode: 'IN',
      name: `p_${params.length + 1}`,
      type: 'VARCHAR(255)',
    });
  }

  function removeParam(index: number) {
    params.splice(index, 1);
  }

  let generatedDDL = $derived.by(() => {
    const cleanName = routineName.trim() || (routineType === 'FUNCTION' ? 'fn_sample' : 'sp_sample');
    const qName = quoteSqlIdentifier(cleanName, driver);
    const cleanBody = body.trim().replace(/;+$/, '');

    const paramStr = params
      .filter((p) => p.name.trim())
      .map((p) => {
        const pName = quoteSqlIdentifier(p.name.trim(), driver);
        if (routineType === 'FUNCTION' || isPostgres) {
          return routineType === 'FUNCTION' ? `${pName} ${p.type.trim()}` : `${p.mode} ${pName} ${p.type.trim()}`;
        }
        return `${p.mode} ${pName} ${p.type.trim()}`;
      })
      .join(', ');

    const createKw = isPostgres ? 'CREATE OR REPLACE' : 'CREATE';
    let header = routineType === 'FUNCTION'
      ? `${createKw} FUNCTION ${qName}(${paramStr}) RETURNS ${returnType.trim() || 'INT'}`
      : `${createKw} PROCEDURE ${qName}(${paramStr})`;

    if (!isPostgres && routineType === 'FUNCTION') {
      header += isDeterministic ? ' DETERMINISTIC' : ' NOT DETERMINISTIC';
    }

    const dropPrefix =
      !isPostgres && origName
        ? `DROP ${routineType} IF EXISTS ${quoteSqlIdentifier(origName, driver)};\n`
        : isPostgres && origName && origName !== cleanName
        ? `DROP ${routineType} IF EXISTS ${quoteSqlIdentifier(origName, driver)};\n`
        : '';

    return `${dropPrefix}${header}\n${cleanBody};`;
  });

  function openInEditor() {
    const sqlText = isPostgres
      ? generatedDDL
      : `DELIMITER ;;\n${generatedDDL.replace(/;$/, ';;')}\nDELIMITER ;`;
    appState.addTab(`${routineName.trim() || 'routine'}.sql`, sqlText);
    appState.isRoutineDesignerOpen = false;
  }

  function handleCreate() {
    if (!routineName.trim()) {
      appState.showToast('Routine name is required', 'error');
      return;
    }
    if (routineType === 'FUNCTION' && !returnType.trim()) {
      appState.showToast('Function return type is required', 'error');
      return;
    }
    if (!body.trim()) {
      appState.showToast('Routine body statements are required', 'error');
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
          appState.showToast(`${routineType === 'FUNCTION' ? 'Function' : 'Procedure'} "${routineName.trim()}" saved successfully`, 'success');
          appState.isRoutineDesignerOpen = false;
          await appState.refreshCatalog();
        } finally {
          isSubmitting = false;
        }
      },
      `${origName ? 'Update' : 'Create'} ${routineType}: ${routineName.trim()}`,
      { database: db }
    );
  }

  function handleClose() {
    appState.isRoutineDesignerOpen = false;
  }
</script>

{#if appState.isRoutineDesignerOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="routine-designer-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-2xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400">
            <Icon name="code" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="routine-designer-title" class="font-bold text-sm text-slate-100 font-sans">
              {origName ? `Edit Routine "${origName}"` : `Create Stored ${routineType === 'FUNCTION' ? 'Function' : 'Procedure'}`}
            </h3>
            <p class="text-[11px] text-slate-400">
              Visual designer for database procedures, functions, parameters, and return types
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
        <!-- Name & Type -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
          <div class="md:col-span-2 space-y-1">
            <label for="routine-name" class="text-xs font-semibold text-slate-300">Name *</label>
            <input
              id="routine-name"
              type="text"
              bind:value={routineName}
              placeholder={routineType === 'FUNCTION' ? 'e.g. fn_calculate_total' : 'e.g. sp_archive_orders'}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-1.5 text-xs text-white font-mono outline-none"
            />
          </div>

          <div class="space-y-1">
            <label for="routine-type" class="text-xs font-semibold text-slate-300">Routine Type</label>
            <select
              id="routine-type"
              bind:value={routineType}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
            >
              <option value="PROCEDURE">PROCEDURE</option>
              <option value="FUNCTION">FUNCTION</option>
            </select>
          </div>
        </div>

        <!-- Returns Type (for Function) & Deterministic -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3 items-center">
          {#if routineType === 'FUNCTION'}
            <div class="space-y-1">
              <label for="routine-returns" class="text-xs font-semibold text-slate-300">Returns Type *</label>
              <input
                id="routine-returns"
                type="text"
                bind:value={returnType}
                placeholder="e.g. INT, VARCHAR(255), DECIMAL(10,2)"
                class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-1.5 text-xs text-white font-mono outline-none"
              />
            </div>
          {/if}

          <div class="flex items-center space-x-2 pt-4">
            <input
              id="routine-deterministic"
              type="checkbox"
              bind:checked={isDeterministic}
              class="rounded bg-slate-900 border-slate-700 text-sky-500 focus:ring-0"
            />
            <label for="routine-deterministic" class="text-xs text-slate-300 select-none cursor-pointer">
              Deterministic (same inputs always yield same result)
            </label>
          </div>
        </div>

        <!-- Parameters Section -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-slate-300">Parameters ({params.length})</span>
            <button
              onclick={addParam}
              type="button"
              class="text-xs text-sky-400 hover:text-sky-300 flex items-center space-x-1 font-semibold"
            >
              <Icon name="plus" class="w-3.5 h-3.5" />
              <span>Add Parameter</span>
            </button>
          </div>

          {#if params.length === 0}
            <div class="p-3 text-center text-xs text-slate-500 bg-[#090d16] border border-dashed border-slate-800 rounded">
              No parameters configured. Click "Add Parameter" if required.
            </div>
          {:else}
            <div class="space-y-1.5 max-h-40 overflow-y-auto">
              {#each params as param, index}
                <div class="flex items-center space-x-2 bg-[#090d16] border border-[#1e293b] p-1.5 rounded">
                  {#if routineType !== 'FUNCTION'}
                    <select
                      bind:value={param.mode}
                      class="bg-slate-900 border border-slate-700 rounded px-2 py-1 text-xs text-slate-200 font-mono outline-none w-20 shrink-0"
                    >
                      <option value="IN">IN</option>
                      <option value="OUT">OUT</option>
                      <option value="INOUT">INOUT</option>
                    </select>
                  {/if}

                  <input
                    type="text"
                    bind:value={param.name}
                    placeholder="param_name"
                    class="flex-1 bg-slate-900 border border-slate-700 rounded px-2 py-1 text-xs text-white font-mono outline-none focus:border-sky-500"
                  />

                  <input
                    type="text"
                    bind:value={param.type}
                    placeholder="Type (e.g. INT)"
                    class="w-36 bg-slate-900 border border-slate-700 rounded px-2 py-1 text-xs text-slate-200 font-mono outline-none focus:border-sky-500"
                  />

                  <button
                    onclick={() => removeParam(index)}
                    type="button"
                    class="text-slate-500 hover:text-rose-400 p-1 rounded"
                    title="Remove parameter"
                    aria-label="Remove parameter"
                  >
                    <Icon name="x" class="w-3.5 h-3.5" />
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Body Statements -->
        <div class="space-y-1">
          <label for="routine-body" class="text-xs font-semibold text-slate-300 flex items-center justify-between">
            <span>Routine Body Statements *</span>
            <span class="text-[10px] text-slate-500 font-mono">{isLoadingDdl ? 'Loading server DDL...' : 'BEGIN ... END block'}</span>
          </label>
          <div class="h-44">
            <SqlCodeEditor
              id="routine-body"
              bind:value={body}
              placeholder={'BEGIN\n  SELECT * FROM users;\nEND'}
            />
          </div>
        </div>

        <!-- DDL Preview -->
        <div class="space-y-1">
          <div class="text-[11px] font-semibold text-slate-400 uppercase tracking-wider flex items-center space-x-1">
            <Icon name="file-code" class="w-3.5 h-3.5 text-sky-400" />
            <span>Generated DDL Preview</span>
          </div>
          <pre class="p-3 rounded bg-[#090d16] border border-slate-800 text-[11px] font-mono text-sky-300/90 overflow-x-auto whitespace-pre-wrap select-text">{generatedDDL}</pre>
        </div>
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0">
        <button
          onclick={openInEditor}
          type="button"
          class="px-3 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition border border-[#334155]"
        >
          <Icon name="edit" class="w-3.5 h-3.5 text-sky-400" />
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
            disabled={isSubmitting || !routineName.trim()}
            type="button"
            class="px-4 py-1.5 bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow-lg shadow-sky-950/40"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Saving...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>{origName ? `Review & Update` : `Review & Create`}</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
