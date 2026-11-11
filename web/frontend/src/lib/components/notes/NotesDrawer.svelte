<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';
  import NoteMarkdownView from './NoteMarkdownView.svelte';

  interface OperationalNote {
    id: string;
    title: string;
    content: string;
    category?: string;
    table_name?: string;
    query_snapshot?: string;
    reminder_at?: string;
    is_pinned?: boolean;
    is_resolved?: boolean;
    created_at?: string;
    updated_at?: string;
  }

  let notes = $state<OperationalNote[]>([]);
  let filter = $state<'all' | 'inbox' | 'due' | 'pinned' | 'resolved'>('inbox');
  let searchQuery = $state('');
  let filterByCurrentTable = $state(false);
  let isLoading = $state(false);

  let activeTableName = $derived(appState.activeTab?.tableName || '');

  // Web Audio reminder chime
  function playReminderChime() {
    try {
      const AudioCtx = window.AudioContext || (window as any).webkitAudioContext;
      if (!AudioCtx) return;
      const ctx = new AudioCtx();
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = 'sine';
      osc.frequency.setValueAtTime(587.33, ctx.currentTime);
      osc.frequency.setValueAtTime(880, ctx.currentTime + 0.12);
      gain.gain.setValueAtTime(0.18, ctx.currentTime);
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.7);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start();
      osc.stop(ctx.currentTime + 0.7);
    } catch (e) {}
  }

  async function loadNotes() {
    isLoading = true;
    try {
      const data = await api.listNotes();
      notes = Array.isArray(data) ? data : [];
    } catch (err: any) {
      notes = [];
    } finally {
      isLoading = false;
    }
  }

  let filteredNotes = $derived.by(() => {
    let list = notes;

    if (filter === 'inbox') {
      list = list.filter((n) => !n.is_resolved);
    } else if (filter === 'due') {
      const now = new Date();
      list = list.filter((n) => n.reminder_at && new Date(n.reminder_at) <= now && !n.is_resolved);
    } else if (filter === 'pinned') {
      list = list.filter((n) => n.is_pinned);
    } else if (filter === 'resolved') {
      list = list.filter((n) => n.is_resolved);
    }

    if (filterByCurrentTable && activeTableName) {
      list = list.filter(
        (n) => n.table_name && n.table_name.toLowerCase() === activeTableName.toLowerCase()
      );
    }

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      list = list.filter(
        (n) =>
          n.title.toLowerCase().includes(q) ||
          n.content.toLowerCase().includes(q) ||
          (n.category && n.category.toLowerCase().includes(q))
      );
    }

    // Sort: pinned first, then updated_at desc
    return list.slice().sort((a, b) => {
      if (a.is_pinned && !b.is_pinned) return -1;
      if (!a.is_pinned && b.is_pinned) return 1;
      const dateA = new Date(a.updated_at || a.created_at || 0).getTime();
      const dateB = new Date(b.updated_at || b.created_at || 0).getTime();
      return dateB - dateA;
    });
  });

  async function togglePin(note: OperationalNote) {
    try {
      const updated = { ...note, is_pinned: !note.is_pinned };
      await api.updateNote(note.id, updated);
      notes = notes.map((n) => (n.id === note.id ? updated : n));
    } catch (e: any) {
      appState.showToast(`Failed to update note: ${e.message}`, 'error');
    }
  }

  async function toggleResolve(note: OperationalNote) {
    try {
      if (!note.is_resolved) {
        await api.resolveNote(note.id);
        notes = notes.map((n) => (n.id === note.id ? { ...n, is_resolved: true } : n));
        appState.showToast('Note marked as resolved', 'info');
      } else {
        const updated = { ...note, is_resolved: false };
        await api.updateNote(note.id, updated);
        notes = notes.map((n) => (n.id === note.id ? updated : n));
      }
    } catch (e: any) {
      appState.showToast(`Failed to update note: ${e.message}`, 'error');
    }
  }

  async function snoozeNote(note: OperationalNote, minutes: number) {
    try {
      await api.snoozeNote(note.id, minutes);
      appState.showToast(`Reminder snoozed by ${minutes} minutes`, 'info');
      await loadNotes();
    } catch (e: any) {
      appState.showToast(`Failed to snooze: ${e.message}`, 'error');
    }
  }

  async function deleteNote(id: string) {
    if (!confirm('Are you sure you want to delete this note?')) return;
    try {
      await api.deleteNote(id);
      notes = notes.filter((n) => n.id !== id);
      appState.showToast('Note deleted', 'info');
    } catch (e: any) {
      appState.showToast(`Failed to delete note: ${e.message}`, 'error');
    }
  }

  function openComposer(noteToEdit?: OperationalNote) {
    appState.editingNote = noteToEdit || null;
    appState.isNoteComposerModalOpen = true;
  }

  function openQueryInWorksheet(sql?: string) {
    if (!sql) return;
    appState.addTab('Note Query.sql', sql);
    appState.showToast('Query opened in new worksheet tab', 'info');
  }

  $effect(() => {
    if (appState.isNotesDrawerOpen) {
      loadNotes();
    }
  });
