# Orloj OSS Boundary

This repository (`orloj`) is the permanently open-source core under Apache-2.0.

## Forever Open

- CRDs and resource schemas.
- API server CRUD/status/watch/events endpoints.
- Scheduler/controllers and worker lifecycle.
- Sequential and message-driven task execution runtimes.
- Core governance primitives, including `AgentRole` and `ToolPermission`.
- Baseline built-in web UI.
- Self-host docs, examples, and local/prod deployment paths.

## Prohibited In OSS Core

- License-gating logic in core runtime paths.
- Forced phone-home requirements for core execution.
- Artificial usage caps in OSS (workers/tasks/users/runs).

## Commercial Extension Rule

- Commercial capabilities must integrate through OSS extension interfaces.
- Do not fork/patch OSS internals for private repos unless explicitly approved and upstreamed.
