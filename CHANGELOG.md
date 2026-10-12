# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Fast Schema Catalog Synchronization & Pre-Flight Health Check (`driver`, `server`, `web`)**: Added `InspectFastCatalog` across the core Driver interface, MySQL, PostgreSQL, and SQLite, querying table names and column definitions in a single sweep without issuing heavy foreign key (`KEY_COLUMN_USAGE`) or index queries. Added `GET /api/session/{id}/fast-catalog` and `GET /api/session/{id}/active-threads`.
- **Schema Cache Pre-Flight Modal & Status Badge (`web`)**: Added `#refresh-all-dialog` triggered by the `🔄 All` button next to the Database Switcher with feature benefits, live server health evaluation (>50 caution, >100 critical warning based on `ActiveThreads`), and an animated progress bar. Added `#schema-cache-badge` displaying the last cached timestamp on the active schema header.
- **Windows PE Binary Icon Consistency Test (`build`, `test`)**: Added automatic recompilation of `rsrc_windows_amd64.syso` and `rsrc_windows_arm64.syso` in `scripts/generate-icons.py` using `rsrc` and unit test `TestWindowsEmbeddedIconConsistency` in `cmd/sqlfox/main_test.go` verifying that all icon frames from `assets/icon.ico` are embedded into Windows PE resource files.

### Changed
- **Lock Contention & Root Blocker UX Overhaul (`web`)**: Redesigned Root Blocker visual hierarchy in the Lock Contention Inspector with prominent `🚨 ROOT BLOCKER` tags, scrollable full SQL query containers (`max-h-48 overflow-y-auto font-mono text-[13px]`), color-coded `Kill Query` (amber) vs `Kill Blocker` (red) buttons, and high-visibility waiting durations for blocked queries.
- **Session Switcher Bar Ergonomics (`web`)**: Removed redundant edit button next to the session switcher dropdown arrow to prevent misclicks when switching active database connections.

## [v0.7.0] - 2026-10-12

### Added
- **Auto-Refresh Visual Countdown Bar (`web`)**: Added GPU-accelerated CSS draining countdown progress bar under refresh controls for Processlist Manager and Cluster & Replication Inspector, synchronized with interval timers, manual refresh triggers, tab switching, and page visibility changes.
- **Multi-Arch Docker & Containerized Distribution (`deploy`, `ci`)**: Published ultra-lightweight (~20MB), non-root Alpine container image (`ghcr.io/herliansyah/sqlfox`) supporting `linux/amd64` and `linux/arm64`. Includes multi-stage `Dockerfile`, production-ready `docker-compose.yml`, persistent volume `/data` mapping, automated GHCR release publishing, and comprehensive host-gateway connection documentation.
- **High-Contrast CodeMirror Dark Theme Palette (`web`)**: Fully customized syntax highlighting for dark background workspaces. Table, database, and column backtick identifiers (`.cm-variable-2`, `.cm-variable-3`) render in Sky-300 cyan (`#7dd3fc`), data types (`.cm-type`) in Violet (`#c084fc`), constants and booleans (`.cm-atom`) in Coral (`#fb923c`), and parentheses/brackets in neutral slate with glowing cyan matching-bracket highlights, eliminating illegible dark-blue defaults.

### Changed
- **Production Ambient Top Stripe Elimination (`web`)**: Removed the top ambient red accent stripe on production connections, consolidating production environment awareness onto the Session Switcher environment badge as the single source of truth.
- **Modern Geometric Tech Fox Brand Mark & Icon Set (`brand`, `web`)**: Replaced legacy multi-facet low-poly fox emblem with a unified, solid orange (`#ea580c`) geometric tech fox mark. Synchronized master SVG vectors across `docs/assets/logo.svg`, `web/dist/assets/logo.svg`, and `web/dist/favicon.svg`, regenerated multi-resolution desktop and PWA icon bundles (`icon.png`, `icon.ico`, `icon-512`, `icon-192`, `apple-touch-icon`), and updated embedded `#sqlfox-brand-mark` symbol in `web/dist/index.html`.

## [v0.6.1] - 2026-10-12

### Added
- **Rich Markdown Note Previews (`web`)**: Render formatted Markdown previews directly in note cards within the Notes Inbox drawer.

