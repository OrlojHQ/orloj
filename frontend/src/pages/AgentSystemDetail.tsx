import { useState, useMemo, useCallback } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAgentSystem,
  useAgents,
  useModelEndpoints,
  useTools,
  useSecrets,
  useMemories,
  useAgentRoles,
  useTasks,
  useTaskSchedules,
  useTaskWebhooks,
  useWorkers,
  useDeleteResource,
  useUpdateResource,
  useHealthCheck,
} from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { toast } from "../components/Toast";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { GraphView } from "../components/GraphView";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { SystemHealthHorizon } from "../components/SystemHealthHorizon";
import { SystemDefinitions } from "../components/SystemDefinitions";
import { TaskTraceTimeline } from "../components/TaskTraceTimeline";
import { ArrowLeft, ChevronRight, Code, Info } from "lucide-react";
import type { AgentSystem } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";
import { CrdManagedBadge } from "../components/CrdManagedBadge";
import { isCrdManaged, CRD_MANAGED_EDIT_WARNING } from "../utils/crd";
import { useDeleteConfirm } from "../hooks/useDeleteConfirm";

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

type Overlay = "yaml" | "status" | null;

export function AgentSystemDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const routeName = nameParam ?? "";
  const { data: system, isLoading, isError, error } = useAgentSystem(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const agents = useAgents();
  const modelEndpoints = useModelEndpoints();
  const tools = useTools();
  const secrets = useSecrets();
  const memories = useMemories();
  const roles = useAgentRoles();
  const tasks = useTasks();
  const taskSchedules = useTaskSchedules();
  const taskWebhooks = useTaskWebhooks();
  const workers = useWorkers();
  const health = useHealthCheck();
  const deleteMutation = useDeleteResource("AgentSystem");
  const updateMutation = useUpdateResource("AgentSystem");
  const confirmDelete = useDeleteConfirm();
  const [overlay, setOverlay] = useState<Overlay>(null);

  const related = useMemo(() => ({
    agents: agents.data,
    modelEndpoints: modelEndpoints.data,
    tools: tools.data,
    secrets: secrets.data,
    memories: memories.data,
    roles: roles.data,
    tasks: tasks.data,
    taskSchedules: taskSchedules.data,
    taskWebhooks: taskWebhooks.data,
    workers: workers.data,
  }), [agents.data, modelEndpoints.data, tools.data, secrets.data, memories.data, roles.data, tasks.data, taskSchedules.data, taskWebhooks.data, workers.data]);

  const sysName = system?.metadata.name;

  const systemTasks = useMemo(
    () => sysName ? (tasks.data ?? []).filter((t) => t.spec.system === sysName) : [],
    [tasks.data, sysName],
  );

  const runningAgents = useMemo(() => {
    const running = new Set<string>();
    for (const task of systemTasks) {
      if (task.status?.phase !== "Running") continue;
      for (const msg of task.status?.messages ?? []) {
        if (msg.phase === "Running" && msg.to_agent) {
          running.add(msg.to_agent);
        }
      }
      if (running.size === 0) {
        const msgs = task.status?.messages ?? [];
        for (let i = msgs.length - 1; i >= 0; i--) {
          if (msgs[i].to_agent) {
            running.add(msgs[i].to_agent!);
            break;
          }
        }
      }
    }
    return running;
  }, [systemTasks]);

  const handleDelete = async () => {
    if (!system || !confirmDelete("AgentSystem", system.metadata.name, system.metadata)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "AgentSystem deleted successfully");
      navigate("/systems");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete AgentSystem");
    }
  };

  const handleNodeClick = useCallback(
    (kind: string, nodeName: string) => {
      const fromGraph = { state: { returnTo: `/systems/${encodeURIComponent(routeName)}` } };
      switch (kind) {
        case "agent":
          navigate(`/agents/${encodeURIComponent(nodeName)}`, fromGraph);
          break;
        case "task":
          navigate(`/tasks/${encodeURIComponent(nodeName)}`, fromGraph);
          break;
        case "schedule":
          navigate(`/task-schedules/${encodeURIComponent(nodeName)}`, fromGraph);
          break;
        case "webhook":
          navigate(`/task-webhooks/${encodeURIComponent(nodeName)}`, fromGraph);
          break;
        case "model":
          navigate("/models", fromGraph);
          break;
        case "tool":
          navigate("/tools", fromGraph);
          break;
        case "secret":
          navigate("/secrets", fromGraph);
          break;
        case "memory":
          navigate("/memories", fromGraph);
          break;
        case "role":
          navigate("/roles", fromGraph);
          break;
        case "worker":
          navigate("/workers", fromGraph);
          break;
        case "adapter":
          navigate(`/context-adapters/${encodeURIComponent(nodeName)}`, fromGraph);
          break;
      }
    },
    [navigate, routeName],
  );

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Agent system"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={() => navigate("/systems")}
      />
    );
  }

  if (isLoading || !system) {
    return (
      <div className="page">
        <div className="loading-placeholder">Loading system...</div>
      </div>
    );
  }

  const yamlContent = JSON.stringify(system, null, 2);
  const contextAdapterRef = system.spec.context_adapter?.trim();
  const apiReachable = health.data === true;

  if (overlay === "yaml") {
    return (
      <div className="page">
        <div className="page__header">
          <div className="page__header-back">
            <button className="btn-ghost" onClick={() => setOverlay(null)} aria-label="Back">
              <ArrowLeft size={16} />
            </button>
            <div>
              <h1 className="page__title">{system.metadata.name} — YAML</h1>
            </div>
          </div>
        </div>
        <YamlEditor
          value={yamlContent}
          editable
          warning={isCrdManaged(system.metadata) ? CRD_MANAGED_EDIT_WARNING : undefined}
          onSave={async (body) => {
            const updated = await saveNamespacedResourceYaml<AgentSystem>(
              queryClient,
              "AgentSystem",
              namespace,
              routeName,
              body,
              (a) => updateMutation.mutateAsync(a) as Promise<AgentSystem>,
            );
            toast("success", "Agent system updated");
            if (updated.metadata.name !== routeName) {
              navigate(
                `${RESOURCE_DETAIL_BASE_PATH.AgentSystem}/${encodeURIComponent(updated.metadata.name)}`,
                { replace: true },
              );
            }
          }}
        />
      </div>
    );
  }

  if (overlay === "status") {
    return (
      <div className="page">
        <div className="page__header">
          <div className="page__header-back">
            <button className="btn-ghost" onClick={() => setOverlay(null)} aria-label="Back">
              <ArrowLeft size={16} />
            </button>
            <div>
              <h1 className="page__title">{system.metadata.name} — Status</h1>
            </div>
          </div>
        </div>
        <div className="detail-grid">
          <div className="detail-field">
            <span className="detail-field__label">Phase</span>
            <StatusBadge phase={system.status?.phase} size="md" />
          </div>
          <div className="detail-field">
            <span className="detail-field__label">Agents</span>
            <span className="detail-field__value">{(system.spec.agents ?? []).join(", ")}</span>
          </div>
          {contextAdapterRef && (
            <div className="detail-field">
              <span className="detail-field__label">Context adapter</span>
              <Link
                className="detail-field__value mono"
                to={`/context-adapters/${encodeURIComponent(contextAdapterRef)}`}
              >
                {contextAdapterRef}
              </Link>
            </div>
          )}
          {system.status?.lastError && (
            <div className="detail-field">
              <span className="detail-field__label">Last Error</span>
              <span className="detail-field__value text-red">{system.status.lastError}</span>
            </div>
          )}
          <div className="detail-field">
            <span className="detail-field__label">Resource Version</span>
            <span className="detail-field__value mono">{system.metadata.resourceVersion ?? "—"}</span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="page page--system-detail">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/systems")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{system.metadata.name}</h1>
            <p className="page__subtitle">
              {system.spec.agents?.length ?? 0} agents &middot; {system.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={system.status?.phase} size="md" />
          <CrdManagedBadge metadata={system.metadata} />
        </div>
        <div className="page__header-actions">
          <button
            className="btn-ghost"
            onClick={() => setOverlay("status")}
            title="Status details"
          >
            <Info size={16} />
          </button>
          <button
            className="btn-ghost"
            onClick={() => setOverlay("yaml")}
            title="Edit YAML"
          >
            <Code size={16} />
          </button>
          <button
            className="btn-secondary text-red"
            onClick={handleDelete}
            disabled={deleteMutation.isPending}
          >
            {deleteMutation.isPending ? "Deleting..." : "Delete"}
          </button>
        </div>
      </div>

      {/* System Health Horizon Banner */}
      <SystemHealthHorizon
        tasks={tasks.data ?? []}
        systemName={sysName!}
        apiReachable={apiReachable}
        workers={workers.data ?? []}
      />

      {/* Modern Topology View */}
      <section className="system-detail__topology">
        <div className="system-detail__topology-header">
          <h2 className="system-detail__section-title">MODERN TOPOLOGY VIEW</h2>
        </div>
        <GraphView
          system={system}
          related={related}
          onNodeClick={handleNodeClick}
          animated
          runningAgents={runningAgents}
        />
      </section>

      {/* Bento Grid: Definitions | Recent Tasks | Trace Timeline */}
      <div className="system-detail__bento">
        <SystemDefinitions
          agents={agents.data ?? []}
          modelEndpoints={modelEndpoints.data ?? []}
          tools={tools.data ?? []}
          workers={workers.data ?? []}
          systemAgentNames={system.spec.agents ?? []}
          apiReachable={apiReachable}
        />

        <div className="system-detail__recent-tasks">
          <div className="system-detail__card-header">
            <h3 className="system-detail__card-title">RECENT TASKS</h3>
          </div>
          <div className="system-detail__tasks-list">
            <div className="system-detail__tasks-row system-detail__tasks-row--header">
              <span>STATUS</span>
              <span>UPDATED</span>
              <span>ACTION</span>
            </div>
            {systemTasks.length === 0 && (
              <p className="text-muted system-detail__tasks-empty">No tasks yet</p>
            )}
            {systemTasks.slice(0, 5).map((task) => (
              <div
                key={task.metadata.name}
                className="system-detail__tasks-row"
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
                <span><StatusBadge phase={task.status?.phase} /></span>
                <span className="text-muted">
                  {formatShortTime(task.status?.completedAt ?? task.status?.startedAt)}
                </span>
                <span className="system-detail__tasks-action">
                  Quick action <ChevronRight size={12} />
                </span>
              </div>
            ))}
            {systemTasks.length > 0 && (
              <div className="system-detail__tasks-footer">
                <span className="text-muted mono">{systemTasks[systemTasks.length - 1]?.metadata.name}</span>
              </div>
            )}
          </div>
        </div>

        <TaskTraceTimeline tasks={tasks.data ?? []} systemName={sysName!} />
      </div>
    </div>
  );
}
