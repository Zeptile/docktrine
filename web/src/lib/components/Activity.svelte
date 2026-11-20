<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Deployment, ApiError } from "$lib/api";
  import { duration, shortImage } from "$lib/format";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import StatusBadge from "./ui/StatusBadge.svelte";
  import { Search } from "lucide-svelte";

  let deployments = $state<Deployment[]>([]);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let filter = $state("");

  let query = $derived(filter.trim().toLowerCase());
  let visible = $derived(
    query
      ? deployments.filter((d) =>
          [`${d.app}/${d.environment}`, d.created_by ?? "", d.image, d.status].some((f) => f.toLowerCase().includes(query))
        )
      : deployments
  );

  // The log, a page per day
  let days = $derived.by(() => {
    const out: { label: string; entries: Deployment[] }[] = [];
    const byTime = [...visible].sort((a, b) => b.created_at.localeCompare(a.created_at));
    for (const d of byTime) {
      const label = dayLabel(new Date(d.created_at));
      if (out[out.length - 1]?.label !== label) out.push({ label, entries: [] });
      out[out.length - 1].entries.push(d);
    }
    return out;
  });

  let people = $derived(new Set(deployments.map((d) => d.created_by).filter(Boolean)).size);

  function dayLabel(date: Date) {
    const today = new Date();
    const yesterday = new Date(Date.now() - 864e5);
    if (date.toDateString() === today.toDateString()) return "Today";
    if (date.toDateString() === yesterday.toDateString()) return "Yesterday";
    return date.toLocaleDateString(undefined, {
      weekday: "short",
      month: "short",
      day: "numeric",
      year: date.getFullYear() === today.getFullYear() ? undefined : "numeric",
    });
  }

  function time(iso: string) {
    return new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  }

  onMount(() => {
    load();
    const timer = setInterval(() => !document.hidden && load(), 10000);
    return () => clearInterval(timer);
  });

  async function load() {
    try {
      deployments = await api.deployments.list(200);
      error = null;
      loaded = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load the activity";
    }
  }
</script>

<header class="flex flex-wrap items-end justify-between gap-4 mb-10">
  <div>
    <h1 class="page-title">Activity</h1>
    <p class="text-muted-foreground mt-1.5">
      Every deploy, newest first{#if loaded && deployments.length}, by {people} {people === 1 ? "person or key" : "people and keys"}{/if}.
    </p>
  </div>
  {#if deployments.length > 8}
    <div class="relative w-full sm:w-64">
      <Search class="w-4 h-4 absolute left-3 top-3 text-muted-foreground pointer-events-none" />
      <input bind:value={filter} type="search" placeholder="App, person or status" aria-label="Filter activity" class="input pl-9" />
    </div>
  {/if}
</header>

{#if error}
  <Alert class="mb-6">{error}</Alert>
{/if}

{#if !loaded}
  {#if !error}<Loading label="Loading activity" />{/if}
{:else if deployments.length === 0}
  <p class="text-muted-foreground">No deploys yet. They're listed here as soon as one starts.</p>
{:else if visible.length === 0}
  <p class="text-muted-foreground">Nothing matches “{filter}”.</p>
{:else}
  <div class="space-y-10">
    {#each days as day (day.label)}
      <section class="grid md:grid-cols-[8.5rem_1fr] gap-x-8 gap-y-2">
        <h2 class="engraved text-xl text-muted-foreground md:pt-2.5 md:sticky md:top-6 self-start">{day.label}</h2>
        <ol class="riveted px-4 sm:px-6 py-1.5">
          {#each day.entries as d (d.id)}
            <li class="border-b last:border-0">
              <a
                href="#/apps/{encodeURIComponent(d.app)}/{encodeURIComponent(d.environment)}"
                class="group grid grid-cols-[4.5rem_auto_1fr] sm:grid-cols-[4.5rem_auto_minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-0.5 py-3 px-1 -mx-1 hover:bg-plate/60"
              >
                <span class="text-[13px] text-muted-foreground tabular-nums whitespace-nowrap">{time(d.created_at)}</span>
                <span class="self-center"><StatusBadge status={d.status} label={false} /></span>
                <span class="min-w-0">
                  <span class="engraved text-[18px] group-hover:text-brass">{d.app}/{d.environment}</span>
                  <span class="text-sm text-muted-foreground">
                    {#if d.status === "failed"}
                      <span class="text-destructive">failed</span>
                    {:else if d.status === "running"}
                      <span class="text-warning">deploying</span>
                    {:else if d.status === "pending"}
                      <span class="text-warning">queued</span>
                    {/if}
                    {d.created_by ? `by ${d.created_by}` : ""}
                  </span>
                  {#if d.status === "failed" && d.message}
                    <span class="block text-[13px] text-muted-foreground truncate">{d.message}</span>
                  {/if}
                </span>
                <span class="col-start-3 sm:col-start-auto text-[13px] text-muted-foreground sm:text-right min-w-0 flex sm:block gap-3">
                  <span class="font-mono text-[12px] text-foreground truncate" title={d.image}><span class="nixie text-[12px] mr-1.5">{d.id}</span>{shortImage(d.image)}</span>
                  {#if d.finished_at}<span class="sm:block">took {duration(d.created_at, d.finished_at)}</span>{/if}
                </span>
              </a>
            </li>
          {/each}
        </ol>
      </section>
    {/each}
  </div>
{/if}
