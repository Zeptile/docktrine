<script lang="ts">
  import { api, auth, ApiError } from "$lib/api";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";

  let { onLogin }: { onLogin: () => void } = $props();

  let apiKey = $state("");
  let loading = $state(false);
  let error: string | null = $state(null);

  async function login(event: SubmitEvent) {
    event.preventDefault();
    try {
      loading = true;
      error = null;
      auth.setKey(apiKey.trim());
      await api.servers.list();
      onLogin();
    } catch (e) {
      auth.clearKey();
      error = e instanceof ApiError ? e.message : "Failed to reach the API";
    } finally {
      loading = false;
    }
  }
</script>

<div class="flex min-h-screen items-center justify-center px-4">
  <Card class="w-full max-w-sm p-6">
    <h1 class="text-2xl font-bold text-foreground">Docktrine</h1>
    <p class="mb-6 text-muted-foreground">Sign in with an API key</p>

    <form onsubmit={login} class="space-y-4">
      <div>
        <label for="api-key" class="block text-sm font-medium mb-2"
          >API key</label
        >
        <input
          id="api-key"
          bind:value={apiKey}
          type="password"
          required
          autocomplete="current-password"
          class="w-full px-3 py-2 border rounded-md bg-background"
        />
      </div>

      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}

      <Button type="submit" class="w-full" disabled={loading || !apiKey.trim()}>
        {loading ? "Signing in..." : "Sign in"}
      </Button>
    </form>
  </Card>
</div>
