<script lang="ts">
  import Header from './lib/components/layout/Header.svelte';
  import Sidebar from './lib/components/layout/Sidebar.svelte';
  import TabBar from './lib/components/editor/TabBar.svelte';
  import SqlEditor from './lib/components/editor/SqlEditor.svelte';
  import VirtualGrid from './lib/components/grid/VirtualGrid.svelte';
  import TableViewer from './lib/components/table/TableViewer.svelte';
  import ProcesslistView from './lib/components/dba/ProcesslistView.svelte';
  import SafetyModal from './lib/components/modals/SafetyModal.svelte';
  import RemoteLockModal from './lib/components/modals/RemoteLockModal.svelte';
  import ConnectionModal from './lib/components/modals/ConnectionModal.svelte';
  import RecordInspectorModal from './lib/components/modals/RecordInspectorModal.svelte';
  import InsertRowModal from './lib/components/modals/InsertRowModal.svelte';
  import CommandPaletteModal from './lib/components/modals/CommandPaletteModal.svelte';
  import ShortcutsModal from './lib/components/modals/ShortcutsModal.svelte';
  import ToastContainer from './lib/components/common/ToastContainer.svelte';
  import { appState } from './lib/stores/appState.svelte';

  let editorSplitPercent = $state(45);
  let isDraggingSplitter = $state(false);

  function startDrag() {
    isDraggingSplitter = true;
    window.addEventListener('mousemove', onDrag);
    window.addEventListener('mouseup', stopDrag);
  }

  function onDrag(e: MouseEvent) {
    if (!isDraggingSplitter) return;
    const headerHeight = 48 + 36; // Header + TabBar
    const availableHeight = window.innerHeight - headerHeight;
    const currentY = e.clientY - headerHeight;
    const pct = Math.max(15, Math.min(85, (currentY / availableHeight) * 100));
    editorSplitPercent = Math.round(pct);
  }

  function stopDrag() {
    isDraggingSplitter = false;
    window.removeEventListener('mousemove', onDrag);
    window.removeEventListener('mouseup', stopDrag);
  }

  function handleGlobalKeyDown(e: KeyboardEvent) {
    const isCtrl = e.ctrlKey || e.metaKey;

    // Ctrl + Enter: Execute
    if (isCtrl && e.key === 'Enter') {
      e.preventDefault();
      appState.executeCurrentTab();
      return;
    }

    // Ctrl + K: Command Palette
    if (isCtrl && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      appState.isCommandPaletteOpen = !appState.isCommandPaletteOpen;
      return;
    }

    // Ctrl + B: Toggle Sidebar
    if (isCtrl && e.key.toLowerCase() === 'b') {
      e.preventDefault();
      appState.sidebarCollapsed = !appState.sidebarCollapsed;
      return;
    }

    // Ctrl + T: New Tab
    if (isCtrl && e.key.toLowerCase() === 't') {
      e.preventDefault();
      appState.addTab();
      return;
    }

    // Ctrl + W: Close Active Tab
    if (isCtrl && e.key.toLowerCase() === 'w') {
      e.preventDefault();
      appState.closeTab(appState.activeTabId);
      return;
    }

    // ?: Shortcuts Modal (when not in an input/textarea)
    if (
      e.key === '?' &&
      !['INPUT', 'TEXTAREA'].includes((document.activeElement?.tagName || ''))
    ) {
      e.preventDefault();
      appState.isShortcutsModalOpen = !appState.isShortcutsModalOpen;
      return;
    }

    // Escape: Close all open modals
    if (e.key === 'Escape') {
      if (appState.isCommandPaletteOpen) appState.isCommandPaletteOpen = false;
      if (appState.isConnectionModalOpen) appState.isConnectionModalOpen = false;
      if (appState.isRecordInspectorOpen) appState.isRecordInspectorOpen = false;
      if (appState.isInsertRowModalOpen) appState.isInsertRowModalOpen = false;
      if (appState.isShortcutsModalOpen) appState.isShortcutsModalOpen = false;
      if (appState.isCreateDbModalOpen) appState.isCreateDbModalOpen = false;
    }
  }
</script>

<svelte:window onkeydown={handleGlobalKeyDown} />

<div class="h-screen w-screen flex flex-col bg-[#090d16] text-slate-100 overflow-hidden font-sans select-none">
  <!-- Top Application Header -->
  <Header />

  <!-- Main Workspace Area -->
  <div class="flex-1 flex overflow-hidden">
    <!-- Collapsible Sidebar -->
    <Sidebar />

    <!-- Right Content: Tabs, Editor & Results Grid / Table Viewer / Processlist -->
    <main class="flex-1 flex flex-col overflow-hidden bg-[#090d16] min-w-0">
      <!-- Query Tab Bar -->
      <TabBar />

      <!-- Active Tab Content Area -->
      <div class="flex-1 flex flex-col overflow-hidden relative">
        {#if appState.activeTab?.isProcesslist}
          <!-- Processlist / DBA Telemetry Tab -->
          <div class="flex-1 overflow-hidden">
            <ProcesslistView />
          </div>
        {:else if appState.activeTab?.isTableViewer}
          <!-- Dedicated Table Viewer Tab (Data, Structure, Indexes, FK, DDL, Diagnostics) -->
          <div class="flex-1 overflow-hidden">
            <TableViewer />
          </div>
        {:else}
          <!-- SQL Worksheet (Query Editor on Top, Grid on Bottom) -->
          <div style="height: {editorSplitPercent}%;" class="min-h-[100px] overflow-hidden">
            <SqlEditor />
          </div>

          <!-- Draggable Splitter Handle -->
          <div
            onmousedown={startDrag}
            role="separator"
            tabindex="-1"
            aria-orientation="horizontal"
            aria-valuenow={editorSplitPercent}
            aria-label="Resize query editor"
            class="h-1 bg-slate-800 hover:bg-sky-500 cursor-row-resize transition-colors z-10 shrink-0 relative flex items-center justify-center group"
          >
            <div class="w-8 h-1 rounded bg-slate-600 group-hover:bg-sky-400"></div>
          </div>

          <!-- Bottom Grid Results Pane -->
          <div class="flex-1 min-h-[100px] overflow-hidden">
            <VirtualGrid />
          </div>
        {/if}
      </div>
    </main>
  </div>

  <!-- Global Modals -->
  <SafetyModal />
  <RemoteLockModal />
  <ConnectionModal />
  <RecordInspectorModal />
  <InsertRowModal />
  <CommandPaletteModal />
  <ShortcutsModal />
  <ToastContainer />
</div>
