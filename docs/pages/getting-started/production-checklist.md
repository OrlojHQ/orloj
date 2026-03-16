# Production Checklist

Use this checklist before broad internal rollout and before OSS launch cut readiness.

## Runtime and Data Plane

- [ ] Use Postgres state backend (`--storage-backend=postgres`) for control plane and workers.
- [ ] Use durable message bus backend in message-driven mode (`nats-jetstream` recommended).
- [ ] Run at least two workers in production namespaces.
- [ ] Verify lease takeover behavior under worker interruption.

## Reliability Gates

- [ ] Run `orloj-loadtest` with quality profile enforcement.
- [ ] Run `orloj-alertcheck` against production-like namespaces.
- [ ] Validate retry/dead-letter thresholds against SLO targets.
- [ ] Validate backup/restore and upgrade runbooks for release candidates.

## Security and Governance

- [ ] Enforce `AgentPolicy`, `AgentRole`, and `ToolPermission` on target systems.
- [ ] Use secret-backed provider/tool auth (`Secret` + `secretRef`).
- [ ] Validate redaction and denial/audit metadata in trace/log paths.
- [ ] Validate approval-hook readiness for high-risk tool operations.

## Contracts and Documentation

- [ ] Keep contract docs aligned with runtime behavior.
- [ ] Ensure API/CRD lifecycle status and deprecation policy are published.
- [ ] Validate docs build in CI: `bun run docs:build`.

## Release Process

- [ ] Require green core tests and pinned consumer compatibility checks.
- [ ] Enforce SBOM, signature, and provenance checks.
- [ ] Publish release notes and migration notes for each release.

## Related Guides

- [Deployment Overview](../deployment/index.md)
- [VPS Deployment](../deployment/vps.md)
- [Kubernetes Deployment](../deployment/kubernetes.md)
- [Runbook](../operations/runbook.md)
- [Security and Isolation](../operations/security.md)
- [Release Process](../project/release-process.md)
- [Roadmap](../phases/roadmap.md)
