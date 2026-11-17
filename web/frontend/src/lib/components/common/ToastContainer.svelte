<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
</script>

<div class="fixed bottom-9 right-3 z-[9999] flex flex-col space-y-2 pointer-events-none max-w-md">
  {#each appState.toasts as toast (toast.id)}
    <div
      class="pointer-events-auto flex flex-col gap-2 px-3.5 py-2.5 rounded-lg shadow-2xl text-xs font-mono border backdrop-blur-md transition-all animate-in slide-in-from-bottom-2 duration-150 {
        toast.type === 'error'
          ? 'bg-rose-950/95 text-rose-200 border-rose-800'
          : toast.type === 'warning'
            ? 'bg-amber-950/95 text-amber-200 border-amber-800'
            : toast.type === 'success'
              ? 'bg-emerald-950/95 text-emerald-200 border-emerald-800'
              : 'bg-slate-900/95 text-slate-200 border-slate-700'
      }"
    >
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center space-x-2 min-w-0">
          {#if toast.type === 'error'}
            <Icon name="alert-triangle" class="w-4 h-4 text-rose-400 shrink-0" />
          {:else if toast.type === 'success'}
            <Icon name="check" class="w-4 h-4 text-emerald-400 shrink-0" />
          {:else if toast.type === 'warning'}
            <Icon name="shield-alert" class="w-4 h-4 text-amber-400 shrink-0" />
          {:else}
            <Icon name="activity" class="w-4 h-4 text-sky-400 shrink-0" />
          {/if}
          <span class="break-words leading-relaxed">{toast.message}</span>
        </div>
        <button
          onclick={() => appState.removeToast(toast.id)}
          class="text-slate-400 hover:text-white p-0.5 shrink-0"
          aria-label="Close notification"
        >
          <Icon name="x" class="w-3 h-3" />
        </button>
      </div>

      {#if toast.actions && toast.actions.length > 0}
        <div class="flex items-center justify-end gap-1.5 pt-1 border-t border-white/10 font-sans">
          {#each toast.actions as act}
            <button
              type="button"
              onclick={() => act.onClick()}
              class="px-2 py-0.5 rounded text-[11px] font-semibold transition {
                act.variant === 'primary'
                  ? 'bg-sky-500 hover:bg-sky-400 text-slate-950'
                  : act.variant === 'danger'
                    ? 'bg-rose-600 hover:bg-rose-500 text-white'
                    : 'bg-slate-800/90 hover:bg-slate-700 text-slate-200 border border-slate-600/70'
              }"
            >
              {act.label}
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/each}
</div>
