<script lang="ts">
  import { toasts, dismiss } from "$lib/toast";
  import { fly } from "svelte/transition";
  import { X } from "lucide-svelte";
</script>

<div
  class="fixed z-50 bottom-4 inset-x-4 sm:left-auto sm:w-96 flex flex-col gap-2"
  role="status"
  aria-live="polite"
>
  {#each $toasts as t (t.id)}
    <div
      transition:fly={{ y: 12, duration: 180 }}
      class="flex items-stretch rounded-md border bg-card shadow-lg overflow-hidden"
    >
      <div
        class="corrugated w-1.5 shrink-0 {t.kind === 'error' ? 'text-destructive' : 'text-success'}"
        aria-hidden="true"
      ></div>
      <p class="flex-1 px-4 py-3 text-sm break-words">{t.message}</p>
      <button
        class="px-3 text-muted-foreground hover:text-foreground"
        aria-label="Dismiss"
        onclick={() => dismiss(t.id)}
      >
        <X class="w-4 h-4" />
      </button>
    </div>
  {/each}
</div>
