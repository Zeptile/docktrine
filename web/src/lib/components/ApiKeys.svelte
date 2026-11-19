<script lang="ts">
  import { onMount } from "svelte";
  import { api, type ApiKeyInfo, ApiError } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import { confirmAction } from "$lib/confirm";
  import { toast } from "$lib/toast";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import { KeyRound, Plus, Trash2, Pencil, Copy, Check } from "lucide-svelte";

  let keys: ApiKeyInfo[] = [];
  let loading = false;
  let error: string | null = null;
  let newName = "";
  let creating = false;
  // Shown once, right after creation
  let created: { name: string; key: string } | null = null;
  let copied = false;
  let renaming: number | null = null;
  let renameValue = "";

  onMount(load);

  async function load() {
    try {
      loading = true;
      error = null;
      keys = await api.keys.list();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load API keys";
    } finally {
      loading = false;
    }
  }

  async function create() {
    creating = true;
    error = null;
    copied = false;
    try {
      const k = await api.keys.create(newName.trim());
      created = { name: k.name, key: k.key };
      newName = "";
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't create the API key";
    } finally {
      creating = false;
    }
  }

  async function copyKey() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.key);
      copied = true;
    } catch {
      // Clipboard blocked; the key stays selectable on screen
    }
  }

  function startRename(k: ApiKeyInfo) {
    renaming = k.id;
    renameValue = k.name;
  }

  async function saveRename(k: ApiKeyInfo) {
    error = null;
    try {
      await api.keys.rename(k.id, renameValue.trim());
      renaming = null;
      toast(`Renamed to ${renameValue.trim()}`);
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't rename the API key";
    }
  }

  async function revoke(k: ApiKeyInfo) {
    const ok = await confirmAction({
      title: `Revoke ${k.name}?`,
      message: k.current
        ? "It's the key this browser is signed in with, so you'll be signed out."
        : `Anything using ${k.prefix}… stops working right away. This can't be undone.`,
      confirmLabel: "Revoke key",
      danger: true,
    });
    if (!ok) return;
    error = null;
    try {
      await api.keys.revoke(k.id);
      toast(`${k.name} revoked`);
      // Revoking our own key signs us out on the next request
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't revoke the API key";
    }
  }
</script>

<div class="space-y-8">
  <div>
    <h1 class="stencil text-5xl">API keys</h1>
    <p class="text-muted-foreground mt-2 max-w-2xl">
      Give each person and system (a laptop, CI) its own key. Deploy history shows the key's
      name, and one key can be revoked without affecting the others. Every key has full access.
    </p>
  </div>

  {#if error}
    <Alert>{error}</Alert>
  {/if}

  {#if created}
    <Card class="flex overflow-hidden border-success/50">
      <span class="corrugated w-2 shrink-0 text-success" aria-hidden="true"></span>
      <div class="p-6 flex-1 min-w-0">
        <h2 class="font-semibold">Key for {created.name} created</h2>
        <p class="text-sm text-muted-foreground mt-1 mb-4">
          Copy it now. It can't be shown again.
        </p>
        <div class="flex flex-wrap gap-2">
          <input
            readonly
            value={created.key}
            aria-label="New API key"
            class="input font-mono flex-1 min-w-64"
            onfocus={(e) => e.currentTarget.select()}
          />
          <Button onclick={copyKey}>
            {#if copied}
              <Check /> Copied
            {:else}
              <Copy /> Copy key
            {/if}
          </Button>
          <Button variant="outline" onclick={() => (created = null)}>Done</Button>
        </div>
      </div>
    </Card>
  {/if}

  <Card class="p-5">
    <form
      class="flex flex-wrap items-end gap-3"
      onsubmit={(e) => {
        e.preventDefault();
        create();
      }}
    >
      <div class="flex-1 min-w-56">
        <label for="key-name" class="label">New key for</label>
        <input
          id="key-name"
          bind:value={newName}
          required
          class="input"
          placeholder="alex-laptop, ci, …"
          autocomplete="off"
        />
      </div>
      <Button type="submit" disabled={creating || !newName.trim()}>
        <Plus /> Create key
      </Button>
    </form>
  </Card>

  {#if loading && keys.length === 0}
    <Loading label="Loading keys" />
  {:else}
    <Card class="overflow-hidden">
      <ul class="divide-y">
        {#each keys as k (k.id)}
          <li class="px-5 py-3.5 flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
            <div class="flex items-center gap-3 min-w-0">
              <KeyRound class="w-4 h-4 text-muted-foreground shrink-0" />
              {#if renaming === k.id}
                <form
                  class="flex flex-wrap gap-2"
                  onsubmit={(e) => {
                    e.preventDefault();
                    saveRename(k);
                  }}
                >
                  <!-- svelte-ignore a11y_autofocus -->
                  <input
                    bind:value={renameValue}
                    required
                    autofocus
                    aria-label="New name"
                    class="input h-8 w-56"
                    onkeydown={(e) => e.key === "Escape" && (renaming = null)}
                  />
                  <Button size="sm" type="submit">Save</Button>
                  <Button size="sm" variant="ghost" type="button" onclick={() => (renaming = null)}>Cancel</Button>
                </form>
              {:else}
                <div class="min-w-0">
                  <div class="font-medium truncate flex items-center gap-2">
                    {k.name}
                    {#if k.current}
                      <span class="text-xs font-normal px-1.5 h-5 inline-flex items-center rounded-sm bg-signal/25">this browser</span>
                    {/if}
                  </div>
                  <div class="text-xs text-muted-foreground font-mono">{k.prefix}…</div>
                </div>
              {/if}
            </div>
            <div class="flex items-center gap-3 shrink-0 ml-auto">
              <span class="text-xs text-muted-foreground text-right">
                created {timeAgo(k.created_at)}<br />
                {k.last_used_at ? `used ${timeAgo(k.last_used_at)}` : "never used"}
              </span>
              <Button size="sm" variant="ghost" title="Rename" aria-label="Rename {k.name}" onclick={() => startRename(k)}>
                <Pencil />
              </Button>
              <Button
                size="sm"
                variant="ghost"
                class="hover:text-destructive"
                title={keys.length === 1 ? "The last key can't be revoked" : "Revoke"}
                aria-label="Revoke {k.name}"
                disabled={keys.length === 1}
                onclick={() => revoke(k)}
              >
                <Trash2 />
              </Button>
            </div>
          </li>
        {/each}
      </ul>
    </Card>
  {/if}
</div>
