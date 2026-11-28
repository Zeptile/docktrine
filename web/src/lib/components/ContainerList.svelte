<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Container, type Server, ApiError } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import { toast } from "$lib/toast";
  import { confirmAction } from "$lib/confirm";
  import { can } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import Readouts from "./ui/Readouts.svelte";
  import { Play, Square, RotateCcw, RefreshCw, Search, SquareTerminal } from "lucide-svelte";

  type State = "all" | "running" | "unhealthy" | "crashed" | "stopped" | "docktrine";
  type Grouping = "owner" | "server" | "none";

  // A container and the node it was found on
  interface Row extends Container {
    server: string;
  }

  const ALL = "*";

  let rows = $state<Row[]>([]);
  let servers = $state<Server[]>([]);
  let selectedServer = $state("");
  let failures = $state<{ server: string; error: string }[]>([]);
  let loading = $state(false);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let filter = $state("");
  let shown = $state<State>("all");
  let grouping = $state<Grouping>("owner");
  let busy = $state<Record<string, boolean>>({});

  let many = $derived(selectedServer === ALL);

  const name = (c: Container) => c.name.replace(/^\//, "") || c.id.substring(0, 12);
  const label = (c: Container, key: string) => c.labels?.[key];

  function stateOf(c: Container): Exclude<State, "all" | "docktrine"> {
    if (c.status === "running") return c.state.Health?.Status === "unhealthy" ? "unhealthy" : "running";
    if (c.status === "dead" || (c.status === "exited" && c.state.ExitCode !== 0) || c.status === "restarting") {
      return "crashed";
    }
    return "stopped";
  }

  // Who the container belongs to: a Docktrine environment, a compose
  // project, or nobody
  function owner(c: Container): { kind: "docktrine" | "compose" | "none"; title: string } {
    const app = label(c, "docktrine.app");
    if (app) return { kind: "docktrine", title: app };
    const project = label(c, "com.docker.compose.project");
    if (project) return { kind: "compose", title: project };
    return { kind: "none", title: "Standalone" };
  }

  let counts = $derived.by(() => {
    const n = { all: rows.length, running: 0, unhealthy: 0, crashed: 0, stopped: 0, docktrine: 0 };
    for (const c of rows) {
      n[stateOf(c)]++;
      if (owner(c).kind === "docktrine") n.docktrine++;
    }
    return n;
  });

  let query = $derived(filter.trim().toLowerCase());
  let visible = $derived(
    rows.filter((c) => {
      if (shown === "docktrine" ? owner(c).kind !== "docktrine" : shown !== "all" && stateOf(c) !== shown) return false;
      if (!query) return true;
      return [name(c), c.image, owner(c).title, c.server, ports(c)].some((s) => s.toLowerCase().includes(query));
    })
  );

  // Trouble first, then running, then the rest; by name within each
  const order = { crashed: 0, unhealthy: 1, running: 2, stopped: 3 };
  const kindOrder = { docktrine: 0, compose: 1, none: 2 };
  let groups = $derived.by(() => {
    const sorted = [...visible].sort((a, b) => order[stateOf(a)] - order[stateOf(b)] || name(a).localeCompare(name(b)));
    if (grouping === "none") return [{ title: "", kind: "", rows: sorted }];
    const map = new Map<string, { title: string; kind: string; rows: Row[] }>();
    for (const c of sorted) {
      const o = grouping === "owner" ? owner(c) : { kind: "server", title: c.server };
      const key = `${o.kind}:${o.title}`;
      if (!map.has(key)) map.set(key, { ...o, rows: [] });
      map.get(key)!.rows.push(c);
    }
    return [...map.values()].sort(
      (a, b) =>
        (kindOrder[a.kind as keyof typeof kindOrder] ?? 0) - (kindOrder[b.kind as keyof typeof kindOrder] ?? 0) ||
        a.title.localeCompare(b.title)
    );
  });

  onMount(() => {
    (async () => {
      try {
        servers = await api.servers.list();
        selectedServer = servers.find((s) => s.is_default)?.name ?? servers[0]?.name ?? "";
      } catch (e) {
        error = e instanceof ApiError ? e.message : "Couldn't load servers";
      }
      await load();
    })();
    const timer = setInterval(() => !document.hidden && !loading && load(), 15000);
    return () => clearInterval(timer);
  });

  async function load() {
    loading = true;
    error = null;
    const targets = many ? servers.map((s) => s.name) : [selectedServer];
    const results = await Promise.allSettled(targets.map((s) => api.containers.list(s || undefined)));
    const next: Row[] = [];
    const failed: typeof failures = [];
    results.forEach((r, i) => {
      if (r.status === "fulfilled") next.push(...(r.value ?? []).map((c) => ({ ...c, server: targets[i] })));
      else failed.push({ server: targets[i], error: r.reason instanceof ApiError ? r.reason.message : "unreachable" });
    });
    rows = next;
    failures = failed;
    // One node that won't answer is an error; one of several is a footnote
    if (!many && failed.length) error = failed[0].error;
    loaded = !(!many && failed.length);
    loading = false;
  }

  function changeServer() {
    if (!many && grouping === "server") grouping = "owner";
    loaded = false;
    rows = [];
    load();
  }

  async function act(c: Row, verb: string, action: () => Promise<unknown>) {
    busy[c.id] = true;
    error = null;
    try {
      await action();
      toast(`${name(c)} ${verb}`);
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : `Couldn't change ${name(c)}`;
    } finally {
      busy[c.id] = false;
    }
  }

  async function stop(c: Row) {
    const o = owner(c);
    const ok = await confirmAction({
      title: `Stop ${name(c)}?`,
      message:
        o.kind === "docktrine"
          ? `It belongs to ${o.title}, whose hostnames stop answering until it's started again or redeployed.`
          : "Anything it serves stops answering until it's started again.",
      confirmLabel: "Stop container",
      danger: true,
    });
    if (ok) act(c, "stopped", () => api.containers.stop(c.id, c.server || undefined));
  }

  function uptime(c: Container) {
    const s = c.state;
    if (c.status === "running") return `up ${timeAgo(s.StartedAt).replace(" ago", "")}`;
    if (c.status === "exited") {
      const why = s.OOMKilled ? "out of memory" : s.ExitCode === 0 ? "stopped" : `exit ${s.ExitCode}`;
      return `${why}, ${timeAgo(s.FinishedAt)}`;
    }
    if (c.status === "created") return `created ${timeAgo(c.created)}`;
    return c.status;
  }

  const lights = { running: "light-live", unhealthy: "light-deploying", crashed: "light-failed", stopped: "light-off" };
  const tones = { running: "text-success", unhealthy: "text-warning", crashed: "text-destructive", stopped: "text-muted-foreground" };

  // registry.example.com/team/app:tag → [registry.example.com/team/, app, tag]
  function imageParts(image: string) {
    const at = image.indexOf("@");
    const ref = at >= 0 ? image.slice(0, at) : image;
    const slash = ref.lastIndexOf("/");
    const colon = ref.lastIndexOf(":");
    const tag = at >= 0 ? image.slice(at + 1, at + 19) : colon > slash ? ref.slice(colon + 1) : "latest";
    const repo = colon > slash ? ref.slice(0, colon) : ref;
    return [repo.slice(0, slash + 1), repo.slice(slash + 1), tag];
  }

  function ports(c: Container) {
    const seen = new Set<string>();
    for (const p of c.ports ?? []) {
      seen.add(p.PublicPort ? `${p.PublicPort}→${p.PrivatePort}` : `${p.PrivatePort}`);
    }
    return [...seen].join(" ");
  }
</script>

<div class="flex flex-wrap items-end justify-between gap-4 mb-6">
  <div>
    <h1 class="page-title">Containers</h1>
    <p class="text-muted-foreground mt-1">
      Everything on {many ? "every node" : (selectedServer || "the node")}, including containers Docktrine didn't start.
    </p>
  </div>
</div>

{#if error}
  <Alert class="mb-6">{error}</Alert>
{/if}

<Readouts
  bind:selected={shown}
  items={[
    { id: "all", label: "Containers", count: counts.all },
    { id: "running", label: "Running", count: counts.running, light: "light-live" },
    { id: "unhealthy", label: "Unhealthy", count: counts.unhealthy, light: "light-deploying" },
    { id: "crashed", label: "Crashed", count: counts.crashed, light: "light-failed" },
    { id: "stopped", label: "Stopped", count: counts.stopped, light: "light-off" },
    { id: "docktrine", label: "From Docktrine", count: counts.docktrine },
  ]}
/>

<div class="mt-5 mb-4 flex flex-wrap items-center gap-2">
  <div class="relative flex-1 min-w-48 sm:max-w-72">
    <Search class="w-4 h-4 absolute left-3 top-3 text-muted-foreground pointer-events-none" />
    <input bind:value={filter} type="search" placeholder="Name, image, project or port" aria-label="Filter containers" class="input pl-9" />
  </div>
  {#if servers.length > 1}
    <select bind:value={selectedServer} onchange={changeServer} class="input w-auto" aria-label="Server">
      <option value={ALL}>All servers</option>
      {#each servers as server}
        <option value={server.name}>{server.name}</option>
      {/each}
    </select>
  {/if}
  <label class="flex items-center gap-2 text-sm text-muted-foreground">
    <span class="hidden sm:inline">Group by</span>
    <select bind:value={grouping} class="input w-auto" aria-label="Group by">
      <option value="owner">owner</option>
      {#if many}<option value="server">server</option>{/if}
      <option value="none">nothing</option>
    </select>
  </label>
  <Button variant="ghost" size="icon" class="ml-auto" onclick={load} disabled={loading} title="Refresh" aria-label="Refresh">
    <RefreshCw class={loading ? "animate-spin" : ""} />
  </Button>
</div>

{#if many && failures.length}
  <p class="mb-4 text-sm text-warning">
    {#each failures as f, i}{i ? "; " : ""}{f.server} didn't answer: {f.error}{/each}
  </p>
{/if}

{#if !loaded && loading}
  <Loading label="Asking {many ? 'every node' : selectedServer || 'the node'} for its containers" />
{:else if loaded && rows.length === 0}
  <p class="text-muted-foreground">No containers on {many ? "any node" : selectedServer}.</p>
{:else if loaded}
  {#if visible.length === 0}
    <p class="text-muted-foreground py-6">Nothing matches{query ? ` “${filter}”` : ""}.</p>
  {:else}
    <div class="riveted px-2 sm:px-4 py-3 overflow-x-auto">
      <table class="w-full text-sm border-separate border-spacing-0">
        <thead>
          <tr class="text-left text-[11px] uppercase tracking-[0.12em] text-muted-foreground">
            <th class="font-medium pl-2 pr-3 pb-2 w-6"><span class="sr-only">State</span></th>
            <th class="font-medium pr-4 pb-2">Name</th>
            <th class="font-medium pr-4 pb-2 hidden md:table-cell">Image</th>
            <th class="font-medium pr-4 pb-2">State</th>
            <th class="font-medium pr-4 pb-2 hidden lg:table-cell">Ports</th>
            {#if many && grouping !== "server"}<th class="font-medium pr-4 pb-2 hidden sm:table-cell">Server</th>{/if}
            {#if $can("deployer")}<th class="pb-2"><span class="sr-only">Actions</span></th>{/if}
          </tr>
        </thead>
        {#each groups as group (group.kind + group.title)}
          <tbody>
            {#if group.title}
              <tr>
                <th colspan="7" class="text-left pt-4 pb-1.5 px-2 border-b-2 border-border font-normal">
                  <span class="engraved text-[1.1rem]">{group.title}</span>
                  <span class="ml-2 text-[12px] text-muted-foreground">
                    {group.kind === "compose" ? "compose project, " : group.kind === "docktrine" ? "Docktrine, " : ""}{group.rows.length}
                    {group.rows.length === 1 ? "container" : "containers"}
                  </span>
                </th>
              </tr>
            {/if}
            {#each group.rows as c (c.server + c.id)}
              {@const st = stateOf(c)}
              {@const [registry, repo, tag] = imageParts(c.image)}
              {@const health = c.state.Health?.Status}
              {@const service = label(c, "com.docker.compose.service")}
              {@const app = label(c, "docktrine.app")}
              {@const env = label(c, "docktrine.env")}
              <tr class="hover:bg-plate/50 [&>td]:border-b [&>td]:border-border/70 [&:last-child>td]:border-0 [&>td]:py-1.5">
                <td class="pl-2 pr-3"><span class="light {lights[st]} block" title={c.status}></span></td>
                <td class="pr-4 max-w-[18rem]">
                  <div class="font-medium truncate" title={name(c)}>{name(c)}</div>
                  <div class="text-[12px] text-muted-foreground truncate">
                    {#if env}
                      <a href="#/apps/{encodeURIComponent(app ?? '')}/{encodeURIComponent(env)}" class="engraved text-[13px] text-brass hover:underline"
                        >{grouping === "owner" ? env : `${app}/${env}`}</a
                      >{#if service}, {service}{/if}
                    {:else if service}service {service}{:else}<span class="font-mono">{c.id.slice(0, 12)}</span>{/if}
                    <span class="md:hidden font-mono">· {repo}:{tag}</span>
                  </div>
                </td>
                <td class="pr-4 hidden md:table-cell max-w-[20rem]">
                  <div class="font-mono text-[12.5px] truncate" title={c.image}>
                    <span class="text-muted-foreground">{registry}</span>{repo}<span class="text-brass">:{tag}</span>
                  </div>
                </td>
                <td class="pr-4 whitespace-nowrap">
                  <div class="text-[13px] {tones[st]}">{uptime(c)}</div>
                  <div class="text-[12px] text-muted-foreground">
                    {#if health && health !== "none"}<span class={health === "unhealthy" ? "text-warning" : ""}>{health}</span
                      >{/if}{#if c.restarts}{health && health !== "none" ? ", " : ""}<span class="text-warning" title="Restarted by Docker"
                        >{c.restarts} {c.restarts === 1 ? "restart" : "restarts"}</span
                      >{/if}
                  </div>
                </td>
                <td class="pr-4 hidden lg:table-cell">
                  <div class="flex flex-wrap gap-1">
                    {#each ports(c).split(" ").filter(Boolean) as p}
                      <span class="font-mono text-[11.5px] px-1.5 py-px rounded bg-plate/70 border border-border">{p}</span>
                    {/each}
                  </div>
                </td>
                {#if many && grouping !== "server"}
                  <td class="pr-4 hidden sm:table-cell text-[13px] text-muted-foreground whitespace-nowrap">{c.server}</td>
                {/if}
                {#if $can("deployer")}
                  <td class="pr-1 text-right whitespace-nowrap">
                    {#if c.status === "running"}
                      <a
                        href="#/shell/{encodeURIComponent(c.server)}/{encodeURIComponent(c.id)}"
                        class="inline-flex items-center justify-center h-8 px-2.5 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground [&_svg]:size-4"
                        title="Open a shell"
                        aria-label="Open a shell in {name(c)}"
                      >
                        <SquareTerminal />
                      </a>
                    {/if}
                    {#if c.status === "running" || c.status === "restarting"}
                      <Button size="sm" variant="ghost" disabled={busy[c.id]} title="Restart" aria-label="Restart {name(c)}"
                        onclick={() => act(c, "restarted", () => api.containers.restart(c.id, c.server || undefined))}>
                        <RotateCcw />
                      </Button>
                      <Button size="sm" variant="ghost" class="hover:text-destructive" disabled={busy[c.id]} title="Stop" aria-label="Stop {name(c)}" onclick={() => stop(c)}>
                        <Square />
                      </Button>
                    {:else}
                      <Button size="sm" variant="ghost" disabled={busy[c.id]} title="Start" aria-label="Start {name(c)}"
                        onclick={() => act(c, "started", () => api.containers.start(c.id, c.server || undefined))}>
                        <Play />
                      </Button>
                    {/if}
                  </td>
                {/if}
              </tr>
            {/each}
          </tbody>
        {/each}
      </table>
    </div>
  {/if}
{/if}
