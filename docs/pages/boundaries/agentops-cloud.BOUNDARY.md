# Orloj Cloud Boundary

Intended location: root of private `orloj-cloud` repository.

## Allowed Scope

- Org/project/account tenancy server services.
- Hosted operations automation (reliability, backups, restore orchestration, SLA workflows).
- Billing, metering ingestion, and usage reporting pipelines.

## Integration Rules

- Consume OSS extension contracts from `orloj`.
- Do not patch or fork OSS core internals for cloud-only behavior.
- Preserve compatibility with published OSS interface/version contracts.
