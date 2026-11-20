<script lang="ts">
  // A brass steam pipe strapped under the top bar: a round body, a flanged
  // coupling every so often and iron clamps holding it up.
  let { class: className = "" }: { class?: string } = $props();
</script>

<svg class="block w-full h-[22px] -mb-[22px] relative z-10 pointer-events-none {className}" aria-hidden="true">
  <defs>
    <!-- Lit from above: a bright band a third of the way down, dark underneath -->
    <linearGradient id="pipe-body" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#5e4314" />
      <stop offset="0.14" stop-color="#b98a3c" />
      <stop offset="0.32" stop-color="#f7e3ad" />
      <stop offset="0.46" stop-color="#d9ab58" />
      <stop offset="0.78" stop-color="#8f6524" />
      <stop offset="1" stop-color="#47330f" />
    </linearGradient>
    <linearGradient id="pipe-flange" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#4f3910" />
      <stop offset="0.12" stop-color="#a77a30" />
      <stop offset="0.3" stop-color="#eed294" />
      <stop offset="0.5" stop-color="#c4944a" />
      <stop offset="0.85" stop-color="#7a5519" />
      <stop offset="1" stop-color="#3d2b0c" />
    </linearGradient>
    <linearGradient id="pipe-clamp" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0" stop-color="#1b1712" />
      <stop offset="0.4" stop-color="#4a4036" />
      <stop offset="1" stop-color="#1b1712" />
    </linearGradient>
    <pattern id="pipe-run" width="240" height="22" patternUnits="userSpaceOnUse">
      <!-- Body -->
      <rect x="0" y="2" width="240" height="12" fill="url(#pipe-body)" />
      <!-- Flanged coupling with a bolt above and below -->
      <rect x="20" y="0" width="16" height="18" rx="1.5" fill="url(#pipe-flange)" />
      <rect x="20" y="0" width="1" height="18" fill="#3d2b0c" opacity="0.7" />
      <rect x="35" y="0" width="1" height="18" fill="#3d2b0c" opacity="0.7" />
      <rect x="27" y="0" width="2" height="18" fill="#3d2b0c" opacity="0.45" />
      <rect x="22.5" y="15.5" width="3.5" height="4" rx="0.6" fill="#6b4c17" />
      <rect x="30" y="15.5" width="3.5" height="4" rx="0.6" fill="#6b4c17" />
      <!-- Iron clamp holding it to the bar -->
      <rect x="140" y="0" width="7" height="16" rx="1" fill="url(#pipe-clamp)" />
      <circle cx="143.5" cy="13" r="1.3" fill="#7d6f60" />
    </pattern>
    <filter id="pipe-shadow" x="0" y="0" width="1" height="2">
      <feDropShadow dx="0" dy="2.5" stdDeviation="1.6" flood-color="#000" flood-opacity="0.35" />
    </filter>
  </defs>
  <rect width="100%" height="20" fill="url(#pipe-run)" filter="url(#pipe-shadow)" />
</svg>
