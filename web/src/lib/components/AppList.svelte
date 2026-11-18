<script lang="ts">
  import { onMount } from "svelte";
  import { api, type AppSummary, ApiError } from "$lib/api";
  import { timeAgo, timeUntil, shortImage } from "$lib/format";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import StatusBadge from "./ui/StatusBadge.svelte";
  import { Boxes, ChevronRight, GitBranch, Clock } from "lucide-svelte";

  let items: AppSummary[] = [];
  let loading = false;
  let loaded = false;
  let error: string | null = null;

  // Grouped by app, with prod-like environments before previews
  $: groups = Object.entries(
    items.reduce<Record<string, AppSummary[]>>((acc, item) => {
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

  $: busy = items.some(
    (i) =>
      i.latest_deployment?.status === "running" ||
      i.latest_deployment?.status === "pending"
  );

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
      error = e instanceof ApiError ? e.message : "Failed to load apps";
    } finally {
      loading = false;
    }
  }
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h2 class="text-xl font-semibold">Apps</h2>
    <Button onclick={() => load()} disabled={loading} variant="outline">
      {loading ? "Loading..." : "Refresh"}
    </Button>
  </div>

  {#if error}
    <div class="p-4 bg-destructive/10 border border-destructive/20 rounded-md">
      <p class="text-destructive text-sm">{error}</p>
    </div>
  {/if}

  {#if !loaded && loading}
    <div class="flex items-center justify-center py-8">
      <div
        class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"
      ></div>
    </div>
  {:else if loaded && groups.length === 0}
    <div class="text-center py-8 text-muted-foreground">
      <Boxes class="w-12 h-12 mx-auto mb-4 opacity-50" />
      <p>No apps deployed yet</p>
      <p class="text-sm">
        Run <code class="px-1 rounded bg-muted">docktrine init</code> in a repo,
        then <code class="px-1 rounded bg-muted">docktrine deploy dev</code>
      </p>
    </div>
  {:else}
    <div class="grid gap-4">
      {#each groups as group (group.app)}
        <Card>
          <div class="px-6 py-4 border-b flex items-center gap-2">
            <Boxes class="w-5 h-5" />
            <h3 class="font-semibold">{group.app}</h3>
            <span class="text-sm text-muted-foreground">
              {group.envs.length}
              {group.envs.length === 1 ? "environment" : "environments"}
            </span>
          </div>
          <ul class="divide-y">
            {#each group.envs as item (item.environment.id)}
              {@const env = item.environment}
              {@const latest = item.latest_deployment}
              {@const current = item.current_deployment}
              <li>
                <a
                  href="#/apps/{encodeURIComponent(env.app)}/{encodeURIComponent(env.name)}"
                  class="px-6 py-3 flex items-center gap-4 hover:bg-accent transition-colors"
                >
                  <div class="w-56 min-w-0">
                    <div class="font-medium flex items-center gap-1.5 truncate">
                      {#if env.is_preview}
                        <GitBranch class="w-4 h-4 shrink-0 text-muted-foreground" />
                      {/if}
                      <span class="truncate">{env.name}</span>
                    </div>
                    <div class="text-xs text-muted-foreground">on {env.server}</div>
                  </div>

                  <div class="w-28">
                    {#if latest}
                      <StatusBadge status={latest.status} />
                    {/if}
                  </div>

                  <div class="flex-1 min-w-0 text-sm text-muted-foreground truncate">
                    {#if current}
                      <span class="font-mono text-foreground" title={current.image}
                        >#{current.id} {shortImage(current.image)}</span
                      >
                    {:else}
                      Nothing running
                    {/if}
                    {#if latest && latest.status === "failed" && current && latest.id !== current.id}
                      <span class="text-red-600"> · #{latest.id} failed</span>
                    {/if}
                  </div>

                  <div class="w-32 text-right text-sm text-muted-foreground">
                    {#if env.expires_at}
                      <span class="inline-flex items-center gap-1" title="Preview TTL">
                        <Clock class="w-3.5 h-3.5" />
                        {timeUntil(env.expires_at)}
                      </span>
                    {:else if latest}
                      {timeAgo(latest.created_at)}
                    {/if}
                  </div>
                  <ChevronRight class="w-4 h-4 text-muted-foreground" />
                </a>
              </li>
            {/each}
          </ul>
        </Card>
      {/each}
    </div>
  {/if}
</div>
