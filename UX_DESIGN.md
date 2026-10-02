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
- **UI Font**: `Inter, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif` (13px baseline for compact navigation, `-webkit-font-smoothing: antialiased`).
- **Code & Grid Font**: `JetBrains Mono, Fira Code, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace` (Query Editor: 14px, line-height 1.65, letter-spacing 0.01em; Virtual Data Grid: 12.5px-13px, `font-variant-numeric: tabular-nums`).

### Header & Toolbar Controls
- **Navbar Container**: `h-12` (48px baseline).
- **Navbar Control Buttons**: `h-7` (28px fixed height, `flex items-center px-2.5 rounded text-xs`) across left sidebar toggle (`◨`), center shortcuts (Command Palette, Schema Search) and right-side action triggers (Processlist, Audit Log, Diagnostics, Privileges, Vault, Copilot, `🔒 Read-Only` toggle, PIN badge). All inline `<kbd>` badges are omitted from button text to prevent visual clutter, with keyboard shortcuts cleanly exposed via native `title="..."` tooltips.
- **Sidebar Visibility & Persistence**: Both Left Sidebar (Schema Explorer) and Right Sidebar (AI Copilot) persist their show/hide states to browser `localStorage` (`sqlfox_left_sidebar_visible` and `sqlfox_copilot_visible`, defaulting to `true`), with clickable header toggle buttons for mouse navigation.
- **Status Bar & Centralized Shortcuts Cheatsheet**: `h-6` (24px) footer containing live environment status, database engine, latency, memory gauge, and a dedicated `⌨️ Shortcuts` trigger. Clicking it (or pressing `?` / `F1`) opens a modal cheatsheet grouping all shortcuts (Query Execution, Navigation, Tabs, Tools).
- **Maximized Search & Filter Textbox Standards**: All search and filter input boxes across toolbars, drawers, and popovers (Processlist `#pl-search-input`, Session Audit Log `#audit-search-input`, Table Data Grid Raw WHERE filter `#filter-raw-input`, Storage Diagnostics `#dbdiag-search-input`, Session Switcher `#session-search-input`, Database Switcher `#database-search-input`, and Snippets Manager `#snippets-filter-input`) follow a unified UX pattern:
  - Dynamically fill/maximize available space (`flex-1 min-w-[200px]` or relative flex wrapper) without hardcoded cramped widths.
  - Leading search icon `🔍` on the left with consistent font-mono and padding.
  - Inline clear button `✕` on the right that appears dynamically when text is typed and vanishes when cleared.
  - Native `Escape` key shortcut inside the input to instantly clear the filter and reset focus.

---

## 6. Table Designer & Structure Management (HeidiSQL-Style)

### Sub-Tab Navigation
When viewing an active table, developers switch between dedicated modes above the grid:
1. **`[📊 Data]`**: Keyset pagination, virtualized cell rendering, inline cell mutations.
2. **`[🏗️ Structure & PK]`**: Column metadata designer (Name, Type, Nullable, Default, Primary Key toggle, Auto-increment). Modifications are staged in memory with visual diff highlights before committing.
3. **`[🔑 Indexes]`**: List and create custom/unique indexes (`BTREE`, `UNIQUE`) or drop existing indexes.
4. **`[🔗 Foreign Keys]`**: Manage referential integrity constraints, source columns, referenced table/column, and cascade actions (`ON DELETE` / `ON UPDATE`).
5. **`[📜 CREATE Code]`**: Clean syntax-highlighted DDL generator with one-click clipboard copy.
6. **`[🗑️ Drop Table]`**: Dedicated high-visibility safety button placed at the right of the sub-tabs bar (and via right-click context menu in the sidebar schema tree), protected by the AST Safety Guard Challenge. Table list rows in the sidebar remain clean with right-aligned row counts and zero floating hover icons.

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
- **Collapsible Schema Object Sections**: Dedicated toggleable headers for `📊 Tables`, `👁️ Views`, and `⚡ Triggers` with interactive chevron indicators (`▼` / `▶`), object count badges, and persistent state saved to browser `localStorage`. Active filter queries automatically expand sections to ensure search visibility.

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

---

## 12. Performance Flight Recorder & Correlated Query Drill-Down

The Performance Flight Recorder enables developers and DBAs to isolate transient latency spikes and root-cause queries without running heavy 24/7 APM daemons.

