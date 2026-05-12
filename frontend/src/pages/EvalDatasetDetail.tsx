import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useEvalDataset, useDeleteResource, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { EvalDataset } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

type Tab = "overview" | "samples" | "yaml";

export function EvalDatasetDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/eval-datasets");
  const routeName = nameParam ?? "";
  const { data: dataset, isLoading, isError, error } = useEvalDataset(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("EvalDataset");
  const updateMutation = useUpdateResource("EvalDataset");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "samples", label: "Samples" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Eval dataset"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !dataset) {
    return (
      <div className="page">
        <div className="loading-placeholder">Loading eval dataset...</div>
      </div>
    );
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete EvalDataset ${dataset.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "EvalDataset deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete EvalDataset");
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
            <h1 className="page__title">{dataset.metadata.name}</h1>
            <p className="page__subtitle">
              {dataset.spec.samples?.length ?? 0} samples · {dataset.metadata.namespace}
            </p>
          </div>
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete"}
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
              <span className="detail-field__label">Samples</span>
              <span className="detail-field__value">{dataset.spec.samples?.length ?? 0}</span>
            </div>
            {dataset.spec.description && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Description</span>
                <span className="detail-field__value">{dataset.spec.description}</span>
              </div>
            )}
          </div>
        )}
        {tab === "samples" && (
          <div className="detail-table-wrap">
            <table className="detail-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Input</th>
                  <th>Expected</th>
                  <th>Scoring</th>
                </tr>
              </thead>
              <tbody>
                {(dataset.spec.samples ?? []).map((s) => (
                  <tr key={s.name}>
                    <td className="mono">{s.name}</td>
                    <td className="mono text-secondary" title={JSON.stringify(s.input)}>
                      {Object.entries(s.input ?? {})
                        .map(([k, v]) => `${k}: ${v}`)
                        .join(", ")
                        .slice(0, 80) || "—"}
                    </td>
                    <td className="mono text-secondary">
                      {s.expected?.output_contains
                        ? `contains "${s.expected.output_contains}"`
                        : s.expected?.output_matches
                          ? `matches /${s.expected.output_matches}/`
                          : s.expected?.output_json_path
                            ? `${s.expected.output_json_path} ${s.expected.equals ? `= ${s.expected.equals}` : ""}`
                            : "—"}
                    </td>
                    <td>{s.scoring?.strategy ?? "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(dataset, null, 2)}
            editable
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<EvalDataset>(
                queryClient,
                "EvalDataset",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<EvalDataset>,
              );
              toast("success", "Eval dataset updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.EvalDataset}/${encodeURIComponent(updated.metadata.name)}`,
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