### Fixed
- **Remote Lock Screen Flash Elimination (`internal/server`)**: Inject the `vault-locked` class and unhide the `#remote-lock-modal` directly into the server-rendered HTML for unauthenticated remote sessions, eliminating visual UI flashing during initial stream rendering and hard reloads.
- **Processlist Fallback & Lock Contention Parsing (`internal/driver`)**:
  - **MySQL / MariaDB**: Added automatic fallback to `information_schema.processlist` when `performance_schema` is disabled or empty, and fixed lock wait joins for MariaDB and MySQL 5.7.
  - **PostgreSQL**: Extended blocking PID parser to handle pgx `int4[]` slices, `int64` arrays, and string representations from `pg_blocking_pids()`.
- **Dropdown Menu Stacking Order (`web`)**: Adjusted Notes Inbox drawer stacking to `z-[35]` to prevent overlapping top navbar dropdown menus (`Monitor`, `DB Tools`, `Settings`).

## [v0.6.0] - 2026-10-11

### Added
- **Transaction Session Engine & Dedicated Connection Pinning (`internal/driver`, `internal/server`)**: Implemented `TxSession` driver interface with transaction isolation, context-based query cancellation, and row-stream cleanup across SQLite, MySQL, and PostgreSQL. Intercepts `BEGIN`/`COMMIT`/`ROLLBACK` statements to pin dedicated database connections per workspace tab, backed by an automated 60-second idle transaction rollback reaper and background session cleanup.

### Security
- **SQL Execution & Identifier Safety Hardening (`internal/safety`, `internal/driver`)**: Strips leading single-line (`--`) and multi-line (`/* ... */`) comments before safety classification to prevent rule-bypass; strictly sanitizes and dialect-escapes table and column identifiers across MySQL, PostgreSQL, and SQLite; restricts pre-execution auto-backups on `DROP`/`TRUNCATE` to physical tables (skipping views and databases); and blocks non-query statements on read-only sessions and `/stream` endpoints.

### Changed
- **CLI Row-Streaming Execution (`internal/cli`)**: Aligned headless query execution (`sqlfox query`) and data transfer (`sqlfox transfer`) CLI runners with driver row-streaming iterators, optimizing memory footprints and chunk throughput during large dataset dumps and transfers.

