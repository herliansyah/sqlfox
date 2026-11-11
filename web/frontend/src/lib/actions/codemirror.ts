import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLineGutter,
  highlightSpecialChars,
  drawSelection,
  highlightActiveLine,
} from '@codemirror/view';
import { EditorState } from '@codemirror/state';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { sql, MySQL, PostgreSQL, SQLite } from '@codemirror/lang-sql';
import { autocompletion, completionKeymap, closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
import { oneDark } from '@codemirror/theme-one-dark';
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import { bracketMatching, syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language';

export interface CodeMirrorOptions {
  doc: string;
  dialect?: 'mysql' | 'postgres' | 'sqlite';
  schema?: Record<string, string[]>;
  onExecute?: () => void;
  onExecuteAtCursor?: (statement: string) => void;
  onFormat?: () => void;
  onChange?: (val: string) => void;
}

function getStatementAtPos(text: string, pos: number): string {
  if (!text.trim()) return '';
  let inSingle = false;
  let inDouble = false;
  let inBacktick = false;
  let inLineComment = false;
  let inBlockComment = false;
  let stmtStart = 0;
  const stmts: { start: number; end: number; text: string }[] = [];

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
  return found ? found.text : text;
}

export function codemirror(node: HTMLElement, options: CodeMirrorOptions) {
  let dialectExtension = MySQL;
  if (options.dialect === 'postgres') dialectExtension = PostgreSQL;
  if (options.dialect === 'sqlite') dialectExtension = SQLite;

  const runKeymap = keymap.of([
    {
      key: 'Ctrl-Enter',
      mac: 'Cmd-Enter',
      run: () => {
        if (options.onExecute) {
          options.onExecute();
          return true;
        }
        return false;
      },
    },
    {
      key: 'Ctrl-Shift-Enter',
      mac: 'Cmd-Shift-Enter',
      run: (view) => {
        if (options.onExecuteAtCursor) {
          const pos = view.state.selection.main.head;
          const text = view.state.doc.toString();
          const stmt = getStatementAtPos(text, pos);
          options.onExecuteAtCursor(stmt);
          return true;
        }
        return false;
      },
    },
    {
      key: 'Ctrl-Shift-f',
      mac: 'Cmd-Shift-f',
      run: () => {
        if (options.onFormat) {
          options.onFormat();
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
    if (update.docChanged && options.onChange) {
      options.onChange(update.state.doc.toString());
    }
  });

  const state = EditorState.create({
    doc: options.doc || '',
    extensions: [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      drawSelection(),
      bracketMatching(),
      closeBrackets(),
      autocompletion(),
      highlightActiveLine(),
      highlightSelectionMatches(),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      sql({ dialect: dialectExtension, schema: options.schema }),
      oneDark,
      runKeymap,
      updateListener,
      EditorView.theme({
        '&': {
          height: '100%',
          backgroundColor: '#090d16',
        },
        '.cm-scroller': {
          fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
          fontSize: '13.5px',
        },
        '.cm-gutters': {
          backgroundColor: '#090d16',
          borderRight: '1px solid #1e293b',
          color: '#475569',
        },
      }),
    ],
  });

  const view = new EditorView({
    state,
    parent: node,
  });

  return {
    update(newOptions: CodeMirrorOptions) {
      if (newOptions.doc !== view.state.doc.toString()) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: newOptions.doc },
        });
      }
    },
    destroy() {
      view.destroy();
    },
  };
}
