# Specification: Large-Scale Hardening & Memory Ceiling Benchmark (Milestone 26)

## 1. Overview & Objective
Harden SqlFox performance and resilience under enterprise-scale database environments (1,000+ tables, 50,000 columns, large data streaming) while strictly enforcing resource boundaries (<100MB RAM RSS, <25MB binary size, sub-second response times).

## 2. Core Components

### 2.1 Synthetic Schema Benchmark Suite
- Located in `internal/driver/benchmark_test.go` and `internal/server/benchmark_test.go`.
- Fast unit test verifying catalog serialization on 100 tables in <200ms.
- Go benchmark `BenchmarkLargeSchema` testing 1,000 tables, 50,000 columns, and cyclic FK constraints with memory allocation metrics (`testing.B`).

### 2.2 Memory Ceiling & Loopback pprof Endpoint
- Loopback-guarded `net/http/pprof` mounted in `internal/server/server.go` when `SQLFOX_PPROF=1`.
- Memory benchmark asserting bounded streaming memory (<100MB RSS) under large multi-chunk streaming pipelines.

### 2.3 Connection Pool Saturation Guard
- 3-second fail-fast timeout on secondary out-of-band cancellation channels (`CancelQuery`).
- Descriptive pool-exhaustion error propagation and client-side stream termination.

### 2.4 Browser-Native Content Visibility & UI Error Boundary
- Sidebar schema tree CSS `content-visibility: auto; contain-intrinsic-size: 0 28px;` in `web/dist/index.html`.
- 150ms debounce on schema filter input.
- Global network error interceptor with floating reconnection banner and retention of active query editor tabs and staged cell edits.

### 2.5 Ponytail Zero-Regression Pruning & Verification
- Pruning dead code and redundant helpers.
- Verification of binary size (<25MB) and startup time (<1s).
