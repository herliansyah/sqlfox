<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';
  import AiSettingsPanel from './AiSettingsPanel.svelte';
  import { focusTrap } from '../../actions/focusTrap';

  type SettingsTab = 'sync' | 'ai' | 'bundle' | 'security';

  let currentTab = $state<SettingsTab>('sync');
  let aiPanel = $state<AiSettingsPanel | null>(null);
  let isAiTesting = $state(false);

  // Cloud Sync State
  let syncEnabled = $state(false);
  let githubToken = $state('');
  let hasSavedToken = $state(false);
  let githubRepo = $state('');
  let githubBranch = $state('main');
  let pathPrefix = $state('sqlfox-vault/');
  let includeVault = $state(true);
  let isTesting = $state(false);
  let isSyncing = $state(false);
  let syncStatus = $state<{ ok: boolean; message: string } | null>(null);
  let remoteSyncInfo = $state<any>(null);

  // Notes Bundle Import State
  let importFileInput = $state<HTMLInputElement | null>(null);
  let isImporting = $state(false);

  // Encrypted Vault Export/Import State
  let vaultPassphrase = $state('');
  let currentMasterPass = $state('');
  let isExportingVault = $state(false);
  let isImportingVault = $state(false);
  let vaultImportInput = $state<HTMLInputElement | null>(null);
  let masterKeyInfo = $state<{ lan_enabled: boolean; pairing_code?: string; pairing_expires_in_sec?: number; lan_ip?: string; port?: number | string } | null>(null);

  let tabletLanUrl = $derived.by(() => {
    const ip = masterKeyInfo?.lan_ip;
    const port = masterKeyInfo?.port || window.location.port;
    if (ip) return `http://${ip}${port ? `:${port}` : ''}`;
    return window.location.origin;
  });

  async function loadSyncConfig() {
    try {
      const [cfg, statusRes, keyRes] = await Promise.allSettled([
        api.getCloudSyncConfig(),
        api.getCloudSyncStatus(),
        api.getMasterKey(),
      ]);
      if (cfg.status === 'fulfilled' && cfg.value) {
        syncEnabled = !!cfg.value.enabled;
        hasSavedToken = !!cfg.value.has_token;
        githubRepo = cfg.value.github_repo || '';
        githubBranch = cfg.value.github_branch || 'main';
        pathPrefix = cfg.value.path_prefix || 'sqlfox-vault/';
        includeVault = cfg.value.include_vault_backup !== false;
      }
      if (statusRes.status === 'fulfilled') {
        remoteSyncInfo = statusRes.value;
      }
      if (keyRes.status === 'fulfilled') {
        masterKeyInfo = keyRes.value;
      }
    } catch (_) {}
  }

  async function saveSyncConfig(silent = false): Promise<boolean> {
    try {
      const payload: any = {
        enabled: syncEnabled,
        github_repo: githubRepo.trim(),
        github_branch: githubBranch.trim(),
        path_prefix: pathPrefix.trim(),
        include_vault_backup: includeVault,
      };
      if (githubToken.trim()) {
        payload.github_token = githubToken.trim();
      }
      await api.saveCloudSyncConfig(payload);
      if (githubToken.trim()) {
        hasSavedToken = true;
      }
      if (!silent) {
        appState.showToast('Cloud Sync settings saved successfully!', 'success');
      }
      return true;
    } catch (err: any) {
      appState.showToast(`Failed to save cloud sync: ${err.message}`, 'error');
      return false;
    }
  }

  async function testCloudSync() {
    isTesting = true;
    syncStatus = null;
    try {
      const data = await api.testCloudSync({
        github_token: githubToken.trim() || undefined,
        github_repo: githubRepo.trim(),
        github_branch: githubBranch.trim() || 'main',
      });
      syncStatus = { ok: true, message: data.message || 'GitHub repository connection verified successfully!' };
    } catch (err: any) {
      syncStatus = { ok: false, message: `Test error: ${err.message}` };
    } finally {
      isTesting = false;
    }
  }

  async function runCloudSyncNow() {
    if (!githubRepo.trim()) {
      appState.showToast('Please specify a GitHub repository (owner/repo)', 'warning');
      return;
    }
    if (!githubToken.trim() && !hasSavedToken) {
      appState.showToast('Please enter a GitHub Personal Access Token (PAT)', 'warning');
      return;
    }
    if (!syncEnabled) {
      syncEnabled = true;
    }
    isSyncing = true;
    try {
      // Auto-save form if fields are modified or token provided before triggering sync
      const saved = await saveSyncConfig(true);
      if (!saved) return;

      const data = await api.runCloudSync();
      appState.showToast(data.message || 'Cloud Sync push & pull completed!', 'success');
      try {
        remoteSyncInfo = await api.getCloudSyncStatus();
      } catch (_) {}
    } catch (err: any) {
      appState.showToast(`Sync execution failed: ${err.message}`, 'error');
    } finally {
      isSyncing = false;
    }
  }

  async function handleExportVault() {
    const pass = vaultPassphrase.trim();
    if (pass.length < 4) {
      appState.showToast('Enter an encryption passphrase (at least 4 characters) to export vault', 'warning');
      return;
    }
    const master = currentMasterPass.trim();
    if (!master) {
      appState.showToast('Enter your current Master Password / PIN to authorize vault export', 'warning');
      return;
    }
    isExportingVault = true;
    try {
      const blob = await api.exportVault(pass, master);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'sqlfox-connections.sqlfox-vault';
      a.click();
      URL.revokeObjectURL(url);
      currentMasterPass = '';
      appState.showToast('Encrypted vault exported (.sqlfox-vault)', 'success');
    } catch (err: any) {
      appState.showToast(`Vault export failed: ${err.message}`, 'error');
    } finally {
      isExportingVault = false;
    }
  }

  async function handleImportVaultFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    const pass = vaultPassphrase.trim();
    if (!pass) {
      appState.showToast('Enter the vault decryption passphrase before selecting the .sqlfox-vault file', 'warning');
      input.value = '';
      return;
    }
    isImportingVault = true;
    try {
      const text = await file.text();
      const archive = JSON.parse(text);
      const res = await api.importVault(pass, archive);
      await appState.loadConnections();
      appState.showToast(`Imported ${res.imported_count} connection(s) into vault`, 'success');
    } catch (err: any) {
      appState.showToast(`Vault import failed: ${err.message}`, 'error');
    } finally {
      isImportingVault = false;
      if (vaultImportInput) vaultImportInput.value = '';
    }
  }

  function exportNotesBundle() {
    window.location.href = '/api/notes/bundle/export';
  }

  async function handleImportFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    isImporting = true;
    try {
      const data = await api.importNotesBundle(file);
      appState.showToast(`Imported ${data.imported_count || 'notes'} successfully!`, 'success');
    } catch (err: any) {
      appState.showToast(`Import error: ${err.message}`, 'error');
    } finally {
      isImporting = false;
      if (importFileInput) importFileInput.value = '';
    }
  }

  $effect(() => {
    if (appState.isSettingsModalOpen) {
      loadSyncConfig();
    }
  });
