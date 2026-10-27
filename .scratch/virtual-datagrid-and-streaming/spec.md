# Specification: Virtual Data Grid & Micro-Chunk Streaming (ADR 0058)

## 1. Overview & Objective
Transform SqlFox into an ultra-responsive, zero-lag database client achieving:
- Interaction to Next Paint (INP) < 16ms (60–120 FPS table scrolling).
- Time to First Row (TTFR) < 50ms for low-RTT queries via NDJSON micro-chunk streaming.
- Sub-5ms instant schema tree searching using an in-memory prefix/substring index.
- Strict DOM ceiling (<500 active table nodes) and client RAM bound (25,000 rows max).

## 2. Core Components

### 2.1 Backend NDJSON Streaming Pipeline
- Add `POST /api/session/{id}/execute-stream` in `internal/server/handlers_query.go` and `internal/server/server.go`.
- Stream response lines in newline-delimited JSON format (`application/x-ndjson`):
  - Line 1: `{"type":"meta","columns":[...],"query_id":"..."}`
  - Subsequent lines: `{"type":"chunk","rows":[[...], ...]}` with micro-chunking (initial batch 20 rows, subsequent batches 200 rows).
  - Final line: `{"type":"done","row_count":N,"execution_ms":M,"profile":...}`
- Flushes data immediately after header and after each chunk using `http.Flusher`.
- Supports out-of-band query cancellation when client aborts the request context.

### 2.2 Client-Side Virtual Data Grid Engine & Cell Detail Drawer
- In `web/dist/index.html`:
  - Enforce uniform fixed-height row windowing (32px) with pure arithmetic viewport mapping ($O(1)$).
  - Calculate `scrollTop` and maintain a fixed pool of ~35 rendered `<tr>` elements + buffer rows.
  - Hybrid column windowing: direct rendering if $\le 50$ columns; slice visible columns if $> 50$ columns.
  - Implement Persistent Cell Detail Drawer (`#cell-detail-drawer`) live-synchronized with focused cell/keyboard arrows with JSON formatting and 1-click Copy.
  - Keyset Segmented Streaming: receive stream chunks, cap active client array at 25,000 rows.

### 2.3 In-Memory Prefix Substring Index for Schema Explorer
- Replace linear scanning and 150ms debounce with an in-memory inverted search index (`schemaSearchIndex`).
- Instant `< 5ms` filtering on keypress across tables, views, and columns without UI hesitation.
