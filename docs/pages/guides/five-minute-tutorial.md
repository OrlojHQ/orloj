# Your First Agent System in 5 Minutes

This tutorial walks you through a three-agent **pipeline**: planner → research → writer. You will go from an empty directory to a running task with real model calls, using the web console to watch progress.

If you prefer to work from the repository’s checked-in examples instead of scaffolding, see [Quickstart](../getting-started/quickstart.md).

## Prerequisites

- **orlojctl** installed — [Homebrew](../getting-started/install.md#homebrew-macos--linux) (`brew tap OrlojHQ/orloj && brew install orlojctl`) or the [install script](../getting-started/install.md#install-script-all-binaries)
- **orlojd** — same install script, [release binaries](https://github.com/OrlojHQ/orloj/releases), or `go run ./cmd/orlojd` from a clone
- An **OpenAI API key** (or change the scaffolded `model-endpoint.yaml` to another [supported provider](../guides/configure-model-routing.md))

## 1. Start the server

In one terminal, run the API server with an embedded worker and in-memory storage (no database required):

```bash
orlojd --storage-backend=memory --embedded-worker
```

The default task execution mode is **sequential**, which is ideal for this walkthrough.

Open the web console at [http://127.0.0.1:8080/](http://127.0.0.1:8080/) — you should see the dashboard.

## 2. Scaffold a pipeline

In another terminal, scaffold the manifests:

```bash
orlojctl init demo
```

This creates a `demo/` directory containing:

- `agents/planner_agent.yaml`, `agents/research_agent.yaml`, `agents/writer_agent.yaml`
- `agent-system.yaml` — graph wiring the three agents in order
- `model-endpoint.yaml` — OpenAI endpoint named `openai-default` referencing a secret
- `task.yaml` — a sample task (you will use `orlojctl run` instead in the next steps)

The **AgentSystem** resource is named **`demo-system`** (prefix `demo` plus `-system`). You will pass that name to `orlojctl run`.

## 3. Add your API key

The scaffold expects a Secret named **`openai-api-key`** with the default key **`value`** (this is what the model gateway reads):

```bash
orlojctl create secret openai-api-key --from-literal value=sk-your-key-here
```

You do not need to edit `model-endpoint.yaml` unless you want a different secret name or provider. See [Configure Model Routing](./configure-model-routing.md) for Anthropic, Azure OpenAI, Ollama, and more.

## 4. Apply manifests

Apply every manifest in the scaffolded directory at once:

```bash
orlojctl apply -f demo/ --run
```

Or apply resources individually:

```bash
orlojctl apply -f demo/agents/
orlojctl apply -f demo/model-endpoint.yaml
orlojctl apply -f demo/agent-system.yaml
```

The first form also applies `task.yaml`, which creates a sample **Task** named `demo-task`. If you want to apply the directory without runnable tasks, omit `--run`. Either way, `orlojctl run` in the next step still creates a **new** task with your topic.

## 5. Run the pipeline

Submit a task with input for the agents (the scaffold uses a `topic` field):

```bash
orlojctl run --system demo-system topic="The future of open source AI"
```

The CLI prints a line like `task run-demo-system-… created, watching…`, waits until the task finishes, and prints the final **output** on success.

## 6. Watch execution

- **Web console:** Open the task from the UI to see topology, status, and streaming detail.
- **Logs snapshot:** After `run` reports the task name, fetch stored log lines (no live follow flag):

```bash
orlojctl logs task/<task-name>
```

Use the exact task name printed when the task was created (for example `run-demo-system-1730000000000`).

## What just happened?

Orloj stored your **Agent**, **AgentSystem**, **ModelEndpoint**, and **Secret** resources, then created a **Task** that references `demo-system`. The embedded worker **claimed** the task, executed the graph in order (planner, then research, then writer), routed each step through the **model gateway** using `openai-default`, and recorded status and output.

Handoffs between agents follow the **execution and messaging** rules for your mode (here, sequential). For a deeper picture of components and scaling, read [Architecture](../concepts/architecture.md) and [Execution & Messaging](../concepts/execution-model.md).

## Next steps

- [Build a Custom Tool](./build-custom-tool.md) — give agents callable tools
- [Set Up Multi-Agent Governance](./setup-governance.md) — policies, roles, and tool permissions
- [Deploy to a VPS](../deploy/vps.md) — Postgres, TLS, and production-like operation
- [Starter Blueprints](./starter-blueprints.md) — try `orlojctl init myproject --blueprint hierarchical` or `--blueprint swarm-loop`
