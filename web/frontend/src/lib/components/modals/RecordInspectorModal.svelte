<script lang="ts">
import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let currentTab = $derived(appState.activeTab);
  let result = $derived(currentTab?.activeResult);
  let totalRows = $derived(result?.rows ? result.rows.length : 0);
  let rIdx = $derived(appState.recordInspectorRowIndex);
  let currentRow = $derived(result?.rows && result.rows[rIdx] ? result.rows[rIdx] : null);

  let editedFields = $state<Record<string, any>>({});
  let searchQuery = $state('');

  function getColName(c: any): string {
    return typeof c === 'string' ? c : c.name || '';
  }

  function getColType(c: any): string {
    return typeof c === 'object' ? c.type || '' : '';
  }

  function isColPK(c: any): boolean {
    return typeof c === 'object' ? Boolean(c.is_primary_key) : false;
  }

  $effect(() => {
    if (appState.isRecordInspectorOpen && currentRow) {
      editedFields = {};
    }
  });

  function handleFieldChange(colName: string, val: any) {
    editedFields[colName] = val;
  }

  function saveField(colName: string, cIdx: number) {
    if (!currentTab?.tableName || !currentRow) return;
    const newVal = editedFields[colName];
    const oldVal = currentRow[cIdx];
    if (newVal === undefined || newVal === oldVal) return;

    // Resolve PK column and value
    const pkColObj = result?.columns.find((c) => isColPK(c));
    const pkCol = pkColObj ? getColName(pkColObj) : undefined;
    const pkVal = pkColObj ? currentRow[result!.columns.indexOf(pkColObj)] : undefined;

    appState.stageEdit(currentTab.tableName, rIdx, cIdx, colName, pkCol, pkVal, oldVal, newVal);
    currentRow[cIdx] = newVal;
    appState.showToast(`Staged change for ${colName}`, 'info');
  }

  function navigate(delta: number) {
    const next = rIdx + delta;
    if (next >= 0 && next < totalRows) {
      appState.recordInspectorRowIndex = next;
    }
  }

  function formatValue(v: any): string {
    if (v === null || v === undefined) return '';
    if (typeof v === 'object') return JSON.stringify(v, null, 2);
    return String(v);
  }
</script>

