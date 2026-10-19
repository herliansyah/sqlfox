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

**Master PIN Masking**:
A shoulder-surfing privacy guard applied to the session header Master PIN badge, masking sensitive digits by default (`fox-••••`) with an interactive peek toggle button (`👁️`).
_Avoid_: PIN hide, password star, secret mask

**Port Hunting**:
An automated sequential port probing mechanism that resolves `EADDRINUSE` port collision errors by scanning subsequent free ports.
_Avoid_: Port hopping, auto-port

**Network Endpoint**:
A non-loopback IPv4 network address of the host machine accessible from other devices on the local area network.
_Avoid_: Public URL, remote host

**Secure-by-Default Binding**:
The runtime policy restricting the daemon's HTTP listener strictly to the local loopback interface (`127.0.0.1`) unless the operator explicitly permits external access via the `--lan` parameter.
_Avoid_: Localhost lock, private bind, internal port

**Remote Lockout Policy**:
The automated rate-limiting quarantine in the Remote Access Guard that temporarily blocks authentication attempts from a remote IP address following 5 consecutive failed Master PIN entries for a 10-minute window.
_Avoid_: IP ban, firewall block, auth cooldown

**DNS Rebinding Guard**:
The ingress validation gate verifying incoming HTTP `Host` headers against approved loopback and explicitly configured network addresses to prevent unauthorized cross-origin requests from browser environments.
_Avoid_: Host filter, domain check, origin gate

**Strict Connection Mode**:
A Connection configuration policy that pins an active session strictly to a single target database, rejecting cross-database queries or DDL mutations even when authenticated as a superuser.
_Avoid_: Locked connection, single DB profile, pinned vault entry

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

**Core Driver Scope**:
The policy restricting SqlFox's database engine support to the core relational trio (MySQL/MariaDB, PostgreSQL, and SQLite) to maximize operational feature depth, query tuning parity, and zero-defect maintenance.
_Avoid_: Database list, supported DBs, connection types

**CGO-Free Invariant**:
The non-negotiable architectural rule mandating that all driver integrations and server runtime binaries compile with `CGO_ENABLED=0`, rejecting any external database engine requiring local C toolchains.
_Avoid_: Pure Go rule, no-C policy, static build restriction

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

**Filter Pushdown Boundary**:
The security and validation perimeter in the Virtual Grid that verifies table quick-filter expressions using a lightweight token regex guard before dispatching them into the server-side `WHERE` clause under an Implicit Safe Limit.
_Avoid_: Grid filter validator, where clause sanitization, filter parser

**Release Manifest Verification**:
The cryptographic supply-chain verification policy requiring matching SHA-256 digests against an official release manifest before in-app self-update binaries can be unpacked or executed.
_Avoid_: Checksum test, hash check, download verify

**Lexical Masking**:
The token-sanitization phase in the Safety Guard (`maskCommentsAndLiterals`) that blanks out comments (`--`, `#`, `/* ... */`), quoted string literals, and quoted identifiers before keyword regex evaluation, preventing SQL keywords hidden in comments or strings from triggering false positives or concealing destructive clauses.
_Avoid_: Comment stripper, SQL cleaner

**Multi-Statement Inspection Gate**:
The safety evaluation policy (`inspectAllStatements`) that parses and evaluates every statement within a compound query batch individually, ensuring that trailing destructive operations (e.g. `DROP` or `DELETE` appended after a `SELECT`) cannot evade inspection, while failing closed if multiple statements demand confirmation challenges.
_Avoid_: Multi-query validator, compound gate

**CTE Clause Stripping**:
A syntax preprocessing step (`stripLeadingCTE`) that isolates and removes leading Common Table Expression (`WITH ... AS (...)`) definitions to evaluate the root executable operation (`SELECT`, `INSERT`, `UPDATE`, `DELETE`, `MERGE`) under the Safety Guard.
_Avoid_: CTE cleaner, WITH handler

**Target Database Affirmation**:
A dedicated visual confirmation gate inside the Challenge Modal and pre-DDL review presenting the resolved target database in prominent contrast, requiring operator acknowledgement or manual typing before executing schema modifications.
_Avoid_: DDL confirm, database check prompt, target alert

