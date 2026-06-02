# Security and Isolation

This page describes current runtime security controls and expected operator practices.

## Current Controls

- `AgentPolicy` gates model/tool/token usage.
- `AgentRole` and `ToolPermission` enforce per-tool authorization.
- `ToolApproval` and `TaskApproval` pause risky actions or sensitive outputs for explicit human review.
- Tool runtime enforces timeout/retry/isolation policy from `Tool.spec.runtime`.
- Unsupported tools and disallowed runtime requests fail closed.
- Permission denials are terminal for the current execution path.

For regulated environments, `TaskApproval` checkpoints add a second control plane beyond tool authorization: a human can review and approve, deny, or request changes on sensitive agent output before the workflow continues.

## Namespace Isolation

Namespaces are an **organizational boundary**, not a security boundary. Any authenticated user with the correct role (e.g., `reader`, `writer`, `admin`) can access resources in any namespace. There is no per-namespace access control by default.

For deployments that require per-namespace or per-resource authorization, the server exposes a `ResourceAuthorizer` extension point (see `ServerOptions.ResourceAuthorizer` in `api/auth_context.go`). A custom authorization layer can implement this interface to enforce fine-grained policies based on the caller's identity, the target namespace, resource type, and HTTP method. This hook is nil by default and all requests that pass the role check are permitted.

### Multi-tenant authorization (Enterprise)

Fine-grained, per-namespace tenant isolation — mapping principals to the namespaces they may access, with separation of duties between tenants — is provided by **Orloj Enterprise**, which ships a managed `ResourceAuthorizer` implementation along with SSO/SCIM identity mapping and audit integration. Contact the Orloj team for access.

Self-hosters on the open-source build can implement the `ResourceAuthorizer` interface themselves. The hook receives the caller's identity, HTTP method, resource type, namespace, and name, and returns an allow/deny decision:

```go
type ResourceAuthorizer interface {
    AuthorizeResource(r *http.Request, method, resourceType, namespace, name string) (allowed bool, statusCode int, message string)
}
```

A minimal policy reads the authenticated identity (`api.AuthIdentityFromRequest`), permits `admin`-role callers cluster-wide, and otherwise checks the caller against an allowed-namespace set. Wire your implementation in via `ServerOptions.ResourceAuthorizer`; it is nil by default, so all requests that pass the built-in role check are permitted.

For true multi-tenant isolation, combine namespace authorization with separate API tokens per tenant, network policies between workloads, and per-tenant secret scoping.

## Control plane API tokens

The HTTP API (including `orlojctl`) authenticates automation with **`Authorization: Bearer <token>`** when you enable token validation on the server. Orloj **does not** mint or email API keys: the **operator** chooses a secret string, configures it on `orlojd`, and distributes the **same** value to people and CI that need API access.

**See also:** [Remote CLI and API access](../deploy/remote-cli-access.md) — end-to-end flow for self-hosters (env vars, `orlojctl config`, `config.json` lifecycle).

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

Pick **one** of these (same token string you generated). **Prefer environment variables** over CLI flags — values passed via `--api-key`, `--secret-encryption-key`, or `--auth-reset-admin-password` are visible in process listings (`ps`) and the server logs a warning when a secret flag is used.

- **Environment (recommended):** `ORLOJ_API_TOKEN='<token>'`
- **Flag:** `orlojd --api-key='<token>'` (env fallback when unset; see server help)

For **multiple** distinct tokens with different roles (reader vs admin-style access), use:

```bash
export ORLOJ_API_TOKENS='reader-bot:reader-token-here:reader,automation-bot:automation-token-here:admin'
```

Format is comma-separated `name:token:role` entries. Legacy `token:role` entries are still accepted for backward compatibility. A2A invoke-only tokens use `name:token:a2a:namespace/system|other-system` and can invoke only those A2A-enabled AgentSystems. When `ORLOJ_API_TOKENS` is set, it populates the token map and a single `ORLOJ_API_TOKEN` is only used if that list is empty (see `loadAuthConfig` in `api/authz.go`).

For runtime-managed tokens (no server restart required), use:

