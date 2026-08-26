import {
  AlertTriangle,
  CircleDot,
  Flag,
  GitFork,
  RotateCcw,
  Wrench,
} from "lucide-react";
import type { RefObject } from "react";
import type { SessionTimelineItem } from "../utils/sessionTimeline";
import { AiDisclosure } from "./AiDisclosure";

interface SessionTimelineProps {
  items: SessionTimelineItem[];
  selectedCheckpointID?: string;
  onSelectCheckpoint: (checkpointID: string) => void;
  endRef?: RefObject<HTMLDivElement | null>;
}

interface TimelineGroup {
  abandoned: boolean;
  items: SessionTimelineItem[];
}

function groupTimeline(items: SessionTimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  for (const item of items) {
    const current = groups[groups.length - 1];
    if (!current || current.abandoned !== item.abandoned) {
      groups.push({ abandoned: item.abandoned, items: [item] });
    } else {
      current.items.push(item);
    }
  }
  return groups;
}

function eventIcon(item: SessionTimelineItem) {
  if (item.eventType.startsWith("tool.")) return <Wrench size={14} />;
  if (item.eventType === "session.rewound") return <RotateCcw size={14} />;
  if (item.eventType === "session.forked") return <GitFork size={14} />;
  if (item.eventType === "error" || item.eventType.endsWith(".failed")) {
    return <AlertTriangle size={14} />;
  }
  return <CircleDot size={12} />;
}

function eventTime(timestamp?: string): string | undefined {
  if (!timestamp) return undefined;
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? undefined : date.toLocaleTimeString();
}

function TimelineEntry({
  item,
  selectedCheckpointID,
  onSelectCheckpoint,
}: {
  item: SessionTimelineItem;
  selectedCheckpointID?: string;
  onSelectCheckpoint: (checkpointID: string) => void;
}) {
  if (item.kind === "checkpoint") {
    const checkpointID = item.checkpointID;
    return (
      <button
        type="button"
        className={`session-checkpoint-marker${
          selectedCheckpointID === checkpointID
            ? " session-checkpoint-marker--selected"
            : ""
        }`}
        disabled={!checkpointID || !item.checkpoint}
        onClick={() => checkpointID && onSelectCheckpoint(checkpointID)}
        title={checkpointID ? `Session checkpoint ${checkpointID}` : undefined}
      >
        <span className="session-checkpoint-marker__line" />
        <span className="session-checkpoint-marker__icon">
          <Flag size={14} />
        </span>
        <span className="session-checkpoint-marker__copy">
          <strong>{item.title}</strong>
          <span>
            {checkpointID ? checkpointID.slice(0, 12) : "Checkpoint unavailable"}
            {" · "}#{item.seq}
          </span>
        </span>
        <span className="session-checkpoint-marker__state">
          {item.abandoned ? "Abandoned" : "Active lineage"}
        </span>
      </button>
    );
  }

  if (item.kind === "message") {
    return (
      <article
        className={`session-message session-message--${item.role}${
          item.reset ? " session-message--reset" : ""
        }`}
      >
        <div className="session-message__meta">
          <span>{item.role}</span>
          <span className="mono">{item.eventType}</span>
          <span className="mono">#{item.startSeq}–{item.endSeq}</span>
          {eventTime(item.timestamp) && (
            <time dateTime={item.timestamp}>{eventTime(item.timestamp)}</time>
          )}
          {item.reset && <span className="session-timeline__reset-label">Reset</span>}
        </div>
        {item.role === "assistant" ? <AiDisclosure kind="generated-output" compact /> : null}
        <pre className="session-message__content">{item.content}</pre>
      </article>
    );
  }

  return (
    <article className={`session-activity session-activity--${item.eventType.replace(/\./g, "-")}`}>
      <span className="session-activity__icon">{eventIcon(item)}</span>
      <div className="session-activity__copy">
        <div className="session-activity__title">
          <strong>{item.title}</strong>
          <span className="mono">{item.eventType}</span>
          <span className="mono">#{item.seq}</span>
          {eventTime(item.timestamp) && (
            <time dateTime={item.timestamp}>{eventTime(item.timestamp)}</time>
          )}
        </div>
        {item.content && <pre>{item.content}</pre>}
      </div>
    </article>
  );
}

export function SessionTimeline({
  items,
  selectedCheckpointID,
  onSelectCheckpoint,
  endRef,
}: SessionTimelineProps) {
  if (items.length === 0) {
    return (
      <div className="session-chat__empty">
        <span className="session-chat__empty-icon" aria-hidden="true">•••</span>
        <p>No activity yet</p>
        <span>Send a turn to begin the live timeline.</span>
      </div>
    );
  }

  return (
    <>
      {groupTimeline(items).map((group, groupIndex) => {
        if (!group.abandoned) {
          return (
            <div className="session-timeline__active" key={`active-${groupIndex}`}>
              {group.items.map((item) => (
                <TimelineEntry
                  key={item.key}
                  item={item}
                  selectedCheckpointID={selectedCheckpointID}
                  onSelectCheckpoint={onSelectCheckpoint}
                />
              ))}
            </div>
          );
        }

        const first = group.items[0];
        const last = group.items[group.items.length - 1];
        return (
          <details
            className="session-timeline__abandoned"
            key={`abandoned-${first?.key ?? groupIndex}`}
          >
            <summary>
              <span>Abandoned timeline</span>
              <span>
                {group.items.length} events · #{first?.startSeq ?? "?"}–#
                {last?.endSeq ?? "?"}
              </span>
            </summary>
            <div className="session-timeline__abandoned-items">
              {group.items.map((item) => (
                <TimelineEntry
                  key={item.key}
                  item={item}
                  selectedCheckpointID={selectedCheckpointID}
                  onSelectCheckpoint={onSelectCheckpoint}
                />
              ))}
            </div>
          </details>
        );
      })}
      <div ref={endRef} />
    </>
  );
}
