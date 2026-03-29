# Quickstart

Get a multi-agent pipeline running in under five minutes. This quickstart uses sequential execution mode -- the simplest way to run Orloj with a single process and no external dependencies.

> **Using Homebrew or release binaries?** Use the guided [5-minute tutorial](../guides/five-minute-tutorial.md) instead: it covers `orlojctl init`, a real OpenAI secret (`value=…`), and `orlojctl run` against `demo-system`. This page is aimed at **from-source** development with `go run` and the checked-in `examples/` blueprints.

## Before You Begin

- Go `1.24+` is installed.
- You are in repository root.

## 1. Start the Server

Start `orlojd` with an embedded worker in sequential mode:

```bash
go run ./cmd/orlojd \
  --storage-backend=memory \
  --task-execution-mode=sequential \
  --embedded-worker
```

This runs the server and a built-in worker in a single process. No separate worker needed.

**Web console:** Open [http://127.0.0.1:8080/](http://127.0.0.1:8080/) in your browser to view agents, systems, tasks, and the task trace. You can use it to inspect the pipeline and task status as you run the steps below.

## 2. Apply a Starter Blueprint

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/ --run
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
  --agent-message-consume
```

See [Execution and Messaging](../concepts/execution-model.md) for details on the message lifecycle, ownership guarantees, and retry behavior.

## Try with a Real Model

The quickstart above uses the mock gateway, which returns placeholder output. To use a real provider (OpenAI, Anthropic, Ollama, etc.), create a **Secret** resource for your API key, create a **ModelEndpoint** that references it via `auth.secretRef`, and point your agents at that endpoint with `model_ref`. See [Configure Model Routing](../guides/configure-model-routing.md) for the full steps.

## Next Steps

- [Starter Blueprints](../guides/starter-blueprints.md) -- pipeline, hierarchical, and swarm-loop topologies
- [Configuration](../operations/configuration.md) -- all flags and environment variables
