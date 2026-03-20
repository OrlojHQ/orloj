import { useMemo } from "react";
import { MetricCard } from "../components/MetricCard";
import {
  useAgentSystems,
  useAgents,
  useTasks,
  useTaskSchedules,
  useTaskWebhooks,
  useWorkers,
  useModelEndpoints,
  useTools,
  useToolApprovals,
  useHealthCheck,
} from "../api/hooks";
import {
  Network,
  Bot,
  ListTodo,
  CalendarClock,
  Cpu,
  Database,
  Wrench,
  AlertTriangle,
  CheckCircle,
  Clock,
  XCircle,
  Webhook,
  ShieldCheck,
  Activity,
  PauseCircle,
  ChevronRight,
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { StatusBadge } from "../components/StatusBadge";
import type { Task } from "../api/types";

function countByPhase(tasks: Task[], phase: string): number {
  return tasks.filter((t) => (t.status?.phase ?? "").toLowerCase() === phase.toLowerCase()).length;
}

function formatShortTime(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatRelativeUpdated(ts: number): string {
  if (!ts) return "";
  const sec = Math.round((Date.now() - ts) / 1000);
  if (sec < 10) return "Updated just now";
  if (sec < 60) return `Updated ${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `Updated ${min}m ago`;
  const hr = Math.floor(min / 60);
  return `Updated ${hr}h ago`;
}

function DashboardSkeleton() {
  return (
    <div className="dashboard-skeleton" aria-hidden>
      <div className="dashboard-skeleton__hero" />
      <div className="dashboard-skeleton__grid">
        {Array.from({ length: 9 }).map((_, i) => (
          <div key={i} className="dashboard-skeleton__metric" />
        ))}
      </div>
    </div>
  );
}

export function Dashboard() {
  const systems = useAgentSystems();
  const agents = useAgents();
  const tasks = useTasks();
  const taskSchedules = useTaskSchedules();
  const taskWebhooks = useTaskWebhooks();
  const workers = useWorkers();
  const models = useModelEndpoints();
  const tools = useTools();
  const approvals = useToolApprovals();
  const health = useHealthCheck();
  const navigate = useNavigate();

  const taskList = tasks.data ?? [];
  const running = countByPhase(taskList, "Running");
  const succeeded = countByPhase(taskList, "Succeeded");
  const failed = countByPhase(taskList, "Failed");
  const deadletter = countByPhase(taskList, "DeadLetter");
  const pending = countByPhase(taskList, "Pending");

  const totalTasks = taskList.length;
  const healthyPct = totalTasks > 0 ? Math.round((succeeded / totalTasks) * 100) : 0;
  const successColor = healthyPct >= 95 ? "green" : healthyPct >= 80 ? "yellow" : "red";

  const workerList = workers.data ?? [];
  const workersOnline = workerList.filter((w) => {
    const p = (w.status?.phase ?? "").toLowerCase();
    return p === "healthy" || p === "ready";
  }).length;

  const modelList = models.data ?? [];
  const modelsReady = modelList.filter((m) => (m.status?.phase ?? "").toLowerCase() === "ready").length;

  const pendingApprovals = useMemo(
    () =>
      (approvals.data ?? []).filter((a) => (a.status?.phase ?? "Pending").toLowerCase() === "pending").length,
    [approvals.data],
  );

  const taskIssues = failed + deadletter;
  const apiOk = health.data === true;
  const workersOk = workerList.length === 0 || workersOnline === workerList.length;
  const modelsOk = modelList.length === 0 || modelsReady === modelList.length;
  const platformTone =
    !apiOk ? "bad" : taskIssues > 0 || pendingApprovals > 0 || !workersOk || !modelsOk ? "warn" : "ok";

  const lastUpdated = useMemo(() => {
    const stamps = [
      systems.dataUpdatedAt,
      tasks.dataUpdatedAt,
      workers.dataUpdatedAt,
      models.dataUpdatedAt,
      health.dataUpdatedAt,
    ].filter(Boolean) as number[];
    return stamps.length ? Math.max(...stamps) : 0;
  }, [
    systems.dataUpdatedAt,
    tasks.dataUpdatedAt,
    workers.dataUpdatedAt,
    models.dataUpdatedAt,
    health.dataUpdatedAt,
  ]);

  const showSkeleton = tasks.isPending || systems.isPending || workers.isPending;

  if (showSkeleton) {
    return (
      <div className="page page--dashboard">
        <div className="page__header">
          <h1 className="page__title">Dashboard</h1>
          <p className="page__subtitle">Orloj control plane overview</p>
        </div>
        <DashboardSkeleton />
      </div>
    );
  }

  return (
    <div className="page page--dashboard">
      <div className="page__header">
        <div>
          <h1 className="page__title">Dashboard</h1>
          <p className="page__subtitle">Orloj control plane overview</p>
          {lastUpdated > 0 && (
            <p className="dashboard-meta text-muted">{formatRelativeUpdated(lastUpdated)} · auto-refreshed</p>
          )}
        </div>
      </div>

      <section
        className={`platform-status platform-status--${platformTone}`}
        aria-label="Platform status"
      >
        <div className="platform-status__main">
          <div className="platform-status__badge">
            <Activity size={18} className="platform-status__badge-icon" />
            <span className="platform-status__badge-label">
              {platformTone === "ok" && "All systems operational"}
              {platformTone === "warn" && "Attention needed"}
              {platformTone === "bad" && "API unavailable"}
            </span>
          </div>
          <ul className="platform-status__list">
            <li>
              <span className={apiOk ? "text-green" : "text-red"}>{apiOk ? "API reachable" : "API unreachable"}</span>
            </li>
            <li>
              <span className={workersOk ? "text-muted" : "text-orange"}>
                {workerList.length === 0
                  ? "Workers: none registered"
                  : `Workers ${workersOnline}/${workerList.length} online`}
              </span>
            </li>
            <li>
              <span className={modelsOk ? "text-muted" : "text-orange"}>
                {modelList.length === 0
                  ? "Models: none configured"
                  : `Models ${modelsReady}/${modelList.length} ready`}
              </span>
            </li>
          </ul>
        </div>
        <div className="platform-status__actions">
          {taskIssues > 0 && (
            <button
              type="button"
              className="platform-status__chip platform-status__chip--danger"
              onClick={() => navigate("/tasks")}
            >
              <XCircle size={14} />
              {taskIssues} task{taskIssues === 1 ? "" : "s"} need review
              <ChevronRight size={14} />
            </button>
          )}
          {pendingApprovals > 0 && (
            <button
              type="button"
              className="platform-status__chip platform-status__chip--warning"
              onClick={() => navigate("/approvals")}
            >
              <ShieldCheck size={14} />
              {pendingApprovals} approval{pendingApprovals === 1 ? "" : "s"} pending
              <ChevronRight size={14} />
            </button>
          )}
          <button type="button" className="platform-status__chip platform-status__chip--ghost" onClick={() => navigate("/workers")}>
            Workers
            <ChevronRight size={14} />
          </button>
          <button type="button" className="platform-status__chip platform-status__chip--ghost" onClick={() => navigate("/models")}>
            Model endpoints
            <ChevronRight size={14} />
          </button>
        </div>
      </section>

      <div className="dashboard-metrics">
        <section className="dashboard-section" aria-labelledby="dash-runtime">
          <h2 id="dash-runtime" className="dashboard-section__title">
            Runtime
          </h2>
          <p className="dashboard-section__hint">Live workload and execution surface</p>
          <div className="metrics-grid metrics-grid--dashboard">
            <MetricCard
              label="Tasks"
              value={totalTasks}
              subtitle={`${running} running · ${pending} pending`}
              icon={<ListTodo size={16} />}
              variant={taskIssues > 0 ? "red" : "default"}
            />
            <MetricCard
              label="Workers"
              value={workerList.length}
              subtitle={`${workersOnline} online`}
              icon={<Cpu size={16} />}
              variant={!workersOk && workerList.length > 0 ? "orange" : "default"}
            />
            <MetricCard
              label="Pending approvals"
              value={pendingApprovals}
              subtitle={pendingApprovals > 0 ? "Requires review" : "Queue clear"}
              icon={<ShieldCheck size={16} />}
              variant={pendingApprovals > 0 ? "orange" : "default"}
            />
          </div>
        </section>

        <section className="dashboard-section" aria-labelledby="dash-definitions">
          <h2 id="dash-definitions" className="dashboard-section__title">
            Definitions
          </h2>
          <p className="dashboard-section__hint">Agents, systems, models, and tools in this namespace</p>
          <div className="metrics-grid metrics-grid--dashboard">
            <MetricCard
              label="Agent systems"
              value={systems.data?.length ?? 0}
              icon={<Network size={16} />}
              variant="default"
            />
            <MetricCard label="Agents" value={agents.data?.length ?? 0} icon={<Bot size={16} />} variant="default" />
            <MetricCard
              label="Model endpoints"
              value={modelList.length}
              subtitle={`${modelsReady} ready`}
              icon={<Database size={16} />}
              variant={!modelsOk && modelList.length > 0 ? "orange" : "default"}
            />
            <MetricCard label="Tools" value={tools.data?.length ?? 0} icon={<Wrench size={16} />} variant="default" />
          </div>
        </section>

        <section className="dashboard-section" aria-labelledby="dash-automation">
          <h2 id="dash-automation" className="dashboard-section__title">
            Automation
          </h2>
          <p className="dashboard-section__hint">Schedules and webhooks</p>
          <div className="metrics-grid metrics-grid--dashboard">
            <MetricCard
              label="Task schedules"
              value={taskSchedules.data?.length ?? 0}
              icon={<CalendarClock size={16} />}
              variant="default"
            />
            <MetricCard
              label="Task webhooks"
              value={taskWebhooks.data?.length ?? 0}
              icon={<Webhook size={16} />}
              variant="default"
            />
          </div>
        </section>
      </div>

      <div className="dashboard-row">
        <div className="card card--dashboard">
          <h2 className="card__title card__title--sentence">Task health</h2>
          <div className="task-health">
            <div className="task-health__bar">
              {succeeded > 0 && (
                <div
                  className="task-health__segment task-health__segment--green"
                  style={{ width: `${(succeeded / Math.max(totalTasks, 1)) * 100}%` }}
                  title={`Succeeded: ${succeeded}`}
                />
              )}
              {running > 0 && (
                <div
                  className="task-health__segment task-health__segment--blue"
                  style={{ width: `${(running / Math.max(totalTasks, 1)) * 100}%` }}
                  title={`Running: ${running}`}
                />
              )}
              {pending > 0 && (
                <div
                  className="task-health__segment task-health__segment--yellow"
                  style={{ width: `${(pending / Math.max(totalTasks, 1)) * 100}%` }}
                  title={`Pending: ${pending}`}
                />
              )}
              {failed > 0 && (
                <div
                  className="task-health__segment task-health__segment--red"
                  style={{ width: `${(failed / Math.max(totalTasks, 1)) * 100}%` }}
                  title={`Failed: ${failed}`}
                />
              )}
              {deadletter > 0 && (
                <div
                  className="task-health__segment task-health__segment--orange"
                  style={{ width: `${(deadletter / Math.max(totalTasks, 1)) * 100}%` }}
                  title={`DeadLetter: ${deadletter}`}
                />
              )}
            </div>
            <div className="task-health__legend">
              <span className="task-health__legend-item">
                <CheckCircle size={12} className="text-green" /> Succeeded: {succeeded}
              </span>
              <span className="task-health__legend-item">
                <Clock size={12} className="text-blue" /> Running: {running}
              </span>
              <span className="task-health__legend-item">
                <PauseCircle size={12} className="text-yellow" /> Pending: {pending}
              </span>
              <span className="task-health__legend-item">
                <XCircle size={12} className="text-red" /> Failed: {failed}
              </span>
              <span className="task-health__legend-item">
                <AlertTriangle size={12} className="text-orange" /> Dead letter: {deadletter}
              </span>
            </div>
            <p className={`task-health__pct text-${successColor}`}>
              {healthyPct}% success rate
              <span className="task-health__pct-meta text-muted"> · {totalTasks} total tasks</span>
            </p>
          </div>
        </div>

        <div className="card card--dashboard">
          <div className="card__header-row">
            <h2 className="card__title card__title--sentence">Recent tasks</h2>
            <button type="button" className="card__link-all" onClick={() => navigate("/tasks")}>
              View all
              <ChevronRight size={14} />
            </button>
          </div>
          <div className="recent-list recent-list--grid recent-list--tasks">
            <div className="recent-list__header">
              <span>Task</span>
              <span>System</span>
              <span>Updated</span>
              <span className="recent-list__header-phase">Phase</span>
            </div>
            {taskList.length === 0 && <p className="text-muted recent-list__empty">No tasks yet</p>}
            {taskList.slice(0, 8).map((task) => (
              <div
                key={task.metadata.name}
                className="recent-list__item"
                onClick={() => navigate(`/tasks/${task.metadata.name}`)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    navigate(`/tasks/${task.metadata.name}`);
                  }
                }}
              >
                <span className="recent-list__name mono">{task.metadata.name}</span>
                <span className="recent-list__system text-muted">{task.spec.system ?? "—"}</span>
                <span className="recent-list__time text-muted">
                  {formatShortTime(task.status?.completedAt ?? task.status?.startedAt)}
                </span>
                <span className="recent-list__badge">
                  <StatusBadge phase={task.status?.phase} />
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="dashboard-row">
        <div className="card card--dashboard">
          <div className="card__header-row">
            <h2 className="card__title card__title--sentence">Agent systems</h2>
            <button type="button" className="card__link-all" onClick={() => navigate("/systems")}>
              View all
              <ChevronRight size={14} />
            </button>
          </div>
          <div className="recent-list recent-list--grid recent-list--systems">
            <div className="recent-list__header">
              <span>System</span>
              <span>Agents</span>
              <span className="recent-list__header-phase">Status</span>
            </div>
            {(systems.data ?? []).length === 0 && (
              <p className="text-muted recent-list__empty">No systems defined</p>
            )}
            {(systems.data ?? []).slice(0, 6).map((sys) => (
              <div
                key={sys.metadata.name}
                className="recent-list__item"
                onClick={() => navigate(`/systems/${sys.metadata.name}`)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    navigate(`/systems/${sys.metadata.name}`);
                  }
                }}
              >
                <span className="recent-list__name mono">{sys.metadata.name}</span>
                <span className="text-muted">{sys.spec.agents?.length ?? 0}</span>
                <span className="recent-list__badge">
                  <StatusBadge phase={sys.status?.phase} />
                </span>
              </div>
            ))}
          </div>
        </div>

        <div className="card card--dashboard">
          <div className="card__header-row">
            <h2 className="card__title card__title--sentence">Workers</h2>
            <button type="button" className="card__link-all" onClick={() => navigate("/workers")}>
              View all
              <ChevronRight size={14} />
            </button>
          </div>
          <div className="recent-list recent-list--grid recent-list--workers">
            <div className="recent-list__header">
              <span>Worker</span>
              <span>Load</span>
              <span className="recent-list__header-phase">Status</span>
            </div>
            {(workers.data ?? []).length === 0 && (
              <p className="text-muted recent-list__empty">No workers registered</p>
            )}
            {(workers.data ?? []).slice(0, 6).map((w) => (
              <div
                key={`${w.metadata.namespace ?? "default"}/${w.metadata.name}`}
                className="recent-list__item"
                onClick={() => navigate(`/workers/${w.metadata.name}`)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    navigate(`/workers/${w.metadata.name}`);
                  }
                }}
              >
                <span className="recent-list__name mono">{w.metadata.name}</span>
                <span className="text-muted">
                  {w.status?.currentTasks ?? 0}/{w.spec.max_concurrent_tasks ?? 1}
                </span>
                <span className="recent-list__badge">
                  <StatusBadge phase={w.status?.phase} />
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
