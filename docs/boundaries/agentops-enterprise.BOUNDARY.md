# Orloj Enterprise Boundary

Intended location: root of private `orloj-enterprise` repository.

## Allowed Scope

- Enterprise identity integrations (SSO/OIDC/SAML, SCIM).
- Org/workspace administration layers.
- Audit export/integrity tooling.
- Human approval workflows and enterprise compliance packaging.
- Air-gapped and hardened deployment assets.

## Guardrails

- Do not move previously open OSS core capabilities behind a paywall.
- Implement enterprise features through OSS extension contracts where possible.
- Keep OSS self-host runtime fully functional without enterprise components.
