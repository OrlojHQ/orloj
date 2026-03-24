# TaskWebhook

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `task_ref` (string): template task reference (`name` or `namespace/name`).
- `suspend` (bool): rejects deliveries when `true`.
- `auth` (object):
  - `profile` (string): `generic` (default) or `github`.
  - `secret_ref` (string): required secret reference (`name` or `namespace/name`).
  - `signature_header` (string)
  - `signature_prefix` (string)
  - `timestamp_header` (string): used by `generic`.
  - `max_skew_seconds` (int): timestamp tolerance for `generic`.
- `idempotency` (object):
  - `event_id_header` (string): header containing unique delivery id.
  - `dedupe_window_seconds` (int): dedupe TTL.
- `payload` (object):
  - `mode` (string): `raw` (v1 only).
  - `input_key` (string): generated task input key for raw payload.

## Defaults and Validation

- `task_ref` is required and must be `name` or `namespace/name`.
- `auth.secret_ref` is required.
- `auth.profile` defaults to `generic`; supported values: `generic`, `github`.
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
