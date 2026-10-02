# Issue 02: Memory Ceiling Budget & Protected pprof Profiling

Status: resolved
Type: task
Feature: large-scale-hardening

## Goal
Implement loopback-protected pprof diagnostic endpoints mounted via `SQLFOX_PPROF=1` and memory ceiling benchmark tests enforcing <100MB RAM budget under heavy streaming.

## Requirements
- Mount `net/http/pprof` endpoints (`/debug/pprof/*`) conditionally when `os.Getenv("SQLFOX_PPROF") == "1"`.
- Guard pprof endpoints against non-loopback IP requests.
- Add test assertions verifying memory usage stays well within the 100MB ceiling during streaming operations.

## Answer
Implemented in `internal/server/pprof.go` and verified in `internal/server/pprof_test.go`:
- `registerPprof` conditionally mounts `/debug/pprof/*` when `SQLFOX_PPROF=1`.
- `loopbackOnly` middleware returns `403 Forbidden` for any non-loopback client IP.
- Verified in `internal/driver/benchmark_test.go` (`BenchmarkMemoryCeilingStreaming`) that 1,000,000 streaming records strictly satisfy the <100MB RAM RSS ceiling.

