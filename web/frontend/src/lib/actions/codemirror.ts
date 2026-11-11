import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLineGutter,
  highlightSpecialChars,
  drawSelection,
  highlightActiveLine,
  Decoration,
  type DecorationSet,
  dropCursor,
  rectangularSelection,
  crosshairCursor,
} from '@codemirror/view';
import { EditorState, StateEffect, StateField, Compartment, Prec } from '@codemirror/state';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { sql, MySQL, PostgreSQL, SQLite, StandardSQL } from '@codemirror/lang-sql';
import {
  autocompletion,
  completionKeymap,
  closeBrackets,
  closeBracketsKeymap,
  type CompletionContext,
  type CompletionResult,
} from '@codemirror/autocomplete';
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import { bracketMatching, syntaxHighlighting, HighlightStyle, indentOnInput } from '@codemirror/language';
import { tags } from '@lezer/highlight';

export interface CodeMirrorOptions {
  doc: string;
  dialect?: 'mysql' | 'postgres' | 'sqlite';
  schema?: Record<string, string[]>;
  tables?: Array<{ name: string; type?: string; columns?: Array<{ name: string; type?: string } | string> }>;
  databases?: string[];
  snippets?: Array<{ title: string; sql: string; tags?: string }>;
  onExecute?: () => void;
  onExecuteAtCursor?: (statement: string) => void;
  onFormat?: () => void;
  onChange?: (val: string) => void;
}

export interface StatementPos {
  start: number;
  end: number;
  text: string;
}

// ponytail: Flash execution decoration highlights the statement being executed via Ctrl+Enter
const addFlashEffect = StateEffect.define<{ from: number; to: number }>();
const clearFlashEffect = StateEffect.define<null>();
const flashMark = Decoration.mark({ class: 'cm-flash-exec' });

const flashField = StateField.define<DecorationSet>({
  create() {
    return Decoration.none;
  },
  update(flashes, tr) {
    flashes = flashes.map(tr.changes);
    for (const e of tr.effects) {
      if (e.is(addFlashEffect)) {
        try {
          flashes = Decoration.set([flashMark.range(e.value.from, e.value.to)]);
        } catch (_) {}
      } else if (e.is(clearFlashEffect)) {
        flashes = Decoration.none;
      }
    }
    return flashes;
  },
  provide: (f) => EditorView.decorations.from(f),
});

function flashStatement(view: EditorView, from: number, to: number) {
  if (from >= to) return;
  view.dispatch({
    effects: addFlashEffect.of({ from, to }),
  });
  setTimeout(() => {
    try {
      view.dispatch({ effects: clearFlashEffect.of(null) });
    } catch (_) {}
  }, 350);
}

// ponytail: High-contrast syntax palette matching SqlFox's signature dark aesthetic
export const sqlfoxHighlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: '#f472b6', fontWeight: '600' },
  { tag: tags.string, color: '#34d399' },
  { tag: tags.number, color: '#fbbf24' },
  { tag: tags.typeName, color: '#c084fc' },
  { tag: [tags.bool, tags.null, tags.atom], color: '#fb923c' },
  { tag: [tags.function(tags.variableName), tags.standard(tags.variableName)], color: '#38bdf8' },
  { tag: [tags.operator, tags.punctuation], color: '#94a3b8' },
  { tag: tags.bracket, color: '#cbd5e1' },
  { tag: [tags.lineComment, tags.blockComment, tags.comment], color: '#64748b', fontStyle: 'italic' },
  { tag: tags.variableName, color: '#7dd3fc' },
]);

