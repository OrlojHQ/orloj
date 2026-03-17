# Roadmap

This roadmap is forward-looking only. Completed delivery history is tracked in `docs/pages/phases/phase-log.md`.

## Core-First Execution Model

- Core (`orloj`) remains the primary focus until OSS release gate criteria are met.
- Closed-source sibling repos are used as compatibility signals, not feature-priority drivers.
- Backward-compatible, additive contract evolution is the default policy.

## Release Gate 0: Deferred Items

Gate 0 is largely complete. The following items were evaluated during the pre-launch audit and deferred to post-launch hardening. See `docs/pages/phases/phase-log.md` for completed Gate 0 work.

### 0.2 Reliability Readiness

Load and failure-injection tooling (`orloj-loadtest`, `orloj-alertcheck`) works for manual validation. Remaining work is CI integration for SLO-backed quality gates and automated backup/restore verification.

Deliverables:
- SLO-backed load and failure quality gates enforced in CI.
- Backup/restore verification and upgrade/canary validation paths.

### 0.6 Draft-to-Stable Contract Graduation

Contracts exist, pass conformance suites, and are documented. Remaining work is formal graduation labeling and versioned change-control process.

Deliverables:
- Graduation plan for Draft specs (`tool-contract-v1`, `wasm-tool-module-contract-v1`, `tool-runtime-conformance`) to stable or explicitly experimental with timeline.
- Versioned contract ownership and change-control expectations.

### 0.9 Supply-Chain and Provenance

Release-pipeline work with no application-code changes required.

Deliverables:
- SBOM generation for release artifacts.
- Signed binaries and images with verification instructions.
- Provenance/attestation checks in release CI.

## Post-Launch Engineering

Items planned for after OSS launch. These address architectural improvements identified during the pre-launch review.

### Database Schema Normalization

Move from the single `resources` JSONB table to dedicated tables per resource type with typed columns. Enables proper indexing, foreign-key constraints, and query performance at scale. The single-table design works for launch but will become a bottleneck.

### Worker API Gateway

Workers currently connect directly to Postgres. Introduce an API-mediated path where workers interact with the server through the REST API or a lightweight RPC layer. This decouples workers from the database, simplifies network security, and enables the server to enforce admission control on worker operations.

### Python SDK

Create a thin Python client library for the Orloj REST API. Target use cases: programmatic task submission, status polling, and result retrieval from Python-based ML/data pipelines.

### Scheduler Package Cleanup

`scheduler/scheduler.go` is a stub that returns the first graph node. Real scheduling logic lives in `TaskSchedulerController` (worker assignment) and `TaskController` (graph traversal and claim). Evaluate removing the package or consolidating the scheduling surface so the code structure matches the actual responsibility boundaries.

### TracingSink Extension Interface

Base OpenTelemetry integration is complete (OTLP export, span instrumentation on agent steps, model calls, tool execution, and message processing). Remaining work is adding a `TracingSink` extension interface alongside the existing `MeteringSink` and `AuditSink` so consumers can plug in custom trace processing.

### Memory Resource Maturity Review

The `Memory` resource exists and agents can reference it via `spec.memory.ref`, but the runtime integration surface is limited. Evaluate whether to flesh out the vector-store retrieval implementation or explicitly scope the Memory resource as experimental with a graduation timeline.

## Pre-Launch: Tool Platform

Tool Platform 2-6 are the remaining pre-launch milestones. They are sequenced -- each builds on the previous. All other milestones (Phases 10-16) are post-launch.

### Tool Platform 5: Tool SDK and Developer Experience

Current state: tool contract is defined in `runtime/tool_contract.go` (`ToolExecutionRequest`/`ToolExecutionResponse`). Conformance harness exists in `runtime/conformance/harness.go`. WASM reference module exists in `examples/tools/wasm-reference/`. A guide exists at `docs/pages/guides/build-custom-tool.md`. But there is no standalone SDK package or developer-facing test tooling.

Deliverables:

- **Provider-agnostic tool SDK**: A Go package that handles the `ToolExecutionRequest`/`ToolExecutionResponse` contract, including envelope validation, error taxonomy mapping, and retry semantics. Developers import this and implement a handler function.
- **Local tool simulator**: A standalone binary or test harness that sends `ToolExecutionRequest` payloads to a tool endpoint and validates responses against the contract. Developers use this to test their tool implementations locally without running the full Orloj stack.
- **Conformance kit**: Package the existing `runtime/conformance/harness.go` as a reusable test kit with a CLI wrapper. Tool developers run this against their implementations to verify contract compliance.
- **Developer docs**: Extend `docs/pages/guides/build-custom-tool.md` with SDK usage, simulator workflow, and conformance testing instructions.
- **Packaging guidance**: Document how to distribute tools (container images, WASM modules, HTTP services) with versioning and registry conventions.