```bash
orlojctl create token <name> --role <role>
orlojctl get tokens
orlojctl delete token <name>
```

When creating an `a2a` role token through the API, include `a2a_agent_systems` with the allowed AgentSystem refs. Native-auth browser sessions are not accepted for A2A JSON-RPC; external A2A callers must use bearer tokens.

### 3. Configure clients (`orlojctl` and automation)

Use the **same** token the server expects:

- **Environment:** `ORLOJ_API_TOKEN` or `ORLOJCTL_API_TOKEN`
- **Flag:** `orlojctl --api-token '<token>' ...`
- **Profile:** `orlojctl config set-profile ... --token-env VAR` so the token stays in the environment, not on disk

See [Remote CLI and API access](../deploy/remote-cli-access.md) for client precedence, default `--server` resolution, and profiles.

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

Authentication endpoints (`/v1/auth/login`, `/v1/auth/setup`, `/v1/auth/change-password`, `/v1/auth/admin/reset-password`) are rate-limited per client IP address. The default policy allows 10 requests per minute sustained with a burst of 20 to accommodate legitimate multi-step flows. Requests that exceed the limit receive HTTP 429.

#### Trusted proxy configuration

By default, the rate limiter ignores `X-Forwarded-For` and `X-Real-IP` headers and uses the TCP peer address (`RemoteAddr`) to identify clients. This prevents attackers from bypassing rate limits by rotating spoofed forwarding headers.

If Orloj runs behind a reverse proxy or load balancer, configure `--trusted-proxies` (env: `ORLOJ_TRUSTED_PROXIES`) with the CIDR(s) of your proxy so the server can extract the real client IP from forwarding headers:

```bash
# Single proxy
orlojd --trusted-proxies='10.0.0.0/8'

# Multiple proxies
orlojd --trusted-proxies='10.0.0.0/8,172.16.0.0/12'

# Single IP (treated as /32)
export ORLOJ_TRUSTED_PROXIES='192.168.1.50'
```

When trusted proxies are configured, `X-Forwarded-For` is parsed right-to-left: entries added by trusted proxies are skipped, and the first untrusted entry is used as the client IP. If the immediate peer is not in the trusted set, forwarding headers are ignored regardless of their content.

