// --- Operational Notes Inbox, Composer, Reminders & Table Feed Handlers ---
    let allOperationalNotes = [];
    let notesInboxFilter = 'inbox'; // 'inbox', 'due', 'pinned', 'resolved', 'all'
    let notesInboxSearch = '';
    let activeComposerNote = null;
    let notifiedReminderIds = new Set();
    let notesReminderTimer = null;

    // Web Audio Chime Synthesizer (Zero-dependency, native browser audio)
    function playReminderChime() {
      try {
        const AudioCtx = window.AudioContext || window.webkitAudioContext;
        if (!AudioCtx) return;
        const ctx = new AudioCtx();
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.type = 'sine';
        osc.frequency.setValueAtTime(587.33, ctx.currentTime); // D5
        osc.frequency.setValueAtTime(880, ctx.currentTime + 0.12); // A5
        gain.gain.setValueAtTime(0.18, ctx.currentTime);
        gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.7);
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.start();
        osc.stop(ctx.currentTime + 0.7);
      } catch (e) {
        // audio blocked by browser policy until user gesture
      }
    }

    function toggleNotesDrawer(force) {
      const drawer = document.getElementById('notes-inbox-drawer');
      if (!drawer) return;
      const isHidden = drawer.classList.contains('hidden');
      const show = force !== undefined ? force : isHidden;
      if (show) {
        drawer.classList.remove('hidden');
        fetchAndRenderInboxNotes();
      } else {
        drawer.classList.add('hidden');
      }
    }

    async function fetchAndRenderInboxNotes() {
      try {
        const res = await apiFetch('/api/notes');
        if (!res.ok) throw new Error('Failed to fetch notes');
        allOperationalNotes = await res.json();
        if (!Array.isArray(allOperationalNotes)) allOperationalNotes = [];
      } catch (err) {
        allOperationalNotes = [];
      }
      updateNotesBadgeAndCounts();
      renderNotesInboxList();
      if (typeof updateCloudSyncHeaderBadge === 'function') {
        updateCloudSyncHeaderBadge();
      }
    }

    function updateNotesBadgeAndCounts() {
      const totalEl = document.getElementById('notes-inbox-total');
      const badgeEl = document.getElementById('notes-inbox-badge');
      const duePillCount = document.getElementById('notes-due-pill-count');

      const now = new Date();
      const dueNotes = allOperationalNotes.filter(n => n.status !== 'resolved' && n.reminder_at && new Date(n.reminder_at) <= now);
      const totalCount = allOperationalNotes.length;

      if (totalEl) totalEl.textContent = `(${totalCount} notes)`;
      if (badgeEl) {
        if (dueNotes.length > 0) {
          badgeEl.textContent = dueNotes.length;
          badgeEl.classList.remove('hidden');
        } else {
          badgeEl.classList.add('hidden');
        }
      }
      if (duePillCount) {
        if (dueNotes.length > 0) {
          duePillCount.textContent = dueNotes.length;
          duePillCount.classList.remove('hidden');
        } else {
          duePillCount.classList.add('hidden');
        }
      }
    }

    function setNotesFilter(filter) {
      notesInboxFilter = filter;
      ['inbox', 'due', 'pinned', 'resolved', 'all'].forEach(f => {
        const btn = document.getElementById(`notes-filter-${f}`);
        if (!btn) return;
        if (f === filter) {
          btn.className = 'px-2.5 py-0.5 rounded-full font-medium bg-app-accent text-slate-950 transition';
        } else {
          btn.className = 'px-2.5 py-0.5 rounded-full font-medium bg-slate-800 text-slate-300 hover:text-white transition';
        }
      });
      renderNotesInboxList();
    }

    function onNotesSearchInput(val) {
      notesInboxSearch = (val || '').toLowerCase().trim();
      const clearBtn = document.getElementById('notes-search-clear-btn');
      if (clearBtn) clearBtn.classList.toggle('hidden', !notesInboxSearch);
      renderNotesInboxList();
    }

    function clearNotesSearch() {
      const input = document.getElementById('notes-search-input');
      if (input) input.value = '';
      notesInboxSearch = '';
      const clearBtn = document.getElementById('notes-search-clear-btn');
      if (clearBtn) clearBtn.classList.add('hidden');
      renderNotesInboxList();
    }

    function renderNotesInboxList() {
      const container = document.getElementById('notes-cards-container');
      if (!container) return;

      const now = new Date();
      let filtered = allOperationalNotes.slice();

      // Apply tab filter
      if (notesInboxFilter === 'inbox') {
        filtered = filtered.filter(n => n.status === 'inbox');
      } else if (notesInboxFilter === 'due') {
        filtered = filtered.filter(n => n.status !== 'resolved' && n.reminder_at && new Date(n.reminder_at) <= now);
      } else if (notesInboxFilter === 'pinned') {
        filtered = filtered.filter(n => n.is_pinned);
      } else if (notesInboxFilter === 'resolved') {
        filtered = filtered.filter(n => n.status === 'resolved');
      }

      // Apply search filter
      if (notesInboxSearch) {
        filtered = filtered.filter(n => {
          const t = (n.title || '').toLowerCase();
          const c = (n.content || '').toLowerCase();
          const s = (n.side_note || '').toLowerCase();
          const obj = (n.object_tags || []).join(' ').toLowerCase();
          const u = (n.user_tags || []).join(' ').toLowerCase();
          return t.includes(notesInboxSearch) || c.includes(notesInboxSearch) || s.includes(notesInboxSearch) || obj.includes(notesInboxSearch) || u.includes(notesInboxSearch);
        });
      }

      if (filtered.length === 0) {
        container.innerHTML = `
          <div class="p-10 text-center text-slate-500 italic text-xs">
            No notes found in this view. Click <b>+ New Note</b> to capture queries, findings, or follow-ups.
          </div>
        `;
        return;
      }

      container.innerHTML = filtered.map(n => renderNoteCardHTML(n, now)).join('');
    }

    function renderNoteSnippetPreview(content) {
      if (!content || !content.trim()) return '';
      let text = content.trim();
      // ponytail: if note content is pure SQL statement without markdown fences, wrap in sql block for interactive preview
      const firstWord = (text.split(/\s+/)[0] || '').toUpperCase();
      const sqlKeywords = ['SELECT', 'INSERT', 'UPDATE', 'DELETE', 'CREATE', 'ALTER', 'DROP', 'TRUNCATE', 'EXPLAIN', 'WITH', 'SHOW', 'DESCRIBE'];
      if (!text.includes('```') && !text.includes('# ') && sqlKeywords.includes(firstWord)) {
        text = '```sql\n' + text + '\n```';
      }
      const snippet = text.length > 360 ? text.substring(0, 360) + '...' : text;
      return renderCopilotMarkdown(snippet);
    }

    function renderNoteCardHTML(n, now) {
      if (!now) now = new Date();
      const isDue = n.status !== 'resolved' && n.reminder_at && new Date(n.reminder_at) <= now;
      const isUpcoming = n.status !== 'resolved' && n.reminder_at && new Date(n.reminder_at) > now;

      let reminderBadge = '';
      if (isDue) {
        reminderBadge = `
          <div class="inline-flex items-center space-x-1 px-2 py-0.5 rounded text-[10px] font-bold bg-rose-500/20 text-rose-300 border border-rose-500/40 animate-pulse">
            <span class="flex items-center space-x-1">${renderIcon('alert-triangle', 'w-3 h-3 text-rose-400')}<span>Overdue: ${new Date(n.reminder_at).toLocaleString()}</span></span>
            <button onclick="event.stopPropagation(); snoozeOperationalNote('${n.id}', 10)" class="ml-1 px-1 rounded bg-rose-900/60 hover:bg-rose-800 text-white flex items-center space-x-0.5" title="Snooze 10m">${renderIcon('clock', 'w-2.5 h-2.5')}<span>+10m</span></button>
            <button onclick="event.stopPropagation(); resolveOperationalNote('${n.id}')" class="ml-0.5 px-1 rounded bg-emerald-900/60 hover:bg-emerald-800 text-white flex items-center" title="Mark Done">${renderIcon('check', 'w-2.5 h-2.5 text-emerald-300')}</button>
          </div>
        `;
      } else if (isUpcoming) {
        reminderBadge = `
          <div class="inline-flex items-center space-x-1 px-2 py-0.5 rounded text-[10px] bg-sky-950/50 text-sky-300 border border-sky-800/50 font-mono">
            <span class="flex items-center space-x-1">${renderIcon('clock', 'w-3 h-3 text-sky-400')}<span>${new Date(n.reminder_at).toLocaleString()}</span></span>
          </div>
        `;
      }

      const statusBadge = n.status === 'resolved'
        ? `<span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] bg-emerald-950 text-emerald-400 border border-emerald-800/60 font-medium">${renderIcon('check', 'w-3 h-3 text-emerald-400')}<span>Done</span></span>`
        : `<span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-[10px] bg-slate-800 text-slate-400 font-medium">${renderIcon('download', 'w-3 h-3 text-slate-400')}<span>Inbox</span></span>`;

      const pinIcon = n.is_pinned ? renderIcon('pin', 'w-3.5 h-3.5 text-amber-400') : renderIcon('pin', 'w-3.5 h-3.5 text-slate-500');
      const pinClass = n.is_pinned ? 'text-amber-400 font-bold' : 'text-slate-500 hover:text-slate-300';

      const objTagsHtml = (n.object_tags || []).map(tag => `
        <span onclick="event.stopPropagation(); filterNotesByTag(${jsArg(tag)})" class="inline-flex items-center space-x-1 px-1.5 py-0.2 rounded text-[10px] font-mono bg-sky-950/70 text-sky-300 border border-sky-800/40 hover:border-sky-500 cursor-pointer transition truncate max-w-[220px]" title="Filter notes by this object">
          ${renderIcon('bookmark', 'w-3 h-3 text-sky-400 shrink-0')}<span>${escapeHtml(tag.replace(/^db:\/\//, ''))}</span>
        </span>
      `).join(' ');

      const userTagsHtml = (n.user_tags || []).map(tag => `
        <span class="inline-flex items-center px-1.5 py-0.2 rounded text-[10px] bg-slate-800 text-slate-300 border border-slate-700 font-mono">
          ${escapeHtml(tag)}
        </span>
      `).join(' ');

      let sideNoteHtml = '';
      if (n.side_note && n.side_note.trim()) {
        sideNoteHtml = `
          <div class="mt-2 p-2 bg-amber-950/20 border-l-2 border-amber-500 rounded text-amber-200 text-xs font-mono leading-relaxed">
            <span class="text-[10px] uppercase font-bold text-amber-400 block mb-0.5">💡 Developer Note:</span>
            ${escapeHtml(n.side_note)}
          </div>
        `;
      }

      let contentSnippet = '';
      if (n.content && n.content.trim()) {
        const previewHtml = renderNoteSnippetPreview(n.content);
        contentSnippet = `
          <div onclick="event.stopPropagation()" class="mt-2 p-2.5 bg-slate-950 border border-slate-800/80 rounded text-slate-200 text-xs leading-relaxed font-sans overflow-y-auto max-h-36 select-text note-preview-content">
            ${previewHtml}
          </div>
        `;
      }

      const capturedStr = n.captured_at ? new Date(n.captured_at).toLocaleString() : '';

      return `
        <div class="bg-app-card hover:bg-slate-900 border ${n.is_pinned ? 'border-amber-500/50 shadow-sm shadow-amber-950/30' : 'border-app-border'} rounded-lg p-3 transition flex flex-col space-y-1.5 cursor-pointer select-text" onclick="openNoteComposerModal('${n.id}')">
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-center space-x-2 min-w-0">
              <button onclick="event.stopPropagation(); togglePinOperationalNote('${n.id}', ${!n.is_pinned})" class="text-xs ${pinClass} transition" title="${n.is_pinned ? 'Unpin note' : 'Pin note to top'}">${pinIcon}</button>
              <h4 class="font-bold text-white text-xs truncate hover:text-app-accent">${escapeHtml(n.title || 'Untitled Note')}</h4>
              ${statusBadge}
            </div>
            <div class="flex items-center space-x-1 shrink-0">
              ${reminderBadge}
            </div>
          </div>

          <div class="flex items-center space-x-2 text-[10px] text-slate-400">
            <span>Captured: ${capturedStr}</span>
          </div>

          ${(objTagsHtml || userTagsHtml) ? `<div class="flex flex-wrap items-center gap-1 mt-1">${objTagsHtml} ${userTagsHtml}</div>` : ''}
          ${sideNoteHtml}
          ${contentSnippet}

          <div class="flex items-center justify-between pt-1.5 border-t border-slate-800/80 text-[11px] text-slate-400 mt-1">
            <div class="flex items-center space-x-2">
              <button onclick="event.stopPropagation(); openNoteInQueryTab('${n.id}')" class="text-sky-400 hover:text-sky-300 flex items-center space-x-1 font-medium" title="Extract SQL and open in Query Editor">
                ${renderIcon('file-code', 'w-3 h-3 text-sky-400')}<span>Open in Query</span>
              </button>
              <button onclick="event.stopPropagation(); openNoteComposerModal('${n.id}')" class="hover:text-white flex items-center space-x-1">
                ${renderIcon('edit', 'w-3 h-3')}<span>Edit</span>
              </button>
              <button onclick="event.stopPropagation(); copyNoteMarkdown('${n.id}')" class="hover:text-white flex items-center space-x-1">
                ${renderIcon('copy', 'w-3 h-3')}<span>Copy MD</span>
              </button>
              <button onclick="event.stopPropagation(); exportNoteMarkdown('${n.id}')" class="hover:text-white flex items-center space-x-1">
                ${renderIcon('download', 'w-3 h-3')}<span>Export</span>
              </button>
            </div>
            <div class="flex items-center space-x-2">
              ${n.status !== 'resolved' ? `<button onclick="event.stopPropagation(); resolveOperationalNote('${n.id}')" class="text-emerald-400 hover:text-emerald-300 font-medium flex items-center space-x-1">${renderIcon('check', 'w-3 h-3 text-emerald-400')}<span>Mark Done</span></button>` : ''}
              <button onclick="event.stopPropagation(); deleteOperationalNoteCard('${n.id}')" class="text-slate-500 hover:text-rose-400 flex items-center p-0.5" title="Delete Note Card">${renderIcon('trash-2', 'w-3.5 h-3.5')}</button>
            </div>
          </div>
        </div>
      `;
    }

    function filterNotesByTag(tag) {
      const input = document.getElementById('notes-search-input');
      if (input) input.value = tag;
      onNotesSearchInput(tag);
    }

    // --- Note Composer / Editor Modal Handlers ---
    let currentComposerSnapshotTab = 'edit';

    function openNoteComposerModal(noteId, defaults) {
      const dlg = document.getElementById('note-composer-dialog');
      if (!dlg) return;

      activeComposerNote = null;
      document.getElementById('nc-modal-title').textContent = noteId ? 'Edit Operational Note' : 'New Operational Note';
      document.getElementById('nc-btn-delete').classList.toggle('hidden', !noteId);

      // Default fields
      let title = '';
      let status = 'inbox';
      let isPinned = false;
      let objectTags = [];
      let userTags = [];
      let reminderAt = '';
      let sideNote = '';
      let content = '';

      if (noteId) {
        const found = allOperationalNotes.find(n => n.id === noteId);
        if (found) {
          activeComposerNote = found;
          title = found.title || '';
          status = found.status || 'inbox';
          isPinned = !!found.is_pinned;
          objectTags = found.object_tags || [];
          userTags = found.user_tags || [];
          reminderAt = found.reminder_at ? formatDatetimeLocal(found.reminder_at) : '';
          sideNote = found.side_note || '';
          content = found.content || '';
        }
      } else if (defaults) {
        if (defaults.title) title = defaults.title;
        if (defaults.status) status = defaults.status;
        if (defaults.is_pinned !== undefined) isPinned = defaults.is_pinned;
        if (defaults.object_tags) objectTags = defaults.object_tags;
        if (defaults.user_tags) userTags = defaults.user_tags;
        if (defaults.side_note) sideNote = defaults.side_note;
        if (defaults.content) content = defaults.content;
        if (defaults.reminder_at) reminderAt = formatDatetimeLocal(defaults.reminder_at);
      }

      document.getElementById('nc-title').value = title;
      document.getElementById('nc-status').value = status;
      document.getElementById('nc-pinned').checked = isPinned;
      document.getElementById('nc-object-tags').value = objectTags.join(', ');
      document.getElementById('nc-user-tags').value = userTags.join(', ');
      document.getElementById('nc-reminder').value = reminderAt;
      document.getElementById('nc-sidenote').value = sideNote;
      document.getElementById('nc-content').value = content;

      setComposerSnapshotTab('edit');

      if (typeof dlg.showModal === 'function') dlg.showModal();
      else dlg.setAttribute('open', '');
    }

    function closeNoteComposerModal() {
      const dlg = document.getElementById('note-composer-dialog');
      if (dlg) {
        if (typeof dlg.close === 'function') dlg.close();
        else dlg.removeAttribute('open');
      }
      activeComposerNote = null;
    }

    function formatDatetimeLocal(dateInput) {
      if (!dateInput) return '';
      const d = new Date(dateInput);
      if (isNaN(d.getTime())) return '';
      const pad = (n) => String(n).padStart(2, '0');
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    }

    function setComposerSnapshotTab(tab) {
      currentComposerSnapshotTab = tab;
      const btnEdit = document.getElementById('nc-tab-edit');
      const btnPrev = document.getElementById('nc-tab-preview');
      const textarea = document.getElementById('nc-content');
      const preview = document.getElementById('nc-content-preview');

      if (tab === 'preview') {
        if (btnEdit) btnEdit.className = 'px-2 py-0.5 rounded text-[10px] text-slate-400 hover:text-white';
        if (btnPrev) btnPrev.className = 'px-2 py-0.5 rounded text-[10px] font-semibold bg-slate-800 text-white';
        if (textarea) textarea.classList.add('hidden');
        if (preview) {
          preview.classList.remove('hidden');
          preview.innerHTML = renderCopilotMarkdown(textarea ? textarea.value : '');
        }
      } else {
        if (btnEdit) btnEdit.className = 'px-2 py-0.5 rounded text-[10px] font-semibold bg-slate-800 text-white';
        if (btnPrev) btnPrev.className = 'px-2 py-0.5 rounded text-[10px] text-slate-400 hover:text-white';
        if (preview) preview.classList.add('hidden');
        if (textarea) textarea.classList.remove('hidden');
      }
    }

    function setNoteReminderPreset(preset) {
      const input = document.getElementById('nc-reminder');
      if (!input) return;
      const now = new Date();
      if (typeof preset === 'number') {
        now.setMinutes(now.getMinutes() + preset);
        input.value = formatDatetimeLocal(now);
      } else if (preset === 'tomorrow') {
        now.setDate(now.getDate() + 1);
        now.setHours(9, 0, 0, 0);
        input.value = formatDatetimeLocal(now);
      }
    }

    function clearNoteReminderPreset() {
      const input = document.getElementById('nc-reminder');
      if (input) input.value = '';
    }

    function attachActiveTableToNoteComposer() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeConnection || !activeTab || !activeTab.tableName) {
        showNativeAlert('No active table is currently selected in tabs.', 'Tag Active Table');
        return;
      }
      const db = activeTab.database || (activeConnection && activeConnection.database_name) || 'db';
      const tag = `db://${activeConnection.id}/${db}/${activeTab.tableName}`;
      const input = document.getElementById('nc-object-tags');
      if (input) {
        const existing = input.value.split(',').map(s => s.trim()).filter(Boolean);
        if (!existing.includes(tag)) {
          existing.push(tag);
          input.value = existing.join(', ');
        }
      }
    }

    async function saveOperationalNoteFromComposer() {
      const title = (document.getElementById('nc-title').value || '').trim() || 'Untitled Note';
      const status = document.getElementById('nc-status').value || 'inbox';
      const isPinned = document.getElementById('nc-pinned').checked;
      const objTagsRaw = document.getElementById('nc-object-tags').value || '';
      const userTagsRaw = document.getElementById('nc-user-tags').value || '';
      const reminderRaw = document.getElementById('nc-reminder').value;
      const sideNote = document.getElementById('nc-sidenote').value || '';
      const content = document.getElementById('nc-content').value || '';

      const objectTags = objTagsRaw.split(',').map(s => s.trim()).filter(Boolean);
      const userTags = userTagsRaw.split(',').map(s => s.trim()).filter(Boolean);

      let reminderAt = null;
      if (reminderRaw) {
        const parsed = new Date(reminderRaw);
        if (!isNaN(parsed.getTime())) reminderAt = parsed.toISOString();
      }

      const payload = {
        title,
        status,
        is_pinned: isPinned,
        object_tags: objectTags,
        user_tags: userTags,
        reminder_at: reminderAt,
        side_note: sideNote,
        content
      };

      try {
        let res;
        if (activeComposerNote && activeComposerNote.id) {
          res = await apiFetch(`/api/notes/${activeComposerNote.id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
        } else {
          res = await apiFetch('/api/notes', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
        }

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || res.statusText);
        }

        closeNoteComposerModal();
        await fetchAndRenderInboxNotes();

        // If table viewer notes mode is active, refresh table feed too
        refreshCurrentTableNotes();

        showNativeAlert('📝 Note saved successfully!', 'Notes Inbox');
      } catch (err) {
        showNativeAlert('Failed to save note: ' + err.message, 'Notes Error');
      }
    }

    async function deleteActiveComposerNote() {
      if (!activeComposerNote || !activeComposerNote.id) return;
      deleteOperationalNoteCard(activeComposerNote.id);
      closeNoteComposerModal();
    }

    async function deleteOperationalNoteCard(id) {
      const confirmDelete = await showNativeConfirm('Are you sure you want to delete this note?', 'Delete Note');
      if (!confirmDelete) return;

      try {
        const res = await apiFetch(`/api/notes/${id}`, { method: 'DELETE' });
        if (!res.ok) throw new Error('Failed to delete note');
        await fetchAndRenderInboxNotes();
        refreshCurrentTableNotes();
      } catch (err) {
        showNativeAlert('Failed to delete note: ' + err.message, 'Delete Error');
      }
    }

    async function togglePinOperationalNote(id, newPinned) {
      const note = allOperationalNotes.find(n => n.id === id);
      if (!note) return;
      try {
        const res = await apiFetch(`/api/notes/${id}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...note, is_pinned: newPinned })
        });
        if (res.ok) {
          await fetchAndRenderInboxNotes();
          refreshCurrentTableNotes();
        }
      } catch (err) {
        console.error('togglePin error:', err);
      }
    }

    async function resolveOperationalNote(id) {
      try {
        const res = await apiFetch(`/api/notes/${id}/resolve`, { method: 'POST' });
        if (res.ok) {
          await fetchAndRenderInboxNotes();
          refreshCurrentTableNotes();
          showNativeAlert('✅ Note marked as resolved!', 'Notes Follow-up');
        }
      } catch (err) {
        console.error('resolveNote error:', err);
      }
    }

    async function snoozeOperationalNote(id, minutes) {
      try {
        const res = await apiFetch(`/api/notes/${id}/snooze`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ minutes: minutes || 10 })
        });
        if (res.ok) {
          await fetchAndRenderInboxNotes();
          showNativeAlert(`💤 Reminder snoozed for ${minutes || 10} minutes.`, 'Reminder Snoozed');
        }
      } catch (err) {
        console.error('snoozeNote error:', err);
      }
    }

    function formatNoteAsMarkdown(n) {
      const timeStr = n.captured_at ? new Date(n.captured_at).toLocaleString() : new Date().toLocaleString();
      let md = `# ${n.title || 'Untitled Note'}\n`;
      md += `*Captured at: ${timeStr}*\n`;
      if (n.object_tags && n.object_tags.length > 0) {
        md += `*Objects: ${n.object_tags.join(', ')}*\n`;
      }
      if (n.user_tags && n.user_tags.length > 0) {
        md += `*Tags: ${n.user_tags.join(', ')}*\n`;
      }
      if (n.side_note && n.side_note.trim()) {
        md += `\n### Developer Notes & Action Items\n${n.side_note}\n`;
      }
      if (n.content && n.content.trim()) {
        md += `\n### Snapshot Content\n${n.content}\n`;
      }
      return md;
    }

    async function copyNoteMarkdown(noteId) {
      const n = allOperationalNotes.find(note => note.id === noteId);
      if (!n) return;
      const md = formatNoteAsMarkdown(n);
      const ok = await copyTextToClipboard(md);
      if (ok) showNativeAlert('📋 Markdown copied to clipboard!', 'Copy Note');
      else showNativeAlert('Failed to copy markdown to clipboard.', 'Clipboard Error');
    }

    function exportNoteMarkdown(noteId) {
      const n = allOperationalNotes.find(note => note.id === noteId);
      if (!n) return;
      const md = formatNoteAsMarkdown(n);
      const filename = `${(n.title || 'note').toLowerCase().replace(/[^a-z0-9_-]/g, '_')}.md`;
      downloadFile(md, filename, 'text/markdown');
    }

    function copyComposerMarkdown() {
      const title = document.getElementById('nc-title').value || 'Note';
      const sideNote = document.getElementById('nc-sidenote').value || '';
      const content = document.getElementById('nc-content').value || '';
      const objTags = document.getElementById('nc-object-tags').value || '';
      const md = `# ${title}\n${objTags ? '*Objects: ' + objTags + '*\n' : ''}\n${sideNote ? '### Developer Notes\n' + sideNote + '\n' : ''}\n### Snapshot Content\n${content}`;
      copyTextToClipboard(md).then(ok => {
        if (ok) showNativeAlert('📋 Markdown copied to clipboard!', 'Copy Note');
      });
    }

    function exportComposerNoteMd() {
      const title = document.getElementById('nc-title').value || 'note';
      const sideNote = document.getElementById('nc-sidenote').value || '';
      const content = document.getElementById('nc-content').value || '';
      const objTags = document.getElementById('nc-object-tags').value || '';
      const md = `# ${title}\n${objTags ? '*Objects: ' + objTags + '*\n' : ''}\n${sideNote ? '### Developer Notes\n' + sideNote + '\n' : ''}\n### Snapshot Content\n${content}`;
      const filename = `${title.toLowerCase().replace(/[^a-z0-9_-]/g, '_')}.md`;
      downloadFile(md, filename, 'text/markdown');
    }

    // --- Query Snapshot Capture & Note Query Extraction (ADR-0044) ---
    // ponytail: capture active/selected query into Operational Note; extract markdown blocks back to query tab.
    function saveActiveQueryToNotes() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      saveTabQueryToNotes(activeTab);
    }

    function saveTabQueryToNotes(tab) {
      if (!tab || tab.isNote) {
        showNativeAlert('Cannot save an internal note scratchpad into another operational note.', 'Save Note');
        return;
      }

      let sql = '';
      if (tab.id === activeTabId && cmEditor) {
        sql = (cmEditor.getSelection() || '').trim();
        if (!sql) sql = (cmEditor.getValue() || '').trim();
      } else {
        sql = (tab.sql || '').trim();
      }

      if (!sql) {
        showNativeAlert('The query tab is empty. Please enter or execute a SQL statement first.', 'Empty Query');
        return;
      }

      const cleanTitle = (tab.title || 'Query Snapshot').replace(/\.sql$/i, '').trim() || 'Query Snapshot';
      const objTags = [];
      if (activeConnection && activeConnection.id) {
        const db = tab.database || (activeConnection.database_name) || '';
        if (tab.tableName && db) {
          objTags.push(`db://${activeConnection.id}/${db}/${tab.tableName}`);
        } else if (db) {
          objTags.push(`db://${activeConnection.id}/${db}`);
        } else {
          objTags.push(`db://${activeConnection.id}`);
        }
      }

      saveToInternalNotes(cleanTitle, '```sql\n' + sql + '\n```', objTags, ['#query-snapshot'], '');
    }

    function extractSqlFromNoteContent(rawContent) {
      if (!rawContent || !rawContent.trim()) return '';
      const str = rawContent.trim();
      const sqlFenceRegex = /```(?:sql)?\s*([\s\S]*?)```/gi;
      const blocks = [];
      let match;
      while ((match = sqlFenceRegex.exec(str)) !== null) {
        const code = (match[1] || '').trim();
        if (code) blocks.push(code);
      }
      if (blocks.length > 0) {
        return blocks.join('\n\n');
      }
      return str;
    }

    function openExtractedQueryInTab(title, rawContent, objectTags) {
      const sql = extractSqlFromNoteContent(rawContent);
      if (!sql) {
        showNativeAlert('This note does not contain any query or snapshot content to open.', 'Empty Content');
        return;
      }

      let targetDb = '';
      if (Array.isArray(objectTags)) {
        for (const tag of objectTags) {
          if (typeof tag === 'string' && tag.startsWith('db://')) {
            const parts = tag.slice(5).split('/');
            if (parts[1]) {
              targetDb = parts[1];
              break;
            }
          }
        }
      }
      if (!targetDb && activeTab && activeTab.database) {
        targetDb = activeTab.database;
      } else if (!targetDb && activeConnection && activeConnection.database_name) {
        targetDb = activeConnection.database_name;
      }

      const safeBase = (title || 'query').trim().replace(/[/\\?%*:|"<>]/g, '-').replace(/\.sql$/i, '') || 'query';
      const tabTitle = `${safeBase}.sql`;

      const curTab = tabs.find(t => t.id === activeTabId);
      const canReuse = curTab && !curTab.isTableViewer && !curTab.isProcesslist && !curTab.isCluster &&
                       !curTab.isFlightRecorder && !curTab.isVariables && !curTab.isNote && !isTabDirtyOrNonEmpty(curTab);

      if (canReuse) {
        curTab.title = tabTitle;
        curTab.sql = sql;
        if (targetDb) curTab.database = targetDb;
        setEditorSQL(sql);
        renderTabs();
        saveTabsState();
      } else {
        const newId = 'tab-' + Date.now();
        tabs.push({
          id: newId,
          title: tabTitle,
          sql: sql,
          database: targetDb,
          isTableViewer: false
        });
        switchTab(newId);
        saveTabsState();
      }

      const statusEl = document.getElementById('query-status-text');
      if (statusEl) {
        statusEl.textContent = `Opened: ${tabTitle}`;
        setTimeout(() => {
          if (statusEl.textContent === `Opened: ${tabTitle}`) statusEl.textContent = 'Ready';
        }, 4000);
      }
    }

    function openNoteInQueryTab(noteId) {
      const n = (allOperationalNotes || []).find(note => note.id === noteId);
      if (!n) return;
      toggleNotesDrawer(false);
      openExtractedQueryInTab(n.title, n.content, n.object_tags);
    }

    function openCurrentComposerInQueryTab() {
      const content = document.getElementById('nc-content')?.value || '';
      const title = document.getElementById('nc-title')?.value || 'Operational Note';
      const rawObjTags = document.getElementById('nc-object-tags')?.value || '';
      const objectTags = rawObjTags ? rawObjTags.split(',').map(s => s.trim()).filter(Boolean) : (activeComposerNote?.object_tags || []);
      closeNoteComposerModal();
      openExtractedQueryInTab(title, content, objectTags);
    }

    // --- Table Viewer Filtered Feed Handlers ---
    let currentTableFeedTableName = '';

    async function loadAndRenderTableNote(tableName) {
      if (!activeConnection || !tableName) return;
      currentTableFeedTableName = tableName;

      const activeTab = tabs.find(t => t.id === activeTabId);
      const dbName = (activeTab && activeTab.database) || (schemaCatalogCache && schemaCatalogCache.database_name) || (activeConnection && activeConnection.database_name) || '';
      const tableTag = `db://${activeConnection.id}/${dbName}/${tableName}`;

      const tagBadge = document.getElementById('table-notes-tag-badge');
      if (tagBadge) tagBadge.textContent = tableTag;

      const feedContainer = document.getElementById('table-notes-feed-container');
      if (feedContainer) feedContainer.innerHTML = '<div class="p-8 text-center text-slate-500 italic text-xs">Loading notes for this table...</div>';

      try {
        const res = await apiFetch(`/api/notes?object_tag=${encodeURIComponent(tableTag)}`);
        if (!res.ok) throw new Error('Failed to load table notes feed');
        const notes = await res.json();
        renderTableNotesFeed(notes, tableTag, tableName);
      } catch (err) {
        if (feedContainer) feedContainer.innerHTML = `<div class="p-6 text-center text-rose-400 text-xs">Error loading notes: ${escapeHtml(err.message)}</div>`;
      }
    }

    function renderTableNotesFeed(notes, tableTag, tableName) {
      const feedContainer = document.getElementById('table-notes-feed-container');
      if (!feedContainer) return;

      if (!notes || notes.length === 0) {
        feedContainer.innerHTML = `
          <div class="p-8 text-center bg-slate-900/40 border border-slate-800 rounded-xl space-y-3">
            <div class="flex justify-center">${renderIcon('file-text', 'w-10 h-10 text-slate-500')}</div>
            <h4 class="font-bold text-white text-sm">No notes or runbooks for table "${escapeHtml(tableName)}"</h4>
            <p class="text-slate-400 text-xs max-w-md mx-auto">
              Document institutional business logic, query runbooks, or track incidents for this table.
              All notes are stored in your local Vault and automatically injected into AI Copilot prompts.
            </p>
            <button onclick="createNewNoteForCurrentTable()" class="bg-app-accent hover:bg-sky-400 text-slate-950 font-bold px-3 py-1.5 rounded text-xs transition inline-flex items-center space-x-1.5 shadow">
              ${renderIcon('plus', 'w-3.5 h-3.5 text-slate-950')}<span>Create Runbook for "${escapeHtml(tableName)}"</span>
            </button>
          </div>
        `;
        return;
      }

      // Separate pinned runbooks from regular incident notes
      const pinned = notes.filter(n => n.is_pinned);
      const incidents = notes.filter(n => !n.is_pinned);

      let html = '';

      if (pinned.length > 0) {
        html += `
          <div class="mb-4 space-y-2">
            <div class="flex items-center space-x-2 text-amber-400 font-bold text-xs uppercase tracking-wider">
              ${renderIcon('pin', 'w-3.5 h-3.5 text-amber-400')}<span>Table Runbook &amp; Business Rules</span>
            </div>
            ${pinned.map(n => `
              <div class="bg-slate-900/90 border border-amber-500/50 rounded-lg p-4 space-y-2.5 shadow-md">
                <div class="flex items-start justify-between">
                  <div>
                    <h4 class="text-sm font-bold text-white">${escapeHtml(n.title)}</h4>
                    <span class="text-[10px] text-slate-400 font-mono">Updated: ${new Date(n.updated_at).toLocaleString()}</span>
                  </div>
                  <div class="flex items-center space-x-2">
                    <button onclick="openNoteInQueryTab('${n.id}')" class="bg-sky-950/70 hover:bg-sky-900 border border-sky-800/60 text-sky-300 px-2.5 py-1 rounded text-xs transition flex items-center space-x-1" title="Extract SQL and open in Query Editor">
                      ${renderIcon('file-code', 'w-3.5 h-3.5 text-sky-300')}<span>Open in Query</span>
                    </button>
                    <button onclick="openNoteComposerModal('${n.id}')" class="bg-slate-800 hover:bg-slate-700 text-slate-200 px-2.5 py-1 rounded text-xs transition flex items-center space-x-1">
                      ${renderIcon('edit', 'w-3.5 h-3.5 text-slate-200')}<span>Edit Runbook</span>
                    </button>
                    <button onclick="exportNoteMarkdown('${n.id}')" class="bg-slate-800 hover:bg-slate-700 text-slate-300 p-1.5 rounded text-xs transition flex items-center" title="Export .md">
                      ${renderIcon('download', 'w-3.5 h-3.5 text-slate-300')}
                    </button>
                  </div>
                </div>
                ${n.side_note ? `
                  <div class="p-2 bg-amber-950/30 border-l-2 border-amber-400 rounded text-amber-200 text-xs font-mono">
                    <span class="font-bold block text-[10px] text-amber-400">Rules & Gotchas:</span>
                    ${escapeHtml(n.side_note)}
                  </div>
                ` : ''}
                <div class="p-3 bg-slate-950 rounded border border-slate-800 text-slate-200 text-xs leading-relaxed overflow-y-auto max-h-72">
                  ${renderCopilotMarkdown(n.content || '')}
                </div>
              </div>
            `).join('')}
          </div>
        `;
      }

      if (incidents.length > 0) {
        html += `
          <div class="space-y-2">
            <div class="flex items-center space-x-2 text-slate-400 font-bold text-xs uppercase tracking-wider">
              ${renderIcon('alert-triangle', 'w-3.5 h-3.5 text-amber-400')}<span>Related Incidents &amp; Query Findings (${incidents.length})</span>
            </div>
            ${incidents.map(n => renderNoteCardHTML(n, new Date())).join('')}
          </div>
        `;
      }

      feedContainer.innerHTML = html;
    }

    function createNewNoteForCurrentTable() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeConnection || !activeTab || !activeTab.tableName) return;
      const db = activeTab.database || (activeConnection && activeConnection.database_name) || 'db';
      const tag = `db://${activeConnection.id}/${db}/${activeTab.tableName}`;
      openNoteComposerModal(null, {
        title: `Runbook: ${activeTab.tableName}`,
        object_tags: [tag],
        user_tags: ['#runbook'],
        is_pinned: true,
        content: `# Table Rules: ${activeTab.tableName}\n\n- Business constraints:\n- Query gotchas:`
      });
    }

    function refreshCurrentTableNotes() {
      if (currentTableFeedTableName) {
        loadAndRenderTableNote(currentTableFeedTableName);
      }
    }

    // --- Background Follow-up Reminder Engine ---
    async function checkDueNotesReminders() {
      try {
        const res = await apiFetch('/api/notes/reminders/due');
        if (!res.ok) return;
        const dueNotes = await res.json();
        if (!Array.isArray(dueNotes)) return;

        const badgeEl = document.getElementById('notes-inbox-badge');
        if (badgeEl) {
          if (dueNotes.length > 0) {
            badgeEl.textContent = dueNotes.length;
            badgeEl.classList.remove('hidden');
          } else {
            badgeEl.classList.add('hidden');
          }
        }

        // Trigger notification and sound chime for newly due notes
        dueNotes.forEach(note => {
          if (!notifiedReminderIds.has(note.id)) {
            notifiedReminderIds.add(note.id);
            playReminderChime();
            showInteractiveReminderToast(note);
          }
        });
      } catch (err) {
        // quiet background poller
      }
    }

    function showInteractiveReminderToast(note) {
      // Create non-blocking bottom-right reminder toast
      let container = document.getElementById('reminder-toasts-container');
      if (!container) {
        container = document.createElement('div');
        container.id = 'reminder-toasts-container';
        container.className = 'fixed bottom-8 right-6 z-50 flex flex-col space-y-2 max-w-sm pointer-events-auto';
        document.body.appendChild(container);
      }

      const toast = document.createElement('div');
      toast.className = 'bg-slate-900 border border-rose-500/80 rounded-xl p-3.5 shadow-2xl text-white text-xs flex flex-col space-y-2 animate-bounce-short ring-2 ring-rose-500/30';
      toast.innerHTML = `
        <div class="flex items-start justify-between">
          <div class="flex items-center space-x-2">
            <span class="text-base">🚨</span>
            <span class="font-bold text-rose-300">Follow-up Reminder Due</span>
          </div>
          <button onclick="this.closest('.animate-bounce-short').remove()" class="text-slate-400 hover:text-white text-xs px-1"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
        </div>
        <p class="font-semibold text-slate-100 truncate">${escapeHtml(note.title)}</p>
        ${note.side_note ? `<p class="text-[11px] text-amber-200 font-mono line-clamp-2">${escapeHtml(note.side_note)}</p>` : ''}
        <div class="flex items-center justify-between pt-1 border-t border-slate-800 text-[11px]">
          <button onclick="openNoteComposerModal('${note.id}'); this.closest('.animate-bounce-short').remove()" class="bg-app-accent hover:bg-sky-400 text-slate-950 font-bold px-2 py-0.5 rounded transition">Open Note</button>
          <div class="flex items-center space-x-1.5">
            <button onclick="snoozeOperationalNote('${note.id}', 10); this.closest('.animate-bounce-short').remove()" class="bg-slate-800 hover:bg-slate-700 text-slate-300 px-2 py-0.5 rounded transition">Snooze 10m</button>
            <button onclick="resolveOperationalNote('${note.id}'); this.closest('.animate-bounce-short').remove()" class="bg-emerald-900/70 hover:bg-emerald-800 text-emerald-200 px-2 py-0.5 rounded transition">Done</button>
          </div>
        </div>
      `;
      container.appendChild(toast);
    }

    // Start background timer: check reminders every 30s
    function startNotesReminderPoller() {
      if (notesReminderTimer) clearInterval(notesReminderTimer);
      notesReminderTimer = setInterval(checkDueNotesReminders, 30000);
      setTimeout(checkDueNotesReminders, 3000);
    }

