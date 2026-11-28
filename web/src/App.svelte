<script lang="ts">
  import AppList from "./lib/components/AppList.svelte";
  import EnvironmentDetail from "./lib/components/EnvironmentDetail.svelte";
  import ContainerList from "./lib/components/ContainerList.svelte";
  import ServerList from "./lib/components/ServerList.svelte";
  import Activity from "./lib/components/Activity.svelte";
  import Launch from "./lib/components/Launch.svelte";
  import Users from "./lib/components/Users.svelte";
  import Account from "./lib/components/Account.svelte";
  import Login from "./lib/components/Login.svelte";
  import Invite from "./lib/components/Invite.svelte";
  import GoTo from "./lib/components/GoTo.svelte";
  import Shell from "./lib/components/Shell.svelte";
  import Logo from "./lib/components/ui/Logo.svelte";
  import ConfirmDialog from "./lib/components/ui/ConfirmDialog.svelte";
  import Toaster from "./lib/components/ui/Toaster.svelte";
  import { me, can, loadMe, signOut, displayName } from "./lib/session";
  import { theme, type Theme } from "./lib/theme";
  import { onMount } from "svelte";
  import { Sun, Moon, Monitor, LogOut, Search } from "lucide-svelte";

  // Routes: #/apps, #/apps/<app>/<env>[/<tab>], #/apps/<app>/<env>/configure,
  // #/launch[/<catalog entry>], #/activity, #/containers, #/servers,
  // #/users, #/account, #/shell/<server>[/<container>], #/invite/<token>
  type Route =
    | { page: "apps" | "activity" | "containers" | "servers" | "users" | "account" }
    | { page: "launch"; preset?: string }
    | { page: "environment"; app: string; env: string; tab?: string }
    | { page: "configure"; app: string; env: string }
    | { page: "shell"; server: string; container?: string }
    | { page: "invite"; token: string };

  const simplePages = ["apps", "launch", "activity", "containers", "servers", "users", "account"] as const;

  let route = $state<Route>(parse(location.hash));
  // Unknown until the first /me answers, so the sign-in page doesn't flash
  let checked = $state(false);
  let goTo = $state(false);

  const nav = [
    { id: "apps", label: "Apps" },
    { id: "activity", label: "Activity" },
    { id: "containers", label: "Containers" },
    { id: "servers", label: "Servers" },
  ];

  const themes: { id: Theme; label: string }[] = [
    { id: "system", label: "Match system theme" },
    { id: "light", label: "Light theme" },
    { id: "dark", label: "Dark theme" },
  ];

  const titles: Record<string, string> = {
    apps: "Apps",
    launch: "Launch an app",
    activity: "Activity",
    containers: "Containers",
    servers: "Servers",
    users: "Users",
    account: "Account",
    invite: "Welcome aboard",
  };

  let active = $derived(
    route.page === "environment" || route.page === "configure" || route.page === "launch"
      ? "apps"
      : route.page === "shell"
        ? route.container ? "containers" : "servers"
        : route.page
  );
  let themeLabel = $derived(themes.find((t) => t.id === $theme)?.label);

  $effect(() => {
    document.title =
      route.page === "environment"
        ? `${route.app}/${route.env} · Docktrine`
        : route.page === "configure"
          ? `Configure ${route.app}/${route.env} · Docktrine`
          : route.page === "shell"
            ? `Shell on ${route.server} · Docktrine`
            : `${titles[route.page]} · Docktrine`;
  });

  function parse(hash: string): Route {
    const parts = hash.replace(/^#\/?/, "").split("/").map(decodeURIComponent);
    if (parts[0] === "apps" && parts.length >= 4 && parts[3] === "configure") {
      return { page: "configure", app: parts[1], env: parts[2] };
    }
    if (parts[0] === "apps" && parts.length >= 3) {
      return { page: "environment", app: parts[1], env: parts[2], tab: parts[3] };
    }
    if (parts[0] === "invite" && parts[1]) return { page: "invite", token: parts[1] };
    if (parts[0] === "launch") return { page: "launch", preset: parts[1] || undefined };
    if (parts[0] === "shell" && parts[1]) return { page: "shell", server: parts[1], container: parts[2] || undefined };
    const page = simplePages.find((p) => p === parts[0]);
    return { page: page ?? "apps" };
  }

  function cycleTheme() {
    const i = themes.findIndex((t) => t.id === $theme);
    theme.set(themes[(i + 1) % themes.length].id);
  }

  function initials(name: string) {
    const words = name.trim().split(/[\s._-]+/).filter(Boolean);
    return ((words[0]?.[0] ?? "?") + (words[1]?.[0] ?? "")).toUpperCase();
  }

  onMount(() => {
    loadMe().finally(() => (checked = true));
    const onUnauthorized = () => me.set(null);
    const onHashChange = () => (route = parse(location.hash));
    const onKey = (e: KeyboardEvent) => {
      // In a terminal, Ctrl+K belongs to the shell (kill to end of line)
      const inTerminal = e.target instanceof Element && e.target.closest(".xterm");
      if ((e.metaKey || (e.ctrlKey && !inTerminal)) && e.key.toLowerCase() === "k" && $me) {
        e.preventDefault();
        goTo = !goTo;
      }
    };
    window.addEventListener("docktrine:unauthorized", onUnauthorized);
    window.addEventListener("hashchange", onHashChange);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("docktrine:unauthorized", onUnauthorized);
      window.removeEventListener("hashchange", onHashChange);
      window.removeEventListener("keydown", onKey);
    };
  });
