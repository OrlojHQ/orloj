# Use Remote A2A Agents

This guide walks through configuring Orloj to call external A2A agents as tools. Remote A2A agents appear as regular tools in your agent's toolset -- the A2A protocol details are handled transparently by the runtime.

## Prerequisites

- Orloj server (`orlojd`) running, with any inbound A2A exposure enabled per AgentSystem when needed
- `orlojctl` available
- A remote A2A agent endpoint (any A2A-compliant agent)

## Step 1: Create a type:a2a Tool

Define a tool that points to the remote A2A agent:

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: external-analyst
spec:
  type: a2a
  description: "Remote analyst agent that produces market research reports"
  a2a:
    agent_url: https://analyst.example.com
    protocol_version: "0.2"
    prefer_streaming: true
```

The `agent_url` is the base URL of the remote A2A agent. The runtime fetches `{agent_url}/.well-known/agent-card.json` to discover capabilities and the JSON-RPC endpoint.

Apply:

```bash
orlojctl apply -f external-analyst-tool.yaml
```

## Step 2: Configure Authentication

If the remote agent requires authentication, use the standard `spec.auth` field:

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: external-analyst
spec:
  type: a2a
  description: "Remote analyst agent"
  a2a:
    agent_url: https://analyst.example.com
    prefer_streaming: true
  auth:
    profile: bearer
    secretRef: analyst-api-key
```

Create the secret:

```yaml
apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: analyst-api-key
spec:
  stringData:
    value: sk-remote-agent-token
```

All four auth profiles work with A2A tools: `bearer`, `api_key_header`, `basic`, and `oauth2_client_credentials`.

## Step 3: Attach to an Agent

Reference the A2A tool in your agent's `spec.tools`, just like any other tool:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: coordinator
spec:
  model_ref: openai-default
  prompt: |
    You coordinate research tasks. When the user asks for market analysis,
    delegate to the external-analyst tool.
  tools:
    - external-analyst
    - web_search
  limits:
    max_steps: 10
    timeout: 120s
```

Apply and test:

```bash
orlojctl apply -f coordinator-agent.yaml
orlojctl task submit --agent coordinator --input "Analyze the AI chip market"
```

## Step 4: Test the Invocation

When the coordinator agent decides to call `external-analyst`, the runtime:

1. Fetches the remote Agent Card from `https://analyst.example.com/.well-known/agent-card.json`.
2. Sends a JSON-RPC `tasks/send` (or `tasks/sendSubscribe` if `prefer_streaming` is true and the remote supports it) to the card's `url`.
3. Maps the A2A response back to a tool result.

Check the task messages to see the A2A interaction:

```bash
orlojctl get tasks <task-name> -o json | jq '.status'
```

## Streaming vs Polling

The `prefer_streaming` field controls how Orloj communicates with the remote agent:

| `prefer_streaming` | Remote supports streaming | Behavior |
|--------------------|--------------------------|----------|
| `true` (default) | Yes | `tasks/sendSubscribe` -- real-time SSE updates |
| `true` | No | Falls back to `tasks/send` |
| `false` | Any | Always uses `tasks/send` |

Streaming is recommended for long-running remote agents. The A2A tool runtime converts streaming status updates into progress events visible in the task trace.

## Error Handling

A2A tool invocations follow the standard tool error taxonomy:

| Scenario | Error code | Retryable |
|----------|-----------|-----------|
| Remote agent unreachable | `connection_error` | Yes |
| Remote returns JSON-RPC error | `a2a_rpc_error` | Depends on error code |
| Remote task fails | `a2a_task_failed` | No |
| Remote task times out | `timeout` | Yes |
| Auth failure (401/403) | `auth_invalid` / `auth_forbidden` | No |
| Card fetch fails | `a2a_discovery_error` | Yes |

The tool's `runtime.retry` policy applies to retryable errors:

```yaml
spec:
  type: a2a
  a2a:
    agent_url: https://analyst.example.com
  runtime:
    timeout: 60s
    retry:
      max_attempts: 3
      backoff: 2s
      max_backoff: 30s
      jitter: full
```

## Registry: List All A2A Agents

View locally exposed agents and configured remote agents:

```bash
curl -s http://localhost:8080/v1/a2a/agents \
  -H "Authorization: Bearer $ORLOJ_TOKEN" | jq .
```

Response:

```json
{
  "localAgents": [
    {
      "name": "research-agent",
      "url": "https://orloj.example.com/v1/agent-systems/research-system/a2a",
      "capabilities": { "streaming": true }
    }
  ],
  "remoteAgents": [
    {
      "name": "external-analyst",
      "url": "https://analyst.example.com",
      "cacheStatus": "ok",
      "lastRefreshed": "2025-06-01T10:30:00Z",
      "card": { "..." }
    }
  ]
}
```

## Governance

A2A tools participate in the full governance pipeline:

- **AgentRole**: bind roles to agents that restrict which A2A tools they can call.
- **ToolPermission**: define per-operation-class rules (e.g., require approval for `write`-class A2A calls).
- **AgentPolicy**: enforce cost, rate, or content policies on A2A tool invocations.

```yaml
apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: allow-external-analyst
spec:
  toolRef: external-analyst
  operation_rules:
    - operations: ["read"]
      verdict: allow
    - operations: ["write"]
      verdict: approval_required
```

## Next Steps

- [Expose Agents via A2A](./a2a-expose-agents.md) -- make your Orloj agents discoverable
- [A2A Interoperability](../concepts/a2a-interoperability.md) -- concept deep-dive
- [A2A JSON-RPC Reference](../reference/a2a-jsonrpc.md) -- protocol details
- [Build a Custom Tool](./build-custom-tool.md) -- for non-A2A tool integrations
