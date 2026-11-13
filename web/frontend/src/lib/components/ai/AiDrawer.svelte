<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface ChatMessage {
    id: string;
    role: 'user' | 'assistant';
    content: string;
    timestamp: Date;
  }

  let messages = $state<ChatMessage[]>([
    {
      id: 'init',
      role: 'assistant',
      content: 'Hello! I am your SqlFox AI Copilot. I can help explain queries, fix SQL syntax errors, suggest optimizations, or generate new queries from plain language. How can I help you today?',
      timestamp: new Date(),
    },
  ]);

  let promptInput = $state('');
  let isLoading = $state(false);
  let mentionQuery = $state<string | null>(null);
  let mentionIndex = $state(0);
  let messagesContainer = $state<HTMLDivElement | null>(null);
  let promptEl = $state<HTMLTextAreaElement | null>(null);
  let isMaximized = $state(localStorage.getItem('sqlfox_ai_drawer_maximized') === 'true');

  function toggleMaximized() {
    isMaximized = !isMaximized;
    localStorage.setItem('sqlfox_ai_drawer_maximized', String(isMaximized));
  }

  // Auto-scroll messages to bottom
  $effect(() => {
    if (messages.length && messagesContainer) {
      setTimeout(() => {
        messagesContainer?.scrollTo({ top: messagesContainer.scrollHeight, behavior: 'smooth' });
      }, 50);
    }
  });

  // Autocomplete matching tables for @mention
  let matchingTables = $derived.by(() => {
    if (mentionQuery === null || !appState.catalog?.tables) return [];
    const q = mentionQuery.toLowerCase();
    return appState.catalog.tables
      .filter((t) => t.name.toLowerCase().includes(q))
      .slice(0, 5);
  });

  function handleInput(e: Event) {
    const target = e.target as HTMLTextAreaElement;
    const val = target.value;
    const cursorPos = target.selectionStart;
    const textBeforeCursor = val.slice(0, cursorPos);
    const match = textBeforeCursor.match(/@([a-zA-Z0-9_-]*)$/);

    if (match) {
      mentionQuery = match[1] ?? '';
      mentionIndex = 0;
    } else {
      mentionQuery = null;
    }
  }

  function insertMention(tableName: string) {
    const match = promptInput.match(/@([a-zA-Z0-9_-]*)$/);
    if (match) {
      const idx = promptInput.lastIndexOf('@');
      promptInput = promptInput.slice(0, idx) + `\`${tableName}\` `;
    } else {
      promptInput += `\`${tableName}\` `;
    }
    mentionQuery = null;
  }

  async function sendMessage(overridePrompt?: string) {
    const text = (overridePrompt ?? promptInput).trim();
    if (!text || isLoading) return;

    promptInput = '';
    mentionQuery = null;

    const userMsg: ChatMessage = {
      id: String(Date.now()),
      role: 'user',
      content: text,
      timestamp: new Date(),
    };
    messages = [...messages, userMsg];
    isLoading = true;

    // Gather active workspace context
    const currentTab = appState.activeTab;
    const activeQuery = currentTab?.sql || '';
    const activeTable = currentTab?.tableName || '';
    const dialect = appState.activeConnection?.driver || 'mysql';

    let schemaContext = `Database Dialect: ${dialect}\nCurrent Database: ${appState.currentDatabase || 'default'}\n`;
    if (activeTable && appState.catalog?.tables) {
      const t = appState.catalog.tables.find((tbl) => tbl.name.toLowerCase() === activeTable.toLowerCase());
      if (t) {
        schemaContext += `Active Table: ${t.name}\nColumns: ${t.columns.map((c) => `${c.name} (${c.type})`).join(', ')}\n`;
      }
    }
    if (activeQuery) {
      schemaContext += `Active Query:\n${activeQuery}\n`;
    }

    try {
      const res = await api.aiChat(text, schemaContext, dialect);
      const assistantMsg: ChatMessage = {
        id: String(Date.now() + 1),
        role: 'assistant',
        content: res.content || 'No response generated.',
        timestamp: new Date(),
      };
      messages = [...messages, assistantMsg];
    } catch (err: any) {
      const errMsg: ChatMessage = {
        id: String(Date.now() + 1),
        role: 'assistant',
        content: `Sorry, an error occurred while connecting to AI: ${err.message}`,
        timestamp: new Date(),
      };
      messages = [...messages, errMsg];
    } finally {
      isLoading = false;
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (mentionQuery !== null && matchingTables.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        mentionIndex = (mentionIndex + 1) % matchingTables.length;
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        mentionIndex = (mentionIndex - 1 + matchingTables.length) % matchingTables.length;
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        const selected = matchingTables[mentionIndex];
        if (selected) insertMention(selected.name);
        return;
      }
      if (e.key === 'Escape') {
        mentionQuery = null;
        return;
      }
    }

    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey || !e.shiftKey)) {
      e.preventDefault();
      sendMessage();
    }
  }

  function insertIntoWorksheet(sql: string) {
    if (appState.activeTab) {
      appState.activeTab.sql = sql;
      appState.showToast('SQL inserted into active worksheet!', 'success');
    } else {
      appState.showToast('Open a worksheet tab first', 'warning');
    }
  }

  function clearChat() {
    messages = [
      {
        id: 'init',
        role: 'assistant',
        content: 'Conversation history cleared. How can I assist you with your queries?',
        timestamp: new Date(),
      },
    ];
  }

  function formatMessageText(content: string): Array<{ type: 'text' | 'sql'; text: string }> {
    const parts: Array<{ type: 'text' | 'sql'; text: string }> = [];
    const regex = /```(?:sql)?\n([\s\S]*?)```/gi;
    let lastIndex = 0;
    let match;

    while ((match = regex.exec(content)) !== null) {
      if (match.index > lastIndex) {
        parts.push({ type: 'text', text: content.slice(lastIndex, match.index) });
      }
      const sqlCode = match[1];
      if (sqlCode !== undefined) {
        parts.push({ type: 'sql', text: sqlCode.trim() });
      }
      lastIndex = regex.lastIndex;
    }

    if (lastIndex < content.length) {
      parts.push({ type: 'text', text: content.slice(lastIndex) });
    }

    return parts.length > 0 ? parts : [{ type: 'text', text: content }];
  }

  // "Send to AI" from anywhere in the app lands here as a reviewable draft.
  $effect(() => {
    const draft = appState.aiPromptDraft;
    if (!draft) return;
    promptInput = appState.consumeAiPromptDraft();
    queueMicrotask(() => {
      const el = promptEl;
      if (!el) return;
      el.focus();
      el.selectionStart = el.selectionEnd = el.value.length;
    });
  });
