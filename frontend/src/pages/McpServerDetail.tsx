import { useState } from "react";
import { useParams } from "react-router-dom";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useMcpServer, useUpdateResource } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";

type Tab = "overview" | "yaml";

export function McpServerDetail() {
  const { name } = useParams<{ name: string }>();
  const { goBack } = useDetailReturnNav("/mcp-servers");
  const { data: server, isLoading } = useMcpServer(name ?? "");
  const deleteMutation = useDeleteResource("McpServer");
  const updateMutation = useUpdateResource("McpServer");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !server) {
    return (
      <div className="page">
        <div className="loading-placeholder">Loading MCP server...</div>
      </div>
    );
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete MCP Server ${server.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(server.metadata.name);
      toast("success", "MCP Server deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete MCP Server");
    }
  };

  const gen = server.status?.generatedTools ?? [];
  const disc = server.status?.discoveredTools ?? [];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{server.metadata.name}</h1>
            <p className="page__subtitle">
              {server.spec.transport ?? "—"} · {server.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={server.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete MCP Server"}
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
              <StatusBadge phase={server.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Transport</span>
              <span className="detail-field__value">{server.spec.transport ?? "—"}</span>
            </div>
            <div className="detail-field detail-field--full">
              <span className="detail-field__label">Command</span>
              <span className="detail-field__value mono">{server.spec.command || "—"}</span>
            </div>
            <div className="detail-field detail-field--full">
              <span className="detail-field__label">Args</span>
              <span className="detail-field__value mono">{(server.spec.args ?? []).join(" ") || "—"}</span>
            </div>
            <div className="detail-field detail-field--full">
              <span className="detail-field__label">Endpoint</span>
              <span className="detail-field__value mono">{server.spec.endpoint ?? "—"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Reconnect</span>
              <span className="detail-field__value">
                {server.spec.reconnect?.max_attempts ?? "—"} attempts, {server.spec.reconnect?.backoff ?? "—"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last synced</span>
              <span className="detail-field__value">
                {server.status?.lastSyncedAt ? new Date(server.status.lastSyncedAt).toLocaleString() : "—"}
              </span>
            </div>
            {disc.length > 0 && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Discovered tools</span>
                <span className="detail-field__value mono">{disc.join(", ")}</span>
              </div>
            )}
            {gen.length > 0 && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Generated Tool resources</span>
                <span className="detail-field__value mono">{gen.join(", ")}</span>
              </div>
            )}
            {server.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{server.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(server, null, 2)}
            editable
            onSave={async (body) => {
              await updateMutation.mutateAsync({
                name: server.metadata.name,
                body,
                rv: server.metadata.resourceVersion,
              });
              toast("success", "MCP Server updated");
            }}
          />
        )}
      </div>
    </div>
  );
}
