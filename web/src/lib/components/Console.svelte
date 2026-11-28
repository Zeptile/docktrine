<script lang="ts">
  import { onMount } from "svelte";
  import { Terminal, type ITheme } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import { WebLinksAddon } from "@xterm/addon-web-links";
  import "@xterm/xterm/css/xterm.css";
  import { openShell, shellRefusal, type ShellMessage, type ShellTarget } from "$lib/api";
  import { cn } from "$lib/utils/cn";
  import Button from "./ui/Button.svelte";
  import { RotateCcw, Copy, ClipboardPaste, SquareTerminal } from "lucide-svelte";

  interface Props {
    target: ShellTarget;
    // What the shell is in, for the toolbar, e.g. "web-1 on local"
    label: string;
    class?: string;
  }

  let { target, label, class: className }: Props = $props();

  type Status =
    | { state: "connecting" }
    | { state: "open"; shell?: string }
    | { state: "exited"; code: number }
    | { state: "closed"; reason: string }
    | { state: "failed"; message: string };

  let status = $state<Status>({ state: "connecting" });
  let hasSelection = $state(false);
  let box: HTMLDivElement;
  let term: Terminal | undefined;
  let fit: FitAddon | undefined;
  let ws: WebSocket | null = null;
  // Set once the server says why it's ending, so onclose doesn't call it a
  // dropped connection
  let ended = false;
  let sessions = 0;

  const encoder = new TextEncoder();
  const isMac = navigator.platform.toUpperCase().includes("MAC");

  // A warm palette for the engine room: verdigris greens, brass yellows,
  // copper reds. The background and text follow the theme's terminal.
  function theme(): ITheme {
    const css = getComputedStyle(document.documentElement);
    const rgb = (name: string) => `rgb(${css.getPropertyValue(name).trim().split(/\s+/).join(", ")})`;
    return {
      background: rgb("--terminal"),
      foreground: rgb("--terminal-foreground"),
      cursor: "#d9ab55",
      cursorAccent: rgb("--terminal"),
      selectionBackground: "rgba(201, 155, 71, 0.35)",
      black: "#2b2520",
      red: "#e0735f",
      green: "#7fbf9f",
      yellow: "#d9ab55",
      blue: "#7f9fc4",
      magenta: "#c48fb4",
      cyan: "#6fbab2",
      white: "#d9ccb0",
      brightBlack: "#7a6d5c",
      brightRed: "#f08d78",
      brightGreen: "#9bd8b9",
      brightYellow: "#f0c470",
      brightBlue: "#9ebbdc",
      brightMagenta: "#dcaacd",
      brightCyan: "#8fd4cc",
      brightWhite: "#f4ead2",
    };
  }

  function send(data: string | Uint8Array) {
    if (ws?.readyState === WebSocket.OPEN) ws.send(data);
  }

  function sendSize() {
    if (term) send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
  }

  function connect() {
    if (!term) return;
    ws?.close();
    ended = false;
    status = { state: "connecting" };
    if (sessions++ > 0) {
      term.write("\r\n\x1b[2m--- new session ---\x1b[0m\r\n");
    }
    fit?.fit();
    const socket = openShell(target, { cols: term.cols, rows: term.rows });
    ws = socket;

    socket.onopen = () => {
      if (ws !== socket) return;
      status = { state: "open" };
      // The size may have changed during the handshake
      sendSize();
      term?.focus();
    };
    socket.onmessage = (e) => {
      if (ws !== socket) return;
      if (typeof e.data !== "string") {
        term?.write(new Uint8Array(e.data as ArrayBuffer));
        return;
      }
      let m: ShellMessage;
      try {
        m = JSON.parse(e.data);
      } catch {
        return;
      }
      if (m.type === "ready") {
        status = { state: "open", shell: m.shell };
      } else if (m.type === "exit") {
        ended = true;
        status = { state: "exited", code: m.code };
        term?.write(`\r\n\x1b[2mThe shell exited with code ${m.code}.\x1b[0m\r\n`);
      } else if (m.type === "closed") {
        ended = true;
        status = { state: "closed", reason: m.reason };
        term?.write(`\r\n\x1b[2m${m.reason}\x1b[0m\r\n`);
      } else if (m.type === "error") {
        ended = true;
        status = { state: "failed", message: m.message };
      }
    };
    socket.onclose = async () => {
      if (ws !== socket) return;
      ws = null;
      if (ended) return;
      if (status.state === "connecting") {
        const reason = await shellRefusal(target);
        status = { state: "failed", message: reason ?? "Couldn't open the shell. The server may be unreachable." };
      } else {
        status = { state: "closed", reason: "The connection dropped." };
      }
    };
  }

  async function copySelection() {
    const text = term?.getSelection();
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // Clipboard blocked; the browser's own copy still works on the selection
    }
    term?.focus();
  }

  async function paste() {
    try {
      const text = await navigator.clipboard.readText();
      if (text) term?.paste(text);
    } catch {
      // Reading the clipboard needs permission; the paste shortcut doesn't
    }
    term?.focus();
  }

  onMount(() => {
    let disposed = false;
    const t = new Terminal({
      fontFamily: '"IBM Plex Mono", ui-monospace, SFMono-Regular, monospace',
      fontSize: 13,
      lineHeight: 1.2,
      fontWeightBold: "500",
      cursorBlink: true,
      scrollback: 5000,
      allowProposedApi: false,
      macOptionIsMeta: true,
      theme: theme(),
    });
    term = t;
    fit = new FitAddon();
    t.loadAddon(fit);
    t.loadAddon(new WebLinksAddon());

    // Ctrl+C is the shell's interrupt, so copying takes Shift as in most
    // terminals; on a Mac, Cmd+C copies and Ctrl+C still interrupts
    t.attachCustomKeyEventHandler((e) => {
      if (e.type !== "keydown") return true;
      const key = e.key.toLowerCase();
      if (!isMac && e.ctrlKey && e.shiftKey && key === "c") {
        e.preventDefault();
        copySelection();
        return false;
      }
      if (!isMac && e.ctrlKey && e.shiftKey && key === "v") {
        // The browser fires a paste event, which xterm handles
        return false;
      }
      return true;
    });
    t.onData((d) => send(encoder.encode(d)));
    t.onBinary((d) => send(Uint8Array.from(d, (c) => c.charCodeAt(0))));
    t.onResize(() => sendSize());
    t.onSelectionChange(() => (hasSelection = t.hasSelection()));

    // Wait for the font, or xterm measures the fallback's cells
    document.fonts.load('13px "IBM Plex Mono"').finally(() => {
      if (disposed) return;
      t.open(box);
      fit?.fit();
      t.focus();
      connect();
    });

    const resize = new ResizeObserver(() => {
      if (box.offsetParent !== null) fit?.fit();
    });
    resize.observe(box);

    // Follow the theme, which toggles the dark class on <html>
    const themeWatch = new MutationObserver(() => (t.options.theme = theme()));
    themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });

    return () => {
      disposed = true;
      resize.disconnect();
      themeWatch.disconnect();
      const socket = ws;
      ws = null;
      socket?.close(1000);
      t.dispose();
    };
  });

  let live = $derived(status.state === "open" || status.state === "connecting");
