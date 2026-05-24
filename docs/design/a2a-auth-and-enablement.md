# A2A Authentication and Per-System Enablement

**Status:** Draft  
**Date:** 2026-05-23  
**Authors:** Jon Mandraki

## Problem

Orloj has implemented the A2A protocol, but the auth model doesn't support the primary use case: an operator running an Orloj instance wants to expose specific AgentSystems to external callers via A2A, optionally behind authentication, without giving those callers access to the Orloj control plane.

Today, A2A is a global server-level flag (`--a2a-enabled`). When it's on, every agent is on the A2A surface. There is no way to expose one AgentSystem while keeping others internal. And when auth is enabled, external callers need a token with the `writer` role — which also grants them full mutation access to agents, tools, secrets, and other Orloj resources.

## Background

### How A2A works in Orloj today

A2A is a system-to-system protocol. An external agent system discovers an Orloj system via its Agent Card, then sends tasks via JSON-RPC. Orloj internally routes to the appropriate agent(s). The external caller doesn't address individual agents — the AgentSystem is the A2A boundary.

Current A2A routes:

| Route | Method | Purpose |
|-------|--------|---------|
| `/.well-known/agent-card.json` | GET | System-level agent card (serves first agent) |
| `/a2a` | POST | Top-level JSON-RPC endpoint |
| `/v1/agents/{name}/.well-known/agent-card.json` | GET | Per-system agent card |
| `/v1/agents/{name}/a2a` | POST | Per-system JSON-RPC endpoint |
| `/v1/a2a/agents` | GET | Registry listing all agents |

All routes go through `withAuth` middleware. There is no special handling for A2A paths in the authorization layer.

### Current auth model

`requiredRoleForRequest()` in `api/authz.go` assigns roles by HTTP method and path. A2A has no explicit entries — POST to `/a2a` falls through to the default `return "writer"` (line 427). GET to agent cards falls through to `return "reader"` (line 410).

Available roles and their hierarchy:

| Role | Satisfies | Purpose |
|------|-----------|---------|
| `admin` | everything | Full control-plane access |
| `writer` | writer, reader | Resource mutation + read |
| `controller` | controller, reader | Status patches + read |
| `reader` | reader | Read-only |

Token sources:

- `ORLOJ_API_TOKEN` — single env token, defaults to **admin** role
- `ORLOJ_API_TOKENS` — comma-separated `name:token:role` entries
- `POST /v1/tokens` — admin-only; creates named tokens in Postgres

### What's wrong

1. **No per-system A2A toggle.** The global `--a2a-enabled` flag is all-or-nothing. An operator with multiple AgentSystems cannot expose some via A2A while keeping others internal.

2. **No A2A-appropriate role.** External callers need `writer` to POST to A2A endpoints. But `writer` also grants access to create/update/delete agents, tools, secrets, MCP servers, and other resources. There is no role that means "can invoke A2A but nothing else."

3. **Discovery requires auth when auth is on.** In production, auth is always enabled. Currently, agent card GET requires `reader` role — so external systems can't even discover the agent card without a token. In the A2A model, discovery should be public for A2A-enabled systems.

4. **Agent card may not advertise auth requirements.** Auth schemes in the card are only set when `authMode != AuthModeOff` (in `cmd/orlojd/main.go`). With `auth-mode=off` + `ORLOJ_API_TOKEN` set, auth is enforced but the card omits the `authentication` block. External systems doing spec-compliant discovery won't know auth is required.

5. **Global flag is redundant if per-system enablement exists.** If each AgentSystem declares whether it's on the A2A surface, the global `--a2a-enabled` flag adds no value and creates a potential misconfiguration (system has `a2a.enabled: true` but global flag is off).

## Design

### Per-system A2A enablement

Add an A2A configuration block to `AgentSystemSpec`:

```go
type AgentSystemA2ASpec struct {
    Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}
```

```go
type AgentSystemSpec struct {
    Agents           []string              `json:"agents,omitempty" yaml:"agents,omitempty"`
    Graph            map[string]GraphEdge  `json:"graph,omitempty" yaml:"graph,omitempty"`
    // ... existing fields ...
    A2A              AgentSystemA2ASpec    `json:"a2a,omitempty" yaml:"a2a,omitempty"`
}
```

