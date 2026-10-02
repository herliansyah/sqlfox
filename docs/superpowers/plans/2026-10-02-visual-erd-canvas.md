# Visual ERD Interactive Canvas Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a zero-dependency interactive ER Diagram canvas that renders database tables as draggable cards with FK relationship lines, using existing `/api/session/{id}/schema` data.

**Architecture:** Pure-frontend `<dialog>` modal containing an SVG overlay (FK lines) + HTML div layer (draggable table cards) inside a pannable/zoomable viewport. All schema data comes from the already-fetched sidebar schema (or a fresh `GET /api/session/{id}/schema` call). No new backend endpoints needed.

**Tech Stack:** Vanilla JS, SVG (FK arrows), CSS transform (pan/zoom), existing Tailwind classes. Zero new dependencies.

**Spec:** `MILESTONES.md` item 16: "Visual ERD Interactive Canvas (Drag-and-drop relationship visualizer)"

## Global Constraints

- Zero new Go dependencies, zero npm packages
- All JS in `web/dist/index.html` (single embedded file, `//go:embed`)
- No external CSS frameworks or icon fonts beyond what's already there
- Tailwind classes only (already loaded via CDN in the page)
- Must not break existing tests (`go test ./...` green throughout)
- Server restart required for HTML embed changes (daemon at `http://localhost:8081`)

## Review Focus

- **Circular FK graphs**: A → B → A; layout algorithm must not infinite-loop — use a visited-set grid placement
- **Tables with no FK**: Should still render as isolated cards in an orderly grid, not piled at origin
- **Very wide schemas (50+ tables)**: Canvas must scroll/pan without freezing — use CSS transform, not re-rendering
- **SQLite databases**: `foreign_keys` array may be empty; ERD must still open cleanly showing all tables
- **Zoom at cursor position**: Scroll-to-zoom should zoom toward pointer, not top-left corner

---

## Task 1: ERD Dialog Shell & Navbar Trigger

**Files:**
- Modify: `web/dist/index.html` — add `<dialog id="erd-dialog">` with toolbar + canvas wrapper; add `🗺️ ERD` button to top navbar DB Tools dropdown

**Interfaces:**
- Produces: `openERDCanvas()` global function; `#erd-dialog` element; `#erd-canvas-wrapper` div

- [ ] **Step 1: Add ERD dialog HTML**

  After the last `</dialog>` before `</body>` (around line 2560 area, after `#fk-peek-popover`), add:

  ```html
  <!-- Dialog 26: Visual ERD Canvas -->
  <dialog id="erd-dialog" class="m-auto bg-slate-950 border border-slate-700 text-white rounded-xl shadow-2xl p-0 outline-none w-[96vw] max-w-[1400px] h-[90vh] backdrop:bg-black/75 backdrop:backdrop-blur-sm overflow-hidden flex flex-col">
    <div class="flex items-center justify-between px-4 py-2.5 border-b border-slate-700 bg-slate-900 flex-shrink-0">
      <div class="flex items-center space-x-3">
        <span class="font-semibold text-sm text-white">🗺️ Entity Relationship Diagram</span>
        <span id="erd-table-count" class="text-xs text-slate-400"></span>
      </div>
      <div class="flex items-center space-x-2">
        <button onclick="erdAutoLayout()" class="h-7 bg-slate-800 hover:bg-slate-700 text-slate-300 px-2.5 rounded border border-slate-600 text-xs transition" title="Auto-arrange tables">⚡ Auto Layout</button>
        <button onclick="erdFitAll()" class="h-7 bg-slate-800 hover:bg-slate-700 text-slate-300 px-2.5 rounded border border-slate-600 text-xs transition" title="Fit all tables in view">⊡ Fit All</button>
        <button onclick="document.getElementById('erd-dialog').close()" class="h-7 w-7 bg-slate-800 hover:bg-rose-900 text-slate-400 hover:text-rose-300 rounded border border-slate-600 text-sm transition flex items-center justify-center" title="Close (Escape)">✕</button>
      </div>
    </div>
    <!-- Canvas area: SVG lines behind, div cards on top -->
    <div id="erd-canvas-wrapper" class="relative flex-1 overflow-hidden bg-slate-950 cursor-grab select-none" style="background-image: radial-gradient(circle, #334155 1px, transparent 1px); background-size: 24px 24px;">
      <div id="erd-viewport" style="position:absolute;top:0;left:0;transform-origin:0 0;will-change:transform;">
        <svg id="erd-svg" style="position:absolute;top:0;left:0;overflow:visible;pointer-events:none;"></svg>
        <div id="erd-cards" style="position:absolute;top:0;left:0;"></div>
      </div>
    </div>
    <div class="px-3 py-1.5 border-t border-slate-800 bg-slate-900 flex items-center justify-between text-[11px] text-slate-500 flex-shrink-0">
      <span>Drag tables to reposition • Scroll to zoom • Middle-click drag to pan</span>
      <span id="erd-zoom-label" class="font-mono">100%</span>
    </div>
  </dialog>
  ```

