# SqlFox UX Design Specification

User experience architecture, ASCII wireframes, design tokens, and interaction models for SqlFox.

## 1. Design Philosophy

- **Speed & Minimalist Flow**: Sub-second UI response, instant keyboard navigation, clean TablePlus-like interface.
- **Safety by Contrast**: Safe operations are whisper-quiet; destructive operations demand unmistakable focus with bold visual contrast.
- **Density Without Clutter**: High data density in the grid, zero redundant chrome or nested toolbars.

---

## 2. Main Interface Wireframe (3-Column Layout)

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ [SqlFox]  Prod-MySQL-Cluster [PRODUCTION 🔴]        [Ctrl+K Command Palette]      [⚙️ Settings] [−][□][×]│
├──────────────┬─────────────────────────────────────────────────────────┬───────────────────────────────┤
│ CONNECTIONS  │ [Tab 1: users.sql ●] [Tab 2: orders] [+]                │ AI ASSISTANT (BYOK / Ollama)  │
│ ▼ Production │─────────────────────────────────────────────────────────│───────────────────────────────┤
│   ● analytics│ 1 | SELECT id, email, created_at, status                │ 🤖 Claude 3.5 Sonnet          │
│   🔴 main_db │ 2 | FROM users                                          │                               │
│     ▼ Tables │ 3 | WHERE created_at >= '2026-01-01'                    │ User:                         │
│       users  │ 4 | ORDER BY id DESC;                                   │ "Optimize this query"         │
│       orders │                                                         │                               │
│       items  │ [▶ Run (Ctrl+Enter)] [⚡ Explain] [🛡️ Impact Preview]    │ AI Copilot:                   │
│ ▶ Staging    ├─────────────────────────────────────────────────────────│ "Query does a range scan on   │
│ ▶ Dev        │ Result: 1,000 rows (24ms)                [Filter...]    │ `created_at`. Consider adding:│
│              │┌──────┬──────────────────────┬────────────┬────────────┐│                               │
│              ││ id   │ email                │ created_at │ status     ││ CREATE INDEX idx_users_status │
│              │├──────┼──────────────────────┼────────────┼────────────┤│   ON users(created_at, status)│
│              ││ 1042 │ alice@example.com    │ 2026-09-10 │ active     ││                               │
│              ││ 1041 │ bob@example.com      │ 2026-09-09 │ pending    ││ [Apply to Editor] [Copy SQL] │
│              ││ 1040 │ charlie@example.com  │ 2026-09-09 │ active     ││                               │
│              │└──────┴──────────────────────┴────────────┴────────────┘│ [Ask anything about schema...]│
├──────────────┴─────────────────────────────────────────────────────────┴───────────────────────────────┤
│ 🔴 PRODUCTION | MySQL 8.0.35 | Ping: 12ms | Memory: 42MB | SSL Enabled | UTF-8                     │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Safety Guard Modal Wireframes

