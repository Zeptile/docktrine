<script lang="ts">
  import { onMount } from "svelte";
  import { api, type AppSummary, ApiError } from "$lib/api";
  import { timeAgo, timeUntil, shortImage } from "$lib/format";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import Readouts from "./ui/Readouts.svelte";
  import Gauge from "./Gauge.svelte";
  import DeployHistory from "./DeployHistory.svelte";
  import { GitBranch, Clock, Search, ExternalLink, Rows3, LayoutGrid } from "lucide-svelte";

  type State = "all" | "failed" | "deploying" | "live" | "idle" | "previews";
  type View = "ledger" | "gauges";
  type Grouping = "app" | "server";

  let items = $state<AppSummary[]>([]);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let filter = $state("");
  let shown = $state<State>("all");
  let grouping = $state<Grouping>("app");
  let view = $state<View>(stored("docktrine.appsView", "ledger") as View);

  function stored(key: string, fallback: string) {
    try {
      return localStorage.getItem(key) ?? fallback;
    } catch {
      return fallback;
    }
  }
  $effect(() => {
    try {
      localStorage.setItem("docktrine.appsView", view);
    } catch {}
  });

  const views: { id: View; label: string; Icon: typeof Rows3 }[] = [
    { id: "ledger", label: "Ledger", Icon: Rows3 },
    { id: "gauges", label: "Gauges", Icon: LayoutGrid },
  ];

  const name = (i: AppSummary) => `${i.environment.app}/${i.environment.name}`;
  const status = (i: AppSummary) => i.latest_deployment?.status ?? "none";

  // Each environment lands in exactly one of these, worst first
  function stateOf(i: AppSummary): Exclude<State, "all" | "previews"> {
    const s = status(i);
    if (s === "failed") return "failed";
    if (s === "running" || s === "pending") return "deploying";
    return i.current_deployment ? "live" : "idle";
  }

  let counts = $derived.by(() => {
    const c = { all: items.length, failed: 0, deploying: 0, live: 0, idle: 0, previews: 0 };
    for (const i of items) {
      c[stateOf(i)]++;
      if (i.environment.is_preview) c.previews++;
    }
    return c;
  });
  let servers = $derived([...new Set(items.map((i) => i.environment.server))].sort());
  let appCount = $derived(new Set(items.map((i) => i.environment.app)).size);
  let deploying = $derived(items.filter((i) => stateOf(i) === "deploying"));

  // The state of the harbour in one sentence, worst news first
  let headline = $derived.by(() => {
    const n = items.length;
    const failed = items.filter((i) => stateOf(i) === "failed");
    const others = n - failed.length - deploying.length;
    const list = (xs: AppSummary[]) => (xs.length <= 2 ? xs.map(name).join(" and ") : `${xs.length} environments`);
    const parts: string[] = [];
    if (failed.length) parts.push(`${list(failed)} ${failed.length === 1 ? "failed its" : "failed their"} last deploy.`);
    if (deploying.length) parts.push(`${list(deploying)} ${deploying.length === 1 ? "is" : "are"} deploying.`);
    if (!parts.length) return n === 1 ? "The one environment is running." : `All ${n} environments are running.`;
    if (others > 0) parts.push(others === 1 ? "The other one is fine." : `The other ${others} are fine.`);
    return parts.join(" ");
  });

  let query = $derived(filter.trim().toLowerCase());
  let visible = $derived(
    items.filter((i) => {
      if (shown === "previews" ? !i.environment.is_preview : shown !== "all" && stateOf(i) !== shown) return false;
      if (!query) return true;
      const host = i.current_deployment?.spec?.hostnames?.join(" ") ?? "";
      return [name(i), i.environment.server, host, i.latest_deployment?.image ?? ""].some((s) => s.toLowerCase().includes(query));
    })
  );

  // Long-lived environments first, then previews; prod last among them reads
  // oddly, so the usual ladder goes dev, staging, prod
  const ladder = ["dev", "development", "test", "qa", "staging", "stage", "prod", "production"];
  const rank = (n: string) => (ladder.includes(n) ? ladder.indexOf(n) : ladder.length);
  function byEnv(a: AppSummary, b: AppSummary) {
    return (
      Number(a.environment.is_preview) - Number(b.environment.is_preview) ||
      rank(a.environment.name) - rank(b.environment.name) ||
      a.environment.name.localeCompare(b.environment.name)
    );
  }

  let groups = $derived.by(() => {
    const key = (i: AppSummary) => (grouping === "app" ? i.environment.app : i.environment.server);
    const map = new Map<string, AppSummary[]>();
    for (const i of visible) map.set(key(i), [...(map.get(key(i)) ?? []), i]);
    return [...map.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([title, envs]) => ({
        title,
        envs: envs.sort((a, b) =>
          grouping === "app" ? byEnv(a, b) : a.environment.app.localeCompare(b.environment.app) || byEnv(a, b)
        ),
      }));
  });

  onMount(() => {
    load();
    // Poll quickly while something deploys, slowly otherwise
    let timer: ReturnType<typeof setTimeout>;
    const tick = () => {
      timer = setTimeout(
        async () => {
          if (!document.hidden) await load();
          tick();
        },
        deploying.length ? 2000 : 15000
      );
    };
    tick();
    return () => clearTimeout(timer);
  });

  async function load() {
    try {
      items = await api.apps.list();
      error = null;
      loaded = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load apps";
    }
  }

  const href = (i: AppSummary) =>
    `#/apps/${encodeURIComponent(i.environment.app)}/${encodeURIComponent(i.environment.name)}`;
