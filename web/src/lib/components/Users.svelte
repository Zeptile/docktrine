<script lang="ts">
  import { onMount } from "svelte";
  import { api, type User, type Role, ApiError } from "$lib/api";
  import { timeAgo, timeUntil } from "$lib/format";
  import { confirmAction } from "$lib/confirm";
  import { toast } from "$lib/toast";
  import { me, loadMe } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import { Copy, Check, UserPlus, KeyRound, Trash2 } from "lucide-svelte";

  const roles: { id: Role; does: string }[] = [
    { id: "viewer", does: "Sees apps, logs, containers and servers" },
    { id: "deployer", does: "Also deploys, rolls back, sets secrets, starts and stops containers" },
    { id: "admin", does: "Also manages servers, users and everyone's API keys" },
  ];

  let users = $state<User[]>([]);
  let loaded = $state(false);
  let error = $state<string | null>(null);

  let username = $state("");
  let displayName = $state("");
  let role = $state<Role>("deployer");
  let inviting = $state(false);

  // The latest invite link, shown once
  let link = $state<{ user: User; url: string; reset: boolean } | null>(null);
  let copied = $state(false);

  let myId = $derived($me?.user?.id);

  onMount(load);

  async function load() {
    try {
      users = await api.users.list();
      error = null;
      loaded = true;
      // The first user made while signed in with a key is the admin's own account
      if (!users.length && !$me?.user) role = "admin";
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't load users";
    }
  }

  function showLink(user: User, token: string, reset: boolean) {
    link = { user, url: `${location.origin}${location.pathname}#/invite/${token}`, reset };
    copied = false;
  }

  async function invite(e: SubmitEvent) {
    e.preventDefault();
    inviting = true;
    error = null;
    try {
      const u = await api.users.create({ username: username.trim(), display_name: displayName.trim(), role });
      showLink(u, u.invite_token, false);
      username = "";
      displayName = "";
      role = "deployer";
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't create the invite";
    } finally {
      inviting = false;
    }
  }

  async function copy() {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link.url);
      copied = true;
    } catch {
      // Clipboard blocked; the link stays selectable
    }
  }

  async function changeRole(u: User, next: Role) {
    if (u.id === myId && next !== "admin") {
      const ok = await confirmAction({
        title: `Make yourself a ${next}?`,
        message: "You'll lose access to this page and to server settings. Another admin can give it back.",
        confirmLabel: `Become ${next}`,
        danger: true,
      });
      if (!ok) return load();
    }
    error = null;
    try {
      await api.users.update(u.id, { display_name: u.display_name, role: next });
      toast(`${u.display_name || u.username} is now ${next === "admin" ? "an" : "a"} ${next}`);
      if (u.id === myId) await loadMe();
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't change the role";
      await load();
    }
  }

  async function reset(u: User) {
    const ok = await confirmAction({
      title: `Reset ${u.display_name || u.username}'s password?`,
      message: "Signs them out everywhere and makes a new invite link for choosing a password. Their API keys keep working.",
      confirmLabel: "Reset password",
    });
    if (!ok) return;
    error = null;
    try {
      const r = await api.users.reset(u.id);
      showLink(r, r.invite_token, true);
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't reset the password";
    }
  }

  async function remove(u: User) {
    const ok = await confirmAction({
      title: `Remove ${u.display_name || u.username}?`,
      message: "They're signed out and their API keys stop working. Deploy history keeps their name.",
      confirmLabel: "Remove user",
      danger: true,
    });
    if (!ok) return;
    error = null;
    try {
      await api.users.remove(u.id);
      toast(`${u.display_name || u.username} removed`);
      if (link?.user.id === u.id) link = null;
      if (u.id === myId) return loadMe();
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't remove the user";
    }
  }

  function initials(u: User) {
    const words = (u.display_name || u.username).split(/[\s._-]+/).filter(Boolean);
    return ((words[0]?.[0] ?? "?") + (words[1]?.[0] ?? "")).toUpperCase();
  }
</script>