- [ ] **Step 2: Add ERD trigger to DB Tools navbar dropdown**

  In the `📦 DB Tools ▾` dropdown (search for the dropdown items near `🔄 Structure Sync`), add a new item:

  ```html
  <button onclick="openERDCanvas(); document.getElementById('db-tools-dropdown').classList.add('hidden')" class="w-full text-left px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-700 hover:text-white transition flex items-center space-x-2">
    <span>🗺️</span><span>Visual ERD</span>
  </button>
  ```

- [ ] **Step 3: Verify dialog opens/closes**

  Restart server, open browser, click `📦 DB Tools ▾` → `🗺️ Visual ERD` → dialog opens. Press Escape to close. No JS errors in console.

- [ ] **Step 4: Commit**

  ```bash
  git add web/dist/index.html
  git commit -m "feat(erd): add visual erd dialog shell and navbar trigger"
  ```

---

## Task 2: ERD State & Schema Loading

**Files:**
- Modify: `web/dist/index.html` — add `erdState` object and `openERDCanvas()` function that fetches schema and populates state

**Interfaces:**
- Consumes: `GET /api/session/{id}/schema?database=<db>` → `{ tables: [{ name, columns:[{name,type,key}], foreign_keys:[{column,referenced_table,referenced_column}] }] }`
- Consumes: `activeConnection.id`, `currentDb` (existing globals)
- Produces: `erdState = { tables:[], positions:{}, scale:1, panX:0, panY:0 }`; `openERDCanvas()` global

- [ ] **Step 1: Add ERD state + open function**

  Add this JS block near other modal opener functions:

  ```javascript
  // --- Visual ERD Canvas ---
  const erdState = {
    tables: [],      // TableMetadata[] from schema API
    positions: {},   // { [tableName]: { x, y } } — persisted per-database in sessionStorage
    scale: 1,
    panX: 0,
    panY: 0,
    draggingCard: null,   // { name, startMouseX, startMouseY, startCardX, startCardY }
    panning: false,
    panStart: null,       // { mouseX, mouseY, panX, panY }
  };

  async function openERDCanvas() {
    if (!activeConnection) { showNativeAlert('Connect to a session first.', 'ERD'); return; }
    const db = currentDb || '';
    const dialog = document.getElementById('erd-dialog');
    dialog.showModal();
    document.getElementById('erd-table-count').textContent = 'Loading…';
    try {
      const url = `/api/session/${activeConnection.id}/schema${db ? '?database=' + encodeURIComponent(db) : ''}`;
      const res = await apiFetch(url);
      const data = await res.json();
      erdState.tables = data.tables || [];
      erdState.scale = 1;
      erdState.panX = 0;
      erdState.panY = 0;
      // Restore positions from sessionStorage
      const storeKey = `erd-pos-${activeConnection.id}-${db}`;
      try { erdState.positions = JSON.parse(sessionStorage.getItem(storeKey) || '{}'); } catch { erdState.positions = {}; }
      erdAutoLayout(false); // layout only tables with no stored position
      erdRender();
    } catch (err) {
      document.getElementById('erd-table-count').textContent = 'Failed to load schema';
      console.error('ERD load error', err);
    }
  }
  ```

- [ ] **Step 2: Verify state loads**

  Open ERD while connected. Check browser console: `erdState.tables.length` should equal number of tables in current DB. No 404 or parse errors.

---

## Task 3: Auto-Layout Algorithm