**Without `--trusted-proxies`**, all requests arriving through a proxy share a single rate-limit bucket (the proxy's IP). The server logs a warning when it detects forwarding headers but has no trusted proxies configured.

The same trust gate applies to `X-Forwarded-Proto` for session cookie security: the `Secure` flag is only set based on the forwarding header when the peer is a trusted proxy.

## A2A Security

When A2A protocol support is enabled, additional security considerations apply.

### SSRF Protection

Outbound A2A requests (fetching remote Agent Cards and sending JSON-RPC calls) use the same `SafeHTTPClient` and `ValidateEndpointURL` checks described in [SSRF Protection](#ssrf-protection). By default, loopback, link-local, cloud metadata, and private network addresses are blocked.

### Auth Enforcement

Inbound A2A JSON-RPC requests are subject to authentication based on the target AgentSystem's `spec.a2a.auth` policy:

- **`bearer`** (default): requires a valid bearer token with `a2a`, `writer`, or `admin` role. Scoped `a2a` tokens can only invoke systems listed in their `a2a_agent_systems`. This is the same enforcement as other protected API endpoints.
- **`public`**: allows unauthenticated A2A invoke for that specific system, even when instance-wide auth is configured. Control-plane APIs (`/v1/agents`, `/v1/tools`, etc.) remain protected. Invalid tokens are still rejected (401) — only missing tokens are permitted.

This enables a common production pattern: admin secret for `orlojctl` / control-plane APIs, plus public unauthenticated A2A invoke for selected systems, all on the same `orlojd` instance.

Agent Card discovery (GET) is always public regardless of `spec.a2a.auth`. Public systems' Agent Cards omit `authentication.schemes` so A2A clients know not to send tokens. The A2A registry (`GET /v1/a2a/agents`) shows only public systems to unauthenticated callers and all accessible systems to authenticated callers.

### Private Endpoint Risks

Setting `--a2a-allow-private-endpoints=true` (env: `ORLOJ_A2A_ALLOW_PRIVATE_ENDPOINTS`) permits outbound A2A requests to private and loopback IPs. This weakens SSRF protection and should only be enabled in trusted network environments (e.g., when remote A2A agents run on the same private network). Cloud metadata endpoints remain blocked regardless of this setting.

### Production Recommendations

- Keep `allowPrivateEndpoints` disabled unless remote agents are on a trusted private network.
- Use TLS for all A2A endpoints to protect task payloads in transit.
- Configure `a2a.rateLimit` to prevent abuse of inbound A2A JSON-RPC endpoints.
- Review remote agent URLs before adding them to `--a2a-remote-agents` or the Helm `a2a.remoteAgents` list.
- Monitor A2A request metrics for anomalous traffic patterns.

## Tool Types

All tool types (`http`, `external`, `grpc`, `webhook-callback`, `mcp`, `cli`, `wasm`, `a2a`) flow through the governed runtime pipeline, so policy enforcement, retry, timeout, and error handling behave identically regardless of transport. See [Tools](../concepts/tools/tool.md) for type details.

### gRPC TLS

gRPC tool connections require TLS (minimum TLS 1.2) by default. Plaintext gRPC is available as an opt-in for development environments only. Production deployments should always use the default TLS transport.

### SSRF Protection

Outbound HTTP, gRPC, and MCP connections validate the target endpoint twice: once at call time (URL parsing and scheme allowlist) and again at dial time via a `net.Dialer.Control` hook that inspects the actual IP the kernel is about to connect to. Dial-time enforcement closes the hostname-bypass and DNS-rebinding gaps that a URL-only check cannot catch. For generic tool and MCP egress, the following destinations are blocked regardless of configuration:

- Loopback addresses (`127.0.0.0/8`, `::1`, and IPv4-mapped IPv6 equivalents like `::ffff:127.0.0.1`)
- Link-local addresses (`169.254.0.0/16`, `fe80::/10`)
- Cloud metadata endpoints (`169.254.169.254` for AWS/GCP/Azure IMDS, `fd00:ec2::254` for AWS IMDSv2 IPv6)
- Unspecified addresses (`0.0.0.0`, `::`)

Private network addresses (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`) and RFC 6598 carrier-grade NAT (`100.64.0.0/10`) are also blocked unless the caller explicitly opts in.

`ModelEndpoint` resources use a model-gateway-specific safe client. Set `spec.allowPrivate: true` only for trusted local/private model servers; it permits loopback plus private/CGNAT addresses for model traffic while still blocking cloud metadata, link-local, and unspecified addresses. The default is `false` for all providers except `ollama`, which defaults to `true` because Ollama is a local-first runtime.

**Upgrading from earlier versions:** if you run an OpenAI-compatible server (vLLM, LM Studio, LocalAI, LiteLLM proxy, TGI, etc.) on localhost or a private network under `provider: openai-compatible`, add `spec.allowPrivate: true` to those `ModelEndpoint` resources before upgrading, or the gateway will fail at dial time with an error that names the resolved IP and the exact field to change.

### MCP Server Security

`McpServer` resources connect to external MCP (Model Context Protocol) servers that expose tools for agent use. Security considerations vary by transport:

- **stdio** (`transport: stdio`): The MCP server runs as a subprocess managed by Orloj. The `command` and `args` fields control exactly what binary is executed. The subprocess inherits only the environment variables explicitly listed in `spec.env` and resolved `spec.env[].secretRef` values -- no host environment leaks into the child process.
- **HTTP** (`transport: http`): The MCP server is a remote endpoint. SSRF validation (above) applies to the `spec.endpoint` URL, blocking loopback, link-local, and private-network targets by default. Use `spec.auth` to attach bearer or API-key credentials to outbound requests.

**Tool scoping:** Use `spec.tool_filter.include` to restrict which tools the MCP server exposes. Without a filter, all tools reported by `tools/list` are generated as `Tool` resources. In production, prefer an explicit allowlist to minimize attack surface.

**Credential injection:** Secrets referenced via `spec.env[].secretRef` follow the same [secret resolution chain](#secret-handling) as other resources. Avoid placing credentials in `spec.env[].value` plaintext fields outside of development.

**Governed runtime:** Tools discovered from MCP servers are generated as standard `Tool` resources with `spec.type: mcp`. They flow through the same governed runtime pipeline (policy enforcement, retry, auth injection, approvals) as all other tool types.

See [MCP Server concept](../concepts/tools/mcp-server.md) and the [Connect an MCP Server](../guides/connect-mcp-server.md) guide for setup details.

## Isolation Modes

- `none` -- direct execution with real HTTP/gRPC calls (no isolation boundary)
- `sandboxed` -- restricted container with secure defaults (see below)
- `container` -- per-invocation isolated container
- `kubernetes` -- ephemeral Kubernetes Job (see below)
- `wasm` -- WebAssembly module with host-guest stdin/stdout boundary

Container backend supports constrained execution for high-risk paths.

WASM backend uses executor-factory boundaries and command-backed runtime execution (default runtime binary `wasmtime`). Invalid wasm runtime configuration fails closed with deterministic non-retryable policy errors.

### Sandboxed Container Defaults

When `isolation_mode=sandboxed` (the default for `high`/`critical` risk tools), the container backend enforces these security constraints:

| Control              | Value                              |
| -------------------- | ---------------------------------- |
| Filesystem           | `--read-only`                      |
| Linux capabilities   | `--cap-drop=ALL`                   |
| Privilege escalation | `--security-opt no-new-privileges` |
| Network              | `--network none`                   |
| User                 | `65532:65532` (non-root)           |
| Memory               | `128m`                             |
| CPU                  | `0.50` cores                       |
| Process limit        | `64` PIDs                          |

These defaults are enforced by `SandboxedContainerDefaults()` in the runtime and validated by conformance tests. Override with `--tool-container-*` flags only when necessary.

### Kubernetes Isolation

When `isolation_mode: kubernetes`, tool invocations run as ephemeral Kubernetes Jobs. This provides cluster-native isolation without requiring a Docker socket on worker nodes.

### Agent Execution on Kubernetes

When `--agent-k8s-enabled=true`, each agent in a multi-agent task runs as an ephemeral Kubernetes Job. This isolates agent execution at the pod level.

**Security properties:**

- **Ephemeral Pods**: Each agent execution creates a dedicated Job and Pod. No state persists between executions.
- **Configurable service accounts**: Agent Pods run under a dedicated service account (`--agent-k8s-service-account`), separate from the orchestrator's service account.
- **Resource limits**: Default memory and CPU limits are enforced on every agent Pod (`--agent-k8s-default-memory`, `--agent-k8s-default-cpu`).
- **Timeout enforcement**: Agent-level `spec.limits.timeout` sets `activeDeadlineSeconds` on the Job.
- **Automatic cleanup**: Completed Jobs are garbage-collected via `ttlSecondsAfterFinished` (default: 600s).
- **Deterministic naming**: Job names are derived from the task, agent, and attempt, enabling crash recovery without orphaned resources.
- **Transparent fallback**: Agents with Docker-dependent tools (container isolation or stdio MCP servers with images) fall back to in-process execution automatically.
- **No privilege escalation**: Agent Pods do not mount the orchestrator's filesystem, Docker socket, or service account token.

**Operational guidance:**

- Use a dedicated namespace (`--agent-k8s-namespace`) to isolate agent Jobs.
- Apply NetworkPolicies to restrict agent Pod egress.
- Set resource quotas on the agent namespace to bound total resource consumption.
- Monitor Job completion and cleanup for stuck or leaked Pods.

**Security properties:**

- **Ephemeral Pods**: Each tool invocation creates a dedicated Job and Pod. No state persists between invocations.
- **Configurable service accounts**: Tool Pods run under a dedicated service account (`--tool-k8s-service-account`), enabling least-privilege RBAC for the tool workload itself.
- **Resource limits**: `spec.cli.resources` (memory, CPU) map to Kubernetes resource requests and limits on the Pod spec, enforced by the kubelet.
- **Timeout enforcement**: `spec.runtime.timeout` sets `activeDeadlineSeconds` on the Job, ensuring runaway tools are killed by the cluster.
- **Automatic cleanup**: Completed Jobs are garbage-collected via `ttlSecondsAfterFinished` (default: 300s, configurable with `--tool-k8s-job-ttl`).
- **Network isolation**: Kubernetes NetworkPolicies can restrict tool Pod egress/ingress independently of worker Pod network rules. This replaces the Docker `--network` flag.
- **No privilege escalation**: Tool Pods do not mount the worker's filesystem, Docker socket, or service account token.

**Operational guidance:**

- Apply NetworkPolicies to the tool Job namespace to restrict egress to only required endpoints.
- Use a dedicated namespace (`--tool-k8s-namespace`) to isolate tool Jobs from application workloads.
- Monitor Job completion rates and cleanup to detect stuck or leaked Pods.
- Set resource quotas on the tool namespace to bound total resource consumption from tool invocations.

### CLI Tool Isolation

CLI tools (`spec.type: cli`) default to `container` isolation regardless of risk level. Container network mode inherits the operator's `--tool-container-network` setting, which defaults to **`none`** (same as HTTP container tools). This keeps CLI tools network-isolated unless the operator or tool spec opts in.

Tools that call external APIs (e.g., `kubectl`, `gh`, `aws`) must set `spec.cli.network: bridge` (or another appropriate mode) explicitly.

**Security properties:**

- **No shell**: invocations use `exec.CommandContext` (execve-style argv). There is no `sh -c` path and no opt-in for shell mode.
- **Arg templates are per-entry**: each Go template produces exactly one argv element. No shell splitting or word expansion occurs.
- **Secrets via env_from only**: process environment is constructed exclusively from `spec.cli.env` (literals) and `spec.cli.env_from` (resolved secrets). No host environment variables leak into the container.
- **Binary allowlist** (optional): `--cli-tool-allowed-commands` rejects commands not on the list before exec.
- **Argv length limit**: `--cli-tool-max-argv-length` (default 4096 bytes) prevents oversized argument lists.
- **`spec.auth` rejected**: CLI tools must use `env_from` for credentials; setting `spec.auth` produces a validation error to prevent silent misconfiguration.

Set `spec.cli.network: bridge` (or another mode) when a CLI tool needs outbound network access. Leave it unset or set `none` for tools that operate on stdin/stdout only (e.g., `jq`, `yq`).

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

# Pass via environment variable (recommended)
export ORLOJ_SECRET_ENCRYPTION_KEY=<hex-key>
orlojd ...
orlojworker ...

# Or via flag (logs a warning; prefer env in production)
orlojd --secret-encryption-key=<hex-key> ...
orlojworker --secret-encryption-key=<hex-key> ...
```

When enabled, all `Secret.spec.data` values are encrypted with AES-256-GCM before being written to the database and decrypted transparently on read. This protects secrets against direct database access, backup exposure, and log/dump leaks.

The key must be identical across all server and worker processes that share the same database. Both hex-encoded (64 characters) and base64-encoded (44 characters) formats are accepted.

**Without** an encryption key, `Secret` data is stored as base64-encoded plaintext in the JSONB payload -- suitable for development but not for production.

On `orlojd`, the same `--secret-encryption-key` / `ORLOJ_SECRET_ENCRYPTION_KEY` setting also wraps the private key used for `SealedSecret` decryption when sealing is enabled. If no encryption key is configured, `SealedSecret` resources remain storable but reconcile to `Error`, and `GET /v1/sealing-key/public` returns `503`.

### Git-safe Sealed Secrets

`Secret` resources protect values in the API and optionally at rest in Postgres, but they are still plaintext manifests before apply. Use `SealedSecret` when you need to commit encrypted secret manifests to git.

The workflow is:

1. `orlojd` creates or loads one active sealing keypair in the control plane.
2. Clients fetch the public key from `GET /v1/sealing-key/public` or `orlojctl seal public-key`.
3. Clients convert a normal `Secret` manifest into a `SealedSecret` manifest locally with `orlojctl seal secret -f secret.yaml`, or generate one directly from literals with `orlojctl seal secret <name> --from-literal key=value`.
4. `orlojd` decrypts the `SealedSecret` and writes a normal `Secret` through the existing secret store path.
5. Workers continue to read the generated `Secret` exactly as they do for manually created secrets.

`SealedSecret` and the generated `Secret` use the same name and namespace in v1. Generated Secrets are marked with `orloj.dev/sealedsecret-owner=<namespace>/<name>`. If a Secret with that name already exists and is not owned by the same `SealedSecret`, reconcile fails closed instead of overwriting user-managed data.

Examples:

```bash
# Seal an existing Secret manifest into secret.sealed.yaml
orlojctl seal secret -f secret.yaml

# Generate a SealedSecret file directly from literals
orlojctl seal secret openai-api-key \
  --from-literal value=sk-prod-123 \
  --out secrets/openai-api-key.sealed.yaml
```

### Sealing Key Security Model

Orloj v1 uses one active control-plane sealing keypair per backing store.

- `orlojd` only generates a sealing key if no active key exists and `ORLOJ_SECRET_ENCRYPTION_KEY` is set. Startup loads an existing active key when present; it does not generate a new key on every restart.
- The generated sealing keypair is RSA-4096.
- The sealing private key is stored in Postgres encrypted with AES-256-GCM under `ORLOJ_SECRET_ENCRYPTION_KEY`.
- Each `SealedSecret` entry uses a fresh random 32-byte AES data key. The entry plaintext is encrypted with AES-256-GCM, and the AES data key is wrapped with RSA-OAEP-SHA256.
- The AES-GCM authenticated data binds the ciphertext to `<namespace>`, `<name>`, and the secret entry key. A ciphertext copied to a different secret identity will fail to decrypt.

Operationally, this means:

- A committed `SealedSecret` manifest is safe to store in git as long as the control-plane private key remains protected.
- If an attacker gets both the database and `ORLOJ_SECRET_ENCRYPTION_KEY`, they can recover the stored sealing private key.
- If an attacker gets code execution on `orlojd`, they can unseal secrets.
- Losing `ORLOJ_SECRET_ENCRYPTION_KEY` makes both encrypted `Secret` data and the stored sealing private key unrecoverable.
- Orloj v1 does not rotate sealing keys automatically yet; it keeps one active key until a future manual rotation flow is introduced.

### Production

For production, choose one or both of the following approaches:

**1. Encrypted Secret resources** -- enable `--secret-encryption-key` and continue using `Secret` resources as in development. This is the simplest upgrade path.

**2. SealedSecret manifests** -- keep declarative secret manifests in git without exposing plaintext. This works well when you want resource-driven configuration and reviewable manifests, but do not want plaintext `Secret` YAML in the repository.

**3. Environment variables** -- bypass `Secret` resources entirely by injecting provider keys into the runtime environment:

```bash
export ORLOJ_SECRET_openai_api_key="sk-prod-key"
```

The resolver normalizes the secret name: a `secretRef: openai-api-key` looks up `ORLOJ_SECRET_openai_api_key` (hyphens become underscores).

**4. External secret managers** -- inject secrets as environment variables using your platform's native mechanism:

- **Kubernetes**: Use [external-secrets-operator](https://external-secrets.io/) or the CSI secrets driver to sync Vault/AWS Secrets Manager/GCP Secret Manager values into pod env vars.
- **HashiCorp Vault**: Use [Vault Agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent) sidecar to render secrets into env or files.
- **Cloud providers**: Use AWS Secrets Manager, GCP Secret Manager, or Azure Key Vault with their respective injection mechanisms.

Approaches 3 and 4 do not require `Secret` resources -- the env-var resolver handles resolution directly.

### API Redaction

The REST API never returns plaintext secret data. All `GET` responses for `Secret` resources replace every value in `spec.data` with `"***"`. This applies to both individual resource fetches and list responses. Secret data is write-only through the API; to verify a secret value, use the resource it references (e.g., test a model endpoint or tool that depends on it).

Event bus messages for secret create/update operations are also redacted before publication.

`SealedSecret` resources are returned as ciphertext blobs. The API never exposes the control-plane private key.

### Security Requirements

- Raw secret values must not appear in logs or trace payloads.
- Store the encryption key itself in a secure location (e.g., a KMS, Vault, or hardware security module). Do not commit it to version control.
- Validate redaction behavior during incident drills.
- Back up `ORLOJ_SECRET_ENCRYPTION_KEY` separately from the database. Losing it prevents decrypting encrypted `Secret` values and the stored `SealedSecret` private key.

## Tool Auth Profiles

Tools can authenticate using one of four profiles via `spec.auth.profile`:

| Profile                     | Suitable for                                      | Notes                                                                                |
| --------------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `bearer` (default)          | API tokens, service keys                          | Injected as `Authorization: Bearer <token>`                                          |
| `api_key_header`            | APIs using custom header auth (e.g., `X-Api-Key`) | Requires `auth.headerName`                                                           |
| `basic`                     | Legacy HTTP basic auth                            | Secret must be `username:password`                                                   |
| `oauth2_client_credentials` | Machine-to-machine OAuth2                         | Requires `auth.tokenURL`; uses multi-key secret with `client_id` and `client_secret` |

### Auth in Container Isolation

For container-isolated tools, auth is injected as environment variables rather than HTTP headers. The container's entrypoint script maps these to the appropriate `curl` headers:

| Env Var                                            | Auth Profile                          |
| -------------------------------------------------- | ------------------------------------- |
| `TOOL_AUTH_BEARER`                                 | `bearer`, `oauth2_client_credentials` |
| `TOOL_AUTH_BASIC`                                  | `basic`                               |
| `TOOL_AUTH_HEADER_NAME` + `TOOL_AUTH_HEADER_VALUE` | `api_key_header`                      |

### Auth Error Handling

Auth failures produce distinct error codes (`auth_invalid` for HTTP 401, `auth_forbidden` for HTTP 403) that are non-retryable. For `oauth2_client_credentials`, a 401 triggers automatic token cache eviction and one retry with a fresh token.

### Auth Audit Trail

Every tool invocation records `tool_auth_profile` and `tool_auth_secret_ref` (the secret name, not the resolved value) in the task trace. Use these fields for audit queries and compliance reporting.

## Audit Logging

Orloj emits normalized audit events for security-relevant operations — admin token/user CRUD, approval decisions (`approved`, `denied`, `expired`, `changes_requested`) with reviewer identity, and other governed runtime actions. Each event carries a timestamp, component, action, outcome, principal, and the affected resource.

**Audit logging is off by default.** The runtime uses a no-op `AuditSink`, so unless you wire a sink, these events are produced but not persisted anywhere durable. This is intentional — Orloj does not assume a particular log destination — but it means **operators must opt in to retain an audit trail**.

### Reference: structured audit sink

A reference sink, `SlogAuditSink` (`runtime/audit_sink_slog.go`), writes audit events as structured JSON via `log/slog`. Wire it through `Extensions`:

```go
ext := agentruntime.Extensions{
    Audit: agentruntime.NewSlogAuditSink(nil), // JSON to stdout
}

server := api.NewServer(api.ServerOptions{
    Extensions: ext,
    // ... other options
})
```

Passing a custom `*slog.Logger` lets you direct records to a file or a collector. For production, forward these records to durable, append-only, access-controlled storage (e.g., a SIEM or log pipeline) rather than relying on container stdout alone.

### Retention and integrity guidance

- **Retain** audit records for at least as long as your compliance regime requires (commonly 1 year; longer for regulated environments).
- **Protect integrity:** ship audit records off-host to write-once or append-only storage so a compromised node cannot rewrite its own trail.
- **Restrict access** to audit storage to a separate role from the operators whose actions are being logged (separation of duties).
- **Validate redaction during drills:** confirm raw secret values never appear in audit records or traces.
- **Alert** on anomalous patterns (spikes in denials, approval overrides, admin token creation).

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

- [Tool](../concepts/tools/tool.md)
- [MCP Server](../concepts/tools/mcp-server.md)
- [Connect an MCP Server](../guides/connect-mcp-server.md)
