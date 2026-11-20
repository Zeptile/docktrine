<script lang="ts">
  // Unique per instance, so several on a page don't share gradients
  const uid = Math.random().toString(36).slice(2, 8);
  // A pressure gauge for an environment. The needle rests at zero with
  // nothing deployed, sits in the red after a failed deploy, swings in the
  // amber while one runs and holds in the green when the latest is live.
  interface Props {
    status?: string;
    size?: number;
    // Sweep up from zero when first shown
    sweep?: boolean;
    class?: string;
  }

  let { status = "none", size = 112, sweep = false, class: className = "" }: Props = $props();

  const angles: Record<string, number> = {
    none: -128,
    failed: -98,
    pending: -48,
    running: -6,
    succeeded: 96,
  };

  // Gauge arc from -135° to 135°, clockwise from straight up
  const R = 44;
  function point(deg: number, r: number) {
    const a = ((deg - 90) * Math.PI) / 180;
    return [60 + r * Math.cos(a), 60 + r * Math.sin(a)];
  }
  function arc(from: number, to: number, r = R) {
    const [x1, y1] = point(from, r);
    const [x2, y2] = point(to, r);
    return `M${x1.toFixed(2)},${y1.toFixed(2)} A${r},${r} 0 ${to - from > 180 ? 1 : 0} 1 ${x2.toFixed(2)},${y2.toFixed(2)}`;
  }

  const ticks = Array.from({ length: 28 }, (_, i) => -135 + i * 10);
  let angle = $derived(angles[status] ?? angles.none);
  let swinging = $derived(status === "running" || status === "pending");
  let lamp = $derived(
    status === "succeeded" ? "#3fae8f" : status === "failed" ? "#d8483a" : swinging ? "#f0b43c" : "#5a5045"
  );

  // Small gauges drop the ticks and thicken the rest so they still read
  let small = $derived(size < 60);
  let zone = $derived(small ? 11 : 6);

  let shown = $state(!sweep);
  $effect(() => {
    if (!sweep) return;
    const t = requestAnimationFrame(() => (shown = true));
    return () => cancelAnimationFrame(t);
  });
</script>

<svg viewBox="0 0 120 120" width={size} height={size} class={className} role="img" aria-label="Gauge: {status}">
  <defs>
    <linearGradient id="bezel-{uid}" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#f4dc9e" />
      <stop offset="0.45" stop-color="#c49341" />
      <stop offset="1" stop-color="#6d4e18" />
    </linearGradient>
    <radialGradient id="face-{uid}" cx="0.45" cy="0.35" r="0.75">
      <stop offset="0" stop-color="#fbf8f0" />
      <stop offset="1" stop-color="#e6dfcf" />
    </radialGradient>
  </defs>

  <circle cx="60" cy="60" r="58" fill="url(#bezel-{uid})" />
  <circle cx="60" cy="60" r="52" fill="#3a2f22" />
  <circle cx="60" cy="60" r="50.5" fill="url(#face-{uid})" />

  <!-- Zones: red (failed), amber (deploying), green (live) -->
  <path d={arc(-135, -75, small ? 40 : R)} stroke="#b23a2a" stroke-width={zone} fill="none" />
  <path d={arc(-75, 30, small ? 40 : R)} stroke="#d99a2b" stroke-width={zone} fill="none" opacity="0.85" />
  <path d={arc(30, 135, small ? 40 : R)} stroke="#2f7a66" stroke-width={zone} fill="none" />

  {#each small ? [] : ticks as t, i}
    {@const [x1, y1] = point(t, i % 3 === 0 ? 34 : 37)}
    {@const [x2, y2] = point(t, 41)}
    <line x1={x1} y1={y1} x2={x2} y2={y2} stroke="#2a231c" stroke-width={i % 3 === 0 ? 1.4 : 0.7} />
  {/each}

  <!-- The tell-tale lamp under the pivot, for reading it at a glance -->
  <circle cx="60" cy="88" r={small ? 9 : 5.2} fill="#3a2f22" />
  <circle cx="60" cy="88" r={small ? 7 : 3.8} fill={lamp} class={swinging ? "flicker" : ""} />
  {#if lamp !== "#5a5045"}
    <circle cx="60" cy="88" r={small ? 11 : 7} fill={lamp} opacity="0.25" />
  {/if}

  <g
    class="needle {swinging ? 'swing' : ''}"
    style="transform: rotate({shown ? angle : -128}deg)"
  >
    <path d={small ? "M60 20 L64.5 60 L55.5 60 Z" : "M60 18 L62.2 60 L57.8 60 Z"} fill="#2a231c" />
    <path d="M60 60 L61.6 72 L58.4 72 Z" fill="#2a231c" />
  </g>
  <circle cx="60" cy="60" r={small ? 8 : 5.5} fill="url(#bezel-{uid})" stroke="#5e4314" stroke-width="0.8" />
</svg>

<style>
  .needle {
    transform-origin: 60px 60px;
    transform-box: view-box;
    transition: transform 1.1s cubic-bezier(0.34, 1.4, 0.5, 1);
  }
  /* Steam up: the needle won't settle */
  .swing {
    animation: swing 1.8s ease-in-out infinite;
  }
  .flicker {
    animation: flicker 1.4s ease-in-out infinite;
  }
  @keyframes swing {
    0%,
    100% {
      translate: 0;
      rotate: -14deg;
    }
    50% {
      rotate: 14deg;
    }
  }
</style>
