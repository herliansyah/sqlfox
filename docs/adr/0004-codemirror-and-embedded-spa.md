# 0004: Lightweight Embedded SPA with CodeMirror and Native DOM Keyset Grid

For high-frequency interactive data grids and SQL editing with sub-second startup and <100MB RAM, SqlFox uses a lightweight embedded single-file SPA architecture (`web/dist/index.html`) embedded via `//go:embed` with CodeMirror SQL editing and high-performance Keyset Pagination over heavy build toolchains (like Monaco Editor or multi-megabyte bundlers).

## Considered Options

- **Monaco Editor**: Powerful, but brings >5MB bundle size, high RAM consumption, and heavy web worker orchestration that violates our lightweight philosophy.
- **Alpine.js + htmx**: Great for document-oriented CRUD, but introduces latency and state friction for interactive grids, inline cell editing, and client-side multi-tab state.
- **Heavy SPA Framework Bundles (Vue/React with `@tanstack/virtual`)**: Requires complex node-based build steps, package managers, and large bundle footprints.
- **Lightweight Single-File SPA with CodeMirror & Plain DOM Keyset Grid (Chosen)**:
  - CodeMirror embedded vendor bundle provides fast syntax highlighting, code completion, and shortcut ergonomics without webpack/vite build friction.
  - Plain DOM table rendering paired with a server-enforced Safe Limit (1,000 rows) and fast O(1) **Keyset Pagination** eliminates memory bloat while delivering 60fps scrolling without virtual scroller complexity.

## Consequences

- The web UI remains completely self-contained in a single asset directory embedded directly into the Go binary. Zero remote CDN dependencies (CodeMirror and Tailwind CSS are vendored locally under `web/dist/vendor/`), enabling instant loading (<10ms) and complete functionality in offline/LAN/air-gapped environments without FOUC.
- Client-side memory footprint stays <30MB, startup time remains sub-second, and builds require zero external frontend tooling.
- Keyset pagination handles millions of rows at the database level rather than buffering large result sets in the browser DOM.
- Critical CSS rules (`.hidden { display: none !important; }` and `dialog:not([open]) { display: none !important; }`) are embedded directly in `<head>` to eliminate any initial render flash or modal bleed.
