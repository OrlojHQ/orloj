import { useAppStore } from "../store";
import type { ListResponse } from "./types";

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
  const resp = await fetch(url, { headers: buildHeaders(token) });
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
    });
    return resp.ok;
  } catch {
    return false;
  }
}
