# Extension Contracts

> **Stability: beta** -- Extension interfaces are functional and in use, but may evolve additively in future releases.

Orloj core exposes optional extension hooks for additive cloud and enterprise integrations. Extensions allow commercial or custom features to plug into the runtime without modifying the open-source core.

## Design Rules

- OSS defaults remain functional with no extension hooks configured.
- Extension behavior is additive and must not alter baseline OSS semantics.
- Consumers should target public interfaces instead of patching core internals.

## Runtime Interfaces

### MeteringSink

Records usage and billing events for external consumption.

```go
type MeteringSink interface {
    RecordMetering(ctx context.Context, event MeteringEvent) error
}

type MeteringEvent struct {
    Timestamp   time.Time
    Namespace   string
    Task        string
    Agent       string
    Model       string
    TokensIn    int
    TokensOut   int
    ToolCalls   int
    DurationMs  int64
}
```

Use cases: usage-based billing, cost attribution per team/system, token consumption dashboards.

### AuditSink

Records audit events for compliance and observability pipelines.

```go
type AuditSink interface {
    RecordAudit(ctx context.Context, event AuditEvent) error
}

type AuditEvent struct {
    Timestamp  time.Time
    Action     string   // "tool_invoke", "policy_deny", "task_create", etc.
    Actor      string   // agent or user identifier
    Resource   string   // resource kind and name
    Namespace  string
    Outcome    string   // "allowed", "denied", "error"
    Details    map[string]string
}
```

Use cases: compliance logging, security audits, governance event streams.

### CapabilityProvider

Exposes deployment capabilities for feature discovery in UI and CLI integrations.

```go
type CapabilityProvider interface {
    Capabilities(ctx context.Context) (CapabilitySnapshot, error)
}

type CapabilitySnapshot struct {
    Features map[string]bool
    Metadata map[string]string
}
```

The snapshot is served at `GET /v1/capabilities`. Extension providers may add capabilities without changing the core API shape. The UI and CLI use this endpoint to enable or disable features based on what the deployment supports.

## Implementing an Extension

1. Implement one or more of the interfaces above.
2. Register the implementation with the runtime at startup (via configuration or plugin loading).
3. The runtime calls your implementation at the appropriate hook points during execution.

Extensions run in-process with the server or worker. They should be fast and non-blocking -- the runtime does not isolate extension failures from core execution.

## Compatibility Expectations

- Interfaces evolve additively by default.
- Breaking changes require versioning and migration guidance.
- Compatibility checks should run against pinned consumer references before release.

## Related Docs

- [Observability](../operations/observability.md) -- OSS tracing, metrics, and logging
