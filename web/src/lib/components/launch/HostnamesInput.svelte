<script lang="ts">
  import { isHostname } from "$lib/launch";
  import { X } from "lucide-svelte";

  interface Props {
    hostnames: string[];
    id: string;
    // Offered as a one-click hostname, e.g. sentry.zeptile.software
    suggestion?: string | null;
  }

  let { hostnames = $bindable(), id, suggestion = null }: Props = $props();

  let draft = $state("");
  let error = $state<string | null>(null);
  let input: HTMLInputElement;

  // Adds whatever was typed or pasted; separators are spaces, commas and newlines
  function commit(text = draft): boolean {
    const parts = text.split(/[\s,]+/).map((p) => p.trim().toLowerCase().replace(/^https?:\/\//, "").replace(/\/.*$/, "")).filter(Boolean);
    const bad = parts.filter((p) => !isHostname(p));
    for (const p of parts) {
      if (isHostname(p) && !hostnames.includes(p)) hostnames.push(p);
    }
    if (bad.length) {
      error = `${bad[0]} isn't a hostname, like app.example.com`;
      draft = bad.join(" ");
      return false;
    }
    error = null;
    draft = "";
    return true;
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Enter" || e.key === "," || e.key === " ") {
      if (draft.trim()) {
        e.preventDefault();
        commit();
      } else if (e.key !== "Enter") {
        e.preventDefault();
      }
    } else if (e.key === "Backspace" && !draft && hostnames.length) {
      hostnames.pop();
    }
  }

  function onPaste(e: ClipboardEvent) {
    const text = e.clipboardData?.getData("text") ?? "";
    if (/[\s,]/.test(text.trim())) {
      e.preventDefault();
      commit(text);
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  class="input h-auto min-h-10 py-1.5 flex flex-wrap items-center gap-1.5 cursor-text focus-within:border-brass focus-within:shadow-[inset_0_1px_2px_rgb(0_0_0/0.08),0_0_0_3px_rgb(var(--brass)/0.22)]
    {error ? 'border-destructive' : ''}"
  onclick={() => input.focus()}
>
  {#each hostnames as host, i (host)}
    <span class="inline-flex items-center gap-1 h-7 pl-2.5 pr-1 rounded-sm bg-plate font-mono text-[13px]">
      {host}
      <button
        type="button"
        class="h-5 w-5 grid place-items-center rounded-sm text-muted-foreground hover:text-foreground hover:bg-plate-deep"
        aria-label="Remove {host}"
        onclick={(e) => {
          e.stopPropagation();
          hostnames.splice(i, 1);
        }}
      >
        <X class="w-3.5 h-3.5" />
      </button>
    </span>
  {/each}
  <input
    bind:this={input}
    bind:value={draft}
    {id}
    onkeydown={onKey}
    onpaste={onPaste}
    onblur={() => draft.trim() && commit()}
    class="flex-1 min-w-40 h-7 bg-transparent outline-none focus-visible:ring-0 focus-visible:ring-offset-0 font-mono text-[13px] placeholder:font-sans placeholder:text-muted-foreground/70"
    placeholder={hostnames.length ? "Add another" : (suggestion ?? "app.example.com")}
    autocomplete="off"
    spellcheck="false"
    aria-invalid={!!error}
    aria-describedby="{id}-error"
  />
</div>
{#if error}
  <p id="{id}-error" class="hint text-destructive">{error}</p>
{:else if suggestion && !hostnames.includes(suggestion)}
  <p class="hint">
    <button type="button" class="text-link hover:underline underline-offset-2" onclick={() => hostnames.push(suggestion!)}>
      Use {suggestion}
    </button>
  </p>
{/if}
