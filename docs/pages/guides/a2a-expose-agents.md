# Expose Agents via A2A

This guide walks through enabling A2A protocol support so external A2A clients can discover and interact with your Orloj agents.

## Prerequisites

- Orloj server (`orlojd`) running with `--embedded-worker`
- `orlojctl` available
- At least one agent and model endpoint configured

If you have not set up Orloj yet, follow the [Install](../getting-started/install.md) and [Quickstart](../getting-started/quickstart.md) guides first.

## Step 1: Enable A2A

Enable A2A support and set the public base URL that external clients will use to reach your Orloj instance.

With server flags:

```bash
orlojd --a2a-enabled --a2a-public-base-url https://orloj.example.com
```

Or via environment variables:

```bash
export ORLOJ_A2A_ENABLED=true
export ORLOJ_A2A_PUBLIC_BASE_URL=https://orloj.example.com
```

The `publicBaseURL` is used to construct the `url` field in generated Agent Cards. It must be reachable by external A2A clients.

## Step 2: Verify the Default Agent Card

Once A2A is enabled, the server exposes Agent Cards at well-known URLs. Verify with curl:

```bash
curl -s http://localhost:8080/.well-known/agent-card.json | jq .
```

Expected output:

```json
{
  "name": "my-agent",
  "description": "Research assistant with web search capabilities",
  "url": "https://orloj.example.com/v1/agents/my-agent/a2a",
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

For a specific agent:

```bash
curl -s http://localhost:8080/v1/agents/research-agent/.well-known/agent-card.json | jq .
```

## Step 3: Test Inbound Task Creation

Send an A2A `tasks/send` request to create a task:

```bash
curl -s -X POST http://localhost:8080/a2a \
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

## Step 4: Per-Agent Endpoints

For multi-agent deployments, use per-agent A2A endpoints. Each agent gets its own card and JSON-RPC endpoint:

```bash
# Discovery
curl -s http://localhost:8080/v1/agents/research-agent/.well-known/agent-card.json

# Task submission
curl -s -X POST http://localhost:8080/v1/agents/research-agent/a2a \
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

The per-agent card's `url` field points to `https://orloj.example.com/v1/agents/research-agent/a2a`, so A2A clients that discover the card know where to send requests.

## Step 5: Streaming Subscribe

For long-running tasks, use `tasks/sendSubscribe` to receive streaming updates via SSE:

```bash
curl -s -N -X POST http://localhost:8080/v1/agents/research-agent/a2a \
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
2. The target agent is resolved from the URL path (per-agent) or request params (shared endpoint).
3. An Orloj Task is created with A2A metadata labels.
4. The AgentSystem executes the task using the normal Orloj pipeline.
5. Task status transitions are mapped to A2A states and returned in the response.
6. For `tasks/sendSubscribe`, the task's trace/watch SSE stream is converted to A2A streaming events.

## Agent Card Customization

Agent Cards are auto-generated, but you can influence the output with annotations:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
  annotations:
    orloj.dev/description: "AI research assistant specializing in academic papers"
spec:
  model_ref: openai-default
  prompt: |
    You are a research assistant. Search for and summarize academic papers.
  tools:
    - web_search
    - arxiv_search
```

The `orloj.dev/description` annotation overrides the description in the generated card.

## Next Steps

- [Use Remote A2A Agents](./a2a-remote-agents.md) -- call external A2A agents from your Orloj pipelines
- [A2A Interoperability](../concepts/a2a-interoperability.md) -- concept deep-dive
- [A2A JSON-RPC Reference](../reference/a2a-jsonrpc.md) -- per-method documentation
- [Agent Card Reference](../reference/resources/agent-card.md) -- full card schema
