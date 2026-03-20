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
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { StatusBadge } from "../components/StatusBadge";
import type { Task } from "../api/types";

function countByPhase(tasks: Task[], phase: string): number {
  return tasks.filter((t) => (t.status?.phase ?? "").toLowerCase() === phase.toLowerCase()).length;
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

  const pendingApprovals = useMemo(() =>
    (approvals.data ?? []).filter((a) => (a.status?.phase ?? "Pending").toLowerCase() === "pending").length,
    [approvals.data],
  );

  return (
    <div className="page">
      <div className="page__header">
        <h1 className="page__title">Dashboard</h1>
        <p className="page__subtitle">Orloj control plane overview</p>
      </div>

      <div className="metrics-grid">
        <MetricCard
          label="Agent Systems"
          value={systems.data?.length ?? 0}
          icon={<Network size={16} />}
          variant="blue"
        />
        <MetricCard
          label="Agents"
          value={agents.data?.length ?? 0}
          icon={<Bot size={16} />}
          variant="default"
        />
        <MetricCard
          label="Tasks"
          value={totalTasks}
          icon={<ListTodo size={16} />}
          variant="default"
        />
        <MetricCard
          label="Task Schedules"
          value={taskSchedules.data?.length ?? 0}
          icon={<CalendarClock size={16} />}
          variant="default"
        />
        <MetricCard
          label="Task Webhooks"
          value={taskWebhooks.data?.length ?? 0}
          icon={<Webhook size={16} />}
          variant="default"
        />
        <MetricCard
          label="Workers"
          value={workers.data?.length ?? 0}
          icon={<Cpu size={16} />}
          variant="default"
        />
        <MetricCard
          label="Model Endpoints"
          value={models.data?.length ?? 0}
          icon={<Database size={16} />}
          variant="default"
        />
        <MetricCard
          label="Tools"
          value={tools.data?.length ?? 0}
          icon={<Wrench size={16} />}
          variant="default"
        />
      </div>

      <div className="attention-cards">
        <div className="attention-card" onClick={() => navigate("/workers")}>
          <Activity size={20} className={`attention-card__icon ${health.data ? "text-green" : "text-red"}`} />
          <div className="attention-card__body">
            <div className="attention-card__title">{health.data ? "API Healthy" : "API Unreachable"}</div>
            <div className="attention-card__desc">{workersOnline}/{workerList.length} workers online</div>
          </div>
        </div>
        <div className="attention-card" onClick={() => navigate("/models")}>
          <Database size={20} className="attention-card__icon text-blue" />
          <div className="attention-card__body">
            <div className="attention-card__title">Model Endpoints</div>
            <div className="attention-card__desc">{modelsReady}/{modelList.length} ready</div>
          </div>
        </div>
        {(failed + deadletter) > 0 && (
          <div className="attention-card attention-card--error" onClick={() => navigate("/tasks")}>
            <XCircle size={20} className="attention-card__icon text-red" />
            <div className="attention-card__body">
              <div className="attention-card__title">Task Failures</div>
              <div className="attention-card__desc">{failed} failed, {deadletter} dead-letter</div>
            </div>
            <div className="attention-card__count text-red">{failed + deadletter}</div>
          </div>
        )}
        {pendingApprovals > 0 && (
          <div className="attention-card attention-card--warning" onClick={() => navigate("/approvals")}>
            <ShieldCheck size={20} className="attention-card__icon text-orange" />
            <div className="attention-card__body">
              <div className="attention-card__title">Pending Approvals</div>
              <div className="attention-card__desc">{pendingApprovals} awaiting review</div>
            </div>
            <div className="attention-card__count text-orange">{pendingApprovals}</div>
          </div>
        )}
      </div>

      <div className="dashboard-row">
        <div className="card">
          <h2 className="card__title">Task Health</h2>
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
                <Clock size={12} className="text-yellow" /> Pending: {pending}
              </span>
              <span className="task-health__legend-item">
                <XCircle size={12} className="text-red" /> Failed: {failed}
              </span>
              <span className="task-health__legend-item">
                <AlertTriangle size={12} className="text-orange" /> DeadLetter: {deadletter}
              </span>
            </div>
            <p className={`task-health__pct text-${successColor}`}>{healthyPct}% success rate</p>
          </div>
        </div>

        <div className="card">
          <h2 className="card__title">Recent Tasks</h2>
          <div className="recent-list">
            {taskList.length === 0 && (
              <p className="text-muted">No tasks yet</p>
            )}
            {taskList.slice(0, 8).map((task) => (
              <div
                key={task.metadata.name}
                className="recent-list__item"
                onClick={() => navigate(`/tasks/${task.metadata.name}`)}
              >
                <span className="recent-list__name mono">{task.metadata.name}</span>
                <span className="recent-list__system text-muted">{task.spec.system}</span>
                <StatusBadge phase={task.status?.phase} />
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="dashboard-row">
        <div className="card">
          <h2 className="card__title">Agent Systems</h2>
          <div className="recent-list">
            {(systems.data ?? []).length === 0 && (
              <p className="text-muted">No systems defined</p>
            )}
            {(systems.data ?? []).slice(0, 6).map((sys) => (
              <div
                key={sys.metadata.name}
                className="recent-list__item"
                onClick={() => navigate(`/systems/${sys.metadata.name}`)}
              >
                <span className="recent-list__name mono">{sys.metadata.name}</span>
                <span className="text-muted">{sys.spec.agents?.length ?? 0} agents</span>
                <StatusBadge phase={sys.status?.phase} />
              </div>
            ))}
          </div>
        </div>

        <div className="card">
          <h2 className="card__title">Workers</h2>
          <div className="recent-list">
            {(workers.data ?? []).length === 0 && (
              <p className="text-muted">No workers registered</p>
            )}
            {(workers.data ?? []).slice(0, 6).map((w) => (
              <div
                key={w.metadata.name}
                className="recent-list__item"
                onClick={() => navigate(`/workers/${w.metadata.name}`)}
              >
                <span className="recent-list__name mono">{w.metadata.name}</span>
                <span className="text-muted">
                  {w.status?.currentTasks ?? 0}/{w.spec.max_concurrent_tasks ?? 1} tasks
                </span>
                <StatusBadge phase={w.status?.phase} />
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
