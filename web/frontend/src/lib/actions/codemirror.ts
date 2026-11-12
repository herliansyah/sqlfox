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
  placeholder,
} from '@codemirror/view';
import { EditorState, EditorSelection, StateEffect, StateField, Compartment, Prec, Annotation } from '@codemirror/state';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { sql, MySQL, PostgreSQL, SQLite } from '@codemirror/lang-sql';
import {
  autocompletion,
  completionKeymap,
  closeBrackets,
  closeBracketsKeymap,
  startCompletion,
  type Completion,
  type CompletionContext,
  type CompletionResult,
} from '@codemirror/autocomplete';
import { search, searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import { bracketMatching, syntaxHighlighting, HighlightStyle, indentOnInput } from '@codemirror/language';
import { tags } from '@lezer/highlight';
import type { QuerySnippet, SchemaCatalog } from '../../types';
import { executionTarget, maskSqlForAnalysis, type StatementPos } from '../utils/sqlStatements';
import { computeSqlCompletions, completionTokenStart, parseQualifiedReference, type CompletionItem } from '../utils/sqlCompletion';

export interface CodeMirrorOptions {
  id?: string;
  doc: string;
  dialect?: 'mysql' | 'postgres' | 'sqlite';
  readOnly?: boolean;
  placeholder?: string;
  activeDatabase?: string;
  catalogs?: Record<string, SchemaCatalog>;
  snippets?: QuerySnippet[];
  loadCatalog?: (database: string) => Promise<unknown>;
  quoteIdentifier?: (identifier: string) => string;
  onExecute?: (sql: string) => void;
  onExecuteAll?: (sql: string) => void;
  onExplain?: (sql: string) => void;
  onFormat?: (target: StatementPos) => void;
  onSnippet?: (snippet: QuerySnippet, insert: (sql: string) => void) => void;
  onChange?: (value: string) => void;
  onReady?: (controller: CodeMirrorController | null) => void;
}

export interface CodeMirrorController {
  getTarget: () => StatementPos;
  getDocument: () => string;
  replaceTarget: (value: string) => boolean;
  insertAtSelection: (value: string) => boolean;
  focus: () => void;
  requestMeasure: () => void;
}

const externalDoc = Annotation.define<boolean>();
const addFlashEffect = StateEffect.define<{ from: number; to: number }>();
const clearFlashEffect = StateEffect.define<null>();
const flashMark = Decoration.mark({ class: 'cm-flash-exec' });

const flashField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(flashes, tr) {
    flashes = flashes.map(tr.changes);
    for (const effect of tr.effects) {
      if (effect.is(addFlashEffect)) flashes = Decoration.set([flashMark.range(effect.value.from, effect.value.to)]);
      else if (effect.is(clearFlashEffect)) flashes = Decoration.none;
    }
    return flashes;
  },
  provide: (field) => EditorView.decorations.from(field),
});

export const sqlfoxHighlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: '#f472b6', fontWeight: '600' },
  { tag: tags.string, color: '#34d399' },
  { tag: tags.number, color: '#fbbf24', fontVariantNumeric: 'tabular-nums' },
  { tag: tags.typeName, color: '#c084fc', fontWeight: '500' },
  { tag: [tags.bool, tags.null, tags.atom], color: '#fb923c' },
  { tag: [tags.function(tags.variableName), tags.standard(tags.variableName)], color: '#38bdf8' },
  { tag: [tags.operator, tags.punctuation], color: '#94a3b8' },
  { tag: tags.bracket, color: '#cbd5e1' },
  { tag: [tags.lineComment, tags.blockComment, tags.comment], color: '#64748b', fontStyle: 'italic' },
  { tag: tags.variableName, color: '#7dd3fc' },
]);

