# AgentSystem

> **Stability: beta** -- This resource kind ships with `orloj.dev/v1` and is suitable for production use, but its schema may evolve with migration guidance in future minor releases.

## spec

- `agents` ([]string): participating agent names.
- `graph` (map[string]GraphEdge): per-node routing.

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

## status

- `phase`, `lastError`, `observedGeneration`

Example: [`examples/resources/agent-systems/`](https://github.com/OrlojHQ/orloj/tree/main/examples/resources/agent-systems)

See also: [Agent system concept](../../concepts/agents/agent-system.md)
