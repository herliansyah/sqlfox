<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let password = $state('');
  let errorMessage = $state('');
  let isUnlocking = $state(false);

  async function handleUnlock(e: Event) {
    e.preventDefault();
    if (!password) return;
    isUnlocking = true;
    errorMessage = '';

    try {
      const ok = await appState.unlockRemote(password);
      if (!ok) {
        errorMessage = 'Invalid master password or PIN.';
      }
    } catch (err: any) {
      errorMessage = err.message || 'Authentication failed. Please verify your PIN.';
    } finally {
      isUnlocking = false;
    }
  }
</script>

{#if appState.isRemoteLocked}
  <div class="fixed inset-0 z-[110] bg-[#020617]/95 backdrop-blur-md flex items-center justify-center p-4 select-none">
    <div class="bg-[#0f172a] border-2 border-sky-400/80 w-full max-w-sm rounded-2xl shadow-[0_0_35px_rgba(56,189,248,0.25)] p-6 space-y-4 text-center animate-in fade-in zoom-in-95 duration-200">
      <!-- Icons -->
      <div class="flex justify-center items-center space-x-2 py-1">
        <Icon name="brand" class="w-12 h-12 flex-shrink-0" />
        <Icon name="lock" class="w-7 h-7 text-sky-400 shrink-0" />
      </div>

      <div>
        <h2 class="text-base font-bold text-white tracking-wide">SqlFox Remote Access Guard</h2>
        <p class="text-xs text-slate-400 leading-relaxed mt-1">
          You are accessing SqlFox from a network device. Please enter your Master Password or PIN to unlock this session.
        </p>
      </div>

      <form onsubmit={handleUnlock} class="space-y-3 pt-1">
        <div>
          <input
            type="password"
            bind:value={password}
            autofocus
            required
            placeholder="Enter Master Password / PIN"
            class="w-full bg-[#090d16] border border-slate-700 focus:border-sky-400 p-2.5 rounded-lg text-white text-center text-sm outline-none font-mono transition shadow-inner"
          />
        </div>

        {#if errorMessage}
          <div class="text-xs text-red-300 bg-red-950/60 border border-red-800 rounded-lg px-2.5 py-1.5 font-medium leading-relaxed">
            {errorMessage}
          </div>
        {/if}

        <button
          type="submit"
          disabled={isUnlocking}
          class="w-full bg-sky-400 hover:bg-sky-300 disabled:opacity-50 text-slate-950 font-bold py-2 rounded-lg text-sm transition shadow-md flex items-center justify-center space-x-2"
        >
          {#if isUnlocking}
            <Icon name="refresh-cw" class="w-4 h-4 animate-spin text-slate-950" />
            <span>Unlocking...</span>
          {:else}
            <span>Unlock Vault</span>
          {/if}
        </button>

        <div class="text-[11px] text-slate-500 pt-1">
          Default Master PIN: <span class="font-mono text-sky-400 font-semibold select-text">{appState.authStatus?.master_password || 'fox-0304'}</span>
        </div>
      </form>
    </div>
  </div>
{/if}
