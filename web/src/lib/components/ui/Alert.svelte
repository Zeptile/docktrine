<script lang="ts">
  import { cn } from "$lib/utils/cn";
  import type { Snippet } from "svelte";
  import { AlertTriangle, CircleAlert, CircleCheck } from "lucide-svelte";

  interface Props {
    kind?: "error" | "warning" | "success";
    class?: string;
    children?: Snippet;
  }

  let { kind = "error", class: className, children }: Props = $props();

  const styles = {
    error: "border-destructive/40 bg-destructive/10 text-destructive",
    warning: "border-warning/40 bg-warning/10 text-warning",
    success: "border-success/40 bg-success/10 text-success",
  };
</script>

<div
  role={kind === "error" ? "alert" : "status"}
  class={cn("flex gap-2.5 rounded-md border px-4 py-3 text-sm", styles[kind], className)}
>
  {#if kind === "error"}
    <CircleAlert class="w-4 h-4 mt-0.5 shrink-0" />
  {:else if kind === "warning"}
    <AlertTriangle class="w-4 h-4 mt-0.5 shrink-0" />
  {:else}
    <CircleCheck class="w-4 h-4 mt-0.5 shrink-0" />
  {/if}
  <div class="min-w-0 break-words text-foreground">
    {#if children}{@render children()}{/if}
  </div>
</div>
