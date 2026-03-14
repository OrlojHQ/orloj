# orloj Documentation

Orloj is a Kubernetes-style control plane for agents, tools, policies, and task execution.

## Start Here

- [Architecture Overview](./architecture/overview.md)
- [Execution and Messaging](./architecture/execution-model.md)
- [Starter Blueprints](./architecture/starter-blueprints.md)
- [CRD Reference](./reference/crds.md)
- [Tool Contract V1 (Draft)](./reference/tool-contract-v1.md)
- [WASM Tool Module Contract V1 (Draft)](./reference/wasm-tool-module-contract-v1.md)
- [API Reference](./reference/api.md)
- [Extension Contracts](./reference/extensions.md)
- [Operations Runbook](./operations/runbook.md)
- [Real Tool Validation (Anthropic Decision Gate)](./operations/real-tool-validation.md)
- [Security and Isolation](./operations/security.md)
- [Load Testing (Draft)](./operations/load-testing.md)
- [Monitoring and Alerts (Draft)](./operations/monitoring-alerts.md)
- [Tool Runtime Conformance (Draft)](./operations/tool-runtime-conformance.md)
- [Runtime Test Scenarios](../testing/scenarios/README.md)
- [Real-Model Runtime Scenarios](../testing/scenarios-real/README.md)
- [Phase Log](./phases/phase-log.md)
- [Roadmap](./phases/roadmap.md)

## Current Focus Areas

- Parallel graph execution (fan-out/fan-in with joins)
- Durable message lifecycle and ownership safety
- Retry policy hardening and dead-letter handling
- Runtime observability APIs and UI diagnostics
- Isolated tool execution and namespaced secret-backed auth
- Deterministic tool error taxonomy (`tool_status`, `tool_code`, `tool_reason`, `retryable`)
- Strict tool runtime request/response contract (`tool_contract_version=v1`) and shared conformance harness
- Bounded timeout/cancel tool runtime semantics (governed/container/wasm)
- Conformance case catalog and wasm runtime scaffold interfaces for backend expansion
- Backend registry hooks for isolated runtimes (no core switch edits for new backends)
- Command-backed wasm runtime executor path (`wasmtime` default) with pluggable executor factory boundary
- Strict wasm guest I/O contract (`contract_version=v1`) and reference WASI guest module
- Model-selected tool invocation (authorized subset) instead of auto-running all configured tools
- Native structured tool-call parsing now available across `openai`, `azure-openai`, `anthropic`, and `ollama`
- Agent role/tool permission governance enforcement
- Multi-provider model routing via namespaced ModelEndpoint resources (`openai`, `anthropic`, `azure-openai`, `ollama`, `mock`, plus registry-registered custom providers)

## Roadmap Note

- Model provider plugins are currently in-process registrations; external runtime provider plugins are planned for future phases.
- Tool contract/versioning uses `v1` as the evolving baseline and should be updated in place unless a new major is explicitly approved.