</script>

<div class={cn("rounded-lg overflow-hidden flex flex-col terminal", className)}>
  <div class="font-sans px-4 py-2 border-b border-white/10 flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
    <div class="flex items-center gap-3 min-w-0 text-sm">
      <SquareTerminal class="w-4 h-4 shrink-0 opacity-70" />
      <span class="font-mono text-[13px] truncate" title={label}>{label}</span>
      <span class="inline-flex items-center gap-2 shrink-0 text-[13px]" role="status">
        {#if status.state === "connecting"}
          <span class="light light-deploying"></span> <span class="opacity-80">Connecting…</span>
        {:else if status.state === "open"}
          <span class="light light-live"></span>
          <span class="text-[rgb(78_198_140)] font-medium">Live{status.shell ? `, ${status.shell}` : ""}</span>
        {:else if status.state === "exited"}
          <span class="light {status.code === 0 ? 'light-off' : 'light-failed'}"></span>
          <span class="opacity-80">Exited with code {status.code}</span>
        {:else if status.state === "closed"}
          <span class="light light-off"></span> <span class="opacity-80">Closed</span>
        {:else}
          <span class="light light-failed"></span> <span class="text-[rgb(240_114_122)]">Failed</span>
        {/if}
      </span>
    </div>
    <div class="flex items-center gap-1.5">
      <Button size="sm" variant="secondary" onclick={copySelection} disabled={!hasSelection}
        title={isMac ? "Copy the selection (⌘C)" : "Copy the selection (Ctrl+Shift+C)"}>
        <Copy /> Copy
      </Button>
      <Button size="sm" variant="secondary" onclick={paste} disabled={status.state !== "open"}
        title={isMac ? "Paste (⌘V)" : "Paste (Ctrl+Shift+V)"}>
        <ClipboardPaste /> Paste
      </Button>
      {#if !live}
        <Button size="sm" onclick={connect}>
          <RotateCcw /> {status.state === "failed" ? "Try again" : "New session"}
        </Button>
      {/if}
    </div>
  </div>
  {#if status.state === "failed"}
    <div class="font-sans px-4 py-2.5 text-sm text-[rgb(240_114_122)] border-b border-white/10">{status.message}</div>
  {:else if status.state === "closed"}
    <div class="font-sans px-4 py-2.5 text-sm opacity-80 border-b border-white/10">{status.reason}</div>
  {/if}
  <!-- Clicking anywhere in the frame gives the terminal the keyboard -->
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="flex-1 min-h-0 p-3 pr-1" onclick={() => term?.focus()}>
    <div bind:this={box} class="h-full w-full"></div>
  </div>
</div>

<style>
  /* xterm puts the class "terminal" on its root, which would draw the
     app's terminal panel (bezel and shadow) a second time inside ours */
  div :global(.xterm.terminal) {
    box-shadow: none;
    background: none;
  }
  /* xterm draws its own scrollbar area; keep it from showing a light
     browser scrollbar on the dark panel */
  div :global(.xterm-viewport) {
    background-color: transparent !important;
    scrollbar-color: rgb(var(--rivet)) transparent;
  }
</style>