**Files:**
- Modify: `web/dist/index.html` — add `erdAutoLayout()` function

**Interfaces:**
- Consumes: `erdState.tables`, `erdState.positions`
- Produces: fills `erdState.positions` for tables without a position; saves to `sessionStorage`

- [ ] **Step 1: Add auto-layout function**

  ```javascript
  function erdAutoLayout(force = true) {
    // ponytail: simple grid layout. Ceiling: no FK-aware topological sort.
    // Upgrade: topological sort by FK depth for cleaner flow.
    const CARD_W = 220, CARD_H_BASE = 80, COL_GAP = 60, ROW_GAP = 40;
    const COLS = Math.max(1, Math.ceil(Math.sqrt(erdState.tables.length)));
    let col = 0, row = 0, rowMaxH = 0;
    erdState.tables.forEach(t => {
      if (!force && erdState.positions[t.name]) return; // keep existing
      const h = CARD_H_BASE + (t.columns || []).length * 20;
      erdState.positions[t.name] = {
        x: col * (CARD_W + COL_GAP) + 40,
        y: row * (rowMaxH + ROW_GAP) + 40,
      };
      rowMaxH = Math.max(rowMaxH, h);
      col++;
      if (col >= COLS) { col = 0; row++; rowMaxH = 0; }
    });
    erdSavePositions();
  }

  function erdSavePositions() {
    if (!activeConnection) return;
    const db = currentDb || '';
    const storeKey = `erd-pos-${activeConnection.id}-${db}`;
    sessionStorage.setItem(storeKey, JSON.stringify(erdState.positions));
  }
  ```

---

## Task 4: Render Tables as Cards

**Files:**
- Modify: `web/dist/index.html` — add `erdRender()` and `erdRenderCards()` functions

**Interfaces:**
- Consumes: `erdState.tables`, `erdState.positions`, `erdState.scale`, `erdState.panX/panY`
- Produces: DOM in `#erd-cards`; sets `#erd-viewport` transform; calls `erdRenderLines()`

- [ ] **Step 1: Add erdRender + erdRenderCards**

  ```javascript
  function erdRender() {
    erdApplyTransform();
    erdRenderCards();
    erdRenderLines();
    const count = erdState.tables.length;
    const fkCount = erdState.tables.reduce((s, t) => s + (t.foreign_keys || []).length, 0);
    document.getElementById('erd-table-count').textContent =
      `${count} table${count !== 1 ? 's' : ''} · ${fkCount} relationship${fkCount !== 1 ? 's' : ''}`;
    document.getElementById('erd-zoom-label').textContent = Math.round(erdState.scale * 100) + '%';
  }

  function erdApplyTransform() {
    const vp = document.getElementById('erd-viewport');
    if (vp) vp.style.transform = `translate(${erdState.panX}px,${erdState.panY}px) scale(${erdState.scale})`;
  }

  function erdRenderCards() {
    const container = document.getElementById('erd-cards');
    if (!container) return;
    container.innerHTML = '';
    erdState.tables.forEach(t => {
      const pos = erdState.positions[t.name] || { x: 40, y: 40 };
      const fkCols = new Set((t.foreign_keys || []).map(fk => fk.column));
      const pkCols = new Set((t.columns || []).filter(c => c.key === 'PRI').map(c => c.name));

      const card = document.createElement('div');
      card.dataset.table = t.name;
      card.className = 'erd-card absolute bg-slate-900 border border-slate-600 rounded-lg shadow-xl text-xs select-none overflow-hidden';
      card.style.cssText = `left:${pos.x}px;top:${pos.y}px;width:220px;cursor:grab;z-index:10;`;
      card.innerHTML = `
        <div class="erd-card-header bg-slate-800 px-2.5 py-1.5 font-semibold text-sky-300 border-b border-slate-700 flex items-center justify-between" style="cursor:grab;">
          <span class="truncate" title="${escapeHtml(t.name)}">📋 ${escapeHtml(t.name)}</span>
          <span class="text-slate-500 text-[10px] font-mono ml-1">${(t.columns||[]).length}</span>
        </div>
        <div class="erd-card-cols divide-y divide-slate-800">
          ${(t.columns || []).slice(0, 15).map(c => `
            <div class="px-2.5 py-0.5 flex items-center justify-between hover:bg-slate-800/60 transition" title="${escapeHtml(c.name)}: ${escapeHtml(c.type||'')}">
              <span class="font-mono truncate text-[11px] ${fkCols.has(c.name) ? 'text-amber-300' : pkCols.has(c.name) ? 'text-emerald-400' : 'text-slate-300'}" style="max-width:130px;">
                ${pkCols.has(c.name) ? '🔑 ' : fkCols.has(c.name) ? '🔗 ' : ''}${escapeHtml(c.name)}
              </span>
              <span class="text-slate-500 text-[10px] font-mono ml-1 flex-shrink-0">${escapeHtml((c.type||'').split('(')[0].toUpperCase())}</span>
            </div>
          `).join('')}
          ${(t.columns||[]).length > 15 ? `<div class="px-2.5 py-0.5 text-slate-500 text-[10px]">+${(t.columns||[]).length - 15} more…</div>` : ''}
        </div>
      `;
      card.addEventListener('mousedown', erdOnCardMouseDown);
      container.appendChild(card);
    });
  }
  ```

