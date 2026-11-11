<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface Snippet {
    id: string;
    title: string;
    description?: string;
    sql: string;
    tags?: string[];
  }

  let snippets = $state<Snippet[]>([]);
  let filterText = $state('');
  let isLoading = $state(false);

  // Editor mode: null = browse, 'new' = creating, or Snippet = editing
  let editingSnippet = $state<Partial<Snippet> | null>(null);
  let editTagsStr = $state('');

  // Param substitution prompt
  let paramPrompt = $state<{
    snippet: Snippet;
    params: { name: string; value: string }[];
  } | null>(null);

  $effect(() => {
    if (appState.isSnippetsModalOpen) {
      loadSnippets();
      editingSnippet = null;
      paramPrompt = null;
    }
  });

  async function loadSnippets() {
    isLoading = true;
    try {
      const res = await api.listSnippets();
      snippets = Array.isArray(res) ? res : [];
    } catch (err: any) {
      // Fallback default templates if endpoint empty
      snippets = [
        {
          id: 'snip-1',
          title: 'Find Top 10 Slow Queries',
          description: 'Inspect MySQL performance schema for slowest queries',
          sql: 'SELECT digest_text, count_star, avg_timer_wait/1000000000 AS avg_latency_ms FROM performance_schema.events_statements_summary_by_digest ORDER BY avg_timer_wait DESC LIMIT 10;',
          tags: ['performance', 'dba'],
        },
        {
          id: 'snip-2',
          title: 'Table Fragmentation & Data Size',
          description: 'Find fragmented tables in active database',
          sql: 'SELECT table_name, data_length, index_length, data_free FROM information_schema.tables WHERE table_schema = {{database_name}} AND data_free > 0 ORDER BY data_free DESC;',
          tags: ['storage', 'maintenance'],
        },
        {
          id: 'snip-3',
          title: 'Filter Records by Date Range',
          description: 'Parameterized template for filtering rows',
          sql: 'SELECT * FROM {{table_name}} WHERE created_at BETWEEN {{start_date}} AND {{end_date}} LIMIT 100;',
          tags: ['template', 'crud'],
        },
      ];
    } finally {
      isLoading = false;
    }
  }

  let filteredSnippets = $derived.by(() => {
    const q = filterText.toLowerCase().trim();
    if (!q) return snippets;
    return snippets.filter(
      (s) =>
        s.title.toLowerCase().includes(q) ||
        s.description?.toLowerCase().includes(q) ||
        s.sql.toLowerCase().includes(q) ||
        s.tags?.some((t) => t.toLowerCase().includes(q))
    );
  });

  function startCreateSnippet() {
    editingSnippet = {
      title: '',
      description: '',
      sql: appState.activeTab?.sql || 'SELECT * FROM users WHERE status = {{status}};',
      tags: [],
    };
    editTagsStr = '';
  }

  function startEditSnippet(s: Snippet) {
    editingSnippet = { ...s };
    editTagsStr = (s.tags || []).join(', ');
  }

  async function handleSaveSnippet() {
    if (!editingSnippet || !editingSnippet.title?.trim() || !editingSnippet.sql?.trim()) {
      appState.showToast('Snippet title and SQL are required', 'error');
      return;
    }

    const tags = editTagsStr
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean);

    try {
      const saved = await api.saveSnippet({
        id: editingSnippet.id,
        title: editingSnippet.title.trim(),
        description: editingSnippet.description?.trim(),
        sql: editingSnippet.sql.trim(),
        tags,
      });
      appState.showToast(`Snippet "${editingSnippet.title}" saved`, 'success');
      editingSnippet = null;
      await loadSnippets();
    } catch (err: any) {
      // Local fallback
      if (editingSnippet.id) {
        const idx = snippets.findIndex((s) => s.id === editingSnippet!.id);
        if (idx !== -1) snippets[idx] = { ...snippets[idx], ...editingSnippet, tags } as Snippet;
      } else {
        snippets.push({
          id: `snip-${Date.now()}`,
          title: editingSnippet.title.trim(),
          description: editingSnippet.description?.trim(),
          sql: editingSnippet.sql.trim(),
          tags,
        });
      }
      editingSnippet = null;
      appState.showToast('Snippet saved locally', 'success');
    }
  }

  async function handleDeleteSnippet(id: string) {
    if (!confirm('Are you sure you want to delete this query snippet?')) return;
    try {
      await api.deleteSnippet(id);
      appState.showToast('Snippet deleted', 'info');
      await loadSnippets();
    } catch (err: any) {
      snippets = snippets.filter((s) => s.id !== id);
      appState.showToast('Snippet deleted', 'info');
    }
  }

  function useSnippet(s: Snippet) {
    // Detect {{var}} placeholders
    const matches = Array.from(s.sql.matchAll(/\{\{([a-zA-Z0-9_]+)\}\}/g));
    const paramNames = Array.from(new Set(matches.map((m) => m[1])));

    if (paramNames.length > 0) {
      paramPrompt = {
        snippet: s,
        params: paramNames.map((name) => {
          let defaultValue = '';
          if (name.includes('db') || name.includes('database')) defaultValue = `'${appState.currentDatabase || 'app'}'`;
          else if (name.includes('table')) defaultValue = appState.filteredTables[0]?.name || 'users';
          else if (name.includes('date')) defaultValue = `'${new Date().toISOString().slice(0, 10)}'`;
          return { name, value: defaultValue };
        }),
      };
      return;
    }

    applySqlToEditor(s.sql, s.title);
  }

  function submitParamPrompt() {
    if (!paramPrompt) return;
    let finalSql = paramPrompt.snippet.sql;
    for (const p of paramPrompt.params) {
      const regex = new RegExp(`\\{\\{${p.name}\\}\\}`, 'g');
      finalSql = finalSql.replace(regex, p.value || `/* ${p.name} */`);
    }
    applySqlToEditor(finalSql, paramPrompt.snippet.title);
    paramPrompt = null;
  }

  function applySqlToEditor(sql: string, title: string) {
    if (appState.activeTab && !appState.activeTab.isTableViewer && !appState.activeTab.isProcesslist && !appState.activeTab.isErdCanvas) {
      appState.activeTab.sql = sql;
      appState.showToast(`Applied snippet: ${title}`, 'success');
    } else {
      appState.addTab(`${title.replace(/[^a-zA-Z0-9_-]/g, '_')}.sql`, sql);
      appState.showToast(`Created worksheet with snippet: ${title}`, 'success');
    }
    appState.isSnippetsModalOpen = false;
  }

  function handleClose() {
    appState.isSnippetsModalOpen = false;
  }