### Challenge Modal: Destructive DDL / DML (`DROP TABLE` / `DELETE without WHERE`)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  ⚠️ SAFETY GUARD: DESTRUCTIVE QUERY INTERCEPTED                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Target Connection:  main_db [PRODUCTION]                               │
│  Target Table:       `customers` (Approx. 4,250,000 rows)               │
│                                                                         │
│  Statement:                                                             │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ DROP TABLE `customers`;                                           │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  🔴 RISK: This operation is IRREVERSIBLE and drops all customer data.    │
│  📁 DDL schema snapshot saved to: .scratch/backups/20260910-cust.sql   │
│                                                                         │
│  To execute this statement, type the exact table name below:            │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ customers                                                         │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│                           [Cancel (Esc)]  [Confirm & Execute (Danger)]  │
└─────────────────────────────────────────────────────────────────────────┘
```

### Impact Preview Drawer (`EXPLAIN` Analysis)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  🛡️ QUERY IMPACT PREVIEW                                         [×]   │
├─────────────────────────────────────────────────────────────────────────┤
│  Query: UPDATE orders SET status = 'archived' WHERE total < 50;         │
│                                                                         │
│  Scan Type:      ALL (Full Table Scan) ⚠️                                │
│  Estimated Rows: 840,210 of 1,200,000 (70% of table)                    │
│  Key Used:       NULL (No index match on `total`)                       │
│  Estimated Lock: Exclusive Row Locks                                    │
│                                                                         │
│  💡 Suggestion: Index missing on `total`. Add index or narrow WHERE.   │
│                                                                         │
│  [Simulate Transaction (2s timeout)]              [Execute Anyway]      │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Keyboard Shortcuts

| Shortcut (macOS / Linux-Win) | Action | Scope |
| :--- | :--- | :--- |
| `Cmd+Enter` / `Ctrl+Enter` | Run Selected or Current Query | Query Editor |
| `Cmd+Shift+Enter` / `Ctrl+Shift+Enter` | Run Query with Impact Preview (`EXPLAIN`) | Query Editor |
| `Esc` or `Cmd+.` / `Ctrl+.` | Cancel Currently Running Query (Out-of-band) | Global |
| `Cmd+T` / `Ctrl+T` | Open New Query Tab | Tabs |
| `Cmd+W` / `Ctrl+W` | Close Active Tab | Tabs |
| `Cmd+K` / `Ctrl+K` | Open Command Palette | Global |
| `Cmd+P` / `Ctrl+P` | Open Schema Search (Quick Open Table/Column) | Global |
| `Cmd+L` / `Ctrl+L` | Toggle AI Assistant Sidebar | Global |
| `Cmd+B` / `Ctrl+B` | Toggle Connections & Schema Sidebar | Global |
| `Cmd+Shift+F` / `Ctrl+Shift+F` | Format SQL Statement | Query Editor |
| `Cmd+S` / `Ctrl+S` | Save Active Query as Snippet | Query Editor |

---

## 5. Design Tokens & Visual Hierarchy

### Color Palette (Tailwind-compatible)
- **Background Deep**: `#090d16` (Slate 950) — main application shell.
- **Background Surface**: `#0f172a` (Slate 900) — editor and panel cards.
- **Border Subtle**: `#1e293b` (Slate 800) — clean dividing borders.
- **Accent Primary**: `#38bdf8` (Sky 400) — focus states, active tab markers, links.
- **Production Warning**: `#ef4444` (Red 500) — production badges, blocked destructive actions.
- **Staging Indicator**: `#f59e0b` (Amber 500) — staging banners and warning toasts.
- **Development Safe**: `#10b981` (Emerald 500) — dev badge, successful query execution.

### Typography
- **UI Font**: `Inter, system-ui, sans-serif` (13px baseline for compact navigation).
- **Code & Grid Font**: `JetBrains Mono, Fira Code, monospace` (13px, ligatures supported).

---

## 6. Table Designer & Structure Management (HeidiSQL-Style)

### Sub-Tab Navigation
When viewing an active table, developers switch between dedicated modes above the grid:
1. **`[📊 Data]`**: Keyset pagination, virtualized cell rendering, inline cell mutations.
2. **`[🏗️ Structure & PK]`**: Column metadata designer (Name, Type, Nullable, Default, Primary Key toggle, Auto-increment). Modifications are staged in memory with visual diff highlights before committing.
3. **`[🔑 Indexes]`**: List and create custom/unique indexes (`BTREE`, `UNIQUE`) or drop existing indexes.
4. **`[🔗 Foreign Keys]`**: Manage referential integrity constraints, source columns, referenced table/column, and cascade actions (`ON DELETE` / `ON UPDATE`).
5. **`[📜 CREATE Code]`**: Clean syntax-highlighted DDL generator with one-click clipboard copy.

### View & Trigger Designers
Dedicated schema dialogs accessible directly from the sidebar tree:
- **View Designer**: View naming and SQL `SELECT` definition editor with `CREATE OR REPLACE VIEW` generator.
- **Trigger Designer**: Structured trigger form with Target Table selector, Timing (`BEFORE`/`AFTER`), Event (`INSERT`/`UPDATE`/`DELETE`), and statement body editor.

