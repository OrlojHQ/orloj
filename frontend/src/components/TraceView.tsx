import { useState, useMemo } from "react";
import type { TaskTraceEvent } from "../api/types";
import { AiDisclosure } from "./AiDisclosure";
import { StatusBadge } from "./StatusBadge";
import type { AiAttribution } from "../compliance/aiAttribution";
import {
  Clock, Cpu, Wrench, AlertTriangle, ChevronDown, ChevronRight,
  Zap, Download, Search, Activity, Copy,
} from "lucide-react";
import { toast } from "./Toast";
import clsx from "clsx";

interface TraceViewProps {
  trace: TaskTraceEvent[];
  resolveAttribution?: (agent?: string | null) => AiAttribution | undefined;
}

type EventDot = "green" | "blue" | "yellow" | "red" | "orange" | "purple" | "gray" | "accent";

const EVENT_TYPE_COLORS: Record<string, { dot: EventDot; color: string; label: string }> = {
  task_start:               { dot: "blue",   color: "var(--blue)",   label: "Task Start" },
  task_summary:             { dot: "blue",   color: "var(--blue)",   label: "Task Summary" },
  agent_start:              { dot: "green",  color: "var(--green)",  label: "Agent Start" },
  agent_end:                { dot: "green",  color: "var(--green)",  label: "Agent End" },
  agent_event:              { dot: "green",  color: "var(--green)",  label: "Agent Event" },
  agent_error:              { dot: "red",    color: "var(--red)",    label: "Agent Error" },
  agent_message:            { dot: "purple", color: "var(--purple)", label: "Message" },
  agent_message_processed:  { dot: "purple", color: "var(--purple)", label: "Msg Processed" },
  agent_message_deadletter: { dot: "orange", color: "var(--orange)", label: "Dead Letter" },
  model_call:               { dot: "accent", color: "var(--accent)", label: "Model Call" },
  model_output:             { dot: "accent", color: "var(--accent)", label: "Model Output" },
  tool_call:                { dot: "yellow", color: "var(--yellow)", label: "Tool Call" },
  context_adapter:          { dot: "yellow", color: "var(--yellow)", label: "Context Adapter" },
  task_approval_pending:             { dot: "accent", color: "var(--accent)", label: "Approval Pending" },
  task_approval_approved:            { dot: "green",  color: "var(--green)",  label: "Approval Approved" },
  task_approval_denied:              { dot: "red",    color: "var(--red)",    label: "Approval Denied" },
  task_approval_expired:             { dot: "orange", color: "var(--orange)", label: "Approval Expired" },
  task_approval_changes_requested:   { dot: "yellow", color: "var(--yellow)", label: "Changes Requested" },
  tool_approval_pending:             { dot: "accent", color: "var(--accent)", label: "Tool Approval Pending" },
  tool_approval_approved:            { dot: "green",  color: "var(--green)",  label: "Tool Approval Approved" },
  tool_approval_denied:              { dot: "red",    color: "var(--red)",    label: "Tool Approval Denied" },
  tool_approval_expired:             { dot: "orange", color: "var(--orange)", label: "Tool Approval Expired" },
  retry_scheduled:          { dot: "orange", color: "var(--orange)", label: "Retry" },
  deadletter:               { dot: "orange", color: "var(--orange)", label: "Dead Letter" },
};

function getEventStyle(type?: string): { dot: EventDot; color: string; label: string } {
  if (!type) return { dot: "gray", color: "var(--text-tertiary)", label: "unknown" };
  return EVENT_TYPE_COLORS[type] ?? { dot: "gray", color: "var(--text-tertiary)", label: type };
}

