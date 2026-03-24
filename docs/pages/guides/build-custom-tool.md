# Build a Custom Tool

This guide is for developers who need to extend agent capabilities by implementing a custom tool. You will implement the Tool Contract v1, register the tool as a resource, configure isolation and retry, and validate it with the conformance harness.

## Prerequisites

- Orloj server (`orlojd`) and at least one worker running
- `orlojctl` available
- Familiarity with the [Tools and Isolation](../concepts/tools/tool.md) concepts

## What You Will Build

A custom HTTP tool that agents can invoke during execution, registered with Orloj and configured with appropriate runtime controls.

## Step 1: Implement the Tool Contract

Every tool must accept a JSON request envelope and return a JSON response envelope.

**Request** (sent by the Orloj runtime to your tool):
```json
{
  "request_id": "req-abc-123",
  "tool": "my-custom-tool",
  "action": "invoke",
  "parameters": {
    "query": "example input"
  },
  "auth": {
    "type": "bearer",
    "token": "sk-..."
  },
  "context": {
    "task": "weekly-report",
    "agent": "research-agent",
    "attempt": 1
  }
}
```

**Success response** (returned by your tool):
```json
{
  "request_id": "req-abc-123",
  "status": "success",
  "result": {
    "data": "your tool output here"
  }
}
```

**Error response** (for retryable failures):
```json
{
  "request_id": "req-abc-123",
  "status": "error",
  "error": {
    "tool_code": "rate_limited",
    "tool_reason": "API rate limit exceeded",
    "retryable": true
  }
}
```

The error taxonomy includes `tool_code` (machine-readable), `tool_reason` (human-readable), and `retryable` (boolean). The runtime uses `retryable` to decide whether to retry or move to dead-letter.

## Step 2: Register the Tool

Create a Tool resource manifest:

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: my-custom-tool
spec:
  type: http
  endpoint: https://your-tool-service.internal/invoke
  capabilities:
    - custom.query.invoke
  operation_classes:
    - read
    - write
  risk_level: medium
  runtime:
    timeout: 10s
    retry:
      max_attempts: 3
      backoff: 1s
      max_backoff: 10s
      jitter: full
  auth:
    secretRef: my-tool-api-key
```

Apply:
```bash
orlojctl apply -f my-custom-tool.yaml
```

### Field Choices

**`risk_level`** -- Determines the default isolation mode:
- `low` / `medium`: defaults to `none` (direct execution)
- `high` / `critical`: defaults to `sandboxed`

**`operation_classes`** -- Declares the types of operations this tool performs. Valid values: `read`, `write`, `delete`, `admin`. Policy rules in `ToolPermission.operation_rules` can define per-class verdicts (`allow`, `deny`, `approval_required`). When omitted, defaults to `["read"]` for low/medium risk or `["write"]` for high/critical risk.

**`runtime.timeout`** -- How long the runtime waits for your tool to respond before treating the invocation as failed. Choose based on your tool's expected latency.

**`runtime.retry`** -- Configure retry behavior for transient failures. The `jitter: full` setting randomizes backoff intervals to prevent thundering herd effects when multiple agents hit the same tool.

## Step 3: Create a Secret and Configure Auth

If your tool requires authentication, create a Secret and set the auth profile on the Tool. Orloj supports four auth profiles:

### Bearer token (default)

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: my-tool-api-key
spec:
  stringData:
    value: your-api-key-here
```

```yaml
spec:
  auth:
    secretRef: my-tool-api-key
```

When `profile` is omitted, it defaults to `bearer`. The runtime injects `Authorization: Bearer <token>`.

### API key header

```yaml
spec:
  auth:
    profile: api_key_header
    secretRef: my-tool-api-key
    headerName: X-Api-Key
```

The runtime injects the secret value as `X-Api-Key: <value>`.

### Basic auth

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: my-basic-creds
spec:
  stringData:
    value: "username:password"
```

```yaml
spec:
  auth:
    profile: basic
    secretRef: my-basic-creds
```

The secret must contain `username:password`. The runtime base64-encodes it and injects `Authorization: Basic <encoded>`.

### OAuth2 client credentials

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: my-oauth-creds
spec:
  stringData:
    client_id: your-client-id
    client_secret: your-client-secret
```