---

## 7. Native Modal Dialog Architecture

All browser-blocking popup methods (`window.alert`, `window.confirm`, `window.prompt`) are eliminated in favor of standard HTML5 `<dialog>` elements (`showModal()`):
- **Notification Dialog (`#app-native-dialog`)**: Asynchronous, non-blocking alert with Esc key support and dark theme styling.
- **Safety Inspection Dialog (`#safety-inspection-dialog`)**: Dedicated security inspection modal displaying risk badges (`LOW`, `MEDIUM`, `HIGH`), destructive flag, challenge requirement, and query reason.
- **DDL Review Modal (`#ddl-review-dialog`)**: Pre-execution verification modal showing generated DDL statements before dispatching to the database.

---

## 8. Type Guard Palette & Default Value Presets

To protect developers from illegal or deprecated syntax (e.g. entering `int(200)` or unquoted string literals):
1. **Curated Type Guard Dropdown**:
   - Organized into native `<optgroup>` categories (Numeric, Text & String, Date & Time, Other).
   - Paired with an intelligent Length input that automatically disables (`N/A`) on lengthless types (`INT`, `BIGINT`, `TEXT`, `DATETIME`, `JSON`, `BOOLEAN`) to prevent deprecated length warnings in modern RDBMSs.
   - Automatically pre-fills standard lengths (e.g. `255` for `VARCHAR`, `10,2` for `DECIMAL`, `1` for `CHAR`).
2. **Auto-Increment & Primary Key Guard**:
   - Auto-increment is strictly locked to integer columns.
   - Marking a column as Primary Key automatically forces `NOT NULL` and locks the nullability control.
3. **Default Value Preset Selector**:
   - Preset dropdown (`None`, `NULL`, `CURRENT_TIMESTAMP`, `0`, `''`, `Custom...`).
   - Automatically applies dialect-aware quoting rules so raw string defaults are safely quoted without manual escaping.

---

## 9. Cross-Database Catalog & Autocomplete

For microservices and multi-database architectures:
- **Cross-Database Indexing**: SqlFox lazily prefetches database schemas across the connection.
- **Dynamic Suggestions**:
  - `FROM ` / `JOIN ` clauses suggest database names alongside local tables with a purple `[DB]` tag.
  - `dbname.` suggests all tables belonging to that database.
  - `dbname.tablename.` suggests all columns belonging to that specific remote table.
  - Cross-database table aliases (`FROM finance.invoices inv WHERE inv.`) resolve cleanly to remote column lists.

---

## 10. HeidiSQL-Style Session Switcher

- **Compact Header Button**: Replaces permanent sidebar connection accordions with a compact session bar displaying the active database icon, name, and environment tag (`[PROD]`, `[STAGE]`, `[DEV]`).
- **Instant Search & Grouping**: Expanding the switcher reveals an inline search filter, environment-grouped connections, and quick-action buttons (`➕ New`, `✏️ Edit`, `🗑️ Delete`).
- **Full-Height Schema Tree**: Reserves 100% of the sidebar's vertical space for exploring schema objects (Tables, Views, Triggers).

---

## 11. Cross-Database Schema Search (`Ctrl+P` / `Cmd+P`)

- **Instant Modal Dialog**: A floating spotlight-style search window accessible via `Ctrl+P` / `Cmd+P`, the header search icon `🔍`, or Command Palette.
- **Unified Object Indexing**: Instant sub-millisecond querying across all connected databases, indexing:
  - Tables with `[TBL]` green badge.
  - Views with `[VIEW]` purple badge.
  - Columns with `[COL]` sky blue badge alongside their data types.
- **Dual Action Keyboard Workflow**:
  - `Enter` / Primary Click: Switches active schema context if needed and immediately opens the Table Viewer.
  - `Shift+Enter` / `[Insert]` Button: Pastes the fully qualified identifier (`db.table` or `db.table.column`) at the editor's cursor position.

