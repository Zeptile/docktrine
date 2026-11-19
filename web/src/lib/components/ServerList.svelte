<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Server, type ServerHealth, ApiError } from "$lib/api";
  import { bytes } from "$lib/format";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import {
    Plus,
    Trash2,
    Server as ServerIcon,
    Activity,
    Network,
    Eraser,
    Pencil,
  } from "lucide-svelte";

  type Panel = "edit" | "ingress" | "prune" | null;

  let servers: Server[] = [];
  let loading = false;
  let error: string | null = null;
  let notice: string | null = null;
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
      servers.forEach((s) => checkHealth(s.name));
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to load servers";
      console.error("Failed to load servers:", e);
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
    try {
      await api.servers.create({
        ...newServer,
        ingress_port: newServer.ingress_port ? Number(newServer.ingress_port) : undefined,
      });
      await loadServers();
      showCreateForm = false;
      newServer = blankServer();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to create server";
    }
  }

  async function deleteServer(name: string) {
    if (!confirm(`Are you sure you want to delete server "${name}"?`)) {
      return;
    }

    try {
      await api.servers.delete(name);
      await loadServers();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to delete server";
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
    notice = null;
    try {
      notice = await action();
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
      return `${name} updated`;
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

  function prune(name: string) {
    const all = !!pruneAll[name];
    if (all && !confirm(`Remove every image on ${name} that no container uses? They'll have to be pulled or rebuilt again.`)) {
      return;
    }
    run(name, async () => {
      const res = await api.servers.prune(name, all);
      await checkHealth(name);
      return `Removed ${res.images_deleted} image(s) from ${name}, reclaimed ${bytes(res.space_reclaimed)}`;
    });
  }
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h2 class="text-xl font-semibold">Servers</h2>
    <div class="flex items-center gap-4">
      <Button onclick={loadServers} disabled={loading} variant="outline">
        {loading ? "Loading..." : "Refresh"}
      </Button>
      <Button onclick={() => (showCreateForm = true)}>
        <Plus class="w-4 h-4 mr-2" />
        Add Server
      </Button>
    </div>
  </div>

  {#if error}
    <div class="p-4 bg-destructive/10 border border-destructive/20 rounded-md">
      <p class="text-destructive text-sm">{error}</p>
    </div>
  {/if}
  {#if notice}
    <div class="p-4 bg-green-50 border border-green-200 rounded-md">
      <p class="text-green-700 text-sm">{notice}</p>
    </div>
  {/if}

  {#if showCreateForm}
    <Card class="p-6">
      <h3 class="text-lg font-medium mb-4">Add New Server</h3>
      <form
        onsubmit={(e) => {
          e.preventDefault();
          createServer();
        }}
        class="space-y-4"
      >
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label for="name" class="block text-sm font-medium mb-2"
              >Name *</label
            >
            <input
              id="name"
              bind:value={newServer.name}
              type="text"
              required
              class="w-full px-3 py-2 border rounded-md bg-background"
              placeholder="prd1-tor"
            />
          </div>
          <div>
            <label for="host" class="block text-sm font-medium mb-2"
              >Docker host *</label
            >
            <input
              id="host"
              bind:value={newServer.host}
              type="text"
              required
              class="w-full px-3 py-2 border rounded-md bg-background font-mono"
              placeholder="ssh://deploy@prd1-tor.zeptile.software"
            />
          </div>
        </div>

        <div>
          <label for="description" class="block text-sm font-medium mb-2"
            >Description</label
          >
          <input
            id="description"
            bind:value={newServer.description}
            type="text"
            class="w-full px-3 py-2 border rounded-md bg-background"
          />
        </div>

        <div>
          <p class="text-sm font-medium mb-1">Ingress, as seen from the gateway</p>
          <p class="text-xs text-muted-foreground mb-2">
            Where Nginx Proxy Manager forwards this node's hostnames, e.g. the
            gateway end of the SSH tunnel to the node. Needed for hostnames.
          </p>
          <div class="grid grid-cols-2 gap-4">
            <input
              bind:value={newServer.ingress_host}
              aria-label="Ingress host"
              class="w-full px-3 py-2 border rounded-md bg-background font-mono"
              placeholder="127.0.0.1"
            />
            <input
              bind:value={newServer.ingress_port}
              aria-label="Ingress port"
              type="number"
              class="w-full px-3 py-2 border rounded-md bg-background font-mono"
              placeholder="8080"
            />
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-4">
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
          <Button type="submit">Create Server</Button>
        </div>
      </form>
    </Card>
  {/if}

  {#if loading && servers.length === 0}
    <div class="flex items-center justify-center py-8">
      <div
        class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"
      ></div>
    </div>
  {:else if servers.length === 0}
    <div class="text-center py-8 text-muted-foreground">
      <ServerIcon class="w-12 h-12 mx-auto mb-4 opacity-50" />
      <p>No servers configured</p>
      <p class="text-sm">Add a server to get started</p>
    </div>
  {:else}
    <div class="grid gap-4">
      {#each servers as server (server.id)}
        {@const h = health[server.name]}
        <Card class="p-6">
          <div class="flex items-start justify-between">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-2">
                <ServerIcon class="w-5 h-5" />
                <h3 class="font-semibold">{server.name}</h3>
                {#if server.is_default}
                  <span class="text-xs px-2 py-1 rounded-full bg-muted text-muted-foreground">default</span>
                {/if}
                {#if h === "loading"}
                  <span class="text-xs text-muted-foreground">checking...</span>
                {:else if h?.reachable}
                  <span class="text-xs px-2 py-1 rounded-full bg-green-100 text-green-700">online</span>
                {:else if h}
                  <span class="text-xs px-2 py-1 rounded-full bg-red-100 text-red-700" title={h.error}>unreachable</span>
                {/if}
              </div>

              <div class="space-y-1 text-sm text-muted-foreground">
                {#if server.description}<p>{server.description}</p>{/if}
                <p><strong>Host:</strong> <span class="font-mono">{server.host}</span></p>
                <p>
                  <strong>Ingress:</strong>
                  {#if h && h !== "loading" && h.reachable}
                    <span class={h.ingress === "running" ? "text-green-700" : "text-yellow-700"}>{h.ingress}</span>
                  {/if}
                  {#if server.ingress_host && server.ingress_port}
                    · gateway forwards to <span class="font-mono">{server.ingress_host}:{server.ingress_port}</span>
                  {:else}
                    · <span class="text-yellow-700">no gateway target set, hostnames won't work</span>
                  {/if}
                </p>
                {#if h && h !== "loading" && h.reachable}
                  <p>
                    Docker {h.server_version} on {h.os} · {h.cpus} CPUs · {bytes(h.memory_bytes)}
                    · {h.containers_running}/{h.containers} containers running · {h.images} images
                  </p>
                {:else if h && h !== "loading"}
                  <p class="text-destructive break-all">{h.error}</p>
                {/if}
              </div>
            </div>

            <div class="flex items-center gap-1 ml-4 shrink-0">
              <Button size="sm" variant="ghost" onclick={() => checkHealth(server.name)} title="Check health">
                <Activity class="w-4 h-4" />
              </Button>
              <Button size="sm" variant="ghost" onclick={() => toggle(server, "edit")} title="Edit">
                <Pencil class="w-4 h-4" />
              </Button>
              <Button size="sm" variant="outline" onclick={() => toggle(server, "ingress")}>
                <Network class="w-4 h-4 mr-2" /> Ingress
              </Button>
              <Button size="sm" variant="outline" onclick={() => toggle(server, "prune")}>
                <Eraser class="w-4 h-4 mr-2" /> Prune
              </Button>
              <Button
                size="sm"
                variant="destructive"
                onclick={() => deleteServer(server.name)}
              >
                <Trash2 class="w-4 h-4 mr-2" />
                Delete
              </Button>
            </div>
          </div>

          {#if panel[server.name] === "edit" && edits[server.name]}
            <form
              class="mt-4 pt-4 border-t grid grid-cols-2 gap-4"
              onsubmit={(e) => {
                e.preventDefault();
                saveEdit(server.name);
              }}
            >
              <label class="text-sm">
                <span class="block font-medium mb-1">Docker host</span>
                <input bind:value={edits[server.name].host} required class="w-full px-3 py-2 border rounded-md bg-background font-mono" />
              </label>
              <label class="text-sm">
                <span class="block font-medium mb-1">Description</span>
                <input bind:value={edits[server.name].description} class="w-full px-3 py-2 border rounded-md bg-background" />
              </label>
              <label class="text-sm">
                <span class="block font-medium mb-1">Gateway ingress host</span>
                <input bind:value={edits[server.name].ingress_host} class="w-full px-3 py-2 border rounded-md bg-background font-mono" placeholder="127.0.0.1" />
              </label>
              <label class="text-sm">
                <span class="block font-medium mb-1">Gateway ingress port</span>
                <input bind:value={edits[server.name].ingress_port} type="number" class="w-full px-3 py-2 border rounded-md bg-background font-mono" placeholder="8080" />
              </label>
              <p class="col-span-2 text-xs text-muted-foreground">
                Changing the gateway target only affects hostnames created or synced after the change; redeploy apps to update theirs.
              </p>
              <div class="col-span-2 flex justify-end gap-2">
                <Button type="button" variant="outline" onclick={() => (panel[server.name] = null)}>Cancel</Button>
                <Button type="submit" disabled={busy[server.name]}>Save</Button>
              </div>
            </form>
          {:else if panel[server.name] === "ingress" && ingress[server.name]}
            <form
              class="mt-4 pt-4 border-t space-y-3"
              onsubmit={(e) => {
                e.preventDefault();
                installIngress(server.name);
              }}
            >
              <p class="text-sm text-muted-foreground">
                (Re)creates the Traefik container on {server.name} that routes
                requests to app containers by hostname. Apps keep running, but
                their hostnames are briefly unreachable while it restarts.
              </p>
              <div class="grid grid-cols-2 gap-4">
                <label class="text-sm">
                  <span class="block font-medium mb-1">Port on the node</span>
                  <input bind:value={ingress[server.name].port} type="number" required min="1" max="65535" class="w-full px-3 py-2 border rounded-md bg-background font-mono" />
                </label>
                <label class="text-sm">
                  <span class="block font-medium mb-1">Bind address</span>
                  <input bind:value={ingress[server.name].bind} class="w-full px-3 py-2 border rounded-md bg-background font-mono" placeholder="all interfaces" />
                  <span class="block text-xs text-muted-foreground mt-1">Use 127.0.0.1 when the gateway reaches the node through an SSH tunnel.</span>
                </label>
              </div>
              <div class="flex justify-end gap-2">
                <Button type="button" variant="outline" onclick={() => (panel[server.name] = null)}>Cancel</Button>
                <Button type="submit" disabled={busy[server.name]}>
                  {busy[server.name] ? "Installing..." : "Install ingress"}
                </Button>
              </div>
            </form>
          {:else if panel[server.name] === "prune"}
            <div class="mt-4 pt-4 border-t space-y-3">
              <label class="flex items-start gap-2 text-sm">
                <input type="checkbox" bind:checked={pruneAll[server.name]} class="w-4 h-4 mt-0.5" />
                <span>
                  Remove every image no container uses, not just dangling ones
                  <span class="block text-xs text-yellow-700">
                    This includes images that aren't Docktrine's, which then have to be pulled or rebuilt again.
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
                  {busy[server.name] ? "Pruning..." : "Prune images"}
                </Button>
              </div>
            </div>
          {/if}
        </Card>
      {/each}
    </div>
  {/if}
</div>
