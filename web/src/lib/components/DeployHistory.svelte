<script lang="ts">
  import type { AppSummary } from "$lib/api";
  import { timeAgo } from "$lib/format";

  // The last dozen deploys as a strip of ticks, like a chart recorder:
  // green went live, red failed, amber is still going. Empty slots stay
  // dim so a young environment reads as young, not as idle.
  let { history = [], slots = 12 }: { history?: AppSummary["history"]; slots?: number } = $props();

  const tone: Record<string, string> = {
    succeeded: "bg-success",
    failed: "bg-destructive",
    running: "bg-amber animate-pulse",
    pending: "bg-amber/50",
  };

  let recent = $derived(history.slice(-slots));
  let failures = $derived(recent.filter((d) => d.status === "failed").length);
</script>

<span
  class="inline-flex items-end gap-[3px] h-4"
  role="img"
  aria-label="{recent.length} recent deploys, {failures} failed"
  title="{recent.length} recent deploys, {failures} failed"
>
  {#each Array(Math.max(0, slots - recent.length)) as _}
    <span class="w-[5px] h-1.5 rounded-[1px] bg-border"></span>
  {/each}
  {#each recent as d (d.id)}
    <span
      class="w-[5px] rounded-[1px] {tone[d.status] ?? 'bg-border'} {d.status === 'failed' ? 'h-4' : 'h-3'}"
      title="#{d.id} {d.status === 'succeeded' ? 'went live' : d.status}, {timeAgo(d.created_at)}"
    ></span>
  {/each}
</span>
