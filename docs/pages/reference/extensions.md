# Extension Contracts

Orloj core exposes optional extension hooks for additive cloud and enterprise integrations.

## Design Rules

- OSS defaults remain functional with no extension hooks configured.
- Extension behavior is additive and must not alter baseline OSS semantics.
- Consumers should target public interfaces instead of patching core internals.

## Runtime Interfaces

- `MeteringSink`
  - `RecordMetering(ctx, MeteringEvent)`
  - for usage and billing pipelines
- `AuditSink`
  - `RecordAudit(ctx, AuditEvent)`
  - for external audit pipelines
- `CapabilityProvider`
  - `Capabilities(ctx) CapabilitySnapshot`
  - used by `GET /v1/capabilities`

## Compatibility Expectations

- interfaces evolve additively by default
- breaking changes require versioning and migration guidance
- compatibility checks should run against pinned consumer references before release

## Related Docs

- [Versioning and Deprecation](../project/versioning-and-deprecation.md)
- [Cloud Boundary](../boundaries/agentops-cloud.BOUNDARY.md)
- [Enterprise Boundary](../boundaries/agentops-enterprise.BOUNDARY.md)
- [Plugins Boundary](../boundaries/agentops-plugins.BOUNDARY.md)
