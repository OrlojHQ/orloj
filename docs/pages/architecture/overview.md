# Architecture Overview

Orloj is organized into three layers: a **server** that manages resources and scheduling, **workers** that execute agent workflows, and a **governance layer** that enforces policies and permissions at runtime.

```
┌─────────────────────────────────────────────────────┐
│                  Server (orlojd)                     │
│                                                     │
│  ┌──────────────┐   ┌────────────────┐              │
│  │  API Server   │──►│ Resource Store  │             │
│  │   (REST)      │   │ mem/postgres   │              │
│  └──────┬───────┘   └────────────────┘              │
│         │                                           │
│         ▼                                           │
│  ┌──────────────┐   ┌────────────────┐              │
│  │   Services    │──►│ Task Scheduler │              │
│  └──────────────┘   └───────┬────────┘              │
└─────────────────────────────┼───────────────────────┘
                              │ assign tasks
                              ▼
┌─────────────────────────────────────────────────────┐
│                 Workers (orlojworker)                │
│                                                     │
│  ┌──────────────┐   ┌───────────────┐               │
│  │  Task Worker  │──►│ Model Gateway │               │
│  │              │   └───────────────┘               │
│  │              │   ┌───────────────┐               │
│  │              │──►│  Tool Runtime  │               │
│  │              │   └───────────────┘               │
│  │              │   ┌───────────────┐               │
│  │       ◄──────┼───│  Message Bus   │               │
│  │              │──►│  mem/nats-js   │               │
│  └──────────────┘   └───────────────┘               │
│         ▲                                           │
└─────────┼───────────────────────────────────────────┘
          │ enforced at runtime
┌─────────┴───────────────────────────────────────────┐
│                    Governance                        │
│                                                     │
│  ┌─────────────┐ ┌───────────┐ ┌────────────────┐   │
│  │ AgentPolicy  │ │ AgentRole │ │ ToolPermission │   │
│  └─────────────┘ └───────────┘ └────────────────┘   │
└─────────────────────────────────────────────────────┘
```

## Server

The server runs as `orlojd` and provides:

**API Server** -- HTTP REST API for creating, reading, updating, and deleting all 13 resource types. Supports watch endpoints for real-time event streaming, optimistic concurrency via `resourceVersion` / `If-Match`, and namespace scoping. Also serves the built-in web console at `/ui/`.

**Resource Store** -- Pluggable storage backend for all resources. Two implementations:
- `memory` -- in-memory store for local development and testing. Fast, no dependencies, data is lost on restart.
- `postgres` -- PostgreSQL-backed store for production. Uses `FOR UPDATE SKIP LOCKED` for safe concurrent task claiming.

**Services** -- Background processes for each resource type: Agent, AgentSystem, ModelEndpoint, Tool, Memory, AgentPolicy, Task, TaskScheduler, TaskSchedule, and Worker. Services drive resources toward their desired state and update status fields.

**Task Scheduler** -- Matches pending tasks to available workers based on requirements (region, GPU, model), respects TaskSchedule cron triggers, and manages the assignment lifecycle.

## Workers

Workers run as `orlojworker` and execute the actual agent workflows:

**Task Worker** -- Claims assigned tasks via the lease mechanism, executes the agent graph step by step, and reports results back through status updates. Supports concurrent task execution up to `max_concurrent_tasks`.

**Model Gateway** -- Routes model requests to the appropriate provider based on the agent's `model_ref` or `model` configuration. Handles provider-specific request formatting, authentication, and response parsing for OpenAI, Anthropic, Azure OpenAI, Ollama, and mock backends.

**Tool Runtime** -- Executes tool invocations with the configured isolation backend (none, sandboxed, container, or WASM). Enforces timeouts, manages retries with capped exponential backoff and jitter, and normalizes responses into the standard tool contract envelope.

**Message Bus** -- Handles agent-to-agent communication within a task's graph. Two implementations:
- `memory` -- in-memory bus for local development.
- `nats-jetstream` -- NATS JetStream for production with durable delivery guarantees.

## Governance Layer

The governance layer is not a separate process -- it is enforced inline during worker execution:

**AgentPolicy** -- Evaluated before each agent turn. Checks `allowed_models`, `blocked_tools`, and `max_tokens_per_run`. Policies can be scoped to specific systems/tasks or applied globally.

**AgentRole + ToolPermission** -- Evaluated before each tool invocation. The worker collects the agent's permissions from all bound roles and checks them against the tool's ToolPermission requirements. Unauthorized calls return `tool_permission_denied`.

All governance decisions are deterministic and fail-closed. Denied actions produce structured errors that flow into task trace and history for auditability.

## Execution Modes

Orloj supports two execution modes. Start with sequential for development, then graduate to message-driven for production.

| Mode | How it works | When to use |
|---|---|---|
| `sequential` | The server drives execution directly in a single process. Simpler, lower latency, easy to debug. | Getting started, development, single-agent systems |
| `message-driven` | Workers consume from the message bus. Agents hand off via durable queued messages. Enables parallel fan-out and horizontal scaling. | Production, multi-agent systems, distributed workloads |

**Sequential** is the default and requires no external dependencies. Use `--embedded-worker` to run everything in one process.

**Message-driven** requires `--task-execution-mode=message-driven` and a message bus backend (`memory` for local testing, `nats-jetstream` for production). This mode provides lease-based ownership, idempotent replay, and dead-letter handling.

## Reliability Characteristics

Orloj's runtime provides several reliability guarantees:

- **Lease-based task ownership** -- Workers hold time-bounded leases on tasks. If a worker crashes, the lease expires and another worker can safely take over.
- **Owner-only message execution** -- Only the worker that holds the task lease can process messages for that task, preventing duplicate execution.
- **Idempotency tracking** -- Message idempotency keys prevent duplicate processing during replay and crash recovery.
- **Capped exponential retry with jitter** -- Both task-level and message-level retries use bounded backoff with configurable jitter to avoid thundering herds.
- **Dead-letter transitions** -- Messages and tasks that exhaust all retries move to a terminal `DeadLetter` phase for manual investigation rather than being silently dropped.

## Related Docs

- [Execution and Messaging](./execution-model.md)
- [Concepts](../concepts/)
- [Runbook](../operations/runbook.md)
- [Configuration](../operations/configuration.md)
- [Resource Reference](../reference/resources.md)
