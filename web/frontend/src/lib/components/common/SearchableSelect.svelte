<script lang="ts">
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';

  interface Props {
    options: string[];
    value: string;
    onchange: (value: string) => void;
    /** Visible prefix, e.g. "scope:" */
    label?: string;
    placeholder?: string;
    title?: string;
    align?: 'left' | 'right';
    triggerClass?: string;
    panelWidth?: string;
  }

  let {
    options,
    value,
    onchange,
    label = '',
    placeholder = 'Search...',
    title = '',
    align = 'left',
    triggerClass = '',
    panelWidth = 'w-64',
  }: Props = $props();

  let open = $state(false);
  let query = $state('');
  let activeIndex = $state(0);
  let triggerEl = $state<HTMLButtonElement | null>(null);
  let searchEl = $state<HTMLInputElement | null>(null);
  let listEl = $state<HTMLDivElement | null>(null);

  let filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.toLowerCase().includes(q));
  });

  $effect(() => {
    if (activeIndex >= filtered.length) activeIndex = Math.max(0, filtered.length - 1);
  });

  async function openMenu() {
    open = true;
    query = '';
    const idx = options.indexOf(value);
    activeIndex = idx >= 0 ? idx : 0;
    await tick();
    searchEl?.focus();
  }

  function closeMenu(restoreFocus = false) {
    open = false;
    query = '';
    if (restoreFocus) triggerEl?.focus();
  }

  function pick(option: string) {
    if (option !== value) onchange(option);
    closeMenu(true);
  }

  function scrollActiveIntoView() {
    tick().then(() => listEl?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' }));
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (filtered.length) {
        activeIndex = (activeIndex + 1) % filtered.length;
        scrollActiveIntoView();
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (filtered.length) {
        activeIndex = (activeIndex - 1 + filtered.length) % filtered.length;
        scrollActiveIntoView();
      }
    } else if (e.key === 'Home') {
      e.preventDefault();
      activeIndex = 0;
      scrollActiveIntoView();
    } else if (e.key === 'End') {
      e.preventDefault();
      activeIndex = Math.max(0, filtered.length - 1);
      scrollActiveIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const option = filtered[activeIndex];
      if (option) pick(option);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      closeMenu(true);
    } else if (e.key === 'Tab') {
      closeMenu();
    }
  }
</script>

<button
  bind:this={triggerEl}
  type="button"
  onclick={() => (open ? closeMenu() : openMenu())}
  onkeydown={(e) => {
    if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ')) {
      e.preventDefault();
      openMenu();
    }
  }}
  {title}
  aria-haspopup="listbox"
  aria-expanded={open}
  class="flex items-center space-x-1 rounded text-xs transition {triggerClass}"
>
  {#if label}<span class="text-slate-500">{label}</span>{/if}
  <span class="font-bold truncate max-w-[180px]">{value || '—'}</span>
  <Icon name="chevron-down" class="w-3 h-3 text-slate-500 shrink-0" />
</button>

{#if open}
  <button
    tabindex="-1"
    aria-label="Close"
    class="fixed inset-0 z-40 bg-transparent cursor-default"
    onclick={() => closeMenu()}
  ></button>

  <div
    class="absolute {align === 'right' ? 'right-0' : 'left-0'} mt-1 {panelWidth} bg-[#0f172a] border border-slate-700/90 rounded-lg shadow-2xl z-50 overflow-hidden animate-in fade-in zoom-in-95 duration-100"
  >
    <div class="p-2 border-b border-slate-800">
      <div class="relative flex items-center">
        <Icon name="search" class="w-3.5 h-3.5 text-slate-500 absolute left-2 pointer-events-none" />
        <input
          bind:this={searchEl}
          type="text"
          bind:value={query}
          onkeydown={handleKeydown}
          oninput={() => (activeIndex = 0)}
          {placeholder}
          role="combobox"
          aria-expanded="true"
          aria-controls="searchable-select-listbox"
          aria-autocomplete="list"
          class="w-full bg-[#050811] border border-slate-700 rounded pl-7 pr-2 py-1 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500"
        />
      </div>
    </div>

    <div id="searchable-select-listbox" bind:this={listEl} role="listbox" class="max-h-64 overflow-y-auto py-1">
      {#if filtered.length === 0}
        <div class="px-3 py-3 text-center text-xs text-slate-500">No matches</div>
      {:else}
        {#each filtered as option, idx (option)}
          {@const isActive = idx === activeIndex}
          <div
            data-active={isActive}
            role="option"
            tabindex="-1"
            aria-selected={option === value}
            onmouseenter={() => (activeIndex = idx)}
            onclick={() => pick(option)}
            onkeydown={(e) => e.key === 'Enter' && pick(option)}
            class="flex items-center justify-between px-3 py-1.5 cursor-pointer text-xs transition-colors {isActive
              ? 'bg-slate-800/90 ring-1 ring-inset ring-sky-500/40'
              : 'hover:bg-slate-800/80'}"
          >
            <span class="font-mono truncate {option === value ? 'text-sky-300 font-semibold' : 'text-slate-200'}">{option}</span>
            {#if option === value}
              <Icon name="check" class="w-3.5 h-3.5 text-sky-400 shrink-0" />
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  </div>
{/if}
