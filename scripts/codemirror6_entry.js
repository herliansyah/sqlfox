if (typeof window !== 'undefined') {
  if (!window.requestAnimationFrame) {
    window.requestAnimationFrame = (fn) => setTimeout(fn, 16);
  }
  if (!window.cancelAnimationFrame) {
    window.cancelAnimationFrame = (id) => clearTimeout(id);
  }
  if (typeof Range !== 'undefined') {
    if (!Range.prototype.getClientRects) {
      Range.prototype.getClientRects = () => [];
    }
    if (!Range.prototype.getBoundingClientRect) {
      Range.prototype.getBoundingClientRect = () => ({ top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0 });
    }
  }
  if (typeof Element !== 'undefined' && !Element.prototype.getClientRects) {
    Element.prototype.getClientRects = () => [];
  }
}

import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  drawSelection,
  dropCursor,
  rectangularSelection,
  crosshairCursor,
  placeholder,
  Decoration,
  ViewPlugin,
  ViewUpdate
} from '@codemirror/view';

import {
  EditorState,
  StateEffect,
  StateField,
  Compartment,
  Prec,
  RangeSet,
  RangeSetBuilder
} from '@codemirror/state';

import {
  defaultKeymap,
  history,
  historyKeymap,
  indentWithTab
} from '@codemirror/commands';

import {
  syntaxHighlighting,
  defaultHighlightStyle,
  indentOnInput,
  bracketMatching,
  HighlightStyle
} from '@codemirror/language';

import {
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
  startCompletion,
  closeCompletion,
  acceptCompletion
} from '@codemirror/autocomplete';

import {
  searchKeymap,
  highlightSelectionMatches
} from '@codemirror/search';

import {
  sql,
  MySQL,
  PostgreSQL,
  SQLite,
  StandardSQL
} from '@codemirror/lang-sql';

import { tags } from '@lezer/highlight';

function resolveDialect(modeStr) {
  const m = (modeStr || '').toLowerCase();
  if (m.includes('postgres') || m.includes('pgsql')) return PostgreSQL;
  if (m.includes('sqlite')) return SQLite;
  return MySQL;
}

const addFlashEffect = StateEffect.define();
const clearFlashEffect = StateEffect.define();
const flashMark = Decoration.mark({ class: 'cm-flash-exec' });

const flashField = StateField.define({
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
  provide: f => EditorView.decorations.from(f)
});

