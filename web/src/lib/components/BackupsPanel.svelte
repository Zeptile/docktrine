<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, type Backup, type BackupList, type Deployment, ApiError } from "$lib/api";
  import { timeAgo, bytes, duration, shortImage } from "$lib/format";
  import { confirmAction } from "$lib/confirm";
  import { toast } from "$lib/toast";
  import { can } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import StatusBadge from "./ui/StatusBadge.svelte";
  import { Archive, ChevronRight, History, LoaderCircle, Trash2 } from "lucide-svelte";

  export let app: string;
  export let env: string;
  // What runs now, for when deploys back up and for what a restore changes
  export let current: Deployment | undefined = undefined;
  export let server = "the server";
  // A deployment is in progress, which backups and restores wait for
  export let deploying = false;
  // Called with the redeploy a restore started
  export let onredeploy: (dep: Deployment) => void = () => {};

  let list: BackupList | null = null;
  let error: string | null = null;
  let acting = false;
  let open = new Set<number>();
  // The backup whose restore choices are shown, and the choice
  let restoring: Backup | null = null;
  let redeploy = true;
  let timer: ReturnType<typeof setTimeout> | undefined;

  $: backups = list?.backups ?? [];
  $: running = !!list && isRunning(list);

  function isRunning(l: NonNullable<typeof list>): boolean {
    return (l.backups ?? []).some((b) => b.status === "running") || !!l.busy;
  }
  $: policy = current?.spec?.backup?.before ?? "setup";
  $: keep = current?.spec?.backup?.keep || list?.keep || 5;
  $: newestOk = backups.find((b) => b.status === "succeeded");

  onMount(load);
  onDestroy(() => clearTimeout(timer));

  async function load() {
    clearTimeout(timer);
    try {
      error = null;
      list = await api.apps.backups.list(app, env);
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load backups";
    }
    // From the fresh list: the reactive `running` only updates after this
    // function returns, so it would still say what the last load saw
    if (list && isRunning(list)) timer = setTimeout(load, 1500);
  }

  async function backUpNow() {
    const ok = await confirmAction({
      title: `Back up ${env} now?`,
      message: `Its containers stop while their volumes are copied, which takes seconds for small volumes and minutes for big ones, then start again.`,
      confirmLabel: "Back up now",
    });
    if (!ok) return;
    acting = true;
    error = null;
    try {
      const b = await api.apps.backups.create(app, env);
      toast(`Backup #${b.id} started`);
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't start the backup";
    } finally {
      acting = false;
    }
  }

  function chooseRestore(b: Backup) {
    restoring = b;
    // Redeploying is only needed, and only possible, when what ran differs
    redeploy = !!b.spec && b.deployment_id !== current?.id;
  }

  async function restore(b: Backup) {
    const what = b.items.map(describe).join(", ");
    const then = b.deployment_id ? `#${b.deployment_id}` : "what ran then";
    const ok = await confirmAction({
      title: `Restore backup #${b.id}?`,
      message:
        `This replaces the current data of ${app}/${env} with the backup from ${new Date(b.created_at).toLocaleString()}: ${what}. ` +
        `Anything written since is lost. Its containers stop first` +
        (redeploy ? `, then ${then} is deployed again so the code matches the data.` : ", then start again on the restored data."),
      confirmLabel: "Replace the data",
      danger: true,
    });
    if (!ok) return;
    acting = true;
    error = null;
    try {
      const res = await api.apps.backups.restore(app, env, b.id, redeploy);
      toast(res.message);
      restoring = null;
      await load();
      if (res.deployment) onredeploy(res.deployment);
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't restore the backup";
    } finally {
      acting = false;
    }
  }

  async function remove(b: Backup) {
    const last = b.id === newestOk?.id;
    const ok = await confirmAction({
      title: `Delete backup #${b.id}?`,
      message: `Deletes its files (${bytes(b.size)}) from ${b.server}.${last ? " It's the newest backup that finished, so there'd be nothing to go back to until the next one." : ""}`,
      confirmLabel: "Delete backup",
      danger: true,
    });
    if (!ok) return;
    error = null;
    try {
      await api.apps.backups.remove(app, env, b.id);
      toast(`Backup #${b.id} deleted`);
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't delete the backup";
    }
  }

  function toggle(id: number) {
    open.has(id) ? open.delete(id) : open.add(id);
    open = open;
  }

  function describe(item: Backup["items"][number]) {
    return item.kind === "stack" ? "the stack's directory" : item.name;
  }

  function why(b: Backup) {
    if (b.triggered_by) return `Before deploy #${b.triggered_by}, since ${b.reason}`;
    return `Taken on request${b.created_by ? ` by ${b.created_by}` : ""}`;
  }

  const policies: Record<string, string> = {
    setup: "before a setup command runs",
    upgrade: "before a setup command runs, and before deploys that change the image or commit",
    never: "never; only when you back up here",
  };
