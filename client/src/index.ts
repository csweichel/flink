export type JsonPrimitive = string | number | boolean | null;
export type JsonValue = JsonPrimitive | JsonValue[] | { [key: string]: JsonValue };
export type JsonObject = { [key: string]: JsonValue };

export interface FlinkClientOptions {
  tenant?: string;
  site?: string;
  baseUrl?: string;
  fetch?: typeof fetch;
  WebSocket?: typeof WebSocket;
  credentials?: RequestCredentials;
  headers?: HeadersInit;
}

export interface UploadResult {
  url: string;
  name: string;
}

export interface UploadOptions {
  filename?: string;
  fieldName?: string;
}

export interface AIRequest {
  prompt: string;
  instructions?: string;
  model?: string;
  maxOutputTokens?: number;
}

export interface AIResponse {
  text: string;
  model?: string;
  configured: boolean;
}

export interface SiteFile {
  path: string;
  content: string;
}

export interface SiteFileInfo {
  path: string;
  size: number;
}

export type MessageHandler<T = unknown> = (message: T, event: MessageEvent) => void;

export interface RealtimeRoom<TSend = unknown, TReceive = unknown> {
  socket: WebSocket;
  send(value: TSend | string): void;
  onMessage(handler: MessageHandler<TReceive>): () => void;
  close(code?: number, reason?: string): void;
}

export class FlinkError extends Error {
  status: number;
  body: unknown;

  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.name = "FlinkError";
    this.status = status;
    this.body = body;
  }
}

export interface FlinkStorageAPI {
  get<T = JsonValue>(key: string): Promise<T>;
  all<T extends Record<string, unknown> = Record<string, JsonValue>>(): Promise<T>;
  set<T = JsonValue>(key: string, value: T): Promise<T>;
  delete(key: string): Promise<{ deleted: true }>;
  del(key: string): Promise<{ deleted: true }>;
}

export interface FlinkFilesAPI {
  list(prefix?: string): Promise<SiteFileInfo[]>;
  read(path?: string): Promise<SiteFile>;
  write(path: string, content: string | Blob | ArrayBuffer | Uint8Array): Promise<{ path: string }>;
  delete(path: string): Promise<{ deleted: true }>;
  del(path: string): Promise<{ deleted: true }>;
  url(path?: string): string;
}

export interface FlinkUploadsAPI {
  upload(file: Blob, options?: UploadOptions): Promise<UploadResult>;
  url(upload: UploadResult | string): string;
  fetch(upload: UploadResult | string, init?: RequestInit): Promise<Response>;
  text(upload: UploadResult | string): Promise<string>;
  json<T = JsonValue>(upload: UploadResult | string): Promise<T>;
  blob(upload: UploadResult | string): Promise<Blob>;
}

export interface FlinkRealtimeAPI {
  room<TSend = unknown, TReceive = unknown>(
    name?: string,
    onMessage?: MessageHandler<TReceive>,
  ): RealtimeRoom<TSend, TReceive>;
}

export interface FlinkClient {
  tenant?: string;
  site: string;
  storage: FlinkStorageAPI;
  files: FlinkFilesAPI;
  uploads: FlinkUploadsAPI;
  realtime: FlinkRealtimeAPI;
  get<T = JsonValue>(key: string): Promise<T>;
  all<T extends Record<string, unknown> = Record<string, JsonValue>>(): Promise<T>;
  set<T = JsonValue>(key: string, value: T): Promise<T>;
  delete(key: string): Promise<{ deleted: true }>;
  del(key: string): Promise<{ deleted: true }>;
  upload(file: Blob, options?: UploadOptions): Promise<UploadResult>;
  room<TSend = unknown, TReceive = unknown>(
    name?: string,
    onMessage?: MessageHandler<TReceive>,
  ): RealtimeRoom<TSend, TReceive>;
  ai(prompt: string | AIRequest): Promise<AIResponse>;
}

