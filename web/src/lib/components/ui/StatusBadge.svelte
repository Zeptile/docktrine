<script lang="ts">
  import { cn } from "$lib/utils/cn";

  interface Props {
    status: string;
    // Show the word next to the light
    label?: boolean;
    class?: string;
  }

  let { status, label = true, class: className }: Props = $props();

  // Deployment statuses and container states, as navigation lights
  const lights: Record<string, string> = {
    succeeded: "light-live",
    running: "light-deploying",
    pending: "light-queued",
    failed: "light-failed",
    exited: "light-off",
    dead: "light-failed",
    restarting: "light-deploying",
    paused: "light-queued",
  };

  const words: Record<string, string> = {
    succeeded: "succeeded",
    running: "deploying",
    pending: "queued",
  };

  const tones: Record<string, string> = {
    failed: "text-destructive",
    dead: "text-destructive",
    running: "text-warning",
    pending: "text-warning",
  };
</script>

<span
  class={cn("inline-flex items-center gap-2 text-[13px] whitespace-nowrap", tones[status] ?? "text-muted-foreground", className)}
  title={label ? undefined : (words[status] ?? status)}
>
  <span class="light {lights[status] ?? 'light-off'}"></span>
  {#if label}{words[status] ?? status}{/if}
</span>
