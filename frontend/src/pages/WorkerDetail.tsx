import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useDeleteResource, useWorker, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

export function WorkerDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: worker, isLoading } = useWorker(name ?? "");
  const deleteMutation = useDeleteResource("Worker");
  const updateMutation = useUpdateResource("Worker");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !worker) {
    return <div className="page"><div className="loading-placeholder">Loading worker...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete Worker ${worker.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync({
        name: worker.metadata.name,
        namespace: worker.metadata.namespace?.trim() || "default",
      });
      toast("success", "Worker deleted successfully");
      navigate("/workers");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Worker");
    }
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/workers")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{worker.metadata.name}</h1>
            <p className="page__subtitle">
              {worker.spec.region ?? "default"} · {worker.metadata.namespace ?? "default"}
            </p>
          </div>
          <StatusBadge phase={worker.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Worker"}
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
              <StatusBadge phase={worker.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Region</span>
              <span className="detail-field__value">{worker.spec.region ?? "default"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">GPU</span>
              <span className="detail-field__value">{worker.spec.capabilities?.gpu ? "Yes" : "No"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Supported Models</span>
              <span className="detail-field__value mono">
                {(worker.spec.capabilities?.supported_models ?? []).join(", ") || "-"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Max Concurrent Tasks</span>
              <span className="detail-field__value">{worker.spec.max_concurrent_tasks ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Current Tasks</span>
              <span className="detail-field__value">{worker.status?.currentTasks ?? 0}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last Heartbeat</span>
              <span className="detail-field__value">{formatDateTime(worker.status?.lastHeartbeat)}</span>
            </div>
            {worker.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{worker.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(worker, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({
                name: worker.metadata.name,
                body,
                rv: worker.metadata.resourceVersion,
                namespace: worker.metadata.namespace?.trim() || "default",
              });
              toast("success", "Worker updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
