# A2A JSON-RPC

> **Stability: beta** -- A2A protocol support ships with `orloj.dev/v1`. The JSON-RPC interface follows the [A2A specification](https://github.com/a2aproject/A2A) and may evolve as the spec matures.

Orloj exposes A2A functionality via JSON-RPC 2.0 over HTTP. All requests are `POST` with `Content-Type: application/json`.

## Endpoints

| Endpoint | Auth | Description |
|----------|------|-------------|
| `POST /a2a` | Bearer token when auth is enabled | Shared JSON-RPC endpoint. Resolves target from params or defaults to the single A2A-enabled AgentSystem. |
| `POST /v1/agent-systems/{name}/a2a` | Bearer token when auth is enabled | Per-system JSON-RPC endpoint. AgentSystem name in the path determines routing. |

Legacy `/v1/agents/{name}/a2a` paths are accepted as aliases for AgentSystem names.

## Request Format

All methods use the standard JSON-RPC 2.0 envelope:

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "method": "tasks/send",
  "params": { ... }
}
```

The `id` field can be a string or integer. If omitted, the request is treated as a notification (no response body).

## Methods

### tasks/send

Create a task and wait for completion. Returns the final task state.

**Params:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Client-provided task ID. The server rejects requests without an `id`. |
| `message` | [A2AMessage](#a2amessage) | Yes | Input message for the agent. |
| `metadata` | map[string]string | No | Arbitrary key-value pairs attached to the task. |

**Result:** [A2ATask](#a2atask)

**Example:**

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "method": "tasks/send",
    "params": {
      "id": "task-001",
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Summarize this document"}]
      },
      "metadata": {
        "source": "external-workflow"
      }
    }
}
```

**Response:**

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "result": {
    "id": "a2a-task-xyz",
    "status": {
      "state": "completed",
      "message": {
        "role": "agent",
        "parts": [{"type": "text", "text": "Here is the summary..."}]
      }
    },
    "artifacts": [
      {
        "name": "summary",
        "parts": [{"type": "text", "text": "..."}],
        "index": 0
      }
    ]
  }
}
```

### tasks/get

Retrieve the current state of an existing task.

**Params:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Task ID to retrieve. |

**Result:** [A2ATask](#a2atask)

**Example:**

```json
{
  "jsonrpc": "2.0",
  "id": "req-2",
  "method": "tasks/get",
  "params": {
    "id": "a2a-task-xyz"
  }
}
```

### tasks/cancel

Request cancellation of a running task.

**Params:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Task ID to cancel. |
| `reason` | string | No | Cancellation reason. |

**Result:** [A2ATask](#a2atask) with status state `canceled`.

**Example:**

```json
{
  "jsonrpc": "2.0",
  "id": "req-3",
  "method": "tasks/cancel",
  "params": {
    "id": "a2a-task-xyz",
    "reason": "No longer needed"
  }
}
```

### tasks/sendSubscribe

Create a task and stream status updates via SSE. The initial HTTP response transitions to an SSE stream.

**Params:** Same as [tasks/send](#taskssend).

**Response:** Server-Sent Events stream with the following event types:

| Event | Data | Description |
|-------|------|-------------|
| `status` | TaskResult JSON | Task state transition (full task result including `id`, `status`, `artifacts`, etc.) |
| `: heartbeat` | _(comment)_ | Keep-alive comment sent every 15 s; not a named event |

The stream ends when the task reaches a terminal state (`completed`, `failed`, `canceled`, `rejected`).

**Example SSE stream:**

```
event: status
data: {"id": "a2a-task-xyz", "status": {"state": "working"}}

: heartbeat

event: status
data: {"id": "a2a-task-xyz", "status": {"state": "completed", "message": {"role": "agent", "parts": [{"type": "text", "text": "Done."}]}}, "artifacts": []}
```

## Error Codes

JSON-RPC errors use standard codes plus A2A-specific extensions:

| Code | Name | Description |
|------|------|-------------|
| -32700 | Parse error | Invalid JSON |
| -32600 | Invalid request | Missing required fields |
| -32601 | Method not found | Unknown method name |
| -32602 | Invalid params | Invalid method parameters |
| -32603 | Internal error | Server-side failure |
| -32001 | Task not found | Referenced task ID does not exist |
| -32002 | Task cancelled | Task has been cancelled |
| -32003 | Agent not found | Target agent does not exist or A2A is not enabled for it |

**Error response example:**

```json
{
  "jsonrpc": "2.0",
  "id": "req-4",
  "error": {
    "code": -32001,
    "message": "task not found",
    "data": {
      "task_id": "a2a-task-unknown"
    }
  }
}
```

## Data Types

### A2ATask

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique task identifier |
| `status` | A2AStatus | Current task status |
| `artifacts` | []A2AArtifact | Output artifacts |
| `history` | []A2AMessage | Message history (when `stateTransitionHistory` is enabled) |
| `metadata` | map[string]string | Task metadata |

### A2AStatus

| Field | Type | Description |
|-------|------|-------------|
| `state` | string | One of: `submitted`, `working`, `input-required`, `completed`, `failed`, `canceled`, `rejected` |
| `message` | A2AMessage | Optional status message from the agent |

### A2AMessage

| Field | Type | Description |
|-------|------|-------------|
| `role` | string | `user` or `agent` |
| `parts` | []A2APart | Message content parts |

### A2APart

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Part type (e.g., `text`, `data`) |
| `text` | string | Text content (when `type=text`) |
| `data` | any | Structured data (when `type=data`) |
| `metadata` | object | Additional part metadata |

### A2AArtifact

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Artifact name |
| `description` | string | Artifact description |
| `parts` | []A2APart | Artifact content |
| `index` | integer | Artifact index for ordering |

## Related

- [A2A Interoperability](../concepts/a2a-interoperability.md) -- architecture and concepts
- [Expose Agents via A2A](../guides/a2a-expose-agents.md) -- enable inbound A2A
- [Use Remote A2A Agents](../guides/a2a-remote-agents.md) -- outbound A2A tools
- [Agent Card](./resources/agent-card.md) -- card schema reference
