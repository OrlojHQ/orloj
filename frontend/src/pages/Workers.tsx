import { useNavigate } from "react-router-dom";
import { useWorkers } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Cpu } from "lucide-react";
import type { Worker } from "../api/types";

export function Workers() {
  const navigate = useNavigate();
  const { data, isLoading } = useWorkers();
  const workers = data ?? [];

  const columns: Column<Worker>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "region", header: "Region", render: (r) => r.spec.region || "—" },
    { key: "gpu", header: "GPU", render: (r) => r.spec.capabilities?.gpu ? "Yes" : "No", width: "70px" },
    {
      key: "capacity",
      header: "Capacity",
      render: (r) => (
        <div className="capacity-bar">
          <div className="capacity-bar__fill" style={{ width: `${Math.min(((r.status?.currentTasks ?? 0) / Math.max(r.spec.max_concurrent_tasks ?? 1, 1)) * 100, 100)}%` }} />
          <span className="capacity-bar__label">{r.status?.currentTasks ?? 0}/{r.spec.max_concurrent_tasks ?? 1}</span>
        </div>
      ),
      width: "150px",
    },
    {
      key: "heartbeat",
      header: "Last Heartbeat",
      render: (r) => r.status?.lastHeartbeat ? new Date(r.status.lastHeartbeat).toLocaleString() : "—",
    },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Workers</h1>
          <p className="page__subtitle">{workers.length} workers</p>
        </div>
      </div>
      {workers.length === 0 && !isLoading ? (
        <EmptyState icon={<Cpu size={40} />} title="No Workers" description="Workers claim and execute tasks from the queue." />
      ) : (
        <ResourceTable columns={columns} data={workers} rowKey={(r) => r.metadata.name} onRowClick={(r) => navigate(`/workers/${r.metadata.name}`)} loading={isLoading} />
      )}
    </div>
  );
}
