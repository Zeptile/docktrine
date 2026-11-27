<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import {
    api,
    type EnvironmentDetail,
    type Deployment,
    ApiError,
  } from "$lib/api";
  import { timeAgo, timeUntil, duration, shortImage } from "$lib/format";
  import { confirmAction } from "$lib/confirm";
  import { toast } from "$lib/toast";
  import { can } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import StatusBadge from "./ui/StatusBadge.svelte";
  import Gauge from "./Gauge.svelte";
  import SecretsPanel from "./SecretsPanel.svelte";
  import LogsPanel from "./LogsPanel.svelte";
  import {
    ExternalLink,
    RefreshCw,
    RotateCcw,
    Trash2,
    GitBranch,
    Clock,
    SlidersHorizontal,
  } from "lucide-svelte";

  export let app: string;
  export let env: string;
  // From the URL: #/apps/<app>/<env>/<tab>
  export let tab: string | undefined = undefined;

  const tabs = [
    { id: "history", label: "Deployments" },
    { id: "logs", label: "Logs" },
    { id: "secrets", label: "Secrets" },
  ];
  $: activeTab = tabs.some((t) => t.id === tab) ? tab : "history";
  $: base = `#/apps/${encodeURIComponent(app)}/${encodeURIComponent(env)}`;

  let detail: EnvironmentDetail | null = null;
  let error: string | null = null;

  // Selected deployment whose log is shown; refreshed while it runs
  let selectedId: number | null = null;
  let selected: Deployment | null = null;
  let logBox: HTMLPreElement;

  let acting = false;
  let showDestroy = false;
  let destroyConfirm = "";
  let destroyPurge = false;

  let timer: ReturnType<typeof setTimeout> | undefined;

  $: environment = detail?.environment;
  $: deployments = detail?.deployments ?? [];
  $: current = deployments.find(
    (d) => d.id === environment?.current_deployment_id
  );
  $: latest = deployments[0];
  $: active = deployments.some(
    (d) => d.status === "running" || d.status === "pending"
  );
  $: failedSinceCurrent = latest?.status === "failed" && latest.id !== current?.id ? latest : null;

  onMount(() => {
    load();
  });
  onDestroy(() => clearTimeout(timer));

  async function load() {
    clearTimeout(timer);
    try {
      error = null;
      detail = await api.apps.get(app, env);
      if (selectedId === null && detail.deployments.length > 0) {
        selectedId = detail.deployments[0].id;
      }
      if (selectedId !== null) await loadSelected();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load this environment";
    }
    // Keep refreshing while a deployment is in progress
    if (active || isRunning(selected)) timer = setTimeout(load, 1500);
  }

  async function loadSelected() {
    if (selectedId === null) return;
    const atBottom =
      !logBox ||
      logBox.scrollHeight - logBox.scrollTop - logBox.clientHeight < 40;
    selected = await api.deployments.get(selectedId);
    if (atBottom && logBox) {
      requestAnimationFrame(() => (logBox.scrollTop = logBox.scrollHeight));
    }
  }

  function isRunning(d: Deployment | null | undefined) {
    return d?.status === "running" || d?.status === "pending";
  }

  async function select(id: number) {
    if (activeTab !== "history") location.hash = base;
    selectedId = id;
    selected = null;
    await loadSelected();
    if (isRunning(selected)) load();
  }

  async function start(action: () => Promise<Deployment>, done: string) {
    acting = true;
    error = null;
    try {
      const dep = await action();
      selectedId = dep.id;
      location.hash = base;
      toast(`${done}: deployment #${dep.id} started`);
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't start the deployment";
    } finally {
      acting = false;
    }
  }

  async function redeploy() {
    if (!current?.spec) return;
    const spec = current.spec;
    const ok = await confirmAction({
      title: `Redeploy ${env}?`,
      message: `Starts a new deployment of #${current.id} (${shortImage(current.image)}) with the current secrets, without rebuilding. Use it to apply secret changes.`,
      confirmLabel: "Redeploy",
    });
    if (ok) start(() => api.deployments.create(spec), "Redeploying");
  }

  async function rollbackTo(d: Deployment) {
    const ok = await confirmAction({
      title: `Roll ${env} back to #${d.id}?`,
      message: `Deploys ${shortImage(d.image)} again, as it was deployed ${timeAgo(d.created_at)}. The running version keeps serving until the old one is healthy.`,
      confirmLabel: "Roll back",
    });
    if (ok) start(() => api.apps.rollback(app, env, d.id), "Rolling back");
  }

  async function destroy() {
    acting = true;
    error = null;
    try {
      await api.apps.destroy(app, env, destroyPurge);
      toast(`${app}/${env} destroyed`);
      location.hash = "#/apps";
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't destroy the environment";
      acting = false;
    }
  }

  function containerStatus(state: string) {
    if (state === "running") return "succeeded";
    if (state === "exited" || state === "dead") return "failed";
    return "running";
  }
