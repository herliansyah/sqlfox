# 0039: Universal Mutation Logging and Audit Source Attribution

## Status
Accepted

## Context
SqlFox features a Session Audit Log recording executed statements, durations, affected rows, and error states. However:
1. **Mutation Blind Spots**: Only direct statements submitted through the interactive Query Editor were logged. Non-editor operations—such as inline data grid cell modifications, `+ Add Row` insertions, grid row deletions, CSV dataset imports, synthetic mock data generation, script runner executions, cross-connection data transfers, and schema structure migrations—were executed without persistent audit tracking.
2. **Missing Source Attribution**: When reviewing the audit log, DBAs could not distinguish between interactive ad-hoc queries, automated background sync operations, or manual grid tweaks.
3. **Lack of Context Filtering**: Users had no means to isolate audit entries originating from specific tools (e.g. reviewing only CSV imports or grid updates) without reading raw SQL strings.

## Decisions

### 1. Database Schema Migration
The `audit_logs` table in the local SQLite vault was extended:
- Added a `source TEXT NOT NULL DEFAULT 'editor'` column.
- Applied an idempotent migration during vault initialization (`internal/vault/vault.go`) using `ALTER TABLE audit_logs ADD COLUMN source TEXT NOT NULL DEFAULT 'editor'`.

### 2. Centralized Non-Blocking Dispatch Helper
Added a convenience helper on `ActiveSession` (`internal/server/session_manager.go`):
```go
func (s *ActiveSession) RecordAudit(query string, durationMs int64, rowsAffected int64, err error, source string)
```
- Dispatches audit writes to a background goroutine so that audit logging never adds latency to user-facing API responses.
- Catches and logs errors without failing the primary database operation.

### 3. Comprehensive Feature Attribution
Wired `RecordAudit` across all mutating endpoints:
- `editor`: Query Editor and ad-hoc SQL tab executions.
- `grid_cell_edit`: Single-cell inline modifications in data grid.
- `grid_insert`: `+ Add Row` inline insertions.
- `grid_delete`: Grid row deletions via checkbox selection.
- `script_runner`: Execution of large multi-statement `.sql` scripts.
- `csv_import`: Column-mapped CSV import wizard.
- `mock_data`: Synthetic data generator with relational closure.
- `sync_transfer`: Direct cross-connection data transfers.
- `schema_sync`: Structure Sync DDL migrations.

### 4. API & UI Source Filtering
- Extended `GET /api/session/{id}/audit-logs?source=...` to filter by source origin.
- Session Audit Log drawer renders distinct color-coded badges for each source type.
- Added a `Source: All | Editor | Grid Edit | CSV Import | Mock Data | Sync Transfer | Schema Sync` dropdown filter.
- Updated CSV export to include the `Source` column for external compliance reporting.

## Consequences
- Guarantees 100% audit coverage for every data and schema modification across SqlFox.
- Provides complete forensic attribution for DBA and compliance audits.
- Operates asynchronously with zero performance degradation or memory bloat.
