# API Reference

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

## Concurrency Semantics

- `PUT` requires `metadata.resourceVersion` or `If-Match`
- stale updates return `409 Conflict`

## Related Docs

- [CRD Reference](./crds.md)
- [Tool Contract v1](./tool-contract-v1.md)
