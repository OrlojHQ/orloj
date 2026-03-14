# Starter Blueprints

This page maps common orchestration patterns to runnable manifests in `examples/blueprints/`.

## Patterns

1. Pipeline
- Purpose: predictable stage-by-stage execution.
- Topology: `planner -> research -> writer`.
- Files: `examples/blueprints/pipeline/`.

2. Hierarchical
- Purpose: manager-led delegation through sub-managers/leads.
- Topology: `manager -> leads -> workers -> editor`.
- Files: `examples/blueprints/hierarchical/`.
- Access-control example: no direct manager edge to `bp-hier-social-worker-agent`.

3. Swarm + Loop
- Purpose: parallel scout exploration with iterative coordinator refinement.
- Topology: `coordinator <-> scouts` and `coordinator -> synthesizer`.
- Files: `examples/blueprints/swarm-loop/`.
- Safety: bounded by `Task.spec.max_turns`.

## Runtime Requirements

Run with message handoffs enabled:

1. `--task-execution-mode=message-driven`
2. non-`none` agent message bus backend (`memory` or `nats-jetstream`)
3. worker inbox consumers enabled (`--agent-message-consume`)

## Quick Apply

Use the blueprint guide at:

- [`examples/blueprints/README.md`](../../examples/blueprints/README.md)

That file includes exact `orlojctl apply -f ...` commands for each blueprint.
