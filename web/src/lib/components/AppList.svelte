<script lang="ts">
  import { onMount } from "svelte";
  import { api, type AppSummary, ApiError } from "$lib/api";
  import { timeAgo, timeUntil, shortImage } from "$lib/format";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import StatusBadge from "./ui/StatusBadge.svelte";
  import { ChevronRight, GitBranch, Clock, RefreshCw, Search } from "lucide-svelte";

  let items: AppSummary[] = [];
  let loading = false;
  let loaded = false;
  let error: string | null = null;
  let filter = "";

  $: query = filter.trim().toLowerCase();
  $: visible = query
    ? items.filter(
        (i) =>
          i.environment.app.includes(query) ||
          i.environment.name.includes(query) ||
          i.environment.server.toLowerCase().includes(query)
      )
    : items;

  // Grouped by app, with prod-like environments before previews
  $: groups = Object.entries(
    visible.reduce<Record<string, AppSummary[]>>((acc, item) => {
      (acc[item.environment.app] ??= []).push(item);
      return acc;
    }, {})
  )
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([app, envs]) => ({
      app,
      envs: envs.sort(
        (a, b) =>
          Number(a.environment.is_preview) - Number(b.environment.is_preview) ||
          a.environment.name.localeCompare(b.environment.name)
      ),
    }));

  $: appCount = new Set(items.map((i) => i.environment.app)).size;
  $: deploying = items.filter((i) => isActive(i)).length;
  $: failing = items.filter((i) => lastFailed(i)).length;
  $: busy = deploying > 0;

  function isActive(i: AppSummary) {
    const s = i.latest_deployment?.status;
    return s === "running" || s === "pending";
  }

  function lastFailed(i: AppSummary) {
    return i.latest_deployment?.status === "failed";
  }

  // The strip down the left of each environment: what state it's in at a glance
  function stripClass(i: AppSummary) {
    if (isActive(i)) return "text-info corrugated-moving";
    if (lastFailed(i)) return "text-destructive";
    if (i.current_deployment) return "text-success";
    return "text-border";
  }

  onMount(() => {
    load();
    // Poll quickly while something deploys, slowly otherwise
    let timer: ReturnType<typeof setTimeout>;
    const tick = () => {
      timer = setTimeout(async () => {
        if (!document.hidden) await load(true);
        tick();
      }, busy ? 2000 : 15000);
    };
    tick();
    return () => clearTimeout(timer);
  });

  async function load(quiet = false) {
    try {
      if (!quiet) loading = true;
      error = null;
      items = await api.apps.list();
      loaded = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load apps";
    } finally {
      loading = false;
    }
  }
</script>

