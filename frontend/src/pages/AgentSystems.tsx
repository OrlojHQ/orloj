import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAgentSystems } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Network, Grid3X3, List } from "lucide-react";
import clsx from "clsx";
import type { AgentSystem } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";
import { Plus } from "lucide-react";

type ViewMode = "cards" | "table";

export function AgentSystems() {
  const { data, isLoading } = useAgentSystems();
  const navigate = useNavigate();
  const [view, setView] = useState<ViewMode>("cards");
  const [showCreate, setShowCreate] = useState(false);

  const systems = data ?? [];

  const columns: Column<AgentSystem>[] = [
    {
      key: "name",
      header: "Name",
      render: (r) => <span className="mono">{r.metadata.name}</span>,
    },
    {
      key: "agents",
      header: "Agents",
      render: (r) => r.spec.agents?.length ?? 0,
      width: "100px",
    },
    {
      key: "edges",
      header: "Edges",
      render: (r) => Object.keys(r.spec.graph ?? {}).length,
      width: "100px",
    },
    {
      key: "namespace",
      header: "Namespace",
      render: (r) => <span className="text-muted">{r.metadata.namespace}</span>,
    },
    {
      key: "phase",
      header: "Status",
      render: (r) => <StatusBadge phase={r.status?.phase} />,
      width: "120px",
    },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Agent Systems</h1>
          <p className="page__subtitle">{systems.length} systems</p>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New System
          </button>
          <div className="view-toggle">
            <button
              className={clsx("view-toggle__btn", view === "cards" && "view-toggle__btn--active")}
              onClick={() => setView("cards")}
              aria-label="Card view"
            >
              <Grid3X3 size={16} />
            </button>
            <button
              className={clsx("view-toggle__btn", view === "table" && "view-toggle__btn--active")}
              onClick={() => setView("table")}
              aria-label="Table view"
            >
              <List size={16} />
            </button>
          </div>
        </div>
      </div>

      {systems.length === 0 && !isLoading ? (
        <EmptyState
          icon={<Network size={40} />}
          title="No Agent Systems"
          description="Create an agent system to define multi-agent architectures and execution graphs."
        />
      ) : view === "cards" ? (
        <div className="card-grid">
          {systems.map((sys) => (
            <div
              key={sys.metadata.name}
              className="resource-card"
              onClick={() => navigate(`/systems/${sys.metadata.name}`)}
            >
              <div className="resource-card__header">
                <Network size={16} className="resource-card__icon" />
                <StatusBadge phase={sys.status?.phase} />
              </div>
              <h3 className="resource-card__name">{sys.metadata.name}</h3>
              <div className="resource-card__meta">
                <span>{sys.spec.agents?.length ?? 0} agents</span>
                <span>{Object.keys(sys.spec.graph ?? {}).length} edges</span>
              </div>
              {sys.metadata.namespace && (
                <span className="resource-card__ns">{sys.metadata.namespace}</span>
              )}
            </div>
          ))}
        </div>
      ) : (
        <ResourceTable
          columns={columns}
          data={systems}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/systems/${r.metadata.name}`)}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="AgentSystem" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
