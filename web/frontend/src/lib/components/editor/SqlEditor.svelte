<script lang="ts">
  import { onDestroy } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { codemirror } from '../../actions/codemirror';
  import { formatSQL } from '../../utils/sqlFormatter';
  import Icon from '../common/Icon.svelte';
  import SearchableSelect from '../common/SearchableSelect.svelte';
  import type { CodeMirrorController } from '../../actions/codemirror';
  import type { QuerySnippet } from '../../../types';
  import { parseSnippetVariables } from '../../utils/snippets';

  // ponytail: only the tab identity drives the editor shell. Binding the editor to
  // `currentTab.sql` re-rendered the whole component on every keystroke, which was the
  // main source of input lag; the CodeMirror document is the source of truth instead.
  let currentTab = $derived(appState.activeTab);
  let activeTabId = $derived(appState.activeTab?.id);

  let editor = $state<CodeMirrorController | null>(null);
  let saveDebounceTimer: ReturnType<typeof setTimeout> | undefined;

  // ponytail: re-measure only when the tab actually changes; typing must not trigger a
  // full component re-render (that caused visible input lag).
  $effect(() => {
    const tabId = currentTab?.id;
    if (!tabId) return;
    editor?.requestMeasure();
  });

  function handleEditorReady(controller: CodeMirrorController | null) {
    editor = controller;
    appState.setEditorInserter(controller ? controller.insertAtSelection : null);
    appState.setEditorTargetProvider(controller ? () => controller.getTarget().text : null);
  }

  function handleSqlChange(newSql: string) {
    if (!currentTab) return;
    currentTab.sql = newSql;
    if (saveDebounceTimer) clearTimeout(saveDebounceTimer);
    saveDebounceTimer = setTimeout(() => appState.saveTabsState(), 350);
  }

  function handleExecute() {
    if (editor) appState.executeCurrentTab(editor.getTarget().text);
    else appState.executeCurrentTab();
  }

  function handleExecuteAll() {
    appState.executeCurrentTab(editor?.getDocument());
  }

  function handleExplain() {
    if (editor) appState.explainCurrentTab(editor.getTarget().text);
    else appState.explainCurrentTab();
  }

  function handleFormat(target?: { text: string }) {
    const sql = target?.text ?? currentTab?.sql ?? '';
    if (!sql.trim()) return;
    const formatted = formatSQL(sql);
    if (target && editor) {
      editor.replaceTarget(formatted);
    } else if (currentTab) {
      currentTab.sql = formatted;
    }
    appState.showToast('Formatted SQL query', 'info');
  }

  function handleSnippet(snippet: QuerySnippet, insert: (sql: string) => void) {
    const variables = parseSnippetVariables(snippet.sql);
    if (variables.length === 0) {
      insert(snippet.sql);
      return;
    }
    appState.requestSnippetParameters(snippet, insert);
  }

  onDestroy(() => {
    if (saveDebounceTimer) clearTimeout(saveDebounceTimer);
  });

  function quoteIdentifier(name: string): string {
    const clean = name.replace(/^[`"\[]+|[`"\]]+$/g, '');
    const needsQuoting = !/^[a-zA-Z_][a-zA-Z0-9_$]*$/.test(clean);
    const driver = appState.activeConnection?.driver;
    if (driver === 'postgres' || driver === 'postgresql' || driver === 'sqlite') {
      return needsQuoting ? `"${clean.replace(/"/g, '""')}"` : clean;
    }
    return needsQuoting ? `\`${clean.replace(/`/g, '``')}\`` : clean;
  }

  function handleTabScopeChange(database: string) {
    if (!currentTab) return;
    currentTab.database = database;
    void appState.ensureEditorCatalog(database).catch(() => {});
    appState.saveTabsState();
  }

  async function handleStartTransaction() {
    await appState.executeTransaction('START TRANSACTION');
  }

  async function handleCommit() {
    await appState.executeTransaction('COMMIT');
  }

  async function handleRollback() {
    await appState.executeTransaction('ROLLBACK');
  }

  function handleSendToAi() {
    const sql = (editor?.getTarget().text || currentTab?.sql || '').trim();
    if (!sql) {
      appState.isAiDrawerOpen = true;
      return;
    }
    appState.sendToAi(
      `Review, explain, or optimize this SQL query:\n\`\`\`sql\n${sql}\n\`\`\``,
      'Query sent to AI Copilot'
    );
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] relative">
  <!-- Query Editor Toolbar -->
  <div class="h-9 border-b border-slate-800 bg-[#0b0f19] px-3 flex items-center justify-between gap-2 overflow-x-auto select-none shrink-0">
    <div class="flex items-center space-x-2 shrink-0">
      <!-- 1. Execution & Target Database Scope -->
      <button
        onclick={handleExecute}
        disabled={currentTab?.isExecuting || currentTab?.isStreaming || currentTab?.isExplaining}
        class="h-7 flex items-center space-x-1.5 px-3 rounded bg-sky-500 hover:bg-sky-400 disabled:opacity-50 text-slate-950 font-bold text-xs transition shadow-[0_0_12px_rgba(56,189,248,0.35)]"
        title="Execute selected SQL or statement at cursor (Ctrl+Enter)"
      >
        {#if currentTab?.isExecuting || currentTab?.isStreaming}
          <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
          <span>Running...</span>
        {:else}
          <Icon name="play" class="w-3.5 h-3.5 fill-current" />
          <span>Execute</span>
        {/if}
        <span class="text-[9px] text-slate-900/80 bg-sky-400 px-1 py-0.2 rounded font-mono ml-1">Ctrl+Enter</span>
      </button>

      <button
        onclick={handleExecuteAll}
        disabled={currentTab?.isExecuting || currentTab?.isStreaming || currentTab?.isExplaining}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-50 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Execute All Statements (Ctrl+Alt+Enter / F5)"
      >
        <Icon name="play" class="w-3.5 h-3.5 text-emerald-400" />
        <span>Execute All</span>
      </button>

      {#if currentTab?.isExecuting || currentTab?.isStreaming || currentTab?.isExplaining}
        <button
          onclick={() => appState.cancelCurrentTab()}
          class="h-7 px-2.5 rounded bg-rose-950/80 hover:bg-rose-900 border border-rose-800 text-rose-300 text-xs font-semibold flex items-center space-x-1 transition"
          title="Cancel running query (Esc)"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
          <span>Cancel (Esc)</span>
        </button>
      {/if}

      {#if appState.availableDatabases.length > 0}
        <div class="h-7 relative flex items-center px-2 rounded bg-slate-900 border border-slate-700/80 text-[11px] font-mono text-slate-300" title="Target Database Scope for this tab">
          <SearchableSelect
            options={appState.availableDatabases}
            value={currentTab?.database || appState.currentDatabase || ''}
            onchange={handleTabScopeChange}
            label="scope:"
            placeholder="Search databases..."
            panelWidth="w-64"
            triggerClass="text-sky-400 hover:text-sky-300"
          />
        </div>
      {/if}

      <div class="h-3.5 w-px bg-slate-800"></div>

      <!-- 2. Query Analysis -->
      <button
        onclick={handleExplain}
        disabled={currentTab?.isExecuting || currentTab?.isExplaining}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-50 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Execution Plan & Index Advisor (Ctrl+Shift+Enter)"
      >
        <Icon name={currentTab?.isExplaining ? 'refresh-cw' : 'activity'} class="w-3.5 h-3.5 text-sky-400 {currentTab?.isExplaining ? 'animate-spin' : ''}" />
        <span>{currentTab?.isExplaining ? 'Explaining...' : 'Explain'}</span>
      </button>

      <button
        onclick={() => appState.inspectCurrentTab(editor?.getTarget().text)}
        disabled={currentTab?.isExecuting || currentTab?.isExplaining}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-50 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Run Safety Guard AST Inspection before execution"
      >
        <Icon name="shield-check" class="w-3.5 h-3.5 text-emerald-400" />
        <span>Safety Check</span>
      </button>

      <button
        onclick={() => (appState.enableQueryProfiling = !appState.enableQueryProfiling)}
        class="h-7 px-2.5 rounded {appState.enableQueryProfiling ? 'bg-amber-950/70 text-amber-300 border-amber-600/70' : 'bg-[#1e293b] text-slate-300 hover:text-white border-slate-700/80 hover:border-slate-600'} border text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Attach SHOW PROFILE stage telemetry to queries (MySQL/MariaDB)"
      >
        <Icon name="zap" class="w-3.5 h-3.5 {appState.enableQueryProfiling ? 'text-amber-400' : 'text-slate-400'}" />
        <span>Profile: {appState.enableQueryProfiling ? 'ON' : 'OFF'}</span>
      </button>

      <button
        onclick={() => {
          if (currentTab) currentTab.showQueryProfiler = !currentTab.showQueryProfiler;
        }}
        class="h-7 px-2.5 rounded {currentTab?.showQueryProfiler ? 'bg-sky-950 text-sky-300 border-sky-600' : 'bg-[#1e293b] text-slate-300 hover:text-white'} border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Toggle Stage Profiler Drawer"
      >
        <Icon name="clock" class="w-3.5 h-3.5 text-sky-400" />
        <span>Profiler</span>
      </button>

      <div class="h-3.5 w-px bg-slate-800"></div>

      <!-- 3. Transaction Controls -->
      {#if !appState.isTransactionActive}
        <button
          onclick={handleStartTransaction}
          disabled={currentTab?.isExecuting}
          class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-[#334155] text-xs flex items-center space-x-1.5 transition shadow-sm"
          title="Start Database Transaction (START TRANSACTION / BEGIN)"
        >
          <Icon name="shield" class="w-3.5 h-3.5 text-amber-400" />
          <span>Begin Tx</span>
        </button>
      {:else}
        <div class="h-7 flex items-center space-x-1.5 px-2.5 rounded-full bg-amber-500/10 border border-amber-500/30 text-amber-300 text-[11px] font-mono animate-pulse">
          <span class="w-2 h-2 rounded-full bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.8)]"></span>
          <span class="font-bold">TX Active</span>
        </div>

        <button
          onclick={handleCommit}
          disabled={currentTab?.isExecuting}
          class="h-7 px-2.5 rounded bg-emerald-950/80 hover:bg-emerald-900 border border-emerald-700/60 text-emerald-300 text-xs font-semibold flex items-center space-x-1 transition shadow-sm"
          title="Commit In-Flight Transaction (COMMIT)"
        >
          <Icon name="check" class="w-3.5 h-3.5" />
          <span>Commit</span>
        </button>

        <button
          onclick={handleRollback}
          disabled={currentTab?.isExecuting}
          class="h-7 px-2.5 rounded bg-rose-950/80 hover:bg-rose-900 border border-rose-700/60 text-rose-300 text-xs font-semibold flex items-center space-x-1 transition shadow-sm"
          title="Rollback In-Flight Transaction (ROLLBACK)"
        >
          <Icon name="rotate-ccw" class="w-3.5 h-3.5" />
          <span>Rollback</span>
        </button>
      {/if}

      <div class="h-3.5 w-px bg-slate-800"></div>

      <!-- 4. Editor Utilities -->
      <button
        onclick={() => handleFormat(editor?.getTarget())}
        disabled={currentTab?.isExecuting || !currentTab?.sql}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Format & Beautify SQL (Ctrl+Shift+F)"
      >
        <Icon name="file-text" class="w-3.5 h-3.5 text-emerald-400" />
        <span>Format</span>
      </button>

      <button
        onclick={() => appState.openSnippetsModal()}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Query Snippets & Templates Library (Ctrl+Shift+S)"
      >
        <Icon name="code" class="w-3.5 h-3.5 text-sky-400" />
        <span>Snippets</span>
      </button>

      <button
        onclick={() => appState.saveQueryToNote(currentTab?.sql, currentTab?.title)}
        disabled={currentTab?.isExecuting || !currentTab?.sql}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Save Active Query as Operational Note"
      >
        <Icon name="bookmark" class="w-3.5 h-3.5 text-amber-400" />
        <span>Save Note</span>
      </button>

      <button
        onclick={() => handleSqlChange('')}
        disabled={!currentTab?.sql}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 hover:text-white border border-[#334155] text-xs flex items-center space-x-1 transition shadow-sm"
        title="Clear SQL Editor"
      >
        <Icon name="trash-2" class="w-3.5 h-3.5 text-slate-400" />
        <span>Clear</span>
      </button>
    </div>

    <!-- Right Side: Live Status & Send to Global AI Sidebar -->
    <div class="flex items-center space-x-2 shrink-0">
      {#if currentTab?.statusText}
        <span class="text-[11px] font-mono px-2 py-0.5 rounded bg-slate-900 border border-slate-800 {currentTab.isExecuting || currentTab.isStreaming ? 'text-sky-300' : 'text-slate-400'}">
          {currentTab.statusText}
        </span>
      {/if}
      <button
        onclick={handleSendToAi}
        class="h-7 px-2.5 rounded {appState.isAiDrawerOpen ? 'bg-purple-950/60 text-purple-300 border-purple-500/60' : 'bg-[#1e293b] text-slate-300 hover:text-white border-[#334155]'} border text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Send active query to AI Copilot sidebar"
      >
        <Icon name="sparkles" class="w-3.5 h-3.5 text-purple-400" />
        <span class="font-semibold">Send to AI</span>
      </button>
    </div>
  </div>

  <!-- CodeMirror 6 Editor Container -->
  {#if currentTab}
    <div
      class="flex-1 overflow-hidden"
      use:codemirror={{
        id: currentTab.id,
        doc: currentTab.sql,
        dialect: appState.activeConnection?.driver,
        activeDatabase: currentTab.database || appState.currentDatabase,
        catalogs: appState.catalogs,
        snippets: appState.snippets,
        loadCatalog: appState.ensureEditorCatalog.bind(appState),
        quoteIdentifier,
        onExecute: handleExecute,
        onExecuteAll: handleExecuteAll,
        onExplain: handleExplain,
        onFormat: handleFormat,
        onSnippet: handleSnippet,
        onChange: handleSqlChange,
        onReady: handleEditorReady,
      }}
    ></div>
  {/if}

</div>
