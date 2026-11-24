<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { copyText } from '../../utils/clipboard';
  import { api } from '../../api/client';
  import {
    classifyColumnType,
    findTableMeta,
    resolveColumnNullable,
    resolveColumnType,
    resolveEnumValues,
    validateCellValue as validateCellAgainstSchema,
  } from '../../utils/dataTypes';
  import { quoteSqlIdentifier, formatSqlLiteral } from '../../utils/sqlStatements';
  import { appState } from '../../stores/appState.svelte';
  import type { ColumnDefinition, FilterPill } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import QueryProfilerPanel from '../editor/QueryProfilerPanel.svelte';
  import { viewportFit } from '../../actions/viewportFit';

  const ROW_HEIGHT = 28;
  const OVERSCAN = 15;

  let containerRef: HTMLDivElement | null = $state(null);
  let scrollTop = $state(0);
  let containerHeight = $state(500);

  let currentTab = $derived(appState.activeTab);
  let result = $derived(appState.activeTab?.activeResult);
  let isExecuting = $derived(Boolean(appState.activeTab?.isExecuting));
  let effectiveTable = $derived(currentTab?.tableName || currentTab?.detectedTableName || '');
  let effectiveDatabase = $derived(currentTab?.detectedTableDatabase || currentTab?.database || appState.currentDatabase);
  let canEdit = $derived(Boolean(effectiveTable && !appState.isReadOnly));

  // Sorting
  let sortCol = $state<string | null>(null);
  let sortDir = $state<'ASC' | 'DESC'>('ASC');
  let effectiveSortCol = $derived(
    currentTab?.isTableViewer ? (currentTab.sortCol ?? null) : sortCol
  );
  let effectiveSortDir = $derived(
    currentTab?.isTableViewer ? (currentTab.sortDir ?? 'ASC') : sortDir
  );

  // Selected row / cell & Context Menu
  let selectedRowIndex = $state<number | null>(null);
  let selectedCell = $state<{ rIdx: number; cIdx: number } | null>(null);
  let rangeAnchor = $state<{ dispRow: number; visCol: number } | null>(null);
  let rangeEnd = $state<{ dispRow: number; visCol: number } | null>(null);
  let localSearchQuery = $state('');
  let localSearchInputEl = $state<HTMLInputElement | null>(null);
  let editingCell = $state<{ rIdx: number; cIdx: number; tempValue: string } | null>(null);
  let flashedCells = $state<Record<string, boolean>>({});
  let detailPrettyJson = $state(false);

  // FK datalist suggestions cache: `${refTable}.${refCol}` -> string[]
  let fkDatalistCache = $state<Record<string, string[]>>({});
  let fkDatalistLoading = new Set<string>();

  // Multi-row selection
  let selectedRowIndices = $state<number[]>([]);

  function flashCell(rIdx: number, cIdx: number) {
    const key = `${rIdx}:${cIdx}`;
    flashedCells = { ...flashedCells, [key]: true };
    setTimeout(() => {
      const next = { ...flashedCells };
      delete next[key];
      flashedCells = next;
    }, 900);
  }

  async function ensureFkSuggestions(fk: { referenced_table: string; referenced_column: string }) {
    if (!appState.activeSessionId || !fk?.referenced_table || !fk?.referenced_column) return;
    const cacheKey = `${fk.referenced_table}.${fk.referenced_column}`;
    if (fkDatalistCache[cacheKey] || fkDatalistLoading.has(cacheKey)) return;
    fkDatalistLoading.add(cacheKey);
    try {
      const res = await api.getTableRows(
        appState.activeSessionId,
        fk.referenced_table,
        100,
        0,
        effectiveDatabase || undefined
      );
      const colNames = (res.columns || []).map((c: any) => (typeof c === 'string' ? c : c.name || ''));
      const cIdx = colNames.findIndex((n) => n.toLowerCase() === fk.referenced_column.toLowerCase());
      if (cIdx !== -1 && Array.isArray(res.rows)) {
        const vals = Array.from(
          new Set(
            res.rows
              .map((row) => row[cIdx])
              .filter((v) => v !== null && v !== undefined)
              .map((v) => String(v))
          )
        );
        fkDatalistCache = { ...fkDatalistCache, [cacheKey]: vals };
      }
    } catch {
      // Ignore silent datalist fetch failures
    } finally {
      fkDatalistLoading.delete(cacheKey);
    }
  }

  // Focus filter handler via Ctrl+F inside grid
  function handleFocusTableFilterEvent() {
    if (!currentTab) return;
    if (!currentTab.showFilterBar) {
      appState.toggleFilterBar();
    }
    if (
      (!currentTab.isTableViewer || currentTab.filterMode !== 'raw') &&
      (!currentTab.filterPills || currentTab.filterPills.length === 0)
    ) {
      appState.addFilterPill();
    }
    tick().then(() => {
      const el =
        document.getElementById('table-filter-raw-input') ||
        document.querySelector<HTMLInputElement>('#grid-filter-pill-val-0');
      el?.focus();
    });
  }

  onMount(() => {
    window.addEventListener('sqlfox:focus-table-filter', handleFocusTableFilterEvent);
  });
  onDestroy(() => {
    window.removeEventListener('sqlfox:focus-table-filter', handleFocusTableFilterEvent);
  });

  // Reset selection, scroll, and editing states when active tab or result set changes
  $effect(() => {
    const _tabId = currentTab?.id;
    const _res = result;
    scrollTop = 0;
    if (containerRef) containerRef.scrollTop = 0;
    selectedRowIndices = [];
    selectedRowIndex = null;
    selectedCell = null;
    editingCell = null;
    contextMenu = null;
    detailPrettyJson = false;
  });

  // Keep containerHeight synced with viewport/splitter resizes before any scroll event.
  $effect(() => {
    const el = containerRef;
    if (!el) return;
    containerHeight = el.clientHeight || 500;
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => {
      containerHeight = el.clientHeight || 500;
    });
    ro.observe(el);
    return () => ro.disconnect();
  });

  // Context Menu State
  let contextMenu = $state<{
    open: boolean;
    x: number;
    y: number;
    rIdx: number;
    cIdx: number;
    colName: string;
    cellValue: any;
  } | null>(null);

  // Column Picker Dropdown State
  let showColPicker = $state(false);
  let colPickerSearch = $state('');

  // Export Dropdown State
  let showExportMenu = $state(false);

  // Cell Detail Drawer
  let showDetailDrawer = $state(false);

  // Filter Bar Toggle State
  let isFilterOpen = $derived(Boolean(currentTab?.showFilterBar));
  let hasFilterEntries = $derived(
    Boolean(
      currentTab?.appliedFilter ||
      (currentTab?.filterPills && currentTab.filterPills.length > 0) ||
      currentTab?.filterRaw
    )
  );
  let hasActiveFilter = $derived(
    Boolean(
      currentTab?.isTableViewer
        ? currentTab?.appliedFilter ||
            (currentTab?.filterPills && currentTab.filterPills.length > 0) ||
            currentTab?.filterRaw
        : currentTab?.filterPills?.some(
            (p) => p.op === 'IS NULL' || p.op === 'IS NOT NULL' || String(p.val ?? '').trim() !== ''
          )
    )
  );

  function toggleFilterBar() {
    const opening = !isFilterOpen;
    appState.toggleFilterBar();
    if (
      opening &&
      currentTab &&
      (!currentTab.isTableViewer || currentTab.filterMode !== 'raw') &&
      (!currentTab.filterPills || currentTab.filterPills.length === 0)
    ) {
      appState.addFilterPill();
    }
  }

  function getColName(col: any): string {
    if (!col) return '';
    return typeof col === 'string' ? col : col.name || '';
  }

  function getColType(col: any): string {
    if (!col || typeof col !== 'object') return '';
    return col.type || '';
  }

  let activeCatalog = $derived(
    (effectiveDatabase && appState.catalogs[effectiveDatabase.toLowerCase()]) || appState.catalog
  );

  let tableMeta = $derived.by(() => findTableMeta(effectiveTable, activeCatalog));

  let tableEffectiveTotal = $derived.by(() => {
    if (typeof result?.total_rows === 'number') {
      return result.total_rows;
    }
    return tableMeta?.row_count ?? tableMeta?.rows_count;
  });

  let tableTotalIsExact = $derived.by(() => {
    return Boolean(result?.is_exact || tableMeta?.row_count_exact);
  });

  function isColPK(col: any): boolean {
    if (typeof col === 'object' && col.is_primary_key !== undefined) return Boolean(col.is_primary_key);
    const colName = getColName(col);
    const colMeta = tableMeta?.columns?.find((c) => c.name.toLowerCase() === colName.toLowerCase());
    return Boolean(colMeta?.is_primary_key);
  }

  function resolveColType(col: any): string {
    return resolveColumnType(col, tableMeta);
  }

  function isColNullable(col: any): boolean {
    return resolveColumnNullable(col, tableMeta);
  }

  let allColumns = $derived<string[]>(
    result?.columns ? result.columns.map((c) => getColName(c)) : []
  );

  let colIndexMap = $derived.by(() => {
    const map = new Map<string, number>();
    allColumns.forEach((name, idx) => map.set(name, idx));
    return map;
  });

  let hiddenCols = $derived<string[]>(currentTab?.hiddenColumns || []);

  let visibleColumnEntries = $derived.by(() => {
    if (!result?.columns) return [] as Array<{ col: string | ColumnDefinition; origIdx: number }>;
    const entries: Array<{ col: string | ColumnDefinition; origIdx: number }> = [];
    result.columns.forEach((col, origIdx) => {
      if (!hiddenCols.includes(getColName(col))) {
        entries.push({ col, origIdx });
      }
    });
    return entries;
  });

  let visibleColumns = $derived(visibleColumnEntries.map((e) => e.col));

  let fkMap = $derived.by(() => {
    const map = new Map<string, any>();
    for (const fk of tableMeta?.foreign_keys || []) {
      const col = (fk as any).column || (fk as any).column_name;
      if (col) map.set(col, fk);
    }
    return map;
  });

  function matchesFilterPill(cellVal: any, pill: FilterPill): boolean {
    if (pill.op === 'IS NULL') return cellVal === null || cellVal === undefined;
    if (pill.op === 'IS NOT NULL') return cellVal !== null && cellVal !== undefined;
    if (String(pill.val ?? '').trim() === '') return true;
    if (cellVal === null || cellVal === undefined) return false;
    const str = typeof cellVal === 'object' ? JSON.stringify(cellVal) : String(cellVal);
    const valStr = String(pill.val ?? '');
    switch (pill.op) {
      case '=':
        return str === valStr;
      case '!=':
        return str !== valStr;
      case 'LIKE': {
        const needle = valStr.replace(/%/g, '').toLowerCase();
        return str.toLowerCase().includes(needle);
      }
      case 'NOT LIKE': {
        const needle = valStr.replace(/%/g, '').toLowerCase();
        return !str.toLowerCase().includes(needle);
      }
      case '>':
      case '<':
      case '>=':
      case '<=': {
        const a = Number(cellVal);
        const b = Number(valStr);
        const cmp = Number.isFinite(a) && Number.isFinite(b) ? a - b : str.localeCompare(valStr);
        if (pill.op === '>') return cmp > 0;
        if (pill.op === '<') return cmp < 0;
        if (pill.op === '>=') return cmp >= 0;
        return cmp <= 0;
      }
      case 'IN': {
        const parts = valStr.split(',').map((s) => s.trim().replace(/^['"]|['"]$/g, ''));
        return parts.includes(str);
      }
      case 'NOT IN': {
        const parts = valStr.split(',').map((s) => s.trim().replace(/^['"]|['"]$/g, ''));
        return !parts.includes(str);
      }
      case 'BETWEEN': {
        const parts = valStr.split(/\s+AND\s+|,/i).map((s) => s.trim());
        if (parts.length < 2) return true;
        return str >= parts[0] && str <= parts[1];
      }
      default:
        return true;
    }
  }

  let processedRows = $derived.by(() => {
    if (!result?.rows) return [];
    let list = result.rows.map((r, idx) => ({ origIdx: idx, data: r }));

    // Client-side pill filtering for non-TableViewer query result sets
    if (!currentTab?.isTableViewer && currentTab?.filterPills && currentTab.filterPills.length > 0) {
      const pills = currentTab.filterPills;
      list = list.filter((item) =>
        pills.every((pill) => {
          const cIdx = colIndexMap.get(pill.col);
          if (cIdx === undefined) return true;
          return matchesFilterPill(item.data[cIdx], pill);
        })
      );
    }

    // Local result search filter (Ctrl+F in grid)
    const sq = localSearchQuery.trim().toLowerCase();
    if (sq) {
      list = list.filter((item) =>
        item.data.some((cellVal: any) => cellVal !== null && cellVal !== undefined && String(cellVal).toLowerCase().includes(sq))
      );
    }

    // Apply sorting if active (only for ad-hoc queries; TableViewer is sorted server-side)
    if (!currentTab?.isTableViewer && sortCol !== null && result.columns) {
      const cIdx = colIndexMap.get(sortCol) ?? -1;
      if (cIdx !== -1) {
        list = [...list].sort((a, b) => {
          const valA = a.data[cIdx];
          const valB = b.data[cIdx];
          if (valA === valB) return 0;
          if (valA === null || valA === undefined) return 1;
          if (valB === null || valB === undefined) return -1;
          if (typeof valA === 'number' && typeof valB === 'number') {
            return sortDir === 'ASC' ? valA - valB : valB - valA;
          }
          return sortDir === 'ASC'
            ? String(valA).localeCompare(String(valB))
            : String(valB).localeCompare(String(valA));
        });
      }
    }
    return list;
  });

  let totalRows = $derived(processedRows.length);
  let totalCols = $derived(visibleColumns.length + 1);

  let startIndex = $derived(Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN));
  let endIndex = $derived(Math.min(totalRows, Math.ceil((scrollTop + containerHeight) / ROW_HEIGHT) + OVERSCAN));

  let topSpacerHeight = $derived(startIndex * ROW_HEIGHT);
  let bottomSpacerHeight = $derived(Math.max(0, (totalRows - endIndex) * ROW_HEIGHT));

  let visibleRows = $derived.by(() => {
    return processedRows.slice(startIndex, endIndex).map((item, idx) => ({
      displayIdx: startIndex + idx,
      origIdx: item.origIdx,
      data: item.data,
    }));
  });

  function onScroll(e: Event) {
    const target = e.currentTarget as HTMLDivElement;
    scrollTop = target.scrollTop;
    containerHeight = target.clientHeight || 500;
  }

  function handleHeaderSort(colName: string) {
    if (currentTab?.isTableViewer) {
      if (currentTab.sortCol === colName) {
        if (currentTab.sortDir === 'ASC') {
          currentTab.sortDir = 'DESC';
        } else {
          currentTab.sortCol = undefined;
          currentTab.sortDir = undefined;
        }
      } else {
        currentTab.sortCol = colName;
        currentTab.sortDir = 'ASC';
      }
      currentTab.currentPage = 1;
      void appState.loadTableData(currentTab);
      return;
    }

    if (sortCol === colName) {
      if (sortDir === 'ASC') {
        sortDir = 'DESC';
      } else {
        sortCol = null;
        sortDir = 'ASC';
      }
    } else {
      sortCol = colName;
      sortDir = 'ASC';
    }
  }

  function formatCellValue(val: any): { display: string; type: string } {
    if (val === null || val === undefined) {
      return { display: 'NULL', type: 'null' };
    }
    if (typeof val === 'boolean') {
      return { display: val ? 'TRUE' : 'FALSE', type: 'boolean' };
    }
    if (typeof val === 'number') {
      return { display: val.toLocaleString(), type: 'number' };
    }
    if (typeof val === 'object') {
      try {
        return { display: JSON.stringify(val), type: 'json' };
      } catch {
        return { display: String(val), type: 'text' };
      }
    }
    return { display: String(val), type: 'text' };
  }

  function getCellClass(type: string) {
    switch (type) {
      case 'null':
        return 'text-rose-400/80 italic font-medium';
      case 'number':
        return 'text-amber-300 font-mono text-right';
      case 'boolean':
        return 'text-emerald-400 font-semibold';
      case 'json':
        return 'text-purple-300 font-mono';
      default:
        return 'text-slate-200 font-mono';
    }
  }

  let isAllSelected = $derived(
    processedRows.length > 0 && selectedRowIndices.length === processedRows.length
  );

  function toggleSelectAll() {
    if (isAllSelected) {
      selectedRowIndices = [];
    } else {
      selectedRowIndices = processedRows.map((r) => r.origIdx);
    }
  }

  function toggleRowSelection(origIdx: number) {
    const idx = selectedRowIndices.indexOf(origIdx);
    if (idx === -1) {
      selectedRowIndices.push(origIdx);
    } else {
      selectedRowIndices.splice(idx, 1);
    }
  }

  async function handleBatchDelete() {
    if (selectedRowIndices.length === 0 || !effectiveTable || !result?.rows) return;
    if (appState.isReadOnly) {
      appState.showToast('Read-only mode is active — delete blocked', 'warning');
      return;
    }
    const validIndices = selectedRowIndices.filter((idx) => result!.rows && result!.rows[idx] !== undefined);
    if (validIndices.length === 0) return;

    const pkCols = result.columns.filter((c) => isColPK(c)).map((c) => getColName(c));

    const ok = await appState.showNativeConfirm(
      `Are you sure you want to delete ${validIndices.length} selected row(s)?`,
      'Batch Deletion'
    );
    if (!ok) return;

    if (pkCols.length === 1) {
      const pkCol = pkCols[0];
      const pkIdx = colIndexMap.get(pkCol) ?? -1;
      const pkVals = validIndices.map((idx) => result!.rows[idx][pkIdx]);
      await appState.deleteRows(pkCol, pkVals);
    } else if (pkCols.length > 1) {
      const pkRows = validIndices.map((idx) => {
        const rowMap: Record<string, any> = {};
        for (const col of pkCols) {
          rowMap[col] = result!.rows[idx][colIndexMap.get(col) ?? -1];
        }
        return rowMap;
      });
      await appState.deleteRowsByCompositePK(pkRows);
    } else {
      const criterias = validIndices.map((idx) => {
        const crit: Record<string, any> = {};
        allColumns.forEach((col, cIdx) => {
          crit[col] = result!.rows[idx][cIdx];
        });
        return crit;
      });
      await appState.deleteRowsByCriteria(criterias);
    }
    selectedRowIndices = [];
  }

  function addDraftRow() {
    if (!currentTab) return;
    if (!canEdit) {
      appState.showToast(appState.isReadOnly ? 'Read-only mode is active' : 'Draft rows require a single target table', 'warning');
      return;
    }
    if (!currentTab.stagedDraftRows) currentTab.stagedDraftRows = [];
    const blank: Record<string, any> = {};
    allColumns.forEach((c) => {
      blank[c] = '';
    });
    currentTab.stagedDraftRows.unshift(blank);
    appState.showToast('Draft row added. Enter values or Save All staged edits.', 'info');
  }

  function removeDraftRow(dIdx: number) {
    if (!currentTab?.stagedDraftRows) return;
    currentTab.stagedDraftRows.splice(dIdx, 1);
  }

  async function saveDraftRow(dIdx: number) {
    if (!canEdit || !currentTab?.stagedDraftRows || !currentTab.stagedDraftRows[dIdx] || !effectiveTable) return;
    const draft = currentTab.stagedDraftRows[dIdx];
    const cleaned: Record<string, any> = {};
    for (const [k, v] of Object.entries(draft)) {
      if (v !== '' && v !== undefined) {
        const colObj = result?.columns?.find((c) => getColName(c).toLowerCase() === k.toLowerCase()) ?? k;
        const valToCheck = v === '__NULL__' ? null : v;
        const err = validateCellAgainstSchema(valToCheck, colObj, tableMeta);
        if (err) {
          appState.showToast(err, 'error');
          return;
        }
        cleaned[k] = valToCheck;
      }
    }
    try {
      await appState.insertRow(cleaned);
      if (currentTab?.stagedDraftRows) {
        currentTab.stagedDraftRows.splice(dIdx, 1);
      }
    } catch (err: any) {
      appState.showToast(`Failed to save draft row: ${err.message}`, 'error');
    }
  }

  function autoFocus(node: HTMLElement) {
    node.focus();
    if (node instanceof HTMLInputElement) node.select();
  }

  /**
   * Clones a visible row into an editable draft.
   */
  function duplicateRowAsDraft(rIdx: number, withoutKey = true) {
    if (!canEdit || !currentTab || !result?.rows || !result.rows[rIdx]) {
      if (appState.isReadOnly) appState.showToast('Read-only mode is active', 'warning');
      return;
    }
    if (!currentTab.stagedDraftRows) currentTab.stagedDraftRows = [];
    const row = result.rows[rIdx];
    const draft: Record<string, any> = {};
    allColumns.forEach((colName, cIdx) => {
      const isPK = Boolean(result!.columns[cIdx] && isColPK(result!.columns[cIdx]));
      draft[colName] = withoutKey && isPK ? '' : row[cIdx];
    });
    currentTab.stagedDraftRows.unshift(draft);
    appState.showToast(
      `Row #${rIdx + 1} duplicated as editable draft${withoutKey ? ' (key cleared)' : ' (key retained)'}`,
      'success'
    );
  }

  function duplicateMultipleRows(indices: number[]) {
    if (!canEdit) return;
    indices.forEach((idx) => duplicateRowAsDraft(idx, true));
  }

  function stepActiveDatePart(delta: number) {
    if (!editingCell) return;
    const val = editingCell.tempValue.trim();
    const match = val.match(/^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2})(?::(\d{2}))?)?$/);
    if (!match) return;
    let y = Number(match[1]);
    let m = Number(match[2]);
    let d = Number(match[3]);
    let hh = match[4] !== undefined ? Number(match[4]) : null;
    let mm = match[5] !== undefined ? Number(match[5]) : null;
    let ss = match[6] !== undefined ? Number(match[6]) : 0;

    const inputEl = document.getElementById('grid-inline-editor') as HTMLInputElement | null;
    const pos = inputEl?.selectionStart ?? val.length;

    const daysInMonth = (yr: number, mo: number) => new Date(yr, mo, 0).getDate();

    if (pos <= 4) {
      y = Math.max(1970, Math.min(2099, y + delta));
    } else if (pos <= 7) {
      m += delta;
      if (m > 12) { m = 1; y++; }
      if (m < 1) { m = 12; y = Math.max(1970, y - 1); }
    } else if (pos <= 10 || hh === null) {
      d += delta;
      const maxD = daysInMonth(y, m);
      if (d > maxD) d = 1;
      if (d < 1) d = maxD;
    } else if (pos <= 13) {
      hh = (hh + delta + 24) % 24;
    } else if (pos <= 16) {
      mm = ((mm ?? 0) + delta + 60) % 60;
    } else {
      ss = (ss + delta + 60) % 60;
    }
    d = Math.min(d, daysInMonth(y, m));
    const pad = (n: number) => String(n).padStart(2, '0');
    const datePart = `${y}-${pad(m)}-${pad(d)}`;
    editingCell.tempValue = hh !== null ? `${datePart} ${pad(hh)}:${pad(mm ?? 0)}:${pad(ss)}` : datePart;
  }

  function applyEditPreset(preset: 'NULL' | 'EMPTY' | 'NOW' | 'DATE' | 'UUID', origIdx: number, cIdx: number) {
    if (!canEdit || !effectiveTable || !result?.rows) return;
    const colDef = result.columns[cIdx];
    let next: any = '';
    const now = new Date();
    const pad = (n: number) => String(n).padStart(2, '0');
    const today = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
    const time = `${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
    if (preset === 'NULL') next = null;
    else if (preset === 'EMPTY') next = '';
    else if (preset === 'DATE') next = today;
    else if (preset === 'NOW') next = `${today} ${time}`;
    else if (preset === 'UUID') {
      next = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
        ? crypto.randomUUID()
        : '00000000-0000-4000-8000-000000000000';
    }
    const errMsg = validateCellAgainstSchema(next, colDef, tableMeta);
    if (errMsg) {
      appState.showToast(errMsg, 'error');
      return;
    }
    editingCell = {
      rIdx: origIdx,
      cIdx,
      tempValue: next === null ? '' : String(next),
    };
    commitDirectCellValue(origIdx, cIdx, next);
  }

  function commitDirectCellValue(origIdx: number, cIdx: number, newVal: any) {
    if (!canEdit || !effectiveTable || !result?.rows) return;
    const colName = allColumns[cIdx];
    const oldVal = result.rows[origIdx][cIdx];
    if (newVal !== oldVal) {
      const pkCols = result.columns.filter((c) => isColPK(c)).map((c) => getColName(c));
      let pkCol: string | undefined;
      let pkVal: any;
      let pkValues: Record<string, any> | undefined;

      if (pkCols.length === 1) {
        pkCol = pkCols[0];
        pkVal = result.rows[origIdx][colIndexMap.get(pkCol) ?? -1];
      } else if (pkCols.length > 1) {
        pkValues = {};
        for (const col of pkCols) {
          pkValues[col] = result.rows[origIdx][colIndexMap.get(col) ?? -1];
        }
      }

      let rowCriteria: Record<string, any> | undefined;
      if (pkCols.length === 0) {
        rowCriteria = {};
        allColumns.forEach((col, idx) => {
          rowCriteria![col] = result!.rows[origIdx][idx];
        });
      }

      appState.stageEdit(
        effectiveTable,
        origIdx,
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
      result.rows[origIdx][cIdx] = newVal;
      flashCell(origIdx, cIdx);
    }
    editingCell = null;
  }

  // Inline Editing
  function startCellEdit(origIdx: number, cIdx: number, val: any) {
    if (!canEdit) {
      if (appState.isReadOnly) {
        appState.showToast('Session is in Read-Only mode — editing disabled', 'warning');
      } else {
        appState.showToast('Inline editing requires a single target table', 'info');
      }
      return;
    }
    const colName = allColumns[cIdx];
    const fk = fkMap.get(colName);
    if (fk) void ensureFkSuggestions(fk);
    editingCell = {
      rIdx: origIdx,
      cIdx,
      tempValue: val === null || val === undefined ? '' : typeof val === 'object' ? JSON.stringify(val) : String(val),
    };
  }

  function commitCellEdit(origIdx: number, cIdx: number) {
    if (!editingCell || !canEdit || !effectiveTable || !result?.rows) return;
    const colDef = result.columns[cIdx];
    const oldVal = result.rows[origIdx][cIdx];
    const newVal = editingCell.tempValue;

    const oldStr = oldVal === null || oldVal === undefined ? '' : typeof oldVal === 'object' ? JSON.stringify(oldVal) : String(oldVal);
    if (newVal !== oldStr) {
      const errMsg = validateCellAgainstSchema(newVal, colDef, tableMeta);
      if (errMsg) {
        appState.showToast(errMsg, 'error');
        return;
      }
      commitDirectCellValue(origIdx, cIdx, newVal);
      return;
    }
    editingCell = null;
  }

  function isCellStaged(table: string, rowObj: any[] | undefined, rIdx: number, cIdx: number): boolean {
    const uid = rowObj ? appState.getRowUid(rowObj) : rIdx;
    return Boolean(appState.stagedEdits[`${table}:${uid}:${cIdx}`]);
  }

  // Context Menu
  function handleCellContextMenu(e: MouseEvent, origIdx: number, cIdx: number, val: any) {
    e.preventDefault();
    contextMenu = {
      open: true,
      x: e.clientX,
      y: e.clientY,
      rIdx: origIdx,
      cIdx,
      colName: allColumns[cIdx],
      cellValue: val,
    };
  }

  function closeContextMenu() {
    if (contextMenu) contextMenu.open = false;
  }

  // Quick Filter helpers
  function applyQuickFilter(op: FilterPill['op']) {
    if (!contextMenu) return;
    const { colName, cellValue } = contextMenu;
    const val = cellValue === null || cellValue === undefined ? '' : String(cellValue);
    appState.addFilterPill(colName, op, val);
    closeContextMenu();
  }

  // Excel-safe CSV export with UTF-8 BOM & leading zero protection
  function exportCSV() {
    if (!result?.rows || !result?.columns) return;
    const headers = result.columns.map((c) => `"${getColName(c).replace(/"/g, '""')}"`).join(',');
    const rows = result.rows.map((row) =>
      row
        .map((cell) => {
          if (cell === null || cell === undefined) return '""';
          const str = typeof cell === 'object' ? JSON.stringify(cell) : String(cell);
          if (/^0\d+$/.test(str)) {
            return `"=""${str.replace(/"/g, '""')}"""`;
          }
          return `"${str.replace(/"/g, '""')}"`;
        })
        .join(',')
    );
    const csv = '\uFEFF' + [headers, ...rows].join('\n');
    downloadFile(csv, `${effectiveTable || 'query_export'}.csv`, 'text/csv;charset=utf-8');
    showExportMenu = false;
  }

  function copyGridAsTSV() {
    if (!result?.rows || !result?.columns) return;
    const cols = visibleColumns.map((c) => getColName(c)).join('\t');
    const lines = processedRows.map((item) =>
      visibleColumnEntries
        .map(({ origIdx: cIdx }) => {
          const v = item.data[cIdx];
          if (v === null || v === undefined) return '';
          return typeof v === 'object' ? JSON.stringify(v) : String(v);
        })
        .join('\t')
    );
    copyText([cols, ...lines].join('\n'));
    showExportMenu = false;
    appState.showToast(`Copied ${processedRows.length} rows as TSV`, 'success');
  }

  function copyGridAsMarkdown() {
    if (!result?.rows || !result?.columns) return;
    const names = visibleColumns.map((c) => getColName(c));
    const header = `| ${names.join(' | ')} |`;
    const sep = `| ${names.map(() => '---').join(' | ')} |`;
    const lines = processedRows.map((item) =>
      `| ${visibleColumnEntries
        .map(({ origIdx: cIdx }) => {
          const v = item.data[cIdx];
          if (v === null || v === undefined) return '*NULL*';
          const s = typeof v === 'object' ? JSON.stringify(v) : String(v);
          return s.replace(/\|/g, '\\|').replace(/\n/g, ' ');
        })
        .join(' | ')} |`
    );
    copyText([header, sep, ...lines].join('\n'));
    showExportMenu = false;
    appState.showToast(`Copied ${processedRows.length} rows as Markdown Table`, 'success');
  }

  function formatRowCell(v: any): string {
    if (v === null || v === undefined) return '';
    return typeof v === 'object' ? JSON.stringify(v) : String(v);
  }

  function formatSqlRowInsert(row: any[], cols: string[], tbl: string, driver?: string): string {
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const q = (s: string) => quoteSqlIdentifier(s, driver);
    const qCols = cols.map((c) => q(c)).join(', ');
    const vals = row
      .map((c) => (typeof c === 'boolean' ? (c ? (isPg ? 'TRUE' : '1') : (isPg ? 'FALSE' : '0')) : formatSqlLiteral(c)))
      .join(', ');
    return `INSERT INTO ${q(tbl)} (${qCols}) VALUES (${vals});`;
  }

  function copyRowAsTSV(rIdx: number) {
    if (!result?.rows || !result.rows[rIdx] || !result.columns) return;
    const cols = result.columns.map((c) => getColName(c)).join('\t');
    const vals = result.rows[rIdx].map((v) => (v === null || v === undefined ? '' : formatRowCell(v))).join('\t');
    copyText(`${cols}\n${vals}`);
    appState.showToast('Copied row as TSV', 'info');
  }

  function copyRowAsJSON(rIdx: number) {
    if (!result?.rows || !result.rows[rIdx] || !result.columns) return;
    const obj: Record<string, any> = {};
    result.columns.forEach((c, idx) => {
      obj[getColName(c)] = result!.rows[rIdx][idx];
    });
    copyText(JSON.stringify(obj, null, 2));
    appState.showToast('Copied row as JSON', 'info');
  }

  function copyRowAsMarkdown(rIdx: number) {
    if (!result?.rows || !result.rows[rIdx] || !result.columns) return;
    const colNames = result.columns.map((c) => getColName(c));
    const header = `| ${colNames.join(' | ')} |`;
    const separator = `| ${colNames.map(() => '---').join(' | ')} |`;
    const row = `| ${result.rows[rIdx].map((v) => (v === null ? '*NULL*' : formatRowCell(v).replace(/\|/g, '\\|'))).join(' | ')} |`;
    copyText(`${header}\n${separator}\n${row}`);
    appState.showToast('Copied row as Markdown Table', 'info');
  }

  function copyRowAsInsertSQL(rIdx: number) {
    if (!result?.rows || !result.rows[rIdx] || !result.columns) return;
    const tbl = effectiveTable || 'table_name';
    const cols = result.columns.map((c) => getColName(c));
    copyText(formatSqlRowInsert(result.rows[rIdx], cols, tbl, appState.activeConnection?.driver));
    appState.showToast('Copied as INSERT SQL statement', 'info');
  }

  function copySelectedRows(format: 'tsv' | 'json' | 'md' | 'sql') {
    if (!result?.rows || !result?.columns) return;
    const indices = selectedRowIndices.length > 0 ? selectedRowIndices : contextMenu ? [contextMenu.rIdx] : [];
    if (indices.length === 0) return;
    if (indices.length === 1) {
      const rIdx = indices[0];
      if (format === 'tsv') copyRowAsTSV(rIdx);
      else if (format === 'json') copyRowAsJSON(rIdx);
      else if (format === 'md') copyRowAsMarkdown(rIdx);
      else copyRowAsInsertSQL(rIdx);
      return;
    }
    const cols = result.columns.map((c) => getColName(c));
    if (format === 'tsv') {
      const lines = indices.map((rIdx) => result!.rows[rIdx].map(formatRowCell).join('\t'));
      copyText(`${cols.join('\t')}\n${lines.join('\n')}`);
    } else if (format === 'json') {
      const objs = indices.map((rIdx) => {
        const obj: Record<string, any> = {};
        cols.forEach((col, i) => (obj[col] = result!.rows[rIdx][i]));
        return obj;
      });
      copyText(JSON.stringify(objs, null, 2));
    } else if (format === 'md') {
      const header = `| ${cols.join(' | ')} |`;
      const sep = `| ${cols.map(() => '---').join(' | ')} |`;
      const lines = indices.map((rIdx) => `| ${result!.rows[rIdx].map((v) => (v === null ? '*NULL*' : formatRowCell(v).replace(/\|/g, '\\|'))).join(' | ')} |`);
      copyText(`${header}\n${sep}\n${lines.join('\n')}`);
    } else {
      const tbl = effectiveTable || 'table_name';
      const lines = indices.map((rIdx) => formatSqlRowInsert(result!.rows[rIdx], cols, tbl, appState.activeConnection?.driver));
      copyText(lines.join('\n'));
    }
    appState.showToast(`Copied ${indices.length} row(s) as ${format.toUpperCase()}`, 'info');
  }

  function exportJSON() {
    if (!result?.rows || !result?.columns) return;
    const cols = result.columns.map((c) => getColName(c));
    const data = result.rows.map((row) => {
      const obj: Record<string, any> = {};
      cols.forEach((col, idx) => {
        obj[col] = row[idx];
      });
      return obj;
    });
    const jsonStr = JSON.stringify(data, null, 2);
    downloadFile(jsonStr, `${currentTab?.tableName || 'query_export'}.json`, 'application/json');
    showExportMenu = false;
  }

  function exportSQL() {
    if (!result?.rows || !result?.columns) return;
    const tbl = currentTab?.tableName || 'exported_table';
    const driver = appState.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const q = (s: string) => quoteSqlIdentifier(s, driver);

    const cols = result.columns.map((c) => q(getColName(c))).join(', ');
    const inserts = result.rows.map((row) => {
      const vals = row.map((c) => {
        if (typeof c === 'boolean') return c ? (isPg ? 'TRUE' : '1') : (isPg ? 'FALSE' : '0');
        return formatSqlLiteral(c);
      }).join(', ');
      return `INSERT INTO ${q(tbl)} (${cols}) VALUES (${vals});`;
    });
    downloadFile(inserts.join('\n'), `${tbl}.sql`, 'text/plain');
    showExportMenu = false;
  }

  function downloadFile(content: string, filename: string, type: string) {
    const blob = new Blob([content], { type });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    appState.showToast(`Exported ${filename}`, 'success');
  }

  // Delete Row (supports composite PK and PK-less criteria fallback)
  async function handleDeleteCurrentRow(targetIdx: number | null = selectedRowIndex) {
    if (targetIdx === null || !effectiveTable || !result?.rows?.[targetIdx]) return;
    if (appState.isReadOnly) {
      appState.showToast('Read-only mode is active — delete blocked', 'warning');
      return;
    }
    const pkCols = result.columns.filter((c) => isColPK(c)).map((c) => getColName(c));
    if (pkCols.length === 1) {
      const pkCol = pkCols[0];
      const pkVal = result.rows[targetIdx][colIndexMap.get(pkCol) ?? -1];
      const ok = await appState.showNativeConfirm(
        `Delete record with ${pkCol} = ${pkVal}?`,
        'Confirm Deletion'
      );
      if (ok) {
        await appState.deleteRows(pkCol, [pkVal]);
        if (selectedRowIndex === targetIdx) selectedRowIndex = null;
      }
    } else if (pkCols.length > 1) {
      const pkMap: Record<string, any> = {};
      const pkDesc: string[] = [];
      for (const col of pkCols) {
        const val = result.rows[targetIdx][colIndexMap.get(col) ?? -1];
        pkMap[col] = val;
        pkDesc.push(`${col}=${val}`);
      }
      const ok = await appState.showNativeConfirm(
        `Delete record with composite PK (${pkDesc.join(', ')})?`,
        'Confirm Deletion'
      );
      if (ok) {
        await appState.deleteRowsByCompositePK([pkMap]);
        if (selectedRowIndex === targetIdx) selectedRowIndex = null;
      }
    } else {
      const crit: Record<string, any> = {};
      allColumns.forEach((col, cIdx) => {
        crit[col] = result!.rows[targetIdx][cIdx];
      });
      const ok = await appState.showNativeConfirm(
        `Delete row #${targetIdx + 1} from ${effectiveTable} by matching all columns?`,
        'Confirm Deletion (No PK)'
      );
      if (ok) {
        await appState.deleteRowsByCriteria([crit]);
        if (selectedRowIndex === targetIdx) selectedRowIndex = null;
      }
    }
  }

  function advanceActiveCell(dRow: number, dCol: number) {
    if (processedRows.length === 0 || visibleColumnEntries.length === 0) return;
    const curDisplayIdx = selectedCell
      ? Math.max(0, processedRows.findIndex((r) => r.origIdx === selectedCell!.rIdx))
      : 0;
    const curVisColIdx = selectedCell
      ? Math.max(0, visibleColumnEntries.findIndex((e) => e.origIdx === selectedCell!.cIdx))
      : 0;

    let nextDisplayIdx = Math.max(0, Math.min(processedRows.length - 1, curDisplayIdx + dRow));
    let nextVisColIdx = curVisColIdx + dCol;
    if (nextVisColIdx >= visibleColumnEntries.length) {
      nextVisColIdx = 0;
      nextDisplayIdx = Math.min(processedRows.length - 1, nextDisplayIdx + 1);
    } else if (nextVisColIdx < 0) {
      nextVisColIdx = visibleColumnEntries.length - 1;
      nextDisplayIdx = Math.max(0, nextDisplayIdx - 1);
    }

    const row = processedRows[nextDisplayIdx];
    const colEntry = visibleColumnEntries[nextVisColIdx];
    if (!row || !colEntry) return;
    selectedCell = { rIdx: row.origIdx, cIdx: colEntry.origIdx };
    rangeAnchor = { dispRow: nextDisplayIdx, visCol: nextVisColIdx };
    rangeEnd = { dispRow: nextDisplayIdx, visCol: nextVisColIdx };
    selectedRowIndex = row.origIdx;

    if (containerRef) {
      const y = nextDisplayIdx * ROW_HEIGHT;
      if (y < containerRef.scrollTop) containerRef.scrollTop = y;
      else if (y + ROW_HEIGHT > containerRef.scrollTop + containerRef.clientHeight - 60) {
        containerRef.scrollTop = y + ROW_HEIGHT - containerRef.clientHeight + 60;
      }
    }
  }

  let selectedRangeBounds = $derived.by(() => {
    if (!rangeAnchor || !rangeEnd) return null;
    return {
      rMin: Math.min(rangeAnchor.dispRow, rangeEnd.dispRow),
      rMax: Math.max(rangeAnchor.dispRow, rangeEnd.dispRow),
      cMin: Math.min(rangeAnchor.visCol, rangeEnd.visCol),
      cMax: Math.max(rangeAnchor.visCol, rangeEnd.visCol),
    };
  });

  let rangeStats = $derived.by(() => {
    const b = selectedRangeBounds;
    if (!b) return null;
    const cellCount = (b.rMax - b.rMin + 1) * (b.cMax - b.cMin + 1);
    if (cellCount <= 1) return null;
    let numCount = 0;
    let sum = 0;
    let min = Infinity;
    let max = -Infinity;
    // Cap scan at 5,000 cells for instant UI feedback
    let scanned = 0;
    for (let r = b.rMin; r <= b.rMax && scanned < 5000; r++) {
      const row = processedRows[r];
      if (!row) continue;
      for (let c = b.cMin; c <= b.cMax && scanned < 5000; c++) {
        scanned++;
        const colEntry = visibleColumnEntries[c];
        if (!colEntry) continue;
        const val = row.data[colEntry.origIdx];
        if (val !== null && val !== undefined && val !== '' && typeof val !== 'boolean') {
          const n = typeof val === 'number' ? val : Number(val);
          if (Number.isFinite(n)) {
            numCount++;
            sum += n;
            if (n < min) min = n;
            if (n > max) max = n;
          }
        }
      }
    }
    return {
      cellCount,
      numCount,
      sum,
      avg: numCount > 0 ? sum / numCount : 0,
      min: numCount > 0 ? min : 0,
      max: numCount > 0 ? max : 0,
    };
  });

  function isCellInRange(dispRow: number, visCol: number): boolean {
    const b = selectedRangeBounds;
    if (!b) return false;
    return dispRow >= b.rMin && dispRow <= b.rMax && visCol >= b.cMin && visCol <= b.cMax;
  }

  function copySelectedRangeTSV() {
    const b = selectedRangeBounds;
    if (b) {
      const lines: string[] = [];
      for (let r = b.rMin; r <= b.rMax; r++) {
        const row = processedRows[r];
        if (!row) continue;
        const vals: string[] = [];
        for (let c = b.cMin; c <= b.cMax; c++) {
          const colEntry = visibleColumnEntries[c];
          const v = colEntry ? row.data[colEntry.origIdx] : '';
          vals.push(v === null || v === undefined ? '' : String(v));
        }
        lines.push(vals.join('\t'));
      }
      copyText(lines.join('\n'));
      appState.showToast(`Copied ${lines.length}×${b.cMax - b.cMin + 1} cell(s) as TSV`, 'success');
      return;
    }
    if (selectedCell && result?.rows?.[selectedCell.rIdx]) {
      const v = result.rows[selectedCell.rIdx][selectedCell.cIdx];
      copyText(v === null || v === undefined ? '' : String(v));
      appState.showToast('Copied cell value', 'success');
    }
  }

  function handleGridKeydown(e: KeyboardEvent) {
    if (editingCell) return;
    const target = e.target as HTMLElement | null;
    if (target && target.closest('input, textarea, select')) return;

    const isCtrl = e.ctrlKey || e.metaKey;
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 'n') {
      e.preventDefault();
      addDraftRow();
      return;
    }
    if (isCtrl && e.key.toLowerCase() === 'f') {
      e.preventDefault();
      localSearchInputEl?.focus();
      localSearchInputEl?.select();
      return;
    }
    if (isCtrl && !e.shiftKey && e.key.toLowerCase() === 'c') {
      const selText = window.getSelection()?.toString();
      if (!selText && (selectedRangeBounds || selectedCell)) {
        e.preventDefault();
        copySelectedRangeTSV();
        return;
      }
    }
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 'e') {
      e.preventDefault();
      if (totalRows > 0) {
        appState.recordInspectorRowIndex = selectedRowIndex ?? 0;
        appState.isRecordInspectorOpen = true;
      }
      return;
    }
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 'd') {
      e.preventDefault();
      if (selectedRowIndex !== null) duplicateRowAsDraft(selectedRowIndex, true);
      return;
    }
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 't') {
      e.preventDefault();
      showDetailDrawer = !showDetailDrawer;
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      advanceActiveCell(1, 0);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      advanceActiveCell(-1, 0);
    } else if (e.key === 'ArrowRight') {
      e.preventDefault();
      advanceActiveCell(0, 1);
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      advanceActiveCell(0, -1);
    } else if (e.key === 'Tab') {
      e.preventDefault();
      advanceActiveCell(0, e.shiftKey ? -1 : 1);
    } else if (e.key === ' ' && selectedRowIndex !== null) {
      e.preventDefault();
      toggleRowSelection(selectedRowIndex);
    } else if ((e.key === 'Enter' || e.key === 'F2') && selectedCell && result?.rows?.[selectedCell.rIdx]) {
      e.preventDefault();
      startCellEdit(selectedCell.rIdx, selectedCell.cIdx, result.rows[selectedCell.rIdx][selectedCell.cIdx]);
    }
  }
