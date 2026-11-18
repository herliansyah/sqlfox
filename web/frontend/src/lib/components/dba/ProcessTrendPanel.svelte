<script lang="ts">
  import Icon from '../common/Icon.svelte';

  export interface TrendSample {
    ts: number;
    running: number;
    total: number;
    qps: number;
  }

  interface Props {
    history: TrendSample[];
    open: boolean;
    onToggle: () => void;
    onEscalate: () => void;
  }

  let { history = [], open = false, onToggle, onEscalate }: Props = $props();

  let wrap: HTMLDivElement | null = $state(null);
  let canvas: HTMLCanvasElement | null = $state(null);
  let width = $state(0);
  const height = 104;

  const stats = $derived.by(() => {
    const count = history.length;
    if (count === 0) {
      return { nowRunning: 0, nowTotal: 0, nowQps: 0, peakRunning: 0, peakQps: 0, avgRunning: '0.0' };
    }
    const last = history[count - 1]!;
    let peakRunning = 0;
    let peakQps = 0;
    let sumRunning = 0;
    for (const pt of history) {
      if (pt.running > peakRunning) peakRunning = pt.running;
      if (pt.qps > peakQps) peakQps = pt.qps;
      sumRunning += pt.running;
    }
    return {
      nowRunning: last.running,
      nowTotal: last.total,
      nowQps: last.qps,
      peakRunning,
      peakQps,
      avgRunning: (sumRunning / count).toFixed(1),
    };
  });

  $effect(() => {
    if (!open || !wrap) return;
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) {
        const w = Math.floor(e.contentRect.width);
        if (w > 0 && w !== width) width = w;
      }
    });
    ro.observe(wrap);
    return () => ro.disconnect();
  });

  $effect(() => {
    if (!open || !canvas || width <= 0) return;
    const pts = history;
    const w = width;
    const h = height;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.floor(w * dpr);
    canvas.height = Math.floor(h * dpr);
    canvas.style.width = `${w}px`;
    canvas.style.height = `${h}px`;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    if (pts.length === 0) {
      ctx.fillStyle = '#64748B';
      ctx.font = '11px ui-monospace, SFMono-Regular, monospace';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText('Collecting activity ticks... Waiting for processlist polling.', w / 2, h / 2);
      return;
    }

    const padLeft = 34;
    const padRight = 38;
    const padTop = 10;
    const padBottom = 16;
    const chartW = Math.max(1, w - padLeft - padRight);
    const chartH = Math.max(1, h - padTop - padBottom);

    let maxThreads = 4;
    let maxQps = 4;
    for (const pt of pts) {
      if (pt.running > maxThreads) maxThreads = pt.running;
      if (pt.qps > maxQps) maxQps = pt.qps;
    }
    maxThreads = Math.ceil(maxThreads * 1.15);
    maxQps = Math.ceil(maxQps * 1.15);

    // Horizontal grid + left/right axis ticks
    ctx.strokeStyle = 'rgba(51, 65, 85, 0.35)';
    ctx.lineWidth = 1;
    ctx.font = '9px ui-monospace, SFMono-Regular, monospace';
    ctx.textBaseline = 'middle';
    const steps = 3;
    for (let i = 0; i <= steps; i++) {
      const y = padTop + (chartH / steps) * i;
      ctx.beginPath();
      ctx.moveTo(padLeft, Math.round(y) + 0.5);
      ctx.lineTo(padLeft + chartW, Math.round(y) + 0.5);
      ctx.stroke();

      ctx.fillStyle = '#38bdf8';
      ctx.textAlign = 'right';
      ctx.fillText(String(Math.round((maxThreads / steps) * (steps - i))), padLeft - 5, y);

      ctx.fillStyle = '#34d399';
      ctx.textAlign = 'left';
      const qv = (maxQps / steps) * (steps - i);
      ctx.fillText(qv >= 1000 ? `${(qv / 1000).toFixed(1)}k` : String(Math.round(qv)), padLeft + chartW + 5, y);
    }

    const maxSlots = 59;
    const count = pts.length;
    const xFor = (idx: number) =>
      count <= 1 ? padLeft + chartW : padLeft + (chartW / maxSlots) * (maxSlots - (count - 1 - idx));
    const yThread = (val: number) => padTop + chartH - (val / maxThreads) * chartH;
    const yQps = (val: number) => padTop + chartH - (val / maxQps) * chartH;

    // Secondary series: QPS (emerald dashed)
    ctx.beginPath();
    ctx.strokeStyle = 'rgba(52, 211, 153, 0.75)';
    ctx.setLineDash([4, 3]);
    ctx.lineWidth = 1.4;
    pts.forEach((pt, i) => {
      const x = xFor(i);
      const y = yQps(pt.qps);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
    ctx.setLineDash([]);

    // Gradient fill for Active Threads
    const grad = ctx.createLinearGradient(0, padTop, 0, padTop + chartH);
    grad.addColorStop(0, 'rgba(56, 189, 248, 0.28)');
    grad.addColorStop(1, 'rgba(56, 189, 248, 0.01)');
    ctx.beginPath();
    pts.forEach((pt, i) => {
      const x = xFor(i);
      const y = yThread(pt.running);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.lineTo(xFor(count - 1), padTop + chartH);
    ctx.lineTo(xFor(0), padTop + chartH);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    // Primary line: Active Threads (sky-400)
    ctx.beginPath();
    ctx.strokeStyle = '#38bdf8';
    ctx.lineWidth = 1.8;
    ctx.lineJoin = 'round';
    pts.forEach((pt, i) => {
      const x = xFor(i);
      const y = yThread(pt.running);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();

    // End dot
    const lastPt = pts[count - 1]!;
    ctx.fillStyle = '#38bdf8';
    ctx.beginPath();
    ctx.arc(xFor(count - 1), yThread(lastPt.running), 2.8, 0, Math.PI * 2);
    ctx.fill();
  });
</script>

{#if open}
  <div class="border-b border-[#1e293b] bg-[#0b1120] flex flex-col shrink-0 select-none">
    <div class="px-4 py-1.5 bg-[#0f172a]/80 border-b border-[#1e293b] flex items-center justify-between flex-wrap gap-2 text-[11px] font-sans">
      <div class="flex items-center space-x-4 flex-wrap gap-y-1">
        <span class="flex items-center space-x-1.5 font-bold text-slate-200">
          <Icon name="trending-up" class="w-3.5 h-3.5 text-sky-400" />
          <span>Activity Trend (Last {history.length}/60 samples)</span>
        </span>
        <span class="flex items-center space-x-1 text-sky-300">
          <span class="w-2 h-2 rounded-full bg-sky-400"></span>
          <span>Active: <strong class="font-mono text-white">{stats.nowRunning}</strong></span>
        </span>
        <span class="flex items-center space-x-1 text-emerald-300">
          <span class="w-2.5 h-0.5 bg-emerald-400"></span>
          <span>QPS: <strong class="font-mono text-white">{stats.nowQps.toFixed(1)}/s</strong></span>
        </span>
        <span class="text-slate-400">
          Total Conns: <strong class="font-mono text-slate-200">{stats.nowTotal}</strong>
        </span>
        <span class="text-amber-300/90">
          Peak Active: <strong class="font-mono">{stats.peakRunning}</strong>
        </span>
        <span class="text-slate-400">
          Avg: <strong class="font-mono">{stats.avgRunning}</strong>
        </span>
      </div>

      <div class="flex items-center space-x-2">
        <button
          type="button"
          onclick={onEscalate}
          class="px-2 py-0.5 rounded bg-rose-950/70 hover:bg-rose-900 border border-rose-800 text-rose-200 text-[10px] font-semibold transition flex items-center space-x-1"
          title="Escalate to Performance Flight Recorder for per-second tick & lock blackbox recording"
        >
          <Icon name="activity" class="w-3 h-3 text-rose-400" />
          <span>Deep Record</span>
        </button>
        <button
          type="button"
          onclick={onToggle}
          class="p-0.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Collapse Activity Trend"
          aria-label="Collapse Activity Trend"
        >
          <Icon name="x" class="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <div bind:this={wrap} class="w-full px-1 py-1 bg-[#070b14]">
      <canvas bind:this={canvas} aria-hidden="true"></canvas>
    </div>
  </div>
{/if}
