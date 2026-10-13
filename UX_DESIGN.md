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
│ [SqlFox]  Prod-MySQL-Cluster [PROD 🔴]  [🔒 Read-Only]   [Ctrl+K Command Palette]  [⚙️ Settings] [−][□][×]│
├──────────────┬─────────────────────────────────────────────────────────┬───────────────────────────────┤
│ SCHEMA       │ [Tab 1: users.sql ●] [Tab 2: orders] [+]                │ AI ASSISTANT (BYOK / Ollama)  │
│ [Search... /]│─────────────────────────────────────────────────────────│───────────────────────────────┤
│ ▼ mydb       │ 1 | SELECT id, email, created_at, status                │ 🤖 Claude 3.5 Sonnet          │
│   ▼ Tables(3)│ 2 | FROM users                                          │                               │
│     users    │ 3 | WHERE created_at >= '2026-01-01'                    │ User:                         │
│     orders   │ 4 | ORDER BY id DESC;                                   │ "Optimize this query"         │
│     items    │                                                         │                               │
│   ▶ Views(0) │ [▶ Run (Ctrl+Enter)] [⚡ Explain] [🛡️ Impact Preview]    │ AI Copilot:                   │
│   ▶ Triggers │─────────────────────────────────────────────────────────│ "Query does a range scan on   │
│              │ Result: 1,000 rows (24ms)                [Filter...]    │ `created_at`. Consider adding:│
│              │┌──────┬──────────────────────┬────────────┬────────────┐│                               │
│              ││ id   │ email                │ created_at │ status     ││ CREATE INDEX idx_users_status │
│              │├──────┼──────────────────────┼────────────┼────────────┤│   ON users(created_at, status)│
│              ││ 1042 │ alice@example.com    │ 2026-09-10 │ active     ││                               │
│              ││ 1041 │ bob@example.com      │ 2026-09-09 │ pending    ││ [Apply to Editor] [Copy SQL] │
│              ││ 1040 │ charlie@example.com  │ 2026-09-09 │ active     ││                               │
│              │└──────┴──────────────────────┴────────────┴────────────┘│ [Ask anything about schema...]│
├──────────────┴─────────────────────────────────────────────────────────┴───────────────────────────────┤
│ MySQL 8.0.35 | Ping: 12ms | Memory: 42MB | SSL Enabled | UTF-8 | ⌨️ Shortcuts                          │
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
| `Cmd+\`` / `Ctrl+\`` | Toggle In-App Activity Console | Global |

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
- **Zero-Shift Navbar Alert Indicators**: All notification and alert indicators on navbar control buttons (such as the Monitor dropdown trigger) use absolute-positioned corner dots (`absolute -top-0.5 -right-0.5`) with incident details exposed via dynamic `title="..."` tooltips and dropdown menus. This guarantees zero horizontal layout shift (0px width jitter) and prevents adjacent buttons from shifting during background polling.
- **Zero-Shift Processlist Incident Triage**: Incident inspection for lock contention and root blockers is triggered on-demand via a toolbar pill button and dedicated modal dialog (`#lock-inspector-dialog`) or transient toast notifications. Inline push banners above the active threads table are strictly eliminated to prevent vertical layout shifts (table jumping up and down) during auto-refresh intervals.
- **Sidebar Visibility & Persistence**: Both Left Sidebar (Schema Explorer) and Right Sidebar (AI Copilot) persist their show/hide states to browser `localStorage` (`sqlfox_left_sidebar_visible` and `sqlfox_copilot_visible`, defaulting to `true`), with clickable header toggle buttons for mouse navigation.
- **Status Bar & Centralized Shortcuts Cheatsheet**: `h-6` (24px) footer dedicated exclusively to runtime technical telemetry (database engine, latency, memory gauge, SSL, encoding) and a dedicated `⌨️ Shortcuts` trigger. To prevent redundant visual noise and banner blindness, environment tags are strictly excluded from the footer and reserved for the Header Session Switcher. Clicking it (or pressing `?` / `F1`) opens a modal cheatsheet grouping all shortcuts (Query Execution, Navigation, Tabs, Tools).
- **Maximized Search & Filter Textbox Standards**: All search and filter input boxes across toolbars, drawers, and popovers (Processlist `#pl-search-input`, Session Audit Log `#audit-search-input`, Table Data Grid Raw WHERE filter `#filter-raw-input`, Storage Diagnostics `#dbdiag-search-input`, Session Switcher `#session-search-input`, Database Switcher `#database-search-input`, and Snippets Manager `#snippets-filter-input`) follow a unified UX pattern:
  - Dynamically fill/maximize available space (`flex-1 min-w-[200px]` or relative flex wrapper) without hardcoded cramped widths.
  - Leading search icon `🔍` on the left with consistent font-mono and padding.
  - Inline clear button `✕` on the right that appears dynamically when text is typed and vanishes when cleared.
  - Native `Escape` key shortcut inside the input to instantly clear the filter and reset focus.

