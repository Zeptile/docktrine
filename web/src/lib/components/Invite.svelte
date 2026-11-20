<script lang="ts">
  import { onMount } from "svelte";
  import { api, auth, ApiError, type User } from "$lib/api";
  import { loadMe } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Logo from "./ui/Logo.svelte";
  import Pipe from "./ui/Pipe.svelte";

  let { token }: { token: string } = $props();

  let user = $state<User | null>(null);
  let invalid = $state<string | null>(null);
  let password = $state("");
  let confirm = $state("");
  let error = $state<string | null>(null);
  let busy = $state(false);

  onMount(async () => {
    try {
      user = await api.auth.invite(token);
    } catch (e) {
      invalid = e instanceof ApiError ? e.message : "Couldn't reach the Docktrine API";
    }
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (password !== confirm) {
      error = "The two passwords don't match.";
      return;
    }
    busy = true;
    error = null;
    try {
      await api.auth.acceptInvite(token, password);
      // The session replaces any key this browser was using
      auth.clearKey();
      await loadMe();
      location.hash = "#/apps";
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't set the password";
    } finally {
      busy = false;
    }
  }
</script>

<div class="min-h-screen treadplate">
  <Pipe />
  <div class="min-h-[calc(100vh-12px)] flex items-center justify-center px-4 py-12">
    <div class="w-full max-w-sm riveted">
      <div class="px-7 pb-8 pt-7 sm:px-9">
        <div class="flex justify-center"><Logo class="w-12 h-12" /></div>
        {#if invalid}
          <h1 class="font-slab text-2xl font-bold mt-4">This link doesn't work</h1>
          <p class="text-sm text-muted-foreground mt-2">{invalid}</p>
          <a href="#/apps" class="inline-block mt-6 text-sm text-link hover:underline">Go to sign in</a>
        {:else if user}
          <h1 class="font-slab text-2xl font-bold mt-4">
            Welcome aboard, {user.display_name || user.username}
          </h1>
          <p class="text-sm text-muted-foreground mt-2">
            Choose a password for <strong class="text-foreground">{user.username}</strong>. You'll join as
            {user.role === "admin" ? "an" : "a"} {user.role}.
          </p>

          <form class="mt-7 space-y-4" onsubmit={submit}>
            <input type="text" autocomplete="username" value={user.username} hidden />
            <div>
              <label for="password" class="label">Password</label>
              <!-- svelte-ignore a11y_autofocus -->
              <input
                id="password"
                type="password"
                bind:value={password}
                required
                minlength="10"
                autofocus
                autocomplete="new-password"
                class="input"
              />
              <p class="hint">At least 10 characters. A few unrelated words work well.</p>
            </div>
            <div>
              <label for="confirm" class="label">Same password again</label>
              <input id="confirm" type="password" bind:value={confirm} required autocomplete="new-password" class="input" />
            </div>
            {#if error}
              <p class="text-sm text-destructive" role="alert">{error}</p>
            {/if}
            <Button type="submit" size="lg" class="w-full" disabled={busy}>
              {busy ? "Setting password…" : "Set password and sign in"}
            </Button>
          </form>
        {:else}
          <p class="text-sm text-muted-foreground mt-4">Checking the invite…</p>
        {/if}
      </div>
    </div>
  </div>
</div>
