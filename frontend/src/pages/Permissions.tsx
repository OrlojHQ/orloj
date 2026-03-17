import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useToolPermissions } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { KeyRound, Plus } from "lucide-react";
import type { ToolPermission } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function Permissions() {
  const navigate = useNavigate();
  const { data, isLoading } = useToolPermissions();
  const [showCreate, setShowCreate] = useState(false);
  const permissions = data ?? [];

  const columns: Column<ToolPermission>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "tool", header: "Tool Ref", render: (r) => <span className="mono">{r.spec.tool_ref ?? "—"}</span> },
    { key: "action", header: "Action", render: (r) => r.spec.action ?? "invoke" },
    { key: "match", header: "Match Mode", render: (r) => r.spec.match_mode ?? "all" },
    { key: "apply", header: "Apply Mode", render: (r) => r.spec.apply_mode ?? "global" },
    { key: "required", header: "Required Permissions", render: (r) => r.spec.required_permissions?.join(", ") || "—" },
    { key: "rules", header: "Op Rules", render: (r) => <span className="text-muted">{r.spec.operation_rules?.length ?? 0}</span>, width: "90px" },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Tool Permissions</h1>
          <p className="page__subtitle">{permissions.length} permissions</p>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Permission
          </button>
        </div>
      </div>
      {permissions.length === 0 && !isLoading ? (
        <EmptyState icon={<KeyRound size={40} />} title="No Permissions" description="Access control rules for tool invocation." />
      ) : (
        <ResourceTable columns={columns} data={permissions} rowKey={(r) => r.metadata.name} onRowClick={(r) => navigate(`/permissions/${r.metadata.name}`)} loading={isLoading} />
      )}
      <CreateResourceDialog kind="ToolPermission" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
