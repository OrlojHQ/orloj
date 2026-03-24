# AgentSystem

An **AgentSystem** composes multiple [Agents](./agent.md) into a directed graph that Orloj executes as a coordinated workflow. The graph defines how messages flow between agents during task execution.

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
