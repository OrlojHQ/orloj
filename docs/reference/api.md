# API Reference (Key Endpoints)

## Resource CRUD

`/v1/<resource>` list/create and `/v1/<resource>/{name}` get/update/delete for:

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

Namespace is scoped by `?namespace=<ns>` (default `default`).

## Capabilities

- `GET /v1/capabilities`
  - Returns deployment capability flags used for feature discovery in UI/CLI integrations.
  - OSS default includes baseline core/runtime/governance/self-host capability ids.
  - Extension providers may add additional capabilities without changing core API shape.

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
  - Public ingress endpoint for `TaskWebhook` delivery.
  - Bearer auth is not required on this route.
  - HMAC signature verification and idempotency checks are required by webhook configuration.
  - Returns `202 Accepted` for accepted or duplicate deliveries.

### Signature Profiles

- `generic`
  - Signature: HMAC-SHA256 over `timestamp + "." + rawBody`
  - Headers (defaults): `X-Signature: sha256=<hex>`, `X-Timestamp`, `X-Event-Id`
  - Replay guard: `X-Timestamp` skew check + event-id dedupe window
- `github`
  - Signature: HMAC-SHA256 over raw body
  - Headers (defaults): `X-Hub-Signature-256: sha256=<hex>`, `X-GitHub-Delivery`
  - Replay guard: event-id dedupe window

## Task Observability

- `GET /v1/tasks/{name}/messages`
  - filters: `phase`, `from_agent`, `to_agent`, `branch_id`, `trace_id`, `limit`
- `GET /v1/tasks/{name}/metrics`
  - same filter model
  - includes totals and `per_agent`/`per_edge` rollups (`retry_count`, `deadletters`, lifecycle counts, latency stats)
- `Task.status.trace[]`
  - runtime tool failures include normalized metadata when available:
  - `tool_contract_version`
  - `tool_request_id`
  - `tool_attempt`
  - `error_code`
  - `error_reason`
  - `retryable`

## Concurrency Semantics

- `PUT` requires `metadata.resourceVersion` or `If-Match`
- stale updates return `409 Conflict`