Operator manifest:

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: legal-vault
  annotations:
    orloj.dev/description: "Legal research and case law analysis"
spec:
  agents: [legal-research-agent, case-summary-agent]
  a2a:
    enabled: true
```

The `a2a.enabled` field replaces the global `--a2a-enabled` server flag. A2A routes are active when at least one AgentSystem has `a2a.enabled: true`. The global flag is removed.

### New `a2a` role

Add a new role to the authorization model that permits A2A invoke and read access but denies control-plane mutations.

Updated role hierarchy:

| Role | Satisfies | Purpose |
|------|-----------|---------|
| `admin` | everything | Full control-plane access |
| `writer` | writer, a2a, reader | Resource mutation + A2A + read |
| `a2a` | a2a, reader | A2A invoke + read |
| `controller` | controller, reader | Status patches + read |
| `reader` | reader | Read-only |

`writer` and `admin` both satisfy `a2a` for backward compatibility — existing tokens continue to work.

### A2A path routing

Update `requiredRoleForRequest()` to explicitly handle A2A paths instead of falling through to `writer`:

| Path | Method | Required role |
|------|--------|--------------|
| `/.well-known/agent-card.json` | GET | `""` (public, for a2a-enabled systems) |
| `/v1/agents/{name}/.well-known/agent-card.json` | GET | `""` (public, for a2a-enabled systems) |
| `/v1/a2a/agents` | GET | `""` (public registry of a2a-enabled systems) |
| `/a2a` | POST | `a2a` |
| `/v1/agents/{name}/a2a` | POST | `a2a` |

Discovery endpoints (GET) are public — this is how external systems find you. The A2A handlers themselves check whether the target AgentSystem has `a2a.enabled: true` and return 404 if not, so unenabled systems are not discoverable even though the route is unauthenticated.

Invoke endpoints (POST) require the `a2a` role when auth is active.

### Agent card auth advertisement

Fix the agent card generation to accurately reflect auth requirements. Currently in `cmd/orlojd/main.go`:

```go
if authMode != api.AuthModeOff {
    authSchemes = append(authSchemes, "bearer")
}
```

This should reflect whether token auth is actually active (tokens configured), not just whether `auth-mode` is explicitly set. When auth is active, the card advertises `authentication.schemes: ["bearer"]` so the calling system knows to send a token for invoke.

### Handler changes

Update A2A handlers in `api/a2a.go`:

- **`handleWellKnownAgentCard`** — instead of serving `agents[0]`, find the first AgentSystem with `a2a.enabled: true` and serve its system card. Return 404 if none found.
- **`handleAgentCard`** — look up AgentSystem by name. Return 404 if not found or not a2a-enabled.
- **`handleA2AJSONRPC`** — resolve target system. Reject with error if the target AgentSystem doesn't have `a2a.enabled: true`.
- **`handleA2ARegistry`** — only list AgentSystems with `a2a.enabled: true`.

## Operator workflow

### Open A2A (no auth)

For dev, demos, or intentionally public systems:

```yaml
# Auth is off (default), system is A2A-enabled
kind: AgentSystem
metadata:
  name: public-assistant
spec:
  agents: [helper-agent]
  a2a:
    enabled: true
```

Anyone can discover and invoke. No tokens needed.

### Authenticated A2A (production)

```yaml
# System is A2A-enabled
kind: AgentSystem
metadata:
  name: legal-vault
spec:
  agents: [legal-research-agent, case-summary-agent]
  a2a:
    enabled: true
```

```bash
# Operator enables auth (always on in production)
# and mints an a2a-role token for the external caller
curl -X POST /v1/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"name": "acme-corp", "role": "a2a"}'
# Returns a token that can ONLY invoke A2A and read agent cards
```

External caller:

```bash
# Discovery — no token needed
curl GET /.well-known/agent-card.json
# → Returns legal-vault card with authentication.schemes: ["bearer"]

