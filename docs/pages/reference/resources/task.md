# Task

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `system` (string): target `AgentSystem` name.
- `mode` (string): `run` (default) or `template`.
- `input` (map[string]string): task payload.
- `priority` (string)
- `max_turns` (int, >= 0): required for cyclic graph traversal.
- `retry` (object):
  - `max_attempts` (int)
  - `backoff` (duration string)
- `message_retry` (object):
  - `max_attempts` (int)
  - `backoff` (duration string)
  - `max_backoff` (duration string)
  - `jitter`: `none`, `full`, `equal`
  - `non_retryable` ([]string)
- `requirements` (object):
  - `region` (string)
  - `gpu` (bool)
  - `model` (string)

## Defaults and Validation

- `input` defaults to `{}`.
- `priority` defaults to `normal`.
- `mode` defaults to `run`.
- `mode=template` marks a task as non-executable template for schedules.
- `max_turns` must be `>= 0`.
- `retry` defaults:
  - `max_attempts` -> `1`
  - `backoff` -> `0s`
- `message_retry` defaults:
  - `max_attempts` -> `retry.max_attempts`
  - `backoff` -> `retry.backoff`
  - `max_backoff` -> `24h`
  - `jitter` -> `full`
- `retry.backoff`, `message_retry.backoff`, and `message_retry.max_backoff` must parse as durations.

## status

Primary fields:

- `phase`: `Pending`, `Running`, `WaitingApproval`, `Succeeded`, `Failed`, `DeadLetter`.
- `lastError`, `startedAt`, `completedAt`, `nextAttemptAt`, `attempts`
- `output`, `assignedWorker`, `claimedBy`, `leaseUntil`, `lastHeartbeat`
- `blocked_on`: exact approval resource currently pausing the task (`kind`, `name`, `reason`)
- `observedGeneration`

The `WaitingApproval` phase indicates the task is paused pending either a `ToolApproval` or `TaskApproval`. `Task.status.blocked_on` identifies the exact blocker so resume logic is deterministic. Approved reviews transition the task back to `Running` or `Succeeded` depending on the checkpoint. Denied or expired approvals transition the task to `Failed`.

Observability arrays:

- `trace[]`: detailed execution/tool-call events.
- `history[]`: lifecycle transitions.
- `messages[]`: message bus records.
- `message_idempotency[]`: message idempotency state.
- `join_states[]`: fan-in join activation state.
- `delegation_states[]`: delegation-gate activation state.

Example: [`examples/resources/tasks/`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/tasks)

See also: [Task concept](../../concepts/tasks/task.md)
