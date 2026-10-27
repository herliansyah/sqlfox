# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Virtual Data Grid & Fast Schema Index (`web/dist/index.html`, `internal/server`)**: Implemented fixed-height DOM row windowing (32px uniform height) with `< 500` active DOM nodes, full keyboard arrow navigation, persistent cell inspector drawer for multiline strings and JSON payloads, and an in-memory prefix index delivering sub-5ms schema explorer keyword filtering (ADR 0058).
- **NDJSON Micro-Chunk Query Streaming Endpoint (`internal/server`)**: Added `/api/session/{id}/execute-stream` delivering line-by-line streaming with immediate 20-row micro-chunk flush for low Time to First Row (TTFR < 50ms), native `AbortController` cancellation tracking, and MySQL profiler support (ADR 0058).
- **PostgreSQL View DDL Introspection & Materialized Views (`internal/driver/postgres`)**: Rebuilt view DDL generation via `pg_get_viewdef` supporting both standard `CREATE VIEW` and `CREATE MATERIALIZED VIEW` definitions (ADR 0059).
- **PostgreSQL Composite Foreign Key & Referential Action Extraction (`internal/driver/postgres`)**: Introspect composite foreign keys using `unnest(con.conkey, con.confkey) WITH ORDINALITY`, capturing all participating column pairs and mapping `confdeltype`/`confupdtype` to standard `ON DELETE` and `ON UPDATE` actions (`CASCADE`, `SET NULL`, `SET DEFAULT`, `RESTRICT`, `NO ACTION`) (ADR 0059).

### Security
- **Client SSL Private Key Encryption at Rest (`internal/vault`)**: Encrypted client SSL private keys (`SSLKey`) in the Connection Vault with AES-256-GCM using versioned envelope markers (`enc:v1:`), preventing plaintext exposure on disk while preserving backward compatibility with legacy unsealed records (ADR 0059).
- **Strict Same-Origin Ingress Guard (`internal/server/auth`)**: Hardened `IngressGuardMiddleware` CSRF validation to require exact same-origin matching (`Origin` host matching request `Host`), blocking cross-port state-mutating requests from other local web servers (e.g., `localhost:3000` targeting `localhost:8080`) (ADR 0059).
- **MySQL Executable Comment Unwrapping in AST Safety Guard (`internal/driver`, `internal/safety`)**: Updated `StripLeadingComments` to unwrap MySQL executable comments (`/*! ... */` and `/*!NNNNN ... */`) into raw executable SQL instead of stripping them, closing an evasion vector and ensuring destructive operations wrapped in comments trigger mandatory confirmation challenges (ADR 0059).
- **Fail-Closed Processlist Batch Termination (`internal/server/handlers_dba`)**: Enforced a fail-closed policy in `handleBatchKillProcess` returning HTTP 503 if the processlist cannot be retrieved, preventing process termination when immune threads (self connection and system threads) cannot be verified (ADR 0059).
- **Release Update Asset Verification Hardening (`internal/server/update`)**: Enforced explicit non-empty asset name validation and strict exact filename matching during SHA-256 digest checks against official release manifests (ADR 0059).

### Fixed
- **Thread-Safe Active Database and Schema Scoping (`internal/driver/mysql`, `internal/driver/postgres`)**: Encapsulated session active database/schema state within mutex-protected accessors (`activeDB()`, `activeSchema()`), preventing data races during concurrent query execution and runtime database switches (ADR 0059).
- **MySQL Custom TLS Failure Handling (`internal/driver/mysql`)**: Aborted connection setup with an explicit error when custom TLS configuration registration fails, preventing silent downgrades to insecure `skip-verify` (ADR 0059).
- **MySQL Multi-Column View DDL Scan (`internal/driver/mysql`)**: Handled 4-column output (`View`, `Create View`, `character_set_client`, `collation_connection`) from `SHOW CREATE TABLE` on MySQL views without triggering column scan errors (ADR 0059).
- **Transaction-Scoped FK Checks in Mock Generator (`internal/mock`)**: Moved MySQL `SET FOREIGN_KEY_CHECKS=0` and `=1` inside the pinned transaction connection (`tx.ExecRaw`), ensuring foreign key constraints are bypassed strictly on the connection performing bulk insertions (ADR 0059).
- **Stateless Pre-Execution Mutation Estimation (`internal/server/session_manager`)**: Eliminated `SelectDatabase` calls during pre-flight row count probes, preventing accidental active session database switching prior to user confirmation (ADR 0059).
- **SQL Dump & Smart Seed View Dependency Ordering (`internal/server/handlers_dump`)**: Sorted views after base tables (`viewsLast`) and generated object-appropriate `DROP VIEW IF EXISTS` / `DROP MATERIALIZED VIEW IF EXISTS` statements, preventing restore failures caused by missing table references or view type mismatches (ADR 0059).
- **Query Mode Toolbar Cleanup & Edit Scoping (`web/dist/index.html`)**: Hid table mutation buttons (Add Row, Mock Data, Import CSV, Edit Mode, and Table SQL Dump) in query mode, restricting query-mode inline cell edits to single-table queries containing a Primary Key.
- **Data Grid Total Row Count Dialect Quoting & Endpoint (`web/dist/index.html`)**: Added dialect-aware identifier quoting (backticks for MySQL/MariaDB, double quotes for PostgreSQL/SQLite) in total count calculations and routed requests through `/api/session/{id}/execute` (ADR 0059).
- **Foreign Key Value Picker Datalist Route (`web/dist/index.html`)**: Corrected FK datalist row lookup endpoint path to `/api/session/{id}/table/{table}/rows` with HTTP status verification (ADR 0059).

