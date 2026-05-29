# Threat Model

This page consolidates Orloj's security design: trust boundaries, the attacker model it defends against, the controls at each boundary, and the residual risks operators must own. It is a companion to [Security and Isolation](./security.md), which documents each control in depth.

This is a self-assessment of the open-source platform as built. It is not a certification or a substitute for an environment-specific risk assessment.

## Scope and assumptions

- Covers the Orloj control plane (`orlojd`), workers (`orlojworker`), the Kubernetes operator (`orloj-operator`), the web console, and the governed tool/agent runtime.
- Assumes a self-hosted deployment. Operators are responsible for the host OS, container runtime, network, TLS termination, secret-key custody, and backups.
- Assumes the threat actor does **not** have the control-plane host's root access or the `ORLOJ_SECRET_ENCRYPTION_KEY`. Controls that depend on those secrets are explicitly noted as out of scope below.

## Trust boundaries

```
            untrusted callers / agents authored by users
                              │
            ┌─────────────────▼──────────────────┐  Boundary 1: API edge
            │  orlojd (control plane API + UI)    │  authn, authz, rate limit
            └─────────────────┬──────────────────┘
                              │ governed runtime pipeline   Boundary 2: governance
            ┌─────────────────▼──────────────────┐  policy, RBAC, approvals
            │  orlojworker (task/agent execution) │
            └───────┬───────────────────┬────────┘
                    │                   │
    Boundary 3:     │                   │   Boundary 4: secret custody
    tool sandbox    ▼                   ▼   encryption at rest, redaction
        ┌───────────────────┐   ┌────────────────────┐
        │ isolated tool/     │   │ secret store /      │
        │ agent execution    │   │ sealing keypair     │
        └─────────┬─────────┘   └────────────────────┘
                  │ Boundary 5: outbound egress (SSRF)
                  ▼
         external HTTP / gRPC / MCP / A2A endpoints
```

**Boundary 1 — API edge.** Separates untrusted network callers from the control plane. Enforced by bearer-token / native-session authentication, role checks (reader/writer/admin/a2a), per-IP authentication rate limiting with trusted-proxy handling, and setup-token protection for first-admin creation.

**Boundary 2 — Governance.** Separates an agent's *intent* (which tool it tries to call) from *authorized action*. Enforced fail-closed by `AgentPolicy`, `AgentRole`, `ToolPermission`, and human `ToolApproval`/`TaskApproval` gates with risk-tier routing.

**Boundary 3 — Tool/agent sandbox.** Separates tool/agent code from the worker host. Enforced by isolation modes (sandboxed container defaults: read-only FS, `cap-drop=ALL`, `no-new-privileges`, `network none`, non-root, resource limits; ephemeral Kubernetes Jobs; shell-free CLI exec).

**Boundary 4 — Secret custody.** Separates secret material from operators, the database, logs, and git. Enforced by AES-256-GCM encryption at rest, RSA-4096 SealedSecrets with per-entry data keys and AAD binding, API/event-bus redaction, and write-only secret APIs.

**Boundary 5 — Outbound egress.** Separates Orloj-initiated outbound calls from internal network resources. Enforced by two-stage SSRF validation including a dial-time IP check that blocks loopback, link-local, cloud-metadata, and (by default) private ranges.

## Attacker model

Threats Orloj is designed to resist:

| Attacker | Example | Primary control |
|----------|---------|-----------------|
| Unauthenticated network caller | Hits the API/UI directly | Boundary 1: authn + rate limit |
| Authenticated but under-privileged caller | Reader tries to mutate resources | Boundary 1: role check |
| Malicious/compromised agent | Agent tries to call a tool it shouldn't | Boundary 2: fail-closed governance |
| Hostile tool/MCP code | Tool tries to read host FS or pivot | Boundary 3: sandbox isolation |
| SSRF via tool/agent input | Tool URL points at cloud metadata | Boundary 5: dial-time IP block |
| Secret exfiltration via API/logs | Reads secret values back out | Boundary 4: redaction + encryption at rest |
| Secrets committed to git | Plaintext `Secret` YAML in a repo | SealedSecret workflow |
| Supply-chain tampering of releases | Modified image pulled by users | Cosign signing, SBOM, build provenance, image scanning |
| Credential brute force | Password/token guessing | Argon2id hashing, auth rate limiting |

## Residual risks (operator-owned)

These are **known and accepted** in the current design. Operators must mitigate them at the deployment layer.

1. **Namespaces are not a security boundary by default.** Any authenticated caller with the right role can access any namespace. Per-namespace tenant isolation is available in **Orloj Enterprise**, or self-hosters can implement the `ResourceAuthorizer` hook themselves (see [Multi-tenant authorization](./security.md#multi-tenant-authorization-enterprise)).
2. **Audit logging is off by default.** The runtime emits audit events but persists them only if an `AuditSink` is wired. Enable the reference `SlogAuditSink` and forward to durable storage. See [Audit Logging](./security.md#audit-logging).
3. **Control-plane TLS is the operator's responsibility.** `orlojd`/`orlojworker` serve plain HTTP; terminate TLS at a reverse proxy/load balancer and configure `--trusted-proxies` accordingly. gRPC *tool* connections require TLS 1.2+ by default.
4. **Host root or `ORLOJ_SECRET_ENCRYPTION_KEY` compromise is out of scope.** An attacker with both the database and the encryption key, or with code execution on `orlojd`, can recover secrets. Protect the key in a KMS/HSM and restrict host access.
5. **Sealing keys do not rotate automatically yet.** One active control-plane sealing keypair is used until a manual rotation flow is introduced. Plan periodic manual rotation and protect the key accordingly.
6. **Private-endpoint and public-A2A opt-ins weaken defaults.** `--a2a-allow-private-endpoints`, `ModelEndpoint spec.allowPrivate`, and `a2a.auth: public` are deliberate trade-offs; enable them only in trusted network contexts.
7. **GitHub Actions / dependency trust.** CI now runs dependency, SAST, secret, and image scanning, but a compromised upstream dependency or action can still introduce risk. Keep Dependabot current and review third-party action updates.

## Verifying the model

The controls above are exercised by CI (build/test/vet, govulncheck, CodeQL, gitleaks, Trivy) and by conformance tests for sandbox defaults and SSRF enforcement. Re-run these scans when the architecture changes.

## Related docs

- [Security and Isolation](./security.md)
- [Governance and Policies](../concepts/governance/index.md)
- [Worker](../concepts/infrastructure/worker.md)
