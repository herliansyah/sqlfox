<script lang="ts">
  /**
   * Flight Timeline — multi-series sparkline timeline for recorded telemetry ticks.
   * Plain canvas + an interval-free redraw, so it never fights Svelte's reactivity.
   */
  interface Props {
    ticks: any[];
    selectedIndex?: number | null;
    onselect?: (index: number) => void;
  }

  let { ticks = [], selectedIndex = null, onselect }: Props = $props();

  let canvas: HTMLCanvasElement | null = $state(null);
  let wrap: HTMLDivElement | null = $state(null);
  let width = $state(0);
  let height = 132;

  const PAD = { left: 6, right: 6, top: 8, bottom: 18 };
  const LANES = [
    { key: 'qps', label: 'QPS', color: '#38bdf8' },
    { key: 'threads_running', label: 'Active threads', color: '#34d399' },
    { key: 'threads_connected', label: 'Connections', color: '#818cf8' },
  ];

  const points = $derived(
    (ticks || []).map((t, i) => ({
      i,
      qps: Number(t?.qps ?? 0),
      threads_running: Number(t?.threads_running ?? t?.active_threads ?? 0),
      threads_connected: Number(t?.threads_connected ?? 0),
      ts: t?.sampled_at ? new Date(t.sampled_at) : null,
    }))
  );

  $effect(() => {
    if (!wrap) return;
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) {
        const w = Math.floor(e.contentRect.width);
        if (w > 0 && w !== width) width = w;
      }
    });
    ro.observe(wrap);
    return () => ro.disconnect();
  });

  function seriesMax(key: string): number {
    let m = 0;
    for (const p of points) m = Math.max(m, Number((p as any)[key]) || 0);
    return m > 0 ? m : 1;
  }

  $effect(() => {
    // Dependencies: data, size, selection.
    const data = points;
    const sel = selectedIndex;
    const w = width;
    const el = canvas;
    if (!el || w <= 0) return;

    const dpr = window.devicePixelRatio || 1;
    el.width = Math.floor(w * dpr);
    el.height = Math.floor(height * dpr);
    el.style.width = `${w}px`;
    el.style.height = `${height}px`;

    const ctx = el.getContext('2d');
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, height);

    const plotW = Math.max(1, w - PAD.left - PAD.right);
    const plotH = height - PAD.top - PAD.bottom;
    const n = data.length;

    // Horizontal grid
    ctx.strokeStyle = 'rgba(148,163,184,0.12)';
    ctx.lineWidth = 1;
    for (let g = 0; g <= 4; g++) {
      const y = PAD.top + (plotH / 4) * g;
      ctx.beginPath();
      ctx.moveTo(PAD.left, Math.round(y) + 0.5);
      ctx.lineTo(PAD.left + plotW, Math.round(y) + 0.5);
      ctx.stroke();
    }

    const xFor = (i: number) => (n <= 1 ? PAD.left + plotW / 2 : PAD.left + (plotW * i) / (n - 1));

    // Draw each lane scaled to its own max so low-volume series stay readable.
    for (const lane of LANES) {
      const max = seriesMax(lane.key);
      ctx.beginPath();
      data.forEach((p, i) => {
        const v = Number((p as any)[lane.key]) || 0;
        const y = PAD.top + plotH - (v / max) * plotH;
        const x = xFor(i);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.strokeStyle = lane.color;
      ctx.lineWidth = 1.6;
      ctx.lineJoin = 'round';
      ctx.stroke();

      if (n > 0) {
        const last = data[n - 1]!;
        const v = Number((last as any)[lane.key]) || 0;
        const y = PAD.top + plotH - (v / max) * plotH;
        ctx.fillStyle = lane.color;
        ctx.beginPath();
        ctx.arc(xFor(n - 1), y, 2.2, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    // Selection marker
    if (sel != null && sel >= 0 && sel < n) {
      const x = xFor(sel);
      ctx.strokeStyle = 'rgba(56,189,248,0.85)';
      ctx.lineWidth = 1;
      ctx.setLineDash([3, 3]);
      ctx.beginPath();
      ctx.moveTo(Math.round(x) + 0.5, PAD.top);
      ctx.lineTo(Math.round(x) + 0.5, PAD.top + plotH);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    // Time axis
    ctx.fillStyle = 'rgba(148,163,184,0.75)';
    ctx.font = '9px ui-monospace, monospace';
    ctx.textBaseline = 'top';
    const labelIdx = n <= 1 ? [0] : [0, Math.floor((n - 1) / 2), n - 1];
    for (const i of labelIdx) {
      const p = data[i];
      if (!p?.ts || isNaN(p.ts.getTime())) continue;
      const label = p.ts.toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      });
      const x = xFor(i);
      ctx.textAlign = i === 0 ? 'left' : i === n - 1 ? 'right' : 'center';
      ctx.fillText(label, x, PAD.top + plotH + 5);
    }

    if (n === 0) {
      ctx.fillStyle = 'rgba(100,116,139,0.9)';
      ctx.font = '11px system-ui, sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText('Waiting for telemetry samples…', w / 2, height / 2 - 4);
    }
  });

  function pick(e: MouseEvent) {
    if (!onselect || points.length === 0) return;
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const plotW = Math.max(1, rect.width - PAD.left - PAD.right);
    const rel = Math.min(plotW, Math.max(0, e.clientX - rect.left - PAD.left));
    const idx =
      points.length <= 1 ? 0 : Math.round((rel / plotW) * (points.length - 1));
    onselect(Math.max(0, Math.min(points.length - 1, idx)));
  }
</script>

<div class="border border-[#1e293b] rounded-lg bg-[#0b0f19] overflow-hidden">
  <div class="px-3 py-2 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between">
    <span class="font-sans font-bold text-slate-300 text-xs">Telemetry Timeline</span>
    <div class="flex items-center gap-3">
      {#each LANES as lane}
        <span class="flex items-center gap-1 text-[10px] font-sans text-slate-400">
          <span class="w-2 h-2 rounded-sm" style="background:{lane.color}"></span>{lane.label}
        </span>
      {/each}
      <span class="text-[10px] text-slate-500 font-mono">{points.length} sample(s)</span>
    </div>
  </div>
  <div
    bind:this={wrap}
    class="relative w-full cursor-crosshair"
    role="button"
    tabindex="0"
    aria-label="Recorded telemetry timeline, click to inspect a sample"
    onclick={pick}
    onkeydown={(e) => {
      if (!onselect || points.length === 0) return;
      const cur = selectedIndex ?? points.length - 1;
      if (e.key === 'ArrowLeft') onselect(Math.max(0, cur - 1));
      if (e.key === 'ArrowRight') onselect(Math.min(points.length - 1, cur + 1));
    }}
  >
    <canvas bind:this={canvas} aria-hidden="true"></canvas>
  </div>
</div>
