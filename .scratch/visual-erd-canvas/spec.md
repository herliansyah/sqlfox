# Specification: Visual ERD Interactive Canvas (Milestone 20)

## 1. Overview & Objective
Provide an interactive, zero-dependency Entity-Relationship Diagram (ERD) canvas within SqlFox allowing developers and DBAs to visually explore table schemas, inspect column data types and key constraints, and trace foreign key relationships without installing external modeling tools or heavy desktop software.

## 2. Architecture & Components

### 2.1 Dialog Modal (`#erd-dialog`)
- Native HTML5 `<dialog id="erd-dialog">` styled with backdrop blur, responsive viewport dimensions (`w-[96vw] max-w-[1400px] h-[90vh]`), and dark theme integration.
- Header toolbar providing:
  - Table count indicator (`erd-table-count`).
  - `⚡ Auto Layout`: Square-root grid arrangement avoiding card overlap.
  - `⊡ Fit All`: Viewport pan and zoom calculation fitting all tables neatly.
  - Close button (`Escape` dismissible).

### 2.2 SVG Relationship Layer (`#erd-svg`)
- SVG canvas layer placed behind table cards rendering foreign key connections.
- Curved connectors using cubic Bézier paths (`M x1,y1 C cx1,y1 cx2,y2 x2,y2`) with amber marker arrowheads.
- Dynamic side pin detection choosing left or right card edges based on relative horizontal card positions.

### 2.3 Draggable Table Cards (`#erd-cards`)
- HTML DOM cards rendered with column schemas:
  - Table header with drag handle.
  - Primary Key columns highlighted with emerald badge and key icon (`🔑`).
  - Foreign Key columns highlighted with amber badge and link icon (`🔗`).
- Smooth drag-and-drop interaction using pointer event delegation, updating connected SVG curves in real time.

### 2.4 Viewport Pan & Zoom (`#erd-viewport`)
- Fluid CSS transform (`translate(x, y) scale(s)`).
- Click-and-drag canvas panning on background areas.
- Pointer-anchored wheel zooming (`0.25x` to `2.5x`).

### 2.5 Coordinate State Persistence
- Card coordinates cached in `sessionStorage` (`erd-pos-{sessionId}-{db}`) preserving layouts across modal toggles while automatically resetting on page reload.

## 3. Constraints & Guidelines
- Zero new external dependencies (pure vanilla JS, SVG, and existing Tailwind classes).
- Zero new backend endpoints (reuses existing `GET /api/session/{id}/schema?db={db}`).
- Single embedded distribution via `web/dist/index.html`.
