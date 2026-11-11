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
  onExecute?: () => void;
  onChange?: (val: string) => void;
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
      sql({ dialect: dialectExtension }),
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
