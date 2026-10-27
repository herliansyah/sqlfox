# 0058: Virtual Data Grid & Micro-Chunk Streaming

## Status
Accepted

## Context
As SqlFox targets enterprise workloads with large schemas (1,000+ tables, 50,000+ columns) and high-volume query outputs (tens of thousands of rows), maintaining sub-second launch and responsive execution requires eliminating client-side UI bottlenecks:
1. **DOM Tree Thrashing on Large Resultsets**: Rendering thousands of raw `<tr>`/`<td>` elements directly into the browser DOM degrades Interaction to Next Paint (INP), spikes browser memory usage, and triggers scroll stutter and frame drops.
2. **First-Row Latency Bottleneck**: Waiting for an entire query resultset or a large monolithic batch to transfer before rendering any table cells produces perceived lag on remote database connections.
3. **Wide Table Overhead**: Queries with 50–150+ columns multiply the rendered cell count exponentially, freezing horizontal scrolling when all columns are mounted simultaneously.
4. **Debounced Schema Filtering**: Filtering extensive schema trees via a 150ms debounce timer and DOM tree re-evaluation creates noticeable typing hesitation.
5. **Vague Performance Demands**: Abstract requirements like "super ngebut" or "zero lag" require explicit, measurable Service Level Objectives (SLOs) bound to the physical constraints of network RTT and browser rendering budgets.

## Decisions

### 1. Measurable Performance SLOs
The performance boundaries are formally codified:
- **Interaction to Next Paint (INP)**: `< 16ms` (consistent 60–120 FPS) during active table scrolling and cell navigation.
- **Time to First Row (TTFR)**: `< 50ms` from query dispatch to first painted rows for local databases / low-RTT network connections.
- **Schema Filter Latency**: `< 5ms` instant keyword filtering across 50,000+ schema symbols without debounce delay.
- **Client DOM Ceiling**: Maximum `< 500` active table DOM elements rendered at any moment regardless of total resultset size.

### 2. Fixed-Height Row Windowing Virtual Data Grid
- **Uniform Row Height**: The Virtual Data Grid enforces a uniform 32px row height with cell truncation (`text-overflow: ellipsis`).
- **O(1) Viewport Projection**: Viewport row indices are computed via pure arithmetic (`Math.floor(scrollTop / 32)`), maintaining a fixed pool of ~30–50 rendered `<tr>` elements that are recycled during scroll.
- **Layout Thrashing Elimination**: Dynamic row height measurements and `ResizeObserver` loops are disbarred to protect 60fps scroll stability.

### 3. Persistent Cell Detail Drawer
- To inspect multiline strings, indented JSON, or raw binary hex truncated by the 32px cell boundary, an on-demand persistent side or bottom drawer is introduced.
- The drawer live-syncs with arrow-key keyboard navigation, eliminating modal dialog fatigue and allowing instant continuous inspection.

### 4. Hybrid Column Windowing
- Queries returning $\le 50$ columns render columns directly with CSS containment (`contain: layout style`).
- When resultsets exceed 50 columns, horizontal column virtualization activates dynamically, slicing only columns visible within the horizontal viewport (+ 2 adjacent buffer columns) to keep cell count within the 500 DOM node budget.

### 5. Fetch Stream Ingestion via NDJSON
- Resultsets are transferred over HTTP POST using standard browser `fetch()` and `ReadableStream`.
- The frontend reader incrementally parses newline-delimited JSON (NDJSON) chunks, immediately mounting the first micro-batch (20–50 rows) for instant first-paint while the remaining payload streams concurrently.
- Native `AbortController` handles immediate client-initiated query cancellation.

### 6. Keyset Segmented Streaming & RAM Ceiling
- Client memory buffers a maximum of 5,000 rows per stream segment with an absolute hard ceiling of 25,000 active rows in browser memory.
- When results exceed 5,000 rows, a non-intrusive stream control bar allows 1-click continuation, preventing browser garbage collection pauses and tab memory bloat.

### 7. Warm Catalog Cache & AST DDL Interceptor
- Database schema metadata is cached aggressively in memory upon connection.
- An AST-based DDL interceptor detects local schema modifications (`CREATE`, `ALTER`, `DROP`, `TRUNCATE`) to asynchronously warm and invalidate the catalog cache, backed by a 5-minute TTL and a manual refresh shortcut `[⟳]`.

### 8. Prefix Substring Index for Schema Explorer
- An in-memory inverted prefix/substring index is constructed in JavaScript during catalog initialization.
- Schema searching queries this index directly, eliminating the 150ms debounce timer and delivering immediate sub-5ms search responsiveness.

## Consequences

### Positive
- **Guaranteed 60–120 FPS**: Virtual windowing bounds DOM nodes to `< 500`, preventing UI frame stutter regardless of dataset volume.
- **Sub-50ms Perceived Response**: First rows appear immediately via micro-chunked streaming rather than waiting for full resultset materialization.
- **Zero Third-Party Dependencies**: The engine is built using pure Vanilla JS and browser standards, conforming to the single-binary zero-bloat Ponytail mandate.
- **Memory Safety**: Strict 25,000-row client-side ceiling and bounded Go buffers prevent browser tabs and backend processes from exceeding memory budgets.

### Negative & Trade-Offs
- **Truncated Cell Viewing**: Multiline values cannot expand table rows inline; operators must use the Cell Detail Drawer or clipboard shortcuts for full inspection.
- **Horizontal Slicing Complexity**: Tables exceeding 50 columns require synchronized horizontal header projection.
