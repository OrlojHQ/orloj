# A2A Interoperability

Orloj implements the [Agent-to-Agent (A2A) protocol](https://github.com/a2aproject/A2A) to enable cross-platform agent communication. Any A2A-compliant client can discover Orloj agents, submit tasks, and stream results -- and Orloj agents can call external A2A agents as tools.

## Why A2A?

Agent runtimes are converging on a shared interoperability layer. A2A provides a standard for agent discovery (Agent Cards), task lifecycle (JSON-RPC), and streaming updates (SSE). Supporting A2A means Orloj agents can participate in multi-vendor agent ecosystems without custom integration code.

## Architecture

A2A support in Orloj has two directions:

**Inbound** -- external A2A clients call Orloj agents:

```
A2A Client
  → GET /.well-known/agent-card.json  (discovery)
  → POST /a2a  or  /v1/agent-systems/{name}/a2a  (JSON-RPC)
    → Orloj creates a Task, runs the AgentSystem
    → Returns A2A status updates and artifacts
```

**Outbound** -- Orloj agents call external A2A agents as tools:

```
Orloj Agent step
  → GovernedToolRuntime (policy, auth, retry)
    → A2AToolRuntime
      → GET {remote}/.well-known/agent-card.json  (discover capabilities)
      → POST {remote}/a2a  (tasks/send or tasks/sendSubscribe)
      → Map remote A2A result → tool result
```

Both directions reuse existing Orloj infrastructure: Tasks, auth, governance, webhooks, and SSE streaming.

## Agent Cards

Every Orloj AgentSystem exposed via A2A publishes an **Agent Card** -- a JSON document describing the system's name, capabilities, skills, authentication requirements, and endpoint URL.

Cards are generated automatically from the agent's metadata, tools, and runtime configuration:

- **Name** and **description** come from the AgentSystem `metadata.name` and `metadata.annotations["orloj.dev/description"]`.
- **Skills** are derived from the agent's attached tools, including each tool's `description` and `input_schema`.
- **Capabilities** reflect runtime support: streaming (from task trace SSE), push notifications (from TaskWebhook support), and state transition history.
- **Authentication** reflects the server's configured auth mode.

Cards are served at:
- `GET /.well-known/agent-card.json` -- only when exactly one AgentSystem is A2A-enabled
- `GET /v1/agent-systems/{name}/.well-known/agent-card.json` -- specific AgentSystem

## State Mapping

A2A defines its own task lifecycle states. Orloj maps them bidirectionally:

| A2A State | Orloj Phase | Direction |
|-----------|-------------|-----------|
| `submitted` | `Pending` | inbound |
| `working` | `Running` | both |
| `input-required` | `WaitingApproval` (with `a2a-input-required` label) | inbound |
| `completed` | `Succeeded` | both |
| `failed` | `Failed` | both |
| `canceled` | `Failed` (with `orloj.dev/a2a-cancelled` label) | inbound |
| `rejected` | `Failed` (with rejection reason) | inbound |

Orloj task output is converted to A2A artifacts, and trace/watch events are converted to A2A streaming status updates for `tasks/sendSubscribe` calls.

## Inbound Routing

Two routing modes are supported for inbound A2A requests:

1. **Per-system endpoints** (recommended): `POST /v1/agent-systems/{name}/a2a` -- the AgentSystem name is in the URL path. Each system's card `url` field points to its per-system endpoint.
2. **Shared endpoint**: `POST /a2a` -- the target is resolved from request params. When a single AgentSystem is A2A-enabled, this endpoint defaults to it.

## Outbound: A2A Tools

External A2A agents are consumed as `type: a2a` tools. The tool spec includes the remote agent URL, optional protocol version, and streaming preference. At invocation time, the A2A tool runtime fetches the remote card, sends a JSON-RPC request, and maps the response back to Orloj's tool result format.

Existing `spec.auth` profiles (bearer, API key, basic, OAuth2) work for authenticating with remote A2A agents.

## Security Model

A2A endpoints participate in Orloj's existing security model:

- **Agent Card discovery** endpoints are public (no auth required) -- cards contain only metadata, not secrets.
- **JSON-RPC endpoints** enforce per-system auth via `spec.a2a.auth` (`"bearer"` by default, or `"public"` for unauthenticated access). All four methods (`tasks/send`, `tasks/get`, `tasks/cancel`, `tasks/sendSubscribe`) require a valid bearer token on `auth: bearer` systems. Native-auth browser sessions are not accepted for A2A invocation.
- **Scoped `a2a` tokens** can invoke only the AgentSystems listed in their `a2a_agent_systems` scope and cannot read or mutate control-plane resources.
- **Outbound calls** to remote agents use tool-level auth (`spec.auth`) and are subject to governance (AgentRole, ToolPermission, AgentPolicy).
- **Private endpoint protection**: by default, outbound A2A calls to private/internal endpoints are blocked (`a2a.allowPrivateEndpoints` defaults to `false`).

## Configuration

A2A is enabled per AgentSystem with `spec.a2a.enabled: true`. Server configuration controls advertised URLs and outbound registry behavior:

| Setting | Description |
|---------|-------------|
| `a2a.publicBaseURL` | Public base URL for Agent Card endpoint URLs |
| `a2a.protocolVersion` | A2A protocol version to advertise |
| `a2a.remoteAgents[]` | Pre-configured remote A2A agents for the registry |
| `a2a.cardCacheTTL` | Cache TTL for fetched remote Agent Cards |

## Related

- [Guide: Expose Agents via A2A](../guides/a2a-expose-agents.md)
- [Guide: Use Remote A2A Agents](../guides/a2a-remote-agents.md)
- [Reference: A2A JSON-RPC](../reference/a2a-jsonrpc.md)
- [Reference: Agent Card](../reference/resources/agent-card.md)
- [Tool](./tools/tool.md) -- tool types including `a2a`