**Schema Scope Mismatch Guard**:
An AST analysis check in the Safety Guard that detects discrepancies between a tab's bound Tab Database Scope and an explicitly qualified database prefix in a SQL statement, intercepting accidental cross-database executions.
_Avoid_: Cross-DB validator, schema mismatch warning

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

**Operational Note**:
A local Vault-persisted record capturing database events, query findings, and runbook rules with object tagging, reminders, and AI mention capabilities.
_Avoid_: Memo, post-it, scratch text

**Notes Inbox**:
The centralized operational hub in SqlFox for reviewing, searching, pinning, and triaging follow-up notes across all connections.
_Avoid_: Notes list, message center, todo drawer

**Object Tag**:
A soft-link URI reference (`db://connection/database/object`) binding an Operational Note to database entities (tables, views, connections) without server-side schema mutation.
_Avoid_: Schema pointer, table link, entity binder

**Follow-up Reminder**:
A scheduled timestamp on an Operational Note that alerts the user via in-app notifications and desktop alerts for pending investigations.
_Avoid_: Clock alarm, cron event, time trigger

**Notes Markdown Live Preview**:
A segmented mode toggle (`Edit` / `Preview`) within notes editing surfaces that compiles and renders Markdown formatted text, tables, and code blocks in real time.
_Avoid_: Notes viewer, markdown tab

**Query Snapshot Capture**:
The action of capturing active SQL text from a Query Editor tab into a new local Vault Operational Note with soft-link object tags bound to the active database connection.
_Avoid_: Save tab as note, export query memo, tab backup

**Note Query Extraction**:
The action of extracting runnable SQL from an Operational Note's snapshot content and loading it into a Query Editor tab bound to the tagged database context.
_Avoid_: Run note, execute memo, import note to tab

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

**Fast Catalog Introspection**:
A lightweight, single-sweep database schema inspection mechanism (`InspectFastCatalog`) that extracts table names and column definitions across all user schemas without querying heavy foreign key constraints (`KEY_COLUMN_USAGE`) or index structures, preventing metadata lock contention during global autocomplete synchronization.
_Avoid_: Fast dump, quick schema scan, shallow schema

**Schema Cache Pre-Flight Dialog**:
An interactive pre-execution safeguard (`#refresh-all-dialog`) intercepting global schema cache refreshes, detailing performance benefits (cross-DB autocomplete, global search, zero FK locks), probing active server thread saturation (`ActiveThreads`), and warning against synchronization under heavy production query load.
_Avoid_: Refresh popup, cache confirm, sync alert

**Schema Cache Timestamp Badge**:
A status badge in the schema header displaying the last synchronization time of the cached database catalog (`#schema-cache-badge`).
_Avoid_: Cache clock, sync tag, schema age

**Context-Aware Completion Provider**:
An AST-lite SQL tokenizer and suggestion engine that dynamically filters completion candidates based on caret position (e.g. table names after `FROM`/`JOIN`, columns after `SELECT`/`WHERE`, and alias resolution `alias.column`).
_Avoid_: Word predictor, auto-suggester, flat matcher

**Completion Item**:
A ranked suggestion candidate with display label, kind badge (`[KW]`, `[TBL]`, `[COL]`, `[FN]`), and replacement text.
_Avoid_: Autocomplete option, hint token

**Session Tab State Persistence**:
A client-side storage mechanism bound per Connection (`sqlfox_tabs_<conn_id>`) that continuously synchronizes open Query Editor tabs, SQL draft buffers, active tab focus, and Table Viewer metadata across page reloads.
_Avoid_: Tab autosave, tab cache, session restore

**Tab Context Menu**:
A secondary-click context menu attached to workspace tabs providing granular and batch tab lifecycle actions (`Close`, `Close Others`, `Close Tabs to the Right`, `Close All`, `Duplicate Tab`, `Rename Tab`).
_Avoid_: Tab menu, tab popup, right-click actions

**Tab Close Confirmation Guard**:
An interactive pre-closure safeguard powered by the Native Modal Dialog that intercepts single and bulk tab closing actions when dirty SQL draft buffers or active states are present, preventing accidental work loss.
_Avoid_: Tab alert, close warning, exit prompt