export function createFlinkClient(options: FlinkClientOptions = {}): FlinkClient {
  const inferred: { tenant?: string; site: string } = options.site ? { site: options.site } : inferTenantAndSite();
  const tenant = options.tenant ?? inferred.tenant;
  const site = options.site ?? inferred.site;
  const baseUrl = trimTrailingSlash(options.baseUrl ?? "");
  const fetchImpl = options.fetch ?? globalThis.fetch?.bind(globalThis);
  const WebSocketImpl = options.WebSocket ?? globalThis.WebSocket;
  const credentials = options.credentials ?? "same-origin";

  if (!fetchImpl) {
    throw new Error("Flink client requires fetch. Pass options.fetch in this runtime.");
  }

  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers);
    new Headers(init.headers).forEach((value, key) => headers.set(key, value));

    const res = await fetchImpl(apiUrl(path), {
      ...init,
      credentials,
      headers,
    });

    if (!res.ok) {
      throw await errorFromResponse(res);
    }

    if (res.status === 204) {
      return undefined as T;
    }
    return (await res.json()) as T;
  }

  function apiUrl(path: string): string {
    return `${baseUrl}/api/public/${encodeURIComponent(site)}${path}`;
  }

  const storage: FlinkStorageAPI = {
    get<T = JsonValue>(key: string) {
      return request<T>(`/data/${encodeURIComponent(key)}`);
    },
    all<T extends Record<string, unknown> = Record<string, JsonValue>>() {
      return request<T>("/data/");
    },
    set<T = JsonValue>(key: string, value: T) {
      return request<T>(`/data/${encodeURIComponent(key)}`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(value),
      });
    },
    delete(key: string) {
      return request<{ deleted: true }>(`/data/${encodeURIComponent(key)}`, { method: "DELETE" });
    },
    del(key: string) {
      return storage.delete(key);
    },
  };

  const files: FlinkFilesAPI = {
    list(prefix?: string) {
      const query = prefix ? `?prefix=${encodeURIComponent(prefix)}` : "";
      return request<SiteFileInfo[]>(`/files${query}`);
    },
    read(path = "index.html") {
      return request<SiteFile>(`/files?path=${encodeURIComponent(path)}`);
    },
    write(path: string, content: string | Blob | ArrayBuffer | Uint8Array) {
      return request<{ path: string }>(`/files?path=${encodeURIComponent(path)}`, {
        method: "PUT",
        body: content,
      });
    },
    delete(path: string) {
      return request<{ deleted: true }>(`/files?path=${encodeURIComponent(path)}`, { method: "DELETE" });
    },
    del(path: string) {
      return files.delete(path);
    },
    url(path = "index.html") {
      const clean = path.replace(/^\/+/, "");
      if (tenant) {
        return `${baseUrl}/t/${encodeURIComponent(tenant)}/s/${encodeURIComponent(site)}/${clean}`;
      }
      return `${baseUrl}/s/${encodeURIComponent(site)}/${clean}`;
    },
  };

  const uploads: FlinkUploadsAPI = {
    upload(file: Blob, uploadOptions: UploadOptions = {}) {
      const form = new FormData();
      const fieldName = uploadOptions.fieldName ?? "file";
      const filename = uploadOptions.filename ?? fileName(file);
      if (filename) {
        form.append(fieldName, file, filename);
      } else {
        form.append(fieldName, file);
      }
      return request<UploadResult>("/uploads", {
        method: "POST",
        body: form,
      });
    },
    url(upload: UploadResult | string) {
      const raw = typeof upload === "string" ? upload : upload.url;
      return absoluteUrl(baseUrl, raw);
    },
    fetch(upload: UploadResult | string, init?: RequestInit) {
      return fetchImpl(uploads.url(upload), { credentials, ...init });
    },
    async text(upload: UploadResult | string) {
      return (await uploads.fetch(upload)).text();
    },
    async json<T = JsonValue>(upload: UploadResult | string) {
      return (await uploads.fetch(upload)).json() as Promise<T>;
    },
    async blob(upload: UploadResult | string) {
      return (await uploads.fetch(upload)).blob();
    },
  };

  const realtime: FlinkRealtimeAPI = {
    room<TSend = unknown, TReceive = unknown>(
      name = "main",
      onMessage?: MessageHandler<TReceive>,
    ): RealtimeRoom<TSend, TReceive> {
      if (!WebSocketImpl) {
        throw new Error("Flink realtime requires WebSocket. Pass options.WebSocket in this runtime.");
      }
      const handlers = new Set<MessageHandler<TReceive>>();
      if (onMessage) {
        handlers.add(onMessage);
      }
      const socket = new WebSocketImpl(wsUrl(baseUrl, site, name));
      socket.addEventListener("message", (event) => {
        const value = parseMessage(event.data) as TReceive;
        handlers.forEach((handler) => handler(value, event));
      });
      return {
        socket,
        send(value: TSend | string) {
          socket.send(typeof value === "string" ? value : JSON.stringify(value));
        },
        onMessage(handler: MessageHandler<TReceive>) {
          handlers.add(handler);
          return () => handlers.delete(handler);
        },
        close(code?: number, reason?: string) {
          socket.close(code, reason);
        },
      };
    },
  };

  return {
    tenant,
    site,
    storage,
    files,
    uploads,
    realtime,
    get: storage.get,
    all: storage.all,
    set: storage.set,
    delete: storage.delete,
    del: storage.del,
    upload: uploads.upload,
    room: realtime.room,
    ai(prompt: string | AIRequest) {
      const body = typeof prompt === "string" ? { prompt } : prompt;
      return request<AIResponse>("/ai", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
      });
    },
  };
}

