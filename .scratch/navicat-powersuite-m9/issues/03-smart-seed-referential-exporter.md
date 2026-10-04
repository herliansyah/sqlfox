# Issue 03: Referential Slice Exporter (FK-Aware Smart Seed Dump)

Status: resolved
Feature: navicat-powersuite-m9

## Description
Provide zero-configuration, schema-wide seed data generation that guarantees 100% error-free restore:
1. **Dependency Analysis**: Interrogate Foreign Key relations (`information_schema.key_column_usage` or SQLite table info).
2. **Topological Sort**: Order tables so parent/master tables are emitted before dependent child tables.
3. **Recursive Transitive Closure**: Sample up to $N$ rows (default: 20) per table, and for any foreign key referenced by a sampled row, ensure the parent row is fetched and included in the dump.
4. **Streaming Endpoint**: `GET /api/session/{id}/export-smart-seed?limit=20&ddl=true` streaming the ordered SQL file directly to client.

## Verification
- Unit test in `internal/server/` creating parent-child tables with FKs, running smart seed exporter, and verifying order and complete referential integrity.