- [ ] **Step 2: Verify cards render**

  Open ERD. Should see table cards with column lists. PK columns highlighted green, FK columns amber. No errors.

---

## Task 5: FK Relationship Lines (SVG)

**Files:**
- Modify: `web/dist/index.html` — add `erdRenderLines()` function

**Interfaces:**
- Consumes: `erdState.tables`, `erdState.positions`, `#erd-svg` SVG element
- Produces: SVG `<line>` elements between FK source card and target card

- [ ] **Step 1: Add erdRenderLines**

  ```javascript
  function erdRenderLines() {
    const svg = document.getElementById('erd-svg');
    if (!svg) return;
    svg.innerHTML = '';

    // Arrow marker definition
    const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
    defs.innerHTML = `
      <marker id="erd-arrow" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
        <path d="M0,0 L0,6 L8,3 z" fill="#f59e0b" opacity="0.8"/>
      </marker>
    `;
    svg.appendChild(defs);

    const CARD_W = 220;
    erdState.tables.forEach(srcTable => {
      (srcTable.foreign_keys || []).forEach(fk => {
        const srcPos = erdState.positions[srcTable.name];
        const dstPos = erdState.positions[fk.referenced_table];
        if (!srcPos || !dstPos) return;

        // Card heights: estimate from column count
        const srcH = 30 + (srcTable.columns || []).length * 20;
        const dstTable = erdState.tables.find(t => t.name === fk.referenced_table);
        const dstH = 30 + ((dstTable?.columns) || []).length * 20;

        const x1 = srcPos.x + CARD_W / 2;
        const y1 = srcPos.y + srcH / 2;
        const x2 = dstPos.x + CARD_W / 2;
        const y2 = dstPos.y + dstH / 2;

        const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
        line.setAttribute('x1', x1); line.setAttribute('y1', y1);
        line.setAttribute('x2', x2); line.setAttribute('y2', y2);
        line.setAttribute('stroke', '#f59e0b');
        line.setAttribute('stroke-width', '1.5');
        line.setAttribute('stroke-opacity', '0.6');
        line.setAttribute('marker-end', 'url(#erd-arrow)');
        svg.appendChild(line);

        // Label: "col → ref_col"
        const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        label.setAttribute('x', (x1 + x2) / 2);
        label.setAttribute('y', (y1 + y2) / 2 - 4);
        label.setAttribute('text-anchor', 'middle');
        label.setAttribute('fill', '#94a3b8');
        label.setAttribute('font-size', '10');
        label.setAttribute('font-family', 'monospace');
        label.textContent = `${fk.column} → ${fk.referenced_column}`;
        svg.appendChild(label);
      });
    });
  }
  ```

- [ ] **Step 2: Verify FK lines**

  Open ERD on a MySQL DB with FK constraints. Lines with amber arrows should connect related tables. Isolated tables (no FK) render without lines.

---

## Task 6: Drag-to-Reposition Cards

**Files:**
- Modify: `web/dist/index.html` — add card drag handlers and global mouse event listeners

