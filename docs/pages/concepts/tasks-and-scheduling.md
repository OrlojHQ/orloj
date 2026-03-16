# Tasks and Scheduling

A **Task** is a request to execute an AgentSystem. Tasks are the unit of work in Orloj -- they carry input, track execution state, and produce output. **TaskSchedules** and **TaskWebhooks** automate task creation from cron expressions and external events.

## Tasks

A Task binds an AgentSystem to a specific input and execution configuration.

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

### Task Lifecycle

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

### Worker Assignment and Leases

The scheduler assigns tasks to workers based on `requirements` (region, GPU, model). Workers claim tasks through a lease mechanism:

1. Scheduler matches task requirements to worker capabilities.
2. Worker claims the task and acquires a time-bounded lease.
3. Worker renews the lease via heartbeats during execution.
4. If the lease expires (worker crash, network partition), another worker may safely take over.

This guarantees exactly-once processing semantics even under failure.

### Retry Configuration

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

### Cyclic Graphs

For AgentSystems with cycles (loops), `spec.max_turns` bounds the number of iterations to prevent infinite execution:

```yaml
spec:
  system: manager-research-loop-system
  input:
    topic: AI coding assistants
  max_turns: 6
```

### Task Templates

Tasks with `mode: template` serve as templates for TaskSchedules and TaskWebhooks. They are not executed directly.

```yaml
spec:
  mode: template
  system: report-system
  input:
    topic: AI startups
```

## Task Schedules

A TaskSchedule creates tasks on a cron-based schedule from a template task.

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

| Field | Description |
|---|---|
| `schedule` | Standard 5-field cron expression. |
| `time_zone` | IANA timezone (defaults to `UTC`). |
| `concurrency_policy` | `forbid` prevents overlapping runs. |
| `starting_deadline_seconds` | Maximum lateness before a missed trigger is skipped. |
| `suspend` | Set to `true` to pause scheduling without deleting the resource. |

## Task Webhooks

A TaskWebhook creates tasks in response to external HTTP events, with built-in signature verification and idempotency.

```yaml
apiVersion: orloj.dev/v1
kind: TaskWebhook
metadata:
  name: report-github-push
spec:
  task_ref: weekly-report-template
  auth:
    profile: github
    secret_ref: webhook-shared-secret
  idempotency:
    event_id_header: X-GitHub-Delivery
    dedupe_window_seconds: 86400
  payload:
    mode: raw
    input_key: webhook_payload
```

Supported auth profiles:

| Profile | Signature Method | Headers |
|---|---|---|
| `generic` | HMAC-SHA256 over `timestamp + "." + rawBody` | `X-Signature`, `X-Timestamp`, `X-Event-Id` |
| `github` | HMAC-SHA256 over raw body | `X-Hub-Signature-256`, `X-GitHub-Delivery` |

## Workers

Workers are the execution units that claim and run tasks. They register capabilities and the scheduler uses these for task matching.

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

## Related Resources

- [Resource Reference: Task, TaskSchedule, TaskWebhook, Worker](../reference/crds.md)
- [Execution and Messaging](../architecture/execution-model.md)
- [Troubleshooting](../operations/troubleshooting.md)
