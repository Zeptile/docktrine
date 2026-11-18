<script lang="ts">
  import { onMount } from "svelte";
  import { api, type ApiKeyInfo, ApiError } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
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
      error = e instanceof ApiError ? e.message : "Failed to load API keys";
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
      error = e instanceof ApiError ? e.message : "Failed to create API key";
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
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to rename API key";
    }
  }

  async function revoke(k: ApiKeyInfo) {
    const message = k.current
      ? `"${k.name}" is the key you're signed in with. Revoking it signs you out. Revoke it?`
      : `Revoke "${k.name}" (${k.prefix}…)? Anything using it stops working.`;
    if (!confirm(message)) return;
    error = null;
    try {
      await api.keys.revoke(k.id);
      // Revoking our own key signs us out on the next request
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to revoke API key";
    }
  }
</script>

<div class="space-y-6">
  <div>
    <h2 class="text-xl font-semibold">API keys</h2>
    <p class="text-sm text-muted-foreground mt-1">
      Give each person and system (laptop, CI) its own key: deploy history shows
      the key's name, and one key can be revoked without affecting the others.
      Every key has full access to every node.
    </p>
  </div>

  {#if error}
    <div class="p-4 bg-destructive/10 border border-destructive/20 rounded-md">
      <p class="text-destructive text-sm">{error}</p>
    </div>
  {/if}

  {#if created}
    <Card class="p-6 border-green-300 bg-green-50">
      <h3 class="font-medium text-green-800">Key "{created.name}" created</h3>
      <p class="text-sm text-green-800 mt-1 mb-3">
        Copy it now. It won't be shown again.
      </p>
      <div class="flex gap-2">
        <input
          readonly
          value={created.key}
          aria-label="New API key"
          class="flex-1 px-3 py-2 border rounded-md bg-background font-mono text-sm"
          onfocus={(e) => e.currentTarget.select()}
        />
        <Button variant="outline" onclick={copyKey}>
          {#if copied}
            <Check class="w-4 h-4 mr-2" /> Copied
          {:else}
            <Copy class="w-4 h-4 mr-2" /> Copy
          {/if}
        </Button>
        <Button variant="ghost" onclick={() => (created = null)}>Done</Button>
      </div>
    </Card>
  {/if}

  <Card class="p-6">
    <form
      class="flex items-end gap-2"
      onsubmit={(e) => {
        e.preventDefault();
        create();
      }}
    >
      <div class="flex-1">
        <label for="key-name" class="block text-sm font-medium mb-2">New key for</label>
        <input
          id="key-name"
          bind:value={newName}
          required
          class="w-full px-3 py-2 border rounded-md bg-background"
          placeholder="alex-laptop, ci, ..."
          autocomplete="off"
        />
      </div>
      <Button type="submit" disabled={creating || !newName.trim()}>
        <Plus class="w-4 h-4 mr-2" /> Create key
      </Button>
    </form>
  </Card>

  <Card>
    {#if loading && keys.length === 0}
      <p class="p-6 text-sm text-muted-foreground">Loading...</p>
    {:else}
      <ul class="divide-y">
        {#each keys as k (k.id)}
          <li class="px-6 py-3 flex items-center justify-between gap-4">
            <div class="flex items-center gap-3 min-w-0">
              <KeyRound class="w-4 h-4 text-muted-foreground shrink-0" />
              {#if renaming === k.id}
                <form
                  class="flex gap-2"
                  onsubmit={(e) => {
                    e.preventDefault();
                    saveRename(k);
                  }}
                >
                  <input
                    bind:value={renameValue}
                    required
                    aria-label="New name"
                    class="px-2 py-1 border rounded-md bg-background text-sm"
                  />
                  <Button size="sm" type="submit">Save</Button>
                  <Button size="sm" variant="ghost" type="button" onclick={() => (renaming = null)}>Cancel</Button>
                </form>
              {:else}
                <div class="min-w-0">
                  <div class="font-medium truncate">
                    {k.name}
                    {#if k.current}
                      <span class="text-xs font-normal text-muted-foreground">· this browser</span>
                    {/if}
                  </div>
                  <div class="text-xs text-muted-foreground font-mono">{k.prefix}…</div>
                </div>
              {/if}
            </div>
            <div class="flex items-center gap-4 shrink-0">
              <span class="text-xs text-muted-foreground text-right">
                created {timeAgo(k.created_at)}<br />
                {k.last_used_at ? `used ${timeAgo(k.last_used_at)}` : "never used"}
              </span>
              <Button size="sm" variant="ghost" title="Rename" onclick={() => startRename(k)}>
                <Pencil class="w-4 h-4" />
              </Button>
              <Button
                size="sm"
                variant="ghost"
                title={keys.length === 1 ? "The last key can't be revoked" : "Revoke"}
                disabled={keys.length === 1}
                onclick={() => revoke(k)}
              >
                <Trash2 class="w-4 h-4" />
              </Button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
</div>
