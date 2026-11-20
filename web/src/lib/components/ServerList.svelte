<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Server, type ServerHealth, ApiError } from "$lib/api";
  import { bytes } from "$lib/format";
  import { confirmAction } from "$lib/confirm";
  import { toast } from "$lib/toast";
  import { can } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import { Plus, Trash2, Activity, Network, Eraser, Pencil, RefreshCw } from "lucide-svelte";

  type Panel = "edit" | "ingress" | "prune" | null;

  let servers: Server[] = [];
  let loading = false;
  let loaded = false;
  let error: string | null = null;
  let showCreateForm = false;
  let newServer: Server = blankServer();

  // Per-server UI state, keyed by name
  let health: Record<string, ServerHealth | "loading"> = {};
  let panel: Record<string, Panel> = {};
  let busy: Record<string, boolean> = {};
  let edits: Record<string, Server> = {};
  let ingress: Record<string, { port: number; bind: string }> = {};
  let pruneAll: Record<string, boolean> = {};

  onMount(async () => {
    await loadServers();
  });

  function blankServer(): Server {
    return { name: "", host: "", description: "", ingress_host: "", ingress_port: undefined };
  }

  async function loadServers() {
    try {
      loading = true;
      error = null;
      servers = await api.servers.list();
      loaded = true;
      servers.forEach((s) => checkHealth(s.name));
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load servers";
    } finally {
      loading = false;
    }
  }

  async function checkHealth(name: string) {
    health[name] = "loading";
    try {
      health[name] = await api.servers.health(name);
    } catch (e) {
      health[name] = {
        reachable: false,
        error: e instanceof ApiError ? e.message : "Health check failed",
      };
    }
  }

  async function createServer() {
    error = null;
    try {
      await api.servers.create({
        ...newServer,
        ingress_port: newServer.ingress_port ? Number(newServer.ingress_port) : undefined,
      });
      toast(`${newServer.name} added`);
      showCreateForm = false;
      newServer = blankServer();
      await loadServers();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't add the server";
    }
  }

  async function deleteServer(name: string) {
    const ok = await confirmAction({
      title: `Remove ${name}?`,
      message: `Docktrine forgets ${name}. Nothing on the node itself is stopped or deleted, and you can add it back later.`,
      confirmLabel: "Remove server",
      danger: true,
    });
    if (!ok) return;
    error = null;
    try {
      await api.servers.delete(name);
      toast(`${name} removed`);
      await loadServers();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't remove the server";
    }
  }

  function toggle(server: Server, which: Panel) {
    const name = server.name;
    panel[name] = panel[name] === which ? null : which;
    if (which === "edit") edits[name] = { ...server };
    if (which === "ingress") {
      ingress[name] ??= {
        port: server.ingress_port || 8880,
        // Nodes reached through an SSH tunnel only need the ingress on loopback
        bind: server.ingress_host === "127.0.0.1" || server.ingress_host === "localhost" ? "127.0.0.1" : "",
      };
    }
  }

  async function run(name: string, action: () => Promise<string>) {
    busy[name] = true;
    error = null;
    try {
      toast(await action());
      panel[name] = null;
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Request failed";
    } finally {
      busy[name] = false;
    }
  }

  function saveEdit(name: string) {
    const s = edits[name];
    run(name, async () => {
      await api.servers.update(name, {
        host: s.host,
        description: s.description,
        ingress_host: s.ingress_host,
        ingress_port: s.ingress_port ? Number(s.ingress_port) : 0,
      });
      await loadServers();
      return `${name} saved`;
    });
  }

  function installIngress(name: string) {
    const opts = ingress[name];
    run(name, async () => {
      const res = await api.servers.installIngress(name, {
        port: Number(opts.port),
        bind: opts.bind || undefined,
      });
      await checkHealth(name);
      return res.message;
    });
  }

  async function prune(name: string) {
    const all = !!pruneAll[name];
    if (all) {
      const ok = await confirmAction({
        title: `Remove every unused image on ${name}?`,
        message: "Includes images Docktrine didn't create. They'll have to be pulled or rebuilt to be used again.",
        confirmLabel: "Remove images",
        danger: true,
      });
      if (!ok) return;
    }
    run(name, async () => {
      const res = await api.servers.prune(name, all);
      await checkHealth(name);
      return `Removed ${res.images_deleted} image(s) from ${name}, freeing ${bytes(res.space_reclaimed)}`;
    });
  }

  function healthTone(h: ServerHealth | "loading" | undefined) {
    if (!h || h === "loading") return "text-muted-foreground";
    return h.reachable ? "text-success" : "text-destructive";
  }

  function healthLight(h: ServerHealth | "loading" | undefined) {
    if (!h || h === "loading") return "light-deploying";
    return h.reachable ? "light-live" : "light-failed";
  }
