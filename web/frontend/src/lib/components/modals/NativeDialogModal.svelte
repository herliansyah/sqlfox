<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

  let inputEl = $state<HTMLInputElement | null>(null);

  $effect(() => {
    if (appState.nativeDialog.isOpen && appState.nativeDialog.type === 'prompt') {
      setTimeout(() => {
        if (inputEl) {
          inputEl.focus();
          inputEl.select();
        }
      }, 50);
    }
  });

  function handleConfirm() {
    if (appState.nativeDialog.type === 'prompt') {
      appState.resolveNativeDialog(appState.nativeDialog.inputValue || '');
    } else {
      appState.resolveNativeDialog(true);
    }
  }

  function handleCancel() {
    if (appState.nativeDialog.type === 'prompt') {
      appState.resolveNativeDialog(null);
    } else {
      appState.resolveNativeDialog(false);
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleConfirm();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      handleCancel();
    }
  }
</script>

{#if appState.nativeDialog.isOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-fade-in"
    onkeydown={handleKeyDown}
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-md rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100"
      role="dialog"
      aria-modal="true"
      aria-labelledby="dialog-title"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/50">
        <h3 id="dialog-title" class="text-sm font-semibold text-slate-200">
          {appState.nativeDialog.title || 'SqlFox Notification'}
        </h3>
      </div>

      <!-- Dialog Body -->
      <div class="p-5 space-y-4">
        <p class="text-xs text-slate-300 leading-relaxed whitespace-pre-wrap">
          {appState.nativeDialog.message}
        </p>

        {#if appState.nativeDialog.type === 'prompt'}
          <div id="app-native-dialog-input-container">
            <input
              id="app-native-dialog-input"
              bind:this={inputEl}
              type="text"
              bind:value={appState.nativeDialog.inputValue}
              class="w-full bg-[#050811] border border-slate-700 rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500"
            />
          </div>
        {/if}
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-slate-900/50 border-t border-slate-800 flex items-center justify-end space-x-2">
        {#if appState.nativeDialog.type === 'confirm' || appState.nativeDialog.type === 'prompt'}
          <button
            type="button"
            onclick={handleCancel}
            class="px-3.5 py-1.5 rounded text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition"
          >
            Cancel
          </button>
        {/if}

        <button
          type="button"
          onclick={handleConfirm}
          class="px-4 py-1.5 rounded text-xs font-bold transition shadow-sm {appState.nativeDialog.type === 'confirm'
            ? 'bg-rose-600 hover:bg-rose-500 text-white'
            : 'bg-sky-500 hover:bg-sky-400 text-slate-950'}"
        >
          {appState.nativeDialog.type === 'confirm'
            ? 'Confirm'
            : appState.nativeDialog.type === 'prompt'
              ? 'Save'
              : 'OK'}
        </button>
      </div>
    </div>
  </div>
{/if}
