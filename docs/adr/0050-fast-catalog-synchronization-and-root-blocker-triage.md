# 0050: Fast Schema Catalog Synchronization & Root Blocker Incident Triage

## Status
Accepted

## Context
As SqlFox expanded into enterprise multi-database environments (MySQL instances hosting dozens or hundreds of databases and PostgreSQL instances with multiple user schemas), two operational friction points emerged:

1. **Schema Introspection Overhead for Cross-Database Autocomplete**:
   SqlFox's standard `InspectSchema` gathers exhaustive metadata per database: tables, columns, data types, indexes, and foreign keys. On MySQL, foreign key resolution queries `information_schema.KEY_COLUMN_USAGE`, which can acquire table metadata locks (`Waiting for table metadata lock`) and add seconds of latency per database on instances with thousands of tables. However, features like CodeMirror cross-database autocomplete (`dbname.tablename.colname`) and global Schema Search (`Ctrl+P`) only require lightweight table and column identifiers, not deep relational constraints.

2. **Root Blocker Incident Triage Ergonomics**:
   When lock contention stalls production databases, DBA operators need to immediately identify the root blocking query and transaction, inspect the full query statement, understand which queries are blocked and for how long, and execute targeted termination (`Kill Query` to abort the active statement or `Kill Blocker` to terminate the entire client connection). Previous flat listings lacked visual prominence and clipped long SQL statements.

3. **Windows Binary Embedded Branding Consistency**:
   Windows PE executables (`sqlfox.exe`) embed Windows icon resources (`.syso`) compiled from `assets/icon.ico`. Without automated synchronization and validation, regenerating `assets/icon.ico` risked leaving stale low-poly icon frames embedded in binary resources.

## Decisions

### 1. Dedicated Fast Catalog Introspection (`InspectFastCatalog`)
We introduce a lightweight schema introspection method to the core driver contract:
- **`driver.Session.InspectFastCatalog(ctx context.Context) (map[string][]driver.TableMetadata, error)`**:
  - **MySQL**: Performs two sweeping queries against `information_schema.tables` and `information_schema.columns` across all non-system databases (`information_schema`, `mysql`, `performance_schema`, `sys`). Bypasses `KEY_COLUMN_USAGE` and index tables entirely, eliminating foreign key lock overhead.
  - **PostgreSQL**: Queries `information_schema.tables` and `information_schema.columns` filtered to non-system schemas (excluding `pg_catalog`, `information_schema`, and `pg_toast%`).
  - **SQLite**: Returns table and column metadata for the `main` schema.
- **REST Endpoint**: Exposed via `GET /api/session/{id}/fast-catalog`.

### 2. Real-Time Active Thread Load & Pre-Flight Safety Dialog
- **Active Thread Probe**: Added `ActiveThreads` to `driver.ServerInfo` and endpoint `GET /api/session/{id}/active-threads`:
  - MySQL: `SHOW GLOBAL STATUS LIKE 'Threads_running'`.
  - PostgreSQL: `SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND pid <> pg_backend_pid()`.
- **Pre-Flight Confirmation Dialog (`#refresh-all-dialog`)**:
  - Accessible via the `🔄 All` button beside the Database Switcher.
  - Summarizes key benefits: Cross-DB autocomplete (`db.table`, `table.col`), global search (`Ctrl+P`), zero foreign key lock, and isolated per-session persistence.
  - Probes live server health status:
    - `> 100 threads`: Displays critical warning (red), recommending deferring refresh to low-traffic windows.
    - `> 50 threads`: Displays caution warning (amber) indicating high concurrent load.
    - `<= 50 threads`: Displays healthy server confirmation (emerald).
  - Progressive execution with visual progress bar and percentage tracking.
- **Schema Cache Timestamp Badge**: Displays the last catalog refresh time (`#schema-cache-badge`) adjacent to the active database refresh icon.

### 3. Root Blocker Incident Triage UX Overhaul
The Lock Contention Inspector (`#lock-dialog`) now renders an elevated visual card for root blockers:
- **Prominent Tagging**: Large `🚨 ROOT BLOCKER` tag with thread ID, username, and holding lock mode.
- **Scrollable Full SQL Statement**: Full statement container (`max-h-48 overflow-y-auto font-mono text-[13px]`) with syntax contrast, eliminating truncated queries during urgent triages.
- **Granular Mitigation Buttons**: Distinct, color-coded buttons for `Kill Query` (amber) and `Kill Blocker` (red).
- **Blocked Query Chain Card**: Visual nested container detailing blocked thread ID, user, elapsed waiting seconds (highlighted in red/white), and the blocked query statement.

### 4. Windows PE Icon Compilation & Consistency Testing
- `scripts/generate-icons.py` automatically recompiles `cmd/sqlfox/rsrc_windows_amd64.syso` and `rsrc_windows_arm64.syso` using `rsrc` directly from `assets/icon.ico`.
- Unit test `TestWindowsEmbeddedIconConsistency` in `cmd/sqlfox/main_test.go` verifies that all resolution frames from `assets/icon.ico` (including 16x16, 32x32, 48x48, 64x64, 128x128, 256x256) are embedded in both Windows PE resource objects.

## Consequences
- Cross-database autocomplete and search caches can be refreshed across dozens of databases in a fraction of a second with zero table metadata lock contention.
- DBAs gain pre-flight awareness of active thread saturation before initiating wide catalog sweeps.
- Root blocker diagnosis during production lock storms is fast, clear, and provides unambiguous termination controls.
- Binary icons on Windows PE binaries remain 100% in sync with canonical repository branding assets.
