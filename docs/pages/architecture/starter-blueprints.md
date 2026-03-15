# Starter Blueprints

This guide maps common orchestration patterns to runnable manifests in `examples/blueprints/`.

## Available Patterns

1. Pipeline
- Purpose: predictable stage-by-stage execution.
- Topology: `planner -> research -> writer`.
- Files: `examples/blueprints/pipeline/`.

2. Hierarchical
- Purpose: manager-led delegation.
- Topology: `manager -> leads -> workers -> editor`.
- Files: `examples/blueprints/hierarchical/`.

3. Swarm and Loop
- Purpose: parallel exploration with iterative coordination.
- Topology: `coordinator <-> scouts` and `coordinator -> synthesizer`.
- Files: `examples/blueprints/swarm-loop/`.
- Safety boundary: bounded by `Task.spec.max_turns`.

## Runtime Requirements

- `--task-execution-mode=message-driven`
- non-`none` `--agent-message-bus-backend` (`memory` or `nats-jetstream`)
- worker consumer enabled with `--agent-message-consume`

## Apply Blueprints

See:

- [`examples/blueprints/README.md`](../../../examples/blueprints/README.md)

That guide includes the exact `orlojctl apply -f ...` commands for each blueprint.
