import { getSessionStreamInit, getSessionStreamUrl } from "./client";
import type { SessionEvent } from "./types";

export type SessionStreamState = "connecting" | "open" | "reconnecting" | "closed";

interface SessionStreamOptions {
  afterSeq?: number;
  onEvent: (event: SessionEvent) => void;
  onStateChange?: (state: SessionStreamState) => void;
}

const INITIAL_RECONNECT_MS = 1000;
const MAX_RECONNECT_MS = 15_000;

export function isPermanentSessionStreamStatus(status: number): boolean {
  return status >= 400 && status < 500 && status !== 408 && status !== 429;
}

export function mergeSessionEvents(
  current: SessionEvent[],
  incoming: SessionEvent[],
): SessionEvent[] {
  const bySeq = new Map(current.map((event) => [event.seq, event]));
  for (const event of incoming) {
    bySeq.set(event.seq, event);
  }
  return [...bySeq.values()].sort((a, b) => a.seq - b.seq);
}

export function subscribeToSession(
  name: string,
  options: SessionStreamOptions,
): () => void {
  let controller: AbortController | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let stopped = false;
  let terminal = false;
  let lastSeq = options.afterSeq ?? 0;
  let reconnectDelay = INITIAL_RECONNECT_MS;

  const handleData = (data: string, eventID: string) => {
    try {
      const parsed = JSON.parse(data) as SessionEvent;
      const eventSeq = Number(parsed.seq || eventID);
      if (!Number.isFinite(eventSeq) || eventSeq <= lastSeq) return;
      lastSeq = eventSeq;
      options.onEvent({ ...parsed, seq: eventSeq });
      terminal =
        parsed.type === "session.cancelled" ||
        parsed.type === "session.completed" ||
        parsed.type === "session.expired" ||
        parsed.type === "turn.failed";
    } catch {
      // Ignore malformed application events without dropping the stream.
    }
  };

  const consume = async (response: Response) => {
    if (!response.body) throw new Error("Session stream has no response body");
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let eventID = "";
    let dataLines: string[] = [];

    const dispatch = () => {
      if (dataLines.length > 0) handleData(dataLines.join("\n"), eventID);
      eventID = "";
      dataLines = [];
    };
    const processLine = (line: string) => {
      if (line === "") {
        dispatch();
        return;
      }
      if (line.startsWith(":")) return;
      const colon = line.indexOf(":");
      const field = colon >= 0 ? line.slice(0, colon) : line;
      let value = colon >= 0 ? line.slice(colon + 1) : "";
      if (value.startsWith(" ")) value = value.slice(1);
      if (field === "id") eventID = value;
      if (field === "data") dataLines.push(value);
    };

    for (;;) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value, { stream: !done }).replace(/\r\n/g, "\n");
      let newline = buffer.indexOf("\n");
      while (newline >= 0) {
        processLine(buffer.slice(0, newline));
        buffer = buffer.slice(newline + 1);
        newline = buffer.indexOf("\n");
      }
      if (done) {
        if (buffer) processLine(buffer.replace(/\r$/, ""));
        dispatch();
        return;
      }
    }
  };

  const connect = async () => {
    if (stopped) return;
    options.onStateChange?.(lastSeq > 0 ? "reconnecting" : "connecting");
    controller = new AbortController();
    try {
      const response = await fetch(
        getSessionStreamUrl(name, lastSeq > 0 ? lastSeq : undefined),
        { ...getSessionStreamInit(), signal: controller.signal },
      );
      if (!response.ok) {
        if (isPermanentSessionStreamStatus(response.status)) {
          stopped = true;
          options.onStateChange?.("closed");
          return;
        }
        throw new Error(`Session stream failed: ${response.status} ${response.statusText}`);
      }
      reconnectDelay = INITIAL_RECONNECT_MS;
      options.onStateChange?.("open");
      await consume(response);
      if (terminal) {
        options.onStateChange?.("closed");
        return;
      }
    } catch (error) {
      if (stopped || (error instanceof DOMException && error.name === "AbortError")) {
        return;
      }
    }
    if (stopped) return;
    options.onStateChange?.("reconnecting");
    reconnectTimer = setTimeout(() => void connect(), reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_MS);
  };

  void connect();

  return () => {
    stopped = true;
    controller?.abort();
    if (reconnectTimer != null) clearTimeout(reconnectTimer);
    options.onStateChange?.("closed");
  };
}