## [v0.10.11] - 2026-10-26

### Fixed
- **Grid Pagination Initial Visibility & Toolbar Cleanup (`web/dist/index.html`)**: Hid `#grid-pagination-bar` by default on cold start and during empty/reset states until a query or table view produces data. Completely eliminated redundant legacy top pagination controls from the toolbar in favor of the unified sticky bottom bar.
- **Pagination UI Localization (`web/dist/index.html`)**: Standardized all bottom pagination bar button labels, tooltips, and dynamic status texts to English (e.g. "Showing X - Y of Z rows", "Page N of M", and "Calculate total row count").


## [v0.10.10] - 2026-10-26

### Added
- **Unified Bottom Pagination Bar with Page Jump & Sizing (`web/dist/index.html`)**: Introduced a sticky bottom pagination bar below the data grid for both SQL Query Editor results and Table Data Viewer. Supports first/previous/next/last page controls (`⏮️`, `◀`, `▶`, `⏭️`), direct page jump input, configurable page sizes (25, 50, 100, 200 rows per page), and on-demand accurate total count calculation for filtered tables.
- **Smart Wildcard Auto-Wrapping for `LIKE` / `NOT LIKE` Filters (`web/dist/index.html`)**: Automatically wraps plain search terms with `%...%` wildcards when using `LIKE` and `NOT LIKE` filter operators if no wildcards (`%` or `_`) are explicitly entered. Preserves explicit wildcards as-is and synchronizes compiled expressions when switching between visual filter builder, raw WHERE mode, and exporting to SQL query tabs.


## [v0.10.9] - 2026-10-26

### Fixed
- **Query Grid Row Windowing & Pagination on Large Result Sets (`web/dist/index.html`)**: Added client-side row windowing (50 rows per page) and keyset pagination controls to the SQL Query Editor grid. Eliminates main-thread freeze, memory bloat (HTML payload reduced by 95%), and sluggish scrolling when rendering large result sets (~1,000 rows with many columns), while preserving all in-memory rows for complete CSV and JSON exports.

## [v0.10.8] - 2026-10-26

### Added
- **Pre-Execution Mutation Row Estimation & Tiered Safety Guard (`internal/safety`, `internal/server`)**: Integrated pre-flight DML inspection that rewrites `UPDATE` and `DELETE` queries into bounded `SELECT COUNT(*)` probes with strict 500ms timeouts to estimate row impact before execution. Features adaptive tiered affirmation: silent execution for single-row primary-key mutations in Development, streamlined 1-click affirmation for 1–100 rows, and automatic escalation to full typing verification for mutations exceeding 100 rows or lacking a `WHERE` clause. Multi-statement batches aggregate row impacts with compound escalation (ADR 0057).
- **Environment-Enforced Mutation Safety Policies (`internal/safety`, `internal/server`)**: Enforces mandatory confirmation challenges across all mutation operations in Staging and Production environments, regardless of affected row counts.