const sqlfoxHighlightStyle = HighlightStyle.define([
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

const sqlfoxTheme = EditorView.theme({
  '&': {
    height: '100%',
    width: '100%',
    backgroundColor: 'transparent',
    color: '#e0f2fe',
    fontFamily: "'JetBrains Mono', 'FiraCode Nerd Font', 'Fira Code', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
    fontSize: '14px',
    lineHeight: '1.65',
    letterSpacing: '0.01em'
  },
  '.cm-scroller': {
    height: '100%',
    width: '100%',
    overflow: 'auto',
    fontFamily: 'inherit'
  },
  '.cm-content': {
    minHeight: '100%',
    boxSizing: 'border-box',
    caretColor: '#38bdf8',
    padding: '8px 0'
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: '#38bdf8',
    borderLeftWidth: '2px'
  },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'rgba(56, 189, 248, 0.25) !important'
  },
  '.cm-gutters': {
    backgroundColor: '#090d16',
    color: '#475569',
    borderRight: '1px solid #1e293b'
  },
  '.cm-gutterElement': {
    padding: '0 8px 0 4px'
  },
  '.cm-activeLineGutter': {
    backgroundColor: '#0f172a',
    color: '#94a3b8'
  },
  '.cm-activeLine': {
    backgroundColor: 'rgba(30, 41, 59, 0.35)'
  },
  '&.cm-focused .cm-matchingBracket, .cm-matchingBracket': {
    color: '#38bdf8 !important',
    backgroundColor: 'rgba(56, 189, 248, 0.2) !important',
    outline: '1px solid rgba(56, 189, 248, 0.4) !important',
    borderRadius: '2px'
  },
  '&.cm-focused .cm-nonmatchingBracket, .cm-nonmatchingBracket': {
    color: '#f87171 !important',
    backgroundColor: 'rgba(239, 68, 68, 0.2) !important',
    outline: '1px solid rgba(239, 68, 68, 0.4) !important',
    borderRadius: '2px'
  },
  '.cm-tooltip.cm-tooltip-autocomplete': {
    backgroundColor: '#090d16 !important',
    border: '1px solid #1e293b !important',
    borderRadius: '6px !important',
    boxShadow: '0 10px 25px -5px rgba(0, 0, 0, 0.7), 0 8px 10px -6px rgba(0, 0, 0, 0.7) !important',
    zIndex: '100 !important'
  },
  '.cm-tooltip-autocomplete ul': {
    maxHeight: '240px',
    padding: '4px',
    fontFamily: "'JetBrains Mono', ui-monospace, monospace",
    fontSize: '13px'
  },
  '.cm-tooltip-autocomplete ul li': {
    display: 'flex',
    alignItems: 'center',
    padding: '4px 8px',
    borderRadius: '4px',
    color: '#cbd5e1',
    cursor: 'pointer'
  },
  '.cm-tooltip-autocomplete ul li[aria-selected]': {
    backgroundColor: '#1e293b !important',
    color: '#38bdf8 !important'
  },
  '.cm-flash-exec': {
    backgroundColor: 'rgba(56, 189, 248, 0.35) !important',
    borderRadius: '2px !important',
    transition: 'background-color 0.35s ease !important'
  }
}, { dark: true });

class CodeMirror6Adapter {
  constructor(view, dialectCompartment, lineWrappingCompartment) {
    this.view = view;
    this.dialectCompartment = dialectCompartment;
    this.lineWrappingCompartment = lineWrappingCompartment;
    this._listeners = { change: [], inputRead: [] };
  }

  get state() {
    const s = this.view.state;
    return Object.assign(s, {
      completionActive: false
    });
  }

  getValue() {
    return this.view.state.doc.toString();
  }

  setValue(val) {
    val = val == null ? '' : String(val);
    if (this.getValue() === val) return;
    const len = val.length;
    this.view.dispatch({
      changes: { from: 0, to: this.view.state.doc.length, insert: val },
      selection: { anchor: len, head: len }
    });
    this.view.requestMeasure();
  }

  getSelection() {
    const sel = this.view.state.selection.main;
    return this.view.state.sliceDoc(sel.from, sel.to);
  }

  replaceSelection(val) {
    val = val == null ? '' : String(val);
    const sel = this.view.state.selection.main;
    const endPos = sel.from + val.length;
    this.view.dispatch({
      changes: { from: sel.from, to: sel.to, insert: val },
      selection: { anchor: endPos, head: endPos },
      scrollIntoView: true
    });
  }

  replaceRange(replacement, from, to) {
    const fromIdx = typeof from === 'number' ? from : this.indexFromPos(from);
    const toIdx = to != null ? (typeof to === 'number' ? to : this.indexFromPos(to)) : fromIdx;
    this.view.dispatch({
      changes: { from: fromIdx, to: toIdx, insert: replacement || '' },
      scrollIntoView: true
    });
  }

  getRange(from, to) {
    const fromIdx = typeof from === 'number' ? from : this.indexFromPos(from);
    const toIdx = to != null ? (typeof to === 'number' ? to : this.indexFromPos(to)) : fromIdx;
    return this.view.state.sliceDoc(fromIdx, toIdx);
  }

  focus() {
    this.view.focus();
  }

  hasFocus() {
    return this.view.hasFocus;
  }

  refresh() {
    this.view.requestMeasure();
  }

  getWrapperElement() {
    return this.view.dom;
  }

  lineCount() {
    return this.view.state.doc.lines;
  }

  getCursor(side) {
    const sel = this.view.state.selection.main;
    let pos = sel.head;
    if (side === 'start' || side === 'from') pos = sel.from;
    else if (side === 'end' || side === 'to') pos = sel.to;
    else if (side === 'anchor') pos = sel.anchor;
    const line = this.view.state.doc.lineAt(pos);
    return { line: line.number - 1, ch: pos - line.from };
  }

  setCursor(line, ch = null) {
    const doc = this.view.state.doc;
    if (typeof line === 'number') {
      if (line >= doc.lines) {
        this.view.dispatch({
          selection: { anchor: doc.length, head: doc.length },
          scrollIntoView: true
        });
        return;
      }
      const safeLineNum = Math.max(1, line + 1);
      const lineObj = doc.line(safeLineNum);
      const targetCh = ch == null ? lineObj.length : Math.max(0, ch);
      const pos = Math.min(lineObj.from + targetCh, lineObj.to);
      this.view.dispatch({
        selection: { anchor: pos, head: pos },
        scrollIntoView: true
      });
      return;
    }

    if (line && typeof line === 'object') {
      const l = line.line != null ? line.line : 0;
      const c = line.ch != null ? line.ch : ch;
      if (l >= doc.lines) {
        this.view.dispatch({
          selection: { anchor: doc.length, head: doc.length },
          scrollIntoView: true
        });
        return;
      }
      const safeLineNum = Math.max(1, l + 1);
      const lineObj = doc.line(safeLineNum);
      const targetCh = c == null ? lineObj.length : Math.max(0, c);
      const pos = Math.min(lineObj.from + targetCh, lineObj.to);
      this.view.dispatch({
        selection: { anchor: pos, head: pos },
        scrollIntoView: true
      });
      return;
    }

    this.view.dispatch({
      selection: { anchor: doc.length, head: doc.length },
      scrollIntoView: true
    });
  }

  indexFromPos(pos) {
    if (!pos) return 0;
    const doc = this.view.state.doc;
    const lNum = Math.min(Math.max(1, (pos.line || 0) + 1), doc.lines);
    const lineObj = doc.line(lNum);
    return Math.min(lineObj.from + Math.max(0, pos.ch || 0), lineObj.to);
  }

  posFromIndex(idx) {
    const doc = this.view.state.doc;
    const safeIdx = Math.min(Math.max(0, idx || 0), doc.length);
    const line = doc.lineAt(safeIdx);
    return { line: line.number - 1, ch: safeIdx - line.from };
  }

  markText(from, to, options = {}) {
    const fromIdx = typeof from === 'number' ? from : this.indexFromPos(from);
    const toIdx = typeof to === 'number' ? to : this.indexFromPos(to);
    if (fromIdx >= toIdx) return { clear: () => {} };
    this.view.dispatch({
      effects: addFlashEffect.of({ from: fromIdx, to: toIdx })
    });
    let cleared = false;
    const clear = () => {
      if (cleared) return;
      cleared = true;
      try {
        this.view.dispatch({ effects: clearFlashEffect.of(null) });
      } catch (_) {}
    };
    setTimeout(clear, 350);
    return { clear };
  }

  showHint(hintOptions) {
    startCompletion(this.view);
  }

  setSize(width, height) {
    if (width != null) this.view.dom.style.width = typeof width === 'number' ? width + 'px' : width;
    if (height != null) this.view.dom.style.height = typeof height === 'number' ? height + 'px' : height;
  }

  setDialect(driverName) {
    const dialect = resolveDialect(driverName);
    this.view.dispatch({
      effects: this.dialectCompartment.reconfigure(sql({ dialect }))
    });
  }

  on(event, fn) {
    if (!this._listeners[event]) this._listeners[event] = [];
    this._listeners[event].push(fn);
  }

  off(event, fn) {
    if (this._listeners[event]) {
      this._listeners[event] = this._listeners[event].filter(f => f !== fn);
    }
  }

  somethingSelected() {
    const sel = this.view.state.selection.main;
    return !sel.empty;
  }

  getLine(lineNum) {
    const doc = this.view.state.doc;
    const safeLine = Math.min(Math.max(1, (lineNum || 0) + 1), doc.lines);
    return doc.line(safeLine).text;
  }

  getOption(opt) {
    if (opt === 'value') return this.getValue();
    if (opt === 'mode') return this._mode || '';
    if (opt === 'readOnly') return Boolean(this._readOnly);
    return null;
  }

  setOption(opt, val) {
    if (opt === 'value') this.setValue(val);
    else if (opt === 'mode') this.setDialect(val);
    else if (opt === 'readOnly') this._readOnly = Boolean(val);
  }

  getDoc() {
    return this;
  }

  destroy() {
    this.view.destroy();
  }
}

function CodeMirror(container, options = {}) {
  const initialValue = options.value || '';
  const dialectCompartment = new Compartment();
  const lineWrappingCompartment = new Compartment();

  const dialect = resolveDialect(options.mode);

  const extensions = [
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightSpecialChars(),
    history(),
    drawSelection(),
    dropCursor(),
    EditorState.allowMultipleSelections.of(true),
    indentOnInput(),
    syntaxHighlighting(sqlfoxHighlightStyle, { fallback: true }),
    bracketMatching(),
    closeBrackets(),
    rectangularSelection(),
    crosshairCursor(),
    highlightActiveLine(),
    highlightSelectionMatches(),
    flashField,
    sqlfoxTheme,
    dialectCompartment.of(sql({ dialect })),
    lineWrappingCompartment.of(options.lineWrapping !== false ? EditorView.lineWrapping : [])
  ];

  extensions.push(EditorView.editorAttributes.of({ class: 'CodeMirror' }));

  const autocompletionConfig = {
    defaultKeymap: true,
    icons: false,
    addToOptions: [
      {
        render(completion) {
          const kind = completion.kind || (completion.type ? completion.type.toUpperCase() : '');
          if (!kind) return null;
          const badge = document.createElement('span');
          const cls = completion.badgeClass || ('cm-badge-' + kind.toLowerCase());
          badge.className = 'cm-hint-badge ' + cls;
          badge.textContent = kind;
          return badge;
        },
        position: 10
      }
    ]
  };

  if (typeof options.completionSource === 'function') {
    autocompletionConfig.override = [options.completionSource];
  }

  extensions.push(autocompletion(autocompletionConfig));

  let adapter;

  const customKeyBindings = [];
  if (options.extraKeys) {
    for (const [keyName, handler] of Object.entries(options.extraKeys)) {
      if (typeof handler !== 'function') continue;
      if (keyName === 'Ctrl-Space') {
        customKeyBindings.push({
          key: 'Ctrl-Space',
          run: (view) => {
            startCompletion(view);
            return true;
          }
        });
        continue;
      }

      let cm6Key = keyName.replace(/\bCmd-/g, 'Mod-').replace(/\bCtrl-/g, 'Mod-');
      customKeyBindings.push({
        key: cm6Key,
        run: () => {
          handler(adapter);
          return true;
        }
      });
      if (keyName.startsWith('Ctrl-')) {
        customKeyBindings.push({
          key: keyName,
          run: () => {
            handler(adapter);
            return true;
          }
        });
      }
    }
  }

  extensions.push(
    Prec.highest(keymap.of(customKeyBindings)),
    keymap.of([
      ...closeBracketsKeymap,
      ...defaultKeymap,
      ...searchKeymap,
      ...historyKeymap,
      ...completionKeymap,
      indentWithTab
    ])
  );

  extensions.push(EditorView.domEventHandlers({
    mousedown(event, view) {
      if (event.button !== 0) return false;
      const scroller = view.scrollDOM;
      if (!scroller) return false;
      const rect = scroller.getBoundingClientRect();
      if (
        event.clientX >= rect.left &&
        event.clientX <= rect.right &&
        event.clientY >= rect.top &&
        event.clientY <= rect.bottom
      ) {
        const lastLineEl = view.contentDOM.lastElementChild;
        if (lastLineEl) {
          const lastLineRect = lastLineEl.getBoundingClientRect();
          if (event.clientY > lastLineRect.bottom + 2) {
            event.preventDefault();
            view.focus();
            const endPos = view.state.doc.length;
            view.dispatch({
              selection: { anchor: endPos, head: endPos },
              scrollIntoView: true
            });
            return true;
          }
        }
      }
      return false;
    }
  }));

  extensions.push(EditorView.updateListener.of((update) => {
    if (!adapter) return;
    if (update.docChanged) {
      adapter._listeners.change.forEach(fn => {
        try { fn(adapter, update); } catch (e) { console.error(e); }
      });
    }
    if (update.transactions.some(tr => tr.isUserEvent('input.type') || tr.isUserEvent('input.complete'))) {
      adapter._listeners.inputRead.forEach(fn => {
        try { fn(adapter, update); } catch (e) { console.error(e); }
      });
    }
  }));

  const initialDoc = initialValue;
  const initialLen = initialDoc.length;
  const state = EditorState.create({
    doc: initialDoc,
    selection: { anchor: initialLen, head: initialLen },
    extensions: extensions
  });

  const view = new EditorView({
    state: state,
    parent: container
  });

  adapter = new CodeMirror6Adapter(view, dialectCompartment, lineWrappingCompartment);
  return adapter;
}

CodeMirror.version = '6.0.2';
CodeMirror.EditorView = EditorView;
CodeMirror.EditorState = EditorState;
CodeMirror.Compartment = Compartment;
CodeMirror.StateEffect = StateEffect;
CodeMirror.StateField = StateField;
CodeMirror.sql = sql;
CodeMirror.MySQL = MySQL;
CodeMirror.PostgreSQL = PostgreSQL;
CodeMirror.SQLite = SQLite;
CodeMirror.StandardSQL = StandardSQL;
CodeMirror.autocompletion = autocompletion;
CodeMirror.startCompletion = startCompletion;
CodeMirror.closeCompletion = closeCompletion;
CodeMirror.Pos = function(line, ch) { return { line, ch }; };
CodeMirror.fromTextArea = function(textarea, options = {}) {
  const container = document.createElement('div');
  textarea.parentNode.insertBefore(container, textarea);
  textarea.style.display = 'none';
  options.value = textarea.value;
  const cm = CodeMirror(container, options);
  cm.on('change', () => { textarea.value = cm.getValue(); });
  return cm;
};

if (typeof window !== 'undefined') {
  window.CodeMirror = CodeMirror;
  window.CodeMirror6 = CodeMirror;
}
if (typeof globalThis !== 'undefined') {
  globalThis.CodeMirror = CodeMirror;
  globalThis.CodeMirror6 = CodeMirror;
}

export default CodeMirror;
export {
  CodeMirror,
  CodeMirror6Adapter,
  EditorView,
  EditorState,
  sql,
  MySQL,
  PostgreSQL,
  SQLite
};
