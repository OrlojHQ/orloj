import { useState, useCallback, useEffect, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useTask, useTaskMessages, useTaskMetrics, useTaskLogs, useAgentSystem, useDeleteResource, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { toast } from "../components/Toast";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { LogViewer } from "../components/LogViewer";
import { GraphView } from "../components/GraphView";
import { MetricCard } from "../components/MetricCard";
import { TraceView } from "../components/TraceView";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft, Clock, Activity, Hash, Zap } from "lucide-react";
import clsx from "clsx";
import type { Task, TaskTraceEvent } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

function traceKey(e: TaskTraceEvent): string {
  return `${e.timestamp}|${e.type}|${e.step ?? ""}|${e.tool ?? ""}`;
}

function mergeTrace(persisted: TaskTraceEvent[], streamed: TaskTraceEvent[]): TaskTraceEvent[] {
  if (streamed.length === 0) return persisted;
  const seen = new Set(persisted.map(traceKey));
  const extras = streamed.filter((e) => !seen.has(traceKey(e)));
  if (extras.length === 0) return persisted;
  return [...persisted, ...extras];
}

function useTaskTraceStream(taskName: string, namespace: string, apiBase: string): TaskTraceEvent[] {
  const [streamed, setStreamed] = useState<TaskTraceEvent[]>([]);
  const seenRef = useRef(new Set<string>());

  useEffect(() => {
    if (!taskName) return;
    setStreamed([]);
    seenRef.current = new Set();

    const base = apiBase.replace(/\/$/, "");
    const url = new URL("/v1/events/watch", base);
    url.searchParams.set("type", "task.trace");
    url.searchParams.set("name", taskName);
    url.searchParams.set("namespace", namespace);

    const es = new EventSource(url.toString());
    const handle = (e: MessageEvent) => {
      try {
        const payload = JSON.parse(e.data);
        const d = payload.data;
        if (!d) return;
        const evt: TaskTraceEvent = {
          timestamp: d.Timestamp,
          type: d.Type,
          step: d.Step,
          tool: d.Tool,
          message: d.Message,
          error_code: d.ErrorCode,
          error_reason: d.ErrorReason,
          retryable: d.Retryable ?? undefined,
          latency_ms: d.LatencyMS,
          tokens: d.Tokens,
          input_tokens: d.InputTokens,
          output_tokens: d.OutputTokens,
          token_usage_source: d.UsageSource,
        };
        const key = `${evt.timestamp}|${evt.type}|${evt.step}|${evt.tool}`;
        if (seenRef.current.has(key)) return;
        seenRef.current.add(key);
        setStreamed((prev) => [...prev, evt]);
      } catch { /* ignore */ }
    };
    es.addEventListener("event", handle);
    es.onmessage = handle;
    return () => { es.close(); };
  }, [taskName, namespace, apiBase]);

  return streamed;
}

type Tab = "overview" | "messages" | "metrics" | "trace" | "logs" | "graph" | "yaml";

const TOOLTIP_AVG_LATENCY_MS =
  "Average end-to-end time from each message's timestamp to when it was processed (milliseconds). Only messages that have both times are included; queue wait is part of this duration.";

const TOOLTIP_P95_LATENCY_MS =
  "The 95th percentile of those same end-to-end latencies: about 95% of measured messages finished within this time or faster (the slowest few are above it).";

