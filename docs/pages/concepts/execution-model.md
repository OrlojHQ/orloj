# Execution and Messaging

This page documents task routing, message lifecycle, and ownership guarantees.

## Graph Routing

`AgentSystem.spec.graph` supports two edge styles:

- `next`: legacy single edge
- `edges[]`: preferred route list with labels/policy metadata

### Conditional Edge Routing

Edges in the `edges[]` list can carry an optional `condition` that is evaluated against the completing agent's output. When conditions are present, only edges whose condition matches will fire. Edges without conditions are unconditional and always fire. If no conditional edge matches, a `default: true` edge fires as a fallback.

Conditions support both string-based operators (`output_contains`, `output_not_contains`, `output_matches`) and JSON path operators (`output_json_path` with `equals`, `not_equals`, `contains`, `greater_than`, `less_than`). JSON path conditions pair with `output_schema` on the Agent spec for guaranteed structured routing.

See [AgentSystem -- Conditional Routing](./agents/agent-system.md#conditional-routing) for full details and patterns, and [AgentSystem -- Structured Output](./agents/agent-system.md#structured-output) for provider-native schema enforcement.

Conditional routing requires message-driven execution mode.

## Fan-out and Fan-in

- Fan-out: one node routes to multiple downstream edges. When conditional routing is active, only matched edges fan out.
- Fan-in: downstream join gate with:
  - `wait_for_all`
  - `quorum` (`quorum_count` or `quorum_percent`)

Join gates automatically adjust their expected branch count when conditional routing reduces the set of dispatched upstream agents. Join state persists in `Task.status.join_states`.

## Delegation

Graph nodes can declare `delegates` alongside `edges`. This gives a node two-phase execution:

1. **Dispatch**: After the node's first execution, the output filters `delegates` (same condition logic as edges). Matched delegates receive messages with `delegate_of` metadata.
2. **Collect**: A delegation gate tracks returns. Join modes (`wait_for_all`, `quorum`) control when the gate fires.
3. **Review**: The node re-executes with delegation context (`inbox.delegation.*`).
4. **Forward**: After review, normal `edges` fire.

Delegates that reach a terminal point (no outgoing edges match) automatically route back to the delegator. The `delegate_of` field propagates through sub-branches, enabling multi-hop delegation trees. Delegation state persists in `Task.status.delegation_states`.

See [AgentSystem -- Delegation](./agents/agent-system.md#delegation) for full details, patterns, and examples.

## Message Lifecycle

`Task.status.messages` includes:

- lifecycle phase: `queued|running|retrypending|waitingapproval|succeeded|deadletter`
- retry fields: `attempts`, `max_attempts`, `next_attempt_at`
- worker ownership fields: `worker`, `processed_at`, `last_error`
- routing/tracing fields: `branch_id`, `parent_branch_id`, `trace_id`, `parent_id`

When a workflow hits a review checkpoint, the current message moves to `waitingapproval` until the linked `TaskApproval` is approved, denied, expired, or rerouted through `request_changes`.

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

## Choosing an Execution Mode

Orloj supports two execution modes that share the same resource model and graph definitions.

**Sequential mode** (`--task-execution-mode=sequential`) runs the entire graph in-process on the server or embedded worker. Best for getting started, development, and single-agent systems. No message bus required.

**Message-driven mode** (`--task-execution-mode=message-driven`) distributes execution across workers via the message bus. Each agent step is a queued message with durable delivery, retry, and dead-letter guarantees. Best for production, parallel fan-out, and horizontal scaling.

Both modes produce the same task trace, history, and output. You can develop in sequential mode and deploy to production in message-driven mode without changing your resource definitions.

See [Configuration](../operations/configuration.md) for the full set of flags.

## Related Docs

- [Architecture Overview](./architecture.md)
- [Configuration](../operations/configuration.md)
