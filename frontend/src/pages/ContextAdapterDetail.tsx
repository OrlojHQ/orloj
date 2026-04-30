import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useContextAdapter, useDeleteResource, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { ContextAdapter } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

type Tab = "overview" | "yaml";

export function ContextAdapterDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/context-adapters");
  const routeName = nameParam ?? "";
  const { data: adapter, isLoading, isError, error } = useContextAdapter(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("ContextAdapter");
  const updateMutation = useUpdateResource("ContextAdapter");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Context adapter"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !adapter) {
    return (
      <div className="page">
        <div className="loading-placeholder">Loading context adapter...</div>
      </div>
    );
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete ContextAdapter ${adapter.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "ContextAdapter deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete ContextAdapter");
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
            <h1 className="page__title">{adapter.metadata.name}</h1>
            <p className="page__subtitle">
              {adapter.spec.tool_ref} · {adapter.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={adapter.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete ContextAdapter"}
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
              <StatusBadge phase={adapter.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Tool ref</span>
              <span className="detail-field__value mono">{adapter.spec.tool_ref}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">On error</span>
              <span className="detail-field__value">{adapter.spec.on_error ?? "reject"}</span>
            </div>
            {adapter.status?.message && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Message</span>
                <span className="detail-field__value">{adapter.status.message}</span>
              </div>
            )}
          </div>
        )}
        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(adapter, null, 2)}
            editable
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<ContextAdapter>(
                queryClient,
                "ContextAdapter",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<ContextAdapter>,
              );
              toast("success", "Context adapter updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.ContextAdapter}/${encodeURIComponent(updated.metadata.name)}`,
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
