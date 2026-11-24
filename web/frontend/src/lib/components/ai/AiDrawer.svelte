<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { splitMarkdownSegments, renderMarkdownBlockHtml } from '../../utils/markdown';
  import { appState } from '../../stores/appState.svelte';
  import { api, loadAiConfig, syncOrMigrateLegacyAiConfig, type AiConfig, type AIProfile } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface ChatMessage {
    id: string;
    role: 'user' | 'assistant';
    content: string;
    timestamp: string;
  }

  interface ChatSession {
    id: string;
    title: string;
    messages: ChatMessage[];
    createdAt: number;
    updatedAt: number;
    forkedFrom?: string;
  }

  const SESSIONS_KEY = 'sqlfox_ai_sessions';
  const WIDTH_KEY = 'sqlfox_copilot_width';

  function createWelcomeSession(forkedFrom?: string): ChatSession {
    const now = Date.now();
    return {
      id: `ai-${now}-${Math.random().toString(36).slice(2, 6)}`,
      title: forkedFrom ? `Fork of ${forkedFrom}` : 'New Chat',
      createdAt: now,
      updatedAt: now,
      forkedFrom,
      messages: [
        {
          id: 'init',
          role: 'assistant',
          content:
            'Hello! I am your SqlFox AI Copilot. I analyze queries, EXPLAIN plans, and metadata schemas (**Zero-Row Guarantee**: row data never leaves your machine). Use `@` to reference tables, columns, or `@note:` entries.',
          timestamp: new Date(now).toISOString(),
        },
      ],
    };
  }

  function loadSessions(): ChatSession[] {
    try {
      const raw = localStorage.getItem(SESSIONS_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed) && parsed.length > 0) return parsed.slice(0, 30);
      }
    } catch { /* ignore */ }
    return [createWelcomeSession()];
  }

  const initialSessions = loadSessions();
  let sessions = $state<ChatSession[]>(initialSessions);
  let currentSessionId = $state<string>(initialSessions[0]?.id || '');

  function persistSessions(next: ChatSession[]) {
    const capped = next.slice(0, 30);
    sessions = capped;
    try {
      localStorage.setItem(SESSIONS_KEY, JSON.stringify(capped));
    } catch { /* ignore */ }
  }

  let activeSession = $derived(
    sessions.find((s) => s.id === currentSessionId) || sessions[0]!
  );
  let messages = $derived(activeSession?.messages || []);

  let aiProfiles = $state<AIProfile[]>([]);
  let activeProfile = $state<AIProfile | null>(null);
  let aiConfig = $state<AiConfig>(loadAiConfig());
  let includeSchemaOverride = $state<boolean>(true);
  let showPrivacyInfo = $state(false);
  let showHistoryPopover = $state(false);

  let promptInput = $state('');
  let isLoading = $state(false);
  let mentionQuery = $state<string | null>(null);
  let mentionIndex = $state(0);
  let cachedNotes = $state<any[]>([]);
  let messagesContainer = $state<HTMLDivElement | null>(null);
  let promptEl = $state<HTMLTextAreaElement | null>(null);

  function safeReadStorage(key: string, fallback = ''): string {
    try {
      return localStorage.getItem(key) ?? fallback;
    } catch {
      return fallback;
    }
  }

  function safeWriteStorage(key: string, val: string) {
    try {
      localStorage.setItem(key, val);
    } catch { /* ignore quota / privacy errors */ }
  }

  let savedWidth = Number(safeReadStorage(WIDTH_KEY, '480') || '480');
  let drawerWidth = $state(Number.isFinite(savedWidth) ? Math.min(920, Math.max(340, savedWidth)) : 480);
  let isMaximized = $state(safeReadStorage('sqlfox_ai_drawer_maximized') === 'true');
  let isResizing = $state(false);
  let chatRequestToken = 0;

  let providerLabel = $derived.by(() => {
    if (activeProfile?.name) return activeProfile.name;
    const p = aiConfig.provider || 'openai';
    const name =
      p === 'anthropic' ? 'Claude' : p === 'gemini' ? 'Gemini' : p === 'ollama' ? 'Ollama' : p === 'custom' ? 'Custom' : 'OpenAI';
    return aiConfig.modelName ? `${name}: ${aiConfig.modelName}` : name;
  });

  async function refreshProfiles() {
    try {
      await syncOrMigrateLegacyAiConfig();
      const list = await api.listAiProfiles();
      aiProfiles = Array.isArray(list) ? list : [];
      activeProfile = aiProfiles.find((p) => p.is_active) || aiProfiles[0] || null;
      if (activeProfile) {
        aiConfig = {
          provider: activeProfile.provider,
          apiKey: '',
          baseUrl: activeProfile.base_url,
          modelName: activeProfile.model_name,
          temperature: activeProfile.temperature,
          language: activeProfile.language,
          responseStyle: activeProfile.response_style,
          customPrompt: activeProfile.custom_prompt,
          includeSchema: activeProfile.include_schema,
          maskIdentifiers: activeProfile.mask_identifiers,
        };
        includeSchemaOverride = activeProfile.include_schema !== false;
      }
    } catch {
      aiConfig = loadAiConfig();
    }
  }

  $effect(() => {
    if (typeof window === 'undefined') return;
    const reload = () => {
      refreshProfiles();
    };
    window.addEventListener('sqlfox:ai-config-updated', reload);
    return () => window.removeEventListener('sqlfox:ai-config-updated', reload);
  });

  $effect(() => {
    if (appState.isAiDrawerOpen) {
      refreshProfiles();
      api.listNotes().then((list) => {
        cachedNotes = Array.isArray(list) ? list : [];
      }).catch(() => {});
    }
  });

  function toggleMaximized() {
    isMaximized = !isMaximized;
    safeWriteStorage('sqlfox_ai_drawer_maximized', String(isMaximized));
  }

  let activeResizeCleanup: (() => void) | null = null;

  function startResize(e: MouseEvent) {
    e.preventDefault();
    activeResizeCleanup?.();
    isResizing = true;
    const onMove = (ev: MouseEvent) => {
      const next = Math.min(960, Math.max(340, window.innerWidth - ev.clientX));
      drawerWidth = next;
    };
    const cleanup = () => {
      isResizing = false;
      safeWriteStorage(WIDTH_KEY, String(drawerWidth));
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', cleanup);
      activeResizeCleanup = null;
    };
    activeResizeCleanup = cleanup;
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', cleanup);
  }

  $effect(() => {
    return () => {
      activeResizeCleanup?.();
    };
  });

  function adjustPromptHeight() {
    if (!promptEl) return;
    promptEl.style.height = 'auto';
    promptEl.style.height = `${Math.min(220, Math.max(62, promptEl.scrollHeight))}px`;
  }

  // Auto-scroll messages to bottom
  $effect(() => {
    if (messages.length && messagesContainer) {
      const t = setTimeout(() => {
        messagesContainer?.scrollTo({ top: messagesContainer.scrollHeight, behavior: 'smooth' });
      }, 50);
      return () => clearTimeout(t);
    }
  });

  let activeCatalog = $derived(
    (appState.activeTab?.database && appState.catalogs[appState.activeTab.database.toLowerCase()]) ||
      (appState.currentDatabase && appState.catalogs[appState.currentDatabase.toLowerCase()]) ||
      appState.catalog
  );

  interface MentionOption {
    kind: 'table' | 'column' | 'note';
    label: string;
    detail: string;
    insertText: string;
  }

  let mentionOptions = $derived.by<MentionOption[]>(() => {
    if (mentionQuery === null) return [];
    const q = mentionQuery.toLowerCase();
    const out: MentionOption[] = [];

    if (q.startsWith('note:')) {
      const nq = q.slice(5);
      for (const n of cachedNotes) {
        if (!nq || String(n.title || '').toLowerCase().includes(nq)) {
          out.push({
            kind: 'note',
            label: `@note:${n.title}`,
            detail: n.category || 'Note',
            insertText: `[Note: ${n.title}]\n${(n.content || '').slice(0, 400)}\n`,
          });
          if (out.length >= 7) break;
        }
      }
      return out;
    }

    const tables = activeCatalog?.tables || [];
    for (const t of tables) {
      if (t.name.toLowerCase().includes(q)) {
        out.push({
          kind: 'table',
          label: t.name,
          detail: `${t.columns?.length || 0} cols`,
          insertText: `\`${t.name}\` `,
        });
      }
      if (q.length >= 2 && t.columns) {
        for (const c of t.columns) {
          const full = `${t.name}.${c.name}`;
          if (full.toLowerCase().includes(q) || c.name.toLowerCase().includes(q)) {
            out.push({
              kind: 'column',
              label: full,
              detail: c.type,
              insertText: `${full} `,
            });
          }
          if (out.length >= 8) break;
        }
      }
      if (out.length >= 8) break;
    }

    if (out.length < 8) {
      for (const n of cachedNotes) {
        if (!q || String(n.title || '').toLowerCase().includes(q) || 'note'.startsWith(q)) {
          out.push({
            kind: 'note',
            label: `@note:${n.title}`,
            detail: 'Operational Note',
            insertText: `[Note: ${n.title}]\n${(n.content || '').slice(0, 400)}\n`,
          });
          if (out.length >= 8) break;
        }
      }
    }
    return out;
  });

  function handleInput(e: Event) {
    adjustPromptHeight();
    const target = e.target as HTMLTextAreaElement;
    const val = target.value;
    const cursorPos = target.selectionStart;
    const textBeforeCursor = val.slice(0, cursorPos);
    const match = textBeforeCursor.match(/@([a-zA-Z0-9_.:-]*)$/);

    if (match) {
      mentionQuery = match[1] ?? '';
      mentionIndex = 0;
    } else {
      mentionQuery = null;
    }
  }

  function insertMentionOption(opt: MentionOption) {
    const idx = promptInput.lastIndexOf('@');
    if (idx >= 0) {
      promptInput = promptInput.slice(0, idx) + opt.insertText;
    } else {
      promptInput += opt.insertText;
    }
    mentionQuery = null;
    queueMicrotask(() => {
      adjustPromptHeight();
      promptEl?.focus();
    });
  }

  function updateSessionMessages(sessionId: string, updater: (msgs: ChatMessage[]) => ChatMessage[], titleHint?: string) {
    const next = sessions.map((s) => {
      if (s.id !== sessionId) return s;
      const nextMsgs = updater(s.messages);
      const nextTitle =
        s.title === 'New Chat' && titleHint
          ? titleHint.slice(0, 42) + (titleHint.length > 42 ? '…' : '')
          : s.title;
      return { ...s, title: nextTitle, messages: nextMsgs, updatedAt: Date.now() };
    });
    // Sort active updated session near top cleanly
    const updated = next.find((s) => s.id === sessionId);
    const rest = next.filter((s) => s.id !== sessionId);
    persistSessions(updated ? [updated, ...rest] : next);
  }

  async function sendChatRequest(opts: {
    action?: 'chat' | 'explain_query' | 'explain_plan' | 'optimize' | 'fix_query';
    prompt: string;
    query?: string;
    plan?: string;
    error_message?: string;
  }) {
    const text = opts.prompt.trim();
    if (!text || isLoading) return;

    const targetSessionId = activeSession.id;
    promptInput = '';
    mentionQuery = null;
    queueMicrotask(adjustPromptHeight);

    const userMsg: ChatMessage = {
      id: String(Date.now()),
      role: 'user',
      content: text,
      timestamp: new Date().toISOString(),
    };
    updateSessionMessages(targetSessionId, (prev) => [...prev, userMsg], text);
    isLoading = true;

    const currentTab = appState.activeTab;
    const isSqlTab = currentTab && !currentTab.isTableViewer && !currentTab.isProcesslist && !currentTab.isCluster && !currentTab.isVariables && !currentTab.isFlightRecorder && !currentTab.isErdCanvas && !currentTab.isNote;
    const activeSql = opts.query ?? (isSqlTab ? currentTab?.sql || '' : '');
    const cfg: AiConfig = { ...aiConfig, includeSchema: includeSchemaOverride };

    const token = ++chatRequestToken;
    try {
      const res = await api.aiChat({
        action: opts.action || 'chat',
        profile_id: activeProfile?.id || undefined,
        prompt: text,
        query: activeSql,
        plan: opts.plan || (currentTab?.explainResult ? JSON.stringify(currentTab.explainResult) : ''),
        error_message: opts.error_message || currentTab?.activeResult?.error || '',
        connection_id: appState.activeConnection?.id || '',
        config: cfg,
      });
      if (token !== chatRequestToken) return;
      const assistantMsg: ChatMessage = {
        id: String(Date.now() + 1),
        role: 'assistant',
        content: res.content || 'No response generated.',
        timestamp: new Date().toISOString(),
      };
      updateSessionMessages(targetSessionId, (prev) => [...prev, assistantMsg]);
    } catch (err: any) {
      if (token !== chatRequestToken) return;
      const errMsg: ChatMessage = {
        id: String(Date.now() + 1),
        role: 'assistant',
        content: `Error communicating with AI provider: ${err.message}`,
        timestamp: new Date().toISOString(),
      };
      updateSessionMessages(targetSessionId, (prev) => [...prev, errMsg]);
    } finally {
      if (token === chatRequestToken) {
        isLoading = false;
      }
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (mentionQuery !== null && mentionOptions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        mentionIndex = (mentionIndex + 1) % mentionOptions.length;
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        mentionIndex = (mentionIndex - 1 + mentionOptions.length) % mentionOptions.length;
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        const selected = mentionOptions[mentionIndex];
        if (selected) insertMentionOption(selected);
        return;
      }
      if (e.key === 'Escape') {
        e.stopPropagation();
        mentionQuery = null;
        return;
      }
    }

    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey || !e.shiftKey)) {
      e.preventDefault();
      sendChatRequest({ prompt: promptInput });
    }
  }

  function resolveEditableSqlTab(sql: string, mode: 'insert' | 'replace') {
    const active = appState.activeTab;
    const isSpecial = (t: any) =>
      t.isTableViewer || t.isProcesslist || t.isCluster || t.isVariables || t.isFlightRecorder || t.isErdCanvas || t.isNote;

    if (active && !isSpecial(active)) {
      if (mode === 'replace' || !active.sql.trim()) {
        active.sql = sql;
      } else {
        active.sql = `${active.sql.trimEnd()}\n\n${sql}`;
      }
      appState.tabs = [...appState.tabs];
      appState.saveTabsState();
      return active;
    }

    const existingEmpty = appState.tabs.find((t) => !isSpecial(t) && !t.sql.trim());
    if (existingEmpty) {
      existingEmpty.sql = sql;
      appState.activeTabId = existingEmpty.id;
      appState.tabs = [...appState.tabs];
      appState.saveTabsState();
      return existingEmpty;
    }

    return appState.addTab('AI Query.sql', sql);
  }

  function insertSql(sql: string, mode: 'insert' | 'replace' | 'run') {
    resolveEditableSqlTab(sql, mode === 'replace' ? 'replace' : 'insert');
    if (mode === 'run') {
      appState.executeCurrentTab();
      appState.showToast('Executing AI SQL in worksheet!', 'info');
    } else {
      appState.showToast(
        mode === 'replace' ? 'Replaced active SQL worksheet!' : 'Inserted SQL into worksheet!',
        'success'
      );
    }
  }

  function startNewChat() {
    chatRequestToken++;
    isLoading = false;
    const next = createWelcomeSession();
    persistSessions([next, ...sessions]);
    currentSessionId = next.id;
    showHistoryPopover = false;
  }

  function forkConversation(upToIndex: number) {
    const sliced = messages.slice(0, upToIndex + 1).map((m) => ({ ...m }));
    const forked: ChatSession = {
      ...createWelcomeSession(activeSession.title),
      title: `Fork: ${activeSession.title}`.slice(0, 44),
      messages: sliced,
    };
    persistSessions([forked, ...sessions]);
    currentSessionId = forked.id;
    appState.showToast('Conversation forked to a new branch!', 'success');
  }

  function deleteSession(id: string) {
    const remaining = sessions.filter((s) => s.id !== id);
    if (remaining.length === 0) {
      const fresh = createWelcomeSession();
      persistSessions([fresh]);
      currentSessionId = fresh.id;
    } else {
      persistSessions(remaining);
      if (currentSessionId === id) currentSessionId = remaining[0]!.id;
    }
  }

  async function clearAllSessions() {
    const ok = await appState.showNativeConfirm('Delete all saved AI Copilot conversation sessions?', 'Clear AI History');
    if (!ok) return;
    const fresh = createWelcomeSession();
    persistSessions([fresh]);
    currentSessionId = fresh.id;
    showHistoryPopover = false;
  }

  // "Send to AI" draft handler
  $effect(() => {
    const draft = appState.aiPromptDraft;
    if (!draft) return;
    promptInput = appState.consumeAiPromptDraft();
    queueMicrotask(() => {
      adjustPromptHeight();
      const el = promptEl;
      if (!el) return;
      el.focus();
      el.selectionStart = el.selectionEnd = el.value.length;
    });
  });

  // Auto-run quick requests triggered elsewhere (e.g. Fix with AI / Explain Plan)
  let handledQuickNonce = 0;
  $effect(() => {
    const req = appState.aiQuickRunRequest;
    if (!req || req.nonce === handledQuickNonce) return;
    handledQuickNonce = req.nonce;
    appState.aiQuickRunRequest = null;

    const promptMap: Record<string, string> = {
      fix_query: req.error
        ? `Fix the SQL query error below and return the corrected SQL:\nError: ${req.error}`
        : 'Analyze and fix any syntax or logical issues in this SQL query.',
      explain_query: 'Explain what this SQL query does, step by step.',
      explain_plan: 'Analyze this EXPLAIN execution plan, pinpoint bottlenecks, and suggest index improvements.',
      optimize: 'Optimize this SQL query and propose covering indexes where helpful.',
    };
    sendChatRequest({
      action: req.action,
      prompt: promptMap[req.action] || 'Help with this SQL query.',
      query: req.sql,
      plan: req.plan,
      error_message: req.error,
    });
  });

  function formatTime(iso: string): string {
    const d = new Date(iso);
    return isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
</script>

<aside
  class="fixed top-12 right-0 bottom-7 bg-[#0b0f19] border-l border-[#1e293b] shadow-2xl z-40 flex flex-col font-sans select-text {isResizing ? '' : 'transition-[width] duration-150'}"
  style="width: {isMaximized
    ? 'min(58rem, calc(100vw - 1.5rem))'
    : `min(${drawerWidth}px, calc(100vw - 1.5rem))`};"
