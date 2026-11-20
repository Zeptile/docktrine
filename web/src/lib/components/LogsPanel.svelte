<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import { api, ApiError } from "$lib/api";
  import Button from "./ui/Button.svelte";
  import { Pause, Play, ArrowDownToLine } from "lucide-svelte";

  export let app: string;
  export let env: string;

  // Keep the browser responsive on chatty apps
  const MAX_CHARS = 500_000;

  let text = "";
  let tail = 200;
  let follow = true;
  let streaming = false;
  let error: string | null = null;
  let box: HTMLPreElement;
  let controller: AbortController | null = null;
  // False once the reader scrolls up, so new lines don't yank them back down
  let pinned = true;

  onMount(() => {
    start();
  });
  onDestroy(() => controller?.abort());

  async function start() {
    controller?.abort();
    const ctrl = new AbortController();
    controller = ctrl;
    text = "";
    error = null;
    streaming = true;
    pinned = true;
    try {
      await api.apps.logs(app, env, { tail, follow, signal: ctrl.signal }, append);
    } catch (e) {
      if (!ctrl.signal.aborted) {
        error = e instanceof ApiError ? e.message : "The log stream ended unexpectedly";
      }
    } finally {
      if (controller === ctrl) streaming = false;
    }
  }

  function stop() {
    controller?.abort();
    streaming = false;
  }

  async function append(chunk: string) {
    text += chunk;
    if (text.length > MAX_CHARS) {
      text = text.slice(text.length - MAX_CHARS);
      text = text.slice(text.indexOf("\n") + 1);
    }
    if (pinned) {
      await tick();
      box.scrollTop = box.scrollHeight;
    }
  }

  function onScroll() {
    pinned = box.scrollHeight - box.scrollTop - box.clientHeight < 40;
  }

  function jumpToEnd() {
    box.scrollTop = box.scrollHeight;
    pinned = true;
  }
</script>

<div class="rounded-lg overflow-hidden flex flex-col terminal">
  <div class="font-sans px-4 py-2.5 border-b border-white/10 flex flex-wrap items-center justify-between gap-3">
    <div class="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm">
      <label class="flex items-center gap-2">
        Last
        <select bind:value={tail} onchange={start} class="select-on-iron h-8 rounded-md bg-white/10 border border-white/15 pl-2 pr-7 text-[13px] [&>option]:bg-[rgb(var(--terminal))]">
          <option value={100}>100</option>
          <option value={200}>200</option>
          <option value={1000}>1,000</option>
          <option value={5000}>5,000</option>
        </select>
        lines
      </label>
      <label class="flex items-center gap-2">
        <input type="checkbox" bind:checked={follow} onchange={start} class="w-4 h-4 accent-[rgb(var(--brass))]" />
        Follow new output
      </label>
      {#if streaming && follow}
        <span class="inline-flex items-center gap-2 text-[rgb(78_198_140)] font-medium">
          <span class="light light-live"></span> Live
        </span>
      {/if}
    </div>
    {#if streaming}
      <Button size="sm" variant="secondary" onclick={stop}>
        <Pause /> Pause
      </Button>
    {:else}
      <Button size="sm" variant="secondary" onclick={start}>
        <Play /> {follow ? "Resume" : "Reload"}
      </Button>
    {/if}
  </div>
  {#if error}
    <div class="font-sans px-4 py-2.5 text-sm text-[rgb(240_114_122)] border-b border-white/10">{error}</div>
  {/if}
  <div class="relative">
    <pre
      bind:this={box}
      onscroll={onScroll}
      class="p-5 overflow-auto h-[34rem] whitespace-pre-wrap break-all">{text || (streaming ? "Waiting for output…" : "No output.")}</pre>
    {#if !pinned}
      <button
        class="font-sans absolute bottom-4 right-6 h-8 px-3 rounded-md bg-brass text-brass-ink text-xs font-semibold shadow-lg inline-flex items-center gap-1.5"
        onclick={jumpToEnd}
      >
        <ArrowDownToLine class="w-3.5 h-3.5" /> Jump to latest
      </button>
    {/if}
  </div>
</div>
