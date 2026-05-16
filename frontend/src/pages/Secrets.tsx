import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ContextBackButton } from "../components/ContextBackButton";
import { detailListNavState } from "../hooks/useDetailReturnNav";
import { useSecrets } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { ListFetchError } from "../components/ListFetchError";
import { Lock, Plus, ShieldCheck } from "lucide-react";
import type { Secret } from "../api/types";
import { CrdManagedBadge } from "../components/CrdManagedBadge";

const SEALED_OWNER_ANNOTATION = "orloj.dev/sealedsecret-owner";

function SourceBadge({ secret }: { secret: Secret }) {
  const owner = secret.metadata.annotations?.[SEALED_OWNER_ANNOTATION];
  if (!owner) return <span className="text-muted">Manual</span>;
  return (
    <span className="badge badge--blue" title={`Managed by SealedSecret ${owner}`}>
      <ShieldCheck size={12} /> Sealed
    </span>
  );
}
import { CreateResourceDialog } from "../components/CreateResourceDialog";

export function Secrets() {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading, isError, error, refetch } = useSecrets();
  const [showCreate, setShowCreate] = useState(false);
  const secrets = data ?? [];

  const columns: Column<Secret>[] = [
    { key: "name", header: "Name", render: (r) => <><span className="mono">{r.metadata.name}</span> <CrdManagedBadge metadata={r.metadata} /></> },
    {
      key: "source",
      header: "Source",
      render: (r) => <SourceBadge secret={r} />,
      width: "100px",
    },
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
        <div className="page__header-back">
          <ContextBackButton />
          <div>
            <h1 className="page__title">Secrets</h1>
            <p className="page__subtitle">{secrets.length} secrets</p>
          </div>
        </div>
        <div className="page__header-actions">
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Secret
          </button>
        </div>
      </div>
      {isError && (
        <ListFetchError
          message={error instanceof Error ? error.message : "Failed to load secrets"}
          onRetry={() => void refetch()}
        />
      )}

      {secrets.length === 0 && !isLoading && !isError ? (
        <EmptyState icon={<Lock size={40} />} title="No Secrets" description="Secrets store sensitive values for tool authentication." />
      ) : (
        <ResourceTable
          columns={columns}
          data={secrets}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/secrets/${encodeURIComponent(r.metadata.name)}`, detailListNavState(location))}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="Secret" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
