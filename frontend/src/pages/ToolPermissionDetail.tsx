import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useDeleteResource, useToolPermission, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

export function ToolPermissionDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: perm, isLoading } = useToolPermission(name ?? "");
  const deleteMutation = useDeleteResource("ToolPermission");
  const updateMutation = useUpdateResource("ToolPermission");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !perm) {
    return <div className="page"><div className="loading-placeholder">Loading tool permission...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete ToolPermission ${perm.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(perm.metadata.name);
      toast("success", "ToolPermission deleted successfully");
      navigate("/permissions");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete ToolPermission");
    }
  };

  const operationRulesDisplay = perm.spec.operation_rules?.length
    ? perm.spec.operation_rules.map((r) => `${r.operation_class ?? "?"}: ${r.verdict ?? "?"}`).join(", ")
    : "none";

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/permissions")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{perm.metadata.name}</h1>
            <p className="page__subtitle">
              {perm.spec.tool_ref ?? "—"} · {perm.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={perm.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Permission"}
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
              <StatusBadge phase={perm.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Tool Ref</span>
              <span className="detail-field__value mono">{perm.spec.tool_ref ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Action</span>
              <span className="detail-field__value">{perm.spec.action ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Match Mode</span>
              <span className="detail-field__value">{perm.spec.match_mode ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Apply Mode</span>
              <span className="detail-field__value">{perm.spec.apply_mode ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Required Permissions</span>
              <span className="detail-field__value mono">
                {perm.spec.required_permissions?.length ? perm.spec.required_permissions.join(", ") : "-"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Target Agents</span>
              <span className="detail-field__value mono">
                {perm.spec.target_agents?.length ? perm.spec.target_agents.join(", ") : "all"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Operation Rules</span>
              <span className="detail-field__value mono">{operationRulesDisplay}</span>
            </div>
            {perm.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{perm.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(perm, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: perm.metadata.name, body, rv: perm.metadata.resourceVersion });
              toast("success", "Permission updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
