# TaskSchedule

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `task_ref` (string): task template reference (`name` or `namespace/name`).
- `schedule` (string): 5-field cron expression.
- `time_zone` (string): IANA timezone.
- `suspend` (bool): stop triggering when `true`.
- `starting_deadline_seconds` (int): max lateness window for catch-up.
- `concurrency_policy` (string): `forbid` (v1).
- `successful_history_limit` (int): retained successful run count.
- `failed_history_limit` (int): retained failed/deadletter run count.

## Defaults and Validation

- `task_ref` is required and must be `name` or `namespace/name`.
- `schedule` is required and must be a valid 5-field cron.
- `time_zone` defaults to `UTC`.
- `starting_deadline_seconds` defaults to `300`.
- `concurrency_policy` defaults to `forbid`.
- `successful_history_limit` defaults to `10`.
- `failed_history_limit` defaults to `3`.

## status

- `phase`, `lastError`, `observedGeneration`
- `lastScheduleTime`, `lastSuccessfulTime`, `nextScheduleTime`
- `lastTriggeredTask`, `activeRuns`

Example: [`examples/resources/task-schedules/`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/task-schedules)

See also: [Task schedule concept](../../concepts/tasks/task-schedule.md)
