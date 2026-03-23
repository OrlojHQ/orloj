# Security and Isolation

This page describes current runtime security controls and expected operator practices.

## Current Controls

- `AgentPolicy` gates model/tool/token usage.
- `AgentRole` and `ToolPermission` enforce per-tool authorization.
- Tool runtime enforces timeout/retry/isolation policy from `Tool.spec.runtime`.
- Unsupported tools and disallowed runtime requests fail closed.
- Permission denials are terminal for the current execution path.

## Namespace Isolation

Namespaces are an **organizational boundary**, not a security boundary. Any authenticated user with the correct role (e.g., `reader`, `writer`, `admin`) can access resources in any namespace. There is no per-namespace access control in the OSS build.

For deployments that require per-namespace or per-resource authorization, the server exposes a `ResourceAuthorizer` extension point (see `ServerOptions.ResourceAuthorizer` in `api/auth_context.go`). An enterprise RBAC layer can implement this interface to enforce fine-grained policies based on the caller's identity, the target namespace, resource type, and HTTP method. In the OSS build this hook is nil and all requests that pass the role check are permitted.

## Control plane API tokens

The HTTP API (including `orlojctl`) authenticates automation with **`Authorization: Bearer <token>`** when you enable token validation on the server. Orloj **does not** mint or email API keys: the **operator** chooses a secret string, configures it on `orlojd`, and distributes the **same** value to people and CI that need API access.

**See also:** [Remote CLI and API access](../deployment/remote-cli-access.md) — end-to-end flow for self-hosters (env vars, `orlojctl config`, `config.json` lifecycle).

This is separate from **native UI sign-in** (`--auth-mode=native`), which uses an admin username/password and **session cookies** in the browser. The CLI does not use that password for API calls; use a bearer token as below (or run with auth disabled in trusted dev environments only).

### 1. Generate a token

Use a cryptographically random value (length is flexible; treat it like a password):

```bash
# Hex (64 characters); easy to paste into env files
openssl rand -hex 32

# Or base64 (~44 characters)
openssl rand -base64 32
```

Store the output in your secrets manager, Kubernetes Secret, or password manager—**not** in git.

### 2. Configure the server

Pick **one** of these (same token string you generated):

- **Flag:** `orlojd --api-key='<token>'`
- **Environment:** `ORLOJ_API_TOKEN='<token>'` (also read when `--api-key` is unset; see server help)

For **multiple** distinct tokens with different roles (reader vs admin-style access), use:

```bash
export ORLOJ_API_TOKENS='reader-token-here:reader,automation-token-here:admin'
```

Format is comma-separated `token:role` pairs. When `ORLOJ_API_TOKENS` is set, it populates the token map and a single `ORLOJ_API_TOKEN` is only used if that list is empty (see `loadAuthConfig` in `api/authz.go`).

### 3. Configure clients (`orlojctl` and automation)

Use the **same** token the server expects:

- **Environment:** `ORLOJ_API_TOKEN` or `ORLOJCTL_API_TOKEN`
- **Flag:** `orlojctl --api-token '<token>' ...`
- **Profile:** `orlojctl config set-profile ... --token-env VAR` so the token stays in the environment, not on disk

See [Remote CLI and API access](../deployment/remote-cli-access.md) for client precedence, default `--server` resolution, and profiles.

### 4. Native auth mode and APIs

If you use `--auth-mode=native`, the UI still requires a bearer token (or session cookie) for protected API routes. Configure `ORLOJ_API_TOKEN` / `--api-key` on the server so `orlojctl` and other API clients can authenticate with `Authorization: Bearer`—the admin password alone is not used for programmatic access.

### 5. Initial setup protection

When deploying with `--auth-mode=native` on a network-exposed instance, set `ORLOJ_SETUP_TOKEN` to prevent unauthorized admin account creation. When this variable is set, the `/v1/auth/setup` endpoint requires a matching `setup_token` field in the JSON request body:

```json
{
  "username": "admin",
  "password": "...",
  "setup_token": "your-setup-token-here"
}
```

The comparison uses constant-time comparison to prevent timing side-channels. Without `ORLOJ_SETUP_TOKEN`, the setup endpoint is open to the first caller (protected only by rate limiting).

### 6. Authentication rate limiting

Authentication endpoints (`/v1/auth/login`, `/v1/auth/setup`, `/v1/auth/change-password`, `/v1/auth/admin-reset-password`) are rate-limited per client IP address. The default policy allows 10 requests per minute sustained with a burst of 20 to accommodate legitimate multi-step flows. Requests that exceed the limit receive HTTP 429.

## Tool Types

All tool types (`http`, `external`, `grpc`, `webhook-callback`) flow through the governed runtime pipeline, so policy enforcement, retry, auth injection, and error handling behave identically regardless of transport. See [Tools and Isolation](../concepts/tools-and-isolation.md) for type details.

### gRPC TLS

gRPC tool connections require TLS (minimum TLS 1.2) by default. Plaintext gRPC is available as an opt-in for development environments only. Production deployments should always use the default TLS transport.

### SSRF Protection

Outbound HTTP, gRPC, and MCP connections validate the target endpoint before connecting. Requests to the following IP ranges are blocked by default:

- Loopback addresses (`127.0.0.0/8`, `::1`)
- Link-local addresses (`169.254.0.0/16`, `fe80::/10`)
- Cloud metadata endpoints (`169.254.169.254`)

Private network addresses (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`) are also blocked unless explicitly allowed. These checks apply when the host is a literal IP address; hostname-based endpoints are validated at the network dialer level.

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

### API Redaction

The REST API never returns plaintext secret data. All `GET` responses for `Secret` resources replace every value in `spec.data` with `"***"`. This applies to both individual resource fetches and list responses. Secret data is write-only through the API; to verify a secret value, use the resource it references (e.g., test a model endpoint or tool that depends on it).

Event bus messages for secret create/update operations are also redacted before publication.

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

## Risk-Tier Routing and Approvals

Tools can declare operation classes (`read`, `write`, `delete`, `admin`) via `spec.operation_classes`. Policy rules in `ToolPermission.spec.operation_rules` define per-class verdicts: `allow`, `deny`, or `approval_required`.

When a tool call triggers `approval_required`:
- The task enters `WaitingApproval` phase.
- A `ToolApproval` resource is created for the pending decision.
- An operator approves or denies via the REST API.
- Approval outcomes produce `approval_pending`, `approval_denied`, or `approval_timeout` error codes.

All approval-related outcomes are non-retryable and do not consume retry budget.

### Operational Guidance

- Use `operation_rules` with `verdict: approval_required` for destructive operations (`delete`, `admin`) in production environments.
- Set appropriate TTLs on `ToolApproval` resources (default: 10 minutes) to prevent tasks from waiting indefinitely.
- Monitor `WaitingApproval` task counts and approval latencies to detect bottlenecks.

## Operational Requirements

- Enforce least-privilege tool permissions.
- Monitor denial and runtime policy error trends.
- Monitor auth failure rates by profile for early detection of expired credentials.
- Monitor approval request volume and response latency for `WaitingApproval` tasks.

## Related Docs

- [Tool Contract v1](../reference/tool-contract-v1.md)
- [Tool Runtime Conformance](./tool-runtime-conformance.md)
