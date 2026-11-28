<script lang="ts">
  import { onMount } from "svelte";
  import { api, type ShellTarget } from "$lib/api";
  import { can } from "$lib/session";
  import Alert from "./ui/Alert.svelte";
  import Loading from "./ui/Loading.svelte";
  import { ArrowLeft } from "lucide-svelte";

  // xterm is big; load it only once a shell opens
  const loadConsole = import("./Console.svelte");

  // From the URL: #/shell/<server> for the node, #/shell/<server>/<container>
  let { server, container }: { server: string; container?: string } = $props();

  let target = $derived<ShellTarget>(
    container ? { kind: "container", server, id: container } : { kind: "node", server }
  );
  let name = $state<string | null>(null);

  onMount(() => {
    if (!container) return;
    // Only for the title; the shell opens either way
    api.containers.get(container, server).then(
      (c) => (name = c.name.replace(/^\//, "")),
      () => {}
    );
  });

  let allowed = $derived(container ? $can("deployer") : $can("admin"));
  let label = $derived(
    container ? `${name ?? container.slice(0, 12)} on ${server}` : `root@${server}`
  );
</script>

<div class="flex flex-wrap items-end justify-between gap-4 mb-5">
  <div class="min-w-0">
    <a href={container ? "#/containers" : "#/servers"} class="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5">
      <ArrowLeft class="w-3.5 h-3.5" /> {container ? "Containers" : "Servers"}
    </a>
    <h1 class="page-title mt-1 truncate">
      {#if container}Shell in <span class="engraved">{name ?? container.slice(0, 12)}</span>{:else}Shell on <span class="engraved">{server}</span>{/if}
    </h1>
    <p class="text-muted-foreground mt-1">
      {#if container}
        A shell inside the container on {server}, as its default user. Changes outside its volumes are lost when it's recreated.
      {:else}
        A root shell on the node itself, outside any container. Be careful: nothing here is undone by a redeploy.
      {/if}
    </p>
  </div>
</div>

{#if !allowed}
  <Alert>{container ? "Opening a shell in a container needs the deployer role." : "Opening a shell on a node needs the admin role."}</Alert>
{:else}
  {#await loadConsole}
    <Loading label="Loading the terminal" />
  {:then { default: Console }}
    <Console {target} {label} class="h-[calc(100dvh-16rem)] min-h-[22rem]" />
  {/await}
  <p class="hint">
    Copy with {navigator.platform.toUpperCase().includes("MAC") ? "⌘C" : "Ctrl+Shift+C"}, paste with
    {navigator.platform.toUpperCase().includes("MAC") ? "⌘V" : "Ctrl+Shift+V"}. The shell ends when you leave this page, and
    after 30 minutes without input.
  </p>
{/if}
