# Webhook Triggers

Use `TaskWebhook` to trigger task runs from signed external HTTP events.

## Purpose

`TaskWebhook` receives inbound deliveries on a generated endpoint, validates signature/auth and idempotency, and creates a run task from a template task.

## Before You Begin

- `orlojd` is running.
- A template task exists with `spec.mode: template`.
- A secret exists for HMAC signing.

## 1. Apply Prerequisites

Apply template task:

```bash
go run ./cmd/orlojctl apply -f examples/resources/tasks/weekly_report_template_task.yaml
```

Apply webhook signing secret:

```bash
go run ./cmd/orlojctl apply -f examples/resources/secrets/webhook_shared_secret.yaml
```

## 2. Apply a Webhook Resource

Generic profile example:

- [`examples/resources/task-webhooks/generic_webhook.yaml`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/task-webhooks/generic_webhook.yaml)

GitHub profile example:

- [`examples/resources/task-webhooks/github_push_webhook.yaml`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/task-webhooks/github_push_webhook.yaml)

Apply one:

```bash
go run ./cmd/orlojctl apply -f examples/resources/task-webhooks/generic_webhook.yaml
```

## 3. Get the Delivery Endpoint

```bash
curl -s "http://127.0.0.1:8080/v1/task-webhooks/report-generic-webhook?namespace=default" | jq -r '.status.endpointPath'
```

`endpointPath` maps to:

- `POST /v1/webhook-deliveries/{endpoint_id}`

## 4. Send a Signed Test Delivery (Generic Profile)

```bash
BODY='{"event":"report.trigger","topic":"AI startups"}'
TS="$(date +%s)"
SECRET='replace-me'
SIG_HEX="$(printf '%s' "${TS}.${BODY}" | openssl dgst -sha256 -hmac "$SECRET" -binary | xxd -p -c 256)"

curl -i -X POST "http://127.0.0.1:8080$(curl -s "http://127.0.0.1:8080/v1/task-webhooks/report-generic-webhook?namespace=default" | jq -r '.status.endpointPath')" \
  -H "Content-Type: application/json" \
  -H "X-Timestamp: ${TS}" \
  -H "X-Event-Id: evt-001" \
  -H "X-Signature: sha256=${SIG_HEX}" \
  --data "$BODY"
```

Expected response:

- HTTP `202 Accepted`
- JSON with `accepted: true`
- `duplicate: false` on first delivery

## 4b. Send a Signed Test Delivery (GitHub Profile)

```bash
BODY='{"ref":"refs/heads/main","repository":{"full_name":"acme/repo"}}'
SECRET='replace-me'
SIG_HEX="$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -binary | xxd -p -c 256)"

curl -i -X POST "http://127.0.0.1:8080$(curl -s \"http://127.0.0.1:8080/v1/task-webhooks/report-github-push?namespace=default\" | jq -r '.status.endpointPath')" \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Delivery: gh-evt-001" \
  -H "X-Hub-Signature-256: sha256=${SIG_HEX}" \
  --data "$BODY"
```

## 5. Verify Task Creation

```bash
go run ./cmd/orlojctl get tasks
```

Webhook-triggered run tasks include:

- `webhook_payload`
- `webhook_event_id`
- `webhook_received_at`
- `webhook_source`

## Profile Notes

- `generic`: signs `timestamp + "." + rawBody` and checks timestamp skew. Default dedup window is 24 hours.
- `github`: signs raw body and uses GitHub delivery id header defaults. Default dedup window is **72 hours** (vs 24h for generic) because GitHub webhooks do not include a timestamp in the HMAC payload, so replay protection relies entirely on event ID deduplication. The 72-hour window matches GitHub's maximum retry window.

## Rotation and Operations

- Secret rotation: update referenced `Secret`; keep webhook resource unchanged.
- Endpoint rotation: recreate webhook with a new `metadata.name` and update sender URL.
- Duplicate deliveries return `202` with `duplicate: true`.

## Troubleshooting

- `401 signature verification failed`: verify signature algorithm, prefix (`sha256=`), and secret.
- `400 missing event id`: include configured event id header (`X-Event-Id` or `X-GitHub-Delivery`).
- `400 webhook task creation failed`: the webhook was authenticated and deduplicated successfully, but task creation failed (e.g., the referenced task is not a template, or validation failed). The HTTP response returns a generic message; the detailed error is recorded in `status.lastError` on the `TaskWebhook` resource. Inspect it with `orlojctl get task-webhook <name>`.
- `404 webhook endpoint not found`: verify current `.status.endpointPath`.

## Related Docs

- [Task Webhook Examples](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/task-webhooks)
- [Resource Reference (`TaskWebhook`)](../reference/resources/task-webhook.md)
- [API Reference (Webhook Delivery)](../reference/api.md)
