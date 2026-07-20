import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  ArrowLeft,
  CircleStop,
  Pause,
  Play,
  Send,
  Wifi,
  WifiOff,
} from "lucide-react";
import { useParams } from "react-router-dom";
import { useSession, useSendSessionTurn, useSessionAction } from "../api/hooks";
import {
  mergeSessionEvents,
  subscribeToSession,
  type SessionStreamState,
} from "../api/sessionStream";
import type { SessionEvent } from "../api/types";
import { DetailSkeleton } from "../components/DetailSkeleton";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { StatusBadge } from "../components/StatusBadge";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";

interface TimelineItem {
  key: string;
  role: string;
  content: string;
  eventType?: string;
  timestamp?: string;
  seq?: number;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value != null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}

function stringField(
  record: Record<string, unknown> | undefined,
  ...keys: string[]
): string | undefined {
  for (const key of keys) {
    const value = record?.[key];
    if (typeof value === "string" && value.length > 0) return value;
  }
  return undefined;
}

function payloadContent(payload: unknown): string | undefined {
  if (typeof payload === "string") return payload;
  const record = asRecord(payload);
  const direct = stringField(record, "content", "text", "delta");
  if (direct) return direct;

  const message = asRecord(record?.message);
  const messageContent = stringField(message, "content", "text");
  if (messageContent) return messageContent;

  const delta = asRecord(record?.delta);
  return stringField(delta, "content", "text");
}

function payloadSummary(payload: unknown): string {
  if (payload == null) return "";
  if (typeof payload === "string") return payload;
  try {
    return JSON.stringify(payload, null, 2);
  } catch {
    return String(payload);
  }
}

function eventRole(event: SessionEvent): string {
  const payload = asRecord(event.payload);
  const explicit = stringField(payload, "role", "author", "sender");
  if (explicit) return explicit.toLowerCase();
  const type = event.type.toLowerCase();
  if (type.includes("user") || type.includes("human")) return "user";
  if (type.includes("assistant") || type.includes("agent") || type.includes("token")) {
    return "assistant";
  }
  return "system";
}

function buildTimeline(events: SessionEvent[]): TimelineItem[] {
  const items: TimelineItem[] = [];
  const messageIndexes = new Map<string, number>();

  for (const event of events) {
    const payload = asRecord(event.payload);
    const messageId =
      event.message_id ?? stringField(payload, "message_id", "id");
    const type = event.type.toLowerCase();
    const content = payloadContent(event.payload);
    const isDelta = type.includes("delta") || type.includes("token");
    if (type === "message.reset" && messageId) {
      const existingIndex = messageIndexes.get(messageId);
      if (existingIndex != null) {
        items[existingIndex].content = "";
        items[existingIndex].eventType = event.type;
        items[existingIndex].seq = event.seq;
      }
      continue;
    }

    if (content) {
      const groupingKey = messageId ?? (isDelta ? event.turn_id : undefined);
      const existingIndex = groupingKey
        ? messageIndexes.get(groupingKey)
        : undefined;
      if (existingIndex != null) {
        const existing = items[existingIndex];
        if (isDelta) existing.content += content;
        else if (existing.content !== content) existing.content = content;
        existing.seq = event.seq;
        continue;
      }

      const key = groupingKey ?? `event-${event.seq}`;
      messageIndexes.set(key, items.length);
      items.push({
        key: `event-${key}`,
        role: eventRole(event),
        content,
        eventType: event.type,
        timestamp: event.timestamp ?? stringField(payload, "timestamp", "created_at"),
        seq: event.seq,
      });
      continue;
    }

    const summary = payloadSummary(event.payload);
    items.push({
      key: `event-${event.seq}`,
      role: "system",
      content: summary || event.type.replace(/[._-]+/g, " "),
      eventType: event.type,
      timestamp: event.timestamp ?? stringField(payload, "timestamp", "created_at"),
      seq: event.seq,
    });
  }

  return items.filter((item) => item.content.length > 0);
}

function phaseOf(phase?: string): string {
  return phase ?? "Unknown";
}