---

## 6. Table Designer & Structure Management

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
- **In-App Self-Update Dialog (`#update-dialog`)**: Native modal presenting release metadata, version diffs (`vCurrent` ➔ `vLatest`), rendered Markdown changelogs, animated download/extraction progress states, and 1-click update execution.

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

## 10. Compact Session Switcher & Environment SSOT

- **Compact Header Button & SSOT**: Replaces permanent sidebar connection accordions with a compact session bar displaying the active database icon, name, and environment tag (`[PROD 🔴]`, `[STAGE 🟡]`, `[DEV 🟢]`). This header button is the **Single Source of Truth (SSOT)** for environment identity; sidebar, footer, and top borders are strictly freed from redundant environment badges and top ambient stripes to prevent visual clutter and alert fatigue. Redundant edit buttons on the session switcher bar are removed to prevent accidental misclicks with the dropdown arrow.
- **Production Read-Only Safety Interlock**: Connections marked as `Production` automatically initialize with the navbar toggle locked to `🔒 Read-Only`. Clicking to switch to `🔓 Read-Write` displays a confirmation modal (*"Switch to Read-Write mode on Production?"*), preventing inadvertent data mutations. High-risk statements (`DROP`, `DELETE`/`UPDATE` without WHERE) continue to require the AST Safety Guard Challenge Modal.
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
3. **Table Runbook Feed (`[📝 Notes & Runbook]`) & Markdown Live Preview**:
   - Provides a scoped operational runbook feed for the active table, displaying tagged notes with pinned runbooks at top.
   - The Note Composer provides live markdown preview rendering formatted headers, code blocks, checklists, and tables alongside captured SQL snapshots.
4. **Fluid Workspace Layout & Minimum Width Constraints**:
   - Center workspace (`#center-workspace`) enforces a hard `min-w-[400px]` floor during horizontal sidebar dragging.
   - Cluster Inspector, Flight Recorder, and Table Diagnostics metric grids use CSS Grid `repeat(auto-fit, minmax(...))` to fluidly re-wrap cards on any display size without horizontal clipping.

---

## 14. Navicat Power Suite & Data Synchronization Modals

The database tools dropdown (`📦 DB Tools ▾`) provides specialized native modals for schema comparison, live cross-connection streaming migration, and row-level reconciliation.

