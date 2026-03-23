import { useState } from "react";
import { useParams } from "react-router-dom";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useTool, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

export function ToolDetail() {
  const { name } = useParams<{ name: string }>();
  const { goBack } = useDetailReturnNav("/tools");
  const { data: tool, isLoading } = useTool(name ?? "");
  const deleteMutation = useDeleteResource("Tool");
  const updateMutation = useUpdateResource("Tool");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !tool) {
    return <div className="page"><div className="loading-placeholder">Loading tool...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete Tool ${tool.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(tool.metadata.name);
      toast("success", "Tool deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Tool");
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
            <h1 className="page__title">{tool.metadata.name}</h1>
            <p className="page__subtitle">
              {tool.spec.type ?? "http"} · {tool.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={tool.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Tool"}
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
              <StatusBadge phase={tool.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Type</span>
              <span className="detail-field__value">{tool.spec.type ?? "http"}</span>
            </div>
            {tool.spec.mcp_server_ref && (
              <div className="detail-field">
                <span className="detail-field__label">MCP server</span>
                <span className="detail-field__value mono">{tool.spec.mcp_server_ref}</span>
              </div>
            )}
            <div className="detail-field">
              <span className="detail-field__label">Endpoint</span>
              <span className="detail-field__value mono">{tool.spec.endpoint ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Risk Level</span>
              <span className="detail-field__value">{tool.spec.risk_level ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Operation Classes</span>
              <span className="detail-field__value">{(tool.spec.operation_classes ?? []).join(", ") || "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Isolation Mode</span>
              <span className="detail-field__value">{tool.spec.runtime?.isolation_mode ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Timeout</span>
              <span className="detail-field__value">{tool.spec.runtime?.timeout ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Retry Max Attempts</span>
              <span className="detail-field__value">{tool.spec.runtime?.retry?.max_attempts ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Auth Profile</span>
              <span className="detail-field__value">{tool.spec.auth?.profile ?? "-"}</span>
            </div>
            {tool.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{tool.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(tool, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: tool.metadata.name, body, rv: tool.metadata.resourceVersion });
              toast("success", "Tool updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
