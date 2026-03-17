import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useSecrets } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Lock, Plus } from "lucide-react";
import type { Secret } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function Secrets() {
  const navigate = useNavigate();
  const { data, isLoading } = useSecrets();
  const [showCreate, setShowCreate] = useState(false);
  const secrets = data ?? [];

  const columns: Column<Secret>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "keys", header: "Keys", render: (r) => Object.keys(r.spec.data ?? {}).length },
    {
      key: "keyNames",
      header: "Key Names",
      render: (r) => <span className="text-muted">{Object.keys(r.spec.data ?? {}).join(", ") || "—"}</span>,
    },
    { key: "namespace", header: "Namespace", render: (r) => <span className="text-muted">{r.metadata.namespace}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Secrets</h1>
          <p className="page__subtitle">{secrets.length} secrets</p>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Secret
          </button>
        </div>
      </div>
      {secrets.length === 0 && !isLoading ? (
        <EmptyState icon={<Lock size={40} />} title="No Secrets" description="Secrets store sensitive values for tool authentication." />
      ) : (
        <ResourceTable columns={columns} data={secrets} rowKey={(r) => r.metadata.name} onRowClick={(r) => navigate(`/secrets/${r.metadata.name}`)} loading={isLoading} />
      )}
      <CreateResourceDialog kind="Secret" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