</script>

<aside
  class="fixed top-12 right-0 bottom-7 bg-[#0b0f19] border-l border-[#1e293b] shadow-2xl z-40 flex flex-col font-sans transition-[width] duration-200 select-text"
  style="width: {isMaximized
    ? 'min(56rem, calc(100vw - 1.5rem))'
    : 'min(24rem, calc(100vw - 1.5rem))'};"
>
  <!-- Drawer Header -->
  <div class="h-10 bg-[#0f172a] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0 select-none">
    <div class="flex items-center space-x-2">
      <div class="p-1 rounded bg-sky-950 border border-sky-800 text-sky-400">
        <Icon name="bot" class="w-3.5 h-3.5" />
      </div>
      <span class="font-bold text-slate-100 text-xs">AI Copilot</span>
      <span class="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" title="Ready"></span>
    </div>

    <div class="flex items-center space-x-1">
      <button
        onclick={toggleMaximized}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title={isMaximized ? 'Restore Copilot Width' : 'Maximize Copilot'}
      >
        <Icon name={isMaximized ? 'minimize-2' : 'maximize-2'} class="w-3.5 h-3.5" />
      </button>
      <button
        onclick={() => (appState.isAiSettingsModalOpen = true)}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title="AI Settings / BYOK"
      >
        <Icon name="settings" class="w-3.5 h-3.5" />
      </button>
      <button
        onclick={clearChat}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title="Clear Conversation"
      >
        <Icon name="rotate-ccw" class="w-3.5 h-3.5" />
      </button>
      <button
        onclick={() => (appState.isAiDrawerOpen = false)}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title="Close Drawer"
      >
        <Icon name="x" class="w-3.5 h-3.5" />
      </button>
    </div>
  </div>

  <!-- Quick Action Chips -->
  <div class="p-2 border-b border-[#1e293b] bg-[#070b14] flex flex-wrap gap-1 flex-shrink-0 select-none">
    <button
      onclick={() => sendMessage('Explain the current query and its potential performance bottlenecks.')}
      disabled={isLoading}
      class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 hover:border-sky-500 text-slate-400 hover:text-sky-300 text-[10px] transition flex items-center space-x-1"
    >
      <Icon name="help-circle" class="w-3 h-3 text-sky-400" />
      <span>Explain Query</span>
    </button>
    <button
      onclick={() => sendMessage('Analyze and fix any syntax or structural errors in this query.')}
      disabled={isLoading}
      class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 hover:border-amber-500 text-slate-400 hover:text-amber-300 text-[10px] transition flex items-center space-x-1"
    >
      <Icon name="check-circle" class="w-3 h-3 text-amber-400" />
      <span>Fix Syntax</span>
    </button>
    <button
      onclick={() => sendMessage('Suggest indexes and rewrite this query for optimal performance.')}
      disabled={isLoading}
      class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 hover:border-emerald-500 text-slate-400 hover:text-emerald-300 text-[10px] transition flex items-center space-x-1"
    >
      <Icon name="zap" class="w-3 h-3 text-emerald-400" />
      <span>Optimize</span>
    </button>
  </div>

  <!-- Messages List -->
  <div bind:this={messagesContainer} class="flex-1 overflow-y-auto p-3 space-y-3">
    {#each messages as msg}
      {@const isUser = msg.role === 'user'}
      <div class="flex flex-col {isUser ? 'items-end' : 'items-start'}">
        <div class="flex items-center space-x-1.5 mb-1 px-1">
          <span class="text-[10px] text-slate-500 font-mono">
            {isUser ? 'You' : 'SqlFox AI'} • {msg.timestamp.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
          </span>
        </div>

        <div
          class="max-w-[92%] rounded-xl px-3 py-2 text-xs leading-relaxed {isUser
            ? 'bg-sky-600 text-white rounded-tr-none shadow-md'
            : 'bg-[#131b2e] border border-[#1e293b] text-slate-200 rounded-tl-none shadow-md'}"
        >
          {#if isUser}
            <div class="whitespace-pre-wrap">{msg.content}</div>
          {:else}
            {#each formatMessageText(msg.content) as part}
              {#if part.type === 'text'}
                <div class="whitespace-pre-wrap mb-2 last:mb-0">{part.text}</div>
              {:else if part.type === 'sql'}
                <div class="my-2 rounded-lg overflow-hidden border border-sky-800/60 bg-[#050811]">
                  <div class="bg-[#0b0f19] px-2.5 py-1 flex items-center justify-between border-b border-sky-950 text-[10px] font-mono text-slate-400 select-none">
                    <span>SQL</span>
                    <div class="flex items-center space-x-1.5">
                      <button
                        onclick={() => {
                          navigator.clipboard.writeText(part.text);
                          appState.showToast('SQL copied to clipboard!', 'info');
                        }}
                        class="hover:text-white"
                        title="Copy SQL"
                      >
                        <Icon name="copy" class="w-3 h-3" />
                      </button>
                      <button
                        onclick={() => insertIntoWorksheet(part.text)}
                        class="hover:text-sky-400 flex items-center space-x-0.5"
                        title="Insert into Worksheet"
                      >
                        <Icon name="corner-down-left" class="w-3 h-3" />
                        <span>Insert</span>
                      </button>
                    </div>
                  </div>
                  <pre class="p-2.5 text-sky-200 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap">{part.text}</pre>
                </div>
              {/if}
            {/each}
          {/if}
        </div>
      </div>
    {/each}

    {#if isLoading}
      <div class="flex items-center space-x-2 text-slate-500 text-xs p-2">
        <Icon name="loader" class="w-3.5 h-3.5 animate-spin text-sky-400" />
        <span>SqlFox AI is analyzing and generating response...</span>
      </div>
    {/if}
  </div>

  <!-- Mention Auto-Complete Popover -->
  {#if mentionQuery !== null && matchingTables.length > 0}
    <div class="mx-3 mb-1 bg-[#0f172a] border border-sky-500/50 rounded-xl shadow-2xl overflow-hidden text-xs">
      <div class="px-2.5 py-1 bg-slate-900 border-b border-slate-800 text-[10px] text-slate-400 font-bold uppercase">
        Mention Tables (@)
      </div>
      {#each matchingTables as table, i}
        <button
          onclick={() => insertMention(table.name)}
          class="w-full text-left px-3 py-1.5 flex items-center justify-between hover:bg-sky-600/20 font-mono text-xs transition {i === mentionIndex ? 'bg-sky-950 text-sky-300' : 'text-slate-300'}"
        >
          <span class="flex items-center space-x-1.5">
            <Icon name="table" class="w-3 h-3 text-slate-500" />
            <span>{table.name}</span>
          </span>
          <span class="text-[10px] text-slate-500 font-sans">{table.columns?.length || 0} cols</span>
        </button>
      {/each}
    </div>
  {/if}

  <!-- Prompt Input Form -->
  <div class="p-3 border-t border-[#1e293b] bg-[#070b14] flex-shrink-0">
    <div class="relative bg-[#0b0f19] border border-[#1e293b] focus-within:border-sky-500 rounded-xl overflow-hidden transition shadow-inner">
      <textarea
        bind:this={promptEl}
        bind:value={promptInput}
        oninput={handleInput}
        onkeydown={handleKeyDown}
        placeholder="Ask AI anything... (type @ for tables, Enter to send)"
        rows="2"
        class="w-full bg-transparent px-3 py-2 text-xs text-slate-200 placeholder-slate-500 resize-none focus:outline-none font-sans"
      ></textarea>

      <div class="px-2 py-1 bg-[#050811] border-t border-[#1e293b]/60 flex items-center justify-between select-none">
        <span class="text-[10px] text-slate-500 font-mono">
          Enter to send • Shift+Enter newline
        </span>
        <button
          onclick={() => sendMessage()}
          disabled={isLoading || !promptInput.trim()}
          class="px-2.5 py-1 rounded bg-sky-600 hover:bg-sky-500 disabled:opacity-40 text-white font-medium text-xs transition flex items-center space-x-1"
        >
          <span>Send</span>
          <Icon name="send" class="w-3 h-3" />
        </button>
      </div>
    </div>
  </div>

  <!-- AI Copilot Footer Status Bar -->
  <div class="h-7 px-3 bg-[#050811] border-t border-[#1e293b] flex items-center justify-between text-[10px] text-slate-500 font-mono select-none flex-shrink-0">
    <div class="flex items-center space-x-1.5">
      <span class="w-1.5 h-1.5 rounded-full {isLoading ? 'bg-amber-400 animate-ping' : 'bg-emerald-400'}"></span>
      <span class="text-slate-400">{isLoading ? 'Generating response...' : 'SqlFox Copilot Ready'}</span>
    </div>
    <button
      onclick={() => (appState.isAiSettingsModalOpen = true)}
      class="hover:text-sky-300 transition flex items-center space-x-1 text-slate-400"
      title="Configure AI Provider & Model"
    >
      <Icon name="sliders" class="w-3 h-3 text-sky-400" />
      <span>Provider Settings</span>
    </button>
  </div>
</aside>
