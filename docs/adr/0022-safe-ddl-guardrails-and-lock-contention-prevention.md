# 0022: Safe DDL Guardrails and Lock Contention Prevention for Large-Scale Databases

To protect high-volume production databases (e.g. 500GB+ tables and high-concurrency OLTP engines) from catastrophic table locks, Metadata Lock (MDL) queues, and transaction deadlocks during DDL alterations, SqlFox introduces engine-extensible Safe DDL Guardrails and Pre-Flight Lock Inspection.

## Context and Problem Statement

On relational database engines (MySQL 5.7/8.x, MariaDB, and future engines like PostgreSQL):
1. **Metadata Lock (MDL) Stalls**: Executing `ALTER TABLE` requires exclusive metadata locks during the prepare and commit phases. Even on small lookup/master tables, if a long-running transaction or query touches the table—or joins it to a massive multi-gigabyte transaction table via foreign keys—the `ALTER TABLE` stalls waiting for locks.
2. **Cascading Pool Exhaustion**: Once an `ALTER TABLE` enters the lock wait queue, database engines queue all subsequent read and write queries behind the DDL request. Because MySQL's default `lock_wait_timeout` is 31,536,000 seconds (1 year), an unmonitored DDL causes complete application connection pool exhaustion in seconds.
3. **Master vs. Transaction Table Blind-Spot**: Developers frequently assume changing a 10-row master/status table is harmless. However, if that master table is referenced by a 500GB transactional table (`orders`, `audit_logs`) actively serving OLTP transactions, the DDL acquires shared/exclusive locks across foreign key relationships, precipitating severe deadlocks.
4. **Table Rebuild Ceiling**: Certain column mutations (type conversions, engine limitations) cannot execute via `ALGORITHM=INSTANT` or `LOCK=NONE`, necessitating online schema migration tools (`gh-ost`, `pt-online-schema-change`) to prevent multi-hour table locks.

## Considered Options

- **Option 1: Unrestricted DDL Execution**: Run user DDL directly over existing pooled connections with default server timeouts. Rejected: causes uncontrollable production outages and deadlock cascades.
- **Option 2: Total DDL Prohibition on Large Tables**: Reject any DDL on tables over a fixed size threshold. Rejected: overly restrictive, prevents legitimate emergency schema fixes, and fails to protect small master tables referenced by large child tables.
- **Option 3: Extensible Safe DDL Guardrails with Pre-Flight Lock Inspection and Fail-Fast Timeouts**:
  - Implement a uniform driver seam (`InspectTableLocks` and `ExecSafeDDL`) implemented for MySQL 8, MySQL 5.7, MariaDB, and SQLite, structured for future drivers (PostgreSQL, Oracle).
  - Pre-Flight Lock Inspection: Interrogate active transactions and locks touching target tables before execution, displaying blocking queries with out-of-band cancellation options in the Challenge Modal.
  - Schema Relationship Introspection: Scan foreign key graphs (inbound and outbound) to warn users when altering tables referenced by large transactional tables.
  - Fail-Fast Dedicated Connection: Execute DDL through isolated `*sql.Conn` instances with a strict session lock timeout (default 3 seconds), closing/resetting the connection immediately to preserve connection pool hygiene.
  - Dual-Preset Online Schema Change (OSC) Generator: Provide ready-to-run `gh-ost` and `pt-online-schema-change` commands when non-instant table rebuilds are required.

## Decision

Adopt **Option 3**.

### Architecture & Driver Seam

1. **Unified Driver Interface**:
   - `InspectTableLocks(ctx context.Context, database, table string) ([]TableLockInfo, error)`
   - `ExecSafeDDL(ctx context.Context, sql string, lockTimeoutSec int) (*ExecResult, error)`
2. **Current Supported Engines**:
   - **MySQL 8.x & MySQL 5.7 & MariaDB**: Queries `information_schema.innodb_trx`, `processlist`, and `performance_schema.metadata_locks` when accessible. Enforces `SET SESSION lock_wait_timeout = <N>` on an isolated `*sql.Conn`.
   - **SQLite**: Enforces `PRAGMA busy_timeout = <N*1000>` on isolated connection.
   - **Future Engine Scalability**: PostgreSQL (`SET lock_timeout = '<N>s'`), Oracle (`ALTER SESSION SET DDL_LOCK_TIMEOUT = <N>`).
3. **Schema Relationship Introspection**:
   - Analyzes `SchemaMetadata` to discover parent-child foreign key couplings, warning if a small table is joined/referenced by massive tables.
4. **OSC Script Generation**:
   - Generates production-ready `gh-ost` (default, triggerless binlog streaming) and `pt-online-schema-change` commands for zero-downtime migrations.

## Consequences

- Zero risk of runaway 1-year lock queue pileups when DDL encounters concurrent locks.
- Instant visibility into active blocking transactions before running schema changes.
- Safe execution path for both massive transactional tables and small, heavily-joined master tables.
