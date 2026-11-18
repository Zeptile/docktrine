<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import {
    api,
    type EnvironmentDetail,
    type Deployment,
    ApiError,
  } from "$lib/api";
  import { timeAgo, timeUntil, duration, shortImage } from "$lib/format";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
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
  } from "lucide-svelte";

  export let app: string;
  export let env: string;

  let detail: EnvironmentDetail | null = null;
  let error: string | null = null;
  let tab: "history" | "logs" | "secrets" = "history";

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
      error = e instanceof ApiError ? e.message : "Failed to load environment";
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
    selectedId = id;
    selected = null;
    await loadSelected();
    if (isRunning(selected)) load();
  }

  async function start(action: () => Promise<Deployment>) {
    acting = true;
    error = null;
    try {
      const dep = await action();
      selectedId = dep.id;
      tab = "history";
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to start deployment";
    } finally {
      acting = false;
    }
  }

  function redeploy() {
    if (!current?.spec) return;
    const spec = current.spec;
    start(() => api.deployments.create(spec));
  }

  function rollbackTo(d: Deployment) {
    if (!confirm(`Roll ${app}/${env} back to deployment #${d.id} (${shortImage(d.image)})?`)) {
      return;
    }
    start(() => api.apps.rollback(app, env, d.id));
  }

  async function destroy() {
    acting = true;
    error = null;
    try {
      await api.apps.destroy(app, env, destroyPurge);
      location.hash = "#/apps";
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to destroy environment";
      acting = false;
    }
  }

  function containerColor(state: string) {
    if (state === "running") return "text-green-600";
    if (state === "exited" || state === "dead") return "text-red-600";
    return "text-yellow-600";
  }
</script>

