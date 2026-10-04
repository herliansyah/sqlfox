# 0019: Inline Table Filter Bar, Server-Side Pushdown, and Grid Query Handoff

To deliver flexible, high-performance data filtering comparable to modern database GUIs (TablePlus, DataGrip, DBeaver) without requiring users to compose manual queries from scratch, SqlFox introduces an Inline Table Filter Bar with dual-mode clause building (Quick Clause Pills & Raw WHERE expressions), validated Server-Side Pushdown, and 1-click Grid Query Handoff.

## Context & Problem

When users open a table in the Table Viewer tab (`Data` mode):
1. **Lack of In-Place Filtering**: The data grid displays the initial 50 rows (`SELECT * FROM table LIMIT 50`), but provides no interactive mechanism to search or filter records without opening a separate Query Editor tab and retyping the full SQL statement.
2. **In-Memory Filtering Limitations**: Client-side filtering in browser memory only filters the rows already fetched (50 rows), making it incapable of finding matching records located deeper in large tables (thousands to millions of rows).
3. **Friction for Non-SQL / Ad-Hoc Lookups**: Developers frequently need quick lookups (e.g. `status = 'pending'`, `email LIKE '%@gmail.com%'`) where typing a full SQL query is tedious, yet occasionally need complex Boolean expressions (`status = 'active' AND (role = 'admin' OR total > 500)`) where rigid visual form builders fall short.
4. **Disconnection Between Table Viewer and Editor**: Once a filter is constructed visually, there was no bridge to graduate that query into the full Query Editor for adding `JOIN`, `GROUP BY`, or subqueries.

## Considered Options

- **Client-Side JavaScript Filter Only**: Filtering only the 50 pre-fetched rows in browser memory. Rejected because it gives false confidence and fails completely on tables with more than 50 rows.
- **Strict Visual Clause Builder Only**: Allowing only structured dropdown pills (`col`, `op`, `val`). Rejected because it lacks the flexibility for nested parentheses, SQL functions (`DATE(created_at) = '2026-01-01'`), and arbitrary boolean logic.
- **Pure Raw WHERE Input Only**: A single text input bar where users must type SQL WHERE syntax. Functional for SQL experts, but slower for quick one-click column filters.
- **Hybrid TablePlus-Style Inline Filter Bar with Server-Side Pushdown (Chosen)**:
  - **Dual Mode**: Seamlessly switch between structured *Quick Clause Pills* (`[Column] [Operator] [Value]`) and freeform *Raw WHERE Expression* bar.
  - **Server-Side Pushdown**: The filter expression is validated at the backend trust boundary and appended directly to the database execution pipeline as an indexed `WHERE (<filter>)` clause with active sorting and pagination.
  - **Backend Safety Guard**: Validates that filter expressions do not contain statement delimiters (`;`), statement verbs (`DROP`, `DELETE`, `UPDATE`, `INSERT`, `ALTER`, `TRUNCATE`, `GRANT`), or output redirection (`INTO OUTFILE`).
  - **Grid Query Handoff**: A 1-click action ("📝 Open in SQL Tab") that converts the current table, active filter, and sort order into a complete editable SQL script in a new Query Editor tab.

## Decision

1. **Backend API Extension (`handleGetTableRows`)**:
   - Extend `GET /api/session/{id}/table/{table}/rows` to accept optional `filter` query parameter.
   - Enforce server-side AST and syntax validation on `filter`:
     - Disallow semicolons (`;`) to prevent statement chaining.
     - Disallow dangerous top-level statements/keywords (`DROP`, `DELETE`, `UPDATE`, `INSERT`, `ALTER`, `TRUNCATE`, `GRANT`, `REVOKE`, `UNION`, `INTO OUTFILE`, `INTO DUMPFILE`).
     - Wrap approved filter expression into `WHERE (<filter>)` in the generated select query.
     - Ensure compatibility with sorting (`ORDER BY`) and Keyset Pagination markers (`WHERE (<filter>) AND (<sort_col> > <last_key>)`).
2. **Frontend UI Seams (`web/dist/index.html`)**:
   - Insert the **Inline Table Filter Bar** as a sticky compact bar (`~34px`) docked above the Data Grid table and below the mode tabs.
   - Support Quick Clause Pills:
     - Column selector populated from cached table columns.
     - Operator selector: `=`, `!=`, `LIKE`, `NOT LIKE`, `>`, `<`, `>=`, `<=`, `IN`, `IS NULL`, `IS NOT NULL`.
     - Value input with Enter key submission.
     - Add condition button (`+ AND`) to chain multiple pills.
   - Support Raw WHERE mode: Single text input with pre-filled `WHERE ` prefix, allowing arbitrary SQL expressions.
   - Quick action buttons: `Run Filter (Enter)`, `Clear (✕)`, and `📝 Open in SQL Tab`.
   - Keyboard shortcut: `Ctrl + F` / `Cmd + F` while on Table Viewer focuses the filter input.

## Consequences

- Full TablePlus and DataGrip filtering parity directly on the Data Grid.
- 100% server-side pushdown ensures lightning-fast queries utilizing database indexes across arbitrary table sizes.
- Zero extra dependencies; implemented using existing pure Go drivers and vanilla JS.
- Complete safety at the backend trust boundary preventing SQL injection or destructive mutations.
