<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let sourceConnId = $state('');
  let sourceDb = $state('');
  let targetConnId = $state('');
  let targetDb = $state('');
  let selectedTable = $state('');
  let deleteMissing = $state(false);

  let srcDatabases = $state<string[]>([]);
  let tgtDatabases = $state<string[]>([]);
  let availableTables = $state<string[]>([]);

  let isComparing = $state(false);
  let isApplying = $state(false);
  let challengeRequired = $state(false);
  let challengeInput = $state('');
  let applyErrors = $state<string[]>([]);

  let syncDiff = $state<{
    table?: string;
    pk_columns?: string[];
    columns?: string[];
    summary?: Record<string, number>;
    operations?: Array<{ type: string; pk: any; old_data?: any[]; data?: any[] }>;
    error?: string;
  } | null>(null);

  let wasOpen = false;

  async function loadSourceMeta(connId: string, db: string) {
    if (!connId) return;
    try {
      const sess = await api.connectSession(connId);
      srcDatabases = sess.schema?.databases || [];
      const activeDb = db || sess.current_database || sess.schema?.database_name || '';
      if (!sourceDb && activeDb) sourceDb = activeDb;
      const schema = activeDb ? await api.getSchema(sess.id, activeDb) : sess.schema;
      availableTables = (schema?.tables || []).filter((t) => t.type !== 'VIEW').map((t) => t.name);
      if (!selectedTable || !availableTables.includes(selectedTable)) {
        const preferred = appState.activeTab?.tableName;
        selectedTable = preferred && availableTables.includes(preferred) ? preferred : (availableTables[0] || '');
      }
    } catch {
      availableTables = [];
    }
  }

  async function loadTargetMeta(connId: string) {
    if (!connId) {
      tgtDatabases = [];
      return;
    }
    try {
      const sess = await api.connectSession(connId);
      tgtDatabases = sess.schema?.databases || [];
      if (!targetDb && (sess.current_database || sess.schema?.database_name)) {
        targetDb = sess.current_database || sess.schema?.database_name || '';
      }
    } catch {
      tgtDatabases = [];
    }
  }

  $effect(() => {
    if (appState.isDataSyncOpen) {
      if (!wasOpen) {
        sourceConnId = appState.activeConnection?.id || '';
        sourceDb = appState.currentDatabase || '';
        targetConnId = '';
        targetDb = '';
        selectedTable = appState.activeTab?.tableName || '';
        deleteMissing = false;
        syncDiff = null;
        challengeRequired = false;
        challengeInput = '';
        applyErrors = [];
        wasOpen = true;
        if (sourceConnId) loadSourceMeta(sourceConnId, sourceDb);
      }
    } else {
      wasOpen = false;
    }
  });

  async function handleCompare() {
    if (!sourceConnId || !targetConnId || !selectedTable) {
      appState.showToast('Source connection, target connection, and table are required', 'warning');
      return;
    }

    isComparing = true;
    syncDiff = null;
    challengeRequired = false;
    challengeInput = '';
    applyErrors = [];
    try {
      const srcSession = (await api.connectSession(sourceConnId)).id || sourceConnId;
      const tgtSession = (await api.connectSession(targetConnId)).id || targetConnId;
      const res = await api.compareDataSync({
        source_session_id: srcSession,
        source_db: sourceDb,
        target_session_id: tgtSession,
        target_db: targetDb,
        table: selectedTable,
        delete_missing: deleteMissing,
      });
      syncDiff = res;
      appState.showToast('Row comparison completed', 'info');
    } catch (err: any) {
      syncDiff = { error: err.message || 'Comparison failed' };
      appState.showToast(`Sync comparison failed: ${err.message}`, 'error');
    } finally {
      isComparing = false;
    }
  }

  async function handleApply(confirmed = false) {
    if (!syncDiff?.operations || syncDiff.operations.length === 0) return;

    const targetTableName = syncDiff.table || selectedTable;
    const tgtConn = appState.connections.find((c) => c.id === targetConnId);
    const isProd = tgtConn?.environment === 'Production';

    if (isProd && !confirmed && !challengeRequired) {
      // Trigger challenge or show inline challenge box right away
      challengeRequired = true;
      return;
    }

    if (challengeRequired) {
      const clean = challengeInput.trim().toUpperCase();
      if (clean !== 'SYNC' && clean !== targetTableName.toUpperCase()) {
        appState.showToast(`Type "${targetTableName}" or "SYNC" to confirm Production Data Sync`, 'error');
        return;
      }
      confirmed = true;
    }

    isApplying = true;
    applyErrors = [];
    try {
      const tgtSession = (await api.connectSession(targetConnId)).id || targetConnId;
      const res = await api.applyDataSync({
        target_session_id: tgtSession,
        target_db: targetDb,
        table: targetTableName,
        columns: syncDiff.columns || [],
        operations: syncDiff.operations,
        confirmed,
        challenge_input: challengeInput.trim(),
      });

      if (res?.blocked && res?.requires_confirm) {
        challengeRequired = true;
        appState.showToast(res.message || 'Production confirmation challenge required', 'warning');
        return;
      }

      if (res?.errors && res.errors.length > 0) {
        applyErrors = res.errors;
        appState.showToast(
          `Sync finished with errors: ${res.executed ?? 0} applied, ${res.errors.length} failed`,
          'warning'
        );
        return;
      }

      appState.showToast(`Applied ${res?.executed ?? syncDiff.operations.length} synchronization operation(s)!`, 'success');
      syncDiff = null;
      appState.isDataSyncOpen = false;
    } catch (err: any) {
      appState.showToast(`Failed to apply sync: ${err.message}`, 'error');
    } finally {
      isApplying = false;
    }
  }

  function formatOpValues(op: { old_data?: any[]; data?: any[] }): string {
    const row = op.data || op.old_data;
    if (!row || !Array.isArray(row)) return '—';
    return row.slice(0, 5).map((v) => (v === null ? 'NULL' : String(v))).join(' | ');
  }

  function handleCancel() {
    if (isComparing || isApplying) return;
    appState.isDataSyncOpen = false;
  }