</script>

<!-- What the latest deploy did, in a few words -->
{#snippet release(item: AppSummary)}
  {@const latest = item.latest_deployment}
  {@const current = item.current_deployment}
  {#if latest?.status === "failed"}
    <span class="text-destructive font-medium">#{latest.id} failed</span>{#if current}<span class="text-muted-foreground">, #{current.id} serving</span>{/if}
  {:else if latest?.status === "running"}
    <span class="text-warning font-medium">#{latest.id} deploying</span>
  {:else if latest?.status === "pending"}
    <span class="text-warning font-medium">#{latest.id} queued</span>
  {:else if current}
    <span class="text-muted-foreground">#{current.id}</span>
    <span class="font-mono text-[12px]" title={current.image}>{shortImage(current.image).replace(`${item.environment.app}:`, ":")}</span>
  {:else}
    <span class="text-muted-foreground">Nothing running</span>
  {/if}
{/snippet}

{#snippet envName(item: AppSummary, withApp: boolean)}
  {@const env = item.environment}
  <span class="flex items-center gap-1.5 min-w-0">
    {#if withApp}<span class="engraved text-muted-foreground truncate">{env.app}/</span>{/if}
    <span class="engraved text-[16px] truncate">{env.name}</span>
    {#if env.is_preview}
      <GitBranch class="w-3.5 h-3.5 shrink-0 text-muted-foreground" aria-label="Preview of {env.branch}" />
    {/if}
  </span>
{/snippet}

{#if error}
  <Alert class="mb-6">{error}</Alert>
{/if}

{#if !loaded}
  {#if !error}<Loading label="Reading the gauges" />{/if}
{:else if items.length === 0}
  <h1 class="page-title text-[2.25rem]">Nothing is deployed yet.</h1>
  <div class="mt-6 max-w-xl text-muted-foreground space-y-3">
    <p>Deploy from a repository with the CLI:</p>
    <pre class="terminal rounded-md px-5 py-4 text-[13px]">docktrine init        <span class="opacity-60"># writes docktrine.yaml</span>
docktrine deploy dev</pre>
    <p>It shows up here as soon as the first deploy starts.</p>
  </div>
{:else}
  <header class="mb-6">
    <h1 class="font-slab font-bold text-[1.6rem] sm:text-[1.85rem] leading-[1.2] max-w-4xl text-balance">{headline}</h1>
    <p class="text-muted-foreground mt-1.5">
      {appCount} {appCount === 1 ? "app" : "apps"} in {items.length}
      {items.length === 1 ? "environment" : "environments"}, on {servers.length}
      {servers.length === 1 ? "server" : "servers"}.
    </p>
  </header>

  <Readouts
    bind:selected={shown}
    items={[
      { id: "all", label: "Environments", count: counts.all },
      { id: "live", label: "Live", count: counts.live, light: "light-live" },
      { id: "deploying", label: "Deploying", count: counts.deploying, light: "light-deploying" },
      { id: "failed", label: "Failed", count: counts.failed, light: "light-failed" },
      { id: "idle", label: "Not running", count: counts.idle, light: "light-off" },
      { id: "previews", label: "Previews", count: counts.previews },
    ]}
  />

  <div class="mt-5 mb-4 flex flex-wrap items-center gap-2">
    <div class="relative flex-1 min-w-48 sm:max-w-72">
      <Search class="w-4 h-4 absolute left-3 top-3 text-muted-foreground pointer-events-none" />
      <input bind:value={filter} type="search" placeholder="App, host, server or image" aria-label="Filter environments" class="input pl-9" />
    </div>
    <label class="flex items-center gap-2 text-sm text-muted-foreground">
      <span class="hidden sm:inline">Group by</span>
      <select bind:value={grouping} class="input w-auto h-10" aria-label="Group by">
        <option value="app">app</option>
        <option value="server">server</option>
      </select>
    </label>
    <div class="ml-auto flex rounded-md border border-input p-0.5 bg-card" role="group" aria-label="View">
      {#each views as { id, label, Icon }}
        <button
          class="h-8 px-2.5 flex items-center gap-1.5 rounded text-sm {view === id ? 'brass-fill' : 'border border-transparent text-muted-foreground hover:text-foreground'}"
          aria-pressed={view === id}
          onclick={() => (view = id)}
        >
          <Icon class="w-4 h-4" />
          <span class="hidden sm:inline">{label}</span>
        </button>
      {/each}
    </div>
  </div>

  {#if groups.length === 0}
    <p class="text-muted-foreground py-6">
      Nothing {shown === "all" ? "" : `${shown === "idle" ? "stopped" : shown} `}matches{query ? ` “${filter}”` : ""}.
    </p>
  {:else if view === "ledger"}
    <!-- The ledger: a row per environment, grouped, every fact in a column -->
    <div class="riveted px-2 sm:px-4 py-3 overflow-x-auto">
      <table class="w-full text-sm border-separate border-spacing-0">
        <thead>
          <tr class="text-left text-[11px] uppercase tracking-[0.12em] text-muted-foreground">
            <th class="font-medium pl-2 pr-3 pb-2 w-[52px]"><span class="sr-only">State</span></th>
            <th class="font-medium pr-4 pb-2">Environment</th>
            <th class="font-medium pr-4 pb-2">Release</th>
            <th class="font-medium pr-4 pb-2 hidden lg:table-cell">Address</th>
            <th class="font-medium pr-4 pb-2 hidden md:table-cell">{grouping === "app" ? "Server" : "App"}</th>
            <th class="font-medium pr-4 pb-2 hidden sm:table-cell">Last 12</th>
            <th class="font-medium pr-2 pb-2 text-right">Deployed</th>
          </tr>
        </thead>
        {#each groups as group (group.title)}
          <tbody>
            <tr>
              <th colspan="7" class="text-left pt-4 pb-1.5 px-2 border-b-2 border-border">
                <span class="engraved text-[1.15rem]">{group.title}</span>
                <span class="ml-2 font-normal text-[12px] text-muted-foreground">
                  {group.envs.length} {group.envs.length === 1 ? "environment" : "environments"}
                </span>
              </th>
            </tr>
            {#each group.envs as item (item.environment.id)}
              {@const env = item.environment}
              {@const latest = item.latest_deployment}
              {@const hosts = item.current_deployment?.spec?.hostnames ?? latest?.spec?.hostnames ?? []}
              <tr class="relative group hover:bg-plate/50 [&>td]:border-b [&>td]:border-border/70 [&:last-child>td]:border-0">
                <td class="pl-2 pr-3 py-1.5 align-middle">
                  <Gauge status={status(item)} size={38} />
                </td>
                <td class="pr-4 py-1.5 max-w-[16rem]">
                  <a href={href(item)} class="block after:absolute after:inset-0 focus-visible:ring-0 group-focus-within:underline">
                    {@render envName(item, grouping === "server")}
                  </a>
                  {#if env.expires_at}
                    <span class="text-[12px] text-muted-foreground inline-flex items-center gap-1" title="Removed in {timeUntil(env.expires_at)} unless redeployed">
                      <Clock class="w-3 h-3" /> {env.branch}, {timeUntil(env.expires_at)} left
                    </span>
                  {:else}
                    <span class="text-[12px] text-muted-foreground md:hidden">{env.server}</span>
                  {/if}
                </td>
                <td class="pr-4 py-1.5 text-[13px] whitespace-nowrap">{@render release(item)}</td>
                <td class="pr-4 py-1.5 hidden lg:table-cell max-w-[15rem]">
                  {#if hosts.length}
                    <a
                      href="https://{hosts[0]}"
                      target="_blank"
                      rel="noreferrer"
                      class="relative z-10 inline-flex items-center gap-1 max-w-full text-[13px] text-link hover:underline"
                      title={hosts.join("\n")}
                    >
                      <span class="truncate">{hosts[0]}</span>
                      <ExternalLink class="w-3 h-3 shrink-0 opacity-60" />
                    </a>
                    {#if hosts.length > 1}<span class="text-[12px] text-muted-foreground">+{hosts.length - 1}</span>{/if}
                  {:else}
                    <span class="text-muted-foreground">—</span>
                  {/if}
                </td>
                <td class="pr-4 py-1.5 hidden md:table-cell text-[13px] text-muted-foreground whitespace-nowrap">
                  {grouping === "app" ? env.server : env.app}
                </td>
                <td class="pr-4 py-1.5 hidden sm:table-cell"><DeployHistory history={item.history} /></td>
                <td class="pr-2 py-1.5 text-right text-[13px] text-muted-foreground whitespace-nowrap">
                  {#if latest}
                    <span title={new Date(latest.created_at).toLocaleString()}>{timeAgo(latest.created_at)}</span>
                    <span class="block text-[12px] truncate max-w-[9rem] ml-auto">{latest.created_by ?? ""}</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        {/each}
      </table>
    </div>
  {:else}
    <!-- The instrument board: a plate per group, a gauge per environment -->
    <div class="grid lg:grid-cols-2 gap-5 items-start">
      {#each groups as group (group.title)}
        <section class="riveted px-4 sm:px-5 pt-4 pb-4">
          <h2 class="engraved text-[1.2rem] leading-none flex items-baseline gap-2 px-1">
            {group.title}
            <span class="font-sans font-normal text-[12px] text-muted-foreground">
              {group.envs.length} {group.envs.length === 1 ? "environment" : "environments"}
            </span>
          </h2>
          <ul class="mt-3 grid grid-cols-[repeat(auto-fill,minmax(8.5rem,1fr))] gap-2">
            {#each group.envs as item (item.environment.id)}
              <li>
                <a
                  href={href(item)}
                  class="h-full flex flex-col items-center text-center rounded-md px-2 pt-3 pb-2.5 bg-plate/50 border border-transparent hover:border-brass/60 hover:bg-plate"
                >
                  <Gauge status={status(item)} size={72} />
                  <span class="mt-1.5 max-w-full">{@render envName(item, grouping === "server")}</span>
                  <span class="text-[12px] max-w-full truncate">{@render release(item)}</span>
                  <span class="mt-auto pt-2"><DeployHistory history={item.history} slots={10} /></span>
                </a>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    </div>
  {/if}
{/if}