**Workspace Pane Splitter**:
An interactive, draggable horizontal divider separating the Query Editor and Virtual Data Grid, providing flexible vertical space allocation, boundary constraints (min 80px editor, min 120px grid), double-click 50:50 reset, and global workspace persistence.
_Avoid_: Divider bar, resizer line, pane separator, window split


**Tab Duplication**:
An action creating an immediate clone of an existing workspace tab directly adjacent to the source tab, replicating its exact SQL draft buffer or view state into a fresh tab labeled `[Title] (Copy)`.
_Avoid_: Copy tab, clone tab, tab fork

**Tab Rename**:
An action enabling users to customize workspace tab titles for organizing complex multi-tab query workflows.
_Avoid_: Tab title editor, change tab label

**Editor Syntax Theme**:
The curated, high-contrast dark-mode syntax color palette governing CodeMirror token styling in the Query Editor, ensuring optimal legibility on dark backgrounds (`#090d16`) by mapping backtick table/database identifiers (`.cm-variable-2`) to Sky-300 cyan (`#7dd3fc`), data types (`.cm-type`) to Violet (`#c084fc`), constants/atoms (`.cm-atom`) to Coral (`#fb923c`), and matching brackets to glowing cyan highlights.
_Avoid_: Custom theme picker, external CSS theme, light theme fallback

**Tab Database Scope**:
The explicit, isolated database context bound to an individual Query Editor tab, decoupled from global sidebar navigation and synchronized with `USE` statements.
_Avoid_: Global database state, active tab schema, session DB pointer

**Target Database Pill**:
The persistent visual status pill rendered adjacent to the Query Editor execution controls displaying the tab's currently bound target database (`[ 🗄️ db_name ]`).
_Avoid_: DB badge, header label, current schema indicator

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
A compact header picker for switching, filtering, and managing saved database connection sessions, reserving the sidebar tree exclusively for schema objects. Header labels are strictly reserved for section identification and stripped of redundant action triggers (`+ New`), hosting session creation canonically inside the popover search toolbar (`➕`) and Command Palette.
_Avoid_: Permanent sidebar connection tree accordion, redundant header action buttons

**Schema Search**:
An instant cross-database catalog finder locating tables, views, and columns across all accessible schemas in the active connection.
_Avoid_: Global grep, object finder, quick navigator

**Database Creator**:
A structured modal dialog for provisioning a new database schema on the active target server with dialect-aware character set and collation defaults.
_Avoid_: Raw create prompt, DB wizard

**Routines Explorer**:
Collapsible sidebar schema tree sections introspecting and categorizing Stored Procedures, Stored Functions, and Scheduled Events with DDL inspection and quick caller generation.
_Avoid_: Routine tree, proc list, function manager

**Smart Routine Caller**:
An automated query generator formulating contextual `CALL sp_name(...)` invocation templates into a fresh Query Editor tab with typed parameter comments and OUT-variable extraction statements.
_Avoid_: Param wizard, procedure runner form, call dialog

**Table Partitions Inspector**:
A dedicated telemetry sub-tab in the Table Designer detailing partition schemes, expressions, value ranges, estimated row counts, and physical storage distribution from `information_schema.partitions`.
_Avoid_: Partition manager, partitioner, segment list

**JSON Cell Viewer Modal**:
A focused native HTML5 modal dialog providing formatted, syntax-highlighted indentation, copy-to-clipboard, and structure validation for large JSON documents in Virtual Data Grid cells.
_Avoid_: JSON cell expander, raw json popup, text viewer

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
A searchable popover dropdown in the sidebar for filtering and switching the active database schema in multi-database server connections, matching the Session Switcher interaction pattern. Stripped of redundant header buttons (`+ New`, `Inspect`), keeping schema creation canonically inside its popover toolbar (`➕`) and storage diagnostics in the Top Monitor menu and Command Palette.
_Avoid_: Raw select dropdown, static DB selector, cluttered header label actions

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
A synthesized parameter map and deterministic WHERE predicate (`singleRowWhere`) constructed from all non-null column values of an existing row, narrowed to match at most one physical row (`LIMIT 1` for MySQL, `rowid IN (...)` for SQLite, `(tableoid, ctid) IN (...)` for PostgreSQL) to ensure safe mutations on tables lacking a formal primary key without altering identical duplicate rows.
_Avoid_: Synthetic PK, pseudo-key, blind update

