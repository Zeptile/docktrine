<script lang="ts">
  import Logo from "./Logo.svelte";

  let { label = "Loading" }: { label?: string } = $props();
</script>

<div class="flex items-center gap-3 py-16 text-muted-foreground" role="status">
  <Logo class="w-6 h-6 animate-spin [animation-duration:2.4s]" />
  <span class="text-sm">{label}…</span>
</div>
