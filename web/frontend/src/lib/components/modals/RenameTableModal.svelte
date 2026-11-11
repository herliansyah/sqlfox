<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let newName = $state('');
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isRenameTableModalOpen) {
      newName = appState.tableToRename || '';
      isSubmitting = false;
    }
  });

  async function handleRename(e: Event) {
    e.preventDefault();
    const oldTbl = appState.tableToRename;
    const next = newName.trim();
    if (!oldTbl || !next || oldTbl === next) return;

    isSubmitting = true;
    try {
      await appState.renameTable(oldTbl, next);
      appState.isRenameTableModalOpen = false;
    } finally {
      isSubmitting = false;
    }
  }
</script>

{#if appState.isRenameTableModalOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm select-none"
    role="dialog"
    aria-modal="true"
  >
    <button
      tabindex="-1"
      aria-label="Close"
      class="fixed inset-0 bg-transparent cursor-default"
      onclick={() => (appState.isRenameTableModalOpen = false)}
    ></button>

    <div class="w-full max-w-sm bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col relative z-10 animate-in fade-in zoom-in-95 duration-150">
      <div class="px-5 py-3.5 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-2">
          <Icon name="edit" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-sm text-slate-100">Rename Table</h3>
        </div>
        <button
          onclick={() => (appState.isRenameTableModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleRename} class="p-5 space-y-4 text-xs font-sans">
        <div class="space-y-1">
          <span class="text-slate-400">Current Table Name:</span>
          <div class="font-mono text-sm font-bold text-sky-400 bg-[#090d16] border border-slate-800 rounded px-2.5 py-1.5">
            {appState.tableToRename}
          </div>
        </div>

        <div class="space-y-1.5">
          <label for="rename-target-name" class="block font-semibold text-slate-300">
            New Table Name <span class="text-rose-400">*</span>
          </label>
          <input
            id="rename-target-name"
            type="text"
            required
            pattern="^[a-zA-Z0-9_-]+$"
            placeholder="e.g. orders_archive"
            bind:value={newName}
            class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-2 text-slate-100 font-mono text-xs outline-none transition"
          />
        </div>

        <div class="pt-2 border-t border-[#1e293b] flex items-center justify-end space-x-2">
          <button
            type="button"
            onclick={() => (appState.isRenameTableModalOpen = false)}
            class="px-3.5 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 rounded font-semibold text-xs transition"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitting || !newName.trim() || newName.trim() === appState.tableToRename}
            class="px-4 py-1.5 bg-sky-500 hover:bg-sky-400 disabled:opacity-50 text-slate-950 font-bold rounded text-xs transition flex items-center space-x-1.5 shadow"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Renaming...</span>
            {:else}
              <span>Apply Rename</span>
            {/if}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
