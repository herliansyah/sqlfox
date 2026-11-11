<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import type { TableMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';

  const CARD_WIDTH = 220;
  const HEADER_HEIGHT = 38;
  const ROW_HEIGHT = 24;

  let containerRef: HTMLDivElement | null = $state(null);
  let scale = $state(1);
  let panX = $state(50);
  let panY = $state(50);
  let isPanning = $state(false);
  let startPanX = 0;
  let startPanY = 0;

  // Search filter
  let searchQuery = $state('');

  // Dragging a specific card
  let draggingTable = $state<string | null>(null);
  let dragOffset = { x: 0, y: 0 };

  // Hovered / selected table or FK
  let selectedTable = $state<string | null>(null);
  let hoveredFK = $state<string | null>(null);

  // Expanded tables set (tables with >12 columns)
  let expandedTables = $state<Record<string, boolean>>({});

  // Table positions in canvas coordinates
  let positions = $state<Record<string, { x: number; y: number }>>({});

  let tables = $derived<TableMetadata[]>(appState.catalog?.tables || []);

  // Compute foreign key lines
  interface FKLine {
    key: string;
    srcTable: string;
    srcCol: string;
    dstTable: string;
    dstCol: string;
    path: string;
    midX: number;
    midY: number;
    label: string;
    isSelected: boolean;
    isHovered: boolean;
    isDimmed: boolean;
  }

  $effect(() => {
    // Initialize or load positions when catalog tables change
    if (tables.length > 0) {
      loadOrAutoLayoutPositions();
    }
  });

  function loadOrAutoLayoutPositions() {
    const connId = appState.activeConnection?.id || 'default';
    const db = appState.currentDatabase || 'default';
    const storageKey = `sqlfox_erd_${connId}_${db}`;
    try {
      const saved = localStorage.getItem(storageKey);
      if (saved) {
        const parsed = JSON.parse(saved);
        if (parsed && typeof parsed === 'object') {
          positions = parsed;
          return;
        }
      }
    } catch (_) {}

    autoLayout();
  }

  function savePositions() {
    const connId = appState.activeConnection?.id || 'default';
    const db = appState.currentDatabase || 'default';
    const storageKey = `sqlfox_erd_${connId}_${db}`;
    try {
      localStorage.setItem(storageKey, JSON.stringify(positions));
    } catch (_) {}
  }

  export function autoLayout() {
    if (tables.length === 0) return;
    const ROW_GAP = 50;
    const COL_GAP = 80;

    // Topological depth layout
    const depth: Record<string, number> = {};
    tables.forEach((t) => (depth[t.name] = 0));

    for (let pass = 0; pass < 3; pass++) {
      tables.forEach((t) => {
        (t.foreign_keys || []).forEach((fk) => {
          const ref = fk.referenced_table;
          if (depth[ref] !== undefined && depth[t.name] <= depth[ref]) {
            depth[t.name] = depth[ref] + 1;
          }
        });
      });
    }

    const layers: Record<number, TableMetadata[]> = {};
    tables.forEach((t) => {
      const d = depth[t.name] || 0;
      if (!layers[d]) layers[d] = [];
      layers[d].push(t);
    });

    const newPos: Record<string, { x: number; y: number }> = {};
    let curX = 60;
    const layerIndices = Object.keys(layers)
      .map(Number)
      .sort((a, b) => a - b);

    layerIndices.forEach((layerIdx) => {
      const tbls = layers[layerIdx];
      let curY = 60;
      tbls.forEach((t) => {
        newPos[t.name] = { x: curX, y: curY };
        const colCount = Math.min(expandedTables[t.name] ? 100 : 12, (t.columns || []).length);
        const cardH = HEADER_HEIGHT + colCount * ROW_HEIGHT + 20;
        curY += cardH + ROW_GAP;
      });
      curX += CARD_WIDTH + COL_GAP;
    });

    positions = newPos;
    savePositions();
  }

  function getCardHeight(t: TableMetadata): number {
    const isExp = Boolean(expandedTables[t.name]);
    const colCount = Math.min(isExp ? 100 : 12, (t.columns || []).length);
    const hasMore = (t.columns || []).length > 12;
    return HEADER_HEIGHT + colCount * ROW_HEIGHT + (hasMore ? 26 : 8);
  }

  let lines = $derived.by<FKLine[]>(() => {
    const list: FKLine[] = [];
    const sel = selectedTable;

    tables.forEach((src) => {
      (src.foreign_keys || []).forEach((fk) => {
        const sp = positions[src.name];
        const dst = tables.find(
          (t) => t.name.toLowerCase() === fk.referenced_table.toLowerCase()
        );
        const dp = dst ? positions[dst.name] : positions[fk.referenced_table];
        if (!sp || !dp) return;

        const fkKey = `${src.name}.${fk.column}->${fk.referenced_table}.${fk.referenced_column}`;
        const isSelected = Boolean(
          sel && (src.name === sel || (dst?.name || fk.referenced_table) === sel)
        );
        const isHovered = hoveredFK === fkKey;
        const isDimmed = Boolean(sel && !isSelected);

        // Calculate Y offsets based on column indices
        const srcCols = src.columns || [];
        const srcIdx = srcCols.findIndex((c) => c.name === fk.column);
        const isSrcExp = Boolean(expandedTables[src.name]);
        const srcLimit = isSrcExp ? srcCols.length : 12;
        const y1 =
          sp.y +
          HEADER_HEIGHT +
          (srcIdx >= 0 && srcIdx < srcLimit ? srcIdx * ROW_HEIGHT + ROW_HEIGHT / 2 : ROW_HEIGHT / 2);

        const dstCols = dst?.columns || [];
        const dstIdx = dstCols.findIndex((c) => c.name === fk.referenced_column);
        const isDstExp = dst ? Boolean(expandedTables[dst.name]) : false;
        const dstLimit = isDstExp ? dstCols.length : 12;
        const y2 =
          dp.y +
          HEADER_HEIGHT +
          (dstIdx >= 0 && dstIdx < dstLimit ? dstIdx * ROW_HEIGHT + ROW_HEIGHT / 2 : ROW_HEIGHT / 2);

        let x1: number, x2: number, cx1: number, cx2: number;
        if (sp.x + CARD_WIDTH + 30 < dp.x) {
          x1 = sp.x + CARD_WIDTH;
          x2 = dp.x;
          const dx = Math.max(35, (x2 - x1) * 0.45);
          cx1 = x1 + dx;
          cx2 = x2 - dx;
        } else if (dp.x + CARD_WIDTH + 30 < sp.x) {
          x1 = sp.x;
          x2 = dp.x + CARD_WIDTH;
          const dx = Math.max(35, (x1 - x2) * 0.45);
          cx1 = x1 - dx;
          cx2 = x2 + dx;
        } else {
          if (sp.x >= dp.x) {
            x1 = sp.x + CARD_WIDTH;
            x2 = dp.x + CARD_WIDTH;
            const dx = 45 + Math.abs(y1 - y2) * 0.12;
            cx1 = Math.max(x1, x2) + dx;
            cx2 = Math.max(x1, x2) + dx;
          } else {
            x1 = sp.x;
            x2 = dp.x;
            const dx = 45 + Math.abs(y1 - y2) * 0.12;
            cx1 = Math.min(x1, x2) - dx;
            cx2 = Math.min(x1, x2) - dx;
          }
        }

        const path = `M ${x1} ${y1} C ${cx1} ${y1}, ${cx2} ${y2}, ${x2} ${y2}`;
        list.push({
          key: fkKey,
          srcTable: src.name,
          srcCol: fk.column,
          dstTable: fk.referenced_table,
          dstCol: fk.referenced_column,
          path,
          midX: (x1 + x2) / 2,
          midY: (y1 + y2) / 2,
          label: `${fk.column} → ${fk.referenced_column}`,
          isSelected,
          isHovered,
          isDimmed,
        });
      });
    });

    return list;
  });

  // Pan & Zoom controls
  function handleMouseDown(e: MouseEvent) {
    if ((e.target as HTMLElement).closest('.erd-card, .erd-btn, input, button')) return;
    isPanning = true;
    startPanX = e.clientX - panX;
    startPanY = e.clientY - panY;
    window.addEventListener('mousemove', onPanMove);
    window.addEventListener('mouseup', onPanUp);
  }

  function onPanMove(e: MouseEvent) {
    if (!isPanning) return;
    panX = e.clientX - startPanX;
    panY = e.clientY - startPanY;
  }

  function onPanUp() {
    isPanning = false;
    window.removeEventListener('mousemove', onPanMove);
    window.removeEventListener('mouseup', onPanUp);
  }

  function handleWheel(e: WheelEvent) {
    e.preventDefault();
    const zoomFactor = e.deltaY < 0 ? 1.08 : 0.92;
    const newScale = Math.min(2.5, Math.max(0.25, scale * zoomFactor));

    // Zoom towards cursor
    if (containerRef) {
      const rect = containerRef.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;
      const mouseY = e.clientY - rect.top;
      panX = mouseX - (mouseX - panX) * (newScale / scale);
      panY = mouseY - (mouseY - panY) * (newScale / scale);
    }
    scale = newScale;
  }

  function zoomStep(delta: number) {
    scale = Math.min(2.5, Math.max(0.25, scale + delta));
  }

  function resetZoom() {
    scale = 1;
    panX = 60;
    panY = 60;
  }

  function fitAll() {
    if (tables.length === 0 || !containerRef) return;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    tables.forEach((t) => {
      const p = positions[t.name] || { x: 0, y: 0 };
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + CARD_WIDTH);
      maxY = Math.max(maxY, p.y + getCardHeight(t));
    });

    const w = containerRef.clientWidth || 800;
    const h = containerRef.clientHeight || 600;
    const contentW = maxX - minX + 100;
    const contentH = maxY - minY + 100;

    const newScale = Math.min(1.2, Math.max(0.3, Math.min(w / contentW, h / contentH)));
    scale = newScale;
    panX = (w - contentW * newScale) / 2 - minX * newScale + 50;
    panY = (h - contentH * newScale) / 2 - minY * newScale + 50;
  }

  // Card dragging
  function startCardDrag(e: MouseEvent, tableName: string) {
    e.stopPropagation();
    draggingTable = tableName;
    const pos = positions[tableName] || { x: 0, y: 0 };
    dragOffset = {
      x: e.clientX / scale - pos.x,
      y: e.clientY / scale - pos.y,
    };
    window.addEventListener('mousemove', onCardDragMove);
    window.addEventListener('mouseup', onCardDragUp);
  }

  function onCardDragMove(e: MouseEvent) {
    if (!draggingTable) return;
    positions[draggingTable] = {
      x: Math.round(e.clientX / scale - dragOffset.x),
      y: Math.round(e.clientY / scale - dragOffset.y),
    };
  }

  function onCardDragUp() {
    draggingTable = null;
    savePositions();
    window.removeEventListener('mousemove', onCardDragMove);
    window.removeEventListener('mouseup', onCardDragUp);
  }

  function escapeXml(unsafe: string): string {
    return (unsafe || '').replace(/[<>&'"]/g, (c) => {
      switch (c) {
        case '<': return '&lt;';
        case '>': return '&gt;';
        case '&': return '&amp;';
        case '\'': return '&apos;';
        case '"': return '&quot;';
        default: return c;
      }
    });
  }

  // Export functions
  function exportSVG() {
    if (tables.length === 0) {
      appState.showToast('No tables to export', 'warning');
      return;
    }

    const CARD_W = 240;
    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;

    tables.forEach((t) => {
      const p = positions[t.name] || { x: 0, y: 0 };
      const h = 50 + Math.min(15, (t.columns || []).length) * 24;
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + CARD_W);
      maxY = Math.max(maxY, p.y + h);
    });

    const pad = 40;
    const w = Math.max(800, maxX - minX + pad * 2);
    const h = Math.max(600, maxY - minY + pad * 2);

    let cardsSvg = '';
    tables.forEach((t) => {
      const p = positions[t.name] || { x: 0, y: 0 };
      const x = p.x - minX + pad;
      const y = p.y - minY + pad;
      const cols = (t.columns || []).slice(0, 15);
      const cardH = 34 + cols.length * 24;

      cardsSvg += `<g transform="translate(${x},${y})">
        <rect width="${CARD_W}" height="${cardH}" rx="8" fill="#0f172a" stroke="#334155" stroke-width="1"/>
        <path d="M0,8 a8,8 0 0 1 8,-8 h${CARD_W - 16} a8,8 0 0 1 8,8 v26 h-${CARD_W} z" fill="#1e293b"/>
        <text x="12" y="22" fill="#38bdf8" font-family="'JetBrains Mono',monospace" font-size="12" font-weight="bold">${escapeXml(t.name)}</text>
        <text x="${CARD_W - 12}" y="22" text-anchor="end" fill="#64748b" font-family="'JetBrains Mono',monospace" font-size="10">${cols.length}</text>`;

      cols.forEach((c, idx) => {
        const cy = 34 + idx * 24;
        const isPK = c.is_primary_key;
        const isFK = (t.foreign_keys || []).some((fk) => fk.column === c.name);
        const colColor = isPK ? '#34d399' : isFK ? '#fbbf24' : '#cbd5e1';
        const prefix = isPK ? '[PK] ' : isFK ? '[FK] ' : '';
        const typeStr = escapeXml((c.type || '').split('(')[0].toUpperCase());

        cardsSvg += `
          <line x1="0" y1="${cy}" x2="${CARD_W}" y2="${cy}" stroke="#1e293b" stroke-width="1"/>
          <text x="12" y="${cy + 16}" fill="${colColor}" font-family="'JetBrains Mono',monospace" font-size="10">${prefix}${escapeXml(c.name)}</text>
          <text x="${CARD_W - 12}" y="${cy + 16}" text-anchor="end" fill="#64748b" font-family="'JetBrains Mono',monospace" font-size="9">${typeStr}</text>`;
      });

      cardsSvg += `</g>`;
    });

    const linesSvg = (lines || [])
      .map((line) => `<path d="${line.path}" stroke="#f59e0b" stroke-width="2" fill="none" opacity="0.85" marker-end="url(#erd-arrow-export)"/>`)
      .join('\n');

    const fullSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
      <rect width="${w}" height="${h}" fill="#020617"/>
      <defs>
        <marker id="erd-arrow-export" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="#f59e0b" opacity="0.85"/>
        </marker>
      </defs>
      <g transform="translate(${-minX + pad},${-minY + pad})">
        ${linesSvg}
      </g>
      ${cardsSvg}
    </svg>`;

    const blob = new Blob([fullSvg], { type: 'image/svg+xml;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${appState.currentDatabase || 'database'}_erd.svg`;
    a.click();
    URL.revokeObjectURL(url);
    appState.showToast('Exported complete ERD diagram as SVG', 'success');
  }

  function exportMermaid() {
    let out = 'erDiagram\n';
    tables.forEach((src) => {
      (src.foreign_keys || []).forEach((fk) => {
        out += `  ${src.name} }o--|| ${fk.referenced_table} : "${fk.column} -> ${fk.referenced_column}"\n`;
      });
    });
    tables.forEach((t) => {
      out += `  ${t.name} {\n`;
      (t.columns || []).forEach((c) => {
        const typeClean = (c.type || 'varchar').replace(/[(), ]/g, '_');
        const pk = c.is_primary_key ? 'PK' : '';
        out += `    ${typeClean} ${c.name} ${pk}\n`;
      });
      out += '  }\n';
    });
    navigator.clipboard.writeText(out);
    appState.showToast('Copied Mermaid ERD syntax to clipboard!', 'success');
  }

  let minimapCanvasRef: HTMLCanvasElement | null = $state(null);

  function renderMinimap() {
    if (!minimapCanvasRef || tables.length === 0) return;
    const ctx = minimapCanvasRef.getContext('2d');
    if (!ctx) return;
    const cw = minimapCanvasRef.width;
    const ch = minimapCanvasRef.height;
    ctx.clearRect(0, 0, cw, ch);

    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    tables.forEach((t) => {
      const p = positions[t.name] || { x: 0, y: 0 };
      const h = getCardHeight(t);
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + CARD_WIDTH);
      maxY = Math.max(maxY, p.y + h);
    });

    const pad = 40;
    const worldW = Math.max(100, maxX - minX + pad * 2);
    const worldH = Math.max(100, maxY - minY + pad * 2);
    const s = Math.min(cw / worldW, ch / worldH);

    tables.forEach((t) => {
      const p = positions[t.name] || { x: 0, y: 0 };
      const h = getCardHeight(t);
      const rx = (p.x - minX + pad) * s;
      const ry = (p.y - minY + pad) * s;
      const rw = CARD_WIDTH * s;
      const rh = h * s;
      ctx.fillStyle = t.name === selectedTable ? '#38bdf8' : '#334155';
      ctx.fillRect(rx, ry, Math.max(2, rw), Math.max(2, rh));
    });

    if (containerRef) {
      const vw = containerRef.clientWidth / scale;
      const vh = containerRef.clientHeight / scale;
      const vx = (-panX / scale - minX + pad) * s;
      const vy = (-panY / scale - minY + pad) * s;
      ctx.strokeStyle = '#38bdf8';
      ctx.lineWidth = 1;
      ctx.strokeRect(vx, vy, vw * s, vh * s);
    }
  }

  $effect(() => {
    // Re-render minimap reactively
    const _pos = positions;
    const _s = scale;
    const _px = panX;
    const _py = panY;
    const _sel = selectedTable;
    if (minimapCanvasRef) {
      renderMinimap();
    }
  });
