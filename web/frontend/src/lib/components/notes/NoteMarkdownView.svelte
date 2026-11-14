<script lang="ts">
import { copyText } from '../../utils/clipboard';
  import { appState } from '../../stores/appState.svelte';
  import Icon from '../common/Icon.svelte';

  interface Props {
    content: string;
    onOpenQuery?: (sql: string) => void;
  }

  let { content, onOpenQuery }: Props = $props();

  let isExpanded = $state(false);

  interface ParsedBlock {
    type: 'header' | 'list' | 'task' | 'quote' | 'code' | 'paragraph';
    level?: number;
    checked?: boolean;
    lang?: string;
    text: string;
  }

  // Parse markdown content into structured blocks
  let blocks = $derived.by<ParsedBlock[]>(() => {
    if (!content) return [];
    const lines = content.split('\n');
    const result: ParsedBlock[] = [];
    let inCode = false;
    let codeLang = '';
    let codeBuffer: string[] = [];

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];

      // Code fence toggling
      if (line.trim().startsWith('```')) {
        if (!inCode) {
          inCode = true;
          codeLang = line.trim().slice(3).trim();
          codeBuffer = [];
        } else {
          inCode = false;
          result.push({
            type: 'code',
            lang: codeLang || 'sql',
            text: codeBuffer.join('\n'),
          });
          codeBuffer = [];
          codeLang = '';
        }
        continue;
      }

      if (inCode) {
        codeBuffer.push(line);
        continue;
      }

      const trimmed = line.trim();
      if (!trimmed) continue;

      // Header
      if (trimmed.startsWith('### ')) {
        result.push({ type: 'header', level: 3, text: trimmed.slice(4) });
      } else if (trimmed.startsWith('## ')) {
        result.push({ type: 'header', level: 2, text: trimmed.slice(3) });
      } else if (trimmed.startsWith('# ')) {
        result.push({ type: 'header', level: 1, text: trimmed.slice(2) });
      }
      // Task checkbox
      else if (trimmed.startsWith('- [x] ') || trimmed.startsWith('- [X] ')) {
        result.push({ type: 'task', checked: true, text: trimmed.slice(6) });
      } else if (trimmed.startsWith('- [ ] ')) {
        result.push({ type: 'task', checked: false, text: trimmed.slice(6) });
      }
      // Unordered list
      else if (trimmed.startsWith('- ') || trimmed.startsWith('* ')) {
        result.push({ type: 'list', text: trimmed.slice(2) });
      }
      // Blockquote
      else if (trimmed.startsWith('> ')) {
        result.push({ type: 'quote', text: trimmed.slice(2) });
      }
      // Paragraph
      else {
        result.push({ type: 'paragraph', text: trimmed });
      }
    }

    // Unclosed code block fallback
    if (inCode && codeBuffer.length > 0) {
      result.push({
        type: 'code',
        lang: codeLang || 'sql',
        text: codeBuffer.join('\n'),
      });
    }

    return result;
  });

  let shouldShowToggle = $derived(blocks.length > 3 || content.length > 220);

  let displayedBlocks = $derived(
    isExpanded || !shouldShowToggle ? blocks : blocks.slice(0, 3)
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

  // Safe inline formatter for bold, italic, and inline code
  function renderInline(text: string): string {
    // Escape HTML first
    let escaped = text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');

    // `code`
    escaped = escaped.replace(/`([^`]+)`/g, '<code class="px-1 py-0.2 rounded bg-[#050811] border border-slate-700/60 text-sky-300 font-mono text-[10.5px]">$1</code>');

    // **bold**
    escaped = escaped.replace(/\*\*([^*]+)\*\*/g, '<strong class="font-bold text-slate-100">$1</strong>');

    // *italic*
    escaped = escaped.replace(/\*([^*]+)\*/g, '<em class="italic text-slate-300">$1</em>');

    return escaped;
  }
</script>

<div class="space-y-1.5 text-xs text-slate-300 leading-relaxed font-sans">
  {#each displayedBlocks as block}
    {#if block.type === 'header'}
      {#if block.level === 1}
        <h3 class="font-bold text-slate-100 text-sm border-b border-slate-800 pb-1 mt-2">
          {@html renderInline(block.text)}
        </h3>
      {:else if block.level === 2}
        <h4 class="font-bold text-slate-200 text-xs mt-1.5 text-sky-300">
          {@html renderInline(block.text)}
        </h4>
      {:else}
        <h5 class="font-semibold text-slate-300 text-xs mt-1">
          {@html renderInline(block.text)}
        </h5>
      {/if}
    {:else if block.type === 'task'}
      <div class="flex items-center space-x-1.5 text-[11.5px] py-0.5">
        <input
          type="checkbox"
          checked={block.checked}
          disabled
          class="rounded bg-slate-900 border-slate-700 text-sky-500 w-3 h-3 cursor-default"
        />
        <span class={block.checked ? 'line-through text-slate-500' : 'text-slate-200'}>
          {@html renderInline(block.text)}
        </span>
      </div>
    {:else if block.type === 'list'}
      <div class="flex items-start space-x-1.5 text-[11.5px] pl-1">
        <span class="text-sky-400 select-none">•</span>
        <span class="flex-1">{@html renderInline(block.text)}</span>
      </div>
    {:else if block.type === 'quote'}
      <blockquote class="border-l-2 border-amber-500/70 pl-2 text-slate-400 italic text-[11px] my-1 bg-amber-950/20 py-0.5 rounded-r">
        {@html renderInline(block.text)}
      </blockquote>
    {:else if block.type === 'code'}
      <div class="my-1.5 rounded-lg overflow-hidden border border-slate-800 bg-[#050811] text-[11px] font-mono shadow-inner">
        <div class="bg-[#0b0f19] px-2.5 py-1 border-b border-slate-800/80 flex items-center justify-between text-[10px] text-slate-400 select-none">
          <span class="uppercase tracking-wider font-semibold text-slate-500">{block.lang || 'SQL'}</span>
          <div class="flex items-center space-x-1.5">
            <button
              type="button"
              onclick={() => copyCode(block.text)}
              class="hover:text-white p-0.5 rounded transition"
              title="Copy snippet"
            >
              <Icon name="copy" class="w-3 h-3" />
            </button>
            <button
              type="button"
              onclick={() => handleOpenInWorksheet(block.text)}
              class="hover:text-sky-300 p-0.5 rounded transition flex items-center space-x-0.5 text-sky-400"
              title="Open in SQL Worksheet"
            >
              <Icon name="play" class="w-2.5 h-2.5" />
              <span>Run</span>
            </button>
          </div>
        </div>
        <pre class="p-2 text-sky-200 overflow-x-auto whitespace-pre leading-relaxed select-text">{block.text}</pre>
      </div>
    {:else}
      <p class="text-[11.5px] leading-relaxed text-slate-300">
        {@html renderInline(block.text)}
      </p>
    {/if}
  {/each}

  {#if shouldShowToggle}
    <button
      type="button"
      onclick={() => (isExpanded = !isExpanded)}
      class="text-[11px] text-amber-400 hover:text-amber-300 transition flex items-center space-x-1.5 font-semibold select-none pt-0.5"
    >
      <Icon name={isExpanded ? 'chevron-up' : 'chevron-down'} class="w-3 h-3 shrink-0" />
      <span>{isExpanded ? 'Show less' : `Show full note (${blocks.length} blocks)`}</span>
    </button>
  {/if}
</div>
