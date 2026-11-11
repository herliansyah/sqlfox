<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let tabContextMenu = $state<{
    open: boolean;
    x: number;
    y: number;
    tabId: string;
  } | null>(null);

  function handleTabContextMenu(e: MouseEvent, tabId: string) {
    e.preventDefault();
    tabContextMenu = {
      open: true,
      x: e.clientX,
      y: e.clientY,
      tabId,
    };
  }

  function closeTabMenu() {
    if (tabContextMenu) tabContextMenu.open = false;
  }
</script>

<svelte:window onclick={closeTabMenu} />

<div class="h-9 bg-[#0b0f19] border-b border-[#334155] flex items-center px-2 select-none overflow-x-auto no-scrollbar gap-1 relative">
  {#each appState.tabs as tab}
    {@const isActive = tab.id === appState.activeTabId}
    <div
      class="group relative flex items-center h-8 px-3 rounded-t border-t border-x text-xs cursor-pointer transition-all max-w-[220px] shrink-0 {
        isActive
          ? 'bg-[#090d16] border-t-2 border-t-sky-400 border-x-[#334155] text-sky-400 font-semibold shadow-sm'
          : 'bg-slate-900/60 border-transparent text-slate-400 hover:bg-slate-850 hover:text-slate-200'
      }"
      onclick={() => (appState.activeTabId = tab.id)}
      oncontextmenu={(e) => handleTabContextMenu(e, tab.id)}
      role="tab"
      tabindex="0"
      onkeydown={(e) => e.key === 'Enter' && (appState.activeTabId = tab.id)}
    >
      <!-- Icon Indicator -->
      <span class="mr-2 flex items-center shrink-0">
        {#if tab.isExecuting}
          <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin text-sky-400" />
        {:else if tab.isProcesslist}
          <Icon name="cpu" class="w-3.5 h-3.5 text-sky-400" />
        {:else if tab.isTableViewer}
          <Icon name="table" class="w-3.5 h-3.5 {isActive ? 'text-emerald-400' : 'text-slate-500'}" />
        {:else if tab.title.endsWith('.ddl.sql')}
          <Icon name="file-code" class="w-3.5 h-3.5 text-sky-400" />
        {:else}
          <Icon name="zap" class="w-3.5 h-3.5 {isActive ? 'text-amber-400' : 'text-slate-500'}" />
        {/if}
      </span>

      <!-- Title in Monospace -->
      <span class="truncate font-mono text-[11px] {isActive ? 'text-slate-100' : 'text-slate-400'}">
        {tab.title}
      </span>

      <!-- Close Action -->
      {#if appState.tabs.length > 1}
        <button
          onclick={(e) => {
            e.stopPropagation();
            appState.closeTab(tab.id);
          }}
          class="ml-2 w-4 h-4 rounded hover:bg-slate-700/80 flex items-center justify-center opacity-0 group-hover:opacity-100 text-slate-400 hover:text-white transition"
          title="Close tab (Ctrl+W)"
        >
          <Icon name="x" class="w-3 h-3" />
        </button>
      {/if}
    </div>
  {/each}

  <!-- Add New Tab Button -->
  <button
    onclick={() => appState.addTab()}
    class="w-6 h-6 rounded bg-[#1e293b]/60 hover:bg-slate-700 text-slate-400 hover:text-sky-300 border border-slate-750 flex items-center justify-center text-xs ml-1 transition shadow-sm"
    title="New SQL Query (Ctrl+T)"
  >
    <Icon name="plus" class="w-3.5 h-3.5" />
  </button>

  <!-- Tab Context Menu -->
  {#if tabContextMenu && tabContextMenu.open}
    <div
      style="top: {tabContextMenu.y}px; left: {tabContextMenu.x}px;"
      class="fixed z-50 w-44 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs select-none"
    >
      <div class="py-1">
        <button
          onclick={() => {
            appState.duplicateTab(tabContextMenu!.tabId);
            closeTabMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="copy" class="w-3.5 h-3.5 text-sky-400" />
          <span>Duplicate Tab</span>
        </button>
      </div>

      <div class="py-1">
        <button
          onclick={() => {
            appState.closeOtherTabs(tabContextMenu!.tabId);
            closeTabMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="x" class="w-3.5 h-3.5 text-amber-400" />
          <span>Close Other Tabs</span>
        </button>
        <button
          onclick={() => {
            appState.closeTab(tabContextMenu!.tabId);
            closeTabMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="trash-2" class="w-3.5 h-3.5 text-rose-400" />
          <span>Close Tab</span>
        </button>
      </div>
    </div>
  {/if}
</div>
