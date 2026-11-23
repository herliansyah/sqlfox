<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api, syncOrMigrateLegacyAiConfig, type AIProfile, type AiConfig } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  interface Props {
    active?: boolean;
    isTesting?: boolean;
    onSaved?: () => void;
  }

  let { active = true, isTesting = $bindable(false), onSaved }: Props = $props();

  let profiles = $state<AIProfile[]>([]);
  let selectedProfileId = $state<string>('');
  let isCreatingNew = $state(false);

  // Form states for the currently edited profile
  let profileName = $state('');
  let provider = $state<'openai' | 'anthropic' | 'gemini' | 'ollama' | 'custom'>('openai');
  let apiKey = $state('');
  let hasSavedKey = $state(false);
  let baseUrl = $state('https://api.openai.com/v1');
  let modelName = $state('gpt-4o-mini');
  let temperature = $state(0.3);
  let language = $state<'auto' | 'en' | 'id'>('auto');
  let responseStyle = $state<'normal' | 'concise' | 'detailed'>('normal');
  let customPrompt = $state('');
  let includeSchema = $state(true);
  let maskIdentifiers = $state(true);
  let isActive = $state(false);

  let showKey = $state(false);
  let isCheckingOllama = $state(false);
  let ollamaModels = $state<string[]>([]);
  let testStatus = $state<{ ok: boolean; message: string } | null>(null);
  let isLoading = $state(false);

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

  async function loadAllProfiles(preferredId?: string) {
    isLoading = true;
    try {
      await syncOrMigrateLegacyAiConfig();
      const list = await api.listAiProfiles();
      profiles = Array.isArray(list) ? list : [];

      if (profiles.length === 0) {
        startNewProfile();
        return;
      }

      let target = profiles.find((p) => p.id === preferredId);
      if (!target) {
        target = profiles.find((p) => p.is_active) || profiles[0];
      }
      if (target) {
        selectProfile(target);
      }
    } catch (err: any) {
      appState.showToast(`Failed to load AI profiles: ${err.message}`, 'error');
    } finally {
      isLoading = false;
    }
  }

  async function selectProfile(p: AIProfile) {
    isCreatingNew = false;
    selectedProfileId = p.id;
    testStatus = null;
    ollamaModels = [];
    apiKey = '';

    try {
      // Fetch single profile to obtain decrypted API key if needed
      const full = await api.getAiProfile(p.id);
      profileName = full.name || '';
      provider = full.provider || 'openai';
      baseUrl = full.base_url || presets[full.provider]?.url || '';
      modelName = full.model_name || presets[full.provider]?.model || '';
      temperature = typeof full.temperature === 'number' ? full.temperature : 0.3;
      language = full.language || 'auto';
      responseStyle = full.response_style || 'normal';
      customPrompt = full.custom_prompt || '';
      includeSchema = full.include_schema !== false;
      maskIdentifiers = full.mask_identifiers !== false;
      isActive = Boolean(full.is_active);
      hasSavedKey = Boolean(full.has_api_key);
      apiKey = full.api_key || '';
    } catch {
      // Fallback to list entry
      profileName = p.name || '';
      provider = p.provider || 'openai';
      baseUrl = p.base_url || '';
      modelName = p.model_name || '';
      temperature = p.temperature ?? 0.3;
      language = p.language || 'auto';
      responseStyle = p.response_style || 'normal';
      customPrompt = p.custom_prompt || '';
      includeSchema = p.include_schema !== false;
      maskIdentifiers = p.mask_identifiers !== false;
      isActive = Boolean(p.is_active);
      hasSavedKey = Boolean(p.has_api_key);
    }
  }

  let lastSelectedProfileId = $state('');

  function startNewProfile() {
    if (selectedProfileId) {
      lastSelectedProfileId = selectedProfileId;
    }
    isCreatingNew = true;
    selectedProfileId = '';
    testStatus = null;
    ollamaModels = [];
    profileName = 'New AI Profile';
    provider = 'openai';
    apiKey = '';
    hasSavedKey = false;
    baseUrl = presets.openai.url;
    modelName = presets.openai.model;
    temperature = 0.3;
    language = 'auto';
    responseStyle = 'normal';
    customPrompt = '';
    includeSchema = true;
    maskIdentifiers = true;
    isActive = profiles.length === 0;
  }

  function cancelNewProfile() {
    isCreatingNew = false;
    testStatus = null;
    ollamaModels = [];
    let target = profiles.find((p) => p.id === lastSelectedProfileId);
    if (!target) {
      target = profiles.find((p) => p.is_active) || profiles[0];
    }
    if (target) {
      selectProfile(target);
    }
  }

  export async function saveConfig() {
    const trimmedName = profileName.trim();
    if (!trimmedName) {
      appState.showToast('Profile name is required', 'warning');
      return;
    }
    const trimmedModel = modelName.trim();
    if (!trimmedModel) {
      appState.showToast('Model identifier is required', 'warning');
      return;
    }

    try {
      const payload: Partial<AIProfile> = {
        name: trimmedName,
        provider,
        api_key: apiKey.trim() || undefined,
        base_url: baseUrl.trim(),
        model_name: trimmedModel,
        temperature,
        language,
        response_style: responseStyle,
        custom_prompt: customPrompt.trim(),
        include_schema: includeSchema,
        mask_identifiers: maskIdentifiers,
        is_active: isActive,
      };

      let saved: AIProfile;
      if (isCreatingNew || !selectedProfileId) {
        saved = await api.createAiProfile(payload);
        appState.showToast(`AI profile "${saved.name}" created!`, 'success');
      } else {
        saved = await api.updateAiProfile(selectedProfileId, payload);
        appState.showToast(`AI profile "${saved.name}" updated!`, 'success');
      }

      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('sqlfox:ai-config-updated'));
      }
      await loadAllProfiles(saved.id);
      onSaved?.();
    } catch (e: any) {
      appState.showToast(`Failed to save AI profile: ${e.message}`, 'error');
    }
  }

  async function activateCurrentProfile() {
    if (!selectedProfileId || isCreatingNew) return;
    try {
      await api.setActiveAiProfile(selectedProfileId);
      isActive = true;
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('sqlfox:ai-config-updated'));
      }
      await loadAllProfiles(selectedProfileId);
      appState.showToast(`Profile activated!`, 'success');
    } catch (e: any) {
      appState.showToast(`Failed to activate profile: ${e.message}`, 'error');
    }
  }

  async function deleteCurrentProfile() {
    if (profiles.length <= 1) {
      appState.showToast('Cannot delete the only remaining profile', 'warning');
      return;
    }
    const current = profiles.find((p) => p.id === selectedProfileId);
    const ok = await appState.showNativeConfirm(
      `Are you sure you want to delete profile "${current?.name || 'this profile'}"?`,
      'Delete AI Profile'
    );
    if (!ok) return;

    try {
      await api.deleteAiProfile(selectedProfileId);
      appState.showToast(`Profile deleted!`, 'success');
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('sqlfox:ai-config-updated'));
      }
      await loadAllProfiles();
    } catch (e: any) {
      appState.showToast(`Failed to delete profile: ${e.message}`, 'error');
    }
  }

  export async function testConnection() {
    isTesting = true;
    testStatus = null;
    try {
      const cfg: AiConfig = {
        provider,
        apiKey: apiKey.trim(),
        baseUrl: baseUrl.trim(),
        modelName: modelName.trim(),
        temperature,
        language,
        responseStyle,
        customPrompt: customPrompt.trim(),
        includeSchema,
        maskIdentifiers,
      };

      const res = await api.aiChat({
        action: 'chat',
        prompt: 'Reply with the single word OK.',
        config: cfg,
        profile_id: selectedProfileId || undefined,
      });
      testStatus = { ok: true, message: `Connected. Model replied: "${(res.content || '').slice(0, 60)}"` };
    } catch (err: any) {
      testStatus = { ok: false, message: err.message || 'Request failed' };
    } finally {
      isTesting = false;
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

  let wasActive = false;
  $effect(() => {
    if (active) {
      if (!wasActive) {
        wasActive = true;
        loadAllProfiles();
      }
    } else {
      wasActive = false;
    }
  });
</script>

<div class="space-y-4">
  <!-- Profile Selector & Toolbar -->
  <div class="p-3 rounded-xl bg-[#070b14] border border-[#1e293b] flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
    <div class="flex items-center space-x-2 flex-1 min-w-0">
      <span class="text-slate-400 text-xs font-semibold whitespace-nowrap">Profile:</span>
      {#if profiles.length > 0}
        <select
          value={isCreatingNew ? '__new__' : selectedProfileId}
          onchange={(e) => {
            const val = e.currentTarget.value;
            if (val === '__new__') {
              startNewProfile();
            } else {
              const found = profiles.find((p) => p.id === val);
              if (found) selectProfile(found);
            }
          }}
          class="bg-[#0b0f19] border border-slate-700 rounded-lg px-2.5 py-1.5 text-xs text-slate-100 font-medium flex-1 focus:outline-none focus:border-sky-500 cursor-pointer"
        >
          {#each profiles as p}
            <option value={p.id}>
              {p.name} {p.is_active ? '★ (Active)' : ''}
            </option>
          {/each}
          {#if isCreatingNew}
            <option value="__new__">
              + {profileName || 'New Profile'} (Draft)
            </option>
          {/if}
        </select>
      {:else}
        <span class="text-xs text-sky-300 italic font-medium px-2 py-1 bg-sky-950/40 rounded border border-sky-800">
          Creating First Profile...
        </span>
      {/if}
    </div>

    <div class="flex items-center space-x-1.5 shrink-0">
      {#if isCreatingNew}
        {#if profiles.length > 0}
          <button
            type="button"
            onclick={cancelNewProfile}
            class="px-2.5 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition flex items-center space-x-1"
            title="Batal membuat profil baru"
          >
            <Icon name="x" class="w-3.5 h-3.5 text-rose-400" />
            <span>Cancel</span>
          </button>
        {/if}
      {:else if selectedProfileId}
        {#if !isActive}
          <button
            type="button"
            onclick={activateCurrentProfile}
            class="px-2.5 py-1.5 rounded-lg bg-emerald-950/80 hover:bg-emerald-900 border border-emerald-700 text-emerald-300 text-xs font-medium transition flex items-center space-x-1"
            title="Set this profile as active Copilot provider"
          >
            <Icon name="check" class="w-3.5 h-3.5" />
            <span>Set Active</span>
          </button>
        {:else}
          <span class="px-2.5 py-1 rounded-lg bg-sky-950/80 border border-sky-800 text-sky-300 text-xs font-semibold flex items-center space-x-1">
            <span class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-pulse"></span>
            <span>Active</span>
          </span>
        {/if}

        <button
          type="button"
          onclick={deleteCurrentProfile}
          disabled={profiles.length <= 1}
          class="p-1.5 rounded-lg border border-rose-900/60 bg-rose-950/40 text-rose-300 hover:bg-rose-900/80 transition disabled:opacity-40 disabled:cursor-not-allowed"
          title={profiles.length <= 1 ? 'Cannot delete the only profile' : 'Delete this profile'}
        >
          <Icon name="trash-2" class="w-3.5 h-3.5" />
        </button>

        <button
          type="button"
          onclick={startNewProfile}
          class="px-2.5 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium transition flex items-center space-x-1"
          title="Add another AI profile"
        >
          <Icon name="plus" class="w-3.5 h-3.5 text-sky-400" />
          <span>New Profile</span>
        </button>
      {/if}
    </div>
  </div>

  <!-- Profile Name -->
  <div>
    <label class="block text-slate-400 text-xs font-medium mb-1">Profile Name *</label>
    <input
      type="text"
      bind:value={profileName}
      placeholder="e.g. OpenAI GPT-4o Mini or Claude Coding"
      class="w-full bg-[#050811] border border-[#1e293b] rounded-lg px-3 py-2 text-slate-200 text-xs focus:outline-none focus:border-sky-500 font-medium"
    />
  </div>

  <!-- Provider Selector -->
  <div>
    <label class="block text-slate-400 text-xs font-medium mb-1.5">AI Provider</label>
    <div class="grid grid-cols-5 gap-1.5">
      {#each ['openai', 'anthropic', 'gemini', 'ollama', 'custom'] as p}
        <button
          type="button"
          onclick={() => onProviderChange(p as any)}
          class="py-1.5 px-2 rounded-lg border text-center font-medium capitalize text-xs transition {provider === p
            ? 'bg-sky-600/30 border-sky-500 text-sky-200 font-bold'
            : 'bg-slate-900 border-slate-800 text-slate-400 hover:border-slate-700'}"
        >
          {p}
        </button>
      {/each}
    </div>
  </div>

  <!-- API Key with Vault Protection Indicator -->
  <div>
    <div class="flex items-center justify-between mb-1">
      <label class="text-slate-400 text-xs font-medium">
        API Key {provider === 'ollama' ? '(Not required for local Ollama)' : '*'}
      </label>
      {#if hasSavedKey && !apiKey}
        <span class="text-[10px] text-emerald-400 font-mono flex items-center space-x-1">
          <Icon name="lock" class="w-3 h-3 text-emerald-400 inline" />
          <span>Encrypted in Vault</span>
        </span>
      {/if}
    </div>
    <div class="relative">
      <input
        type={showKey ? 'text' : 'password'}
        bind:value={apiKey}
        placeholder={hasSavedKey ? '•••••••••••••••• (Encrypted in Vault, leave blank to keep)' : (provider === 'ollama' ? 'Local Ollama requires no key' : 'sk-...')}
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

  <!-- Endpoints & Model -->
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
      <label class="block text-slate-400 text-xs font-medium mb-1">Model Identifier *</label>
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
            ? 'bg-sky-600/30 border-sky-500 text-sky-200 font-bold'
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
      placeholder="e.g. Always prefer CTEs over subqueries, prioritize indexing..."
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
