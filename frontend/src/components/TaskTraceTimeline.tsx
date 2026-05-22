import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { ChevronRight } from "lucide-react";
import type { Task } from "../api/types";

interface TaskTraceTimelineProps {
  tasks: Task[];
  systemName: string;
}

interface TraceEvent {
  timestamp: number;
  description: string;
  taskName: string;
  color: "gold" | "green" | "red" | "blue" | "gray";
}

function formatRelative(ts: number): string {
  const sec = Math.round((Date.now() - ts) / 1000);
  if (sec < 5) return "just now";
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  return `${Math.floor(hr / 24)}d ago`;
}

function eventColor(phase?: string, type?: string): TraceEvent["color"] {
  if (phase === "Succeeded" || type === "succeeded") return "green";
  if (phase === "Failed" || type === "failed" || type === "error") return "red";
  if (phase === "Running" || type === "assigned" || type === "started") return "gold";
  if (type === "tool_call" || type === "tool") return "blue";
  return "gray";
}

function describeMessage(msg: { from_agent?: string; to_agent?: string; phase?: string; type?: string }): string {
  if (msg.to_agent && msg.phase === "Running") return `${msg.to_agent} started`;
  if (msg.to_agent && msg.phase === "Succeeded") return `${msg.to_agent} completed`;
  if (msg.to_agent && msg.phase === "Failed") return `${msg.to_agent} failed`;
  if (msg.to_agent) return `${msg.to_agent} ${(msg.phase ?? "active").toLowerCase()}`;
  if (msg.phase) return `task ${msg.phase.toLowerCase()}`;
  return "event";
}

function describeHistory(evt: { type?: string; message?: string }): string {
  if (evt.message) return evt.message.length > 40 ? evt.message.slice(0, 37) + "..." : evt.message;
  if (evt.type) return evt.type.replace(/_/g, " ");
  return "event";
}

export function TaskTraceTimeline({ tasks, systemName }: TaskTraceTimelineProps) {
  const navigate = useNavigate();

  const events = useMemo(() => {
    const systemTasks = tasks.filter((t) => t.spec.system === systemName);
    const all: TraceEvent[] = [];

    for (const task of systemTasks) {
      for (const msg of task.status?.messages ?? []) {
        if (!msg.timestamp) continue;
        const ts = new Date(msg.timestamp).getTime();
        if (Number.isNaN(ts)) continue;
        all.push({
          timestamp: ts,
          description: describeMessage(msg),
          taskName: task.metadata.name,
          color: eventColor(msg.phase, msg.type),
        });
      }

      for (const evt of task.status?.history ?? []) {
        if (!evt.timestamp) continue;
        const ts = new Date(evt.timestamp).getTime();
        if (Number.isNaN(ts)) continue;
        all.push({
          timestamp: ts,
          description: describeHistory(evt),
          taskName: task.metadata.name,
          color: eventColor(undefined, evt.type),
        });
      }
    }

    all.sort((a, b) => b.timestamp - a.timestamp);
    return all.slice(0, 5);
  }, [tasks, systemName]);

  if (events.length === 0) {
    return (
      <div className="trace-timeline">
        <h3 className="trace-timeline__title">TASK TRACE TIMELINE</h3>
        <div className="trace-timeline__empty">No trace events yet</div>
      </div>
    );
  }

  return (
    <div className="trace-timeline">
      <h3 className="trace-timeline__title">TASK TRACE TIMELINE</h3>
      <div className="trace-timeline__feed">
        {events.map((evt, i) => (
          <div
            key={`${evt.taskName}-${evt.timestamp}-${i}`}
            className="trace-timeline__event"
            style={{ animationDelay: `${i * 50}ms` }}
            onClick={() => navigate(`/tasks/${encodeURIComponent(evt.taskName)}?tab=trace`)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                navigate(`/tasks/${encodeURIComponent(evt.taskName)}?tab=trace`);
              }
            }}
          >
            <span className={`trace-timeline__dot trace-timeline__dot--${evt.color}`} />
            <span className="trace-timeline__time">{formatRelative(evt.timestamp)}</span>
            <span className="trace-timeline__desc">{evt.description}</span>
            <span className="trace-timeline__task mono">{evt.taskName}</span>
          </div>
        ))}
      </div>
      <button
        type="button"
        className="trace-timeline__footer"
        onClick={() => navigate("/tasks")}
      >
        View all traces <ChevronRight size={12} />
      </button>
    </div>
  );
}
