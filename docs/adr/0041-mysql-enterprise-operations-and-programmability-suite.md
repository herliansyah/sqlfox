# 0041: MySQL Enterprise Operations and Programmability Suite

To bridge the critical gap between lightweight database browsing and high-stakes operational workflows, SqlFox introduces a unified DBA Fire-Fighting diagnostic suite alongside full developer programmability and partition inspection for MySQL.

## Context and Problem Statement

1. **Post-Mortem Deadlock Blindspot**:
   SqlFox's real-time Processlist Manager successfully visualizes active lock wait trees (`sys.innodb_lock_waits`), but deadlocks in MySQL cause immediate transaction rollback. Once rolled back, the conflict vanishes completely from the active thread list. DBAs investigating application errors (`Deadlock found when trying to get lock; try restarting transaction`) are forced to manually execute and parse multi-thousand-line raw text dumps from `SHOW ENGINE INNODB STATUS`.
2. **Workload Analysis without Daemon Overhead**:
   While the Performance Flight Recorder captures real-time metric spikes during active recording sessions, DBAs and developers frequently need to diagnose slow queries retrospectively (e.g. following an overnight incident or when auditing unindexed queries). MySQL provides aggregated telemetry via `sys.statement_analysis` and `performance_schema.events_statements_summary_by_digest`, but lacking a lightweight visualization dashboard prevents rapid workload triage without installing heavy third-party monitoring daemons.
3. **Programmability & Routines Gap**:
   Production MySQL databases frequently house critical business logic in Stored Procedures, Stored Functions, and Scheduled Events. Currently, SqlFox only introspects Tables, Views, and Triggers in the schema explorer. Users must switch to legacy desktop clients (Workbench, DBeaver) simply to review routine DDL or test procedure invocations.
4. **Large-Scale Partitioning & JSON Ergonomics**:
   Large-scale databases (500GB+ tables) rely heavily on table partitioning (`PARTITION BY RANGE/HASH`), yet Table Designer lacks visibility into partition boundaries, row distributions, and storage overhead. Furthermore, large JSON documents in Virtual Data Grid cells are cumbersome to inspect without formatted pretty-printing and syntax verification.

## Considered Options

- **Option 1: Ad-hoc Manual Workflows (Status Quo)**:
  Rely on users manually executing `SHOW ENGINE INNODB STATUS`, querying `sys.statement_analysis` via the raw SQL editor, and writing manual `CALL` statements.
  *Rejected*: Imposes high friction and cognitive burden during P0 incident triage; ignores the primary reason users rely on GUI database managers.
- **Option 2: Heavy GUI Form Builders & Dedicated Daemon Monitoring**:
  Build complex multi-input GUI forms for Stored Procedure parameters, and install background daemon workers to track query history and partition changes.
  *Rejected*: Violates the Ponytail principle (zero unnecessary abstractions, single-binary <100MB RAM budget) and introduces severe maintenance overhead.
- **Option 3: Lean Native Telemetry & Smart Handoff Suite (Ponytail Architecture)**:
  - **Deadlock Inspector**: An automated AST/regex parser for `SHOW ENGINE INNODB STATUS` (`LATEST DETECTED DEADLOCK`) integrated directly into a Processlist Manager trio switcher, breaking down Transaction 1 vs Transaction 2, locks held vs waited, and victim rollback attribution.
  - **Statement Digest Dashboard**: A zero-daemon analytical view pulling top queries by cumulative latency, average latency, execution count, rows examined vs sent, and full table scans from `sys.statement_analysis` (with automatic fallback to `performance_schema.events_statements_summary_by_digest`) and Challenge Modal-guarded statistics reset (`CALL sys.ps_truncate_all_tables(false)`).
  - **Routines Explorer & Smart Routine Caller**: Collapsible sidebar schema tree sections for Procedures, Functions, and Events with 1-click DDL inspection and a smart SQL handoff (`CALL sp_name(...)`) into a fresh Query Editor tab with typed parameter comments, leveraging CodeMirror 6 and the existing safe execution pipeline.
  - **Scheduled Events Controller**: DDL inspection plus 1-click status toggle (`ALTER EVENT name ENABLE/DISABLE`) for rapid maintenance window operations.
  - **Table Partitions Inspector**: A read-only telemetry sub-tab `[Partitions]` in Table Designer detailing partition schemes, expressions, value ranges, row counts, and physical storage sizes from `information_schema.partitions`.
  - **JSON Cell Viewer Modal**: A focused native dialog in the Virtual Data Grid providing structured formatting, syntax validation, and clipboard copying without breaking 60fps fixed-row virtual scrolling.

## Decision

Adopt **Option 3**.

### Architecture & Driver Seams

1. **DBA Fire-Fighting Seams (`driver.Session`)**:
   - `InspectDeadlock(ctx context.Context) (*DeadlockReport, error)`: Parses `SHOW ENGINE INNODB STATUS` to extract structured deadlock details, with graceful fallback if no deadlock has occurred since server startup.
   - `GetStatementDigest(ctx context.Context, limit int) ([]StatementDigestEntry, error)`: Queries `sys.statement_analysis` or `performance_schema.events_statements_summary_by_digest`.
   - `ResetStatementDigest(ctx context.Context) error`: Executes `CALL sys.ps_truncate_all_tables(false)` with explicit Safety Guard Challenge confirmation.
2. **Schema & Programmability Seams (`driver.Session`)**:
   - Extend `InspectSchema` and `SchemaMetadata` with `Routines []RoutineMetadata` and `Events []EventMetadata`.
   - `GetRoutineDDL(ctx context.Context, routineType, name string) (string, error)`.
   - `GetEventDDL(ctx context.Context, name string) (string, error)`.
   - `ToggleEventStatus(ctx context.Context, name string, enable bool) error`.
   - `GetTablePartitions(ctx context.Context, database, table string) ([]PartitionMetadata, error)`.
3. **Frontend Integration**:
   - Processlist Trio Switcher: `[ 🧵 Active Threads ]`, `[ 📊 Statement Digest ]`, `[ 💀 Deadlock History ]`.
   - Sidebar Explorer: Collapsible `Procedures`, `Functions`, and `Events` sections.
   - Table Designer: Dedicated `[Partitions]` sub-tab alongside `[Data]`, `[Structure & PK]`, `[Indexes]`, `[Foreign Keys]`, and `[CREATE Code]`.
   - Virtual Data Grid: JSON formatting button/double-click modal `#json-cell-dialog`.

## Consequences

- DBAs gain immediate post-mortem insight into transaction deadlocks and historical query bottlenecks without third-party agents or reading raw engine status text.
- Developers can inspect, edit, and invoke Stored Procedures and Functions natively without leaving SqlFox.
- High-volume databases with partitioned tables obtain instant visibility into segment storage and row distributions.
- Zero extra daemon processes, maintaining the strict <100MB RAM footprint and sub-second binary startup.
