import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useMcpServer, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { McpServer } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";
import { CrdManagedBadge } from "../components/CrdManagedBadge";
import { isCrdManaged, CRD_MANAGED_EDIT_WARNING } from "../utils/crd";
import { useDeleteConfirm } from "../hooks/useDeleteConfirm";

type Tab = "overview" | "yaml";

export function McpServerDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/mcp-servers");
  const routeName = nameParam ?? "";
  const { data: server, isLoading, isError, error } = useMcpServer(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("McpServer");
  const updateMutation = useUpdateResource("McpServer");
  const confirmDelete = useDeleteConfirm();
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="MCP server"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !server) {
    return (
      <div className="page">
        <div className="loading-placeholder">Loading MCP server...</div>
      </div>
    );
  }

  const handleDelete = async () => {
    if (!confirmDelete("McpServer", server.metadata.name, server.metadata)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
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
          <CrdManagedBadge metadata={server.metadata} />
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
            warning={isCrdManaged(server.metadata) ? CRD_MANAGED_EDIT_WARNING : undefined}
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<McpServer>(
                queryClient,
                "McpServer",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<McpServer>,
              );
              toast("success", "MCP Server updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.McpServer}/${encodeURIComponent(updated.metadata.name)}`,
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
