<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let sourceConnId = $state('');
  let sourceDb = $state('');
  let targetConnId = $state('');
  let targetDb = $state('');
  let includeDrops = $state(false);

  let isComparing = $state(false);
  let isApplying = $state(false);
  let hasCompared = $state(false);

  interface DiffItem {
    id: string;
    type: string;
    table: string;
    column?: string;
    index?: string;
    action: string;
    source_def?: string;
    target_def?: string;
    sql: string;
    selected: boolean;
  }

  let diffs = $state<DiffItem[]>([]);
  let viewMode = $state<'list' | 'script'>('list');
  let challengeInput = $state('');

  $effect(() => {
    if (appState.isStructureSyncModalOpen) {
      sourceConnId = appState.activeConnection?.id || '';
      sourceDb = appState.currentDatabase || '';
      targetConnId = '';
      targetDb = '';
      includeDrops = false;
      diffs = [];
      hasCompared = false;
      viewMode = 'list';
      challengeInput = '';
    }
  });

  let targetConn = $derived(appState.connections.find((c) => c.id === targetConnId));
  let isTargetProduction = $derived(targetConn?.environment === 'Production');
  let challengeTarget = $derived(targetDb || targetConn?.name || 'MIGRATE');

  let selectedCount = $derived(diffs.filter((d) => d.selected).length);

  let migrationScript = $derived(
    diffs
      .filter((d) => d.selected && d.sql)
      .map((d) => d.sql)
      .join('\n\n')
  );

  function toggleAllDiffs(checked: boolean) {
    diffs.forEach((d) => (d.selected = checked));
  }

  async function handleCompare() {
    if (!sourceConnId || !targetConnId) {
      appState.showToast('Please select both source and target connections', 'error');
      return;
    }
    if (sourceConnId === targetConnId && sourceDb === targetDb && !sourceDb) {
      appState.showToast('Source and target cannot be identical without separate database names', 'warning');
      return;
    }

    isComparing = true;
    try {
      // Ensure target session is connected
      const res = await fetch('/api/sync/structure/compare', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          source_session_id: sourceConnId,
          source_db: sourceDb,
          target_session_id: targetConnId,
          target_db: targetDb,
          include_drops: includeDrops,
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        appState.showToast(`Comparison failed: ${data.error || res.statusText}`, 'error');
        return;
      }

      diffs = (data.diffs || []).map((d: any, idx: number) => ({
        ...d,
        id: d.id || `diff-${idx}`,
        selected: d.selected !== false,
      }));
      hasCompared = true;
      if (diffs.length === 0) {
        appState.showToast('Schemas are completely identical! Zero differences found.', 'success');
      } else {
        appState.showToast(`Found ${diffs.length} structural difference(s)`, 'info');
      }
    } catch (err: any) {
      appState.showToast(`Error comparing schemas: ${err.message}`, 'error');
    } finally {
      isComparing = false;
    }
  }

  async function handleApply() {
    const selectedSQLs = diffs.filter((d) => d.selected && d.sql).map((d) => d.sql);
    if (selectedSQLs.length === 0) {
      appState.showToast('No schema changes selected for migration', 'warning');
      return;
    }

    if (isTargetProduction && challengeInput.trim() !== challengeTarget) {
      appState.showToast(`Please type "${challengeTarget}" to confirm production migration`, 'error');
      return;
    }

    if (!confirm(`Apply ${selectedSQLs.length} schema change(s) to target database?`)) return;

    isApplying = true;
    try {
      const res = await fetch('/api/sync/structure/apply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          target_session_id: targetConnId,
          target_db: targetDb,
          statements: selectedSQLs,
          confirmed: true,
          challenge_input: challengeInput.trim(),
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        appState.showToast(`Migration apply failed: ${data.error || res.statusText}`, 'error');
        return;
      }

      appState.showToast('Structure migration applied successfully!', 'success');
      appState.isStructureSyncModalOpen = false;
      await appState.refreshCatalog();
    } catch (err: any) {
      appState.showToast(`Error: ${err.message}`, 'error');
    } finally {
      isApplying = false;
    }
  }

  function handleClose() {
    appState.isStructureSyncModalOpen = false;
  }
</script>