### Structure Sync (Schema Diff) Modal Wireframe (`#structure-sync-dialog`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 🔄 Structure Sync (Schema Diff)                                       [×]   │
├─────────────────────────────────────────────────────────────────────────────┤
│ Source: [ local_dev / store_db ▾ ]     Target: [ prod_replica / store_db ▾ ]│
│                                                        [ 🔍 Compare Schemas ]│
├─────────────────────────────────────────────────────────────────────────────┤
│ Structural Differences (3 items detected):                                  │
│ ┌──┬─────────┬──────────────────────┬─────────────────────────────────────┐ │
│ │☑ │ Type    │ Object               │ Proposed DDL Statement              │ │
│ ├──┼─────────┼──────────────────────┼─────────────────────────────────────┤ │
│ │☑ │ + TABLE │ `audit_events`       │ CREATE TABLE `audit_events` (...)   │ │
│ │☑ │ + COL   │ `users`.`deleted_at` │ ALTER TABLE `users` ADD COLUMN ...  │ │
│ │☑ │ ~ COL   │ `orders`.`amount`    │ ALTER TABLE `orders` MODIFY ...     │ │
│ └──┴─────────┴──────────────────────┴─────────────────────────────────────┘ │
│                                                                             │
│ Generated Migration Script Preview:                                         │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ -- SqlFox Structure Sync Migration                                      │ │
│ │ ALTER TABLE `users` ADD COLUMN `deleted_at` DATETIME NULL;               │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                   [Copy DDL]  [Apply to Target Database 🚀] │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Cross-Connection Data Transfer Modal Wireframe (`#data-transfer-dialog`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 🚚 Cross-Connection Data Transfer                                     [×]   │
├─────────────────────────────────────────────────────────────────────────────┤
│ Source Session: [ remote-prod ▾ ]       Target Session: [ local-dev ▾ ]     │
│ Database:       [ main_db     ▾ ]       Database:       [ dev_db    ▾ ]     │
├─────────────────────────────────────────────────────────────────────────────┤
│ Tables to Transfer:                                                         │
│ [☑ Select All]  [☐ Clear]                                                  │
│  ☑ `customers` (4,200 rows)      ☑ `orders` (18,500 rows)                   │
│  ☑ `order_items` (42,000 rows)   ☐ `logs` (580,000 rows)                    │
│                                                                             │
│ Options:                                                                    │
│  ☑ Create missing tables on target     ☑ Truncate target tables before copy │
│  Batch Size: [ 500 ▾ ]                                                      │
│                                                                             │
│ Progress: [████████████████████████████░░░░░░░░░] 74% (38,400 / 64,700 rows)│
│ Transferring `order_items`... 1,200 rows/sec                                │
│                                                [Cancel]  [Start Transfer 🚀]│
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 15. Visual ERD Interactive Canvas (`#erd-dialog`)

The Entity-Relationship Diagram canvas allows developers to explore table relationships, drag and position tables, and trace foreign key dependencies across complex schemas.

### Canvas Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 🗺️ Entity Relationship Diagram — store_db (12 tables)      [⚡ Auto Layout] │
│                                              [⊡ Fit All] [ 100% ▾ ] [× Close]│
├─────────────────────────────────────────────────────────────────────────────┤
│ ┌───────────────────────────┐                                               │
│ │ 📋 customers              │                                               │
│ ├───────────────────────────┤                                               │
│ │ 🔑 id          INT(11)    │────────┐                                      │
│ │    email       VARCHAR    │        │                                      │
│ │    created_at  DATETIME   │        │                                      │
│ └───────────────────────────┘        │ SVG Bézier Curve                     │
│                                      │ (amber path with arrowhead)          │
│                                      ▼                                      │
│                                ┌───────────────────────────┐                │
│                                │ 📋 orders                 │                │
│                                ├───────────────────────────┤                │
│                                │ 🔑 id          INT(11)    │                │
│                                │ 🔗 customer_id INT(11)    │                │
│                                │    total       DECIMAL    │                │
│                                └───────────────────────────┘                │
│                                                                             │
│ [Drag table cards to rearrange] • [Scroll wheel to zoom] • [Pan canvas]     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Top Navbar Access**:
   - Accessible under `📦 DB Tools ▾` -> `🗺️ Visual ERD Canvas` from anywhere in the application.
2. **Dynamic SVG Connections**:
   - Foreign key relationship lines are rendered as cubic Bézier curves connecting child table FK pins to parent table PK pins with directional arrowheads.
   - Dynamic edge detection selects the nearest card edge (left or right) based on relative horizontal card positions.
3. **Smooth Dragging & Event Delegation**:
   - Tables are rendered as lightweight HTML cards; dragging table headers updates card translation and recalculates connecting SVG curves in real time.
4. **Auto-Layout & Fit All**:
   - `⚡ Auto Layout` computes a square-root grid placement for all tables, avoiding overlapping cards.
   - `⊡ Fit All` calculates the bounding box of all table cards and scales/pans the viewport with padding to fit the modal screen.
5. **Session-Level Coordinate Cache**:
   - Table card positions are saved to `sessionStorage` (`erd-pos-{sessionId}-{db}`), so custom layouts remain intact during the active browsing session.

---

## 16. Enterprise-Scale UI Ergonomics & Resilience

### Global Reconnection Banner Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│       [ ⚠️ Connection lost. Reconnecting in 2s... ] (Floating Banner)       │
├─────────────────────────────────────────────────────────────────────────────┤
│ [SqlFox]  Prod-DB [PROD 🔴]   [Ctrl+K Command Palette]                      │
│                                                                             │
│ [Tab 1: migration.sql ●] [Tab 2: analytics]                                 │
│ 1 | UPDATE accounts SET balance = balance + 100 WHERE id = 42;              │
│ 2 | -- Unsaved draft is preserved across transient network disconnects      │
│                                                                             │
│ Virtual Data Grid: 10 staged cell updates kept intact in client memory      │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Native Sidebar Content-Visibility Virtualization**:
   - Table and view cards in the sidebar tree use `content-visibility: auto; contain-intrinsic-size: 0 28px;`.
   - The browser automatically skips style recalculation and layout rendering for off-screen items, maintaining 60fps scrolling across 1,000 to 10,000 database objects without external virtual list dependencies.
2. **Debounced Schema Filtering**:
   - The schema search input is wrapped in a 150ms debounce timer (`schemaFilterDebounceTimer`).
   - Typing quickly filters the in-memory schema cache without triggering DOM re-render spikes per keystroke.
   - Clicking `✕` (clear filter) bypasses the debounce and re-renders immediately.
3. **Global Reconnection Banner (`#global-reconnect-banner`)**:
   - If an API or network fetch fails, an ambient floating banner appears (`⚠️ Connection lost. Reconnecting in Xs...`).
   - Automatically probes `/api/auth/status` with exponential backoff (1s, 2s, 4s, 8s).
   - Upon reconnection, transitions smoothly to a green toast (`✓ Connection restored`) for 1.5s before fading out.
4. **State Retention Guarantee**:
   - Editor tabs, unsaved SQL scripts, active query results, and staged cell modifications (`stagedInserts`, `stagedUpdates`, `stagedDeletions`) remain untouched in client memory during disconnections, preventing accidental data loss.

---

## 17. Heuristic Index Advisor & Tuning Interface

### Execution Plan (EXPLAIN) Modal with Visual Execution Flow & Advisor Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ ⚡ Execution Plan (EXPLAIN) [MYSQL]       [📊 Plan Grid] [🌳 Visual Tree (2)]│
│ Query cost analysis, index utilization    [💡 Advisor (1)] [{ } Raw Plan]  [✕]│
├─────────────────────────────────────────────────────────────────────────────┤
│ ┌─ TARGET QUERY (128 chars) ─────────────── [✨ Format] [📋 Copy] [⚡ Editor]─┐ │
│ │ SELECT * FROM orders WHERE user_id = 1 AND amount > 50 ORDER BY created_at│ │
│ └───────────────────────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Visual Tree View Canvas]                                                   │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Total Steps: 2 · Est. Rows Examined: ~1,500 · ⚠️ 1 Full Scan · ⚠️ Filesort│ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ ┌── Step 1 ────────────────────────────────────────── ⚠️ ALL (Full Scan) ──┐ │
│ │ 🗄️ orders                                            ⚠ No index          │ │
│ │ [Examined: ~1,500 rows]  [Filtered: 10.0%]  [Select Type: SIMPLE]       │ │
│ │ ⚠ Filesort (Sort Buffer/Disk) · Where Filter                             │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                        │
│                        ↓ passes ~1,500 rows                                 │
│                                    │                                        │
│ ┌── 🏁 Client Result Set ──────────────────────────────────────────────────┐ │
│ │ Execution pipeline finished                       ~150 output rows       │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Dedicated Target Query Card**:
   - Query snippet is decoupled from the header, displayed in a dedicated syntax card with `✨ Format` / `↺ Raw` toggle, `📋 Copy SQL`, and `⚡ Editor` integration.
