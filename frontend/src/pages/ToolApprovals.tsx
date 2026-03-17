import { useState, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useToolApprovals } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { FilterPills } from "../components/FilterPills";
import { EmptyState } from "../components/EmptyState";
import { ShieldCheck, Plus } from "lucide-react";
import type { ToolApproval } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

const PHASES = ["All", "Pending", "Approved", "Denied", "Expired"];

export function ToolApprovals() {
  const { data, isLoading } = useToolApprovals();
  const navigate = useNavigate();
  const [phaseFilter, setPhaseFilter] = useState("All");
  const [showCreate, setShowCreate] = useState(false);
  const approvals = data ?? [];

  const phaseCounts = useMemo(() => {
    const counts: Record<string, number> = { All: approvals.length };
    for (const p of PHASES.slice(1)) counts[p] = 0;
    for (const a of approvals) {
      const phase = a.status?.phase ?? "Pending";
      counts[phase] = (counts[phase] ?? 0) + 1;
    }
    return counts;
  }, [approvals]);

  const filtered = useMemo(() => {
    if (phaseFilter === "All") return approvals;
    return approvals.filter((a) => (a.status?.phase ?? "Pending") === phaseFilter);
  }, [approvals, phaseFilter]);

  const columns: Column<ToolApproval>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "tool", header: "Tool", render: (r) => <span className="mono">{r.spec.tool ?? "—"}</span> },
    { key: "opClass", header: "Operation", render: (r) => r.spec.operation_class ?? "—", width: "110px" },
    { key: "agent", header: "Agent", render: (r) => r.spec.agent ?? "—" },
    { key: "task", header: "Task", render: (r) => <span className="mono text-muted">{r.spec.task_ref ?? "—"}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
    { key: "created", header: "Created", render: (r) => r.metadata.createdAt ? new Date(r.metadata.createdAt).toLocaleString() : "—" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Approvals</h1>
          <p className="page__subtitle">{approvals.length} tool approvals</p>
        </div>
        <button className="btn-primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> New Approval
        </button>
      </div>

      <FilterPills
        options={PHASES.map((p) => ({ value: p, label: p, count: phaseCounts[p] ?? 0 }))}
        selected={phaseFilter}
        onSelect={setPhaseFilter}
      />

      {filtered.length === 0 && !isLoading ? (
        <EmptyState
          icon={<ShieldCheck size={40} />}
          title={phaseFilter === "All" ? "No Approvals" : `No ${phaseFilter} Approvals`}
          description="Tool approvals are created when an agent invokes a tool that requires human authorization."
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={filtered}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/approvals/${r.metadata.name}`)}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="ToolApproval" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
