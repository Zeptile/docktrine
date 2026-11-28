<script lang="ts">
  import { api, ApiError, type UpdateInfo, type BackupPolicy } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import { can } from "$lib/session";
  import { toast } from "$lib/toast";
  import { refName, shortCommit, shortDigest, newImages, updateSummary } from "$lib/updates";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import { ArrowUpCircle, ArrowRight, ExternalLink, RefreshCw, LoaderCircle } from "lucide-svelte";

  interface Props {
    app: string;
    env: string;
    update?: UpdateInfo;
    // "panel" when an update is available; "line" for the quiet status
    // under what's running
    variant: "panel" | "line";
    // A deployment is in progress
    busy?: boolean;
    // When the environment backs up its volumes; "setup" unless set
    backupBefore?: BackupPolicy;
    onupdate: () => void;
    // The check finished; the page reloads to show it
    onchecked: () => void;
  }

  let { app, env, update, variant, busy = false, backupBefore = "setup", onupdate, onchecked }: Props = $props();

  let checking = $state(false);
  let error = $state<string | null>(null);

  let available = $derived(update?.status === "available");
  let images = $derived(update ? newImages(update) : []);

  async function checkNow() {
    checking = true;
    error = null;
    try {
      const u = await api.apps.checkUpdates(app, env);
      if (u?.status === "current") toast(`${env} runs the newest version`);
      onchecked();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't check for updates";
    } finally {
      checking = false;
    }
  }
</script>

{#snippet checkButton(size: "sm" | "default")}
  {#if $can("deployer")}
    <Button variant="ghost" {size} onclick={checkNow} disabled={checking}>
      {#if checking}<LoaderCircle class="animate-spin" />{:else}<RefreshCw />{/if}
      {checking ? "Checking…" : "Check now"}
    </Button>
  {/if}
{/snippet}

{#if variant === "panel" && update && available}
  <section class="riveted mt-8 px-6 sm:px-8 py-5 border-l-[3px] border-l-brass" aria-labelledby="update-title">
    <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
      <div class="min-w-0">
        <h2 id="update-title" class="section-title flex items-center gap-2">
          <ArrowUpCircle class="w-5 h-5 text-brass shrink-0" /> Update available
        </h2>
        <p class="text-sm text-muted-foreground mt-1">
          {updateSummary(update)}. Checked {timeAgo(update.checked_at)}.
        </p>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        {@render checkButton("default")}
        {#if $can("deployer")}
          <Button onclick={onupdate} disabled={busy} title={busy ? "A deployment is in progress" : undefined}>
            <ArrowUpCircle /> Update
          </Button>
        {/if}
      </div>
    </div>

    <ul class="mt-4 space-y-2 text-sm">
      {#if update.git?.update}
        {@const git = update.git}
        <li class="flex flex-wrap items-center gap-x-2.5 gap-y-1 min-w-0">
          <span class="text-muted-foreground">{refName(git)}</span>
          <code class="font-mono text-[12.5px]" title={git.current}>{shortCommit(git.current)}</code>
          <ArrowRight class="w-3.5 h-3.5 text-muted-foreground" aria-label="to" />
          <code class="font-mono text-[12.5px] font-semibold" title={git.latest}>{shortCommit(git.latest)}</code>
          {#if git.compare_url}
            <a href={git.compare_url} target="_blank" rel="noreferrer" class="text-link hover:underline underline-offset-2 inline-flex items-center gap-1">
              See the changes <ExternalLink class="w-3.5 h-3.5" />
            </a>
          {/if}
        </li>
      {/if}
      {#each images as img (img.image + img.service)}
        <li class="flex flex-wrap items-center gap-x-2.5 gap-y-1 min-w-0">
          {#if img.service}<span class="text-muted-foreground">{img.service}</span>{/if}
          <span class="font-mono text-[12.5px] [overflow-wrap:anywhere]">{img.image}</span>
          <code class="font-mono text-[12.5px] text-muted-foreground" title={img.current}>{shortDigest(img.current)}</code>
          <ArrowRight class="w-3.5 h-3.5 text-muted-foreground" aria-label="to" />
          <code class="font-mono text-[12.5px] font-semibold" title={img.latest}>{shortDigest(img.latest)}</code>
        </li>
      {/each}
    </ul>

    {#if update.setup}
      <Alert kind="warning" class="mt-4">
        Updating runs the setup command again, <code class="font-mono text-[12.5px]">{update.setup}</code>{update.git?.update ? ", since the commit changes" : ""}.
        It may migrate data, which rolling back doesn't undo.
        {#if backupBefore === "never"}
          Backups are off for this environment, so nothing is saved first.
        {:else}
          The volumes are backed up first, so the Backups tab can restore them.
        {/if}
      </Alert>
    {/if}
    {#if error}<Alert class="mt-4">{error}</Alert>{/if}
  </section>
{:else if variant === "line" && !available}
  <div class="mt-2 flex flex-wrap items-center gap-x-1 text-[13px] text-muted-foreground">
    {#if !update}
      <span>Not checked for updates yet.</span>
    {:else if update.status === "current"}
      <span>The newest version, checked {timeAgo(update.checked_at)}.</span>
    {:else if update.status === "pinned"}
      <span title="Checked {timeAgo(update.checked_at)}">
        {update.git?.ref_kind === "commit"
          ? `Pinned to commit ${shortCommit(update.git.latest)}; it doesn't update.`
          : "Runs nothing by a tag that can move."}
      </span>
    {:else}
      <span class="text-warning [overflow-wrap:anywhere]" title={update.error}>
        Couldn't check for updates {timeAgo(update.checked_at)}{update.error ? `: ${update.error}` : "."}
      </span>
    {/if}
    {@render checkButton("sm")}
  </div>
  {#if error}<p class="text-[13px] text-destructive mt-1">{error}</p>{/if}
{/if}
