import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTaskWebhooks } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { CreateResourceDialog } from "../components/CreateResourceDialog";
import { Plus, Webhook } from "lucide-react";
import type { TaskWebhook } from "../api/types";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

export function TaskWebhooks() {
  const { data, isLoading } = useTaskWebhooks();
  const navigate = useNavigate();
  const [showCreate, setShowCreate] = useState(false);

  const webhooks = data ?? [];

  const columns: Column<TaskWebhook>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "taskRef", header: "Task Template", render: (r) => <span className="mono">{r.spec.task_ref ?? "-"}</span> },
    { key: "profile", header: "Auth Profile", render: (r) => r.spec.auth?.profile ?? "generic", width: "120px" },
    { key: "endpointID", header: "Endpoint ID", render: (r) => <span className="mono">{r.status?.endpointID ?? "-"}</span> },
    { key: "suspend", header: "Suspended", render: (r) => (r.spec.suspend ? "Yes" : "No"), width: "95px" },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
    { key: "lastDelivery", header: "Last Delivery", render: (r) => formatDateTime(r.status?.lastDeliveryTime) },
    {
      key: "counts",
      header: "A / D / R",
      render: (r) => `${r.status?.acceptedCount ?? 0} / ${r.status?.duplicateCount ?? 0} / ${r.status?.rejectedCount ?? 0}`,
      width: "130px",
    },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Task Webhooks</h1>
          <p className="page__subtitle">{webhooks.length} webhooks</p>
        </div>
        <button className="btn-primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> New Webhook
        </button>
      </div>

      {webhooks.length === 0 && !isLoading ? (
        <EmptyState
          icon={<Webhook size={40} />}
          title="No Task Webhooks"
          description="Create event-driven webhook triggers that start one-off run tasks from task templates."
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={webhooks}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/task-webhooks/${r.metadata.name}`)}
          loading={isLoading}
        />
      )}

      <CreateResourceDialog kind="TaskWebhook" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