</script>

<aside
  class="fixed top-12 right-0 bottom-7 w-96 bg-[#0b0f19] border-l border-[#1e293b] shadow-2xl z-40 flex flex-col font-sans transition-transform duration-200 select-text"
>
  <!-- Drawer Header -->
  <div class="h-10 bg-[#0f172a] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0 select-none">
    <div class="flex items-center space-x-2">
      <Icon name="file-text" class="w-4 h-4 text-amber-400" />
      <span class="font-bold text-slate-100 text-xs">Operational Notes</span>
      <span class="text-slate-500 font-mono text-[10px]">({filteredNotes.length})</span>
    </div>

    <div class="flex items-center space-x-1">
      <button
        onclick={() => openComposer()}
        class="px-2 py-0.5 rounded bg-amber-600 hover:bg-amber-500 text-white font-medium text-xs transition flex items-center space-x-1 shadow-sm"
      >
        <Icon name="plus" class="w-3 h-3" />
        <span>New Note</span>
      </button>
      <button
        onclick={() => (appState.isNotesDrawerOpen = false)}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition ml-1"
        title="Close Drawer"
      >
        <Icon name="x" class="w-3.5 h-3.5" />
      </button>
    </div>
  </div>

  <!-- Search & Filters -->
  <div class="p-2 border-b border-[#1e293b] bg-[#070b14] space-y-2 flex-shrink-0 select-none">
    <div class="relative">
      <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-2" />
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Search notes by title, content, tag..."
        class="w-full bg-[#0b0f19] border border-[#1e293b] rounded-lg pl-8 pr-3 py-1.5 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-amber-500"
      />
    </div>

    <div class="flex items-center justify-between">
      <div class="flex items-center space-x-1">
        {#each ['inbox', 'due', 'pinned', 'resolved', 'all'] as f}
          <button
            onclick={() => (filter = f as any)}
            class="px-2 py-0.5 rounded-full capitalize text-[10px] font-medium transition {filter === f
              ? 'bg-amber-500/20 text-amber-300 border border-amber-500/40'
              : 'text-slate-400 hover:text-white hover:bg-slate-800'}"
          >
            {f}
          </button>
        {/each}
      </div>

      {#if activeTableName}
        <button
          onclick={() => (filterByCurrentTable = !filterByCurrentTable)}
          class="text-[10px] px-2 py-0.5 rounded border transition flex items-center space-x-1 {filterByCurrentTable
            ? 'bg-sky-950 border-sky-600 text-sky-300'
            : 'border-slate-800 text-slate-500 hover:text-slate-300'}"
        >
          <span>@{activeTableName}</span>
        </button>
      {/if}
    </div>
  </div>

  <!-- Notes List Body -->
  <div class="flex-1 overflow-y-auto p-3 space-y-2.5">
    {#if isLoading}
      <div class="p-8 text-center text-slate-500 flex flex-col items-center justify-center space-y-2">
        <Icon name="loader" class="w-5 h-5 animate-spin text-amber-400" />
        <span class="text-xs">Loading operational notes...</span>
      </div>
    {:else if filteredNotes.length === 0}
      <div class="p-8 text-center text-slate-500 flex flex-col items-center justify-center space-y-2">
        <Icon name="file-text" class="w-8 h-8 text-slate-600" />
        <span class="text-xs font-medium text-slate-400">No notes in this view</span>
        <button
          onclick={() => openComposer()}
          class="text-[11px] text-amber-400 hover:underline"
        >
          + Create your first note
        </button>
      </div>
    {:else}
      {#each filteredNotes as note}
        <div
          class="p-3 rounded-xl border bg-[#0e1424] transition shadow-md hover:border-slate-700 flex flex-col space-y-2 {note.is_pinned
            ? 'border-amber-500/50 shadow-amber-950/20'
            : note.is_resolved
            ? 'border-[#1e293b] opacity-60'
            : 'border-[#1e293b]'}"
        >
          <div class="flex items-start justify-between">
            <div class="flex items-center space-x-1.5 flex-1 min-w-0 pr-2">
              <button
                onclick={() => toggleResolve(note)}
                class="text-slate-500 hover:text-emerald-400 shrink-0"
                title={note.is_resolved ? 'Reopen note' : 'Mark resolved'}
              >
                <Icon name={note.is_resolved ? 'check-circle' : 'circle'} class="w-3.5 h-3.5 {note.is_resolved ? 'text-emerald-400' : ''}" />
              </button>
              <h4 class="font-bold text-slate-200 text-xs truncate {note.is_resolved ? 'line-through text-slate-500' : ''}">
                {note.title}
              </h4>
            </div>

            <div class="flex items-center space-x-1 shrink-0">
              <button
                onclick={() => togglePin(note)}
                class="p-1 rounded text-slate-500 hover:text-amber-400 transition"
                title={note.is_pinned ? 'Unpin' : 'Pin note'}
              >
                <Icon name="star" class="w-3.5 h-3.5 {note.is_pinned ? 'text-amber-400 fill-amber-400' : ''}" />
              </button>
              <button
                onclick={() => openComposer(note)}
                class="p-1 rounded text-slate-500 hover:text-sky-300 transition"
                title="Edit note"
              >
                <Icon name="edit" class="w-3.5 h-3.5" />
              </button>
              <button
                onclick={() => deleteNote(note.id)}
                class="p-1 rounded text-slate-500 hover:text-rose-400 transition"
                title="Delete note"
              >
                <Icon name="trash-2" class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>

          <div class="py-1">
            <NoteMarkdownView content={note.content} onOpenQuery={openQueryInWorksheet} />
          </div>

          {#if note.query_snapshot}
            <div class="p-2 rounded bg-[#070a12] border border-sky-950 flex items-center justify-between text-[11px] font-mono text-sky-300">
              <span class="truncate flex-1 mr-2">{note.query_snapshot}</span>
              <button
                onclick={() => openQueryInWorksheet(note.query_snapshot)}
                class="text-[10px] text-sky-400 hover:underline shrink-0 flex items-center space-x-1"
              >
                <Icon name="play" class="w-2.5 h-2.5" />
                <span>Open</span>
              </button>
            </div>
          {/if}

          <!-- Metadata Badges & Due Reminder -->
          <div class="flex items-center justify-between pt-1 text-[10px] text-slate-500 select-none">
            <div class="flex items-center space-x-1.5 flex-wrap">
              {#if note.category}
                <span class="px-1.5 py-0.2 rounded bg-slate-800 text-slate-400">
                  #{note.category}
                </span>
              {/if}
              {#if note.table_name}
                <span class="px-1.5 py-0.2 rounded bg-sky-950 text-sky-300 border border-sky-800 font-mono">
                  {note.table_name}
                </span>
              {/if}
            </div>

            {#if note.reminder_at}
              {@const isDue = new Date(note.reminder_at) <= new Date()}
              <div class="flex items-center space-x-1 {isDue ? 'text-rose-400 font-semibold' : 'text-slate-400'}">
                <Icon name="clock" class="w-3 h-3" />
                <span>{new Date(note.reminder_at).toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>
                {#if isDue}
                  <button
                    onclick={() => snoozeNote(note, 15)}
                    class="ml-1 text-[9px] px-1 py-0.2 rounded bg-rose-950 border border-rose-800 text-rose-300 hover:bg-rose-900"
                    title="Snooze 15 minutes"
                  >
                    +15m
                  </button>
                {/if}
              </div>
            {/if}
          </div>
        </div>
      {/each}
    {/if}
  </div>
</aside>
