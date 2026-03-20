# Orloj

*Named after the [Prague Orloj](https://en.wikipedia.org/wiki/Prague_astronomical_clock), an astronomical clock that has coordinated complex mechanisms for over 600 years.*

[![Release](https://img.shields.io/github/v/release/OrlojHQ/orloj?display_name=tag&sort=semver)](https://github.com/OrlojHQ/orloj/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/OrlojHQ/orloj/ci.yml?branch=main&label=ci)](https://github.com/OrlojHQ/orloj/actions/workflows/ci.yml)
[![Release workflow](https://img.shields.io/github/actions/workflow/status/OrlojHQ/orloj/release.yml?label=release+workflow)](https://github.com/OrlojHQ/orloj/actions/workflows/release.yml)
[![Docs](https://img.shields.io/github/actions/workflow/status/OrlojHQ/orloj/docs.yml?branch=main&label=docs)](https://github.com/OrlojHQ/orloj/actions/workflows/docs.yml)
[![License](https://img.shields.io/github/license/OrlojHQ/orloj)](LICENSE)

**A lightweight orchestration plane for multi-agent AI systems.**

Define agents, tools, policies, and workflows as declarative YAML. Orloj handles scheduling, execution, model routing, governance enforcement, and reliability -- so you can run multi-agent systems in production with the same operational rigor you expect from infrastructure.

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

```bash
# Build from source (requires Go 1.24+)
go build -o orlojd ./cmd/orlojd
go build -o orlojctl ./cmd/orlojctl

# Start the server with an embedded worker
./orlojd --storage-backend=memory --task-execution-mode=sequential --embedded-worker --model-gateway-provider=mock

# Apply a starter blueprint (pipeline: planner -> research -> writer)
./orlojctl apply -f examples/blueprints/pipeline/

# Check the result
./orlojctl get task bp-pipeline-task
```

When you are ready to scale, switch to message-driven mode with distributed workers and Postgres persistence. See the [Quickstart guide](docs/pages/getting-started/quickstart.md#scaling-to-production) for details.

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

| Resource | Purpose |
|---|---|
| Agent | Unit of work backed by a language model |
| AgentSystem | Directed graph composing multiple agents |
| ModelEndpoint | Connection to a model provider |
| Tool | External capability with isolation and retry |
| Secret | Credential storage (dev use; env vars for production) |
| Memory | Vector-backed retrieval for agents |
| AgentPolicy | Token, model, and tool constraints |
| AgentRole | Named permission set bound to agents |
| ToolPermission | Required permissions for tool invocation |
| ToolApproval | Approval record for gated tool invocations |
| Task | Request to execute an AgentSystem |
| TaskSchedule | Cron-based task creation |
| TaskWebhook | Event-triggered task creation |
| Worker | Execution unit with capability declaration |
| McpServer | MCP server connection that discovers/syncs MCP tools |

## Documentation

Full documentation is available at the [docs site](docs/pages/index.md) or locally:

```bash
bun install && bun run docs:dev
```

Key pages:

- [Getting Started](docs/pages/getting-started/index.md) -- install, quickstart
- [Architecture Overview](docs/pages/architecture/overview.md) -- server, workers, governance, execution modes
- [Concepts](docs/pages/concepts/index.md) -- agents, tasks, tools, model routing, governance
- [Guides](docs/pages/guides/index.md) -- deploy a pipeline, configure routing, build tools, set up governance
- [Reference](docs/pages/reference/index.md) -- CLI, API, resource schemas, contracts
- [Deployment](docs/pages/deployment/index.md) -- local, VPS, Kubernetes, [remote CLI access](docs/pages/deployment/remote-cli-access.md)
- [Security and Isolation](docs/pages/operations/security.md) -- control plane API tokens (`openssl rand`), secrets, tool isolation

## Docker Compose

Run the full stack (Postgres + server + 2 workers) with Docker Compose:

```bash
docker compose up --build -d
docker compose ps
```

## Kubernetes (Helm)

Deploy with the Helm chart in `charts/orloj`:

```bash
export REGISTRY=ghcr.io/<your-org-or-user>
export TAG=v0.1.0

helm upgrade --install orloj ./charts/orloj \
  --namespace orloj \
  --create-namespace \
  --set orlojd.image.repository="${REGISTRY}/orloj-orlojd" \
  --set orlojd.image.tag="${TAG}" \
  --set orlojworker.image.repository="${REGISTRY}/orloj-orlojworker" \
  --set orlojworker.image.tag="${TAG}" \
  --set postgres.auth.password='<strong-password>' \
  --set runtimeSecret.modelGatewayApiKey='<model-provider-api-key>'
```

See the Kubernetes runbook for full operations and troubleshooting: [docs/pages/deployment/kubernetes.md](docs/pages/deployment/kubernetes.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
