<script lang="ts">
  let { class: className = "" }: { class?: string } = $props();
</script>

<!-- A container door seen end-on: corrugations and a locking bar -->
<svg viewBox="0 0 32 32" class={className} aria-hidden="true">
  <rect width="32" height="32" rx="6" fill="#F5B700" />
  <g fill="#142433">
    <rect x="6" y="9" width="3" height="14" />
    <rect x="11" y="9" width="3" height="14" />
    <rect x="16" y="9" width="3" height="14" />
    <rect x="21" y="9" width="5" height="14" rx="1" />
  </g>
</svg>
