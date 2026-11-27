<script lang="ts">
  import { onMount } from "svelte";
  import {
    api,
    ApiError,
    type Server,
    type Spec,
    type StackInfo,
    type ImageInfo,
    type Deployment,
  } from "$lib/api";
  import {
    isName,
    slugify,
    nameFromImage,
    nameFromRepo,
    parseRepo,
    splitCommand,
    joinCommand,
    isEnvKey,
    commonDomain,
    newRow,
    type VarRow,
  } from "$lib/launch";
  import { toast } from "$lib/toast";
  import { can } from "$lib/session";
  import Button from "./ui/Button.svelte";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import VariablesEditor from "./launch/VariablesEditor.svelte";
  import HostnamesInput from "./launch/HostnamesInput.svelte";
  import { Box, GitBranch, FileCode, X, LoaderCircle, CircleCheck, Rocket, HardDrive } from "lucide-svelte";

  interface Props {
    // Both set: configure this existing environment instead of launching
    app?: string;
    env?: string;
  }

  let { app: editApp, env: editEnv }: Props = $props();
  const editing = !!(editApp && editEnv);

  type Kind = "image" | "git" | "compose";

  const kinds: { id: Kind; title: string; detail: string; Icon: typeof Box }[] = [
    { id: "image", title: "Container image", detail: "One container from a registry, like postgres:16", Icon: Box },
    { id: "git", title: "Compose from Git", detail: "A stack's repository, like getsentry/self-hosted", Icon: GitBranch },
    { id: "compose", title: "Paste compose", detail: "A docker-compose.yml you have at hand", Icon: FileCode },
  ];

  let kind = $state<Kind>("image");

  // What to run
  let image = $state("");
  let repo = $state("");
  let ref = $state("");
  let composePath = $state("");
  let composeText = $state("");

  // What the server read about it
  let imageInfo = $state<ImageInfo | null>(null);
  let stack = $state<StackInfo | null>(null);
  let inspecting = $state(false);
  let inspectError = $state<string | null>(null);
  // Whatever source the current inspect result is for, to tell when it's stale
  let inspectedFor = $state("");

  // Where it runs
  let appName = $state("");
  let envName = $state("prod");
  let node = $state("");
  let nameTouched = false;

  // How it's reached
  let hostnames = $state<string[]>([]);
  let port = $state<number | null>(null);
  let service = $state("");
  let portTouched = false;

  let rows = $state<VarRow[]>([]);

  // Stacks from Git: a command run on the server before the stack starts
  let setupCommand = $state("");
  let setupAlways = $state(false);
  let setupForce = $state(false);
  // Filled in from the repository rather than typed
  let setupSuggested = $state(false);
  let setupTouched = false;

  // Image only
  let volumes = $state<{ source: string; target: string }[]>([]);
  let command = $state("");
  let healthcheck = $state("");
  let showAdvanced = $state(false);

  // Context
  let servers = $state<Server[]>([]);
  let existing = $state(new Set<string>());
  let domain = $state<string | null>(null);
  let npmConfigured = $state(true);
  let current = $state<Deployment | null>(null);

  let loaded = $state(false);
  let loadError = $state<string | null>(null);
  let submitting = $state(false);
  let submitError = $state<string | null>(null);
  let attempted = $state(false);

  let sourceKey = $derived(
    kind === "image" ? `image:${image.trim()}` : kind === "git" ? `git:${repo.trim()}#${ref.trim()}:${composePath.trim()}` : `compose:${composeText}`
  );
  let stale = $derived(inspectedFor !== sourceKey);

  let stackServices = $derived(stack?.services ?? []);
  let exposedService = $derived(stackServices.find((s) => s.name === service));
  let published = $derived(
    stackServices.flatMap((s) => s.ports.filter((p) => p.published).map((p) => ({ service: s.name, port: p })))
  );
  // "$SENTRY_BIND" reads better as what it comes to, 9000
  function resolvePort(published: string): string {
    const m = published.match(/^\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?$/);
    if (!m) return published;
    const row = rows.find((r) => r.key === m[1]);
    return row?.value || row?.stack?.default || published;
  }
  // Variables the setup command reads, like $SENTRY_ADMIN_PASSWORD
  let setupVariables = $derived([...new Set([...setupCommand.matchAll(/\$\{?([A-Za-z_][A-Za-z0-9_]*)/g)].map((m) => m[1]))]);
  // Sentry's installer can't create the first user without asking for it
  let offerFirstUser = $derived(setupCommand.includes("--skip-user-creation") && !setupCommand.includes("createuser"));
  let suggestion = $derived(domain && isName(appName) ? `${appName}.${domain}` : null);
  let conflict = $derived(!editing && existing.has(`${appName}/${envName}`));
  let unmountedVolumes = $derived(
    (imageInfo?.volumes ?? []).filter((path) => !volumes.some((v) => v.target.trim() === path))
  );

  let problems = $derived.by(() => {
    const p: string[] = [];
    if (kind === "image" && !image.trim()) p.push("Name the image to run.");
    if (kind === "git" && !parseRepo(repo)) p.push("Give the repository's address.");
    if (kind === "compose" && !composeText.trim()) p.push("Paste the compose file.");
    if (kind !== "image" && (stale || !stack?.services) && !inspectError && (repo.trim() || composeText.trim()))
      p.push(kind === "git" ? "Read the repository first." : "Wait for the compose file to be read.");
    if (kind !== "image" && inspectError && !stale) p.push(kind === "git" ? "The repository couldn't be read." : "The compose file has a problem.");
    if (!isName(appName)) p.push("Give the app a name of lowercase letters, digits and dashes.");
    if (!isName(envName)) p.push("Give the environment a name of lowercase letters, digits and dashes.");
    if (!node) p.push("Pick a server.");
    if (conflict) p.push(`${appName}/${envName} already exists.`);
    if (hostnames.length && !port) p.push("Give the port the hostnames route to.");
    if (hostnames.length && kind !== "image" && !service) p.push("Pick the service the hostnames route to.");
    if (port !== null && (port < 1 || port > 65535)) p.push("The port must be between 1 and 65535.");
    const missing = rows.filter((r) => r.stack?.required && !r.value && !r.stored).map((r) => r.key);
    if (missing.length) p.push(`Set ${missing.join(", ")}.`);
    if (rows.some((r) => r.key && !isEnvKey(r.key))) p.push("Fix the variable names marked in red.");
    const keys = rows.map((r) => r.key).filter(Boolean);
    if (new Set(keys).size !== keys.length) p.push("A variable is set twice.");
    if (rows.some((r) => !r.key && r.value)) p.push("A variable has a value but no name.");
    if (volumes.some((v) => (v.source.trim() || v.target.trim()) && !validVolume(v))) p.push("Fix the volumes: a name or /host/path, and a /container/path.");
    if (kind === "git") {
      for (const name of setupVariables) {
        const row = rows.find((r) => r.key === name);
        if (row && !row.value && !row.stored && !row.stack?.has_default) p.push(`Set ${name}, which the setup command uses.`);
      }
    }
    return p;
  });

  function validVolume(v: { source: string; target: string }) {
    const source = v.source.trim();
    return (/^\//.test(source) || /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/.test(source)) && /^\//.test(v.target.trim());
  }

  onMount(async () => {
    try {
      const [serverList, apps, config] = await Promise.all([api.servers.list(), api.apps.list(), api.config()]);
      servers = serverList;
      existing = new Set(apps.map((a) => `${a.environment.app}/${a.environment.name}`));
      domain = commonDomain(apps.flatMap((a) => a.current_deployment?.spec?.hostnames ?? []));
      npmConfigured = config.npm_configured;
      node = (serverList.find((s) => s.is_default) ?? serverList[0])?.name ?? "";
      if (editing) await loadExisting();
      loaded = true;
    } catch (e) {
      loadError = e instanceof ApiError ? e.message : "Couldn't load what the form needs";
    }
  });

  // Fills the form from what runs now, so changing it is a redeploy
  async function loadExisting() {
    const [detail, stored] = await Promise.all([
      api.apps.get(editApp!, editEnv!),
      api.apps.secrets.list(editApp!, editEnv!),
    ]);
    const deps = detail.deployments;
    current = deps.find((d) => d.id === detail.environment.current_deployment_id) ?? deps[0] ?? null;
    const spec = current?.spec;
    appName = editApp!;
    envName = editEnv!;
    node = detail.environment.server;
    nameTouched = portTouched = true;
    if (!spec) return;

    hostnames = [...(spec.hostnames ?? [])];
    port = spec.port || null;
    const storedKeys = new Set(stored.map((s) => s.key));
    rows = [
      ...Object.entries(spec.env ?? {}).map(([key, value]) => newRow({ key, value })),
      ...(spec.secrets ?? []).map((key) => newRow({ key, secret: true, stored: storedKeys.has(key) })),
    ];

    if (spec.source) {
      kind = "git";
      setupTouched = true;
      setupCommand = spec.setup?.command ?? "";
      setupAlways = !!spec.setup?.always;
      repo = spec.source.repo;
      ref = spec.source.ref ?? "";
      composePath = spec.source.path ?? "";
      service = spec.expose?.service ?? "";
    } else if (spec.compose) {
      kind = "compose";
      composeText = spec.compose;
      service = spec.expose?.service ?? "";
    } else {
      kind = "image";
      image = spec.image ?? "";
      volumes = (spec.volumes ?? []).map((v) => {
        const i = v.indexOf(":");
        return { source: v.slice(0, i), target: v.slice(i + 1) };
      });
      command = spec.command?.length ? joinCommand(spec.command) : "";
      healthcheck = spec.healthcheck ?? "";
      showAdvanced = volumes.length > 0 || !!command || !!healthcheck;
    }
    inspect();
  }

  // One inspect at a time; a newer one makes older answers irrelevant
  let seq = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function inspectSoon(delay = 700) {
    clearTimeout(timer);
    timer = setTimeout(inspect, delay);
  }

  async function inspect() {
    clearTimeout(timer);
    const key = sourceKey;
    const mine = ++seq;
    inspectError = null;

    if (kind === "image") {
      const ref = image.trim();
      if (!ref) return;
      if (!nameTouched) appName = nameFromImage(ref);
      inspecting = true;
      try {
        const info = await api.launch.inspectImage(ref);
        if (mine !== seq) return;
        imageInfo = info;
        if (!portTouched && info.port) port = info.port;
      } catch (e) {
        if (mine !== seq) return;
        imageInfo = null;
        inspectError = e instanceof ApiError ? e.message : "Couldn't read the image";
      }
    } else {
      let body: { source: { repo: string; ref?: string; path?: string } } | { compose: string };
      if (kind === "git") {
        const link = parseRepo(repo);
        if (!link) return;
        if (!nameTouched) appName = nameFromRepo(link.repo);
        body = { source: { repo: link.repo, ref: ref.trim() || undefined, path: composePath.trim() || undefined } };
      } else {
        if (!composeText.trim()) return;
        body = { compose: composeText };
      }
      inspecting = true;
      try {
        const info = await api.launch.inspectStack(body);
        if (mine !== seq) return;
        stack = info;
        if (info.services) {
          if (kind === "git" && !setupTouched) {
            setupCommand = info.setup ?? "";
            setupSuggested = !!info.setup;
          }
          if (!info.services.some((s) => s.name === service)) service = info.service ?? "";
          if (!portTouched && info.port) port = info.port;
          mergeStackVariables(info);
        }
      } catch (e) {
        if (mine !== seq) return;
        stack = null;
        inspectError = e instanceof ApiError ? e.message : "Couldn't read the stack";
      }
    }
    inspecting = false;
    inspectedFor = key;
  }

  // Rows for what the stack reads, keeping anything already typed
  function mergeStackVariables(info: StackInfo) {
    const vars = info.variables ?? [];
    const names = new Set(vars.map((v) => v.name));
    // Drop untouched rows of a previous read that the stack no longer has
    rows = rows.filter((r) => !r.stack || names.has(r.key) || r.value || r.stored);
    for (const r of rows) if (r.stack && !names.has(r.key)) r.stack = undefined;
    for (const v of vars) {
      const row = rows.find((r) => r.key === v.name);
      if (row) row.stack = v;
      else rows.push(newRow({ key: v.name, value: v.suggested ?? "", secret: !!v.secret, stack: v }));
    }
  }

  function chooseKind(k: Kind) {
    if (k === kind) return;
    kind = k;
    imageInfo = null;
    stack = null;
    inspectError = null;
    inspectedFor = "";
    // Variables read from the previous source don't apply to this one
    rows = rows.filter((r) => !r.stack || r.value);
    for (const r of rows) r.stack = undefined;
    if (!portTouched) port = null;
    service = "";
    if (sourceKey.split(":").slice(1).join(":").trim()) inspect();
  }

  // People paste the browser's address, which may name a branch and a file
  function onRepoCommit() {
    const link = parseRepo(repo);
    if (!link) return;
    repo = link.repo;
    if (link.ref && !ref) ref = link.ref;
    if (link.path && !composePath) composePath = link.path;
    if (stale) inspect();
  }

  function selectService(name: string) {
    service = name;
    const svc = stackServices.find((s) => s.name === name);
    if (svc?.ports.length) port = svc.ports[0].target;
  }

  async function submit() {
    attempted = true;
    if (problems.length) return;
    submitting = true;
    submitError = null;

    const spec: Spec = {
      app: appName,
      environment: envName,
      node,
      origin: "ui",
    };
    if (kind === "image") {
      spec.image = image.trim();
      const vols = volumes.filter((v) => v.source.trim() && v.target.trim()).map((v) => `${v.source.trim()}:${v.target.trim()}`);
      if (vols.length) spec.volumes = vols;
      const cmd = splitCommand(command);
      if (cmd.length) spec.command = cmd;
      if (healthcheck.trim()) spec.healthcheck = healthcheck.trim().startsWith("/") ? healthcheck.trim() : `/${healthcheck.trim()}`;
    } else if (kind === "git") {
      const link = parseRepo(repo)!;
      // No commit: deploying from the form takes the latest of the ref
      spec.source = { repo: link.repo, ref: ref.trim() || undefined, path: (composePath.trim() || stack?.compose_file) ?? undefined };
      if (setupCommand.trim()) {
        spec.setup = { command: setupCommand.trim(), always: setupAlways || undefined, force: setupForce || undefined };
      }
    } else {
      spec.compose = composeText;
    }
    if (hostnames.length) {
      spec.hostnames = [...hostnames];
      spec.port = port ?? undefined;
      if (kind !== "image") spec.expose = { service, port: port! };
    } else if (kind === "image" && port) {
      spec.port = port;
    }

    const env: Record<string, string> = {};
    const secretNames: string[] = [];
    const secretValues: Record<string, string> = {};
    for (const r of rows) {
      if (!r.key) continue;
      if (r.secret) {
        if (r.value) secretValues[r.key] = r.value;
        if (r.value || r.stored) secretNames.push(r.key);
      } else if (r.value !== "" || !r.stack) {
        // Blank stack variables are left to their defaults
        env[r.key] = r.value;
      }
    }
    if (Object.keys(env).length) spec.env = env;
    if (secretNames.length) spec.secrets = secretNames;

    try {
      if (Object.keys(secretValues).length) await api.apps.secrets.set(appName, envName, secretValues);
      const dep = await api.deployments.create(spec);
      toast(`${editing ? "Deploying changes to" : "Launching"} ${appName}/${envName}: deployment #${dep.id}`);
      location.hash = `#/apps/${encodeURIComponent(appName)}/${encodeURIComponent(envName)}`;
    } catch (e) {
      submitError = e instanceof ApiError ? e.message : "Couldn't start the deployment";
      submitting = false;
    }
  }

  // Adds a step that creates Sentry's first user from two variables
  function addFirstUser() {
    setupCommand =
      setupCommand.trimEnd() +
      '\ndocker compose run --rm web createuser --email "$SENTRY_ADMIN_EMAIL" --password "$SENTRY_ADMIN_PASSWORD" --superuser --no-input || echo "No user was created; it may exist already"';
    setupTouched = true;
    setupSuggested = false;
    if (!rows.some((r) => r.key === "SENTRY_ADMIN_EMAIL")) rows.push(newRow({ key: "SENTRY_ADMIN_EMAIL" }));
    if (!rows.some((r) => r.key === "SENTRY_ADMIN_PASSWORD")) rows.push(newRow({ key: "SENTRY_ADMIN_PASSWORD", secret: true }));
  }

  function describeSource(): string {
    if (kind === "image") return image.trim() || "an image";
    if (kind === "git") {
      const link = parseRepo(repo);
      const name = link ? link.repo.replace(/^https:\/\//, "") : "a repository";
      return ref.trim() ? `${name} at ${ref.trim()}` : name;
    }
    return "the pasted compose file";
  }
</script>

{#snippet section(n: number, title: string, intro: string | null, body: import("svelte").Snippet)}
  <section class="riveted px-6 sm:px-8 py-6">
    <h2 class="section-title flex items-baseline gap-3">
      <span class="nixie text-[13px]">{n}</span>{title}
    </h2>
    {#if intro}<p class="text-sm text-muted-foreground mt-1.5 max-w-2xl">{intro}</p>{/if}
    <div class="mt-5">{@render body()}</div>
  </section>
{/snippet}

{#snippet inspectStatus()}
  {#if inspecting}
    <p class="hint flex items-center gap-1.5"><LoaderCircle class="w-3.5 h-3.5 animate-spin" /> Reading…</p>
  {:else if inspectError && !stale}
    <p class="hint text-destructive">{inspectError}</p>
  {:else if kind === "image" && imageInfo && !stale}
    <p class="hint flex items-start gap-1.5">
      <CircleCheck class="w-3.5 h-3.5 mt-[3px] text-success shrink-0" />
      <span>
        {imageInfo.ports.length ? `Listens on ${imageInfo.ports.join(", ")}.` : "Declares no ports."}
        {#if imageInfo.volumes.length}Keeps data in <span class="font-mono">{imageInfo.volumes.join(", ")}</span>.{/if}
        {#if imageInfo.healthcheck}Has its own health check.{/if}
      </span>
    </p>
  {:else if kind !== "image" && stack?.services && !stale}
    <p class="hint flex items-start gap-1.5">
      <CircleCheck class="w-3.5 h-3.5 mt-[3px] text-success shrink-0" />
      <span>
        {#if stack.compose_file}Read <span class="font-mono">{stack.compose_file}</span>{#if stack.commit} at <span class="font-mono">{stack.commit.slice(0, 7)}</span>{/if}:{:else}Read:{/if}
        {stack.services.length} {stack.services.length === 1 ? "service" : "services"},
        {stack.variables?.length ?? 0} {stack.variables?.length === 1 ? "variable" : "variables"}.
      </span>
    </p>
  {/if}
{/snippet}

{#if !$can("deployer")}
  <Alert>Launching apps needs the deployer role.</Alert>
{:else if loadError}
  <Alert>{loadError}</Alert>
{:else if !loaded}
  <Loading label={editing ? `Loading ${editApp}/${editEnv}` : "Getting ready"} />
{:else}
  <header class="mb-8">
    {#if editing}
      <a href="#/apps/{encodeURIComponent(editApp!)}/{encodeURIComponent(editEnv!)}" class="engraved text-lg text-muted-foreground hover:text-brass">{editApp} / {editEnv}</a>
      <h1 class="page-title mt-0.5">Configure {editEnv}</h1>
      <p class="text-muted-foreground mt-1.5 max-w-2xl">
        Change what runs and how, then deploy the changes. The running version keeps serving until the new one is up.
      </p>
    {:else}
      <h1 class="page-title">Launch an app</h1>
      <p class="text-muted-foreground mt-1.5 max-w-2xl">
        Run a container image or a compose stack on one of your servers, without a repository of your own or a
        docktrine.yaml.
      </p>
    {/if}
  </header>

  {#if editing && current?.spec && current.spec.origin !== "ui"}
    <Alert kind="warning" class="mb-6">
      {editApp} was deployed with the CLI from its docktrine.yaml. Changes made here last until its next CLI deploy,
      which replaces them, so change docktrine.yaml too.
    </Alert>
  {/if}

  <form
    class="grid lg:grid-cols-[minmax(0,1fr)_20rem] gap-6 items-start"
    novalidate
    onsubmit={(e) => {
      e.preventDefault();
      submit();
    }}
  >
    <div class="space-y-6 min-w-0">
      {#snippet what()}
        {#if !editing}
          <div class="grid sm:grid-cols-3 gap-3 mb-6" role="radiogroup" aria-label="What to run">
            {#each kinds as k}
              <button
                type="button"
                role="radio"
                aria-checked={kind === k.id}
                onclick={() => chooseKind(k.id)}
                class="text-left rounded-md p-4 border-2 transition-colors
                  {kind === k.id ? 'border-brass bg-brass/[0.08]' : 'border-border bg-plate/40 hover:border-brass/50'}"
              >
                <k.Icon class="w-5 h-5 {kind === k.id ? 'text-brass' : 'text-muted-foreground'}" />
                <span class="block font-semibold mt-2">{k.title}</span>
                <span class="block text-[13px] text-muted-foreground mt-0.5">{k.detail}</span>
              </button>
            {/each}
          </div>
        {/if}

        {#if kind === "image"}
          <label for="image" class="label">Image</label>
          <input
            id="image"
            bind:value={image}
            oninput={() => inspectSoon()}
            onblur={() => stale && inspect()}
            class="input font-mono"
            placeholder="postgres:16, ghcr.io/owner/app:1.2"
            autocomplete="off"
            spellcheck="false"
          />
          {@render inspectStatus()}
          {#if !inspectError || stale}
            <p class="hint">From Docker Hub or any registry. Private images work from your central registry.</p>
          {/if}
        {:else if kind === "git"}
          <label for="repo" class="label">Repository</label>
          <div class="flex gap-2">
            <input
              id="repo"
              bind:value={repo}
              onblur={onRepoCommit}
              onkeydown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  onRepoCommit();
                }
              }}
              class="input font-mono flex-1"
              placeholder="https://github.com/getsentry/self-hosted"
              autocomplete="off"
              spellcheck="false"
            />
            <Button type="button" variant="outline" class="h-10" onclick={() => (onRepoCommit(), inspect())} disabled={!parseRepo(repo) || inspecting}>
              {stack && !stale ? "Read again" : "Read"}
            </Button>
          </div>
          {@render inspectStatus()}
          {#if !inspectError || stale}
            <p class="hint">A public repository with a compose file. Its other files, like configs it mounts, go to the server with it.</p>
          {/if}
          <div class="grid sm:grid-cols-2 gap-4 mt-4">
            <div>
              <label for="ref" class="label">Branch, tag or commit <span class="text-muted-foreground font-normal">(optional)</span></label>
              <input id="ref" bind:value={ref} onblur={() => stale && parseRepo(repo) && inspect()} class="input font-mono" placeholder="The default branch" autocomplete="off" spellcheck="false" />
            </div>
            <div>
              <label for="compose-path" class="label">Compose file <span class="text-muted-foreground font-normal">(optional)</span></label>
              {#if stack?.compose_files?.length && !stack.services}
                <select id="compose-path" class="input font-mono" bind:value={composePath} onchange={inspect}>
                  <option value="" disabled>Pick one</option>
                  {#each stack.compose_files as f}<option value={f}>{f}</option>{/each}
                </select>
                <p class="hint text-warning">There's no compose file at the root. Pick the one to run.</p>
              {:else}
                <input id="compose-path" bind:value={composePath} onblur={() => stale && parseRepo(repo) && inspect()} class="input font-mono" placeholder={stack?.compose_file ?? "Found at the root"} autocomplete="off" spellcheck="false" />
              {/if}
            </div>
          </div>
          {#if editing && current?.spec?.source?.commit}
            <p class="hint mt-3">
              Running <span class="font-mono">{current.spec.source.commit.slice(0, 7)}</span>. Deploying from here takes the latest of
              {ref.trim() || "the default branch"}; pin a tag or commit above to stay on one version.
            </p>
          {/if}
        {:else}
          <label for="compose-text" class="label">docker-compose.yml</label>
          <textarea
            id="compose-text"
            bind:value={composeText}
            oninput={() => inspectSoon(900)}
            rows="12"
            class="input h-auto py-2.5 font-mono text-[13px] leading-relaxed"
            placeholder={"services:\n  web:\n    image: nginx\n    ports: [\"80\"]"}
            spellcheck="false"
          ></textarea>
          {@render inspectStatus()}
          {#if !inspectError || stale}
            <p class="hint">Only the file goes to the server. A stack that mounts files next to it should come from Git instead.</p>
          {/if}
        {/if}
      {/snippet}
      {@render section(1, "What to run", null, what)}

      {#snippet where()}
        <div class="grid sm:grid-cols-3 gap-4">
          <div>
            <label for="app-name" class="label">App</label>
            <input
              id="app-name"
              bind:value={appName}
              oninput={() => ((nameTouched = true), (appName = appName.toLowerCase()))}
              onblur={() => (appName = slugify(appName))}
              disabled={editing}
              class="input font-mono {attempted && !isName(appName) ? 'border-destructive' : ''}"
              placeholder="sentry"
              autocomplete="off"
              spellcheck="false"
            />
          </div>
          <div>
            <label for="env-name" class="label">Environment</label>
            <input
              id="env-name"
              bind:value={envName}
              oninput={() => (envName = envName.toLowerCase())}
              onblur={() => (envName = slugify(envName))}
              disabled={editing}
              class="input font-mono {attempted && !isName(envName) ? 'border-destructive' : ''}"
              placeholder="prod"
              autocomplete="off"
              spellcheck="false"
            />
          </div>
          <div>
            <label for="node" class="label">Server</label>
            <select id="node" bind:value={node} class="input" disabled={editing}>
              {#each servers as s}<option value={s.name}>{s.name}</option>{/each}
            </select>
          </div>
        </div>
        {#if conflict}
          <Alert kind="warning" class="mt-4">
            {appName}/{envName} already exists.
            <a class="underline underline-offset-2 font-medium" href="#/apps/{encodeURIComponent(appName)}/{encodeURIComponent(envName)}/configure">Configure it</a>
            to change it, or pick another name.
          </Alert>
        {:else if editing}
          <p class="hint">To rename it or move it to another server, launch it again under the new name and destroy this one.</p>
        {:else}
          <p class="hint">It shows up as <span class="font-mono">{appName || "app"}/{envName || "env"}</span> among your apps.</p>
        {/if}
      {/snippet}
      {@render section(2, "Name and place", null, where)}

      {#snippet reach()}
        <label for="hostnames" class="label">Hostnames <span class="text-muted-foreground font-normal">(optional)</span></label>
        <HostnamesInput id="hostnames" bind:hostnames {suggestion} />
        {#if hostnames.length && !npmConfigured}
          <p class="hint text-warning">
            Nginx Proxy Manager isn't configured on this Docktrine server, so the hostnames won't be set up on the gateway.
          </p>
        {:else if !hostnames.length}
          <p class="hint">Without one it stays private: other containers on {node || "the server"} can still reach it.</p>
        {/if}

        {#if kind === "image"}
          <div class="mt-5 max-w-56">
            <label for="port" class="label">Container port</label>
            <input
              id="port"
              type="number"
              min="1"
              max="65535"
              bind:value={port}
              oninput={() => (portTouched = true)}
              class="input font-mono {attempted && hostnames.length && !port ? 'border-destructive' : ''}"
              placeholder={imageInfo?.port ? String(imageInfo.port) : "8080"}
            />
          </div>
          <p class="hint">The port the app listens on inside the container; hostnames route to it.</p>
        {:else if hostnames.length}
          <div class="mt-5 grid sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)] gap-4">
            <div>
              <label for="service" class="label">Service</label>
              <select
                id="service"
                value={service}
                onchange={(e) => selectService(e.currentTarget.value)}
                class="input {attempted && !service ? 'border-destructive' : ''}"
                disabled={!stackServices.length}
              >
                <option value="" disabled>{stackServices.length ? "Pick the front door" : "Read the stack first"}</option>
                {#each stackServices as s}
                  <option value={s.name}>{s.name}{s.ports.length ? ` (${s.ports.map((p) => p.target).join(", ")})` : ""}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="service-port" class="label">Port</label>
              <input
                id="service-port"
                type="number"
                min="1"
                max="65535"
                bind:value={port}
                oninput={() => (portTouched = true)}
                class="input font-mono {attempted && !port ? 'border-destructive' : ''}"
                list="service-ports"
              />
              <datalist id="service-ports">
                {#each exposedService?.ports ?? [] as p}<option value={p.target}></option>{/each}
              </datalist>
            </div>
          </div>
          <p class="hint">The service hostnames route to, usually the stack's web server or proxy, and the port it listens on inside its container.</p>
        {/if}

        {#if kind !== "image" && published.length}
          <p class="hint mt-4">
            The stack also publishes
            {#each published as p, i}<span class="font-mono">{resolvePort(p.port.published!)}</span> ({p.service}){i < published.length - 1 ? ", " : ""}{/each}
            on {node}'s own network interfaces.
          </p>
        {/if}
      {/snippet}
      {@render section(3, "Hostnames", null, reach)}

      {#snippet variables()}
        <VariablesEditor bind:rows fromStack={kind !== "image" && !!stack?.services} />
      {/snippet}
      {@render section(
        4,
        "Variables",
        kind === "image"
          ? "Environment variables for the container. The lock stores a value as an encrypted secret."
          : "What the stack reads from its environment and its .env file. Leave a value blank to keep its default; the lock stores it as an encrypted secret.",
        variables
      )}

      {#if kind === "git"}
        {#snippet setup()}
          <label for="setup-command" class="label">Setup command <span class="text-muted-foreground font-normal">(optional)</span></label>
          <textarea
            id="setup-command"
            bind:value={setupCommand}
            oninput={() => ((setupTouched = true), (setupSuggested = false))}
            rows={Math.min(8, Math.max(2, setupCommand.split("\n").reduce((n, line) => n + Math.ceil((line.length || 1) / 80), 0) + 1))}
            class="input h-auto py-2.5 font-mono text-[13px] leading-relaxed"
            placeholder="./install.sh"
            spellcheck="false"
          ></textarea>
          {#if setupSuggested && setupCommand}
            <p class="hint">
              Filled in from the repository's install script, with the options that keep it from stopping to ask questions.
            </p>
          {/if}
          <p class="hint">
            Runs in bash, in the stack's directory on {node || "the server"}, with Docker, docker compose and the variables
            above, and stops at the first command that fails. Its output appears in the deploy log. Install scripts can take
            a while; Sentry's takes 10 to 20 minutes the first time.
          </p>

          {#if offerFirstUser}
            <div class="mt-4 rounded-md bg-plate/60 px-4 py-3 text-sm">
              <p>This install skips creating the first user, so there'd be no one to sign in as.</p>
              <button type="button" class="mt-1.5 font-medium text-link hover:underline underline-offset-2" onclick={addFirstUser}>
                Also create the first user</button
              ><span class="text-muted-foreground">, from SENTRY_ADMIN_EMAIL and SENTRY_ADMIN_PASSWORD in Variables</span>
            </div>
          {/if}

          {#if setupCommand.trim()}
            <fieldset class="mt-5">
              <legend class="label">When it runs</legend>
              <div class="space-y-2 text-sm">
                <label class="flex items-start gap-2.5">
                  <input type="radio" name="setup-when" checked={!setupAlways} onchange={() => (setupAlways = false)} class="mt-1 accent-[rgb(var(--brass))]" />
                  <span>
                    On the first deploy, and whenever the stack's commit or this command changes
                    <span class="block hint mt-0.5">The usual for install scripts, which also handle upgrades.</span>
                  </span>
                </label>
                <label class="flex items-start gap-2.5">
                  <input type="radio" name="setup-when" checked={setupAlways} onchange={() => (setupAlways = true)} class="mt-1 accent-[rgb(var(--brass))]" />
                  <span>On every deploy</span>
                </label>
              </div>
            </fieldset>
            {#if editing && !setupAlways}
              <label class="flex items-start gap-2.5 text-sm mt-4">
                <input type="checkbox" bind:checked={setupForce} class="w-4 h-4 mt-0.5 accent-[rgb(var(--brass))]" />
                <span>Run it on this deploy too, even if nothing changed</span>
              </label>
            {/if}
          {/if}
        {/snippet}
        {@render section(5, "Setup", "Some stacks need a step before they start, like an install script that creates their configuration and migrates their database.", setup)}
      {/if}

      {#if kind === "image"}
        {#snippet advanced()}
          {#if unmountedVolumes.length}
            <Alert kind="warning" class="mb-5">
              <span>
                The image keeps data in <span class="font-mono">{unmountedVolumes.join(", ")}</span>, which is lost on every
                redeploy unless it's in a volume.
              </span>
              <span class="block mt-2">
                {#each unmountedVolumes as path}
                  <button
                    type="button"
                    class="mr-3 underline underline-offset-2 font-medium"
                    onclick={() => {
                      volumes.push({ source: slugify(path.split("/").filter(Boolean).pop() ?? "data") || "data", target: path });
                      showAdvanced = true;
                    }}
                  >
                    Keep {path} in a volume
                  </button>
                {/each}
              </span>
            </Alert>
          {/if}

          {#if !showAdvanced}
            <button type="button" class="text-sm font-medium text-link hover:underline underline-offset-2" onclick={() => (showAdvanced = true)}>
              Set volumes, the command or a health check
            </button>
          {:else}
            <fieldset>
              <legend class="label">Volumes</legend>
              {#if volumes.length}
                <ul class="space-y-2 mb-2">
                  {#each volumes as v, i}
                    <li class="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1.5fr)_auto] gap-2 items-center">
                      <input bind:value={v.source} class="input font-mono h-9" placeholder="data or /srv/host/path" aria-label="Volume name or host path" spellcheck="false" />
                      <span class="text-muted-foreground">:</span>
                      <input bind:value={v.target} class="input font-mono h-9" placeholder="/path/in/container" aria-label="Path in the container" spellcheck="false" />
                      <button type="button" class="h-9 w-9 grid place-items-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted" aria-label="Remove volume" onclick={() => volumes.splice(i, 1)}>
                        <X class="w-4 h-4" />
                      </button>
                    </li>
                  {/each}
                </ul>
              {/if}
              <Button size="sm" type="button" variant="outline" onclick={() => volumes.push({ source: "", target: "" })}><HardDrive /> Add volume</Button>
              <p class="hint">
                A name like <span class="font-mono">data</span> is a volume kept across deploys (as
                <span class="font-mono">{appName || "app"}-{envName || "env"}-data</span>); a path starting with / is a directory on the server.
              </p>
            </fieldset>

            <div class="grid sm:grid-cols-2 gap-4 mt-5">
              <div>
                <label for="command" class="label">Command <span class="text-muted-foreground font-normal">(optional)</span></label>
                <input id="command" bind:value={command} class="input font-mono" placeholder="The image's own" autocomplete="off" spellcheck="false" />
              </div>
              <div>
                <label for="healthcheck" class="label">Health check path <span class="text-muted-foreground font-normal">(optional)</span></label>
                <input id="healthcheck" bind:value={healthcheck} class="input font-mono" placeholder="/healthz" autocomplete="off" spellcheck="false" />
              </div>
            </div>
            <p class="hint">
              With a health check, the new version takes traffic once the path answers on the container port; the image needs wget or curl.
              Without one, it has to stay up for 10 seconds.
            </p>
          {/if}
        {/snippet}
        {@render section(5, "Storage and startup", null, advanced)}
      {/if}
    </div>

    <!-- What will happen, and the button that makes it happen -->
    <aside class="lg:sticky lg:top-6 riveted px-6 py-6">
      <h2 class="section-title">{editing ? "Deploy the changes" : "Ready to launch"}</h2>
      <dl class="mt-4 space-y-3 text-sm">
        <div>
          <dt class="text-[13px] text-muted-foreground">Runs</dt>
          <dd class="font-mono text-[13px] break-words [overflow-wrap:anywhere] mt-0.5">{describeSource()}</dd>
        </div>
        <div>
          <dt class="text-[13px] text-muted-foreground">As</dt>
          <dd class="mt-0.5"><span class="engraved">{appName || "…"}/{envName || "…"}</span> on {node || "…"}</dd>
        </div>
        <div>
          <dt class="text-[13px] text-muted-foreground">Reached at</dt>
          <dd class="mt-0.5 break-all">
            {#if hostnames.length}
              {hostnames.join(", ")}{#if port}<span class="text-muted-foreground"> → {kind === "image" ? "" : `${service || "…"}:`}{port}</span>{/if}
            {:else}
              <span class="text-muted-foreground">Private, no hostnames</span>
            {/if}
          </dd>
        </div>
        {#if kind === "git" && setupCommand.trim()}
          <div>
            <dt class="text-[13px] text-muted-foreground">First</dt>
            <dd class="font-mono text-[13px] mt-0.5 [overflow-wrap:anywhere]">{setupCommand.trim().split("\n")[0]}{setupCommand.trim().includes("\n") ? " …" : ""}</dd>
          </div>
        {/if}
        {#if rows.some((r) => r.secret && (r.value || r.stored))}
          {@const n = rows.filter((r) => r.secret && (r.value || r.stored)).length}
          <div>
            <dt class="text-[13px] text-muted-foreground">Secrets</dt>
            <dd class="mt-0.5">{n} stored encrypted</dd>
          </div>
        {/if}
      </dl>

      {#if attempted && problems.length}
        <ul class="mt-5 space-y-1.5 text-sm text-destructive" role="alert">
          {#each problems as p}<li class="flex gap-2"><span aria-hidden="true">•</span>{p}</li>{/each}
        </ul>
      {/if}
      {#if submitError}
        <Alert class="mt-5">{submitError}</Alert>
      {/if}

      <Button type="submit" size="lg" class="w-full mt-6" disabled={submitting || inspecting}>
        {#if submitting}
          <LoaderCircle class="animate-spin" /> Starting…
        {:else}
          <Rocket /> {editing ? "Deploy changes" : "Launch"}
        {/if}
      </Button>
      <p class="hint text-center">You'll follow the deploy log next.</p>
    </aside>
  </form>
{/if}
