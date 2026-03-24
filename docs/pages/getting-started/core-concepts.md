# Core Concepts

This page introduces Orloj's key building blocks and how they fit together. Read this before diving into the individual concept pages.

## Resource Map

```
                  TaskSchedule ──creates──▶ Task ◀──creates── TaskWebhook
                                             │
                                          triggers
                                             ▼
                                        AgentSystem
                                        ╱          ╲
                                   composes      composes
                                     ╱                ╲
                                Agent A ─────────── Agent B
                               ╱   │   ╲           ╱   │
                          calls  invokes reads  calls invokes
                            ╱      │    ╲       ╱      │
                   ModelEndpoint  Tool  Memory  │      │
                        │          │            │      │
                   resolves    resolves          │      │
                    auth via    auth via         │      │
                        ╲       ╱               │      │
                         Secret                 │      │
                                                │      │
              ┄┄┄┄┄┄┄┄ Governance ┄┄┄┄┄┄┄┄┄┄┄┄┄┤┄┄┄┄┄┄┤
              ┆                                 ┆      ┆
        AgentPolicy ┄┄ constrains ┄┄▶ Agent A, Agent B
        AgentRole   ┄┄ grants permissions to ┄▶ Agents
        ToolPermission ┄ controls access to ┄▶ Tools

              Worker ──claims and executes──▶ Task
```

## Agents

An [**Agent**](../concepts/agents/agent.md) is a declarative unit of work backed by a language model. You define its prompt, model, tools, and constraints in YAML.

```yaml
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  prompt: "You are a research assistant."
  tools: [web_search]
  limits:
    max_steps: 6
```

## Agent Systems

An [**AgentSystem**](../concepts/agents/agent-system.md) composes agents into a directed graph -- pipelines, hierarchies, or swarm loops.

```yaml
kind: AgentSystem
metadata:
  name: report-system
spec:
  agents: [planner, researcher, writer]
  graph:
    planner:
      edges: [{to: researcher}]
    researcher:
      edges: [{to: writer}]
```

## Tasks

A [**Task**](../concepts/tasks/task.md) is a request to execute an AgentSystem. Tasks track lifecycle state (`Pending` -> `Running` -> `Succeeded`), support retry, and produce output.

```yaml
kind: Task
metadata:
  name: weekly-report
spec:
  system: report-system
  input:
    topic: AI startups
```

## Tools

A [**Tool**](../concepts/tools/tool.md) is an external capability agents can invoke. Six transport types (HTTP, external, gRPC, webhook-callback, MCP, queue) and four isolation modes (none, sandboxed, container, WASM).

```yaml
kind: Tool
metadata:
  name: web_search
spec:
  type: http
  endpoint: https://api.search.com
  auth:
    secretRef: search-api-key
```

## Model Endpoints

A [**ModelEndpoint**](../concepts/tools/model-endpoint.md) configures a connection to a model provider (OpenAI, Anthropic, Azure OpenAI, Ollama). Agents reference endpoints by name, decoupling agent definitions from provider details.

```yaml
kind: ModelEndpoint
metadata:
  name: openai-default
spec:
  provider: openai
  default_model: gpt-4o-mini
  auth:
    secretRef: openai-api-key
```

## Memory

[**Memory**](../concepts/memory/) gives agents persistent storage across execution steps and task runs. Three layers: conversation history, task-scoped shared state, and persistent backends (in-memory, pgvector, HTTP).

## Governance

The [**governance layer**](../concepts/governance/) controls what agents can do at runtime:

- [**AgentPolicy**](../concepts/governance/agent-policy.md) -- constrain models, block tools, cap tokens
- [**AgentRole**](../concepts/governance/agent-role.md) -- grant named permissions to agents
- [**ToolPermission**](../concepts/governance/tool-permission.md) -- require permissions to invoke tools

Governance is fail-closed: unauthorized tool calls are denied, not silently ignored.

## Automation

- [**TaskSchedule**](../concepts/tasks/task-schedule.md) -- create tasks on a cron schedule
- [**TaskWebhook**](../concepts/tasks/task-webhook.md) -- create tasks from external HTTP events

## Infrastructure

- [**Worker**](../concepts/infrastructure/worker.md) -- execution unit that claims and runs tasks
- [**Secret**](../concepts/tools/secret.md) -- stores API keys and credentials
- [**McpServer**](../concepts/tools/mcp-server.md) -- connects to MCP servers and auto-discovers tools

## Next Steps

- [Architecture Overview](../concepts/architecture.md) -- understand the three-layer architecture
- [Deploy Your First Pipeline](../guides/deploy-pipeline.md) -- build and run a multi-agent pipeline
- [Explore Concepts](../concepts/agents/agent.md) -- dive into individual resource pages