### Fixed
- **UI Glitch & Viewport Resilience Hardening**:
  - **Context Menu Lifecycle & Mutual Exclusion**: Unified context menu dismissal via `closeAllContextMenus()` across Tab, Sidebar Table, Data Grid, and Processlist context menus, preventing multiple floating menus from rendering concurrently when right-clicking across workspace boundaries. Added coordinate clamping (`Math.max(10, ...)`) and `max-h-[85vh] overflow-y-auto` to `#grid-context-menu` to prevent long ENUM option lists from overflowing offscreen.
  - **Tab Bar Compression Guard**: Enforced `shrink-0` on `#tabs-container` and `shrink-0 whitespace-nowrap tab-label` on tab items in `renderTabs()` per ADR-0048 to prevent flex squishing when numerous tabs are open.
  - **Modal Dialog & Form Height Resilience**: Added `max-h-[90vh]` and `overflow-y-auto` across all dialogs and modals (`ddl-modal`, `app-native-dialog`, `connection-modal`, `ai-settings-modal`, `vault-modal`, `safety-challenge-modal`, `view-designer-dialog`, `trigger-designer-dialog`, `database-creator-dialog`, `safety-inspection-dialog`, `mock-data-dialog`, `structure-sync-dialog`, `data-sync-dialog`, `data-transfer-dialog`, `table-creator-dialog`, `smart-seed-dialog`, `run-sql-dialog`, `ddl-review-dialog`) and backdrop-click dismissal to ensure action buttons and footers remain visible and accessible on constrained screen heights or high zoom.
  - **Processlist Lock Contention Indicator Synchronization**: Fixed view switcher in Processlist tab (`setProcesslistView`) to toggle `pl-lock-summary` instead of non-existent orphan IDs `pl-lock-banner` and `pl-no-lock-banner`, ensuring lock alert badges properly hide when switching away to digest/deadlock views.
  - **Inline Attribute Single-Quote Sanitization**: Enhanced `escapeHtml` to escape single-quotes (`'`) as `&#39;`, preventing JavaScript syntax errors (`Uncaught SyntaxError: Unexpected identifier`) when values, usernames, or database identifiers containing apostrophes are rendered into inline `onclick` handlers.
  - **New Tab Plus Button Flex Guard**: Enforced `shrink-0` on `#tabs-container` new query tab button (`+`) to prevent flex compression when multiple tabs are open.
  - **Cluster Node Switcher Connection Auto-Fill**: Corrected target input IDs in `switchSessionToNode` from non-existent legacy IDs (`conn-host`, `conn-port`, `conn-name`) to modal elements (`modal-conn-host`, `modal-conn-port`, `modal-conn-name`), allowing one-click cluster node connections from the Galera/Cluster tab to properly populate the connection form.
  - **Privilege Matrix Scope Handler**: Defined missing `onPrivScopeChanged()` function to eliminate unhandled `ReferenceError` when switching between Global (`*.*`) and Active Database scope in the User Accounts & Privilege Matrix dialog.
  - **Inline Attribute Dataset Safety**: Replaced fragile inline string literal passing with HTML5 dataset attributes (`data-query`, `data-snippet`, `data-var`, `data-val`) across Root Blocker Inspector, Server Variables, Cluster Health checks, and Foreign Key Peek, completely eliminating JavaScript `SyntaxError` caused by browser attribute entity decoding on values containing single quotes, contractions, or newlines.
  - **JSON Cell Viewer Viewport Resilience**: Added `overflow-y-auto`, resilient `min-h-[180px]`, and `resize-none` to `#json-cell-dialog` body to prevent action buttons and footer metadata from being pushed offscreen or clipped on compact displays.
  - **Responsive Header Search Squeeze Protection**: Added responsive collapsing (`hidden xl:inline`) on Command Palette and Schema Search labels to retain clickable icon triggers while preventing right-side toolbar controls (Monitor, DB Tools, Settings, Notes, Copilot) from overflowing outside the viewport on <1280px displays.
  - **Complete Backdrop Dismissal & Escape Coverage**: Added `table-creator-dialog`, `view-designer-dialog`, `trigger-designer-dialog`, `safety-inspection-dialog`, and `ddl-review-dialog` to the native backdrop click auto-dismiss listener and global Escape key teardown loop.
  - **Notes Search Escape Isolation**: Handled `Escape` key inside `#notes-search-input` to clear query and blur with `event.stopPropagation()`, preventing unexpected collapse of the entire Notes Inbox drawer.
  - **Universal Single-Quote Dataset Hardening**: Converted table, view, routine, and event items in Schema Explorer, database diagnostics maintenance buttons, table index/FK drop actions, and grid inline editing (`startInlineEdit`, `updateGridDraftCell`) to use HTML5 `dataset` attributes, eliminating `SyntaxError: missing ) after argument list` on identifiers containing apostrophes or quotes.


## [v0.5.2] - 2026-10-09

### Added
- **Manual Update Check Affordance**: Dedicated "Check for Updates" button in the Changelog modal footer (`#changelog-check-updates-btn`) allowing operators to trigger on-demand release checks directly from the UI, with explicit error state feedback when GitHub API limits or network issues occur.

### Fixed
- **Processlist & Activity Trend UI Resilience**: Enforced `whitespace-nowrap`, `shrink-0`, and `overflow-x-auto gap-3` across Processlist toolbar toggles (`Hide Sleep`, `Hide System`, `Threads: N`), collapsible Activity Trend panel header, Statement Digest, and Deadlock History toolbars to prevent word wrapping and squashed controls during window resizing.
- **Connection Action Visibility & Grid Export Dropdown**: Replaced `hidden group-hover:inline-block` with `opacity-0 group-hover:opacity-100` on connection sidebar items to bypass global `.hidden !important` suppression, added hover/click edit affordances to the sidebar session switcher and navbar connection badge, and removed `overflow-x-auto` from the grid toolbar to prevent clipping the CSV/JSON export dropdown menu.

## [v0.5.1] - 2026-10-09

### Fixed
- **Navbar Dropdown Menu Visibility**: Removed `overflow-x-auto` on the top navbar container to prevent CSS clipping of absolutely positioned dropdown menus (`Monitor`, `DB Tools`, `Settings`) at the 48px header boundary.
- **Single-Instance Port Collision Guard (`cmd/sqlfox`)**: Added explicit application signature validation (`app: sqlfox` and `X-SqlFox-App` header) to health checks and PID liveness verification to prevent false single-instance detection when third-party services occupy port 8080.

