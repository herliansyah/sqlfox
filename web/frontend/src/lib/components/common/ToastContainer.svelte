<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
</script>

<div class="fixed bottom-4 right-4 z-50 flex flex-col space-y-2 pointer-events-none max-w-sm">
  {#each appState.toasts as toast (toast.id)}
    <div
      class="pointer-events-auto flex items-center justify-between px-3 py-2 rounded-lg shadow-xl text-xs font-mono border backdrop-blur-md transition-all animate-in slide-in-from-bottom-2 duration-150 {
        toast.type === 'error'
          ? 'bg-rose-950/90 text-rose-200 border-rose-800'
          : toast.type === 'warning'
            ? 'bg-amber-950/90 text-amber-200 border-amber-800'
            : toast.type === 'success'
              ? 'bg-emerald-950/90 text-emerald-200 border-emerald-800'
              : 'bg-slate-900/90 text-slate-200 border-slate-700'
      }"
    >
      <div class="flex items-center space-x-2 mr-3 truncate">
        {#if toast.type === 'error'}
          <Icon name="alert-triangle" class="w-4 h-4 text-rose-400 shrink-0" />
        {:else if toast.type === 'success'}
          <Icon name="check" class="w-4 h-4 text-emerald-400 shrink-0" />
        {:else if toast.type === 'warning'}
          <Icon name="shield-alert" class="w-4 h-4 text-amber-400 shrink-0" />
        {:else}
          <Icon name="activity" class="w-4 h-4 text-sky-400 shrink-0" />
        {/if}
        <span class="truncate">{toast.message}</span>
      </div>
      <button
        onclick={() => appState.removeToast(toast.id)}
        class="text-slate-400 hover:text-white p-0.5"
      >
        <Icon name="x" class="w-3 h-3" />
      </button>
    </div>
  {/each}
</div>