### Fixed
- **Fail-Fast Fetch Timeout for Metadata Requests (`web/dist/index.html`)**: Added default 15-second timeout via `AbortSignal.timeout(15000)` in `apiFetch`, preventing UI freezes and indefinite hangs on dead or half-open TCP sockets.
- **Idle Session Auto-Wakeup & Tab Focus Re-Ping (`web/dist/index.html`)**: Implemented automatic background session wake-up on `visibilitychange` and window `focus` after >5 minutes of idle time or system sleep, seamlessly re-validating database connectivity and refreshing state without manual page reloads.
- **Processlist Fetch Loading State Indicator (`web/dist/index.html`)**: Added visual pulsing loading indicator to the Processlist table header during in-flight data fetching.


## [v0.10.7] - 2026-10-22

### Added
- **Visual Transaction Controls & Toolbar Status (`web/dist/index.html`, `internal/server`)**: Integrated visual active transaction pill (`[⚡ TX Active]`) into query editor toolbar displaying live transaction state (`tx_active`), paired with direct `Commit` and `Rollback` buttons and global keyboard shortcuts (`Ctrl+Shift+C` and `Ctrl+Shift+R`) (ADR 0057, Roadmap 1.2).
- **Processlist Dual Kill Controls & State Sync (`web/dist/index.html`, `internal/server`)**: Added dedicated `Kill C` (terminate connection) action alongside `Kill Q` (cancel query) for active processes in Processlist table, complete with deferred auto-refresh (600ms) and structured audit logging.

### Fixed
- **Uncommitted Transaction Tab Close Guard (`web/dist/index.html`)**: Added confirmation warning dialog preventing accidental query tab closure while an uncommitted database transaction remains active, executing clean rollback on confirmed tab exit.
- **Vault Audit Log Auto-Retention & Disk Ceiling (`internal/vault`)**: Enforced a 50,000-record ceiling and automatic 30-day retention purge on vault open, eliminating unbounded audit log disk storage growth.
- **Cross-Engine Transfer DDL & Type Normalization (`internal/driver`)**: Normalized column types and stripped dialect-specific clauses (e.g. `UNSIGNED`, engine-specific collations, `ON UPDATE CURRENT_TIMESTAMP`) during cross-database data transfers and structure sync across MySQL, PostgreSQL, and SQLite.
- **Out-of-Band Processlist Kill Connection (`internal/driver/mysql`, `internal/driver/postgres`)**: Dialed dedicated control connections in MySQL and PostgreSQL `KillProcess` to terminate hung threads even when the connection pool is fully saturated.
- **PostgreSQL Kill Result Verification (`internal/driver/postgres`)**: Verified boolean return values of `pg_terminate_backend` and `pg_cancel_backend` to detect and propagate process termination failures.
- **Extended Transaction Connection Pinning (`internal/server`)**: Added connection pinning support for `BEGIN READ ONLY`, `BEGIN IMMEDIATE`, and `BEGIN EXCLUSIVE` transaction statements.
- **MySQL EXPLAIN Dedicated Connection Context (`internal/driver/mysql`)**: Applied active database context to dedicated explain connections, preventing "no database selected" errors.
- **Multipart Form Temporary File Leaks (`internal/server`)**: Guaranteed cleanup of uploaded multipart form temporary files during data import, dump restoration, and cloud notes bundle sync.
- **DefaultBroadcaster Data Race (`internal/logger`)**: Resolved data race in log broadcaster initialization via double-checked mutex locking.
- **Grid JSON Export Object Formatting (`web/dist/index.html`)**: Formatted exported JSON data as an array of key-value objects keyed by column names instead of raw 2D arrays.
- **Cluster Tab Polling Timer Cleanup (`web/dist/index.html`)**: Cleared background cluster polling timer when closing Galera Cluster tabs.
- **HTTP Server Persistent Stream & Large Dump Write Timeout (`cmd/sqlfox/main.go`)**: Disabled global `WriteTimeout` on daemon `http.Server` (retaining `ReadHeaderTimeout: 10s` and `IdleTimeout: 120s`), eliminating 60-second connection drops on SSE telemetry streams (`/api/logs/stream`) and large SQL database dumps.
- **Safety Guard CTE Comment & Escape Bypass (`internal/safety/guard.go`, `internal/safety/guard_test.go`)**: Enhanced `stripLeadingCTE` to properly ignore single-line comments (`--`, `#`), multi-line comments (`/* ... */`), and escaped quotes (`\'`, `\"`) within CTE definitions, preventing destructive `DELETE` and `UPDATE` statements from bypassing Safety Guard evaluation. Added `stripLeadingCTE` fallback to lexical inspection.
- **MySQL Multi-Statement Execution in Query Editor (`internal/driver/mysql/mysql.go`)**: Appended `&multiStatements=true` to the MySQL driver DSN, enabling execution of multi-statement batches without MySQL Error 1064 syntax failures.
- **Google Gemini API Key Special Character URL Encoding (`internal/ai/client.go`)**: Added `url.QueryEscape` to Gemini completion request URL formation, preventing API key corruption when keys contain characters like `+`, `/`, `&`, or `=`.
- **IPv6 Host Literal Parsing (`internal/driver/mysql/mysql.go`, `internal/driver/postgres/postgres.go`)**: Replaced raw string concatenation with standard `net.JoinHostPort` for MySQL and PostgreSQL dialers, ensuring IPv6 addresses (e.g. `::1`) are properly bracketed (`[::1]:port`).

