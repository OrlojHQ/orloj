# API Reference

> **Stability: beta** -- This API surface ships with `orloj.dev/v1` and is suitable for production use, but may evolve with migration guidance in future minor releases.

This page summarizes key HTTP endpoints and behavior contracts.

## Resource CRUD

`/v1/<resource>` supports list/create and `/v1/<resource>/{name}` supports get/update/delete for:

- agents
- agent-systems
- model-endpoints
- tools
- secrets
- sealed-secrets
- memories
- mcp-servers
- agent-policies
- agent-roles
- tool-permissions
- tool-approvals
- task-approvals
- tasks
- task-schedules
- task-webhooks
- workers

Namespace defaults to `default` and can be overridden with `?namespace=<ns>`.

On create and update requests, `metadata.namespace` in the request body must match the effective request namespace (from `?namespace=` or the default). A mismatch returns `400 Bad Request`. When creating resources in a non-default namespace, pass `?namespace=<ns>` on the request URL and set the same value in the manifest body (or omit it and let the server apply the query namespace).

Mutation requests (`POST`, `PUT`, `PATCH`) must use a supported `Content-Type` (`application/json`, `application/yaml`, `application/x-yaml`, or `text/yaml`). Other values return `415 Unsupported Media Type`. Requests with no `Content-Type` header are accepted for backward compatibility.

`GET /v1/sealing-key/public` is cluster-scoped and returns the active public key used to create `SealedSecret` manifests. It returns `503` when no active sealing key is available.

## Capabilities

- `GET /v1/capabilities`
  - returns deployment capability flags for feature discovery in UI/CLI integrations
  - extension providers may add capabilities without changing core API shape

## Authentication Endpoints

- `GET /v1/auth/config`
  - returns auth mode and login/setup requirements; when mode is `native`, `setup_token_required` is true if the server was started with `ORLOJ_SETUP_TOKEN` (the UI shows a setup-token field; `POST /v1/auth/setup` must include `setup_token`)
- `POST /v1/auth/setup`
  - one-time native admin bootstrap when auth mode is `native`
- `POST /v1/auth/login`
  - local username/password login; sets session cookie
- `POST /v1/auth/logout`
  - clears local session cookie
- `GET /v1/auth/me`
  - returns current auth state and identity (`method`, `name`, `role`) for UI/CLI bootstrap
- `POST /v1/auth/users`
  - admin-only native-auth endpoint; creates a local user and returns a generated password once
- `GET /v1/auth/users`
  - admin-only native-auth endpoint; lists local users
- `DELETE /v1/auth/users/{username}`
  - admin-only native-auth endpoint; deletes a local user (last-admin delete is blocked)
- `POST /v1/auth/admin/reset-password`
  - admin-authenticated local password reset endpoint for a specific `username`
- `POST /v1/tokens`
  - admin-only endpoint; creates a named API token and returns the token once
- `GET /v1/tokens`
  - admin-only endpoint; lists store-managed tokens (`name`, `role`, `created_at`)
- `DELETE /v1/tokens/{name}`
  - admin-only endpoint; revokes a store-managed token

## Status and Logs

- `GET|PUT /v1/<resource>/{name}/status`
- `GET /v1/agents/{name}/logs`
- `GET /v1/tasks/{name}/logs`

## Approval Decision Endpoints

- `POST /v1/tool-approvals/{name}/approve`
- `POST /v1/tool-approvals/{name}/deny`
- `POST /v1/task-approvals/{name}/approve`
- `POST /v1/task-approvals/{name}/deny`
- `POST /v1/task-approvals/{name}/request-changes`

Decision request bodies may include:

- `decided_by`: reviewer identity
- `comment`: reviewer note
- `reason`: legacy alias for `comment`

`TaskApproval request-changes` requires reviewer feedback via `comment` or the legacy `reason` field. It returns `409 Conflict` when the checkpoint has `allow_request_changes: false` or the approval has already reached `max_review_cycles`. `comment` is also supported on tool approval decisions for consistent reviewer audit trails.

## Watches and Events

- `GET /v1/agents/watch`
- `GET /v1/tasks/watch`
- `GET /v1/task-schedules/watch`
- `GET /v1/task-webhooks/watch`
- `GET /v1/events/watch`

## Webhook Delivery

- `POST /v1/webhook-deliveries/{endpoint_id}`
  - public ingress for `TaskWebhook` delivery
  - returns `202 Accepted` for accepted or duplicate deliveries
  - relies on webhook auth configuration for signature and idempotency validation

### Signature Profiles

- `generic`
  - signature: HMAC-SHA256 over `timestamp + "." + rawBody`
  - headers: `X-Signature: sha256=<hex>`, `X-Timestamp`, `X-Event-Id`
- `github`
  - signature: HMAC-SHA256 over raw body
  - headers: `X-Hub-Signature-256: sha256=<hex>`, `X-GitHub-Delivery`

Both profiles support replay protection through timestamp skew and/or event-id dedupe checks.

## Memory Entries

