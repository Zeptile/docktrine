<script lang="ts">
  import { api, type AppSummary, type Preset, type Server } from "$lib/api";
  import { can } from "$lib/session";
  import StatusBadge from "./ui/StatusBadge.svelte";
  import { Search } from "lucide-svelte";

  let { open = $bindable(false) }: { open?: boolean } = $props();

  let dialog: HTMLDialogElement;
  let query = $state("");
  let selected = $state(0);
  let envs = $state<AppSummary[]>([]);
  let catalog = $state<Preset[]>([]);
  let servers = $state<Server[]>([]);

  interface Item {
    href: string;
    label: string;
    hint: string;
    env?: AppSummary;
  }

  let pages = $derived<Item[]>([
    { href: "#/apps", label: "Apps", hint: "Page" },
    ...($can("deployer") ? [{ href: "#/launch", label: "Launch an app", hint: "Page" }] : []),
    { href: "#/activity", label: "Activity", hint: "Page" },
    { href: "#/containers", label: "Containers", hint: "Page" },
    { href: "#/servers", label: "Servers", hint: "Page" },
    ...($can("admin") ? [{ href: "#/users", label: "Users", hint: "Page" }] : []),
    { href: "#/account", label: "Account and API keys", hint: "Page" },
  ]);

  let items = $derived.by<Item[]>(() => {
    const q = query.trim().toLowerCase();
    const envItems = envs.flatMap((s) => {
      const name = `${s.environment.app}/${s.environment.name}`;
      const base = `#/apps/${encodeURIComponent(s.environment.app)}/${encodeURIComponent(s.environment.name)}`;
      const env = { href: base, label: name, hint: s.environment.server, env: s };
      // Logs and secrets only when asked for, to keep the list short
      const extra = [
        { ...env, href: base + "/logs", label: name + " logs" },
        { ...env, href: base + "/secrets", label: name + " secrets" },
        ...($can("deployer")
          ? [
              { ...env, href: base + "/configure", label: "Configure " + name },
              { ...env, href: base + "/shell", label: name + " shell" },
            ]
          : []),
      ];
      return q ? [env, ...extra] : [env];
    });
    // Known apps only when asked for, like "launch" or "sentry"
    const known = q ? catalog.map((p) => ({ href: `#/launch/${p.id}`, label: `Launch ${p.name}`, hint: "Known app" })) : [];
    // A root shell on each node, for admins who ask for one
    const shells = q && $can("admin")
      ? servers.map((s) => ({ href: `#/shell/${encodeURIComponent(s.name)}`, label: `Shell on ${s.name}`, hint: "Server" }))
      : [];
    const all = [...envItems, ...pages, ...known, ...shells];
    if (!q) return all;
    const words = q.split(/\s+/);
    return all.filter((i) => words.every((w) => i.label.toLowerCase().includes(w)));
  });

  $effect(() => {
    if (open && !dialog.open) {
      query = "";
      selected = 0;
      dialog.showModal();
      api.apps.list().then((list) => (envs = list), () => {});
      if ($can("deployer") && !catalog.length) api.launch.catalog().then((list) => (catalog = list), () => {});
      if ($can("admin")) api.servers.list().then((list) => (servers = list), () => {});
    }
    if (!open && dialog.open) dialog.close();
  });

  $effect(() => {
    query;
    selected = 0;
  });

  // Any navigation, by link or the back button, closes it
  $effect(() => {
    const close = () => (open = false);
    window.addEventListener("hashchange", close);
    return () => window.removeEventListener("hashchange", close);
  });

  function go(item: Item | undefined) {
    if (!item) return;
    location.hash = item.href;
    open = false;
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      selected = Math.min(items.length - 1, selected + 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      selected = Math.max(0, selected - 1);
    } else if (e.key === "Enter") {
      e.preventDefault();
      go(items[selected]);
    }
  }
</script>

<dialog
  bind:this={dialog}
  onclose={() => (open = false)}
  onclick={(e) => e.target === dialog && (open = false)}
  class="mt-[12vh] w-[min(36rem,calc(100vw-2rem))] rounded-lg border bg-card text-foreground p-0 shadow-2xl backdrop:bg-[rgb(var(--terminal)/0.45)]"
  aria-label="Go to"
>
  <div class="flex items-center gap-3 px-4 border-b">
    <Search class="w-4 h-4 text-muted-foreground shrink-0" />
    <!-- svelte-ignore a11y_autofocus -->
    <input
      bind:value={query}
      onkeydown={onKey}
      autofocus
      placeholder="An app, environment or page"
      aria-label="Search"
      class="flex-1 h-12 bg-transparent outline-none focus-visible:ring-0 focus-visible:ring-offset-0 text-[15px] placeholder:text-muted-foreground"
    />
    <kbd class="text-xs text-muted-foreground">Esc</kbd>
  </div>
  <ul class="max-h-[50vh] overflow-y-auto p-1.5" role="listbox">
    {#each items as item, i (item.href)}
      <li role="option" aria-selected={i === selected}>
        <button
          class="w-full flex items-center gap-3 px-3 h-10 rounded-md text-left {i === selected ? 'bg-plate' : ''}"
          onmouseenter={() => (selected = i)}
          onclick={() => go(item)}
        >
          {#if item.env}
            <StatusBadge status={item.env.latest_deployment?.status ?? "none"} label={false} />
            <span class="engraved text-[17px] truncate">{item.label}</span>
          {:else}
            <span class="w-2.5"></span>
            <span class="text-sm truncate">{item.label}</span>
          {/if}
          <span class="ml-auto text-xs text-muted-foreground shrink-0">{item.hint}</span>
        </button>
      </li>
    {:else}
      <li class="px-3 py-6 text-sm text-muted-foreground text-center">Nothing matches “{query}”.</li>
    {/each}
  </ul>
</dialog>
