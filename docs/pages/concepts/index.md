# Concepts

This section explains the core building blocks of Orloj and how they fit together. Each concept page covers what a resource is, why it exists, how to configure it, and how it interacts with the rest of the system.

If you are new to Orloj, start with the [Architecture Overview](../architecture/overview.md) to understand the system's layers, then work through the concepts below.

## At a Glance

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

## Core Resources

**[Agents and Agent Systems](./agents-and-systems.md)** -- Agents are declarative units of work backed by language models. Agent Systems compose agents into directed graphs (pipelines, hierarchies, swarm loops) that Orloj executes as coordinated workflows.

**[Tasks and Scheduling](./tasks-and-scheduling.md)** -- Tasks are requests to execute an Agent System. They carry input, track execution state through a well-defined lifecycle, and support cron-based scheduling and webhook-triggered creation.

**[Tools and Isolation](./tools-and-isolation.md)** -- Tools are external capabilities that agents invoke during execution. Orloj provides a standardized tool contract, four isolation backends (none, sandboxed, container, WASM), and configurable timeout and retry.

**[Model Routing](./model-routing.md)** -- ModelEndpoints decouple agents from specific model providers. Configure connections to OpenAI, Anthropic, Azure OpenAI, or Ollama, and bind agents to endpoints by reference.

**[Governance and Policies](./governance.md)** -- AgentPolicy, AgentRole, and ToolPermission resources enforce authorization at the execution layer. The governance model is fail-closed: unauthorized tool calls are denied, not silently ignored.

## Architecture and Execution

**[Architecture Overview](../architecture/overview.md)** -- The three-layer architecture: server, workers, and governance.

**[Execution and Messaging](../architecture/execution-model.md)** -- Graph routing, fan-out/fan-in, message lifecycle, ownership guarantees, and tool selection.

**[Starter Blueprints](../architecture/starter-blueprints.md)** -- Ready-to-run pipeline, hierarchical, and swarm-loop topologies with example manifests.
