# ContextAdapter

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

A ContextAdapter configures a pre-agent sanitization step that transforms raw task input before any agent sees it. It references a [Tool](./tool.md) that receives the input as JSON and returns sanitized JSON. The adapter enforces the handoff contract and error policy but leaves all sanitization logic to the tool.

This is useful for workflows that ingest sensitive data (PII, financial records, medical information) where the AI agent needs to reason about the data but should never have access to the raw values.

## spec

- `tool_ref` (string, required): name of a Tool resource. The tool receives the task's `spec.input` as a `map[string]string` JSON object and must return a `map[string]string` JSON object with sanitized values.
- `on_error` (string): behavior when the tool call fails or returns invalid output.
  - `reject` (default): abort the task with an error. No raw data reaches any agent.
  - `passthrough`: log a warning and pass the original unmodified input to the agent. Useful for development or non-critical paths.

## How it works

The ContextAdapter is declared on an [AgentSystem](./agent-system.md) via `spec.context_adapter`, not on individual tasks. This ensures every task that runs against the system is automatically protected.

```yaml
apiVersion: orloj.dev/v1
kind: ContextAdapter
metadata:
  name: tx-sanitizer
spec:
  tool_ref: tx-sanitize-tool
  on_error: reject
---
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: fraud-detection
spec:
  context_adapter: tx-sanitizer
  agents:
    - tx-analyst
```

At runtime, the adapter fires after a task is created but before the first agent executes:

1. Raw task input (`task.spec.input`) is JSON-encoded and sent to the tool.
2. The tool performs sanitization (masking, tokenization, scrubbing, etc.) and returns a JSON object.
3. The sanitized map replaces the original input for agent execution.
4. If the tool fails and `on_error` is `reject`, the task aborts. If `passthrough`, the raw input is used with a logged warning.

The adapter runs once per task, before the first agent only. On task resume (e.g. after a human review checkpoint), the adapter does not re-run.

## Tool contract

The referenced tool receives a JSON object:

```json
{
  "account_number": "4111-1111-1111-1111",
  "ssn": "123-45-6789",
  "amount": "9800.00",
  "memo": "wire transfer"
}
```

It must return a JSON object with the same or modified keys:

```json
{
  "account_number": "ACCT_7x3k9m",
  "ssn": "XXX-XX-XXXX",
  "amount": "9800.00",
  "memo": "wire transfer"
}
```

The tool decides what to sanitize and how. WASM tools (via Wazero) are recommended for handling PII because they run fully sandboxed with no filesystem or network access, but any tool runtime works (container, CLI, HTTP, gRPC).

## Defaults and Validation

- `spec.tool_ref` is required. Normalization trims whitespace.
- `spec.on_error` defaults to `reject` when omitted or empty.
- `spec.on_error` must be `reject` or `passthrough`.
- `status.phase` defaults to `Pending`.

## status

- `phase`: `Pending` or `Ready`.
- `message`: description of the current state.

See also: [AgentSystem](./agent-system.md), [Tool](./tool.md), [Build a WASM Tool](../../guides/build-wasm-tool.md)
