# 0045: Core Engine Scope Freeze and CGO-Free Invariant

## Status
Accepted

## Context
As SqlFox evolved into a resilient multi-engine database cockpit (ADR-0042) supporting MySQL (5.7, 8.0, 8.4 LTS, 9.x), MariaDB, PostgreSQL (9.6 - 16+), and SQLite, architectural proposals emerged regarding adding additional database engines such as DuckDB, ClickHouse, Microsoft SQL Server, and Oracle Database.

In SqlFox, a database driver is not merely a thin wrapper around `database/sql`. An integrated driver must satisfy over 15 distinct operational seams across the codebase:
1. Stateless catalog and schema introspection (tables, columns, composite indexes, foreign keys, views, triggers, and routines).
2. Out-of-band asynchronous query cancellation (`KILL QUERY`, `pg_cancel_backend(pid)`).
3. Real-time cluster health inspection, replication telemetry, and lag monitoring.
4. Server variable introspection and cluster tuning advisory.
5. Keyset streaming, PK auto-detection, and PK-less safe mutations.
6. Pre-flight lock inspection, fail-fast DDL timeouts, and Smart Tiered Backups.
7. Declarative Structure Sync, cross-database data transfers, and streaming dumps.
8. Heuristic Index Advisor and execution plan parsing (`EXPLAIN`).

Furthermore, SqlFox adheres strictly to the Ponytail principle (zero unnecessary dependencies, resident set size RAM <100MB, single self-contained binary <25MB, and sub-second launch times).

## Considered Options

- **Option 1: Unrestricted Driver Expansion (DuckDB, ClickHouse, MSSQL, Oracle)**:
  Attempt to add every requested engine.
  *Rejected*: Highly fragmented feature matrix where non-core engines become fragile "second-class citizens" with broken tabs. Engine drivers requiring C toolchains (e.g. CGO-based DuckDB or Oracle OCI) violate the cross-compilation guarantee (`CGO_ENABLED=0`) established in ADR-0001.
- **Option 2: Plugin / External Daemon Architecture**:
  Support secondary engines via gRPC or out-of-process daemon sidecars.
  *Rejected*: Violates the single-binary zero-runtime-dependency promise, inflates RAM usage, and adds severe debugging and release friction.
- **Option 3: Core Relational Trio Freeze & Non-Negotiable CGO-Free Invariant (Selected)**:
  - Formally freeze database engine scope to the core relational trio: **MySQL / MariaDB**, **PostgreSQL**, and **SQLite**.
  - Enforce the **CGO-Free Invariant** as a non-negotiable rule: any driver requiring local C compiler toolchains is rejected.
  - Channel engineering capacity exclusively into engine parity depth: specifically, normalizing heterogeneous execution plans (`EXPLAIN`) across PostgreSQL (`Seq Scan`) and SQLite (`SCAN table`) into the unified Heuristic Index Advisor.

## Decision

Adopt **Option 3**:
1. **Core Driver Scope**: SqlFox officially bounds its engine support to MySQL/MariaDB, PostgreSQL, and SQLite through v1.0. No speculative drivers will be introduced.
2. **CGO-Free Invariant**: `CGO_ENABLED=0` remains a permanent invariant. The distribution model remains a single self-contained executable under 25MB with bounded memory (<100MB RAM).
3. **Execution Plan Normalization**: Extend `internal/advisor` to natively understand PostgreSQL hierarchical JSON execution plans and SQLite query plan detail strings, ensuring first-class tuning recommendations across all three engines.

## Consequences

- Prevents architectural bloat, dependency creep, and perpetual maintenance overhead.
- Preserves single-command cross-compilation across Linux, macOS, and Windows.
- Delivers deep, rock-solid feature parity for the databases powering >85% of real-world developer and DBA workloads.
