<script lang="ts">
  import { tick } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { quoteSqlIdentifier } from '../../utils/sqlStatements';
  import Icon from '../common/Icon.svelte';

  interface SearchEntry {
    kind: 'TBL' | 'VIEW' | 'COL';
    database: string;
    table: string;
    column?: string;
    type?: string;
    label: string;
    insertPath: string;
  }

  let query = $state('');
  let activeIndex = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);
  let listEl = $state<HTMLDivElement | null>(null);

  let allEntries = $derived.by((): SearchEntry[] => {
    const out: SearchEntry[] = [];
    const seen = new Set<string>();
    const driver = appState.activeConnection?.driver;

    const addCatalog = (dbName: string, tables: any[]) => {
      for (const t of tables || []) {
        const kind: 'TBL' | 'VIEW' = t.type === 'VIEW' ? 'VIEW' : 'TBL';
        const qDb = dbName ? `${quoteSqlIdentifier(dbName, driver)}.` : '';
        const qTbl = quoteSqlIdentifier(t.name, driver);
        const tKey = `${dbName.toLowerCase()}::${t.name.toLowerCase()}`;
        if (!seen.has(tKey)) {
          seen.add(tKey);
          out.push({
            kind,
            database: dbName,
            table: t.name,
            type: kind === 'VIEW' ? 'View' : 'Table',
            label: dbName ? `${dbName}.${t.name}` : t.name,
            insertPath: `${qDb}${qTbl}`,
          });
        }
        for (const c of t.columns || []) {
          const cName = typeof c === 'string' ? c : c.name;
          const cType = typeof c === 'string' ? '' : c.type || '';
          const cKey = `${tKey}::${cName.toLowerCase()}`;
          if (!seen.has(cKey)) {
            seen.add(cKey);
            out.push({
              kind: 'COL',
              database: dbName,
              table: t.name,
              column: cName,
              type: cType,
              label: dbName ? `${dbName}.${t.name}.${cName}` : `${t.name}.${cName}`,
              insertPath: `${qDb}${qTbl}.${quoteSqlIdentifier(cName, driver)}`,
            });
          }
        }
      }
    };

    if (appState.catalog?.tables) {
      addCatalog(appState.currentDatabase || appState.catalog.database_name || '', appState.catalog.tables);
    }
    for (const [key, cat] of Object.entries(appState.catalogs || {})) {
      if (cat?.tables) {
        addCatalog(cat.database_name || key, cat.tables);
      }
    }
    return out;
  });

  let results = $derived.by(() => {
    const q = query.toLowerCase().trim();
    if (!q) return allEntries.slice(0, 60);
    const terms = q.split(/\s+/).filter(Boolean);
    return allEntries
      .filter((entry) => {
        const hay = `${entry.label} ${entry.type || ''}`.toLowerCase();
        return terms.every((t) => hay.includes(t));
      })
      .slice(0, 80);
  });

  $effect(() => {
    if (appState.isSchemaSearchOpen) {
      query = '';
      activeIndex = 0;
      tick().then(() => inputEl?.focus());
    }
  });

  $effect(() => {
    if (activeIndex >= results.length) {
      activeIndex = Math.max(0, results.length - 1);
    }
  });

  async function openEntry(entry: SearchEntry) {
    appState.isSchemaSearchOpen = false;
    if (entry.database && entry.database !== appState.currentDatabase) {
      await appState.selectDatabase(entry.database);
    }
    appState.openTableDataTab(entry.table);
  }

  function insertEntry(entry: SearchEntry) {
    appState.isSchemaSearchOpen = false;
    appState.insertSnippetSQL(entry.insertPath, entry.table);
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (results.length) {
        activeIndex = (activeIndex + 1) % results.length;
        tick().then(() => listEl?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' }));
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (results.length) {
        activeIndex = (activeIndex - 1 + results.length) % results.length;
        tick().then(() => listEl?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' }));
      }
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const target = results[activeIndex];
      if (!target) return;
      if (e.shiftKey) insertEntry(target);
      else void openEntry(target);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      appState.isSchemaSearchOpen = false;
    }
  }
</script>

{#if appState.isSchemaSearchOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div
    id="schema-search-modal"
    class="fixed inset-0 z-[95] flex items-start justify-center pt-[10vh] bg-black/70 backdrop-blur-sm p-4"
    onclick={(e) => e.target === e.currentTarget && (appState.isSchemaSearchOpen = false)}
  >
    <div class="bg-[#0f172a] border border-slate-700 w-full max-w-2xl rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[75vh] animate-in fade-in zoom-in-95 duration-100">
      <div class="p-3 border-b border-slate-800 flex items-center gap-2.5 bg-[#090d16]">
        <Icon name="search" class="w-4 h-4 text-sky-400 shrink-0" />
        <input
          bind:this={inputEl}
          bind:value={query}
          onkeydown={handleKeyDown}
          type="text"
          placeholder="Search tables, views, and columns across all databases..."
          class="flex-1 bg-transparent text-sm text-slate-100 placeholder-slate-500 focus:outline-none font-mono"
        />
        <span class="text-[10px] font-mono text-slate-500 hidden sm:inline">
          Enter: Open · Shift+Enter: Insert
        </span>
        <button
          onclick={() => (appState.isSchemaSearchOpen = false)}
          class="p-1 text-slate-400 hover:text-white rounded"
          aria-label="Close schema search"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div bind:this={listEl} class="flex-1 overflow-y-auto divide-y divide-slate-800/50">
        {#if results.length === 0}
          <div class="p-8 text-center text-xs text-slate-500">
            No matching tables, views, or columns found across cached catalogs.
          </div>
        {:else}
          {#each results as item, i (item.kind + ':' + item.label)}
            <div
              data-active={i === activeIndex}
              onmouseenter={() => (activeIndex = i)}
              class="px-3.5 py-2 flex items-center justify-between gap-2 text-xs transition-colors {i === activeIndex
                ? 'bg-sky-500/15 text-white'
                : 'hover:bg-slate-800/60 text-slate-300'}"
            >
              <button
                type="button"
                onclick={() => openEntry(item)}
                class="flex-1 flex items-center gap-2.5 text-left min-w-0"
              >
                <span
                  class="px-1.5 py-0.5 rounded text-[9px] font-mono font-bold shrink-0 {item.kind === 'TBL'
                    ? 'bg-sky-500/20 text-sky-300 border border-sky-500/40'
                    : item.kind === 'VIEW'
                      ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/40'
                      : 'bg-purple-500/20 text-purple-300 border border-purple-500/40'}"
                >
                  {item.kind}
                </span>
                <span class="font-mono truncate">{item.label}</span>
                {#if item.type}
                  <span class="text-[10px] text-slate-500 font-mono truncate">{item.type}</span>
                {/if}
              </button>

              <div class="flex items-center gap-1.5 shrink-0">
                <button
                  type="button"
                  onclick={() => insertEntry(item)}
                  class="px-2 py-0.5 rounded bg-slate-800 hover:bg-sky-600 text-slate-300 hover:text-white border border-slate-700 text-[10px] font-mono transition"
                  title="Insert qualified identifier into SQL Editor (Shift+Enter)"
                >
                  Insert ↵
                </button>
              </div>
            </div>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}