<div class="space-y-6">
  <div class="flex items-start justify-between gap-4">
    <div>
      <a
        href="#/apps"
        class="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1 mb-2"
      >
        <ArrowLeft class="w-4 h-4" /> Apps
      </a>
      <h2 class="text-xl font-semibold flex items-center gap-2">
        {app} <span class="text-muted-foreground font-normal">/</span>
        {env}
        {#if latest && isRunning(latest)}
          <StatusBadge status={latest.status} />
        {:else if current}
          <StatusBadge status={current.status} />
        {/if}
        {#if latest?.status === "failed" && latest.id !== current?.id}
          <span class="text-sm font-normal text-red-600">
            last deploy #{latest.id} failed{current ? `, #${current.id} still serving` : ""}
          </span>
        {/if}
      </h2>
      {#if environment}
        <div class="text-sm text-muted-foreground mt-1 flex flex-wrap items-center gap-x-4 gap-y-1">
          <span>Node <strong class="text-foreground">{environment.server}</strong></span>
          {#if current}
            <span>
              Running <span class="font-mono text-foreground" title={current.image}
                >#{current.id} {shortImage(current.image)}</span
              >
            </span>
          {/if}
          {#if environment.branch}
            <span class="inline-flex items-center gap-1">
              <GitBranch class="w-3.5 h-3.5" />{environment.branch}
            </span>
          {/if}
          {#if environment.expires_at}
            <span class="inline-flex items-center gap-1" title={new Date(environment.expires_at).toLocaleString()}>
              <Clock class="w-3.5 h-3.5" />
              {timeUntil(environment.expires_at) === "expired"
                ? "Expired, removed at next cleanup"
                : `Expires in ${timeUntil(environment.expires_at)}`}
            </span>
          {/if}
        </div>
      {/if}
    </div>

    <div class="flex items-center gap-2 shrink-0">
      <Button variant="outline" onclick={load}>
        <RefreshCw class="w-4 h-4" />
      </Button>
      <Button
        variant="outline"
        onclick={redeploy}
        disabled={acting || active || !current?.spec}
        title={active ? "A deployment is in progress" : "Deploy the running version again, without rebuilding"}
      >
        <RotateCcw class="w-4 h-4 mr-2" /> Redeploy
      </Button>
      <Button
        variant="destructive"
        onclick={() => (showDestroy = !showDestroy)}
        disabled={acting || active || !detail}
      >
        <Trash2 class="w-4 h-4 mr-2" /> Destroy
      </Button>
    </div>
  </div>

  {#if error}
    <div class="p-4 bg-destructive/10 border border-destructive/20 rounded-md">
      <p class="text-destructive text-sm">{error}</p>
    </div>
  {/if}

  {#if showDestroy}
    <Card class="p-6 border-destructive/40">
      <h3 class="font-medium mb-1">Destroy {app}/{env}</h3>
      <p class="text-sm text-muted-foreground mb-4">
        Removes its containers and hostnames from {environment?.server} and the
        gateway, and deletes its deployment history. Secrets are kept, so
        deploying {env} again picks them back up.
      </p>
      <label class="flex items-center gap-2 text-sm mb-4">
        <input type="checkbox" bind:checked={destroyPurge} class="w-4 h-4" />
        Also delete volumes <span class="text-destructive">(data is lost for good)</span>
      </label>
      <label for="destroy-confirm" class="block text-sm mb-2">
        Type <strong class="font-mono">{env}</strong> to confirm
      </label>
      <div class="flex gap-2">
        <input
          id="destroy-confirm"
          bind:value={destroyConfirm}
          class="flex-1 px-3 py-2 border rounded-md bg-background font-mono"
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
          {acting ? "Destroying..." : "Destroy"}
        </Button>
      </div>
    </Card>
  {/if}

  {#if detail}
    <div class="grid md:grid-cols-2 gap-4">
      <Card class="p-6">
        <h3 class="font-medium mb-3">Hostnames</h3>
        {#if detail.hostnames.length === 0}
          <p class="text-sm text-muted-foreground">None. The app isn't exposed through the gateway.</p>
        {:else}
          <ul class="space-y-1 text-sm">
            {#each detail.hostnames as host}
              <li>
                <a
                  href="https://{host}"
                  target="_blank"
                  rel="noreferrer"
                  class="text-primary hover:underline inline-flex items-center gap-1"
                >
                  {host} <ExternalLink class="w-3.5 h-3.5" />
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </Card>

      <Card class="p-6">
        <h3 class="font-medium mb-3">Containers</h3>
        {#if detail.containers_error}
          <p class="text-sm text-destructive">
            Couldn't reach {environment?.server}: {detail.containers_error}
          </p>
        {:else if !detail.containers?.length}
          <p class="text-sm text-muted-foreground">No containers running.</p>
        {:else}
          <ul class="space-y-2 text-sm">
            {#each detail.containers as c (c.id)}
              <li class="flex items-center justify-between gap-3">
                <span class="font-mono truncate" title={c.image}>
                  {c.service ?? c.name}
                </span>
                <span class="{containerColor(c.state)} shrink-0">{c.status}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </Card>
    </div>

    <div class="border-b flex space-x-1">
      {#each [["history", "Deployments"], ["logs", "Logs"], ["secrets", "Secrets"]] as [id, label]}
        <button
          class="px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors {tab === id
            ? 'border-primary text-foreground'
            : 'border-transparent text-muted-foreground hover:text-foreground'}"
          onclick={() => (tab = id as typeof tab)}
        >
          {label}
        </button>
      {/each}
    </div>

    {#if tab === "history"}
      <div class="grid lg:grid-cols-5 gap-4">
        <Card class="lg:col-span-2 overflow-hidden">
          {#if deployments.length === 0}
            <p class="p-6 text-sm text-muted-foreground">No deployments yet.</p>
          {:else}
            <ul class="divide-y max-h-[32rem] overflow-y-auto">
              {#each deployments as d (d.id)}
                <li>
                  <button
                    class="w-full text-left px-4 py-3 hover:bg-accent transition-colors {selectedId === d.id ? 'bg-accent' : ''}"
                    onclick={() => select(d.id)}
                  >
                    <div class="flex items-center justify-between gap-2">
                      <span class="font-medium">
                        #{d.id}
                        {#if d.id === environment?.current_deployment_id}
                          <span class="text-xs text-green-700 font-normal">· live</span>
                        {/if}
                      </span>
                      <StatusBadge status={d.status} />
                    </div>
                    <div class="text-xs font-mono text-muted-foreground truncate mt-1" title={d.image}>
                      {shortImage(d.image)}
                    </div>
                    <div class="text-xs text-muted-foreground mt-0.5 truncate">
                      {timeAgo(d.created_at)}{d.created_by ? ` · ${d.created_by}` : ""}
                    </div>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </Card>

        <Card class="lg:col-span-3 flex flex-col min-h-[20rem]">
          {#if selected}
            <div class="px-4 py-3 border-b flex items-center justify-between gap-2">
              <div class="min-w-0">
                <div class="font-medium flex items-center gap-2">
                  Deployment #{selected.id} <StatusBadge status={selected.status} />
                </div>
                <div class="text-xs text-muted-foreground mt-0.5">
                  {new Date(selected.created_at).toLocaleString()} · {isRunning(selected) ? "running for" : "took"} {duration(selected.created_at, selected.finished_at)}
                </div>
              </div>
              {#if selected.status === "succeeded" && selected.id !== environment?.current_deployment_id}
                <Button
                  size="sm"
                  variant="outline"
                  disabled={acting || active}
                  onclick={() => selected && rollbackTo(selected)}
                >
                  <RotateCcw class="w-4 h-4 mr-2" /> Roll back to this
                </Button>
              {/if}
            </div>
            {#if selected.message}
              <div class="px-4 py-2 text-sm border-b {selected.status === 'failed' ? 'text-destructive bg-destructive/5' : 'text-muted-foreground'}">
                {selected.message}
              </div>
            {/if}
            <pre
              bind:this={logBox}
              class="flex-1 p-4 text-xs font-mono bg-muted/50 overflow-auto max-h-[28rem] whitespace-pre-wrap break-all">{selected.log || "No output yet."}</pre>
          {:else if selectedId !== null}
            <div class="p-6 text-sm text-muted-foreground">Loading...</div>
          {:else}
            <div class="p-6 text-sm text-muted-foreground">Select a deployment to see its log.</div>
          {/if}
        </Card>
      </div>
    {:else if tab === "logs"}
      <LogsPanel {app} {env} />
    {:else if tab === "secrets"}
      <SecretsPanel {app} {env} required={current?.spec?.secrets ?? []} />
    {/if}
  {:else if !error}
    <div class="flex items-center justify-center py-8">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
    </div>
  {/if}
</div>