## [v0.5.0] - 2026-10-09

### Added
- **Cloud Sync & GitHub Replication Engine (`internal/vault`, `internal/server`)**: Asynchronous, offline-first background replication of user notes, documentation, and SQL snippets to any GitHub repository using official GitHub REST API (v3/v4) without requiring external `git` CLI installations. Features sensitive token encryption at rest via AES-256-GCM and Argon2id, atomic commits using the Git Database API, mirror deletions, optional encrypted Connection Vault backup archive, and non-blocking real-time status indicators (`Synced 🟢`, `Syncing 🟡`, `Error 🔴`).
- **Single-Instance Desktop Guard & Lock Management (`cmd/sqlfox`)**: Automatic single-instance protection on desktop and local environments. Re-launching SqlFox while an instance is already running on the PC detects the active server via state file verification (`sqlfox.lock`) and fast loopback health check, focuses/opens the running instance in the user's default browser, and cleanly exits (code 0) without spawning redundant processes on other ports. Added `-multi` flag to permit concurrent instances when intentionally desired.
- **Multi-Engine Execution Plan Normalization (`internal/advisor`)**: Engine-accurate full-table scan (`Seq Scan`, `SCAN TABLE`) and filesort elimination (`Sort`, `USE TEMP B-TREE`) parsing across PostgreSQL (recursive JSON plan tree) and SQLite (detail pattern matching) in `ParseExplainHints`. Frozen core relational engine scope to MySQL, PostgreSQL, and SQLite (ADR-0045).
- **Notes Bundle Portability Suite (.zip)**: 1-click export and import of all Operational Notes, Table Documentation, and SQL Snippets packaged into a portable, unencrypted ZIP bundle containing human-readable Markdown and SQL files for offline inspection and cross-tool migration.
- **Unified Cloud Sync & Migration Modal**: Modern 3-tab modal (`☁️ GitHub Sync`, `📦 Notes Bundle (.zip)`, `🔐 Vault Archive (.sqlfox-vault)`) accessible from the Top Settings dropdown, Notes Inbox drawer header, and Command Palette (`Ctrl+K`).

### Security
- **Secure-by-Default Ingress Binding & LAN Guard (`cmd/sqlfox`, `internal/server`)**: HTTP web daemon strictly binds to loopback (`127.0.0.1`) by default for zero network exposure on untrusted public Wi-Fi networks. Requires explicit `--lan` flag (or `-host 0.0.0.0`) to enable tablet and multi-device access.
- **High-Entropy Master PIN & Remote Lockout Policy (`internal/server/auth`)**: Upgraded auto-generated PINs to 6-character alphanumeric strings (`fox-[a-z0-9]{6}`, >2.17 billion combinations). Remote Access Guard automatically locks out and quarantines any remote IP after 5 consecutive failed unlock attempts for 10 minutes (`429 Too Many Requests`).
- **DNS Rebinding & Cross-Origin CSRF Guard (`internal/server/auth`)**: Ingress middleware inspects `Host` headers and rejects external hostnames pointing to loopback (`403 Forbidden`). State-mutating API requests reject unauthorized `Origin` and `Sec-Fetch-Site: cross-site` payloads.
- **Fail-Closed Release Manifest Verification (`internal/server/update`)**: In-app self-update strictly restricts downloads to the official repository (`github.com/herliansyah/sqlfox/releases/download/...`) and cryptographically verifies archive SHA-256 digests against release checksum manifests (`checksums.txt` / `SHA256SUMS`).

### Fixed
- **Navbar Monitor & Processlist Layout Shifts**: Replaced variable-width badge on the top monitor button with an absolute-positioned corner status dot to prevent horizontal button jitter. Eliminated vertical layout shift in Processlist Manager by replacing inline push banners above the threads table with an on-demand modal dialog for Lock Contention & Root Blocker Analysis, subtle in-table row highlighting, and transient toast notifications.

