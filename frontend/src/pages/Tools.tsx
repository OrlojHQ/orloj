import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useTools } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Wrench, Plus } from "lucide-react";
import clsx from "clsx";
import type { Tool } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

const RISK_COLORS: Record<string, string> = {
  low: "text-green",
  medium: "text-yellow",
  high: "text-orange",
  critical: "text-red",
};

export function Tools() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading } = useTools();
  const [showCreate, setShowCreate] = useState(false);
  const tools = data ?? [];

  const columns: Column<Tool>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "type", header: "Type", render: (r) => r.spec.type ?? "http" },
    { key: "endpoint", header: "Endpoint", render: (r) => <span className="text-muted mono text-ellipsis">{r.spec.endpoint ?? "—"}</span> },
    {
      key: "risk",
      header: "Risk",
      render: (r) => <span className={clsx(RISK_COLORS[r.spec.risk_level ?? "low"])}>{r.spec.risk_level ?? "low"}</span>,
      width: "90px",
    },
    { key: "ops", header: "Operations", render: (r) => <span className="text-muted">{r.spec.operation_classes?.join(", ") || "—"}</span> },
    { key: "isolation", header: "Isolation", render: (r) => r.spec.runtime?.isolation_mode ?? "none", width: "100px" },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Tools</h1>
            <p className="page__subtitle">{tools.length} tools</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Tool
          </button>
        </div>
      </div>
      {tools.length === 0 && !isLoading ? (
        <EmptyState icon={<Wrench size={40} />} title="No Tools" description="Define external capabilities for agents to invoke." />
      ) : (
        <ResourceTable
          columns={columns}
          data={tools}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/tools/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="Tool" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
