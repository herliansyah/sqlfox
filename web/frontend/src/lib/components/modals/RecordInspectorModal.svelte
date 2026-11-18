<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let currentTab = $derived(appState.activeTab);
  let result = $derived(currentTab?.activeResult);
  let totalRows = $derived(result?.rows ? result.rows.length : 0);
  let rIdx = $derived(appState.recordInspectorRowIndex);
  let currentRow = $derived(result?.rows && result.rows[rIdx] ? result.rows[rIdx] : null);
  let effectiveTable = $derived(currentTab?.tableName || currentTab?.detectedTableName || '');
  let canEdit = $derived(Boolean(effectiveTable && !appState.isReadOnly));

  let editedFields = $state<Record<string, any>>({});
  let searchQuery = $state('');

  let activeCatalog = $derived(
    (currentTab?.database && appState.catalogs[currentTab.database.toLowerCase()]) || appState.catalog
  );

  let tableMeta = $derived(
    activeCatalog?.tables?.find((t) => t.name.toLowerCase() === effectiveTable.toLowerCase()) || null
  );

  let fkMap = $derived.by(() => {
    const map = new Map<string, any>();
    for (const fk of tableMeta?.foreign_keys || []) {
      const col = (fk as any).column || (fk as any).column_name;
      if (col) map.set(col.toLowerCase(), fk);
    }
    return map;
  });

  function getColName(c: any): string {
    return typeof c === 'string' ? c : c.name || '';
  }

  function getColType(c: any): string {
    if (typeof c === 'object' && c.type) return String(c.type);
    const name = getColName(c);
    const meta = tableMeta?.columns?.find((cm) => cm.name.toLowerCase() === name.toLowerCase());
    return meta?.type || '';
  }

  function isBooleanType(colType: string): boolean {
    const t = colType.toLowerCase().trim();
    return t === 'bool' || t === 'boolean' || t === 'tinyint(1)' || t === 'bit(1)';
  }

  function isJsonType(colType: string, val: any): boolean {
    const t = colType.toLowerCase();
    if (t.includes('json')) return true;
    if (typeof val === 'string') {
      const trimmed = val.trim();
      if ((trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
        try {
          JSON.parse(trimmed);
          return true;
        } catch {
          return false;
        }
      }
    }
    return typeof val === 'object' && val !== null;
  }

  function isColPK(c: any): boolean {
    if (typeof c === 'object' && c.is_primary_key !== undefined) return Boolean(c.is_primary_key);
    const colName = getColName(c);
    const colMeta = tableMeta?.columns?.find((cm) => cm.name.toLowerCase() === colName.toLowerCase());
    return Boolean(colMeta?.is_primary_key);
  }

  $effect(() => {
    if (appState.isRecordInspectorOpen && currentRow) {
      editedFields = {};
    }
  });

  function handleFieldChange(colName: string, val: any) {
    editedFields[colName] = val;
  }

  function revertField(colName: string, cIdx: number) {
    delete editedFields[colName];
    editedFields = { ...editedFields };
    if (effectiveTable && currentRow) {
      const uid = appState.getRowUid(currentRow);
      const key = `${effectiveTable}:${uid}:${cIdx}`;
      const staged = appState.stagedEdits[key];
      if (staged) {
        currentRow[cIdx] = staged.oldVal;
        delete appState.stagedEdits[key];
        appState.stagedEdits = { ...appState.stagedEdits };
      }
    }
  }

  function saveField(colName: string, cIdx: number) {
    if (!canEdit || !effectiveTable || !currentRow || !result?.columns) return;
    const newVal = editedFields[colName];
    const oldVal = currentRow[cIdx];
    if (newVal === undefined || newVal === oldVal) return;

    // Resolve PK column and value
    const pkColObj = result.columns.find((c) => isColPK(c));
    const pkCol = pkColObj ? getColName(pkColObj) : undefined;
    const pkColIdx = pkCol ? result.columns.findIndex((c) => getColName(c) === pkCol) : -1;
    const pkVal = pkColIdx !== -1 ? currentRow[pkColIdx] : undefined;

    let rowCriteria: Record<string, any> | undefined;
    if (!pkCol) {
      rowCriteria = {};
      result.columns.forEach((c: any, idx: number) => {
        const colKey = getColName(c);
        rowCriteria![colKey] = currentRow[idx];
      });
    }

    appState.stageEdit(
      effectiveTable,
      rIdx,
      cIdx,
      colName,
      pkCol,
      pkVal,
      oldVal,
      newVal,
      currentTab?.database || appState.currentDatabase,
      rowCriteria
    );
    currentRow[cIdx] = newVal;
    delete editedFields[colName];
    editedFields = { ...editedFields };
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

  function copyRecordAsJSON() {
    if (!currentRow || !result?.columns) return;
    const obj: Record<string, any> = {};
    result.columns.forEach((c: any, idx: number) => {
      obj[getColName(c)] = currentRow[idx];
    });
    copyText(JSON.stringify(obj, null, 2));
    appState.showToast('Copied record as JSON', 'info');
  }

  function copyRecordAsText() {
    if (!currentRow || !result?.columns) return;
    const lines = result.columns.map((c: any, idx: number) => `${getColName(c)}: ${currentRow[idx] === null ? 'NULL' : formatValue(currentRow[idx])}`);
    copyText(lines.join('\n'));
    appState.showToast('Copied record as key-value text', 'info');
  }

  function handleInspectorKeydown(e: KeyboardEvent) {
    if (!appState.isRecordInspectorOpen) return;
    if (e.key === 'Escape') {
      e.stopPropagation();
      appState.isRecordInspectorOpen = false;
      return;
    }
    const target = e.target as HTMLElement | null;
    if (target && target.closest('input, textarea, select, [contenteditable="true"]')) return;
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      navigate(-1);
    } else if (e.key === 'ArrowRight' || e.key === ' ') {
      e.preventDefault();
      navigate(1);
    }
  }
</script>

<svelte:window onkeydown={handleInspectorKeydown} />

{#if appState.isRecordInspectorOpen && currentRow && result?.columns}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 select-none">
    <div id="record-inspector-dialog" role="dialog" aria-modal="true" aria-label="Record inspector" class="w-full max-w-2xl bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl flex flex-col max-h-[85vh] overflow-hidden">
      <!-- Modal Header -->
      <div class="px-5 py-3 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-3">
          <Icon name="table" class="w-5 h-5 text-sky-400" />
          <div>
            <h3 class="font-bold text-sm text-slate-100 flex items-center space-x-2">
              <span>Record Inspector:</span>
              <span class="font-mono text-sky-400 font-semibold">{effectiveTable || 'Query Result'}</span>
              {#if appState.isReadOnly}
                <span class="text-[10px] font-mono px-1.5 py-0.2 rounded bg-amber-950/70 border border-amber-800 text-amber-300">Read-Only</span>
              {/if}
            </h3>
            <div class="text-[10px] text-slate-400 font-mono">
              Row {rIdx + 1} of {totalRows.toLocaleString()}
            </div>
          </div>
        </div>

        <!-- Copy, Prev / Next Navigation & Close -->
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={copyRecordAsJSON}
            class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[10px] font-mono transition"
            title="Copy whole record as JSON"
          >
            Copy JSON
          </button>
          <button
            type="button"
            onclick={copyRecordAsText}
            class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[10px] font-mono transition"
            title="Copy whole record as text"
          >
            Copy Text
          </button>
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
          {@const fk = fkMap.get(colName.toLowerCase())}
          {@const isStaged = Boolean(effectiveTable && currentRow && appState.stagedEdits[`${effectiveTable}:${appState.getRowUid(currentRow)}:${cIdx}`])}
          {@const isDraftDirty = editedFields[colName] !== undefined && editedFields[colName] !== val}
          {@const isDirty = isStaged || isDraftDirty}
          {@const q = searchQuery.toLowerCase().trim()}
          {#if !q || colName.toLowerCase().includes(q) || colType.toLowerCase().includes(q)}
            <div class="bg-[#090d16] border {isDirty ? 'border-amber-500/60' : 'border-[#1e293b]'} rounded-lg p-3 space-y-1.5 transition hover:border-[#334155]">
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
                  {#if isDirty}
                    <span class="text-[10px] font-semibold text-amber-400">● Modified</span>
                  {/if}
                </div>

                <div class="flex items-center space-x-1.5">
                  {#if val === null}
                    <span class="text-[10px] text-rose-400 italic">NULL</span>
                  {/if}
                  {#if fk && val !== null && val !== undefined}
                    <button
                      type="button"
                      onclick={(e) => appState.openFkPeek(e, effectiveTable, colName, fk.ref_table || fk.referenced_table, fk.ref_col || fk.referenced_column, val, currentTab?.database)}
                      class="px-1.5 py-0.5 rounded bg-sky-950/70 hover:bg-sky-900 border border-sky-800/70 text-sky-300 text-[10px] font-mono flex items-center space-x-1 transition"
                      title="Peek referenced row in {fk.ref_table || fk.referenced_table}"
                    >
                      <Icon name="link" class="w-3 h-3" />
                      <span>FK</span>
                    </button>
                  {/if}
                  {#if isJsonType(colType, val)}
                    <button
                      type="button"
                      onclick={() => appState.openJsonCellModal(effectiveTable || undefined, rIdx, cIdx, colName, val, !canEdit)}
                      class="px-1.5 py-0.5 rounded bg-purple-950/70 hover:bg-purple-900 border border-purple-800/70 text-purple-300 text-[10px] font-mono transition"
                      title="Open in JSON Inspector Modal"
                    >
                      {'{ } JSON'}
                    </button>
                  {/if}
                  {#if isDirty}
                    <button
                      type="button"
                      onclick={() => revertField(colName, cIdx)}
                      class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-amber-300 text-[10px] font-mono border border-slate-700"
                      title="Revert field to original value"
                    >
                      Revert
                    </button>
                  {/if}
                  <button
                    onclick={() => { copyText(formatted); appState.showToast(`Copied ${colName}`, 'info'); }}
                    class="p-1 text-slate-500 hover:text-slate-300"
                    title="Copy value"
                  >
                    <Icon name="copy" class="w-3 h-3" />
                  </button>
                </div>
              </div>

              {#if isBooleanType(colType) && canEdit}
                {@const curVal = editedFields[colName] !== undefined ? editedFields[colName] : val}
                <div class="flex items-center space-x-1.5 pt-0.5">
                  {#each [{ label: 'TRUE', v: true }, { label: 'FALSE', v: false }, { label: 'NULL', v: null }] as opt}
                    <button
                      type="button"
                      onclick={() => {
                        handleFieldChange(colName, opt.v);
                        saveField(colName, cIdx);
                      }}
                      class="px-2.5 py-1 rounded text-[11px] font-mono font-semibold border transition {curVal === opt.v || (opt.v === true && curVal === 1) || (opt.v === false && curVal === 0)
                        ? 'bg-emerald-500/20 border-emerald-500 text-emerald-300'
                        : 'bg-[#0f172a] border-[#334155] text-slate-400 hover:text-white'}"
                    >
                      {opt.label}
                    </button>
                  {/each}
                </div>
              {:else if formatted.length > 80 || formatted.includes('\n')}
                <textarea
                  rows="3"
                  readonly={!canEdit}
                  value={editedFields[colName] !== undefined ? editedFields[colName] : formatted}
                  oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLTextAreaElement).value)}
                  onblur={() => saveField(colName, cIdx)}
                  class="w-full bg-[#0f172a] border border-[#334155] rounded p-2 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500"
                ></textarea>
              {:else}
                <div class="flex items-center space-x-2">
                  <input
                    type="text"
                    readonly={!canEdit}
                    value={editedFields[colName] !== undefined ? editedFields[colName] : formatted}
                    oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLInputElement).value)}
                    onblur={() => saveField(colName, cIdx)}
                    class="flex-1 bg-[#0f172a] border border-[#334155] rounded px-2.5 py-1 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500"
                  />
                  {#if canEdit}
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
                  {/if}
                </div>
              {/if}
            </div>
          {/if}
        {/each}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#334155] bg-[#0b0f19] flex items-center justify-between text-xs">
        <span class="text-[11px] text-slate-500 font-mono">← / → or Space to browse records • Edits stage on blur</span>
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
