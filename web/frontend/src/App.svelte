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
  import SchemaSearchModal from './lib/components/modals/SchemaSearchModal.svelte';
  import DdlViewModal from './lib/components/modals/DdlViewModal.svelte';
  import SafetyInspectionModal from './lib/components/modals/SafetyInspectionModal.svelte';
  import LockInspectorModal from './lib/components/modals/LockInspectorModal.svelte';
  import SmartSeedModal from './lib/components/modals/SmartSeedModal.svelte';
  import NoteMarkdownView from './lib/components/notes/NoteMarkdownView.svelte';
  import Icon from './lib/components/common/Icon.svelte';
  import { copyText } from './lib/utils/clipboard';
  import { appState } from './lib/stores/appState.svelte';
  import { onDestroy, tick } from 'svelte';
  import { formatSQL } from './lib/utils/sqlFormatter';

  let editorSplitPercent = $state(Math.max(15, Math.min(85, Number(localStorage.getItem('sqlfox_svelte_editor_split')) || 45)));
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
    localStorage.setItem('sqlfox_svelte_editor_split', String(editorSplitPercent));
  }

  // Release window listeners if the component unmounts mid-drag.
  $effect(() => {
    return () => {
      window.removeEventListener('mousemove', onDrag);
      window.removeEventListener('mouseup', stopDrag);
    };
  });

  function resetSplitter() {
    editorSplitPercent = 50;
    localStorage.setItem('sqlfox_svelte_editor_split', '50');
  }

  onDestroy(stopDrag);

  function handleGlobalKeyDown(e: KeyboardEvent) {
    const isCtrl = e.ctrlKey || e.metaKey;
    const target = e.target as HTMLElement | null;
    const inCodeMirror = !!target?.closest('.cm-editor');
    const inTextControl = inCodeMirror || !!target?.closest('input, textarea, [contenteditable="true"]');
    if (e.defaultPrevented) return;

    if (isCtrl && e.key.toLowerCase() === 's' && target?.closest('[data-table-notes-editor]')) {
      e.preventDefault();
      window.dispatchEvent(new CustomEvent('sqlfox:save-table-notes'));
      return;
    }

    // CodeMirror owns editor shortcuts while focused; never dispatch them twice globally.
    if (e.key === 'F5' && !inTextControl) {
      e.preventDefault();
      appState.executeCurrentTab();
      return;
    }
    if (isCtrl && e.key === 'Enter' && !inCodeMirror) {
      e.preventDefault();
      if (e.altKey) appState.executeCurrentTab();
      else if (e.shiftKey) appState.explainCurrentTab(appState.getEditorTargetSql() ?? undefined);
      else appState.executeCurrentTab(appState.getEditorTargetSql() ?? undefined);
      return;
    }
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 'f' && !inCodeMirror) {
      e.preventDefault();
      if (appState.activeTab?.sql) {
        appState.activeTab.sql = formatSQL(appState.activeTab.sql);
        appState.showToast('Formatted SQL worksheet', 'info');
      }
      return;
    }
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 's') {
      e.preventDefault();
      appState.openSnippetsModal();
      return;
    }
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 'c' && appState.isTransactionActive) {
      e.preventDefault();
      appState.executeTransaction('COMMIT');
      return;
    }
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 'r' && appState.isTransactionActive) {
      e.preventDefault();
      appState.executeTransaction('ROLLBACK');
      return;
    }

    // Ctrl + K: Command Palette
    if (isCtrl && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      appState.isCommandPaletteOpen = !appState.isCommandPaletteOpen;
      return;
    }

    // Ctrl + P: Cross-DB Schema Search
    if (isCtrl && !e.shiftKey && e.key.toLowerCase() === 'p') {
      e.preventDefault();
      appState.isSchemaSearchOpen = !appState.isSchemaSearchOpen;
      return;
    }

    // Ctrl + L: Toggle AI Copilot Drawer
    if (isCtrl && !e.shiftKey && e.key.toLowerCase() === 'l') {
      e.preventDefault();
      appState.toggleAiDrawer();
      return;
    }

    // Ctrl + `: Toggle Activity Console
    if (isCtrl && (e.code === 'Backquote' || e.key === '`')) {
      e.preventDefault();
      appState.toggleActivityDrawer();
      return;
    }

    // Ctrl + ,: Global Settings Modal
    if (isCtrl && e.key === ',') {
      e.preventDefault();
      appState.isSettingsModalOpen = !appState.isSettingsModalOpen;
      return;
    }

    // Ctrl + Shift + N: Open New Workspace Window
    if (isCtrl && e.shiftKey && e.key.toLowerCase() === 'n') {
      e.preventDefault();
      window.open(window.location.href, '_blank', 'noopener,noreferrer');
      return;
    }

    // Ctrl + F in Table Viewer: Focus table filter bar
    if (isCtrl && !e.shiftKey && e.key.toLowerCase() === 'f' && appState.activeTab?.isTableViewer && !inCodeMirror) {
      e.preventDefault();
      window.dispatchEvent(new CustomEvent('sqlfox:focus-table-filter'));
      return;
    }

    // Ctrl + B: Toggle Sidebar
    if (isCtrl && e.key.toLowerCase() === 'b') {
      e.preventDefault();
      appState.toggleSidebar();
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

    // /: Focus Sidebar Schema Filter
    if (e.key === '/' && !isCtrl && !e.altKey && !inTextControl) {
      e.preventDefault();
      if (appState.sidebarCollapsed) appState.toggleSidebar(false);
      tick().then(() => document.getElementById('schema-search-input')?.focus());
      return;
    }

    // F1 or ?: Shortcuts Modal
    if (e.key === 'F1' || (e.key === '?' && !inTextControl)) {
      e.preventDefault();
      appState.isShortcutsModalOpen = !appState.isShortcutsModalOpen;
      return;
    }

    // Escape: Resolve active safety/native dialog or close modals / cancel query
    if (e.key === 'Escape') {
      if (appState.nativeDialog.isOpen) {
        if (appState.nativeDialog.resolve) {
          appState.nativeDialog.resolve(appState.nativeDialog.type === 'prompt' ? null : false);
        }
        appState.nativeDialog.isOpen = false;
        return;
      }
      if (appState.safetyChallenge.isOpen) {
        appState.cancelSafetyChallenge();
        return;
      }

      const anyOverlayOpen =
        appState.isCommandPaletteOpen ||
        appState.isSchemaSearchOpen ||
        appState.isSmartSeedModalOpen ||
        appState.isLockInspectorOpen ||
        appState.ddlViewModal.isOpen ||
        appState.safetyInspectionModal.isOpen ||
        appState.isConnectionModalOpen ||
        appState.isRecordInspectorOpen ||
        appState.isInsertRowModalOpen ||
        appState.isShortcutsModalOpen ||
        appState.isCreateDbModalOpen ||
        appState.isTableCreatorOpen ||
        appState.isRenameTableModalOpen ||
        appState.isDuplicateTableModalOpen ||
        appState.isViewDesignerOpen ||
        appState.isTriggerDesignerOpen ||
        appState.isRoutineDesignerOpen ||
        appState.isEventDesignerOpen ||
        appState.isDumpModalOpen ||
        appState.isStructureSyncModalOpen ||
        appState.isRunSqlModalOpen ||
        appState.isSnippetsModalOpen ||
        appState.isCsvImportModalOpen ||
        appState.isPrivilegeMatrixOpen ||
        appState.isDbDiagnosticsOpen ||
        appState.isAiDrawerOpen ||
        appState.isAiSettingsModalOpen ||
        appState.isNotesDrawerOpen ||
        appState.isNoteComposerModalOpen ||
        appState.isActivityDrawerOpen ||
        appState.isUpdateModalOpen ||
        appState.isMockDataModalOpen ||
        appState.isSettingsModalOpen ||
        appState.jsonCellModal.isOpen ||
        appState.isDataTransferOpen ||
        appState.isDataSyncOpen ||
        appState.isRefreshAllOpen ||
        appState.isImpactPreviewOpen ||
        appState.isAuditDrawerOpen ||
        appState.ddlReview.isOpen ||
        appState.fkPeekState.isOpen;

      if (!anyOverlayOpen && appState.activeTab?.showQueryProfiler) {
        appState.activeTab.showQueryProfiler = false;
        return;
      }

      if (!anyOverlayOpen && (appState.activeTab?.isExecuting || appState.activeTab?.isExplaining || appState.activeTab?.isStreaming)) {
        appState.cancelCurrentTab();
        return;
      }

      if (appState.isCommandPaletteOpen) appState.isCommandPaletteOpen = false;
      if (appState.isSchemaSearchOpen) appState.isSchemaSearchOpen = false;
      if (appState.isSmartSeedModalOpen) appState.isSmartSeedModalOpen = false;
      if (appState.isLockInspectorOpen) appState.isLockInspectorOpen = false;
      if (appState.ddlViewModal.isOpen) appState.ddlViewModal.isOpen = false;
      if (appState.safetyInspectionModal.isOpen) appState.safetyInspectionModal.isOpen = false;
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
      if (appState.isSnippetsModalOpen) {
        appState.pendingSnippet = null;
        appState.isSnippetsModalOpen = false;
      }
      if (appState.isCsvImportModalOpen) appState.isCsvImportModalOpen = false;
      if (appState.isPrivilegeMatrixOpen) appState.isPrivilegeMatrixOpen = false;
      if (appState.isDbDiagnosticsOpen) appState.isDbDiagnosticsOpen = false;
      if (appState.isAiDrawerOpen) appState.toggleAiDrawer(false);
      if (appState.isAiSettingsModalOpen) appState.isAiSettingsModalOpen = false;
      if (appState.isNotesDrawerOpen) appState.isNotesDrawerOpen = false;
      if (appState.isNoteComposerModalOpen) appState.isNoteComposerModalOpen = false;
      if (appState.isActivityDrawerOpen) appState.isActivityDrawerOpen = false;
      if (appState.isUpdateModalOpen) appState.isUpdateModalOpen = false;
      if (appState.isMockDataModalOpen) appState.closeMockDataModal();
      if (appState.isSettingsModalOpen) appState.isSettingsModalOpen = false;
      if (appState.jsonCellModal.isOpen) appState.jsonCellModal.isOpen = false;
      if (appState.isDataTransferOpen) appState.isDataTransferOpen = false;
      if (appState.isDataSyncOpen) appState.isDataSyncOpen = false;
      if (appState.isRefreshAllOpen) appState.isRefreshAllOpen = false;
      if (appState.isImpactPreviewOpen) appState.isImpactPreviewOpen = false;
      if (appState.isAuditDrawerOpen) appState.isAuditDrawerOpen = false;
      if (appState.ddlReview.isOpen) appState.ddlReview.isOpen = false;
      if (appState.fkPeekState.isOpen) appState.fkPeekState.isOpen = false;
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
        {#if appState.activeTab?.isProcesslist || appState.activeTab?.isCluster || appState.activeTab?.isVariables || appState.activeTab?.isFlightRecorder}
          <!-- Processlist / DBA Telemetry Tab -->
          <div class="flex-1 overflow-hidden">
            <ProcesslistView />
          </div>
        {:else if appState.activeTab?.isErdCanvas}
          <!-- ERD Canvas Visual Schema Designer Tab -->
          <div class="flex-1 overflow-hidden">
            <ErdCanvas />
          </div>
        {:else if appState.activeTab?.isNote}
          <!-- Dedicated Markdown Notes Tab -->
          {@const noteTab = appState.activeTab}
          {@const mode = noteTab.noteMode || 'edit'}
          <div class="flex-1 flex flex-col overflow-hidden bg-[#090d16]">
            <div class="h-10 px-4 border-b border-slate-800 bg-[#0f172a] flex items-center justify-between shrink-0 text-xs">
              <div class="flex items-center space-x-2">
                <Icon name="file-text" class="w-4 h-4 text-amber-400" />
                <span class="font-bold text-slate-200 font-mono">{noteTab.title}</span>
                <div class="inline-flex rounded border border-slate-700 bg-[#090d16] p-0.5 ml-2">
                  <button
                    type="button"
                    onclick={() => (noteTab.noteMode = 'edit')}
                    class="px-2 py-0.5 rounded text-[11px] font-semibold transition {mode === 'edit' ? 'bg-amber-500 text-slate-950' : 'text-slate-400 hover:text-slate-200'}"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onclick={() => (noteTab.noteMode = 'preview')}
                    class="px-2 py-0.5 rounded text-[11px] font-semibold transition {mode === 'preview' ? 'bg-amber-500 text-slate-950' : 'text-slate-400 hover:text-slate-200'}"
                  >
                    Preview
                  </button>
                </div>
              </div>
              <div class="flex items-center space-x-1.5">
                <button
                  type="button"
                  onclick={() => {
                    copyText(noteTab.sql || '');
                    appState.showToast('Copied Markdown note to clipboard', 'info');
                  }}
                  class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 flex items-center space-x-1 transition"
                >
                  <Icon name="copy" class="w-3 h-3" />
                  <span>Copy MD</span>
                </button>
                <button
                  type="button"
                  onclick={() => {
                    const blob = new Blob([noteTab.sql || ''], { type: 'text/markdown;charset=utf-8' });
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = noteTab.title.endsWith('.md') ? noteTab.title : `${noteTab.title}.md`;
                    a.click();
                    URL.revokeObjectURL(url);
                  }}
                  class="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 flex items-center space-x-1 transition"
                >
                  <Icon name="download" class="w-3 h-3" />
                  <span>Export .md</span>
                </button>
                <button
                  type="button"
                  onclick={() => appState.saveQueryToNote()}
                  class="px-2.5 py-1 rounded bg-amber-600 hover:bg-amber-500 text-white font-semibold flex items-center space-x-1 transition"
                >
                  <Icon name="bookmark" class="w-3 h-3" />
                  <span>Save to Notes</span>
                </button>
              </div>
            </div>
            <div class="flex-1 overflow-y-auto p-5 select-text">
              {#if mode === 'preview'}
                <div class="max-w-3xl mx-auto bg-[#0b0f19] border border-slate-800 rounded-xl p-5">
                  <NoteMarkdownView
                    content={noteTab.sql || '_Empty Markdown note — switch to Edit to start writing._'}
                    onOpenQuery={(sql) => appState.addTab('Query.sql', sql)}
                  />
                </div>
              {:else}
                <textarea
                  value={noteTab.sql}
                  oninput={(e) => {
                    noteTab.sql = (e.currentTarget as HTMLTextAreaElement).value;
                    appState.saveTabsState();
                  }}
                  placeholder="# Operational Note&#10;&#10;Write Markdown runbooks, SQL snippets, or incident timelines..."
                  class="w-full h-full min-h-[360px] bg-[#0b0f19] border border-slate-800 focus:border-amber-500/60 rounded-xl p-4 font-mono text-xs text-slate-200 leading-relaxed outline-none resize-none"
                ></textarea>
              {/if}
            </div>
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
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <div
            id="workspace-pane-splitter"
            onmousedown={startDrag}
            ondblclick={resetSplitter}
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
  <SchemaSearchModal />
  <DdlViewModal />
  <SafetyInspectionModal />
  <LockInspectorModal />
  <SmartSeedModal />

  <div class:hidden={!appState.isAiDrawerOpen}>
    <AiDrawer />
  </div>
  {#if appState.isNotesDrawerOpen}
    <NotesDrawer />
  {/if}
  <ActivityConsoleDrawer />

  {#if appState.isDaemonStopped}
    <div class="fixed inset-0 z-[200] bg-slate-950/95 backdrop-blur-md flex items-center justify-center p-6 text-center select-none">
      <div class="max-w-md bg-[#0f172a] border border-slate-700 rounded-2xl p-6 shadow-2xl space-y-3">
        <div class="w-12 h-12 rounded-full bg-rose-950 border border-rose-800 text-rose-400 flex items-center justify-center mx-auto">
          <Icon name="square" class="w-5 h-5" />
        </div>
        <h2 class="text-base font-bold text-slate-100">SqlFox Daemon Stopped</h2>
        <p class="text-xs text-slate-400 leading-relaxed">
          The local SqlFox server process has shut down gracefully. It is safe to close this browser tab or restart the binary to reconnect.
        </p>
      </div>
    </div>
  {/if}

  <ToastContainer />
</div>
