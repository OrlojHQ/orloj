# Execution and Messaging

This page documents task routing, message lifecycle, and ownership guarantees.

## Graph Routing

`AgentSystem.spec.graph` supports two edge styles:

- `next`: legacy single edge
- `edges[]`: preferred route list with labels/policy metadata

## Fan-out and Fan-in

- Fan-out: one node routes to multiple downstream edges.
- Fan-in: downstream join gate with:
  - `wait_for_all`
  - `quorum` (`quorum_count` or `quorum_percent`)

Join state persists in `Task.status.join_states`.

## Message Lifecycle

`Task.status.messages` includes:

- lifecycle phase: `queued|running|retrypending|succeeded|deadletter`
- retry fields: `attempts`, `max_attempts`, `next_attempt_at`
- worker ownership fields: `worker`, `processed_at`, `last_error`
- routing/tracing fields: `branch_id`, `parent_branch_id`, `trace_id`, `parent_id`

## Tool Selection Model

- `Agent.spec.tools[]` defines candidate tools.
- Model responses select specific tool calls for each step.
- Only selected and authorized tools are executed.
- Unauthorized tool selections fail closed as `tool_permission_denied`.

## Ownership and Safety Guarantees

- only `Task.status.claimedBy` worker may process messages
- leases are renewed during active processing
- lease expiry allows safe takeover by another worker
- idempotency keys protect replay and crash recovery

## Related Docs

- [Tool Contract v1](../reference/tool-contract-v1.md)
- [Tool Runtime Conformance](../operations/tool-runtime-conformance.md)
