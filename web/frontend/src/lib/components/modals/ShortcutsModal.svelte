<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  const shortcuts = [
    { key: 'Ctrl / Cmd + Enter', desc: 'Execute selected SQL or statement at cursor' },
    { key: 'Ctrl / Cmd + Alt + Enter / F5', desc: 'Execute all statements in worksheet' },
    { key: 'Ctrl / Cmd + Shift + Enter', desc: 'Explain selected SQL or statement at cursor' },
    { key: 'Ctrl / Cmd + Shift + F', desc: 'Format selected SQL or statement at cursor' },
    { key: 'Ctrl + Space', desc: 'Open schema-aware SQL autocomplete' },
    { key: 'Ctrl / Cmd + Shift + S', desc: 'Open query snippets' },
    { key: 'Ctrl / Cmd + Shift + C / R', desc: 'Commit / rollback active transaction' },
    { key: 'Ctrl / Cmd + F', desc: 'Search inside SQL editor' },
    { key: 'Ctrl / Cmd + Z / Shift + Z', desc: 'Undo / redo editor change' },
    { key: 'Ctrl + K', desc: 'Open Command Palette' },
    { key: 'Ctrl + B', desc: 'Toggle Sidebar visibility' },
    { key: 'Ctrl + T', desc: 'Open new SQL query tab' },
    { key: 'Ctrl + W', desc: 'Close active tab' },
    { key: 'Esc', desc: 'Close active modal / dialog' },
    { key: 'Double-Click Cell', desc: 'Edit data cell inline or copy value' },
    { key: 'Double-Click Row #', desc: 'Open Record Inspector for row' },
  ];
</script>

{#if appState.isShortcutsModalOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm select-none"
    role="dialog"
    tabindex="-1"
    aria-modal="true"
  >
    <button
      tabindex="-1"
      aria-label="Close"
      class="fixed inset-0 bg-transparent cursor-default"
      onclick={() => (appState.isShortcutsModalOpen = false)}
    ></button>

    <div class="w-full max-w-md bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col relative z-10">
      <div class="px-5 py-3.5 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-2">
          <Icon name="sliders" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-sm text-slate-100">Keyboard Shortcuts</h3>
        </div>
        <button
          onclick={() => (appState.isShortcutsModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 overflow-y-auto space-y-2 text-xs font-sans">
        {#each shortcuts as sc}
          <div class="flex items-center justify-between py-1.5 border-b border-slate-800/60">
            <span class="text-slate-300">{sc.desc}</span>
            <kbd class="px-2 py-0.5 rounded bg-slate-900 border border-slate-700 font-mono text-sky-400 text-[11px] font-semibold">
              {sc.key}
            </kbd>
          </div>
        {/each}
      </div>

      <div class="px-5 py-3 border-t border-[#334155] bg-[#0b0f19] text-right">
        <button
          onclick={() => (appState.isShortcutsModalOpen = false)}
          class="px-4 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-200 rounded text-xs font-semibold"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
