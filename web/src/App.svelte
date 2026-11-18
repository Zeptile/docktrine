<script lang="ts">
  import AppList from "./lib/components/AppList.svelte";
  import EnvironmentDetail from "./lib/components/EnvironmentDetail.svelte";
  import ContainerList from "./lib/components/ContainerList.svelte";
  import ServerList from "./lib/components/ServerList.svelte";
  import ApiKeys from "./lib/components/ApiKeys.svelte";
  import Login from "./lib/components/Login.svelte";
  import { auth } from "./lib/api";
  import { onMount } from "svelte";

  // Routes: #/apps, #/apps/<app>/<env>, #/containers, #/servers, #/keys
  type Route =
    | { page: "apps" }
    | { page: "environment"; app: string; env: string }
    | { page: "containers" }
    | { page: "servers" }
    | { page: "keys" };

  let route: Route = parse(location.hash);
  let authenticated = !!auth.getKey();

  const tabs = [
    { id: "apps", label: "Apps" },
    { id: "containers", label: "Containers" },
    { id: "servers", label: "Servers" },
    { id: "keys", label: "API keys" },
  ];

  $: activeTab = route.page === "environment" ? "apps" : route.page;

  function parse(hash: string): Route {
    const parts = hash.replace(/^#\/?/, "").split("/").map(decodeURIComponent);
    if (parts[0] === "apps" && parts.length >= 3) {
      return { page: "environment", app: parts[1], env: parts[2] };
    }
    if (parts[0] === "containers" || parts[0] === "servers" || parts[0] === "keys") {
      return { page: parts[0] };
    }
    return { page: "apps" };
  }

  onMount(() => {
    const onUnauthorized = () => (authenticated = false);
    const onHashChange = () => (route = parse(location.hash));
    window.addEventListener("docktrine:unauthorized", onUnauthorized);
    window.addEventListener("hashchange", onHashChange);
    return () => {
      window.removeEventListener("docktrine:unauthorized", onUnauthorized);
      window.removeEventListener("hashchange", onHashChange);
    };
  });

  function logout() {
    auth.clearKey();
    authenticated = false;
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
        <p class="text-muted-foreground">Apps and nodes</p>
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
        {#each tabs as tab}
          <a
            href="#/{tab.id}"
            class="px-4 py-2 text-sm font-medium rounded-t-lg transition-colors {activeTab ===
            tab.id
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:text-foreground'}"
          >
            {tab.label}
          </a>
        {/each}
      </div>
    </div>
  </nav>

  <div class="container mx-auto px-4 py-6">
    {#if route.page === "apps"}
      <AppList />
    {:else if route.page === "environment"}
      {#key `${route.app}/${route.env}`}
        <EnvironmentDetail app={route.app} env={route.env} />
      {/key}
    {:else if route.page === "containers"}
      <ContainerList />
    {:else if route.page === "servers"}
      <ServerList />
    {:else if route.page === "keys"}
      <ApiKeys />
    {/if}
  </div>
  {/if}
</main>
