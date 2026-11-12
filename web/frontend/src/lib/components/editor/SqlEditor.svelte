<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { codemirror } from '../../actions/codemirror';
  import { api } from '../../api/client';
  import { formatSQL } from '../../utils/sqlFormatter';
  import Icon from '../common/Icon.svelte';
  import type { CodeMirrorController } from '../../actions/codemirror';
  import type { QuerySnippet } from '../../types';
  import { parseSnippetVariables } from '../../utils/snippets';

  // ponytail: only the tab identity drives the editor shell. Binding the editor to
  // `currentTab.sql` re-rendered the whole component on every keystroke, which was the
  // main source of input lag; the CodeMirror document is the source of truth instead.
  let currentTab = $derived(appState.activeTab);
  let activeTabId = $derived(appState.activeTab?.id);

  // AI Copilot state
  let showAiAssistant = $state(false);
  let aiPrompt = $state('');
  let aiResponse = $state('');
  let isAiLoading = $state(false);

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

  function quoteIdentifier(name: string): string {
    const clean = name.replace(/^[`"\[]+|[`"\]]+$/g, '');
    const needsQuoting = !/^[a-zA-Z_][a-zA-Z0-9_$]*$/.test(clean);
    if (appState.activeConnection?.driver === 'postgres' || appState.activeConnection?.driver === 'sqlite') {
      return needsQuoting ? `"${clean.replace(/"/g, '""')}"` : clean;
    }
    return needsQuoting ? `\`${clean.replace(/`/g, '``')}\`` : clean;
  }

  function handleTabScopeChange(event: Event) {
    if (!currentTab) return;
    const database = (event.target as HTMLSelectElement).value;
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

  async function handleAiSubmit(promptToRun?: string) {
    const text = promptToRun || aiPrompt;
    if (!text.trim()) return;
    isAiLoading = true;
    aiResponse = '';
    try {
      const res = await api.aiChat(
        text,
        currentTab?.sql,
        appState.activeConnection?.driver || 'mysql'
      );
      aiResponse = res.response || res.error || 'No response';
    } catch (err: any) {
      aiResponse = `Error: ${err.message}`;
    } finally {
      isAiLoading = false;
    }
  }

  function insertAiSql() {
    if (!aiResponse || !currentTab) return;
    // Extract SQL from markdown code block if present
    const match = aiResponse.match(/```(?:sql)?\n([\s\S]*?)```/);
    const sqlToInsert = match ? match[1].trim() : aiResponse.trim();
    currentTab.sql = sqlToInsert;
    showAiAssistant = false;
    appState.showToast('Inserted AI SQL into editor', 'success');
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] relative">
  <!-- Query Editor Toolbar -->
  <div class="h-9 border-b border-slate-800 bg-[#0b0f19] px-3 flex items-center justify-between select-none shrink-0">
    <div class="flex items-center space-x-2">
      <!-- Run Button -->
      <button
        onclick={handleExecute}
        disabled={currentTab?.isExecuting}
        class="flex items-center space-x-1.5 px-3 py-1 rounded bg-sky-500 hover:bg-sky-400 disabled:opacity-50 text-slate-950 font-bold text-xs transition shadow-[0_0_12px_rgba(56,189,248,0.35)]"
        title="Execute selected SQL or statement at cursor (Ctrl+Enter)"
      >
        {#if currentTab?.isExecuting}
          <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
          <span>Running...</span>
        {:else}
          <Icon name="play" class="w-3.5 h-3.5 fill-current" />
          <span>Execute</span>
        {/if}
        <span class="text-[9px] text-slate-900/80 bg-sky-400 px-1 py-0.2 rounded font-mono ml-1">Ctrl+Enter</span>
      </button>

      <!-- Run All Button -->
      <button
        onclick={handleExecuteAll}
        disabled={currentTab?.isExecuting}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-50 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Execute All Statements (Ctrl+Alt+Enter / F5)"
      >
        <Icon name="play" class="w-3.5 h-3.5 text-emerald-400" />
        <span>Run All</span>
      </button>

      <!-- Cancel Button (visible when query is running) -->
      {#if currentTab?.isExecuting}
        <button
          onclick={() => appState.cancelCurrentTab()}
          class="h-7 px-2.5 rounded bg-rose-950/80 hover:bg-rose-900 border border-rose-800 text-rose-300 text-xs font-semibold flex items-center space-x-1 transition"
          title="Cancel running query"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
          <span>Cancel</span>
        </button>
      {/if}

      <!-- Explain Plan Button -->
      <button
        onclick={handleExplain}
        disabled={currentTab?.isExecuting}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Generate Query Execution Plan (EXPLAIN)"
      >
        <Icon name="activity" class="w-3.5 h-3.5 text-sky-400" />
        <span>Explain</span>
      </button>

      <!-- Pre-Flight Impact Tree Button -->
      <button
        onclick={() => appState.openImpactPreview()}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="View Pre-Flight Impact Preview & Execution Tree"
      >
        <Icon name="search" class="w-3.5 h-3.5 text-sky-400" />
        <span>Impact Tree</span>
      </button>

      <!-- Profiler Toggle Button -->
      <button
        onclick={() => {
          if (currentTab) currentTab.showQueryProfiler = !currentTab.showQueryProfiler;
        }}
        class="h-7 px-2.5 rounded {currentTab?.showQueryProfiler ? 'bg-sky-950 text-sky-300 border-sky-600' : 'bg-[#1e293b] text-slate-300 hover:text-white'} border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Toggle Stage Profiler (SHOW PROFILE)"
      >
        <Icon name="clock" class="w-3.5 h-3.5 text-sky-400" />
        <span>Profiler</span>
      </button>

      <!-- Format SQL Button -->
      <button
        onclick={() => handleFormat(editor?.getTarget())}
        disabled={currentTab?.isExecuting || !currentTab?.sql}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Format & Beautify SQL (Ctrl+Shift+F)"
      >
        <Icon name="file-text" class="w-3.5 h-3.5 text-emerald-400" />
        <span>Format</span>
        <span class="text-[9px] text-slate-500 font-mono hidden sm:inline">Ctrl+Shift+F</span>
      </button>

      <!-- Save as Operational Note Button -->
      <button
        onclick={() => appState.saveQueryToNote(currentTab?.sql, currentTab?.title)}
        disabled={currentTab?.isExecuting || !currentTab?.sql}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 disabled:opacity-40 text-slate-300 hover:text-white border border-slate-700/80 hover:border-slate-600 text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Save Active Query as Operational Note"
      >
        <Icon name="bookmark" class="w-3.5 h-3.5 text-amber-400" />
        <span>Save Note</span>
      </button>

      <div class="h-3.5 w-px bg-slate-800"></div>

      <!-- Tab Database Scope Selector Pill (ADR 0055) -->
      {#if appState.availableDatabases.length > 0}
        <div class="flex items-center space-x-1 px-2 py-0.5 rounded bg-slate-900 border border-slate-700/80 text-[11px] font-mono text-slate-300" title="Tab-Specific Database Scope (ADR 0055)">
          <span class="text-slate-500">scope:</span>
          <select
            value={currentTab?.database || appState.currentDatabase}
            onchange={handleTabScopeChange}
            class="bg-transparent text-sky-400 font-bold focus:outline-none cursor-pointer text-xs"
          >
            {#each appState.availableDatabases as db}
              <option value={db} class="bg-[#0f172a] text-slate-200">{db}</option>
            {/each}
          </select>
        </div>
      {/if}

      <div class="h-3.5 w-px bg-slate-800"></div>

      <!-- Clear Editor Button -->
      <button
        onclick={() => handleSqlChange('')}
        class="h-7 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white border border-[#334155] text-xs flex items-center space-x-1 transition shadow-sm"
        title="Clear SQL Editor"
      >
        <Icon name="trash-2" class="w-3.5 h-3.5 text-slate-400" />
        <span>Clear</span>
      </button>

      <div class="h-3.5 w-px bg-slate-800"></div>

      <!-- Transaction Controls -->
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
        <!-- In-flight Transaction Pill -->
        <div class="flex items-center space-x-1.5 px-2.5 py-1 rounded-full bg-amber-500/10 border border-amber-500/30 text-amber-300 text-[11px] font-mono animate-pulse">
          <span class="w-2 h-2 rounded-full bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.8)]"></span>
          <span class="font-bold">TX Active</span>
        </div>

        <!-- Commit Button -->
        <button
          onclick={handleCommit}
          disabled={currentTab?.isExecuting}
          class="h-7 px-2.5 rounded bg-emerald-950/80 hover:bg-emerald-900 border border-emerald-700/60 text-emerald-300 text-xs font-semibold flex items-center space-x-1 transition shadow-sm"
          title="Commit In-Flight Transaction (COMMIT)"
        >
          <Icon name="check" class="w-3.5 h-3.5" />
          <span>Commit</span>
        </button>

        <!-- Rollback Button -->
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
    </div>

    <!-- Right Side: Dialect Indicator & AI Assistant -->
    <div class="flex items-center space-x-2 text-xs text-slate-400">
      <!-- AI Assistant Button -->
      <button
        onclick={() => (showAiAssistant = !showAiAssistant)}
        class="h-7 px-2.5 rounded {showAiAssistant ? 'bg-purple-950/60 text-purple-300 border-purple-500/60' : 'bg-[#1e293b] text-slate-300 hover:text-white'} border border-[#334155] text-xs flex items-center space-x-1.5 transition shadow-sm"
        title="Open AI SQL Copilot"
      >
        <Icon name="sparkles" class="w-3.5 h-3.5 text-purple-400" />
        <span class="font-semibold">AI Copilot</span>
      </button>

      <div class="flex items-center space-x-1.5 px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-[11px] font-mono text-slate-300">
        <Icon name="database" class="w-3 h-3 text-sky-400" />
        <span>{appState.activeConnection?.driver?.toUpperCase() || 'MYSQL'}</span>
      </div>
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

  <!-- Explain Plan Drawer (if active) -->
  {#if currentTab?.explainResult}
    <div class="h-44 bg-[#0b0f19] border-t border-[#334155] flex flex-col p-3 z-20 shrink-0 overflow-hidden font-mono text-xs">
      <div class="flex items-center justify-between pb-1 select-none">
        <div class="flex items-center space-x-2 font-bold text-slate-200">
          <Icon name="activity" class="w-4 h-4 text-sky-400" />
          <span>Query Execution Plan (EXPLAIN)</span>
        </div>
        <button
          onclick={() => { if (currentTab) currentTab.explainResult = null; }}
          class="text-slate-400 hover:text-white p-0.5"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>
      <div class="flex-1 bg-[#090d16] border border-[#1e293b] rounded p-2 overflow-auto text-sky-300">
        <pre class="whitespace-pre-wrap">{typeof currentTab.explainResult === 'object' ? JSON.stringify(currentTab.explainResult, null, 2) : currentTab.explainResult}</pre>
      </div>
    </div>
  {/if}

  <!-- AI Copilot Side Drawer -->
  {#if showAiAssistant}
    <div class="absolute right-0 top-9 bottom-0 w-80 bg-[#0f172a] border-l border-[#334155] shadow-2xl z-30 flex flex-col p-3 text-xs select-none animate-in slide-in-from-right-4 duration-150">
      <div class="flex items-center justify-between pb-2 border-b border-[#1e293b]">
        <div class="flex items-center space-x-2 font-bold text-purple-300">
          <Icon name="sparkles" class="w-4 h-4 text-purple-400" />
          <span>AI SQL Copilot</span>
        </div>
        <button
          onclick={() => (showAiAssistant = false)}
          class="text-slate-400 hover:text-white p-0.5"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>

      <!-- Quick prompts -->
      <div class="grid grid-cols-2 gap-1.5 py-2.5">
        <button
          onclick={() => handleAiSubmit('Explain this SQL query and what it does')}
          class="px-2 py-1 bg-[#1e293b] hover:bg-slate-700 rounded text-[11px] text-slate-300 text-left border border-slate-750"
        >
          Explain Query
        </button>
        <button
          onclick={() => handleAiSubmit('Optimize this query for performance and indexing')}
          class="px-2 py-1 bg-[#1e293b] hover:bg-slate-700 rounded text-[11px] text-slate-300 text-left border border-slate-750"
        >
          Optimize SQL
        </button>
      </div>

      <!-- Chat input -->
      <div class="space-y-1.5 pb-2">
        <textarea
          rows="2"
          bind:value={aiPrompt}
          placeholder="Ask AI to write or fix SQL..."
          class="w-full bg-[#090d16] border border-[#334155] rounded p-2 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-purple-500 resize-none font-sans"
        ></textarea>
        <button
          onclick={() => handleAiSubmit()}
          disabled={isAiLoading || !aiPrompt.trim()}
          class="w-full py-1.5 bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white font-bold rounded flex items-center justify-center space-x-1.5 transition"
        >
          {#if isAiLoading}
            <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
            <span>Thinking...</span>
          {:else}
            <Icon name="sparkles" class="w-3.5 h-3.5" />
            <span>Generate / Explain</span>
          {/if}
        </button>
      </div>

      <!-- Response area -->
      <div class="flex-1 bg-[#090d16] border border-[#1e293b] rounded p-2 overflow-y-auto select-text font-mono text-[11px] text-slate-300 leading-relaxed space-y-2">
        {#if aiResponse}
          <div class="whitespace-pre-wrap">{aiResponse}</div>
          <button
            onclick={insertAiSql}
            class="w-full py-1 bg-[#1e293b] hover:bg-purple-950/60 hover:text-purple-300 text-slate-300 border border-[#334155] rounded font-sans font-semibold transition"
          >
            Insert SQL into Editor
          </button>
        {:else}
          <div class="p-6 text-center text-slate-600 italic font-sans text-xs">
            Ask AI to explain, generate, or optimize queries.
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
