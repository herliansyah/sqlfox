<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableName: string;
  }

  const { tableName }: Props = $props();

  interface NoteItem {
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

  let notes = $state<NoteItem[]>([]);
  let isLoading = $state(false);

  $effect(() => {
    if (tableName) {
      loadNotes();
    }
  });

  async function loadNotes() {
    isLoading = true;
    try {
      const res = await api.listNotes();
      const allNotes = Array.isArray(res) ? res : [];
      notes = allNotes.filter(
        (n: NoteItem) =>
          n.table_name?.toLowerCase() === tableName.toLowerCase() ||
          n.content?.toLowerCase().includes(tableName.toLowerCase())
      );
    } catch (err: any) {
      notes = [];
    } finally {
      isLoading = false;
    }
  }

  let pinnedRunbooks = $derived(notes.filter((n) => n.is_pinned));
  let regularNotes = $derived(notes.filter((n) => !n.is_pinned));

  function createRunbook() {
    appState.editingNote = {
      title: `Runbook & Rules: ${tableName}`,
      content: `### Business Rules for \`${tableName}\`\n\n- Primary Key strategy:\n- Cascade deletion policies:\n- Common queries & indexes:\n`,
      category: 'Runbook',
      table_name: tableName,
      is_pinned: true,
    };
    appState.isNoteComposerModalOpen = true;
  }

  function createIncidentNote() {
    appState.editingNote = {
      title: `Incident: ${tableName}`,
      content: '',
      category: 'Incident',
      table_name: tableName,
      is_pinned: false,
    };
    appState.isNoteComposerModalOpen = true;
  }

  function editNote(n: NoteItem) {
    appState.editingNote = { ...n };
    appState.isNoteComposerModalOpen = true;
  }

  async function toggleResolve(n: NoteItem) {
    try {
      if (!n.is_resolved) {
        await api.resolveNote(n.id);
        appState.showToast(`Note "${n.title}" resolved`, 'info');
      } else {
        await api.updateNote(n.id, { is_resolved: false });
      }
      await loadNotes();
    } catch (err: any) {
      appState.showToast(`Failed: ${err.message}`, 'error');
    }
  }

  function openQueryInEditor(sql: string) {
    appState.addTab(`query_${tableName}.sql`, sql);
    appState.showToast('Opened query snapshot in Worksheet', 'info');
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-y-auto p-5 select-none font-sans text-xs">
  <!-- Header Bar -->
  <div class="flex items-center justify-between pb-4 mb-5 border-b border-[#1e293b] shrink-0">
    <div class="flex items-center space-x-2.5">
      <div class="p-2 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-400">
        <Icon name="file-text" class="w-4 h-4" />
      </div>
      <div>
        <h3 class="font-bold text-slate-100 text-sm flex items-center space-x-2">
          <span>Table Runbooks & Operational Notes</span>
          <span class="px-2 py-0.5 rounded-full bg-slate-900 border border-slate-800 text-[10px] text-slate-400 font-mono">
            {notes.length} note(s)
          </span>
        </h3>
        <p class="text-[11px] text-slate-400">
          Persistent institutional knowledge, incident logs, and SQL snapshots for <code class="font-mono text-sky-400">{tableName}</code>
        </p>
      </div>
    </div>

    <div class="flex items-center space-x-2">
      <button
        type="button"
        onclick={loadNotes}
        class="p-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-400 hover:text-white transition"
        title="Refresh notes"
      >
        <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin' : ''}" />
      </button>
      <button
        type="button"
        onclick={createIncidentNote}
        class="px-2.5 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-medium transition flex items-center space-x-1.5"
      >
        <Icon name="plus" class="w-3.5 h-3.5" />
        <span>Add Note</span>
      </button>
      <button
        type="button"
        onclick={createRunbook}
        class="px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 text-white font-semibold transition flex items-center space-x-1.5 shadow-lg shadow-amber-950/30"
      >
        <Icon name="pin" class="w-3.5 h-3.5" />
        <span>New Pinned Runbook</span>
      </button>
    </div>
  </div>

  {#if isLoading}
    <div class="p-16 text-center text-slate-500 flex flex-col items-center gap-2">
      <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-amber-400" />
      <span>Loading operational notes for {tableName}...</span>
    </div>
  {:else if notes.length === 0}
    <div class="p-12 text-center bg-[#0b0f19] border border-dashed border-[#1e293b] rounded-2xl flex flex-col items-center justify-center space-y-3 my-auto max-w-md mx-auto">
      <div class="p-3 rounded-full bg-amber-500/10 text-amber-400">
        <Icon name="file-text" class="w-8 h-8" />
      </div>
      <div class="space-y-1">
        <h4 class="font-bold text-slate-200 text-sm">No runbooks or notes for "{tableName}"</h4>
        <p class="text-slate-400 text-xs">
          Document gotchas, migration caveats, business definitions, and query patterns for this table.
        </p>
      </div>
      <button
        type="button"
        onclick={createRunbook}
        class="px-3.5 py-2 rounded-lg bg-amber-600 hover:bg-amber-500 text-white font-semibold text-xs transition flex items-center space-x-1.5 shadow-lg"
      >
        <Icon name="pin" class="w-3.5 h-3.5" />
        <span>Create First Runbook</span>
      </button>
    </div>
  {:else}
    <div class="space-y-5 max-w-4xl mx-auto w-full">
      <!-- Pinned Runbooks Section -->
      {#if pinnedRunbooks.length > 0}
        <div class="space-y-3">
          <div class="text-[11px] font-bold text-amber-400 uppercase tracking-wider flex items-center space-x-1.5">
            <Icon name="pin" class="w-3.5 h-3.5" />
            <span>Pinned Runbooks & Business Rules</span>
          </div>

          <div class="grid gap-3">
            {#each pinnedRunbooks as note (note.id)}
              <div class="p-4 rounded-xl bg-[#0b0f19] border border-amber-500/40 shadow-lg space-y-2.5">
                <div class="flex items-start justify-between">
                  <div>
                    <h4 class="font-bold text-slate-100 text-sm flex items-center space-x-2">
                      <span>{note.title}</span>
                      <span class="px-2 py-0.2 rounded bg-amber-950/80 border border-amber-700/60 text-amber-300 text-[10px] uppercase font-bold">
                        {note.category || 'Runbook'}
                      </span>
                    </h4>
                    <span class="text-[10px] text-slate-500 font-mono">
                      Updated {new Date(note.updated_at || note.created_at || '').toLocaleString()}
                    </span>
                  </div>

                  <div class="flex items-center space-x-1.5">
                    {#if note.query_snapshot}
                      <button
                        type="button"
                        onclick={() => openQueryInEditor(note.query_snapshot!)}
                        class="px-2 py-1 rounded bg-sky-950/60 hover:bg-sky-900/60 text-sky-300 border border-sky-800 text-xs transition flex items-center space-x-1"
                        title="Open SQL snapshot in editor"
                      >
                        <Icon name="file-code" class="w-3 h-3" />
                        <span>Run SQL</span>
                      </button>
                    {/if}
                    <button
                      type="button"
                      onclick={() => editNote(note)}
                      class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition"
                      title="Edit note"
                    >
                      <Icon name="edit" class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div class="text-slate-300 text-xs leading-relaxed whitespace-pre-wrap font-sans bg-[#050811] p-3 rounded-lg border border-[#1e293b]">
                  {note.content}
                </div>

                {#if note.query_snapshot}
                  <div class="space-y-1">
                    <span class="text-[10px] text-slate-500 uppercase font-bold font-mono">Captured Query Snapshot</span>
                    <pre class="p-2.5 rounded bg-[#050811] border border-[#1e293b] font-mono text-emerald-300 text-[11px] overflow-x-auto whitespace-pre-wrap">{note.query_snapshot}</pre>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Regular Operational Notes Section -->
      {#if regularNotes.length > 0}
        <div class="space-y-3">
          <div class="text-[11px] font-bold text-slate-400 uppercase tracking-wider flex items-center space-x-1.5">
            <Icon name="file-text" class="w-3.5 h-3.5" />
            <span>Incident Notes & Logs</span>
          </div>

          <div class="grid gap-2.5">
            {#each regularNotes as note (note.id)}
              <div class="p-3.5 rounded-xl bg-[#0b0f19] border border-[#1e293b] hover:border-slate-700 transition space-y-2 {note.is_resolved ? 'opacity-60' : ''}">
                <div class="flex items-start justify-between">
                  <div>
                    <h4 class="font-semibold text-slate-200 text-xs flex items-center space-x-2">
                      <span class={note.is_resolved ? 'line-through text-slate-500' : ''}>{note.title}</span>
                      {#if note.category}
                        <span class="px-1.5 py-0.2 rounded bg-slate-900 border border-slate-800 text-slate-400 text-[9px] uppercase font-mono">
                          {note.category}
                        </span>
                      {/if}
                    </h4>
                    <span class="text-[10px] text-slate-500 font-mono">
                      {new Date(note.created_at || '').toLocaleDateString()}
                    </span>
                  </div>

                  <div class="flex items-center space-x-1">
                    <button
                      type="button"
                      onclick={() => toggleResolve(note)}
                      class="px-2 py-0.5 rounded text-[11px] border transition flex items-center space-x-1 {note.is_resolved ? 'bg-emerald-950/60 border-emerald-800 text-emerald-300' : 'bg-slate-900 border-slate-800 text-slate-400 hover:text-white'}"
                    >
                      <Icon name="check" class="w-3 h-3" />
                      <span>{note.is_resolved ? 'Resolved' : 'Resolve'}</span>
                    </button>
                    <button
                      type="button"
                      onclick={() => editNote(note)}
                      class="p-1 text-slate-400 hover:text-white rounded"
                    >
                      <Icon name="edit" class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                {#if note.content}
                  <p class="text-slate-400 text-xs leading-relaxed whitespace-pre-wrap">{note.content}</p>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>
