# Extension Contracts

Orloj core exposes optional extension hooks for additive cloud/enterprise integrations.

All hooks default to OSS-safe no-op/local behavior when not configured.

## Runtime Interfaces

- `MeteringSink`
  - Method: `RecordMetering(ctx, MeteringEvent)`
  - Use for usage/billing pipelines.
- `AuditSink`
  - Method: `RecordAudit(ctx, AuditEvent)`
  - Use for external audit pipelines.
- `CapabilityProvider`
  - Method: `Capabilities(ctx) CapabilitySnapshot`
  - Used by `GET /v1/capabilities` for feature discovery.

## Compatibility

- Extension interfaces are additive and backward compatible by default.
- OSS core behavior must remain unchanged when extension hooks are omitted.
- Commercial/private integrations should consume these contracts instead of patching core internals.
