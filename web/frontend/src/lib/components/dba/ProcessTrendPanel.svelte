<script lang="ts">
  import { onMount } from 'svelte';
  import type { ProcesslistThread } from '../../../types';
  import Icon from '../common/Icon.svelte';

  let { threads = [] }: { threads: ProcesslistThread[] } = $props();

  let canvasEl = $state<HTMLCanvasElement | null>(null);
  let isOpen = $state(false);

  interface TrendPoint {
    time: number;
    active: number;
    total: number;
  }

  let history = $state<TrendPoint[]>([]);
  let peakActive = $state(0);
  let avgActive = $state(0);

  $effect(() => {
    // When threads update, push a point
    const activeCount = threads.filter((t) => t.command !== 'Sleep' && t.command !== 'Daemon').length;
    const totalCount = threads.length;
    const now = Date.now();

    history.push({ time: now, active: activeCount, total: totalCount });
    if (history.length > 60) {
      history.shift();
    }

    // Recalculate metrics
    let max = 0;
    let sum = 0;
    history.forEach((pt) => {
      if (pt.active > max) max = pt.active;
      sum += pt.active;
    });
    peakActive = max;
    avgActive = history.length > 0 ? parseFloat((sum / history.length).toFixed(1)) : 0;

    renderCanvas();
  });

  function renderCanvas() {
    if (!canvasEl || history.length === 0) return;
    const ctx = canvasEl.getContext('2d');
    if (!ctx) return;

    const width = canvasEl.width;
    const height = canvasEl.height;
    ctx.clearRect(0, 0, width, height);

    const maxVal = Math.max(10, peakActive + 2);
    const stepX = width / Math.max(1, history.length - 1);

    // Grid lines
    ctx.strokeStyle = '#1e293b';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, height / 2);
    ctx.lineTo(width, height / 2);
    ctx.stroke();

    // Line: Active threads
    ctx.strokeStyle = '#38bdf8';
    ctx.lineWidth = 2;
    ctx.beginPath();

    history.forEach((pt, i) => {
      const x = i * stepX;
      const y = height - (pt.active / maxVal) * (height - 10) - 5;
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();

    // Gradient fill under active threads line
    ctx.lineTo(width, height);
    ctx.lineTo(0, height);
    ctx.closePath();
    const grad = ctx.createLinearGradient(0, 0, 0, height);
    grad.addColorStop(0, 'rgba(56, 189, 248, 0.25)');
    grad.addColorStop(1, 'rgba(56, 189, 248, 0.0)');
    ctx.fillStyle = grad;
    ctx.fill();
  }

  onMount(() => {
    renderCanvas();
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
          <span class="text-sky-400 font-bold">{threads.filter((t) => t.command !== 'Sleep').length}</span>
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
      <div class="h-24 w-full bg-[#050811] rounded-lg border border-slate-800 p-2 relative overflow-hidden">
        <canvas
          bind:this={canvasEl}
          width="800"
          height="80"
          class="w-full h-full block"
        ></canvas>
      </div>
    </div>
  {/if}
</div>
