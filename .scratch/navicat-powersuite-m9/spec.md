# Specification: Navicat Power Suite - Milestone 9 (CRUD Table Suite & SQL Dump Tooling)

## 1. Overview & Objective
Transform the Virtual Data Grid into an effortless spreadsheet-like data editor and provide robust streaming SQL dump export/import alongside referential slice seeding (FK-aware smart seed dump).

## 2. Core Components

### 2.1 Table CRUD Operator (Spreadsheet Data Manipulation)
- **Inline "+ Add Row" Draft**: A button docked on the Table Viewer toolbar (`+ Add Row`) that prepends an editable draft row to the top of the Virtual Data Grid. Column cells accept input values and are registered in client-side staged changes (`stagedInserts`).
- **Commit & Type Guard Validation**: When the user clicks `Apply (N)`, the backend receives `POST /api/session/{id}/table/{table}/insert-row` or batch payload and executes `INSERT INTO <table> (...) VALUES (...)` with dialect-aware quoting and type validation.
- **Multi-Row Deletion**: A checkbox column on the left edge of the grid allowing single and multi-row selection (`Select All`). Clicking `Delete Selected (N)` triggers a single atomic `DELETE FROM <table> WHERE <pk> IN (...)`.
  - In `Development`/`Staging`, confirmation is handled via `showNativeConfirm`.
  - In `Production`, execution passes through the Safety Guard Challenge Modal.
- **Column Subheader Filter**: A quick filter textbox docked immediately below every column header. Pressing `Enter` converts the input into a `Quick Clause Pill` in the existing Inline Filter Bar, pushing down `WHERE column LIKE '%val%'` or `=` to the database server.
- **Clipboard Formats**: Context menu and toolbar actions for:
  - `Copy as TSV` (Tab-Separated Values, Excel-ready)
  - `Copy as Markdown Table` (GFM markdown table)
  If rows are checked, copies selected rows; otherwise copies all rows visible on the current page.

### 2.2 Comprehensive Import & Export Tooling
- **Streaming SQL Dump Export (`GET /api/session/{id}/export-dump`)**:
  - Direct HTTP chunked streaming download (`Content-Disposition: attachment; filename=<db>_dump.sql`).
  - Emits header, `CREATE TABLE` DDL for each table, and chunked batch `INSERT INTO` statements (e.g. 50-100 rows per insert statement).
  - Streams directly over `io.Pipe` and `RowStream` with memory bounded to <10MB.
- **Referential Slice Exporter / Smart Seed Dump (`GET /api/session/{id}/export-smart-seed`)**:
  - Modal prompt with configurable bounded row sample (default `limit=20` rows per root table) and toggle `Include DDL`.
  - Dependency crawler: Analyzes the schema's Foreign Key relationship graph, performs topological sorting (parent tables before child tables), recursively fetches upstream parent referenced rows (transitive closure), and writes inserts in dependency order.
  - Guaranteed 100% clean restore without foreign key constraint violations.
- **Large SQL Script Runner (`POST /api/session/{id}/run-sql-script`)**:
  - Modal with `.sql` file picker, execution boundary options (`Stop on error` toggle), and real-time execution progress reporting (streaming SSE/chunked JSON).
  - Memory-bounded multi-statement parsing without reading full gigabyte files into RAM.

## 3. Non-Functional Requirements & Constraints
- **Zero New External Dependencies**: Use standard Go library (`net/http`, `io`, `bufio`, `database/sql`, `fmt`) and embedded frontend vanilla JS.
- **Memory Bounded**: Memory buffers strictly capped at <10MB. No buffering of entire database dumps in RAM.
- **Safety Guard Integration**: All write mutations (`INSERT`, `DELETE`) route through standard safety validations.
