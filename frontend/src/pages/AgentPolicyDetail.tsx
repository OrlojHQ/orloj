import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useAgentPolicy, useDeleteResource, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { AgentPolicy } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";
import { CrdManagedBadge } from "../components/CrdManagedBadge";
import { isCrdManaged, CRD_MANAGED_EDIT_WARNING } from "../utils/crd";
import { useDeleteConfirm } from "../hooks/useDeleteConfirm";

type Tab = "overview" | "yaml";

export function AgentPolicyDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const routeName = nameParam ?? "";
  const { data: policy, isLoading, isError, error } = useAgentPolicy(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("AgentPolicy");
  const updateMutation = useUpdateResource("AgentPolicy");
  const confirmDelete = useDeleteConfirm();
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Agent policy"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={() => navigate("/policies")}
      />
    );
  }

  if (isLoading || !policy) {
    return <div className="page"><div className="loading-placeholder">Loading agent policy...</div></div>;
  }

  const handleDelete = async () => {
    if (!confirmDelete("AgentPolicy", policy.metadata.name, policy.metadata)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
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
          <CrdManagedBadge metadata={policy.metadata} />
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
            <div className="detail-field">
              <span className="detail-field__label">Target Agents</span>
              <span className="detail-field__value mono">
                {policy.spec.target_agents?.length ? policy.spec.target_agents.join(", ") : "all"}
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
            warning={isCrdManaged(policy.metadata) ? CRD_MANAGED_EDIT_WARNING : undefined}
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<AgentPolicy>(
                queryClient,
                "AgentPolicy",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<AgentPolicy>,
              );
              toast("success", "Policy updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.AgentPolicy}/${encodeURIComponent(updated.metadata.name)}`,
                  { replace: true },
                );
              }
            }}
          />
        )}
      </div>
    </div>
  );
}