export const sqlfoxTheme = EditorView.theme(
  {
    '&': {
      height: '100%',
      width: '100%',
      backgroundColor: '#090d16',
      color: '#e0f2fe',
      fontFamily: "'JetBrains Mono', 'Fira Code', ui-monospace, monospace",
      fontSize: '13.5px',
      lineHeight: '1.6',
      letterSpacing: '0.01em',
    },
    '.cm-scroller': {
      height: '100%',
      width: '100%',
      overflow: 'auto',
      fontFamily: 'inherit',
    },
    '.cm-content': {
      minHeight: '100%',
      boxSizing: 'border-box',
      caretColor: '#38bdf8',
      padding: '8px 0',
    },
    '.cm-cursor, .cm-dropCursor': {
      borderLeftColor: '#38bdf8',
      borderLeftWidth: '2px',
    },
    '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
      backgroundColor: 'rgba(56, 189, 248, 0.25) !important',
    },
    '.cm-gutters': {
      backgroundColor: '#090d16',
      color: '#475569',
      borderRight: '1px solid #1e293b',
    },
    '.cm-gutterElement': {
      padding: '0 8px 0 4px',
    },
    '.cm-activeLineGutter': {
      backgroundColor: '#0f172a',
      color: '#94a3b8',
    },
    '.cm-activeLine': {
      backgroundColor: 'rgba(30, 41, 59, 0.35)',
    },
    '&.cm-focused .cm-matchingBracket, .cm-matchingBracket': {
      color: '#38bdf8 !important',
      backgroundColor: 'rgba(56, 189, 248, 0.2) !important',
      outline: '1px solid rgba(56, 189, 248, 0.4) !important',
      borderRadius: '2px',
    },
    '&.cm-focused .cm-nonmatchingBracket, .cm-nonmatchingBracket': {
      color: '#f87171 !important',
      backgroundColor: 'rgba(239, 68, 68, 0.2) !important',
      outline: '1px solid rgba(239, 68, 68, 0.4) !important',
      borderRadius: '2px',
    },
    '.cm-tooltip.cm-tooltip-autocomplete': {
      backgroundColor: '#090d16 !important',
      border: '1px solid #1e293b !important',
      borderRadius: '6px !important',
      boxShadow: '0 10px 25px -5px rgba(0, 0, 0, 0.7), 0 8px 10px -6px rgba(0, 0, 0, 0.7) !important',
      zIndex: '100 !important',
    },
    '.cm-tooltip-autocomplete ul': {
      maxHeight: '240px',
      padding: '4px',
      fontFamily: "'JetBrains Mono', ui-monospace, monospace",
      fontSize: '13px',
    },
    '.cm-tooltip-autocomplete ul li': {
      display: 'flex',
      alignItems: 'center',
      padding: '4px 8px',
      borderRadius: '4px',
      color: '#cbd5e1',
      cursor: 'pointer',
    },
    '.cm-tooltip-autocomplete ul li[aria-selected]': {
      backgroundColor: '#1e293b !important',
      color: '#38bdf8 !important',
    },
    '.cm-flash-exec': {
      backgroundColor: 'rgba(56, 189, 248, 0.35) !important',
      borderRadius: '2px !important',
      transition: 'background-color 0.35s ease !important',
    },
  },
  { dark: true }
);

const SQL_KEYWORDS = [
  'SELECT', 'FROM', 'WHERE', 'INSERT INTO', 'UPDATE', 'DELETE FROM',
  'JOIN', 'INNER JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'CROSS JOIN',
  'ON', 'GROUP BY', 'ORDER BY', 'HAVING', 'LIMIT', 'OFFSET',
  'UNION ALL', 'UNION', 'DISTINCT', 'AS', 'AND', 'OR', 'NOT',
  'IN', 'BETWEEN', 'LIKE', 'IS NULL', 'IS NOT NULL', 'EXISTS',
  'CASE', 'WHEN', 'THEN', 'ELSE', 'END', 'CREATE TABLE',
  'ALTER TABLE', 'DROP TABLE', 'TRUNCATE TABLE', 'INDEX',
  'PRIMARY KEY', 'FOREIGN KEY', 'SET', 'VALUES', 'ASC', 'DESC',
  'DEFAULT', 'AUTO_INCREMENT', 'DATABASE', 'SCHEMA', 'SHOW TABLES',
];

const SQL_FUNCTIONS = [
  'COUNT()', 'SUM()', 'AVG()', 'MIN()', 'MAX()', 'COALESCE()',
  'CONCAT()', 'NOW()', 'DATE()', 'IFNULL()', 'LOWER()', 'UPPER()',
  'SUBSTRING()', 'TRIM()', 'ROUND()', 'LENGTH()', 'CURRENT_TIMESTAMP()',
];

