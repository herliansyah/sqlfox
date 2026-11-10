// --- Update Engine & Changelog Viewer ---
    let latestUpdateInfo = null;

    async function checkOrOpenUpdate(userInitiated = false) {
      const verTextEl = document.getElementById('footer-version-text');
      const updateBtn = document.getElementById('footer-update-btn');
      const checkBtn = document.getElementById('btn-changelog-check-update');
      let originalBtnHTML = '';

      if (userInitiated && latestUpdateInfo && latestUpdateInfo.has_update) {
        openUpdateDialog(latestUpdateInfo);
        return;
      }

      if (userInitiated) {
        showToast('Checking for updates...', 'info');
        if (checkBtn) {
          originalBtnHTML = checkBtn.innerHTML;
          checkBtn.disabled = true;
          checkBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')}<span>Checking...</span>`;
        }
      }

      try {
        const url = userInitiated ? '/api/system/update/check?force=true' : '/api/system/update/check';
        const res = await fetch(url);
        const data = await res.json();

        if (data.current_version && verTextEl) {
          verTextEl.textContent = `SqlFox ${data.current_version}`;
        }

        latestUpdateInfo = data;

        if (data.error && userInitiated) {
          showToast(`Update check: ${data.error}`, 'warning');
          if (checkBtn && originalBtnHTML) checkBtn.innerHTML = originalBtnHTML;
          return;
        }

        if (data.has_update) {
          if (updateBtn) {
            updateBtn.classList.remove('hidden');
            updateBtn.classList.add('inline-flex');
            const badgeText = document.getElementById('footer-update-badge-text');
            if (badgeText) badgeText.textContent = `Update ${data.latest_version}`;
          }
          if (userInitiated) {
            openUpdateDialog(data);
          }
        } else {
          if (updateBtn) updateBtn.classList.add('hidden');
          if (userInitiated) {
            showToast(`You are running the latest version (${data.current_version || 'current'})`, 'success');
            if (checkBtn) {
              checkBtn.innerHTML = `${renderIcon('check', 'w-3.5 h-3.5 text-emerald-400 mr-1.5')}<span>Up to date (${data.current_version || 'latest'})</span>`;
              setTimeout(() => {
                if (checkBtn && originalBtnHTML) checkBtn.innerHTML = originalBtnHTML;
              }, 3000);
            }
          }
        }
      } catch (err) {
        console.error('Update check failed:', err);
        if (userInitiated) {
          showToast(`Update check failed: ${err.message}`, 'error');
          if (checkBtn && originalBtnHTML) checkBtn.innerHTML = originalBtnHTML;
        }
      } finally {
        if (checkBtn && (!latestUpdateInfo || latestUpdateInfo.has_update || latestUpdateInfo.error)) {
          checkBtn.disabled = false;
        } else if (checkBtn && !userInitiated) {
          checkBtn.disabled = false;
        } else if (checkBtn) {
          checkBtn.disabled = false;
        }
      }
    }

    function openUpdateDialog(info) {
      if (!info) return;
      const dialog = document.getElementById('update-dialog');
      if (!dialog) return;
      closeChangelogDialog();

      const tagBadge = document.getElementById('update-tag-badge');
      if (tagBadge) tagBadge.textContent = info.latest_version || 'Latest';
      const curVer = document.getElementById('update-cur-ver');
      if (curVer) curVer.textContent = info.current_version || '--';
      const newVer = document.getElementById('update-new-ver');
      if (newVer) newVer.textContent = info.latest_version || '--';

      const pubDate = info.published_at ? new Date(info.published_at).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) : 'Recent';
      const dateEl = document.getElementById('update-date');
      if (dateEl) dateEl.textContent = `Published: ${pubDate}`;

      const changelogBody = document.getElementById('update-changelog-body');
      if (changelogBody) {
        if (info.release_notes) {
          changelogBody.innerHTML = renderCopilotMarkdown(info.release_notes);
        } else {
          changelogBody.innerHTML = '<p class="text-slate-400 italic">No changelog notes provided for this release.</p>';
        }
      }

      const ghLink = document.getElementById('update-github-link');
      if (ghLink && info.latest_version) {
        ghLink.href = `https://github.com/herliansyah/sqlfox/releases/tag/${encodeURIComponent(info.latest_version)}`;
      }

      // Reset progress & status
      const statusContainer = document.getElementById('update-status-container');
      if (statusContainer) statusContainer.classList.add('hidden');
      const errorMsg = document.getElementById('update-error-msg');
      if (errorMsg) errorMsg.classList.add('hidden');

      const applyBtn = document.getElementById('btn-update-apply');
      if (applyBtn) {
        applyBtn.disabled = false;
        applyBtn.innerHTML = `${renderIcon('zap', 'w-3.5 h-3.5 mr-1.5')}<span>Update Now</span>`;
        if (!info.asset_url) {
          applyBtn.disabled = true;
          applyBtn.title = 'No compatible binary asset found for your OS/Arch';
        }
      }

      if (typeof dialog.showModal === 'function') {
        dialog.showModal();
      }
    }

    function closeUpdateDialog() {
      const dialog = document.getElementById('update-dialog');
      if (dialog && typeof dialog.close === 'function') {
        dialog.close();
      }
    }

    let cachedChangelog = null;

    async function openChangelogDialog() {
      const dialog = document.getElementById('changelog-dialog');
      if (!dialog) return;

      const bodyEl = document.getElementById('changelog-modal-body');
      const badgeEl = document.getElementById('changelog-current-badge');
      const verTextEl = document.getElementById('footer-version-text');

      if (badgeEl && verTextEl) {
        badgeEl.textContent = verTextEl.textContent.replace('SqlFox ', '') || 'v0.1.0';
      }

      if (typeof dialog.showModal === 'function') {
        dialog.showModal();
      }

      if (cachedChangelog && bodyEl) {
        bodyEl.innerHTML = renderCopilotMarkdown(cachedChangelog);
        return;
      }

      try {
        const res = await fetch('/api/system/changelog');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        if (data.changelog && bodyEl) {
          cachedChangelog = data.changelog;
          bodyEl.innerHTML = renderCopilotMarkdown(data.changelog);
          if (data.version && badgeEl) {
            badgeEl.textContent = data.version;
          }
          if (data.version && verTextEl) {
            verTextEl.textContent = `SqlFox ${data.version}`;
          }
          const updateCurVerEl = document.getElementById('update-cur-ver');
          if (data.version && updateCurVerEl) {
            updateCurVerEl.textContent = data.version;
          }
        } else if (bodyEl) {
          bodyEl.innerHTML = '<p class="text-slate-400 italic">No changelog entries found.</p>';
        }
      } catch (err) {
        console.error('Failed to load changelog:', err);
        if (bodyEl) {
          bodyEl.innerHTML = `<p class="text-rose-400">Failed to load changelog: ${escapeHtml(err.message)}</p>`;
        }
      }
    }

    function closeChangelogDialog() {
      const dialog = document.getElementById('changelog-dialog');
      if (dialog && typeof dialog.close === 'function') {
        dialog.close();
      }
    }

    async function performSystemUpdate() {
      if (!latestUpdateInfo || !latestUpdateInfo.asset_url) {
        showToast('No download asset available for this platform.', 'error');
        return;
      }

      const applyBtn = document.getElementById('btn-update-apply');
      const cancelBtn = document.getElementById('btn-update-cancel');
      const statusContainer = document.getElementById('update-status-container');
      const statusLabel = document.getElementById('update-status-label');
      const progressBar = document.getElementById('update-progress-bar');
      const errorMsg = document.getElementById('update-error-msg');

      if (applyBtn) applyBtn.disabled = true;
      if (cancelBtn) cancelBtn.disabled = true;
      if (statusContainer) statusContainer.classList.remove('hidden');
      if (errorMsg) errorMsg.classList.add('hidden');

      if (statusLabel) statusLabel.textContent = 'Downloading release archive...';
      if (progressBar) progressBar.style.width = '35%';

      try {
        const res = await fetch('/api/system/update/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            asset_url: latestUpdateInfo.asset_url,
            version: latestUpdateInfo.latest_version
          })
        });

        const data = await res.json();
        if (!res.ok || data.error) {
          throw new Error(data.error || 'Failed to apply update');
        }

        if (statusLabel) statusLabel.textContent = 'Binary updated! Restarting SqlFox...';
        if (progressBar) {
          progressBar.style.width = '100%';
          progressBar.classList.remove('from-emerald-500', 'to-sky-500');
          progressBar.classList.add('bg-emerald-500');
        }

        // Countdown and reconnect polling
        let countdown = 2;
        let attempts = 0;
        const interval = setInterval(async () => {
          if (countdown > 0) {
            if (statusLabel) statusLabel.textContent = `Restarted! Reconnecting in ${countdown}s...`;
            countdown--;
            return;
          }
          if (statusLabel) statusLabel.textContent = 'Reconnecting to SqlFox...';
          attempts++;
          try {
            const probe = await fetch('/api/auth/status', { cache: 'no-store' });
            if (probe.ok) {
              clearInterval(interval);
              window.location.reload();
              return;
            }
          } catch (_) {
            // Server restarting; continue polling
          }
          if (attempts > 30) {
            clearInterval(interval);
            window.location.reload();
          }
        }, 1000);

      } catch (err) {
        console.error('Update failed:', err);
        if (applyBtn) applyBtn.disabled = false;
        if (cancelBtn) cancelBtn.disabled = false;
        if (errorMsg) {
          errorMsg.textContent = `❌ ${err.message}`;
          errorMsg.classList.remove('hidden');
        }
        if (statusLabel) statusLabel.textContent = 'Update halted';
        if (progressBar) progressBar.classList.add('bg-rose-500');
      }
    }

    // Auto-check for updates 2 seconds after page load
    function startAutoUpdateCheck() {
      setTimeout(() => {
        checkOrOpenUpdate(false);
      }, 2000);
    }

    // --- Activity Console & Live Event Stream ---
    // ponytail: Zero-dependency real-time telemetry streaming via native browser EventSource and in-memory table buffer
    let activityLogs = [];
    let activityEventSource = null;
    let activityPaused = false;
    let activityLevelFilter = 'ALL';
    let activitySearchQuery = '';
    let activityAutoScroll = true;
    let activityUnreadErrors = 0;
    let activityConsoleOpen = false;

    function initActivityConsole() {
      connectActivityEventSource();
      // Keyboard shortcut: Ctrl + ` (backtick)
      window.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === '`') {
          e.preventDefault();
          toggleActivityConsole();
        }
      });
      // Close export menu on click outside
      document.addEventListener('click', (e) => {
        const menu = document.getElementById('activity-export-menu');
        if (menu && !menu.classList.contains('hidden') && !e.target.closest('#activity-export-menu') && !e.target.closest('button[onclick*="toggleActivityExportMenu"]')) {
          menu.classList.add('hidden');
        }
      });
    }

    function connectActivityEventSource() {
      if (activityEventSource) return;
      try {
        activityEventSource = new EventSource('/api/logs/stream');
        const dot = document.getElementById('footer-activity-dot');
        const statusEl = document.getElementById('activity-live-status');

        activityEventSource.onopen = () => {
          if (dot) {
            dot.className = 'inline-block w-2 h-2 rounded-full bg-emerald-500 animate-pulse';
            dot.title = 'Live Event Stream connected';
          }
          if (statusEl && !activityPaused) {
            statusEl.textContent = '● LIVE';
            statusEl.className = 'inline-flex items-center px-1.5 py-0.2 rounded text-[10px] font-mono bg-emerald-950/80 text-emerald-400 border border-emerald-700/50';
          }
        };

        activityEventSource.onmessage = (e) => {
          if (!e.data) return;
          try {
            const evt = JSON.parse(e.data);
            handleIncomingActivityEvent(evt);
          } catch (err) {
            console.error('Failed to parse SSE log record:', err);
          }
        };

        activityEventSource.onerror = () => {
          if (dot) {
            dot.className = 'inline-block w-2 h-2 rounded-full bg-amber-500';
            dot.title = 'Reconnecting log stream...';
          }
          if (statusEl && !activityPaused) {
            statusEl.textContent = '● RECONNECTING';
            statusEl.className = 'inline-flex items-center px-1.5 py-0.2 rounded text-[10px] font-mono bg-amber-950/80 text-amber-400 border border-amber-700/50';
          }
        };
      } catch (err) {
        console.error('EventSource initialization error:', err);
      }
    }

    function handleIncomingActivityEvent(evt) {
      if (!evt) return;
      // FIFO ring buffer: keep max 1000 items in browser memory
      if (activityLogs.length >= 1000) {
        activityLogs.shift();
      }
      activityLogs.push(evt);

      // Track unread errors if drawer is closed
      const lvl = (evt.level || '').toUpperCase();
      if (!activityConsoleOpen && (lvl === 'ERROR' || lvl === 'WARN' || lvl === 'WARNING')) {
        activityUnreadErrors++;
        updateActivityErrorBadge();
      }

      // Do not waste CPU/memory rendering DOM when drawer is closed or paused
      if (activityPaused || !activityConsoleOpen) {
        return;
      }

      scheduleActivityRender();
    }

    let activityRenderScheduled = false;
    function scheduleActivityRender() {
      if (activityRenderScheduled || !activityConsoleOpen || activityPaused) return;
      activityRenderScheduled = true;
      requestAnimationFrame(() => {
        activityRenderScheduled = false;
        renderActivityLogs();
      });
    }

    function updateActivityErrorBadge() {
      const badge = document.getElementById('footer-activity-err-badge');
      if (!badge) return;
      if (activityUnreadErrors > 0) {
        badge.textContent = activityUnreadErrors > 99 ? '99+' : activityUnreadErrors;
        badge.classList.remove('hidden');
      } else {
        badge.classList.add('hidden');
      }
    }

    function toggleActivityConsole(forceState) {
      const drawer = document.getElementById('activity-console-drawer');
      if (!drawer) return;
      activityConsoleOpen = typeof forceState === 'boolean' ? forceState : drawer.classList.contains('hidden');

      if (activityConsoleOpen) {
        drawer.classList.remove('hidden');
        activityUnreadErrors = 0;
        updateActivityErrorBadge();
        renderActivityLogs();
        if (activityAutoScroll) {
          scrollActivityToBottom();
        }
      } else {
        drawer.classList.add('hidden');
      }
    }

    function toggleActivityPause() {
      activityPaused = !activityPaused;
      const btnIcon = document.getElementById('activity-pause-icon');
      const btnLabel = document.getElementById('activity-pause-label');
      const statusEl = document.getElementById('activity-live-status');

      if (activityPaused) {
        if (btnIcon) btnIcon.innerHTML = renderIcon('play', 'w-3 h-3');
        if (btnLabel) btnLabel.textContent = 'Resume';
        if (statusEl) {
          statusEl.innerHTML = `${renderIcon('pause', 'w-2.5 h-2.5 inline mr-1')} PAUSED`;
          statusEl.className = 'inline-flex items-center px-1.5 py-0.2 rounded text-[10px] font-mono bg-amber-950/80 text-amber-400 border border-amber-700/50';
        }
      } else {
        if (btnIcon) btnIcon.innerHTML = renderIcon('pause', 'w-3 h-3');
        if (btnLabel) btnLabel.textContent = 'Pause';
        if (statusEl) {
          statusEl.innerHTML = '<span class="w-1.5 h-1.5 rounded-full bg-emerald-400 inline-block mr-1 animate-pulse"></span> LIVE';
          statusEl.className = 'inline-flex items-center px-1.5 py-0.2 rounded text-[10px] font-mono bg-emerald-950/80 text-emerald-400 border border-emerald-700/50';
        }
        renderActivityLogs();
        if (activityAutoScroll) {
          scrollActivityToBottom();
        }
      }
    }

    function setActivityLevelFilter(level) {
      activityLevelFilter = level;
      const levels = ['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG'];
      levels.forEach(lvl => {
        const btn = document.getElementById(`activity-filter-${lvl.toLowerCase()}`);
        if (!btn) return;
        if (lvl === level) {
          btn.className = 'px-2 py-0.5 rounded font-mono transition bg-sky-600 text-white font-medium';
        } else {
          btn.className = 'px-2 py-0.5 rounded font-mono transition text-slate-400 hover:text-white';
        }
      });
      renderActivityLogs();
    }

    function onActivitySearchInput(val) {
      activitySearchQuery = (val || '').toLowerCase().trim();
      renderActivityLogs();
    }

    function clearActivityLogs() {
      activityLogs = [];
      activityUnreadErrors = 0;
      updateActivityErrorBadge();
      renderActivityLogs();
    }

    function handleActivityScroll() {
      const container = document.getElementById('activity-table-container');
      if (!container) return;
      // Smart Auto-Scroll: if user scrolled away from bottom, pause auto-scroll
      const isAtBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 30;
      activityAutoScroll = isAtBottom;
    }

    function scrollActivityToBottom() {
      const container = document.getElementById('activity-table-container');
      if (container) {
        container.scrollTop = container.scrollHeight;
      }
    }

    function renderActivityLogs() {
      const tbody = document.getElementById('activity-table-body');
      const emptyState = document.getElementById('activity-empty-state');
      if (!tbody) return;

      const filtered = activityLogs.filter(item => {
        // Level filter
        if (activityLevelFilter !== 'ALL') {
          const itemLvl = (item.level || '').toUpperCase();
          if (itemLvl !== activityLevelFilter) {
            if (!(activityLevelFilter === 'WARN' && itemLvl === 'WARNING')) {
              return false;
            }
          }
        }
        // Search query
        if (activitySearchQuery) {
          const msg = (item.message || '').toLowerCase();
          const mod = (item.module || '').toLowerCase();
          const attrsStr = item.attrs ? JSON.stringify(item.attrs).toLowerCase() : '';
          if (!msg.includes(activitySearchQuery) && !mod.includes(activitySearchQuery) && !attrsStr.includes(activitySearchQuery)) {
            return false;
          }
        }
        return true;
      });

      if (filtered.length === 0) {
        tbody.innerHTML = '';
        if (emptyState) emptyState.classList.remove('hidden');
        return;
      }

      if (emptyState) emptyState.classList.add('hidden');

      const rows = filtered.map(item => {
        const timeStr = formatActivityTime(item.timestamp);
        const lvlBadge = getActivityLevelBadge(item.level);
        const modBadge = getActivityModuleBadge(item.module);
        const metaStr = getActivityMetaString(item);
        const hasDetails = item.attrs && Object.keys(item.attrs).length > 0;
        const rowId = `act-row-${item.id}`;

        let detailRow = '';
        if (hasDetails) {
          detailRow = `
            <tr id="${rowId}-detail" class="hidden bg-slate-900/60 font-mono text-[10px] text-slate-400">
              <td colspan="5" class="py-1.5 px-4">
                <div class="p-2 bg-slate-950 rounded border border-slate-800 overflow-x-auto whitespace-pre-wrap select-text">${escapeHtml(JSON.stringify(item.attrs, null, 2))}</div>
              </td>
            </tr>
          `;
        }

        return `
          <tr class="hover:bg-slate-900/40 transition cursor-pointer group" onclick="toggleActivityRowDetail('${rowId}-detail')">
            <td class="py-1 px-3 text-slate-400 whitespace-nowrap">${escapeHtml(timeStr)}</td>
            <td class="py-1 px-2 whitespace-nowrap">${lvlBadge}</td>
            <td class="py-1 px-2 whitespace-nowrap">${modBadge}</td>
            <td class="py-1 px-3 text-slate-200 group-hover:text-white break-all leading-snug">${escapeHtml(item.message || '')}</td>
            <td class="py-1 px-3 text-right text-slate-400 whitespace-nowrap">${metaStr} ${hasDetails ? '<svg class="w-3 h-3 text-slate-500 inline shrink-0"><use href="#icon-chevron-right"/></svg>' : ''}</td>
          </tr>
          ${detailRow}
        `;
      }).join('');

      tbody.innerHTML = rows;

      if (activityAutoScroll) {
        scrollActivityToBottom();
      }
    }

    function toggleActivityRowDetail(id) {
      const el = document.getElementById(id);
      if (el) {
        el.classList.toggle('hidden');
      }
    }

    function formatActivityTime(ts) {
      if (!ts) return '--:--:--.---';
      const d = new Date(ts);
      if (isNaN(d.getTime())) return ts;
      const pad = (n, s=2) => String(n).padStart(s, '0');
      return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
    }

    function getActivityLevelBadge(lvl) {
      const u = (lvl || '').toUpperCase();
      switch (u) {
        case 'ERROR':
          return '<span class="px-1.5 py-0.2 rounded text-[10px] font-bold bg-rose-950 text-rose-400 border border-rose-800/60">ERROR</span>';
        case 'WARN':
        case 'WARNING':
          return '<span class="px-1.5 py-0.2 rounded text-[10px] font-bold bg-amber-950 text-amber-400 border border-amber-800/60">WARN</span>';
        case 'DEBUG':
          return '<span class="px-1.5 py-0.2 rounded text-[10px] font-bold bg-purple-950 text-purple-400 border border-purple-800/60">DEBUG</span>';
        default:
          return '<span class="px-1.5 py-0.2 rounded text-[10px] font-bold bg-emerald-950 text-emerald-400 border border-emerald-800/60">INFO</span>';
      }
    }

    function getActivityModuleBadge(mod) {
      const m = (mod || 'app').toLowerCase();
      let color = 'text-slate-400 bg-slate-900 border-slate-700/60';
      if (m === 'query') color = 'text-sky-400 bg-sky-950/60 border-sky-800/60';
      else if (m === 'http') color = 'text-teal-400 bg-teal-950/60 border-teal-800/60';
      else if (m === 'schema') color = 'text-indigo-400 bg-indigo-950/60 border-indigo-800/60';
      else if (m === 'ai') color = 'text-pink-400 bg-pink-950/60 border-pink-800/60';
      else if (m === 'auth') color = 'text-amber-400 bg-amber-950/60 border-amber-800/60';
      else if (m === 'sync') color = 'text-emerald-400 bg-emerald-950/60 border-emerald-800/60';

      return `<span class="px-1.5 py-0.2 rounded text-[10px] font-medium border ${color}">${escapeHtml(m)}</span>`;
    }

    function getActivityMetaString(item) {
      if (!item.attrs) return '';
      const parts = [];
      if (item.attrs.duration_ms !== undefined) {
        parts.push(`${item.attrs.duration_ms}ms`);
      }
      if (item.attrs.rows !== undefined) {
        parts.push(`${item.attrs.rows} rows`);
      }
      if (item.attrs.status !== undefined) {
        parts.push(`HTTP ${item.attrs.status}`);
      }
      return parts.join(' • ');
    }

    function toggleActivityExportMenu() {
      const menu = document.getElementById('activity-export-menu');
      if (menu) menu.classList.toggle('hidden');
    }

    function exportActivityLogs(format) {
      const menu = document.getElementById('activity-export-menu');
      if (menu) menu.classList.add('hidden');

      if (activityLogs.length === 0) {
        showNativeAlert('No logs to export.', 'Export Logs');
        return;
      }

      let content = '';
      let filename = `sqlfox-activity-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}`;

      if (format === 'json') {
        content = JSON.stringify(activityLogs, null, 2);
        filename += '.json';
      } else {
        content = activityLogs.map(item => {
          const ts = formatActivityTime(item.timestamp);
          const lvl = (item.level || 'INFO').toUpperCase().padEnd(5, ' ');
          const mod = (item.module || 'app').padEnd(8, ' ');
          const attrsStr = item.attrs ? ' ' + JSON.stringify(item.attrs) : '';
          return `[${ts}] [${lvl}] [${mod}] ${item.message || ''}${attrsStr}`;
        }).join('\n');
        filename += '.log';
      }

      const blob = new Blob([content], { type: format === 'json' ? 'application/json' : 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    }