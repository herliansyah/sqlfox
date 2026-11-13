<script lang="ts">
  import type { ProcesslistThread } from '../../../types';
  import Icon from '../common/Icon.svelte';

  let { threads = [] }: { threads: ProcesslistThread[] } = $props();

  let canvasEl = $state<HTMLCanvasElement | null>(null);
  let isOpen = $state(true);

  interface TrendPoint {
    time: number;
    active: number;
  }

  // Plain array on purpose: a reactive array mutated from the sampler would
  // retrigger itself and fill the window with one value (a flat line).
  let history: TrendPoint[] = [];
  let revision = $state(0);
  let peakActive = $state(0);
  let avgActive = $state(0);

  function activeCount() {
    return threads.filter((t) => t.command !== 'Sleep' && t.command !== 'Daemon').length;
  }

  function sample() {
    history.push({ time: Date.now(), active: activeCount() });
    if (history.length > 60) history.shift();

    let max = 0;
    let sum = 0;
    for (const pt of history) {
      if (pt.active > max) max = pt.active;
      sum += pt.active;
    }
    peakActive = max;
    avgActive = history.length ? Math.round((sum / history.length) * 10) / 10 : 0;
    revision++;
  }

  $effect(() => {
    revision;
    isOpen;
    canvasEl;
    renderCanvas();
  });

  function renderCanvas() {
    const canvas = canvasEl;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    if (rect.width < 2 || rect.height < 2) return;

    const dpr = window.devicePixelRatio || 1;
    const w = Math.round(rect.width);
    const h = Math.round(rect.height);
    if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
    }

    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const maxVal = Math.max(4, peakActive + 1);
    const padTop = 6;
    const padBottom = 6;
    const chartH = h - padTop - padBottom;
    const stepX = w / Math.max(1, history.length - 1);
    const yFor = (v: number) => padTop + chartH - (v / maxVal) * chartH;

    // Crisp horizontal guides at 25 / 50 / 75%.
    ctx.strokeStyle = '#1e293b';
    ctx.lineWidth = 1;
    for (const frac of [0.25, 0.5, 0.75]) {
      const y = Math.round(padTop + chartH * frac) + 0.5;
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(w, y);
      ctx.stroke();
    }

    if (history.length > 1) {
      const trace = () => {
        ctx.beginPath();
        history.forEach((pt, i) => {
          const x = i * stepX;
          const y = yFor(pt.active);
          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        });
      };

      trace();
      ctx.lineTo((history.length - 1) * stepX, padTop + chartH);
      ctx.lineTo(0, padTop + chartH);
      ctx.closePath();
      ctx.fillStyle = 'rgba(56, 189, 248, 0.08)';
      ctx.fill();

      trace();
      ctx.strokeStyle = '#38bdf8';
      ctx.lineWidth = 1.5;
      ctx.lineJoin = 'round';
      ctx.stroke();

      const last = history[history.length - 1]!;
      ctx.fillStyle = '#38bdf8';
      ctx.beginPath();
      ctx.arc((history.length - 1) * stepX, yFor(last.active), 2, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  $effect(() => {
    const canvas = canvasEl;
    if (!canvas) return;
    const observer = new ResizeObserver(() => renderCanvas());
    observer.observe(canvas);
    return () => observer.disconnect();
  });

  // Sample once per fresh processlist fetch. Polling is the only source of new
  // `threads`, so a paused Processlist freezes the chart instead of shifting
  // stale samples leftward on a wall-clock timer.
  $effect(() => {
    threads;
    sample();
  });
</script>

<div class="border-b border-slate-800 bg-[#070b14]">
  <!-- Toggle header -->
  <div class="px-4 py-2 flex items-center justify-between text-xs">
    <button
      type="button"
      onclick={() => (isOpen = !isOpen)}
      class="flex items-center space-x-2 text-slate-300 hover:text-white font-medium transition"
    >
      <Icon name="trending-up" class="w-4 h-4 text-sky-400" />
      <span>Live Process Activity Trend</span>
      <Icon name={isOpen ? 'chevron-down' : 'chevron-right'} class="w-3.5 h-3.5 text-slate-400" />
    </button>

    {#if isOpen}
      <div class="flex items-center space-x-4 text-[11px] font-mono">
        <div>
          <span class="text-slate-500">Active Now: </span>
          <span class="text-sky-400 font-bold">{activeCount()}</span>
        </div>
        <div>
          <span class="text-slate-500">Peak: </span>
          <span class="text-amber-400 font-bold">{peakActive}</span>
        </div>
        <div>
          <span class="text-slate-500">Avg: </span>
          <span class="text-emerald-400 font-bold">{avgActive}</span>
        </div>
      </div>
    {/if}
  </div>

  {#if isOpen}
    <div class="px-4 pb-3">
      <div class="h-24 w-full bg-[#050811] rounded-md border border-slate-800 p-2 relative overflow-hidden">
        <canvas bind:this={canvasEl} class="w-full h-full block"></canvas>
        {#if revision < 2}
          <div class="absolute inset-0 flex items-center justify-center text-[10px] font-mono text-slate-500 pointer-events-none">
            Collecting samples...
          </div>
        {/if}
      </div>
      <div class="mt-1 flex items-center justify-between text-[10px] font-mono text-slate-600">
        <span>60s ago</span>
        <span>Active threads / second</span>
        <span>now</span>
      </div>
    </div>
  {/if}
</div>
