export interface ViewportFitOptions {
  x?: number;
  y?: number;
  anchor?: HTMLElement | null;
  align?: 'left' | 'right';
  margin?: number;
  gap?: number;
}

/**
 * Svelte action that keeps a popup/context menu completely inside the viewport.
 * - With `x`/`y`: positions `fixed` at cursor, flipping or clamping to stay on-screen.
 * - With `anchor`: positions `fixed` relative to the trigger element, flipping above/left when needed.
 * - With neither: adjusts an `absolute` dropdown in-place if it overflows the viewport.
 */
export function viewportFit(node: HTMLElement, initialOpts: ViewportFitOptions = {}) {
  let opts = initialOpts;
  let rafId = 0;

  function update() {
    const m = opts.margin ?? 8;
    const gap = opts.gap ?? 4;
    const vw = window.innerWidth;
    const vh = window.innerHeight;

    // Reset previous max-height/transform before measuring natural size
    node.style.maxHeight = `${Math.max(120, vh - m * 2)}px`;
    node.style.overflowY = 'auto';

    if (opts.x !== undefined && opts.y !== undefined) {
      node.style.position = 'fixed';
      const rect = node.getBoundingClientRect();
      const w = rect.width;
      const h = rect.height;

      let left = opts.x;
      if (left + w > vw - m) {
        left = opts.x - w >= m ? opts.x - w : Math.max(m, vw - w - m);
      }
      left = Math.max(m, left);

      let top = opts.y;
      if (top + h > vh - m) {
        top = opts.y - h >= m ? opts.y - h : Math.max(m, vh - h - m);
      }
      top = Math.max(m, top);

      node.style.left = `${Math.round(left)}px`;
      node.style.top = `${Math.round(top)}px`;
      node.style.maxHeight = `${Math.max(120, vh - top - m)}px`;
      return;
    }

    if (opts.anchor) {
      node.style.position = 'fixed';
      node.style.right = 'auto';
      node.style.bottom = 'auto';
      const a = opts.anchor.getBoundingClientRect();
      const rect = node.getBoundingClientRect();
      const w = rect.width;
      const h = rect.height;

      let left = opts.align === 'right' ? a.right - w : a.left;
      if (left + w > vw - m) left = vw - w - m;
      if (left < m) left = m;

      const spaceBelow = vh - a.bottom - gap - m;
      const spaceAbove = a.top - gap - m;
      let top = a.bottom + gap;
      let maxH = spaceBelow;

      if (h > spaceBelow && spaceAbove > spaceBelow) {
        top = Math.max(m, a.top - gap - Math.min(h, spaceAbove));
        maxH = spaceAbove;
      }

      node.style.left = `${Math.round(left)}px`;
      node.style.top = `${Math.round(top)}px`;
      node.style.maxHeight = `${Math.max(120, Math.floor(maxH))}px`;
      return;
    }

    // In-place adjustment for relative/absolute dropdowns
    node.style.transform = '';
    const rect = node.getBoundingClientRect();
    let dx = 0;
    if (rect.right > vw - m) dx = vw - m - rect.right;
    if (rect.left + dx < m) dx = m - rect.left;
    if (dx !== 0) {
      node.style.transform = `translateX(${Math.round(dx)}px)`;
    }
    if (rect.bottom > vh - m) {
      const availBelow = vh - rect.top - m;
      const availAbove = rect.top - m;
      if (availBelow < 160 && availAbove > availBelow) {
        node.style.top = 'auto';
        node.style.bottom = '100%';
        node.style.marginTop = '0px';
        node.style.marginBottom = '4px';
        node.style.maxHeight = `${Math.max(120, Math.floor(availAbove))}px`;
      } else {
        node.style.maxHeight = `${Math.max(120, Math.floor(availBelow))}px`;
      }
    }
  }

  function scheduleUpdate() {
    cancelAnimationFrame(rafId);
    rafId = requestAnimationFrame(update);
  }

  // Run immediately after layout and observe size changes (e.g. dynamic content)
  scheduleUpdate();
  const ro = new ResizeObserver(scheduleUpdate);
  ro.observe(node);
  window.addEventListener('resize', scheduleUpdate);

  return {
    update(newOpts: ViewportFitOptions = {}) {
      opts = newOpts;
      scheduleUpdate();
    },
    destroy() {
      cancelAnimationFrame(rafId);
      ro.disconnect();
      window.removeEventListener('resize', scheduleUpdate);
    },
  };
}
