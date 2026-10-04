# Specification: Navicat Power Suite - Milestone 11 (Schema & Data Synchronization Engine)

## 1. Overview & Objective
Provide enterprise-grade synchronization capabilities between database connections in SqlFox without third-party dependencies or memory bloat:
1. **Structure Sync**: Automated AST/metadata DDL diff between source and target databases, generating visual difference summaries and precise non-destructive migration scripts (`CREATE TABLE`, `ALTER TABLE ADD/MODIFY COLUMN`, `CREATE INDEX`).
2. **Cross-Connection Data Transfer**: Direct server-side streaming pipe between two live database sessions (`SourceSession` -> `TargetSession`) without temporary disk files and bounded memory (<50MB).
3. **Row-Level Data Sync**: Primary-key differential reconciler detecting inserted, updated, and deleted records between source and target tables, with preview verification and atomic batch execution.

## 2. Architecture & Components

### 2.1 Structure Sync (`POST /api/sync/structure/compare` & `POST /api/sync/structure/apply`)
- Compares table definitions, column types/nullability/defaults, and indexes between source session/database and target session/database.
- Categorizes discrepancies:
  - Missing tables in target: `CREATE TABLE` (DDL extracted directly from source).
  - Extra tables in target: `DROP TABLE` (optional toggle, default false).
  - Missing columns in target: `ALTER TABLE target ADD COLUMN col ...`
  - Discrepant columns: `ALTER TABLE target MODIFY COLUMN col ...` (or `ALTER COLUMN`)
  - Missing indexes: `CREATE [UNIQUE] INDEX idx ON target(...)`
- Generates executable DDL migration script with granular per-statement inclusion checkboxes in the UI.

### 2.2 Cross-Connection Data Transfer (`POST /api/sync/data/transfer`)
- Streams data directly across two active sessions:
  - Configurable target mode: `Create target table if missing`, `Truncate target table before insert`.
  - Batch size parameter: default 500 rows per insert statement.
  - Source `QueryStream` yields chunked rows -> server builds multi-row parameter bindings -> executes on target `Session.Exec`.
  - Emits streaming progress events (SSE / chunked NDJSON: current table, rows transferred, total rows, elapsed time) allowing live progress bar feedback.
- Strictly memory-bounded: chunks processed in-flight, zero temporary files, memory buffer < 50MB.

### 2.3 Row-Level Data Sync (`POST /api/sync/data/compare` & `POST /api/sync/data/apply`)
- Keyset / PK delta reconciler for a chosen table (or table list).
- Inspects primary key on source and target tables.
- Computes delta:
  - Rows present in source but missing in target -> `INSERT INTO target VALUES (...)`
  - Rows present in both but with column value drift -> `UPDATE target SET ... WHERE pk = ...`
  - Rows present in target but missing in source -> `DELETE FROM target WHERE pk = ...` (toggleable option)
- Returns visual preview of operations with counts and diff rows before execution.
- Applies changes atomically inside a transaction or sequential parameterized batches on target.

### 2.4 Ergonomics & UI Integration
- Top Navbar `📦 DB Tools ▾` dropdown links:
  - `🔄 Structure Sync` -> Opens Structure Sync modal / dedicated view.
  - `🚚 Data Transfer` -> Opens Cross-Connection Data Transfer modal.
  - `⚖️ Data Sync` -> Opens Row-Level Data Sync modal.
- Native modals following SqlFox zero-dependency CSS theme and high-contrast design.
- Production environment safety guards: If the target connection is marked `Production`, DDL and Data sync actions require the standard Safety Guard challenge confirmation.

## 3. Constraints & Guidelines
- Zero external Go dependencies (pure stdlib `net/http`, `database/sql`, `fmt`, `strings`, `sync`).
- Minimal code footprint adhering to Ponytail lazy senior dev principles.
- Comprehensive automated verification via `internal/server/sync_test.go`.
