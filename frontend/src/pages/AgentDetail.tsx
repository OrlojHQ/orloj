import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useAgent, useAgentLogs, useDeleteResource, useUpdateResource } from "../api/hooks";
import { toast } from "../components/Toast";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { LogViewer } from "../components/LogViewer";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";

type Tab = "overview" | "yaml" | "logs";

export function AgentDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: agent, isLoading } = useAgent(name ?? "");
  const logs = useAgentLogs(name ?? "");
  const deleteMutation = useDeleteResource("Agent");
  const updateMutation = useUpdateResource("Agent");
  const [tab, setTab] = useState<Tab>("overview");

  const handleDelete = async () => {
    if (!agent || !window.confirm(`Delete Agent ${agent.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(agent.metadata.name);
      toast("success", "Agent deleted successfully");
      navigate("/agents");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Agent");
    }
  };

  if (isLoading || !agent) {
    return <div className="page"><div className="loading-placeholder">Loading agent...</div></div>;
  }

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
    { id: "logs", label: "Logs" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/agents")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{agent.metadata.name}</h1>
            <p className="page__subtitle">{agent.spec.model_ref} &middot; {agent.metadata.namespace}</p>
          </div>
          <StatusBadge phase={agent.status?.phase} size="md" />
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
            onSave={async (body) => {
              await updateMutation.mutateAsync({ name: agent.metadata.name, body, rv: agent.metadata.resourceVersion });
              toast("success", "Agent updated");
            }}
          />
        )}
        {tab === "logs" && <LogViewer logs={logs.data ?? ""} loading={logs.isLoading} />}
      </div>
    </div>
  );
}
