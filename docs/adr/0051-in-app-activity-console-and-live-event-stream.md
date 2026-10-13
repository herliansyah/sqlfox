# 0051: In-App Activity Console and Live Event Stream

To equip SqlFox with real-time in-app operational visibility via a streaming Server-Sent Events (SSE) telemetry transport and an interactive, collapsible Activity Console drawer directly within the web interface, preserving the single-binary zero-dependency Ponytail standard.

## Context and Problem Statement

1. **Lack of In-App Operational Feedback**:
   While ADR 0043 established the `Diagnostic Logger` using standard library `log/slog`, logs were only accessible via terminal stderr or persistent file sinks. Desktop operators running the application in browser tabs had no mechanism to observe runtime activities (executed SQL statements, latencies, schema introspections, background tasks, warnings, or errors) in real time without opening a terminal or running `tail -f`.
2. **Realtime Transport Overhead**:
   Delivering live log events from the Go daemon to web clients could tempt introduction of heavy WebSocket dependencies or polling intervals that add CPU/network chatter.
3. **Ergonomics & Reading Comfort**:
   During active operations, rapidly scrolling logs make reading recent warnings or errors frustrating unless frozen. Operators require smart viewport pause controls, severity filters, and client-side log exports.

## Considered Options

- **Option 1: WebSockets (`nhooyr.io/websocket` or `gorilla/websocket`)**:
  *Rejected*: Introduces third-party dependencies and bidirectional framing overhead for what is strictly a unidirectional (server-to-client) telemetry broadcast.
- **Option 2: Periodic Polling (`GET /api/logs?since=...`)**:
  *Rejected*: Polling introduces latency or wasteful HTTP request chatter when the system is idle.
- **Option 3: Pure Go Stdlib SSE (`text/event-stream`) + In-Memory Telemetry Ring Buffer (Selected)**:
  - Add `Telemetry Ring Buffer` (fixed-capacity 100 entries) and `Broadcaster` in `internal/logger` implementing `slog.Handler`.
  - Expose `GET /api/logs/stream` using native Go `http.Flusher` and standard browser `EventSource`.
  - Implement collapsible `Activity Console` bottom drawer in the embedded SPA with 5 columns (Time, Level, Module, Message, Duration/Meta), smart auto-scroll, freeze/pause toggle, level filters, quick search, and `.log` / `.json` export.

## Decision

Adopt **Option 3**.

## Consequences

- Real-time zero-polling log visibility inside the desktop application UI with sub-10ms delivery latency.
- Zero external dependencies introduced, strictly using Go standard library `log/slog` and `net/http`.
- Operators can inspect what SqlFox is doing at any moment, pause the feed, filter by severity, and export logs for troubleshooting.
