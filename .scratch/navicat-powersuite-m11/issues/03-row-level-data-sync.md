# Issue: Row-Level Data Sync (Delta Reconciler)

Status: resolved
Type: task

## Goal
Implement row-level differential reconciliation between source and target tables based on primary key scanning, generating preview of `INSERT`, `UPDATE`, and `DELETE` operations before atomic execution.

## Requirements
- Endpoint `POST /api/sync/data/compare` accepting:
  - `source_session_id`, `source_db`
  - `target_session_id`, `target_db`
  - `table` (string)
  - `delete_missing` (bool, default false)
- Endpoint `POST /api/sync/data/apply` accepting:
  - `target_session_id`, `target_db`
  - `table` (string)
  - `operations` (JSON containing rows to insert, update, delete)
  - `confirmed` (bool)
  - `challenge_input` (string, for Production targets)
- Keyset diffing algorithm:
  - Identify primary key column(s) of the table.
  - Query source rows and target rows.
  - Compute:
    - PK in source but not in target -> queue `INSERT`.
    - PK in both, but column values differ -> queue `UPDATE`.
    - PK in target but not in source (and `delete_missing` true) -> queue `DELETE`.
  - Return summary counts and visual row preview diff.
  - Apply handler executes changes inside an atomic batch/transaction on target.

## Answer
Implemented in `internal/server/handlers_sync.go` via `handleSyncDataCompare` and `handleSyncDataApply`. Tested and verified in `internal/server/sync_test.go` (`TestSyncDataCompareAndApply`).

