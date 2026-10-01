# 0024: Ponytail Pruning, Stateless Service Simplification, and Unified AI Streaming Parser

To maintain extreme codebase simplicity, minimize architectural surface area, and prevent object-oriented boilerplate accumulation in Go, SqlFox executes a comprehensive Ponytail (lazy senior developer) audit and pruning.

## Context & Problem

Following the implementation of advanced DBA diagnostics, cluster health telemetry, and safe DDL guardrails, several minor structural redundancies and boilerplate patterns accumulated in the codebase:
1. **Stateless Service Wrapper Structs**:
   - `safety.SafetyGuard`: A zero-field struct instantiated via `NewSafetyGuard()` and threaded through `Server`, `SessionManager`, and `ActiveSession`, solely to invoke the pure function `safety.Inspect(query, env)`.
   - `dataimport.CSVImporter`: A zero-field struct instantiated via `NewCSVImporter()` wrapping the single method `Import(...)`.
   - `ai.Assistant`: A zero-field struct with redundant delegation methods (`AssemblePrompts`, `Assist`) where `Assistant.Assist` was unreachable dead code according to static analysis.
2. **Triplicate JSON & SSE Response Parser**:
   - `internal/ai/client.go` implemented 125 lines across three repetitive fallback decoding strategies (single JSON object, line-by-line SSE buffer, and multi-chunk `json.Decoder` loop), each repeating identical choice/delta/message/text mapping.
3. **Empty Lockfile**:
   - An empty `package-lock.json` was reintroduced to the root directory despite `package.json` having zero runtime or development dependencies.

## Decision

1. **Prune Stateless Wrapper Structs (`yagni`)**:
   - Delete `type SafetyGuard struct{}`, `NewSafetyGuard()`, and method `(g *SafetyGuard) Inspect(...)`. Call package-level `safety.Inspect` directly.
   - Delete `guard` pointer fields from `Server`, `SessionManager`, and `ActiveSession`.
   - Delete `type CSVImporter struct{}` and `NewCSVImporter()`. Expose package-level `dataimport.Import(...)` directly.
   - Delete `type Assistant struct{}` and `NewAssistant()`. Call package-level `ai.AssemblePrompts(...)` and `ai.Assist(...)` directly.
2. **Streamline AI Response Parser (`shrink`)**:
   - Consolidate `parseOpenAIResponse` into a unified single-pass stream decoder utilizing a shared `openAIChunk.text()` extractor.
   - Preserves 100% compatibility across standard single JSON, chunked JSON, and Server-Sent Events (SSE) streaming proxies (e.g. 9router, LiteLLM) in under 50 lines.
3. **Remove Empty Lockfile (`delete`)**:
   - Remove `package-lock.json`.

## Consequences

- **Net Code Reduction**: -144 net lines across 14 files (+56 insertions, -200 deletions).
- **Zero Dead Code**: `golang.org/x/tools/cmd/deadcode` reports zero unreachable functions across all packages and test binaries.
- **Cleaner Seams**: Removed unnecessary struct dependencies and constructor parameters across `Server` and `SessionManager`.
- **100% Test Integrity**: All unit and integration test suites pass with zero regressions.
