# Issue 04: Large SQL Script Runner & Clipboard Formats

Status: resolved
Feature: navicat-powersuite-m9

## Description
Provide safe execution of large multi-statement `.sql` files and quick sharing formats:
1. **Large SQL Runner Backend**: `POST /api/session/{id}/run-sql-script` reading streamed SQL multipart file, breaking statements cleanly by semicolon delimiters (respecting string literals and comments), and executing statement by statement with chunked JSON progress reporting (`total_statements`, `executed_statements`, `errors`).
2. **Execution Options**: Support `stop_on_error` parameter (default `true`).
3. **Clipboard Formats**: Implement `Copy as TSV` and `Copy as Markdown Table` for grid data (supporting selected rows or current page).

## Verification
- Unit test in `internal/server/` streaming a multi-statement SQL script and asserting progress reporting and statement execution.
- UI test verifying clipboard formats copy clean TSV and GFM tables.