### Security
- **DNS Rebinding Test Bypass Restriction (`internal/server/auth.go`)**: Restricted Host header validation bypass strictly to unit test environments via `testing.Testing()`.
- **Connection Save Plaintext Password Sanitization (`internal/server/handlers_connections.go`)**: Sanitized `conn.Password = ""` prior to serializing the JSON response of `handleSaveConnection`, preventing plaintext database passwords from being echoed back in HTTP response bodies.
- **Auth Manager In-Memory Token and Lockout Expiry Purge (`internal/server/auth.go`)**: Added `purgeExpiredLocked()` to automatically sweep expired authentication tokens and expired IP lockout records during token generation, validation, and failed login tracking.
- **MySQL Driver TLS Configuration Cleanup (`internal/driver/mysql/mysql.go`)**: Stored `tlsKey` on `MySQLSession` and registered `gomysql.DeregisterTLSConfig(s.tlsKey)` upon session closure (`Close()`), eliminating monotonic memory growth in the driver's global TLS config map.

### Documentation
- **ADR 0020 SSH Documentation Alignment (`README.md`, `ARCHITECTURE.md`, `SECURITY.md`, `docs/USER_GUIDE.md`, `docs/USER_GUIDE_ID.md`)**: Synchronized user guides, architecture specifications, and security threat models to reflect native platform SSH forwarding (`ssh -N -L`) and SSL/TLS certificate vault storage in accordance with ADR 0020.
- **SqlFox Future Roadmap & Architectural Backlog (`docs/FUTURE_ROADMAP.md`)**: Documented comprehensive future backlog for missing features (in-process bastion manager, visual transaction controls, database object designers, Excel export, localStorage tab persistence) and bloat evaluations (GitHub notes sync, PXC cluster driver weight, flight recorder APM scope, frontend single-file SPA modularization).

## [v0.10.6] - 2026-10-21

### Fixed
- **JavaScript Syntax Error in `selectConnection` Startup Routine (`web/dist/index.html`)**: Removed duplicate unclosed `try` block in `selectConnection` that caused browser parsing failures and prevented the web interface from loading on startup.
- **Embedded Web JavaScript Syntax Test Guard (`web/embed_test.go`)**: Added automated syntax verification test (`TestEmbeddedIndexHTMLJavaScriptSyntax`) to parse and validate embedded HTML script blocks during test runs.


## [v0.10.5] - 2026-10-21

### Added
- **Web Busy State Guards, Timers & Progress Indicators (`web/dist/index.html`)**: Added concurrency locks and execution guards for EXPLAIN, DDL review, and connection tests to prevent duplicate submissions and race conditions. Added live elapsed stopwatch timer, indeterminate progress bar, and timeout safety guards (30s for EXPLAIN, 15s for connection tests) preventing indefinite UI freezes.
- **DDL/DML Execution Success Banner (`web/dist/index.html`)**: Dedicated success banner in the query results panel for non-query statements (ALTER, CREATE, UPDATE, DELETE), cleanly clearing previous errors and preserving execution state across tab switching.

### Fixed
- **Advisor MySQL Online DDL Syntax & Version Awareness (`internal/advisor`, `internal/server`)**: Fixed MySQL Error 1064 syntax error by ensuring correct comma placement before `ALGORITHM` in ALTER TABLE statements. Added database version awareness to safely fall back to standard DDL on legacy MySQL (< 5.6) and MariaDB (< 10.0), enforced length prefix limits on `TEXT`/`BLOB` indexed columns, and connected advisor copy buttons to the toast notification system.
- **Query Editor Routing for EXPLAIN & Advisor DDL (`web/dist/index.html`)**: Routed `openImpactQueryInEditor` and `applyAdvisorDDLToEditor` through the canonical `setEditorSQL` helper, automatically opening and focusing a new query tab when triggered from non-editor views (e.g. Processlist or Flight Recorder).


