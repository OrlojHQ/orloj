import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAgents } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Bot, Plus } from "lucide-react";
import type { Agent } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function Agents() {
  const { data, isLoading } = useAgents();
  const navigate = useNavigate();
  const [showCreate, setShowCreate] = useState(false);
  const agents = data ?? [];

  const columns: Column<Agent>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "model", header: "Model Ref", render: (r) => r.spec.model_ref || "—" },
    { key: "tools", header: "Tools", render: (r) => r.spec.tools?.length ?? 0, width: "80px" },
    { key: "roles", header: "Roles", render: (r) => r.spec.roles?.length ?? 0, width: "80px" },
    { key: "maxSteps", header: "Max Steps", render: (r) => r.spec.limits?.max_steps ?? 10, width: "100px" },
    { key: "namespace", header: "Namespace", render: (r) => <span className="text-muted">{r.metadata.namespace}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  if (agents.length === 0 && !isLoading) {
    return (
      <div className="page">
        <div className="page__header">
          <div>
            <h1 className="page__title">Agents</h1>
          </div>
          <div className="page__header-actions">
            <button className="btn-primary" onClick={() => setShowCreate(true)}>
              <Plus size={14} /> New Agent
            </button>
          </div>
        </div>
        <EmptyState
          icon={<Bot size={40} />}
          title="No Agents"
          description="Create agents to define individual AI runtime configurations."
        />
        <CreateResourceDialog kind="Agent" open={showCreate} onClose={() => setShowCreate(false)} />
      </div>
    );
  }

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Agents</h1>
          <p className="page__subtitle">{agents.length} agents</p>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Agent
          </button>
        </div>
      </div>
      <ResourceTable
        columns={columns}
        data={agents}
        rowKey={(r) => r.metadata.name}
        onRowClick={(r) => navigate(`/agents/${r.metadata.name}`)}
        loading={isLoading}
      />
      <CreateResourceDialog kind="Agent" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
