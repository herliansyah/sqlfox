<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface RoutineParam {
    mode: 'IN' | 'OUT' | 'INOUT';
    name: string;
    type: string;
  }

  let routineName = $state('');
  let routineType = $state<'PROCEDURE' | 'FUNCTION'>('PROCEDURE');
  let returnType = $state('INT');
  let isDeterministic = $state(true);
  let params = $state<RoutineParam[]>([]);
  let body = $state('BEGIN\n  -- Routine body statements\nEND');
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isRoutineDesignerOpen) {
      if (appState.routineToEdit) {
        routineName = appState.routineToEdit.name;
        routineType = (appState.routineToEdit.type as any) === 'FUNCTION' ? 'FUNCTION' : 'PROCEDURE';
        returnType = appState.routineToEdit.data_type || 'INT';
        body = appState.routineToEdit.definition || 'BEGIN\n  -- Routine body statements\nEND';
        params = [];
      } else {
        routineName = '';
        routineType = 'PROCEDURE';
        returnType = 'INT';
        isDeterministic = true;
        params = [
          { mode: 'IN', name: 'p_id', type: 'INT' },
        ];
        body = 'BEGIN\n  -- Statements\nEND';
      }
      isSubmitting = false;
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
    const driver = appState.activeConnection?.driver;
    const isSQLite = driver === 'sqlite';
    const isPostgres = driver === 'postgres';
    const quote = isSQLite || isPostgres ? '"' : '`';
    const cleanName = routineName.trim() || (routineType === 'FUNCTION' ? 'fn_sample' : 'sp_sample');
    const cleanBody = body.trim().replace(/;+$/, '');

    const paramStr = params
      .filter((p) => p.name.trim())
      .map((p) => {
        const pName = `${quote}${p.name.trim()}${quote}`;
        if (routineType === 'FUNCTION') {
          return `${pName} ${p.type.trim()}`;
        }
        return `${p.mode} ${pName} ${p.type.trim()}`;
      })
      .join(', ');

    let header = routineType === 'FUNCTION'
      ? `CREATE FUNCTION ${quote}${cleanName}${quote}(${paramStr}) RETURNS ${returnType.trim() || 'INT'}`
      : `CREATE PROCEDURE ${quote}${cleanName}${quote}(${paramStr})`;

    if (isDeterministic) {
      header += ' DETERMINISTIC';
    } else {
      header += ' NOT DETERMINISTIC';
    }

    return `${header}\n${cleanBody};`;
  });

  function openInEditor() {
    appState.addTab(`create_${routineType.toLowerCase()}_${routineName.trim() || 'routine'}.sql`, generatedDDL);
    appState.isRoutineDesignerOpen = false;
  }

  async function handleCreate() {
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
    if (!appState.activeSessionId) return;

    isSubmitting = true;
    try {
      const res = await api.executeQuery(appState.activeSessionId, generatedDDL, false);
      if (res.error) {
        appState.showToast(`Failed to save routine: ${res.error}`, 'error');
        return;
      }
      appState.showToast(`${routineType === 'FUNCTION' ? 'Function' : 'Procedure'} "${routineName.trim()}" created successfully`, 'success');
      appState.isRoutineDesignerOpen = false;
      await appState.refreshCatalog();
    } catch (err: any) {
      appState.showToast(`Error: ${err.message}`, 'error');
    } finally {
      isSubmitting = false;
    }
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
              {appState.routineToEdit ? `Edit Routine "${appState.routineToEdit.name}"` : `Create Stored ${routineType === 'FUNCTION' ? 'Function' : 'Procedure'}`}
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
            <span class="text-[10px] text-slate-500 font-mono">BEGIN ... END block</span>
          </label>
          <textarea
            id="routine-body"
            bind:value={body}
            rows="6"
            placeholder="BEGIN\n  SELECT * FROM users;\nEND"
            class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded p-2.5 text-xs text-slate-100 font-mono outline-none resize-y"
          ></textarea>
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
              <span>Create {routineType === 'FUNCTION' ? 'Function' : 'Procedure'}</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
