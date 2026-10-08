# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
