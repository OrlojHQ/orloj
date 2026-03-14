# Roadmap

## Near-Term Sequence

1. Phase 9: Operational Readiness
- load test harness and scenarios for message-driven multi-worker mode
- failure injection suite (lease expiry, worker crash, bus/retry stress)
- dashboards and alerts for retry storms and dead-letter growth

2. Tool Platform 1: Tool Architecture
- tool contract/schema versioning
- capability taxonomy and risk model hardening
- deterministic tool error model and denial reason taxonomy
- draft specs: `docs/reference/tool-contract-v1.md` and `docs/operations/tool-runtime-conformance.md`

3. Tool Platform 2: Tool Runtime
- unified executor contract across native/container/wasm/external modes
- stronger sandbox defaults and resource governance
- cancellation/timeout semantics and runtime conformance tests
- expand supported `Tool.spec.type` implementations beyond current HTTP-first runtime path (for example gRPC, queue/job, and pod/service-native adapters)

4. Tool Platform 3: Tool Auth and Secret Binding
- per-tool auth profiles and secret binding model
- rotation-aware secret resolution behavior
- standardized redaction and auth audit fields

5. Tool Platform 4: Tool Governance and Approvals
- policy hooks for each tool operation class
- risk-tier routing (allow, deny, approval required)
- human approval workflow integration for high-risk actions

6. Tool Platform 5: Tool Developer Experience
- provider-agnostic tool SDK
- local tool simulator and conformance test kit
- packaging/distribution pattern for internal and external tools

7. Tool Platform 6: Tool Observability and SLOs
- per-tool latency/error/retry metrics with SLO targets
- tool execution tracing across task/message lifecycle
- reliability scorecards and alert thresholds

8. Phase 10: External Provider Runtime
- move model provider plugins from in-process registration to external runtime plugins
- plugin lifecycle, health checks, and isolation boundaries
- independent provider deployment/versioning model

9. Phase 11: Policy Engine
- expanded policy scopes (model/tool/data/cost/execution)
- deterministic policy reason codes and evaluation traceability

10. Phase 12: Approvals and Audit
- explicit approval policy resources and workflows
- immutable audit trail for sensitive model/tool actions

11. Phase 13: Multi-Tenancy and Quotas
- namespace/tenant budgets and concurrency quotas
- stronger tenant isolation for tools, secrets, and providers

12. Phase 14: Production Reliability
- backup/restore and DR procedures with tests
- upgrade/canary strategies and reliability conformance suites

13. Phase 15: Packaging and Platform DX
- deployment packaging for enterprise installs
- docs site pipeline and versioned reference examples

14. Phase 16: GitOps Sync for Orloj Resources
- repository watcher/reconciler for Orloj manifests (Git -> Orloj API)
- auto-apply on commit with optional auto-prune for deleted manifests
- drift detection, sync status/history, and rollback-friendly revision tracking
- policy controls for branch/path scope and sync safety guards

## Notes

- Anthropic remains a first-class built-in provider.
- Current provider plugin model is in-process; external runtime providers are a planned evolution in Phase 10.
- Tool Platform phases are intentionally split out as a dedicated stream because tool safety, governance, and operability are core to enterprise adoption.
- Current production-ready tool execution path is HTTP-first; additional tool types are planned roadmap work and should be tracked as explicit runtime milestones.
- Versioning convention: update `v1` contracts in place unless a new major is explicitly approved.
- Deferred follow-up (explicitly postponed): wire `cmd/orloj-alertcheck` into scheduled ops execution and map `monitoring/dashboards/retry-deadletter-overview.json` into the production dashboard stack.
