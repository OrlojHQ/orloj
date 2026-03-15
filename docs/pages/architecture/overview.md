# Architecture Overview

This page describes the core control-plane and worker runtime architecture.

## Layers

1. Control plane
- API server
- CRD storage backends (`memory` or `postgres`)
- scheduler/controllers

2. Worker data plane
- `orlojworker` task execution
- model gateway and router (`Agent.spec.model_ref` -> `ModelEndpoint`)
- tool runtime with isolation backends
- task/message bus consumers

3. Governance and safety
- `AgentPolicy` enforcement hooks
- `AgentRole` and `ToolPermission` authorization
- secret-backed provider/tool auth paths
- deterministic denial/error classification

## Runtime Modes

- `sequential`: controller-driven execution flow.
- `message-driven`: worker-consumer execution with queued message handoff.

## Reliability Characteristics

- lease-based task ownership
- owner-only message execution with takeover on lease expiry
- idempotency tracking for replay/crash recovery
- capped exponential retry with jitter
- explicit dead-letter terminal transitions

## Related Docs

- [Execution and Messaging](./execution-model.md)
- [Runbook](../operations/runbook.md)
- [CRD Reference](../reference/crds.md)
