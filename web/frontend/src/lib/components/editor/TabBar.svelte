<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let containerWidth = $state(0);
  let showOverflowDropdown = $state(false);
  let overflowSearchQuery = $state('');
  let overflowActiveIndex = $state(0);
  let overflowListEl = $state<HTMLDivElement | null>(null);
  let overflowBtnRef = $state<HTMLButtonElement | null>(null);
  let dropdownCoords = $state<{ top: number; left: number }>({ top: 40, left: 0 });

  let tabContextMenu = $state<{
    open: boolean;
    x: number;
    y: number;
    tabId: string;
  } | null>(null);

  function hasDraft(tab: { sql: string; isTableViewer?: boolean; isProcesslist?: boolean; isErdCanvas?: boolean; isFlightRecorder?: boolean; isVariables?: boolean }): boolean {
    return !tab.isTableViewer && !tab.isProcesslist && !tab.isErdCanvas && !tab.isFlightRecorder && !tab.isVariables && tab.sql.trim().length > 0;
  }

  // Enterprise tab width estimator: font-mono text-[11px] + padding + icon + close button
  function estimateTabWidth(title: string): number {
    const charWidth = 7.2;
    const chromeWidth = 64; // icon + close button + padding
    return Math.max(105, Math.min(200, Math.round(title.length * charWidth + chromeWidth)));
  }

  // Calculate how many tabs fit in the visible tab strip without spilling off-screen
  let visibleCount = $derived.by(() => {
    if (containerWidth <= 0 || appState.tabs.length <= 1) return appState.tabs.length;

    const addBtnWidth = 32;
    const overflowBtnWidth = 72; // width for overflow button (» count)
    const gap = 4;
    const safetyMargin = 16;

    // Check if ALL tabs fit comfortably without overflow button
    let totalAll = 0;
    for (const tab of appState.tabs) {
      totalAll += estimateTabWidth(tab.title) + gap;
    }
    if (totalAll + addBtnWidth + safetyMargin <= containerWidth) {
      return appState.tabs.length;
    }

    // When tabs exceed container width, reserve space for the overflow dropdown button
    const availableWidth = containerWidth - addBtnWidth - overflowBtnWidth - safetyMargin;
    let accumulated = 0;
    let count = 0;
    for (let i = 0; i < appState.tabs.length; i++) {
      const tabW = estimateTabWidth(appState.tabs[i].title) + gap;
      if (accumulated + tabW <= availableWidth) {
        accumulated += tabW;
        count++;
      } else {
        break;
      }
    }
    return Math.max(1, count);
  });

  // Split tabs into visible and overflow subsets
  let visibleTabs = $derived(appState.tabs.slice(0, visibleCount));
  let overflowTabs = $derived(appState.tabs.slice(visibleCount));

  // Search filter inside overflow dropdown
  let filteredOverflowTabs = $derived.by(() => {
    const q = overflowSearchQuery.trim().toLowerCase();
    if (!q) return overflowTabs;
    return overflowTabs.filter(
      (t) =>
        t.title.toLowerCase().includes(q) ||
        (t.database && t.database.toLowerCase().includes(q)) ||
        (t.tableName && t.tableName.toLowerCase().includes(q))
    );
  });

  // Enterprise Best Practice: Active tab is GUARANTEED to be in the visible tab strip!
  $effect(() => {
    const vCount = visibleCount;
    const activeId = appState.activeTabId;
    if (!activeId || vCount <= 0 || appState.tabs.length <= vCount) return;

    const activeIdx = appState.tabs.findIndex((t) => t.id === activeId);
    if (activeIdx >= vCount) {
      untrack(() => {
        // Bring active tab into the visible slot (vCount - 1)
        const [activeTab] = appState.tabs.splice(activeIdx, 1);
        appState.tabs.splice(vCount - 1, 0, activeTab);
        appState.tabs = [...appState.tabs];
        appState.saveTabsState();
      });
    }
  });

  function updateDropdownCoords() {
    if (!overflowBtnRef) return;
    const rect = overflowBtnRef.getBoundingClientRect();
    const dropdownWidth = 288; // w-72 = 18rem = 288px
    const left = Math.max(10, Math.min(rect.right - dropdownWidth, window.innerWidth - dropdownWidth - 10));
    dropdownCoords = {
      top: rect.bottom + 4,
      left,
    };
  }

  function toggleOverflowDropdown(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (!showOverflowDropdown) {
      updateDropdownCoords();
      overflowActiveIndex = 0;
      showOverflowDropdown = true;
      // Focus the panel so arrow keys work without an extra click.
      tick().then(() => document.getElementById('tab-overflow-dropdown')?.focus());
    } else {
      showOverflowDropdown = false;
    }
  }

  $effect(() => {
    if (overflowActiveIndex >= filteredOverflowTabs.length) {
      overflowActiveIndex = Math.max(0, filteredOverflowTabs.length - 1);
    }
  });

  function scrollOverflowActiveIntoView() {
    tick().then(() =>
      overflowListEl?.querySelector('[data-overflow-active="true"]')?.scrollIntoView({ block: 'nearest' })
    );
  }

  function handleOverflowKeydown(e: KeyboardEvent) {
    const count = filteredOverflowTabs.length;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (count) {
        overflowActiveIndex = (overflowActiveIndex + 1) % count;
        scrollOverflowActiveIntoView();
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (count) {
        overflowActiveIndex = (overflowActiveIndex - 1 + count) % count;
        scrollOverflowActiveIntoView();
      }
    } else if (e.key === 'Home') {
      e.preventDefault();
      overflowActiveIndex = 0;
      scrollOverflowActiveIntoView();
    } else if (e.key === 'End') {
      e.preventDefault();
      overflowActiveIndex = Math.max(0, count - 1);
      scrollOverflowActiveIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const tab = filteredOverflowTabs[overflowActiveIndex];
      if (tab) selectOverflowTab(tab.id);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      showOverflowDropdown = false;
      overflowBtnRef?.focus();
    } else if (e.key === 'Tab') {
      showOverflowDropdown = false;
    }
  }

  function selectOverflowTab(tabId: string) {
    appState.activeTabId = tabId;
    showOverflowDropdown = false;
    overflowSearchQuery = '';
  }

  function closeTabFromOverflow(tabId: string) {
    appState.closeTab(tabId);
    if (appState.tabs.length <= visibleCount) {
      showOverflowDropdown = false;
    }
  }

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

  function handleMiddleClick(e: MouseEvent, tabId: string) {
    if (e.button === 1) {
      // Middle click to close tab
      e.preventDefault();
      appState.closeTab(tabId);
    }
  }