### Changed
- **UI Resilience & Non-Wrapping Controls**: Universal CSS and layout guard across all application buttons and toolbar chrome (`button:not(.allow-wrap)`). Enforces `white-space: nowrap !important;` and `flex-shrink: 0;` to eliminate awkward squished button shapes, button multi-line word-wrapping, and overlapping toolbar items when the browser window narrows. High-density toolbars (`header`, `editor-toolbar`, `table-mode-tabs`, `grid-toolbar`, `footer`) now feature smooth horizontal scrolling (`overflow-x-auto gap-3`) with preserved icon and label geometry.
- **Terminal Console PIN Privacy**: Console startup output suppresses Master PIN and Network URLs in default localhost mode, revealing them only when `--lan` mode is activated.
- **CLI Runner Server Flags**: Default host flag updated to `127.0.0.1` and `--lan` added to help documentation.

## [v0.4.0] - 2026-10-08

### Added
- **Process Activity Trend Panel**: Ephemeral in-memory rolling time-series panel in the Processlist Manager rendering active executing query concurrency and total thread counts on native HTML5 Canvas over a 60-point circular buffer. Surfaces real-time operational statistics (`Running Now`, `Total Threads`, `Peak Concurrency`, `60s Avg Running`), collapsible layout with persistent toggle state in `localStorage`, and 1-click deep escalation to the Flight Recorder.
- **Master PIN Shoulder-Surfing Masking & Toggle**: Replaced exposed plain-text Master PIN badge in the application header with a secure masked view (`fox-••••`) and interactive peek toggle button (`👁️`), preventing casual shoulder-surfing observation in open office, coffee shop, or screen-sharing environments.
- **Process Detail Modal Enhancements & HTTP Dialog Clipboard Copy**: Detailed modal inspection for active threads in the Processlist displaying full query text, user, host, database, state, execution time, and lock dependencies. Enhanced clipboard copy fallback supporting plain HTTP network environments where `navigator.clipboard` is restricted by browser security policies.
- **Workspace Pane Splitter**: Draggable horizontal divider separating the Query Editor and Virtual Data Grid, featuring boundary constraints (min 80px editor, min 120px grid), double-click 50:50 reset, persistent layout preference in `localStorage`, and dynamic tab visibility.
- **Workspace Tab Context Menu & Lifecycle Suite**: Added secondary-click context menu on workspace tabs providing granular and batch tab actions: `Close` (`Ctrl+W`), `Close Others`, `Close Tabs to the Right`, `Close All`, `Duplicate Tab` (with SQL draft clone), and `Rename Tab...` via native modal prompt.
- **Tab Close Confirmation Guard**: Interactive pre-closure protection powered by the Native Modal Dialog safeguarding against accidental tab closure and discarding of dirty SQL drafts or active workspaces across single (`✕`, `Ctrl+W`, menu) and batch tab operations. Clean, unmodified tabs close instantaneously without alert fatigue.
- **Empty Workspace Graceful Reset**: Closing all tabs or the final active tab cleanly resets the workspace into a fresh, empty `Query 1.sql` tab rather than freezing or blocking.
- **Unified Operational Notes Inbox & Follow-up Lifecycle**:
  - Centralized operational notebook engine persisted in SQLite vault (`operational_notes` schema) capturing titles, lifecycle status (`inbox`, `resolved`, `archived`), soft-link database object tags (`db://conn/db/table`), categorization tags (`#incident`, etc.), rich markdown developer action notes, and read-only raw execution/AI snapshots.
  - Slide-over Notes Inbox drawer (`#notes-inbox-drawer`) accessible via the header `[📝 Notes]` button, featuring real-time overdue badge counts, search filtering, status tabs, and pinned runbook toggles.
  - Follow-up reminder alarm scheduler with background 30-second evaluation loop, zero-dependency audio chime via Web Audio API (D5-A5 frequency ramp), and non-blocking interactive notification toasts (`Open Note`, `Snooze 10m`, `Done`).
  - Scoped table documentation feed in the Table Viewer (`[📝 Notes & Runbook]`) displaying notes tagged to the active table with pinned runbooks at top.
  - Interactive AI Copilot `@` mention autocomplete in chat textarea (`[@note:<title>]`) dynamically injecting snapshot query telemetry and developer action notes into AI prompt context.
  - Full REST API suite (`/api/notes`, `/api/notes/reminders/due`, `/api/notes/{id}/snooze`, `/api/notes/{id}/resolve`) with backward compatibility bridging for legacy table notes.