**Interfaces:**
- Consumes: `erdState.draggingCard`, `erdState.scale`, `erdState.positions`
- Produces: updates `erdState.positions`, triggers `erdRenderLines()` on drag, saves to `sessionStorage`

- [ ] **Step 1: Add drag handlers**

  ```javascript
  function erdOnCardMouseDown(e) {
    if (e.button !== 0) return;
    e.stopPropagation();
    const card = e.currentTarget;
    const name = card.dataset.table;
    const pos = erdState.positions[name] || { x: 0, y: 0 };
    erdState.draggingCard = { name, startMouseX: e.clientX, startMouseY: e.clientY, startCardX: pos.x, startCardY: pos.y };
    card.style.cursor = 'grabbing';
    card.style.zIndex = '100';
    e.preventDefault();
  }

  function erdOnMouseMove(e) {
    if (erdState.draggingCard) {
      const dx = (e.clientX - erdState.draggingCard.startMouseX) / erdState.scale;
      const dy = (e.clientY - erdState.draggingCard.startMouseY) / erdState.scale;
      const nx = erdState.draggingCard.startCardX + dx;
      const ny = erdState.draggingCard.startCardY + dy;
      erdState.positions[erdState.draggingCard.name] = { x: Math.max(0, nx), y: Math.max(0, ny) };
      // Move card DOM directly (fast) then re-draw lines
      const card = document.querySelector(`[data-table="${CSS.escape(erdState.draggingCard.name)}"]`);
      if (card) { card.style.left = nx + 'px'; card.style.top = ny + 'px'; }
      erdRenderLines();
    } else if (erdState.panning && erdState.panStart) {
      erdState.panX = erdState.panStart.panX + (e.clientX - erdState.panStart.mouseX);
      erdState.panY = erdState.panStart.panY + (e.clientY - erdState.panStart.mouseY);
      erdApplyTransform();
    }
  }

  function erdOnMouseUp(e) {
    if (erdState.draggingCard) {
      const card = document.querySelector(`[data-table="${CSS.escape(erdState.draggingCard.name)}"]`);
      if (card) { card.style.cursor = 'grab'; card.style.zIndex = '10'; }
      erdState.draggingCard = null;
      erdSavePositions();
    }
    if (erdState.panning) {
      erdState.panning = false;
      erdState.panStart = null;
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (wrapper) wrapper.style.cursor = 'grab';
    }
  }
  ```

- [ ] **Step 2: Wire up pan on canvas background**

  On `#erd-canvas-wrapper` mousedown (not on a card):

  ```javascript
  function erdOnWrapperMouseDown(e) {
    if (e.button === 1 || (e.button === 0 && !e.target.closest('.erd-card'))) {
      erdState.panning = true;
      erdState.panStart = { mouseX: e.clientX, mouseY: e.clientY, panX: erdState.panX, panY: erdState.panY };
      e.currentTarget.style.cursor = 'grabbing';
      e.preventDefault();
    }
  }
  ```

- [ ] **Step 3: Wire up scroll-to-zoom**

  ```javascript
  function erdOnWheel(e) {
    e.preventDefault();
    const wrapper = document.getElementById('erd-canvas-wrapper');
    const rect = wrapper.getBoundingClientRect();
    // Mouse position relative to wrapper
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    const delta = e.deltaY > 0 ? 0.9 : 1.1;
    const newScale = Math.min(3, Math.max(0.2, erdState.scale * delta));
    // Adjust pan so zoom anchors at cursor
    erdState.panX = mx - (mx - erdState.panX) * (newScale / erdState.scale);
    erdState.panY = my - (my - erdState.panY) * (newScale / erdState.scale);
    erdState.scale = newScale;
    erdApplyTransform();
    document.getElementById('erd-zoom-label').textContent = Math.round(newScale * 100) + '%';
  }
  ```

