<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import {
    classifyColumnType,
    findTableMeta,
    resolveColumnNullable,
    resolveColumnType,
    resolveEnumValues,
    validateCellValue,
  } from '../../utils/dataTypes';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let currentTab = $derived(appState.activeTab);
  let result = $derived(currentTab?.activeResult);
  let totalRows = $derived(result?.rows ? result.rows.length : 0);
  let rIdx = $derived(appState.recordInspectorRowIndex);
  let currentRow = $derived(result?.rows && result.rows[rIdx] ? result.rows[rIdx] : null);
  let effectiveTable = $derived(currentTab?.tableName || currentTab?.detectedTableName || '');
  let effectiveDatabase = $derived(currentTab?.detectedTableDatabase || currentTab?.database || appState.currentDatabase);
  let canEdit = $derived(Boolean(effectiveTable && !appState.isReadOnly));

  let editedFields = $state<Record<string, any>>({});
  let searchQuery = $state('');
  let filterMode = $state<'all' | 'non-null' | 'modified'>('all');

  let activeCatalog = $derived(
    (effectiveDatabase && appState.catalogs[effectiveDatabase.toLowerCase()]) || appState.catalog
  );

  let tableMeta = $derived(findTableMeta(effectiveTable, activeCatalog));

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
    return resolveColumnType(c, tableMeta);
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

  function isFieldDirty(colName: string, cIdx: number, val: any): boolean {
    const isStaged = Boolean(
      effectiveTable &&
        currentRow &&
        appState.stagedEdits[`${effectiveTable}:${appState.getRowUid(currentRow)}:${cIdx}`]
    );
    const isDraftDirty = editedFields[colName] !== undefined && editedFields[colName] !== val;
    return isStaged || isDraftDirty;
  }

  let modifiedCount = $derived.by(() => {
    if (!result?.columns || !currentRow) return 0;
    let count = 0;
    result.columns.forEach((col: any, idx: number) => {
      if (isFieldDirty(getColName(col), idx, currentRow[idx])) count++;
    });
    return count;
  });

  let visibleColumns = $derived.by(() => {
    if (!result?.columns || !currentRow) return [];
    const q = searchQuery.toLowerCase().trim();
    return result.columns
      .map((col: any, cIdx: number) => {
        const colName = getColName(col);
        const colType = getColType(col);
        const val = currentRow[cIdx];
        const dirty = isFieldDirty(colName, cIdx, val);
        return { col, cIdx, colName, colType, val, dirty };
      })
      .filter(({ colName, colType, val, dirty }) => {
        if (filterMode === 'non-null' && (val === null || val === undefined)) return false;
        if (filterMode === 'modified' && !dirty) return false;
        if (!q) return true;
        const valStr = val === null || val === undefined ? 'null' : formatValue(val).toLowerCase();
        return (
          colName.toLowerCase().includes(q) ||
          colType.toLowerCase().includes(q) ||
          valStr.includes(q)
        );
      });
  });

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

    const colDef = result.columns[cIdx];
    const errMsg = validateCellValue(newVal, colDef, tableMeta);
    if (errMsg) {
      appState.showToast(errMsg, 'error');
      return;
    }

    const pkCols = result.columns.filter((c) => isColPK(c)).map((c) => getColName(c));
    let pkCol: string | undefined;
    let pkVal: any;
    let pkValues: Record<string, any> | undefined;

    if (pkCols.length === 1) {
      pkCol = pkCols[0];
      const pkColIdx = result.columns.findIndex((c) => getColName(c) === pkCol);
      pkVal = pkColIdx !== -1 ? currentRow[pkColIdx] : undefined;
    } else if (pkCols.length > 1) {
      pkValues = {};
      for (const col of pkCols) {
        const idx = result.columns.findIndex((c) => getColName(c) === col);
        pkValues[col] = idx !== -1 ? currentRow[idx] : undefined;
      }
    }

    let rowCriteria: Record<string, any> | undefined;
    if (pkCols.length === 0) {
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
      effectiveDatabase,
      rowCriteria,
      pkValues
    );
    currentRow[cIdx] = newVal;
    delete editedFields[colName];
    editedFields = { ...editedFields };
    appState.showToast(`Staged change for ${colName}`, 'info');
  }

  function commitPendingEdits() {
    if (!canEdit || !result?.columns) return;
    for (let i = 0; i < result.columns.length; i++) {
      const colName = getColName(result.columns[i]);
      if (editedFields[colName] !== undefined) {
        saveField(colName, i);
      }
    }
  }

  function navigate(delta: number) {
    const next = rIdx + delta;
    if (next >= 0 && next < totalRows) {
      commitPendingEdits();
      appState.recordInspectorRowIndex = next;
    }
  }

  function jumpTo(index: number) {
    if (index >= 0 && index < totalRows && index !== rIdx) {
      commitPendingEdits();
      appState.recordInspectorRowIndex = index;
    }
  }

  function closeModal() {
    commitPendingEdits();
    appState.isRecordInspectorOpen = false;
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
    const lines = result.columns.map(
      (c: any, idx: number) => `${getColName(c)}: ${currentRow[idx] === null ? 'NULL' : formatValue(currentRow[idx])}`
    );
    copyText(lines.join('\n'));
    appState.showToast('Copied record as key-value text', 'info');
  }

  function handleInspectorKeydown(e: KeyboardEvent) {
    if (!appState.isRecordInspectorOpen) return;
    if (e.key === 'Escape') {
      e.stopPropagation();
      closeModal();
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
  <div
    role="presentation"
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-[2px] select-none"
    onclick={(e) => {
      if (e.target === e.currentTarget) closeModal();
    }}
  >
    <div
      id="record-inspector-dialog"
      role="dialog"
      aria-modal="true"
      aria-label="Record inspector"
      class="w-full max-w-3xl bg-[#0d1320] border border-slate-700/80 rounded-xl shadow-[0_24px_64px_-12px_rgba(0,0,0,0.85)] flex flex-col max-h-[86vh] overflow-hidden"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 bg-[#090d16] flex items-center justify-between gap-4">
        <div class="flex items-center gap-3 min-w-0">
          <div class="w-8 h-8 rounded-lg bg-sky-500/10 border border-sky-500/25 flex items-center justify-center text-sky-400 shrink-0">
            <Icon name="table" class="w-4 h-4" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <h3 class="text-sm font-semibold text-slate-100 tracking-tight truncate">
                {effectiveTable || 'Query Result'}
              </h3>
              {#if appState.isReadOnly}
                <span class="inline-flex items-center gap-1 text-[10px] font-medium px-1.5 py-0.5 rounded bg-amber-950/70 border border-amber-800/70 text-amber-300">
                  <Icon name="lock" class="w-2.5 h-2.5" />
                  Read-Only
                </span>
              {/if}
              {#if modifiedCount > 0}
                <span class="inline-flex items-center gap-1 text-[10px] font-medium px-1.5 py-0.5 rounded bg-amber-500/15 border border-amber-500/30 text-amber-300 tabular-nums">
                  <span class="w-1.5 h-1.5 rounded-full bg-amber-400"></span>
                  {modifiedCount} staged
                </span>
              {/if}
            </div>
            <div class="text-[11px] text-slate-400 font-mono tabular-nums flex items-center gap-2 mt-0.5">
              <span>Row {(rIdx + 1).toLocaleString()} of {totalRows.toLocaleString()}</span>
              <span class="text-slate-700">•</span>
              <span>{result.columns.length} columns</span>
            </div>
          </div>
        </div>

        <!-- Actions & Pager -->
        <div class="flex items-center gap-2 shrink-0">
          <div class="hidden sm:inline-flex items-center rounded-lg border border-slate-700/80 bg-slate-900/80 divide-x divide-slate-700/80 overflow-hidden">
            <button
              type="button"
              onclick={copyRecordAsJSON}
              class="px-2.5 py-1.5 text-[11px] font-medium text-slate-300 hover:text-white hover:bg-slate-800 transition flex items-center gap-1.5"
              title="Copy whole record as JSON"
            >
              <Icon name="code" class="w-3 h-3 text-slate-400" />
              <span>JSON</span>
            </button>
            <button
              type="button"
              onclick={copyRecordAsText}
              class="px-2.5 py-1.5 text-[11px] font-medium text-slate-300 hover:text-white hover:bg-slate-800 transition flex items-center gap-1.5"
              title="Copy whole record as key-value text"
            >
              <Icon name="copy" class="w-3 h-3 text-slate-400" />
              <span>Text</span>
            </button>
          </div>

          <div id="record-inspector-pager" class="inline-flex items-center bg-slate-900/90 rounded-lg border border-slate-700/80 p-0.5">
            <button
              type="button"
              onclick={() => jumpTo(0)}
              disabled={rIdx <= 0}
              class="p-1.5 rounded-md hover:bg-slate-800 disabled:opacity-30 text-slate-300 disabled:hover:bg-transparent transition"
              title="First record"
              aria-label="First record"
            >
              <Icon name="chevrons-left" class="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              onclick={() => navigate(-1)}
              disabled={rIdx <= 0}
              class="p-1.5 rounded-md hover:bg-slate-800 disabled:opacity-30 text-slate-300 disabled:hover:bg-transparent transition"
              title="Previous record (←)"
              aria-label="Previous record"
            >
              <Icon name="chevron-left" class="w-3.5 h-3.5" />
            </button>
            <span class="px-2 text-[11px] font-mono text-slate-300 tabular-nums">
              {rIdx + 1}<span class="text-slate-600 mx-0.5">/</span>{totalRows}
            </span>
            <button
              type="button"
              onclick={() => navigate(1)}
              disabled={rIdx >= totalRows - 1}
              class="p-1.5 rounded-md hover:bg-slate-800 disabled:opacity-30 text-slate-300 disabled:hover:bg-transparent transition"
              title="Next record (→)"
              aria-label="Next record"
            >
              <Icon name="chevron-right" class="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              onclick={() => jumpTo(totalRows - 1)}
              disabled={rIdx >= totalRows - 1}
              class="p-1.5 rounded-md hover:bg-slate-800 disabled:opacity-30 text-slate-300 disabled:hover:bg-transparent transition"
              title="Last record"
              aria-label="Last record"
            >
              <Icon name="chevrons-right" class="w-3.5 h-3.5" />
            </button>
          </div>

          <button
            type="button"
            onclick={closeModal}
            class="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition"
            title="Close (Esc)"
            aria-label="Close record inspector"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Search & Filter Bar -->
      <div class="px-5 py-2.5 border-b border-slate-800/80 bg-[#0b101b] flex items-center gap-3">
        <div class="relative flex-1">
          <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
          <input
            id="record-inspector-search"
            type="text"
            bind:value={searchQuery}
            placeholder="Filter by column name, type, or value..."
            class="w-full bg-[#090d16] border border-slate-700/80 rounded-lg pl-8 pr-8 py-1.5 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500/30 transition"
          />
          {#if searchQuery}
            <button
              type="button"
              onclick={() => (searchQuery = '')}
              class="absolute right-2 top-1/2 -translate-y-1/2 p-0.5 text-slate-500 hover:text-slate-200 rounded"
              title="Clear filter"
              aria-label="Clear filter"
            >
              <Icon name="x" class="w-3.5 h-3.5" />
            </button>
          {/if}
        </div>

        <div class="inline-flex items-center rounded-lg bg-[#090d16] border border-slate-700/80 p-0.5 text-[11px] shrink-0">
          <button
            type="button"
            onclick={() => (filterMode = 'all')}
            class="px-2.5 py-1 rounded-md font-medium transition {filterMode === 'all'
              ? 'bg-slate-800 text-slate-100 shadow-sm'
              : 'text-slate-400 hover:text-slate-200'}"
          >
            All
          </button>
          <button
            type="button"
            onclick={() => (filterMode = 'non-null')}
            class="px-2.5 py-1 rounded-md font-medium transition {filterMode === 'non-null'
              ? 'bg-slate-800 text-slate-100 shadow-sm'
              : 'text-slate-400 hover:text-slate-200'}"
          >
            Non-NULL
          </button>
          {#if modifiedCount > 0 || filterMode === 'modified'}
            <button
              type="button"
              onclick={() => (filterMode = 'modified')}
              class="px-2.5 py-1 rounded-md font-medium transition {filterMode === 'modified'
                ? 'bg-amber-500/20 text-amber-300'
                : 'text-amber-400/80 hover:text-amber-300'}"
            >
              Modified ({modifiedCount})
            </button>
          {/if}
        </div>

        <span class="hidden md:inline-block text-[11px] font-mono text-slate-500 tabular-nums shrink-0">
          {visibleColumns.length}/{result.columns.length}
        </span>
      </div>

      <!-- Fields Property Sheet -->
      <div id="record-inspector-fields" class="flex-1 overflow-y-auto divide-y divide-slate-800/60 bg-[#0d1320]">
        {#if visibleColumns.length === 0}
          <div class="py-12 px-6 text-center space-y-2">
            <p class="text-xs text-slate-400">
              No columns match {searchQuery ? `"${searchQuery}"` : 'the active filter'}.
            </p>
            <button
              type="button"
              onclick={() => {
                searchQuery = '';
                filterMode = 'all';
              }}
              class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium border border-slate-700 transition"
            >
              Reset filters
            </button>
          </div>
        {:else}
          {#each visibleColumns as { col, cIdx, colName, colType, val, dirty } (cIdx)}
            {@const isPK = isColPK(col)}
            {@const nullable = resolveColumnNullable(col, tableMeta)}
            {@const fk = fkMap.get(colName.toLowerCase())}
            {@const activeVal = editedFields[colName] !== undefined ? editedFields[colName] : val}
            {@const formatted = formatValue(activeVal)}
            {@const isNullVal = activeVal === null || activeVal === undefined}
            {@const enumVals = canEdit ? resolveEnumValues(col, tableMeta) : []}
            {@const kind = classifyColumnType(colType, enumVals)}
            {@const isJson = isJsonType(colType, activeVal)}
            {@const isLong = formatted.length > 72 || formatted.includes('\n')}

            <div
              class="grid grid-cols-1 sm:grid-cols-[210px_1fr] gap-2 sm:gap-4 px-5 py-3 items-start transition-colors {dirty
                ? 'bg-amber-500/[0.04] border-l-2 border-l-amber-400'
                : 'hover:bg-slate-900/40 border-l-2 border-l-transparent'}"
            >
              <!-- Column Label & Metadata -->
              <div class="min-w-0 pt-1 space-y-1">
                <div class="flex items-center gap-1.5 min-w-0">
                  {#if isPK}
                    <span title="Primary Key" class="shrink-0 flex items-center">
                      <Icon name="key" class="w-3.5 h-3.5 text-amber-400" />
                    </span>
                  {/if}
                  <span
                    class="font-mono text-xs font-semibold truncate {dirty ? 'text-amber-200' : 'text-slate-200'}"
                    title={colName}
                  >
                    {colName}
                  </span>
                  {#if dirty}
                    <span class="w-1.5 h-1.5 rounded-full bg-amber-400 shrink-0" title="Modified (staged)"></span>
                  {/if}
                </div>

                <div class="flex items-center gap-1.5 flex-wrap">
                  {#if colType}
                    <span class="text-[10px] font-mono text-slate-500 uppercase tracking-wide truncate max-w-[160px]" title={colType}>
                      {colType}
                    </span>
                  {/if}
                  {#if fk}
                    <span
                      class="text-[9px] font-mono px-1 py-0.2 rounded bg-sky-950/60 border border-sky-800/60 text-sky-400"
                      title="Foreign Key → {fk.ref_table || fk.referenced_table}.{fk.ref_col || fk.referenced_column}"
                    >
                      FK
                    </span>
                  {/if}
                </div>
              </div>

              <!-- Value Editor & Actions -->
              <div class="min-w-0 space-y-1.5">
                <div class="flex items-start gap-1.5">
                  <div class="flex-1 min-w-0">
                    {#if isBooleanType(colType) && canEdit}
                      {@const boolOpts = nullable ? [{ label: 'TRUE', v: true }, { label: 'FALSE', v: false }, { label: 'NULL', v: null }] : [{ label: 'TRUE', v: true }, { label: 'FALSE', v: false }]}
                      <div class="inline-flex items-center rounded-lg bg-[#090d16] border border-slate-700/80 p-0.5">
                        {#each boolOpts as opt}
                          {@const isSelected =
                            activeVal === opt.v ||
                            (opt.v === true && activeVal === 1) ||
                            (opt.v === false && activeVal === 0)}
                          <button
                            type="button"
                            onclick={() => {
                              handleFieldChange(colName, opt.v);
                              saveField(colName, cIdx);
                            }}
                            class="px-3 py-1 rounded-md text-[11px] font-mono font-medium transition {isSelected
                              ? opt.v === null
                                ? 'bg-rose-500/20 text-rose-300 shadow-sm'
                                : 'bg-emerald-500/20 text-emerald-300 shadow-sm'
                              : 'text-slate-400 hover:text-slate-200'}"
                          >
                            {opt.label}
                          </button>
                        {/each}
                      </div>
                    {:else if kind.isEnum && canEdit}
                      <select
                        value={isNullVal ? (nullable ? '__NULL__' : '') : String(activeVal)}
                        onchange={(e) => {
                          const v = (e.currentTarget as HTMLSelectElement).value;
                          if (v === '__NULL__' && !nullable) return;
                          handleFieldChange(colName, v === '__NULL__' ? null : v);
                          saveField(colName, cIdx);
                        }}
                        class="w-full h-8 bg-[#090d16] border border-slate-700/80 rounded-lg px-2.5 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500/30 transition"
                      >
                        {#if nullable}
                          <option value="__NULL__">NULL</option>
                        {:else if isNullVal || !enumVals.includes(String(activeVal))}
                          <option value="" disabled>Select value...</option>
                        {/if}
                        {#if !isNullVal && !enumVals.includes(String(activeVal))}
                          <option value={String(activeVal)}>{String(activeVal)}</option>
                        {/if}
                        {#each enumVals as ev}
                          <option value={ev}>{ev}</option>
                        {/each}
                      </select>
                    {:else if kind.isDate && canEdit}
                      <input
                        type="date"
                        value={isNullVal ? '' : formatted}
                        oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLInputElement).value)}
                        onblur={() => saveField(colName, cIdx)}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault();
                            saveField(colName, cIdx);
                          }
                        }}
                        class="w-full h-8 bg-[#090d16] border border-slate-700/80 rounded-lg px-2.5 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500/30 transition [color-scheme:dark]"
                      />
                    {:else if kind.isTime && canEdit}
                      <input
                        type="time"
                        step="1"
                        value={isNullVal ? '' : formatted}
                        oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLInputElement).value)}
                        onblur={() => saveField(colName, cIdx)}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault();
                            saveField(colName, cIdx);
                          }
                        }}
                        class="w-full h-8 bg-[#090d16] border border-slate-700/80 rounded-lg px-2.5 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500/30 transition [color-scheme:dark]"
                      />
                    {:else if isLong || isJson}
                      <textarea
                        rows={Math.min(8, Math.max(3, formatted.split('\n').length))}
                        readonly={!canEdit}
                        value={isNullVal ? '' : formatted}
                        placeholder={isNullVal ? 'NULL' : '(empty)'}
                        oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLTextAreaElement).value)}
                        onblur={() => saveField(colName, cIdx)}
                        class="w-full bg-[#090d16] border border-slate-700/80 rounded-lg p-2.5 text-xs font-mono leading-relaxed text-slate-100 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500/30 transition select-text {isNullVal
                          ? 'placeholder:text-rose-400/70 placeholder:italic'
                          : 'placeholder:text-slate-600 placeholder:italic'} {!canEdit ? 'opacity-85 cursor-default' : ''}"
                      ></textarea>
                    {:else}
                      <input
                        type="text"
                        readonly={!canEdit}
                        value={isNullVal ? '' : formatted}
                        placeholder={isNullVal ? 'NULL' : '(empty string)'}
                        oninput={(e) => handleFieldChange(colName, (e.currentTarget as HTMLInputElement).value)}
                        onblur={() => saveField(colName, cIdx)}
                        onkeydown={(e) => {
                          if (e.key === 'Enter' && canEdit) {
                            e.preventDefault();
                            saveField(colName, cIdx);
                          }
                        }}
                        class="w-full h-8 bg-[#090d16] border border-slate-700/80 rounded-lg px-2.5 text-xs font-mono text-slate-100 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500/30 transition select-text {isNullVal
                          ? 'placeholder:text-rose-400/70 placeholder:italic'
                          : 'placeholder:text-slate-600 placeholder:italic'} {!canEdit ? 'opacity-85 cursor-default' : ''}"
                      />
                    {/if}
                  </div>

                  <!-- Contextual Field Actions -->
                  <div class="flex items-center gap-1 shrink-0 pt-0.5">
                    {#if fk && !isNullVal}
                      <button
                        type="button"
                        onclick={(e) =>
                          appState.openFkPeek(
                            e,
                            effectiveTable,
                            colName,
                            fk.ref_table || fk.referenced_table,
                            fk.ref_col || fk.referenced_column,
                            activeVal,
                            effectiveDatabase
                          )}
                        class="h-7 px-2 rounded-md bg-sky-950/60 hover:bg-sky-900/80 border border-sky-800/70 text-sky-300 text-[11px] font-mono inline-flex items-center gap-1 transition"
                        title="Peek referenced row in {fk.ref_table || fk.referenced_table}"
                      >
                        <Icon name="link" class="w-3 h-3" />
                        <span>Peek</span>
                      </button>
                    {/if}

                    {#if isJson}
                      <button
                        type="button"
                        onclick={() =>
                          appState.openJsonCellModal(
                            effectiveTable || undefined,
                            rIdx,
                            cIdx,
                            colName,
                            activeVal,
                            !canEdit
                          )}
                        class="h-7 px-2 rounded-md bg-purple-950/60 hover:bg-purple-900/80 border border-purple-800/70 text-purple-300 text-[11px] font-mono transition"
                        title="Open in JSON Inspector Modal"
                      >
                        {'{ }'}
                      </button>
                    {/if}

                    {#if canEdit && nullable && !isBooleanType(colType) && !isNullVal}
                      <button
                        type="button"
                        onclick={() => {
                          handleFieldChange(colName, null);
                          saveField(colName, cIdx);
                        }}
                        class="h-7 px-2 rounded-md bg-slate-800/80 hover:bg-rose-950/70 text-slate-400 hover:text-rose-300 text-[10px] font-mono border border-slate-700/80 hover:border-rose-800/70 transition"
                        title="Set value to NULL"
                      >
                        NULL
                      </button>
                    {/if}

                    {#if dirty}
                      <button
                        type="button"
                        onclick={() => revertField(colName, cIdx)}
                        class="h-7 w-7 rounded-md bg-amber-500/10 hover:bg-amber-500/20 text-amber-300 border border-amber-500/30 inline-flex items-center justify-center transition"
                        title="Revert field to original value"
                        aria-label="Revert {colName}"
                      >
                        <Icon name="rotate-ccw" class="w-3.5 h-3.5" />
                      </button>
                    {/if}

                    <button
                      type="button"
                      onclick={() => {
                        copyText(isNullVal ? 'NULL' : formatted);
                        appState.showToast(`Copied ${colName}`, 'info');
                      }}
                      class="h-7 w-7 rounded-md text-slate-500 hover:text-slate-200 hover:bg-slate-800/80 inline-flex items-center justify-center transition"
                      title="Copy value"
                      aria-label="Copy {colName}"
                    >
                      <Icon name="copy" class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          {/each}
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-slate-800 bg-[#090d16] flex items-center justify-between gap-4 text-xs">
        <div class="text-[11px] text-slate-500 flex items-center gap-2 min-w-0 truncate">
          <span class="hidden sm:inline font-mono">← / → browse</span>
          <span class="hidden sm:inline text-slate-700">•</span>
          <span class="hidden sm:inline font-mono">Esc close</span>
          {#if canEdit}
            <span class="hidden sm:inline text-slate-700">•</span>
            <span>Edits stage on blur or Enter</span>
          {/if}
        </div>
        <button
          type="button"
          onclick={closeModal}
          class="px-4 py-1.5 bg-sky-500 hover:bg-sky-400 text-slate-950 font-semibold rounded-lg shadow-sm transition shrink-0"
        >
          Done
        </button>
      </div>
    </div>
  </div>
{/if}
