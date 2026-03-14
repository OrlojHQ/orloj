import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useDeleteResource, useTaskSchedule, useTasks } from "../api/hooks";
import { StatusBadge } from "../components/StatusBadge";
import { YamlEditor } from "../components/YamlEditor";
import { ResourceTable, type Column } from "../components/ResourceTable";
import { ArrowLeft } from "lucide-react";
import clsx from "clsx";
import type { Task } from "../api/types";
import { toast } from "../components/Toast";

type Tab = "overview" | "runs" | "yaml";

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

export function TaskScheduleDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: taskSchedule, isLoading } = useTaskSchedule(name ?? "");
  const tasks = useTasks();
  const deleteMutation = useDeleteResource("TaskSchedule");
  const [tab, setTab] = useState<Tab>("overview");

  const scheduleNamespace = taskSchedule?.metadata.namespace ?? "default";

  const runs = useMemo(() => {
    if (!taskSchedule) return [];

    return (tasks.data ?? [])
      .filter((task) => {
        const labels = task.metadata.labels ?? {};
        return (
          labels["orloj.dev/task-schedule"] === taskSchedule.metadata.name &&
          labels["orloj.dev/task-schedule-namespace"] === scheduleNamespace
        );
      })
      .sort((a, b) => {
        const at = a.metadata.createdAt ?? "";
        const bt = b.metadata.createdAt ?? "";
        return bt.localeCompare(at);
      });
  }, [tasks.data, taskSchedule, scheduleNamespace]);

  const runColumns: Column<Task>[] = [
    { key: "name", header: "Run Task", render: (r) => <span className="mono">{r.metadata.name}</span> },
    { key: "phase", header: "Status", render: (r) => <StatusBadge phase={r.status?.phase} />, width: "120px" },
    { key: "worker", header: "Worker", render: (r) => <span className="text-muted">{r.status?.assignedWorker ?? "-"}</span> },
    { key: "started", header: "Started", render: (r) => formatDateTime(r.status?.startedAt) },
    { key: "completed", header: "Completed", render: (r) => formatDateTime(r.status?.completedAt) },
  ];

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" },
    { id: "runs", label: `Runs (${runs.length})` },
    { id: "yaml", label: "YAML" },
  ];

  if (isLoading || !taskSchedule) {
    return <div className="page"><div className="loading-placeholder">Loading task schedule...</div></div>;
  }

  const handleDelete = async () => {
    if (!window.confirm(`Delete TaskSchedule ${taskSchedule.metadata.name}?`)) return;
    try {
      await deleteMutation.mutateAsync(taskSchedule.metadata.name);
      toast("success", "TaskSchedule deleted successfully");
      navigate("/task-schedules");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete TaskSchedule");
    }
  };

  return (
    <div className="page">
      <div className="page__header">
        <div className="page__header-back">
          <button className="btn-ghost" onClick={() => navigate("/task-schedules")} aria-label="Back">
            <ArrowLeft size={16} />
          </button>
          <div>
            <h1 className="page__title">{taskSchedule.metadata.name}</h1>
            <p className="page__subtitle">
              {taskSchedule.spec.task_ref ?? "-"} · {taskSchedule.metadata.namespace}
            </p>
          </div>
          <StatusBadge phase={taskSchedule.status?.phase} size="md" />
        </div>
        <button
          className="btn-secondary text-red"
          onClick={handleDelete}
          disabled={deleteMutation.isPending}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete Schedule"}
        </button>
      </div>

      <div className="tab-bar">
        {tabs.map((t) => (
          <button
            key={t.id}
            className={clsx("tab-bar__tab", tab === t.id && "tab-bar__tab--active")}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="tab-content">
        {tab === "overview" && (
          <div className="detail-grid">
            <div className="detail-field">
              <span className="detail-field__label">Phase</span>
              <StatusBadge phase={taskSchedule.status?.phase} size="md" />
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Task Template</span>
              <span className="detail-field__value mono">{taskSchedule.spec.task_ref ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Schedule</span>
              <span className="detail-field__value mono">{taskSchedule.spec.schedule ?? "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Time Zone</span>
              <span className="detail-field__value">{taskSchedule.spec.time_zone ?? "UTC"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Suspended</span>
              <span className="detail-field__value">{taskSchedule.spec.suspend ? "Yes" : "No"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Concurrency Policy</span>
              <span className="detail-field__value">{taskSchedule.spec.concurrency_policy ?? "forbid"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Starting Deadline (seconds)</span>
              <span className="detail-field__value">{taskSchedule.spec.starting_deadline_seconds ?? 300}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last Triggered Task</span>
              <span
                className={clsx(
                  "detail-field__value",
                  taskSchedule.status?.lastTriggeredTask && "detail-field__link",
                )}
                onClick={() => {
                  if (taskSchedule.status?.lastTriggeredTask) {
                    navigate(`/tasks/${taskSchedule.status.lastTriggeredTask}`);
                  }
                }}
              >
                {taskSchedule.status?.lastTriggeredTask ?? "-"}
              </span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Successful History Limit</span>
              <span className="detail-field__value">{taskSchedule.spec.successful_history_limit ?? 10}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Failed History Limit</span>
              <span className="detail-field__value">{taskSchedule.spec.failed_history_limit ?? 3}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last Schedule Time</span>
              <span className="detail-field__value">{formatDateTime(taskSchedule.status?.lastScheduleTime)}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Last Successful Time</span>
              <span className="detail-field__value">{formatDateTime(taskSchedule.status?.lastSuccessfulTime)}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Next Schedule Time</span>
              <span className="detail-field__value">{formatDateTime(taskSchedule.status?.nextScheduleTime)}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Active Runs</span>
              <span className="detail-field__value mono">{(taskSchedule.status?.activeRuns ?? []).join(", ") || "-"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Observed Generation</span>
              <span className="detail-field__value">{taskSchedule.status?.observedGeneration ?? "-"}</span>
            </div>
            {taskSchedule.status?.lastError && (
              <div className="detail-field detail-field--full">
                <span className="detail-field__label">Last Error</span>
                <span className="detail-field__value text-red">{taskSchedule.status.lastError}</span>
              </div>
            )}
          </div>
        )}

        {tab === "runs" && (
          <ResourceTable
            columns={runColumns}
            data={runs}
            rowKey={(r) => r.metadata.name}
            onRowClick={(r) => navigate(`/tasks/${r.metadata.name}`)}
            emptyMessage="No generated runs for this schedule"
            loading={tasks.isLoading}
          />
        )}

        {tab === "yaml" && <YamlEditor value={JSON.stringify(taskSchedule, null, 2)} />}
      </div>
    </div>
  );
}
