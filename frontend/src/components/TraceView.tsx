import { useState, useMemo } from "react";
import type { TaskTraceEvent } from "../api/types";
import { StatusBadge } from "./StatusBadge";
import { Clock, Cpu, Wrench, AlertTriangle, ChevronDown, ChevronRight, Zap } from "lucide-react";
import clsx from "clsx";

interface TraceViewProps {
  trace: TaskTraceEvent[];
}

const EVENT_TYPE_COLORS: Record<string, { color: string; bg: string; label: string }> = {
  task_start:               { color: "var(--blue)",   bg: "var(--blue-bg)",   label: "Task Start" },
  task_summary:             { color: "var(--blue)",   bg: "var(--blue-bg)",   label: "Task Summary" },
  agent_start:              { color: "var(--green)",  bg: "var(--green-bg)",  label: "Agent Start" },
  agent_end:                { color: "var(--green)",  bg: "var(--green-bg)",  label: "Agent End" },
  agent_event:              { color: "var(--green)",  bg: "var(--green-bg)",  label: "Agent Event" },
  agent_error:              { color: "var(--red)",    bg: "var(--red-bg)",    label: "Agent Error" },
  agent_message:            { color: "var(--purple)", bg: "var(--purple-bg)", label: "Message" },
  agent_message_processed:  { color: "var(--purple)", bg: "var(--purple-bg)", label: "Msg Processed" },
  agent_message_deadletter: { color: "var(--orange)", bg: "var(--orange-bg)", label: "Dead Letter" },
  tool_call:                { color: "var(--yellow)", bg: "var(--yellow-bg)", label: "Tool Call" },
  retry_scheduled:          { color: "var(--orange)", bg: "var(--orange-bg)", label: "Retry" },
  deadletter:               { color: "var(--orange)", bg: "var(--orange-bg)", label: "Dead Letter" },
};

function getEventStyle(type?: string) {
  if (!type) return { color: "var(--text-tertiary)", bg: "var(--gray-bg)", label: type ?? "unknown" };
  return EVENT_TYPE_COLORS[type] ?? { color: "var(--text-tertiary)", bg: "var(--gray-bg)", label: type };
}