2. **Modern Segmented Tab Bar**:
   - High-contrast, sleek navigation control switching between `📊 Plan Grid`, `🌳 Visual Tree` (with live step counter badge), `💡 Advisor` (with alert badge), and `{ } Raw Plan`.
3. **Rich Visual Execution Tree**:
   - Renders graphical node cards with color-coded access badges (`ALL` in red, `INDEX` in amber, `RANGE` in yellow, `REF/EQ_REF` in emerald), table names, keys, metrics strips, and downward pipeline flow arrows (`↓ passes ~N rows`).
   - Universal Fallback: When `FORMAT=JSON` is unavailable (e.g. SQLite, older engines), renders the pipeline flow directly from tabular rows, ensuring a visual diagram is **never** empty.
4. **Index Advisor & 1-Click Clipboard**:
   - Color-coded ESR pill badges (`Equal`, `Sort`, `Range`).
   - One-click copy for safe DDL and direct editor injection.

---

## 18. In-App Self-Update Modal, Changelog Viewer & Workspace Author Attribution

### Status Bar & Update Modal Wireframe

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 🐬 MySQL 8.0.35 • localhost:3306 / mydb • Up 4d 12h • Latency: 2ms                                    │
│   Memory: 42MB  [📖 Docs]  [⌨️ Shortcuts]  [🚀 Update]  SqlFox v0.2.0                                 │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ 🚀 SqlFox Update  [v0.2.0]                                          [✕ Close]│
│ A new release is ready to install                                           │
├─────────────────────────────────────────────────────────────────────────────┤
│ [ Current: v0.1.0  ➔  New: v0.2.0 ]                    Published: Oct 4, 2026│
├─────────────────────────────────────────────────────────────────────────────┤
│ 📋 WHAT'S CHANGED IN THIS RELEASE                                           │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ ## What's Changed                                                       │ │
│ │ * Added In-App Self-Update Engine with 1-click binary replacement       │ │
│ │ * Integrated interactive Markdown changelog viewer                      │ │
│ │ * Added workspace author credit and GitHub attribution                  │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ [====================================                    ] 65%              │
│ Downloading release archive...                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ [GitHub Icon] View on GitHub                         [ Later ]  [🚀 Update Now]│
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Two-Region Status Bar**:
   - **Left**: Real-time database connection telemetry: dialect engine flavor & version (`🐬 MySQL 8.0.35` / `🦭 MariaDB 10.11`), connection host/database target, server uptime (`Up 4d 12h`), and interactive latency ping (`footer-latency`).
   - **Right**: Workspace utility cluster: live activity console button, heap memory telemetry, documentation link, keyboard shortcuts cheatsheet launcher, dynamic update action badge (`#footer-update-btn`), and persistent application version launcher (`#footer-version-btn`) opening the Changelog and About dialog.
