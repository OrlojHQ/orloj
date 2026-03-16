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

Orloj resolves secrets referenced by `secretRef` fields (on ModelEndpoint and Tool resources) using a chain of resolvers, tried in order:

1. **Resource Store** -- looks up a `Secret` resource by name and reads the base64-encoded value from `spec.data`.
2. **Environment Variables** -- looks up `ORLOJ_SECRET_<name>` (configurable prefix via `--model-secret-env-prefix` / `--tool-secret-env-prefix`).

The first resolver that returns a value wins.

### Development

Use `Secret` resources for local development. They are convenient for getting started but store base64-encoded values in the database (Postgres JSONB or in-memory), which is not encrypted at rest.

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-api-key
spec:
  stringData:
    value: sk-your-key-here
```

### Production

In production, use environment variables or an external secret manager that injects values into the worker environment:

**Environment variables (simplest)**
```bash
export ORLOJ_SECRET_openai_api_key="sk-prod-key"
```

The resolver normalizes the secret name: a `secretRef: openai-api-key` looks up `ORLOJ_SECRET_openai_api_key` (hyphens become underscores).

**External secret managers** -- inject secrets as environment variables using your platform's native mechanism:

- **Kubernetes**: Use [external-secrets-operator](https://external-secrets.io/) or the CSI secrets driver to sync Vault/AWS Secrets Manager/GCP Secret Manager values into pod env vars.
- **HashiCorp Vault**: Use [Vault Agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent) sidecar to render secrets into env or files.
- **Cloud providers**: Use AWS Secrets Manager, GCP Secret Manager, or Azure Key Vault with their respective injection mechanisms.

In all cases, the `Secret` resource is not needed -- the env-var resolver handles resolution directly.

### Security Requirements

- Raw secret values must not appear in logs or trace payloads.
- Validate redaction behavior during incident drills.

## Operational Requirements

- Enforce least-privilege tool permissions.
- Monitor denial and runtime policy error trends.
- Integrate approval hooks for high-risk operations before OSS cut.

## Related Docs

- [Security Policy](../project/security-policy.md)
- [Tool Contract v1](../reference/tool-contract-v1.md)
- [Tool Runtime Conformance](./tool-runtime-conformance.md)
