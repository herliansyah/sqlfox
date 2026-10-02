# Issue: Structure Sync Engine

Status: resolved
Type: task

## Goal
Implement AST/metadata-driven schema diffing between two database sessions/databases, generating side-by-side object comparison and safe executable DDL migration statements (`CREATE TABLE`, `ALTER TABLE ADD COLUMN`, `ALTER TABLE MODIFY COLUMN`, `CREATE INDEX`).

## Requirements
- Endpoint `POST /api/sync/structure/compare` accepting:
  - `source_session_id`, `source_db`
  - `target_session_id`, `target_db`
  - `include_drops` (bool, default false)
- Endpoint `POST /api/sync/structure/apply` accepting:
  - `target_session_id`, `target_db`
  - `statements` ([]string)
  - `confirmed` (bool)
  - `challenge_input` (string, for Production targets)
- Schema comparison logic:
  - Detect missing tables in target -> extract DDL via `GetTableDDL` or build `CREATE TABLE`.
  - Detect missing columns in target -> build `ALTER TABLE <table> ADD COLUMN <col> <type> [NOT NULL] [DEFAULT ...]`.
  - Detect mismatched columns (type, nullability) -> build `ALTER TABLE <table> MODIFY COLUMN <col> <type>...`.
  - Detect missing indexes in target -> build `CREATE [UNIQUE] INDEX <name> ON <table> (<cols>)`.
  - Output structured JSON diff with categorized items and ready-to-run SQL script.

## Answer
Implemented in `internal/server/handlers_sync.go` via `handleSyncStructureCompare` and `handleSyncStructureApply`. Tested and verified in `internal/server/sync_test.go` (`TestSyncStructureCompareAndApply`).

