import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useAgentRoles } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { KeyRound, Plus } from "lucide-react";
import type { AgentRole } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function Roles() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading } = useAgentRoles();
  const [showCreate, setShowCreate] = useState(false);
  const roles = data ?? [];

  const columns: Column<AgentRole>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "desc", header: "Description", render: (r) => r.spec.description || "—" },
    { key: "perms", header: "Permissions", render: (r) => r.spec.permissions?.join(", ") || "none" },
    { key: "namespace", header: "Namespace", render: (r) => <span className="text-muted">{r.metadata.namespace}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Agent Roles</h1>
            <p className="page__subtitle">{roles.length} roles</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Role
          </button>
        </div>
      </div>
      {roles.length === 0 && !isLoading ? (
        <EmptyState icon={<KeyRound size={40} />} title="No Roles" description="Permission grants bound to agents." />
      ) : (
        <ResourceTable
          columns={columns}
          data={roles}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/roles/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="AgentRole" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
