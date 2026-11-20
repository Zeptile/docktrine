<script lang="ts">
  import { cn } from "$lib/utils/cn";
  import type { HTMLButtonAttributes } from "svelte/elements";
  import type { Snippet } from "svelte";

  interface Props extends HTMLButtonAttributes {
    variant?:
      | "default"
      | "destructive"
      | "outline"
      | "secondary"
      | "ghost"
      | "link";
    size?: "default" | "sm" | "lg" | "icon";
    class?: string;
    children?: Snippet;
  }

  let {
    variant = "default",
    size = "default",
    class: className,
    children,
    ...props
  }: Props = $props();

  const variants = {
    default: "brass-fill font-semibold",
    destructive: "bg-destructive text-destructive-foreground font-semibold hover:bg-destructive/90",
    outline: "border border-input bg-card hover:bg-muted hover:border-foreground/30",
    secondary: "bg-plate text-foreground hover:bg-plate-deep",
    ghost: "text-muted-foreground hover:bg-muted hover:text-foreground",
    link: "text-link underline-offset-4 hover:underline",
  };

  const sizes = {
    default: "h-9 px-3.5 gap-2",
    sm: "h-8 px-2.5 gap-1.5 text-[13px]",
    lg: "h-11 px-5 gap-2 text-[15px]",
    icon: "h-9 w-9",
  };
</script>

<button
  class={cn(
    "inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors active:translate-y-px disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0",
    variants[variant],
    sizes[size],
    className
  )}
  {...props}
>
  {#if children}
    {@render children()}
  {/if}
</button>
