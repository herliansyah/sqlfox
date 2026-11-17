<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';
  import AiSettingsPanel from './AiSettingsPanel.svelte';

  let aiPanel = $state<AiSettingsPanel | null>(null);
  let isTesting = $state(false);
</script>

{#if appState.isAiSettingsModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-xl max-h-[90vh] shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a] shrink-0">
        <div class="flex items-center space-x-2">
          <Icon name="bot" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">AI Copilot Settings (BYOK)</h3>
        </div>
        <button type="button" onclick={() => (appState.isAiSettingsModalOpen = false)} class="text-slate-400 hover:text-white" aria-label="Close">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 overflow-y-auto flex-1">
        <AiSettingsPanel
          bind:this={aiPanel}
          active={appState.isAiSettingsModalOpen}
          bind:isTesting={isTesting}
          onSaved={() => (appState.isAiSettingsModalOpen = false)}
        />
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a] shrink-0">
        <button
          type="button"
          onclick={() => aiPanel?.testConnection()}
          disabled={isTesting}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition flex items-center space-x-1.5"
        >
          <Icon name="activity" class="w-3.5 h-3.5 {isTesting ? 'animate-spin text-sky-400' : ''}" />
          <span>{isTesting ? 'Testing...' : 'Test Connection'}</span>
        </button>

        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={() => (appState.isAiSettingsModalOpen = false)}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Cancel
          </button>
          <button
            type="button"
            onclick={() => aiPanel?.saveConfig()}
            class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold transition"
          >
            Save Settings
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
