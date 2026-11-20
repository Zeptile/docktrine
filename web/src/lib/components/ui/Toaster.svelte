<script lang="ts">
  import { toasts, dismiss } from "$lib/toast";
  import { fly } from "svelte/transition";
  import { X } from "lucide-svelte";
</script>

<div class="fixed z-50 bottom-4 inset-x-4 sm:left-auto sm:w-96 flex flex-col gap-2" role="status" aria-live="polite">
  {#each $toasts as t (t.id)}
    <div
      transition:fly={{ y: 12, duration: 180 }}
      class="flex items-center gap-3 rounded-md bg-[rgb(var(--terminal))] text-[rgb(var(--terminal-foreground))] shadow-lg pl-4"
    >
      <span class="light {t.kind === 'error' ? 'light-failed' : 'light-live'}"></span>
      <p class="flex-1 py-3 text-sm break-words">{t.message}</p>
      <button class="self-stretch px-3 opacity-60 hover:opacity-100" aria-label="Dismiss" onclick={() => dismiss(t.id)}>
        <X class="w-4 h-4" />
      </button>
    </div>
  {/each}
</div>
