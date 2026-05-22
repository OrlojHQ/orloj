import { useMemo } from "react";
import {
  useAgentSystems,
  useAgents,
  useTasks,
  useTaskSchedules,
  useTaskWebhooks,
  useWorkers,
  useModelEndpoints,
  useTools,
  useHealthCheck,
  useSecrets,
  useMemories,
  useMcpServers,
  useCapabilities,
} from "../api/hooks";
import {
  Network,
  Bot,
  CalendarClock,
  Cpu,
  Database,
  Wrench,
  Webhook,
  ChevronRight,
  Play,
  Lock,
  BrainCircuit,
  Server,
  Zap,
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { StatusBadge } from "../components/StatusBadge";
import { SystemHealthHorizon } from "../components/SystemHealthHorizon";

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

function DashboardSkeleton() {
  return (
    <div className="dashboard-skeleton" aria-hidden>
      <div className="dashboard-skeleton__hero" />
      <div className="dashboard-skeleton__grid">
        {Array.from({ length: 6 }).map((_, i) => (
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
  const secrets = useSecrets();
  const memories = useMemories();
  const mcpServers = useMcpServers();
  const capabilities = useCapabilities();
  const health = useHealthCheck();
  const navigate = useNavigate();

  const taskList = tasks.data ?? [];
  const workerList = workers.data ?? [];
  const modelList = models.data ?? [];
  const apiOk = health.data === true;

  const activeTasks = useMemo(() => {
    return [...taskList].sort((a, b) => {
      const phaseOrder = (p?: string) => {
        const lp = (p ?? "").toLowerCase();
        if (lp === "running") return 0;
        if (lp === "pending") return 1;
        return 2;
      };
      const diff = phaseOrder(a.status?.phase) - phaseOrder(b.status?.phase);
      if (diff !== 0) return diff;
      const aT = a.status?.startedAt ?? a.metadata.createdAt ?? "";
      const bT = b.status?.startedAt ?? b.metadata.createdAt ?? "";
      return bT.localeCompare(aT);
    });
  }, [taskList]);

  const capCount = capabilities.data?.capabilities?.length ?? 0;

  const showSkeleton =
    tasks.isPending || systems.isPending || workers.isPending;

  if (showSkeleton) {
    return (
      <div className="page page--dashboard">
        <DashboardSkeleton />
      </div>
    );
  }

  return (
    <div className="page page--dashboard">
      {/* System Health Rollup — reuses the same component as the system detail page */}
      <section aria-label="System Health Rollup">
        <h2 className="dash-section-heading">System Health Rollup</h2>
        <SystemHealthHorizon
          tasks={taskList}
          apiReachable={apiOk}
          workers={workerList}
        />
      </section>

      {/* Main content: Active Tasks + Bento Sidebar */}
      <div className="dash-body">
        <section className="dash-tasks" aria-label="Active Tasks">
          <h2 className="dash-section-heading">Active Tasks</h2>
          <div className="dash-tasks__table">
            <div className="dash-tasks__header">
              <span>TASK</span>
              <span>SYSTEM</span>
              <span>UPDATED</span>
              <span>PHASE</span>
            </div>
            {activeTasks.length === 0 && (
              <p className="dash-tasks__empty text-muted">No tasks yet</p>
            )}
            {activeTasks.slice(0, 15).map((task) => (
              <div
                key={task.metadata.name}
                className="dash-tasks__row"
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
                <span className="dash-tasks__name">
                  <Play size={12} className="dash-tasks__play-icon" />
                  <span className="mono">{task.metadata.name}</span>
                </span>
                <span className="dash-tasks__system text-muted">
                  <Network size={12} className="dash-tasks__system-icon" />
                  {task.spec.system ?? "—"}
                </span>
                <span className="dash-tasks__time text-muted">
                  {formatShortTime(
                    task.status?.completedAt ??
                      task.status?.startedAt ??
                      task.metadata.createdAt,
                  )}
                </span>
                <span className="dash-tasks__phase">
                  <StatusBadge phase={task.status?.phase} />
                  <ChevronRight size={14} className="dash-tasks__chevron" />
                </span>
              </div>
            ))}
            {activeTasks.length > 15 && (
              <div className="dash-tasks__footer">
                <button
                  type="button"
                  className="dash-tasks__view-all"
                  onClick={() => navigate("/tasks")}
                >
                  View all {activeTasks.length} tasks <ChevronRight size={14} />
                </button>
              </div>
            )}
          </div>
        </section>

        <aside className="dash-sidebar" aria-label="Bento Sidebar">
          <div className="dash-sidebar__group">
            <h3 className="dash-sidebar__group-title">System Definitions</h3>
            <div className="dash-sidebar__items">
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/systems")}
                role="button"
                tabIndex={0}
              >
                <Network size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Agent Systems</span>
                <span className="dash-sidebar__item-count">
                  {systems.data?.length ?? 0}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/agents")}
                role="button"
                tabIndex={0}
              >
                <Bot size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Agents</span>
                <span className="dash-sidebar__item-count">
                  {agents.data?.length ?? 0}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/task-schedules")}
                role="button"
                tabIndex={0}
              >
                <CalendarClock size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Schedules</span>
                <span className="dash-sidebar__item-count">
                  {taskSchedules.data?.length ?? 0}
                </span>
              </div>
            </div>
          </div>

          <div className="dash-sidebar__group">
            <h3 className="dash-sidebar__group-title">Infrastructure</h3>
            <div className="dash-sidebar__items">
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/workers")}
                role="button"
                tabIndex={0}
              >
                <Cpu size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Workers</span>
                <span className="dash-sidebar__item-count">
                  {workerList.length}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/models")}
                role="button"
                tabIndex={0}
              >
                <Database size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">
                  Model Endpoints
                </span>
                <span className="dash-sidebar__item-count">
                  {modelList.length}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/tools")}
                role="button"
                tabIndex={0}
              >
                <Wrench size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Tools</span>
                <span className="dash-sidebar__item-count">
                  {tools.data?.length ?? 0}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/mcp-servers")}
                role="button"
                tabIndex={0}
              >
                <Server size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">MCP Servers</span>
                <span className="dash-sidebar__item-count">
                  {mcpServers.data?.length ?? 0}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/memories")}
                role="button"
                tabIndex={0}
              >
                <BrainCircuit size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Memories</span>
                <span className="dash-sidebar__item-count">
                  {memories.data?.length ?? 0}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/secrets")}
                role="button"
                tabIndex={0}
              >
                <Lock size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Secrets</span>
                <span className="dash-sidebar__item-count">
                  {secrets.data?.length ?? 0}
                </span>
              </div>
            </div>
          </div>

          <div className="dash-sidebar__group">
            <h3 className="dash-sidebar__group-title">Automation</h3>
            <div className="dash-sidebar__items">
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/task-schedules")}
                role="button"
                tabIndex={0}
              >
                <CalendarClock size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Task Schedules</span>
                <span className="dash-sidebar__item-count">
                  {taskSchedules.data?.length ?? 0}
                </span>
              </div>
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/task-webhooks")}
                role="button"
                tabIndex={0}
              >
                <Webhook size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Task Webhooks</span>
                <span className="dash-sidebar__item-count">
                  {taskWebhooks.data?.length ?? 0}
                </span>
              </div>
            </div>
          </div>

          <div className="dash-sidebar__group">
            <h3 className="dash-sidebar__group-title">Capabilities</h3>
            <div className="dash-sidebar__items">
              <div
                className="dash-sidebar__item"
                onClick={() => navigate("/capabilities")}
                role="button"
                tabIndex={0}
              >
                <Zap size={14} className="dash-sidebar__icon" />
                <span className="dash-sidebar__item-label">Capabilities</span>
                <span className="dash-sidebar__item-count">{capCount}</span>
              </div>
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}
