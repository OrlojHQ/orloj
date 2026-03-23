import { useAppStore } from "../store";
import type { ListResponse, MemoryEntriesResponse } from "./types";

const GET_HEAD_TIMEOUT_MS = 30_000;
const MUTATE_TIMEOUT_MS = 60_000;

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

function timeoutMsForMethod(method: string): number {
  const m = method.toUpperCase();
  return m === "GET" || m === "HEAD" ? GET_HEAD_TIMEOUT_MS : MUTATE_TIMEOUT_MS;
}

/** Fetch with deadline (aligned with server: ~30s GET/HEAD, ~60s mutating). */
async function fetchOrThrow(url: string, init: RequestInit = {}): Promise<Response> {
  const { token } = getConnection();
  const method = (init.method ?? "GET").toUpperCase();
  const timeoutMs = timeoutMsForMethod(method);
  const timeoutCtrl = new AbortController();
  const tid = setTimeout(() => timeoutCtrl.abort(), timeoutMs);
  const { signal: userSignal, ...rest } = init;
  if (userSignal) {
    if (userSignal.aborted) {
      clearTimeout(tid);
      throw new Error("Request aborted");
    }
    userSignal.addEventListener(
      "abort",
      () => {
        clearTimeout(tid);
        timeoutCtrl.abort();
      },
      { once: true },
    );
  }
  try {
    const resp = await fetch(url, {
      ...rest,
      signal: timeoutCtrl.signal,
      credentials: rest.credentials ?? "same-origin",
      headers: { ...buildHeaders(token), ...rest.headers },
    });
    return resp;
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      throw new Error("Request timed out or was cancelled");
    }
    throw err;
  } finally {
    clearTimeout(tid);
  }
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
  const resp = await fetchOrThrow(url, options);
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(
      `${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`,
    );
  }
  return (await resp.json()) as T;
}

export type ListOptions = {
  /** Omit namespace query so the server returns workers (and any other types) from all namespaces. */
  allNamespaces?: boolean;
  /** Server list page size (keyset pagination). */
  limit?: number;
  /** Cursor from the previous response's `continue` field. */
  after?: string;
  /**
   * Kubernetes-style label selector: comma-separated `key=value` pairs.
   * Server also accepts `labels` as an alias.
   */
  labelSelector?: string;
};

export async function list<T>(resourcePath: string, opts?: ListOptions): Promise<ListResponse<T>> {
  const { namespace, apiBase } = getConnection();
  const path = resourcePath.replace(/^\/+/, "");
  let url: string;
  if (opts?.allNamespaces) {
    const base = apiBase.replace(/\/$/, "");
    const u = new URL(`/v1/${path}`, base);
    if (opts?.limit != null) u.searchParams.set("limit", String(opts.limit));
    if (opts?.after) u.searchParams.set("after", opts.after);
    if (opts?.labelSelector?.trim()) u.searchParams.set("labelSelector", opts.labelSelector.trim());
    url = u.toString();
  } else {
    const qp: Record<string, string> = {};
    if (opts?.limit != null) qp.limit = String(opts.limit);
    if (opts?.after) qp.after = opts.after;
    if (opts?.labelSelector?.trim()) qp.labelSelector = opts.labelSelector.trim();
    url = buildUrl(path, namespace, qp);
  }
  return request<ListResponse<T>>(url);
}

/**
 * Follows `continue` cursors until all items are loaded. Uses cursor pagination on each request.
 */
export async function listAll<T>(
  resourcePath: string,
  opts?: Omit<ListOptions, "after" | "limit"> & { pageLimit?: number },
): Promise<T[]> {
  const pageLimit = opts?.pageLimit ?? 200;
  const all: T[] = [];
  let after: string | undefined;
  for (;;) {
    const page = await list<T>(resourcePath, { ...opts, limit: pageLimit, after });
    all.push(...(page.items ?? []));
    const c = page.continue?.trim();
    if (!c) break;
    after = c;
  }
  return all;
}

export type ScopedRequestOptions = {
  namespace?: string;
};

export async function get<T>(resourcePath: string, name: string, opts?: ScopedRequestOptions): Promise<T> {
  const ns = opts?.namespace ?? getConnection().namespace;
  const url = buildUrl(`${resourcePath}/${name}`, ns);
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
  opts?: ScopedRequestOptions,
): Promise<T> {
  const ns = opts?.namespace ?? getConnection().namespace;
  const url = buildUrl(`${resourcePath}/${name}`, ns);
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (resourceVersion) {
    headers["If-Match"] = resourceVersion;
  }
  return request<T>(url, { method: "PUT", headers, body: JSON.stringify(body) });
}

export async function del(resourcePath: string, name: string, opts?: ScopedRequestOptions): Promise<void> {
  const ns = opts?.namespace ?? getConnection().namespace;
  const url = buildUrl(`${resourcePath}/${name}`, ns);
  const resp = await fetchOrThrow(url, { method: "DELETE" });
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
  const { namespace } = getConnection();
  const url = buildUrl(`${resourcePath}/${name}/logs`, namespace);
  const resp = await fetchOrThrow(url);
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
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetchOrThrow(`${base}/v1/namespaces`);
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
    const { apiBase } = getConnection();
    const base = apiBase.replace(/\/$/, "");
    const resp = await fetchOrThrow(`${base}/healthz`);
    return resp.ok;
  } catch {
    return false;
  }
}

export interface AuthConfigResponse {
  mode: "off" | "native" | "sso" | string;
  setup_required: boolean;
  login_methods: string[];
}

/** True when the server uses built-in username/password + session auth (`native`). */
export function isNativeAuthMode(mode: string | undefined): boolean {
  return mode === "native";
}

export interface AuthMeResponse {
  authenticated: boolean;
  username?: string;
  method?: string;
}

export interface AuthChangePasswordResponse {
  status: string;
}

export async function getAuthConfig(): Promise<AuthConfigResponse> {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetchOrThrow(`${base}/v1/auth/config`);
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
  return (await resp.json()) as AuthConfigResponse;
}

export async function getAuthMe(): Promise<AuthMeResponse> {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetchOrThrow(`${base}/v1/auth/me`);
  if (!resp.ok) {
    return { authenticated: false };
  }
  return (await resp.json()) as AuthMeResponse;
}

export async function setupLocalAuth(username: string, password: string): Promise<AuthMeResponse> {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetchOrThrow(`${base}/v1/auth/setup`, {
    method: "POST",
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
  const resp = await fetchOrThrow(`${base}/v1/auth/login`, {
    method: "POST",
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
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetchOrThrow(`${base}/v1/auth/logout`, {
    method: "POST",
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
}

export async function changeLocalAuthPassword(
  currentPassword: string,
  newPassword: string,
): Promise<AuthChangePasswordResponse> {
  const { apiBase } = getConnection();
  const base = apiBase.replace(/\/$/, "");
  const resp = await fetchOrThrow(`${base}/v1/auth/change-password`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status} ${resp.statusText}${body ? `: ${body}` : ""}`);
  }
  return (await resp.json()) as AuthChangePasswordResponse;
}
