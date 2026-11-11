<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  let rawValue = $state('');
  let isValidJson = $state<boolean | null>(null);
  let jsonError = $state('');
  let copied = $state(false);

  let cell = $derived(appState.jsonCellModal);

  $effect(() => {
    if (cell.isOpen) {
      rawValue = cell.value || '';
      validateJson(rawValue);
      copied = false;
    }
  });

  function validateJson(text: string) {
    if (!text.trim()) {
      isValidJson = true;
      jsonError = '';
      return;
    }
    try {
      JSON.parse(text);
      isValidJson = true;
      jsonError = '';
    } catch (err: any) {
      isValidJson = false;
      jsonError = err.message || 'Invalid JSON syntax';
    }
  }

  function formatJson() {
    try {
      const parsed = JSON.parse(rawValue);
      rawValue = JSON.stringify(parsed, null, 2);
      validateJson(rawValue);
    } catch (err: any) {
      appState.showToast(`Cannot format invalid JSON: ${err.message}`, 'warning');
    }
  }

  function minifyJson() {
    try {
      const parsed = JSON.parse(rawValue);
      rawValue = JSON.stringify(parsed);
      validateJson(rawValue);
    } catch (err: any) {
      appState.showToast(`Cannot minify invalid JSON: ${err.message}`, 'warning');
    }
  }

  function copyToClipboard() {
    navigator.clipboard.writeText(rawValue);
    copied = true;
    setTimeout(() => (copied = false), 2000);
    appState.showToast('Copied JSON to clipboard', 'info');
  }

  function handleSave() {
    if (isValidJson === false) {
      if (!confirm('JSON syntax appears invalid. Do you want to save anyway?')) {
        return;
      }
    }
    appState.saveJsonCellValue(rawValue);
    appState.jsonCellModal.isOpen = false;
  }
</script>

{#if cell.isOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm select-none"
    role="dialog"
    aria-modal="true"
  >
    <button
      tabindex="-1"
      aria-label="Close"
      class="fixed inset-0 bg-transparent cursor-default"
      onclick={() => (appState.jsonCellModal.isOpen = false)}
    ></button>

    <div class="w-full max-w-2xl bg-[#0f172a] border border-[#334155] rounded-xl shadow-2xl flex flex-col h-[75vh] max-h-[750px] overflow-hidden relative z-10 animate-in fade-in zoom-in-95 duration-150">
      <!-- Header -->
      <div class="px-5 py-3 border-b border-[#334155] bg-[#0b0f19] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2">
          <Icon name="code" class="w-4 h-4 text-purple-400" />
          <h3 class="font-bold text-sm text-slate-100">
            JSON Cell Inspector
            {#if cell.colName}
              <span class="font-mono text-purple-400 text-xs font-semibold">({cell.colName})</span>
            {/if}
          </h3>
        </div>

        <div class="flex items-center space-x-2">
          {#if isValidJson === true}
            <span class="text-[10px] font-mono text-emerald-400 bg-emerald-950/60 border border-emerald-800/80 px-2 py-0.5 rounded font-semibold">
              Valid JSON
            </span>
          {:else if isValidJson === false}
            <span class="text-[10px] font-mono text-rose-400 bg-rose-950/60 border border-rose-800/80 px-2 py-0.5 rounded font-semibold">
              Syntax Error
            </span>
          {/if}

          <button
            onclick={() => (appState.jsonCellModal.isOpen = false)}
            class="text-slate-400 hover:text-white p-1 rounded hover:bg-slate-800 transition"
          >
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Action Toolbar -->
      <div class="h-9 bg-[#0b0f19]/80 border-b border-[#1e293b] px-3 flex items-center justify-between text-xs shrink-0 select-none">
        <div class="flex items-center space-x-1.5">
          <button
            onclick={formatJson}
            class="h-6 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-200 border border-[#334155] flex items-center space-x-1 text-[11px] transition"
            title="Prettify JSON with 2-space indentation"
          >
            <span>Format</span>
          </button>
          <button
            onclick={minifyJson}
            class="h-6 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-200 border border-[#334155] flex items-center space-x-1 text-[11px] transition"
            title="Minify JSON into single line"
          >
            <span>Minify</span>
          </button>
        </div>

        <button
          onclick={copyToClipboard}
          class="h-6 px-2.5 rounded bg-[#1e293b] hover:bg-slate-700 text-slate-200 border border-[#334155] flex items-center space-x-1 text-[11px] transition"
        >
          <Icon name="copy" class="w-3 h-3 text-sky-400" />
          <span>{copied ? 'Copied!' : 'Copy'}</span>
        </button>
      </div>

      <!-- Editor Textarea -->
      <div class="flex-1 overflow-hidden p-3 bg-[#090d16] flex flex-col relative">
        <textarea
          bind:value={rawValue}
          oninput={(e) => validateJson((e.target as HTMLTextAreaElement).value)}
          placeholder="Paste or enter JSON payload..."
          spellcheck="false"
          class="w-full flex-1 bg-[#090d16] text-slate-100 font-mono text-xs p-3 rounded-lg border border-[#1e293b] focus:border-purple-500 outline-none resize-none leading-relaxed select-text"
        ></textarea>

        {#if jsonError}
          <div class="mt-2 text-[11px] font-mono text-rose-300 bg-rose-950/40 border border-rose-800/60 p-2 rounded shrink-0">
            {jsonError}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-[#334155] bg-[#0b0f19] flex items-center justify-between shrink-0">
        <button
          type="button"
          onclick={() => (appState.jsonCellModal.isOpen = false)}
          class="px-4 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 rounded font-semibold text-xs transition"
        >
          Cancel
        </button>

        {#if !cell.isReadOnly}
          <button
            type="button"
            onclick={handleSave}
            class="px-4 py-1.5 bg-purple-500 hover:bg-purple-400 text-slate-950 font-bold rounded text-xs transition flex items-center space-x-1.5 shadow"
          >
            <Icon name="check" class="w-3.5 h-3.5" />
            <span>Apply to Grid Cell</span>
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}