</script>

<div
  bind:this={containerRef}
  onmousedown={handleMouseDown}
  onwheel={handleWheel}
  role="region"
  aria-label="ERD Canvas"
  class="h-full w-full bg-[#070b14] relative overflow-hidden select-none cursor-grab active:cursor-grabbing"
  style="background-image: radial-gradient(#1e293b 1px, transparent 1px); background-size: {Math.max(12, Math.round(24 * scale))}px {Math.max(12, Math.round(24 * scale))}px; background-position: {panX}px {panY}px;"
>
  <!-- Top Floating Toolbar -->
  <div class="absolute top-3 left-3 right-3 z-30 flex items-center justify-between pointer-events-none">
    <!-- Left: Title, Stats & Filter -->
    <div class="flex items-center space-x-2 bg-[#0f172a]/90 backdrop-blur-md border border-[#334155] rounded-lg p-1.5 shadow-2xl pointer-events-auto">
      <div class="flex items-center space-x-1.5 px-2 text-xs font-semibold text-slate-200">
        <Icon name="layers" class="w-4 h-4 text-sky-400" />
        <span>ERD Canvas:</span>
        <span class="font-mono text-sky-400">{appState.currentDatabase || 'None'}</span>
        <span class="text-slate-500 font-mono text-[11px]">
          ({tables.length} tables • {lines.length} FKs)
        </span>
      </div>

      <div class="h-4 w-px bg-slate-700"></div>

      <!-- Search Filter -->
      <div class="relative flex items-center">
        <Icon name="search" class="w-3 h-3 text-slate-500 absolute left-2 pointer-events-none" />
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Filter tables..."
          class="bg-[#090d16] border border-slate-700 focus:border-sky-500 rounded pl-7 pr-4 py-1 text-xs text-slate-200 font-mono outline-none w-36 sm:w-48"
        />
        {#if searchQuery}
          <button
            type="button"
            onclick={() => (searchQuery = '')}
            class="absolute right-1 top-1.5 text-slate-400 hover:text-white p-0.5 rounded"
            aria-label="Clear search"
          >
            <Icon name="x" class="w-3 h-3" />
          </button>
        {/if}
      </div>
    </div>

    <!-- Right: Zoom & Export Controls -->
    <div class="flex items-center space-x-1.5 bg-[#0f172a]/90 backdrop-blur-md border border-[#334155] rounded-lg p-1.5 shadow-2xl pointer-events-auto">
      <button
        onclick={autoLayout}
        class="px-2.5 py-1 bg-[#1e293b] hover:bg-slate-700 border border-slate-700 text-slate-200 rounded text-xs flex items-center space-x-1 transition"
        title="Auto-arrange tables by foreign key depth"
      >
        <Icon name="sliders" class="w-3.5 h-3.5 text-sky-400" />
        <span>Auto-Layout</span>
      </button>

      <button
        onclick={fitAll}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition"
        title="Fit All Tables into View"
      >
        <Icon name="maximize" class="w-3.5 h-3.5" />
      </button>

      <button
        onclick={() => zoomStep(0.15)}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition"
        title="Zoom In"
      >
        <Icon name="plus" class="w-3.5 h-3.5" />
      </button>

      <span class="text-[11px] font-mono text-slate-400 px-1 min-w-[42px] text-center">
        {Math.round(scale * 100)}%
      </span>

      <button
        onclick={() => zoomStep(-0.15)}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition"
        title="Zoom Out"
      >
        <span class="w-3.5 text-center font-bold text-sm leading-none">-</span>
      </button>

      <button
        onclick={resetZoom}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition text-xs"
        title="Reset Zoom to 100%"
      >
        <Icon name="refresh-cw" class="w-3.5 h-3.5 text-slate-400" />
      </button>

      <div class="h-4 w-px bg-slate-700"></div>

      <!-- Export Buttons -->
      <button
        onclick={exportSVG}
        class="px-2 py-1 bg-sky-950/60 hover:bg-sky-900/60 border border-sky-800 text-sky-300 font-semibold rounded text-xs flex items-center space-x-1 transition"
        title="Download diagram as SVG vector image"
      >
        <Icon name="share-2" class="w-3.5 h-3.5" />
        <span>SVG</span>
      </button>

      <button
        onclick={exportMermaid}
        class="px-2 py-1 bg-[#1e293b] hover:bg-slate-700 border border-slate-700 text-slate-200 rounded text-xs flex items-center space-x-1 transition"
        title="Copy Mermaid.js ERD diagram syntax to clipboard"
      >
        <Icon name="code" class="w-3.5 h-3.5 text-purple-400" />
        <span>Mermaid</span>
      </button>
    </div>
  </div>

  <!-- Zoom & Pan Transform Layer -->
  <div
    class="absolute inset-0 origin-top-left pointer-events-none"
    style="transform: translate({panX}px, {panY}px) scale({scale});"
  >
    <!-- SVG Connector Lines Layer -->
    <svg class="erd-svg-content absolute inset-0 w-[8000px] h-[8000px] overflow-visible pointer-events-none">
      <defs>
        <marker
          id="erd-arrow"
          viewBox="0 0 10 10"
          refX="8"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M 0 1 L 10 5 L 0 9 z" fill="#f59e0b" />
        </marker>
        <marker
          id="erd-arrow-active"
          viewBox="0 0 10 10"
          refX="8"
          refY="5"
          markerWidth="7"
          markerHeight="7"
          orient="auto-start-reverse"
        >
          <path d="M 0 0 L 10 5 L 0 10 z" fill="#38bdf8" />
        </marker>
      </defs>

      {#each lines as line (line.key)}
        <g class="pointer-events-auto">
          <!-- Hitarea for easier hover -->
          <path
            d={line.path}
            stroke="transparent"
            stroke-width="16"
            fill="none"
            class="cursor-pointer"
            onmouseenter={() => (hoveredFK = line.key)}
            onmouseleave={() => (hoveredFK = null)}
            onclick={() => (selectedTable = line.srcTable)}
          />
          <!-- Visible Path -->
          <path
            d={line.path}
            stroke={line.isHovered || line.isSelected ? '#38bdf8' : '#f59e0b'}
            stroke-width={line.isHovered || line.isSelected ? 2.5 : 1.5}
            stroke-opacity={line.isDimmed ? 0.15 : line.isHovered || line.isSelected ? 1 : 0.65}
            fill="none"
            marker-end={line.isHovered || line.isSelected ? 'url(#erd-arrow-active)' : 'url(#erd-arrow)'}
          />
          <!-- Badge on Midpoint -->
          {#if !line.isDimmed}
            <g
              transform="translate({line.midX}, {line.midY})"
              class="cursor-pointer select-none"
              onmouseenter={() => (hoveredFK = line.key)}
              onmouseleave={() => (hoveredFK = null)}
              onclick={() => (selectedTable = line.srcTable)}
            >
              <rect
                x={-Math.max(50, line.label.length * 3.5 + 8)}
                y={-8}
                width={Math.max(100, line.label.length * 7 + 16)}
                height="16"
                rx="4"
                fill="#090d16"
                stroke={line.isHovered || line.isSelected ? '#38bdf8' : '#334155'}
                stroke-width="1"
              />
              <text
                x="0"
                y="3"
                text-anchor="middle"
                fill={line.isHovered || line.isSelected ? '#38bdf8' : '#94a3b8'}
                font-size="9"
                font-family="monospace"
              >
                {line.label}
              </text>
            </g>
          {/if}
        </g>
      {/each}
    </svg>

    <!-- Table Cards Container -->
    <div class="absolute inset-0 pointer-events-none">
      {#each tables as table (table.name)}
        {@const pos = positions[table.name] || { x: 60, y: 60 }}
        {@const isExp = Boolean(expandedTables[table.name])}
        {@const isSelected = selectedTable === table.name}
        {@const isMatched = !searchQuery || table.name.toLowerCase().includes(searchQuery.toLowerCase())}
        {@const allCols = table.columns || []}
        {@const visibleCols = isExp ? allCols : allCols.slice(0, 12)}
        {@const hasMore = allCols.length > 12}
        {@const fkCols = new Set((table.foreign_keys || []).map((fk) => fk.column))}

        <div
          class="erd-card absolute rounded-xl shadow-2xl border transition-shadow pointer-events-auto bg-[#0b0f19] select-none {isSelected
            ? 'border-sky-400 ring-2 ring-sky-400/40 shadow-[0_0_25px_rgba(56,189,248,0.3)] z-30'
            : isMatched
            ? 'border-[#334155] hover:border-slate-500 z-10'
            : 'border-slate-800 opacity-25 z-0'}"
          style="left: {pos.x}px; top: {pos.y}px; width: {CARD_WIDTH}px;"
          onclick={() => (selectedTable = isSelected ? null : table.name)}
          role="article"
        >
          <!-- Card Header (Drag Handle) -->
          <div
            onmousedown={(e) => startCardDrag(e, table.name)}
            class="h-[38px] px-3 bg-[#0f172a] rounded-t-xl border-b border-[#1e293b] flex items-center justify-between cursor-grab active:cursor-grabbing group"
          >
            <div class="flex items-center space-x-1.5 truncate">
              <Icon name="table" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
              <span class="font-mono text-xs font-bold text-slate-100 truncate" title={table.name}>
                {table.name}
              </span>
            </div>

            <div class="flex items-center space-x-1 shrink-0 ml-1">
              <button
                onclick={(e) => {
                  e.stopPropagation();
                  appState.openTableDataTab(table.name);
                }}
                class="p-1 hover:bg-slate-700 text-slate-400 hover:text-emerald-300 rounded transition opacity-0 group-hover:opacity-100"
                title="Open Table Data Grid"
              >
                <Icon name="eye" class="w-3 h-3" />
              </button>
              <button
                onclick={(e) => {
                  e.stopPropagation();
                  appState.openTableStructureTab(table.name);
                }}
                class="p-1 hover:bg-slate-700 text-slate-400 hover:text-sky-300 rounded transition opacity-0 group-hover:opacity-100"
                title="View Table Structure"
              >
                <Icon name="columns" class="w-3 h-3" />
              </button>
            </div>
          </div>

          <!-- Columns List -->
          <div class="divide-y divide-[#1e293b]/60 text-xs font-mono">
            {#each visibleCols as col}
              {@const isPK = col.is_primary_key}
              {@const isFK = fkCols.has(col.name)}
              <div
                class="px-2.5 py-1 flex items-center justify-between hover:bg-slate-800/40 text-[11px] {isPK ? 'text-amber-300 font-semibold bg-amber-950/10' : isFK ? 'text-sky-300' : 'text-slate-300'}"
                title="{col.name} ({col.type})"
              >
                <div class="flex items-center space-x-1.5 truncate">
                  {#if isPK}
                    <Icon name="key" class="w-3 h-3 text-amber-400 shrink-0" />
                  {:else if isFK}
                    <Icon name="link" class="w-3 h-3 text-sky-400 shrink-0" />
                  {:else}
                    <span class="w-3 h-3 inline-block shrink-0"></span>
                  {/if}
                  <span class="truncate">{col.name}</span>
                </div>
                <span class="text-[9px] text-slate-500 font-mono ml-1 uppercase shrink-0">
                  {col.type.split('(')[0]}
                </span>
              </div>
            {/each}

            {#if hasMore}
              <button
                onclick={(e) => {
                  e.stopPropagation();
                  expandedTables[table.name] = !expandedTables[table.name];
                }}
                class="w-full text-center py-1 text-[10px] text-sky-400 hover:text-sky-300 hover:bg-slate-800/60 font-mono transition"
              >
                <span class="inline-flex items-center justify-center space-x-1">
                  <Icon name={isExp ? 'chevron-up' : 'chevron-down'} class="w-3 h-3" />
                  <span>{isExp ? 'Collapse' : `+${allCols.length - 12} more columns...`}</span>
                </span>
              </button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Minimap in bottom right -->
  <div class="absolute bottom-4 right-4 z-20 bg-[#0f172a]/85 backdrop-blur-md border border-[#334155] rounded-lg p-1.5 shadow-2xl pointer-events-none hidden sm:block">
    <div class="text-[9px] font-mono font-semibold text-slate-400 mb-1 px-1 flex items-center justify-between">
      <span>MINIMAP</span>
      <span class="text-sky-400">{Math.round(scale * 100)}%</span>
    </div>
    <canvas
      bind:this={minimapCanvasRef}
      width="160"
      height="100"
      class="rounded bg-[#070b14] border border-slate-800"
    ></canvas>
  </div>
</div>
