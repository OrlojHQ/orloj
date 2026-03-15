# Security and Isolation

This page describes current runtime security controls and expected operator practices.

## Current Controls

- `AgentPolicy` gates model/tool/token usage.
- `AgentRole` and `ToolPermission` enforce per-tool authorization.
- Tool runtime enforces timeout/retry/isolation policy from `Tool.spec.runtime`.
- Unsupported tools and disallowed runtime requests fail closed.
- Permission denials are terminal for the current execution path.

## Isolation Modes

- `none`
- `sandboxed`
- `container`
- `wasm`

Container backend supports constrained execution for high-risk paths.

WASM backend uses executor-factory boundaries and command-backed runtime execution (default runtime binary `wasmtime`). Invalid wasm runtime configuration fails closed with deterministic non-retryable policy errors.

## Secret Handling

- secrets are namespace-scoped `Secret` resources
- `spec.data` values are base64-encoded
- `stringData` supports write-time plaintext input
- runtime resolves `Tool.spec.auth.secretRef` and injects auth into isolated execution
- raw secret values must not appear in logs or trace payloads

## Operational Requirements

- enforce least-privilege tool permissions
- validate redaction behavior during incident drills
- monitor denial and runtime policy error trends
- integrate approval hooks for high-risk operations before OSS cut

## Related Docs

- [Security Policy](../project/security-policy.md)
- [Tool Contract v1](../reference/tool-contract-v1.md)
- [Tool Runtime Conformance](./tool-runtime-conformance.md)
