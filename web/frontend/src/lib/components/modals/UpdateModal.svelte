<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface UpdateInfo {
    current_version: string;
    latest_version: string;
    has_update: boolean;
    published_at?: string;
    release_notes?: string;
    error?: string;
  }

  let activeTab = $state<'update' | 'changelog'>('update');
  let info = $state<UpdateInfo | null>(null);
  let isChecking = $state(false);
  let isUpdating = $state(false);
  let updateStatus = $state<string | null>(null);
  let errorMessage = $state<string | null>(null);

  let changelogContent = $state<string>('');
  let changelogVersion = $state<string>('');
  let isLoadingChangelog = $state(false);

  async function checkUpdate(force = true) {
    isChecking = true;
    errorMessage = null;
    try {
      const data = await api.checkSystemUpdate(force);
      info = data;
      if (data.error) {
        errorMessage = data.error;
      }
    } catch (err: any) {
      errorMessage = err.message || 'Failed to check for updates';
    } finally {
      isChecking = false;
    }
  }

  async function loadChangelog() {
    if (changelogContent) return;
    isLoadingChangelog = true;
    try {
      const res = await api.getSystemChangelog();
      changelogContent = res.changelog || 'No changelog entries found.';
      changelogVersion = res.version || '';
    } catch (err: any) {
      changelogContent = `Failed to load changelog: ${err.message}`;
    } finally {
      isLoadingChangelog = false;
    }
  }

  async function applyUpdate() {
    if (!info?.latest_version) return;
    if (!confirm(`Apply update to v${info.latest_version}? SqlFox will automatically restart.`)) return;

    isUpdating = true;
    updateStatus = 'Downloading binary asset from GitHub release...';
    errorMessage = null;

    try {
      await api.applySystemUpdate(info.latest_version);
      updateStatus = 'Update applied successfully! Restarting SqlFox daemon...';
      setTimeout(() => {
        window.location.reload();
      }, 3000);
    } catch (err: any) {
      errorMessage = err.message || 'Failed to apply update';
      isUpdating = false;
      updateStatus = null;
    }
  }

  $effect(() => {
    if (appState.isUpdateModalOpen) {
      checkUpdate(false);
    }
  });

  $effect(() => {
    if (activeTab === 'changelog') {
      loadChangelog();
    }
  });
</script>

