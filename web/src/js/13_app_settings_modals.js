// --- Modals & Connection Management ---
    function openNewConnModal() {
      document.getElementById('modal-conn-id').value = '';
      document.getElementById('modal-conn-title').textContent = 'Add Database Connection';
      document.getElementById('modal-conn-name').value = '';
      document.getElementById('modal-conn-env').value = 'Development';
      document.getElementById('modal-conn-driver').value = 'mysql';
      document.getElementById('modal-conn-host').value = '';
      document.getElementById('modal-conn-port').value = 3306;
      document.getElementById('modal-conn-db').value = '';
      document.getElementById('modal-conn-user').value = '';
      const passEl = document.getElementById('modal-conn-pass');
      passEl.value = '';
      passEl.placeholder = '••••••••';
      onDriverChange();
      document.getElementById('connection-modal').classList.remove('hidden');
    }

    function openEditConnModal(connId) {
      const conn = connections.find(c => c.id === connId);
      if (!conn) return;

      document.getElementById('modal-conn-id').value = conn.id;
      document.getElementById('modal-conn-title').textContent = 'Edit Connection: ' + conn.name;
      document.getElementById('modal-conn-name').value = conn.name || '';
      document.getElementById('modal-conn-env').value = conn.environment || 'Development';
      document.getElementById('modal-conn-driver').value = conn.driver || 'mysql';
      document.getElementById('modal-conn-host').value = conn.host || '';
      document.getElementById('modal-conn-port').value = conn.port || 3306;
      document.getElementById('modal-conn-db').value = conn.database_name || '';
      document.getElementById('modal-conn-user').value = conn.user || '';
      const passEl = document.getElementById('modal-conn-pass');
      passEl.value = '';
      passEl.placeholder = '•••••••• (leave blank to keep current)';
      onDriverChange();
      document.getElementById('connection-modal').classList.remove('hidden');
    }

    function closeConnModal() {
      document.getElementById('connection-modal').classList.add('hidden');
    }

    async function deleteConnection(connId, e) {
      if (e) e.stopPropagation();
      const conn = connections.find(c => c.id === connId);
      const name = conn ? conn.name : connId;
      if (!await showNativeConfirm(`Are you sure you want to delete connection "${name}"?`, 'Delete Connection')) {
        return;
      }
      try {
        const res = await apiFetch(`/api/connections/${connId}`, { method: 'DELETE' });
        if (res.ok) {
          if (activeConnection && activeConnection.id === connId) {
            activeConnection = null;
            activeSession = null;
            try {
              sessionStorage.removeItem('sqlfox_active_conn_id');
              if (localStorage.getItem('sqlfox_last_conn_id') === connId) {
                localStorage.removeItem('sqlfox_last_conn_id');
              }
            } catch {}
            renderHeaderConnectionBadge(null);
            updateFooterStats();
            document.getElementById('schema-tree').innerHTML = '<div class="text-slate-500 text-center py-4">Select a connection to view schema</div>';
            document.getElementById('table-count-badge').textContent = '0';
          }
          await loadConnections();
        } else {
          const data = await res.json();
          showNativeAlert('Failed to delete connection: ' + (data.error || res.statusText), 'Delete Connection');
        }
      } catch (err) {
        showNativeAlert('Failed to delete connection: ' + err.message, 'Delete Connection');
      }
    }

    async function testConnectionFromModal() {
      const btn = document.querySelector('#connection-modal button[onclick*="testConnectionFromModal"]');
      if (btn && btn.disabled) return;
      const driver = document.getElementById('modal-conn-driver').value;
      const host = document.getElementById('modal-conn-host').value.trim();
      if (!host) {
        showNativeAlert('Host / SQLite File Path is required to test connection', 'Test Connection');
        return;
      }
      let dbName = document.getElementById('modal-conn-db').value.trim();
      if (driver === 'sqlite' && !dbName) {
        dbName = host;
      }
      const connId = document.getElementById('modal-conn-id').value;
      const payload = {
        id: connId || undefined,
        driver: driver,
        host: host,
        port: parseInt(document.getElementById('modal-conn-port').value) || (driver === 'postgres' ? 5432 : 3306),
        database_name: dbName,
        user: document.getElementById('modal-conn-user').value.trim(),
        password: document.getElementById('modal-conn-pass').value
      };

      const origHtml = btn ? btn.innerHTML : '';
      if (btn) {
        btn.disabled = true;
        btn.classList.add('opacity-50', 'cursor-not-allowed');
        btn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Testing...</span>`;
      }

      // ponytail: 15s timeout guard prevents freeze on unreachable IP / blackholed TCP port
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 15000);

      try {
        const res = await apiFetch('/api/connections/test-transient/test', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
          signal: controller.signal
        });
        const data = await res.json();
        if (res.ok && data.success) {
          showNativeAlert(`✅ Connection successful! Latency: ${data.latency_ms}ms`, 'Connection Test');
        } else {
          showNativeAlert(`❌ Connection failed: ${data.error || res.statusText || 'Unable to connect'}`, 'Connection Test');
        }
      } catch (err) {
        if (err.name === 'AbortError') {
          showNativeAlert('❌ Connection test timed out after 15s. Verify host IP, port, and firewall rules.', 'Connection Timeout');
        } else {
          showNativeAlert(`❌ Connection failed: ${err.message}`, 'Connection Test');
        }
      } finally {
        clearTimeout(timeoutId);
        if (btn) {
          btn.disabled = false;
          btn.classList.remove('opacity-50', 'cursor-not-allowed');
          btn.innerHTML = origHtml;
        }
      }
    }

    function onDriverChange() {
      const d = document.getElementById('modal-conn-driver').value;
      const mysqlFields = document.getElementById('mysql-specific-fields');
      const hostLabel = document.getElementById('modal-host-label');
      const portInput = document.getElementById('modal-conn-port');
      if (d === 'sqlite') {
        mysqlFields.classList.add('hidden');
        hostLabel.textContent = 'SQLite File Path';
      } else {
        mysqlFields.classList.remove('hidden');
        hostLabel.textContent = 'Host';
        if (d === 'postgres') {
          if (!portInput.value || portInput.value === '3306') {
            portInput.value = '5432';
          }
        } else if (d === 'mysql') {
          if (!portInput.value || portInput.value === '5432') {
            portInput.value = '3306';
          }
        }
      }
    }

    async function saveConnection(e) {
      if (e) e.preventDefault();
      const id = document.getElementById('modal-conn-id').value;
      const driver = document.getElementById('modal-conn-driver').value;
      let host = document.getElementById('modal-conn-host').value.trim();
      let dbName = document.getElementById('modal-conn-db').value.trim();
      if (driver === 'sqlite' && !dbName) {
        dbName = host;
      }
      const conn = {
        id: id || undefined,
        name: document.getElementById('modal-conn-name').value.trim(),
        environment: document.getElementById('modal-conn-env').value,
        driver: driver,
        host: host,
        port: parseInt(document.getElementById('modal-conn-port').value) || (driver === 'postgres' ? 5432 : 3306),
        database_name: dbName,
        user: document.getElementById('modal-conn-user').value.trim(),
        password: document.getElementById('modal-conn-pass').value
      };

      try {
        const res = await apiFetch('/api/connections', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(conn)
        });
        if (res.ok) {
          const saved = await res.json();
          closeConnModal();
          await loadConnections();
          if (saved && saved.id) {
            await selectConnection(saved.id);
          }
          showNativeAlert(`Connection "${conn.name}" saved and activated successfully.`, 'Connection Saved');
        } else {
          const data = await res.json();
          showNativeAlert('Failed to save connection: ' + (data.error || res.statusText), 'Save Connection');
        }
      } catch (err) {
        showNativeAlert('Failed to save connection: ' + err.message, 'Save Connection');
      }
    }
    const saveConnectionFromModal = saveConnection;

    async function viewTableDDL(tableName) {
      if (!activeConnection) return;
      const res = await apiFetch(`/api/session/${activeConnection.id}/ddl/${tableName}`);
      const data = await res.json();
      document.getElementById('ddl-modal-title').textContent = `CREATE TABLE ${tableName}`;
      document.getElementById('ddl-modal-content').textContent = data.ddl;
      document.getElementById('ddl-modal').classList.remove('hidden');
    }
    function closeDDLModal() {
      document.getElementById('ddl-modal').classList.add('hidden');
    }
    async function copyDDLToClipboard() {
      const text = document.getElementById('ddl-modal-content').textContent;
      const ok = await copyTextToClipboard(text);
      if (ok) {
        showNativeAlert('Copied to clipboard!', 'Clipboard');
      } else {
        showNativeAlert('Failed to copy DDL in this browser context.', 'Clipboard Error');
      }
    }
    function insertDDLToEditor() {
      const text = document.getElementById('ddl-modal-content').textContent;
      setEditorSQL(text);
      closeDDLModal();
    }

    // --- Query Snippets & Dynamic Variables ---
    let loadedSnippets = [];
    let activeSnippetUnderParam = null;
    let activeSnippetVars = [];

    async function loadSnippets() {
      try {
        const res = await apiFetch('/api/snippets');
        if (res.ok) {
          loadedSnippets = await res.json();
          renderSnippetsList();
        }
      } catch (err) {
        console.error('Failed to load snippets:', err);
      }
    }

    function openSnippetsManager() {
      const dlg = document.getElementById('snippets-manager-dialog');
      if (!dlg) return;
      closeSnippetEditForm();
      const filterInput = document.getElementById('snippets-filter-input');
      if (filterInput) filterInput.value = '';
      renderSnippetsList();
      try {
        if (typeof dlg.showModal === 'function') {
          dlg.showModal();
        } else {
          dlg.setAttribute('open', '');
        }
      } catch (err) {
        console.error('Error opening snippets manager:', err);
      }
      if (filterInput) filterInput.focus();
    }

    function closeSnippetsManager() {
      const dlg = document.getElementById('snippets-manager-dialog');
      if (dlg && dlg.open) dlg.close();
    }

    function onSnippetsFilterInput(val) {
      const clearBtn = document.getElementById('snippets-filter-clear-btn');
      if (clearBtn) {
        if (val && val.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
      renderSnippetsList();
    }

    function clearSnippetsFilter() {
      const input = document.getElementById('snippets-filter-input');
      const clearBtn = document.getElementById('snippets-filter-clear-btn');
      if (input) {
        input.value = '';
        input.focus();
      }
      if (clearBtn) {
        clearBtn.classList.add('hidden');
      }
      renderSnippetsList();
    }

    function renderSnippetsList() {
      const container = document.getElementById('snippets-cards-container');
      const countBadge = document.getElementById('snippets-count-badge');
      if (!container) return;

      const filterEl = document.getElementById('snippets-filter-input');
      const query = (filterEl ? filterEl.value : '').toLowerCase().trim();
      const clearBtn = document.getElementById('snippets-filter-clear-btn');
      if (clearBtn && filterEl) {
        if (filterEl.value.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }

      const filtered = (loadedSnippets || []).filter(s => {
        if (!query) return true;
        const lowerTitle = (s.title || '').toLowerCase();
        const lowerTags = (s.tags || '').toLowerCase();
        const lowerSql = (s.sql_content || '').toLowerCase();
        return lowerTitle.includes(query) || lowerTags.includes(query) || lowerSql.includes(query);
      });

      if (countBadge) countBadge.textContent = `${filtered.length} snippet${filtered.length === 1 ? '' : 's'}`;

      if (filtered.length === 0) {
        container.innerHTML = `
          <div class="p-8 text-center text-slate-500 font-mono text-xs">
            ${query ? 'No snippets match your filter.' : 'No snippets available. Click "+ New Snippet" to create one.'}
          </div>
        `;
        return;
      }

      container.innerHTML = filtered.map(s => {
        const tagsHtml = (s.tags || '').split(',').map(t => t.trim()).filter(Boolean).map(t =>
          `<span class="bg-slate-800 text-slate-300 px-1.5 py-0.5 rounded text-[10px] font-mono border border-slate-700">${escapeHtml(t)}</span>`
        ).join(' ');

        const sqlSnippet = s.sql_content.length > 120 ? s.sql_content.slice(0, 120) + '...' : s.sql_content;
        const vars = parseSnippetVariables(s.sql_content);
        const varsBadge = vars.length > 0
          ? `<span class="bg-pink-950/60 text-pink-400 border border-pink-800/60 px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold">${vars.length} param${vars.length === 1 ? '' : 's'}</span>`
          : '';

        return `
          <div class="p-3 hover:bg-slate-800/40 rounded-lg transition group">
            <div class="flex items-start justify-between">
              <div class="space-y-1">
                <div class="flex items-center space-x-2">
                  <span class="font-bold text-slate-200 text-xs">${escapeHtml(s.title)}</span>
                  ${tagsHtml}
                  ${varsBadge}
                </div>
                <pre class="text-[11px] font-mono text-emerald-400/90 whitespace-pre-wrap leading-relaxed max-h-16 overflow-hidden">${escapeHtml(sqlSnippet)}</pre>
              </div>
              <div class="flex items-center space-x-1.5 opacity-90 group-hover:opacity-100 flex-shrink-0 ml-3">
                <button onclick="invokeSnippetById('${s.id}')" class="bg-app-accent hover:bg-sky-400 text-slate-950 font-bold px-2.5 py-1 rounded text-xs transition" title="Insert into Editor">
                  Insert
                </button>
                <button onclick="openEditSnippetForm('${s.id}')" class="bg-slate-800 hover:bg-slate-700 text-slate-300 px-2 py-1 rounded text-xs transition flex items-center" title="Edit Snippet">
                  ${renderIcon('edit', 'w-3.5 h-3.5')}
                </button>
                <button onclick="deleteSnippet('${s.id}')" class="bg-slate-800 hover:bg-red-900/60 text-slate-400 hover:text-red-300 px-2 py-1 rounded text-xs transition flex items-center" title="Delete Snippet">
                  ${renderIcon('trash-2', 'w-3.5 h-3.5')}
                </button>
              </div>
            </div>
          </div>
        `;
      }).join('');
    }

    function openNewSnippetForm() {
      document.getElementById('snippet-edit-id').value = '';
      document.getElementById('snippet-edit-title').value = '';
      document.getElementById('snippet-edit-tags').value = '';
      document.getElementById('snippet-edit-sql').value = '';
      const form = document.getElementById('snippet-edit-form');
      form.classList.remove('hidden');
      document.getElementById('snippet-edit-title').focus();
    }

    function openEditSnippetForm(id) {
      const s = (loadedSnippets || []).find(x => x.id === id);
      if (!s) return;
      document.getElementById('snippet-edit-id').value = s.id;
      document.getElementById('snippet-edit-title').value = s.title || '';
      document.getElementById('snippet-edit-tags').value = s.tags || '';
      document.getElementById('snippet-edit-sql').value = s.sql_content || '';
      const form = document.getElementById('snippet-edit-form');
      form.classList.remove('hidden');
      document.getElementById('snippet-edit-title').focus();
    }

    function closeSnippetEditForm() {
      const form = document.getElementById('snippet-edit-form');
      if (form) form.classList.add('hidden');
    }

    async function saveSnippetFromForm() {
      const id = document.getElementById('snippet-edit-id').value;
      const title = document.getElementById('snippet-edit-title').value.trim();
      const tags = document.getElementById('snippet-edit-tags').value.trim();
      const sql_content = document.getElementById('snippet-edit-sql').value.trim();

      if (!title) {
        showNativeAlert('Snippet title is required', 'Save Snippet');
        return;
      }
      if (!sql_content) {
        showNativeAlert('Snippet SQL content is required', 'Save Snippet');
        return;
      }

      try {
        const res = await apiFetch('/api/snippets', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id, title, tags, sql_content })
        });
        if (res.ok) {
          closeSnippetEditForm();
          await loadSnippets();
        } else {
          const data = await res.json();
          showNativeAlert(data.error || 'Failed to save snippet', 'Save Snippet');
        }
      } catch (err) {
        showNativeAlert('Error: ' + err.message, 'Save Snippet');
      }
    }

    async function deleteSnippet(id) {
      const confirmed = await showNativeConfirm('Are you sure you want to delete this snippet?', 'Delete Snippet');
      if (!confirmed) return;
      try {
        const res = await apiFetch(`/api/snippets/${id}`, { method: 'DELETE' });
        if (res.ok) {
          await loadSnippets();
        } else {
          const data = await res.json();
          showNativeAlert(data.error || 'Failed to delete snippet', 'Delete Snippet');
        }
      } catch (err) {
        showNativeAlert('Error: ' + err.message, 'Delete Snippet');
      }
    }

    function parseSnippetVariables(sql) {
      if (!sql) return [];
      const regex = /\{\{([a-zA-Z0-9_]+)(?::([^}]*))?\}\}/g;
      const vars = [];
      const seen = new Set();
      let match;
      while ((match = regex.exec(sql)) !== null) {
        const name = match[1];
        const defaultVal = match[2] !== undefined ? match[2] : '';
        if (!seen.has(name)) {
          seen.add(name);
          vars.push({ name, defaultVal });
        }
      }
      return vars;
    }

    function invokeSnippetById(id) {
      const s = (loadedSnippets || []).find(x => x.id === id);
      if (s) invokeSnippet(s);
    }

    function invokeSnippet(snippet) {
      if (!snippet || !snippet.sql_content) return;
      const vars = parseSnippetVariables(snippet.sql_content);
      closeSnippetsManager();
      if (vars.length === 0) {
        insertSnippetSQL(snippet.sql_content);
      } else {
        openSnippetParamModal(snippet, vars);
      }
    }

    function openSnippetParamModal(snippet, vars) {
      activeSnippetUnderParam = snippet;
      activeSnippetVars = vars;

      const dlg = document.getElementById('snippet-param-dialog');
      if (!dlg) return;

      document.getElementById('snippet-param-modal-title').textContent = `Snippet: ${snippet.title}`;
      document.getElementById('snippet-param-modal-subtitle').textContent = `Fill in values for ${vars.length} dynamic placeholder${vars.length === 1 ? '' : 's'}`;

      const container = document.getElementById('snippet-param-inputs-container');
      container.innerHTML = vars.map(v => `
        <div>
          <label class="block text-slate-300 mb-1 font-semibold">${escapeHtml(v.name)}</label>
          <input type="text" data-var-name="${escapeHtml(v.name)}" value="${escapeHtml(v.defaultVal)}" class="snippet-param-input w-full bg-slate-900 border border-slate-700 p-2 text-white font-mono rounded outline-none focus:border-app-accent text-xs">
        </div>
      `).join('');

      container.querySelectorAll('.snippet-param-input').forEach((inp) => {
        inp.addEventListener('input', updateSnippetParamLivePreview);
        inp.addEventListener('keydown', (e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            confirmSnippetSubstitution();
          }
        });
      });

      updateSnippetParamLivePreview();
      try {
        if (typeof dlg.showModal === 'function') {
          dlg.showModal();
        } else {
          dlg.setAttribute('open', '');
        }
      } catch (err) {
        console.error('Error opening snippet param dialog:', err);
      }

      const firstInp = container.querySelector('.snippet-param-input');
      if (firstInp) {
        firstInp.focus();
        firstInp.select();
      }
    }

    function closeSnippetParamModal() {
      const dlg = document.getElementById('snippet-param-dialog');
      if (dlg && dlg.open) dlg.close();
      activeSnippetUnderParam = null;
      activeSnippetVars = [];
    }

    function updateSnippetParamLivePreview() {
      if (!activeSnippetUnderParam) return;
      const values = {};
      document.querySelectorAll('.snippet-param-input').forEach(inp => {
        values[inp.dataset.varName] = inp.value;
      });
      const substituted = activeSnippetUnderParam.sql_content.replace(/\{\{([a-zA-Z0-9_]+)(?::([^}]*))?\}\}/g, (match, name, def) => {
        return values[name] !== undefined ? values[name] : (def !== undefined ? def : match);
      });
      document.getElementById('snippet-param-sql-preview').textContent = substituted;
    }

    function confirmSnippetSubstitution() {
      if (!activeSnippetUnderParam) return;
      const values = {};
      document.querySelectorAll('.snippet-param-input').forEach(inp => {
        values[inp.dataset.varName] = inp.value;
      });
      const substituted = activeSnippetUnderParam.sql_content.replace(/\{\{([a-zA-Z0-9_]+)(?::([^}]*))?\}\}/g, (match, name, def) => {
        return values[name] !== undefined ? values[name] : (def !== undefined ? def : match);
      });

      closeSnippetParamModal();
      closeSnippetsManager();
      insertSnippetSQL(substituted);
    }

    function insertSnippetSQL(sql) {
      if (cmEditor) {
        cmEditor.replaceSelection(sql);
        cmEditor.focus();
      } else {
        const ta = document.getElementById('sql-editor-textarea');
        if (ta) ta.value += sql;
      }
    }

    // --- Desktop Native App & Browser Session Helpers ---
    function initBrowserPopoutAffordance() {
      const btn = document.getElementById('btn-open-in-browser');
      if (!btn) return;
      // Show only when running inside a frameless native app window (Chromium --app / PWA / standalone)
      const isStandalone = (window.matchMedia && (window.matchMedia('(display-mode: standalone)').matches || window.matchMedia('(display-mode: window-controls-overlay)').matches)) ||
                           Boolean(window.navigator.standalone) ||
                           (window.locationbar && !window.locationbar.visible);
      if (isStandalone) {
        btn.classList.remove('hidden');
        btn.classList.add('inline-flex');
      } else {
        btn.classList.add('hidden');
        btn.classList.remove('inline-flex');
      }
    }

    async function openCurrentInBrowser() {
      try {
        const res = await apiFetch('/api/system/open-browser', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: window.location.href })
        });
        if (res.ok) {
          showToast('Opening in default web browser...', 'info');
          return;
        }
      } catch (_) {}

      const win = window.open(window.location.href, '_blank', 'noopener,noreferrer');
      if (win) {
        showToast('Opened in web browser tab', 'info');
      } else {
        showToast('Popup was blocked; please allow popups or copy URL', 'warning');
      }
    }

    async function copyCurrentAppURL() {
      const url = window.location.href;
      try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(url);
          showToast('SqlFox URL copied to clipboard: ' + url, 'success');
          return;
        }
      } catch (_) {}
      prompt('SqlFox App URL:', url);
    }

    async function confirmShutDownServer() {
      if (!confirm('Are you sure you want to shut down the SqlFox server daemon? Any active background jobs and connections will be closed.')) {
        return;
      }
      try {
        const res = await apiFetch('/api/system/shutdown', { method: 'POST' });
        if (res.ok) {
          document.body.innerHTML = `
            <div class="fixed inset-0 bg-slate-950 flex flex-col items-center justify-center p-6 text-center select-none z-[9999]">
              <div class="p-8 bg-slate-900 border border-slate-800 rounded-2xl max-w-sm w-full space-y-4 shadow-2xl">
                <div class="text-5xl">👋</div>
                <h2 class="text-xl font-bold text-white tracking-wide">SqlFox Daemon Stopped</h2>
                <p class="text-xs text-slate-400 leading-relaxed">The server has shut down gracefully. You may safely close this window.</p>
                <button onclick="window.close()" class="px-4 py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-bold rounded-lg w-full transition cursor-pointer">Close Window</button>
              </div>
            </div>
          `;
          return;
        }
        showToast('Failed to trigger shutdown', 'error');
      } catch (err) {
        showToast('Server disconnected or already stopped', 'info');
      }
    }

    // --- Unified App Settings Modal (Connections, GitHub Sync, Notes Bundle, AI Copilot) ---
    function openSettingsModal(tab = 'vault') {
      document.getElementById('vault-modal').classList.remove('hidden');
      switchVaultModalTab(tab);
    }
    function openExportImportModal(tab) {
      openSettingsModal(tab || 'vault');
    }
    function openCloudSyncModal(tab) {
      openSettingsModal(tab || 'github');
    }
    function closeVaultModal() {
      document.getElementById('vault-modal').classList.add('hidden');
    }
    function closeSettingsModal() {
      closeVaultModal();
    }

    function switchVaultModalTab(tab) {
      const tabMap = {
        'vault': { pane: 'pane-vault-archive', btn: 'tab-btn-vault' },
        'connections': { pane: 'pane-vault-archive', btn: 'tab-btn-vault' },
        'github': { pane: 'pane-vault-github', btn: 'tab-btn-sync' },
        'sync': { pane: 'pane-vault-github', btn: 'tab-btn-sync' },
        'bundle': { pane: 'pane-vault-bundle', btn: 'tab-btn-bundle' },
        'notes': { pane: 'pane-vault-bundle', btn: 'tab-btn-bundle' },
        'ai': { pane: 'pane-vault-ai', btn: 'tab-btn-ai' }
      };
      const active = tabMap[tab] || tabMap['vault'];
      const targetPane = active.pane;
      const targetBtn = active.btn;

      ['pane-vault-archive', 'pane-vault-github', 'pane-vault-bundle', 'pane-vault-ai'].forEach(pId => {
        const pane = document.getElementById(pId);
        if (pane) pane.classList.toggle('hidden', pId !== targetPane);
      });

      ['tab-btn-vault', 'tab-btn-sync', 'tab-btn-bundle', 'tab-btn-ai'].forEach(bId => {
        const btn = document.getElementById(bId);
        if (btn) {
          if (bId === targetBtn) {
            btn.className = 'px-3 py-1.5 font-bold text-sky-400 border-b-2 border-sky-400';
          } else {
            btn.className = 'px-3 py-1.5 text-slate-400 hover:text-slate-200';
          }
        }
      });

      if (targetPane === 'pane-vault-github') {
        loadCloudSyncConfig();
      } else if (targetPane === 'pane-vault-ai') {
        initAISettingsForm();
      } else if (targetPane === 'pane-vault-archive') {
        renderLANStatusInSettings();
      }
    }

    async function loadCloudSyncConfig() {
      try {
        const res = await apiFetch('/api/cloud-sync/config');
        if (!res.ok) return;
        const cfg = await res.json();
        const enabledEl = document.getElementById('cloud-sync-enabled');
        const tokenEl = document.getElementById('cloud-sync-token');
        const repoEl = document.getElementById('cloud-sync-repo');
        const branchEl = document.getElementById('cloud-sync-branch');
        const prefixEl = document.getElementById('cloud-sync-prefix');
        const vaultEl = document.getElementById('cloud-sync-include-vault');

        if (enabledEl) enabledEl.checked = !!cfg.enabled;
        if (tokenEl && cfg.has_token) tokenEl.placeholder = cfg.masked_token || '••••••••';
        if (repoEl) repoEl.value = cfg.github_repo || '';
        if (branchEl) branchEl.value = cfg.github_branch || 'main';
        if (prefixEl) prefixEl.value = cfg.path_prefix || '';
        if (vaultEl) vaultEl.checked = cfg.include_vault_backup !== false;

        renderCloudSyncStatus(cfg);
        updateCloudSyncHeaderBadge(cfg);
      } catch (e) {
        console.error('Failed to load cloud sync config', e);
      }
    }

    function renderCloudSyncStatus(cfg) {
      const iconEl = document.getElementById('cloud-sync-status-icon');
      const titleEl = document.getElementById('cloud-sync-status-title');
      const subEl = document.getElementById('cloud-sync-status-sub');
      if (!iconEl || !titleEl || !subEl) return;

      if (!cfg.enabled) {
        iconEl.className = 'w-2.5 h-2.5 rounded-full bg-slate-500 inline-block shrink-0';
        titleEl.textContent = 'Cloud Sync Disabled';
        subEl.textContent = 'Enable automatic GitHub sync to replicate notes and snippets.';
      } else if (cfg.last_sync_status === 'syncing') {
        iconEl.className = 'w-2.5 h-2.5 rounded-full bg-amber-400 inline-block shrink-0';
        titleEl.textContent = 'Syncing to GitHub...';
        subEl.textContent = 'Replicating changes in background';
      } else if (cfg.last_sync_status === 'error') {
        iconEl.className = 'w-2.5 h-2.5 rounded-full bg-rose-500 inline-block shrink-0';
        titleEl.textContent = 'Sync Error';
        subEl.textContent = cfg.last_sync_error || 'Failed to sync with repository';
      } else if (cfg.last_sync_status === 'synced') {
        iconEl.className = 'w-2.5 h-2.5 rounded-full bg-emerald-400 inline-block shrink-0';
        titleEl.textContent = 'Synced with GitHub';
        const at = cfg.last_sync_at ? new Date(cfg.last_sync_at).toLocaleTimeString() : 'Recently';
        const sha = cfg.last_commit_sha ? ' (' + cfg.last_commit_sha.substring(0, 7) + ')' : '';
        subEl.textContent = `Last sync at ${at}${sha}`;
      } else {
        iconEl.className = 'w-2.5 h-2.5 rounded-full bg-slate-500 inline-block shrink-0';
        titleEl.textContent = 'Cloud Sync Ready';
        subEl.textContent = cfg.github_repo ? `Bound to ${cfg.github_repo}` : 'Repository not configured';
      }
    }

    async function updateCloudSyncHeaderBadge(cfgParam) {
      try {
        const badge = document.getElementById('notes-cloud-sync-badge');
        const dot = document.getElementById('notes-cloud-sync-dot');
        const txt = document.getElementById('notes-cloud-sync-text');
        if (!badge || !dot || !txt) return;

        let cfg = cfgParam;
        if (!cfg) {
          const res = await apiFetch('/api/cloud-sync/status');
          if (res.ok) cfg = await res.json();
        }
        if (!cfg || !cfg.enabled) {
          badge.classList.add('hidden');
          return;
        }

        badge.classList.remove('hidden');
        if (cfg.status === 'syncing') {
          dot.className = 'w-2 h-2 rounded-full bg-amber-400 inline-block shrink-0';
          txt.textContent = 'Syncing...';
          badge.title = 'Syncing notes to GitHub';
        } else if (cfg.status === 'error') {
          dot.className = 'w-2 h-2 rounded-full bg-rose-500 inline-block shrink-0';
          txt.textContent = 'Error';
          badge.title = cfg.last_error || 'Cloud sync error';
        } else if (cfg.status === 'pending') {
          dot.className = 'w-2 h-2 rounded-full bg-amber-400 inline-block shrink-0';
          txt.textContent = 'Pending';
          badge.title = 'Changes queued for sync';
        } else {
          dot.className = 'w-2 h-2 rounded-full bg-emerald-400 inline-block shrink-0';
          txt.textContent = 'Synced';
          badge.title = 'Notes synced with GitHub';
        }
      } catch (e) {
        // non-blocking
      }
    }

    async function saveCloudSyncConfig() {
      const enabled = document.getElementById('cloud-sync-enabled').checked;
      const token = document.getElementById('cloud-sync-token').value.trim();
      const repo = document.getElementById('cloud-sync-repo').value.trim();
      const branch = document.getElementById('cloud-sync-branch').value.trim();
      const prefix = document.getElementById('cloud-sync-prefix').value.trim();
      const includeVault = document.getElementById('cloud-sync-include-vault').checked;

      const res = await apiFetch('/api/cloud-sync/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          enabled,
          github_token: token,
          github_repo: repo,
          github_branch: branch,
          path_prefix: prefix,
          include_vault_backup: includeVault
        })
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        return showNativeAlert(err.message || 'Failed to save config', 'Cloud Sync');
      }

      showNativeAlert('Cloud Sync settings saved successfully', 'Cloud Sync');
      await loadCloudSyncConfig();
    }

    async function testCloudSync() {
      const token = document.getElementById('cloud-sync-token').value.trim();
      const repo = document.getElementById('cloud-sync-repo').value.trim();
      const branch = document.getElementById('cloud-sync-branch').value.trim();

      const res = await apiFetch('/api/cloud-sync/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          github_token: token,
          github_repo: repo,
          github_branch: branch
        })
      });

      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        return showNativeAlert(data.message || 'Connection test failed', 'Cloud Sync Test');
      }
      showNativeAlert(data.message || 'Repository connection verified successfully!', 'Cloud Sync Test');
    }

    async function runCloudSyncNow() {
      const btn = document.getElementById('btn-cloud-sync-now');
      if (btn) btn.disabled = true;
      try {
        const res = await apiFetch('/api/cloud-sync/run', { method: 'POST' });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
          showNativeAlert(data.message || 'Sync failed', 'Cloud Sync Error');
        } else {
          showNativeAlert(data.message || `Successfully synced ${data.files_count} files (${data.commit_sha?.substring(0, 7)})`, 'Cloud Sync');
        }
        await loadCloudSyncConfig();
      } finally {
        if (btn) btn.disabled = false;
      }
    }

    function doExportNotesBundle() {
      window.location.href = '/api/notes/bundle/export';
    }

    async function doImportNotesBundle() {
      const fileInput = document.getElementById('import-bundle-file');
      if (!fileInput || !fileInput.files[0]) {
        return showNativeAlert('Please select a .zip notes bundle file first', 'Notes Bundle Import');
      }

      const file = fileInput.files[0];
      const formData = new FormData();
      formData.append('bundle', file);

      const res = await apiFetch('/api/notes/bundle/import', {
        method: 'POST',
        body: formData
      });

      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        return showNativeAlert(data.message || 'Failed to import bundle', 'Notes Bundle Import');
      }

      showNativeAlert(data.message || `Imported ${data.imported_count} notes and snippets`, 'Notes Bundle Import');
      closeVaultModal();
      if (typeof fetchAndRenderInboxNotes === 'function') {
        await fetchAndRenderInboxNotes();
      }
      if (typeof loadSnippetsList === 'function') {
        await loadSnippetsList();
      }
    }

    async function doExportVault() {
      const pass = document.getElementById('export-passphrase').value;
      if (!pass) return showNativeAlert('Passphrase is required', 'Vault Export');
      const res = await apiFetch('/api/vault/export', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ export_passphrase: pass })
      });
      const blob = await res.blob();
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = 'sqlfox-connections.sqlfox-vault';
      a.click();
      closeVaultModal();
    }

    async function doImportVault() {
      const fileInput = document.getElementById('import-file');
      const pass = document.getElementById('import-passphrase').value;
      if (!fileInput.files[0] || !pass) return showNativeAlert('File and passphrase required', 'Vault Import');

      const text = await fileInput.files[0].text();
      const res = await apiFetch('/api/vault/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ export_passphrase: pass, archive: JSON.parse(text) })
      });
      const data = await res.json();
      showNativeAlert(data.message || 'Imported ' + data.imported_count + ' connections', 'Vault Import');
      closeVaultModal();
      await loadConnections();
    }

    // --- Export Grid Results ---
    function toggleExportCSVMenu(force, e) {
      if (e && typeof e.stopPropagation === 'function') e.stopPropagation();
      const menu = document.getElementById('export-csv-menu');
      if (!menu) return;
      if (typeof force === 'boolean') {
        menu.classList.toggle('hidden', !force);
      } else {
        const isHidden = menu.classList.contains('hidden');
        closeAllContextMenus();
        if (isHidden) {
          menu.classList.remove('hidden');
        } else {
          menu.classList.add('hidden');
        }
      }
    }

    document.addEventListener('click', (e) => {
      const container = document.getElementById('export-csv-dropdown-container');
      if (container && !container.contains(e.target)) {
        toggleExportCSVMenu(false);
      }
    });

    function exportCSV(excelSafe = false) {
      if (!currentResults || currentResults.length === 0) {
        return showNativeAlert('No results to export', 'Export CSV');
      }
      const header = currentColumns.map(c => `"${c.name}"`).join(',');
      const rows = currentResults.map(r => r.map(v => {
        if (v === null || v === undefined) return '';
        const str = String(v);
        if (excelSafe) {
          // If value has leading zero followed by digits (e.g. 08123456789) or phone +, format as formula
          if (/^0\d+$/.test(str) || /^\+\d+$/.test(str)) {
            return `="\"${str}\""`;
          }
        }
        return `"${str.replace(/"/g, '""')}"`;
      }).join(','));

      let csv = [header, ...rows].join('\n');
      if (excelSafe) {
        // Prepend UTF-8 Byte Order Mark for Excel
        csv = '\uFEFF' + csv;
      }
      const filename = excelSafe ? 'query-results-excel.csv' : 'query-results.csv';
      downloadFile(csv, filename, 'text/csv;charset=utf-8');
    }

    function exportJSON() {
      if (!currentResults || currentResults.length === 0) {
        return showNativeAlert('No results to export', 'Export JSON');
      }
      let exportData = currentResults;
      if (Array.isArray(currentColumns) && currentColumns.length > 0) {
        exportData = currentResults.map(row => {
          if (!Array.isArray(row)) return row;
          const obj = {};
          currentColumns.forEach((col, idx) => {
            const colName = (col && col.name) ? col.name : `col_${idx + 1}`;
            obj[colName] = row[idx] !== undefined ? row[idx] : null;
          });
          return obj;
        });
      }
      const json = JSON.stringify(exportData, null, 2);
      downloadFile(json, 'query-results.json', 'application/json');
    }

    // --- Mock Data Generator ---
    function openMockDataModal() {
      if (!activeConnection) { showNativeAlert('Please connect to a database first.', 'No Connection'); return; }
      if (!activeGridTable) { showNativeAlert('Please open a table in the Table Viewer first.', 'No Table Selected'); return; }
      const dlg = document.getElementById('mock-data-dialog');
      if (!dlg) return;
      document.getElementById('mock-data-table-label').textContent = activeGridTable;
      document.getElementById('mock-data-rows').value = '100';
      const status = document.getElementById('mock-data-status');
      status.classList.add('hidden');
      status.textContent = '';
      document.getElementById('mock-data-submit-btn').disabled = false;
      if (typeof dlg.showModal === 'function') dlg.showModal(); else dlg.setAttribute('open', '');
    }

    function closeMockDataModal() {
      const dlg = document.getElementById('mock-data-dialog');
      if (dlg) { if (typeof dlg.close === 'function') dlg.close(); else dlg.removeAttribute('open'); }
    }

    async function submitMockDataGeneration() {
      const rows = parseInt(document.getElementById('mock-data-rows').value, 10);
      if (!rows || rows < 1 || rows > 5000) {
        showNativeAlert('Please enter a row count between 1 and 5000.', 'Invalid Input'); return;
      }
      const tableName = activeGridTable;
      const currentDb = schemaCatalogCache?.database_name || activeConnection.database_name || '';
      const statusEl = document.getElementById('mock-data-status');
      const btn = document.getElementById('mock-data-submit-btn');

      statusEl.className = 'p-3 rounded text-[11px] bg-slate-800 text-slate-300';
      statusEl.textContent = `⏳ Generating ${rows.toLocaleString()} rows into ${tableName}…`;
      statusEl.classList.remove('hidden');
      btn.disabled = true;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/table/${encodeURIComponent(tableName)}/mock-data`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ rows, database: currentDb })
        });
        const data = await res.json();
        if (!res.ok) {
          statusEl.className = 'p-3 rounded text-[11px] bg-red-950 text-red-300 border border-red-800';
          statusEl.textContent = `❌ ${data.error || 'Generation failed'}`;
          btn.disabled = false;
          return;
        }
        statusEl.className = 'p-3 rounded text-[11px] bg-emerald-950 text-emerald-300 border border-emerald-800';
        statusEl.textContent = `✅ Inserted ${data.inserted.toLocaleString()} rows in ${data.duration_ms}ms`;
        btn.disabled = false;
        // Reload table view
        setTimeout(() => {
          closeMockDataModal();
          if (typeof loadTableRows === 'function') loadTableRows(tableName, currentDb);
          else if (typeof refreshGridFromToolbar === 'function') refreshGridFromToolbar();
        }, 1200);
      } catch (err) {
        statusEl.className = 'p-3 rounded text-[11px] bg-red-950 text-red-300 border border-red-800';
        statusEl.textContent = `❌ Network error: ${err.message}`;
        btn.disabled = false;
      }
    }

    // --- CSV Import & Visual Column Mapping Logic ---
    let currentCSVFile = null;
    let currentCSVHeaders = [];
    let currentCSVSamples = [];

    function openCSVImportModal() {
      const dlg = document.getElementById('csv-import-dialog');
      if (!dlg) return;

      // Populate target tables dropdown
      const select = document.getElementById('csv-import-target-table');
      select.innerHTML = '';
      const tables = schemaCatalogCache?.tables || [];
      if (tables.length === 0) {
        select.innerHTML = '<option value="">(No tables available)</option>';
      } else {
        tables.forEach(t => {
          const opt = document.createElement('option');
          opt.value = t.name;
          opt.textContent = t.name + (t.type === 'VIEW' ? ' (view)' : '');
          select.appendChild(opt);
        });
      }

      // If active tab is viewing a table, auto-select it
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && activeTab.tableName) {
        select.value = activeTab.tableName;
      }

      // Reset fields
      const fileInput = document.getElementById('csv-import-file-input');
      if (fileInput) fileInput.value = '';
      currentCSVFile = null;
      currentCSVHeaders = [];
      currentCSVSamples = [];
      document.getElementById('csv-file-summary').classList.add('hidden');
      document.getElementById('csv-mapping-wrapper').classList.add('hidden');
      document.getElementById('csv-mapping-tbody').innerHTML = '';
      document.getElementById('csv-import-status').classList.add('hidden');
      document.getElementById('csv-btn-start-import').disabled = true;
      document.getElementById('csv-import-row-preview').textContent = '';

      dlg.showModal();
    }

    function closeCSVImportModal() {
      const dlg = document.getElementById('csv-import-dialog');
      if (dlg) dlg.close();
    }

    function onCSVTargetTableChanged() {
      if (currentCSVHeaders.length > 0) {
        renderCSVMappingRows();
      }
    }

    function handleCSVFileSelected(event) {
      const file = event.target.files?.[0];
      if (!file) return;

      currentCSVFile = file;
      const statusEl = document.getElementById('csv-import-status');
      statusEl.classList.add('hidden');

      const reader = new FileReader();
      // Read first 64KB for instant client-side header detection and delimiter sniffing
      reader.onload = function(e) {
        const text = e.target.result || '';
        parseCSVPreview(text, file.name, file.size);
      };
      reader.readAsText(file.slice(0, 65536));
    }

    function parseCSVPreview(rawText, fileName, fileSize) {
      // Strip BOM if present
      let clean = rawText;
      if (clean.charCodeAt(0) === 0xFEFF) {
        clean = clean.slice(1);
      }

      const lines = clean.split(/\r?\n/).filter(l => l.trim().length > 0);
      if (lines.length === 0) {
        showNativeAlert('The selected file appears to be empty.', 'CSV Import');
        return;
      }

      // Sniff delimiter if set to auto
      const delimSelect = document.getElementById('csv-import-delimiter');
      let delimiter = delimSelect.value;
      if (delimiter === 'auto') {
        const counts = { ',': 0, ';': 0, '\t': 0, '|': 0 };
        const testLines = lines.slice(0, 5);
        testLines.forEach(l => {
          Object.keys(counts).forEach(d => {
            counts[d] += (l.split(d).length - 1);
          });
        });
        let best = ',';
        let max = 0;
        for (const d in counts) {
          if (counts[d] > max) {
            max = counts[d];
            best = d;
          }
        }
        delimiter = best;
      }

      function parseCSVLine(line, sep) {
        const res = [];
        let cur = '';
        let inQuotes = false;
        for (let i = 0; i < line.length; i++) {
          const char = line[i];
          if (char === '"') {
            if (inQuotes && line[i+1] === '"') {
              cur += '"';
              i++;
            } else {
              inQuotes = !inQuotes;
            }
          } else if (char === sep && !inQuotes) {
            res.push(cur.trim());
            cur = '';
          } else {
            cur += char;
          }
        }
        res.push(cur.trim());
        return res;
      }

      currentCSVHeaders = parseCSVLine(lines[0], delimiter);
      currentCSVSamples = [];
      for (let i = 1; i < Math.min(lines.length, 4); i++) {
        currentCSVSamples.push(parseCSVLine(lines[i], delimiter));
      }

      // Display file summary
      const sizeKB = (fileSize / 1024).toFixed(1);
      const delimLabel = delimiter === '\t' ? 'Tab (\\t)' : `'${delimiter}'`;
      const summaryEl = document.getElementById('csv-file-summary');
      summaryEl.textContent = `✓ ${fileName} (${sizeKB} KB) — Detected ${currentCSVHeaders.length} columns, delimiter ${delimLabel}`;
      summaryEl.classList.remove('hidden');

      renderCSVMappingRows();
      document.getElementById('csv-mapping-wrapper').classList.remove('hidden');
      document.getElementById('csv-btn-start-import').disabled = false;
    }

    function renderCSVMappingRows() {
      const targetTable = document.getElementById('csv-import-target-table').value;
      const tableMeta = (schemaCatalogCache?.tables || []).find(t => t.name === targetTable);
      const targetColumns = tableMeta ? (tableMeta.columns || []) : [];

      const tbody = document.getElementById('csv-mapping-tbody');
      tbody.innerHTML = '';

      currentCSVHeaders.forEach((header, idx) => {
        const tr = document.createElement('tr');
        tr.className = 'hover:bg-slate-900/60';

        const tdNum = document.createElement('td');
        tdNum.className = 'p-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none';
        tdNum.textContent = idx + 1;

        // Sample text
        const samples = currentCSVSamples.map(r => r[idx] || '').filter(Boolean).slice(0, 2);
        const sampleText = samples.length > 0 ? ` (e.g. "${samples.join('", "')}")` : '';

        // Auto-match target column by name (case-insensitive)
        const cleanHeader = header.toLowerCase().replace(/[^a-z0-9_]/g, '');
        let matchedCol = '';
        targetColumns.forEach(c => {
          const cleanCol = c.name.toLowerCase().replace(/[^a-z0-9_]/g, '');
          if (cleanCol === cleanHeader || cleanCol === cleanHeader.replace(/^(tbl_|col_)/, '')) {
            matchedCol = c.name;
          }
        });

        // Left: CSV Column
        const tdLeft = document.createElement('td');
        tdLeft.className = 'p-2 pl-3';
        tdLeft.innerHTML = `
          <div class="font-medium text-slate-200">${escapeHtml(header || `Col #${idx + 1}`)}</div>
          <div class="text-[10px] text-slate-500 truncate max-w-xs">${escapeHtml(sampleText)}</div>
        `;

        // Middle: Arrow
        const tdMid = document.createElement('td');
        tdMid.className = 'p-2 text-center text-slate-600 text-xs';
        tdMid.innerHTML = renderIcon('arrow-right', 'w-3.5 h-3.5 text-slate-500 mx-auto');

        // Right: Target Table Column Dropdown
        const tdRight = document.createElement('td');
        tdRight.className = 'p-2 pr-3';
        const select = document.createElement('select');
        select.dataset.csvIndex = idx;
        select.className = 'csv-col-map-select w-full bg-slate-900 border border-slate-700 rounded px-2 py-1 text-slate-200 outline-none focus:border-app-accent font-mono text-xs';

        // Add [Skip] option
        const skipOpt = document.createElement('option');
        skipOpt.value = 'SKIP';
        skipOpt.textContent = '[Skip / Do Not Import]';
        skipOpt.className = 'text-slate-500';
        select.appendChild(skipOpt);

        targetColumns.forEach(c => {
          const opt = document.createElement('option');
          opt.value = c.name;
          opt.textContent = `${c.name} (${c.type || 'TEXT'})`;
          if (c.name === matchedCol) {
            opt.selected = true;
          }
          select.appendChild(opt);
        });

        // If no match found by name, default to index match if within bounds
        if (!matchedCol && targetColumns[idx]) {
          select.value = targetColumns[idx].name;
        }

        tdRight.appendChild(select);

        tr.appendChild(tdNum);
        tr.appendChild(tdLeft);
        tr.appendChild(tdMid);
        tr.appendChild(tdRight);
        tbody.appendChild(tr);
      });
    }

    async function startCSVImport() {
      if (!currentCSVFile) return;
      const targetTable = document.getElementById('csv-import-target-table').value;
      if (!targetTable) {
        return showNativeAlert('Please select a target table', 'CSV Import');
      }

      const selects = document.querySelectorAll('.csv-col-map-select');
      const mapping = {};
      let mappedCount = 0;
      selects.forEach(s => {
        const idx = s.dataset.csvIndex;
        const target = s.value;
        if (target && target !== 'SKIP') {
          mapping[idx] = target;
          mappedCount++;
        }
      });

      if (mappedCount === 0) {
        return showNativeAlert('Please map at least one column to import.', 'CSV Import');
      }

      const delimiter = document.getElementById('csv-import-delimiter').value;
      const emptyAsNull = document.getElementById('csv-import-empty-as-null').checked;
      const skipErrors = document.getElementById('csv-import-skip-errors').checked;

      const formData = new FormData();
      formData.append('file', currentCSVFile);
      formData.append('delimiter', delimiter);
      formData.append('empty_as_null', emptyAsNull ? 'true' : 'false');
      formData.append('skip_errors', skipErrors ? 'true' : 'false');
      formData.append('mapping', JSON.stringify(mapping));

      const btn = document.getElementById('csv-btn-start-import');
      btn.disabled = true;
      btn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Importing...</span>`;

      const statusEl = document.getElementById('csv-import-status');
      statusEl.className = 'p-3 rounded text-xs bg-sky-950 border border-sky-800 text-sky-300';
      statusEl.textContent = 'Uploading and streaming CSV rows into database...';
      statusEl.classList.remove('hidden');

      try {
        const activeDB = (activeConnection && activeConnection.database_name) || '';
        const url = `/api/session/${activeConnection.id}/table/${encodeURIComponent(targetTable)}/import-csv?db=${encodeURIComponent(activeDB)}`;
        const res = await fetch(url, {
          method: 'POST',
          body: formData
        });

        const data = await res.json();
        if (!res.ok || data.error) {
          throw new Error(data.error || 'Import failed');
        }

        statusEl.className = 'p-3 rounded text-xs bg-emerald-950 border border-emerald-800 text-emerald-300';
        let msg = `✓ Successfully imported ${data.imported_rows} rows into '${targetTable}'.`;
        if (data.skipped_rows > 0) {
          msg += ` (${data.skipped_rows} skipped).`;
        }
        statusEl.textContent = msg;

        // If user is currently viewing this table, refresh data
        const activeTab = tabs.find(t => t.id === activeTabId);
        if (activeTab && activeTab.tableName === targetTable && activeTab.tableMode === 'data') {
          setTimeout(() => executeQuery(activeTab.sql), 500);
        }

        setTimeout(() => {
          closeCSVImportModal();
          showNativeAlert(msg, 'CSV Import Completed');
        }, 800);
      } catch (err) {
        statusEl.className = 'p-3 rounded text-xs bg-rose-950 border border-rose-800 text-rose-300';
        statusEl.textContent = 'Import error: ' + err.message;
      } finally {
        btn.disabled = false;
        btn.innerHTML = '<svg class="w-3.5 h-3.5 inline mr-1"><use href="#icon-upload"/></svg><span>Start Import</span>';
      }
    }

    function downloadFile(content, name, type) {
      const blob = new Blob([content], { type });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = name;
      a.click();
    }

    // --- Keyboard Shortcuts ---
    let keyboardShortcutsInitialized = false;
    function setupKeyboardShortcuts() {
      // ponytail: Idempotency guard prevents duplicate keydown listeners on re-init
      if (keyboardShortcutsInitialized) return;
      keyboardShortcutsInitialized = true;

      const searchInput = document.getElementById('schema-search-input');
      if (searchInput) {
        searchInput.addEventListener('input', (e) => {
          schemaSearchSelectedIndex = 0;
          renderSchemaSearchResults(e.target.value);
        });

        searchInput.addEventListener('keydown', (e) => {
          if (e.key === 'ArrowDown') {
            e.preventDefault();
            if (schemaSearchResults.length > 0) {
              schemaSearchSelectedIndex = (schemaSearchSelectedIndex + 1) % schemaSearchResults.length;
              renderSchemaSearchResults(searchInput.value);
            }
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            if (schemaSearchResults.length > 0) {
              schemaSearchSelectedIndex = (schemaSearchSelectedIndex - 1 + schemaSearchResults.length) % schemaSearchResults.length;
              renderSchemaSearchResults(searchInput.value);
            }
          } else if (e.key === 'Enter') {
            e.preventDefault();
            selectSchemaSearchResult(schemaSearchSelectedIndex, e.shiftKey);
          } else if (e.key === 'Escape') {
            e.preventDefault();
            closeSchemaSearch();
          }
        });
      }

      const schemaFilterEl = document.getElementById('schema-filter-input');
      if (schemaFilterEl) {
        schemaFilterEl.addEventListener('keydown', (e) => {
          if (e.key === 'Escape') {
            e.stopPropagation();
            clearSchemaFilter();
            schemaFilterEl.blur();
          } else if (e.key === 'ArrowDown' || e.key === 'Enter') {
            const firstClickable = document.querySelector('#schema-tree [data-name]');
            if (firstClickable) {
              e.preventDefault();
              firstClickable.click();
            }
          }
        });
      }

      const dbSwitcherBtn = document.getElementById('database-switcher-btn');
      if (dbSwitcherBtn) {
        dbSwitcherBtn.addEventListener('keydown', (e) => {
          if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            toggleDatabaseDropdown(true);
          }
        });
      }

      const sessionSwitcherBtn = document.getElementById('session-switcher-btn');
      if (sessionSwitcherBtn) {
        sessionSwitcherBtn.addEventListener('keydown', (e) => {
          if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            toggleSessionDropdown(true);
          }
        });
      }

      const tabDbPill = document.getElementById('tab-database-pill-container');
      if (tabDbPill) {
        tabDbPill.addEventListener('keydown', (e) => {
          if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            openTabDatabaseSwitcher(e);
          }
        });
      }

      window.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
          e.preventDefault();
          // ponytail: If CodeMirror is focused, its extraKeys already executes the query; prevent bubbling double-dispatch
          if (cmEditor && cmEditor.hasFocus()) return;
          if (e.shiftKey) explainActiveQuery();
          else if (e.altKey) runActiveQuery(false, '', null, null, true);
          else runActiveQuery();
        } else if (e.key === 'F5' && !['INPUT', 'TEXTAREA'].includes(document.activeElement?.tagName) && !document.activeElement?.closest('.CodeMirror')) {
          e.preventDefault();
          runActiveQuery(false, '', null, null, true);
        } else if ((e.ctrlKey || e.metaKey) && (e.key === ',' || e.code === 'Comma')) {
          e.preventDefault();
          openSettingsModal();
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's' && !e.shiftKey) {
          if (document.activeElement && document.activeElement.id === 'table-notes-textarea') {
            e.preventDefault();
            saveActiveTableNote();
          }
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'p') {

          e.preventDefault();
          toggleSchemaSearch();
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
          e.preventDefault();
          toggleCommandPalette();
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'l') {
          e.preventDefault();
          toggleAISidebar();
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {
          e.preventDefault();
          toggleLeftSidebar();
        } else if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === 'n') {
          e.preventDefault();
          window.open(window.location.href, '_blank');
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 't') {
          e.preventDefault();
          addNewQueryTab();
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'w') {
          e.preventDefault();
          closeTab(activeTabId);
        } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f' && !e.shiftKey) {
          const activeTab = tabs.find(t => t.id === activeTabId);
          if (activeTab && activeTab.isTableViewer && (activeTab.tableMode === 'data' || !activeTab.tableMode)) {
            e.preventDefault();
            focusTableFilter();
          }
        } else if (e.key === '/' && !['INPUT', 'TEXTAREA'].includes(document.activeElement?.tagName) && !document.activeElement?.closest('.CodeMirror')) {
          e.preventDefault();
          const sFilter = document.getElementById('schema-filter-input');
          if (sFilter) {
            sFilter.focus();
            sFilter.select();
          }
        } else if ((e.key === '?' || e.key === 'F1') && !['INPUT', 'TEXTAREA'].includes(document.activeElement?.tagName) && !document.activeElement?.closest('.CodeMirror')) {
          e.preventDefault();
          toggleShortcutsModal();
        } else if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === 's') {
          e.preventDefault();
          openSnippetsManager();
        } else if (e.key === 'Escape') {
          closeAllContextMenus();
          closeTabContextMenu();
          closeTableContextMenu();
          closeShortcutsModal();
          closeSafetyModal();
          closeImpactModal();
          closeConnModal();
          closeDDLModal();
          closeVaultModal();
          closeCommandPalette();
          closeSchemaSearch();
          closeAISettingsModal();
          closeSnippetsManager();
          closeSnippetParamModal();
          closeDatabaseDiagnosticsModal();
          closePrivilegeMatrixModal();
          closeFKPeekPopover();
          closeProcessDetailModal();
          closeProcessContextMenu();
          closeProfileView();
          closeUpdateDialog();
          closeChangelogDialog();
          closeLockInspectorModal();
          closeJsonCellModal();
          closeNoteComposerModal();
          closeMockDataModal();
          closeCSVImportModal();
          closeRunSQLScriptModal();
          closeStructureSyncModal();
          closeDataTransferModal();
          closeDataSyncModal();
          closeDumpDatabaseModal();
          closeSmartSeedModal();
          toggleSessionDropdown(false);
          toggleDatabaseDropdown(false);
          toggleTopMonitorMenu(false);
          toggleTopDBToolsMenu(false);
          toggleTopSettingsMenu(false);
          toggleExportCSVMenu(false);
          toggleNotesDrawer(false);
          ['erd-dialog', 'table-creator-dialog', 'view-designer-dialog', 'trigger-designer-dialog', 'routine-designer-dialog', 'event-designer-dialog', 'database-creator-dialog', 'safety-inspection-dialog', 'ddl-review-dialog'].forEach(id => {
            const dlg = document.getElementById(id);
            if (dlg && typeof dlg.close === 'function') dlg.close();
          });
        }
      });

      // Close native dialogs when clicking on backdrop
      ['snippets-manager-dialog', 'snippet-param-dialog', 'database-creator-dialog', 'csv-import-dialog', 'mock-data-dialog', 'schema-search-modal', 'db-diagnostics-dialog', 'privilege-matrix-dialog', 'pl-detail-modal', 'update-dialog', 'changelog-dialog', 'note-composer-dialog', 'erd-dialog', 'dump-database-dialog', 'smart-seed-dialog', 'run-sql-dialog', 'structure-sync-dialog', 'data-transfer-dialog', 'data-sync-dialog', 'table-creator-dialog', 'view-designer-dialog', 'trigger-designer-dialog', 'routine-designer-dialog', 'event-designer-dialog', 'safety-inspection-dialog', 'ddl-review-dialog'].forEach(id => {
        const dlg = document.getElementById(id);
        if (dlg) {
          dlg.addEventListener('click', (e) => {
            if (e.target === dlg && typeof dlg.close === 'function') {
              dlg.close();
            }
          });
        }
      });
    }

    // --- Sidebar Visibility & LocalStorage Persistence ---
    function initSidebarVisibility() {
      try {
        const leftSaved = localStorage.getItem('sqlfox_left_sidebar_visible');
        if (leftSaved !== null) {
          const isLeftVisible = leftSaved === 'true';
          const leftEl = document.getElementById('left-sidebar');
          if (leftEl) {
            if (isLeftVisible) leftEl.classList.remove('hidden');
            else leftEl.classList.add('hidden');
          }
        }

        const aiSaved = localStorage.getItem('sqlfox_copilot_visible');
        if (aiSaved !== null) {
          const isAIVisible = aiSaved === 'true';
          const aiEl = document.getElementById('ai-sidebar');
          if (aiEl) {
            if (isAIVisible) {
              aiEl.classList.remove('hidden');
              updateAISidebarMode();
      setTimeout(() => { if (typeof updateTabOverflow === 'function') updateTabOverflow(); }, 150);
            } else {
              aiEl.classList.add('hidden');
            }
          }
        } else {
          const aiEl = document.getElementById('ai-sidebar');
          if (aiEl) {
            if (window.innerWidth < 768) {
              aiEl.classList.add('hidden');
            } else {
              updateAISidebarMode();
      setTimeout(() => { if (typeof updateTabOverflow === 'function') updateTabOverflow(); }, 150);
            }
          }
        }
      } catch (_) {}
    }

    function toggleAISidebar(forceState) {
      const el = document.getElementById('ai-sidebar');
      if (!el) return;
      const willBeHidden = typeof forceState === 'boolean' ? !forceState : !el.classList.contains('hidden');
      if (willBeHidden) {
        el.classList.add('hidden');
      } else {
        el.classList.remove('hidden');
        updateAISidebarMode();
      setTimeout(() => { if (typeof updateTabOverflow === 'function') updateTabOverflow(); }, 150);
      }

      try {
        localStorage.setItem('sqlfox_copilot_visible', (!willBeHidden).toString());
      } catch (_) {}
    }

    function toggleLeftSidebar(forceState) {
      const el = document.getElementById('left-sidebar');
      if (!el) return;
      const willBeHidden = typeof forceState === 'boolean' ? !forceState : !el.classList.contains('hidden');
      if (willBeHidden) el.classList.add('hidden');
      else el.classList.remove('hidden');

      updateAISidebarMode();
      setTimeout(() => { if (typeof updateTabOverflow === 'function') updateTabOverflow(); }, 150);

      try {
        localStorage.setItem('sqlfox_left_sidebar_visible', (!willBeHidden).toString());
      } catch (_) {}
    }
    function toggleCommandPalette() {
      const modal = document.getElementById('command-palette-modal');
      modal.classList.toggle('hidden');
      if (!modal.classList.contains('hidden')) {
        document.getElementById('palette-input').value = '';
        renderPaletteActions('');
        document.getElementById('palette-input').focus();
      }
    }
    function closeCommandPalette() {
      document.getElementById('command-palette-modal').classList.add('hidden');
    }

    function renderPaletteActions(filter) {
      const actions = [
        { name: 'Schema Search (Quick Open Table/Column)', shortcut: 'Ctrl+P', run: () => toggleSchemaSearch() },
        { name: 'Keyboard Shortcuts Cheat Sheet', shortcut: '?', run: () => openShortcutsModal() },
        { name: 'Query Snippets Manager', shortcut: 'Ctrl+Shift+S', run: () => openSnippetsManager() },
        { name: 'Database Storage & Health Inspector', shortcut: '', run: () => openDatabaseDiagnosticsModal() },
        { name: 'Table Storage Diagnostics', shortcut: '', run: () => openDatabaseDiagnosticsModal() },
        { name: 'User Accounts & Privilege Matrix', shortcut: '', run: () => openPrivilegeMatrixModal() },
        { name: 'Lock Contention & Root Blocker Analysis', shortcut: '', run: () => openLockInspector() },
        { name: 'Create Database', shortcut: '', run: () => openCreateDatabaseModal() },
        { name: 'Execute Query', shortcut: 'Ctrl+Enter', run: () => runActiveQuery() },
        { name: 'Execute All Statements (Full Script)', shortcut: 'Ctrl+Alt+Enter / F5', run: () => runActiveQuery(false, '', null, null, true) },
        { name: 'Explain Query Plan', shortcut: 'Ctrl+Shift+Enter', run: () => explainActiveQuery() },
        { name: 'Save Query as Operational Note', shortcut: '', run: () => saveActiveQueryToNotes() },
        { name: 'Open Operational Notes Inbox', shortcut: '', run: () => toggleNotesDrawer(true) },
        { name: 'New Query Tab', shortcut: 'Ctrl+T', run: () => addNewQueryTab() },
        { name: 'Open New Window', shortcut: 'Ctrl+Shift+N', run: () => window.open(window.location.href, '_blank') },
        { name: 'Open in Web Browser', shortcut: '', run: () => openCurrentInBrowser() },
        { name: 'Copy App URL to Clipboard', shortcut: '', run: () => copyCurrentAppURL() },
        { name: 'Shut Down SqlFox Daemon', shortcut: '', run: () => confirmShutDownServer() },
        { name: 'New Connection', shortcut: '', run: () => openNewConnModal() },
        { name: 'Toggle Read-Only Mode', shortcut: '', run: () => toggleReadOnly() },
        { name: 'Settings & Configuration', shortcut: 'Ctrl+,', run: () => openSettingsModal('vault') },
        { name: 'AI Copilot Settings (BYOK)', shortcut: '', run: () => openSettingsModal('ai') },
        { name: 'Cloud Sync & GitHub Replication', shortcut: '', run: () => openCloudSyncModal('github') },
        { name: 'Download Notes Bundle (.zip)', shortcut: '', run: () => doExportNotesBundle() },
        { name: 'Export Vault (.sqlfox-vault)', shortcut: '', run: () => openExportImportModal() },
        { name: 'Toggle AI Copilot', shortcut: 'Ctrl+L', run: () => toggleAISidebar() },
        { name: 'Check for Updates', shortcut: '', run: () => checkOrOpenUpdate(true) },
        { name: 'View Changelog', shortcut: '', run: () => openChangelogDialog() },
        { name: 'Open Documentation (GitHub)', shortcut: '', run: () => window.open('https://github.com/herliansyah/sqlfox/blob/main/docs/USER_GUIDE.md', '_blank', 'noopener,noreferrer') }
      ];

      const list = document.getElementById('palette-actions-list');
      const filtered = actions.filter(a => a.name.toLowerCase().includes(filter.toLowerCase()));
      list.innerHTML = filtered.map(a => `
        <div onclick="executePaletteAction('${a.name}')" class="p-2 flex items-center justify-between hover:bg-slate-800 rounded cursor-pointer">
          <span class="text-slate-200">${a.name}</span>
          <kbd class="text-[10px] text-slate-500 font-mono">${a.shortcut}</kbd>
        </div>
      `).join('');
    }

    function filterPaletteActions(e) {
      renderPaletteActions(e.target.value);
    }

    function executePaletteAction(name) {
      closeCommandPalette();
      if (name.startsWith('Schema Search')) toggleSchemaSearch();
      else if (name === 'Keyboard Shortcuts Cheat Sheet') openShortcutsModal();
      else if (name === 'Query Snippets Manager') openSnippetsManager();
      else if (name === 'Database Storage & Health Inspector' || name === 'Table Storage Diagnostics') openDatabaseDiagnosticsModal();
      else if (name === 'User Accounts & Privilege Matrix') openPrivilegeMatrixModal();
      else if (name.startsWith('Lock Contention')) openLockInspector();
      else if (name === 'Create Database') openCreateDatabaseModal();
      else if (name === 'Execute Query') runActiveQuery();
      else if (name === 'Execute All Statements (Full Script)') runActiveQuery(false, '', null, null, true);
      else if (name === 'Explain Query Plan') explainActiveQuery();
      else if (name === 'Save Query as Operational Note') saveActiveQueryToNotes();
      else if (name === 'Open Operational Notes Inbox') toggleNotesDrawer(true);
      else if (name === 'New Query Tab') addNewQueryTab();
      else if (name === 'Open New Window') window.open(window.location.href, '_blank');
      else if (name === 'Open in Web Browser') openCurrentInBrowser();
      else if (name === 'Copy App URL to Clipboard') copyCurrentAppURL();
      else if (name === 'Shut Down SqlFox Daemon') confirmShutDownServer();
      else if (name === 'New Connection') openNewConnModal();
      else if (name === 'Toggle Read-Only Mode') toggleReadOnly();
      else if (name === 'Settings & Configuration') openSettingsModal('vault');
      else if (name === 'AI Copilot Settings (BYOK)') openSettingsModal('ai');
      else if (name === 'Cloud Sync & GitHub Replication') openCloudSyncModal('github');
      else if (name === 'Download Notes Bundle (.zip)') doExportNotesBundle();
      else if (name === 'Export Vault (.sqlfox-vault)') openExportImportModal();
      else if (name === 'Toggle AI Copilot') toggleAISidebar();
      else if (name === 'Check for Updates') checkOrOpenUpdate(true);
      else if (name === 'View Changelog') openChangelogDialog();
      else if (name === 'Open Documentation (GitHub)') window.open('https://github.com/herliansyah/sqlfox/blob/main/docs/USER_GUIDE.md', '_blank', 'noopener,noreferrer');
    }

    // --- Table Diagnostics Handlers ---
    async function renderTableDiagnostics(tableName) {
      if (!activeConnection || !tableName) return;
      const activeTab = tabs.find(t => t.id === activeTabId);
      const currentDb = (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';

      document.getElementById('td-data-size').textContent = 'Loading...';
      document.getElementById('td-index-size').textContent = '...';
      document.getElementById('td-overhead-size').textContent = '...';
      document.getElementById('td-rows-count').textContent = '...';
      document.getElementById('td-maintenance-output').classList.add('hidden');

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/tables?database=${encodeURIComponent(currentDb)}&table=${encodeURIComponent(tableName)}`);
        const data = await res.json();
        if (!res.ok || !Array.isArray(data) || data.length === 0) {
          document.getElementById('td-data-size').textContent = '-';
          return;
        }
        const t = data[0];
        document.getElementById('td-data-size').textContent = formatBytes(t.data_size);
        document.getElementById('td-index-size').textContent = formatBytes(t.index_size);
        document.getElementById('td-overhead-size').textContent = formatBytes(t.overhead_size);
        document.getElementById('td-rows-count').textContent = Number(t.rows).toLocaleString();
        document.getElementById('td-engine').textContent = t.engine || 'Standard';
        document.getElementById('td-collation').textContent = t.collation || '-';
        document.getElementById('td-row-format').textContent = t.row_format || '-';
        document.getElementById('td-created').textContent = t.created_at || '-';
        document.getElementById('td-updated').textContent = t.updated_at || '-';

        checkTableRedundantIndexes(tableName);
      } catch (err) {
        console.error('Failed to load table diagnostics:', err);
      }
    }

    async function runTableMaintenance(action) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeConnection || !activeTab || !activeTab.tableName) return;
      const currentDb = activeTab.database || (schemaCatalogCache && schemaCatalogCache.database_name) || '';
      const outBox = document.getElementById('td-maintenance-output');
      outBox.classList.remove('hidden');
      outBox.innerHTML = `<span class="text-sky-400">Executing ${action} TABLE on ${escapeHtml(activeTab.tableName)}...</span>`;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/maintenance`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            database: currentDb,
            tables: [activeTab.tableName],
            action: action
          })
        });
        const results = await res.json();
        if (!res.ok) {
          outBox.innerHTML = `<span class="text-red-400">Error: ${escapeHtml(results.error || 'Maintenance action failed')}</span>`;
          return;
        }
        outBox.innerHTML = results.map(r => `
          <div><strong>${escapeHtml(r.op)}:</strong> <span class="${r.status === 'OK' || r.status === 'status' ? 'text-emerald-400' : 'text-amber-400'}">${escapeHtml(r.status)}</span> - ${escapeHtml(r.message || '')}</div>
        `).join('');
        setTimeout(() => renderTableDiagnostics(activeTab.tableName), 500);
      } catch (err) {
        outBox.innerHTML = `<span class="text-red-400">Request error: ${escapeHtml(err.message)}</span>`;
      }
    }

    async function checkTableRedundantIndexes(tableName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const currentDb = (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';
      const container = document.getElementById('td-redundant-indexes-container');
      if (!container) return;

      container.innerHTML = '<div class="text-slate-500 italic py-1">Analyzing index prefixes and uniqueness...</div>';

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/redundant-indexes?database=${encodeURIComponent(currentDb)}&table=${encodeURIComponent(tableName)}`);
        const list = await res.json();
        if (!res.ok || !Array.isArray(list) || list.length === 0) {
          container.innerHTML = '<div class="text-emerald-400 flex items-center space-x-1.5 py-1"><span>✓</span><span>No redundant or duplicate indexes detected on this table.</span></div>';
          return;
        }

        container.innerHTML = list.map(item => `
          <div class="p-2.5 bg-amber-950/20 border border-amber-800/60 rounded flex items-center justify-between">
            <div class="space-y-0.5">
              <div class="flex items-center space-x-2">
                <span class="font-bold text-amber-300">${escapeHtml(item.redundant_index)}</span>
                <span class="text-[10px] bg-amber-900/60 text-amber-200 px-1.5 py-0.2 rounded font-mono">${escapeHtml((item.redundant_cols || []).join(', '))}</span>
                <span class="text-slate-400 text-xs">shadowed by</span>
                <span class="font-bold text-sky-300">${escapeHtml(item.shadowed_by)}</span>
              </div>
              <div class="text-[11px] text-slate-400">${escapeHtml(item.reason)}</div>
            </div>
            <button data-drop-sql="${escapeHtml(item.drop_sql)}" data-table="${escapeHtml(tableName)}" onclick="promptDropRedundantIndex(this.dataset.dropSql, this.dataset.table)" class="bg-rose-900 hover:bg-rose-800 text-rose-200 px-2.5 py-1 rounded text-xs transition">
              Drop Index
            </button>
          </div>
        `).join('');
      } catch (err) {
        container.innerHTML = `<div class="text-red-400 text-xs">Failed to scan indexes: ${escapeHtml(err.message)}</div>`;
      }
    }

    function promptDropRedundantIndex(dropSql, tableName) {
      if (!dropSql) return;
      openDDLReviewModal(dropSql, async () => {
        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/execute`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              sql: dropSql,
              confirmed: true,
              challenge_input: tableName,
              query_id: 'drop_idx_' + Date.now()
            })
          });
          const data = await res.json();
          if (!res.ok) {
            showNativeAlert(data.error || 'Failed to drop index', 'DDL Execution Error');
            return;
          }
          showNativeAlert('Redundant index dropped successfully.', 'Index Cleaned');
          await refreshSchema();
          checkTableRedundantIndexes(tableName);
        } catch (err) {
          showNativeAlert('Drop index failed: ' + err.message, 'Network Error');
        }
      });
    }

    // --- Database Storage & Health Inspector Handlers ---
    let dbDiagAllDatabases = [];
    let dbDiagSelectedDbName = '';
    let dbDiagShowSystemDbs = false;
    let dbDiagLevel = 'databases'; // 'databases' or 'tables'

    function openDatabaseDiagnosticsModal(targetDb = null) {
      if (!activeConnection) {
        showNativeAlert('Please connect to a database first.', 'No Active Session');
        return;
      }
      const dlg = document.getElementById('db-diagnostics-dialog');
      if (!dlg) return;

      if (targetDb) {
        inspectDatabaseTables(targetDb);
      } else {
        showDbDiagDatabasesView();
        loadAllDatabasesDiagnostics();
      }

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    function closeDatabaseDiagnosticsModal() {
      const dlg = document.getElementById('db-diagnostics-dialog');
      if (dlg) {
        if (typeof dlg.close === 'function') dlg.close();
        else dlg.removeAttribute('open');
      }
    }

    function showDbDiagDatabasesView() {
      dbDiagLevel = 'databases';
      document.getElementById('dbdiag-level-databases')?.classList.remove('hidden');
      document.getElementById('dbdiag-level-tables')?.classList.add('hidden');
      document.getElementById('dbdiag-btn-back')?.classList.add('hidden');
      document.getElementById('dbdiag-breadcrumb-sep')?.classList.add('hidden');
      const bcDb = document.getElementById('dbdiag-breadcrumb-db');
      if (bcDb) {
        bcDb.classList.add('hidden');
        bcDb.textContent = '';
      }
      const titleEl = document.getElementById('dbdiag-modal-title');
      if (titleEl) titleEl.textContent = 'Database Storage & Health Inspector';
      const descEl = document.getElementById('dbdiag-header-desc');
      if (descEl) descEl.textContent = 'Session-wide physical disk consumption, table footprints, and health recommendations';

      updateDbDiagOverviewCards();
    }

    function updateDbDiagOverviewCards() {
      const dbs = dbDiagAllDatabases.filter(d => dbDiagShowSystemDbs || !d.is_system);
      let totalStorage = 0;
      let totalTables = 0;
      let totalOverhead = 0;

      dbs.forEach(d => {
        totalStorage += (d.total_size || (d.data_size + d.index_size) || 0);
        totalTables += (d.table_count || 0);
        totalOverhead += (d.overhead_size || 0);
      });

      const lbl1 = document.getElementById('dbdiag-stat-label-1');
      const val1 = document.getElementById('dbdiag-stat-val-1');
      if (lbl1) lbl1.textContent = 'Total Databases';
      if (val1) val1.textContent = dbs.length.toString();

      const lbl2 = document.getElementById('dbdiag-stat-label-2');
      const val2 = document.getElementById('dbdiag-stat-val-2');
      if (lbl2) lbl2.textContent = 'Total Storage';
      if (val2) val2.textContent = formatBytes(totalStorage);

      const lbl3 = document.getElementById('dbdiag-stat-label-3');
      const val3 = document.getElementById('dbdiag-stat-val-3');
      if (lbl3) lbl3.textContent = 'Total Tables';
      if (val3) val3.textContent = totalTables.toLocaleString();

      const lbl4 = document.getElementById('dbdiag-stat-label-4');
      const val4 = document.getElementById('dbdiag-stat-val-4');
      if (lbl4) lbl4.textContent = 'Fragmentation Overhead';
      if (val4) val4.textContent = formatBytes(totalOverhead);

      const t1 = document.getElementById('dbdiag-total-tables'); if (t1) t1.textContent = totalTables.toString();
      const t2 = document.getElementById('dbdiag-total-data'); if (t2) t2.textContent = formatBytes(totalStorage);
      const t3 = document.getElementById('dbdiag-total-index'); if (t3) t3.textContent = '0 B';
      const t4 = document.getElementById('dbdiag-total-overhead'); if (t4) t4.textContent = formatBytes(totalOverhead);
    }

    async function loadAllDatabasesDiagnostics() {
      if (!activeConnection) return;
      const tbody = document.getElementById('dbdiag-databases-tbody');
      if (tbody) tbody.innerHTML = '<tr><td colspan="8" class="p-4 text-center text-slate-500">Inspecting database storage across session...</td></tr>';

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/databases`);
        const list = await res.json();
        if (!res.ok || !Array.isArray(list)) {
          if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-4 text-center text-red-400">Failed to load databases diagnostics</td></tr>`;
          return;
        }

        dbDiagAllDatabases = list;
        updateDbDiagOverviewCards();
        renderDbDiagDatabasesRows();
      } catch (err) {
        if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-4 text-center text-red-400">Error: ${escapeHtml(err.message)}</td></tr>`;
      }
    }

    function onToggleSystemDbs(checked) {
      dbDiagShowSystemDbs = checked;
      updateDbDiagOverviewCards();
      renderDbDiagDatabasesRows();
    }

    function onDbDiagDbSearchInput(val) {
      renderDbDiagDatabasesRows((val || '').toLowerCase().trim());
    }

    function renderDbDiagDatabasesRows(filterQuery = '') {
      const tbody = document.getElementById('dbdiag-databases-tbody');
      if (!tbody) return;

      const filtered = dbDiagAllDatabases.filter(d => {
        if (!dbDiagShowSystemDbs && d.is_system) return false;
        if (filterQuery && !d.name.toLowerCase().includes(filterQuery)) return false;
        return true;
      });

      if (filtered.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" class="p-4 text-center text-slate-500">No databases match filter.</td></tr>';
        return;
      }

      const activeDb = schemaCatalogCache?.database_name || activeConnection.database_name || '';

      tbody.innerHTML = filtered.map(d => {
        const isCurrent = d.name.toLowerCase() === activeDb.toLowerCase();
        const totalSize = d.total_size || (d.data_size + d.index_size) || 0;
        return `
          <tr class="hover:bg-slate-800/40 transition">
            <td class="p-2">
              <div class="flex items-center space-x-1.5">
                ${renderIcon('database', 'w-4 h-4 text-sky-400')}
                <span class="font-bold ${isCurrent ? 'text-app-accent' : 'text-white'}">${escapeHtml(d.name)}</span>
                ${isCurrent ? '<span class="text-[9px] bg-sky-950 text-sky-400 border border-sky-800 px-1 rounded font-mono">active</span>' : ''}
                ${d.is_system ? '<span class="text-[9px] bg-slate-800 text-slate-400 px-1 rounded font-mono">system</span>' : ''}
              </div>
            </td>
            <td class="p-2 text-right text-slate-300 font-mono">${(d.table_count || 0).toLocaleString()}</td>
            <td class="p-2 text-right font-bold text-sky-400 font-mono">${formatBytes(totalSize)}</td>
            <td class="p-2 text-right text-slate-300 font-mono">${formatBytes(d.data_size || 0)}</td>
            <td class="p-2 text-right text-indigo-400 font-mono">${formatBytes(d.index_size || 0)}</td>
            <td class="p-2 text-right font-mono ${d.overhead_size > 0 ? 'text-amber-400 font-bold' : 'text-slate-500'}">${formatBytes(d.overhead_size || 0)}</td>
            <td class="p-2 text-slate-400 text-xs">${escapeHtml(d.collation || d.charset || '-')}</td>
            <td class="p-2 text-center">
              <button type="button" data-name="${escapeHtml(d.name)}" onclick="inspectDatabaseTables(this.dataset.name)" class="bg-slate-800 hover:bg-sky-600 hover:text-white text-sky-400 px-2.5 py-1 rounded text-xs font-semibold transition flex items-center space-x-1 mx-auto">
                <span>Inspect</span>
                ${renderIcon('arrow-right', 'w-3 h-3 text-sky-400')}
              </button>
            </td>
          </tr>
        `;
      }).join('');
    }

    function inspectDatabaseTables(dbName) {
      dbDiagLevel = 'tables';
      dbDiagSelectedDbName = dbName;

      document.getElementById('dbdiag-level-databases')?.classList.add('hidden');
      document.getElementById('dbdiag-level-tables')?.classList.remove('hidden');
      document.getElementById('dbdiag-btn-back')?.classList.remove('hidden');
      document.getElementById('dbdiag-breadcrumb-sep')?.classList.remove('hidden');
      const bcDb = document.getElementById('dbdiag-breadcrumb-db');
      if (bcDb) {
        bcDb.classList.remove('hidden');
        bcDb.textContent = dbName;
      }
      const titleEl = document.getElementById('dbdiag-modal-title');
      if (titleEl) titleEl.textContent = 'Table Storage & Health Grid';
      const descEl = document.getElementById('dbdiag-header-desc');
      if (descEl) descEl.textContent = `Physical storage, row counts, collations & recommendations for "${dbName}"`;

      switchDbDiagTab('tables');
      loadDatabaseDiagnostics(dbName);
    }

    function switchDbDiagTab(tab) {
      const btnTables = document.getElementById('dbdiag-tab-tables-btn');
      const btnIndexes = document.getElementById('dbdiag-tab-indexes-btn');
      const viewTables = document.getElementById('dbdiag-view-tables');
      const viewIndexes = document.getElementById('dbdiag-view-indexes');
      const toolbar = document.getElementById('dbdiag-tables-toolbar');

      if (tab === 'tables') {
        btnTables.className = 'px-3 py-1 rounded text-xs font-semibold bg-app-surface text-sky-400 border border-sky-800/60';
        btnIndexes.className = 'px-3 py-1 rounded text-xs text-slate-400 hover:text-white';
        viewTables.classList.remove('hidden');
        viewIndexes.classList.add('hidden');
        if (toolbar) toolbar.classList.remove('hidden');
      } else {
        btnIndexes.className = 'px-3 py-1 rounded text-xs font-semibold bg-app-surface text-sky-400 border border-sky-800/60';
        btnTables.className = 'px-3 py-1 rounded text-xs text-slate-400 hover:text-white';
        viewIndexes.classList.remove('hidden');
        viewTables.classList.add('hidden');
        if (toolbar) toolbar.classList.add('hidden');
      }
    }

    async function loadDatabaseDiagnostics(targetDb = null) {
      if (!activeConnection) return;
      const currentDb = targetDb || dbDiagSelectedDbName || schemaCatalogCache?.database_name || activeConnection.database_name || '';
      dbDiagSelectedDbName = currentDb;
      const tbody = document.getElementById('dbdiag-tables-tbody');
      if (tbody) tbody.innerHTML = '<tr><td colspan="9" class="p-4 text-center text-slate-500">Inspecting table storage & health...</td></tr>';
      dbDiagSelected.clear();

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/tables?database=${encodeURIComponent(currentDb)}`);
        const tables = await res.json();
        if (!res.ok || !Array.isArray(tables)) {
          if (tbody) tbody.innerHTML = `<tr><td colspan="9" class="p-4 text-center text-red-400">Failed to load diagnostics</td></tr>`;
          return;
        }

        dbDiagTables = tables;
        let totalData = 0;
        let totalIndex = 0;
        let totalOverhead = 0;

        tables.forEach(t => {
          totalData += (t.data_size || 0);
          totalIndex += (t.index_size || 0);
          totalOverhead += (t.overhead_size || 0);
        });

        const lbl1 = document.getElementById('dbdiag-stat-label-1');
        const val1 = document.getElementById('dbdiag-stat-val-1');
        if (lbl1) lbl1.textContent = 'DB Tables';
        if (val1) val1.textContent = tables.length.toString();

        const lbl2 = document.getElementById('dbdiag-stat-label-2');
        const val2 = document.getElementById('dbdiag-stat-val-2');
        if (lbl2) lbl2.textContent = 'Data Size';
        if (val2) val2.textContent = formatBytes(totalData);

        const lbl3 = document.getElementById('dbdiag-stat-label-3');
        const val3 = document.getElementById('dbdiag-stat-val-3');
        if (lbl3) lbl3.textContent = 'Index Size';
        if (val3) val3.textContent = formatBytes(totalIndex);

        const lbl4 = document.getElementById('dbdiag-stat-label-4');
        const val4 = document.getElementById('dbdiag-stat-val-4');
        if (lbl4) lbl4.textContent = 'Fragmentation Overhead';
        if (val4) val4.textContent = formatBytes(totalOverhead);

        const t1 = document.getElementById('dbdiag-total-tables'); if (t1) t1.textContent = tables.length.toString();
        const t2 = document.getElementById('dbdiag-total-data'); if (t2) t2.textContent = formatBytes(totalData);
        const t3 = document.getElementById('dbdiag-total-index'); if (t3) t3.textContent = formatBytes(totalIndex);
        const t4 = document.getElementById('dbdiag-total-overhead'); if (t4) t4.textContent = formatBytes(totalOverhead);

        renderDbDiagTableRows(tables);
      } catch (err) {
        if (tbody) tbody.innerHTML = `<tr><td colspan="9" class="p-4 text-center text-red-400">Error: ${escapeHtml(err.message)}</td></tr>`;
      }
    }

    function renderDbDiagTableRows(tables) {
      const tbody = document.getElementById('dbdiag-tables-tbody');
      if (!tbody) return;
      if (!tables || tables.length === 0) {
        tbody.innerHTML = '<tr><td colspan="9" class="p-4 text-center text-slate-500">No tables match filter.</td></tr>';
        return;
      }

      tbody.innerHTML = tables.map(t => {
        const isChecked = dbDiagSelected.has(t.name);
        const totalSize = (t.data_size || 0) + (t.index_size || 0);

        const recommendations = [];
        const isView = (t.engine || '').toUpperCase() === 'VIEW';

        // 1. High Fragmentation
        if (t.overhead_size > 0 && (t.overhead_size > 50 * 1024 * 1024 || (totalSize > 0 && (t.overhead_size / totalSize) > 0.20))) {
          recommendations.push(`
            <button type="button" data-table="${escapeHtml(t.name)}" onclick="runDbMaintenanceBatch('OPTIMIZE', [this.dataset.table])" class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-500/20 text-amber-300 border border-amber-500/40 hover:bg-amber-500/30 transition cursor-pointer" title="Overhead: ${formatBytes(t.overhead_size)}. Click to run OPTIMIZE">
      <svg class="w-4 h-4 text-amber-400 inline shrink-0"><use href="#icon-alert-triangle"/></svg>
              <span>Fragmented</span>
            </button>
          `);
        }

        // 2. Missing Primary Key
        if (!t.has_primary_key && !isView) {
          recommendations.push(`
            <button type="button" data-table="${escapeHtml(t.name)}" onclick="openTableHealthFix('pk', this.dataset.table)" class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-rose-500/20 text-rose-300 border border-rose-500/40 hover:bg-rose-500/30 transition cursor-pointer" title="Missing primary key. Click to open ADD PRIMARY KEY fix in editor">
              ${renderIcon('shield-alert', 'w-3.5 h-3.5 text-rose-400 mr-1')}
              <span>No PK</span>
            </button>
          `);
        }

        // 3. Legacy Storage Engine
        if ((t.engine || '').toUpperCase() === 'MYISAM') {
          recommendations.push(`
            <button type="button" data-table="${escapeHtml(t.name)}" onclick="openTableHealthFix('engine', this.dataset.table)" class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-500/20 text-amber-300 border border-amber-500/40 hover:bg-amber-500/30 transition cursor-pointer" title="Non-transactional MyISAM. Click to convert to InnoDB">
      <svg class="w-4 h-4 text-amber-400 inline shrink-0"><use href="#icon-alert-triangle"/></svg>
              <span>MyISAM</span>
            </button>
          `);
        }

        // 4. Legacy Charset / Collation
        if (t.collation && (t.collation.toLowerCase().startsWith('latin1') || t.collation.toLowerCase().startsWith('utf8_') || t.collation.toLowerCase().startsWith('utf8mb3'))) {
          recommendations.push(`
            <button type="button" data-table="${escapeHtml(t.name)}" onclick="openTableHealthFix('charset', this.dataset.table)" class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-indigo-500/20 text-indigo-300 border border-indigo-500/40 hover:bg-indigo-500/30 transition cursor-pointer" title="Legacy collation ${escapeHtml(t.collation)}. Click to convert to utf8mb4">
              ${renderIcon('info', 'w-3.5 h-3.5 text-indigo-400 mr-1')}
              <span>${escapeHtml(t.collation.split('_')[0])}</span>
            </button>
          `);
        }

        const healthHtml = recommendations.length > 0
          ? `<div class="flex flex-wrap gap-1">${recommendations.join('')}</div>`
          : `<span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"><span>✓</span><span>Optimal</span></span>`;

        return `
          <tr class="hover:bg-slate-800/40 transition">
            <td class="p-2"><input type="checkbox" data-table="${escapeHtml(t.name)}" onchange="toggleSelectDbDiagTable(this.dataset.table, this.checked)" ${isChecked ? 'checked' : ''} class="accent-sky-500"></td>
            <td class="p-2 font-bold text-white">${escapeHtml(t.name)}</td>
            <td class="p-2 text-slate-400">${escapeHtml(t.engine || '-')}</td>
            <td class="p-2 text-right text-slate-300 font-mono">${Number(t.rows).toLocaleString()}</td>
            <td class="p-2 text-right text-sky-400 font-mono">${formatBytes(totalSize)}</td>
            <td class="p-2 text-right font-mono ${t.overhead_size > 0 ? 'text-amber-400 font-bold' : 'text-slate-500'}">${formatBytes(t.overhead_size)}</td>
            <td class="p-2 text-slate-400 text-xs">${escapeHtml(t.collation || '-')}</td>
            <td class="p-2">${healthHtml}</td>
            <td class="p-2 text-center">
              <button type="button" data-table="${escapeHtml(t.name)}" onclick="runDbMaintenanceBatch('OPTIMIZE', [this.dataset.table])" class="bg-slate-800 hover:bg-sky-600 hover:text-white text-slate-300 px-2 py-0.5 rounded text-[11px] transition">
                Optimize
              </button>
            </td>
          </tr>
        `;
      }).join('');
    }

    function openTableHealthFix(fixType, tableName) {
      const currentDb = dbDiagSelectedDbName || schemaCatalogCache?.database_name || activeConnection?.database_name || '';
      const safeTable = '`' + tableName.replace(/`/g, '``') + '`';
      let sql = '';
      let title = `${tableName}-fix.sql`;

      if (fixType === 'pk') {
        sql = `-- Table Health Advisor: Add Primary Key to ${safeTable}\n-- Recommendation: Replace 'id' with your desired existing or new primary key column\nALTER TABLE ${safeTable} ADD PRIMARY KEY (\`id\`);\n`;
      } else if (fixType === 'engine') {
        sql = `-- Table Health Advisor: Convert ${safeTable} from MyISAM to crash-safe ACID InnoDB\nALTER TABLE ${safeTable} ENGINE = InnoDB;\n`;
      } else if (fixType === 'charset') {
        sql = `-- Table Health Advisor: Upgrade ${safeTable} to modern full Unicode utf8mb4\nALTER TABLE ${safeTable} CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n`;
      }

      closeDatabaseDiagnosticsModal();

      const newId = 'tab-' + Date.now();
      tabs.push({
        id: newId,
        title: title,
        sql: sql,
        database: currentDb,
        isTableViewer: false
      });
      switchTab(newId);
    }

    function onDbDiagSearchInput(val) {
      const clearBtn = document.getElementById('dbdiag-search-clear-btn');
      if (clearBtn) {
        if (val && val.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
      filterDbDiagTables(val);
    }

    function clearDbDiagSearch() {
      const input = document.getElementById('dbdiag-search-input');
      const clearBtn = document.getElementById('dbdiag-search-clear-btn');
      if (input) {
        input.value = '';
        input.focus();
      }
      if (clearBtn) {
        clearBtn.classList.add('hidden');
      }
      filterDbDiagTables('');
    }

    function filterDbDiagTables(query) {
      const q = (query || '').toLowerCase().trim();
      const filtered = dbDiagTables.filter(t => t.name.toLowerCase().includes(q));
      renderDbDiagTableRows(filtered);
    }

    function toggleSelectAllDbDiagTables(checked) {
      if (checked) {
        dbDiagTables.forEach(t => dbDiagSelected.add(t.name));
      } else {
        dbDiagSelected.clear();
      }
      renderDbDiagTableRows(dbDiagTables);
    }

    function toggleSelectDbDiagTable(name, checked) {
      if (checked) dbDiagSelected.add(name);
      else dbDiagSelected.delete(name);
    }

    async function runDbMaintenanceBatch(action, specificTables = null) {
      if (!activeConnection) return;
      const tables = specificTables || Array.from(dbDiagSelected);
      if (!tables || tables.length === 0) {
        showNativeAlert('Please select at least one table to perform maintenance.', 'Select Table');
        return;
      }
      const currentDb = dbDiagSelectedDbName || schemaCatalogCache?.database_name || activeConnection.database_name || '';
      const statusEl = document.getElementById('dbdiag-status-msg');
      if (statusEl) statusEl.textContent = `Running ${action} on ${tables.length} table(s)...`;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/maintenance`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ database: currentDb, tables, action })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(data.error || 'Maintenance action failed', 'Error');
          return;
        }
        if (statusEl) statusEl.textContent = `✓ ${action} completed successfully on ${tables.length} table(s).`;
        setTimeout(() => loadDatabaseDiagnostics(currentDb), 800);
      } catch (err) {
        showNativeAlert('Maintenance failed: ' + err.message, 'Network Error');
      }
    }

    async function scanDatabaseRedundantIndexes() {
      if (!activeConnection) return;
      const currentDb = dbDiagSelectedDbName || schemaCatalogCache?.database_name || activeConnection.database_name || '';
      const container = document.getElementById('dbdiag-indexes-container');
      container.innerHTML = '<div class="text-slate-500 italic p-4 text-center">Scanning database index definitions...</div>';

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/redundant-indexes?database=${encodeURIComponent(currentDb)}`);
        const list = await res.json();
        if (!res.ok || !Array.isArray(list) || list.length === 0) {
          container.innerHTML = '<div class="p-6 text-center text-emerald-400 font-semibold">✓ No redundant or duplicate indexes found across the entire database!</div>';
          return;
        }

        container.innerHTML = list.map(item => `
          <div class="p-3 bg-slate-950 border border-slate-800 rounded flex items-center justify-between">
            <div class="space-y-1">
              <div class="flex items-center space-x-2">
                <span class="font-bold text-white text-xs">${escapeHtml(item.table)}</span>
                <span class="text-slate-500">::</span>
                <span class="font-bold text-amber-300">${escapeHtml(item.redundant_index)}</span>
                <span class="text-[10px] bg-slate-800 text-slate-300 px-1.5 py-0.5 rounded">${escapeHtml((item.redundant_cols || []).join(', '))}</span>
                <span class="text-slate-400 text-xs">shadowed by</span>
                <span class="font-bold text-sky-400">${escapeHtml(item.shadowed_by)}</span>
              </div>
              <div class="text-[11px] text-slate-400">${escapeHtml(item.reason)}</div>
            </div>
            <button data-drop-sql="${escapeHtml(item.drop_sql)}" data-table="${escapeHtml(item.table)}" onclick="promptDropRedundantIndex(this.dataset.dropSql, this.dataset.table)" class="bg-rose-900 hover:bg-rose-800 text-white px-3 py-1 rounded text-xs transition">
              Drop Index
            </button>
          </div>
        `).join('');
      } catch (err) {
        container.innerHTML = `<div class="p-4 text-center text-red-400">Failed to scan: ${escapeHtml(err.message)}</div>`;
      }
    }

    // --- Query Profiler Handlers ---
    function toggleQueryProfiler() {
      enableQueryProfiling = !enableQueryProfiling;
      const textEl = document.getElementById('profile-toggle-text');
      const btnEl = document.getElementById('btn-toggle-profile');
      if (textEl && btnEl) {
        if (enableQueryProfiling) {
          textEl.textContent = 'Profile: ON';
          btnEl.className = 'bg-amber-950/80 hover:bg-amber-900 text-amber-300 font-semibold px-2.5 py-1 rounded border border-amber-600 transition flex items-center space-x-1';
        } else {
          textEl.textContent = 'Profile: OFF';
          btnEl.className = 'bg-app-card hover:bg-slate-700 text-slate-300 px-2.5 py-1 rounded border border-app-border transition flex items-center space-x-1';
        }
      }
    }

    function toggleProfileView() {
      const panel = document.getElementById('query-profiler-panel');
      const gridTable = document.getElementById('virtual-grid-wrapper');
      const paginationBar = document.getElementById('grid-pagination-bar');
      if (!panel) return;
      const isHidden = panel.classList.contains('hidden');
      if (isHidden) {
        panel.classList.remove('hidden');
        if (gridTable) gridTable.classList.add('hidden');
        if (paginationBar) paginationBar.classList.add('hidden');
        if (lastQueryProfile) renderQueryProfile(lastQueryProfile);
      } else {
        closeProfileView();
      }
    }

    function closeProfileView() {
      const panel = document.getElementById('query-profiler-panel');
      const gridTable = document.getElementById('virtual-grid-wrapper');
      const paginationBar = document.getElementById('grid-pagination-bar');
      if (panel) panel.classList.add('hidden');
      if (gridTable) gridTable.classList.remove('hidden');
      if (paginationBar) paginationBar.classList.remove('hidden');
    }

    function renderQueryProfile(profile) {
      const totalBadge = document.getElementById('profiler-total-badge');
      const notice = document.getElementById('profiler-sqlite-notice');
      const tbody = document.getElementById('profiler-stages-tbody');
      if (!profile || !tbody) return;

      const isSQLite = activeConnection && activeConnection.driver === 'sqlite';
      if (notice) notice.classList.toggle('hidden', !isSQLite);

      if (totalBadge) totalBadge.textContent = `${(profile.duration_ms || 0).toFixed(2)} ms total`;

      const stages = Array.isArray(profile.stages) ? profile.stages : [];
      if (stages.length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" class="p-4 text-center text-slate-500">No profile stage data available.</td></tr>';
        return;
      }

      tbody.innerHTML = stages.map((s, idx) => {
        const pct = Math.max(0, Math.min(100, s.percentage || 0));
        let barColor = 'bg-sky-500';
        if (pct > 50) barColor = 'bg-red-500';
        else if (pct > 20) barColor = 'bg-amber-500';

        let durStr = `${s.duration_us} µs`;
        if (s.duration_us >= 1000) {
          durStr = `${(s.duration_us / 1000).toFixed(2)} ms`;
        }

        return `
          <tr class="hover:bg-slate-800/40 transition border-b border-slate-800/40">
            <td class="p-2 text-center text-slate-500 font-mono">${idx + 1}</td>
            <td class="p-2 font-semibold text-white">${escapeHtml(s.stage)}</td>
            <td class="p-2 text-right text-slate-300 font-mono">${durStr}</td>
            <td class="p-2">
              <div class="w-full bg-slate-800 rounded-full h-2 overflow-hidden">
                <div class="${barColor} h-2 rounded-full transition-all duration-300" style="width: ${pct}%"></div>
              </div>
            </td>
            <td class="p-2 text-right text-slate-400 font-mono text-[11px]">${pct.toFixed(1)}%</td>
          </tr>
        `;
      }).join('');
    }

    // --- Foreign Key Hyperlink & Data Peeking Handlers ---
    async function peekForeignKey(event, table, col, refTable, refCol, val) {
      if (!activeConnection || !refTable || !refCol) return;
      const activeTab = tabs.find(t => t.id === activeTabId);
      const currentDb = (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || '';

      const popover = document.getElementById('fk-peek-popover');
      const title = document.getElementById('fk-peek-title');
      const content = document.getElementById('fk-peek-content');
      const btnJump = document.getElementById('fk-peek-btn-jump');
      if (!popover || !content) return;

      title.textContent = `${refTable} (${refCol} = ${val})`;
      content.innerHTML = '<div class="text-slate-400 p-2">Fetching referenced record...</div>';

      const posX = Math.min(window.innerWidth - 420, Math.max(10, event.clientX - 100));
      const posY = Math.min(window.innerHeight - 300, Math.max(10, event.clientY + 15));
      popover.style.left = posX + 'px';
      popover.style.top = posY + 'px';
      popover.classList.remove('hidden');

      if (btnJump) {
        btnJump.onclick = () => {
          closeFKPeekPopover();
          openTableViewer(refTable);
        };
      }

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/peek?database=${encodeURIComponent(currentDb)}&table=${encodeURIComponent(refTable)}&column=${encodeURIComponent(refCol)}&value=${encodeURIComponent(val)}`);
        const data = await res.json();
        if (!res.ok || !data.row) {
          content.innerHTML = '<div class="text-amber-400 p-2">No matching record found in parent table.</div>';
          return;
        }

        const cols = data.columns || Object.keys(data.row);
        content.innerHTML = `
          <table class="w-full text-left border-collapse text-[11px]">
            <tbody class="divide-y divide-slate-800">
              ${cols.map(c => `
                <tr class="hover:bg-slate-800/40">
                  <td class="p-1 text-slate-400 font-semibold w-1/3">${escapeHtml(c)}</td>
                  <td class="p-1 text-white select-text truncate max-w-xs font-mono">${data.row[c] === null ? '<span class="text-slate-600 italic">NULL</span>' : escapeHtml(String(data.row[c]))}</td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        `;
      } catch (err) {
        content.innerHTML = `<div class="text-red-400 p-2">Error: ${escapeHtml(err.message)}</div>`;
      }
    }

    function closeFKPeekPopover() {
      const popover = document.getElementById('fk-peek-popover');
      if (popover) popover.classList.add('hidden');
    }

    document.addEventListener('click', (e) => {
      const popover = document.getElementById('fk-peek-popover');
      if (popover && !popover.classList.contains('hidden') && !popover.contains(e.target)) {
        closeFKPeekPopover();
      }
      const plMenu = document.getElementById('pl-context-menu');
      if (plMenu && !plMenu.classList.contains('hidden')) {
        if (!plMenu.contains(e.target) && !e.target.closest('[data-pl-menu-btn]')) {
          closeProcessContextMenu();
        }
      }
    });

    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        closeAllContextMenus();
        closeFKPeekPopover();
        return;
      }

      // If active cell is selected (and user is not currently inside an input/textarea)
      if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable)) {
        return;
      }

      if (activeGridSelectedTd && activeGridSelectedRow !== null && activeGridSelectedCol !== null) {
        if (e.key === 'ArrowUp' || e.key === 'ArrowDown' || e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
          e.preventDefault();
          navigateGridCellByArrow(e.key);
          return;
        }
        if (e.ctrlKey && e.shiftKey && (e.key === 'N' || e.key === 'n')) {
          e.preventDefault();
          insertGridCellValue(null);
        } else if (e.ctrlKey && e.shiftKey && (e.key === 'E' || e.key === 'e')) {
          e.preventDefault();
          insertGridCellValue('');
        } else if (e.ctrlKey && e.shiftKey && (e.key === 'T' || e.key === 't')) {
          e.preventDefault();
          insertGridCellValue('NOW');
        } else if (e.ctrlKey && e.shiftKey && (e.key === 'D' || e.key === 'd')) {
          e.preventDefault();
          insertGridCellValue('DATE');
        }
      }
    });

    // --- Visual Privilege Matrix Handlers ---
    const PRIVILEGE_DEFINITIONS = {
      data: ['SELECT', 'INSERT', 'UPDATE', 'DELETE', 'FILE'],
      structure: ['CREATE', 'ALTER', 'INDEX', 'DROP', 'CREATE VIEW', 'SHOW VIEW', 'CREATE ROUTINE', 'ALTER ROUTINE', 'EXECUTE', 'EVENT', 'TRIGGER'],
      admin: ['GRANT OPTION', 'SUPER', 'PROCESS', 'RELOAD', 'SHUTDOWN', 'SHOW DATABASES', 'LOCK TABLES', 'REPLICATION CLIENT', 'REPLICATION SLAVE']
    };

    function initPrivilegeCheckboxes() {
      ['data', 'structure', 'admin'].forEach(grp => {
        const container = document.getElementById(`priv-group-${grp}`);
        if (!container) return;
        container.innerHTML = PRIVILEGE_DEFINITIONS[grp].map(p => `
          <label class="flex items-center space-x-2 text-xs text-slate-300 hover:text-white cursor-pointer select-none">
            <input type="checkbox" data-priv="${p}" class="accent-sky-500 priv-checkbox">
            <span>${p}</span>
          </label>
        `).join('');
      });
    }

    function openPrivilegeMatrixModal() {
      if (!activeConnection || (activeConnection.driver !== 'mysql' && activeConnection.driver !== 'mariadb' && activeConnection.driver !== 'postgres' && activeConnection.driver !== 'postgresql' && activeConnection.driver !== 'pg')) {
        showNativeAlert('User and role management is supported for MySQL, MariaDB, and PostgreSQL connections.', 'Privilege Matrix');
        return;
      }
      initPrivilegeCheckboxes();
      switchPrivTab('account');
      loadPrivilegeUsers();
      const dlg = document.getElementById('privilege-matrix-dialog');
      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    function closePrivilegeMatrixModal() {
      const dlg = document.getElementById('privilege-matrix-dialog');
      if (dlg) {
        if (typeof dlg.close === 'function') dlg.close();
        else dlg.removeAttribute('open');
      }
    }

    function switchPrivTab(tab) {
      const btnAcc = document.getElementById('priv-tab-account-btn');
      const btnGrants = document.getElementById('priv-tab-grants-btn');
      const viewAcc = document.getElementById('priv-view-account');
      const viewGrants = document.getElementById('priv-view-grants');

      if (tab === 'account') {
        btnAcc.className = 'px-3 py-1 rounded font-semibold bg-app-surface text-sky-400 border border-sky-800/60';
        btnGrants.className = 'px-3 py-1 rounded text-slate-400 hover:text-white';
        viewAcc.classList.remove('hidden');
        viewGrants.classList.add('hidden');
      } else {
        btnGrants.className = 'px-3 py-1 rounded font-semibold bg-app-surface text-sky-400 border border-sky-800/60';
        btnAcc.className = 'px-3 py-1 rounded text-slate-400 hover:text-white';
        viewGrants.classList.remove('hidden');
        viewAcc.classList.add('hidden');
      }
    }

    async function loadPrivilegeUsers() {
      const listEl = document.getElementById('priv-user-list');
      listEl.innerHTML = '<div class="text-slate-500 italic p-2">Loading users...</div>';

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/users`);
        const users = await res.json();
        if (!res.ok || !Array.isArray(users)) {
          listEl.innerHTML = '<div class="text-red-400 p-2">Failed to load users</div>';
          return;
        }

        allPrivUsers = users;
        if (users.length === 0) {
          listEl.innerHTML = '<div class="text-slate-500 p-2">No user accounts found.</div>';
          return;
        }

        const isPG = activeConnection && (activeConnection.driver === 'postgres' || activeConnection.driver === 'postgresql' || activeConnection.driver === 'pg');
        listEl.innerHTML = users.map(u => {
          const displayHost = isPG ? 'N/A (pg_hba.conf)' : u.host;
          return `
          <div data-user="${escapeHtml(u.user)}" data-host="${escapeHtml(u.host)}" onclick="selectPrivilegeUser(this.dataset.user, this.dataset.host)" class="p-2 rounded bg-slate-900/60 hover:bg-slate-800 cursor-pointer flex items-center justify-between border border-transparent priv-user-item ${activePrivUser && activePrivUser.user === u.user && activePrivUser.host === u.host ? 'border-sky-500 bg-slate-800 text-white' : 'text-slate-300'}">
            <div class="flex items-center space-x-1.5 truncate">
              ${renderIcon(isPG ? 'shield' : 'user', 'w-3.5 h-3.5 text-slate-400')}
              <span class="font-bold truncate">${escapeHtml(u.user)}</span>
            </div>
            <span class="text-[10px] bg-slate-950 text-slate-400 px-1 py-0.2 rounded font-mono">${escapeHtml(displayHost)}</span>
          </div>
        `}).join('');

        if (users.length > 0 && !activePrivUser) {
          selectPrivilegeUser(users[0].user, users[0].host);
        }
      } catch (err) {
        listEl.innerHTML = `<div class="text-red-400 p-2">Error: ${escapeHtml(err.message)}</div>`;
      }
    }

    async function selectPrivilegeUser(user, host) {
      const isPG = activeConnection && (activeConnection.driver === 'postgres' || activeConnection.driver === 'postgresql' || activeConnection.driver === 'pg');
      activePrivUser = { user, host, isNew: false };
      document.querySelectorAll('.priv-user-item').forEach(el => {
        el.classList.remove('border-sky-500', 'bg-slate-800', 'text-white');
      });
      document.getElementById('priv-input-user').value = user;
      const hostInput = document.getElementById('priv-input-host');
      if (hostInput) {
        hostInput.value = isPG ? 'N/A (pg_hba.conf)' : host;
        hostInput.disabled = isPG;
      }
      document.getElementById('priv-input-password').value = '';

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/users/grants?user=${encodeURIComponent(user)}&host=${encodeURIComponent(host)}`);
        const grants = await res.json();
        parseAndApplyGrantsToCheckboxes(grants || []);
      } catch (err) {
        console.error('Failed to load user grants:', err);
      }
    }

    function startCreateNewUser() {
      const isPG = activeConnection && (activeConnection.driver === 'postgres' || activeConnection.driver === 'postgresql' || activeConnection.driver === 'pg');
      activePrivUser = { user: '', host: isPG ? '' : '%', isNew: true };
      document.getElementById('priv-input-user').value = '';
      const hostInput = document.getElementById('priv-input-host');
      if (hostInput) {
        hostInput.value = isPG ? 'N/A (pg_hba.conf)' : '%';
        hostInput.disabled = isPG;
      }
      document.getElementById('priv-input-password').value = '';
      switchPrivTab('account');
      applyPrivilegePreset('none');
      setTimeout(() => document.getElementById('priv-input-user').focus(), 100);
    }

    function parseAndApplyGrantsToCheckboxes(grantLines) {
      applyPrivilegePreset('none');
      const allText = grantLines.join(' ').toUpperCase();
      const hasAll = allText.includes('ALL PRIVILEGES');
      const hasGrantOption = allText.includes('WITH GRANT OPTION');

      document.getElementById('priv-with-grant').checked = hasGrantOption;

      document.querySelectorAll('.priv-checkbox').forEach(cb => {
        const priv = cb.getAttribute('data-priv').toUpperCase();
        if (hasAll || allText.includes(priv)) {
          cb.checked = true;
        }
      });
    }

    function applyPrivilegePreset(preset) {
      const checkboxes = document.querySelectorAll('.priv-checkbox');
      checkboxes.forEach(cb => cb.checked = false);

      if (preset === 'all') {
        checkboxes.forEach(cb => cb.checked = true);
        document.getElementById('priv-with-grant').checked = true;
      } else if (preset === 'readonly') {
        document.querySelectorAll('input[data-priv="SELECT"], input[data-priv="SHOW VIEW"]').forEach(cb => cb.checked = true);
      } else if (preset === 'readwrite') {
        document.querySelectorAll('input[data-priv="SELECT"], input[data-priv="INSERT"], input[data-priv="UPDATE"], input[data-priv="DELETE"], input[data-priv="SHOW VIEW"]').forEach(cb => cb.checked = true);
      }
    }

    function onPrivScopeChanged() {
      // Scope toggle between GLOBAL (*.*) and DATABASE (Active Database); applied during applyUserGrantsFromMatrix
    }

    async function savePrivilegeAccount() {
      const user = document.getElementById('priv-input-user').value.trim();
      const host = document.getElementById('priv-input-host').value.trim();
      const password = document.getElementById('priv-input-password').value;
      if (!user || !host) {
        showNativeAlert('User name and host origin are required.', 'Validation Error');
        return;
      }

      const isNew = activePrivUser?.isNew || false;
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/save-user`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ user, host, password, is_new: isNew })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(data.error || 'Failed to save account', 'Account Error');
          return;
        }
        showNativeAlert(`Account '${user}'@'${host}' saved successfully.`, 'Account Saved');
        activePrivUser = { user, host, isNew: false };
        loadPrivilegeUsers();
      } catch (err) {
        showNativeAlert('Save user failed: ' + err.message, 'Network Error');
      }
    }

    async function deletePrivilegeUser() {
      if (!activePrivUser || !activePrivUser.user) {
        showNativeAlert('Please select a user to delete.', 'Select User');
        return;
      }
      const ok = await showNativeConfirm(`Are you sure you want to permanently drop user account '${activePrivUser.user}'@'${activePrivUser.host}'?`, 'Drop User Confirmation');
      if (!ok) return;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/delete-user`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ user: activePrivUser.user, host: activePrivUser.host })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(data.error || 'Failed to drop user', 'Drop User Error');
          return;
        }
        showNativeAlert(`User '${activePrivUser.user}'@'${activePrivUser.host}' dropped.`, 'User Removed');
        activePrivUser = null;
        loadPrivilegeUsers();
      } catch (err) {
        showNativeAlert('Drop user failed: ' + err.message, 'Network Error');
      }
    }

    async function applyUserGrantsFromMatrix() {
      if (!activePrivUser || !activePrivUser.user) {
        showNativeAlert('Please select or save a user first.', 'Select User');
        return;
      }

      const selectedPrivs = [];
      document.querySelectorAll('.priv-checkbox:checked').forEach(cb => {
        selectedPrivs.push(cb.getAttribute('data-priv'));
      });

      const scope = document.getElementById('priv-scope-select').value;
      const withGrant = document.getElementById('priv-with-grant').checked;
      const currentDb = schemaCatalogCache?.database_name || activeConnection.database_name || '';

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/apply-grants`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            user: activePrivUser.user,
            host: activePrivUser.host,
            scope: scope,
            database: scope === 'DATABASE' ? currentDb : '',
            privileges: selectedPrivs,
            with_grant: withGrant
          })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(data.error || 'Failed to apply grants', 'Grant Error');
          return;
        }
        showNativeAlert(`Privileges granted to '${activePrivUser.user}'@'${activePrivUser.host}'.`, 'Grants Applied');
      } catch (err) {
        showNativeAlert('Apply grants failed: ' + err.message, 'Network Error');
      }
    }



