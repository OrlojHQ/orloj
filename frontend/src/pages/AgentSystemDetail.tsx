import { useState, useMemo, useEffect, useCallback } from "react";
import { useParams, useNavigate, useSearchParams, Link } from "react-router-dom";
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
} from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { toast } from "../components/Toast";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { GraphView } from "../components/GraphView";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import type { AgentSystem, Task } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";
import { CrdManagedBadge } from "../components/CrdManagedBadge";
import { isCrdManaged, CRD_MANAGED_EDIT_WARNING } from "../utils/crd";
import { useDeleteConfirm } from "../hooks/useDeleteConfirm";

type Tab = "graph" | "tasks" | "yaml" | "status";

const TAB_PARAM = "tab";
const VALID_TABS: readonly Tab[] = ["graph", "tasks", "yaml", "status"];

function parseTab(raw: string | null): Tab | null {
  if (!raw) return null;
  return VALID_TABS.includes(raw as Tab) ? (raw as Tab) : null;
}

export function AgentSystemDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const routeName = nameParam ?? "";
  const [searchParams, setSearchParams] = useSearchParams();
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
  const deleteMutation = useDeleteResource("AgentSystem");
  const updateMutation = useUpdateResource("AgentSystem");
  const confirmDelete = useDeleteConfirm();
  const [tab, setTab] = useState<Tab>(() => parseTab(searchParams.get(TAB_PARAM)) ?? "graph");

  useEffect(() => {
    setTab(parseTab(searchParams.get(TAB_PARAM)) ?? "graph");
  }, [routeName, searchParams]);

  const setTabInUrl = useCallback(
    (t: Tab) => {
      setTab(t);
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (t === "graph") next.delete(TAB_PARAM);
          else next.set(TAB_PARAM, t);
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const returnToWithTab = useCallback(
    (t: Tab) => {
      const base = `/systems/${encodeURIComponent(routeName)}`;
      if (t === "graph") return base;
      return `${base}?${TAB_PARAM}=${t}`;
    },
    [routeName],
  );

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

  const taskColumns: Column<Task>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "phase", header: "Phase", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
    { key: "worker", header: "Worker", render: (r) => <span className="text-muted">{r.status?.assignedWorker ?? "—"}</span> },
    { key: "attempts", header: "Attempts", render: (r) => r.status?.attempts ?? 0, width: "90px" },
  ];

  const handleNodeClick = (kind: string, nodeName: string) => {
    const fromGraph = { state: { returnTo: returnToWithTab("graph") } };
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
  };

  const yamlContent = JSON.stringify(system, null, 2);

  const contextAdapterRef = system.spec.context_adapter?.trim();

  const tabs: { id: Tab; label: string }[] = [
    { id: "graph", label: "Resource Tree" },
    { id: "tasks", label: `Tasks (${systemTasks.length})` },
    { id: "yaml", label: "YAML" },
    { id: "status", label: "Status" },
  ];

  return (
    <div className="page">
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
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete System"}
        </button>
      </div>

      <div className="tab-bar">
        {tabs.map((t) => (
          <button
            key={t.id}
            className={clsx("tab-bar__tab", tab === t.id && "tab-bar__tab--active")}
            onClick={() => setTabInUrl(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="tab-content">
        {tab === "graph" && (
          <GraphView system={system} related={related} onNodeClick={handleNodeClick} animated runningAgents={runningAgents} />
        )}
        {tab === "tasks" && (
          <ResourceTable
            columns={taskColumns}
            data={systemTasks}
            rowKey={(r) => r.metadata.name}
            onRowClick={(r) =>
              navigate(`/tasks/${encodeURIComponent(r.metadata.name)}`, {
                state: { returnTo: returnToWithTab("tasks") },
              })
            }
            emptyMessage="No tasks for this system"
          />
        )}
        {tab === "yaml" && (
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
        )}
        {tab === "status" && (
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
        )}
      </div>
    </div>
  );
}