function getStatementAtPos(text: string, pos: number): StatementPos {
  if (!text.trim()) return { start: 0, end: 0, text: '' };
  let inSingle = false;
  let inDouble = false;
  let inBacktick = false;
  let inLineComment = false;
  let inBlockComment = false;
  let stmtStart = 0;
  const stmts: StatementPos[] = [];

  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    const next = i + 1 < text.length ? text[i + 1] : '';

    if (inLineComment) {
      if (ch === '\n' || ch === '\r') inLineComment = false;
      continue;
    }
    if (inBlockComment) {
      if (ch === '*' && next === '/') {
        inBlockComment = false;
        i++;
      }
      continue;
    }
    if (inSingle) {
      if (ch === '\\') { i++; continue; }
      if (ch === "'") inSingle = false;
      continue;
    }
    if (inDouble) {
      if (ch === '\\') { i++; continue; }
      if (ch === '"') inDouble = false;
      continue;
    }
    if (inBacktick) {
      if (ch === '`') inBacktick = false;
      continue;
    }

    if (ch === '-' && next === '-') {
      inLineComment = true;
      i++;
      continue;
    }
    if (ch === '/' && next === '*') {
      inBlockComment = true;
      i++;
      continue;
    }
    if (ch === "'") { inSingle = true; continue; }
    if (ch === '"') { inDouble = true; continue; }
    if (ch === '`') { inBacktick = true; continue; }

    if (ch === ';') {
      stmts.push({
        start: stmtStart,
        end: i + 1,
        text: text.slice(stmtStart, i + 1).trim(),
      });
      stmtStart = i + 1;
    }
  }

  if (stmtStart < text.length) {
    stmts.push({
      start: stmtStart,
      end: text.length,
      text: text.slice(stmtStart).trim(),
    });
  }

  const found = stmts.find((s) => pos >= s.start && pos <= s.end);
  return found || { start: 0, end: text.length, text: text.trim() };
}

