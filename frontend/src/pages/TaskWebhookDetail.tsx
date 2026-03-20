import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { ArrowLeft, Clipboard } from "lucide-react";
import clsx from "clsx";
import { useDeleteResource, useTaskWebhook, useTasks, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { toast } from "../components/Toast";
import type { Task } from "../api/types";

type Tab = "overview" | "runs" | "yaml";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

function taskNameFromRef(ref?: string): string | null {
  if (!ref) return null;
  const trimmed = ref.trim();
  if (!trimmed) return null;
  const slash = trimmed.indexOf("/");
  if (slash > 0 && slash < trimmed.length - 1) {
    return trimmed.slice(slash + 1);
  }
  return trimmed;
}

export function TaskWebhookDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/task-webhooks");
  const { data: taskWebhook, isLoading } = useTaskWebhook(name ?? "");
  const tasks = useTasks();
  const deleteMutation = useDeleteResource("TaskWebhook");
  const updateMutation = useUpdateResource("TaskWebhook");
  const [tab, setTab] = useState<Tab>("overview");

  const webhookNamespace = taskWebhook?.metadata.namespace ?? "default";

  const runs = useMemo(() => {
    if (!taskWebhook) return [];
    return (tasks.data ?? [])
      .filter((task) => {
        const labels = task.metadata.labels ?? {};
        return (
          labels["orloj.dev/task-webhook"] === taskWebhook.metadata.name &&
          labels["orloj.dev/task-webhook-namespace"] === webhookNamespace
        );
      })
      .sort((a, b) => {
        const at = a.metadata.createdAt ?? "";
        const bt = b.metadata.createdAt ?? "";
        return bt.localeCompare(at);
      });
  }, [tasks.data, taskWebhook, webhookNamespace]);

  const runColumns: Column<Task>[] = [
    { key: "name", header: "Run Task", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
    { key: "worker", header: "Worker", render: (r) => <span className="text-muted">{r.status?.assignedWorker ?? "-"}</span> },
    { key: "started", header: "Started", render: (r) => formatDateTime(r.status?.startedAt) },
    { key: "completed", header: "Completed", render: (r) => formatDateTime(r.status?.completedAt) },
  ];

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "runs", label: `Runs (${runs.length})` },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !taskWebhook) {
    return <div className="page"><div className="loading-placeholder">Loading task webhook...</div></div>;
  }

  const webhookDetailPath = `/task-webhooks/${encodeURIComponent(name ?? "")}`;

  const handleDelete = async () => {
    if (!window.confirm(`Delete TaskWebhook ${taskWebhook.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(taskWebhook.metadata.name);
      toast("success", "TaskWebhook deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete TaskWebhook");
    }
  };

  const copyValue = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast("success", `${label} copied`);
    } catch {
      toast("error", `Failed to copy ${label.toLowerCase()}`);
    }
  };

  const lastTriggeredTaskName = taskNameFromRef(taskWebhook.status?.lastTriggeredTask);

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{taskWebhook.metadata.name}</h1>
            <p className="page__subtitle">
              {taskWebhook.spec.task_ref ?? "-"} · {taskWebhook.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={taskWebhook.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Webhook"}
        </button>
      </div>

      <div className="tab-bar">
        {tabs.map((t) => (
          <button
            key={t.id}
            className={clsx("tab-bar__tab", tab === t.id && "tab-bar__tab--active")}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="tab-content">
        {tab === "overview" && (
          <div className="detail-grid">
            <div className="detail-field">
              <span className="detail-field__label">Phase</span>
              <StatusBadge phase={taskWebhook.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Task Template</span>
              <span className="detail-field__value mono">{taskWebhook.spec.task_ref ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Suspended</span>
              <span className="detail-field__value">{taskWebhook.spec.suspend ? "Yes" : "No"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Auth Profile</span>
              <span className="detail-field__value">{taskWebhook.spec.auth?.profile ?? "generic"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Secret Ref</span>
              <span className="detail-field__value mono">{taskWebhook.spec.auth?.secret_ref ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Signature Header</span>
              <span className="detail-field__value mono">{taskWebhook.spec.auth?.signature_header ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Signature Prefix</span>
              <span className="detail-field__value mono">{taskWebhook.spec.auth?.signature_prefix ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Timestamp Header</span>
              <span className="detail-field__value mono">{taskWebhook.spec.auth?.timestamp_header ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Max Skew (seconds)</span>
              <span className="detail-field__value">{taskWebhook.spec.auth?.max_skew_seconds ?? 300}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Event ID Header</span>
              <span className="detail-field__value mono">{taskWebhook.spec.idempotency?.event_id_header ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Dedupe Window (seconds)</span>
              <span className="detail-field__value">{taskWebhook.spec.idempotency?.dedupe_window_seconds ?? 86400}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Payload Mode</span>
              <span className="detail-field__value">{taskWebhook.spec.payload?.mode ?? "raw"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Payload Input Key</span>
              <span className="detail-field__value mono">{taskWebhook.spec.payload?.input_key ?? "webhook_payload"}</span>
            </div>

            <div className="detail-field">
              <span className="detail-field__label">Endpoint ID</span>
              <span className="detail-field__value mono">{taskWebhook.status?.endpointID ?? "-"}</span>
              {!!taskWebhook.status?.endpointID && (
                <button className="btn-ghost" onClick={() => copyValue(taskWebhook.status!.endpointID!, "Endpoint ID")}>
                  <Clipboard size={14} /> Copy
                </button>
              )}
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Endpoint Path</span>
              <span className="detail-field__value mono">{taskWebhook.status?.endpointPath ?? "-"}</span>
              {!!taskWebhook.status?.endpointPath && (
                <button className="btn-ghost" onClick={() => copyValue(taskWebhook.status!.endpointPath!, "Endpoint path")}>
                  <Clipboard size={14} /> Copy
                </button>
              )}
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last Delivery Time</span>
              <span className="detail-field__value">{formatDateTime(taskWebhook.status?.lastDeliveryTime)}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last Event ID</span>
              <span className="detail-field__value mono">{taskWebhook.status?.lastEventID ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last Triggered Task</span>
              <span
                className={clsx("detail-field__value", lastTriggeredTaskName && "detail-field__link")}
                onClick={() => {
                  if (lastTriggeredTaskName) {
                    navigate(`/tasks/${encodeURIComponent(lastTriggeredTaskName)}`, {
                      state: { returnTo: webhookDetailPath },
                    });
                  }
                }}
              >
                {taskWebhook.status?.lastTriggeredTask ?? "-"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Accepted Count</span>
              <span className="detail-field__value">{taskWebhook.status?.acceptedCount ?? 0}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Duplicate Count</span>
              <span className="detail-field__value">{taskWebhook.status?.duplicateCount ?? 0}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Rejected Count</span>
              <span className="detail-field__value">{taskWebhook.status?.rejectedCount ?? 0}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Observed Generation</span>
              <span className="detail-field__value">{taskWebhook.status?.observedGeneration ?? "-"}</span>
            </div>
            {taskWebhook.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{taskWebhook.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "runs" && (
          <ResourceTable
            columns={runColumns}
            data={runs}
            rowKey={(r) => r.metadata.name}
            onRowClick={(r) =>
              navigate(`/tasks/${encodeURIComponent(r.metadata.name)}`, { state: { returnTo: webhookDetailPath } })
            }
            emptyMessage="No generated runs for this webhook"
            loading={tasks.isLoading}
          />
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(taskWebhook, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: taskWebhook.metadata.name, body, rv: taskWebhook.metadata.resourceVersion });
              toast("success", "Task webhook updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
