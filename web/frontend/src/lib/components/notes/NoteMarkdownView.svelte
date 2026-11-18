<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { splitMarkdownSegments, renderMarkdownBlockHtml } from '../../utils/markdown';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  interface Props {
    content: string;
    collapsible?: boolean;
    onOpenQuery?: (sql: string) => void;
  }

  let { content = '', collapsible = true, onOpenQuery }: Props = $props();

  let isExpanded = $state(false);

  let segments = $derived(splitMarkdownSegments(content || ''));
  let shouldShowToggle = $derived(collapsible && (segments.length > 3 || content.length > 320));
  let displayedSegments = $derived(
    isExpanded || !shouldShowToggle ? segments : segments.slice(0, 3)
  );

  function copyCode(text: string) {
    copyText(text);
    appState.showToast('Code copied to clipboard!', 'info');
  }

  function handleOpenInWorksheet(sql: string) {
    if (onOpenQuery) {
      onOpenQuery(sql);
    } else {
      appState.addTab('Note Query.sql', sql);
      appState.showToast('SQL loaded into worksheet', 'success');
    }
  }
</script>

<div class="space-y-2 text-xs text-slate-300 leading-relaxed font-sans">
  {#each displayedSegments as seg}
    {#if seg.type === 'code'}
      <div class="my-1.5 rounded-lg overflow-hidden border border-slate-800 bg-[#050811] text-[11px] font-mono shadow-inner">
        <div class="bg-[#0b0f19] px-2.5 py-1 border-b border-slate-800/80 flex items-center justify-between text-[10px] text-slate-400 select-none">
          <span class="uppercase tracking-wider font-semibold text-slate-500">{seg.lang || 'SQL'}</span>
          <div class="flex items-center space-x-2">
            <button
              type="button"
              onclick={() => copyCode(seg.content)}
              class="hover:text-white p-0.5 rounded transition flex items-center space-x-1"
              title="Copy snippet"
            >
              <Icon name="copy" class="w-3 h-3" />
              <span>Copy</span>
            </button>
            <button
              type="button"
              onclick={() => handleOpenInWorksheet(seg.content)}
              class="hover:text-sky-300 p-0.5 rounded transition flex items-center space-x-1 text-sky-400"
              title="Open in SQL Worksheet"
            >
              <Icon name="play" class="w-2.5 h-2.5" />
              <span>Open</span>
            </button>
          </div>
        </div>
        <pre class="p-2.5 text-sky-200 overflow-x-auto whitespace-pre leading-relaxed select-text">{seg.content}</pre>
      </div>
    {:else}
      <div class="prose-sqlfox">
        {@html renderMarkdownBlockHtml(seg.content)}
      </div>
    {/if}
  {/each}

  {#if shouldShowToggle}
    <button
      type="button"
      onclick={() => (isExpanded = !isExpanded)}
      class="text-[11px] text-amber-400 hover:text-amber-300 transition flex items-center space-x-1.5 font-semibold select-none pt-0.5"
    >
      <Icon name={isExpanded ? 'chevron-up' : 'chevron-down'} class="w-3 h-3 shrink-0" />
      <span>{isExpanded ? 'Show less' : `Show full note (${segments.length} blocks)`}</span>
    </button>
  {/if}
</div>
