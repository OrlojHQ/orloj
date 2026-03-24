# Worker

A **Worker** is an execution unit that claims and runs [Tasks](../tasks/task.md). Workers register their capabilities (region, GPU, supported models) and the scheduler uses these for task matching.

## Defining a Worker

```yaml
apiVersion: orloj.dev/v1
kind: Worker
metadata:
  name: worker-a
spec:
  region: default
  max_concurrent_tasks: 1
  capabilities:
    gpu: false
    supported_models:
      - gpt-4o
```

### Key Fields

| Field | Description |
|---|---|
| `region` | Region label used for task requirement matching. |
| `max_concurrent_tasks` | Maximum number of tasks this worker will claim simultaneously. Defaults to `1`. |
| `capabilities.gpu` | Whether this worker has GPU access. |
| `capabilities.supported_models` | Model identifiers this worker can serve. |

## How Workers Operate

Workers connect to the Orloj server and participate in task assignment:

1. The worker registers itself with its capabilities.
2. The scheduler matches tasks to workers based on `Task.spec.requirements` (region, GPU, model).
3. When matched, the worker claims the task and acquires a time-bounded lease.
4. The worker renews the lease via heartbeats during execution.
5. If the lease expires (worker crash, network partition), another worker may safely take over.

Workers can run as separate processes (`orlojworker`) or embedded in the server process (`orlojd --embedded-worker`) for single-process development.

## Status

| Field | Description |
|---|---|
| `phase` | Worker lifecycle phase. |
| `lastHeartbeat` | Timestamp of last heartbeat. |
| `currentTasks` | Tasks currently claimed by this worker. |

## Related

- [Task](../tasks/task.md) -- the work units that workers execute
- [Resource Reference: Worker](../../reference/resources/worker.md)
- [Deployment: Local](../../deploy/local.md)
