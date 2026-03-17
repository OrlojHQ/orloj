# Task Scheduling (Cron)

Use `TaskSchedule` to create recurring run tasks from a task template.

## Purpose

`TaskSchedule` evaluates a 5-field cron expression and creates a new `Task` run from a template task (`spec.mode=template`).

## Before You Begin

- `orlojd` is running (scheduler/controller active).
- At least one worker is available for execution.
- The target `Task` template exists and sets `spec.mode: template`.

## 1. Apply a Task Template

```bash
go run ./cmd/orlojctl apply -f examples/tasks/weekly_report_template_task.yaml
```

Template reference used by schedules:

- `metadata.name`: `weekly-report-template`
- `spec.mode`: `template`

## 2. Apply a Schedule

Example schedule resource:

- [`examples/task-schedules/weekly_report_schedule.yaml`](../../../examples/task-schedules/weekly_report_schedule.yaml)

Apply it:

```bash
go run ./cmd/orlojctl apply -f examples/task-schedules/weekly_report_schedule.yaml
```

Key fields:

- `spec.task_ref`: template task name (`name` or `namespace/name`)
- `spec.schedule`: 5-field cron expression (for example, `0 9 * * 1`)
- `spec.time_zone`: IANA timezone (for example, `America/Chicago`)
- `spec.concurrency_policy`: v1 supports `forbid`
- `spec.starting_deadline_seconds`: lateness window before a missed slot is skipped

## 3. Verify Schedule State

List schedules:

```bash
go run ./cmd/orlojctl get task-schedules
```

Inspect schedule status directly:

```bash
curl -s "http://127.0.0.1:8080/v1/task-schedules/weekly-report?namespace=default" | jq .status
```

Important status fields:

- `nextScheduleTime`
- `lastScheduleTime`
- `lastTriggeredTask`
- `activeRuns`

## 4. Verify Triggered Run Tasks

```bash
go run ./cmd/orlojctl get tasks
```

Generated run tasks are labeled with schedule metadata:

- `orloj.dev/task-schedule`
- `orloj.dev/task-schedule-namespace`
- `orloj.dev/task-schedule-slot`

## Common Controls

- Pause scheduling: set `spec.suspend: true`
- Resume scheduling: set `spec.suspend: false`
- Retention: tune `successful_history_limit` and `failed_history_limit`

## Troubleshooting

- If no tasks are created, verify `spec.task_ref` points to an existing template task.
- If schedule status is `Error`, inspect `.status.lastError` and timezone/cron syntax.
- If runs are skipped, check `starting_deadline_seconds` and `concurrency_policy` behavior.

## Related Docs

- [Resource Reference (`TaskSchedule`)](../reference/resources.md)
- [API Reference](../reference/api.md)
- [Troubleshooting](./troubleshooting.md)
