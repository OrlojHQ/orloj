import { useAppStore } from "../store";
import type { ListResponse, MemoryEntriesResponse } from "./types";

function getConnection() {
  const { apiBase, namespace, token } = useAppStore.getState();
  return { apiBase, namespace, token };
}

function buildHeaders(token: string): HeadersInit {
  const headers: HeadersInit = { Accept: "application/json" };
  if (token.trim()) {
    headers.Authorization = `Bearer ${token.trim()}`;
  }
  return headers;
}

function buildUrl(
  path: string,
  namespace: string,
  params?: Record<string, string>,
): string {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const url = new URL(`/v1/${path}`, base);
  url.searchParams.set("namespace", namespace);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      url.searchParams.set(k, v);
    }
  }
  return url.toString();
}

async function request<T>(
  url: string,
  options: RequestInit = {},
): Promise<T> {
  const { token } = getConnection();
  const resp = await fetch(url, {
    ...options,
    credentials: "same-origin",
    headers: { ...buildHeaders(token), ...options.headers },
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(
      `${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`,
    );
  }
  return (await resp.json()) as T;
}

export async function list<T>(resourcePath: string): Promise<T[]> {
  const { namespace } = getConnection();
  const url = buildUrl(resourcePath, namespace);
  const data = await request<ListResponse<T>>(url);
  return data.items ?? [];
}

export async function get<T>(resourcePath: string, name: string): Promise<T> {
  const { namespace } = getConnection();
  const url = buildUrl(`${resourcePath}/${name}`, namespace);
  return request<T>(url);
}

export async function create<T>(resourcePath: string, body: unknown): Promise<T> {
  const { namespace } = getConnection();
  const url = buildUrl(resourcePath, namespace);
  return request<T>(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export async function update<T>(
  resourcePath: string,
  name: string,
  body: unknown,
  resourceVersion?: string,
): Promise<T> {
  const { namespace } = getConnection();
  const url = buildUrl(`${resourcePath}/${name}`, namespace);
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (resourceVersion) {
    headers["If-Match"] = resourceVersion;
  }
  return request<T>(url, { method: "PUT", headers, body: JSON.stringify(body) });
}

export async function del(resourcePath: string, name: string): Promise<void> {
  const { namespace } = getConnection();
  const url = buildUrl(`${resourcePath}/${name}`, namespace);
  const { token } = getConnection();
  const resp = await fetch(url, {
    method: "DELETE",
    credentials: "same-origin",
    headers: buildHeaders(token),
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
}

export async function postAction<T>(resourcePath: string, name: string, action: string): Promise<T> {
  const { namespace } = getConnection();
  const url = buildUrl(`${resourcePath}/${name}/${action}`, namespace);
  return request<T>(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
}

export async function getStatus<T>(resourcePath: string, name: string): Promise<T> {
  const { namespace } = getConnection();
  const url = buildUrl(`${resourcePath}/${name}/status`, namespace);
  return request<T>(url);
}

export async function getLogs(resourcePath: string, name: string): Promise<string> {
  const { namespace, token } = getConnection();
  const url = buildUrl(`${resourcePath}/${name}/logs`, namespace);
  const resp = await fetch(url, { headers: buildHeaders(token), credentials: "same-origin" });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
  return resp.text();
}

interface MessagesResponse<T> {
  name: string;
  namespace: string;
  total: number;
  filtered_from: number;
  lifecycle_counts: Record<string, number>;
  messages: T[];
}

export async function getMessages<T>(
  name: string,
  filters?: Record<string, string>,
): Promise<T[]> {
  const { namespace } = getConnection();
  const url = buildUrl(`tasks/${name}/messages`, namespace, filters);
  const data = await request<MessagesResponse<T>>(url);
  return data.messages ?? [];
}

export async function getMetrics<T>(name: string): Promise<T> {
  const { namespace } = getConnection();
  const url = buildUrl(`tasks/${name}/metrics`, namespace);
  return request<T>(url);
}

export async function listMemoryEntries(
  name: string,
  params?: { prefix?: string; q?: string; limit?: number },
): Promise<MemoryEntriesResponse> {
  const { namespace } = getConnection();
  const qp: Record<string, string> = {};
  if (params?.prefix) qp.prefix = params.prefix;
  if (params?.q) qp.q = params.q;
  if (params?.limit) qp.limit = String(params.limit);
  const url = buildUrl(`memories/${name}/entries`, namespace, qp);
  return request<MemoryEntriesResponse>(url);
}

export async function listNamespaces(): Promise<string[]> {
  const { apiBase, token } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetch(`${base}/v1/namespaces`, {
    headers: buildHeaders(token),
    credentials: "same-origin",
  });
  if (!resp.ok) return ["default"];
  const data = (await resp.json()) as { namespaces: string[] };
  return data.namespaces ?? ["default"];
}

export async function getCapabilities<T>(): Promise<T> {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  return request<T>(`${base}/v1/capabilities`);
}

export async function healthCheck(): Promise<boolean> {
  try {
    const { apiBase, token } = getConnection();
    const base = apiBase.replace(/\/$/, "");
    const resp = await fetch(`${base}/healthz`, {
      headers: buildHeaders(token),
      credentials: "same-origin",
    });
    return resp.ok;
  } catch {
    return false;
  }
}

export interface AuthConfigResponse {
  mode: "off" | "local" | "sso" | string;
  setup_required: boolean;
  login_methods: string[];
}

export interface AuthMeResponse {
  authenticated: boolean;
  username?: string;
  method?: string;
}

export async function getAuthConfig(): Promise<AuthConfigResponse> {
  const { apiBase, token } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetch(`${base}/v1/auth/config`, {
    headers: buildHeaders(token),
    credentials: "same-origin",
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
  return (await resp.json()) as AuthConfigResponse;
}

export async function getAuthMe(): Promise<AuthMeResponse> {
  const { apiBase, token } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetch(`${base}/v1/auth/me`, {
    headers: buildHeaders(token),
    credentials: "same-origin",
  });
  if (!resp.ok) {
    return { authenticated: false };
  }
  return (await resp.json()) as AuthMeResponse;
}

export async function setupLocalAuth(username: string, password: string): Promise<AuthMeResponse> {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetch(`${base}/v1/auth/setup`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
  return (await resp.json()) as AuthMeResponse;
}

export async function loginLocalAuth(username: string, password: string): Promise<AuthMeResponse> {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetch(`${base}/v1/auth/login`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
  return (await resp.json()) as AuthMeResponse;
}

export async function logoutLocalAuth(): Promise<void> {
  const { apiBase, token } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetch(`${base}/v1/auth/logout`, {
    method: "POST",
    credentials: "same-origin",
    headers: buildHeaders(token),
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
}
