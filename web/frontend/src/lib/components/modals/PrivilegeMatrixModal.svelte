<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface DbUser {
    user: string;
    host: string;
  }

  let users = $state<DbUser[]>([]);
  let selectedUser = $state<DbUser | null>(null);
  let isLoading = $state(false);
  let isSaving = $state(false);
  let grantsRaw = $state<string[]>([]);

  const ALL_PRIVS = [
    'SELECT',
    'INSERT',
    'UPDATE',
    'DELETE',
    'CREATE',
    'DROP',
    'ALTER',
    'INDEX',
    'EXECUTE',
    'CREATE VIEW',
    'SHOW VIEW',
    'TRIGGER',
  ];

  let selectedPrivileges = $state<Set<string>>(new Set());
  let targetDbScope = $state<'current' | 'all'>('current');

  $effect(() => {
    if (appState.isPrivilegeMatrixOpen && appState.activeSessionId) {
      loadUsers();
    }
  });

  async function loadUsers() {
    isLoading = true;
    try {
      const res = await api.listUsers(appState.activeSessionId!);
      users = res.users || (Array.isArray(res) ? res : []);
      if (users.length > 0 && !selectedUser) {
        selectUser(users[0]);
      }
    } catch (err: any) {
      appState.showToast(`Failed to load DB users: ${err.message}`, 'error');
    } finally {
      isLoading = false;
    }
  }

  async function selectUser(u: DbUser) {
    selectedUser = u;
    selectedPrivileges.clear();
    try {
      const res = await api.getUserGrants(appState.activeSessionId!, u.user, u.host);
      grantsRaw = res.grants || [];

      // Parse active privs
      const grantText = grantsRaw.join(' ');
      if (grantText.includes('ALL PRIVILEGES')) {
        ALL_PRIVS.forEach((p) => selectedPrivileges.add(p));
      } else {
        ALL_PRIVS.forEach((p) => {
          if (new RegExp(`\\b${p}\\b`, 'i').test(grantText)) {
            selectedPrivileges.add(p);
          }
        });
      }
    } catch (err: any) {
      grantsRaw = [`Error inspecting grants: ${err.message}`];
    }
  }

  function togglePrivilege(priv: string) {
    if (selectedPrivileges.has(priv)) {
      selectedPrivileges.delete(priv);
    } else {
      selectedPrivileges.add(priv);
    }
  }

  function toggleAllPrivs(checked: boolean) {
    if (checked) {
      ALL_PRIVS.forEach((p) => selectedPrivileges.add(p));
    } else {
      selectedPrivileges.clear();
    }
  }

  async function handleApplyGrants() {
    if (!selectedUser || !appState.activeSessionId) return;

    const privList = Array.from(selectedPrivileges);
    const scope = targetDbScope === 'current' && appState.currentDatabase
      ? `\`${appState.currentDatabase}\`.*`
      : '*.*';

    const privString = privList.length === ALL_PRIVS.length
      ? 'ALL PRIVILEGES'
      : privList.length > 0
      ? privList.join(', ')
      : 'USAGE';

    const sql = `GRANT ${privString} ON ${scope} TO '${selectedUser.user}'@'${selectedUser.host}'; FLUSH PRIVILEGES;`;

    if (!confirm(`Apply grant updates for '${selectedUser.user}'@'${selectedUser.host}'?\n\n${sql}`)) return;

    isSaving = true;
    try {
      await api.executeQuery(appState.activeSessionId, sql, false);
      appState.showToast(`Updated privileges for ${selectedUser.user}`, 'success');
      await selectUser(selectedUser);
    } catch (err: any) {
      appState.showToast(`Failed to update grants: ${err.message}`, 'error');
    } finally {
      isSaving = false;
    }
  }

  function handleClose() {
    appState.isPrivilegeMatrixOpen = false;
  }
</script>

