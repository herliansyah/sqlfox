<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let targetTableName = $state('');
  let copyData = $state(true);
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isDuplicateTableModalOpen) {
      targetTableName = `${appState.tableToDuplicate}_copy`;
      copyData = true;
      isSubmitting = false;
    }
  });

  async function handleDuplicate(e: Event) {
    e.preventDefault();
    const src = appState.tableToDuplicate;
    const tgt = targetTableName.trim();
    if (!src || !tgt || src === tgt) return;

    isSubmitting = true;
    try {
      await appState.duplicateTable(src, tgt, copyData);
      appState.isDuplicateTableModalOpen = false;
    } finally {
      isSubmitting = false;
    }
  }
</script>

{#if appState.isDuplicateTableModalOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm select-none"
    role="dialog"
    aria-modal="true"
  >
    <button
      tabindex="-1"
      aria-label="Close"
      class="fixed inset-0 bg-transparent cursor-default"
      onclick={() => (appState.isDuplicateTableModalOpen = false)}
    ></button>

    <div class="w-full max-w-sm bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col relative z-10 animate-in fade-in zoom-in-95 duration-150">
      <div class="px-5 py-3.5 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-2">
          <Icon name="copy" class="w-4 h-4 text-emerald-400" />
          <h3 class="font-bold text-sm text-slate-100">Duplicate Table</h3>
        </div>
        <button
          onclick={() => (appState.isDuplicateTableModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleDuplicate} class="p-5 space-y-4 text-xs font-sans">
        <div class="space-y-1">
          <span class="text-slate-400">Source Table:</span>
          <div class="font-mono text-sm font-bold text-sky-400 bg-[#090d16] border border-slate-800 rounded px-2.5 py-1.5">
            {appState.tableToDuplicate}
          </div>
        </div>

        <div class="space-y-1.5">
          <label for="dup-target-name" class="block font-semibold text-slate-300">
            Target Table Name <span class="text-rose-400">*</span>
          </label>
          <input
            id="dup-target-name"
            type="text"
            required
            pattern="^[a-zA-Z0-9_-]+$"
            placeholder="e.g. users_backup"
            bind:value={targetTableName}
            class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-2 text-slate-100 font-mono text-xs outline-none transition"
          />
        </div>

        <label class="flex items-center space-x-2 p-2 rounded bg-[#090d16] border border-slate-800 cursor-pointer">
          <input
            type="checkbox"
            bind:checked={copyData}
            class="rounded bg-slate-900 border-[#334155] text-emerald-500 focus:ring-0"
          />
          <span class="text-slate-200">Copy table data rows as well</span>
        </label>

        <div class="pt-2 border-t border-[#1e293b] flex items-center justify-end space-x-2">
          <button
            type="button"
            onclick={() => (appState.isDuplicateTableModalOpen = false)}
            class="px-3.5 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 rounded font-semibold text-xs transition"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitting || !targetTableName.trim() || targetTableName.trim() === appState.tableToDuplicate}
            class="px-4 py-1.5 bg-emerald-500 hover:bg-emerald-400 disabled:opacity-50 text-slate-950 font-bold rounded text-xs transition flex items-center space-x-1.5 shadow"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Duplicating...</span>
            {:else}
              <span>Duplicate Table</span>
            {/if}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
