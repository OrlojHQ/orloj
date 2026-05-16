import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useQueryClient } from "@tanstack/react-query";
import { useDeleteResource, useModelEndpoint, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { ModelEndpoint } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { CrdManagedBadge } from "../components/CrdManagedBadge";
import { isCrdManaged, CRD_MANAGED_EDIT_WARNING } from "../utils/crd";
import { useDeleteConfirm } from "../hooks/useDeleteConfirm";

type Tab = "overview" | "yaml";

export function ModelEndpointDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/models");
  const routeName = nameParam ?? "";
  const { data: ep, isLoading, isError, error } = useModelEndpoint(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("ModelEndpoint");
  const updateMutation = useUpdateResource("ModelEndpoint");
  const confirmDelete = useDeleteConfirm();
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Model endpoint"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !ep) {
    return <div className="page"><div className="loading-placeholder">Loading model endpoint...</div></div>;
  }

  const handleDelete = async () => {
    if (!confirmDelete("ModelEndpoint", ep.metadata.name, ep.metadata)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "ModelEndpoint deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete ModelEndpoint");
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
            <h1 className="page__title">{ep.metadata.name}</h1>
            <p className="page__subtitle">
              {ep.spec.provider ?? "—"} · {ep.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={ep.status?.phase} size="md" />
          <CrdManagedBadge metadata={ep.metadata} />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Endpoint"}
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
              <StatusBadge phase={ep.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Provider</span>
              <span className="detail-field__value">{ep.spec.provider ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Base URL</span>
              <span className="detail-field__value mono">{ep.spec.base_url ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Default Model</span>
              <span className="detail-field__value mono">{ep.spec.default_model ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Auth Secret Ref</span>
              <span className="detail-field__value mono">{ep.spec.auth?.secretRef ?? "-"}</span>
            </div>
            {ep.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{ep.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(ep, null, 2)}
            editable
            warning={isCrdManaged(ep.metadata) ? CRD_MANAGED_EDIT_WARNING : undefined}
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<ModelEndpoint>(
                queryClient,
                "ModelEndpoint",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<ModelEndpoint>,
              );
              toast("success", "Model endpoint updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.ModelEndpoint}/${encodeURIComponent(updated.metadata.name)}`,
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
