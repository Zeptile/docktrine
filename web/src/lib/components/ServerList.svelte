<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Server, ApiError } from "$lib/api";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import { Plus, Trash2, Server as ServerIcon } from "lucide-svelte";

  let servers: Server[] = [];
  let loading = false;
  let error: string | null = null;
  let showCreateForm = false;
  let newServer: Omit<Server, "id" | "created_at" | "updated_at"> = {
    name: "",
    host: "",
    port: 2376,
    tls: false,
    cert_path: "",
    key_path: "",
    ca_path: "",
  };

  onMount(async () => {
    await loadServers();
  });

  async function loadServers() {
    try {
      loading = true;
      error = null;
      servers = await api.servers.list();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to load servers";
      console.error("Failed to load servers:", e);
    } finally {
      loading = false;
    }
  }

  async function createServer() {
    try {
      await api.servers.create(newServer);
      await loadServers();
      showCreateForm = false;
      resetForm();
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

  function resetForm() {
    newServer = {
      name: "",
      host: "",
      port: 2376,
      tls: false,
      cert_path: "",
      key_path: "",
      ca_path: "",
    };
  }

  function cancelCreate() {
    showCreateForm = false;
    resetForm();
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
              placeholder="Server name"
            />
          </div>
          <div>
            <label for="host" class="block text-sm font-medium mb-2"
              >Host *</label
            >
            <input
              id="host"
              bind:value={newServer.host}
              type="text"
              required
              class="w-full px-3 py-2 border rounded-md bg-background"
              placeholder="localhost or IP address"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label for="port" class="block text-sm font-medium mb-2">Port</label
            >
            <input
              id="port"
              bind:value={newServer.port}
              type="number"
              class="w-full px-3 py-2 border rounded-md bg-background"
              placeholder="2376"
            />
          </div>
          <div class="flex items-center space-x-2 pt-6">
            <input
              id="tls"
              bind:checked={newServer.tls}
              type="checkbox"
              class="w-4 h-4"
            />
            <label for="tls" class="text-sm font-medium">Enable TLS</label>
          </div>
        </div>

        {#if newServer.tls}
          <div class="space-y-4">
            <div>
              <label for="cert_path" class="block text-sm font-medium mb-2"
                >Certificate Path</label
              >
              <input
                id="cert_path"
                bind:value={newServer.cert_path}
                type="text"
                class="w-full px-3 py-2 border rounded-md bg-background"
                placeholder="/path/to/cert.pem"
              />
            </div>
            <div>
              <label for="key_path" class="block text-sm font-medium mb-2"
                >Key Path</label
              >
              <input
                id="key_path"
                bind:value={newServer.key_path}
                type="text"
                class="w-full px-3 py-2 border rounded-md bg-background"
                placeholder="/path/to/key.pem"
              />
            </div>
            <div>
              <label for="ca_path" class="block text-sm font-medium mb-2"
                >CA Path</label
              >
              <input
                id="ca_path"
                bind:value={newServer.ca_path}
                type="text"
                class="w-full px-3 py-2 border rounded-md bg-background"
                placeholder="/path/to/ca.pem"
              />
            </div>
          </div>
        {/if}

        <div class="flex justify-end gap-2 pt-4">
          <Button type="button" variant="outline" onclick={cancelCreate}>
            Cancel
          </Button>
          <Button type="submit">Create Server</Button>
        </div>
      </form>
    </Card>
  {/if}

  {#if loading}
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
        <Card class="p-6">
          <div class="flex items-start justify-between">
            <div class="flex-1">
              <div class="flex items-center gap-2 mb-2">
                <ServerIcon class="w-5 h-5" />
                <h3 class="font-semibold">{server.name}</h3>
                {#if server.tls}
                  <span
                    class="text-xs px-2 py-1 rounded-full bg-green-100 text-green-700"
                  >
                    TLS
                  </span>
                {/if}
              </div>

              <div class="space-y-1 text-sm text-muted-foreground">
                <p><strong>Host:</strong> {server.host}</p>
                <p><strong>Port:</strong> {server.port || 2376}</p>
                {#if server.created_at}
                  <p>
                    <strong>Created:</strong>
                    {new Date(server.created_at).toLocaleDateString()}
                  </p>
                {/if}
              </div>
            </div>

            <div class="flex items-center gap-2 ml-4">
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
        </Card>
      {/each}
    </div>
  {/if}
</div>
