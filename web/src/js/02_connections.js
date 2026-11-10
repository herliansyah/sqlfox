// --- Connections & Session Management ---
    let sessionFilterQuery = '';
    let multiDBCatalogCache = {};
    const sessionCatalogStore = {}; // Map of connId -> { [dbName]: schemaOrCatalog }
    const sessionCatalogTimestamps = {}; // Map of connId -> { [dbName]: timestamp }

    async function loadConnections() {
      try {
        const res = await apiFetch('/api/connections');
        connections = await res.json();
        renderConnectionsTree();

        // Auto-restore active connection: check per-tab sessionStorage first, fallback to global localStorage
        let activeConnId = null;
        try {
          activeConnId = sessionStorage.getItem('sqlfox_active_conn_id') || localStorage.getItem('sqlfox_last_conn_id');
        } catch {
          activeConnId = null;
        }
        if (activeConnId && connections.some(c => c.id === activeConnId)) {
          await selectConnection(activeConnId);
        } else if (connections.length === 1) {
          await selectConnection(connections[0].id);
        }
      } catch (err) {
        console.error('Failed to load connections:', err);
      }
    }

    let sessionDropdownSelectedIndex = -1;
    let currentFilteredConnections = [];

    function toggleSessionDropdown(forceState) {
      const menu = document.getElementById('session-dropdown-menu');
      if (!menu) return;
      const shouldOpen = typeof forceState === 'boolean' ? forceState : menu.classList.contains('hidden');
      if (isConnectionSwitching && shouldOpen) return; // ponytail: prevent opening dropdown during in-flight connection switch
      if (shouldOpen) {
        toggleDatabaseDropdown(false);
        menu.classList.remove('hidden');
        sessionDropdownSelectedIndex = -1;
        const input = document.getElementById('session-search-input');
        const clearBtn = document.getElementById('session-search-clear-btn');
        if (clearBtn) clearBtn.classList.add('hidden');
        if (input) {
          input.value = '';
          sessionFilterQuery = '';
          input.focus();
        }
        renderConnectionsTree();
      } else {
        menu.classList.add('hidden');
      }
    }

    function onSessionSearchInput(val) {
      sessionDropdownSelectedIndex = -1;
      const clearBtn = document.getElementById('session-search-clear-btn');
      if (clearBtn) {
        if (val && val.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
      filterSessionList(val);
    }

    function onSessionSearchKeyDown(e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        toggleSessionDropdown(false);
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        if (currentFilteredConnections.length > 0) {
          sessionDropdownSelectedIndex = (sessionDropdownSelectedIndex + 1) % currentFilteredConnections.length;
          renderConnectionsTree();
        }
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        if (currentFilteredConnections.length > 0) {
          sessionDropdownSelectedIndex = sessionDropdownSelectedIndex <= 0
            ? currentFilteredConnections.length - 1
            : sessionDropdownSelectedIndex - 1;
          renderConnectionsTree();
        }
      } else if (e.key === 'Enter') {
        e.preventDefault();
        const targetIdx = sessionDropdownSelectedIndex >= 0 ? sessionDropdownSelectedIndex : 0;
        if (currentFilteredConnections.length > 0 && targetIdx < currentFilteredConnections.length) {
          const selectedConn = currentFilteredConnections[targetIdx];
          selectConnection(selectedConn.id);
          toggleSessionDropdown(false);
        }
      }
    }

    function clearSessionSearch() {
      sessionDropdownSelectedIndex = -1;
      const input = document.getElementById('session-search-input');
      const clearBtn = document.getElementById('session-search-clear-btn');
      if (input) {
        input.value = '';
        input.focus();
      }
      if (clearBtn) {
        clearBtn.classList.add('hidden');
      }
      filterSessionList('');
    }

    function filterSessionList(query) {
      sessionFilterQuery = (query || '').toLowerCase().trim();
      renderConnectionsTree();
    }

    // Dismiss session dropdown when clicking outside
    document.addEventListener('click', (e) => {
      const switcherBtn = document.getElementById('session-switcher-btn');
      const menu = document.getElementById('session-dropdown-menu');
      if (menu && !menu.classList.contains('hidden')) {
        if (switcherBtn && !switcherBtn.contains(e.target) && !menu.contains(e.target)) {
          menu.classList.add('hidden');
        }
      }
    });

    function renderHeaderConnectionBadge(conn, status = '') {
      const badgeEl = document.getElementById('active-conn-badge');
      if (!badgeEl) return;
      if (!conn) {
        badgeEl.className = "flex items-center space-x-2 text-xs";
        badgeEl.onclick = null;
        badgeEl.title = "";
        badgeEl.innerHTML = '<span class="text-slate-400">No active connection</span>';
        return;
      }
      const env = conn.environment || 'Development';
      const isProd = env === 'Production';
      const isStage = env === 'Staging';
      const pillClass = isProd ? 'bg-red-500/20 text-red-400 border border-red-500/40' :
                        (isStage ? 'bg-amber-500/20 text-amber-400 border border-amber-500/40' :
                                   'bg-emerald-500/20 text-emerald-400 border border-emerald-500/40');
      const envShort = isProd ? 'PROD' : (isStage ? 'STAGE' : 'DEV');
      const dotClass = isProd ? 'bg-red-400' : (isStage ? 'bg-amber-400' : 'bg-emerald-400');
      const pillLabel = `<span class="w-1.5 h-1.5 rounded-full ${dotClass} inline-block mr-1.5"></span>${envShort}`;

      const statusHtml = status ? ` <span class="text-slate-400 text-xs font-normal">(${escapeHtml(status)})</span>` : '';
      badgeEl.className = "flex items-center space-x-2 text-xs group cursor-pointer";
      badgeEl.title = `Active Connection: ${conn.name} (${env}) - Click or hover to edit`;
      badgeEl.onclick = () => openEditConnModal(conn.id);
      badgeEl.innerHTML = `
        <span class="px-2 py-0.5 rounded font-bold font-mono text-[11px] ${pillClass}">${pillLabel}</span>
        <span class="font-semibold text-white group-hover:text-sky-300 transition">${escapeHtml(conn.name)}${statusHtml}</span>
        <button onclick="event.stopPropagation(); openEditConnModal('${conn.id}')" class="opacity-0 group-hover:opacity-100 hover:text-white px-1 text-xs text-slate-400 transition" title="Edit Connection: ${escapeHtml(conn.name)}">${renderIcon('edit', 'w-3 h-3 text-slate-400 group-hover:text-white')}</button>
      `;
    }

    function renderConnectionsTree() {
      const el = document.getElementById('connections-tree');
      const switcherName = document.getElementById('session-switcher-name');
      const switcherEnvBadge = document.getElementById('session-switcher-env-badge');

      if (activeConnection) {
        if (switcherName) switcherName.textContent = activeConnection.name;
        if (switcherEnvBadge) switcherEnvBadge.classList.add('hidden'); // Free sidebar button from redundant badge
      } else {
        if (switcherName) switcherName.textContent = 'Select Connection';
        if (switcherEnvBadge) switcherEnvBadge.classList.add('hidden');
      }

      if (!el) return;

      if (!connections || connections.length === 0) {
        currentFilteredConnections = [];
        sessionDropdownSelectedIndex = -1;
        el.innerHTML = `<div class="text-slate-500 text-center py-4">No saved connections.<br><button onclick="openNewConnModal(); toggleSessionDropdown(false);" class="text-app-accent hover:underline mt-1">+ Create one</button></div>`;
        return;
      }

      let filteredConns = connections;
      if (sessionFilterQuery) {
        filteredConns = connections.filter(c =>
          c.name.toLowerCase().includes(sessionFilterQuery) ||
          (c.driver || '').toLowerCase().includes(sessionFilterQuery) ||
          (c.environment || '').toLowerCase().includes(sessionFilterQuery)
        );
      }

      if (filteredConns.length === 0) {
        currentFilteredConnections = [];
        sessionDropdownSelectedIndex = -1;
        el.innerHTML = `<div class="text-slate-500 text-center py-3 text-xs italic">No matching connections</div>`;
        return;
      }

      const groups = { Production: [], Staging: [], Development: [] };
      filteredConns.forEach(c => {
        const env = c.environment || 'Development';
        if (!groups[env]) groups[env] = [];
        groups[env].push(c);
      });

      currentFilteredConnections = [];
      for (const [env, list] of Object.entries(groups)) {
        list.forEach(c => currentFilteredConnections.push(c));
      }

      if (sessionDropdownSelectedIndex >= currentFilteredConnections.length) {
        sessionDropdownSelectedIndex = currentFilteredConnections.length > 0 ? currentFilteredConnections.length - 1 : -1;
      }

      let html = '';
      for (const [env, list] of Object.entries(groups)) {
        if (list.length === 0) continue;
        const envColor = env === 'Production' ? 'text-app-danger' : (env === 'Staging' ? 'text-app-warning' : 'text-app-safe');
        html += `<div class="font-bold ${envColor} uppercase text-[10px] mt-2 mb-1 px-1 flex items-center gap-1.5"><span class="w-1.5 h-1.5 rounded-full bg-current inline-block"></span><span>${env}</span></div>`;
        list.forEach(c => {
          const itemIdx = currentFilteredConnections.indexOf(c);
          const isAct = activeConnection && activeConnection.id === c.id;
          const isHighlighted = itemIdx === sessionDropdownSelectedIndex;
          let itemClasses = 'group flex items-center justify-between px-2 py-1.5 rounded cursor-pointer transition ';
          if (isHighlighted) {
            itemClasses += 'bg-sky-900/80 text-white font-semibold ring-1 ring-app-accent';
          } else if (isAct) {
            itemClasses += 'bg-sky-950/60 text-app-accent font-semibold border border-sky-800';
          } else {
            itemClasses += 'hover:bg-slate-800 text-slate-300';
          }
          html += `
            <div id="session-item-${itemIdx}" onclick="selectConnection('${c.id}'); toggleSessionDropdown(false);" class="${itemClasses}">
              <div class="flex items-center space-x-1.5 truncate">
                ${renderIcon('database', 'w-3.5 h-3.5 text-sky-400 shrink-0')}
                <span class="truncate">${escapeHtml(c.name)}</span>
              </div>
              <div class="flex items-center space-x-1">
                <span class="text-[10px] text-slate-500 font-mono group-hover:hidden">${escapeHtml(c.driver)}</span>
                <button onclick="event.stopPropagation(); openEditConnModal('${c.id}'); toggleSessionDropdown(false);" class="opacity-0 group-hover:opacity-100 hover:text-white px-1 text-xs transition" title="Edit Connection">${renderIcon('edit', 'w-3 h-3')}</button>
                <button onclick="event.stopPropagation(); deleteConnection('${c.id}', event)" class="opacity-0 group-hover:opacity-100 hover:text-app-danger px-1 text-xs transition" title="Delete Connection">${renderIcon('trash-2', 'w-3 h-3 text-rose-400')}</button>
              </div>
            </div>
          `;
        });
      }
      el.innerHTML = html;

      if (sessionDropdownSelectedIndex >= 0) {
        const activeItem = document.getElementById(`session-item-${sessionDropdownSelectedIndex}`);
        if (activeItem) {
          activeItem.scrollIntoView({ block: 'nearest' });
        }
      }
    }

    let isConnectionSwitching = false;
    async function selectConnection(connId) {
      if (isConnectionSwitching) return;
      const conn = connections.find(c => c.id === connId);
      if (!conn) return;
      if (activeConnection && activeConnection.id !== connId && isQueryExecuting) {
        await cancelRunningQuery();
      }
      isConnectionSwitching = true;
      const prevConnection = activeConnection;
      const sessionBtn = document.getElementById('session-switcher-btn');
      const sessionIcon = document.getElementById('session-switcher-icon');
      const prevSessionIconHTML = sessionIcon ? sessionIcon.innerHTML : '';
      if (sessionBtn) {
        sessionBtn.classList.add('cursor-wait', 'opacity-75', 'pointer-events-none');
      }
      if (sessionIcon) {
        sessionIcon.innerHTML = renderIcon('loader', 'w-3.5 h-3.5 animate-spin text-sky-400 shrink-0');
      }
      // ponytail: adaptive slow-network timer for connection handshake
      let slowConnTimer = setTimeout(() => {
        if (isConnectionSwitching) {
          showToast(`Server connection to '${conn.name}' is slow, still connecting...`, 'warning');
        }
      }, 3000);
      try {
        if (activeConnection && activeConnection.id !== connId) {
          saveTabsState();
        }
        activeConnection = conn;
        if (typeof cmEditor !== 'undefined' && cmEditor && typeof cmEditor.setDialect === 'function') {
          cmEditor.setDialect(conn.driver);
        }
        updateTransactionStatusUI(false);
        if (!sessionCatalogStore[conn.id]) {
          sessionCatalogStore[conn.id] = {};
          sessionCatalogTimestamps[conn.id] = {};
        }
        multiDBCatalogCache = sessionCatalogStore[conn.id];
        renderConnectionsTree();

        // Update badge to indicate active session cleanly without intrusive status text
        renderHeaderConnectionBadge(conn);
        document.getElementById('schema-tree').innerHTML = `<div class="text-slate-500 text-center py-4 flex items-center justify-center space-x-2">${renderIcon('loader', 'w-4 h-4 animate-spin')}<span>Connecting to database...</span></div>`;

        // Connect session
        const res = await apiFetch(`/api/session/connect/${conn.id}`, { method: 'POST' });
        const data = await res.json();
        if (!res.ok) {
          // Safe Reversion: restore previous connection state if handshake failed
          activeConnection = prevConnection;
          if (typeof cmEditor !== 'undefined' && cmEditor && typeof cmEditor.setDialect === 'function' && prevConnection) {
            cmEditor.setDialect(prevConnection.driver);
          }
          renderHeaderConnectionBadge(prevConnection);
          renderConnectionsTree();
          document.getElementById('schema-tree').innerHTML = `<div class="text-app-danger p-2 text-xs">Connection Error:<br><span class="font-mono text-[11px]">${escapeHtml(data.error || 'Connection failed')}</span></div>`;
          showNativeAlert('Failed to connect: ' + (data.error || res.statusText), 'Connection Error');
          return;
        }
        activeSession = data;
        try {
          sessionStorage.setItem('sqlfox_active_conn_id', conn.id);
          localStorage.setItem('sqlfox_last_conn_id', conn.id);
        } catch {}
        renderHeaderConnectionBadge(conn);
        const btnPL = document.getElementById('btn-open-processlist');
        const btnPriv = document.getElementById('btn-open-privileges');
        const btnFR = document.getElementById('btn-open-flight-recorder');
        if (btnFR) btnFR.classList.remove('hidden');
        const btnVars = document.getElementById('btn-open-variables');
        if (btnVars) btnVars.classList.remove('hidden');
        const btnCluster = document.getElementById('btn-open-cluster');
        checkFlightRecorderStatus();

        if (btnPL) {
          if (conn.driver === 'mysql' || conn.driver === 'mariadb') {
            btnPL.classList.remove('hidden');
            if (btnPriv) btnPriv.classList.remove('hidden');
            if (btnCluster) btnCluster.classList.remove('hidden');
          } else {
            btnPL.classList.add('hidden');
            if (btnPriv) btnPriv.classList.add('hidden');
            if (btnCluster) btnCluster.classList.add('hidden');
            const plTab = tabs.find(t => t.isProcesslist);
            if (plTab) closeTab(plTab.id);
            const clTab = tabs.find(t => t.isCluster);
            if (clTab) closeTab(clTab.id);
          }
        }
        updateReadOnlyUI(activeSession.read_only);
        await loadSchemaTree(conn.id);
        restoreTabsState(conn.id);
        updateFooterStats(data.latency_ms, data.memory_mb);
      } catch (err) {
        // Safe Reversion: restore previous connection state on network exception
        activeConnection = prevConnection;
        renderHeaderConnectionBadge(prevConnection);
        renderConnectionsTree();
        document.getElementById('schema-tree').innerHTML = `<div class="text-app-danger p-2 text-xs">Connection Error: ${escapeHtml(err.message)}</div>`;
        showNativeAlert('Failed to connect: ' + err.message, 'Connection Error');
      } finally {
        clearTimeout(slowConnTimer);
        isConnectionSwitching = false;
        if (sessionBtn) {
          sessionBtn.classList.remove('cursor-wait', 'opacity-75', 'pointer-events-none');
        }
        if (sessionIcon) {
          sessionIcon.innerHTML = prevSessionIconHTML || `<svg class="w-3.5 h-3.5 text-sky-400 shrink-0" aria-hidden="true"><use href="#icon-database"/></svg>`;
        }
        renderHeaderConnectionBadge(activeConnection);
      }
    }

    async function loadSchemaTree(connId, targetDB = '') {
      const treeEl = document.getElementById('schema-tree');
      if (treeEl) {
        treeEl.innerHTML = `<div class="text-slate-400 text-center py-6 flex flex-col items-center justify-center space-y-2 text-xs font-mono">
          <div class="flex items-center space-x-2">
            ${renderIcon('loader', 'w-4 h-4 animate-spin text-sky-400')}
            <span>Inspecting schema${targetDB ? ' (' + escapeHtml(targetDB) + ')' : ''}...</span>
          </div>
          <span class="text-[10px] text-slate-500">Querying database metadata & tables</span>
        </div>`;
      }
      try {
        const url = targetDB
          ? `/api/session/${connId}/schema?db=${encodeURIComponent(targetDB)}`
          : `/api/session/${connId}/schema`;
        const res = await apiFetch(url);
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || res.statusText || 'Failed to inspect schema');
        }
        const schema = await res.json();
        schemaCatalogCache = schema;

        // Index into multi-database catalog cache
        const activeDb = (schema.database_name || targetDB || 'default').toLowerCase();
        multiDBCatalogCache[activeDb] = schema;
        if (!sessionCatalogTimestamps[connId]) sessionCatalogTimestamps[connId] = {};
        sessionCatalogTimestamps[connId][activeDb] = Date.now();
        updateSchemaTimestampBadge(activeDb);

        // Fetch lightweight fast-catalog in background (only if not already populated)
        // ponytail: fast-catalog inspects only tables and columns in a single query, eliminating heavy foreign key scans on background databases
        const hasOtherDBs = Object.keys(multiDBCatalogCache).some(k => k !== activeDb);
        if (!hasOtherDBs) {
          (async () => {
            try {
              const r = await apiFetch(`/api/session/${connId}/fast-catalog`, { silent: true });
              if (r.ok) {
                const fastCat = await r.json();
                if (fastCat && typeof fastCat === 'object') {
                  const now = Date.now();
                  Object.keys(fastCat).forEach(dbKey => {
                    const lk = dbKey.toLowerCase();
                    if (!multiDBCatalogCache[lk]) {
                      multiDBCatalogCache[lk] = {
                        database_name: dbKey,
                        databases: schema.databases || [],
                        tables: fastCat[dbKey] || [],
                        triggers: [],
                        routines: [],
                        events: []
                      };
                      sessionCatalogTimestamps[connId][lk] = now;
                    }
                  });
                }
              }
            } catch (_) {}
          })();
        }

        renderSchemaTree(schema);
      } catch (err) {
        if (treeEl) treeEl.innerHTML = `<div class="text-app-danger p-2 text-xs">Error: ${escapeHtml(err.message)}</div>`;
        throw err;
      }
    }

