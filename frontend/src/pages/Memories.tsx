import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useMemories } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Brain, Plus } from "lucide-react";
import type { Memory } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function Memories() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading } = useMemories();
  const [showCreate, setShowCreate] = useState(false);
  const memories = data ?? [];

  const columns: Column<Memory>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "type", header: "Type", render: (r) => r.spec.type ?? "—" },
    { key: "provider", header: "Provider", render: (r) => r.spec.provider ?? "—" },
    { key: "embedding", header: "Embedding Model", render: (r) => <span className="mono">{r.spec.embedding_model ?? "—"}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Memories</h1>
            <p className="page__subtitle">{memories.length} memories</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Memory
          </button>
        </div>
      </div>
      {memories.length === 0 && !isLoading ? (
        <EmptyState icon={<Brain size={40} />} title="No Memories" description="Persistent memory configurations for agents." />
      ) : (
        <ResourceTable
          columns={columns}
          data={memories}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/memories/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="Memory" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