export function SessionDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const routeName = nameParam ?? "";
  const { goBack } = useDetailReturnNav("/sessions");
  const sessionQuery = useSession(routeName);
  const turnMutation = useSendSessionTurn();
  const actionMutation = useSessionAction();
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const [streamState, setStreamState] = useState<SessionStreamState>("connecting");
  const [content, setContent] = useState("");
  const [interrupt, setInterrupt] = useState(false);
  const timelineEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setEvents([]);
  }, [routeName]);

  useEffect(() => {
    if (!routeName) return;
    return subscribeToSession(routeName, {
      onEvent: (event) => {
        setEvents((current) => mergeSessionEvents(current, [event]));
      },
      onStateChange: setStreamState,
    });
    // Reconnecting on every detail poll would interrupt the live stream.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [routeName]);

  const timeline = useMemo(() => buildTimeline(events), [events]);

  useEffect(() => {
    timelineEndRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [timeline.length]);

  if (sessionQuery.isError) {
    return (
      <ResourceDetailLoadError
        title="Session"
        message={
          sessionQuery.error instanceof Error
            ? sessionQuery.error.message
            : "Failed to load session"
        }
        goBack={goBack}
      />
    );
  }

  if (sessionQuery.isLoading || !sessionQuery.data) {
    return <DetailSkeleton />;
  }

  const session = sessionQuery.data;
  const phase = phaseOf(session.status?.phase);
  const phaseLower = phase.toLowerCase();
  const paused = phaseLower === "paused";
  const terminal =
    phaseLower === "cancelled" ||
    phaseLower === "canceled" ||
    phaseLower === "completed" ||
    phaseLower === "succeeded" ||
    phaseLower === "failed" ||
    phaseLower === "expired";
  const lastError = session.status?.lastError;

  const sendTurn = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = content.trim();
    if (!trimmed || turnMutation.isPending || terminal) return;
    try {
      await turnMutation.mutateAsync({
        name: routeName,
        content: trimmed,
        interrupt: interrupt || undefined,
        idempotencyKey: crypto.randomUUID(),
      });
      setContent("");
      setInterrupt(false);
    } catch {
      // The mutation error remains visible below the composer.
    }
  };

  const runAction = async (action: "pause" | "resume" | "cancel") => {
    try {
      await actionMutation.mutateAsync({ name: routeName, action });
    } catch {
      // The mutation error remains visible in the header.
    }
  };

  return (
    <div className="page session-detail">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back to sessions">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{session.metadata.name}</h1>
            <p className="page__subtitle">
              {session.spec.system ?? "No agent system"} &middot;{" "}
              {session.metadata.namespace ?? "default"}
            </p>
          </div>
          <StatusBadge phase={phase} size="md" pulse={phaseLower === "running"} />
        </div>
        <div className="page__header-actions session-detail__actions">
          <span
            className={`session-stream-state session-stream-state--${streamState}`}
            aria-live="polite"
          >
            {streamState === "open" ? <Wifi size={14} /> : <WifiOff size={14} />}
            {streamState === "open"
              ? "Live"
              : streamState === "reconnecting"
                ? "Reconnecting"
                : streamState === "closed"
                  ? "Disconnected"
                  : "Connecting"}
          </span>
          <button
            className="btn-secondary"
            disabled={terminal || actionMutation.isPending}
            onClick={() => void runAction(paused ? "resume" : "pause")}
          >
            {paused ? <Play size={14} /> : <Pause size={14} />}
            {paused ? "Resume" : "Pause"}
          </button>
          <button
            className="btn-secondary text-red"
            disabled={terminal || actionMutation.isPending}
            onClick={() => void runAction("cancel")}
          >
            <CircleStop size={14} /> Cancel
          </button>
        </div>
      </div>

      {(lastError || actionMutation.isError) && (
        <div className="list-fetch-error" role="alert">
          <p>
            {actionMutation.error instanceof Error
              ? actionMutation.error.message
              : lastError ?? "Session action failed"}
          </p>
        </div>
      )}

      <section className="session-chat" aria-label="Session conversation">
        <div className="session-chat__timeline" aria-live="polite">
          {timeline.length === 0 ? (
            <div className="session-chat__empty">
              <MessagesSquareIcon />
              <p>No messages yet</p>
              <span>Send a turn to begin the conversation.</span>
            </div>
          ) : (
            timeline.map((item) => (
              <article
                key={item.key}
                className={`session-message session-message--${item.role}`}
              >
                <div className="session-message__meta">
                  <span>{item.role}</span>
                  {item.eventType && <span className="mono">{item.eventType}</span>}
                  {item.seq != null && <span className="mono">#{item.seq}</span>}
                  {item.timestamp && (
                    <time dateTime={item.timestamp}>
                      {new Date(item.timestamp).toLocaleTimeString()}
                    </time>
                  )}
                </div>
                <pre className="session-message__content">{item.content}</pre>
              </article>
            ))
          )}
          <div ref={timelineEndRef} />
        </div>

        <form className="session-composer" onSubmit={sendTurn}>
          <textarea
            rows={3}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
            placeholder={
              terminal ? "This session has ended" : "Send a message…"
            }
            aria-label="Session message"
            disabled={terminal}
          />
          <div className="session-composer__footer">
            <label className="checkbox-inline">
              <input
                type="checkbox"
                checked={interrupt}
                onChange={(event) => setInterrupt(event.target.checked)}
                disabled={terminal}
              />
              Interrupt active turn
            </label>
            <button
              type="submit"
              className="btn-primary"
              disabled={!content.trim() || turnMutation.isPending || terminal}
            >
              <Send size={14} />
              {turnMutation.isPending ? "Sending…" : "Send"}
            </button>
          </div>
          {turnMutation.isError && (
            <p className="text-red text-xs" role="alert">
              {turnMutation.error instanceof Error
                ? turnMutation.error.message
                : "Failed to send turn"}
            </p>
          )}
        </form>
      </section>
    </div>
  );
}

function MessagesSquareIcon() {
  return <span className="session-chat__empty-icon" aria-hidden="true">•••</span>;
}