</script>

<div class="space-y-8">
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="page-title">Servers</h1>
      <p class="text-muted-foreground mt-1.5">The nodes apps deploy to, reached over SSH.</p>
    </div>
    <div class="flex items-center gap-2">
      <Button variant="ghost" size="icon" onclick={loadServers} disabled={loading} title="Refresh" aria-label="Refresh">
        <RefreshCw class={loading ? "animate-spin" : ""} />
      </Button>
      {#if !showCreateForm && $can("admin")}
        <Button onclick={() => (showCreateForm = true)}>
          <Plus /> Add server
        </Button>
      {/if}
    </div>
  </div>

  {#if error}
    <Alert>{error}</Alert>
  {/if}

  {#if showCreateForm}
    <div class="rounded-lg bg-plate/70">
      <form
        onsubmit={(e) => {
          e.preventDefault();
          createServer();
        }}
        class="p-6 flex-1 min-w-0 space-y-5"
      >
        <h2 class="text-lg font-semibold">Add a server</h2>
        <div class="grid sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-4">
          <div>
            <label for="name" class="label">Name</label>
            <input id="name" bind:value={newServer.name} required class="input" placeholder="prd1-tor" autocomplete="off" />
            <p class="hint">What docktrine.yaml calls it in <code>node:</code></p>
          </div>
          <div>
            <label for="host" class="label">Docker host</label>
            <input id="host" bind:value={newServer.host} required class="input font-mono" placeholder="ssh://deploy@prd1-tor.zeptile.software" autocomplete="off" />
          </div>
        </div>

        <div>
          <label for="description" class="label">Description <span class="text-muted-foreground font-normal">(optional)</span></label>
          <input id="description" bind:value={newServer.description} class="input" />
        </div>

        <fieldset>
          <legend class="label">Where the gateway reaches this node's ingress</legend>
          <div class="grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)] gap-4">
            <input bind:value={newServer.ingress_host} aria-label="Ingress host" class="input font-mono" placeholder="127.0.0.1" />
            <input bind:value={newServer.ingress_port} aria-label="Ingress port" type="number" class="input font-mono" placeholder="8880" />
          </div>
          <p class="hint">
            Nginx Proxy Manager forwards this node's hostnames here, e.g. the gateway end of an
            SSH tunnel. Hostnames don't work without it.
          </p>
        </fieldset>

        <div class="flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onclick={() => {
              showCreateForm = false;
              newServer = blankServer();
            }}
          >
            Cancel
          </Button>
          <Button type="submit">Add server</Button>
        </div>
      </form>
    </div>
  {/if}

  {#if !loaded && loading}
    <Loading label="Loading servers" />
  {:else if loaded && servers.length === 0 && !showCreateForm}
    <div>
      <p class="text-muted-foreground mb-4">No servers yet. Add the first node apps will deploy to.</p>
      {#if $can("admin")}<Button onclick={() => (showCreateForm = true)}><Plus /> Add server</Button>{/if}
    </div>
  {:else}
    <div class="space-y-6">
      {#each servers as server (server.id)}
        {@const h = health[server.name]}
        <section class="riveted px-6 sm:px-8 pb-6">
          <div class="pt-5">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span class="light {healthLight(h)}"></span>
                  <h2 class="engraved text-[1.4rem] leading-none">{server.name}</h2>
                  {#if server.is_default}
                    <span class="text-xs px-1.5 h-5 inline-flex items-center rounded-sm border text-muted-foreground" title="Used when docktrine.yaml doesn't name a node">default</span>
                  {/if}
                  <span class="text-sm {healthTone(h)}">
                    {#if h === "loading" || !h}checking…{:else if h.reachable}online{:else}unreachable{/if}
                  </span>
                </div>
                {#if server.description}
                  <p class="text-sm text-muted-foreground mt-1">{server.description}</p>
                {/if}
              </div>

              <div class="flex flex-wrap items-center gap-1.5">
                <Button size="sm" variant="ghost" onclick={() => checkHealth(server.name)} title="Check again" aria-label="Check {server.name} again">
                  <Activity />
                </Button>
                {#if $can("admin")}
                <Button size="sm" variant={panel[server.name] === "edit" ? "secondary" : "outline"} onclick={() => toggle(server, "edit")} aria-expanded={panel[server.name] === "edit"}>
                  <Pencil /> Edit
                </Button>
                <Button size="sm" variant={panel[server.name] === "ingress" ? "secondary" : "outline"} onclick={() => toggle(server, "ingress")} aria-expanded={panel[server.name] === "ingress"}>
                  <Network /> Ingress
                </Button>
                <Button size="sm" variant={panel[server.name] === "prune" ? "secondary" : "outline"} onclick={() => toggle(server, "prune")} aria-expanded={panel[server.name] === "prune"}>
                  <Eraser /> Prune
                </Button>
                {/if}
              </div>
            </div>

            <dl class="mt-5 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-4 text-sm">
              <div class="min-w-0 sm:col-span-2">
                <dt class="text-[13px] text-muted-foreground">Docker host</dt>
                <dd class="font-mono text-[13px] truncate mt-0.5" title={server.host}>{server.host}</dd>
              </div>
              <div class="min-w-0 sm:col-span-2">
                <dt class="text-[13px] text-muted-foreground">Ingress</dt>
                <dd class="mt-0.5">
                  {#if h && h !== "loading" && h.reachable}
                    <span class={h.ingress === "running" ? "text-success" : "text-warning"}>{h.ingress === "running" ? "Running" : h.ingress}</span>,
                  {/if}
                  {#if server.ingress_host && server.ingress_port}
                    gateway forwards to <span class="font-mono text-[13px]">{server.ingress_host}:{server.ingress_port}</span>
                  {:else}
                    <span class="text-warning">no gateway target set, so hostnames won't work</span>
                  {/if}
                </dd>
              </div>
              {#if h && h !== "loading" && h.reachable}
                <div>
                  <dt class="text-[13px] text-muted-foreground">Docker</dt>
                  <dd class="mt-0.5">{h.server_version} on {h.os}</dd>
                </div>
                <div>
                  <dt class="text-[13px] text-muted-foreground">Capacity</dt>
                  <dd class="mt-0.5">{h.cpus} CPUs, {bytes(h.memory_bytes)}</dd>
                </div>
                <div>
                  <dt class="text-[13px] text-muted-foreground">Containers</dt>
                  <dd class="mt-0.5">{h.containers_running} running of {h.containers}</dd>
                </div>
                <div>
                  <dt class="text-[13px] text-muted-foreground">Images</dt>
                  <dd class="mt-0.5">{h.images}</dd>
                </div>
              {:else if h && h !== "loading"}
                <div class="sm:col-span-2 lg:col-span-4">
                  <dt class="text-[13px] text-muted-foreground">Error</dt>
                  <dd class="mt-0.5 text-destructive break-words">{h.error}</dd>
                </div>
              {/if}
            </dl>
          </div>

          {#if panel[server.name] === "edit" && edits[server.name]}
            <form
              class="mt-5 rounded-lg bg-plate/70 p-5 sm:p-6 grid sm:grid-cols-2 gap-4"
              onsubmit={(e) => {
                e.preventDefault();
                saveEdit(server.name);
              }}
            >
              <label class="text-sm">
                <span class="label">Docker host</span>
                <input bind:value={edits[server.name].host} required class="input font-mono" />
              </label>
              <label class="text-sm">
                <span class="label">Description</span>
                <input bind:value={edits[server.name].description} class="input" />
              </label>
              <label class="text-sm">
                <span class="label">Gateway ingress host</span>
                <input bind:value={edits[server.name].ingress_host} class="input font-mono" placeholder="127.0.0.1" />
              </label>
              <label class="text-sm">
                <span class="label">Gateway ingress port</span>
                <input bind:value={edits[server.name].ingress_port} type="number" class="input font-mono" placeholder="8880" />
              </label>
              <p class="sm:col-span-2 hint mt-0">
                A new gateway target applies to hostnames as their apps are next deployed.
              </p>
              <div class="sm:col-span-2 flex flex-wrap justify-between gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  class="text-destructive hover:text-destructive hover:bg-destructive/10"
                  onclick={() => deleteServer(server.name)}
                >
                  <Trash2 /> Remove server
                </Button>
                <div class="flex gap-2">
                  <Button type="button" variant="outline" onclick={() => (panel[server.name] = null)}>Cancel</Button>
                  <Button type="submit" disabled={busy[server.name]}>Save changes</Button>
                </div>
              </div>
            </form>
          {:else if panel[server.name] === "ingress" && ingress[server.name]}
            <form
              class="mt-5 rounded-lg bg-plate/70 p-5 sm:p-6 space-y-4"
              onsubmit={(e) => {
                e.preventDefault();
                installIngress(server.name);
              }}
            >
              <p class="text-sm text-muted-foreground max-w-2xl">
                Installs or replaces the Traefik container on {server.name} that routes requests
                to app containers by hostname. Apps keep running, but their hostnames stop
                answering for about a second while it restarts.
              </p>
              <div class="grid sm:grid-cols-2 gap-4">
                <label class="text-sm">
                  <span class="label">Port on the node</span>
                  <input bind:value={ingress[server.name].port} type="number" required min="1" max="65535" class="input font-mono" />
                </label>
                <label class="text-sm">
                  <span class="label">Bind address</span>
                  <input bind:value={ingress[server.name].bind} class="input font-mono" placeholder="All interfaces" />
                  <span class="hint block">Use 127.0.0.1 when the gateway reaches the node through an SSH tunnel.</span>
                </label>
              </div>
              <div class="flex justify-end gap-2">
                <Button type="button" variant="outline" onclick={() => (panel[server.name] = null)}>Cancel</Button>
                <Button type="submit" disabled={busy[server.name]}>
                  {busy[server.name] ? "Installing…" : "Install ingress"}
                </Button>
              </div>
            </form>
          {:else if panel[server.name] === "prune"}
            <div class="mt-5 rounded-lg bg-plate/70 p-5 sm:p-6 space-y-4">
              <p class="text-sm text-muted-foreground max-w-2xl">
                Removes dangling images: old layers that no tag points to anymore, left behind by
                deploys.
              </p>
              <label class="flex items-start gap-2.5 text-sm">
                <input type="checkbox" bind:checked={pruneAll[server.name]} class="w-4 h-4 mt-0.5 accent-[rgb(var(--danger))]" />
                <span>
                  Also remove every image no container uses
                  <span class="block hint mt-0.5 text-warning">
                    Includes images that aren't Docktrine's. They'll have to be pulled or rebuilt to be used again.
                  </span>
                </span>
              </label>
              <div class="flex justify-end gap-2">
                <Button variant="outline" onclick={() => (panel[server.name] = null)}>Cancel</Button>
                <Button
                  variant={pruneAll[server.name] ? "destructive" : "default"}
                  disabled={busy[server.name]}
                  onclick={() => prune(server.name)}
                >
                  {busy[server.name] ? "Pruning…" : pruneAll[server.name] ? "Prune all unused images" : "Prune dangling images"}
                </Button>
              </div>
            </div>
          {/if}
        </section>
      {/each}
    </div>
  {/if}
</div>
