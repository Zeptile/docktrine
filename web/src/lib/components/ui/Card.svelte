<script lang="ts">
  import { cn } from "$lib/utils/cn";
  import type { HTMLAttributes } from "svelte/elements";
  import type { Snippet } from "svelte";

  interface Props extends HTMLAttributes<HTMLDivElement> {
    class?: string;
    children?: Snippet;
  }

  let { class: className, children, ...props }: Props = $props();
</script>

<div class={cn("rounded-lg border bg-card text-foreground", className)} {...props}>
  {#if children}
    {@render children()}
  {/if}
</div>