>
  <!-- Left-Edge Resizer Handle -->
  <button
    type="button"
    tabindex="-1"
    aria-label="Resize AI Copilot drawer"
    onmousedown={startResize}
    class="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-sky-500/40 transition z-50 border-0 p-0 bg-transparent"
  ></button>

  <!-- Drawer Header -->
  <div class="h-10 bg-[#0f172a] border-b border-[#1e293b] px-3.5 flex items-center justify-between flex-shrink-0 select-none gap-2">
    <div class="flex items-center space-x-2 min-w-0 flex-1">
      <Icon name="bot" class="w-4 h-4 text-sky-400 shrink-0" />
      <span class="font-bold text-slate-100 text-xs shrink-0 hidden sm:inline">Copilot</span>
      {#if aiProfiles.length > 0}
        <select
          value={activeProfile?.id || ''}
          onchange={async (e) => {
            const targetId = e.currentTarget.value;
            if (targetId && targetId !== activeProfile?.id) {
              try {
                await api.setActiveAiProfile(targetId);
                window.dispatchEvent(new CustomEvent('sqlfox:ai-config-updated'));
                await refreshProfiles();
                appState.showToast(`Switched active AI profile: ${activeProfile?.name || 'selected'}`, 'info');
              } catch (err: any) {
                appState.showToast(`Failed to switch profile: ${err.message}`, 'error');
              }
            }
          }}
          class="bg-[#050811] border border-slate-700 hover:border-sky-500 rounded px-1.5 py-0.5 text-[11px] font-medium text-sky-300 max-w-[170px] truncate focus:outline-none cursor-pointer"
          title="Active AI Copilot Profile (Click to switch)"
        >
          {#each aiProfiles as p}
            <option value={p.id}>
              {p.name}
            </option>
          {/each}
        </select>
      {:else}
        <button
          id="ai-provider-badge"
          type="button"
          onclick={() => (appState.isAiSettingsModalOpen = true)}
          class="text-[10px] text-sky-400 hover:underline font-mono truncate max-w-[140px]"
          title="Click to configure AI provider & model"
        >
          {providerLabel}
        </button>
      {/if}
    </div>

    <div class="flex items-center space-x-1 relative shrink-0">
      <button
        type="button"
        onclick={startNewChat}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title="New Chat"
      >
        <Icon name="plus" class="w-3.5 h-3.5" />
      </button>
      <button
        type="button"
        onclick={() => (showHistoryPopover = !showHistoryPopover)}
        class="p-1 rounded transition {showHistoryPopover ? 'bg-slate-800 text-sky-300' : 'text-slate-400 hover:text-white hover:bg-slate-800'}"
        title="Session History ({sessions.length})"
      >
        <Icon name="history" class="w-3.5 h-3.5" />
      </button>
      <button
        type="button"
        onclick={toggleMaximized}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title={isMaximized ? 'Restore Copilot Width' : 'Maximize Copilot'}
      >
        <Icon name={isMaximized ? 'minimize-2' : 'maximize-2'} class="w-3.5 h-3.5" />
      </button>
      <button
        type="button"
        onclick={() => (appState.isAiSettingsModalOpen = true)}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title="AI Settings / BYOK"
      >
        <Icon name="settings" class="w-3.5 h-3.5" />
      </button>
      <button
        type="button"
        onclick={() => (appState.isAiDrawerOpen = false)}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title="Close Drawer"
      >
        <Icon name="x" class="w-3.5 h-3.5" />
      </button>

      {#if showHistoryPopover}
        <div class="fixed inset-0 z-30" role="presentation" onclick={() => (showHistoryPopover = false)}></div>
        <div class="absolute right-0 top-8 w-72 bg-[#0f172a] border border-slate-700 rounded-xl shadow-2xl z-40 overflow-hidden text-xs">
          <div class="px-3 py-2 border-b border-slate-800 flex items-center justify-between bg-slate-900">
            <span class="font-bold text-slate-200">Saved Chats ({sessions.length}/30)</span>
            <button type="button" onclick={clearAllSessions} class="text-[10px] text-rose-400 hover:underline">
              Clear All
            </button>
          </div>
          <div class="max-h-64 overflow-y-auto divide-y divide-slate-800/70">
            {#each sessions as s (s.id)}
              <div class="px-3 py-2 flex items-center justify-between gap-2 hover:bg-slate-800/60 {s.id === activeSession.id ? 'bg-sky-950/40' : ''}">
                <button
                  type="button"
                  onclick={() => { currentSessionId = s.id; showHistoryPopover = false; }}
                  class="flex-1 text-left min-w-0"
                >
                  <div class="font-semibold text-slate-200 truncate">{s.title}</div>
                  <div class="text-[10px] text-slate-500 font-mono">
                    {s.messages.length} msgs · {new Date(s.updatedAt).toLocaleDateString()}
                  </div>
                </button>
                <button
                  type="button"
                  onclick={() => deleteSession(s.id)}
                  class="p-1 text-slate-500 hover:text-rose-400 rounded"
                  title="Delete chat"
                >
                  <Icon name="trash-2" class="w-3 h-3" />
                </button>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  </div>

  <!-- Zero-Row Guarantee & Context Guard Strip -->
  <div class="px-3 py-1.5 border-b border-[#1e293b] bg-[#060913] flex items-center justify-between text-[10.5px] select-none shrink-0 gap-2">
    <button
      type="button"
      onclick={() => (showPrivacyInfo = !showPrivacyInfo)}
      class="flex items-center space-x-1 text-emerald-400 hover:text-emerald-300 font-medium"
      title="Schema metadata only; table rows are never transmitted"
    >
      <Icon name="shield-check" class="w-3 h-3 shrink-0" />
      <span>Zero-Row Guarantee {aiConfig.maskIdentifiers ? '· Masked' : ''}</span>
    </button>

    <label class="flex items-center space-x-1.5 text-slate-300 cursor-pointer">
      <input
        type="checkbox"
        bind:checked={includeSchemaOverride}
        class="rounded bg-slate-900 border-slate-700 text-sky-500 focus:ring-0"
      />
      <span>Include Schema</span>
    </label>
  </div>

  {#if showPrivacyInfo}
    <div class="px-3 py-2 bg-emerald-950/25 border-b border-emerald-900/50 text-[10.5px] text-emerald-200/90 leading-relaxed shrink-0">
      SqlFox only sends SQL structure &amp; optional column DDL metadata. Zero table row values leave your environment.
      Privacy Firewall masking is currently <strong>{aiConfig.maskIdentifiers ? 'ENABLED' : 'DISABLED'}</strong> in Settings.
    </div>
  {/if}

  <!-- Quick Action Chips -->
  <div class="p-2 border-b border-[#1e293b] bg-[#070b14] flex flex-wrap gap-1 flex-shrink-0 select-none">
    <button
      type="button"
      onclick={() => sendChatRequest({ action: 'explain_query', prompt: 'Explain what this SQL query does and any potential bottlenecks.' })}
      disabled={isLoading}
      class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 hover:border-sky-500 text-slate-400 hover:text-sky-300 text-[10px] transition flex items-center space-x-1"
    >
      <Icon name="help-circle" class="w-3 h-3 text-sky-400" />
      <span>Explain Query</span>
    </button>
    <button
      type="button"
      onclick={() => sendChatRequest({ action: 'explain_plan', prompt: 'Analyze the EXPLAIN execution plan and point out table scans or missing indexes.' })}
      disabled={isLoading}
      class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 hover:border-cyan-500 text-slate-400 hover:text-cyan-300 text-[10px] transition flex items-center space-x-1"
    >
      <Icon name="activity" class="w-3 h-3 text-cyan-400" />
      <span>Explain Plan</span>
    </button>
    <button
      type="button"
      onclick={() => sendChatRequest({ action: 'fix_query', prompt: 'Analyze and fix any errors or syntax issues in this SQL query.' })}
      disabled={isLoading}
      class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 hover:border-amber-500 text-slate-400 hover:text-amber-300 text-[10px] transition flex items-center space-x-1"
    >
      <Icon name="check-circle" class="w-3 h-3 text-amber-400" />
      <span>Fix Query</span>
    </button>
    <button
      type="button"
      onclick={() => sendChatRequest({ action: 'optimize', prompt: 'Optimize this query and suggest indexes for best performance.' })}
      disabled={isLoading}
      class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 hover:border-emerald-500 text-slate-400 hover:text-emerald-300 text-[10px] transition flex items-center space-x-1"
    >
      <Icon name="zap" class="w-3 h-3 text-emerald-400" />
      <span>Optimize</span>
    </button>
  </div>

  <!-- Messages List -->
  <div bind:this={messagesContainer} class="flex-1 overflow-y-auto p-3 space-y-3">
    {#each messages as msg, msgIdx (msg.id)}
      {@const isUser = msg.role === 'user'}
      <div class="flex flex-col {isUser ? 'items-end' : 'items-start'}">
        <div class="flex items-center space-x-1.5 mb-1 px-1">
          <span class="text-[10px] text-slate-500 font-mono">
            {isUser ? 'You' : 'SqlFox AI'} · {formatTime(msg.timestamp)}
          </span>
        </div>

        <div
          class="max-w-[94%] rounded-xl px-3 py-2 text-xs leading-relaxed {isUser
            ? 'bg-[#16233b] border border-sky-700/50 text-slate-100 rounded-tr-none shadow-md space-y-1.5'
            : 'bg-[#131b2e] border border-[#1e293b] text-slate-200 rounded-tl-none shadow-md space-y-2'}"
        >
          {#if isUser}
            {#each splitMarkdownSegments(msg.content) as seg}
              {#if seg.type === 'code'}
                <pre class="my-1 p-2 rounded-lg bg-[#0b1220] border border-sky-900/50 text-sky-200 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap">{seg.content}</pre>
              {:else}
                <div>{@html renderMarkdownBlockHtml(seg.content)}</div>
              {/if}
            {/each}
          {:else}
            {#each splitMarkdownSegments(msg.content) as seg}
              {#if seg.type === 'code'}
                <div class="my-1.5 rounded-lg overflow-hidden border border-sky-800/60 bg-[#050811]">
                  <div class="bg-[#0b0f19] px-2.5 py-1 flex items-center justify-between border-b border-sky-950 text-[10px] font-mono text-slate-400 select-none gap-2 flex-wrap">
                    <span class="uppercase">{seg.lang || 'SQL'}</span>
                    <div class="flex items-center space-x-2">
                      <button
                        type="button"
                        onclick={() => { copyText(seg.content); appState.showToast('SQL copied to clipboard!', 'info'); }}
                        class="hover:text-white flex items-center space-x-0.5"
                        title="Copy SQL"
                      >
                        <Icon name="copy" class="w-3 h-3" />
                        <span>Copy</span>
                      </button>
                      <button
                        type="button"
                        onclick={() => insertSql(seg.content, 'insert')}
                        class="hover:text-sky-300 text-sky-400 flex items-center space-x-0.5"
                        title="Insert into Worksheet"
                      >
                        <Icon name="corner-down-left" class="w-3 h-3" />
                        <span>Insert</span>
                      </button>
                      <button
                        type="button"
                        onclick={() => insertSql(seg.content, 'replace')}
                        class="hover:text-amber-300 text-amber-400 flex items-center space-x-0.5"
                        title="Replace active worksheet SQL"
                      >
                        <Icon name="edit" class="w-2.5 h-2.5" />
                        <span>Replace</span>
                      </button>
                      <button
                        type="button"
                        onclick={() => insertSql(seg.content, 'run')}
                        class="hover:text-emerald-300 text-emerald-400 font-bold flex items-center space-x-0.5"
                        title="Insert and Run immediately"
                      >
                        <Icon name="play" class="w-2.5 h-2.5" />
                        <span>Run</span>
                      </button>
                    </div>
                  </div>
                  <pre class="p-2.5 text-sky-200 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap">{seg.content}</pre>
                </div>
              {:else}
                <div>{@html renderMarkdownBlockHtml(seg.content)}</div>
              {/if}
            {/each}

            {#if msg.id !== 'init'}
              <div class="pt-1.5 border-t border-slate-800/80 flex items-center justify-end space-x-2 text-[10px] text-slate-400 select-none">
                <button
                  type="button"
                  onclick={() => { copyText(msg.content); appState.showToast('Response copied to clipboard!', 'info'); }}
                  class="hover:text-white flex items-center space-x-1"
                >
                  <Icon name="copy" class="w-2.5 h-2.5" /><span>Copy</span>
                </button>
                <button
                  type="button"
                  onclick={() => appState.saveMarkdownToNote(msg.content, `AI Copilot Note (${formatTime(msg.timestamp)})`, { category: 'AI Copilot', userTags: ['#ai-copilot'] })}
                  class="hover:text-amber-300 flex items-center space-x-1"
                >
                  <Icon name="bookmark" class="w-2.5 h-2.5" /><span>Save to Notes</span>
                </button>
                <button
                  type="button"
                  onclick={() => forkConversation(msgIdx)}
                  class="hover:text-sky-300 flex items-center space-x-1"
                >
                  <Icon name="git-branch" class="w-2.5 h-2.5" /><span>Fork</span>
                </button>
              </div>
            {/if}
          {/if}
        </div>
      </div>
    {/each}

    {#if isLoading}
      <div class="flex items-center space-x-2 text-slate-500 text-xs p-2">
        <Icon name="loader" class="w-3.5 h-3.5 animate-spin text-sky-400" />
        <span>SqlFox AI is analyzing schema &amp; query context...</span>
      </div>
    {/if}
  </div>

  <!-- Mention Auto-Complete Popover -->
  {#if mentionQuery !== null && mentionOptions.length > 0}
    <div class="mx-3 mb-1 bg-[#0f172a] border border-sky-500/50 rounded-xl shadow-2xl overflow-hidden text-xs max-h-48 overflow-y-auto">
      <div class="px-2.5 py-1 bg-slate-900 border-b border-slate-800 text-[10px] text-slate-400 font-bold uppercase">
        Mention Tables, Columns, or @note:
      </div>
      {#each mentionOptions as opt, i}
        <button
          type="button"
          onclick={() => insertMentionOption(opt)}
          class="w-full text-left px-3 py-1.5 flex items-center justify-between hover:bg-sky-600/20 font-mono text-xs transition {i === mentionIndex ? 'bg-sky-950 text-sky-300' : 'text-slate-300'}"
        >
          <span class="flex items-center space-x-1.5 truncate">
            <Icon name={opt.kind === 'note' ? 'file-text' : opt.kind === 'column' ? 'code' : 'table'} class="w-3 h-3 text-slate-500 shrink-0" />
            <span class="truncate">{opt.label}</span>
          </span>
          <span class="text-[10px] text-slate-500 font-sans shrink-0 ml-2">{opt.detail}</span>
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
        placeholder="Ask Copilot (use @table, @col, or @note: ... Enter = Send)"
        rows="2"
        class="w-full bg-transparent px-3 py-2 text-xs text-slate-200 placeholder-slate-500 resize-none focus:outline-none font-sans max-h-[220px]"
      ></textarea>

      <div class="px-2 py-1 bg-[#050811] border-t border-[#1e293b]/60 flex items-center justify-between select-none">
        <span class="text-[10px] text-slate-500 font-mono">
          Enter to send · Shift+Enter newline
        </span>
        <button
          type="button"
          onclick={() => sendChatRequest({ prompt: promptInput })}
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
      type="button"
      onclick={() => (appState.isAiSettingsModalOpen = true)}
      class="hover:text-sky-300 transition flex items-center space-x-1 text-slate-400"
      title="Configure AI Provider & Model"
    >
      <Icon name="sliders" class="w-3 h-3 text-sky-400" />
      <span>Settings</span>
    </button>
  </div>
</aside>
