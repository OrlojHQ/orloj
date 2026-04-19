# AgentSystem

An **AgentSystem** composes multiple [Agents](./agent.md) into a directed graph that Orloj executes as a coordinated workflow. The graph defines how messages flow between agents during task execution.

Agent systems can also declare human review checkpoints:

- `spec.graph.<node>.review`: review a node's output before downstream routing continues
- `spec.completion_review`: review the final task output before the task is marked `Succeeded`

## Defining an AgentSystem

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: report-system
  labels:
    orloj.dev/domain: reporting
    orloj.dev/usecase: weekly-report
spec:
  agents:
    - planner-agent
    - research-agent
    - writer-agent
  graph:
    planner-agent:
      next: research-agent
    research-agent:
      next: writer-agent
```

## Graph Topologies

The `graph` field supports three fundamental patterns:

**Pipeline** -- sequential stage-by-stage execution where each agent hands off to the next.

```yaml
graph:
  planner-agent:
    edges:
      - to: research-agent
  research-agent:
    edges:
      - to: writer-agent
```

**Hierarchical** -- a manager delegates to leads, who delegate to workers, with a join gate that waits for all branches before proceeding.

```yaml
graph:
  manager-agent:
    edges:
      - to: research-lead-agent
      - to: social-lead-agent
  research-lead-agent:
    edges:
      - to: research-worker-agent
  social-lead-agent:
    edges:
      - to: social-worker-agent
  research-worker-agent:
    edges:
      - to: editor-agent
  social-worker-agent:
    edges:
      - to: editor-agent
  editor-agent:
    join:
      mode: wait_for_all
```

**Swarm with loop** -- parallel scouts report back to a coordinator in iterative cycles, bounded by `Task.spec.max_turns`.

```yaml
graph:
  coordinator-agent:
    edges:
      - to: scout-alpha-agent
      - to: scout-beta-agent
      - to: synthesizer-agent
  scout-alpha-agent:
    edges:
      - to: coordinator-agent
  scout-beta-agent:
    edges:
      - to: coordinator-agent
```

## Fan-out and Fan-in

When a graph node has multiple outbound edges, messages fan out to all targets in parallel. Fan-in is handled through join gates:

| Join Mode | Behavior |
|---|---|
| `wait_for_all` | Waits for every upstream branch to complete before activating the join node. |
| `quorum` | Activates after `quorum_count` or `quorum_percent` of upstream branches complete. |

If an upstream branch fails, the `on_failure` policy determines behavior: `deadletter` (default), `skip`, or `continue_partial`.

## Human Review Checkpoints

Attach a review checkpoint to a graph node when a human must inspect that output before the workflow continues.

```yaml
graph:
  writer-agent:
    review:
      checkpoint_id: writer-review
      reason: Editor must approve the draft before compliance runs.
      ttl: 30m
      allow_request_changes: true
      max_review_cycles: 3
    next: compliance-agent
completion_review:
  checkpoint_id: final-review
  reason: Final human signoff before success.
```

When a checkpoint is reached, Orloj creates a `TaskApproval`, pauses the task in `WaitingApproval`, and records the exact blocker in `Task.status.blocked_on`.

If `allow_request_changes` is `false`, reviewers can only approve or deny that checkpoint. If a reviewer keeps sending work back, `max_review_cycles` caps the number of rerun loops before Orloj rejects additional `request_changes` decisions.

## Conditional Routing

Edges can carry a `condition` that is evaluated against the completing agent's output. Only edges whose condition matches will fire. This enables data-dependent graph routing where agents decide which downstream agents should run.

```yaml
graph:
  classifier-agent:
    edges:
      - to: billing-agent
        condition:
          output_contains: "BILLING"
      - to: tech-agent
        condition:
          output_contains: "TECH"
      - to: general-agent
        condition:
          default: true
