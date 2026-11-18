<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Container, type Server, ApiError } from "$lib/api";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import { Play, Square, RotateCcw } from "lucide-svelte";

  let containers: Container[] = [];
  let servers: Server[] = [];
  let selectedServer: string = "";
  let loading = false;
  let error: string | null = null;

  onMount(async () => {
    await Promise.all([loadContainers(), loadServers()]);
  });

  async function loadContainers() {
    try {
      loading = true;
      error = null;
      containers = await api.containers.list(selectedServer || undefined);
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to load containers";
      console.error("Failed to load containers:", e);
    } finally {
      loading = false;
    }
  }

  async function loadServers() {
    try {
      servers = await api.servers.list();
    } catch (e) {
      console.error("Failed to load servers:", e);
    }
  }

  async function startContainer(containerId: string) {
    try {
      await api.containers.start(containerId, selectedServer || undefined);
      await loadContainers();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to start container";
    }
  }

  async function stopContainer(containerId: string) {
    try {
      await api.containers.stop(containerId, selectedServer || undefined);
      await loadContainers();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to stop container";
    }
  }

  async function restartContainer(containerId: string) {
    try {
      await api.containers.restart(containerId, selectedServer || undefined);
      await loadContainers();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to restart container";
    }
  }

  function getStatusColor(status: string) {
    if (status.includes("running")) return "text-green-600";
    if (status.includes("exited")) return "text-red-600";
    if (status.includes("paused")) return "text-yellow-600";
    return "text-gray-600";
  }

  function formatCreated(created: number) {
    return new Date(created * 1000).toLocaleDateString();
  }

  function getContainerName(names: string[]) {
    return names[0]?.replace(/^\//, "") || "Unknown";
  }

  // Reload containers when selected server changes
  $: if (selectedServer !== undefined) {
    loadContainers();
  }
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h2 class="text-xl font-semibold">Containers</h2>
    <div class="flex items-center gap-4">
      {#if servers.length > 0}
        <select
          bind:value={selectedServer}
          class="px-3 py-2 border rounded-md bg-background"
        >
          <option value="">All Servers</option>
          {#each servers as server}
            <option value={server.name}>{server.name}</option>
          {/each}
        </select>
      {/if}

      <Button onclick={loadContainers} disabled={loading}>
        {loading ? "Loading..." : "Refresh"}
      </Button>
    </div>
  </div>

  {#if error}
    <div class="p-4 bg-destructive/10 border border-destructive/20 rounded-md">
      <p class="text-destructive text-sm">{error}</p>
    </div>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-8">
      <div
        class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"
      ></div>
    </div>
  {:else if containers.length === 0}
    <div class="text-center py-8 text-muted-foreground">
      <p>No containers found</p>
    </div>
  {:else}
    <div class="grid gap-4">
      {#each containers as container (container.Id)}
        <Card class="p-6">
          <div class="flex items-start justify-between">
            <div class="flex-1">
              <div class="flex items-center gap-2 mb-2">
                <h3 class="font-semibold">
                  {getContainerName(container.Names)}
                </h3>
                <span
                  class="text-xs px-2 py-1 rounded-full bg-secondary {getStatusColor(
                    container.Status
                  )}"
                >
                  {container.Status}
                </span>
              </div>

              <div class="space-y-1 text-sm text-muted-foreground">
                <p><strong>Image:</strong> {container.Image}</p>
                <p><strong>ID:</strong> {container.Id.substring(0, 12)}</p>
                <p>
                  <strong>Created:</strong>
                  {formatCreated(container.Created)}
                </p>
                {#if container.Ports && container.Ports.length > 0}
                  <p>
                    <strong>Ports:</strong>
                    {container.Ports.map((p) =>
                      p.PublicPort
                        ? `${p.PublicPort}:${p.PrivatePort}/${p.Type}`
                        : `${p.PrivatePort}/${p.Type}`
                    ).join(", ")}
                  </p>
                {/if}
              </div>
            </div>

            <div class="flex items-center gap-2 ml-4">
              {#if container.State === "running"}
                <Button
                  size="sm"
                  variant="outline"
                  onclick={() => stopContainer(container.Id)}
                >
                  <Square class="w-4 h-4 mr-2" />
                  Stop
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onclick={() => restartContainer(container.Id)}
                >
                  <RotateCcw class="w-4 h-4 mr-2" />
                  Restart
                </Button>
              {:else}
                <Button size="sm" onclick={() => startContainer(container.Id)}>
                  <Play class="w-4 h-4 mr-2" />
                  Start
                </Button>
              {/if}
            </div>
          </div>
        </Card>
      {/each}
    </div>
  {/if}
</div>
