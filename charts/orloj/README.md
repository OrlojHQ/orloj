# Orloj Helm Chart

This chart deploys:

- `orlojd` (API/control plane)
- `orlojworker` (task workers)
- Postgres
- NATS (JetStream enabled)

## Install

```bash
helm upgrade --install orloj ./charts/orloj \
  --namespace orloj \
  --create-namespace \
  --set orlojd.image.repository=ghcr.io/<your-org-or-user>/orloj-orlojd \
  --set orlojworker.image.repository=ghcr.io/<your-org-or-user>/orloj-orlojworker \
  --set orlojd.image.tag=v0.1.0 \
  --set orlojworker.image.tag=v0.1.0 \
  --set postgres.auth.password='<strong-password>' \
  --set runtimeSecret.modelGatewayApiKey='<model-provider-api-key>'
```

## Uninstall

```bash
helm uninstall orloj --namespace orloj
```

## Important Values

- `orlojd.image.repository`, `orlojd.image.tag`
- `orlojworker.image.repository`, `orlojworker.image.tag`
- `postgres.auth.user`, `postgres.auth.password`, `postgres.auth.database`, `postgres.auth.dsn`
- `runtimeSecret.modelGatewayApiKey`
- `runtimeConfig.*` (Orloj runtime env vars)

## Notes

- By default, this chart deploys a single Postgres replica with a PVC and a single NATS replica.
- For production hardening, externalize Postgres/NATS and rotate credentials.
