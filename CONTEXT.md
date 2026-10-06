# SqlFox Domain Glossary

Core ubiquitous language for the SqlFox database management tool.

## Connections & Environment

**Connection**:
A configured target database endpoint containing host, network, protocol, and authentication credentials.
_Avoid_: Profile, host config, server alias

**Environment**:
The operational trust tier assigned to a Connection (`Production`, `Staging`, `Development`) that dictates the enforcement level of the Safety Guard.
_Avoid_: Stage, label, group

**Environment Badge**:
The singular visual status tag in the Session Switcher displaying the connection's Environment tier (`[PROD 🔴]`, `[STAGE 🟡]`, `[DEV 🟢]`), serving as the single source of truth for environment identity without duplicate labels in sidebar or footer.
_Avoid_: Multi-surface environment tags, sidebar environment headers, footer environment labels

**Connection Vault**:
The encrypted local SQLite storage holding connection metadata and credentials.
_Avoid_: Credential manager, config file

**Vault Archive**:
An encrypted, portable export file (`.sqlfox-vault`) protected by an export passphrase for cross-machine migration.
_Avoid_: Backup zip, cred dump

**Remote Access Guard**:
A security gate intercepting HTTP and WebSocket requests from non-loopback IP addresses, requiring Master Password authentication before granting access to the vault and sessions.
_Avoid_: Firewall, IP filter

**Port Hunting**:
An automated sequential port probing mechanism that resolves `EADDRINUSE` port collision errors by scanning subsequent free ports.
_Avoid_: Port hopping, auto-port

**Network Endpoint**:
The non-loopback IPv4 network address of the host machine accessible from other devices on the local area network.
_Avoid_: Public URL, remote host

## Drivers & Sessions

**Driver**:
A protocol-specific database connector (e.g. MySQL, SQLite, PostgreSQL) implementing lifecycle and session creation.
_Avoid_: Adapter, DB client

**Session**:
An active client interaction context bound to a Connection, managing open tabs, temporary schema cache, and transaction state.
_Avoid_: Workspace, window, console

**Search Path**:
The active PostgreSQL schema namespace configured for a session (`SET search_path = <schema>`), governing unqualified table resolution and UI schema browsing.
_Avoid_: Active catalog, schema switch, default db

**Backend Process ID (PID)**:
The unique server-side process identifier of a PostgreSQL backend worker, utilized for out-of-band cancellation (`pg_cancel_backend`) and lock tree visualization (`pg_blocking_pids`).
_Avoid_: Thread ID, connection ID, worker ID

**RowStream**:
A memory-bounded record sequence streamed from database drivers to the frontend via WebSocket/SSE chunks.
_Avoid_: Result buffer, fetch array

**Out-of-Band Cancellation**:
A query interruption mechanism dispatched via a secondary control connection issuing protocol termination (`KILL QUERY` on MySQL, `pg_cancel_backend(pid)` on PostgreSQL) without destroying the primary connection.
_Avoid_: Context cancel, socket kill

## Safety & Governance

**Safety Guard**:
The deterministic AST analysis gate that intercepts and evaluates SQL statements prior to execution against the target database.
_Avoid_: Linter, query validator, firewall

**Destructive Query**:
A SQL statement capable of permanently altering or removing schema objects or mass row data without explicit targeting (e.g., unqualified DELETE/UPDATE, DROP, TRUNCATE).
_Avoid_: Bad query, invalid SQL, dangerous command

**Challenge Modal**:
A mandatory confirmation dialog requiring explicit typing of target identifiers before executing high-risk operations.
_Avoid_: Alert box, confirm prompt

**Implicit Safe Limit**:
A bounded row restriction automatically applied to arbitrary user queries in the editor to prevent client-side and server-side memory exhaustion.
_Avoid_: Pagination cap, auto-limit

**Keyset Pagination**:
A stateful table viewing traversal mechanism that seeks records using indexed sequential keys rather than ordinal offsets.
_Avoid_: Cursor scan, row offset

**Impact Preview**:
A non-intrusive metadata and execution plan evaluation (`EXPLAIN`) displaying estimated affected rows and scan types prior to execution.
_Avoid_: Dry run, query check

**Smart Tiered Backup**:
An automated pre-DDL backup policy that captures DDL always and table data conditionally based on a 50MB size threshold.
_Avoid_: Auto-dump, full backup

**Safe Execution Pipeline**:
The unified execution seam within an active Session that evaluates queries against the Safety Guard, verifies Environment trust tiers and Challenge Modals, triggers pre-DDL Smart Tiered Backups, and enforces the Implicit Safe Limit across all client transports (HTTP and WebSocket).
_Avoid_: Query runner, execution handler, direct driver call

**Pre-Flight Lock Inspection**:
An automated diagnostic check intercepting DDL and large-scale mutations to detect open transactions, active metadata lock waiters, and blocking queries touching the target table prior to statement submission.
_Avoid_: Lock scanner, trx sniffer

**Fail-Fast DDL Timeout**:
An isolated session lock timeout policy applied on a dedicated connection (`lock_wait_timeout` or `busy_timeout`) that immediately aborts blocked DDL operations within seconds to prevent cascading connection pool exhaustion.
_Avoid_: Quick timeout, lock drop

**Schema Relationship Introspection**:
An automated dependency analysis scanning inbound and outbound foreign keys to identify high-risk schema alterations on master/lookup tables referenced by massive transaction tables.
_Avoid_: FK tracker, relation map

**OSC Generator**:
An automated utility formulating production-ready Online Schema Change commands (`gh-ost`, `pt-online-schema-change`) with load throttles and binlog streaming for non-instant schema migrations.
_Avoid_: DDL scripter, ghost wrapper

## Artificial Intelligence

**BYOK (Bring Your Own Key)**:
Direct client-side configuration and local storage of user-provided AI provider credentials, bypassing any intermediary server.
_Avoid_: API proxy, SaaS key

