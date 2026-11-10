// --- Cross-Database Schema Search (Ctrl+P / Quick Open) ---
    let schemaSearchIndex = [];
    let schemaSearchResults = [];
    let schemaSearchSelectedIndex = 0;

    function buildSchemaSearchIndex() {
      const items = [];
      const dbs = new Set();
      if (activeConnection && activeConnection.database_name) {
        dbs.add(activeConnection.database_name);
      }
      if (schemaCatalogCache && schemaCatalogCache.database_name) {
        dbs.add(schemaCatalogCache.database_name);
      }
      if (typeof multiDBCatalogCache === 'object') {
        Object.keys(multiDBCatalogCache).forEach(db => dbs.add(db));
      }

      dbs.forEach(dbName => {
        const lowerDb = dbName.toLowerCase();
        const catalog = (typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[lowerDb]) ||
          (schemaCatalogCache && schemaCatalogCache.database_name && schemaCatalogCache.database_name.toLowerCase() === lowerDb ? schemaCatalogCache : null) ||
          schemaCatalogCache;

        if (catalog) {
          // Tables
          (catalog.tables || []).forEach(tbl => {
            const isView = tbl.type === 'VIEW';
            items.push({
              kind: isView ? 'VIEW' : 'TBL',
              database: dbName,
              table: tbl.name,
              column: null,
              type: isView ? 'VIEW' : 'TABLE',
              fullName: `${dbName}.${tbl.name}`
            });

            // Columns
            (tbl.columns || []).forEach(col => {
              items.push({
                kind: 'COL',
                database: dbName,
                table: tbl.name,
                column: col.name,
                type: col.type || '',
                fullName: `${dbName}.${tbl.name}.${col.name}`
              });
            });
          });

          // Standalone Views if any
          (catalog.views || []).forEach(v => {
            if (!items.some(it => it.kind === 'VIEW' && it.database === dbName && it.table === v.name)) {
              items.push({
                kind: 'VIEW',
                database: dbName,
                table: v.name,
                column: null,
                type: 'VIEW',
                fullName: `${dbName}.${v.name}`
              });
            }
          });
        }
      });

      schemaSearchIndex = items;
    }

    function toggleSchemaSearch() {
      const dlg = document.getElementById('schema-search-modal');
      if (!dlg) return;
      if (dlg.hasAttribute('open') || (typeof dlg.open === 'boolean' && dlg.open)) {
        closeSchemaSearch();
      } else {
        openSchemaSearch();
      }
    }

    function openSchemaSearch() {
      buildSchemaSearchIndex();
      const dlg = document.getElementById('schema-search-modal');
      const input = document.getElementById('schema-search-input');
      if (typeof dlg.showModal === 'function') {
        dlg.showModal();
      } else {
        dlg.setAttribute('open', '');
      }
      input.value = '';
      schemaSearchSelectedIndex = 0;
      renderSchemaSearchResults('');
      input.focus();
    }

    function closeSchemaSearch() {
      const dlg = document.getElementById('schema-search-modal');
      if (dlg) {
        if (typeof dlg.close === 'function') dlg.close();
        else dlg.removeAttribute('open');
      }
    }

    function renderSchemaSearchResults(query) {
      query = (query || '').trim().toLowerCase();
      const resultsContainer = document.getElementById('schema-search-results');
      const countEl = document.getElementById('schema-search-count');

      if (!query) {
        // Show top tables by default
        schemaSearchResults = schemaSearchIndex.filter(it => it.kind === 'TBL' || it.kind === 'VIEW').slice(0, 30);
      } else {
        schemaSearchResults = schemaSearchIndex.filter(it => {
          if (it.fullName.toLowerCase().includes(query)) return true;
          if (it.table && it.table.toLowerCase().includes(query)) return true;
          if (it.column && it.column.toLowerCase().includes(query)) return true;
          return false;
        }).slice(0, 50);
      }

      countEl.textContent = `${schemaSearchResults.length} items`;
      if (schemaSearchResults.length === 0) {
        resultsContainer.innerHTML = `<div class="p-6 text-center text-slate-500">No tables, views, or columns found matching "<span class="text-slate-300">${escapeHtml(query)}</span>"</div>`;
        return;
      }

      if (schemaSearchSelectedIndex >= schemaSearchResults.length) {
        schemaSearchSelectedIndex = 0;
      }

      resultsContainer.innerHTML = schemaSearchResults.map((it, idx) => {
        const isSelected = idx === schemaSearchSelectedIndex;
        const kindBadge = it.kind === 'TBL'
          ? '<span class="bg-emerald-950 text-emerald-400 border border-emerald-800 px-1.5 py-0.5 rounded text-[10px] font-semibold">[TBL]</span>'
          : it.kind === 'VIEW'
          ? '<span class="bg-purple-950 text-purple-400 border border-purple-800 px-1.5 py-0.5 rounded text-[10px] font-semibold">[VIEW]</span>'
          : '<span class="bg-sky-950 text-sky-400 border border-sky-800 px-1.5 py-0.5 rounded text-[10px] font-semibold">[COL]</span>';

        const title = it.kind === 'COL'
          ? `<span class="font-mono font-bold text-white">${escapeHtml(it.column)}</span> <span class="text-slate-400 font-mono text-[11px]">${escapeHtml(it.type)}</span>`
          : `<span class="font-mono font-bold text-white">${escapeHtml(it.table)}</span>`;

        const location = `<span class="text-slate-400 font-mono text-[11px]">${escapeHtml(it.database)}${it.kind === 'COL' ? '.' + escapeHtml(it.table) : ''}</span>`;

        return `
          <div id="schema-item-${idx}" onclick="selectSchemaSearchResult(${idx}, false)" class="px-3 py-2 flex items-center justify-between hover:bg-slate-800/80 rounded cursor-pointer transition ${isSelected ? 'bg-slate-800 border-l-2 border-app-accent' : ''}">
            <div class="flex items-center space-x-2.5">
              ${kindBadge}
              <div>
                <div class="flex items-center space-x-1.5">${title}</div>
                <div class="text-[10px] text-slate-500">${location}</div>
              </div>
            </div>
            <div class="flex items-center space-x-2">
              <button onclick="event.stopPropagation(); selectSchemaSearchResult(${idx}, true)" class="text-slate-400 hover:text-sky-300 hover:bg-slate-700 px-2 py-1 rounded text-[11px] border border-slate-700 transition" title="Insert identifier into query editor">
                Insert ↵
              </button>
            </div>
          </div>
        `;
      }).join('');

      const activeEl = document.getElementById(`schema-item-${schemaSearchSelectedIndex}`);
      if (activeEl) {
        activeEl.scrollIntoView({ block: 'nearest' });
      }
    }

    async function selectSchemaSearchResult(index, isInsert = false) {
      if (index < 0 || index >= schemaSearchResults.length) return;
      const item = schemaSearchResults[index];
      closeSchemaSearch();

      if (isInsert) {
        // Insert identifier into active editor
        let identifier = '';
        if (item.kind === 'COL') {
          identifier = `${item.database}.${item.table}.${item.column}`;
        } else {
          identifier = `${item.database}.${item.table}`;
        }

        if (cmEditor) {
          cmEditor.replaceSelection(identifier);
          cmEditor.focus();
        } else {
          const ta = document.getElementById('sql-editor-textarea');
          if (ta) {
            ta.value += identifier;
          }
        }
        return;
      }

      // Open Table Viewer
      if (!activeConnection) {
        showNativeAlert('Please select or create a connection first.', 'No Connection');
        return;
      }

      // If item is in another database, switch database first
      const currentActiveDb = schemaCatalogCache?.database_name || activeConnection.database_name;
      if (item.database && currentActiveDb && item.database.toLowerCase() !== currentActiveDb.toLowerCase()) {
        try {
          await apiFetch(`/api/session/${activeConnection.id}/select-db`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ database: item.database })
          });
          await loadSchemaTree(activeConnection.id, item.database);
        } catch (e) {
          console.warn('Auto switch db failed:', e);
        }
      }

      openTableViewer(item.table);
    }

    // --- Visual Transaction Management (Roadmap 1.2) ---
    let isCurrentSessionTxActive = false;

    function updateTransactionStatusUI(txActive) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && txActive !== undefined) {
        activeTab.txActive = !!txActive;
      }
      isCurrentSessionTxActive = (activeTab && activeTab.txActive !== undefined) ? activeTab.txActive : !!txActive;
      const pill = document.getElementById('tab-tx-pill-container');
      if (pill) {
        if (isCurrentSessionTxActive) {
          pill.classList.remove('hidden');
          pill.classList.add('flex');
        } else {
          pill.classList.add('hidden');
          pill.classList.remove('flex');
        }
      }
    }

    async function commitActiveTransaction(event) {
      if (event) event.stopPropagation();
      await executeDirectSQL("COMMIT;");
    }

    async function rollbackActiveTransaction(event) {
      if (event) event.stopPropagation();
      await executeDirectSQL("ROLLBACK;");
    }

    async function executeDirectSQL(sqlText) {
      if (!activeConnection) return;
      try {
        const activeTab = tabs.find(t => t.id === activeTabId);
        const res = await apiFetch(`/api/session/${activeConnection.id}/execute`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sql: sqlText,
            tab_id: activeTab?.id || ''
          })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(data.error || 'Execution failed', 'Transaction Error');
          return;
        }
        updateTransactionStatusUI(data.tx_active);
        renderQuerySuccessBanner(sqlText, data.message || `${sqlText.trim()} executed`, data.execution_ms || 0, 0, null);
      } catch (err) {
        showNativeAlert(err.message, 'Transaction Error');
      }
    }

    // --- Query Stream 5k Batch Pager (ADR 0058) ---
    function hasExplicitSQLLimit(sql) {
      if (!sql || typeof sql !== 'string') return false;
      const cleaned = sql.replace(/--[^\r\n]*|\/\*[\s\S]*?\*\//g, '').trim().replace(/;+\s*$/, '');
      return /\bLIMIT\s+\d+(\s*,\s*\d+)?(\s+OFFSET\s+\d+)?\s*$/i.test(cleaned);
    }

    function wrapSQLWithPagination(sql, limit, offset) {
      const cleaned = (sql || '').trim().replace(/;+\s*$/, '');
      // ponytail: Query limit + 1 so backend cursor can probe the extra row and correctly determine has_more for continuous Next 5k paging past batch 2
      const fetchLimit = limit + 1;
      // ponytail: CTE queries in MySQL cannot be wrapped inside derived tables; preserve top-level WITH
      const cteMatch = cleaned.match(/^(\s*WITH\s+[\s\S]+?\))\s*(SELECT[\s\S]+)$/i);
      if (cteMatch) {
        return `${cteMatch[1]} SELECT * FROM (\n${cteMatch[2]}\n) AS _p LIMIT ${fetchLimit} OFFSET ${offset}`;
      }
      return `SELECT * FROM (\n${cleaned}\n) AS _p LIMIT ${fetchLimit} OFFSET ${offset}`;
    }

    function updateQueryStreamPagerUI(tab) {
      const pager = document.getElementById('query-stream-pager');
      if (!pager) return;

      if (!tab || tab.isTableViewer || tab.isProcesslist || tab.isCluster || tab.isFlightRecorder || tab.isVariables || (tab.multiResults && tab.multiResults.length > 1)) {
        pager.classList.add('hidden');
        pager.classList.remove('flex');
        return;
      }

      const offset = tab.streamBatchOffset || 0;
      const hasMore = !!tab.hasMoreStream;

      if (!hasMore && offset === 0) {
        pager.classList.add('hidden');
        pager.classList.remove('flex');
        return;
      }

      pager.classList.remove('hidden');
      pager.classList.add('flex');

      const batchNum = Math.floor(offset / 5000) + 1;
      const badge = document.getElementById('query-stream-page-badge');
      if (badge) {
        badge.textContent = `Batch ${batchNum}`;
      }

      const batchInfo = document.getElementById('query-stream-batch-info');
      if (batchInfo) {
        if (offset === 0) {
          batchInfo.textContent = '5,000+ rows';
        } else {
          const start = (offset + 1).toLocaleString();
          const currentRows = (tab.rawQueryRows && tab.rawQueryRows.length) || 0;
          const end = (offset + currentRows).toLocaleString();
          batchInfo.textContent = hasMore ? `${start}–${end}+ rows` : `${start}–${end} rows`;
        }
      }

      const btnPrev = document.getElementById('btn-stream-prev');
      if (btnPrev) {
        btnPrev.disabled = offset <= 0;
      }

      const btnNext = document.getElementById('btn-stream-next');
      if (btnNext) {
        btnNext.disabled = !hasMore;
      }
    }

    function loadNextStreamBatch() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.baseStreamSQL) return;
      const nextOffset = (activeTab.streamBatchOffset || 0) + 5000;
      const wrappedSQL = wrapSQLWithPagination(activeTab.baseStreamSQL, 5000, nextOffset);
      runActiveQuery(true, '', wrappedSQL, nextOffset);
    }

    function loadPreviousStreamBatch() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab || !activeTab.baseStreamSQL) return;
      const prevOffset = Math.max(0, (activeTab.streamBatchOffset || 0) - 5000);
      if (prevOffset === 0) {
        runActiveQuery(true, '', activeTab.baseStreamSQL, 0);
      } else {
        const wrappedSQL = wrapSQLWithPagination(activeTab.baseStreamSQL, 5000, prevOffset);
        runActiveQuery(true, '', wrappedSQL, prevOffset);
      }
    }

    // --- Query Execution & Safety Guard ---
    async function runActiveQuery(confirmed = false, challengeInput = '', overrideSQL = null, overrideOffset = null, fullScript = false) {
      // ponytail: Concurrency guard prevents duplicate simultaneous executions (e.g. double-click, DDL multi-trigger)
      if (isQueryExecuting) return;
      if (!activeConnection) {
        showNativeAlert('Please select or create a connection first.', 'No Connection');
        return;
      }

      const isBatchPaging = overrideSQL !== null && overrideOffset !== null;
      const sql = (overrideSQL !== null ? overrideSQL : getEditorSQL(fullScript)).trim();
      if (!sql) return;

      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetTabId = activeTab ? activeTab.id : activeTabId;
      const currentDb = (activeTab && activeTab.database) ? activeTab.database : (schemaCatalogCache?.database_name || activeConnection.database_name || '');

      if (activeTab) {
        if (isBatchPaging) {
          activeTab.streamBatchOffset = overrideOffset;
        } else {
          activeTab.baseStreamSQL = sql;
          activeTab.streamBatchOffset = 0;
          activeTab.hasMoreStream = false;
          // ponytail: Clear previous execution artifacts to ensure clean state isolation
          activeTab.rawQueryRows = null;
          activeTab.rawQueryCols = null;
          activeTab.rawQueryError = null;
          activeTab.rawQueryExec = null;
          activeTab.queryStats = '';
        }
      }
      updateQueryStreamPagerUI(activeTab);

      // ADR 0055: Bidirectional USE <database> synchronization
      if (!isBatchPaging) {
        const useMatch = sql.match(/^\s*USE\s+[`'"]?([\w\.\-]+)[`'"]?\s*;?$/i);
        if (useMatch) {
          const newDb = useMatch[1];
          if (activeTab) {
            activeTab.database = newDb;
            updateTabDatabasePill(activeTab);
            saveTabsState();
            showToast(`Tab database context switched to '${newDb}'`);
          }
        }
      }

      // ADR 0055: Intercept unconfirmed DDL for Target Database Affirmation
      if (!isBatchPaging && !confirmed && /^\s*(CREATE|ALTER)\s+(TABLE|VIEW|INDEX)/i.test(sql)) {
        const ddlMatch = sql.match(/^\s*(CREATE|ALTER)\s+(TABLE|VIEW|INDEX)\s+(?:IF\s+(?:NOT\s+)?EXISTS\s+)?([`'"\w\.\-]+)/i);
        let extractedTable = '';
        let extractedDB = '';
        if (ddlMatch && ddlMatch[3]) {
          const parts = ddlMatch[3].replace(/[`'"]/g, '').split('.');
          if (parts.length >= 2) {
            extractedDB = parts[parts.length - 2];
            extractedTable = parts[parts.length - 1];
          } else {
            extractedTable = parts[0];
          }
        }
        const targetDB = extractedDB || currentDb;
        const isMismatch = !!(extractedDB && currentDb && extractedDB.toLowerCase() !== currentDb.toLowerCase());
        const isProdOrStaging = activeConnection?.environment === 'Production' || activeConnection?.environment === 'Staging';
        openSafetyModal(sql, {
          risk_level: isMismatch || isProdOrStaging ? 'DANGEROUS' : 'WARNING',
          requires_affirmation: true,
          requires_challenge: isMismatch || isProdOrStaging,
          schema_scope_mismatch: isMismatch,
          target_table: extractedTable,
          target_database: targetDB,
          operation: (ddlMatch && ddlMatch[1].toUpperCase()) || 'DDL',
          reason: isMismatch
            ? `Schema Scope Mismatch: query explicitly targets database '${extractedDB}', but current tab is scoped to '${currentDb}'.`
            : `${(ddlMatch && ddlMatch[1].toUpperCase()) || 'DDL'} statement targeting database '${targetDB}'. Verify destination before execution.`
        });
        return;
      }

      if (activeTab) {
        activeTab.isExecuting = true;
        renderTabs();
      }
      setQueryRunning(true);

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/execute-stream`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sql: sql,
            confirmed: confirmed,
            challenge_input: challengeInput,
            database: currentDb,
            query_id: 'active',
            profile: enableQueryProfiling,
            limit: 5000,
            tab_id: targetTabId
          })
        });

        let errBody = null;
        if (res.status === 422) {
          errBody = await res.json().catch(() => ({ error: 'Execution failed' }));
          setQueryRunning(false);
          const originTab = tabs.find(t => t.id === targetTabId);
          if (originTab) originTab.isExecuting = false;
          renderTabs();
          if (errBody.blocked) {
            // Safety Guard Triggered!
            openSafetyModal(sql, errBody.inspection);
            return;
          }
        }

        if (!res.ok) {
          // The 422 body was already consumed above; reuse it instead of reading the stream twice.
          const data = errBody || await res.json().catch(() => ({ error: 'Execution failed' }));
          setQueryRunning(false);
          const originTab = tabs.find(t => t.id === targetTabId);
          if (originTab) originTab.isExecuting = false;
          renderTabs();
          const errMsg = data.error || 'Execution failed';
          if (originTab) {
            originTab.rawQueryError = { sql, message: errMsg };
            originTab.rawQueryRows = null;
            originTab.rawQueryCols = null;
            originTab.rawQueryExec = null;
            originTab.multiResults = null;
          }
          if (activeTabId === targetTabId) {
            renderMultiResultTabsUI(originTab);
            updateQueryStreamPagerUI(originTab);
            renderQueryErrorBanner(sql, errMsg);
          } else {
            showToast(`Query in '${originTab?.title || 'tab'}' failed: ${errMsg}`);
          }
          return;
        }

        const contentType = (res.headers && typeof res.headers.get === 'function' ? res.headers.get('content-type') : '') || '';
        let data = null;
        let doneResult = null;

        if (contentType.includes('application/x-ndjson') && res.body) {
          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let streamBuf = '';
          let streamedCols = null;
          let streamedRows = [];
          let streamedResults = [];
          let firstChunkPainted = false;
          let execResult = null;

          const parsed = extractSingleTableFromSQL(sql);
          const detectedTable = parsed ? parsed.table : null;
          const detectedDB = (parsed && parsed.database) || currentDb;

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            // ponytail: Abort stream reading immediately if originating tab was closed during streaming
            if (!tabs.some(t => t.id === targetTabId)) {
              try { await reader.cancel(); } catch (_) {}
              break;
            }
            streamBuf += decoder.decode(value, { stream: true });
            const lines = streamBuf.split('\n');
            streamBuf = lines.pop();

            for (const line of lines) {
              const trimmed = line.trim();
              if (!trimmed) continue;
              let item;
              try { item = JSON.parse(trimmed); } catch (e) { continue; }

              if (item.type === 'error' || item.error) {
                const errMsg = item.error || 'Query stream error';
                throw new Error(errMsg);
              } else if (item.type === 'meta') {
                const rIdx = item.result_index !== undefined ? item.result_index : 0;
                if (!streamedResults[rIdx]) {
                  streamedResults[rIdx] = { index: rIdx, type: 'query', sql: item.sql || '', columns: item.columns || [], rows: [] };
                } else {
                  streamedResults[rIdx].type = 'query';
                  streamedResults[rIdx].columns = item.columns || [];
                  if (item.sql) streamedResults[rIdx].sql = item.sql;
                }
                if (rIdx === 0) streamedCols = item.columns || [];
              } else if (item.type === 'chunk') {
                const rIdx = item.result_index !== undefined ? item.result_index : 0;
                if (!streamedResults[rIdx]) {
                  streamedResults[rIdx] = { index: rIdx, type: 'query', sql: '', columns: [], rows: [] };
                }
                if (Array.isArray(item.rows) && item.rows.length > 0) {
                  if (streamedResults[rIdx].rows.length < 25000) {
                    streamedResults[rIdx].rows.push(...item.rows);
                  }
                  // First micro-chunk instant paint (<50ms TTFR) - only if still viewing target tab
                  if (rIdx === 0) {
                    if (streamedRows.length < 25000) {
                      streamedRows.push(...item.rows);
                    }
                    if (!firstChunkPainted && streamedRows.length >= 20) {
                      firstChunkPainted = true;
                      if (activeTabId === targetTabId) {
                        renderGridResults(streamedCols, streamedRows, null, detectedTable, detectedDB);
                        const stats = document.getElementById('grid-result-stats');
                        if (stats) stats.textContent = `${streamedRows.length} rows (streaming...)`;
                      }
                    }
                  }
                }
              } else if (item.type === 'statement_done') {
                const rIdx = item.result_index !== undefined ? item.result_index : 0;
                if (streamedResults[rIdx]) {
                  streamedResults[rIdx].rowCount = item.row_count;
                  streamedResults[rIdx].executionMs = item.execution_ms;
                  streamedResults[rIdx].txActive = item.tx_active;
                  if (item.sql) streamedResults[rIdx].sql = item.sql;
                }
              } else if (item.type === 'exec') {
                const rIdx = item.result_index !== undefined ? item.result_index : 0;
                streamedResults[rIdx] = {
                  index: rIdx,
                  type: 'exec',
                  sql: item.sql || '',
                  message: item.message,
                  executionMs: item.execution_ms,
                  rowsAffected: item.rows_affected,
                  backupFile: item.backup_file,
                  backupWarning: item.backup_warning,
                  txActive: item.tx_active
                };
                if (rIdx === 0) execResult = item;
              } else if (item.type === 'done') {
                doneResult = item;
              }
            }
          }

          if (streamedCols && !doneResult && tabs.some(t => t.id === targetTabId)) {
            throw new Error('Query stream ended prematurely');
          }

          setQueryRunning(false);
          const originTab = tabs.find(t => t.id === targetTabId);
          if (originTab) originTab.isExecuting = false;
          renderTabs();

          if (streamedResults.length > 1) {
            data = {
              type: 'multi',
              results: streamedResults.map(r => ({
                type: r.type,
                index: r.index,
                sql: r.sql || '',
                columns: r.columns || [],
                rows: r.rows || [],
                row_count: r.rowCount !== undefined ? r.rowCount : (r.rows ? r.rows.length : 0),
                execution_ms: r.executionMs || 0,
                rows_affected: r.rowsAffected !== undefined ? r.rowsAffected : 0,
                message: r.message || '',
                backup_file: r.backupFile,
                backup_warning: r.backupWarning,
                tx_active: r.txActive
              })),
              execution_ms: doneResult ? doneResult.execution_ms : 0,
              tx_active: doneResult ? doneResult.tx_active : false
            };
          } else if (streamedCols) {
            data = {
              type: 'query',
              columns: streamedCols,
              rows: streamedRows,
              row_count: doneResult ? doneResult.row_count : streamedRows.length,
              execution_ms: doneResult ? doneResult.execution_ms : 0,
              tx_active: doneResult ? doneResult.tx_active : false,
              profile: doneResult ? doneResult.profile : null
            };
          } else if (execResult) {
            data = execResult;
          } else {
            data = doneResult || { type: 'exec', message: 'Query completed', execution_ms: 0, rows_affected: 0 };
          }
        } else {
          data = await res.json();
          setQueryRunning(false);
          const originTab = tabs.find(t => t.id === targetTabId);
          if (originTab) originTab.isExecuting = false;
          renderTabs();
        }

        if (data.results && data.results.length === 1) {
          const single = data.results[0];
          data = { ...single, tx_active: data.tx_active !== undefined ? data.tx_active : single.tx_active };
        }

        if (data.profile) {
          lastQueryProfile = data.profile;
          if (activeTabId === targetTabId) {
            const btnViewProf = document.getElementById('btn-view-profile');
            if (btnViewProf) btnViewProf.classList.remove('hidden');
            renderQueryProfile(data.profile);
          }
        } else {
          lastQueryProfile = null;
          if (activeTabId === targetTabId) {
            const btnViewProf = document.getElementById('btn-view-profile');
            if (btnViewProf) btnViewProf.classList.add('hidden');
          }
        }

        updateTransactionStatusUI(data.tx_active);

        const originTab = tabs.find(t => t.id === targetTabId);
        if (data.results && data.results.length > 1) {
          if (originTab) {
            originTab.multiResults = data.results;
            originTab.activeResultIndex = 0;
            originTab.hasMoreStream = false;
            originTab.streamBatchOffset = 0;
            originTab.multiResults.forEach(r => {
              r.streamBatchOffset = 0;
              const rCount = (r.rows && r.rows.length) || 0;
              r.has_more = rCount >= 5000;
            });
          }
          selectMultiResultTab(targetTabId, 0);
          if (activeTabId === targetTabId) {
            renderMultiResultTabsUI(originTab);
          } else if (originTab) {
            showToast(`Batch queries in '${originTab.title || 'tab'}' completed (${data.results.length} result sets)`);
          }
          if (/^\s*(CREATE|ALTER|DROP|RENAME)\b/i.test(sql) && activeConnection) {
            await refreshSchema();
          }
        } else {
          if (originTab) {
            originTab.multiResults = null;
            originTab.activeResultIndex = 0;
          }
          if (activeTabId === targetTabId) {
            renderMultiResultTabsUI(originTab);
          }

          if (data.type === 'query') {
            const parsed = extractSingleTableFromSQL(sql);
            const detectedTable = parsed ? parsed.table : null;
            const detectedDB = (parsed && parsed.database) || currentDb;
            const activeTab = originTab;

            const rawCount = (data.rows && data.rows.length) || 0;
            const hasExplicit = hasExplicitSQLLimit(activeTab?.baseStreamSQL || sql);
            const hasMore = !hasExplicit && Boolean(
              (doneResult && doneResult.has_more) ||
              (!doneResult && rawCount >= 5000)
            );

            if (activeTab) {
              activeTab.rawQueryRows = data.rows || [];
              activeTab.rawQueryCols = data.columns || [];
              activeTab.rawQueryError = null;
              activeTab.rawQueryExec = null;
              activeTab.queryFilters = {};
              activeTab.sortCol = '';
              activeTab.sortDir = '';
              activeTab.queryPage = 0;
              activeTab.queryTable = detectedTable;
              activeTab.queryDB = detectedDB;
              activeTab.hasMoreStream = hasMore;

              activeTab.queryStats = `${data.row_count} rows (${data.execution_ms}ms)`;
              const curOffset = activeTab.streamBatchOffset || 0;
              if (hasMore) {
                const startNum = (curOffset + 1).toLocaleString();
                const endNum = (curOffset + rawCount).toLocaleString();
                activeTab.queryStats = curOffset > 0 ? `Rows ${startNum}–${endNum}+ (${data.execution_ms}ms)` : `5,000+ rows (${data.execution_ms}ms)`;
              } else if (curOffset > 0) {
                const startNum = (curOffset + 1).toLocaleString();
                const endNum = (curOffset + rawCount).toLocaleString();
                activeTab.queryStats = `Rows ${startNum}–${endNum} (${data.execution_ms}ms)`;
              }
            }
            if (detectedDB && activeConnection && (typeof multiDBCatalogCache === 'object') && !multiDBCatalogCache[detectedDB.toLowerCase()]) {
              apiFetch(`/api/session/${activeConnection.id}/schema?db=${encodeURIComponent(detectedDB)}`)
                .then(r => r.ok ? r.json() : null)
                .then(s => {
                  if (s && multiDBCatalogCache) {
                    multiDBCatalogCache[detectedDB.toLowerCase()] = s;
                    if (activeTabId === targetTabId) {
                      renderGridResults(data.columns, data.rows, null, detectedTable, detectedDB, true);
                    }
                  }
                })
                .catch(() => {});
            }

            if (activeTabId === targetTabId) {
              selectedRowIndices.clear();
              updateSelectedRowsUI();
              window.lastFailedQuery = null;
              window.lastFailedError = null;
              renderGridResults(data.columns, data.rows, null, detectedTable, detectedDB);
              const statsEl = document.getElementById('grid-result-stats');
              if (statsEl && originTab) {
                statsEl.textContent = originTab.queryStats;
                if (hasMore) {
                  statsEl.classList.add('text-amber-400', 'font-medium');
                } else {
                  statsEl.classList.remove('text-amber-400', 'font-medium');
                }
              }
              updateQueryStreamPagerUI(originTab);
            } else {
              const originTab = tabs.find(t => t.id === targetTabId);
              if (originTab) {
                showToast(`Query in '${originTab.title || 'tab'}' completed (${data.row_count} rows)`);
              }
            }

            if (/^\s*(CREATE|ALTER|DROP|RENAME)\b/i.test(sql) && activeConnection) {
              await refreshSchema();
            }
          } else {
            const originTab = tabs.find(t => t.id === targetTabId);
            if (originTab) {
              originTab.rawQueryRows = null;
              originTab.rawQueryCols = null;
              originTab.rawQueryError = null;
              originTab.hasMoreStream = false;
              originTab.streamBatchOffset = 0;
              originTab.rawQueryExec = {
                sql: sql,
                message: data.message,
                executionMs: data.execution_ms,
                rowsAffected: data.rows_affected,
                backupFile: data.backup_file
              };
              originTab.queryStats = `${data.message} (${data.execution_ms}ms)`;
            }
            if (activeTabId === targetTabId) {
              updateQueryStreamPagerUI(originTab);
              renderQuerySuccessBanner(sql, data.message, data.execution_ms, data.rows_affected, data.backup_file);
              const statsEl = document.getElementById('grid-result-stats');
              if (statsEl && originTab) statsEl.textContent = originTab.queryStats;
            } else if (originTab) {
              showToast(`Query in '${originTab.title || 'tab'}' completed: ${data.message}`);
            }
            if (data.backup_warning) {
              showNativeAlert(`Query succeeded, but the automated backup is NOT complete:\n${data.backup_warning}${data.backup_file ? `\n\nPartial snapshot:\n${data.backup_file}` : ''}`, 'Safety Guard Backup Warning');
            } else if (data.backup_file) {
              showNativeAlert(`Query succeeded!\nAutomated Backup Snapshot created at:\n${data.backup_file}`, 'Safety Guard Backup');
            }
            await refreshSchema();
          }
        }
        // ponytail: refresh memory telemetry without overwriting ping latency with query execution duration
        updateFooterStats();
        const drawer = document.getElementById('audit-drawer');
        if (drawer && !drawer.classList.contains('hidden')) {
          setTimeout(loadAuditLogs, 150);
        }
      } catch (err) {
        setQueryRunning(false);
        const originTab = tabs.find(t => t.id === targetTabId);
        if (originTab) originTab.isExecuting = false;
        renderTabs();
        if (isBatchPaging && originTab && originTab.multiResults && originTab.multiResults[originTab.activeResultIndex]) {
          showToast(`Failed to fetch next batch: ${err.message}. Statement may depend on preceding statements; try 'Open in New Tab'.`, 'warning');
          if (activeTabId === targetTabId) {
            updateQueryStreamPagerUI(originTab);
            renderMultiResultTabsUI(originTab);
          }
        } else {
          if (originTab) {
            originTab.rawQueryError = { sql, message: err.message };
            originTab.rawQueryRows = null;
            originTab.rawQueryCols = null;
            originTab.rawQueryExec = null;
            originTab.multiResults = null;
          }
          if (activeTabId === targetTabId) {
            renderMultiResultTabsUI(originTab);
            updateQueryStreamPagerUI(originTab);
            renderQueryErrorBanner(sql, err.message);
          } else if (originTab) {
            showToast(`Query in '${originTab.title || 'tab'}' failed: ${err.message}`);
          }
        }
        const drawer = document.getElementById('audit-drawer');
        if (drawer && !drawer.classList.contains('hidden')) {
          setTimeout(loadAuditLogs, 150);
        }
      }
    }

    function renderQueryErrorBanner(sql, errMessage) {
      currentColumns = [];
      currentResults = [];
      activeGridTable = null;
      activeGridPKCol = null;
      activeGridSelectedRow = null;
      activeGridSelectedCol = null;
      activeGridSelectedTd = null;
      if (typeof closeCellDetailDrawer === 'function') closeCellDetailDrawer();
      selectedRowIndices.clear();
      updateSelectedRowsUI();
      const table = document.getElementById('results-table');
      if (table) {
        table.classList.add('table-empty');
        table.style.width = '100%';
      }
      const tbody = document.getElementById('grid-tbody');
      const thead = document.getElementById('grid-thead');
      thead.innerHTML = '<tr><th class="p-2 pl-4 text-red-400">Execution Error</th></tr>';
      tbody.innerHTML = `
        <tr>
          <td class="p-6">
            <div class="bg-red-950/30 border border-red-800/80 rounded-lg p-4 text-red-200 space-y-3">
              <div class="flex items-center justify-between">
                <span class="font-bold flex items-center space-x-2 text-red-400 text-sm">
                  ${renderIcon('x-circle', 'w-4 h-4 text-red-400')}
                  <span>Database Execution Error</span>
                </span>
                <button onclick="fixCurrentQueryWithAI()" class="bg-app-accent hover:bg-sky-400 text-slate-950 font-bold px-3 py-1.5 rounded text-xs transition flex items-center space-x-1.5 shadow">
          <svg class="w-4 h-4 text-indigo-400 shrink-0" aria-hidden="true"><use href="#icon-bot"/></svg>
                  <span>Fix with AI</span>
                </button>
              </div>
              <pre class="font-mono text-xs text-red-300 bg-red-950/60 p-3 rounded overflow-x-auto whitespace-pre-wrap">${escapeHtml(errMessage)}</pre>
            </div>
          </td>
        </tr>
      `;
      document.getElementById('grid-result-stats').textContent = 'Error executing query';
      window.lastFailedQuery = sql;
      window.lastFailedError = errMessage;
    }

    function renderQuerySuccessBanner(sql, message, executionMs, rowsAffected, backupFile) {
      currentColumns = [];
      currentResults = [];
      activeGridTable = null;
      activeGridPKCol = null;
      selectedRowIndices.clear();
      updateSelectedRowsUI();
      window.lastFailedQuery = null;
      window.lastFailedError = null;

      const table = document.getElementById('results-table');
      if (table) {
        table.classList.add('table-empty');
        table.style.width = '100%';
      }
      const thead = document.getElementById('grid-thead');
      const tbody = document.getElementById('grid-tbody');
      thead.innerHTML = '<tr><th class="p-2 pl-4 text-emerald-400 font-semibold w-full">Execution Succeeded</th></tr>';

      const statsBadge = (executionMs !== undefined && executionMs !== null)
        ? `<span class="bg-emerald-900/50 border border-emerald-700/60 text-emerald-300 px-2 py-0.5 rounded text-xs font-mono">⚡ ${executionMs}ms</span>`
        : '';
      const rowsBadge = (rowsAffected !== undefined && rowsAffected !== null)
        ? `<span class="bg-slate-800 border border-slate-700 text-slate-300 px-2 py-0.5 rounded text-xs font-mono inline-flex items-center gap-1">${renderIcon('activity', 'w-3 h-3 text-slate-400')}<span>${rowsAffected} row(s) affected</span></span>`
        : '';
      const backupHtml = backupFile ? `
        <div class="mt-3 p-3 bg-amber-950/30 border border-amber-800/60 rounded text-amber-200 text-xs flex items-start space-x-2">
                <svg class="w-4 h-4 text-emerald-400 shrink-0" aria-hidden="true"><use href="#icon-shield"/></svg>
          <div>
            <span class="font-semibold">Automated Backup Snapshot created:</span>
            <div class="font-mono text-[11px] text-amber-300/90 break-all mt-0.5">${escapeHtml(backupFile)}</div>
          </div>
        </div>
      ` : '';

      tbody.innerHTML = `
        <tr>
          <td class="p-6">
            <div class="bg-emerald-950/30 border border-emerald-800/80 rounded-lg p-4 text-emerald-200 space-y-3">
              <div class="flex items-center justify-between flex-wrap gap-2">
                <span class="font-bold flex items-center space-x-2 text-emerald-400 text-sm">
                  ${renderIcon('check-circle', 'w-4 h-4 text-emerald-400')}
                  <span>Command Executed Successfully</span>
                </span>
                <div class="flex items-center space-x-2">
                  ${statsBadge}
                  ${rowsBadge}
                </div>
              </div>
              <div class="font-mono text-xs text-emerald-300 bg-emerald-950/60 p-3 rounded border border-emerald-900/60">
                ${escapeHtml(message || 'Statement executed successfully.')}
              </div>
              ${backupHtml}
            </div>
          </td>
        </tr>
      `;
    }

    function renderMultiResultTabsUI(tab) {
      const bar = document.getElementById('multi-result-bar');
      const container = document.getElementById('multi-result-tabs');
      const actionsEl = document.getElementById('multi-result-actions');
      if (!bar || !container) return;
      if (!tab || !tab.multiResults || tab.multiResults.length <= 1) {
        bar.classList.add('hidden');
        container.innerHTML = '';
        if (actionsEl) actionsEl.innerHTML = '';
        return;
      }

      // ponytail: Option A ensures streaming batch pager is strictly hidden during multi-query executions
      const streamPager = document.getElementById('query-stream-pager');
      if (streamPager) {
        streamPager.classList.add('hidden');
        streamPager.classList.remove('flex');
      }

      bar.classList.remove('hidden');
      const activeIdx = tab.activeResultIndex || 0;
      container.innerHTML = tab.multiResults.map((res, idx) => {
        const isActive = idx === activeIdx;
        const isQuery = res.type === 'query';
        const count = res.row_count !== undefined ? res.row_count : (res.rowCount !== undefined ? res.rowCount : (res.rows ? res.rows.length : 0));
        const aff = res.rows_affected !== undefined ? res.rows_affected : (res.rowsAffected !== undefined ? res.rowsAffected : 0);
        const label = isQuery
          ? `Result ${idx + 1} (${count.toLocaleString()} rows)`
          : `Result ${idx + 1} (${aff} affected)`;
        const activeClass = isActive
          ? 'bg-slate-800 text-slate-100 border-app-accent font-semibold shadow-sm ring-1 ring-app-accent/40'
          : 'bg-slate-900/60 text-slate-400 border-slate-800 hover:text-slate-200 hover:bg-slate-850';

        return `<button type="button" onclick="selectMultiResultTab(activeTabId, ${idx})"
          class="px-2.5 py-0.5 text-xs rounded border transition flex items-center space-x-1.5 ${activeClass}">
          <span class="w-1.5 h-1.5 rounded-full ${isQuery ? 'bg-sky-400' : 'bg-emerald-400'}"></span>
          <span>${escapeHtml(label)}</span>
        </button>`;
      }).join('');

      if (actionsEl) {
        const activeRes = tab.multiResults[activeIdx];
        if (activeRes && activeRes.type === 'query') {
          const rawCount = (activeRes.rows && activeRes.rows.length) || 0;
          const isCapped = Boolean(activeRes.has_more || rawCount >= 5000);
          const cappedNotice = isCapped
            ? `<span class="text-amber-400 font-mono text-[11px] flex items-center space-x-1 shrink-0" title="Query result reached default 5,000 row cap">
                 <svg class="w-3.5 h-3.5 text-amber-400 shrink-0" aria-hidden="true"><use href="#icon-alert-triangle"/></svg>
                 <span>Capped at 5k rows</span>
               </span>`
            : '';
          const openBtn = activeRes.sql
            ? `<button type="button" onclick="openMultiResultInNewTab(activeTabId, ${activeIdx})"
                 class="px-2 py-0.5 text-[11px] rounded bg-slate-800 hover:bg-slate-700 text-sky-400 hover:text-white border border-slate-700 hover:border-sky-500/50 transition flex items-center space-x-1 shrink-0"
                 title="Open and run this statement in a new standalone tab with full paging & editor access">
                 <svg class="w-3 h-3 shrink-0" aria-hidden="true"><use href="#icon-external-link"/></svg>
                 <span>Open in New Tab</span>
               </button>`
            : '';
          actionsEl.innerHTML = `${cappedNotice}${openBtn}`;
        } else {
          actionsEl.innerHTML = '';
        }
      }
    }

    function openMultiResultInNewTab(tabId, index) {
      const tab = tabs.find(t => t.id === tabId) || tabs.find(t => t.id === activeTabId);
      if (!tab || !tab.multiResults || !tab.multiResults[index]) return;
      const res = tab.multiResults[index];
      const sql = (res.sql || '').trim();
      if (!sql) return;

      const title = `Result ${index + 1} (${tab.title || 'Query'})`;
      openInQueryEditor(sql, title);
      const newTab = tabs.find(t => t.id === activeTabId);
      if (newTab && tab.database) {
        newTab.database = tab.database;
        updateTabDatabasePill(newTab);
      }
      setTimeout(() => {
        runActiveQuery(false, '', sql);
      }, 50);
    }
    window.openMultiResultInNewTab = openMultiResultInNewTab;

    function selectMultiResultTab(tabId, index) {
      const tab = tabs.find(t => t.id === tabId) || tabs.find(t => t.id === activeTabId);
      if (!tab || !tab.multiResults || !tab.multiResults[index]) return;
      tab.activeResultIndex = index;
      const res = tab.multiResults[index];
      const parsed = extractSingleTableFromSQL(res.sql || '');
      const detectedTable = parsed ? parsed.table : null;
      const currentDb = (tab && tab.database) || schemaCatalogCache?.database_name || activeConnection?.database_name || '';
      const detectedDB = (parsed && parsed.database) || currentDb;

      if (res.type === 'query') {
        const rawCount = (res.rows && res.rows.length) || 0;
        tab.rawQueryRows = res.rows || [];
        tab.rawQueryCols = res.columns || [];
        tab.rawQueryError = null;
        tab.rawQueryExec = null;
        tab.queryTable = detectedTable;
        tab.queryDB = detectedDB;
        const count = res.row_count !== undefined ? res.row_count : (res.rowCount !== undefined ? res.rowCount : rawCount);
        const ms = res.execution_ms !== undefined ? res.execution_ms : (res.executionMs || 0);

        tab.baseStreamSQL = res.sql || '';
        tab.streamBatchOffset = 0;
        tab.hasMoreStream = false;
        res.has_more = rawCount >= 5000;

        tab.queryStats = rawCount >= 5000 ? `5,000+ rows (${ms}ms)` : `${count} rows (${ms}ms)`;

        if (activeTabId === tabId) {
          selectedRowIndices.clear();
          updateSelectedRowsUI();
          window.lastFailedQuery = null;
          window.lastFailedError = null;
          renderGridResults(tab.rawQueryCols, tab.rawQueryRows, null, detectedTable, detectedDB);
          const statsEl = document.getElementById('grid-result-stats');
          if (statsEl) {
            statsEl.textContent = tab.queryStats;
            if (rawCount >= 5000) {
              statsEl.classList.add('text-amber-400', 'font-medium');
            } else {
              statsEl.classList.remove('text-amber-400', 'font-medium');
            }
          }
          updateQueryStreamPagerUI(tab);
          renderMultiResultTabsUI(tab);
        }
      } else {
        const ms = res.execution_ms !== undefined ? res.execution_ms : (res.executionMs || 0);
        const aff = res.rows_affected !== undefined ? res.rows_affected : (res.rowsAffected || 0);
        const msg = res.message || 'Statement executed successfully.';
        const backupFile = res.backup_file || res.backupFile;

        tab.rawQueryRows = null;
        tab.rawQueryCols = null;
        tab.rawQueryError = null;
        tab.hasMoreStream = false;
        tab.streamBatchOffset = 0;
        tab.rawQueryExec = {
          sql: res.sql || '',
          message: msg,
          executionMs: ms,
          rowsAffected: aff,
          backupFile: backupFile
        };
        tab.queryStats = `${msg} (${ms}ms)`;

        if (activeTabId === tabId) {
          updateQueryStreamPagerUI(tab);
          renderQuerySuccessBanner(res.sql || '', msg, ms, aff, backupFile);
          const statsEl = document.getElementById('grid-result-stats');
          if (statsEl) {
            statsEl.textContent = tab.queryStats;
            statsEl.classList.remove('text-amber-400', 'font-medium');
          }
          renderMultiResultTabsUI(tab);
        }
      }
    }

    async function fixCurrentQueryWithAI() {
      const sql = window.lastFailedQuery || getEditorSQL();
      const err = window.lastFailedError || 'Unknown database error';
      const aiSidebar = document.getElementById('ai-sidebar');
      if (aiSidebar && aiSidebar.classList.contains('hidden')) {
        toggleAISidebar();
      }
      appendAIMessage('user', `Fix this SQL query:\n\`\`\`sql\n${sql}\n\`\`\`\nDatabase Error:\n${err}`);
      await callAI('fix_query', '', sql, { error_message: err });
    }

    async function inspectActiveQuery() {
      if (!activeConnection) {
        showNativeAlert('Please select or connect to a database first.', 'Safety Check');
        return;
      }
      const sql = getEditorSQL().trim();
      if (!sql) {
        showNativeAlert('Query editor is empty. Enter SQL to inspect.', 'Safety Check');
        return;
      }
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/inspect`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ sql })
        });
        const data = await res.json();
        openSafetyInspectionModal(data, sql);
      } catch (err) {
        showNativeAlert('Safety inspection failed: ' + err.message, 'Safety Check Error');
      }
    }

    function openSafetyInspectionModal(data, sql) {
      const dlg = document.getElementById('safety-inspection-dialog');
      if (!dlg) return;

      const riskBadge = document.getElementById('inspect-risk-badge');
      const risk = (data.risk_level || 'LOW').toUpperCase();
      riskBadge.textContent = risk;
      if (risk === 'HIGH') {
        riskBadge.className = 'px-2.5 py-0.5 rounded font-mono font-bold text-xs bg-red-900/60 text-red-300 border border-red-700';
      } else if (risk === 'MEDIUM') {
        riskBadge.className = 'px-2.5 py-0.5 rounded font-mono font-bold text-xs bg-amber-900/60 text-amber-300 border border-amber-700';
      } else {
        riskBadge.className = 'px-2.5 py-0.5 rounded font-mono font-bold text-xs bg-emerald-900/60 text-emerald-300 border border-emerald-700';
      }

      const destrEl = document.getElementById('inspect-destructive-text');
      if (data.is_destructive) {
        destrEl.innerHTML = `<span class="text-red-400 font-semibold inline-flex items-center gap-1.5">${renderIcon('alert-triangle', 'w-3.5 h-3.5 text-red-400')}<span>Yes (Alters schema or mass rows)</span></span>`;
      } else {
        destrEl.innerHTML = `<span class="text-emerald-400 font-semibold inline-flex items-center gap-1.5">${renderIcon('check-circle', 'w-3.5 h-3.5 text-emerald-400')}<span>No (Safe read-only operation)</span></span>`;
      }

      const challEl = document.getElementById('inspect-challenge-text');
      if (data.requires_challenge) {
        challEl.innerHTML = `<span class="text-amber-400 font-semibold inline-flex items-center gap-1.5">${renderIcon('alert-triangle', 'w-3.5 h-3.5 text-amber-400')}<span>Required (${escapeHtml(data.target_table || 'N/A')})</span></span>`;
      } else {
        challEl.innerHTML = '<span class="text-slate-400">Not required</span>';
      }

      const reasonBox = document.getElementById('inspect-reason-box');
      reasonBox.textContent = data.reason || (data.is_destructive ? 'Destructive query intercepted by Safety Guard.' : 'Deterministic AST evaluation: Statement is safe to execute.');

      const sqlBox = document.getElementById('inspect-sql-preview');
      if (sqlBox) sqlBox.textContent = sql;

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    let queryTimerInterval = null;
    let queryStartTime = 0;

    function setQueryRunning(isRunning, label = 'Executing...') {
      isQueryExecuting = !!isRunning;
      const cancelBtn = document.getElementById('cancel-query-btn');
      const status = document.getElementById('query-status-text');
      const runBtn = document.getElementById('btn-run-query');
      const runAllBtn = document.getElementById('btn-run-all-query');
      const explainBtn = document.getElementById('btn-explain-query');
      const explainIcon = document.getElementById('btn-explain-icon');
      const explainText = document.getElementById('btn-explain-text');
      const progressBar = document.getElementById('query-progress-bar');

      if (queryTimerInterval) {
        clearInterval(queryTimerInterval);
        queryTimerInterval = null;
      }

      if (runBtn) {
        runBtn.disabled = isRunning;
        if (isRunning) {
          runBtn.classList.add('opacity-50', 'cursor-not-allowed');
        } else {
          runBtn.classList.remove('opacity-50', 'cursor-not-allowed');
        }
      }

      if (runAllBtn) {
        runAllBtn.disabled = isRunning;
        if (isRunning) {
          runAllBtn.classList.add('opacity-50', 'cursor-not-allowed');
        } else {
          runAllBtn.classList.remove('opacity-50', 'cursor-not-allowed');
        }
      }

      if (explainBtn) {
        explainBtn.disabled = isRunning;
        if (isRunning) {
          explainBtn.classList.add('opacity-50', 'cursor-not-allowed');
        } else {
          explainBtn.classList.remove('opacity-50', 'cursor-not-allowed');
        }
      }

      if (isRunning) {
        queryStartTime = Date.now();
        // ponytail: query-progress-bar disabled to keep editor UI quiet; tab spinner + status toolbar provide sufficient feedback
        if (progressBar) progressBar.classList.add('hidden');

        const isExplain = label.toLowerCase().includes('explain');
        if (isExplain && explainIcon && explainText) {
          explainIcon.innerHTML = '<span class="animate-spin inline-block">⏳</span>';
          explainText.textContent = 'Explaining...';
        }

        if (status) {
          status.textContent = `${label} (0.0s)`;
        }
        if (cancelBtn) cancelBtn.classList.remove('hidden');

        queryTimerInterval = setInterval(() => {
          if (!isQueryExecuting) {
            clearInterval(queryTimerInterval);
            queryTimerInterval = null;
            return;
          }
          const elapsedSec = ((Date.now() - queryStartTime) / 1000).toFixed(1);
          if (status) status.textContent = `${label} (${elapsedSec}s)`;
        }, 100);
      } else {
        if (progressBar) progressBar.classList.add('hidden');

        if (explainIcon && explainText) {
          explainIcon.textContent = '⚡';
          explainText.textContent = 'Explain';
        }

        if (status && (status.textContent.startsWith('Executing') || status.textContent.startsWith('Explaining'))) {
          status.textContent = 'Ready';
        }
        if (cancelBtn) cancelBtn.classList.add('hidden');
      }
    }

    async function explainActiveQuery() {
      // ponytail: Concurrency guard prevents duplicate simultaneous explain dispatches
      if (isQueryExecuting) return;
      if (!activeConnection) {
        showNativeAlert('Please select or create a connection first.', 'No Connection');
        return;
      }
      const sql = getEditorSQL().trim();
      if (!sql) {
        showNativeAlert('Please enter or select a SQL query in the editor to explain.', 'Empty Query');
        return;
      }
      const currentDb = schemaCatalogCache?.database_name || activeConnection.database_name || '';

      setQueryRunning(true, 'Explaining...');

      // ponytail: 30s timeout guard to prevent UI hanging indefinitely on slow remote connections
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 30000);

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/explain`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ sql, database: currentDb }),
          signal: controller.signal
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(data.error || 'Failed to explain query plan', 'Explain Error');
          return;
        }
        openImpactModal(data, sql);
      } catch (err) {
        if (err.name === 'AbortError') {
          showNativeAlert('Explain request timed out after 30s. The query execution plan took too long to generate.', 'Timeout');
        } else {
          showNativeAlert('Explain request failed: ' + err.message, 'Network Error');
        }
      } finally {
        clearTimeout(timeoutId);
        setQueryRunning(false);
      }
    }

    async function cancelRunningQuery() {
      if (!activeConnection) return;
      await apiFetch(`/api/session/${activeConnection.id}/cancel`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query_id: 'active' })
      });
      setQueryRunning(false);
      tabs.forEach(t => { t.isExecuting = false; });
      renderTabs();
    }

    // --- Safety Challenge Modal (ADR 0055 Target Database Affirmation) ---
    function openSafetyModal(sql, inspection) {
      pendingDestructiveSQL = sql;
      pendingDestructiveResult = inspection;
      document.getElementById('challenge-reason').textContent = inspection.reason || '';
      document.getElementById('challenge-sql-preview').textContent = sql;
      document.getElementById('challenge-target-name').textContent = inspection.target_table || inspection.target_database || 'target';

      const activeTab = tabs.find(t => t.id === activeTabId);
      const targetDb = inspection.target_database || (activeTab && activeTab.database) || schemaCatalogCache?.database_name || activeConnection?.database_name || 'default';
      const targetDbNameEl = document.getElementById('challenge-target-db-name');
      if (targetDbNameEl) targetDbNameEl.textContent = targetDb;

      const mismatchTag = document.getElementById('challenge-mismatch-tag');
      if (mismatchTag) {
        if (inspection.schema_scope_mismatch) {
          mismatchTag.classList.remove('hidden');
        } else {
          mismatchTag.classList.add('hidden');
        }
      }

      const modalTitle = document.getElementById('challenge-modal-title');
      const confirmBtn = document.getElementById('challenge-confirm-btn');
      const inputGroup = document.getElementById('challenge-input-group');
      const inputEl = document.getElementById('challenge-input');
      inputEl.value = '';

      const isDevAffirmation = inspection.requires_affirmation && !inspection.requires_challenge && activeConnection?.environment === 'Development' && !inspection.schema_scope_mismatch;

      if (isDevAffirmation) {
        if (modalTitle) modalTitle.textContent = 'TARGET DATABASE AFFIRMATION (DDL)';
        if (inputGroup) inputGroup.classList.add('hidden');
        if (confirmBtn) {
          confirmBtn.className = 'bg-amber-600 hover:bg-amber-500 text-white font-bold px-4 py-1.5 text-xs rounded transition shadow cursor-pointer';
          confirmBtn.textContent = `Confirm & Execute on [${targetDb}]`;
        }
      } else {
        if (modalTitle) modalTitle.textContent = inspection.schema_scope_mismatch ? 'SAFETY GUARD: SCHEMA SCOPE MISMATCH DETECTED' : 'SAFETY GUARD: DESTRUCTIVE QUERY INTERCEPTED';
        if (inputGroup) inputGroup.classList.remove('hidden');
        if (confirmBtn) {
          confirmBtn.className = 'bg-app-danger hover:bg-red-600 text-white font-bold px-4 py-1.5 text-xs rounded transition shadow cursor-pointer';
          confirmBtn.textContent = 'Confirm & Execute (Danger)';
        }
        setTimeout(() => inputEl.focus(), 100);
      }

      document.getElementById('safety-challenge-modal').classList.remove('hidden');
    }

    function closeSafetyModal() {
      document.getElementById('safety-challenge-modal').classList.add('hidden');
      pendingDestructiveSQL = null;
      pendingDestructiveResult = null;
    }

    function confirmSafetyChallenge() {
      if (!pendingDestructiveResult) return;
      const isDevAffirmation = pendingDestructiveResult.requires_affirmation && !pendingDestructiveResult.requires_challenge && activeConnection?.environment === 'Development' && !pendingDestructiveResult.schema_scope_mismatch;

      let typed = '';
      if (!isDevAffirmation) {
        typed = document.getElementById('challenge-input').value.trim();
        const expectedTable = pendingDestructiveResult.target_table || '';
        const expectedDB = pendingDestructiveResult.target_database || '';
        const matchTable = expectedTable && typed.toLowerCase() === expectedTable.toLowerCase();
        const matchDB = expectedDB && typed.toLowerCase() === expectedDB.toLowerCase();
        if (!matchTable && !matchDB) {
          showNativeAlert(`Verification failed! You typed '${typed}', but expected '${expectedTable || expectedDB}'.`, 'Safety Challenge');
          return;
        }
      }
      const sqlToExecute = pendingDestructiveSQL;
      closeSafetyModal();
      runActiveQuery(true, typed, sqlToExecute);
    }

    // --- Impact Preview Modal & Visual Execution Tree ---
    let currentImpactRawSQL = '';
    let isImpactSQLFormatted = false;

    function updateImpactQuerySnippet() {
      const snippetEl = document.getElementById('impact-query-snippet');
      const formatText = document.getElementById('impact-query-format-text');
      const lenEl = document.getElementById('impact-query-length');
      if (!snippetEl) return;

      const text = isImpactSQLFormatted ? formatSQL(currentImpactRawSQL) : currentImpactRawSQL;
      snippetEl.textContent = text || '';
      if (formatText) formatText.textContent = isImpactSQLFormatted ? 'Raw' : 'Format';
      if (lenEl) lenEl.textContent = currentImpactRawSQL ? `${currentImpactRawSQL.length} chars` : '';
    }

    function toggleImpactQueryFormat() {
      isImpactSQLFormatted = !isImpactSQLFormatted;
      updateImpactQuerySnippet();
    }

    async function copyImpactQuery() {
      if (!currentImpactRawSQL) return;
      const text = isImpactSQLFormatted ? formatSQL(currentImpactRawSQL) : currentImpactRawSQL;
      const ok = await copyTextToClipboard(text);
      if (ok) {
        showToast('Query copied to clipboard', 'success');
      }
    }

    function openImpactQueryInEditor() {
      if (!currentImpactRawSQL) return;
      const activeTab = tabs.find(t => t.id === activeTabId);
      const isQueryTab = activeTab && !activeTab.isTableViewer && !activeTab.isProcesslist && !activeTab.isCluster && !activeTab.isFlightRecorder && !activeTab.isVariables && !activeTab.isNote;
      if (!isQueryTab) {
        addNewQueryTab();
      }
      setEditorSQL(currentImpactRawSQL);
      const currentTab = tabs.find(t => t.id === activeTabId);
      if (currentTab) currentTab.sql = currentImpactRawSQL;
      if (cmEditor) cmEditor.focus();
      closeImpactModal();
      showToast('Query loaded into Query Editor');
    }

    async function copyImpactRawJSON() {
      const rawPlanEl = document.getElementById('impact-raw-plan');
      if (rawPlanEl && rawPlanEl.textContent) {
        const ok = await copyTextToClipboard(rawPlanEl.textContent);
        if (ok) {
          showToast('Raw execution plan copied to clipboard', 'success');
        }
      }
    }

    function switchImpactTab(tab) {
      const tableBtn = document.getElementById('impact-tab-table-btn');
      const treeBtn  = document.getElementById('impact-tab-tree-btn');
      const advisorBtn = document.getElementById('impact-tab-advisor-btn');
      const rawBtn   = document.getElementById('impact-tab-raw-btn');
      const tableView = document.getElementById('impact-table-view');
      const treeView  = document.getElementById('impact-tree-view');
      const advisorView = document.getElementById('impact-advisor-view');
      const rawView   = document.getElementById('impact-raw-view');
      if (!tableBtn || !rawBtn || !tableView || !rawView) return;

      const active = 'px-3 py-1.5 rounded-md font-semibold bg-sky-500/20 text-sky-300 border border-sky-500/40 shadow-sm transition flex items-center space-x-1.5';
      const inactive = 'px-3 py-1.5 rounded-md font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800/40 border border-transparent transition flex items-center space-x-1.5';

      tableBtn.className = inactive;
      if (treeBtn) treeBtn.className = inactive;
      if (advisorBtn) advisorBtn.className = inactive;
      rawBtn.className   = inactive;

      tableView.classList.add('hidden');
      if (treeView) treeView.classList.add('hidden');
      if (advisorView) advisorView.classList.add('hidden');
      rawView.classList.add('hidden');

      if (tab === 'table') {
        tableBtn.className = active;
        tableView.classList.remove('hidden');
      } else if (tab === 'tree') {
        if (treeBtn) treeBtn.className = active;
        if (treeView) treeView.classList.remove('hidden');
      } else if (tab === 'advisor') {
        if (advisorBtn) advisorBtn.className = active;
        if (advisorView) advisorView.classList.remove('hidden');
      } else {
        rawBtn.className = active;
        rawView.classList.remove('hidden');
      }
    }

    function openImpactModal(planData, sql) {
      currentImpactRawSQL = sql || '';
      isImpactSQLFormatted = false;
      updateImpactQuerySnippet();

      const rawPlanEl = document.getElementById('impact-raw-plan');
      if (rawPlanEl) {
        if (planData && planData.raw_json) {
          try {
            const parsed = JSON.parse(planData.raw_json);
            rawPlanEl.textContent = JSON.stringify(parsed, null, 2);
          } catch {
            rawPlanEl.textContent = planData.raw_json;
          }
        } else {
          rawPlanEl.textContent = JSON.stringify(planData, null, 2);
        }
      }

      const tbody = document.getElementById('impact-plan-tbody');
      const emptyState = document.getElementById('impact-empty-state');
      const summaryBadge = document.getElementById('impact-summary-badge');
      if (tbody) tbody.innerHTML = '';

      const rows = planData && Array.isArray(planData.rows) ? planData.rows : [];
      const treeBadge = document.getElementById('impact-tree-badge');
      if (treeBadge) {
        treeBadge.textContent = rows.length > 0 ? rows.length : (planData && planData.raw_json ? '1+' : '0');
      }

      // Update engine badge in header
      const engineBadge = document.getElementById('impact-plan-engine-badge');
      if (engineBadge) {
        const driver = (activeConnection && activeConnection.driver) ? activeConnection.driver.toUpperCase() : 'OPTIMIZER';
        engineBadge.textContent = driver;
      }

      if (rows.length === 0) {
        if (emptyState) emptyState.classList.remove('hidden');
        if (summaryBadge) summaryBadge.innerHTML = '<span class="text-slate-500 italic">0 execution steps returned</span>';
      } else {
        if (emptyState) emptyState.classList.add('hidden');
        let hasFullTableScan = false;
        let totalRows = 0;

        rows.forEach((r, idx) => {
          totalRows += (r.rows || 0);
          const tr = document.createElement('tr');
          tr.className = 'hover:bg-slate-800/50 transition border-b border-slate-800/60';

          // Access Type badge styling
          let typeBadge = `<span class="text-slate-400 font-medium">${escapeHtml(r.type || '-')}</span>`;
          const upperType = (r.type || '').toUpperCase();
          if (upperType === 'ALL') {
            hasFullTableScan = true;
            typeBadge = `<span class="bg-red-950/80 text-red-300 border border-red-700/80 px-2 py-0.5 rounded text-[10px] font-bold shadow-sm inline-flex items-center gap-1"><span>⚠️</span><span>ALL (Full Scan)</span></span>`;
          } else if (upperType === 'INDEX') {
            typeBadge = `<span class="bg-amber-950/80 text-amber-300 border border-amber-700/80 px-2 py-0.5 rounded text-[10px] font-semibold">index (Full)</span>`;
          } else if (['RANGE', 'REF', 'EQ_REF', 'CONST', 'SYSTEM'].includes(upperType)) {
            typeBadge = `<span class="bg-emerald-950/80 text-emerald-300 border border-emerald-700/80 px-2 py-0.5 rounded text-[10px] font-semibold inline-flex items-center gap-1"><span>✓</span><span>${escapeHtml(r.type)}</span></span>`;
          } else if (upperType.includes('SCAN') || upperType.includes('SEARCH')) {
            const isSearch = upperType.includes('SEARCH');
            typeBadge = `<span class="${isSearch ? 'bg-sky-950 text-sky-300 border-sky-800' : 'bg-red-950 text-red-300 border-red-800'} border px-2 py-0.5 rounded text-[10px] font-semibold">${escapeHtml(r.type)}</span>`;
          }

          const filteredText = typeof r.filtered === 'number' && r.filtered > 0 ? `${r.filtered.toFixed(1)}%` : '-';
          const keyText = r.key ? `<span class="text-sky-300 font-semibold inline-flex items-center gap-1"><span>🔑</span><span>${escapeHtml(r.key)}</span></span>` : '<span class="text-slate-600">NULL</span>';

          // Parse extra into readable pills
          let extraHtml = '-';
          if (r.extra) {
            const extraParts = r.extra.split(';').map(s => s.trim()).filter(Boolean);
            extraHtml = extraParts.map(part => {
              const up = part.toUpperCase();
              if (up.includes('FILESORT')) return `<span class="bg-amber-950 text-amber-300 border border-amber-800 px-1.5 py-0.2 rounded text-[10px]">⚠ filesort</span>`;
              if (up.includes('TEMPORARY')) return `<span class="bg-red-950 text-red-300 border border-red-800 px-1.5 py-0.2 rounded text-[10px]">⚠ temporary</span>`;
              if (up.includes('USING INDEX')) return `<span class="bg-emerald-950 text-emerald-300 border border-emerald-800 px-1.5 py-0.2 rounded text-[10px]">✓ using index</span>`;
              return `<span class="text-slate-400">${escapeHtml(part)}</span>`;
            }).join(' ');
          }

          tr.innerHTML = `
            <td class="p-2.5 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${idx + 1}</td>
            <td class="p-2.5 text-slate-300">${escapeHtml(r.select_type || '-')}</td>
            <td class="p-2.5 font-bold text-white"><span class="text-slate-500 font-normal mr-1">🗄️</span>${escapeHtml(r.table || '-')}</td>
            <td class="p-2.5">${typeBadge}</td>
            <td class="p-2.5">${keyText}</td>
            <td class="p-2.5 text-right font-semibold text-slate-200">${(r.rows || 0).toLocaleString()}</td>
            <td class="p-2.5 text-right text-slate-400">${filteredText}</td>
            <td class="p-2.5 text-slate-300 max-w-xs">${extraHtml}</td>
          `;
          if (tbody) tbody.appendChild(tr);
        });

        if (summaryBadge) {
          if (hasFullTableScan) {
            summaryBadge.innerHTML = `<span class="text-amber-400 font-semibold inline-flex items-center gap-1.5">${renderIcon('alert-triangle', 'w-4 h-4 text-amber-400')}<span>Full table scan detected!</span></span> <span class="text-slate-400">Review recommendations in</span> <button onclick="switchImpactTab('advisor')" class="text-amber-300 hover:underline font-bold">Index Advisor</button>`;
          } else {
            summaryBadge.innerHTML = `<span class="text-emerald-400 font-semibold inline-flex items-center gap-1">${renderIcon('check', 'w-4 h-4 text-emerald-400')}<span>${rows.length} execution step${rows.length > 1 ? 's' : ''} planned</span></span> <span class="text-slate-500">(~${totalRows.toLocaleString()} rows examined)</span>`;
          }
        }
      }

      // Render advisor recommendations
      renderAdvisorContent(planData && planData.advisor);

      // Render rich visual execution tree (seamlessly supports raw_json AND rows fallback!)
      renderPlanTree(planData);

      switchImpactTab('table');
      document.getElementById('impact-preview-modal').classList.remove('hidden');
    }

    function renderAdvisorContent(adv) {
      const badge = document.getElementById('impact-advisor-badge');
      const bloatContainer = document.getElementById('impact-advisor-bloat-container');
      const list = document.getElementById('impact-advisor-list');
      const empty = document.getElementById('impact-advisor-empty');
      if (!list) return;

      if (bloatContainer) bloatContainer.innerHTML = '';
      list.innerHTML = '';

      if (!adv || (!adv.recommendations?.length && !adv.bloat_warnings?.length)) {
        if (badge) badge.classList.add('hidden');
        if (empty) empty.classList.remove('hidden');
        return;
      }

      if (empty) empty.classList.add('hidden');

      const recCount = adv.recommendations ? adv.recommendations.length : 0;
      if (badge) {
        if (recCount > 0) {
          badge.textContent = recCount;
          badge.classList.remove('hidden');
        } else {
          badge.classList.add('hidden');
        }
      }

      // 1. Render Bloat Warnings (prominent amber alert banner)
      if (bloatContainer && adv.bloat_warnings && adv.bloat_warnings.length > 0) {
        adv.bloat_warnings.forEach(b => {
          const alert = document.createElement('div');
          alert.className = 'bg-amber-950/40 border border-amber-800/80 rounded p-3 text-xs text-amber-200 flex items-start space-x-2.5';
          alert.innerHTML = `
            <span class="text-base leading-none">⚠️</span>
            <div class="flex-1">
              <div class="font-bold text-amber-300">Index Bloat Warning (${escapeHtml(b.table)}):</div>
              <div class="mt-0.5 text-slate-300">${escapeHtml(b.warning)}</div>
            </div>
          `;
          bloatContainer.appendChild(alert);
        });
      }

      // 2. Render Recommendations
      if (adv.recommendations && adv.recommendations.length > 0) {
        adv.recommendations.forEach((rec, idx) => {
          const card = document.createElement('div');
          card.className = 'border border-slate-800 rounded bg-slate-900/60 p-3 space-y-2.5';

          let esrPills = '';
          if (rec.equality_columns && rec.equality_columns.length > 0) {
            esrPills += `<span class="bg-emerald-950 text-emerald-400 border border-emerald-800 px-1.5 py-0.5 rounded text-[10px] font-semibold" title="Equality Condition">Equal: ${rec.equality_columns.map(escapeHtml).join(', ')}</span> `;
          }
          if (rec.sort_columns && rec.sort_columns.length > 0) {
            esrPills += `<span class="bg-amber-950 text-amber-400 border border-amber-800 px-1.5 py-0.5 rounded text-[10px] font-semibold" title="Sort/Group by Condition">Sort: ${rec.sort_columns.map(escapeHtml).join(', ')}</span> `;
          }
          if (rec.range_columns && rec.range_columns.length > 0) {
            esrPills += `<span class="bg-sky-950 text-sky-400 border border-sky-800 px-1.5 py-0.5 rounded text-[10px] font-semibold" title="Range / Inequality Condition">Range: ${rec.range_columns.map(escapeHtml).join(', ')}</span> `;
          }

          let redundancyNote = '';
          if (rec.is_redundant) {
            redundancyNote = `<div class="text-[11px] text-amber-400 bg-amber-950/30 border border-amber-900/50 rounded px-2 py-1">⚠ Redundant: ${escapeHtml(rec.redundant_reason)}</div>`;
          } else if (rec.replace_suggestion) {
            redundancyNote = `<div class="text-[11px] text-sky-300 bg-sky-950/30 border border-sky-900/50 rounded px-2 py-1">💡 Consolidation: ${escapeHtml(rec.replace_suggestion)}</div>`;
          }

          card.innerHTML = `
            <div class="flex items-center justify-between">
              <div class="flex items-center space-x-2">
                <span class="px-1.5 py-0.5 bg-slate-800 text-slate-300 text-[10px] font-bold rounded uppercase">Table: ${escapeHtml(rec.table)}</span>
                <span class="font-mono text-xs font-semibold text-white">${escapeHtml(rec.suggested_index)}</span>
              </div>
              <div class="flex items-center space-x-1">${esrPills}</div>
            </div>
            <div class="text-xs text-slate-300">${escapeHtml(rec.reason || '')}</div>
            ${rec.estimated_benefit ? `<div class="text-[11px] text-emerald-400">⚡ ${escapeHtml(rec.estimated_benefit)}</div>` : ''}
            ${redundancyNote}
            <div class="bg-slate-950 border border-slate-800 rounded p-2 flex items-center justify-between">
              <code class="font-mono text-[11px] text-sky-300 select-all overflow-x-auto whitespace-pre">${escapeHtml(rec.safe_ddl)}</code>
              <div class="flex items-center space-x-1 ml-2 flex-shrink-0">
                <button type="button" data-ddl="${escapeHtml(rec.safe_ddl)}" onclick="copyAdvisorDDL(this)" class="px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded text-[11px] transition flex items-center space-x-1" title="Copy Index DDL">
                  <span class="advisor-copy-icon">${renderIcon('copy', 'w-3 h-3 text-slate-300')}</span><span class="advisor-copy-label">Copy</span>
                </button>
                <button type="button" data-ddl="${escapeHtml(rec.safe_ddl)}" onclick="applyAdvisorDDLToEditor(this.getAttribute('data-ddl'))" class="px-2 py-1 bg-sky-950 hover:bg-sky-900 text-sky-300 border border-sky-800 rounded text-[11px] transition flex items-center space-x-1">
                  ${renderIcon('zap', 'w-3 h-3 text-sky-400')}<span>Editor</span>
                </button>
              </div>
            </div>
          `;
          list.appendChild(card);
        });
      } else if (!adv.bloat_warnings || adv.bloat_warnings.length === 0) {
        if (empty) empty.classList.remove('hidden');
      }
    }

    async function copyAdvisorDDL(btn) {
      if (!btn) return;
      const ddl = btn.getAttribute('data-ddl');
      if (!ddl) return;
      const ok = await copyTextToClipboard(ddl);
      if (ok) {
        showToast('Index DDL copied to clipboard', 'success');
        const iconSpan = btn.querySelector('.advisor-copy-icon');
        const labelSpan = btn.querySelector('.advisor-copy-label');
        if (iconSpan && labelSpan) {
          const oldIcon = iconSpan.textContent;
          const oldLabel = labelSpan.textContent;
          iconSpan.innerHTML = renderIcon('check', 'w-3 h-3 text-emerald-400');
          labelSpan.textContent = 'Copied!';
          btn.classList.add('text-emerald-400');
          setTimeout(() => {
            iconSpan.innerHTML = renderIcon('copy', 'w-3 h-3 text-slate-300');
            labelSpan.textContent = 'Copy';
            btn.classList.remove('text-emerald-400');
          }, 1500);
        }
      } else {
        showNativeAlert('Failed to copy index DDL to clipboard.', 'Clipboard Error');
      }
    }

    function applyAdvisorDDLToEditor(ddl) {
      if (!ddl) return;
      const activeTab = tabs.find(t => t.id === activeTabId);
      const isQueryTab = activeTab && !activeTab.isTableViewer && !activeTab.isProcesslist && !activeTab.isCluster && !activeTab.isFlightRecorder && !activeTab.isVariables && !activeTab.isNote;
      if (!isQueryTab) {
        addNewQueryTab();
      }
      setEditorSQL(ddl);
      const currentTab = tabs.find(t => t.id === activeTabId);
      if (currentTab) currentTab.sql = ddl;
      if (cmEditor) cmEditor.focus();
      closeImpactModal();
      showToast('Index DDL inserted into Query Editor');
    }

    function closeImpactModal() {
      document.getElementById('impact-preview-modal').classList.add('hidden');
    }

    // --- Rich Visual Query Plan Tree Renderer ---
    // ponytail: zero-dependency visual graph; renders DAG node cards from either JSON plan (PG/MySQL) or Tabular Rows (MySQL/MariaDB/SQLite).
    function renderPlanTree(planData) {
      const canvas = document.getElementById('impact-tree-canvas');
      const empty  = document.getElementById('impact-tree-empty');
      const summaryBar = document.getElementById('impact-tree-summary-bar');
      if (!canvas) return;
      canvas.innerHTML = '';

      if (!planData) {
        if (empty) empty.classList.remove('hidden');
        if (summaryBar) summaryBar.innerHTML = '';
        return;
      }

      const rows = Array.isArray(planData.rows) ? planData.rows : [];
      let rawJSON = planData.raw_json;
      let parsedJSON = null;
      if (rawJSON) {
        try { parsedJSON = JSON.parse(rawJSON); } catch { parsedJSON = null; }
      }

      const isPG = Array.isArray(parsedJSON) && parsedJSON[0] && parsedJSON[0]['Plan'];
      const isMySQLJSON = parsedJSON && parsedJSON['query_block'];

      // Helper: Node card generator
      function makeNodeCard(node) {
        let cardBorder = 'border-slate-800 bg-slate-900/90 shadow-slate-950/40';
        if (node.isDanger) cardBorder = 'border-red-600/80 bg-gradient-to-br from-red-950/30 to-slate-900 shadow-red-950/20';
        else if (node.isWarning) cardBorder = 'border-amber-600/80 bg-gradient-to-br from-amber-950/30 to-slate-900 shadow-amber-950/20';
        else if (node.isOptimal) cardBorder = 'border-emerald-600/70 bg-gradient-to-br from-emerald-950/30 to-slate-900 shadow-emerald-950/20';
        else if (node.isSpecial) cardBorder = 'border-sky-600/70 bg-gradient-to-br from-sky-950/30 to-slate-900 shadow-sky-950/20';

        let extraHtml = '';
        if (node.extraPills && node.extraPills.length > 0) {
          extraHtml = `<div class="flex flex-wrap gap-1 mt-2.5 pt-2 border-t border-slate-800/80">` +
            node.extraPills.map(p => `<span class="${p.cls || 'bg-slate-800 text-slate-300 border-slate-700'} border px-1.5 py-0.5 rounded text-[10px] font-medium inline-flex items-center gap-1">${escapeHtml(p.text)}</span>`).join('') +
            `</div>`;
        }

        let filterHtml = '';
        if (node.filterExpr) {
          filterHtml = `<div class="mt-2 text-[10px] font-mono text-slate-400 bg-slate-950/70 p-1.5 rounded border border-slate-800/80 truncate" title="${escapeHtml(node.filterExpr)}"><span class="text-slate-500">filter: </span>${escapeHtml(node.filterExpr)}</div>`;
        }

        return `
          <div class="rounded-xl border ${cardBorder} p-3.5 shadow-lg transition hover:border-slate-500 w-full max-w-xl mx-auto">
            <div class="flex items-center justify-between gap-2 border-b border-slate-800/80 pb-2 mb-2">
              <div class="flex items-center space-x-1.5">
                <span class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-slate-800 text-slate-300 border border-slate-700">Step ${node.step}</span>
                <span class="text-xs font-bold text-white tracking-wide">${escapeHtml(node.operationTitle || 'Table Access')}</span>
              </div>
              <span class="${node.badgeCls} px-2 py-0.5 rounded text-[10px] font-bold border shadow-sm flex items-center gap-1">
                ${node.badgeIcon ? `<span>${node.badgeIcon}</span>` : ''}
                <span>${escapeHtml(node.badgeText)}</span>
              </span>
            </div>

            <div class="space-y-2">
              <div class="flex items-center justify-between text-xs">
                <div class="flex items-center space-x-1.5">
                  <span class="text-slate-400 text-sm">🗄️</span>
                  <span class="font-bold text-white">${escapeHtml(node.table || '(derived/subquery)')}</span>
                  ${node.alias && node.alias !== node.table ? `<span class="text-slate-500 text-[11px] font-mono">as ${escapeHtml(node.alias)}</span>` : ''}
                </div>
                <div class="text-[11px] font-mono text-slate-400">
                  ${node.keyUsed ? `<span class="text-sky-300 font-semibold flex items-center gap-1"><span>🔑</span><span>${escapeHtml(node.keyUsed)}</span></span>` : (node.isDanger ? `<span class="text-red-400 font-semibold">⚠ No index</span>` : `<span class="text-slate-500">None</span>`)}
                </div>
              </div>

              <!-- Metrics Strip -->
              <div class="grid grid-cols-2 sm:grid-cols-3 gap-1.5 pt-0.5 text-[11px]">
                <div class="bg-slate-950/70 border border-slate-800/80 rounded px-2 py-1">
                  <span class="text-slate-500 block text-[9px] uppercase tracking-wider">Examined</span>
                  <span class="font-mono font-bold text-slate-200">~${(node.rows || 0).toLocaleString()} rows</span>
                </div>
                <div class="bg-slate-950/70 border border-slate-800/80 rounded px-2 py-1">
                  <span class="text-slate-500 block text-[9px] uppercase tracking-wider">Filtered</span>
                  <span class="font-mono font-bold ${(node.filtered != null && node.filtered < 50) ? 'text-amber-400' : 'text-emerald-400'}">${node.filtered != null ? node.filtered.toFixed(1) + '%' : '-'}</span>
                </div>
                <div class="bg-slate-950/70 border border-slate-800/80 rounded px-2 py-1 col-span-2 sm:col-span-1">
                  <span class="text-slate-500 block text-[9px] uppercase tracking-wider">${node.costLabel || 'Select Type'}</span>
                  <span class="font-mono font-semibold text-sky-300 truncate block">${node.costVal || escapeHtml(node.selectType || '-')}</span>
                </div>
              </div>

              ${filterHtml}
              ${extraHtml}
            </div>
          </div>
        `;
      }

      function makeFlowConnector(yieldRows) {
        return `
          <div class="flex flex-col items-center my-1 select-none">
            <div class="w-0.5 h-3 bg-gradient-to-b from-slate-700 to-sky-500"></div>
            <div class="flex items-center space-x-1.5 px-3 py-0.5 bg-slate-900 border border-slate-700/80 rounded-full text-[10px] text-slate-300 shadow-sm my-0.5">
              <span class="text-sky-400 font-bold">↓</span>
              <span>passes ~${(yieldRows || 0).toLocaleString()} rows</span>
            </div>
            <div class="w-0.5 h-3 bg-gradient-to-b from-sky-500 to-slate-700"></div>
          </div>
        `;
      }

      function makeTerminalNode(totalRows) {
        return `
          <div class="w-full max-w-xl mx-auto mt-2 p-3 rounded-xl bg-emerald-950/20 border border-emerald-800/50 flex items-center justify-between text-xs shadow-md">
            <div class="flex items-center space-x-2.5">
              <div class="w-7 h-7 rounded-lg bg-emerald-500/10 border border-emerald-500/25 flex items-center justify-center text-emerald-400 text-sm">🏁</div>
              <div>
                <div class="font-bold text-emerald-300">Client Result Set</div>
                <div class="text-[11px] text-slate-400">Query execution pipeline finished</div>
              </div>
            </div>
            <div class="text-right">
              <span class="font-mono font-bold text-white text-sm">~${(totalRows || 0).toLocaleString()}</span>
              <span class="text-slate-400 text-[10px] block">output rows</span>
            </div>
          </div>
        `;
      }

      // Check if we can build from PostgreSQL JSON
      if (isPG) {
        if (empty) empty.classList.add('hidden');
        let stepCount = 0;
        let totalRows = 0;
        let fullScanCount = 0;

        function traversePG(stmtPlan) {
          const cards = [];
          function walk(node, depth) {
            stepCount++;
            const type = (node['Node Type'] || '').toLowerCase();
            const isDanger = type === 'seq scan';
            const isOptimal = type.includes('index scan') || type.includes('index only');
            const isWarning = type.includes('nested loop') || (node['Temp Written Blocks'] > 0);
            if (isDanger) fullScanCount++;

            const rowsExamined = node['Actual Rows'] != null ? node['Actual Rows'] : (node['Plan Rows'] || 0);
            totalRows += rowsExamined;

            let badgeCls = 'bg-slate-800 text-slate-300 border-slate-700';
            let badgeIcon = '⚙️';
            let badgeText = node['Node Type'] || 'Node';
            if (isDanger) {
              badgeCls = 'bg-red-950/90 text-red-300 border-red-700';
              badgeIcon = '⚠️';
              badgeText = 'Seq Scan';
            } else if (isOptimal) {
              badgeCls = 'bg-emerald-950/90 text-emerald-300 border-emerald-700';
              badgeIcon = '✓';
              badgeText = 'Index Scan';
            } else if (type.includes('hash join') || type.includes('merge join')) {
              badgeCls = 'bg-indigo-950/90 text-indigo-300 border-indigo-700';
              badgeIcon = '⮀';
            }

            const extraPills = [];
            if (node['Temp Written Blocks'] > 0) extraPills.push({ text: '⚠ Disk Spill', cls: 'bg-red-950 text-red-300 border-red-800' });
            if (node['Actual Total Time'] != null) extraPills.push({ text: `⏱ ${node['Actual Total Time'].toFixed(2)}ms`, cls: 'bg-slate-800 text-slate-300 border-slate-700' });

            const cardHtml = makeNodeCard({
              step: stepCount,
              operationTitle: node['Node Type'] || 'Scan',
              badgeCls,
              badgeIcon,
              badgeText,
              table: node['Relation Name'] || '(intermediate)',
              alias: node['Alias'],
              keyUsed: node['Index Name'] || null,
              isDanger,
              isWarning,
              isOptimal,
              rows: rowsExamined,
              filtered: null,
              costLabel: 'Cost',
              costVal: node['Total Cost'] != null ? node['Total Cost'].toFixed(1) : '-',
              filterExpr: node['Filter'] || node['Join Filter'] || null,
              extraPills
            });
            cards.push(cardHtml);

            const children = node['Plans'] || [];
            children.forEach(c => walk(c, depth + 1));
          }

          walk(stmtPlan['Plan'], 0);
          return cards;
        }

        let allCards = [];
        parsedJSON.forEach(stmt => {
          allCards = allCards.concat(traversePG(stmt));
        });

        let flowHtml = '';
        allCards.forEach((cHtml, idx) => {
          flowHtml += cHtml;
          if (idx < allCards.length - 1) flowHtml += makeFlowConnector(totalRows);
        });
        flowHtml += makeTerminalNode(totalRows);
        canvas.innerHTML = flowHtml;

        if (summaryBar) {
          summaryBar.innerHTML = `
            <div class="flex items-center space-x-2.5">
              <span class="text-slate-400">Total Steps: <strong class="text-white">${stepCount}</strong></span>
              <span class="text-slate-600">·</span>
              <span class="text-slate-400">Rows: <strong class="text-white">${totalRows.toLocaleString()}</strong></span>
            </div>
            <div class="flex items-center space-x-2">
              ${fullScanCount > 0 ? `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-red-950/90 text-red-300 border border-red-700 flex items-center gap-1"><span>⚠️</span><span>${fullScanCount} Sequential Scan${fullScanCount > 1 ? 's' : ''}</span></span>` : `<span class="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-950/90 text-emerald-300 border border-emerald-700 flex items-center gap-1"><span>✓</span><span>Index-Optimized</span></span>`}
            </div>
          `;
        }
        return;
      }

      // Check if we can build from MySQL JSON
      if (isMySQLJSON) {
        if (empty) empty.classList.add('hidden');
        const qb = parsedJSON['query_block'];
        const tableNodes = [];

        function collectMySQLTables(node) {
          if (!node) return;
          if (node['table']) tableNodes.push(node['table']);
          else if (node['table_name']) tableNodes.push(node);
          if (node['nested_loop']) {
            node['nested_loop'].forEach(n => collectMySQLTables(n));
          }
          if (node['attached_subqueries']) {
            node['attached_subqueries'].forEach(sq => {
              if (sq['query_block']) collectMySQLTables(sq['query_block']);
            });
          }
        }
        collectMySQLTables(qb);

        if (tableNodes.length > 0) {
          let totalRows = 0;
          let fullScanCount = 0;
          let filesortCount = 0;
          let flowHtml = '';

          tableNodes.forEach((t, idx) => {
            const at = (t['access_type'] || '').toUpperCase();
            const isDanger = at === 'ALL';
            const isWarning = at === 'INDEX' || !!t['using_filesort'];
            const isOptimal = ['REF', 'EQ_REF', 'CONST', 'SYSTEM', 'RANGE'].includes(at);
            if (isDanger) fullScanCount++;
            if (t['using_filesort']) filesortCount++;

            const rowsExamined = t['rows_examined_per_scan'] || t['rows'] || 0;
            totalRows += rowsExamined;

            let badgeCls = 'bg-slate-800 text-slate-300 border-slate-700';
            let badgeIcon = '⚙️';
            let badgeText = at || 'SCAN';
            if (isDanger) {
              badgeCls = 'bg-red-950/90 text-red-300 border-red-700';
              badgeIcon = '⚠️';
              badgeText = 'ALL (Full Scan)';
            } else if (isOptimal) {
              badgeCls = 'bg-emerald-950/90 text-emerald-300 border-emerald-700';
              badgeIcon = '✓';
              badgeText = at;
            }

            const extraPills = [];
            if (t['using_filesort']) extraPills.push({ text: '⚠ Filesort', cls: 'bg-amber-950 text-amber-300 border-amber-800' });
            if (t['using_temporary_table']) extraPills.push({ text: '⚠ Temp Table', cls: 'bg-red-950 text-red-300 border-red-800' });

            const card = makeNodeCard({
              step: idx + 1,
              operationTitle: 'Table Access',
              badgeCls,
              badgeIcon,
              badgeText,
              table: t['table_name'] || '(subquery)',
              alias: null,
              keyUsed: t['key'] || null,
              isDanger,
              isWarning,
              isOptimal,
              rows: rowsExamined,
              filtered: typeof t['filtered'] === 'number' ? t['filtered'] : parseFloat(t['filtered'] || '100'),
              costLabel: 'Read Cost',
              costVal: t['cost_info']?.read_cost || '-',
              filterExpr: t['attached_condition'] || null,
              extraPills
            });

            flowHtml += card;
            if (idx < tableNodes.length - 1) flowHtml += makeFlowConnector(rowsExamined);
          });

          flowHtml += makeTerminalNode(totalRows);
          canvas.innerHTML = flowHtml;

          if (summaryBar) {
            summaryBar.innerHTML = `
              <div class="flex items-center space-x-2.5">
                <span class="text-slate-400">Total Steps: <strong class="text-white">${tableNodes.length}</strong></span>
                <span class="text-slate-600">·</span>
                <span class="text-slate-400">Est. Rows Examined: <strong class="text-white">${totalRows.toLocaleString()}</strong></span>
              </div>
              <div class="flex items-center space-x-2">
                ${fullScanCount > 0 ? `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-red-950/90 text-red-300 border border-red-700 flex items-center gap-1"><span>⚠️</span><span>${fullScanCount} Full Scan${fullScanCount > 1 ? 's' : ''}</span></span>` : `<span class="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-950/90 text-emerald-300 border border-emerald-700 flex items-center gap-1"><span>✓</span><span>No Full Scans</span></span>`}
                ${filesortCount > 0 ? `<span class="px-2 py-0.5 rounded text-[10px] font-semibold bg-amber-950/90 text-amber-300 border border-amber-700 flex items-center gap-1"><span>⚠️</span><span>Filesort</span></span>` : ''}
              </div>
            `;
          }
          return;
        }
      }

      // Universal Pipeline Fallback: Build directly from tabular rows!
      // Guarantees MariaDB, SQLite, MySQL standard explain ALWAYS display a visual graph!
      if (rows.length > 0) {
        if (empty) empty.classList.add('hidden');
        let totalRows = 0;
        let fullScanCount = 0;
        let filesortCount = 0;
        let indexScanCount = 0;
        let flowHtml = '';

        rows.forEach((r, idx) => {
          const upperType = (r.type || '').toUpperCase();
          const extraStr = (r.extra || '').toUpperCase();

          let isDanger = false;
          let isWarning = false;
          let isOptimal = false;
          let operationTitle = 'Table Access';
          let badgeCls = 'bg-slate-800 text-slate-300 border-slate-700';
          let badgeIcon = '⚙️';
          let badgeText = r.type || 'STEP';

          if (upperType === 'ALL') {
            isDanger = true;
            fullScanCount++;
            operationTitle = 'Full Table Scan';
            badgeCls = 'bg-red-950/90 text-red-300 border-red-700';
            badgeIcon = '⚠️';
            badgeText = 'ALL (Full Scan)';
          } else if (upperType === 'INDEX') {
            isWarning = true;
            operationTitle = 'Full Index Scan';
            badgeCls = 'bg-amber-950/90 text-amber-300 border-amber-700';
            badgeIcon = '⚡';
            badgeText = 'index (Full)';
          } else if (upperType === 'RANGE') {
            isWarning = true;
            operationTitle = 'Index Range Scan';
            badgeCls = 'bg-yellow-950/90 text-yellow-300 border-yellow-700';
            badgeIcon = '🔍';
            badgeText = 'RANGE';
          } else if (['REF', 'EQ_REF', 'CONST', 'SYSTEM'].includes(upperType)) {
            isOptimal = true;
            indexScanCount++;
            operationTitle = 'Index Key Lookup';
            badgeCls = 'bg-emerald-950/90 text-emerald-300 border-emerald-700';
            badgeIcon = '✓';
            badgeText = upperType;
          } else if (upperType.includes('SCAN') || upperType.includes('SEARCH')) {
            const isSearch = upperType.includes('SEARCH');
            if (isSearch) {
              isOptimal = true;
              indexScanCount++;
              operationTitle = 'Index Search';
              badgeCls = 'bg-sky-950/90 text-sky-300 border-sky-700';
              badgeIcon = '✓';
              badgeText = 'SEARCH';
            } else {
              isDanger = true;
              fullScanCount++;
              operationTitle = 'Table Scan';
              badgeCls = 'bg-red-950/90 text-red-300 border-red-700';
              badgeIcon = '⚠️';
              badgeText = 'SCAN';
            }
          }

          if (extraStr.includes('FILESORT')) filesortCount++;

          const extraPills = [];
          if (extraStr.includes('FILESORT')) extraPills.push({ text: '⚠ Filesort (Sort Buffer/Disk)', cls: 'bg-amber-950 text-amber-300 border-amber-800' });
          if (extraStr.includes('TEMPORARY')) extraPills.push({ text: '⚠ Temp Table', cls: 'bg-red-950 text-red-300 border-red-800' });
          if (extraStr.includes('USING INDEX')) extraPills.push({ text: '✓ Covering Index', cls: 'bg-emerald-950 text-emerald-300 border-emerald-800' });
          if (extraStr.includes('USING WHERE')) extraPills.push({ text: 'Where Filter', cls: 'bg-slate-800 text-slate-300 border-slate-700' });

          const rowExamined = r.rows || 0;
          totalRows += rowExamined;

          const card = makeNodeCard({
            step: r.id || (idx + 1),
            operationTitle,
            badgeCls,
            badgeIcon,
            badgeText,
            table: r.table || '(derived)',
            alias: null,
            keyUsed: r.key || null,
            isDanger,
            isWarning,
            isOptimal,
            rows: rowExamined,
            filtered: typeof r.filtered === 'number' && r.filtered > 0 ? r.filtered : null,
            costLabel: 'Select Type',
            costVal: r.select_type || '-',
            filterExpr: r.extra && !extraStr.includes('USING') ? r.extra : null,
            extraPills
          });

          flowHtml += card;
          if (idx < rows.length - 1) flowHtml += makeFlowConnector(rowExamined);
        });

        flowHtml += makeTerminalNode(totalRows);
        canvas.innerHTML = flowHtml;

        if (summaryBar) {
          summaryBar.innerHTML = `
            <div class="flex items-center space-x-2.5">
              <span class="text-slate-400">Total Steps: <strong class="text-white">${rows.length}</strong></span>
              <span class="text-slate-600">·</span>
              <span class="text-slate-400">Est. Rows Examined: <strong class="text-white">~${totalRows.toLocaleString()}</strong></span>
            </div>
            <div class="flex items-center space-x-2">
              ${fullScanCount > 0 ? `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-red-950/90 text-red-300 border border-red-700 flex items-center gap-1"><span>⚠️</span><span>${fullScanCount} Full Scan${fullScanCount > 1 ? 's' : ''}</span></span>` : `<span class="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-950/90 text-emerald-300 border border-emerald-700 flex items-center gap-1"><span>✓</span><span>No Full Scans</span></span>`}
              ${filesortCount > 0 ? `<span class="px-2 py-0.5 rounded text-[10px] font-semibold bg-amber-950/90 text-amber-300 border border-amber-700 flex items-center gap-1"><span>⚠️</span><span>Filesort</span></span>` : ''}
              ${indexScanCount > 0 ? `<span class="px-2 py-0.5 rounded text-[10px] font-semibold bg-sky-950/90 text-sky-300 border border-sky-700 flex items-center gap-1"><span>🔑</span><span>${indexScanCount} Indexed</span></span>` : ''}
            </div>
          `;
        }
        return;
      }

      // No steps available at all
      if (empty) empty.classList.remove('hidden');
      if (summaryBar) summaryBar.innerHTML = '';
    }