```

### Condition Operators

**String-based operators** -- evaluate against the raw output text:

| Operator | Behavior |
|---|---|
| `output_contains` | Matches if the agent's output contains the string (case-insensitive). |
| `output_not_contains` | Matches if the agent's output does NOT contain the string (case-insensitive). |
| `output_matches` | Matches if the agent's output matches the regex pattern. |
| `default` | Marks the edge as a fallback — fires only when no conditional edge matches. |

**JSON path operators** -- extract a value from JSON output and compare:

| Operator | Behavior |
|---|---|
| `output_json_path` | Dot-notation path (e.g. `$.route`, `$.result.category`) to extract from JSON output. Required when using comparison operators. |
| `equals` | Matches when the extracted value equals this string. |
| `not_equals` | Matches when the extracted value does NOT equal this string. |
| `contains` | For arrays: matches when any element equals this value. For strings: matches on substring (case-insensitive). |
| `greater_than` | Matches when the extracted numeric value is greater than this threshold. |
| `less_than` | Matches when the extracted numeric value is less than this threshold. |

JSON path operators require the agent's output to be valid JSON. When `output_json_path` is set, at least one comparison operator (`equals`, `not_equals`, `contains`, `greater_than`, `less_than`) must also be set. If the output is not valid JSON or the path does not exist, the condition evaluates to false.

### Evaluation Rules

- Edges **without** a condition are unconditional and always fire (backward-compatible).
- When any edges have conditions: conditional edges are evaluated first. If one or more match, only matched edges (plus any unconditional edges) fire.
- If no conditional edge matches, `default: true` edges fire (plus unconditional edges).
- If no conditional edge matches and no default exists, the task completes at this node.
- At most one `default` edge is allowed per source node.
- A `default` edge must not combine with other condition fields.
- Multiple condition fields on the same edge are combined with AND logic.

### Patterns

**Triage / routing** -- a classifier agent routes to the right specialist:

```yaml
graph:
  intake-agent:
    edges:
      - to: refund-agent
        condition:
          output_contains: "REFUND"
      - to: support-agent
        condition:
          output_contains: "SUPPORT"
      - to: general-agent
        condition:
          default: true
```

**Quality gates** -- skip expensive downstream stages when not needed:

```yaml
graph:
  screener-agent:
    edges:
      - to: deep-research-agent
        condition:
          output_contains: "VIABLE"
      - to: rejection-agent
        condition:
          output_contains: "REJECT"
```

**Intelligent hierarchical delegation** -- a manager activates only the leads that are needed, saving cost and time:

```yaml
graph:
  manager-agent:
    edges:
      - to: research-lead
        condition:
          output_contains: "NEEDS_RESEARCH"
      - to: engineering-lead
        condition:
          output_contains: "NEEDS_ENGINEERING"
      - to: legal-lead
        condition:
          output_contains: "NEEDS_LEGAL"
  research-lead:
    edges:
      - to: editor-agent
  engineering-lead:
    edges:
      - to: editor-agent
  legal-lead:
    edges:
      - to: editor-agent
  editor-agent:
    join:
      mode: wait_for_all
```

When the manager only activates research-lead and legal-lead, the editor's `wait_for_all` join gate automatically adjusts to wait for 2 branches instead of 3.

**Structured JSON routing** -- combine `output_schema` with JSON path conditions for reliable, typed routing:

```yaml
graph:
  classifier-agent:
    edges:
      - to: research-lead
        condition:
          output_json_path: "$.route"
          equals: "research"
      - to: legal-lead
        condition:
          output_json_path: "$.domains"
          contains: "legal"
      - to: high-priority-agent
        condition:
          output_json_path: "$.confidence"
          greater_than: "0.9"
      - to: general-agent
        condition:
          default: true
```

This pairs naturally with `output_schema` on the classifier agent to guarantee valid JSON output. See [Structured Output](#structured-output) below.

**Iterative refinement with exit conditions** -- a review loop that terminates on quality, not just a turn counter:

```yaml
graph:
  writer-agent:
    edges:
      - to: critic-agent
  critic-agent:
    edges:
      - to: writer-agent
        condition:
          output_contains: "REVISION_NEEDED"
      - to: publisher-agent
        condition:
          output_contains: "APPROVED"
```

Conditional routing requires **message-driven** execution mode (`--task-execution-mode=message-driven`).

## Structured Output

Agents can declare an `output_schema` in their execution spec to constrain model responses to valid JSON matching a specific schema. This uses provider-native structured output features (constrained decoding) for guaranteed schema compliance.

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: classifier-agent
spec:
  model_ref: openai-gpt4
  prompt: Classify the incoming request.
  execution:
    output_schema:
      type: object
      properties:
        route:
          type: string
          enum: [research, engineering, legal]
        confidence:
          type: number
        domains:
          type: array
          items:
            type: string
      required: [route, confidence, domains]
      additionalProperties: false
```

### Provider Support

| Provider | Structured Output | Mechanism |
|---|---|---|
| OpenAI | Full schema enforcement | `response_format.json_schema` with constrained decoding |
| OpenAI-compatible | Full schema enforcement | Same as OpenAI |
| Azure OpenAI | Full schema enforcement | Same as OpenAI |
| Anthropic | Full schema enforcement | `output_config.format` with constrained decoding |
| Ollama | Best-effort JSON | `format` field with schema; enforcement depends on model capabilities |

### Combining with JSON Path Conditions