**Schema Context Window**:
The curated, just-in-time subset of database DDL metadata supplied to AI prompts based on active queries or explicit user references.
_Avoid_: Full schema dump, DB prompt

**AI Persona & System Instruction**:
User-defined behavioral guidelines and response language (e.g. Bahasa Indonesia, concise SQL) injected into Copilot's prompt context.
_Avoid_: Prompt tweak, bot style

**Chat Session**:
A discrete conversational thread with Copilot, uniquely identified and persisted locally with title and message history.
_Avoid_: Message log, chat thread

**Chat Session Forking**:
The process of branching a new, isolated Chat Session containing conversation history up to a selected Copilot response, enabling parallel exploration threads without corrupting the originating session.
_Avoid_: Duplicate chat, copy thread

**SQL Code Block Actions**:
Interactive action controls attached to rendered SQL code blocks within Copilot chat messages (`Copy`, `Insert & Run`, and `Replace`) enabling direct query execution and editor manipulation.
_Avoid_: Code buttons, snippet toolbar

**Internal Notes**:
A persistent in-memory/in-tab Markdown scratchpad (`Notes.md`) allowing developers to accumulate query explanations, execution findings, and AI insights.
_Avoid_: Text file, memo pad, clipboard buffer

**Notes Markdown Live Preview**:
A segmented mode toggle (`Edit` / `Preview`) within the Internal Notes tab that compiles and renders Markdown formatted text, tables, and code blocks in real time.
_Avoid_: Notes viewer, markdown tab

**Error Healer**:
An automated diagnostic loop intercepting failed SQL executions and dispatching root-cause analysis and query fixes to Copilot.
_Avoid_: Error fixer, bug helper

**Copilot Assistant**:
The unified AI reasoning coordinator in `internal/ai` that curates the Schema Context Window, injects relevant Table Knowledge Notes, enforces Firewall identifier masking, and compiles localized AI Personas across all AI actions (explain, optimize, debug, NL-to-SQL).
_Avoid_: AI handler, prompt builder, bot service

## Editor & Autocomplete

**Query Editor**:
An integrated code editor supporting SQL syntax highlighting, line numbers, and keyboard shortcuts.
_Avoid_: Text box, input area, raw textarea

**Schema Catalog Cache**:
An in-memory client-side cache of active database tables, columns, data types, and indexes fetched via session schema introspection.
_Avoid_: DB dump, schema buffer, table registry

**Context-Aware Completion Provider**:
An AST-lite SQL tokenizer and suggestion engine that dynamically filters completion candidates based on caret position (e.g. table names after `FROM`/`JOIN`, columns after `SELECT`/`WHERE`, and alias resolution `alias.column`).
_Avoid_: Word predictor, auto-suggester, flat matcher

**Completion Item**:
A ranked suggestion candidate with display label, kind badge (`[KW]`, `[TBL]`, `[COL]`, `[FN]`), and replacement text.
_Avoid_: Autocomplete option, hint token

**Session Tab State Persistence**:
A client-side storage mechanism bound per Connection (`sqlfox_tabs_<conn_id>`) that continuously synchronizes open Query Editor tabs, SQL draft buffers, active tab focus, and Table Viewer metadata across page reloads.
_Avoid_: Tab autosave, tab cache, session restore


## Schema Management & Native Dialogs

**SQL Formatter**:
A client-side query beautifier that splits SQL clauses (`SELECT`, `FROM`, `WHERE`, `JOIN`, `GROUP BY`, `ORDER BY`) into indented lines and capitalizes keywords without remote roundtrips.
_Avoid_: Online formatter, backend beautifier

**Native Modal Dialog**:
A non-blocking HTML5 `<dialog>` element providing consistent styling, focus trapping, and Esc key dismissal across the entire application, replacing primitive browser popup functions.
_Avoid_: Alert box, confirm prompt, window.alert, window.confirm

**Table Designer**:
An interactive visual workspace for inspecting and altering table definitions (columns, data types, primary keys, indexes, and foreign keys) with staged change accumulation.
_Avoid_: Schema wizard, raw alter window

**View Designer**:
A specialized schema editor for creating and modifying database views with query syntax verification.
_Avoid_: View wizard

**Trigger Designer**:
A structured GUI form for configuring database triggers specifying timing (`BEFORE`/`AFTER`), event (`INSERT`/`UPDATE`/`DELETE`), target table, and trigger body statements.
_Avoid_: Trigger wizard

**DDL Review Modal**:
A pre-flight verification step displaying staged DDL statements for user approval before dispatching through the Safety Guard and Smart Tiered Backup.
_Avoid_: Direct execute, silent DDL

**Type Guard Palette**:
A curated database-dialect-aware dropdown and length restriction system that prevents invalid data type specifications (e.g. disallowing length on INT/TEXT/DATETIME).
_Avoid_: Freeform type input, unvalidated type string

**Default Value Preset**:
A structured selector offering valid default states (`None`, `NULL`, `CURRENT_TIMESTAMP`, `0`, `Empty String`, `Custom`) to prevent literal quoting mistakes and invalid nullability combinations.
_Avoid_: Raw default input, unquoted string default

**Cross-Database Catalog**:
A cached multi-database schema index enabling contextual autocompletion across schemas (e.g. `dbname.tablename.colname`) without manual switching.
_Avoid_: Single-db autocomplete, schema silo

**Session Switcher**:
A compact header picker for switching, filtering, and managing saved database connection sessions, reserving the sidebar tree exclusively for schema objects.
_Avoid_: Permanent sidebar connection tree accordion

**Schema Search**:
An instant cross-database catalog finder locating tables, views, and columns across all accessible schemas in the active connection.
_Avoid_: Global grep, object finder, quick navigator

**Database Creator**:
A structured modal dialog for provisioning a new database schema on the active target server with dialect-aware character set and collation defaults.
_Avoid_: Raw create prompt, DB wizard

