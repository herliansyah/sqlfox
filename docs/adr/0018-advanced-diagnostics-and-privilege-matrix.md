# 0018: Advanced Diagnostics, Query Profiling, and Privilege Matrix

To deliver comprehensive DBA observability and user management without external dependencies, SqlFox introduces physical storage diagnostics, redundant index detection, query execution profiling, foreign key relational data peeking, and an interactive privilege management matrix.

## Context & Problem

While basic table schema browsing, query execution, and processlist inspection exist, DBAs and developers frequently face operational challenges that previously required switching to external GUI tools (HeidiSQL, Navicat, phpMyAdmin) or writing complex manual administrative queries:
1. **Storage Bloat & Fragmentation**: No physical insight into disk usage per table (`DATA_LENGTH`, `INDEX_LENGTH`, `DATA_FREE`), and no quick access to maintenance operations (`OPTIMIZE TABLE`, `ANALYZE TABLE`, `CHECK TABLE`, or `VACUUM` on SQLite).
2. **Index Inefficiencies**: Shadowed, duplicate, or prefix-overlapping indexes silently consume disk and memory write bandwidth without query planner benefits.
3. **Query Bottleneck Isolation**: `EXPLAIN` gives query plans, but cannot reveal microsecond execution lifecycle phase distribution (system lock, sorting result, temporary table creation on disk).
4. **Relational Context Loss**: The Virtual Data Grid displays raw foreign key values, requiring manual queries to lookup parent row data.
5. **Access Control Overhead**: Managing database users, password credentials, host origin restrictions, and granular permissions requires memorizing intricate MySQL `GRANT`/`REVOKE` syntax.

## Considered Options

- **All-in-One Monolithic DBA Dashboard**: Bundling all diagnostics and user management into a single heavyweight screen. High cognitive load and poor contextual relevance when browsing specific tables or writing queries.
- **CLI-Only Scripts & Manual Guides**: Relying on external shell scripts or manual documentation for optimization and privilege granting. High friction and error-prone.
- **Contextual Diagnostics & Dialect-Aware Security Suite (Chosen)**:
  - **Table Diagnostics**: Dual-seam integration — a database-wide storage overview modal ranking tables by size and fragmentation, alongside a `[Diagnostics]` sub-tab within the Table Designer for individual tables with maintenance action triggers.
  - **Redundant Index Detector**: Heuristic schema analysis identifying exact duplicate indexes, left-prefix shadowed indexes, and redundant primary key indexes, generating safe `ALTER TABLE ... DROP INDEX` migrations routed through the DDL Review Modal and Safety Guard.
  - **Execution Profiler**: Toggleable `⏱️ Profile` action in Query Editor capturing lifecycle stage timings (`SHOW PROFILE` / `performance_schema`), presented with visual duration percentage bars and microsecond timings, with clear fallback notices on SQLite.
  - **Foreign Key Hyperlink & Data Peeking**: Grid cells with FK constraints show a subtle 🔗 indicator on hover. Clicking opens a non-destructive floating preview of the parent row via a dedicated prepared `/peek` endpoint with a 1-click "Open in New Tab" navigation action.
  - **Visual Privilege Matrix**: Native modal dialog for MySQL connections featuring account provisioning (User, Host, Password) and categorized checkbox privilege matrices (Data, Structure, Administration) with quick role presets (`Read Only`, `Read & Write`, `Full Admin`, `Clear All`), dynamically hidden for file-based SQLite databases.

## Decision

1. **Driver & Session Abstractions**:
   - Add `GetTableDiagnostics(ctx, database, table)` and `ExecuteMaintenance(ctx, database, tables, action)` to `driver.Session`.
   - Add `GetRedundantIndexes(ctx, database, table)` performing heuristic AST/schema checks.
   - Extend `ExecuteQuery` / Safe Execution Pipeline to capture execution profile stages when requested via query options.
   - Add `PeekForeignKeyRow(ctx, database, table, column, value)` using parameter-bound queries.
   - Add `GetUserList(ctx)`, `GetUserGrants(ctx, user, host)`, `CreateUser(ctx, ...)`, `DropUser(ctx, ...)`, and `ApplyUserGrants(ctx, ...)` for MySQL driver.
2. **REST API Seams**:
   - `GET /api/session/{id}/diagnostics/tables`: Storage and fragmentation statistics.
   - `POST /api/session/{id}/diagnostics/maintenance`: Maintenance operations (`OPTIMIZE`, `ANALYZE`, `CHECK`, `VACUUM`).
   - `GET /api/session/{id}/diagnostics/redundant-indexes`: Static index redundancy detection.
   - `GET /api/session/{id}/peek`: Prepared parent row preview for FK hyperlinks.
   - `GET /api/session/{id}/users`: User account list.
   - `GET /api/session/{id}/users/grants`: Active privileges for target user.
   - `POST /api/session/{id}/users`: Create or drop user.
   - `POST /api/session/{id}/users/grants`: Apply structured privilege grants.
3. **Safety & Review Integration**:
   - All dropping of redundant indexes and maintenance commands pass through the Safety Guard and DDL Review Modal.
   - Privilege modifications execute within targeted transactions with audit logging.

## Consequences

- Full coverage of SQLyog and HeidiSQL DBA diagnostic capabilities natively within the browser SPA.
- Zero external dependencies added to frontend or backend.
- Safe execution path guaranteed for all administrative mutations.
- Distinct dialect isolation: MySQL-specific administrative tooling gracefully adapts or hides on SQLite connections.
