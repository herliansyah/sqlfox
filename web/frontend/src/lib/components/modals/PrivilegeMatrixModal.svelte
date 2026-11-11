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
  let isCreatingNew = $state(false);
  let isLoading = $state(false);
  let isSaving = $state(false);
  let grantsRaw = $state<string[]>([]);

  // Account creation state
  let newUsername = $state('');
  let newHost = $state('%');
  let newPassword = $state('');

  const PRIV_GROUPS: Record<string, string[]> = {
    Data: ['SELECT', 'INSERT', 'UPDATE', 'DELETE', 'FILE'],
    Structure: [
      'CREATE',
      'ALTER',
      'INDEX',
      'DROP',
      'CREATE VIEW',
      'SHOW VIEW',
      'CREATE ROUTINE',
      'ALTER ROUTINE',
      'EXECUTE',
      'EVENT',
      'TRIGGER',
    ],
    Admin: [
      'GRANT OPTION',
      'SUPER',
      'PROCESS',
      'RELOAD',
      'SHUTDOWN',
      'SHOW DATABASES',
      'LOCK TABLES',
      'REPLICATION CLIENT',
      'REPLICATION SLAVE',
    ],
  };

  const ALL_PRIVS = Object.values(PRIV_GROUPS).flat();

  let selectedPrivileges = $state<Set<string>>(new Set());
  let targetDbScope = $state<'current' | 'all'>('current');
  let withGrantOption = $state(false);

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
      if (users.length > 0 && !selectedUser && !isCreatingNew) {
        selectUser(users[0]);
      }
    } catch (err: any) {
      appState.showToast(`Failed to load DB users: ${err.message}`, 'error');
    } finally {
      isLoading = false;
    }
  }

  async function selectUser(u: DbUser) {
    isCreatingNew = false;
    selectedUser = u;
    selectedPrivileges.clear();
    withGrantOption = false;

    try {
      const res = await api.getUserGrants(appState.activeSessionId!, u.user, u.host);
      grantsRaw = res.grants || [];

      // Parse active privs
      const grantText = grantsRaw.join(' ').toUpperCase();
      if (grantText.includes('ALL PRIVILEGES')) {
        ALL_PRIVS.forEach((p) => selectedPrivileges.add(p));
      } else {
        ALL_PRIVS.forEach((p) => {
          if (new RegExp(`\\b${p}\\b`, 'i').test(grantText)) {
            selectedPrivileges.add(p);
          }
        });
      }
      withGrantOption = grantText.includes('WITH GRANT OPTION');
    } catch (err: any) {
      grantsRaw = [`Error inspecting grants: ${err.message}`];
    }
  }

  function startNewUser() {
    isCreatingNew = true;
    selectedUser = null;
    newUsername = '';
    const isPG = appState.activeConnection?.driver?.toLowerCase().includes('postgres');
    newHost = isPG ? 'N/A' : '%';
    newPassword = '';
    selectedPrivileges.clear();
    withGrantOption = false;
    grantsRaw = [];
  }

  function togglePrivilege(priv: string) {
    if (selectedPrivileges.has(priv)) {
      selectedPrivileges.delete(priv);
    } else {
      selectedPrivileges.add(priv);
    }
  }

  function applyPreset(preset: 'all' | 'readonly' | 'readwrite' | 'none') {
    selectedPrivileges.clear();
    if (preset === 'all') {
      ALL_PRIVS.forEach((p) => selectedPrivileges.add(p));
    } else if (preset === 'readonly') {
      ['SELECT', 'SHOW VIEW'].forEach((p) => selectedPrivileges.add(p));
    } else if (preset === 'readwrite') {
      ['SELECT', 'INSERT', 'UPDATE', 'DELETE', 'CREATE VIEW', 'SHOW VIEW'].forEach((p) =>
        selectedPrivileges.add(p)
      );
    }
  }

  async function handleSaveNewUser() {
    if (!appState.activeSessionId) return;
    if (!newUsername.trim()) {
      appState.showToast('Username is required', 'warning');
      return;
    }

    isSaving = true;
    try {
      await api.saveUser(appState.activeSessionId, {
        user: newUsername.trim(),
        host: newHost.trim() || '%',
        password: newPassword,
        is_new: true,
      });

      appState.showToast(`User '${newUsername}' created successfully`, 'success');
      const createdUser = { user: newUsername.trim(), host: newHost.trim() || '%' };

      // Apply initial privileges if selected
      if (selectedPrivileges.size > 0) {
        await api.applyGrants(appState.activeSessionId, {
          user: createdUser.user,
          host: createdUser.host,
          scope: targetDbScope === 'current' ? 'DATABASE' : 'GLOBAL',
          database: targetDbScope === 'current' ? appState.currentDatabase || '' : undefined,
          privileges: Array.from(selectedPrivileges),
          with_grant: withGrantOption,
        });
      }

      await loadUsers();
      await selectUser(createdUser);
    } catch (err: any) {
      appState.showToast(`Failed to create user: ${err.message}`, 'error');
    } finally {
      isSaving = false;
    }
  }

  async function handleDeleteUser() {
    if (!selectedUser || !appState.activeSessionId) return;
    if (!confirm(`Are you sure you want to permanently drop user '${selectedUser.user}'@'${selectedUser.host}'?`)) {
      return;
    }

    isSaving = true;
    try {
      await api.deleteUser(appState.activeSessionId, {
        user: selectedUser.user,
        host: selectedUser.host,
      });
      appState.showToast(`User '${selectedUser.user}' dropped`, 'success');
      selectedUser = null;
      await loadUsers();
    } catch (err: any) {
      appState.showToast(`Failed to drop user: ${err.message}`, 'error');
    } finally {
      isSaving = false;
    }
  }

  async function handleApplyGrants() {
    if (!selectedUser || !appState.activeSessionId) return;

    isSaving = true;
    try {
      await api.applyGrants(appState.activeSessionId, {
        user: selectedUser.user,
        host: selectedUser.host,
        scope: targetDbScope === 'current' ? 'DATABASE' : 'GLOBAL',
        database: targetDbScope === 'current' ? appState.currentDatabase || '' : undefined,
        privileges: Array.from(selectedPrivileges),
        with_grant: withGrantOption,
      });

      appState.showToast(`Privileges updated for '${selectedUser.user}'@'${selectedUser.host}'`, 'success');
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
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-4xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-3.5 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-purple-500/10 border border-purple-500/30 text-purple-400">
            <Icon name="shield" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="priv-modal-title" class="font-bold text-sm text-slate-100">
              Database Users & Privilege Matrix
            </h3>
            <p class="text-[11px] text-slate-400">
              Visual user administration and GRANT/REVOKE permission matrix
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
        <div class="w-full md:w-60 bg-[#090d16] flex flex-col overflow-y-auto shrink-0 p-2 space-y-1">
          <div class="flex items-center justify-between px-2 py-1">
            <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500">
              Users ({users.length})
            </span>
            <button
              onclick={startNewUser}
              class="text-[11px] text-purple-400 hover:text-purple-300 font-medium flex items-center space-x-1"
            >
              <Icon name="plus" class="w-3 h-3" />
              <span>New</span>
            </button>
          </div>

          {#if isLoading}
            <div class="p-4 text-center text-xs text-slate-500">Loading users...</div>
          {:else}
            {#each users as u}
              {@const isSelected = selectedUser?.user === u.user && selectedUser?.host === u.host && !isCreatingNew}
              <button
                onclick={() => selectUser(u)}
                class="w-full text-left px-2.5 py-1.5 rounded text-xs flex items-center justify-between font-mono transition {isSelected ? 'bg-purple-950/60 text-purple-300 border border-purple-800/60 font-bold' : 'text-slate-300 hover:bg-slate-800'}"
              >
                <span class="truncate">{u.user}</span>
                <span class="text-[10px] text-slate-500 shrink-0 ml-1">@{u.host}</span>
              </button>
            {/each}
          {/if}
        </div>

        <!-- Right: Privileges Matrix & Account Editor -->
        <div class="flex-1 overflow-y-auto p-4 space-y-4 text-xs font-sans">
          {#if isCreatingNew}
            <!-- New User Creation Form -->
            <div class="bg-[#0f172a] p-4 rounded-lg border border-[#1e293b] space-y-3">
              <h4 class="font-bold text-slate-200 text-sm flex items-center space-x-2">
                <Icon name="user" class="w-4 h-4 text-purple-400" />
                <span>Create New Database User</span>
              </h4>
              <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div>
                  <label for="new-username" class="block text-[11px] text-slate-400 mb-1">Username</label>
                  <input
                    id="new-username"
                    type="text"
                    bind:value={newUsername}
                    placeholder="db_user"
                    class="w-full bg-[#090d16] border border-[#334155] rounded px-2.5 py-1.5 text-white font-mono text-xs focus:border-purple-500 outline-none"
                  />
                </div>
                <div>
                  <label for="new-host" class="block text-[11px] text-slate-400 mb-1">Host Origin</label>
                  <input
                    id="new-host"
                    type="text"
                    bind:value={newHost}
                    placeholder="%"
                    class="w-full bg-[#090d16] border border-[#334155] rounded px-2.5 py-1.5 text-white font-mono text-xs focus:border-purple-500 outline-none"
                  />
                </div>
                <div>
                  <label for="new-password" class="block text-[11px] text-slate-400 mb-1">Password</label>
                  <input
                    id="new-password"
                    type="password"
                    bind:value={newPassword}
                    placeholder="••••••••"
                    class="w-full bg-[#090d16] border border-[#334155] rounded px-2.5 py-1.5 text-white font-mono text-xs focus:border-purple-500 outline-none"
                  />
                </div>
              </div>
            </div>
          {:else if selectedUser}
            <!-- Selected User Details -->
            <div class="flex items-center justify-between bg-[#0f172a] p-3 rounded-lg border border-[#1e293b]">
              <div>
                <span class="text-[10px] text-slate-500 uppercase font-bold tracking-wider block">Selected User</span>
                <span class="font-bold text-slate-100 font-mono text-sm">
                  '{selectedUser.user}'@'{selectedUser.host}'
                </span>
              </div>

              <div class="flex items-center space-x-2">
                <button
                  onclick={handleDeleteUser}
                  class="px-2.5 py-1 text-xs text-red-400 hover:text-red-300 hover:bg-red-950/40 border border-red-900/40 rounded transition flex items-center space-x-1"
                >
                  <Icon name="trash-2" class="w-3.5 h-3.5" />
                  <span>Drop User</span>
                </button>
              </div>
            </div>
          {/if}

          <!-- Scope & Presets Bar -->
          <div class="flex flex-wrap items-center justify-between gap-2 bg-[#0d1322] p-2.5 rounded-lg border border-[#1e293b]">
            <div class="flex items-center space-x-2 text-xs">
              <span class="text-slate-400">Target Scope:</span>
              <select
                bind:value={targetDbScope}
                class="bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-white font-mono text-xs outline-none"
              >
                <option value="current">Database ({appState.currentDatabase || '*'}.*)</option>
                <option value="all">Global (*.*)</option>
              </select>
            </div>

            <!-- Quick Presets -->
            <div class="flex items-center space-x-1.5">
              <span class="text-slate-400 text-[11px] mr-1">Presets:</span>
              <button
                type="button"
                onclick={() => applyPreset('readonly')}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px]"
              >
                Read Only
              </button>
              <button
                type="button"
                onclick={() => applyPreset('readwrite')}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px]"
              >
                Read/Write
              </button>
              <button
                type="button"
                onclick={() => applyPreset('all')}
                class="px-2 py-0.5 rounded bg-purple-900/60 hover:bg-purple-800 text-purple-200 text-[11px]"
              >
                All
              </button>
              <button
                type="button"
                onclick={() => applyPreset('none')}
                class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-400 text-[11px]"
              >
                Clear
              </button>
            </div>
          </div>

          <!-- Categorized Privilege Groups -->
          <div class="space-y-3">
            {#each Object.entries(PRIV_GROUPS) as [groupName, privList]}
              <div class="space-y-1.5">
                <span class="text-[11px] font-bold text-slate-400 uppercase tracking-wider block">
                  {groupName} Privileges
                </span>
                <div class="grid grid-cols-2 sm:grid-cols-3 gap-1.5">
                  {#each privList as priv}
                    {@const isChecked = selectedPrivileges.has(priv)}
                    <label class="flex items-center space-x-2 px-2.5 py-1.5 rounded border border-[#1e293b] hover:border-slate-700 cursor-pointer select-none transition {isChecked ? 'bg-purple-950/20 border-purple-800/40 text-purple-300' : 'bg-[#090d16] text-slate-300'}">
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
            {/each}

            <!-- With Grant Option Toggle -->
            <label class="flex items-center space-x-2 pt-1 cursor-pointer select-none text-slate-300">
              <input
                type="checkbox"
                bind:checked={withGrantOption}
                class="rounded bg-slate-900 border-slate-700 text-purple-500 focus:ring-0"
              />
              <span class="font-semibold text-xs">WITH GRANT OPTION (Allow user to grant permissions to others)</span>
            </label>
          </div>

          <!-- SHOW GRANTS Raw Output -->
          {#if grantsRaw.length > 0}
            <div class="space-y-1">
              <span class="font-bold text-slate-400 text-[10px] uppercase tracking-wider block">
                Active SHOW GRANTS
              </span>
              <pre class="p-2.5 rounded bg-[#090d16] border border-slate-800 text-[11px] text-slate-400 font-mono overflow-x-auto whitespace-pre-wrap max-h-24 select-text">{grantsRaw.join('\n')}</pre>
            </div>
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
        {#if isCreatingNew}
          <button
            onclick={handleSaveNewUser}
            disabled={isSaving || !newUsername.trim()}
            type="button"
            class="px-4 py-1.5 bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow"
          >
            {#if isSaving}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Creating User...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Create User & Apply</span>
            {/if}
          </button>
        {:else}
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
        {/if}
      </div>
    </div>
  </div>
{/if}