2. **Transparent Changelog Inspection**:
   - Renders GitHub release notes using SqlFox's built-in zero-dependency Markdown parser, displaying formatted bullet points, headers, and code snippets before the user commits to updating.
3. **One-Click In-App Binary Replacement**:
   - Clicking `[🚀 Update Now]` triggers background archive download, atomic in-place replacement, and automatic application restart.
   - Displays real-time progress phases (*Downloading archive...*, *Extracting binary...*, *Restarting SqlFox...*).
4. **Reconnection Countdown**:
   - Displays a 3-second visual countdown following restart confirmation before automatically reloading the page, ensuring smooth reconnection to the upgraded daemon without manual browser refresh.

---

## 19. Grid Edit Presets, Type-Aware Editors & Universal Mutation Audit Drawer

### Edit Presets Pill & Type-Aware Editor Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ [NULL (Alt+N)] [Empty (Alt+E)] [NOW (Alt+T)] [Date (Alt+D)] [UUID (Alt+U)]      │
│┌───────────────────────────────────────────────────────────────────────────────┐│
││ 2026-10-05 16:30:00                                                  [✕] [✓]  ││
│└───────────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Floating Value Presets Toolbar**:
   - Docks above or below the active grid cell editor.
   - Provides 1-click pills for `NULL` (`Alt+N`), `Empty` (`Alt+E`), `NOW` (`Alt+T`), `Date` (`Alt+D`), and `UUID` (`Alt+U`).
   - Supports keyboard shortcuts to stage sentinel values without reaching for the mouse.
2. **Type-Aware Input Switching**:
   - **ENUM**: Dropdown menu containing introspected enum values with keyboard arrows traversal.
   - **BOOLEAN**: Segmented toggle switch (`TRUE` / `FALSE` / `NULL`).
   - **Foreign Keys**: Hyperlink with parent table lookup and hover inspection.