**Composite Primary Key Sync**:
The multi-column reconciliation algorithm in Data Sync that serializes composite keys using an unambiguous unit separator (`\x1f`), maps source column indices to target positions, and synthesizes multi-clause predicates (`WHERE pk1 = ? AND pk2 = ?`) for updates and deletes.
_Avoid_: Multi-key sync, combined key matcher

**Canonical Sync Key**:
A normalized data representation (`syncValueKey`) used during cross-driver row comparisons that reconciles type variations (such as `int64`, string numbers, and raw byte arrays) and reliably distinguishes `NULL` (`\x00NULL`) from empty string or text values.
_Avoid_: Value normalizer, diff hash

**Data Sync Comparison Ceiling**:
A memory protection threshold (e.g., 10,000 rows) that prevents memory exhaustion by rejecting row-level data diffing on oversized tables with an HTTP 422 Unprocessable Entity error.
_Avoid_: Row limit, sync cap

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

**Processlist Batch Action Bar**:
An ephemeral floating action dock in the Processlist Manager triggered when one or more non-immune threads are selected, providing bulk operational commands (`Batch Kill Query`, `Batch Kill Connection`, `Copy IDs`, `Export`, `Save to Notes`) and selection counters without layout shift.
_Avoid_: Bulk action bar, multi-select toolbar, process action dock

**Protected Process Thread**:
A database thread immune to client selection and termination within SqlFox, specifically the user's active session connection (`isSelf`) and essential database system/replication threads (`isSystem`: `system user`, `event_scheduler`, `Binlog Dump`).
_Avoid_: Immune thread, system process, locked thread

**Batch Termination Guardrail Modal**:
A dedicated high-safety confirmation dialog summarizing targeted threads (IDs, users, durations, SQL snippets) before executing multi-thread terminations, enforcing an explicit challenge input in Production environments or when terminating multiple sessions.
_Avoid_: Batch kill popup, confirmation dialog, kill alert

**Process Activity Trend Panel**:
An ephemeral, collapsible in-memory rolling time-series visualization in the Processlist Manager rendering thread concurrency (`Threads Running` vs `Total Connected`) and operational metrics (`Now`, `Peak`, `Avg`) on native HTML5 Canvas without incurring background persistence or additional database queries.
_Avoid_: Process chart, query monitor graph, live APM widget

**Processlist Primary Toolbar**:
The top-level operational navigation bar in the Processlist Manager hosting the view switcher (`Active Threads`, `Statement Digest`, `Deadlock History`), live thread summary pill (`Threads: N`), root blocker alert badge (`🚨 X Locks`), and global telemetry controls (auto-refresh intervals, manual refresh, and trend toggle).
_Avoid_: Process header, processlist topbar, thread nav bar

**Processlist Filter Subtoolbar**:
The contextual secondary toolbar positioned directly beneath the primary toolbar in the Processlist Manager dedicated to the Active Threads view, housing quick filter chips (`All`, `Active`, `Locked`, `Slow`, `Sleep`), flexible multi-field search input, sleep/system visibility toggles, and slow-query execution threshold controls.
_Avoid_: Process filter bar, thread search line, sub-header filter

**Processlist Column Visibility Picker**:
An ephemeral popover dropdown in the Processlist Filter Subtoolbar allowing users to customize visible table columns (`Action`, `User`, `Host`, `Database`, `Command`, `Time`, `State`), enforcing strict immunity on essential identifier columns (`Select`, `ID`, `SQL / Query Info`), with persistent configuration stored in browser `localStorage`.
_Avoid_: Column manager, processlist column chooser, hide column menu

**Lock Contention & Root Blocker Inspector**:
A dedicated zero-shift incident-triage modal accessible from the Top Monitor menu, Command Palette, and Processlist Manager toolbar displaying the Root Blocker dependency tree, surfacing blocking queries, locked resources, and single-click termination actions without incurring layout shifts on the active threads table.
_Avoid_: Lock viewer, deadlock popup, blocker window

