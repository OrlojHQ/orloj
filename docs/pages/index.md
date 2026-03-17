# What is Orloj?

Orloj is an open-source orchestration plane for multi-agent AI systems. Define your agents, tools, policies, and workflows as declarative YAML manifests. Orloj handles scheduling, execution, model routing, governance enforcement, and reliability -- so you can run multi-agent systems in production with the same operational rigor you expect from infrastructure.

## Why Orloj?

Running AI agents in production today looks a lot like running containers before Kubernetes: ad-hoc scripts, no governance, no observability, and no standard way to manage the lifecycle of an agent fleet.

Orloj solves this by providing an orchestration plane purpose-built for AI agent systems:

- **Agents become manageable infrastructure.** Declare agents, their models, tools, and constraints in version-controlled manifests. Apply them with a single command.
- **Multi-agent workflows are first-class.** Define pipelines, hierarchies, and swarm topologies as directed graphs. The runtime handles message routing, fan-out/fan-in, and turn-bounded loops.
- **Governance is built in, not bolted on.** Policies, roles, and tool permissions are enforced at the execution layer. Unauthorized tool calls fail closed -- not silently.
- **Production reliability by default.** Lease-based task ownership, capped exponential retry with jitter, idempotency tracking, and dead-letter handling are part of the core runtime.

## How It Works

1. **Start the server** -- run `orlojd` to host the API, resource store, and task scheduler.
2. **Connect workers** -- run one or more `orlojworker` instances that claim and execute tasks. (Or use `--embedded-worker` for single-process development.)
3. **Define your system** -- write declarative YAML manifests for agents, tools, policies, and the agent graph.
4. **Submit a task** -- apply a Task resource. The scheduler assigns it to a worker, which executes the agent graph and returns results.

**Server** (`orlojd`) -- API server, resource storage (in-memory or Postgres), background services, and task scheduler.

**Workers** (`orlojworker`) -- task execution, model gateway routing, tool runtime with isolation, and message bus consumers.

**Governance** -- AgentPolicy, AgentRole, and ToolPermission resources enforced inline during every tool call and model interaction.

You interact with Orloj through `orlojctl` (the CLI), the REST API, or the built-in web console.

## Key Capabilities

| Capability                  | Description                                                                                   |
| --------------------------- | --------------------------------------------------------------------------------------------- |
| **Agents-as-Code**          | Declarative YAML manifests for agents, systems, tools, policies, and tasks                    |
| **DAG-based orchestration** | Pipeline, hierarchical, and swarm-loop topologies with fan-out/fan-in support                 |
| **Model routing**           | Per-agent model binding via ModelEndpoint resources (OpenAI, Anthropic, Azure OpenAI, Ollama) |
| **Tool isolation**          | Container, WASM, or sandboxed execution with configurable timeouts and retry                  |
| **Governance and RBAC**     | AgentPolicy, AgentRole, and ToolPermission with fail-closed enforcement                       |
| **Task scheduling**         | Cron-based schedules and webhook-triggered task creation from external events                 |
| **Reliability**             | Lease-based ownership, idempotent replay, capped retry with jitter, dead-letter transitions   |
| **Observability**           | Task trace, message lifecycle, per-agent/per-edge metrics, and live event streaming           |
| **Web console**             | Built-in UI with topology views, task inspection, and command palette                         |

## Scope

Orloj focuses on production orchestration for multi-agent systems. It assumes you have already chosen your models, prompts, and tools -- Orloj's job is to run them reliably at scale with governance enforced.

Orloj does not train or fine-tune models, provide prompt authoring workflows, or serve as a user-facing chatbot framework. While it uses a Kubernetes-style resource model (`apiVersion`, `kind`, `spec`, `status`), it is a standalone system and does not require a Kubernetes cluster.

## Get Started

1. [Install Orloj](./getting-started/install.md) -- run from source, build binaries, or use Docker Compose.
2. [Quickstart](./getting-started/quickstart.md) -- deploy a multi-agent pipeline in under five minutes.
3. [Explore Concepts](./concepts/) -- understand agents, tasks, tools, governance, and the execution model.
4. [Follow a Guide](./guides/) -- step-by-step tutorials for common workflows.
