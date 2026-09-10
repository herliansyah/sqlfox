# 0004: CodeMirror 6 and Embedded Reactive SPA over htmx and Monaco

For high-frequency interactive data grids and SQL editing with sub-second startup and <100MB RAM, we use CodeMirror 6 and a reactive frontend bundle (Vue 3/Svelte with `@tanstack/virtual`) embedded via `//go:embed` instead of htmx or Monaco Editor.

## Considered Options

- **Monaco Editor**: Powerful, but brings >5MB bundle size, high RAM consumption, and heavy web worker orchestration that violates our lightweight philosophy.
- **Alpine.js + htmx**: Great for document-oriented CRUD, but introduces latency and state friction for 100k-row virtual scrolling, inline cell editing, and client-side tab state.
- **CodeMirror 6 + Reactive Virtualized SPA**: Modular (<350KB), rich extensible SQL language server support, zero virtual DOM overhead, and ultra-smooth virtual scrolling.

## Consequences

The web UI builds to a single static asset package embedded directly in the Go binary. Client-side memory remains minimal and UI interactions remain 60fps without server roundtrips.
