// --- Create Table Designer Modal Handlers ---
    function openCreateTableModal() {
      newTableColumns = [
        { name: 'id', type: 'INT', is_primary_key: true, is_nullable: false, default_value: '', extra: 'auto_increment' },
        { name: 'created_at', type: 'TIMESTAMP', is_primary_key: false, is_nullable: false, default_value: 'CURRENT_TIMESTAMP', extra: '' }
      ];
      document.getElementById('new-table-name').value = '';
      renderCreatorColumns();
      const dlg = document.getElementById('table-creator-dialog');
      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    function renderCreatorColumns() {
      const tbody = document.getElementById('creator-columns-tbody');
      tbody.innerHTML = newTableColumns.map((c, i) => {
        const { base, length } = parseColumnType(c.type);
        const isLenDisabled = isLengthlessType(base);
        const isInt = isIntegerType(base);
        const isEnum = base === 'ENUM' || base === 'SET';

        const typeLengthHtml = isEnum ? `
          <button type="button" onclick="openEnumValuesPopover(event, 'creator', ${i})" class="flex items-center space-x-1.5 bg-slate-900 border border-slate-700 hover:border-app-accent px-2 py-1 rounded text-sky-300 font-mono text-xs max-w-[150px] truncate group transition shadow-inner" title="Click to edit ${base} values">
            <span class="group-hover:scale-110 transition-transform">${renderIcon('bookmark', 'w-3.5 h-3.5 text-sky-400')}</span>
            <span id="creator-enum-pill-${i}" class="truncate">${renderEnumPillSummary(length)}</span>
          </button>
        ` : `
          <input type="text" value="${escapeHtml(length)}" ${isLenDisabled ? 'disabled placeholder="N/A"' : 'placeholder="Len"'} oninput="updateCreatorColLength(${i}, this.value)" class="w-16 bg-slate-900 border border-slate-700 text-white text-xs py-1 px-1 rounded outline-none font-mono ${isLenDisabled ? 'opacity-30 cursor-not-allowed bg-slate-950' : 'focus:border-app-accent'}">
        `;

        return `
          <tr>
            <td class="p-1.5 text-center">
              <button type="button" onclick="toggleCreatorColPK(${i})" class="p-1 rounded hover:bg-slate-800 ${c.is_primary_key ? 'text-amber-400 font-bold' : 'text-slate-600'}" title="Toggle Primary Key">${renderIcon('key', 'w-3.5 h-3.5 text-amber-400')}</button>
            </td>
            <td class="p-1.5">
              <input type="text" value="${escapeHtml(c.name)}" oninput="newTableColumns[${i}].name = this.value; validateCreatorColumns();" class="w-full bg-slate-900 border border-slate-700 px-1.5 py-1 rounded text-white font-mono text-xs outline-none focus:border-app-accent">
            </td>
            <td class="p-1.5">
              <div class="flex items-center space-x-1">
                <select onchange="updateCreatorColTypeBase(${i}, this.value)" class="bg-slate-900 border border-slate-700 text-sky-200 text-xs py-1 px-1 rounded outline-none font-mono focus:border-app-accent">
                  ${renderTypeSelectOptions(base)}
                </select>
                ${typeLengthHtml}
              </div>
            </td>
            <td class="p-1.5 text-center">
              <input type="checkbox" ${c.is_nullable ? 'checked' : ''} ${c.is_primary_key ? 'disabled title="Primary Key cannot be nullable"' : ''} onchange="newTableColumns[${i}].is_nullable = this.checked; if (!this.checked && newTableColumns[${i}].default_value === 'NULL') newTableColumns[${i}].default_value = ''; renderCreatorColumns();" class="rounded bg-slate-900 text-app-accent ${c.is_primary_key ? 'opacity-30 cursor-not-allowed' : ''}">
            </td>
            <td class="p-1.5">
              ${renderDefaultPresetSelect(c, i, 'creator')}
            </td>
            <td class="p-1.5 text-center">
              <input type="checkbox" ${(c.extra || '').toLowerCase().includes('auto_increment') ? 'checked' : ''} ${!isInt ? 'disabled title="Only integer types can auto-increment"' : ''} onchange="newTableColumns[${i}].extra = this.checked ? 'auto_increment' : ''" class="rounded bg-slate-900 text-app-accent ${!isInt ? 'opacity-30 cursor-not-allowed' : ''}">
            </td>
            <td class="p-1.5 text-center">
              <button onclick="newTableColumns.splice(${i}, 1); renderCreatorColumns();" class="text-slate-500 hover:text-red-400">✕</button>
            </td>
          </tr>
        `;
      }).join('');

      validateCreatorColumns();
    }

    function updateCreatorColTypeBase(idx, newBase) {
      if (!newTableColumns[idx]) return;
      const col = newTableColumns[idx];
      const parsed = parseColumnType(col.type);
      let base = (newBase || 'VARCHAR').toUpperCase();
      let length = parsed.length;

      if (isLengthlessType(base)) {
        length = '';
      } else if (base === 'ENUM' || base === 'SET') {
        length = (length && (parsed.base === 'ENUM' || parsed.base === 'SET')) ? length : "'option1', 'option2'";
      } else if (!length) {
        if (base === 'VARCHAR') length = '255';
        else if (base === 'DECIMAL') length = '10,2';
        else if (base === 'CHAR') length = '1';
      }

      if (!isIntegerType(base) && (col.extra || '').toLowerCase().includes('auto_increment')) {
        col.extra = '';
      }

      col.type = formatColumnType(base, length);
      renderCreatorColumns();
    }

    function updateCreatorColLength(idx, newLength) {
      if (!newTableColumns[idx]) return;
      const col = newTableColumns[idx];
      const parsed = parseColumnType(col.type);
      col.type = formatColumnType(parsed.base, newLength);
      validateCreatorColumns();
    }

    function updateCreatorColDefaultPreset(idx, preset) {
      if (!newTableColumns[idx]) return;
      const col = newTableColumns[idx];
      if (preset.startsWith('__ENUM__:')) {
        col.default_value = preset.slice(9);
      } else if (preset === 'none') {
        col.default_value = '';
      } else if (preset === 'NULL') {
        col.default_value = 'NULL';
        if (!col.is_primary_key) col.is_nullable = true;
      } else if (preset === 'CURRENT_TIMESTAMP') {
        col.default_value = 'CURRENT_TIMESTAMP';
      } else if (preset === '0') {
        col.default_value = '0';
      } else if (preset === 'empty_string') {
        col.default_value = "''";
      } else if (preset === 'custom') {
        if (!col.default_value || ['NULL', 'CURRENT_TIMESTAMP', '0', "''"].includes(col.default_value)) {
          col.default_value = '';
        }
      }
      renderCreatorColumns();
    }

    function validateCreatorColumns() {
      // In-place validation for table creator
      return validateTableColumns(newTableColumns, true);
    }

    function toggleCreatorColPK(idx) {
      if (!newTableColumns[idx]) return;
      const col = newTableColumns[idx];
      col.is_primary_key = !col.is_primary_key;
      if (col.is_primary_key) {
        col.is_nullable = false;
        if (col.default_value === 'NULL') col.default_value = '';
      }
      renderCreatorColumns();
    }

    function addCreatorColumn() {
      newTableColumns.push({
        name: 'col_' + (newTableColumns.length + 1),
        type: 'VARCHAR(255)',
        is_primary_key: false,
        is_nullable: true,
        default_value: '',
        extra: ''
      });
      renderCreatorColumns();
    }

    async function submitCreateTable() {
      const name = document.getElementById('new-table-name').value.trim();
      if (!name) {
        showNativeAlert('Table name is required.', 'Create Table');
        return;
      }
      if (newTableColumns.length === 0) {
        showNativeAlert('At least one column is required.', 'Create Table');
        return;
      }

      const errors = validateTableColumns(newTableColumns, true);
      if (errors.length > 0) {
        showNativeAlert('Please resolve column errors before creating the table:\n• ' + errors.join('\n• '), 'Table Validation');
        return;
      }

      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const colDefs = newTableColumns.map(c => {
        const { base } = parseColumnType(c.type);
        const nullClause = c.is_nullable ? '' : 'NOT NULL';
        const dfltClause = formatDefaultClause(c.default_value, base);
        const quote = isSQLite ? '"' : '`';
        return `  ${quote}${c.name}${quote} ${c.type} ${nullClause} ${dfltClause} ${c.extra || ''}`.replace(/\s+/g, ' ').trim();
      });

      const pkCols = newTableColumns.filter(c => c.is_primary_key).map(c => (isSQLite ? `"${c.name}"` : `\`${c.name}\``));
      if (pkCols.length > 0) {
        colDefs.push(`  PRIMARY KEY (${pkCols.join(', ')})`);
      }

      const quote = isSQLite ? '"' : '`';
      const sql = `CREATE TABLE ${quote}${name}${quote} (\n${colDefs.join(',\n')}\n);`;
      document.getElementById('table-creator-dialog').close();

      openDDLReviewModal(sql, async () => {
        const ok = await executeDDLStatements(sql, `Table "${name}" created successfully.`);
        if (ok) {
          await loadSchemaTree(activeConnection.id);
          openTableViewer(name);
        }
      });
    }

    async function dropTable(tableName) {
      if (!await showNativeConfirm(`Are you sure you want to DROP TABLE "${tableName}"?\nAll table data will be removed.`, 'Drop Table')) {
        return;
      }
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const quote = isSQLite ? '"' : '`';
      const sql = `DROP TABLE ${quote}${tableName}${quote};`;
      openDDLReviewModal(sql, async () => {
        const ok = await executeDDLStatements(sql, `Table "${tableName}" dropped successfully.`);
        if (ok) {
          await loadSchemaTree(activeConnection.id);
        }
      });
    }

    // --- Shared Modal CodeMirror & Hybrid Tab Handoff Architecture ---
    let viewCMInstance = null;
    let triggerCMInstance = null;

    function stripDefiner(sql) {
      if (!sql || typeof sql !== 'string') return '';
      return sql.replace(/\s*(?:\/\*!\d+\s*)?DEFINER\s*=\s*(?:`[^`]+`|'[^']+'|[^\s@]+)@(?:`[^`]+`|'[^']+'|[^\s]+)(?:\s*\*\/)?/gi, '');
    }

    function extractViewSelectQuery(ddl) {
      if (!ddl || typeof ddl !== 'string') return '';
      const clean = stripDefiner(ddl);
      const viewMatch = clean.match(/\bVIEW\s+(?:`[^`]+`|"[^"]+"|[^\s(]+)(?:\s*\([^)]*\))?\s+AS\s+([\s\S]+)$/i);
      if (viewMatch) {
        return viewMatch[1].trim().replace(/;+$/, '');
      }
      const match = clean.match(/\bAS\s+([\s\S]+)$/i);
      if (match) {
        return match[1].trim().replace(/;+$/, '');
      }
      return clean.trim();
    }

    function getOrCreateModalCodeMirror(containerId, initialValue = '', mode = 'text/x-mysql') {
      const container = document.getElementById(containerId);
      if (!container || typeof CodeMirror === 'undefined') return null;
      if (container._cmInstance) {
        container._cmInstance.setValue(initialValue || '');
        return container._cmInstance;
      }
      container.innerHTML = '';
      const cm = CodeMirror(container, {
        value: initialValue || '',
        mode: mode,
        lineNumbers: true,
        matchBrackets: true,
        autoCloseBrackets: true,
        lineWrapping: true,
        completionSource: typeof cm6SqlCompletionSource === 'function' ? cm6SqlCompletionSource : undefined,
        extraKeys: {
          'Ctrl-Space': function(cmInst) {
            if (typeof sqlContextHint === 'function') {
              cmInst.showHint({ hint: sqlContextHint, completeSingle: false });
            }
          }
        }
      });
      container._cmInstance = cm;
      return cm;
    }

    function openInQueryEditor(sql, title = null) {
      addNewQueryTab();
      const tab = tabs.find(t => t.id === activeTabId);
      if (tab) {
        if (title) tab.title = title;
        tab.sql = sql;
      }
      setEditorSQL(sql);
      renderTabs();
      if (cmEditor) {
        cmEditor.focus();
        cmEditor.setCursor(cmEditor.lineCount(), 0);
      }
    }
    window.openInQueryEditor = openInQueryEditor;

    // --- View Designer Handlers ---
    async function openCreateViewModal(viewName = '', existingSQL = '') {
      document.getElementById('view-designer-title').textContent = viewName ? 'Alter View ' + viewName : 'Create Database View';
      document.getElementById('view-designer-name').value = viewName;
      const dlg = document.getElementById('view-designer-dialog');
      dlg.dataset.isEditing = viewName ? 'true' : 'false';
      dlg.dataset.originalName = viewName || '';

      let queryToUse = existingSQL || '';
      if (viewName && !existingSQL && activeConnection) {
        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/ddl/${encodeURIComponent(viewName)}`);
          if (res.ok) {
            const data = await res.json();
            if (data && data.ddl) {
              queryToUse = extractViewSelectQuery(data.ddl);
            }
          }
        } catch (err) {
          console.warn('Failed to introspect existing view DDL:', err);
        }
      }

      if (!queryToUse) {
        queryToUse = 'SELECT * FROM ...';
      }

      document.getElementById('view-designer-query').value = queryToUse;
      viewCMInstance = getOrCreateModalCodeMirror('view-designer-cm-container', queryToUse);

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');

      setTimeout(() => {
        if (viewCMInstance) {
          viewCMInstance.refresh();
          viewCMInstance.focus();
        }
      }, 50);
    }

    function getViewEditorQuery() {
      if (viewCMInstance) return viewCMInstance.getValue().trim();
      const el = document.getElementById('view-designer-query');
      return el ? el.value.trim() : '';
    }

    function assembleViewDDL(name, query, isEditing = false, originalName = '') {
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const quote = isSQLite ? '"' : '`';
      const safeName = isSQLite ? name.replace(/"/g, '""') : name.replace(/`/g, '``');
      const cleanQuery = (query || '').trim().replace(/;+$/, '');
      let sql = '';
      if (isEditing && originalName && originalName !== name) {
        const safeOrig = isSQLite ? originalName.replace(/"/g, '""') : originalName.replace(/`/g, '``');
        sql += `DROP VIEW IF EXISTS ${quote}${safeOrig}${quote};\n`;
      }
      const createStmt = isSQLite
        ? `CREATE VIEW IF NOT EXISTS ${quote}${safeName}${quote} AS\n${cleanQuery};`
        : `CREATE OR REPLACE VIEW ${quote}${safeName}${quote} AS\n${cleanQuery};`;
      return sql + createStmt;
    }

    function openActiveViewInQueryTab() {
      const dlg = document.getElementById('view-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const originalName = dlg ? dlg.dataset.originalName || '' : '';
      const name = document.getElementById('view-designer-name').value.trim() || 'v_new_view';
      const query = getViewEditorQuery() || 'SELECT 1';
      const sql = assembleViewDDL(name, query, isEditing, originalName);
      if (dlg) dlg.close();
      openInQueryEditor(sql, `View: ${name}`);
    }

    async function submitCreateView() {
      const dlg = document.getElementById('view-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const originalName = dlg ? dlg.dataset.originalName || '' : '';
      const name = document.getElementById('view-designer-name').value.trim();
      const query = getViewEditorQuery();
      if (!name || !query) {
        showNativeAlert('View name and SELECT query are required.', 'View Designer');
        return;
      }
      const sql = assembleViewDDL(name, query, isEditing, originalName);

      if (dlg) dlg.close();
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `View "${name}" saved successfully.`);
      });
    }

    async function dropView(viewName) {
      if (!await showNativeConfirm(`Are you sure you want to DROP VIEW "${viewName}"?`, 'Drop View')) {
        return;
      }
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const quote = isSQLite ? '"' : '`';
      const safeName = isSQLite ? viewName.replace(/"/g, '""') : viewName.replace(/`/g, '``');
      const sql = `DROP VIEW IF EXISTS ${quote}${safeName}${quote};`;
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `View "${viewName}" dropped successfully.`);
      });
    }

    // --- Trigger Designer Handlers ---
    function openCreateTriggerModal(triggerName = '', existingData = null) {
      document.getElementById('trigger-designer-title').textContent = triggerName ? 'Edit Trigger ' + triggerName : 'Create Database Trigger';
      document.getElementById('trigger-designer-name').value = triggerName;
      const dlg = document.getElementById('trigger-designer-dialog');
      dlg.dataset.isEditing = triggerName ? 'true' : 'false';
      dlg.dataset.originalName = triggerName || '';

      const tblSelect = document.getElementById('trigger-designer-table');
      if (tblSelect && schemaCatalogCache) {
        const baseTables = (schemaCatalogCache.tables || []).filter(t => t.type !== 'VIEW');
        tblSelect.innerHTML = baseTables.map(t => `<option value="${escapeHtml(t.name)}">${escapeHtml(t.name)}</option>`).join('');
      }

      if (!existingData && triggerName && schemaCatalogCache && Array.isArray(schemaCatalogCache.triggers)) {
        existingData = schemaCatalogCache.triggers.find(tr => tr.name === triggerName) || null;
      }

      let bodyToUse = 'BEGIN\n  -- Trigger logic here\nEND;';
      if (existingData) {
        if (existingData.table && tblSelect) {
          if (!tblSelect.querySelector(`option[value="${CSS.escape ? CSS.escape(existingData.table) : existingData.table}"]`)) {
            const opt = document.createElement('option');
            opt.value = existingData.table;
            opt.textContent = existingData.table;
            tblSelect.appendChild(opt);
          }
          tblSelect.value = existingData.table;
        }
        if (existingData.timing) document.getElementById('trigger-designer-timing').value = existingData.timing;
        if (existingData.event) document.getElementById('trigger-designer-event').value = existingData.event;
        if (existingData.statement) bodyToUse = existingData.statement;
      }

      document.getElementById('trigger-designer-body').value = bodyToUse;
      triggerCMInstance = getOrCreateModalCodeMirror('trigger-designer-cm-container', bodyToUse);

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');

      setTimeout(() => {
        if (triggerCMInstance) {
          triggerCMInstance.refresh();
          triggerCMInstance.focus();
        }
      }, 50);
    }

    function getTriggerEditorBody() {
      if (triggerCMInstance) return triggerCMInstance.getValue().trim();
      const el = document.getElementById('trigger-designer-body');
      return el ? el.value.trim() : '';
    }

    function assembleTriggerDDL(name, table, timing, event, body, isEditing = false, originalName = '') {
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const quote = isSQLite ? '"' : '`';
      const safeName = isSQLite ? name.replace(/"/g, '""') : name.replace(/`/g, '``');
      const safeTable = isSQLite ? table.replace(/"/g, '""') : table.replace(/`/g, '``');
      const cleanBody = (body || '').trim().replace(/;+$/, '');
      let sql = '';
      if (isEditing) {
        if (originalName && originalName !== name) {
          const safeOrig = isSQLite ? originalName.replace(/"/g, '""') : originalName.replace(/`/g, '``');
          sql += `DROP TRIGGER IF EXISTS ${quote}${safeOrig}${quote};\n`;
        }
        sql += `DROP TRIGGER IF EXISTS ${quote}${safeName}${quote};\n`;
      }
      if (isSQLite) {
        sql += `CREATE TRIGGER ${quote}${safeName}${quote} ${timing} ${event} ON ${quote}${safeTable}${quote}\n${cleanBody};`;
      } else {
        sql += `CREATE TRIGGER ${quote}${safeName}${quote} ${timing} ${event} ON ${quote}${safeTable}${quote} FOR EACH ROW\n${cleanBody};`;
      }
      return sql;
    }

    function openActiveTriggerInQueryTab() {
      const dlg = document.getElementById('trigger-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const originalName = dlg ? dlg.dataset.originalName || '' : '';
      const name = document.getElementById('trigger-designer-name').value.trim() || 'trg_new';
      const table = document.getElementById('trigger-designer-table').value || 'target_table';
      const timing = document.getElementById('trigger-designer-timing').value;
      const event = document.getElementById('trigger-designer-event').value;
      const body = getTriggerEditorBody() || 'BEGIN\n  -- Trigger logic\nEND;';
      const sql = assembleTriggerDDL(name, table, timing, event, body, isEditing, originalName);
      if (dlg) dlg.close();
      openInQueryEditor(sql, `Trigger: ${name}`);
    }

    async function submitCreateTrigger() {
      const dlg = document.getElementById('trigger-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const originalName = dlg ? dlg.dataset.originalName || '' : '';
      const name = document.getElementById('trigger-designer-name').value.trim();
      const table = document.getElementById('trigger-designer-table').value;
      const timing = document.getElementById('trigger-designer-timing').value;
      const event = document.getElementById('trigger-designer-event').value;
      const body = getTriggerEditorBody();

      if (!name || !table || !body) {
        showNativeAlert('Trigger name, target table, and body statements are required.', 'Trigger Designer');
        return;
      }

      const sql = assembleTriggerDDL(name, table, timing, event, body, isEditing, originalName);

      if (dlg) dlg.close();
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `Trigger "${name}" saved successfully.`);
      });
    }

    async function dropTrigger(triggerName) {
      if (!await showNativeConfirm(`Are you sure you want to DROP TRIGGER "${triggerName}"?`, 'Drop Trigger')) {
        return;
      }
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const quote = isSQLite ? '"' : '`';
      const safeName = isSQLite ? triggerName.replace(/"/g, '""') : triggerName.replace(/`/g, '``');
      const sql = `DROP TRIGGER IF EXISTS ${quote}${safeName}${quote};`;
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `Trigger "${triggerName}" dropped successfully.`);
      });
    }

    // --- Routine Designer Handlers ---
    let routineCMInstance = null;

    function toggleRoutineTypeUI() {
      const type = document.getElementById('routine-designer-type').value;
      const returnContainer = document.getElementById('routine-return-type-container');
      const isFunc = type === 'FUNCTION';
      if (returnContainer) {
        if (isFunc) returnContainer.classList.remove('hidden');
        else returnContainer.classList.add('hidden');
      }
      const dlg = document.getElementById('routine-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const typeDisplay = isFunc ? 'Function' : 'Procedure';
      if (!isEditing) {
        document.getElementById('routine-designer-title').textContent = `Create Stored ${typeDisplay}`;
      }
      const nameInput = document.getElementById('routine-designer-name');
      if (nameInput) {
        nameInput.placeholder = isFunc ? 'fn_calculate_tax' : 'sp_process_orders';
      }
      document.querySelectorAll('.routine-param-mode').forEach(sel => {
        sel.style.display = isFunc ? 'none' : 'block';
      });
    }

    function addRoutineParameterRow(name = '', mode = 'IN', type = 'INT') {
      const list = document.getElementById('routine-params-list');
      if (!list) return;
      const isFunc = document.getElementById('routine-designer-type').value === 'FUNCTION';
      const row = document.createElement('div');
      row.className = 'flex items-center space-x-1.5 routine-param-row';
      row.innerHTML = `
        <select class="routine-param-mode bg-slate-950 border border-slate-700 p-1 text-white font-mono rounded text-[11px] w-20" style="display: ${isFunc ? 'none' : 'block'};">
          <option value="IN" ${mode === 'IN' ? 'selected' : ''}>IN</option>
          <option value="OUT" ${mode === 'OUT' ? 'selected' : ''}>OUT</option>
          <option value="INOUT" ${mode === 'INOUT' ? 'selected' : ''}>INOUT</option>
        </select>
        <input type="text" placeholder="param_name" value="${escapeHtml(name)}" class="routine-param-name flex-1 bg-slate-950 border border-slate-700 p-1 text-white font-mono rounded text-[11px] outline-none focus:border-app-accent">
        <input type="text" placeholder="INT / VARCHAR(255)" value="${escapeHtml(type)}" class="routine-param-type flex-1 bg-slate-950 border border-slate-700 p-1 text-white font-mono rounded text-[11px] outline-none focus:border-app-accent">
        <button type="button" onclick="this.closest('.routine-param-row').remove()" class="text-slate-500 hover:text-rose-400 p-1 font-mono text-xs" title="Remove parameter">✕</button>
      `;
      list.appendChild(row);
    }

    async function openCreateRoutineModal(routineName = '', routineType = 'PROCEDURE') {
      const isEditing = Boolean(routineName);
      const dlg = document.getElementById('routine-designer-dialog');
      dlg.dataset.isEditing = isEditing ? 'true' : 'false';
      dlg.dataset.originalName = routineName || '';
      dlg.dataset.originalType = routineType || 'PROCEDURE';

      const isFunc = (routineType || '').toUpperCase() === 'FUNCTION';
      const typeDisplay = isFunc ? 'Function' : 'Procedure';
      document.getElementById('routine-designer-title').textContent = isEditing
        ? `Edit Stored ${typeDisplay} "${routineName}"`
        : `Create Stored ${typeDisplay}`;
      document.getElementById('routine-designer-name').value = routineName || '';
      document.getElementById('routine-designer-type').value = routineType || 'PROCEDURE';
      document.getElementById('routine-designer-deterministic').checked = true;
      document.getElementById('routine-designer-returns').value = 'INT';

      const paramsList = document.getElementById('routine-params-list');
      if (paramsList) paramsList.innerHTML = '';

      let bodyToUse = routineType === 'FUNCTION'
        ? 'BEGIN\n  -- Statements\n  RETURN 0;\nEND'
        : 'BEGIN\n  -- Statements\nEND';

      if (isEditing && activeConnection) {
        const routineMeta = schemaCatalogCache?.routines?.find(r => r.name === routineName && (r.type || '').toUpperCase() === (routineType || '').toUpperCase());
        if (routineMeta) {
          if (routineMeta.data_type) {
            document.getElementById('routine-designer-returns').value = routineMeta.data_type;
          }
          if (Array.isArray(routineMeta.parameters)) {
            routineMeta.parameters.forEach(p => {
              addRoutineParameterRow(p.name, p.mode || 'IN', p.type || 'INT');
            });
          }
        }

        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/ddl/routine/${encodeURIComponent(routineName)}?type=${encodeURIComponent(routineType)}`);
          if (res.ok) {
            const data = await res.json();
            if (data && data.ddl) {
              const cleanDDL = stripDefiner(data.ddl);
              if (/\bDETERMINISTIC\b/i.test(cleanDDL) && !/\bNOT\s+DETERMINISTIC\b/i.test(cleanDDL)) {
                document.getElementById('routine-designer-deterministic').checked = true;
              } else if (/\bNOT\s+DETERMINISTIC\b/i.test(cleanDDL)) {
                document.getElementById('routine-designer-deterministic').checked = false;
              }
              const beginMatch = cleanDDL.match(/\bBEGIN\b[\s\S]*\bEND\b/i);
              if (beginMatch) {
                bodyToUse = beginMatch[0];
              } else {
                const returnMatch = cleanDDL.match(/\bRETURN\b[\s\S]+$/i);
                if (returnMatch) {
                  bodyToUse = returnMatch[0].replace(/;+$/, '');
                }
              }
            }
          }
        } catch (err) {
          console.warn('Failed to introspect routine DDL:', err);
        }
      }

      toggleRoutineTypeUI();

      routineCMInstance = getOrCreateModalCodeMirror('routine-designer-cm-container', bodyToUse);

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');

      setTimeout(() => {
        if (routineCMInstance) {
          routineCMInstance.refresh();
          routineCMInstance.focus();
        }
      }, 50);
    }

    function getRoutineEditorBody() {
      if (routineCMInstance) return routineCMInstance.getValue().trim();
      return 'BEGIN\nEND';
    }

    function collectRoutineParameters() {
      const params = [];
      document.querySelectorAll('.routine-param-row').forEach(row => {
        const name = (row.querySelector('.routine-param-name')?.value || '').trim();
        const type = (row.querySelector('.routine-param-type')?.value || '').trim() || 'INT';
        const mode = (row.querySelector('.routine-param-mode')?.value || 'IN').trim();
        if (name) {
          params.push({ name, mode, type });
        }
      });
      return params;
    }

    function assembleRoutineDDL(name, type, params, returnType, isDeterministic, body, isEditing = false, originalName = '', originalType = '') {
      const quote = '`';
      const isFunc = type.toUpperCase() === 'FUNCTION';
      const cleanBody = (body || '').trim().replace(/;+$/, '');
      const paramList = params.map(p => {
        const pName = `${quote}${p.name.replace(/`/g, '``')}${quote}`;
        const pType = p.type;
        if (isFunc) return `${pName} ${pType}`;
        const mode = (p.mode || 'IN').toUpperCase();
        return `${mode} ${pName} ${pType}`;
      }).join(', ');

      let header = isFunc
        ? `CREATE FUNCTION ${quote}${name.replace(/`/g, '``')}${quote}(${paramList}) RETURNS ${returnType || 'INT'}`
        : `CREATE PROCEDURE ${quote}${name.replace(/`/g, '``')}${quote}(${paramList})`;

      if (isDeterministic) {
        header += ' DETERMINISTIC';
      } else {
        header += ' NOT DETERMINISTIC';
      }

      const createStmt = `${header}\n${cleanBody};`;
      if (isEditing) {
        const dropKeyword = isFunc ? 'FUNCTION' : 'PROCEDURE';
        let dropSQL = '';
        if (originalName && (originalName !== name || (originalType && originalType.toUpperCase() !== type.toUpperCase()))) {
          const origKeyword = (originalType || type).toUpperCase() === 'FUNCTION' ? 'FUNCTION' : 'PROCEDURE';
          dropSQL += `DROP ${origKeyword} IF EXISTS ${quote}${originalName.replace(/`/g, '``')}${quote};\n`;
        }
        dropSQL += `DROP ${dropKeyword} IF EXISTS ${quote}${name.replace(/`/g, '``')}${quote};\n`;
        return `${dropSQL}${createStmt}`;
      }
      return createStmt;
    }

    function openActiveRoutineInQueryTab() {
      const dlg = document.getElementById('routine-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const originalName = dlg ? dlg.dataset.originalName || '' : '';
      const originalType = dlg ? dlg.dataset.originalType || '' : '';
      const name = document.getElementById('routine-designer-name').value.trim() || 'sp_new';
      const type = document.getElementById('routine-designer-type').value;
      const returnType = document.getElementById('routine-designer-returns').value.trim() || 'INT';
      const isDeterministic = document.getElementById('routine-designer-deterministic').checked;
      const params = collectRoutineParameters();
      const body = getRoutineEditorBody();

      const sql = assembleRoutineDDL(name, type, params, returnType, isDeterministic, body, isEditing, originalName, originalType);
      if (dlg) dlg.close();
      openInQueryEditor(sql, `${type}: ${name}`);
    }

    async function submitCreateRoutine() {
      const dlg = document.getElementById('routine-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const originalName = dlg ? dlg.dataset.originalName || '' : '';
      const originalType = dlg ? dlg.dataset.originalType || '' : '';
      const name = document.getElementById('routine-designer-name').value.trim();
      const type = document.getElementById('routine-designer-type').value;
      const returnType = document.getElementById('routine-designer-returns').value.trim();
      const isDeterministic = document.getElementById('routine-designer-deterministic').checked;
      const params = collectRoutineParameters();
      const body = getRoutineEditorBody();

      if (!name) {
        showNativeAlert('Routine name is required.', 'Routine Designer');
        return;
      }
      if (type === 'FUNCTION' && !returnType) {
        showNativeAlert('Function return data type is required (e.g. INT, VARCHAR(255)).', 'Routine Designer');
        return;
      }
      if (!body) {
        showNativeAlert('Routine body statements are required.', 'Routine Designer');
        return;
      }

      const sql = assembleRoutineDDL(name, type, params, returnType, isDeterministic, body, isEditing, originalName, originalType);

      if (dlg) dlg.close();
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `${type} "${name}" saved successfully.`);
      });
    }

    async function dropRoutine(name, type = 'PROCEDURE') {
      const upperType = (type || 'PROCEDURE').toUpperCase();
      if (!await showNativeConfirm(`Are you sure you want to DROP ${upperType} "${name}"?`, `Drop ${upperType}`)) {
        return;
      }
      const quote = '`';
      const sql = `DROP ${upperType} IF EXISTS ${quote}${name.replace(/`/g, '``')}${quote};`;
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `${upperType} "${name}" dropped successfully.`);
      });
    }

    // --- Event Designer Handlers ---
    let eventCMInstance = null;

    function toggleEventScheduleTypeUI() {
      const mode = document.querySelector('input[name="event-schedule-type"]:checked')?.value || 'EVERY';
      const everyContainer = document.getElementById('event-sched-every-container');
      const atContainer = document.getElementById('event-sched-at-container');
      if (everyContainer && atContainer) {
        if (mode === 'EVERY') {
          everyContainer.classList.remove('hidden');
          atContainer.classList.add('hidden');
        } else {
          everyContainer.classList.add('hidden');
          atContainer.classList.remove('hidden');
        }
      }
    }

    async function openCreateEventModal(eventName = '') {
      const isEditing = Boolean(eventName);
      const dlg = document.getElementById('event-designer-dialog');
      dlg.dataset.isEditing = isEditing ? 'true' : 'false';
      dlg.dataset.originalName = eventName || '';

      document.getElementById('event-designer-title').textContent = isEditing
        ? `Edit Event "${eventName}"`
        : 'Create Scheduled Event';
      document.getElementById('event-designer-name').value = eventName || '';
      document.getElementById('event-designer-status').value = 'ENABLE';
      document.getElementById('event-designer-preserve').checked = true;

      const everyRadio = document.querySelector('input[name="event-schedule-type"][value="EVERY"]');
      if (everyRadio) everyRadio.checked = true;
      document.getElementById('event-designer-interval-val').value = '1';
      document.getElementById('event-designer-interval-unit').value = 'DAY';
      document.getElementById('event-designer-starts').value = '';
      document.getElementById('event-designer-ends').value = '';
      document.getElementById('event-designer-at').value = '';

      let bodyToUse = 'BEGIN\n  -- Statements\nEND';

      if (isEditing && activeConnection) {
        const evMeta = schemaCatalogCache?.events?.find(e => e.name === eventName);
        if (evMeta) {
          if (evMeta.status) {
            document.getElementById('event-designer-status').value = evMeta.status.toUpperCase().includes('ENABLE') ? 'ENABLE' : 'DISABLE';
          }
          if (evMeta.interval && evMeta.interval.toUpperCase().startsWith('EVERY ')) {
            const parts = evMeta.interval.trim().split(/\s+/);
            if (parts.length >= 3) {
              document.getElementById('event-designer-interval-val').value = parts[1];
              document.getElementById('event-designer-interval-unit').value = parts[2].toUpperCase();
            }
          }
          if (evMeta.starts_at) {
            document.getElementById('event-designer-starts').value = evMeta.starts_at;
          }
          if (evMeta.ends_at) {
            document.getElementById('event-designer-ends').value = evMeta.ends_at;
          }
          if (evMeta.definition) {
            bodyToUse = evMeta.definition.trim();
          }
        }

        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/ddl/event/${encodeURIComponent(eventName)}`);
          if (res.ok) {
            const data = await res.json();
            if (data && data.ddl) {
              const cleanDDL = stripDefiner(data.ddl);
              const doMatch = cleanDDL.match(/\bDO\s+([\s\S]+)$/i);
              if (doMatch) {
                bodyToUse = doMatch[1].trim().replace(/;$/, '');
              }
              if (/\bON\s+COMPLETION\s+NOT\s+PRESERVE\b/i.test(cleanDDL)) {
                document.getElementById('event-designer-preserve').checked = false;
              }
            }
          }
        } catch (err) {
          console.warn('Failed to introspect event DDL:', err);
        }
      }

      toggleEventScheduleTypeUI();

      eventCMInstance = getOrCreateModalCodeMirror('event-designer-cm-container', bodyToUse);

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');

      setTimeout(() => {
        if (eventCMInstance) {
          eventCMInstance.refresh();
          eventCMInstance.focus();
        }
      }, 50);
    }

    function getEventEditorBody() {
      if (eventCMInstance) return eventCMInstance.getValue().trim();
      return 'BEGIN\nEND';
    }

    function assembleEventDDL(name, scheduleMode, intervalVal, intervalUnit, startsAt, endsAt, atTime, status, preserve, body, isEditing = false, originalName = '') {
      const quote = '`';
      const cleanBody = (body || '').trim().replace(/;+$/, '');
      const formatTs = (ts) => /^(?:NOW\(\)|CURRENT_TIMESTAMP)/i.test((ts || '').trim()) ? ts.trim() : `'${(ts || '').replace(/'/g, "''")}'`;
      let sched = '';
      if (scheduleMode === 'EVERY') {
        sched = `EVERY ${intervalVal || 1} ${intervalUnit || 'DAY'}`;
        if (startsAt) sched += ` STARTS ${formatTs(startsAt)}`;
        if (endsAt) sched += ` ENDS ${formatTs(endsAt)}`;
      } else {
        sched = `AT ${formatTs(atTime)}`;
      }

      const completion = preserve ? 'ON COMPLETION PRESERVE' : 'ON COMPLETION NOT PRESERVE';
      const statusStr = status || 'ENABLE';
      const createStmt = `CREATE EVENT ${quote}${name.replace(/`/g, '``')}${quote}\nON SCHEDULE ${sched}\n${completion} ${statusStr}\nDO\n${cleanBody};`;

      if (isEditing) {
        let dropSQL = '';
        if (originalName && originalName !== name) {
          dropSQL += `DROP EVENT IF EXISTS ${quote}${originalName.replace(/`/g, '``')}${quote};\n`;
        }
        dropSQL += `DROP EVENT IF EXISTS ${quote}${name.replace(/`/g, '``')}${quote};\n`;
        return `${dropSQL}${createStmt}`;
      }
      return createStmt;
    }

    function collectEventFormData() {
      const name = document.getElementById('event-designer-name').value.trim();
      const status = document.getElementById('event-designer-status').value;
      const preserve = document.getElementById('event-designer-preserve').checked;
      const scheduleMode = document.querySelector('input[name="event-schedule-type"]:checked')?.value || 'EVERY';
      const intervalVal = document.getElementById('event-designer-interval-val').value.trim() || '1';
      const intervalUnit = document.getElementById('event-designer-interval-unit').value;
      const startsAt = document.getElementById('event-designer-starts').value.trim();
      const endsAt = document.getElementById('event-designer-ends').value.trim();
      const atTime = document.getElementById('event-designer-at').value.trim();
      const body = getEventEditorBody();
      const dlg = document.getElementById('event-designer-dialog');
      const isEditing = dlg && dlg.dataset.isEditing === 'true';
      const originalName = dlg ? dlg.dataset.originalName || '' : '';

      return { name, status, preserve, scheduleMode, intervalVal, intervalUnit, startsAt, endsAt, atTime, body, isEditing, originalName };
    }

    function openActiveEventInQueryTab() {
      const data = collectEventFormData();
      const name = data.name || 'ev_new';
      const sql = assembleEventDDL(name, data.scheduleMode, data.intervalVal, data.intervalUnit, data.startsAt, data.endsAt, data.atTime, data.status, data.preserve, data.body, data.isEditing, data.originalName);
      const dlg = document.getElementById('event-designer-dialog');
      if (dlg) dlg.close();
      openInQueryEditor(sql, `Event: ${name}`);
    }

    async function submitCreateEvent() {
      const data = collectEventFormData();
      if (!data.name) {
        showNativeAlert('Event name is required.', 'Event Designer');
        return;
      }
      if (data.scheduleMode === 'AT' && !data.atTime) {
        showNativeAlert('Timestamp is required for one-time (AT) schedule.', 'Event Designer');
        return;
      }
      if (!data.body) {
        showNativeAlert('Event action statement (DO) is required.', 'Event Designer');
        return;
      }

      const sql = assembleEventDDL(data.name, data.scheduleMode, data.intervalVal, data.intervalUnit, data.startsAt, data.endsAt, data.atTime, data.status, data.preserve, data.body, data.isEditing, data.originalName);

      const dlg = document.getElementById('event-designer-dialog');
      if (dlg) dlg.close();
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `Scheduled event "${data.name}" saved successfully.`);
      });
    }

    async function dropEvent(name) {
      if (!await showNativeConfirm(`Are you sure you want to DROP scheduled event "${name}"?`, 'Drop Event')) {
        return;
      }
      const quote = '`';
      const sql = `DROP EVENT IF EXISTS ${quote}${name.replace(/`/g, '``')}${quote};`;
      openDDLReviewModal(sql, async () => {
        await executeDDLStatements(sql, `Scheduled event "${name}" dropped successfully.`);
      });
    }

