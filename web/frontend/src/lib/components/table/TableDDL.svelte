<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import { codemirror } from '../../actions/codemirror';
  import Icon from '../common/Icon.svelte';

  interface Props {
    tableName: string;
  }

  let { tableName }: Props = $props();

  let ddl = $state<string>('');
  let isLoading = $state<boolean>(true);
  let error = $state<string | null>(null);
  let copied = $state<boolean>(false);

  $effect(() => {
    if (tableName && appState.activeSessionId) {
      loadDDL();
    }
  });

  async function loadDDL() {
    isLoading = true;
    error = null;
    try {
      const db = appState.activeTab?.database || appState.currentDatabase || undefined;
      const res = await api.getTableDDL(appState.activeSessionId!, tableName, db);
      ddl = res.ddl || '';
    } catch (err: any) {
      error = err.message || 'Failed to fetch table DDL';
    } finally {
      isLoading = false;
    }
  }

  function copyDDL() {
    navigator.clipboard.writeText(ddl);
    copied = true;
    setTimeout(() => (copied = false), 2000);
  }
</script>

<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text font-mono text-xs p-4">
  <div class="flex items-center justify-between pb-3">
    <div class="flex items-center space-x-2 text-slate-300 font-sans font-semibold">
      <Icon name="file-code" class="w-4 h-4 text-sky-400" />
      <span>CREATE TABLE DDL for {tableName}</span>
    </div>
    <button
      onclick={copyDDL}
      class="px-3 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-200 border border-[#334155] rounded text-xs flex items-center space-x-1.5 transition"
    >
      <Icon name="copy" class="w-3.5 h-3.5 text-sky-400" />
      <span>{copied ? 'Copied!' : 'Copy DDL'}</span>
    </button>
  </div>

  <div class="flex-1 bg-[#0b0f19] border border-[#1e293b] rounded-xl p-4 overflow-auto shadow-inner">
    {#if isLoading}
      <div class="flex items-center justify-center p-12 text-slate-400 space-x-2">
        <Icon name="refresh-cw" class="w-4 h-4 animate-spin text-sky-400" />
        <span>Fetching CREATE TABLE syntax...</span>
      </div>
    {:else if error}
      <div class="p-4 text-rose-400 bg-rose-950/40 border border-rose-800 rounded">
        {error}
      </div>
    {:else}
      <div
        class="h-full w-full overflow-hidden"
        use:codemirror={{
          doc: ddl,
          dialect: appState.activeConnection?.driver,
          readOnly: true,
        }}
      ></div>
    {/if}
  </div>
</div>