**Root Blocker Incident Triage**:
The elevated visual diagnostic card in the Lock Contention Inspector displaying root blocking transactions with prominent status badging (`🚨 ROOT BLOCKER`), scrollable full SQL queries (`max-h-48 overflow-y-auto`), dedicated mitigation buttons (`Kill Query` vs `Kill Blocker`), and waiting session duration metrics.
_Avoid_: Blocker card, kill widget, blocker box

**Deadlock Inspector**:
A dedicated post-mortem diagnostic parser within the Processlist Manager analyzing `SHOW ENGINE INNODB STATUS` (`LATEST DETECTED DEADLOCK`) to isolate victim transactions, holding locks, waiting locks, and conflicting SQL statements.
_Avoid_: Deadlock alert, lock crash log, engine parser

**Statement Digest Dashboard**:
A zero-daemon analytical view aggregating normalized query executions, cumulative latencies, rows examined vs sent, and full table scans from `sys.statement_analysis` or `performance_schema.events_statements_summary_by_digest` with Challenge-guarded statistics reset.
_Avoid_: Slow log viewer, query profiler, top sql list

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

**Non-Wrapping Controls**:
The design and styling invariant enforcing `white-space: nowrap !important;` and `flex-shrink: 0;` on all interactive buttons, toolbar icons, badge pills, and tab headers, preventing distorted pill dimensions and multi-line label collapse during viewport resizing.
_Avoid_: Multiline button labels, squished button shapes, colliding controls

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
- **0041**: MySQL Enterprise Operations & Programmability Suite (`docs/adr/0041-mysql-enterprise-operations-and-programmability-suite.md`)
- **0042**: Multi-Engine Feature Parity & Dialect Resilience (`docs/adr/0042-multi-engine-feature-parity-and-dialect-resilience.md`)
- **0043**: Diagnostic Logger and Observability Suite (`docs/adr/0043-diagnostic-logger-and-observability-suite.md`)
- **0044**: Unified Operational Notes Inbox and Follow-up Lifecycle (`docs/adr/0044-unified-operational-notes-inbox-and-followup-lifecycle.md`)
- **0045**: Core Engine Scope Freeze and CGO-Free Invariant (`docs/adr/0045-core-engine-scope-freeze-and-cgo-free-invariant.md`)
- **0046**: Secure-by-Default Binding and Ingress Hardening (`docs/adr/0046-secure-default-binding-and-ingress-hardening.md`)
- **0047**: Single-Instance Guard and Lock Management (`docs/adr/0047-single-instance-guard-and-lock-management.md`)
- **0048**: UI Resilience and Non-Wrapping Controls (`docs/adr/0048-ui-resilience-and-non-wrapping-controls.md`)
- **0049**: Containerized Deployment and Multi-Arch Docker Distribution (`docs/adr/0049-containerized-deployment-and-multi-arch-docker-distribution.md`)
- **0050**: Fast Schema Catalog Synchronization and Root Blocker Incident Triage (`docs/adr/0050-fast-catalog-synchronization-and-root-blocker-triage.md`)
- **0051**: In-App Activity Console and Live Event Stream (`docs/adr/0051-in-app-activity-console-and-live-event-stream.md`)

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

**Terminal Startup Banner**:
An ANSI-accented terminal status card rendered upon interactive daemon launch displaying the fox mascot, active network endpoints (Local / LAN), master PIN security status, vault path, and core database driver availability.
_Avoid_: Splash screen, CLI intro, boot logo

**CLI Hero Header**:
A compact stylized ANSI title banner displayed at the head of interactive `sqlfox help` output establishing product identity without obstructing command reference readability.
_Avoid_: Help splash, console banner

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

**Execution Plan Normalizer**:
The metadata extraction component in `internal/advisor/` that transforms heterogeneous database execution plan outputs (MySQL tabular/JSON `type: ALL`, PostgreSQL JSON tree `"Seq Scan"`, and SQLite `SCAN table`) into unified `ExplainTableHint` telemetry.
_Avoid_: Plan parser, explain adapter, query translator