- **Query Snapshot Capture & Note Query Extraction**: Seamless two-way bridge between Query Editor tabs and Operational Notes. Operators can instantly capture active queries (or highlighted SQL selections) directly into a Vault Operational Note from the editor toolbar (`📝 Save Note`), tab context menu (`📝 Save as Operational Note...`), or Command Palette (`Ctrl+K`). Conversely, notes can be extracted with 1 click (`⚡ Open in Query`) directly into the Query Editor, with automatic empty tab reuse, safe tab naming, and soft-link database target resolution.

### Fixed
- **Tab Context Menu Event Coordinates**: Resolved unhandled `ReferenceError: y is not defined` when right-clicking on workspace tabs, ensuring the context menu opens reliably at cursor coordinates.

## [v0.3.0] - 2026-10-08

### Added
- **Diagnostic Logging & Observability Suite (`internal/logger`)**: Built-in structured logging pipeline using Go standard library `log/slog` with zero external dependencies. Supports customizable verbosity levels (`debug`, `info`, `warn`, `error`), human-readable `text` and machine-parseable `json` formats, dual console/file streaming (`-log-file`), silent mode (`-silent`, `-q`), HTTP request telemetry middleware with static asset noise filtration, and sensitive credential masking (`/api/auth/unlock`, `/api/connections`).
- **Windows Double-Click Automatic Configuration**: Automatic loader `loadEnvFiles()` reading `sqlfox.env` and `.env` files located in the executable directory, current working directory, and `~/.sqlfox/` for effortless logging and configuration when launching via Windows double-click without a terminal.
- **PostgreSQL Deep Catalog Introspection & Parity**:
  - Routines Explorer for Stored Functions and Procedures introspecting `pg_proc` and `pg_namespace` with overload-safe signatures (`func_name(type1, type2)`) and full DDL retrieval via `pg_get_functiondef`.
  - Adaptive Statement Digest querying `pg_stat_statements` with real-time average execution times, call counts, and a 1-click helper note when the extension is disabled.
  - Replication and Standby Inspector probing `pg_stat_replication` on primary nodes and `pg_stat_wal_receiver` on standbys.
  - User & Role Management Matrix introspecting `pg_roles` attributes (`SUPERUSER`, `CREATEDB`, `CREATEROLE`, `REPLICATION`, `BYPASSRLS`, `LOGIN`) and role hierarchy (`MEMBER OF`).
- **Developer Programmability & Grid Suite**:
  - Routines Explorer for MySQL/MariaDB with 1-click `CALL sp(...)` templates and OUT-variable queries.
  - Scheduled Events controller with status toggles (`ALTER EVENT ENABLE/DISABLE`).
  - Table Partitions telemetry sub-tab in Table Designer detailing partition methods, expressions, and row counts.
  - JSON Cell Viewer modal in virtual grid with pretty-printed formatting and syntax validation.
- **DBA Fire-Fighting & Diagnostic Suite**:
  - Deadlock Inspector parsing `SHOW ENGINE INNODB STATUS` with conflict transaction matrix.
  - Statement Digest Dashboard querying `sys.statement_analysis` with Safety Guard challenge reset action.
  - Database Storage Inspector analyzing table disk allocation, indexes, fragmented free space, and data-to-index ratios.
  - Table Health Advisor recommending fragmentation maintenance (`OPTIMIZE TABLE` / `VACUUM ANALYZE`) and engine optimizations.
  - Server Variable Inspector and Cluster Tuning Advisor evaluating cache hit ratios, buffer pool utilization, connection saturation, and temp table spilling with actionable config recommendations.
  - Real-Time Lock Contention Inspector displaying active lock waits and blocking transactions.
