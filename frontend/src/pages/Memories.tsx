import { useMemories } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Brain } from "lucide-react";
import type { Memory } from "../api/types";

export function Memories() {
  const { data, isLoading } = useMemories();
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
        <div>
          <h1 className="page__title">Memories</h1>
          <p className="page__subtitle">{memories.length} memories</p>
        </div>
      </div>
      {memories.length === 0 && !isLoading ? (
        <EmptyState icon={<Brain size={40} />} title="No Memories" description="Persistent memory configurations for agents." />
      ) : (
        <ResourceTable columns={columns} data={memories} rowKey={(r) => r.metadata.name} loading={isLoading} />
      )}
    </div>
  );
}
