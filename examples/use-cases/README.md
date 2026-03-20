# Use case templates

Each subdirectory is a **copy-pasteable bundle**: YAML for every resource the scenario needs, plus its **own `README.md`** that explains **what the use case is for**, **who it suits**, **when to pick a different pattern**, **what you get**, file layout, and **`orlojctl apply` order**.

Resource names are prefixed (`uc-weekly-brief-*`, `uc-pmo-*`, …) so they do not collide with [starter blueprints](../blueprints/) if both are applied in the same cluster.

## How this relates to the rest of `examples/`

| Layer | What it is | When to use it |
| --- | --- | --- |
| **Use cases** (this folder) | End-to-end **templates** for a real story | Ship or adapt a full scenario quickly |
| **Blueprints** (`examples/blueprints/`) | Minimal topology references | Learn patterns with shorter names (`bp-*`) |
| **By kind** ([`resources/`](../resources/README.md)) | Single-resource samples (`resources/agents/`, …) | Learn schemas |

Operator tutorials: [Guides](../../docs/pages/guides/index.md).

## Prerequisites

1. **Edit secrets** in each folder’s `secret-*.yaml` before apply (OpenAI key, webhook HMAC secret).
2. **`ModelEndpoint` `openai-default`** is duplicated per use case folder so **one directory** is enough to copy; skip `model-endpoint.yaml` / `secret-openai.yaml` if you already defined them.
3. **Message-driven** execution with `--agent-message-consume` is recommended for blueprints-style graphs; **swarm** needs it for parallel fan-out ([Starter blueprints](../../docs/pages/architecture/starter-blueprints.md)).

## Scenarios

| Directory | Topology | Audience |
| --- | --- | --- |
| [weekly-intelligence-brief](./weekly-intelligence-brief/README.md) | Pipeline + optional `TaskSchedule` | Novice / small team |
| [cross-functional-pmo](./cross-functional-pmo/README.md) | Hierarchical + `wait_for_all` merge | Enterprise-style delegation |
| [roadmap-synthesis-swarm](./roadmap-synthesis-swarm/README.md) | Swarm + loop (`max_turns`) | Product / strategy |
| [event-driven-webhook](./event-driven-webhook/README.md) | Pipeline + `Task` template + `TaskWebhook` | Event-driven automation |

## Resource kinds

Typical bundles include: `Agent`, `AgentSystem`, `ModelEndpoint`, `Secret`, `Task`, `TaskSchedule`, `TaskWebhook`. Extend with `Memory`, `Tool`, `AgentPolicy`, `AgentRole`, `ToolPermission`, `Worker`, `McpServer` from [`examples/`](../README.md) and the [resource reference](../../docs/pages/reference/resources.md).
