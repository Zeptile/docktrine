<script lang="ts">
  import { onMount } from "svelte";
  import { api, type SecretInfo, ApiError } from "$lib/api";
  import { timeAgo } from "$lib/format";
  import { confirmAction } from "$lib/confirm";
  import { toast } from "$lib/toast";
  import Button from "./ui/Button.svelte";
  import Card from "./ui/Card.svelte";
  import Alert from "./ui/Alert.svelte";
  import { KeyRound, Plus, Trash2, Eye, EyeOff } from "lucide-svelte";

  export let app: string;
  export let env: string;
  // Secret names the running spec asks for, from docktrine.yaml
  export let required: string[] = [];

  let secrets: SecretInfo[] = [];
  let loading = false;
  let error: string | null = null;
  let key = "";
  let value = "";
  let reveal = false;
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
      error = e instanceof ApiError ? e.message : "Couldn't load secrets";
    } finally {
      loading = false;
    }
  }

  async function save() {
    saving = true;
    error = null;
    try {
      const res = await api.apps.secrets.set(app, env, { [key.trim()]: value });
      toast(res.message);
      key = "";
      value = "";
      reveal = false;
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't save the secret";
    } finally {
      saving = false;
    }
  }

  async function remove(name: string) {
    const ok = await confirmAction({
      title: `Delete ${name}?`,
      message: `${app}/${env} keeps its current value until the next deploy, which runs without it.`,
      confirmLabel: "Delete secret",
      danger: true,
    });
    if (!ok) return;
    error = null;
    try {
      await api.apps.secrets.delete(app, env, name);
      toast(`${name} deleted. Redeploy to apply it.`);
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Couldn't delete the secret";
    }
  }
</script>

<div class="space-y-4">
  {#if error}
    <Alert>{error}</Alert>
  {/if}
  {#if missing.length > 0}
    <Alert kind="warning">
      docktrine.yaml asks for
      {#each missing as name, i}<button
          type="button"
          class="font-mono underline decoration-dotted underline-offset-2"
          title="Set {name}"
          onclick={() => (key = name)}>{name}</button
        >{i < missing.length - 1 ? ", " : ""}{/each},
      which {missing.length === 1 ? "isn't" : "aren't"} set. The next deploy fails until
      {missing.length === 1 ? "it is" : "they are"}.
    </Alert>
  {/if}

  <Card class="p-5">
    <form
      class="flex flex-wrap items-end gap-3"
      onsubmit={(e) => {
        e.preventDefault();
        save();
      }}
    >
      <div class="flex-1 min-w-48">
        <label for="secret-key" class="label">Name</label>
        <input
          id="secret-key"
          bind:value={key}
          required
          pattern="[A-Za-z_][A-Za-z0-9_]*"
          title="Letters, digits and underscores, not starting with a digit"
          list="secret-suggestions"
          class="input font-mono"
          placeholder="DATABASE_URL"
          autocomplete="off"
        />
        <datalist id="secret-suggestions">
          {#each missing as name}<option value={name}></option>{/each}
        </datalist>
      </div>
      <div class="flex-[2] min-w-64">
        <label for="secret-value" class="label">Value</label>
        <div class="relative">
          <input
            id="secret-value"
            bind:value
            type={reveal ? "text" : "password"}
            required
            class="input font-mono pr-10"
            autocomplete="new-password"
          />
          <button
            type="button"
            class="absolute right-0 top-0 h-10 w-10 grid place-items-center text-muted-foreground hover:text-foreground"
            aria-label={reveal ? "Hide value" : "Show value"}
            onclick={() => (reveal = !reveal)}
          >
            {#if reveal}<EyeOff class="w-4 h-4" />{:else}<Eye class="w-4 h-4" />{/if}
          </button>
        </div>
      </div>
      <Button type="submit" disabled={saving}>
        <Plus />
        {setKeys.has(key.trim()) ? "Replace secret" : "Add secret"}
      </Button>
    </form>
    <p class="hint mt-3">
      Values are encrypted on the server and can't be viewed again. Changes apply on the next deploy.
    </p>
  </Card>

  <Card class="overflow-hidden">
    {#if loading && secrets.length === 0}
      <p class="p-6 text-sm text-muted-foreground">Loading…</p>
    {:else if secrets.length === 0}
      <div class="p-10 text-center text-sm text-muted-foreground">
        <KeyRound class="w-7 h-7 mx-auto mb-3 opacity-60" />
        No secrets set for {app}/{env}.
      </div>
    {:else}
      <ul class="divide-y">
        {#each secrets as s (s.key)}
          <li class="px-5 py-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
            <div class="min-w-0">
              <span class="font-mono text-sm break-all">{s.key}</span>
              {#if unused.includes(s)}
                <span class="text-xs text-muted-foreground ml-2">not used by docktrine.yaml</span>
              {/if}
            </div>
            <div class="flex items-center gap-3">
              <span class="text-xs text-muted-foreground">updated {timeAgo(s.updated_at)}</span>
              <Button size="sm" variant="ghost" onclick={() => remove(s.key)} title="Delete {s.key}" aria-label="Delete {s.key}">
                <Trash2 />
              </Button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
</div>
