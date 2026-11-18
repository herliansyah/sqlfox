<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { MYSQL_CHARSET_COLLATIONS } from '../../utils/dataTypes';
  import { quoteSqlIdentifier } from '../../utils/sqlStatements';
  import Icon from '../common/Icon.svelte';

  let dbName = $state('');
  let charset = $state('utf8mb4');
  let collation = $state('utf8mb4_unicode_ci');
  let isSubmitting = $state(false);

  const charsetNames = Object.keys(MYSQL_CHARSET_COLLATIONS);
  let availableCollations = $derived(charset ? MYSQL_CHARSET_COLLATIONS[charset] || [] : []);
  let driver = $derived(appState.activeConnection?.driver?.toLowerCase() || 'mysql');
  let isSqlite = $derived(driver === 'sqlite');
  let isPostgres = $derived(driver === 'postgres' || driver === 'postgresql');

  let wasOpen = false;

  $effect(() => {
    if (appState.isCreateDbModalOpen) {
      if (!wasOpen) {
        dbName = '';
        charset = 'utf8mb4';
        collation = 'utf8mb4_unicode_ci';
        isSubmitting = false;
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  function onCharsetChange(newCharset: string) {
    charset = newCharset;
    const list = newCharset ? MYSQL_CHARSET_COLLATIONS[newCharset] || [] : [];
    collation = list[0] || '';
  }

  let generatedDDL = $derived.by(() => {
    if (isSqlite) {
      return '-- SQLite stores each connection as a single database file on disk.';
    }
    const name = dbName.trim() || 'new_database';
    const qDb = quoteSqlIdentifier(name, driver);
    if (isPostgres) {
      return `CREATE DATABASE ${qDb};`;
    }
    if (!charset) {
      return `CREATE DATABASE ${qDb};`;
    }
    return collation
      ? `CREATE DATABASE ${qDb}\n  CHARACTER SET ${charset}\n  COLLATE ${collation};`
      : `CREATE DATABASE ${qDb}\n  CHARACTER SET ${charset};`;
  });

  async function handleCreate(e: Event) {
    e.preventDefault();
    if (isSqlite || !dbName.trim()) return;
    isSubmitting = true;
    try {
      await appState.createDatabase(dbName.trim(), charset, collation);
      appState.isCreateDbModalOpen = false;
    } finally {
      isSubmitting = false;
    }
  }
</script>

{#if appState.isCreateDbModalOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 select-none"
    role="dialog"
    aria-modal="true"
  >
    <button
      tabindex="-1"
      aria-label="Close"
      class="fixed inset-0 bg-transparent cursor-default"
      onclick={() => (appState.isCreateDbModalOpen = false)}
    ></button>

    <div class="w-full max-w-md bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col relative z-10 animate-in fade-in zoom-in-95 duration-150">
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between">
        <div class="flex items-center space-x-2.5">
          <Icon name="database" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-sm text-slate-100">Create New Database</h3>
        </div>
        <button
          onclick={() => (appState.isCreateDbModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Form Body -->
      <form onsubmit={handleCreate} class="p-5 space-y-4 text-xs font-sans">
        {#if isSqlite}
          <div class="p-3.5 rounded-lg bg-amber-950/30 border border-amber-800/60 text-amber-200 space-y-1.5">
            <div class="flex items-center space-x-2 font-bold text-xs">
              <Icon name="info" class="w-4 h-4 text-amber-400 shrink-0" />
              <span>SQLite Single-File Architecture</span>
            </div>
            <p class="text-[11px] text-amber-300/90 leading-relaxed">
              SQLite stores an entire database inside a single <code class="font-mono">.db</code> file on disk and does not support creating multiple schemas with <code class="font-mono">CREATE DATABASE</code>. Create a new SQLite Connection Profile to point to another file.
            </p>
          </div>
        {:else}
          <!-- DB Name -->
          <div class="space-y-1.5">
            <label for="new-db-name" class="block font-semibold text-slate-300">
              Database Name <span class="text-rose-400">*</span>
            </label>
            <input
              id="new-db-name"
              type="text"
              required
              placeholder="e.g. ecommerce_production"
              bind:value={dbName}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-2 text-slate-100 font-mono text-xs outline-none transition"
            />
            <span class="text-[10px] text-slate-500">Identifier will be quoted automatically in the DDL review modal before executing.</span>
          </div>

          {#if !isPostgres}
            <!-- Charset & Collation -->
            <div class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <label for="db-charset" class="block font-semibold text-slate-300">Character Set</label>
                <select
                  id="db-charset"
                  value={charset}
                  onchange={(e) => onCharsetChange((e.target as HTMLSelectElement).value)}
                  class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-2 text-slate-100 font-mono text-xs outline-none"
                >
                  <option value="">(Server Default)</option>
                  {#each charsetNames as cs}
                    <option value={cs}>{cs}{cs === 'utf8mb4' ? ' (Recommended)' : ''}</option>
                  {/each}
                </select>
              </div>

              <div class="space-y-1.5">
                <label for="db-collation" class="block font-semibold text-slate-300">Collation</label>
                <select
                  id="db-collation"
                  bind:value={collation}
                  disabled={!charset}
                  class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-2 text-slate-100 font-mono text-xs outline-none disabled:opacity-50"
                >
                  {#if !charset}
                    <option value="">(Server Default)</option>
                  {:else}
                    {#each availableCollations as col}
                      <option value={col}>{col}</option>
                    {/each}
                  {/if}
                </select>
              </div>
            </div>
          {/if}
        {/if}

        <!-- SQL DDL Preview -->
        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="text-[10px] font-bold uppercase tracking-wider text-slate-400">SQL Preview</span>
            {#if !isSqlite}
              <span class="text-[10px] text-slate-500">Opens DDL Review before execution</span>
            {/if}
          </div>
          <pre class="bg-[#090d16] border border-slate-800 rounded p-2.5 font-mono text-[11px] text-sky-300 overflow-x-auto select-all leading-relaxed">{generatedDDL}</pre>
        </div>

        <!-- Actions -->
        <div class="pt-3 border-t border-[#1e293b] flex items-center justify-end space-x-2">
          <button
            type="button"
            onclick={() => (appState.isCreateDbModalOpen = false)}
            class="px-4 py-2 bg-[#1e293b] hover:bg-slate-700 text-slate-300 rounded font-semibold text-xs transition"
          >
            Cancel
          </button>
          {#if !isSqlite}
            <button
              type="submit"
              disabled={isSubmitting || !dbName.trim()}
              class="px-4 py-2 bg-sky-500 hover:bg-sky-400 disabled:opacity-50 text-slate-950 font-bold rounded text-xs transition flex items-center space-x-1.5 shadow-md shadow-sky-500/20"
            >
              {#if isSubmitting}
                <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
                <span>Preparing...</span>
              {:else}
                <Icon name="plus" class="w-3.5 h-3.5" />
                <span>Review &amp; Create</span>
              {/if}
            </button>
          {/if}
        </div>
      </form>
    </div>
  </div>
{/if}
