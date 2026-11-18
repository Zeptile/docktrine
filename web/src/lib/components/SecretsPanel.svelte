<script lang="ts">
  import { onMount } from "svelte";
  import { api, type SecretInfo, ApiError } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import { KeyRound, Plus, Trash2, AlertTriangle } from "lucide-svelte";

  export let app: string;
  export let env: string;
  // Secret names the running spec asks for, from docktrine.yaml
  export let required: string[] = [];

  let secrets: SecretInfo[] = [];
  let loading = false;
  let error: string | null = null;
  let notice: string | null = null;
  let key = "";
  let value = "";
  let saving = false;

  $: setKeys = new Set(secrets.map((s) => s.key));
  $: missing = required.filter((k) => !setKeys.has(k));
  $: unused = secrets.filter((s) => required.length > 0 && !required.includes(s.key));

  onMount(load);

  async function load() {
    try {
      loading = true;
      error = null;
      secrets = await api.apps.secrets.list(app, env);
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to load secrets";
    } finally {
      loading = false;
    }
  }

  async function save() {
    saving = true;
    error = null;
    notice = null;
    try {
      const res = await api.apps.secrets.set(app, env, { [key.trim()]: value });
      notice = res.message;
      key = "";
      value = "";
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to save secret";
    } finally {
      saving = false;
    }
  }

  async function remove(name: string) {
    if (!confirm(`Delete secret ${name} from ${app}/${env}?`)) return;
    error = null;
    notice = null;
    try {
      await api.apps.secrets.delete(app, env, name);
      notice = `${name} deleted, redeploy to apply`;
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Failed to delete secret";
    }
  }
</script>

<div class="space-y-4">
  {#if error}
    <div class="p-4 bg-destructive/10 border border-destructive/20 rounded-md">
      <p class="text-destructive text-sm">{error}</p>
    </div>
  {/if}
  {#if notice}
    <div class="p-4 bg-green-50 border border-green-200 rounded-md">
      <p class="text-green-700 text-sm">{notice}</p>
    </div>
  {/if}
  {#if missing.length > 0}
    <div class="p-4 bg-yellow-50 border border-yellow-200 rounded-md text-sm text-yellow-800 flex gap-2">
      <AlertTriangle class="w-4 h-4 mt-0.5 shrink-0" />
      <span>
        docktrine.yaml asks for
        {#each missing as name, i}<code class="font-mono">{name}</code>{i < missing.length - 1 ? ", " : ""}{/each},
        which {missing.length === 1 ? "isn't" : "aren't"} set. The next deploy will fail until
        {missing.length === 1 ? "it is" : "they are"}.
      </span>
    </div>
  {/if}

  <Card class="p-6">
    <form
      class="flex flex-wrap items-end gap-2"
      onsubmit={(e) => {
        e.preventDefault();
        save();
      }}
    >
      <div class="flex-1 min-w-48">
        <label for="secret-key" class="block text-sm font-medium mb-2">Name</label>
        <input
          id="secret-key"
          bind:value={key}
          required
          pattern="[A-Za-z_][A-Za-z0-9_]*"
          title="Letters, digits and underscores, not starting with a digit"
          list="secret-suggestions"
          class="w-full px-3 py-2 border rounded-md bg-background font-mono"
          placeholder="DATABASE_URL"
          autocomplete="off"
        />
        <datalist id="secret-suggestions">
          {#each missing as name}<option value={name}></option>{/each}
        </datalist>
      </div>
      <div class="flex-[2] min-w-64">
        <label for="secret-value" class="block text-sm font-medium mb-2">Value</label>
        <input
          id="secret-value"
          bind:value
          type="password"
          required
          class="w-full px-3 py-2 border rounded-md bg-background font-mono"
          autocomplete="new-password"
        />
      </div>
      <Button type="submit" disabled={saving}>
        <Plus class="w-4 h-4 mr-2" />
        {setKeys.has(key.trim()) ? "Replace" : "Add"}
      </Button>
    </form>
    <p class="text-xs text-muted-foreground mt-3">
      Values are encrypted on the server and never shown again. Changes apply on
      the next deploy.
    </p>
  </Card>

  <Card>
    {#if loading && secrets.length === 0}
      <p class="p-6 text-sm text-muted-foreground">Loading...</p>
    {:else if secrets.length === 0}
      <div class="p-6 text-center text-sm text-muted-foreground">
        <KeyRound class="w-8 h-8 mx-auto mb-2 opacity-50" />
        No secrets for {app}/{env}
      </div>
    {:else}
      <ul class="divide-y">
        {#each secrets as s (s.key)}
          <li class="px-6 py-3 flex items-center justify-between gap-4">
            <div>
              <span class="font-mono text-sm">{s.key}</span>
              {#if unused.includes(s)}
                <span class="text-xs text-muted-foreground ml-2">not used by docktrine.yaml</span>
              {/if}
            </div>
            <div class="flex items-center gap-4">
              <span class="text-xs text-muted-foreground">updated {timeAgo(s.updated_at)}</span>
              <Button size="sm" variant="ghost" onclick={() => remove(s.key)} title="Delete">
                <Trash2 class="w-4 h-4" />
              </Button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
</div>
