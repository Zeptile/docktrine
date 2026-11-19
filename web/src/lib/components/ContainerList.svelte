<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Container, type Server, ApiError } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import { toast } from "$lib/toast";
  import { confirmAction } from "$lib/confirm";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import { Play, Square, RotateCcw, RefreshCw, Search } from "lucide-svelte";

  let containers: Container[] = [];
  let servers: Server[] = [];
  let selectedServer = "";
  let loading = false;
  let loaded = false;
  let error: string | null = null;
  let filter = "";
  let busy: Record<string, boolean> = {};

  $: query = filter.trim().toLowerCase();
  $: visible = containers
    .filter((c) => !query || name(c).toLowerCase().includes(query) || c.image.toLowerCase().includes(query))
    .sort((a, b) => Number(b.status === "running") - Number(a.status === "running") || name(a).localeCompare(name(b)));
  $: runningCount = containers.filter((c) => c.status === "running").length;

  onMount(async () => {
    try {
      servers = await api.servers.list();
      selectedServer = servers.find((s) => s.is_default)?.name ?? servers[0]?.name ?? "";
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load servers";
    }
    await loadContainers();
  });

  async function loadContainers() {
    try {
      loading = true;
      error = null;
      containers = await api.containers.list(selectedServer || undefined);
      loaded = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load containers";
    } finally {
      loading = false;
    }
  }

  function changeServer() {
    loaded = false;
    containers = [];
    loadContainers();
  }

  async function act(c: Container, verb: string, action: () => Promise<unknown>) {
    busy[c.id] = true;
    error = null;
    try {
      await action();
      toast(`${name(c)} ${verb}`);
      await loadContainers();
    } catch (e) {
      error = e instanceof ApiError ? e.message : `Couldn't change ${name(c)}`;
    } finally {
      busy[c.id] = false;
    }
  }

  async function stop(c: Container) {
    const app = c.labels?.["docktrine.app"];
    const ok = await confirmAction({
      title: `Stop ${name(c)}?`,
      message: app
        ? `It belongs to ${app}/${c.labels?.["docktrine.env"]}, whose hostnames stop answering until it's started again or redeployed.`
        : "Anything it serves stops answering until it's started again.",
      confirmLabel: "Stop container",
      danger: true,
    });
    if (ok) act(c, "stopped", () => api.containers.stop(c.id, selectedServer || undefined));
  }

  function name(c: Container) {
    return c.name.replace(/^\//, "") || c.id.substring(0, 12);
  }

  // What happened last, in Docker's terms but readable
  function describe(c: Container) {
    const s = c.state;
    const health = s.Health?.Status;
    if (c.status === "running") {
      return `up since ${timeAgo(s.StartedAt)}${health && health !== "none" ? `, ${health}` : ""}`;
    }
    if (c.status === "exited") {
      return s.ExitCode === 0 ? `stopped ${timeAgo(s.FinishedAt)}` : `exited (code ${s.ExitCode}) ${timeAgo(s.FinishedAt)}`;
    }
    return c.status;
  }

  function tone(c: Container) {
    const state = c.status;
    if (state === "running") return "text-success";
    if (state === "exited" && c.state.ExitCode === 0) return "text-muted-foreground";
    if (state === "exited" || state === "dead") return "text-destructive";
    if (state === "paused" || state === "restarting") return "text-warning";
    return "text-muted-foreground";
  }

  function ports(c: Container) {
    const seen = new Set<string>();
    for (const p of c.ports ?? []) {
      seen.add(p.PublicPort ? `${p.PublicPort}→${p.PrivatePort}` : `${p.PrivatePort}`);
    }
    return [...seen].join(", ");
  }
</script>

<div class="space-y-8">
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="stencil text-5xl">Containers</h1>
      <p class="text-muted-foreground mt-2">
        Every container on a node, including ones Docktrine didn't start.
        {#if loaded}{runningCount} of {containers.length} running.{/if}
      </p>
    </div>
    <div class="flex flex-wrap items-center gap-2 w-full sm:w-auto">
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={changeServer} class="input w-auto" aria-label="Node">
          {#each servers as server}
            <option value={server.name}>{server.name}</option>
          {/each}
        </select>
      {/if}
      <div class="relative flex-1 sm:w-56">
        <Search class="w-4 h-4 absolute left-3 top-3 text-muted-foreground pointer-events-none" />
        <input bind:value={filter} type="search" placeholder="Filter by name or image" aria-label="Filter containers" class="input pl-9" />
      </div>
      <Button variant="outline" size="icon" onclick={loadContainers} disabled={loading} title="Refresh" aria-label="Refresh">
        <RefreshCw class={loading ? "animate-spin" : ""} />
      </Button>
    </div>
  </div>

  {#if error}
    <Alert>{error}</Alert>
  {/if}

  {#if !loaded && loading}
    <Loading label="Asking {selectedServer || 'the node'} for its containers" />
  {:else if loaded && containers.length === 0}
    <Card class="p-10 text-center text-muted-foreground">No containers on {selectedServer}.</Card>
  {:else if loaded}
    <Card class="overflow-hidden">
      {#if visible.length === 0}
        <p class="p-8 text-center text-muted-foreground">Nothing matches “{filter}”.</p>
      {/if}
      <ul class="divide-y">
        {#each visible as c (c.id)}
          {@const app = c.labels?.["docktrine.app"]}
          <li class="px-5 py-3.5 flex flex-wrap sm:flex-nowrap items-center gap-x-5 gap-y-2">
            <span class="w-2.5 h-2.5 rounded-full bg-current shrink-0 {tone(c)}" title={c.status}></span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 min-w-0">
                <span class="font-semibold truncate">{name(c)}</span>
                {#if app}
                  <a
                    href="#/apps/{encodeURIComponent(app)}/{encodeURIComponent(c.labels?.['docktrine.env'] ?? '')}"
                    class="text-xs px-1.5 h-5 inline-flex items-center rounded-sm bg-signal/20 text-foreground hover:bg-signal/40 shrink-0"
                    title="Deployed by Docktrine"
                  >
                    {app}/{c.labels?.["docktrine.env"]}
                  </a>
                {/if}
              </div>
              <div class="text-xs text-muted-foreground font-mono truncate mt-0.5" title={c.image}>{c.image}</div>
            </div>
            <div class="text-sm text-muted-foreground sm:w-56 sm:text-right">
              <div class={tone(c)}>{describe(c)}</div>
              {#if ports(c)}
                <div class="text-xs font-mono">{ports(c)}</div>
              {/if}
            </div>
            <div class="flex items-center gap-1.5 shrink-0 ml-auto sm:ml-0">
              {#if c.status === "running"}
                <Button size="sm" variant="outline" disabled={busy[c.id]} onclick={() => act(c, "restarted", () => api.containers.restart(c.id, selectedServer || undefined))}>
                  <RotateCcw /> Restart
                </Button>
                <Button size="sm" variant="outline" disabled={busy[c.id]} onclick={() => stop(c)}>
                  <Square /> Stop
                </Button>
              {:else}
                <Button size="sm" variant="outline" disabled={busy[c.id]} onclick={() => act(c, "started", () => api.containers.start(c.id, selectedServer || undefined))}>
                  <Play /> Start
                </Button>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    </Card>
  {/if}
</div>
