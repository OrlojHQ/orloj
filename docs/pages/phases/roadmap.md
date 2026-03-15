# Roadmap

This roadmap is forward-looking only. Completed delivery history is tracked in `docs/pages/phases/phase-log.md`.

## Core-First Execution Model

- Core (`orloj`) remains the primary focus until OSS release gate criteria are met.
- Closed-source sibling repos are used as compatibility signals, not feature-priority drivers.
- Backward-compatible, additive contract evolution is the default policy.

## Release Gate 0: Core OSS Readiness

Gate 0 is a hard blocker for OSS launch and must pass before broad post-core expansion.
Non-goal for Gate 0: multi-control-plane high availability; defer HA implementation to later reliability hardening.

### 0.1 Extensibility Readiness

Deliverables:
- Stable versioned extension contracts for runtime and API integrations.
- Explicit compatibility policy for contract evolution and versioning.
- Consumer-facing conformance checks that validate extension assumptions.

Done means:
- Contract docs are current and linked from `docs/pages/index.md`.
- A no-unversioned-breaking-change CI guard exists for extension surfaces.
- Compatibility checks are green against pinned `orloj-cloud`, `orloj-enterprise`, and `orloj-plugins` integration expectations.

Test gate:
- `go test ./...` in core.
- Core contract/conformance suites pass.
- Consumer compatibility smoke jobs pass for pinned references.

### 0.2 Reliability Readiness

Deliverables:
- SLO-backed load and failure quality gates are published and enforced.
- Deferred operational follow-through from prior phases is completed:
  - schedule/integration path for `cmd/orloj-alertcheck`
  - production dashboard mapping for `monitoring/dashboards/retry-deadletter-overview.json`
- Backup/restore verification and upgrade/canary validation paths are defined.

Done means:
- Quality-profile gates are part of release validation.
- Alert and dashboard paths are wired into operator workflow.
- Reliability runbooks include rollback and incident-response checks.

Test gate:
- Load and failure-injection scenarios pass quality thresholds.
- Alert checks and dashboard contracts validate in CI or release verification.
- Backup/restore and upgrade validation tests pass.

### 0.3 Security Readiness

Deliverables:
- Rotation-aware secret resolution behavior for tool and provider auth paths.
- Redaction and auth audit field standards enforced across logs/traces.
- Approval-hook foundation for high-risk tool operations.

Done means:
- Sensitive fields are consistently redacted by policy.
- Security-critical tool actions emit deterministic reason codes.
- High-risk execution classes support approval-required routing hooks.

Test gate:
- Security regression tests pass for secret resolution, redaction, and policy errors.
- Runtime/tool conformance tests verify canonical error and denial semantics.

### 0.4 DX and Documentation Readiness

Deliverables:
- Clean install/run paths for local and production-style deployments.
- Operator runbooks and contract docs updated for release.
- Versioning and deprecation policy documented with upgrade guidance.

Done means:
- All release-critical docs are link-complete from `docs/pages/index.md`.
- Examples are runnable from repository root.
- Release notes and migration notes templates are in place.

Test gate:
- Documentation link checks and scenario walkthrough checks pass.
- Example manifests and commands validate against current binaries.

### 0.5 API/CRD Stability Policy

Deliverables:
- Public surface lifecycle labels for APIs and CRDs (`experimental`, `beta`, `stable`).
- Deprecation/removal windows for each stability level.
- CI API/schema diff guard that blocks unversioned breaking changes.

Done means:
- Public API and CRD docs declare lifecycle status and compatibility expectations.
- Deprecation policy includes minimum support window and migration-note requirements.
- Breaking API/schema changes require explicit versioning and approval path.

Test gate:
- CI contract/API diff checks fail on unversioned breaking changes.
- Policy compliance checks validate lifecycle labels and deprecation metadata in docs.

### 0.6 Draft-to-Stable Contract Graduation

Deliverables:
- Graduation plan for release-critical Draft specs to either stable or explicitly experimental with timeline:
  - `docs/pages/reference/tool-contract-v1.md`
  - `docs/pages/reference/wasm-tool-module-contract-v1.md`
  - `docs/pages/operations/tool-runtime-conformance.md`
  - release-gated sections of load-testing and monitoring docs
