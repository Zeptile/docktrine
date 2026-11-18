const API_BASE = "/api";
const API_KEY_STORAGE = "docktrine.apiKey";

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
  Id: string;
  Names: string[];
  Image: string;
  Command: string;
  Created: number;
  Status: string;
  State: string;
  Ports: Array<{
    PrivatePort: number;
    PublicPort?: number;
    Type: string;
    IP?: string;
  }>;
  Labels: Record<string, string>;
  SizeRw?: number;
  SizeRootFs?: number;
  HostConfig: {
    NetworkMode: string;
  };
  NetworkSettings: {
    Networks: Record<string, any>;
  };
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

export interface Spec {
  app: string;
  environment: string;
  node: string;
  image?: string;
  compose?: string;
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

export interface SecretInfo {
  key: string;
  updated_at: string;
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
      ...(memoryKey ? { "X-API-Key": memoryKey } : {}),
      ...options.headers,
    },
  });

  if (response.status === 401) {
    auth.clearKey();
    window.dispatchEvent(new Event("docktrine:unauthorized"));
  }

  if (!response.ok) {
    const error = await response
      .json()
      .catch(() => ({ error: response.statusText }));
    throw new ApiError(response.status, error.error || response.statusText);
  }

  return response.json();
}

export const api = {
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
        {
          headers: memoryKey ? { "X-API-Key": memoryKey } : {},
          signal: opts.signal,
        }
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

  deployments: {
    get: (id: number): Promise<Deployment> => request(`/deployments/${id}`),

    // Redeploys a stored spec as-is: same image, no build
    create: (spec: Spec): Promise<Deployment> =>
      request("/deployments", { method: "POST", body: JSON.stringify(spec) }),
  },
};

const enc = encodeURIComponent;

export { ApiError };
