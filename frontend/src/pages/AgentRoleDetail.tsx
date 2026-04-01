import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useAgentRole, useDeleteResource, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { AgentRole } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

type Tab = "overview" | "yaml";

export function AgentRoleDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/roles");
  const routeName = nameParam ?? "";
  const { data: role, isLoading, isError, error } = useAgentRole(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("AgentRole");
  const updateMutation = useUpdateResource("AgentRole");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Agent role"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !role) {
    return <div className="page"><div className="loading-placeholder">Loading agent role...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete AgentRole ${role.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "AgentRole deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete AgentRole");
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
            <h1 className="page__title">{role.metadata.name}</h1>
            <p className="page__subtitle">{role.metadata.namespace}</p>
          </div>
          <StatusBadge phase={role.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Role"}
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
              <StatusBadge phase={role.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Description</span>
              <span className="detail-field__value">{role.spec.description ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Permissions</span>
              <span className="detail-field__value mono">
                {role.spec.permissions?.length ? role.spec.permissions.join(", ") : "none"}
              </span>
            </div>
            {role.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{role.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(role, null, 2)}
            editable
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<AgentRole>(
                queryClient,
                "AgentRole",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<AgentRole>,
              );
              toast("success", "Role updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.AgentRole}/${encodeURIComponent(updated.metadata.name)}`,
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