3. **PK-Less Row Targeting Indicator**:
   - When editing a table without a primary key, a subtle badge indicates that full-row criteria will be matched, guarding against unintended multi-row modifications with an enforced `LIMIT 1`.
4. **Universal Mutation Audit Badging & Source Filtering**:
   - The Session Audit Log drawer renders dedicated source badges: `[Editor]`, `[Grid Edit]`, `[Grid Add]`, `[Grid Del]`, `[Script]`, `[CSV Import]`, `[Mock Data]`, `[Sync Transfer]`, `[Schema Sync]`.
   - A dropdown filter allows users to view audit entries by specific source context.
   - Audit CSV export includes the `Source` column for forensic tracking.

---

## 20. Operational Notes Inbox, Composer & Reminder Follow-up Lifecycle

### Operational Notes Inbox Drawer & Composer Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 📝 Operational Notes Inbox                                        [+ New] [×]│
├─────────────────────────────────────────────────────────────────────────────┤
│ 🔍 [Search notes, tags, tables...                    ]                      │
│ [ All (12) ] [ Inbox (4) ] [ Overdue (1) ] [ Resolved (6) ] [ Archived (1) ]│
├─────────────────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ 📌 Slow Query Incident - checkout_orders             [🔔 Overdue 10m]   │ │
│ │ 🏷️ db://prod_cluster/store/checkout_orders  #incident #slow-query        │ │
│ │ Action: Needs compound index on (status, created_at) to avoid filesort.  │ │
│ │ 📋 Snapshot: SELECT * FROM checkout_orders WHERE status = 'pending'...  │ │
│ │ [Edit] [Snooze 10m] [✓ Resolve]                                         │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Header Inbox Trigger & Overdue Badge**:
   - The global header features a dedicated `[📝 Notes]` button.
   - A pulsing amber badge (`#notes-inbox-badge`) indicates when one or more notes have active reminders that are overdue, giving DBA and developers immediate situational awareness without intrusive modal blocking.
2. **Slide-Over Notes Drawer (`#notes-inbox-drawer`)**:
   - Slides smoothly from the right side of the screen over the active workspace.
   - Provides instant search, segmented status filters (`All`, `Inbox`, `Resolved`, `Archived`), and pinned runbook toggles.
   - Direct inline actions on cards: **Edit**, **Snooze 10m**, **Resolve/Reopen**, and **Delete**.
3. **Dual-Context Note Composer Modal (`#note-composer-dialog`)**:
   - **Operational Metadata**: Title, status, pin toggle, soft-link object tag (`db://conn/db/table`), and comma-separated user tags.
   - **Reminder Alarms**: Quick-schedule buttons (`+10m`, `+1h`, `Tomorrow 09:00`, `Next Monday`) and datetime-local input.
   - **Developer Notes**: Markdown textarea with live preview for runbooks and investigations.
   - **Raw Snapshot**: Code block previewing captured SQL queries, execution metrics, or AI explanations, preserving ground-truth forensic evidence alongside developer annotations.
4. **Interactive Alarm Chime & Toast**:
   - When a reminder matures, a Web Audio API chime plays (587Hz -> 880Hz ramp, zero MP3 asset dependency).
   - A non-blocking toast surfaces in the bottom-right corner with 1-click actions: `Open Note`, `Snooze 10m`, or `Done`.
5. **AI Copilot `@` Mention Autocomplete**:
   - Typing `@` in the Copilot prompt displays an interactive dropdown autocomplete matching operational note titles.
   - Selecting a note inserts `[@note:<title>]`.
   - On dispatch, the server extracts referenced notes and injects the raw snapshot and developer action notes directly into the system prompt context.

## 21. Workspace Ergonomics, Tab Lifecycle Suite & Process Activity Trend

