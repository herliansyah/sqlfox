# 0034: Large-Scale Hardening and Memory Ceiling Benchmark

## Status
Accepted

## Context
Following the completion of core developer operations and headless automation features (ADR-0030 to ADR-0033), SqlFox must be hardened for enterprise-scale workloads:
1. **Large Schema Introspection**: Workspaces managing databases with 1,000+ tables, 50,000+ columns, and cyclical foreign key references experience potential bottlenecks in metadata retrieval, JSON serialization, and DOM rendering.
2. **Strict Memory Budget**: SqlFox has an architectural commitment to remain lightweight (<100MB resident set size RAM), even under heavy streaming loads (e.g. 1,000,000 rows in RowStream, large SQL dumps, and high-throughput cross-connection transfers).
3. **Connection Pool Saturation**: High concurrency or network latency can saturate target database connection pools, leaving secondary out-of-band cancellation channels (`KILL QUERY` / `pg_cancel_backend`) stalled if not bounded with a fail-fast timeout.
4. **Web UI Resilience**: Transient network interruptions, server restarts, or large payload delivery must degrade gracefully without crashing client tabs or discarding in-flight query drafts and staged table edits.
5. **Binary Footprint**: Continual feature additions risk binary bloating beyond the 25MB ceiling and degrading the sub-second launch experience.

## Decisions

### 1. In-Memory Synthetic Schema Benchmark Suite
To measure and prevent performance regressions without bloating the production binary, a synthetic schema generator is implemented purely in Go test files (`*_test.go`).
- **Tiered Execution**:
  - Fast smoke test (100 tables, 5,000 columns) runs as part of standard `go test ./...` (<200ms) to ensure continuous regression protection in regular dev loops.
  - Comprehensive stress test (1,000 tables, 50,000 columns, FK cycles) is invoked via Go standard benchmarking: `go test -bench=BenchmarkLargeSchema -benchmem ./internal/...`.
- **Validation Criteria**: Asserts that full schema catalog building and JSON serialization execute in <500ms with bounded memory allocations per table object.

### 2. Browser-Native Content Visibility Virtualization
To maintain 60fps scrolling and eliminate DOM freezes when browsing databases with 1,000 to 10,000 objects in the sidebar:
- Sidebar table nodes utilize native CSS `content-visibility: auto; contain-intrinsic-size: 0 28px;` to skip layout and rendering computations for off-screen elements.
- Schema tree search filtering (`filterSchemaTree`) is wrapped in a 150ms debounce timer to prevent thrashing DOM reconciliation on every keystroke.

### 3. Loopback-Guarded `pprof` Diagnostic Endpoint
- `net/http/pprof` endpoints (`/debug/pprof/*`) are mounted dynamically on the HTTP server only when the environment variable `SQLFOX_PPROF=1` is set.
- All pprof requests are strictly intercepted by the Remote Access Guard, restricting access to loopback (`127.0.0.1` / `::1`) clients to prevent credential or memory structure leakage in multi-IP configurations.

### 4. Fail-Fast Control Channel for Query Cancellation
- Secondary control connections opened for out-of-band query cancellation (`KILL QUERY` on MySQL, `pg_cancel_backend` on PostgreSQL) enforce a strict 3-second context timeout.
- In the event of target connection pool exhaustion where the control connection cannot be established:
  - The client immediately terminates the local `RowStream` consumer to stop memory consumption.
  - A descriptive diagnostic notification informs the operator: *"Database connection limit reached. Cannot establish control channel to cancel query."*

### 5. Global Web UI Error Boundary & State Retention
- A global network interceptor monitors API and WebSocket transport health.
- When a transport error occurs, an unobtrusive floating notification banner appears (*"Connection lost. Reconnecting in Xs..."*) with exponential backoff (1s, 2s, 4s, up to 8s).
- Active editor tabs, unsaved SQL queries, and staged grid edits (`stagedInserts`, `stagedUpdates`, `stagedDeletions`) are strictly preserved in client memory and restored once connectivity is re-established.

### 6. Ponytail Zero-Regression Pruning
- Systematic removal of dead code, redundant helpers, and duplicate formatting functions across driver and server packages.
- Build flags `-ldflags="-s -w"` verified in release pipelines to maintain single binary size well under 25MB (<20MB stripped) and instant startup (<1s).

## Consequences
- Guaranteed sub-second schema exploration and search responsiveness even on enterprise databases with thousands of tables.
- Definite protection against memory leaks and memory spikes over 100MB RAM during streaming workloads.
- Zero white-screen crashes or lost user edits during transient server disconnects.
- Clean, compact single-binary distribution preserving SqlFox minimalism.
