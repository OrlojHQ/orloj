<p align="center">
  <img src="docs/public/logo.png" alt="Orloj" width="200" />
</p>

# Orloj

_Named after the [Prague Orloj](https://en.wikipedia.org/wiki/Prague_astronomical_clock), an astronomical clock that has coordinated complex mechanisms for over 600 years._

[![Release](https://img.shields.io/github/v/release/OrlojHQ/orloj?display_name=tag&sort=semver)](https://github.com/OrlojHQ/orloj/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/OrlojHQ/orloj/ci.yml?branch=main&label=ci)](https://github.com/OrlojHQ/orloj/actions/workflows/ci.yml)
[![Release workflow](https://img.shields.io/github/actions/workflow/status/OrlojHQ/orloj/release.yml?label=release+workflow)](https://github.com/OrlojHQ/orloj/actions/workflows/release.yml)
[![Docs](https://img.shields.io/github/actions/workflow/status/OrlojHQ/orloj/docs.yml?branch=main&label=docs)](https://github.com/OrlojHQ/orloj/actions/workflows/docs.yml)
[![License](https://img.shields.io/github/license/OrlojHQ/orloj)](LICENSE)

**A lightweight orchestration plane for multi-agent AI systems.**

Define agents, tools, policies, and workflows as declarative YAML. Orloj handles scheduling, execution, model routing, governance enforcement, and reliability -- so you can run multi-agent systems in production with the same operational rigor you expect from infrastructure.

> **Status:** Orloj is under active development. APIs and resource schemas may change between minor versions before 1.0.

## Why Orloj

Running AI agents in production today looks a lot like running containers before container orchestration: ad-hoc scripts, no governance, no observability, and no standard way to manage an agent fleet. Orloj provides:

- **Agents-as-Code** -- declare agents, their models, tools, and constraints in version-controlled YAML manifests.
- **DAG-based orchestration** -- pipeline, hierarchical, and swarm-loop topologies with fan-out/fan-in support.
- **Model routing** -- bind agents to OpenAI, Anthropic, Azure OpenAI, or Ollama endpoints. Switch providers without changing agent definitions.
- **Tool isolation** -- execute tools in containers, WASM sandboxes, or process isolation with configurable timeout and retry.
- **Governance built in** -- policies, roles, and tool permissions enforced at the execution layer. Unauthorized tool calls fail closed.
- **Production reliability** -- lease-based task ownership, idempotent replay, capped exponential retry with jitter, and dead-letter handling.
- **Web console** -- built-in UI with topology views, task inspection, and live event streaming.

## Quickstart

Download **orlojd** (server) and **orlojctl** (CLI) for your platform from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases), extract them, and run:

```bash
# Start the server with an embedded worker
./orlojd --storage-backend=memory --task-execution-mode=sequential --embedded-worker
```

Open **http://127.0.0.1:8080/** to explore the web console, then apply a starter blueprint. The example manifests live in this repo -- clone it or [browse them on GitHub](https://github.com/OrlojHQ/orloj/tree/main/examples):

```bash
# Apply a starter blueprint (pipeline: planner -> research -> writer)
./orlojctl apply -f examples/blueprints/pipeline/

# Check the result
./orlojctl get task bp-pipeline-task
```

Or build from source (requires Go 1.25+):

```bash
go build -o orlojd ./cmd/orlojd
go build -o orlojctl ./cmd/orlojctl
```

When you are ready to scale, switch to message-driven mode with distributed workers and Postgres persistence. See the [Quickstart guide](https://docs.orloj.dev/getting-started/quickstart#scaling-to-production) for details.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Server (orlojd)                     │
│                                                     │
│  ┌──────────────┐   ┌────────────────┐              │
│  │  API Server   │──►│ Resource Store  │             │
│  │   (REST)      │   │ mem / postgres  │             │
│  └──────┬───────┘   └────────────────┘              │
│         │                                           │
│         ▼                                           │
│  ┌──────────────┐   ┌────────────────┐              │
│  │   Services    │──►│ Task Scheduler │              │
│  └──────────────┘   └───────┬────────┘              │
└─────────────────────────────┼───────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────┐
│                 Workers (orlojworker)                │
│                                                     │
│  ┌──────────────┐   ┌───────────────┐               │
│  │  Task Worker  │──►│ Model Gateway │               │
│  │              │   └───────────────┘               │
│  │              │──►┌───────────────┐               │
│  │              │   │  Tool Runtime  │               │
│  │              │   └───────────────┘               │
│  │       ◄──────┼───┌───────────────┐               │
│  │              │──►│  Message Bus   │               │
│  └──────────────┘   └───────────────┘               │
└─────────────────────────────────────────────────────┘
```

**Server** (`orlojd`) -- API server, resource store (in-memory or Postgres), background services, and task scheduler.

**Workers** (`orlojworker`) -- claim tasks, execute agent graphs, route model requests, run tools, and handle inter-agent messaging.

**Governance** -- AgentPolicy, AgentRole, and ToolPermission resources enforced inline during every tool call and model interaction.

## Resources

Orloj manages 15 resource types, all defined as declarative YAML with `apiVersion`, `kind`, `metadata`, `spec`, and `status` fields:

| Resource       | Purpose                                               |
| -------------- | ----------------------------------------------------- |
| Agent          | Unit of work backed by a language model               |
| AgentSystem    | Directed graph composing multiple agents              |
| ModelEndpoint  | Connection to a model provider                        |
| Tool           | External capability with isolation and retry          |
| Secret         | Credential storage (dev use; env vars for production) |
| Memory         | Vector-backed retrieval for agents                    |
| AgentPolicy    | Token, model, and tool constraints                    |
| AgentRole      | Named permission set bound to agents                  |
| ToolPermission | Required permissions for tool invocation              |
| ToolApproval   | Approval record for gated tool invocations            |
| Task           | Request to execute an AgentSystem                     |
| TaskSchedule   | Cron-based task creation                              |
| TaskWebhook    | Event-triggered task creation                         |
| Worker         | Execution unit with capability declaration            |
| McpServer      | MCP server connection that discovers/syncs MCP tools  |

## Documentation

Browse **[docs.orloj.dev](https://docs.orloj.dev)**, or build the same site locally:

```bash
cd docs && bun install && bun run dev
```

Key pages (sources in `docs/pages/`):

- [Getting Started](https://docs.orloj.dev/getting-started/install) -- install, quickstart
- [Concepts](https://docs.orloj.dev/concepts/architecture) -- architecture, agents, tasks, tools, model routing, governance
- [Guides](https://docs.orloj.dev/guides/) -- deploy a pipeline, configure routing, build tools, set up governance
- [Deploy & Operate](https://docs.orloj.dev/deploy/) -- local, VPS, Kubernetes, [remote CLI access](https://docs.orloj.dev/deploy/remote-cli-access)
- [Reference](https://docs.orloj.dev/reference/cli) -- CLI, API, resource schemas
- [Security](https://docs.orloj.dev/operations/security) -- control plane API tokens, secrets, tool isolation
- [Examples](examples/README.md) -- per-kind YAML under `examples/resources/`, starter `blueprints/`, and `use-cases/` (in this repo)

## Docker Compose

Run the full stack (Postgres + server + 2 workers) with Docker Compose:

```bash
docker compose up --build -d
docker compose ps
```

The Compose images include the server and workers only. To drive the API from your machine, install **`orlojctl`** from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases) (CLI-only tarball) or build from this repo; see [Deploy & Operate](https://docs.orloj.dev/deploy/).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
