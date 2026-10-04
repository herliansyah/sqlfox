# Issue 03: Connection Pool Saturation Guard & Fail-Fast Control Channel

Status: resolved
Type: task
Feature: large-scale-hardening

## Goal
Implement a 3-second fail-fast timeout and pool exhaustion diagnostic handling on out-of-band cancellation channels to avoid hanging during connection saturation.

## Requirements
- Enforce 3-second context timeout on auxiliary control connections (`CancelQuery`) in MySQL and PostgreSQL drivers.
- Abort client-side streaming cleanly when control connection cannot be dialed.
- Return structured diagnostic error indicating pool saturation.

## Answer
Implemented in:
- `internal/driver/mysql/mysql.go`: Wrapped `CancelQuery` in a 3-second timeout (`context.WithTimeout(ctx, 3*time.Second)`), returning descriptive saturation error if timeout occurs.
- `internal/driver/postgres/postgres.go`: Wrapped `CancelQuery` in a 3-second timeout (`context.WithTimeout(ctx, 3*time.Second)`).
- `internal/server/handlers_query.go`: `handleCancelQuery` uses a 3-second timeout and returns `504 Gateway Timeout` when connection limit/saturation occurs.
- `internal/server/session_manager.go`: `ActiveSession.CancelQuery` executes the client-side cancel func immediately before driver dispatch, stopping memory consumption even if target server pool is unresponsive.
- Verified in `internal/server/session_execution_test.go` (`TestActiveSession_CancelQuery_ClientCancelGuaranteed`).