Exit criteria:
- New tool developers can implement and verify against a single documented flow.
- SDK and simulator match runtime contract requirements.

Test gate:
- SDK fixtures pass runtime conformance checks.
- Developer-path scenarios are validated in CI smoke jobs.

### Tool Platform 6: Tool Observability

Current state: per-agent and per-edge metrics exist in the custom metrics API (`GET /v1/tasks/{name}/metrics`). Prometheus metrics exist for task/agent/message level (`telemetry/metrics.go`). OTel spans cover agent steps and tool execution (`telemetry/spans.go`). But there are no per-tool Prometheus metrics, no tool-level SLO targets, and no reliability scorecards.

Deliverables:

- **Per-tool Prometheus metrics**: Add `orloj_tool_call_duration_seconds` (histogram, labels: `tool`, `agent`, `status`), `orloj_tool_errors_total` (counter, labels: `tool`, `error_code`), `orloj_tool_retries_total` (counter, labels: `tool`). Instrument in the governed tool runtime.
- **Tool execution tracing**: Ensure OTel spans on tool calls include `tool.name`, `tool.type`, `tool.attempt`, `tool.error_code`, `tool.latency_ms` attributes. Link tool spans to parent agent step spans for end-to-end trace correlation.
- **SLO targets on `Tool.spec`**: Allow tools to declare latency/error-rate SLO targets (e.g. `spec.slo.p99_latency_ms`, `spec.slo.error_rate`). Emit metrics that can be compared against these targets for alerting.
- **Reliability scorecards**: A CLI command or API endpoint that computes per-tool reliability scores from recent metrics (success rate, p50/p99 latency, retry rate). Usable in CI or operator workflows.
- **Alert thresholds**: Default alert profile for tool SLO violations, extending the existing `monitoring/alerts/` convention.

Exit criteria:
- Tool-level SLO observability is production-usable.
- Alerting thresholds map to actionable runbooks.

Test gate:
- Metrics/tracing payload tests and alert-profile validations pass.
- Reliability scorecard generation is reproducible in CI.

## Post-Launch Milestones

These milestones are planned for after OSS launch.

### Phase 10: External Provider Runtime

Deliverables:
- Move provider plugins from in-process registration to external runtime model.
- Provider lifecycle health checks and isolation boundaries.
- Independent provider deployment and versioning model.

Exit criteria:
- Providers can evolve independently without core switch edits.
- Failure isolation is bounded and observable.

### Phase 11: Policy Engine

Deliverables:
- Expanded policy scopes (`model`, `tool`, `data`, `cost`, `execution`).
- Deterministic policy reason codes and evaluation traceability.

Exit criteria:
- Policy evaluation behavior is explicit, auditable, and reproducible.

### Phase 12: Approvals and Audit

Deliverables:
- Approval policy resources and workflows.
- Immutable audit trail for sensitive model/tool actions.

Exit criteria:
- Sensitive operations are either approved or blocked with audit evidence.

### Phase 13: Multi-Tenancy and Quotas

Deliverables:
- Namespace or tenant budgets and concurrency quotas.
- Stronger tenant isolation for tools, secrets, and providers.

Exit criteria:
- Tenant-level controls are enforceable with clear denial telemetry.

### Phase 14: Production Reliability

Deliverables:
- Backup/restore and disaster-recovery procedures with tests.
- Upgrade/canary strategies and reliability conformance suites.
- Multi-server HA hardening (leader election/failover).

Exit criteria:
- Operational recovery and upgrade safety are validated before release.

### Phase 15: Packaging and Platform DX

Deliverables:
- Deployment packaging for enterprise/self-host installs.
- Versioned docs pipeline and reference examples.

Exit criteria:
- Release artifacts and docs are reproducible and publish-ready.

### Phase 16: GitOps Sync for Orloj Resources

Deliverables:
- Repository watcher/sync service for Orloj manifests (`Git -> Orloj API`).
- Auto-apply with optional auto-prune for deleted manifests.
- Drift detection, sync status/history, and rollback-friendly revision tracking.
- Policy controls for branch/path scope and sync safety guards.

Exit criteria:
- GitOps sync is deterministic, auditable, and safe-by-default.

## Contract Stability Track (Cross-Cutting)

Ongoing work to maintain contract compatibility as the platform evolves. Gate 0.1 and 0.5 baseline requirements are met; this track covers continued enforcement.

Deliverables:
- CI enforcement for contract/API diff guardrails that block unversioned breaking changes.
- Contract compatibility matrix maintenance and release-time verification workflow.

## Release Packaging Track (Cross-Cutting)

Covers release mechanics for Gate 0.9 deliverables and ongoing release hygiene.

Deliverables:
- Reproducible release artifacts with deterministic build inputs.
- SBOM, signature, and provenance checks in release CI.
- Release-note, changelog, and migration-note workflow.
