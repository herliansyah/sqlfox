// --- Visual ERD Canvas ---
    // ponytail: performance-first ERD. Cards diffed in-place, lines use cached SVG path elements,
    // drag/pan/zoom run on single rAF loop, touch gestures for mobile.
    // Ceiling: canvas-based rendering would be next level for 500+ tables.
    const erdState = {
      tables: [],
      positions: {},
      scale: 1, panX: 0, panY: 0,
      draggingCard: null,
      panning: false, panStart: null,
      selectedTable: null,
      hoveredFK: null,
      filterQuery: '',
      expandedTables: new Set(),
      justDragged: false,
      _cardEls: new Map(),   // tableName → DOM element cache
      _lineEls: new Map(),   // fkKey → {group, path, hitarea, label, rect, text} cache
      _dirty: 0,             // bitmask: 1=transform, 2=cards, 4=lines, 8=states, 16=minimap
      _rafId: null,
      _touchState: null,     // {type:'pan'|'pinch', ...}
    };
    const ERD_CARD_W = 240;
    const ERD_DIRTY_TRANSFORM = 1, ERD_DIRTY_CARDS = 2, ERD_DIRTY_LINES = 4, ERD_DIRTY_STATES = 8, ERD_DIRTY_MINIMAP = 16;
    let selectedERDContextMenuTable = null;

    function erdScheduleRender(flags) {
      erdState._dirty |= flags;
      if (!erdState._rafId) {
        erdState._rafId = requestAnimationFrame(erdFlush);
      }
    }

    function erdFlush() {
      erdState._rafId = null;
      const d = erdState._dirty;
      erdState._dirty = 0;
      if (d & ERD_DIRTY_TRANSFORM) erdApplyTransform();
      if (d & ERD_DIRTY_CARDS) erdRenderCards();
      if (d & ERD_DIRTY_LINES) erdRenderLines();
      if (d & ERD_DIRTY_STATES) erdUpdateVisualStates();
      if (d & ERD_DIRTY_MINIMAP) erdRenderMinimap();
    }

    async function openERDCanvas() {
      if (!activeConnection) { showNativeAlert('Connect to a session first.', 'ERD'); return; }
      const db = (schemaCatalogCache && schemaCatalogCache.database_name) || activeConnection.database_name || '';
      document.getElementById('erd-dialog').showModal();
      document.getElementById('erd-table-count').textContent = 'Loading schema…';
      erdState.selectedTable = null;
      erdState.hoveredFK = null;
      erdState.filterQuery = '';
      erdState._cardEls.clear();
      const cardsContainer = document.getElementById('erd-cards');
      if (cardsContainer) cardsContainer.innerHTML = '';
      closeERDExportMenu();
      closeERDContextMenu();
      const sInput = document.getElementById('erd-search-input');
      if (sInput) sInput.value = '';
      const clearBtn = document.getElementById('erd-search-clear');
      if (clearBtn) clearBtn.classList.add('hidden');
      try {
        const url = `/api/session/${activeConnection.id}/schema${db ? '?db=' + encodeURIComponent(db) : ''}`;
        const res = await apiFetch(url);
        const data = await res.json();
        erdState.tables = data.tables || [];
        erdState.scale = 1; erdState.panX = 0; erdState.panY = 0;
        const storeKey = `erd-pos-${activeConnection.id}-${db}`;
        try { erdState.positions = JSON.parse(sessionStorage.getItem(storeKey) || '{}'); } catch { erdState.positions = {}; }
        const savedPositionsList = Object.values(erdState.positions);
        const hasCorruptVerticalStack = erdState.tables.length > 2 && savedPositionsList.length > 0 && new Set(savedPositionsList.map(p => p.x)).size <= 1;
        erdAutoLayout(hasCorruptVerticalStack);
        erdRender();
        erdFitAll();
      } catch (err) {
        document.getElementById('erd-table-count').textContent = 'Failed to load schema';
        console.error('ERD load error', err);
      }
    }

    async function erdRefreshSchema() {
      if (!activeConnection) return;
      const db = (schemaCatalogCache && schemaCatalogCache.database_name) || activeConnection.database_name || '';
      const icon = document.getElementById('erd-refresh-icon');
      if (icon) icon.classList.add('animate-spin');
      try {
        const url = `/api/session/${activeConnection.id}/schema${db ? '?db=' + encodeURIComponent(db) : ''}`;
        const res = await apiFetch(url);
        const data = await res.json();
        erdState.tables = data.tables || [];
        erdState._cardEls.clear();
        const cardsContainer = document.getElementById('erd-cards');
        if (cardsContainer) cardsContainer.innerHTML = '';
        erdAutoLayout(false);
        erdRender();
        showToast('ERD schema updated', 'success');
      } catch (err) {
        showToast('Failed to refresh ERD schema: ' + err.message, 'error');
      } finally {
        if (icon) icon.classList.remove('animate-spin');
      }
    }

    function erdGetCardHeight(t) {
      const isExp = erdState.expandedTables.has(t.name);
      const colCount = Math.min(isExp ? 100 : 15, (t.columns || []).length);
      return 42 + colCount * 24 + 10;
    }

    function erdAutoLayout(force = true) {
      // ponytail: FK-aware topological depth layout. Ceiling: force-directed for >100 tables.
      const ROW_GAP = 40, COL_GAP = 70;
      if (!erdState.tables || erdState.tables.length === 0) return;

      const N = erdState.tables.length;
      const maxRowsPerCol = N <= 2 ? 1 : Math.max(2, Math.min(4, Math.ceil(Math.sqrt(N * 0.7))));

      const depth = {};
      erdState.tables.forEach(t => depth[t.name] = 0);
      for (let pass = 0; pass < 3; pass++) {
        erdState.tables.forEach(t => {
          (t.foreign_keys || []).forEach(fk => {
            const refName = fk.referenced_table;
            if (depth[refName] !== undefined && depth[t.name] <= depth[refName]) {
              depth[t.name] = depth[refName] + 1;
            }
          });
        });
      }

      const layers = {};
      erdState.tables.forEach(t => {
        const d = depth[t.name] || 0;
        if (!layers[d]) layers[d] = [];
        layers[d].push(t);
      });

      const cardsContainer = document.getElementById('erd-cards');
      if (cardsContainer) cardsContainer.classList.add('erd-animating');

      let curX = 40;
      const layerKeys = Object.keys(layers).map(Number).sort((a, b) => a - b);

      layerKeys.forEach(layerIdx => {
        const tbls = layers[layerIdx];
        let curY = 40;
        let countInCol = 0;

        tbls.forEach(t => {
          if (countInCol >= maxRowsPerCol) {
            curX += ERD_CARD_W + COL_GAP;
            curY = 40;
            countInCol = 0;
          }

          if (force || !erdState.positions[t.name]) {
            erdState.positions[t.name] = { x: curX, y: curY };
          }
          curY += erdGetCardHeight(t) + ROW_GAP;
          countInCol++;
        });

        curX += ERD_CARD_W + COL_GAP;
      });

      erdSavePositions();

      setTimeout(() => {
        if (cardsContainer) cardsContainer.classList.remove('erd-animating');
      }, 400);
    }

    function erdSavePositions() {
      if (!activeConnection) return;
      const db = (schemaCatalogCache && schemaCatalogCache.database_name) || activeConnection.database_name || '';
      sessionStorage.setItem(`erd-pos-${activeConnection.id}-${db}`, JSON.stringify(erdState.positions));
    }

    function erdRender() {
      erdApplyTransform();
      erdRenderCards();
      erdRenderLines();
      erdRenderMinimap();
      const count = erdState.tables.length;
      const fkCount = erdState.tables.reduce((s, t) => s + (t.foreign_keys || []).length, 0);
      const db = (schemaCatalogCache && schemaCatalogCache.database_name) || activeConnection?.database_name || '';
      document.getElementById('erd-table-count').textContent =
        `${db ? db + ' · ' : ''}${count} table${count !== 1 ? 's' : ''} · ${fkCount} FK${fkCount !== 1 ? 's' : ''}`;
      const zoomLbl = document.getElementById('erd-zoom-label');
      if (zoomLbl) zoomLbl.textContent = Math.round(erdState.scale * 100) + '%';
    }

    function erdApplyTransform() {
      const vp = document.getElementById('erd-viewport');
      if (vp) vp.style.transform = `translate(${erdState.panX}px,${erdState.panY}px) scale(${erdState.scale})`;
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (wrapper) {
        const size = Math.max(12, Math.round(24 * erdState.scale));
        wrapper.style.backgroundPosition = `${erdState.panX % size}px ${erdState.panY % size}px`;
        wrapper.style.backgroundSize = `${size}px ${size}px`;
      }
    }

    function erdBuildCardHTML(t) {
      const pos = erdState.positions[t.name] || { x: 40, y: 40 };
      const fkCols = new Set((t.foreign_keys || []).map(fk => fk.column));
      const pkCols = new Set((t.columns || []).filter(c => c.is_primary_key || c.key === 'PRI').map(c => c.name));
      const isExp = erdState.expandedTables.has(t.name);
      const allCols = t.columns || [];
      const visibleCols = isExp ? allCols : allCols.slice(0, 15);
      const hasMore = allCols.length > 15;

      return `<div class="bg-slate-800/90 border-b border-slate-700 px-2.5 py-1.5 flex items-center justify-between cursor-grab handle">
            <span class="flex items-center gap-1.5 min-w-0 truncate font-semibold text-sky-400" title="Double click to open table: ${escapeHtml(t.name)}">
              ${renderIcon('table', 'w-3.5 h-3.5 text-slate-400 shrink-0')}
              <span class="truncate">${escapeHtml(t.name)}</span>
            </span>
            <div class="flex items-center space-x-1 shrink-0 ml-1">
              <span class="text-[10px] text-slate-500 font-mono">${allCols.length}</span>
              <button onclick="erdOpenTableViewer('${escapeHtml(t.name)}', event)" class="p-1 hover:bg-slate-700 text-slate-400 hover:text-white rounded transition" title="Open Table Data">
                ${renderIcon('eye', 'w-3 h-3')}
              </button>
              <button onclick="erdOpenTableStructure('${escapeHtml(t.name)}', event)" class="p-1 hover:bg-slate-700 text-slate-400 hover:text-white rounded transition" title="Edit Table Structure">
                ${renderIcon('columns', 'w-3 h-3')}
              </button>
              <button onclick="erdShowCardContextMenu(event, '${escapeHtml(t.name)}')" class="p-1 hover:bg-slate-700 text-slate-400 hover:text-white rounded transition" title="More Actions">
                <span class="text-xs leading-none">⋯</span>
              </button>
            </div>
          </div>
          <div class="divide-y divide-slate-800/80">
            ${visibleCols.map(c => `
              <div class="erd-col-row px-2.5 py-1 flex items-center justify-between text-[11px] transition-colors" title="${escapeHtml(c.name)}: ${escapeHtml(c.type || '')}">
                <span class="font-mono truncate max-w-[145px] flex items-center" style="color:${pkCols.has(c.name) ? '#34d399' : fkCols.has(c.name) ? '#fbbf24' : '#cbd5e1'};">
                  ${pkCols.has(c.name) ? renderIcon('key', 'w-3 h-3 inline text-emerald-400 mr-1 shrink-0') : fkCols.has(c.name) ? renderIcon('link', 'w-3 h-3 inline text-amber-400 mr-1 shrink-0') : ''}
                  <span class="truncate">${escapeHtml(c.name)}</span>
                </span>
                <span class="text-[10px] text-slate-500 font-mono shrink-0 ml-1 bg-slate-950/60 px-1 py-0.2 rounded border border-slate-800/80">${escapeHtml((c.type || '').split('(')[0].toUpperCase())}</span>
              </div>
            `).join('')}
            ${hasMore ? `
              <button onclick="erdToggleExpandTable('${escapeHtml(t.name)}', event)" class="w-full text-center py-1 bg-slate-950/50 hover:bg-slate-800/60 text-[10px] text-sky-400 hover:text-sky-300 font-mono transition">
                ${isExp ? '▲ Collapse columns' : `▼ +${allCols.length - 15} more columns…`}
              </button>
            ` : ''}
          </div>`;
    }

    function erdRenderCards() {
      // ponytail: diff-based card rendering. Only creates/removes/updates cards that changed.
      // Avoids full innerHTML rebuild which destroys and recreates thousands of DOM nodes.
      const container = document.getElementById('erd-cards');
      if (!container) return;

      const sel = erdState.selectedTable;
      const query = erdState.filterQuery;
      const connectedTables = new Set();
      if (sel) {
        erdState.tables.forEach(t => {
          (t.foreign_keys || []).forEach(fk => {
            if (t.name === sel) connectedTables.add(fk.referenced_table);
            if (fk.referenced_table === sel) connectedTables.add(t.name);
          });
        });
      }

      const currentNames = new Set(erdState.tables.map(t => t.name));

      // Remove cards for tables that no longer exist
      for (const [name, el] of erdState._cardEls) {
        if (!currentNames.has(name)) {
          el.remove();
          erdState._cardEls.delete(name);
        }
      }

      // Create or update cards
      erdState.tables.forEach(t => {
        const pos = erdState.positions[t.name] || { x: 40, y: 40 };
        const isSelected = sel === t.name;
        const isConnected = connectedTables.has(t.name);
        const isMatch = !query || t.name.toLowerCase().includes(query);
        const isDimmed = (sel && !isSelected && !isConnected) || (query && !isMatch);

        let card = erdState._cardEls.get(t.name);
        if (!card) {
          card = document.createElement('div');
          card.dataset.table = t.name;
          card.style.width = ERD_CARD_W + 'px';
          card.style.position = 'absolute';
          card.setAttribute('onclick', `erdSelectTable('${escapeHtml(t.name)}', event)`);
          card.setAttribute('ondblclick', `erdOpenTableViewer('${escapeHtml(t.name)}', event)`);
          card.setAttribute('oncontextmenu', `erdShowCardContextMenu(event, '${escapeHtml(t.name)}')`);
          card.innerHTML = erdBuildCardHTML(t);
          container.appendChild(card);
          erdState._cardEls.set(t.name, card);
        }

        card.className = 'erd-card absolute select-none cursor-grab rounded-lg bg-slate-900 border overflow-hidden shadow-xl text-xs' +
          (isSelected ? ' erd-selected' : isConnected ? ' erd-connected' : isDimmed ? ' erd-dimmed' : ' border-slate-700/80');
        card.style.left = pos.x + 'px';
        card.style.top = pos.y + 'px';
        card.style.zIndex = isSelected ? '30' : isConnected ? '20' : '10';
      });
    }

    function erdRenderLines() {
      const g = document.getElementById('erd-lines');
      if (!g) return;
      const sel = erdState.selectedTable;
      let html = '';

      erdState.tables.forEach(src => {
        (src.foreign_keys || []).forEach(fk => {
          const sp = erdState.positions[src.name];
          const dst = erdState.tables.find(t => (t.name || '').toLowerCase() === (fk.referenced_table || '').toLowerCase());
          const dp = dst ? erdState.positions[dst.name] : erdState.positions[fk.referenced_table];
          if (!sp || !dp) return;

          const fkKey = `${src.name}.${fk.column}->${fk.referenced_table}.${fk.referenced_column}`;
          const isSelected = sel && (src.name === sel || (dst?.name || fk.referenced_table) === sel);
          const isHovered = erdState.hoveredFK === fkKey;
          const isDimmed = sel && !isSelected;

          const srcCols = src.columns || [];
          const srcColIdx = srcCols.findIndex(c => c.name === fk.column);
          const isSrcExp = erdState.expandedTables.has(src.name);
          const srcLimit = isSrcExp ? srcCols.length : 15;
          const srcVisible = srcColIdx >= 0 && srcColIdx < srcLimit;
          const y1 = sp.y + 34 + (srcVisible ? srcColIdx * 24 + 12 : 12);

          const dstCols = dst ? (dst.columns || []) : [];
          const dstColIdx = dstCols.findIndex(c => c.name === fk.referenced_column);
          const isDstExp = dst ? erdState.expandedTables.has(dst.name) : false;
          const dstLimit = isDstExp ? dstCols.length : 15;
          const dstVisible = dstColIdx >= 0 && dstColIdx < dstLimit;
          const y2 = dp.y + 34 + (dstVisible ? dstColIdx * 24 + 12 : 12);

          let x1, x2, cx1, cx2;
          const isLeft = (sp.x + ERD_CARD_W + 30) < dp.x;
          const isRight = (dp.x + ERD_CARD_W + 30) < sp.x;

          if (isLeft) {
            x1 = sp.x + ERD_CARD_W; x2 = dp.x;
            const dx = Math.max(35, (x2 - x1) * 0.45);
            cx1 = x1 + dx; cx2 = x2 - dx;
          } else if (isRight) {
            x1 = sp.x; x2 = dp.x + ERD_CARD_W;
            const dx = Math.max(35, (x1 - x2) * 0.45);
            cx1 = x1 - dx; cx2 = x2 + dx;
          } else {
            if (sp.x >= dp.x) {
              x1 = sp.x + ERD_CARD_W; x2 = dp.x + ERD_CARD_W;
              const dx = 45 + Math.abs(y1 - y2) * 0.12;
              cx1 = Math.max(x1, x2) + dx; cx2 = Math.max(x1, x2) + dx;
            } else {
              x1 = sp.x; x2 = dp.x;
              const dx = 45 + Math.abs(y1 - y2) * 0.12;
              cx1 = Math.min(x1, x2) - dx; cx2 = Math.min(x1, x2) - dx;
            }
          }

          const stroke = isHovered || isSelected ? '#38bdf8' : '#f59e0b';
          const strokeWidth = isHovered || isSelected ? '2.4' : '1.5';
          const opacity = isDimmed ? '0.12' : (isHovered || isSelected ? '1' : '0.65');
          const marker = isHovered || isSelected ? 'url(#erd-arrow-active)' : 'url(#erd-arrow)';
          const midX = (x1 + x2) / 2;
          const midY = (y1 + y2) / 2;
          const labelText = `${fk.column} → ${fk.referenced_column}`;
          const badgeW = Math.max(60, labelText.length * 6.5 + 14);

          html += `
            <g class="erd-fk-group" data-fk="${escapeHtml(fkKey)}" data-src="${escapeHtml(src.name)}" data-dst="${escapeHtml(dst?.name || fk.referenced_table)}">
              <path d="M${x1},${y1} C${cx1},${y1} ${cx2},${y2} ${x2},${y2}" class="erd-fk-hitarea" stroke="transparent" stroke-width="14" fill="none" pointer-events="stroke" style="cursor:pointer;" onmouseenter="erdHighlightFK('${escapeHtml(fkKey)}')" onmouseleave="erdUnhighlightFK()" onclick="erdSelectTable('${escapeHtml(src.name)}', event)"/>
              <path d="M${x1},${y1} C${cx1},${y1} ${cx2},${y2} ${x2},${y2}" class="erd-fk-path ${isSelected ? 'erd-active' : ''}" stroke="${stroke}" stroke-width="${strokeWidth}" stroke-opacity="${opacity}" fill="none" marker-end="${marker}"/>
              <g class="erd-fk-label" pointer-events="auto" style="cursor:pointer;${isDimmed ? 'display:none;' : ''}" onmouseenter="erdHighlightFK('${escapeHtml(fkKey)}')" onmouseleave="erdUnhighlightFK()" onclick="erdSelectTable('${escapeHtml(src.name)}', event)">
                <rect x="${midX - badgeW/2}" y="${midY - 8.5}" width="${badgeW}" height="17" rx="4" fill="#090d16" stroke="${stroke}" stroke-width="${isHovered ? 1.5 : 1}" stroke-opacity="${opacity}"/>
                <text x="${midX}" y="${midY + 3.5}" text-anchor="middle" fill="${isHovered || isSelected ? '#ffffff' : '#94a3b8'}" font-size="9" font-family="'JetBrains Mono',monospace">${escapeHtml(labelText)}</text>
              </g>
            </g>`;
        });
      });
      g.innerHTML = html;
    }

    function erdUpdateVisualStates() {
      // ponytail: in-place DOM class/attribute toggle for 60fps instant table selection and filter feedback.
      const sel = erdState.selectedTable;
      const query = erdState.filterQuery;
      const connectedTables = new Set();
      if (sel) {
        erdState.tables.forEach(t => {
          (t.foreign_keys || []).forEach(fk => {
            if (t.name === sel) connectedTables.add(fk.referenced_table);
            if (fk.referenced_table === sel) connectedTables.add(t.name);
          });
        });
      }

      for (const [name, card] of erdState._cardEls) {
        const isSelected = sel === name;
        const isConnected = connectedTables.has(name);
        const isMatch = !query || name.toLowerCase().includes(query);
        const isDimmed = (sel && !isSelected && !isConnected) || (query && !isMatch);

        card.classList.toggle('erd-selected', isSelected);
        card.classList.toggle('erd-connected', !isSelected && isConnected);
        card.classList.toggle('erd-dimmed', isDimmed);
        card.classList.toggle('border-slate-700/80', !isSelected && !isConnected && !isDimmed);
        card.style.zIndex = isSelected ? '30' : (isConnected ? '20' : '10');
      }

      const groups = document.querySelectorAll('#erd-lines .erd-fk-group');
      for (let i = 0; i < groups.length; i++) {
        const g = groups[i];
        const fkKey = g.dataset.fk;
        const isHovered = erdState.hoveredFK === fkKey;
        const srcName = g.dataset.src;
        const dstName = g.dataset.dst;
        const isSelected = !!sel && (srcName === sel || dstName === sel);
        const isDimmed = !!sel && !isSelected;

        const path = g.querySelector('.erd-fk-path');
        if (path) {
          path.classList.toggle('erd-active', isSelected);
          const stroke = isHovered || isSelected ? '#38bdf8' : '#f59e0b';
          const strokeWidth = isHovered || isSelected ? '2.4' : '1.5';
          const opacity = isDimmed ? '0.12' : (isHovered || isSelected ? '1' : '0.65');
          const marker = isHovered || isSelected ? 'url(#erd-arrow-active)' : 'url(#erd-arrow)';
          path.setAttribute('stroke', stroke);
          path.setAttribute('stroke-width', strokeWidth);
          path.setAttribute('stroke-opacity', opacity);
          path.setAttribute('marker-end', marker);
        }

        const label = g.querySelector('.erd-fk-label');
        if (label) {
          label.style.display = isDimmed ? 'none' : '';
          const rect = label.querySelector('rect');
          const text = label.querySelector('text');
          if (rect) {
            rect.setAttribute('stroke', isHovered || isSelected ? '#38bdf8' : '#f59e0b');
            rect.setAttribute('stroke-width', isHovered ? '1.5' : '1');
            rect.setAttribute('stroke-opacity', isDimmed ? '0.12' : (isHovered || isSelected ? '1' : '0.65'));
          }
          if (text) {
            text.setAttribute('fill', isHovered || isSelected ? '#ffffff' : '#94a3b8');
          }
        }
      }
    }

    function erdRenderMinimap() {
      const canvas = document.getElementById('erd-minimap');
      if (!canvas || !erdState.tables.length) {
        if (canvas) canvas.style.display = 'none';
        return;
      }
      canvas.style.display = '';
      const ctx = canvas.getContext('2d');
      const cw = canvas.width, ch = canvas.height;
      ctx.clearRect(0, 0, cw, ch);

      // Find bounds
      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      erdState.tables.forEach(t => {
        const p = erdState.positions[t.name] || { x: 0, y: 0 };
        const h = erdGetCardHeight(t);
        minX = Math.min(minX, p.x); minY = Math.min(minY, p.y);
        maxX = Math.max(maxX, p.x + ERD_CARD_W); maxY = Math.max(maxY, p.y + h);
      });
      const pad = 20;
      const worldW = maxX - minX + pad * 2, worldH = maxY - minY + pad * 2;
      const s = Math.min(cw / worldW, ch / worldH);

      // Draw table dots
      const sel = erdState.selectedTable;
      erdState.tables.forEach(t => {
        const p = erdState.positions[t.name] || { x: 0, y: 0 };
        const h = erdGetCardHeight(t);
        const rx = (p.x - minX + pad) * s, ry = (p.y - minY + pad) * s;
        const rw = ERD_CARD_W * s, rh = h * s;
        ctx.fillStyle = t.name === sel ? '#38bdf8' : '#334155';
        ctx.fillRect(rx, ry, Math.max(2, rw), Math.max(1, rh));
      });

      // Draw viewport rectangle
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (wrapper) {
        const ww = wrapper.clientWidth, wh = wrapper.clientHeight;
        const vpX = (-erdState.panX / erdState.scale - minX + pad) * s;
        const vpY = (-erdState.panY / erdState.scale - minY + pad) * s;
        const vpW = (ww / erdState.scale) * s;
        const vpH = (wh / erdState.scale) * s;
        ctx.strokeStyle = 'rgba(56, 189, 248, 0.6)';
        ctx.lineWidth = 1.5;
        ctx.strokeRect(vpX, vpY, vpW, vpH);
      }
    }

    function erdMinimapNav(e) {
      e.preventDefault();
      const canvas = document.getElementById('erd-minimap');
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (!canvas || !wrapper || !erdState.tables.length) return;

      // Find world bounds
      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      erdState.tables.forEach(t => {
        const p = erdState.positions[t.name] || { x: 0, y: 0 };
        const h = erdGetCardHeight(t);
        minX = Math.min(minX, p.x); minY = Math.min(minY, p.y);
        maxX = Math.max(maxX, p.x + ERD_CARD_W); maxY = Math.max(maxY, p.y + h);
      });
      const pad = 20;
      const worldW = maxX - minX + pad * 2, worldH = maxY - minY + pad * 2;
      const s = Math.min(canvas.width / worldW, canvas.height / worldH);

      const rect = canvas.getBoundingClientRect();
      const doNav = (clientX, clientY) => {
        const mx = clientX - rect.left, my = clientY - rect.top;
        const worldX = mx / s + minX - pad;
        const worldY = my / s + minY - pad;
        const ww = wrapper.clientWidth, wh = wrapper.clientHeight;
        erdState.panX = ww / 2 - worldX * erdState.scale;
        erdState.panY = wh / 2 - worldY * erdState.scale;
        erdScheduleRender(ERD_DIRTY_TRANSFORM | ERD_DIRTY_MINIMAP);
      };

      // Initial click
      const touch = e.touches ? e.touches[0] : e;
      doNav(touch.clientX, touch.clientY);

      const onMove = (ev) => {
        const t = ev.touches ? ev.touches[0] : ev;
        doNav(t.clientX, t.clientY);
      };
      const onUp = () => {
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup', onUp);
        document.removeEventListener('touchmove', onMove);
        document.removeEventListener('touchend', onUp);
      };
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup', onUp);
      document.addEventListener('touchmove', onMove, { passive: true });
      document.addEventListener('touchend', onUp);
    }

    function erdFitAll() {
      const tables = erdState.tables;
      if (!tables || tables.length === 0) return;
      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      tables.forEach(t => {
        const p = erdState.positions[t.name] || { x: 0, y: 0 };
        const h = erdGetCardHeight(t);
        minX = Math.min(minX, p.x); minY = Math.min(minY, p.y);
        maxX = Math.max(maxX, p.x + ERD_CARD_W); maxY = Math.max(maxY, p.y + h);
      });
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (!wrapper) return;
      const ww = wrapper.clientWidth || 1000, wh = wrapper.clientHeight || 700;
      const cw = maxX - minX + 100, ch = maxY - minY + 100;
      const scale = Math.min(1.2, Math.max(0.18, Math.min(ww / cw, wh / ch)));
      erdState.scale = scale;
      erdState.panX = (ww - (maxX - minX) * scale) / 2 - minX * scale;
      erdState.panY = (wh - (maxY - minY) * scale) / 2 - minY * scale;
      erdApplyTransform();
      erdRenderMinimap();
      const zoomLbl = document.getElementById('erd-zoom-label');
      if (zoomLbl) zoomLbl.textContent = Math.round(scale * 100) + '%';
    }

    function erdZoomStep(delta) {
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (!wrapper) return;
      const cx = wrapper.clientWidth / 2, cy = wrapper.clientHeight / 2;
      const newScale = Math.min(3, Math.max(0.15, erdState.scale * (1 + delta)));
      erdState.panX = cx - (cx - erdState.panX) * (newScale / erdState.scale);
      erdState.panY = cy - (cy - erdState.panY) * (newScale / erdState.scale);
      erdState.scale = newScale;
      erdApplyTransform();
      erdRenderMinimap();
      const zoomLbl = document.getElementById('erd-zoom-label');
      if (zoomLbl) zoomLbl.textContent = Math.round(newScale * 100) + '%';
    }

    function erdResetZoom() {
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (!wrapper) return;
      const cx = wrapper.clientWidth / 2, cy = wrapper.clientHeight / 2;
      const newScale = 1;
      erdState.panX = cx - (cx - erdState.panX) * (newScale / erdState.scale);
      erdState.panY = cy - (cy - erdState.panY) * (newScale / erdState.scale);
      erdState.scale = newScale;
      erdApplyTransform();
      erdRenderMinimap();
      const zoomLbl = document.getElementById('erd-zoom-label');
      if (zoomLbl) zoomLbl.textContent = '100%';
    }

    function erdSelectTable(tableName, e) {
      if (e) e.stopPropagation();
      if (erdState.justDragged) {
        erdState.justDragged = false;
        return;
      }
      closeERDContextMenu();
      closeERDExportMenu();
      if (erdState.selectedTable === tableName) {
        erdState.selectedTable = null;
      } else {
        erdState.selectedTable = tableName;
      }
      erdUpdateVisualStates();
      erdScheduleRender(ERD_DIRTY_MINIMAP);
    }

    function erdHighlightFK(fkKey) {
      erdState.hoveredFK = fkKey;
      erdUpdateVisualStates();
    }

    function erdUnhighlightFK() {
      if (erdState.hoveredFK) {
        erdState.hoveredFK = null;
        erdUpdateVisualStates();
      }
    }

    function erdToggleExpandTable(tableName, e) {
      if (e) e.stopPropagation();
      if (erdState.expandedTables.has(tableName)) {
        erdState.expandedTables.delete(tableName);
      } else {
        erdState.expandedTables.add(tableName);
      }
      // Rebuild this card's inner content
      const card = erdState._cardEls.get(tableName);
      const t = erdState.tables.find(x => x.name === tableName);
      if (card && t) {
        card.innerHTML = erdBuildCardHTML(t);
      }
      erdRenderLines();
      erdScheduleRender(ERD_DIRTY_MINIMAP);
    }

    function erdFilterTables(query) {
      erdState.filterQuery = (query || '').trim().toLowerCase();
      const clearBtn = document.getElementById('erd-search-clear');
      if (clearBtn) clearBtn.classList.toggle('hidden', !erdState.filterQuery);
      erdUpdateVisualStates();
    }

    function erdClearFilter() {
      const sInput = document.getElementById('erd-search-input');
      if (sInput) sInput.value = '';
      erdFilterTables('');
    }

    function erdFocusFirstMatch() {
      if (!erdState.filterQuery) return;
      const match = erdState.tables.find(t => t.name.toLowerCase().includes(erdState.filterQuery));
      if (match) erdFocusTable(match.name);
    }

    function erdFocusTable(tableName) {
      const p = erdState.positions[tableName];
      if (!p) return;
      erdState.selectedTable = tableName;
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (!wrapper) return;
      const ww = wrapper.clientWidth, wh = wrapper.clientHeight;
      erdState.panX = ww / 2 - (p.x + ERD_CARD_W / 2) * erdState.scale;
      erdState.panY = wh / 2 - (p.y + 70) * erdState.scale;
      erdApplyTransform();
      erdUpdateVisualStates();
      erdScheduleRender(ERD_DIRTY_MINIMAP);
    }

    function erdOpenTableViewer(tableName, e) {
      if (e) e.stopPropagation();
      closeERDContextMenu();
      document.getElementById('erd-dialog').close();
      openTableViewer(tableName);
    }

    function erdOpenTableStructure(tableName, e) {
      if (e) e.stopPropagation();
      closeERDContextMenu();
      document.getElementById('erd-dialog').close();
      openTableViewer(tableName);
      setTableMode('structure');
    }

    function erdOpenCreateTable() {
      openCreateTableModal();
    }

    function erdShowCardContextMenu(e, tableName) {
      if (e) { e.preventDefault(); e.stopPropagation(); }
      selectedERDContextMenuTable = tableName;
      const menu = document.getElementById('erd-context-menu');
      if (!menu) return;
      menu.classList.remove('hidden');
      const x = Math.min(window.innerWidth - 185, e.clientX);
      const y = Math.min(window.innerHeight - 220, e.clientY);
      menu.style.left = `${x}px`;
      menu.style.top = `${y}px`;
    }

    function closeERDContextMenu() {
      const menu = document.getElementById('erd-context-menu');
      if (menu) menu.classList.add('hidden');
      selectedERDContextMenuTable = null;
    }

    function erdContextMenuAction(action) {
      const tbl = selectedERDContextMenuTable;
      closeERDContextMenu();
      if (!tbl) return;
      if (action === 'data') {
        erdOpenTableViewer(tbl);
      } else if (action === 'structure') {
        erdOpenTableStructure(tbl);
      } else if (action === 'ddl') {
        viewTableDDL(tbl);
      } else if (action === 'copy') {
        copyTextToClipboard(tbl).then(ok => {
          if (ok) showToast(`Copied "${tbl}" to clipboard`, 'success');
        });
      } else if (action === 'query') {
        erdGenerateSelectSQL(tbl);
      } else if (action === 'center') {
        erdFocusTable(tbl);
      }
    }

    function erdGenerateSelectSQL(tableName) {
      const t = erdState.tables.find(x => x.name === tableName);
      if (!t) return;
      let sql = `SELECT * FROM \`${tableName}\``;
      if (t.foreign_keys && t.foreign_keys.length > 0) {
        t.foreign_keys.forEach((fk, i) => {
          const alias = `t${i + 1}`;
          sql += `\nLEFT JOIN \`${fk.referenced_table}\` AS ${alias} ON \`${tableName}\`.\`${fk.column}\` = ${alias}.\`${fk.referenced_column}\``;
        });
      }
      sql += `\nLIMIT 50;`;

      document.getElementById('erd-dialog').close();
      const newId = 'tab-' + Date.now();
      const targetDb = (schemaCatalogCache && schemaCatalogCache.database_name) || activeConnection?.database_name || '';
      tabs.push({
        id: newId,
        title: `${tableName}-query.sql`,
        sql: sql,
        database: targetDb,
        isTableViewer: false
      });
      switchTab(newId);
      showToast(`Generated query for ${tableName}`, 'success');
    }

    function erdToggleExportMenu(e) {
      if (e && typeof e.stopPropagation === 'function') e.stopPropagation();
      const menu = document.getElementById('erd-export-menu');
      if (!menu) return;
      if (typeof e === 'boolean') {
        menu.classList.toggle('hidden', !e);
      } else {
        menu.classList.toggle('hidden');
      }
    }

    function closeERDExportMenu() {
      const menu = document.getElementById('erd-export-menu');
      if (menu) menu.classList.add('hidden');
    }

    function erdExportMermaid() {
      if (!erdState.tables || erdState.tables.length === 0) {
        showToast('No tables to export', 'warning');
        return;
      }
      let out = 'erDiagram\n';
      erdState.tables.forEach(t => {
        const safeName = t.name.replace(/[^a-zA-Z0-9_]/g, '_');
        out += `    ${safeName} {\n`;
        (t.columns || []).slice(0, 30).forEach(c => {
          const type = (c.type || 'text').split('(')[0].replace(/[^a-zA-Z0-9_]/g, '');
          const safeCol = c.name.replace(/[^a-zA-Z0-9_]/g, '_');
          const isPK = c.is_primary_key || c.key === 'PRI';
          const isFK = (t.foreign_keys || []).some(fk => fk.column === c.name);
          const tag = isPK ? ' PK' : (isFK ? ' FK' : '');
          out += `        ${type} ${safeCol}${tag}\n`;
        });
        out += `    }\n`;
      });
      erdState.tables.forEach(src => {
        const srcSafe = src.name.replace(/[^a-zA-Z0-9_]/g, '_');
        (src.foreign_keys || []).forEach(fk => {
          const dstSafe = (fk.referenced_table || '').replace(/[^a-zA-Z0-9_]/g, '_');
          if (dstSafe) {
            out += `    ${srcSafe} }o--|| ${dstSafe} : "${fk.column}"\n`;
          }
        });
      });
      copyTextToClipboard(out).then(ok => {
        if (ok) showToast('Mermaid ERD syntax copied to clipboard!', 'success');
        else showToast('Failed to copy Mermaid syntax', 'error');
      });
      closeERDExportMenu();
    }

    function erdExportSVG() {
      if (!erdState.tables || erdState.tables.length === 0) {
        showToast('No tables to export', 'warning');
        return;
      }
      const CARD_W = 240;
      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      erdState.tables.forEach(t => {
        const p = erdState.positions[t.name] || { x: 0, y: 0 };
        const h = 50 + Math.min(15, (t.columns || []).length) * 24;
        minX = Math.min(minX, p.x); minY = Math.min(minY, p.y);
        maxX = Math.max(maxX, p.x + CARD_W); maxY = Math.max(maxY, p.y + h);
      });
      const pad = 40;
      const w = maxX - minX + pad * 2;
      const h = maxY - minY + pad * 2;

      let cardsSvg = '';
      erdState.tables.forEach(t => {
        const p = erdState.positions[t.name] || { x: 0, y: 0 };
        const x = p.x - minX + pad;
        const y = p.y - minY + pad;
        const cols = (t.columns || []).slice(0, 15);
        const cardH = 34 + cols.length * 24;
        cardsSvg += `<g transform="translate(${x},${y})">
          <rect width="${CARD_W}" height="${cardH}" rx="8" fill="#0f172a" stroke="#334155" stroke-width="1"/>
          <path d="M0,8 a8,8 0 0 1 8,-8 h${CARD_W-16} a8,8 0 0 1 8,8 v26 h-${CARD_W} z" fill="#1e293b"/>
          <text x="12" y="22" fill="#38bdf8" font-family="'JetBrains Mono',monospace" font-size="12" font-weight="bold">${escapeHtml(t.name)}</text>
          <text x="${CARD_W-12}" y="22" text-anchor="end" fill="#64748b" font-family="'JetBrains Mono',monospace" font-size="10">${cols.length}</text>`;
        cols.forEach((c, idx) => {
          const cy = 34 + idx * 24;
          const isPK = c.is_primary_key || c.key === 'PRI';
          const isFK = (t.foreign_keys || []).some(fk => fk.column === c.name);
          const colColor = isPK ? '#34d399' : (isFK ? '#fbbf24' : '#cbd5e1');
          const prefix = isPK ? '🔑 ' : (isFK ? '🔗 ' : '');
          cardsSvg += `
            <line x1="0" y1="${cy}" x2="${CARD_W}" y2="${cy}" stroke="#1e293b" stroke-width="1"/>
            <text x="12" y="${cy + 16}" fill="${colColor}" font-family="'JetBrains Mono',monospace" font-size="10">${prefix}${escapeHtml(c.name)}</text>
            <text x="${CARD_W - 12}" y="${cy + 16}" text-anchor="end" fill="#64748b" font-family="'JetBrains Mono',monospace" font-size="9">${escapeHtml((c.type || '').split('(')[0].toUpperCase())}</text>`;
        });
        cardsSvg += `</g>`;
      });

      const linesSvg = document.getElementById('erd-lines')?.innerHTML || '';
      const fullSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
        <rect width="${w}" height="${h}" fill="#020617"/>
        <defs>
          <marker id="erd-arrow-export" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto"><path d="M0,0 L0,6 L8,3 z" fill="#f59e0b" opacity="0.85"/></marker>
        </defs>
        <g transform="translate(${-minX + pad},${-minY + pad})">
          ${linesSvg.replace(/url\(#erd-arrow[^)]*\)/g, 'url(#erd-arrow-export)')}
        </g>
        ${cardsSvg}
      </svg>`;

      const blob = new Blob([fullSvg], { type: 'image/svg+xml' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `erd-${activeConnection?.name || 'database'}-${Date.now()}.svg`;
      a.click();
      URL.revokeObjectURL(url);
      showToast('ERD diagram downloaded as SVG!', 'success');
      closeERDExportMenu();
    }

    function erdOnWrapperMouseDown(e) {
      if (e.button !== 0 && e.button !== 1) return;
      closeERDContextMenu();
      closeERDExportMenu();

      const card = e.target.closest('.erd-card');
      if (card && e.button === 0) {
        if (e.target.closest('button')) return;
        const name = card.dataset.table;
        const pos = erdState.positions[name] || { x: 0, y: 0 };
        erdState.draggingCard = { name, startMouseX: e.clientX, startMouseY: e.clientY, startCardX: pos.x, startCardY: pos.y, moved: false };
        card.style.cursor = 'grabbing';
        card.style.zIndex = '50';
        card.classList.add('erd-dragging');
        e.preventDefault();
        return;
      }

      if (!e.target.closest('.erd-fk-label') && !e.target.closest('.erd-fk-group') && erdState.selectedTable) {
        erdState.selectedTable = null;
        erdUpdateVisualStates();
        erdScheduleRender(ERD_DIRTY_MINIMAP);
      }

      erdState.panning = true;
      erdState.panStart = { mouseX: e.clientX, mouseY: e.clientY, panX: erdState.panX, panY: erdState.panY };
      e.currentTarget.style.cursor = 'grabbing';
      e.preventDefault();
    }

    // ponytail: Touch events — pan with 1 finger, pinch-zoom with 2 fingers.
    function erdOnTouchStart(e) {
      closeERDContextMenu();
      closeERDExportMenu();

      if (e.touches.length === 1) {
        const t = e.touches[0];
        const card = t.target.closest?.('.erd-card');
        if (card && !t.target.closest?.('button')) {
          const name = card.dataset.table;
          const pos = erdState.positions[name] || { x: 0, y: 0 };
          erdState.draggingCard = { name, startMouseX: t.clientX, startMouseY: t.clientY, startCardX: pos.x, startCardY: pos.y, moved: false };
          card.classList.add('erd-dragging');
          e.preventDefault();
          return;
        }
        erdState._touchState = { type: 'pan', startX: t.clientX, startY: t.clientY, panX: erdState.panX, panY: erdState.panY };
      } else if (e.touches.length === 2) {
        erdState.draggingCard = null;
        const t0 = e.touches[0], t1 = e.touches[1];
        const dist = Math.hypot(t1.clientX - t0.clientX, t1.clientY - t0.clientY);
        const mx = (t0.clientX + t1.clientX) / 2, my = (t0.clientY + t1.clientY) / 2;
        erdState._touchState = { type: 'pinch', startDist: dist, startScale: erdState.scale, mx, my, panX: erdState.panX, panY: erdState.panY };
        e.preventDefault();
      }
    }

    function erdOnTouchMove(e) {
      if (erdState.draggingCard && e.touches.length === 1) {
        const t = e.touches[0];
        const dx = (t.clientX - erdState.draggingCard.startMouseX) / erdState.scale;
        const dy = (t.clientY - erdState.draggingCard.startMouseY) / erdState.scale;
        if (!erdState.draggingCard.moved && (Math.abs(dx) > 5 || Math.abs(dy) > 5)) {
          erdState.draggingCard.moved = true;
        }
        if (erdState.draggingCard.moved) {
          const nx = Math.max(0, erdState.draggingCard.startCardX + dx);
          const ny = Math.max(0, erdState.draggingCard.startCardY + dy);
          erdState.positions[erdState.draggingCard.name] = { x: nx, y: ny };
          const card = erdState._cardEls.get(erdState.draggingCard.name);
          if (card) { card.style.left = nx + 'px'; card.style.top = ny + 'px'; }
          erdScheduleRender(ERD_DIRTY_LINES | ERD_DIRTY_MINIMAP);
          e.preventDefault();
        }
        return;
      }

      const ts = erdState._touchState;
      if (!ts) return;

      if (ts.type === 'pan' && e.touches.length === 1) {
        const t = e.touches[0];
        erdState.panX = ts.panX + (t.clientX - ts.startX);
        erdState.panY = ts.panY + (t.clientY - ts.startY);
        erdScheduleRender(ERD_DIRTY_TRANSFORM | ERD_DIRTY_MINIMAP);
      } else if (ts.type === 'pinch' && e.touches.length === 2) {
        const t0 = e.touches[0], t1 = e.touches[1];
        const dist = Math.hypot(t1.clientX - t0.clientX, t1.clientY - t0.clientY);
        const newScale = Math.min(3, Math.max(0.15, ts.startScale * (dist / ts.startDist)));
        const wrapper = document.getElementById('erd-canvas-wrapper');
        if (wrapper) {
          const rect = wrapper.getBoundingClientRect();
          const mx = ts.mx - rect.left, my = ts.my - rect.top;
          erdState.panX = mx - (mx - ts.panX) * (newScale / ts.startScale);
          erdState.panY = my - (my - ts.panY) * (newScale / ts.startScale);
        }
        erdState.scale = newScale;
        const zoomLbl = document.getElementById('erd-zoom-label');
        if (zoomLbl) zoomLbl.textContent = Math.round(newScale * 100) + '%';
        erdScheduleRender(ERD_DIRTY_TRANSFORM | ERD_DIRTY_MINIMAP);
        e.preventDefault();
      }
    }

    function erdOnTouchEnd(e) {
      if (erdState.draggingCard && e.touches.length === 0) {
        const card = erdState._cardEls.get(erdState.draggingCard.name);
        if (card) {
          card.classList.remove('erd-dragging');
          card.style.cursor = 'grab';
          card.style.zIndex = erdState.selectedTable === erdState.draggingCard.name ? '30' : '10';
        }
        const didMove = !!erdState.draggingCard.moved;
        erdState.draggingCard = null;
        if (didMove) {
          erdSavePositions();
          erdState.justDragged = true;
          setTimeout(() => { erdState.justDragged = false; }, 80);
        }
      }
      if (e.touches.length === 0) {
        erdState._touchState = null;
      }
    }

    function erdOnMouseMove(e) {
      if (erdState.draggingCard) {
        const dx = (e.clientX - erdState.draggingCard.startMouseX) / erdState.scale;
        const dy = (e.clientY - erdState.draggingCard.startMouseY) / erdState.scale;
        if (!erdState.draggingCard.moved && (Math.abs(dx) > 3 || Math.abs(dy) > 3)) {
          erdState.draggingCard.moved = true;
        }
        const nx = Math.max(0, erdState.draggingCard.startCardX + dx);
        const ny = Math.max(0, erdState.draggingCard.startCardY + dy);
        erdState.positions[erdState.draggingCard.name] = { x: nx, y: ny };

        // ponytail: direct DOM update for dragged card + batched rAF for lines only.
        const card = erdState._cardEls.get(erdState.draggingCard.name);
        if (card) { card.style.left = nx + 'px'; card.style.top = ny + 'px'; }
        erdScheduleRender(ERD_DIRTY_LINES | ERD_DIRTY_MINIMAP);
      } else if (erdState.panning && erdState.panStart) {
        erdState.panX = erdState.panStart.panX + (e.clientX - erdState.panStart.mouseX);
        erdState.panY = erdState.panStart.panY + (e.clientY - erdState.panStart.mouseY);
        erdScheduleRender(ERD_DIRTY_TRANSFORM | ERD_DIRTY_MINIMAP);
      }
    }

    function erdOnMouseUp() {
      if (erdState.draggingCard) {
        const card = erdState._cardEls.get(erdState.draggingCard.name);
        if (card) {
          card.classList.remove('erd-dragging');
          card.style.cursor = 'grab';
          card.style.zIndex = erdState.selectedTable === erdState.draggingCard.name ? '30' : '10';
        }
        const didMove = !!erdState.draggingCard.moved;
        erdState.draggingCard = null;
        if (didMove) {
          erdSavePositions();
          erdState.justDragged = true;
          setTimeout(() => { erdState.justDragged = false; }, 50);
        }
      }
      if (erdState.panning) {
        erdState.panning = false;
        erdState.panStart = null;
        const w = document.getElementById('erd-canvas-wrapper');
        if (w) w.style.cursor = 'grab';
      }
    }

    function erdOnWheel(e) {
      e.preventDefault();
      const wrapper = document.getElementById('erd-canvas-wrapper');
      if (!wrapper) return;
      const rect = wrapper.getBoundingClientRect();
      const mx = e.clientX - rect.left, my = e.clientY - rect.top;
      // ponytail: responsive zoom factor — 3x faster than original Math.pow(0.9985, deltaY).
      const zoomFactor = e.deltaY > 0 ? 0.92 : 1.087;
      const newScale = Math.min(3, Math.max(0.15, erdState.scale * zoomFactor));
      if (Math.abs(newScale - erdState.scale) < 0.0001) return;

      erdState.panX = mx - (mx - erdState.panX) * (newScale / erdState.scale);
      erdState.panY = my - (my - erdState.panY) * (newScale / erdState.scale);
      erdState.scale = newScale;
      erdApplyTransform();
      erdRenderMinimap();
      const zoomLbl = document.getElementById('erd-zoom-label');
      if (zoomLbl) zoomLbl.textContent = Math.round(newScale * 100) + '%';
    }

    // Keyboard shortcuts inside ERD dialog
    function erdOnKeyDown(e) {
      const dialog = document.getElementById('erd-dialog');
      if (!dialog || !dialog.open) return;

      if (e.key === 'Escape') {
        if (erdState.selectedTable) {
          e.preventDefault(); e.stopPropagation();
          erdState.selectedTable = null;
          erdUpdateVisualStates();
          erdScheduleRender(ERD_DIRTY_MINIMAP);
          return;
        }
        closeERDContextMenu();
        closeERDExportMenu();
        // Let default Escape close dialog
      } else if (e.key === 'f' && !e.ctrlKey && !e.metaKey && !e.altKey && e.target.tagName !== 'INPUT') {
        e.preventDefault();
        erdFitAll();
      } else if ((e.key === '+' || e.key === '=') && !e.target.closest('input')) {
        e.preventDefault();
        erdZoomStep(0.2);
      } else if (e.key === '-' && !e.target.closest('input')) {
        e.preventDefault();
        erdZoomStep(-0.2);
      }
    }

    document.addEventListener('mousemove', erdOnMouseMove);
    document.addEventListener('mouseup', erdOnMouseUp);
    document.addEventListener('keydown', erdOnKeyDown);

    // Attach touch events to wrapper when dialog opens
    {
      const erdWrapper = document.getElementById('erd-canvas-wrapper');
      if (erdWrapper && typeof erdWrapper.addEventListener === 'function') {
        erdWrapper.addEventListener('touchstart', erdOnTouchStart, { passive: false });
        erdWrapper.addEventListener('touchmove', erdOnTouchMove, { passive: false });
        erdWrapper.addEventListener('touchend', erdOnTouchEnd, { passive: true });
        // Prevent default on wheel to avoid passive violation
        erdWrapper.addEventListener('wheel', erdOnWheel, { passive: false });
      }
    }

    document.addEventListener('click', (e) => {
      if (!e.target.closest('#erd-context-menu')) closeERDContextMenu();
      if (!e.target.closest('#erd-export-menu') && !e.target.closest('#erd-export-btn')) closeERDExportMenu();
    });
    // --- End Visual ERD Canvas ---

    function formatServerEngine(driverName, rawVersion) {
      if (!rawVersion) {
        if (driverName === "mysql") return { label: "MySQL", icon: renderIcon("database", "w-3.5 h-3.5 text-sky-400") };
        if (driverName === "postgres") return { label: "PostgreSQL", icon: renderIcon("database", "w-3.5 h-3.5 text-indigo-400") };
        if (driverName === "sqlite") return { label: "SQLite", icon: renderIcon("database", "w-3.5 h-3.5 text-amber-400") };
        return { label: (driverName || "DB").toUpperCase(), icon: renderIcon("database", "w-3.5 h-3.5 text-slate-400") };
      }

      const lower = rawVersion.toLowerCase();
      if (lower.includes("mariadb")) {
        const match = rawVersion.match(/(\d+\.\d+(\.\d+)?)/);
        const ver = match ? match[1] : "";
        return { label: ver ? `MariaDB ${ver}` : "MariaDB", icon: renderIcon("database", "w-3.5 h-3.5 text-sky-400"), raw: rawVersion };
      }
      if (driverName === "postgres" || lower.includes("postgres")) {
        const match = rawVersion.match(/(\d+(\.\d+)?)/);
        const ver = match ? match[1] : "";
        return { label: ver ? `Postgres ${ver}` : "Postgres", icon: renderIcon("database", "w-3.5 h-3.5 text-indigo-400"), raw: rawVersion };
      }
      if (driverName === "sqlite" || lower.includes("sqlite")) {
        const match = rawVersion.match(/(\d+\.\d+(\.\d+)?)/);
        const ver = match ? match[1] : "";
        return { label: ver ? `SQLite ${ver}` : "SQLite", icon: renderIcon("database", "w-3.5 h-3.5 text-amber-400"), raw: rawVersion };
      }

      const match = rawVersion.match(/(\d+\.\d+(\.\d+)?)/);
      const ver = match ? match[1] : "";
      return { label: ver ? `MySQL ${ver}` : "MySQL", icon: renderIcon("database", "w-3.5 h-3.5 text-sky-400"), raw: rawVersion };
    }

    function formatServerUptime(seconds) {
      if (!seconds || seconds <= 0) return '';
      const d = Math.floor(seconds / 86400);
      const h = Math.floor((seconds % 86400) / 3600);
      const m = Math.floor((seconds % 3600) / 60);

      if (d > 0) return `Up ${d}d ${h}h`;
      if (h > 0) return `Up ${h}h ${m}m`;
      return `Up ${m}m`;
    }

    async function pingActiveSession() {
      if (!activeConnection) return;
      const latencyEl = document.getElementById('footer-latency');
      if (latencyEl) {
        latencyEl.textContent = 'Latency: ⏳';
        latencyEl.className = 'text-sky-400 font-mono transition';
      }
      try {
        const res = await apiFetch(`/api/session/${activeConnection.id}/ping`, { method: 'POST' });
        if (res.ok) {
          const data = await res.json();
          if (activeSession) {
            activeSession.latency_ms = data.latency_ms;
            activeSession.memory_mb = data.memory_mb;
            if (data.server_info) {
              activeSession.server_info = data.server_info;
            }
            if (typeof data.read_only === 'boolean' && activeSession.read_only !== data.read_only) {
              activeSession.read_only = data.read_only;
              if (typeof updateReadOnlyUI === 'function') updateReadOnlyUI(activeSession.read_only);
            }
          }
          updateFooterStats(data.latency_ms, data.memory_mb);
        }
      } catch (err) {
        if (latencyEl) {
          latencyEl.textContent = 'Latency: err';
          latencyEl.className = 'text-rose-400 font-mono';
        }
      }
    }

    function updateFooterStats(latencyMs, memoryMb) {
      const footerEnv = document.getElementById('footer-env-badge');
      if (footerEnv) footerEnv.classList.add('hidden');

      const engineEl = document.getElementById('footer-server-engine');
      const engineIconEl = document.getElementById('footer-engine-icon');
      const engineLabelEl = document.getElementById('footer-engine-label');
      const dot1El = document.getElementById('footer-server-dot-1');
      const dbInfoEl = document.getElementById('footer-db-info');
      const dot2El = document.getElementById('footer-server-dot-2');
      const uptimeEl = document.getElementById('footer-server-uptime');
      const dot3El = document.getElementById('footer-server-dot-3');
      const latencyEl = document.getElementById('footer-latency');
      const memEl = document.getElementById('footer-memory');

      if (activeConnection) {
        const dbTarget = activeConnection.host ? `${activeConnection.host}:${activeConnection.port || 3306}` : (activeConnection.database_name || 'local');
        const dbName = activeConnection.database_name ? ` / ${activeConnection.database_name}` : '';
        if (dbInfoEl) {
          dbInfoEl.textContent = `${dbTarget}${dbName}`;
          dbInfoEl.title = `Host: ${dbTarget} | Database: ${activeConnection.database_name || 'default'}`;
        }

        const serverInfo = (activeSession && activeSession.server_info) ? activeSession.server_info : null;
        const parsed = formatServerEngine(activeConnection.driver, serverInfo ? serverInfo.version : null);

        if (engineEl) {
          if (engineIconEl && engineLabelEl) {
            engineIconEl.innerHTML = parsed.icon;
            engineLabelEl.textContent = parsed.label;
          } else {
            engineEl.innerHTML = `${parsed.icon} <span>${parsed.label}</span>`;
          }
          engineEl.className = 'inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-800/70 text-sky-300 border border-slate-700/60 shadow-sm shrink-0';
          engineEl.title = serverInfo ? `Server Version: ${serverInfo.version || parsed.label}${serverInfo.active_conns ? ` | Active Conns: ${serverInfo.active_conns}` : ''}` : parsed.label;
          engineEl.classList.remove('hidden');
        }
        if (dot1El) dot1El.classList.remove('hidden');

        if (serverInfo && serverInfo.uptime_sec > 0) {
          const uptimeStr = formatServerUptime(serverInfo.uptime_sec);
          if (uptimeStr && uptimeEl) {
            uptimeEl.textContent = uptimeStr;
            uptimeEl.title = `Server Uptime: ${uptimeStr} (${serverInfo.uptime_sec.toLocaleString()} seconds)`;
            uptimeEl.classList.remove('hidden');
            if (dot2El) dot2El.classList.remove('hidden');
          } else {
            if (uptimeEl) uptimeEl.classList.add('hidden');
            if (dot2El) dot2El.classList.add('hidden');
          }
        } else {
          if (uptimeEl) uptimeEl.classList.add('hidden');
          if (dot2El) dot2El.classList.add('hidden');
        }

        if (dot3El) dot3El.classList.remove('hidden');

        const lat = latencyMs !== undefined ? latencyMs : (activeSession ? activeSession.latency_ms : null);
        const mem = memoryMb !== undefined ? memoryMb : (activeSession ? activeSession.memory_mb : null);

        if (latencyEl) {
          if (lat !== null && lat !== undefined) {
            const latStr = lat === 0 ? '<1ms' : `${lat}ms`;
            latencyEl.textContent = `Latency: ${latStr}`;
            latencyEl.className = lat < 50 ? 'text-emerald-400 font-mono cursor-pointer hover:underline transition' :
                                  (lat < 150 ? 'text-amber-400 font-mono cursor-pointer hover:underline transition' :
                                               'text-rose-400 font-mono cursor-pointer hover:underline transition');
            latencyEl.title = 'Click to re-ping database';
          } else {
            latencyEl.textContent = 'Latency: --';
            latencyEl.className = 'text-slate-400 font-mono cursor-pointer hover:underline transition';
            latencyEl.title = 'Click to test database latency';
          }
        }

        if (memEl) {
          if (mem !== null && mem !== undefined) {
            memEl.textContent = `Memory: ${mem}MB`;
          } else if (window.performance && performance.memory) {
            const usedMB = Math.round(performance.memory.usedJSHeapSize / 1024 / 1024);
            memEl.textContent = `Memory: ${usedMB}MB`;
          }
        }
      } else {
        if (dbInfoEl) {
          dbInfoEl.textContent = 'No active connection';
          dbInfoEl.title = '';
        }
        if (engineEl) {
          if (engineIconEl && engineLabelEl) {
            engineIconEl.innerHTML = renderIcon('database', 'w-3.5 h-3.5 text-slate-500');
            engineLabelEl.textContent = 'No Engine';
          } else {
            engineEl.innerHTML = renderIcon('database', 'w-3.5 h-3.5 text-slate-500 mr-1.5') + '<span>No Engine</span>';
          }
          engineEl.className = 'inline-flex items-center space-x-1.5 px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-800/70 text-slate-400 border border-slate-700/60 shadow-sm shrink-0';
          engineEl.title = 'No active database connection';
          engineEl.classList.remove('hidden');
        }
        if (dot1El) dot1El.classList.add('hidden');
        if (dot2El) dot2El.classList.add('hidden');
        if (uptimeEl) uptimeEl.classList.add('hidden');
        if (dot3El) dot3El.classList.add('hidden');

        if (latencyEl) {
          latencyEl.textContent = 'Latency: --';
          latencyEl.className = 'text-slate-400 font-mono';
          latencyEl.title = '';
        }
        if (memEl) memEl.textContent = 'Memory: --';
      }
    }

    // Staged grid edits are keyed by row identity instead of array index, so sorting, filtering or
    // paging the grid can never re-target a staged edit (or its discard) at a different row.
    const gridRowUids = new WeakMap();
    let gridRowUidSeq = 0;
    function stagedKeyFor(tableName, rIdx, cIdx) {
      const row = currentResults && currentResults[rIdx];
      let uid = 'i' + rIdx;
      if (row && typeof row === 'object') {
        uid = gridRowUids.get(row);
        if (uid === undefined) {
          uid = ++gridRowUidSeq;
          gridRowUids.set(row, uid);
        }
      }
      return `${tableName || ''}:${uid}:${cIdx}`;
    }

    // stageGridEdit records the edited row object (restored on discard) and keeps the ORIGINAL old
    // value when the same cell is edited more than once.
    function stageGridEdit(key, entry) {
      const prev = stagedEdits.get(key);
      if (prev) entry.oldVal = prev.oldVal;
      entry.rowRef = prev?.rowRef || (currentResults && currentResults[entry.rIdx]) || null;
      stagedEdits.set(key, entry);
    }

    function escapeHtml(str) {
      return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

    // jsArg renders a value as a JS string literal safe inside a double-quoted inline handler attribute
    // (onclick="fn(${jsArg(v)})"). escapeHtml alone is not enough there: the HTML parser decodes &#39;
    // back to ' before the handler's JS is compiled.
    function jsArg(value) {
      return escapeHtml(JSON.stringify(String(value ?? '')));
    }

    // ponytail: unified lightweight toast notification system supporting dialog top-layers and multi-severity styling
    function showToast(message, type = 'info') {
      if (!message || typeof document === 'undefined') return;
      const activeDialog = typeof document.querySelector === 'function' ? document.querySelector('dialog[open]') : null;
      const host = activeDialog || document.body;
      if (!host || typeof host.querySelector !== 'function' || typeof host.appendChild !== 'function') return;
      let container = host.querySelector('.app-toast-container');
      if (!container) {
        if (typeof document.createElement !== 'function') return;
        container = document.createElement('div');
        container.className = 'app-toast-container fixed bottom-8 right-6 z-[9999] flex flex-col space-y-2 max-w-sm pointer-events-auto';
        host.appendChild(container);
      }

      const styles = {
        info: 'bg-slate-900 border-sky-500/80 text-sky-200 ring-sky-500/30',
        success: 'bg-slate-900 border-emerald-500/80 text-emerald-200 ring-emerald-500/30',
        warning: 'bg-slate-900 border-amber-500/80 text-amber-200 ring-amber-500/30',
        error: 'bg-slate-900 border-rose-500/80 text-rose-200 ring-rose-500/30'
      };
      const icons = {
        info: renderIcon('info', 'w-4 h-4 text-sky-400 shrink-0'),
        success: renderIcon('check-circle', 'w-4 h-4 text-emerald-400 shrink-0'),
        warning: renderIcon('alert-triangle', 'w-4 h-4 text-amber-400 shrink-0'),
        error: renderIcon('x-circle', 'w-4 h-4 text-rose-400 shrink-0')
      };

      const toast = document.createElement('div');
      const styleClass = styles[type] || styles.info;
      const icon = icons[type] || icons.info;
      toast.className = `border rounded-xl px-3.5 py-2.5 shadow-2xl text-xs flex items-center space-x-2.5 transition-all duration-300 ring-2 animate-in fade-in slide-in-from-bottom-2 ${styleClass}`;
      toast.innerHTML = `
        <span class="shrink-0 select-none">${icon}</span>
        <span class="flex-1 font-medium leading-tight">${escapeHtml(message)}</span>
        <button onclick="this.parentElement.remove()" class="text-slate-400 hover:text-white text-xs px-1 select-none"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
      `;
      container.appendChild(toast);
      setTimeout(() => {
        if (toast.parentNode) {
          toast.classList.add('opacity-0', 'translate-y-2');
          setTimeout(() => toast.remove(), 300);
        }
      }, 4000);
    }