<div class="space-y-8">
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="stencil text-5xl">Apps</h1>
      {#if loaded && items.length > 0}
        <p class="text-muted-foreground mt-2">
          {appCount}
          {appCount === 1 ? "app" : "apps"} in {items.length}
          {items.length === 1 ? "environment" : "environments"}{#if deploying}, <span class="text-info font-medium">{deploying} deploying</span>{/if}{#if failing}, <span class="text-destructive font-medium">{failing} with a failed last deploy</span>{/if}.
        </p>
      {/if}
    </div>
    <div class="flex items-center gap-2 w-full sm:w-auto">
      {#if items.length > 4}
        <div class="relative flex-1 sm:w-64">
          <Search class="w-4 h-4 absolute left-3 top-3 text-muted-foreground pointer-events-none" />
          <input
            bind:value={filter}
            type="search"
            placeholder="Filter apps"
            aria-label="Filter apps"
            class="input pl-9"
          />
        </div>
      {/if}
      <Button variant="outline" size="icon" onclick={() => load()} disabled={loading} title="Refresh" aria-label="Refresh">
        <RefreshCw class={loading ? "animate-spin" : ""} />
      </Button>
    </div>
  </div>

  {#if error}
    <Alert>{error}</Alert>
  {/if}

  {#if !loaded && loading}
    <Loading label="Loading apps" />
  {:else if loaded && items.length === 0}
    <Card class="px-6 py-14 text-center">
      <div class="flex justify-center gap-1 mb-6" aria-hidden="true">
        <span class="corrugated-x w-10 h-7 rounded-[2px] text-border"></span>
        <span class="w-10 h-7 rounded-[2px] border-2 border-dashed border-border"></span>
        <span class="corrugated-x w-10 h-7 rounded-[2px] text-border"></span>
      </div>
      <h2 class="text-lg font-semibold">No apps deployed yet</h2>
      <p class="text-muted-foreground mt-2 max-w-md mx-auto">
        In a repository, run <code>docktrine init</code> to write a
        <code>docktrine.yaml</code>, then <code>docktrine deploy dev</code>.
      </p>
    </Card>
  {:else if groups.length === 0}
    <p class="text-muted-foreground py-8 text-center">Nothing matches “{filter}”.</p>
  {:else}
    <div class="space-y-8">
      {#each groups as group (group.app)}
        <section>
          <h2 class="stencil text-[1.75rem] mb-3">{group.app}</h2>
          <Card class="overflow-hidden">
            <ul class="divide-y">
              {#each group.envs as item (item.environment.id)}
                {@const env = item.environment}
                {@const latest = item.latest_deployment}
                {@const current = item.current_deployment}
                <li>
                  <a
                    href="#/apps/{encodeURIComponent(env.app)}/{encodeURIComponent(env.name)}"
                    class="group flex items-stretch hover:bg-muted/60 transition-colors focus-visible:ring-inset focus-visible:ring-offset-0"
                  >
                    <span class="corrugated w-2 shrink-0 {stripClass(item)}" aria-hidden="true"></span>
                    <div class="flex-1 min-w-0 px-4 sm:px-5 py-3.5 flex flex-wrap sm:flex-nowrap items-center gap-x-6 gap-y-1">
                      <div class="min-w-0 flex-1 sm:flex-none sm:w-56">
                        <div class="font-semibold flex items-center gap-1.5">
                          {#if env.is_preview}
                            <GitBranch class="w-4 h-4 shrink-0 text-muted-foreground" />
                          {/if}
                          <span class="truncate">{env.name}</span>
                        </div>
                        <div class="text-xs text-muted-foreground">on {env.server}</div>
                      </div>

                      <div class="min-w-0 text-sm order-last sm:order-none w-full sm:w-auto sm:flex-1">
                        {#if current}
                          <span class="font-mono text-[13px] truncate block" title={current.image}>
                            #{current.id} {shortImage(current.image)}
                          </span>
                        {:else if !latest || latest.status === "failed"}
                          <span class="text-muted-foreground">Nothing running</span>
                        {/if}
                        {#if latest && latest.id !== current?.id && latest.status !== "succeeded"}
                          <span class="flex items-center gap-2 mt-0.5">
                            <StatusBadge status={latest.status} />
                            <span class="text-xs text-muted-foreground font-mono truncate">#{latest.id} {shortImage(latest.image)}</span>
                          </span>
                        {/if}
                      </div>

                      <div class="flex items-center gap-3 text-sm text-muted-foreground shrink-0">
                        {#if env.expires_at}
                          <span class="inline-flex items-center gap-1" title="Removed in {timeUntil(env.expires_at)} unless redeployed">
                            <Clock class="w-3.5 h-3.5" />
                            {timeUntil(env.expires_at)}
                          </span>
                        {:else if latest}
                          <span title={new Date(latest.created_at).toLocaleString()}>{timeAgo(latest.created_at)}</span>
                        {/if}
                        <ChevronRight class="w-4 h-4 transition-transform group-hover:translate-x-0.5" />
                      </div>
                    </div>
                  </a>
                </li>
              {/each}
            </ul>
          </Card>
        </section>
      {/each}
    </div>
  {/if}
</div>
