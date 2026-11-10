
    // --- Canonical Vector Icon Helper (ADR 0037 Standardization) ---
    function renderIcon(name, extraClasses = 'w-4 h-4') {
      return `<svg class="${extraClasses} inline-block shrink-0 fill-none stroke-current" aria-hidden="true"><use href="#icon-${name}"/></svg>`;
    }

    // --- Application State ---
    let connections = [];
    let activeConnection = null;
    let activeSession = null;
    let cmEditor = null;
    let schemaCatalogCache = null;
    let tabs = [
      { id: 'tab-1', title: 'Query 1.sql', sql: '', isTableViewer: false }
    ];
    let activeTabId = 'tab-1';
    let currentResults = [];
    let currentColumns = [];
    let pendingDestructiveSQL = null;
    let pendingDestructiveResult = null;
    let lastKeysetKey = null;
    let isQueryExecuting = false;

    // Table CRUD & Spreadsheet State
    let selectedRowIndices = new Set();
    let stagedDraftRows = [];
    let activeGridRightClickedRow = null;
    let activeGridRightClickedCol = null;
    let activeGridRightClickedTd = null;
    let activeGridSelectedRow = null;
    let activeGridSelectedCol = null;
    let activeGridSelectedTd = null;

    // Table Designer & Modal State
    let currentTableMetadata = null;
    let stagedStructure = null;
    let activeTableMode = 'data';
    let newTableColumns = [];

    // Advanced Diagnostics, Profiling & Privilege Matrix State
    let enableQueryProfiling = false;
    let lastQueryProfile = null;
    let activePrivUser = null;
    let allPrivUsers = [];
    let dbDiagTables = [];
    let dbDiagSelected = new Set();

    function formatBytes(bytes) {
      if (!bytes || isNaN(bytes) || bytes === 0) return '0 B';
      const k = 1024;
      const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    // --- Native Dialog Helper Functions (HTML5 <dialog>) ---
    function showNativeAlert(message, title = 'SqlFox Notification') {
      return new Promise((resolve) => {
        const dlg = document.getElementById('app-native-dialog');
        if (!dlg) {
          window.alert(message);
          return resolve(true);
        }
        document.getElementById('app-native-dialog-title').textContent = title;
        document.getElementById('app-native-dialog-body').textContent = message;
        const inputContainer = document.getElementById('app-native-dialog-input-container');
        if (inputContainer) inputContainer.classList.add('hidden');
        const cancelBtn = document.getElementById('app-native-dialog-cancel');
        const confirmBtn = document.getElementById('app-native-dialog-confirm');
        cancelBtn.classList.add('hidden');
        confirmBtn.textContent = 'OK';
        confirmBtn.className = 'bg-app-accent hover:bg-sky-400 text-slate-950 font-bold px-4 py-1.5 rounded text-xs transition';

        const onConfirm = () => {
          confirmBtn.removeEventListener('click', onConfirm);
          dlg.close();
          resolve(true);
        };
        confirmBtn.addEventListener('click', onConfirm, { once: true });
        dlg.onclose = () => resolve(true);
        if (typeof dlg.showModal === 'function') dlg.showModal();
        else dlg.setAttribute('open', '');
      });
    }

    function showNativeConfirm(message, title = 'Confirm Action') {
      return new Promise((resolve) => {
        const dlg = document.getElementById('app-native-dialog');
        if (!dlg) {
          return resolve(window.confirm(message));
        }
        document.getElementById('app-native-dialog-title').textContent = title;
        document.getElementById('app-native-dialog-body').textContent = message;
        const inputContainer = document.getElementById('app-native-dialog-input-container');
        if (inputContainer) inputContainer.classList.add('hidden');
        const cancelBtn = document.getElementById('app-native-dialog-cancel');
        const confirmBtn = document.getElementById('app-native-dialog-confirm');
        cancelBtn.classList.remove('hidden');
        confirmBtn.textContent = 'Confirm';
        confirmBtn.className = 'bg-app-danger hover:bg-red-600 text-white font-bold px-4 py-1.5 rounded text-xs transition';

        let resolved = false;
        const cleanup = () => {
          confirmBtn.removeEventListener('click', onConfirm);
          cancelBtn.removeEventListener('click', onCancel);
        };
        const onConfirm = () => {
          if (resolved) return;
          resolved = true;
          cleanup();
          dlg.close();
          resolve(true);
        };
        const onCancel = () => {
          if (resolved) return;
          resolved = true;
          cleanup();
          dlg.close();
          resolve(false);
        };
        confirmBtn.addEventListener('click', onConfirm);
        cancelBtn.addEventListener('click', onCancel);
        dlg.onclose = () => {
          if (!resolved) {
            resolved = true;
            cleanup();
            resolve(false);
          }
        };
        if (typeof dlg.showModal === 'function') dlg.showModal();
        else dlg.setAttribute('open', '');
      });
    }

    function showNativePrompt(message, defaultValue = '', title = 'Input') {
      return new Promise((resolve) => {
        const dlg = document.getElementById('app-native-dialog');
        if (!dlg) {
          return resolve(window.prompt(message, defaultValue));
        }
        document.getElementById('app-native-dialog-title').textContent = title;
        document.getElementById('app-native-dialog-body').textContent = message;
        const inputContainer = document.getElementById('app-native-dialog-input-container');
        const input = document.getElementById('app-native-dialog-input');
        if (inputContainer && input) {
          inputContainer.classList.remove('hidden');
          input.value = defaultValue || '';
        }
        const cancelBtn = document.getElementById('app-native-dialog-cancel');
        const confirmBtn = document.getElementById('app-native-dialog-confirm');
        cancelBtn.classList.remove('hidden');
        confirmBtn.textContent = 'Save';
        confirmBtn.className = 'bg-app-accent hover:bg-sky-400 text-slate-950 font-bold px-4 py-1.5 rounded text-xs transition';

        let resolved = false;
        const cleanup = () => {
          confirmBtn.removeEventListener('click', onConfirm);
          cancelBtn.removeEventListener('click', onCancel);
          if (input) input.removeEventListener('keydown', onKeyDown);
          if (inputContainer) inputContainer.classList.add('hidden');
        };
        const onConfirm = () => {
          if (resolved) return;
          resolved = true;
          const val = input ? input.value : '';
          cleanup();
          dlg.close();
          resolve(val);
        };
        const onCancel = () => {
          if (resolved) return;
          resolved = true;
          cleanup();
          dlg.close();
          resolve(null);
        };
        const onKeyDown = (e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            onConfirm();
          } else if (e.key === 'Escape') {
            e.preventDefault();
            onCancel();
          }
        };
        confirmBtn.addEventListener('click', onConfirm);
        cancelBtn.addEventListener('click', onCancel);
        if (input) input.addEventListener('keydown', onKeyDown);
        dlg.onclose = () => {
          if (!resolved) {
            resolved = true;
            cleanup();
            resolve(null);
          }
        };
        if (typeof dlg.showModal === 'function') dlg.showModal();
        else dlg.setAttribute('open', '');
        if (input) {
          setTimeout(() => { input.focus(); input.select(); }, 50);
        }
      });
    }

    // Override browser window.alert to automatically use native <dialog>
    const _origAlert = window.alert;
    window.alert = function(msg) {
      showNativeAlert(String(msg));
    };

    function openDDLReviewModal(sql, onConfirm) {
      const dlg = document.getElementById('ddl-review-dialog');
      if (!dlg) return;
      document.getElementById('ddl-review-sql').textContent = sql;
      const confirmBtn = document.getElementById('ddl-review-confirm-btn');
      const cancelBtns = dlg.querySelectorAll('button:not(#ddl-review-confirm-btn)');

      const newBtn = confirmBtn.cloneNode(true);
      newBtn.disabled = false;
      newBtn.classList.remove('opacity-50', 'cursor-not-allowed');
      newBtn.innerHTML = 'Execute DDL';
      cancelBtns.forEach(b => { b.disabled = false; b.classList.remove('opacity-50', 'cursor-not-allowed'); });
      confirmBtn.parentNode.replaceChild(newBtn, confirmBtn);

      newBtn.onclick = async () => {
        // ponytail: prevent duplicate DDL execution clicks and lock UI with timer
        if (newBtn.disabled) return;
        newBtn.disabled = true;
        newBtn.classList.add('opacity-50', 'cursor-not-allowed');
        cancelBtns.forEach(b => { b.disabled = true; b.classList.add('opacity-50', 'cursor-not-allowed'); });

        const ddlStart = Date.now();
        newBtn.innerHTML = `${renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5')} Executing DDL (<span id="ddl-exec-timer">0.0s</span>)...`;
        const timerInterval = setInterval(() => {
          const el = document.getElementById('ddl-exec-timer');
          if (el) el.textContent = ((Date.now() - ddlStart) / 1000).toFixed(1) + 's';
        }, 100);

        try {
          if (typeof onConfirm === 'function') {
            await onConfirm();
          }
        } finally {
          clearInterval(timerInterval);
          newBtn.disabled = false;
          newBtn.classList.remove('opacity-50', 'cursor-not-allowed');
          newBtn.innerHTML = 'Execute DDL';
          cancelBtns.forEach(b => { b.disabled = false; b.classList.remove('opacity-50', 'cursor-not-allowed'); });
          dlg.close();
        }
      };
      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    async function executeDDLStatements(ddlSQL, successMessage = 'DDL executed successfully.', skipSchemaRefresh = false) {
      if (!activeConnection) {
        showNativeAlert('No active database connection.', 'Execution Error');
        return false;
      }
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/execute`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sql: ddlSQL,
            confirmed: true,
            challenge_input: ''
          })
        });
        const data = await res.json();
        if (!res.ok) {
          if (res.status === 422 && data.blocked) {
            openSafetyModal(ddlSQL, data.inspection);
            return false;
          }
          showNativeAlert('DDL execution failed:\n' + (data.error || res.statusText), 'Execution Error');
          return false;
        }
        if (data.backup_file || data.backup_warning) {
          showNativeAlert(successMessage + (data.backup_file ? `\n\nBackup snapshot created:\n${data.backup_file}` : '') + (data.backup_warning ? `\n\nWARNING: ${data.backup_warning}` : ''), 'Success');
        } else {
          showToast(successMessage, 'success');
        }
        if (!skipSchemaRefresh) {
          await refreshSchema();
        }
        return true;
      } catch (err) {
        showNativeAlert('Error executing DDL: ' + err.message, 'Execution Error');
        return false;
      }
    }

    // AI Settings State (stored in localStorage)
    // Corrupt/legacy localStorage must not abort the whole script (every global handler is defined below).
    function readStoredJSON(key, fallback) {
      try {
        const raw = localStorage.getItem(key);
        if (raw == null) return fallback;
        const parsed = JSON.parse(raw);
        return (parsed !== null && typeof parsed === typeof fallback && Array.isArray(parsed) === Array.isArray(fallback)) ? parsed : fallback;
      } catch (_) {
        return fallback;
      }
    }
    let aiConfig = readStoredJSON('sqlfox_ai_config', {"provider":"openai","model":"gpt-4o-mini","apiKey":"","endpoint":"","language":"id","responseStyle":"concise","customPrompt":"","includeSchema":true,"maskIdentifiers":true});
    let currentChatSessionId = null;
    let chatSessions = readStoredJSON('sqlfox_ai_sessions', []);

    // --- Global Network Error Boundary & Health Watcher (Milestone 26) ---
    let isServerReconnecting = false;
    let reconnectAttempts = 0;
    let reconnectTimer = null;

    function showReconnectBanner(countdown) {
      let banner = document.getElementById('global-reconnect-banner');
      if (!banner) {
        banner = document.createElement('div');
        banner.id = 'global-reconnect-banner';
        banner.className = 'fixed top-3 left-1/2 -translate-x-1/2 z-[9999] bg-amber-500/95 text-slate-950 font-semibold px-4 py-2 rounded-lg shadow-2xl backdrop-blur flex items-center space-x-2 text-xs border border-amber-300 transition-all duration-200';
        document.body.appendChild(banner);
      }
      banner.className = 'fixed top-3 left-1/2 -translate-x-1/2 z-[9999] bg-amber-500/95 text-slate-950 font-semibold px-4 py-2 rounded-lg shadow-2xl backdrop-blur flex items-center space-x-2 text-xs border border-amber-300 transition-all duration-200 animate-pulse';
      banner.innerHTML = `<span class="inline-flex items-center gap-1.5">${renderIcon('alert-triangle', 'w-4 h-4 text-amber-400')}<span>Connection lost. Reconnecting in ${countdown}s...</span></span>`;
      banner.classList.remove('hidden');
    }

    function hideReconnectBanner() {
      const banner = document.getElementById('global-reconnect-banner');
      if (banner && !banner.classList.contains('hidden')) {
        banner.className = 'fixed top-3 left-1/2 -translate-x-1/2 z-[9999] bg-emerald-500/95 text-slate-950 font-semibold px-4 py-2 rounded-lg shadow-2xl backdrop-blur flex items-center space-x-2 text-xs border border-emerald-300 transition-all duration-200';
        banner.innerHTML = `<span class="inline-flex items-center gap-1.5">${renderIcon('check', 'w-4 h-4 text-emerald-400')}<span>Connection restored</span></span>`;
        setTimeout(() => {
          banner.classList.add('hidden');
        }, 1500);
      }
      isServerReconnecting = false;
      reconnectAttempts = 0;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (activeConnection && typeof pingActiveSession === 'function') {
        pingActiveSession();
      }
    }

    function triggerAutoReconnect() {
      if (isServerReconnecting) return;
      isServerReconnecting = true;
      reconnectAttempts = 0;

      const scheduleNext = () => {
        reconnectAttempts++;
        const delaySec = Math.min(Math.pow(2, reconnectAttempts - 1), 8);
        showReconnectBanner(delaySec);

        reconnectTimer = setTimeout(async () => {
          try {
            const probe = await fetch('/api/auth/status', { cache: 'no-store' });
            if (probe.ok) {
              hideReconnectBanner();
              return;
            }
          } catch (_) {}
          scheduleNext();
        }, delaySec * 1000);
      };

      scheduleNext();
    }

    // --- Remote Access Guard & API Fetch Helper ---
    // ponytail: global top progress bar disabled to eliminate visual noise; all modules utilize localized in-place spinners
    let globalInFlightRequests = 0;
    function startGlobalProgress() {
      globalInFlightRequests++;
    }

    function stopGlobalProgress() {
      globalInFlightRequests = Math.max(0, globalInFlightRequests - 1);
    }

    async function apiFetch(url, options = {}) {
      options.headers = options.headers || {};
      const token = sessionStorage.getItem('sqlfox_remote_token');
      if (token && !options.headers['Authorization']) {
        options.headers['Authorization'] = `Bearer ${token}`;
      }

      // ponytail: fail-fast timeout on idle/dead sockets unless caller explicitly configures signal or timeout
      if (!options.signal) {
        const isLongRunning = url.includes('/execute') || url.includes('/dump') || url.includes('/import') || url.includes('/sync') || url.includes('/transfer');
        const timeoutMs = options.timeout !== undefined ? options.timeout : (isLongRunning ? 0 : 15000);
        if (timeoutMs > 0 && typeof AbortSignal !== 'undefined' && typeof AbortSignal.timeout === 'function') {
          options.signal = AbortSignal.timeout(timeoutMs);
        }
      }

      const isSilent = !!(options.silent || url.includes('/api/auth/status') || url.includes('/api/ping') || url.includes('/flight-recorder/status') || url.includes('/fast-catalog'));
      if (!isSilent) startGlobalProgress();

      try {
        const res = await fetch(url, options);
        if (res.status === 401) {
          showRemoteLockScreen();
        } else if (res.status === 403 && typeof activeSession !== 'undefined' && activeSession && !activeSession.read_only) {
          try {
            res.clone().json().then(body => {
              if (body && typeof body.error === 'string' && body.error.toLowerCase().includes('read-only')) {
                activeSession.read_only = true;
                if (typeof updateReadOnlyUI === 'function') updateReadOnlyUI(true);
              }
            }).catch(() => {});
          } catch (_) {}
        } else if (isServerReconnecting && res.ok) {
          hideReconnectBanner();
        }
        return res;
      } catch (err) {
        // ponytail: only trigger global reconnect banner for real network/daemon drops, not aborted non-critical timeouts
        if (!isSilent && err && err.name !== 'AbortError' && err.name !== 'TimeoutError') {
          triggerAutoReconnect();
        }
        throw err;
      } finally {
        if (!isSilent) stopGlobalProgress();
      }
    }

    async function checkAuthStatus() {
      try {
        const res = await apiFetch('/api/auth/status');
        const data = await res.json();
        if (data.is_remote && !data.unlocked) {
          showRemoteLockScreen();
          return false;
        } else {
          document.body.classList.remove('vault-locked');
        }
      } catch (err) {
        console.warn('Auth status check skipped:', err);
      }
      return true;
    }

    function showRemoteLockScreen() {
      document.body.classList.add('vault-locked');
      const modal = document.getElementById('remote-lock-modal');
      if (modal) modal.classList.remove('hidden');
      const passInput = document.getElementById('remote-unlock-password');
      const errorEl = document.getElementById('remote-unlock-error');
      if (errorEl) {
        errorEl.classList.add('hidden');
        errorEl.textContent = '';
      }
      if (passInput) {
        passInput.classList.remove('border-red-500', 'focus:border-red-500');
        passInput.value = '';
        setTimeout(() => passInput.focus(), 100);
      }
    }

    async function unlockRemoteVault(e) {
      e.preventDefault();
      const passInput = document.getElementById('remote-unlock-password');
      const errorEl = document.getElementById('remote-unlock-error');
      const submitBtn = document.getElementById('remote-unlock-btn') || (e.target && e.target.querySelector('button[type="submit"]'));
      const pass = passInput ? passInput.value : '';

      if (errorEl) {
        errorEl.classList.add('hidden');
        errorEl.textContent = '';
      }
      if (passInput) {
        passInput.classList.remove('border-red-500', 'focus:border-red-500');
      }
      if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.classList.add('opacity-70', 'cursor-not-allowed');
      }

      try {
        const res = await fetch('/api/auth/unlock', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ password: pass })
        });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.token) {
          if (passInput) passInput.value = '';
          sessionStorage.setItem('sqlfox_remote_token', data.token);
          // ponytail: sync token to cookie so browser page reloads from LAN do not re-lock the screen
          document.cookie = 'sqlfox_token=' + encodeURIComponent(data.token) + '; path=/; SameSite=Lax';
          await initAppWorkspace();
        } else {
          const errMsg = (data && data.error) ? data.error : 'Incorrect master password!';
          if (errorEl) {
            errorEl.textContent = errMsg;
            errorEl.classList.remove('hidden');
          } else {
            showNativeAlert(errMsg, 'Authentication');
          }
          if (passInput) {
            passInput.classList.add('border-red-500', 'focus:border-red-500');
            passInput.select();
            passInput.focus();
          }
        }
      } catch (err) {
        const errMsg = 'Unlock failed: ' + (err && err.message ? err.message : 'Network error');
        if (errorEl) {
          errorEl.textContent = errMsg;
          errorEl.classList.remove('hidden');
        } else {
          showNativeAlert(errMsg, 'Authentication');
        }
        if (passInput) {
          passInput.select();
          passInput.focus();
        }
      } finally {
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.classList.remove('opacity-70', 'cursor-not-allowed');
        }
      }
    }

    let cachedMasterPIN = '';
    let cachedLANIP = '';
    let isLANEnabled = false;
    let masterPINVisible = false;
    let masterPINAutoMaskTimer = null;

    const PIN_EYE_SVG = '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path><circle cx="12" cy="12" r="3"></circle>';
    const PIN_EYE_OFF_SVG = '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path><line x1="1" y1="1" x2="23" y2="23"></line>';

    function updateMasterPINDisplay() {
      const pinTextEl = document.getElementById('master-pin-text');
      const eyeIconEl = document.getElementById('master-pin-eye-icon');
      const toggleBtn = document.getElementById('master-pin-toggle-btn');
      if (!pinTextEl) return;

      if (masterPINVisible) {
        pinTextEl.textContent = cachedMasterPIN;
        pinTextEl.classList.remove('tracking-widest');
        if (eyeIconEl) eyeIconEl.innerHTML = PIN_EYE_OFF_SVG;
        if (toggleBtn) toggleBtn.title = 'Hide Master PIN';
      } else {
        const maskLen = (cachedMasterPIN && cachedMasterPIN.length) || 6;
        pinTextEl.textContent = '•'.repeat(maskLen);
        pinTextEl.classList.add('tracking-widest');
        if (eyeIconEl) eyeIconEl.innerHTML = PIN_EYE_SVG;
        if (toggleBtn) toggleBtn.title = 'Peek Master PIN (auto-hides in 10s)';
      }
    }

    // ponytail: 10s auto-mask timeout prevents shoulder-surfing if screen is left unattended
    function toggleMasterPINVisibility(e) {
      if (e) e.stopPropagation();
      masterPINVisible = !masterPINVisible;
      if (masterPINAutoMaskTimer) {
        clearTimeout(masterPINAutoMaskTimer);
        masterPINAutoMaskTimer = null;
      }
      if (masterPINVisible) {
        masterPINAutoMaskTimer = setTimeout(() => {
          masterPINVisible = false;
          updateMasterPINDisplay();
        }, 10000);
      }
      updateMasterPINDisplay();
    }

    async function loadMasterPIN() {
      try {
        const res = await apiFetch('/api/auth/master-key');
        if (res.ok) {
          const data = await res.json();
          isLANEnabled = !!data.lan_enabled;
          cachedLANIP = data.lan_ip || '';
          if (data.master_password) {
            cachedMasterPIN = data.master_password;
            masterPINVisible = false;
            updateMasterPINDisplay();
            const badgeEl = document.getElementById('master-pin-badge');
            if (badgeEl) badgeEl.classList.remove('hidden');
          }
          renderLANStatusInSettings();
        }
      } catch (err) {
        // Restricted if remote, ignore silently
      }
    }

    function renderLANStatusInSettings() {
      const tabletBtn = document.getElementById('btn-copy-tablet-lan');
      const lanHint = document.getElementById('lan-status-hint');
      if (isLANEnabled) {
        if (tabletBtn) tabletBtn.classList.remove('hidden');
        if (lanHint) {
          const proto = window.location.protocol || 'http:';
          const defaultPort = proto === 'https:' ? '443' : '80';
          const port = window.location.port || defaultPort;
          const ip = cachedLANIP || window.location.hostname;
          const pinText = cachedMasterPIN ? escapeHtml(cachedMasterPIN) : 'configured';
          lanHint.innerHTML = `<span class="text-emerald-400 font-semibold">● LAN Access Active:</span> Connect tablet/phone at <span class="font-mono text-amber-200">${proto}//${ip}:${port}</span> (Master PIN: <span class="font-mono text-sky-300 font-bold">${pinText}</span>)`;
        }
      } else {
        if (tabletBtn) tabletBtn.classList.add('hidden');
        if (lanHint) {
          lanHint.innerHTML = `<span class="text-slate-400">ℹ️ <b>LAN Access: Disabled (Localhost Only).</b> To allow tablet/remote connections, start with <code class="text-slate-200">--lan</code>, install desktop with <code class="text-slate-200">sqlfox desktop install --lan</code>, or set <code class="text-slate-200">SQLFOX_LAN=1</code> in <code class="text-slate-200">sqlfox.env</code>.</span>`;
        }
      }
    }

    async function copyTabletLANURL() {
      const proto = window.location.protocol || 'http:';
      const defaultPort = proto === 'https:' ? '443' : '80';
      const port = window.location.port || defaultPort;
      const ip = cachedLANIP || window.location.hostname;
      const lanURL = `${proto}//${ip}:${port}`;
      const ok = await copyTextToClipboard(lanURL);
      if (ok) {
        showToast(`Tablet LAN URL copied: ${lanURL} (Master PIN: ${cachedMasterPIN || 'required'})`, 'success');
      } else {
        prompt('Tablet / LAN URL:', lanURL);
      }
    }

    // ponytail: unified clipboard copy with execCommand fallback for unsecure HTTP / LAN IP contexts and modal dialogs.
    // Known ceiling: execCommand is deprecated in current W3C specs, but is the only functional clipboard API on unsecure contexts. Upgrade path: local TLS/HTTPS.
    async function copyTextToClipboard(text) {
      if (text === null || text === undefined) return false;
      const str = String(text);
      if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
        try {
          await navigator.clipboard.writeText(str);
          return true;
        } catch (_) {
          // Fall through to legacy fallback if rejected (e.g. permission denied or insecure context)
        }
      }

      try {
        const textArea = document.createElement('textarea');
        textArea.value = str;
        textArea.style.position = 'fixed';
        textArea.style.left = '-999999px';
        textArea.style.top = '-999999px';
        textArea.style.opacity = '0';
        textArea.setAttribute('readonly', '');
        // Append inside active open dialog if any, so HTML5 modal dialog inertness doesn't block focus/selection
        const activeDialog = document.querySelector('dialog[open]');
        const container = activeDialog || document.body;
        container.appendChild(textArea);
        textArea.focus();
        textArea.select();
        textArea.setSelectionRange(0, textArea.value.length);
        const successful = document.execCommand('copy');
        container.removeChild(textArea);
        if (successful) return true;
      } catch (e) {
        console.warn('Fallback copy error:', e);
      }
      return false;
    }

    async function copyMasterPIN() {
      const pin = cachedMasterPIN || document.getElementById('master-pin-text').textContent;
      const ok = await copyTextToClipboard(pin);
      if (ok) {
        // ponytail: omit raw PIN value from alert message to prevent shoulder-surfing popups
        showNativeAlert('Master PIN copied to clipboard!\nUse this PIN to unlock SqlFox when accessing from your tablet/phone.', 'Remote Access');
      } else {
        showNativeAlert('Failed to copy Master PIN to clipboard.', 'Clipboard Error');
      }
    }

    async function initAppWorkspace() {
      document.body.classList.remove('vault-locked');
      document.getElementById('remote-lock-modal').classList.add('hidden');
      const initTable = document.getElementById('results-table');
      if (initTable) initTable.classList.add('table-empty');
      initSidebarVisibility();
      restoreTabsState();
      initCodeMirror();
      initAIChatSession();
      initWorkspacePaneSplitter();
      updateAIProviderBadge();
      await loadConnections();
      await loadMasterPIN();
      await loadSnippets();
      renderTabs();
      setupKeyboardShortcuts();
      updateFooterStats();
      initBrowserPopoutAffordance();
      if (typeof startNotesReminderPoller === 'function') startNotesReminderPoller();
      if (typeof startAutoUpdateCheck === 'function') startAutoUpdateCheck();
      if (typeof initActivityConsole === 'function') initActivityConsole();
    }

    // --- Lifecycle Initialization ---
    window.addEventListener('DOMContentLoaded', async () => {
      initSidebarVisibility();
      const lockModal = document.getElementById('remote-lock-modal');
      if (lockModal && !lockModal.classList.contains('hidden')) {
        const passInput = document.getElementById('remote-unlock-password');
        if (passInput) passInput.focus();
      }
      const authorized = await checkAuthStatus();
      if (!authorized) {
        return; // Remote client is locked: do not initialize workspace
      }
      await initAppWorkspace();
    });

    // ponytail: reload safety guard for in-flight queries and uncommitted staged table edits
    window.addEventListener('beforeunload', (e) => {
      saveTabsState();
      const stagedCount = (typeof stagedEdits !== 'undefined' ? stagedEdits.size : 0) +
                          (typeof stagedDraftRows !== 'undefined' ? stagedDraftRows.length : 0);
      if (isQueryExecuting || stagedCount > 0) {
        e.preventDefault();
        e.returnValue = '';
        return '';
      }
    });

    window.addEventListener('pagehide', () => {
      saveTabsState();
    });

