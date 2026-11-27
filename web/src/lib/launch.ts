// Helpers for the launch form: naming, parsing what people paste, and
// guessing sensible defaults.

const namePattern = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;

export function isName(s: string): boolean {
  return namePattern.test(s);
}

// Lowercase letters, digits and dashes, as app and environment names need
export function slugify(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/, "");
}

// postgres:16 → postgres, ghcr.io/umami-software/umami:latest → umami
export function nameFromImage(image: string): string {
  const path = image.trim().split("@")[0];
  const last = path.slice(path.lastIndexOf("/") + 1).split(":")[0];
  return slugify(last);
}

// Repository names that say nothing about what they run
const generic = new Set(["self-hosted", "selfhosted", "docker", "compose", "docker-compose", "deploy", "deployment", "infra", "stack"]);

// https://github.com/getsentry/self-hosted → sentry
export function nameFromRepo(repo: string): string {
  const parts = repo.replace(/\.git$/, "").split("/").filter(Boolean);
  const name = parts[parts.length - 1] ?? "";
  const owner = parts[parts.length - 2] ?? "";
  if (generic.has(name.toLowerCase()) && owner) {
    return slugify(owner.replace(/^get(?=[a-z])/i, ""));
  }
  return slugify(name.replace(/^docker-|-docker$/i, ""));
}

export interface RepoLink {
  repo: string;
  ref?: string;
  path?: string;
}

// Accepts what people copy from a browser: github.com/a/b,
// https://github.com/a/b/tree/v2, https://github.com/a/b/blob/main/deploy/compose.yml
export function parseRepo(input: string): RepoLink | null {
  let s = input.trim();
  if (!s) return null;
  if (!/^[a-z]+:\/\//i.test(s)) s = "https://" + s;
  let url: URL;
  try {
    url = new URL(s);
  } catch {
    return null;
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") return null;
  url.protocol = "https:";

  const parts = url.pathname.split("/").filter(Boolean);
  const hosted = ["github.com", "gitlab.com", "codeberg.org"].includes(url.hostname);
  // On GitLab the browse path has a "-" segment: /a/b/-/tree/main
  const dash = parts.indexOf("-");
  const marker = parts.findIndex((p, i) => (p === "tree" || p === "blob") && i >= 2 && (!hosted || dash < 0 || i === dash + 1));
  if (hosted && marker > 0) {
    const repoParts = parts.slice(0, dash >= 0 && dash < marker ? dash : marker);
    const ref = parts[marker + 1];
    const rest = parts.slice(marker + 2).join("/");
    const link: RepoLink = { repo: `https://${url.hostname}/${repoParts.join("/")}` };
    if (ref) link.ref = decodeURIComponent(ref);
    if (rest && parts[marker] === "blob") link.path = decodeURIComponent(rest);
    return link;
  }
  if (parts.length < 2) return null;
  return { repo: `https://${url.hostname}/${parts.join("/").replace(/\/$/, "")}` };
}

// Splits a command line the way a shell would, for quotes and spaces
export function splitCommand(s: string): string[] {
  const out: string[] = [];
  let cur = "";
  let quote: string | null = null;
  let started = false;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (quote) {
      if (c === quote) quote = null;
      else if (c === "\\" && quote === '"' && i + 1 < s.length) cur += s[++i];
      else cur += c;
    } else if (c === "'" || c === '"') {
      quote = c;
      started = true;
    } else if (c === "\\" && i + 1 < s.length) {
      cur += s[++i];
      started = true;
    } else if (/\s/.test(c)) {
      if (started) out.push(cur);
      cur = "";
      started = false;
    } else {
      cur += c;
      started = true;
    }
  }
  if (started) out.push(cur);
  return out;
}

export function joinCommand(args: string[]): string {
  return args.map((a) => (a === "" || /[\s'"\\$`]/.test(a) ? `'${a.replace(/'/g, `'\\''`)}'` : a)).join(" ");
}

const keyPattern = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function isEnvKey(s: string): boolean {
  return keyPattern.test(s);
}

// Reads pasted KEY=value lines, as in a .env file
export function parseDotenv(text: string): { key: string; value: string }[] {
  const out: { key: string; value: string }[] = [];
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    const eq = line.indexOf("=");
    if (eq <= 0) continue;
    const key = line.slice(0, eq).replace(/^export\s+/, "").trim();
    if (!keyPattern.test(key)) continue;
    let value = line.slice(eq + 1).trim();
    const q = value[0];
    if ((q === '"' || q === "'") && value.lastIndexOf(q) > 0) {
      value = value.slice(1, value.lastIndexOf(q));
    } else {
      const hash = value.indexOf(" #");
      if (hash >= 0) value = value.slice(0, hash).trim();
    }
    out.push({ key, value });
  }
  return out;
}

// Names that hold credentials; they default to being stored as secrets
export function looksSecret(key: string): boolean {
  return /(PASSWORD|PASSWD|PASS$|SECRET|TOKEN|PRIVATE|(^|_)KEY$|API_?KEY|CREDENTIAL|DSN$|SALT)/i.test(key);
}

const hostnamePattern = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

export function isHostname(s: string): boolean {
  return hostnamePattern.test(s);
}

// The domain most existing hostnames sit under, e.g. zeptile.software for
// a.zeptile.software and b.dev.zeptile.software; dev domains lose to the
// shorter one only when it is as common
export function commonDomain(hostnames: string[]): string | null {
  const counts = new Map<string, number>();
  for (const host of hostnames) {
    const labels = host.split(".");
    if (labels.length < 3) continue;
    const parent = labels.slice(1).join(".");
    counts.set(parent, (counts.get(parent) ?? 0) + 1);
  }
  let best: string | null = null;
  let bestCount = 0;
  for (const [domain, count] of counts) {
    if (count > bestCount || (count === bestCount && best && domain.length < best.length)) {
      best = domain;
      bestCount = count;
    }
  }
  return best;
}

export interface VarRow {
  id: number;
  key: string;
  value: string;
  // Stored encrypted as a secret rather than in the deployment's spec
  secret: boolean;
  // Read from the stack, so the name is fixed and a default may apply
  stack?: import("./api").StackVariable;
  // Already stored on the server; left blank, it keeps its value
  stored?: boolean;
}

let nextRow = 1;

export function newRow(fields: Partial<VarRow> = {}): VarRow {
  return { id: nextRow++, key: "", value: "", secret: false, ...fields };
}

// Whether a stack variable needs someone to look at it: compose refuses to
// start without it, or nothing gives it a value
export function needsValue(row: VarRow): boolean {
  if (!row.stack) return true;
  return !!row.stack.required || (!row.stack.has_default && !row.stack.suggested);
}