**Execution Plan Inspector**:
A comprehensive modal interface for inspecting database query execution plans (`EXPLAIN`), featuring a dedicated Target Query card with client-side formatting, a modern segmented tab switcher, the tabular **Execution Plan Grid**, the graphical **Visual Execution Tree**, the heuristic **Index Advisor**, and raw output inspection.
_Avoid_: Raw plan dump, text explain drawer, query modal

**Visual Execution Tree**:
A graphical directed pipeline flow component that translates execution plans (from PostgreSQL/MySQL JSON or tabular steps) into connected visual node cards showing operation access types, target tables, key lookups, row volumes, and bottleneck alerts.
_Avoid_: Text tree, ASCII plan, static explain graph

**Execution Plan Grid**:
An interactive tabular display of database engine execution plans (`EXPLAIN`), highlighting access types (Full Table Scan warnings), estimated row counts, and used indexes with a toggleable raw JSON payload view.
_Avoid_: Flat explain list, console explain

**Database Switcher**:
A searchable popover dropdown in the sidebar for filtering and switching the active database schema in multi-database server connections, matching the Session Switcher interaction pattern.
_Avoid_: Raw select dropdown, static DB selector

**Schema Object Filter**:
An real-time, sticky client-side search input in the sidebar explorer for instantaneously filtering tables, views, and triggers by name with quick keyboard access (`/`).
_Avoid_: Tree grep, manual scroll search

**Collapsible Schema Tree**:
An interactive sidebar grouping mechanism organizing schema items into independent collapsible sections (`Tables`, `Views`, `Triggers`) with counts and persistent state across reloads.
_Avoid_: Flat tree, uncollapsible object list

**Keyboard Shortcuts Cheatsheet**:
A centralized native modal dialog displaying organized hotkeys grouped by functional domain (Query Execution, Navigation, Tabs, Tools), discoverable via `?`, `F1`, status bar, or Command Palette.
_Avoid_: Hardcoded button shortcut badges, fragmented hotkey hints

**Maximized Search & Filter Control**:
A standardized toolbar input design pattern featuring dynamic horizontal expansion, leading search icon (`🔍`), dynamic clear button (`✕`), and native `Escape` reset handling across all application toolbars and dialogs.
_Avoid_: Cramped fixed-width input, static un-clearable textbox

## Productivity & Knowledge Base

**Query Snippet**:
A reusable parameterized SQL template stored in the local Connection Vault, supporting dynamic substitution tokens and optional connection or engine scoping tags.
_Avoid_: Saved query, SQL macro, code block

**Dynamic Variable**:
A substitution placeholder (e.g. `{{tenant_id}}`) within a Query Snippet resolved prior to editor injection via a parameter modal.
_Avoid_: Query argument, template param, input token

**Snippet Parameter Modal**:
A native modal dialog prompting the user for Dynamic Variable values when invoking a Query Snippet, performing live substitution into the Query Editor.
_Avoid_: Variable popup, param window, prompt box

**Table Knowledge Note**:
A local Markdown documentation record bound to a specific database table stored in the local Connection Vault without modifying the remote schema.
_Avoid_: Server table comment, schema note, table wiki

## Data Interchange & Portability

**CSV Delimiter Sniffing**:
An automated frequency-analysis check on initial CSV records to detect whether the file uses commas, semicolons, tabs, or pipe separators.
_Avoid_: Auto-split, delimiter guesser

**Column Mapping Matrix**:
A visual pre-flight configuration interface mapping each CSV source column to an existing database table column with auto-matching and skip capabilities.
_Avoid_: Column chooser, field selector

**Excel-Safe CSV Export**:
A CSV generation format that prepends a UTF-8 BOM and wraps leading-zero string identifiers in spreadsheet formulas (`="0812..."`) to prevent loss of leading digits or scientific notation coercion.
_Avoid_: Excel patch, quote hack

**CSV Ingestion Engine**:
A streaming data import module that combines automated Delimiter Sniffing, Column Mapping Matrix resolution, parameter ceiling batching, and row-by-row fallback retries to populate target database tables from arbitrary CSV feeds.
_Avoid_: CSV loader, import script, raw insert loop

## Data Synchronization & Table Operations

**Data Transfer**:
A streaming bulk migration process that reads schema and rows directly from a source session and writes them to a target session across connections without temporary disk staging.
_Avoid_: DB copier, dump-and-load pipe, table clone

**Data Sync**:
A row-level reconciliation mechanism that performs keyset comparisons between source and target tables, computes delta changes (`INSERT`, `UPDATE`, `DELETE`), and executes selective atomic updates.
_Avoid_: Row matcher, table synchronizer, data merge

**Structure Sync**:
An AST-based DDL comparison engine that identifies structural discrepancies (missing tables, altered columns, missing indexes, foreign key drift) between two schemas and generates idempotent migration scripts.
_Avoid_: Schema diff tool, DDL merger, table reconciler

**Table CRUD Operator**:
A direct spreadsheet-like interaction model within the Virtual Data Grid allowing users to stage new row insertions, delete selected rows under Safety Guard validation, and apply quick visual column filters without raw SQL.
_Avoid_: Edit grid, table viewer, row manager

**Staged Grid Changes**:
An in-memory ledger accumulating uncommitted cell modifications across rows and columns in the Virtual Data Grid, highlighting modified cells with visual dirty badges (`●`) and providing atomic "Apply" and "Discard" actions.
_Avoid_: Unsaved buffer, dirty queue, batch edit cache

**Immediate Persist Mode**:
A data grid editing mode where cell modifications automatically persist to the remote database upon input blur or Enter key submission, bypassing manual staging.
_Avoid_: Direct save, auto-flush, instant write

**Type-Colored Grid**:
A visual typography and layout convention within the Virtual Data Grid that dynamically styles cell text color and alignment based on database column data types (Emerald `text-emerald-400` and right-aligned for Numerics, Purple `text-purple-400` and right-aligned for Temporal, Rose `text-rose-400` for Booleans, Amber `text-amber-400` for Binary, and Slate `text-slate-200` left-aligned for Text), preserving dimmed italic styling for `NULL` values and staged amber indicators for uncommitted cell mutations.
_Avoid_: Colorful cells, syntax grid, rainbow rows.