### Interface Wireframe

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ [SqlFox]  Prod-DB [PROD 🔴]    [⚡ Processlist] [🌐 Cluster] [📈 Recorder ● REC 00:03:12]   [🤖 Copilot]│
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ [Tab 1: users.sql] [Tab 2: 📈 Flight Recorder] [+]                                                     │
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ [⏹ Stop Recording]  [Interval: 1s ▼]  [● REC 03:12 (Auto-stop in: 06:48)]    [Sesi: Live ▼] [📥 Export]│
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ ┌──────────────────────┬──────────────────────┬──────────────────────┬───────────────────────────────┐ │
│ │ THROUGHPUT           │ THREADS RUNNING      │ ROW LOCK WAITS       │ BUFFER POOL WAITS             │ │
│ │ 🟢 1,420 qps         │ 🔴 18 / 64 conn      │ 🟡 3 waits           │ 🟣 0 stalls                   │ │
│ └──────────────────────┴──────────────────────┴──────────────────────┴───────────────────────────────┘ │
│                                                                                                        │
│ ┌────────────────────────────────────────────────────────────────────────────────────────────────────┐ │
│ │ Telemetry Timeline: Hover or click any spike to isolate active queries                            │ │
│ │ 1600│                                              /\   <- [Spike at 22:15:42]                     │ │
│ │ 1200│                                             /  \      QPS: 1,420 | Threads: 18 | Locks: 3    │ │
│ │  800│     /\                                     /    \                                            │ │
│ │  400│____/  \___________________________________/      \__________________________________________ │ │
│ │    0└─────────────────────────────────────────────│────────────────────────────────────────────────┘ │
│ │     22:14:00             22:14:30             22:15:00             22:15:30             22:16:00     │
│ └────────────────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                                        │
│ Correlated Non-Idle Queries [At 22:15:42 - 2 active queries]                                           │
│ ┌────────────────────────────────────────────────────────────────────────────────────────────────────┐ │
│ │ Thread #40812  ⏱ 4s  root@192.168.1.50  📁 analytics  (executing)   [⚡ Explain][📝 To Tab][🤖 AI]│ │
│ │ SELECT o.id, SUM(i.price) FROM orders o JOIN order_items i ON o.id=i.order_id GROUP BY o.id;      │ │
│ ├────────────────────────────────────────────────────────────────────────────────────────────────────┤ │
│ │ Thread #40890  ⏱ 2s  app@192.168.1.12   📁 analytics  (Updating)    [⚡ Explain][📝 To Tab][🤖 AI]│ │
│ │ UPDATE user_balances SET balance = balance - 150.00 WHERE user_id = 9812;                          │ │
│ └────────────────────────────────────────────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Global Recording Pulsing Badge**:
   - Starting a flight recording shows a high-contrast pulsing red badge in the top navbar (`● REC 00:03:12`) visible across all tabs.
   - Built-in 10-minute auto-stop safety timer prevents forgotten recording runs from polling production databases indefinitely.
2. **Interactive Scrubber & Inspection**:
   - Scrubbing the Canvas timeline dynamically moves a vertical dashed cursor and reveals instant hover tooltips with exact numeric vectors.
   - Clicking a spike pins the timeline and updates the Correlated Queries drawer below.
3. **Seamless Query Handoff**:
   - Every captured slow or blocking statement provides 1-click actions:
     - `⚡ Explain`: Visual execution plan breakdown.
     - `📝 To Tab`: Pre-populates the query in an editor tab for isolated debugging.
     - `🤖 Ask Copilot`: Formulates an AI prompt with the query, duration, system QPS, and running thread count for instant tuning advice.
     - `📋 Copy`: Copies raw SQL to clipboard.
4. **Vault Persistence & JSON Export**:
   - Finished sessions are stored in the local SQLite vault, selectable via dropdown for post-incident retrospective reviews, and exportable to portable JSON files.

---

## 13. AI Copilot Rich Markdown Blocks, Session Forking & Fluid Workspace

The AI Copilot interface integrates execution controls directly into generated message blocks, supports conversation branching, and enforces responsive workspace ergonomics.

### Copilot Message & Interactive SQL Block Wireframe

```
┌─────────────────────────────────────────────────────────────┐
│ 🤖 Copilot                                   [🔀 Branch] [📋]│
├─────────────────────────────────────────────────────────────┤
│ To optimize customer spend calculation, use an aggregated   │
│ covering index and limit query scope:                       │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ SQL (MySQL)             [📋 Copy] [▶ Insert & Run] [🔄]  │ │
│ ├─────────────────────────────────────────────────────────┤ │
│ │ SELECT customer_id, SUM(amount) AS total_spend          │ │
│ │ FROM transactions                                       │ │
│ │ WHERE transaction_date >= '2026-01-01'                  │ │
│ │ GROUP BY customer_id                                    │ │
│ │ ORDER BY total_spend DESC LIMIT 10;                     │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ **Expected Impact**: Converts table scan to index-only range │
│ scan with 95% latency reduction.                            │
└─────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Interactive SQL Block Toolbars**:
   - Every fenced SQL code block rendered in the Copilot chat drawer features a top toolbar:
     - `📋 Copy`: Copies the raw SQL code block to clipboard.
     - `▶ Insert & Run`: Automatically switches to or creates a query editor tab, populates the SQL statement, and immediately dispatches execution to the active database session.
     - `🔄 Replace`: Replaces the current query editor content with the snippet.
2. **Chat Session Forking (`🔀 Branch`)**:
   - Each assistant message includes a `🔀 Branch` button.
   - Clicking it branches a new Chat Session named `<Title> (Fork)`, copying the prompt and message sequence up to that point.
   - Allows users to explore alternative tuning ideas or test edge cases without losing the root conversation thread.
3. **Internal Notes (`Notes.md`) Markdown Live Preview**:
   - The Notes tab provides an `✏️ Edit` / `👁️ Preview` segmented switch.
   - In preview mode, raw markdown is rendered with formatted headers, code blocks, lists, and tables.
4. **Fluid Workspace Layout & Minimum Width Constraints**:
   - Center workspace (`#center-workspace`) enforces a hard `min-w-[400px]` floor during horizontal sidebar dragging.
   - Cluster Inspector, Flight Recorder, and Table Diagnostics metric grids use CSS Grid `repeat(auto-fit, minmax(...))` to fluidly re-wrap cards on any display size without horizontal clipping.



