import type {
  SessionCheckpoint,
  SessionEvent,
} from "../api/types";

export type SessionTimelineKind = "message" | "activity" | "checkpoint";

export interface SessionTimelineItem {
  key: string;
  kind: SessionTimelineKind;
  role: string;
  title: string;
  content?: string;
  eventType: string;
  timestamp?: string;
  seq: number;
  startSeq: number;
  endSeq: number;
  turnID?: string;
  messageID?: string;
  checkpointID?: string;
  checkpoint?: SessionCheckpoint;
  abandoned: boolean;
  reset?: boolean;
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

function numberField(
  record: Record<string, unknown> | undefined,
  ...keys: string[]
): number | undefined {
  for (const key of keys) {
    const value = record?.[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
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

function payloadSummary(payload: unknown): string | undefined {
  if (payload == null) return undefined;
  if (typeof payload === "string") return payload;
  const record = asRecord(payload);
  if (record && Object.keys(record).length === 0) return undefined;
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
  if (
    type.includes("assistant") ||
    type.includes("agent") ||
    type.includes("token")
  ) {
    return "assistant";
  }
  return "system";
}

function humanize(value: string): string {
  const normalized = value.replace(/[._-]+/g, " ").trim();
  return normalized
    .split(/\s+/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function checkpointTitle(
  checkpoint: SessionCheckpoint | undefined,
  payload: Record<string, unknown> | undefined,
): string {
  const safePoint =
    checkpoint?.safe_point ?? stringField(payload, "safe_point") ?? "checkpoint";
  const agent = checkpoint?.agent ?? stringField(payload, "agent");
  const step = humanize(safePoint);
  return agent ? `${step} · ${agent}` : step;
}

function activityCopy(event: SessionEvent): {
  title: string;
  content?: string;
} {
  const payload = asRecord(event.payload);
  const reason = stringField(payload, "reason", "message", "error");
  switch (event.type.toLowerCase()) {
    case "session.created":
      return { title: "Session created", content: stringField(payload, "system") };
    case "session.paused":
      return { title: "Session paused", content: reason };
    case "session.resumed":
      return { title: "Session resumed", content: reason };
    case "session.cancelled":
      return { title: "Session cancelled", content: reason };
    case "session.completed":
      return { title: "Session completed", content: reason };
    case "session.expired":
      return { title: "Session expired", content: reason };
    case "session.recovered":
      return { title: "Execution recovered", content: reason };
    case "session.rewound": {
      const checkpointID = stringField(payload, "checkpoint_id");
      const checkpointSequence = numberField(payload, "checkpoint_sequence");
      return {
        title: "Session rewound",
        content: checkpointID
          ? `Restored Session checkpoint ${checkpointID}${checkpointSequence != null ? ` at event #${checkpointSequence}` : ""}.`
          : reason,
      };
    }
    case "session.forked": {
      const source = stringField(payload, "source_session");
      const sourceCheckpoint = stringField(payload, "source_checkpoint_id");
      return {
        title: "Session forked",
        content: source
          ? `Forked from ${source}${sourceCheckpoint ? ` at Session checkpoint ${sourceCheckpoint}` : ""}.`
          : reason,
      };
    }
    case "turn.queued":
      return { title: "Turn queued", content: reason };
    case "turn.started":
      return {
        title: "Turn started",
        content: stringField(payload, "worker")
          ? `Worker ${stringField(payload, "worker")}`
          : reason,
      };
    case "turn.retrying":
      return { title: "Turn retrying", content: reason };
    case "turn.completed":
      return { title: "Turn completed", content: reason };
    case "turn.failed":
      return { title: "Turn failed", content: reason };
    case "turn.cancelled":
      return { title: "Turn cancelled", content: reason };
    case "tool.started": {
      const tool = stringField(payload, "tool") ?? "tool";
      const input = payload?.input;
      return {
        title: `${tool} started`,
        content: input == null ? undefined : payloadSummary(input),
      };
    }
    case "tool.completed": {
      const tool = stringField(payload, "tool") ?? "tool";
      const result = payload?.output ?? payload?.result ?? payload?.message;
      return {
        title: `${tool} completed`,
        content: result == null ? reason : payloadSummary(result),
      };
    }
    case "approval.requested":
      return { title: "Approval requested", content: reason };
    case "approval.resolved":
      return { title: "Approval resolved", content: reason };
    case "checkpoint.pruned":
      return { title: "Session checkpoints pruned", content: reason };
    case "error":
      return { title: "Execution error", content: reason ?? payloadSummary(event.payload) };
    default:
      return {
        title: humanize(event.type),
        content: payloadSummary(event.payload),
      };
  }
}

export function activeSessionCheckpointIDs(
  checkpoints: SessionCheckpoint[],
  headID?: string,
): Set<string> {
  const byID = new Map(checkpoints.map((checkpoint) => [checkpoint.id, checkpoint]));
  const active = new Set<string>();
  let current = headID?.trim() ?? "";
  while (current && !active.has(current)) {
    active.add(current);
    const checkpoint = byID.get(current);
    if (!checkpoint) break;
    const parent = checkpoint.parent_checkpoint_id?.trim() ?? "";
    // Fork roots point to sourceSession/checkpointID and are intentionally
    // external to this Session's checkpoint collection.
    if (!parent || !byID.has(parent)) break;
    current = parent;
  }
  return active;
}

export function buildSessionTimeline(
  events: SessionEvent[],
  checkpoints: SessionCheckpoint[],
  lastCheckpointID?: string,
): SessionTimelineItem[] {
  const sortedEvents = [...events].sort((a, b) => a.seq - b.seq);
  const checkpointsByID = new Map(
    checkpoints.map((checkpoint) => [checkpoint.id, checkpoint]),
  );
  const activeCheckpointIDs = activeSessionCheckpointIDs(
    checkpoints,
    lastCheckpointID,
  );
  const items: SessionTimelineItem[] = [];
  const messageIndexes = new Map<string, number>();

  for (const event of sortedEvents) {
    const payload = asRecord(event.payload);
    const messageID =
      event.message_id ?? stringField(payload, "message_id", "id");
    const type = event.type.toLowerCase();
    const isMessage =
      type.startsWith("message.") ||
      type.includes("token") ||
      type.includes("delta");

    if (type === "message.reset" && messageID) {
      const existingIndex = messageIndexes.get(messageID);
      if (existingIndex != null) {
        const existing = items[existingIndex];
        existing.endSeq = event.seq;
        existing.seq = event.seq;
        existing.reset = true;
        existing.abandoned = true;
      }
      messageIndexes.delete(messageID);
      const copy = activityCopy(event);
      items.push({
        key: `event-${event.seq}`,
        kind: "activity",
        role: "system",
        title: "Tentative output reset",
        content: copy.content,
        eventType: event.type,
        timestamp: event.timestamp,
        seq: event.seq,
        startSeq: event.seq,
        endSeq: event.seq,
        turnID: event.turn_id,
        messageID,
        abandoned: false,
      });
      continue;
    }

    if (isMessage) {
      const content = payloadContent(event.payload);
      if (content) {
        const isDelta = type.includes("delta") || type.includes("token");
        const groupingKey = messageID ?? (isDelta ? event.turn_id : undefined);
        const existingIndex = groupingKey
          ? messageIndexes.get(groupingKey)
          : undefined;
        if (existingIndex != null) {
          const existing = items[existingIndex];
          if (isDelta) existing.content = `${existing.content ?? ""}${content}`;
          else if (existing.content !== content) existing.content = content;
          existing.endSeq = event.seq;
          existing.seq = event.seq;
          existing.eventType = event.type;
          continue;
        }

        const key = groupingKey ?? `event-${event.seq}`;
        messageIndexes.set(key, items.length);
        items.push({
          key: `message-${key}`,
          kind: "message",
          role: eventRole(event),
          title: eventRole(event),
          content,
          eventType: event.type,
          timestamp:
            event.timestamp ??
            stringField(payload, "timestamp", "created_at"),
          seq: event.seq,
          startSeq: event.seq,
          endSeq: event.seq,
          turnID: event.turn_id,
          messageID,
          abandoned: false,
        });
        continue;
      }
    }

    if (type === "checkpoint.created") {
      const checkpointID = stringField(payload, "checkpoint_id");
      const checkpoint = checkpointID
        ? checkpointsByID.get(checkpointID)
        : undefined;
      const isKnownAbandoned =
        activeCheckpointIDs.size > 0 &&
        checkpointID != null &&
        !activeCheckpointIDs.has(checkpointID);
      items.push({
        key: `checkpoint-${checkpointID ?? event.seq}`,
        kind: "checkpoint",
        role: "system",
        title: checkpointTitle(checkpoint, payload),
        content: checkpointID
          ? `Session checkpoint ${checkpointID}`
          : "Session checkpoint created",
        eventType: event.type,
        timestamp: checkpoint?.created_at ?? event.timestamp,
        seq: event.seq,
        startSeq: event.seq,
        endSeq: event.seq,
        turnID: event.turn_id,
        checkpointID,
        checkpoint,
        abandoned: isKnownAbandoned,
      });
      continue;
    }

    const copy = activityCopy(event);
    items.push({
      key: `event-${event.seq}`,
      kind: "activity",
      role: "system",
      title: copy.title,
      content: copy.content,
      eventType: event.type,
      timestamp: event.timestamp,
      seq: event.seq,
      startSeq: event.seq,
      endSeq: event.seq,
      turnID: event.turn_id,
      messageID,
      checkpointID: stringField(payload, "checkpoint_id"),
      abandoned: false,
    });
  }

  // The runtime rebuilds the active transcript from the most recent rewind.
  // Earlier abandoned segments may become active again when an operator rewinds
  // to a checkpoint on one of those branches.
  const latestRewind = [...sortedEvents]
    .reverse()
    .find((event) => event.type.toLowerCase() === "session.rewound");
  const latestRewindPayload = asRecord(latestRewind?.payload);
  const latestCheckpointSequence = numberField(
    latestRewindPayload,
    "checkpoint_sequence",
  );
  const rewindRange =
    latestRewind &&
    latestCheckpointSequence != null &&
    latestCheckpointSequence < latestRewind.seq
      ? { after: latestCheckpointSequence, before: latestRewind.seq }
      : undefined;

  for (const item of items) {
    if (
      rewindRange &&
      item.endSeq > rewindRange.after &&
      item.startSeq < rewindRange.before
    ) {
      item.abandoned = true;
    }
    if (
      item.kind === "checkpoint" &&
      item.checkpointID &&
      activeCheckpointIDs.has(item.checkpointID)
    ) {
      item.abandoned = false;
    }
  }

  return items;
}
