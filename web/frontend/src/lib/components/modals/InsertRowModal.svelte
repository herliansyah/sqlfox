<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { parseEnumValues } from '../../utils/dataTypes';
  import Icon from '../common/Icon.svelte';

  let currentTab = $derived(appState.activeTab);
  let activeCatalog = $derived(
    (currentTab?.database && appState.catalogs[currentTab.database.toLowerCase()]) || appState.catalog
  );
  let activeTableMeta = $derived.by(() => {
    if (!currentTab?.tableName || !activeCatalog?.tables) return null;
    return activeCatalog.tables.find(
      (t) => t.name.toLowerCase() === currentTab.tableName!.toLowerCase()
    );
  });

  let formData = $state<Record<string, any>>({});
  let nullCols = $state<Record<string, boolean>>({});
  let emptyStringCols = $state<Record<string, boolean>>({});
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isInsertRowModalOpen && !activeTableMeta && currentTab?.tableName && appState.activeSessionId) {
      void appState.loadCatalog(undefined, currentTab.database);
    }
  });

  let wasOpen = false;

  $effect(() => {
    if (appState.isInsertRowModalOpen && activeTableMeta) {
      if (!wasOpen) {
        const initial: Record<string, any> = {};
        const nulls: Record<string, boolean> = {};
        activeTableMeta.columns.forEach((col) => {
          if (!col.is_primary_key && col.default_value !== undefined && col.default_value !== null) {
            initial[col.name] = col.default_value;
          } else {
            initial[col.name] = '';
          }
          nulls[col.name] = false;
        });
        formData = initial;
        nullCols = nulls;
        emptyStringCols = {};
        wasOpen = true;
      }
    } else if (!appState.isInsertRowModalOpen) {
      wasOpen = false;
    }
  });

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!currentTab?.tableName || appState.isReadOnly) {
      if (appState.isReadOnly) appState.showToast('Read-only mode is active', 'warning');
      return;
    }
    isSubmitting = true;
    try {
      const cleaned: Record<string, any> = {};
      for (const [k, v] of Object.entries(formData)) {
        if (nullCols[k]) {
          cleaned[k] = null;
        } else if (emptyStringCols[k]) {
          cleaned[k] = '';
        } else if (v !== '' && v !== undefined) {
          cleaned[k] = v;
        }
      }
      await appState.insertRow(cleaned);
    } finally {
      isSubmitting = false;
    }
  }
</script>