Structured output and JSON path conditions are designed to work together. The `output_schema` guarantees the agent produces valid, typed JSON, and `output_json_path` conditions route on the structured fields:

```yaml
# Agent definition
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: classifier-agent
spec:
  model_ref: openai-gpt4
  prompt: Classify the request and assess confidence.
  execution:
    output_schema:
      type: object
      properties:
        route:
          type: string
          enum: [research, engineering, legal]
        confidence:
          type: number
      required: [route, confidence]
      additionalProperties: false

---
# AgentSystem graph routing on structured output
graph:
  classifier-agent:
    edges:
      - to: research-lead
        condition:
          output_json_path: "$.route"
          equals: "research"
      - to: engineering-lead
        condition:
          output_json_path: "$.route"
          equals: "engineering"
      - to: legal-lead
        condition:
          output_json_path: "$.route"
          equals: "legal"
```

## Delegation

The `delegates` field on a graph node gives an agent two-phase execution: **dispatch** to downstream agents, **collect** their reports, **review** with all results, and **forward** onward.

```yaml
graph:
  vp-engineering:
    delegates:
      - to: backend-lead
        condition:
          output_json_path: "$.teams"
          contains: "backend"
      - to: security-lead
        condition:
          output_json_path: "$.risk_level"
          equals: "high"
    delegate_join:
      mode: wait_for_all
    edges:
      - to: synthesizer
```

### How Delegation Works

1. **Dispatch phase**: The node executes. Its output filters the `delegates` list (same condition logic as edges). Matched delegates receive messages with `delegate_of` metadata pointing back to the delegator.
2. **Collect phase**: Delegates execute and report back. A delegation gate collects returns using the same join modes as regular fan-in (`wait_for_all`, `quorum`).
3. **Review phase**: Once enough delegates return, the node **re-executes** with all delegate outputs in context via `inbox.delegation.*` keys.
4. **Forward phase**: After the review execution, normal `edges` fire for onward routing.

### Automatic Return Routing

When a delegate-activated agent reaches a terminal point (no outgoing edges match), the runtime automatically routes back to the delegator. The `delegate_of` field propagates through sub-edges, so multi-hop delegate branches still return to the correct delegator.

- **Simple delegate** (leaf node): executes once, reports back
- **Delegate with edges**: follows its own edges; the terminal node reports back
- **Delegate with its own delegates** (nested): inner delegation completes first, then the delegate reviews and reports back

### Agent Context

The agent sees delegation context through `inbox.*` keys:

- **Dispatch phase**: Normal `inbox.from`, `inbox.content` — no delegation keys
- **Review phase**: `inbox.delegation.enabled: true`, `inbox.delegation.mode`, `inbox.delegation.sources`, `inbox.delegation.payloads`

### Delegation Join Modes

`delegate_join` supports the same modes as regular join gates:

| Mode | Behavior |
|---|---|
| `wait_for_all` | Re-execute after every dispatched delegate returns. |
| `quorum` | Re-execute after `quorum_count` or `quorum_percent` of delegates return. |

### Patterns

**Hierarchical company** -- CEO delegates to VPs, VPs delegate to leads, each manager gets dispatch + review:

```yaml
graph:
  ceo-agent:
    delegates:
      - to: vp-engineering
        condition:
          output_json_path: "$.departments"
          contains: "engineering"
      - to: vp-product
        condition:
          output_json_path: "$.departments"
          contains: "product"
    delegate_join:
      mode: wait_for_all
    edges:
      - to: board-report-agent

  vp-engineering:
    delegates:
      - to: backend-lead
      - to: security-lead
    delegate_join:
      mode: wait_for_all
```

**Fast-response quorum** -- review fires after the first 2 of 3 analysts complete:

```yaml
graph:
  research-manager:
    delegates:
      - to: analyst-a
      - to: analyst-b
      - to: analyst-c
    delegate_join:
      mode: quorum
      quorum_count: 2
```

**Node with both join and delegates** -- composes correctly. The join gate fires first (collecting upstream), then the dispatch phase, then the delegation gate, then the review phase, then edges.

## Labels

Labels on AgentSystem metadata follow Kubernetes conventions and are useful for filtering, governance scoping, and operational grouping:

```yaml
metadata:
  labels:
    orloj.dev/domain: reporting
    orloj.dev/usecase: weekly-report
    orloj.dev/env: dev
```

## Related

- [Agent](./agent.md) -- the individual agents that compose a system
- [Task](../tasks/task.md) -- how to execute an AgentSystem
- [Resource Reference: AgentSystem](../../reference/resources/agent-system.md)
- [Execution and Messaging](../execution-model.md)
- [Starter Blueprints](../../guides/starter-blueprints.md)
