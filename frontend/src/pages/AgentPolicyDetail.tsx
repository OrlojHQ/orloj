import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useAgentPolicy, useDeleteResource, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

export function AgentPolicyDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: policy, isLoading } = useAgentPolicy(name ?? "");
  const deleteMutation = useDeleteResource("AgentPolicy");
  const updateMutation = useUpdateResource("AgentPolicy");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !policy) {
    return <div className="page"><div className="loading-placeholder">Loading agent policy...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete AgentPolicy ${policy.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(policy.metadata.name);
      toast("success", "AgentPolicy deleted successfully");
      navigate("/policies");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete AgentPolicy");
    }
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/policies")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{policy.metadata.name}</h1>
            <p className="page__subtitle">
              {policy.spec.apply_mode ?? "global"} · {policy.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={policy.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Policy"}
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
              <StatusBadge phase={policy.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Apply Mode</span>
              <span className="detail-field__value">{policy.spec.apply_mode ?? "global"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Max Tokens Per Run</span>
              <span className="detail-field__value mono">{policy.spec.max_tokens_per_run ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Allowed Models</span>
              <span className="detail-field__value mono">
                {policy.spec.allowed_models?.length ? policy.spec.allowed_models.join(", ") : "any"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Blocked Tools</span>
              <span className="detail-field__value mono">
                {policy.spec.blocked_tools?.length ? policy.spec.blocked_tools.join(", ") : "none"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Target Systems</span>
              <span className="detail-field__value mono">
                {policy.spec.target_systems?.length ? policy.spec.target_systems.join(", ") : "all"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Target Tasks</span>
              <span className="detail-field__value mono">
                {policy.spec.target_tasks?.length ? policy.spec.target_tasks.join(", ") : "all"}
              </span>
            </div>
            {policy.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{policy.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(policy, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: policy.metadata.name, body, rv: policy.metadata.resourceVersion });
              toast("success", "Policy updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
