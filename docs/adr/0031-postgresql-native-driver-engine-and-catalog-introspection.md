# 0031: PostgreSQL Native Driver Engine and Catalog Introspection

## Status
Accepted

## Context
SqlFox initially supported MySQL/MariaDB and SQLite. However, PostgreSQL is the primary relational database for modern application development. Supporting PostgreSQL unlocks a major demographic of developers, DBAs, and CI/CD pipelines.

Adding PostgreSQL introduces architectural distinctions from MySQL:
1. **Connection Scope**: In PostgreSQL, connections are permanently pinned to a physical database. `USE db_name` does not exist; namespace separation within a database is achieved via schemas (`search_path`).
2. **Catalog Metadata**: ANSI `information_schema` is notoriously slow on PostgreSQL schemas with numerous tables. System catalog tables (`pg_catalog`) provide 10x-50x faster introspection.
3. **Out-of-Band Cancellation**: PostgreSQL queries execute in dedicated backend processes identifiable via `pg_backend_pid()`. Interrupting long-running queries out-of-band requires invoking `pg_cancel_backend(pid)` via a separate control session.
4. **Safety Guard Grammar**: Dialect differences (`$1`, `RETURNING`, `ILIKE`, `::text`) can trigger syntax errors in MySQL-specific AST parsers.

## Decisions

### 1. Zero-CGO Native Driver (`pgx/v5/stdlib`)
We integrate `github.com/jackc/pgx/v5/stdlib`. It is pure Go, actively maintained, supports PostgreSQL 9.6 through 16+, and implements the standard `database/sql` driver interface.

### 2. Physical Database Binding & Schema Search Path Multi-Tenancy
- The Vault `ConnectionRecord` specifies the physical target database (`DatabaseName`).
- The session UI dropdown lists available PostgreSQL schemas (`public`, etc.) queried from `pg_catalog.pg_namespace`.
- Selecting a schema executes `SET search_path = <schema>`, maintaining full UI parity with MySQL's database switcher without re-dialing the connection pool.

### 3. High-Performance Introspection via `pg_catalog`
All schema metadata (tables, views, columns, native types, primary keys, foreign keys, and indexes) is introspected directly from `pg_catalog` (`pg_class`, `pg_attribute`, `pg_type`, `pg_constraint`, `pg_index`, `pg_description`). This keeps metadata retrieval fast (<50ms) and avoids `information_schema` join overhead.

### 4. Out-of-Band Query Cancellation with Backend PID Tracking
- Each query execution records the backend PID obtained from `SELECT pg_backend_pid()`.
- Cancellation requests (`CancelQuery`) invoke `SELECT pg_cancel_backend(pid)` through an auxiliary control connection.
- `GetProcesslist()` queries `pg_stat_activity` and computes root blockers using the native `pg_blocking_pids(pid)` array function.
- Process termination invokes `SELECT pg_terminate_backend(pid)`.

### 5. Standardized JSON Serialization for TanStack Virtual Grid
`RowStream` serializes native PostgreSQL types cleanly for browser consumption:
- `UUID`: String
- `JSON` / `JSONB`: JSON string
- Arrays (`text[]`, `int[]`, etc.): JSON arrays `[...]`
- `BYTEA`: Hex formatted `\x...`
- `TIMESTAMPTZ`: RFC3339 / ISO-8601 string
- `BOOLEAN`: Native boolean

### 6. Fail-Closed Dialect Fallback for Safety Guard
If the primary AST parser rejects a query due to Postgres-specific grammar, a fast token-stream scanner inspects the query for destructive patterns (unqualified `DELETE`/`UPDATE` without `WHERE`, `DROP`, `TRUNCATE`) to ensure safety enforcement remains deterministic and fail-closed.

## Consequences
- SqlFox natively manages PostgreSQL instances with zero external binary or CGO dependencies.
- Retains memory footprint ceiling (<100MB RAM) and sub-second launch time.
- Preserves full feature parity across Query Editor, Virtual Data Grid, Processlist Manager, Lock Dependency Tree, ERD Visualizer, and Headless CLI.
