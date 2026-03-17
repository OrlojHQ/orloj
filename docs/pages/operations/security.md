# Security and Isolation

This page describes current runtime security controls and expected operator practices.

## Current Controls

- `AgentPolicy` gates model/tool/token usage.
- `AgentRole` and `ToolPermission` enforce per-tool authorization.
- Tool runtime enforces timeout/retry/isolation policy from `Tool.spec.runtime`.
- Unsupported tools and disallowed runtime requests fail closed.
- Permission denials are terminal for the current execution path.

## Tool Types

All tool types (`http`, `external`, `grpc`, `webhook-callback`) flow through the governed runtime pipeline, so policy enforcement, retry, auth injection, and error handling behave identically regardless of transport. See [Tools and Isolation](../concepts/tools-and-isolation.md) for type details.

## Isolation Modes

- `none` -- direct execution with real HTTP/gRPC calls (no isolation boundary)
- `sandboxed` -- restricted container with secure defaults (see below)
- `container` -- per-invocation isolated container
- `wasm` -- WebAssembly module with host-guest stdin/stdout boundary

Container backend supports constrained execution for high-risk paths.

WASM backend uses executor-factory boundaries and command-backed runtime execution (default runtime binary `wasmtime`). Invalid wasm runtime configuration fails closed with deterministic non-retryable policy errors.

### Sandboxed Container Defaults

When `isolation_mode=sandboxed` (the default for `high`/`critical` risk tools), the container backend enforces these security constraints:

| Control | Value |
|---|---|
| Filesystem | `--read-only` |
| Linux capabilities | `--cap-drop=ALL` |
| Privilege escalation | `--security-opt no-new-privileges` |
| Network | `--network none` |
| User | `65532:65532` (non-root) |
| Memory | `128m` |
| CPU | `0.50` cores |
| Process limit | `64` PIDs |

These defaults are enforced by `SandboxedContainerDefaults()` in the runtime and validated by conformance tests. Override with `--tool-container-*` flags only when necessary.

## Secret Handling

Orloj resolves secrets referenced by `secretRef` fields (on ModelEndpoint and Tool resources) using a chain of resolvers, tried in order:

1. **Resource Store** -- looks up a `Secret` resource by name and reads the base64-encoded value from `spec.data`.
2. **Environment Variables** -- looks up `ORLOJ_SECRET_<name>` (configurable prefix via `--model-secret-env-prefix` / `--tool-secret-env-prefix`).

The first resolver that returns a value wins.

### Development

Use `Secret` resources for local development. The fastest way is the imperative CLI command -- no YAML file needed:

```bash
orlojctl create secret openai-api-key --from-literal value=sk-your-key-here
```

Or with a YAML manifest:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: openai-api-key
spec:
  stringData:
    value: sk-your-key-here
```

### Encryption at Rest

When using the Postgres storage backend, enable encryption at rest for `Secret` resources by passing a 256-bit AES key to both `orlojd` and `orlojworker`:

```bash
# Generate a key (hex-encoded, 64 characters)
openssl rand -hex 32

# Pass via flag
orlojd --secret-encryption-key=<hex-key> ...
orlojworker --secret-encryption-key=<hex-key> ...

# Or via environment variable
export ORLOJ_SECRET_ENCRYPTION_KEY=<hex-key>
```

When enabled, all `Secret.spec.data` values are encrypted with AES-256-GCM before being written to the database and decrypted transparently on read. This protects secrets against direct database access, backup exposure, and log/dump leaks.

The key must be identical across all server and worker processes that share the same database. Both hex-encoded (64 characters) and base64-encoded (44 characters) formats are accepted.

**Without** an encryption key, `Secret` data is stored as base64-encoded plaintext in the JSONB payload -- suitable for development but not for production.

### Production

For production, choose one or both of the following approaches:

**1. Encrypted Secret resources** -- enable `--secret-encryption-key` and continue using `Secret` resources as in development. This is the simplest upgrade path.

**2. Environment variables** -- bypass `Secret` resources entirely by injecting provider keys into the runtime environment:

```bash
export ORLOJ_SECRET_openai_api_key="sk-prod-key"
```

The resolver normalizes the secret name: a `secretRef: openai-api-key` looks up `ORLOJ_SECRET_openai_api_key` (hyphens become underscores).

**3. External secret managers** -- inject secrets as environment variables using your platform's native mechanism:

- **Kubernetes**: Use [external-secrets-operator](https://external-secrets.io/) or the CSI secrets driver to sync Vault/AWS Secrets Manager/GCP Secret Manager values into pod env vars.
- **HashiCorp Vault**: Use [Vault Agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent) sidecar to render secrets into env or files.
- **Cloud providers**: Use AWS Secrets Manager, GCP Secret Manager, or Azure Key Vault with their respective injection mechanisms.

Approaches 2 and 3 do not require `Secret` resources -- the env-var resolver handles resolution directly.

### Security Requirements

- Raw secret values must not appear in logs or trace payloads.
- Store the encryption key itself in a secure location (e.g., a KMS, Vault, or hardware security module). Do not commit it to version control.
- Validate redaction behavior during incident drills.

## Tool Auth Profiles

Tools can authenticate using one of four profiles via `spec.auth.profile`:

| Profile | Suitable for | Notes |
|---------|-------------|-------|
| `bearer` (default) | API tokens, service keys | Injected as `Authorization: Bearer <token>` |
| `api_key_header` | APIs using custom header auth (e.g., `X-Api-Key`) | Requires `auth.headerName` |
| `basic` | Legacy HTTP basic auth | Secret must be `username:password` |
| `oauth2_client_credentials` | Machine-to-machine OAuth2 | Requires `auth.tokenURL`; uses multi-key secret with `client_id` and `client_secret` |

### Auth in Container Isolation

For container-isolated tools, auth is injected as environment variables rather than HTTP headers. The container's entrypoint script maps these to the appropriate `curl` headers:

| Env Var | Auth Profile |
|---------|-------------|
| `TOOL_AUTH_BEARER` | `bearer`, `oauth2_client_credentials` |
| `TOOL_AUTH_BASIC` | `basic` |
| `TOOL_AUTH_HEADER_NAME` + `TOOL_AUTH_HEADER_VALUE` | `api_key_header` |

### Auth Error Handling

Auth failures produce distinct error codes (`auth_invalid` for HTTP 401, `auth_forbidden` for HTTP 403) that are non-retryable. For `oauth2_client_credentials`, a 401 triggers automatic token cache eviction and one retry with a fresh token.

### Auth Audit Trail

Every tool invocation records `tool_auth_profile` and `tool_auth_secret_ref` (the secret name, not the resolved value) in the task trace. Use these fields for audit queries and compliance reporting.

## Operational Requirements

- Enforce least-privilege tool permissions.
- Monitor denial and runtime policy error trends.
- Monitor auth failure rates by profile for early detection of expired credentials.
- Approval hooks for high-risk operations are a post-launch roadmap item (Phase 12). See [Roadmap](../phases/roadmap.md).

## Related Docs

- [Security Policy](../project/security-policy.md)
- [Tool Contract v1](../reference/tool-contract-v1.md)
- [Tool Runtime Conformance](./tool-runtime-conformance.md)
