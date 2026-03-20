import { useState, useMemo, useEffect, useCallback } from "react";
import { useParams, useNavigate, useSearchParams } from "react-router-dom";
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
import { toast } from "../components/Toast";
import { GraphView } from "../components/GraphView";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import type { Task } from "../api/types";

type Tab = "graph" | "tasks" | "yaml" | "status";

const TAB_PARAM = "tab";
const VALID_TABS: readonly Tab[] = ["graph", "tasks", "yaml", "status"];

function parseTab(raw: string | null): Tab | null {
  if (!raw) return null;
  return VALID_TABS.includes(raw as Tab) ? (raw as Tab) : null;
}

export function AgentSystemDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { data: system, isLoading } = useAgentSystem(name ?? "");
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
  const [tab, setTab] = useState<Tab>(() => parseTab(searchParams.get(TAB_PARAM)) ?? "graph");

  useEffect(() => {
    setTab(parseTab(searchParams.get(TAB_PARAM)) ?? "graph");
  }, [name, searchParams]);

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
      const base = `/systems/${encodeURIComponent(name ?? "")}`;
      if (t === "graph") return base;
      return `${base}?${TAB_PARAM}=${t}`;
    },
    [name],
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
    if (!system || !window.confirm(`Delete AgentSystem ${system.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(system.metadata.name);
      toast("success", "AgentSystem deleted successfully");
      navigate("/systems");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete AgentSystem");
    }
  };

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
    }
  };

  const yamlContent = JSON.stringify(system, null, 2);

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
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: system.metadata.name, body, rv: system.metadata.resourceVersion });
              toast("success", "Agent system updated");
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
