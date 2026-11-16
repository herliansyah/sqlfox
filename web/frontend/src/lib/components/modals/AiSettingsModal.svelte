<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api, loadAiConfig, type AiConfig } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let provider = $state<'openai' | 'anthropic' | 'gemini' | 'ollama' | 'custom'>('openai');
  let apiKey = $state('');
  let baseUrl = $state('https://api.openai.com/v1');
  let modelName = $state('gpt-4o-mini');
  let temperature = $state(0.3);
  let language = $state<'auto' | 'en' | 'id'>('auto');
  let responseStyle = $state<'normal' | 'concise' | 'detailed'>('normal');
  let customPrompt = $state('');
  let includeSchema = $state(true);
  let maskIdentifiers = $state(true);

  let showKey = $state(false);
  let isTesting = $state(false);
  let isCheckingOllama = $state(false);
  let ollamaModels = $state<string[]>([]);
  let testStatus = $state<{ ok: boolean; message: string } | null>(null);

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
      if (preset.url) baseUrl = preset.url;
      if (preset.model) modelName = preset.model;
    }
    testStatus = null;
    ollamaModels = [];
  }

  function applyLoadedConfig() {
    const cfg = loadAiConfig();
    provider = (cfg.provider as any) || 'openai';
    apiKey = cfg.apiKey || '';
    baseUrl = cfg.baseUrl || '';
    modelName = cfg.modelName || '';
    temperature = cfg.temperature ?? 0.3;
    language = cfg.language || 'auto';
    responseStyle = cfg.responseStyle || 'normal';
    customPrompt = cfg.customPrompt || '';
    includeSchema = cfg.includeSchema !== false;
    maskIdentifiers = cfg.maskIdentifiers !== false;
  }

  function currentConfig(): AiConfig {
    return {
      provider,
      apiKey,
      baseUrl: baseUrl.trim(),
      modelName: modelName.trim(),
      temperature,
      language,
      responseStyle,
      customPrompt: customPrompt.trim(),
      includeSchema,
      maskIdentifiers,
    };
  }

  function saveConfig() {
    try {
      const config = currentConfig();
      localStorage.setItem('sqlfox_ai_config', JSON.stringify(config));
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('sqlfox:ai-config-updated'));
      }
      appState.showToast('AI settings saved successfully!', 'success');
      appState.isAiSettingsModalOpen = false;
    } catch (e: any) {
      appState.showToast(`Failed to save AI settings: ${e.message}`, 'error');
    }
  }

  async function detectOllamaModels() {
    isCheckingOllama = true;
    testStatus = null;
    ollamaModels = [];
    const ep = (baseUrl || 'http://localhost:11434').replace(/\/+$/, '');
    try {
      const res = await api.checkOllama(ep);
      let detectedNames: string[] = [];
      if (ep.startsWith('http://localhost:') || ep.startsWith('http://127.0.0.1:')) {
        try {
          const r = await fetch(`${ep}/api/tags`);
          if (r.ok) {
            const tags = await r.json();
            if (Array.isArray(tags?.models)) {
              detectedNames = tags.models.map((m: any) => m.name || m.model).filter(Boolean);
            }
          }
        } catch { /* ignore CORS/local mismatch */ }
      }
      ollamaModels = detectedNames;
      if (res.available) {
        if (detectedNames.length > 0 && !detectedNames.includes(modelName)) {
          modelName = detectedNames[0]!;
        }
        testStatus = {
          ok: true,
          message: detectedNames.length > 0
            ? `Ollama online (${detectedNames.length} model(s) detected via /api/tags)`
            : 'Ollama daemon is reachable and ready.',
        };
      } else {
        testStatus = { ok: false, message: `Ollama is not reachable at ${ep}` };
      }
    } catch (err: any) {
      testStatus = { ok: false, message: err.message || 'Failed to reach Ollama' };
    } finally {
      isCheckingOllama = false;
    }
  }

  async function testConnection() {
    isTesting = true;
    testStatus = null;
    try {
      const res = await api.aiChat(
        'Reply with the single word OK.',
        '',
        'SQL',
        currentConfig()
      );
      testStatus = { ok: true, message: `Connected. Model replied: "${(res.content || '').slice(0, 60)}"` };
    } catch (err: any) {
      testStatus = { ok: false, message: err.message || 'Request failed' };
    } finally {
      isTesting = false;
    }
  }

  let wasOpen = false;
  $effect(() => {
    if (appState.isAiSettingsModalOpen) {
      if (!wasOpen) {
        wasOpen = true;
        testStatus = null;
        ollamaModels = [];
        applyLoadedConfig();
      }
    } else {
      wasOpen = false;
    }
  });
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

      <div class="p-5 space-y-4 overflow-y-auto flex-1">
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
              aria-label="Toggle API key visibility"
            >
              <Icon name={showKey ? 'eye-off' : 'eye'} class="w-4 h-4" />
            </button>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="text-slate-400 text-xs font-medium">Base Endpoint URL</label>
              {#if provider === 'ollama'}
                <button
                  type="button"
                  onclick={detectOllamaModels}
                  disabled={isCheckingOllama}
                  class="text-[10px] text-sky-400 hover:underline flex items-center space-x-1"
                >
                  <Icon name="refresh-cw" class="w-2.5 h-2.5 {isCheckingOllama ? 'animate-spin' : ''}" />
                  <span>Check Ollama /tags</span>
                </button>
              {/if}
            </div>
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

        {#if ollamaModels.length > 0}
          <div class="flex flex-wrap items-center gap-1.5">
            <span class="text-[10px] text-slate-400 font-medium">Detected Ollama Models:</span>
            {#each ollamaModels as om}
              <button
                type="button"
                onclick={() => (modelName = om)}
                class="px-2 py-0.5 rounded font-mono text-[10px] border transition {modelName === om
                  ? 'bg-sky-600/30 border-sky-500 text-sky-200'
                  : 'bg-slate-900 border-slate-800 text-slate-300 hover:border-slate-600'}"
              >
                {om}
              </button>
            {/each}
          </div>
        {/if}

        <!-- Language & Response Style -->
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Response Language</label>
            <select
              bind:value={language}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
            >
              <option value="auto">Auto (Match Prompt)</option>
              <option value="en">English</option>
              <option value="id">Bahasa Indonesia</option>
            </select>
          </div>
          <div>
            <label class="block text-slate-400 text-xs font-medium mb-1">Response Style</label>
            <select
              bind:value={responseStyle}
              class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500"
            >
              <option value="normal">Balanced / Standard</option>
              <option value="concise">Concise (Direct SQL &amp; Bullets)</option>
              <option value="detailed">Detailed (Deep Explanation)</option>
            </select>
          </div>
        </div>

        <div>
          <label class="block text-slate-400 text-xs font-medium mb-1">Custom System Instructions (Optional)</label>
          <textarea
            bind:value={customPrompt}
            rows="2"
            placeholder="e.g. Always prefer CTEs over subqueries, include index recommendations..."
            class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500 resize-y"
          ></textarea>
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

        <!-- Privacy & Context Guardrails -->
        <div class="p-3 rounded-xl bg-[#050811] border border-[#1e293b] space-y-2.5">
          <label class="flex items-start space-x-2.5 cursor-pointer select-none">
            <input type="checkbox" bind:checked={includeSchema} class="mt-0.5 rounded border-slate-700 text-sky-500" />
            <div>
              <span class="text-slate-200 font-medium block">Include Schema DDL Context by Default</span>
              <span class="text-[10.5px] text-slate-500 block">Attaches active table column names &amp; types to AI requests (never sends table rows)</span>
            </div>
          </label>
          <label class="flex items-start space-x-2.5 cursor-pointer select-none">
            <input type="checkbox" bind:checked={maskIdentifiers} class="mt-0.5 rounded border-slate-700 text-emerald-500" />
            <div>
              <span class="text-emerald-300 font-medium block">Privacy Firewall: Mask Schema Identifiers</span>
              <span class="text-[10.5px] text-slate-500 block">Anonymizes table, column, and database identifiers before outbound LLM calls</span>
            </div>
          </label>
        </div>

        {#if testStatus}
          <div class="p-3 rounded-lg border text-xs flex items-center space-x-2 {testStatus.ok ? 'bg-emerald-950/40 border-emerald-800 text-emerald-300' : 'bg-rose-950/40 border-rose-800 text-rose-300'}">
            <Icon name={testStatus.ok ? 'check-circle' : 'alert-circle'} class="w-4 h-4 shrink-0" />
            <span>{testStatus.message}</span>
          </div>
        {/if}
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a] shrink-0">
        <button
          type="button"
          onclick={testConnection}
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
