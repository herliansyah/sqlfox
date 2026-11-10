// --- Schema Tree Filtering & Prefix Index (ADR 0058) ---
    let schemaFilterQuery = '';
    let databaseFilterQuery = '';
    let schemaTreeFastIndex = null;

    function buildSchemaTreeFastIndex(schema) {
      if (!schema) {
        schemaTreeFastIndex = null;
        return;
      }
      const tables = [];
      const views = [];
      const triggers = schema.triggers || [];

      (schema.tables || []).forEach(t => {
        const item = {
          ...t,
          columns: t.columns || [],
          _lower: (t.name || '').toLowerCase()
        };
        if (t.type === 'VIEW') views.push(item);
        else tables.push(item);
      });

      schemaTreeFastIndex = {
        _schemaRef: schema,
        database: schema.database_name,
        tables: tables,
        views: views,
        triggers: triggers.map(tr => ({
          ...tr,
          _lower: ((tr.name || '') + ' ' + (tr.table || '')).toLowerCase()
        }))
      };
    }

    function filterSchemaTree(query) {
      schemaFilterQuery = (query || '').toLowerCase().trim();
      const clearBtn = document.getElementById('schema-filter-clear-btn');
      if (clearBtn) {
        if (schemaFilterQuery) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
      // Instant sub-5ms filtering via pre-warmed schema search index without debounce delay (ADR 0058)
      if (schemaCatalogCache) {
        renderSchemaTree(schemaCatalogCache);
      }
    }

    function clearSchemaFilter() {
      const input = document.getElementById('schema-filter-input');
      if (input) {
        input.value = '';
        input.focus();
      }
      schemaFilterQuery = '';
      const clearBtn = document.getElementById('schema-filter-clear-btn');
      if (clearBtn) clearBtn.classList.add('hidden');
      if (schemaCatalogCache) {
        renderSchemaTree(schemaCatalogCache);
      }
    }

    let databaseDropdownSelectedIndex = -1;
    let currentFilteredDatabases = [];

    function toggleDatabaseDropdown(forceState) {
      const menu = document.getElementById('database-dropdown-menu');
      if (!menu) return;
      const shouldOpen = typeof forceState === 'boolean' ? forceState : menu.classList.contains('hidden');
      if (isDatabaseSwitching && shouldOpen) return; // ponytail: prevent opening dropdown during in-flight database switch
      if (shouldOpen) {
        toggleSessionDropdown(false);
        menu.classList.remove('hidden');
        databaseDropdownSelectedIndex = -1;
        const input = document.getElementById('database-search-input');
        const clearBtn = document.getElementById('database-search-clear-btn');
        if (clearBtn) clearBtn.classList.add('hidden');
        if (input) {
          input.value = '';
          databaseFilterQuery = '';
          input.focus();
        }
        renderDatabaseDropdownList();
      } else {
        menu.classList.add('hidden');
      }
    }

    function onDatabaseSearchInput(val) {
      databaseDropdownSelectedIndex = -1;
      const clearBtn = document.getElementById('database-search-clear-btn');
      if (clearBtn) {
        if (val && val.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
      filterDatabaseList(val);
    }

    function onDatabaseSearchKeyDown(e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        toggleDatabaseDropdown(false);
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        if (currentFilteredDatabases.length > 0) {
          databaseDropdownSelectedIndex = (databaseDropdownSelectedIndex + 1) % currentFilteredDatabases.length;
          renderDatabaseDropdownList();
        }
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        if (currentFilteredDatabases.length > 0) {
          databaseDropdownSelectedIndex = databaseDropdownSelectedIndex <= 0
            ? currentFilteredDatabases.length - 1
            : databaseDropdownSelectedIndex - 1;
          renderDatabaseDropdownList();
        }
      } else if (e.key === 'Enter') {
        e.preventDefault();
        const targetIdx = databaseDropdownSelectedIndex >= 0 ? databaseDropdownSelectedIndex : 0;
        if (currentFilteredDatabases.length > 0 && targetIdx < currentFilteredDatabases.length) {
          const selectedDb = currentFilteredDatabases[targetIdx];
          selectDatabase(selectedDb);
          toggleDatabaseDropdown(false);
        }
      }
    }

    function clearDatabaseSearch() {
      databaseDropdownSelectedIndex = -1;
      const input = document.getElementById('database-search-input');
      const clearBtn = document.getElementById('database-search-clear-btn');
      if (input) {
        input.value = '';
        input.focus();
      }
      if (clearBtn) {
        clearBtn.classList.add('hidden');
      }
      filterDatabaseList('');
    }

    function filterDatabaseList(query) {
      databaseFilterQuery = (query || '').toLowerCase().trim();
      renderDatabaseDropdownList();
    }

    function renderDatabaseDropdownList() {
      const listEl = document.getElementById('database-list-tree');
      if (!listEl) return;
      if (!schemaCatalogCache || !schemaCatalogCache.databases || schemaCatalogCache.databases.length === 0) {
        currentFilteredDatabases = [];
        databaseDropdownSelectedIndex = -1;
        listEl.innerHTML = '<div class="text-slate-500 text-center py-2 text-xs italic">No databases found</div>';
        return;
      }

      let dbs = schemaCatalogCache.databases;
      if (databaseFilterQuery) {
        dbs = dbs.filter(d => d.toLowerCase().includes(databaseFilterQuery));
      }

      currentFilteredDatabases = dbs;

      if (dbs.length === 0) {
        databaseDropdownSelectedIndex = -1;
        listEl.innerHTML = `<div class="text-slate-500 text-center py-2 text-xs italic">No databases matching "${escapeHtml(databaseFilterQuery)}"</div>`;
        return;
      }

      if (databaseDropdownSelectedIndex >= dbs.length) {
        databaseDropdownSelectedIndex = dbs.length > 0 ? dbs.length - 1 : -1;
      }

      const activeDb = schemaCatalogCache.database_name || '';
      const connId = activeConnection ? activeConnection.id : '';
      const tsMap = (connId && sessionCatalogTimestamps[connId]) ? sessionCatalogTimestamps[connId] : {};

      listEl.innerHTML = dbs.map((d, idx) => {
        const lower = d.toLowerCase();
        const isAct = lower === activeDb.toLowerCase();
        const isHighlighted = idx === databaseDropdownSelectedIndex;
        const cached = !!(multiDBCatalogCache && multiDBCatalogCache[lower]);
        const ts = tsMap[lower];
        let cacheLabel = '';
        if (cached && ts) {
          const dt = new Date(ts);
          cacheLabel = `${dt.getHours().toString().padStart(2,'0')}:${dt.getMinutes().toString().padStart(2,'0')}`;
        }

        let itemClasses = 'group flex items-center justify-between px-2 py-1.5 rounded cursor-pointer transition ';
        if (isHighlighted) {
          itemClasses += 'bg-sky-900/80 text-white font-semibold ring-1 ring-app-accent';
        } else if (isAct) {
          itemClasses += 'bg-sky-950/60 text-app-accent font-bold border border-sky-800';
        } else {
          itemClasses += 'hover:bg-slate-800 text-slate-300';
        }

        return `
          <div id="database-item-${idx}" onclick="selectDatabase(${jsArg(d)}); toggleDatabaseDropdown(false);" class="${itemClasses}">
            <span class="truncate flex items-center space-x-1.5">
              ${renderIcon('database', 'w-3.5 h-3.5 text-sky-400 shrink-0')}
              <span class="truncate">${escapeHtml(d)}</span>
            </span>
            <div class="flex items-center space-x-1.5 shrink-0">
              ${cacheLabel ? `<span class="text-[9px] text-slate-500 font-mono flex items-center" title="Cached at ${cacheLabel}"><svg class="w-2.5 h-2.5 text-amber-400 inline mr-0.5" aria-hidden="true"><use href="#icon-zap"/></svg>${cacheLabel}</span>` : ''}
              ${isAct ? '<span class="text-[9px] text-sky-400 uppercase font-mono">active</span>' : ''}
              <button onclick="event.stopPropagation(); refreshDatabaseCatalog(${jsArg(d)}, this);" class="opacity-0 group-hover:opacity-100 hover:text-white p-0.5 text-[10px] rounded hover:bg-slate-700 transition" title="Refresh ${escapeHtml(d)} catalog">${renderIcon('refresh-cw', 'w-3 h-3 text-slate-400 hover:text-sky-300')}</button>
            </div>
          </div>
        `;
      }).join('');

      if (databaseDropdownSelectedIndex >= 0) {
        const activeItem = document.getElementById(`database-item-${databaseDropdownSelectedIndex}`);
        if (activeItem) {
          activeItem.scrollIntoView({ block: 'nearest' });
        }
      }
    }

    let isDatabaseSwitching = false;
    async function selectDatabase(dbName) {
      if (!activeConnection || !dbName) return;
      if (isDatabaseSwitching) return; // ponytail: prevent concurrent database switch race conditions
      isDatabaseSwitching = true;
      toggleDatabaseDropdown(false);

      const switcherBtn = document.getElementById('database-switcher-btn');
      const activeNameEl = document.getElementById('database-switcher-active-name');
      const prevDbName = (schemaCatalogCache && schemaCatalogCache.database_name) || (activeNameEl ? activeNameEl.textContent.trim() : '') || 'default';

      if (switcherBtn) {
        switcherBtn.classList.add('cursor-wait', 'opacity-75', 'pointer-events-none');
      }
      if (activeNameEl) {
        activeNameEl.innerHTML = `<span class="inline-flex items-center space-x-1">${renderIcon('loader', 'w-3 h-3 animate-spin text-sky-400 shrink-0')}<span>Switching to ${escapeHtml(dbName)}...</span></span>`;
      }

      // ponytail: adaptive warning timer if server connection is slow (> 3 seconds)
      let slowTimer = setTimeout(() => {
        if (isDatabaseSwitching) {
          if (activeNameEl) {
            activeNameEl.innerHTML = `<span class="inline-flex items-center space-x-1 text-amber-300 font-mono">${renderIcon('loader', 'w-3 h-3 animate-spin text-amber-400 shrink-0')}<span class="truncate">Server responding slowly...</span></span>`;
          }
          showToast('Server responding slowly, still processing database switch...', 'warning');
        }
      }, 3000);

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/select-db`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ database: dbName })
        });
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || res.statusText || 'Failed to select database');
        }
        await loadSchemaTree(activeConnection.id, dbName);
        showToast(`Switched active database to '${dbName}'`, 'success');
      } catch (e) {
        console.warn('select-db error:', e);
        showToast(`Failed to switch database: ${e.message}`, 'error');
        // Safe Revert: restore previous active database name on failure
        if (activeNameEl) {
          activeNameEl.textContent = prevDbName;
        }
        const dbSelect = document.getElementById('database-selector');
        if (dbSelect) {
          dbSelect.value = prevDbName;
        }
      } finally {
        clearTimeout(slowTimer);
        isDatabaseSwitching = false;
        if (switcherBtn) {
          switcherBtn.classList.remove('cursor-wait', 'opacity-75', 'pointer-events-none');
        }
        renderDatabaseDropdownList();
      }
    }

    async function onDatabaseSelectChange(dbName) {
      await selectDatabase(dbName);
    }

    // Dismiss database dropdown and table context menu when clicking outside
    document.addEventListener('click', (e) => {
      const switcherBtn = document.getElementById('database-switcher-btn');
      const menu = document.getElementById('database-dropdown-menu');
      if (menu && !menu.classList.contains('hidden')) {
        if (switcherBtn && !switcherBtn.contains(e.target) && !menu.contains(e.target)) {
          menu.classList.add('hidden');
        }
      }
      if (!e.target.closest('#table-context-menu, #tab-context-menu, #grid-context-menu, #pl-context-menu, #btn-col-picker, #col-picker-dropdown, #pl-btn-col-picker, #pl-col-picker-dropdown, #btn-export-csv, #export-csv-menu, #export-csv-dropdown-container, #tab-database-pill-container, #tab-db-switcher-menu, #tab-overflow-wrapper, #tab-overflow-dropdown')) {
        closeAllContextMenus();
      }
    });

    // Dismiss context menus when right-clicking on unhandled page surfaces
    document.addEventListener('contextmenu', (e) => {
      if (!e.target.closest('#table-context-menu, #tab-context-menu, #grid-context-menu, .grid-submenu, #pl-context-menu, td[data-col], #tabs-container, #schema-tree, .pl-row, #btn-col-picker, #col-picker-dropdown, #pl-btn-col-picker, #pl-col-picker-dropdown, #btn-export-csv, #export-csv-menu, #export-csv-dropdown-container, #tab-database-pill-container, #tab-db-switcher-menu, #tab-overflow-wrapper, #tab-overflow-dropdown')) {
        closeAllContextMenus();
      }
    });

    function closeAllContextMenus() {
      if (typeof closeTabContextMenu === 'function') closeTabContextMenu();
      if (typeof closeTableContextMenu === 'function') closeTableContextMenu();
      if (typeof closeGridContextMenu === 'function') closeGridContextMenu();
      if (typeof closeProcessContextMenu === 'function') closeProcessContextMenu();
      const colPicker = document.getElementById('col-picker-dropdown');
      if (colPicker) colPicker.classList.add('hidden');
      const plColPicker = document.getElementById('pl-col-picker-dropdown');
      if (plColPicker) plColPicker.classList.add('hidden');
      const exportCsv = document.getElementById('export-csv-menu');
      if (exportCsv) exportCsv.classList.add('hidden');
      const tabDbMenu = document.getElementById('tab-db-switcher-menu');
      if (tabDbMenu) tabDbMenu.classList.add('hidden');
      if (typeof closeTabOverflowMenu === 'function') closeTabOverflowMenu();
    }

    // --- Table Context Menu & Actions ---
    let selectedContextMenuTable = null;

    function showTableContextMenu(e, tableName) {
      if (e) {
        e.preventDefault();
        e.stopPropagation();
      }
      closeAllContextMenus();
      selectedContextMenuTable = tableName;
      const menu = document.getElementById('table-context-menu');
      if (!menu) return;

      const x = Math.max(10, Math.min(e.clientX, window.innerWidth - 170));
      const y = Math.max(10, Math.min(e.clientY, window.innerHeight - 130));
      menu.style.left = `${x}px`;
      menu.style.top = `${y}px`;
      menu.classList.remove('hidden');
    }

    function closeTableContextMenu() {
      const menu = document.getElementById('table-context-menu');
      if (menu) menu.classList.add('hidden');
      selectedContextMenuTable = null;
    }

    function tableContextMenuAction(action) {
      const tbl = selectedContextMenuTable;
      closeTableContextMenu();
      if (!tbl) return;
      if (action === 'open') {
        openTableViewer(tbl);
      } else if (action === 'ddl') {
        viewTableDDL(tbl);
      } else if (action === 'drop') {
        dropTable(tbl);
      }
    }

    function dropCurrentActiveTable() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && activeTab.isTableViewer && activeTab.tableName) {
        dropTable(activeTab.tableName);
      }
    }

    // --- Schema Explorer Section Collapsible State ---
    let schemaSectionCollapsed = {
      tables: false,
      views: false,
      triggers: false,
      procedures: false,
      functions: false,
      routines: false,
      events: false
    };

    try {
      const savedCollapse = localStorage.getItem('sqlfox_schema_collapse');
      if (savedCollapse) schemaSectionCollapsed = Object.assign(schemaSectionCollapsed, JSON.parse(savedCollapse));
    } catch (_) {}

    function toggleSchemaSection(section) {
      if (schemaSectionCollapsed[section] === undefined) {
        schemaSectionCollapsed[section] = false;
      }
      schemaSectionCollapsed[section] = !schemaSectionCollapsed[section];
      try {
        localStorage.setItem('sqlfox_schema_collapse', JSON.stringify(schemaSectionCollapsed));
      } catch (_) {}
      if (schemaCatalogCache) {
        renderSchemaTree(schemaCatalogCache);
      }
    }

    // --- Shortcuts Modal Handlers ---
    function openShortcutsModal() {
      const modal = document.getElementById('shortcuts-modal');
      if (modal) modal.classList.remove('hidden');
    }
    function closeShortcutsModal() {
      const modal = document.getElementById('shortcuts-modal');
      if (modal) modal.classList.add('hidden');
    }
    function toggleShortcutsModal() {
      const modal = document.getElementById('shortcuts-modal');
      if (modal) {
        if (modal.classList.contains('hidden')) openShortcutsModal();
        else closeShortcutsModal();
      }
    }

    function renderSchemaTree(schema) {
      const treeEl = document.getElementById('schema-tree');
      const dbContainer = document.getElementById('database-selector-container');
      const dbActiveName = document.getElementById('database-switcher-active-name');
      const dbSelect = document.getElementById('database-selector');

      if (!schema) {
        if (dbContainer) dbContainer.classList.add('hidden');
        if (treeEl) treeEl.innerHTML = `<div class="text-slate-500 text-center py-6 text-xs">No schema loaded.</div>`;
        const badgeEl = document.getElementById('table-count-badge');
        if (badgeEl) badgeEl.textContent = '0';
        return;
      }

      // Populate database switcher
      if (schema.databases && schema.databases.length > 0) {
        if (dbContainer) dbContainer.classList.remove('hidden');
        if (dbActiveName) dbActiveName.textContent = schema.database_name || schema.databases[0] || 'default';
        if (dbSelect) {
          dbSelect.innerHTML = schema.databases.map(d => `
            <option value="${escapeHtml(d)}" ${d === schema.database_name ? 'selected' : ''}>${escapeHtml(d)}</option>
          `).join('');
        }
        renderDatabaseDropdownList();
      } else {
        if (dbContainer) dbContainer.classList.add('hidden');
      }

      if (!schemaTreeFastIndex || schemaTreeFastIndex._schemaRef !== schema || schemaTreeFastIndex.database !== schema.database_name) {
        buildSchemaTreeFastIndex(schema);
      }

      const allTables = schemaTreeFastIndex ? schemaTreeFastIndex.tables : ((schema.tables || []).filter(t => t.type !== 'VIEW'));
      const allViews = schemaTreeFastIndex ? schemaTreeFastIndex.views : ((schema.tables || []).filter(t => t.type === 'VIEW'));
      const allTriggers = schemaTreeFastIndex ? schemaTreeFastIndex.triggers : (schema.triggers || []);

      const q = schemaFilterQuery;
      const tablesList = q ? allTables.filter(t => t._lower ? t._lower.includes(q) : t.name.toLowerCase().includes(q)) : allTables;
      const viewsList = q ? allViews.filter(v => v._lower ? v._lower.includes(q) : v.name.toLowerCase().includes(q)) : allViews;
      const triggersList = q ? allTriggers.filter(tr => tr._lower ? tr._lower.includes(q) : tr.name.toLowerCase().includes(q)) : allTriggers;

      const allRoutines = schema.routines || [];
      const allProcedures = allRoutines.filter(r => (r.type || 'PROCEDURE').toUpperCase() === 'PROCEDURE');
      const proceduresList = q ? allProcedures.filter(r => r.name.toLowerCase().includes(q)) : allProcedures;

      const allFunctions = allRoutines.filter(r => (r.type || '').toUpperCase() === 'FUNCTION');
      const functionsList = q ? allFunctions.filter(r => r.name.toLowerCase().includes(q)) : allFunctions;

      const allEvents = schema.events || [];
      const eventsList = q ? allEvents.filter(e => e.name.toLowerCase().includes(q)) : allEvents;

      const badgeEl = document.getElementById('table-count-badge');
      if (badgeEl) {
        badgeEl.textContent = q ? `${tablesList.length}/${allTables.length}` : allTables.length;
      }

      if (q && tablesList.length === 0 && viewsList.length === 0 && triggersList.length === 0 && proceduresList.length === 0 && functionsList.length === 0 && eventsList.length === 0) {
        treeEl.innerHTML = `
          <div class="text-slate-500 text-center py-6 text-xs italic">
            No objects matching "${escapeHtml(q)}"<br>
            <button onclick="clearSchemaFilter()" class="text-app-accent hover:underline mt-1">Clear filter</button>
          </div>
        `;
        return;
      }

      let html = '';

      // 1. Tables Section
      const isTablesCollapsed = schemaSectionCollapsed.tables && !q;
      if (!q || tablesList.length > 0) {
        html += `
          <div class="mb-2.5">
            <div class="flex items-center justify-between text-[11px] font-semibold text-slate-400 uppercase tracking-wider px-1 py-1 border-b border-slate-800/80 mb-1 hover:bg-slate-800/40 rounded transition select-none">
              <button onclick="toggleSchemaSection('tables')" class="flex items-center space-x-1.5 flex-1 text-left group cursor-pointer" title="${isTablesCollapsed ? 'Expand Tables' : 'Collapse Tables'}">
                <span class="text-[9px] text-slate-500 group-hover:text-slate-300 font-mono w-3">${isTablesCollapsed ? '▶' : '▼'}</span>
                ${renderIcon('table', 'w-3.5 h-3.5 text-sky-400 shrink-0')}
                <span class="group-hover:text-white transition">Tables</span>
                <span class="text-[10px] text-slate-500 font-mono font-normal">(${tablesList.length}${q ? '/' + allTables.length : ''})</span>
              </button>
              <button onclick="openCreateTableModal()" class="text-app-accent hover:text-white text-xs px-1" title="Create New Table">${renderIcon('plus', 'w-3 h-3 text-sky-400')}</button>
            </div>
            ${isTablesCollapsed ? '' : `
            <div class="space-y-0.5">
              ${tablesList.length === 0 ? `<div class="text-slate-600 text-[11px] px-2 py-1 italic">No tables. Click + to create.</div>` :
                tablesList.map(t => {
                  const rowTip = t.row_count > 0 ? ` (~${t.row_count.toLocaleString()} rows)` : '';
                  return `
                    <div style="content-visibility: auto; contain-intrinsic-size: 0 28px;" class="rounded hover:bg-slate-800/60 p-1 group flex items-center justify-between cursor-pointer" data-name="${escapeHtml(t.name)}" onclick="openTableViewer(this.dataset.name)" oncontextmenu="showTableContextMenu(event, this.dataset.name)" title="${escapeHtml(t.name)}${rowTip}">
                      <span class="flex items-center space-x-1.5 truncate text-slate-200 group-hover:text-white flex-1 min-w-0">
                        ${renderIcon('table', 'w-3.5 h-3.5 text-slate-400 group-hover:text-sky-300 shrink-0')}
                        <span class="truncate">${escapeHtml(t.name)}</span>
                      </span>
                    </div>
                  `;
                }).join('')
              }
            </div>
            `}
          </div>
        `;
      }

      // 2. Views Section
      const isViewsCollapsed = schemaSectionCollapsed.views && !q;
      if (!q || viewsList.length > 0) {
        html += `
          <div class="mb-2.5">
            <div class="flex items-center justify-between text-[11px] font-semibold text-slate-400 uppercase tracking-wider px-1 py-1 border-b border-slate-800/80 mb-1 hover:bg-slate-800/40 rounded transition select-none">
              <button onclick="toggleSchemaSection('views')" class="flex items-center space-x-1.5 flex-1 text-left group cursor-pointer" title="${isViewsCollapsed ? 'Expand Views' : 'Collapse Views'}">
                <span class="text-[9px] text-slate-500 group-hover:text-slate-300 font-mono w-3">${isViewsCollapsed ? '▶' : '▼'}</span>
                ${renderIcon('eye', 'w-3.5 h-3.5 text-purple-400 shrink-0')}
                <span class="group-hover:text-white transition">Views</span>
                <span class="text-[10px] text-slate-500 font-mono font-normal">(${viewsList.length}${q ? '/' + allViews.length : ''})</span>
              </button>
              <button onclick="openCreateViewModal()" class="text-app-accent hover:text-white text-xs px-1" title="Create New View">${renderIcon('plus', 'w-3 h-3 text-purple-400')}</button>
            </div>
            ${isViewsCollapsed ? '' : `
            <div class="space-y-0.5">
              ${viewsList.length === 0 ? `<div class="text-slate-600 text-[11px] px-2 py-1 italic">No views. Click + to create.</div>` :
                viewsList.map(v => `
                  <div style="content-visibility: auto; contain-intrinsic-size: 0 28px;" class="rounded hover:bg-slate-800/60 p-1 group flex items-center justify-between cursor-pointer" data-name="${escapeHtml(v.name)}" onclick="openTableViewer(this.dataset.name)">
                    <span class="flex items-center space-x-1.5 truncate text-purple-300 group-hover:text-white flex-1 min-w-0 mr-1" title="${escapeHtml(v.name)}">
                      ${renderIcon('eye', 'w-3.5 h-3.5 text-purple-400 shrink-0')}
                      <span class="truncate">${escapeHtml(v.name)}</span>
                    </span>
                    <div class="flex items-center space-x-1">
                      <button type="button" data-name="${escapeHtml(v.name)}" onclick="event.stopPropagation(); openCreateViewModal(this.dataset.name)" class="opacity-0 group-hover:opacity-100 hover:text-app-accent px-1" title="Edit View">${renderIcon('edit', 'w-3 h-3 text-purple-400 hover:text-white')}</button>
                      <button type="button" data-name="${escapeHtml(v.name)}" onclick="event.stopPropagation(); dropView(this.dataset.name)" class="opacity-0 group-hover:opacity-100 hover:text-red-400 px-0.5" title="Drop View">${renderIcon('trash-2', 'w-3 h-3 text-rose-400')}</button>
                    </div>
                  </div>
                `).join('')
              }
            </div>
            `}
          </div>
        `;
      }

      // 3. Triggers Section
      const isTriggersCollapsed = schemaSectionCollapsed.triggers && !q;
      if (!q || triggersList.length > 0) {
        html += `
          <div class="mb-2.5">
            <div class="flex items-center justify-between text-[11px] font-semibold text-slate-400 uppercase tracking-wider px-1 py-1 border-b border-slate-800/80 mb-1 hover:bg-slate-800/40 rounded transition select-none">
              <button onclick="toggleSchemaSection('triggers')" class="flex items-center space-x-1.5 flex-1 text-left group cursor-pointer" title="${isTriggersCollapsed ? 'Expand Triggers' : 'Collapse Triggers'}">
                <span class="text-[9px] text-slate-500 group-hover:text-slate-300 font-mono w-3">${isTriggersCollapsed ? '▶' : '▼'}</span>
                ${renderIcon('zap', 'w-3.5 h-3.5 text-amber-400 shrink-0')}
                <span class="group-hover:text-white transition">Triggers</span>
                <span class="text-[10px] text-slate-500 font-mono font-normal">(${triggersList.length}${q ? '/' + allTriggers.length : ''})</span>
              </button>
              <button onclick="openCreateTriggerModal()" class="text-app-accent hover:text-white text-xs px-1 flex items-center" title="Create New Trigger">${renderIcon('plus', 'w-3.5 h-3.5 text-amber-400')}</button>
            </div>
            ${isTriggersCollapsed ? '' : `
            <div class="space-y-0.5">
              ${triggersList.length === 0 ? `<div class="text-slate-600 text-[11px] px-2 py-1 italic">No triggers. Click + to create.</div>` :
                triggersList.map(trg => `
                  <div class="rounded hover:bg-slate-800/60 p-1 group flex items-center justify-between cursor-pointer" data-name="${escapeHtml(trg.name)}" onclick="openCreateTriggerModal(this.dataset.name)">
                    <span class="flex items-center space-x-1.5 truncate text-amber-300 group-hover:text-white">
                      ${renderIcon('zap', 'w-3.5 h-3.5 text-amber-400 shrink-0')}
                      <span class="truncate max-w-[120px]">${escapeHtml(trg.name)}</span>
                    </span>
                    <div class="flex items-center space-x-1">
                      <span class="text-[10px] text-slate-500 font-mono">${escapeHtml(trg.table || '')}</span>
                      <button type="button" data-name="${escapeHtml(trg.name)}" onclick="event.stopPropagation(); dropTrigger(this.dataset.name)" class="opacity-0 group-hover:opacity-100 hover:text-red-400 px-0.5" title="Drop Trigger">${renderIcon('trash-2', 'w-3 h-3 text-rose-400')}</button>
                    </div>
                  </div>
                `).join('')
              }
            </div>
            `}
          </div>
        `;
      }

      // 4. Stored Procedures Section
      const isProceduresCollapsed = schemaSectionCollapsed.procedures && !q;
      if (!q || proceduresList.length > 0) {
        html += `
          <div class="mb-2.5">
            <div class="flex items-center justify-between text-[11px] font-semibold text-slate-400 uppercase tracking-wider px-1 py-1 border-b border-slate-800/80 mb-1 hover:bg-slate-800/40 rounded transition select-none">
              <button onclick="toggleSchemaSection('procedures')" class="flex items-center space-x-1.5 flex-1 text-left group cursor-pointer" title="${isProceduresCollapsed ? 'Expand Procedures' : 'Collapse Procedures'}">
                <span class="text-[9px] text-slate-500 group-hover:text-slate-300 font-mono w-3">${isProceduresCollapsed ? '▶' : '▼'}</span>
                ${renderIcon('sliders', 'w-3.5 h-3.5 text-indigo-400 shrink-0')}
                <span class="group-hover:text-white transition">Procedures</span>
                <span class="text-[10px] text-slate-500 font-mono font-normal">(${proceduresList.length}${q ? '/' + allProcedures.length : ''})</span>
              </button>
              <button onclick="openCreateRoutineModal('', 'PROCEDURE')" class="text-app-accent hover:text-white text-xs px-1 flex items-center" title="Create New Stored Procedure">${renderIcon('plus', 'w-3.5 h-3.5 text-indigo-400')}</button>
            </div>
            ${isProceduresCollapsed ? '' : `
            <div class="space-y-0.5">
              ${proceduresList.length === 0 ? `<div class="text-slate-600 text-[11px] px-2 py-1 italic">No stored procedures.</div>` :
                proceduresList.map(r => `
                  <div class="rounded hover:bg-slate-800/60 p-1 group flex items-center justify-between cursor-pointer" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'PROCEDURE')}" onclick="openRoutineCaller(this.dataset.name, this.dataset.type)">
                    <span class="flex items-center space-x-1.5 truncate text-sky-300 group-hover:text-white flex-1 min-w-0 mr-1" title="${escapeHtml(r.name)} (${r.type || 'PROCEDURE'})">
                      <span class="text-xs flex-shrink-0 flex items-center">${renderIcon('sliders', 'w-3 h-3 text-indigo-400')}</span>
                      <span class="truncate">${escapeHtml(r.name)}</span>
                    </span>
                    <div class="flex items-center space-x-1">
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'PROCEDURE')}" onclick="event.stopPropagation(); viewRoutineDDL(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-sky-300 px-1 text-[11px] flex items-center" title="View DDL">${renderIcon('file-code', 'w-3 h-3 text-sky-400')}</button>
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'PROCEDURE')}" onclick="event.stopPropagation(); openCreateRoutineModal(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-indigo-300 px-1 text-[11px] flex items-center" title="Edit Routine">${renderIcon('edit', 'w-3 h-3 text-indigo-400')}</button>
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'PROCEDURE')}" onclick="event.stopPropagation(); dropRoutine(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-rose-400 px-0.5 text-[11px] flex items-center" title="Drop Routine">${renderIcon('trash-2', 'w-3 h-3 text-rose-400')}</button>
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'PROCEDURE')}" onclick="event.stopPropagation(); openRoutineCaller(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-emerald-400 px-1 text-[11px] flex items-center" title="Call in Query Editor">${renderIcon('play', 'w-3 h-3 text-emerald-400')}</button>
                    </div>
                  </div>
                `).join('')
              }
            </div>
            `}
          </div>
        `;
      }

      // 5. Stored Functions Section
      const isFunctionsCollapsed = schemaSectionCollapsed.functions && !q;
      if (!q || functionsList.length > 0) {
        html += `
          <div class="mb-2.5">
            <div class="flex items-center justify-between text-[11px] font-semibold text-slate-400 uppercase tracking-wider px-1 py-1 border-b border-slate-800/80 mb-1 hover:bg-slate-800/40 rounded transition select-none">
              <button onclick="toggleSchemaSection('functions')" class="flex items-center space-x-1.5 flex-1 text-left group cursor-pointer" title="${isFunctionsCollapsed ? 'Expand Functions' : 'Collapse Functions'}">
                <span class="text-[9px] text-slate-500 group-hover:text-slate-300 font-mono w-3">${isFunctionsCollapsed ? '▶' : '▼'}</span>
                <span class="font-serif italic font-bold text-sky-400 text-xs shrink-0 w-3.5 text-center">ƒ</span>
                <span class="group-hover:text-white transition">Functions</span>
                <span class="text-[10px] text-slate-500 font-mono font-normal">(${functionsList.length}${q ? '/' + allFunctions.length : ''})</span>
              </button>
              <button onclick="openCreateRoutineModal('', 'FUNCTION')" class="text-app-accent hover:text-white text-xs px-1 flex items-center" title="Create New Stored Function">${renderIcon('plus', 'w-3.5 h-3.5 text-sky-400')}</button>
            </div>
            ${isFunctionsCollapsed ? '' : `
            <div class="space-y-0.5">
              ${functionsList.length === 0 ? `<div class="text-slate-600 text-[11px] px-2 py-1 italic">No stored functions.</div>` :
                functionsList.map(r => `
                  <div class="rounded hover:bg-slate-800/60 p-1 group flex items-center justify-between cursor-pointer" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'FUNCTION')}" onclick="openRoutineCaller(this.dataset.name, this.dataset.type)">
                    <span class="flex items-center space-x-1.5 truncate text-sky-300 group-hover:text-white flex-1 min-w-0 mr-1" title="${escapeHtml(r.name)} (${r.type || 'FUNCTION'})">
                      <span class="font-serif italic font-bold text-sky-400 text-xs shrink-0 w-3 text-center">ƒ</span>
                      <span class="truncate">${escapeHtml(r.name)}</span>
                    </span>
                    <div class="flex items-center space-x-1">
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'FUNCTION')}" onclick="event.stopPropagation(); viewRoutineDDL(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-sky-300 px-1 text-[11px] flex items-center" title="View DDL">${renderIcon('file-code', 'w-3 h-3 text-sky-400')}</button>
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'FUNCTION')}" onclick="event.stopPropagation(); openCreateRoutineModal(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-indigo-300 px-1 text-[11px] flex items-center" title="Edit Routine">${renderIcon('edit', 'w-3 h-3 text-indigo-400')}</button>
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'FUNCTION')}" onclick="event.stopPropagation(); dropRoutine(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-rose-400 px-0.5 text-[11px] flex items-center" title="Drop Routine">${renderIcon('trash-2', 'w-3 h-3 text-rose-400')}</button>
                      <button type="button" data-name="${escapeHtml(r.name)}" data-type="${escapeHtml(r.type || 'FUNCTION')}" onclick="event.stopPropagation(); openRoutineCaller(this.dataset.name, this.dataset.type)" class="opacity-0 group-hover:opacity-100 hover:text-emerald-400 px-1 text-[11px] flex items-center" title="Call in Query Editor">${renderIcon('play', 'w-3 h-3 text-emerald-400')}</button>
                    </div>
                  </div>
                `).join('')
              }
            </div>
            `}
          </div>
        `;
      }

      // 6. Scheduled Events Section
      const isEventsCollapsed = schemaSectionCollapsed.events && !q;
      if (!q || eventsList.length > 0) {
        html += `
          <div class="mb-2.5">
            <div class="flex items-center justify-between text-[11px] font-semibold text-slate-400 uppercase tracking-wider px-1 py-1 border-b border-slate-800/80 mb-1 hover:bg-slate-800/40 rounded transition select-none">
              <button onclick="toggleSchemaSection('events')" class="flex items-center space-x-1.5 flex-1 text-left group cursor-pointer" title="${isEventsCollapsed ? 'Expand Events' : 'Collapse Events'}">
                <span class="text-[9px] text-slate-500 group-hover:text-slate-300 font-mono w-3">${isEventsCollapsed ? '▶' : '▼'}</span>
                ${renderIcon('clock', 'w-3.5 h-3.5 text-cyan-400 shrink-0')}
                <span class="group-hover:text-white transition">Events</span>
                <span class="text-[10px] text-slate-500 font-mono font-normal">(${eventsList.length}${q ? '/' + allEvents.length : ''})</span>
              </button>
              <button onclick="openCreateEventModal()" class="text-app-accent hover:text-white text-xs px-1 flex items-center" title="Create New Scheduled Event">${renderIcon('plus', 'w-3.5 h-3.5 text-cyan-400')}</button>
            </div>
            ${isEventsCollapsed ? '' : `
            <div class="space-y-0.5">
              ${eventsList.length === 0 ? `<div class="text-slate-600 text-[11px] px-2 py-1 italic">No scheduled events.</div>` :
                eventsList.map(ev => {
                  const isEnabled = (ev.status || '').toUpperCase() === 'ENABLED';
                  return `
                    <div class="rounded hover:bg-slate-800/60 p-1 group flex items-center justify-between cursor-pointer" data-name="${escapeHtml(ev.name)}" onclick="viewEventDDL(this.dataset.name)">
                      <span class="flex items-center space-x-1.5 truncate ${isEnabled ? 'text-emerald-300' : 'text-slate-400'} group-hover:text-white flex-1 min-w-0 mr-1" title="${escapeHtml(ev.name)} (${ev.status})">
                        <span class="text-xs flex-shrink-0">${isEnabled ? '<span class="inline-block w-2 h-2 rounded-full bg-emerald-400"></span>' : '<span class="inline-block w-2 h-2 rounded-full bg-slate-500"></span>'}</span>
                        <span class="truncate">${escapeHtml(ev.name)}</span>
                      </span>
                      <div class="flex items-center space-x-1">
                        <button type="button" data-name="${escapeHtml(ev.name)}" onclick="event.stopPropagation(); viewEventDDL(this.dataset.name)" class="opacity-0 group-hover:opacity-100 hover:text-sky-300 px-1 text-[11px] flex items-center" title="View DDL">${renderIcon('file-code', 'w-3 h-3 text-sky-400')}</button>
                        <button type="button" data-name="${escapeHtml(ev.name)}" onclick="event.stopPropagation(); openCreateEventModal(this.dataset.name)" class="opacity-0 group-hover:opacity-100 hover:text-cyan-300 px-1 text-[11px] flex items-center" title="Edit Event">${renderIcon('edit', 'w-3 h-3 text-cyan-400')}</button>
                        <button type="button" data-name="${escapeHtml(ev.name)}" onclick="event.stopPropagation(); dropEvent(this.dataset.name)" class="opacity-0 group-hover:opacity-100 hover:text-rose-400 px-0.5 text-[11px] flex items-center" title="Drop Event">${renderIcon('trash-2', 'w-3 h-3 text-rose-400')}</button>
                        <button type="button" data-name="${escapeHtml(ev.name)}" data-next-enabled="${!isEnabled}" onclick="event.stopPropagation(); toggleEventStatusAction(this.dataset.name, this.dataset.nextEnabled === 'true')" class="opacity-0 group-hover:opacity-100 hover:text-amber-300 px-1 text-[11px] flex items-center" title="${isEnabled ? 'Disable Event' : 'Enable Event'}">${isEnabled ? renderIcon('square', 'w-2.5 h-2.5 text-amber-400') : renderIcon('play', 'w-2.5 h-2.5 text-emerald-400')}</button>
                      </div>
                    </div>
                  `;
                }).join('')
              }
            </div>
            `}
          </div>
        `;
      }

      treeEl.innerHTML = html;
    }

    async function viewRoutineDDL(name, type) {
      if (!activeConnection) return;
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/ddl/routine/${encodeURIComponent(name)}?type=${encodeURIComponent(type || 'PROCEDURE')}`);
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert('Failed to load routine DDL: ' + (data.error || res.statusText), 'DDL Error');
          return;
        }
        openViewDDLModal(name, data.ddl);
      } catch (err) {
        showNativeAlert('Error loading routine DDL: ' + err.message, 'Network Error');
      }
    }

    function openRoutineCaller(name, type) {
      const isFunction = (type || '').toUpperCase() === 'FUNCTION';
      let routine = null;
      if (schemaCatalogCache && schemaCatalogCache.routines) {
        routine = schemaCatalogCache.routines.find(r => r.name === name);
      }

      let callSQL = '';
      if (isFunction) {
        // e.g. SELECT fn_calculate_tax(100) AS result;
        const paramPlaceholders = (routine?.parameters || []).map(p => `/* ${p.mode || 'IN'} ${p.name} (${p.type || ''}) */ NULL`).join(', ');
        callSQL = `-- Invoking Stored Function: ${name}\nSELECT \`${name}\`(${paramPlaceholders}) AS \`${name}_result\`;\n`;
      } else {
        // e.g. CALL sp_name(arg1, @out_arg2); SELECT @out_arg2;
        const params = routine?.parameters || [];
        const outVars = [];
        const argSnippets = [];

        params.forEach((p, idx) => {
          const mode = (p.mode || 'IN').toUpperCase();
          if (mode === 'OUT' || mode === 'INOUT') {
            const varName = `@${p.name || 'out_' + idx}`;
            outVars.push(varName);
            argSnippets.push(`/* ${mode} ${p.name} */ ${varName}`);
          } else {
            argSnippets.push(`/* IN ${p.name} (${p.type || ''}) */ NULL`);
          }
        });

        callSQL = `-- Invoking Stored Procedure: ${name}\nCALL \`${name}\`(\n  ${argSnippets.join(',\n  ')}\n);\n`;
        if (outVars.length > 0) {
          callSQL += `\n-- Retrieve OUT parameters:\nSELECT ${outVars.join(', ')};\n`;
        }
      }

      addNewQueryTab();
      setEditorSQL(callSQL);
      showNativeAlert(`Call template generated for ${type} "${name}" in a new Query tab.`, 'Smart Routine Caller');
    }

    async function viewEventDDL(name) {
      if (!activeConnection) return;
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/ddl/event/${encodeURIComponent(name)}`);
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert('Failed to load event DDL: ' + (data.error || res.statusText), 'DDL Error');
          return;
        }
        openViewDDLModal(name, data.ddl);
      } catch (err) {
        showNativeAlert('Error loading event DDL: ' + err.message, 'Network Error');
      }
    }

    async function toggleEventStatusAction(name, enable) {
      if (!activeConnection) return;
      const action = enable ? 'ENABLE' : 'DISABLE';
      const ok = await showNativeConfirm(`Are you sure you want to ${action} scheduled event "${name}"?`, `Event Action: ${action}`);
      if (!ok) return;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/event/${encodeURIComponent(name)}/toggle`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enable })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(`Failed to ${action} event: ` + (data.error || res.statusText), 'Event Error');
          return;
        }
        showNativeAlert(`Scheduled event "${name}" is now ${action}D.`, 'Event Updated');
        if (activeConnection) {
          await loadSchemaTree(activeConnection.id);
        }
      } catch (err) {
        showNativeAlert(`Network error: ` + err.message, 'Event Error');
      }
    }


    // --- MySQL / MariaDB Character Sets & Collations Catalog ---
    const MYSQL_CHARSET_COLLATIONS = {
      'DEFAULT': { label: '(Server Default)', default: '', collations: [] },
      'utf8mb4': {
        label: 'utf8mb4 (Unicode 4-byte, recommended)',
        default: 'utf8mb4_unicode_ci',
        collations: [
          'utf8mb4_unicode_ci',
          'utf8mb4_general_ci',
          'utf8mb4_0900_ai_ci',
          'utf8mb4_bin',
          'utf8mb4_unicode_520_ci',
          'utf8mb4_spanish_ci',
          'utf8mb4_indonesian_ci',
          'utf8mb4_vietnamese_ci'
        ]
      },
      'latin1': {
        label: 'latin1 (cp1252 West European / Swedish / Legacy)',
        default: 'latin1_swedish_ci',
        collations: [
          'latin1_swedish_ci',
          'latin1_general_ci',
          'latin1_general_cs',
          'latin1_bin',
          'latin1_german1_ci',
          'latin1_german2_ci',
          'latin1_danish_ci',
          'latin1_spanish_ci'
        ]
      },
      'utf8': {
        label: 'utf8 (utf8mb3 3-byte Unicode)',
        default: 'utf8_general_ci',
        collations: [
          'utf8_general_ci',
          'utf8_unicode_ci',
          'utf8_bin',
          'utf8_turkish_ci',
          'utf8_persian_ci',
          'utf8_spanish_ci'
        ]
      },
      'ascii': {
        label: 'ascii (US ASCII 7-bit)',
        default: 'ascii_general_ci',
        collations: ['ascii_general_ci', 'ascii_bin']
      },
      'binary': {
        label: 'binary (Binary byte array)',
        default: 'binary',
        collations: ['binary']
      },
      'cp1250': {
        label: 'cp1250 (Windows Central European)',
        default: 'cp1250_general_ci',
        collations: ['cp1250_general_ci', 'cp1250_czech_cs', 'cp1250_polish_ci', 'cp1250_bin']
      },
      'cp1251': {
        label: 'cp1251 (Windows Cyrillic)',
        default: 'cp1251_general_ci',
        collations: ['cp1251_general_ci', 'cp1251_ukrainian_ci', 'cp1251_bin']
      },
      'cp1256': {
        label: 'cp1256 (Windows Arabic)',
        default: 'cp1256_general_ci',
        collations: ['cp1256_general_ci', 'cp1256_bin']
      },
      'cp1257': {
        label: 'cp1257 (Windows Baltic)',
        default: 'cp1257_general_ci',
        collations: ['cp1257_general_ci', 'cp1257_lithuanian_ci', 'cp1257_bin']
      },
      'big5': {
        label: 'big5 (Traditional Chinese)',
        default: 'big5_chinese_ci',
        collations: ['big5_chinese_ci', 'big5_bin']
      },
      'gbk': {
        label: 'gbk (Simplified Chinese GBK)',
        default: 'gbk_chinese_ci',
        collations: ['gbk_chinese_ci', 'gbk_bin']
      },
      'gb2312': {
        label: 'gb2312 (Simplified Chinese GB2312)',
        default: 'gb2312_chinese_ci',
        collations: ['gb2312_chinese_ci', 'gb2312_bin']
      },
      'sjis': {
        label: 'sjis (Shift-JIS Japanese)',
        default: 'sjis_japanese_ci',
        collations: ['sjis_japanese_ci', 'sjis_bin']
      },
      'ujis': {
        label: 'ujis (EUC-JP Japanese)',
        default: 'ujis_japanese_ci',
        collations: ['ujis_japanese_ci', 'ujis_bin']
      },
      'euckr': {
        label: 'euckr (EUC-KR Korean)',
        default: 'euckr_korean_ci',
        collations: ['euckr_korean_ci', 'euckr_bin']
      },
      'greek': {
        label: 'greek (ISO 8859-7 Greek)',
        default: 'greek_general_ci',
        collations: ['greek_general_ci', 'greek_bin']
      },
      'hebrew': {
        label: 'hebrew (ISO 8859-8 Hebrew)',
        default: 'hebrew_general_ci',
        collations: ['hebrew_general_ci', 'hebrew_bin']
      },
      'tis620': {
        label: 'tis620 (TIS620 Thai)',
        default: 'tis620_thai_ci',
        collations: ['tis620_thai_ci', 'tis620_bin']
      },
      'utf16': {
        label: 'utf16 (UTF-16 Unicode)',
        default: 'utf16_unicode_ci',
        collations: ['utf16_unicode_ci', 'utf16_general_ci', 'utf16_bin']
      },
      'utf32': {
        label: 'utf32 (UTF-32 Unicode)',
        default: 'utf32_unicode_ci',
        collations: ['utf32_unicode_ci', 'utf32_general_ci', 'utf32_bin']
      }
    };

    function initDBCharsetOptions() {
      const charsetSelect = document.getElementById('new-db-charset');
      if (!charsetSelect) return;

      const currentVal = charsetSelect.value || 'utf8mb4';
      charsetSelect.innerHTML = '';

      for (const [key, item] of Object.entries(MYSQL_CHARSET_COLLATIONS)) {
        const opt = document.createElement('option');
        opt.value = key;
        opt.textContent = item.label;
        if (key === currentVal) opt.selected = true;
        charsetSelect.appendChild(opt);
      }

      onDBCharsetChange();
    }

    function onDBCharsetChange() {
      const charsetSelect = document.getElementById('new-db-charset');
      const collationSelect = document.getElementById('new-db-collation');
      if (!charsetSelect || !collationSelect) return;

      const chosenCharset = charsetSelect.value;
      const data = MYSQL_CHARSET_COLLATIONS[chosenCharset];

      collationSelect.innerHTML = '';

      if (!data || chosenCharset === 'DEFAULT' || !data.collations || data.collations.length === 0) {
        collationSelect.disabled = true;
        collationSelect.innerHTML = '<option value="">(Server Default)</option>';
        return;
      }

      collationSelect.disabled = false;
      data.collations.forEach(col => {
        const opt = document.createElement('option');
        opt.value = col;
        opt.textContent = col + (col === data.default ? ' (Default)' : '');
        if (col === data.default) opt.selected = true;
        collationSelect.appendChild(opt);
      });
    }

    function openCreateDatabaseModal() {
      if (!activeConnection) {
        showNativeAlert('Please select or create a connection first.', 'No Connection');
        return;
      }
      document.getElementById('new-db-name').value = '';
      initDBCharsetOptions();
      const dlg = document.getElementById('database-creator-dialog');
      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
      setTimeout(() => document.getElementById('new-db-name').focus(), 100);
    }

    async function submitCreateDatabase() {
      const dbName = document.getElementById('new-db-name').value.trim();
      const charset = document.getElementById('new-db-charset').value;
      const collation = document.getElementById('new-db-collation').value;

      if (!dbName) {
        showNativeAlert('Database name is required.', 'Create Database');
        return;
      }

      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      if (isSQLite) {
        showNativeAlert('SQLite operates on single-file databases. Use "+ New" in the session switcher to configure another SQLite file.', 'SQLite Notice');
        return;
      }

      let sql = `CREATE DATABASE \`${dbName.replace(/`/g, '``')}\``;
      if (charset && charset !== 'DEFAULT') {
        sql += ` CHARACTER SET ${charset}`;
        if (collation) {
          sql += ` COLLATE ${collation}`;
        }
      }
      sql += ';';

      document.getElementById('database-creator-dialog').close();

      openDDLReviewModal(sql, async () => {
        // ponytail: skip redundant schema refresh on old db and close DDL modal promptly before switching
        const ok = await executeDDLStatements(sql, `Database "${dbName}" created successfully.`, true);
        const ddlDlg = document.getElementById('ddl-review-dialog');
        if (ddlDlg) ddlDlg.close();
        if (ok) {
          await selectDatabase(dbName);
        }
      });
    }

    function updateSchemaTimestampBadge(dbName) {
      const badge = document.getElementById('schema-cache-badge');
      if (!badge) return;
      const connId = activeConnection ? activeConnection.id : '';
      const ts = (connId && sessionCatalogTimestamps[connId]) ? sessionCatalogTimestamps[connId][dbName.toLowerCase()] : null;
      if (ts) {
        const dt = new Date(ts);
        const timeStr = `${dt.getHours().toString().padStart(2,'0')}:${dt.getMinutes().toString().padStart(2,'0')}`;
        badge.textContent = `⚡${timeStr}`;
        badge.title = `Cached at ${dt.toLocaleTimeString()}`;
        badge.classList.remove('hidden');
      } else {
        badge.classList.add('hidden');
      }
    }

    let isRefreshingCatalog = false;
    async function refreshDatabaseCatalog(dbName, triggerBtn = null) {
      if (!activeConnection || !dbName || isRefreshingCatalog) return;
      isRefreshingCatalog = true;
      if (triggerBtn) {
        triggerBtn.classList.add('animate-spin', 'pointer-events-none', 'text-sky-300');
      }
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/schema?db=${encodeURIComponent(dbName)}`);
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || res.statusText || 'Failed to refresh catalog');
        }
        const sc = await res.json();
        const lower = dbName.toLowerCase();
        multiDBCatalogCache[lower] = sc;
        if (!sessionCatalogTimestamps[activeConnection.id]) sessionCatalogTimestamps[activeConnection.id] = {};
        sessionCatalogTimestamps[activeConnection.id][lower] = Date.now();
        if (schemaCatalogCache && (schemaCatalogCache.database_name || '').toLowerCase() === lower) {
          schemaCatalogCache = sc;
          renderSchemaTree(sc);
          updateSchemaTimestampBadge(dbName);
        }
        renderDatabaseDropdownList();
        showToast(`Catalog refreshed for '${dbName}'`, 'success');
      } catch (err) {
        console.warn('refreshDatabaseCatalog error:', err);
        showToast(`Failed to refresh catalog for '${dbName}': ${err.message}`, 'error');
      } finally {
        isRefreshingCatalog = false;
        if (triggerBtn) {
          triggerBtn.classList.remove('animate-spin', 'pointer-events-none', 'text-sky-300');
        }
      }
    }

    let isRefreshingSchema = false;
    async function refreshSchema() {
      if (!activeConnection || isRefreshingSchema) return;
      isRefreshingSchema = true;
      const btn = document.getElementById('btn-refresh-schema');
      if (btn) btn.classList.add('animate-spin', 'text-sky-400', 'pointer-events-none');
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/refresh-schema`, { method: 'POST' });
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || res.statusText || 'Failed to refresh schema');
        }
        const currentDB = schemaCatalogCache ? schemaCatalogCache.database_name : '';
        await loadSchemaTree(activeConnection.id, currentDB);
        showToast('Schema refreshed successfully', 'success');
      } catch (err) {
        showToast(`Failed to refresh schema: ${err.message}`, 'error');
      } finally {
        isRefreshingSchema = false;
        if (btn) btn.classList.remove('animate-spin', 'text-sky-400', 'pointer-events-none');
      }
    }

    // --- Refresh All Schema Catalogs with Active Threads Safety Guard ---
    let refreshAllActiveThreads = 0;
    let isExecutingRefreshAll = false;

    async function openRefreshAllModal() {
      if (!activeConnection) return;
      const dialog = document.getElementById('refresh-all-dialog');
      if (!dialog) return;

      const threadCountEl = document.getElementById('refresh-all-thread-count');
      const healthContainer = document.getElementById('refresh-all-health-container');
      const msgEl = document.getElementById('refresh-all-health-message');
      const ackContainer = document.getElementById('refresh-all-ack-container');
      const ackCheckbox = document.getElementById('refresh-all-ack-checkbox');
      const proceedBtn = document.getElementById('refresh-all-btn-proceed');
      const progressContainer = document.getElementById('refresh-all-progress-container');

      if (ackCheckbox) ackCheckbox.checked = false;
      if (progressContainer) progressContainer.classList.add('hidden');
      if (proceedBtn) {
        proceedBtn.disabled = true;
        proceedBtn.textContent = 'Mulai Refresh All';
        proceedBtn.classList.remove('bg-rose-600', 'hover:bg-rose-500');
        proceedBtn.classList.add('bg-app-accent', 'hover:bg-sky-400');
      }

      if (threadCountEl) threadCountEl.textContent = 'Checking...';
      if (msgEl) msgEl.textContent = 'Connecting to database server...';
      if (healthContainer) {
        healthContainer.className = 'rounded-lg p-3 text-xs border space-y-2 bg-slate-950/50 border-slate-800 text-slate-300';
      }

      if (typeof dialog.showModal === 'function') {
        dialog.showModal();
      }

      // Check active threads out-of-band
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/active-threads`);
        if (!res.ok) throw new Error('Failed to inspect server workload status.');
        const data = await res.json();
        const threads = typeof data.active_threads === 'number' ? data.active_threads : 1;
        refreshAllActiveThreads = threads;

        if (threadCountEl) {
          threadCountEl.textContent = `${threads} active query threads`;
        }

        if (threads < 10) {
          // Level 1: < 10 threads -> Safe
          if (healthContainer) {
            healthContainer.className = 'rounded-lg p-3 text-xs border space-y-2 bg-emerald-950/30 border-emerald-800/60 text-emerald-300';
          }
          if (msgEl) {
            msgEl.innerHTML = '✅ <strong>Safe Condition:</strong> Server workload is low (&lt; 10 active threads). It is safe to synchronize schema catalogs across all databases.';
          }
          if (ackContainer) ackContainer.classList.add('hidden');
          if (proceedBtn) proceedBtn.disabled = false;
        } else if (threads <= 20) {
          // Level 2: 10 - 20 threads -> Severe Warning with mandatory ack
          if (healthContainer) {
            healthContainer.className = 'rounded-lg p-3 text-xs border space-y-2 bg-amber-950/40 border-amber-800/80 text-amber-200';
          }
          if (msgEl) {
            msgEl.innerHTML = '⚠️ <strong>Elevated Workload Warning:</strong> Server is actively processing concurrent queries (10–20 threads). Full schema catalog refresh may incur temporary dictionary I/O overhead.';
          }
          if (ackContainer) ackContainer.classList.remove('hidden');
          if (proceedBtn) proceedBtn.disabled = true; // Requires checkbox
        } else {
          // Level 3: > 20 threads -> HARD AUTO-BLOCK
          if (healthContainer) {
            healthContainer.className = 'rounded-lg p-3 text-xs border space-y-2 bg-rose-950/50 border-rose-800 text-rose-200';
          }
          if (msgEl) {
            msgEl.innerHTML = '🚫 <strong>ACTION BLOCKED:</strong> Critical server workload detected (&gt; 20 active query threads). To safeguard production database performance and prevent thread queueing, Refresh All is restricted. Please retry when query traffic subsides.';
          }
          if (ackContainer) ackContainer.classList.add('hidden');
          if (proceedBtn) {
            proceedBtn.disabled = true;
            proceedBtn.textContent = 'Blocked (Server Busy)';
          }
        }
      } catch (err) {
        if (threadCountEl) threadCountEl.textContent = 'N/A';
        if (msgEl) msgEl.textContent = `Notice: Could not inspect active query threads (${err.message}).`;
        if (proceedBtn) proceedBtn.disabled = false;
      }
    }

    function toggleRefreshAllProceedState() {
      const ackCheckbox = document.getElementById('refresh-all-ack-checkbox');
      const proceedBtn = document.getElementById('refresh-all-btn-proceed');
      if (proceedBtn && refreshAllActiveThreads >= 10 && refreshAllActiveThreads <= 20) {
        proceedBtn.disabled = !ackCheckbox.checked;
      }
    }

    function closeRefreshAllModal() {
      const dialog = document.getElementById('refresh-all-dialog');
      if (dialog && typeof dialog.close === 'function') {
        dialog.close();
      }
    }

    async function executeRefreshAll() {
      if (!activeConnection || isExecutingRefreshAll) return;
      isExecutingRefreshAll = true;

      const proceedBtn = document.getElementById('refresh-all-btn-proceed');
      const cancelBtn = document.getElementById('refresh-all-btn-cancel');
      const progressContainer = document.getElementById('refresh-all-progress-container');
      const progressLabel = document.getElementById('refresh-all-progress-label');
      const progressCounter = document.getElementById('refresh-all-progress-counter');
      const progressBar = document.getElementById('refresh-all-progress-bar');

      if (proceedBtn) {
        proceedBtn.disabled = true;
        proceedBtn.textContent = 'Processing...';
      }
      if (cancelBtn) cancelBtn.disabled = true;
      if (progressContainer) progressContainer.classList.remove('hidden');

      try {
        if (progressLabel) progressLabel.textContent = 'Fetching fast catalogs...';
        if (progressBar) progressBar.style.width = '30%';

        // 1. Fetch Fast-Catalog for all databases in single lightweight pass
        const res = await apiFetch(`/api/session/${activeConnection.id}/fast-catalog`);
        if (!res.ok) throw new Error('Failed to retrieve cross-database catalogs.');
        const fastCat = await res.json();

        const connId = activeConnection.id;
        const now = Date.now();
        if (!sessionCatalogTimestamps[connId]) sessionCatalogTimestamps[connId] = {};

        const dbKeys = Object.keys(fastCat || {});
        let count = 0;
        const total = dbKeys.length;

        for (const dbKey of dbKeys) {
          count++;
          const lk = dbKey.toLowerCase();
          multiDBCatalogCache[lk] = {
            database_name: dbKey,
            databases: schemaCatalogCache ? schemaCatalogCache.databases : [],
            tables: fastCat[dbKey] || [],
            triggers: [],
            routines: [],
            events: []
          };
          sessionCatalogTimestamps[connId][lk] = now;

          if (progressBar) progressBar.style.width = `${Math.round((count / total) * 100)}%`;
          if (progressCounter) progressCounter.textContent = `${count}/${total}`;
          if (progressLabel) progressLabel.textContent = `Caching ${dbKey}...`;

          // Small throttling pace to yield to browser UI loop
          await new Promise(r => setTimeout(r, 40));
        }

        // 2. Refresh active database with full schema if present
        const activeDb = schemaCatalogCache ? schemaCatalogCache.database_name : '';
        if (activeDb) {
          if (progressLabel) progressLabel.textContent = `Refreshing active database (${activeDb})...`;
          const actRes = await apiFetch(`/api/session/${connId}/schema?db=${encodeURIComponent(activeDb)}`);
          if (actRes.ok) {
            const sc = await actRes.json();
            schemaCatalogCache = sc;
            multiDBCatalogCache[activeDb.toLowerCase()] = sc;
            sessionCatalogTimestamps[connId][activeDb.toLowerCase()] = Date.now();
            renderSchemaTree(sc);
            updateSchemaTimestampBadge(activeDb);
          }
        }

        renderDatabaseDropdownList();
        if (progressLabel) progressLabel.textContent = 'Complete! All schema catalogs synchronized.';
        await new Promise(r => setTimeout(r, 600));
        closeRefreshAllModal();
        showToast('All database schema catalogs refreshed successfully.', 'success');
      } catch (err) {
        showNativeAlert('Failed to refresh catalogs:\n' + err.message, 'Refresh Error');
      } finally {
        isExecutingRefreshAll = false;
        if (cancelBtn) cancelBtn.disabled = false;
        if (proceedBtn) {
          proceedBtn.disabled = false;
          proceedBtn.textContent = 'Proceed with Refresh All';
        }
      }
    }

