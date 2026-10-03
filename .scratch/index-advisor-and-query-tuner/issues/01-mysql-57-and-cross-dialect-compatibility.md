# Issue: MySQL 5.7 and Cross-Dialect Index DDL & EXPLAIN Introspection Compatibility

Status: resolved
Type: research

## Question
What are the exact syntax rules, catalog introspection queries, and EXPLAIN plan output differences for non-blocking index creation and index discovery across MySQL 5.7 (InnoDB), MySQL 8.0+, PostgreSQL, and SQLite?

Specifically:
1. What exact syntax is valid for online non-blocking index creation in MySQL 5.7 (e.g. `ALTER TABLE tbl ADD INDEX idx_name (col1, col2) ALGORITHM=INPLACE, LOCK=NONE;`) versus PostgreSQL (`CREATE INDEX CONCURRENTLY idx_name ON tbl (col1, col2);`) versus SQLite (`CREATE INDEX IF NOT EXISTS idx_name ON tbl (col1, col2);`)?
2. How does `EXPLAIN FORMAT=JSON` differ between MySQL 5.7 and MySQL 8.0 (specifically regarding `cost_info`, `using_filesort`, and `attached_condition` fields)?
3. How should existing indexes be retrieved cleanly from MySQL 5.7 `information_schema.statistics` (or `SHOW INDEX FROM`) to detect existing prefix coverage and prevent redundant index suggestions?

## Answer
1. **Online Non-Blocking DDL**:
   - **MySQL 5.7 / 8.0 (InnoDB)**: `ALTER TABLE tbl ADD [UNIQUE] INDEX idx (col1, col2) ALGORITHM=INPLACE, LOCK=NONE;` enables concurrent reads & writes. Guard against 767-byte prefix limits on `COMPACT`/`REDUNDANT` tables with utf8mb4 (wrap >191 char columns). Enforce a 3s `lock_wait_timeout` to prevent MDL queueing.
   - **PostgreSQL**: `CREATE [UNIQUE] INDEX CONCURRENTLY [IF NOT EXISTS] idx ON tbl (col1, col2);` avoids table locking, but cannot run in a transaction block. Interrupted builds leave invalid indexes (`indisvalid = false`) that must be dropped.
   - **SQLite**: `CREATE [UNIQUE] INDEX IF NOT EXISTS idx ON tbl (col1, col2);`. No online non-blocking mode; requires exclusive write lock guarded by `PRAGMA busy_timeout = 3000`.
2. **`EXPLAIN FORMAT=JSON` Traversal**:
   - Both MySQL 5.7 and 8.0 share the core JSON path attributes (`cost_info`, `using_filesort`, `attached_condition`, `access_type`), but optimizer operations (`ordering_operation`, `grouping_operation`, `nested_loop`) vary by query. The Go parser must use a recursive walker over the JSON node tree to reliably extract table-level plan steps.
   - MySQL 5.7 does not support `FORMAT=TREE` or `EXPLAIN ANALYZE`; `FORMAT=JSON` is the universal structured format.
3. **Catalog Introspection & Bloat Detection**:
   - Query `information_schema.STATISTICS` without 8.0-specific columns (`IS_VISIBLE`, `EXPRESSION`) to maintain 5.7 compatibility.
   - Fix existing bug in `internal/driver/mysql/mysql.go` where composite index columns after the first were not updated in `tables[idx].Indexes`.
   - Calculate bloat threshold when secondary index count > 5 or `INDEX_LENGTH > DATA_LENGTH` from `information_schema.TABLES` (MySQL) or `pg_indexes_size > pg_table_size` (PostgreSQL).