## Distribution & Workspace Attribution

**Update Engine**:
The native Go subsystem (`internal/server/update.go`) responsible for polling GitHub Releases, evaluating semver version deltas, streaming platform-specific binary archives (`.tar.gz` / `.zip`), performing atomic binary replacement on disk, and triggering graceful application re-execution without third-party dependencies.
_Avoid_: Auto-installer, updater daemon, external package manager

**Update Modal**:
A native HTML5 dialog (`#update-dialog`) presenting release metadata, version diffs (`vCurrent` -> `vLatest`), rendered Markdown changelog release notes, and real-time self-update progress states.
_Avoid_: Update popup, new version alert, release prompt

**Status Bar Footer**:
The persistent bottom viewport bar (`<footer>`) structured into two distinct regions: real-time database runtime telemetry on the left (dialect engine flavor & version, connection target, server uptime, interactive ping latency), and system heap memory consumption, documentation link, keyboard shortcuts modal launcher, and application version badge with self-update notifications on the right.
_Avoid_: Bottom bar, taskbar, dock, personal branding in workspace chrome

**Author Credit**:
The creator and repository attribution residing elegantly inside the Changelog / About dialog footer (`#changelog-dialog`), featuring creator credit (`Created by herliansyah`) linked to the author's GitHub profile alongside the official repository link, keeping the primary workspace status bar completely clean and distraction-free.
_Avoid_: Footer branding in status bar, watermark, sponsor banner, donation nag, personal promo in main workspace

**Changelog**:
The canonical, structured human-readable document (`CHANGELOG.md`) tracking notable application changes across releases, formatted strictly under the Keep a Changelog standard and Semantic Versioning.
_Avoid_: Release notes file, update log, commit digest

**Release Routine**:
The deterministic, script-driven protocol (`scripts/release.sh`) for cutting a release: running test pre-flights, advancing `[Unreleased]` changes to a dated semver heading, bumping code version constants, committing, creating annotated git tags, and triggering multi-platform CI release builds.
_Avoid_: Manual tag, ad-hoc release, unversioned bump

**Changelog Viewer**:
The native HTML5 modal dialog (`#changelog-dialog`) presenting complete application history and offline-accessible release logs rendered from embedded markdown, accessible directly from the footer version element and Command Palette.
_Avoid_: Release popup, changelog window, version toast

**Containerized Deployment**:
The operational execution pattern of SqlFox packaged as a minimal, non-root multi-arch container image (`ghcr.io/herliansyah/sqlfox`), binding the HTTP server to `0.0.0.0` and consolidating persistent state (`config.db`, `master.key`, backups) under a dedicated `/data` volume.
_Avoid_: Docker app, virtual machine, container bundle

**Host Gateway**:
The network bridge mapping (`host.docker.internal:host-gateway`) enabling a containerized SqlFox instance to connect directly to databases hosted natively on the container host machine.
_Avoid_: Localhost forwarder, host proxy, loopback redirect

## Brand Identity & Application Icons

**Brand Mark**:
The canonical modern geometric tech fox head emblem in vector SVG format embodying SqlFox's identity, rendered in solid brand orange (`#ea580c`) with sharp minimalist contours.
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

## Documentation & Operator Resources

**User Guide**:
The comprehensive operator manual and operational playbook (`docs/USER_GUIDE.md` in English and `docs/USER_GUIDE_ID.md` in Bahasa Indonesia) serving as the authoritative reference across all experience levels, from developers to senior database administrators.
_Avoid_: Wiki, help doc, manual pdf, readme clone

**Documentation Link**:
The persistent external hyperlink affordance (`📖 Docs`) embedded within the Status Bar Footer, Keyboard Shortcuts Modal, and Command Palette routing operators directly to the canonical GitHub-hosted User Guide.
_Avoid_: Help button, doc url, external link

## Multi-Engine Parity & Dialect Capabilities

**Engine Capability Matrix**:
The adaptive feature contract dynamically reporting supported capabilities per active database dialect (e.g. SQLite has no user management or routines; PostgreSQL has functions but no scheduled events; MySQL has native event scheduler). Governs UI visibility to gracefully disable or hide unsupported tabs without dead buttons.
_Avoid_: Static driver check, hardcoded menu switches, feature hack