**Natural Column Sizing**:
A responsive column layout policy within the Virtual Data Grid that sizes table columns to fit their header and content within bounded min-width (90px) and max-width (384px / `max-w-md`) constraints without forcing full-viewport width stretching on narrow datasets.
_Avoid_: Full-width grid, stretched columns, bloated cells.

**Row Criteria**:
A synthesized parameter map and WHERE predicate (`WHERE col1 = ? AND col2 = ? ... LIMIT 1`) constructed from all non-null column values of an existing row, enabling safe inline updates and deletions on tables lacking a formal primary key or unique index.
_Avoid_: Synthetic PK, pseudo-key, blind update

**Grid Value Presets**:
A floating quick-action toolbar and keyboard shortcut suite (`Alt+N` for NULL, `Alt+E` for Empty String, `Alt+T` for NOW(), `Alt+D` for Current Date, `Alt+U` for UUID v4) providing rapid data entry during grid cell editing.
_Avoid_: Quick inputs, macro keys, cell templates

**Type-Aware Cell Editor**:
A contextual inline input interface that adapts its UI control and validation rules based on column data type (native `<select>` for `ENUM`, toggle button for `BOOLEAN`, and foreign key search picker for relational columns).
_Avoid_: Custom editor, input widget, field control

**SQL Dump Runner**:
A memory-bounded streaming executor and generator for `.sql` files that breaks multi-statement scripts into transactional chunks without loading entire multi-gigabyte files into memory.
_Avoid_: SQL runner, dump importer, script player

## DBA Operations & Diagnostics Suite

**Processlist Manager**:
An interactive real-time database thread and connection monitor featuring slow-query threshold alerts and one-click out-of-band query or session termination (`KILL QUERY` / `KILL CONNECTION`).
_Avoid_: Process monitor, task list, thread viewer

**Process Inspector Modal**:
A dedicated diagnostic dialog displaying complete thread execution metadata, formatted full SQL statement text without truncation, 1-click clipboard actions, and integrated operational shortcuts (`EXPLAIN`, editor transfer, AI review, and termination).
_Avoid_: Process details popup, thread viewer window, query popover

**Table Diagnostics**:
A physical storage inspection module providing exact data length, index length, and fragmentation overhead statistics alongside one-click table maintenance actions (`OPTIMIZE TABLE`, `ANALYZE TABLE`, `CHECK TABLE`).
_Avoid_: Table health, storage checker, disk tool

**Database Storage & Health Inspector**:
A comprehensive two-level diagnostic interface inspecting physical storage, table counts, collation distribution, and operational hygiene across entire database sessions and individual schemas.
_Avoid_: DB size checker, storage modal, disk viewer

**Session Storage Overview**:
The session-level view within the Database Storage & Health Inspector aggregating total instance storage, table counts, default character sets, and collations across all accessible databases with system schema toggling.
_Avoid_: Database list, catalog summary, instance sizes

**Table Storage & Health Grid**:
The schema-level view within the Database Storage & Health Inspector detailing physical table storage (data, index, and overhead bytes), row formats, collations, and actionable health evaluations per table.
_Avoid_: Table size list, db breakdown, storage table

**Table Health Advisor**:
A rule-based diagnostic engine evaluating tables for structural and performance anti-patterns (excessive fragmentation overhead, missing primary keys, legacy storage engines, and deprecated collations) with 1-click remediation handoffs.
_Avoid_: Table linter, health assistant, schema checker

**Privilege Matrix**:
A visual user management and access control interface allowing DBAs to provision accounts, restrict host origins, and configure granular permissions across global, database, and table scopes via an interactive grid.
_Avoid_: User editor, permission manager, grant wizard

**Foreign Key Hyperlink**:
A navigational link embedded within data grid cells referencing foreign keys, enabling inline relational data peeking and direct traversal to parent table records without manual SQL queries.
_Avoid_: FK jump, relational link, key preview

**Execution Profiler**:
A fine-grained query execution analysis tool visualizing sequential engine lifecycle stages (`System lock`, `optimizing`, `statistics`, `Sorting result`, `Copying to tmp table`) and stage durations via `SHOW PROFILE` or `performance_schema`.
_Avoid_: Query timer, stage tracer, performance graph

**Redundant Index Detector**:
A static schema analysis check identifying duplicate, shadowed, or redundant prefix indexes that consume disk and memory bandwidth while degrading write throughput.
_Avoid_: Index linter, key cleaner, duplicate checker

**Session Audit Log**:
A persistent session-level ledger tracking all executed SQL statements and data/schema mutations with origin attribution (`source`), timestamps, execution durations in milliseconds, affected row counts, and error states.
_Avoid_: History tab, query log, command list

**Audit Source Attribution**:
A canonical origin taxonomy tagging every audit log entry with its triggering feature context: `editor` (interactive Query Editor / SQL tab), `grid_cell_edit` (inline cell edit in data grid), `grid_insert` (inline row addition), `grid_delete` (grid row deletion), `script_runner` (large SQL script runner), `csv_import` (CSV dataset import wizard), `mock_data` (synthetic data generator), `sync_transfer` (cross-database data transfer), or `schema_sync` (DDL structure synchronization).
_Avoid_: Trigger type, log category, caller tag


**Lock Dependency Tree**:
An interactive hierarchical diagnostic view that maps blocked database sessions to their blocking transactions (utilizing `sys.innodb_lock_waits` / `performance_schema.data_lock_waits`), explicitly isolating the root blocker session, locked resource, lock mode, and root cause.
_Avoid_: Lock viewer, block list, wait graph

