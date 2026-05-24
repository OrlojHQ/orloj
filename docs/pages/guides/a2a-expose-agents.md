# Expose AgentSystems via A2A

This guide walks through exposing selected Orloj AgentSystems to external A2A clients.

## Prerequisites

- Orloj server (`orlojd`) running with `--embedded-worker`
- `orlojctl` available
- At least one agent and model endpoint configured

If you have not set up Orloj yet, follow the [Install](../getting-started/install.md) and [Quickstart](../getting-started/quickstart.md) guides first.

## Step 1: Expose an AgentSystem

Add `spec.a2a.enabled: true` to each AgentSystem that should be reachable through A2A. Systems without this block remain internal.

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: research-system
spec:
  agents:
    - research-agent
  a2a:
    enabled: true
```

Set the public base URL so generated Agent Cards point at the externally reachable host:

```bash
orlojd --a2a-public-base-url https://orloj.example.com
```

## Step 2: Verify the Default Agent Card

If exactly one AgentSystem is A2A-enabled, the root well-known URL returns its card:

```bash
curl -s http://localhost:8080/.well-known/agent-card.json | jq .
```

Expected output:

```json
{
  "name": "research-system",
  "description": "Research assistant with web search capabilities",
  "url": "https://orloj.example.com/v1/agent-systems/research-system/a2a",
  "protocolVersion": "0.2",
  "capabilities": {
    "streaming": true,
    "pushNotifications": false,
    "stateTransitionHistory": true
  },
  "skills": [
    {
      "id": "web-search",
      "name": "web_search",
      "description": "Search the web for information",
      "tags": ["search", "web"]
    }
  ],
  "authentication": {
    "schemes": ["bearer"]
  }
}
```

For a specific AgentSystem:

```bash
curl -s http://localhost:8080/v1/agent-systems/research-system/.well-known/agent-card.json | jq .
```

## Step 3: Test Inbound Task Creation

Send an A2A `tasks/send` request to create a task:

```bash
curl -s -X POST http://localhost:8080/v1/agent-systems/research-system/a2a \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ORLOJ_TOKEN" \
  -d '{
    "jsonrpc": "2.0",
    "id": "req-1",
    "method": "tasks/send",
    "params": {
      "id": "task-001",
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Summarize recent AI news"}]
      }
    }
  }' | jq .
```

The response contains an A2A task with a status:

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "result": {
    "id": "a2a-task-abc123",
    "status": {
      "state": "completed",
      "message": {
        "role": "agent",
        "parts": [{"type": "text", "text": "Here is a summary of recent AI news..."}]
      }
    },
    "artifacts": [
      {
        "name": "summary",
        "parts": [{"type": "text", "text": "..."}]
      }
    ]
  }
}
```

## Step 4: Multiple Systems

For deployments with multiple exposed systems, use per-system cards and endpoints:

```bash
# Discovery
curl -s http://localhost:8080/v1/agent-systems/research-system/.well-known/agent-card.json

# Task submission
curl -s -X POST http://localhost:8080/v1/agent-systems/research-system/a2a \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ORLOJ_TOKEN" \
  -d '{
    "jsonrpc": "2.0",
    "id": "req-2",
    "method": "tasks/send",
    "params": {
      "id": "task-002",
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Find papers on transformer architecture"}]
      }
    }
  }'
```

The per-system card's `url` field points to `https://orloj.example.com/v1/agent-systems/research-system/a2a`, so A2A clients that discover the card know where to send requests.

## Step 5: Streaming Subscribe

For long-running tasks, use `tasks/sendSubscribe` to receive streaming updates via SSE:

```bash
curl -s -N -X POST http://localhost:8080/v1/agent-systems/research-system/a2a \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ORLOJ_TOKEN" \
  -d '{
    "jsonrpc": "2.0",
    "id": "req-3",
    "method": "tasks/sendSubscribe",
    "params": {
      "id": "task-003",
      "message": {
        "role": "user",
        "parts": [{"type": "text", "text": "Write a detailed report on quantum computing"}]
      }
    }
  }'
```

The server responds with a stream of SSE events containing status updates and artifact chunks as the agent works.

## How It Works

When an A2A request arrives:

1. The JSON-RPC method is parsed and validated.
2. The target AgentSystem is resolved from the URL path or request params (shared endpoint).
3. An Orloj Task is created with A2A metadata labels.
4. The AgentSystem executes the task using the normal Orloj pipeline.
5. Task status transitions are mapped to A2A states and returned in the response.
6. For `tasks/sendSubscribe`, the task's trace/watch SSE stream is converted to A2A streaming events.

## Per-System Auth Policy

By default, A2A invoke requires a bearer token when instance-wide auth is configured. To allow unauthenticated callers to invoke a specific system, set `spec.a2a.auth: public`:

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: public-assistant
spec:
  agents:
    - assistant-agent
  a2a:
    enabled: true
    auth: public
```

This is useful when you want admin tokens for the control plane (`/v1/agents`, `/v1/tools`, etc.) but need a public-facing A2A endpoint for external clients. Invalid tokens are still rejected — only missing tokens are permitted on public systems.

Public systems' Agent Cards omit `authentication.schemes`, so A2A clients that discover the card know not to send tokens. The A2A registry (`GET /v1/a2a/agents`) shows public systems to unauthenticated callers.

To require auth (the default), omit `auth` or set `spec.a2a.auth: bearer`.

## Agent Card Customization

Agent Cards are auto-generated from the AgentSystem and its agents, but you can influence the output with annotations:

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: research-system
  annotations:
    orloj.dev/description: "AI research assistant specializing in academic papers"
spec:
  agents:
    - research-agent
  a2a:
    enabled: true
```

The `orloj.dev/description` annotation overrides the description in the generated card.

## Next Steps

- [Use Remote A2A Agents](./a2a-remote-agents.md) -- call external A2A agents from your Orloj pipelines
- [A2A Interoperability](../concepts/a2a-interoperability.md) -- concept deep-dive
- [A2A JSON-RPC Reference](../reference/a2a-jsonrpc.md) -- per-method documentation
- [Agent Card Reference](../reference/resources/agent-card.md) -- full card schema
