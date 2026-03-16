# Getting Started

Install Orloj, run a multi-agent pipeline, and see results -- all in under five minutes.

## Recommended Path

1. **[Install](./install.md)** -- build from source or use Docker Compose.
2. **[Quickstart](./quickstart.md)** -- start the server, apply a starter blueprint, and watch a pipeline execute.
3. **[Production Checklist](./production-checklist.md)** -- readiness gates before broader rollout.

The quickstart uses **sequential mode** with an embedded worker -- a single process, no external dependencies. When you are ready to scale, the [Quickstart](./quickstart.md#scaling-to-production) shows how to graduate to message-driven mode with distributed workers.

## What You Will Build

The quickstart applies the **pipeline blueprint** -- a three-agent graph (`planner -> research -> writer`) that demonstrates how Orloj orchestrates multi-agent workflows. See [Starter Blueprints](../architecture/starter-blueprints.md) for hierarchical and swarm-loop patterns.

## Prerequisites

- Go `1.24+`
- Docker (optional, needed for container-isolated tools)
