<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  const sections = [
    {
      title: 'Query & Execution',
      items: [
        { key: 'Ctrl / Cmd + Enter', desc: 'Execute selected SQL or statement at cursor' },
        { key: 'Ctrl / Cmd + Alt + Enter / F5', desc: 'Execute all statements in worksheet' },
        { key: 'Ctrl / Cmd + Shift + Enter', desc: 'Explain query plan / Impact Preview' },
        { key: 'Ctrl / Cmd + Shift + F', desc: 'Format SQL statement or selection' },
        { key: 'Ctrl + Space', desc: 'Trigger schema-aware autocomplete' },
        { key: 'Ctrl / Cmd + Shift + S', desc: 'Open Query Snippets library' },
        { key: 'Ctrl / Cmd + Shift + C / R', desc: 'Commit / Rollback active transaction' },
      ],
    },
    {
      title: 'Navigation & Search',
      items: [
        { key: 'Ctrl / Cmd + K', desc: 'Open Command Palette' },
        { key: 'Ctrl / Cmd + P', desc: 'Quick Schema Search across tables & columns' },
        { key: '/', desc: 'Focus Sidebar Schema Filter (when outside editor)' },
        { key: 'Ctrl / Cmd + F', desc: 'Find in SQL editor or Table Data filter' },
        { key: 'Ctrl / Cmd + B', desc: 'Toggle Sidebar explorer' },
        { key: 'Ctrl / Cmd + L', desc: 'Toggle AI Copilot drawer' },
        { key: 'Ctrl / Cmd + `', desc: 'Toggle System Activity Console' },
        { key: 'Double-Click Cell / Row #', desc: 'Inline edit cell or open Record Inspector' },
      ],
    },
    {
      title: 'Tabs & Workspaces',
      items: [
        { key: 'Ctrl / Cmd + T', desc: 'Open new SQL worksheet tab' },
        { key: 'Ctrl / Cmd + W', desc: 'Close active tab' },
        { key: 'Ctrl / Cmd + ,', desc: 'Open Settings & Cloud Sync' },
        { key: 'Ctrl / Cmd + Shift + N', desc: 'Open new workspace window' },
        { key: 'F1', desc: 'Show Keyboard Shortcuts cheatsheet' },
        { key: 'Esc', desc: 'Cancel active query or close modal / drawer' },
      ],
    },
  ];
</script>

{#if appState.isShortcutsModalOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 select-none"
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

    <div class="w-full max-w-2xl bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[85vh] relative z-10">
      <div class="px-5 py-3.5 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-2">
          <Icon name="keyboard" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-sm text-slate-100">Keyboard Shortcuts &amp; Navigation</h3>
        </div>
        <button
          onclick={() => (appState.isShortcutsModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 overflow-y-auto space-y-5 text-xs font-sans">
        {#each sections as sec}
          <div class="space-y-2">
            <h4 class="text-[10.5px] font-bold uppercase tracking-wider text-sky-400 border-b border-slate-800 pb-1">
              {sec.title}
            </h4>
            <div class="space-y-1">
              {#each sec.items as sc}
                <div class="flex items-center justify-between py-1.5 border-b border-slate-800/40 last:border-b-0">
                  <span class="text-slate-300 pr-3">{sc.desc}</span>
                  <kbd class="px-2 py-0.5 rounded bg-slate-900 border border-slate-700 font-mono text-sky-300 text-[11px] font-semibold shrink-0">
                    {sc.key}
                  </kbd>
                </div>
              {/each}
            </div>
          </div>
        {/each}
      </div>

      <div class="px-5 py-3 border-t border-[#334155] bg-[#0b0f19] flex items-center justify-between text-[11px]">
        <div class="flex items-center space-x-3 text-slate-400">
          <span><strong class="text-sky-400">Pro tip:</strong> Press <kbd class="px-1 py-0.2 rounded bg-slate-900 border border-slate-700 font-mono">Esc</kbd> anytime to cancel running queries or dismiss popups.</span>
          <a
            href="https://github.com/herliansyah/sqlfox/blob/main/docs/USER_GUIDE.md"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sky-400 hover:underline font-semibold shrink-0"
          >
            User Guide ↗
          </a>
        </div>
        <button
          onclick={() => (appState.isShortcutsModalOpen = false)}
          class="px-4 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-200 rounded text-xs font-semibold shrink-0 ml-3"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
