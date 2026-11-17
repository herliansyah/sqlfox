<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { ConnectionProfile, DatabaseDriver, Environment } from '../../../types';
  import Icon from '../common/Icon.svelte';

  let isEditing = $derived(!!appState.connectionToEdit);
  let connId = $state('');
  let name = $state('');
  let driver = $state<DatabaseDriver>('mysql');
  let environment = $state<Environment>('Development');
  let host = $state('127.0.0.1');
  let port = $state(3306);
  let user = $state('root');
  let password = $state('');
  let database = $state('');
  let readOnly = $state(false);

  // SSH & SSL
  let showAdvanced = $state(false);
  let useSSL = $state(false);
  let sslCA = $state('');
  let sslCert = $state('');
  let sslKey = $state('');

  let isTesting = $state(false);
  let testResult = $state<{ success: boolean; message: string } | null>(null);

  $effect(() => {
    if (appState.isConnectionModalOpen) {
      testResult = null;
      if (appState.connectionToEdit) {
        const c = appState.connectionToEdit;
        connId = c.id;
        name = c.name;
        driver = c.driver;
        environment = c.environment;
        host = c.host || (c.driver === 'sqlite' ? './database.db' : '127.0.0.1');
        port = c.port || (c.driver === 'postgres' ? 5432 : 3306);
        user = c.user || '';
        password = '';
        database = c.database || c.database_name || '';
        readOnly = !!c.read_only;
        useSSL = !!c.use_ssl;
        sslCA = c.ssl_ca || '';
        sslCert = c.ssl_cert || '';
        sslKey = c.ssl_key || '';
      } else {
        connId = '';
        name = 'New Connection';
        driver = 'mysql';
        environment = 'Development';
        host = '127.0.0.1';
        port = 3306;
        user = 'root';
        password = '';
        database = '';
        readOnly = false;
        useSSL = false;
        sslCA = '';
        sslCert = '';
        sslKey = '';
      }
    }
  });

  function handleDriverChange(d: DatabaseDriver) {
    driver = d;
    if (d === 'postgres' && port === 3306) port = 5432;
    if (d === 'mysql' && port === 5432) port = 3306;
    if (d === 'sqlite' && !host.trim()) {
      host = './data.db';
    }
  }

  async function handleTest() {
    if (!host.trim()) {
      testResult = { success: false, message: driver === 'sqlite' ? 'SQLite database path is required' : 'Host / Server IP is required' };
      return;
    }
    isTesting = true;
    testResult = null;
    try {
      const targetDb = driver === 'sqlite' ? host.trim() : database.trim();
      const res: any = await api.testTransientConnection({
        id: connId || undefined,
        name: name.trim() || 'Test Transient',
        driver,
        host: host.trim(),
        port: Number(port),
        user: user.trim(),
        ...(password ? { password } : {}),
        database: targetDb,
        database_name: targetDb,
        environment,
        use_ssl: useSSL,
        ssl_ca: sslCA.trim(),
        ssl_cert: sslCert.trim(),
        ssl_key: sslKey.trim(),
      });
      testResult = {
        success: !!res.success,
        message: res.success
          ? `Connected successfully (${res.latency_ms ?? 1} ms, ${res.version ?? res.server ?? 'OK'})`
          : res.error || 'Connection failed',
      };
    } catch (err: any) {
      testResult = { success: false, message: err.message || 'Connection test failed' };
    } finally {
      isTesting = false;
    }
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    const targetDb = driver === 'sqlite' ? host.trim() : database.trim();
    const payload: Partial<ConnectionProfile> = {
      id: connId || undefined,
      name: name.trim() || `${driver.toUpperCase()} on ${host}`,
      driver,
      environment,
      host: host.trim(),
      port: Number(port),
      user: user.trim(),
      database: targetDb,
      database_name: targetDb,
      read_only: readOnly,
      use_ssl: useSSL,
      ssl_ca: sslCA.trim(),
      ssl_cert: sslCert.trim(),
      ssl_key: sslKey.trim(),
    };
    if (password) {
      payload.password = password;
    }
    await appState.saveConnection(payload);
  }

  async function handleDelete() {
    if (!connId) return;
    if (await appState.showNativeConfirm(`Are you sure you want to delete connection "${name}"?`, 'Delete Connection')) {
      await appState.deleteConnection(connId);
      appState.isConnectionModalOpen = false;
    }
  }