{#if appState.isPrivilegeMatrixOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 font-sans">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="priv-modal-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-3xl flex flex-col max-h-[88vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-purple-500/10 border border-purple-500/30 text-purple-400">
            <Icon name="shield" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="priv-modal-title" class="font-bold text-sm text-slate-100">
              Database Users & Privilege Matrix
            </h3>
            <p class="text-[11px] text-slate-400">
              Visual access control manager and GRANT/REVOKE permission matrix
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

      <!-- Dialog Body -->
      <div class="flex-1 overflow-hidden flex flex-col md:flex-row divide-y md:divide-y-0 md:divide-x divide-[#1e293b]">
        <!-- Left: Users List -->
        <div class="w-full md:w-56 bg-[#090d16] flex flex-col overflow-y-auto shrink-0 p-2 space-y-1">
          <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500 px-2 py-1 block">
            Database Users ({users.length})
          </span>

          {#if isLoading}
            <div class="p-4 text-center text-xs text-slate-500">Loading users...</div>
          {:else}
            {#each users as u}
              {@const isSelected = selectedUser?.user === u.user && selectedUser?.host === u.host}
              <button
                onclick={() => selectUser(u)}
                class="w-full text-left px-2.5 py-1.5 rounded text-xs flex items-center justify-between font-mono transition {isSelected ? 'bg-purple-950/60 text-purple-300 border border-purple-800/60 font-bold' : 'text-slate-300 hover:bg-slate-800'}"
              >
                <span class="truncate">{u.user}</span>
                <span class="text-[10px] text-slate-500">@{u.host}</span>
              </button>
            {/each}
          {/if}
        </div>

        <!-- Right: Privileges Matrix & DDL -->
        <div class="flex-1 overflow-y-auto p-4 space-y-4 text-xs font-sans">
          {#if selectedUser}
            <div class="flex items-center justify-between bg-[#0f172a] p-3 rounded-lg border border-[#1e293b]">
              <div>
                <span class="text-[10px] text-slate-500 uppercase font-bold tracking-wider block">Selected User</span>
                <span class="font-bold text-slate-100 font-mono text-sm">
                  '{selectedUser.user}'@'{selectedUser.host}'
                </span>
              </div>

              <!-- Scope Toggle -->
              <div class="flex items-center space-x-2 text-xs">
                <span class="text-slate-400">Target Scope:</span>
                <select
                  bind:value={targetDbScope}
                  class="bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-white font-mono text-xs outline-none"
                >
                  <option value="current">Current Database ({appState.currentDatabase || '*'}.*)</option>
                  <option value="all">Global (*.*)</option>
                </select>
              </div>
            </div>

            <!-- Privilege Matrix Checkbox Grid -->
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="font-bold text-slate-300">Privilege Matrix</span>
                <label class="flex items-center space-x-1.5 text-slate-400 text-xs cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={selectedPrivileges.size === ALL_PRIVS.length}
                    onchange={(e) => toggleAllPrivs((e.target as HTMLInputElement).checked)}
                    class="rounded bg-slate-900 border-slate-700 text-purple-500 focus:ring-0"
                  />
                  <span>Select All</span>
                </label>
              </div>

              <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
                {#each ALL_PRIVS as priv}
                  {@const isChecked = selectedPrivileges.has(priv)}
                  <label class="flex items-center space-x-2 p-2 rounded border border-[#1e293b] hover:border-slate-700 cursor-pointer select-none {isChecked ? 'bg-purple-950/20 border-purple-800/40 text-purple-300' : 'bg-[#090d16] text-slate-300'}">
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onchange={() => togglePrivilege(priv)}
                      class="rounded bg-slate-900 border-slate-700 text-purple-500 focus:ring-0"
                    />
                    <span class="font-mono text-[11px] font-semibold">{priv}</span>
                  </label>
                {/each}
              </div>
            </div>

            <!-- Existing Raw SHOW GRANTS -->
            <div class="space-y-1">
              <span class="font-bold text-slate-400 text-[11px] uppercase tracking-wider block">
                Active SHOW GRANTS Output
              </span>
              <pre class="p-2.5 rounded bg-[#090d16] border border-slate-800 text-[11px] text-slate-400 font-mono overflow-x-auto whitespace-pre-wrap max-h-24 select-text">{grantsRaw.join('\n')}</pre>
            </div>
          {:else}
            <div class="p-8 text-center text-slate-500 italic">Select a user to inspect permissions</div>
          {/if}
        </div>
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-end space-x-2 shrink-0">
        <button
          onclick={handleClose}
          type="button"
          class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition"
        >
          Close
        </button>
        <button
          onclick={handleApplyGrants}
          disabled={isSaving || !selectedUser}
          type="button"
          class="px-4 py-1.5 bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow"
        >
          {#if isSaving}
            <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
            <span>Updating Grants...</span>
          {:else}
            <Icon name="check" class="w-3.5 h-3.5" />
            <span>Apply Grants</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
