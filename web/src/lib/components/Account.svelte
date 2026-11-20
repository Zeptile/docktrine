<script lang="ts">
  import { onMount } from "svelte";
  import { api, type ApiKeyInfo, ApiError } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import { confirmAction } from "$lib/confirm";
  import { toast } from "$lib/toast";
  import { me, can, loadMe } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import { Plus, Trash2, Pencil, Copy, Check } from "lucide-svelte";

  let error = $state<string | null>(null);

  // Profile
  let displayName = $state($me?.user?.display_name ?? "");
  let savingName = $state(false);

  // Password
  let current = $state("");
  let next = $state("");
  let confirmNext = $state("");
  let savingPassword = $state(false);
  let passwordError = $state<string | null>(null);

  // API keys
  let keys = $state<ApiKeyInfo[]>([]);
  let keysLoaded = $state(false);
  let newKey = $state("");
  let creating = $state(false);
  let created = $state<{ name: string; key: string } | null>(null);
  let copied = $state(false);
  let renaming = $state<number | null>(null);
  let renameValue = $state("");

  let user = $derived($me?.user ?? null);
  // Admins see everyone's keys, grouped by owner
  let showOwners = $derived($can("admin"));

  onMount(loadKeys);

  async function loadKeys() {
    try {
      keys = await api.keys.list();
      keysLoaded = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load API keys";
    }
  }

  async function saveName(e: SubmitEvent) {
    e.preventDefault();
    savingName = true;
    error = null;
    try {
      await api.me.update(displayName.trim());
      await loadMe();
      toast("Name saved");
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't save your name";
    } finally {
      savingName = false;
    }
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    passwordError = null;
    if (next !== confirmNext) {
      passwordError = "The two new passwords don't match.";
      return;
    }
    savingPassword = true;
    try {
      await api.me.changePassword(current, next);
      current = next = confirmNext = "";
      toast("Password changed. Your other sessions were signed out.");
    } catch (e) {
      passwordError = e instanceof ApiError ? e.message : "Couldn't change the password";
    } finally {
      savingPassword = false;
    }
  }

  async function createKey(e: SubmitEvent) {
    e.preventDefault();
    creating = true;
    error = null;
    copied = false;
    try {
      const k = await api.keys.create(newKey.trim());
      created = { name: k.name, key: k.key };
      newKey = "";
      await loadKeys();
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

  async function saveRename(k: ApiKeyInfo) {
    error = null;
    try {
      await api.keys.rename(k.id, renameValue.trim());
      renaming = null;
      toast(`Renamed to ${renameValue.trim()}`);
      await loadKeys();
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
      await loadKeys();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't revoke the API key";
    }
  }
</script>

<header class="mb-10">
  <h1 class="page-title">Account</h1>
  <p class="text-muted-foreground mt-1.5">
    {#if user}
      Signed in as <strong class="text-foreground font-medium">{user.username}</strong>, {user.role === "admin" ? "an" : "a"} {user.role}.
    {:else}
      Signed in with the API key {$me?.key_name}, which acts as admin. It has no profile or password.
    {/if}
  </p>
</header>

{#if error}
  <Alert class="mb-6">{error}</Alert>
{/if}

<div class="space-y-14">
  {#if user}
    <section class="grid md:grid-cols-[14rem_1fr] gap-x-10 gap-y-4">
      <div>
        <h2 class="section-title">Profile</h2>
        <p class="text-sm text-muted-foreground mt-1">How you appear to other users.</p>
      </div>
      <form class="flex flex-wrap items-end gap-3 max-w-lg" onsubmit={saveName}>
        <div class="flex-1 min-w-56">
          <label for="display-name" class="label">Name</label>
          <input id="display-name" bind:value={displayName} class="input" placeholder={user.username} autocomplete="name" />
        </div>
        <Button type="submit" variant="outline" disabled={savingName || displayName.trim() === user.display_name}>Save name</Button>
      </form>
    </section>

    {#if $me?.via === "session"}
      <section class="grid md:grid-cols-[14rem_1fr] gap-x-10 gap-y-4 border-t pt-10">
        <div>
          <h2 class="section-title">Password</h2>
          <p class="text-sm text-muted-foreground mt-1">Changing it signs out your other browsers.</p>
        </div>
        <form class="space-y-4 max-w-lg" onsubmit={changePassword}>
          <input type="text" autocomplete="username" value={user.username} hidden />
          <div>
            <label for="current" class="label">Current password</label>
            <input id="current" type="password" bind:value={current} required autocomplete="current-password" class="input" />
          </div>
          <div class="grid sm:grid-cols-2 gap-4">
            <div>
              <label for="next" class="label">New password</label>
              <input id="next" type="password" bind:value={next} required minlength="10" autocomplete="new-password" class="input" />
            </div>
            <div>
              <label for="confirm-next" class="label">Same again</label>
              <input id="confirm-next" type="password" bind:value={confirmNext} required autocomplete="new-password" class="input" />
            </div>
          </div>
          <p class="hint mt-0">At least 10 characters.</p>
          {#if passwordError}
            <p class="text-sm text-destructive" role="alert">{passwordError}</p>
          {/if}
          <Button type="submit" variant="outline" disabled={savingPassword}>Change password</Button>
        </form>
      </section>
    {/if}
  {/if}

  <section class="grid md:grid-cols-[14rem_1fr] gap-x-10 gap-y-4 {user ? 'border-t pt-10' : ''}">
    <div>
      <h2 class="section-title">API keys</h2>
      <p class="text-sm text-muted-foreground mt-1">
        For the CLI and CI. A key acts with {user ? "your" : "its owner's"} role{showOwners ? "; as an admin you see everyone's" : ""}.
      </p>
    </div>
    <div class="min-w-0 space-y-6">
      {#if created}
        <div class="rounded-lg border-2 border-success/60 p-5">
          <h3 class="font-semibold">Key {created.name} created</h3>
          <p class="text-sm text-muted-foreground mt-1 mb-4">Copy it now. It can't be shown again.</p>
          <div class="flex flex-wrap gap-2">
            <input readonly value={created.key} aria-label="New API key" class="input font-mono text-[13px] flex-1 min-w-64" onfocus={(e) => e.currentTarget.select()} />
            <Button onclick={copyKey}>
              {#if copied}<Check /> Copied{:else}<Copy /> Copy key{/if}
            </Button>
            <Button variant="ghost" onclick={() => (created = null)}>Done</Button>
          </div>
        </div>
      {/if}

      <form class="flex flex-wrap items-end gap-3 max-w-lg" onsubmit={createKey}>
        <div class="flex-1 min-w-56">
          <label for="key-name" class="label">New key for</label>
          <input id="key-name" bind:value={newKey} required class="input" placeholder="laptop, ci, …" autocomplete="off" />
        </div>
        <Button type="submit" disabled={creating || !newKey.trim()}><Plus /> Create key</Button>
      </form>

      {#if keysLoaded && keys.length === 0}
        <p class="text-sm text-muted-foreground">No keys yet.</p>
      {:else if keysLoaded}
        <ul class="riveted px-5 py-1.5">
          {#each keys as k (k.id)}
            <li class="border-b last:border-0 py-3 px-1 flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
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
                      <span class="text-xs font-normal text-brass">this browser</span>
                    {/if}
                  </div>
                  <div class="text-[13px] text-muted-foreground">
                    <span class="font-mono text-[12px]">{k.prefix}…</span>{#if showOwners}, {k.owner || "no owner, acts as admin"}{/if}
                  </div>
                </div>
              {/if}
              <div class="flex items-center gap-2 shrink-0 ml-auto">
                <span class="text-[13px] text-muted-foreground text-right leading-snug">
                  created {timeAgo(k.created_at)}<br />
                  {k.last_used_at ? `used ${timeAgo(k.last_used_at)}` : "never used"}
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  title="Rename"
                  aria-label="Rename {k.name}"
                  onclick={() => {
                    renaming = k.id;
                    renameValue = k.name;
                  }}
                >
                  <Pencil />
                </Button>
                <Button size="sm" variant="ghost" class="hover:text-destructive" title="Revoke" aria-label="Revoke {k.name}" onclick={() => revoke(k)}>
                  <Trash2 />
                </Button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </section>
</div>