</script>

<div class="space-y-5 max-w-4xl">
  {#if error}
    <Alert>{error}</Alert>
  {/if}

  <div class="rounded-lg bg-plate/70 p-5 flex flex-wrap items-start justify-between gap-4">
    <div class="text-sm max-w-2xl space-y-1.5">
      <p>
        Deploys back up {env}'s volumes {policies[policy] ?? policies.setup}. The last {keep} are kept, and the newest one that
        finished is never deleted automatically.
      </p>
      <p class="text-muted-foreground">
        Kept on {server} in <span class="font-mono text-[13px] break-all">{list?.dir ?? "…"}</span>.
        Change when under Configure.
      </p>
    </div>
    {#if $can("deployer")}
      <Button
        variant="outline"
        onclick={backUpNow}
        disabled={acting || running || deploying}
        title={deploying ? "A deployment is in progress" : running ? "A backup is in progress" : undefined}
      >
        {#if running}<LoaderCircle class="animate-spin" />{:else}<Archive />{/if}
        Back up now
      </Button>
    {/if}
  </div>

  {#if list && backups.length === 0}
    <p class="py-6 text-sm text-muted-foreground flex items-center gap-2.5">
      <Archive class="w-4 h-4 opacity-70" /> No backups of {app}/{env} yet.
    </p>
  {:else if list}
    <ul class="border-t">
      {#each backups as b (b.id)}
        <li class="border-b">
          <div class="px-1 py-3 flex flex-wrap items-center gap-x-4 gap-y-2">
            <button
              class="flex items-center gap-3 min-w-0 flex-1 text-left"
              aria-expanded={open.has(b.id)}
              onclick={() => toggle(b.id)}
            >
              <ChevronRight class="w-4 h-4 shrink-0 text-muted-foreground transition-transform {open.has(b.id) ? 'rotate-90' : ''}" />
              <StatusBadge status={b.status} label={false} />
              <span class="min-w-0">
                <span class="flex items-baseline gap-2.5">
                  <span class="nixie">{b.id}</span>
                  <span class="text-sm" title={new Date(b.created_at).toLocaleString()}>{timeAgo(b.created_at)}</span>
                  {#if b.status === "succeeded"}
                    <span class="text-xs text-muted-foreground">{bytes(b.size)}</span>
                  {:else if b.status === "running"}
                    <span class="text-xs text-warning">backing up…</span>
                  {:else}
                    <span class="text-xs text-destructive">failed</span>
                  {/if}
                  {#if b.restored_at}
                    <span class="text-xs text-muted-foreground" title={new Date(b.restored_at).toLocaleString()}>restored {timeAgo(b.restored_at)}</span>
                  {/if}
                </span>
                <span class="block text-[13px] text-muted-foreground mt-0.5 truncate">{why(b)}</span>
              </span>
            </button>
            {#if $can("deployer") && b.status !== "running"}
              <div class="flex items-center gap-1.5 ml-auto">
                {#if b.status === "succeeded"}
                  <Button size="sm" variant="outline" disabled={acting || running || deploying} onclick={() => chooseRestore(b)}>
                    <History /> Restore
                  </Button>
                {/if}
                <Button size="sm" variant="ghost" onclick={() => remove(b)} disabled={acting} title="Delete backup #{b.id}" aria-label="Delete backup #{b.id}">
                  <Trash2 />
                </Button>
              </div>
            {/if}
          </div>

          {#if restoring?.id === b.id}
            <div class="mx-1 mb-4 border-l-[3px] border-destructive pl-5 py-1">
              <p class="text-sm font-medium">Restoring replaces the current data of {env} with this backup's.</p>
              <p class="hint mt-1">
                Anything written since {new Date(b.created_at).toLocaleString()} is lost. Its containers stop while the
                volumes are replaced.
              </p>
              <fieldset class="mt-4 space-y-2 text-sm">
                <legend class="sr-only">After restoring</legend>
                <label class="flex items-start gap-2.5">
                  <input type="radio" name="restore-{b.id}" checked={redeploy} disabled={!b.spec} onchange={() => (redeploy = true)} class="mt-1 accent-[rgb(var(--brass))]" />
                  <span>
                    Then deploy {b.deployment_id ? `#${b.deployment_id}` : "what ran then"} again
                    {#if b.spec}<span class="font-mono text-[12.5px]">({shortImage(b.spec.image ?? (b.spec.source?.commit ? b.spec.source.commit.slice(0, 7) : "compose"))})</span>{/if}
                    <span class="block hint mt-0.5">
                      {b.deployment_id === current?.id
                        ? "It's what runs now, so this only restarts it."
                        : "Recommended: the code goes back to what wrote this data, which a migration since may no longer read."}
                    </span>
                  </span>
                </label>
                <label class="flex items-start gap-2.5">
                  <input type="radio" name="restore-{b.id}" checked={!redeploy} onchange={() => (redeploy = false)} class="mt-1 accent-[rgb(var(--brass))]" />
                  <span>
                    Keep {current ? `#${current.id}` : "what runs now"} and start it again on the restored data
                  </span>
                </label>
              </fieldset>
              <div class="flex flex-wrap gap-2 mt-4">
                <Button variant="outline" size="sm" onclick={() => (restoring = null)}>Cancel</Button>
                <Button variant="destructive" size="sm" disabled={acting} onclick={() => restore(b)}>
                  {#if acting}<LoaderCircle class="animate-spin" /> Restoring…{:else}Restore #{b.id}{/if}
                </Button>
              </div>
            </div>
          {/if}

          {#if open.has(b.id)}
            <div class="pl-10 pr-1 pb-4 text-sm space-y-3">
              {#if b.message}
                <p class={b.status === "failed" ? "text-destructive break-words" : "text-muted-foreground"}>{b.message}</p>
              {/if}
              {#if b.items.length}
                <table class="w-full max-w-2xl text-[13px]">
                  <thead class="text-muted-foreground text-left">
                    <tr><th class="font-normal py-1">Backed up</th><th class="font-normal py-1 text-right">Size</th><th class="font-normal py-1 text-right">Compressed</th></tr>
                  </thead>
                  <tbody>
                    {#each b.items as item}
                      <tr class="border-t">
                        <td class="py-1.5 font-mono break-all pr-4">{describe(item)}</td>
                        <td class="py-1.5 text-right whitespace-nowrap">{bytes(item.size)}</td>
                        <td class="py-1.5 text-right whitespace-nowrap">{bytes(item.archive_size)}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              {/if}
              {#if b.skipped.length}
                <div>
                  <p class="text-muted-foreground">Not backed up:</p>
                  <ul class="mt-1 space-y-1">
                    {#each b.skipped as s}
                      <li class="text-[13px]">
                        <span class="font-mono break-all">{s.source}</span> at <span class="font-mono">{s.target}</span>{s.container ? ` in ${s.container}` : ""}:
                        <span class="text-muted-foreground">{s.reason}</span>
                      </li>
                    {/each}
                  </ul>
                </div>
              {/if}
              <p class="text-muted-foreground text-[13px]">
                {#if b.deployment_id}Taken while #{b.deployment_id} ran. {/if}
                {#if b.finished_at}Took {duration(b.created_at, b.finished_at)}. {/if}
                In <span class="font-mono break-all">{b.path}</span> on {b.server}.
              </p>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
