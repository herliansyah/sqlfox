# SqlFox Domain Glossary

Core ubiquitous language for the SqlFox database management tool.

## Connections & Environment

**Connection**:
A configured target database endpoint containing host, network, protocol, and authentication credentials.
_Avoid_: Profile, host config, server alias

**Environment**:
The operational trust tier assigned to a Connection (`Production`, `Staging`, `Development`) that dictates the enforcement level of the Safety Guard.
_Avoid_: Stage, label, group

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
A protocol-specific database connector (e.g. MySQL, SQLite) implementing lifecycle and session creation.
_Avoid_: Adapter, DB client

**Session**:
An active client interaction context bound to a Connection, managing open tabs, temporary schema cache, and transaction state.
_Avoid_: Workspace, window, console

**RowStream**:
A memory-bounded record sequence streamed from database drivers to the frontend via WebSocket/SSE chunks.
_Avoid_: Result buffer, fetch array

**Out-of-Band Cancellation**:
A query interruption mechanism dispatched via a secondary control connection issuing protocol termination (`KILL QUERY`) without destroying the primary connection.
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

**Internal Notes**:
A persistent in-memory/in-tab Markdown scratchpad (`Notes.md`) allowing developers to accumulate query explanations, execution findings, and AI insights.
_Avoid_: Text file, memo pad, clipboard buffer

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

## Schema Management & Native Dialogs

**SQL Formatter**:
A client-side query beautifier that splits SQL clauses (`SELECT`, `FROM`, `WHERE`, `JOIN`, `GROUP BY`, `ORDER BY`) into indented lines and capitalizes keywords without remote roundtrips.
_Avoid_: Online formatter, backend beautifier

**Native Modal Dialog**:
A non-blocking HTML5 `<dialog>` element providing consistent styling, focus trapping, and Esc key dismissal across the entire application, replacing primitive browser popup functions.
_Avoid_: Alert box, confirm prompt, window.alert, window.confirm

**Table Designer**:
An interactive HeidiSQL-inspired workspace for inspecting and altering table definitions (columns, data types, primary keys, indexes, and foreign keys) with staged change accumulation.
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
A compact HeidiSQL-style header picker for switching, filtering, and managing saved database connection sessions, reserving the sidebar tree exclusively for schema objects.
_Avoid_: Permanent sidebar connection tree accordion

**Schema Search**:
An instant cross-database catalog finder locating tables, views, and columns across all accessible schemas in the active connection.
_Avoid_: Global grep, object finder, quick navigator

**Database Creator**:
A structured modal dialog for provisioning a new database schema on the active target server with dialect-aware character set and collation defaults.
_Avoid_: Raw create prompt, DB wizard

**Execution Plan Grid**:
An interactive, HeidiSQL-inspired tabular display of database engine execution plans (`EXPLAIN`), highlighting access types (Full Table Scan warnings), estimated row counts, and used indexes with a toggleable raw JSON payload view.
_Avoid_: Raw plan dump, text explain drawer

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
A persistent session-level ledger tracking all executed SQL statements with timestamps, execution durations in milliseconds, affected/returned row counts, and error states.
_Avoid_: History tab, query log, command list

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

## Codebase Architecture & Maintenance (Ponytail)

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