</script>

{#if appState.isConnectionModalOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm select-none">
    <div class="w-full max-w-lg bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-[#334155] flex items-center justify-between bg-[#0b0f19]">
        <div class="flex items-center space-x-2.5">
          <Icon name="database" class="w-5 h-5 text-sky-400" />
          <h2 class="font-bold text-sm text-slate-100">
            {isEditing ? `Edit Connection: ${name}` : 'New Database Connection'}
          </h2>
        </div>
        <button
          onclick={() => (appState.isConnectionModalOpen = false)}
          class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Form Body -->
      <form onsubmit={handleSubmit} class="p-5 overflow-y-auto space-y-4 text-xs font-sans">
        <!-- Driver Picker -->
        <div>
          <span class="block text-[11px] font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Database Driver</span>
          <div class="grid grid-cols-3 gap-2">
            {#each ['mysql', 'postgres', 'sqlite'] as d}
              <button
                type="button"
                onclick={() => handleDriverChange(d as DatabaseDriver)}
                class="py-2 px-3 rounded-lg border text-xs font-semibold flex items-center justify-center space-x-2 transition {
                  driver === d
                    ? 'bg-sky-500/15 border-sky-400 text-sky-300 shadow-sm'
                    : 'bg-[#1e293b]/70 border-[#334155] text-slate-400 hover:text-slate-200 hover:bg-slate-800'
                }"
              >
                <span>{d === 'postgres' ? 'PostgreSQL' : d === 'sqlite' ? 'SQLite' : 'MySQL'}</span>
              </button>
            {/each}
          </div>
        </div>

        <!-- Connection Name & Environment -->
        <div class="grid grid-cols-2 gap-3">
          <label class="block text-[11px] font-semibold text-slate-300">
            Display Name
            <input
              type="text"
              bind:value={name}
              placeholder="e.g. Local Prod Replica"
              required
              class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500"
            />
          </label>
          <label class="block text-[11px] font-semibold text-slate-300">
            Environment Guard
            <select
              bind:value={environment}
              class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 focus:outline-none focus:border-sky-500 font-semibold"
            >
              <option value="Development">Development (Low Risk)</option>
              <option value="Staging">Staging (Medium Risk)</option>
              <option value="Production">Production (High Risk / Challenge)</option>
            </select>
          </label>
        </div>

        <!-- Host & Port (or File Path if SQLite) -->
        {#if driver === 'sqlite'}
          <label class="block text-[11px] font-semibold text-slate-300">
            SQLite Database Path
            <input
              type="text"
              bind:value={host}
              placeholder="/path/to/database.db"
              required
              class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:border-sky-500"
            />
          </label>
        {:else}
          <div class="grid grid-cols-3 gap-3">
            <label class="col-span-2 block text-[11px] font-semibold text-slate-300">
              Host / Server IP
              <input
                type="text"
                bind:value={host}
                placeholder="127.0.0.1 or db.internal"
                required
                class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:border-sky-500"
              />
            </label>
            <label class="block text-[11px] font-semibold text-slate-300">
              Port
              <input
                type="number"
                bind:value={port}
                class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-sky-500"
              />
            </label>
          </div>

          <!-- User & Password -->
          <div class="grid grid-cols-2 gap-3">
            <label class="block text-[11px] font-semibold text-slate-300">
              Username
              <input
                type="text"
                bind:value={user}
                placeholder="root"
                class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:border-sky-500"
              />
            </label>
            <label class="block text-[11px] font-semibold text-slate-300">
              Password {isEditing ? '(leave blank to keep current)' : ''}
              <input
                type="password"
                bind:value={password}
                placeholder="••••••••"
                class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:border-sky-500"
              />
            </label>
          </div>

          <!-- Initial Database -->
          <label class="block text-[11px] font-semibold text-slate-300">
            Initial Database (Optional)
            <input
              type="text"
              bind:value={database}
              placeholder="e.g. ecommerce_production"
              class="w-full mt-1 bg-[#090d16] border border-[#334155] rounded px-3 py-1.5 text-xs text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:border-sky-500"
            />
          </label>
        {/if}

        <!-- Read Only Toggle -->
        <label class="flex items-center space-x-2 cursor-pointer pt-1">
          <input
            type="checkbox"
            bind:checked={readOnly}
            class="rounded bg-[#090d16] border-[#334155] text-sky-500 focus:ring-0"
          />
          <span class="text-xs text-slate-300 font-medium">Read-Only Mode (Blocks all INSERT, UPDATE, DELETE queries)</span>
        </label>

        <!-- Advanced Options Toggle -->
        {#if driver !== 'sqlite'}
          <div class="pt-2 border-t border-[#1e293b]">
            <button
              type="button"
              onclick={() => (showAdvanced = !showAdvanced)}
              class="text-xs text-sky-400 hover:text-sky-300 flex items-center space-x-1 font-semibold"
            >
              <Icon name={showAdvanced ? 'chevron-down' : 'chevron-right'} class="w-3.5 h-3.5" />
              <span>Advanced: TLS / SSL Certificates</span>
            </button>

            {#if showAdvanced}
              <div class="mt-3 p-3 bg-[#090d16] rounded-lg border border-[#1e293b] space-y-3">
                <label class="flex items-center space-x-2 cursor-pointer">
                  <input
                    type="checkbox"
                    bind:checked={useSSL}
                    class="rounded bg-[#0f172a] border-[#334155] text-sky-500 focus:ring-0"
                  />
                  <span class="text-xs text-slate-200 font-semibold">Enable TLS / SSL Encryption</span>
                </label>

                {#if useSSL}
                  <div class="space-y-2 pt-1">
                    <label class="block text-[10px] text-slate-400">
                      CA Certificate Path (optional)
                      <input
                        type="text"
                        bind:value={sslCA}
                        placeholder="/path/to/ca.pem"
                        class="w-full mt-0.5 bg-[#0f172a] border border-[#334155] rounded px-2 py-1 text-xs text-slate-100 font-mono"
                      />
                    </label>
                    <div class="grid grid-cols-2 gap-2">
                      <label class="block text-[10px] text-slate-400">
                        Client Cert Path (mTLS)
                        <input
                          type="text"
                          bind:value={sslCert}
                          placeholder="/path/to/client-cert.pem"
                          class="w-full mt-0.5 bg-[#0f172a] border border-[#334155] rounded px-2 py-1 text-xs text-slate-100 font-mono"
                        />
                      </label>
                      <label class="block text-[10px] text-slate-400">
                        Client Key Path (mTLS)
                        <input
                          type="text"
                          bind:value={sslKey}
                          placeholder="/path/to/client-key.pem"
                          class="w-full mt-0.5 bg-[#0f172a] border border-[#334155] rounded px-2 py-1 text-xs text-slate-100 font-mono"
                        />
                      </label>
                    </div>
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        {/if}

        <!-- Test Result Banner -->
        {#if testResult}
          <div class="p-2.5 rounded-lg border text-xs font-mono {testResult.success ? 'bg-emerald-950/60 border-emerald-800 text-emerald-200' : 'bg-rose-950/60 border-rose-800 text-rose-200'}">
            {testResult.message}
          </div>
        {/if}

        <!-- Actions -->
        <div class="pt-3 border-t border-[#334155] flex items-center justify-between">
          <div>
            {#if isEditing}
              <button
                type="button"
                onclick={handleDelete}
                class="px-3 py-1.5 bg-rose-950/60 hover:bg-rose-900 border border-rose-800 text-rose-300 rounded text-xs font-semibold transition"
              >
                Delete Profile
              </button>
            {/if}
          </div>

          <div class="flex items-center space-x-2">
            <button
              type="button"
              onclick={handleTest}
              disabled={isTesting}
              class="px-3 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-200 border border-[#334155] rounded text-xs font-semibold transition flex items-center space-x-1.5"
            >
              {#if isTesting}
                <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin text-sky-400" />
                <span>Testing...</span>
              {:else}
                <Icon name="activity" class="w-3.5 h-3.5 text-sky-400" />
                <span>Test Connection</span>
              {/if}
            </button>

            <button
              type="submit"
              class="px-4 py-1.5 bg-sky-500 hover:bg-sky-400 text-slate-950 rounded text-xs font-bold transition shadow-[0_0_12px_rgba(56,189,248,0.35)]"
            >
              Save Profile
            </button>
          </div>
        </div>
      </form>
    </div>
  </div>
{/if}
