<script lang="ts">
  import Header from './lib/components/layout/Header.svelte';
  import Sidebar from './lib/components/layout/Sidebar.svelte';
  import Footer from './lib/components/layout/Footer.svelte';
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
  import CreateDatabaseModal from './lib/components/modals/CreateDatabaseModal.svelte';
  import TableCreatorModal from './lib/components/modals/TableCreatorModal.svelte';
  import RenameTableModal from './lib/components/modals/RenameTableModal.svelte';
  import DuplicateTableModal from './lib/components/modals/DuplicateTableModal.svelte';
  import JsonCellModal from './lib/components/modals/JsonCellModal.svelte';
  import ViewDesignerModal from './lib/components/modals/ViewDesignerModal.svelte';
  import TriggerDesignerModal from './lib/components/modals/TriggerDesignerModal.svelte';
  import RoutineDesignerModal from './lib/components/modals/RoutineDesignerModal.svelte';
  import EventDesignerModal from './lib/components/modals/EventDesignerModal.svelte';
  import DumpModal from './lib/components/modals/DumpModal.svelte';
  import StructureSyncModal from './lib/components/modals/StructureSyncModal.svelte';
  import RunSqlModal from './lib/components/modals/RunSqlModal.svelte';
  import SnippetsModal from './lib/components/modals/SnippetsModal.svelte';
  import CsvImportModal from './lib/components/modals/CsvImportModal.svelte';
  import PrivilegeMatrixModal from './lib/components/modals/PrivilegeMatrixModal.svelte';
  import DbDiagnosticsModal from './lib/components/modals/DbDiagnosticsModal.svelte';
  import ErdCanvas from './lib/components/erd/ErdCanvas.svelte';
  import AiDrawer from './lib/components/ai/AiDrawer.svelte';
  import AiSettingsModal from './lib/components/modals/AiSettingsModal.svelte';
  import NotesDrawer from './lib/components/notes/NotesDrawer.svelte';
  import NoteComposerModal from './lib/components/modals/NoteComposerModal.svelte';
  import ActivityConsoleDrawer from './lib/components/activity/ActivityConsoleDrawer.svelte';
  import UpdateModal from './lib/components/modals/UpdateModal.svelte';
  import MockDataModal from './lib/components/modals/MockDataModal.svelte';
  import SettingsModal from './lib/components/modals/SettingsModal.svelte';
  import ToastContainer from './lib/components/common/ToastContainer.svelte';
  import NativeDialogModal from './lib/components/modals/NativeDialogModal.svelte';
  import ReconnectBanner from './lib/components/common/ReconnectBanner.svelte';
  import DdlReviewModal from './lib/components/modals/DdlReviewModal.svelte';
  import RefreshAllModal from './lib/components/modals/RefreshAllModal.svelte';
  import ImpactPreviewModal from './lib/components/modals/ImpactPreviewModal.svelte';
  import DataTransferModal from './lib/components/modals/DataTransferModal.svelte';
  import DataSyncModal from './lib/components/modals/DataSyncModal.svelte';
  import AuditDrawer from './lib/components/activity/AuditDrawer.svelte';
  import FkPeekPopover from './lib/components/grid/FkPeekPopover.svelte';
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

    // Ctrl + Enter: Execute whole script (Shift+Enter is handled by CodeMirror to execute statement at cursor)
    if (isCtrl && !e.shiftKey && e.key === 'Enter') {
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
      if (appState.isTableCreatorOpen) appState.isTableCreatorOpen = false;
      if (appState.isRenameTableModalOpen) appState.isRenameTableModalOpen = false;
      if (appState.isDuplicateTableModalOpen) appState.isDuplicateTableModalOpen = false;
      if (appState.isViewDesignerOpen) appState.isViewDesignerOpen = false;
      if (appState.isTriggerDesignerOpen) appState.isTriggerDesignerOpen = false;
      if (appState.isRoutineDesignerOpen) appState.isRoutineDesignerOpen = false;
      if (appState.isEventDesignerOpen) appState.isEventDesignerOpen = false;
      if (appState.isDumpModalOpen) appState.isDumpModalOpen = false;
      if (appState.isStructureSyncModalOpen) appState.isStructureSyncModalOpen = false;
      if (appState.isRunSqlModalOpen) appState.isRunSqlModalOpen = false;
      if (appState.isSnippetsModalOpen) appState.isSnippetsModalOpen = false;
      if (appState.isCsvImportModalOpen) appState.isCsvImportModalOpen = false;
      if (appState.isPrivilegeMatrixOpen) appState.isPrivilegeMatrixOpen = false;
      if (appState.isDbDiagnosticsOpen) appState.isDbDiagnosticsOpen = false;
      if (appState.isAiDrawerOpen) appState.isAiDrawerOpen = false;
      if (appState.isAiSettingsModalOpen) appState.isAiSettingsModalOpen = false;
      if (appState.isNotesDrawerOpen) appState.isNotesDrawerOpen = false;
      if (appState.isNoteComposerModalOpen) appState.isNoteComposerModalOpen = false;
      if (appState.isActivityDrawerOpen) appState.isActivityDrawerOpen = false;
      if (appState.isUpdateModalOpen) appState.isUpdateModalOpen = false;
      if (appState.isMockDataModalOpen) appState.isMockDataModalOpen = false;
      if (appState.isSettingsModalOpen) appState.isSettingsModalOpen = false;
      if (appState.jsonCellModal.isOpen) appState.jsonCellModal.isOpen = false;
      if (appState.isDataTransferOpen) appState.isDataTransferOpen = false;
      if (appState.isDataSyncOpen) appState.isDataSyncOpen = false;
      if (appState.isRefreshAllOpen) appState.isRefreshAllOpen = false;
      if (appState.isImpactPreviewOpen) appState.isImpactPreviewOpen = false;
      if (appState.isAuditDrawerOpen) appState.isAuditDrawerOpen = false;
      if (appState.ddlReviewModal.isOpen) appState.ddlReviewModal.isOpen = false;
      if (appState.fkPeek.isOpen) appState.fkPeek.isOpen = false;
      if (appState.nativeDialog.isOpen) {
        if (appState.nativeDialog.resolve) appState.nativeDialog.resolve(false);
        appState.nativeDialog.isOpen = false;
      }
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
        {:else if appState.activeTab?.isErdCanvas}
          <!-- ERD Canvas Visual Schema Designer Tab -->
          <div class="flex-1 overflow-hidden">
            <ErdCanvas />
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
            id="workspace-pane-splitter"
            onmousedown={startDrag}
            role="separator"
            tabindex="-1"
            aria-orientation="horizontal"
            aria-valuenow={editorSplitPercent}
            aria-label="Resize query editor"
            class="h-[7px] -my-[3px] cursor-row-resize transition-colors z-20 shrink-0 relative flex items-center justify-center group select-none"
          >
            <!-- Uniform 1px divider line -->
            <div class="absolute inset-x-0 h-px bg-slate-800 group-hover:bg-sky-500 transition-colors"></div>
            <!-- Minimalist grab handle pill -->
            <div class="w-8 h-1 rounded-full bg-slate-700/80 group-hover:bg-sky-400 z-10 transition-colors shadow-sm flex items-center justify-center">
              <div class="w-3 h-0.5 rounded-full bg-slate-500 group-hover:bg-sky-100"></div>
            </div>
          </div>

          <!-- Bottom Grid Results Pane -->
          <div class="flex-1 min-h-[100px] overflow-hidden">
            <VirtualGrid />
          </div>
        {/if}
      </div>
    </main>
  </div>

  <!-- Bottom Application Status Bar -->
  <Footer />

  <!-- Global Modals -->
  <SafetyModal />
  <RemoteLockModal />
  <ConnectionModal />
  <RecordInspectorModal />
  <InsertRowModal />
  <CommandPaletteModal />
  <ShortcutsModal />
  <CreateDatabaseModal />
  <TableCreatorModal />
  <RenameTableModal />
  <DuplicateTableModal />
  <ViewDesignerModal />
  <TriggerDesignerModal />
  <RoutineDesignerModal />
  <EventDesignerModal />
  <DumpModal />
  <StructureSyncModal />
  <RunSqlModal />
  <SnippetsModal />
  <CsvImportModal />
  <PrivilegeMatrixModal />
  <DbDiagnosticsModal />
  <JsonCellModal />
  <AiSettingsModal />
  <NoteComposerModal />
  <UpdateModal />
  <MockDataModal />
  <SettingsModal />
  <NativeDialogModal />
  <ReconnectBanner />
  <DdlReviewModal />
  <RefreshAllModal />
  <ImpactPreviewModal />
  <DataTransferModal />
  <DataSyncModal />
  <AuditDrawer />
  <FkPeekPopover />

  {#if appState.isAiDrawerOpen}
    <AiDrawer />
  {/if}
  {#if appState.isNotesDrawerOpen}
    <NotesDrawer />
  {/if}
  <ActivityConsoleDrawer />

  <ToastContainer />
</div>
