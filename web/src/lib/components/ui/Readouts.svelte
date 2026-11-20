<script lang="ts" generics="T extends string">
  // A row of counters across a riveted plate, each one a filter. The
  // selected one is underlined in brass; picking it again clears it.
  interface Readout {
    id: T;
    label: string;
    count: number;
    // A lamp class from app.css (light-live, light-failed, ...)
    light?: string;
  }

  let { items, selected = $bindable() }: { items: Readout[]; selected: T } = $props();
</script>

<div class="riveted px-3 sm:px-4 py-2 grid grid-cols-3 sm:flex sm:divide-x" role="group" aria-label="Filter by state">
  {#each items as r, i (r.id)}
    <button
      class="group relative flex-1 min-w-0 text-left px-4 sm:px-5 pt-2.5 pb-3 transition-opacity
        {r.count === 0 && selected !== r.id && i > 0 ? 'opacity-55 hover:opacity-100' : ''}"
      aria-pressed={selected === r.id}
      onclick={() => (selected = selected === r.id ? items[0].id : r.id)}
    >
      <span class="flex items-center gap-2 text-[11px] uppercase tracking-[0.12em] font-medium whitespace-nowrap transition-colors
        {selected === r.id ? 'text-brass' : 'text-muted-foreground group-hover:text-foreground'}">
        {#if r.light}<span class="shrink-0 light {r.count ? r.light : 'light-off'} !w-2 !h-2"></span>{/if}
        <span class="truncate min-w-0">{r.label}</span>
      </span>
      <span class="block font-slab font-bold text-[1.6rem] leading-tight tabular-nums">{r.count}</span>
      {#if selected === r.id}
        <span class="absolute left-4 sm:left-5 bottom-1 w-8 h-[3px] rounded-full bg-brass"></span>
      {/if}
    </button>
  {/each}
</div>