### Multi-Pane Workspace & Splitter Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [Query 1.sql ✕] [Query 2.sql ✕] [users (Table) ✕] [+]                       │
├─────────────────────────────────────────────────────────────────────────────┤
│ ▶ Run (Ctrl+Enter) | ⚡ Explain | 🛡️ Safety | ✨ Format | 📝 Save Note     │
├─────────────────────────────────────────────────────────────────────────────┤
│ 1 | SELECT o.id, o.customer_id, o.amount, o.status                         │
│ 2 | FROM orders o                                                           │
│ 3 | WHERE o.created_at >= '2026-10-01'                                      │
│ 4 | ORDER BY o.id DESC LIMIT 50;                                            │
├═════════════════════════════════════════════════════════════════════════════┤
│ ═════════════════════ [ ─── Workspace Pane Splitter ─── ] ═════════════════ │ ← Drag (row-resize) / DblClick (50:50)
├─────────────────────────────────────────────────────────────────────────────┤
│ 15 rows returned in 12ms                                     [Export CSV ▾] │
├────┬────────────┬─────────────┬─────────────┬───────────┬───────────────────┤
│    │ id (INT)   │ customer_id │ amount      │ status    │ created_at        │
├────┼────────────┼─────────────┼─────────────┼───────────┼───────────────────┤
│ 1  │ 98412      │ 1042        │ 450.00      │ completed │ 2026-10-08 14:22  │
│ 2  │ 98411      │ 891         │ 120.50      │ pending   │ 2026-10-08 14:19  │
└────┴────────────┴─────────────┴─────────────┴───────────┴───────────────────┘
```

### Tab Context Menu Wireframe

```
┌───────────────────────────────────────────┐
│ ✕ Close                           Ctrl+W  │
│ 🗂️ Close Others                            │
│ ➡️ Close Tabs to the Right                 │
│ 🗑️ Close All                               │
├───────────────────────────────────────────┤
│ 📝 Save as Operational Note...            │
│ ⎘ Duplicate Tab                           │
│ ✏️ Rename Tab...                           │
└───────────────────────────────────────────┘
```

### Process Activity Trend Panel Wireframe

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ ⚡ Processlist (28 threads)    [Slow > 1s] [Filter...]  [📈 Activity Trend ▾]│
├─────────────────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Running: 3  |  Total: 28  |  Peak: 12  |  Avg: 2.4  | [Escalate to Recorder]│ │
│ │ 12 ┤                                   /\                               │ │
│ │  8 ┤              /\                  /  \                              │ │
│ │  4 ┤  ___________/  \________________/    \__________________________   │ │
│ │  0 ┼─────────────────────────────────────────────────────────────────── │ │
│ │    60s ago                                                      Now     │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Interaction Models

1. **Workspace Pane Splitter**:
   - Draggable horizontal separator between `#editor-container` and `#grid-container`.
   - Hover shows sky-blue accent with row-resize cursor.
   - Enforces minimum height bounds (Editor min 80px, Grid min 120px).
   - Double-click resets height to 50:50 ratio.
   - Persists height ratio in `localStorage` (`sqlfox_workspace_editor_height`).
   - Automatically hides when non-query tabs (Table Viewer, Processlist, etc.) are active.
2. **Tab Context Menu & Close Confirmation Guard**:
   - Right-click tab context menu provides `Close`, `Close Others`, `Close Tabs to the Right`, `Close All`, `Duplicate Tab`, `Rename Tab...`, and `Save as Operational Note...`.
   - Modifying a query tab marks the buffer as dirty. Closing a dirty tab triggers a native confirm modal. Clean tabs close immediately without prompt.
   - Closing all tabs resets to a fresh `Query 1.sql`.
3. **Master PIN Shoulder-Surfing Masking**:
   - Master PIN badge in header displays `fox-••••` by default.
   - Clicking `👁️` temporarily reveals plain text digits.
4. **Process Activity Trend Panel**:
   - Collapsible 60-second in-memory circular buffer rendered on HTML5 Canvas.
   - Primary solid sky-blue line shows active running queries (`Command != 'Sleep'`).
   - Secondary dashed slate line shows total thread count.
   - Metric pills show instant KPIs with 1-click shortcut to Flight Recorder.
5. **UI Resilience & Non-Wrapping Controls**:
   - Universal anti-shrink and anti-wrap guard on all buttons (`button:not(.allow-wrap)`) preventing multi-line label collapse or deformed pill dimensions when the window narrows.
   - Horizontal toolbars (`header`, `editor-toolbar`, `table-mode-tabs`, `grid-toolbar`, `footer`) employ bounded horizontal overflow (`overflow-x-auto gap-3`) with `shrink-0` groups to prevent button overlap, text collisions, or UI clipping.
   - Content text (AI chats, Operational Notes, modal logs) preserves word wrapping for maximum reading comfort.

