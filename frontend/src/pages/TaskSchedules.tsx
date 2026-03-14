import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTaskSchedules } from "../api/hooks";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { CreateResourceDialog } from "../components/CreateResourceDialog";
import { CalendarClock, Plus } from "lucide-react";
import type { TaskSchedule } from "../api/types";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

export function TaskSchedules() {
  const { data, isLoading } = useTaskSchedules();
  const navigate = useNavigate();
  const [showCreate, setShowCreate] = useState(false);

  const schedules = data ?? [];

  const columns: Column<TaskSchedule>[] = [
    {
      key: "name",
      header: "Name",
      render: (r) => <span className="mono">{r.metadata.name}</span>,
    },
    {
      key: "taskRef",
      header: "Task Template",
      render: (r) => <span className="mono">{r.spec.task_ref ?? "-"}</span>,
    },
    {
      key: "schedule",
      header: "Schedule",
      render: (r) => (
        <span className="mono">
          {r.spec.schedule ?? "-"}
          {r.spec.time_zone ? ` (${r.spec.time_zone})` : ""}
        </span>
      ),
    },
    {
      key: "suspend",
      header: "Suspended",
      render: (r) => (r.spec.suspend ? "Yes" : "No"),
      width: "95px",
    },
    {
      key: "phase",
      header: "Status",
      render: (r) => <StatusBadge phase={r.status?.phase} />,
      width: "120px",
    },
    {
      key: "nextSchedule",
      header: "Next Schedule",
      render: (r) => formatDateTime(r.status?.nextScheduleTime),
    },
    {
      key: "activeRuns",
      header: "Active Runs",
      render: (r) => r.status?.activeRuns?.length ?? 0,
      width: "90px",
    },
  ];

  return (
    <div className="page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Task Schedules</h1>
          <p className="page__subtitle">{schedules.length} schedules</p>
        </div>
        <button className="btn-primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> New Schedule
        </button>
      </div>

      {schedules.length === 0 && !isLoading ? (
        <EmptyState
          icon={<CalendarClock size={40} />}
          title="No Task Schedules"
          description="Create recurring schedules to trigger Task templates on a cron expression."
        />
      ) : (
        <ResourceTable
          columns={columns}
          data={schedules}
          rowKey={(r) => r.metadata.name}
          onRowClick={(r) => navigate(`/task-schedules/${r.metadata.name}`)}
          loading={isLoading}
        />
      )}

      <CreateResourceDialog kind="TaskSchedule" open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
