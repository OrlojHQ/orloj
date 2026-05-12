import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDetailReturnNav } from "../hooks/useDetailReturnNav";
import { useEvalRun, useDeleteResource, useUpdateResource } from "../api/hooks";
import { useAppStore } from "../store";
import { saveNamespacedResourceYaml } from "../hooks/saveDetailYamlWithFreshRv";
import { StatusBadge } from "../components/StatusBadge";
import { MetricCard } from "../components/MetricCard";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceDetailLoadError } from "../components/ResourceDetailLoadError";
import { ArrowLeft, CheckCircle2, XCircle, Gauge, Timer, Coins } from "lucide-react";
import clsx from "clsx";
import { toast } from "../components/Toast";
import type { EvalRun } from "../api/types";
import { RESOURCE_DETAIL_BASE_PATH } from "../api/types";

type Tab = "overview" | "results" | "yaml";

export function EvalRunDetail() {
  const { name: nameParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { goBack } = useDetailReturnNav("/eval-runs");
  const routeName = nameParam ?? "";
  const { data: run, isLoading, isError, error } = useEvalRun(routeName);
  const queryClient = useQueryClient();
  const namespace = useAppStore((s) => s.namespace);
  const deleteMutation = useDeleteResource("EvalRun");
  const updateMutation = useUpdateResource("EvalRun");
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "results", label: "Results" },
    { id: "yaml", label: "YAML" },
  ];

  if (isError) {
    return (
      <ResourceDetailLoadError
        title="Eval run"
        message={error instanceof Error ? error.message : "Failed to load"}
        goBack={goBack}
      />
    );
  }

  if (isLoading || !run) {
    return (
      <div className="page">
        <div className="loading-placeholder">Loading eval run...</div>
      </div>
    );
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete EvalRun ${run.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(routeName);
      toast("success", "EvalRun deleted successfully");
      goBack();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete EvalRun");
    }
  };

  const summary = run.status?.summary;
  const results = run.status?.results ?? [];
  const passRate = summary?.pass_rate;
  const passRateStr = passRate != null ? `${(passRate * 100).toFixed(1)}%` : "—";
  const meanScoreStr = summary?.mean_score != null ? summary.mean_score.toFixed(3) : "—";
  const totalTokens = summary?.total_tokens ?? 0;
  const meanLatency = summary?.mean_latency_ms;
  const meanLatencyStr = meanLatency != null && meanLatency > 0
    ? meanLatency >= 1000
      ? `${(meanLatency / 1000).toFixed(1)}s`
      : `${meanLatency}ms`
    : "—";

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={goBack} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{run.metadata.name}</h1>
            <p className="page__subtitle">
              {run.spec.dataset_ref} → {run.spec.system} · {run.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={run.status?.phase} size="md" />
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
          <>
            <div className="metric-grid">
              <MetricCard
                label="Pass rate"
                value={passRateStr}
                icon={<CheckCircle2 size={16} />}
                variant={passRate != null ? (passRate >= 0.8 ? "green" : passRate >= 0.5 ? "yellow" : "red") : "default"}
                hint="Fraction of scored samples that passed"
              />
              <MetricCard
                label="Mean score"
                value={meanScoreStr}
                icon={<Gauge size={16} />}
                variant="blue"
                hint="Average score across all scored samples (0.0–1.0)"
              />
              <MetricCard
                label="Tokens"
                value={totalTokens.toLocaleString()}
                icon={<Coins size={16} />}
                hint="Total tokens consumed across all samples"
              />
              <MetricCard
                label="Avg latency"
                value={meanLatencyStr}
                icon={<Timer size={16} />}
                hint="Average execution time per sample"
              />
            </div>

            <div className="detail-grid" style={{ marginTop: "1.5rem" }}>
              <div className="detail-field">
                <span className="detail-field__label">Phase</span>
                <StatusBadge phase={run.status?.phase} size="md" />
              </div>
              <div className="detail-field">
                <span className="detail-field__label">Dataset</span>
                <span className="detail-field__value mono">{run.spec.dataset_ref}</span>
              </div>
              <div className="detail-field">
                <span className="detail-field__label">System</span>
                <span className="detail-field__value mono">{run.spec.system}</span>
              </div>
              <div className="detail-field">
                <span className="detail-field__label">Scoring</span>
                <span className="detail-field__value">{run.spec.scoring?.strategy || "exact_match"}</span>
              </div>
              <div className="detail-field">
                <span className="detail-field__label">Concurrency</span>
                <span className="detail-field__value">{run.spec.concurrency ?? 1}</span>
              </div>
              <div className="detail-field">
                <span className="detail-field__label">Progress</span>
                <span className="detail-field__value">
                  {run.status?.completedSamples ?? 0} / {run.status?.totalSamples ?? 0} samples
                </span>
              </div>
              {run.spec.scoring?.model_ref && (
                <div className="detail-field">
                  <span className="detail-field__label">Judge model</span>
                  <span className="detail-field__value mono">{run.spec.scoring.model_ref}</span>
                </div>
              )}
              {run.spec.timeout && (
                <div className="detail-field">
                  <span className="detail-field__label">Timeout</span>
                  <span className="detail-field__value">{run.spec.timeout}</span>
                </div>
              )}
              {run.status?.message && (
                <div className="detail-field detail-field--full">
                  <span className="detail-field__label">Message</span>
                  <span className="detail-field__value">{run.status.message}</span>
                </div>
              )}
            </div>

            {run.spec.agent_overrides && Object.keys(run.spec.agent_overrides).length > 0 && (
              <div style={{ marginTop: "1.5rem" }}>
                <h3 className="detail-section-title">Agent overrides</h3>
                <div className="detail-table-wrap">
                  <table className="detail-table">
                    <thead>
                      <tr>
                        <th>Agent</th>
                        <th>Model ref</th>
                        <th>Prompt</th>
                      </tr>
                    </thead>
                    <tbody>
                      {Object.entries(run.spec.agent_overrides).map(([agent, o]) => (
                        <tr key={agent}>
                          <td className="mono">{agent}</td>
                          <td className="mono text-secondary">{o.model_ref || "—"}</td>
                          <td className="text-secondary">
                            {o.prompt
                              ? o.prompt.length > 60
                                ? o.prompt.slice(0, 60) + "…"
                                : o.prompt
                              : "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </>
        )}

        {tab === "results" && (
          <div className="detail-table-wrap">
            {results.length === 0 ? (
              <p className="text-secondary" style={{ padding: "1rem" }}>No results yet.</p>
            ) : (
              <table className="detail-table">
                <thead>
                  <tr>
                    <th>Sample</th>
                    <th style={{ width: "70px" }}>Pass</th>
                    <th style={{ width: "80px" }}>Score</th>
                    <th>Output</th>
                    <th style={{ width: "90px" }}>Latency</th>
                    <th>Error</th>
                  </tr>
                </thead>
                <tbody>
                  {results.map((r) => (
                    <tr key={r.sample_name}>
                      <td className="mono">{r.sample_name}</td>
                      <td>
                        {r.pass != null ? (
                          r.pass ? (
                            <CheckCircle2 size={16} className="text-green" />
                          ) : (
                            <XCircle size={16} className="text-red" />
                          )
                        ) : (
                          "—"
                        )}
                      </td>
                      <td className="mono">
                        {r.score != null ? r.score.toFixed(3) : "—"}
                      </td>
                      <td className="text-secondary" title={r.output}>
                        {r.output
                          ? r.output.length > 100
                            ? r.output.slice(0, 100) + "…"
                            : r.output
                          : "—"}
                      </td>
                      <td className="mono text-secondary">{r.latency || "—"}</td>
                      <td className="text-red" title={r.error}>
                        {r.error
                          ? r.error.length > 60
                            ? r.error.slice(0, 60) + "…"
                            : r.error
                          : ""}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}

        {tab === "yaml" && (
          <YamlEditor
            value={JSON.stringify(run, null, 2)}
            editable
            onSave={async (body) => {
              const updated = await saveNamespacedResourceYaml<EvalRun>(
                queryClient,
                "EvalRun",
                namespace,
                routeName,
                body,
                (a) => updateMutation.mutateAsync(a) as Promise<EvalRun>,
              );
              toast("success", "Eval run updated");
              if (updated.metadata.name !== routeName) {
                navigate(
                  `${RESOURCE_DETAIL_BASE_PATH.EvalRun}/${encodeURIComponent(updated.metadata.name)}`,
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
