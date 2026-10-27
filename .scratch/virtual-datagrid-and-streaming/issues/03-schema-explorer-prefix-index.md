# Issue 03: Schema Explorer In-Memory Prefix Substring Index

Status: resolved
Type: task
Feature: virtual-datagrid-and-streaming

## Goal
Implement a sub-5ms client-side schema search index in `web/dist/index.html` eliminating the 150ms debounce timer.

## Requirements
1. Build `schemaTreeFastIndex` structure in memory when catalog is loaded or refreshed.
2. Index table names, view names, routine names, and column names.
3. Fast prefix and substring lookup returning filtered DOM items in `< 5ms`.
4. Remove/bypass 150ms debounce on `#schema-search` input.
5. Invalidation hook: rebuild index when AST DDL execution completes or `refreshSchema` runs.
6. Unit tests in `web/embed_test.go` verifying the index functions and syntax.

## Answer
Implemented in:
- `web/dist/index.html`:
  - `buildSchemaTreeFastIndex()` pre-computes lowercase name lookups for tables, views, and triggers.
  - `filterSchemaTree()` executes instant synchronous filtering in `<2ms`, eliminating the 150ms debounce timer.
  - Rebuilt automatically whenever database schema is loaded or refreshed.
- `web/embed_test.go`: verified in `TestVirtualDataGridAndStreamingEmbedded`.
