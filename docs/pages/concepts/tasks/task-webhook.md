# TaskWebhook

A **TaskWebhook** creates [Tasks](./task.md) in response to external HTTP events, with built-in signature verification and idempotency.

## Defining a TaskWebhook

```yaml
apiVersion: orloj.dev/v1
kind: TaskWebhook
metadata:
  name: report-github-push
spec:
  task_ref: weekly-report-template
  auth:
    profile: github
    secret_ref: webhook-shared-secret
  idempotency:
    event_id_header: X-GitHub-Delivery
    dedupe_window_seconds: 86400
  payload:
    mode: raw
    input_key: webhook_payload
```

## Auth Profiles

TaskWebhooks verify incoming requests using HMAC signature verification. Two profiles are supported:

| Profile | Signature Method | Headers |
|---|---|---|
| `generic` | HMAC-SHA256 over `timestamp + "." + rawBody` | `X-Signature`, `X-Timestamp`, `X-Event-Id` |
| `github` | HMAC-SHA256 over raw body | `X-Hub-Signature-256`, `X-GitHub-Delivery` |

The shared secret is stored in a [Secret](../tools/secret.md) resource referenced by `auth.secret_ref`.

## Idempotency

TaskWebhooks deduplicate deliveries using the event ID header. If a delivery with the same event ID arrives within the `dedupe_window_seconds`, it is rejected as a duplicate.

## How It Works

When an HTTP request hits the webhook endpoint:

1. The runtime verifies the HMAC signature against the shared secret.
2. The event ID is checked against the deduplication window.
3. If valid and not a duplicate, a new Task is created from the template.
4. The webhook payload is injected into the task input under `input_key`.

## Related

- [Task](./task.md) -- the tasks that webhooks create
- [TaskSchedule](./task-schedule.md) -- cron-based task automation
- [Resource Reference: TaskWebhook](../../reference/resources/task-webhook.md)