</script>

{#if route.page === "invite"}
  <Invite token={route.token} />
{:else if !checked}
  <!-- Waiting for the first /me -->
{:else if !$me}
  <Login />
{:else}
  <header class="bg-iron text-iron-foreground">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 flex flex-wrap items-center gap-x-6">
      <a href="#/apps" class="flex items-center gap-2.5 h-16 shrink-0" aria-label="Docktrine home">
        <Logo class="w-9 h-9" />
        <span class="font-slab text-2xl font-bold tracking-[0.01em]">Docktrine</span>
      </a>

      <nav
        class="order-last md:order-none basis-full md:basis-auto flex gap-1 -mx-1 overflow-x-auto pb-3 md:pb-0"
        aria-label="Main"
      >
        {#each nav as item}
          {@render navLink(item.id, item.label)}
        {/each}
        {#if $can("admin")}
          {@render navLink("users", "Users")}
        {/if}
      </nav>

      <div class="ml-auto flex items-center gap-1">
        <button
          class="h-9 px-2.5 flex items-center gap-2 rounded-md text-sm text-iron-muted hover:text-iron-foreground hover:bg-white/10"
          onclick={() => (goTo = true)}
          aria-label="Go to an app or page"
        >
          <Search class="w-4 h-4" />
          <kbd class="hidden lg:inline font-sans text-xs border border-white/20 rounded px-1">⌘K</kbd>
        </button>
        <button
          class="h-9 w-9 grid place-items-center rounded-md text-iron-muted hover:text-iron-foreground hover:bg-white/10"
          onclick={cycleTheme}
          title={themeLabel}
          aria-label="{themeLabel}, click to change"
        >
          {#if $theme === "light"}
            <Sun class="w-4 h-4" />
          {:else if $theme === "dark"}
            <Moon class="w-4 h-4" />
          {:else}
            <Monitor class="w-4 h-4" />
          {/if}
        </button>
        <a
          href="#/account"
          class="flex items-center gap-2 h-10 pl-1 pr-2.5 rounded-full hover:bg-white/10
            {active === 'account' ? 'bg-white/10' : ''}"
          title="Your account"
        >
          <span class="w-8 h-8 rounded-full brass-fill grid place-items-center text-xs font-bold">
            {initials(displayName($me))}
          </span>
          <span class="hidden sm:block text-sm leading-tight">
            {displayName($me)}
            <span class="block text-[11px] text-iron-muted">{$me.role}{$me.via === "key" ? ", API key" : ""}</span>
          </span>
        </a>
        <button
          class="h-9 w-9 grid place-items-center rounded-md text-iron-muted hover:text-iron-foreground hover:bg-white/10"
          onclick={signOut}
          title="Sign out"
          aria-label="Sign out"
        >
          <LogOut class="w-4 h-4" />
        </button>
      </div>
    </div>
  </header>
  <div class="brass-trim" aria-hidden="true"></div>

  <main class="max-w-7xl mx-auto px-4 sm:px-6 py-8 lg:py-10">
    {#if $me.via === "key" && !$me.user}
      <p class="mb-8 text-sm text-muted-foreground border-l-[3px] border-brass pl-3">
        You're signed in with the API key {$me.key_name}.
        {#if route.page !== "users"}<a href="#/users" class="text-link hover:underline">Invite yourself</a>{:else}Invite yourself below{/if}
        to sign in with a password instead.
      </p>
    {/if}

    {#if route.page === "apps"}
      <AppList />
    {:else if route.page === "environment"}
      {#key `${route.app}/${route.env}`}
        <EnvironmentDetail app={route.app} env={route.env} tab={route.tab} />
      {/key}
    {:else if route.page === "launch"}
      {#key `launch/${route.preset ?? ""}`}<Launch preset={route.preset} />{/key}
    {:else if route.page === "configure"}
      {#key `${route.app}/${route.env}`}
        <Launch app={route.app} env={route.env} />
      {/key}
    {:else if route.page === "activity"}
      <Activity />
    {:else if route.page === "containers"}
      <ContainerList />
    {:else if route.page === "servers"}
      <ServerList />
    {:else if route.page === "users"}
      <Users />
    {:else if route.page === "account"}
      <Account />
    {:else if route.page === "shell"}
      {#key `${route.server}/${route.container ?? ""}`}
        <Shell server={route.server} container={route.container} />
      {/key}
    {/if}
  </main>

  <GoTo bind:open={goTo} />
{/if}

{#snippet navLink(id: string, label: string)}
  <a
    href="#/{id}"
    aria-current={active === id ? "page" : undefined}
    class="h-9 px-3 flex items-center rounded-md text-sm whitespace-nowrap
      {active === id ? 'brass-fill font-semibold' : 'text-iron-muted hover:text-iron-foreground hover:bg-white/10'}"
  >
    {label}
  </a>
{/snippet}

<ConfirmDialog />
<Toaster />
