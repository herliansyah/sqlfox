<script lang="ts">
  import { focusTrap } from '../../actions/focusTrap';
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { renderFullMarkdownHtml } from '../../utils/markdown';
  import Icon from '../common/Icon.svelte';

  interface UpdateInfo {
    current_version: string;
    latest_version: string;
    has_update: boolean;
    published_at?: string;
    release_notes?: string;
    asset_url?: string;
    asset_name?: string;
    download_size?: number;
    error?: string;
  }

  let activeTab = $state<'update' | 'changelog'>('update');
  let info = $state<UpdateInfo | null>(null);
  let isChecking = $state(false);
  let isUpdating = $state(false);
  let updateProgress = $state(0);
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
      appState.updateInfo = data;
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

  async function pollUntilReady() {
    const deadline = Date.now() + 30000;
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 900));
      try {
        const res = await fetch('/api/auth/status');
        if (res.ok) {
          updateProgress = 100;
          updateStatus = 'Daemon restarted and ready! Reloading application...';
          setTimeout(() => window.location.reload(), 400);
          return;
        }
      } catch {
        // Still restarting
      }
    }
    window.location.reload();
  }

  async function applyUpdate() {
    if (!info?.latest_version || !info?.asset_url) return;
    const ok = await appState.showNativeConfirm(
      `Apply update to ${info.latest_version.startsWith('v') ? '' : 'v'}${info.latest_version}? SqlFox will download the signed release binary, verify its checksum, and restart automatically.`,
      'Confirm In-App System Update'
    );
    if (!ok) return;

    isUpdating = true;
    updateProgress = 25;
    updateStatus = `Downloading ${info.asset_name || 'binary asset'} from GitHub release...`;
    errorMessage = null;

    const ticker = setInterval(() => {
      if (updateProgress < 85) updateProgress = Math.min(85, updateProgress + 5);
    }, 600);

    try {
      await api.applySystemUpdate(info.asset_url, info.latest_version);
      clearInterval(ticker);
      updateProgress = 92;
      updateStatus = 'Update verified & installed! Waiting for SqlFox daemon to restart...';
      await pollUntilReady();
    } catch (err: any) {
      clearInterval(ticker);
      errorMessage = err.message || 'Failed to apply update';
      isUpdating = false;
      updateProgress = 0;
      updateStatus = null;
    }
  }

  let wasOpen = false;
  $effect(() => {
    if (appState.isUpdateModalOpen) {
      if (!wasOpen) {
        wasOpen = true;
        activeTab = 'update';
        checkUpdate(false);
      }
    } else {
      wasOpen = false;
    }
  });

  $effect(() => {
    if (appState.isUpdateModalOpen && activeTab === 'changelog') {
      loadChangelog();
    }
  });

  function formatMb(bytes?: number): string {
    if (!bytes) return '';
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  let githubReleaseHref = $derived(
    info?.latest_version
      ? `https://github.com/herliansyah/sqlfox/releases/tag/${info.latest_version.startsWith('v') ? info.latest_version : 'v' + info.latest_version}`
      : 'https://github.com/herliansyah/sqlfox/releases'
  );
</script>

{#if appState.isUpdateModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 flex items-center justify-center p-4">
    <div role="dialog" aria-modal="true" use:focusTrap class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-2xl shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans max-h-[88vh]">
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="zap" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">SqlFox Self-Update &amp; Releases</h3>
        </div>
        <button type="button" onclick={() => (appState.isUpdateModalOpen = false)} disabled={isUpdating} class="text-slate-400 hover:text-white disabled:opacity-40" aria-label="Close">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Tab Switcher -->
      <div class="flex items-center justify-between border-b border-[#1e293b] bg-[#090d16] px-5 pt-2">
        <div class="flex">
          <button
            type="button"
            onclick={() => (activeTab = 'update')}
            class="px-4 py-2 text-xs font-medium border-b-2 transition -mb-px {activeTab === 'update' ? 'border-sky-500 text-sky-400 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
          >
            Check Updates
          </button>
          <button
            type="button"
            onclick={() => (activeTab = 'changelog')}
            class="px-4 py-2 text-xs font-medium border-b-2 transition -mb-px flex items-center space-x-1.5 {activeTab === 'changelog' ? 'border-sky-500 text-sky-400 font-bold' : 'border-transparent text-slate-400 hover:text-slate-200'}"
          >
            <Icon name="file-text" class="w-3.5 h-3.5" />
            <span>Full Changelog</span>
            {#if changelogVersion}
              <span class="text-[10px] px-1 py-0.5 bg-slate-800 text-slate-400 rounded font-mono">v{changelogVersion}</span>
            {/if}
          </button>
        </div>

        <a
          href={githubReleaseHref}
          target="_blank"
          rel="noopener noreferrer"
          class="text-[11px] text-sky-400 hover:underline flex items-center space-x-1 mb-1"
        >
          <Icon name="link" class="w-3 h-3" />
          <span>View on GitHub</span>
        </a>
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
                <span>Update Check / Apply Failed</span>
              </div>
              <p class="text-xs text-rose-400">{errorMessage}</p>
            </div>
          {:else if info}
            <!-- Version Status Cards -->
            <div class="grid grid-cols-2 gap-3">
              <div class="p-3 bg-[#050811] border border-[#1e293b] rounded-xl">
                <span class="text-slate-500 text-[10px] uppercase font-mono block">Current Version</span>
                <span class="text-sm font-bold font-mono text-slate-200">{info.current_version ? `${info.current_version.startsWith('v') ? '' : 'v'}${info.current_version}` : '--'}</span>
              </div>
              <div class="p-3 bg-[#050811] border border-[#1e293b] rounded-xl">
                <span class="text-slate-500 text-[10px] uppercase font-mono block">Latest Release</span>
                <div class="flex items-center space-x-2">
                  <span class="text-sm font-bold font-mono text-sky-400">{info.latest_version?.startsWith('v') ? '' : 'v'}{info.latest_version || '--'}</span>
                  {#if info.has_update}
                    <span class="px-1.5 py-0.5 rounded bg-amber-950 border border-amber-700 text-amber-300 text-[10px] font-bold">
                      NEW
                    </span>
                  {:else}
                    <span class="px-1.5 py-0.5 rounded bg-emerald-950 border border-emerald-700 text-emerald-300 text-[10px] font-bold">
                      UP TO DATE
                    </span>
                  {/if}
                </div>
                {#if info.asset_name}
                  <span class="text-[10px] text-slate-500 font-mono block mt-0.5 truncate" title={info.asset_name}>
                    {info.asset_name}{info.download_size ? ` (${formatMb(info.download_size)})` : ''}
                  </span>
                {/if}
              </div>
            </div>

            {#if info.has_update && !info.asset_url}
              <div class="p-3 rounded-xl bg-amber-950/35 border border-amber-800 text-amber-300 text-[11px] flex items-center space-x-2">
                <Icon name="alert-triangle" class="w-4 h-4 text-amber-400 shrink-0" />
                <span>No pre-built binary asset matches this OS/architecture in this release. Download manually from GitHub Releases.</span>
              </div>
            {/if}

            <!-- Release Notes Body rendered with markdown.ts -->
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <span class="text-slate-400 text-xs font-medium">Release Notes</span>
                {#if info.published_at}
                  <span class="text-slate-500 text-[10px]">
                    Published: {new Date(info.published_at).toLocaleDateString([], { year: 'numeric', month: 'short', day: 'numeric' })}
                  </span>
                {/if}
              </div>
              <div class="p-3.5 bg-[#050811] border border-[#1e293b] rounded-xl max-h-60 overflow-y-auto text-slate-300 text-xs leading-relaxed">
                {@html renderFullMarkdownHtml(info.release_notes || 'No release notes provided.')}
              </div>
            </div>

            {#if updateStatus}
              <div class="p-3 rounded-xl bg-sky-950/40 border border-sky-800 text-sky-300 space-y-2">
                <div class="flex items-center space-x-2">
                  <Icon name="refresh-cw" class="w-4 h-4 animate-spin text-sky-400 shrink-0" />
                  <span>{updateStatus}</span>
                </div>
                <div class="w-full h-1.5 bg-slate-900 rounded-full overflow-hidden">
                  <div class="h-full bg-sky-400 transition-all duration-300" style="width: {updateProgress}%"></div>
                </div>
              </div>
            {/if}
          {/if}
        {:else}
          <!-- Changelog Tab rendered with markdown.ts -->
          <div class="space-y-2">
            {#if isLoadingChangelog}
              <div class="p-8 text-center text-slate-500 flex flex-col items-center justify-center space-y-2">
                <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
                <span>Loading CHANGELOG.md...</span>
              </div>
            {:else}
              <div class="p-4 bg-[#050811] border border-[#1e293b] rounded-xl text-xs text-slate-300 max-h-[380px] overflow-y-auto select-text leading-relaxed">
                {@html renderFullMarkdownHtml(changelogContent)}
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <button
          type="button"
          onclick={() => checkUpdate(true)}
          disabled={isChecking || isUpdating}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition flex items-center space-x-1.5"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isChecking ? 'animate-spin' : ''}" />
          <span>Check Again</span>
        </button>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={() => (appState.isUpdateModalOpen = false)}
            disabled={isUpdating}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition disabled:opacity-40"
          >
            Close
          </button>
          {#if activeTab === 'update' && info?.has_update}
            <button
              type="button"
              onclick={applyUpdate}
              disabled={isUpdating || !info?.asset_url}
              class="px-4 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-emerald-900/30"
              title={!info?.asset_url ? 'No release asset available for this platform' : 'Download and apply update'}
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
