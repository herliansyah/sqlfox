# 0027: Streaming SQL Dump, Referential Slice Seeding, and Grid Spreadsheet CRUD Architecture

To empower developers and DBAs with rapid spreadsheet-like data manipulation and lightweight database portability matching advanced tools like Navicat, SqlFox implements an inline Table CRUD operator, memory-bounded streaming SQL dumps, referential slice seeding (FK-aware smart seed dump), large SQL script runner, and standardized clipboard formats.

## Context & Problem

1. **Grid Data Manipulation Friction**:
   - While cell editing is supported (ADR-0016), adding new records required switching to the Query Editor and manually authoring `INSERT INTO <table> (...) VALUES (...)` statements.
   - Deleting multiple records required running raw SQL queries or dropping tables entirely.
   - Quick column-specific filtering required either composing clauses in the top filter bar or typing raw SQL WHERE expressions.
2. **Database Portability & Seeding Limitations**:
   - SqlFox previously only supported CSV import/export and JSON vault exports.
   - Developers spinning up staging/test environments needed standard `.sql` dumps containing `CREATE TABLE` and chunked batch `INSERT` statements.
   - In microservice and staging workflows, dumping massive gigabyte databases to seed a local developer machine is impractical. Conversely, random or naive row sampling (`LIMIT 20`) breaks due to foreign key violations during restore.
3. **Large Script Execution & Memory Spikes**:
   - Executing multi-megabyte SQL dump files via the Query Editor froze the client browser and caused memory exhaustion.
4. **Team Sharing Friction**:
   - Copying grid query results to spreadsheets or PR descriptions required downloading a CSV file and converting it manually.

## Decision

1. **Spreadsheet Table CRUD Suite**:
   - **Inline "+ Add Row"**: Introduces an inline draft row at the top of the Virtual Data Grid. Column cells accept input and are registered in `stagedInserts`. Committing via `Apply (N)` issues `POST /api/session/{id}/table/{table}/insert-row` with type-aware validation.
   - **Multi-Row Deletion**: Checkbox column on the grid allows multi-row selection with a batch `Delete Selected (N)` action executing `DELETE FROM <table> WHERE <pk> IN (...)`. Governed by native confirmation in Dev/Staging and Safety Guard Challenge Modal in Production.
   - **Column Subheader Filter**: Instant text input below each column header that converts typed criteria into Quick Clause Pills in the Inline Filter Bar upon Enter, triggering server-side pushdown.
2. **Memory-Bounded Streaming SQL Dump Export**:
   - Endpoint: `GET /api/session/{id}/export-dump`.
   - Streams directly over HTTP chunked transfer encoding (`text/sql`) using `RowStream` without buffering entire tables or writing temporary files to disk (buffer <10MB).
   - Generates dialect-aware `DROP TABLE IF EXISTS`, `CREATE TABLE`, and chunked multi-row `INSERT INTO <table> VALUES (...), (...)` statements with `FOREIGN_KEY_CHECKS=0;` wrapping.
3. **Referential Slice Exporter (FK-Aware Smart Seed Dump)**:
   - Endpoint: `GET /api/session/{id}/export-smart-seed?limit=20`.
   - Traverses the entire schema Foreign Key graph, performs topological sorting to emit parent/master tables before dependent child tables, and samples up to $N$ rows per table while recursively resolving and including all upstream referenced parent records (transitive closure).
   - Guarantees 100% error-free SQL restore on clean database instances.
4. **Large SQL Script Runner**:
   - Endpoint: `POST /api/session/{id}/run-sql-script`.
   - Streaming executor that parses multi-statement `.sql` files on the fly, dispatching statement by statement with chunked JSON progress reporting and configurable `Stop on error` boundaries.
5. **Clipboard Formats**:
   - Exposes `Copy as TSV` (Excel-safe tab-delimited) and `Copy as Markdown Table` (GitHub Flavored Markdown) in both the toolbar `Export ▾` dropdown and row right-click context menu.

## Consequences

- Direct spreadsheet editing experience without leaving the Table Viewer data grid.
- Zero server disk footprint and minimal memory overhead (<10MB) for arbitrarily large SQL dumps.
- Effortless development environment seeding with guaranteed relational integrity.
- Zero new external dependencies, maintaining single-binary distribution.
