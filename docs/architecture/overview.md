# Architecture Overview

## Layers

1. Control plane
- API server
- CRD storage backends (memory or Postgres)
- scheduler/controllers

2. Data plane
- agent workers
- runtime execution engine
- tool runtime and model gateway
- model router (`Agent.spec.model_ref` -> `ModelEndpoint`)
- provider plugins (currently in-process registry; external runtime plugins planned)
- task/message buses

3. Governance and safety
- AgentPolicy enforcement hooks
- tool capability and risk metadata
- isolated tool execution paths
- secret-based tool authentication

## Key Runtime Modes

- `sequential`: controller-driven in-process execution order.
- `message-driven`: worker-consumer execution with queued handoff messages and retries.

## Primary Reliability Guarantees

- worker lease-based task ownership
- strict owner-only message execution (with takeover on lease expiry)
- durable idempotency tracking in task status
- retry with capped exponential backoff and jitter
- explicit dead-letter transitions for terminal failures
