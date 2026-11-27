<script lang="ts">
  import { newRow, needsValue, isRequired, parseDotenv, looksSecret, isEnvKey, type VarRow } from "$lib/launch";
  import { generateSecret, secretFormat } from "$lib/generate";
  import { toast } from "$lib/toast";
  import { SvelteSet } from "svelte/reactivity";
  import Button from "../ui/Button.svelte";
  import { Lock, LockOpen, Plus, X, ChevronRight, ClipboardPaste, Dices, Eye, EyeOff, Copy } from "lucide-svelte";

  interface Props {
    rows: VarRow[];
    // Whether the rows came from reading a stack, which changes the wording
    fromStack?: boolean;
  }

  let { rows = $bindable(), fromStack = false }: Props = $props();

  let showDefaults = $state(false);
  let pasting = $state(false);
  let pasted = $state("");
  // Rows whose value is shown in the clear, so a generated one can be read
  // and copied before it is stored
  let shown = new SvelteSet<number>();
  // The stored secret waiting on a yes to being replaced
  let rotating = $state<number | null>(null);

  let main = $derived(rows.filter((r) => needsValue(r)));
  let defaults = $derived(rows.filter((r) => !needsValue(r)));
  let changedDefaults = $derived(defaults.filter((r) => r.value !== "").length);
  let duplicate = $derived.by(() => {
    const seen = new Set<string>();
    const dupes = new Set<string>();
    for (const r of rows) {
      if (!r.key) continue;
      if (seen.has(r.key)) dupes.add(r.key);
      seen.add(r.key);
    }
    return dupes;
  });

  // Credentials someone has to supply: no value yet, nothing stored, and no
  // default that would do
  function isMissingSecret(r: VarRow): boolean {
    return !!r.key && canGenerate(r) && !r.value && !r.stored && needsValue(r);
  }

  function canGenerate(r: VarRow): boolean {
    return !!r.key && isEnvKey(r.key) && (r.secret || looksSecret(r.key));
  }

  let missing = $derived(rows.filter(isMissingSecret));

  function generate(row: VarRow) {
    // A catalog entry knows the format its app expects, e.g. base64 for
    // Plausible's SECRET_KEY_BASE; otherwise guess from the name
    row.value = generateSecret(row.preset?.generate ?? secretFormat(row.key, row.stack?.description, row.stack?.suggested ?? row.stack?.default));
    row.secret = true;
    shown.add(row.id);
    rotating = null;
  }

  function generateMissing() {
    const todo = [...missing];
    for (const r of todo) generate(r);
    toast(`Generated ${todo.length} secrets. Copy them before you launch; they aren't shown again.`);
  }

  // A stored secret is only replaced after a second click, since the app
  // may already be using the old one
  function onGenerate(row: VarRow) {
    if (row.stored && !row.value) rotating = row.id;
    else generate(row);
  }

  function toggleShown(row: VarRow) {
    if (!shown.delete(row.id)) shown.add(row.id);
  }

  async function copyValue(row: VarRow) {
    try {
      await navigator.clipboard.writeText(row.value);
      toast(`Copied ${row.key}.`);
    } catch {
      toast("Couldn't copy. Show the value and select it instead.", "error");
    }
  }

  function add() {
    rows.push(newRow());
  }

  function remove(row: VarRow) {
    const i = rows.indexOf(row);
    if (i >= 0) rows.splice(i, 1);
  }

  // Sets what was pasted: existing names take the new value, new names
  // become rows, and credentials are stored as secrets
  function merge(text: string): number {
    const entries = parseDotenv(text);
    for (const { key, value } of entries) {
      const existing = rows.find((r) => r.key === key);
      if (existing) existing.value = value;
      else {
        // Fill an empty row first, so pasting into a blank form doesn't
        // leave a stray one behind
        const blank = rows.find((r) => !r.stack && !r.key && !r.value);
        if (blank) Object.assign(blank, { key, value, secret: looksSecret(key) });
        else rows.push(newRow({ key, value, secret: looksSecret(key) }));
      }
    }
    return entries.length;
  }

  function onPaste(e: ClipboardEvent) {
    const text = e.clipboardData?.getData("text") ?? "";
    if (text.includes("=") && (text.includes("\n") || /^[A-Za-z_][A-Za-z0-9_]*=/.test(text.trim()))) {
      if (merge(text) > 0) e.preventDefault();
    }
  }

  function applyPasted() {
    merge(pasted);
    pasted = "";
    pasting = false;
  }

  function placeholder(row: VarRow): string {
    if (row.stored) return "Stored; leave blank to keep it";
    if (isRequired(row)) return row.preset?.generate ? "Required; generate one" : "Required";
    if (row.preset?.from_hostname && !row.stack?.has_default) return "From the first hostname";
    if (row.stack?.has_default) return row.stack.default ? `Default: ${row.stack.default}` : "Empty by default";
    if (row.stack) return "Not set";
    return "Value";
  }
