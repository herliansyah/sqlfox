# Specification: PostgreSQL Native Driver Engine & Schema Introspection (Milestone 22)

## 1. Overview & Goal
Implement native PostgreSQL driver support in SqlFox using pure-Go `github.com/jackc/pgx/v5/stdlib`, conforming to `driver.Driver` and `driver.Session` interfaces, enabling full table browsing, keyset streaming, query execution, out-of-band cancellation, and processlist inspection.

## 2. Architecture & Components

### 2.1 Driver Implementation (`internal/driver/postgres/postgres.go`)
- Registers `postgres` driver in `internal/driver/registry.go`.
- Connects using `pgx/v5/stdlib` with DSN assembly supporting SSL modes (`disable`, `require`, `verify-ca`, `verify-full`), user, password, host, port, and database.
- Implements `Session` interface:
  - `QueryStream`: streaming rows with type-safe formatting.
  - `Exec`: executing mutations.
  - `InspectSchema`: high-speed metadata querying via `pg_catalog`.
  - `SelectDatabase`: `SET search_path = <schema>`.
  - `GetTableDDL`: reconstructed `CREATE TABLE` from catalog info.
  - `CancelQuery`: out-of-band execution of `SELECT pg_cancel_backend(pid)`.
  - `GetProcesslist`: querying `pg_stat_activity` with `pg_blocking_pids(pid)`.
  - `KillProcess`: `pg_cancel_backend(pid)` or `pg_terminate_backend(pid)`.

### 2.2 Schema Introspection via `pg_catalog`
- Tables & Views: `pg_catalog.pg_class` filtered by namespace.
- Columns & Types: `pg_catalog.pg_attribute` joined with `pg_type`.
- Primary & Foreign Keys: `pg_catalog.pg_constraint`.
- Indexes: `pg_catalog.pg_index` joined with `pg_class`.

### 2.3 Web UI & Headless CLI Integration
- UI Connection Modal: add `PostgreSQL` to driver dropdown with default port `5432`.
- CLI Auto-Detection: `postgres://` or `postgresql://` URI schemes auto-resolve to postgres driver.
