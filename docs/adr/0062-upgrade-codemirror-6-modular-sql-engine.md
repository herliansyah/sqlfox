# 0062: Upgrade to CodeMirror 6 Modular Architecture and Multi-Dialect SQL Engine

## Status
Accepted

## Context
SqlFox previously utilized CodeMirror 5 (v5.65.18) with legacy mode scripts and addons (`mode/sql/sql.min.js`, `addon/hint/show-hint.min.js`). While functional, CodeMirror 5 is legacy software with several constraints:
1. **Monolithic & Non-Modular**: Lacks modern state/view separation, functional transaction pipelines, and incremental Lezer-based AST parsing.
2. **Static SQL Mode**: Code was locked to `text/x-mysql` without native dynamic dialect switching when operators switched between MySQL, PostgreSQL, and SQLite databases.
3. **Legacy Autocomplete Overhead**: Relied on DOM-invasive `show-hint` addons rather than reactive tooltip compartments and modern snippet completions.

## Decisions

### 1. CodeMirror 6 Bundling & Build Pipeline
- Introduce `scripts/codemirror6_entry.js` and `scripts/build_codemirror.js` leveraging `esbuild` to compile a unified, minified, self-contained bundle (`web/dist/vendor/codemirror/codemirror.min.js`, ~422 KB).
- Maintain zero external CDN dependencies, embedding static assets directly into the Go binary via `//go:embed dist/*`.
- Automatically invoke `build:cm` prior to frontend assembly in `scripts/build_web.js`.

### 2. Backward-Compatible CodeMirror 6 Adapter
- Implement `CodeMirror6Adapter` wrapping `@codemirror/view`'s `EditorView` to preserve seamless compatibility with existing frontend APIs:
  - `getValue()`, `setValue(val)`
  - `getSelection()`, `replaceSelection(val)`, `replaceRange(val, from, to)`
  - `getCursor()`, `setCursor(line, ch)`, `indexFromPos(pos)`, `posFromIndex(idx)`
  - `lineCount()`, `focus()`, `hasFocus()`, `refresh()`
  - Execution statement highlight via native `Decoration.mark` and `StateEffect` (`markText`)
  - Event subscriptions via `on('change', ...)`, `on('inputRead', ...)`

### 3. Dynamic Multi-Dialect Support via Compartments
- Utilize `@codemirror/lang-sql` with `Compartment`:
  - Dynamically switches dialect between `MySQL`, `PostgreSQL`, and `SQLite` whenever the operator activates or connects to a database session (`cmEditor.setDialect(conn.driver)`).
  - Defaults gracefully to MySQL/StandardSQL.

### 4. High-Contrast Dark Theme & Autocomplete Integration
- Define `sqlfoxHighlightStyle` using `@lezer/highlight` tags matching the SqlFox dark theme palette:
  - Keywords: Pink-400 (`#f472b6`, bold)
  - Strings: Emerald-400 (`#34d399`)
  - Numbers: Amber-400 (`#fbbf24`)
  - Types: Purple-400 (`#c084fc`)
  - Atoms / Booleans / Null: Orange-400 (`#fb923c`)
  - Functions / Builtins: Sky-400 (`#38bdf8`)
  - Identifiers / Columns / Tables: Sky-300 (`#7dd3fc`)
  - Comments: Slate-500 (`#64748b`, italic)
  - Matching brackets: Glowing cyan outline (`#38bdf8`)
- Provide `cm6SqlCompletionSource` integrating SqlFox's contextual schema catalog, SQL keywords, and user snippets with `@codemirror/autocomplete`.

## Consequences
- **Positive**:
  - Modern, robust, and extensible editor architecture.
  - Full syntax highlighting and parsing parity for PostgreSQL, SQLite, and MySQL.
  - Flawless single-binary offline embedding without runtime CDN requests.
  - All existing 69 embedded frontend unit tests and 20 node regression test suites pass without regression.
- **Negative / Neutral**:
  - CodeMirror bundle size slightly increased from ~205 KB to ~422 KB to accommodate full Lezer parsers for multiple SQL dialects; well within single-binary size constraints.