const monoFont = "'JetBrains Mono', 'FiraCode Nerd Font', 'Fira Code', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace";
export const sqlfoxTheme = EditorView.theme({
  '&': {
    height: '100%', width: '100%', backgroundColor: '#090d16', color: '#e0f2fe',
    fontFamily: monoFont, fontSize: '14px', lineHeight: '1.65', letterSpacing: '0.01em',
  },
  '.cm-scroller': { height: '100%', width: '100%', overflow: 'auto', fontFamily: 'inherit' },
  '.cm-content': { minHeight: '100%', boxSizing: 'border-box', caretColor: '#38bdf8', padding: '8px 0' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#38bdf8', borderLeftWidth: '2px' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'rgba(56, 189, 248, 0.25) !important',
  },
  '.cm-gutters': { backgroundColor: '#090d16', color: '#475569', borderRight: '1px solid #1e293b' },
  '.cm-gutterElement': { padding: '0 8px 0 4px' },
  '.cm-activeLineGutter': { backgroundColor: '#0f172a', color: '#94a3b8' },
  '.cm-activeLine': { backgroundColor: 'rgba(30, 41, 59, 0.35)' },
  '&.cm-focused .cm-matchingBracket, .cm-matchingBracket': {
    color: '#38bdf8 !important', backgroundColor: 'rgba(56, 189, 248, 0.2) !important',
    outline: '1px solid rgba(56, 189, 248, 0.4) !important', borderRadius: '2px',
  },
  '&.cm-focused .cm-nonmatchingBracket, .cm-nonmatchingBracket': {
    color: '#f87171 !important', backgroundColor: 'rgba(239, 68, 68, 0.2) !important',
    outline: '1px solid rgba(239, 68, 68, 0.4) !important', borderRadius: '2px',
  },
  '.cm-tooltip.cm-tooltip-autocomplete': {
    backgroundColor: '#090d16 !important', border: '1px solid #1e293b !important', borderRadius: '6px !important',
    boxShadow: '0 10px 25px -5px rgba(0, 0, 0, 0.7), 0 8px 10px -6px rgba(0, 0, 0, 0.7) !important',
    overflow: 'hidden', zIndex: '100 !important',
  },
  '.cm-tooltip-autocomplete ul': { maxHeight: '240px', padding: '4px', fontFamily: monoFont, fontSize: '13px' },
  '.cm-tooltip-autocomplete ul li': {
    display: 'flex', alignItems: 'center', padding: '4px 8px', borderRadius: '4px', color: '#cbd5e1', cursor: 'pointer',
  },
  '.cm-tooltip-autocomplete ul li[aria-selected]': { backgroundColor: '#1e293b !important', color: '#38bdf8 !important' },
  '.cm-completionLabel': { color: '#e0f2fe', fontWeight: '500' },
  '.cm-completionDetail': { marginLeft: 'auto', paddingLeft: '16px', color: '#64748b', fontSize: '11px', fontStyle: 'italic' },
  '.cm-hint-badge': {
    display: 'inline-flex', alignItems: 'center', justifyContent: 'center', flexShrink: '0',
    fontSize: '9px', lineHeight: '1.2', fontWeight: '700', letterSpacing: '0.05em',
    padding: '1px 4px', borderRadius: '3px', marginRight: '8px', textTransform: 'uppercase',
  },
  '.cm-badge-kw': { backgroundColor: '#334155', color: '#94a3b8' },
  '.cm-badge-tbl': { backgroundColor: 'rgba(16,185,129,.2)', color: '#34d399' },
  '.cm-badge-col': { backgroundColor: 'rgba(56,189,248,.2)', color: '#38bdf8' },
  '.cm-badge-fn': { backgroundColor: 'rgba(245,158,11,.2)', color: '#fbbf24' },
  '.cm-badge-db': { backgroundColor: 'rgba(168,85,247,.2)', color: '#c084fc' },
  '.cm-badge-snp': { backgroundColor: 'rgba(236,72,153,.2)', color: '#f472b6' },
  '.cm-flash-exec': {
    backgroundColor: 'rgba(56,189,248,.35) !important', borderRadius: '2px !important',
    transition: 'background-color .35s ease !important',
  },
  '.cm-panels': { backgroundColor: '#0f172a', color: '#cbd5e1', borderColor: '#334155' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid #334155' },
  '.cm-searchMatch': { backgroundColor: 'rgba(251,191,36,.25)', outline: '1px solid rgba(251,191,36,.45)' },
  '.cm-searchMatch-selected': { backgroundColor: 'rgba(56,189,248,.35)' },
}, { dark: true });

function resolveDialect(dialect?: string) {
  if (dialect === 'postgres') return PostgreSQL;
  if (dialect === 'sqlite') return SQLite;
  return MySQL;
}

function badge(completion: Completion): HTMLElement | null {
  const kind = (completion as Completion & { kind?: string }).kind;
  if (!kind) return null;
  const element = document.createElement('span');
  element.className = `cm-hint-badge cm-badge-${kind.toLowerCase()}`;
  element.textContent = kind;
  return element;
}

export function codemirror(node: HTMLElement, options: CodeMirrorOptions) {
  let currentOptions = options;
  let currentId = options.id || '__default';
  let destroyed = false;
  let flashTimer: ReturnType<typeof setTimeout> | undefined;
  // ponytail: cache the last known document so the reactive update path never
  // re-materialises the whole document string on every keystroke.
  let lastKnownDoc = options.doc || '';
  const dialectCompartment = new Compartment();
  const readOnlyCompartment = new Compartment();
  const states = new Map<string, EditorState>();
  const scrollPositions = new Map<string, number>();

  const flash = (from: number, to: number) => {
    if (from >= to) return;
    if (flashTimer) clearTimeout(flashTimer);
    view.dispatch({ effects: addFlashEffect.of({ from, to }) });
    flashTimer = setTimeout(() => {
      if (!destroyed) view.dispatch({ effects: clearFlashEffect.of(null) });
    }, 350);
  };

  const target = () => {
    const selection = view.state.selection.main;
    return executionTarget(view.state.doc.toString(), selection.from, selection.to, currentOptions.dialect);
  };

  const replaceRange = (from: number, to: number, value: string) => {
    const end = from + value.length;
    view.dispatch({ changes: { from, to, insert: value }, selection: { anchor: end }, scrollIntoView: true });
  };

  const controller: CodeMirrorController = {
    getTarget: target,
    getDocument: () => view.state.doc.toString(),
    replaceTarget(value) {
      const range = target();
      if (!range.text.trim()) return false;
      replaceRange(range.start, range.end, value);
      return true;
    },
    insertAtSelection(value) {
      if (destroyed || currentOptions.readOnly) return false;
      const selection = view.state.selection.main;
      replaceRange(selection.from, selection.to, value);
      view.focus();
      return true;
    },
    focus: () => view.focus(),
    requestMeasure: () => view.requestMeasure(),
  };

  const runKeymap = Prec.highest(keymap.of([
    {
      key: 'Mod-Enter',
      run: () => {
        if (!currentOptions.onExecute) return false;
        const range = target();
        flash(range.start, range.end);
        currentOptions.onExecute(range.text);
        return true;
      },
    },
    {
      key: 'Mod-Alt-Enter',
      run: () => {
        if (!currentOptions.onExecuteAll) return false;
        const text = view.state.doc.toString();
        flash(0, text.length);
        currentOptions.onExecuteAll(text);
        return true;
      },
    },
    {
      key: 'F5',
      run: () => {
        if (!currentOptions.onExecuteAll) return false;
        const text = view.state.doc.toString();
        flash(0, text.length);
        currentOptions.onExecuteAll(text);
        return true;
      },
    },
    {
      key: 'Mod-Shift-Enter',
      run: () => {
        if (!currentOptions.onExplain) return false;
        const range = target();
        flash(range.start, range.end);
        currentOptions.onExplain(range.text);
        return true;
      },
    },
    {
      key: 'Mod-Shift-f',
      run: () => {
        if (!currentOptions.onFormat) return false;
        currentOptions.onFormat(target());
        return true;
      },
    },
    { key: 'Ctrl-Space', run: (editor) => { startCompletion(editor); return true; } },
  ]));

  function toCompletion(item: CompletionItem): Completion {
    const option: Completion & { kind?: string } = {
      label: item.label,
      type: 'text',
      kind: item.kind,
      detail: item.detail || '',
      boost: item.boost,
      apply: item.apply || item.label,
    };
    if (item.kind === 'FN' && item.label.endsWith('()')) {
      option.apply = (editor, _completion, from, to) => {
        const end = from + item.label.length;
        editor.dispatch({ changes: { from, to, insert: item.label }, selection: { anchor: end - 1 }, scrollIntoView: true });
      };
    } else if (item.kind === 'KW' && item.label.includes(' ')) {
      // The completion range already spans the typed phrase, so replace it wholesale.
      option.apply = (editor, _completion, from, to) => {
        const end = from + item.label.length;
        editor.dispatch({ changes: { from, to, insert: item.label }, selection: { anchor: end }, scrollIntoView: true });
      };
    } else if (item.snippet) {
      option.apply = (editor, _completion, from, to) => {
        const snippet = item.snippet!;
        const insert = (sql: string) => {
          const end = from + sql.length;
          editor.dispatch({ changes: { from, to, insert: sql }, selection: { anchor: end }, scrollIntoView: true });
        };
        if (currentOptions.onSnippet) currentOptions.onSnippet(snippet, insert);
        else insert(snippet.sql);
      };
    }
    return option;
  }

  async function completionSource(context: CompletionContext): Promise<CompletionResult | null> {
    const text = context.state.doc.toString();
    const line = text.slice(text.lastIndexOf('\n', context.pos - 1) + 1, context.pos);
    const snip = line.match(/(?:^|\s)(snip:[a-zA-Z0-9_]*)$/i);
    if (snip) {
      const masked = maskSqlForAnalysis(text, currentOptions.dialect);
      if (masked.slice(context.pos - snip[1].length, context.pos) !== snip[1]) return null;
      const query = snip[1].slice(5).toLowerCase();
      const options = (currentOptions.snippets || [])
        .filter((snippet) => !query || snippet.title.toLowerCase().includes(query) || snippet.tags.some((tag) => tag.toLowerCase().includes(query)))
        .map((snippet) => toCompletion({ label: snippet.title, kind: 'SNP', detail: snippet.tags.join(', ') || 'Template', snippet, boost: 20 }));
      return options.length ? { from: context.pos - snip[1].length, to: context.pos, options, filter: false } : null;
    }

    let requestedDatabase = '';
    const compute = () => computeSqlCompletions({
      text,
      cursor: context.pos,
      catalogs: currentOptions.catalogs || {},
      activeDatabase: currentOptions.activeDatabase,
      snippets: currentOptions.snippets,
      dialect: currentOptions.dialect,
      quoteIdentifier: currentOptions.quoteIdentifier,
      onDatabaseRequested: (database) => { requestedDatabase = database; },
    });
    let items = compute();
    if (requestedDatabase && currentOptions.loadCatalog) {
      try { await currentOptions.loadCatalog(requestedDatabase); } catch { /* completion stays empty on request failure */ }
      if (destroyed || view.state.doc.toString() !== text || view.state.selection.main.head !== context.pos) return null;
      requestedDatabase = '';
      items = compute();
    }
    if (!items.length) return null;
    const from = completionTokenStart(text, context.pos);
    const prefix = text.slice(from, context.pos).toLowerCase();
    items.sort((a, b) => {
      const rank = (item: CompletionItem) => (item.boost || 0) * 10
        + (item.label.toLowerCase() === prefix ? 200 : item.label.toLowerCase().startsWith(prefix) ? 80 : 0);
      return rank(b) - rank(a) || a.label.localeCompare(b.label);
    });
    const qualifiedTrigger = parseQualifiedReference(text, context.pos)?.trailingDot;
    if (!context.explicit && from === context.pos && !qualifiedTrigger) return null;
    return { from, to: context.pos, options: items.map(toCompletion), filter: false };
  }

  const updateListener = EditorView.updateListener.of((update) => {
    if (!update.docChanged) return;
    if (update.transactions.some((tr) => tr.annotation(externalDoc))) return;
    // ponytail: only materialise the document string when the caller actually needs it.
    const value = update.state.doc.toString();
    lastKnownDoc = value;
    currentOptions.onChange?.(value);
  });

  const clickBelowLastLine = EditorView.domEventHandlers({
    mousedown(event, editor) {
      if (event.button !== 0) return false;
      const lastLine = editor.contentDOM.lastElementChild;
      if (!lastLine || event.clientY <= lastLine.getBoundingClientRect().bottom + 2) return false;
      event.preventDefault();
      editor.focus();
      editor.dispatch({ selection: { anchor: editor.state.doc.length }, scrollIntoView: true });
      return true;
    },
  });

  function makeState(doc: string): EditorState {
    return EditorState.create({
      doc,
      selection: { anchor: doc.length },
      extensions: [
        lineNumbers(), highlightActiveLineGutter(), highlightSpecialChars(), history(), drawSelection(), dropCursor(),
        EditorState.allowMultipleSelections.of(true), rectangularSelection(), crosshairCursor(), indentOnInput(),
        bracketMatching(), closeBrackets(), highlightActiveLine(), highlightSelectionMatches(), search({ top: true }),
        EditorView.lineWrapping, flashField, syntaxHighlighting(sqlfoxHighlightStyle, { fallback: true }), sqlfoxTheme,
        dialectCompartment.of(sql({ dialect: resolveDialect(currentOptions.dialect) })),
        readOnlyCompartment.of([
          EditorState.readOnly.of(!!currentOptions.readOnly),
          EditorView.editable.of(!currentOptions.readOnly),
        ]),
        currentOptions.placeholder ? placeholder(currentOptions.placeholder) : [],
        autocompletion({
          override: [completionSource], activateOnTyping: true, closeOnBlur: true, maxRenderedOptions: 50,
          defaultKeymap: true, icons: false, interactionDelay: 0,
          addToOptions: [{ render: badge, position: 10 }],
        }),
        runKeymap,
        keymap.of([...closeBracketsKeymap, ...defaultKeymap, ...searchKeymap, ...historyKeymap, ...completionKeymap, indentWithTab]),
        clickBelowLastLine, updateListener,
      ],
    });
  }

  const view = new EditorView({ state: makeState(options.doc || ''), parent: node });
  currentOptions.onReady?.(controller);

  return {
    update(next: CodeMirrorOptions) {
      if (destroyed) return;
      const previousId = currentId;
      const previousDialect = currentOptions.dialect;
      const previousReadOnly = currentOptions.readOnly;
      currentOptions = next;
      currentId = next.id || '__default';
      const switched = currentId !== previousId;

      if (switched) {
        states.set(previousId, view.state);
        scrollPositions.set(previousId, view.scrollDOM.scrollTop);
        view.setState(states.get(currentId) || makeState(next.doc || ''));
        view.scrollDOM.scrollTop = scrollPositions.get(currentId) || 0;
        lastKnownDoc = view.state.doc.toString();
      }

      // Reconcile the document for both the active tab and restored tabs.
      if (next.doc !== lastKnownDoc) {
        const doc = next.doc || '';
        const selection = view.state.selection;
        const ranges = selection.ranges.map((range) => EditorSelection.range(
          Math.min(range.anchor, doc.length),
          Math.min(range.head, doc.length),
        ));
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: doc },
          selection: EditorSelection.create(ranges, selection.mainIndex),
          scrollIntoView: true,
          annotations: externalDoc.of(true),
        });
        lastKnownDoc = doc;
      }

      if (switched || next.dialect !== previousDialect) {
        view.dispatch({ effects: dialectCompartment.reconfigure(sql({ dialect: resolveDialect(next.dialect) })) });
      }
      if (switched || !!next.readOnly !== !!previousReadOnly) {
        view.dispatch({ effects: readOnlyCompartment.reconfigure([
          EditorState.readOnly.of(!!next.readOnly), EditorView.editable.of(!next.readOnly),
        ]) });
      }
      if (switched) view.requestMeasure();
    },
    destroy() {
      destroyed = true;
      if (flashTimer) clearTimeout(flashTimer);
      currentOptions.onReady?.(null);
      states.clear();
      scrollPositions.clear();
      view.destroy();
    },
  };
}
