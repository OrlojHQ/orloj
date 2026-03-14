import { useModelEndpoints } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Database } from "lucide-react";
import type { ModelEndpoint } from "../api/types";

export function ModelEndpoints() {
  const { data, isLoading } = useModelEndpoints();
  const endpoints = data ?? [];

  const columns: Column<ModelEndpoint>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "provider", header: "Provider", render: (r) => r.spec.provider ?? "—" },
    { key: "model", header: "Default Model", render: (r) => <span className="mono">{r.spec.default_model ?? "—"}</span> },
    { key: "url", header: "Base URL", render: (r) => <span className="text-muted">{r.spec.base_url ?? "—"}</span> },
    { key: "auth", header: "Auth", render: (r) => r.spec.auth?.secretRef ? <span className="mono">{r.spec.auth.secretRef}</span> : "—" },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Model Endpoints</h1>
          <p className="page__subtitle">{endpoints.length} endpoints</p>
        </div>
      </div>
      {endpoints.length === 0 && !isLoading ? (
        <EmptyState icon={<Database size={40} />} title="No Model Endpoints" description="Configure LLM provider connections." />
      ) : (
        <ResourceTable columns={columns} data={endpoints} rowKey={(r) => r.metadata.name} loading={isLoading} />
      )}
    </div>
  );
}
