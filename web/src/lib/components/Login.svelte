<script lang="ts">
  import { api, auth, ApiError } from "$lib/api";
  import Button from "./ui/Button.svelte";
  import Logo from "./ui/Logo.svelte";

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
      error =
        e instanceof ApiError && e.status === 401
          ? "That key wasn't accepted. Check it was copied in full, and that it hasn't been revoked."
          : e instanceof ApiError
            ? e.message
            : "Couldn't reach the Docktrine server.";
    } finally {
      loading = false;
    }
  }
</script>

<div class="min-h-screen grid lg:grid-cols-[minmax(0,5fr)_minmax(0,6fr)]">
  <!-- The yard: a stack of containers, one per environment you'll deploy -->
  <div class="relative hidden lg:flex flex-col justify-between overflow-hidden bg-yard text-yard-foreground p-10">
    <div class="flex items-center gap-3">
      <Logo class="w-8 h-8" />
      <span class="stencil text-3xl tracking-[0.08em]">DOCKTRINE</span>
    </div>
    <div class="absolute inset-x-10 bottom-36 flex flex-col gap-2" aria-hidden="true">
      {#each [["#F5B700", "w-2/5 ml-[34%]"], ["#2563A8", "w-1/2 ml-[22%]"], ["#B42318", "w-3/4 ml-[12%]"], ["#1A7548", "w-full"]] as [color, size]}
        <div class="corrugated-x h-16 rounded-[3px] opacity-90 {size}" style="color: {color}"></div>
      {/each}
    </div>
    <p class="relative text-yard-muted text-sm max-w-xs">
      Apps, environments and the nodes they run on.
    </p>
  </div>

  <div class="flex items-center justify-center px-6 py-16">
    <div class="w-full max-w-sm">
      <div class="flex items-center gap-3 mb-10 lg:hidden">
        <Logo class="w-8 h-8" />
        <span class="stencil text-3xl tracking-[0.08em]">DOCKTRINE</span>
      </div>

      <h1 class="text-2xl font-semibold">Sign in</h1>
      <p class="text-muted-foreground mt-1 mb-8">Paste an API key for this Docktrine server.</p>

      <form onsubmit={login} class="space-y-5">
        <div>
          <label for="api-key" class="label">API key</label>
          <input
            id="api-key"
            bind:value={apiKey}
            type="password"
            required
            autocomplete="current-password"
            placeholder="dkt_…"
            class="input font-mono"
          />
        </div>

        {#if error}
          <p class="text-sm text-destructive" role="alert">{error}</p>
        {/if}

        <Button type="submit" size="lg" class="w-full" disabled={loading || !apiKey.trim()}>
          {loading ? "Signing in…" : "Sign in"}
        </Button>
      </form>

      <p class="hint mt-8 leading-relaxed">
        No key? Ask someone with access to run <code>docktrine keys create &lt;your-name&gt;</code>.
        Locked out entirely? Run <code>docktrine-api create-key &lt;name&gt;</code> in the Docktrine container.
      </p>
    </div>
  </div>
</div>
