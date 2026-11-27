const API_BASE = "/api";
const API_KEY_STORAGE = "docktrine.apiKey";

// People sign in with a password (a session cookie). Signing in with an API
// key, kept in localStorage, remains for the first admin and for recovery.
export const auth = {
  getKey(): string | null {
    try {
      return localStorage.getItem(API_KEY_STORAGE);
    } catch {
      return null;
    }
  },

  setKey(key: string) {
    try {
      localStorage.setItem(API_KEY_STORAGE, key);
    } catch {
      // Storage unavailable; the key only lasts for this page load
    }
    memoryKey = key;
  },

  clearKey() {
    try {
      localStorage.removeItem(API_KEY_STORAGE);
    } catch {}
    memoryKey = null;
  },
};

let memoryKey: string | null = auth.getKey();

export interface Container {
  id: string;
  // Docker's name, with a leading slash
  name: string;
  image: string;
  created: string;
  status: string;
  state: {
    Status: string;
    Running: boolean;
    StartedAt: string;
    FinishedAt: string;
    ExitCode: number;
    OOMKilled?: boolean;
    Health?: { Status: string };
  };
  ports: Array<{
    PrivatePort: number;
    PublicPort?: number;
    Type: string;
    IP?: string;
  }> | null;
  labels: Record<string, string> | null;
  restarts?: number;
}

export interface Server {
  id?: number;
  name: string;
  host: string;
  description?: string;
  is_default?: boolean;
  // Where the gateway (NPM) forwards this node's traffic, e.g. the gateway
  // end of an SSH tunnel to the node's ingress
  ingress_host?: string;
  ingress_port?: number;
  created_at?: string;
  updated_at?: string;
}

export interface ServerHealth {
  reachable: boolean;
  error?: string;
  server_version?: string;
  os?: string;
  cpus?: number;
  memory_bytes?: number;
  containers?: number;
  containers_running?: number;
  images?: number;
  ingress?: string;
}

export type DeploymentStatus = "pending" | "running" | "succeeded" | "failed";

// A compose stack read from a public Git repository at deploy time
export interface GitSource {
  repo: string;
  ref?: string;
  // The compose file, relative to the repository root
  path?: string;
  // The commit a deployment ran, recorded by the server
  commit?: string;
}

export interface Spec {
  app: string;
  environment: string;
  node: string;
  image?: string;
  compose?: string;
  source?: GitSource;
  // Run on the node before a stack from Git starts, e.g. ./install.sh
  setup?: { command: string; always?: boolean; force?: boolean };
  // "ui" for apps launched from the web UI; the CLI leaves it out
  origin?: string;
  expose?: { service: string; port: number };
  hostnames?: string[];
  port?: number;
  env?: Record<string, string>;
  secrets?: string[];
  volumes?: string[];
  command?: string[];
  healthcheck?: string;
  preview?: boolean;
  branch?: string;
  ttl?: string;
}

export interface Environment {
  id: number;
  app: string;
  name: string;
  server: string;
  is_preview: boolean;
  branch?: string;
  expires_at?: string;
  current_deployment_id?: number;
  created_at: string;
  updated_at: string;
}

export interface Deployment {
  id: number;
  environment_id: number;
  app: string;
  environment: string;
  image: string;
  spec: Spec | null;
  status: DeploymentStatus;
  message?: string;
  log?: string;
  created_by?: string;
  created_at: string;
  finished_at?: string;
}

export interface AppSummary {
  environment: Environment;
  // The last dozen deployments, oldest first
  history: { id: number; status: DeploymentStatus; created_at: string }[];
  latest_deployment?: Deployment;
  current_deployment?: Deployment;
}

export interface ContainerStatus {
  id: string;
  name: string;
  service?: string;
  image: string;
  state: string;
  status: string;
  deployment: number;
}

export interface EnvironmentDetail {
  environment: Environment;
  deployments: Deployment[];
  hostnames: string[];
  containers?: ContainerStatus[];
  containers_error?: string;
}

export interface StackPort {
  target: number;
  // Published on the node, e.g. "9000" or "$SENTRY_BIND"
  published?: string;
}

export interface StackService {
  name: string;
  image?: string;
  build?: boolean;
  ports: StackPort[];
}

