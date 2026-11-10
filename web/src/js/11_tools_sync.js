// --- Database-Level & Table-Level SQL Dump Export & Smart Seed ---
    function toggleTopDBToolsMenu(force = null) {
      const menu = document.getElementById('top-db-tools-menu');
      if (!menu) return;
      const willOpen = typeof force === 'boolean' ? force : menu.classList.contains('hidden');
      if (willOpen) closeAllContextMenus();
      if (typeof force === 'boolean') {
        menu.classList.toggle('hidden', !force);
      } else {
        menu.classList.toggle('hidden');
      }
    }

    function toggleTopMonitorMenu(force = null) {
      const menu = document.getElementById('top-monitor-menu');
      if (!menu) return;
      const willOpen = typeof force === 'boolean' ? force : menu.classList.contains('hidden');
      if (willOpen) closeAllContextMenus();
      if (typeof force === 'boolean') {
        menu.classList.toggle('hidden', !force);
      } else {
        menu.classList.toggle('hidden');
      }
      if (!menu.classList.contains('hidden') && activeConnection) {
        fetchProcesslist(true);
      }
    }

    function toggleTopSettingsMenu(force = null) {
      const menu = document.getElementById('top-settings-menu');
      if (!menu) return;
      const willOpen = typeof force === 'boolean' ? force : menu.classList.contains('hidden');
      if (willOpen) closeAllContextMenus();
      if (typeof force === 'boolean') {
        menu.classList.toggle('hidden', !force);
      } else {
        menu.classList.toggle('hidden');
      }
    }

    document.addEventListener('click', (e) => {
      if (!e.target.closest('#top-db-tools-container')) {
        toggleTopDBToolsMenu(false);
      }
      if (!e.target.closest('#top-monitor-container')) {
        toggleTopMonitorMenu(false);
      }
      if (!e.target.closest('#top-settings-container')) {
        toggleTopSettingsMenu(false);
      }
    });

    function openDumpDatabaseModal() {
      if (!activeConnection) {
        showNativeAlert('Please select an active connection first.', 'Dump Database');
        return;
      }
      const activeTab = tabs.find(t => t.id === activeTabId);
      const dbName = activeGridDB || (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || (activeConnection.database) || '';
      const targetDbEl = document.getElementById('dump-modal-target-db');
      if (targetDbEl) {
        targetDbEl.textContent = dbName || (activeConnection.name ? `${activeConnection.name} (default catalog)` : 'Default Database');
      }
      const schemaCheck = document.getElementById('dump-include-schema');
      const dataCheck = document.getElementById('dump-include-data');
      if (schemaCheck) schemaCheck.checked = true;
      if (dataCheck) dataCheck.checked = true;

      const dlg = document.getElementById('dump-database-dialog');
      if (dlg) {
        if (typeof dlg.showModal === 'function') dlg.showModal();
        else dlg.setAttribute('open', '');
      }
    }

    function closeDumpDatabaseModal() {
      const dlg = document.getElementById('dump-database-dialog');
      if (dlg) {
        if (typeof dlg.close === 'function') dlg.close();
        else dlg.removeAttribute('open');
      }
    }

    function exportDatabaseDump() {
      openDumpDatabaseModal();
    }

    function executeDatabaseDump() {
      if (!activeConnection) return;
      const activeTab = tabs.find(t => t.id === activeTabId);
      const dbName = activeGridDB || (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || (activeConnection.database) || '';
      const includeSchema = document.getElementById('dump-include-schema') ? document.getElementById('dump-include-schema').checked : true;
      const includeData = document.getElementById('dump-include-data') ? document.getElementById('dump-include-data').checked : true;

      if (!includeSchema && !includeData) {
        showNativeAlert('Please select at least Table Structure or Table Data to export.', 'Dump Database');
        return;
      }

      let url = `/api/session/${activeConnection.id}/export-dump?schema=${includeSchema}&data=${includeData}`;
      if (dbName) url += `&db=${encodeURIComponent(dbName)}`;
      closeDumpDatabaseModal();
      window.location.href = url;
    }

    function exportTableDump(tableName = null) {
      if (!activeConnection) return;
      const tbl = tableName || activeGridTable;
      if (!tbl) {
        exportDatabaseDump();
        return;
      }
      const activeTab = tabs.find(t => t.id === activeTabId);
      const dbName = activeGridDB || (activeTab && activeTab.database) || '';
      let url = `/api/session/${activeConnection.id}/export-dump?schema=true&data=true&table=${encodeURIComponent(tbl)}`;
      if (dbName) url += `&db=${encodeURIComponent(dbName)}`;
      window.location.href = url;
    }

    function exportSQLDump() {
      exportTableDump();
    }

    function openSmartSeedModal() {
      const dlg = document.getElementById('smart-seed-dialog');
      if (dlg) dlg.showModal();
    }

    function closeSmartSeedModal() {
      const dlg = document.getElementById('smart-seed-dialog');
      if (dlg) dlg.close();
    }

    function executeSmartSeedDownload() {
      if (!activeConnection) return;
      const limitInput = document.getElementById('smart-seed-limit-input');
      const ddlCheckbox = document.getElementById('smart-seed-ddl-checkbox');
      const limit = limitInput ? parseInt(limitInput.value, 10) || 20 : 20;
      const ddl = ddlCheckbox ? ddlCheckbox.checked : true;

      const activeTab = tabs.find(t => t.id === activeTabId);
      const dbName = activeGridDB || (activeTab && activeTab.database) || '';

      let url = `/api/session/${activeConnection.id}/export-smart-seed?limit=${limit}&ddl=${ddl}`;
      if (dbName) url += `&db=${encodeURIComponent(dbName)}`;

      closeSmartSeedModal();
      window.location.href = url;
    }

    // --- Large SQL Script Runner Modal & Streaming Parser ---
    let selectedRunSQLFile = null;

    function openRunSQLScriptModal() {
      const dlg = document.getElementById('run-sql-dialog');
      if (dlg) {
        selectedRunSQLFile = null;
        const fileInput = document.getElementById('run-sql-file-input');
        if (fileInput) fileInput.value = '';
        const meta = document.getElementById('run-sql-file-meta');
        if (meta) { meta.classList.add('hidden'); meta.textContent = ''; }
        const prog = document.getElementById('run-sql-progress-container');
        if (prog) prog.classList.add('hidden');
        const log = document.getElementById('run-sql-log');
        if (log) log.innerHTML = '';
        const runBtn = document.getElementById('run-sql-btn-run');
        if (runBtn) { runBtn.disabled = true; runBtn.textContent = '▶ Start Execution'; }
        dlg.showModal();
      }
    }

    function closeRunSQLScriptModal() {
      const dlg = document.getElementById('run-sql-dialog');
      if (dlg) dlg.close();
    }

    function handleRunSQLFileSelected(input) {
      if (input.files && input.files[0]) {
        selectedRunSQLFile = input.files[0];
        const meta = document.getElementById('run-sql-file-meta');
        if (meta) {
          meta.textContent = `${selectedRunSQLFile.name} (${formatBytes(selectedRunSQLFile.size)})`;
          meta.classList.remove('hidden');
        }
        const runBtn = document.getElementById('run-sql-btn-run');
        if (runBtn) runBtn.disabled = false;
      }
    }

    async function executeRunSQLScript() {
      if (!activeConnection || !selectedRunSQLFile) return;

      const runBtn = document.getElementById('run-sql-btn-run');
      const stopOnErrorCheckbox = document.getElementById('run-sql-stop-on-error');
      const stopOnError = stopOnErrorCheckbox ? stopOnErrorCheckbox.checked : true;
      const progContainer = document.getElementById('run-sql-progress-container');
      const progLabel = document.getElementById('run-sql-progress-label');
      const progCount = document.getElementById('run-sql-progress-count');
      const progBar = document.getElementById('run-sql-progress-bar');
      const logDiv = document.getElementById('run-sql-log');

      if (progContainer) progContainer.classList.remove('hidden');
      if (logDiv) logDiv.innerHTML = '<div class="text-indigo-400">Uploading and parsing script...</div>';
      if (runBtn) {
        runBtn.disabled = true;
        runBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Running...</span>`;
      }

      const formData = new FormData();
      formData.append('file', selectedRunSQLFile);

      const activeTab = tabs.find(t => t.id === activeTabId);
      const dbName = activeGridDB || (activeTab && activeTab.database) || '';

      try {
        let url = `/api/session/${activeConnection.id}/run-sql-script?stop_on_error=${stopOnError}`;
        if (dbName) url += `&db=${encodeURIComponent(dbName)}`;

        const response = await fetch(url, {
          method: 'POST',
          body: formData
        });

        if (!response.ok) {
          const err = await response.text();
          if (logDiv) logDiv.innerHTML += `<div class="text-rose-400">Failed to run script: ${escapeHtml(err)}</div>`;
          if (runBtn) { runBtn.disabled = false; runBtn.innerHTML = `${renderIcon('play', 'w-3.5 h-3.5 inline mr-1')}<span>Retry</span>`; }
          return;
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let partialLine = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          partialLine += decoder.decode(value, { stream: true });
          const lines = partialLine.split('\n');
          partialLine = lines.pop();

          for (const line of lines) {
            if (!line.trim()) continue;
            try {
              const event = JSON.parse(line);
              if (event.statement_index !== undefined) {
                const pct = Math.round((event.statement_index / event.total) * 100);
                if (progBar) progBar.style.width = `${pct}%`;
                if (progCount) progCount.textContent = `${event.statement_index} / ${event.total}`;
                if (progLabel) progLabel.textContent = `Executing: ${escapeHtml(event.sql_preview)}`;

                if (event.status === 'error') {
                  if (logDiv) logDiv.innerHTML += `<div class="text-rose-400">❌ [Stmt #${event.statement_index}] Error: ${escapeHtml(event.error)}</div>`;
                } else {
                  if (logDiv) logDiv.innerHTML += `<div class="text-emerald-400/90">✓ [Stmt #${event.statement_index}] (${event.duration_ms}ms, ${event.rows_affected} rows)</div>`;
                }
                if (logDiv) logDiv.scrollTop = logDiv.scrollHeight;
              } else if (event.status === 'completed' || event.status === 'halted') {
                if (progLabel) progLabel.textContent = event.status === 'completed' ? 'Execution Completed!' : 'Execution Halted (Error)';
                if (logDiv) {
                  logDiv.innerHTML += `<div class="font-bold text-white pt-2 border-t border-slate-800">Done: ${event.executed_statements} executed, ${event.error_count} error(s).</div>`;
                  logDiv.scrollTop = logDiv.scrollHeight;
                }
              }
            } catch (e) {
              console.error('Error parsing NDJSON event:', e);
            }
          }
        }

        if (runBtn) {
          runBtn.disabled = false;
          runBtn.innerHTML = `${renderIcon('check', 'w-3.5 h-3.5 inline mr-1 text-emerald-400')}<span>Done</span>`;
        }
        if (typeof refreshActiveSchema === 'function') {
          refreshActiveSchema();
        }
      } catch (err) {
        if (logDiv) logDiv.innerHTML += `<div class="text-rose-400">Network error: ${escapeHtml(err.message)}</div>`;
        if (runBtn) { runBtn.disabled = false; runBtn.innerHTML = `${renderIcon('play', 'w-3.5 h-3.5 inline mr-1')}<span>Retry</span>`; }
      }
    }

    // =========================================================================
    // Milestone 11: Schema & Data Synchronization Engine (Client UI)
    // =========================================================================

    function populateSyncConnectionSelects(srcSelectId, tgtSelectId) {
      const src = document.getElementById(srcSelectId);
      const tgt = document.getElementById(tgtSelectId);
      if (!src || !tgt) return;
      src.innerHTML = '';
      tgt.innerHTML = '';
      connections.forEach(c => {
        const isCurrent = activeConnection && activeConnection.id === c.id;
        const optSrc = document.createElement('option');
        optSrc.value = c.id;
        optSrc.textContent = `${c.name} (${c.environment || 'dev'}, ${c.driver})`;
        if (isCurrent) optSrc.selected = true;
        src.appendChild(optSrc);

        const optTgt = document.createElement('option');
        optTgt.value = c.id;
        optTgt.textContent = `${c.name} (${c.environment || 'dev'}, ${c.driver})`;
        tgt.appendChild(optTgt);
      });
      if (connections.length > 1 && tgt.options.length > 1) {
        tgt.selectedIndex = 1;
      }
    }

    async function ensureSyncSessionConnected(connId) {
      try {
        await apiFetch(`/api/session/connect/${connId}`, { method: 'POST' });
      } catch (e) {
        console.warn('Session already connected or connect returned:', e);
      }
    }

    // --- 1. Structure Sync UI ---
    let currentStructureDiffs = [];

    function openStructureSyncModal() {
      populateSyncConnectionSelects('sync-struct-source-conn', 'sync-struct-target-conn');
      const diffContainer = document.getElementById('sync-struct-diff-container');
      if (diffContainer) diffContainer.classList.add('hidden');
      const applyBtn = document.getElementById('sync-struct-btn-apply');
      if (applyBtn) applyBtn.disabled = true;
      currentStructureDiffs = [];
      const dlg = document.getElementById('structure-sync-dialog');
      if (dlg) dlg.showModal();
    }

    function closeStructureSyncModal() {
      const dlg = document.getElementById('structure-sync-dialog');
      if (dlg) dlg.close();
    }

    function switchStructureSyncView(view) {
      const diffTab = document.getElementById('sync-struct-tab-diff');
      const sqlTab = document.getElementById('sync-struct-tab-sql');
      const diffView = document.getElementById('sync-struct-view-diff');
      const sqlView = document.getElementById('sync-struct-view-sql');
      if (!diffTab || !sqlTab || !diffView || !sqlView) return;

      if (view === 'diff') {
        diffTab.className = 'px-2.5 py-1 rounded bg-slate-800 text-emerald-400 font-medium';
        sqlTab.className = 'px-2.5 py-1 rounded hover:bg-slate-800 text-slate-400';
        diffView.classList.remove('hidden');
        sqlView.classList.add('hidden');
      } else {
        sqlTab.className = 'px-2.5 py-1 rounded bg-slate-800 text-emerald-400 font-medium';
        diffTab.className = 'px-2.5 py-1 rounded hover:bg-slate-800 text-slate-400';
        diffView.classList.add('hidden');
        sqlView.classList.remove('hidden');
        updateStructureScriptPreview();
      }
    }

    function updateStructureScriptPreview() {
      const preview = document.getElementById('sync-struct-script-preview');
      if (!preview) return;
      const selectedSQLs = currentStructureDiffs
        .filter(d => d.selected && d.sql)
        .map(d => d.sql);
      preview.value = selectedSQLs.join('\n\n');
    }

    function toggleAllStructureDiffs(checked) {
      currentStructureDiffs.forEach(d => { d.selected = checked; });
      const checkboxes = document.querySelectorAll('.sync-struct-diff-check');
      checkboxes.forEach(cb => { cb.checked = checked; });
      const master = document.getElementById('sync-struct-master-check');
      if (master) master.checked = checked;
      updateStructureScriptPreview();
      const applyBtn = document.getElementById('sync-struct-btn-apply');
      if (applyBtn) {
        applyBtn.disabled = !currentStructureDiffs.some(d => d.selected);
      }
    }

    function toggleSingleStructureDiff(diffId, checked) {
      const item = currentStructureDiffs.find(d => d.id === diffId);
      if (item) item.selected = checked;
      updateStructureScriptPreview();
      const applyBtn = document.getElementById('sync-struct-btn-apply');
      if (applyBtn) {
        applyBtn.disabled = !currentStructureDiffs.some(d => d.selected);
      }
    }

    async function executeStructureCompare() {
      const srcConnId = document.getElementById('sync-struct-source-conn')?.value;
      const tgtConnId = document.getElementById('sync-struct-target-conn')?.value;
      const srcDB = document.getElementById('sync-struct-source-db')?.value?.trim() || '';
      const tgtDB = document.getElementById('sync-struct-target-db')?.value?.trim() || '';
      const includeDrops = document.getElementById('sync-struct-include-drops')?.checked || false;

      if (!srcConnId || !tgtConnId) {
        showToast('Please select source and target connections', 'error');
        return;
      }
      if (srcConnId === tgtConnId && srcDB === tgtDB && !srcDB) {
        showToast('Source and target cannot be identical without distinct database names', 'warning');
        return;
      }

      const cmpBtn = document.getElementById('sync-struct-btn-compare');
      if (cmpBtn) {
        cmpBtn.disabled = true;
        cmpBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Comparing...</span>`;
      }

      try {
        await ensureSyncSessionConnected(srcConnId);
        await ensureSyncSessionConnected(tgtConnId);

        const res = await apiFetch('/api/sync/structure/compare', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            source_session_id: srcConnId,
            source_db: srcDB,
            target_session_id: tgtConnId,
            target_db: tgtDB,
            include_drops: includeDrops
          })
        });

        const data = await res.json();
        currentStructureDiffs = data.diffs || [];

        const diffContainer = document.getElementById('sync-struct-diff-container');
        if (diffContainer) diffContainer.classList.remove('hidden');

        const badge = document.getElementById('sync-struct-count-badge');
        if (badge) badge.textContent = `${currentStructureDiffs.length} discrepancy(ies)`;

        const tbody = document.getElementById('sync-struct-diff-rows');
        if (tbody) {
          if (currentStructureDiffs.length === 0) {
            tbody.innerHTML = `<tr><td colspan="5" class="p-6 text-center text-slate-400 font-sans">Schemas are completely identical! Zero differences found.</td></tr>`;
          } else {
            tbody.innerHTML = currentStructureDiffs.map(d => {
              let actionBadge = '';
              if (d.action.startsWith('CREATE') || d.action.startsWith('ADD')) {
                actionBadge = `<span class="px-1.5 py-0.5 rounded bg-emerald-950 border border-emerald-800 text-emerald-400 font-semibold">${d.action}</span>`;
              } else if (d.action.startsWith('MODIFY')) {
                actionBadge = `<span class="px-1.5 py-0.5 rounded bg-amber-950 border border-amber-800 text-amber-400 font-semibold">${d.action}</span>`;
              } else {
                actionBadge = `<span class="px-1.5 py-0.5 rounded bg-rose-950 border border-rose-800 text-rose-400 font-semibold">${d.action}</span>`;
              }

              const objName = d.column ? `${d.table}.${d.column}` : (d.index ? `${d.table}.${d.index}` : d.table);

              return `<tr class="hover:bg-slate-900/60">
                <td class="p-2 text-center"><input type="checkbox" ${d.selected ? 'checked' : ''} onchange="toggleSingleStructureDiff('${d.id}', this.checked)" class="sync-struct-diff-check accent-emerald-500"></td>
                <td class="p-2 font-bold text-white">${escapeHtml(objName)}</td>
                <td class="p-2">${actionBadge}</td>
                <td class="p-2 text-sky-300">${escapeHtml(d.source_def || '')}</td>
                <td class="p-2 text-slate-400">${escapeHtml(d.target_def || '')}</td>
              </tr>`;
            }).join('');
          }
        }

        updateStructureScriptPreview();

        // Check if target is Production
        const tgtConn = connections.find(c => c.id === tgtConnId);
        const challengeBox = document.getElementById('sync-struct-challenge-container');
        if (challengeBox) {
          if (tgtConn && tgtConn.environment && tgtConn.environment.toLowerCase() === 'production') {
            challengeBox.classList.remove('hidden');
            const targetSpan = document.getElementById('sync-struct-challenge-target');
            if (targetSpan) targetSpan.textContent = tgtDB || tgtConn.name || 'MIGRATE';
          } else {
            challengeBox.classList.add('hidden');
          }
        }

        const applyBtn = document.getElementById('sync-struct-btn-apply');
        if (applyBtn) {
          applyBtn.disabled = currentStructureDiffs.length === 0;
          applyBtn.textContent = `⚡ Execute Migration (${currentStructureDiffs.length})`;
        }
      } catch (err) {
        showToast('Structure compare failed: ' + err.message, 'error');
      } finally {
        if (cmpBtn) {
          cmpBtn.disabled = false;
          cmpBtn.innerHTML = '<svg class="w-3.5 h-3.5 inline mr-1"><use href="#icon-search"/></svg><span>Compare Schemas</span>';
        }
      }
    }

    async function executeStructureApply() {
      const tgtConnId = document.getElementById('sync-struct-target-conn')?.value;
      const tgtDB = document.getElementById('sync-struct-target-db')?.value?.trim() || '';
      const selectedSQLs = currentStructureDiffs
        .filter(d => d.selected && d.sql)
        .map(d => d.sql);

      if (selectedSQLs.length === 0) {
        showToast('No migration statements selected to apply', 'warning');
        return;
      }

      const tgtConn = connections.find(c => c.id === tgtConnId);
      const isProd = tgtConn && tgtConn.environment && tgtConn.environment.toLowerCase() === 'production';
      let challengeInput = '';

      if (isProd) {
        const inp = document.getElementById('sync-struct-challenge-input');
        challengeInput = inp ? inp.value.trim() : '';
        const expected = tgtDB || tgtConn.name || 'MIGRATE';
        if (challengeInput.toUpperCase() !== expected.toUpperCase() && challengeInput.toUpperCase() !== 'MIGRATE') {
          showToast(`Challenge input required. Type '${expected}' or 'MIGRATE'`, 'error');
          return;
        }
      }

      const applyBtn = document.getElementById('sync-struct-btn-apply');
      if (applyBtn) {
        applyBtn.disabled = true;
        applyBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Applying...</span>`;
      }

      try {
        const res = await apiFetch('/api/sync/structure/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            target_session_id: tgtConnId,
            target_db: tgtDB,
            statements: selectedSQLs,
            confirmed: true,
            challenge_input: challengeInput
          })
        });

        const data = await res.json();
        if (data.success) {
          showToast(`✓ Migration applied successfully (${data.executed} statements executed)`, 'success');
          closeStructureSyncModal();
          if (typeof refreshActiveSchema === 'function') refreshActiveSchema();
        } else {
          showToast(`Migration partially failed: ${data.errors?.join('; ')}`, 'error');
        }
      } catch (err) {
        showToast('Apply failed: ' + err.message, 'error');
      } finally {
        if (applyBtn) {
          applyBtn.disabled = false;
          applyBtn.innerHTML = `${renderIcon('zap', 'w-3.5 h-3.5 inline mr-1')}<span>Execute Migration</span>`;
        }
      }
    }

    // --- 2. Cross-Connection Data Transfer UI ---
    let transferSourceTables = [];

    function openDataTransferModal() {
      populateSyncConnectionSelects('sync-transfer-source-conn', 'sync-transfer-target-conn');
      loadTransferSourceTables();
      const prog = document.getElementById('sync-transfer-progress-container');
      if (prog) prog.classList.add('hidden');
      const dlg = document.getElementById('data-transfer-dialog');
      if (dlg) dlg.showModal();
    }

    function closeDataTransferModal() {
      const dlg = document.getElementById('data-transfer-dialog');
      if (dlg) dlg.close();
    }

    async function loadTransferSourceTables() {
      const srcConnId = document.getElementById('sync-transfer-source-conn')?.value;
      const srcDB = document.getElementById('sync-transfer-source-db')?.value?.trim() || '';
      const listDiv = document.getElementById('sync-transfer-table-list');
      if (!srcConnId || !listDiv) return;

      listDiv.innerHTML = '<div class="text-slate-400 col-span-2 text-center py-4 text-xs">Loading tables...</div>';

      try {
        await ensureSyncSessionConnected(srcConnId);
        let url = `/api/session/${srcConnId}/schema`;
        if (srcDB) url += `?db=${encodeURIComponent(srcDB)}`;
        const res = await apiFetch(url);
        const data = await res.json();
        transferSourceTables = (data.tables || []).map(t => t.name);

        if (transferSourceTables.length === 0) {
          listDiv.innerHTML = '<div class="text-slate-500 col-span-2 text-center py-4 font-sans text-xs">No tables found in source database</div>';
        } else {
          listDiv.innerHTML = transferSourceTables.map(tbl => `
            <label class="flex items-center space-x-2 text-slate-200 cursor-pointer hover:text-white">
              <input type="checkbox" value="${escapeHtml(tbl)}" checked class="sync-transfer-tbl-check accent-cyan-500 w-4 h-4 cursor-pointer">
              <span class="truncate">${escapeHtml(tbl)}</span>
            </label>
          `).join('');
        }
      } catch (err) {
        listDiv.innerHTML = `<div class="text-rose-400 col-span-2 text-center py-4 font-sans text-xs">Failed to load tables: ${escapeHtml(err.message)}</div>`;
      }
    }

    function toggleAllTransferTables(checked) {
      document.querySelectorAll('.sync-transfer-tbl-check').forEach(cb => { cb.checked = checked; });
    }

    async function executeDataTransfer() {
      const srcConnId = document.getElementById('sync-transfer-source-conn')?.value;
      const tgtConnId = document.getElementById('sync-transfer-target-conn')?.value;
      const srcDB = document.getElementById('sync-transfer-source-db')?.value?.trim() || '';
      const tgtDB = document.getElementById('sync-transfer-target-db')?.value?.trim() || '';

      const checkedTables = Array.from(document.querySelectorAll('.sync-transfer-tbl-check:checked')).map(cb => cb.value);
      if (checkedTables.length === 0) {
        showToast('Please select at least one table to transfer', 'warning');
        return;
      }

      const createTables = document.getElementById('sync-transfer-create-tables')?.checked || false;
      const truncateTarget = document.getElementById('sync-transfer-truncate')?.checked || false;
      const batchSize = parseInt(document.getElementById('sync-transfer-batch-size')?.value, 10) || 500;

      const progContainer = document.getElementById('sync-transfer-progress-container');
      const progLabel = document.getElementById('sync-transfer-progress-label');
      const progCount = document.getElementById('sync-transfer-progress-count');
      const progBar = document.getElementById('sync-transfer-progress-bar');
      const logDiv = document.getElementById('sync-transfer-log');
      const startBtn = document.getElementById('sync-transfer-btn-start');

      if (progContainer) progContainer.classList.remove('hidden');
      if (progLabel) progLabel.textContent = 'Connecting and transferring tables...';
      if (logDiv) logDiv.innerHTML = '<div class="text-cyan-400">Initializing cross-connection data transfer...</div>';
      if (startBtn) {
        startBtn.disabled = true;
        startBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Transferring...</span>`;
      }

      try {
        await ensureSyncSessionConnected(srcConnId);
        await ensureSyncSessionConnected(tgtConnId);

        const response = await fetch('/api/sync/data/transfer', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            source_session_id: srcConnId,
            source_db: srcDB,
            target_session_id: tgtConnId,
            target_db: tgtDB,
            tables: checkedTables,
            create_tables: createTables,
            truncate_target: truncateTarget,
            batch_size: batchSize
          })
        });

        if (!response.ok) {
          const err = await response.text();
          if (logDiv) logDiv.innerHTML += `<div class="text-rose-400">Transfer failed: ${escapeHtml(err)}</div>`;
        if (startBtn) { startBtn.disabled = false; startBtn.innerHTML = renderIcon('zap', 'w-3.5 h-3.5 inline mr-1') + '<span>Start Transfer</span>'; }
          return;
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop();

          for (const line of lines) {
            if (!line.trim()) continue;
            try {
              const event = JSON.parse(line);
              if (event.event === 'table_start') {
                if (progLabel) progLabel.textContent = `Transferring table: ${event.table}`;
                if (logDiv) logDiv.innerHTML += `<div class="text-sky-300 flex items-center gap-1.5">${renderIcon('package', 'w-3.5 h-3.5 text-sky-400 shrink-0')}<span>Table: ${escapeHtml(event.table)} started</span></div>`;
              } else if (event.event === 'progress') {
                if (progCount) progCount.textContent = `${event.total_transferred} rows copied`;
                if (progBar) progBar.style.width = '60%';
              } else if (event.event === 'table_done') {
                if (logDiv) logDiv.innerHTML += `<div class="text-emerald-400 flex items-center gap-1.5">${renderIcon('check', 'w-3.5 h-3.5 text-emerald-400 shrink-0')}<span>Table ${escapeHtml(event.table)}: ${event.rows_copied} rows copied</span></div>`;
              } else if (event.event === 'done') {
                if (progBar) progBar.style.width = '100%';
                if (progLabel) progLabel.textContent = 'Transfer Completed!';
                if (progCount) progCount.textContent = `${event.total_rows} total rows transferred`;
                if (logDiv) logDiv.innerHTML += `<div class="font-bold text-white pt-2 border-t border-slate-800">Done: ${event.total_tables} tables, ${event.total_rows} rows transferred.</div>`;
              } else if (event.event === 'table_error') {
                if (logDiv) logDiv.innerHTML += `<div class="text-rose-400 flex items-center gap-1.5">${renderIcon('x-circle', 'w-3.5 h-3.5 text-rose-400 shrink-0')}<span>Table ${escapeHtml(event.table)} error: ${escapeHtml(event.error)}</span></div>`;
              }
              if (logDiv) logDiv.scrollTop = logDiv.scrollHeight;
            } catch (e) {
              console.error('Error parsing transfer event:', e);
            }
          }
        }

        showToast('✓ Data transfer completed successfully', 'success');
        if (startBtn) {
          startBtn.disabled = false;
          startBtn.innerHTML = renderIcon('check', 'w-3.5 h-3.5 inline mr-1 text-emerald-400') + '<span>Done</span>';
        }
      } catch (err) {
        if (logDiv) logDiv.innerHTML += `<div class="text-rose-400">Network error: ${escapeHtml(err.message)}</div>`;
        if (startBtn) { startBtn.disabled = false; startBtn.innerHTML = renderIcon('zap', 'w-3.5 h-3.5 inline mr-1') + '<span>Start Transfer</span>'; }
      }
    }

    // --- 3. Row-Level Data Sync UI ---
    let currentDataSyncOperations = [];
    let currentDataSyncColumns = [];

    function openDataSyncModal() {
      populateSyncConnectionSelects('sync-data-source-conn', 'sync-data-target-conn');
      loadDataSyncTables();
      const resContainer = document.getElementById('sync-data-results-container');
      if (resContainer) resContainer.classList.add('hidden');
      const applyBtn = document.getElementById('sync-data-btn-apply');
      if (applyBtn) applyBtn.disabled = true;
      currentDataSyncOperations = [];
      const dlg = document.getElementById('data-sync-dialog');
      if (dlg) dlg.showModal();
    }

    function closeDataSyncModal() {
      const dlg = document.getElementById('data-sync-dialog');
      if (dlg) dlg.close();
    }

    async function loadDataSyncTables() {
      const srcConnId = document.getElementById('sync-data-source-conn')?.value;
      const srcDB = document.getElementById('sync-data-source-db')?.value?.trim() || '';
      const select = document.getElementById('sync-data-table-select');
      if (!srcConnId || !select) return;

      select.innerHTML = '<option>Loading...</option>';

      try {
        await ensureSyncSessionConnected(srcConnId);
        let url = `/api/session/${srcConnId}/schema`;
        if (srcDB) url += `?db=${encodeURIComponent(srcDB)}`;
        const res = await apiFetch(url);
        const data = await res.json();
        const tables = (data.tables || []).map(t => t.name);

        select.innerHTML = '';
        tables.forEach(tbl => {
          const opt = document.createElement('option');
          opt.value = tbl;
          opt.textContent = tbl;
          select.appendChild(opt);
        });
      } catch (err) {
        select.innerHTML = '<option>Error loading tables</option>';
      }
    }

    async function executeDataCompare() {
      const srcConnId = document.getElementById('sync-data-source-conn')?.value;
      const tgtConnId = document.getElementById('sync-data-target-conn')?.value;
      const srcDB = document.getElementById('sync-data-source-db')?.value?.trim() || '';
      const tgtDB = document.getElementById('sync-data-target-db')?.value?.trim() || '';
      const table = document.getElementById('sync-data-table-select')?.value;
      const deleteMissing = document.getElementById('sync-data-delete-missing')?.checked || false;

      if (!table) {
        showToast('Please select a table to compare', 'warning');
        return;
      }

      const cmpBtn = document.getElementById('sync-data-btn-compare');
      if (cmpBtn) {
        cmpBtn.disabled = true;
        cmpBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Comparing...</span>`;
      }

      try {
        await ensureSyncSessionConnected(srcConnId);
        await ensureSyncSessionConnected(tgtConnId);

        const res = await apiFetch('/api/sync/data/compare', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            source_session_id: srcConnId,
            source_db: srcDB,
            target_session_id: tgtConnId,
            target_db: tgtDB,
            table: table,
            delete_missing: deleteMissing
          })
        });

        const data = await res.json();
        currentDataSyncOperations = data.operations || [];
        currentDataSyncColumns = data.columns || [];

        const resultsContainer = document.getElementById('sync-data-results-container');
        if (resultsContainer) resultsContainer.classList.remove('hidden');

        const insertBadge = document.getElementById('sync-data-insert-badge');
        const updateBadge = document.getElementById('sync-data-update-badge');
        const deleteBadge = document.getElementById('sync-data-delete-badge');
        if (insertBadge) insertBadge.textContent = `+ ${data.summary?.insert_count || 0} Inserts`;
        if (updateBadge) updateBadge.textContent = `~ ${data.summary?.update_count || 0} Updates`;
        if (deleteBadge) deleteBadge.textContent = `- ${data.summary?.delete_count || 0} Deletes`;

        const tbody = document.getElementById('sync-data-diff-rows');
        if (tbody) {
          if (currentDataSyncOperations.length === 0) {
            tbody.innerHTML = `<tr><td colspan="3" class="p-6 text-center text-slate-400 font-sans">Tables are completely synchronized! Zero row differences found.</td></tr>`;
          } else {
            tbody.innerHTML = currentDataSyncOperations.slice(0, 100).map(op => {
              let badge = '';
              if (op.type === 'insert') {
                badge = `<span class="px-1.5 py-0.5 rounded bg-emerald-950 border border-emerald-800 text-emerald-400 font-semibold text-[10px]">INSERT</span>`;
              } else if (op.type === 'update') {
                badge = `<span class="px-1.5 py-0.5 rounded bg-amber-950 border border-amber-800 text-amber-400 font-semibold text-[10px]">UPDATE</span>`;
              } else {
                badge = `<span class="px-1.5 py-0.5 rounded bg-rose-950 border border-rose-800 text-rose-400 font-semibold text-[10px]">DELETE</span>`;
              }

              const rowPreview = (op.data || []).map(v => v === null ? '<null>' : (typeof v === 'object' ? (v.$base64 !== undefined ? '<binary>' : JSON.stringify(v)) : v)).join(', ');

              return `<tr class="hover:bg-slate-900/60">
                <td class="p-2">${badge}</td>
                <td class="p-2 font-bold text-white">${escapeHtml(String(op.pk))}</td>
                <td class="p-2 text-slate-300 truncate max-w-lg" title="${escapeHtml(rowPreview)}">${escapeHtml(rowPreview)}</td>
              </tr>`;
            }).join('');
          }
        }

        // Check if target is Production
        const tgtConn = connections.find(c => c.id === tgtConnId);
        const challengeBox = document.getElementById('sync-data-challenge-container');
        if (challengeBox) {
          if (tgtConn && tgtConn.environment && tgtConn.environment.toLowerCase() === 'production') {
            challengeBox.classList.remove('hidden');
            const targetSpan = document.getElementById('sync-data-challenge-target');
            if (targetSpan) targetSpan.textContent = table || 'SYNC';
          } else {
            challengeBox.classList.add('hidden');
          }
        }

        const applyBtn = document.getElementById('sync-data-btn-apply');
        if (applyBtn) {
          applyBtn.disabled = currentDataSyncOperations.length === 0;
          applyBtn.textContent = `✓ Apply Sync to Target (${currentDataSyncOperations.length} rows)`;
        }
      } catch (err) {
        showToast('Data compare failed: ' + err.message, 'error');
      } finally {
        if (cmpBtn) {
          cmpBtn.disabled = false;
          cmpBtn.innerHTML = '<svg class="w-3.5 h-3.5 inline mr-1"><use href="#icon-search"/></svg><span>Compare Rows</span>';
        }
      }
    }

    async function executeDataApply() {
      const tgtConnId = document.getElementById('sync-data-target-conn')?.value;
      const tgtDB = document.getElementById('sync-data-target-db')?.value?.trim() || '';
      const table = document.getElementById('sync-data-table-select')?.value;

      if (!table || currentDataSyncOperations.length === 0) {
        showToast('No operations to apply', 'warning');
        return;
      }

      const tgtConn = connections.find(c => c.id === tgtConnId);
      const isProd = tgtConn && tgtConn.environment && tgtConn.environment.toLowerCase() === 'production';
      let challengeInput = '';

      if (isProd) {
        const inp = document.getElementById('sync-data-challenge-input');
        challengeInput = inp ? inp.value.trim() : '';
        const expected = table || 'SYNC';
        if (challengeInput.toUpperCase() !== expected.toUpperCase() && challengeInput.toUpperCase() !== 'SYNC') {
          showToast(`Challenge input required. Type '${expected}' or 'SYNC'`, 'error');
          return;
        }
      }

      const applyBtn = document.getElementById('sync-data-btn-apply');
      if (applyBtn) {
        applyBtn.disabled = true;
        applyBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Applying...</span>`;
      }

      try {
        const res = await apiFetch('/api/sync/data/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            target_session_id: tgtConnId,
            target_db: tgtDB,
            table: table,
            columns: currentDataSyncColumns,
            operations: currentDataSyncOperations,
            confirmed: true,
            challenge_input: challengeInput
          })
        });

        const data = await res.json();
        if (data.success) {
          showToast(`✓ Data sync applied: +${data.inserted} inserted, ~${data.updated} updated, -${data.deleted} deleted`, 'success');
          closeDataSyncModal();
        } else {
          showToast(`Data sync had errors: ${data.errors?.join('; ')}`, 'error');
        }
      } catch (err) {
        showToast('Data sync apply failed: ' + err.message, 'error');
      } finally {
        if (applyBtn) {
          applyBtn.disabled = false;
          applyBtn.innerHTML = renderIcon('check', 'w-3.5 h-3.5 inline mr-1') + '<span>Apply Changes to Target</span>';
        }
      }
    }

    let isApplyingGridChanges = false;
    async function applyGridChanges() {
      const totalCount = stagedEdits.size + stagedDraftRows.length;
      if (totalCount === 0 || isApplyingGridChanges) return;
      if (!activeConnection) return;

      if (activeEditCell) {
        await commitInlineEdit(activeEditCell, false);
      }

      if (gridConfirmSave) {
        const ok = await showNativeConfirm(`Apply ${totalCount} pending change(s) to table "${activeGridTable}"?`, 'Apply Changes');
        if (!ok) return;
      }

      isApplyingGridChanges = true;
      const applyBtn = document.getElementById('btn-grid-apply');
      const prevApplyHTML = applyBtn ? applyBtn.innerHTML : '';
      if (applyBtn) {
        applyBtn.disabled = true;
        applyBtn.classList.add('opacity-60', 'pointer-events-none');
        applyBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1')}<span>Applying...</span>`;
      }

      // ponytail: adaptive warning timer if server connection is slow (> 3 seconds)
      let slowApplyTimer = setTimeout(() => {
        if (isApplyingGridChanges) {
          showToast('Server responding slowly, still applying changes...', 'warning');
        }
      }, 3000);

      try {
        const activeTab = tabs.find(t => t.id === activeTabId);
        // ponytail: Prioritize activeGridDB over activeTab.database so staged edits commit to the table's schema.
        const dbName = activeGridDB || (activeTab && activeTab.database) || '';

        // 1. Process any draft rows
        while (stagedDraftRows.length > 0) {
          const draft = stagedDraftRows[0];
          try {
            const cleanData = {};
            const pkLower = (activeGridPKCol || '').toLowerCase();
            for (const [colName, val] of Object.entries(draft.data || {})) {
              if (val !== undefined && val !== null) {
                if (val === '' && pkLower && colName.toLowerCase() === pkLower) {
                  continue;
                }
                cleanData[colName] = val;
              }
            }
            const res = await apiFetch(`/api/session/${activeConnection.id}/insert-row`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                database: dbName,
                table: activeGridTable,
                data: cleanData
              })
            });
            if (!res.ok) {
              const err = await res.json().catch(() => ({}));
              showNativeAlert('Failed to insert row: ' + (err.error || res.statusText), 'Insert Error');
              return;
            }
            stagedDraftRows.shift();
          } catch (err) {
            showNativeAlert('Network error inserting row: ' + err.message, 'Insert Error');
            return;
          }
        }

        // 2. Process any cell edits
        if (stagedEdits.size > 0) {
          const edits = Array.from(stagedEdits.values()).map(item => ({
            database: item.db || dbName,
            table: item.table,
            column: item.colName,
            value: item.newVal,
            pk_column: item.pkCol,
            pk_value: item.pkVal,
            row_criteria: item.rowCriteria
          }));

          try {
            const res = await apiFetch(`/api/session/${activeConnection.id}/cell-edit`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ database: dbName, edits })
            });
            if (!res.ok) {
              const err = await res.json().catch(() => ({}));
              showNativeAlert('Failed to apply cell edits: ' + (err.error || res.statusText), 'Apply Error');
              return;
            }
            stagedEdits.clear();
          } catch (err) {
            showNativeAlert('Network error applying cell edits: ' + err.message, 'Apply Error');
            return;
          }
        }

        showNativeAlert(`✅ Successfully applied ${totalCount} change(s)!`, 'Changes Applied');
        if (activeTab && activeTab.isTableViewer) {
          if (activeGridTable) {
            await loadTableRows(activeGridTable, activeGridDB);
          }
        } else {
          await runActiveQuery();
        }
      } finally {
        clearTimeout(slowApplyTimer);
        isApplyingGridChanges = false;
        updateStagedEditsUI();
        if (applyBtn) {
          applyBtn.disabled = false;
          applyBtn.classList.remove('opacity-60', 'pointer-events-none');
          applyBtn.innerHTML = prevApplyHTML;
        }
      }
    }

    async function discardGridChanges() {
      if (stagedEdits.size === 0 && stagedDraftRows.length === 0) return;

      if (activeEditCell) {
        cancelInlineEdit(activeEditCell);
      }

      const totalCount = stagedEdits.size + stagedDraftRows.length;
      if (gridConfirmSave) {
        const ok = await showNativeConfirm(`Discard all ${totalCount} unapplied change(s)?`, 'Discard Changes');
        if (!ok) return;
      }

      stagedDraftRows = [];
      for (const item of stagedEdits.values()) {
        const row = item.rowRef || currentResults[item.rIdx];
        if (row) row[item.cIdx] = item.oldVal;
      }
      stagedEdits.clear();
      updateStagedEditsUI();

      if (activeGridTable) {
        renderGridResults(currentColumns, currentResults, activeGridPKCol, activeGridTable, activeGridDB);
      }
    }

