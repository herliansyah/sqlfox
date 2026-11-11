<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from './Icon.svelte';

  function manualRetry() {
    appState.attemptReconnect(() => {
      appState.showToast('Daemon still unreachable. Will auto-retry...', 'warning');
    });
  }
</script>

{#if appState.isReconnecting}
  <div class="fixed top-0 left-0 right-0 z-[120] bg-gradient-to-r from-amber-600 to-rose-600 text-white px-4 py-2 text-xs flex items-center justify-between shadow-lg transition-all animate-bounce-subtle font-sans">
    <div class="flex items-center space-x-2">
      <Icon name="loader" class="w-4 h-4 animate-spin text-white shrink-0" />
      <span class="font-bold">Connection to SqlFox daemon lost!</span>
      <span class="text-amber-100 opacity-90">
        Reconnecting in {appState.reconnectCountdown}s (Attempt #{appState.reconnectAttempts})...
      </span>
    </div>

    <div class="flex items-center space-x-2">
      <button
        type="button"
        onclick={manualRetry}
        class="bg-white/20 hover:bg-white/30 text-white font-bold px-3 py-1 rounded text-[11px] transition backdrop-blur-sm"
      >
        Retry Now
      </button>
      <button
        type="button"
        onclick={() => appState.hideReconnectBanner()}
        class="text-white/70 hover:text-white p-1 rounded hover:bg-white/10 transition"
        title="Dismiss warning"
        aria-label="Dismiss warning"
      >
        <Icon name="x" class="w-3.5 h-3.5" />
      </button>
    </div>
  </div>
{/if}
