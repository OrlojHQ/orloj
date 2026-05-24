# AgentSystem

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `agents` ([]string): participating agent names.
- `graph` (map[string]GraphEdge): per-node routing.
- `context_adapter` (string): optional reference to a [ContextAdapter](./context-adapter.md) resource. When set, the adapter's tool sanitizes raw task input before the first agent runs.
- `completion_review` (ReviewCheckpoint): optional final human review before the task is marked `Succeeded`.
- `a2a.enabled` (bool): expose this AgentSystem through inbound A2A Agent Card discovery and JSON-RPC invocation. Omitted or `false` keeps the system off the A2A surface.
- `a2a.auth` (string: `"public"` | `"bearer"`): controls whether A2A invoke requires authentication for this system. `"public"` allows unauthenticated callers; `"bearer"` (default when omitted) requires a valid API token when instance-wide auth is enabled. Public systems' Agent Cards omit `authentication.schemes` so A2A clients know not to send tokens.

`GraphEdge` fields:

- `next` (string): legacy single-hop route.
- `edges` ([]GraphRoute): fan-out routes.
  - `to` (string)
  - `labels` (map[string]string)
  - `policy` (map[string]string)
- `join` (GraphJoin): fan-in behavior.
  - `mode`: `wait_for_all` or `quorum`
  - `quorum_count` (int, >= 0)
  - `quorum_percent` (int, 0-100)
  - `on_failure`: `deadletter`, `skip`, `continue_partial`
- `review` (ReviewCheckpoint): optional human review checkpoint for this node's output.

`ReviewCheckpoint` fields:

- `checkpoint_id` (string, required)
- `display_name` (string)
- `reason` (string)
- `ttl` (duration string, defaults to `10m`)
- `allow_request_changes` (bool, defaults to `true`)
- `max_review_cycles` (int, defaults to `3`)

If `allow_request_changes` is `false`, reviewers can only approve or deny that checkpoint. Once `max_review_cycles` is reached, additional `request_changes` attempts are rejected.

## Defaults and Validation

- `graph[*].next` and `graph[*].edges[].to` are trimmed.
- Route targets are normalized/deduplicated for execution.
- `join` normalization defaults:
  - `mode` -> `wait_for_all`
  - `on_failure` -> `deadletter`
  - `quorum_percent` clamped to `0..100`
  - invalid values are coerced to safe defaults in graph normalization.
- Runtime task validation additionally checks:
  - graph nodes/edges must reference agents in `spec.agents`
  - cyclic graphs require `Task.spec.max_turns > 0`
  - non-cyclic graphs require at least one entrypoint (zero indegree node)
  - review `checkpoint_id` values must be unique within the system

## status

- `phase`, `lastError`, `observedGeneration`

Example: [`examples/resources/agent-systems/`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/agent-systems)

See also: [Agent system concept](../../concepts/agents/agent-system.md)
