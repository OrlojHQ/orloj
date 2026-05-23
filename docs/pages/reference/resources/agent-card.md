# Agent Card

> **Stability: beta** -- Agent Cards are generated resources that follow the [A2A specification](https://github.com/a2aproject/A2A). The schema may evolve as the A2A spec matures.

An Agent Card is a JSON document that describes an Orloj agent's A2A identity, capabilities, skills, and endpoint. Cards are auto-generated from agent metadata and served at well-known discovery URLs.

Agent Cards are not stored resources -- they are computed on request from the agent's current state.

## Discovery URLs

| URL | Description |
|-----|-------------|
| `GET /.well-known/agent-card.json` | Default agent card |
| `GET /v1/agents/{name}/.well-known/agent-card.json` | Card for a specific agent |

Both endpoints are public (no authentication required). Cards contain only metadata, not secrets.

## Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Human-readable agent name. Derived from `metadata.name`. |
| `description` | string | No | Agent description. From `metadata.annotations["orloj.dev/description"]` or a prompt summary. |
| `url` | string (URI) | Yes | A2A JSON-RPC endpoint URL. Constructed from `a2a.publicBaseURL` + agent path. |
| `version` | string | No | Agent version. From `metadata.labels["orloj.dev/version"]` if present. |
| `protocolVersion` | string | No | A2A protocol version. From `a2a.protocolVersion` config. |
| `capabilities` | object | No | See [Capabilities](#capabilities). |
| `skills` | []object | No | See [Skills](#skills). |
| `authentication` | object | No | See [Authentication](#authentication). |
| `provider` | object | No | See [Provider](#provider). |

### Capabilities

| Field | Type | Description |
|-------|------|-------------|
| `streaming` | boolean | Whether the agent supports `tasks/sendSubscribe`. Derived from task trace SSE support. |
| `pushNotifications` | boolean | Whether push notifications are available. Derived from TaskWebhook support. |
| `stateTransitionHistory` | boolean | Whether task history is available. |

### Skills

Each skill entry represents a tool available to the agent:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Skill identifier. From `Tool.metadata.name`. |
| `name` | string | Yes | Display name. From `Tool.metadata.name`. |
| `description` | string | No | From `Tool.spec.description`. Omitted if the tool has no description. |
| `inputSchema` | object | No | From `Tool.spec.input_schema`. Omitted (not empty) if the tool has no schema. |
| `tags` | []string | No | Derived from tool capabilities and operation classes. |

### Authentication

| Field | Type | Description |
|-------|------|-------------|
| `schemes` | []string | Auth schemes accepted by the JSON-RPC endpoint. Reflects server auth configuration (e.g., `["bearer"]`). |

### Provider

| Field | Type | Description |
|-------|------|-------------|
| `organization` | string | Organization name. From server configuration. |
| `url` | string (URI) | Organization URL. |

## Field Mapping

How Orloj resources map to Agent Card fields:

| Card Field | Source |
|------------|--------|
| `name` | `Agent.metadata.name` |
| `description` | `Agent.metadata.annotations["orloj.dev/description"]`, or prompt excerpt |
| `url` | `a2a.publicBaseURL` + `/v1/agents/{name}/a2a` |
| `protocolVersion` | `a2a.protocolVersion` server config |
| `capabilities.streaming` | Server SSE support enabled |
| `capabilities.pushNotifications` | TaskWebhook controller active |
| `skills[].id` | `Tool.metadata.name` (for each tool in `Agent.spec.tools`) |
| `skills[].description` | `Tool.spec.description` |
| `skills[].inputSchema` | `Tool.spec.input_schema` |
| `authentication.schemes` | Server auth mode |

## Example Card

For an agent defined as:

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
  annotations:
    orloj.dev/description: "AI research assistant for academic papers"
spec:
  model_ref: openai-default
  tools:
    - web_search
    - arxiv_search
```

The generated card (at `GET /v1/agents/research-agent/.well-known/agent-card.json`):

```json
{
  "name": "research-agent",
  "description": "AI research assistant for academic papers",
  "url": "https://orloj.example.com/v1/agents/research-agent/a2a",
  "protocolVersion": "0.2",
  "capabilities": {
    "streaming": true,
    "pushNotifications": false,
    "stateTransitionHistory": true
  },
  "skills": [
    {
      "id": "web_search",
      "name": "web_search",
      "description": "Search the web for information",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" }
        },
        "required": ["query"]
      },
      "tags": ["search"]
    },
    {
      "id": "arxiv_search",
      "name": "arxiv_search",
      "description": "Search arXiv for academic papers",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" },
          "max_results": { "type": "integer" }
        },
        "required": ["query"]
      },
      "tags": ["search", "academic"]
    }
  ],
  "authentication": {
    "schemes": ["bearer"]
  },
  "provider": {
    "organization": "Orloj"
  }
}
```

## Related

- [A2A Interoperability](../../concepts/a2a-interoperability.md) -- architecture and concepts
- [A2A JSON-RPC](../a2a-jsonrpc.md) -- protocol reference
- [Agent](./agent.md) -- agent resource schema