</script>

{#if appState.isDataSyncOpen}
  <div
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 p-4 animate-fade-in"
    role="presentation"
  >
    <div
      class="bg-[#0f172a] border border-slate-700 w-full max-w-3xl rounded-xl shadow-2xl flex flex-col overflow-hidden text-slate-100 max-h-[90vh]"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-slate-800 flex items-center justify-between bg-slate-900/60 shrink-0">
        <div class="flex items-center space-x-2">
          <Icon name="scale" class="w-4 h-4 text-emerald-400" />
          <h3 class="text-sm font-semibold text-slate-100">
            Row-Level Data Synchronization
          </h3>
        </div>
        <button
          type="button"
          onclick={handleCancel}
          disabled={isComparing || isApplying}
          class="text-slate-400 hover:text-slate-200 p-1 rounded hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 space-y-4 overflow-y-auto flex-1 text-xs">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <!-- Source -->
          <div class="p-3 rounded-lg border border-slate-800 bg-[#050811] space-y-2">
            <h4 class="font-bold text-sky-400 uppercase">Source (Master / Truth)</h4>
            <div>
              <label for="ds-src-conn" class="block text-slate-400 mb-1">Connection</label>
              <select
                id="ds-src-conn"
                bind:value={sourceConnId}
                onchange={() => { sourceDb = ''; selectedTable = ''; loadSourceMeta(sourceConnId, ''); }}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100"
              >
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="ds-src-db" class="block text-slate-400 mb-1">Database</label>
              {#if srcDatabases.length > 0}
                <select
                  id="ds-src-db"
                  bind:value={sourceDb}
                  onchange={() => loadSourceMeta(sourceConnId, sourceDb)}
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono"
                >
                  {#each srcDatabases as db}
                    <option value={db}>{db}</option>
                  {/each}
                </select>
              {:else}
                <input
                  id="ds-src-db"
                  type="text"
                  bind:value={sourceDb}
                  placeholder="Optional database"
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono"
                />
              {/if}
            </div>
          </div>

          <!-- Target -->
          <div class="p-3 rounded-lg border border-slate-800 bg-[#050811] space-y-2">
            <h4 class="font-bold text-emerald-400 uppercase">Target (To Reconcile)</h4>
            <div>
              <label for="ds-tgt-conn" class="block text-slate-400 mb-1">Connection</label>
              <select
                id="ds-tgt-conn"
                bind:value={targetConnId}
                onchange={() => { targetDb = ''; loadTargetMeta(targetConnId); }}
                class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100"
              >
                <option value="">Select Target Connection...</option>
                {#each appState.connections as conn}
                  <option value={conn.id}>{conn.name} ({conn.driver})</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="ds-tgt-db" class="block text-slate-400 mb-1">Database</label>
              {#if tgtDatabases.length > 0}
                <select
                  id="ds-tgt-db"
                  bind:value={targetDb}
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono"
                >
                  {#each tgtDatabases as db}
                    <option value={db}>{db}</option>
                  {/each}
                </select>
              {:else}
                <input
                  id="ds-tgt-db"
                  type="text"
                  bind:value={targetDb}
                  placeholder="Target DB"
                  class="w-full bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono"
                />
              {/if}
            </div>
          </div>
        </div>

        <!-- Unified Table & Options Bar -->
        <div class="p-3 rounded-lg border border-slate-800 bg-[#050811] flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center space-x-2 flex-1 min-w-[200px]">
            <label for="ds-table" class="text-slate-400 font-semibold shrink-0">Table to Reconcile *</label>
            <select
              id="ds-table"
              bind:value={selectedTable}
              class="flex-1 bg-slate-900 border border-slate-700 rounded px-2.5 py-1.5 text-slate-100 font-mono focus:outline-none focus:border-emerald-500"
            >
              {#each availableTables as tbl}
                <option value={tbl}>{tbl}</option>
              {/each}
            </select>
          </div>

          <label class="flex items-center space-x-2 text-rose-300 cursor-pointer select-none">
            <input
              type="checkbox"
              bind:checked={deleteMissing}
              class="rounded border-slate-700 bg-slate-900 text-rose-500 focus:ring-0"
            />
            <span>Delete target rows missing from source</span>
          </label>
        </div>

        {#if syncDiff}
          {#if syncDiff.error}
            <div class="p-4 rounded-lg border border-rose-500/40 bg-rose-950/30 text-xs text-rose-300">
              {syncDiff.error}
            </div>
          {:else}
            <div class="p-4 rounded-lg border border-slate-800 bg-[#050811] space-y-3">
              <div class="flex items-center justify-between">
                <h4 class="text-xs font-bold text-slate-200">Reconciliation Summary ({syncDiff.table || selectedTable})</h4>
                <span class="text-[11px] font-mono text-slate-400">
                  PK: {syncDiff.pk_columns?.join(', ') || 'auto'}
                </span>
              </div>
              <div class="grid grid-cols-3 gap-2 text-center text-xs">
                <div class="p-2 rounded bg-emerald-950/40 border border-emerald-800/60">
                  <div class="text-emerald-400 font-bold text-lg font-mono">{syncDiff.summary?.insert_count ?? 0}</div>
                  <div class="text-[11px] text-slate-400">Rows to Insert</div>
                </div>
                <div class="p-2 rounded bg-amber-950/40 border border-amber-800/60">
                  <div class="text-amber-400 font-bold text-lg font-mono">{syncDiff.summary?.update_count ?? 0}</div>
                  <div class="text-[11px] text-slate-400">Rows to Update</div>
                </div>
                <div class="p-2 rounded bg-rose-950/40 border border-rose-800/60">
                  <div class="text-rose-400 font-bold text-lg font-mono">{syncDiff.summary?.delete_count ?? 0}</div>
                  <div class="text-[11px] text-slate-400">Rows to Delete</div>
                </div>
              </div>

              {#if syncDiff.operations && syncDiff.operations.length > 0}
                <div class="space-y-1.5">
                  <div class="flex items-center justify-between text-[11px] text-slate-400">
                    <span>Row Diff Preview ({Math.min(100, syncDiff.operations.length)} of {syncDiff.operations.length} operations)</span>
                  </div>
                  <div class="max-h-48 overflow-y-auto border border-slate-800 rounded-lg">
                    <table class="w-full text-left border-collapse font-mono text-[11px]">
                      <thead class="bg-slate-900 border-b border-slate-800 text-slate-400 sticky top-0">
                        <tr>
                          <th class="py-1.5 px-2.5 w-20">Op</th>
                          <th class="py-1.5 px-2.5 w-32">PK</th>
                          <th class="py-1.5 px-2.5">Row Data Preview</th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-slate-800/60">
                        {#each syncDiff.operations.slice(0, 100) as op}
                          {@const opUp = String(op.type || '').toUpperCase()}
                          <tr class="hover:bg-slate-900/50">
                            <td class="py-1 px-2.5">
                              <span class="px-1.5 py-0.5 rounded text-[10px] font-bold {opUp === 'INSERT' ? 'bg-emerald-950 text-emerald-300 border border-emerald-800' : opUp === 'UPDATE' ? 'bg-amber-950 text-amber-300 border border-amber-800' : 'bg-rose-950 text-rose-300 border border-rose-800'}">
                                {opUp}
                              </span>
                            </td>
                            <td class="py-1 px-2.5 text-sky-300 truncate max-w-[140px]" title={JSON.stringify(op.pk)}>{JSON.stringify(op.pk)}</td>
                            <td class="py-1 px-2.5 text-slate-300 truncate max-w-md" title={formatOpValues(op)}>{formatOpValues(op)}</td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                </div>
              {:else}
                <div class="text-center p-3 text-xs text-slate-400">
                  Both tables are already in complete synchronization. No modifications needed.
                </div>
              {/if}
            </div>
          {/if}
        {/if}

        {#if challengeRequired}
          <div class="p-3 rounded-lg border border-rose-800 bg-rose-950/35 space-y-2">
            <div class="font-bold text-rose-300 flex items-center space-x-1.5">
              <Icon name="shield-alert" class="w-4 h-4 text-rose-400" />
              <span>Production Safety Guard: Type "{syncDiff?.table || selectedTable}" or "SYNC" to apply changes</span>
            </div>
            <input
              type="text"
              bind:value={challengeInput}
              placeholder={syncDiff?.table || selectedTable || 'SYNC'}
              class="w-full bg-slate-900 border border-rose-700 rounded px-3 py-1.5 text-xs text-white font-mono outline-none focus:border-rose-500"
            />
          </div>
        {/if}

        {#if applyErrors.length > 0}
          <div class="p-3 rounded-lg border border-rose-800 bg-rose-950/30 text-[11px] font-mono text-rose-300 max-h-24 overflow-y-auto space-y-0.5">
            {#each applyErrors as err}
              <div>{err}</div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 bg-slate-900/60 border-t border-slate-800 flex items-center justify-between shrink-0">
        <button
          type="button"
          onclick={handleCompare}
          disabled={isComparing || isApplying || !sourceConnId || !targetConnId || !selectedTable}
          class="px-4 py-1.5 rounded text-xs font-semibold bg-slate-800 hover:bg-slate-700 text-slate-200 transition disabled:opacity-50"
        >
          {isComparing ? 'Comparing Checksums...' : 'Compare Tables'}
        </button>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={handleCancel}
            disabled={isComparing || isApplying}
            class="px-3.5 py-1.5 rounded text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition disabled:opacity-50"
          >
            Cancel
          </button>

          {#if syncDiff?.operations && syncDiff.operations.length > 0}
            <button
              type="button"
              onclick={() => handleApply(false)}
              disabled={isApplying}
              class="px-4 py-1.5 rounded text-xs font-bold bg-emerald-600 hover:bg-emerald-500 text-white transition shadow flex items-center space-x-1 disabled:opacity-50"
            >
              {#if isApplying}
                <Icon name="loader" class="w-3.5 h-3.5 animate-spin" />
                <span>Applying...</span>
              {:else}
                <span>{challengeRequired ? 'Confirm & Apply Sync' : 'Apply Sync Changes'}</span>
              {/if}
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}
