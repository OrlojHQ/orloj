# Quickstart

Get a multi-agent pipeline running in under five minutes. This quickstart uses sequential execution mode -- the simplest way to run Orloj with a single process and no external dependencies.

## Before You Begin

- Go `1.24+` is installed.
- You are in repository root.

## 1. Start the Server

Start `orlojd` with an embedded worker in sequential mode:

```bash
go run ./cmd/orlojd \
  --storage-backend=memory \
  --task-execution-mode=sequential \
  --embedded-worker \
  --model-gateway-provider=mock
```

This runs the server and a built-in worker in a single process. No separate worker needed.

## 2. Apply a Starter Blueprint

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/
```

This creates agents, an agent system (the pipeline graph), and a task in one command.

## 3. Verify Execution

```bash
go run ./cmd/orlojctl get task bp-pipeline-task
```

Expected result: task reaches `Succeeded`.

## Scaling to Production

When you are ready to run multi-worker, distributed workloads, switch to **message-driven** mode. This unlocks parallel fan-out, durable message delivery, and horizontal scaling.

Start the server:

```bash
go run ./cmd/orlojd \
  --storage-backend=postgres \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=nats-jetstream
```

Start one or more workers:

```bash
go run ./cmd/orlojworker \
  --storage-backend=postgres \
  --task-execution-mode=message-driven \
  --agent-message-bus-backend=nats-jetstream \
  --agent-message-consume \
  --model-gateway-provider=openai
```

See [Execution and Messaging](../architecture/execution-model.md) for details on the message lifecycle, ownership guarantees, and retry behavior.

## Next Steps

- [Starter Blueprints](../architecture/starter-blueprints.md) -- pipeline, hierarchical, and swarm-loop topologies
- [Production Checklist](./production-checklist.md) -- readiness gates for production rollout
- [Configuration](../operations/configuration.md) -- all flags and environment variables
