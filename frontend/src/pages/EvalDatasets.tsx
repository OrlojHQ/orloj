import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useEvalDatasets } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { EmptyState } from "../components/EmptyState";
import { ClipboardList, Plus } from "lucide-react";
import type { EvalDataset } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function EvalDatasets() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading } = useEvalDatasets();
  const [showCreate, setShowCreate] = useState(false);
  const datasets = data ?? [];

  const columns: Column<EvalDataset>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    {
      key: "samples",
      header: "Samples",
      render: (r) => r.spec.samples?.length ?? 0,
      width: "100px",
    },
    {
      key: "description",
      header: "Description",
      render: (r) => (
        <span className="text-secondary" title={r.spec.description}>
          {r.spec.description
            ? r.spec.description.length > 80
              ? r.spec.description.slice(0, 80) + "…"
              : r.spec.description
            : "—"}
        </span>
      ),
    },
    {
      key: "namespace",
      header: "Namespace",
      render: (r) => <span className="mono text-secondary">{r.metadata.namespace}</span>,
      width: "120px",
    },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Eval datasets</h1>
            <p className="page__subtitle">{datasets.length} datasets</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New dataset
          </button>
        </div>
      </div>
      {datasets.length === 0 && !isLoading ? (
        <EmptyState
          icon={<ClipboardList size={40} />}
          title="No eval datasets"
          description="Create a dataset with golden (input, expected output) samples to evaluate your agent systems."
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={datasets}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) =>
            navigate(`/eval-datasets/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))
          }
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="EvalDataset" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
