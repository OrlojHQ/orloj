import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useContextAdapters } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Filter, Plus } from "lucide-react";
import type { ContextAdapter } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function ContextAdapters() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading } = useContextAdapters();
  const [showCreate, setShowCreate] = useState(false);
  const adapters = data ?? [];

  const columns: Column<ContextAdapter>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    {
      key: "tool",
      header: "Tool ref",
      render: (r) => <span className="mono">{r.spec.tool_ref}</span>,
    },
    {
      key: "on_error",
      header: "On error",
      render: (r) => r.spec.on_error ?? "reject",
      width: "100px",
    },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Context adapters</h1>
            <p className="page__subtitle">{adapters.length} context adapters</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Context adapter
          </button>
        </div>
      </div>
      {adapters.length === 0 && !isLoading ? (
        <EmptyState
          icon={<Filter size={40} />}
          title="No context adapters"
          description="Tool-backed sanitization for task input before the first agent runs."
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={adapters}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) =>
            navigate(`/context-adapters/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))
          }
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="ContextAdapter" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
