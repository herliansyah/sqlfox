# 0042: Multi-Engine Feature Parity and Dialect Resilience

To transform SqlFox from a MySQL-centric database manager into a truly resilient, multi-engine operational cockpit across MySQL (5.7, 8.0, 8.4 LTS, 9.x), MariaDB, PostgreSQL, and SQLite, while adhering strictly to the Ponytail principle (zero unnecessary bloat, single binary <100MB RAM budget).

## Context and Problem Statement

1. **MySQL 8.4 LTS / 9.x Compatibility Breakage**:
   Oracle removed legacy syntax `SHOW SLAVE STATUS` and `SHOW MASTER STATUS` in MySQL 8.4 LTS, replacing them exclusively with `SHOW REPLICA STATUS` and `SHOW BINARY LOG STATUS`. Telemetry column names were also renamed (`slave_io_running` -> `replica_io_running`, `seconds_behind_master` -> `seconds_behind_source`). In SqlFox, running Cluster Inspector against MySQL 8.4+ nodes results in hard SQL syntax errors.
2. **MariaDB Telemetry & Lock Wait Fragility**:
   MariaDB shares the `MySQLDriver` implementation, but standard MariaDB installations often omit the `sys` schema and disable `performance_schema` by default (`performance_schema = OFF`). SqlFox's Processlist lock inspection (`sys.innodb_lock_waits`) and Statement Digest query analytics threw fatal errors instead of graceful fallbacks.
3. **PostgreSQL Parity Deficits**:
   While basic CRUD, streaming, and schema inspection worked for PostgreSQL, major feature seams remained unimplemented stubs:
   - Routine Explorer & Caller returned static unsupported comments (`-- Routine DDL not supported`).
   - Statement Digest returned empty slices despite the prevalence of `pg_stat_statements`.
   - Cluster Inspector was reduced to a shallow `pg_is_in_recovery()` check, omitting `pg_stat_replication` and lag diagnostics.
   - User Management in the Web UI was hardcoded to alert that user privileges are exclusively for MySQL/MariaDB.
   - Structure Sync generated MySQL-specific `ALTER TABLE tbl MODIFY COLUMN col` syntax, failing on PostgreSQL which mandates `ALTER TABLE tbl ALTER COLUMN col TYPE ...`.
4. **SQLite Dialect Boundaries**:
   SQLite is an in-process, single-file embedded database lacking user privilege management, background routine execution, and server clustering. Attempting to force identical abstractions led to confusing UI states.

## Considered Options

- **Option 1: Hard 100% Homogeneous Emulation**:
  Force identical synthetic abstractions for missing features across all databases (e.g. creating custom event queues for SQLite/PostgreSQL, creating fake user managers for SQLite).
  *Rejected*: Violates core domain realities, introduces fragile boilerplate, and misleads operators.
- **Option 2: Status Quo with Static Exclusions**:
  Keep PostgreSQL and SQLite features permanently stubbed or disabled with static alert modals.
  *Rejected*: Leaves PostgreSQL as a second-class citizen and fails to support modern MySQL 8.4+ enterprise deployments.
- **Option 3: Pragmatic Engine-Native Capabilities & Adaptive Seams (Selected)**:
  - **Engine Capability Matrix**: Dynamically advertise supported feature sets per driver so the UI cleanly disposes or disables unsupported tabs without dead buttons.
  - **Universal Replication Normalizer**: Implement an intelligent dual-syntax probe in `MySQLSession` supporting `SHOW REPLICA STATUS` / `SHOW BINARY LOG STATUS` with graceful fallback to `SHOW SLAVE STATUS` / `SHOW MASTER STATUS`, mapping legacy and modern column keys into unified `driver.ReplicationInfo`.
  - **MariaDB Diagnostic Gracefulness**: Sniff engine version; if `sys` is missing, fallback to `information_schema.innodb_lock_waits`. If `performance_schema` is disabled, present an informative configuration guide instead of failing.
  - **PostgreSQL All-in Parity**:
    - **Routine Explorer**: Introspect procedures and functions from `pg_proc` with signature-aware labeling (`func_name(type1, type2)`) and extract DDL via `pg_get_functiondef`.
    - **Adaptive Telemetry Probe**: Query `pg_stat_statements` if installed; provide a 1-click helper snippet if uninstalled.
    - **Replication Inspector**: Query `pg_stat_replication` and `pg_stat_wal_receiver` for primary/standby lag telemetry.
    - **Dialect-Aware Role Matrix**: Expose User Management for PostgreSQL using native `ROLE` models (login roles, role attributes, database grants) without forced host wildcarding.
    - **Dialect-Correct Structure Sync**: Generate atomic `ALTER TABLE tbl ALTER COLUMN col TYPE ...`, `SET NOT NULL`, and `SET DEFAULT` statements for PostgreSQL.
  - **Dialect-Aware Value Formatting & Literal Serialization**: Standardize SQL value and binary literal formatting across engines via `FormatSQLValueForDriver` (e.g. `X'...'` for MySQL/SQLite vs `'\x...'::bytea` for PostgreSQL) and proper string escaping (`EscapeSQLString`) for reliable cross-engine export, dump, and audit logs.
  - **Delimiters & Quoting Resilience**: Support custom MySQL `DELIMITER` statements (`SplitWithDelimiters`) and PostgreSQL dollar-quoting (`$$...$$`) in SQL script splitters, preventing routine definitions and complex blocks from being prematurely truncated at standard semicolons.

## Decision

Adopt **Option 3**.

## Consequences

- MySQL 8.4 LTS, 9.x, and MariaDB clusters operate seamlessly with zero syntax failures or hard crashes.
- PostgreSQL gains enterprise feature depth (Routines, Query Analytics, Cluster Telemetry, User Roles, and clean DDL synchronization).
- SQLite interfaces remain clean and lightweight without misleading dead actions.
- Preserves the single-binary zero-external-daemon architecture with strictly bounded memory consumption (<100MB RAM).