{#if appState.isStructureSyncModalOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="sync-modal-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-4xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
            <Icon name="refresh-cw" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="sync-modal-title" class="font-bold text-sm text-slate-100 font-sans">
              Structure Synchronization Engine
            </h3>
            <p class="text-[11px] text-slate-400 font-sans">
              Compare schema differences between connections and generate non-destructive ALTER scripts
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

      <!-- Config Header -->
      <div class="p-4 bg-[#090d16] border-b border-[#1e293b] grid grid-cols-1 md:grid-cols-2 gap-4 text-xs font-sans">
        <!-- Source Box -->
        <div class="bg-[#0f172a] border border-[#1e293b] p-3 rounded-lg space-y-2">
          <div class="flex items-center space-x-1.5 font-bold text-slate-300">
            <span class="w-2 h-2 rounded-full bg-sky-400"></span>
            <span>Source (Reference Schema)</span>
          </div>

          <div class="grid grid-cols-2 gap-2">
            <div>
              <label for="sync-src-conn" class="text-[11px] text-slate-400 block mb-0.5">Connection</label>
              <select
                id="sync-src-conn"
                bind:value={sourceConnId}
                class="w-full bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-xs text-white font-mono outline-none"
              >
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.environment})</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="sync-src-db" class="text-[11px] text-slate-400 block mb-0.5">Database Name</label>
              <input
                id="sync-src-db"
                type="text"
                bind:value={sourceDb}
                placeholder="Database"
                class="w-full bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-xs text-white font-mono outline-none"
              />
            </div>
          </div>
        </div>

        <!-- Target Box -->
        <div class="bg-[#0f172a] border border-[#1e293b] p-3 rounded-lg space-y-2">
          <div class="flex items-center justify-between">
            <div class="flex items-center space-x-1.5 font-bold text-slate-300">
              <span class="w-2 h-2 rounded-full {isTargetProduction ? 'bg-rose-400' : 'bg-emerald-400'}"></span>
              <span>Target (Database to Modify)</span>
            </div>
            {#if isTargetProduction}
              <span class="px-1.5 py-0.2 rounded bg-rose-950 text-rose-400 text-[10px] font-bold border border-rose-800">PRODUCTION</span>
            {/if}
          </div>

          <div class="grid grid-cols-2 gap-2">
            <div>
              <label for="sync-tgt-conn" class="text-[11px] text-slate-400 block mb-0.5">Connection</label>
              <select
                id="sync-tgt-conn"
                bind:value={targetConnId}
                class="w-full bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-xs text-white font-mono outline-none"
              >
                <option value="">Select Target Connection...</option>
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.environment})</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="sync-tgt-db" class="text-[11px] text-slate-400 block mb-0.5">Database Name</label>
              <input
                id="sync-tgt-db"
                type="text"
                bind:value={targetDb}
                placeholder="Target DB"
                class="w-full bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-xs text-white font-mono outline-none"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Compare Trigger Bar -->
      <div class="px-4 py-2 bg-[#0b0f19] border-b border-[#1e293b] flex items-center justify-between text-xs font-sans">
        <label class="flex items-center space-x-2 text-slate-300 cursor-pointer select-none">
          <input
            type="checkbox"
            bind:checked={includeDrops}
            class="rounded bg-slate-900 border-slate-700 text-rose-500 focus:ring-0"
          />
          <span>Include DROP TABLE for extra tables on target</span>
        </label>

        <button
          onclick={handleCompare}
          disabled={isComparing || !sourceConnId || !targetConnId}
          class="px-4 py-1.5 bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white rounded font-semibold flex items-center space-x-1.5 transition shadow"
        >
          {#if isComparing}
            <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
            <span>Comparing Schemas...</span>
          {:else}
            <Icon name="search" class="w-3.5 h-3.5" />
            <span>Compare Schemas</span>
          {/if}
        </button>
      </div>

      <!-- Diff Results View -->
      <div class="flex-1 overflow-hidden flex flex-col">
        {#if !hasCompared}
          <div class="flex-1 flex flex-col items-center justify-center p-8 text-center text-slate-500 space-y-2">
            <Icon name="layers" class="w-10 h-10 text-slate-600" />
            <p class="font-sans text-xs">Select source and target databases, then click "Compare Schemas"</p>
          </div>
        {:else}
          <!-- Sub-Tabs: List vs Script -->
          <div class="h-8 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between text-xs font-sans">
            <div class="flex items-center space-x-2">
              <button
                onclick={() => (viewMode = 'list')}
                class="px-2.5 py-1 rounded font-semibold transition {viewMode === 'list' ? 'bg-[#1e293b] text-emerald-400' : 'text-slate-400 hover:text-white'}"
              >
                Differences ({diffs.length})
              </button>
              <button
                onclick={() => (viewMode = 'script')}
                class="px-2.5 py-1 rounded font-semibold transition {viewMode === 'script' ? 'bg-[#1e293b] text-emerald-400' : 'text-slate-400 hover:text-white'}"
              >
                Generated Migration Script
              </button>
            </div>

            <span class="text-slate-400 font-mono text-[11px]">
              {selectedCount} of {diffs.length} statements selected
            </span>
          </div>

          <!-- Body -->
          <div class="flex-1 overflow-auto">
            {#if viewMode === 'list'}
              <table class="w-full text-left border-collapse text-xs font-sans">
                <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-400 text-[10.5px] uppercase tracking-wider sticky top-0 z-10">
                  <tr>
                    <th class="py-2 px-3 w-8 text-center">
                      <input
                        type="checkbox"
                        checked={diffs.length > 0 && selectedCount === diffs.length}
                        onchange={(e) => toggleAllDiffs((e.target as HTMLInputElement).checked)}
                        class="rounded bg-slate-900 border-slate-700 text-emerald-500 focus:ring-0"
                      />
                    </th>
                    <th class="py-2 px-3">Object</th>
                    <th class="py-2 px-3">Action</th>
                    <th class="py-2 px-3">Source Definition</th>
                    <th class="py-2 px-3">Target Definition</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[#1e293b]/60 font-mono text-[11px]">
                  {#if diffs.length === 0}
                    <tr>
                      <td colspan="5" class="p-8 text-center text-emerald-400 font-sans italic">
                        Zero differences detected. Both database schemas are perfectly synchronized!
                      </td>
                    </tr>
                  {:else}
                    {#each diffs as diff (diff.id)}
                      <tr class="hover:bg-slate-800/40 transition-colors">
                        <td class="py-2 px-3 text-center">
                          <input
                            type="checkbox"
                            bind:checked={diff.selected}
                            class="rounded bg-slate-900 border-slate-700 text-emerald-500 focus:ring-0"
                          />
                        </td>
                        <td class="py-2 px-3 font-bold text-white">
                          {diff.column ? `${diff.table}.${diff.column}` : diff.index ? `${diff.table}.${diff.index}` : diff.table}
                        </td>
                        <td class="py-2 px-3">
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-bold {
                            diff.action.startsWith('CREATE') || diff.action.startsWith('ADD')
                              ? 'bg-emerald-950 text-emerald-400 border border-emerald-800'
                              : diff.action.startsWith('MODIFY')
                              ? 'bg-amber-950 text-amber-400 border border-amber-800'
                              : 'bg-rose-950 text-rose-400 border border-rose-800'
                          }">
                            {diff.action}
                          </span>
                        </td>
                        <td class="py-2 px-3 text-sky-300 max-w-xs truncate" title={diff.source_def}>
                          {diff.source_def || '-'}
                        </td>
                        <td class="py-2 px-3 text-slate-400 max-w-xs truncate" title={diff.target_def}>
                          {diff.target_def || '-'}
                        </td>
                      </tr>
                    {/each}
                  {/if}
                </tbody>
              </table>
            {:else}
              <div class="h-full p-4 flex flex-col">
                <textarea
                  readonly
                  value={migrationScript}
                  class="flex-1 w-full bg-[#090d16] border border-[#1e293b] rounded-lg p-3 text-xs font-mono text-emerald-300 outline-none resize-none"
                  placeholder="No statements selected"
                ></textarea>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer & Production Challenge -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0 font-sans">
        <div>
          {#if isTargetProduction && selectedCount > 0}
            <div class="flex items-center space-x-2 text-xs">
              <span class="text-rose-400 font-bold">Type "{challengeTarget}" to confirm:</span>
              <input
                type="text"
                bind:value={challengeInput}
                placeholder={challengeTarget}
                class="bg-[#090d16] border border-rose-800/80 rounded px-2 py-1 text-xs text-white font-mono outline-none focus:border-rose-500 w-36"
              />
            </div>
          {/if}
        </div>

        <div class="flex items-center space-x-2">
          <button
            onclick={handleClose}
            type="button"
            class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition"
          >
            Close
          </button>
          <button
            onclick={handleApply}
            disabled={isApplying || selectedCount === 0 || (isTargetProduction && challengeInput.trim() !== challengeTarget)}
            type="button"
            class="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow-lg shadow-emerald-950/40"
          >
            {#if isApplying}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Applying Migration...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Apply {selectedCount} Selected Change(s)</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
