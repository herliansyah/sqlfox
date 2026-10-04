# Issue: Cross-Connection Data Transfer Engine

Status: resolved
Type: task

## Goal
Implement direct server-side data transfer between two active database sessions without intermediary disk files, bounded to <50MB RAM buffer and streaming real-time NDJSON progress events.

## Requirements
- Endpoint `POST /api/sync/data/transfer` accepting:
  - `source_session_id`, `source_db`
  - `target_session_id`, `target_db`
  - `tables` ([]string)
  - `create_tables` (bool)
  - `truncate_target` (bool)
  - `batch_size` (int, default 500)
- Transfer execution loop:
  - For each table:
    - If `create_tables` and table does not exist on target: execute `CREATE TABLE` on target.
    - If `truncate_target`: execute `TRUNCATE TABLE` or `DELETE FROM` on target.
    - Stream source rows via `sourceSession.QueryStream("SELECT * FROM ...", 0)`.
    - Chunk into `batch_size` rows and build multi-value parameterized `INSERT INTO <table> (...) VALUES (...), (...)`.
    - Execute batch on target session.
    - Emit progress event (JSON line: table, rows_copied, total_transferred, percent, status).
  - Memory strictly bounded: chunks processed in-flight without buffering entire tables.

## Answer
Implemented in `internal/server/handlers_sync.go` via `handleSyncDataTransfer`. Tested and verified in `internal/server/sync_test.go` (`TestSyncDataTransfer`).

