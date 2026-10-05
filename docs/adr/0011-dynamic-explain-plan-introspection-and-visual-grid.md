# 0011: Dynamic EXPLAIN Plan Introspection and Visual Execution Plan Grid

To ensure query execution plans reliably parse across all database engines and versions (MySQL 5.5–8.4, MariaDB 10.x/11.x, SQLite) and present actionable performance insights rather than raw empty JSON, SqlFox implements dynamic column-mapped EXPLAIN scanning and a visual execution plan grid.

## Problem

When clicking **Explain Query** (`Ctrl+Shift+Enter` or toolbar button), users encountered `{ "raw_json": "", "rows": null }`:
1. **Hardcoded Column Scanning**: `internal/driver/mysql/mysql.go` previously scanned a fixed list of 12 destination variables. In MariaDB and MySQL without partition/filter extensions, standard `EXPLAIN` returns 10 columns. When `rows.Scan` threw `sql: expected 10 destination arguments in Scan, not 12`, the error was silently ignored by `if err == nil`, causing `planRows` to remain `nil` and serialize as `"rows": null`.
2. **Missing SQL Sanitization**: Queries with trailing semicolons (`;`) or leading `EXPLAIN` keywords triggered syntax errors when concatenated blindly with `"EXPLAIN "`.
3. **Raw JSON Modal UI**: The impact modal previously lacked a tabular visualization and simply displayed `JSON.stringify(planData, null, 2)` inside a code block.

## Solution

1. **Dynamic Column Mapping (`driver.ScanExplainRows`)**:
   - Inspects `rows.Columns()` dynamically on any result set.
   - Maps columns case-insensitively by name (`id`, `select_type`, `table`, `type`, `key`, `rows`, `filtered`, `extra`, etc.) regardless of column count or order.
   - Reliably parses MariaDB 10-column formats, MySQL 8.0 12-column formats, and SQLite `EXPLAIN QUERY PLAN` 4-column formats without crashing or dropping rows.
2. **Sanitization & Partial Selection**:
   - Strips trailing semicolons and redundant `EXPLAIN` prefixes before dispatch.
   - `getEditorSQL()` prioritizes highlighted/selected text in CodeMirror (`cmEditor.getSelection()`), allowing users to explain single statements inside multi-statement editor scripts.
3. **Visual Execution Plan Grid (`#impact-preview-modal`)**:
   - Replaced raw text dump with an interactive data grid showing `#`, `Type`, `Table`, `Access`, `Key Used`, `Rows`, `Filtered`, and `Extra`.
   - Access types are highlighted with visual alert badges (e.g. red `ALL (Full Scan)` warning, amber `index`, green `ref`/`range`).
   - Tab switcher allows toggling between **Plan Grid** and **Raw JSON** inspection.

## Consequences

- Explain works reliably across all supported MySQL, MariaDB, and SQLite versions.
- Users receive immediate, visual guidance on query bottlenecks and missing indexes.
- Output JSON always serializes clean empty arrays (`[]`) rather than `null`.
