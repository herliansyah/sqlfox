# Issue 02: Streaming SQL Dump Export

Status: resolved
Feature: navicat-powersuite-m9

## Description
Provide a high-throughput, low-memory streaming SQL dump generator:
1. **Endpoint**: `GET /api/session/{id}/export-dump?db=<db>&table=<optional_table>&data=<bool>&schema=<bool>`
2. **Streaming Execution**: Direct HTTP chunked response (`text/sql` attachment) without buffering entire dump on disk or in memory.
3. **Syntax Standards**: Emits SQL headers (`SET FOREIGN_KEY_CHECKS=0;`), `DROP TABLE IF EXISTS`, `CREATE TABLE`, and chunked `INSERT INTO <table> VALUES (...), (...)` batches (e.g. 50-100 rows per insert).
4. **Memory Constraint**: Bounded memory buffer (<10MB).

## Verification
- Unit test in `internal/server/` verifying streaming dump output contains accurate DDL and multi-row INSERTs for test database.
