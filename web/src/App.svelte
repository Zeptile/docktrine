<script lang="ts">
  import AppList from "./lib/components/AppList.svelte";
  import EnvironmentDetail from "./lib/components/EnvironmentDetail.svelte";
  import ContainerList from "./lib/components/ContainerList.svelte";
  import ServerList from "./lib/components/ServerList.svelte";
  import ApiKeys from "./lib/components/ApiKeys.svelte";
  import Login from "./lib/components/Login.svelte";
  import Logo from "./lib/components/ui/Logo.svelte";
  import ConfirmDialog from "./lib/components/ui/ConfirmDialog.svelte";
  import Toaster from "./lib/components/ui/Toaster.svelte";
  import { auth } from "./lib/api";
  import { theme, type Theme } from "./lib/theme";
  import { onMount } from "svelte";
  import { Sun, Moon, Monitor, LogOut } from "lucide-svelte";

  // Routes: #/apps, #/apps/<app>/<env>[/<tab>], #/containers, #/servers, #/keys
  type Route =
    | { page: "apps" }
    | { page: "environment"; app: string; env: string; tab?: string }
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

  const themes: { id: Theme; label: string }[] = [
    { id: "system", label: "Match system theme" },
    { id: "light", label: "Light theme" },
    { id: "dark", label: "Dark theme" },
  ];

  $: activeTab = route.page === "environment" ? "apps" : route.page;
  $: document.title =
    route.page === "environment"
      ? `${route.app}/${route.env} · Docktrine`
      : `${tabs.find((t) => t.id === route.page)?.label} · Docktrine`;

  function parse(hash: string): Route {
    const parts = hash.replace(/^#\/?/, "").split("/").map(decodeURIComponent);
    if (parts[0] === "apps" && parts.length >= 3) {
      return { page: "environment", app: parts[1], env: parts[2], tab: parts[3] };
    }
    if (parts[0] === "containers" || parts[0] === "servers" || parts[0] === "keys") {
      return { page: parts[0] };
    }
    return { page: "apps" };
  }

  function cycleTheme() {
    const i = themes.findIndex((t) => t.id === $theme);
    theme.set(themes[(i + 1) % themes.length].id);
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

{#if !authenticated}
  <Login onLogin={() => (authenticated = true)} />
{:else}
  <header class="bg-yard text-yard-foreground">
    <div class="max-w-6xl mx-auto px-4 sm:px-6 flex flex-wrap items-center gap-x-6">
      <a href="#/apps" class="flex items-center gap-2.5 h-14 shrink-0" aria-label="Docktrine home">
        <Logo class="w-7 h-7" />
        <span class="stencil text-2xl tracking-[0.08em]">DOCKTRINE</span>
      </a>

      <nav
        class="order-last sm:order-none w-full sm:w-auto flex gap-1 -mx-2 sm:mx-0 overflow-x-auto"
        aria-label="Main"
      >
        {#each tabs as tab}
          <a
            href="#/{tab.id}"
            aria-current={activeTab === tab.id ? "page" : undefined}
            class="relative px-3 h-11 sm:h-14 flex items-center text-sm font-medium whitespace-nowrap transition-colors
              {activeTab === tab.id ? 'text-yard-foreground' : 'text-yard-muted hover:text-yard-foreground'}"
          >
            {tab.label}
            {#if activeTab === tab.id}
              <span class="absolute inset-x-3 bottom-0 h-[3px] rounded-t-sm bg-signal"></span>
            {/if}
          </a>
        {/each}
      </nav>

      <div class="ml-auto flex items-center gap-1">
        <button
          class="h-9 w-9 grid place-items-center rounded-md text-yard-muted hover:text-yard-foreground hover:bg-white/10 transition-colors"
          onclick={cycleTheme}
          title={themes.find((t) => t.id === $theme)?.label}
          aria-label="{themes.find((t) => t.id === $theme)?.label}, click to change"
        >
          {#if $theme === "light"}
            <Sun class="w-4 h-4" />
          {:else if $theme === "dark"}
            <Moon class="w-4 h-4" />
          {:else}
            <Monitor class="w-4 h-4" />
          {/if}
        </button>
        <button
          class="h-9 px-2.5 flex items-center gap-2 rounded-md text-sm text-yard-muted hover:text-yard-foreground hover:bg-white/10 transition-colors"
          onclick={logout}
        >
          <LogOut class="w-4 h-4" />
          <span class="hidden md:inline">Sign out</span>
        </button>
      </div>
    </div>
  </header>

  <main class="max-w-6xl mx-auto px-4 sm:px-6 py-8">
    {#if route.page === "apps"}
      <AppList />
    {:else if route.page === "environment"}
      {#key `${route.app}/${route.env}`}
        <EnvironmentDetail app={route.app} env={route.env} tab={route.tab} />
      {/key}
    {:else if route.page === "containers"}
      <ContainerList />
    {:else if route.page === "servers"}
      <ServerList />
    {:else if route.page === "keys"}
      <ApiKeys />
    {/if}
  </main>
{/if}

<ConfirmDialog />
<Toaster />
