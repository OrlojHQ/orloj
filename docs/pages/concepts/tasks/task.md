# Task

A **Task** is a request to execute an [AgentSystem](../agents/agent-system.md). Tasks are the unit of work in Orloj -- they carry input, track execution state, and produce output.

## Defining a Task

```yaml
apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: weekly-report
spec:
  system: report-system
  input:
    topic: AI startups
  priority: high
  retry:
    max_attempts: 3
    backoff: 5s
  message_retry:
    max_attempts: 2
    backoff: 250ms
    max_backoff: 2s
    jitter: full
  requirements:
    region: default
    model: gpt-4o
```

## Task Lifecycle

Every task moves through a well-defined set of phases:

```
Pending ──► Running ──► Succeeded
                   └──► Failed
                   └──► DeadLetter
```

| Phase | Meaning |
|---|---|
| `Pending` | Task is created and waiting for a worker to claim it. |
| `Running` | A worker has claimed the task and is executing the agent graph. |
| `Succeeded` | All agents in the graph completed successfully. |
| `Failed` | Execution failed and retries are not exhausted. May transition back to `Pending`. |
| `DeadLetter` | All retry attempts exhausted. Terminal state requiring manual investigation. |

## Worker Assignment and Leases

The scheduler assigns tasks to workers based on `requirements` (region, GPU, model). Workers claim tasks through a lease mechanism:

1. Scheduler matches task requirements to worker capabilities.
2. Worker claims the task and acquires a time-bounded lease.
3. Worker renews the lease via heartbeats during execution.
4. If the lease expires (worker crash, network partition), another worker may safely take over.

This guarantees exactly-once processing semantics even under failure.

## Retry Configuration

Tasks support two levels of retry:

**Task-level retry** (`spec.retry`) -- retries the entire task from the beginning if it fails.

```yaml
retry:
  max_attempts: 3
  backoff: 5s
```

**Message-level retry** (`spec.message_retry`) -- retries individual agent-to-agent messages within the graph without restarting the full task.

```yaml
message_retry:
  max_attempts: 2
  backoff: 250ms
  max_backoff: 2s
  jitter: full
```

Retry uses capped exponential backoff with configurable jitter (`none`, `full`, `equal`). Messages that exhaust retries transition to `deadletter` phase.

## Cyclic Graphs

For AgentSystems with cycles (loops), `spec.max_turns` bounds the number of iterations to prevent infinite execution:

```yaml
spec:
  system: manager-research-loop-system
  input:
    topic: AI coding assistants
  max_turns: 6
```

## Task Templates

Tasks with `mode: template` serve as templates for [TaskSchedules](./task-schedule.md) and [TaskWebhooks](./task-webhook.md). They are not executed directly.

```yaml
spec:
  mode: template
  system: report-system
  input:
    topic: AI startups
```

## Related

- [TaskSchedule](./task-schedule.md) -- automate task creation with cron
- [TaskWebhook](./task-webhook.md) -- trigger tasks from external events
- [Worker](../infrastructure/worker.md) -- the execution units that run tasks
- [Resource Reference: Task](../../reference/resources/task.md)
- [Execution and Messaging](../execution-model.md)
