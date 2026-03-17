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

### Secret Encryption at Rest

Add optional encryption for `Secret.spec.data` values stored in the database. Accept a `--secret-encryption-key` flag and encrypt/decrypt transparently in the resource store layer. This makes the built-in Secret resource viable for production use alongside the existing env-var resolution path.

### Python SDK

Create a thin Python client library for the Orloj REST API. Target use cases: programmatic task submission, status polling, and result retrieval from Python-based ML/data pipelines.

### Scheduler Package Cleanup

`scheduler/scheduler.go` is a stub that returns the first graph node. Real scheduling logic lives in `TaskSchedulerController` (worker assignment) and `TaskController` (graph traversal and claim). Evaluate removing the package or consolidating the scheduling surface so the code structure matches the actual responsibility boundaries.

### TracingSink Extension Interface

Base OpenTelemetry integration is complete (OTLP export, span instrumentation on agent steps, model calls, tool execution, and message processing). Remaining work is adding a `TracingSink` extension interface alongside the existing `MeteringSink` and `AuditSink` so consumers can plug in custom trace processing.

### Memory Resource Maturity Review

The `Memory` CRD exists and agents can reference it via `spec.memory.ref`, but the runtime integration surface is limited. Evaluate whether to flesh out the vector-store retrieval implementation or explicitly scope the Memory resource as experimental with a graduation timeline.

## Active Milestones

### Tool Platform 2 (Remaining Runtime Work)

Deliverables:
- External tool executor mode integrated into unified runtime contract.
- Stronger sandbox and resource-governance defaults.
- Additional `Tool.spec.type` adapters beyond HTTP-first runtime (`grpc`, `queue/job`, `pod/service`-native).

Exit criteria:
- Runtime wiring no longer depends on mode-specific core switch edits.
- Resource governance defaults are secure-by-default and documented.
- New tool types are contract-tested and interoperable with existing policy controls.

Test gate:
- Runtime conformance suites cover all supported execution modes.
- New adapter implementations include unit, integration, and policy-behavior tests.

### Tool Platform 3

Deliverables:
- Per-tool auth profiles with explicit secret binding model.
- Rotation-aware secret resolution semantics.
- Standardized redaction and auth audit fields across runtime traces.

Exit criteria:
- Tool auth behavior is deterministic and documented by contract.
- Auth failures classify into canonical tool error taxonomy.

Test gate:
- Secret resolution and redaction tests pass across runtime backends.
- Contract and trace-parsing tests validate auth metadata and failure mapping.

### Tool Platform 4

Deliverables:
- Policy hooks for tool operation classes.
- Risk-tier routing (`allow`, `deny`, `approval_required`) for high-risk actions.
- Human approval workflow integration points.

Exit criteria:
- High-risk operations can be blocked pending approval by policy.
- Policy outcomes are observable with deterministic reason codes.

Test gate:
- Governance policy tests cover allow/deny/approval flows.
- Retry/deadletter behavior is correct for approval-related denials/timeouts.

### Tool Platform 5

Deliverables:
- Provider-agnostic tool SDK.
- Local tool simulator and conformance kit.
- Packaging/distribution guidance for internal and external tool implementations.

Exit criteria:
- New tool developers can implement and verify against a single documented flow.
- SDK and simulator match runtime contract requirements.

Test gate:
- SDK fixtures pass runtime conformance checks.
- Developer-path scenarios are validated in CI smoke jobs.

### Tool Platform 6

Deliverables:
- Per-tool latency/error/retry metrics with SLO targets.
- Tool execution tracing across task/message lifecycle.
- Reliability scorecards and alert thresholds.

Exit criteria:
- Tool-level SLO observability is production-usable.
- Alerting thresholds map to actionable runbooks.

Test gate:
- Metrics/tracing payload tests and alert-profile validations pass.
- Reliability scorecard generation is reproducible in CI.

### Phase 10: External Provider Runtime

Deliverables:
- Move provider plugins from in-process registration to external runtime model.
- Provider lifecycle health checks and isolation boundaries.
- Independent provider deployment and versioning model.

Exit criteria:
- Providers can evolve independently without core switch edits.
- Failure isolation is bounded and observable.

Test gate:
- Provider lifecycle and health-check integration tests pass.
- Compatibility checks validate built-in and external provider parity.

### Phase 11: Policy Engine

Deliverables:
- Expanded policy scopes (`model`, `tool`, `data`, `cost`, `execution`).
- Deterministic policy reason codes and evaluation traceability.

Exit criteria:
- Policy evaluation behavior is explicit, auditable, and reproducible.

Test gate:
- Cross-scope policy tests pass with deterministic reason-code assertions.

### Phase 12: Approvals and Audit

Deliverables:
- Approval policy resources and workflows.
- Immutable audit trail for sensitive model/tool actions.

Exit criteria:
- Sensitive operations are either approved or blocked with audit evidence.

Test gate:
- End-to-end approval lifecycle tests pass.
- Audit immutability and replay-verification tests pass.

### Phase 13: Multi-Tenancy and Quotas

Deliverables:
- Namespace or tenant budgets and concurrency quotas.
- Stronger tenant isolation for tools, secrets, and providers.

Exit criteria:
- Tenant-level controls are enforceable with clear denial telemetry.

Test gate:
- Quota/isolation tests pass under load and failure scenarios.

### Phase 14: Production Reliability

Deliverables:
- Backup/restore and disaster-recovery procedures with tests.
- Upgrade/canary strategies and reliability conformance suites.
- Multi-server HA hardening (leader election/failover) as post-Gate-0 optional reliability work.

Exit criteria:
- Operational recovery and upgrade safety are validated before release.

Test gate:
- DR exercises and upgrade/canary validation suites pass.

### Phase 15: Packaging and Platform DX

Deliverables:
- Deployment packaging for enterprise/self-host installs.
- Versioned docs pipeline and reference examples.

Exit criteria:
- Release artifacts and docs are reproducible and publish-ready.

Test gate:
- Packaging reproducibility checks pass in CI.
- Versioned docs build and link checks pass.

### Phase 16: GitOps Sync for Orloj Resources

Deliverables:
- Repository watcher/sync service for Orloj manifests (`Git -> Orloj API`).
- Auto-apply with optional auto-prune for deleted manifests.
- Drift detection, sync status/history, and rollback-friendly revision tracking.
- Policy controls for branch/path scope and sync safety guards.

Exit criteria:
- GitOps sync is deterministic, auditable, and safe-by-default.

Test gate:
- Drift/detect-and-sync/rollback integration scenarios pass.

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