</script>

{#snippet rowView(row: VarRow)}
  {@const badKey = !!row.key && !isEnvKey(row.key)}
  <li class="py-2.5 border-b border-border/70 last:border-0">
    <div class="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto] sm:grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto_auto] gap-2 items-center">
      {#if row.stack}
        <span class="font-mono text-[13px] break-all flex items-center gap-2" title={row.key}>
          {row.key}
          {#if isRequired(row)}
            <span class="font-sans text-[11px] uppercase tracking-wider text-warning">required</span>
          {/if}
        </span>
      {:else if row.preset}
        <!-- Named by the catalog, so it can be removed but not renamed -->
        <span class="font-mono text-[13px] break-all flex items-center gap-2" title={row.key}>
          {row.key}
          {#if isRequired(row)}
            <span class="font-sans text-[11px] uppercase tracking-wider text-warning">required</span>
          {/if}
        </span>
      {:else}
        <input
          bind:value={row.key}
          onpaste={onPaste}
          class="input font-mono h-9 {badKey || duplicate.has(row.key) ? 'border-destructive' : ''}"
          placeholder="NAME"
          aria-label="Variable name"
          autocomplete="off"
          spellcheck="false"
          oninput={() => {
            if (!row.value && looksSecret(row.key)) row.secret = true;
          }}
        />
      {/if}
      <input
        bind:value={row.value}
        onpaste={row.stack ? undefined : onPaste}
        type={row.secret && !shown.has(row.id) ? "password" : "text"}
        class="input font-mono h-9"
        placeholder={placeholder(row)}
        aria-label="Value of {row.key || 'the variable'}"
        autocomplete={row.secret && !shown.has(row.id) ? "new-password" : "off"}
        spellcheck="false"
      />
      <div class="flex items-center gap-0.5">
      {#if row.value && row.secret}
        <button
          type="button"
          class="h-9 w-9 grid place-items-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted"
          aria-label="{shown.has(row.id) ? 'Hide' : 'Show'} the value of {row.key}"
          aria-pressed={shown.has(row.id)}
          title={shown.has(row.id) ? "Hide the value" : "Show the value"}
          onclick={() => toggleShown(row)}
        >
          {#if shown.has(row.id)}<EyeOff class="w-4 h-4" />{:else}<Eye class="w-4 h-4" />{/if}
        </button>
        <button
          type="button"
          class="h-9 w-9 grid place-items-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted"
          aria-label="Copy the value of {row.key}"
          title="Copy the value"
          onclick={() => copyValue(row)}
        >
          <Copy class="w-4 h-4" />
        </button>
      {/if}
      {#if canGenerate(row)}
        <button
          type="button"
          class="h-9 w-9 grid place-items-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted"
          aria-label={row.stored && !row.value ? `Generate a new value for ${row.key}` : `Generate a value for ${row.key}`}
          title={row.stored && !row.value ? "Generate a new value to replace the stored one" : row.value ? "Generate a different value" : "Generate a random value"}
          onclick={() => onGenerate(row)}
        >
          <Dices class="w-4 h-4" />
        </button>
      {/if}
      <button
        type="button"
        class="h-9 w-9 grid place-items-center rounded-md border {row.secret
          ? 'border-brass/60 text-brass bg-brass/10'
          : 'border-transparent text-muted-foreground hover:text-foreground hover:bg-muted'}"
        aria-pressed={row.secret}
        aria-label="{row.secret ? 'Stored as a secret' : 'Stored in plain text'}, click to change"
        title={row.secret
          ? "Secret: encrypted on the server and never shown again"
          : "Plain: visible in the deployment history. Click to store it as a secret"}
        onclick={() => (row.secret = !row.secret)}
      >
        {#if row.secret}<Lock class="w-4 h-4" />{:else}<LockOpen class="w-4 h-4" />{/if}
      </button>
      </div>
      {#if !row.stack}
        <button
          type="button"
          class="h-9 w-9 hidden sm:grid place-items-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted"
          aria-label="Remove {row.key || 'this variable'}"
          onclick={() => remove(row)}
        >
          <X class="w-4 h-4" />
        </button>
      {:else}
        <span class="hidden sm:block w-9"></span>
      {/if}
    </div>
    {#if rotating === row.id}
      <div class="mt-2 rounded-md border border-warning/50 bg-warning/10 px-3 py-2 text-sm">
        <p>
          Replace the stored value of {row.key}? If the app already uses it, such as a database password that was set
          on first start, changing it here can lock the app out until it is changed there too.
        </p>
        <div class="mt-2 flex gap-2">
          <Button size="sm" type="button" onclick={() => generate(row)}>Generate a new value</Button>
          <Button size="sm" type="button" variant="ghost" onclick={() => (rotating = null)}>Keep the stored one</Button>
        </div>
      </div>
    {/if}
    {#if shown.has(row.id) && row.value && row.secret}
      <p class="hint mt-1">Copy this now. Once launched it is stored encrypted and not shown again.</p>
    {/if}
    {#if badKey}
      <p class="hint text-destructive">Letters, digits and underscores, not starting with a digit.</p>
    {:else if duplicate.has(row.key)}
      <p class="hint text-destructive">{row.key} is set twice.</p>
    {:else if row.preset?.description || row.stack?.description}
      <p class="hint mt-1">{row.preset?.description || row.stack?.description}</p>
    {/if}
  </li>
{/snippet}

<div>
  {#if missing.length > 1}
    <div class="mb-1 flex items-center gap-3 flex-wrap">
      <Button size="sm" type="button" variant="outline" onclick={generateMissing}>
        <Dices /> Generate {missing.length} missing secrets
      </Button>
      <span class="hint">Random values, shown so you can copy them.</span>
    </div>
  {/if}
  {#if main.length > 0}
    <ul>
      {#each main as row (row.id)}
        {@render rowView(row)}
      {/each}
    </ul>
  {:else if defaults.length === 0}
    <p class="text-sm text-muted-foreground py-2">
      {fromStack ? "The stack doesn't read any variables." : "No variables yet."}
    </p>
  {/if}

  {#if defaults.length > 0}
    <button
      type="button"
      class="mt-2 flex items-center gap-1.5 text-sm font-medium hover:text-brass"
      aria-expanded={showDefaults}
      onclick={() => (showDefaults = !showDefaults)}
    >
      <ChevronRight class="w-4 h-4 transition-transform {showDefaults ? 'rotate-90' : ''}" />
      {main.length > 0 ? `${defaults.length} more with defaults` : `${defaults.length} variables, all with defaults`}
      {#if changedDefaults > 0}<span class="text-muted-foreground font-normal">, {changedDefaults} changed</span>{/if}
    </button>
    {#if showDefaults}
      <ul class="mt-1 pl-5 border-l-2 border-border">
        {#each defaults as row (row.id)}
          {@render rowView(row)}
        {/each}
      </ul>
    {/if}
  {/if}

  {#if pasting}
    <div class="mt-4 space-y-2">
      <label for="paste-env" class="label">Paste a .env file</label>
      <textarea
        id="paste-env"
        bind:value={pasted}
        rows="5"
        class="input h-auto py-2 font-mono text-[13px]"
        placeholder={"DATABASE_URL=postgres://…\nLOG_LEVEL=info"}
        spellcheck="false"
      ></textarea>
      <div class="flex gap-2">
        <Button size="sm" type="button" onclick={applyPasted} disabled={!pasted.trim()}>Add these</Button>
        <Button size="sm" type="button" variant="ghost" onclick={() => ((pasting = false), (pasted = ""))}>Cancel</Button>
      </div>
    </div>
  {:else}
    <div class="mt-3 flex flex-wrap gap-2">
      <Button size="sm" type="button" variant="outline" onclick={add}><Plus /> Add variable</Button>
      <Button size="sm" type="button" variant="ghost" onclick={() => (pasting = true)}><ClipboardPaste /> Paste .env</Button>
    </div>
  {/if}
</div>
