<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import { api, ApiError } from "$lib/api";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import { Pause, Play } from "lucide-svelte";

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
    try {
      await api.apps.logs(app, env, { tail, follow, signal: ctrl.signal }, append);
    } catch (e) {
      if (!ctrl.signal.aborted) {
        error = e instanceof ApiError ? e.message : "Log stream ended unexpectedly";
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
    const atBottom =
      !box || box.scrollHeight - box.scrollTop - box.clientHeight < 40;
    text += chunk;
    if (text.length > MAX_CHARS) {
      text = text.slice(text.length - MAX_CHARS);
      text = text.slice(text.indexOf("\n") + 1);
    }
    if (atBottom) {
      await tick();
      box.scrollTop = box.scrollHeight;
    }
  }
</script>

<Card class="flex flex-col">
  <div class="px-4 py-3 border-b flex items-center justify-between gap-4">
    <div class="flex items-center gap-4 text-sm">
      <label class="flex items-center gap-2">
        Last
        <select
          bind:value={tail}
          onchange={start}
          class="px-2 py-1 border rounded-md bg-background"
        >
          <option value={100}>100</option>
          <option value={200}>200</option>
          <option value={1000}>1000</option>
          <option value={5000}>5000</option>
        </select>
        lines
      </label>
      <label class="flex items-center gap-2">
        <input type="checkbox" bind:checked={follow} onchange={start} class="w-4 h-4" />
        Follow
      </label>
      {#if streaming && follow}
        <span class="inline-flex items-center gap-1.5 text-green-700">
          <span class="w-1.5 h-1.5 rounded-full bg-current animate-pulse"></span> Live
        </span>
      {/if}
    </div>
    {#if streaming}
      <Button size="sm" variant="outline" onclick={stop}>
        <Pause class="w-4 h-4 mr-2" /> Stop
      </Button>
    {:else}
      <Button size="sm" variant="outline" onclick={start}>
        <Play class="w-4 h-4 mr-2" /> {follow ? "Resume" : "Reload"}
      </Button>
    {/if}
  </div>
  {#if error}
    <div class="px-4 py-2 text-sm text-destructive border-b">{error}</div>
  {/if}
  <pre
    bind:this={box}
    class="p-4 text-xs font-mono bg-muted/50 overflow-auto h-[32rem] whitespace-pre-wrap break-all">{text || (streaming ? "Waiting for output..." : "No output.")}</pre>
</Card>