<header class="mb-10">
  <h1 class="page-title">Users</h1>
  <p class="text-muted-foreground mt-1.5 max-w-2xl">
    People who sign in to Docktrine. Each one makes their own API keys, which act with their role.
  </p>
  <dl class="mt-5 grid sm:grid-cols-3 gap-x-6 gap-y-3 text-sm max-w-4xl">
    {#each roles as r}
      <div class="border-l-2 border-border pl-3">
        <dt class="font-semibold">{r.id[0].toUpperCase() + r.id.slice(1)}</dt>
        <dd class="text-muted-foreground">{r.does}</dd>
      </div>
    {/each}
  </dl>
</header>

{#if error}
  <Alert class="mb-6">{error}</Alert>
{/if}

{#if link}
  <div class="mb-8 rounded-lg border-2 border-brass/60 p-5">
    <h2 class="section-title">
      {link.reset ? "New password link" : "Invite link"} for {link.user.display_name || link.user.username}
    </h2>
    <p class="text-sm text-muted-foreground mt-1 mb-4">
      Send it to them privately. It works once, for the next 7 days, and isn't shown again.
    </p>
    <div class="flex flex-wrap gap-2">
      <input readonly value={link.url} aria-label="Invite link" class="input font-mono text-[13px] flex-1 min-w-64" onfocus={(e) => e.currentTarget.select()} />
      <Button onclick={copy}>
        {#if copied}<Check /> Copied{:else}<Copy /> Copy link{/if}
      </Button>
      <Button variant="ghost" onclick={() => (link = null)}>Done</Button>
    </div>
  </div>
{/if}

<form class="rounded-lg bg-plate/70 p-5 grid sm:grid-cols-[1fr_1fr_10rem_auto] gap-3 items-end" onsubmit={invite}>
  <div>
    <label for="new-username" class="label">Username</label>
    <input
      id="new-username"
      bind:value={username}
      required
      pattern="[a-z0-9][a-z0-9._\-]*"
      maxlength="32"
      title="Lowercase letters, digits, dots, dashes and underscores"
      class="input"
      placeholder="alex"
      autocomplete="off"
      autocapitalize="off"
    />
  </div>
  <div>
    <label for="new-name" class="label">Name <span class="text-muted-foreground font-normal">(optional)</span></label>
    <input id="new-name" bind:value={displayName} class="input" placeholder="Alex Martin" autocomplete="off" />
  </div>
  <div>
    <label for="new-role" class="label">Role</label>
    <select id="new-role" bind:value={role} class="input">
      {#each roles as r}<option value={r.id}>{r.id}</option>{/each}
    </select>
  </div>
  <Button type="submit" disabled={inviting || !username.trim()}>
    <UserPlus /> Invite
  </Button>
</form>

{#if !loaded}
  {#if !error}<Loading label="Loading users" />{/if}
{:else if users.length === 0}
  <p class="mt-8 text-muted-foreground max-w-xl">
    No users yet. Invite yourself first, as an admin, then open the link to choose your password.
  </p>
{:else}
  <ul class="mt-8 riveted px-5 sm:px-7 py-2">
    {#each users as u (u.id)}
      <li class="border-b last:border-0 py-3.5 px-1 flex flex-wrap items-center gap-x-4 gap-y-2">
        <span
          class="w-9 h-9 shrink-0 rounded-full grid place-items-center text-xs font-semibold
            {u.invited ? 'border-2 border-dashed border-input text-muted-foreground' : 'bg-foreground text-background'}"
          aria-hidden="true">{initials(u)}</span
        >
        <div class="min-w-0 flex-1">
          <div class="font-medium truncate">
            {u.display_name || u.username}
            {#if u.id === myId}<span class="text-xs font-normal text-muted-foreground ml-1">you</span>{/if}
          </div>
          <div class="text-[13px] text-muted-foreground truncate">
            {u.username},
            {#if u.invited}
              {u.invite_expires_at && timeUntil(u.invite_expires_at) !== "expired"
                ? `invited, link valid for ${timeUntil(u.invite_expires_at)}`
                : "invite expired"}
            {:else if u.last_seen_at}
              last seen {timeAgo(u.last_seen_at)}
            {:else}
              never signed in
            {/if}
          </div>
        </div>
        <select
          value={u.role}
          onchange={(e) => changeRole(u, e.currentTarget.value as Role)}
          class="input h-8 w-auto py-0 text-[13px]"
          aria-label="Role of {u.username}"
        >
          {#each roles as r}<option value={r.id}>{r.id}</option>{/each}
        </select>
        <div class="flex items-center gap-1 ml-auto sm:ml-0">
          <Button size="sm" variant="ghost" onclick={() => reset(u)} title={u.invited ? "New invite link" : "Reset password"}>
            <KeyRound /> <span class="hidden md:inline">{u.invited ? "New link" : "Reset password"}</span>
          </Button>
          <Button
            size="sm"
            variant="ghost"
            class="hover:text-destructive"
            onclick={() => remove(u)}
            title="Remove {u.username}"
            aria-label="Remove {u.username}"
          >
            <Trash2 />
          </Button>
        </div>
      </li>
    {/each}
  </ul>
{/if}
