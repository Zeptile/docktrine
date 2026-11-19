<script lang="ts">
  let { label = "Loading" }: { label?: string } = $props();
</script>

<!-- Three containers being stacked, one after another -->
<div class="flex flex-col items-center justify-center gap-3 py-16 text-muted-foreground" role="status">
  <div class="flex items-end gap-1" aria-hidden="true">
    {#each [0, 1, 2] as i}
      <span
        class="corrugated-x block w-6 h-4 rounded-[2px] text-signal animate-[stack_1.2s_ease-in-out_infinite]"
        style="animation-delay: {i * 0.15}s"
      ></span>
    {/each}
  </div>
  <span class="text-sm">{label}…</span>
</div>

<style>
  @keyframes -global-stack {
    0%,
    100% {
      transform: translateY(0);
      opacity: 0.5;
    }
    40% {
      transform: translateY(-6px);
      opacity: 1;
    }
  }
</style>
