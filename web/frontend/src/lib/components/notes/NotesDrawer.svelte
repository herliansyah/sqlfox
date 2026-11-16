<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import { api, type OperationalNote } from '../../api/client';
  import Icon from '../common/Icon.svelte';
  import NoteMarkdownView from './NoteMarkdownView.svelte';

  let notes = $state<OperationalNote[]>([]);
  let filter = $state<'all' | 'inbox' | 'due' | 'pinned' | 'resolved'>('inbox');
  let activeTagFilter = $state<string | null>(null);
  let searchQuery = $state('');
  let filterByCurrentTable = $state(false);
  let isLoading = $state(false);
  let cloudConnected = $state<boolean | null>(null);
  let isSyncingCloud = $state(false);

  let activeTableName = $derived(appState.activeTab?.tableName || '');

  async function loadNotes() {
    isLoading = true;
    try {
      const data = await api.listNotes();
      notes = Array.isArray(data) ? data : [];
    } catch {
      notes = [];
    } finally {
      isLoading = false;
    }
  }

  async function checkCloudBadge() {
    try {
      const st = await api.getCloudSyncStatus();
      cloudConnected = Boolean(st?.enabled && st?.github_repo);
    } catch {
      cloudConnected = null;
    }
  }

  async function syncCloudNow() {
    if (!cloudConnected) {
      appState.isSettingsModalOpen = true;
      return;
    }
    isSyncingCloud = true;
    try {
      const res = await api.runCloudSync();
      appState.showToast(res.message || 'Cloud sync completed', res.success === false ? 'warning' : 'success');
      await loadNotes();
    } catch (e: any) {
      appState.showToast(`Cloud sync failed: ${e.message}`, 'error');
    } finally {
      isSyncingCloud = false;
    }
  }

  let dueCount = $derived.by(() => {
    const now = new Date();
    return notes.filter((n) => n.reminder_at && new Date(n.reminder_at) <= now && !n.is_resolved).length;
  });

  let availableTags = $derived.by(() => {
    const set = new Set<string>();
    for (const n of notes) {
      for (const ut of n.user_tags || []) set.add(ut);
      if (n.category) {
        const c = n.category.startsWith('#') ? n.category : `#${n.category}`;
        set.add(c);
      }
    }
    return Array.from(set).sort();
  });

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

    if (activeTagFilter) {
      const target = activeTagFilter.toLowerCase();
      list = list.filter((n) => {
        const tags = (n.user_tags || []).map((t) => t.toLowerCase());
        const cat = n.category ? `#${n.category.replace(/^#/, '').toLowerCase()}` : '';
        return tags.includes(target) || cat === target;
      });
    }

    if (filterByCurrentTable && activeTableName) {
      const tLow = activeTableName.toLowerCase();
      list = list.filter(
        (n) =>
          n.table_name?.toLowerCase() === tLow ||
          (n.object_tags || []).some((ot) => ot.toLowerCase().endsWith(`/${tLow}`))
      );
    }

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      list = list.filter(
        (n) =>
          n.title.toLowerCase().includes(q) ||
          (n.side_note && n.side_note.toLowerCase().includes(q)) ||
          n.content.toLowerCase().includes(q) ||
          (n.category && n.category.toLowerCase().includes(q)) ||
          (n.user_tags || []).some((t) => t.toLowerCase().includes(q))
      );
    }

    return list.slice().sort((a, b) => {
      if (a.is_pinned && !b.is_pinned) return -1;
      if (!a.is_pinned && b.is_pinned) return 1;
      const dateA = new Date(a.updated_at || a.created_at || 0).getTime();
      const dateB = new Date(b.updated_at || b.created_at || 0).getTime();
      return dateB - dateA;
    });
  });

  function formatNoteMarkdown(note: OperationalNote): string {
    const lines = [`# ${note.title}`];
    if (note.side_note) lines.push(`> ${note.side_note}`);
    const meta: string[] = [];
    if (note.user_tags?.length) meta.push(note.user_tags.join(' '));
    if (note.object_tags?.length) meta.push(note.object_tags.join(' '));
    if (meta.length) lines.push(meta.join(' · '));
    lines.push('', note.content || '');
    return lines.join('\n').trim();
  }

  function exportAllMarkdown() {
    if (filteredNotes.length === 0) return;
    const md = filteredNotes.map(formatNoteMarkdown).join('\n\n---\n\n');
    const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `sqlfox-notes-${new Date().toISOString().slice(0, 10)}.md`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    appState.showToast(`Exported ${filteredNotes.length} note(s) to .md`, 'success');
  }

  function exportSingleMarkdown(note: OperationalNote) {
    const blob = new Blob([formatNoteMarkdown(note)], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const slug = (note.title || 'note').toLowerCase().replace(/[^a-z0-9]+/g, '-');
    a.download = `${slug}.md`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function togglePin(note: OperationalNote) {
    try {
      const updated = { ...note, is_pinned: !note.is_pinned };
      await api.updateNote(note.id, updated);
      notes = notes.map((n) => (n.id === note.id ? updated : n));
      appState.notesRevision++;
    } catch (e: any) {
      appState.showToast(`Failed to update note: ${e.message}`, 'error');
    }
  }

  async function toggleResolve(note: OperationalNote) {
    try {
      if (!note.is_resolved) {
        await api.resolveNote(note.id);
        notes = notes.map((n) => (n.id === note.id ? { ...n, status: 'resolved', is_resolved: true } : n));
        appState.showToast('Note marked as resolved', 'info');
      } else {
        const updated = { ...note, status: 'inbox', is_resolved: false };
        await api.updateNote(note.id, updated);
        notes = notes.map((n) => (n.id === note.id ? updated : n));
        appState.showToast('Note reopened', 'info');
      }
      appState.notesRevision++;
      appState.checkDueNotesReminders();
    } catch (e: any) {
      appState.showToast(`Failed to update note: ${e.message}`, 'error');
    }
  }

  async function snoozeNote(note: OperationalNote, minutes: number) {
    try {
      await api.snoozeNote(note.id, minutes);
      appState.showToast(`Reminder snoozed by ${minutes} minutes`, 'info');
      await loadNotes();
      appState.notesRevision++;
      appState.checkDueNotesReminders();
    } catch (e: any) {
      appState.showToast(`Failed to snooze: ${e.message}`, 'error');
    }
  }

  async function deleteNote(id: string) {
    const ok = await appState.showNativeConfirm(
      'Are you sure you want to permanently delete this operational note?',
      'Delete Note'
    );
    if (!ok) return;
    try {
      await api.deleteNote(id);
      notes = notes.filter((n) => n.id !== id);
      appState.notesRevision++;
      appState.checkDueNotesReminders();
      appState.showToast('Note deleted', 'info');
    } catch (e: any) {
      appState.showToast(`Failed to delete note: ${e.message}`, 'error');
    }
  }

  function openComposer(noteToEdit?: OperationalNote) {
    appState.editingNote = noteToEdit || null;
    appState.isNoteComposerModalOpen = true;
  }

  function extractDbFromTags(note?: OperationalNote): string {
    for (const tag of note?.object_tags || []) {
      const m = tag.match(/^db:\/\/[^/]+\/([^/]+)\//);
      if (m && m[1] && m[1] !== 'db') return m[1];
    }
    return appState.currentDatabase || '';
  }

  function openQueryInWorksheet(sql?: string, note?: OperationalNote) {
    if (!sql) return;
    const isSpecial = (t: any) =>
      t.isTableViewer || t.isProcesslist || t.isCluster || t.isVariables || t.isFlightRecorder || t.isErdCanvas || t.isNote;
    const targetDb = extractDbFromTags(note);
    const active = appState.activeTab;
    const reusable = (active && !isSpecial(active) && !active.sql.trim())
      ? active
      : appState.tabs.find((t) => !isSpecial(t) && !t.sql.trim());

    if (reusable) {
      reusable.sql = sql;
      if (targetDb) reusable.database = targetDb;
      appState.activeTabId = reusable.id;
      appState.tabs = [...appState.tabs];
      appState.saveTabsState();
    } else {
      const tab = appState.addTab('Note Query.sql', sql);
      if (targetDb) tab.database = targetDb;
    }
    appState.showToast('Query opened in worksheet', 'info');
  }

  $effect(() => {
    const open = appState.isNotesDrawerOpen;
    const rev = appState.notesRevision;
    if (open) {
      void rev;
      loadNotes();
      checkCloudBadge();
    }
  });
</script>

<aside
  class="fixed top-12 right-0 bottom-7 w-full max-w-[540px] bg-[#0b0f19] border-l border-[#1e293b] shadow-2xl z-40 flex flex-col font-sans transition-transform duration-200 select-text"
>
  <!-- Drawer Header -->
  <div class="h-10 bg-[#0f172a] border-b border-[#1e293b] px-4 flex items-center justify-between flex-shrink-0 select-none gap-2">
    <div class="flex items-center space-x-2 min-w-0">
      <Icon name="file-text" class="w-4 h-4 text-amber-400 shrink-0" />
      <span class="font-bold text-slate-100 text-xs truncate">Operational Notes</span>
      <span class="text-slate-500 font-mono text-[10px]">({filteredNotes.length})</span>
      {#if dueCount > 0}
        <span class="px-1.5 py-0.2 rounded-full bg-rose-600 text-white font-mono text-[9.5px] font-bold">
          {dueCount} due
        </span>
      {/if}
    </div>

    <div class="flex items-center space-x-1.5 shrink-0">
      <button
        type="button"
        onclick={syncCloudNow}
        disabled={isSyncingCloud}
        class="px-2 py-0.5 rounded border text-[10.5px] flex items-center space-x-1 transition {cloudConnected
          ? 'bg-emerald-950/50 border-emerald-800 text-emerald-300 hover:bg-emerald-900/60'
          : 'bg-slate-800 border-slate-700 text-slate-400 hover:text-white'}"
        title={cloudConnected ? 'Run Cloud Sync Backup' : 'Configure Cloud Sync in Settings'}
      >
        <Icon name={isSyncingCloud ? 'refresh-cw' : 'cloud'} class="w-3 h-3 {isSyncingCloud ? 'animate-spin' : ''}" />
        <span>{cloudConnected ? 'Sync' : 'Cloud'}</span>
      </button>
      <button
        type="button"
        onclick={exportAllMarkdown}
        disabled={filteredNotes.length === 0}
        class="p-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 transition disabled:opacity-40"
        title="Export visible notes as .md"
      >
        <Icon name="download" class="w-3.5 h-3.5" />
      </button>
      <button
        type="button"
        onclick={() => openComposer()}
        class="px-2 py-0.5 rounded bg-amber-600 hover:bg-amber-500 text-white font-medium text-xs transition flex items-center space-x-1 shadow-sm"
      >
        <Icon name="plus" class="w-3 h-3" />
        <span>New Note</span>
      </button>
      <button
        type="button"
        onclick={() => (appState.isNotesDrawerOpen = false)}
        class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
        title="Close Drawer"
      >
        <Icon name="x" class="w-3.5 h-3.5" />
      </button>
    </div>
  </div>

  <!-- Search & Filters -->
  <div class="p-2.5 border-b border-[#1e293b] bg-[#070b14] space-y-2 flex-shrink-0 select-none">
    <div class="relative">
      <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-2" />
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Search notes by title, subtitle, content, tag..."
        class="w-full bg-[#0b0f19] border border-[#1e293b] rounded-lg pl-8 pr-3 py-1.5 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-amber-500"
      />
    </div>

    <div class="flex items-center justify-between flex-wrap gap-1.5">
      <div class="flex items-center space-x-1">
        {#each ['inbox', 'due', 'pinned', 'resolved', 'all'] as f}
          <button
            type="button"
            onclick={() => (filter = f as any)}
            class="px-2 py-0.5 rounded-full capitalize text-[10px] font-medium transition flex items-center space-x-1 {filter === f
              ? 'bg-amber-500/20 text-amber-300 border border-amber-500/40'
              : 'text-slate-400 hover:text-white hover:bg-slate-800'}"
          >
            <span>{f}</span>
            {#if f === 'due' && dueCount > 0}
              <span class="px-1 rounded-full bg-rose-600 text-white text-[9px] font-mono">{dueCount}</span>
            {/if}
          </button>
        {/each}
      </div>

      {#if activeTableName}
        <button
          type="button"
          onclick={() => (filterByCurrentTable = !filterByCurrentTable)}
          class="text-[10px] px-2 py-0.5 rounded border transition flex items-center space-x-1 {filterByCurrentTable
            ? 'bg-sky-950 border-sky-600 text-sky-300'
            : 'border-slate-800 text-slate-500 hover:text-slate-300'}"
        >
          <span>@{activeTableName}</span>
        </button>
      {/if}
    </div>

    {#if availableTags.length > 0}
      <div class="flex items-center gap-1 flex-wrap pt-0.5">
        {#each availableTags as tag}
          <button
            type="button"
            onclick={() => (activeTagFilter = activeTagFilter === tag ? null : tag)}
            class="px-1.5 py-0.2 rounded text-[10px] font-mono border transition {activeTagFilter === tag
              ? 'bg-amber-950/70 border-amber-600 text-amber-200'
              : 'bg-slate-900 border-slate-800 text-slate-400 hover:text-slate-200'}"
          >
            {tag}
          </button>
        {/each}
      </div>
    {/if}
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
          type="button"
          onclick={() => openComposer()}
          class="text-[11px] text-amber-400 hover:underline"
        >
          + Create your first note
        </button>
      </div>
    {:else}
      {#each filteredNotes as note (note.id)}
        <div
          class="p-3 rounded-xl border bg-[#0e1424] transition shadow-md hover:border-slate-700 flex flex-col space-y-2 {note.is_pinned
            ? 'border-amber-500/50 shadow-amber-950/20'
            : note.is_resolved
            ? 'border-[#1e293b] opacity-65'
            : 'border-[#1e293b]'}"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-start space-x-1.5 flex-1 min-w-0">
              <button
                type="button"
                onclick={() => toggleResolve(note)}
                class="text-slate-500 hover:text-emerald-400 shrink-0 mt-0.5"
                title={note.is_resolved ? 'Reopen note' : 'Mark resolved'}
              >
                <Icon name={note.is_resolved ? 'check-circle' : 'circle'} class="w-3.5 h-3.5 {note.is_resolved ? 'text-emerald-400' : ''}" />
              </button>
              <div class="min-w-0 flex-1">
                <h4 class="font-bold text-slate-200 text-xs truncate {note.is_resolved ? 'line-through text-slate-500' : ''}">
                  {note.title}
                </h4>
                {#if note.side_note}
                  <p class="text-[10.5px] text-amber-300/80 italic truncate">{note.side_note}</p>
                {/if}
              </div>
            </div>

            <div class="flex items-center space-x-0.5 shrink-0">
              <button
                type="button"
                onclick={() => { copyText(formatNoteMarkdown(note)); appState.showToast('Markdown copied!', 'info'); }}
                class="p-1 rounded text-slate-500 hover:text-slate-200 transition"
                title="Copy Markdown"
              >
                <Icon name="copy" class="w-3 h-3" />
              </button>
              <button
                type="button"
                onclick={() => exportSingleMarkdown(note)}
                class="p-1 rounded text-slate-500 hover:text-slate-200 transition"
                title="Export .md"
              >
                <Icon name="download" class="w-3 h-3" />
              </button>
              <button
                type="button"
                onclick={() => togglePin(note)}
                class="p-1 rounded text-slate-500 hover:text-amber-400 transition"
                title={note.is_pinned ? 'Unpin' : 'Pin note'}
              >
                <Icon name="star" class="w-3.5 h-3.5 {note.is_pinned ? 'text-amber-400 fill-amber-400' : ''}" />
              </button>
              <button
                type="button"
                onclick={() => openComposer(note)}
                class="p-1 rounded text-slate-500 hover:text-sky-300 transition"
                title="Edit note"
              >
                <Icon name="edit" class="w-3.5 h-3.5" />
              </button>
              <button
                type="button"
                onclick={() => deleteNote(note.id)}
                class="p-1 rounded text-slate-500 hover:text-rose-400 transition"
                title="Delete note"
              >
                <Icon name="trash-2" class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>

          <div class="py-0.5">
            <NoteMarkdownView content={note.content} onOpenQuery={(sql) => openQueryInWorksheet(sql, note)} />
          </div>

          {#if note.query_snapshot}
            <div class="p-2 rounded bg-[#070a12] border border-sky-950 flex items-center justify-between text-[11px] font-mono text-sky-300">
              <span class="truncate flex-1 mr-2">{note.query_snapshot}</span>
              <button
                type="button"
                onclick={() => openQueryInWorksheet(note.query_snapshot, note)}
                class="text-[10px] text-sky-400 hover:underline shrink-0 flex items-center space-x-1"
              >
                <Icon name="play" class="w-2.5 h-2.5" />
                <span>Open in Query</span>
              </button>
            </div>
          {/if}

          <!-- Metadata Badges & Due Reminder -->
          <div class="flex items-center justify-between pt-1 text-[10px] text-slate-500 select-none flex-wrap gap-1.5">
            <div class="flex items-center gap-1 flex-wrap">
              {#each note.user_tags || [] as ut}
                <button
                  type="button"
                  onclick={() => (activeTagFilter = activeTagFilter === ut ? null : ut)}
                  class="px-1.5 py-0.2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300"
                >
                  {ut}
                </button>
              {/each}
              {#if !note.user_tags?.length && note.category}
                <span class="px-1.5 py-0.2 rounded bg-slate-800 text-slate-400">#{note.category}</span>
              {/if}
              {#each note.object_tags || [] as ot}
                <span class="px-1.5 py-0.2 rounded bg-sky-950 text-sky-300 border border-sky-800 font-mono truncate max-w-[170px]" title={ot}>
                  {ot}
                </span>
              {/each}
              {#if !note.object_tags?.length && note.table_name}
                <span class="px-1.5 py-0.2 rounded bg-sky-950 text-sky-300 border border-sky-800 font-mono">
                  {note.table_name}
                </span>
              {/if}
            </div>

            {#if note.reminder_at}
              {@const reminderDate = new Date(note.reminder_at)}
              {#if !isNaN(reminderDate.getTime())}
                {@const isDue = reminderDate <= new Date() && !note.is_resolved}
                <div class="flex items-center space-x-1 {isDue ? 'text-rose-400 font-semibold' : 'text-slate-400'}">
                  <Icon name="clock" class="w-3 h-3" />
                  <span>{reminderDate.toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>
                  {#if isDue}
                    <button
                      type="button"
                      onclick={() => snoozeNote(note, 15)}
                      class="ml-1 text-[9px] px-1 py-0.2 rounded bg-rose-950 border border-rose-800 text-rose-300 hover:bg-rose-900"
                      title="Snooze 15 minutes"
                    >
                      +15m
                    </button>
                  {/if}
                </div>
              {/if}
            {/if}
          </div>
        </div>
      {/each}
    {/if}
  </div>
</aside>
