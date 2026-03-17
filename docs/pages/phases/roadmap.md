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

The `Memory` CRD exists and agents can reference it via `spec.memory.ref`, but the runtime integration surface is limited. Evaluate whether to flesh out the vector-store retrieval implementation or explicitly scope the Memory resource as experimental with a graduation timeline.

## Pre-Launch: Tool Platform

Tool Platform 2-6 are the remaining pre-launch milestones. They are sequenced -- each builds on the previous. All other milestones (Phases 10-16) are post-launch.

### Tool Platform 2: Remaining Runtime Work

Current state: three isolation backends exist (`none`, `container`, `wasm`). The `container` backend runs curl in Docker. The `wasm` backend runs wasmtime as a subprocess. But `mode=none` uses `MockToolClient` which returns fake output and never makes real HTTP calls. Only the `http` tool type is supported. `Tool.spec.type` is not validated at apply time.

Deliverables:

- **Real HTTP tool executor for `mode=none`**: Replace `MockToolClient` in `runtime/tool_runtime_governed.go` with an actual HTTP client so tools work without container/wasm isolation. This is the base runtime path used when `isolation_mode=none`.
- **External tool executor mode**: Add a `Tool.spec.type=external` value and a delegator runtime that forwards `ToolExecutionRequest` to a configured endpoint via HTTP/gRPC. Enables tools to run as standalone services outside the Orloj process.
- **Additional `Tool.spec.type` adapters**: Add `grpc` adapter (call a gRPC service endpoint) and at least one async adapter (`queue` or `webhook-callback`). Each adapter goes through the governed runtime pipeline and interoperates with existing policy controls.
- **Sandbox defaults hardening**: Document and enforce secure defaults for `sandboxed` isolation mode (network policy, resource limits, filesystem restrictions). Container backend already has memory/CPU flags -- make them secure-by-default.
- **Tool type validation**: Add validation in `crds/resource_types.go` that rejects unknown `Tool.spec.type` values at apply time.

Exit criteria:
- Runtime wiring no longer depends on mode-specific core switch edits.
- Resource governance defaults are secure-by-default and documented.
- New tool types are contract-tested and interoperable with existing policy controls.

Test gate:
- Runtime conformance suites cover all supported execution modes.
- New adapter implementations include unit, integration, and policy-behavior tests.

### Tool Platform 3: Tool Auth and Secret Binding

Current state: `Tool.spec.auth.secretRef` exists and resolves via `StoreSecretResolver` -> `EnvSecretResolver` chain (`runtime/tool_secret_resolver.go`). Container backend injects a bearer token as `TOOL_AUTH_BEARER`. WASM passes auth via the execution envelope. Redaction is implemented in `runtime/redact.go`. But auth is limited to a single `secretRef` that resolves to one bearer token value.

Deliverables:

- **Per-tool auth profiles**: Expand `Tool.spec.auth` beyond single `secretRef` to support multiple auth modes -- bearer token, API key header, basic auth, OAuth2 client credentials. Each mode has its own secret binding shape and injection semantics.
- **Rotation-aware secret resolution**: Define explicit rotation semantics. Currently each resolution is a fresh store lookup (correct), but the contract should specify rotation behavior for long-running tasks and cached resolution in message-driven paths.
- **Auth failure classification**: Map auth failures (expired token, invalid credentials, 401/403 from tool endpoint) into the canonical tool error taxonomy (`runtime/tool_error.go`) with deterministic `tool_code` values like `auth_expired`, `auth_invalid`, `auth_forbidden`.
- **Auth audit fields in traces**: Extend `TaskTraceEvent` to include auth metadata on tool calls (secret name used, auth mode) without leaking the actual credential. Ensure redaction covers all auth injection points across all backends.

Exit criteria:
- Tool auth behavior is deterministic and documented by contract.
- Auth failures classify into canonical tool error taxonomy.

Test gate:
- Secret resolution and redaction tests pass across runtime backends.
- Contract and trace-parsing tests validate auth metadata and failure mapping.

### Tool Platform 4: Policy Hooks and Risk-Tier Routing

Current state: `AgentPolicy`, `AgentRole`, `ToolPermission` exist and are enforced at tool call time via `AgentToolAuthorizer`. Denials are fail-closed with deterministic reason codes. But there is no concept of operation classes or risk tiers -- authorization is all-or-nothing per tool.

Deliverables:

- **Tool operation class annotations**: Allow tools to declare operation classes in `Tool.spec` (e.g. `read`, `write`, `delete`, `admin`). Policy evaluation considers the operation class, not just tool name. `ToolPermission` can grant access to specific operation classes.
- **Risk-tier routing**: `ToolPermission` or `AgentPolicy` can specify `allow`, `deny`, or `approval_required` per operation class. `approval_required` pauses the tool call and emits a pending-approval event.
- **Human approval workflow**: Define an approval mechanism (resource, status field, or API endpoint). When a tool call requires approval, the task transitions to a waiting state. An external actor (human or system) approves/denies via API. The task resumes or fails accordingly.
- **Policy reason codes**: Extend the existing denial reason codes to cover approval-related outcomes (`approval_pending`, `approval_denied`, `approval_timeout`).
- **Retry/deadletter behavior**: Approval timeouts and denials are non-retryable. Approval-pending is a pausable state that does not consume retry budget.

Exit criteria:
- High-risk operations can be blocked pending approval by policy.
- Policy outcomes are observable with deterministic reason codes.

Test gate:
- Governance policy tests cover allow/deny/approval flows.
- Retry/deadletter behavior is correct for approval-related denials/timeouts.

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
