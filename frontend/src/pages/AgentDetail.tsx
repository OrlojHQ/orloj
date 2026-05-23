import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useAgent, useAgentLogs, useDeleteResource, useUpdateResource, useAgentCard, useCapabilities } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { toast } from "../components/Toast";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { LogViewer } from "../components/LogViewer";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import type { Agent } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";
import { CrdManagedBadge } from "../components/CrdManagedBadge";
import { isCrdManaged, CRD_MANAGED_EDIT_WARNING } from "../utils/crd";
import { useDeleteConfirm } from "../hooks/useDeleteConfirm";
import { AgentCardPreview } from "../components/AgentCardPreview";

type Tab = "overview" | "yaml" | "logs" | "agent-card";

export function AgentDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/agents");
  const routeName = nameParam ?? "";
  const { data: agent, isLoading, isError, error } = useAgent(routeName);
  const logs = useAgentLogs(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("Agent");
  const updateMutation = useUpdateResource("Agent");
  const confirmDelete = useDeleteConfirm();
  const { data: capSnap } = useCapabilities();
  const a2aEnabled = capSnap?.capabilities?.some((c) => c.id === "a2a" && c.enabled) ?? false;
  const { data: agentCard } = useAgentCard(a2aEnabled ? routeName : "");
  const [tab, setTab] = useState<Tab>("overview");

  const handleDelete = async () => {
    if (!agent || !confirmDelete("Agent", agent.metadata.name, agent.metadata)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "Agent deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Agent");
    }
  };

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Agent"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !agent) {
    return <div className="page"><div className="loading-placeholder">Loading agent...</div></div>;
  }

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
    { id: "logs", label: "Logs" },
    ...(a2aEnabled ? [{ id: "agent-card" as const, label: "Agent Card" }] : []),
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{agent.metadata.name}</h1>
            <p className="page__subtitle">{agent.spec.model_ref} &middot; {agent.metadata.namespace}</p>
          </div>
          <StatusBadge phase={agent.status?.phase} size="md" />
          <CrdManagedBadge metadata={agent.metadata} />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Agent"}
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
              <span className="detail-field__label">Model Ref</span>
              <span className="detail-field__value mono">{agent.spec.model_ref || "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Max Steps</span>
              <span className="detail-field__value">{agent.spec.limits?.max_steps ?? 10}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Timeout</span>
              <span className="detail-field__value">{agent.spec.limits?.timeout ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Tools</span>
              <span className="detail-field__value">{agent.spec.tools?.join(", ") || "none"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Roles</span>
              <span className="detail-field__value">{agent.spec.roles?.join(", ") || "none"}</span>
            </div>
            {agent.spec.prompt && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Prompt</span>
                <pre className="detail-field__pre">{agent.spec.prompt}</pre>
              </div>
            )}
            {agent.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{agent.status.lastError}</span>
              </div>
            )}
          </div>
        )}
        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(agent, null, 2)}
            editable
            warning={isCrdManaged(agent.metadata) ? CRD_MANAGED_EDIT_WARNING : undefined}
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<Agent>(
                queryClient,
                "Agent",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<Agent>,
              );
              toast("success", "Agent updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.Agent}/${encodeURIComponent(updated.metadata.name)}`,
                  { replace: true },
                );
              }
            }}
          />
        )}
        {tab === "logs" && <LogViewer logs={logs.data ?? ""} loading={logs.isLoading} />}
        {tab === "agent-card" && agentCard && <AgentCardPreview card={agentCard} />}
        {tab === "agent-card" && !agentCard && (
          <p className="text-muted">No agent card available</p>
        )}
      </div>
    </div>
  );
}