```yaml
spec:
  auth:
    profile: oauth2_client_credentials
    secretRef: my-oauth-creds
    tokenURL: https://auth.provider.com/oauth/token
    scopes:
      - read
      - write
```

The runtime exchanges client credentials for an access token, caches it with TTL, and injects `Authorization: Bearer <access_token>`. Tokens are refreshed automatically on expiry or HTTP 401.

Apply:
```bash
orlojctl apply -f my-tool-secret.yaml
orlojctl apply -f my-custom-tool.yaml
```

## Step 4: Grant Agent Access

Add the tool to an agent's `tools` list:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  tools:
    - web_search
    - my-custom-tool
  limits:
    max_steps: 6
    timeout: 30s
```

If governance is enabled, you also need a ToolPermission and an AgentRole that grants the required permissions. See the [governance guide](./setup-governance.md) for details.

## Step 5: Choose a Tool Type

The examples above use `type: http` (the default). If your tool uses a different transport or execution model, set `spec.type` accordingly. The tool type and isolation mode are independent -- any type can run under any isolation mode.

### External (standalone service)

For tools that need the full Orloj execution context (task, agent, namespace, attempt):

```yaml
spec:
  type: external
  endpoint: https://your-tool-service.internal/execute
  runtime:
    timeout: 30s
```

Your service receives the complete `ToolExecutionRequest` JSON envelope and must return a `ToolExecutionResponse`. This is the right choice when your tool is a dedicated microservice that makes decisions based on who called it and why.

### gRPC

For tools that expose a gRPC service:

```yaml
spec:
  type: grpc
  endpoint: your-grpc-service:50051
  runtime:
    timeout: 15s
```

Implement the `orloj.tool.v1.ToolService/Execute` unary method. Payloads are the same `ToolExecutionRequest` / `ToolExecutionResponse` envelopes as `external`, marshaled as JSON over gRPC (no protobuf compilation needed).

### Webhook-Callback (async / long-running)

For tools that take seconds-to-minutes to complete:

```yaml
spec:
  type: webhook-callback
  endpoint: https://your-async-tool.internal/submit
  runtime:
    timeout: 120s
```

Execution flow:

1. Orloj POSTs a `ToolExecutionRequest` to your endpoint.
2. Your tool returns `202 Accepted` to acknowledge receipt.
3. Orloj polls `{endpoint}/{request_id}` at intervals until your tool returns a `ToolExecutionResponse` with a terminal status, or the timeout expires.
4. Alternatively, your tool can push the result to Orloj's callback delivery API instead of waiting for a poll.

Use this for batch processing, CI triggers, human approval workflows, or any tool where the response isn't immediate.

## Step 6: Configure Isolation (Optional)

For tools that run untrusted code or interact with sensitive resources, set an explicit isolation mode. This is independent of tool type.

**Container isolation:**
```yaml
spec:
  runtime:
    isolation_mode: container
    timeout: 15s
```

**WASM isolation** (for tools compiled to WebAssembly):
```yaml
spec:
  type: wasm
  runtime:
    isolation_mode: wasm
    timeout: 5s
```

WASM tools communicate over stdin/stdout using the same JSON envelope.

**Sandboxed isolation** (secure-by-default container):
```yaml
spec:
  risk_level: high
  runtime:
    isolation_mode: sandboxed
```

Sandboxed mode runs tools in a locked-down container: read-only filesystem, no capabilities, no privilege escalation, no network, non-root user, and strict memory/CPU/pids limits. This is the default for `high` and `critical` risk tools.

## Step 7: Validate with the Conformance Harness

Orloj provides a tool runtime conformance harness that tests your tool against the contract specification. The harness covers eight test groups:

1. **Contract** -- request/response envelope validation
2. **Timeout** -- tool respects configured timeouts
3. **Retry** -- retryable errors trigger retry; non-retryable errors do not
4. **Auth** -- credentials are passed correctly
5. **Policy** -- governance denials are handled properly
6. **Isolation** -- isolation backends enforce boundaries
7. **Observability** -- trace metadata is propagated
8. **Determinism** -- identical inputs produce consistent outputs

## Next Steps

- [Tool](../concepts/tools/tool.md) -- tool types, isolation, and contract details
- [Tools and Isolation](../concepts/tools/tool.md) -- concept deep-dive
- [Connect an MCP Server](./connect-mcp-server.md) -- for MCP-compatible tool servers instead of custom implementations
