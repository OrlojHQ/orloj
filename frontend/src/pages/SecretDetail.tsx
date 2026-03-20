import { useState } from "react";
import { useParams } from "react-router-dom";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useSecret, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

export function SecretDetail() {
  const { name } = useParams<{ name: string }>();
  const { goBack } = useDetailReturnNav("/secrets");
  const { data: secret, isLoading } = useSecret(name ?? "");
  const deleteMutation = useDeleteResource("Secret");
  const updateMutation = useUpdateResource("Secret");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !secret) {
    return <div className="page"><div className="loading-placeholder">Loading secret...</div></div>;
  }

  const dataKeys = Object.keys(secret.spec.data ?? {});

  const handleDelete = async () => {
    if (!window.confirm(`Delete Secret ${secret.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(secret.metadata.name);
      toast("success", "Secret deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Secret");
    }
  };

  const redactedSecret = {
    ...secret,
    spec: {
      ...secret.spec,
      data: Object.fromEntries(dataKeys.map((k) => [k, "***"])),
      ...(secret.spec.stringData
        ? { stringData: Object.fromEntries(Object.keys(secret.spec.stringData).map((k) => [k, "***"])) }
        : {}),
    },
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{secret.metadata.name}</h1>
            <p className="page__subtitle">{secret.metadata.namespace ?? "default"}</p>
          </div>
          <StatusBadge phase={secret.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Secret"}
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
              <StatusBadge phase={secret.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Keys</span>
              <span className="detail-field__value">{dataKeys.length}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Key Names</span>
              <span className="detail-field__value mono">{dataKeys.join(", ") || "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Created At</span>
              <span className="detail-field__value">
                {secret.metadata.createdAt ? new Date(secret.metadata.createdAt).toLocaleString() : "-"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Namespace</span>
              <span className="detail-field__value">{secret.metadata.namespace ?? "default"}</span>
            </div>
            {secret.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{secret.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(redactedSecret, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: secret.metadata.name, body, rv: secret.metadata.resourceVersion });
              toast("success", "Secret updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