</script>

{#if appState.isSnippetsModalOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 font-sans">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="snippets-modal-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-2xl flex flex-col max-h-[85vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400">
            <Icon name="bookmark" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="snippets-modal-title" class="font-bold text-sm text-slate-100">
              Query Snippets & Templates Library
            </h3>
            <p class="text-[11px] text-slate-400">
              Reusable SQL templates with dynamic placeholder variables
            </p>
          </div>
        </div>
        <button
          onclick={handleClose}
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Content Area -->
      <div class="flex-1 overflow-hidden flex flex-col">
        {#if paramPrompt}
          <!-- Parameter Fill-in Form -->
          <div class="p-5 space-y-4 flex-1 overflow-y-auto">
            <div class="bg-sky-950/40 border border-sky-800/60 p-3 rounded-lg text-xs space-y-1">
              <span class="font-bold text-sky-300 block">Configure Template Parameters</span>
              <p class="text-slate-400 text-[11px]">
                Fill in the values for the template placeholders in "{paramPrompt.snippet.title}":
              </p>
            </div>

            <div class="space-y-3">
              {#each paramPrompt.params as param}
                <div class="space-y-1">
                  <label for="param-{param.name}" class="text-xs font-mono font-semibold text-sky-400">
                    {`{{${param.name}}}`}
                  </label>
                  <input
                    id="param-{param.name}"
                    type="text"
                    bind:value={param.value}
                    class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-1.5 text-xs text-white font-mono outline-none"
                  />
                </div>
              {/each}
            </div>
          </div>

          <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-end space-x-2 shrink-0 text-xs">
            <button
              onclick={() => (paramPrompt = null)}
              class="px-3.5 py-1.5 font-semibold text-slate-400 hover:text-white transition"
            >
              Back
            </button>
            <button
              onclick={submitParamPrompt}
              class="px-4 py-1.5 bg-sky-600 hover:bg-sky-500 text-white rounded font-semibold flex items-center space-x-1.5 transition shadow"
            >
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Insert Formatted Query</span>
            </button>
          </div>
        {:else if editingSnippet}
          <!-- Create / Edit Form -->
          <div class="p-5 space-y-3 flex-1 overflow-y-auto text-xs">
            <div class="space-y-1">
              <label for="snip-title" class="font-semibold text-slate-300">Snippet Title *</label>
              <input
                id="snip-title"
                type="text"
                bind:value={editingSnippet.title}
                placeholder="e.g. Find Active Subscriptions"
                class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-1.5 text-xs text-white outline-none"
              />
            </div>

            <div class="space-y-1">
              <label for="snip-desc" class="font-semibold text-slate-300">Description</label>
              <input
                id="snip-desc"
                type="text"
                bind:value={editingSnippet.description}
                placeholder="Brief summary of what this query does"
                class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-1.5 text-xs text-white outline-none"
              />
            </div>

            <div class="space-y-1">
              <label for="snip-tags" class="font-semibold text-slate-300">Tags (comma separated)</label>
              <input
                id="snip-tags"
                type="text"
                bind:value={editTagsStr}
                placeholder="analytics, users, reports"
                class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-1.5 text-xs text-white outline-none"
              />
            </div>

            <div class="space-y-1">
              <label for="snip-sql" class="font-semibold text-slate-300 flex items-center justify-between">
                <span>SQL Query *</span>
                <span class="text-[10px] text-slate-500 font-mono">Supports {"{{placeholder}}"}</span>
              </label>
              <textarea
                id="snip-sql"
                bind:value={editingSnippet.sql}
                rows="6"
                placeholder="SELECT * FROM table WHERE id = {{id}};"
                class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded p-2.5 text-xs text-emerald-300 font-mono outline-none resize-y"
              ></textarea>
            </div>
          </div>

          <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-end space-x-2 shrink-0 text-xs">
            <button
              onclick={() => (editingSnippet = null)}
              class="px-3.5 py-1.5 font-semibold text-slate-400 hover:text-white transition"
            >
              Cancel
            </button>
            <button
              onclick={handleSaveSnippet}
              class="px-4 py-1.5 bg-sky-600 hover:bg-sky-500 text-white rounded font-semibold flex items-center space-x-1.5 transition shadow"
            >
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Save Snippet</span>
            </button>
          </div>
        {:else}
          <!-- Browse Snippets Mode -->
          <div class="p-3 bg-[#090d16] border-b border-[#1e293b] flex items-center justify-between gap-3">
            <div class="relative flex-1">
              <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-2.5 pointer-events-none" />
              <input
                type="text"
                bind:value={filterText}
                placeholder="Search snippets by name, tag, or SQL..."
                class="w-full bg-[#0b0f19] border border-[#334155] focus:border-sky-500 rounded pl-8 pr-4 py-1.5 text-xs text-slate-200 placeholder-slate-500 outline-none"
              />
            </div>

            <button
              onclick={startCreateSnippet}
              class="px-3 py-1.5 bg-sky-600 hover:bg-sky-500 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow shrink-0"
            >
              <Icon name="plus" class="w-3.5 h-3.5" />
              <span>New Snippet</span>
            </button>
          </div>

          <div class="flex-1 overflow-y-auto p-4 space-y-3">
            {#if filteredSnippets.length === 0}
              <div class="p-8 text-center text-slate-500 text-xs italic">
                {isLoading ? 'Loading query snippets...' : 'No snippets match your filter.'}
              </div>
            {:else}
              {#each filteredSnippets as s (s.id)}
                <div class="bg-[#0f172a] border border-[#1e293b] hover:border-slate-600 rounded-xl p-3.5 space-y-2.5 transition group">
                  <div class="flex items-center justify-between">
                    <div>
                      <h4 class="font-bold text-xs text-slate-100 group-hover:text-sky-300 transition-colors">
                        {s.title}
                      </h4>
                      {#if s.description}
                        <p class="text-[11px] text-slate-400 mt-0.5">{s.description}</p>
                      {/if}
                    </div>

                    <div class="flex items-center space-x-1.5 shrink-0">
                      <button
                        onclick={() => startEditSnippet(s)}
                        class="p-1 hover:bg-slate-700 text-slate-400 hover:text-white rounded transition"
                        title="Edit snippet"
                      >
                        <Icon name="edit" class="w-3.5 h-3.5" />
                      </button>
                      <button
                        onclick={() => handleDeleteSnippet(s.id)}
                        class="p-1 hover:bg-slate-700 text-slate-400 hover:text-rose-400 rounded transition"
                        title="Delete snippet"
                      >
                        <Icon name="trash-2" class="w-3.5 h-3.5" />
                      </button>
                      <button
                        onclick={() => useSnippet(s)}
                        class="px-2.5 py-1 bg-sky-950 hover:bg-sky-900 border border-sky-800 text-sky-300 rounded text-xs font-semibold flex items-center space-x-1 transition shadow-sm ml-1"
                        title="Insert into worksheet"
                      >
                        <Icon name="play" class="w-3 h-3 fill-current" />
                        <span>Use</span>
                      </button>
                    </div>
                  </div>

                  <!-- SQL Snippet Preview -->
                  <pre class="p-2.5 rounded bg-[#090d16] border border-slate-800 text-[11px] text-emerald-300/90 font-mono overflow-x-auto whitespace-pre-wrap max-h-24 select-text">{s.sql}</pre>

                  <!-- Tags -->
                  {#if s.tags && s.tags.length > 0}
                    <div class="flex items-center space-x-1">
                      {#each s.tags as tag}
                        <span class="px-1.5 py-0.2 rounded bg-slate-800 text-slate-400 text-[10px] font-mono border border-slate-700">
                          #{tag}
                        </span>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}