**Referential Slice Exporter**:
A zero-configuration, schema-wide database dumper that exports complete DDL structure alongside bounded row samples (e.g., 20 rows per table) without requiring manual table tagging; automatically scans the entire Foreign Key dependency graph and recursively fetches all upstream referenced parent rows (transitive closure), writing inserts in topological order to guarantee referentially integral, zero-error SQL restore files.
_Avoid_: Partial dump, table sampler, mini export

**Inline Table Filter Bar**:
A dedicated querying control docked above the Virtual Data Grid that allows users to filter remote records using either structured clause pills or raw SQL expressions without navigating away from the table view.
_Avoid_: Search box, table filter, grid search

**Quick Clause Pill**:
A modular visual filter condition element (`[Column] [Operator] [Value]`) composed inside the Inline Table Filter Bar that compiles to a sanitized SQL WHERE clause.
_Avoid_: Filter tag, query chip, search token

**Raw WHERE Expression**:
A freeform SQL Boolean expression typed directly into the Inline Table Filter Bar executed via Server-Side Filter Pushdown with Safety Guard injection validation.
_Avoid_: Custom query, manual filter, SQL string

**Server-Side Filter Pushdown**:
An execution strategy that forwards active filter expressions to the database engine as an indexed `WHERE` clause rather than filtering pre-fetched rows in client memory.
_Avoid_: In-memory filter, client search, local filter

**Grid Query Handoff**:
A one-click action transferring the currently active table, column filters, and sort order from the Table Viewer directly into a new Query Editor tab as a complete editable SQL script.
_Avoid_: Export to SQL, copy query, open in editor

**Cluster & Replication Inspector**:
A dynamic topology and diagnostic monitor within the DBA Operations Suite that automatically detects Galera/PXC clustering, standard MySQL replication, or standalone instances to visualize node synchronization, quorum health, and replication queues.
_Avoid_: Cluster monitor, Galera viewer, replication dashboard

**Flow Control Pressure Indicator**:
A real-time metric evaluator measuring `wsrep_flow_control_paused` to detect cluster-wide write throttling, isolating whether local write set applying or a remote peer node is exerting backpressure.
_Avoid_: Lag meter, pause checker, throttling alert

**Cluster Bottleneck Detective**:
A heuristic diagnostic engine correlating inter-node network replication latency (`wsrep_evs_repl_latency`), disk IO wait states (`wait/io/file/innodb/*`), and thread queues to identify the slowest node or hardware resource causing cluster stalls.
_Avoid_: Slow finder, cluster analyzer, node debugger

**Peer Node Auto-Discovery**:
An out-of-band probing mechanism that extracts active cluster IP addresses from `wsrep_incoming_addresses` and dispatches lightweight status queries across all cluster members in parallel using the active session credentials to compare receive queues and pinpoint the culprit node.
_Avoid_: Node scanner, cluster pinger, server crawler

**Tri-Metric Bottleneck Matrix**:
A unified diagnostic heuristic comparing inter-node network round-trip latency (`wsrep_evs_repl_latency`), physical storage write stalls (`Innodb_buffer_pool_wait_free`, fsync wait events), and worker saturation (`wsrep_slave_threads`) to isolate whether storage, network, or concurrency is the bottleneck.
_Avoid_: Hardware checker, slow alert, performance dashboard

**Performance Flight Recorder**:
An on-demand, time-bounded telemetry recorder capturing differential database server status metrics and active processlist snapshots during an operational investigation window.
_Avoid_: APM agent, background daemon, metrics monitor

**Recording Session**:
A stateful capture lifecycle (`IDLE`, `RECORDING`, `STOPPED`) initiated and terminated explicitly by the user, governed by an automatic safety timeout and surfaced via a persistent global recording badge.
_Avoid_: Profiling job, trace task, run batch

**Quad-Metric Telemetry**:
A 4-dimensional differential performance vector sampled per tick: Throughput (QPS), Concurrency (`Threads_running`), Contention (Row Lock Waits), and Storage Latency (InnoDB Buffer Pool Waits).
_Avoid_: Server stats, raw status, generic metrics

**Correlated Process Snapshot**:
A lightweight, filtered snapshot of non-idle running threads tied to a specific metric timestamp, captured automatically when concurrency or query duration crosses threshold limits.
_Avoid_: Full thread dump, process backup

**Server Variable Inspector**:
An interactive system configuration explorer that introspects and displays all live database engine parameters (`SHOW GLOBAL VARIABLES`), categorized by subsystem (Memory, Concurrency, Durability, Cluster, Optimizer), with real-time search and filter capabilities.
_Avoid_: Config viewer, variables list, settings browser

**Variable Tuning Advisor**:
A heuristic knowledge and diagnostic engine that correlates active server variable settings against curated best-practice rules, surfacing operational explanations, trade-off impacts (low vs high risks), baseline recommendations, and 1-click `my.cnf` / `SET GLOBAL` configuration snippets.
_Avoid_: Tuning wizard, config helper, optimizer assistant

**Cluster Parameter Advisor**:
A specialized heuristic validator embedded within the Cluster & Replication Inspector that analyzes Galera and replication configuration parameters (`wsrep_slave_threads`, `innodb_autoinc_lock_mode`, `binlog_format`, `wsrep_sync_wait`) and flags suboptimal or destabilizing values with traffic-light health badges.
_Avoid_: Cluster config checker, replication validator, wsrep reviewer

## Codebase Architecture & Maintenance (Ponytail)

**Canonical UI Language**:
The entire application user interface, notification modals, pre-built Copilot prompt templates, and CLI output strictly adhere to American English as the standard base language. User-preferred localizations (e.g. Bahasa Indonesia) are exclusively managed dynamically through the user-configured *AI Persona & System Instruction* for Copilot conversational output, avoiding premature i18n abstraction layers.
_Avoid_: Mixed-language UI, multilingual i18n framework, hardcoded non-English templates

