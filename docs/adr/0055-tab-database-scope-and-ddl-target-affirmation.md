# 0055: Tab Database Scope & DDL Target Affirmation Guardrails

## Status
Accepted

## Context
When executing manual DDL mutations (`ALTER TABLE`, `CREATE TABLE`, `DROP`, `TRUNCATE`) in the Query Editor, operators run the risk of modifying the wrong database schema—for instance, intending to execute migrations on database A while the active session context is pointing to database B.

In multi-database environments (especially when connecting with superuser/root privileges that grant access across all schemas on a host), this vulnerability stems from several design gaps:
1. **Global Session Database State**: The active database target was historically tracked globally per connection session rather than per tab. Switching databases in the sidebar inadvertently mutated the target database for all currently open query tabs.
2. **Silent DDL Execution**: Statements like `CREATE TABLE` were evaluated as `RiskNone` in the Safety Guard, bypassing any pre-execution challenge or target verification.
3. **Lack of Target Affirmation**: Operators frequently rely on muscle memory (`Ctrl+Enter`), executing statements without a conspicuous visual confirmation of the destination database immediately preceding statement dispatch.
4. **Schema Scope Mismatches**: Copy-pasting queries with fully qualified table names (`db_b.table_name`) into an editor tab ostensibly targeting `db_a` executed silently without sanity-checking the conflicting schema intents.

## Decisions

### 1. Isolated Tab Database Scope
We isolate database context binding to individual Query Editor tabs:
- **Binding at Tab Creation**: When a new query tab is opened (via `+` or `Ctrl+T`), it inherits the currently active database context and permanently binds it to that tab instance (`tab.database`).
- **Sidebar Decoupling**: Navigating or clicking tables/databases in the sidebar no longer alters the execution target of existing background tabs.
- **Persistence**: Tab-scoped database assignments are persisted across page reloads via `Session Tab State Persistence` (`sqlfox_tabs_<conn_id>`).

### 2. Bidirectional `USE` Synchronization
To preserve natural SQL authoring habits while preventing UI-backend state drift:
- When an operator executes a manual `USE <database>;` statement inside the editor, the Safe Execution Pipeline intercepts the command, shifts the session's active database, and automatically synchronizes the tab's bound `tab.database`.
- The UI triggers an informative non-blocking toast: *"Tab database context switched to <database>"*, instantly updating autocomplete catalogs and toolbar badges.

### 3. Persistent Target Database Pill
We introduce a high-visibility visual anchor placed directly adjacent to the editor's primary execution button (`Run Query`):
- Displays the currently bound database in bold contrast (`[ 🗄️ db_name ]`).
- Features a quick dropdown picker enabling instant tab-level database reassignment without touching the sidebar.

### 4. Schema Scope Mismatch Guard
The deterministic AST Safety Guard is extended to detect cross-database conflicts:
- For any statement referencing qualified identifiers (`<db_target>.<table_name>`), the guard compares `<db_target>` against the tab's bound database.
- If a mismatch is detected (e.g. tab is set to `db_a`, but SQL contains `ALTER TABLE db_b.orders`), execution is intercepted with a prominent **Schema Scope Mismatch** modal asking the operator to explicitly confirm whether they intended to target `db_b` or re-scope the tab.

### 5. Tiered Target Database Affirmation
All DDL operations (`CREATE TABLE`, `ALTER TABLE`, `DROP TABLE/VIEW`, `TRUNCATE`) are subject to mandatory pre-flight target verification:
- **Development Environment**: Displays a 1-Click Target Database Affirmation modal emphasizing the destination database in a high-contrast badge (`Target Database: [ DB_NAME ]`) with an explicit confirmation button (`Execute on <db_name>`).
- **Staging & Production Environments**: Escalates to a mandatory Challenge Modal requiring the operator to explicitly type the target identifier or database name to unlock execution.

### 6. Strict Connection Mode (Database Pinning)
To provide defense-in-depth when operating with superuser/root credentials that possess `ALL PRIVILEGES`:
- Connection profile configuration introduces an optional **Strict Connection Mode** toggle.
- When enabled, the connection session is strictly pinned to a designated database: cross-database switching (`USE`) and cross-database queries referencing external schemas are rejected at the driver gateway.

## Consequences
- Prevents accidental schema corruption and cross-database pollution caused by background tab switching or muscle memory.
- Provides immediate visual clarity regarding where every query will execute directly at the point of action.
- Balances high security on production/staging workloads with low friction on local development via tiered affirmation modals.
- Eliminates the security risk of over-privileged superuser connections inadvertently altering neighbor schemas.
