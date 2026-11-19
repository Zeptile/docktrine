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
    default:
      "bg-signal text-signal-ink font-semibold shadow-[inset_0_-2px_0_rgb(0_0_0/0.18)] hover:brightness-105 active:shadow-none active:translate-y-px",
    destructive:
      "bg-destructive text-destructive-foreground font-semibold hover:bg-destructive/90 active:translate-y-px",
    outline:
      "border border-input bg-card hover:bg-muted hover:border-muted-foreground/40",
    secondary: "bg-muted text-foreground hover:bg-muted/70",
    ghost: "text-muted-foreground hover:bg-muted hover:text-foreground",
    link: "text-link underline-offset-4 hover:underline",
  };

  const sizes = {
    default: "h-10 px-4 gap-2",
    sm: "h-8 px-3 gap-1.5 text-[13px]",
    lg: "h-11 px-6 gap-2",
    icon: "h-10 w-10",
  };
</script>

<button
  class={cn(
    "inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-[background-color,border-color,filter,transform] disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0",
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