</script>

<svelte:window
  onclick={() => {
    closeTabMenu();
    showOverflowDropdown = false;
  }}
  onkeydown={(e) => {
    if (e.key === 'Escape') {
      showOverflowDropdown = false;
      closeTabMenu();
    }
  }}
  onresize={() => {
    if (showOverflowDropdown) {
      updateDropdownCoords();
    }
  }}
/>

<div
  bind:clientWidth={containerWidth}
  id="tabs-bar"
  class="h-9 bg-[#0b0f19] border-b border-slate-800 flex items-center px-2 select-none justify-between relative overflow-hidden"
>
  <!-- Visible Tabs Strip (fits strictly within available screen width) -->
  <div id="tabs-container" class="flex items-center min-w-0 flex-1 overflow-hidden gap-1">
    {#each visibleTabs as tab (tab.id)}
      {@const isActive = tab.id === appState.activeTabId}
      <div
        class="group relative flex items-center h-8 px-3 rounded-t border-t border-x text-xs cursor-pointer transition-all max-w-[200px] min-w-[100px] shrink-0 {
          isActive
            ? 'bg-[#090d16] border-t border-x border-slate-800 text-sky-400 font-semibold shadow-sm'
            : 'bg-slate-900/60 border-t border-x border-transparent text-slate-400 hover:bg-slate-850 hover:text-slate-200'
        }"
        onclick={() => (appState.activeTabId = tab.id)}
        oncontextmenu={(e) => handleTabContextMenu(e, tab.id)}
        onmouseup={(e) => handleMiddleClick(e, tab.id)}
        role="tab"
        tabindex="0"
        title={tab.title + (tab.database ? ` (${tab.database})` : '')}
        onkeydown={(e) => e.key === 'Enter' && (appState.activeTabId = tab.id)}
      >
        {#if isActive}
          <div class="absolute top-0 left-0 right-0 h-[2px] bg-sky-400 rounded-t pointer-events-none"></div>
        {/if}
        <!-- Icon Indicator -->
        <span class="mr-2 flex items-center shrink-0">
          {#if tab.isExecuting}
            <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin text-sky-400" />
          {:else if tab.isProcesslist}
            <Icon name="cpu" class="w-3.5 h-3.5 text-sky-400" />
          {:else if tab.isErdCanvas}
            <Icon name="layers" class="w-3.5 h-3.5 {isActive ? 'text-indigo-400' : 'text-slate-500'}" />
          {:else if tab.isTableViewer}
            <Icon name="table" class="w-3.5 h-3.5 {isActive ? 'text-emerald-400' : 'text-slate-500'}" />
          {:else}
            <Icon name="file-code" class="w-3.5 h-3.5 {isActive ? 'text-sky-400' : 'text-slate-500'}" />
          {/if}
        </span>

        <!-- Title in Monospace -->
        <span class="truncate font-mono text-[11px] {isActive ? 'text-slate-100' : 'text-slate-400'}">
          {tab.title}
        </span>

        {#if hasDraft(tab)}
          <span class="ml-1.5 w-1.5 h-1.5 rounded-full bg-amber-400 shrink-0" title="SQL draft"></span>
        {/if}

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
      id="add-tab-btn"
      onclick={() => appState.addTab()}
      class="w-6 h-6 rounded bg-[#1e293b]/60 hover:bg-slate-700 text-slate-400 hover:text-sky-300 border border-slate-700/80 flex items-center justify-center text-xs ml-1 shrink-0 transition shadow-sm"
      title="New SQL Query (Ctrl+T)"
    >
      <Icon name="plus" class="w-3.5 h-3.5" />
    </button>
  </div>

  <!-- Tab Overflow Dropdown Menu Trigger (Enterprise Pattern) -->
  <div id="tab-overflow-wrapper" class="relative flex items-center pl-1.5 shrink-0 z-20 {overflowTabs.length === 0 ? 'hidden' : ''}">
    <button
      id="tab-overflow-btn"
      bind:this={overflowBtnRef}
      type="button"
      onclick={toggleOverflowDropdown}
      class="h-6 px-2 rounded bg-slate-800/80 hover:bg-slate-700 text-sky-400 hover:text-white border border-slate-700/80 flex items-center space-x-1.5 text-xs font-mono transition shadow-sm cursor-pointer"
      title="More Tabs ({overflowTabs.length} hidden)"
    >
      <Icon name="chevrons-right" class="w-3.5 h-3.5" />
      <span id="tab-overflow-count" class="font-bold text-[11px] text-sky-300">{overflowTabs.length}</span>
    </button>
  </div>

  <!-- Tab Context Menu -->
  {#if tabContextMenu && tabContextMenu.open}
    <div
      id="tab-context-menu"
      style="top: {tabContextMenu.y}px; left: {tabContextMenu.x}px;"
      class="fixed z-[110] w-44 bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl py-1 divide-y divide-slate-800 text-xs select-none"
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
        <button
          onclick={() => {
            const targetTab = appState.tabs.find((t) => t.id === tabContextMenu!.tabId);
            if (targetTab) {
              appState.saveQueryToNote(targetTab.sql, targetTab.title);
            }
            closeTabMenu();
          }}
          class="w-full text-left px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2 text-slate-200"
        >
          <Icon name="bookmark" class="w-3.5 h-3.5 text-amber-400" />
          <span>Save as Note</span>
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

<!-- Floating Enterprise Tab Overflow Dropdown Menu (Fixed coordinates, never clipped by parent overflow) -->
{#if showOverflowDropdown}
  <!-- Fixed Backdrop to close on outside click -->
  <div
    tabindex="-1"
    role="presentation"
    class="fixed inset-0 z-[95] bg-transparent cursor-default"
    onclick={(e) => {
      e.stopPropagation();
      showOverflowDropdown = false;
    }}
  ></div>

  <div
    id="tab-overflow-dropdown"
    style="top: {dropdownCoords.top}px; left: {dropdownCoords.left}px;"
    class="fixed w-72 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-[100] divide-y divide-slate-800 text-xs overflow-hidden animate-in fade-in zoom-in-95 duration-100"
    onclick={(e) => e.stopPropagation()}
    role="menu"
    tabindex="-1"
    onkeydown={handleOverflowKeydown}
  >
    <!-- Header -->
    <div class="px-3 py-2 bg-slate-900/90 flex items-center justify-between">
      <div class="flex items-center space-x-1.5">
        <span class="font-bold text-[11px] text-slate-300 uppercase tracking-wider">Hidden Tabs</span>
        <span class="px-1.5 py-0.5 rounded-full bg-sky-500/20 text-sky-400 font-mono text-[10px] font-semibold">
          {overflowTabs.length}
        </span>
      </div>
      <span class="text-[10px] text-slate-500">
        Total {appState.tabs.length} tabs
      </span>
    </div>

    <!-- Search input if > 3 overflow tabs -->
    {#if overflowTabs.length > 3}
      <div class="p-1.5 bg-[#0b0f19]">
        <div class="relative flex items-center">
          <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2 pointer-events-none" />
          <input
            type="text"
            bind:value={overflowSearchQuery}
            oninput={() => (overflowActiveIndex = 0)}
            role="combobox"
            aria-expanded="true"
            aria-controls="tab-overflow-listbox"
            aria-autocomplete="list"
            placeholder="Filter hidden tabs... (↑↓ Enter)"
            class="w-full bg-[#1e293b]/70 border border-slate-700/80 rounded pl-7 pr-2 py-1 text-[11px] text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500/70"
          />
        </div>
      </div>
    {/if}

    <!-- Dropdown Tab Items -->
    <div id="tab-overflow-listbox" bind:this={overflowListEl} role="listbox" class="max-h-72 overflow-y-auto divide-y divide-slate-800/40 py-0.5">
      {#if filteredOverflowTabs.length === 0}
        <div class="px-3 py-4 text-center text-slate-500 text-xs italic">
          No matching tabs found
        </div>
      {:else}
        {#each filteredOverflowTabs as tab, idx (tab.id)}
          {@const isActiveOverflow = idx === overflowActiveIndex}
          <div
            data-overflow-active={isActiveOverflow}
            class="flex items-center justify-between px-2.5 py-1.5 transition-colors group cursor-pointer {isActiveOverflow
              ? 'bg-slate-800/90 ring-1 ring-inset ring-sky-500/40'
              : 'hover:bg-slate-800/80'}"
            onmouseenter={() => (overflowActiveIndex = idx)}
            onclick={() => selectOverflowTab(tab.id)}
            onkeydown={(e) => e.key === 'Enter' && selectOverflowTab(tab.id)}
            role="option"
            aria-selected={tab.id === appState.activeTabId}
            tabindex="-1"
          >
            <!-- Tab Icon & Title -->
            <div class="flex items-center space-x-2 min-w-0 flex-1 pr-2">
              <span class="flex items-center shrink-0">
                {#if tab.isExecuting}
                  <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin text-sky-400" />
                {:else if tab.isProcesslist}
                  <Icon name="cpu" class="w-3.5 h-3.5 text-sky-400" />
                {:else if tab.isErdCanvas}
                  <Icon name="layers" class="w-3.5 h-3.5 text-indigo-400" />
                {:else if tab.isTableViewer}
                  <Icon name="table" class="w-3.5 h-3.5 text-emerald-400" />
                {:else}
                  <Icon name="file-code" class="w-3.5 h-3.5 text-sky-400" />
                {/if}
              </span>

              <span class="truncate font-mono text-[11px] text-slate-300 group-hover:text-white" title={tab.title}>
                {tab.title}
              </span>

              {#if hasDraft(tab)}
                <span class="w-1.5 h-1.5 rounded-full bg-amber-400 shrink-0" title="SQL draft"></span>
              {/if}

              {#if tab.database}
                <span class="shrink-0 px-1 py-0.2 rounded text-[9px] font-mono bg-slate-800 text-amber-400/80 border border-slate-700/60" title="Target Database">
                  {tab.database}
                </span>
              {/if}
            </div>

            <!-- Close Tab Button -->
            <button
              type="button"
              onclick={(e) => {
                e.stopPropagation();
                closeTabFromOverflow(tab.id);
              }}
              class="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-rose-500/20 text-slate-400 hover:text-rose-400 transition shrink-0"
              title="Close Tab"
            >
              <Icon name="x" class="w-3 h-3" />
            </button>
          </div>
        {/each}
      {/if}
    </div>
  </div>
{/if}
