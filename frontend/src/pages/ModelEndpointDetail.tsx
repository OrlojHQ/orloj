import { useState } from "react";
import { useParams } from "react-router-dom";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useModelEndpoint, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

export function ModelEndpointDetail() {
  const { name } = useParams<{ name: string }>();
  const { goBack } = useDetailReturnNav("/models");
  const { data: ep, isLoading } = useModelEndpoint(name ?? "");
  const deleteMutation = useDeleteResource("ModelEndpoint");
  const updateMutation = useUpdateResource("ModelEndpoint");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !ep) {
    return <div className="page"><div className="loading-placeholder">Loading model endpoint...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete ModelEndpoint ${ep.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(ep.metadata.name);
      toast("success", "ModelEndpoint deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete ModelEndpoint");
    }
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{ep.metadata.name}</h1>
            <p className="page__subtitle">
              {ep.spec.provider ?? "—"} · {ep.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={ep.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Endpoint"}
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
              <StatusBadge phase={ep.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Provider</span>
              <span className="detail-field__value">{ep.spec.provider ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Base URL</span>
              <span className="detail-field__value mono">{ep.spec.base_url ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Default Model</span>
              <span className="detail-field__value mono">{ep.spec.default_model ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Auth Secret Ref</span>
              <span className="detail-field__value mono">{ep.spec.auth?.secretRef ?? "-"}</span>
            </div>
            {ep.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{ep.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(ep, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: ep.metadata.name, body, rv: ep.metadata.resourceVersion });
              toast("success", "Model endpoint updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