async function errorFromResponse(res: Response): Promise<FlinkError> {
  let body: unknown;
  try {
    body = await res.json();
  } catch {
    body = await res.text().catch(() => "");
  }
  const message =
    body && typeof body === "object" && "error" in body && typeof body.error === "string"
      ? body.error
      : res.statusText || `HTTP ${res.status}`;
  return new FlinkError(message, res.status, body);
}

function inferTenantAndSite(): { tenant?: string; site: string } {
  const location = globalThis.location;
  if (!location) {
    throw new Error("Flink client could not infer site outside a browser. Pass createFlinkClient({ site }).");
  }
  const tenantMatch = location.pathname.match(/^\/t\/([^/]+)\/s\/([^/]+)/);
  if (tenantMatch?.[1] && tenantMatch?.[2]) {
    return { tenant: decodeURIComponent(tenantMatch[1]), site: decodeURIComponent(tenantMatch[2]) };
  }
  const match = location.pathname.match(/^\/s\/([^/]+)/);
  if (match?.[1]) {
    return { site: decodeURIComponent(match[1]) };
  }
  const label = location.hostname.split(".")[0];
  const parts = label.split("--");
  if (parts.length === 2) {
    return { tenant: parts[0], site: parts[1] };
  }
  return { site: label };
}

function wsUrl(baseUrl: string, site: string, room: string): string {
  const encoded = `${encodeURIComponent(site)}/${encodeURIComponent(room || "main")}`;
  if (baseUrl) {
    const url = new URL(baseUrl, globalThis.location?.href);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    url.pathname = `/ws/${encoded}`;
    url.search = "";
    url.hash = "";
    return url.toString();
  }
  const location = globalThis.location;
  if (!location) {
    throw new Error("Flink realtime requires baseUrl outside a browser.");
  }
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${location.host}/ws/${encoded}`;
}

function absoluteUrl(baseUrl: string, raw: string): string {
  if (/^https?:\/\//i.test(raw)) {
    return raw;
  }
  if (!baseUrl) {
    return raw;
  }
  return `${baseUrl}${raw.startsWith("/") ? raw : `/${raw}`}`;
}

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, "");
}

function fileName(file: Blob): string | undefined {
  return "name" in file && typeof file.name === "string" ? file.name : undefined;
}

function parseMessage(value: unknown): unknown {
  if (typeof value !== "string") {
    return value;
  }
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}