function formatDuration(ms?: number): string {
  if (ms == null) return "—";
  if (ms < 1) return "<1ms";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`;
  const min = Math.floor(ms / 60_000);
  return `${min}m ${((ms - min * 60_000) / 1000).toFixed(0)}s`;
}

/** Compact labels for the timeline axis so adjacent ticks don't collide. */
function formatAxisTick(ms: number): string {
  if (ms < 1) return "0";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60_000) {
    const s = ms / 1000;
    return s < 10 ? `${s.toFixed(1)}s` : `${Math.round(s)}s`;
  }
  const min = Math.floor(ms / 60_000);
  const sec = Math.round((ms - min * 60_000) / 1000);
  // Prefer "4m52s" over "4:52" so it reads as a duration, not a clock time.
  return sec === 0 ? `${min}m` : `${min}m${sec}s`;
}

function axisTicksFor(totalMs: number): number[] {
  // Fewer ticks once labels grow (minutes) so the last markers stay readable.
  if (totalMs >= 120_000) return [0, 0.5, 1];
  if (totalMs >= 30_000) return [0, 1 / 3, 2 / 3, 1];
  return [0, 0.25, 0.5, 0.75, 1];
}

function formatOffset(ms: number): string {
  if (ms < 1000) return `+${Math.round(ms)}ms`;
  return `+${(ms / 1000).toFixed(2)}s`;
}

function formatTokens(n?: number): string {
  if (n == null || n === 0) return "";
  if (n < 1000) return `${n}`;
  return `${(n / 1000).toFixed(1)}k`;
}

function eventTokens(ev: TaskTraceEvent): number {
  if (!ev.tokens || ev.type === "model_output") return 0;
  return ev.tokens;
}

interface Row {
  ev: TaskTraceEvent;
  key: string;
  /** Offsets relative to trace origin, in ms. Spans are [start, end]; point events have start === end. */
  startMs: number;
  endMs: number;
  isPoint: boolean;
}

interface BranchLane {
  key: string;
  branchId: string | null;
  parentBranchId?: string;
  depth: number;
  rows: Row[];
  startMs: number;
  endMs: number;
  tokens: number;
}

interface Group {
  key: string;
  agent: string | null;
  rows: Row[];
  lanes: BranchLane[];
  startMs: number;
  endMs: number;
  tokens: number;
}

interface TraceModel {
  rows: Row[];
  totalMs: number;
  totalTokens: number;
  totalToolCalls: number;
  modelTimeMs: number;
  errorCount: number;
  agents: string[];
  branches: string[];
  tokenSeries: number[];
}

function buildModel(trace: TaskTraceEvent[]): TraceModel {
  const agents = new Set<string>();
  const branches = new Set<string>();
  let totalTokens = 0;
  let totalToolCalls = 0;
  let modelTimeMs = 0;
  let errorCount = 0;

  const raw = trace.map((ev, i) => {
    const ts = ev.timestamp ? new Date(ev.timestamp).getTime() : NaN;
    const latency = ev.latency_ms ?? 0;
    if (ev.agent) agents.add(ev.agent);
    if (ev.branch_id) branches.add(ev.branch_id);
    const tok = eventTokens(ev);
    totalTokens += tok;
    if (ev.tool_calls) totalToolCalls += ev.tool_calls;
    if (ev.type === "model_call" && ev.latency_ms) modelTimeMs += ev.latency_ms;
    if (ev.error_code || ev.type === "agent_error") errorCount++;
    return {
      ev,
      key: `${i}|${ev.timestamp ?? ""}|${ev.type ?? ""}`,
      tsMs: ts,
      // The event timestamp marks completion; latency_ms is how long the operation ran.
      spanStart: Number.isNaN(ts) ? NaN : ts - latency,
      isPoint: latency <= 0,
    };
  });

  const starts = raw.map((r) => r.spanStart).filter((v) => !Number.isNaN(v));
  const ends = raw.map((r) => r.tsMs).filter((v) => !Number.isNaN(v));
  const origin = starts.length ? Math.min(...starts) : 0;
  const last = ends.length ? Math.max(...ends) : origin;
  const totalMs = Math.max(last - origin, 1);

  const rows: Row[] = raw.map((r) => ({
    ev: r.ev,
    key: r.key,
    startMs: Number.isNaN(r.spanStart) ? 0 : r.spanStart - origin,
    endMs: Number.isNaN(r.tsMs) ? 0 : r.tsMs - origin,
    isPoint: r.isPoint,
  }));

  let cumulative = 0;
  const tokenSeries = rows.map((row) => {
    cumulative += eventTokens(row.ev);
    return cumulative;
  });

  return {
    rows,
    totalMs,
    totalTokens,
    totalToolCalls,
    modelTimeMs,
    errorCount,
    agents: [...agents].sort(),
    branches: [...branches].sort(),
    tokenSeries,
  };
}

function laneFromRows(key: string, branchId: string | null, parentBranchId: string | undefined, depth: number, rows: Row[]): BranchLane {
  let startMs = rows[0]?.startMs ?? 0;
  let endMs = rows[0]?.endMs ?? 0;
  let tokens = 0;
  for (const row of rows) {
    startMs = Math.min(startMs, row.startMs);
    endMs = Math.max(endMs, row.endMs);
    tokens += eventTokens(row.ev);
  }
  return { key, branchId, parentBranchId, depth, rows, startMs, endMs, tokens };
}

/** Build branch sub-lanes under an agent group, nesting via parent_branch_id when present. */
function buildBranchLanes(groupKey: string, rows: Row[]): BranchLane[] {
  const unbranched: Row[] = [];
  const byBranch = new Map<string, Row[]>();
  const parentOf = new Map<string, string | undefined>();

  for (const row of rows) {
    const bid = row.ev.branch_id?.trim();
    if (!bid) {
      unbranched.push(row);
      continue;
    }
    const list = byBranch.get(bid) ?? [];
    list.push(row);
    byBranch.set(bid, list);
    if (!parentOf.has(bid)) {
      const parent = row.ev.parent_branch_id?.trim() || undefined;
      parentOf.set(bid, parent);
    }
  }

  if (byBranch.size === 0) {
    return [laneFromRows(`${groupKey}|default`, null, undefined, 0, rows)];
  }

  const children = new Map<string, string[]>();
  const roots: string[] = [];
  for (const bid of byBranch.keys()) {
    const parent = parentOf.get(bid);
    if (parent && byBranch.has(parent) && parent !== bid) {
      const list = children.get(parent) ?? [];
      list.push(bid);
      children.set(parent, list);
    } else {
      roots.push(bid);
    }
  }

  // Stable order: first appearance in the group's event stream.
  const firstIndex = new Map<string, number>();
  rows.forEach((row, i) => {
    const bid = row.ev.branch_id?.trim();
    if (bid && !firstIndex.has(bid)) firstIndex.set(bid, i);
  });
  const byAppearance = (a: string, b: string) => (firstIndex.get(a) ?? 0) - (firstIndex.get(b) ?? 0);
  roots.sort(byAppearance);
  for (const [, kids] of children) kids.sort(byAppearance);

  const lanes: BranchLane[] = [];
  if (unbranched.length > 0) {
    lanes.push(laneFromRows(`${groupKey}|default`, null, undefined, 0, unbranched));
  }

  const visit = (bid: string, depth: number) => {
    const branchRows = byBranch.get(bid);
    if (!branchRows) return;
    lanes.push(laneFromRows(`${groupKey}|b|${bid}`, bid, parentOf.get(bid), depth, branchRows));
    for (const child of children.get(bid) ?? []) visit(child, depth + 1);
  };
  for (const root of roots) visit(root, 0);

  return lanes;
}

/** Group consecutive rows by agent so the waterfall reads as one lane per agent run. */
function groupRows(rows: Row[]): Group[] {
  const groups: Group[] = [];
  for (const row of rows) {
    const agent = row.ev.agent ?? null;
    const prev = groups[groups.length - 1];
    if (!prev || prev.agent !== agent) {
      groups.push({
        key: `g${groups.length}-${agent ?? "task"}`,
        agent,
        rows: [row],
        lanes: [],
        startMs: row.startMs,
        endMs: row.endMs,
        tokens: eventTokens(row.ev),
      });
    } else {
      prev.rows.push(row);
      prev.startMs = Math.min(prev.startMs, row.startMs);
      prev.endMs = Math.max(prev.endMs, row.endMs);
      prev.tokens += eventTokens(row.ev);
    }
  }
  for (const g of groups) {
    g.lanes = buildBranchLanes(g.key, g.rows);
  }
  return groups;
}

function generateTokenSparkPath(series: number[], width: number, height: number): string {
  if (series.length === 0) return "";
  const max = Math.max(...series, 1);
  if (series.length === 1) {
    const y = height - (series[0] / max) * height * 0.85 - height * 0.08;
    return `M 0 ${y} L ${width} ${y}`;
  }
  const xStep = width / (series.length - 1);
  const points = series.map((v, idx) => {
    const x = idx * xStep;
    const y = height - (v / max) * height * 0.85 - height * 0.08;
    return `${x} ${y}`;
  });
  return `M ${points[0]} ${points.slice(1).map((p) => `L ${p}`).join(" ")}`;
}

const SPARK_WIDTH = 120;
const SPARK_HEIGHT = 28;

export function TraceView({ trace, resolveAttribution }: TraceViewProps) {
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());
  const [collapsedLanes, setCollapsedLanes] = useState<Set<string>>(new Set());
  const [filterAgent, setFilterAgent] = useState<string | null>(null);
  const [filterBranch, setFilterBranch] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");

  const model = useMemo(() => buildModel(trace), [trace]);

  const filteredRows = useMemo(() => {
    const q = searchQuery.toLowerCase();
    return model.rows.filter(({ ev }) => {
      if (filterAgent && ev.agent !== filterAgent) return false;
      if (filterBranch && ev.branch_id !== filterBranch) return false;
      if (q) {
        const text = [ev.type, ev.agent, ev.tool, ev.message, ev.error_code, ev.error_reason, ev.step_id, ev.branch_id]
          .filter(Boolean)
          .join(" ")
          .toLowerCase();
        if (!text.includes(q)) return false;
      }
      return true;
    });
  }, [model.rows, filterAgent, filterBranch, searchQuery]);

  const groups = useMemo(() => groupRows(filteredRows), [filteredRows]);

  const sparkPath = useMemo(
    () => generateTokenSparkPath(model.tokenSeries, SPARK_WIDTH, SPARK_HEIGHT),
    [model.tokenSeries],
  );

  const toggleExpanded = (key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleGroup = (key: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleLane = (key: string) => {
    setCollapsedLanes((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const handleExport = () => {
    try {
      const json = JSON.stringify(trace, null, 2);
      const blob = new Blob([json], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `trace-${Date.now()}.json`;
      a.click();
      URL.revokeObjectURL(url);
      toast("success", "Trace exported");
    } catch {
      toast("error", "Failed to export trace");
    }
  };

  const copyEventJson = async (ev: TaskTraceEvent) => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(ev, null, 2));
      toast("success", "Event JSON copied");
    } catch {
      toast("error", "Failed to copy event JSON");
    }
  };

  if (trace.length === 0) {
    return (
      <div className="trace-view trace-view--empty">
        <Activity size={28} className="trace-view__empty-icon" />
        <div className="trace-view__empty-title">No trace events yet</div>
        <div className="trace-view__empty-desc">Events appear here in real time once the task starts executing.</div>
      </div>
    );
  }

  const { totalMs } = model;
  const axisTicks = axisTicksFor(totalMs);
  const hasBranchLanes = (group: Group) =>
    group.lanes.some((l) => l.branchId != null) || group.lanes.length > 1;

  const renderBar = (row: Row) => {
    const style = getEventStyle(row.ev.type);
    const left = Math.min((row.startMs / totalMs) * 100, 100);
    const width = Math.max(((row.endMs - row.startMs) / totalMs) * 100, 0);
    const tooltip = `${style.label} · t${formatOffset(row.startMs)}${row.isPoint ? "" : ` · ${formatDuration(row.endMs - row.startMs)}`}`;
    return (
      <div className="trace-view__bar-track" title={tooltip}>
        {axisTicks.slice(1, -1).map((t) => (
          <span key={t} className="trace-view__bar-gridline" style={{ left: `${t * 100}%` }} />
        ))}
        {row.isPoint ? (
          <span className="trace-view__bar-point" style={{ left: `${left}%`, background: style.color }} />
        ) : (
          <div className="trace-view__bar-fill" style={{ left: `${left}%`, width: `${width}%`, background: style.color }} />
        )}
      </div>
    );
  };

  const renderEventRow = (row: Row, indent = 0) => {
    const { ev } = row;
    const style = getEventStyle(ev.type);
    const isExpanded = expandedKeys.has(row.key);
    const isError = !!ev.error_code || ev.type === "agent_error";
    const isGeneratedEvent = Boolean(ev.message) && (ev.type === "model_output" || ev.type === "agent_message");
    const attribution = isGeneratedEvent ? resolveAttribution?.(ev.agent) : undefined;
    return (
      <div key={row.key}>
        <div
          className={clsx(
            "trace-view__row",
            isError && "trace-view__row--error",
            isExpanded && "trace-view__row--expanded",
            indent > 0 && "trace-view__row--branched",
          )}
          style={indent > 0 ? { paddingLeft: 12 + indent * 16 } : undefined}
          onClick={() => toggleExpanded(row.key)}
        >
          <div className="trace-view__col-expand">
            {isExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          </div>
          <div className="trace-view__col-type">
            <span className={clsx("badge", "badge--neutral", `badge--dot-${style.dot}`, "trace-view__type-badge")}>
              <span className="badge__dot" />
              {style.label}
            </span>
          </div>
          <div className="trace-view__col-detail text-ellipsis">
            {isGeneratedEvent ? <AiDisclosure kind="generated-output" provider={attribution?.provider} modelId={attribution?.modelId} compact /> : null}
            {ev.tool ?? ev.message ?? ev.step_id ?? ""}
          </div>
          <div className="trace-view__col-start mono">{formatOffset(row.startMs)}</div>
          <div className="trace-view__col-latency mono">
            {row.isPoint ? "—" : formatDuration(row.endMs - row.startMs)}
          </div>
          <div className="trace-view__col-tokens mono">{formatTokens(ev.tokens)}</div>
          <div className="trace-view__col-bar">{renderBar(row)}</div>
        </div>

        {isExpanded && (
          <div className="trace-view__detail" style={indent > 0 ? { paddingLeft: 24 + indent * 16 } : undefined}>
            <div className="trace-view__detail-actions">
              <button
                type="button"
                className="btn-ghost btn-sm"
                onClick={(e) => {
                  e.stopPropagation();
                  void copyEventJson(ev);
                }}
                title="Copy event JSON"
                aria-label="Copy event JSON"
              >
                <Copy size={13} /> Copy JSON
              </button>
            </div>
            <div className="trace-view__detail-grid">
              {ev.timestamp && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Timestamp</span>
                  <span className="trace-view__detail-value">{new Date(ev.timestamp).toLocaleString()}</span>
                </div>
              )}
              <div className="trace-view__detail-field">
                <span className="trace-view__detail-label">Offset</span>
                <span className="trace-view__detail-value mono">t{formatOffset(row.startMs)}</span>
              </div>
              {ev.step_id && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Step ID</span>
                  <span className="trace-view__detail-value mono">{ev.step_id}</span>
                </div>
              )}
              {ev.attempt != null && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Attempt</span>
                  <span className="trace-view__detail-value">{ev.attempt}</span>
                </div>
              )}
              {ev.branch_id && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Branch</span>
                  <span className="trace-view__detail-value mono">{ev.branch_id}</span>
                </div>
              )}
              {ev.parent_branch_id && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Parent Branch</span>
                  <span className="trace-view__detail-value mono">{ev.parent_branch_id}</span>
                </div>
              )}
              {ev.tool && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Tool</span>
                  <span className="trace-view__detail-value mono">{ev.tool}</span>
                </div>
              )}
              {ev.tool_calls != null && ev.tool_calls > 0 && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Tool Calls</span>
                  <span className="trace-view__detail-value">{ev.tool_calls}</span>
                </div>
              )}
              {ev.tokens != null && ev.tokens > 0 && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Tokens</span>
                  <span className="trace-view__detail-value">{ev.tokens.toLocaleString()}</span>
                </div>
              )}
              {ev.input_tokens != null && ev.input_tokens > 0 && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Input Tokens</span>
                  <span className="trace-view__detail-value">{ev.input_tokens.toLocaleString()}</span>
                </div>
              )}
              {ev.output_tokens != null && ev.output_tokens > 0 && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Output Tokens</span>
                  <span className="trace-view__detail-value">{ev.output_tokens.toLocaleString()}</span>
                </div>
              )}
              {ev.token_usage_source && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Token Source</span>
                  <span className="trace-view__detail-value mono">{ev.token_usage_source}</span>
                </div>
              )}
              {ev.error_code && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Error Code</span>
                  <StatusBadge phase="failed" />
                  <span className="trace-view__detail-value mono text-red">{ev.error_code}</span>
                </div>
              )}
              {ev.error_reason && (
                <div className="trace-view__detail-field trace-view__detail-field--full">
                  <span className="trace-view__detail-label">Error Reason</span>
                  <span className="trace-view__detail-value text-red">{ev.error_reason}</span>
                </div>
              )}
              {ev.retryable != null && (
                <div className="trace-view__detail-field">
                  <span className="trace-view__detail-label">Retryable</span>
                  <span className="trace-view__detail-value">{ev.retryable ? "Yes" : "No"}</span>
                </div>
              )}
              {ev.message && (
                <div className="trace-view__detail-field trace-view__detail-field--full">
                  <span className="trace-view__detail-label">Message</span>
                  <pre className="trace-view__detail-pre">{ev.message}</pre>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="trace-view">
      <div className="trace-view__summary">
        <div className="trace-view__stat">
          <Clock size={14} />
          <span className="trace-view__stat-value">{formatDuration(model.totalMs)}</span>
          <span className="trace-view__stat-label">duration</span>
        </div>
        {model.modelTimeMs > 0 && (
          <div className="trace-view__stat">
            <Cpu size={14} />
            <span className="trace-view__stat-value">{formatDuration(model.modelTimeMs)}</span>
            <span className="trace-view__stat-label">model time</span>
          </div>
        )}
        <div className="trace-view__stat">
          <Zap size={14} />
          <span className="trace-view__stat-value">{model.rows.length}</span>
          <span className="trace-view__stat-label">events</span>
        </div>
        <div className="trace-view__stat">
          <Cpu size={14} />
          <span className="trace-view__stat-value">{formatTokens(model.totalTokens) || "0"}</span>
          <span className="trace-view__stat-label">tokens</span>
        </div>
        {sparkPath && model.totalTokens > 0 && (
          <div className="trace-view__stat trace-view__stat--spark" title="Cumulative tokens over time">
            <svg
              width={SPARK_WIDTH}
              height={SPARK_HEIGHT}
              viewBox={`0 0 ${SPARK_WIDTH} ${SPARK_HEIGHT}`}
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
              aria-hidden
            >
              <defs>
                <linearGradient id="trace-token-spark-fill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="rgba(212, 160, 74, 0.3)" />
                  <stop offset="100%" stopColor="rgba(212, 160, 74, 0)" />
                </linearGradient>
              </defs>
              <path
                d={`${sparkPath} L ${SPARK_WIDTH} ${SPARK_HEIGHT} L 0 ${SPARK_HEIGHT} Z`}
                fill="url(#trace-token-spark-fill)"
              />
              <path
                d={sparkPath}
                stroke="#D4A04A"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
            <span className="trace-view__stat-label">token trend</span>
          </div>
        )}
        <div className="trace-view__stat">
          <Wrench size={14} />
          <span className="trace-view__stat-value">{model.totalToolCalls}</span>
          <span className="trace-view__stat-label">tool calls</span>
        </div>
        {model.errorCount > 0 && (
          <div className="trace-view__stat trace-view__stat--error">
            <AlertTriangle size={14} />
            <span className="trace-view__stat-value">{model.errorCount}</span>
            <span className="trace-view__stat-label">errors</span>
          </div>
        )}
      </div>

      <div className="trace-toolbar">
        <Search size={14} className="text-muted" />
        <input
          className="trace-toolbar__search"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search trace events..."
        />
        {model.agents.length > 1 && (
          <select
            className="trace-view__select"
            value={filterAgent ?? ""}
            onChange={(e) => setFilterAgent(e.target.value || null)}
          >
            <option value="">All agents</option>
            {model.agents.map((a) => <option key={a} value={a}>{a}</option>)}
          </select>
        )}
        {model.branches.length > 1 && (
          <select
            className="trace-view__select"
            value={filterBranch ?? ""}
            onChange={(e) => setFilterBranch(e.target.value || null)}
          >
            <option value="">All branches</option>
            {model.branches.map((b) => <option key={b} value={b}>{b}</option>)}
          </select>
        )}
        <span className="log-viewer__match-count">{filteredRows.length}/{trace.length} events</span>
        <div className="log-viewer__actions">
          <button className="btn-secondary btn-sm" onClick={handleExport}>
            <Download size={13} /> Export JSON
          </button>
        </div>
      </div>

      <div className="trace-view__waterfall">
        <div className="trace-view__header-row">
          <div className="trace-view__col-expand" />
          <div className="trace-view__col-type">Type</div>
          <div className="trace-view__col-detail">Detail</div>
          <div className="trace-view__col-start">Start</div>
          <div className="trace-view__col-latency">Duration</div>
          <div className="trace-view__col-tokens">Tokens</div>
          <div className="trace-view__col-bar">
            <div className="trace-view__axis">
              {axisTicks.map((t) => (
                <span
                  key={t}
                  className={`trace-view__axis-tick${t === 0 ? " trace-view__axis-tick--start" : ""}${t === 1 ? " trace-view__axis-tick--end" : ""}`}
                  style={{ left: `${t * 100}%` }}
                >
                  {t === 0 ? "0" : formatAxisTick(totalMs * t)}
                </span>
              ))}
            </div>
          </div>
        </div>

        {groups.map((group) => {
          const collapsed = collapsedGroups.has(group.key);
          const groupLeft = Math.min((group.startMs / totalMs) * 100, 100);
          const groupWidth = Math.max(((group.endMs - group.startMs) / totalMs) * 100, 0.5);
          const showLanes = hasBranchLanes(group);
          return (
            <div key={group.key} className="trace-view__group">
              <div
                className="trace-view__group-header"
                onClick={() => toggleGroup(group.key)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    toggleGroup(group.key);
                  }
                }}
              >
                <div className="trace-view__col-expand">
                  {collapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
                </div>
                <div className="trace-view__group-title">
                  <span className="mono">{group.agent ?? "task"}</span>
                  <span className="trace-view__group-meta">
                    {group.rows.length} {group.rows.length === 1 ? "event" : "events"}
                    {group.tokens > 0 && ` · ${formatTokens(group.tokens)} tok`}
                    {` · ${formatDuration(group.endMs - group.startMs)}`}
                  </span>
                </div>
                <div className="trace-view__col-bar">
                  <div className="trace-view__bar-track trace-view__bar-track--group">
                    <div
                      className="trace-view__bar-fill trace-view__bar-fill--group"
                      style={{ left: `${groupLeft}%`, width: `${groupWidth}%` }}
                    />
                  </div>
                </div>
              </div>

              {!collapsed && !showLanes && group.rows.map((row) => renderEventRow(row))}

              {!collapsed && showLanes && group.lanes.map((lane) => {
                const laneCollapsed = collapsedLanes.has(lane.key);
                const isDefault = lane.branchId == null;
                const laneLeft = Math.min((lane.startMs / totalMs) * 100, 100);
                const laneWidth = Math.max(((lane.endMs - lane.startMs) / totalMs) * 100, 0.5);

                if (isDefault) {
                  return (
                    <div key={lane.key} className="trace-view__branch">
                      {lane.rows.map((row) => renderEventRow(row, 0))}
                    </div>
                  );
                }

                return (
                  <div key={lane.key} className="trace-view__branch">
                    <div
                      className="trace-view__branch-header"
                      style={{ paddingLeft: 28 + lane.depth * 16 }}
                      onClick={() => toggleLane(lane.key)}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          toggleLane(lane.key);
                        }
                      }}
                    >
                      <div className="trace-view__col-expand">
                        {laneCollapsed ? <ChevronRight size={11} /> : <ChevronDown size={11} />}
                      </div>
                      <div className="trace-view__branch-title">
                        <span className="mono">{lane.branchId}</span>
                        <span className="trace-view__group-meta">
                          {lane.rows.length} {lane.rows.length === 1 ? "event" : "events"}
                          {lane.tokens > 0 && ` · ${formatTokens(lane.tokens)} tok`}
                          {` · ${formatDuration(lane.endMs - lane.startMs)}`}
                        </span>
                      </div>
                      <div className="trace-view__col-bar">
                        <div className="trace-view__bar-track trace-view__bar-track--branch">
                          <div
                            className="trace-view__bar-fill trace-view__bar-fill--branch"
                            style={{ left: `${laneLeft}%`, width: `${laneWidth}%` }}
                          />
                        </div>
                      </div>
                    </div>
                    {!laneCollapsed && lane.rows.map((row) => renderEventRow(row, lane.depth + 1))}
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}
