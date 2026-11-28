<script lang="ts">
  import { untrack } from "svelte";
  import { api, ApiError, type Check, type PreflightRequest } from "$lib/api";
  import { CircleCheck, CircleX, AlertTriangle, Info, LoaderCircle } from "lucide-svelte";

  interface Props {
    // Null until the form knows where it runs
    request: PreflightRequest | null;
    // Read by the form, to hold the launch on a failed check
    checks?: Check[];
    checking?: boolean;
  }

  let { request, checks = $bindable([]), checking = $bindable(false) }: Props = $props();

  let error = $state<string | null>(null);
  // The node the shown checks are for
  let checkedNode = $state("");

  // Re-check when what's sent changes, a moment after typing stops; an
  // answer to an older request is dropped
  let key = $derived(request ? JSON.stringify(request) : "");
  let controller: AbortController | undefined;
  let sentKey = "";

  $effect(() => {
    const k = key;
    // The request is rebuilt on every keystroke; only its content matters
    const body = untrack(() => request);
    if (!body) {
      controller?.abort();
      sentKey = "";
      checks = [];
      checking = false;
      return;
    }
    if (untrack(() => k === sentKey && !error)) return;
    sentKey = k;
    checking = true;
    const timer = setTimeout(() => run(body), 500);
    return () => clearTimeout(timer);
  });

  async function run(body: PreflightRequest) {
    controller?.abort();
    const mine = (controller = new AbortController());
    try {
      const res = await api.launch.preflight(body, mine.signal);
      if (mine.signal.aborted) return;
      checks = res.checks;
      checkedNode = body.node;
      error = null;
    } catch (e) {
      if (mine.signal.aborted) return;
      checks = [];
      error = e instanceof ApiError ? e.message : "Couldn't run the checks";
    }
    checking = false;
  }

  const icons = {
    ok: { Icon: CircleCheck, class: "text-success" },
    info: { Icon: Info, class: "text-muted-foreground" },
    warning: { Icon: AlertTriangle, class: "text-warning" },
    failed: { Icon: CircleX, class: "text-destructive" },
  };
</script>

{#if request}
  <div class="mt-5 pt-5 border-t border-border">
    <h3 class="text-[13px] text-muted-foreground flex items-center gap-1.5">
      Checks on {checkedNode || request.node}
      {#if checking}<LoaderCircle class="w-3.5 h-3.5 animate-spin" aria-label="Checking" />{/if}
    </h3>
    {#if error}
      <p class="hint text-warning">{error}</p>
    {:else if !checks.length && checking}
      <p class="hint">Looking at {request.node}…</p>
    {:else}
      <ul class="mt-2 space-y-2.5 text-sm transition-opacity {checking ? 'opacity-60' : ''}" aria-live="polite">
        {#each checks as c (c.id)}
          {@const s = icons[c.status]}
          <li class="flex gap-2">
            <s.Icon class="w-4 h-4 mt-0.5 shrink-0 {s.class}" />
            <div class="min-w-0">
              <p class={c.status === "failed" ? "text-destructive font-medium" : ""}>{c.title}</p>
              {#if c.detail && c.status !== "ok"}
                <p class="text-[13px] text-muted-foreground mt-0.5 [overflow-wrap:anywhere]">
                  {c.detail}
                  {#if c.id === "ingress"}<a href="#/servers" class="text-link hover:underline underline-offset-2">Servers</a>{/if}
                </p>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
