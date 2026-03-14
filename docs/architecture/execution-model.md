# Execution and Messaging Model

## Graph Routing

`AgentSystem.spec.graph` supports two edge styles:

- legacy single edge: `next`
- rich edge list: `edges[]` (preferred)

Rich edges allow route-level metadata for labels and policy expansion.

## Fan-out / Fan-in

- Fan-out: one node emits to multiple downstream edges in a single hop.
- Fan-in: downstream node join gate with:
  - `wait_for_all`
  - `quorum` (`quorum_count` or `quorum_percent`)

Join state is persisted in `Task.status.join_states`.

## Message Lifecycle

Messages in `Task.status.messages` include lifecycle fields:

- `phase`: `queued|running|retrypending|succeeded|deadletter`
- `attempts`, `max_attempts`, `next_attempt_at`
- `worker`, `processed_at`, `last_error`
- routing/tracing fields: `branch_id`, `parent_branch_id`, `trace_id`, `parent_id`

## Agent Tool Selection

- tools listed in `Agent.spec.tools[]` define the candidate set for a step
- runtime builds a governed per-agent tool runtime from that set (policy + role/permission checks)
- model response now selects which tool(s) to invoke for the current step
- only selected and authorized tools are executed; unrequested tools are not auto-invoked
- unauthorized model-selected tools are denied and surfaced as hard failures (`tool_permission_denied`)

## Ownership and Safety

- only `Task.status.claimedBy` worker may start message processing
- lease renewal during processing (`leaseUntil`, `lastHeartbeat`)
- takeover-safe handoff when lease expires
- persistent idempotency keys protect replay/crash recovery
