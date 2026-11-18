<script lang="ts">
  import { cn } from "$lib/utils/cn";

  interface Props {
    status: string;
    class?: string;
  }

  let { status, class: className }: Props = $props();

  const styles: Record<string, string> = {
    succeeded: "bg-green-100 text-green-700",
    running: "bg-blue-100 text-blue-700",
    pending: "bg-blue-100 text-blue-700",
    failed: "bg-red-100 text-red-700",
    // Container states
    exited: "bg-red-100 text-red-700",
    restarting: "bg-yellow-100 text-yellow-700",
    paused: "bg-yellow-100 text-yellow-700",
  };
</script>

<span
  class={cn(
    "inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full font-medium",
    styles[status] ?? "bg-muted text-muted-foreground",
    className
  )}
>
  {#if status === "running" || status === "pending"}
    <span class="w-1.5 h-1.5 rounded-full bg-current animate-pulse"></span>
  {/if}
  {status}
</span>
