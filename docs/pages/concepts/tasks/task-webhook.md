# TaskWebhook

A **TaskWebhook** creates [Tasks](./task.md) in response to external HTTP events, with built-in signature verification and idempotency.

## Defining a TaskWebhook

A TaskWebhook can reference a separate template Task via `task_ref`, or define the task spec inline via `task_template`. Exactly one must be set.

### Using a template reference

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

### Using an inline template

When only one webhook uses a template, you can embed the task spec directly in the webhook to avoid creating a separate Task resource:

```yaml
apiVersion: orloj.dev/v1
kind: TaskWebhook
metadata:
  name: ingest-events
spec:
  task_template:
    system: event-pipeline
    priority: normal
    input:
      webhook_payload: ""
  auth:
    profile: generic
    secret_ref: ingest-secret
  idempotency:
    event_id_header: X-Event-Id
  payload:
    input_key: webhook_payload
```

## Auth Profiles

TaskWebhooks verify incoming requests using signature verification. Four profiles are supported:

| Profile | Signature Method | Headers |
|---|---|---|
| `generic` | HMAC-SHA256 over `timestamp + "." + rawBody` | `X-Signature`, `X-Timestamp`, `X-Event-Id` |
| `github` | HMAC-SHA256 over raw body | `X-Hub-Signature-256`, `X-GitHub-Delivery` |
| `hmac` | Configurable HMAC (algorithm, payload format, encoding, header parsing) | User-defined |
| `shared_token` | Constant-time comparison of a static token header | User-defined |

The shared secret is stored in a [Secret](../tools/secret.md) resource referenced by `auth.secret_ref`.

### `hmac` Profile

The `hmac` profile provides full control over HMAC verification for services that don't match the `generic` or `github` presets.

| Field | Description | Default |
|---|---|---|
| `algorithm` | Hash algorithm: `sha256`, `sha1`, `sha512` | `sha256` |
| `payload_format` | How to construct the signing input: `body`, `timestamp_dot_body`, `prefix_timestamp_body` | `body` |
| `payload_prefix` | Literal prefix for `prefix_timestamp_body` (e.g., `v0`) | |
| `payload_separator` | Separator between payload parts | `.` |
| `signature_encoding` | Signature encoding: `hex` or `base64` | `hex` |
| `header_format` | How to parse the signature header: `plain` or `kv_pairs` | `plain` |
| `signature_key` | Key holding the signature in a `kv_pairs` header | |
| `timestamp_key` | Key holding the timestamp in a `kv_pairs` header | |

Example -- Stripe:

```yaml
auth:
  profile: hmac
  secret_ref: stripe-secret
  algorithm: sha256
  payload_format: timestamp_dot_body
  signature_header: Stripe-Signature
  header_format: kv_pairs
  signature_key: v1
  timestamp_key: t
  signature_encoding: hex
```

Example -- Slack:

```yaml
auth:
  profile: hmac
  secret_ref: slack-signing-secret
  algorithm: sha256
  payload_format: prefix_timestamp_body
  payload_prefix: "v0"
  payload_separator: ":"
  signature_header: X-Slack-Signature
  signature_prefix: "v0="
  timestamp_header: X-Slack-Request-Timestamp
  signature_encoding: hex
```

### `shared_token` Profile

For services that send a static secret token in a header (no HMAC). Comparison is constant-time.

Example -- Telegram:

```yaml
auth:
  profile: shared_token
  secret_ref: telegram-bot-secret
  signature_header: X-Telegram-Bot-Api-Secret-Token
```

## Idempotency

TaskWebhooks deduplicate deliveries using the event ID header. If a delivery with the same event ID arrives within the `dedupe_window_seconds`, it is rejected as a duplicate.

## How It Works

When an HTTP request hits the webhook endpoint:

1. The runtime verifies the request against the shared secret using the configured auth profile (HMAC signature or shared token comparison).
2. The event ID is checked against the deduplication window.
3. If valid and not a duplicate, a new Task is created from the template.
4. The webhook payload is injected into the task input under `input_key`.

## Related

- [Task](./task.md) -- the tasks that webhooks create
- [TaskSchedule](./task-schedule.md) -- cron-based task automation
- [Resource Reference: TaskWebhook](../../reference/resources/task-webhook.md)
