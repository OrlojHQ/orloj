# Agents and Agent Systems

An **Agent** is a declarative unit of work backed by a language model. An **AgentSystem** composes multiple agents into a directed graph that Orloj executes as a coordinated workflow.

## Agents

An Agent manifest defines what the agent does (its prompt), what model powers it, what tools it can call, and what constraints bound its execution.

```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: research-agent
spec:
  model_ref: openai-default
  prompt: |
    You are a research assistant.
    Produce concise evidence-backed answers.
  tools:
    - web_search
    - vector_db
  memory:
    ref: research-memory
  roles:
    - analyst-role
  limits:
    max_steps: 6
    timeout: 30s
```

### Key Fields

| Field | Description |
|---|---|
| `model` | Direct model identifier (e.g. `gpt-4o`). Defaults to `gpt-4o-mini` if neither `model` nor `model_ref` is set. |
| `model_ref` | Reference to a [ModelEndpoint](./model-routing.md) resource for provider-aware routing. |
| `prompt` | The system instruction that defines the agent's behavior. |
| `tools` | List of [Tool](./tools-and-isolation.md) names this agent may call. Tool calls are subject to governance checks. |
| `roles` | Bound [AgentRole](./governance.md) names. Roles carry permissions that authorize tool usage. |
| `memory.ref` | Reference to a Memory resource for vector-backed retrieval. |
| `limits.max_steps` | Maximum execution steps per task turn. Defaults to `10`. |
| `limits.timeout` | Maximum wall-clock time per task turn. |

### How an Agent Executes

When the runtime activates an agent during a task, it:

1. Loads the agent's prompt and any memory context.
2. Routes the request to the configured model via the model gateway.
3. If the model selects tool calls, the runtime checks governance (AgentPolicy, AgentRole, ToolPermission) and executes authorized tools.
4. Results flow back to the model for the next step, up to `max_steps` or `timeout`.

## Agent Systems

An AgentSystem wires agents into a directed graph. The graph defines how messages flow between agents during task execution.

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

### Graph Topologies

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

### Fan-out and Fan-in

When a graph node has multiple outbound edges, messages fan out to all targets in parallel. Fan-in is handled through join gates:

| Join Mode | Behavior |
|---|---|
| `wait_for_all` | Waits for every upstream branch to complete before activating the join node. |
| `quorum` | Activates after `quorum_count` or `quorum_percent` of upstream branches complete. |

If an upstream branch fails, the `on_failure` policy determines behavior: `deadletter` (default), `skip`, or `continue_partial`.

### Labels

Labels on AgentSystem metadata follow Kubernetes conventions and are useful for filtering, governance scoping, and operational grouping:

```yaml
metadata:
  labels:
    orloj.dev/domain: reporting
    orloj.dev/usecase: weekly-report
    orloj.dev/env: dev
```

## Related Resources

- [Resource Reference: Agent and AgentSystem](../reference/resources.md)
- [Execution and Messaging](../architecture/execution-model.md)
- [Starter Blueprints](../architecture/starter-blueprints.md)
- [Guide: Deploy Your First Pipeline](../guides/deploy-pipeline.md)