export function TaskDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/tasks");
  const routeName = nameParam ?? "";
  const { data: task, isLoading, isError, error } = useTask(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const apiBase = useAppStore((s) => s.apiBase);
  const streamedTrace = useTaskTraceStream(routeName, namespace, apiBase);
  const [msgPhase, setMsgPhase] = useState("");
  const [msgFrom, setMsgFrom] = useState("");
  const [msgTo, setMsgTo] = useState("");
  const [msgBranch, setMsgBranch] = useState("");
  const [msgTrace, setMsgTrace] = useState("");
  const [msgLimit, setMsgLimit] = useState("");
  const [msgFiltersApplied, setMsgFiltersApplied] = useState<Record<string, string>>({});

  const buildMessageFilters = useCallback(() => {
    const q: Record<string, string> = {};
    if (msgPhase.trim()) q.phase = msgPhase.trim();
    if (msgFrom.trim()) q.from_agent = msgFrom.trim();
    if (msgTo.trim()) q.to_agent = msgTo.trim();
    if (msgBranch.trim()) q.branch_id = msgBranch.trim();
    if (msgTrace.trim()) q.trace_id = msgTrace.trim();
    if (msgLimit.trim()) q.limit = msgLimit.trim();
    return q;
  }, [msgPhase, msgFrom, msgTo, msgBranch, msgTrace, msgLimit]);

  const messages = useTaskMessages(
    routeName,
    Object.keys(msgFiltersApplied).length > 0 ? msgFiltersApplied : undefined,
  );
  const metrics = useTaskMetrics(routeName);
  const logs = useTaskLogs(routeName);
  const system = useAgentSystem(task?.spec.system ?? "");
  const deleteMutation = useDeleteResource("Task");
  const updateMutation = useUpdateResource("Task");
  const [tab, setTab] = useState<Tab>("overview");

  const handleDelete = async () => {
    if (!task || !window.confirm(`Delete Task ${task.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "Task deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Task");
    }
  };

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Task"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !task) {
    return <div className="page"><div className="loading-placeholder">Loading task...</div></div>;
  }

  const persistedTrace = task.status?.trace ?? [];
  const traceEvents = mergeTrace(persistedTrace, streamedTrace);

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "messages", label: `Messages (${messages.data?.length ?? 0})` },
    { id: "metrics", label: "Metrics" },
    { id: "trace", label: `Trace (${traceEvents.length})` },
    { id: "logs", label: "Logs" },
    { id: "graph", label: "Graph" },
    { id: "yaml", label: "YAML" },
  ];

  const m = metrics.data?.totals;

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{task.metadata.name}</h1>
            <p className="page__subtitle">{task.spec.system} &middot; {task.metadata.namespace}</p>
          </div>
          <StatusBadge
            phase={task.metadata?.labels?.["orloj.dev/a2a-cancelled"] === "true" ? "Cancelled (A2A)" : task.status?.phase}
            size="md"
            pulse={task.status?.phase === "Running"}
          />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Task"}
        </button>
      </div>

      <div className="tab-bar" role="tablist">
        {tabs.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            aria-controls={`tabpanel-${t.id}`}
            className={clsx("tab-bar__tab", tab === t.id && "tab-bar__tab--active")}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="tab-content" role="tabpanel" id={`tabpanel-${tab}`} aria-labelledby={tab}>
        {tab === "overview" && (
          <>
            {task.status?.blocked_on?.name && (
              <div className="card card--mb">
                <div className="detail-field">
                  <span className="detail-field__label">Blocked On</span>
                  <button
                    type="button"
                    className="detail-field__value detail-field__link btn-link"
                    onClick={() => navigate((task.status?.blocked_on?.kind ?? "").toLowerCase() === "taskapproval" ? `/approvals/task/${task.status?.blocked_on?.name}` : `/approvals/${task.status?.blocked_on?.name}`)}
                  >
                    {task.status?.blocked_on?.kind ?? "Approval"} &middot; {task.status?.blocked_on?.name}
                  </button>
                </div>
              </div>
            )}
            {(() => {
              const labels = task.metadata.labels ?? {};
              const a2aTaskId = labels["orloj.dev/a2a-task-id"];
              const a2aContextId = labels["orloj.dev/a2a-context-id"];
              const a2aClient = labels["orloj.dev/a2a-client"];
              const a2aCancelled = labels["orloj.dev/a2a-cancelled"] === "true";
              const hasA2A = a2aTaskId || a2aContextId || a2aClient || a2aCancelled;
              if (!hasA2A) return null;
              return (
                <div className="card card--mb">
                  <div className="detail-grid">
                    {a2aCancelled && (
                      <div className="detail-field">
                        <span className="detail-field__label">A2A Status</span>
                        <StatusBadge phase="Cancelled (A2A)" />
                      </div>
                    )}
                    {a2aTaskId && (
                      <div className="detail-field">
                        <span className="detail-field__label">A2A Task ID</span>
                        <span className="detail-field__value mono">{a2aTaskId}</span>
                      </div>
                    )}
                    {a2aContextId && (
                      <div className="detail-field">
                        <span className="detail-field__label">A2A Context</span>
                        <span className="detail-field__value mono">{a2aContextId}</span>
                      </div>
                    )}
                    {a2aClient && (
                      <div className="detail-field">
                        <span className="detail-field__label">A2A Client</span>
                        <span className="detail-field__value mono">{a2aClient}</span>
                      </div>
                    )}
                  </div>
                </div>
              );
            })()}
            <div className="detail-grid">
            <div className="detail-field">
              <span className="detail-field__label">Phase</span>
              <StatusBadge phase={task.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">System</span>
              <button
                type="button"
                className="detail-field__value detail-field__link btn-link"
                onClick={() => navigate(`/systems/${task.spec.system}`)}
              >
                {task.spec.system}
              </button>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Priority</span>
              <span className="detail-field__value">{task.spec.priority ?? "normal"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Attempts</span>
              <span className="detail-field__value">{task.status?.attempts ?? 0}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Assigned Worker</span>
              <span className="detail-field__value mono">{task.status?.assignedWorker ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Started At</span>
              <span className="detail-field__value">{task.status?.startedAt ? new Date(task.status.startedAt).toLocaleString() : "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Completed At</span>
              <span className="detail-field__value">{task.status?.completedAt ? new Date(task.status.completedAt).toLocaleString() : "—"}</span>
            </div>
            {task.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{task.status.lastError}</span>
              </div>
            )}
            {task.spec.input && Object.keys(task.spec.input).length > 0 && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Input</span>
                <pre className="detail-field__pre">{JSON.stringify(task.spec.input, null, 2)}</pre>
              </div>
            )}
            {task.status?.output && Object.keys(task.status.output).length > 0 && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Output</span>
                <pre className="detail-field__pre">{JSON.stringify(task.status.output, null, 2)}</pre>
              </div>
            )}
            </div>
          </>
        )}

        {tab === "messages" && (
          <div className="messages-list">
            <div className="message-filters">
              <div className="message-filters__field">
                <label htmlFor="msg-filter-phase">phase</label>
                <input
                  id="msg-filter-phase"
                  placeholder="queued,running,…"
                  value={msgPhase}
                  onChange={(e) => setMsgPhase(e.target.value)}
                />
              </div>
              <div className="message-filters__field">
                <label htmlFor="msg-filter-from">from_agent</label>
                <input id="msg-filter-from" value={msgFrom} onChange={(e) => setMsgFrom(e.target.value)} />
              </div>
              <div className="message-filters__field">
                <label htmlFor="msg-filter-to">to_agent</label>
                <input id="msg-filter-to" value={msgTo} onChange={(e) => setMsgTo(e.target.value)} />
              </div>
              <div className="message-filters__field">
                <label htmlFor="msg-filter-branch">branch_id</label>
                <input id="msg-filter-branch" value={msgBranch} onChange={(e) => setMsgBranch(e.target.value)} />
              </div>
              <div className="message-filters__field">
                <label htmlFor="msg-filter-trace">trace_id</label>
                <input id="msg-filter-trace" value={msgTrace} onChange={(e) => setMsgTrace(e.target.value)} />
              </div>
              <div className="message-filters__field">
                <label htmlFor="msg-filter-limit">limit</label>
                <input id="msg-filter-limit" type="number" min={0} placeholder="50" value={msgLimit} onChange={(e) => setMsgLimit(e.target.value)} />
              </div>
              <button type="button" className="btn-primary" onClick={() => setMsgFiltersApplied(buildMessageFilters())}>
                Apply filters
              </button>
              <button
                type="button"
                className="btn-secondary"
                onClick={() => {
                  setMsgPhase("");
                  setMsgFrom("");
                  setMsgTo("");
                  setMsgBranch("");
                  setMsgTrace("");
                  setMsgLimit("");
                  setMsgFiltersApplied({});
                }}
              >
                Clear
              </button>
            </div>
            {messages.isLoading && <p className="text-muted">Loading messages…</p>}
            {messages.isError && (
              <p className="text-red">
                {messages.error instanceof Error ? messages.error.message : "Failed to load messages"}
              </p>
            )}
            {(messages.data ?? []).length === 0 && !messages.isLoading && !messages.isError && (
              <p className="text-muted">
                {Object.keys(msgFiltersApplied).length > 0
                  ? "No messages match the current filters"
                  : "No messages yet"}
              </p>
            )}
            {(messages.data ?? []).map((msg, i) => (
              <div key={msg.message_id ?? i} className="message-item">
                <div className="message-item__header">
                  <span className="mono">{msg.from_agent ?? "system"}</span>
                  <span className="text-muted">&rarr;</span>
                  <span className="mono">{msg.to_agent}</span>
                  <StatusBadge phase={msg.phase} />
                  {msg.timestamp && (
                    <span className="text-muted text-xs">{new Date(msg.timestamp).toLocaleString()}</span>
                  )}
                </div>
                {msg.content && <pre className="message-item__content">{msg.content}</pre>}
                {msg.last_error && <p className="text-red text-xs">{msg.last_error}</p>}
              </div>
            ))}
          </div>
        )}

        {tab === "metrics" && metrics.isLoading && (
          <div className="loading-placeholder">Loading metrics...</div>
        )}
        {tab === "metrics" && !metrics.isLoading && m && (
          <div>
            <div className="metrics-grid">
              <MetricCard label="Total Messages" value={m.messages} icon={<Hash size={16} />} />
              <MetricCard label="In Flight" value={m.in_flight} icon={<Activity size={16} />} variant="blue" />
              <MetricCard label="Succeeded" value={m.succeeded} icon={<Zap size={16} />} variant="green" />
              <MetricCard label="DeadLetters" value={m.deadletters} variant="orange" />
              <MetricCard label="Retries" value={m.retry_count} variant="yellow" />
              <MetricCard
                label="Avg Latency"
                value={`${m.latency_ms_avg}ms`}
                icon={<Clock size={16} />}
                hint={TOOLTIP_AVG_LATENCY_MS}
              />
              <MetricCard
                label="P95 Latency"
                value={`${m.latency_ms_p95}ms`}
                icon={<Clock size={16} />}
                variant="blue"
                hint={TOOLTIP_P95_LATENCY_MS}
              />
            </div>
          </div>
        )}
        {tab === "metrics" && !metrics.isLoading && !m && (
          metrics.isError ? (
            <p className="text-red">
              {metrics.error instanceof Error ? metrics.error.message : "Failed to load metrics"}
            </p>
          ) : (
            <p className="text-muted">No metrics available</p>
          )
        )}

        {tab === "trace" && <TraceView trace={traceEvents} />}

        {tab === "logs" && logs.isError && (
          <p className="text-red">
            {logs.error instanceof Error ? logs.error.message : "Failed to load logs"}
          </p>
        )}
        {tab === "logs" && <LogViewer logs={logs.data ?? ""} loading={logs.isLoading} />}

        {tab === "graph" && system.data && (
          <GraphView system={system.data} animated={task.status?.phase === "Running"} />
        )}
        {tab === "graph" && !system.data && <p className="text-muted">System not found</p>}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(task, null, 2)}
            editable
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<Task>(
                queryClient,
                "Task",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<Task>,
              );
              toast("success", "Task updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.Task}/${encodeURIComponent(updated.metadata.name)}`,
                  { replace: true },
                );
              }
            }}
          />
        )}
      </div>
    </div>
  );
}
