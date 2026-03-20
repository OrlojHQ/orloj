# Starter Blueprints

Blueprints are ready-to-run templates that combine agents, an agent system (the graph), and a task into a single directory. They are the fastest way to see Orloj in action and to understand each orchestration pattern.

For copy-paste **use case** bundles (YAML + README per scenario) that map these patterns to novice and enterprise-style problems, see [examples/use-cases/](../../../examples/use-cases/README.md).

## Available Patterns

### Pipeline

Predictable stage-by-stage execution: `planner -> research -> writer`.

```bash
orlojctl apply -f examples/blueprints/pipeline/
```

### Hierarchical

Manager-led delegation: `manager -> leads -> workers -> editor`.

```bash
orlojctl apply -f examples/blueprints/hierarchical/
```

### Swarm and Loop

Parallel exploration with iterative coordination: `coordinator <-> scouts -> synthesizer`. Safety-bounded by `Task.spec.max_turns`.

```bash
orlojctl apply -f examples/blueprints/swarm-loop/
```

## Runtime Compatibility

Blueprints work in both execution modes:

- **Sequential** -- run with `--embedded-worker` for single-process development. Good for getting started.
- **Message-driven** -- run with `--agent-message-bus-backend=memory` (or `nats-jetstream`) and `--agent-message-consume` for distributed execution. Required for parallel fan-out in the swarm-loop pattern.

## What is Inside a Blueprint

Each blueprint directory contains:

- `agents/*.yaml` -- individual Agent resources with prompts, model config, and tool bindings.
- `agent-system.yaml` -- the AgentSystem resource defining the graph topology (nodes and edges).
- `task.yaml` -- a Task resource that targets the agent system with sample input.

Apply the entire directory with `orlojctl apply -f <path>/` to create all resources at once.
