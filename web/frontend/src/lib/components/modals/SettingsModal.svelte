<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  type SettingsTab = 'sync' | 'bundle' | 'security';

  let currentTab = $state<SettingsTab>('sync');

  // Cloud Sync State
  let syncEnabled = $state(false);
  let githubToken = $state('');
  let githubRepo = $state('');
  let githubBranch = $state('main');
  let pathPrefix = $state('sqlfox-vault/');
  let includeVault = $state(true);
  let isTesting = $state(false);
  let isSyncing = $state(false);
  let syncStatus = $state<{ ok: boolean; message: string } | null>(null);

  // Notes Bundle Import State
  let importFileInput = $state<HTMLInputElement | null>(null);
  let isImporting = $state(false);

  async function loadSyncConfig() {
    try {
      const cfg = await api.getCloudSyncConfig();
      if (cfg) {
        syncEnabled = !!cfg.enabled;
        githubRepo = cfg.github_repo || '';
        githubBranch = cfg.github_branch || 'main';
        pathPrefix = cfg.path_prefix || 'sqlfox-vault/';
        includeVault = cfg.include_vault_backup !== false;
      }
    } catch (e) {}
  }

  async function saveSyncConfig() {
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
      appState.showToast('Cloud Sync settings saved successfully!', 'success');
    } catch (err: any) {
      appState.showToast(`Failed to save cloud sync: ${err.message}`, 'error');
    }
  }

  async function testCloudSync() {
    isTesting = true;
    syncStatus = null;
    try {
      const data = await api.testCloudSync({
        github_token: githubToken.trim() || undefined,
        github_repo: githubRepo.trim(),
      });
      syncStatus = { ok: true, message: data.message || 'GitHub repository connection verified successfully!' };
    } catch (err: any) {
      syncStatus = { ok: false, message: `Test error: ${err.message}` };
    } finally {
      isTesting = false;
    }
  }

  async function runCloudSyncNow() {
    isSyncing = true;
    try {
      const data = await api.runCloudSync();
      appState.showToast(data.message || 'Cloud Sync push & pull completed!', 'success');
    } catch (err: any) {
      appState.showToast(`Sync execution failed: ${err.message}`, 'error');
    } finally {
      isSyncing = false;
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
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
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
        {#if currentTab === 'sync'}
          <label class="flex items-center space-x-2 p-2.5 rounded-xl bg-slate-900/60 border border-slate-800 cursor-pointer">
            <input type="checkbox" bind:checked={syncEnabled} class="rounded border-slate-700 text-sky-500" />
            <div>
              <span class="text-slate-200 font-semibold text-xs block">Enable GitHub Cloud Sync</span>
              <span class="text-slate-400 text-[11px]">Synchronizes your connections, notes, and query snippets to a private GitHub repository</span>
            </div>
          </label>

          <div class="space-y-3">
            <div>
              <label class="block text-slate-400 text-xs font-medium mb-1">GitHub Personal Access Token (PAT) *</label>
              <input
                type="password"
                bind:value={githubToken}
                placeholder="ghp_xxxxxxxxxxxxxxxxxxxx (needs repo scope)"
                class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
              />
            </div>

            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="block text-slate-400 text-xs font-medium mb-1">Repository (owner/repo) *</label>
                <input
                  type="text"
                  bind:value={githubRepo}
                  placeholder="e.g. yourname/sqlfox-vault"
                  class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
                />
              </div>
              <div>
                <label class="block text-slate-400 text-xs font-medium mb-1">Branch</label>
                <input
                  type="text"
                  bind:value={githubBranch}
                  placeholder="main"
                  class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
                />
              </div>
            </div>

            <div>
              <label class="block text-slate-400 text-xs font-medium mb-1">Repository Path Prefix</label>
              <input
                type="text"
                bind:value={pathPrefix}
                placeholder="sqlfox-vault/"
                class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
              />
            </div>
          </div>

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
              disabled={isSyncing || !syncEnabled}
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
                <p class="text-slate-400 text-[11px] mt-0.5">Download an offline JSON bundle of all team operational notes, snapshots & reminders.</p>
              </div>
              <button
                onclick={exportNotesBundle}
                class="px-3.5 py-1.5 rounded bg-amber-600 hover:bg-amber-500 text-white font-semibold text-xs transition flex items-center space-x-1.5 shadow-md shadow-amber-950/30 shrink-0 ml-3"
              >
                <Icon name="download" class="w-3.5 h-3.5" />
                <span>Export Bundle</span>
              </button>
            </div>

            <div class="p-4 rounded-xl bg-[#050811] border border-[#1e293b] flex items-center justify-between">
              <div>
                <h4 class="font-bold text-slate-200 text-xs">Import Operational Notes Bundle</h4>
                <p class="text-slate-400 text-[11px] mt-0.5">Restore notes from a previously exported SqlFox notes bundle file.</p>
              </div>
              <div>
                <input
                  type="file"
                  accept=".json"
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
                  <span>{isImporting ? 'Importing...' : 'Upload File'}</span>
                </button>
              </div>
            </div>
          </div>
        {:else if currentTab === 'security'}
          <div class="space-y-4">
            <div class="p-4 rounded-xl bg-[#050811] border border-[#1e293b] space-y-2">
              <span class="text-slate-400 font-medium text-xs block">Remote LAN Master PIN</span>
              <p class="text-slate-400 text-xs leading-relaxed">
                When SqlFox is accessed from another computer on the local network (LAN) or behind a reverse proxy, the client must authenticate with the Master PIN before any database commands can be issued.
              </p>
              <div class="p-2.5 rounded bg-slate-900 border border-slate-800 font-mono text-emerald-400 text-sm font-bold tracking-widest inline-block">
                fox-0304
              </div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <span class="text-slate-500 text-xs">SqlFox Single-Binary Database Suite</span>
        <div class="flex items-center space-x-2">
          <button
            onclick={() => (appState.isSettingsModalOpen = false)}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Cancel
          </button>
          {#if currentTab === 'sync'}
            <button
              onclick={saveSyncConfig}
              class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold transition"
            >
              Save Configuration
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}