</script>

<svelte:window onclick={closeContextMenu} />

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text relative">
  <!-- Multi-Statement Execution Results Tabs -->
  {#if currentTab?.multiResults && currentTab.multiResults.length > 1}
    <div class="h-8 bg-[#0b0f19] border-b border-slate-800 px-3 flex items-center justify-between gap-2 select-none shrink-0">
      <div class="flex items-center space-x-1 overflow-x-auto min-w-0">
        {#each currentTab.multiResults as mRes, mIdx}
          {@const isCmd = !mRes.columns || mRes.columns.length === 0}
          {@const countVal = isCmd ? (mRes.rows_affected ?? mRes.affected_rows ?? 0) : (mRes.row_count ?? mRes.rows?.length ?? 0)}
          <button
            type="button"
            onclick={() => {
              if (currentTab) {
                currentTab.activeResultIndex = mIdx;
                currentTab.activeResult = mRes;
              }
            }}
            class="px-2.5 py-1 rounded text-xs font-mono font-medium transition flex items-center space-x-1.5 shrink-0 {(currentTab.activeResultIndex || 0) === mIdx ? 'bg-sky-500/20 text-sky-300 font-bold border border-sky-500/50' : 'text-slate-400 hover:text-white hover:bg-slate-800'}"
          >
            <span class="w-1.5 h-1.5 rounded-full shrink-0 {mRes.error ? 'bg-rose-400' : isCmd ? 'bg-emerald-400' : 'bg-sky-400'}"></span>
            <span>
              Result {mIdx + 1} ({mRes.error ? 'err' : isCmd ? `${countVal} affected` : `${countVal} rows`})
            </span>
            {#if mRes.truncated || mRes.capped_at}
              <span class="px-1 py-0.2 rounded bg-amber-950/70 border border-amber-800/70 text-[9px] text-amber-300" title="Capped at {mRes.capped_at || '5k'} rows">
                Capped
              </span>
            {/if}
          </button>
        {/each}
      </div>
      {#if result}
        <button
          type="button"
          onclick={() => {
            const tab = appState.addTab(`Result ${(currentTab.activeResultIndex || 0) + 1}`, currentTab.sql || '');
            tab.activeResult = JSON.parse(JSON.stringify(result));
            appState.saveTabsState();
          }}
          class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[10px] font-mono shrink-0 transition"
          title="Open active result set in a dedicated tab"
        >
          Open in New Tab
        </button>
      {/if}
    </div>
  {/if}

  <!-- Profiler Panel Drawer -->
  {#if currentTab?.showQueryProfiler}
    <div class="h-48 shrink-0 overflow-hidden">
      <QueryProfilerPanel />
    </div>
  {/if}

  <!-- Grid Control Toolbar -->
  <div id="grid-toolbar" class="h-9 bg-[#0b0f19] border-b border-slate-800 px-3 flex items-center justify-between text-xs select-none shrink-0 gap-2">
    <!-- Left: CRUD & Staged Actions -->
    <div class="flex items-center space-x-2">
      <!-- In-Place Draft Row Button -->
      <button
        onclick={addDraftRow}
        class="h-6 px-2.5 rounded bg-sky-500/20 hover:bg-sky-500/30 text-sky-300 border border-sky-500/40 font-semibold flex items-center space-x-1 transition text-[11px]"
        title="Add in-place draft row"
      >
        <Icon name="plus" class="w-3 h-3 text-sky-400" />
        <span>Draft Row</span>
      </button>

      {#if selectedRowIndices.length > 0}
        <button
          onclick={handleBatchDelete}
          class="h-6 px-2.5 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold flex items-center space-x-1 transition text-[11px] shadow animate-in fade-in"
          title="Delete selected rows"
        >
          <Icon name="trash-2" class="w-3 h-3" />
          <span>Delete ({selectedRowIndices.length})</span>
        </button>
      {/if}

      {#if currentTab?.isTableViewer}
        <button
          id="btn-grid-add-row"
          onclick={() => (appState.isInsertRowModalOpen = true)}
          class="h-6 px-2.5 rounded bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-300 border border-emerald-500/40 font-semibold flex items-center space-x-1.5 transition text-[11px]"
          title="Insert new record"
        >
          <Icon name="plus" class="w-3 h-3 text-emerald-400" />
          <span>Insert Row</span>
        </button>

        <button
          id="btn-import-csv"
          onclick={() => appState.openCsvImportModal()}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 flex items-center space-x-1 transition text-[11px]"
          title="Import records from a CSV file"
        >
          <Icon name="upload" class="w-3 h-3 text-indigo-300" />
          <span>Import CSV</span>
        </button>

        <button
          id="btn-mock-data"
          onclick={() => appState.openMockDataModal(effectiveTable)}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 flex items-center space-x-1 transition text-[11px]"
          title="Generate synthetic test records"
        >
          <Icon name="sparkles" class="w-3 h-3 text-purple-300" />
          <span>Mock Data</span>
        </button>

        <button
          onclick={() => handleDeleteCurrentRow()}
          disabled={selectedRowIndex === null}
          class="h-6 px-2 rounded bg-rose-500/10 hover:bg-rose-500/20 disabled:opacity-40 text-rose-300 border border-rose-500/30 flex items-center space-x-1 transition text-[11px]"
          title="Delete selected row"
        >
          <Icon name="trash-2" class="w-3 h-3 text-rose-400" />
          <span>Delete</span>
        </button>
      {/if}

      {#if currentTab?.isTableViewer || (result?.columns && result.columns.length > 0)}
        {#if currentTab?.isTableViewer}
          <div class="h-4 w-px bg-slate-800 mx-0.5"></div>
        {/if}

        <!-- Filter Bar Toggle Button -->
        <button
          id="btn-grid-filter"
          type="button"
          onclick={toggleFilterBar}
          class="h-6 px-2 rounded {isFilterOpen ? 'bg-sky-500/20 text-sky-300 border-sky-500/40 font-medium' : hasActiveFilter ? 'bg-emerald-950/40 text-emerald-300 border-emerald-600/50 hover:bg-emerald-900/40' : 'bg-[#1e293b] text-slate-300 hover:text-white'} border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1.5 text-[11px] transition"
          title={hasActiveFilter
            ? currentTab?.isTableViewer
              ? 'Filter active (click to toggle WHERE / Pills bar)'
              : 'Filter active (click to toggle Filter Pills bar)'
            : currentTab?.isTableViewer
              ? 'Toggle Filter (WHERE / Pills)'
              : 'Toggle Filter (Pills)'}
        >
          <Icon name="filter" class="w-3 h-3 {isFilterOpen ? 'text-sky-400' : hasActiveFilter ? 'text-emerald-400' : 'text-slate-400'}" />
          <span>Filter</span>
          {#if hasActiveFilter}
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" title="Active filter applied"></span>
          {/if}
        </button>
      {/if}

      {#if currentTab?.isTableViewer || canEdit}
        <div
          id="grid-edit-mode-container"
          class="flex items-center space-x-1.5 rounded border border-slate-800 bg-slate-900 px-2 py-0.5 text-[11px]"
        >
          <span class="text-[10px] text-slate-400">Edit:</span>
          <button
            id="btn-grid-mode-staged"
            type="button"
            onclick={() => appState.setGridEditMode('staged')}
            class="rounded px-1.5 py-0.5 text-[10px] font-medium transition {appState.gridEditMode === 'staged'
              ? 'bg-sky-500/20 text-sky-300'
              : 'text-slate-400 hover:text-slate-200'}"
            title="Manual apply: changes stay staged until you click Apply"
          >
            Staged
          </button>
          <button
            id="btn-grid-mode-auto"
            type="button"
            onclick={() => appState.setGridEditMode('auto')}
            class="rounded px-1.5 py-0.5 text-[10px] font-medium transition {appState.gridEditMode === 'auto'
              ? 'bg-amber-500/20 text-amber-300'
              : 'text-slate-400 hover:text-slate-200'}"
            title="Auto-commit: persist directly on blur/Enter"
          >
            Auto
          </button>
          <label class="flex items-center space-x-1 pl-1.5 border-l border-slate-700/80 text-[10px] text-slate-400 cursor-pointer" title="Require confirmation prompt before saving edits">
            <input
              type="checkbox"
              checked={appState.gridConfirmSave}
              onchange={(e) => appState.setGridConfirmSave((e.currentTarget as HTMLInputElement).checked)}
              class="rounded border-slate-700 bg-slate-950 text-sky-500 focus:ring-0"
            />
            <span>Confirm Save</span>
          </label>
        </div>
      {/if}

      <!-- Staged Edits Review Pill -->
      {#if appState.stagedCount > 0}
        <div class="flex items-center space-x-1.5 bg-amber-950/60 border border-amber-600/70 rounded px-2 py-0.5 animate-in fade-in">
          <span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
          <span class="text-[11px] font-bold text-amber-300 font-mono">
            {appState.stagedCount} Staged Edit{appState.stagedCount > 1 ? 's' : ''}
          </span>
          <button
            onclick={() => appState.commitStagedEdits()}
            class="ml-1 px-1.5 py-0.2 rounded bg-amber-500 text-slate-950 font-bold hover:bg-amber-400 text-[10px] transition"
          >
            Save All
          </button>
          <button
            onclick={() => appState.discardStagedEdits()}
            class="px-1 text-slate-400 hover:text-white text-[10px] transition"
          >
            Discard
          </button>
        </div>
      {/if}
    </div>

    <!-- Right: Range Stats, Local Search, Column Picker, Export & Inspect -->
    <div class="flex items-center space-x-2">
      {#if rangeStats}
        <div
          class="hidden xl:flex items-center space-x-2 px-2 py-0.5 rounded bg-sky-950/60 border border-sky-700/50 text-[10px] font-mono text-sky-200"
          title="Selection summary (Shift+Click range, Ctrl+C to copy TSV)"
        >
          <span>{rangeStats.cellCount} cells</span>
          {#if rangeStats.numCount > 0}
            <span class="text-slate-500">|</span>
            <span>Sum: <strong class="text-emerald-300">{Number(rangeStats.sum.toFixed(4))}</strong></span>
            <span>Avg: <strong class="text-sky-300">{Number(rangeStats.avg.toFixed(4))}</strong></span>
            <span>Min: {Number(rangeStats.min.toFixed(4))}</span>
            <span>Max: {Number(rangeStats.max.toFixed(4))}</span>
          {/if}
        </div>
      {/if}

      <div class="relative flex items-center">
        <Icon name="search" class="w-3 h-3 text-slate-500 absolute left-2 pointer-events-none" />
        <input
          bind:this={localSearchInputEl}
          type="text"
          bind:value={localSearchQuery}
          placeholder="Filter results (Ctrl+F)..."
          class="h-6 w-36 focus:w-48 transition-all pl-6 pr-5 rounded bg-[#090d16] border border-slate-700/80 focus:border-sky-500/70 text-[11px] text-slate-200 placeholder:text-slate-500 outline-none"
        />
        {#if localSearchQuery}
          <button
            onclick={() => (localSearchQuery = '')}
            class="absolute right-1.5 text-slate-500 hover:text-slate-200"
            title="Clear result filter"
            aria-label="Clear result filter"
          >
            <Icon name="x" class="w-3 h-3" />
          </button>
        {/if}
      </div>

      <!-- Column Picker Dropdown -->
      <div class="relative">
        <button
          id="btn-col-picker"
          onclick={(e) => { e.stopPropagation(); showColPicker = !showColPicker; }}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
          title="Toggle column visibility"
          aria-expanded={showColPicker}
          aria-controls="col-picker-dropdown"
        >
          <Icon name="columns" class="w-3 h-3 text-sky-400" />
          <span>Columns (<span id="col-picker-count-badge">{visibleColumns.length}</span>/{allColumns.length})</span>
          <Icon name="chevron-down" class="w-2.5 h-2.5 text-slate-400" />
        </button>

        {#if showColPicker}
          <button
            tabindex="-1"
            aria-label="Close"
            class="fixed inset-0 z-30 bg-transparent cursor-default"
            onclick={() => (showColPicker = false)}
          ></button>
          <div
            id="col-picker-dropdown"
            use:viewportFit
            class="absolute right-0 mt-1 w-56 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-40 p-2 text-xs space-y-2 animate-in fade-in duration-100"
          >
            <input
              id="col-picker-search"
              type="text"
              bind:value={colPickerSearch}
              placeholder="Search columns..."
              class="w-full bg-[#090d16] border border-slate-700/80 rounded px-2 py-1 text-xs text-slate-100 font-mono focus:outline-none"
            />
            <div class="flex items-center justify-between text-[10px] text-sky-400 px-1">
              <button onclick={() => appState.showAllColumns()} class="hover:underline">Show All</button>
              <button onclick={() => appState.hideAllColumns(allColumns)} class="hover:underline">Hide All</button>
            </div>
            <div id="col-picker-list" class="max-h-48 overflow-y-auto space-y-1 py-1 divide-y divide-slate-800/40">
              {#each allColumns as col}
                {#if !colPickerSearch || col.toLowerCase().includes(colPickerSearch.toLowerCase())}
                  <label class="flex items-center space-x-2 py-1 px-1 rounded hover:bg-slate-800 cursor-pointer text-slate-300 font-mono text-[11px]">
                    <input
                      type="checkbox"
                      checked={!hiddenCols.includes(col)}
                      onchange={() => appState.toggleColumnHidden(col)}
                      class="rounded bg-[#090d16] border-slate-700 text-sky-500 focus:ring-0"
                    />
                    <span class="truncate">{col}</span>
                  </label>
                {/if}
              {/each}
            </div>
          </div>
        {/if}
      </div>

      <!-- Export Dropdown -->
      <div id="export-csv-dropdown-container" class="relative">
        <button
          onclick={(e) => { e.stopPropagation(); showExportMenu = !showExportMenu; }}
          class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
          title="Export data"
        >
          <Icon name="share-2" class="w-3 h-3 text-sky-400" />
          <span>Export</span>
          <Icon name="chevron-down" class="w-2.5 h-2.5 text-slate-400" />
        </button>

        {#if showExportMenu}
          <button
            tabindex="-1"
            aria-label="Close"
            class="fixed inset-0 z-30 bg-transparent cursor-default"
            onclick={() => (showExportMenu = false)}
          ></button>
          <div
            id="export-csv-menu"
            use:viewportFit
            class="absolute right-0 mt-1 w-44 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-40 py-1 divide-y divide-slate-800 animate-in fade-in duration-100 text-xs"
          >
            <button
              onclick={exportCSV}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="file-text" class="w-3.5 h-3.5 text-emerald-400" />
              <span>Export as CSV</span>
            </button>
            <button
              onclick={exportJSON}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="file-code" class="w-3.5 h-3.5 text-amber-400" />
              <span>Export as JSON</span>
            </button>
            <button
              onclick={exportSQL}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="code" class="w-3.5 h-3.5 text-sky-400" />
              <span>Export as SQL</span>
            </button>
            <button
              onclick={copyGridAsTSV}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="copy" class="w-3.5 h-3.5 text-emerald-300" />
              <span>Copy Grid as TSV</span>
            </button>
            <button
              onclick={copyGridAsMarkdown}
              class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <Icon name="copy" class="w-3.5 h-3.5 text-purple-300" />
              <span>Copy Grid as Markdown</span>
            </button>
            {#if currentTab?.isTableViewer && currentTab.tableName}
              <button
                id="export-menu-table-dump"
                onclick={() => {
                  showExportMenu = false;
                  appState.exportTableDump(currentTab!.tableName);
                }}
                class="flex w-full flex-col items-start px-3 py-1.5 text-left hover:bg-slate-800"
              >
                <span class="font-medium text-sky-400">Table SQL Dump</span>
                <span class="text-[10px] text-slate-400">Export this table's DDL + data (.sql)</span>
              </button>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Record Inspector Trigger -->
      <button
        onclick={() => {
          appState.recordInspectorRowIndex = selectedRowIndex ?? 0;
          appState.isRecordInspectorOpen = true;
        }}
        disabled={totalRows === 0}
        class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
        title="Inspect record in vertical form"
      >
        <Icon name="eye" class="w-3 h-3 text-sky-400" />
        <span>Inspect</span>
      </button>

      <!-- Cell Drawer Toggle -->
      <button
        onclick={() => (showDetailDrawer = !showDetailDrawer)}
        class="h-6 px-2 rounded {showDetailDrawer ? 'bg-sky-500/20 text-sky-300 border-sky-500/40' : 'bg-[#1e293b] text-slate-400 hover:text-white'} border border-slate-700/80 hover:border-slate-600 flex items-center space-x-1 text-[11px] transition"
        title="Toggle Cell Drawer"
      >
        <Icon name="sidebar" class="w-3 h-3" />
        <span>Drawer</span>
      </button>
    </div>
  </div>

  <!-- TablePlus-style Filter Bar (Table Viewer: Pills / WHERE; Query Tab: Client-side Pills) -->
  {#if isFilterOpen && (currentTab?.isTableViewer || (result?.columns && result.columns.length > 0))}
    <div class="min-h-9 bg-[#0b0f19] border-b border-slate-800 px-3 py-1.5 flex flex-wrap items-center justify-between gap-2 text-xs select-none shrink-0 animate-in fade-in duration-100">
      <div class="flex flex-wrap items-center gap-2 flex-1 min-w-0">
        {#if currentTab?.isTableViewer}
          <!-- Mode Toggle: Pills vs WHERE -->
          <div class="inline-flex rounded border border-slate-700/80 bg-[#090d16] p-0.5 select-none" role="group">
            <button
              type="button"
              onclick={() => appState.setFilterMode('pills')}
              class="px-2 py-0.5 text-[11px] font-semibold rounded transition {currentTab.filterMode !== 'raw' ? 'bg-sky-500 text-slate-950 shadow-sm' : 'text-slate-400 hover:text-white'}"
              title="Filter by column pills"
            >
              Pills
            </button>
            <button
              type="button"
              onclick={() => appState.setFilterMode('raw')}
              class="px-2 py-0.5 text-[11px] font-semibold rounded transition {currentTab.filterMode === 'raw' ? 'bg-sky-500 text-slate-950 shadow-sm' : 'text-slate-400 hover:text-white'}"
              title="Freeform SQL WHERE expression"
            >
              WHERE
            </button>
          </div>
        {:else}
          <span class="px-1.5 py-0.5 bg-sky-950/80 text-sky-400 border border-sky-800/80 rounded font-mono text-[10px] font-bold tracking-wider select-none shrink-0">
            FILTER
          </span>
        {/if}

        {#if currentTab?.isTableViewer && currentTab.filterMode === 'raw'}
          <!-- Freeform SQL WHERE Expression Input -->
          <div class="flex-1 flex items-center space-x-1.5 min-w-[260px]">
            <span class="px-1.5 py-0.5 bg-sky-950 text-sky-400 border border-sky-800 rounded font-mono text-[10px] font-bold tracking-wider select-none shrink-0">
              WHERE
            </span>
            <div class="relative flex-1 flex items-center">
              <input
                id="table-filter-raw-input"
                type="text"
                value={currentTab.filterRaw || ''}
                oninput={(e) => appState.setFilterRaw((e.target as HTMLInputElement).value)}
                onkeydown={(e) => {
                  if (e.key === 'Enter') appState.applyTableFilter();
                  if (e.key === 'Escape') {
                    appState.setFilterRaw('');
                    (e.target as HTMLInputElement).blur();
                  }
                }}
                placeholder="e.g. status = 'active' AND (views > 100 OR is_featured = 1)"
                class="w-full bg-[#090d16] text-slate-100 placeholder-slate-500 border border-[#334155] focus:border-sky-400 pl-2.5 pr-7 py-1 rounded outline-none font-mono text-xs ring-1 ring-transparent focus:ring-sky-500/30 transition"
              />
              {#if currentTab.filterRaw}
                <button
                  type="button"
                  onclick={() => appState.setFilterRaw('')}
                  class="absolute right-2 text-slate-500 hover:text-slate-200 text-xs px-1 rounded transition"
                  title="Clear input (Esc)"
                >
                  <Icon name="x" class="w-3 h-3" />
                </button>
              {/if}
            </div>
            <button
              type="button"
              onclick={() => appState.applyTableFilter()}
              class="px-2.5 py-1 rounded bg-sky-600 hover:bg-sky-500 text-white font-semibold text-[11px] transition shrink-0"
            >
              Apply
            </button>
          </div>
        {:else}
          <!-- Pills Mode: List of condition pills + '+ AND' button -->
          <div class="flex flex-wrap items-center gap-1.5">
            {#each currentTab?.filterPills || [] as pill, idx}
              {@const isUnary = pill.op === 'IS NULL' || pill.op === 'IS NOT NULL'}
              <div class="inline-flex items-center space-x-1 bg-[#090d16] border border-sky-800/80 rounded px-1.5 py-0.5 text-[11px] font-mono shadow-sm">
                <!-- Column Select -->
                <select
                  value={pill.col}
                  onchange={(e) => {
                    appState.updateFilterPill(idx, { col: (e.target as HTMLSelectElement).value });
                  }}
                  class="bg-transparent text-sky-300 font-bold outline-none cursor-pointer text-[11px] py-0.5 border-none"
                >
                  {#each allColumns as col}
                    <option value={col} class="bg-[#0f172a] text-slate-200">{col}</option>
                  {/each}
                </select>

                <!-- Operator Select -->
                <select
                  value={pill.op}
                  onchange={(e) => {
                    const newOp = (e.target as HTMLSelectElement).value as FilterPill['op'];
                    appState.updateFilterPill(idx, { op: newOp });
                  }}
                  class="bg-transparent text-sky-400 font-semibold outline-none cursor-pointer text-[11px] py-0.5 border-none"
                >
                  <option value="=" class="bg-[#0f172a] text-slate-200">=</option>
                  <option value="!=" class="bg-[#0f172a] text-slate-200">!=</option>
                  <option value="LIKE" class="bg-[#0f172a] text-slate-200">LIKE</option>
                  <option value="NOT LIKE" class="bg-[#0f172a] text-slate-200">NOT LIKE</option>
                  <option value=">" class="bg-[#0f172a] text-slate-200">&gt;</option>
                  <option value="<" class="bg-[#0f172a] text-slate-200">&lt;</option>
                  <option value=">=" class="bg-[#0f172a] text-slate-200">&gt;=</option>
                  <option value="<=" class="bg-[#0f172a] text-slate-200">&lt;=</option>
                  <option value="IN" class="bg-[#0f172a] text-slate-200">IN</option>
                  <option value="NOT IN" class="bg-[#0f172a] text-slate-200">NOT IN</option>
                  <option value="BETWEEN" class="bg-[#0f172a] text-slate-200">BETWEEN</option>
                  <option value="IS NULL" class="bg-[#0f172a] text-slate-200">IS NULL</option>
                  <option value="IS NOT NULL" class="bg-[#0f172a] text-slate-200">IS NOT NULL</option>
                </select>

                <!-- Value Input (hidden if IS NULL / IS NOT NULL) -->
                {#if !isUnary}
                  <input
                    id="grid-filter-pill-val-{idx}"
                    type="text"
                    value={pill.val}
                    oninput={(e) => {
                      appState.updateFilterPill(idx, { val: (e.target as HTMLInputElement).value });
                    }}
                    onkeydown={(e) => {
                      if (e.key === 'Enter' && currentTab?.isTableViewer) appState.applyTableFilter();
                    }}
                    placeholder={pill.op === 'LIKE' || pill.op === 'NOT LIKE' ? 'substring or %' : (pill.op === 'IN' || pill.op === 'NOT IN' ? '1, 2, 3' : 'val')}
                    class="bg-transparent text-slate-100 font-mono text-[11px] outline-none px-1 py-0.5 w-20 focus:w-36 transition-all border-b border-transparent focus:border-sky-400"
                  />
                {/if}

                <!-- Remove Pill Button -->
                <button
                  type="button"
                  onclick={() => appState.removeFilterPill(idx)}
                  class="text-slate-500 hover:text-rose-400 p-0.5 transition"
                  title="Remove condition"
                >
                  <Icon name="x" class="w-3 h-3" />
                </button>
              </div>
            {/each}

            <!-- + AND Button -->
            <button
              type="button"
              onclick={() => appState.addFilterPill()}
              class="text-xs text-slate-400 hover:text-sky-300 hover:bg-slate-800 px-2 py-0.5 rounded border border-dashed border-slate-700 transition flex items-center space-x-1"
              title="Add condition (+ AND)"
            >
              <Icon name="plus" class="w-3 h-3 text-sky-400" />
              <span>AND</span>
            </button>

            {#if currentTab?.isTableViewer && currentTab.filterPills && currentTab.filterPills.length > 0}
              <button
                type="button"
                onclick={() => appState.applyTableFilter()}
                class="px-2 py-0.5 rounded bg-sky-600 hover:bg-sky-500 text-white font-semibold text-[11px] transition"
              >
                Apply
              </button>
            {/if}
          </div>
        {/if}

        <!-- Active Filter Indicator Badge -->
        {#if currentTab?.isTableViewer ? currentTab.appliedFilter : hasActiveFilter}
          <span class="px-2 py-0.5 rounded text-[10px] font-medium bg-emerald-950 text-emerald-300 border border-emerald-700 flex items-center space-x-1" title="Active filter applied">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
            <span>Filtered</span>
          </span>
        {/if}
      </div>

      <!-- Right side: Clear Filter & Open in SQL Tab -->
      <div class="flex items-center space-x-2 shrink-0">
        {#if hasFilterEntries}
          <button
            type="button"
            onclick={() => appState.clearTableFilter()}
            class="text-[11px] text-slate-400 hover:text-rose-400 transition underline"
            title="Clear all filters"
          >
            Clear Filter
          </button>
        {/if}

        {#if currentTab?.isTableViewer}
          <button
            type="button"
            onclick={() => appState.handoffTableQueryToEditor()}
            class="h-6 px-2 rounded bg-[#1e293b] hover:bg-slate-700 text-sky-400 hover:text-sky-300 border border-[#334155] flex items-center space-x-1 text-[11px] transition font-mono"
            title="Open filtered query in full SQL Editor"
          >
            <Icon name="file-code" class="w-3 h-3" />
            <span>SQL</span>
          </button>
        {/if}

        <button
          type="button"
          onclick={toggleFilterBar}
          class="h-6 w-6 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-400 hover:text-white border border-slate-700/80 hover:border-slate-600 flex items-center justify-center transition"
          title="Hide filter bar"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  {/if}

  <!-- Data Grid Virtual Table -->
  {#if isExecuting}
    <div class="flex-1 flex flex-col items-center justify-center p-8 space-y-3">
      <Icon name="refresh-cw" class="w-7 h-7 text-sky-400 animate-spin" />
      <span class="text-xs text-slate-300 font-mono">Executing query on {appState.currentDatabase}...</span>
    </div>
  {:else if result?.error}
    <div class="p-6 m-4 bg-rose-950/20 border border-rose-500/40 rounded-xl max-w-3xl shadow-xl overflow-y-auto space-y-3">
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center space-x-2 text-rose-400 font-bold text-sm">
          <Icon name="alert-triangle" class="w-4 h-4 text-rose-400" />
          <span>Execution Error</span>
        </div>
        <button
          type="button"
          onclick={() => appState.fixQueryWithAI(currentTab?.sql || '', result!.error || '')}
          class="px-3 py-1 rounded bg-purple-600 hover:bg-purple-500 text-white font-semibold text-xs flex items-center space-x-1.5 transition shadow"
        >
          <Icon name="sparkles" class="w-3.5 h-3.5" />
          <span>Fix with AI</span>
        </button>
      </div>
      <pre class="text-xs font-mono text-rose-200 bg-rose-950/50 p-3.5 rounded-lg border border-rose-900/60 overflow-x-auto whitespace-pre-wrap leading-relaxed">{result.error}</pre>
    </div>
  {:else if result && (!result.columns || result.columns.length === 0) && (result.type === 'exec' || result.type === 'command' || result.rows_affected !== undefined || result.affected_rows !== undefined || result.message || result.backup_file)}
    {@const affected = result.rows_affected ?? result.affected_rows ?? 0}
    {@const execMs = result.execution_ms ?? result.duration_ms}
    <div class="p-6 m-4 bg-emerald-950/20 border border-emerald-500/40 rounded-xl max-w-2xl shadow-xl space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center space-x-2 text-emerald-300 font-bold text-sm">
          <Icon name="check-circle" class="w-4 h-4 text-emerald-400" />
          <span>Command Executed Successfully</span>
        </div>
        {#if execMs !== undefined}
          <span class="px-2 py-0.5 rounded bg-slate-900 border border-slate-800 font-mono text-[11px] text-slate-300">
            {execMs} ms
          </span>
        {/if}
      </div>
      <div class="flex flex-wrap items-center gap-2 text-xs font-mono">
        <span class="px-2.5 py-1 rounded bg-emerald-950 border border-emerald-800 text-emerald-300 font-bold">
          {affected} row(s) affected
        </span>
        {#if result.message}
          <span class="text-slate-300">{result.message}</span>
        {/if}
      </div>
      {#if result.backup_file}
        <div class="p-3 rounded-lg bg-[#090d16] border border-sky-800/60 text-xs space-y-1">
          <div class="font-semibold text-sky-300 flex items-center space-x-1.5">
            <Icon name="shield-check" class="w-3.5 h-3.5 text-sky-400" />
            <span>Pre-Mutation Snapshot Saved</span>
          </div>
          <div class="font-mono text-[11px] text-slate-300 break-all">{result.backup_file}</div>
          {#if result.backup_data_included !== undefined}
            <div class="text-[10.5px] text-slate-400">
              Data included: {result.backup_data_included ? 'Yes' : 'Schema / Metadata only'}
            </div>
          {/if}
        </div>
      {/if}
      {#if result.backup_warning}
        <div class="p-2.5 rounded-lg bg-amber-950/40 border border-amber-800/60 text-amber-300 text-xs">
          Backup Warning: {result.backup_warning}
        </div>
      {/if}
    </div>
  {:else if result?.columns && result.columns.length > 0}
    <div
      bind:this={containerRef}
      onscroll={onScroll}
      onkeydown={handleGridKeydown}
      tabindex="0"
      class="flex-1 overflow-auto relative border-t border-[#1e293b] outline-none"
    >
      <table id="results-table" class="border-collapse text-left font-mono text-xs w-auto">
        <!-- Sticky Header -->
        <thead class="sticky top-0 z-20 bg-[#0f172a] border-b border-[#334155] shadow-sm select-none">
          <tr>
            <th class="w-16 px-2 py-2 text-center text-[10px] text-slate-500 bg-[#090d16] border-r border-[#1e293b] font-medium sticky left-0 z-30">
              <div class="flex items-center justify-center space-x-1.5">
                <input
                  type="checkbox"
                  checked={isAllSelected}
                  onchange={toggleSelectAll}
                  class="rounded border-slate-700 bg-slate-900 text-sky-500 focus:ring-0"
                  title="Select all rows"
                />
                <span>#</span>
              </div>
            </th>
            {#each visibleColumns as col}
              {@const colName = getColName(col)}
              {@const colType = resolveColType(col)}
              {@const isPK = isColPK(col)}
              <th
                onclick={() => handleHeaderSort(colName)}
                class="px-3 py-1.5 font-semibold text-slate-200 border-r border-[#1e293b] whitespace-nowrap bg-[#0f172a] text-[11px] tracking-tight hover:bg-slate-800 cursor-pointer transition-colors"
              >
                <div class="flex items-center space-x-1.5">
                  {#if isPK}
                    <Icon name="key" class="w-3 h-3 text-amber-400 shrink-0" />
                  {/if}
                  <span class="font-mono text-slate-100">{colName}</span>
                  {#if colType}
                    <span class="text-[9px] font-mono text-slate-400 bg-slate-800 px-1 py-0.2 rounded border border-slate-700 uppercase">
                      {colType}
                    </span>
                  {/if}
                  {#if effectiveSortCol === colName}
                    <Icon name={effectiveSortDir === 'ASC' ? 'chevron-up' : 'chevron-down'} class="w-3 h-3 text-sky-400 shrink-0 ml-1" />
                  {/if}
                </div>
              </th>
            {/each}
          </tr>
        </thead>

        <!-- Virtual Rows Body -->
        <tbody>
          <!-- In-Place Draft Rows -->
          {#if currentTab?.stagedDraftRows && currentTab.stagedDraftRows.length > 0}
            {#each currentTab.stagedDraftRows as draft, dIdx}
              <tr class="bg-sky-950/40 border-b border-sky-800/60 text-sky-200 h-7 font-mono text-xs">
                <td class="w-16 px-1.5 py-1 text-center bg-sky-950/80 border-r border-sky-800/60 sticky left-0 z-10 flex items-center justify-center space-x-1">
                  <button
                    type="button"
                    onclick={() => saveDraftRow(dIdx)}
                    class="text-emerald-400 hover:text-emerald-200 p-0.5 rounded hover:bg-emerald-950/50 inline-flex items-center justify-center transition"
                    title="Save draft row"
                    aria-label="Save draft row"
                  >
                    <Icon name="check" class="w-3.5 h-3.5" />
                  </button>
                  <button
                    type="button"
                    onclick={() => removeDraftRow(dIdx)}
                    class="text-rose-400 hover:text-rose-200 p-0.5 rounded hover:bg-rose-950/50 inline-flex items-center justify-center transition"
                    title="Discard draft row"
                    aria-label="Discard draft row"
                  >
                    <Icon name="x" class="w-3.5 h-3.5" />
                  </button>
                </td>
                {#each visibleColumns as col}
                  {@const colName = getColName(col)}
                  {@const colType = resolveColType(col)}
                  {@const nullable = isColNullable(col)}
                  {@const enumVals = resolveEnumValues(col, tableMeta)}
                  {@const kind = classifyColumnType(colType, enumVals)}
                  <td class="px-2 py-0.5 border-r border-sky-800/40">
                    {#if kind.isBool}
                      <select
                        bind:value={draft[colName]}
                        class="w-full bg-[#090d16] text-sky-100 outline-none text-xs font-mono rounded px-1 py-0.5 border border-sky-800/60"
                      >
                        <option value="">(default)</option>
                        <option value="1">TRUE (1)</option>
                        <option value="0">FALSE (0)</option>
                        {#if nullable}
                          <option value="__NULL__">NULL</option>
                        {/if}
                      </select>
                    {:else if kind.isEnum}
                      <select
                        bind:value={draft[colName]}
                        class="w-full bg-[#090d16] text-sky-100 outline-none text-xs font-mono rounded px-1 py-0.5 border border-sky-800/60"
                      >
                        <option value="">(default)</option>
                        {#each enumVals as ev}
                          <option value={ev}>{ev}</option>
                        {/each}
                        {#if nullable}
                          <option value="__NULL__">NULL</option>
                        {/if}
                      </select>
                    {:else if kind.isDate}
                      <input
                        type="date"
                        bind:value={draft[colName]}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') saveDraftRow(dIdx);
                        }}
                        class="w-full bg-transparent text-sky-100 outline-none text-xs font-mono [color-scheme:dark]"
                      />
                    {:else if kind.isTime}
                      <input
                        type="time"
                        step="1"
                        bind:value={draft[colName]}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') saveDraftRow(dIdx);
                        }}
                        class="w-full bg-transparent text-sky-100 outline-none text-xs font-mono [color-scheme:dark]"
                      />
                    {:else}
                      <input
                        type="text"
                        inputmode={kind.isInt ? 'numeric' : kind.isDecimal ? 'decimal' : undefined}
                        maxlength={kind.maxLength ?? undefined}
                        bind:value={draft[colName]}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') saveDraftRow(dIdx);
                        }}
                        placeholder="draft..."
                        class="w-full bg-transparent text-sky-100 placeholder-sky-600/60 outline-none text-xs font-mono"
                      />
                    {/if}
                  </td>
                {/each}
              </tr>
            {/each}
          {/if}

          {#if topSpacerHeight > 0}
            <tr style="height: {topSpacerHeight}px; pointer-events: none;">
              <td colspan={totalCols} style="padding:0; border:0; height:{topSpacerHeight}px;"></td>
            </tr>
          {/if}

          {#if processedRows.length === 0}
            <tr>
              <td colspan={totalCols} class="p-12 text-center text-slate-500 italic text-xs">
                Zero records returned.
              </td>
            </tr>
          {:else}
            {#each visibleRows as rowItem (rowItem.origIdx)}
              {@const isRowSelected = selectedRowIndex === rowItem.origIdx}
              {@const isChecked = selectedRowIndices.includes(rowItem.origIdx)}
              <tr
                onclick={() => (selectedRowIndex = rowItem.origIdx)}
                class="border-b border-[#1e293b]/60 transition-colors h-7 {
                  isChecked ? 'bg-sky-500/20' : isRowSelected ? 'bg-sky-500/15' : rowItem.displayIdx % 2 === 1 ? 'bg-slate-900/20' : 'bg-transparent'
                } hover:bg-sky-500/10"
              >
                <!-- Row Gutter -->
                <td
                  ondblclick={() => {
                    appState.recordInspectorRowIndex = rowItem.origIdx;
                    appState.isRecordInspectorOpen = true;
                  }}
                  class="w-16 px-2 py-1 text-center text-[10px] text-slate-500 bg-[#090d16] border-r border-[#1e293b] select-none font-mono sticky left-0 z-10 cursor-pointer hover:text-sky-400"
                  title="Double-click to open Record Inspector"
                >
                  <div class="flex items-center justify-center space-x-1.5">
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onclick={(e) => e.stopPropagation()}
                      onchange={() => toggleRowSelection(rowItem.origIdx)}
                      class="rounded border-slate-700 bg-slate-900 text-sky-500 focus:ring-0"
                    />
                    <span>{rowItem.displayIdx + 1}</span>
                  </div>
                </td>

                <!-- Cells -->
                {#each visibleColumnEntries as { col, origIdx: cIdx }, visColIdx (cIdx)}
                  {@const dispRowIdx = rowItem.displayIdx}
                  {@const colName = getColName(col)}
                  {@const colType = resolveColType(col)}
                  {@const nullable = isColNullable(col)}
                  {@const cellVal = cIdx >= 0 ? rowItem.data[cIdx] : undefined}
                  {@const formatted = formatCellValue(cellVal)}
                  {@const isStaged = effectiveTable && cIdx >= 0 ? isCellStaged(effectiveTable, rowItem.data, rowItem.origIdx, cIdx) : false}
                  {@const isFlashed = Boolean(flashedCells[`${rowItem.origIdx}:${cIdx}`])}
                  {@const isCellSelected = selectedCell?.rIdx === rowItem.origIdx && selectedCell?.cIdx === cIdx}
                  {@const inRange = isCellInRange(dispRowIdx, visColIdx)}
                  {@const isEditingThis = editingCell && editingCell.rIdx === rowItem.origIdx && editingCell.cIdx === cIdx}
                  {@const fk = fkMap.get(colName)}
                  {@const enumVals = resolveEnumValues(col, tableMeta)}
                  {@const kind = classifyColumnType(colType, enumVals)}
                  {@const isBool = kind.isBool}
                  {@const isDateLike = kind.isDateTime}
                  {@const fkListId = fk ? `fk-datalist-${rowItem.origIdx}-${cIdx}` : undefined}
                  {@const fkOpts = fk ? fkDatalistCache[`${fk.referenced_table}.${fk.referenced_column}`] || [] : []}
                  {@const isJsonLike = formatted.type === 'json' || kind.isJson}
                  {@const showUuidPreset = kind.isUuid || (kind.isString && (!kind.maxLength || kind.maxLength >= 36))}
                  {@const hasAnyPreset = nullable || kind.isString || kind.isDateTime || kind.isDate || showUuidPreset}

                  <td
                    onclick={(e) => {
                      selectedCell = { rIdx: rowItem.origIdx, cIdx };
                      selectedRowIndex = rowItem.origIdx;
                      if (e.shiftKey && rangeAnchor) {
                        rangeEnd = { dispRow: dispRowIdx, visCol: visColIdx };
                      } else {
                        rangeAnchor = { dispRow: dispRowIdx, visCol: visColIdx };
                        rangeEnd = { dispRow: dispRowIdx, visCol: visColIdx };
                      }
                    }}
                    ondblclick={() => startCellEdit(rowItem.origIdx, cIdx, cellVal)}
                    oncontextmenu={(e) => handleCellContextMenu(e, rowItem.origIdx, cIdx, cellVal)}
                    class="px-3 py-1 border-r border-[#1e293b]/40 whitespace-nowrap max-w-sm {isEditingThis ? 'overflow-visible z-30' : 'truncate'} {getCellClass(formatted.type)} cursor-cell hover:bg-sky-900/30 transition-colors relative {
                      isStaged ? 'bg-amber-950/40 text-amber-200 font-semibold ring-1 ring-amber-500' : isFlashed ? 'bg-emerald-950/60 ring-1 ring-emerald-400' : isCellSelected ? 'ring-1 ring-inset ring-sky-400/80 bg-sky-900/20' : inRange ? 'bg-sky-900/25' : ''
                    }"
                    title="{formatted.display}{canEdit ? ' (Double-click to edit, right-click for filters)' : ''}"
                  >
                    {#if isEditingThis}
                      <div class="relative flex items-center space-x-1 min-w-[140px]">
                        {#if isBool}
                          {@const boolSelectVal = cellVal === null || cellVal === undefined ? (nullable ? '__NULL__' : '') : (cellVal === true || cellVal === 1 || String(cellVal).toLowerCase() === 'true' || String(cellVal) === '1' ? '1' : '0')}
                          <select
                            use:autoFocus
                            value={boolSelectVal}
                            onchange={(e) => {
                              const v = (e.currentTarget as HTMLSelectElement).value;
                              if (v === '__NULL__' && !nullable) return;
                              commitDirectCellValue(rowItem.origIdx, cIdx, v === '__NULL__' ? null : v === '1');
                            }}
                            onkeydown={(e) => {
                              if (e.key === 'Escape') editingCell = null;
                            }}
                            onblur={() => (editingCell = null)}
                            class="w-full bg-[#0f172a] text-slate-100 border border-sky-400 px-1 py-0 text-xs font-mono outline-none"
                          >
                            {#if !nullable && boolSelectVal === ''}
                              <option value="" disabled>Select...</option>
                            {/if}
                            <option value="1">TRUE (1)</option>
                            <option value="0">FALSE (0)</option>
                            {#if nullable}
                              <option value="__NULL__">NULL</option>
                            {/if}
                          </select>
                        {:else if kind.isEnum}
                          {@const enumSelectVal = cellVal === null || cellVal === undefined ? (nullable ? '__NULL__' : '') : String(cellVal)}
                          <select
                            use:autoFocus
                            value={enumSelectVal}
                            onchange={(e) => {
                              const v = (e.currentTarget as HTMLSelectElement).value;
                              if (v === '__NULL__' && !nullable) return;
                              commitDirectCellValue(rowItem.origIdx, cIdx, v === '__NULL__' ? null : v);
                            }}
                            onkeydown={(e) => {
                              if (e.key === 'Escape') editingCell = null;
                            }}
                            onblur={() => (editingCell = null)}
                            class="w-full bg-[#0f172a] text-slate-100 border border-sky-400 px-1 py-0 text-xs font-mono outline-none"
                          >
                            {#if !nullable && (!enumSelectVal || !enumVals.includes(enumSelectVal))}
                              <option value="" disabled>Select...</option>
                            {/if}
                            {#each enumVals as ev}
                              <option value={ev}>{ev}</option>
                            {/each}
                            {#if nullable}
                              <option value="__NULL__">NULL</option>
                            {/if}
                          </select>
                        {:else if kind.isDate}
                          <input
                            id="grid-inline-editor"
                            use:autoFocus
                            type="date"
                            bind:value={editingCell!.tempValue}
                            onblur={() => commitCellEdit(rowItem.origIdx, cIdx)}
                            onkeydown={(e) => {
                              if (e.key === 'Tab') {
                                e.preventDefault();
                                commitCellEdit(rowItem.origIdx, cIdx);
                                advanceActiveCell(0, e.shiftKey ? -1 : 1);
                                return;
                              }
                              if (e.key === 'Enter') commitCellEdit(rowItem.origIdx, cIdx);
                              if (e.key === 'Escape') editingCell = null;
                            }}
                            class="w-full bg-[#0f172a] text-slate-100 border border-sky-400 px-1 py-0 text-xs font-mono outline-none [color-scheme:dark]"
                          />
                        {:else if kind.isTime}
                          <input
                            id="grid-inline-editor"
                            use:autoFocus
                            type="time"
                            step="1"
                            bind:value={editingCell!.tempValue}
                            onblur={() => commitCellEdit(rowItem.origIdx, cIdx)}
                            onkeydown={(e) => {
                              if (e.key === 'Tab') {
                                e.preventDefault();
                                commitCellEdit(rowItem.origIdx, cIdx);
                                advanceActiveCell(0, e.shiftKey ? -1 : 1);
                                return;
                              }
                              if (e.key === 'Enter') commitCellEdit(rowItem.origIdx, cIdx);
                              if (e.key === 'Escape') editingCell = null;
                            }}
                            class="w-full bg-[#0f172a] text-slate-100 border border-sky-400 px-1 py-0 text-xs font-mono outline-none [color-scheme:dark]"
                          />
                        {:else}
                          <input
                            id="grid-inline-editor"
                            use:autoFocus
                            type="text"
                            inputmode={kind.isInt ? 'numeric' : kind.isDecimal ? 'decimal' : undefined}
                            maxlength={kind.maxLength ?? undefined}
                            list={fkListId}
                            bind:value={editingCell!.tempValue}
                            onblur={() => commitCellEdit(rowItem.origIdx, cIdx)}
                            onkeydown={(e) => {
                              if (isDateLike && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
                                e.preventDefault();
                                stepActiveDatePart(e.key === 'ArrowUp' ? 1 : -1);
                                return;
                              }
                              if (e.key === 'Tab') {
                                e.preventDefault();
                                commitCellEdit(rowItem.origIdx, cIdx);
                                advanceActiveCell(0, e.shiftKey ? -1 : 1);
                                return;
                              }
                              if (e.key === 'Enter') commitCellEdit(rowItem.origIdx, cIdx);
                              if (e.key === 'Escape') editingCell = null;
                            }}
                            class="w-full bg-[#0f172a] text-slate-100 border border-sky-400 px-1 py-0 text-xs font-mono outline-none"
                          />
                          {#if fkListId && fkOpts.length > 0}
                            <datalist id={fkListId}>
                              {#each fkOpts as opt}
                                <option value={opt}></option>
                              {/each}
                            </datalist>
                          {/if}
                          {#if isDateLike}
                            <div class="flex items-center space-x-0.5 shrink-0">
                              <button
                                type="button"
                                onmousedown={(e) => e.preventDefault()}
                                onclick={() => stepActiveDatePart(1)}
                                class="px-1 bg-slate-800 hover:bg-slate-700 text-sky-300 text-[9px] rounded border border-slate-700"
                                title="Increment segment (↑)"
                              >▲</button>
                              <button
                                type="button"
                                onmousedown={(e) => e.preventDefault()}
                                onclick={() => stepActiveDatePart(-1)}
                                class="px-1 bg-slate-800 hover:bg-slate-700 text-sky-300 text-[9px] rounded border border-slate-700"
                                title="Decrement segment (↓)"
                              >▼</button>
                            </div>
                          {/if}
                        {/if}
                      </div>
                      <!-- Inline Editor Presets Strip -->
                      {#if hasAnyPreset}
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div
                          onmousedown={(e) => e.preventDefault()}
                          class="absolute left-0 top-full mt-0.5 z-40 flex items-center space-x-1 bg-[#0b0f19] border border-slate-700 rounded px-1.5 py-0.5 shadow-xl text-[9.5px] font-mono"
                        >
                          {#if nullable}
                            <button type="button" onclick={() => applyEditPreset('NULL', rowItem.origIdx, cIdx)} class="px-1 py-0.2 rounded bg-rose-950/70 hover:bg-rose-900 text-rose-300 border border-rose-800">NULL</button>
                          {/if}
                          {#if kind.isString}
                            <button type="button" onclick={() => applyEditPreset('EMPTY', rowItem.origIdx, cIdx)} class="px-1 py-0.2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700">''</button>
                          {/if}
                          {#if kind.isDateTime}
                            <button type="button" onclick={() => applyEditPreset('NOW', rowItem.origIdx, cIdx)} class="px-1 py-0.2 rounded bg-sky-950/70 hover:bg-sky-900 text-sky-300 border border-sky-800">NOW</button>
                          {/if}
                          {#if kind.isDate || kind.isDateTime}
                            <button type="button" onclick={() => applyEditPreset('DATE', rowItem.origIdx, cIdx)} class="px-1 py-0.2 rounded bg-sky-950/70 hover:bg-sky-900 text-sky-300 border border-sky-800">DATE</button>
                          {/if}
                          {#if showUuidPreset}
                            <button type="button" onclick={() => applyEditPreset('UUID', rowItem.origIdx, cIdx)} class="px-1 py-0.2 rounded bg-purple-950/70 hover:bg-purple-900 text-purple-300 border border-purple-800">UUID</button>
                          {/if}
                        </div>
                      {/if}
                    {:else}
                      <div class="flex items-center justify-between group/cell">
                        <span class="truncate block">{formatted.display}</span>
                        <div class="flex items-center space-x-0.5 shrink-0 ml-1">
                          {#if fk && cellVal !== null && cellVal !== undefined}
                            <button
                              type="button"
                              onclick={(e) => {
                                e.stopPropagation();
                                appState.openFkPeek(e, effectiveTable, colName, fk.referenced_table, fk.referenced_column, cellVal, effectiveDatabase);
                              }}
                              class="opacity-0 group-hover/cell:opacity-100 hover:text-sky-300 text-sky-400 p-0.5 rounded hover:bg-slate-800 inline-flex items-center"
                              title="Peek {fk.referenced_table}.{fk.referenced_column}"
                              aria-label="Peek {fk.referenced_table}.{fk.referenced_column}"
                            >
                              <Icon name="link" class="w-3 h-3" />
                            </button>
                          {/if}
                          {#if isJsonLike}
                            <button
                              type="button"
                              onclick={(e) => {
                                e.stopPropagation();
                                appState.openJsonCellModal(effectiveTable || undefined, rowItem.origIdx, cIdx, colName, cellVal, !canEdit);
                              }}
                              class="opacity-0 group-hover/cell:opacity-100 hover:text-purple-300 text-purple-400 px-1 rounded hover:bg-slate-800 text-[9px] font-mono"
                              title="Inspect / Edit JSON"
                            >
                              {'{ }'}
                            </button>
                          {/if}
                          {#if canEdit}
                            <button
                              type="button"
                              onclick={(e) => {
                                e.stopPropagation();
                                startCellEdit(rowItem.origIdx, cIdx, cellVal);
                              }}
                              class="opacity-0 group-hover/cell:opacity-100 hover:text-sky-300 text-slate-400 p-0.5 rounded hover:bg-slate-800 inline-flex items-center"
                              title="Edit Cell"
                            >
                              <Icon name="edit" class="w-2.5 h-2.5" />
                            </button>
                          {/if}
                        </div>
                      </div>
                    {/if}
                  </td>
                {/each}
              </tr>
            {/each}
          {/if}

          {#if bottomSpacerHeight > 0}
            <tr style="height: {bottomSpacerHeight}px; pointer-events: none;">
              <td colspan={totalCols} style="padding:0; border:0; height:{bottomSpacerHeight}px;"></td>
            </tr>
          {/if}
        </tbody>
      </table>
    </div>

    <!-- Cell Details Drawer (expandable) -->
    {#if showDetailDrawer && selectedCell && result?.rows[selectedCell.rIdx]}
      {@const val = result.rows[selectedCell.rIdx][selectedCell.cIdx]}
      {@const colObj = result.columns[selectedCell.cIdx]}
      {@const colName = allColumns[selectedCell.cIdx]}
      {@const colType = resolveColType(colObj)}
      {@const safeValStr = val === null ? 'NULL' : typeof val === 'object' ? JSON.stringify(val, null, 2) : (() => {
        const s = String(val ?? '');
        if (detailPrettyJson) {
          try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
        }
        return s;
      })()}
      <div class="h-36 bg-[#0b0f19] border-t border-[#334155] flex flex-col p-2.5 z-20 shrink-0">
        <div class="flex items-center justify-between text-[11px] text-slate-400 mb-1.5 select-none">
          <div class="flex items-center space-x-2">
            <span class="font-bold text-slate-200">
              Cell: <span class="text-sky-400 font-mono">{colName}</span>
            </span>
            {#if colType}
              <span class="px-1.5 py-0.2 rounded bg-slate-900 border border-slate-800 font-mono text-[9.5px] uppercase text-slate-400">
                {colType}
              </span>
            {/if}
            <span class="text-[10px] font-mono text-slate-500">Row #{selectedCell.rIdx + 1}</span>
          </div>
          <div class="flex items-center space-x-2">
            <button
              type="button"
              onclick={() => (detailPrettyJson = !detailPrettyJson)}
              class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-[10px] {detailPrettyJson ? 'text-purple-300 border-purple-700' : 'text-slate-300'}"
            >
              {detailPrettyJson ? 'Raw' : 'Format JSON'}
            </button>
            <button
              type="button"
              onclick={() => {
                const rawCopy = val === null || val === undefined ? '' : typeof val === 'object' ? JSON.stringify(val, null, 2) : String(val);
                copyText(rawCopy);
                appState.showToast('Copied cell value', 'info');
              }}
              class="hover:text-white flex items-center space-x-1"
            >
              <Icon name="copy" class="w-3 h-3" />
              <span>Copy</span>
            </button>
          </div>
        </div>
        <textarea
          readonly
          class="flex-1 bg-[#090d16] border border-[#1e293b] rounded p-2 text-xs font-mono text-slate-200 resize-none outline-none"
          value={safeValStr}
        ></textarea>
      </div>
    {/if}

    <!-- Bottom Status & Pagination Bar -->
    <div class="h-8 bg-[#0b0f19] border-t border-[#334155] px-4 flex items-center justify-between text-xs text-slate-400 font-mono select-none shrink-0">
      <div id="grid-bottom-stats" class="flex items-center space-x-3">
        <span class="flex items-center space-x-1.5">
          <Icon name="table" class="w-3.5 h-3.5 text-sky-400" />
          {#if currentTab?.isTableViewer && tableEffectiveTotal !== undefined && tableEffectiveTotal !== null}
            {@const curPage = currentTab.currentPage || 1}
            {@const pageSize = currentTab.pageSize || 25}
            <span class="font-bold text-slate-200">
              {totalRows > 0 ? `${((curPage - 1) * pageSize + 1).toLocaleString()}–${((curPage - 1) * pageSize + totalRows).toLocaleString()}` : '0'}
            </span>
            <span class="text-slate-500">
              of {tableTotalIsExact ? '' : '~'}{tableEffectiveTotal.toLocaleString()} records
            </span>
          {:else}
            <span class="font-bold text-slate-200">{totalRows.toLocaleString()}</span>
            <span class="text-slate-500">records</span>
            {#if currentTab?.manualTotalRows !== undefined && currentTab?.manualTotalRows !== null}
              <span class="text-slate-600">•</span>
              <span class="text-slate-400">Total:</span>
              <span class="font-bold text-emerald-400">{currentTab.manualTotalRows.toLocaleString()}</span>
              <span class="text-slate-500">records</span>
            {:else if !currentTab?.isTableViewer && (totalRows > 0 || currentTab?.hasMoreStream)}
              <button
                onclick={() => appState.countQueryTotal(currentTab)}
                disabled={currentTab?.isCountingTotal || isExecuting}
                class="ml-1.5 px-2 py-0.5 rounded bg-slate-900 hover:bg-slate-800 disabled:opacity-50 text-[10px] text-sky-400 hover:text-sky-300 border border-slate-700/80 transition flex items-center space-x-1"
                title="Calculate exact total rows for this query via SELECT COUNT(*)"
              >
                <Icon name={currentTab?.isCountingTotal ? 'refresh-cw' : 'hash'} class="w-3 h-3 {currentTab?.isCountingTotal ? 'animate-spin text-sky-300' : ''}" />
                <span>{currentTab?.isCountingTotal ? 'Counting...' : 'Count Total'}</span>
              </button>
            {/if}
          {/if}
        </span>
        {#if result.duration_ms !== undefined}
          <span class="text-slate-600">•</span>
          <span class="flex items-center space-x-1">
            <Icon name="clock" class="w-3 h-3 text-slate-500" />
            <span class="text-slate-300">{result.duration_ms} ms</span>
          </span>
        {/if}
      </div>

      <!-- Pagination Controls for Table Viewer -->
      {#if currentTab?.isTableViewer}
        {@const pageSize = currentTab.pageSize || 25}
        {@const curPage = currentTab.currentPage || 1}
        {@const totalPages = typeof tableEffectiveTotal === 'number' && tableEffectiveTotal >= 0 ? Math.max(1, Math.ceil(tableEffectiveTotal / pageSize)) : null}
        <div id="grid-pagination-bar" class="flex items-center space-x-2 text-[11px]">
          <span class="text-slate-500">Page Size:</span>
          <select
            id="grid-page-size-select"
            value={pageSize}
            onchange={(e) => {
              if (currentTab) {
                currentTab.pageSize = Number((e.currentTarget as HTMLSelectElement).value);
                currentTab.currentPage = 1;
                appState.loadTableData(currentTab);
              }
            }}
            class="bg-[#090d16] border border-[#334155] rounded px-1.5 py-0.5 text-slate-200 focus:outline-none"
          >
            <option value={25}>25</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
            <option value={250}>250</option>
            <option value={500}>500</option>
            <option value={1000}>1000</option>
          </select>

          <button
            id="btn-page-first"
            onclick={() => {
              if (currentTab && curPage > 1) {
                currentTab.currentPage = 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={curPage <= 1}
            class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
            title="First Page"
          >
            <Icon name="chevrons-left" class="w-3.5 h-3.5" />
          </button>
          <button
            id="btn-page-prev"
            onclick={() => {
              if (currentTab && curPage > 1) {
                currentTab.currentPage = curPage - 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={curPage <= 1}
            class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
            title="Previous Page"
          >
            <Icon name="chevron-left" class="w-3.5 h-3.5" />
          </button>
          <span class="flex items-center space-x-1 px-1 text-slate-300">
            <span>Page</span>
            <!-- Editable page jump: parity with the legacy pager, clamped to the valid range. -->
            <input
              id="grid-page-input"
              type="number"
              min="1"
              max={tableTotalIsExact ? (totalPages ?? undefined) : undefined}
              value={curPage}
              onkeydown={(e) => {
                if (e.key === 'Escape') {
                  (e.currentTarget as HTMLInputElement).value = String(curPage);
                  (e.currentTarget as HTMLInputElement).blur();
                }
              }}
              onchange={(e) => {
                const target = Number((e.currentTarget as HTMLInputElement).value);
                if (!currentTab || !Number.isFinite(target)) return;
                const clamped = Math.max(1, tableTotalIsExact && totalPages !== null ? Math.min(totalPages, Math.floor(target)) : Math.floor(target));
                if (clamped === curPage) return;
                currentTab.currentPage = clamped;
                appState.loadTableData(currentTab);
              }}
              class="w-14 bg-[#090d16] border border-[#334155] rounded px-1 py-0.5 text-right text-slate-200 font-mono focus:outline-none focus:border-sky-500 [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
              aria-label="Jump to page"
            />
            {#if totalPages}
              <span>of <span id="grid-total-pages">{tableTotalIsExact ? '' : '~'}{totalPages.toLocaleString()}</span></span>
            {:else if totalRows >= pageSize}
              <span class="text-slate-400">({curPage}+)</span>
            {/if}
          </span>
          <button
            id="btn-page-next"
            onclick={() => {
              if (currentTab) {
                currentTab.currentPage = curPage + 1;
                appState.loadTableData(currentTab);
              }
            }}
            disabled={totalRows < pageSize || (tableTotalIsExact && totalPages !== null && curPage >= totalPages)}
            class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
            title="Next Page"
          >
            <Icon name="chevron-right" class="w-3.5 h-3.5" />
          </button>
          {#if totalPages !== null}
            <button
              id="btn-page-last"
              onclick={() => {
                if (currentTab && curPage < totalPages) {
                  currentTab.currentPage = totalPages;
                  appState.loadTableData(currentTab);
                }
              }}
              disabled={tableTotalIsExact ? curPage >= totalPages : totalRows < pageSize}
              class="px-2 py-1 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 flex items-center justify-center transition"
              title="Last Page"
            >
              <Icon name="chevrons-right" class="w-3.5 h-3.5" />
            </button>
          {/if}
        </div>
      {:else if result && result.rows && (result.rows.length >= (currentTab?.queryLimit || 1000) || (currentTab?.streamBatchOffset ?? 0) > 0 || currentTab?.hasMoreStream)}
        {@const streamOff = currentTab?.streamBatchOffset || 0}
        {@const batchLimit = currentTab?.queryLimit || 1000}
        {@const batchLabel = batchLimit >= 1000 ? `${batchLimit / 1000}k` : `${batchLimit}`}
        <!-- Query Stream Dynamic Batch Pager (ADR 0058) -->
        <div class="flex items-center space-x-2 text-[11px]">
          <span class="px-2 py-0.5 rounded bg-sky-950/80 border border-sky-800 text-sky-300 font-mono text-[10px] font-bold">
            Batch #{Math.floor(streamOff / batchLimit) + 1} (Rows {(streamOff + 1).toLocaleString()}–{(streamOff + totalRows).toLocaleString()})
          </span>
          <button
            onclick={() => appState.pageStreamBatch('prev')}
            disabled={(currentTab?.streamBatchOffset || 0) <= 0 || isExecuting}
            class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 transition"
            title={`Load previous ${batchLabel} records`}
          >
            Prev {batchLabel}
          </button>
          <button
            onclick={() => appState.pageStreamBatch('next')}
            disabled={(!currentTab?.hasMoreStream && (result.rows?.length || 0) < batchLimit) || isExecuting}
            class="px-2 py-0.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-30 border border-[#334155] text-slate-200 transition"
            title={`Load next ${batchLabel} records`}
          >
            Next {batchLabel}
          </button>
        </div>
      {:else}
        <div class="text-[11px] text-slate-500">
          Double-click to edit cell • Right-click for quick filters
        </div>
      {/if}
    </div>
  {:else}
    <div class="flex-1 flex flex-col items-center justify-center p-8 space-y-3 text-slate-500 select-none">
      <Icon name="terminal" class="w-8 h-8 text-slate-600" />
      <span class="text-xs">No query results yet. Click a table in the sidebar or press Ctrl+Enter to run SQL.</span>
    </div>
  {/if}

  <!-- Cell Context Menu -->
  {#if contextMenu && contextMenu.open}
    <div
      id="grid-context-menu"
      use:viewportFit={{ x: contextMenu.x, y: contextMenu.y }}
      style="top: {contextMenu.y}px; left: {contextMenu.x}px;"
      class="fixed z-50 w-60 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs font-sans animate-in fade-in duration-75 select-none"
    >
      <div class="px-3 py-1 text-[10px] font-bold text-slate-400 uppercase tracking-wider font-mono truncate">
        Filter: {contextMenu.colName}
      </div>

      <div id="grid-ctx-filter-parent" class="py-1">
        <div id="grid-ctx-filter-submenu">
          <div id="grid-ctx-filter-options" class="space-y-0.5">
            <button
              onclick={() => applyQuickFilter('=')}
              class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <span class="w-4 text-center flex-shrink-0 font-mono font-bold text-sky-400">=</span>
              <span class="truncate">Filter = '{String(contextMenu.cellValue ?? 'NULL')}'</span>
            </button>
            <button
              onclick={() => applyQuickFilter('!=')}
              class="w-full text-left px-3 py-1 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
            >
              <span class="w-4 text-center flex-shrink-0 font-mono font-bold text-amber-400">!=</span>
              <span class="truncate">Filter != '{String(contextMenu.cellValue ?? 'NULL')}'</span>
            </button>
            {#if contextMenu.cellValue !== null && contextMenu.cellValue !== undefined}
              <div class="grid grid-cols-6 gap-1 px-3 py-1">
                <button
                  onclick={() => applyQuickFilter('<')}
                  class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
                  title="Less than"
                >&lt;</button>
                <button
                  onclick={() => applyQuickFilter('<=')}
                  class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
                  title="Less or equal"
                >&le;</button>
                <button
                  onclick={() => applyQuickFilter('>')}
                  class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
                  title="Greater than"
                >&gt;</button>
                <button
                  onclick={() => applyQuickFilter('>=')}
                  class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-mono text-[10px] font-bold text-center"
                  title="Greater or equal"
                >&ge;</button>
                <button
                  onclick={() => applyQuickFilter('LIKE')}
                  class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-purple-300 font-mono text-[10px] font-bold text-center"
                  title="Filter LIKE '%{String(contextMenu.cellValue)}%'"
                >%</button>
                <button
                  onclick={() => applyQuickFilter('NOT LIKE')}
                  class="px-1 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-rose-300 font-mono text-[10px] font-bold text-center"
                  title="Filter NOT LIKE '%{String(contextMenu.cellValue)}%'"
                >!%</button>
              </div>
            {/if}
            <div class="px-3 py-1 flex items-center justify-between gap-1.5">
              <span class="text-[10px] text-slate-500">Null check</span>
              <div class="flex items-center gap-1 text-[10px] font-mono">
                <button
                  onclick={() => applyQuickFilter('IS NULL')}
                  class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-rose-300 font-medium"
                >IS NULL</button>
                <button
                  onclick={() => applyQuickFilter('IS NOT NULL')}
                  class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium"
                >NOT NULL</button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div id="grid-ctx-mutations-parent" class="py-1">
        <div id="grid-ctx-table-mutations">
          <div id="grid-ctx-insert-section">
            <div id="grid-ctx-insert-options">
              {#if canEdit}
                {@const ctxColDef = result?.columns?.[contextMenu.cIdx]}
                {@const ctxNullable = ctxColDef ? isColNullable(ctxColDef) : true}
                {@const ctxEnumVals = ctxColDef ? resolveEnumValues(ctxColDef, tableMeta) : []}
                {@const ctxKind = classifyColumnType(ctxColDef ? resolveColType(ctxColDef) : '', ctxEnumVals)}
                {@const ctxShowUuid = ctxKind.isUuid || (ctxKind.isString && (!ctxKind.maxLength || ctxKind.maxLength >= 36))}
                {@const ctxHasPresets = ctxNullable || ctxKind.isDateTime || ctxKind.isDate || ctxShowUuid}
                <button
                  onclick={() => {
                    const { rIdx, cIdx, cellValue } = contextMenu!;
                    closeContextMenu();
                    startCellEdit(rIdx, cIdx, cellValue);
                  }}
                  class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-sky-300 font-medium"
                >
                  <Icon name="edit" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
                  <span>Edit Cell (Inline)</span>
                </button>
                {#if ctxHasPresets}
                  <div class="px-3 py-1 border-y border-slate-800/60 flex items-center justify-between gap-1 text-[10px] font-mono">
                    <span class="text-slate-500">⚡ Insert:</span>
                    <div class="flex items-center gap-1">
                      {#if ctxNullable}
                        <button onclick={() => { const { rIdx, cIdx } = contextMenu!; closeContextMenu(); applyEditPreset('NULL', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-rose-300">NULL</button>
                      {/if}
                      {#if ctxKind.isDateTime}
                        <button onclick={() => { const { rIdx, cIdx } = contextMenu!; closeContextMenu(); applyEditPreset('NOW', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-sky-300">NOW</button>
                      {/if}
                      {#if ctxKind.isDate || ctxKind.isDateTime}
                        <button onclick={() => { const { rIdx, cIdx } = contextMenu!; closeContextMenu(); applyEditPreset('DATE', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300">TODAY</button>
                      {/if}
                      {#if ctxShowUuid}
                        <button onclick={() => { const { rIdx, cIdx } = contextMenu!; closeContextMenu(); applyEditPreset('UUID', rIdx, cIdx); }} class="px-1 rounded bg-slate-800 hover:bg-slate-700 text-purple-300">UUID</button>
                      {/if}
                    </div>
                  </div>
                {/if}
                {#if selectedRowIndices.length > 1}
                  <button
                    id="grid-ctx-btn-dup-nokey"
                    onclick={() => {
                      const indices = [...selectedRowIndices];
                      closeContextMenu();
                      duplicateMultipleRows(indices);
                    }}
                    class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-emerald-300 font-medium"
                  >
                    <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                    <span>Duplicate {selectedRowIndices.length} Rows</span>
                  </button>
                {:else}
                  <div class="px-3 py-1 flex items-center justify-between gap-1.5">
                    <span class="text-[11px] text-slate-400 flex items-center gap-1.5">
                      <Icon name="copy" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                      <span>Duplicate</span>
                    </span>
                    <div class="flex items-center gap-1 text-[10px]">
                      <button
                        id="grid-ctx-btn-dup-nokey"
                        onclick={() => {
                          duplicateRowAsDraft(contextMenu!.rIdx, true);
                          closeContextMenu();
                        }}
                        class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium"
                        title="Duplicate Row (Without Key)"
                      >No Key</button>
                      <button
                        id="grid-ctx-btn-dup-key"
                        onclick={() => {
                          duplicateRowAsDraft(contextMenu!.rIdx, false);
                          closeContextMenu();
                        }}
                        class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-indigo-300 font-medium"
                        title="Duplicate Row (With Key)"
                      >With Key</button>
                    </div>
                  </div>
                {/if}
                <button
                  id="grid-ctx-btn-add-draft"
                  onclick={() => {
                    addDraftRow();
                    closeContextMenu();
                  }}
                  class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-emerald-300"
                >
                  <Icon name="plus" class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                  <span>Add Row Draft</span>
                </button>
                <button
                  id="grid-ctx-btn-delete-row"
                  onclick={() => {
                    if (selectedRowIndices.length > 1) {
                      closeContextMenu();
                      handleBatchDelete();
                    } else {
                      selectedRowIndex = contextMenu!.rIdx;
                      handleDeleteCurrentRow();
                      closeContextMenu();
                    }
                  }}
                  class="w-full text-left px-3 py-1.5 hover:bg-rose-950/60 flex items-center space-x-2 text-rose-300"
                >
                  <Icon name="trash-2" class="w-3.5 h-3.5 text-rose-400 shrink-0" />
                  <span>{selectedRowIndices.length > 1 ? `Delete ${selectedRowIndices.length} Selected Rows` : 'Delete Selected Row(s)'}</span>
                </button>
              {/if}
            </div>
          </div>
        </div>
      </div>

      <div class="py-1">
        <button
          onclick={() => {
            const { rIdx, cIdx, colName, cellValue } = contextMenu!;
            closeContextMenu();
            appState.openJsonCellModal(effectiveTable || undefined, rIdx, cIdx, colName, cellValue, !canEdit);
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="code" class="w-3.5 h-3.5 text-purple-400 shrink-0" />
          <span>Open in JSON Editor</span>
        </button>
        <button
          onclick={() => {
            appState.recordInspectorRowIndex = contextMenu!.rIdx;
            appState.isRecordInspectorOpen = true;
            closeContextMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="eye" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
          <span>Inspect Record</span>
        </button>
        <button
          id="grid-ctx-btn-copy-cell"
          onclick={() => {
            const cv = contextMenu!.cellValue;
            copyText(cv === null || cv === undefined ? '' : typeof cv === 'object' ? JSON.stringify(cv, null, 2) : String(cv));
            appState.showToast('Copied cell value to clipboard', 'info');
            closeContextMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="copy" class="w-3.5 h-3.5 text-slate-400 shrink-0" />
          <span>Copy Cell Value</span>
        </button>
      </div>

      <div id="grid-ctx-copy-parent" class="py-1">
        <div id="grid-ctx-copy-submenu" class="px-3 py-1 flex items-center justify-between gap-1.5">
          <span class="text-[11px] text-slate-400 shrink-0">
            {selectedRowIndices.length > 1 ? `Copy ${selectedRowIndices.length}` : 'Copy Row'}
          </span>
          <div class="flex items-center gap-1 text-[10px] font-mono">
            <button
              id="grid-ctx-btn-copy-row-tsv"
              onclick={() => {
                if (selectedRowIndices.length > 1) copySelectedRows('tsv');
                else copyRowAsTSV(contextMenu!.rIdx);
                closeContextMenu();
              }}
              class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-emerald-300 font-medium"
              title="Copy as TSV"
            >TSV</button>
            <button
              id="grid-ctx-btn-copy-row-json"
              onclick={() => {
                if (selectedRowIndices.length > 1) copySelectedRows('json');
                else copyRowAsJSON(contextMenu!.rIdx);
                closeContextMenu();
              }}
              class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-amber-300 font-medium"
              title="Copy as JSON"
            >JSON</button>
            <button
              id="grid-ctx-btn-copy-row-md"
              onclick={() => {
                if (selectedRowIndices.length > 1) copySelectedRows('md');
                else copyRowAsMarkdown(contextMenu!.rIdx);
                closeContextMenu();
              }}
              class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 font-medium"
              title="Copy as Markdown"
            >MD</button>
            <button
              id="grid-ctx-btn-copy-row-sql"
              onclick={() => {
                if (selectedRowIndices.length > 1) copySelectedRows('sql');
                else copyRowAsInsertSQL(contextMenu!.rIdx);
                closeContextMenu();
              }}
              class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-purple-300 font-medium"
              title="Copy as INSERT SQL"
            >SQL</button>
          </div>
        </div>
      </div>
    </div>
  {/if}
</div>
