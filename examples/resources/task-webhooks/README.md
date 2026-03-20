# TaskWebhook Examples

These manifests trigger `Task` templates (`spec.mode: template`) from webhook deliveries.

## Resources

- `generic_webhook.yaml`: generic HMAC profile (`X-Signature`, `X-Timestamp`, `X-Event-Id`)
- `github_push_webhook.yaml`: GitHub preset (`X-Hub-Signature-256`, `X-GitHub-Delivery`)

## Apply

```bash
go run ./cmd/orlojctl apply -f examples/resources/tasks/weekly_report_template_task.yaml
go run ./cmd/orlojctl apply -f examples/resources/secrets/webhook_shared_secret.yaml
go run ./cmd/orlojctl apply -f examples/resources/task-webhooks/generic_webhook.yaml
```

Fetch endpoint details:

```bash
go run ./cmd/orlojctl get task-webhooks
```

`ENDPOINT_PATH` maps to `POST /v1/webhook-deliveries/{endpoint_id}`.

## Rotation Runbook

- Rotate secret value: update the referenced `Secret` and keep the same `TaskWebhook`.
- Rotate endpoint id/path: recreate the `TaskWebhook` with a new `metadata.name` (or namespace/name pair), then update upstream webhook sender URL.
