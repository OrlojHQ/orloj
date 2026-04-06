# TaskWebhook

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `task_ref` (string): template task reference (`name` or `namespace/name`). Mutually exclusive with `task_template`.
- `task_template` (object): inline task spec used instead of a separate template Task. Mutually exclusive with `task_ref`. Fields: `system` (required), `priority`, `input`, `max_turns`, `retry`, `message_retry`.
- `suspend` (bool): rejects deliveries when `true`.
- `auth` (object):
  - `profile` (string): `generic` (default), `github`, `hmac`, or `shared_token`.
  - `secret_ref` (string): required secret reference (`name` or `namespace/name`).
  - `signature_header` (string)
  - `signature_prefix` (string)
  - `timestamp_header` (string): used by `generic` and `hmac` with plain header format.
  - `max_skew_seconds` (int): timestamp tolerance (default `300`).
  - `algorithm` (string): HMAC hash algorithm -- `sha256` (default), `sha1`, `sha512`. Used with `hmac` profile.
  - `payload_format` (string): HMAC signing input -- `body`, `timestamp_dot_body`, or `prefix_timestamp_body`. Used with `hmac` profile.
  - `payload_prefix` (string): literal prefix for `prefix_timestamp_body` format.
  - `payload_separator` (string): separator between parts (default `.`). Used with `prefix_timestamp_body`.
  - `signature_encoding` (string): `hex` (default) or `base64`. Used with `hmac` profile.
  - `header_format` (string): `plain` (default) or `kv_pairs`. Used with `hmac` profile.
  - `signature_key` (string): key for signature in `kv_pairs` header (e.g., `v1` for Stripe).
  - `timestamp_key` (string): key for timestamp in `kv_pairs` header (e.g., `t` for Stripe).
- `idempotency` (object):
  - `event_id_header` (string): header containing unique delivery id.
  - `dedupe_window_seconds` (int): dedupe TTL.
- `payload` (object):
  - `mode` (string): `raw` (v1 only).
  - `input_key` (string): generated task input key for raw payload.

## Defaults and Validation

- Exactly one of `task_ref` or `task_template` must be set.
- `task_ref` must be `name` or `namespace/name`.
- When `task_template` is set, `system` is required; `priority` defaults to `normal`; retry/message_retry defaults mirror Task defaults.
- `auth.secret_ref` is required.
- `auth.profile` defaults to `generic`; supported values: `generic`, `github`, `hmac`, `shared_token`.
- profile defaults:
  - `generic`:
    - `signature_header` -> `X-Signature`
    - `signature_prefix` -> `sha256=`
    - `timestamp_header` -> `X-Timestamp`
    - `idempotency.event_id_header` -> `X-Event-Id`
  - `github`:
    - `signature_header` -> `X-Hub-Signature-256`
    - `signature_prefix` -> `sha256=`
    - `idempotency.event_id_header` -> `X-GitHub-Delivery`
  - `hmac`: `algorithm` -> `sha256`, `payload_format` -> `body`, `signature_encoding` -> `hex`, `header_format` -> `plain`, `payload_separator` -> `.`, `idempotency.event_id_header` -> `X-Event-Id`. `signature_header` is required. `timestamp_header` required when `payload_format` uses a timestamp and `header_format` is `plain`. `signature_key` required when `header_format` is `kv_pairs`.
  - `shared_token`: `signature_header` is required (the header containing the static token). `idempotency.event_id_header` -> `X-Event-Id`.
- `auth.max_skew_seconds` defaults to `300` and must be `>= 0`.
- `idempotency.dedupe_window_seconds` must be `>= 0`. Defaults to `259200` (72 hours) for `github` profile or `86400` (24 hours) for `generic` profile.
- `payload.mode` defaults to `raw` and only `raw` is allowed in v1.
- `payload.input_key` defaults to `webhook_payload`.

## status

- `phase`, `lastError`, `observedGeneration`
- `endpointID`, `endpointPath`
- `lastDeliveryTime`, `lastEventID`, `lastTriggeredTask`
- `acceptedCount`, `duplicateCount`, `rejectedCount`

Example: `examples/resources/task-webhooks/*.yaml`

See also: [Task webhook concepts](../../concepts/tasks/task-webhook.md).