{#if appState.isUpdateModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-xl shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans max-h-[88vh]">
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="zap" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">SqlFox Self-Update & Releases</h3>
        </div>
        <button onclick={() => (appState.isUpdateModalOpen = false)} class="text-slate-400 hover:text-white" aria-label="Close">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Tab Switcher -->
      <div class="flex border-b border-[#1e293b] bg-[#090d16] px-5 pt-2">
        <button
          onclick={() => (activeTab = 'update')}
          class="px-4 py-2 text-xs font-medium border-b-2 transition -mb-px {activeTab === 'update' ? 'border-sky-500 text-sky-400 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          Check Updates
        </button>
        <button
          onclick={() => (activeTab = 'changelog')}
          class="px-4 py-2 text-xs font-medium border-b-2 transition -mb-px flex items-center space-x-1.5 {activeTab === 'changelog' ? 'border-sky-500 text-sky-400 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
        >
          <Icon name="file-text" class="w-3.5 h-3.5" />
          <span>Full Changelog</span>
          {#if changelogVersion}
            <span class="text-[10px] px-1 py-0.2 bg-slate-800 text-slate-400 rounded font-mono">v{changelogVersion}</span>
          {/if}
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 space-y-4 overflow-y-auto flex-1">
        {#if activeTab === 'update'}
          {#if isChecking}
            <div class="p-8 text-center text-slate-500 flex flex-col items-center justify-center space-y-2">
              <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
              <span>Checking GitHub releases...</span>
            </div>
          {:else if errorMessage}
            <div class="p-4 rounded-xl bg-rose-950/40 border border-rose-800 text-rose-300 space-y-1">
              <div class="font-bold flex items-center space-x-1.5">
                <Icon name="alert-triangle" class="w-4 h-4 text-rose-400" />
                <span>Update Check Failed</span>
              </div>
              <p class="text-xs text-rose-400">{errorMessage}</p>
            </div>
          {:else if info}
            <!-- Version Status Cards -->
            <div class="grid grid-cols-2 gap-3">
              <div class="p-3 bg-[#050811] border border-[#1e293b] rounded-xl">
                <span class="text-slate-500 text-[10px] uppercase font-mono block">Current Version</span>
                <span class="text-sm font-bold font-mono text-slate-200">v{info.current_version || '0.18.0'}</span>
              </div>
              <div class="p-3 bg-[#050811] border border-[#1e293b] rounded-xl">
                <span class="text-slate-500 text-[10px] uppercase font-mono block">Latest Release</span>
                <div class="flex items-center space-x-2">
                  <span class="text-sm font-bold font-mono text-sky-400">v{info.latest_version || '--'}</span>
                  {#if info.has_update}
                    <span class="px-1.5 py-0.2 rounded bg-amber-950 border border-amber-700 text-amber-300 text-[10px] font-bold">
                      NEW
                    </span>
                  {:else}
                    <span class="px-1.5 py-0.2 rounded bg-emerald-950 border border-emerald-700 text-emerald-300 text-[10px] font-bold">
                      UP TO DATE
                    </span>
                  {/if}
                </div>
              </div>
            </div>

            <!-- Release Notes Body -->
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <span class="text-slate-400 text-xs font-medium">Release Notes</span>
                {#if info.published_at}
                  <span class="text-slate-500 text-[10px]">
                    Published: {new Date(info.published_at).toLocaleDateString([], { year: 'numeric', month: 'short', day: 'numeric' })}
                  </span>
                {/if}
              </div>
              <div class="p-3 bg-[#050811] border border-[#1e293b] rounded-xl max-h-48 overflow-y-auto text-slate-300 text-xs leading-relaxed whitespace-pre-wrap font-sans">
                {info.release_notes || 'No release notes provided.'}
              </div>
            </div>

            {#if updateStatus}
              <div class="p-3 rounded-xl bg-sky-950/40 border border-sky-800 text-sky-300 flex items-center space-x-2">
                <Icon name="refresh-cw" class="w-4 h-4 animate-spin text-sky-400 shrink-0" />
                <span>{updateStatus}</span>
              </div>
            {/if}
          {/if}
        {:else}
          <!-- Changelog Tab -->
          <div class="space-y-2">
            {#if isLoadingChangelog}
              <div class="p-8 text-center text-slate-500 flex flex-col items-center justify-center space-y-2">
                <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
                <span>Loading CHANGELOG.md...</span>
              </div>
            {:else}
              <div class="p-3.5 bg-[#050811] border border-[#1e293b] rounded-xl font-mono text-[11px] text-slate-300 whitespace-pre-wrap max-h-[380px] overflow-y-auto select-text leading-relaxed">
                {changelogContent}
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <button
          onclick={() => checkUpdate(true)}
          disabled={isChecking || isUpdating}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition flex items-center space-x-1.5"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isChecking ? 'animate-spin' : ''}" />
          <span>Check Again</span>
        </button>

        <div class="flex items-center space-x-2">
          <button
            onclick={() => (appState.isUpdateModalOpen = false)}
            disabled={isUpdating}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Close
          </button>
          {#if activeTab === 'update' && info?.has_update}
            <button
              onclick={applyUpdate}
              disabled={isUpdating}
              class="px-4 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-emerald-900/30"
            >
              <Icon name="zap" class="w-3.5 h-3.5 {isUpdating ? 'animate-spin' : ''}" />
              <span>{isUpdating ? 'Updating...' : 'Update Now'}</span>
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}