- **Universal Mutation Logging & Audit Source Attribution**: Comprehensive audit ledger persistence for all data and schema mutating operations across the platform with origin tracking (`source`). Attributions include `editor` (Query Editor / SQL Tab), `grid_cell_edit` (inline cell updates), `grid_insert` (inline `+ Add Row`), `grid_delete` (grid row deletions), `script_runner` (large SQL script runner), `csv_import` (CSV import wizard), `mock_data` (synthetic data generator), `sync_transfer` (cross-database data sync), and `schema_sync` (DDL structure synchronization). Added source badge visualization and dropdown source filtering to the Session Audit Log drawer and CSV export.
- **Value Presets & PK-Less Grid Editing**:
  - Full-row criteria matching (`WHERE col1 = ? AND col2 = ? ... LIMIT 1`) enabling inline cell editing and row deletion on tables lacking primary keys, guarded by strict 1-row mutation limits.
  - Multi-driver live schema introspection fallback for dynamic primary key and unique constraint detection across MySQL, PostgreSQL, and SQLite.
  - Quick insert value presets (NULL, Empty string, NOW, Current Date, UUID v4) with floating pill toolbar and hotkeys (`Alt+N`, `Alt+E`, `Alt+T`, `Alt+D`, `Alt+U`).
  - Type-aware cell editors providing native select dropdowns for `ENUM` types, toggle switches for `BOOLEAN`, and searchable foreign key record pickers.
  - Resilient RFC4122 v4 UUID generator fallback for non-secure HTTP / remote LAN contexts.
- **Execution Plan Overhaul, Telemetry Status Bar & Menu Navigation**:
  - Restructured Execution Plan (EXPLAIN) inspector featuring a dedicated Target Query card with SQL syntax formatting, raw toggle, modern segmented tab switcher (`📊 Plan Grid`, `🌳 Visual Tree`, `💡 Advisor`, `{ } Raw Plan`), and rich visual tree rendering with universal fallback across MySQL, PostgreSQL, and SQLite.
  - Overhauled bottom telemetry status bar displaying real-time engine flavor badge (`[MYSQL]`, `[POSTGRES]`, `[SQLITE]`), server uptime, ping round-trip latency, memory telemetry, and author attribution with themed `[GitHub]` and `[☕ Sponsor]` pill buttons.
  - Reorganized top navbar menus into ergonomic grouped dropdowns (`Monitor ▾`, `DB Tools ▾`, `Settings ▾`, `Copilot ▾`).
  - Formatted SQL view with raw toggle in Process Detail Inspector.
  - Responsive container height and scrolling fixes for Flight Recorder correlated queries table.
- **Session Tab State Persistence**: Persistent tab state in `localStorage` scoped per connection (`sqlfox_tabs_<conn_id>`), preserving open Query Editor tabs, active SQL drafts, Table Viewer tabs, and active tab focus across page refreshes, with smart `beforeunload` guard for running queries and staged table edits.
- **Comprehensive Bilingual Documentation**: Added comprehensive bilingual English and Indonesian user and DBA guides with direct in-app links.

### Changed
- **Universal MySQL 8.4 LTS & 9.x Replication Probe**: Dual-syntax probe querying `SHOW REPLICA STATUS` and `SHOW BINARY LOG STATUS` first, with graceful fallback to legacy `SHOW SLAVE STATUS` and `SHOW MASTER STATUS` for MySQL 5.7 / 8.0, normalizing all output column keys (`replica_io_running` <-> `slave_io_running`, `seconds_behind_source` <-> `seconds_behind_master`) across engine versions.
- **MariaDB Lock Wait Fallback**: Added automatic fallback to `information_schema.innodb_lock_waits` when `sys.innodb_lock_waits` is not installed on MariaDB.

### Fixed
- **PostgreSQL Structure Sync Column Alteration DDL**: Decomposed column modifications into dialect-valid atomic `ALTER TABLE tbl ALTER COLUMN col TYPE ...; ALTER TABLE tbl ALTER COLUMN col SET/DROP NOT NULL;` statements instead of MySQL-specific `MODIFY COLUMN` syntax.
- **Virtual Grid & Navigation State**:
  - Prioritized active grid database context during inline cell editing and table operations to prevent cross-database drift.
  - Fixed query reload on query tab edit and schema preservation in cross-database table viewers.
  - Reset row selections and table select-all checkboxes upon switching active tables.
  - Fixed column filtering and clipboard copy fallback over non-secure HTTP / remote LAN contexts.
  - Preserved natural column widths and isolated query tab results to prevent layout clipping.
- **ERD Visualizer**: Resolved missing primary keys, foreign keys, and relationship connector lines.
- **Performance Flight Recorder**: Filtered out background system threads and empty queries from correlated snapshots.
- **Monitoring UI**: Persisted monitoring tabs across page reloads and standardized English terminology.
- Added missing `advise` command routing to top-level CLI dispatch in `cmd/sqlfox/main.go`.

## [v0.2.0] - 2026-10-05