</script>

{#if appState.isSettingsModalOpen}
  <div role="dialog" aria-modal="true" use:focusTrap class="fixed inset-0 z-50 bg-black/75 flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-xl shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <!-- Header -->
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="sliders" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">SqlFox Global Settings & Cloud Sync</h3>
        </div>
        <button onclick={() => (appState.isSettingsModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Navigation Tabs -->
      <div class="flex items-center space-x-1 px-5 pt-3 border-b border-[#1e293b] bg-[#070b14] select-none text-xs">
        <button
          onclick={() => (currentTab = 'sync')}
          class="px-3 py-1.5 font-medium transition border-b-2 {currentTab === 'sync' ? 'border-sky-400 text-sky-300 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          GitHub Cloud Sync
        </button>
        <button
          onclick={() => (currentTab = 'ai')}
          class="px-3 py-1.5 font-medium transition border-b-2 flex items-center space-x-1 {currentTab === 'ai' ? 'border-sky-400 text-sky-300 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          <Icon name="bot" class="w-3 h-3 text-sky-400" />
          <span>AI Copilot</span>
        </button>
        <button
          onclick={() => (currentTab = 'bundle')}
          class="px-3 py-1.5 font-medium transition border-b-2 {currentTab === 'bundle' ? 'border-amber-400 text-amber-300 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          Notes Portability
        </button>
        <button
          onclick={() => (currentTab = 'security')}
          class="px-3 py-1.5 font-medium transition border-b-2 {currentTab === 'security' ? 'border-emerald-400 text-emerald-300 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          Security & LAN PIN
        </button>
      </div>

      <!-- Content Panes -->
      <div class="p-5 space-y-4 max-h-[460px] overflow-y-auto">
        {#if currentTab === 'ai'}
          <AiSettingsPanel
            bind:this={aiPanel}
            active={appState.isSettingsModalOpen && currentTab === 'ai'}
            bind:isTesting={isAiTesting}
          />
        {:else if currentTab === 'sync'}
          <label class="flex items-center space-x-2 p-2.5 rounded-xl bg-slate-900/60 border border-slate-800 cursor-pointer">
            <input type="checkbox" bind:checked={syncEnabled} class="rounded border-slate-700 text-sky-500" />
            <div>
              <span class="text-slate-200 font-semibold text-xs block">Enable GitHub Cloud Sync</span>
              <span class="text-slate-400 text-[11px]">Synchronizes your connections, notes, and query snippets to a private GitHub repository</span>
            </div>
          </label>

          <div class="space-y-3">
            <div>
              <div class="flex items-center justify-between mb-1">
                <label for="sync-github-token" class="block text-slate-400 text-xs font-medium">GitHub Personal Access Token (PAT) *</label>
                {#if hasSavedToken}
                  <span class="inline-flex items-center space-x-1 text-[10.5px] text-emerald-400 font-mono bg-emerald-950/60 border border-emerald-800/70 px-2 py-0.5 rounded">
                    <Icon name="check-circle" class="w-3 h-3 text-emerald-400" />
                    <span>Configured in vault</span>
                  </span>
                {/if}
              </div>
              <input
                id="sync-github-token"
                type="password"
                bind:value={githubToken}
                placeholder={hasSavedToken ? '•••••••••••• (Leave blank to keep existing token)' : 'ghp_xxxxxxxxxxxxxxxxxxxx (needs repo scope)'}
                class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
              />
              <p class="text-[10px] text-slate-500 mt-1">
                {#if hasSavedToken && !githubToken}
                  An encrypted token is currently configured in your vault. Enter a new token only if you wish to overwrite it.
                {:else}
                  Classic PAT requires <code class="text-sky-300 font-mono">repo</code> scope. Fine-grained PAT requires Read &amp; Write to Contents.
                {/if}
              </p>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <div>
                <label for="sync-github-repo" class="block text-slate-400 text-xs font-medium mb-1">Repository (owner/repo) *</label>
                <input
                  id="sync-github-repo"
                  type="text"
                  bind:value={githubRepo}
                  placeholder="e.g. yourname/sqlfox-vault"
                  class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
                />
              </div>
              <div>
                <label for="sync-github-branch" class="block text-slate-400 text-xs font-medium mb-1">Branch</label>
                <input
                  id="sync-github-branch"
                  type="text"
                  bind:value={githubBranch}
                  placeholder="main"
                  class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
                />
              </div>
            </div>

            <div>
              <label for="sync-path-prefix" class="block text-slate-400 text-xs font-medium mb-1">Repository Path Prefix</label>
              <input
                id="sync-path-prefix"
                type="text"
                bind:value={pathPrefix}
                placeholder="sqlfox-vault/"
                class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
              />
            </div>

            <label class="flex items-center space-x-2 p-2.5 rounded-xl bg-slate-900/40 border border-slate-800/80 cursor-pointer">
              <input type="checkbox" bind:checked={includeVault} class="rounded border-slate-700 text-sky-500" />
              <div>
                <span class="text-slate-200 font-semibold text-xs block">Include Encrypted Vault Backup</span>
                <span class="text-slate-400 text-[10.5px]">Back up encrypted connection credentials alongside notes &amp; snippets</span>
              </div>
            </label>
          </div>

          {#if remoteSyncInfo}
            <div class="p-3 rounded-xl bg-[#050811] border border-[#1e293b] flex items-center justify-between text-[11px]">
              <div class="space-y-0.5">
                <div class="font-semibold text-slate-300">Cloud Sync Status</div>
                <div class="text-slate-400 font-mono">
                  {remoteSyncInfo.last_sync_at ? `Last synced: ${new Date(remoteSyncInfo.last_sync_at).toLocaleString()}` : remoteSyncInfo.status || (syncEnabled ? 'Configured & ready' : 'Sync disabled')}
                </div>
              </div>
              <span class="px-2 py-0.5 rounded font-mono text-[10px] {remoteSyncInfo.last_error ? 'bg-rose-950 text-rose-300 border border-rose-800' : syncEnabled ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' : 'bg-slate-900 text-slate-400 border border-slate-800'}">
                {remoteSyncInfo.last_error ? 'Error' : syncEnabled ? 'Active' : 'Off'}
              </span>
            </div>
          {/if}

          {#if syncStatus}
            <div class="p-3 rounded-xl border text-xs flex items-center space-x-2 {syncStatus.ok ? 'bg-emerald-950/40 border-emerald-800 text-emerald-300' : 'bg-rose-950/40 border-rose-800 text-rose-300'}">
              <Icon name={syncStatus.ok ? 'check-circle' : 'alert-circle'} class="w-4 h-4 shrink-0" />
              <span>{syncStatus.message}</span>
            </div>
          {/if}

          <div class="flex items-center justify-between pt-2">
            <button
              onclick={testCloudSync}
              disabled={isTesting || !githubRepo}
              class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-slate-300 text-xs font-medium transition flex items-center space-x-1.5"
            >
              <Icon name="activity" class="w-3.5 h-3.5 {isTesting ? 'animate-spin text-sky-400' : ''}" />
              <span>{isTesting ? 'Testing...' : 'Test Connection'}</span>
            </button>

            <button
              onclick={runCloudSyncNow}
              disabled={isSyncing || !githubRepo.trim() || (!githubToken.trim() && !hasSavedToken)}
              class="px-3.5 py-1.5 rounded bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-md shadow-sky-950/30"
            >
              <Icon name="refresh-cw" class="w-3.5 h-3.5 {isSyncing ? 'animate-spin' : ''}" />
              <span>{isSyncing ? 'Syncing...' : 'Sync Now (Push & Pull)'}</span>
            </button>
          </div>
        {:else if currentTab === 'bundle'}
          <!-- Notes Bundle Portability -->
          <div class="space-y-4">
            <div class="p-4 rounded-xl bg-[#050811] border border-[#1e293b] flex items-center justify-between">
              <div>
                <h4 class="font-bold text-slate-200 text-xs">Export Operational Notes Bundle</h4>
                <p class="text-slate-400 text-[11px] mt-0.5">Download a ZIP bundle containing team operational notes, Markdown snapshots &amp; reminders.</p>
              </div>
              <button
                onclick={exportNotesBundle}
                class="px-3.5 py-1.5 rounded bg-amber-600 hover:bg-amber-500 text-white font-semibold text-xs transition flex items-center space-x-1.5 shadow-md shadow-amber-950/30 shrink-0 ml-3"
              >
                <Icon name="download" class="w-3.5 h-3.5" />
                <span>Export .ZIP</span>
              </button>
            </div>

            <div class="p-4 rounded-xl bg-[#050811] border border-[#1e293b] flex items-center justify-between">
              <div>
                <h4 class="font-bold text-slate-200 text-xs">Import Operational Notes Bundle</h4>
                <p class="text-slate-400 text-[11px] mt-0.5">Restore notes from a previously exported SqlFox <code class="font-mono text-amber-300">.zip</code> bundle archive.</p>
              </div>
              <div>
                <input
                  type="file"
                  accept=".zip,application/zip"
                  bind:this={importFileInput}
                  onchange={handleImportFile}
                  class="hidden"
                />
                <button
                  onclick={() => importFileInput?.click()}
                  disabled={isImporting}
                  class="px-3.5 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold text-xs transition flex items-center space-x-1.5 border border-[#334155] shrink-0 ml-3"
                >
                  <Icon name="upload" class="w-3.5 h-3.5" />
                  <span>{isImporting ? 'Importing...' : 'Upload Bundle'}</span>
                </button>
              </div>
            </div>
          </div>
        {:else if currentTab === 'security'}
          <div class="space-y-4">
            <!-- Encrypted Vault Export / Import -->
            <div class="p-4 rounded-xl bg-[#050811] border border-[#1e293b] space-y-3">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-bold text-slate-200 text-xs">Encrypted Connection Vault (.sqlfox-vault)</h4>
                  <p class="text-slate-400 text-[11px] mt-0.5">Export or restore encrypted database credentials using a portable passphrase.</p>
                </div>
                <Icon name="shield-check" class="w-4 h-4 text-emerald-400 shrink-0" />
              </div>

              <input
                type="password"
                bind:value={vaultPassphrase}
                placeholder="Enter export / import encryption passphrase..."
                class="w-full bg-[#090d16] border border-[#1e293b] rounded-lg px-3 py-1.5 text-slate-200 font-mono text-xs focus:outline-none focus:border-emerald-500"
              />
              <input
                type="password"
                bind:value={currentMasterPass}
                placeholder="Current Master Password / PIN (required for export)..."
                class="w-full bg-[#090d16] border border-[#1e293b] rounded-lg px-3 py-1.5 text-slate-200 font-mono text-xs focus:outline-none focus:border-amber-500"
              />

              <div class="flex items-center justify-end space-x-2">
                <input
                  type="file"
                  accept=".sqlfox-vault,.json"
                  bind:this={vaultImportInput}
                  onchange={handleImportVaultFile}
                  class="hidden"
                />
                <button
                  type="button"
                  onclick={() => vaultImportInput?.click()}
                  disabled={isImportingVault}
                  class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 border border-[#334155] text-slate-200 text-xs font-semibold transition flex items-center space-x-1.5"
                >
                  <Icon name="upload" class="w-3.5 h-3.5" />
                  <span>{isImportingVault ? 'Importing...' : 'Import Vault'}</span>
                </button>
                <button
                  type="button"
                  onclick={handleExportVault}
                  disabled={isExportingVault}
                  class="px-3 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold transition flex items-center space-x-1.5"
                >
                  <Icon name="download" class="w-3.5 h-3.5" />
                  <span>{isExportingVault ? 'Exporting...' : 'Export Vault'}</span>
                </button>
              </div>
            </div>

            <!-- Live LAN & Master PIN Card -->
            <div class="p-4 rounded-xl bg-[#050811] border border-[#1e293b] space-y-2.5">
              <div class="flex items-center justify-between">
                <span class="text-slate-200 font-bold text-xs">Remote LAN Access &amp; Master PIN</span>
                <span class="px-2 py-0.5 rounded font-mono text-[10px] {masterKeyInfo?.lan_enabled || appState.authStatus?.lan_enabled ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' : 'bg-slate-900 text-slate-400 border border-slate-800'}">
                  {masterKeyInfo?.lan_enabled || appState.authStatus?.lan_enabled ? 'LAN Listener Active' : 'Localhost Only'}
                </span>
              </div>
              <p class="text-slate-400 text-[11px] leading-relaxed">
                Remote browsers or tablets on your LAN must authenticate with the runtime Master PIN before executing commands.
              </p>

              <div class="flex flex-wrap items-center gap-2 pt-1">
                {#if masterKeyInfo?.pairing_code || appState.authStatus?.pairing_code}
                  {@const pin = masterKeyInfo?.pairing_code || appState.authStatus?.pairing_code || ''}
                  <div class="px-2.5 py-1 rounded bg-slate-900 border border-slate-800 font-mono text-emerald-400 text-xs font-bold tracking-widest">
                    PAIRING CODE (1x, 5m): {pin}
                  </div>
                  <button
                    type="button"
                    onclick={() => {
                      copyText(pin);
                      appState.showToast('One-time Pairing Code copied to clipboard', 'success');
                    }}
                    class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-[11px] font-medium transition flex items-center space-x-1"
                  >
                    <Icon name="copy" class="w-3 h-3" />
                    <span>Copy Code</span>
                  </button>
                {:else}
                  <span class="text-[11px] text-slate-500 italic">Master PIN is printed in the local terminal startup banner on localhost.</span>
                {/if}

                <button
                  type="button"
                  onclick={() => {
                    copyText(tabletLanUrl);
                    appState.showToast(`Copied Tablet/LAN URL: ${tabletLanUrl}`, 'success');
                  }}
                  class="px-2.5 py-1 rounded bg-sky-950/70 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[11px] font-medium transition flex items-center space-x-1 ml-auto"
                >
                  <Icon name="link" class="w-3 h-3" />
                  <span>Copy LAN URL</span>
                </button>
              </div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        {#if currentTab === 'ai'}
          <button
            type="button"
            onclick={() => aiPanel?.testConnection()}
            disabled={isAiTesting}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition flex items-center space-x-1.5"
          >
            <Icon name="activity" class="w-3.5 h-3.5 {isAiTesting ? 'animate-spin text-sky-400' : ''}" />
            <span>{isAiTesting ? 'Testing...' : 'Test Connection'}</span>
          </button>
        {:else}
          <span class="text-slate-500 text-xs">SqlFox Single-Binary Database Suite</span>
        {/if}
        <div class="flex items-center space-x-2">
          <button
            onclick={() => (appState.isSettingsModalOpen = false)}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Cancel
          </button>
          {#if currentTab === 'sync'}
            <button
              onclick={() => saveSyncConfig(false)}
              class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold transition"
            >
              Save Configuration
            </button>
          {:else if currentTab === 'ai'}
            <button
              type="button"
              onclick={() => aiPanel?.saveConfig()}
              class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold transition"
            >
              Save Settings
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}
