# ADR-0029: Visual ERD Interactive Canvas

**Status:** Accepted  
**Date:** 2026-10-02

## Context

Users need a quick visual overview of table structures and FK relationships in any connected database. Existing FK peek popover provides single-cell lookup but no holistic diagram.

## Decision

Implemented as a pure-frontend `<dialog id="erd-dialog">` using:
- **SVG layer** (`#erd-svg`): FK relationship arrows drawn as cubic Bézier `<path>` elements with amber `<marker>` arrowheads; right/left edge selection based on relative card positions.
- **HTML div layer** (`#erd-cards`): Draggable table cards rendered via `innerHTML` with column lists; PK columns highlighted emerald (🔑), FK columns amber (🔗).
- **Zoom/Pan**: CSS `transform: translate(x,y) scale(s)` on `#erd-viewport`; scroll-to-zoom anchors at cursor position via pan adjustment formula.
- **Auto Layout**: Square-root grid placement (`ponytail:` grid, ceiling: no FK-aware topological sort).
- **Fit All**: Computes bounding box of all cards and scales/pans viewport to fit with padding.
- **Position Persistence**: `sessionStorage` keyed by `erd-pos-{sessionId}-{db}` — survives modal close/reopen, reset on page reload.

Uses existing `GET /api/session/{id}/schema` endpoint. Zero new backend endpoints, zero new dependencies.

## Consequences

- No backend changes; zero new Go modules or npm packages.
- SVG lines are center-to-center (not port-anchored); lines may overlap cards for closely-spaced tables. Port anchoring is a future upgrade path.
- Positions stored in `sessionStorage`, not `localStorage` — intentional: ERD layouts are per-working-session, not permanent.
- Auto-layout uses a simple grid; FK-aware topological sort (cleaner flow for deeply-nested schemas) is the named upgrade path in the `ponytail:` comment.