function formatDuration(ms?: number): string {
  if (ms == null) return "—";
  if (ms < 1) return "<1ms";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function formatTokens(n?: number): string {
  if (n == null || n === 0) return "";
  if (n < 1000) return `${n}`;
  return `${(n / 1000).toFixed(1)}k`;
}

interface TraceStats {
  totalEvents: number;
  totalLatency: number;
  totalTokens: number;
  totalToolCalls: number;
  errorCount: number;
  agentSet: Set<string>;
}

function computeStats(trace: TaskTraceEvent[]): TraceStats {
  const stats: TraceStats = {
    totalEvents: trace.length,
    totalLatency: 0,
    totalTokens: 0,
    totalToolCalls: 0,
    errorCount: 0,
    agentSet: new Set(),
  };
  for (const ev of trace) {
    if (ev.latency_ms) stats.totalLatency += ev.latency_ms;
    if (ev.tokens) stats.totalTokens += ev.tokens;
    if (ev.tool_calls) stats.totalToolCalls += ev.tool_calls;
    if (ev.error_code || ev.type === "agent_error") stats.errorCount++;
    if (ev.agent) stats.agentSet.add(ev.agent);
  }
  return stats;
}

export function TraceView({ trace }: TraceViewProps) {
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null);
  const [filterAgent, setFilterAgent] = useState<string | null>(null);
  const [filterBranch, setFilterBranch] = useState<string | null>(null);

  const stats = useMemo(() => computeStats(trace), [trace]);

  const agents = useMemo(() => [...stats.agentSet].sort(), [stats.agentSet]);
  const branches = useMemo(() => {
    const set = new Set<string>();
    for (const ev of trace) if (ev.branch_id) set.add(ev.branch_id);
    return [...set].sort();
  }, [trace]);

  const filteredTrace = useMemo(() => {
    return trace.filter((ev) => {
      if (filterAgent && ev.agent !== filterAgent) return false;
      if (filterBranch && ev.branch_id !== filterBranch) return false;
      return true;
    });
  }, [trace, filterAgent, filterBranch]);

  const originTime = useMemo(() => {
    if (trace.length === 0) return 0;
    const ts = trace[0].timestamp;
    return ts ? new Date(ts).getTime() : 0;
  }, [trace]);

  const maxOffset = useMemo(() => {
    if (trace.length === 0) return 1;
    let max = 0;
    for (const ev of trace) {
      const ts = ev.timestamp ? new Date(ev.timestamp).getTime() - originTime : 0;
      const end = ts + (ev.latency_ms ?? 0);
      if (end > max) max = end;
    }
    return max || 1;
  }, [trace, originTime]);

  if (trace.length === 0) {
    return <div className="trace-view trace-view--empty">No trace events recorded</div>;
  }

  return (
    <div className="trace-view">
      <div className="trace-view__summary">
        <div className="trace-view__stat">
          <Zap size={14} />
          <span className="trace-view__stat-value">{stats.totalEvents}</span>
          <span className="trace-view__stat-label">events</span>
        </div>
        <div className="trace-view__stat">
          <Clock size={14} />
          <span className="trace-view__stat-value">{formatDuration(stats.totalLatency)}</span>
          <span className="trace-view__stat-label">total latency</span>
        </div>
        <div className="trace-view__stat">
          <Cpu size={14} />
          <span className="trace-view__stat-value">{formatTokens(stats.totalTokens) || "0"}</span>
          <span className="trace-view__stat-label">tokens</span>
        </div>
        <div className="trace-view__stat">
          <Wrench size={14} />
          <span className="trace-view__stat-value">{stats.totalToolCalls}</span>
          <span className="trace-view__stat-label">tool calls</span>
        </div>
        {stats.errorCount > 0 && (
          <div className="trace-view__stat trace-view__stat--error">
            <AlertTriangle size={14} />
            <span className="trace-view__stat-value">{stats.errorCount}</span>
            <span className="trace-view__stat-label">errors</span>
          </div>
        )}
      </div>

      {(agents.length > 1 || branches.length > 1) && (
        <div className="trace-view__filters">
          {agents.length > 1 && (
            <select
              className="trace-view__select"
              value={filterAgent ?? ""}
              onChange={(e) => setFilterAgent(e.target.value || null)}
            >
              <option value="">All agents</option>
              {agents.map((a) => <option key={a} value={a}>{a}</option>)}
            </select>
          )}
          {branches.length > 1 && (
            <select
              className="trace-view__select"
              value={filterBranch ?? ""}
              onChange={(e) => setFilterBranch(e.target.value || null)}
            >
              <option value="">All branches</option>
              {branches.map((b) => <option key={b} value={b}>{b}</option>)}
            </select>
          )}
        </div>
      )}

      <div className="trace-view__waterfall">
        <div className="trace-view__header-row">
          <div className="trace-view__col-expand" />
          <div className="trace-view__col-type">Type</div>
          <div className="trace-view__col-agent">Agent</div>
          <div className="trace-view__col-detail">Detail</div>
          <div className="trace-view__col-latency">Latency</div>
          <div className="trace-view__col-tokens">Tokens</div>
          <div className="trace-view__col-bar">Timeline</div>
        </div>

        {filteredTrace.map((ev, i) => {
          const style = getEventStyle(ev.type);
          const offset = ev.timestamp ? new Date(ev.timestamp).getTime() - originTime : 0;
          const barLeft = (offset / maxOffset) * 100;
          const barWidth = Math.max(((ev.latency_ms ?? 0) / maxOffset) * 100, 0.5);
          const isExpanded = expandedIdx === i;
          const isError = !!ev.error_code || ev.type === "agent_error";

          return (
            <div key={i}>
              <div
                className={clsx("trace-view__row", isError && "trace-view__row--error", isExpanded && "trace-view__row--expanded")}
                onClick={() => setExpandedIdx(isExpanded ? null : i)}
              >
                <div className="trace-view__col-expand">
                  {isExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                </div>
                <div className="trace-view__col-type">
                  <span className="trace-view__type-badge" style={{ color: style.color, background: style.bg }}>
                    {style.label}
                  </span>
                </div>
                <div className="trace-view__col-agent mono">{ev.agent ?? "—"}</div>
                <div className="trace-view__col-detail text-ellipsis">
                  {ev.tool ?? ev.message ?? ev.step_id ?? ""}
                </div>
                <div className="trace-view__col-latency mono">{formatDuration(ev.latency_ms)}</div>
                <div className="trace-view__col-tokens mono">{formatTokens(ev.tokens)}</div>
                <div className="trace-view__col-bar">
                  <div className="trace-view__bar-track">
                    <div
                      className="trace-view__bar-fill"
                      style={{ left: `${barLeft}%`, width: `${barWidth}%`, background: style.color }}
                    />
                  </div>
                </div>
              </div>

              {isExpanded && (
                <div className="trace-view__detail">
                  <div className="trace-view__detail-grid">
                    {ev.timestamp && (
                      <div className="trace-view__detail-field">
                        <span className="trace-view__detail-label">Timestamp</span>
                        <span className="trace-view__detail-value">{new Date(ev.timestamp).toLocaleString()}</span>
                      </div>
                    )}
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
        })}
      </div>
    </div>
  );
}
