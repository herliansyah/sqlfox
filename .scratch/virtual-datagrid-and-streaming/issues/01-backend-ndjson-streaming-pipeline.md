# Issue 01: Backend NDJSON Streaming Pipeline

Status: resolved
Type: task
Feature: virtual-datagrid-and-streaming

## Goal
Implement a high-throughput, low-latency NDJSON streaming query execution endpoint in the Go backend (`POST /api/session/{id}/execute-stream`).

## Requirements
1. Route mounted in `internal/server/server.go`: `POST /api/session/{id}/execute-stream`.
2. Content-Type: `application/x-ndjson`.
3. Support `http.Flusher` to flush header and row chunks immediately:
   - Initial micro-chunk of 20 rows flushed immediately (<15ms) for sub-50ms TTFR.
   - Subsequent chunks batched at 200 rows.
   - Line 1: `{"type":"meta","columns":[...],"target_table":"..."}`.
   - Body lines: `{"type":"chunk","rows":[[...], ...]}`.
   - Final line: `{"type":"done","row_count":N,"execution_ms":M,"tx_active":...}`.
4. Integrate with `inspectAllStatements` safety guard. If query requires challenge/affirmation and is not confirmed, return 422 with inspection payload just like `handleExecuteQuery`.
5. Non-SELECT queries return a single execution line `{"type":"exec","message":"...","rows_affected":N,"execution_ms":M}`.
6. Context cancellation hook: when client disconnects (or `AbortController` fires), cancel the active query immediately.
7. Unit tests in `internal/server/stream_query_test.go`.

## Answer
Implemented in:
- `internal/server/session_manager.go`: `ActiveSession.ExecuteStream` implementing NDJSON micro-chunking (first 20 rows immediate flush, then 200-row chunks, followed by done payload).
- `internal/server/handlers_query.go`: `handleExecuteStream` with delayed stream header writer to guarantee 422/403 HTTP error codes on pre-execution gate failures.
- `internal/server/server.go`: route registered under `POST /api/session/{id}/execute-stream`.
- `internal/server/stream_query_test.go`: verified micro-chunking (20 rows first chunk, 30 rows second chunk, meta and done line format), HTTP handler tests, and Safety Guard block tests. Tests execute in <0.9s.
