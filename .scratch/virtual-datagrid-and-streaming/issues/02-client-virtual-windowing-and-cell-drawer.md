# Issue 02: Client Virtual Windowing & Cell Detail Drawer

Status: resolved
Type: task
Feature: virtual-datagrid-and-streaming
Blocked by: 01

## Goal
Implement fixed-height (32px) DOM recycling virtual windowing in `web/dist/index.html` for query results, coupled with a persistent keyboard-synced Cell Detail Drawer and fetch ReadableStream ingestion.

## Requirements
1. Consume `POST /api/session/{id}/execute-stream` using `fetch()` and `ReadableStream`.
2. First row chunk painted immediately upon receipt (TTFR < 50ms).
3. Fixed-height 32px row windowing: compute visible row range based on `scrollTop` of `#virtual-grid-wrapper`.
4. Render only ~35-40 `<tr>` elements + top/bottom spacer height, guaranteeing `< 500` DOM elements at all times.
5. Persistent Cell Detail Drawer (`#cell-detail-drawer`) with:
   - Full string display, JSON pretty formatting, and 1-click Copy button.
   - Synchronizes live with arrow-key keyboard navigation and mouse clicks.
6. Hybrid column windowing: when column count exceeds 50, slice visible columns during horizontal scroll.
7. Unit tests in `web/embed_test.go` verifying Virtual Data Grid functions, element IDs, and syntax validation with Node VM.

## Answer
Implemented in:
- `web/dist/index.html`:
  - `renderVirtualWindow()` & `initVirtualGridScroll()` computing $O(1)$ window bounds using `VIRTUAL_ROW_HEIGHT = 32`, maintaining top/bottom spacer rows and `< 500` DOM nodes.
  - Persistent Cell Detail Drawer (`#cell-detail-drawer`) with live sync (`syncCellDetailDrawer`), JSON pretty formatter, and copy button.
  - Streaming query runner in `runActiveQuery()` consuming `application/x-ndjson` chunks via `ReadableStream` reader, mounting initial micro-chunk (20 rows) for sub-50ms TTFR.
- `web/embed_test.go`: `TestVirtualDataGridAndStreamingEmbedded` and `TestEmbeddedIndexHTMLJavaScriptSyntax` verifying Node VM syntax and embedded tokens.