{#if appState.isRecordInspectorOpen && currentRow && result?.columns}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm select-none">
    <div id="record-inspector-dialog" role="dialog" aria-modal="true" aria-label="Record inspector" class="w-full max-w-2xl bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl flex flex-col max-h-[85vh] overflow-hidden">
      <!-- Modal Header -->
      <div class="px-5 py-3 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-3">
          <Icon name="table" class="w-5 h-5 text-sky-400" />
          <div>
            <h3 class="font-bold text-sm text-slate-100 flex items-center space-x-2">
              <span>Record Inspector:</span>
              <span class="font-mono text-sky-400 font-semibold">{currentTab?.tableName || 'Table'}</span>
            </h3>
            <div class="text-[10px] text-slate-400 font-mono">
              Row {rIdx + 1} of {totalRows.toLocaleString()}
            </div>
          </div>
        </div>

        <!-- Prev / Next Navigation & Close -->
        <div class="flex items-center space-x-2">
          <div id="record-inspector-pager" class="flex items-center bg-[#1e293b] rounded border border-[#334155] p-0.5">
            <button
              onclick={() => navigate(-1)}
              disabled={rIdx <= 0}
              class="px-2 py-1 rounded hover:bg-slate-700 disabled:opacity-30 text-slate-300 disabled:hover:bg-transparent"
              title="Previous record"
            >
              <Icon name="chevron-left" class="w-3.5 h-3.5" />
            </button>
            <span class="px-2 text-[10px] font-mono text-slate-400">{rIdx + 1} / {totalRows}</span>
            <button
              onclick={() => navigate(1)}
              disabled={rIdx >= totalRows - 1}
              class="px-2 py-1 rounded hover:bg-slate-700 disabled:opacity-30 text-slate-300 disabled:hover:bg-transparent"
              title="Next record"
            >
              <Icon name="chevron-right" class="w-3.5 h-3.5" />
            </button>
          </div>

          <button
            onclick={() => (appState.isRecordInspectorOpen = false)}
            class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Quick Search inside fields -->
      <div class="px-5 py-2 border-b border-[#1e293b] bg-[#090d16]">
        <input
          id="record-inspector-search"
          type="text"
          bind:value={searchQuery}
          placeholder="Filter columns in this record..."
          class="w-full bg-[#0f172a] border border-[#334155] rounded px-3 py-1 text-xs text-slate-200 placeholder-slate-500 font-mono focus:outline-none focus:border-sky-500"
        />
      </div>

      <!-- Fields List -->
      <div id="record-inspector-fields" class="p-5 overflow-y-auto space-y-3.5 text-xs">
        {#each result.columns as col, cIdx}
          {@const colName = getColName(col)}
          {@const colType = getColType(col)}
          {@const isPK = isColPK(col)}
          {@const val = currentRow[cIdx]}
          {@const formatted = formatValue(val)}
          {#if !searchQuery || colName.toLowerCase().includes(searchQuery.toLowerCase())}
            <div class="bg-[#090d16] border border-[#1e293b] rounded-lg p-3 space-y-1.5 transition hover:border-[#334155]">
              <div class="flex items-center justify-between">
                <div class="flex items-center space-x-2">
                  {#if isPK}
                    <Icon name="key" class="w-3.5 h-3.5 text-amber-400 shrink-0" />
                  {/if}
                  <span class="font-mono font-bold text-slate-200">{colName}</span>
                  {#if colType}
                    <span class="text-[9px] font-mono text-slate-500 bg-slate-900 px-1 rounded border border-slate-800 uppercase">
                      {colType}
                    </span>
                  {/if}
                </div>

                <div class="flex items-center space-x-1">
                  {#if val === null}
                    <span class="text-[10px] text-rose-400 italic">NULL</span>
                  {/if}
                  <button
                    onclick={() => copyText(formatted)}
                    class="p-1 text-slate-500 hover:text-slate-300"
                    title="Copy value"
                  >
                    <Icon name="copy" class="w-3 h-3" />
                  </button>
                </div>
              </div>

              {#if formatted.length > 80 || formatted.includes('\n')}
                <textarea
                  rows="3"
                  value={editedFields[colName] !== undefined ? editedFields[colName] : formatted}
                  oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLTextAreaElement).value)}
                  onblur={() => saveField(colName, cIdx)}
                  class="w-full bg-[#0f172a] border border-[#334155] rounded p-2 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500"
                ></textarea>
              {:else}
                <div class="flex items-center space-x-2">
                  <input
                    type="text"
                    value={editedFields[colName] !== undefined ? editedFields[colName] : formatted}
                    oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLInputElement).value)}
                    onblur={() => saveField(colName, cIdx)}
                    class="flex-1 bg-[#0f172a] border border-[#334155] rounded px-2.5 py-1 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500"
                  />
                  <button
                    type="button"
                    onclick={() => {
                      handleFieldChange(colName, null);
                      saveField(colName, cIdx);
                    }}
                    class="px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-rose-300 rounded text-[10px] font-mono border border-slate-700"
                  >
                    NULL
                  </button>
                </div>
              {/if}
            </div>
          {/if}
        {/each}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#334155] bg-[#0b0f19] flex items-center justify-between text-xs">
        <span class="text-[11px] text-slate-500 font-mono">Edits auto-stage when you blur an input field</span>
        <button
          onclick={() => (appState.isRecordInspectorOpen = false)}
          class="px-4 py-1.5 bg-sky-500 hover:bg-sky-400 text-slate-950 font-bold rounded shadow transition"
        >
          Done
        </button>
      </div>
    </div>
  </div>
{/if}