</script>

<header class="riveted px-6 py-6 sm:px-8 flex flex-wrap items-center gap-x-7 gap-y-4">
  <Gauge status={latest?.status ?? "none"} size={124} sweep class="shrink-0" />
  <div class="min-w-0 flex-1">
    <a href="#/apps" class="engraved text-lg text-muted-foreground hover:text-brass">{app} /</a>
    <h1 class="font-slab font-bold text-[2.5rem] sm:text-[3rem] leading-none mt-0.5 break-all flex flex-wrap items-center gap-x-5 gap-y-2">
      {env}
      {#if latest && isRunning(latest)}
        <StatusBadge status={latest.status} class="font-sans font-normal text-sm" />
      {/if}
    </h1>
    {#if environment}
      <p class="text-sm text-muted-foreground mt-3 flex flex-wrap items-center gap-x-5 gap-y-1.5">
        <span>on <a href="#/servers" class="text-foreground hover:underline">{environment.server}</a></span>
        {#if environment.branch}
          <span class="inline-flex items-center gap-1.5"><GitBranch class="w-3.5 h-3.5" /> {environment.branch}</span>
        {/if}
        {#if environment.expires_at}
          <span class="inline-flex items-center gap-1.5" title={new Date(environment.expires_at).toLocaleString()}>
            <Clock class="w-3.5 h-3.5" />
            {timeUntil(environment.expires_at) === "expired"
              ? "Expired, removed at the next cleanup"
              : `Removed in ${timeUntil(environment.expires_at)} unless redeployed`}
          </span>
        {/if}
      </p>
    {/if}
  </div>

  <div class="flex items-center gap-2 shrink-0">
    <Button variant="ghost" size="icon" onclick={load} title="Refresh" aria-label="Refresh">
      <RefreshCw />
    </Button>
    {#if $can("deployer")}
      {#if current?.spec || latest?.spec}
        <a
          href="{base}/configure"
          class="inline-flex items-center justify-center gap-2 h-9 px-3.5 rounded-md text-sm font-medium border border-input bg-card hover:bg-muted hover:border-foreground/30 [&_svg]:size-4"
          title="Change the image, variables, hostnames and more, then deploy"
        >
          <SlidersHorizontal /> Configure
        </a>
      {/if}
      <Button
        variant="outline"
        onclick={redeploy}
        disabled={acting || active || !current?.spec}
        title={active ? "A deployment is in progress" : "Deploy the running version again, without rebuilding"}
      >
        <RotateCcw /> Redeploy
      </Button>
    {/if}
  </div>
</header>

{#if error}
  <Alert class="mt-6">{error}</Alert>
{/if}

{#if detail}
  {#if failedSinceCurrent}
    <Alert kind="warning" class="mt-8">
      <span class="font-medium">Deploy #{failedSinceCurrent.id} failed {timeAgo(failedSinceCurrent.finished_at ?? failedSinceCurrent.created_at)}{failedSinceCurrent.message ? ":" : "."}</span>
      {#if failedSinceCurrent.message}{failedSinceCurrent.message}.{/if}
      {current ? `#${current.id} is still serving.` : "Nothing is running."}
      {#if selectedId !== failedSinceCurrent.id || activeTab !== "history"}
        <button class="underline underline-offset-2 font-medium" onclick={() => failedSinceCurrent && select(failedSinceCurrent.id)}>
          See its log
        </button>
      {/if}
    </Alert>
  {/if}

  <dl class="mt-8 grid md:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)] riveted px-6 sm:px-8 md:divide-x divide-y md:divide-y-0">
    <div class="py-4 md:pr-6 min-w-0">
      <dt class="text-[13px] text-muted-foreground mb-1.5">Running</dt>
      <dd>
        {#if current}
          <p class="font-mono text-[13px] truncate" title={current.image}>{shortImage(current.image)}</p>
          <p class="text-sm text-muted-foreground mt-1">
            #{current.id}, deployed {timeAgo(current.created_at)}{current.created_by ? ` by ${current.created_by}` : ""}
          </p>
        {:else}
          <p class="text-sm text-muted-foreground">Nothing yet.</p>
        {/if}
      </dd>
    </div>

    <div class="py-4 md:px-6 min-w-0">
      <dt class="text-[13px] text-muted-foreground mb-1.5">Hostnames</dt>
      <dd>
        {#if detail.hostnames.length === 0}
          <p class="text-sm text-muted-foreground">None; not exposed through the gateway.</p>
        {:else}
          <ul class="space-y-1 text-sm">
            {#each detail.hostnames as host}
              <li class="min-w-0">
                <a href="https://{host}" target="_blank" rel="noreferrer" class="text-link hover:underline underline-offset-2 inline-flex items-center gap-1.5 max-w-full">
                  <span class="truncate">{host}</span> <ExternalLink class="w-3.5 h-3.5 shrink-0" />
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </dd>
    </div>

    <div class="py-4 md:pl-6 min-w-0">
      <dt class="text-[13px] text-muted-foreground mb-1.5">Containers</dt>
      <dd>
        {#if detail.containers_error}
          <p class="text-sm text-destructive break-words">Couldn't reach {environment?.server}: {detail.containers_error}</p>
        {:else if !detail.containers?.length}
          <p class="text-sm text-muted-foreground">None running.</p>
        {:else}
          <ul class="space-y-1.5 text-sm">
            {#each detail.containers as c (c.id)}
              <li class="flex items-center gap-2.5 min-w-0">
                <StatusBadge status={containerStatus(c.state)} label={false} />
                <span class="truncate" title={c.image}>{c.service ?? c.name}</span>
                <span class="ml-auto text-xs text-muted-foreground shrink-0">{c.status}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </dd>
    </div>
  </dl>

  <nav class="mt-10 border-b flex gap-6 mb-6" aria-label="Environment">
    {#each tabs as t}
      <a
        href={t.id === "history" ? base : `${base}/${t.id}`}
        aria-current={activeTab === t.id ? "page" : undefined}
        class="relative py-2.5 text-sm font-medium {activeTab === t.id ? 'text-foreground' : 'text-muted-foreground hover:text-foreground'}"
      >
        {t.label}
        {#if activeTab === t.id}
          <span class="absolute inset-x-0 -bottom-px h-[3px] bg-brass"></span>
        {/if}
      </a>
    {/each}
  </nav>

  {#if activeTab === "history"}
    <div class="grid lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-6 items-start">
      {#if deployments.length === 0}
        <p class="text-sm text-muted-foreground">No deployments yet.</p>
      {:else}
        <!-- A track line through each deployment's light, newest on top -->
        <ol class="relative max-h-[36rem] overflow-y-auto">
          {#each deployments as d, i (d.id)}
            <li class="relative">
              {#if i < deployments.length - 1}
                <span class="absolute left-[1.05rem] top-7 -bottom-1 w-px bg-border" aria-hidden="true"></span>
              {/if}
              <button
                class="w-full text-left flex items-start gap-3.5 pl-3 pr-3 py-3 rounded-md focus-visible:ring-inset focus-visible:ring-offset-0
                  {selectedId === d.id ? 'bg-plate' : 'hover:bg-plate/50'}"
                aria-pressed={selectedId === d.id}
                onclick={() => select(d.id)}
              >
                <span class="pt-1.5 relative"><StatusBadge status={d.status} label={false} /></span>
                <span class="flex-1 min-w-0">
                  <span class="flex items-baseline gap-2.5">
                    <span class="nixie">{d.id}</span>
                    <span class="font-mono text-[12.5px] truncate" title={d.image}>{shortImage(d.image)}</span>
                  </span>
                  <span class="block text-[13px] text-muted-foreground mt-1 truncate">
                    {timeAgo(d.created_at)}{d.created_by ? `, ${d.created_by}` : ""}
                  </span>
                </span>
                {#if d.id === environment?.current_deployment_id}
                  <span class="mt-0.5 text-[12px] font-semibold text-success border border-success/50 rounded-sm px-1.5">live</span>
                {:else if d.status === "failed"}
                  <span class="mt-0.5 text-[12px] text-destructive">failed</span>
                {:else if isRunning(d)}
                  <span class="mt-0.5 text-[12px] text-warning">{d.status === "pending" ? "queued" : "deploying"}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ol>
      {/if}

      <div class="rounded-lg overflow-hidden flex flex-col min-h-[20rem] terminal">
        {#if selected}
          <div class="px-5 py-3.5 border-b border-white/10 flex flex-wrap items-center justify-between gap-3 font-sans">
            <div class="min-w-0">
              <div class="text-sm font-semibold flex items-center gap-2.5">
                Deployment <span class="nixie">{selected.id}</span>
                <StatusBadge status={selected.status} class="text-[12px] font-normal" />
              </div>
              <div class="text-xs opacity-70 mt-1">
                {new Date(selected.created_at).toLocaleString()}, {isRunning(selected) ? "running for" : "took"} {duration(selected.created_at, selected.finished_at)}
              </div>
            </div>
            {#if $can("deployer") && selected.status === "succeeded" && selected.id !== environment?.current_deployment_id}
              <Button size="sm" variant="secondary" disabled={acting || active} onclick={() => selected && rollbackTo(selected)}>
                <RotateCcw /> Roll back to #{selected.id}
              </Button>
            {/if}
          </div>
          {#if selected.message}
            <div class="px-5 py-2.5 text-[13px] font-sans border-b border-white/10 {selected.status === 'failed' ? 'text-[rgb(240_114_122)]' : 'opacity-70'}">
              {selected.message}
            </div>
          {/if}
          <pre bind:this={logBox} class="flex-1 p-5 overflow-auto max-h-[32rem] whitespace-pre-wrap break-all">{selected.log || "No output yet."}</pre>
        {:else if selectedId !== null}
          <p class="p-5 opacity-70">Loading the log…</p>
        {:else}
          <p class="p-5 opacity-70 font-sans text-sm">Select a deployment to see its log.</p>
        {/if}
      </div>
    </div>
  {:else if activeTab === "logs"}
    <LogsPanel {app} {env} />
  {:else if activeTab === "secrets"}
    <SecretsPanel {app} {env} required={current?.spec?.secrets ?? []} />
  {/if}

  {#if $can("deployer")}
    <section class="mt-16 pt-6 border-t">
      {#if !showDestroy}
        <div class="flex flex-wrap items-center justify-between gap-4">
          <p class="text-sm text-muted-foreground max-w-xl">
            Destroying {env} removes its containers and hostnames and deletes its deployment history.
          </p>
          <Button
            variant="ghost"
            class="text-destructive hover:text-destructive hover:bg-destructive/10"
            onclick={() => (showDestroy = true)}
            disabled={acting || active}
            title={active ? "Wait for the deployment in progress to finish" : undefined}
          >
            <Trash2 /> Destroy {env}
          </Button>
        </div>
      {:else}
        <div class="border-l-[3px] border-destructive pl-5 py-1">
          <h2 class="section-title">Destroy <span class="engraved text-lg font-normal">{app}/{env}</span></h2>
          <p class="text-sm text-muted-foreground mt-1 mb-5 max-w-2xl">
            Removes its containers from {environment?.server}, its hostnames from the gateway, and its deployment
            history. Secrets are kept, so deploying {env} again picks them back up.
          </p>
          <label class="flex items-start gap-2.5 text-sm mb-5">
            <input type="checkbox" bind:checked={destroyPurge} class="w-4 h-4 mt-0.5 accent-[rgb(var(--danger))]" />
            <span>Also delete its volumes. <span class="text-destructive">The data in them can't be recovered.</span></span>
          </label>
          <label for="destroy-confirm" class="label">Type <span class="font-mono">{env}</span> to confirm</label>
          <div class="flex flex-wrap gap-2 max-w-xl">
            <input id="destroy-confirm" bind:value={destroyConfirm} class="input font-mono flex-1 min-w-48" autocomplete="off" />
            <Button variant="outline" onclick={() => { showDestroy = false; destroyConfirm = ""; }}>Cancel</Button>
            <Button variant="destructive" disabled={destroyConfirm !== env || acting} onclick={destroy}>
              {acting ? "Destroying…" : `Destroy ${env}`}
            </Button>
          </div>
        </div>
      {/if}
    </section>
  {/if}
{:else if !error}
  <Loading label="Loading {app}/{env}" />
{/if}