**Native Platform Tunneling**:
Relying on standard OS facilities (`ssh -N -L 3306:remote-db:3306 user@bastion` or platform VPN) for remote private network database connectivity, avoiding custom in-process SSH dialer complexity and unexposed vault database columns.
_Avoid_: In-process tunnel, custom bastion client

## Architectural Decision Records (ADR)
- **0001**: CGo-Free Pure Go Drivers (`docs/adr/0001-cgo-free-pure-go-drivers.md`)
- **0002**: Deterministic AST Safety Guard (`docs/adr/0002-deterministic-ast-safety-guard.md`)
- **0003**: Desktop Localhost Runtime Model (`docs/adr/0003-desktop-localhost-runtime-model.md`)
- **0004**: CodeMirror and Embedded SPA (`docs/adr/0004-codemirror-and-embedded-spa.md`)
- **0005**: Out-of-Band Query Cancellation (`docs/adr/0005-out-of-band-query-cancellation.md`)
- **0006**: Smart Tiered DDL Backup (`docs/adr/0006-smart-tiered-ddl-backup.md`)
- **0007**: Multi-IP Access and Port Hunting (`docs/adr/0007-multi-ip-access-and-port-hunting.md`)
- **0008**: Native Dialogs and Staged DDL Designer (`docs/adr/0008-native-dialogs-and-staged-ddl-designer.md`)
- **0009**: Cross-Database Schema Search (`docs/adr/0009-cross-database-schema-search.md`)
- **0010**: Stateless Schema Introspection and Database Isolation (`docs/adr/0010-stateless-schema-introspection-and-database-isolation.md`)
- **0011**: Dynamic EXPLAIN Plan Introspection and Visual Grid (`docs/adr/0011-dynamic-explain-plan-introspection-and-visual-grid.md`)
- **0012**: Sidebar Schema Filtering and Searchable Database Switcher (`docs/adr/0012-sidebar-schema-filtering-and-searchable-database-switcher.md`)
- **0013**: Parameterized Query Snippets and Vault Persistence (`docs/adr/0013-parameterized-query-snippets-and-vault-persistence.md`)
- **0014**: Table Knowledge Notes and AI Context Injection (`docs/adr/0014-table-knowledge-notes-and-ai-context-injection.md`)
- **0015**: Zero-Dependency CSV Import with Column Mapping (`docs/adr/0015-zero-dependency-csv-import-with-column-mapping.md`)
- **0016**: Grid Inline Edit Lifecycle and Staged Apply (`docs/adr/0016-grid-inline-edit-lifecycle-and-staged-apply.md`)
- **0017**: Real-Time Processlist Manager and Session Audit Log (`docs/adr/0017-real-time-processlist-manager-and-session-audit-log.md`)
- **0018**: Advanced Diagnostics and Privilege Matrix (`docs/adr/0018-advanced-diagnostics-and-privilege-matrix.md`)
- **0019**: Inline Table Filter Bar, Server-Side Pushdown, and Grid Query Handoff (`docs/adr/0019-inline-table-filter-bar-and-server-side-pushdown.md`)
- **0020**: Ponytail Audit, Codebase Pruning, and Native Platform Tunneling (`docs/adr/0020-ponytail-audit-codebase-pruning-and-native-ssh.md`)
- **0021**: Workspace Ergonomics, Shortcuts Discovery, and Search Standardization (`docs/adr/0021-workspace-ergonomics-and-search-standardization.md`)
- **0022**: Safe DDL Guardrails and Lock Contention Prevention (`docs/adr/0022-safe-ddl-guardrails-and-lock-contention-prevention.md`)
- **0023**: Dynamic Cluster & Replication Inspector with Peer Node Auto-Discovery and Tri-Metric Bottleneck Detective (`docs/adr/0023-cluster-and-replication-inspector.md`)
- **0024**: Ponytail Pruning, Stateless Service Simplification, and Unified AI Streaming Parser (`docs/adr/0024-ponytail-pruning-and-stateless-service-simplification.md`)
- **0025**: On-Demand Performance Flight Recorder and Correlated Process Snapshots (`docs/adr/0025-on-demand-performance-flight-recorder-and-correlated-process-snapshots.md`)
- **0026**: Copilot Session Forking, Rich Markdown Blocks, and Fluid Workspace Ergonomics (`docs/adr/0026-copilot-session-forking-rich-markdown-and-fluid-workspace.md`)
- **0027**: Streaming SQL Dump, Referential Slice Seeding, and Grid Spreadsheet CRUD Architecture (`docs/adr/0027-streaming-sql-dump-referential-slice-seeding-and-grid-crud.md`)
- **0028**: Schema Structure Sync, Cross-Connection Data Transfer, and Row-Level Data Sync Engine (`docs/adr/0028-schema-structure-sync-cross-connection-data-transfer-and-row-data-sync.md`)
- **0029**: Visual ERD Interactive Canvas (`docs/adr/0029-visual-erd-canvas.md`)
- **0030**: Headless CLI Suite Architecture and Fail-Closed Automation Policy (`docs/adr/0030-headless-cli-suite-and-fail-closed-automation-policy.md`)
- **0031**: PostgreSQL Native Driver Engine and Catalog Introspection (`docs/adr/0031-postgresql-native-driver-engine-and-catalog-introspection.md`)
- **0032**: FK-Aware Mock Data Generation and Referential Closure (`docs/adr/0032-fk-aware-mock-data-generation-and-referential-closure.md`)
- **0033**: Scheduled Headless Automation and Declarative Task Runner (`docs/adr/0033-scheduled-headless-automation-and-declarative-task-runner.md`)
- **0034**: Large-Scale Hardening and Memory Ceiling Benchmark (`docs/adr/0034-large-scale-hardening-and-memory-ceiling-benchmark.md`)
- **0035**: Heuristic Index Advisor & Query Tuner (`docs/adr/0035-heuristic-index-advisor-and-query-tuner.md`)
- **0036**: In-App Self-Update Engine, Changelog Viewer & Author Attribution (`docs/adr/0036-in-app-self-update-changelog-and-author-attribution.md`)
- **0037**: Canonical Brand Mark & Cross-Platform Icon Standardization (`docs/adr/0037-canonical-brand-mark-and-cross-platform-icon-standardization.md`)
- **0038**: Grid Edit Presets, Type Constraints, and PK-Less Mutations (`docs/adr/0038-grid-edit-presets-type-constraints-and-pk-less-mutations.md`)
- **0039**: Universal Mutation Logging and Audit Source Attribution (`docs/adr/0039-universal-mutation-logging-and-audit-source-attribution.md`)
- **0040**: Server Variable Inspector and Cluster Tuning Advisor (`docs/adr/0040-server-variable-inspector-and-cluster-tuning-advisor.md`)

