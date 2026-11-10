// --- ReadOnly Mode ---
    async function toggleReadOnly() {
      if (!activeConnection) return;
      if (activeConnection.environment === 'Production' && activeSession && activeSession.read_only) {
        const ok = await showNativeConfirm('Switch to Read-Write mode for this Production session? Data modification operations (INSERT/UPDATE/DELETE/DDL) will be permitted.', 'Unlock Read-Write (Production)');
        if (!ok) return;
      }
      const res = await apiFetch(`/api/session/${activeConnection.id}/toggle-readonly`, { method: 'POST' });
      const data = await res.json();
      activeSession.read_only = data.read_only;
      updateReadOnlyUI(data.read_only);
    }

    function updateReadOnlyUI(isReadOnly) {
      const btn = document.getElementById('readonly-toggle-btn');
      const icon = document.getElementById('readonly-icon');
      const text = document.getElementById('readonly-text');
      btn.classList.remove('hidden');

      if (isReadOnly) {
        btn.className = 'h-7 px-2.5 rounded font-medium transition border flex items-center space-x-1.5 bg-red-950/60 border-red-700 text-red-300 hover:bg-red-900/60';
        icon.innerHTML = renderIcon('lock', 'w-3.5 h-3.5 text-red-400');
        text.textContent = 'Read-Only';
      } else {
        btn.className = 'h-7 px-2.5 rounded font-medium transition border flex items-center space-x-1.5 bg-emerald-950/60 border-emerald-700 text-emerald-300 hover:bg-emerald-900/60';
        icon.innerHTML = renderIcon('unlock', 'w-3.5 h-3.5 text-emerald-400');
        text.textContent = 'Write Allowed';
      }
    }

    // --- AI Assistant ---
    let isAIMaximized = false;
    let savedAIWidthBeforeMaximize = null;

    function autoResizeAIPrompt(el) {
      if (!el) return;
      el.style.height = 'auto';
      const newHeight = Math.min(Math.max(el.scrollHeight, 96), 260);
      el.style.height = newHeight + 'px';
    }

    let selectedMentionIdx = 0;
    let currentMentionMatches = [];

    function handleAIPromptInput(el) {
      autoResizeAIPrompt(el);
      const val = el.value;
      const cursorPos = el.selectionStart;
      const textBeforeCursor = val.slice(0, cursorPos);
      const match = textBeforeCursor.match(/@([a-zA-Z0-9_\-\s]*)$/);

      const dropdown = document.getElementById('ai-mention-dropdown');
      if (!dropdown) return;

      if (!match) {
        dropdown.classList.add('hidden');
        currentMentionMatches = [];
        return;
      }

      const query = match[1].toLowerCase().trim();
      currentMentionMatches = allOperationalNotes.filter(n => {
        return (n.title || '').toLowerCase().includes(query) || (n.side_note || '').toLowerCase().includes(query) || (n.id || '').toLowerCase().includes(query);
      }).slice(0, 8);

      if (currentMentionMatches.length === 0) {
        dropdown.classList.add('hidden');
        return;
      }

      selectedMentionIdx = 0;
      renderMentionDropdown(currentMentionMatches);
    }

    function renderMentionDropdown(matches) {
      const dropdown = document.getElementById('ai-mention-dropdown');
      if (!dropdown) return;
      dropdown.classList.remove('hidden');

      dropdown.innerHTML = matches.map((m, idx) => `
        <div onclick="selectNoteMention(${idx})" class="p-2 hover:bg-slate-800 cursor-pointer flex flex-col space-y-0.5 ${idx === selectedMentionIdx ? 'bg-slate-800/90 border-l-2 border-app-accent' : ''}">
          <div class="flex items-center justify-between">
            <span class="font-bold text-white text-xs truncate flex items-center gap-1">${renderIcon('file-text', 'w-3.5 h-3.5 text-sky-400 shrink-0')}<span>${escapeHtml(m.title)}</span></span>
            <span class="text-[10px] text-slate-400 font-mono">${m.status}</span>
          </div>
          ${m.side_note ? `<span class="text-[10px] text-amber-300 font-mono truncate flex items-center gap-1">${renderIcon('sparkles', 'w-3 h-3 text-amber-300 shrink-0')}<span>${escapeHtml(m.side_note)}</span></span>` : ''}
        </div>
      `).join('');
    }

    function selectNoteMention(idx) {
      if (!currentMentionMatches[idx]) return;
      const note = currentMentionMatches[idx];
      const el = document.getElementById('ai-prompt-input');
      const dropdown = document.getElementById('ai-mention-dropdown');
      if (!el) return;

      const val = el.value;
      const cursorPos = el.selectionStart;
      const textBeforeCursor = val.slice(0, cursorPos);
      const textAfterCursor = val.slice(cursorPos);

      const replacedBefore = textBeforeCursor.replace(/@[a-zA-Z0-9_\-\s]*$/, `[@note:${note.title}] `);
      el.value = replacedBefore + textAfterCursor;
      el.selectionStart = el.selectionEnd = replacedBefore.length;
      el.focus();
      if (dropdown) dropdown.classList.add('hidden');
      currentMentionMatches = [];
      autoResizeAIPrompt(el);
    }

    function handleAIPromptKeydown(e) {
      const dropdown = document.getElementById('ai-mention-dropdown');
      const isDropdownOpen = dropdown && !dropdown.classList.contains('hidden') && currentMentionMatches.length > 0;

      if (isDropdownOpen) {
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          selectedMentionIdx = (selectedMentionIdx + 1) % currentMentionMatches.length;
          renderMentionDropdown(currentMentionMatches);
          return;
        } else if (e.key === 'ArrowUp') {
          e.preventDefault();
          selectedMentionIdx = (selectedMentionIdx - 1 + currentMentionMatches.length) % currentMentionMatches.length;
          renderMentionDropdown(currentMentionMatches);
          return;
        } else if (e.key === 'Enter' || e.key === 'Tab') {
          e.preventDefault();
          selectNoteMention(selectedMentionIdx);
          return;
        } else if (e.key === 'Escape') {
          e.preventDefault();
          dropdown.classList.add('hidden');
          currentMentionMatches = [];
          return;
        }
      }

      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        submitAIChat(e);
      }
    }

    async function submitAIChat(e) {
      if (e && typeof e.preventDefault === 'function') e.preventDefault();
      const input = document.getElementById('ai-prompt-input');
      const prompt = input.value.trim();
      if (!prompt) return;

      appendAIMessage('user', prompt);
      input.value = '';
      autoResizeAIPrompt(input);

      await callAI('chat', prompt);
    }

    async function aiQuickAction(action) {
      const sql = getEditorSQL().trim();
      appendAIMessage('user', `Run ${action} on: \`${sql}\``);
      await callAI(action, '', sql);
    }

    async function callAI(action, prompt = '', query = '', extra = {}) {
      appendAIMessage('system', 'Thinking...');
      try {
        const includeSchema = document.getElementById('ai-include-schema') ? document.getElementById('ai-include-schema').checked : true;
        let connId = activeConnection ? activeConnection.id : '';
        if (!connId && connections && connections.length === 1) {
          connId = connections[0].id;
        }
        const res = await apiFetch('/api/ai/chat', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            action: action,
            prompt: prompt,
            query: query || getEditorSQL().trim(),
            error_message: extra.error_message || '',
            plan: extra.plan || '',
            connection_id: connId,
            provider: aiConfig.provider,
            api_key: aiConfig.apiKey,
            model: aiConfig.model,
            endpoint: aiConfig.endpoint,
            mask_identifiers: aiConfig.maskIdentifiers !== false,
            language: aiConfig.language || 'id',
            response_style: aiConfig.responseStyle || 'concise',
            custom_prompt: aiConfig.customPrompt || '',
            include_schema: includeSchema
          })
        });
        const data = await res.json();
        removeLastAIMessage();
        if (data.error) {
          appendAIMessage('error', data.error);
        } else {
          appendAIMessage('assistant', data.response || data.content || 'No response from model');
        }
      } catch (err) {
        removeLastAIMessage();
        appendAIMessage('error', 'AI request failed: ' + err.message);
      }
    }

    function renderCopilotMarkdown(rawText) {
      if (!rawText) return '';

      // 1. Normalize line endings (CRLF -> LF)
      let text = String(rawText).replace(/\r\n/g, '\n').replace(/\r/g, '\n');

      // 2. Extract code blocks safely
      const blocks = [];
      text = text.replace(/```([a-zA-Z0-9_\-]*)[ \t]*\n([\s\S]*?)```/g, (match, lang, code) => {
        const id = `__SQLFOX_BLOCK_${blocks.length}__`;
        const trimmedCode = code.trim();
        const lowerLang = (lang || '').toLowerCase().trim();

        if (lowerLang === 'sql' || (!lowerLang && (trimmedCode.toUpperCase().startsWith('SELECT') || trimmedCode.toUpperCase().startsWith('INSERT') || trimmedCode.toUpperCase().startsWith('UPDATE') || trimmedCode.toUpperCase().startsWith('DELETE') || trimmedCode.toUpperCase().startsWith('CREATE') || trimmedCode.toUpperCase().startsWith('ALTER') || trimmedCode.toUpperCase().startsWith('DROP') || trimmedCode.toUpperCase().startsWith('EXPLAIN')))) {
          blocks.push(`
            <div class="my-2.5 bg-slate-950 p-2.5 rounded border border-slate-700 font-mono text-[11px] text-sky-300 overflow-x-auto">
              <div class="flex justify-between items-center mb-1.5 text-[10px] text-slate-400 border-b border-slate-800 pb-1.5 flex-wrap gap-1">
                <span class="font-bold text-slate-300 flex items-center space-x-1">${renderIcon('zap', 'w-3.5 h-3.5 text-amber-400')}<span>Generated SQL</span></span>
                <div class="flex items-center space-x-1.5">
                  <button type="button" onclick="insertSQLFromBlock(this)" class="hover:text-app-accent px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-sky-300 transition" title="Insert & Execute">Insert & Run</button>
                  <button type="button" onclick="replaceSQLFromBlock(this)" class="hover:text-app-accent px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 transition" title="Replace Editor Content">Replace</button>
                  <button type="button" onclick="copySQLFromBlock(this)" class="hover:text-app-accent px-1.5 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 transition" title="Copy SQL">Copy SQL</button>
                </div>
              </div>
              <code class="block whitespace-pre overflow-x-auto font-mono text-sky-200">${escapeHtml(trimmedCode)}</code>
            </div>
          `);
        } else {
          blocks.push(`
            <pre class="my-2 p-2.5 bg-slate-950 border border-slate-800 rounded font-mono text-xs text-sky-300 overflow-x-auto leading-relaxed"><code>${escapeHtml(trimmedCode)}</code></pre>
          `);
        }
        return `\n\n${id}\n\n`;
      });

      // 3. Extract tables
      const tableRegex = /((?:^|\n)\|[^\n]+\|\n\|[\s\-:|]+\|\n(?:\|[^\n]+\|\n?)+)/g;
      text = text.replace(tableRegex, (match) => {
        const id = `__SQLFOX_BLOCK_${blocks.length}__`;
        const lines = match.trim().split('\n').filter(l => l.trim().startsWith('|'));
        if (lines.length < 2) return match;
        const parseRow = (line) => line.split('|').slice(1, -1).map(c => c.trim());
        const headers = parseRow(lines[0]);
        const rows = lines.slice(2).map(parseRow);
        const ths = headers.map(h => `<th class="p-1.5 border border-slate-700 bg-slate-900 text-slate-200 font-semibold">${escapeHtml(h)}</th>`).join('');
        const trs = rows.map(r => {
          const tds = r.map(c => `<td class="p-1.5 border border-slate-800 text-slate-300">${escapeHtml(c)}</td>`).join('');
          return `<tr class="hover:bg-slate-800/40">${tds}</tr>`;
        }).join('');
        blocks.push(`<div class="overflow-x-auto my-2.5 rounded border border-slate-700"><table class="w-full text-[11px] text-left border-collapse"><thead><tr>${ths}</tr></thead><tbody>${trs}</tbody></table></div>`);
        return `\n\n${id}\n\n`;
      });

      // 4. Escape remaining HTML
      let safe = escapeHtml(text);

      // Horizontal rules
      safe = safe.replace(/^\s*---+\s*$/gim, '\n<hr class="my-4 border-slate-800"/>\n');

      // 5. Inline code
      safe = safe.replace(/`([^`]+)`/g, '<code class="bg-slate-800 text-sky-300 px-1 py-0.5 rounded text-[11px] font-mono">$1</code>');

      // 6. Headings (H1 to H6)
      safe = safe.replace(/^\s*###### (.*$)/gim, '\n<h6 class="text-[11px] font-bold text-slate-300 mt-2 mb-0.5">$1</h6>\n');
      safe = safe.replace(/^\s*##### (.*$)/gim, '\n<h5 class="text-[11px] font-bold text-slate-200 mt-2 mb-0.5">$1</h5>\n');
      safe = safe.replace(/^\s*#### (.*$)/gim, '\n<h4 class="text-xs font-bold text-slate-200 mt-2.5 mb-1 pb-0.5">$1</h4>\n');
      safe = safe.replace(/^\s*### (.*$)/gim, '\n<h3 class="text-xs font-bold text-slate-100 mt-2.5 mb-1 border-b border-slate-800/80 pb-0.5">$1</h3>\n');
      safe = safe.replace(/^\s*## (.*$)/gim, '\n<h2 class="text-sm font-bold text-white mt-3 mb-1 pb-0.5 border-b border-slate-700">$1</h2>\n');
      safe = safe.replace(/^\s*# (.*$)/gim, '\n<h1 class="text-base font-bold text-white mt-3.5 mb-1.5 pb-0.5 border-b border-slate-600">$1</h1>\n');

      // 7. Lists BEFORE bold/italics
      safe = safe.replace(/^\s*[-*]\s+(.*$)/gim, '<li class="ml-4 list-disc text-slate-300 my-0.5">$1</li>');
      safe = safe.replace(/^\s*(?:\d+)\.\s+(.*$)/gim, '<li class="ml-4 list-decimal text-slate-300 my-0.5">$1</li>');

      // Wrap consecutive list items in <ul>
      safe = safe.replace(/((?:<li[^>]*>[\s\S]*?<\/li>\n?)+)/g, '\n<ul class="my-1.5 space-y-0.5">$1</ul>\n');

      // 8. Bold & Italics
      safe = safe.replace(/\*\*([^*]+)\*\*/g, '<strong class="font-bold text-slate-100">$1</strong>');
      safe = safe.replace(/\*([^*]+)\*/g, '<em class="italic text-slate-300">$1</em>');

      // 9. Blockquotes
      safe = safe.replace(/^&gt;\s+(.*$)/gim, '\n<blockquote class="border-l-2 border-app-accent pl-2.5 my-1.5 text-slate-400 italic text-[11px]">$1</blockquote>\n');

      // 10. Paragraphs
      const paragraphs = safe.split(/\n{2,}/);
      safe = paragraphs.map(p => {
        p = p.trim();
        if (!p) return '';
        if (p.startsWith('__SQLFOX_BLOCK_') || p.startsWith('<h') || p.startsWith('<ul') || p.startsWith('<ol') || p.startsWith('<pre') || p.startsWith('<div') || p.startsWith('<blockquote') || p.startsWith('<hr')) {
          return p;
        }
        return `<p class="my-1.5 text-slate-200 leading-relaxed">${p.replace(/\n/g, '<br/>')}</p>`;
      }).join('');

      // 11. Restore blocks
      blocks.forEach((blockHtml, i) => {
        safe = safe.replace(new RegExp(`__SQLFOX_BLOCK_${i}__`, 'g'), blockHtml);
      });

      return safe;
    }

    function insertSQLFromBlock(btn) {
      const codeEl = btn.closest('.font-mono').querySelector('code');
      if (codeEl) {
        setEditorSQL(codeEl.textContent.trim());
        runActiveQuery();
      }
    }

    function replaceSQLFromBlock(btn) {
      const codeEl = btn.closest('.font-mono').querySelector('code');
      if (codeEl) {
        setEditorSQL(codeEl.textContent.trim());
      }
    }

    function copySQLFromBlock(btn) {
      const codeEl = btn.closest('.font-mono').querySelector('code');
      if (codeEl) {
        copySQLToClipboard(codeEl.textContent.trim());
      }
    }

    function appendAIMessage(role, text, save = true) {
      const container = document.getElementById('ai-messages');
      const div = document.createElement('div');
      div.className = 'ai-chat-bubble ';
      div.dataset.role = role;
      div.dataset.rawText = text;

      if (role === 'user') {
        div.className += 'bg-sky-950/60 p-2.5 rounded-lg border border-sky-800 text-sky-100 shadow-sm';
        const formatted = renderCopilotMarkdown(text);
        div.innerHTML = `<div class="text-[10px] text-sky-400 font-bold mb-1">You</div><div class="user-content leading-relaxed text-sky-100">${formatted}</div>`;
      } else if (role === 'assistant') {
        div.className += 'bg-app-card p-3 rounded-lg border border-app-border text-slate-200 shadow-sm';
        const formatted = renderCopilotMarkdown(text);
        div.innerHTML = `
          <div class="copilot-content">${formatted}</div>
          <div class="flex items-center justify-between pt-2 mt-2 border-t border-slate-800/80 text-[11px] text-slate-400 flex-wrap gap-1">
            <span class="text-[10px] text-slate-500 font-mono">Copilot</span>
            <div class="flex items-center space-x-1 sm:space-x-2">
              <button onclick="copyAIMessageBubble(this)" class="hover:text-app-accent px-1.5 py-0.5 rounded hover:bg-slate-800 transition flex items-center space-x-1" title="Copy response to clipboard">
                ${renderIcon('copy', 'w-3 h-3 mr-1')}
                <span>Copy</span>
              </button>
              <button onclick="saveAIMessageToNotes(this)" class="hover:text-app-accent px-1.5 py-0.5 rounded hover:bg-slate-800 transition flex items-center space-x-1" title="Save response to Notes.md">
                ${renderIcon('file-text', 'w-3 h-3 mr-1')}
                <span>Save to Notes</span>
              </button>
              <button onclick="forkAIChatSessionFromMsg(this)" class="hover:text-emerald-400 px-1.5 py-0.5 rounded hover:bg-slate-800 transition flex items-center space-x-1 text-slate-300" title="Fork conversation into a new chat session from this point">
                ${renderIcon('git-branch', 'w-3 h-3 mr-1')}
                <span>Fork</span>
              </button>
            </div>
          </div>
        `;
      } else if (role === 'error') {
        div.className += 'bg-red-950/40 p-2.5 rounded-lg text-xs text-red-300 border border-red-800/60';
        div.innerHTML = `<b>Error:</b> ${escapeHtml(text)}`;
      } else {
        div.className += 'text-slate-500 italic text-[11px]';
        div.textContent = text;
      }
      container.appendChild(div);
      container.scrollTop = container.scrollHeight;

      if (save && (role === 'user' || role === 'assistant')) {
        saveCurrentChatSession();
      }
    }

    function removeLastAIMessage() {
      const container = document.getElementById('ai-messages');
      if (container.lastChild) container.removeChild(container.lastChild);
    }

    async function copySQLToClipboard(sql) {
      const ok = await copyTextToClipboard(sql);
      if (ok) {
        showNativeAlert('📋 SQL copied to clipboard!', 'Clipboard');
      } else {
        showNativeAlert('Failed to copy SQL in this browser context.', 'Clipboard Error');
      }
    }

    async function copyAIMessageBubble(btn) {
      const bubble = btn.closest('.ai-chat-bubble');
      if (!bubble) return;
      const rawText = bubble.dataset.rawText || '';
      const ok = await copyTextToClipboard(rawText);
      if (ok) {
        const origHtml = btn.innerHTML;
        btn.innerHTML = '<span class="text-emerald-400 font-bold">✓ Copied!</span>';
        setTimeout(() => {
          btn.innerHTML = origHtml;
        }, 1600);
      }
    }

    function saveAIMessageToNotes(btn) {
      const bubble = btn.closest('.ai-chat-bubble');
      if (!bubble) return;
      const rawText = bubble.dataset.rawText || '';
      const activeTab = tabs.find(t => t.id === activeTabId);
      const objTags = [];
      if (activeConnection && activeConnection.id) {
        const db = (activeTab && activeTab.database) || (activeConnection.database_name) || '';
        if (activeTab && activeTab.tableName) {
          objTags.push(`db://${activeConnection.id}/${db}/${activeTab.tableName}`);
        } else {
          objTags.push(`db://${activeConnection.id}/${db}`);
        }
      }
      saveToInternalNotes('Copilot Explanation', rawText, objTags, ['#ai-copilot']);
    }

    function forkAIChatSessionFromMsg(btn) {
      const bubble = btn.closest('.ai-chat-bubble');
      if (!bubble) return;

      const container = document.getElementById('ai-messages');
      const allBubbles = Array.from(container.querySelectorAll('.ai-chat-bubble'));
      const targetIdx = allBubbles.indexOf(bubble);
      if (targetIdx === -1) return;

      const messagesToFork = [];
      for (let i = 0; i <= targetIdx; i++) {
        const b = allBubbles[i];
        const role = b.dataset.role;
        const text = b.dataset.rawText;
        if ((role === 'user' || role === 'assistant') && text) {
          messagesToFork.push({ role, text });
        }
      }

      if (messagesToFork.length === 0) return;

      saveCurrentChatSession();

      const currentSession = chatSessions.find(s => s.id === currentChatSessionId);
      const baseTitle = currentSession ? currentSession.title : 'Chat';
      const cleanTitle = baseTitle.replace(/^\[Fork\]\s*/, '');
      const forkedTitle = `[Fork] ${cleanTitle}`;

      const newSessionId = 'chat-fork-' + Date.now();
      const newSession = {
        id: newSessionId,
        title: forkedTitle,
        updatedAt: new Date().toISOString(),
        messages: messagesToFork
      };

      chatSessions.unshift(newSession);
      if (chatSessions.length > 30) chatSessions = chatSessions.slice(0, 30);
      localStorage.setItem('sqlfox_ai_sessions', JSON.stringify(chatSessions));

      currentChatSessionId = newSessionId;
      container.innerHTML = '';
      messagesToFork.forEach(m => {
        appendAIMessage(m.role, m.text, false);
      });

      updateChatHistoryCount();
      closeAIChatHistory();
      showNativeAlert(`🔀 Conversation successfully forked to new session: "${forkedTitle}"`, 'Chat Forked');
    }

    async function saveToInternalNotes(title, content, objectTags, userTags, sideNote) {
      // 1. Maintain in-tab scratchpad Notes.md
      let notesTab = tabs.find(t => t.isNote);
      const timestamp = new Date().toLocaleString();
      const formattedNote = `\n\n---\n### ${title} (${timestamp})\n\n${content}\n`;

      if (!notesTab) {
        notesTab = {
          id: 'tab-notes',
          title: 'Notes.md',
          sql: `# SqlFox Internal Notes\nAccumulated query analysis, execution findings, and AI insights.\n` + formattedNote,
          isNote: true,
          isTableViewer: false
        };
        tabs.push(notesTab);
      } else {
        if (activeTabId === notesTab.id) {
          notesTab.sql = getEditorSQL() + formattedNote;
        } else {
          notesTab.sql = (notesTab.sql || '') + formattedNote;
        }
      }

      // 2. Persist to Central Operational Notes in Vault
      let createdNoteId = null;
      try {
        const payload = {
          title: title || 'Quick Capture',
          content: content || '',
          side_note: sideNote || '',
          status: 'inbox',
          object_tags: objectTags || [],
          user_tags: userTags || ['#quick-capture']
        };
        const res = await apiFetch('/api/notes', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        if (res.ok) {
          const created = await res.json();
          createdNoteId = created.id;
          fetchAndRenderInboxNotes();
          refreshCurrentTableNotes();
        }
      } catch (err) {
        console.error('saveToInternalNotes vault error:', err);
      }

      // 3. Show non-blocking interactive toast
      showQuickCaptureToast(title, createdNoteId);
    }

    function showQuickCaptureToast(title, noteId) {
      let container = document.getElementById('reminder-toasts-container');
      if (!container) {
        container = document.createElement('div');
        container.id = 'reminder-toasts-container';
        container.className = 'fixed bottom-8 right-6 z-50 flex flex-col space-y-2 max-w-sm pointer-events-auto';
        document.body.appendChild(container);
      }

      const toast = document.createElement('div');
      toast.className = 'bg-slate-900 border border-app-accent/80 rounded-xl p-3 shadow-2xl text-white text-xs flex items-center justify-between space-x-3 transition ring-2 ring-sky-500/20';
      toast.innerHTML = `
        <div class="flex items-center space-x-2 truncate">
          <span class="shrink-0 text-slate-400">${renderIcon('file-text', 'w-4 h-4 text-sky-400')}</span>
          <span class="truncate">Captured to Notes: <b class="text-sky-300 font-semibold">${escapeHtml(title)}</b></span>
        </div>
        <div class="flex items-center space-x-1.5 shrink-0">
          ${noteId ? `<button onclick="openNoteComposerModal('${noteId}'); this.closest('.shadow-2xl').remove()" class="bg-app-accent hover:bg-sky-400 text-slate-950 font-bold px-2 py-0.5 rounded text-[11px] transition">Edit / Set Alarm</button>` : ''}
          <button onclick="this.closest('.shadow-2xl').remove()" class="text-slate-400 hover:text-white px-1"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
        </div>
      `;
      container.appendChild(toast);
      setTimeout(() => {
        if (toast.parentNode) toast.remove();
      }, 5000);
    }

    function toggleAIMaximize() {
      const sidebar = document.getElementById('ai-sidebar');
      const icon = document.getElementById('ai-maximize-icon');
      if (!sidebar) return;

      isAIMaximized = !isAIMaximized;
      sidebar.classList.add('transition-all', 'duration-200');

      if (isAIMaximized) {
        savedAIWidthBeforeMaximize = sidebar.style.width || (localStorage.getItem('sqlfox_copilot_width') || '380px');
        const leftSidebar = document.getElementById('left-sidebar');
        const leftWidth = (leftSidebar && !leftSidebar.classList.contains('hidden') && leftSidebar.offsetWidth > 0) ? leftSidebar.offsetWidth : 0;
        const maxAllowed = Math.max(320, window.innerWidth - leftWidth - 80);
        const targetWidth = Math.min(maxAllowed, Math.max(480, Math.floor(window.innerWidth * 0.45)));
        sidebar.style.width = targetWidth + 'px';
        sidebar.classList.add('absolute', 'top-0', 'right-0', 'bottom-0', 'z-30', 'shadow-2xl', 'ai-sidebar-overlay');
        sidebar.classList.remove('relative');
        if (icon) icon.innerHTML = renderIcon('minimize-2', 'w-3.5 h-3.5');
      } else {
        const restoreWidth = savedAIWidthBeforeMaximize || localStorage.getItem('sqlfox_copilot_width') || '';
        sidebar.style.width = restoreWidth;
        updateAISidebarMode();
      setTimeout(() => { if (typeof updateTabOverflow === 'function') updateTabOverflow(); }, 150);
        if (icon) icon.innerHTML = renderIcon('maximize-2', 'w-3.5 h-3.5');
      }
    }

    function updateAISidebarMode() {
      const sidebar = document.getElementById('ai-sidebar');
      if (!sidebar || sidebar.classList.contains('hidden')) return;
      const widthNum = parseInt(sidebar.style.width, 10) || sidebar.offsetWidth || 380;
      const leftSidebar = document.getElementById('left-sidebar');
      const leftWidth = (leftSidebar && !leftSidebar.classList.contains('hidden') && leftSidebar.offsetWidth > 0) ? leftSidebar.offsetWidth : 0;
      const availableMain = window.innerWidth - leftWidth - widthNum;
      if (isAIMaximized || widthNum > 420 || availableMain < 500 || window.innerWidth < 1024) {
        sidebar.classList.add('absolute', 'top-0', 'right-0', 'bottom-0', 'z-30', 'shadow-2xl', 'ai-sidebar-overlay');
        sidebar.classList.remove('relative');
      } else {
        sidebar.classList.remove('absolute', 'top-0', 'right-0', 'bottom-0', 'z-30', 'shadow-2xl', 'ai-sidebar-overlay');
        sidebar.classList.add('relative', 'z-10');
      }
    }

    function initAISidebarResizer() {
      const resizer = document.getElementById('ai-sidebar-resizer');
      const sidebar = document.getElementById('ai-sidebar');
      if (!resizer || !sidebar) return;

      try {
        const savedWidth = localStorage.getItem('sqlfox_copilot_width');
        if (savedWidth) {
          sidebar.style.width = savedWidth;
        }
      } catch (_) {}

      updateAISidebarMode();
      setTimeout(() => { if (typeof updateTabOverflow === 'function') updateTabOverflow(); }, 150);
      window.addEventListener('resize', updateAISidebarMode);
      window.addEventListener('resize', () => { if (typeof updateTabOverflow === 'function') requestAnimationFrame(updateTabOverflow); });

      let isDragging = false;
      let startX = 0;
      let startWidth = 0;

      resizer.addEventListener('mousedown', (e) => {
        isDragging = true;
        startX = e.clientX;
        startWidth = sidebar.offsetWidth;
        sidebar.classList.remove('transition-all', 'duration-200');
        document.body.style.cursor = 'col-resize';
        document.body.style.userSelect = 'none';
        e.preventDefault();
      });

      window.addEventListener('mousemove', (e) => {
        if (!isDragging) return;
        const delta = startX - e.clientX;
        const leftSidebar = document.getElementById('left-sidebar');
        const leftWidth = (leftSidebar && !leftSidebar.classList.contains('hidden') && leftSidebar.offsetWidth > 0) ? leftSidebar.offsetWidth : 0;
        const maxAllowed = Math.max(300, window.innerWidth - leftWidth - 80);
        const newWidth = Math.max(300, Math.min(maxAllowed, startWidth + delta));
        sidebar.style.width = newWidth + 'px';

        // ponytail: When widened beyond standard dock width (420px) or when available space in main is cramped (<500px),
        // transition to overlay floating mode so the center workspace is NEVER pushed or distorted.
        const availableMain = window.innerWidth - leftWidth - newWidth;
        if (newWidth > 420 || availableMain < 500) {
          sidebar.classList.add('absolute', 'top-0', 'right-0', 'bottom-0', 'z-30', 'shadow-2xl', 'ai-sidebar-overlay');
          sidebar.classList.remove('relative');
        } else {
          sidebar.classList.remove('absolute', 'top-0', 'right-0', 'bottom-0', 'z-30', 'shadow-2xl', 'ai-sidebar-overlay');
          sidebar.classList.add('relative', 'z-10');
        }

        isAIMaximized = false;
        const icon = document.getElementById('ai-maximize-icon');
        if (icon) icon.innerHTML = renderIcon('maximize-2', 'w-3.5 h-3.5');
      });

      window.addEventListener('mouseup', () => {
        if (!isDragging) return;
        isDragging = false;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        sidebar.classList.add('transition-all', 'duration-200');
        try {
          localStorage.setItem('sqlfox_copilot_width', sidebar.style.width);
        } catch (_) {}
      });

      // ponytail: double click resizer resets sidebar width to default w-[380px] (380px) and clears stored width
      resizer.addEventListener('dblclick', (e) => {
        e.preventDefault();
        sidebar.classList.add('transition-all', 'duration-200');
        sidebar.style.width = '';
        try {
          localStorage.removeItem('sqlfox_copilot_width');
        } catch (_) {}
        isAIMaximized = false;
        savedAIWidthBeforeMaximize = null;
        const icon = document.getElementById('ai-maximize-icon');
        if (icon) icon.innerHTML = renderIcon('maximize-2', 'w-3.5 h-3.5');
        updateAISidebarMode();
        setTimeout(() => { if (typeof updateTabOverflow === 'function') updateTabOverflow(); }, 150);
      });
    }

    // --- Chat Session & History Management ---
    function initAIChatSession() {
      if (!currentChatSessionId) {
        currentChatSessionId = 'chat-' + Date.now();
      }
      initAISidebarResizer();
      updateChatHistoryCount();
    }

    function startNewAIChat() {
      saveCurrentChatSession();
      currentChatSessionId = 'chat-' + Date.now();
      const container = document.getElementById('ai-messages');
      container.innerHTML = `
        <div class="bg-app-card/60 p-2.5 rounded border border-app-border text-slate-300">
          👋 Started a <b>New Chat</b>. Ask me to write SQL, optimize queries, or fix errors.
        </div>
      `;
      closeAIChatHistory();
      updateChatHistoryCount();
    }

    function saveCurrentChatSession() {
      if (!currentChatSessionId) return;
      const container = document.getElementById('ai-messages');
      const msgEls = container.querySelectorAll('.ai-chat-bubble');
      if (msgEls.length === 0) return;

      const messages = [];
      let firstUserPrompt = '';
      msgEls.forEach(el => {
        const role = el.dataset.role || 'system';
        const raw = el.dataset.rawText || el.textContent;
        if (role === 'user' && !firstUserPrompt) {
          firstUserPrompt = raw.slice(0, 35);
        }
        messages.push({ role, text: raw });
      });

      if (messages.length === 0) return;

      const title = firstUserPrompt || 'SQL Assistant Chat';
      const existingIdx = chatSessions.findIndex(s => s.id === currentChatSessionId);
      const sessionObj = {
        id: currentChatSessionId,
        title: title,
        updatedAt: new Date().toISOString(),
        messages: messages
      };

      if (existingIdx >= 0) {
        chatSessions[existingIdx] = sessionObj;
      } else {
        chatSessions.unshift(sessionObj);
      }
      if (chatSessions.length > 30) chatSessions = chatSessions.slice(0, 30);
      localStorage.setItem('sqlfox_ai_sessions', JSON.stringify(chatSessions));
      updateChatHistoryCount();
    }

    function toggleAIChatHistory() {
      const popover = document.getElementById('ai-history-popover');
      if (popover.classList.contains('hidden')) {
        renderAIChatHistoryList();
        popover.classList.remove('hidden');
      } else {
        popover.classList.add('hidden');
      }
    }

    function closeAIChatHistory() {
      const popover = document.getElementById('ai-history-popover');
      if (popover) popover.classList.add('hidden');
    }

    function renderAIChatHistoryList() {
      const listEl = document.getElementById('ai-history-list');
      if (!listEl) return;
      if (!chatSessions || chatSessions.length === 0) {
        listEl.innerHTML = '<div class="text-slate-500 text-center py-4 text-[11px]">No chat history yet.</div>';
        return;
      }

      listEl.innerHTML = chatSessions.map(s => {
        const dateStr = new Date(s.updatedAt).toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
        const isActive = s.id === currentChatSessionId;
        return `
          <div class="flex items-center justify-between p-1.5 rounded hover:bg-slate-800 cursor-pointer ${isActive ? 'bg-slate-800/80 border-l-2 border-app-accent' : ''}" onclick="resumeAIChatSession('${s.id}')">
            <div class="truncate flex-1 pr-2">
              <div class="truncate text-slate-200 font-medium text-[11px]">${escapeHtml(s.title)}</div>
              <div class="text-[9px] text-slate-500">${dateStr} · ${s.messages.length} msgs</div>
            </div>
            <button onclick="event.stopPropagation(); deleteAIChatSession('${s.id}')" class="text-slate-500 hover:text-red-400 px-1 py-0.5 flex items-center" title="Delete chat">${renderIcon('trash-2', 'w-3.5 h-3.5')}</button>
          </div>
        `;
      }).join('');
    }

    function resumeAIChatSession(sessionId) {
      const session = chatSessions.find(s => s.id === sessionId);
      if (!session) return;
      saveCurrentChatSession();
      currentChatSessionId = sessionId;
      const container = document.getElementById('ai-messages');
      container.innerHTML = '';
      session.messages.forEach(m => {
        appendAIMessage(m.role, m.text, false);
      });
      closeAIChatHistory();
    }

    function deleteAIChatSession(sessionId) {
      chatSessions = chatSessions.filter(s => s.id !== sessionId);
      localStorage.setItem('sqlfox_ai_sessions', JSON.stringify(chatSessions));
      if (currentChatSessionId === sessionId) {
        startNewAIChat();
      } else {
        renderAIChatHistoryList();
      }
      updateChatHistoryCount();
    }

    async function clearAllAIHistory() {
      if (!await showNativeConfirm('Clear all AI chat history?', 'Clear Chat History')) return;
      chatSessions = [];
      localStorage.removeItem('sqlfox_ai_sessions');
      startNewAIChat();
    }

    function updateChatHistoryCount() {
      const countEl = document.getElementById('ai-history-count');
      if (countEl) {
        countEl.textContent = `History (${chatSessions.length})`;
      }
    }

    function updateAIProviderBadge() {
      const badge = document.getElementById('ai-provider-badge');
      if (!badge) return;
      if (!aiConfig || !aiConfig.provider) {
        badge.innerHTML = renderIcon('settings', 'w-3 h-3 inline mr-1') + '<span>BYOK (Not Configured)</span>';
        badge.className = 'text-[10px] text-amber-400 hover:underline font-mono cursor-pointer transition';
        return;
      }

      let label = '';
      if (aiConfig.provider === 'openai') {
        const is9router = (aiConfig.endpoint || '').toLowerCase().includes('9router') || (aiConfig.endpoint || '').includes(':8000');
        label = (is9router ? '9router' : 'OpenAI') + (aiConfig.model ? `: ${aiConfig.model}` : '');
      } else if (aiConfig.provider === 'ollama') {
        label = 'Ollama' + (aiConfig.model ? `: ${aiConfig.model}` : '');
      } else if (aiConfig.provider === 'anthropic') {
        label = 'Claude' + (aiConfig.model ? `: ${aiConfig.model}` : '');
      } else if (aiConfig.provider === 'gemini') {
        label = 'Gemini' + (aiConfig.model ? `: ${aiConfig.model}` : '');
      } else {
        label = aiConfig.provider;
      }
      badge.textContent = label;
      badge.className = 'text-[10px] text-sky-400 hover:underline font-mono cursor-pointer transition truncate max-w-[170px]';
    }

    // --- AI Settings (BYOK & Proxy) ---
    function initAISettingsForm() {
      const provEl = document.getElementById('ai-provider');
      if (!provEl) return;
      provEl.value = aiConfig.provider || 'openai';
      document.getElementById('ai-endpoint').value = aiConfig.endpoint || '';
      document.getElementById('ai-model').value = aiConfig.model || '';
      document.getElementById('ai-key').value = aiConfig.apiKey || '';
      document.getElementById('ai-mask-identifiers').checked = aiConfig.maskIdentifiers !== false;
      document.getElementById('ai-language').value = aiConfig.language || 'id';
      document.getElementById('ai-style').value = aiConfig.responseStyle || 'concise';
      document.getElementById('ai-custom-prompt').value = aiConfig.customPrompt || '';
      onAIProviderChange();
    }

    function toggleAISettings() {
      openSettingsModal('ai');
    }

    function closeAISettingsModal() {
      closeVaultModal();
    }

    function onAIProviderChange() {
      const prov = document.getElementById('ai-provider').value;
      const endpointEl = document.getElementById('ai-endpoint');
      const modelEl = document.getElementById('ai-model');
      const keyEl = document.getElementById('ai-key');

      if (prov === 'openai') {
        endpointEl.placeholder = 'http://192.168.x.x:8000/v1 (or https://api.openai.com/v1)';
        if (!modelEl.value) modelEl.placeholder = 'e.g. gpt-4o-mini, deepseek-chat, qwen2.5';
        keyEl.placeholder = 'Optional for 9router / local proxy, sk-... for cloud';
      } else if (prov === 'ollama') {
        endpointEl.placeholder = 'http://localhost:11434';
        if (!modelEl.value) modelEl.placeholder = 'e.g. llama3, qwen2.5-coder, mistral';
        keyEl.placeholder = 'Not required for Ollama';
      } else if (prov === 'anthropic') {
        endpointEl.placeholder = 'https://api.anthropic.com/v1/messages';
        if (!modelEl.value) modelEl.placeholder = 'claude-3-5-sonnet-20241022';
        keyEl.placeholder = 'sk-ant-...';
      } else if (prov === 'gemini') {
        endpointEl.placeholder = 'https://generativelanguage.googleapis.com/v1beta';
        if (!modelEl.value) modelEl.placeholder = 'gemini-1.5-flash';
        keyEl.placeholder = 'AIzaSy...';
      }
    }

    function saveAISettings(e) {
      if (e) e.preventDefault();
      aiConfig = {
        provider: document.getElementById('ai-provider').value,
        endpoint: document.getElementById('ai-endpoint').value.trim(),
        model: document.getElementById('ai-model').value.trim(),
        apiKey: document.getElementById('ai-key').value.trim(),
        maskIdentifiers: document.getElementById('ai-mask-identifiers').checked,
        language: document.getElementById('ai-language').value,
        responseStyle: document.getElementById('ai-style').value,
        customPrompt: document.getElementById('ai-custom-prompt').value.trim()
      };
      localStorage.setItem('sqlfox_ai_config', JSON.stringify(aiConfig));
      updateAIProviderBadge();
      closeVaultModal();
      showNativeAlert('✅ AI Copilot settings saved! Using ' + aiConfig.provider + (aiConfig.endpoint ? ' (' + aiConfig.endpoint + ')' : ''), 'AI Settings');
    }

    async function testAISettings() {
      const prov = document.getElementById('ai-provider').value;
      const ep = document.getElementById('ai-endpoint').value.trim();
      const model = document.getElementById('ai-model').value.trim();
      const key = document.getElementById('ai-key').value.trim();

      const btn = document.getElementById('btn-test-ai') || document.querySelector('button[onclick="testAISettings()"]');
      const origText = btn ? btn.textContent : '';
      if (btn) btn.innerHTML = renderIcon('loader', 'w-3.5 h-3.5 animate-spin mr-1.5') + '<span>Testing...</span>';

      try {
        const res = await apiFetch('/api/ai/chat', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            action: 'chat',
            prompt: 'Say "hello from proxy" in 3 words.',
            provider: prov,
            endpoint: ep,
            model: model,
            api_key: key,
            mask_identifiers: false
          })
        });
        const data = await res.json();
        if (res.ok && !data.error) {
          showNativeAlert('✅ AI Proxy connected successfully!\nResponse: ' + data.content, 'AI Proxy Test');
        } else {
          showNativeAlert('❌ AI Proxy test failed: ' + (data.error || res.statusText), 'AI Proxy Test');
        }
      } catch (err) {
        showNativeAlert('❌ AI Proxy test error: ' + err.message, 'AI Proxy Test');
      } finally {
        if (btn) btn.textContent = origText;
      }
    }

