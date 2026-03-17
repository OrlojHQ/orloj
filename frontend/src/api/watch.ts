import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "../store";

type WatchEvent = {
  type: string;
  object?: unknown;
};

const INITIAL_BACKOFF = 1000;
const MAX_BACKOFF = 30000;

function createReconnectingSource(
  apiBase: string,
  path: string,
  namespace: string,
  token: string,
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
    if (token.trim()) {
      url.searchParams.set("token", token.trim());
    }

    const es = new EventSource(url.toString());

    es.onopen = () => {
      backoff = INITIAL_BACKOFF;
    };

    es.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data) as WatchEvent;
        onEvent(data);
      } catch {
        // ignore parse errors
      }
    };

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

export function useWatchInvalidation() {
  const qc = useQueryClient();
  const apiBase = useAppStore((s) => s.apiBase);
  const namespace = useAppStore((s) => s.namespace);
  const token = useAppStore((s) => s.token);
  const connected = useAppStore((s) => s.connected);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!connected) return;

    const abort = new AbortController();
    abortRef.current = abort;

    const paths = ["tasks/watch", "agents/watch", "task-schedules/watch", "task-webhooks/watch", "events/watch"];

    for (const path of paths) {
      createReconnectingSource(apiBase, path, namespace, token, (evt) => {
        const eventType = (evt.type ?? "").toLowerCase();
        if (eventType === "modified" || eventType === "updated" || eventType === "added" || eventType === "deleted") {
          if (path.startsWith("tasks")) {
            qc.invalidateQueries({ queryKey: ["Task"] });
          } else if (path.startsWith("agents")) {
            qc.invalidateQueries({ queryKey: ["Agent"] });
          } else if (path.startsWith("task-schedules")) {
            qc.invalidateQueries({ queryKey: ["TaskSchedule"] });
          } else if (path.startsWith("task-webhooks")) {
            qc.invalidateQueries({ queryKey: ["TaskWebhook"] });
          } else {
            qc.invalidateQueries();
          }
        }
      }, abort.signal);
    }

    return () => {
      abort.abort();
    };
  }, [apiBase, namespace, token, connected, qc]);
}