## Visual Schema & ERD Canvas

**Visual ERD Canvas**:
An interactive, zero-dependency schema visualization tool within a native modal dialog that renders database tables as draggable cards and foreign key constraints as dynamic SVG cubic Bézier curves.
_Avoid_: Schema graph, DB diagram, table map

**Auto Layout**:
A square-root grid placement algorithm that automatically organizes database table cards on the Visual ERD Canvas to prevent overlap without manual positioning.
_Avoid_: Auto position, magic layout, table aligner

**Fit All Viewport**:
A bounding-box calculation and CSS scaling/translation transformation that smoothly pans and zooms the Visual ERD Canvas to display all table cards within the visible modal viewport.
_Avoid_: Zoom to fit, reset zoom, center diagram

**ERD Session Position Cache**:
A session-level storage (`sessionStorage`) mechanism preserving dragged table card coordinates on the Visual ERD Canvas per database session across modal toggles without modifying remote schemas.
_Avoid_: Diagram layout file, saved canvas, position state

## Headless CLI & Automation

**Headless Mode**:
Non-interactive, terminal-driven execution of SqlFox subcommands (`query`, `transfer`, `job`) without launching the embedded HTTP daemon or browser interface.
_Avoid_: Silent mode, daemonless mode, batch mode

**Direct DSN Targeting**:
Specifying database connection coordinates directly via connection string (`--dsn`) for ephemeral execution in CI/CD pipelines, bypassing the local encrypted connection vault.
_Avoid_: Raw URL, ad-hoc connection, unvaulted connection

**Fail-Closed Safety Guard**:
The CLI execution policy that unconditionally aborts destructive SQL statements with a non-zero exit code in non-interactive sessions unless an explicit bypass flag (`--force`) is supplied.
_Avoid_: Panic mode, strict abort, safety error

**Stream Output Format**:
Output serializations supported by headless query execution (`table`, `json`, `ndjson`, `csv`) routing clean data streams to standard output (`stdout`) and diagnostics/progress to standard error (`stderr`).
_Avoid_: Print mode, report type

**Per-Table Atomicity**:
Executing cross-connection data transfers within independent transaction boundaries per target table rather than a single monolithic cross-table transaction, preventing lock contention and undo log exhaustion.
_Avoid_: Table isolation, chunked commit

**Multi-Statement Delimiter Parsing**:
Token-aware SQL statement splitting that distinguishes statement terminators (`;`) from semicolons within string literals, quotes, and block comments during headless script execution.
_Avoid_: Semicolon split, raw splitting

**Job Specification**:
A declarative, version-controlled JSON definition (`sqlfox-job.json`) that specifies an ordered sequence of database automation tasks, connection targets, output configurations, and retention policies.
_Avoid_: Batch script, cron config, pipeline spec

**Task Runner**:
The headless CLI orchestration engine dispatched via `sqlfox job run` that validates, sequences, and executes scheduled automation tasks with structured logging and fail-fast semantics.
_Avoid_: Cron daemon, worker process, background agent

**Dump Rotation**:
An automated artifact pruning mechanism that enforces retention policies (`keep_last` count or `max_age` duration) against matching backup files to prevent disk exhaustion.
_Avoid_: File garbage collection, cleaner, auto-purge

**Pre-Flight Job Check**:
A non-destructive validation execution triggered via `sqlfox job check` verifying job specification syntax, vault credentials, network connectivity, and destination filesystem write permissions without performing data mutations.
_Avoid_: Dry run, linter, config test

## Mock Data Generation

**Mock Data Generator**:
A zero-dependency synthetic data synthesizer in `internal/mock/` generating type-aware realistic column values and inserting them into a target table respecting foreign key referential integrity.
_Avoid_: Test data loader, seed script, fake data tool

**Topological Seed Order**:
A Kahn's-algorithm-derived insertion sequence that sorts tables by their FK dependency graph so parent reference tables are always populated before their dependent child tables.
_Avoid_: FK insertion order, dependency queue

**FK Pool**:
An in-memory collection of up to 1,000 sampled primary key values fetched from a parent reference table after its own seed pass, used to satisfy foreign key column constraints in child table INSERT batches.
_Avoid_: Parent key cache, reference pool, FK buffer

**Self-Referential FK**:
A foreign key column that references the primary key of the same table (e.g. `manager_id → users.id`), resolved in a two-pass strategy: rows are inserted with NULL in the self-referential column, then a follow-up UPDATE assigns randomly sampled PKs from the just-inserted pool.
_Avoid_: Recursive FK, circular self-reference

## Large-Scale Hardening & Benchmarking

**Memory Ceiling Budget**:
An architectural constraint strictly enforcing that SqlFox resident set size (RSS) RAM consumption remains under 100MB even when processing enterprise-scale streaming pipelines (`RowStream`, cross-connection transfer, and streaming dumps).
_Avoid_: Memory limit, RAM cap, buffer threshold

**Synthetic Schema Benchmark**:
An automated in-memory test harness in `_test.go` simulating 1,000+ tables, 50,000 columns, and cyclical foreign keys to measure schema catalog caching, serialization throughput, and memory allocations without production footprint.
_Avoid_: Fake schema generator, mock DB, test database

