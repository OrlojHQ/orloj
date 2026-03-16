# Build a Custom Tool

This guide is for developers who need to extend agent capabilities by implementing a custom tool. You will implement the Tool Contract v1, register the tool as a resource, configure isolation and retry, and validate it with the conformance harness.

## Prerequisites

- Orloj server (`orlojd`) and at least one worker running
- `orlojctl` available
- Familiarity with the [Tools and Isolation](../concepts/tools-and-isolation.md) concepts

## What You Will Build

A custom HTTP tool that agents can invoke during execution, registered with Orloj and configured with appropriate runtime controls.

## Step 1: Implement the Tool Contract

Every tool must accept a JSON request envelope and return a JSON response envelope. This is the [Tool Contract v1](../reference/tool-contract-v1.md).

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

**`runtime.timeout`** -- How long the runtime waits for your tool to respond before treating the invocation as failed. Choose based on your tool's expected latency.

**`runtime.retry`** -- Configure retry behavior for transient failures. The `jitter: full` setting randomizes backoff intervals to prevent thundering herd effects when multiple agents hit the same tool.

## Step 3: Create a Secret (If Needed)

If your tool requires authentication:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: my-tool-api-key
spec:
  stringData:
    value: your-api-key-here
```

```bash
orlojctl apply -f my-tool-secret.yaml
```

## Step 4: Grant Agent Access

Add the tool to an agent's `tools` list:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model: gpt-4o
  tools:
    - web_search
    - my-custom-tool
  limits:
    max_steps: 6
    timeout: 30s
```

If governance is enabled, you also need a ToolPermission and an AgentRole that grants the required permissions. See the [governance guide](./setup-governance.md) for details.

## Step 5: Configure Isolation (Optional)

For tools that run untrusted code or interact with sensitive resources, set an explicit isolation mode:

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

WASM tools communicate over stdin/stdout using the same JSON envelope. See the [WASM Tool Module Contract v1](../reference/wasm-tool-module-contract-v1.md) for the host-guest communication specification.

## Step 6: Validate with the Conformance Harness

Orloj provides a tool runtime conformance harness that tests your tool against the contract specification. The harness covers eight test groups:

1. **Contract** -- request/response envelope validation
2. **Timeout** -- tool respects configured timeouts
3. **Retry** -- retryable errors trigger retry; non-retryable errors do not
4. **Auth** -- credentials are passed correctly
5. **Policy** -- governance denials are handled properly
6. **Isolation** -- isolation backends enforce boundaries
7. **Observability** -- trace metadata is propagated
8. **Determinism** -- identical inputs produce consistent outputs

See [Tool Runtime Conformance](../operations/tool-runtime-conformance.md) for detailed instructions on running the harness.

## Next Steps

- [Tool Contract v1](../reference/tool-contract-v1.md) -- full contract specification
- [WASM Tool Module Contract v1](../reference/wasm-tool-module-contract-v1.md) -- WASM-specific contract
- [Tools and Isolation](../concepts/tools-and-isolation.md) -- concept deep-dive
- [Tool Runtime Conformance](../operations/tool-runtime-conformance.md) -- running the test harness
