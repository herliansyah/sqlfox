# 0028: Schema Structure Sync, Cross-Connection Data Transfer, and Row-Level Data Sync Engine

To support complete database lifecycle management, continuous staging/production deployments, and seamless data migration matching Navicat's synchronization power suite, SqlFox implements an AST/metadata schema comparison engine, a direct memory-bounded cross-connection data transfer pipeline, and a primary-key row-level delta reconciler.

## Context & Problem

1. **Schema Drift Friction**:
   - Developers and teams evolving application features often create tables, add columns, or adjust indexes in local/staging environments.
   - Manually identifying schema drift between staging and production required executing custom SQL queries or third-party diff tools.
   - Crafting manual `ALTER TABLE` migration scripts carries a high risk of syntax errors, type incompatibilities, or breaking existing indexes.
2. **Cross-Connection Migration Friction**:
   - Moving data from one host or database to another previously required downloading a full dump file to local disk and manually importing it into the target host.
   - For multi-gigabyte databases or air-gapped environments, disk storage was wasted and manual intervention was error-prone.
3. **Row-Level Sync & Keyset Discrepancies**:
   - Master data (e.g. system configurations, lookup tables, product catalogs) frequently diverges across environments.
   - Developers needed a way to compute exact delta rows (`INSERT` missing, `UPDATE` modified, `DELETE` stale) with visual confirmation before executing atomic syncs.

## Decision

1. **Structure Sync Engine**:
   - Endpoints: `POST /api/sync/structure/compare` and `POST /api/sync/structure/apply`.
   - Compares two active database connections (`Source` vs `Target`):
     - Missing tables in target -> generates `CREATE TABLE` statements extracted from source DDL.
     - Missing columns in target -> generates `ALTER TABLE <table> ADD COLUMN <col> <type> [NOT NULL] [DEFAULT ...]`.
     - Altered columns in target -> generates `ALTER TABLE <table> MODIFY COLUMN <col> <type>...`.
     - Missing indexes in target -> generates `CREATE [UNIQUE] INDEX <name> ON <table> (<cols>)`.
   - Exposes a Side-by-Side Visual Diff UI with categorized checkboxes and a DDL Review Modal before executing changes on the target database with safety guard validation.
2. **Cross-Connection Data Transfer Engine**:
   - Endpoint: `POST /api/sync/data/transfer`.
   - Streams rows directly from `SourceSession` to `TargetSession` over an in-memory streaming pipe without intermediary disk files.
   - Configurable options: `create_tables` (auto-creates target table if missing), `truncate_target` (cleans target table before insertion), and `batch_size` (default 500 rows).
   - Generates streaming NDJSON progress feedback (table name, rows transferred, percent, elapsed time).
   - Strictly memory bounded (<50MB buffer) using batch streaming.
3. **Row-Level Data Sync (Delta Reconciler)**:
   - Endpoints: `POST /api/sync/data/compare` and `POST /api/sync/data/apply`.
   - Inspects primary key definitions of selected tables on source and target.
   - Computes set operations on keyset:
     - PK in source, not in target -> queued for `INSERT`.
     - PK in both, with differing column values -> queued for `UPDATE`.
     - PK in target, not in source -> queued for `DELETE` (toggleable option).
   - Shows interactive diff preview and applies modifications inside an atomic transaction on target.
4. **Ergonomics & Safety**:
   - Dedicated dialog modals (`#structure-sync-dialog`, `#data-transfer-dialog`, `#data-sync-dialog`) accessible from the top navbar `📦 DB Tools ▾` menu.
   - Production target environments strictly enforce the Safety Guard Challenge confirmation modal before DDL or data mutations execute.

## Consequences

- Full Navicat-grade synchronization workflow natively available inside SqlFox.
- Zero local disk footprint and bounded memory utilization (<50MB) during multi-table migrations.
- Complete visibility and verification of DDL and DML operations before execution.
- Zero third-party dependencies added, preserving single-binary Go compilation.
