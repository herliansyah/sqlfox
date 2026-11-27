<script lang="ts">
  import Header from './lib/components/layout/Header.svelte';
  import Sidebar from './lib/components/layout/Sidebar.svelte';
  import Footer from './lib/components/layout/Footer.svelte';
  import TabBar from './lib/components/editor/TabBar.svelte';
  import SqlEditor from './lib/components/editor/SqlEditor.svelte';
  import VirtualGrid from './lib/components/grid/VirtualGrid.svelte';
  import TableViewer from './lib/components/table/TableViewer.svelte';
  import RemoteLockModal from './lib/components/modals/RemoteLockModal.svelte';
  import ToastContainer from './lib/components/common/ToastContainer.svelte';
  import NativeDialogModal from './lib/components/modals/NativeDialogModal.svelte';
  import ReconnectBanner from './lib/components/common/ReconnectBanner.svelte';
  import Icon from './lib/components/common/Icon.svelte';
  import { copyText } from './lib/utils/clipboard';
  import { appState } from './lib/stores/appState.svelte';
  import { onDestroy, tick } from 'svelte';
  import { formatSQL } from './lib/utils/sqlFormatter';

  function safeReadSplit(): number {
    try {
      return Number(localStorage.getItem('sqlfox_svelte_editor_split')) || 45;
    } catch {
      return 45;
    }
  }

  function safeWriteSplit(val: number) {
    try {
      localStorage.setItem('sqlfox_svelte_editor_split', String(val));
    } catch { /* ignore */ }
  }

  let editorSplitPercent = $state(Math.max(15, Math.min(85, safeReadSplit())));
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
    safeWriteSplit(editorSplitPercent);
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
    safeWriteSplit(50);
  }

  onDestroy(() => {
    stopDrag();
    appState.destroy();
  });

  function handleGlobalKeyDown(e: KeyboardEvent) {
    if (appState.isRemoteLocked) return;
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

    const modalOpen = appState.hasOpenModal;

    // Never fire global query/transaction shortcuts while a modal/dialog is active in front of the editor.
    if (!modalOpen) {
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
          appState.patchTab(appState.activeTab.id, { sql: formatSQL(appState.activeTab.sql) });
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

    // Ctrl + F outside CodeMirror: Focus grid filter bar
    if (
      isCtrl &&
      !e.shiftKey &&
      e.key.toLowerCase() === 'f' &&
      !inCodeMirror &&
      (appState.activeTab?.isTableViewer || (appState.activeTab?.activeResult?.columns?.length ?? 0) > 0)
    ) {
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
        appState.resolveNativeDialog(appState.nativeDialog.type === 'prompt' ? null : false);
        return;
      }
      if (appState.safetyChallenge.isOpen) {
        appState.cancelSafetyChallenge();
        return;
      }

      if (appState.closeTopOverlay()) {
        return;
      }

      if (appState.activeTab?.showQueryProfiler) {
        appState.patchTab(appState.activeTab.id, { showQueryProfiler: false });
        return;
      }

      if (appState.activeTab?.isExecuting || appState.activeTab?.isExplaining || appState.activeTab?.isStreaming) {
        appState.cancelCurrentTab();
        return;
      }
    }
  }
</script>

<svelte:window onkeydown={handleGlobalKeyDown} />

<div
  class="h-screen w-screen flex flex-col bg-[#090d16] text-slate-100 overflow-hidden font-sans select-none"
  class:hidden={appState.isRemoteLocked}