export interface StackVariable {
  name: string;
  // What applies when it isn't set
  default?: string;
  has_default?: boolean;
  // A starting value from .env.example
  suggested?: string;
  required?: boolean;
  secret?: boolean;
  description?: string;
}

export interface StackInfo {
  compose_file?: string;
  // Offered when the repository root has no compose file
  compose_files?: string[];
  commit?: string;
  services?: StackService[];
  variables?: StackVariable[];
  service?: string;
  port?: number;
  // A setup command suggested from the repository's install script
  setup?: string;
  // What it needs of the server, for the preflight checks
  requirements?: Requirements;
}

export interface Requirements {
  // The script the minimums come from, e.g. install/_min-requirements.sh
  source?: string;
  // The first whose condition holds applies
  variants?: { when?: { variable: string; value: string; not?: boolean }; memory_mb?: number; cpus?: number }[];
  docker_version?: string;
  reserved_memory_mb?: number;
  reserved_cpus?: number;
}

export interface PreflightRequest {
  node: string;
  app: string;
  environment: string;
  hostnames?: string[];
  requirements?: Requirements;
  // Published ports as the compose file writes them
  ports?: { service: string; published: string }[];
  // Values variables will have; secrets are left out
  variables?: Record<string, string>;
}

export type CheckStatus = "ok" | "info" | "warning" | "failed";

export interface Check {
  id: string;
  status: CheckStatus;
  title: string;
  detail?: string;
}

export interface ImageInfo {
  image: string;
  ports: number[];
  volumes: string[];
  healthcheck?: boolean;
  port?: number;
}

// A random value for a secret variable: alnum and hex count characters,
// base64 counts random bytes, as the secret generator takes them
export interface GenerateFormat {
  kind: "alnum" | "hex" | "base64";
  length: number;
}

export interface PresetVariable {
  name: string;
  value?: string;
  description?: string;
  required?: boolean;
  secret?: boolean;
  generate?: GenerateFormat;
  // Filled from the first hostname, e.g. "https://{hostname}"
  from_hostname?: string;
}

// A known app from the catalog, which fills the launch form
export interface Preset {
  id: string;
  name: string;
  description: string;
  homepage: string;
  kind: "image" | "git" | "compose";
  version: string;
  image?: string;
  source?: { repo: string; ref: string; path?: string };
  compose?: string;
  expose?: { service: string; port: number };
  port?: number;
  volumes?: string[];
  healthcheck?: string;
  setup?: string;
  variables?: PresetVariable[];
  requirements?: Requirements;
  notes?: string[];
}

export interface SecretInfo {
  key: string;
  updated_at: string;
}

export interface ApiKeyInfo {
  id: number;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at?: string | null;
  // Keys without an owner act as admin
  user_id: number | null;
  owner: string;
  // The key this browser is signed in with
  current: boolean;
}

export type Role = "viewer" | "deployer" | "admin";

export interface User {
  id: number;
  username: string;
  display_name: string;
  role: Role;
  created_at: string;
  last_seen_at?: string | null;
  // Hasn't chosen a password yet
  invited: boolean;
  invite_expires_at?: string;
}

export interface Me {
  // Null when signed in with a key that has no owner
  user: User | null;
  role: Role;
  via: "session" | "key";
  key_name?: string;
}

export interface CreateContainerRequest {
  image: string;
  name?: string;
  env?: string[];
  ports?: Record<
    string,
    Array<{
      HostIp: string;
      HostPort: string;
    }>
  >;
  volumes?: string[];
  command?: string[];
  server?: string;
}

class ApiError extends Error {
  constructor(
    public status: number,
    message: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...headers(),
      ...options.headers,
    },
  });

  if (response.status === 401 && !endpoint.startsWith("/auth/")) {
    auth.clearKey();
    window.dispatchEvent(new Event("docktrine:unauthorized"));
  }

  if (!response.ok) {
    const error = await response
      .json()
      .catch(() => ({ error: response.statusText }));
    throw new ApiError(response.status, error.error || response.statusText);
  }

  if (response.status === 204) return undefined as T;
  return response.json();
}

