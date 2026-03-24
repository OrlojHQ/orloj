# Worker

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `region` (string)
- `capabilities.gpu` (bool)
- `capabilities.supported_models` ([]string)
- `max_concurrent_tasks` (int)

## Defaults and Validation

- `max_concurrent_tasks` defaults to `1` when `<= 0`.

## status

- `phase`, `lastError`, `lastHeartbeat`, `observedGeneration`, `currentTasks`

Example: `examples/resources/workers/worker_a.yaml`

See also: [Worker concepts](../../concepts/infrastructure/worker.md).