### Added
- **Visual Table Structure Designer**: Visual schema designer with native dialogs, indexed column configurators, and cross-database autocomplete.
- **Visual Query Plan Tree & Grid**: Graphical explain plan inspector with bottleneck badges, cost breakdowns, and offline schema introspection tooling.
- **Pure Go PostgreSQL Native Driver**: High-performance PostgreSQL connector with native catalog introspection without external cgo dependencies.
- **Spreadsheet CRUD & Inline Editing**: Data grid inline cell editing with staged apply mode, revert capabilities, and transaction safety.
- **Real-Time Processlist Manager**: Live running query diagnostics, process killer, redundant index detector, table diagnostics, privilege matrix, and session audit logs.
- **Inline Table Filter Bar**: Quick filter bar with server-side pushdown and seamless handoff to full SQL queries.
- **Safe DDL Guardrails**: Pre-flight lock contention inspection, blocking query detectors, and migration safety checks.
- **Cluster & Replication Inspector**: Dynamic cluster topology monitor with replication lag telemetry and bottleneck identification.
- **Performance Flight Recorder**: On-demand flight recorder capturing correlated process snapshots and metric histories.
- **Interactive Visual ERD Canvas**: Draggable interactive entity-relationship diagram with foreign key connectors and schema exports.
- **FK-Aware Mock Data Generator**: Smart test data generation respecting referential integrity and foreign key closures.
- **Heuristic Index Advisor**: Automated index recommendations, index bloat guards, and safe DDL migration generation.
- **Headless CLI Suite**: Automated query execution runner, cross-connection data transfer, and declarative cron task runner.
- **Streaming SQL Dumps & Row Sync**: Fast streaming schema/data export, referential slice seeding, and cross-connection synchronization.
- **Canonical Brand Mark & App Icon Set**: Standardized cross-platform visual identity featuring geometric low-poly vector Fox Brand Mark (`docs/assets/logo.svg`, `web/dist/assets/logo.svg`), multi-resolution Windows ICO (`favicon.ico`, `icon.ico`), Apple Touch Icon (`apple-touch-icon.png`), and standalone PWA manifest (`manifest.webmanifest`).
- **Native Windows PE Icon Embedding**: Pre-compiled multi-arch COFF resource objects (`rsrc_windows_amd64.syso`, `rsrc_windows_arm64.syso`) embedding the Brand Mark into `sqlfox.exe` during standard Go compilation.
- **In-App Titlebar & Modal SVG Branding**: Inline crisp SVG Brand Mark in navbar header, native confirmation dialogs, and remote lock screen, replacing OS-dependent emoji placeholders.
- **In-App Self-Update Engine**: Native zero-dependency self-update mechanism with platform-specific binary replacement and graceful restart.
- **BYOK AI Copilot Enhancements**: Chat session forking/branching, rich Markdown rendering with 1-click execution blocks (`Insert & Run`, `Replace`), and fluid responsive drawer layout.
- **Author Attribution & Support**: Integrated creator credits and Saweria sponsorship link in status bar footer.

### Changed
- Streamlined router and middleware using Go standard libraries, pruning Chi and WebSocket dependencies.
- Unified environment status badge as single source of truth across UI surfaces.
- Hardened memory ceilings and high-concurrency benchmarks for low-footprint operation (<100MB RAM).

### Fixed
- Handled Windows executable locking during in-app self-update via rename staging.
- Synchronized update restart hooks to eliminate timing races on Windows CI runners.
- Fixed Windows path resolution in declarative job specification JSON loading.
- Fixed invalid secrets context in release workflow npm publish conditional.

---

## [v0.1.0] - 2026-10-05

### Added
- Initial release of SqlFox, the lightweight, zero-dependency database manager.
- Zero-cgo embedded database drivers for MySQL/MariaDB and SQLite.
- Multi-IP network binding with sequential linear port hunting on address collision.
- Remote Access Guard requiring Master PIN authentication for non-loopback network connections.
- Localized CodeMirror SQL editor with fast contextual schema autocomplete.
- In-memory Connection Vault with AES-GCM credential persistence and portable encrypted archives.
- AI SQL Assistant supporting chat sessions, dynamic schema column injection, and automatic error healing.
- Multi-OS cross-compilation pipeline supporting Linux, macOS, and Windows (amd64 and arm64).