**Universal Replication Normalizer**:
The driver abstraction seam absorbing version-breaking telemetry terminology across MySQL generations (e.g. normalizing MySQL 8.4+ `SHOW REPLICA STATUS` / `SHOW BINARY LOG STATUS` and `replica_*`/`source_*` fields with legacy MySQL 5.7/8.0 `SHOW SLAVE STATUS` / `SHOW MASTER STATUS` and `slave_*`/`master_*` fields) into a unified `driver.ReplicationInfo`.
_Avoid_: Ad-hoc version regex, duplicate replication structs

**Adaptive Telemetry Probe**:
A non-fatal diagnostic interrogation pattern for optional server features (such as `pg_stat_statements` on PostgreSQL or `performance_schema` / `sys` on MariaDB). If enabled, delivers full workload metrics; if uninstalled or disabled, presents contextual 1-click helper snippets and configuration guidance rather than hard application errors.
_Avoid_: Silent failure, raw query error, mock stub

**Dialect-Aware Role Matrix**:
The security management domain model adapting to engine-specific authorization paradigms: displaying `'user'@'host'` and privilege hierarchies on MySQL/MariaDB, while mapping to login roles, role attributes (`SUPERUSER`, `CREATEDB`, etc.), and database grants on PostgreSQL where IP filtering is governed by `pg_hba.conf`.
_Avoid_: MySQL-only user manager, forced host wildcarding

## Observability & Diagnostic Logging

**Diagnostic Logger**:
The structured Go standard library (`log/slog`) telemetry coordinator governing runtime verbosity tiers (`debug`, `info`, `warn`, `error`, `silent`). Emits machine-readable JSON or human-friendly text formats without external logging libraries.
_Avoid_: Print logger, raw stdout, debug print

**Log Sink**:
The destination pipeline receiving structured log events, configurable as terminal console, isolated file append stream, or concurrent dual-stream (Tee) with silent terminal override.
_Avoid_: Log target, output dump, print destination

**Credential Masking**:
The deterministic boundary filter intercepting log messages and attributes to sanitize Master Passwords, PINs, database connection secrets, and authorization tokens into `[REDACTED]` prior to emission.
_Avoid_: Secret hiding, raw dump, password censor

**Live Event Stream**:
The server-sent event (`/api/logs/stream`) subscription delivering structured runtime telemetry directly from the Diagnostic Logger to connected web sessions in real time.
_Avoid_: Log socket, polling feed, stdout pipe

**Activity Console**:
The collapsible bottom-docked diagnostic drawer in the user interface rendering streaming application events in an interactive, filterable, pausable, and exportable running table.
_Avoid_: Log window, debug popup, terminal tab

**Telemetry Ring Buffer**:
An in-memory fixed-capacity ring buffer capturing the latest 100 diagnostic log records to provide immediate operational context upon client connection.
_Avoid_: Memory dump, unconstrained queue

**Smart Auto-Scroll**:
The viewport tracking behavior in the Activity Console that automatically pauses auto-scrolling when the operator scrolls upward to inspect historical entries and resumes when scrolled to the newest record.
_Avoid_: Forced scroll, jump lock




## Cloud Replication & Portability

**Cloud Sync**:
The background replication mechanism that synchronizes notes, snippets, and encrypted vault archives to a remote Git repository via the GitHub REST API without requiring external git CLI dependencies.
_Avoid_: Git backup, cloud upload, drive sync

**Offline-First Replication**:
The architectural boundary where local SQLite persistence is authoritative and immediate, with cloud synchronization acting as an asynchronous, non-blocking downstream replica.
_Avoid_: Local cache, write-through

**Sync Status**:
The operational state indicator of the background replication queue (`Synced`, `Syncing`, `Pending`, `Error`) surfaced on the UI without interrupting editing workflows.
_Avoid_: Upload status, git state

**Notes Bundle**:
A portable, unencrypted ZIP archive containing human-readable Markdown notes and SQL snippets for offline reading and cross-tool export when GitHub synchronization is not configured.
_Avoid_: Note dump, export zip
