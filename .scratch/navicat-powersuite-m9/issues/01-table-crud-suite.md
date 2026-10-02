# Issue 01: Table CRUD Suite (Inline Add Row, Multi-Row Delete, Column Subheader Filter)

Status: resolved
Feature: navicat-powersuite-m9

## Description
Provide spreadsheet-like CRUD operations directly on the Table Viewer Data Grid:
1. **Inline "+ Add Row"**: Toolbar button adding an editable draft row at top of the grid with pending `stagedInserts`.
2. **Backend Row Insert**: `POST /api/session/{id}/table/{table}/insert-row` accepting row data map, generating dialect-quoted `INSERT INTO`, and returning the created row / ID.
3. **Multi-Row Delete**: Checkbox column on the grid with `Delete Selected (N)` action executing batch `DELETE FROM <table> WHERE <pk> IN (...)`.
4. **Column Subheader Filter**: Quick input field below each column header that converts typed filter to a Quick Clause Pill on Enter.

## Verification
- Unit test for `insert-row` and batch `delete-rows` in `internal/server/`.
- UI verification for row drafting, checkbox multi-selection, and filter subheader synchronization.