- [ ] **Step 4: Register all listeners when dialog opens**

  In `openERDCanvas()`, after `erdRender()`:

  ```javascript
  const wrapper = document.getElementById('erd-canvas-wrapper');
  // Remove then re-add to avoid duplicate listeners
  wrapper.onmousedown = erdOnWrapperMouseDown;
  wrapper.onwheel = erdOnWheel;
  document.addEventListener('mousemove', erdOnMouseMove, { once: false });
  document.addEventListener('mouseup', erdOnMouseUp, { once: false });
  ```

  Clean up on close (add to dialog's `close` event once):

  ```javascript
  document.getElementById('erd-dialog').addEventListener('close', () => {
    document.removeEventListener('mousemove', erdOnMouseMove);
    document.removeEventListener('mouseup', erdOnMouseUp);
  }, { once: false });
  ```

- [ ] **Step 5: Verify drag + pan + zoom**

  - Drag a table card → it moves, FK lines follow
  - Scroll wheel → zooms toward cursor
  - Click empty canvas and drag → pans

- [ ] **Step 6: Commit**

  ```bash
  git add web/dist/index.html
  git commit -m "feat(erd): add draggable table cards, fk lines, pan, and zoom"
  ```

---

## Task 7: Fit-All & Toolbar UX

**Files:**
- Modify: `web/dist/index.html` — add `erdFitAll()` function

**Interfaces:**
- Consumes: `erdState.positions`, all card bounding boxes, `#erd-canvas-wrapper` dimensions

- [ ] **Step 1: Add erdFitAll**

  ```javascript
  function erdFitAll() {
    if (erdState.tables.length === 0) return;
    const CARD_W = 220;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    erdState.tables.forEach(t => {
      const pos = erdState.positions[t.name] || { x: 0, y: 0 };
      const h = 30 + (t.columns || []).length * 20;
      minX = Math.min(minX, pos.x);
      minY = Math.min(minY, pos.y);
      maxX = Math.max(maxX, pos.x + CARD_W);
      maxY = Math.max(maxY, pos.y + h);
    });
    const wrapper = document.getElementById('erd-canvas-wrapper');
    const ww = wrapper.clientWidth;
    const wh = wrapper.clientHeight;
    const contentW = maxX - minX + 80;
    const contentH = maxY - minY + 80;
    const scale = Math.min(1, ww / contentW, wh / contentH);
    erdState.scale = scale;
    erdState.panX = (ww - contentW * scale) / 2 - minX * scale + 40 * scale;
    erdState.panY = (wh - contentH * scale) / 2 - minY * scale + 40 * scale;
    erdApplyTransform();
    document.getElementById('erd-zoom-label').textContent = Math.round(scale * 100) + '%';
  }
  ```

- [ ] **Step 2: Call erdFitAll automatically on open**

  At end of `openERDCanvas()` after `erdRender()`, add `erdFitAll()`.

- [ ] **Step 3: Verify fit-all**

  Open ERD with 10+ tables. Canvas should fit all cards in view. Click `⊡ Fit All` to reset after manually panning.

- [ ] **Step 4: Commit**

  ```bash
  git add web/dist/index.html
  git commit -m "feat(erd): add fit-all, auto-layout button, and zoom label"
  ```

---

## Task 8: MILESTONES.md & ADR

**Files:**
- Modify: `MILESTONES.md` — mark checklist item 16 `[x]`, add completion note
- Create: `docs/adr/0029-visual-erd-canvas.md`

- [ ] **Step 1: Write ADR**

  ```markdown
  # ADR-0029: Visual ERD Interactive Canvas

  **Status:** Accepted  
  **Date:** 2026-10-02

  ## Context
  Users need a quick visual overview of table relationships in any connected database.

  ## Decision
  Implemented as a pure-frontend `<dialog>` using SVG for FK relationship arrows and HTML divs for draggable table cards.
  Uses existing `GET /api/session/{id}/schema` data — zero new backend endpoints.
  Positions persisted in `sessionStorage` keyed by connection+database.

  ## Consequences
  - No backend changes; zero new dependencies.
  - SVG line placement is approximate (center-to-center); pixel-perfect port anchoring is a future upgrade path.
  - Positions reset between page reloads on different sessions (sessionStorage, not localStorage).
  ```

- [ ] **Step 2: Mark checklist done in MILESTONES.md**

  Change `[ ] 16. Visual ERD Interactive Canvas` → `[x] 16. Visual ERD Interactive Canvas`.

- [ ] **Step 3: Final commit**

  ```bash
  go test ./... # verify green
  git add docs/adr/0029-visual-erd-canvas.md MILESTONES.md
  git commit -m "docs(erd): add adr-0029 and mark visual erd milestone complete"
  ```
