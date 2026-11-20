<script lang="ts">
  import { onMount } from "svelte";
  import { api, auth, ApiError } from "$lib/api";
  import { loadMe } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Logo from "./ui/Logo.svelte";
  import Pipe from "./ui/Pipe.svelte";
  import Gauge from "./Gauge.svelte";

  let mode = $state<"password" | "key">("password");
  let username = $state("");
  let password = $state("");
  let key = $state("");
  let error = $state<string | null>(null);
  let busy = $state(false);
  // Before anyone has a password, the only way in is the first API key
  let hasUsers = $state(true);

  onMount(async () => {
    try {
      hasUsers = (await api.auth.status()).has_users;
      if (!hasUsers) mode = "key";
    } catch {}
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = null;
    try {
      if (mode === "password") {
        await api.auth.login(username.trim(), password);
      } else {
        auth.setKey(key.trim());
      }
      if (!(await loadMe())) {
        auth.clearKey();
        error = "That API key isn't valid. Check for missing characters.";
      }
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't reach the Docktrine API";
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
        <div class="flex flex-col items-center text-center">
          <Gauge status={hasUsers ? "succeeded" : "pending"} size={132} sweep />
          <div class="mt-4 flex items-center gap-2.5">
            <Logo class="w-8 h-8" />
            <h1 class="font-slab text-3xl font-bold">Docktrine</h1>
          </div>
          <p class="text-sm text-muted-foreground mt-1">Deployments for {location.hostname}</p>
        </div>

        <form class="mt-8 space-y-4" onsubmit={submit}>
          {#if mode === "password"}
            <div>
              <label for="username" class="label">Username</label>
              <!-- svelte-ignore a11y_autofocus -->
              <input id="username" bind:value={username} required autofocus autocomplete="username" class="input" />
            </div>
            <div>
              <label for="password" class="label">Password</label>
              <input id="password" type="password" bind:value={password} required autocomplete="current-password" class="input" />
            </div>
          {:else}
            {#if !hasUsers}
              <p class="text-sm text-muted-foreground">
                Nobody has a password yet. Sign in with the first API key, from
                <code>data/initial-api-key</code> on the server, then invite yourself.
              </p>
            {/if}
            <div>
              <label for="key" class="label">API key</label>
              <!-- svelte-ignore a11y_autofocus -->
              <input
                id="key"
                type="password"
                bind:value={key}
                required
                autofocus
                autocomplete="off"
                placeholder="dkt_…"
                class="input font-mono"
              />
            </div>
          {/if}

          {#if error}
            <p class="text-sm text-destructive" role="alert">{error}</p>
          {/if}

          <Button type="submit" size="lg" class="w-full" disabled={busy}>
            {busy ? "Signing in…" : "Sign in"}
          </Button>
        </form>

        {#if hasUsers}
          <button
            class="mt-5 text-sm text-link hover:underline"
            onclick={() => {
              mode = mode === "password" ? "key" : "password";
              error = null;
            }}
          >
            {mode === "password" ? "Sign in with an API key instead" : "Sign in with a password instead"}
          </button>
        {/if}
      </div>
    </div>
  </div>
</div>
