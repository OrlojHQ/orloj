# Orloj Enterprise Observability Boundary

Intended location: root of private `orloj-enterprise` repository (observability module).

## Allowed Scope

- **Cost tracking and attribution** — Model pricing tables mapping `(provider, model)` to per-token costs. Per-task, per-agent, and per-tenant cost rollups, budget alerting, and cost anomaly detection. Built on the existing `MeteringSink` extension and per-step token tracking in `TaskTraceEvent`.

- **Managed Grafana dashboards** — Pre-built, versioned dashboard packs for task health, agent execution waterfalls, token/cost trends, error rates, and SLO burn-rate panels. Translates the existing dashboard contract specs (`monitoring/dashboards/`) into production Grafana JSON with Tempo and Prometheus datasources.

- **Advanced metering sink implementations** — Production `MeteringSink` that writes to a managed time-series store with configurable retention policies, RBAC, and export to billing systems. The OSS no-op default remains unchanged.

- **Multi-tenant observability isolation** — Per-namespace and per-tenant metrics partitioning, dashboard scoping, and alert routing so teams see only their own operational data.

- **Alertmanager rule packs** — Curated Prometheus alerting rules derived from `orloj-alertcheck` profiles, with escalation policies and integrations for PagerDuty, Slack, OpsGenie, and email.

- **SLO management** — SLI/SLO definitions for task completion rate, agent latency, and error rates with burn-rate alerting and error-budget tracking dashboards.

- **Audit trail analytics** — Production `AuditSink` implementation with immutable storage, compliance reporting, query UI, and configurable retention/export for regulatory requirements.

- **Trace analytics and search** — Cross-task trace search, slow-path analysis, and failure correlation across agent graphs. Built on OTel data exported by the OSS layer; adds indexed storage, a query API, and a search UI.

## Guardrails

- Do not move previously open OSS observability primitives behind a paywall. OpenTelemetry spans, Prometheus `/metrics`, the trace waterfall UI tab, and structured logging (`log/slog`) must remain in OSS.
- Implement enterprise features through OSS extension contracts (`MeteringSink`, `AuditSink`, future `TracingSink`) wherever possible.
- Keep OSS self-host runtime fully functional without enterprise observability components.

## Integration Rules

- Consume OSS extension contracts from `orloj`.
- Do not patch or fork OSS core internals for enterprise-only observability behavior.
- Preserve compatibility with published OSS interface/version contracts.
