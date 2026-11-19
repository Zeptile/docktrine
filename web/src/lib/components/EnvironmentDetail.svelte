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
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import StatusBadge from "./ui/StatusBadge.svelte";
  import SecretsPanel from "./SecretsPanel.svelte";
  import LogsPanel from "./LogsPanel.svelte";
  import {
    ArrowLeft,
    ExternalLink,
    RefreshCw,
    RotateCcw,
    Trash2,
    GitBranch,
    Clock,
    Server,
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

  function containerTone(state: string) {
    if (state === "running") return "text-success";
    if (state === "exited" || state === "dead") return "text-destructive";
    return "text-warning";
  }

  function deploymentTone(d: Deployment) {
    if (isRunning(d)) return "text-info corrugated-moving";
    if (d.status === "failed") return "text-destructive";
    if (d.id === environment?.current_deployment_id) return "text-success";
    return "text-border";
  }
</script>

<div class="space-y-8">
  <div class="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
    <div class="min-w-0">
      <a
        href="#/apps"
        class="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 mb-3"
      >
        <ArrowLeft class="w-4 h-4" /> {app}
      </a>
      <h1 class="stencil text-5xl break-all flex flex-wrap items-center gap-x-4 gap-y-2">
        {env}
        {#if latest && isRunning(latest)}
          <StatusBadge status={latest.status} class="font-sans tracking-normal" />
        {/if}
      </h1>
      {#if environment}
        <div class="text-sm text-muted-foreground mt-3 flex flex-wrap items-center gap-x-5 gap-y-1.5">
          <span class="inline-flex items-center gap-1.5">
            <Server class="w-3.5 h-3.5" /> {environment.server}
          </span>
          {#if environment.branch}
            <span class="inline-flex items-center gap-1.5">
              <GitBranch class="w-3.5 h-3.5" /> {environment.branch}
            </span>
          {/if}
          {#if environment.expires_at}
            <span class="inline-flex items-center gap-1.5" title={new Date(environment.expires_at).toLocaleString()}>
              <Clock class="w-3.5 h-3.5" />
              {timeUntil(environment.expires_at) === "expired"
                ? "Expired, removed at the next cleanup"
                : `Removed in ${timeUntil(environment.expires_at)} unless redeployed`}
            </span>
          {/if}
        </div>
      {/if}
    </div>

    <div class="flex items-center gap-2 shrink-0">
      <Button variant="outline" size="icon" onclick={load} title="Refresh" aria-label="Refresh">
        <RefreshCw />
      </Button>
      <Button
        variant="outline"
        onclick={redeploy}
        disabled={acting || active || !current?.spec}
        title={active ? "A deployment is in progress" : "Deploy the running version again, without rebuilding"}
      >
        <RotateCcw /> Redeploy
      </Button>
    </div>
  </div>

  {#if error}
    <Alert>{error}</Alert>
  {/if}

  {#if detail}
    {#if failedSinceCurrent}
      <Alert kind="warning">
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

    <Card class="grid md:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)] md:divide-x divide-y md:divide-y-0">
      <section class="p-5 min-w-0">
        <h2 class="text-xs font-semibold text-muted-foreground mb-2">Running</h2>
        {#if current}
          <p class="font-mono text-sm truncate" title={current.image}>{shortImage(current.image)}</p>
          <p class="text-sm text-muted-foreground mt-1">
            #{current.id}, deployed {timeAgo(current.created_at)}{current.created_by ? ` by ${current.created_by}` : ""}
          </p>
        {:else}
          <p class="text-sm text-muted-foreground">Nothing yet.</p>
        {/if}
      </section>

      <section class="p-5 min-w-0">
        <h2 class="text-xs font-semibold text-muted-foreground mb-2">Hostnames</h2>
        {#if detail.hostnames.length === 0}
          <p class="text-sm text-muted-foreground">None; not exposed through the gateway.</p>
        {:else}
          <ul class="space-y-1 text-sm">
            {#each detail.hostnames as host}
              <li class="min-w-0">
                <a
                  href="https://{host}"
                  target="_blank"
                  rel="noreferrer"
                  class="text-link hover:underline underline-offset-2 inline-flex items-center gap-1.5 max-w-full"
                >
                  <span class="truncate">{host}</span> <ExternalLink class="w-3.5 h-3.5 shrink-0" />
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="p-5 min-w-0">
        <h2 class="text-xs font-semibold text-muted-foreground mb-2">Containers</h2>
        {#if detail.containers_error}
          <p class="text-sm text-destructive break-words">
            Couldn't reach {environment?.server}: {detail.containers_error}
          </p>
        {:else if !detail.containers?.length}
          <p class="text-sm text-muted-foreground">None running.</p>
        {:else}
          <ul class="space-y-1.5 text-sm">
            {#each detail.containers as c (c.id)}
              <li class="flex items-center gap-2 min-w-0">
                <span class="w-2 h-2 rounded-full bg-current shrink-0 {containerTone(c.state)}"></span>
                <span class="font-mono text-[13px] truncate" title={c.image}>{c.service ?? c.name}</span>
                <span class="ml-auto text-xs text-muted-foreground shrink-0">{c.status}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </Card>

    <div>
      <nav class="border-b flex gap-1 mb-5" aria-label="Environment">
        {#each tabs as t}
          <a
            href={t.id === "history" ? base : `${base}/${t.id}`}
            aria-current={activeTab === t.id ? "page" : undefined}
            class="relative px-3 py-2.5 text-sm font-medium transition-colors
              {activeTab === t.id ? 'text-foreground' : 'text-muted-foreground hover:text-foreground'}"
          >
            {t.label}
            {#if activeTab === t.id}
              <span class="absolute inset-x-3 -bottom-px h-[3px] rounded-t-sm bg-signal"></span>
            {/if}
          </a>
        {/each}
      </nav>

      {#if activeTab === "history"}
        <div class="grid lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-4 items-start">
          <Card class="overflow-hidden">
            {#if deployments.length === 0}
              <p class="p-6 text-sm text-muted-foreground">No deployments yet.</p>
            {:else}
              <ul class="divide-y max-h-[36rem] overflow-y-auto">
                {#each deployments as d (d.id)}
                  <li>
                    <button
                      class="w-full text-left flex items-stretch transition-colors focus-visible:ring-inset focus-visible:ring-offset-0
                        {selectedId === d.id ? 'bg-muted' : 'hover:bg-muted/60'}"
                      aria-pressed={selectedId === d.id}
                      onclick={() => select(d.id)}
                    >
                      <span class="corrugated w-1.5 shrink-0 {deploymentTone(d)}" aria-hidden="true"></span>
                      <span class="flex-1 min-w-0 px-4 py-3 flex items-start gap-3">
                        <span class="stencil text-2xl w-12 shrink-0 pt-0.5">#{d.id}</span>
                        <span class="flex-1 min-w-0">
                          <span class="flex items-center gap-2">
                            <span class="font-mono text-[13px] truncate" title={d.image}>{shortImage(d.image)}</span>
                          </span>
                          <span class="block text-xs text-muted-foreground mt-0.5 truncate">
                            {timeAgo(d.created_at)}{d.created_by ? ` by ${d.created_by}` : ""}
                          </span>
                        </span>
                        {#if d.id === environment?.current_deployment_id}
                          <span class="h-5 px-1.5 rounded-sm bg-success text-card text-[11px] font-bold tracking-wide inline-flex items-center">live</span>
                        {:else if d.status !== "succeeded"}
                          <StatusBadge status={d.status} />
                        {/if}
                      </span>
                    </button>
                  </li>
                {/each}
              </ul>
            {/if}
          </Card>

          <Card class="overflow-hidden flex flex-col min-h-[20rem]">
            {#if selected}
              <div class="px-5 py-4 border-b flex flex-wrap items-center justify-between gap-3">
                <div class="min-w-0">
                  <div class="font-semibold flex items-center gap-2">
                    Deployment #{selected.id} <StatusBadge status={selected.status} />
                  </div>
                  <div class="text-xs text-muted-foreground mt-1">
                    {new Date(selected.created_at).toLocaleString()}, {isRunning(selected) ? "running for" : "took"} {duration(selected.created_at, selected.finished_at)}
                  </div>
                </div>
                {#if selected.status === "succeeded" && selected.id !== environment?.current_deployment_id}
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={acting || active}
                    onclick={() => selected && rollbackTo(selected)}
                  >
                    <RotateCcw /> Roll back to #{selected.id}
                  </Button>
                {/if}
              </div>
              {#if selected.message}
                <div class="px-5 py-2.5 text-sm border-b {selected.status === 'failed' ? 'text-destructive bg-destructive/10' : 'text-muted-foreground'}">
                  {selected.message}
                </div>
              {/if}
              <pre
                bind:this={logBox}
                class="terminal flex-1 p-5 overflow-auto max-h-[32rem] whitespace-pre-wrap break-all">{selected.log || "No output yet."}</pre>
            {:else if selectedId !== null}
              <Loading label="Loading the log" />
            {:else}
              <div class="p-6 text-sm text-muted-foreground">Select a deployment to see its log.</div>
            {/if}
          </Card>
        </div>
      {:else if activeTab === "logs"}
        <LogsPanel {app} {env} />
      {:else if activeTab === "secrets"}
        <SecretsPanel {app} {env} required={current?.spec?.secrets ?? []} />
      {/if}
    </div>

    <section class="border-t pt-6">
      {#if !showDestroy}
        <div class="flex flex-wrap items-center justify-between gap-4">
          <p class="text-sm text-muted-foreground max-w-xl">
            Destroying {env} removes its containers and hostnames and deletes its deployment history.
          </p>
          <Button
            variant="outline"
            class="text-destructive hover:text-destructive border-destructive/40 hover:border-destructive hover:bg-destructive/10"
            onclick={() => (showDestroy = true)}
            disabled={acting || active}
            title={active ? "Wait for the deployment in progress to finish" : undefined}
          >
            <Trash2 /> Destroy {env}
          </Button>
        </div>
      {:else}
        <Card class="flex overflow-hidden border-destructive/50">
          <span class="corrugated w-2 shrink-0 text-destructive" aria-hidden="true"></span>
          <div class="p-6 flex-1 min-w-0">
            <h2 class="font-semibold">Destroy {app}/{env}</h2>
            <p class="text-sm text-muted-foreground mt-1 mb-5 max-w-2xl">
              Removes its containers from {environment?.server}, its hostnames from the
              gateway, and its deployment history. Secrets are kept, so deploying {env}
              again picks them back up.
            </p>
            <label class="flex items-start gap-2.5 text-sm mb-5">
              <input type="checkbox" bind:checked={destroyPurge} class="w-4 h-4 mt-0.5 accent-[rgb(var(--danger))]" />
              <span>Also delete its volumes. <span class="text-destructive">The data in them can't be recovered.</span></span>
            </label>
            <label for="destroy-confirm" class="label">
              Type <span class="font-mono">{env}</span> to confirm
            </label>
            <div class="flex flex-wrap gap-2">
              <input
                id="destroy-confirm"
                bind:value={destroyConfirm}
                class="input font-mono flex-1 min-w-48"
                autocomplete="off"
              />
              <Button variant="outline" onclick={() => { showDestroy = false; destroyConfirm = ""; }}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                disabled={destroyConfirm !== env || acting}
                onclick={destroy}
              >
                {acting ? "Destroying…" : `Destroy ${env}`}
              </Button>
            </div>
          </div>
        </Card>
      {/if}
    </section>
  {:else if !error}
    <Loading label="Loading {app}/{env}" />
  {/if}
</div>
