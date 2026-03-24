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
- `observedGeneration`

The `WaitingApproval` phase indicates the task is paused pending a `ToolApproval` decision. When the linked `ToolApproval` is approved, the task transitions back to `Running`. When denied or expired, the task transitions to `Failed` with an `approval_denied` or `approval_timeout` reason.

Observability arrays:

- `trace[]`: detailed execution/tool-call events.
- `history[]`: lifecycle transitions.
- `messages[]`: message bus records.
- `message_idempotency[]`: message idempotency state.
- `join_states[]`: fan-in join activation state.

Example: [`examples/resources/tasks/`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/tasks)

See also: [Task concept](../../concepts/tasks/task.md)
