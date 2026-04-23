import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useSealedSecrets } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { ListFetchError } from "../components/ListFetchError";
import { Lock, Plus } from "lucide-react";
import type { SealedSecret } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function SealedSecrets() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading, isError, error, refetch } = useSealedSecrets();
  const [showCreate, setShowCreate] = useState(false);
  const sealedSecrets = data ?? [];

  const columns: Column<SealedSecret>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "keys", header: "Keys", render: (r) => Object.keys(r.spec.encryptedData ?? {}).length },
    {
      key: "keyNames",
      header: "Key Names",
      render: (r) => <span className="text-muted">{Object.keys(r.spec.encryptedData ?? {}).join(", ") || "—"}</span>,
    },
    {
      key: "template",
      header: "Template",
      render: (r) => {
        const labelCount = Object.keys(r.spec.template?.labels ?? {}).length;
        const annotationCount = Object.keys(r.spec.template?.annotations ?? {}).length;
        return <span className="text-muted">{labelCount} labels, {annotationCount} annotations</span>;
      },
    },
    { key: "namespace", header: "Namespace", render: (r) => <span className="text-muted">{r.metadata.namespace}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Sealed Secrets</h1>
            <p className="page__subtitle">{sealedSecrets.length} sealed secrets</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Sealed Secret
          </button>
        </div>
      </div>
      {isError && (
        <ListFetchError
          message={error instanceof Error ? error.message : "Failed to load sealed secrets"}
          onRetry={() => void refetch()}
        />
      )}

      {sealedSecrets.length === 0 && !isLoading && !isError ? (
        <EmptyState icon={<Lock size={40} />} title="No Sealed Secrets" description="Git-safe encrypted secrets that reconcile into normal Secret resources." />
      ) : (
        <ResourceTable
          columns={columns}
          data={sealedSecrets}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/sealed-secrets/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="SealedSecret" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
