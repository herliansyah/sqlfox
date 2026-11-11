<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let title = $state('');
  let content = $state('');
  let category = $state('General');
  let tableName = $state('');
  let querySnapshot = $state('');
  let reminderAt = $state('');
  let isPinned = $state(false);
  let isSaving = $state(false);

  let editingNote = $derived(appState.editingNote);

  const categories = ['General', 'Incident', 'Optimization', 'Schema', 'Task', 'Runbook'];

  function setQuickReminder(hours: number) {
    const d = new Date(Date.now() + hours * 3600 * 1000);
    // Format YYYY-MM-DDTHH:mm
    const pad = (n: number) => String(n).padStart(2, '0');
    reminderAt = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function captureQuerySnapshot() {
    const q = appState.activeTab?.query || '';
    if (q.trim()) {
      querySnapshot = q.trim();
      appState.showToast('Current worksheet query captured into note snapshot!', 'info');
    } else {
      appState.showToast('No active query found in current tab', 'warning');
    }
  }

  $effect(() => {
    if (appState.isNoteComposerModalOpen) {
      if (editingNote) {
        title = editingNote.title || '';
        content = editingNote.content || '';
        category = editingNote.category || 'General';
        tableName = editingNote.table_name || '';
        querySnapshot = editingNote.query_snapshot || '';
        reminderAt = editingNote.reminder_at ? editingNote.reminder_at.slice(0, 16) : '';
        isPinned = !!editingNote.is_pinned;
      } else {
        title = '';
        content = '';
        category = 'General';
        tableName = appState.activeTab?.tableName || '';
        querySnapshot = '';
        reminderAt = '';
        isPinned = false;
      }
    }
  });

  async function saveNote() {
    if (!title.trim()) {
      appState.showToast('Note title is required', 'warning');
      return;
    }

    isSaving = true;
    try {
      const payload = {
        title: title.trim(),
        content: content.trim(),
        category: category.trim() || undefined,
        table_name: tableName.trim() || undefined,
        query_snapshot: querySnapshot.trim() || undefined,
        reminder_at: reminderAt ? new Date(reminderAt).toISOString() : undefined,
        is_pinned: isPinned,
      };

      if (editingNote && editingNote.id) {
        await api.updateNote(editingNote.id, payload);
        appState.showToast('Note updated successfully!', 'success');
      } else {
        await api.createNote(payload);
        appState.showToast('Note created successfully!', 'success');
      }

      appState.isNoteComposerModalOpen = false;
      appState.editingNote = null;
    } catch (err: any) {
      appState.showToast(`Failed to save note: ${err.message}`, 'error');
    } finally {
      isSaving = false;
    }
  }
</script>

{#if appState.isNoteComposerModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="file-text" class="w-4 h-4 text-amber-400" />
          <h3 class="font-bold text-slate-100 text-sm">
            {editingNote ? 'Edit Operational Note' : 'Create Operational Note'}
          </h3>
        </div>
        <button onclick={() => (appState.isNoteComposerModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-3.5">
        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Title *</label>
          <input
            type="text"
            bind:value={title}
            placeholder="e.g. Migration follow-up, Index required on orders"
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-amber-500"
          />
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Category / Tag</label>
            <select
              bind:value={category}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-amber-500"
            >
              {#each categories as cat}
                <option value={cat}>{cat}</option>
              {/each}
            </select>
          </div>

          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Linked Table</label>
            <input
              type="text"
              bind:value={tableName}
              placeholder="e.g. orders, users"
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-amber-500"
            />
          </div>
        </div>

        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="text-slate-400 text-xs font-medium">Content / Markdown Details</label>
            <span class="text-slate-500 text-[10px]">Markdown supported</span>
          </div>
          <textarea
            bind:value={content}
            placeholder="Add detailed operational context, query findings, team reminders..."
            rows="4"
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg p-3 text-slate-200 text-xs focus:outline-none focus:border-amber-500 font-sans resize-none"
          ></textarea>
        </div>

        <!-- Query Snapshot Capture -->
        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="text-slate-400 text-xs font-medium">Attached Query Snapshot</label>
            <button
              type="button"
              onclick={captureQuerySnapshot}
              class="text-[10px] text-sky-400 hover:text-sky-300 flex items-center space-x-1"
            >
              <Icon name="camera" class="w-3 h-3" />
              <span>Capture Active Query</span>
            </button>
          </div>
          <input
            type="text"
            bind:value={querySnapshot}
            placeholder="Paste or capture SQL query..."
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-sky-300 font-mono text-xs focus:outline-none focus:border-amber-500"
          />
        </div>

        <!-- Reminder Due Date -->
        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="text-slate-400 text-xs font-medium">Follow-up Reminder Date & Time</label>
            <div class="flex items-center space-x-1">
              <button
                type="button"
                onclick={() => setQuickReminder(1)}
                class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-slate-400 hover:text-amber-300"
              >
                +1h
              </button>
              <button
                type="button"
                onclick={() => setQuickReminder(24)}
                class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-slate-400 hover:text-amber-300"
              >
                +1d
              </button>
              <button
                type="button"
                onclick={() => setQuickReminder(168)}
                class="px-1.5 py-0.5 rounded bg-slate-900 border border-slate-800 text-[10px] text-slate-400 hover:text-amber-300"
              >
                +1w
              </button>
            </div>
          </div>
          <input
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

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-end space-x-2 bg-[#0f172a]">
        <button
          onclick={() => (appState.isNoteComposerModalOpen = false)}
          disabled={isSaving}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
        >
          Cancel
        </button>
        <button
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
{/if}
