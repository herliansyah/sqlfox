<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { renderFullMarkdownHtml } from '../../utils/markdown';
  import Icon from '../common/Icon.svelte';

  let title = $state('');
  let sideNote = $state('');
  let content = $state('');
  let category = $state('General');
  let tableName = $state('');
  let userTags = $state<string[]>([]);
  let objectTags = $state<string[]>([]);
  let userTagInput = $state('');
  let objectTagInput = $state('');
  let reminderAt = $state('');
  let isPinned = $state(false);
  let editorTab = $state<'write' | 'preview'>('write');
  let isSaving = $state(false);

  let editingNote = $derived(appState.editingNote);
  let isExistingNote = $derived(Boolean(editingNote?.id));

  const categories = ['General', 'Incident', 'Optimization', 'Schema', 'Task', 'Runbook'];

  function pad(n: number) {
    return String(n).padStart(2, '0');
  }

  function toLocalDatetimeInput(d: Date): string {
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function setQuickMinutes(mins: number) {
    reminderAt = toLocalDatetimeInput(new Date(Date.now() + mins * 60 * 1000));
  }

  function setTomorrow9AM() {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    d.setHours(9, 0, 0, 0);
    reminderAt = toLocalDatetimeInput(d);
  }

  function addUserTag() {
    const raw = userTagInput.trim().replace(/^#/, '');
    if (!raw) return;
    const tag = `#${raw}`;
    if (!userTags.includes(tag)) userTags = [...userTags, tag];
    userTagInput = '';
  }

  function addObjectTag() {
    const raw = objectTagInput.trim();
    if (!raw) return;
    if (!objectTags.includes(raw)) objectTags = [...objectTags, raw];
    objectTagInput = '';
  }

  function tagActiveTable() {
    const connId = appState.activeConnection?.id || 'conn';
    const db = appState.activeTab?.database || appState.currentDatabase || 'db';
    const tbl = appState.activeTab?.tableName || tableName.trim();
    if (!tbl) {
      appState.showToast('No active table found to link', 'warning');
      return;
    }
    const uri = `db://${connId}/${db}/${tbl}`;
    if (!objectTags.includes(uri)) objectTags = [...objectTags, uri];
    if (!tableName.trim()) tableName = tbl;
  }

  function captureActiveQuery() {
    const q = (appState.activeTab?.sql || '').trim();
    if (!q) {
      appState.showToast('Active tab has no SQL query', 'warning');
      return;
    }
    const block = `\n\`\`\`sql\n${q}\n\`\`\`\n`;
    content = content ? `${content.trimEnd()}\n${block}` : block.trimStart();
    appState.showToast('SQL query appended into note Markdown!', 'info');
  }

  let wasOpen = false;
  $effect(() => {
    if (appState.isNoteComposerModalOpen) {
      if (!wasOpen) {
        editorTab = 'write';
        if (editingNote) {
          title = editingNote.title || '';
          sideNote = editingNote.side_note || '';
          content = editingNote.content || '';
          category = editingNote.category || 'General';
          tableName = editingNote.table_name || '';
          userTags = Array.isArray(editingNote.user_tags) ? [...editingNote.user_tags] : [];
          objectTags = Array.isArray(editingNote.object_tags) ? [...editingNote.object_tags] : [];
          if (editingNote.reminder_at) {
            const d = new Date(editingNote.reminder_at);
            reminderAt = isNaN(d.getTime()) ? '' : toLocalDatetimeInput(d);
          } else {
            reminderAt = '';
          }
          isPinned = !!editingNote.is_pinned;
        } else {
          title = '';
          sideNote = '';
          content = '';
          category = 'General';
          const tbl = appState.activeTab?.tableName || '';
          tableName = tbl;
          userTags = [];
          objectTags = tbl && appState.activeConnection?.id
            ? [`db://${appState.activeConnection.id}/${appState.activeTab?.database || appState.currentDatabase || 'db'}/${tbl}`]
            : tbl ? [`db://table/${tbl}`] : [];
          reminderAt = '';
          isPinned = false;
        }
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  async function saveNote() {
    if (!title.trim()) {
      appState.showToast('Note title is required', 'warning');
      return;
    }

    isSaving = true;
    try {
      const mergedUserTags = [...userTags];
      const catTag = category.trim() ? `#${category.trim()}` : '';
      if (catTag && !mergedUserTags.some((t) => t.toLowerCase() === catTag.toLowerCase())) {
        mergedUserTags.unshift(catTag);
      }

      const payload: any = {
        ...(editingNote || {}),
        title: title.trim(),
        side_note: sideNote.trim(),
        content: content.trim(),
        status: editingNote?.status || (editingNote?.is_resolved ? 'resolved' : 'inbox'),
        category: category.trim() || undefined,
        table_name: tableName.trim() || undefined,
        user_tags: mergedUserTags,
        object_tags: objectTags,
        reminder_at: reminderAt ? new Date(reminderAt).toISOString() : null,
        is_pinned: isPinned,
      };

      if (isExistingNote) {
        await api.updateNote(editingNote.id, payload);
        appState.showToast('Note updated successfully!', 'success');
      } else {
        await api.createNote(payload);
        appState.showToast('Note created successfully!', 'success');
      }

      appState.notesRevision++;
      appState.checkDueNotesReminders();
      appState.isNoteComposerModalOpen = false;
      appState.editingNote = null;
    } catch (err: any) {
      appState.showToast(`Failed to save note: ${err.message}`, 'error');
    } finally {
      isSaving = false;
    }
  }

  async function handleDelete() {
    if (!isExistingNote) return;
    const ok = await appState.showNativeConfirm(
      `Permanently delete note "${editingNote.title}"?`,
      'Delete Note'
    );
    if (!ok) return;
    isSaving = true;
    try {
      await api.deleteNote(editingNote.id);
      appState.notesRevision++;
      appState.checkDueNotesReminders();
      appState.isNoteComposerModalOpen = false;
      appState.editingNote = null;
      appState.showToast('Note deleted', 'info');
    } catch (err: any) {
      appState.showToast(`Failed to delete note: ${err.message}`, 'error');
    } finally {
      isSaving = false;
    }
  }
</script>

{#if appState.isNoteComposerModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-2xl max-h-[90vh] shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a] shrink-0">
        <div class="flex items-center space-x-2">
          <Icon name="file-text" class="w-4 h-4 text-amber-400" />
          <h3 class="font-bold text-slate-100 text-sm">
            {isExistingNote ? 'Edit Operational Note' : 'Create Operational Note'}
          </h3>
        </div>
        <button type="button" onclick={() => (appState.isNoteComposerModalOpen = false)} class="text-slate-400 hover:text-white" aria-label="Close">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-3.5 overflow-y-auto flex-1">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label for="note-title" class="block text-slate-400 text-xs font-medium mb-1">Title *</label>
            <input
              id="note-title"
              type="text"
              bind:value={title}
              placeholder="e.g. Migration follow-up, Index required on orders"
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-amber-500"
            />
          </div>
          <div>
            <label for="note-side-note" class="block text-slate-400 text-xs font-medium mb-1">Subtitle / Side Note</label>
            <input
              id="note-side-note"
              type="text"
              bind:value={sideNote}
              placeholder="Short summary or runbook subtitle..."
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-amber-500"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label for="note-category" class="block text-slate-400 text-xs font-medium mb-1">Primary Category</label>
            <select
              id="note-category"
              bind:value={category}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-amber-500"
            >
              {#each categories as cat}
                <option value={cat}>{cat}</option>
              {/each}
            </select>
          </div>

          <div>
            <div class="flex items-center justify-between mb-1">
              <label for="note-linked-table" class="text-slate-400 text-xs font-medium">Linked Table</label>
              <button type="button" onclick={tagActiveTable} class="text-[10px] text-sky-400 hover:underline">
                + Tag Active Table
              </button>
            </div>
            <input
              id="note-linked-table"
              type="text"
              bind:value={tableName}
              placeholder="e.g. orders, users"
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-amber-500"
            />
          </div>
        </div>

        <!-- Markdown Editor + Live Preview -->
        <div>
          <div class="flex items-center justify-between mb-1.5">
            <div class="flex items-center space-x-1 bg-[#050811] p-0.5 rounded-lg border border-[#1e293b]">
              <button
                type="button"
                onclick={() => (editorTab = 'write')}
                class="px-2.5 py-0.5 rounded text-[11px] font-semibold transition {editorTab === 'write' ? 'bg-slate-800 text-amber-300' : 'text-slate-400 hover:text-white'}"
              >
                Write Markdown
              </button>
              <button
                type="button"
                onclick={() => (editorTab = 'preview')}
                class="px-2.5 py-0.5 rounded text-[11px] font-semibold transition flex items-center space-x-1 {editorTab === 'preview' ? 'bg-slate-800 text-sky-300' : 'text-slate-400 hover:text-white'}"
              >
                <Icon name="eye" class="w-3 h-3" />
                <span>Preview</span>
              </button>
            </div>

            <button
              type="button"
              onclick={captureActiveQuery}
              class="text-[11px] text-sky-400 hover:underline flex items-center space-x-1"
            >
              <Icon name="camera" class="w-3 h-3" />
              <span>Insert Active SQL</span>
            </button>
          </div>

          {#if editorTab === 'write'}
            <textarea
              bind:value={content}
              placeholder="Add detailed operational context, findings, fenced ```sql blocks, checklists (- [ ])..."
              rows="6"
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg p-3 text-slate-200 text-xs focus:outline-none focus:border-amber-500 font-mono leading-relaxed resize-y"
            ></textarea>
          {:else}
            <div class="min-h-[144px] max-h-60 overflow-y-auto bg-[#050811] border border-[#1e293b] rounded-lg p-3 text-xs text-slate-300">
              {@html renderFullMarkdownHtml(content || '*Nothing written yet.*')}
            </div>
          {/if}
        </div>

        <!-- Multi-tag User & Object Tags -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="p-2.5 rounded-lg bg-[#050811] border border-[#1e293b] space-y-1.5">
            <span class="text-[10.5px] font-semibold text-slate-400 block">User Tags (#tag)</span>
            <div class="flex flex-wrap gap-1">
              {#each userTags as ut, idx}
                <span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded bg-slate-800 text-slate-200 text-[10px]">
                  <span>{ut}</span>
                  <button type="button" onclick={() => (userTags = userTags.filter((_, i) => i !== idx))} class="text-slate-400 hover:text-rose-400">×</button>
                </span>
              {/each}
            </div>
            <div class="flex space-x-1">
              <input
                type="text"
                bind:value={userTagInput}
                onkeydown={(e) => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); addUserTag(); } }}
                placeholder="#tag + Enter"
                class="flex-1 bg-slate-900 border border-slate-800 rounded px-2 py-1 text-[11px] text-slate-200 outline-none focus:border-amber-500"
              />
              <button type="button" onclick={addUserTag} class="px-2 py-1 rounded bg-slate-800 text-slate-300 text-[10px] hover:bg-slate-700">+</button>
            </div>
          </div>

          <div class="p-2.5 rounded-lg bg-[#050811] border border-[#1e293b] space-y-1.5">
            <span class="text-[10.5px] font-semibold text-slate-400 block">Object Scope Tags (db://...)</span>
            <div class="flex flex-wrap gap-1">
              {#each objectTags as ot, idx}
                <span class="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded bg-sky-950/70 border border-sky-800 text-sky-300 font-mono text-[10px] max-w-full">
                  <span class="truncate max-w-[180px]" title={ot}>{ot}</span>
                  <button type="button" onclick={() => (objectTags = objectTags.filter((_, i) => i !== idx))} class="text-sky-400 hover:text-rose-400">×</button>
                </span>
              {/each}
            </div>
            <div class="flex space-x-1">
              <input
                type="text"
                bind:value={objectTagInput}
                onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addObjectTag(); } }}
                placeholder="db://conn/db/table"
                class="flex-1 bg-slate-900 border border-slate-800 rounded px-2 py-1 text-[11px] text-slate-200 font-mono outline-none focus:border-sky-500"
              />
              <button type="button" onclick={addObjectTag} class="px-2 py-1 rounded bg-slate-800 text-slate-300 text-[10px] hover:bg-slate-700">+</button>
            </div>
          </div>
        </div>

        <!-- Reminder Due Date -->
        <div>
          <div class="flex items-center justify-between mb-1 flex-wrap gap-1">
            <label for="note-reminder-at" class="text-slate-400 text-xs font-medium">Follow-up Reminder Date &amp; Time</label>
            <div class="flex items-center space-x-1">
              <button type="button" onclick={() => setQuickMinutes(15)} class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-slate-400 hover:text-amber-300">+15m</button>
              <button type="button" onclick={() => setQuickMinutes(60)} class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-slate-400 hover:text-amber-300">+1h</button>
              <button type="button" onclick={setTomorrow9AM} class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-slate-400 hover:text-amber-300">Tomorrow 9 AM</button>
              {#if reminderAt}
                <button type="button" onclick={() => (reminderAt = '')} class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-rose-400 hover:bg-rose-950/60">Clear</button>
              {/if}
            </div>
          </div>
          <input
            id="note-reminder-at"
            type="datetime-local"
            bind:value={reminderAt}
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-amber-500"
          />
        </div>

        <label class="flex items-center space-x-2 p-2 rounded-lg bg-slate-900/50 border border-slate-800 cursor-pointer">
          <input type="checkbox" bind:checked={isPinned} class="rounded border-slate-700 text-amber-500" />
          <span class="text-slate-300 text-xs">Pin this note to top of inbox</span>
        </label>
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a] shrink-0">
        <div>
          {#if isExistingNote}
            <button
              type="button"
              onclick={handleDelete}
              disabled={isSaving}
              class="px-3 py-1.5 rounded bg-rose-950/60 hover:bg-rose-900 border border-rose-800 text-rose-300 text-xs font-medium transition flex items-center space-x-1"
            >
              <Icon name="trash-2" class="w-3.5 h-3.5" />
              <span>Delete</span>
            </button>
          {/if}
        </div>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={() => (appState.isNoteComposerModalOpen = false)}
            disabled={isSaving}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Cancel
          </button>
          <button
            type="button"
            onclick={saveNote}
            disabled={isSaving || !title.trim()}
            class="px-4 py-1.5 rounded bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white text-xs font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-amber-900/30"
          >
            <Icon name="check" class="w-3.5 h-3.5 {isSaving ? 'animate-spin' : ''}" />
            <span>{isSaving ? 'Saving...' : 'Save Note'}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
