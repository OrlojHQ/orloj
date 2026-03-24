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
- memories
- agent-policies
- agent-roles
- tool-permissions
- tasks
- task-schedules
- task-webhooks
- workers

Namespace defaults to `default` and can be overridden with `?namespace=<ns>`.

## Capabilities

- `GET /v1/capabilities`
  - returns deployment capability flags for feature discovery in UI/CLI integrations
  - extension providers may add capabilities without changing core API shape

## Authentication Endpoints

- `GET /v1/auth/config`
  - returns auth mode and login/setup requirements
- `POST /v1/auth/setup`
  - one-time native admin bootstrap when auth mode is `native`
- `POST /v1/auth/login`
  - local username/password login; sets session cookie
- `POST /v1/auth/logout`
  - clears local session cookie
- `GET /v1/auth/me`
  - returns current auth state for UI bootstrap
- `POST /v1/auth/admin/reset-password`
  - admin-authenticated local password rotation endpoint

## Status and Logs

- `GET|PUT /v1/<resource>/{name}/status`
- `GET /v1/agents/{name}/logs`
- `GET /v1/tasks/{name}/logs`

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
| `after` | Cursor token — the `metadata.name` of the last item from the previous page. Returns items with names lexicographically after this value. |
| `namespace` | Filter by namespace. |

When more results are available, the response includes a `continue` field:

```json
{
  "continue": "task-00042",
  "items": [ ... ]
}
```

Pass `continue` as the `after` parameter in the next request to fetch the next page. When `continue` is absent or empty, there are no more results.

Offset-based pagination (`?offset=N`) is supported for backward compatibility on the Tasks endpoint but is deprecated in favor of `?after=`.

### Watch Resources

```
GET /v1/agents/watch
```

Returns a server-sent event stream of resource changes. Events include the resource kind, name, and the change type (created, updated, deleted).

## Concurrency Semantics

- `PUT` requires `metadata.resourceVersion` or `If-Match`
- stale updates return `409 Conflict`

## Related Docs

- [Resource Reference](./resources/)
- [CLI Reference](./cli.md)
- [Tool](../concepts/tools/tool.md)
- [Glossary](./glossary.md)
