<script lang="ts">
  import { pending } from "$lib/confirm";
  import Button from "./Button.svelte";

  let dialog: HTMLDialogElement;

  $effect(() => {
    if ($pending && !dialog.open) dialog.showModal();
    if (!$pending && dialog.open) dialog.close();
  });

  function answer(ok: boolean) {
    $pending?.resolve(ok);
  }
</script>

<dialog
  bind:this={dialog}
  onclose={() => answer(false)}
  onclick={(e) => e.target === dialog && answer(false)}
  class="w-[min(28rem,calc(100vw-2rem))] rounded-lg border bg-card text-foreground p-0 shadow-2xl backdrop:bg-[rgb(var(--terminal)/0.55)] backdrop:backdrop-blur-[2px]"
>
  {#if $pending}
    <div class="p-6 border-t-4 {$pending.danger ? 'border-t-destructive' : 'border-t-brass'} rounded-t-lg">
      <h2 class="text-lg font-semibold">{$pending.title}</h2>
      <p class="text-sm text-muted-foreground mt-2 break-words">{$pending.message}</p>
      <div class="flex justify-end gap-2 mt-6">
        <Button variant="outline" onclick={() => answer(false)}>Cancel</Button>
        <Button variant={$pending.danger ? "destructive" : "default"} onclick={() => answer(true)}>
          {$pending.confirmLabel}
        </Button>
      </div>
    </div>
  {/if}
</dialog>
