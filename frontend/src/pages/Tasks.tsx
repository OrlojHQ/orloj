import { useState, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useTasks } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { FilterPills } from "../components/FilterPills";
import { EmptyState } from "../components/EmptyState";
import { ListTodo, Plus } from "lucide-react";
import type { Task } from "../api/types";
import { CreateResourceDialog } from "../components/CreateResourceDialog";

const PHASES = ["All", "Pending", "Running", "Succeeded", "Failed", "DeadLetter"];

export function Tasks() {
  const { data, isLoading } = useTasks();
  const navigate = useNavigate();
  const [phaseFilter, setPhaseFilter] = useState("All");
  const [showTemplateTasks, setShowTemplateTasks] = useState(false);
  const [showCreate, setShowCreate] = useState(false);

  const tasks = data ?? [];
  const visibleTasks = useMemo(() => {
    if (showTemplateTasks) return tasks;
    return tasks.filter((t) => t.spec.mode !== "template");
  }, [tasks, showTemplateTasks]);
  const templateTaskCount = useMemo(
    () => tasks.filter((t) => t.spec.mode === "template").length,
    [tasks],
  );

  const phaseCounts = useMemo(() => {
    const counts: Record<string, number> = { All: visibleTasks.length };
    for (const p of PHASES.slice(1)) counts[p] = 0;
    for (const t of visibleTasks) {
      const phase = t.status?.phase ?? "Pending";
      counts[phase] = (counts[phase] ?? 0) + 1;
    }
    return counts;
  }, [visibleTasks]);

  const filtered = useMemo(() => {
    if (phaseFilter === "All") return visibleTasks;
    return visibleTasks.filter((t) => (t.status?.phase ?? "Pending") === phaseFilter);
  }, [visibleTasks, phaseFilter]);

  const columns: Column<Task>[] = [
    { key: "name", header: "Name", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "system", header: "System", render: (r) => r.spec.system ?? "—" },
    { key: "phase", header: "Phase", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
    { key: "worker", header: "Worker", render: (r) => <span className="text-muted">{r.status?.assignedWorker ?? "—"}</span> },
    { key: "attempts", header: "Attempts", render: (r) => r.status?.attempts ?? 0, width: "90px" },
    { key: "priority", header: "Priority", render: (r) => r.spec.priority ?? "normal", width: "90px" },
    { key: "created", header: "Created", render: (r) => r.metadata.createdAt ? new Date(r.metadata.createdAt).toLocaleString() : "—" },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Tasks</h1>
          <p className="page__subtitle">{visibleTasks.length} tasks</p>
        </div>
        <div className="page__header-actions">
          <label className="checkbox-inline">
            <input
              type="checkbox"
              checked={showTemplateTasks}
              onChange={(e) => setShowTemplateTasks(e.target.checked)}
            />
            <span>Show template tasks</span>
            {!showTemplateTasks && templateTaskCount > 0 && (
              <span className="text-muted">({templateTaskCount} hidden)</span>
            )}
          </label>
          <button className="btn-primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> New Task
          </button>
        </div>
      </div>

      <FilterPills
        options={PHASES.map((p) => ({ value: p, label: p, count: phaseCounts[p] ?? 0 }))}
        selected={phaseFilter}
        onSelect={setPhaseFilter}
      />

      {filtered.length === 0 && !isLoading ? (
        <EmptyState
          icon={<ListTodo size={40} />}
          title={phaseFilter === "All" ? "No Tasks" : `No ${phaseFilter} Tasks`}
          description={
            !showTemplateTasks && templateTaskCount > 0
              ? "No runnable tasks match the filter. Enable template tasks to view task templates."
              : "Tasks are execution requests routed to agent systems."
          }
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={filtered}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/tasks/${r.metadata.name}`)}
          loading={isLoading}
        />
      )}
      <CreateResourceDialog kind="Task" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
