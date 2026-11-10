// --- Processlist Manager & Lock Dependency Tree ---
    let processlistData = { current_thread_id: 0, threads: [], locks: [] };
    let statementDigestData = [];
    let deadlockReportData = null;
    let currentProcesslistView = 'threads'; // 'threads', 'digest', 'deadlock'
    let processlistRefreshInterval = 'pause';
    let processlistTimer = null;
    let _plFiltersInitialized = false;
    const plTrendHistory = [];
    let plTrendPanelOpen = false;

    function switchProcesslistView(view) {
      currentProcesslistView = view;

      const threadsBtn = document.getElementById('pl-view-threads-btn');
      const digestBtn = document.getElementById('pl-view-digest-btn');
      const deadlockBtn = document.getElementById('pl-view-deadlock-btn');

      const threadsSec = document.getElementById('pl-threads-section');
      const digestSec = document.getElementById('pl-digest-section');
      const deadlockSec = document.getElementById('pl-deadlock-section');
      const refreshGroup = document.getElementById('pl-refresh-control-group');

      if (threadsBtn) threadsBtn.className = view === 'threads'
        ? 'px-2.5 py-1 rounded text-xs bg-slate-800 text-sky-400 font-semibold flex items-center space-x-1.5 transition'
        : 'px-2.5 py-1 rounded text-xs text-slate-400 hover:text-white flex items-center space-x-1.5 transition';

      if (digestBtn) digestBtn.className = view === 'digest'
        ? 'px-2.5 py-1 rounded text-xs bg-slate-800 text-sky-400 font-semibold flex items-center space-x-1.5 transition'
        : 'px-2.5 py-1 rounded text-xs text-slate-400 hover:text-white flex items-center space-x-1.5 transition';

      if (deadlockBtn) deadlockBtn.className = view === 'deadlock'
        ? 'px-2.5 py-1 rounded text-xs bg-slate-800 text-red-400 font-semibold flex items-center space-x-1.5 transition'
        : 'px-2.5 py-1 rounded text-xs text-slate-400 hover:text-white flex items-center space-x-1.5 transition';

      if (threadsSec) threadsSec.classList.toggle('hidden', view !== 'threads');
      if (digestSec) digestSec.classList.toggle('hidden', view !== 'digest');
      if (deadlockSec) deadlockSec.classList.toggle('hidden', view !== 'deadlock');

      // Subtoolbar, refresh controls, counters, and lock summary only visible on live threads view
      const filterSubbar = document.getElementById('pl-filter-subtoolbar');
      if (filterSubbar) filterSubbar.classList.toggle('hidden', view !== 'threads');
      if (refreshGroup) refreshGroup.classList.toggle('hidden', view !== 'threads');

      const summaryText = document.getElementById('pl-summary-text');
      if (summaryText) summaryText.classList.toggle('hidden', view !== 'threads');

      const refreshNowBtn = document.getElementById('pl-refresh-now-btn');
      if (refreshNowBtn) refreshNowBtn.classList.toggle('hidden', view !== 'threads');

      const trendBtn = document.getElementById('pl-toggle-trend-btn');
      if (trendBtn) trendBtn.classList.toggle('hidden', view !== 'threads');

      const lockSummary = document.getElementById('pl-lock-summary');
      if (lockSummary) {
        if (view !== 'threads') {
          lockSummary.classList.add('hidden');
        } else if ((processlistData?.locks || []).length > 0) {
          lockSummary.classList.remove('hidden');
        }
      }

      if (view !== 'threads') {
        const trendPanel = document.getElementById('pl-trend-panel');
        if (trendPanel && !trendPanel.classList.contains('hidden')) {
          toggleProcesslistTrendPanel();
        }
      }

      if (view === 'digest' && (!statementDigestData || statementDigestData.length === 0)) {
        fetchStatementDigest();
      } else if (view === 'deadlock' && !deadlockReportData) {
        fetchDeadlockStatus();
      }
    }

    async function fetchStatementDigest() {
      if (!activeConnection) return;
      const tbody = document.getElementById('pl-digest-tbody');
      if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-8 text-center text-slate-400"><div class="animate-pulse">Loading statement digest telemetry...</div></td></tr>`;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/statement-digest?limit=30`);
        const data = await res.json();
        if (!res.ok) {
          if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-8 text-center text-red-400">Failed to load statement digest: ${escapeHtml(data.error || res.statusText)}</td></tr>`;
          return;
        }
        statementDigestData = Array.isArray(data) ? data : [];
        renderStatementDigestUI();
      } catch (err) {
        if (tbody) tbody.innerHTML = `<tr><td colspan="8" class="p-8 text-center text-red-400">Network error loading statement digest: ${escapeHtml(err.message)}</td></tr>`;
      }
    }

    function renderStatementDigestUI() {
      const tbody = document.getElementById('pl-digest-tbody');
      if (!tbody) return;

      if (!statementDigestData || statementDigestData.length === 0) {
        tbody.innerHTML = `<tr><td colspan="8" class="p-8 text-center text-slate-500 italic">No statement digest data available. Ensure performance_schema is enabled.</td></tr>`;
        return;
      }

      tbody.innerHTML = statementDigestData.map((item, idx) => {
        const rank = idx + 1;
        const totalLat = (item.total_latency_ms || 0).toLocaleString();
        const avgLat = (item.avg_latency_ms || 0).toLocaleString();
        const execs = (item.exec_count || 0).toLocaleString();
        const rowsExam = (item.rows_examined || 0).toLocaleString();
        const rowsSent = (item.rows_sent || 0).toLocaleString();
        const fullScanBadge = item.full_scan
          ? `<span class="bg-red-950/60 text-red-400 border border-red-800/80 px-1.5 py-0.5 rounded text-[10px] font-bold">YES</span>`
          : `<span class="text-slate-500 text-[10px]">No</span>`;

        return `
          <tr class="hover:bg-slate-800/50 transition border-b border-slate-800/60 font-mono text-xs">
            <td class="p-2.5 text-center text-slate-400 font-bold">${rank}</td>
            <td class="p-2.5 text-right font-medium text-sky-400">${execs}</td>
            <td class="p-2.5 text-right font-bold text-amber-300">${totalLat} ms</td>
            <td class="p-2.5 text-right text-slate-300">${avgLat} ms</td>
            <td class="p-2.5 text-right text-slate-400">${rowsExam} / ${rowsSent}</td>
            <td class="p-2.5 text-center">${fullScanBadge}</td>
            <td class="p-2.5 max-w-xl truncate text-slate-200" title="${escapeHtml(item.query)}">
              <code>${escapeHtml(item.query || '-')}</code>
            </td>
            <td class="p-2.5 text-center">
              <button data-query="${escapeHtml(item.query || '')}" onclick="sendProcessQueryToEditor(this.dataset.query)" class="bg-app-card hover:bg-slate-700 text-slate-300 px-2 py-0.5 rounded text-[11px] border border-app-border transition" title="Open query in Editor tab">
                ${renderIcon('edit', 'w-3 h-3 inline mr-1 text-sky-400')}<span>Edit</span>
              </button>
            </td>
          </tr>
        `;
      }).join('');
    }

    function sendDigestQueryToEditor(encodedSQL) {
      try {
        const sql = decodeURIComponent(encodedSQL);
        if (sql) {
          sendProcessQueryToEditor(sql);
        }
      } catch (err) {
        console.error('sendDigestQueryToEditor failed:', err);
      }
    }

    async function confirmResetStatementDigest() {
      if (!activeConnection) return;
      const ok = await showNativeConfirm('Are you sure you want to reset and truncate statement digest statistics on this server?', 'Reset Statement Digest');
      if (!ok) return;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/statement-digest/reset`, {
          method: 'POST'
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert('Failed to reset digest: ' + (data.error || res.statusText), 'Reset Error');
          return;
        }
        showNativeAlert('Statement digest statistics reset successfully.', 'Reset Complete');
        await fetchStatementDigest();
      } catch (err) {
        showNativeAlert('Reset request failed: ' + err.message, 'Network Error');
      }
    }

    async function fetchDeadlockStatus() {
      if (!activeConnection) return;
      const container = document.getElementById('pl-deadlock-content');
      if (container) container.innerHTML = `<div class="p-8 text-center text-slate-400"><div class="animate-pulse">Inspecting SHOW ENGINE INNODB STATUS...</div></div>`;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/deadlock`);
        const data = await res.json();
        if (!res.ok) {
          if (container) container.innerHTML = `<div class="p-8 text-center text-red-400">Failed to inspect deadlock: ${escapeHtml(data.error || res.statusText)}</div>`;
          return;
        }
        deadlockReportData = data;
        renderDeadlockUI();
      } catch (err) {
        if (container) container.innerHTML = `<div class="p-8 text-center text-red-400">Network error inspecting deadlock: ${escapeHtml(err.message)}</div>`;
      }
    }

    function renderDeadlockUI() {
      const container = document.getElementById('pl-deadlock-content');
      const timeBadge = document.getElementById('pl-deadlock-time-badge');
      if (!container) return;

      if (!deadlockReportData || !deadlockReportData.has_deadlock) {
        if (timeBadge) timeBadge.textContent = '';
        container.innerHTML = `
          <div class="bg-emerald-950/30 border border-emerald-900/50 rounded-lg p-6 text-center space-y-2 max-w-xl mx-auto my-8">
            <div class="w-10 h-10 mx-auto rounded-full bg-emerald-500/10 border border-emerald-500/25 flex items-center justify-center text-emerald-400">${renderIcon('shield', 'w-6 h-6')}</div>
            <div class="text-sm font-bold text-emerald-300">Clean Engine State</div>
            <p class="text-xs text-emerald-400/80 leading-relaxed">
              ${escapeHtml(deadlockReportData?.summary || 'No deadlocks detected since MySQL server restart.')}
            </p>
          </div>
        `;
        return;
      }

      if (timeBadge && deadlockReportData.detected_at) {
        timeBadge.textContent = `Timestamp: ${deadlockReportData.detected_at}`;
      }

      const txs = deadlockReportData.transactions || [];
      const txCards = txs.map(tx => {
        const victimBadge = tx.is_victim
          ? `<span class="bg-red-950 text-red-300 border border-red-700 px-2 py-0.5 rounded text-[11px] font-bold inline-flex items-center gap-1">${renderIcon('alert-triangle', 'w-3 h-3 text-red-400')}<span>ROLLED BACK (VICTIM)</span></span>`
          : `<span class="bg-emerald-950 text-emerald-300 border border-emerald-700 px-2 py-0.5 rounded text-[11px] font-bold inline-flex items-center gap-1">${renderIcon('check', 'w-3 h-3 text-emerald-400')}<span>UNBLOCKED (COMMITTED)</span></span>`;

        return `
          <div class="bg-slate-900 border ${tx.is_victim ? 'border-red-800/80' : 'border-slate-700'} rounded-lg p-4 space-y-3 min-w-0 overflow-hidden shadow">
            <div class="flex items-center justify-between border-b border-slate-800 pb-2">
              <div class="flex items-center space-x-2">
                <span class="font-bold text-white text-sm">Transaction ${tx.index}</span>
                <span class="font-mono text-xs text-slate-400">Trx ID: ${escapeHtml(tx.tx_id || '-')}</span>
                <span class="font-mono text-xs text-sky-400">Thread #${tx.thread_id}</span>
              </div>
              ${victimBadge}
            </div>

            <div class="grid grid-cols-2 gap-2 text-xs font-mono">
              <div><span class="text-slate-400">Active Duration:</span> <span class="text-slate-200">${tx.active_seconds}s</span></div>
              <div><span class="text-slate-400">Undo Logs:</span> <span class="text-slate-200">${tx.undo_log_entries}</span></div>
              <div class="truncate" title="${escapeHtml(tx.app_user || '-') } / ${escapeHtml(tx.host || '-')}"><span class="text-slate-400">User / Host:</span> <span class="text-slate-200">${escapeHtml(tx.app_user || '-') } / ${escapeHtml(tx.host || '-')}</span></div>
              <div><span class="text-slate-400">Lock Mode:</span> <span class="text-amber-300">${escapeHtml(tx.lock_mode || '-')}</span></div>
            </div>

            ${tx.lock_held ? `<div class="text-xs bg-slate-950 p-2 rounded border border-slate-800 overflow-hidden"><span class="text-slate-400 block font-semibold mb-0.5">Holds Lock:</span><code class="text-emerald-300 text-[11px] block break-all whitespace-normal leading-relaxed">${escapeHtml(tx.lock_held)}</code></div>` : ''}
            ${tx.lock_waited ? `<div class="text-xs bg-slate-950 p-2 rounded border border-slate-800 overflow-hidden"><span class="text-slate-400 block font-semibold mb-0.5">Waiting For Lock:</span><code class="text-amber-300 text-[11px] block break-all whitespace-normal leading-relaxed">${escapeHtml(tx.lock_waited)}</code></div>` : ''}

            <div class="space-y-1">
              <div class="flex items-center justify-between text-xs text-slate-400">
                <span class="font-semibold">Executing SQL Statement:</span>
                <button data-query="${escapeHtml(tx.query || '')}" onclick="sendProcessQueryToEditor(this.dataset.query)" class="text-sky-400 hover:text-sky-200 text-[11px]">📝 Open in Editor</button>
              </div>
              <pre class="bg-slate-950 border border-slate-800 p-2.5 rounded font-mono text-xs text-slate-200 overflow-x-auto whitespace-pre-wrap break-all max-h-48 leading-relaxed">${escapeHtml(tx.query || '-- Query not available')}</pre>
            </div>
          </div>
        `;
      }).join('');

      container.innerHTML = `
        <div class="bg-red-950/30 border border-red-900/60 rounded-lg p-3.5 flex items-center justify-between text-xs text-red-200 w-full">
          <div class="flex items-center space-x-2">
            ${renderIcon('shield-alert', 'w-5 h-5 text-rose-400')}
            <span class="font-bold">${escapeHtml(deadlockReportData.summary || 'Deadlock detected between concurrent transactions.')}</span>
          </div>
          <span class="text-[11px] text-red-400 font-mono">${escapeHtml(deadlockReportData.detected_at || '')}</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 w-full">
          ${txCards}
        </div>

        ${deadlockReportData.raw_section ? `
          <details class="bg-slate-900 border border-slate-800 rounded-lg p-3 text-xs w-full overflow-hidden">
            <summary class="font-semibold text-slate-300 cursor-pointer select-none">Raw InnoDB Deadlock Status Dump</summary>
            <pre class="mt-2.5 bg-slate-950 border border-slate-800 p-3 rounded font-mono text-[11px] text-slate-400 overflow-x-auto whitespace-pre leading-relaxed break-words">${escapeHtml(deadlockReportData.raw_section)}</pre>
          </details>
        ` : ''}
      `;
    }


    // --- Processlist Batch Operations & Quick Filter Chips (ADR 0052) ---
    let plActiveChip = 'all';
    let plSelectedThreadIds = new Set();
    let plSavedIntervalBeforeModal = null;
    let plBatchKillMode = 'query';

    function setProcesslistFilterChip(chip) {
      plActiveChip = chip || 'all';
      updateProcesslistFilterChipsUI();
      renderProcesslistUI();
    }

    function updateProcesslistFilterChipsUI() {
      const container = document.getElementById('pl-filter-chips');
      if (!container) return;
      const btns = container.querySelectorAll('button[data-pl-chip]');
      btns.forEach(btn => {
        const chip = btn.getAttribute('data-pl-chip');
        if (chip === plActiveChip) {
          btn.className = 'px-2 py-0.5 rounded text-white bg-slate-800 font-semibold transition cursor-pointer';
        } else {
          btn.className = 'px-2 py-0.5 rounded text-slate-400 hover:text-white transition cursor-pointer';
        }
      });
    }

    function toggleSelectAllProcesslistThreads(checked) {
      const threads = processlistData.threads || [];
      const locks = processlistData.locks || [];
      const currentThreadId = processlistData.current_thread_id || 0;
      const hideSleep = document.getElementById('pl-hide-sleep')?.checked ?? false;
      const hideSystem = document.getElementById('pl-hide-system')?.checked ?? false;
      const thresholdEl = document.getElementById('pl-slow-threshold');
      const minSeconds = thresholdEl ? (parseInt(thresholdEl.value, 10) || 0) : 0;
      const searchEl = document.getElementById('pl-search-input');
      const search = searchEl ? searchEl.value.trim().toLowerCase() : '';

      const visibleSelectable = threads.filter(t => {
        const isSelf = t.id === currentThreadId;
        const isSystem = (t.user === 'system user' || t.user === 'event_scheduler' || t.command === 'Binlog Dump');
        if (isSelf || isSystem) return false;

        if (hideSleep && (t.command === 'Sleep' || t.state === 'Sleep')) return false;
        if (hideSystem && (t.user === 'system user' || t.user === 'event_scheduler' || t.command === 'Binlog Dump')) return false;
        if (minSeconds > 0 && t.time < minSeconds) return false;
        if (search) {
          const matchUser = (t.user || '').toLowerCase().includes(search);
          const matchHost = (t.host || '').toLowerCase().includes(search);
          const matchDb = (t.db || '').toLowerCase().includes(search);
          const matchInfo = (t.info || '').toLowerCase().includes(search);
          const matchState = (t.state || '').toLowerCase().includes(search);
          const matchCmd = (t.command || '').toLowerCase().includes(search);
          const matchId = String(t.id).includes(search);
          if (!matchUser && !matchHost && !matchDb && !matchInfo && !matchState && !matchCmd && !matchId) return false;
        }

        if (plActiveChip === 'active' && (t.command === 'Sleep' || t.state === 'Sleep')) return false;
        if (plActiveChip === 'locked') {
          const isLocked = locks.some(l => l.waiting_thread_id === t.id || l.blocking_thread_id === t.id);
          if (!isLocked) return false;
        }
        if (plActiveChip === 'slow' && t.time < 5) return false;
        if (plActiveChip === 'sleep' && !((t.command === 'Sleep' || t.state === 'Sleep') && t.time >= 60)) return false;

        return true;
      });

      if (checked) {
        visibleSelectable.forEach(t => plSelectedThreadIds.add(t.id));
      } else {
        visibleSelectable.forEach(t => plSelectedThreadIds.delete(t.id));
      }
      renderProcesslistUI();
    }

    function onProcesslistThreadCheckboxChange(threadId, checked) {
      const tid = Number(threadId);
      if (checked) {
        plSelectedThreadIds.add(tid);
      } else {
        plSelectedThreadIds.delete(tid);
      }
      updateProcesslistBatchBarUI();
      updateProcesslistSelectAllHeader();
    }

    function updateProcesslistSelectAllHeader() {
      const selectAllEl = document.getElementById('pl-select-all');
      if (!selectAllEl) return;
      const checkboxes = document.querySelectorAll('#pl-tbody input[type="checkbox"][data-pl-tid]');
      if (checkboxes.length === 0) {
        selectAllEl.checked = false;
        selectAllEl.disabled = true;
      } else {
        selectAllEl.disabled = false;
        selectAllEl.checked = Array.from(checkboxes).every(cb => cb.checked);
      }
    }

    function updateProcesslistBatchBarUI() {
      const bar = document.getElementById('pl-batch-action-bar');
      const countEl = document.getElementById('pl-batch-count');
      if (!bar) return;
      const count = plSelectedThreadIds.size;
      if (count > 0) {
        bar.classList.remove('hidden');
        if (countEl) countEl.textContent = `${count} Selected`;
      } else {
        bar.classList.add('hidden');
      }
    }

    function clearProcesslistSelection() {
      plSelectedThreadIds.clear();
      renderProcesslistUI();
    }

    async function copyBatchThreadIds() {
      if (plSelectedThreadIds.size === 0) return;
      const idsStr = Array.from(plSelectedThreadIds).sort((a, b) => a - b).join(', ');
      const ok = await copyTextToClipboard(idsStr);
      if (ok) {
        showToast(`Copied ${plSelectedThreadIds.size} thread IDs to clipboard`, 'success');
      } else {
        showNativeAlert(`Thread IDs: ${idsStr}`, 'Thread IDs');
      }
    }

    function exportBatchProcesslist() {
      if (plSelectedThreadIds.size === 0) return;
      const threads = (processlistData.threads || []).filter(t => plSelectedThreadIds.has(Number(t.id)));
      const jsonStr = JSON.stringify(threads, null, 2);
      const blob = new Blob([jsonStr], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `sqlfox-processlist-batch-${Date.now()}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      showToast(`Exported ${threads.length} threads to JSON`, 'success');
    }

    function saveBatchProcessesToNotes() {
      if (plSelectedThreadIds.size === 0) return;
      const threads = (processlistData.threads || []).filter(t => plSelectedThreadIds.has(Number(t.id)) && t.info && t.command !== 'Sleep');
      if (threads.length === 0) {
        showToast('None of the selected threads have active query statements to save.', 'warning');
        return;
      }
      threads.forEach(t => {
        saveProcessQueryToNotes(t.info, t.db);
      });
      showToast(`Saved ${threads.length} query note(s) to Notes Inbox`, 'success');
    }

    function removeThreadFromBatchKill(threadId) {
      const numId = Number(threadId);
      plSelectedThreadIds.delete(numId);

      // Uncheck in the main processlist table
      const cb = document.querySelector(`input[data-pl-tid="${numId}"]`);
      if (cb) cb.checked = false;

      // Update select-all header checkbox and floating batch dock
      updateProcesslistSelectAllHeader();
      updateProcesslistBatchBarUI();

      if (plSelectedThreadIds.size === 0) {
        closeBatchKillModal();
        showToast('All threads removed from termination batch', 'info');
        return;
      }

      // Re-render modal content
      renderBatchKillModalContent();
    }

    function renderBatchKillModalContent() {
      const isConn = plBatchKillMode === 'connection';
      const actionName = isConn ? 'KILL CONNECTION' : 'KILL QUERY';

      const titleEl = document.getElementById('pl-batch-modal-title');
      const countEl = document.getElementById('pl-batch-modal-count');
      const subEl = document.getElementById('pl-batch-modal-subtitle');
      const iconEl = document.getElementById('pl-batch-modal-icon');
      const execBtn = document.getElementById('pl-batch-execute-btn');
      const listEl = document.getElementById('pl-batch-modal-list');
      const challengeBox = document.getElementById('pl-batch-challenge-box');
      const challengeInput = document.getElementById('pl-batch-challenge-input');
      const challengeReason = document.getElementById('pl-batch-challenge-reason');

      if (titleEl) titleEl.textContent = `Confirm Batch ${actionName}`;
      if (countEl) countEl.textContent = `${plSelectedThreadIds.size} Threads`;
      if (subEl) subEl.textContent = isConn
        ? 'Disconnecting client connections will terminate their database sessions immediately.'
        : 'Cancelling running queries will abort active statements while keeping client connections open.';
      if (iconEl) iconEl.innerHTML = renderIcon(isConn ? 'slash' : 'square', 'w-5 h-5 text-rose-400');
      if (execBtn) {
        execBtn.innerHTML = `<span id="pl-batch-execute-icon">${renderIcon(isConn ? 'slash' : 'square', 'w-3.5 h-3.5 inline mr-1')}</span><span id="pl-batch-execute-label">Execute ${actionName}</span>`;
      }

      const selectedThreads = (processlistData.threads || []).filter(t => plSelectedThreadIds.has(Number(t.id)));
      if (listEl) {
        listEl.innerHTML = selectedThreads.map(t => {
          const rawSql = t.info || (t.command === 'Sleep' ? 'Idle connection (Sleep)' : '-');
          const displaySql = rawSql.replace(/[\r\n\t]+/g, ' ').trim();
          const sqlSnippet = displaySql.length > 90 ? displaySql.slice(0, 90) + '...' : displaySql;
          return `
            <div class="py-1.5 flex items-center justify-between gap-2 group hover:bg-slate-900/60 px-1.5 rounded transition">
              <div class="min-w-0 flex-1">
                <div class="flex items-center space-x-1.5">
                  <span class="font-bold text-sky-400">#${t.id}</span>
                  <span class="text-slate-300 font-semibold">${escapeHtml(t.user)}@${escapeHtml(t.host)}</span>
                  <span class="text-slate-500">(${escapeHtml(t.db || '-')})</span>
                  <span class="px-1 py-0.2 rounded bg-slate-800 text-slate-300 text-[10px]">${escapeHtml(t.command)}</span>
                </div>
                <div class="text-[10px] text-slate-400 truncate mt-0.5" title="${escapeHtml(rawSql)}">${escapeHtml(sqlSnippet)}</div>
              </div>
              <div class="flex items-center space-x-2 shrink-0">
                <span class="font-bold text-right ${t.time >= 30 ? 'text-red-400' : (t.time >= 5 ? 'text-amber-400' : 'text-slate-300')}">${t.time}s</span>
                <button type="button" onclick="removeThreadFromBatchKill(${t.id})" class="text-slate-500 hover:text-red-400 hover:bg-slate-800 p-1 rounded transition text-xs cursor-pointer flex items-center justify-center w-5 h-5" title="Remove thread #${t.id} from this batch"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
              </div>
            </div>
          `;
        }).join('');
      }

      // Check if Safety Challenge is required (Production environment OR >5 threads)
      const isProd = activeConnection && (activeConnection.environment || '').toLowerCase() === 'production';
      const isLargeBatch = plSelectedThreadIds.size > 5;
      const needsChallenge = isProd || isLargeBatch;

      if (challengeBox) {
        if (needsChallenge) {
          challengeBox.classList.remove('hidden');
          if (challengeReason) {
            challengeReason.textContent = isProd
              ? 'Production Environment Safety Guardrail'
              : `Large Batch Safety Guardrail (${plSelectedThreadIds.size} threads)`;
          }
          if (execBtn) {
            const isMatch = challengeInput && challengeInput.value.trim().toUpperCase() === 'KILL';
            execBtn.disabled = !isMatch;
          }
        } else {
          challengeBox.classList.add('hidden');
          if (execBtn) execBtn.disabled = false;
        }
      }
    }

    function openBatchKillModal(mode) {
      if (plSelectedThreadIds.size === 0) return;
      plBatchKillMode = mode === 'connection' ? 'connection' : 'query';
      const dlg = document.getElementById('pl-batch-kill-dialog');
      if (!dlg) return;

      // Temporarily pause polling
      if (typeof processlistRefreshInterval !== 'undefined') {
        plSavedIntervalBeforeModal = processlistRefreshInterval;
      }
      if (typeof processlistTimer !== 'undefined' && processlistTimer) {
        clearInterval(processlistTimer);
        processlistTimer = null;
      }

      renderBatchKillModalContent();

      const challengeInput = document.getElementById('pl-batch-challenge-input');
      if (challengeInput) {
        challengeInput.value = '';
        setTimeout(() => challengeInput.focus(), 50);
      }

      if (typeof dlg.showModal === 'function') {
        if (!dlg.open) {
          dlg.showModal();
        }
      } else {
        dlg.setAttribute('open', '');
      }
    }

    function validateBatchKillChallenge() {
      const inp = document.getElementById('pl-batch-challenge-input');
      const btn = document.getElementById('pl-batch-execute-btn');
      if (!inp || !btn) return;
      const isMatch = inp.value.trim().toUpperCase() === 'KILL';
      btn.disabled = !isMatch;
    }

    function closeBatchKillModal() {
      const dlg = document.getElementById('pl-batch-kill-dialog');
      if (dlg && dlg.open) dlg.close();
      // Restore polling
      if (plSavedIntervalBeforeModal && plSavedIntervalBeforeModal !== 'pause') {
        setProcesslistInterval(plSavedIntervalBeforeModal);
      }
      plSavedIntervalBeforeModal = null;
    }

    async function executeBatchKill() {
      if (!activeConnection || plSelectedThreadIds.size === 0) return;
      const targetIds = Array.from(plSelectedThreadIds);
      const mode = plBatchKillMode;
      const modeLabel = mode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY';

      const execBtn = document.getElementById('pl-batch-execute-btn');
      if (execBtn) {
        execBtn.disabled = true;
        execBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin inline mr-1')}<span>Terminating...</span>`;
      }

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/batch-kill`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ thread_ids: targetIds, mode: mode })
        });
        const data = await res.json();
        closeBatchKillModal();

        if (!res.ok) {
          showNativeAlert('Batch termination failed: ' + (data.error || res.statusText), 'Batch Kill Error');
          return;
        }

        const successCount = data.success_count || 0;
        const failedCount = data.failed_count || 0;

        if (failedCount === 0) {
          showToast(`✓ Batch ${modeLabel} succeeded for ${successCount} thread(s)`, 'success');
        } else if (successCount > 0) {
          const failReasons = (data.results || [])
            .filter(r => !r.success)
            .map(r => `#${r.thread_id}: ${r.error}`)
            .slice(0, 3)
            .join(', ');
          showToast(`⚠️ Batch ${modeLabel}: ${successCount} terminated, ${failedCount} skipped (${failReasons})`, 'warning');
        } else {
          showToast(`❌ Batch ${modeLabel} failed: all ${failedCount} thread(s) rejected or skipped`, 'error');
        }

        // Clear selection & refetch processlist
        clearProcesslistSelection();
        await fetchProcesslist(true);
        // ponytail: Deferred refresh to capture async DB engine cleanup / rollback completion
        setTimeout(() => {
          fetchProcesslist(true);
        }, 750);
      } catch (err) {
        closeBatchKillModal();
        showNativeAlert('Batch termination request failed: ' + err.message, 'Network Error');
      }
    }

    function initProcesslistFilters() {
      const sleepEl = document.getElementById('pl-hide-sleep');
      const sysEl = document.getElementById('pl-hide-system');
      if (sleepEl) {
        const stored = localStorage.getItem('sqlfox_pl_hide_sleep');
        sleepEl.checked = stored !== null ? stored === 'true' : true;
      }
      if (sysEl) {
        const stored = localStorage.getItem('sqlfox_pl_hide_system');
        sysEl.checked = stored !== null ? stored === 'true' : true;
      }
      const trendStored = localStorage.getItem('sqlfox_pl_trend_open');
      if (trendStored === 'true') {
        toggleProcesslistTrendPanel(true, true);
      }
      _plFiltersInitialized = true;
      applyProcesslistColVisibility();
    }

    function onProcesslistFilterChange() {
      const sleepEl = document.getElementById('pl-hide-sleep');
      const sysEl = document.getElementById('pl-hide-system');
      if (sleepEl) localStorage.setItem('sqlfox_pl_hide_sleep', sleepEl.checked);
      if (sysEl) localStorage.setItem('sqlfox_pl_hide_system', sysEl.checked);
      renderProcesslistUI();
    }

    function onProcesslistSearchInput(val) {
      const clearBtn = document.getElementById('pl-search-clear-btn');
      if (clearBtn) {
        if (val && val.length > 0) {
          clearBtn.classList.remove('hidden');
        } else {
          clearBtn.classList.add('hidden');
        }
      }
      renderProcesslistUI();
    }

    function clearProcesslistSearch() {
      const input = document.getElementById('pl-search-input');
      const clearBtn = document.getElementById('pl-search-clear-btn');
      if (input) {
        input.value = '';
        input.focus();
      }
      if (clearBtn) {
        clearBtn.classList.add('hidden');
      }
      renderProcesslistUI();
    }

    // --- Processlist Column Visibility Management (ADR 0054) ---
    const PL_COLUMNS = [
      { key: 'rownum', label: '#', mandatory: true, inPicker: false },
      { key: 'select', label: 'Select', mandatory: true, inPicker: false },
      { key: 'action', label: 'Action', mandatory: false, inPicker: true },
      { key: 'id', label: 'ID', mandatory: true, inPicker: true },
      { key: 'user', label: 'User', mandatory: false, inPicker: true },
      { key: 'host', label: 'Host', mandatory: false, inPicker: true },
      { key: 'db', label: 'Database', mandatory: false, inPicker: true },
      { key: 'command', label: 'Command', mandatory: false, inPicker: true },
      { key: 'time', label: 'Time (s)', mandatory: false, inPicker: true },
      { key: 'state', label: 'State', mandatory: false, inPicker: true },
      { key: 'query', label: 'SQL / Query Info', mandatory: true, inPicker: true }
    ];

    let plHiddenColumns = new Set();
    try {
      const stored = localStorage.getItem('sqlfox-processlist-hidden-cols');
      if (stored) {
        const parsed = JSON.parse(stored);
        if (Array.isArray(parsed)) {
          // Strictly protect mandatory columns from being hidden
          const valid = parsed.filter(k => k !== 'rownum' && k !== 'select' && k !== 'id' && k !== 'query');
          plHiddenColumns = new Set(valid);
        }
      }
    } catch (e) {}

    function isProcesslistColVisible(key) {
      if (key === 'rownum' || key === 'select' || key === 'id' || key === 'query') return true;
      return !plHiddenColumns.has(key);
    }

    function toggleProcesslistColPicker(e) {
      if (e) e.stopPropagation();
      const dropdown = document.getElementById('pl-col-picker-dropdown');
      if (!dropdown) return;
      const isHidden = dropdown.classList.contains('hidden');
      closeAllContextMenus();
      if (isHidden) {
        renderProcesslistColPickerList();
        dropdown.classList.remove('hidden');
      } else {
        dropdown.classList.add('hidden');
      }
    }

    function renderProcesslistColPickerList() {
      const container = document.getElementById('pl-col-picker-list');
      if (!container) return;
      const pickerCols = PL_COLUMNS.filter(c => c.inPicker);

      container.innerHTML = pickerCols.map(c => {
        const isMandatory = c.mandatory;
        const isVisible = isProcesslistColVisible(c.key);
        const lockBadge = isMandatory ? '<span class="text-amber-400 font-bold ml-1 text-[10px]" title="Mandatory column cannot be hidden">🔒 Mandatory</span>' : '';
        return `
          <label class="flex items-center justify-between p-1 hover:bg-slate-800 rounded cursor-pointer select-none text-[11px] ${isMandatory ? 'opacity-80' : ''}">
            <div class="flex items-center space-x-2 truncate">
              <input type="checkbox" ${isVisible ? 'checked' : ''} ${isMandatory ? 'disabled' : ''}
                     onchange="setProcesslistColVisibility('${c.key}', this.checked)"
                     class="accent-sky-500 w-3.5 h-3.5 rounded cursor-pointer ${isMandatory ? 'cursor-not-allowed opacity-60' : ''}">
              <span class="truncate font-mono ${isVisible ? 'text-slate-200' : 'text-slate-500 line-through'}">${escapeHtml(c.label)}</span>
              ${lockBadge}
            </div>
          </label>
        `;
      }).join('');

      updateProcesslistColPickerBadge();
    }

    function setProcesslistColVisibility(key, isVisible) {
      if (key === 'select' || key === 'id' || key === 'query') return;
      if (isVisible) {
        plHiddenColumns.delete(key);
      } else {
        plHiddenColumns.add(key);
      }
      try {
        localStorage.setItem('sqlfox-processlist-hidden-cols', JSON.stringify([...plHiddenColumns]));
      } catch (e) {}

      applyProcesslistColVisibility();
      renderProcesslistColPickerList();
    }

    function showAllProcesslistColumns() {
      plHiddenColumns.clear();
      try {
        localStorage.removeItem('sqlfox-processlist-hidden-cols');
      } catch (e) {}
      applyProcesslistColVisibility();
      renderProcesslistColPickerList();
    }

    function resetDefaultProcesslistColumns() {
      showAllProcesslistColumns();
    }

    function updateProcesslistColPickerBadge() {
      const badge = document.getElementById('pl-col-picker-count');
      if (!badge) return;
      const pickerCols = PL_COLUMNS.filter(c => c.inPicker);
      const total = pickerCols.length;
      const visible = pickerCols.filter(c => isProcesslistColVisible(c.key)).length;
      badge.textContent = `(${visible}/${total})`;
    }

    function applyProcesslistColVisibility() {
      // Synchronize <th> elements in thead
      PL_COLUMNS.forEach(c => {
        const th = document.querySelector(`th[data-pl-col="${c.key}"]`);
        if (th) {
          th.classList.toggle('hidden', !isProcesslistColVisible(c.key));
        }
      });
      updateProcesslistColPickerBadge();
      // Synchronize <td> elements in tbody
      renderProcesslistUI();
    }

    function openProcesslistTab() {
      if (!activeConnection) {
        showNativeAlert('Please select and connect to a database session first.', 'Processlist');
        return;
      }
      initProcesslistFilters();
      let tab = tabs.find(t => t.isProcesslist);
      if (!tab) {
        tab = {
          id: 'tab-processlist',
          title: 'Processlist',
          isProcesslist: true,
        };
        tabs.push(tab);
      }
      switchTab(tab.id);
      fetchProcesslist(false);
    }

    // ponytail: zero-shift lock contention inspection via dedicated modal & unobtrusive toasts
    let currentLockInspectorLocks = [];

    async function copyLockQueryFromData(btn, lockIdx, field) {
      const lock = (currentLockInspectorLocks || [])[lockIdx];
      if (!lock) return;
      const sql = lock[field] || '';
      if (!sql) return;
      const ok = await copyTextToClipboard(sql);
      if (ok) {
        showToast('SQL query copied to clipboard', 'success');
        if (btn) {
          const origHTML = btn.innerHTML;
          btn.innerHTML = `${renderIcon('check', 'w-3.5 h-3.5 inline mr-1 text-emerald-400')}<span>Copied!</span>`;
          btn.classList.add('bg-emerald-900', 'text-emerald-200', 'border-emerald-700');
          setTimeout(() => {
            if (btn) {
              btn.innerHTML = origHTML;
              btn.classList.remove('bg-emerald-900', 'text-emerald-200', 'border-emerald-700');
            }
          }, 1500);
        }
      } else {
        showToast('Failed to copy SQL query', 'error');
      }
    }

    function openLockProcessDetail(threadId, lockIdx, field) {
      const lock = (currentLockInspectorLocks || [])[lockIdx];
      let fallback = null;
      if (lock) {
        const isBlocking = field === 'blocking_query';
        fallback = {
          user: isBlocking ? lock.blocking_user : lock.waiting_user,
          info: lock[field] || '',
          is_root_blocker: lock.is_root_blocker && isBlocking,
          time: isBlocking ? 0 : (lock.waiting_seconds || 0),
          state: isBlocking ? `Holding ${lock.lock_mode || 'Lock'}` : `Waiting for Thread #${lock.blocking_thread_id}`
        };
      }
      openProcessDetailModal(threadId, fallback);
    }

    function openLockInspectorModal() {
      const dlg = document.getElementById('lock-inspector-dialog');
      if (!dlg) return;
      renderLockInspectorModalContent();
      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    function closeLockInspectorModal() {
      const dlg = document.getElementById('lock-inspector-dialog');
      if (dlg) {
        if (typeof dlg.close === 'function') dlg.close();
        else dlg.removeAttribute('open');
      }
    }

    function renderLockInspectorModalContent() {
      const locks = (processlistData && processlistData.locks) || [];
      currentLockInspectorLocks = locks;
      const rootBlockerCount = locks.filter(l => l.is_root_blocker).length;
      const modalBadge = document.getElementById('lock-modal-badge');
      const modalSubtitle = document.getElementById('lock-modal-subtitle');
      const listEl = document.getElementById('lock-modal-tree-list');

      if (modalBadge) {
        modalBadge.textContent = rootBlockerCount > 0 ? `${rootBlockerCount} Root Blocker${rootBlockerCount > 1 ? 's' : ''}` : `${locks.length} Lock Waits`;
      }
      if (modalSubtitle) {
        modalSubtitle.textContent = rootBlockerCount > 0
          ? `${rootBlockerCount} root blocker(s) detected holding database transactions`
          : 'Root blocker dependency graph & waiting query mitigation';
      }

      if (!listEl) return;

      if (locks.length === 0) {
        listEl.innerHTML = `
          <div class="p-8 text-center text-slate-400 space-y-2 select-none">
            <div class="w-10 h-10 mx-auto rounded-full bg-emerald-500/10 border border-emerald-500/25 flex items-center justify-center text-emerald-400">${renderIcon('check-circle', 'w-6 h-6')}</div>
            <div class="font-bold text-slate-200 text-sm">No Active Lock Contention</div>
            <div class="text-xs text-slate-400">All database threads and transactions are running unobstructed.</div>
          </div>
        `;
        return;
      }

      listEl.innerHTML = locks.map((l, idx) => {
        if (l.is_root_blocker) {
          return `
            <div class="p-4 bg-red-950/30 border border-red-800 rounded-lg flex flex-col space-y-3">
              <div class="flex flex-wrap items-center justify-between gap-2.5">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="bg-red-600 text-white font-bold text-xs px-2.5 py-0.5 rounded tracking-wide inline-flex items-center gap-1">${renderIcon('alert-triangle', 'w-3.5 h-3.5')}<span>ROOT BLOCKER</span></span>
                  <span class="font-bold text-white text-base">Thread #${l.blocking_thread_id}</span>
                  <span class="text-slate-400 text-xs">(${escapeHtml(l.blocking_user || 'unknown')})</span>
                  <span class="text-amber-400 text-xs font-semibold px-2 py-0.5 bg-amber-950/60 rounded border border-amber-800/80">holding ${escapeHtml(l.lock_mode || 'LOCK')}</span>
                  ${l.locked_resource ? `<span class="text-sky-300 text-xs font-mono px-2 py-0.5 bg-sky-950/60 rounded border border-sky-800/80">on ${escapeHtml(l.locked_resource)}</span>` : ''}
                </div>
                <div class="flex items-center space-x-2 shrink-0">
                  <button onclick="openLockProcessDetail(${l.blocking_thread_id}, ${idx}, 'blocking_query')" class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-semibold text-xs transition cursor-pointer flex items-center space-x-1" title="Inspect full thread metrics and statement">
                    ${renderIcon('search', 'w-3 h-3 text-sky-400')}<span>Show Detail</span>
                  </button>
                  <button onclick="confirmKillProcess(${l.blocking_thread_id}, 'query')" class="px-3 py-1 rounded bg-amber-600 hover:bg-amber-500 text-slate-950 font-bold text-xs transition cursor-pointer">Kill Query</button>
                  <button onclick="confirmKillProcess(${l.blocking_thread_id}, 'connection')" class="px-3 py-1 rounded bg-red-600 hover:bg-red-500 text-white font-bold text-xs transition cursor-pointer">Kill Blocker</button>
                </div>
              </div>

              ${l.blocking_query ? `
                <div class="bg-slate-950 rounded-lg border border-slate-800 overflow-hidden">
                  <div class="bg-slate-900/90 px-3 py-1.5 border-b border-slate-800 flex items-center justify-between text-xs">
                    <span class="text-slate-400 font-semibold text-[11px] flex items-center space-x-1.5">
                      ${renderIcon('zap', 'w-3.5 h-3.5 text-amber-400 mr-1')}
                      <span>Blocking Query (Holding Lock):</span>
                    </span>
                    <div class="flex items-center space-x-2">
                      <button onclick="copyLockQueryFromData(this, ${idx}, 'blocking_query')" class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[11px] font-medium flex items-center space-x-1 transition cursor-pointer" title="Copy SQL statement to clipboard">
                        ${renderIcon('copy', 'w-3 h-3 text-slate-400')}<span>Copy SQL</span>
                      </button>
                      <button onclick="openLockProcessDetail(${l.blocking_thread_id}, ${idx}, 'blocking_query')" class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 text-sky-300 border border-sky-800 text-[11px] font-medium flex items-center space-x-1 transition cursor-pointer" title="Open in Process Detail Inspector">
                        ${renderIcon('search', 'w-3 h-3 text-sky-400')}<span>Detail</span>
                      </button>
                    </div>
                  </div>
                  <div class="p-3 text-[13px] leading-relaxed text-slate-200 font-mono whitespace-pre-wrap break-all max-h-60 overflow-y-auto select-text">${escapeHtml(l.blocking_query)}</div>
                </div>
              ` : `
                <div class="p-2.5 bg-slate-950/70 rounded-lg border border-slate-800 text-xs text-slate-400 italic flex items-center justify-between">
                  <span>-- Connection is idle in transaction (holding locks without active running query)</span>
                  <button onclick="openLockProcessDetail(${l.blocking_thread_id}, ${idx}, 'blocking_query')" class="text-sky-400 hover:text-sky-300 text-[11px] font-mono underline cursor-pointer">${renderIcon('search', 'w-3 h-3 text-sky-400')} Show Detail</button>
                </div>
              `}

              <div class="pl-4 border-l-2 border-red-700/80 text-xs text-red-200 space-y-2 pt-1">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="flex flex-wrap items-center gap-1.5 text-xs">
                    <span>↳ Blocked:</span>
                    <span class="font-bold text-white text-sm">Thread #${l.waiting_thread_id}</span>
                    <span class="text-slate-300">(${escapeHtml(l.waiting_user || 'unknown')})</span>
                    <span class="text-red-300 font-medium">waiting <strong class="text-white font-mono">${l.waiting_seconds}s</strong> for ${escapeHtml(l.lock_mode || 'lock')}</span>
                  </div>
                  <div class="flex items-center space-x-2">
                    <button onclick="openLockProcessDetail(${l.waiting_thread_id}, ${idx}, 'waiting_query')" class="px-2.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[11px] font-medium flex items-center space-x-1 transition cursor-pointer" title="Inspect waiting thread">
                      ${renderIcon('search', 'w-3 h-3 text-sky-400')}<span>Show Detail</span>
                    </button>
                    <button onclick="confirmKillProcess(${l.waiting_thread_id}, 'connection')" class="px-2.5 py-0.5 rounded bg-red-950 hover:bg-red-900 text-red-300 border border-red-800 text-[11px] font-medium transition cursor-pointer">
                      Kill Waiting #${l.waiting_thread_id}
                    </button>
                  </div>
                </div>

                ${l.waiting_query ? `
                  <div class="bg-slate-950 rounded-lg border border-slate-800 overflow-hidden">
                    <div class="bg-slate-900/70 px-2.5 py-1 border-b border-slate-800 flex items-center justify-between text-[11px]">
                      <span class="text-slate-400 font-semibold flex items-center space-x-1">
                        <span>⏳</span>
                        <span>Waiting Query:</span>
                      </span>
                      <div class="flex items-center space-x-1.5">
                        <button onclick="copyLockQueryFromData(this, ${idx}, 'waiting_query')" class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[10px] font-medium flex items-center space-x-1 transition cursor-pointer" title="Copy waiting SQL to clipboard">
                          ${renderIcon('copy', 'w-3 h-3 text-slate-400')}<span>Copy SQL</span>
                        </button>
                        <button onclick="openLockProcessDetail(${l.waiting_thread_id}, ${idx}, 'waiting_query')" class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 text-sky-300 border border-sky-800 text-[10px] font-medium flex items-center space-x-1 transition cursor-pointer" title="Inspect waiting query detail">
                          ${renderIcon('search', 'w-3 h-3 text-sky-400')}<span>Detail</span>
                        </button>
                      </div>
                    </div>
                    <div class="p-2.5 text-xs leading-relaxed text-slate-300 font-mono whitespace-pre-wrap break-all max-h-48 overflow-y-auto select-text">${escapeHtml(l.waiting_query)}</div>
                  </div>
                ` : `
                  <div class="p-2 bg-slate-950/60 rounded text-xs text-slate-500 font-mono italic">-- No waiting query text available</div>
                `}
              </div>
            </div>
          `;
        } else {
          return `
            <div class="p-3.5 bg-slate-950/60 border border-slate-800 rounded-lg flex flex-col space-y-2.5 text-xs">
              <div class="flex flex-wrap items-center justify-between gap-2.5">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-amber-400 font-bold text-sm">Thread #${l.waiting_thread_id}</span>
                  <span class="text-slate-400">waiting on</span>
                  <span class="text-red-400 font-bold text-sm">Thread #${l.blocking_thread_id}</span>
                  <span class="text-slate-400 font-mono">(${l.waiting_seconds}s)</span>
                  ${l.locked_resource ? `<span class="text-sky-300 font-mono px-2 py-0.5 bg-sky-950/50 rounded border border-sky-800/60">resource: ${escapeHtml(l.locked_resource)}</span>` : ''}
                </div>
                <div class="flex items-center space-x-2 shrink-0">
                  <button onclick="openLockProcessDetail(${l.blocking_thread_id}, ${idx}, 'blocking_query')" class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-xs transition cursor-pointer flex items-center space-x-1">
                    ${renderIcon('search', 'w-3 h-3 text-sky-400 mr-1')}<span>Blocker #${l.blocking_thread_id}</span>
                  </button>
                  <button onclick="openLockProcessDetail(${l.waiting_thread_id}, ${idx}, 'waiting_query')" class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-xs transition cursor-pointer flex items-center space-x-1">
                    ${renderIcon('search', 'w-3 h-3 text-amber-400 mr-1')}<span>Waiting #${l.waiting_thread_id}</span>
                  </button>
                  <button onclick="confirmKillProcess(${l.blocking_thread_id}, 'connection')" class="px-3 py-1 rounded bg-red-950 hover:bg-red-800 text-red-300 text-xs border border-red-800 transition whitespace-nowrap cursor-pointer shrink-0">Kill Target #${l.blocking_thread_id}</button>
                </div>
              </div>

              ${(l.waiting_query || l.blocking_query) ? `
                <div class="space-y-1.5 pt-1">
                  ${l.blocking_query ? `
                    <div class="bg-slate-950 rounded border border-slate-800 overflow-hidden">
                      <div class="bg-slate-900/60 px-2.5 py-1 border-b border-slate-800 flex items-center justify-between text-[11px]">
                        <span class="text-slate-400 font-semibold">Blocker Query (#${l.blocking_thread_id}):</span>
                        <div class="flex items-center space-x-1.5">
                          <button onclick="copyLockQueryFromData(this, ${idx}, 'blocking_query')" class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[10px] font-medium transition cursor-pointer">
                            ${renderIcon('copy', 'w-3 h-3 text-slate-400')}<span>Copy SQL</span>
                          </button>
                          <button onclick="openLockProcessDetail(${l.blocking_thread_id}, ${idx}, 'blocking_query')" class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 text-sky-300 border border-sky-800 text-[10px] font-medium transition cursor-pointer">
                            ${renderIcon('search', 'w-3 h-3 text-sky-400')}<span>Detail</span>
                          </button>
                        </div>
                      </div>
                      <div class="p-2 text-xs leading-relaxed text-slate-300 font-mono whitespace-pre-wrap break-all max-h-36 overflow-y-auto select-text">${escapeHtml(l.blocking_query)}</div>
                    </div>
                  ` : ''}
                  ${l.waiting_query ? `
                    <div class="bg-slate-950 rounded border border-slate-800 overflow-hidden">
                      <div class="bg-slate-900/60 px-2.5 py-1 border-b border-slate-800 flex items-center justify-between text-[11px]">
                        <span class="text-slate-400 font-semibold">Waiting Query (#${l.waiting_thread_id}):</span>
                        <div class="flex items-center space-x-1.5">
                          <button onclick="copyLockQueryFromData(this, ${idx}, 'waiting_query')" class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[10px] font-medium transition cursor-pointer">
                            ${renderIcon('copy', 'w-3 h-3 text-slate-400')}<span>Copy SQL</span>
                          </button>
                          <button onclick="openLockProcessDetail(${l.waiting_thread_id}, ${idx}, 'waiting_query')" class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 text-sky-300 border border-sky-800 text-[10px] font-medium transition cursor-pointer">
                            ${renderIcon('search', 'w-3 h-3 text-sky-400')}<span>Detail</span>
                          </button>
                        </div>
                      </div>
                      <div class="p-2 text-xs leading-relaxed text-slate-300 font-mono whitespace-pre-wrap break-all max-h-36 overflow-y-auto select-text">${escapeHtml(l.waiting_query)}</div>
                    </div>
                  ` : ''}
                </div>
              ` : ''}
            </div>
          `;
        }
      }).join('');
    }

    function showNoLockBanner() {
      // ponytail: unobtrusive toast replacement, no vertical layout shift
      showToast('✅ No active lock contention detected. All database threads running unobstructed.', 'success');
    }

    function dismissNoLockBanner() {
      // ponytail: backwards compatibility no-op
    }

    async function openLockInspector() {
      if (!activeConnection) {
        showNativeAlert('Please select and connect to a database session first.', 'Lock Inspector');
        return;
      }
      openLockInspectorModal();
      await fetchProcesslist(true);
      renderLockInspectorModalContent();
    }

    function setProcesslistInterval(interval) {
      processlistRefreshInterval = interval;
      ['pause', '1s', '3s', '5s'].forEach(opt => {
        const btn = document.getElementById(`pl-refresh-${opt}`);
        if (!btn) return;
        const isCur = (opt === 'pause' && interval === 'pause') ||
                      (opt === '1s' && interval === 1000) ||
                      (opt === '3s' && interval === 3000) ||
                      (opt === '5s' && interval === 5000);
        btn.className = isCur
          ? 'px-2 py-0.5 rounded text-[11px] bg-slate-800 text-white font-medium'
          : 'px-2 py-0.5 rounded text-[11px] text-slate-400 hover:text-white';
      });

      if (processlistTimer) {
        clearInterval(processlistTimer);
        processlistTimer = null;
      }
      if (interval !== 'pause') {
        triggerCountdownBar('pl-refresh-bar', interval);
        processlistTimer = setInterval(() => {
          const activeTab = tabs.find(t => t.id === activeTabId);
          if (activeTab && activeTab.isProcesslist && !document.hidden) {
            fetchProcesslist(true);
            triggerCountdownBar('pl-refresh-bar', interval);
          }
        }, interval);
      } else {
        triggerCountdownBar('pl-refresh-bar', 'pause');
      }
    }

    let isFetchingProcesslist = false;

    async function fetchProcesslist(silent = false) {
      if (!activeConnection) return;
      if (isFetchingProcesslist) return; // ponytail: prevent concurrent overlapping requests when interval is fast
      isFetchingProcesslist = true;
      if (!silent && processlistRefreshInterval !== 'pause') {
        setProcesslistInterval(processlistRefreshInterval);
      }
      if (!silent) {
        const tbody = document.getElementById('pl-tbody');
        if (tbody && (!processlistData.threads || processlistData.threads.length === 0)) {
          tbody.innerHTML = `<tr><td colspan="9" class="p-8 text-center text-slate-400"><div class="animate-pulse">Loading processlist...</div></td></tr>`;
        }
      }
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/processlist`, { silent });
        if (!res.ok) {
          if (!silent) {
            const err = await res.json();
            showNativeAlert('Failed to load processlist: ' + (err.error || res.statusText), 'Processlist Error');
          }
          return;
        }
        const data = await res.json();
        processlistData = data || { current_thread_id: 0, threads: [], locks: [] };
        renderProcesslistUI();
      } catch (err) {
        if (!silent) {
          showNativeAlert('Failed to load processlist: ' + err.message, 'Processlist Error');
        }
      } finally {
        isFetchingProcesslist = false;
      }
    }

    function renderProcesslistUI() {
      const threads = processlistData.threads || [];
      const locks = processlistData.locks || [];
      const currentThreadId = processlistData.current_thread_id || 0;

      // ponytail: record active concurrency into ephemeral ring buffer (max 60 ticks)
      recordProcesslistTrendTick(threads);
      if (plTrendPanelOpen) {
        renderProcesslistTrendChart();
      }

      const thresholdEl = document.getElementById('pl-slow-threshold');
      const minSeconds = thresholdEl ? (parseInt(thresholdEl.value, 10) || 0) : 0;
      const searchEl = document.getElementById('pl-search-input');
      const search = searchEl ? searchEl.value.trim().toLowerCase() : '';
      const clearBtn = document.getElementById('pl-search-clear-btn');
      if (clearBtn && searchEl) {
        if (searchEl.value.length > 0) {
          clearBtn.classList.remove('hidden');
        } else {
          clearBtn.classList.add('hidden');
        }
      }

      // Count slow threads (> 5 sec) and active lock contention
      const slowCount = threads.filter(t => t.time >= 5).length;
      const rootBlockerCount = locks.filter(l => l.is_root_blocker).length;
      const lockWaitCount = locks.length;

      const badgeHeader = document.getElementById('processlist-slow-badge');
      const lockBadge = document.getElementById('lock-blocker-badge');

      if (badgeHeader) {
        if (slowCount > 0) {
          badgeHeader.textContent = slowCount;
          badgeHeader.classList.remove('hidden');
        } else {
          badgeHeader.classList.add('hidden');
        }
      }

      if (lockBadge) {
        if (lockWaitCount > 0) {
          lockBadge.textContent = rootBlockerCount > 0 ? `${rootBlockerCount} Blocker${rootBlockerCount > 1 ? 's' : ''}` : `${lockWaitCount} Locks`;
          lockBadge.classList.remove('hidden');
        } else {
          lockBadge.classList.add('hidden');
        }
      }

      // ponytail: zero-shift monitor button alert dot & tooltip update (preserves navbar layout width completely)
      const monitorAlertDot = document.getElementById('monitor-alert-dot');
      const monitorBtn = document.getElementById('top-monitor-btn');
      if (monitorAlertDot) {
        if (lockWaitCount > 0) {
          monitorAlertDot.className = "absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-red-500 border border-slate-900 animate-pulse pointer-events-none";
          monitorAlertDot.classList.remove('hidden');
          if (monitorBtn) monitorBtn.title = `⚡ Monitor - 🚨 ${rootBlockerCount > 0 ? `${rootBlockerCount} Root Blocker(s)` : `${lockWaitCount} Lock Wait(s)`} active`;
        } else if (slowCount > 0) {
          monitorAlertDot.className = "absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-amber-400 border border-slate-900 pointer-events-none";
          monitorAlertDot.classList.remove('hidden');
          if (monitorBtn) monitorBtn.title = `⚡ Monitor - ⚠️ ${slowCount} Slow Query thread(s) running (>5s)`;
        } else {
          monitorAlertDot.classList.add('hidden');
          if (monitorBtn) monitorBtn.title = "Database Monitor & Diagnostics";
        }
      }

      // Update tab title if processlist tab is present
      const plTab = tabs.find(t => t.isProcesslist);
      if (plTab) {
        plTab.title = slowCount > 0 ? `Processlist (${slowCount})` : 'Processlist';
        const plTabEl = document.querySelector(`[onclick*="switchTab('${plTab.id}')"] .truncate`);
        if (plTabEl) plTabEl.textContent = plTab.title;
      }

      // Summary text
      const summaryEl = document.getElementById('pl-summary-text');
      if (summaryEl) {
        summaryEl.textContent = `Threads: ${threads.length}${slowCount > 0 ? ` (${slowCount} slow)` : ''}`;
      }

      // ponytail: zero-shift lock indicator in Processlist toolbar
      const lockSummary = document.getElementById('pl-lock-summary');
      const lockCountEl = document.getElementById('pl-lock-count');

      if (locks.length > 0) {
        if (lockSummary) lockSummary.classList.remove('hidden');
        if (lockCountEl) {
          lockCountEl.textContent = rootBlockerCount > 0 ? `${rootBlockerCount} Blocker${rootBlockerCount > 1 ? 's' : ''}` : `${locks.length} Locks`;
        }
      } else {
        if (lockSummary) lockSummary.classList.add('hidden');
      }

      // Keep open lock inspector dialog in sync if active
      const lockDialog = document.getElementById('lock-inspector-dialog');
      if (lockDialog && lockDialog.open) {
        renderLockInspectorModalContent();
      }

      if (!_plFiltersInitialized) {
        initProcesslistFilters();
      }

      const hideSleep = document.getElementById('pl-hide-sleep')?.checked ?? false;
      const hideSystem = document.getElementById('pl-hide-system')?.checked ?? false;

      // Filter threads
      const filtered = threads.filter(t => {
        if (hideSleep && (t.command === 'Sleep' || t.state === 'Sleep')) return false;
        if (hideSystem && (t.user === 'system user' || t.user === 'event_scheduler' || t.command === 'Binlog Dump')) return false;
        if (minSeconds > 0 && t.time < minSeconds) return false;
        if (search) {
          const matchUser = (t.user || '').toLowerCase().includes(search);
          const matchHost = (t.host || '').toLowerCase().includes(search);
          const matchDb = (t.db || '').toLowerCase().includes(search);
          const matchInfo = (t.info || '').toLowerCase().includes(search);
          const matchState = (t.state || '').toLowerCase().includes(search);
          const matchCmd = (t.command || '').toLowerCase().includes(search);
          const matchId = String(t.id).includes(search);
          if (!matchUser && !matchHost && !matchDb && !matchInfo && !matchState && !matchCmd && !matchId) return false;
        }

        // Quick filter chips (ADR 0052)
        if (plActiveChip === 'active' && (t.command === 'Sleep' || t.state === 'Sleep')) return false;
        if (plActiveChip === 'locked') {
          const isLocked = locks.some(l => l.waiting_thread_id === t.id || l.blocking_thread_id === t.id);
          if (!isLocked) return false;
        }
        if (plActiveChip === 'slow' && t.time < 5) return false;
        if (plActiveChip === 'sleep' && !((t.command === 'Sleep' || t.state === 'Sleep') && t.time >= 60)) return false;

        return true;
      });

      // ponytail: prune disappeared threads from selection set across polling ticks
      const currentIds = new Set(threads.map(t => t.id));
      for (const id of plSelectedThreadIds) {
        if (!currentIds.has(id)) {
          plSelectedThreadIds.delete(id);
        }
      }
      updateProcesslistBatchBarUI();

      const tbody = document.getElementById('pl-tbody');
      if (!tbody) return;

      const visibleColCount = PL_COLUMNS.filter(c => isProcesslistColVisible(c.key)).length;
      if (filtered.length === 0) {
        tbody.innerHTML = `<tr><td colspan="${visibleColCount}" class="p-8 text-center text-slate-500 italic">No threads matched the current filter.</td></tr>`;
        updateProcesslistSelectAllHeader();
        return;
      }

      tbody.innerHTML = filtered.map((t, rIdx) => {
        const isRootBlocker = locks.some(l => l.is_root_blocker && l.blocking_thread_id === t.id);
        const isWaitingOnLock = locks.some(l => l.waiting_thread_id === t.id);
        const isSelf = t.id === currentThreadId;
        const isSystem = (t.user === 'system user' || t.user === 'event_scheduler' || t.command === 'Binlog Dump');
        const isCritical = t.time >= 30;
        const isWarning = t.time >= 5 && !isCritical;
        const hasQuery = t.info && t.command !== 'Sleep' && t.info.trim() !== '';
        const rawSql = t.info || (t.command === 'Sleep' ? 'Idle connection (Sleep)' : '-');
        const displaySql = rawSql.replace(/[\r\n\t]+/g, ' ').trim();
        const isImmune = isSelf || isSystem;
        const isSelected = plSelectedThreadIds.has(t.id);

        let rowClass = 'hover:bg-slate-800/40 transition cursor-pointer select-none';
        if (isRootBlocker) {
          rowClass += ' bg-red-950/40 text-red-100 border-l-4 border-red-500 font-semibold';
        } else if (isWaitingOnLock) {
          rowClass += ' bg-amber-950/20 text-amber-200 border-l-2 border-amber-600';
        } else if (isCritical) {
          rowClass += ' bg-red-950/20 text-red-200 border-l-2 border-red-500';
        } else if (isWarning) {
          rowClass += ' bg-amber-950/20 text-amber-200 border-l-2 border-amber-500';
        }

        return `
          <tr class="${rowClass}" onclick="openProcessDetailModal(${t.id})">
            <td data-pl-col="rownum" class="p-2 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none ${isProcesslistColVisible('rownum') ? '' : 'hidden'}">${rIdx + 1}</td>
            <td data-pl-col="select" class="p-2 text-center whitespace-nowrap ${isProcesslistColVisible('select') ? '' : 'hidden'}" onclick="event.stopPropagation()">
              ${isImmune ? `
                <input type="checkbox" disabled class="opacity-20 cursor-not-allowed w-3.5 h-3.5" title="${isSelf ? 'Cannot target own active connection' : 'Cannot target protected system thread'}">
              ` : `
                <input type="checkbox" data-pl-tid="${t.id}" ${isSelected ? 'checked' : ''} onchange="onProcesslistThreadCheckboxChange(${t.id}, this.checked)" class="accent-sky-500 w-3.5 h-3.5 cursor-pointer">
              `}
            </td>
            <td data-pl-col="action" class="p-2 text-center whitespace-nowrap ${isProcesslistColVisible('action') ? '' : 'hidden'}" onclick="event.stopPropagation()">
              <div class="inline-flex items-center space-x-1">
                <button type="button" onclick="openProcessDetailModal(${t.id})" class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-800 text-sky-300 text-[10px] font-semibold border border-sky-800 transition flex items-center space-x-1 cursor-pointer" title="Inspect full thread details &amp; SQL">
                  ${renderIcon('eye', 'w-3.5 h-3.5 mr-1')}<span>View</span>
                </button>
                ${hasQuery ? `
                  <button type="button" onclick="confirmKillProcess(${t.id}, 'query')" ${isSystem ? 'disabled' : ''} class="px-1.5 py-0.5 rounded bg-amber-950 hover:bg-amber-800 text-amber-300 text-[10px] font-semibold border border-amber-800 disabled:opacity-30 disabled:cursor-not-allowed transition cursor-pointer" title="${isSystem ? 'Cannot kill query on protected system thread' : 'Kill currently executing query only (KILL QUERY)'}">
                    Kill Q
                  </button>
                ` : `
                  <button type="button" disabled class="px-1.5 py-0.5 rounded bg-slate-900/50 text-slate-600 border border-slate-800/80 text-[10px] font-semibold opacity-35 cursor-not-allowed transition" title="No active query running on this thread">
                    Kill Q
                  </button>
                `}
                <button type="button" onclick="confirmKillProcess(${t.id}, 'connection')" ${isSelf || isSystem ? 'disabled' : ''} class="px-1.5 py-0.5 rounded bg-red-950 hover:bg-red-800 text-red-300 text-[10px] font-semibold border border-red-800 disabled:opacity-30 disabled:cursor-not-allowed transition cursor-pointer" title="${isSelf ? 'Cannot kill own active session connection' : 'Terminate client connection (KILL)'}">
                  Kill C
                </button>
                <button type="button" data-pl-menu-btn onclick="showProcessContextMenu(event, this, ${t.id})" class="px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white text-xs border border-slate-700 transition cursor-pointer" title="More thread actions">
                  ⋮
                </button>
              </div>
            </td>
            <td data-pl-col="id" class="p-2 font-bold whitespace-nowrap ${isProcesslistColVisible('id') ? '' : 'hidden'}">
              <span>${t.id}</span>
              ${isRootBlocker ? '<span class="ml-1 px-1.5 py-0.5 rounded bg-red-600 text-white font-bold text-[9px] animate-pulse">🚨 Blocker</span>' : ''}
              ${isWaitingOnLock && !isRootBlocker ? '<span class="ml-1 px-1.5 py-0.5 rounded bg-amber-950 text-amber-300 text-[9px] border border-amber-800">🔒 Waiting</span>' : ''}
              ${isSelf ? '<span class="ml-1 px-1 py-0.2 rounded bg-sky-950 text-sky-400 text-[9px] border border-sky-800">You</span>' : ''}
              ${isSystem ? '<span class="ml-1 px-1 py-0.2 rounded bg-slate-800 text-slate-400 text-[9px]">Sys</span>' : ''}
            </td>
            <td data-pl-col="user" class="p-2 truncate max-w-[100px] whitespace-nowrap ${isProcesslistColVisible('user') ? '' : 'hidden'}" title="${escapeHtml(t.user)}">${escapeHtml(t.user)}</td>
            <td data-pl-col="host" class="p-2 text-slate-400 truncate max-w-[130px] whitespace-nowrap ${isProcesslistColVisible('host') ? '' : 'hidden'}" title="${escapeHtml(t.host)}">${escapeHtml(t.host)}</td>
            <td data-pl-col="db" class="p-2 text-sky-300 truncate max-w-[100px] whitespace-nowrap ${isProcesslistColVisible('db') ? '' : 'hidden'}" title="${escapeHtml(t.db || '-')}">${escapeHtml(t.db || '-')}</td>
            <td data-pl-col="command" class="p-2 text-slate-300 whitespace-nowrap ${isProcesslistColVisible('command') ? '' : 'hidden'}">${escapeHtml(t.command)}</td>
            <td data-pl-col="time" class="p-2 text-right font-bold whitespace-nowrap ${isProcesslistColVisible('time') ? '' : 'hidden'} ${isCritical ? 'text-red-400' : (isWarning ? 'text-amber-400' : 'text-slate-300')}">${t.time}s</td>
            <td data-pl-col="state" class="p-2 text-slate-400 text-[11px] truncate max-w-[140px] whitespace-nowrap ${isProcesslistColVisible('state') ? '' : 'hidden'}" title="${escapeHtml(t.state || '-')}">${escapeHtml(t.state || '-')}</td>
            <td data-pl-col="query" class="p-2 font-mono text-[11px] select-text cursor-pointer max-w-0 w-full ${isProcesslistColVisible('query') ? '' : 'hidden'} ${hasQuery ? 'text-slate-200' : 'text-slate-500 italic'}" title="${escapeHtml(rawSql)}">
              <div class="truncate w-full">${escapeHtml(displaySql)}</div>
            </td>
          </tr>
        `;
      }).join('');

      updateProcesslistSelectAllHeader();
    }

    // --- Process Activity Trend Panel (Ephemeral in-memory rolling buffer) ---
    // ponytail: rolling in-memory buffer limited to 60 data points, zero backend persistence
    function recordProcesslistTrendTick(threads) {
      if (!Array.isArray(threads)) return;
      // Active executing queries (Command != 'Sleep')
      const running = threads.filter(t => t.command && t.command !== 'Sleep').length;
      const total = threads.length;
      plTrendHistory.push({ time: Date.now(), running, total });
      if (plTrendHistory.length > 60) {
        plTrendHistory.shift();
      }
    }

    function toggleProcesslistTrendPanel(forceState, silent) {
      const panel = document.getElementById('pl-trend-panel');
      const btn = document.getElementById('pl-toggle-trend-btn');
      const caret = document.getElementById('pl-trend-caret');
      if (!panel) return;

      if (typeof forceState === 'boolean') {
        plTrendPanelOpen = forceState;
      } else {
        plTrendPanelOpen = !plTrendPanelOpen;
      }

      if (plTrendPanelOpen) {
        panel.classList.remove('hidden');
        if (caret) caret.textContent = '▴';
        if (btn) {
          btn.classList.add('bg-sky-950', 'text-sky-300', 'border-sky-700');
          btn.classList.remove('bg-slate-800', 'text-slate-300');
        }
        if (!silent) localStorage.setItem('sqlfox_pl_trend_open', 'true');
        requestAnimationFrame(() => renderProcesslistTrendChart());
      } else {
        panel.classList.add('hidden');
        if (caret) caret.textContent = '▾';
        if (btn) {
          btn.classList.remove('bg-sky-950', 'text-sky-300', 'border-sky-700');
          btn.classList.add('bg-slate-800', 'text-slate-300');
        }
        if (!silent) localStorage.setItem('sqlfox_pl_trend_open', 'false');
      }
    }

    function escalateToFlightRecorder() {
      openFlightRecorderTab();
    }

    function renderProcesslistTrendChart() {
      if (!plTrendPanelOpen) return;
      const canvas = document.getElementById('pl-trend-canvas');
      if (!canvas) return;

      const rect = canvas.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) return;

      const dpr = window.devicePixelRatio || 1;
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.scale(dpr, dpr);

      const w = rect.width;
      const h = rect.height;

      ctx.clearRect(0, 0, w, h);

      const count = plTrendHistory.length;
      let latestRunning = 0;
      let latestTotal = 0;
      let peakRunning = 0;
      let sumRunning = 0;

      if (count > 0) {
        const last = plTrendHistory[count - 1];
        latestRunning = last.running;
        latestTotal = last.total;
        plTrendHistory.forEach(pt => {
          if (pt.running > peakRunning) peakRunning = pt.running;
          sumRunning += pt.running;
        });
      }

      const avgRunning = count > 0 ? (sumRunning / count).toFixed(1) : '0.0';

      const nowEl = document.getElementById('pl-trend-now');
      const totalEl = document.getElementById('pl-trend-total');
      const peakEl = document.getElementById('pl-trend-peak');
      const avgEl = document.getElementById('pl-trend-avg');
      if (nowEl) nowEl.textContent = latestRunning;
      if (totalEl) totalEl.textContent = latestTotal;
      if (peakEl) peakEl.textContent = peakRunning;
      if (avgEl) avgEl.textContent = avgRunning;

      if (count === 0) {
        ctx.fillStyle = '#64748B';
        ctx.font = '11px "JetBrains Mono", ui-monospace, SFMono-Regular, monospace';
        ctx.textAlign = 'center';
        ctx.fillText('Collecting activity ticks... Waiting for processlist polling.', w / 2, h / 2);
        return;
      }

      const padLeft = 32;
      const padRight = 12;
      const padTop = 10;
      const padBottom = 16;
      const chartW = w - padLeft - padRight;
      const chartH = h - padTop - padBottom;

      let maxVal = Math.max(4, peakRunning);
      plTrendHistory.forEach(pt => {
        if (pt.total > maxVal) maxVal = pt.total;
      });
      maxVal = Math.ceil(maxVal * 1.15);

      // Grid lines
      ctx.strokeStyle = 'rgba(51, 65, 85, 0.35)';
      ctx.lineWidth = 1;
      ctx.fillStyle = '#64748B';
      ctx.font = '9px "JetBrains Mono", ui-monospace, SFMono-Regular, monospace';
      ctx.textAlign = 'right';

      const steps = 3;
      for (let i = 0; i <= steps; i++) {
        const val = Math.round((maxVal / steps) * (steps - i));
        const y = padTop + (chartH / steps) * i;
        ctx.beginPath();
        ctx.moveTo(padLeft, y);
        ctx.lineTo(w - padRight, y);
        ctx.stroke();
        ctx.fillText(String(val), padLeft - 6, y + 3);
      }

      const maxSlots = 59;
      const getX = (idx) => {
        if (count <= 1) return padLeft + chartW;
        return padLeft + (chartW / maxSlots) * (maxSlots - (count - 1 - idx));
      };
      const getY = (val) => padTop + chartH - (val / maxVal) * chartH;

      // Draw secondary line: Total Threads (Slate-500, dashed)
      ctx.beginPath();
      ctx.strokeStyle = 'rgba(100, 116, 139, 0.55)';
      ctx.setLineDash([3, 3]);
      ctx.lineWidth = 1.2;
      for (let i = 0; i < count; i++) {
        const x = getX(i);
        const y = getY(plTrendHistory[i].total);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      ctx.stroke();
      ctx.setLineDash([]);

      // Draw gradient fill for Running queries
      const grad = ctx.createLinearGradient(0, padTop, 0, padTop + chartH);
      grad.addColorStop(0, 'rgba(56, 189, 248, 0.25)');
      grad.addColorStop(1, 'rgba(56, 189, 248, 0.01)');

      ctx.beginPath();
      for (let i = 0; i < count; i++) {
        const x = getX(i);
        const y = getY(plTrendHistory[i].running);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      const lastX = getX(count - 1);
      const firstX = getX(0);
      ctx.lineTo(lastX, padTop + chartH);
      ctx.lineTo(firstX, padTop + chartH);
      ctx.closePath();
      ctx.fillStyle = grad;
      ctx.fill();

      // Draw primary line: Running Queries (Sky-400)
      ctx.beginPath();
      ctx.strokeStyle = '#38bdf8';
      ctx.lineWidth = 1.8;
      for (let i = 0; i < count; i++) {
        const x = getX(i);
        const y = getY(plTrendHistory[i].running);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      ctx.stroke();

      // Draw glowing end dot on latest point
      if (count > 0) {
        const endX = getX(count - 1);
        const endY = getY(plTrendHistory[count - 1].running);
        ctx.beginPath();
        ctx.arc(endX, endY, 3, 0, Math.PI * 2);
        ctx.fillStyle = '#38bdf8';
        ctx.fill();
        ctx.strokeStyle = '#0284c7';
        ctx.lineWidth = 1;
        ctx.stroke();
      }
    }

    let selectedProcessThread = null;

    function openProcessDetailModal(threadId, fallbackThread) {
      try {
        closeProcessContextMenu();
        let thread = (processlistData.threads || []).find(t => String(t.id) === String(threadId));
        if (!thread && fallbackThread) {
          thread = {
            id: Number(threadId) || threadId,
            user: fallbackThread.user || 'unknown',
            host: fallbackThread.host || '-',
            db: fallbackThread.db || (activeConnection ? activeConnection.database : '-'),
            command: fallbackThread.command || (fallbackThread.info ? 'Query' : 'Sleep'),
            time: fallbackThread.time || 0,
            state: fallbackThread.state || (fallbackThread.is_root_blocker ? 'Holding Locks' : 'Waiting for Lock'),
            info: fallbackThread.info || fallbackThread.query || ''
          };
        }
        if (!thread) {
          showToast(`Process thread #${threadId} is not available in processlist.`, 'info');
          return;
        }
        selectedProcessThread = thread;

        const isSelf = thread.id === (processlistData.current_thread_id || 0);
        const isSystem = (thread.user === 'system user' || thread.user === 'event_scheduler' || thread.command === 'Binlog Dump');
        const hasQuery = thread.info && thread.command !== 'Sleep' && thread.info.trim() !== '';

        const modal = document.getElementById('pl-detail-modal');
        if (!modal) return;

        const titleEl = document.getElementById('pl-modal-thread-id');
        if (titleEl) {
          titleEl.textContent = '#' + thread.id + (isSelf ? ' (You)' : (isSystem ? ' (Sys)' : ''));
        }
        const userEl = document.getElementById('pl-modal-user');
        if (userEl) userEl.textContent = thread.user || '-';
        const hostEl = document.getElementById('pl-modal-host');
        if (hostEl) hostEl.textContent = thread.host || '-';
        const dbEl = document.getElementById('pl-modal-db');
        if (dbEl) dbEl.textContent = thread.db || '-';
        const cmdEl = document.getElementById('pl-modal-command');
        if (cmdEl) cmdEl.textContent = thread.command || '-';
        const stateEl = document.getElementById('pl-modal-state');
        if (stateEl) stateEl.textContent = thread.state || '-';

        const timeEl = document.getElementById('pl-modal-time');
        if (timeEl) {
          timeEl.textContent = `${thread.time}s`;
          if (thread.time >= 30) {
            timeEl.className = 'font-mono font-bold block text-red-400';
          } else if (thread.time >= 5) {
            timeEl.className = 'font-mono font-bold block text-amber-400';
          } else {
            timeEl.className = 'font-mono font-bold block text-slate-200';
          }
        }

        // Check lock involvement
        const locks = processlistData.locks || [];
        const rootBlockerLock = locks.find(l => l.is_root_blocker && l.blocking_thread_id === thread.id);
        const waitingLock = locks.find(l => l.waiting_thread_id === thread.id);
        const lockInfoEl = document.getElementById('pl-modal-lock-info');
        if (lockInfoEl) {
          if (rootBlockerLock) {
            lockInfoEl.classList.remove('hidden');
            lockInfoEl.innerHTML = `
              <div class="font-bold flex items-center space-x-1.5 text-red-300">
                ${renderIcon('alert-triangle', 'w-3.5 h-3.5 text-red-400 mr-1')}<span>ROOT BLOCKER</span>
                <span>This thread is holding locks blocking other transactions!</span>
              </div>
              <div class="text-[11px] text-red-200">
                Holding ${escapeHtml(rootBlockerLock.lock_mode || 'LOCK')} on ${escapeHtml(rootBlockerLock.locked_resource || 'resource')}.
              </div>
            `;
          } else if (waitingLock) {
            lockInfoEl.classList.remove('hidden');
            lockInfoEl.innerHTML = `
              <div class="font-bold flex items-center space-x-1.5 text-amber-300">
                ${renderIcon('clock', 'w-3.5 h-3.5 text-amber-400 mr-1')}<span>BLOCKED WAITING</span>
                <span>Waiting for Thread #${waitingLock.blocking_thread_id} (${waitingLock.waiting_seconds}s)</span>
              </div>
              <div class="text-[11px] text-amber-200">
                Waiting for lock mode: ${escapeHtml(waitingLock.lock_mode || 'lock')}
              </div>
            `;
          } else {
            lockInfoEl.classList.add('hidden');
          }
        }

        // Full SQL Code display (default raw SQL)
        const sqlCodeEl = document.getElementById('pl-modal-sql-code');
        const btnFormat = document.getElementById('pl-modal-format-sql-btn');
        if (sqlCodeEl) {
          if (hasQuery) {
            sqlCodeEl.textContent = thread.info;
            isModalSQLFormatted = false;
            if (btnFormat) {
              btnFormat.disabled = false;
              btnFormat.innerHTML = `<svg class="w-3.5 h-3.5 text-amber-300 shrink-0 inline mr-1" aria-hidden="true"><use href="#icon-sparkles"/></svg><span>Format SQL</span>`;
              btnFormat.classList.remove('bg-indigo-900', 'text-indigo-200', 'border-indigo-700');
            }
          } else {
            sqlCodeEl.textContent = (thread.command === 'Sleep' ? '-- Connection is idle (Sleep)\n-- No query currently executing' : '-- No query details provided');
            isModalSQLFormatted = false;
            if (btnFormat) {
              btnFormat.disabled = true;
              btnFormat.innerHTML = `<svg class="w-3.5 h-3.5 text-amber-300 shrink-0 inline mr-1" aria-hidden="true"><use href="#icon-sparkles"/></svg><span>Format SQL</span>`;
              btnFormat.classList.remove('bg-indigo-900', 'text-indigo-200', 'border-indigo-700');
            }
          }
        }

        // Footer buttons state
        const btnExplain = document.getElementById('pl-modal-btn-explain');
        const btnEditor = document.getElementById('pl-modal-btn-editor');
        const btnAI = document.getElementById('pl-modal-btn-ai');
        const btnNotes = document.getElementById('pl-modal-btn-notes');
        const btnKillQ = document.getElementById('pl-modal-btn-kill-q');
        const btnKillC = document.getElementById('pl-modal-btn-kill-c');

        if (btnExplain) btnExplain.disabled = !hasQuery;
        if (btnEditor) btnEditor.disabled = !hasQuery;
        if (btnAI) btnAI.disabled = !hasQuery;
        if (btnNotes) btnNotes.disabled = !hasQuery;
        if (btnKillQ) btnKillQ.disabled = !hasQuery || isSystem;
        if (btnKillC) btnKillC.disabled = isSelf || isSystem;

        if (typeof modal.showModal === 'function') {
          if (!modal.open) {
            modal.showModal();
          }
        } else {
          modal.setAttribute('open', '');
        }
      } catch (err) {
        console.error('[SqlFox] openProcessDetailModal error:', err);
      }
    }

    function closeProcessDetailModal() {
      const modal = document.getElementById('pl-detail-modal');
      if (modal) {
        if (typeof modal.close === 'function') {
          modal.close();
        } else {
          modal.removeAttribute('open');
        }
      }
    }

    function showProcessContextMenu(event, btnEl, threadId) {
      if (event) {
        event.stopPropagation();
        event.preventDefault();
      }
      const menu = document.getElementById('pl-context-menu');
      if (!menu) return;

      // If already open for this thread, toggle close
      if (!menu.classList.contains('hidden') && selectedProcessThread && String(selectedProcessThread.id) === String(threadId)) {
        closeProcessContextMenu();
        return;
      }
      closeAllContextMenus();

      const thread = (processlistData.threads || []).find(t => String(t.id) === String(threadId));
      if (!thread) {
        console.warn('[SqlFox] Process thread not found for menu:', threadId);
        return;
      }
      selectedProcessThread = thread;

      const isSelf = thread.id === (processlistData.current_thread_id || 0);
      const isSystem = (thread.user === 'system user' || thread.user === 'event_scheduler' || thread.command === 'Binlog Dump');
      const hasQuery = thread.info && thread.command !== 'Sleep' && thread.info.trim() !== '';

      const btnExplain = document.getElementById('pl-menu-explain');
      const btnEditor = document.getElementById('pl-menu-editor');
      const btnAI = document.getElementById('pl-menu-ai');
      const btnNotes = document.getElementById('pl-menu-notes');
      const btnCopySQL = document.getElementById('pl-menu-copy-sql');
      const btnKillQ = document.getElementById('pl-menu-kill-query');
      const btnKillC = document.getElementById('pl-menu-kill-conn');

      if (btnExplain) btnExplain.classList.toggle('hidden', !hasQuery);
      if (btnEditor) btnEditor.classList.toggle('hidden', !hasQuery);
      if (btnAI) btnAI.classList.toggle('hidden', !hasQuery);
      if (btnNotes) btnNotes.classList.toggle('hidden', !hasQuery);
      if (btnCopySQL) btnCopySQL.classList.toggle('hidden', !hasQuery);

      if (btnKillQ) {
        btnKillQ.classList.toggle('hidden', !hasQuery);
        btnKillQ.disabled = isSystem;
      }
      if (btnKillC) {
        btnKillC.disabled = isSelf || isSystem;
      }

      const anchor = btnEl || (event && event.currentTarget) || (event && event.target);
      if (anchor && anchor.getBoundingClientRect) {
        const rect = anchor.getBoundingClientRect();
        let top = rect.bottom + 4;
        let left = Math.max(10, Math.min(window.innerWidth - 220, rect.left));
        if (top + 280 > window.innerHeight) {
          top = Math.max(10, rect.top - 280);
        }
        menu.style.top = `${top}px`;
        menu.style.left = `${left}px`;
      } else if (event) {
        menu.style.top = `${event.clientY + 4}px`;
        menu.style.left = `${Math.max(10, Math.min(window.innerWidth - 220, event.clientX))}px`;
      }
      menu.style.display = 'block';
      menu.style.zIndex = '9999';
      menu.classList.remove('hidden');
    }

    function closeProcessContextMenu() {
      const menu = document.getElementById('pl-context-menu');
      if (menu) {
        menu.classList.add('hidden');
        menu.style.display = 'none';
      }
    }

    function inspectProcessFromMenu() {
      closeProcessContextMenu();
      if (selectedProcessThread) openProcessDetailModal(selectedProcessThread.id);
    }

    function explainProcessFromMenu() {
      closeProcessContextMenu();
      if (selectedProcessThread?.info) explainSpecificQuery(selectedProcessThread.info, selectedProcessThread.db);
    }

    function sendProcessToEditorFromMenu() {
      closeProcessContextMenu();
      if (selectedProcessThread?.info) sendProcessQueryToEditor(selectedProcessThread.info);
    }

    function sendProcessToAIFromMenu() {
      closeProcessContextMenu();
      if (selectedProcessThread?.info) sendProcessQueryToAI(selectedProcessThread.info, selectedProcessThread.db);
    }

    function saveProcessToNotesFromMenu() {
      closeProcessContextMenu();
      if (selectedProcessThread?.info) saveProcessQueryToNotes(selectedProcessThread.info, selectedProcessThread.db);
    }

    async function copyProcessSQLFromMenu() {
      closeProcessContextMenu();
      if (!selectedProcessThread?.info) return;
      const ok = await copyTextToClipboard(selectedProcessThread.info);
      if (ok) {
        showNativeAlert('SQL statement copied to clipboard.', 'Copied');
      } else {
        showNativeAlert('Copy failed in this browser context.', 'Copy Error');
      }
    }

    async function copyProcessJSONFromMenu() {
      closeProcessContextMenu();
      if (!selectedProcessThread) return;
      const ok = await copyTextToClipboard(JSON.stringify(selectedProcessThread, null, 2));
      if (ok) {
        showNativeAlert('Thread details JSON copied to clipboard.', 'Copied');
      } else {
        showNativeAlert('Copy failed in this browser context.', 'Copy Error');
      }
    }

    function killProcessQueryFromMenu() {
      closeProcessContextMenu();
      if (selectedProcessThread) confirmKillProcess(selectedProcessThread.id, 'query');
    }

    function killProcessConnFromMenu() {
      closeProcessContextMenu();
      if (selectedProcessThread) confirmKillProcess(selectedProcessThread.id, 'connection');
    }

    let isModalSQLFormatted = false;

    function formatModalProcessSQL() {
      if (!selectedProcessThread || !selectedProcessThread.info || !selectedProcessThread.info.trim()) {
        showNativeAlert('No SQL statement executing on this thread.', 'Format SQL');
        return;
      }
      const sqlCodeEl = document.getElementById('pl-modal-sql-code');
      const btn = document.getElementById('pl-modal-format-sql-btn');
      if (!sqlCodeEl) return;

      if (!isModalSQLFormatted) {
        sqlCodeEl.textContent = formatSQL(selectedProcessThread.info);
        isModalSQLFormatted = true;
        if (btn) {
          btn.innerHTML = '<span>↺ Raw SQL</span>';
          btn.classList.add('bg-indigo-900', 'text-indigo-200', 'border-indigo-700');
        }
      } else {
        sqlCodeEl.textContent = selectedProcessThread.info;
        isModalSQLFormatted = false;
        if (btn) {
          btn.innerHTML = '<svg class="w-3.5 h-3.5 inline mr-1 text-sky-400 shrink-0"><use href="#icon-sparkles"/></svg><span>Format SQL</span>';
          btn.classList.remove('bg-indigo-900', 'text-indigo-200', 'border-indigo-700');
        }
      }
    }

    async function copyModalProcessSQL() {
      const sqlCodeEl = document.getElementById('pl-modal-sql-code');
      const textToCopy = (sqlCodeEl && sqlCodeEl.textContent && !sqlCodeEl.textContent.startsWith('-- '))
        ? sqlCodeEl.textContent
        : (selectedProcessThread ? selectedProcessThread.info : '');
      if (!textToCopy) {
        showNativeAlert('No SQL statement executing on this thread.', 'Copy SQL');
        return;
      }
      const ok = await copyTextToClipboard(textToCopy);
      if (ok) {
        const btn = document.getElementById('pl-modal-copy-sql-btn');
        if (btn) {
          const orig = btn.innerHTML;
          btn.innerHTML = `${renderIcon('check', 'w-3.5 h-3.5 inline mr-1 text-emerald-400')}<span>Copied!</span>`;
          btn.classList.add('bg-emerald-800', 'text-white');
          setTimeout(() => {
            btn.innerHTML = orig;
            btn.classList.remove('bg-emerald-800', 'text-white');
          }, 1500);
        }
      } else {
        showNativeAlert('Failed to copy SQL in this browser context.', 'Copy Error');
      }
    }

    async function copyModalProcessJSON() {
      if (!selectedProcessThread) return;
      const ok = await copyTextToClipboard(JSON.stringify(selectedProcessThread, null, 2));
      if (ok) {
        const btn = document.getElementById('pl-modal-copy-json-btn');
        if (btn) {
          const orig = btn.innerHTML;
          btn.innerHTML = `${renderIcon('check', 'w-3.5 h-3.5 inline mr-1 text-emerald-400')}<span>Copied!</span>`;
          btn.classList.add('bg-emerald-800', 'text-white');
          setTimeout(() => {
            btn.innerHTML = orig;
            btn.classList.remove('bg-emerald-800', 'text-white');
          }, 1500);
        }
      } else {
        showNativeAlert('Failed to copy JSON in this browser context.', 'Copy Error');
      }
    }

    function explainProcessFromModal() {
      if (!selectedProcessThread?.info) return;
      closeProcessDetailModal();
      explainSpecificQuery(selectedProcessThread.info, selectedProcessThread.db);
    }

    function sendProcessToEditorFromModal() {
      if (!selectedProcessThread?.info) return;
      closeProcessDetailModal();
      sendProcessQueryToEditor(selectedProcessThread.info);
    }

    function sendProcessToAIFromModal() {
      if (!selectedProcessThread?.info) return;
      closeProcessDetailModal();
      sendProcessQueryToAI(selectedProcessThread.info, selectedProcessThread.db);
    }

    function saveProcessToNotesFromModal() {
      if (!selectedProcessThread?.info) return;
      closeProcessDetailModal();
      saveProcessQueryToNotes(selectedProcessThread.info, selectedProcessThread.db);
    }

    function killProcessQueryFromModal() {
      if (!selectedProcessThread) return;
      closeProcessDetailModal();
      confirmKillProcess(selectedProcessThread.id, 'query');
    }

    function killProcessConnFromModal() {
      if (!selectedProcessThread) return;
      closeProcessDetailModal();
      confirmKillProcess(selectedProcessThread.id, 'connection');
    }

    async function confirmKillProcess(threadId, mode) {
      const modeLabel = mode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY';
      const ok = await showNativeConfirm(`Are you sure you want to execute ${modeLabel} for thread #${threadId}?`, `Confirm ${modeLabel}`);
      if (!ok) return;
      await doKillProcess(threadId, mode);
    }

    async function doKillProcess(threadId, mode) {
      if (!activeConnection) return;
      const modeLabel = mode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY';
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/kill`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ thread_id: threadId, mode: mode })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert('Kill action failed: ' + (data.error || res.statusText), 'Kill Error');
          return;
        }
        showToast(`✓ ${modeLabel} sent for thread #${threadId}`, 'success');
        // ponytail: Initial immediate refresh followed by deferred refresh to capture async DB engine cleanup
        await fetchProcesslist(true);
        setTimeout(() => {
          fetchProcesslist(true);
        }, 750);
      } catch (err) {
        showNativeAlert('Kill action failed: ' + err.message, 'Kill Error');
      }
    }

    async function explainSpecificQuery(sql, dbName) {
      if (!activeConnection || !sql) return;
      const currentDb = dbName || schemaCatalogCache?.database_name || activeConnection.database_name || '';
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/explain`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ sql, database: currentDb })
        });
        const data = await res.json();
        if (!res.ok) {
          showNativeAlert(data.error || 'Failed to explain query plan', 'Explain Error');
          return;
        }
        openImpactModal(data, sql);
      } catch (err) {
        showNativeAlert('Explain request failed: ' + err.message, 'Network Error');
      }
    }

    function sendProcessQueryToEditor(sql) {
      if (!sql) return;
      addNewQueryTab();
      setEditorSQL(sql);
      showNativeAlert('Query loaded into a new Query Editor tab.', 'Opened in Editor');
    }

    async function sendProcessQueryToAI(sql, dbName) {
      if (!sql) return;
      const aiSidebar = document.getElementById('ai-sidebar');
      if (aiSidebar && aiSidebar.classList.contains('hidden')) {
        toggleAISidebar();
      }
      const promptInput = document.getElementById('ai-prompt-input');
      if (!promptInput) return;

      const promptTemplate = `Please analyze this query from the Processlist and provide optimization suggestions and indexes:\n\n\`\`\`sql\n${sql}\n\`\`\``;

      // If AI input already has draft text, confirm before overwriting
      if (promptInput.value && promptInput.value.trim().length > 0) {
        const confirmReplace = await showNativeConfirm(
          'AI Copilot input already contains your draft text.\n\nDo you want to overwrite it with this Processlist query analysis?\n\n(Choose OK to Overwrite, or Cancel to keep your draft)',
          'Confirm AI Input'
        );
        if (!confirmReplace) return;
      }

      promptInput.value = promptTemplate;
      promptInput.focus();
      promptInput.setSelectionRange(promptInput.value.length, promptInput.value.length);
      showNativeAlert('Query has been inserted into AI Copilot input. Please review and click Send.', 'AI Draft Ready');
    }

    function saveProcessQueryToNotes(sql, dbName) {
      if (!sql) return;
      const title = 'Slow Query (Processlist' + (dbName ? ` - ${dbName}` : '') + ')';
      const objTags = [];
      if (activeConnection && activeConnection.id) {
        objTags.push(`db://${activeConnection.id}/${dbName || ''}`);
      }
      saveToInternalNotes(title, '```sql\n' + sql + '\n```', objTags, ['#processlist', '#slow-query'], 'Captured thread query from Processlist Monitor for latency investigation.');
    }

    // --- Cluster & Replication Inspector ---
    let clusterData = null;
    let clusterTimer = null;
    let clusterRefreshInterval = 'pause';

    function openClusterTab() {
      if (!activeConnection) {
        showNativeAlert('Please select and connect to a database session first.', 'Cluster Inspector');
        return;
      }
      let tab = tabs.find(t => t.isCluster);
      if (!tab) {
        tab = {
          id: 'tab-cluster',
          title: 'Cluster Health',
          isCluster: true,
        };
        tabs.push(tab);
      }
      switchTab(tab.id);
      fetchClusterHealth(false);
    }

    function setClusterInterval(interval) {
      clusterRefreshInterval = interval;
      ['pause', '1s', '3s', '5s'].forEach(opt => {
        const btn = document.getElementById(`cl-refresh-${opt}`);
        if (!btn) return;
        const isCur = (opt === 'pause' && interval === 'pause') ||
                      (opt === '1s' && interval === 1000) ||
                      (opt === '3s' && interval === 3000) ||
                      (opt === '5s' && interval === 5000);
        btn.className = isCur
          ? 'px-2 py-0.5 rounded text-[11px] bg-slate-800 text-white font-medium'
          : 'px-2 py-0.5 rounded text-[11px] text-slate-400 hover:text-white';
      });

      if (clusterTimer) {
        clearInterval(clusterTimer);
        clusterTimer = null;
      }
      if (interval !== 'pause') {
        triggerCountdownBar('cl-refresh-bar', interval);
        clusterTimer = setInterval(() => {
          const activeTab = tabs.find(t => t.id === activeTabId);
          if (activeTab && activeTab.isCluster && !document.hidden) {
            fetchClusterHealth(true);
            triggerCountdownBar('cl-refresh-bar', interval);
          }
        }, interval);
      } else {
        triggerCountdownBar('cl-refresh-bar', 'pause');
      }
    }

    async function fetchClusterHealth(silent = false) {
      if (!activeConnection) return;
      if (!silent && clusterRefreshInterval !== 'pause') {
        setClusterInterval(clusterRefreshInterval);
      }
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/cluster`, { silent });
        if (!res.ok) {
          if (!silent) {
            const err = await res.json().catch(() => ({}));
            showNativeAlert('Failed to load cluster diagnostics: ' + (err.error || res.statusText), 'Cluster Error');
          }
          return;
        }
        const data = await res.json();
        clusterData = data;
        renderClusterUI();
        const updatedEl = document.getElementById('cl-last-updated');
        if (updatedEl) {
          updatedEl.textContent = 'Updated: ' + new Date().toLocaleTimeString();
        }
      } catch (err) {
        if (!silent) {
          showNativeAlert('Failed to load cluster diagnostics: ' + err.message, 'Cluster Error');
        }
      }
    }

    function renderClusterUI() {
      if (!clusterData) return;
      const d = clusterData;

      // 1. Diagnosis Card & Icon
      const diagCard = document.getElementById('cl-diagnosis-card');
      const diagIcon = document.getElementById('cl-diagnosis-icon');
      const diagSev = document.getElementById('cl-diagnosis-severity');
      const diagText = document.getElementById('cl-diagnosis-text');
      const statusBadge = document.getElementById('cluster-status-badge');

      const isCritical = d.bottleneck_severity === 'critical';
      const isWarning = d.bottleneck_severity === 'warning';

      if (diagCard) {
        if (isCritical) {
          diagCard.className = 'p-3.5 rounded-lg border flex items-start justify-between space-x-3 transition bg-red-950/40 border-red-800 text-red-200';
          if (diagIcon) diagIcon.innerHTML = renderIcon('alert-triangle', 'w-5 h-5 text-red-400');
          if (diagSev) {
            diagSev.textContent = 'CRITICAL BOTTLENECK';
            diagSev.className = 'px-1.5 py-0.2 rounded text-[10px] uppercase font-mono bg-red-900 text-red-100 border border-red-700';
          }
        } else if (isWarning) {
          diagCard.className = 'p-3.5 rounded-lg border flex items-start justify-between space-x-3 transition bg-amber-950/40 border-amber-800 text-amber-200';
          if (diagIcon) diagIcon.innerHTML = renderIcon('alert-circle', 'w-5 h-5 text-amber-400');
          if (diagSev) {
            diagSev.textContent = 'PERFORMANCE WARNING';
            diagSev.className = 'px-1.5 py-0.2 rounded text-[10px] uppercase font-mono bg-amber-900 text-amber-100 border border-amber-700';
          }
        } else {
          diagCard.className = 'p-3.5 rounded-lg border flex items-start justify-between space-x-3 transition bg-emerald-950/30 border-emerald-800 text-emerald-200';
          if (diagIcon) diagIcon.innerHTML = renderIcon('check-circle', 'w-5 h-5 text-emerald-400');
          if (diagSev) {
            diagSev.textContent = 'HEALTHY';
            diagSev.className = 'px-1.5 py-0.2 rounded text-[10px] uppercase font-mono bg-emerald-900 text-emerald-100 border border-emerald-700';
          }
        }
        if (diagText) {
          diagText.textContent = d.bottleneck_diagnosis || 'Cluster operates normally.';
        }
      }

      if (statusBadge) {
        if (isCritical) {
          statusBadge.classList.remove('hidden');
          statusBadge.textContent = 'CRITICAL';
          statusBadge.className = 'bg-red-500/20 text-red-300 text-[10px] px-1 rounded-full font-bold';
        } else if (isWarning) {
          statusBadge.classList.remove('hidden');
          statusBadge.textContent = 'WARN';
          statusBadge.className = 'bg-amber-500/20 text-amber-300 text-[10px] px-1 rounded-full font-bold';
        } else {
          statusBadge.classList.remove('hidden');
          statusBadge.textContent = d.topology_type === 'galera' ? 'PXC' : (d.topology_type === 'replication' ? 'REPL' : 'OK');
          statusBadge.className = 'bg-emerald-500/20 text-emerald-300 text-[10px] px-1 rounded-full font-bold';
        }
      }

      // 2. Summary Cards
      const cardTopo = document.getElementById('cl-card-topology');
      const cardQuorum = document.getElementById('cl-card-quorum');
      if (cardTopo) cardTopo.textContent = d.topology_type ? d.topology_type.toUpperCase() : 'UNKNOWN';
      if (cardQuorum) cardQuorum.textContent = `Status: ${d.cluster_status || 'Primary'} ${d.cluster_name ? `(${d.cluster_name})` : ''}`;

      const cardFC = document.getElementById('cl-card-fc');
      const cardFCDetail = document.getElementById('cl-card-fc-detail');
      const fcPct = (d.flow_control_paused * 100).toFixed(2);
      if (cardFC) {
        cardFC.textContent = `${fcPct}%`;
        if (d.flow_control_paused > 0.05) cardFC.className = 'text-base font-bold text-red-400 mt-1';
        else if (d.flow_control_paused > 0.005) cardFC.className = 'text-base font-bold text-amber-400 mt-1';
        else cardFC.className = 'text-base font-bold text-emerald-400 mt-1';
      }
      if (cardFCDetail) cardFCDetail.textContent = `Sent: ${d.flow_control_sent || 0} | Recv: ${d.flow_control_recv || 0}`;

      const cardCulprit = document.getElementById('cl-card-culprit');
      const cardCulpritDetail = document.getElementById('cl-card-culprit-detail');
      if (cardCulprit) {
        if (d.slowest_node) {
          cardCulprit.textContent = d.slowest_node;
          cardCulprit.className = 'text-base font-bold text-red-400 mt-1 truncate';
          if (cardCulpritDetail) cardCulpritDetail.textContent = 'Triggered Flow Control / High Queue';
        } else {
          cardCulprit.textContent = 'None (Balanced)';
          cardCulprit.className = 'text-base font-bold text-emerald-400 mt-1';
          if (cardCulpritDetail) cardCulpritDetail.textContent = 'No node throttling cluster';
        }
      }

      const cardNodes = document.getElementById('cl-card-nodes-count');
      const cardLocalState = document.getElementById('cl-card-local-state');
      if (cardNodes) cardNodes.textContent = `${d.cluster_size || (d.nodes ? d.nodes.length : 1)} Member(s)`;
      if (cardLocalState) cardLocalState.textContent = `Local: ${d.local_node_state || 'Active'} (${d.local_node_name || 'Node'})`;

      // 3. Tri-Metric Diagnostics
      const tri = d.tri_metric || {};
      const netAvgEl = document.getElementById('cl-tri-net-avg');
      const netRangeEl = document.getElementById('cl-tri-net-range');
      const netBadgeEl = document.getElementById('cl-tri-net-badge');
      if (netAvgEl) netAvgEl.textContent = (tri.network_latency_avg_ms || 0).toFixed(2);
      if (netRangeEl) netRangeEl.textContent = `Min: ${(tri.network_latency_min_ms || 0).toFixed(2)}ms | Peak: ${(tri.network_latency_max_ms || 0).toFixed(2)}ms`;
      if (netBadgeEl) {
        if ((tri.network_latency_avg_ms || 0) > 5.0 || (tri.network_latency_max_ms || 0) > 50.0) {
          netBadgeEl.textContent = 'High Latency';
          netBadgeEl.className = 'px-1.5 py-0.2 rounded text-[10px] font-mono bg-amber-950 text-amber-300 border border-amber-800';
        } else {
          netBadgeEl.textContent = 'Healthy';
          netBadgeEl.className = 'px-1.5 py-0.2 rounded text-[10px] font-mono bg-emerald-950 text-emerald-300 border border-emerald-800';
        }
      }

      const diskWaitEl = document.getElementById('cl-tri-disk-wait');
      const diskDetailEl = document.getElementById('cl-tri-disk-detail');
      const diskBadgeEl = document.getElementById('cl-tri-disk-badge');
      if (diskWaitEl) diskWaitEl.textContent = tri.buffer_pool_wait_free || 0;
      if (diskDetailEl) diskDetailEl.textContent = `Redo Log Waits: ${tri.log_waits || 0} | Fsyncs: ${tri.os_log_fsyncs || 0}`;
      if (diskBadgeEl) {
        if ((tri.buffer_pool_wait_free || 0) > 0 || (tri.log_waits || 0) > 0) {
          diskBadgeEl.textContent = 'Disk I/O Stall';
          diskBadgeEl.className = 'px-1.5 py-0.2 rounded text-[10px] font-mono bg-red-950 text-red-300 border border-red-800';
        } else {
          diskBadgeEl.textContent = 'No Stalls';
          diskBadgeEl.className = 'px-1.5 py-0.2 rounded text-[10px] font-mono bg-emerald-950 text-emerald-300 border border-emerald-800';
        }
      }

      const threadsEl = document.getElementById('cl-tri-threads');
      const conflictDetailEl = document.getElementById('cl-tri-conflict-detail');
      const workerBadgeEl = document.getElementById('cl-tri-worker-badge');
      if (threadsEl) threadsEl.textContent = tri.slave_threads || 1;
      if (conflictDetailEl) conflictDetailEl.textContent = `Cert Failures: ${tri.cert_failures || 0} | BF Aborts: ${tri.bf_aborts || 0}`;
      if (workerBadgeEl) {
        if ((tri.cert_failures || 0) > 50 || (tri.bf_aborts || 0) > 50) {
          workerBadgeEl.textContent = 'Lock Contention';
          workerBadgeEl.className = 'px-1.5 py-0.2 rounded text-[10px] font-mono bg-amber-950 text-amber-300 border border-amber-800';
        } else {
          workerBadgeEl.textContent = 'Normal';
          workerBadgeEl.className = 'px-1.5 py-0.2 rounded text-[10px] font-mono bg-emerald-950 text-emerald-300 border border-emerald-800';
        }
      }

      // 4. Peer Node Cards Grid
      const grid = document.getElementById('cl-nodes-grid');
      if (!grid) return;
      const nodes = d.nodes || [];

      if (nodes.length === 0) {
        grid.innerHTML = '<div class="col-span-full text-center text-slate-500 py-6 italic text-xs">No cluster members detected.</div>';
        return;
      }

      grid.innerHTML = nodes.map(n => {
        const isCulprit = n.is_culprit;
        const isUnreachable = n.status === 'unreachable';
        const isSynced = (n.state || '').toLowerCase() === 'synced';

        let borderClass = 'border-slate-800 bg-slate-900/60';
        if (isCulprit) borderClass = 'border-red-600/80 bg-red-950/30 ring-1 ring-red-500/50';
        else if (isUnreachable) borderClass = 'border-slate-800/60 bg-slate-900/30 opacity-75';

        return `
          <div class="border rounded-lg p-3.5 space-y-2.5 transition ${borderClass}">
            <div class="flex items-start justify-between">
              <div>
                <div class="font-bold text-white text-xs flex items-center space-x-1.5">
                  <span class="truncate max-w-[160px]">${escapeHtml(n.name || n.address)}</span>
                  ${n.is_local ? '<span class="px-1 py-0.2 rounded bg-sky-950 text-sky-300 text-[9px] border border-sky-800">Local Session</span>' : ''}
                  ${isCulprit ? '<span class="px-1.5 py-0.2 rounded bg-red-900 text-red-100 text-[9px] font-bold uppercase animate-pulse">🚨 Slow Node</span>' : ''}
                </div>
                <div class="text-[11px] font-mono text-slate-400 mt-0.5">${escapeHtml(n.address)}</div>
              </div>
              <div>
                <span class="px-1.5 py-0.5 rounded text-[10px] font-mono uppercase font-bold ${isUnreachable ? 'bg-slate-800 text-slate-400' : (isSynced ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' : 'bg-amber-950 text-amber-300 border border-amber-800')}">
                  ${escapeHtml(n.state || (isUnreachable ? 'Restricted' : 'Active'))}
                </span>
              </div>
            </div>

            ${isUnreachable ? `
              <div class="p-2 rounded bg-slate-950/60 text-[11px] text-slate-400 font-mono flex items-center space-x-1.5">
                ${renderIcon('lock', 'w-3 h-3 text-slate-400')}
                <span>Unreachable directly (VPC / Firewall restricted)</span>
              </div>
            ` : `
              <div class="grid grid-cols-2 gap-2 text-[11px] font-mono bg-slate-950/50 p-2 rounded">
                <div>
                  <span class="text-slate-400">Recv Queue:</span>
                  <span class="font-bold ml-1 ${n.receive_queue > 20 ? 'text-amber-400' : 'text-slate-200'}">${n.receive_queue || 0}</span>
                </div>
                <div>
                  <span class="text-slate-400">FC Sent:</span>
                  <span class="font-bold ml-1 ${n.flow_control_sent > 0 ? 'text-red-400' : 'text-slate-200'}">${n.flow_control_sent || 0}</span>
                </div>
              </div>
            `}

            <div class="flex items-center justify-end space-x-1.5 pt-1 border-t border-slate-800/80">
              ${n.is_local ? `
                <button onclick="openProcesslistTab()" class="px-2 py-1 rounded bg-sky-950 hover:bg-sky-900 text-sky-300 text-[10px] font-medium border border-sky-800 transition flex items-center space-x-1">
                  ${renderIcon('zap', 'w-3 h-3 text-sky-400 mr-1')}<span>Inspect Processlist</span>
                </button>
              ` : `
                <button onclick="switchSessionToNode(${jsArg(n.address)})" class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] font-medium border border-slate-700 transition flex items-center space-x-1" title="Switch active session connection to this node">
                  ${renderIcon('link', 'w-3 h-3 text-slate-400 mr-1')}<span>Switch Session</span>
                </button>
              `}
            </div>
          </div>
        `;
      }).join('');

      // 5. Cluster Parameter Tuning Guardrails
      const tuningListEl = document.getElementById('cl-tuning-checks-list');
      const tuningCardEl = document.getElementById('cl-tuning-guardrails-card');
      const checks = d.tuning_checks || [];
      if (tuningListEl) {
        if (checks.length === 0) {
          if (tuningCardEl) tuningCardEl.classList.add('hidden');
        } else {
          if (tuningCardEl) tuningCardEl.classList.remove('hidden');
          tuningListEl.innerHTML = checks.map(c => {
            const isOpt = c.status === 'optimal';
            const isCrit = c.status === 'critical';
            const badgeBg = isCrit ? 'bg-red-950 text-red-200 border-red-800' : (isOpt ? 'bg-emerald-950 text-emerald-200 border-emerald-800' : 'bg-amber-950 text-amber-200 border-amber-800');
            const dotClass = isCrit ? 'bg-red-400' : (isOpt ? 'bg-emerald-400' : 'bg-amber-400');
            return `
              <div class="border border-slate-800/80 bg-slate-900/60 rounded p-3 text-xs flex flex-col md:flex-row md:items-start justify-between gap-3">
                <div class="space-y-1 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono font-bold text-white text-[12px]">${escapeHtml(c.variable)}</span>
                    <span class="px-1.5 py-0.2 rounded text-[10px] font-mono border font-semibold inline-flex items-center ${badgeBg}"><span class="w-1.5 h-1.5 rounded-full ${dotClass} inline-block mr-1"></span>${escapeHtml(c.status.toUpperCase())}</span>
                    <span class="text-slate-400 font-mono text-[11px]">Current: <strong class="text-white">${escapeHtml(c.current_value)}</strong></span>
                    <span class="text-slate-500 font-mono text-[11px]">Recommended: <strong class="text-emerald-400">${escapeHtml(c.recommended)}</strong></span>
                  </div>
                  <div class="text-slate-300 text-[11px]">${escapeHtml(c.summary)}</div>
                  ${c.trade_off ? `<div class="text-slate-400 text-[10px] italic flex items-center gap-1">${renderIcon('scale', 'w-3 h-3 text-slate-400')}<span>${escapeHtml(c.trade_off)}</span></div>` : ''}
                </div>
                <div class="shrink-0 flex items-center space-x-2 pt-1 md:pt-0">
                  ${c.remediation ? `
                    <button data-snippet="${escapeHtml(c.remediation)}" data-msg="Remediation snippet copied!" onclick="copySnippetText(this.dataset.snippet, this.dataset.msg)" class="px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded border border-slate-700 text-[11px] font-mono flex items-center space-x-1" title="Copy remediation fix">
                      ${renderIcon('copy', 'w-3 h-3 text-slate-300 mr-1')}<span>Copy Fix</span>
                    </button>
                  ` : ''}
                  <button data-var="${escapeHtml(c.variable)}" data-val="${escapeHtml(c.current_value)}" data-summary="${escapeHtml(c.summary)}" onclick="askCopilotAboutVariable(this.dataset.var, this.dataset.val, this.dataset.summary)" class="px-2 py-1 bg-indigo-600/30 hover:bg-indigo-600/50 text-indigo-300 border border-indigo-500/40 rounded text-[11px] flex items-center space-x-1" title="Ask AI Copilot for advice">
                    ${renderIcon('sparkles', 'w-3 h-3 text-indigo-300 mr-1')}<span>Ask Copilot</span>
                  </button>
                </div>
              </div>
            `;
          }).join('');
        }
      }
    }

    function askCopilotClusterTuning() {
      if (!clusterData) {
        showNativeAlert('No cluster data available yet. Please refresh first.', 'Copilot Tuning');
        return;
      }
      const d = clusterData;
      const tri = d.tri_metric || {};
      toggleAISidebar(true);
      const promptInput = document.getElementById('ai-prompt-input');
      if (!promptInput) return;

      const promptTemplate = `Please analyze this database cluster health and provide performance tuning recommendations and bottleneck solutions:

- **Topology**: ${d.topology_type} (${d.cluster_name || 'unnamed'})
- **Quorum / Status**: ${d.cluster_status} (Size: ${d.cluster_size} nodes)
- **Local State**: ${d.local_node_state} (${d.local_node_name})
- **Primary Diagnosis**: ${d.bottleneck_diagnosis || 'Normal'} (Severity: ${d.bottleneck_severity})
- **Slowest / Culprit Node**: ${d.slowest_node || 'None (Balanced)'}
- **Flow Control Paused**: ${(d.flow_control_paused * 100).toFixed(2)}% (Sent: ${d.flow_control_sent}, Recv: ${d.flow_control_recv})
- **Network EVS Latency**: avg ${tri.network_latency_avg_ms?.toFixed(2) || 0}ms, max ${tri.network_latency_max_ms?.toFixed(2) || 0}ms
- **InnoDB Disk Stalls**: ${tri.buffer_pool_wait_free || 0} buffer pool wait free, ${tri.log_waits || 0} log flush waits
- **Concurrency**: ${tri.slave_threads || 1} wsrep_slave_threads, ${tri.cert_failures || 0} cert failures, ${tri.bf_aborts || 0} bf aborts

What are concrete steps to resolve this bottleneck? Is my.cnf tuning required (such as wsrep_slave_threads, innodb_buffer_pool_size), write query optimization, or hardware/network inspection?`;

      promptInput.value = promptTemplate;
      promptInput.focus();
      promptInput.setSelectionRange(promptInput.value.length, promptInput.value.length);
      showNativeAlert('Cluster diagnostic summary inserted into Copilot. Click Send for tuning recommendations.', 'Copilot Ready');
    }

    function switchSessionToNode(addr) {
      if (!addr) return;
      const parts = addr.split(':');
      const host = parts[0];
      const port = parts[1] ? parseInt(parts[1], 10) : 3306;

      openNewConnModal();
      setTimeout(() => {
        const hostInput = document.getElementById('modal-conn-host');
        const portInput = document.getElementById('modal-conn-port');
        const nameInput = document.getElementById('modal-conn-name');
        if (hostInput) hostInput.value = host;
        if (portInput) portInput.value = port;
        if (nameInput) nameInput.value = `${activeConnection ? activeConnection.name : 'Cluster'} (${host})`;
        showNativeAlert(`Connection modal auto-filled with node host ${host}:${port}. Enter password if required and click Connect.`, 'Switch Node Session');
      }, 100);
    }

    // --- Server Variable Inspector & Tuning Advisor ---
    let serverVariablesData = null;
    let serverVariablesCategory = 'all';
    let serverVariablesExpanded = {};

    function openServerVariablesTab() {
      if (!activeConnection) {
        showNativeAlert('Please select and connect to a database session first.', 'Server Variables');
        return;
      }
      let tab = tabs.find(t => t.isVariables);
      if (!tab) {
        tab = {
          id: 'tab-server-variables',
          title: 'Server Variables',
          isVariables: true,
        };
        tabs.push(tab);
      }
      switchTab(tab.id);
      fetchServerVariables();
    }

    async function fetchServerVariables() {
      if (!activeConnection) return;
      try {
        const tbody = document.getElementById('server-variables-tbody');
        if (tbody) {
          tbody.innerHTML = '<tr><td colspan="6" class="p-6 text-center text-slate-400 italic">Introspecting server configuration variables...</td></tr>';
        }
        const res = await apiFetch(`/api/session/${activeConnection.id}/diagnostics/variables`);
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          showNativeAlert('Failed to introspect server variables: ' + (err.error || res.statusText), 'Variables Error');
          return;
        }
        const data = await res.json();
        serverVariablesData = data;
        filterServerVariables();
      } catch (err) {
        showNativeAlert('Failed to load server variables: ' + err.message, 'Variables Error');
      }
    }

    function setVariableCategory(cat) {
      serverVariablesCategory = cat;
      ['all', 'curated', 'cluster', 'memory', 'concurrency', 'io', 'optimizer'].forEach(c => {
        const btn = document.getElementById(`var-cat-${c}`);
        if (!btn) return;
        if (c === cat) {
          btn.className = 'px-2 py-0.5 rounded text-[11px] bg-slate-800 text-white font-medium';
        } else {
          btn.className = 'px-2 py-0.5 rounded text-[11px] text-slate-400 hover:text-white';
        }
      });
      filterServerVariables();
    }

    function toggleVariableExpand(varName) {
      serverVariablesExpanded[varName] = !serverVariablesExpanded[varName];
      filterServerVariables();
    }

    function filterServerVariables() {
      if (!serverVariablesData) return;
      const tbody = document.getElementById('server-variables-tbody');
      const countEl = document.getElementById('var-stats-count');
      if (!tbody) return;

      const searchInput = document.getElementById('var-search-input');
      const query = (searchInput ? searchInput.value : '').toLowerCase().trim();

      const all = serverVariablesData.variables || [];
      const filtered = all.filter(v => {
        // Category filter
        if (serverVariablesCategory === 'curated' && !v.is_curated) return false;
        if (serverVariablesCategory === 'cluster' && !v.category.includes('Cluster') && !v.category.includes('WSREP')) return false;
        if (serverVariablesCategory === 'memory' && !v.category.includes('Memory') && !v.category.includes('Buffer')) return false;
        if (serverVariablesCategory === 'concurrency' && !v.category.includes('Concurrency') && !v.category.includes('Lock')) return false;
        if (serverVariablesCategory === 'io' && !v.category.includes('IO') && !v.category.includes('Durability')) return false;
        if (serverVariablesCategory === 'optimizer' && !v.category.includes('Optimizer') && !v.category.includes('Performance')) return false;

        // Search query
        if (query) {
          const matchName = v.name.toLowerCase().includes(query);
          const matchVal = (v.value || '').toLowerCase().includes(query);
          const matchDesc = (v.description || '').toLowerCase().includes(query);
          if (!matchName && !matchVal && !matchDesc) return false;
        }
        return true;
      });

      if (countEl) {
        countEl.textContent = `Showing: ${filtered.length} / ${serverVariablesData.total_count} (${serverVariablesData.curated_count} curated)`;
      }

      if (filtered.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="p-6 text-center text-slate-500 italic">No matching server variables found.</td></tr>';
        return;
      }

      tbody.innerHTML = filtered.map((v, idx) => {
        const isExpanded = !!serverVariablesExpanded[v.name];
        const isOpt = v.severity === 'optimal';
        const isWarn = v.severity === 'warning';
        const isCrit = v.severity === 'critical';

        let badgeHtml = '';
        if (v.is_curated) {
          if (isCrit) {
            badgeHtml = '<span class="px-1.5 py-0.5 rounded text-[10px] font-mono border font-semibold bg-red-950 text-red-200 border-red-800">🔴 Critical</span>';
          } else if (isWarn) {
            badgeHtml = '<span class="px-1.5 py-0.5 rounded text-[10px] font-mono border font-semibold bg-amber-950 text-amber-200 border-amber-800">🟡 Warning</span>';
          } else if (isOpt) {
            badgeHtml = '<span class="px-1.5 py-0.5 rounded text-[10px] font-mono border font-semibold bg-emerald-950 text-emerald-200 border-emerald-800">🟢 Optimal</span>';
          }
        }

        return `
          <tr class="hover:bg-slate-900/60 transition ${v.is_curated ? 'bg-slate-950/40' : ''}">
            <td class="py-2 px-3 w-10 text-right pr-2 text-slate-500 font-mono text-xs border-r border-slate-800/40 select-none">${idx + 1}</td>
            <td class="py-2 px-3 text-center">
              ${v.is_curated ? `
                <button onclick="toggleVariableExpand(${jsArg(v.name)})" class="text-slate-400 hover:text-white font-mono font-bold text-xs" title="Expand tuning details">
                  ${isExpanded ? '▼' : '▶'}
                </button>
              ` : '<span class="text-slate-700">•</span>'}
            </td>
            <td class="py-2 px-3 font-mono text-white flex items-center space-x-1.5">
              <span>${escapeHtml(v.name)}</span>
              ${v.is_curated ? `<span class="text-amber-400 text-[10px] inline-flex items-center" title="Curated Tuning Rule">${renderIcon('star', 'w-3 h-3 text-amber-400 shrink-0')}</span>` : ''}
              ${v.is_dynamic ? '<span class="text-[9px] px-1 py-0.2 rounded bg-sky-950 text-sky-400 border border-sky-800" title="Can be modified live via SET GLOBAL">Dynamic</span>' : ''}
            </td>
            <td class="py-2 px-3 font-mono font-semibold text-slate-200 max-w-[200px] truncate" title="${escapeHtml(v.value)}">
              ${escapeHtml(v.value)}
            </td>
            <td class="py-2 px-3 text-slate-400 text-[11px]">
              <span class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800">${escapeHtml(v.category)}</span>
            </td>
            <td class="py-2 px-3 text-slate-300">
              <div class="flex items-center space-x-2">
                ${badgeHtml}
                <span class="text-[11px] truncate max-w-xs text-slate-400" title="${escapeHtml(v.recommendation || v.description)}">
                  ${escapeHtml(v.recommendation || v.description || '-')}
                </span>
              </div>
            </td>
            <td class="py-2 px-3 text-right">
              <div class="flex items-center justify-end space-x-1">
                ${v.set_global_snippet ? `
                  <button data-snippet="${escapeHtml(v.set_global_snippet)}" data-msg="SET GLOBAL snippet copied!" onclick="copySnippetText(this.dataset.snippet, this.dataset.msg)" class="p-1 hover:bg-slate-800 text-slate-400 hover:text-slate-200 rounded flex items-center justify-center" title="Copy SET GLOBAL / SQL command">
                    ${renderIcon('copy', 'w-3.5 h-3.5 text-slate-400')}
                  </button>
                ` : ''}
                <button data-var="${escapeHtml(v.name)}" data-val="${escapeHtml(v.value)}" data-desc="${escapeHtml(v.description || '')}" onclick="askCopilotAboutVariable(this.dataset.var, this.dataset.val, this.dataset.desc)" class="px-1.5 py-0.5 bg-indigo-600/30 hover:bg-indigo-600/50 text-indigo-300 border border-indigo-500/40 rounded text-[10px] flex items-center space-x-1" title="Ask AI Copilot for deep analysis">
                  ${renderIcon('sparkles', 'w-3 h-3 text-indigo-300 mr-1')}<span>Ask AI</span>
                </button>
              </div>
            </td>
          </tr>
          ${isExpanded ? `
            <tr class="bg-slate-950/80">
              <td colspan="6" class="p-4 border-t border-b border-slate-800">
                <div class="space-y-3 max-w-4xl text-xs">
                  <div>
                    <div class="font-bold text-white text-[12px] flex items-center space-x-2">
                      ${renderIcon('book-open', 'w-3.5 h-3.5 text-sky-400 mr-1.5')}<span>Variable Purpose &amp; Function</span>
                      <span class="text-slate-400 font-normal font-mono text-[11px]">(${escapeHtml(v.scope || 'Global')})</span>
                    </div>
                    <div class="text-slate-300 mt-1 leading-relaxed">${escapeHtml(v.description || 'No description available.')}</div>
                  </div>

                  <div class="grid grid-cols-1 md:grid-cols-2 gap-3 pt-1">
                    <div class="p-3 rounded bg-amber-950/20 border border-amber-800/40 text-amber-200 space-y-1">
                      <div class="font-semibold text-xs flex items-center space-x-1">
                        ${renderIcon('chevron-down', 'w-3.5 h-3.5 text-amber-400 mr-1')}
                        <span>Impact If Too Low</span>
                      </div>
                      <div class="text-[11px] text-amber-300/90 leading-relaxed">${escapeHtml(v.impact_low || '-')}</div>
                    </div>
                    <div class="p-3 rounded bg-rose-950/20 border border-rose-800/40 text-rose-200 space-y-1">
                      <div class="font-semibold text-xs flex items-center space-x-1">
                        ${renderIcon('chevron-down', 'w-3.5 h-3.5 text-rose-400 rotate-180 mr-1')}
                        <span>Impact If Too High</span>
                      </div>
                      <div class="text-[11px] text-rose-300/90 leading-relaxed">${escapeHtml(v.impact_high || '-')}</div>
                    </div>
                  </div>

                  <div class="p-3 rounded bg-emerald-950/20 border border-emerald-800/40 text-emerald-200 space-y-1">
                    <div class="font-semibold text-xs flex items-center space-x-1">
                      ${renderIcon('target', 'w-3.5 h-3.5 text-emerald-400 mr-1')}
                      <span>Baseline Recommendation / Formula</span>
                    </div>
                    <div class="text-[11px] text-emerald-300/90 leading-relaxed">${escapeHtml(v.recommendation || '-')}</div>
                    ${v.suggested_value ? `<div class="text-[11px] font-mono text-white pt-0.5">Recommended Value: <strong class="text-emerald-400">${escapeHtml(v.suggested_value)}</strong></div>` : ''}
                  </div>

                  <div class="flex flex-wrap items-center justify-between gap-2 pt-1 border-t border-slate-800">
                    <div class="flex items-center space-x-2">
                      <button data-snippet="${escapeHtml(v.set_global_snippet)}" data-msg="SET GLOBAL snippet copied!" onclick="copySnippetText(this.dataset.snippet, this.dataset.msg)" class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-mono text-[11px] flex items-center space-x-1">
                        ${renderIcon('copy', 'w-3 h-3 text-slate-300 mr-1')}<span>Copy SET GLOBAL</span>
                      </button>
                      <button data-snippet="${escapeHtml(v.my_cnf_snippet)}" data-msg="my.cnf directive copied!" onclick="copySnippetText(this.dataset.snippet, this.dataset.msg)" class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-mono text-[11px] flex items-center space-x-1">
                        ${renderIcon('copy', 'w-3 h-3 text-slate-300 mr-1')}<span>Copy my.cnf</span>
                      </button>
                    </div>
                    <div>
                      <button data-var="${escapeHtml(v.name)}" data-val="${escapeHtml(v.value)}" data-desc="${escapeHtml(v.description || '')}" onclick="askCopilotAboutVariable(this.dataset.var, this.dataset.val, this.dataset.desc)" class="px-2.5 py-1 bg-indigo-600/30 hover:bg-indigo-600/50 text-indigo-300 border border-indigo-500/40 rounded text-xs flex items-center space-x-1">
                        ${renderIcon('sparkles', 'w-3.5 h-3.5 text-indigo-300 mr-1')}<span>Consult This Variable with Copilot</span>
                      </button>
                    </div>
                  </div>
                </div>
              </td>
            </tr>
          ` : ''}
        `;
      }).join('');
    }

    async function copySnippetText(text, title = 'Copied') {
      const ok = await copyTextToClipboard(text);
      if (ok) {
        showNativeAlert(text, title);
      } else {
        showNativeAlert('Copy failed in this browser context.', 'Copy Error');
      }
    }

    function askCopilotAboutVariable(varName, currVal, desc) {
      toggleAISidebar(true);
      const promptInput = document.getElementById('ai-prompt-input');
      if (!promptInput) return;

      const promptTemplate = `Please explain database parameter "${varName}" in depth (current active value: "${currVal}"):

1. What is the primary purpose and role of this variable in database architecture?
2. What are the impacts if set too low vs too high (trade-offs between memory, disk I/O, latency, and concurrency)?
3. What is the recommended optimal value or formula for production servers?
4. Is this variable dynamic (changeable via SET GLOBAL) or static (requires mysqld restart and editing my.cnf)?`;

      promptInput.value = promptTemplate;
      promptInput.focus();
      promptInput.setSelectionRange(promptInput.value.length, promptInput.value.length);
      showNativeAlert(`Variable analysis for "${varName}" inserted into Copilot. Click Send to analyze.`, 'Copilot Ready');
    }

    // --- Performance Flight Recorder ---
    let flightState = {
      isRecording: false,
      recordingId: null,
      name: '',
      startedAt: null,
      durationSeconds: 0,
      sampleCount: 0,
      intervalMs: 1000,
      ticks: [],
      selectedTickIndex: null,
      sessions: [],
      selectedSessionId: null,
    };
    let currentFlightQueries = [];
    let flightPollTimer = null;
    let flightCountdownTimer = null;

    function openFlightRecorderTab() {
      if (!activeConnection) {
        showNativeAlert('Please select and connect to a database session first.', 'Flight Recorder');
        return;
      }
      let tab = tabs.find(t => t.isFlightRecorder);
      if (!tab) {
        tab = {
          id: 'tab-flight-recorder',
          title: 'Flight Recorder',
          isFlightRecorder: true,
        };
        tabs.push(tab);
      }
      switchTab(tab.id);
      loadFlightSessions();
      checkFlightRecorderStatus();
      refreshFlightRecorderUI();
    }

    async function checkFlightRecorderStatus() {
      if (!activeConnection) return;
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/flight-recorder/status`);
        if (!res.ok) return;
        const data = await res.json();

        flightState.isRecording = !!data.is_recording;
        if (data.is_recording) {
          flightState.recordingId = data.recording_id;
          flightState.name = data.name;
          flightState.startedAt = new Date(data.started_at);
          flightState.durationSeconds = data.duration_seconds || 0;
          flightState.sampleCount = data.sample_count || 0;
          flightState.intervalMs = data.interval_ms || 1000;
          if (data.recent_ticks && data.recent_ticks.length > 0 && !flightState.selectedSessionId) {
            flightState.ticks = data.recent_ticks;
          }
          startFlightPollLoop();
        } else {
          stopFlightPollLoop();
        }
        updateFlightNavbarBadge();
        refreshFlightRecorderUI();
      } catch (err) {
        console.error('Failed to check flight recorder status:', err);
      }
    }

    function updateFlightNavbarBadge() {
      const dot = document.getElementById('flight-recorder-rec-dot');
      const badge = document.getElementById('flight-recorder-timer-badge');
      const monitorDot = document.getElementById('monitor-rec-dot');
      if (!dot || !badge) return;

      if (flightState.isRecording) {
        dot.className = 'inline-block w-2 h-2 rounded-full bg-rose-500 animate-ping';
        badge.classList.remove('hidden');
        const mins = Math.floor(flightState.durationSeconds / 60);
        const secs = flightState.durationSeconds % 60;
        badge.textContent = `REC ${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
        if (monitorDot) monitorDot.classList.remove('hidden');
      } else {
        dot.className = 'inline-block w-2 h-2 rounded-full bg-slate-500';
        badge.classList.add('hidden');
        if (monitorDot) monitorDot.classList.add('hidden');
      }
    }

    async function toggleFlightRecorder() {
      if (!activeConnection) return;
      if (!flightState.isRecording) {
        // Start recording
        const intervalEl = document.getElementById('fr-interval-select');
        const intervalMs = intervalEl ? parseInt(intervalEl.value, 10) : 1000;
        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/flight-recorder/start`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ interval_ms: intervalMs })
          });
          if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            showNativeAlert('Failed to start recording: ' + (err.error || res.statusText), 'Flight Recorder');
            return;
          }
          const data = await res.json();
          flightState.isRecording = true;
          flightState.recordingId = data.recording_id;
          flightState.startedAt = new Date(data.started_at);
          flightState.durationSeconds = 0;
          flightState.sampleCount = 0;
          flightState.intervalMs = intervalMs;
          flightState.ticks = [];
          flightState.selectedTickIndex = null;
          flightState.selectedSessionId = null;

          startFlightPollLoop();
          updateFlightNavbarBadge();
          refreshFlightRecorderUI();
          loadFlightSessions();
        } catch (err) {
          showNativeAlert('Failed to start recording: ' + err.message, 'Flight Recorder');
        }
      } else {
        // Stop recording
        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/flight-recorder/stop`, {
            method: 'POST'
          });
          if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            showNativeAlert('Failed to stop recording: ' + (err.error || res.statusText), 'Flight Recorder');
            return;
          }
          const finishedSession = await res.json();
          flightState.isRecording = false;
          stopFlightPollLoop();
          updateFlightNavbarBadge();
          await loadFlightSessions();
          if (finishedSession && finishedSession.id) {
            loadFlightRecorderSession(finishedSession.id);
          } else {
            refreshFlightRecorderUI();
          }
          showNativeAlert('Flight recording completed and saved to vault.', 'Recording Saved');
        } catch (err) {
          showNativeAlert('Failed to stop recording: ' + err.message, 'Flight Recorder');
        }
      }
    }

    function startFlightPollLoop() {
      if (flightPollTimer) clearInterval(flightPollTimer);
      if (flightCountdownTimer) clearInterval(flightCountdownTimer);

      flightCountdownTimer = setInterval(() => {
        if (flightState.isRecording) {
          flightState.durationSeconds++;
          updateFlightNavbarBadge();
          const pill = document.getElementById('fr-pill-text');
          if (pill) {
            const mins = Math.floor(flightState.durationSeconds / 60);
            const secs = flightState.durationSeconds % 60;
            const remaining = Math.max(0, 600 - flightState.durationSeconds);
            const rMin = Math.floor(remaining / 60);
            const rSec = remaining % 60;
            pill.textContent = `REC ${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')} (Auto-stop in: ${String(rMin).padStart(2, '0')}:${String(rSec).padStart(2, '0')})`;
          }
        }
      }, 1000);

      flightPollTimer = setInterval(async () => {
        if (!activeConnection || !flightState.isRecording) return;
        try {
          const res = await apiFetch(`/api/session/${activeConnection.id}/flight-recorder/status`);
          if (res.ok) {
            const data = await res.json();
            if (!data.is_recording) {
              // Auto-stopped by server 10m safety guard
              flightState.isRecording = false;
              stopFlightPollLoop();
              updateFlightNavbarBadge();
              loadFlightSessions();
              refreshFlightRecorderUI();
              return;
            }
            if (data.recent_ticks && !flightState.selectedSessionId) {
              flightState.ticks = data.recent_ticks;
              const activeTab = tabs.find(t => t.id === activeTabId);
              if (activeTab && activeTab.isFlightRecorder) {
                renderFlightMetricsCards();
                drawFlightRecorderChart();
                if (flightState.selectedTickIndex == null && flightState.ticks.length > 0) {
                  renderCorrelatedQueries(flightState.ticks[flightState.ticks.length - 1]);
                }
              }
            }
          }
        } catch (e) {}
      }, flightState.intervalMs || 1000);
    }

    function stopFlightPollLoop() {
      if (flightPollTimer) {
        clearInterval(flightPollTimer);
        flightPollTimer = null;
      }
      if (flightCountdownTimer) {
        clearInterval(flightCountdownTimer);
        flightCountdownTimer = null;
      }
    }

    async function loadFlightSessions() {
      if (!activeConnection) return;
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/flight-recorder/sessions`);
        if (!res.ok) return;
        const sessions = await res.json();
        flightState.sessions = sessions || [];

        const dropdown = document.getElementById('fr-session-dropdown');
        if (dropdown) {
          const prevVal = dropdown.value;
          dropdown.innerHTML = `<option value="">-- Active Live View --</option>` +
            flightState.sessions.map(s => {
              const dt = new Date(s.started_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
              const m = Math.floor(s.duration_seconds / 60);
              const sec = s.duration_seconds % 60;
              return `<option value="${s.id}">${escapeHtml(s.name)} (${dt} - ${m}m ${sec}s, ${s.sample_count} pts)</option>`;
            }).join('');
          dropdown.value = prevVal || '';
        }
      } catch (err) {
        console.error('Failed to load flight sessions:', err);
      }
    }

    async function onSelectFlightRecorderSession(recId) {
      if (!recId) {
        flightState.selectedSessionId = null;
        const delBtn = document.getElementById('fr-btn-delete');
        if (delBtn) delBtn.classList.add('hidden');
        checkFlightRecorderStatus();
        return;
      }
      await loadFlightRecorderSession(recId);
    }

    async function loadFlightRecorderSession(recId) {
      if (!activeConnection || !recId) return;
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/flight-recorder/sessions/${recId}`);
        if (!res.ok) {
          showNativeAlert('Failed to load recording session.', 'Flight Recorder');
          return;
        }
        const detail = await res.json();
        flightState.selectedSessionId = recId;
        flightState.ticks = detail.ticks || [];
        flightState.selectedTickIndex = flightState.ticks.length > 0 ? flightState.ticks.length - 1 : null;

        const dropdown = document.getElementById('fr-session-dropdown');
        if (dropdown) dropdown.value = recId;

        const delBtn = document.getElementById('fr-btn-delete');
        if (delBtn) delBtn.classList.remove('hidden');

        refreshFlightRecorderUI();
      } catch (err) {
        showNativeAlert('Error loading session: ' + err.message, 'Flight Recorder');
      }
    }

    async function deleteCurrentFlightSession() {
      if (!activeConnection || !flightState.selectedSessionId) return;
      const ok = await showNativeConfirm('Delete this flight recording and all its sampled metrics permanently?', 'Delete Recording');
      if (!ok) return;

      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/flight-recorder/sessions/${flightState.selectedSessionId}`, {
          method: 'DELETE'
        });
        if (!res.ok) {
          showNativeAlert('Failed to delete recording.', 'Delete Error');
          return;
        }
        flightState.selectedSessionId = null;
        flightState.ticks = [];
        flightState.selectedTickIndex = null;
        const delBtn = document.getElementById('fr-btn-delete');
        if (delBtn) delBtn.classList.add('hidden');

        await loadFlightSessions();
        checkFlightRecorderStatus();
      } catch (err) {
        showNativeAlert('Failed to delete recording: ' + err.message, 'Delete Error');
      }
    }

    function exportFlightRecorderJSON() {
      if (!flightState.ticks || flightState.ticks.length === 0) {
        showNativeAlert('No recorded metrics available to export.', 'Export Flight Recorder');
        return;
      }
      const exportData = {
        exported_at: new Date().toISOString(),
        connection: activeConnection ? activeConnection.name : 'unknown',
        recording_id: flightState.selectedSessionId || flightState.recordingId,
        ticks: flightState.ticks
      };
      const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `flight-recorder-${Date.now()}.json`;
      a.click();
    }

    function refreshFlightRecorderUI() {
      // Toggle button
      const btnToggle = document.getElementById('fr-btn-toggle');
      const btnIcon = document.getElementById('fr-btn-icon');
      const btnText = document.getElementById('fr-btn-text');
      const pillDot = document.getElementById('fr-pill-dot');
      const pillText = document.getElementById('fr-pill-text');

      if (btnToggle && btnIcon && btnText) {
        if (flightState.isRecording) {
          btnToggle.className = 'h-7 px-3 bg-slate-800 hover:bg-slate-700 text-rose-300 border border-rose-600 rounded font-medium flex items-center space-x-1.5 transition shadow-sm';
          btnIcon.innerHTML = renderIcon('square', 'w-3 h-3 text-rose-400');
          btnText.textContent = 'Stop Recording';
        } else {
          btnToggle.className = 'h-7 px-3 bg-rose-600 hover:bg-rose-500 text-white rounded font-medium flex items-center space-x-1.5 transition shadow-sm';
          btnIcon.innerHTML = '<span class="w-2 h-2 rounded-full bg-white inline-block"></span>';
          btnText.textContent = 'Start Recording';
        }
      }

      if (pillDot && pillText) {
        if (flightState.isRecording) {
          pillDot.className = 'inline-block w-2 h-2 rounded-full bg-rose-500 animate-pulse';
          const mins = Math.floor(flightState.durationSeconds / 60);
          const secs = flightState.durationSeconds % 60;
          pillText.textContent = `REC ${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
        } else if (flightState.selectedSessionId) {
          pillDot.className = 'inline-block w-2 h-2 rounded-full bg-emerald-500';
          pillText.textContent = `REVIEWING (${flightState.ticks.length} pts)`;
        } else {
          pillDot.className = 'inline-block w-2 h-2 rounded-full bg-slate-500';
          pillText.textContent = 'IDLE';
        }
      }

      setupFlightCanvasInteraction();
      renderFlightMetricsCards();
      drawFlightRecorderChart();

      if (flightState.selectedTickIndex != null && flightState.ticks[flightState.selectedTickIndex]) {
        renderCorrelatedQueries(flightState.ticks[flightState.selectedTickIndex]);
      } else if (flightState.ticks.length > 0) {
        renderCorrelatedQueries(flightState.ticks[flightState.ticks.length - 1]);
      } else {
        const qList = document.getElementById('fr-queries-list');
        if (qList) {
          qList.innerHTML = `<div class="text-slate-500 text-center py-6 text-xs italic">Select a point on the chart timeline to inspect active queries running at that second.</div>`;
        }
      }
    }

    function renderFlightMetricsCards() {
      const qpsEl = document.getElementById('fr-metric-qps');
      const thEl = document.getElementById('fr-metric-threads');
      const lkEl = document.getElementById('fr-metric-locks');
      const bfEl = document.getElementById('fr-metric-buffer');

      const tick = (flightState.selectedTickIndex != null && flightState.ticks[flightState.selectedTickIndex])
        ? flightState.ticks[flightState.selectedTickIndex]
        : (flightState.ticks.length > 0 ? flightState.ticks[flightState.ticks.length - 1] : null);

      if (!tick) {
        if (qpsEl) qpsEl.textContent = '0 qps';
        if (thEl) thEl.textContent = '0 / 0 conn';
        if (lkEl) lkEl.textContent = '0 waits';
        if (bfEl) bfEl.textContent = '0 stalls';
        return;
      }

      if (qpsEl) qpsEl.textContent = `${tick.qps.toLocaleString()} qps`;
      if (thEl) thEl.textContent = `${tick.threads_running} / ${tick.threads_connected} conn`;
      if (lkEl) lkEl.textContent = `${tick.row_lock_waits.toLocaleString()} waits`;
      if (bfEl) bfEl.textContent = `${tick.buffer_pool_wait_free.toLocaleString()} stalls`;
    }

    function setupFlightCanvasInteraction() {
      const canvas = document.getElementById('fr-canvas');
      if (!canvas || canvas.dataset.initialized) return;
      canvas.dataset.initialized = 'true';

      const tooltip = document.getElementById('fr-canvas-tooltip');

      canvas.addEventListener('mousemove', (e) => {
        if (!flightState.ticks || flightState.ticks.length === 0) return;
        const rect = canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const paddingLeft = 45;
        const paddingRight = 15;
        const chartWidth = rect.width - paddingLeft - paddingRight;

        if (x < paddingLeft || x > rect.width - paddingRight) {
          if (tooltip) tooltip.classList.add('hidden');
          return;
        }

        const ratio = (x - paddingLeft) / chartWidth;
        const index = Math.round(ratio * (flightState.ticks.length - 1));
        const clampedIdx = Math.max(0, Math.min(flightState.ticks.length - 1, index));
        const tick = flightState.ticks[clampedIdx];

        if (tooltip) {
          const dt = new Date(tick.sampled_at).toLocaleTimeString();
          tooltip.innerHTML = `
            <div class="font-bold text-white border-b border-slate-700 pb-1 mb-1 flex items-center justify-between gap-3">
              <span>⏱ ${dt}</span>
              <span class="text-slate-400 font-normal">#${clampedIdx + 1}</span>
            </div>
            <div class="text-emerald-400 font-medium">QPS: <span class="font-bold">${tick.qps}</span></div>
            <div class="text-rose-400 font-medium">Threads: <span class="font-bold">${tick.threads_running}</span> / ${tick.threads_connected}</div>
            <div class="text-amber-400 font-medium">Lock Waits: <span class="font-bold">${tick.row_lock_waits}</span></div>
            <div class="text-purple-400 font-medium">Buffer Waits: <span class="font-bold">${tick.buffer_pool_wait_free}</span></div>
          `;
          tooltip.classList.remove('hidden');
          let tipX = x + 15;
          if (tipX + 160 > rect.width) tipX = x - 170;
          tooltip.style.left = `${tipX}px`;
          tooltip.style.top = `20px`;
        }
      });

      canvas.addEventListener('mouseleave', () => {
        if (tooltip) tooltip.classList.add('hidden');
      });

      canvas.addEventListener('click', (e) => {
        if (!flightState.ticks || flightState.ticks.length === 0) return;
        const rect = canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const paddingLeft = 45;
        const paddingRight = 15;
        const chartWidth = rect.width - paddingLeft - paddingRight;
        if (x < paddingLeft || x > rect.width - paddingRight) return;

        const ratio = (x - paddingLeft) / chartWidth;
        const index = Math.round(ratio * (flightState.ticks.length - 1));
        const clampedIdx = Math.max(0, Math.min(flightState.ticks.length - 1, index));

        flightState.selectedTickIndex = clampedIdx;
        drawFlightRecorderChart();
        renderFlightMetricsCards();
        renderCorrelatedQueries(flightState.ticks[clampedIdx]);
      });

      window.addEventListener('resize', () => {
        const activeTab = tabs.find(t => t.id === activeTabId);
        if (activeTab && activeTab.isFlightRecorder) {
          drawFlightRecorderChart();
        }
        if (activeTab && activeTab.isProcesslist && plTrendPanelOpen) {
          renderProcesslistTrendChart();
        }
      });
    }

    function drawFlightRecorderChart() {
      const canvas = document.getElementById('fr-canvas');
      if (!canvas) return;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;

      const rect = canvas.getBoundingClientRect();
      const dpr = window.devicePixelRatio || 1;
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      ctx.scale(dpr, dpr);

      const w = rect.width;
      const h = rect.height;
      const padLeft = 45;
      const padRight = 15;
      const padTop = 15;
      const padBottom = 25;
      const chartW = w - padLeft - padRight;
      const chartH = h - padTop - padBottom;

      ctx.clearRect(0, 0, w, h);

      const ticks = flightState.ticks;
      if (!ticks || ticks.length === 0) {
        ctx.fillStyle = '#64748B';
        ctx.font = '12px "JetBrains Mono", ui-monospace, SFMono-Regular, monospace';
        ctx.textAlign = 'center';
        ctx.fillText('No telemetry ticks recorded yet. Click "● Start Recording" to capture performance.', w / 2, h / 2);
        return;
      }

      // Find max values for dual scale (QPS scale on left, Threads/Locks scale on right)
      let maxQps = 10;
      let maxConcurrency = 5;

      ticks.forEach(t => {
        if (t.qps > maxQps) maxQps = t.qps;
        if (t.threads_running > maxConcurrency) maxConcurrency = t.threads_running;
        if (t.threads_connected > maxConcurrency) maxConcurrency = t.threads_connected;
        if (t.row_lock_waits > maxConcurrency) maxConcurrency = t.row_lock_waits;
        if (t.buffer_pool_wait_free > maxConcurrency) maxConcurrency = t.buffer_pool_wait_free;
      });

      maxQps = Math.ceil(maxQps * 1.15);
      maxConcurrency = Math.ceil(maxConcurrency * 1.2);

      // Draw subtle grid lines
      ctx.strokeStyle = 'rgba(51, 65, 85, 0.4)';
      ctx.lineWidth = 1;
      ctx.fillStyle = '#64748B';
      ctx.font = '10px "JetBrains Mono", ui-monospace, monospace';
      ctx.textAlign = 'right';

      const gridSteps = 4;
      for (let i = 0; i <= gridSteps; i++) {
        const y = padTop + (chartH / gridSteps) * i;
        ctx.beginPath();
        ctx.moveTo(padLeft, y);
        ctx.lineTo(w - padRight, y);
        ctx.stroke();

        const qVal = Math.round(maxQps - (maxQps / gridSteps) * i);
        ctx.fillText(qVal.toString(), padLeft - 6, y + 3);
      }

      // X-Axis Time Markers
      ctx.textAlign = 'center';
      const timeStep = Math.max(1, Math.floor(ticks.length / 5));
      for (let i = 0; i < ticks.length; i += timeStep) {
        const x = padLeft + (i / Math.max(1, ticks.length - 1)) * chartW;
        const dt = new Date(ticks[i].sampled_at);
        const timeStr = dt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
        ctx.fillText(timeStr, x, h - 8);
      }

      // Helper function to draw series line
      function drawSeries(getDataVal, maxVal, strokeColor, fillColor) {
        if (ticks.length === 1) {
          const x = padLeft + chartW / 2;
          const y = padTop + chartH - (getDataVal(ticks[0]) / maxVal) * chartH;
          ctx.fillStyle = strokeColor;
          ctx.beginPath();
          ctx.arc(x, y, 4, 0, Math.PI * 2);
          ctx.fill();
          return;
        }

        ctx.strokeStyle = strokeColor;
        ctx.lineWidth = 2;
        ctx.beginPath();
        for (let i = 0; i < ticks.length; i++) {
          const val = getDataVal(ticks[i]);
          const x = padLeft + (i / (ticks.length - 1)) * chartW;
          const y = padTop + chartH - (val / maxVal) * chartH;
          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();

        if (fillColor) {
          ctx.lineTo(padLeft + chartW, padTop + chartH);
          ctx.lineTo(padLeft, padTop + chartH);
          ctx.closePath();
          ctx.fillStyle = fillColor;
          ctx.fill();
        }
      }

      // 1. Draw QPS (Green Area + Line)
      drawSeries(t => t.qps, maxQps, '#10B981', 'rgba(16, 185, 129, 0.08)');

      // 2. Draw Threads Connected (Blue)
      drawSeries(t => t.threads_connected, maxConcurrency, '#3B82F6', null);

      // 3. Draw Threads Running (Rose)
      drawSeries(t => t.threads_running, maxConcurrency, '#F43F5E', 'rgba(244, 63, 94, 0.08)');

      // 4. Draw Row Lock Waits (Amber)
      drawSeries(t => t.row_lock_waits, maxConcurrency, '#F59E0B', null);

      // 5. Draw Selected Highlight Scrubber
      if (flightState.selectedTickIndex != null && ticks[flightState.selectedTickIndex]) {
        const selIdx = flightState.selectedTickIndex;
        const selX = padLeft + (selIdx / Math.max(1, ticks.length - 1)) * chartW;

        ctx.setLineDash([4, 4]);
        ctx.strokeStyle = '#FFFFFF';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(selX, padTop);
        ctx.lineTo(selX, padTop + chartH);
        ctx.stroke();
        ctx.setLineDash([]);

        // Draw dot on selected QPS
        const qY = padTop + chartH - (ticks[selIdx].qps / maxQps) * chartH;
        ctx.fillStyle = '#10B981';
        ctx.beginPath();
        ctx.arc(selX, qY, 5, 0, Math.PI * 2);
        ctx.fill();

        // Draw dot on selected Threads Running
        const thY = padTop + chartH - (ticks[selIdx].threads_running / maxConcurrency) * chartH;
        ctx.fillStyle = '#F43F5E';
        ctx.beginPath();
        ctx.arc(selX, thY, 5, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    function renderCorrelatedQueries(tick) {
      const qList = document.getElementById('fr-queries-list');
      const timeBadge = document.getElementById('fr-queries-timestamp-badge');
      const countEl = document.getElementById('fr-queries-count');
      if (!qList) return;

      const dt = new Date(tick.sampled_at).toLocaleTimeString();
      if (timeBadge) timeBadge.textContent = `At ${dt}`;

      let queries = [];
      try {
        queries = JSON.parse(tick.top_queries_json || '[]');
      } catch (e) {
        queries = [];
      }

      // Filter out system threads or empty statements (defense-in-depth for historical recordings)
      queries = queries.filter(q => q && q.info && q.info.trim() !== '' && q.user !== 'system user' && q.user !== 'event_scheduler');

      currentFlightQueries = queries;

      if (countEl) countEl.textContent = `${queries.length} active queries`;

      if (!queries || queries.length === 0) {
        qList.innerHTML = `
          <div class="p-6 text-center text-slate-500 bg-slate-900/50 rounded border border-slate-800/80">
            <span class="text-emerald-400 font-medium">✓ No slow or blocking queries captured at this second.</span>
            <div class="text-[11px] text-slate-500 mt-1">Database threads were either sleeping in connection pools or running instantaneous sub-millisecond lookups.</div>
          </div>
        `;
        return;
      }

      qList.innerHTML = queries.map((q, idx) => {
        const sqlEscaped = escapeHtml(q.info || '/* empty statement */');
        const timeSec = q.time || 0;
        const timeBadgeColor = timeSec > 2 ? 'bg-rose-900 text-rose-200 border-rose-700' : (timeSec > 0 ? 'bg-amber-900 text-amber-200 border-amber-700' : 'bg-slate-800 text-slate-300 border-slate-700');

        return `
          <div class="p-3 bg-slate-900/90 border border-slate-800 rounded-lg space-y-2">
            <div class="flex items-center justify-between text-xs">
              <div class="flex items-center space-x-2">
                <span class="font-bold text-white font-mono">Thread #${q.thread_id}</span>
                <span class="px-1.5 py-0.5 rounded text-[10px] font-mono border inline-flex items-center gap-1 ${timeBadgeColor}">${renderIcon('clock', 'w-2.5 h-2.5 text-slate-400')}<span>${timeSec}s</span></span>
                <span class="text-slate-400 text-[11px]">${escapeHtml(q.user || 'app')}@${escapeHtml(q.host || 'localhost')}</span>
                ${q.db ? `<span class="px-1.5 py-0.2 rounded text-[10px] bg-slate-800 text-slate-300 font-mono inline-flex items-center gap-1">${renderIcon('folder', 'w-2.5 h-2.5 text-slate-400')}<span>${escapeHtml(q.db)}</span></span>` : ''}
                ${q.state ? `<span class="text-[11px] text-slate-400 italic">(${escapeHtml(q.state)})</span>` : ''}
              </div>

              <!-- Action buttons -->
              <div class="flex items-center space-x-1.5 text-xs">
                <button onclick="explainFlightQuery(${idx})" class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 text-sky-300 border border-sky-800 transition flex items-center space-x-1" title="Explain Execution Plan">
                  <svg class="w-3.5 h-3.5 text-sky-400 shrink-0" aria-hidden="true"><use href="#icon-zap"/></svg><span>Explain</span>
                </button>
                <button onclick="sendFlightQueryToEditor(${idx})" class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 transition flex items-center space-x-1" title="Load Query into Editor Tab">
                  <svg class="w-3.5 h-3.5 text-slate-400 shrink-0" aria-hidden="true"><use href="#icon-terminal"/></svg><span>To Tab</span>
                </button>
                <button onclick="askCopilotFlightQuery(${idx})" class="px-2 py-0.5 rounded bg-purple-950 hover:bg-purple-900 text-purple-300 border border-purple-800 transition flex items-center space-x-1" title="Ask AI Copilot for tuning advice">
                  ${renderIcon('bot', 'w-3 h-3 text-purple-300 mr-1')}<span>Ask Copilot</span>
                </button>
                <button onclick="copyFlightQuery(this, ${idx})" class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 transition flex items-center space-x-1" title="Copy SQL">
                  ${renderIcon('copy', 'w-3 h-3 text-slate-300 mr-1')}<span>Copy</span>
                </button>
              </div>
            </div>

            <!-- SQL Text Box -->
            <pre class="bg-slate-950 p-2.5 rounded border border-slate-800 text-xs font-mono text-emerald-300 overflow-x-auto whitespace-pre-wrap max-h-32 select-all leading-relaxed">${sqlEscaped}</pre>
          </div>
        `;
      }).join('');
    }

    function explainFlightQuery(idx) {
      const q = currentFlightQueries[idx];
      if (!q || !q.info) return;
      explainSpecificQuery(q.info, q.db);
    }

    function sendFlightQueryToEditor(idx) {
      const q = currentFlightQueries[idx];
      if (!q || !q.info) return;
      sendProcessQueryToEditor(q.info);
    }

    function askCopilotFlightQuery(idx) {
      const q = currentFlightQueries[idx];
      if (!q || !q.info) return;
      const tick = flightState.selectedTickIndex != null && flightState.ticks[flightState.selectedTickIndex]
        ? flightState.ticks[flightState.selectedTickIndex]
        : { qps: 0, threads_running: 0 };

      const aiSidebar = document.getElementById('ai-sidebar');
      if (aiSidebar && aiSidebar.classList.contains('hidden')) {
        toggleAISidebar();
      }
      const promptInput = document.getElementById('ai-prompt-input');
      if (!promptInput) return;

      const template = `This query was captured during a performance spike in Performance Flight Recorder (Query Duration: ${q.time || 0}s, System QPS: ${tick.qps}, Threads Running: ${tick.threads_running}):

\`\`\`sql
${q.info}
\`\`\`

Please analyze what is causing this query to be slow and suggest necessary query or index optimizations.`;

      promptInput.value = template;
      promptInput.focus();
      showNativeAlert('Query and spike metrics inserted into Copilot. Click Send for optimization analysis.', 'Copilot Ready');
    }

    async function copyFlightQuery(btn, idx) {
      const q = currentFlightQueries[idx];
      if (!q || !q.info) return;
      const ok = await copyTextToClipboard(q.info);
      if (ok) {
        const orig = btn.innerHTML;
        btn.innerHTML = `${renderIcon('check', 'w-3.5 h-3.5 inline mr-1 text-emerald-400')}<span>Copied!</span>`;
        btn.classList.add('bg-emerald-800', 'text-white');
        setTimeout(() => {
          btn.innerHTML = orig;
          btn.classList.remove('bg-emerald-800', 'text-white');
        }, 1500);
      } else {
        showNativeAlert('Copy failed in this browser context.', 'Copy Error');
      }
    }

    // --- Session Audit Log Drawer ---
    let auditLogsCache = [];

    function toggleAuditDrawer(force) {
      const drawer = document.getElementById('audit-drawer');
      if (!drawer) return;
      const isHidden = drawer.classList.contains('hidden');
      const shouldOpen = force !== undefined ? force : isHidden;
      if (shouldOpen) {
        drawer.classList.remove('hidden');
        loadAuditLogs();
      } else {
        drawer.classList.add('hidden');
      }
    }

    function onAuditSearchInput(val) {
      const clearBtn = document.getElementById('audit-search-clear-btn');
      if (clearBtn) {
        if (val && val.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
      loadAuditLogs();
    }

    function clearAuditSearch() {
      const input = document.getElementById('audit-search-input');
      const clearBtn = document.getElementById('audit-search-clear-btn');
      if (input) {
        input.value = '';
        input.focus();
      }
      if (clearBtn) {
        clearBtn.classList.add('hidden');
      }
      loadAuditLogs();
    }

    let auditSourceFilter = 'all';

    function onAuditSourceChange(val) {
      auditSourceFilter = val || 'all';
      loadAuditLogs();
    }

    function getAuditSourceBadge(src) {
      switch (src) {
        case 'grid_cell_edit':
          return '<span class="px-1.5 py-0.5 rounded bg-amber-950/80 text-amber-300 border border-amber-800/60 text-[10px] font-medium" title="Inline Cell Edit (Grid)">Cell Edit</span>';
        case 'grid_insert':
          return '<span class="px-1.5 py-0.5 rounded bg-emerald-950/80 text-emerald-300 border border-emerald-800/60 text-[10px] font-medium" title="Insert Row (+ Add Row)">+ Add Row</span>';
        case 'grid_delete':
          return '<span class="px-1.5 py-0.5 rounded bg-rose-950/80 text-rose-300 border border-rose-800/60 text-[10px] font-medium" title="Delete Rows (Grid)">Delete</span>';
        case 'script_runner':
          return '<span class="px-1.5 py-0.5 rounded bg-cyan-950/80 text-cyan-300 border border-cyan-800/60 text-[10px] font-medium" title="Script Runner (.sql)">Script</span>';
        case 'csv_import':
          return '<span class="px-1.5 py-0.5 rounded bg-indigo-950/80 text-indigo-300 border border-indigo-800/60 text-[10px] font-medium" title="CSV Import">CSV</span>';
        case 'mock_data':
          return '<span class="px-1.5 py-0.5 rounded bg-purple-950/80 text-purple-300 border border-purple-800/60 text-[10px] font-medium" title="Synthetic Mock Data">Mock Data</span>';
        case 'sync_transfer':
          return '<span class="px-1.5 py-0.5 rounded bg-blue-950/80 text-blue-300 border border-blue-800/60 text-[10px] font-medium" title="Data Transfer Sync">Data Sync</span>';
        case 'schema_sync':
          return '<span class="px-1.5 py-0.5 rounded bg-teal-950/80 text-teal-300 border border-teal-800/60 text-[10px] font-medium" title="Schema Structure Sync">DDL Sync</span>';
        case 'editor':
        default:
          return '<span class="px-1.5 py-0.5 rounded bg-sky-950/80 text-sky-300 border border-sky-800/60 text-[10px] font-medium" title="Query Editor / SQL Tab">Editor</span>';
      }
    }

    async function loadAuditLogs() {
      if (!activeConnection) return;
      const searchEl = document.getElementById('audit-search-input');
      const search = searchEl ? searchEl.value.trim() : '';
      const clearBtn = document.getElementById('audit-search-clear-btn');
      if (clearBtn && searchEl) {
        if (searchEl.value.length > 0) clearBtn.classList.remove('hidden');
        else clearBtn.classList.add('hidden');
      }
      try {
        const sourceParam = (auditSourceFilter && auditSourceFilter !== 'all') ? `&source=${encodeURIComponent(auditSourceFilter)}` : '';
        const url = `/api/session/${activeConnection.id}/audit-logs?limit=200${search ? '&search=' + encodeURIComponent(search) : ''}${sourceParam}`;
        const res = await apiFetch(url);
        if (!res.ok) return;
        const data = await res.json();
        auditLogsCache = data.logs || [];
        const countEl = document.getElementById('audit-total-count');
        if (countEl) countEl.textContent = `(${data.total || 0} entries)`;
        renderAuditLogsUI();
      } catch (err) {
        console.error('Failed to load audit logs:', err);
      }
    }

    function renderAuditLogsUI() {
      const tbody = document.getElementById('audit-tbody');
      if (!tbody) return;

      if (!auditLogsCache || auditLogsCache.length === 0) {
        tbody.innerHTML = `<tr><td colspan="7" class="p-8 text-center text-slate-500 italic">No query history records found.</td></tr>`;
        return;
      }

      tbody.innerHTML = auditLogsCache.map((l, idx) => {
        const dt = new Date(l.executed_at);
        const timeStr = isNaN(dt.getTime()) ? l.executed_at : dt.toLocaleTimeString();
        const isErr = !!l.error;

        return `
          <tr class="hover:bg-slate-800/40 transition">
            <td class="p-2 text-slate-400 whitespace-nowrap text-[10px]">${escapeHtml(timeStr)}</td>
            <td class="p-2 text-center whitespace-nowrap">${getAuditSourceBadge(l.source)}</td>
            <td class="p-2 text-right text-slate-300 font-mono text-[11px] whitespace-nowrap">${l.duration_ms} ms</td>
            <td class="p-2 text-right text-slate-400 font-mono text-[11px] whitespace-nowrap">${l.rows_affected}</td>
            <td class="p-2 text-center whitespace-nowrap">
              ${isErr
                ? `<span class="px-1.5 py-0.5 rounded bg-red-950 text-red-400 border border-red-800 text-[10px]" title="${escapeHtml(l.error)}">ERR</span>`
                : `<span class="px-1.5 py-0.5 rounded bg-emerald-950 text-emerald-400 border border-emerald-800 text-[10px]">OK</span>`}
            </td>
            <td class="p-2 font-mono text-[11px] text-slate-200 truncate max-w-xl" title="${escapeHtml(l.query_sql)}">
              ${escapeHtml(l.query_sql)}
            </td>
            <td class="p-2 text-center whitespace-nowrap">
              <div class="inline-flex space-x-1">
                <button onclick="copyAuditSQL(${idx})" class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] border border-slate-700" title="Copy SQL statement">Copy</button>
                <button onclick="insertAuditSQLToEditor(${idx})" class="px-2 py-0.5 rounded bg-app-accent hover:bg-sky-400 text-slate-950 font-bold text-[10px]" title="Insert into Query Editor">To Editor</button>
              </div>
            </td>
          </tr>
        `;
      }).join('');
    }

    async function copyAuditSQL(sqlOrIdx) {
      const sql = (typeof sqlOrIdx === 'number' && auditLogsCache[sqlOrIdx]) ? auditLogsCache[sqlOrIdx].query_sql : sqlOrIdx;
      const ok = await copyTextToClipboard(sql);
      if (ok) {
        showNativeAlert('SQL statement copied to clipboard.', 'Copied');
      } else {
        showNativeAlert('Copy failed in this browser context.', 'Copy Error');
      }
    }

    function insertAuditSQLToEditor(sqlOrIdx) {
      const sql = (typeof sqlOrIdx === 'number' && auditLogsCache[sqlOrIdx]) ? auditLogsCache[sqlOrIdx].query_sql : sqlOrIdx;
      setEditorSQL(sql);
      toggleAuditDrawer(false);
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab && (activeTab.isTableViewer || activeTab.isProcesslist || activeTab.isCluster || activeTab.isFlightRecorder || activeTab.isVariables)) {
        const queryTab = tabs.find(t => !t.isTableViewer && !t.isProcesslist && !t.isCluster && !t.isFlightRecorder && !t.isVariables);
        if (queryTab) switchTab(queryTab.id);
        else addNewQueryTab();
      }
    }

    async function clearAuditLogs() {
      if (!activeConnection) return;
      const ok = await showNativeConfirm('Are you sure you want to permanently clear all query execution logs for this session?', 'Clear Audit History');
      if (!ok) return;
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/audit-logs`, { method: 'DELETE' });
        if (res.ok) {
          loadAuditLogs();
        }
      } catch (err) {
        showNativeAlert('Failed to clear audit logs: ' + err.message, 'Clear Error');
      }
    }

    function exportAuditLogsCSV() {
      if (!auditLogsCache || auditLogsCache.length === 0) {
        showNativeAlert('No audit logs available to export.', 'Export CSV');
        return;
      }
      const headers = ['Executed At', 'Source', 'Duration (ms)', 'Rows Affected', 'Status', 'Error', 'SQL'];
      const rows = auditLogsCache.map(l => [
        `"${String(l.executed_at).replace(/"/g, '""')}"`,
        `"${String(l.source || 'editor').replace(/"/g, '""')}"`,
        l.duration_ms,
        l.rows_affected,
        l.error ? 'ERROR' : 'OK',
        `"${String(l.error || '').replace(/"/g, '""')}"`,
        `"${String(l.query_sql || '').replace(/"/g, '""')}"`
      ]);
      const csvContent = '\uFEFF' + [headers.join(','), ...rows.map(r => r.join(','))].join('\r\n');
      const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `session-audit-logs-${Date.now()}.csv`;
      a.click();
    }

    function openTableViewer(tableName) {
      const activeDb = (schemaCatalogCache && schemaCatalogCache.database_name) || '';
      let tab = tabs.find(t => t.tableName === tableName && (!t.database || t.database === activeDb));
      if (!tab) {
        tab = {
          id: 'tab-tbl-' + (activeDb ? activeDb + '-' : '') + tableName,
          title: tableName,
          isTableViewer: true,
          tableName: tableName,
          database: activeDb,
          tableMode: 'data',
          filterMode: 'pills',
          filterPills: [],
          filterRaw: '',
          appliedFilter: '',
          sortCol: '',
          sortDir: '',
          offset: 0,
          sql: `SELECT * FROM \`${tableName}\` LIMIT 50;`
        };
        tabs.push(tab);
      }
      switchTab(tab.id);
      setTableMode('data');
    }

