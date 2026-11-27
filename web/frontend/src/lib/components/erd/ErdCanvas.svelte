<script lang="ts">
  import { copyText } from '../../utils/clipboard';
  import { buildErdSvg, buildLeftJoinSQL, buildMermaidErd } from './erdExport';
  import { api } from '../../api/client';
  import { appState } from '../../stores/appState.svelte';
  import type { TableMetadata } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import { untrack } from 'svelte';
  import ErdContextMenu from './ErdContextMenu.svelte';

  const CARD_WIDTH = 224;
  const HEADER_HEIGHT = 38;
  const ROW_HEIGHT = 24;
  const MAX_COL_PREVIEW = 12;

  let containerRef: HTMLDivElement | null = $state(null);
  let searchEl: HTMLInputElement | null = $state(null);
  let scale = $state(1);
  let panX = $state(50);
  let panY = $state(50);
  let isPanning = $state(false);
  let startPanX = 0;
  let startPanY = 0;
  let panOriginX = 0;
  let panOriginY = 0;
  let panMoved = false;

  // Touch pan & pinch state
  let touchMode: 'none' | 'pan' | 'drag' | 'pinch' = 'none';
  let touchStartDist = 0;
  let touchStartScale = 1;
  let touchMidX = 0;
  let touchMidY = 0;

  let searchQuery = $state('');
  let draggingTable = $state<string | null>(null);
  let dragOffset = { x: 0, y: 0 };

  let selectedTable = $state<string | null>(null);
  let hoveredFK = $state<string | null>(null);
  let contextMenu = $state<{ tableName: string; x: number; y: number } | null>(null);

  let expandedTables = $state<Record<string, boolean>>({});
  let positions = $state<Record<string, { x: number; y: number }>>({});
  let isRefreshing = $state(false);

  let minimapCanvasRef: HTMLCanvasElement | null = $state(null);
  let isDraggingMinimap = false;
  let minimapTransform = { minX: 0, minY: 0, pad: 40, s: 1 };

  let activeCatalog = $derived.by(() => {
    const db = appState.activeTab?.database?.toLowerCase();
    if (db && appState.catalogs[db]) return appState.catalogs[db];
    return appState.catalog;
  });

  let tables = $derived<TableMetadata[]>(activeCatalog?.tables || []);

  // Tables directly connected via FK to the currently selected table
  let connectedTables = $derived.by(() => {
    const set = new Set<string>();
    if (!selectedTable) return set;
    const selLow = selectedTable.toLowerCase();
    for (const src of tables) {
      for (const fk of src.foreign_keys || []) {
        const dstLow = (fk.referenced_table || '').toLowerCase();
        if (src.name.toLowerCase() === selLow && fk.referenced_table) {
          set.add(fk.referenced_table);
        } else if (dstLow === selLow) {
          set.add(src.name);
        }
      }
    }
    return set;
  });

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
    if (tables.length > 0) {
      untrack(() => loadOrAutoLayoutPositions());
    }
  });

  function storageKeyForErd(): string {
    const connId = appState.activeConnection?.id || 'default';
    const db = appState.activeTab?.database || appState.currentDatabase || 'default';
    return `sqlfox_erd_${connId}_${db}`;
  }

  function looksCorruptOrStacked(posMap: Record<string, { x: number; y: number }>): boolean {
    const present = tables.map((t) => posMap[t.name]).filter(Boolean);
    if (present.length < 4) return false;
    const uniqueX = new Set(present.map((p) => Math.round(p!.x / 20)));
    const uniquePairs = new Set(present.map((p) => `${Math.round(p!.x)},${Math.round(p!.y)}`));
    if (uniquePairs.size <= Math.max(1, Math.floor(present.length / 3))) return true;
    // Detect collapsed vertical single-column stack with many tables
    if (uniqueX.size === 1 && present.length >= 6) return true;
    return false;
  }

  function loadOrAutoLayoutPositions() {
    const key = storageKeyForErd();
    const connId = appState.activeConnection?.id || 'default';
    const db = appState.activeTab?.database || appState.currentDatabase || 'default';
    let loaded: Record<string, { x: number; y: number }> | null = null;

    try {
      const saved = localStorage.getItem(key);
      if (saved) {
        const parsed = JSON.parse(saved);
        if (parsed && typeof parsed === 'object') loaded = parsed;
      } else {
        // Migrate legacy sessionStorage keys (`erd-pos-...`) if present
        const legacyKeys = [`erd-pos-${connId}-${db}`, `erd-pos-${db}`];
        for (const lk of legacyKeys) {
          const legacyRaw = sessionStorage.getItem(lk);
          if (legacyRaw) {
            const parsed = JSON.parse(legacyRaw);
            if (parsed && typeof parsed === 'object') {
              loaded = parsed;
              localStorage.setItem(key, JSON.stringify(parsed));
              sessionStorage.removeItem(lk);
              break;
            }
          }
        }
      }
    } catch { /* ignore storage errors */ }

    if (loaded && !looksCorruptOrStacked(loaded)) {
      // Place any newly discovered tables next to existing layout without stacking at 0,0
      const copy: Record<string, { x: number; y: number }> = { ...loaded };
      let maxX = 60;
      for (const p of Object.values(copy)) {
        if (p && typeof p.x === 'number') maxX = Math.max(maxX, p.x + CARD_WIDTH + 60);
      }
      let extraY = 60;
      let added = false;
      for (const t of tables) {
        if (!copy[t.name]) {
          copy[t.name] = { x: maxX, y: extraY };
          extraY += getCardHeight(t) + 40;
          if (extraY > 1400) {
            extraY = 60;
            maxX += CARD_WIDTH + 60;
          }
          added = true;
        }
      }
      positions = copy;
      if (added) savePositions();
      return;
    }

    autoLayout();
  }

  function savePositions() {
    try {
      localStorage.setItem(storageKeyForErd(), JSON.stringify(positions));
    } catch { /* ignore */ }
  }

  export function autoLayout() {
    if (tables.length === 0) return;
    const ROW_GAP = 46;
    const COL_GAP = 76;
    const MAX_COL_HEIGHT = 1350;

    // Topological depth layout based on FK references
    const depth: Record<string, number> = {};
    tables.forEach((t) => (depth[t.name] = 0));

    for (let pass = 0; pass < 4; pass++) {
      tables.forEach((t) => {
        (t.foreign_keys || []).forEach((fk) => {
          const ref = tables.find((rt) => rt.name.toLowerCase() === (fk.referenced_table || '').toLowerCase())?.name || fk.referenced_table;
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
      const tbls = layers[layerIdx] || [];
      let curY = 60;
      tbls.forEach((t) => {
        const cardH = getCardHeight(t);
        if (curY > 60 && curY + cardH > MAX_COL_HEIGHT) {
          // Wrap overflowing layer into the next sub-column so tall schemas stay compact
          curX += CARD_WIDTH + COL_GAP;
          curY = 60;
        }
        newPos[t.name] = { x: curX, y: curY };
        curY += cardH + ROW_GAP;
      });
      curX += CARD_WIDTH + COL_GAP;
    });

    positions = newPos;
    savePositions();
  }

  function getCardHeight(t: TableMetadata): number {
    const isExp = Boolean(expandedTables[t.name]);
    const colCount = Math.min(isExp ? 100 : MAX_COL_PREVIEW, (t.columns || []).length);
    const hasMore = (t.columns || []).length > MAX_COL_PREVIEW;
    return HEADER_HEIGHT + colCount * ROW_HEIGHT + (hasMore ? 26 : 8);
  }

  let lines = $derived.by<FKLine[]>(() => {
    const list: FKLine[] = [];
    const sel = selectedTable;
    const tableByLower = new Map<string, (typeof tables)[number]>();
    for (const t of tables) {
      tableByLower.set(t.name.toLowerCase(), t);
    }

    tables.forEach((src) => {
      (src.foreign_keys || []).forEach((fk) => {
        const sp = positions[src.name];
        const dst = tableByLower.get((fk.referenced_table || '').toLowerCase());
        const dp = dst ? positions[dst.name] : positions[fk.referenced_table];
        if (!sp || !dp) return;

        const fkCol = fk.column || fk.column_name || '';
        const fkKey = `${src.name}.${fkCol}->${fk.referenced_table}.${fk.referenced_column}`;
        const isSelected = Boolean(
          sel && (src.name === sel || (dst?.name || fk.referenced_table) === sel)
        );
        const isHovered = hoveredFK === fkKey;
        const isDimmed = Boolean(sel && !isSelected);

        const srcCols = src.columns || [];
        const srcIdx = srcCols.findIndex((c) => c.name === fkCol);
        const isSrcExp = Boolean(expandedTables[src.name]);
        const srcLimit = isSrcExp ? srcCols.length : MAX_COL_PREVIEW;
        const y1 =
          sp.y +
          HEADER_HEIGHT +
          (srcIdx >= 0 && srcIdx < srcLimit ? srcIdx * ROW_HEIGHT + ROW_HEIGHT / 2 : ROW_HEIGHT / 2);

        const dstCols = dst?.columns || [];
        const dstIdx = dstCols.findIndex((c) => c.name === fk.referenced_column);
        const isDstExp = dst ? Boolean(expandedTables[dst.name]) : false;
        const dstLimit = isDstExp ? dstCols.length : MAX_COL_PREVIEW;
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
          srcCol: fkCol,
          dstTable: fk.referenced_table,
          dstCol: fk.referenced_column,
          path,
          midX: (x1 + x2) / 2,
          midY: (y1 + y2) / 2,
          label: `${fkCol} → ${fk.referenced_column}`,
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
    if (e.button !== 0) return;
    contextMenu = null;
    if ((e.target as HTMLElement).closest('.erd-card, .erd-btn, .erd-minimap, .erd-menu, input, button')) return;
    isPanning = true;
    panOriginX = e.clientX;
    panOriginY = e.clientY;
    panMoved = false;
    startPanX = e.clientX - panX;
    startPanY = e.clientY - panY;
    window.addEventListener('mousemove', onPanMove);
    window.addEventListener('mouseup', onPanUp);
  }

  function onPanMove(e: MouseEvent) {
    if (!isPanning) return;
    if (!panMoved && Math.hypot(e.clientX - panOriginX, e.clientY - panOriginY) > 4) {
      panMoved = true;
    }
    panX = e.clientX - startPanX;
    panY = e.clientY - startPanY;
  }

  function onPanUp() {
    isPanning = false;
    if (!panMoved) {
      selectedTable = null;
    }
    window.removeEventListener('mousemove', onPanMove);
    window.removeEventListener('mouseup', onPanUp);
  }

  function zoomTowardPoint(newScale: number, viewX: number, viewY: number) {
    const clamped = Math.min(2.5, Math.max(0.25, newScale));
    panX = viewX - (viewX - panX) * (clamped / scale);
    panY = viewY - (viewY - panY) * (clamped / scale);
    scale = clamped;
  }

  function handleWheel(e: WheelEvent) {
    e.preventDefault();
    const zoomFactor = e.deltaY < 0 ? 1.08 : 0.92;
    if (containerRef) {
      const rect = containerRef.getBoundingClientRect();
      zoomTowardPoint(scale * zoomFactor, e.clientX - rect.left, e.clientY - rect.top);
    } else {
      scale = Math.min(2.5, Math.max(0.25, scale * zoomFactor));
    }
  }

  function zoomStep(delta: number) {
    const cx = (containerRef?.clientWidth || 800) / 2;
    const cy = (containerRef?.clientHeight || 600) / 2;
    zoomTowardPoint(scale + delta, cx, cy);
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
    const pad = 60;
    const contentW = Math.max(100, maxX - minX + pad * 2);
    const contentH = Math.max(100, maxY - minY + pad * 2);

    const newScale = Math.min(1.2, Math.max(0.25, Math.min(w / contentW, h / contentH)));
    scale = newScale;
    panX = (w - contentW * newScale) / 2 - (minX - pad) * newScale;
    panY = (h - contentH * newScale) / 2 - (minY - pad) * newScale;
  }

  function focusTable(tableName: string) {
    selectedTable = tableName;
    const p = positions[tableName];
    const t = tables.find((tbl) => tbl.name === tableName);
    if (!p || !t || !containerRef) return;
    const w = containerRef.clientWidth || 800;
    const h = containerRef.clientHeight || 600;
    const cx = p.x + CARD_WIDTH / 2;
    const cy = p.y + getCardHeight(t) / 2;
    panX = w / 2 - cx * scale;
    panY = h / 2 - cy * scale;
  }

  // Card dragging (clamped >= 0)
  function startCardDrag(e: MouseEvent, tableName: string) {
    if (e.button !== 0) return;
    e.stopPropagation();
    contextMenu = null;
    draggingTable = tableName;
    const pos = positions[tableName] || { x: 0, y: 0 };
    dragOffset = {
      x: e.clientX / scale - pos.x,
      y: e.clientY / scale - pos.y,
    };
    window.addEventListener('mousemove', onCardDragMove);
    window.addEventListener('mouseup', onCardDragUp);
  }

  let dragRafId: number | null = null;
  let pendingDragCoords: { x: number; y: number } | null = null;

  function onCardDragMove(e: MouseEvent) {
    if (!draggingTable) return;
    pendingDragCoords = {
      x: Math.max(0, Math.round(e.clientX / scale - dragOffset.x)),
      y: Math.max(0, Math.round(e.clientY / scale - dragOffset.y)),
    };
    if (dragRafId !== null) return;
    dragRafId = requestAnimationFrame(() => {
      dragRafId = null;
      if (draggingTable && pendingDragCoords) {
        positions[draggingTable] = pendingDragCoords;
      }
    });
  }

  function onCardDragUp() {
    if (dragRafId !== null) {
      cancelAnimationFrame(dragRafId);
      dragRafId = null;
    }
    if (draggingTable && pendingDragCoords) {
      positions[draggingTable] = pendingDragCoords;
    }
    pendingDragCoords = null;
    draggingTable = null;
    savePositions();
    window.removeEventListener('mousemove', onCardDragMove);
    window.removeEventListener('mouseup', onCardDragUp);
  }

  // Touch gestures (1-finger pan or card drag, 2-finger pinch zoom)
  function handleTouchStart(e: TouchEvent) {
    if ((e.target as HTMLElement).closest('.erd-btn, .erd-minimap, .erd-menu, input, button')) return;
    if (e.touches.length === 2) {
      touchMode = 'pinch';
      const [a, b] = [e.touches[0]!, e.touches[1]!];
      touchStartDist = Math.hypot(b.clientX - a.clientX, b.clientY - a.clientY);
      touchStartScale = scale;
      const rect = containerRef?.getBoundingClientRect();
      touchMidX = (a.clientX + b.clientX) / 2 - (rect?.left || 0);
      touchMidY = (a.clientY + b.clientY) / 2 - (rect?.top || 0);
      return;
    }
    if (e.touches.length === 1) {
      const t = e.touches[0]!;
      const cardEl = (e.target as HTMLElement).closest('[data-erd-card]') as HTMLElement | null;
      if (cardEl?.dataset.erdCard) {
        const tbl = cardEl.dataset.erdCard;
        touchMode = 'drag';
        draggingTable = tbl;
        const pos = positions[tbl] || { x: 0, y: 0 };
        dragOffset = { x: t.clientX / scale - pos.x, y: t.clientY / scale - pos.y };
      } else {
        touchMode = 'pan';
        startPanX = t.clientX - panX;
        startPanY = t.clientY - panY;
      }
    }
  }

  function handleTouchMove(e: TouchEvent) {
    if (touchMode === 'none') return;
    e.preventDefault();
    if (touchMode === 'pinch' && e.touches.length === 2) {
      const [a, b] = [e.touches[0]!, e.touches[1]!];
      const dist = Math.hypot(b.clientX - a.clientX, b.clientY - a.clientY);
      if (touchStartDist > 0) {
        zoomTowardPoint(touchStartScale * (dist / touchStartDist), touchMidX, touchMidY);
      }
    } else if (touchMode === 'pan' && e.touches.length === 1) {
      const t = e.touches[0]!;
      panX = t.clientX - startPanX;
      panY = t.clientY - startPanY;
    } else if (touchMode === 'drag' && draggingTable && e.touches.length === 1) {
      const t = e.touches[0]!;
      positions[draggingTable] = {
        x: Math.max(0, Math.round(t.clientX / scale - dragOffset.x)),
        y: Math.max(0, Math.round(t.clientY / scale - dragOffset.y)),
      };
    }
  }

  function handleTouchEnd() {
    if (touchMode === 'drag' && draggingTable) {
      draggingTable = null;
      savePositions();
    }
    touchMode = 'none';
  }

  // Minimap click / drag navigation
  function panViewportFromMinimap(clientX: number, clientY: number) {
    if (!minimapCanvasRef || !containerRef) return;
    const rect = minimapCanvasRef.getBoundingClientRect();
    const mx = clientX - rect.left;
    const my = clientY - rect.top;
    const { minX, minY, pad, s } = minimapTransform;
    if (s <= 0) return;
    const worldX = mx / s + minX - pad;
    const worldY = my / s + minY - pad;
    const vw = containerRef.clientWidth || 800;
    const vh = containerRef.clientHeight || 600;
    panX = vw / 2 - worldX * scale;
    panY = vh / 2 - worldY * scale;
  }

  let activeMinimapMove: ((ev: MouseEvent) => void) | null = null;
  let activeMinimapUp: (() => void) | null = null;

  function handleMinimapMouseDown(e: MouseEvent) {
    e.stopPropagation();
    isDraggingMinimap = true;
    panViewportFromMinimap(e.clientX, e.clientY);
    if (activeMinimapMove) window.removeEventListener('mousemove', activeMinimapMove);
    if (activeMinimapUp) window.removeEventListener('mouseup', activeMinimapUp);
    activeMinimapMove = (ev: MouseEvent) => {
      if (isDraggingMinimap) panViewportFromMinimap(ev.clientX, ev.clientY);
    };
    activeMinimapUp = () => {
      isDraggingMinimap = false;
      if (activeMinimapMove) window.removeEventListener('mousemove', activeMinimapMove);
      if (activeMinimapUp) window.removeEventListener('mouseup', activeMinimapUp);
      activeMinimapMove = null;
      activeMinimapUp = null;
    };
    window.addEventListener('mousemove', activeMinimapMove);
    window.addEventListener('mouseup', activeMinimapUp);
  }

  // Release window listeners if the component unmounts mid-pan/drag.
  $effect(() => {
    return () => {
      window.removeEventListener('mousemove', onPanMove);
      window.removeEventListener('mouseup', onPanUp);
      window.removeEventListener('mousemove', onCardDragMove);
      window.removeEventListener('mouseup', onCardDragUp);
      if (activeMinimapMove) window.removeEventListener('mousemove', activeMinimapMove);
      if (activeMinimapUp) window.removeEventListener('mouseup', activeMinimapUp);
    };
  });

  // Keyboard navigation shortcuts for ERD
  $effect(() => {
    if (typeof window === 'undefined') return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (appState.isRemoteLocked || appState.hasOpenModal) return;
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      const inInput = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable);
      if (inInput) return;
      if (e.key === 'Escape') {
        if (contextMenu) contextMenu = null;
        else if (selectedTable) selectedTable = null;
        else if (searchQuery) searchQuery = '';
      } else if (e.key === 'f' || e.key === 'F') {
        e.preventDefault();
        fitAll();
      } else if (e.key === '+' || e.key === '=') {
        e.preventDefault();
        zoomStep(0.15);
      } else if (e.key === '-' || e.key === '_') {
        e.preventDefault();
        zoomStep(-0.15);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  });

  function openContextMenu(e: MouseEvent, tableName: string) {
    e.preventDefault();
    e.stopPropagation();
    selectedTable = tableName;
    contextMenu = {
      tableName,
      x: e.clientX,
      y: e.clientY,
    };
  }

  function generateLeftJoinSQL(tableName: string) {
    contextMenu = null;
    const t = tables.find((x) => x.name === tableName);
    if (!t) return;
    const driver = appState.activeConnection?.driver || 'mysql';
    const { sql, joinCount } = buildLeftJoinSQL(t, tables, driver);
    appState.addTab(`join_${t.name}.sql`, sql);
    appState.showToast(
      joinCount > 0 ? `Generated LEFT JOIN query (${joinCount} joins)` : `Generated SELECT template for ${t.name}`,
      'success'
    );
  }

  async function viewTableDdlModal(tableName: string) {
    contextMenu = null;
    if (!appState.activeSessionId) return;
    try {
      const res = await api.getTableDDL(appState.activeSessionId, tableName);
      appState.openDdlViewModal(`TABLE DDL: ${tableName}`, res?.ddl || `-- No DDL returned for ${tableName}`);
    } catch (err: any) {
      appState.showToast(`Failed to load DDL: ${err.message}`, 'error');
    }
  }

  function askAiAboutTable(tableName: string) {
    contextMenu = null;
    const t = tables.find((x) => x.name === tableName);
    if (!t) return;
    const cols = (t.columns || []).map((c) => `- ${c.name} (${c.type}${c.is_primary_key ? ', PK' : ''})`).join('\n');
    const fks = (t.foreign_keys || []).map((fk) => `- ${fk.column || fk.column_name} -> ${fk.referenced_table}.${fk.referenced_column}`).join('\n');
    appState.sendToAi(
      `Analyze table \`${t.name}\` schema, relationships, and suggest useful indexes or query patterns:\n\nColumns:\n${cols}\n${fks ? `\nForeign Keys:\n${fks}` : ''}`,
      `Table ${t.name} sent to AI`
    );
  }

  async function handleRefreshCanvas() {
    isRefreshing = true;
    try {
      await appState.refreshCatalog();
      loadOrAutoLayoutPositions();
      appState.showToast('Schema catalog refreshed', 'info');
    } finally {
      isRefreshing = false;
    }
  }

  function exportSVG() {
    if (tables.length === 0) {
      appState.showToast('No tables to export', 'warning');
      return;
    }
    const fullSvg = buildErdSvg({
      tables,
      positions,
      expandedTables,
      lines: lines || [],
      cardWidth: CARD_WIDTH,
      headerHeight: HEADER_HEIGHT,
      rowHeight: ROW_HEIGHT,
      maxColPreview: MAX_COL_PREVIEW,
      getCardHeight,
    });
    const blob = new Blob([fullSvg], { type: 'image/svg+xml;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${appState.activeTab?.database || appState.currentDatabase || 'database'}_erd.svg`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    appState.showToast('Exported complete ERD diagram as SVG', 'success');
  }

  function exportMermaid() {
    copyText(buildMermaidErd(tables));
    appState.showToast('Copied Mermaid ERD syntax to clipboard!', 'success');
  }

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
    minimapTransform = { minX, minY, pad, s };

    tables.forEach((t) => {
      const p = positions[t.name] || { x: 0, y: 0 };
      const h = getCardHeight(t);
      const rx = (p.x - minX + pad) * s;
      const ry = (p.y - minY + pad) * s;
      const rw = CARD_WIDTH * s;
      const rh = h * s;
      ctx.fillStyle =
        t.name === selectedTable
          ? '#38bdf8'
          : connectedTables.has(t.name)
            ? '#f59e0b'
            : '#334155';
      ctx.fillRect(rx, ry, Math.max(2, rw), Math.max(2, rh));
    });

    if (containerRef) {
      const vw = containerRef.clientWidth / scale;
      const vh = containerRef.clientHeight / scale;
      const vx = (-panX / scale - minX + pad) * s;
      const vy = (-panY / scale - minY + pad) * s;
      ctx.strokeStyle = '#38bdf8';
      ctx.lineWidth = 1.2;
      ctx.strokeRect(vx, vy, vw * s, vh * s);
    }
  }

  $effect(() => {
    const _pos = positions;
    const _s = scale;
    const _px = panX;
    const _py = panY;
    const _sel = selectedTable;
    const _conn = connectedTables;
    if (minimapCanvasRef) {
      renderMinimap();
    }
  });
</script>

<div
  bind:this={containerRef}
  onmousedown={handleMouseDown}
  onwheel={handleWheel}
  ontouchstart={handleTouchStart}
  ontouchmove={handleTouchMove}
  ontouchend={handleTouchEnd}
  role="presentation"
  aria-label="ERD Canvas"
  class="h-full w-full bg-[#070b14] relative overflow-hidden select-none cursor-grab active:cursor-grabbing"
  style="background-image: radial-gradient(#1e293b 1px, transparent 1px); background-size: {Math.max(12, Math.round(24 * scale))}px {Math.max(12, Math.round(24 * scale))}px; background-position: {panX}px {panY}px;"
>
  <!-- Top Floating Toolbar -->
  <div class="absolute top-3 left-3 right-3 z-30 flex items-center justify-between gap-2 flex-wrap pointer-events-none">
    <!-- Left: Title, Stats & Filter -->
    <div class="flex items-center space-x-2 bg-[#0f172a]/90 backdrop-blur-md border border-[#334155] rounded-lg p-1.5 shadow-2xl pointer-events-auto">
      <div class="flex items-center space-x-1.5 px-2 text-xs font-semibold text-slate-200">
        <Icon name="layers" class="w-4 h-4 text-sky-400" />
        <span>ERD Canvas:</span>
        <span class="font-mono text-sky-400">{appState.activeTab?.database || appState.currentDatabase || 'None'}</span>
        <span class="text-slate-500 font-mono text-[11px]">
          ({tables.length} tables · {lines.length} FKs)
        </span>
      </div>

      <div class="h-4 w-px bg-slate-700"></div>

      <!-- Search Filter (Enter jumps to match) -->
      <div class="relative flex items-center">
        <Icon name="search" class="w-3 h-3 text-slate-500 absolute left-2 pointer-events-none" />
        <input
          bind:this={searchEl}
          type="text"
          bind:value={searchQuery}
          onkeydown={(e) => {
            if (e.key === 'Enter' && searchQuery.trim()) {
              const first = tables.find((t) => t.name.toLowerCase().includes(searchQuery.trim().toLowerCase()));
              if (first) focusTable(first.name);
            } else if (e.key === 'Escape') {
              searchQuery = '';
              searchEl?.blur();
            }
          }}
          placeholder="Filter tables (Enter to focus)..."
          class="bg-[#090d16] border border-slate-700 focus:border-sky-500 rounded pl-7 pr-6 py-1 text-xs text-slate-200 font-mono outline-none w-40 sm:w-52"
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

    <!-- Right: Canvas Tools & Export Controls -->
    <div class="flex items-center space-x-1.5 bg-[#0f172a]/90 backdrop-blur-md border border-[#334155] rounded-lg p-1.5 shadow-2xl pointer-events-auto flex-wrap">
      <button
        type="button"
        onclick={() => (appState.isTableCreatorOpen = true)}
        class="px-2.5 py-1 bg-emerald-600/90 hover:bg-emerald-500 text-white font-semibold rounded text-xs flex items-center space-x-1 transition"
        title="Create New Table"
      >
        <Icon name="plus" class="w-3.5 h-3.5" />
        <span>New Table</span>
      </button>

      <button
        type="button"
        onclick={handleRefreshCanvas}
        disabled={isRefreshing}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition"
        title="Refresh Schema Catalog"
      >
        <Icon name="refresh-cw" class="w-3.5 h-3.5 {isRefreshing ? 'animate-spin text-sky-400' : ''}" />
      </button>

      <button
        type="button"
        onclick={autoLayout}
        class="px-2.5 py-1 bg-[#1e293b] hover:bg-slate-700 border border-slate-700 text-slate-200 rounded text-xs flex items-center space-x-1 transition"
        title="Auto-arrange tables by foreign key depth"
      >
        <Icon name="sliders" class="w-3.5 h-3.5 text-sky-400" />
        <span>Auto-Layout</span>
      </button>

      <button
        type="button"
        onclick={fitAll}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition"
        title="Fit All Tables into View (Shortcut: F)"
      >
        <Icon name="maximize" class="w-3.5 h-3.5" />
      </button>

      <button
        type="button"
        onclick={() => zoomStep(0.15)}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition"
        title="Zoom In (+)"
      >
        <Icon name="plus" class="w-3.5 h-3.5" />
      </button>

      <span class="text-[11px] font-mono text-slate-400 px-1 min-w-[42px] text-center">
        {Math.round(scale * 100)}%
      </span>

      <button
        type="button"
        onclick={() => zoomStep(-0.15)}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition"
        title="Zoom Out (-)"
      >
        <Icon name="minus" class="w-3.5 h-3.5" />
      </button>

      <button
        type="button"
        onclick={resetZoom}
        class="p-1.5 hover:bg-slate-700 text-slate-300 hover:text-white rounded transition text-xs"
        title="Reset Zoom to 100%"
      >
        <Icon name="rotate-ccw" class="w-3.5 h-3.5 text-slate-400" />
      </button>

      <div class="h-4 w-px bg-slate-700"></div>

      <!-- Export Buttons -->
      <button
        type="button"
        onclick={exportSVG}
        class="px-2 py-1 bg-sky-950/60 hover:bg-sky-900/60 border border-sky-800 text-sky-300 font-semibold rounded text-xs flex items-center space-x-1 transition"
        title="Download diagram as SVG vector image"
      >
        <Icon name="share-2" class="w-3.5 h-3.5" />
        <span>SVG</span>
      </button>

      <button
        type="button"
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
          <path
            d={line.path}
            stroke="transparent"
            stroke-width="16"
            fill="none"
            role="button"
            tabindex="0"
            aria-label="Highlight relationship {line.label}"
            class="cursor-pointer"
            onmouseenter={() => (hoveredFK = line.key)}
            onmouseleave={() => (hoveredFK = null)}
            onclick={() => (selectedTable = line.srcTable)}
            onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectedTable = line.srcTable; } }}
          />
          <path
            d={line.path}
            stroke={line.isHovered || line.isSelected ? '#38bdf8' : '#f59e0b'}
            stroke-width={line.isHovered || line.isSelected ? 2.5 : 1.5}
            stroke-opacity={line.isDimmed ? 0.15 : line.isHovered || line.isSelected ? 1 : 0.65}
            fill="none"
            marker-end={line.isHovered || line.isSelected ? 'url(#erd-arrow-active)' : 'url(#erd-arrow)'}
          />
          {#if !line.isDimmed}
            <g
              transform="translate({line.midX}, {line.midY})"
              role="button"
              tabindex="0"
              aria-label="Relationship {line.label}"
              class="cursor-pointer select-none"
              onmouseenter={() => (hoveredFK = line.key)}
              onmouseleave={() => (hoveredFK = null)}
              onclick={() => (selectedTable = line.srcTable)}
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectedTable = line.srcTable; } }}
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
        {@const isConnected = !isSelected && connectedTables.has(table.name)}
        {@const isMatched = !searchQuery || table.name.toLowerCase().includes(searchQuery.toLowerCase())}
        {@const isDimmed = (Boolean(selectedTable) && !isSelected && !isConnected) || !isMatched}
        {@const allCols = table.columns || []}
        {@const visibleCols = isExp ? allCols : allCols.slice(0, MAX_COL_PREVIEW)}
        {@const hasMore = allCols.length > MAX_COL_PREVIEW}
        {@const fkCols = new Set((table.foreign_keys || []).map((fk) => fk.column || fk.column_name))}

        <div
          data-erd-card={table.name}
          class="erd-card absolute rounded-xl shadow-2xl border transition-colors pointer-events-auto bg-[#0b0f19] select-none {isSelected
            ? 'border-sky-400 ring-2 ring-sky-400/40 shadow-[0_0_25px_rgba(56,189,248,0.3)] z-30'
            : isConnected
            ? 'erd-connected border-amber-400/90 ring-1 ring-amber-400/35 shadow-[0_0_20px_rgba(245,158,11,0.25)] z-20'
            : isDimmed
            ? 'erd-dimmed border-slate-800 opacity-25 z-0'
            : 'border-[#334155] hover:border-slate-500 z-10'}"
          style="left: {pos.x}px; top: {pos.y}px; width: {CARD_WIDTH}px;"
          role="group"
          aria-label="Table {table.name}"
          onclick={() => (selectedTable = isSelected ? null : table.name)}
          ondblclick={() => appState.openTableDataTab(table.name)}
          oncontextmenu={(e) => openContextMenu(e, table.name)}
        >
          <!-- Card Header (Drag Handle) -->
          <div
            role="presentation"
            onmousedown={(e) => startCardDrag(e, table.name)}
            class="h-[38px] px-2.5 bg-[#0f172a] rounded-t-xl border-b border-[#1e293b] flex items-center justify-between cursor-grab active:cursor-grabbing group gap-1"
            title="Drag to move · Double-click to open data · Right-click for actions"
          >
            <div
              role="button"
              tabindex="0"
              aria-pressed={isSelected}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  selectedTable = isSelected ? null : table.name;
                }
              }}
              class="flex items-center space-x-1.5 min-w-0 flex-1"
            >
              <Icon name="table" class="w-3.5 h-3.5 {isConnected ? 'text-amber-400' : 'text-sky-400'} shrink-0" />
              <span class="font-mono text-xs font-bold text-slate-100 truncate" title={table.name}>
                {table.name}
              </span>
              <span class="px-1 rounded bg-slate-800/90 text-[9px] font-mono text-slate-400 shrink-0">
                {allCols.length}
              </span>
            </div>

            <div class="flex items-center space-x-0.5 shrink-0">
              <button
                type="button"
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
                type="button"
                onclick={(e) => {
                  e.stopPropagation();
                  appState.openTableStructureTab(table.name);
                }}
                class="p-1 hover:bg-slate-700 text-slate-400 hover:text-sky-300 rounded transition opacity-0 group-hover:opacity-100"
                title="View Table Structure"
              >
                <Icon name="columns" class="w-3 h-3" />
              </button>
              <button
                type="button"
                onclick={(e) => openContextMenu(e, table.name)}
                class="p-1 hover:bg-slate-700 text-slate-400 hover:text-white rounded transition"
                title="Card Actions Menu"
              >
                <Icon name="more-vertical" class="w-3 h-3" />
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
                  {(col.type || '').split('(')[0]}
                </span>
              </div>
            {/each}

            {#if hasMore}
              <button
                type="button"
                onclick={(e) => {
                  e.stopPropagation();
                  expandedTables[table.name] = !expandedTables[table.name];
                }}
                class="w-full text-center py-1 text-[10px] text-sky-400 hover:text-sky-300 hover:bg-slate-800/60 font-mono transition"
              >
                <span class="inline-flex items-center justify-center space-x-1">
                  <Icon name={isExp ? 'chevron-up' : 'chevron-down'} class="w-3 h-3" />
                  <span>{isExp ? 'Collapse' : `+${allCols.length - MAX_COL_PREVIEW} more columns...`}</span>
                </span>
              </button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Card Context Menu -->
  <ErdContextMenu
    bind:contextMenu
    onViewDdl={viewTableDdlModal}
    onGenerateLeftJoin={generateLeftJoinSQL}
    onAskAi={askAiAboutTable}
  />

  <!-- Interactive Minimap in bottom right -->
  <div class="erd-minimap absolute bottom-4 right-4 z-20 bg-[#0f172a]/90 backdrop-blur-md border border-[#334155] rounded-lg p-1.5 shadow-2xl hidden sm:block">
    <div class="text-[9px] font-mono font-semibold text-slate-400 mb-1 px-1 flex items-center justify-between">
      <span>MINIMAP (Click/Drag)</span>
      <span class="text-sky-400">{Math.round(scale * 100)}%</span>
    </div>
    <canvas
      bind:this={minimapCanvasRef}
      width="168"
      height="104"
      onmousedown={handleMinimapMouseDown}
      ontouchstart={(e) => {
        e.stopPropagation();
        if (e.touches[0]) panViewportFromMinimap(e.touches[0].clientX, e.touches[0].clientY);
      }}
      ontouchmove={(e) => {
        e.preventDefault();
        e.stopPropagation();
        if (e.touches[0]) panViewportFromMinimap(e.touches[0].clientX, e.touches[0].clientY);
      }}
      class="rounded bg-[#070b14] border border-slate-800 cursor-crosshair block"
    ></canvas>
  </div>
</div>
