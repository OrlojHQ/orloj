import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useDeleteResource, useMemory, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

export function MemoryDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: memory, isLoading } = useMemory(name ?? "");
  const deleteMutation = useDeleteResource("Memory");
  const updateMutation = useUpdateResource("Memory");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !memory) {
    return <div className="page"><div className="loading-placeholder">Loading memory...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete Memory ${memory.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(memory.metadata.name);
      toast("success", "Memory deleted successfully");
      navigate("/memories");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Memory");
    }
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/memories")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{memory.metadata.name}</h1>
            <p className="page__subtitle">
              {memory.spec.type ?? "—"} · {memory.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={memory.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Memory"}
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
              <StatusBadge phase={memory.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Type</span>
              <span className="detail-field__value">{memory.spec.type ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Provider</span>
              <span className="detail-field__value">{memory.spec.provider ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Embedding Model</span>
              <span className="detail-field__value mono">{memory.spec.embedding_model ?? "-"}</span>
            </div>
            {memory.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{memory.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(memory, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: memory.metadata.name, body, rv: memory.metadata.resourceVersion });
              toast("success", "Memory updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