{#if appState.isInsertRowModalOpen && currentTab?.tableName}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm select-none">
    <div class="w-full max-w-lg bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl flex flex-col max-h-[85vh] overflow-hidden">
      <!-- Header -->
      <div class="px-5 py-3 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-2.5">
          <Icon name="plus" class="w-5 h-5 text-emerald-400" />
          <h3 class="font-bold text-sm text-slate-100">
            Insert Row into <span class="font-mono text-emerald-400 font-semibold">{currentTab.tableName}</span>
          </h3>
        </div>
        <button
          onclick={() => (appState.isInsertRowModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Form Body -->
      <form onsubmit={handleSubmit} class="p-5 overflow-y-auto space-y-3.5 text-xs">
        {#if appState.isReadOnly}
          <div class="p-2.5 rounded-lg bg-amber-950/50 border border-amber-800/70 text-amber-300 text-[11px] flex items-center space-x-2">
            <Icon name="lock" class="w-3.5 h-3.5 shrink-0" />
            <span>Session is in Read-Only mode. Row insertion is blocked.</span>
          </div>
        {/if}

        {#if activeTableMeta}
          {#each activeTableMeta.columns as col}
            {@const tLow = (col.type || '').toLowerCase()}
            {@const enumVals = parseEnumValues(col.type || '')}
            {@const isBool = tLow === 'bool' || tLow === 'boolean' || tLow === 'tinyint(1)'}
            {@const isLong = tLow.includes('text') || tLow.includes('json')}
            {@const isNum = /(int|decimal|numeric|float|double|real|serial)/.test(tLow) && !isBool}
            <div class="space-y-1">
              <div class="flex items-center justify-between text-slate-300">
                <label class="font-mono font-semibold flex items-center space-x-1.5">
                  {#if col.is_primary_key}
                    <Icon name="key" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
                  {/if}
                  <span>{col.name}</span>
                </label>
                <div class="flex items-center space-x-1.5 font-mono text-[10px] text-slate-400">
                  <span class="bg-slate-900 px-1 rounded border border-slate-800 uppercase">{col.type}</span>
                  {#if col.is_nullable}
                    <span class="text-slate-500">NULL</span>
                  {:else}
                    <span class="text-rose-400 font-bold">REQUIRED</span>
                  {/if}
                </div>
              </div>

              <div class="flex items-center space-x-1.5">
                {#if enumVals.length > 0}
                  <select
                    disabled={nullCols[col.name]}
                    value={formData[col.name] ?? ''}
                    onchange={(e) => {
                      nullCols[col.name] = false;
                      emptyStringCols[col.name] = false;
                      formData[col.name] = (e.currentTarget as HTMLSelectElement).value;
                    }}
                    class="flex-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-emerald-500 disabled:opacity-40"
                  >
                    <option value="">(Default / Choose value)</option>
                    {#each enumVals as ev}
                      <option value={ev}>{ev}</option>
                    {/each}
                  </select>
                {:else if isBool}
                  <div class="flex-1 flex items-center space-x-1.5">
                    {#each [{ label: 'TRUE', v: '1' }, { label: 'FALSE', v: '0' }] as opt}
                      <button
                        type="button"
                        onclick={() => {
                          nullCols[col.name] = false;
                          emptyStringCols[col.name] = false;
                          formData[col.name] = opt.v;
                        }}
                        class="px-3 py-1 rounded text-[11px] font-mono border transition {!nullCols[col.name] && String(formData[col.name]) === opt.v
                          ? 'bg-emerald-500/20 border-emerald-500 text-emerald-300 font-bold'
                          : 'bg-[#090d16] border-[#334155] text-slate-400 hover:text-white'}"
                      >
                        {opt.label}
                      </button>
                    {/each}
                  </div>
                {:else if isLong}
                  <textarea
                    rows="2"
                    disabled={nullCols[col.name]}
                    value={formData[col.name] ?? ''}
                    oninput={(e) => {
                      nullCols[col.name] = false;
                      emptyStringCols[col.name] = false;
                      formData[col.name] = (e.currentTarget as HTMLTextAreaElement).value;
                    }}
                    placeholder={col.default_value ? `Default: ${col.default_value}` : 'Enter text or JSON...'}
                    class="flex-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 font-mono placeholder-slate-600 focus:outline-none focus:border-emerald-500 disabled:opacity-40"
                  ></textarea>
                {:else}
                  <input
                    type={isNum ? 'number' : 'text'}
                    step={/(decimal|numeric|float|double|real)/.test(tLow) ? 'any' : undefined}
                    disabled={nullCols[col.name]}
                    value={nullCols[col.name] ? '' : (formData[col.name] ?? '')}
                    oninput={(e) => {
                      nullCols[col.name] = false;
                      emptyStringCols[col.name] = false;
                      formData[col.name] = (e.currentTarget as HTMLInputElement).value;
                    }}
                    placeholder={nullCols[col.name] ? 'NULL' : emptyStringCols[col.name] ? '"" (empty string)' : col.is_primary_key ? '(Auto-generated or leave blank)' : col.default_value ? `Default: ${col.default_value}` : ''}
                    class="flex-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 font-mono placeholder-slate-600 focus:outline-none focus:border-emerald-500 disabled:opacity-40"
                  />
                {/if}

                {#if !isNum && !isBool}
                  <button
                    type="button"
                    onclick={() => {
                      nullCols[col.name] = false;
                      emptyStringCols[col.name] = !emptyStringCols[col.name];
                      if (emptyStringCols[col.name]) formData[col.name] = '';
                    }}
                    class="px-2 py-1.5 rounded text-[10px] font-mono border transition {emptyStringCols[col.name] ? 'bg-sky-950 border-sky-600 text-sky-300 font-bold' : 'bg-slate-900 hover:bg-slate-800 text-slate-400 border-slate-700'}"
                    title="Send explicit empty string ''"
                  >
                    ""
                  </button>
                {/if}

                {#if col.is_nullable}
                  <button
                    type="button"
                    onclick={() => {
                      emptyStringCols[col.name] = false;
                      nullCols[col.name] = !nullCols[col.name];
                      if (nullCols[col.name]) formData[col.name] = null;
                    }}
                    class="px-2 py-1.5 rounded text-[10px] font-mono border transition {nullCols[col.name] ? 'bg-rose-950 border-rose-600 text-rose-300 font-bold' : 'bg-slate-900 hover:bg-slate-800 text-slate-400 hover:text-rose-300 border-slate-700'}"
                    title="Toggle explicit SQL NULL"
                  >
                    NULL
                  </button>
                {/if}
              </div>
            </div>
          {/each}
        {:else}
          <div class="p-4 text-center text-slate-500 italic">
            Column definitions loading...
          </div>
        {/if}

        <!-- Actions -->
        <div class="pt-3 border-t border-[#334155] flex items-center justify-end space-x-2">
          <button
            type="button"
            onclick={() => (appState.isInsertRowModalOpen = false)}
            class="px-3 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 rounded font-medium transition"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitting || appState.isReadOnly}
            class="px-4 py-1.5 bg-emerald-500 hover:bg-emerald-400 disabled:opacity-40 text-slate-950 rounded font-bold transition shadow-[0_0_12px_rgba(52,211,153,0.35)] flex items-center space-x-1.5"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Inserting...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Insert Record</span>
            {/if}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