// ponytail: Context-aware completion source with dot dereferencing, aliases, and visual badge scoring
function createSqlCompletionSource(getOpts: () => CodeMirrorOptions) {
  return (context: CompletionContext): CompletionResult | null => {
    const word = context.matchBefore(/[a-zA-Z0-9_.]*/);
    if (!word && !context.explicit) return null;
    if (!context.explicit && (!word || word.text === '' || word.from === word.to)) return null;

    const fullText = context.state.doc.toString();
    const cursorIndex = context.pos;
    const opts = getOpts();

    // Statement-bounded scoping
    const lastSemi = fullText.lastIndexOf(';', cursorIndex > 0 ? cursorIndex - 1 : 0);
    const nextSemi = fullText.indexOf(';', cursorIndex);
    const stmtStart = lastSemi >= 0 ? lastSemi + 1 : 0;
    const stmtEnd = nextSemi >= 0 ? nextSemi : fullText.length;
    const stmt = fullText.slice(stmtStart, stmtEnd);
    const textBeforeCursor = fullText.slice(0, cursorIndex);
    const textBeforeInStmt = fullText.slice(stmtStart, cursorIndex);

    const availableDatabases = new Set((opts.databases || []).map((d) => d.toLowerCase()));

    // Extract table aliases in active statement: e.g. FROM users u, JOIN orders AS o
    const aliases: Record<string, string> = {};
    const aliasRegex = /\b(?:FROM|JOIN)\s+([`"]?[a-zA-Z0-9_]+(?:\.[a-zA-Z0-9_]+)?[`"]?)(?:\s+(?:AS\s+)?([`"]?[a-zA-Z0-9_]+[`"]?))?/gi;
    let match;
    while ((match = aliasRegex.exec(stmt)) !== null) {
      const rawTable = match[1].replace(/[`"]/g, '');
      const rawAlias = match[2] ? match[2].replace(/[`"]/g, '') : null;
      if (rawAlias && !/^(WHERE|ON|JOIN|LEFT|RIGHT|INNER|GROUP|ORDER|LIMIT|SET)$/i.test(rawAlias)) {
        aliases[rawAlias.toLowerCase()] = rawTable;
      }
    }

    const lineUpToCursor = textBeforeCursor.slice(textBeforeCursor.lastIndexOf('\n') + 1);
    const twoDotMatch = lineUpToCursor.match(/([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]*)$/);
    const oneDotMatch = lineUpToCursor.match(/([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]*)$/);
    const wordMatch = lineUpToCursor.match(/([a-zA-Z0-9_]+)$/);

    interface Candidate {
      text: string;
      displayText?: string;
      kind: string;
      badgeClass?: string;
      type?: string;
      score: number;
    }

    let candidates: Candidate[] = [];
    let partial = '';
    let fromPos = context.pos;

    // 1. Two dot dereferencing: db.table.column
    if (twoDotMatch) {
      const tblPart = twoDotMatch[2].toLowerCase();
      const colPartial = (twoDotMatch[3] || '').toLowerCase();
      partial = twoDotMatch[3] || '';
      fromPos = context.pos - partial.length;

      const matchedTable = (opts.tables || []).find((t) => t.name.toLowerCase() === tblPart);
      if (matchedTable && matchedTable.columns) {
        matchedTable.columns.forEach((c) => {
          const colName = typeof c === 'string' ? c : c.name;
          const colType = typeof c === 'string' ? '' : c.type || '';
          const lowerCol = colName.toLowerCase();
          if (!colPartial || lowerCol.includes(colPartial)) {
            let score = 60;
            if (lowerCol === colPartial) score += 100;
            else if (lowerCol.startsWith(colPartial)) score += 50;
            candidates.push({
              text: colName,
              kind: 'COL',
              badgeClass: 'cm-badge-col',
              type: colType,
              score,
            });
          }
        });
      }
    } else if (oneDotMatch) {
      // 2. Single dot dereferencing: db.table OR table.col OR alias.col
      const qualifier = oneDotMatch[1];
      partial = oneDotMatch[2] || '';
      fromPos = context.pos - partial.length;
      const lowerQualifier = qualifier.toLowerCase();
      const lowerPartial = partial.toLowerCase();

      if (availableDatabases.has(lowerQualifier)) {
        (opts.tables || []).forEach((tbl) => {
          const lowerTbl = tbl.name.toLowerCase();
          if (!lowerPartial || lowerTbl.includes(lowerPartial)) {
            let score = 60;
            if (lowerTbl === lowerPartial) score += 100;
            else if (lowerTbl.startsWith(lowerPartial)) score += 50;
            candidates.push({
              text: tbl.name,
              kind: 'TBL',
              badgeClass: 'cm-badge-tbl',
              type: tbl.type || 'TABLE',
              score,
            });
          }
        });
      } else {
        const resolved = aliases[lowerQualifier] || qualifier;
        const matchedTable = (opts.tables || []).find((t) => t.name.toLowerCase() === resolved.toLowerCase());
        if (matchedTable && matchedTable.columns) {
          matchedTable.columns.forEach((c) => {
            const colName = typeof c === 'string' ? c : c.name;
            const colType = typeof c === 'string' ? '' : c.type || '';
            const lowerCol = colName.toLowerCase();
            if (!lowerPartial || lowerCol.includes(lowerPartial)) {
              let score = 50;
              if (lowerCol === lowerPartial) score += 100;
              else if (lowerCol.startsWith(lowerPartial)) score += 50;
              candidates.push({
                text: colName,
                kind: 'COL',
                badgeClass: 'cm-badge-col',
                type: colType,
                score,
              });
            }
          });
        }
      }
    } else {
      // 3. No dot: Contextual completions based on preceding SQL clause
      if (wordMatch) {
        partial = wordMatch[1] || '';
        fromPos = context.pos - partial.length;
      }
      const lowerPartial = partial.toLowerCase();

      const cleanBefore = textBeforeInStmt.replace(/--.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '');
      const keywords = [
        ...cleanBefore.matchAll(
          /\b(SELECT|FROM|JOIN|LEFT\s+JOIN|RIGHT\s+JOIN|INNER\s+JOIN|WHERE|ON|GROUP\s+BY|ORDER\s+BY|SET|INTO|UPDATE)\b/gi
        ),
      ];
      const clause = keywords.length > 0 ? keywords[keywords.length - 1][1].toUpperCase().replace(/\s+/g, ' ') : '';
      const isTableClause = /^(FROM|JOIN|LEFT JOIN|RIGHT JOIN|INNER JOIN|INTO|UPDATE)$/.test(clause);
      const isColumnClause = /^(SELECT|WHERE|ON|GROUP BY|ORDER BY|SET|HAVING)$/.test(clause);

      // Databases suggestion in table clause
      if (isTableClause && availableDatabases.size > 0) {
        availableDatabases.forEach((dbName) => {
          if (!lowerPartial || dbName.includes(lowerPartial)) {
            let score = 115;
            if (dbName === lowerPartial) score += 80;
            else if (dbName.startsWith(lowerPartial)) score += 40;
            candidates.push({
              text: dbName,
              kind: 'DB',
              badgeClass: 'cm-badge-db',
              type: 'DATABASE',
              score,
            });
          }
        });
      }

      // Tables suggestion
      (opts.tables || []).forEach((tbl) => {
        const lowerTbl = tbl.name.toLowerCase();
        if (!lowerPartial || lowerTbl.includes(lowerPartial)) {
          let score = 30;
          if (lowerTbl === lowerPartial) score += 100;
          else if (lowerTbl.startsWith(lowerPartial)) score += 50;
          if (isTableClause) score += 120;
          candidates.push({
            text: tbl.name,
            kind: 'TBL',
            badgeClass: 'cm-badge-tbl',
            type: tbl.type || 'TABLE',
            score,
          });
        }

        // Columns suggestion
        if (tbl.columns) {
          const isMentioned = stmt.toLowerCase().includes(tbl.name.toLowerCase());
          tbl.columns.forEach((c) => {
            const colName = typeof c === 'string' ? c : c.name;
            const colType = typeof c === 'string' ? '' : c.type || '';
            const lowerCol = colName.toLowerCase();
            if (!lowerPartial || lowerCol.includes(lowerPartial)) {
              let score = 20;
              if (lowerCol === lowerPartial) score += 100;
              else if (lowerCol.startsWith(lowerPartial)) score += 50;
              if (isColumnClause) score += 60;
              if (isMentioned) score += 30;
              candidates.push({
                text: colName,
                kind: 'COL',
                badgeClass: 'cm-badge-col',
                type: colType ? `${tbl.name}.${colType}` : tbl.name,
                score,
              });
            }
          });
        }
      });

      // SQL Functions
      SQL_FUNCTIONS.forEach((fn) => {
        const baseName = fn.replace('()', '');
        const lowerFn = baseName.toLowerCase();
        if (!lowerPartial || lowerFn.includes(lowerPartial)) {
          let score = 25;
          if (lowerFn === lowerPartial) score += 100;
          else if (lowerFn.startsWith(lowerPartial)) score += 40;
          if (isColumnClause) score += 30;
          if (isTableClause) score -= 40;
          candidates.push({
            text: fn,
            kind: 'FN',
            badgeClass: 'cm-badge-fn',
            type: 'FUNCTION',
            score,
          });
        }
      });

      // SQL Keywords
      SQL_KEYWORDS.forEach((kw) => {
        const lowerKw = kw.toLowerCase();
        if (!lowerPartial || lowerKw.includes(lowerPartial)) {
          let score = 10;
          if (lowerKw === lowerPartial) score += 100;
          else if (lowerKw.startsWith(lowerPartial)) score += 45;
          if (!isTableClause && !isColumnClause) score += 30;
          candidates.push({
            text: kw,
            kind: 'KW',
            badgeClass: 'cm-badge-kw',
            type: 'KEYWORD',
            score,
          });
        }
      });

      // Snippets
      if (opts.snippets && lowerPartial && lowerPartial.length >= 2) {
        opts.snippets.forEach((s) => {
          const lowerTitle = (s.title || '').toLowerCase();
          const lowerTags = (s.tags || '').toLowerCase();
          if (lowerTitle.includes(lowerPartial) || lowerTags.includes(lowerPartial)) {
            let score = 35;
            if (lowerTitle.startsWith(lowerPartial)) score += 55;
            candidates.push({
              text: s.sql || s.title,
              displayText: s.title,
              kind: 'SNP',
              badgeClass: 'cm-badge-snp',
              type: s.tags ? `SNIPPET (${s.tags})` : 'SNIPPET',
              score,
            });
          }
        });
      }
    }

    // Deduplicate and rank candidates
    const seen = new Set<string>();
    candidates = candidates.filter((item) => {
      const key = `${item.kind}:${item.text}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
    candidates.sort((a, b) => b.score - a.score || a.text.localeCompare(b.text));

    if (candidates.length === 0) return null;

    return {
      from: Math.max(0, fromPos),
      to: context.pos,
      options: candidates.slice(0, 50).map((item) => ({
        label: item.displayText || item.text,
        displayLabel: item.displayText || item.text,
        kind: item.kind,
        badgeClass: item.badgeClass,
        type: item.kind ? item.kind.toLowerCase() : 'text',
        detail: item.type || '',
        boost: (item.score || 0) / 100,
        apply: (view, completion, from, to) => {
          view.dispatch({
            changes: { from, to, insert: item.text },
            selection: { anchor: from + item.text.length, head: from + item.text.length },
          });
        },
      })),
      filter: false,
    };
  };
}

export function codemirror(node: HTMLElement, options: CodeMirrorOptions) {
  let currentOptions = { ...options };
  const dialectCompartment = new Compartment();

  function resolveDialect(d?: string) {
    if (d === 'postgres') return PostgreSQL;
    if (d === 'sqlite') return SQLite;
    return MySQL;
  }

  const runKeymap = keymap.of([
    {
      key: 'Ctrl-Enter',
      mac: 'Cmd-Enter',
      run: (view) => {
        const text = view.state.doc.toString();
        const sel = view.state.selection.main;
        if (!sel.empty) {
          flashStatement(view, sel.from, sel.to);
        } else {
          const stmtPos = getStatementAtPos(text, sel.head);
          flashStatement(view, stmtPos.start, stmtPos.end);
        }
        if (currentOptions.onExecute) {
          currentOptions.onExecute();
          return true;
        }
        return false;
      },
    },
    {
      key: 'Ctrl-Shift-Enter',
      mac: 'Cmd-Shift-Enter',
      run: (view) => {
        const pos = view.state.selection.main.head;
        const text = view.state.doc.toString();
        const stmtPos = getStatementAtPos(text, pos);
        flashStatement(view, stmtPos.start, stmtPos.end);
        if (currentOptions.onExecuteAtCursor) {
          currentOptions.onExecuteAtCursor(stmtPos.text);
          return true;
        }
        return false;
      },
    },
    {
      key: 'Ctrl-Shift-f',
      mac: 'Cmd-Shift-f',
      run: () => {
        if (currentOptions.onFormat) {
          currentOptions.onFormat();
          return true;
        }
        return false;
      },
    },
    ...closeBracketsKeymap,
    ...defaultKeymap,
    ...searchKeymap,
    ...historyKeymap,
    ...completionKeymap,
    indentWithTab,
  ]);

  const updateListener = EditorView.updateListener.of((update) => {
    if (update.docChanged && currentOptions.onChange) {
      currentOptions.onChange(update.state.doc.toString());
    }
  });

  const autocompletionConfig = {
    override: [createSqlCompletionSource(() => currentOptions)],
    defaultKeymap: true,
    icons: false,
    addToOptions: [
      {
        render(completion: any) {
          const kind = completion.kind || (completion.type ? completion.type.toUpperCase() : '');
          if (!kind) return null;
          const badge = document.createElement('span');
          const cls = completion.badgeClass || 'cm-badge-' + kind.toLowerCase();
          badge.className = 'cm-hint-badge ' + cls;
          badge.textContent = kind;
          return badge;
        },
        position: 10,
      },
    ],
  };

  const state = EditorState.create({
    doc: currentOptions.doc || '',
    extensions: [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      drawSelection(),
      dropCursor(),
      rectangularSelection(),
      crosshairCursor(),
      indentOnInput(),
      bracketMatching(),
      closeBrackets(),
      autocompletion(autocompletionConfig),
      highlightActiveLine(),
      highlightSelectionMatches(),
      flashField,
      syntaxHighlighting(sqlfoxHighlightStyle, { fallback: true }),
      sqlfoxTheme,
      dialectCompartment.of(
        sql({
          dialect: resolveDialect(currentOptions.dialect),
          schema: currentOptions.schema,
        })
      ),
      runKeymap,
      updateListener,
    ],
  });

  const view = new EditorView({
    state,
    parent: node,
  });

  return {
    update(newOptions: CodeMirrorOptions) {
      const prevDialect = currentOptions.dialect;
      const prevSchema = currentOptions.schema;
      currentOptions = { ...newOptions };

      if (newOptions.doc !== view.state.doc.toString()) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: newOptions.doc },
        });
      }

      if (newOptions.dialect !== prevDialect || newOptions.schema !== prevSchema) {
        view.dispatch({
          effects: dialectCompartment.reconfigure(
            sql({
              dialect: resolveDialect(newOptions.dialect),
              schema: newOptions.schema,
            })
          ),
        });
      }
    },
    destroy() {
      view.destroy();
    },
  };
}
