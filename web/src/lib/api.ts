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
  port?: number;
  tls?: boolean;
  cert_path?: string;
  key_path?: string;
  ca_path?: string;
  created_at?: string;
  updated_at?: string;
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

    delete: (name: string): Promise<{ message: string }> => {
      return request(`/servers/${name}`, { method: "DELETE" });
    },
  },
};

export { ApiError };
