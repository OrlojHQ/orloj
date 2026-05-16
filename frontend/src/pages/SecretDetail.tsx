import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useDeleteResource, useSecret, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft, ShieldCheck } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { Secret } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";
import { CrdManagedBadge } from "../components/CrdManagedBadge";
import { isCrdManaged, CRD_MANAGED_EDIT_WARNING } from "../utils/crd";
import { useDeleteConfirm } from "../hooks/useDeleteConfirm";

const SEALED_OWNER_ANNOTATION = "orloj.dev/sealedsecret-owner";

type Tab = "overview" | "yaml";

export function SecretDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/secrets");
  const routeName = nameParam ?? "";
  const { data: secret, isLoading, isError, error } = useSecret(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("Secret");
  const updateMutation = useUpdateResource("Secret");
  const confirmDelete = useDeleteConfirm();
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Secret"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !secret) {
    return <div className="page"><div className="loading-placeholder">Loading secret...</div></div>;
  }

  const dataKeys = Object.keys(secret.spec.data ?? {});
  const sealedOwner = secret.metadata.annotations?.[SEALED_OWNER_ANNOTATION];

  const handleDelete = async () => {
    if (!confirmDelete("Secret", secret.metadata.name, secret.metadata)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "Secret deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete Secret");
    }
  };

  const redactedSecret = {
    ...secret,
    spec: {
      ...secret.spec,
      data: Object.fromEntries(dataKeys.map((k) => [k, "***"])),
      ...(secret.spec.stringData
        ? { stringData: Object.fromEntries(Object.keys(secret.spec.stringData).map((k) => [k, "***"])) }
        : {}),
    },
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{secret.metadata.name}</h1>
            <p className="page__subtitle">{secret.metadata.namespace ?? "default"}</p>
          </div>
          <StatusBadge phase={secret.status?.phase} size="md" />
          <CrdManagedBadge metadata={secret.metadata} />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Secret"}
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
              <StatusBadge phase={secret.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Source</span>
              <span className="detail-field__value">
                {sealedOwner ? (
                  <span className="badge badge--blue" title={`Managed by SealedSecret ${sealedOwner}`}>
                    <ShieldCheck size={12} /> Sealed — {sealedOwner}
                  </span>
                ) : (
                  "Manual"
                )}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Keys</span>
              <span className="detail-field__value">{dataKeys.length}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Key Names</span>
              <span className="detail-field__value mono">{dataKeys.join(", ") || "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Created At</span>
              <span className="detail-field__value">
                {secret.metadata.createdAt ? new Date(secret.metadata.createdAt).toLocaleString() : "-"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Namespace</span>
              <span className="detail-field__value">{secret.metadata.namespace ?? "default"}</span>
            </div>
            {secret.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{secret.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(redactedSecret, null, 2)}
            editable
            warning={isCrdManaged(secret.metadata) ? CRD_MANAGED_EDIT_WARNING : undefined}
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<Secret>(
                queryClient,
                "Secret",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<Secret>,
              );
              toast("success", "Secret updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.Secret}/${encodeURIComponent(updated.metadata.name)}`,
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
