# TaskSchedule

A **TaskSchedule** creates [Tasks](./task.md) on a cron-based schedule from a template task. Use this to automate recurring work like daily reports, periodic data processing, or scheduled monitoring runs.

## Defining a TaskSchedule

```yaml
apiVersion: orloj.dev/v1
kind: TaskSchedule
metadata:
  name: weekly-report
spec:
  task_ref: weekly-report-template
  schedule: "0 9 * * 1"
  time_zone: America/Chicago
  suspend: false
  starting_deadline_seconds: 300
  concurrency_policy: forbid
  successful_history_limit: 10
  failed_history_limit: 3
```

### Key Fields

| Field | Description |
|---|---|
| `task_ref` | Reference to a Task with `mode: template`. |
| `schedule` | Standard 5-field cron expression. |
| `time_zone` | IANA timezone (defaults to `UTC`). |
| `concurrency_policy` | `forbid` prevents overlapping runs. |
| `starting_deadline_seconds` | Maximum lateness before a missed trigger is skipped. |
| `suspend` | Set to `true` to pause scheduling without deleting the resource. |

## How It Works

When the cron fires, the scheduler:

1. Checks `concurrency_policy` -- if `forbid` and a previous run is still active, the trigger is skipped.
2. Checks `starting_deadline_seconds` -- if the trigger is later than the deadline, it is skipped.
3. Creates a new Task from the template, inheriting all `spec` fields from the referenced task.
4. The new task enters `Pending` and follows the normal [Task lifecycle](./task.md#task-lifecycle).

## Related

- [Task](./task.md) -- the tasks that schedules create
- [TaskWebhook](./task-webhook.md) -- trigger tasks from external events
- [Resource Reference: TaskSchedule](../../reference/resources/task-schedule.md)
