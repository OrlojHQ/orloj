# Deploy Your First Pipeline

This guide is for platform engineers who want to run a multi-agent pipeline end-to-end. You will define three agents, wire them into a sequential graph, submit a task, and inspect the results.

## Prerequisites

- Orloj server (`orlojd`) running (sequential mode with `--embedded-worker` is fine for this guide)
- `orlojctl` available (or `go run ./cmd/orlojctl`)

If you have not set up Orloj yet, follow the [Install](../getting-started/install.md) and [Quickstart](../getting-started/quickstart.md) guides first.

## What You Will Build

A three-stage pipeline where each agent hands off to the next:

```
planner ──► researcher ──► writer
```

The planner breaks the task into research requirements, the researcher gathers evidence, and the writer produces the final output.

## Step 1: Define the Agents

Create three agent manifests. Each agent has a model, a system prompt, and execution limits.

**Planner agent** (`planner-agent.yaml`):
```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: bp-pipeline-planner-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the planning stage.
    Break the task into concrete research and writing requirements.
  limits:
    max_steps: 4
    timeout: 20s
```

**Research agent** (`research-agent.yaml`):
```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: bp-pipeline-research-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the research stage.
    Produce concise, verifiable findings for the writer.
  limits:
    max_steps: 6
    timeout: 30s
```

**Writer agent** (`writer-agent.yaml`):
```yaml
apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: bp-pipeline-writer-agent
spec:
  model_ref: openai-default
  prompt: |
    You are the writing stage.
    Synthesize prior handoffs into a polished final output.
  limits:
    max_steps: 4
    timeout: 20s
```

Apply all three:
```bash
orlojctl apply -f planner-agent.yaml
orlojctl apply -f research-agent.yaml
orlojctl apply -f writer-agent.yaml
```

## Step 2: Define the Agent System

The AgentSystem wires the agents into a pipeline graph:

```yaml
apiVersion: orloj.dev/v1
kind: AgentSystem
metadata:
  name: bp-pipeline-system
  labels:
    orloj.dev/pattern: pipeline
spec:
  agents:
    - bp-pipeline-planner-agent
    - bp-pipeline-research-agent
    - bp-pipeline-writer-agent
  graph:
    bp-pipeline-planner-agent:
      edges:
        - to: bp-pipeline-research-agent
    bp-pipeline-research-agent:
      edges:
        - to: bp-pipeline-writer-agent
```

The `graph` field defines a directed acyclic graph. Each node lists its outbound edges. The planner routes to the researcher, who routes to the writer. The writer has no outbound edges, making it the terminal node.

Apply the system:
```bash
orlojctl apply -f agent-system.yaml
```

## Step 3: Submit a Task

Create a task that targets the pipeline system:

```yaml
apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: bp-pipeline-task
spec:
  system: bp-pipeline-system
  input:
    topic: state of enterprise AI copilots
  priority: high
  retry:
    max_attempts: 2
    backoff: 2s
  message_retry:
    max_attempts: 2
    backoff: 250ms
    max_backoff: 2s
    jitter: full
```

Apply the task:
```bash
orlojctl apply -f task.yaml
```

## Step 4: Monitor Execution

Watch the task progress:
```bash
orlojctl get tasks -w
```

View agent logs:
```bash
orlojctl logs task/bp-pipeline-task
```

Trace the execution path through the graph:
```bash
orlojctl trace task bp-pipeline-task
```

Visualize the system graph:
```bash
orlojctl graph system bp-pipeline-system
```

## What Happens at Runtime

1. The scheduler assigns `bp-pipeline-task` to an available worker.
2. The worker claims the task and acquires a lease.
3. The planner agent runs first (entry node -- zero indegree in the graph).
4. The planner's output is routed as a message to the research agent.
5. The research agent processes the message and routes its output to the writer.
6. The writer produces the final output. With no further edges, the task transitions to `Succeeded`.

If any agent fails, the message-level retry configuration kicks in. After `max_attempts` exhaustion, the message moves to `deadletter`. If the task-level retry is also exhausted, the task itself transitions to `DeadLetter`.

## Using the Pre-Built Blueprint

The complete pipeline blueprint is available in the repository:

```bash
orlojctl apply -f examples/blueprints/pipeline/agents/
orlojctl apply -f examples/blueprints/pipeline/agent-system.yaml
orlojctl apply -f examples/blueprints/pipeline/task.yaml
```

## Next Steps

- [Starter Blueprints](../architecture/starter-blueprints.md) -- explore hierarchical and swarm-loop topologies
- [Set Up Multi-Agent Governance](./setup-governance.md) -- add policies and permissions to your pipeline
- [Tasks and Scheduling](../concepts/tasks-and-scheduling.md) -- understand the full task lifecycle
