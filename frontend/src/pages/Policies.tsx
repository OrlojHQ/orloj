import { useAgentPolicies } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Shield } from "lucide-react";
import type { AgentPolicy } from "../api/types";

export function Policies() {
  const { data, isLoading } = useAgentPolicies();
  const policies = data ?? [];

  const columns: Column<AgentPolicy>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "mode", header: "Apply Mode", render: (r) => r.spec.apply_mode ?? "scoped" },
    { key: "tokens", header: "Max Tokens", render: (r) => r.spec.max_tokens_per_run ?? "—", width: "110px" },
    { key: "models", header: "Allowed Models", render: (r) => r.spec.allowed_models?.join(", ") || "any" },
    { key: "blocked", header: "Blocked Tools", render: (r) => r.spec.blocked_tools?.join(", ") || "none" },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Agent Policies</h1>
          <p className="page__subtitle">{policies.length} policies</p>
        </div>
      </div>
      {policies.length === 0 && !isLoading ? (
        <EmptyState icon={<Shield size={40} />} title="No Policies" description="Governance rules for agent runtime behavior." />
      ) : (
        <ResourceTable columns={columns} data={policies} rowKey={(r) => r.metadata.name} loading={isLoading} />
      )}
    </div>
  );
}
