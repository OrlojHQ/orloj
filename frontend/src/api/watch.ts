import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "../store";

type WatchEvent = {
  type: string;
  object?: unknown;
};

function createEventSource(
  apiBase: string,
  path: string,
  namespace: string,
  token: string,
  onEvent: (evt: WatchEvent) => void,
  onError?: () => void,
): EventSource {
  const base = apiBase.replace(/\/$/, "");
  const url = new URL(`/v1/${path}`, base);
  url.searchParams.set("namespace", namespace);
  if (token.trim()) {
    url.searchParams.set("token", token.trim());
  }

  const es = new EventSource(url.toString());
  es.onmessage = (e) => {
    try {
      const data = JSON.parse(e.data) as WatchEvent;
      onEvent(data);
    } catch {
      // ignore parse errors
    }
  };
  es.onerror = () => {
    onError?.();
  };
  return es;
}

export function useWatchInvalidation() {
  const qc = useQueryClient();
  const apiBase = useAppStore((s) => s.apiBase);
  const namespace = useAppStore((s) => s.namespace);
  const token = useAppStore((s) => s.token);
  const connected = useAppStore((s) => s.connected);
  const sourcesRef = useRef<EventSource[]>([]);

  useEffect(() => {
    if (!connected) return;

    const paths = ["tasks/watch", "agents/watch", "task-schedules/watch", "task-webhooks/watch", "events/watch"];
    const sources = paths.map((path) =>
      createEventSource(apiBase, path, namespace, token, (evt) => {
        const eventType = (evt.type ?? "").toLowerCase();
        if (eventType === "modified" || eventType === "added" || eventType === "deleted") {
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
      }),
    );

    sourcesRef.current = sources;
    return () => {
      sources.forEach((s) => s.close());
    };
  }, [apiBase, namespace, token, connected, qc]);
}
