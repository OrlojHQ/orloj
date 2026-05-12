import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useEvalRuns } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { FlaskConical, Plus } from "lucide-react";
import type { EvalRun } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function EvalRuns() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading } = useEvalRuns();
  const [showCreate, setShowCreate] = useState(false);
  const runs = data ?? [];

  const columns: Column<EvalRun>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    {
      key: "dataset",
      header: "Dataset",
      render: (r) => <span className="mono text-secondary">{r.spec.dataset_ref}</span>,
    },
    {
      key: "system",
      header: "System",
      render: (r) => <span className="mono text-secondary">{r.spec.system}</span>,
    },
    {
      key: "phase",
      header: "Phase",
      render: (r) => <StatusBadge phase={r.status?.phase} />,
      width: "140px",
    },
    {
      key: "pass_rate",
      header: "Pass rate",
      render: (r) => {
        const pr = r.status?.summary?.pass_rate;
        if (pr == null) return "—";
        return `${(pr * 100).toFixed(1)}%`;
      },
      width: "100px",
    },
    {
      key: "samples",
      header: "Samples",
      render: (r) => {
        const total = r.status?.totalSamples ?? 0;
        const completed = r.status?.completedSamples ?? 0;
        return total > 0 ? `${completed}/${total}` : "—";
      },
      width: "100px",
    },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Eval runs</h1>
            <p className="page__subtitle">{runs.length} runs</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New run
          </button>
        </div>
      </div>
      {runs.length === 0 && !isLoading ? (
        <EmptyState
          icon={<FlaskConical size={40} />}
          title="No eval runs"
          description="Run an evaluation against a dataset and agent system to measure quality."
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={runs}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) =>
            navigate(`/eval-runs/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))
          }
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="EvalRun" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
