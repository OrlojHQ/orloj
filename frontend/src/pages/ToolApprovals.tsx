import { useState, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useToolApprovals, useTaskApprovals } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { FilterPills } from "../components/FilterPills";
import { EmptyState } from "../components/EmptyState";
import { ShieldCheck, Plus } from "lucide-react";
import type { ToolApproval, TaskApproval } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

const PHASES = ["All", "Pending", "Approved", "Denied", "ChangesRequested", "Expired"];

type ApprovalRow =
  | { type: "tool"; approval: ToolApproval }
  | { type: "task"; approval: TaskApproval };

export function ToolApprovals() {
  const { data: toolApprovals, isLoading: toolsLoading } = useToolApprovals();
  const { data: taskApprovals, isLoading: tasksLoading } = useTaskApprovals();
  const navigate = useNavigate();
  const [phaseFilter, setPhaseFilter] = useState("All");
  const [showCreate, setShowCreate] = useState(false);

  const approvals = useMemo<ApprovalRow[]>(
    () => [
      ...(toolApprovals ?? []).map((approval) => ({ type: "tool" as const, approval })),
      ...(taskApprovals ?? []).map((approval) => ({ type: "task" as const, approval })),
    ],
    [toolApprovals, taskApprovals],
  );

  const phaseCounts = useMemo(() => {
    const counts: Record<string, number> = { All: approvals.length };
    for (const p of PHASES.slice(1)) counts[p] = 0;
    for (const item of approvals) {
      const phase = item.approval.status?.phase ?? "Pending";
      counts[phase] = (counts[phase] ?? 0) + 1;
    }
    return counts;
  }, [approvals]);

  const filtered = useMemo(() => {
    if (phaseFilter === "All") return approvals;
    return approvals.filter((item) => (item.approval.status?.phase ?? "Pending") === phaseFilter);
  }, [approvals, phaseFilter]);

  const a2aInputRows = useMemo(
    () => filtered.filter((r) => r.approval.metadata.labels?.["orloj.dev/blocked-reason"] === "a2a-input-required"),
    [filtered],
  );
  const regularRows = useMemo(
    () => filtered.filter((r) => r.approval.metadata.labels?.["orloj.dev/blocked-reason"] !== "a2a-input-required"),
    [filtered],
  );

  const columns: Column<ApprovalRow>[] = [
    {
      key: "name",
      header: "Name",
      render: (r) => (
        <span className="mono">
          {r.approval.metadata.name}
          {r.approval.metadata.labels?.["orloj.dev/blocked-reason"] === "a2a-input-required" && (
            <span className="badge badge--blue" style={{ marginLeft: "0.5rem" }}>A2A Input</span>
          )}
        </span>
      ),
    },
    { key: "type", header: "Type", render: (r) => (r.type === "tool" ? "Tool" : "Task"), width: "90px" },
    {
      key: "subject",
      header: "Subject",
      render: (r) =>
        r.type === "tool"
          ? <span className="mono">{r.approval.spec.tool ?? "—"}</span>
          : <span className="mono">{r.approval.spec.checkpoint_id ?? "—"}</span>,
    },
    {
      key: "detail",
      header: "Detail",
      render: (r) => (r.type === "tool" ? r.approval.spec.operation_class ?? "—" : r.approval.spec.checkpoint_type ?? "—"),
      width: "130px",
    },
    {
      key: "agent",
      header: "Agent",
      render: (r) => (r.type === "tool" ? r.approval.spec.agent ?? "—" : r.approval.spec.agent ?? "—"),
    },
    {
      key: "task",
      header: "Task",
      render: (r) => <span className="mono text-muted">{r.approval.spec.task_ref ?? "—"}</span>,
    },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.approval.status?.phase} />, width: "140px" },
    {
      key: "created",
      header: "Created",
      render: (r) => (r.approval.metadata.createdAt ? new Date(r.approval.metadata.createdAt).toLocaleString() : "—"),
    },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Approvals</h1>
          <p className="page__subtitle">{approvals.length} tool and task approvals</p>
        </div>
        <button className="btn-primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> New Tool Approval
        </button>
      </div>

      <FilterPills
        options={PHASES.map((p) => ({ value: p, label: p, count: phaseCounts[p] ?? 0 }))}
        selected={phaseFilter}
        onSelect={setPhaseFilter}
      />

      {filtered.length === 0 && !toolsLoading && !tasksLoading ? (
        <EmptyState
          icon={<ShieldCheck size={40} />}
          title={phaseFilter === "All" ? "No Approvals" : `No ${phaseFilter} Approvals`}
          description="Approvals are created when tool calls or review checkpoints need human authorization."
        />
      ) : (
        <>
          {a2aInputRows.length > 0 && (
            <>
              <h2 className="section-heading">A2A Input Requests</h2>
              <ResourceTable
                columns={columns}
                data={a2aInputRows}
                rowKey={(r) => `${r.type}:${r.approval.metadata.name}`}
                onRowClick={(r) => navigate(r.type === "tool" ? `/approvals/${r.approval.metadata.name}` : `/approvals/task/${r.approval.metadata.name}`)}
                loading={toolsLoading || tasksLoading}
              />
            </>
          )}
          {regularRows.length > 0 && (
            <>
              {a2aInputRows.length > 0 && <h2 className="section-heading">Approvals</h2>}
              <ResourceTable
                columns={columns}
                data={regularRows}
                rowKey={(r) => `${r.type}:${r.approval.metadata.name}`}
                onRowClick={(r) => navigate(r.type === "tool" ? `/approvals/${r.approval.metadata.name}` : `/approvals/task/${r.approval.metadata.name}`)}
                loading={toolsLoading || tasksLoading}
              />
            </>
          )}
        </>
      )}
      <CreateResourceDialog kind="ToolApproval" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
