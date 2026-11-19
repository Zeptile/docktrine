<script lang="ts">
  import { cn } from "$lib/utils/cn";

  interface Props {
    status: string;
    class?: string;
  }

  let { status, class: className }: Props = $props();

  const styles: Record<string, string> = {
    succeeded: "text-success bg-success/10 border-success/30",
    running: "text-info bg-info/10 border-info/30",
    pending: "text-info bg-info/10 border-info/30",
    failed: "text-destructive bg-destructive/10 border-destructive/30",
    // Container states
    exited: "text-destructive bg-destructive/10 border-destructive/30",
    dead: "text-destructive bg-destructive/10 border-destructive/30",
    restarting: "text-warning bg-warning/10 border-warning/30",
    paused: "text-warning bg-warning/10 border-warning/30",
  };

  const labels: Record<string, string> = {
    running: "deploying",
    pending: "queued",
  };
</script>

<span
  class={cn(
    "inline-flex items-center gap-1.5 h-5 px-1.5 rounded-sm border text-[11px] font-semibold tracking-wide whitespace-nowrap",
    styles[status] ?? "text-muted-foreground bg-muted border-border",
    className
  )}
>
  <span
    class="w-1.5 h-1.5 rounded-full bg-current {status === 'running' || status === 'pending'
      ? 'animate-pulse'
      : ''}"
  ></span>
  {labels[status] ?? status}
</span>
