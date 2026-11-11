<script lang="ts">
  interface Props {
    name: string;
    class?: string;
  }

  let { name, class: className = 'w-4 h-4' }: Props = $props();

  let isFilled = $derived(name === 'play' || name === 'pause' || className.includes('fill-'));
</script>

{#if name === 'brand'}
  <svg class="{className} flex-shrink-0" aria-label="SqlFox Logo">
    <use href="#sqlfox-brand-mark" />
  </svg>
{:else}
  <svg class="{className} inline-block shrink-0 stroke-current {isFilled ? 'fill-current' : 'fill-none'}" aria-hidden="true">
    <use href="#icon-{name}" />
  </svg>
{/if}
