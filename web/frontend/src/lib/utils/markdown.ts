/**
 * Shared XSS-safe Markdown renderer & segmenter for AI Copilot, Operational Notes,
 * Markdown scratchpad tabs, and Release Changelog views.
 */

export function escapeHtml(str: unknown): string {
  return String(str ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

export interface MarkdownSegment {
  type: 'markdown' | 'code';
  content: string;
  lang?: string;
}

export function splitMarkdownSegments(raw: string): MarkdownSegment[] {
  if (!raw) return [];
  const text = String(raw);
  const parts: MarkdownSegment[] = [];
  const fenceRegex = /```([a-zA-Z0-9_-]*)[ \t]*\r?\n([\s\S]*?)```/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = fenceRegex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push({ type: 'markdown', content: text.slice(lastIndex, match.index) });
    }
    parts.push({
      type: 'code',
      lang: (match[1] || 'text').toLowerCase(),
      content: match[2].replace(/\n$/, ''),
    });
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < text.length) {
    parts.push({ type: 'markdown', content: text.slice(lastIndex) });
  }
  return parts;
}

function renderInlineMarkdown(escapedText: string): string {
  // Protect inline code and links before emphasis passes so underscores/asterisks inside them aren't mangled.
  const tokens: string[] = [];
  const hold = (html: string) => {
    const idx = tokens.push(html) - 1;
    return `\u0000MD${idx}\u0000`;
  };

  let out = escapedText
    .replace(/`([^`\n]+)`/g, (_m, code) =>
      hold(`<code class="px-1.5 py-0.5 rounded bg-slate-800/90 border border-slate-700/70 text-sky-300 font-mono text-[11px]">${code}</code>`)
    )
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (_m, label, url) =>
      hold(`<a href="${url}" target="_blank" rel="noopener noreferrer" class="text-sky-400 hover:text-sky-300 underline">${label}</a>`)
    )
    .replace(/\*\*([^*]+)\*\*/g, '<strong class="font-semibold text-slate-100">$1</strong>')
    .replace(/__([^_]+)__/g, '<strong class="font-semibold text-slate-100">$1</strong>')
    .replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em class="italic text-slate-300">$2</em>')
    .replace(/(^|[^_])_([^_\n]+)_/g, '$1<em class="italic text-slate-300">$2</em>');

  return out.replace(/\u0000MD(\d+)\u0000/g, (_m, idx) => tokens[Number(idx)] ?? '');
}

export function renderMarkdownBlockHtml(raw: string): string {
  if (!raw) return '';
  const lines = String(raw).split(/\r?\n/);
  const html: string[] = [];
  let inUl = false;
  let inOl = false;
  let inTable = false;
  let tableHeaderDone = false;

  const closeLists = () => {
    if (inUl) { html.push('</ul>'); inUl = false; }
    if (inOl) { html.push('</ol>'); inOl = false; }
  };
  const closeTable = () => {
    if (inTable) {
      html.push(tableHeaderDone ? '</tbody></table></div>' : '</table></div>');
      inTable = false;
      tableHeaderDone = false;
    }
  };

  for (let i = 0; i < lines.length; i++) {
    const rawLine = lines[i];
    const trimmed = rawLine.trim();

    if (!trimmed) {
      closeLists();
      closeTable();
      continue;
    }

    // Horizontal rule
    if (/^([-*_]){3,}$/.test(trimmed)) {
      closeLists();
      closeTable();
      html.push('<hr class="my-2.5 border-slate-800" />');
      continue;
    }

    // Headings H1-H6
    const hMatch = trimmed.match(/^(#{1,6})\s+(.*)$/);
    if (hMatch) {
      closeLists();
      closeTable();
      const level = hMatch[1].length;
      const content = renderInlineMarkdown(escapeHtml(hMatch[2]));
      const cls =
        level === 1
          ? 'text-sm font-bold text-slate-100 mt-2 mb-1 pb-1 border-b border-slate-800'
          : level === 2
          ? 'text-[13px] font-bold text-slate-100 mt-2 mb-1'
          : 'text-xs font-semibold text-sky-300 mt-1.5 mb-0.5';
      html.push(`<h${level} class="${cls}">${content}</h${level}>`);
      continue;
    }

    // Markdown table row
    if (trimmed.startsWith('|') && trimmed.includes('|', 1)) {
      closeLists();
      const cells = trimmed.replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());
      if (cells.every((c) => /^:?-{2,}:?$/.test(c))) {
        if (inTable && !tableHeaderDone) {
          html.push('</thead><tbody>');
          tableHeaderDone = true;
        }
        continue;
      }
      if (!inTable) {
        inTable = true;
        tableHeaderDone = false;
        html.push('<div class="overflow-x-auto my-2 rounded border border-slate-800"><table class="w-full text-[11px] border-collapse">');
        html.push('<thead><tr class="bg-slate-900/90 text-slate-300 border-b border-slate-800">');
        for (const c of cells) {
          html.push(`<th class="text-left px-2.5 py-1.5 font-semibold">${renderInlineMarkdown(escapeHtml(c))}</th>`);
        }
        html.push('</tr>');
        // Peek if next line is separator
        const nextLine = (lines[i + 1] || '').trim();
        if (!nextLine.startsWith('|') || !nextLine.includes('-')) {
          html.push('</thead><tbody>');
          tableHeaderDone = true;
        }
      } else {
        if (!tableHeaderDone) {
          html.push('</thead><tbody>');
          tableHeaderDone = true;
        }
        html.push('<tr class="border-b border-slate-800/60 hover:bg-slate-800/30">');
        for (const c of cells) {
          html.push(`<td class="px-2.5 py-1.5 text-slate-300">${renderInlineMarkdown(escapeHtml(c))}</td>`);
        }
        html.push('</tr>');
      }
      continue;
    } else {
      closeTable();
    }

    // Blockquote
    if (trimmed.startsWith('>')) {
      closeLists();
      const text = trimmed.replace(/^>\s?/, '');
      html.push(`<blockquote class="border-l-2 border-sky-500/60 pl-2.5 py-0.5 my-1 text-slate-300 italic bg-slate-900/40 rounded-r">${renderInlineMarkdown(escapeHtml(text))}</blockquote>`);
      continue;
    }

    // Task list / Unordered list
    const ulMatch = trimmed.match(/^[-*+]\s+(.*)$/);
    if (ulMatch) {
      if (inOl) { html.push('</ol>'); inOl = false; }
      if (!inUl) {
        html.push('<ul class="list-disc list-inside space-y-0.5 my-1 text-slate-300">');
        inUl = true;
      }
      let itemText = ulMatch[1];
      const taskMatch = itemText.match(/^\[( |x|X)\]\s+(.*)$/);
      if (taskMatch) {
        const checked = taskMatch[1].toLowerCase() === 'x';
        itemText = `${checked ? '☑' : '☐'} ${taskMatch[2]}`;
      }
      html.push(`<li class="leading-relaxed">${renderInlineMarkdown(escapeHtml(itemText))}</li>`);
      continue;
    }

    // Ordered list
    const olMatch = trimmed.match(/^\d+\.\s+(.*)$/);
    if (olMatch) {
      if (inUl) { html.push('</ul>'); inUl = false; }
      if (!inOl) {
        html.push('<ol class="list-decimal list-inside space-y-0.5 my-1 text-slate-300">');
        inOl = true;
      }
      html.push(`<li class="leading-relaxed">${renderInlineMarkdown(escapeHtml(olMatch[1]))}</li>`);
      continue;
    }

    closeLists();
    html.push(`<p class="leading-relaxed my-1 text-slate-200">${renderInlineMarkdown(escapeHtml(rawLine))}</p>`);
  }

  closeLists();
  closeTable();
  return html.join('');
}

export function renderFullMarkdownHtml(raw: string): string {
  const parts = splitMarkdownSegments(raw);
  return parts
    .map((p) => {
      if (p.type === 'code') {
        const langBadge = p.lang ? `<div class="px-2.5 py-1 bg-slate-900 border-b border-slate-800 text-[10px] font-mono uppercase text-slate-400">${escapeHtml(p.lang)}</div>` : '';
        return `<div class="my-2 rounded-lg border border-slate-800 bg-[#050811] overflow-hidden">${langBadge}<pre class="p-2.5 text-[11px] font-mono text-sky-300 overflow-x-auto whitespace-pre-wrap">${escapeHtml(p.content)}</pre></div>`;
      }
      return renderMarkdownBlockHtml(p.content);
    })
    .join('');
}
