// --- Data Grid Rendering & Inline Edit ---
    let activeEditCell = null; // { td, rIdx, cIdx, pkCol, tableName, originalVal, input, isCommitting }
    let stagedEdits = new Map(); // key: `${tableName}:${rIdx}:${cIdx}`, val: { rIdx, cIdx, db, table, colName, pkCol, pkVal, oldVal, newVal }
    let gridEditMode = localStorage.getItem('sqlfox_grid_edit_mode') || 'staged'; // 'staged' | 'auto'
    let gridConfirmSave = localStorage.getItem('sqlfox_grid_confirm_save') !== 'false';
    let activeGridTable = null;
    let activeGridDB = null;
    let activeGridPKCol = null;

    function setGridEditMode(mode) {
      gridEditMode = mode;
      localStorage.setItem('sqlfox_grid_edit_mode', mode);
      updateGridEditModeUI();
      updateStagedEditsUI();
    }

    function toggleGridConfirmSave(checked) {
      gridConfirmSave = checked;
      localStorage.setItem('sqlfox_grid_confirm_save', String(checked));
    }

    function updateGridEditModeUI() {
      const btnStaged = document.getElementById('btn-grid-mode-staged');
      const btnAuto = document.getElementById('btn-grid-mode-auto');
      const chkConfirm = document.getElementById('grid-confirm-checkbox');

      if (chkConfirm) chkConfirm.checked = gridConfirmSave;

      if (btnStaged && btnAuto) {
        if (gridEditMode === 'auto') {
          btnAuto.className = 'px-1.5 py-0.5 rounded text-[10px] font-bold bg-amber-500 text-slate-950 transition shadow';
          btnStaged.className = 'px-1.5 py-0.5 rounded text-[10px] font-medium text-slate-400 hover:text-white transition';
        } else {
          btnStaged.className = 'px-1.5 py-0.5 rounded text-[10px] font-bold bg-sky-500 text-slate-950 transition shadow';
          btnAuto.className = 'px-1.5 py-0.5 rounded text-[10px] font-medium text-slate-400 hover:text-white transition';
        }
      }
    }

    function updateStagedEditsUI() {
      const applyBtn = document.getElementById('btn-grid-apply');
      const discardBtn = document.getElementById('btn-grid-discard');
      const countSpan = document.getElementById('grid-staged-count');

      const count = stagedEdits.size + stagedDraftRows.length;
      if (countSpan) countSpan.textContent = count;

      if (applyBtn && discardBtn) {
        if (count > 0 && (gridEditMode === 'staged' || stagedDraftRows.length > 0)) {
          applyBtn.classList.remove('hidden');
          discardBtn.classList.remove('hidden');
        } else {
          applyBtn.classList.add('hidden');
          discardBtn.classList.add('hidden');
        }
      }
    }

    // Resolves table metadata, primary key, and whether grid mutations are safe/permitted
    function resolveGridTableMeta() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const isTableViewer = Boolean(activeTab && activeTab.isTableViewer);
      let tableName = activeGridTable || (activeTab && (activeTab.tableName || activeTab.queryTable)) || '';
      let dbName = activeGridDB || (activeTab && (activeTab.database || activeTab.queryDB)) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';

      if (!tableName && activeTab && !isTableViewer) {
        const sql = activeTab.sql || getEditorSQL();
        const parsed = extractSingleTableFromSQL(sql);
        if (parsed && parsed.table) {
          tableName = parsed.table;
          if (parsed.database) dbName = parsed.database;
        }
      }

      const targetCatalog = (dbName && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[dbName.toLowerCase()]) ||
                            (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) ||
                            schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === (tableName || '').toLowerCase());

      let pkCol = activeGridPKCol;
      if (tableMeta && Array.isArray(tableMeta.columns)) {
        const pks = tableMeta.columns.filter(c => c.is_primary_key || c.key === 'PRI');
        if (pks.length === 1) {
          pkCol = pks[0].name;
        } else {
          pkCol = null;
        }
      } else if (!pkCol && tableMeta && Array.isArray(tableMeta.columns)) {
        const pks = tableMeta.columns.filter(c => c.is_primary_key || c.key === 'PRI');
        if (pks.length === 1) pkCol = pks[0].name;
      }

      const pkInCols = Boolean(pkCol && currentColumns && currentColumns.some(c => (c.name || '').toLowerCase() === pkCol.toLowerCase()));
      const canMutate = isTableViewer || Boolean(tableName && pkCol && pkInCols);

      return {
        isTableViewer,
        tableName,
        dbName,
        tableMeta,
        pkCol,
        pkInCols,
        canMutate
      };
    }

    // ponytail: Grid toolbar mode-switching between Table Viewer and Query Result modes
    function syncGridToolbarMode() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const isTableViewer = Boolean(activeTab && activeTab.isTableViewer);
      const meta = resolveGridTableMeta();
      const canMutate = isTableViewer || meta.canMutate;

      const btnAddRow = document.getElementById('btn-grid-add-row');
      const btnMockData = document.getElementById('btn-mock-data');
      const btnImportCSV = document.getElementById('btn-import-csv');
      const editModeContainer = document.getElementById('grid-edit-mode-container');
      const tableDumpBtn = document.getElementById('export-menu-table-dump');
      const gridResultStats = document.getElementById('grid-result-stats');
      const gridLimitBadge = document.getElementById('grid-limit-badge');

      if (btnAddRow) btnAddRow.classList.toggle('hidden', !canMutate);
      if (btnMockData) btnMockData.classList.toggle('hidden', !isTableViewer);
      if (btnImportCSV) btnImportCSV.classList.toggle('hidden', !isTableViewer);
      if (editModeContainer) editModeContainer.classList.toggle('hidden', !canMutate);
      if (tableDumpBtn) tableDumpBtn.classList.toggle('hidden', !isTableViewer);
      if (gridResultStats) gridResultStats.classList.toggle('hidden', isTableViewer);
      if (gridLimitBadge) gridLimitBadge.classList.toggle('hidden', isTableViewer);
      updateSelectedRowsUI();
    }

    // --- TablePlus-Style Inline Filter Bar & Pushdown ---
    function setFilterBarMode(mode) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      if (mode === 'raw' && (!activeTab.filterRaw || activeTab.filterRaw.trim() === '') && activeTab.filterPills && activeTab.filterPills.length > 0) {
        activeTab.filterRaw = compileFilterPills(activeTab.filterPills);
      }
      activeTab.filterMode = mode;
      syncFilterBarUI(activeTab);
    }

    function syncFilterBarUI(activeTab) {
      if (!activeTab || !activeTab.isTableViewer) return;
      const mode = activeTab.filterMode || 'pills';
      const btnPills = document.getElementById('btn-filter-mode-pills');
      const btnRaw = document.getElementById('btn-filter-mode-raw');
      const pillsContainer = document.getElementById('filter-pills-container');
      const addPillBtn = document.getElementById('btn-add-filter-pill');
      const rawContainer = document.getElementById('filter-raw-container');
      const rawInput = document.getElementById('filter-raw-input');
      const activeIndicator = document.getElementById('filter-active-indicator');

      const activeBtnClass = 'px-2 py-0.5 text-[11px] font-semibold rounded bg-app-accent text-slate-950 transition';
      const inactiveBtnClass = 'px-2 py-0.5 text-[11px] font-semibold rounded text-slate-400 hover:text-white transition';

      if (btnPills) btnPills.className = mode === 'pills' ? activeBtnClass : inactiveBtnClass;
      if (btnRaw) btnRaw.className = mode === 'raw' ? activeBtnClass : inactiveBtnClass;

      if (mode === 'pills') {
        if (pillsContainer) pillsContainer.classList.remove('hidden');
        if (addPillBtn) addPillBtn.classList.remove('hidden');
        if (rawContainer) rawContainer.classList.add('hidden');
        renderFilterPills(activeTab);
      } else {
        if (pillsContainer) pillsContainer.classList.add('hidden');
        if (addPillBtn) addPillBtn.classList.add('hidden');
        if (rawContainer) rawContainer.classList.remove('hidden');
        if (rawInput) {
          rawInput.value = activeTab.filterRaw !== undefined ? activeTab.filterRaw : (activeTab.appliedFilter || '');
          const clearBtn = document.getElementById('filter-raw-clear-btn');
          if (clearBtn) {
            if (rawInput.value.length > 0) clearBtn.classList.remove('hidden');
            else clearBtn.classList.add('hidden');
          }
        }
      }

      if (activeIndicator) {
        const isFiltered = Boolean(activeTab.appliedFilter);
        activeIndicator.classList.toggle('hidden', !isFiltered);
      }
    }

    function renderFilterPills(activeTab) {
      if (!activeTab) activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      if (!Array.isArray(activeTab.filterPills)) {
        activeTab.filterPills = [];
      }
      const container = document.getElementById('filter-pills-container');
      if (!container) return;

      if (activeTab.filterPills.length === 0) {
        container.innerHTML = '<span class="text-[11px] text-slate-500 italic select-none">No filter rules</span>';
        return;
      }

      const cols = currentColumns || [];

      container.innerHTML = activeTab.filterPills.map((pill, i) => {
        const colName = pill.col || pill.column || '';
        const op = pill.op || pill.operator || '=';
        const val = pill.val !== undefined ? pill.val : (pill.value !== undefined ? pill.value : '');
        const isUnary = op === 'IS NULL' || op === 'IS NOT NULL';
        return `
          <div class="filter-pill flex items-center space-x-1 bg-slate-950 border border-slate-700 rounded px-1.5 py-0.5 text-xs text-slate-200 shadow-sm" data-idx="${i}">
            <select class="pill-col bg-transparent text-sky-300 font-mono text-xs outline-none cursor-pointer border-r border-slate-800 pr-1 py-0.5" onchange="onPillColChange(${i}, this.value)">
              ${cols.length > 0 ? cols.map(c => `<option value="${escapeHtml(c.name)}" ${colName === c.name ? 'selected' : ''} class="bg-slate-900 text-slate-200">${escapeHtml(c.name)}</option>`).join('') : `<option value="${escapeHtml(colName)}">${escapeHtml(colName || 'column')}</option>`}
            </select>
            <select class="pill-op bg-transparent text-amber-300 font-mono text-xs outline-none cursor-pointer border-r border-slate-800 pr-1 py-0.5" onchange="onPillOpChange(${i}, this.value)">
              <option value="=" ${op === '=' ? 'selected' : ''} class="bg-slate-900 text-slate-200">=</option>
              <option value="!=" ${op === '!=' ? 'selected' : ''} class="bg-slate-900 text-slate-200">!=</option>
              <option value="LIKE" ${op === 'LIKE' ? 'selected' : ''} class="bg-slate-900 text-slate-200">LIKE</option>
              <option value="NOT LIKE" ${op === 'NOT LIKE' ? 'selected' : ''} class="bg-slate-900 text-slate-200">NOT LIKE</option>
              <option value=">" ${op === '>' ? 'selected' : ''} class="bg-slate-900 text-slate-200">&gt;</option>
              <option value="<" ${op === '<' ? 'selected' : ''} class="bg-slate-900 text-slate-200">&lt;</option>
              <option value=">=" ${op === '>=' ? 'selected' : ''} class="bg-slate-900 text-slate-200">&gt;=</option>
              <option value="<=" ${op === '<=' ? 'selected' : ''} class="bg-slate-900 text-slate-200">&lt;=</option>
              <option value="IN" ${op === 'IN' ? 'selected' : ''} class="bg-slate-900 text-slate-200">IN</option>
              <option value="IS NULL" ${op === 'IS NULL' ? 'selected' : ''} class="bg-slate-900 text-slate-200">IS NULL</option>
              <option value="IS NOT NULL" ${op === 'IS NOT NULL' ? 'selected' : ''} class="bg-slate-900 text-slate-200">IS NOT NULL</option>
            </select>
            <input type="text" class="pill-val bg-transparent text-slate-100 font-mono text-xs outline-none px-1 py-0.5 w-24 focus:w-36 transition-all ${isUnary ? 'hidden' : ''}"
                   value="${escapeHtml(String(val))}"
                   placeholder="${op === 'LIKE' || op === 'NOT LIKE' ? 'substring or %wildcard%' : (op === 'IN' ? '1, 2, 3' : 'val')}"
                   onkeydown="if(event.key === 'Enter') applyTableFilter()"
                   oninput="onPillValueInput(${i}, this.value)">
            <button type="button" onclick="removeFilterPill(${i})" class="text-slate-500 hover:text-rose-400 px-0.5 py-0.5 transition" title="Remove condition"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
          </div>
        `;
      }).join('');
    }

    function addFilterPill() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      if (!Array.isArray(activeTab.filterPills)) activeTab.filterPills = [];
      const defaultCol = (currentColumns && currentColumns.length > 0) ? currentColumns[0].name : '';
      activeTab.filterPills.push({ col: defaultCol, column: defaultCol, op: '=', operator: '=', val: '', value: '' });
      renderFilterPills(activeTab);
      setTimeout(() => {
        const container = document.getElementById('filter-pills-container');
        if (container) {
          const inputs = container.querySelectorAll('.pill-val');
          if (inputs.length > 0) inputs[inputs.length - 1].focus();
        }
      }, 20);
    }

    function removeFilterPill(index) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      if (!Array.isArray(activeTab.filterPills)) return;
      activeTab.filterPills.splice(index, 1);
      renderFilterPills(activeTab);
    }

    function onPillColChange(index, col) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && activeTab.filterPills && activeTab.filterPills[index]) {
        activeTab.filterPills[index].col = col;
        activeTab.filterPills[index].column = col;
      }
    }

    function onPillOpChange(index, op) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && activeTab.filterPills && activeTab.filterPills[index]) {
        activeTab.filterPills[index].op = op;
        activeTab.filterPills[index].operator = op;
        renderFilterPills(activeTab);
      }
    }

    function onPillValueInput(index, val) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && activeTab.filterPills && activeTab.filterPills[index]) {
        activeTab.filterPills[index].val = val;
        activeTab.filterPills[index].value = val;
      }
    }

    function compileFilterPills(pills) {
      if (!Array.isArray(pills) || pills.length === 0) return '';
      const clauses = [];
      for (const p of pills) {
        if (!p) continue;
        const colName = p.col || p.column;
        if (!colName) continue;
        const colEscaped = '`' + colName.replace(/`/g, '``') + '`';
        const op = (p.op || p.operator || '=').toUpperCase();
        if (op === 'IS NULL' || op === 'IS NOT NULL') {
          clauses.push(`${colEscaped} ${op}`);
          continue;
        }
        const val = String(p.val !== undefined ? p.val : (p.value !== undefined ? p.value : '')).trim();
        if (val === '') continue;
        if (op === 'IN') {
          if (val.startsWith('(') && val.endsWith(')')) {
            clauses.push(`${colEscaped} IN ${val}`);
          } else {
            const items = val.split(',').map(s => s.trim()).filter(Boolean);
            const formatted = items.map(item => {
              if (!isNaN(item) && item !== '') return item;
              return `'${item.replace(/'/g, "''")}'`;
            }).join(', ');
            clauses.push(`${colEscaped} IN (${formatted})`);
          }
        } else if (op === 'LIKE' || op === 'NOT LIKE') {
          // ponytail: smart wildcard auto-wrap for LIKE/NOT LIKE filter.
          // If the user didn't explicitly include SQL wildcards (% or _), automatically wrap with '%...%'.
          // If explicit wildcards exist (e.g. 'effendi%', '%effendi'), preserve them as-is.
          let finalVal = val;
          if (!val.includes('%') && !val.includes('_')) {
            finalVal = `%${val}%`;
          }
          clauses.push(`${colEscaped} ${op} '${finalVal.replace(/'/g, "''")}'`);
        } else {
          if (!isNaN(val) && val !== '' && !val.includes(' ')) {
            clauses.push(`${colEscaped} ${op} ${val}`);
          } else {
            clauses.push(`${colEscaped} ${op} '${val.replace(/'/g, "''")}'`);
          }
        }
      }
      return clauses.join(' AND ');
    }

    function applyTableFilter() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      activeTab.offset = 0;
      if (activeTab.filterMode === 'raw') {
        const rawInput = document.getElementById('filter-raw-input');
        let rawVal = rawInput ? rawInput.value.trim() : '';
        rawVal = rawVal.replace(/^where\s+/i, '').trim();
        activeTab.filterRaw = rawVal;
        activeTab.appliedFilter = rawVal;
      } else {
        activeTab.appliedFilter = compileFilterPills(activeTab.filterPills || []);
      }
      syncFilterBarUI(activeTab);
      loadTableRows(activeTab.tableName);
    }

    function onFilterRawInput(val) {
      const clearBtn = document.getElementById('filter-raw-clear-btn');
      if (clearBtn) {
        if (val && val.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
    }

    function clearFilterRawInput() {
      const input = document.getElementById('filter-raw-input');
      const clearBtn = document.getElementById('filter-raw-clear-btn');
      if (input) {
        input.value = '';
        input.focus();
      }
      if (clearBtn) {
        clearBtn.classList.add('hidden');
      }
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && activeTab.isTableViewer) {
        activeTab.filterRaw = '';
      }
    }

    function clearTableFilter() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      activeTab.appliedFilter = '';
      activeTab.filterRaw = '';
      activeTab.filterPills = [];
      activeTab.offset = 0;
      const rawInput = document.getElementById('filter-raw-input');
      if (rawInput) rawInput.value = '';
      const clearBtn = document.getElementById('filter-raw-clear-btn');
      if (clearBtn) clearBtn.classList.add('hidden');
      document.querySelectorAll('#grid-thead .col-filter-input').forEach(inp => {
        inp.value = '';
      });
      syncFilterBarUI(activeTab);
      loadTableRows(activeTab.tableName);
    }

    function focusTableFilter() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      if (activeTab.filterMode === 'raw') {
        const rawInput = document.getElementById('filter-raw-input');
        if (rawInput) {
          rawInput.focus();
          rawInput.select();
        }
      } else {
        if (!activeTab.filterPills || activeTab.filterPills.length === 0) {
          addFilterPill();
        } else {
          const container = document.getElementById('filter-pills-container');
          const firstInput = container ? container.querySelector('.pill-val') : null;
          if (firstInput) {
            firstInput.focus();
            firstInput.select();
          }
        }
      }
    }

    function handoffTableQueryToEditor() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;
      const tableName = activeTab.tableName;
      const targetTable = '`' + tableName.replace(/`/g, '``') + '`';
      let sql = `SELECT * FROM ${targetTable}`;
      const filter = (activeTab.appliedFilter || '').trim();
      if (filter) {
        sql += `\nWHERE ${filter}`;
      }
      if (activeTab.sortCol) {
        const escSortCol = '`' + activeTab.sortCol.replace(/`/g, '``') + '`';
        sql += `\nORDER BY ${escSortCol} ${activeTab.sortDir || 'ASC'}`;
      }
      sql += `\nLIMIT 50;`;

      const newId = 'tab-' + Date.now();
      tabs.push({
        id: newId,
        title: `${tableName}-query.sql`,
        sql: sql,
        database: activeTab.database,
        isTableViewer: false
      });
      switchTab(newId);
    }

    function sortColumn(colIndex) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !currentColumns[colIndex]) return;
      const colName = currentColumns[colIndex].name;
      if (activeTab.sortCol === colName) {
        if (activeTab.sortDir === 'ASC') {
          activeTab.sortDir = 'DESC';
        } else {
          activeTab.sortCol = '';
          activeTab.sortDir = '';
        }
      } else {
        activeTab.sortCol = colName;
        activeTab.sortDir = 'ASC';
      }

      if (activeTab.isTableViewer) {
        activeTab.offset = 0;
        loadTableRows(activeTab.tableName);
      } else {
        activeTab.queryPage = 0;
        const rows = (activeTab.queryFilters && Object.keys(activeTab.queryFilters).length > 0) ? currentResults : (activeTab.rawQueryRows || currentResults || []);
        if (!activeTab.sortCol) {
          if (activeTab.queryFilters && Object.keys(activeTab.queryFilters).length > 0) {
            applyQueryResultsFilter(activeTab);
          } else {
            renderGridResults(currentColumns, activeTab.rawQueryRows || rows, activeGridPKCol, activeGridTable, activeGridDB, true);
          }
          return;
        }
        // isFilterPass=true: a sorted view must not overwrite the tab's raw (server-order) result set.
        renderGridResults(currentColumns, sortQueryRows(activeTab, rows), activeGridPKCol, activeGridTable, activeGridDB, true);
      }
    }

    // sortQueryRows returns a sorted copy of rows by the tab's sortCol/sortDir (row arrays are shared,
    // so staged edits keyed by row identity keep pointing at the right rows).
    function sortQueryRows(tab, rows) {
      const colIndex = (currentColumns || []).findIndex(c => c.name === tab.sortCol);
      if (colIndex < 0 || !tab.sortCol) return rows || [];
      return [...(rows || [])].sort((a, b) => {
        const valA = a[colIndex];
        const valB = b[colIndex];
        if (valA === null || valA === undefined) return 1;
        if (valB === null || valB === undefined) return -1;
        const numA = Number(valA);
        const numB = Number(valB);
        let cmp = 0;
        if (!isNaN(numA) && !isNaN(numB)) {
          cmp = numA - numB;
        } else {
          cmp = String(valA).localeCompare(String(valB));
        }
        return tab.sortDir === 'DESC' ? -cmp : cmp;
      });
    }

    function getGridActivePageSize(activeTab) {
      if (activeTab && typeof activeTab.pageSize === 'number' && activeTab.pageSize > 0) {
        return activeTab.pageSize;
      }
      return 50;
    }

    function getTableViewerTotalRows(activeTab) {
      if (!activeTab || !activeTab.isTableViewer) return null;
      if (typeof activeTab.exactRowCount === 'number') {
        return activeTab.exactRowCount;
      }
      if (activeTab.appliedFilter) {
        return null;
      }
      const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === (activeGridTable || activeTab.tableName || '').toLowerCase());
      if (tableMeta && typeof tableMeta.row_count === 'number' && tableMeta.row_count >= 0) {
        return tableMeta.row_count;
      }
      return null;
    }

    function goToFirstPage() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      if (activeTab.isTableViewer) {
        if ((activeTab.offset || 0) === 0) return;
        activeTab.offset = 0;
        loadTableRows(activeTab.tableName);
      } else {
        if ((activeTab.queryPage || 0) === 0) return;
        activeTab.queryPage = 0;
        renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
        const gridWrapper = document.getElementById('virtual-grid-wrapper');
        if (gridWrapper) gridWrapper.scrollTop = 0;
      }
    }

    function goToLastPage() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const pageSize = getGridActivePageSize(activeTab);
      if (activeTab.isTableViewer) {
        const totalRows = getTableViewerTotalRows(activeTab);
        if (typeof totalRows === 'number' && totalRows > 0) {
          const totalPages = Math.max(1, Math.ceil(totalRows / pageSize));
          const targetOffset = Math.max(0, (totalPages - 1) * pageSize);
          if (activeTab.offset !== targetOffset) {
            activeTab.offset = targetOffset;
            loadTableRows(activeTab.tableName);
          }
        }
      } else {
        const rows = (activeTab.queryFilters && Object.keys(activeTab.queryFilters).length > 0) ? currentResults : (activeTab.rawQueryRows || currentResults || []);
        const totalRows = rows ? rows.length : 0;
        const totalPages = Math.max(1, Math.ceil(totalRows / pageSize));
        const targetPage = Math.max(0, totalPages - 1);
        if (activeTab.queryPage !== targetPage) {
          activeTab.queryPage = targetPage;
          renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
          const gridWrapper = document.getElementById('virtual-grid-wrapper');
          if (gridWrapper) gridWrapper.scrollTop = 0;
        }
      }
    }

    function goToNextPage() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const pageSize = getGridActivePageSize(activeTab);
      if (activeTab.isTableViewer) {
        activeTab.offset = (activeTab.offset || 0) + pageSize;
        loadTableRows(activeTab.tableName);
      } else {
        const rows = (activeTab.queryFilters && Object.keys(activeTab.queryFilters).length > 0) ? currentResults : (activeTab.rawQueryRows || currentResults || []);
        const totalRows = rows ? rows.length : 0;
        const page = activeTab.queryPage || 0;
        if ((page + 1) * pageSize < totalRows) {
          activeTab.queryPage = page + 1;
          renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
          const gridWrapper = document.getElementById('virtual-grid-wrapper');
          if (gridWrapper) gridWrapper.scrollTop = 0;
        }
      }
    }

    function goToPreviousPage() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const pageSize = getGridActivePageSize(activeTab);
      if (activeTab.isTableViewer) {
        activeTab.offset = Math.max(0, (activeTab.offset || 0) - pageSize);
        loadTableRows(activeTab.tableName);
      } else {
        const page = activeTab.queryPage || 0;
        if (page > 0) {
          activeTab.queryPage = page - 1;
          renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
          const gridWrapper = document.getElementById('virtual-grid-wrapper');
          if (gridWrapper) gridWrapper.scrollTop = 0;
        }
      }
    }

    function loadNextPage() {
      goToNextPage();
    }

    function loadPreviousPage() {
      goToPreviousPage();
    }

    function goToPage(pageNum) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const pageSize = getGridActivePageSize(activeTab);
      let target = parseInt(pageNum, 10);
      if (isNaN(target) || target < 1) target = 1;

      if (activeTab.isTableViewer) {
        const totalRows = getTableViewerTotalRows(activeTab);
        // Only an exact count may clamp the page; catalog estimates can be lower than reality.
        if (typeof activeTab.exactRowCount === 'number' && typeof totalRows === 'number' && totalRows > 0) {
          const totalPages = Math.max(1, Math.ceil(totalRows / pageSize));
          if (target > totalPages) target = totalPages;
        }
        activeTab.offset = (target - 1) * pageSize;
        loadTableRows(activeTab.tableName);
      } else {
        const rows = (activeTab.queryFilters && Object.keys(activeTab.queryFilters).length > 0) ? currentResults : (activeTab.rawQueryRows || currentResults || []);
        const totalRows = rows ? rows.length : 0;
        const totalPages = Math.max(1, Math.ceil(totalRows / pageSize));
        if (target > totalPages) target = totalPages;
        activeTab.queryPage = Math.max(0, target - 1);
        renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
        const gridWrapper = document.getElementById('virtual-grid-wrapper');
        if (gridWrapper) gridWrapper.scrollTop = 0;
      }
    }

    function onPageInputKeydown(event, input) {
      if (event.key === 'Enter') {
        event.preventDefault();
        goToPage(input.value);
      } else if (event.key === 'Escape') {
        const activeTab = tabs.find(t => t.id === activeTabId);
        const pageSize = getGridActivePageSize(activeTab);
        if (activeTab?.isTableViewer) {
          input.value = Math.floor((activeTab.offset || 0) / pageSize) + 1;
        } else {
          input.value = (activeTab?.queryPage || 0) + 1;
        }
        input.blur();
      }
    }

    function onGridPageSizeChange(val) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const newSize = parseInt(val, 10);
      if (isNaN(newSize) || newSize <= 0) return;
      activeTab.pageSize = newSize;
      if (activeTab.isTableViewer) {
        activeTab.offset = 0;
        loadTableRows(activeTab.tableName);
      } else {
        activeTab.queryPage = 0;
        renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
        const gridWrapper = document.getElementById('virtual-grid-wrapper');
        if (gridWrapper) gridWrapper.scrollTop = 0;
      }
    }

    async function requestAccurateTableCount() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer || !activeConnection) return;
      const btn = document.getElementById('btn-count-accurate');
      if (btn) btn.textContent = 'Menghitung...';
      try {
        const tableName = activeTab.tableName;
        const isMySQLFamily = ['mysql', 'mariadb'].includes(activeConnection.driver);
        const targetTable = isMySQLFamily
          ? '`' + tableName.replace(/`/g, '``') + '`'
          : '"' + tableName.replace(/"/g, '""') + '"';
        let sql = `SELECT COUNT(*) AS total FROM ${targetTable}`;
        if (activeTab.appliedFilter) {
          sql += ` WHERE (${activeTab.appliedFilter})`;
        }
        const db = activeGridDB || activeTab.database;
        const res = await apiFetch(`/api/session/${activeConnection.id}/execute`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ sql, database: db })
        });
        if (res.ok) {
          const data = await res.json();
          if (data.rows && data.rows.length > 0 && data.rows[0].length > 0) {
            const count = Number(data.rows[0][0]);
            if (!isNaN(count)) {
              activeTab.exactRowCount = count;
              renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
            }
          }
        }
      } catch (e) {
        console.error('Failed to count table rows:', e);
      } finally {
        if (btn) btn.textContent = 'Hitung Akurat';
      }
    }

    async function loadTableRows(tableName, targetDb = null) {
      if (!activeConnection) return;
      try {
        const activeTab = tabs.find(t => t.id === activeTabId);
        const requestingTabId = activeTab ? activeTab.id : activeTabId;
        // Only the latest request per tab may render: rapid paging/sorting must not let a slower,
        // older response overwrite the page the pager now shows.
        const loadSeq = activeTab ? (activeTab._loadSeq = (activeTab._loadSeq || 0) + 1) : 0;
        const isStale = () => activeTabId !== requestingTabId || (activeTab && activeTab._loadSeq !== loadSeq);
        // ponytail: Prioritize targetDb and activeGridDB over fallback schemaCatalogCache so cross-database tables load from their own schema.
        const dbName = targetDb || (activeTab && activeTab.isTableViewer && activeTab.database) || activeGridDB || (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';
        activeGridTable = tableName;
        activeGridDB = dbName;
        stagedEdits.clear();
        updateStagedEditsUI();
        activeEditCell = null;
        selectedRowIndices.clear();
        updateSelectedRowsUI();

        const filter = (activeTab && activeTab.appliedFilter) || '';
        const sortCol = (activeTab && activeTab.sortCol) || '';
        const sortDir = (activeTab && activeTab.sortDir) || '';
        const offset = (activeTab && activeTab.offset) || 0;
        const pageSize = getGridActivePageSize(activeTab);
        saveTabsState();

        let url = `/api/session/${activeConnection.id}/table/${encodeURIComponent(tableName)}/rows?limit=${pageSize}&offset=${offset}`;
        if (dbName) url += `&db=${encodeURIComponent(dbName)}`;
        if (filter) url += `&filter=${encodeURIComponent(filter)}`;
        if (sortCol) url += `&sort=${encodeURIComponent(sortCol)}&dir=${encodeURIComponent(sortDir || 'ASC')}`;

        // ponytail: immediate loading state prevents screen from appearing frozen on slow table queries
        const tbody = document.getElementById('grid-tbody');
        if (tbody) {
          tbody.innerHTML = `<tr><td colspan="100" class="p-12 text-center text-slate-400 font-mono text-xs"><div class="inline-flex items-center space-x-2">${renderIcon('loader', 'w-4 h-4 animate-spin text-sky-400')}<span>Loading rows for '${escapeHtml(tableName)}'...</span></div></td></tr>`;
        }
        const btnFirst = document.getElementById('btn-page-first');
        const btnPrev = document.getElementById('btn-page-prev');
        const btnNext = document.getElementById('btn-page-next');
        const btnLast = document.getElementById('btn-page-last');
        if (btnFirst) btnFirst.disabled = true;
        if (btnPrev) btnPrev.disabled = true;
        if (btnNext) btnNext.disabled = true;
        if (btnLast) btnLast.disabled = true;

        const res = await apiFetch(url);
        if (isStale()) return;
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          if (isStale()) return;
          console.error('Failed to load rows:', errData);
          renderGridResults([], [], null, tableName, dbName);
          const tbody = document.getElementById('grid-tbody');
          if (tbody) {
            tbody.innerHTML = `<tr><td class="p-8 text-center text-app-danger font-mono text-xs">Failed to load data: ${escapeHtml(errData.error || res.statusText)}</td></tr>`;
          }
          return;
        }
        const data = await res.json();
        if (isStale()) return;
        renderGridResults(data.columns, data.rows, data.pk_col, tableName, dbName);
      } catch (err) {
        console.error('Failed to load rows:', err);
        const tbody = document.getElementById('grid-tbody');
        if (tbody) {
          tbody.innerHTML = `<tr><td class="p-8 text-center text-app-danger font-mono text-xs">Failed to load data: ${escapeHtml(err.message || 'Network error')}</td></tr>`;
        }
        renderGridResults([], [], null, tableName, dbName);
      }
    }

    // --- Column Visibility Picker Logic ---
    function getHiddenColumns(tableName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return new Set();
      if (!activeTab.hiddenColumns) {
        activeTab.hiddenColumns = new Set();
        const tName = tableName || activeGridTable || activeTab.tableName || '';
        const dbName = activeGridDB || activeTab.database || '';
        if (tName) {
          try {
            const raw = sessionStorage.getItem(`sqlfox-hidden-cols:${dbName}:${tName}`);
            if (raw) {
              const arr = JSON.parse(raw);
              if (Array.isArray(arr)) {
                activeTab.hiddenColumns = new Set(arr);
              }
            }
          } catch (e) {}
        }
      }
      return activeTab.hiddenColumns;
    }

    function isColumnHidden(colName, tableName) {
      if (!colName) return false;
      const hidden = getHiddenColumns(tableName);
      if (activeGridPKCol && colName.toLowerCase() === activeGridPKCol.toLowerCase()) {
        return false;
      }
      return hidden.has(colName);
    }

    function updateColumnPickerBadge() {
      const badge = document.getElementById('col-picker-count-badge');
      if (!badge) return;
      if (!currentColumns || currentColumns.length === 0) {
        badge.textContent = '(0/0)';
        badge.classList.remove('text-amber-400', 'font-bold');
        badge.classList.add('text-slate-400');
        return;
      }
      const total = currentColumns.length;
      let hiddenCount = 0;
      currentColumns.forEach(c => {
        if (isColumnHidden(c.name, activeGridTable)) hiddenCount++;
      });
      const visible = total - hiddenCount;
      badge.textContent = `(${visible}/${total})`;
      if (hiddenCount > 0) {
        badge.classList.remove('text-slate-400');
        badge.classList.add('text-amber-400', 'font-bold');
      } else {
        badge.classList.remove('text-amber-400', 'font-bold');
        badge.classList.add('text-slate-400');
      }
    }

    function toggleColumnPickerDropdown(e) {
      if (e) e.stopPropagation();
      const dropdown = document.getElementById('col-picker-dropdown');
      if (!dropdown) return;
      const isHidden = dropdown.classList.contains('hidden');
      closeAllContextMenus();
      if (isHidden) {
        renderColumnPickerList();
        dropdown.classList.remove('hidden');
      } else {
        dropdown.classList.add('hidden');
      }
    }

    function renderColumnPickerList(filterText = '') {
      const container = document.getElementById('col-picker-list');
      if (!container || !currentColumns) return;
      const hidden = getHiddenColumns(activeGridTable);
      const query = (filterText || '').toLowerCase().trim();

      const items = currentColumns.filter(c => {
        if (!query) return true;
        return c.name.toLowerCase().includes(query) || (c.type || '').toLowerCase().includes(query);
      });

      if (items.length === 0) {
        container.innerHTML = '<div class="p-2 text-center text-slate-500 italic text-[11px]">No columns match</div>';
        return;
      }

      container.innerHTML = items.map(c => {
        const isPK = Boolean(activeGridPKCol && c.name.toLowerCase() === activeGridPKCol.toLowerCase());
        const isChecked = !hidden.has(c.name);
        const lockBadge = isPK ? '<span class="text-amber-400 font-bold ml-1 text-[10px]" title="Primary Key cannot be hidden">🔒 PK</span>' : '';
        return `
          <label class="flex items-center justify-between p-1 hover:bg-slate-800 rounded cursor-pointer select-none text-[11px] ${isPK ? 'opacity-90' : ''}">
            <div class="flex items-center space-x-2 truncate">
              <input type="checkbox" ${isChecked ? 'checked' : ''} ${isPK ? 'disabled' : ''}
                     onchange="toggleColumnVisibility(${jsArg(c.name)}, this.checked)"
                     class="accent-sky-500 w-3.5 h-3.5 rounded cursor-pointer ${isPK ? 'cursor-not-allowed opacity-60' : ''}">
              <span class="truncate font-mono ${isChecked ? 'text-slate-200' : 'text-slate-500 line-through'}">${escapeHtml(c.name)}</span>
              ${lockBadge}
            </div>
            <span class="text-[9px] text-slate-500 font-mono ml-2 shrink-0">${escapeHtml(c.type || '')}</span>
          </label>
        `;
      }).join('');
    }

    function filterColumnPickerList(val) {
      renderColumnPickerList(val);
    }

    function toggleColumnVisibility(colName, isVisible) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const hidden = getHiddenColumns(activeGridTable);
      if (isVisible) {
        hidden.delete(colName);
      } else {
        hidden.add(colName);
      }
      const tName = activeGridTable || activeTab.tableName || '';
      const dbName = activeGridDB || activeTab.database || '';
      if (tName) {
        try {
          sessionStorage.setItem(`sqlfox-hidden-cols:${dbName}:${tName}`, JSON.stringify([...hidden]));
        } catch (e) {}
      }
      updateColumnPickerBadge();
      renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
      renderColumnPickerList(document.getElementById('col-picker-search')?.value || '');
    }

    function showAllColumns() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const hidden = getHiddenColumns(activeGridTable);
      hidden.clear();
      const tName = activeGridTable || activeTab.tableName || '';
      const dbName = activeGridDB || activeTab.database || '';
      if (tName) {
        try {
          sessionStorage.removeItem(`sqlfox-hidden-cols:${dbName}:${tName}`);
        } catch (e) {}
      }
      updateColumnPickerBadge();
      renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
      renderColumnPickerList(document.getElementById('col-picker-search')?.value || '');
    }

    function hideAllColumns() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !currentColumns || currentColumns.length === 0) return;
      const hidden = getHiddenColumns(activeGridTable);
      hidden.clear();
      const effectivePK = activeGridPKCol || currentColumns[0]?.name;
      currentColumns.forEach(c => {
        // Sticky PK protection: Primary Key (or fallback first column) remains visible
        if (c.name !== effectivePK && !c.isPrimary) {
          hidden.add(c.name);
        }
      });
      const tName = activeGridTable || activeTab.tableName || '';
      const dbName = activeGridDB || activeTab.database || '';
      if (tName) {
        try {
          sessionStorage.setItem(`sqlfox-hidden-cols:${dbName}:${tName}`, JSON.stringify([...hidden]));
        } catch (e) {}
      }
      updateColumnPickerBadge();
      renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
      renderColumnPickerList(document.getElementById('col-picker-search')?.value || '');
    }

    const renderVirtualGrid = renderGridResults;

    function renderGridPaginationBar(activeTab, rows, isFilterPass) {
      const bar = document.getElementById('grid-pagination-bar');
      if (!bar) return;
      if (!activeTab) activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;

      bar.classList.remove('hidden');

      const pageSize = getGridActivePageSize(activeTab);
      const isTable = Boolean(activeTab.isTableViewer);
      const bottomStats = document.getElementById('grid-bottom-stats');
      const btnCountAccurate = document.getElementById('btn-count-accurate');
      const pageSizeSelect = document.getElementById('grid-page-size-select');
      const pageInput = document.getElementById('grid-page-input');
      const totalPagesEl = document.getElementById('grid-total-pages');
      const btnFirst = document.getElementById('btn-page-first');
      const btnPrev = document.getElementById('btn-page-prev');
      const btnNext = document.getElementById('btn-page-next');
      const btnLast = document.getElementById('btn-page-last');

      if (pageSizeSelect) {
        pageSizeSelect.value = String(pageSize);
      }

      if (isTable) {
        const off = activeTab.offset || 0;
        const curRowsCount = rows ? rows.length : 0;
        const startRow = curRowsCount > 0 ? (off + 1) : 0;
        const endRow = off + curRowsCount;
        const totalRows = getTableViewerTotalRows(activeTab);
        const curPage = Math.floor(off / pageSize) + 1;

        if (typeof totalRows === 'number') {
          // Catalog row counts are estimates (e.g. InnoDB TABLE_ROWS): never let them block paging.
          const isExact = typeof activeTab.exactRowCount === 'number';
          const totalPages = Math.max(1, Math.ceil(totalRows / pageSize));
          if (bottomStats) {
            bottomStats.textContent = `Showing ${startRow} - ${endRow} of ${totalRows.toLocaleString()} rows${activeTab.appliedFilter ? ' (filtered)' : ''}`;
          }
          if (pageInput) {
            pageInput.value = curPage;
            if (isExact) {
              pageInput.max = totalPages;
            } else if (typeof pageInput.removeAttribute === 'function') {
              pageInput.removeAttribute('max'); // an estimate must not cap manual page entry
            }
          }
          if (totalPagesEl) totalPagesEl.textContent = String(totalPages);
          if (btnFirst) btnFirst.disabled = curPage <= 1;
          if (btnPrev) btnPrev.disabled = curPage <= 1;
          // With an estimate, a full page means there may be more rows than the catalog claims.
          if (btnNext) btnNext.disabled = isExact ? curPage >= totalPages : (curPage >= totalPages && curRowsCount < pageSize);
          if (btnLast) btnLast.disabled = curPage >= totalPages;
          if (btnCountAccurate) btnCountAccurate.classList.add('hidden');
        } else {
          // Total rows not known yet (e.g. filtered table without exact count)
          const hasMore = curRowsCount >= pageSize;
          if (bottomStats) {
            bottomStats.textContent = `Showing ${startRow} - ${endRow} rows (filtered)`;
          }
          if (pageInput) {
            pageInput.value = curPage;
            pageInput.max = '';
            if (typeof pageInput.removeAttribute === 'function') {
              pageInput.removeAttribute('max');
            }
          }
          if (totalPagesEl) totalPagesEl.textContent = hasMore ? (curPage + '+') : String(curPage);
          if (btnFirst) btnFirst.disabled = curPage <= 1;
          if (btnPrev) btnPrev.disabled = curPage <= 1;
          if (btnNext) btnNext.disabled = !hasMore;
          if (btnLast) btnLast.disabled = true;
          if (btnCountAccurate) btnCountAccurate.classList.remove('hidden');
        }
      } else {
        // Query Results Tab (in-memory)
        if (btnCountAccurate) btnCountAccurate.classList.add('hidden');
        const allRows = rows || [];
        const totalRows = allRows.length;
        const totalPages = Math.max(1, Math.ceil(totalRows / pageSize));
        const curPage = Math.min((activeTab.queryPage || 0) + 1, totalPages);
        const startRow = totalRows > 0 ? ((curPage - 1) * pageSize + 1) : 0;
        const endRow = Math.min(curPage * pageSize, totalRows);

        if (bottomStats) {
          bottomStats.textContent = `Showing ${startRow} - ${endRow} of ${totalRows.toLocaleString()} rows${isFilterPass && activeTab?.rawQueryRows && rows.length !== activeTab.rawQueryRows.length ? ' (filtered)' : ''}`;
        }
        if (pageInput) {
          pageInput.value = curPage;
          pageInput.max = totalPages;
        }
        if (totalPagesEl) totalPagesEl.textContent = String(totalPages);
        if (btnFirst) btnFirst.disabled = curPage <= 1;
        if (btnPrev) btnPrev.disabled = curPage <= 1;
        if (btnNext) btnNext.disabled = curPage >= totalPages;
        if (btnLast) btnLast.disabled = curPage >= totalPages;
      }
    }

    // --- Persistent Cell Detail Drawer (ADR 0058) ---
    let isCellDrawerOpen = false;

    function openCellDetailDrawer(colName, colType, rowIndex, rawVal) {
      const drawer = document.getElementById('cell-detail-drawer');
      if (!drawer) return;
      drawer.classList.remove('hidden');
      isCellDrawerOpen = true;

      const colEl = document.getElementById('cell-drawer-col');
      const typeEl = document.getElementById('cell-drawer-type');
      const rowEl = document.getElementById('cell-drawer-row');
      const contentEl = document.getElementById('cell-drawer-content');

      if (colEl) colEl.textContent = colName || 'Column';
      if (typeEl) typeEl.textContent = colType || 'text';
      if (rowEl) rowEl.textContent = `Row #${rowIndex + 1}`;
      if (contentEl) {
        if (rawVal === null || rawVal === undefined) {
          contentEl.textContent = 'NULL';
          contentEl.classList.add('italic', 'text-amber-400');
        } else {
          contentEl.classList.remove('italic', 'text-amber-400');
          if (typeof rawVal === 'object') {
            try {
              contentEl.textContent = JSON.stringify(rawVal, null, 2);
            } catch (e) {
              contentEl.textContent = String(rawVal);
            }
          } else {
            contentEl.textContent = String(rawVal);
          }
        }
      }
    }

    function syncCellDetailDrawer(td, rIdx, cIdx) {
      if (!isCellDrawerOpen) return;
      const col = currentColumns[cIdx];
      const colName = col ? col.name : `Col ${cIdx}`;
      const colType = col ? col.type : 'text';
      let rawVal = null;
      if (currentResults && currentResults[rIdx] && currentResults[rIdx][cIdx] !== undefined) {
        rawVal = currentResults[rIdx][cIdx];
      } else if (td) {
        if (td.dataset.isNull === 'true') {
          rawVal = null;
        } else if (td.dataset.rawVal !== undefined) {
          rawVal = td.dataset.rawVal;
        } else {
          rawVal = td.textContent;
        }
      }
      openCellDetailDrawer(colName, colType, rIdx, rawVal);
    }

    function closeCellDetailDrawer() {
      const drawer = document.getElementById('cell-detail-drawer');
      if (drawer) drawer.classList.add('hidden');
      isCellDrawerOpen = false;
    }

    function toggleCellDetailDrawer() {
      if (isCellDrawerOpen) {
        closeCellDetailDrawer();
      } else {
        if (activeGridSelectedTd && activeGridSelectedRow !== null && activeGridSelectedCol !== null) {
          isCellDrawerOpen = true;
          syncCellDetailDrawer(activeGridSelectedTd, activeGridSelectedRow, activeGridSelectedCol);
        } else {
          openCellDetailDrawer('No cell selected', '', 0, 'Select any table cell to inspect full content');
        }
      }
    }

    function formatCellDrawerJSON() {
      const contentEl = document.getElementById('cell-drawer-content');
      if (!contentEl) return;
      try {
        const parsed = JSON.parse(contentEl.textContent);
        contentEl.textContent = JSON.stringify(parsed, null, 2);
        showToast('JSON formatted');
      } catch (e) {
        showToast('Cell content is not valid JSON');
      }
    }

    function copyCellDrawerContent() {
      const contentEl = document.getElementById('cell-drawer-content');
      if (!contentEl) return;
      navigator.clipboard.writeText(contentEl.textContent)
        .then(() => showToast('Cell content copied to clipboard'))
        .catch(() => showToast('Failed to copy'));
    }

    // --- Virtual Data Grid Windowing Engine (ADR 0058) ---
    const VIRTUAL_ROW_HEIGHT = 32;
    let virtualGridRows = [];
    let virtualGridCols = [];
    let virtualGridPKCol = null;
    let virtualGridTable = null;
    let virtualGridDB = null;
    let virtualGridHasCheckbox = false;
    let virtualGridFkMap = null;
    let virtualGridColMetaMap = null;
    let isVirtualGridActive = false;
    let virtualGridStartIdx = 0;

    function renderVirtualWindow() {
      const wrapper = document.getElementById('virtual-grid-wrapper');
      const tbody = document.getElementById('grid-tbody');
      if (!wrapper || !tbody || !isVirtualGridActive) return;

      const totalRows = virtualGridRows.length;
      if (totalRows === 0) return;

      // ponytail: Auto-commit in-flight inline edit on scroll to prevent data loss
      if (activeEditCell) {
        commitInlineEdit(activeEditCell, true);
      }

      const scrollTop = wrapper.scrollTop;
      const clientHeight = wrapper.clientHeight || 500;

      const buffer = 5;
      const startIndex = Math.max(0, Math.floor(scrollTop / VIRTUAL_ROW_HEIGHT) - buffer);
      const visibleCount = Math.ceil(clientHeight / VIRTUAL_ROW_HEIGHT) + (buffer * 2);
      const endIndex = Math.min(totalRows, startIndex + visibleCount);

      const topPadding = startIndex * VIRTUAL_ROW_HEIGHT;
      const bottomPadding = Math.max(0, (totalRows - endIndex) * VIRTUAL_ROW_HEIGHT);

      const slice = virtualGridRows.slice(startIndex, endIndex);
      const visibleColsCount = virtualGridCols.filter(c => !isColumnHidden(c?.name, virtualGridTable)).length;
      const colSpan = visibleColsCount + 1 + (virtualGridHasCheckbox ? 1 : 0);

      let html = '';
      if (topPadding > 0) {
        html += `<tr style="height: ${topPadding}px; pointer-events: none;"><td colspan="${colSpan}" style="padding:0; border:0; height:${topPadding}px;"></td></tr>`;
      }

      html += slice.map((row, pIdx) => {
        const rIdx = (virtualGridStartIdx || 0) + startIndex + pIdx;
        const rowNum = rIdx + 1;
        const gutterCell = `<td class="p-1 px-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${rowNum}</td>`;
        const isSelected = selectedRowIndices.has(rIdx);
        let checkboxCell = '';
        if (virtualGridHasCheckbox) {
          checkboxCell = `<td class="p-1.5 w-8 text-center border-r border-slate-800/40"><input type="checkbox" class="grid-row-checkbox accent-sky-500 w-3.5 h-3.5 cursor-pointer" data-row="${rIdx}" ${isSelected ? 'checked' : ''} onchange="toggleSelectRow(${rIdx}, this.checked)"></td>`;
        }
        return `
        <tr class="h-8 hover:bg-slate-800/40 transition ${isSelected ? 'bg-sky-950/20' : ''}" oncontextmenu="openGridContextMenu(event, ${rIdx})">
          ${gutterCell}
          ${checkboxCell}
          ${row.map((val, cIdx) => {
            if (isColumnHidden(virtualGridCols[cIdx]?.name, virtualGridTable)) return '';
            const stagedKey = stagedKeyFor(virtualGridTable || '', rIdx, cIdx);
            const staged = stagedEdits.get(stagedKey);
            const rawVal = staged ? staged.newVal : val;
            const isDirty = Boolean(staged);
            const isNullVal = rawVal === null || rawVal === undefined;
            const rawValStr = isNullVal ? '' : (typeof rawVal === 'object' ? JSON.stringify(rawVal) : String(rawVal));
            const colName = virtualGridCols[cIdx]?.name;
            const colType = virtualGridCols[cIdx]?.type;
            const colMeta = colName ? (virtualGridColMetaMap ? virtualGridColMetaMap.get(colName.toLowerCase()) : null) : null;
            const resolvedType = colMeta?.type || colType;
            const displayVal = formatGridCellValue(rawVal, resolvedType);
            let typeStyle = getGridTypeStyle(colType);
            if (colMeta?.type) typeStyle = getGridTypeStyle(colMeta.type);
            const fkInfo = virtualGridFkMap ? virtualGridFkMap.get(colName) : null;
            let fkBadge = '';
            if (fkInfo && displayVal !== null && displayVal !== '') {
              fkBadge = `<button type="button" data-table="${escapeHtml(virtualGridTable || '')}" data-col="${escapeHtml(colName)}" data-reftable="${escapeHtml(fkInfo.referenced_table)}" data-refcol="${escapeHtml(fkInfo.referenced_column)}" data-val="${escapeHtml(String(displayVal))}" onclick="event.stopPropagation(); peekForeignKey(event, this.dataset.table, this.dataset.col, this.dataset.reftable, this.dataset.refcol, this.dataset.val)" class="ml-1 opacity-0 group-hover:opacity-100 hover:text-sky-300 text-sky-400 font-bold transition text-xs inline-flex items-center" title="Peek referenced record">${renderIcon('link', 'w-3 h-3')}</button>`;
            }
            const isJson = displayVal !== null && typeof displayVal === 'string' && (typeStyle.category === 'json' || (displayVal.trim().startsWith('{') && displayVal.trim().endsWith('}')) || (displayVal.trim().startsWith('[') && displayVal.trim().endsWith(']')));
            let jsonBadge = '';
            if (isJson) {
              jsonBadge = `<button type="button" onclick="event.stopPropagation(); openJsonCellModal(${rIdx}, ${cIdx})" class="ml-1 opacity-0 group-hover:opacity-100 hover:text-amber-300 text-amber-400 font-mono font-bold transition text-xs" title="Open JSON Cell Viewer">{ }</button>`;
            }
            const isCellSelected = rIdx === activeGridSelectedRow && cIdx === activeGridSelectedCol;
            let cellClass = isDirty
              ? `p-1 px-3 border border-amber-600/60 bg-amber-950/40 text-amber-200 ${typeStyle.align} min-w-[90px] max-w-md truncate whitespace-nowrap grid-cell group select-text cursor-default transition h-8 max-h-8`
              : `p-1 px-3 border-r border-slate-800/40 ${typeStyle.color} ${typeStyle.align} min-w-[90px] max-w-md truncate whitespace-nowrap grid-cell group select-text cursor-default transition h-8 max-h-8`;
            if (isCellSelected) {
              cellClass += ' cell-selected ring-1 ring-sky-400 bg-sky-950/20';
            }
            return `
              <td class="${cellClass}"
                  data-row="${rIdx}" data-col="${cIdx}"
                  data-pk="${escapeHtml(virtualGridPKCol || '')}" data-table="${escapeHtml(virtualGridTable || '')}"
                  data-is-null="${isNullVal ? 'true' : 'false'}"
                  data-raw-val="${escapeHtml(rawValStr)}"
                  onclick="selectGridCell(this, ${rIdx}, ${cIdx})"
                  ondblclick="startInlineEdit(this, ${rIdx}, ${cIdx}, this.dataset.pk, this.dataset.table)">
                <span class="cell-val">${displayVal === null ? '<span class="text-slate-500 italic">NULL</span>' : escapeHtml(String(displayVal))}</span>
                ${fkBadge}
                ${jsonBadge}
              </td>
            `;
          }).join('')}
        </tr>
        `;
      }).join('');

      if (bottomPadding > 0) {
        html += `<tr style="height: ${bottomPadding}px; pointer-events: none;"><td colspan="${colSpan}" style="padding:0; border:0; height:${bottomPadding}px;"></td></tr>`;
      }

      tbody.innerHTML = html;

      // ponytail: Re-bind activeGridSelectedTd if selected cell is in recycled viewport, otherwise nullify to avoid retaining detached DOM node
      if (activeGridSelectedRow !== null && activeGridSelectedCol !== null) {
        const selectedTd = tbody.querySelector(`td[data-row="${activeGridSelectedRow}"][data-col="${activeGridSelectedCol}"]`);
        activeGridSelectedTd = selectedTd || null;
      } else {
        activeGridSelectedTd = null;
      }
    }

    function initVirtualGridScroll() {
      const wrapper = document.getElementById('virtual-grid-wrapper');
      if (wrapper && !wrapper.dataset.virtualScrollBound) {
        wrapper.dataset.virtualScrollBound = 'true';
        let ticking = false;
        wrapper.addEventListener('scroll', () => {
          if (isVirtualGridActive && !ticking) {
            ticking = true;
            window.requestAnimationFrame(() => {
              renderVirtualWindow();
              ticking = false;
            });
          }
        }, { passive: true });
      }
    }

    function renderGridResults(cols, rows, pkCol = null, tableName = null, databaseName = null, isFilterPass = false) {
      currentColumns = cols || [];
      currentResults = rows || [];
      const activeTab = tabs.find(t => t.id === activeTabId);

      if (activeTab && !activeTab.isTableViewer && !isFilterPass) {
        activeTab.rawQueryRows = rows || [];
        activeTab.rawQueryCols = cols || [];
      }

      if (!tableName && activeTab && activeTab.tableName) {
        tableName = activeTab.tableName;
      }
      if (!tableName && activeTab && !activeTab.isTableViewer) {
        const sql = activeTab.sql || getEditorSQL();
        const parsed = extractSingleTableFromSQL(sql);
        if (parsed && parsed.table) {
          tableName = parsed.table;
          if (parsed.database) databaseName = parsed.database;
        }
      }
      activeGridTable = tableName;
      activeGridDB = databaseName || (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';

      // Fallback: resolve primary key from schema catalog cache if not provided or empty
      const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === (tableName || '').toLowerCase());
      if (!pkCol && tableMeta && Array.isArray(tableMeta.columns)) {
        const pks = tableMeta.columns.filter(c => c.is_primary_key || c.key === 'PRI');
        if (pks.length === 1) pkCol = pks[0].name;
      }

      activeGridPKCol = pkCol;
      activeEditCell = null;
      activeGridSelectedTd = null;
      updateColumnPickerBadge();

      const thead = document.getElementById('grid-thead');
      const tbody = document.getElementById('grid-tbody');
      updateGridEditModeUI();
      updateStagedEditsUI();
      updateSelectedRowsUI();
      syncGridToolbarMode();

      if (!cols || cols.length === 0) {
        const table = document.getElementById('results-table');
        if (table) {
          table.classList.add('table-empty');
          table.style.width = '100%';
        }
        thead.innerHTML = '<tr><th class="p-2.5 text-center text-slate-400 font-semibold w-full">No data</th></tr>';
        tbody.innerHTML = '<tr><td class="p-12 text-center text-slate-500 w-full">Zero records returned.</td></tr>';
        return;
      }
      const table = document.getElementById('results-table');
      if (table) {
        table.classList.remove('table-empty');
        table.style.width = '';
        table.style.minWidth = '';
      }

      const isTableViewer = Boolean(activeTab && activeTab.isTableViewer);
      const pkInCols = Boolean(pkCol && cols && cols.some(c => (c.name || '').toLowerCase() === pkCol.toLowerCase()));
      const hasCheckbox = isTableViewer || Boolean(tableName && pkCol && pkInCols);

      // Render Headers (Row 1: Names & Sort, Row 2: Subheader Filters)
      let headerRow1 = '<tr>';
      headerRow1 += `<th class="p-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800 bg-slate-900/80 select-none">#</th>`;
      if (hasCheckbox) {
        const allSelected = currentResults.length > 0 && selectedRowIndices.size === currentResults.length;
        headerRow1 += `<th class="p-2 w-8 text-center border-r border-slate-800 bg-slate-900/80 select-none">
          <input type="checkbox" id="grid-select-all" ${allSelected ? 'checked' : ''} onchange="toggleSelectAllRows(this.checked)" class="accent-sky-500 w-3.5 h-3.5 cursor-pointer" title="Select all rows">
        </th>`;
      }
      headerRow1 += cols.map((c, i) => {
        if (isColumnHidden(c.name, tableName)) return '';
        let sortIcon = '';
        if (activeTab && activeTab.sortCol === c.name) {
          sortIcon = activeTab.sortDir === 'DESC' ? ' <span class="text-sky-400 font-bold">▼</span>' : ' <span class="text-sky-400 font-bold">▲</span>';
        }
        return `
        <th class="p-2 px-3 font-semibold text-xs border-r border-slate-800 cursor-pointer hover:text-white select-none transition min-w-[90px] max-w-md whitespace-nowrap" onclick="sortColumn(${i})" title="Click to sort by ${escapeHtml(c.name)}">
          <div class="flex items-center justify-between space-x-2">
            <span class="truncate">${escapeHtml(c.name)}${sortIcon}</span>
            <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-1.5">${c.type}</span>
          </div>
        </th>
      `;}).join('') + '</tr>';

      let headerRow2 = '<tr class="bg-slate-950/90 border-b border-slate-800">';
      headerRow2 += '<th class="p-1 w-10 text-center text-slate-600 border-r border-slate-800 text-[10px] font-mono select-none">#</th>';
      if (hasCheckbox) {
        headerRow2 += '<th class="p-1 w-8 text-center text-slate-500 border-r border-slate-800 text-[10px]">🔍</th>';
      }
      headerRow2 += cols.map(c => {
        if (isColumnHidden(c.name, tableName)) return '';
        let initialVal = '';
        if (activeTab?.isTableViewer) {
          const existingPill = (activeTab?.filterPills || []).find(p => (p.col || p.column) === c.name);
          if (existingPill) {
            const pOp = existingPill.op || existingPill.operator || '=';
            const pVal = existingPill.val !== undefined ? existingPill.val : (existingPill.value !== undefined ? existingPill.value : '');
            if (pOp === '=' && !String(pVal).includes('%')) {
              initialVal = String(pVal);
            } else if (pOp === 'LIKE' && String(pVal).startsWith('%') && String(pVal).endsWith('%') && String(pVal).length >= 2) {
              initialVal = String(pVal);
            } else if (pOp === 'IS NULL' || pOp === 'IS NOT NULL') {
              initialVal = pOp;
            } else {
              initialVal = `${pOp} ${pVal}`.trim();
            }
          }
        } else {
          initialVal = (activeTab?.queryFilters && activeTab.queryFilters[c.name]) || '';
        }
        return `
        <th class="p-1 px-1.5 border-r border-slate-800 min-w-[90px] max-w-md">
          <input type="text" value="${escapeHtml(initialVal)}" class="col-filter-input w-full bg-slate-900 text-slate-200 border border-slate-700/80 rounded px-1.5 py-0.5 text-[11px] font-mono outline-none focus:border-sky-500 transition" placeholder="Filter..." data-col="${escapeHtml(c.name)}" onkeydown="handleColFilterKeydown(event, this)">
        </th>
      `;}).join('') + '</tr>';
      thead.innerHTML = headerRow1 + headerRow2;

      // Resolve foreign keys for table
      const fkMap = new Map();
      if (tableMeta && Array.isArray(tableMeta.foreign_keys)) {
        tableMeta.foreign_keys.forEach(fk => {
          if (fk.column && fk.referenced_table && fk.referenced_column) {
            fkMap.set(fk.column, fk);
          }
        });
      }

      // ponytail: Client-side row windowing / pagination for query results to prevent DOM bloat and main-thread freeze on large result sets (>50 rows).
      // Known ceiling: Client-side slice of in-memory rows. Upgrade path: Server-side cursor keyset pagination if queries exceed 1,000 rows.
      const QUERY_PAGE_SIZE = getGridActivePageSize(activeTab);
      const isPagedQuery = Boolean(activeTab && !activeTab.isTableViewer && rows && rows.length > QUERY_PAGE_SIZE);
      const queryPage = (activeTab && typeof activeTab.queryPage === 'number') ? activeTab.queryPage : 0;
      const startIdx = isPagedQuery ? queryPage * QUERY_PAGE_SIZE : 0;
      const endIdx = isPagedQuery ? Math.min(startIdx + QUERY_PAGE_SIZE, rows.length) : (rows ? rows.length : 0);
      const pagedRows = isPagedQuery ? rows.slice(startIdx, endIdx) : (rows || []);

      // Update grid result stats and pagination
      const gridResultStats = document.getElementById('grid-result-stats');
      if (gridResultStats) {
        if (activeTab && activeTab.isTableViewer) {
          const off = activeTab.offset || 0;
          if (!rows || rows.length === 0) {
            gridResultStats.textContent = `0 rows${activeTab.appliedFilter ? ' (filtered)' : ''}`;
          } else {
            gridResultStats.textContent = `${off + 1} - ${off + rows.length} rows${activeTab.appliedFilter ? ' (filtered)' : ''}`;
          }
        } else if (isPagedQuery) {
          gridResultStats.textContent = `${startIdx + 1} - ${endIdx} of ${rows.length} rows${isFilterPass && activeTab?.rawQueryRows && rows.length !== activeTab.rawQueryRows.length ? ' (filtered)' : ''}`;
        } else {
          gridResultStats.textContent = `${rows ? rows.length : 0} rows${isFilterPass && activeTab?.rawQueryRows && rows.length !== activeTab.rawQueryRows.length ? ' (filtered)' : ''}`;
        }
      }


      // Update Bottom Pagination Bar UI
      renderGridPaginationBar(activeTab, rows, isFilterPass);

      if (activeTab && activeTab.isTableViewer) {
        syncFilterBarUI(activeTab);
      }

      // Render Draft Rows (if any)
      let draftRowsHTML = '';
      if (hasCheckbox && stagedDraftRows.length > 0) {
        draftRowsHTML = stagedDraftRows.map((draft, dIdx) => `
          <tr class="bg-sky-950/40 border-b border-sky-800/40 text-sky-200 draft-row" data-draft-idx="${dIdx}">
            <td class="p-1.5 w-10 text-center text-sky-400 font-mono font-bold text-xs border-r border-slate-800/40 select-none" title="Draft new row">+</td>
            <td class="p-1.5 w-8 text-center border-r border-slate-800/40">
              <button onclick="removeGridDraftRow(${dIdx})" class="text-rose-400 hover:text-rose-200 text-xs font-bold" title="Discard this draft row"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
            </td>
            ${cols.map(c => {
              if (isColumnHidden(c.name, tableName)) return '';
              const val = draft.data[c.name] ?? '';
              const isAutoPK = c.name === pkCol && (c.extra?.includes('auto') || c.type?.toLowerCase().includes('int'));
              const typeStyle = getGridTypeStyle(c.type);
              return `
                <td class="p-1 px-2 border-r border-slate-800/40 min-w-[90px] max-w-md ${typeStyle.align}">
                  <input type="text" value="${escapeHtml(String(val))}" placeholder="${isAutoPK ? '(auto)' : escapeHtml(c.name)}"
                         data-col="${escapeHtml(c.name)}" data-draft-idx="${dIdx}"
                         oninput="updateGridDraftCell(${dIdx}, this.dataset.col, this.value)"
                         class="w-full bg-slate-950 ${typeStyle.inputColor} ${typeStyle.inputAlign} border border-sky-500/50 rounded px-1.5 py-0.5 text-xs font-mono outline-none focus:border-sky-400 shadow-inner">
                </td>
              `;
            }).join('')}
          </tr>
        `).join('');
      }

      // Render Regular Rows
      const visibleCols = cols.filter(c => !isColumnHidden(c.name, tableName));
      if ((!rows || rows.length === 0) && stagedDraftRows.length === 0) {
        const colSpan = visibleCols.length + 1 + (hasCheckbox ? 1 : 0);
        tbody.innerHTML = `<tr><td colspan="${colSpan}" class="p-12 text-center text-slate-500 w-full">Zero rows returned.</td></tr>`;
        return;
      }

      const colMetaMap = new Map();
      (tableMeta?.columns || []).forEach(c => {
        if (c && c.name) colMetaMap.set(c.name.toLowerCase(), c);
      });

      // Activate Virtual Data Grid Windowing Engine for datasets > 50 rows (ADR 0058)
      if (pagedRows.length > 50 && stagedDraftRows.length === 0) {
        isVirtualGridActive = true;
        virtualGridRows = pagedRows;
        virtualGridCols = cols;
        virtualGridPKCol = pkCol;
        virtualGridTable = tableName;
        virtualGridDB = activeGridDB;
        virtualGridHasCheckbox = hasCheckbox;
        virtualGridFkMap = fkMap;
        virtualGridColMetaMap = colMetaMap;
        virtualGridStartIdx = (activeTab && activeTab.isTableViewer) ? (activeTab.offset || 0) : (isPagedQuery ? startIdx : 0);
        initVirtualGridScroll();
        renderVirtualWindow();
        return;
      }
      isVirtualGridActive = false;
      virtualGridStartIdx = (activeTab && activeTab.isTableViewer) ? (activeTab.offset || 0) : 0;

      tbody.innerHTML = draftRowsHTML + pagedRows.map((row, pIdx) => {
        const rIdx = isPagedQuery ? (startIdx + pIdx) : pIdx;
        const rowNum = (activeTab && activeTab.isTableViewer) ? ((activeTab.offset || 0) + pIdx + 1) : (isPagedQuery ? (startIdx + pIdx + 1) : (pIdx + 1));
        const isSelected = selectedRowIndices.has(rIdx);
        const gutterCell = `<td class="p-1.5 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${rowNum}</td>`;
        let checkboxCell = '';
        if (hasCheckbox) {
          checkboxCell = `<td class="p-1.5 w-8 text-center border-r border-slate-800/40"><input type="checkbox" class="grid-row-checkbox accent-sky-500 w-3.5 h-3.5 cursor-pointer" data-row="${rIdx}" ${isSelected ? 'checked' : ''} onchange="toggleSelectRow(${rIdx}, this.checked)"></td>`;
        }
        return `
        <tr class="hover:bg-slate-800/40 transition ${isSelected ? 'bg-sky-950/20' : ''}" oncontextmenu="openGridContextMenu(event, ${rIdx})">
          ${gutterCell}
          ${checkboxCell}
          ${row.map((val, cIdx) => {
            if (isColumnHidden(cols[cIdx]?.name, tableName)) return '';
            const stagedKey = stagedKeyFor(tableName || '', rIdx, cIdx);
            const staged = stagedEdits.get(stagedKey);
            const rawVal = staged ? staged.newVal : val;
            const isDirty = Boolean(staged);
            const colName = cols[cIdx]?.name;
            const colType = cols[cIdx]?.type;
            const colMeta = colName ? colMetaMap.get(colName.toLowerCase()) : null;
            const resolvedType = colMeta?.type || colType;
            const displayVal = formatGridCellValue(rawVal, resolvedType);
            let typeStyle = getGridTypeStyle(colType);
            if (colMeta?.type) typeStyle = getGridTypeStyle(colMeta.type);
            const fkInfo = fkMap.get(colName);
            let fkBadge = '';
            if (fkInfo && displayVal !== null && displayVal !== '') {
              fkBadge = `<button type="button" data-table="${escapeHtml(tableName || '')}" data-col="${escapeHtml(colName)}" data-reftable="${escapeHtml(fkInfo.referenced_table)}" data-refcol="${escapeHtml(fkInfo.referenced_column)}" data-val="${escapeHtml(String(displayVal))}" onclick="event.stopPropagation(); peekForeignKey(event, this.dataset.table, this.dataset.col, this.dataset.reftable, this.dataset.refcol, this.dataset.val)" class="ml-1 opacity-0 group-hover:opacity-100 hover:text-sky-300 text-sky-400 font-bold transition text-xs inline-flex items-center" title="Peek referenced record (${escapeHtml(fkInfo.referenced_table)}.${escapeHtml(fkInfo.referenced_column)})">${renderIcon('link', 'w-3 h-3')}</button>`;
            }
            const isJson = displayVal !== null && typeof displayVal === 'string' && (typeStyle.category === 'json' || (displayVal.trim().startsWith('{') && displayVal.trim().endsWith('}')) || (displayVal.trim().startsWith('[') && displayVal.trim().endsWith(']')));
            let jsonBadge = '';
            if (isJson) {
              jsonBadge = `<button type="button" onclick="event.stopPropagation(); openJsonCellModal(${rIdx}, ${cIdx})" class="ml-1 opacity-0 group-hover:opacity-100 hover:text-amber-300 text-amber-400 font-mono font-bold transition text-xs" title="Open JSON Cell Viewer">{ }</button>`;
            }
            const cellClass = isDirty
              ? `p-1.5 px-3 border border-amber-600/60 bg-amber-950/40 text-amber-200 ${typeStyle.align} min-w-[90px] max-w-md truncate whitespace-nowrap grid-cell group select-text cursor-default transition`
              : `p-1.5 px-3 border-r border-slate-800/40 ${typeStyle.color} ${typeStyle.align} min-w-[90px] max-w-md truncate whitespace-nowrap grid-cell group select-text cursor-default transition`;
            return `
              <td class="${cellClass}"
                  data-row="${rIdx}" data-col="${cIdx}"
                  data-pk="${escapeHtml(pkCol || '')}" data-table="${escapeHtml(tableName || '')}"
                  onclick="selectGridCell(this, ${rIdx}, ${cIdx})"
                  ondblclick="startInlineEdit(this, ${rIdx}, ${cIdx}, this.dataset.pk, this.dataset.table)">
                <span class="cell-val">${displayVal === null ? '<span class="text-slate-500 italic">NULL</span>' : escapeHtml(String(displayVal))}</span>
                ${fkBadge}
                ${jsonBadge}
                ${isDirty ? '<span class="ml-1 text-[9px] text-amber-400 font-bold" title="Unapplied staged change">●</span>' : ''}
                <button type="button" onclick="event.stopPropagation(); openGridCellMenu(event, ${rIdx}, ${cIdx}, this.parentElement)" class="ml-1 opacity-0 group-hover:opacity-70 hover:!opacity-100 text-slate-400 hover:text-sky-300 font-bold transition text-[10px] select-none inline-flex items-center" title="Insert Value / Cell Menu (Right-click)">${renderIcon('edit', 'w-3 h-3')}</button>
              </td>
            `;
          }).join('')}
        </tr>
      `;}).join('');
    }

    function selectGridCell(td, rIdx, cIdx) {
      if (activeEditCell) {
        if (activeEditCell.td === td) return;
        commitInlineEdit(activeEditCell, true);
      }
      if (activeGridSelectedTd && activeGridSelectedTd !== td) {
        activeGridSelectedTd.classList.remove('cell-selected', 'ring-1', 'ring-sky-400', 'bg-sky-950/20');
      }
      activeGridSelectedRow = rIdx;
      activeGridSelectedCol = cIdx;
      activeGridSelectedTd = td;

      document.querySelectorAll('#grid-tbody td.cell-selected').forEach(el => {
        if (el !== td) el.classList.remove('cell-selected', 'ring-1', 'ring-sky-400', 'bg-sky-950/20');
      });
      td.classList.add('cell-selected', 'ring-1', 'ring-sky-400', 'bg-sky-950/20');
      syncCellDetailDrawer(td, rIdx, cIdx);
    }

    // ponytail: Keyboard arrow navigation across data grid cells live-synced with Cell Detail Drawer (ADR 0058)
    function navigateGridCellByArrow(key) {
      if (activeGridSelectedRow === null || activeGridSelectedCol === null) return;
      const totalRows = (currentResults && currentResults.length) || 0;
      if (totalRows === 0) return;

      const cols = currentColumns || [];
      const visibleColIndices = [];
      cols.forEach((c, idx) => {
        if (!isColumnHidden(c.name, activeGridTable)) {
          visibleColIndices.push(idx);
        }
      });
      if (visibleColIndices.length === 0) return;

      let newRow = activeGridSelectedRow;
      let newCol = activeGridSelectedCol;

      const currentVisIdx = visibleColIndices.indexOf(activeGridSelectedCol);
      const safeVisIdx = currentVisIdx >= 0 ? currentVisIdx : 0;

      if (key === 'ArrowUp') {
        newRow = Math.max(0, activeGridSelectedRow - 1);
      } else if (key === 'ArrowDown') {
        newRow = Math.min(totalRows - 1, activeGridSelectedRow + 1);
      } else if (key === 'ArrowLeft') {
        if (safeVisIdx > 0) {
          newCol = visibleColIndices[safeVisIdx - 1];
        }
      } else if (key === 'ArrowRight') {
        if (safeVisIdx < visibleColIndices.length - 1) {
          newCol = visibleColIndices[safeVisIdx + 1];
        }
      }

      if (newRow === activeGridSelectedRow && newCol === activeGridSelectedCol) return;

      activeGridSelectedRow = newRow;
      activeGridSelectedCol = newCol;

      const wrapper = document.getElementById('virtual-grid-wrapper');
      if (isVirtualGridActive && wrapper) {
        const targetTop = (newRow - (virtualGridStartIdx || 0)) * VIRTUAL_ROW_HEIGHT;
        const currentScroll = wrapper.scrollTop;
        const clientHeight = wrapper.clientHeight || 500;
        if (targetTop < currentScroll) {
          wrapper.scrollTop = targetTop;
        } else if (targetTop + VIRTUAL_ROW_HEIGHT > currentScroll + clientHeight) {
          wrapper.scrollTop = targetTop + VIRTUAL_ROW_HEIGHT - clientHeight;
        }
        renderVirtualWindow();
      }

      const tbody = document.getElementById('grid-tbody');
      const targetTd = tbody ? tbody.querySelector(`td[data-row="${newRow}"][data-col="${newCol}"]`) : null;
      if (targetTd) {
        selectGridCell(targetTd, newRow, newCol);
        if (typeof targetTd.scrollIntoView === 'function') {
          targetTd.scrollIntoView({ block: 'nearest', inline: 'nearest' });
        }
      } else {
        syncCellDetailDrawer(null, newRow, newCol);
      }
    }

    // ponytail: heuristic data type categorization for data grid syntax coloring and alignment.
    // Known ceiling: does not parse nested composite types (e.g. Postgres arrays of timestamps). Upgrade path: dialect-aware token parser.
    function getGridTypeCategory(typeStr) {
      if (!typeStr || typeof typeStr !== 'string') return 'text';
      const t = typeStr.trim().toLowerCase();
      if (t === 'json' || t === 'jsonb') {
        return 'json';
      }
      if (t === 'bool' || t === 'boolean' || t.startsWith('tinyint(1)')) {
        return 'boolean';
      }
      if (/^(date|time|datetime|timestamp|timestamptz|timetz|year|interval)/i.test(t)) {
        return 'temporal';
      }
      if (/^(u?int|u?bigint|u?smallint|u?mediumint|u?tinyint|serial|bigserial|smallserial|float|double|decimal|numeric|real|bit|money|dec|fixed)/i.test(t) ||
          /\b(int|integer|float|double|decimal|numeric|real)\b/i.test(t)) {
        return 'numeric';
      }
      if (/^(blob|tinyblob|mediumblob|longblob|binary|varbinary|bytea|raw)/i.test(t)) {
        return 'binary';
      }
      return 'text';
    }

    function getGridTypeStyle(typeStr) {
      const cat = getGridTypeCategory(typeStr);
      switch (cat) {
        case 'json':
          return { color: 'text-amber-300', align: 'text-left', inputAlign: 'text-left', inputColor: 'text-amber-300', category: 'json' };
        case 'numeric':
          return { color: 'text-emerald-400', align: 'text-right', inputAlign: 'text-right', inputColor: 'text-emerald-400', category: 'numeric' };
        case 'temporal':
          return { color: 'text-purple-400', align: 'text-right', inputAlign: 'text-right', inputColor: 'text-purple-400', category: 'temporal' };
        case 'boolean':
          return { color: 'text-rose-400', align: 'text-left', inputAlign: 'text-left', inputColor: 'text-rose-400', category: 'boolean' };
        case 'binary':
          return { color: 'text-amber-400', align: 'text-left', inputAlign: 'text-left', inputColor: 'text-amber-400', category: 'binary' };
        default:
          return { color: 'text-slate-200', align: 'text-left', inputAlign: 'text-left', inputColor: 'text-sky-200', category: 'text' };
      }
    }

    function parseEnumValues(typeStr) {
      if (!typeStr || typeof typeStr !== 'string') return null;
      const match = typeStr.match(/^(?:enum|set)\s*\(([\s\S]+)\)$/i);
      if (!match) return null;
      const inner = match[1];
      const values = [];
      const regex = /'((?:''|[^'])*)'|"((?:""|[^"])*)"/g;
      let m;
      while ((m = regex.exec(inner)) !== null) {
        values.push((m[1] !== undefined ? m[1].replace(/''/g, "'") : m[2].replace(/""/g, '"')));
      }
      return values.length > 0 ? values : null;
    }

    function formatCurrentTimestamp() {
      const d = new Date();
      const pad = (n) => String(n).padStart(2, '0');
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
    }

    function formatCurrentDate() {
      const d = new Date();
      const pad = (n) => String(n).padStart(2, '0');
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    }

    // ponytail: Format cell values according to data type semantics.
    // Specifically formats DATE as strictly YYYY-MM-DD, DATETIME as YYYY-MM-DD HH:mm:ss, TIME as HH:mm:ss.
    function formatGridCellValue(val, typeStr) {
      if (val === null || val === undefined) return null;
      if (typeof val !== 'string' && typeof val !== 'number' && !(val instanceof Date)) {
        return val;
      }
      const s = String(val).trim();
      if (!s) return s;
      const t = (typeStr || '').trim().toLowerCase();

      // 1. DATE type: strictly date only (YYYY-MM-DD), no hours/minutes/seconds
      if (t === 'date' || t.startsWith('date(')) {
        const m = s.match(/^(\d{4}-\d{2}-\d{2})/);
        if (m) return m[1];
        const d = new Date(s);
        if (!isNaN(d.getTime())) {
          const pad = (n) => String(n).padStart(2, '0');
          return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
        }
        return s;
      }

      // 2. DATETIME / TIMESTAMP types: clean SQL format YYYY-MM-DD HH:mm:ss
      if (/^(datetime|timestamp|timestamptz)/i.test(t)) {
        const m = s.match(/^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})/);
        if (m) return `${m[1]} ${m[2]}`;
        const d = new Date(s);
        if (!isNaN(d.getTime())) {
          const pad = (n) => String(n).padStart(2, '0');
          return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
        }
        return s;
      }

      // 3. TIME types: HH:mm:ss
      if (/^(time|timetz)/i.test(t)) {
        const m = s.match(/(\d{2}:\d{2}:\d{2})/);
        if (m) return m[1];
        return s;
      }

      // 4. Default fallback: if string is an ISO timestamp with T00:00:00Z, clean it
      if (s.includes('T') && s.endsWith('Z')) {
        const m = s.match(/^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}:\d{2})/);
        if (m) {
          return m[2] === '00:00:00' ? m[1] : `${m[1]} ${m[2]}`;
        }
      }

      return s;
    }

    // ponytail: Edge-case-correct days in month calculation using native Date.
    function getDaysInMonth(year, month) {
      // month is 1-indexed (1 = Jan, 12 = Dec). Day 0 of next month is last day of current month.
      return new Date(year, month, 0).getDate();
    }

    // ponytail: Dynamic segment stepping for date/datetime/time based on caret/selection position.
    // Handles leap years, month boundaries, and auto-clamps days (e.g. 2026-01-31 + 1 month -> 2026-02-28).
    function stepActiveDatePart(delta) {
      if (!activeEditCell || !activeEditCell.input) return;
      const inp = activeEditCell.input;
      const cur = inp.value.trim();
      const rawType = (activeEditCell.rawType || '').toLowerCase();
      const isTimeOnly = rawType === 'time';
      const isDatetime = !isTimeOnly && /^(datetime|timestamp|timestamptz)/i.test(rawType);
      const pad = (n) => String(n).padStart(2, '0');

      let year = 2026, month = 1, day = 1, hours = 0, minutes = 0, seconds = 0;
      const now = new Date();

      if (isTimeOnly) {
        hours = now.getHours();
        minutes = now.getMinutes();
        seconds = now.getSeconds();
        const tm = cur.match(/^(\d{2}):(\d{2})(?::(\d{2}))?/);
        if (tm) {
          hours = parseInt(tm[1], 10);
          minutes = parseInt(tm[2], 10);
          if (tm[3]) seconds = parseInt(tm[3], 10);
        }
      } else {
        year = now.getFullYear();
        month = now.getMonth() + 1;
        day = now.getDate();
        hours = now.getHours();
        minutes = now.getMinutes();
        seconds = now.getSeconds();
        const m = cur.match(/^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2})(?::(\d{2}))?)?/);
        if (m) {
          year = parseInt(m[1], 10);
          month = parseInt(m[2], 10);
          day = parseInt(m[3], 10);
          if (m[4]) hours = parseInt(m[4], 10);
          if (m[5]) minutes = parseInt(m[5], 10);
          if (m[6]) seconds = parseInt(m[6], 10);
        }
      }

      // Detect caret / selection segment
      // YYYY-MM-DD HH:mm:ss
      // 0123 56 89 12 45 78
      const pos = (inp.selectionStart !== undefined && inp.selectionStart !== null)
        ? inp.selectionStart
        : (activeEditCell.lastPartPos !== undefined ? activeEditCell.lastPartPos : 8);

      let part = 'day';
      let selRange = [8, 10]; // default day

      if (isTimeOnly) {
        if (pos <= 2) {
          part = 'hour';
          selRange = [0, 2];
        } else if (pos <= 5) {
          part = 'minute';
          selRange = [3, 5];
        } else {
          part = 'second';
          selRange = [6, 8];
        }
      } else {
        if (pos <= 4) {
          part = 'year';
          selRange = [0, 4];
        } else if (pos <= 7) {
          part = 'month';
          selRange = [5, 7];
        } else if (pos <= 10) {
          part = 'day';
          selRange = [8, 10];
        } else if (pos <= 13) {
          part = 'hour';
          selRange = [11, 13];
        } else if (pos <= 16) {
          part = 'minute';
          selRange = [14, 16];
        } else {
          part = 'second';
          selRange = [17, 19];
        }
      }

      // Step targeted part
      if (part === 'year') {
        year += delta;
        const maxD = getDaysInMonth(year, month);
        if (day > maxD) day = maxD;
      } else if (part === 'month') {
        month += delta;
        while (month > 12) {
          month -= 12;
          year += 1;
        }
        while (month < 1) {
          month += 12;
          year -= 1;
        }
        const maxD = getDaysInMonth(year, month);
        if (day > maxD) day = maxD;
      } else if (part === 'day') {
        const dObj = new Date(year, month - 1, day + delta, hours, minutes, seconds);
        year = dObj.getFullYear();
        month = dObj.getMonth() + 1;
        day = dObj.getDate();
        hours = dObj.getHours();
        minutes = dObj.getMinutes();
        seconds = dObj.getSeconds();
      } else if (part === 'hour') {
        hours = (hours + delta) % 24;
        if (hours < 0) hours += 24;
      } else if (part === 'minute') {
        minutes = (minutes + delta) % 60;
        if (minutes < 0) minutes += 60;
      } else if (part === 'second') {
        seconds = (seconds + delta) % 60;
        if (seconds < 0) seconds += 60;
      }

      // Format updated string
      if (isTimeOnly) {
        inp.value = `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
      } else if (isDatetime) {
        inp.value = `${year}-${pad(month)}-${pad(day)} ${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
      } else {
        inp.value = `${year}-${pad(month)}-${pad(day)}`;
      }

      activeEditCell.lastPartPos = selRange[0];
      if (typeof inp.setSelectionRange === 'function') {
        try {
          inp.setSelectionRange(selRange[0], selRange[1]);
        } catch (_) {}
      }

      if (typeof Event !== 'undefined' && typeof inp.dispatchEvent === 'function') {
        inp.dispatchEvent(new Event('input', { bubbles: true }));
      }
      if (typeof inp.focus === 'function') inp.focus();
    }

    function stepActiveDate(delta) {
      return stepActiveDatePart(delta);
    }

    function generateUUID() {
      if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        try { return crypto.randomUUID(); } catch (_) {}
      }
      return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
        const r = (Math.random() * 16) | 0;
        const v = c === 'x' ? r : (r & 0x3) | 0x8;
        return v.toString(16);
      });
    }

    function openGridCellMenu(e, rIdx, cIdx, td) {
      e.preventDefault();
      e.stopPropagation();
      selectGridCell(td, rIdx, cIdx);
      openGridContextMenu(e, rIdx);
    }

    function editRightClickedCell() {
      const td = activeGridRightClickedTd || activeGridSelectedTd;
      const rIdx = activeGridRightClickedRow ?? activeGridSelectedRow;
      const cIdx = activeGridRightClickedCol ?? activeGridSelectedCol;
      if (td && rIdx !== null && cIdx !== null) {
        startInlineEdit(td, rIdx, cIdx, activeGridPKCol, activeGridTable);
      }
    }

    async function applyPresetToActiveEdit(valType) {
      if (!activeEditCell) return;
      let val = null;
      if (valType === null) val = null;
      else if (valType === '') val = '';
      else if (valType === 'NOW') val = formatCurrentTimestamp();
      else if (valType === 'DATE') val = formatCurrentDate();
      else if (valType === 'UUID') val = generateUUID();
      else val = valType;

      activeEditCell.hasExplicitVal = true;
      activeEditCell.explicitVal = val;
      if (activeEditCell.input) {
        activeEditCell.input.value = val === null ? '' : String(val);
      }
      await commitInlineEdit(activeEditCell, false);
    }

    async function insertGridCellValue(valType) {
      closeGridContextMenu();
      const rIdx = activeGridRightClickedRow ?? activeGridSelectedRow;
      const cIdx = activeGridRightClickedCol ?? activeGridSelectedCol;
      const td = activeGridRightClickedTd ?? activeGridSelectedTd;

      if (rIdx === null || cIdx === null || !td) return;
      if (activeSession && activeSession.read_only) {
        showNativeAlert('Session is in READ-ONLY mode. Unlock first.', 'Read-Only Mode');
        return;
      }

      let val = null;
      if (valType === null) val = null;
      else if (valType === '') val = '';
      else if (valType === 'NOW') val = formatCurrentTimestamp();
      else if (valType === 'DATE') val = formatCurrentDate();
      else if (valType === 'UUID') val = generateUUID();
      else val = valType;

      if (activeEditCell && activeEditCell.td === td) {
        await applyPresetToActiveEdit(valType);
        return;
      }

      const activeTab = tabs.find(t => t.id === activeTabId);
      let tableName = activeGridTable || (activeTab && activeTab.tableName) || '';
      if (!tableName) {
        const sql = (activeTab && activeTab.sql) || getEditorSQL();
        const parsed = extractSingleTableFromSQL(sql);
        if (parsed && parsed.table) {
          tableName = parsed.table;
          activeGridTable = tableName;
          if (parsed.database) activeGridDB = parsed.database;
        }
      }
      if (!tableName) {
        showNativeAlert('Inline editing is only available for single-table queries. Specify a table to edit.', 'Edit Cell');
        return;
      }

      let pkCol = activeGridPKCol;
      if (!pkCol && tableName) {
        const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
        const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === tableName.toLowerCase());
        if (tableMeta && Array.isArray(tableMeta.columns)) {
          const pks = tableMeta.columns.filter(c => c.is_primary_key);
          if (pks.length === 1) {
            pkCol = pks[0].name;
            activeGridPKCol = pkCol;
          }
        }
      }

      if (activeTab && !activeTab.isTableViewer && !pkCol) {
        showNativeAlert(`Inline editing in query results requires a Primary Key on table '${tableName}'.`, 'Edit Cell');
        return;
      }

      const col = currentColumns[cIdx];
      const pkIdx = pkCol ? currentColumns.findIndex(c => (c.name || '').toLowerCase() === pkCol.toLowerCase()) : -1;
      const pkVal = pkIdx !== -1 ? currentResults[rIdx][pkIdx] : null;

      const rowCriteria = {};
      if (pkIdx === -1) {
        currentColumns.forEach((c, idx) => {
          rowCriteria[c.name] = currentResults[rIdx][idx];
        });
      }

      // ponytail: Prioritize activeGridDB over activeTab.database so cross-database updates hit the correct schema.
      const dbName = activeGridDB || (activeTab && activeTab.database) || '';

      if (gridEditMode === 'auto') {
        // The grid may be re-rendered (page/tab change) while awaiting: update the edited row object,
        // never "whatever is at index rIdx now".
        const rowRef = currentResults[rIdx];
        const stillShown = () => currentResults[rIdx] === rowRef && td && td.isConnected;
        if (gridConfirmSave) {
          const colName = col ? col.name : `Column #${cIdx}`;
          const displayVal = val === null ? 'NULL' : `"${val}"`;
          const ok = await showNativeConfirm(`Save changes to ${tableName}.${colName} = ${displayVal}?`, 'Confirm Save');
          if (!ok) return;
        }

        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/cell-edit`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              database: dbName,
              table: tableName,
              column: col.name,
              value: val,
              pk_column: pkIdx !== -1 ? pkCol : '',
              pk_value: pkVal,
              row_criteria: pkIdx === -1 ? rowCriteria : undefined
            })
          });
          if (res.ok) {
            if (rowRef) rowRef[cIdx] = val;
            if (stillShown()) {
              renderCellContent(td, rIdx, cIdx, pkCol, tableName);
              td.classList.add('bg-emerald-950/40', 'border-emerald-600/60');
              setTimeout(() => td.classList.remove('bg-emerald-950/40', 'border-emerald-600/60'), 1500);
            }
          } else {
            const err = await res.json().catch(() => ({}));
            showNativeAlert('Failed to save cell: ' + (err.error || res.statusText), 'Save Error');
            if (stillShown()) renderCellContent(td, rIdx, cIdx, pkCol, tableName);
          }
        } catch (err) {
          showNativeAlert('Network error saving cell: ' + err.message, 'Save Error');
        }
      } else {
        const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
        stageGridEdit(stagedKey, {
          rIdx,
          cIdx,
          db: dbName,
          table: tableName,
          colName: col.name,
          pkCol: pkIdx !== -1 ? pkCol : '',
          pkVal,
          rowCriteria: pkIdx === -1 ? rowCriteria : undefined,
          oldVal: currentResults[rIdx][cIdx],
          newVal: val
        });
        currentResults[rIdx][cIdx] = val;
        renderCellContent(td, rIdx, cIdx, pkCol, tableName);
        updateStagedEditsUI();
      }
    }

    function startInlineEdit(td, rIdx, cIdx, pkCol, tableName) {
      if (activeSession && activeSession.read_only) {
        showNativeAlert('Session is in READ-ONLY mode. Unlock first.', 'Read-Only Mode');
        return;
      }

      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!tableName) {
        tableName = activeGridTable || (activeTab && activeTab.tableName) || '';
        if (!tableName) {
          const sql = (activeTab && activeTab.sql) || getEditorSQL();
          const parsed = extractSingleTableFromSQL(sql);
          if (parsed && parsed.table) {
            tableName = parsed.table;
            activeGridTable = tableName;
            if (parsed.database) activeGridDB = parsed.database;
          }
        }
      }

      if (!pkCol) {
        pkCol = activeGridPKCol;
        if (!pkCol && tableName) {
          const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
          const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === tableName.toLowerCase());
          if (tableMeta && Array.isArray(tableMeta.columns)) {
            const pks = tableMeta.columns.filter(c => c.is_primary_key);
            if (pks.length === 1) {
              pkCol = pks[0].name;
              activeGridPKCol = pkCol;
            }
          }
        }
      }

      if (!tableName) {
        showNativeAlert('Inline editing is only available for single-table queries. Specify a table to edit.', 'Edit Cell');
        return;
      }

      if (activeTab && !activeTab.isTableViewer && !pkCol) {
        showNativeAlert(`Inline editing in query results requires a Primary Key on table '${tableName}'.`, 'Edit Cell');
        return;
      }

      if (activeEditCell) {
        if (activeEditCell.td === td) {
          if (activeEditCell.input) activeEditCell.input.focus();
          return;
        }
        commitInlineEdit(activeEditCell, true);
      }

      const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
      const staged = stagedEdits.get(stagedKey);
      const originalVal = currentResults[rIdx][cIdx];
      const currentVal = staged ? staged.newVal : originalVal;

      const col = currentColumns[cIdx];
      const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === (tableName || '').toLowerCase());
      const colMeta = (tableMeta?.columns || []).find(c => (c.name || '').toLowerCase() === (col?.name || '').toLowerCase());
      const rawType = (colMeta?.type || col?.type || '').toLowerCase();
      const isNullable = colMeta ? colMeta.is_nullable : true;
      const fkInfo = (tableMeta?.foreign_keys || []).find(fk => (fk.column || '').toLowerCase() === (col?.name || '').toLowerCase());
      const enumValues = parseEnumValues(colMeta?.type || col?.type);

      td.style.position = 'relative';
      const isTopRow = rIdx === 0;
      const pillPosClass = isTopRow ? 'top-full mt-1' : '-top-7';

      let editorHtml = '';
      if (enumValues && enumValues.length > 0) {
        editorHtml = `
          <div id="active-cell-preset-bar" class="absolute ${pillPosClass} left-0 z-30 bg-slate-900 border border-slate-700/80 rounded shadow-xl px-1.5 py-0.5 flex items-center space-x-1 text-[10px] select-none whitespace-nowrap ring-1 ring-black/40">
            <span class="text-sky-400 font-semibold mr-0.5">ENUM:</span>
            ${enumValues.slice(0, 5).map(v => `
              <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(${jsArg(v)})" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-200 hover:text-white rounded border border-slate-700 font-mono transition">${escapeHtml(v)}</button>
            `).join('')}
            ${isNullable ? `<button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(null)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-white rounded border border-slate-700 font-mono transition">NULL</button>` : ''}
          </div>
          <select class="w-full bg-slate-950 text-sky-200 border border-sky-400 px-1 py-0.5 rounded outline-none font-mono text-xs ring-2 ring-sky-500/40 shadow-inner cursor-pointer">
            ${isNullable ? `<option value="__SQLFOX_NULL__" ${currentVal === null ? 'selected' : ''} class="italic text-slate-500">NULL</option>` : ''}
            ${enumValues.map(v => `<option value="${escapeHtml(v)}" ${String(currentVal) === String(v) ? 'selected' : ''}>${escapeHtml(v)}</option>`).join('')}
          </select>
        `;
      } else if (rawType === 'boolean' || rawType === 'bool' || rawType === 'tinyint(1)') {
        editorHtml = `
          <div id="active-cell-preset-bar" class="absolute ${pillPosClass} left-0 z-30 bg-slate-900 border border-slate-700/80 rounded shadow-xl px-1.5 py-0.5 flex items-center space-x-1 text-[10px] select-none whitespace-nowrap ring-1 ring-black/40">
            <span class="text-sky-400 font-semibold mr-0.5">BOOL:</span>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('1')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-emerald-400 hover:text-emerald-300 rounded border border-slate-700 font-mono transition">1 (TRUE)</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('0')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-rose-400 hover:text-rose-300 rounded border border-slate-700 font-mono transition">0 (FALSE)</button>
            ${isNullable ? `<button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(null)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-white rounded border border-slate-700 font-mono transition">NULL</button>` : ''}
          </div>
          <select class="w-full bg-slate-950 text-sky-200 border border-sky-400 px-1 py-0.5 rounded outline-none font-mono text-xs ring-2 ring-sky-500/40 shadow-inner cursor-pointer">
            ${isNullable ? `<option value="__SQLFOX_NULL__" ${currentVal === null ? 'selected' : ''} class="italic text-slate-500">NULL</option>` : ''}
            <option value="1" ${String(currentVal) === '1' || currentVal === true || String(currentVal).toLowerCase() === 'true' ? 'selected' : ''}>1 (TRUE)</option>
            <option value="0" ${String(currentVal) === '0' || currentVal === false || String(currentVal).toLowerCase() === 'false' ? 'selected' : ''}>0 (FALSE)</option>
          </select>
        `;
      } else if (fkInfo && fkInfo.referenced_table && fkInfo.referenced_column) {
        const datalistId = `fk-datalist-${rIdx}-${cIdx}`;
        editorHtml = `
          <div id="active-cell-preset-bar" class="absolute ${pillPosClass} left-0 z-30 bg-slate-900 border border-slate-700/80 rounded shadow-xl px-1.5 py-0.5 flex items-center space-x-1 text-[10px] select-none whitespace-nowrap ring-1 ring-black/40">
            <span class="text-sky-400 font-semibold mr-0.5">🔗 ${escapeHtml(fkInfo.referenced_table)}.${escapeHtml(fkInfo.referenced_column)}:</span>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(null)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-sky-200 rounded border border-slate-700 font-mono transition" title="Set to NULL (Ctrl+Shift+N)">NULL</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white rounded border border-slate-700 font-mono transition" title="Set to Empty String">'' Empty</button>
          </div>
          <input type="text" list="${datalistId}" value="${currentVal === null ? '' : escapeHtml(String(currentVal))}"
                 placeholder="Select or enter ${escapeHtml(fkInfo.referenced_column)}..."
                 class="w-full bg-slate-950 text-sky-200 border border-sky-400 px-1 py-0.5 rounded outline-none font-mono text-xs ring-2 ring-sky-500/40 shadow-inner">
          <datalist id="${datalistId}"></datalist>
        `;
      } else if (rawType === 'date' || rawType.startsWith('date(')) {
        const defaultDate = formatGridCellValue(currentVal, 'date') || '';
        editorHtml = `
          <div id="active-cell-preset-bar" class="absolute ${pillPosClass} left-0 z-30 bg-slate-900 border border-slate-700/80 rounded shadow-xl px-1.5 py-0.5 flex items-center space-x-1 text-[10px] select-none whitespace-nowrap ring-1 ring-black/40">
            <span class="text-slate-400 font-semibold mr-0.5">⚡</span>
            <button type="button" onmousedown="event.preventDefault(); stepActiveDatePart(1)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-white rounded border border-slate-700 font-mono font-bold transition" title="Increment Active Segment (Arrow Up)">▲</button>
            <button type="button" onmousedown="event.preventDefault(); stepActiveDatePart(-1)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-white rounded border border-slate-700 font-mono font-bold transition" title="Decrement Active Segment (Arrow Down)">▼</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('DATE')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-teal-400 hover:text-teal-300 rounded border border-slate-700 font-mono transition" title="Current Date (Ctrl+Shift+D)">Today</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(null)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-sky-200 rounded border border-slate-700 font-mono transition" title="Set to NULL (Ctrl+Shift+N)">NULL</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white rounded border border-slate-700 font-mono transition" title="Set to Empty String (Ctrl+Shift+E)">'' Empty</button>
          </div>
          <div class="flex items-stretch w-full bg-slate-950 border border-sky-400 rounded ring-2 ring-sky-500/40 shadow-inner overflow-hidden">
            <input type="text" value="${escapeHtml(defaultDate)}" placeholder="YYYY-MM-DD"
                   class="flex-1 min-w-0 bg-transparent text-sky-200 px-1 py-0.5 outline-none font-mono text-xs">
            <div class="flex flex-col border-l border-slate-700/80 bg-slate-900 divide-y divide-slate-800 select-none">
              <button type="button" tabindex="-1" onmousedown="event.preventDefault(); stepActiveDatePart(1)" class="px-1 text-[9px] leading-none h-1/2 flex items-center justify-center text-slate-400 hover:text-sky-300 hover:bg-slate-800 transition" title="Increment (Up)">▲</button>
              <button type="button" tabindex="-1" onmousedown="event.preventDefault(); stepActiveDatePart(-1)" class="px-1 text-[9px] leading-none h-1/2 flex items-center justify-center text-slate-400 hover:text-sky-300 hover:bg-slate-800 transition" title="Decrement (Down)">▼</button>
            </div>
          </div>
        `;
      } else if (/^(datetime|timestamp|timestamptz)/i.test(rawType)) {
        const defaultDatetime = formatGridCellValue(currentVal, 'datetime') || '';
        editorHtml = `
          <div id="active-cell-preset-bar" class="absolute ${pillPosClass} left-0 z-30 bg-slate-900 border border-slate-700/80 rounded shadow-xl px-1.5 py-0.5 flex items-center space-x-1 text-[10px] select-none whitespace-nowrap ring-1 ring-black/40">
            <span class="text-slate-400 font-semibold mr-0.5">⚡</span>
            <button type="button" onmousedown="event.preventDefault(); stepActiveDatePart(1)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-white rounded border border-slate-700 font-mono font-bold transition" title="Increment Active Segment (Arrow Up)">▲</button>
            <button type="button" onmousedown="event.preventDefault(); stepActiveDatePart(-1)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-white rounded border border-slate-700 font-mono font-bold transition" title="Decrement Active Segment (Arrow Down)">▼</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('NOW')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-emerald-400 hover:text-emerald-300 rounded border border-slate-700 font-mono transition" title="Current Timestamp (Ctrl+Shift+T)">NOW</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('DATE')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-teal-400 hover:text-teal-300 rounded border border-slate-700 font-mono transition" title="Current Date">Today</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(null)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-sky-200 rounded border border-slate-700 font-mono transition" title="Set to NULL (Ctrl+Shift+N)">NULL</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white rounded border border-slate-700 font-mono transition" title="Set to Empty String (Ctrl+Shift+E)">'' Empty</button>
          </div>
          <div class="flex items-stretch w-full bg-slate-950 border border-sky-400 rounded ring-2 ring-sky-500/40 shadow-inner overflow-hidden">
            <input type="text" value="${escapeHtml(defaultDatetime)}" placeholder="YYYY-MM-DD HH:mm:ss"
                   class="flex-1 min-w-0 bg-transparent text-purple-300 px-1 py-0.5 outline-none font-mono text-xs">
            <div class="flex flex-col border-l border-slate-700/80 bg-slate-900 divide-y divide-slate-800 select-none">
              <button type="button" tabindex="-1" onmousedown="event.preventDefault(); stepActiveDatePart(1)" class="px-1 text-[9px] leading-none h-1/2 flex items-center justify-center text-slate-400 hover:text-sky-300 hover:bg-slate-800 transition" title="Increment (Up)">▲</button>
              <button type="button" tabindex="-1" onmousedown="event.preventDefault(); stepActiveDatePart(-1)" class="px-1 text-[9px] leading-none h-1/2 flex items-center justify-center text-slate-400 hover:text-sky-300 hover:bg-slate-800 transition" title="Decrement (Down)">▼</button>
            </div>
          </div>
        `;
      } else if (rawType === 'time') {
        editorHtml = `
          <div id="active-cell-preset-bar" class="absolute ${pillPosClass} left-0 z-30 bg-slate-900 border border-slate-700/80 rounded shadow-xl px-1.5 py-0.5 flex items-center space-x-1 text-[10px] select-none whitespace-nowrap ring-1 ring-black/40">
            <span class="text-slate-400 font-semibold mr-0.5">⚡</span>
            <button type="button" onmousedown="event.preventDefault(); stepActiveDatePart(1)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-white rounded border border-slate-700 font-mono font-bold transition" title="Increment Active Segment (Arrow Up)">▲</button>
            <button type="button" onmousedown="event.preventDefault(); stepActiveDatePart(-1)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-white rounded border border-slate-700 font-mono font-bold transition" title="Decrement Active Segment (Arrow Down)">▼</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(null)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-sky-200 rounded border border-slate-700 font-mono transition" title="Set to NULL (Ctrl+Shift+N)">NULL</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white rounded border border-slate-700 font-mono transition" title="Set to Empty String (Ctrl+Shift+E)">'' Empty</button>
          </div>
          <div class="flex items-stretch w-full bg-slate-950 border border-sky-400 rounded ring-2 ring-sky-500/40 shadow-inner overflow-hidden">
            <input type="text" value="${currentVal === null ? '' : escapeHtml(String(currentVal))}" placeholder="HH:mm:ss"
                   class="flex-1 min-w-0 bg-transparent text-sky-200 px-1 py-0.5 outline-none font-mono text-xs">
            <div class="flex flex-col border-l border-slate-700/80 bg-slate-900 divide-y divide-slate-800 select-none">
              <button type="button" tabindex="-1" onmousedown="event.preventDefault(); stepActiveDatePart(1)" class="px-1 text-[9px] leading-none h-1/2 flex items-center justify-center text-slate-400 hover:text-sky-300 hover:bg-slate-800 transition" title="Increment (Up)">▲</button>
              <button type="button" tabindex="-1" onmousedown="event.preventDefault(); stepActiveDatePart(-1)" class="px-1 text-[9px] leading-none h-1/2 flex items-center justify-center text-slate-400 hover:text-sky-300 hover:bg-slate-800 transition" title="Decrement (Down)">▼</button>
            </div>
          </div>
        `;
      } else {
        const typeStyle = getGridTypeStyle(colMeta?.type || col?.type);
        editorHtml = `
          <div id="active-cell-preset-bar" class="absolute ${pillPosClass} left-0 z-30 bg-slate-900 border border-slate-700/80 rounded shadow-xl px-1.5 py-0.5 flex items-center space-x-1 text-[10px] select-none whitespace-nowrap ring-1 ring-black/40">
            <span class="text-slate-400 font-semibold mr-0.5">⚡</span>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit(null)" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-sky-200 rounded border border-slate-700 font-mono transition" title="Set to NULL (Ctrl+Shift+N)">NULL</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white rounded border border-slate-700 font-mono transition" title="Set to Empty String (Ctrl+Shift+E)">'' Empty</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('NOW')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-emerald-400 hover:text-emerald-300 rounded border border-slate-700 font-mono transition" title="Current Timestamp (Ctrl+Shift+T)">NOW</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('DATE')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-teal-400 hover:text-teal-300 rounded border border-slate-700 font-mono transition" title="Current Date (Ctrl+Shift+D)">Date</button>
            <button type="button" onmousedown="event.preventDefault(); applyPresetToActiveEdit('UUID')" class="px-1.5 py-0.5 bg-slate-800 hover:bg-slate-700 text-purple-400 hover:text-purple-300 rounded border border-slate-700 font-mono transition" title="Generate UUID">UUID</button>
          </div>
          <input type="text" value="${currentVal === null ? '' : escapeHtml(String(currentVal))}"
                 class="w-full bg-slate-950 ${typeStyle.inputColor} ${typeStyle.inputAlign} border border-sky-400 px-1 py-0.5 rounded outline-none font-mono text-xs ring-2 ring-sky-500/40 shadow-inner">
        `;
      }

      td.innerHTML = editorHtml;
      // Allow popups/steppers to be visible outside truncated table cells during edit
      if (td) {
        td.classList.remove('truncate');
        td.style.overflow = 'visible';
        td.style.zIndex = '35';
      }

      const input = td.querySelector('input, select');
      const editInfo = {
        td,
        rIdx,
        cIdx,
        pkCol,
        tableName,
        originalVal,
        input,
        isCommitting: false,
        hasExplicitVal: false,
        explicitVal: null,
        rawType,
        enumValues,
        lastPartPos: 8 // default to day part
      };
      activeEditCell = editInfo;

      // Track caret position when clicking or typing in date/datetime inputs
      const trackCaret = () => {
        if (input && input.selectionStart !== undefined && input.selectionStart !== null) {
          editInfo.lastPartPos = input.selectionStart;
        }
      };
      input.addEventListener('click', trackCaret);
      input.addEventListener('keyup', trackCaret);
      input.addEventListener('select', trackCaret);

      // Asynchronously fetch FK parent rows if FK column
      if (fkInfo && fkInfo.referenced_table && fkInfo.referenced_column) {
        const datalistId = `fk-datalist-${rIdx}-${cIdx}`;
        const refTable = fkInfo.referenced_table;
        const refCol = fkInfo.referenced_column;
        const targetDb = activeGridDB || (activeTab && activeTab.database) || '';
        apiFetch(`/api/session/${activeConnection.id}/table/${encodeURIComponent(refTable)}/rows?db=${encodeURIComponent(targetDb)}&limit=100`)
          .then(res => res.ok ? res.json() : null)
          .then(data => {
            const dl = document.getElementById(datalistId);
            if (dl && data && Array.isArray(data.rows) && Array.isArray(data.columns)) {
              const rColIdx = data.columns.findIndex(c => (c.name || '').toLowerCase() === refCol.toLowerCase());
              const nameColIdx = data.columns.findIndex(c => /name|title|username|email|label|desc/i.test(c.name || ''));
              if (rColIdx !== -1) {
                dl.innerHTML = data.rows.map(r => {
                  const val = r[rColIdx];
                  const label = (nameColIdx !== -1 && nameColIdx !== rColIdx && r[nameColIdx] !== null) ? ` (${r[nameColIdx]})` : '';
                  return `<option value="${escapeHtml(String(val))}">${escapeHtml(String(val))}${escapeHtml(label)}</option>`;
                }).join('');
              }
            }
          })
          .catch(() => {});
      }

      input.focus();
      const isDateOrTimeType = rawType === 'date' || rawType.startsWith('date(') || /^(datetime|timestamp|timestamptz|time)/i.test(rawType);
      if (isDateOrTimeType) {
        // Highlight day segment by default for fast date adjustments
        try {
          if (rawType === 'time') {
            input.setSelectionRange(0, 2);
            editInfo.lastPartPos = 0;
          } else {
            input.setSelectionRange(8, 10);
            editInfo.lastPartPos = 8;
          }
        } catch (_) {}
      } else if (typeof input.select === 'function') {
        input.select();
      }

      if (input.tagName === 'SELECT') {
        input.addEventListener('change', () => {
          commitInlineEdit(editInfo, false);
        });
      }

      input.addEventListener('keydown', async (e) => {
        const isDateType = rawType === 'date' || rawType.startsWith('date(');
        const isDatetimeType = /^(datetime|timestamp|timestamptz)/i.test(rawType);
        const isTimeType = rawType === 'time';
        if ((isDateType || isDatetimeType || isTimeType) && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
          e.preventDefault();
          stepActiveDatePart(e.key === 'ArrowUp' ? 1 : -1);
          return;
        }
        if (e.key === 'Enter') {
          e.preventDefault();
          commitInlineEdit(editInfo, false);
        } else if (e.key === 'Escape') {
          e.preventDefault();
          cancelInlineEdit(editInfo);
        } else if (e.ctrlKey && e.shiftKey && (e.key === 'N' || e.key === 'n')) {
          e.preventDefault();
          applyPresetToActiveEdit(null);
        } else if (e.ctrlKey && e.shiftKey && (e.key === 'E' || e.key === 'e')) {
          e.preventDefault();
          applyPresetToActiveEdit('');
        } else if (e.ctrlKey && e.shiftKey && (e.key === 'T' || e.key === 't')) {
          e.preventDefault();
          applyPresetToActiveEdit('NOW');
        } else if (e.ctrlKey && e.shiftKey && (e.key === 'D' || e.key === 'd')) {
          e.preventDefault();
          applyPresetToActiveEdit('DATE');
        } else if (e.key === 'Tab') {
          e.preventDefault();
          const nextCIdx = e.shiftKey ? cIdx - 1 : cIdx + 1;
          commitInlineEdit(editInfo, true);
          if (nextCIdx >= 0 && nextCIdx < currentColumns.length) {
            const rowEl = td.parentElement;
            const nextTd = rowEl ? rowEl.querySelector(`td[data-col="${nextCIdx}"]`) : null;
            if (nextTd) startInlineEdit(nextTd, rIdx, nextCIdx, pkCol, tableName);
          }
        }
      });

      input.addEventListener('blur', () => {
        setTimeout(() => {
          if (activeEditCell && activeEditCell.input === input && !activeEditCell.isCommitting) {
            commitInlineEdit(activeEditCell, false);
          }
        }, 150);
      });
    }

    async function commitInlineEdit(editInfo, isSwitchingCells) {
      if (!editInfo || editInfo.isCommitting) return;
      editInfo.isCommitting = true;
      if (editInfo.td) {
        editInfo.td.style.position = '';
        editInfo.td.style.overflow = '';
        editInfo.td.style.zIndex = '';
        editInfo.td.classList.add('truncate');
      }

      const input = editInfo.input;
      const td = editInfo.td;
      const rIdx = editInfo.rIdx;
      const cIdx = editInfo.cIdx;
      const pkCol = editInfo.pkCol;
      const tableName = editInfo.tableName;
      const originalVal = editInfo.originalVal;
      const col = currentColumns[cIdx];

      const newVal = input ? input.value : '';
      let effectiveVal;
      if (editInfo.hasExplicitVal) {
        effectiveVal = editInfo.explicitVal;
      } else if (input && input.tagName === 'SELECT') {
        effectiveVal = input.value === '__SQLFOX_NULL__' ? null : input.value;
      } else {
        const isNull = newVal === '' && (originalVal === null || (col && col.type && col.type.toLowerCase().includes('null')));
        effectiveVal = isNull ? null : newVal;
      }

      // Input Validation according to data type
      if (effectiveVal !== null) {
        const valStr = String(effectiveVal).trim();
        const enumValues = editInfo.enumValues;
        const rawType = (editInfo.rawType || '').toLowerCase();
        if (enumValues && enumValues.length > 0) {
          if (!enumValues.includes(valStr) && valStr !== '') {
            showNativeAlert(`Value "${valStr}" is not a valid ENUM option for '${col.name}'.\nAllowed: ${enumValues.join(', ')}`, 'Validation Error');
            renderCellContent(td, rIdx, cIdx, pkCol, tableName);
            if (activeEditCell === editInfo) activeEditCell = null;
            return;
          }
        } else if (rawType === 'boolean' || rawType === 'bool') {
          if (valStr !== '1' && valStr !== '0' && valStr !== 'true' && valStr !== 'false' && valStr !== '') {
            showNativeAlert(`Value "${valStr}" is not a valid BOOLEAN for '${col.name}'.\nAllowed: 1 (TRUE), 0 (FALSE)`, 'Validation Error');
            renderCellContent(td, rIdx, cIdx, pkCol, tableName);
            if (activeEditCell === editInfo) activeEditCell = null;
            return;
          }
        } else if (rawType.includes('int') && !rawType.includes('point')) {
          if (valStr !== '' && !/^-?\d+$/.test(valStr)) {
            showNativeAlert(`Value "${valStr}" is not a valid integer for '${col.name}' (${rawType}).`, 'Validation Error');
            renderCellContent(td, rIdx, cIdx, pkCol, tableName);
            if (activeEditCell === editInfo) activeEditCell = null;
            return;
          }
        } else if ((rawType.includes('decimal') || rawType.includes('float') || rawType.includes('double') || rawType.includes('numeric')) && !rawType.includes('char')) {
          if (valStr !== '' && !/^-?\d*(\.\d+)?$/.test(valStr)) {
            showNativeAlert(`Value "${valStr}" is not a valid number for '${col.name}' (${rawType}).`, 'Validation Error');
            renderCellContent(td, rIdx, cIdx, pkCol, tableName);
            if (activeEditCell === editInfo) activeEditCell = null;
            return;
          }
        } else if (rawType === 'date' || rawType.startsWith('date(')) {
          if (valStr !== '') {
            const normalized = formatGridCellValue(valStr, 'date');
            const m = normalized ? normalized.match(/^(\d{4})-(\d{2})-(\d{2})$/) : null;
            if (m) {
              const y = parseInt(m[1], 10);
              const mon = parseInt(m[2], 10);
              let d = parseInt(m[3], 10);
              const maxD = getDaysInMonth(y, mon);
              if (d > maxD) d = maxD;
              const pad = (n) => String(n).padStart(2, '0');
              effectiveVal = `${y}-${pad(mon)}-${pad(d)}`;
            } else {
              showNativeAlert(`Value "${valStr}" is not a valid date for '${col.name}'. Expected YYYY-MM-DD.`, 'Validation Error');
              renderCellContent(td, rIdx, cIdx, pkCol, tableName);
              if (activeEditCell === editInfo) activeEditCell = null;
              return;
            }
          }
        } else if (/^(datetime|timestamp|timestamptz)/i.test(rawType)) {
          if (valStr !== '' && valStr !== 'NOW()' && valStr !== 'CURRENT_TIMESTAMP') {
            const normalized = formatGridCellValue(valStr, 'datetime');
            const m = normalized ? normalized.match(/^(\d{4})-(\d{2})-(\d{2})(?: (\d{2}:\d{2}:\d{2}))?$/) : null;
            if (m) {
              const y = parseInt(m[1], 10);
              const mon = parseInt(m[2], 10);
              let d = parseInt(m[3], 10);
              const maxD = getDaysInMonth(y, mon);
              if (d > maxD) d = maxD;
              const pad = (n) => String(n).padStart(2, '0');
              const timePart = m[4] || '00:00:00';
              effectiveVal = `${y}-${pad(mon)}-${pad(d)} ${timePart}`;
            } else {
              showNativeAlert(`Value "${valStr}" is not a valid datetime for '${col.name}'. Expected YYYY-MM-DD HH:mm:ss.`, 'Validation Error');
              renderCellContent(td, rIdx, cIdx, pkCol, tableName);
              if (activeEditCell === editInfo) activeEditCell = null;
              return;
            }
          }
        }
      }

      const isChanged = (originalVal === null && effectiveVal !== null) ||
                        (originalVal !== null && effectiveVal === null) ||
                        (originalVal !== null && effectiveVal !== null && String(originalVal) !== String(effectiveVal));

      if (!isChanged) {
        renderCellContent(td, rIdx, cIdx, pkCol, tableName);
        if (activeEditCell === editInfo) activeEditCell = null;
        return;
      }

      const activeTab = tabs.find(t => t.id === activeTabId);
      // ponytail: Prioritize activeGridDB over activeTab.database so cross-database inline edits save to the correct schema.
      const dbName = activeGridDB || (activeTab && activeTab.database) || '';

      const pkIdx = pkCol ? currentColumns.findIndex(c => (c.name || '').toLowerCase() === pkCol.toLowerCase()) : -1;
      const pkVal = pkIdx !== -1 ? currentResults[rIdx][pkIdx] : null;

      const rowCriteria = {};
      if (pkIdx === -1) {
        currentColumns.forEach((c, idx) => {
          rowCriteria[c.name] = currentResults[rIdx][idx];
        });
      }

      if (gridEditMode === 'auto') {
        if (gridConfirmSave) {
          const colName = col ? col.name : `Column #${cIdx}`;
          const displayVal = effectiveVal === null ? 'NULL' : `"${effectiveVal}"`;
          const ok = await showNativeConfirm(`Save changes to ${tableName}.${colName} = ${displayVal}?`, 'Confirm Save');
          if (!ok) {
            renderCellContent(td, rIdx, cIdx, pkCol, tableName);
            if (activeEditCell === editInfo) activeEditCell = null;
            return;
          }
        }

        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/cell-edit`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              database: dbName,
              table: tableName,
              column: col.name,
              value: effectiveVal,
              pk_column: pkIdx !== -1 ? pkCol : '',
              pk_value: pkVal,
              row_criteria: pkIdx === -1 ? rowCriteria : undefined
            })
          });
          if (res.ok) {
            currentResults[rIdx][cIdx] = effectiveVal;
            renderCellContent(td, rIdx, cIdx, pkCol, tableName);
            td.classList.add('bg-emerald-950/40', 'border-emerald-600/60');
            setTimeout(() => td.classList.remove('bg-emerald-950/40', 'border-emerald-600/60'), 1500);
          } else {
            const err = await res.json().catch(() => ({}));
            showNativeAlert('Failed to save cell: ' + (err.error || res.statusText), 'Save Error');
            renderCellContent(td, rIdx, cIdx, pkCol, tableName);
          }
        } catch (err) {
          showNativeAlert('Network error saving cell: ' + err.message, 'Save Error');
          renderCellContent(td, rIdx, cIdx, pkCol, tableName);
        }
      } else {
        const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);

        stageGridEdit(stagedKey, {
          rIdx,
          cIdx,
          db: dbName,
          table: tableName,
          colName: col.name,
          pkCol: pkIdx !== -1 ? pkCol : '',
          pkVal,
          rowCriteria: pkIdx === -1 ? rowCriteria : undefined,
          oldVal: originalVal,
          newVal: effectiveVal
        });

        currentResults[rIdx][cIdx] = effectiveVal;
        renderCellContent(td, rIdx, cIdx, pkCol, tableName);
        updateStagedEditsUI();
      }

      if (activeEditCell === editInfo) activeEditCell = null;
    }

    function cancelInlineEdit(editInfo) {
      if (!editInfo) return;
      if (editInfo.td) {
        editInfo.td.style.position = '';
        editInfo.td.style.overflow = '';
        editInfo.td.style.zIndex = '';
        editInfo.td.classList.add('truncate');
      }
      renderCellContent(editInfo.td, editInfo.rIdx, editInfo.cIdx, editInfo.pkCol, editInfo.tableName);
      activeEditCell = null;
    }

    function renderCellContent(td, rIdx, cIdx, pkCol, tableName) {
      if (td) {
        td.style.position = '';
        td.style.overflow = '';
        td.style.zIndex = '';
        td.classList.add('truncate');
      }
      const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
      const staged = stagedEdits.get(stagedKey);
      const rawVal = staged ? staged.newVal : currentResults[rIdx][cIdx];
      const isDirty = Boolean(staged);
      const col = currentColumns[cIdx];
      const colName = col?.name;
      const colType = col?.type;

      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === (tableName || '').toLowerCase());
      const colMeta = (tableMeta?.columns || []).find(c => (c.name || '').toLowerCase() === (colName || '').toLowerCase());
      const resolvedType = colMeta?.type || colType;

      const val = formatGridCellValue(rawVal, resolvedType);
      const typeStyle = getGridTypeStyle(resolvedType);
      const isJson = val !== null && typeof val === 'string' && (typeStyle.category === 'json' || (val.trim().startsWith('{') && val.trim().endsWith('}')) || (val.trim().startsWith('[') && val.trim().endsWith(']')));
      let jsonBadge = '';
      if (isJson) {
        jsonBadge = `<button type="button" onclick="event.stopPropagation(); openJsonCellModal(${rIdx}, ${cIdx})" class="ml-1 opacity-0 group-hover:opacity-100 hover:text-amber-300 text-amber-400 font-mono font-bold transition text-xs" title="Open JSON Cell Viewer">{ }</button>`;
      }

      td.innerHTML = `
        <span class="cell-val">${val === null ? '<span class="text-slate-500 italic">NULL</span>' : escapeHtml(String(val))}</span>
        ${jsonBadge}
        ${isDirty ? '<span class="ml-1 text-[9px] text-amber-400 font-bold" title="Unapplied staged change">●</span>' : ''}
        <button type="button" onclick="event.stopPropagation(); openGridCellMenu(event, ${rIdx}, ${cIdx}, this.parentElement)" class="ml-1 opacity-0 group-hover:opacity-70 hover:!opacity-100 text-slate-400 hover:text-sky-300 font-bold transition text-[10px] select-none inline-flex items-center" title="Insert Value / Cell Menu (Right-click)">${renderIcon('edit', 'w-3 h-3')}</button>
      `;

      if (isDirty) {
        td.className = "p-1.5 px-3 truncate max-w-xs grid-cell group select-text cursor-default transition bg-amber-950/40 border border-amber-600/60 text-amber-200 " + typeStyle.align;
      } else {
        td.className = `p-1.5 px-3 truncate max-w-xs grid-cell group select-text cursor-default transition border-r border-slate-800/40 ${typeStyle.color} ${typeStyle.align}`;
      }
    }

    // --- Table CRUD Suite: Add Row & Multi-Row Delete ---
    function addGridDraftRow() {
      if (activeSession && activeSession.read_only) {
        showNativeAlert('Session is in READ-ONLY mode. Unlock first.', 'Read-Only Mode');
        return;
      }
      const meta = resolveGridTableMeta();
      if (!meta.canMutate) {
        showNativeAlert('Adding draft rows is only available for single-table queries containing a Primary Key.', 'Add Row');
        return;
      }
      if (meta.tableName) activeGridTable = meta.tableName;
      if (meta.dbName) activeGridDB = meta.dbName;
      if (meta.pkCol) activeGridPKCol = meta.pkCol;

      if (!activeGridTable) {
        showNativeAlert('Please select a table to add records.', 'Add Row');
        return;
      }
      const draft = { data: {} };
      stagedDraftRows.unshift(draft);
      updateStagedEditsUI();
      renderGridResults(currentColumns, currentResults, meta.pkCol || activeGridPKCol, meta.tableName || activeGridTable, meta.dbName || activeGridDB);
    }

    function removeGridDraftRow(dIdx) {
      if (dIdx >= 0 && dIdx < stagedDraftRows.length) {
        stagedDraftRows.splice(dIdx, 1);
        updateStagedEditsUI();
        const meta = resolveGridTableMeta();
        renderGridResults(currentColumns, currentResults, meta.pkCol || activeGridPKCol, meta.tableName || activeGridTable, meta.dbName || activeGridDB);
      }
    }

    function updateGridDraftCell(dIdx, colName, val) {
      if (stagedDraftRows[dIdx]) {
        stagedDraftRows[dIdx].data[colName] = val;
      }
    }

    function toggleSelectAllRows(checked) {
      selectedRowIndices.clear();
      if (checked && currentResults) {
        currentResults.forEach((_, idx) => selectedRowIndices.add(idx));
      }
      document.querySelectorAll('.grid-row-checkbox').forEach(cb => {
        cb.checked = checked;
      });
      updateSelectedRowsUI();
    }

    function toggleSelectRow(rIdx, checked) {
      if (checked) {
        selectedRowIndices.add(rIdx);
      } else {
        selectedRowIndices.delete(rIdx);
      }
      const selectAll = document.getElementById('grid-select-all');
      if (selectAll) {
        selectAll.checked = selectedRowIndices.size === (currentResults?.length || 0) && selectedRowIndices.size > 0;
      }
      updateSelectedRowsUI();
    }

    function updateSelectedRowsUI() {
      const btn = document.getElementById('btn-grid-delete-selected');
      const countSpan = document.getElementById('grid-selected-count');
      const selectAll = document.getElementById('grid-select-all');
      const count = selectedRowIndices.size;
      if (selectAll && count === 0) {
        selectAll.checked = false;
      }
      if (countSpan) countSpan.textContent = count;
      if (btn) {
        if (count > 0) {
          btn.classList.remove('hidden');
        } else {
          btn.classList.add('hidden');
        }
      }
    }

    async function deleteSelectedGridRows() {
      if (activeSession && activeSession.read_only) {
        showNativeAlert('Session is in READ-ONLY mode. Unlock first.', 'Read-Only Mode');
        return;
      }
      const meta = resolveGridTableMeta();
      if (!meta.canMutate) {
        showNativeAlert('Deletion is only available for single-table queries containing a Primary Key.', 'Delete Blocked');
        return;
      }
      if (meta.tableName) activeGridTable = meta.tableName;
      if (meta.dbName) activeGridDB = meta.dbName;
      if (meta.pkCol) activeGridPKCol = meta.pkCol;

      if (!activeGridTable) return;

      const targetIndices = getTargetGridRowIndices();
      const count = targetIndices.length;
      if (count === 0) return;

      const pkIdx = activeGridPKCol ? currentColumns.findIndex(c => (c.name || '').toLowerCase() === activeGridPKCol.toLowerCase()) : -1;
      if (!meta.isTableViewer && pkIdx === -1) {
        showNativeAlert('Cannot delete rows: The Primary Key column must be included in your query results.', 'Delete Blocked');
        return;
      }

      // In Production, require typing challenge
      const confirmMsg = count > 1
        ? `Are you sure you want to permanently delete ${count} selected rows from "${activeGridTable}"?`
        : `Are you sure you want to permanently delete row #${targetIndices[0] + 1} from "${activeGridTable}"?`;

      if (activeConnection && activeConnection.environment === 'Production') {
        const ok = await showNativeConfirm(`[PRODUCTION SAFETY GUARD] Confirm permanent deletion of ${count} row(s) from table "${activeGridTable}"?`, 'Production Delete Warning');
        if (!ok) return;
      } else {
        const ok = await showNativeConfirm(confirmMsg, 'Delete Rows');
        if (!ok) return;
      }

      const activeTab = tabs.find(t => t.id === activeTabId);
      const dbName = activeGridDB || (activeTab && activeTab.database) || '';

      const deletePayload = {
        database: dbName,
        table: activeGridTable
      };

      if (pkIdx !== -1) {
        deletePayload.pk_column = activeGridPKCol;
        deletePayload.pk_values = targetIndices.map(idx => currentResults[idx][pkIdx]);
      } else {
        deletePayload.row_criterias = targetIndices.map(idx => {
          const rc = {};
          currentColumns.forEach((c, cIdx) => {
            rc[c.name] = currentResults[idx][cIdx];
          });
          return rc;
        });
      }

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/delete-rows`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(deletePayload)
        });
        if (res.ok) {
          targetIndices.forEach(idx => selectedRowIndices.delete(idx));
          updateSelectedRowsUI();
          showNativeAlert(`✅ Successfully deleted ${count} row(s)!`, 'Rows Deleted');
          if (activeTab && activeTab.isTableViewer) {
            await loadTableRows(activeGridTable, activeGridDB);
          } else {
            await runActiveQuery();
          }
        } else {
          const err = await res.json().catch(() => ({}));
          showNativeAlert('Failed to delete rows: ' + (err.error || res.statusText), 'Delete Error');
        }
      } catch (err) {
        showNativeAlert('Network error deleting rows: ' + err.message, 'Delete Error');
      }
    }

    function handleColFilterKeydown(event, input) {
      if (event.key === 'Escape') {
        input.value = '';
        handleColFilterKeydown({ key: 'Enter', preventDefault() {} }, input);
        input.blur();
        return;
      }
      if (event.key === 'Enter') {
        event.preventDefault();
        const col = input.dataset.col;
        const val = input.value.trim();
        const activeTab = tabs.find(t => t.id === activeTabId);
        if (!activeTab) return;

        if (activeTab.isTableViewer) {
          if (!Array.isArray(activeTab.filterPills)) {
            activeTab.filterPills = [];
          }

          activeTab.filterPills = activeTab.filterPills.filter(p => (p.col || p.column) !== col);

          if (val !== '') {
            let parsedOp = '=';
            let parsedVal = val;
            const opMatch = val.match(/^(=|!=|<>|>=|<=|>|<|LIKE|NOT\s+LIKE|IS\s+NULL|IS\s+NOT\s+NULL|IN)\s*(.*)$/i);
            if (opMatch) {
              parsedOp = opMatch[1].toUpperCase().replace(/\s+/, ' ');
              if (parsedOp === '<>') parsedOp = '!=';
              parsedVal = opMatch[2].trim();
            } else if (val.includes('%')) {
              parsedOp = 'LIKE';
            }

            activeTab.filterPills.push({
              col: col,
              column: col,
              op: parsedOp,
              operator: parsedOp,
              val: parsedVal,
              value: parsedVal
            });
          }

          activeTab.filterMode = 'pills';
          activeTab.offset = 0;
          syncFilterBarUI(activeTab);
          applyTableFilter();
        } else {
          // Query Results tab: in-memory client-side filter
          if (!activeTab.queryFilters) activeTab.queryFilters = {};
          if (val === '') {
            delete activeTab.queryFilters[col];
          } else {
            activeTab.queryFilters[col] = val;
          }
          applyQueryResultsFilter(activeTab);
        }
      }
    }

    // ponytail: in-memory predicate evaluator for query result filtering and spreadsheet lookups.
    // Known ceiling: doesn't execute complex nested SQL subqueries or joins. Upgrade path: SQL AST evaluator.
    function matchesFilterCondition(cellVal, expr) {
      if (!expr || expr.trim() === '') return true;
      const trimmedExpr = expr.trim();
      const cellStr = cellVal === null || cellVal === undefined ? '' : String(cellVal);

      if (/^IS\s+NULL$/i.test(trimmedExpr)) {
        return cellVal === null || cellVal === undefined;
      }
      if (/^IS\s+NOT\s+NULL$/i.test(trimmedExpr)) {
        return cellVal !== null && cellVal !== undefined;
      }

      if (cellVal === null || cellVal === undefined) return false;

      const inMatch = trimmedExpr.match(/^IN\s*\(([^)]+)\)$/i);
      if (inMatch) {
        const items = inMatch[1].split(',').map(s => s.trim().replace(/^['"]|['"]$/g, '').toLowerCase());
        return items.includes(cellStr.toLowerCase());
      }

      const opMatch = trimmedExpr.match(/^(!=|<>|>=|<=|>|<|=|LIKE|NOT\s+LIKE)\s*(.*)$/i);
      let op = '=';
      let targetVal = trimmedExpr;
      if (opMatch) {
        op = opMatch[1].toUpperCase().replace(/\s+/, ' ');
        if (op === '<>') op = '!=';
        targetVal = opMatch[2].trim().replace(/^['"]|['"]$/g, '');
      } else if (trimmedExpr.includes('%')) {
        op = 'LIKE';
      } else {
        return cellStr.toLowerCase().includes(trimmedExpr.toLowerCase());
      }

      if (op === '=') {
        if (!isNaN(Number(cellVal)) && !isNaN(Number(targetVal)) && targetVal !== '') {
          return Number(cellVal) === Number(targetVal);
        }
        return cellStr.toLowerCase() === targetVal.toLowerCase();
      }
      if (op === '!=') {
        if (!isNaN(Number(cellVal)) && !isNaN(Number(targetVal)) && targetVal !== '') {
          return Number(cellVal) !== Number(targetVal);
        }
        return cellStr.toLowerCase() !== targetVal.toLowerCase();
      }
      if (op === '>' || op === '>=' || op === '<' || op === '<=') {
        const numCell = Number(cellVal);
        const numTarget = Number(targetVal);
        if (!isNaN(numCell) && !isNaN(numTarget) && targetVal !== '') {
          if (op === '>') return numCell > numTarget;
          if (op === '>=') return numCell >= numTarget;
          if (op === '<') return numCell < numTarget;
          if (op === '<=') return numCell <= numTarget;
        }
        if (op === '>') return cellStr > targetVal;
        if (op === '>=') return cellStr >= targetVal;
        if (op === '<') return cellStr < targetVal;
        if (op === '<=') return cellStr <= targetVal;
      }
      if (op === 'LIKE' || op === 'NOT LIKE') {
        let pattern = targetVal;
        // ponytail: smart wildcard auto-wrap for in-memory grid filter matching
        if (!pattern.includes('%') && !pattern.includes('_')) {
          pattern = `%${pattern}%`;
        }
        const regexStr = '^' + pattern
          .replace(/[.+^${}()|[\]\\]/g, '\\$&')
          .replace(/%/g, '.*')
          .replace(/_/g, '.') + '$';
        const regex = new RegExp(regexStr, 'i');
        const isMatch = regex.test(cellStr);
        return op === 'LIKE' ? isMatch : !isMatch;
      }

      return true;
    }

    function applyQueryResultsFilter(activeTab) {
      if (!activeTab) activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || activeTab.isTableViewer) return;
      activeTab.queryPage = 0;
      const allRows = activeTab.rawQueryRows || currentResults || [];
      const cols = activeTab.rawQueryCols || currentColumns || [];
      const filters = activeTab.queryFilters || {};
      const filterKeys = Object.keys(filters).filter(k => (filters[k] || '').trim() !== '');

      if (filterKeys.length === 0) {
        renderGridResults(cols, allRows, activeGridPKCol, activeGridTable, activeGridDB, true);
        const stats = document.getElementById('grid-result-stats');
        if (stats) stats.textContent = `${allRows.length} rows`;
        return;
      }

      const filtered = allRows.filter(row => {
        return filterKeys.every(colName => {
          const colIdx = cols.findIndex(c => c.name === colName);
          if (colIdx === -1) return true;
          const rawCellVal = row[colIdx];
          const filterVal = filters[colName];
          return matchesFilterCondition(rawCellVal, filterVal);
        });
      });

      renderGridResults(cols, filtered, activeGridPKCol, activeGridTable, activeGridDB, true);
      const stats = document.getElementById('grid-result-stats');
      if (stats) stats.textContent = `${filtered.length} / ${allRows.length} rows (filtered)`;
    }

    // --- Floating Grid Row Context Menu ---
    function openGridContextMenu(e, rIdx) {
      e.preventDefault();
      closeAllContextMenus();
      const td = e.target.closest('td[data-col]');
      const cIdx = td ? parseInt(td.dataset.col, 10) : -1;

      activeGridRightClickedRow = rIdx;
      activeGridRightClickedCol = cIdx >= 0 ? cIdx : null;
      activeGridRightClickedTd = td;

      if (td && cIdx >= 0) {
        selectGridCell(td, rIdx, cIdx);
      }

      const menu = document.getElementById('grid-context-menu');
      if (!menu) return;

      const activeTab = tabs.find(t => t.id === activeTabId);
      const isTableViewer = Boolean(activeTab && activeTab.isTableViewer);
      const gridMeta = resolveGridTableMeta();
      const canMutate = gridMeta.canMutate;

      const ctxMutations = document.getElementById('grid-ctx-table-mutations');
      const mutParent = document.getElementById('grid-ctx-mutations-parent');
      const mutLabel = document.getElementById('grid-ctx-mutations-label');
      if (ctxMutations) {
        ctxMutations.classList.remove('hidden');
        const mutButtons = [
          document.getElementById('grid-ctx-btn-dup-nokey'),
          document.getElementById('grid-ctx-btn-dup-key'),
          document.getElementById('grid-ctx-btn-add-draft'),
          document.getElementById('grid-ctx-btn-delete-row')
        ];
        mutButtons.forEach(btn => {
          if (!btn) return;
          btn.disabled = !canMutate;
          if (canMutate) {
            btn.classList.remove('opacity-40', 'cursor-not-allowed');
            if (typeof btn.removeAttribute === 'function') btn.removeAttribute('title');
            btn.title = '';
          } else {
            btn.classList.add('opacity-40', 'cursor-not-allowed');
            const titleMsg = 'Mutations are only available for single-table queries containing a Primary Key';
            if (typeof btn.setAttribute === 'function') btn.setAttribute('title', titleMsg);
            btn.title = titleMsg;
          }
        });
      }
      if (mutParent) {
        mutParent.disabled = !canMutate;
        if (canMutate) {
          mutParent.classList.remove('opacity-40', 'cursor-not-allowed');
          if (typeof mutParent.removeAttribute === 'function') mutParent.removeAttribute('title');
          mutParent.title = '';
        } else {
          mutParent.classList.add('opacity-40', 'cursor-not-allowed');
          const titleMsg = 'Mutations are only available for single-table queries containing a Primary Key';
          if (typeof mutParent.setAttribute === 'function') mutParent.setAttribute('title', titleMsg);
          mutParent.title = titleMsg;
        }
      }

      const btnCopyCell = document.getElementById('grid-ctx-btn-copy-cell');
      if (btnCopyCell) {
        btnCopyCell.classList.toggle('hidden', cIdx === null || cIdx === undefined || cIdx < 0);
      }

      const isBatch = selectedRowIndices && selectedRowIndices.size > 0 && selectedRowIndices.has(rIdx);
      const selCount = isBatch ? selectedRowIndices.size : 1;
      const delLabel = document.getElementById('grid-ctx-delete-row-label');
      if (delLabel) {
        delLabel.textContent = selCount > 1 ? `Delete ${selCount} Selected Rows` : 'Delete Selected Row';
      }
      const copyTsvLabel = document.getElementById('grid-ctx-copy-tsv-label');
      if (copyTsvLabel) {
        copyTsvLabel.textContent = selCount > 1 ? `Copy ${selCount} Rows as TSV` : 'Copy Row as TSV';
      }
      const copyJsonLabel = document.getElementById('grid-ctx-copy-json-label');
      if (copyJsonLabel) {
        copyJsonLabel.textContent = selCount > 1 ? `Copy ${selCount} Rows as JSON` : 'Copy Row as JSON';
      }
      const copySqlLabel = document.getElementById('grid-ctx-copy-sql-label');
      if (copySqlLabel) {
        copySqlLabel.textContent = selCount > 1 ? `Copy ${selCount} Rows as SQL INSERT` : 'Copy Row as SQL INSERT';
      }
      if (mutLabel) {
        mutLabel.textContent = selCount > 1 ? `Row Operations (${selCount})` : 'Row Operations';
      }

      const filterSec = document.getElementById('grid-ctx-filter-section');
      const filterParent = document.getElementById('grid-ctx-filter-parent');
      const filterColLabel = document.getElementById('grid-ctx-filter-colname');
      const filterOpts = document.getElementById('grid-ctx-filter-options');
      const insertSec = document.getElementById('grid-ctx-insert-section');
      const insertParent = document.getElementById('grid-ctx-insert-parent');
      const smartActions = document.getElementById('grid-ctx-smart-actions');
      const colLabel = document.getElementById('grid-ctx-colname');
      const btnEdit = document.getElementById('grid-ctx-btn-edit');
      const btnJson = document.getElementById('grid-ctx-btn-json');

      if (cIdx >= 0 && currentColumns[cIdx]) {
        const col = currentColumns[cIdx];
        const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
        const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === (activeGridTable || '').toLowerCase());
        const colMeta = (tableMeta?.columns || []).find(c => (c.name || '').toLowerCase() === (col?.name || '').toLowerCase());
        const rawType = (colMeta?.type || col?.type || '').toLowerCase();
        const enumValues = parseEnumValues(colMeta?.type || col?.type);
        const fkInfo = (tableMeta?.foreign_keys || []).find(fk => (fk.column || '').toLowerCase() === (col?.name || '').toLowerCase());

        if (filterSec && filterOpts) {
          if (!isTableViewer) {
            filterSec.classList.add('hidden');
            if (filterParent) filterParent.classList.add('hidden');
          } else {
            filterSec.classList.remove('hidden');
            if (filterParent) filterParent.classList.remove('hidden');
            if (filterColLabel) filterColLabel.textContent = `(${col.name})`;

            const tableName = activeGridTable || (activeTab && activeTab.tableName) || '';
            const stagedKey = stagedKeyFor(tableName || '', rIdx, cIdx);
            const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
            const currentVal = staged ? staged.newVal : (currentResults[rIdx] ? currentResults[rIdx][cIdx] : null);
            const typeCategory = getGridTypeCategory(rawType);

            if (currentVal === null || currentVal === undefined) {
              filterOpts.innerHTML = `
                <button data-col="${escapeHtml(col.name)}" onclick="applyQuickFilterFromCell(this.dataset.col, 'IS NULL', '')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-sky-300 hover:text-white transition">
                  <span class="flex items-center space-x-2 min-w-0">
                    <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-sky-400 text-sm">∅</span>
                    <span class="font-mono whitespace-nowrap">Filter IS NULL</span>
                  </span>
                </button>
                <button data-col="${escapeHtml(col.name)}" onclick="applyQuickFilterFromCell(this.dataset.col, 'IS NOT NULL', '')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-slate-300 hover:text-white transition">
                  <span class="flex items-center space-x-2 min-w-0">
                    <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-emerald-400 text-xs">✓</span>
                    <span class="font-mono whitespace-nowrap">Filter IS NOT NULL</span>
                  </span>
                </button>
              `;
            } else {
              const valStr = String(currentVal);
              const displayStr = valStr.length > 20 ? valStr.substring(0, 18) + '…' : valStr;
              let rangeButtons = '';
              if (typeCategory === 'numeric' || typeCategory === 'temporal') {
                rangeButtons = `
                  <button data-col="${escapeHtml(col.name)}" data-val="${escapeHtml(valStr)}" onclick="applyQuickFilterFromCell(this.dataset.col, '>', this.dataset.val)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-slate-300 hover:text-white transition">
                    <span class="flex items-center space-x-2 min-w-0">
                      <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-emerald-400 font-bold text-xs">&gt;</span>
                      <span class="truncate font-mono">${escapeHtml(col.name)} &gt; '${escapeHtml(displayStr)}'</span>
                    </span>
                  </button>
                  <button data-col="${escapeHtml(col.name)}" data-val="${escapeHtml(valStr)}" onclick="applyQuickFilterFromCell(this.dataset.col, '<', this.dataset.val)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-slate-300 hover:text-white transition">
                    <span class="flex items-center space-x-2 min-w-0">
                      <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-emerald-400 font-bold text-xs">&lt;</span>
                      <span class="truncate font-mono">${escapeHtml(col.name)} &lt; '${escapeHtml(displayStr)}'</span>
                    </span>
                  </button>
                `;
              }
              let likeButton = '';
              if (typeCategory === 'text') {
                likeButton = `
                  <button data-col="${escapeHtml(col.name)}" data-val="${escapeHtml(valStr)}" onclick="applyQuickFilterFromCell(this.dataset.col, 'LIKE', this.dataset.val)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-purple-300 hover:text-white transition">
                    <span class="flex items-center space-x-2 min-w-0">
                      <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-purple-400 font-bold text-sm">*</span>
                      <span class="truncate font-mono">${escapeHtml(col.name)} LIKE '%${escapeHtml(displayStr)}%'</span>
                    </span>
                  </button>
                `;
              }
              filterOpts.innerHTML = `
                <button data-col="${escapeHtml(col.name)}" data-val="${escapeHtml(valStr)}" onclick="applyQuickFilterFromCell(this.dataset.col, '=', this.dataset.val)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-sky-300 hover:text-white transition">
                  <span class="flex items-center space-x-2 min-w-0">
                    <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-sky-400 font-bold text-xs">=</span>
                    <span class="truncate font-mono">${escapeHtml(col.name)} = '${escapeHtml(displayStr)}'</span>
                  </span>
                </button>
                <button data-col="${escapeHtml(col.name)}" data-val="${escapeHtml(valStr)}" onclick="applyQuickFilterFromCell(this.dataset.col, '!=', this.dataset.val)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-rose-300 hover:text-white transition">
                  <span class="flex items-center space-x-2 min-w-0">
                    <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-rose-400 font-bold text-xs">!=</span>
                    <span class="truncate font-mono">${escapeHtml(col.name)} != '${escapeHtml(displayStr)}'</span>
                  </span>
                </button>
                ${rangeButtons}
                ${likeButton}
              `;
            }
          }
        }

        let canEditCell = canMutate;

        if (insertSec) insertSec.classList.toggle('hidden', !canEditCell);
        if (insertParent) insertParent.classList.toggle('hidden', !canEditCell);
        if (smartActions) smartActions.classList.toggle('hidden', (!isTableViewer) && (!canEditCell));
        if (colLabel) colLabel.textContent = `(${col.name})`;
        if (btnEdit) btnEdit.classList.toggle('hidden', !canEditCell);
        if (btnJson) btnJson.classList.toggle('hidden', !canEditCell);

        const optsContainer = document.getElementById('grid-ctx-insert-options');
        if (optsContainer) {
          if (enumValues && enumValues.length > 0) {
            optsContainer.innerHTML = `
              <button onclick="insertGridCellValue(null)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-sky-300 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-sky-400 text-sm">∅</span>
                  <span class="truncate font-mono">Set NULL</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+N</span>
              </button>
              ${enumValues.map(v => `
                <button data-val="${escapeHtml(v)}" onclick="insertGridCellValue(this.dataset.val)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-slate-200 hover:text-white transition">
                  <span class="flex items-center space-x-2 min-w-0">
                    <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center text-sky-400 text-xs">●</span>
                    <span class="truncate font-mono">${escapeHtml(v)}</span>
                  </span>
                </button>
              `).join('')}
            `;
          } else if (rawType === 'boolean' || rawType === 'bool' || rawType === 'tinyint(1)') {
            optsContainer.innerHTML = `
              <button onclick="insertGridCellValue(null)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-sky-300 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-sky-400 text-sm">∅</span>
                  <span class="truncate font-mono">Set NULL</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+N</span>
              </button>
              <button onclick="insertGridCellValue('1')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-emerald-400 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-emerald-400 text-xs">✓</span>
                  <span class="truncate font-mono">1 (TRUE)</span>
                </span>
              </button>
              <button onclick="insertGridCellValue('0')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-rose-400 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-rose-400 text-xs">✕</span>
                  <span class="truncate font-mono">0 (FALSE)</span>
                </span>
              </button>
            `;
          } else if (fkInfo && fkInfo.referenced_table && fkInfo.referenced_column) {
            optsContainer.innerHTML = `
              <div class="px-2.5 py-1 text-[10px] text-slate-400 font-mono flex items-center space-x-2 select-none">
                <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center text-xs">${renderIcon('link', 'w-3.5 h-3.5 text-sky-400')}</span>
                <span class="truncate">References: <span class="text-sky-400 font-semibold">${escapeHtml(fkInfo.referenced_table)}.${escapeHtml(fkInfo.referenced_column)}</span></span>
              </div>
              <button onclick="insertGridCellValue(null)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-sky-300 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-sky-400 text-sm">∅</span>
                  <span class="truncate font-mono">Set NULL</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+N</span>
              </button>
              <button onclick="insertGridCellValue('')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-slate-300 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-slate-400 font-bold text-xs">""</span>
                  <span class="truncate font-mono">Set Empty String</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+E</span>
              </button>
            `;
          } else {
            optsContainer.innerHTML = `
              <button onclick="insertGridCellValue(null)" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-sky-300 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-sky-400 text-sm">∅</span>
                  <span class="truncate font-mono">Set NULL</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+N</span>
              </button>
              <button onclick="insertGridCellValue('')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-slate-300 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center font-mono text-slate-400 font-bold text-xs">""</span>
                  <span class="truncate font-mono">Set Empty String</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+E</span>
              </button>
              <button onclick="insertGridCellValue('NOW')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-emerald-400 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center text-xs">${renderIcon('clock', 'w-3.5 h-3.5 text-indigo-400')}</span>
                  <span class="truncate font-mono">Current Timestamp (NOW)</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+T</span>
              </button>
              <button onclick="insertGridCellValue('DATE')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-teal-400 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center text-xs">${renderIcon('calendar', 'w-3.5 h-3.5 text-indigo-400')}</span>
                  <span class="truncate font-mono">Current Date</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">Ctrl+Shift+D</span>
              </button>
              <button onclick="insertGridCellValue('UUID')" class="w-full text-left px-2.5 py-1.5 hover:bg-slate-800 rounded flex items-center justify-between text-purple-400 hover:text-white transition">
                <span class="flex items-center space-x-2 min-w-0">
                  <span class="w-5 text-center flex-shrink-0 inline-flex items-center justify-center text-xs">${renderIcon('key', 'w-3.5 h-3.5 text-amber-400')}</span>
                  <span class="truncate font-mono">UUID()</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono flex-shrink-0 ml-2">v4</span>
              </button>
            `;
          }
        }
      } else {
        if (filterSec) filterSec.classList.add('hidden');
        if (filterParent) filterParent.classList.add('hidden');
        if (insertSec) insertSec.classList.add('hidden');
        if (insertParent) insertParent.classList.add('hidden');
        if (smartActions) smartActions.classList.add('hidden');
        if (btnEdit) btnEdit.classList.add('hidden');
        if (btnJson) btnJson.classList.add('hidden');
      }

      menu.classList.remove('hidden');
      closeAllGridSubmenus();

      const pad = 8;
      const rect = (typeof menu.getBoundingClientRect === 'function')
        ? menu.getBoundingClientRect()
        : { width: 220, height: 220 };
      const menuWidth = rect.width || 220;
      const menuHeight = rect.height || 220;

      const clientX = (e && typeof e.clientX === 'number') ? e.clientX : 0;
      const clientY = (e && typeof e.clientY === 'number') ? e.clientY : 0;
      const winW = (typeof window !== 'undefined' && window.innerWidth) ? window.innerWidth : 1200;
      const winH = (typeof window !== 'undefined' && window.innerHeight) ? window.innerHeight : 800;

      // Ensure menu never exceeds viewport height
      const maxH = Math.max(120, winH - (pad * 2));
      menu.style.maxHeight = `${maxH}px`;
      menu.style.overflowY = 'auto';

      let x = clientX;
      if (x + menuWidth > winW - pad) {
        x = Math.max(pad, winW - menuWidth - pad);
      } else {
        x = Math.max(pad, x);
      }

      let y = clientY;
      if (y + menuHeight > winH - pad) {
        if (clientY - menuHeight >= pad) {
          y = clientY - menuHeight;
        } else {
          y = Math.max(pad, winH - menuHeight - pad);
        }
      } else {
        y = Math.max(pad, y);
      }

      menu.style.left = `${Math.round(x)}px`;
      menu.style.top = `${Math.round(y)}px`;
    }

    // --- Submenu Management for Grid Context Menu ---
    let activeGridSubmenuTimer = null;
    let activeGridSubmenuId = null;

    function closeAllGridSubmenus() {
      if (activeGridSubmenuTimer) {
        clearTimeout(activeGridSubmenuTimer);
        activeGridSubmenuTimer = null;
      }
      const submenus = (typeof document !== 'undefined' && document.querySelectorAll) ? document.querySelectorAll('.grid-submenu') : [];
      if (submenus && submenus.forEach) {
        submenus.forEach(sub => {
          if (sub && sub.classList) sub.classList.add('hidden');
        });
      }
      activeGridSubmenuId = null;
    }

    function cancelCloseGridSubmenus() {
      if (activeGridSubmenuTimer) {
        clearTimeout(activeGridSubmenuTimer);
        activeGridSubmenuTimer = null;
      }
    }

    function scheduleCloseGridSubmenus() {
      if (activeGridSubmenuTimer) {
        clearTimeout(activeGridSubmenuTimer);
      }
      activeGridSubmenuTimer = setTimeout(() => {
        closeAllGridSubmenus();
      }, 180);
    }

    function openGridSubmenu(submenuId, triggerEl) {
      if (!triggerEl) return;
      if (triggerEl.disabled || (triggerEl.classList && triggerEl.classList.contains('cursor-not-allowed'))) {
        closeAllGridSubmenus();
        return;
      }
      cancelCloseGridSubmenus();

      // Hide other open submenus
      const submenus = (typeof document !== 'undefined' && document.querySelectorAll) ? document.querySelectorAll('.grid-submenu') : [];
      if (submenus && submenus.forEach) {
        submenus.forEach(s => {
          if (s && s.id !== submenuId && s.classList) s.classList.add('hidden');
        });
      }

      const sub = document.getElementById(submenuId);
      if (!sub) return;

      sub.classList.remove('hidden');
      activeGridSubmenuId = submenuId;

      const triggerRect = (typeof triggerEl.getBoundingClientRect === 'function')
        ? triggerEl.getBoundingClientRect()
        : { left: 0, right: 220, top: 0, bottom: 30 };
      const pad = 6;
      const subRect = (typeof sub.getBoundingClientRect === 'function')
        ? sub.getBoundingClientRect()
        : { width: 220, height: 160 };
      const subWidth = subRect.width || 220;
      const subHeight = subRect.height || 160;

      const winW = (typeof window !== 'undefined' && window.innerWidth) ? window.innerWidth : 1200;
      const winH = (typeof window !== 'undefined' && window.innerHeight) ? window.innerHeight : 800;

      // Horizontal: open to right by default; flip to left if overflows screen
      let left = triggerRect.right + 2;
      if (left + subWidth > winW - pad) {
        left = triggerRect.left - subWidth - 2;
      }
      if (left < pad) {
        left = Math.max(pad, winW - subWidth - pad);
      }

      // Vertical: align with trigger top; clamp if exceeds bottom
      let top = triggerRect.top;
      if (top + subHeight > winH - pad) {
        top = Math.max(pad, winH - subHeight - pad);
      }
      if (top < pad) {
        top = pad;
      }

      sub.style.maxHeight = `${Math.max(120, winH - (pad * 2))}px`;
      sub.style.overflowY = 'auto';
      sub.style.left = `${Math.round(left)}px`;
      sub.style.top = `${Math.round(top)}px`;
    }

    function toggleGridSubmenu(submenuId, triggerEl) {
      const sub = document.getElementById(submenuId);
      if (!sub) return;
      if (sub.classList.contains('hidden')) {
        openGridSubmenu(submenuId, triggerEl);
      } else {
        sub.classList.add('hidden');
        activeGridSubmenuId = null;
      }
    }

    function applyQuickFilterFromCell(colName, op, val) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.isTableViewer) return;

      if (typeof setFilterBarMode === 'function' && typeof filterBarMode !== 'undefined' && filterBarMode === 'raw') {
        setFilterBarMode('pills');
      }
      if (!Array.isArray(activeTab.filterPills)) {
        activeTab.filterPills = [];
      }
      const rawVal = val !== undefined && val !== null ? String(val) : '';
      activeTab.filterPills.push({
        col: colName,
        column: colName,
        op: op,
        operator: op,
        val: rawVal,
        value: rawVal
      });
      if (typeof renderFilterPills === 'function') {
        renderFilterPills(activeTab);
      }
      if (typeof applyTableFilter === 'function') {
        applyTableFilter();
      }
      closeGridContextMenu();
    }

    function closeGridContextMenu() {
      const menu = document.getElementById('grid-context-menu');
      if (menu) menu.classList.add('hidden');
      closeAllGridSubmenus();
    }

    document.addEventListener('click', (e) => {
      if (e.target && e.target.closest) {
        if (!e.target.closest('#grid-context-menu') && !e.target.closest('.grid-submenu')) {
          closeGridContextMenu();
        }
      } else {
        closeGridContextMenu();
      }
    });

    // --- Vertical Record Inspector (Single Record Form View) ---
    let activeInspectorRowIdx = -1;

    function openRecordInspector(rIdx) {
      if (!currentResults || currentResults.length === 0) {
        if (typeof showNativeAlert === 'function') {
          showNativeAlert('No records to inspect in the active table.', 'Record Inspector');
        }
        return;
      }
      const targetIdx = Math.max(0, Math.min(rIdx !== undefined && rIdx !== null ? rIdx : 0, currentResults.length - 1));
      activeInspectorRowIdx = targetIdx;

      const dlg = document.getElementById('record-inspector-dialog');
      if (!dlg) return;

      const activeTab = tabs.find(t => t.id === activeTabId);
      const tableName = activeGridTable || (activeTab && activeTab.tableName) || 'Record';
      const tblBadge = document.getElementById('record-inspector-table-badge');
      if (tblBadge) tblBadge.textContent = tableName;

      renderRecordInspectorFields();
      if (!dlg.open && typeof dlg.showModal === 'function') {
        dlg.showModal();
      }
    }

    function openRecordInspectorFromContextMenu() {
      const rIdx = (typeof activeGridRightClickedRow === 'number' && activeGridRightClickedRow >= 0)
        ? activeGridRightClickedRow
        : ((typeof activeGridSelectedRow === 'number' && activeGridSelectedRow >= 0) ? activeGridSelectedRow : 0);
      openRecordInspector(rIdx);
    }

    function closeRecordInspector() {
      const dlg = document.getElementById('record-inspector-dialog');
      if (dlg && (dlg.open || typeof dlg.close === 'function')) {
        dlg.close();
      }
      activeInspectorRowIdx = -1;
    }

    function navigateRecordInspector(delta) {
      if (!currentResults || currentResults.length === 0) return;
      const newIdx = activeInspectorRowIdx + delta;
      if (newIdx >= 0 && newIdx < currentResults.length) {
        activeInspectorRowIdx = newIdx;
        activeGridSelectedRow = newIdx;
        renderRecordInspectorFields();
      }
    }

    function filterRecordInspectorFields(query) {
      renderRecordInspectorFields(query);
    }

    function renderRecordInspectorFields(filterText = '') {
      const container = document.getElementById('record-inspector-fields');
      const rowInfo = document.getElementById('record-inspector-row-info');
      const prevBtn = document.getElementById('record-inspector-prev');
      const nextBtn = document.getElementById('record-inspector-next');
      if (!container || !currentColumns || !currentResults) return;

      const rIdx = activeInspectorRowIdx >= 0 ? activeInspectorRowIdx : 0;
      const row = currentResults[rIdx];
      if (!row) return;

      if (rowInfo) rowInfo.textContent = `Row ${rIdx + 1} of ${currentResults.length}`;
      if (prevBtn) prevBtn.disabled = rIdx <= 0;
      if (nextBtn) nextBtn.disabled = rIdx >= currentResults.length - 1;

      const activeTab = tabs.find(t => t.id === activeTabId);
      const tableName = activeGridTable || (activeTab && activeTab.tableName) || '';
      const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === (tableName || '').toLowerCase());
      const fkMap = new Map();
      if (tableMeta && Array.isArray(tableMeta.foreign_keys)) {
        tableMeta.foreign_keys.forEach(fk => {
          if (fk.column && fk.referenced_table && fk.referenced_column) fkMap.set(fk.column, fk);
        });
      }

      const query = (filterText || '').toLowerCase().trim();

      const items = currentColumns.map((col, cIdx) => ({ col, cIdx })).filter(({ col }) => {
        if (!query) return true;
        return col.name.toLowerCase().includes(query) || (col.type || '').toLowerCase().includes(query);
      });

      if (items.length === 0) {
        container.innerHTML = '<div class="p-6 text-center text-slate-500 italic">No columns match filter</div>';
        return;
      }

      container.innerHTML = items.map(({ col, cIdx }) => {
        const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
        const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
        const currentVal = staged ? staged.newVal : row[cIdx];
        const isModified = Boolean(staged);
        const isPK = Boolean(activeGridPKCol && col.name.toLowerCase() === activeGridPKCol.toLowerCase());
        const fkInfo = fkMap.get(col.name);
        const typeCat = getGridTypeCategory(col.type);

        const pkBadge = isPK ? '<span class="px-1.5 py-0.5 rounded bg-emerald-950 text-emerald-300 font-mono text-[10px] font-bold border border-emerald-800">🔑 PK</span>' : '';
        const fkBadge = fkInfo ? `<button type="button" data-table="${escapeHtml(tableName)}" data-col="${escapeHtml(col.name)}" data-reftable="${escapeHtml(fkInfo.referenced_table)}" data-refcol="${escapeHtml(fkInfo.referenced_column)}" data-val="${escapeHtml(String(currentVal ?? ''))}" onclick="peekForeignKey(event, this.dataset.table, this.dataset.col, this.dataset.reftable, this.dataset.refcol, this.dataset.val)" class="px-1.5 py-0.5 rounded bg-sky-950 text-sky-300 font-mono text-[10px] hover:bg-sky-900 border border-sky-800 cursor-pointer" title="Peek referenced record">🔗 ${escapeHtml(fkInfo.referenced_table)}.${escapeHtml(fkInfo.referenced_column)}</button>` : '';
        const modBadge = isModified ? '<span class="text-amber-400 font-bold text-[10px] flex items-center space-x-1"><span>●</span><span>Modified</span></span>' : '';

        let valueFieldHTML = '';
        if (typeCat === 'json') {
          let prettyJSON = '';
          try {
            const parsed = typeof currentVal === 'string' ? JSON.parse(currentVal) : currentVal;
            prettyJSON = JSON.stringify(parsed, null, 2);
          } catch (e) {
            prettyJSON = String(currentVal ?? '');
          }
          valueFieldHTML = `
            <div class="space-y-1">
              <textarea data-col="${escapeHtml(col.name)}" data-cidx="${cIdx}" onchange="onRecordFieldChange(this.dataset.col, parseInt(this.dataset.cidx, 10), this.value)" class="w-full bg-slate-950 text-amber-300 border ${isModified ? 'border-amber-500' : 'border-slate-800'} rounded p-2 text-xs font-mono outline-none focus:border-amber-400 h-24 resize-y">${escapeHtml(prettyJSON)}</textarea>
              <div class="flex items-center justify-between">
                <button type="button" onclick="closeRecordInspector(); openJsonCellModal(${rIdx}, ${cIdx})" class="text-[11px] text-amber-400 hover:text-amber-300 font-mono font-medium flex items-center space-x-1">
                  <span>{ }</span><span>Open Dedicated JSON Modal</span>
                </button>
              </div>
            </div>
          `;
        } else if (typeCat === 'boolean') {
          const isTrue = currentVal === true || currentVal === 1 || currentVal === '1' || currentVal === 'true';
          const isNull = currentVal === null || currentVal === undefined;
          valueFieldHTML = `
            <div class="flex items-center space-x-2">
              <button type="button" data-col="${escapeHtml(col.name)}" data-cidx="${cIdx}" onclick="onRecordFieldChange(this.dataset.col, parseInt(this.dataset.cidx, 10), '1')" class="px-2.5 py-1 rounded text-xs font-mono font-semibold transition ${isTrue ? 'bg-emerald-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'}">✓ TRUE (1)</button>
              <button type="button" data-col="${escapeHtml(col.name)}" data-cidx="${cIdx}" onclick="onRecordFieldChange(this.dataset.col, parseInt(this.dataset.cidx, 10), '0')" class="px-2.5 py-1 rounded text-xs font-mono font-semibold transition ${!isTrue && !isNull ? 'bg-rose-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'}">✕ FALSE (0)</button>
              <button type="button" data-col="${escapeHtml(col.name)}" data-cidx="${cIdx}" onclick="onRecordFieldChange(this.dataset.col, parseInt(this.dataset.cidx, 10), null)" class="px-2 py-1 rounded text-xs font-mono transition ${isNull ? 'bg-slate-700 text-amber-300 font-bold' : 'bg-slate-800 text-slate-400 hover:text-white'}">∅ NULL</button>
            </div>
          `;
        } else if (typeCat === 'text' && String(currentVal || '').length > 60) {
          valueFieldHTML = `
            <textarea data-col="${escapeHtml(col.name)}" data-cidx="${cIdx}" onchange="onRecordFieldChange(this.dataset.col, parseInt(this.dataset.cidx, 10), this.value)" class="w-full bg-slate-950 text-slate-200 border ${isModified ? 'border-amber-500' : 'border-slate-800'} rounded p-2 text-xs font-mono outline-none focus:border-sky-400 h-20 resize-y">${escapeHtml(String(currentVal ?? ''))}</textarea>
          `;
        } else {
          const valDisplay = currentVal === null || currentVal === undefined ? '' : String(currentVal);
          valueFieldHTML = `
            <div class="flex items-center space-x-2">
              <input type="text" value="${escapeHtml(valDisplay)}" placeholder="${currentVal === null ? 'NULL' : ''}"
                     data-col="${escapeHtml(col.name)}" data-cidx="${cIdx}"
                     onchange="onRecordFieldChange(this.dataset.col, parseInt(this.dataset.cidx, 10), this.value)"
                     class="w-full bg-slate-950 text-slate-200 border ${isModified ? 'border-amber-500' : 'border-slate-800'} rounded px-2.5 py-1 text-xs font-mono outline-none focus:border-sky-400" />
              <button type="button" data-col="${escapeHtml(col.name)}" data-cidx="${cIdx}" onclick="onRecordFieldChange(this.dataset.col, parseInt(this.dataset.cidx, 10), null)" class="text-[11px] px-1.5 py-1 bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-amber-300 rounded font-mono shrink-0 transition" title="Set to NULL">∅</button>
            </div>
          `;
        }

        return `
          <div class="pt-2.5 pb-2 grid grid-cols-1 md:grid-cols-[200px,1fr] gap-2 items-start">
            <div class="space-y-1">
              <div class="flex items-center space-x-1.5">
                <span class="font-mono font-semibold text-slate-200 text-xs">${escapeHtml(col.name)}</span>
                ${pkBadge}
              </div>
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="text-[10px] text-slate-500 font-mono">${escapeHtml(col.type || '')}</span>
                ${fkBadge}
                ${modBadge}
              </div>
            </div>
            <div>
              ${valueFieldHTML}
            </div>
          </div>
        `;
      }).join('');
    }

    function onRecordFieldChange(colName, cIdx, newVal) {
      if (activeInspectorRowIdx < 0 || !currentResults || !currentResults[activeInspectorRowIdx]) return;
      const rIdx = activeInspectorRowIdx;
      const activeTab = tabs.find(t => t.id === activeTabId);
      const tableName = activeGridTable || (activeTab && activeTab.tableName) || '';
      const db = activeGridDB || (activeTab && activeTab.database) || '';
      const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);

      const row = currentResults[rIdx];
      const oldVal = row ? row[cIdx] : null;

      // Primary Key resolution (using activeGridPKCol or row values)
      let pkVal = null;
      let pkCol = activeGridPKCol;
      if (pkCol && currentColumns) {
        const pkIdx = currentColumns.findIndex(c => c.name.toLowerCase() === pkCol.toLowerCase());
        if (pkIdx >= 0 && row) pkVal = row[pkIdx];
      }

      if (typeof stagedEdits !== 'undefined' && stagedEdits) {
        const prevStaged = stagedEdits.get(stagedKey);
        const originalVal = prevStaged ? prevStaged.oldVal : oldVal;
        if (newVal === originalVal) {
          // Reverted to the original value: drop the staged edit and undo any inline mutation.
          stagedEdits.delete(stagedKey);
          if (row) row[cIdx] = originalVal;
        } else {
          stageGridEdit(stagedKey, {
            rIdx,
            cIdx,
            db,
            table: tableName,
            colName,
            pkCol,
            pkVal,
            oldVal,
            newVal
          });
        }
      }

      if (typeof updateStagedEditsUI === 'function') updateStagedEditsUI();
      renderRecordInspectorFields(document.getElementById('record-inspector-search')?.value || '');
      renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB, true);
    }

    function copyRecordAsJSON() {
      if (activeInspectorRowIdx < 0 || !currentResults || !currentResults[activeInspectorRowIdx]) return;
      const row = currentResults[activeInspectorRowIdx];
      const obj = {};
      const tableName = activeGridTable || '';
      currentColumns.forEach((col, cIdx) => {
        const stagedKey = stagedKeyFor(tableName, activeInspectorRowIdx, cIdx);
        const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
        obj[col.name] = staged ? staged.newVal : row[cIdx];
      });
      const str = JSON.stringify(obj, null, 2);
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(str).then(() => {
          if (typeof showNativeAlert === 'function') showNativeAlert('Record copied to clipboard as JSON', 'Clipboard');
        }).catch(() => {
          fallbackCopyTextToClipboard(str);
        });
      } else {
        fallbackCopyTextToClipboard(str);
      }
    }

    function copyRecordAsText() {
      if (activeInspectorRowIdx < 0 || !currentResults || !currentResults[activeInspectorRowIdx]) return;
      const row = currentResults[activeInspectorRowIdx];
      const tableName = activeGridTable || '';
      const lines = currentColumns.map((col, cIdx) => {
        const stagedKey = stagedKeyFor(tableName, activeInspectorRowIdx, cIdx);
        const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
        const val = staged ? staged.newVal : row[cIdx];
        return `${col.name}: ${val === null ? 'NULL' : String(val)}`;
      });
      const str = lines.join('\n');
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(str).then(() => {
          if (typeof showNativeAlert === 'function') showNativeAlert('Record copied to clipboard as text', 'Clipboard');
        }).catch(() => {
          fallbackCopyTextToClipboard(str);
        });
      } else {
        fallbackCopyTextToClipboard(str);
      }
    }

    // Keyboard Shortcuts: Space to open Inspector, Arrow Left/Right to navigate
    document.addEventListener('keydown', (e) => {
      if (e.target && (e.target.matches('input, textarea, select') || e.target.isContentEditable)) {
        return;
      }

      const dlg = document.getElementById('record-inspector-dialog');
      if (dlg && dlg.open) {
        if (e.key === 'ArrowLeft') {
          e.preventDefault();
          navigateRecordInspector(-1);
        } else if (e.key === 'ArrowRight') {
          e.preventDefault();
          navigateRecordInspector(1);
        } else if (e.key === 'Escape') {
          closeRecordInspector();
        }
        return;
      }

      if (e.key === ' ' || e.code === 'Space') {
        const activeTab = tabs.find(t => t.id === activeTabId);
        if (activeTab && activeTab.isTableViewer && currentResults && currentResults.length > 0) {
          e.preventDefault();
          const targetRow = (typeof activeGridSelectedRow === 'number' && activeGridSelectedRow >= 0) ? activeGridSelectedRow : 0;
          openRecordInspector(targetRow);
        }
      }
    });

    // --- JSON Cell Viewer Modal ---
    let activeJsonCellContext = null;

    function openJsonCellModal(rIdx, cIdx) {
      const dlg = document.getElementById('json-cell-dialog');
      if (!dlg) return;

      const col = currentColumns[cIdx];
      const colName = col ? col.name : `Col #${cIdx}`;
      const activeTab = tabs.find(t => t.id === activeTabId);
      const tableName = activeGridTable || (activeTab && activeTab.tableName) || '';

      const stagedKey = stagedKeyFor(tableName || '', rIdx, cIdx);
      const staged = stagedEdits.get(stagedKey);
      const rawVal = staged ? staged.newVal : (currentResults[rIdx] ? currentResults[rIdx][cIdx] : null);

      activeJsonCellContext = {
        rIdx,
        cIdx,
        colName,
        tableName,
        rawVal
      };

      const colBadge = document.getElementById('json-cell-col-badge');
      if (colBadge) colBadge.textContent = colName;
      const rowBadge = document.getElementById('json-cell-row-badge');
      if (rowBadge) rowBadge.textContent = `Row #${rIdx + 1}`;

      const editor = document.getElementById('json-cell-editor');
      if (editor) {
        if (rawVal === null || rawVal === undefined) {
          editor.value = 'null';
        } else if (typeof rawVal === 'object') {
          editor.value = JSON.stringify(rawVal, null, 2);
        } else {
          const strVal = String(rawVal);
          try {
            const parsed = JSON.parse(strVal);
            editor.value = JSON.stringify(parsed, null, 2);
          } catch (_) {
            editor.value = strVal;
          }
        }
      }

      validateJsonCellInput();

      const applyBtn = document.getElementById('json-cell-apply-btn');
      if (applyBtn) {
        const canEdit = !activeSession?.read_only;
        applyBtn.style.display = canEdit ? 'inline-flex' : 'none';
      }

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    function openJsonCellModalFromContextMenu() {
      const rIdx = activeGridRightClickedRow ?? activeGridSelectedRow;
      const cIdx = activeGridRightClickedCol ?? activeGridSelectedCol;
      if (rIdx !== null && cIdx !== null) {
        openJsonCellModal(rIdx, cIdx);
      }
    }

    function closeJsonCellModal() {
      const dlg = document.getElementById('json-cell-dialog');
      if (dlg) {
        if (typeof dlg.close === 'function') dlg.close();
        else dlg.removeAttribute('open');
      }
      activeJsonCellContext = null;
    }

    function validateJsonCellInput() {
      const editor = document.getElementById('json-cell-editor');
      const text = editor ? editor.value : '';
      const badge = document.getElementById('json-cell-status-badge');
      const icon = document.getElementById('json-cell-status-icon');
      const textEl = document.getElementById('json-cell-status-text');
      const meta = document.getElementById('json-cell-meta-info');

      const byteLength = new Blob([text]).size;
      const lines = text.split('\n').length;
      if (meta) {
        meta.textContent = `Size: ${formatBytes(byteLength)} | ${lines} line${lines === 1 ? '' : 's'}`;
      }

      if (!text.trim()) {
        if (badge) badge.className = 'text-xs font-mono px-2.5 py-1 rounded bg-slate-950 border border-slate-800 text-slate-400 flex items-center space-x-1.5';
        if (icon) icon.innerHTML = renderIcon('alert-circle', 'w-3 h-3 text-slate-400');
        if (textEl) textEl.textContent = 'Empty';
        return { valid: true, parsed: null };
      }

      try {
        const parsed = JSON.parse(text);
        if (badge) badge.className = 'text-xs font-mono px-2.5 py-1 rounded bg-emerald-950/80 text-emerald-300 border border-emerald-800/60 flex items-center space-x-1.5';
        if (icon) icon.innerHTML = renderIcon('check', 'w-3 h-3 text-emerald-400');

        let summary = 'Primitive';
        if (parsed === null) summary = 'null';
        else if (Array.isArray(parsed)) summary = `Array (${parsed.length} items)`;
        else if (typeof parsed === 'object') summary = `Object (${Object.keys(parsed).length} keys)`;
        else summary = typeof parsed;

        if (textEl) textEl.textContent = `Valid JSON · ${summary}`;
        return { valid: true, parsed };
      } catch (err) {
        if (badge) badge.className = 'text-xs font-mono px-2.5 py-1 rounded bg-rose-950/80 text-rose-300 border border-rose-800/60 flex items-center space-x-1.5';
        if (icon) icon.innerHTML = renderIcon('alert-triangle', 'w-3 h-3 text-rose-400');
        if (textEl) textEl.textContent = `Invalid JSON: ${err.message}`;
        return { valid: false, error: err.message };
      }
    }

    function formatJsonCellContent() {
      const editor = document.getElementById('json-cell-editor');
      if (!editor) return;
      try {
        const parsed = JSON.parse(editor.value);
        editor.value = JSON.stringify(parsed, null, 2);
        validateJsonCellInput();
      } catch (err) {
        showNativeAlert('Cannot format invalid JSON: ' + err.message, 'Invalid JSON');
      }
    }

    function minifyJsonCellContent() {
      const editor = document.getElementById('json-cell-editor');
      if (!editor) return;
      try {
        const parsed = JSON.parse(editor.value);
        editor.value = JSON.stringify(parsed);
        validateJsonCellInput();
      } catch (err) {
        showNativeAlert('Cannot minify invalid JSON: ' + err.message, 'Invalid JSON');
      }
    }

    async function copyJsonCellContent() {
      const editor = document.getElementById('json-cell-editor');
      if (!editor) return;
      const ok = await copyTextToClipboard(editor.value);
      if (ok) {
        const textEl = document.getElementById('json-cell-status-text');
        if (textEl) {
          const old = textEl.textContent;
          textEl.textContent = '📋 Copied to clipboard!';
          setTimeout(() => { if (textEl) textEl.textContent = old; }, 1500);
        }
      } else {
        showNativeAlert('Failed to copy to clipboard in this browser context.', 'Clipboard Error');
      }
    }

    async function applyJsonCellChanges() {
      if (!activeJsonCellContext) return;
      const editor = document.getElementById('json-cell-editor');
      const val = editor ? editor.value : '';

      const check = validateJsonCellInput();
      if (!check.valid) {
        const proceed = await showNativeConfirm(`JSON syntax error: ${check.error}\n\nSave anyway as raw string?`, 'Invalid JSON Syntax');
        if (!proceed) return;
      }

      const { rIdx, cIdx } = activeJsonCellContext;
      await applyJsonValueToGrid(rIdx, cIdx, val);
      closeJsonCellModal();
    }

    async function applyJsonValueToGrid(rIdx, cIdx, val) {
      if (activeSession && activeSession.read_only) {
        showNativeAlert('Session is in READ-ONLY mode.', 'Read-Only');
        return;
      }

      const activeTab = tabs.find(t => t.id === activeTabId);
      let tableName = activeGridTable || (activeTab && activeTab.tableName) || '';
      if (!tableName) {
        const sql = (activeTab && activeTab.sql) || getEditorSQL();
        const parsed = extractSingleTableFromSQL(sql);
        if (parsed && parsed.table) {
          tableName = parsed.table;
          activeGridTable = tableName;
          if (parsed.database) activeGridDB = parsed.database;
        }
      }
      if (!tableName) {
        showNativeAlert('Inline editing is only available for single-table queries.', 'Edit Cell');
        return;
      }

      let pkCol = activeGridPKCol;
      if (!pkCol && tableName) {
        const targetCatalog = (activeGridDB && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeGridDB.toLowerCase()]) || (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
        const tableMeta = (targetCatalog?.tables || []).find(t => (t.name || '').toLowerCase() === tableName.toLowerCase());
        if (tableMeta && Array.isArray(tableMeta.columns)) {
          const pks = tableMeta.columns.filter(c => c.is_primary_key);
          if (pks.length === 1) {
            pkCol = pks[0].name;
            activeGridPKCol = pkCol;
          }
        }
      }

      const col = currentColumns[cIdx];
      const pkIdx = pkCol ? currentColumns.findIndex(c => (c.name || '').toLowerCase() === pkCol.toLowerCase()) : -1;
      const pkVal = pkIdx !== -1 ? currentResults[rIdx][pkIdx] : null;

      const rowCriteria = {};
      if (pkIdx === -1) {
        currentColumns.forEach((c, idx) => {
          rowCriteria[c.name] = currentResults[rIdx][idx];
        });
      }

      const dbName = activeGridDB || (activeTab && activeTab.database) || '';
      const td = document.querySelector(`#grid-tbody td[data-row="${rIdx}"][data-col="${cIdx}"]`);

      if (gridEditMode === 'auto') {
        if (gridConfirmSave) {
          const colName = col ? col.name : `Column #${cIdx}`;
          const ok = await showNativeConfirm(`Save changes to ${tableName}.${colName}?`, 'Confirm Save');
          if (!ok) return;
        }

        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/cell-edit`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              database: dbName,
              table: tableName,
              column: col.name,
              value: val,
              pk_column: pkIdx !== -1 ? pkCol : '',
              pk_value: pkVal,
              row_criteria: pkIdx === -1 ? rowCriteria : undefined
            })
          });
          if (res.ok) {
            currentResults[rIdx][cIdx] = val;
            if (td) {
              renderCellContent(td, rIdx, cIdx, pkCol, tableName);
              td.classList.add('bg-emerald-950/40', 'border-emerald-600/60');
              setTimeout(() => td.classList.remove('bg-emerald-950/40', 'border-emerald-600/60'), 1500);
            }
          } else {
            const err = await res.json().catch(() => ({}));
            showNativeAlert('Failed to save cell: ' + (err.error || res.statusText), 'Save Error');
            if (td) renderCellContent(td, rIdx, cIdx, pkCol, tableName);
          }
        } catch (err) {
          showNativeAlert('Network error saving cell: ' + err.message, 'Save Error');
        }
      } else {
        const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
        stageGridEdit(stagedKey, {
          rIdx,
          cIdx,
          db: dbName,
          table: tableName,
          colName: col.name,
          pkCol: pkIdx !== -1 ? pkCol : '',
          pkVal,
          rowCriteria: pkIdx === -1 ? rowCriteria : undefined,
          oldVal: currentResults[rIdx][cIdx],
          newVal: val
        });
        currentResults[rIdx][cIdx] = val;
        if (td) renderCellContent(td, rIdx, cIdx, pkCol, tableName);
        updateStagedEditsUI();
      }
    }

    // --- Clipboard Formats: TSV & Markdown Table ---
    function getGridExportRows(selectedOnly = null) {
      if (selectedOnly === true && selectedRowIndices && selectedRowIndices.size > 0) {
        return Array.from(selectedRowIndices).map(idx => currentResults[idx]);
      }
      return currentResults || [];
    }

    async function copyGridAsTSV() {
      if (!currentColumns || currentColumns.length === 0) {
        showNativeAlert('No data to copy.', 'Copy TSV');
        return;
      }
      const rows = getGridExportRows();
      if (rows.length === 0) {
        showNativeAlert('No rows to copy.', 'Copy TSV');
        return;
      }

      const headers = currentColumns.map(c => c.name).join('\t');
      const body = rows.map(r => r.map(v => v === null ? '' : String(v).replace(/\t/g, ' ')).join('\t')).join('\n');
      const tsv = headers + '\n' + body;

      const ok = await copyTextToClipboard(tsv);
      if (ok) {
        showNativeAlert(`✅ Copied entire grid (${rows.length} rows) to clipboard as TSV!`, 'Copied TSV');
      } else {
        showNativeAlert('Failed to copy to clipboard in this browser context.', 'Clipboard Error');
      }
    }

    async function copyGridAsMarkdown() {
      if (!currentColumns || currentColumns.length === 0) {
        showNativeAlert('No data to copy.', 'Copy Markdown');
        return;
      }
      const rows = getGridExportRows();
      if (rows.length === 0) {
        showNativeAlert('No rows to copy.', 'Copy Markdown');
        return;
      }

      const headers = '| ' + currentColumns.map(c => c.name).join(' | ') + ' |';
      const sep = '| ' + currentColumns.map(() => '---').join(' | ') + ' |';
      const body = rows.map(r => '| ' + r.map(v => v === null ? 'NULL' : String(v).replace(/\|/g, '\\|')).join(' | ') + ' |').join('\n');
      const md = headers + '\n' + sep + '\n' + body;

      const ok = await copyTextToClipboard(md);
      if (ok) {
        showNativeAlert(`✅ Copied entire grid (${rows.length} rows) to clipboard as Markdown!`, 'Copied Markdown');
      } else {
        showNativeAlert('Failed to copy to clipboard in this browser context.', 'Clipboard Error');
      }
    }

    function getTargetGridRowIndices() {
      if (selectedRowIndices && selectedRowIndices.size > 0) {
        if (activeGridRightClickedRow === null || activeGridRightClickedRow === undefined || selectedRowIndices.has(activeGridRightClickedRow)) {
          return Array.from(selectedRowIndices).sort((a, b) => a - b);
        }
      }
      if (activeGridRightClickedRow !== null && activeGridRightClickedRow !== undefined && activeGridRightClickedRow >= 0) {
        return [activeGridRightClickedRow];
      }
      if (selectedRowIndices && selectedRowIndices.size > 0) {
        return Array.from(selectedRowIndices).sort((a, b) => a - b);
      }
      return [];
    }

    async function copyCellValueFromGrid() {
      const rIdx = activeGridRightClickedRow ?? activeGridSelectedRow;
      const cIdx = activeGridRightClickedCol ?? activeGridSelectedCol;
      if (rIdx === null || rIdx === undefined || cIdx === null || cIdx === undefined || rIdx < 0 || cIdx < 0) {
        showToast('No cell selected to copy', 'warning');
        return;
      }
      const row = currentResults && currentResults[rIdx];
      if (!row) return;

      const meta = resolveGridTableMeta();
      const tableName = meta.tableName || activeGridTable || '';
      const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
      const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
      const val = staged ? staged.newVal : row[cIdx];

      const textToCopy = val === null || val === undefined ? '' : (typeof val === 'object' ? JSON.stringify(val) : String(val));
      const ok = await copyTextToClipboard(textToCopy);
      if (ok) {
        const preview = textToCopy === '' ? '(empty)' : (textToCopy.length > 35 ? textToCopy.slice(0, 32) + '...' : textToCopy);
        showToast(`Copied cell: ${preview}`);
      } else {
        showNativeAlert('Failed to copy cell value to clipboard.', 'Clipboard Error');
      }
    }

    async function copySelectedRowsAsTSV() {
      const rowIndices = getTargetGridRowIndices();
      if (rowIndices.length === 0 || !currentColumns || currentColumns.length === 0) {
        showToast('No rows selected to copy', 'warning');
        return;
      }
      const meta = resolveGridTableMeta();
      const tableName = meta.tableName || activeGridTable || '';
      const visibleCols = currentColumns.filter(c => !isColumnHidden(c.name, tableName));

      const lines = rowIndices.map(rIdx => {
        const row = currentResults[rIdx];
        if (!row) return null;
        return visibleCols.map(c => {
          const cIdx = currentColumns.findIndex(col => col.name === c.name);
          if (cIdx === -1) return '';
          const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
          const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
          const v = staged ? staged.newVal : row[cIdx];
          if (v === null || v === undefined) return '';
          return String(v).replace(/\t/g, ' ').replace(/[\r\n]+/g, ' ');
        }).join('\t');
      }).filter(l => l !== null);

      const ok = await copyTextToClipboard(lines.join('\n'));
      if (ok) {
        showToast(`Copied ${lines.length} row(s) as TSV`);
      } else {
        showNativeAlert('Failed to copy to clipboard.', 'Clipboard Error');
      }
    }

    async function copySelectedRowsAsJSON() {
      const rowIndices = getTargetGridRowIndices();
      if (rowIndices.length === 0 || !currentColumns || currentColumns.length === 0) {
        showToast('No rows selected to copy', 'warning');
        return;
      }
      const meta = resolveGridTableMeta();
      const tableName = meta.tableName || activeGridTable || '';
      const visibleCols = currentColumns.filter(c => !isColumnHidden(c.name, tableName));

      const records = rowIndices.map(rIdx => {
        const row = currentResults[rIdx];
        if (!row) return null;
        const obj = {};
        visibleCols.forEach(c => {
          const cIdx = currentColumns.findIndex(col => col.name === c.name);
          if (cIdx === -1) return;
          const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
          const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
          obj[c.name] = staged ? staged.newVal : row[cIdx];
        });
        return obj;
      }).filter(Boolean);

      const payload = records.length === 1 ? records[0] : records;
      const ok = await copyTextToClipboard(JSON.stringify(payload, null, 2));
      if (ok) {
        showToast(`Copied ${records.length} row(s) as JSON`);
      } else {
        showNativeAlert('Failed to copy to clipboard.', 'Clipboard Error');
      }
    }

    async function copySelectedRowsAsSQL() {
      const rowIndices = getTargetGridRowIndices();
      if (rowIndices.length === 0 || !currentColumns || currentColumns.length === 0) {
        showToast('No rows selected to copy', 'warning');
        return;
      }
      const meta = resolveGridTableMeta();
      const tableName = meta.tableName || activeGridTable || 'table_name';
      const visibleCols = currentColumns.filter(c => !isColumnHidden(c.name, tableName));
      const colList = visibleCols.map(c => quoteIdentifierForDriver(c.name)).join(', ');
      const quotedTable = quoteIdentifierForDriver(tableName);

      const sqlStatements = rowIndices.map(rIdx => {
        const row = currentResults[rIdx];
        if (!row) return null;
        const vals = visibleCols.map(c => {
          const cIdx = currentColumns.findIndex(col => col.name === c.name);
          if (cIdx === -1) return 'NULL';
          const stagedKey = stagedKeyFor(tableName, rIdx, cIdx);
          const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
          const v = staged ? staged.newVal : row[cIdx];
          if (v === null || v === undefined) return 'NULL';
          if (typeof v === 'number') return String(v);
          if (typeof v === 'boolean') return v ? 'TRUE' : 'FALSE';
          return `'${String(v).replace(/'/g, "''")}'`;
        }).join(', ');
        return `INSERT INTO ${quotedTable} (${colList}) VALUES (${vals});`;
      }).filter(Boolean);

      const ok = await copyTextToClipboard(sqlStatements.join('\n'));
      if (ok) {
        showToast(`Copied ${sqlStatements.length} row(s) as SQL INSERT`);
      } else {
        showNativeAlert('Failed to copy to clipboard.', 'Clipboard Error');
      }
    }

    function duplicateGridRow(withoutKey = true) {
      if (activeSession && activeSession.read_only) {
        showNativeAlert('Session is in READ-ONLY mode. Unlock first.', 'Read-Only Mode');
        return;
      }
      const meta = resolveGridTableMeta();
      if (!meta.canMutate) {
        showNativeAlert('Duplication is only available for single-table queries containing a Primary Key.', 'Duplicate Row');
        return;
      }
      const targetIndices = getTargetGridRowIndices();
      if (targetIndices.length === 0) {
        showToast('No row selected to duplicate', 'warning');
        return;
      }

      const pkColName = (meta.pkCol || activeGridPKCol || '').toLowerCase();
      const newDrafts = targetIndices.map(idx => {
        const row = currentResults && currentResults[idx];
        if (!row) return null;
        const draftData = {};
        currentColumns.forEach((col, cIdx) => {
          const isPK = pkColName && (col.name || '').toLowerCase() === pkColName;
          if (withoutKey && isPK) {
            // Omit PK value so PostgreSQL / MySQL / SQLite auto-generates key via sequence or auto_increment
          } else {
            const stagedKey = stagedKeyFor(meta.tableName || activeGridTable || '', idx, cIdx);
            const staged = (typeof stagedEdits !== 'undefined' && stagedEdits) ? stagedEdits.get(stagedKey) : null;
            const val = staged ? staged.newVal : row[cIdx];
            if (val !== null && val !== undefined) {
              draftData[col.name] = val;
            }
          }
        });
        return { data: draftData };
      }).filter(Boolean);

      stagedDraftRows.unshift(...newDrafts);
      updateStagedEditsUI();
      renderGridResults(currentColumns, currentResults, meta.pkCol || activeGridPKCol, meta.tableName || activeGridTable, meta.dbName || activeGridDB);
      showToast(`${targetIndices.length} row(s) duplicated as draft (${withoutKey ? 'without key' : 'with key'})`);
    }

