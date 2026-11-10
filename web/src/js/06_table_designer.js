// --- Table Sub-Tabs & Table Designer ---
    function setTableMode(mode, refresh = true) {
      activeTableMode = mode;
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab) activeTab.tableMode = mode;

      const btnData = document.getElementById('btn-mode-data');
      const btnStruct = document.getElementById('btn-mode-structure');
      const btnIdx = document.getElementById('btn-mode-indexes');
      const btnFK = document.getElementById('btn-mode-foreign-keys');
      const btnDDL = document.getElementById('btn-mode-ddl');
      const btnNotes = document.getElementById('btn-mode-notes');
      const btnDiag = document.getElementById('btn-mode-diagnostics');
      const btnParts = document.getElementById('btn-mode-partitions');

      const inactiveClass = 'px-3 py-1 rounded text-xs text-slate-400 hover:text-white hover:bg-slate-800 flex items-center space-x-1';
      const activeClass = 'px-3 py-1 rounded text-xs font-semibold bg-app-surface text-app-accent border-b-2 border-app-accent flex items-center space-x-1';

      if (btnData) btnData.className = mode === 'data' ? activeClass : inactiveClass;
      if (btnStruct) btnStruct.className = mode === 'structure' ? activeClass : inactiveClass;
      if (btnIdx) btnIdx.className = mode === 'indexes' ? activeClass : inactiveClass;
      if (btnFK) btnFK.className = mode === 'foreign-keys' ? activeClass : inactiveClass;
      if (btnDDL) btnDDL.className = mode === 'ddl' ? activeClass : inactiveClass;
      if (btnNotes) btnNotes.className = mode === 'notes' ? activeClass : inactiveClass;
      if (btnDiag) btnDiag.className = mode === 'diagnostics' ? activeClass : inactiveClass;
      if (btnParts) btnParts.className = mode === 'partitions' ? activeClass : inactiveClass;

      const gridToolbar = document.getElementById('grid-toolbar');
      const filterBar = document.getElementById('table-filter-bar');
      const gridTable = document.getElementById('virtual-grid-wrapper');
      const paginationBar = document.getElementById('grid-pagination-bar');
      const structView = document.getElementById('table-structure-view');
      const idxView = document.getElementById('table-indexes-view');
      const fkView = document.getElementById('table-fk-view');
      const ddlView = document.getElementById('table-ddl-view');
      const notesView = document.getElementById('table-notes-view');
      const diagView = document.getElementById('table-diagnostics-view');
      const partsView = document.getElementById('table-partitions-view');

      const isDataViewer = mode === 'data' && activeTab && activeTab.isTableViewer;
      if (gridToolbar) gridToolbar.classList.toggle('hidden', mode !== 'data');
      if (mode === 'data') syncGridToolbarMode();
      if (filterBar) {
        filterBar.classList.toggle('hidden', !isDataViewer);
        if (isDataViewer) syncFilterBarUI(activeTab);
      }
      if (gridTable) gridTable.classList.toggle('hidden', mode !== 'data');
      if (paginationBar) paginationBar.classList.toggle('hidden', mode !== 'data');
      if (structView) structView.classList.toggle('hidden', mode !== 'structure');
      if (idxView) idxView.classList.toggle('hidden', mode !== 'indexes');
      if (fkView) fkView.classList.toggle('hidden', mode !== 'foreign-keys');
      if (ddlView) ddlView.classList.toggle('hidden', mode !== 'ddl');
      if (notesView) notesView.classList.toggle('hidden', mode !== 'notes');
      if (diagView) diagView.classList.toggle('hidden', mode !== 'diagnostics');
      if (partsView) partsView.classList.toggle('hidden', mode !== 'partitions');

      if (refresh && activeTab && activeTab.isTableViewer) {
        if (mode === 'data') {
          loadTableRows(activeTab.tableName);
        } else if (mode === 'structure') {
          renderTableStructure(activeTab.tableName);
        } else if (mode === 'indexes') {
          renderTableIndexes(activeTab.tableName);
        } else if (mode === 'foreign-keys') {
          renderTableFKs(activeTab.tableName);
        } else if (mode === 'ddl') {
          renderTableDDL(activeTab.tableName);
        } else if (mode === 'notes') {
          loadAndRenderTableNote(activeTab.tableName);
        } else if (mode === 'diagnostics') {
          renderTableDiagnostics(activeTab.tableName);
        } else if (mode === 'partitions') {
          renderTablePartitions(activeTab.tableName);
        }
      }
    }

    async function renderTablePartitions(tableName) {
      if (!activeConnection || !tableName) return;
      const countEl = document.getElementById('tp-partitions-count');
      const tbody = document.getElementById('tp-partitions-tbody');
      if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-8 text-center text-slate-400"><div class="animate-pulse">Loading partition telemetry...</div></td></tr>`;

      try {
        const currentDb = schemaCatalogCache?.database_name || activeConnection.database_name || '';
        const res = await apiFetch(`/api/session/${activeConnection.id}/partitions/${encodeURIComponent(tableName)}?db=${encodeURIComponent(currentDb)}`);
        const data = await res.json();
        if (!res.ok) {
          if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-8 text-center text-red-400">Failed to load partitions: ${escapeHtml(data.error || res.statusText)}</td></tr>`;
          return;
        }

        const partitions = Array.isArray(data) ? data : [];
        if (countEl) countEl.textContent = `(${partitions.length} partition${partitions.length === 1 ? '' : 's'})`;

        if (partitions.length === 0) {
          if (tbody) tbody.innerHTML = `
            <tr>
              <td colspan="8" class="p-8 text-center text-slate-400 italic">
                Table <code>${escapeHtml(tableName)}</code> is a standard non-partitioned table.
              </td>
            </tr>
          `;
          return;
        }

        if (tbody) {
          tbody.innerHTML = partitions.map((p, idx) => {
            const dataBytes = formatBytes(p.data_length || 0);
            const idxBytes = formatBytes(p.index_length || 0);
            const rows = (p.table_rows || 0).toLocaleString();

            return `
              <tr class="hover:bg-slate-800/50 transition border-b border-slate-800/60 font-mono text-xs">
                <td class="p-2.5 text-center text-slate-400">${idx + 1}</td>
                <td class="p-2.5 font-bold text-sky-400">${escapeHtml(p.name)}</td>
                <td class="p-2.5 text-slate-300">${escapeHtml(p.method || '-')}</td>
                <td class="p-2.5 text-slate-300"><code>${escapeHtml(p.expression || '-')}</code></td>
                <td class="p-2.5 text-slate-400 text-[11px] max-w-xs truncate" title="${escapeHtml(p.description || '')}">${escapeHtml(p.description || '-')}</td>
                <td class="p-2.5 text-right font-medium text-emerald-400">${rows}</td>
                <td class="p-2.5 text-right text-slate-300">${dataBytes}</td>
                <td class="p-2.5 text-right text-slate-400">${idxBytes}</td>
              </tr>
            `;
          }).join('');
        }
      } catch (err) {
        if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-8 text-center text-red-400">Network error loading partitions: ${escapeHtml(err.message)}</td></tr>`;
      }
    }

    function refreshCurrentTablePartitions() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && activeTab.isTableViewer) {
        renderTablePartitions(activeTab.tableName);
      }
    }



    function renderTableStructure(tableName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetCatalog = (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => t.name === tableName);
      const nameEl = document.getElementById('structure-table-name');
      if (nameEl) nameEl.textContent = tableName;
      if (!tableMeta) {
        document.getElementById('structure-tbody').innerHTML = '<tr><td colspan="8" class="p-4 text-center text-slate-500">Table metadata not found in cache. Refresh schema.</td></tr>';
        return;
      }

      currentTableMetadata = JSON.parse(JSON.stringify(tableMeta));
      if (!stagedStructure || stagedStructure.name !== tableName) {
        stagedStructure = JSON.parse(JSON.stringify(tableMeta));
      }

      renderStructureRows();
    }

    // --- Type Guard & Default Value Presets ---
    // ponytail: Curated standard types prevents syntax errors (e.g. invalid lengths on INT/TEXT)
    const STANDARD_DATA_TYPES = [
      { group: 'Numeric', types: ['INT', 'BIGINT', 'TINYINT', 'SMALLINT', 'DECIMAL', 'FLOAT', 'DOUBLE'] },
      { group: 'Text & String', types: ['VARCHAR', 'CHAR', 'TEXT', 'LONGTEXT', 'MEDIUMTEXT'] },
      { group: 'Selection / Enum', types: ['ENUM', 'SET'] },
      { group: 'Date & Time', types: ['DATETIME', 'TIMESTAMP', 'DATE', 'TIME'] },
      { group: 'Other / Binary', types: ['JSON', 'BOOLEAN', 'BLOB', 'LONGBLOB'] }
    ];

    function parseColumnType(fullType) {
      if (!fullType) return { base: 'VARCHAR', length: '255' };
      const match = String(fullType).match(/^([a-zA-Z0-9_]+)(?:\(([^)]+)\))?/i);
      if (!match) return { base: fullType.toUpperCase(), length: '' };
      return { base: match[1].toUpperCase(), length: match[2] ? match[2].trim() : '' };
    }

    function parseEnumOptionList(raw) {
      if (!raw) return [];
      let s = String(raw).trim();
      const wrapMatch = s.match(/^(?:enum|set)\s*\(([\s\S]+)\)$/i);
      if (wrapMatch) s = wrapMatch[1].trim();
      const list = [];
      const regex = /\s*(?:'((?:''|[^'])*)'|"((?:""|[^"])*)"|([^,]+))\s*(?:,|$)/g;
      let m;
      while ((m = regex.exec(s)) !== null) {
        let val = '';
        if (m[1] !== undefined) val = m[1].replace(/''/g, "'");
        else if (m[2] !== undefined) val = m[2].replace(/""/g, '"');
        else if (m[3] !== undefined) val = m[3].trim().replace(/^['"]+|['"]+$/g, '');
        val = val.trim();
        if (val && !list.includes(val)) list.push(val);
        if (regex.lastIndex === m.index) regex.lastIndex++;
      }
      return list;
    }

    function renderEnumPillSummary(lengthStr) {
      const opts = parseEnumOptionList(lengthStr);
      if (opts.length === 0) return 'Set values...';
      if (opts.length <= 2) return opts.map(o => `'${escapeHtml(o)}'`).join(',');
      return `${opts.length} values ('${escapeHtml(opts[0])}',...)`;
    }

    function isLengthlessType(base) {
      const u = (base || '').toUpperCase();
      return ['INT', 'BIGINT', 'TINYINT', 'SMALLINT', 'MEDIUMINT', 'TEXT', 'LONGTEXT', 'MEDIUMTEXT', 'TINYTEXT', 'DATETIME', 'DATE', 'TIMESTAMP', 'TIME', 'JSON', 'BOOLEAN', 'BLOB', 'LONGBLOB'].includes(u);
    }

    function isIntegerType(base) {
      const u = (base || '').toUpperCase();
      return ['INT', 'BIGINT', 'TINYINT', 'SMALLINT', 'MEDIUMINT'].includes(u);
    }

    function formatColumnType(base, length) {
      base = (base || 'VARCHAR').toUpperCase();
      if (isLengthlessType(base) || !length) {
        return base;
      }
      return `${base}(${length})`;
    }

    function detectDefaultPreset(val) {
      if (val === null || val === undefined || val === '') return 'none';
      const u = String(val).trim().toUpperCase();
      if (u === 'NULL') return 'NULL';
      if (u === 'CURRENT_TIMESTAMP' || u === 'NOW()') return 'CURRENT_TIMESTAMP';
      if (u === '0') return '0';
      if (val === "''" || val === '""') return 'empty_string';
      return 'custom';
    }

    function formatDefaultClause(val, baseType) {
      if (val === null || val === undefined || val === '' || val === 'none') return '';
      const u = String(val).trim();
      if (u.toUpperCase() === 'NULL') return 'DEFAULT NULL';
      if (u.toUpperCase() === 'CURRENT_TIMESTAMP' || u.toUpperCase() === 'NOW()') return 'DEFAULT CURRENT_TIMESTAMP';
      if (u === "''" || u === '""') return "DEFAULT ''";
      const upperBase = (baseType || '').toUpperCase();
      if (isIntegerType(upperBase) || ['DECIMAL', 'FLOAT', 'DOUBLE'].includes(upperBase)) {
        if (!isNaN(Number(u))) return `DEFAULT ${u}`;
      }
      if ((u.startsWith("'") && u.endsWith("'")) || (u.startsWith('"') && u.endsWith('"')) || u.includes('(')) {
        return `DEFAULT ${u}`;
      }
      return `DEFAULT '${u.replace(/'/g, "''")}'`;
    }

    function renderDefaultPresetSelect(c, idx, context) {
      const parsed = parseColumnType(c.type);
      const isEnum = parsed.base === 'ENUM' || parsed.base === 'SET';
      const enumOpts = isEnum ? parseEnumOptionList(parsed.length) : [];
      const preset = detectDefaultPreset(c.default_value);
      const isCustomPreset = preset === 'custom';
      const cleanDefault = (c.default_value || '').replace(/^'(.*)'$/, '$1');
      const isEnumPreset = enumOpts.includes(cleanDefault);

      const onChangeFn = context === 'staged' ? `updateStagedColDefaultPreset(${idx}, this.value)` : `updateCreatorColDefaultPreset(${idx}, this.value)`;
      const onInputFn = context === 'staged' ? `updateStagedCol(${idx}, 'default_value', this.value)` : `newTableColumns[${idx}].default_value = this.value; validateCreatorColumns();`;
      const bgClass = context === 'staged' ? 'bg-slate-950' : 'bg-slate-900';

      let enumOptHtml = '';
      if (enumOpts.length > 0) {
        enumOptHtml = `
          <optgroup label="Enum Options">
            ${enumOpts.map(opt => `<option value="__ENUM__:${escapeHtml(opt)}" ${(cleanDefault === opt) ? 'selected' : ''}>'${escapeHtml(opt)}'</option>`).join('')}
          </optgroup>
        `;
      }

      return `
        <div class="flex items-center space-x-1">
          <select onchange="${onChangeFn}" class="${bgClass} border border-slate-700 text-slate-300 text-xs py-1 px-1 rounded outline-none font-mono focus:border-app-accent max-w-[125px]">
            <option value="none" ${preset === 'none' && !isEnumPreset ? 'selected' : ''}>None</option>
            <option value="NULL" ${preset === 'NULL' ? 'selected' : ''} ${c.is_primary_key ? 'disabled' : ''}>NULL</option>
            <option value="CURRENT_TIMESTAMP" ${preset === 'CURRENT_TIMESTAMP' ? 'selected' : ''}>CURRENT_TIMESTAMP</option>
            <option value="0" ${preset === '0' && !isEnumPreset ? 'selected' : ''}>0</option>
            <option value="empty_string" ${preset === 'empty_string' ? 'selected' : ''}>'' (Empty)</option>
            ${enumOptHtml}
            <option value="custom" ${isCustomPreset && !isEnumPreset ? 'selected' : ''}>Custom...</option>
          </select>
          <input type="text" value="${(isCustomPreset && !isEnumPreset) ? escapeHtml(c.default_value || '') : ''}" placeholder="Expr/Val" oninput="${onInputFn}" class="${(isCustomPreset && !isEnumPreset) ? '' : 'hidden'} w-24 ${bgClass} border border-slate-700 focus:border-app-accent px-1.5 py-1 rounded text-slate-200 font-mono text-xs outline-none">
        </div>
      `;
    }

    function renderTypeSelectOptions(currentBase) {
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const upper = (currentBase || 'VARCHAR').toUpperCase();
      let hasCurrent = false;
      let html = '';
      STANDARD_DATA_TYPES.forEach(group => {
        if (isSQLite && group.group === 'Selection / Enum') return;
        html += `<optgroup label="${group.group}">`;
        group.types.forEach(t => {
          if (t === upper) hasCurrent = true;
          html += `<option value="${t}" ${t === upper ? 'selected' : ''}>${t}</option>`;
        });
        html += `</optgroup>`;
      });
      if (!hasCurrent && upper) {
        html = `<option value="${escapeHtml(upper)}" selected>${escapeHtml(upper)}</option>` + html;
      }
      return html;
    }

    function validateTableColumns(columns, isCreator) {
      const errors = [];
      const nameCounts = new Map();

      (columns || []).forEach((c, idx) => {
        if (c._deleted) return;
        const name = (c.name || '').trim();
        const rowLabel = name ? `'${name}'` : `Row ${idx + 1}`;
        if (!name) {
          errors.push(`Row ${idx + 1}: Column name cannot be empty.`);
        } else {
          const lower = name.toLowerCase();
          nameCounts.set(lower, (nameCounts.get(lower) || 0) + 1);
          if (nameCounts.get(lower) > 1) {
            errors.push(`Duplicate column name "${name}".`);
          }
        }

        const { base, length } = parseColumnType(c.type);
        if (base === 'ENUM' || base === 'SET') {
          const opts = parseEnumOptionList(length);
          if (opts.length === 0) {
            errors.push(`Column ${rowLabel} (${base}): At least 1 option value must be defined.`);
          }
        } else if (base === 'VARCHAR' || base === 'CHAR') {
          if (!length || !/^\d+$/.test(length.trim()) || parseInt(length, 10) <= 0) {
            errors.push(`Column ${rowLabel} (${base}): Requires a positive numeric length (e.g. 255).`);
          }
        }
      });

      return errors;
    }

    function renderStructureRows() {
      const tbody = document.getElementById('structure-tbody');
      if (!stagedStructure || !stagedStructure.columns) return;

      tbody.innerHTML = stagedStructure.columns.map((c, i) => {
        if (c._deleted) {
          return `
            <tr class="bg-red-950/20 text-slate-500 line-through opacity-60">
              <td class="p-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${i + 1}</td>
              <td class="p-2 text-center">${c.is_primary_key ? renderIcon('key', 'w-3.5 h-3.5 text-amber-400 inline shrink-0') : ''}</td>
              <td class="p-2 font-semibold">${escapeHtml(c.name)}</td>
              <td class="p-2">${escapeHtml(c.type)}</td>
              <td class="p-2 text-center">${c.is_nullable ? 'YES' : 'NO'}</td>
              <td class="p-2">${escapeHtml(c.default_value || '')}</td>
              <td class="p-2 text-center">${c.extra || ''}</td>
              <td class="p-2 text-center">
                <button onclick="restoreStructureCol(${i})" class="text-xs text-sky-400 hover:underline">Restore</button>
              </td>
            </tr>
          `;
        }

        const isNew = c._isNew;
        const rowBg = isNew ? 'bg-emerald-950/30' : (c._modified ? 'bg-amber-950/30' : 'hover:bg-slate-800/40');
        const { base, length } = parseColumnType(c.type);
        const isLenDisabled = isLengthlessType(base);
        const isInt = isIntegerType(base);
        const isEnum = base === 'ENUM' || base === 'SET';

        const typeLengthHtml = isEnum ? `
          <button type="button" onclick="openEnumValuesPopover(event, 'staged', ${i})" class="flex items-center space-x-1.5 bg-slate-950 border border-slate-700 hover:border-app-accent px-2 py-1 rounded text-sky-300 font-mono text-xs max-w-[150px] truncate group transition shadow-inner" title="Click to edit ${base} values">
            <span class="group-hover:scale-110 transition-transform">${renderIcon('bookmark', 'w-3.5 h-3.5 text-sky-400')}</span>
            <span id="staged-enum-pill-${i}" class="truncate">${renderEnumPillSummary(length)}</span>
          </button>
        ` : `
          <input type="text" value="${escapeHtml(length)}" ${isLenDisabled ? 'disabled placeholder="N/A"' : 'placeholder="Len"'} oninput="updateStagedColLength(${i}, this.value)" class="w-16 bg-slate-950 border border-slate-700 text-white text-xs py-1 px-1 rounded outline-none font-mono ${isLenDisabled ? 'opacity-30 cursor-not-allowed bg-slate-900' : 'focus:border-app-accent'}">
        `;

        return `
          <tr class="${rowBg} transition border-b border-slate-800/40">
            <td class="p-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${i + 1}</td>
            <td class="p-2 text-center">
              <button onclick="toggleColPK(${i})" class="p-1 rounded hover:bg-slate-700 ${c.is_primary_key ? 'text-amber-400 font-bold' : 'text-slate-600'}" title="Toggle Primary Key">
                ${renderIcon('key', 'w-3.5 h-3.5 text-amber-400')}
              </button>
            </td>
            <td class="p-2">
              <input type="text" value="${escapeHtml(c.name)}" oninput="updateStagedCol(${i}, 'name', this.value)" class="w-full bg-slate-950 border border-slate-700 focus:border-app-accent px-1.5 py-1 rounded text-white font-mono text-xs outline-none">
            </td>
            <td class="p-2">
              <div class="flex items-center space-x-1">
                <select onchange="updateStagedColTypeBase(${i}, this.value)" class="bg-slate-950 border border-slate-700 text-sky-200 text-xs py-1 px-1 rounded outline-none font-mono focus:border-app-accent">
                  ${renderTypeSelectOptions(base)}
                </select>
                ${typeLengthHtml}
              </div>
            </td>
            <td class="p-2 text-center">
              <input type="checkbox" ${c.is_nullable ? 'checked' : ''} ${c.is_primary_key ? 'disabled title="Primary Key cannot be nullable"' : ''} onchange="updateStagedCol(${i}, 'is_nullable', this.checked)" class="rounded bg-slate-900 border-slate-700 text-app-accent ${c.is_primary_key ? 'opacity-30 cursor-not-allowed' : ''}">
            </td>
            <td class="p-2">
              ${renderDefaultPresetSelect(c, i, 'staged')}
            </td>
            <td class="p-2 text-center">
              <input type="checkbox" ${(c.extra || '').toLowerCase().includes('auto_increment') ? 'checked' : ''} ${!isInt ? 'disabled title="Auto increment only supported on integer types"' : ''} onchange="updateStagedCol(${i}, 'auto_increment', this.checked)" class="rounded bg-slate-900 border-slate-700 text-app-accent ${!isInt ? 'opacity-30 cursor-not-allowed' : ''}">
            </td>
            <td class="p-2 text-center">
              <button onclick="removeStructureCol(${i})" class="text-slate-500 hover:text-red-400 p-1" title="Delete Column">${renderIcon('trash-2', 'w-3.5 h-3.5')}</button>
            </td>
          </tr>
        `;
      }).join('');

      updateStructureSaveState();
    }

    function addStructureColumn() {
      if (!stagedStructure) return;
      stagedStructure.columns.push({
        name: 'new_col_' + (stagedStructure.columns.length + 1),
        type: 'VARCHAR(255)',
        is_nullable: true,
        is_primary_key: false,
        default_value: '',
        extra: '',
        _isNew: true
      });
      renderStructureRows();
    }

    // ponytail: In-place DOM updates preserves focus and caret position during continuous editing
    function updateStagedColTypeBase(idx, newBase) {
      if (!stagedStructure || !stagedStructure.columns[idx]) return;
      const col = stagedStructure.columns[idx];
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
      if (!col._isNew) col._modified = true;
      renderStructureRows();
    }

    function updateStagedColLength(idx, newLength) {
      if (!stagedStructure || !stagedStructure.columns[idx]) return;
      const col = stagedStructure.columns[idx];
      const parsed = parseColumnType(col.type);
      col.type = formatColumnType(parsed.base, newLength);
      if (!col._isNew) col._modified = true;
      updateStructureSaveState();
    }

    function updateStagedColDefaultPreset(idx, preset) {
      if (!stagedStructure || !stagedStructure.columns[idx]) return;
      const col = stagedStructure.columns[idx];
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
      if (!col._isNew) col._modified = true;
      renderStructureRows();
    }

    function updateStagedCol(idx, field, val) {
      if (!stagedStructure || !stagedStructure.columns[idx]) return;
      const col = stagedStructure.columns[idx];
      if (field === 'auto_increment') {
        const { base } = parseColumnType(col.type);
        if (isIntegerType(base)) {
          col.extra = val ? 'auto_increment' : '';
        } else {
          col.extra = '';
        }
      } else if (field === 'is_nullable') {
        if (!col.is_primary_key) {
          col.is_nullable = val;
          if (!val && col.default_value === 'NULL') {
            col.default_value = '';
          }
        }
      } else {
        col[field] = val;
      }
      if (!col._isNew) col._modified = true;
      updateStructureSaveState();
    }

    function toggleColPK(idx) {
      if (!stagedStructure || !stagedStructure.columns[idx]) return;
      const col = stagedStructure.columns[idx];
      col.is_primary_key = !col.is_primary_key;
      if (col.is_primary_key) {
        col.is_nullable = false;
        if (col.default_value === 'NULL') col.default_value = '';
      }
      if (!col._isNew) col._modified = true;
      renderStructureRows();
    }

    function removeStructureCol(idx) {
      if (!stagedStructure || !stagedStructure.columns[idx]) return;
      const col = stagedStructure.columns[idx];
      if (col._isNew) {
        stagedStructure.columns.splice(idx, 1);
      } else {
        col._deleted = true;
      }
      renderStructureRows();
    }

    function restoreStructureCol(idx) {
      if (!stagedStructure || !stagedStructure.columns[idx]) return;
      stagedStructure.columns[idx]._deleted = false;
      renderStructureRows();
    }

    function discardStructureChanges() {
      if (!currentTableMetadata) return;
      stagedStructure = JSON.parse(JSON.stringify(currentTableMetadata));
      renderStructureRows();
    }

    function getStructureDiffCount() {
      if (!stagedStructure || !currentTableMetadata) return 0;
      let diffCount = 0;
      stagedStructure.columns.forEach(c => {
        if (c._isNew || c._modified || c._deleted) diffCount++;
      });
      return diffCount;
    }

    function updateStructureSaveState() {
      if (!stagedStructure) return;
      const diffCount = getStructureDiffCount();
      const errors = validateTableColumns(stagedStructure.columns, false);
      const saveBtn = document.getElementById('btn-save-structure');
      const discardBtn = document.getElementById('btn-discard-structure');
      const diffBadge = document.getElementById('structure-diff-badge');

      if (diffCount > 0) {
        discardBtn.classList.remove('hidden');
        diffBadge.classList.remove('hidden');
        if (errors.length > 0) {
          saveBtn.disabled = true;
          saveBtn.className = 'bg-slate-800 text-amber-400 border border-amber-600/60 font-bold px-3 py-1 rounded transition cursor-not-allowed flex items-center space-x-1';
          diffBadge.className = 'bg-red-950 text-red-300 border border-red-600 px-1.5 rounded text-[10px] font-bold';
          diffBadge.textContent = `⚠️ ${errors.length} issue${errors.length > 1 ? 's' : ''}`;
          saveBtn.title = errors.join('\n');
        } else {
          saveBtn.disabled = false;
          saveBtn.className = 'bg-app-safe hover:bg-emerald-600 text-slate-950 font-bold px-3 py-1 rounded transition cursor-pointer flex items-center space-x-1 shadow-sm';
          diffBadge.className = 'bg-emerald-950 text-emerald-300 border border-emerald-600 px-1 rounded text-[10px]';
          diffBadge.textContent = diffCount;
          saveBtn.title = 'Save changes to database structure';
        }
      } else {
        saveBtn.disabled = true;
        saveBtn.className = 'bg-slate-800 text-slate-500 font-bold px-3 py-1 rounded transition cursor-not-allowed flex items-center space-x-1';
        discardBtn.classList.add('hidden');
        diffBadge.classList.add('hidden');
        saveBtn.title = '';
      }
    }

    // --- Floating ENUM / SET Value Builder Popover Handlers ---
    let activeEnumBuilderState = null; // { context: 'staged'|'creator', colIdx: number, base: 'ENUM'|'SET', tags: string[] }

    function openEnumValuesPopover(event, context, colIdx) {
      if (event) event.stopPropagation();
      const col = context === 'staged' ? stagedStructure?.columns[colIdx] : newTableColumns[colIdx];
      if (!col) return;

      const { base, length } = parseColumnType(col.type);
      const initialTags = parseEnumOptionList(length);
      activeEnumBuilderState = {
        context,
        colIdx,
        base: (base === 'SET') ? 'SET' : 'ENUM',
        tags: initialTags.length > 0 ? initialTags : ['option1', 'option2']
      };

      const popover = document.getElementById('enum-builder-popover');
      const titleEl = document.getElementById('enum-popover-title');
      const colEl = document.getElementById('enum-popover-col');
      const inputEl = document.getElementById('enum-quick-input');
      const warningEl = document.getElementById('enum-builder-warning');

      if (titleEl) titleEl.textContent = `Edit ${activeEnumBuilderState.base} Values`;
      if (colEl) colEl.textContent = col.name || `Col #${colIdx + 1}`;
      if (inputEl) inputEl.value = '';
      if (warningEl) warningEl.classList.add('hidden');

      renderEnumBuilderTags();

      // Position popover relative to trigger button
      popover.classList.remove('hidden');
      const rect = event ? event.currentTarget.getBoundingClientRect() : { left: 100, top: 100, bottom: 130 };
      const popoverWidth = 320;
      let left = rect.left;
      if (left + popoverWidth > window.innerWidth - 16) {
        left = window.innerWidth - popoverWidth - 16;
      }
      if (left < 16) left = 16;

      let top = rect.bottom + 6;
      if (top + 280 > window.innerHeight) {
        top = Math.max(16, rect.top - 280);
      }

      popover.style.left = `${left}px`;
      popover.style.top = `${top}px`;

      initEnumQuickInputListeners();
      setTimeout(() => {
        if (inputEl) inputEl.focus();
      }, 50);
    }

    function renderEnumBuilderTags() {
      if (!activeEnumBuilderState) return;
      const listEl = document.getElementById('enum-tags-list');
      const countEl = document.getElementById('enum-tag-count');
      if (countEl) countEl.textContent = activeEnumBuilderState.tags.length;

      if (!listEl) return;
      if (activeEnumBuilderState.tags.length === 0) {
        listEl.innerHTML = `<span class="text-slate-500 italic text-[11px] p-1">No options yet. Type below and press Enter.</span>`;
      } else {
        listEl.innerHTML = activeEnumBuilderState.tags.map((t, idx) => `
          <span class="inline-flex items-center space-x-1 bg-sky-950/80 text-sky-200 border border-sky-800/80 px-2 py-0.5 rounded font-mono text-[11px]">
            <span>${escapeHtml(t)}</span>
            <button type="button" onclick="removeEnumBuilderTag(${idx})" class="text-sky-400 hover:text-red-300 ml-1 font-bold"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
          </span>
        `).join('');
      }

      updateEnumBuilderPreview();
    }

    function addEnumBuilderTag(rawVal) {
      if (!activeEnumBuilderState) return;
      let cleaned = String(rawVal || '').trim();
      cleaned = cleaned.replace(/^['"]+|['"]+$/g, '').trim();
      if (!cleaned) return;

      const warningEl = document.getElementById('enum-builder-warning');
      const warningText = document.getElementById('enum-warning-text');

      if (activeEnumBuilderState.tags.includes(cleaned)) {
        if (warningEl && warningText) {
          warningText.textContent = `Option "${cleaned}" already exists.`;
          warningEl.classList.remove('hidden');
        }
        return;
      }

      if (warningEl) warningEl.classList.add('hidden');
      activeEnumBuilderState.tags.push(cleaned);
      renderEnumBuilderTags();
    }

    function removeEnumBuilderTag(idx) {
      if (!activeEnumBuilderState) return;
      activeEnumBuilderState.tags.splice(idx, 1);
      renderEnumBuilderTags();
    }

    function updateEnumBuilderPreview() {
      if (!activeEnumBuilderState) return;
      const previewEl = document.getElementById('enum-builder-sql-preview');
      const warningEl = document.getElementById('enum-builder-warning');
      const warningText = document.getElementById('enum-warning-text');

      const formatted = activeEnumBuilderState.tags.map(t => `'${t.replace(/'/g, "''")}'`).join(', ');
      const sqlPreview = `${activeEnumBuilderState.base}(${formatted})`;
      if (previewEl) previewEl.textContent = sqlPreview;

      if (activeEnumBuilderState.tags.length === 0) {
        if (warningEl && warningText) {
          warningText.textContent = 'At least 1 option is required.';
          warningEl.classList.remove('hidden');
        }
      } else if (warningEl && warningText && warningText.textContent.includes('At least 1')) {
        warningEl.classList.add('hidden');
      }
    }

    function applyEnumBuilder() {
      if (!activeEnumBuilderState) return;
      if (activeEnumBuilderState.tags.length === 0) {
        showNativeAlert('Please define at least one option for the ' + activeEnumBuilderState.base + ' type.', 'Enum Validation');
        return;
      }

      const formatted = activeEnumBuilderState.tags.map(t => `'${t.replace(/'/g, "''")}'`).join(', ');
      const newType = `${activeEnumBuilderState.base}(${formatted})`;
      const { context, colIdx } = activeEnumBuilderState;

      if (context === 'staged' && stagedStructure && stagedStructure.columns[colIdx]) {
        const col = stagedStructure.columns[colIdx];
        col.type = newType;
        if (!col._isNew) col._modified = true;
        renderStructureRows();
      } else if (context === 'creator' && newTableColumns[colIdx]) {
        const col = newTableColumns[colIdx];
        col.type = newType;
        renderCreatorColumns();
      }

      closeEnumBuilderPopover();
    }

    function closeEnumBuilderPopover() {
      activeEnumBuilderState = null;
      const popover = document.getElementById('enum-builder-popover');
      if (popover) popover.classList.add('hidden');
    }

    let enumListenersBound = false;
    function initEnumQuickInputListeners() {
      if (enumListenersBound) return;
      const input = document.getElementById('enum-quick-input');
      if (!input) return;
      enumListenersBound = true;

      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ',') {
          e.preventDefault();
          const val = input.value;
          if (val) {
            addEnumBuilderTag(val);
            input.value = '';
          }
        } else if (e.key === 'Escape') {
          closeEnumBuilderPopover();
        }
      });

      input.addEventListener('paste', (e) => {
        e.preventDefault();
        const text = (e.clipboardData || window.clipboardData).getData('text');
        if (text) {
          const list = parseEnumOptionList(text);
          if (list.length > 0) {
            list.forEach(item => addEnumBuilderTag(item));
          } else {
            addEnumBuilderTag(text);
          }
          input.value = '';
        }
      });
    }

    function saveStructureChanges() {
      if (!stagedStructure || !currentTableMetadata) return;
      const errors = validateTableColumns(stagedStructure.columns, false);
      if (errors.length > 0) {
        showNativeAlert('Cannot save table structure due to validation errors:\n• ' + errors.join('\n• '), 'Structure Validation');
        return;
      }
      const tableName = stagedStructure.name;
      const originalCols = currentTableMetadata.columns || [];
      const ddlStatements = [];
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');

      // 1. Column drops
      stagedStructure.columns.filter(c => c._deleted).forEach(c => {
        if (isSQLite) {
          ddlStatements.push(`ALTER TABLE "${tableName}" DROP COLUMN "${c.name}";`);
        } else {
          ddlStatements.push(`ALTER TABLE \`${tableName}\` DROP COLUMN \`${c.name}\`;`);
        }
      });

      // 2. New columns
      stagedStructure.columns.filter(c => c._isNew && !c._deleted).forEach(c => {
        const { base } = parseColumnType(c.type);
        const nullClause = c.is_nullable ? 'NULL' : 'NOT NULL';
        const dfltClause = formatDefaultClause(c.default_value, base);
        const extraClause = c.extra || '';
        if (isSQLite) {
          ddlStatements.push(`ALTER TABLE "${tableName}" ADD COLUMN "${c.name}" ${c.type} ${nullClause} ${dfltClause} ${extraClause};`.replace(/\s+/g, ' ').trim());
        } else {
          ddlStatements.push(`ALTER TABLE \`${tableName}\` ADD COLUMN \`${c.name}\` ${c.type} ${nullClause} ${dfltClause} ${extraClause};`.replace(/\s+/g, ' ').trim());
        }
      });

      // 3. Modified columns (MySQL natively supported)
      stagedStructure.columns.filter(c => c._modified && !c._isNew && !c._deleted).forEach(c => {
        const { base } = parseColumnType(c.type);
        const nullClause = c.is_nullable ? 'NULL' : 'NOT NULL';
        const dfltClause = formatDefaultClause(c.default_value, base);
        const extraClause = c.extra || '';
        if (isSQLite) {
          ddlStatements.push(`-- SQLite: Column modified "${c.name}" (${c.type})`);
        } else {
          ddlStatements.push(`ALTER TABLE \`${tableName}\` MODIFY COLUMN \`${c.name}\` ${c.type} ${nullClause} ${dfltClause} ${extraClause};`.replace(/\s+/g, ' ').trim());
        }
      });

      // 4. Primary Key modifications
      const origPKs = originalCols.filter(c => c.is_primary_key).map(c => c.name).sort().join(',');
      const currentPKCols = stagedStructure.columns.filter(c => c.is_primary_key && !c._deleted).map(c => c.name);
      const newPKs = currentPKCols.slice().sort().join(',');
      if (origPKs !== newPKs && !isSQLite) {
        if (origPKs) {
          ddlStatements.push(`ALTER TABLE \`${tableName}\` DROP PRIMARY KEY;`);
        }
        if (currentPKCols.length > 0) {
          ddlStatements.push(`ALTER TABLE \`${tableName}\` ADD PRIMARY KEY (\`${currentPKCols.join('`, `')}\`);`);
        }
      }

      if (ddlStatements.length === 0) {
        showNativeAlert('No structural modifications detected to save.', 'Structure Designer');
        return;
      }

      const fullDDL = ddlStatements.join('\n');
      openDDLReviewModal(fullDDL, async () => {
        const ok = await executeDDLStatements(fullDDL, `Table "${tableName}" structure updated successfully.`);
        if (ok) {
          stagedStructure = null;
          await loadSchemaTree(activeConnection.id);
          renderTableStructure(tableName);
        }
      });
    }

    // --- Indexes & Foreign Keys View Handlers ---
    let selectedIndexCols = [];
    let editingIndexOldName = null;

    function renderTableIndexes(tableName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetCatalog = (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => t.name === tableName);
      const tbody = document.getElementById('indexes-tbody');
      if (!tableMeta || !tableMeta.indexes || tableMeta.indexes.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="p-4 text-center text-slate-500">No custom indexes on this table.</td></tr>';
        return;
      }
      tbody.innerHTML = tableMeta.indexes.map((idx, i) => `
        <tr class="hover:bg-slate-800/40 border-b border-slate-800/40">
          <td class="p-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${i + 1}</td>
          <td class="p-2 font-mono font-semibold text-sky-200">${escapeHtml(idx.name)}</td>
          <td class="p-2 font-mono text-slate-300">${escapeHtml((idx.columns || []).join(', '))}</td>
          <td class="p-2 text-center">${idx.is_unique ? '<span class="text-emerald-400 font-bold">UNIQUE</span>' : '<span class="text-slate-500">INDEX</span>'}</td>
          <td class="p-2 font-mono text-slate-400">${escapeHtml(idx.type || 'BTREE')}</td>
          <td class="p-2 text-center">
            ${idx.name === 'PRIMARY' ? '<span class="text-slate-600 text-xs font-bold">PK</span>' : `
              <div class="flex items-center justify-center space-x-1">
                <button type="button" data-index="${escapeHtml(idx.name)}" onclick="editTableIndex(this.dataset.index)" class="text-slate-400 hover:text-sky-300 p-1 flex items-center" title="Edit Index">${renderIcon('edit', 'w-3.5 h-3.5')}</button>
                <button type="button" data-index="${escapeHtml(idx.name)}" onclick="dropTableIndex(this.dataset.index)" class="text-slate-500 hover:text-red-400 p-1" title="Drop Index">${renderIcon('trash-2', 'w-3.5 h-3.5')}</button>
              </div>
            `}
          </td>
        </tr>
      `).join('');
    }

    function openAddIndexForm(editIdxName = null) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const targetCatalog = (activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => t.name === tableName);
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');

      editingIndexOldName = editIdxName;
      selectedIndexCols = [];

      const form = document.getElementById('add-index-inline-form');
      const titleEl = document.getElementById('index-form-title');
      const badgeEl = document.getElementById('index-form-mode-badge');
      const nameInput = document.getElementById('new-index-name');
      const typeSelect = document.getElementById('new-index-type');

      // Adjust type select for SQLite
      if (typeSelect) {
        if (isSQLite) {
          typeSelect.innerHTML = `
            <option value="INDEX">INDEX (Normal)</option>
            <option value="UNIQUE">UNIQUE</option>
          `;
        } else {
          typeSelect.innerHTML = `
            <option value="INDEX">INDEX (Normal BTREE)</option>
            <option value="UNIQUE">UNIQUE</option>
            <option value="FULLTEXT">FULLTEXT</option>
            <option value="SPATIAL">SPATIAL</option>
          `;
        }
      }

      if (editingIndexOldName) {
        const existing = (tableMeta?.indexes || []).find(idx => idx.name === editingIndexOldName);
        if (existing) {
          selectedIndexCols = Array.isArray(existing.columns) ? [...existing.columns] : [];
          if (nameInput) nameInput.value = existing.name;
          if (typeSelect) {
            if (existing.is_unique) typeSelect.value = 'UNIQUE';
            else if (existing.type && ['FULLTEXT', 'SPATIAL'].includes(existing.type.toUpperCase())) typeSelect.value = existing.type.toUpperCase();
            else typeSelect.value = 'INDEX';
          }
        }
        if (titleEl) titleEl.textContent = `Edit Index: ${editingIndexOldName}`;
        if (badgeEl) badgeEl.classList.remove('hidden');
      } else {
        if (titleEl) titleEl.textContent = 'Add New Index';
        if (badgeEl) badgeEl.classList.add('hidden');
        if (nameInput) nameInput.value = '';
        if (typeSelect) typeSelect.value = 'INDEX';
      }

      renderIndexColumnPickers(tableMeta?.columns || []);
      form.classList.remove('hidden');
    }

    function editTableIndex(idxName) {
      openAddIndexForm(idxName);
    }

    function renderIndexColumnPickers(availableCols) {
      const selectedContainer = document.getElementById('index-selected-chips');
      const availableContainer = document.getElementById('index-available-chips');
      const hiddenInput = document.getElementById('new-index-cols');

      if (hiddenInput) hiddenInput.value = selectedIndexCols.join(', ');

      if (selectedContainer) {
        if (selectedIndexCols.length === 0) {
          selectedContainer.innerHTML = '<span class="text-slate-500 italic text-[11px] px-1">Click column pills below to add in order...</span>';
        } else {
          selectedContainer.innerHTML = selectedIndexCols.map((c, i) => `
            <span class="inline-flex items-center space-x-1 bg-sky-950 border border-sky-700 text-sky-200 px-2 py-0.5 rounded font-mono text-[11px] shadow-sm">
              <span class="text-slate-400 font-sans text-[10px]">#${i + 1}</span>
              <span class="font-bold">${escapeHtml(c)}</span>
              <button type="button" onclick="removeIndexCol(${jsArg(c)})" class="text-sky-400 hover:text-red-300 ml-1"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
            </span>
          `).join('');
        }
      }

      if (availableContainer) {
        availableContainer.innerHTML = availableCols.map(col => {
          const isSelected = selectedIndexCols.includes(col.name);
          return `
            <button type="button" onclick="toggleIndexCol(${jsArg(col.name)})" class="px-2 py-1 rounded font-mono text-xs transition border flex items-center space-x-1 ${isSelected ? 'bg-sky-900/60 border-sky-500 text-sky-200 shadow-sm' : 'bg-slate-900 border-slate-700 text-slate-300 hover:border-slate-500 hover:text-white'}">
              <span>${isSelected ? '✓' : '+'}</span>
              <span>${escapeHtml(col.name)}</span>
              <span class="text-[10px] text-slate-500 ml-1">${escapeHtml(col.type)}</span>
            </button>
          `;
        }).join('');
      }

      updateIndexPreview();
    }

    function toggleIndexCol(colName) {
      if (selectedIndexCols.includes(colName)) {
        selectedIndexCols = selectedIndexCols.filter(c => c !== colName);
      } else {
        selectedIndexCols.push(colName);
      }
      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetCatalog = (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => t.name === activeTab?.tableName);
      renderIndexColumnPickers(tableMeta?.columns || []);
      if (!document.getElementById('new-index-name').value.trim()) {
        autoGenerateIndexName();
      }
    }

    function removeIndexCol(colName) {
      toggleIndexCol(colName);
    }

    function onIndexTypeChange() {
      updateIndexPreview();
      autoGenerateIndexName();
    }

    function autoGenerateIndexName() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const type = document.getElementById('new-index-type')?.value || 'INDEX';
      const prefix = type === 'UNIQUE' ? 'uniq' : (type === 'FULLTEXT' ? 'ft' : (type === 'SPATIAL' ? 'sp' : 'idx'));
      const colSlug = selectedIndexCols.slice(0, 3).join('_') || 'cols';
      const nameInput = document.getElementById('new-index-name');
      if (nameInput) nameInput.value = `${prefix}_${tableName}_${colSlug}`;
      updateIndexPreview();
    }

    function updateIndexPreview() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const tableName = activeTab ? activeTab.tableName : 'table';
      const previewEl = document.getElementById('index-form-sql-preview');
      const name = document.getElementById('new-index-name')?.value.trim() || 'idx_name';
      const type = document.getElementById('new-index-type')?.value || 'INDEX';
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');

      if (!previewEl) return;
      if (selectedIndexCols.length === 0) {
        previewEl.textContent = '-- Select columns to build index definition';
        return;
      }

      const formattedCols = isSQLite ? selectedIndexCols.map(c => `"${c}"`).join(', ') : selectedIndexCols.map(c => `\`${c}\``).join(', ');
      const typeClause = type === 'UNIQUE' ? 'UNIQUE ' : (type === 'FULLTEXT' ? 'FULLTEXT ' : (type === 'SPATIAL' ? 'SPATIAL ' : ''));
      const quote = isSQLite ? '"' : '`';
      previewEl.textContent = `CREATE ${typeClause}INDEX ${quote}${name}${quote} ON ${quote}${tableName}${quote} (${formattedCols});`;
    }

    async function submitNewIndex() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const name = document.getElementById('new-index-name').value.trim();
      const type = document.getElementById('new-index-type')?.value || 'INDEX';

      if (!name) {
        showNativeAlert('Please specify an index name.', 'Index Builder');
        return;
      }
      if (selectedIndexCols.length === 0) {
        showNativeAlert('Please select at least one column for the index.', 'Index Builder');
        return;
      }

      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const quote = isSQLite ? '"' : '`';
      const formattedCols = isSQLite ? selectedIndexCols.map(c => `"${c}"`).join(', ') : selectedIndexCols.map(c => `\`${c}\``).join(', ');
      const typeClause = type === 'UNIQUE' ? 'UNIQUE ' : (type === 'FULLTEXT' ? 'FULLTEXT ' : (type === 'SPATIAL' ? 'SPATIAL ' : ''));

      const ddlStatements = [];
      if (editingIndexOldName) {
        if (isSQLite) {
          ddlStatements.push(`DROP INDEX "${editingIndexOldName}";`);
        } else {
          ddlStatements.push(`ALTER TABLE \`${tableName}\` DROP INDEX \`${editingIndexOldName}\`;`);
        }
      }

      ddlStatements.push(`CREATE ${typeClause}INDEX ${quote}${name}${quote} ON ${quote}${tableName}${quote} (${formattedCols});`);
      const fullDDL = ddlStatements.join('\n');

      document.getElementById('add-index-inline-form').classList.add('hidden');
      openDDLReviewModal(fullDDL, async () => {
        const ok = await executeDDLStatements(fullDDL, editingIndexOldName ? `Index "${name}" updated successfully.` : `Index "${name}" created successfully.`);
        if (ok) {
          editingIndexOldName = null;
          await loadSchemaTree(activeConnection.id);
          renderTableIndexes(tableName);
        }
      });
    }

    async function dropTableIndex(idxName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');
      const sql = isSQLite
        ? `DROP INDEX "${idxName}";`
        : `ALTER TABLE \`${tableName}\` DROP INDEX \`${idxName}\`;`;

      openDDLReviewModal(sql, async () => {
        const ok = await executeDDLStatements(sql, `Index "${idxName}" dropped successfully.`);
        if (ok) {
          await loadSchemaTree(activeConnection.id);
          renderTableIndexes(tableName);
        }
      });
    }

    function renderTableFKs(tableName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetCatalog = (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => t.name === tableName);
      const tbody = document.getElementById('foreign-keys-tbody');
      if (!tableMeta || !tableMeta.foreign_keys || tableMeta.foreign_keys.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="p-4 text-center text-slate-500">No foreign keys configured on this table.</td></tr>';
        return;
      }
      tbody.innerHTML = tableMeta.foreign_keys.map((fk, i) => `
        <tr class="hover:bg-slate-800/40 border-b border-slate-800/40">
          <td class="p-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${i + 1}</td>
          <td class="p-2 font-mono font-semibold text-sky-200">${escapeHtml(fk.name)}</td>
          <td class="p-2 font-mono text-slate-300">${escapeHtml(fk.column)}</td>
          <td class="p-2 font-mono text-emerald-300">${escapeHtml(fk.referenced_table)} (${escapeHtml(fk.referenced_column)})</td>
          <td class="p-2 font-mono text-xs text-slate-400">${escapeHtml(fk.on_delete || 'NO ACTION')}</td>
          <td class="p-2 font-mono text-xs text-slate-400">${escapeHtml(fk.on_update || 'NO ACTION')}</td>
          <td class="p-2 text-center">
            <button type="button" data-fk="${escapeHtml(fk.name)}" onclick="dropTableFK(this.dataset.fk)" class="text-slate-500 hover:text-red-400 p-1" title="Drop Foreign Key">${renderIcon('trash-2', 'w-3.5 h-3.5')}</button>
          </td>
        </tr>
      `).join('');
    }

    function openAddFKForm() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const isSQLite = activeConnection && (activeConnection.driver === 'sqlite' || activeConnection.driver === 'sqlite3');

      if (isSQLite) {
        showNativeAlert('Foreign Key constraints in SQLite can only be defined during table creation (CREATE TABLE) and cannot be added via ALTER TABLE.', 'SQLite Dialect Guard');
        return;
      }

      const targetCatalog = (activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => t.name === tableName);
      const allTables = targetCatalog?.tables || [];

      const colSelect = document.getElementById('new-fk-col');
      const refTableSelect = document.getElementById('new-fk-ref-table');

      if (colSelect && tableMeta) {
        colSelect.innerHTML = (tableMeta.columns || []).map(c => `<option value="${escapeHtml(c.name)}">${escapeHtml(c.name)} (${escapeHtml(c.type)})</option>`).join('');
      }

      if (refTableSelect) {
        refTableSelect.innerHTML = allTables.map(t => `<option value="${escapeHtml(t.name)}" ${t.name !== tableName ? 'selected' : ''}>${escapeHtml(t.name)}</option>`).join('');
      }

      const initialRefTable = refTableSelect ? refTableSelect.value : '';
      if (initialRefTable) {
        onFKRefTableChange(initialRefTable);
      } else {
        autoGenerateFKName();
      }

      document.getElementById('add-fk-inline-form').classList.remove('hidden');
    }

    function onFKRefTableChange(refTableName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetCatalog = (activeTab && activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const refTableMeta = (targetCatalog?.tables || []).find(t => t.name === refTableName);
      const refColSelect = document.getElementById('new-fk-ref-col');

      if (refTableMeta && refColSelect) {
        const pkCol = (refTableMeta.columns || []).find(c => c.is_primary_key);
        refColSelect.innerHTML = (refTableMeta.columns || []).map(c => `
          <option value="${escapeHtml(c.name)}" ${(pkCol && pkCol.name === c.name) ? 'selected' : ''}>${escapeHtml(c.name)} (${escapeHtml(c.type)}) ${c.is_primary_key ? '🔑' : ''}</option>
        `).join('');
      }

      checkFKTypeCompatibility();
      autoGenerateFKName();
    }

    function autoGenerateFKName() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const colSelect = document.getElementById('new-fk-col');
      const refTableSelect = document.getElementById('new-fk-ref-table');
      const nameInput = document.getElementById('new-fk-name');

      const col = colSelect ? colSelect.value : 'col';
      const refTable = refTableSelect ? refTableSelect.value : 'ref';
      if (nameInput) nameInput.value = `fk_${tableName}_${col}_${refTable}`;
    }

    function checkFKTypeCompatibility() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const targetCatalog = (activeTab.database && typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[activeTab.database.toLowerCase()]) || schemaCatalogCache;
      const tableMeta = (targetCatalog?.tables || []).find(t => t.name === activeTab.tableName);

      const colName = document.getElementById('new-fk-col')?.value;
      const refTableName = document.getElementById('new-fk-ref-table')?.value;
      const refColName = document.getElementById('new-fk-ref-col')?.value;
      const warningEl = document.getElementById('fk-type-mismatch-warning');
      const mismatchText = document.getElementById('fk-mismatch-text');

      if (!colName || !refTableName || !refColName || !warningEl) return;

      const srcCol = (tableMeta?.columns || []).find(c => c.name === colName);
      const refTable = (targetCatalog?.tables || []).find(t => t.name === refTableName);
      const refCol = (refTable?.columns || []).find(c => c.name === refColName);

      if (srcCol && refCol) {
        const srcBase = parseColumnType(srcCol.type).base;
        const refBase = parseColumnType(refCol.type).base;
        if (srcBase !== refBase) {
          if (mismatchText) {
            mismatchText.textContent = `Type mismatch warning: Source column "${colName}" is ${srcCol.type}, but referenced column "${refColName}" is ${refCol.type}. MySQL may reject this constraint (errno 150).`;
          }
          warningEl.classList.remove('hidden');
          return;
        }
      }

      warningEl.classList.add('hidden');
    }

    async function submitNewFK() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const name = document.getElementById('new-fk-name').value.trim();
      const col = document.getElementById('new-fk-col').value;
      const refTable = document.getElementById('new-fk-ref-table').value;
      const refCol = document.getElementById('new-fk-ref-col').value.trim();
      const onDelete = document.getElementById('new-fk-on-delete').value;
      const onUpdate = document.getElementById('new-fk-on-update').value;

      if (!name || !col || !refTable || !refCol) {
        showNativeAlert('All foreign key fields are required.', 'Add Foreign Key');
        return;
      }

      const sql = `ALTER TABLE \`${tableName}\` ADD CONSTRAINT \`${name}\` FOREIGN KEY (\`${col}\`) REFERENCES \`${refTable}\` (\`${refCol}\`) ON DELETE ${onDelete} ON UPDATE ${onUpdate};`;
      document.getElementById('add-fk-inline-form').classList.add('hidden');
      openDDLReviewModal(sql, async () => {
        const ok = await executeDDLStatements(sql, `Foreign key "${name}" created successfully.`);
        if (ok) {
          await loadSchemaTree(activeConnection.id);
          renderTableFKs(tableName);
        }
      });
    }

    async function dropTableFK(fkName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.tableName) return;
      const tableName = activeTab.tableName;
      const sql = `ALTER TABLE \`${tableName}\` DROP FOREIGN KEY \`${fkName}\`;`;
      openDDLReviewModal(sql, async () => {
        const ok = await executeDDLStatements(sql, `Foreign key "${fkName}" dropped successfully.`);
        if (ok) {
          await loadSchemaTree(activeConnection.id);
          renderTableFKs(tableName);
        }
      });
    }

    async function renderTableDDL(tableName) {
      const pre = document.getElementById('table-raw-ddl');
      pre.textContent = 'Loading DDL...';
      try {
        const activeTab = tabs.find(t => t.id === activeTabId);
        const dbName = (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';
        const url = dbName
          ? `/api/session/${activeConnection.id}/ddl/${encodeURIComponent(tableName)}?db=${encodeURIComponent(dbName)}`
          : `/api/session/${activeConnection.id}/ddl/${encodeURIComponent(tableName)}`;
        const res = await apiFetch(url);
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          pre.textContent = 'Failed to load DDL: ' + (errData.error || res.statusText);
          return;
        }
        const data = await res.json();
        pre.textContent = data.ddl || '-- No DDL available';
      } catch (err) {
        pre.textContent = 'Failed to load DDL: ' + err.message;
      }
    }

    async function copyCurrentTableDDL() {
      const text = document.getElementById('table-raw-ddl').textContent;
      const ok = await copyTextToClipboard(text);
      if (ok) {
        showNativeAlert('Table CREATE SQL copied to clipboard!', 'Copy SQL');
      } else {
        showNativeAlert('Failed to copy Table DDL in this browser context.', 'Copy Error');
      }
    }

