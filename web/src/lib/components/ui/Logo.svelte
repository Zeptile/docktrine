<script lang="ts">
  // Unique per instance, so several on a page don't share gradients
  const uid = Math.random().toString(36).slice(2, 8);
  let { class: className = "" }: { class?: string } = $props();

  // Ten teeth around a hub: each tooth a short trapezoid on the rim
  const teeth = Array.from({ length: 10 }, (_, i) => i * 36);
</script>

<!-- A brass cog with an anchor cut through it: the dock and the machinery -->
<svg viewBox="0 0 32 32" class={className} aria-hidden="true">
  <defs>
    <linearGradient id="logo-brass-{uid}" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#f0d38c" />
      <stop offset="0.5" stop-color="#c99a45" />
      <stop offset="1" stop-color="#8e6522" />
    </linearGradient>
  </defs>
  <g fill="url(#logo-brass-{uid})" stroke="#5e4314" stroke-width="0.6">
    {#each teeth as angle}
      <path d="M14 2.2h4l0.9 4h-5.8z" transform="rotate({angle} 16 16)" />
    {/each}
    <circle cx="16" cy="16" r="11.2" />
  </g>
  <circle cx="16" cy="16" r="8.4" fill="#2a231c" />
  <g fill="none" stroke="url(#logo-brass-{uid})" stroke-width="1.5" stroke-linecap="round">
    <circle cx="16" cy="10.4" r="1.3" />
    <path d="M16 11.7v10.4M13.2 13.6h5.6M11.4 18.3c.6 2.6 2.4 3.8 4.6 3.8s4-1.2 4.6-3.8" />
  </g>
  <g fill="url(#logo-brass-{uid})">
    <path d="M10.4 17.2l2 .2-1 1.8z" />
    <path d="M21.6 17.2l-2 .2 1 1.8z" />
  </g>
</svg>
