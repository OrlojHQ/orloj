import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useSealedSecret, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { SealedSecret } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

type Tab = "overview" | "yaml";

export function SealedSecretDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/sealed-secrets");
  const routeName = nameParam ?? "";
  const { data: sealedSecret, isLoading, isError, error } = useSealedSecret(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("SealedSecret");
  const updateMutation = useUpdateResource("SealedSecret");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Sealed Secret"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !sealedSecret) {
    return <div className="page"><div className="loading-placeholder">Loading sealed secret...</div></div>;
  }

  const encryptedKeys = Object.keys(sealedSecret.spec.encryptedData ?? {});
  const templateLabels = Object.entries(sealedSecret.spec.template?.labels ?? {});
  const templateAnnotations = Object.entries(sealedSecret.spec.template?.annotations ?? {});

  const handleDelete = async () => {
    if (!window.confirm(`Delete SealedSecret ${sealedSecret.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "Sealed Secret deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Sealed Secret");
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
            <h1 className="page__title">{sealedSecret.metadata.name}</h1>
            <p className="page__subtitle">{sealedSecret.metadata.namespace ?? "default"}</p>
          </div>
          <StatusBadge phase={sealedSecret.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Sealed Secret"}
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
              <StatusBadge phase={sealedSecret.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Encrypted Keys</span>
              <span className="detail-field__value">{encryptedKeys.length}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Namespace</span>
              <span className="detail-field__value">{sealedSecret.metadata.namespace ?? "default"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Created At</span>
              <span className="detail-field__value">
                {sealedSecret.metadata.createdAt ? new Date(sealedSecret.metadata.createdAt).toLocaleString() : "-"}
              </span>
            </div>
            <div className="detail-field detail-field--full">
              <span className="detail-field__label">Key Names</span>
              <span className="detail-field__value mono">{encryptedKeys.join(", ") || "-"}</span>
            </div>
            <div className="detail-field detail-field--full">
              <span className="detail-field__label">Template Labels</span>
              <span className="detail-field__value mono">
                {templateLabels.length > 0 ? templateLabels.map(([k, v]) => `${k}=${v}`).join(", ") : "-"}
              </span>
            </div>
            <div className="detail-field detail-field--full">
              <span className="detail-field__label">Template Annotations</span>
              <span className="detail-field__value mono">
                {templateAnnotations.length > 0 ? templateAnnotations.map(([k, v]) => `${k}=${v}`).join(", ") : "-"}
              </span>
            </div>
            {sealedSecret.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{sealedSecret.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(sealedSecret, null, 2)}
            editable
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<SealedSecret>(
                queryClient,
                "SealedSecret",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<SealedSecret>,
              );
              toast("success", "Sealed Secret updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.SealedSecret}/${encodeURIComponent(updated.metadata.name)}`,
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
