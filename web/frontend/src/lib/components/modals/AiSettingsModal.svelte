<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let provider = $state<'openai' | 'anthropic' | 'gemini' | 'ollama' | 'custom'>('openai');
  let apiKey = $state('');
  let baseUrl = $state('https://api.openai.com/v1');
  let modelName = $state('gpt-4o-mini');
  let temperature = $state(0.3);
  let showKey = $state(false);
  let isTesting = $state(false);
  let testStatus = $state<{ ok: boolean; message: string } | null>(null);

  // Presets
  const presets: Record<string, { url: string; model: string }> = {
    openai: { url: 'https://api.openai.com/v1', model: 'gpt-4o-mini' },
    anthropic: { url: 'https://api.anthropic.com/v1', model: 'claude-3-5-sonnet-20241022' },
    gemini: { url: 'https://generativelanguage.googleapis.com/v1beta', model: 'gemini-1.5-flash' },
    ollama: { url: 'http://localhost:11434', model: 'llama3.2:latest' },
    custom: { url: '', model: '' },
  };

  function onProviderChange(p: typeof provider) {
    provider = p;
    const preset = presets[p];
    if (preset) {
      // Custom starts blank: keep whatever the operator already typed.
      if (preset.url) baseUrl = preset.url;
      if (preset.model) modelName = preset.model;
    }
  }

  function loadSavedConfig() {
    try {
      const saved = localStorage.getItem('sqlfox_ai_config');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (parsed.provider) provider = parsed.provider;
        if (parsed.apiKey) apiKey = parsed.apiKey;
        if (parsed.baseUrl) baseUrl = parsed.baseUrl;
        if (parsed.modelName) modelName = parsed.modelName;
        if (parsed.temperature !== undefined) temperature = parsed.temperature;
      }
    } catch (e) {}
  }

  function saveConfig() {
    try {
      const config = { provider, apiKey, baseUrl, modelName, temperature };
      localStorage.setItem('sqlfox_ai_config', JSON.stringify(config));
      appState.showToast('AI settings saved successfully!', 'success');
      appState.isAiSettingsModalOpen = false;
    } catch (e: any) {
      appState.showToast(`Failed to save AI settings: ${e.message}`, 'error');
    }
  }

  async function testConnection() {
    isTesting = true;
    testStatus = null;
    try {
      // Real round-trip: a fake "looks valid" check is what hid the broken custom provider before.
      const res = await api.aiChat(
        'Reply with the single word OK.',
        '',
        'SQL',
        { provider, apiKey, baseUrl, modelName, temperature }
      );
      testStatus = { ok: true, message: `Connected. Model replied: "${(res.content || '').slice(0, 60)}"` };
    } catch (err: any) {
      testStatus = { ok: false, message: err.message || 'Request failed' };
    } finally {
      isTesting = false;
    }
  }

  $effect(() => {
    if (appState.isAiSettingsModalOpen) {
      loadSavedConfig();
    }
  });
</script>

{#if appState.isAiSettingsModalOpen}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="bot" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">AI Copilot Settings (BYOK)</h3>
        </div>
        <button onclick={() => (appState.isAiSettingsModalOpen = false)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1.5">AI Provider</label>
          <div class="grid grid-cols-5 gap-1.5">
            {#each ['openai', 'anthropic', 'gemini', 'ollama', 'custom'] as p}
              <button
                type="button"
                onclick={() => onProviderChange(p as any)}
                class="py-1.5 px-2 rounded-lg border text-center font-medium capitalize text-xs transition {provider === p
                  ? 'bg-sky-600/30 border-sky-500 text-sky-200'
                  : 'bg-slate-900 border-slate-800 text-slate-400 hover:border-slate-700'}"
              >
                {p}
              </button>
            {/each}
          </div>
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">
            API Key {provider === 'ollama' ? '(Not required for local Ollama)' : '*'}
          </label>
          <div class="relative">
            <input
              type={showKey ? 'text' : 'password'}
              bind:value={apiKey}
              placeholder={provider === 'ollama' ? 'Local Ollama requires no key' : 'sk-...'}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg pl-3 pr-10 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
            />
            <button
              type="button"
              onclick={() => (showKey = !showKey)}
              class="absolute right-2.5 top-2 text-slate-500 hover:text-slate-300"
            >
              <Icon name={showKey ? 'eye-off' : 'eye'} class="w-4 h-4" />
            </button>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Base Endpoint URL</label>
            <input
              type="text"
              bind:value={baseUrl}
              placeholder={provider === 'custom' ? 'https://your-proxy/v1' : 'https://api.openai.com/v1'}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
            />
          </div>
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Model Identifier</label>
            <input
              type="text"
              bind:value={modelName}
              placeholder="e.g. gpt-4o-mini"
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 font-mono text-xs focus:outline-none focus:border-sky-500"
            />
          </div>
        </div>

        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="text-slate-400 text-xs font-medium">Temperature: {temperature}</label>
            <span class="text-slate-500 text-[10px]">Deterministic (0.0) ↔ Creative (1.0)</span>
          </div>
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            bind:value={temperature}
            class="w-full accent-sky-500"
          />
        </div>

        {#if testStatus}
          <div class="p-3 rounded-lg border text-xs flex items-center space-x-2 {testStatus.ok ? 'bg-emerald-950/40 border-emerald-800 text-emerald-300' : 'bg-rose-950/40 border-rose-800 text-rose-300'}">
            <Icon name={testStatus.ok ? 'check-circle' : 'alert-circle'} class="w-4 h-4 shrink-0" />
            <span>{testStatus.message}</span>
          </div>
        {/if}
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <button
          onclick={testConnection}
          disabled={isTesting}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition flex items-center space-x-1.5"
        >
          <Icon name="activity" class="w-3.5 h-3.5 {isTesting ? 'animate-spin text-sky-400' : ''}" />
          <span>{isTesting ? 'Testing...' : 'Test Connection'}</span>
        </button>

        <div class="flex items-center space-x-2">
          <button
            onclick={() => (appState.isAiSettingsModalOpen = false)}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            Cancel
          </button>
          <button
            onclick={saveConfig}
            class="px-4 py-1.5 rounded bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold transition"
          >
            Save Settings
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
