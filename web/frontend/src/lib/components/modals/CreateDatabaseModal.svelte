<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let dbName = $state('');
  let charset = $state('utf8mb4');
  let collation = $state('utf8mb4_unicode_ci');
  let isSubmitting = $state(false);

  const charsetCollations: Record<string, string[]> = {
    utf8mb4: [
      'utf8mb4_unicode_ci',
      'utf8mb4_general_ci',
      'utf8mb4_0900_ai_ci',
      'utf8mb4_bin',
    ],
    utf8: ['utf8_general_ci', 'utf8_unicode_ci', 'utf8_bin'],
    latin1: ['latin1_swedish_ci', 'latin1_general_ci', 'latin1_bin'],
    ascii: ['ascii_general_ci', 'ascii_bin'],
  };

  let availableCollations = $derived(charsetCollations[charset] || ['utf8mb4_unicode_ci']);

  $effect(() => {
    if (appState.isCreateDbModalOpen) {
      dbName = '';
      charset = 'utf8mb4';
      collation = 'utf8mb4_unicode_ci';
      isSubmitting = false;
    }
  });

  function onCharsetChange(newCharset: string) {
    charset = newCharset;
    const list = charsetCollations[newCharset] || [];
    collation = list[0] || '';
  }

  let generatedDDL = $derived.by(() => {
    const name = dbName.trim() || 'new_database';
    const driver = appState.activeConnection?.driver;
    if (driver === 'postgres') {
      return `CREATE DATABASE "${name}";`;
    }
    return `CREATE DATABASE \`${name}\`\n  CHARACTER SET ${charset}\n  COLLATE ${collation};`;
  });

  async function handleCreate(e: Event) {
    e.preventDefault();
    if (!dbName.trim()) return;
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
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm select-none"
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
        <!-- DB Name -->
        <div class="space-y-1.5">
          <label for="new-db-name" class="block font-semibold text-slate-300">
            Database Name <span class="text-rose-400">*</span>
          </label>
          <input
            id="new-db-name"
            type="text"
            required
            pattern="^[a-zA-Z0-9_-]+$"
            placeholder="e.g. ecommerce_production"
            bind:value={dbName}
            class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-3 py-2 text-slate-100 font-mono text-xs outline-none transition"
          />
          <span class="text-[10px] text-slate-500">Only letters, numbers, underscores, and dashes.</span>
        </div>

        {#if appState.activeConnection?.driver !== 'postgres'}
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
                <option value="utf8mb4">utf8mb4 (Recommended)</option>
                <option value="utf8">utf8</option>
                <option value="latin1">latin1</option>
                <option value="ascii">ascii</option>
              </select>
            </div>

            <div class="space-y-1.5">
              <label for="db-collation" class="block font-semibold text-slate-300">Collation</label>
              <select
                id="db-collation"
                bind:value={collation}
                class="w-full bg-[#090d16] border border-[#334155] focus:border-sky-500 rounded px-2.5 py-2 text-slate-100 font-mono text-xs outline-none"
              >
                {#each availableCollations as col}
                  <option value={col}>{col}</option>
                {/each}
              </select>
            </div>
          </div>
        {/if}

        <!-- SQL DDL Preview -->
        <div class="space-y-1.5">
          <span class="text-[10px] font-bold uppercase tracking-wider text-slate-400">SQL Preview</span>
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
          <button
            type="submit"
            disabled={isSubmitting || !dbName.trim()}
            class="px-4 py-2 bg-sky-500 hover:bg-sky-400 disabled:opacity-50 text-slate-950 font-bold rounded text-xs transition flex items-center space-x-1.5 shadow-md shadow-sky-500/20"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Creating...</span>
            {:else}
              <Icon name="plus" class="w-3.5 h-3.5" />
              <span>Create Database</span>
            {/if}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