- Versioned contract ownership and change-control expectations.

Done means:
- No release-critical contract remains ambiguous Draft at OSS launch.
- Any intentionally experimental contract has explicit scope limits and graduation timeline.
- `docs/pages/index.md` and reference links reflect final stability state.

Test gate:
- Contract conformance suites are mapped to each stabilized contract version.
- Docs checks validate that contract status labels and linked references are consistent.

### 0.7 OSS Security Operations

Deliverables:
- Public security disclosure workflow and response SLA.
- CVE triage and patch-release process for security fixes.
- Incident ownership and external communication path.

Done means:
- Security reporting instructions and timelines are published and actionable.
- Security patch process is included in release checklist and runbooks.
- Responsible disclosure flow is tested internally.

Test gate:
- Release checklist enforces security process artifacts.
- Security incident-response tabletop or dry-run verification completes successfully.

### 0.8 OSS Governance and Support

Deliverables:
- Community support path and maintainer/reviewer operating model.
- Release cadence/support policy and contribution decision model.
- Escalation path for breaking-change approvals.

Done means:
- Governance/support docs are publish-ready and linked from primary docs entrypoints.
- Breaking-change decisions follow a documented approval path.
- Contributor experience is defined without private tribal process.

Test gate:
- Docs completeness check validates governance/support presence and links.
- Release process check validates governance gates were applied to contract-breaking proposals.

### 0.9 Supply-Chain and Provenance

Deliverables:
- SBOM generation for release artifacts.
- Signed binaries and images with verification instructions.
- Provenance/attestation checks in release CI.

Done means:
- Every release artifact has verifiable origin and integrity metadata.
- Supply-chain checks are mandatory in pre-release pipeline.
- Verification steps are documented for operators/users.

Test gate:
- SBOM, signature, and provenance checks pass before release tag creation.
- Dependency/vulnerability policy gate passes with documented exceptions.

### 0.10 Operability Baseline

Deliverables:
- Control-plane operability baseline for OSS operators: readiness, health, metrics, and trace/log correlation conventions.
- Operator-facing SLI definitions and runbook mappings for core control-plane behavior.
- Baseline operational checks wired into release verification.

Done means:
- Operators can assess control-plane health from documented endpoints/signals.
- Core operational runbooks map alerts/signals to concrete remediation actions.
- Operability expectations are explicit for single-control-plane deployments.

Test gate:
- Operability smoke checks validate readiness/health and baseline telemetry signals.
- Runbook validation scenarios pass for startup, degraded behavior, and recovery.

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
- Multi-control-plane HA hardening (leader election/failover) as post-Gate-0 optional reliability work.

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
- Repository watcher/reconciler for Orloj manifests (`Git -> Orloj API`).
- Auto-apply with optional auto-prune for deleted manifests.
- Drift detection, sync status/history, and rollback-friendly revision tracking.
- Policy controls for branch/path scope and sync safety guards.

Exit criteria:
- GitOps sync is deterministic, auditable, and safe-by-default.

Test gate:
- Drift/reconcile/rollback integration scenarios pass.

## Contract Stability Track (Cross-Cutting)

This track operationalizes Release Gate 0.1 and 0.5 across the full delivery lifecycle.

Deliverables:
- Contract compatibility matrix maintenance and release-time verification workflow.
- CI enforcement and policy ownership for contract/API diff guardrails.
- Release checklist integration requiring consumer compatibility signals.

Done means:
- Contract changes require compatibility notes and versioning rationale.
- Core changes are validated against pinned consumer expectations.

Test gate:
- Contract diff checks and compatibility smoke checks pass in release CI.

## Pre-OSS Packaging Track (Cross-Cutting)

This track operationalizes Release Gate 0.7 and 0.9 for release mechanics.

Deliverables:
- Reproducible release artifacts with deterministic build inputs.
- Release-note, changelog, and migration-note workflow.
- Security/process checkpoint integration into release pipeline.

Done means:
- Release process is automated, documented, and auditable.

Test gate:
- Reproducibility and signature verification checks pass before release tags.
