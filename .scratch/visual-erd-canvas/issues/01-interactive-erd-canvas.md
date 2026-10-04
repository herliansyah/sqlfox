# Issue 01: Visual ERD Interactive Canvas

Status: resolved
Type: task
Feature: visual-erd-canvas

## Goal
Implement a zero-dependency interactive Entity-Relationship Diagram (ERD) canvas modal in SqlFox rendering draggable table cards with cubic Bézier SVG foreign key lines, auto-layout, viewport fit, and session position caching.

## Requirements
- Dialog modal `#erd-dialog` with toolbar controls (`⚡ Auto Layout`, `⊡ Fit All`, table counter).
- SVG layer `#erd-svg` with amber arrow markers and Bézier curves dynamically pinned to left/right card edges.
- Draggable table cards `#erd-cards` showing columns, emerald PK badges (`🔑`), and amber FK badges (`🔗`).
- Smooth canvas drag panning and cursor-anchored wheel zooming on `#erd-viewport`.
- Coordinate persistence in `sessionStorage` (`erd-pos-{sessionId}-{db}`).
- Zero new backend endpoints, consuming `GET /api/session/{id}/schema?db={db}`.

## Answer
Implemented and merged via PR #18 in `web/dist/index.html` with:
- Top Navbar `📦 DB Tools ▾` menu item `🗺️ Visual ERD Canvas` triggering `openERDCanvas()`.
- SVG Bézier line rendering with marker arrowheads in `erdRenderLines()`.
- Draggable card event handlers (`erdOnCardMouseDown`, `erdOnMouseMove`, `erdOnMouseUp`).
- Canvas pan & zoom handlers (`erdOnCanvasMouseDown`, `erdOnWheel`).
- Auto-layout grid algorithm (`erdAutoLayout`) and viewport bounding-box calculator (`erdFitAll`).
- Documented in `docs/adr/0029-visual-erd-canvas.md` and implementation plan `docs/superpowers/plans/2026-10-02-visual-erd-canvas.md`.
