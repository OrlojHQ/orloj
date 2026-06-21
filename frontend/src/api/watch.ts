import { useEffect, useRef } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useAppStore } from "../store";

type WatchResource = {
  metadata?: {
    name?: string;
    namespace?: string;
  };
};

type WatchEvent = {
  type: string;
  resource?: WatchResource;
  object?: unknown;
};

const INITIAL_BACKOFF = 1000;
const MAX_BACKOFF = 30000;

function createReconnectingSource(
  apiBase: string,
  path: string,
  namespace: string,
  onEvent: (evt: WatchEvent) => void,
  abortSignal: AbortSignal,
): void {
  let backoff = INITIAL_BACKOFF;
  let timeoutId: ReturnType<typeof setTimeout> | null = null;

  function connect() {
    if (abortSignal.aborted) return;

    const base = apiBase.replace(/\/$/, "");
    const url = new URL(`/v1/${path}`, base);
    url.searchParams.set("namespace", namespace);

    const es = new EventSource(url.toString());

    es.onopen = () => {
      backoff = INITIAL_BACKOFF;
    };

    const handleSSE = (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data) as WatchEvent;
        onEvent(data);
      } catch {
        // ignore parse errors
      }
    };

    es.addEventListener("resource", handleSSE);
    es.addEventListener("event", handleSSE);
    es.onmessage = handleSSE;

    es.onerror = () => {
      es.close();
      if (abortSignal.aborted) return;
      timeoutId = setTimeout(connect, backoff);
      backoff = Math.min(backoff * 2, MAX_BACKOFF);
    };

    abortSignal.addEventListener("abort", () => {
      es.close();
      if (timeoutId != null) clearTimeout(timeoutId);
    }, { once: true });
  }

  connect();
}

function watchedResourceKind(path: string): string | null {
  if (path.startsWith("tasks")) return "Task";
  if (path.startsWith("agents")) return "Agent";
  if (path.startsWith("task-schedules")) return "TaskSchedule";
  if (path.startsWith("task-webhooks")) return "TaskWebhook";
  return null;
}

function isResourceMutation(eventType: string): boolean {
  return eventType === "modified" || eventType === "updated" || eventType === "added" || eventType === "deleted";
}

function eventResource(evt: WatchEvent): WatchResource | undefined {
  const candidate = evt.resource ?? evt.object;
  if (!candidate || typeof candidate !== "object") return undefined;
  return candidate as WatchResource;
}

function invalidateWatchedResource(qc: QueryClient, kind: string, namespace: string, evt: WatchEvent): void {
  const eventType = (evt.type ?? "").toLowerCase();
  if (!isResourceMutation(eventType)) return;

  const meta = eventResource(evt)?.metadata;
  const resourceNamespace = meta?.namespace?.trim() || namespace;
  const resourceName = meta?.name?.trim();

  if ((eventType === "modified" || eventType === "updated") && resourceName) {
    void qc.invalidateQueries({ queryKey: [kind, resourceNamespace, resourceName], exact: true });
    return;
  }

  void qc.invalidateQueries({ queryKey: [kind, resourceNamespace] });
}

export function useWatchInvalidation() {
  const qc = useQueryClient();
  const apiBase = useAppStore((s) => s.apiBase);
  const namespace = useAppStore((s) => s.namespace);
  const connected = useAppStore((s) => s.connected);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!connected) return;

    const abort = new AbortController();
    abortRef.current = abort;

    const paths = ["tasks/watch", "agents/watch", "task-schedules/watch", "task-webhooks/watch", "events/watch"];

    for (const path of paths) {
      createReconnectingSource(apiBase, path, namespace, (evt) => {
        const kind = watchedResourceKind(path);
        if (!kind) return;
        invalidateWatchedResource(qc, kind, namespace, evt);
      }, abort.signal);
    }

    return () => {
      abort.abort();
    };
  }, [apiBase, namespace, connected, qc]);
}