---

## 22. Fast Schema Catalog Synchronization & Root Blocker Incident Triage

### 1. Fast Schema Catalog Synchronization Pre-Flight Modal (`#refresh-all-dialog`)

```
┌────────────────────────────────────────────────────────────────────────┐
│ 🔄 Refresh All Schema Catalogs                                     [×]│
│ Synchronize local autocomplete & search catalog cache                  │
├────────────────────────────────────────────────────────────────────────┤
│ 💡 Why is this feature useful?                                         │
│ • Cross-DB Autocomplete: Instant db.table and table.col completions   │
│ • Global Search (Ctrl+P): Find tables & views across all databases     │
│ • Zero Foreign Key Lock: Scans lightweight identifiers only           │
│ • Isolated Persistence: Cache preserved per active session             │
├────────────────────────────────────────────────────────────────────────┤
│ Server Health Status:  [● Normal: 3 Running Threads]                   │
│ Safe to synchronize. Server is currently experiencing low thread load. │
├────────────────────────────────────────────────────────────────────────┤
│ Progress: [████████████████████████████████████████] 100% (8/8 DBs)   │
│ Complete! All schema catalogs synchronized.                            │
├────────────────────────────────────────────────────────────────────────┤
│ [Cancel]                                     [Proceed with Refresh All]│
└────────────────────────────────────────────────────────────────────────┘
```

### 2. Root Blocker Incident Triage Wireframe (`#lock-dialog`)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ 🚨 ROOT BLOCKER   Thread #4120  (batch_worker)  holding EXCLUSIVE on orders             │
│                                              [Kill Query] [🚨 Kill Blocker]             │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ SQL: UPDATE orders SET status = 'processing', updated_at = NOW()                        │
│      WHERE status = 'pending' AND batch_id = 941;                                       │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ ↳ Blocked: Thread #4891 (web_app) waiting 45s for lock:                                 │
│   ALTER TABLE orders ADD COLUMN priority_score INT DEFAULT 0;                           │
## 23. In-App Activity Console & Live Diagnostic Stream

A collapsible bottom-docked telemetry drawer situated above the status footer, providing real-time visibility into internal daemon activities without leaving the query workspace.

### 1. Activity Console Drawer Wireframe (`#activity-console-drawer`)

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ ⚡ Activity Console  [● LIVE]   [ALL] [ERROR] [WARN] [INFO] [DEBUG]   [Filter logs...]  [⏸ Pause] [🗑️ Clear] [💾 Export ▼] [✕] │
├──────────────┬───────┬──────────┬─────────────────────────────────────────────────────────┬────────────┤
│ TIME         │ LEVEL │ MODULE   │ ACTIVITY / MESSAGE                                      │ DETAILS    │
├──────────────┼───────┼──────────┼─────────────────────────────────────────────────────────┼────────────┤
│ 15:04:02.140 │ INFO  │ query    │ query_executed (SELECT * FROM users WHERE active = 1)   │ 12ms • 5 rows ▶│
│ 15:04:05.812 │ WARN  │ http     │ http_request (POST /api/connections/4/test)             │ 400 • 28ms ▶│
│ 15:04:08.330 │ ERROR │ query    │ query_failed (SELECT * FROM nonexistent_table)          │ 3ms • syntax err ▶│
└──────────────┴───────┴──────────┴─────────────────────────────────────────────────────────┴────────────┘
│ Footer: 🗄️ MySQL 8.0 • prod-db • Latency: 12ms • ⚡ Activity [● Live] [🔴 1] • Memory: 32MB        │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 2. Interaction & State Design
- **Toggle Access**: Open or close via `Ctrl + ` ` keyboard shortcut or clicking the footer button `⚡ Activity`.
- **Smart Auto-Scroll**: Viewport tracks newest events at bottom. When an operator scrolls up to inspect historical entries, auto-scrolling automatically freezes until scrolled back to the newest row.
- **Freeze & Pause**: The `⏸ Pause` button locks incoming table updates while retaining the telemetry connection in the background.
- **Client-Side Retention & Export**: Maintains a FIFO ring buffer of 1,000 items in browser memory. 1-click export allows instant download as human-readable `.log` (plain text) or structured `.json`.











