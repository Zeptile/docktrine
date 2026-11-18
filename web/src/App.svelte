<script lang="ts">
  import ContainerList from "./lib/components/ContainerList.svelte";
  import ServerList from "./lib/components/ServerList.svelte";
  import Login from "./lib/components/Login.svelte";
  import { auth } from "./lib/api";
  import { onMount } from "svelte";

  let activeTab = "containers";
  let authenticated = !!auth.getKey();

  onMount(() => {
    const onUnauthorized = () => (authenticated = false);
    window.addEventListener("docktrine:unauthorized", onUnauthorized);
    return () =>
      window.removeEventListener("docktrine:unauthorized", onUnauthorized);
  });

  function logout() {
    auth.clearKey();
    authenticated = false;
  }

  function setActiveTab(tab: string) {
    activeTab = tab;
  }
</script>

<main class="min-h-screen bg-background">
  {#if !authenticated}
    <Login onLogin={() => (authenticated = true)} />
  {:else}
  <header class="border-b">
    <div class="container mx-auto px-4 py-4 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-foreground">Docktrine</h1>
        <p class="text-muted-foreground">Docker Container Management</p>
      </div>
      <button
        class="text-sm text-muted-foreground hover:text-foreground"
        on:click={logout}
      >
        Sign out
      </button>
    </div>
  </header>

  <nav class="border-b">
    <div class="container mx-auto px-4">
      <div class="flex space-x-1">
        <button
          class="px-4 py-2 text-sm font-medium rounded-t-lg transition-colors {activeTab ===
          'containers'
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:text-foreground'}"
          on:click={() => setActiveTab("containers")}
        >
          Containers
        </button>
        <button
          class="px-4 py-2 text-sm font-medium rounded-t-lg transition-colors {activeTab ===
          'servers'
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:text-foreground'}"
          on:click={() => setActiveTab("servers")}
        >
          Servers
        </button>
      </div>
    </div>
  </nav>

  <div class="container mx-auto px-4 py-6">
    {#if activeTab === "containers"}
      <ContainerList />
    {:else if activeTab === "servers"}
      <ServerList />
    {/if}
  </div>
  {/if}
</main>