- `GET /v1/memories/{name}/entries`
  - query parameters:
    - `q` (string): search query. When provided, searches entries by keyword match (or vector similarity if the backend supports it).
    - `prefix` (string): filter entries by key prefix. Ignored when `q` is set.
    - `limit` (int): maximum number of entries to return. Defaults to `100`.
    - `namespace` (string): resource namespace. Defaults to `default`.
  - returns `{"entries": [{"key": "...", "value": "...", "score": 0.95}], "count": N}`
  - returns `404` if the Memory resource does not exist
  - returns an empty list if no persistent backend is registered for the Memory resource

## Task Observability Endpoints

- `GET /v1/tasks/{name}/messages`
  - filters: `phase`, `from_agent`, `to_agent`, `branch_id`, `trace_id`, `limit`
- `GET /v1/tasks/{name}/metrics`
  - includes totals and `per_agent`/`per_edge` rollups

`Task.status.trace[]` may include normalized tool metadata:

- `tool_contract_version`
- `tool_request_id`
- `tool_attempt`
- `error_code`
- `error_reason`
- `retryable`

## Request and Response Examples

### Create a Resource

```
POST /v1/agents
Content-Type: application/json
```

```json
{
  "apiVersion": "orloj.dev/v1",
  "kind": "Agent",
  "metadata": {
    "name": "research-agent",
    "namespace": "default"
  },
  "spec": {
    "model_ref": "openai-default",
    "prompt": "You are a research assistant.",
    "tools": ["web_search"],
    "limits": {
      "max_steps": 6,
      "timeout": "30s"
    }
  }
}
```

Response (`201 Created`):

```json
{
  "apiVersion": "orloj.dev/v1",
  "kind": "Agent",
  "metadata": {
    "name": "research-agent",
    "namespace": "default",
    "resourceVersion": "1"
  },
  "spec": { "...": "..." },
  "status": {
    "phase": "Pending"
  }
}
```

### Get a Resource

```
GET /v1/agents/research-agent?namespace=default
```

Returns the full resource including `metadata`, `spec`, and `status`.

### Update a Resource

```
PUT /v1/agents/research-agent
Content-Type: application/json
If-Match: "1"
```

The request body must include the full resource. The `resourceVersion` (or `If-Match` header) must match the current version. Stale updates return `409 Conflict`.

### Delete a Resource

```
DELETE /v1/agents/research-agent?namespace=default
```

Returns `200 OK` on success.

### List Resources

```
GET /v1/agents?namespace=default
```

Returns an array of all resources of that type in the specified namespace.

#### Pagination

All list endpoints support cursor-based pagination via query parameters:

| Parameter | Description |
|---|---|
| `limit` | Maximum number of items to return (1–1000). |
| `after` | Cursor token from the previous page's `continue` field. Accepts scoped `namespace/name` tokens (preferred) or bare names (legacy; scoped to the request namespace). Returns items lexicographically after this cursor. |
| `namespace` | Filter by namespace. |
| `labelSelector` | Comma-separated `key=value` pairs; label filtering is applied before the page is finalized so each page contains up to `limit` matching items. |

When more results are available, the response includes a `continue` field:

```json
{
  "continue": "production/task-00042",
  "items": [ ... ]
}
```

Pass `continue` as the `after` parameter in the next request to fetch the next page. When `continue` is absent or empty, there are no more results. Bare-name cursors from older clients (e.g. `"task-00042"`) are still accepted and scoped to the request namespace.

Offset-based pagination (`?offset=N`) is supported for backward compatibility on the Tasks endpoint but is deprecated in favor of `?after=`.

### Watch Resources

```
GET /v1/agents/watch
```

Returns a server-sent event stream of resource changes. Events include the resource kind, name, and the change type (created, updated, deleted).

Watch streams are bounded:

- **Max duration:** 30 minutes per connection. The server sends a final `close` event and terminates the stream when the limit is reached.
- **Connection limits:** concurrent watch connections are capped globally and per client IP. Excess connections receive `429 Too Many Requests` or `503 Service Unavailable`.

## Concurrency Semantics

- `PUT` requires `metadata.resourceVersion` or `If-Match`
- stale updates return `409 Conflict`

## A2A Endpoints

AgentSystems opt in to inbound A2A with `spec.a2a.enabled: true`. Enabled systems are available through:

- `GET /.well-known/agent-card.json` — root Agent Card when exactly one AgentSystem is A2A-enabled.
- `GET /v1/agent-systems/{name}/.well-known/agent-card.json` — per-system Agent Card.
- `POST /a2a` — shared JSON-RPC 2.0 endpoint for A2A task operations.
- `POST /v1/agent-systems/{name}/a2a` — JSON-RPC endpoint scoped to one AgentSystem.
- `GET /v1/a2a/agents` — registry listing A2A-enabled systems visible to the bearer token plus remote entries.

See [A2A Interoperability](../concepts/a2a-interoperability.md) for protocol details.

## Related Docs

- [Resource Reference](./resources/)
- [CLI Reference](./cli.md)
- [Tool](../concepts/tools/tool.md)
- [Glossary](./glossary.md)