**Content Visibility Virtualization**:
A zero-dependency browser-native rendering optimization utilizing CSS `content-visibility: auto` and `contain-intrinsic-size` on sidebar schema trees to maintain 60fps scrolling and instant filtering across thousands of database objects without third-party virtual list libraries.
_Avoid_: Virtual tree, windowing component, lazy DOM

**Fail-Fast Control Channel**:
A time-bounded (3s) secondary database connection path used for out-of-band cancellation (`KILL QUERY` / `pg_cancel_backend`) that aborts client-side streaming and surfaces pool exhaustion diagnostics when server-side connection slots are saturated.
_Avoid_: Secondary killer, kill connection, cancel hook

## Query Optimization & Index Tuning

**Heuristic Index Advisor**:
An automated, deterministic rule-based query tuning engine in `internal/advisor/` that inspects AST predicates (`WHERE`, `JOIN`, `ORDER BY`, `GROUP BY`), existing catalog indexes, and EXPLAIN plans to formulate optimal `CREATE INDEX` recommendations.
_Avoid_: AI indexer, magic optimizer, auto-indexer

**ESR Rule (Equality, Sort, Range)**:
An indexing column sequencing heuristic ordering equality conditions first, sort/group columns second, and range predicates last to eliminate filesorts and maximize composite index selectivity.
_Avoid_: Column sorter, index ordering hack, query alignment

**Index Bloat Warning**:
A diagnostic notice alerting users when a table exceeds safe index thresholds (e.g. >5 secondary indexes or index-to-data storage overhead) to caution against write degradation while still delivering recommended index definitions.
_Avoid_: Too-many-indexes error, write penalty alert

**Safe DDL Generation**:
Dialect-aware index creation DDL leveraging non-blocking execution strategies (`ALGORITHM=INPLACE, LOCK=NONE` for MySQL 5.7 and 8.0, `CONCURRENTLY` for PostgreSQL, and `IF NOT EXISTS` for SQLite).
_Avoid_: Raw index SQL, blocking alter, instant DDL

## Distribution & Workspace Attribution

**Update Engine**:
The native Go subsystem (`internal/server/update.go`) responsible for polling GitHub Releases, evaluating semver version deltas, streaming platform-specific binary archives (`.tar.gz` / `.zip`), performing atomic binary replacement on disk, and triggering graceful application re-execution without third-party dependencies.
_Avoid_: Auto-installer, updater daemon, external package manager

**Update Modal**:
A native HTML5 dialog (`#update-dialog`) presenting release metadata, version diffs (`vCurrent` -> `vLatest`), rendered Markdown changelog release notes, and real-time self-update progress states.
_Avoid_: Update popup, new version alert, release prompt

**Status Bar Footer**:
The persistent bottom viewport bar (`<footer>`) structured into two distinct regions: real-time database runtime telemetry on the left (dialect engine flavor & version, connection target, server uptime, interactive ping latency), and developer attribution, system heap memory consumption, keyboard shortcuts modal launcher, and application version badge with self-update notifications on the right.
_Avoid_: Bottom bar, taskbar, dock

**Author Credit**:
The workspace attribution element residing at the far-right terminus of the status bar footer (after the application version badge), featuring creator credit (`Created by herliansyah`) linked to the author's GitHub profile, the official repository icon, and an unobtrusive sponsorship link (`☕ Traktir Kopi`).
_Avoid_: Footer branding, watermark, sponsor banner

**Sponsorship Link**:
A passive, non-intrusive external hyperlink in the right-side footer cluster pointing directly to the author's Saweria funding page (`https://saweria.co/herliansyah26`) for user-supported project appreciation without popups or interstitial modals.
_Avoid_: Donation popup, paywall, donation nag

**Changelog**:
The canonical, structured human-readable document (`CHANGELOG.md`) tracking notable application changes across releases, formatted strictly under the Keep a Changelog standard and Semantic Versioning.
_Avoid_: Release notes file, update log, commit digest

**Release Routine**:
The deterministic, script-driven protocol (`scripts/release.sh`) for cutting a release: running test pre-flights, advancing `[Unreleased]` changes to a dated semver heading, bumping code version constants, committing, creating annotated git tags, and triggering multi-platform CI release builds.
_Avoid_: Manual tag, ad-hoc release, unversioned bump

**Changelog Viewer**:
The native HTML5 modal dialog (`#changelog-dialog`) presenting complete application history and offline-accessible release logs rendered from embedded markdown, accessible directly from the footer version element and Command Palette.
_Avoid_: Release popup, changelog window, version toast

## Brand Identity & Application Icons

**Brand Mark**:
The canonical geometric low-poly fox emblem in vector SVG format embodying SqlFox's identity, featuring Amber/Orange facets (`#f97316`), deep orange shadow depth (`#ea580c`), and futuristic cyan accents (`#38bdf8`).
_Avoid_: Generic fox emoji, temporary logo, mascot image

**App Icon Set**:
The standardized cross-platform icon bundle generated deterministically from the Brand Mark, encompassing scalable vector assets (`logo.svg`, `favicon.svg`), multi-resolution Windows ICO (`favicon.ico`, `icon.ico`), Apple Touch Icon (`apple-touch-icon.png`), and PWA manifests (`icon-192.png`, `icon-512.png`).
_Avoid_: Web-only icons, favicon bundle, image assets

**Titlebar Brand**:
The crisp, inline vector SVG representation of the Brand Mark embedded in the top navbar header and native HTML5 dialogs, providing unified, OS-independent visual fidelity.
_Avoid_: Header emoji, window title icon, navbar image

**Binary Icon Resource**:
The pre-compiled COFF resource object (`cmd/sqlfox/rsrc_windows_*.syso`) embedding the Brand Mark directly into the Windows PE binary header (`sqlfox.exe`) during standard Go compilation without external build-time compilers.
_Avoid_: Resource hacker, external res wrapper, manual icon stamper




