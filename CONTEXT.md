# SqlFox Domain Glossary

Core ubiquitous language for the SqlFox database management tool.

## Connections & Environment

**Connection**:
A configured target database endpoint containing host, network, protocol (direct or SSH tunnel), and authentication credentials.
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