# Invoke — requires a2a token
curl -X POST /v1/agents/legal-vault/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -d '{"jsonrpc":"2.0","method":"tasks/send",...}'
# → 200 OK

# Control plane — blocked
curl -X POST /v1/agents \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -d '...'
# → 403 Forbidden
```

### Mixed systems (some A2A, some internal)

```yaml
# Exposed via A2A
kind: AgentSystem
metadata:
  name: legal-vault
spec:
  agents: [legal-research-agent, case-summary-agent]
  a2a:
    enabled: true
---
# Internal only — not on A2A surface
kind: AgentSystem
metadata:
  name: internal-ops
spec:
  agents: [monitoring-agent, alerting-agent]
  # no a2a block, or a2a.enabled: false
```

`internal-ops` is invisible to A2A — no card, no invoke, doesn't appear in the registry. But it's fully usable internally via the Orloj control plane.

## Implementation plan

### Files to change

#### 1. Add `a2a` role

- **`store/auth_store.go`** — add `"a2a"` to `allowedAuthRoleValues` map
- **`api/authz.go`** — add `"a2a"` to `normalizeAPIRole` switch; update `roleAllows` so `a2a` satisfies `reader` and `a2a`, `writer`/`admin` satisfy `a2a`

#### 2. Route A2A paths

- **`api/authz.go`** — add explicit cases in `requiredRoleForRequest()` for A2A paths: POST to `/a2a` and paths ending in `/a2a` return `"a2a"`; GET to well-known and registry paths return `""` (public)

#### 3. Add A2A spec to AgentSystem

- **`resources/resource_types.go`** — add `AgentSystemA2ASpec` struct and `A2A` field to `AgentSystemSpec`

#### 4. Remove global A2A flag

- **`cmd/orlojd/main.go`** — remove `--a2a-enabled` flag; derive A2A availability from stored AgentSystem specs
- **`api/server.go`** — update `A2AConfig` to remove the `Enabled` field; the handlers check per-system enablement instead

#### 5. Update A2A handlers

- **`api/a2a.go`** — update all handlers to check `AgentSystem.Spec.A2A.Enabled` instead of `s.a2aConfig.Enabled`; filter registry to only a2a-enabled systems; make discovery endpoints serve only a2a-enabled systems

#### 6. Fix agent card auth advertisement

- **`cmd/orlojd/main.go`** — set auth schemes when token auth is active, not just when `authMode != off`

#### 7. Generated artifacts

- Run `make generate-crds` — commit updated YAML under `config/crd/bases/`
- Sync Helm chart at `charts/orloj/templates/operator-crds.yaml`
- Update OpenAPI schemas under `openapi/` — add `a2a` to role enum, add A2A spec to AgentSystem schema
- Run `npx --yes @redocly/cli@1.28.5 lint openapi/openapi.yaml`

#### 8. Tests

- `roleAllows` — `a2a` satisfies `reader`/`a2a` but not `writer`; `writer`/`admin` satisfy `a2a`
- `requiredRoleForRequest` — returns `"a2a"` for POST to A2A paths; returns `""` for GET to discovery paths
- AgentSystem with `a2a.enabled: true` is discoverable and invokable
- AgentSystem without `a2a.enabled` returns 404 on card and rejects invoke
- `a2a`-role token cannot POST to `/v1/agents` or `/v1/tools`

#### 9. Changelog

- Add entries under `## [Unreleased]` → `Added`

## Out of scope (future work)

- **Per-system token scoping** — restricting an `a2a` token to specific AgentSystems. Currently an `a2a` token can invoke any a2a-enabled system on the instance. For multi-tenant scenarios where different customers should only access their system, token scoping or the `ResourceAuthorizer` extension point could be used. Not needed when the instance runs a single a2a-enabled system.
- **Per-subscriber rate limiting** — current A2A rate limiting is IP-based. Per-token rate limiting would require identity-aware rate limiting.
- **OAuth / mTLS auth schemes** — the card can advertise additional auth schemes beyond `bearer`. Supporting these requires integration with external identity providers.
- **Lessee self-service** — token rotation, usage dashboards, etc. Currently all token management is admin-only via `/v1/tokens`.