>
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
            {#await import('./lib/components/dba/ProcesslistView.svelte') then { default: ProcesslistView }}
              <ProcesslistView />
            {/await}
          </div>
        {:else if appState.activeTab?.isErdCanvas}
          <!-- ERD Canvas Visual Schema Designer Tab -->
          <div class="flex-1 overflow-hidden">
            {#await import('./lib/components/erd/ErdCanvas.svelte') then { default: ErdCanvas }}
              <ErdCanvas />
            {/await}
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
                    onclick={() => appState.patchTab(noteTab.id, { noteMode: 'edit' })}
                    class="px-2 py-0.5 rounded text-[11px] font-semibold transition {mode === 'edit' ? 'bg-amber-500 text-slate-950' : 'text-slate-400 hover:text-slate-200'}"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onclick={() => appState.patchTab(noteTab.id, { noteMode: 'preview' })}
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
                  {#await import('./lib/components/notes/NoteMarkdownView.svelte') then { default: NoteMarkdownView }}
                    <NoteMarkdownView
                      content={noteTab.sql || '_Empty Markdown note — switch to Edit to start writing._'}
                      onOpenQuery={(sql) => appState.addTab('Query.sql', sql)}
                    />
                  {/await}
                </div>
              {:else}
                <textarea
                  value={noteTab.sql}
                  oninput={(e) => {
                    appState.patchTab(noteTab.id, { sql: (e.currentTarget as HTMLTextAreaElement).value });
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

  <!-- Global Modals (lazy-loaded on demand to keep initial bundle lean) -->
  {#if appState.safetyChallenge.isOpen}
    {#await import('./lib/components/modals/SafetyModal.svelte') then { default: SafetyModal }}<SafetyModal />{/await}
  {/if}
  {#if appState.isConnectionModalOpen}
    {#await import('./lib/components/modals/ConnectionModal.svelte') then { default: ConnectionModal }}<ConnectionModal />{/await}
  {/if}
  {#if appState.isRecordInspectorOpen}
    {#await import('./lib/components/modals/RecordInspectorModal.svelte') then { default: RecordInspectorModal }}<RecordInspectorModal />{/await}
  {/if}
  {#if appState.isInsertRowModalOpen}
    {#await import('./lib/components/modals/InsertRowModal.svelte') then { default: InsertRowModal }}<InsertRowModal />{/await}
  {/if}
  {#if appState.isCommandPaletteOpen}
    {#await import('./lib/components/modals/CommandPaletteModal.svelte') then { default: CommandPaletteModal }}<CommandPaletteModal />{/await}
  {/if}
  {#if appState.isShortcutsModalOpen}
    {#await import('./lib/components/modals/ShortcutsModal.svelte') then { default: ShortcutsModal }}<ShortcutsModal />{/await}
  {/if}
  {#if appState.isCreateDbModalOpen}
    {#await import('./lib/components/modals/CreateDatabaseModal.svelte') then { default: CreateDatabaseModal }}<CreateDatabaseModal />{/await}
  {/if}
  {#if appState.isTableCreatorOpen}
    {#await import('./lib/components/modals/TableCreatorModal.svelte') then { default: TableCreatorModal }}<TableCreatorModal />{/await}
  {/if}
  {#if appState.isRenameTableModalOpen}
    {#await import('./lib/components/modals/RenameTableModal.svelte') then { default: RenameTableModal }}<RenameTableModal />{/await}
  {/if}
  {#if appState.isDuplicateTableModalOpen}
    {#await import('./lib/components/modals/DuplicateTableModal.svelte') then { default: DuplicateTableModal }}<DuplicateTableModal />{/await}
  {/if}
  {#if appState.isViewDesignerOpen}
    {#await import('./lib/components/modals/ViewDesignerModal.svelte') then { default: ViewDesignerModal }}<ViewDesignerModal />{/await}
  {/if}
  {#if appState.isTriggerDesignerOpen}
    {#await import('./lib/components/modals/TriggerDesignerModal.svelte') then { default: TriggerDesignerModal }}<TriggerDesignerModal />{/await}
  {/if}
  {#if appState.isRoutineDesignerOpen}
    {#await import('./lib/components/modals/RoutineDesignerModal.svelte') then { default: RoutineDesignerModal }}<RoutineDesignerModal />{/await}
  {/if}
  {#if appState.isEventDesignerOpen}
    {#await import('./lib/components/modals/EventDesignerModal.svelte') then { default: EventDesignerModal }}<EventDesignerModal />{/await}
  {/if}
  {#if appState.isDumpModalOpen}
    {#await import('./lib/components/modals/DumpModal.svelte') then { default: DumpModal }}
      <DumpModal />
    {/await}
  {/if}
  {#if appState.isStructureSyncModalOpen}
    {#await import('./lib/components/modals/StructureSyncModal.svelte') then { default: StructureSyncModal }}
      <StructureSyncModal />
    {/await}
  {/if}
  {#if appState.isRunSqlModalOpen}
    {#await import('./lib/components/modals/RunSqlModal.svelte') then { default: RunSqlModal }}<RunSqlModal />{/await}
  {/if}
  {#if appState.isSnippetsModalOpen}
    {#await import('./lib/components/modals/SnippetsModal.svelte') then { default: SnippetsModal }}<SnippetsModal />{/await}
  {/if}
  {#if appState.isCsvImportModalOpen}
    {#await import('./lib/components/modals/CsvImportModal.svelte') then { default: CsvImportModal }}
      <CsvImportModal />
    {/await}
  {/if}
  {#if appState.isPrivilegeMatrixOpen}
    {#await import('./lib/components/modals/PrivilegeMatrixModal.svelte') then { default: PrivilegeMatrixModal }}
      <PrivilegeMatrixModal />
    {/await}
  {/if}
  {#if appState.isDbDiagnosticsOpen}
    {#await import('./lib/components/modals/DbDiagnosticsModal.svelte') then { default: DbDiagnosticsModal }}
      <DbDiagnosticsModal />
    {/await}
  {/if}
  {#if appState.jsonCellModal.isOpen}
    {#await import('./lib/components/modals/JsonCellModal.svelte') then { default: JsonCellModal }}<JsonCellModal />{/await}
  {/if}
  {#if appState.isAiSettingsModalOpen}
    {#await import('./lib/components/modals/AiSettingsModal.svelte') then { default: AiSettingsModal }}<AiSettingsModal />{/await}
  {/if}
  {#if appState.isNoteComposerModalOpen}
    {#await import('./lib/components/modals/NoteComposerModal.svelte') then { default: NoteComposerModal }}<NoteComposerModal />{/await}
  {/if}
  {#if appState.isUpdateModalOpen}
    {#await import('./lib/components/modals/UpdateModal.svelte') then { default: UpdateModal }}<UpdateModal />{/await}
  {/if}
  {#if appState.isMockDataModalOpen}
    {#await import('./lib/components/modals/MockDataModal.svelte') then { default: MockDataModal }}
      <MockDataModal />
    {/await}
  {/if}
  {#if appState.isSettingsModalOpen}
    {#await import('./lib/components/modals/SettingsModal.svelte') then { default: SettingsModal }}<SettingsModal />{/await}
  {/if}
  <NativeDialogModal />
  <ReconnectBanner />
  {#if appState.ddlReview.isOpen}
    {#await import('./lib/components/modals/DdlReviewModal.svelte') then { default: DdlReviewModal }}<DdlReviewModal />{/await}
  {/if}
  {#if appState.isRefreshAllOpen}
    {#await import('./lib/components/modals/RefreshAllModal.svelte') then { default: RefreshAllModal }}<RefreshAllModal />{/await}
  {/if}
  {#if appState.isImpactPreviewOpen}
    {#await import('./lib/components/modals/ImpactPreviewModal.svelte') then { default: ImpactPreviewModal }}<ImpactPreviewModal />{/await}
  {/if}
  {#if appState.isDataTransferOpen}
    {#await import('./lib/components/modals/DataTransferModal.svelte') then { default: DataTransferModal }}
      <DataTransferModal />
    {/await}
  {/if}
  {#if appState.isDataSyncOpen}
    {#await import('./lib/components/modals/DataSyncModal.svelte') then { default: DataSyncModal }}
      <DataSyncModal />
    {/await}
  {/if}
  {#if appState.isAuditDrawerOpen}
    {#await import('./lib/components/activity/AuditDrawer.svelte') then { default: AuditDrawer }}
      <AuditDrawer />
    {/await}
  {/if}
  {#if appState.fkPeekState.isOpen}
    {#await import('./lib/components/grid/FkPeekPopover.svelte') then { default: FkPeekPopover }}<FkPeekPopover />{/await}
  {/if}
  {#if appState.isSchemaSearchOpen}
    {#await import('./lib/components/modals/SchemaSearchModal.svelte') then { default: SchemaSearchModal }}<SchemaSearchModal />{/await}
  {/if}
  {#if appState.ddlViewModal.isOpen}
    {#await import('./lib/components/modals/DdlViewModal.svelte') then { default: DdlViewModal }}<DdlViewModal />{/await}
  {/if}
  {#if appState.safetyInspectionModal.isOpen}
    {#await import('./lib/components/modals/SafetyInspectionModal.svelte') then { default: SafetyInspectionModal }}<SafetyInspectionModal />{/await}
  {/if}
  {#if appState.isLockInspectorOpen}
    {#await import('./lib/components/modals/LockInspectorModal.svelte') then { default: LockInspectorModal }}<LockInspectorModal />{/await}
  {/if}
  {#if appState.isSmartSeedModalOpen}
    {#await import('./lib/components/modals/SmartSeedModal.svelte') then { default: SmartSeedModal }}
      <SmartSeedModal />
    {/await}
  {/if}

  {#if appState.isAiDrawerOpen}
    {#await import('./lib/components/ai/AiDrawer.svelte') then { default: AiDrawer }}<AiDrawer />{/await}
  {/if}
  {#if appState.isNotesDrawerOpen}
    {#await import('./lib/components/notes/NotesDrawer.svelte') then { default: NotesDrawer }}
      <NotesDrawer />
    {/await}
  {/if}
  {#if appState.isActivityDrawerOpen}
    {#await import('./lib/components/activity/ActivityConsoleDrawer.svelte') then { default: ActivityConsoleDrawer }}<ActivityConsoleDrawer />{/await}
  {/if}

  {#if appState.isDaemonStopped}
    <div class="fixed inset-0 z-[200] bg-slate-950/95 flex items-center justify-center p-6 text-center select-none">
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

<RemoteLockModal />