function headers(): Record<string, string> {
  // The API refuses cookie-authenticated writes without this header (CSRF)
  const h: Record<string, string> = { "X-Docktrine-Client": "web" };
  if (memoryKey) h["X-API-Key"] = memoryKey;
  return h;
}

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

export const api = {
  auth: {
    status: (): Promise<{ has_users: boolean }> => request("/auth/status"),
    login: (username: string, password: string): Promise<User> =>
      request("/auth/login", json("POST", { username, password })),
    logout: (): Promise<void> => request("/auth/logout", json("POST")),
    invite: (token: string): Promise<User> => request(`/auth/invite/${enc(token)}`),
    acceptInvite: (token: string, password: string): Promise<User> =>
      request(`/auth/invite/${enc(token)}`, json("POST", { password })),
  },

  me: {
    get: (): Promise<Me> => request("/me"),
    update: (display_name: string): Promise<User> => request("/me", json("PATCH", { display_name })),
    changePassword: (current: string, next: string): Promise<void> =>
      request("/me/password", json("PUT", { current, new: next })),
  },

  users: {
    list: (): Promise<User[]> => request("/users"),
    // The token goes in the invite link: /#/invite/<token>
    create: (u: { username: string; display_name: string; role: Role }): Promise<User & { invite_token: string }> =>
      request("/users", json("POST", u)),
    update: (id: number, u: { display_name: string; role: Role }): Promise<User> =>
      request(`/users/${id}`, json("PATCH", u)),
    reset: (id: number): Promise<User & { invite_token: string }> => request(`/users/${id}/reset`, json("POST")),
    remove: (id: number): Promise<void> => request(`/users/${id}`, json("DELETE")),
  },

  // Container operations
  containers: {
    list: (server?: string): Promise<Container[]> => {
      const params = server ? `?server=${encodeURIComponent(server)}` : "";
      return request(`/containers${params}`);
    },

    get: (id: string, server?: string): Promise<Container> => {
      const params = server ? `?server=${encodeURIComponent(server)}` : "";
      return request(`/containers/${id}${params}`);
    },

    start: (id: string, server?: string): Promise<{ message: string }> => {
      const params = server ? `?server=${encodeURIComponent(server)}` : "";
      return request(`/containers/start/${id}${params}`, { method: "POST" });
    },

    stop: (id: string, server?: string): Promise<{ message: string }> => {
      const params = server ? `?server=${encodeURIComponent(server)}` : "";
      return request(`/containers/stop/${id}${params}`, { method: "POST" });
    },

    restart: (
      id: string,
      server?: string,
      pullLatest?: boolean
    ): Promise<{ message: string }> => {
      const params = new URLSearchParams();
      if (server) params.set("server", server);
      if (pullLatest) params.set("pull_latest", "true");
      const queryString = params.toString() ? `?${params.toString()}` : "";
      return request(`/containers/restart/${id}${queryString}`, {
        method: "POST",
      });
    },

    create: (
      container: CreateContainerRequest
    ): Promise<{ message: string }> => {
      return request("/containers/create", {
        method: "POST",
        body: JSON.stringify(container),
      });
    },
  },

  // Server operations
  servers: {
    list: (): Promise<Server[]> => request("/servers"),

    get: (name: string): Promise<Server> => request(`/servers/${name}`),

    create: (
      server: Omit<Server, "id" | "created_at" | "updated_at">
    ): Promise<Server> => {
      return request("/servers", {
        method: "POST",
        body: JSON.stringify(server),
      });
    },

    update: (name: string, server: Partial<Server>): Promise<Server> => {
      return request(`/servers/${enc(name)}`, {
        method: "PUT",
        body: JSON.stringify(server),
      });
    },

    delete: (name: string): Promise<{ message: string }> => {
      return request(`/servers/${name}`, { method: "DELETE" });
    },

    health: (name: string): Promise<ServerHealth> =>
      request(`/servers/${enc(name)}/health`),

    installIngress: (
      name: string,
      opts: { port: number; bind?: string }
    ): Promise<{ message: string }> =>
      request(`/servers/${enc(name)}/ingress`, {
        method: "POST",
        body: JSON.stringify(opts),
      }),

    prune: (
      name: string,
      all: boolean
    ): Promise<{ images_deleted: number; space_reclaimed: number }> =>
      request(`/servers/${enc(name)}/prune${all ? "?all=true" : ""}`, {
        method: "POST",
      }),
  },

  apps: {
    list: (): Promise<AppSummary[]> => request("/apps"),

    get: (app: string, env: string): Promise<EnvironmentDetail> =>
      request(`/apps/${enc(app)}/${enc(env)}`),

    rollback: (
      app: string,
      env: string,
      deploymentId?: number
    ): Promise<Deployment> =>
      request(`/apps/${enc(app)}/${enc(env)}/rollback`, {
        method: "POST",
        body: deploymentId
          ? JSON.stringify({ deployment_id: deploymentId })
          : undefined,
      }),

    destroy: (
      app: string,
      env: string,
      purge: boolean
    ): Promise<{ message: string }> =>
      request(`/apps/${enc(app)}/${enc(env)}${purge ? "?purge=true" : ""}`, {
        method: "DELETE",
      }),

    secrets: {
      list: (app: string, env: string): Promise<SecretInfo[]> =>
        request(`/apps/${enc(app)}/${enc(env)}/secrets`),

      set: (
        app: string,
        env: string,
        values: Record<string, string>
      ): Promise<{ message: string }> =>
        request(`/apps/${enc(app)}/${enc(env)}/secrets`, {
          method: "PUT",
          body: JSON.stringify({ values }),
        }),

      delete: (
        app: string,
        env: string,
        key: string
      ): Promise<{ message: string }> =>
        request(`/apps/${enc(app)}/${enc(env)}/secrets/${enc(key)}`, {
          method: "DELETE",
        }),
    },

    // Streams plain-text logs, calling onChunk as text arrives. With follow,
    // it keeps streaming until the signal aborts.
    logs: async (
      app: string,
      env: string,
      opts: { tail?: number; follow?: boolean; signal?: AbortSignal },
      onChunk: (text: string) => void
    ): Promise<void> => {
      const params = new URLSearchParams({ tail: String(opts.tail ?? 200) });
      if (opts.follow) params.set("follow", "true");
      const response = await fetch(
        `${API_BASE}/apps/${enc(app)}/${enc(env)}/logs?${params}`,
        { headers: headers(), signal: opts.signal }
      );
      if (!response.ok || !response.body) {
        throw new ApiError(response.status, response.statusText);
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        onChunk(decoder.decode(value, { stream: true }));
      }
    },
  },

  config: (): Promise<{ registry: string; npm_configured: boolean }> => request("/config"),

  // Reading what to launch, to fill in the form
  launch: {
    inspectStack: (body: { source: GitSource } | { compose: string }): Promise<StackInfo> =>
      request("/stacks/inspect", json("POST", body)),
    inspectImage: (image: string): Promise<ImageInfo> =>
      request(`/images/inspect?image=${enc(image)}`),
    // Whether the server can run it: resources, ports, ingress
    preflight: (body: PreflightRequest, signal?: AbortSignal): Promise<{ checks: Check[] }> =>
      request("/launch/preflight", { ...json("POST", body), signal }),
    // Known apps, to start the form from
    catalog: (): Promise<Preset[]> => request("/catalog"),
  },

  keys: {
    list: (): Promise<ApiKeyInfo[]> => request("/keys"),

    // The response is the only time the full key is available
    create: (name: string): Promise<ApiKeyInfo & { key: string }> =>
      request("/keys", { method: "POST", body: JSON.stringify({ name }) }),

    rename: (id: number, name: string): Promise<{ message: string }> =>
      request(`/keys/${id}`, { method: "PATCH", body: JSON.stringify({ name }) }),

    revoke: (id: number): Promise<{ message: string }> =>
      request(`/keys/${id}`, { method: "DELETE" }),
  },

  deployments: {
    list: (limit = 100): Promise<Deployment[]> => request(`/deployments?limit=${limit}`),

    get: (id: number): Promise<Deployment> => request(`/deployments/${id}`),

    // Redeploys a stored spec as-is: same image, no build
    create: (spec: Spec): Promise<Deployment> =>
      request("/deployments", { method: "POST", body: JSON.stringify(spec) }),
  },
};

const enc = encodeURIComponent;

export { ApiError };