## [v0.10.4] - 2026-10-20

### Added
- **Floating ENUM / SET Value Builder (`web/dist/index.html`)**: Added a floating modal popover for defining and managing `ENUM` and `SET` column options with interactive tag chips, quick-add input (Enter or comma delimiters), smart paste parsing (auto-sanitizing quotes/spaces), real-time syntax checking (duplicate and empty option warnings), live SQL preview, and dynamic injection into column Default Value presets (ADR 0056).
- **Composite Index Builder (`web/dist/index.html`)**: Introduced an interactive visual column ordering picker with order badges (#1, #2...), available table column chips, index type selection (`INDEX`, `UNIQUE`, `FULLTEXT`, `SPATIAL`), auto-naming (`autoGenerateIndexName`), and staged in-place edit workflow (`editTableIndex`).
- **Cascading Foreign Key Selector & Type Guard (`web/dist/index.html`)**: Integrated active database catalog awareness (`multiDBCatalogCache`), dynamic referenced column dropdown with Primary Key auto-selection, auto-naming, and real-time DataType Compatibility verification to prevent MySQL errno 150 errors prior to DDL execution.
- **Table Structure Pre-Flight Validation Engine (`web/dist/index.html`)**: Added real-time validation for duplicate column names, empty names, missing ENUM options, and invalid lengths, surfacing visual error badges and disabling DDL execution until resolved.

### Fixed
- **Grid Export Dropdown Toggle & Outside-Click Collision (`web/dist/index.html`)**: Added missing button identifier (`btn-export-csv`) and included `#export-csv-dropdown-container` in outside-click detection, preventing inadvertent menu dismissals and ensuring active context menus close cleanly when opening export options.
- **Table Designer & Creator Focus Retention (`web/dist/index.html`)**: Eliminated full-table DOM re-rendering (`innerHTML` destruction) on column attribute input, preserving active cursor focus and caret position during continuous typing.

## [v0.10.3] - 2026-10-19

### Added
- **Tab Database Scope & Target Database Switcher (`web/dist/index.html`, `internal/server`)**: Bound database schema context directly to individual query tabs, decoupling tab execution scope from sidebar selection. Added an interactive Target Database Pill in the query editor toolbar with search filtering, keyboard navigation, and quick switching across available databases (ADR 0055).
- **Schema Scope Mismatch Guard (`internal/safety`, `web/dist/index.html`)**: Enhanced SQL safety guardrails to detect cross-database execution conflicts when statement target schemas differ from the active tab's bound database scope, surfacing an explicit safety warning prior to execution.
- **Tiered DDL Target Database Affirmation (`internal/safety`, `web/dist/index.html`)**: Introduced environment-aware DDL execution safety: streamlined one-click target confirmation in Development environments while enforcing full typing verification challenges in Production and Staging.
- **Bidirectional `USE <database>` Synchronization (`web/dist/index.html`)**: Automatically syncs tab database scope and UI indicators when running manual `USE <database>` statements in SQL queries, surfacing instant feedback via toast notification.

### Fixed
- **Windows Audit Log Test Stability (`internal/server/dba_test.go`)**: Filtered audit log query by `source=editor` to eliminate order-dependent race assertions across asynchronous audit writes on Windows runners.

## [v0.10.2] - 2026-10-19

### Added
- **Global Settings Shortcut (`web/dist/index.html`)**: Added `Ctrl+,` (and `Cmd+,` on macOS) global keyboard shortcut to open the App Settings modal instantly, aligned with standard desktop developer tooling.

### Changed
- **Unified App Settings Modal & Navbar Simplification (`web/dist/index.html`)**: Replaced top navbar Settings dropdown with a single direct `⚙️ Settings` button that opens a consolidated 4-tab modal (`🔐 Connections`, `☁️ GitHub Sync`, `📦 Notes Bundle`, `🤖 AI Copilot`). Merged standalone AI Copilot settings into the unified modal with seamless tab delegation across all existing UI entry points (Notes Drawer, Copilot panel, Command Palette `Ctrl+K`).

### Fixed
- **Query Execution Concurrency & Keydown Guard (`web/dist/index.html`)**: Added an `isQueryExecuting` guard to `runActiveQuery` and disabled the Run button during execution to prevent duplicate simultaneous queries and race conditions. Guarded global window `keydown` handler when CodeMirror is focused to prevent duplicate execution dispatch on `Ctrl+Enter`, and made keyboard shortcuts registration idempotent.
- **Footer Connection Latency Display (`web/dist/index.html`)**: Stopped passing query execution duration (`execution_ms`) to `updateFooterStats`, preventing SQL runtimes from overwriting network ping latency. Sub-millisecond ping response is now cleanly formatted as `<1ms` instead of `0ms`.

## [v0.10.1] - 2026-10-19

### Fixed
- **Self-Update & Toast Feedback System (`web/dist/index.html`)**: Implemented global `showToast(message, type)` notification system supporting dialog top-layers and multi-severity styling (`info`, `success`, `warning`, `error`), resolving `ReferenceError: showToast is not defined` across 34 call sites.
- **In-App Update Check Button Responsiveness (`web/dist/index.html`)**: Added immediate loading spinner and in-modal feedback to the `Check for Updates` button in the Changelog dialog.
- **Version Alignment & Release Automation (`scripts/release.sh`, `web/dist/index.html`)**: Updated hardcoded legacy footer version to current release (`v0.10.0`) and extended `scripts/release.sh` to automatically synchronize version strings across `web/dist/index.html` on every release.
- **Windows Startup Banner Test Stability (`cmd/sqlfox`)**: Prevent process hang on Windows test runners by providing an empty stdin buffer.

## [v0.10.0] - 2026-10-19

### Added
- **Processlist Manager — Batch Kill Operations & Safety Guardrails (`internal/server`, `web/dist/index.html`)**: Multi-select batch query and connection termination (`POST /api/session/{id}/batch-kill`) with immune thread guardrails protecting caller connection and engine background workers, complete with structured audit logging (ADR 0052).
- **Processlist Manager — Two-Tier Responsive Toolbar (`web/dist/index.html`)**: Split toolbar into primary navigation (refresh, auto-refresh interval, batch actions) and contextual filter subtoolbar (state, command, user, database, minimum elapsed time) to eliminate horizontal crowding on standard displays (ADR 0053).
- **Processlist Manager — Column Visibility Picker (`web/dist/index.html`)**: Interactive column toggle dropdown with `Show All` and `Reset to Default` actions, protecting core identifiers (`ID`, `User`, `Time`, `Info`) from accidental concealment, persisted in `localStorage` across sessions (ADR 0054).
- **Virtual Grid Ergonomics Suite — Right-Click Quick-Filters (`web/dist/index.html`)**: Instant contextual filtering directly from any grid cell via right-click (TablePlus ergonomics). Supports `= 'val'`, `!= 'val'`, `> 'val'`, `< 'val'`, `LIKE '%val%'`, and `IS NULL` / `IS NOT NULL`. Automatically resolves dialect-specific quoting (single-quotes, double-escaped apostrophes), merges filters into the Table Viewer filter clause, and refreshes the result set instantaneously.
- **Virtual Grid Ergonomics Suite — Column Visibility Picker (`web/dist/index.html`)**: Interactive column toggle dropdown (`Columns (N/Total)`) in the Table Viewer toolbar. Supports instant search/filtering across column names, `Show All` / `Hide All` one-click actions, sticky Primary Key protection (`🔒 PKs cannot be hidden`) to prevent breaks in row mutations and keyset pagination, and tab-level `sessionStorage` persistence preserving user column layouts during tab switching.
- **Virtual Grid Ergonomics Suite — Vertical Record Inspector / Single Record Form View (`web/dist/index.html`)**: Modal dialog (`Spacebar` hotkey or right-click &rarr; `📋 Inspect Record (Form View)`) transforming horizontal row data into a vertical, searchable two-column card view (DBeaver ergonomics). Ideal for wide tables (50+ columns) and lengthy text/JSON payloads. Features two-column responsive grid layout, live record pagination (`Previous` / `Next` or `ArrowLeft` / `ArrowRight`), column search filter, 1-click JSON formatting and syntax copy (`Copy JSON`, `Copy Text`), and full inline editing integrated seamlessly with the global `stagedEdits` change tracker and save bar.

### Fixed
- **Copilot Drawer Responsive Overlay & Boundary Clamping (`web/dist/index.html`)**: Automatically switch the Copilot sidebar into floating overlay mode (`absolute right-0 top-0 bottom-0 z-30`) on initial load and window resize when viewport width is $\le 1024\text{px}$ or when available workspace is $<500\text{px}$, preventing horizontal clipping and ensuring the close button (`✕`) is always accessible on tablet and laptop screens. Compacted header buttons with responsive labels to prevent header overflow.
- **Header Dropdown Stacking Context Over Grid Controls (`web/dist/index.html`)**: Explicitly set `position: relative !important; z-index: 40 !important;` on `<header>`, ensuring top dropdown menus (Monitor, DB Tools, Settings) always stack on top of the virtual grid and column picker dropdown. Automatically dismiss open grid dropdowns when opening header menus or clicking outside.
- **Windows Banner Test Binary Compatibility (`cmd/sqlfox`)**: Added `.exe` suffix targeting Windows test environments in CLI startup banner tests.

## [v0.9.1] - 2026-10-15

### Added
- **Terminal Startup Banner & CLI Hero Header (`internal/cli`, `cmd/sqlfox`)**: Interactive startup banner styled as a heavy devtool box (K9s/Lazygit aesthetic) displaying local/LAN endpoints, Master PIN, vault location, and supported database engines. Added compact ANSI hero header for `sqlfox help` with strict TTY, `NO_COLOR`, and `TERM=dumb` guards to keep redirection and Unix pipes clean.
- **Multi-Statement Compound Safety Gate (`internal/server`, `internal/safety`)**: Decompose and inspect all statements within multi-statement SQL executions. Reject compound inputs containing multiple destructive commands that require confirmation challenges, preventing trailing destructive command smuggling (e.g. `SELECT 1; DROP TABLE victim`) and ensuring read-only session enforcement across all sub-statements.
- **Dialect-Specific Single-Row Targeting for PK-Less Mutations (`internal/server`)**: Enforce physical single-row targeting (`singleRowWhere`) for cell edits and row deletions on tables without a primary key. Uses `LIMIT 1` for MySQL, `rowid IN (SELECT rowid ... LIMIT 1)` for SQLite, and `(tableoid, ctid) IN (SELECT tableoid, ctid ... LIMIT 1)` for PostgreSQL (partition-safe via `tableoid`), guaranteeing that mutating or deleting one duplicate row never touches identical sister rows.
- **Composite Primary Key Data Sync (`internal/server`)**: Full composite primary key support for row-level data comparison and application. Serializes composite keys with an unambiguous unit separator (`\x1f`), synthesizes multi-clause `WHERE` criteria, and maps source columns to target columns (`dataIdx`) to prevent omitted columns from being overwritten with `NULL`.
- **Dialect-Aware Value Formatting & Literal Serialization (`internal/driver`)**: Standardize cross-engine literal generation (`FormatSQLValueForDriver`, `hexLiteral`) supporting engine-native hex formats (`X'...'` for MySQL/SQLite, `'\x...'::bytea` for PostgreSQL), string escaping (`EscapeSQLString`), and custom delimiter script splitting (`SplitWithDelimiters`) with PostgreSQL dollar-quoting (`$$`).
- **Comprehensive Mutation Safety Regression Suite (`internal/server/mutation_safety_test.go`)**: Added automated regression checks covering PK-less duplicate-row isolation, multi-statement DROP injection protection, truncate-transfer rollback recovery, and row-level sync table size caps.

### Changed
- **Opt-In Console Logging & Startup Log Placement (`cmd/sqlfox`)**: Made terminal console logging opt-in (enabled only via `-v`, `-verbose`, `-log-level`, or `SQLFOX_LOG_LEVEL`) to keep the startup banner clean, while continuing full event streaming to the in-app Activity Console. Startup logs are placed below the banner when logging is active.
- **Data Sync Memory Safety Ceiling (`internal/server`)**: Enforce an upper bound (10,000 rows) on row-level data sync comparisons, responding with HTTP 422 Unprocessable Entity to protect against daemon memory exhaustion on oversized tables.
- **Canonical Sync Value Normalization (`internal/server`)**: Standardized row comparison hashing (`syncValueKey`) across driver types (`int64`, string representations, byte slices) while strictly distinguishing `NULL` (`\x00NULL`) from empty strings or literal strings.

### Fixed
- **Safety Guard Lexical Comment & Literal Bypass (`internal/safety`)**: Preprocess queries by masking line comments (`--`, `#`), block comments (`/* ... */`), and quoted literals (`maskCommentsAndLiterals`) before running regex-based fallback checks, preventing comments from concealing destructive queries or triggering false positives.
- **Common Table Expression (CTE) Inspection Bypass (`internal/safety`)**: Strip leading `WITH ... AS (...)` CTE clauses (`stripLeadingCTE`) to inspect the underlying root executable operation (`SELECT`, `INSERT`, `UPDATE`, `DELETE`, `MERGE`) when Vitess parser AST parsing fails.
- **Data Transfer Truncate Rollback Protection (`internal/server`)**: Ensure target table rows are preserved inside an atomic transaction when a cross-connection data transfer with `truncate_target: true` encounters batch insertion failures.
- **Explicit Database Selection Error Handling (`internal/server`)**: Verify and handle errors from `UseDatabase` / `SelectDatabase` across CRUD, sync, dump, and query handlers rather than ignoring return values.

## [v0.9.0] - 2026-10-14

### Added
- **In-App Activity Console & Live Event Stream (`internal/logger`, `internal/server`, `web`)**: Real-time operational observability console directly inside the SqlFox interface. Emits runtime application telemetry over Server-Sent Events (SSE: `GET /api/logs/stream`) backed by a 100-entry in-memory ring buffer for instant cold-start history. Features a collapsible bottom-docked drawer with a 5-column running table (Time, Level, Module, Message, Details), smart auto-scroll with freeze detection on upward scroll, level filtering (ALL, ERROR, WARN, INFO, DEBUG), instant search, client-side log export (`.log` plain text or `.json`), and a footer status button (`⚡ Activity`) with live pulsing indicator and unread error badge (ADR 0051).
- **Driver & Query Execution Telemetry (`internal/server`)**: Integrated structured `slog` telemetry into query execution and audit logging (`RecordAudit`), broadcasting executed SQL statements, latencies (`duration_ms`), affected rows, and error details in real-time.

### Changed
- **Telemetry Rendering Throttling & Polling Noise Filtering (`internal/server`, `web`)**: Filtered high-frequency background polling requests (`/notes/reminders/due`, `/flight-recorder/status`, `/processlist`) with 200 OK responses from HTTP telemetry middleware to prevent log flooding. Throttled Activity Console DOM updates with `requestAnimationFrame` and bypassed rendering operations when the drawer is closed to reduce V8 GC pressure and maintain a stable memory footprint.

### Fixed
- **Dual-Stack Loopback Port Probe (`internal/server`)**: Actively probe IPv4 (`127.0.0.1`) and IPv6 (`[::1]`) loopback interfaces via `net.DialTimeout` before verifying TCP binding during port auto-scanning, preventing false-positive port availability on Windows Winsock and dual-stack systems when another service occupies wildcard `:port` or `[::1]:port`.

### Removed
- **Status Bar Footer Personal Branding & GitHub Pill (`web`)**: Removed `Created by herliansyah` and the external GitHub pill button from the persistent bottom status bar footer to provide an uncluttered, distraction-free enterprise workspace interface. Relocated author credit and repository link elegantly into the Changelog / About dialog footer.
- **Donation & Sponsorship References**: Removed all forms of donations, including the Saweria sponsorship link and pill button in the status bar footer, `.github/FUNDING.yml`, and sponsorship badges/sections from documentation.

## [v0.8.1] - 2026-10-13

### Fixed
- **Lazy Session Auto-Reconnect (`internal/session`, `internal/server`)**: Transparently re-establish database connection sessions from the vault when accessed after being evicted due to inactivity, preventing unexpected HTTP 404 errors across processlist, query execution, and schema inspection when the app is left idle. Protect `LastActive` timestamps under session mutex.
- **Dynamic Date Segment Stepper & Clamping (`web`)**: Replaced native date input with custom text input and inline segment stepper buttons in the table editor. Increment/decrement active segment (year, month, day, hour, minute, second) based on caret position, auto-clamp day values to valid days in month, ensure table cell overflow remains visible during inline editing, and format temporal values cleanly.
- **Large Numeric Formatting & Scientific Notation Prevention (`internal/driver`, `internal/sync`)**: Format numeric values using decimal notation instead of `%v` in `FormatSQLValue`, preventing Go from switching to exponential scientific notation (e.g. `1.6906691e+07`) on values $\ge 10^6$. Added native unquoted handling for `json.Number`, standardized primary key representation in data synchronization transfers, and added test coverage for large integer PKs.

## [v0.8.0] - 2026-10-12

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
